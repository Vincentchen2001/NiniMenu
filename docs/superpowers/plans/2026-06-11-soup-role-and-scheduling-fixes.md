# Soup Role Tightening & Scheduling Controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the over-broad soup detection (198 → ~60 real soups), cap slow soups at 2/week, and stop adjacent days from repeating the same soup main ingredient.

**Architecture:** A new `models.IsSoupName(name, category)` becomes the single source of truth for "is this dish a soup" (category contains 汤 OR name ends with 汤/羹); both `inferDishRole` and `isSoupDish` rewire to it, and `DishTraitVersion` bumps to 2 so startup `backfillDishTraits` re-labels all auto-inferred rows. Two new default menu rules (seed v3 → v4) implement the weekly cap and the adjacent-day repeat penalty; the latter needs a new `prev_soups` env list + `countOverlapPrevSoup` helper threaded from the week-plan generator (sequential days pass the previous day's soups; single-day regeneration passes both neighbors).

**Tech Stack:** Go (Gin + GORM + expr-lang), React/TS frontend, SQLite. Backend tests: `go test ./...` from `backend/`.

**Spec:** `docs/superpowers/specs/2026-06-11-soup-role-and-scheduling-fixes-design.md`

**Two deliberate deviations from the spec (both simplifications, same semantics):**
1. Spec §4 sketched a `countWeekSlowSoup(minutes)` helper. Not needed: the template
   renderer already emits `len(filter(week, ...))` expressions for the `slow` and
   `spicy` categories, so rule A reuses that pattern with zero new env surface.
2. Spec §5 sketched a `no_repeat`/`prev_day` template. The template validator only
   accepts scope `meal|day|week`, and the `no_repeat` renderer is protein-specific.
   Rule B uses the `avoid` template type instead (exactly like the three v3 penalty
   rules) — same rendered expression, no renderer surgery.

**Branch policy:** work directly on `main` (established working mode for this repo — v0.7.1 shipped the same way). Commit after every task.

---

## File Structure

| File | Responsibility / change |
|---|---|
| `backend/internal/models/dish_traits.go` | + `IsSoupName`; `inferDishRole` soup branch uses it; `DishTraitVersion` 1→2 |
| `backend/internal/models/dish_traits_test.go` | NEW — `IsSoupName` table + role-inference boundary table |
| `backend/internal/services/week_plan_preferences.go` | `isSoupDish` → thin wrapper; delete `soupDishKeywords` + `containsAnyKeyword` |
| `backend/internal/services/menu_rule_service.go` | `buildRuleEnv` + `prevSoups` param, `prev_soups` env, `countOverlapPrevSoup` closure; `ruleDishFieldMatches` ingredients case; aromatics blocklist; `menuRulesSeedVersion` 3→4 |
| `backend/internal/services/menu_rule_templates.go` | `slow_soup` + `soup_ingredient_repeat` category cases |
| `backend/internal/models/menu_rules.go` | 2 new default rules (priority 290/300) |
| `backend/internal/services/plan_service.go` | `weekPlanDayContext.prevSoups`; `soupsFromDishes` + `adjacentPlannedSoups`; `generateDay` threading; both `buildRuleEnv` call sites |
| `backend/internal/services/plan_service_test.go` | `TestIsSoupDish` update; `TestAdjacentPlannedSoups`; `TestSoupSchedulingRulesViaScoreEvaluation` |
| `backend/internal/services/menu_rule_templates_test.go` | env-helper test; renderer test; drift 12→14; migration test updates + new v3→v4 test |
| `frontend/src/lib/menuRuleTemplates.ts` | 2 display labels |
| `frontend/src/components/rules/PresetRuleCard.tsx` | `hasPoints` includes prefer-strength limit rules |
| `CHANGELOG.md` | v0.7.2 section |

Task order matters: Task 3 changes `buildRuleEnv`'s signature, so it mechanically updates all call sites (passing `nil` from plan_service); Task 6 replaces those `nil`s with real data. Task 5 (default rules) needs Task 3 (env helper must exist for `compileMenuRule` in the drift test) and Task 4 (renderer cases must exist for template re-rendering).

---

### Task 1: `models.IsSoupName` + role-inference tightening

**Files:**
- Create: `backend/internal/models/dish_traits_test.go`
- Modify: `backend/internal/models/dish_traits.go`

- [ ] **Step 1: Write the failing tests**

Create `backend/internal/models/dish_traits_test.go` (the `models` package has no test files yet):

```go
package models

import "testing"

func TestIsSoupName(t *testing.T) {
	tests := []struct {
		name     string
		dishName string
		category string
		want     bool
	}{
		{"category soup", "番茄豆腐汤", "汤品", true},
		{"name suffix 汤 in cuisine category", "莲藕花生排骨汤", "粤菜", true},
		{"name suffix 羹", "西湖牛肉羹", "杭帮菜", true},
		{"mid-name 汤 is not soup", "酸汤鱼", "贵州菜", false},
		{"mid-name 汤 staple", "黑芝麻汤圆", "主食", false},
		{"上汤 prefix veg dish", "上汤娃娃菜", "粤菜", false},
		{"煲 is not soup", "咸鱼鸡粒茄子煲", "粤菜", false},
		{"plain meat dish", "红烧排骨", "家常菜", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSoupName(tt.dishName, tt.category); got != tt.want {
				t.Fatalf("IsSoupName(%q, %q) = %v, want %v", tt.dishName, tt.category, got, tt.want)
			}
		})
	}
}

// TestInferDishTraitsSoupRole pins the boundary table from the design spec:
// step/remark/tag text and mid-name 汤 must stop producing role=soup, while
// suffix-named and 汤品-categorized soups keep it.
func TestInferDishTraitsSoupRole(t *testing.T) {
	tests := []struct {
		name string
		dish Dish
		want string
	}{
		{
			name: "step text 汤汁 no longer makes soup",
			dish: Dish{Name: "红烧排骨", Category: "家常菜", Taste: "酱香", Tags: `["家常菜","硬菜"]`,
				Ingredients: `[{"name":"猪肋排","amount":"600克"}]`,
				Steps:       `[{"text":"倒热水没过排骨，小火炖35分钟，开盖收浓汤汁。","time":35}]`},
			want: "meat",
		},
		{
			name: "remark 原汤 no longer makes soup",
			dish: Dish{Name: "清蒸鲈鱼", Category: "粤菜", Taste: "鲜", Remark: "蒸好后倒掉原汤，腥味更轻。",
				Ingredients: `[{"name":"鲈鱼","amount":"1条"}]`},
			want: "meat",
		},
		{
			name: "mid-name 汤 cuisine main",
			dish: Dish{Name: "酸汤鱼", Category: "贵州菜", Ingredients: `[{"name":"草鱼","amount":"1条"}]`},
			want: "meat",
		},
		{
			name: "汤圆 is staple",
			dish: Dish{Name: "黑芝麻汤圆", Category: "主食", Ingredients: `[{"name":"糯米粉","amount":"200克"}]`},
			want: "staple",
		},
		{
			name: "煲仔饭 is staple",
			dish: Dish{Name: "腊味煲仔饭", Category: "粤菜", Ingredients: `[{"name":"米饭","amount":"2碗"},{"name":"腊肠","amount":"2根"}]`},
			want: "staple",
		},
		{
			name: "tag 汤品 no longer makes soup",
			dish: Dish{Name: "开水白菜", Category: "川菜", Tags: `["川菜","晚餐","汤品"]`,
				Ingredients: `[{"name":"白菜","amount":"1棵"}]`},
			want: "veg",
		},
		{
			name: "name suffix 汤 stays soup",
			dish: Dish{Name: "莲藕花生排骨汤", Category: "粤菜",
				Ingredients: `[{"name":"排骨","amount":"300克"},{"name":"莲藕","amount":"1节"}]`},
			want: "soup",
		},
		{
			name: "category 汤品 stays soup",
			dish: Dish{Name: "紫菜蛋花汤", Category: "汤品",
				Ingredients: `[{"name":"紫菜","amount":"适量"},{"name":"鸡蛋","amount":"1个"}]`},
			want: "soup",
		},
		{
			name: "name suffix 羹 stays soup",
			dish: Dish{Name: "西湖牛肉羹", Category: "杭帮菜", Ingredients: `[{"name":"牛肉","amount":"100克"}]`},
			want: "soup",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferDishTraits(tt.dish)
			if got.DishRole != tt.want {
				t.Fatalf("InferDishTraits(%s).DishRole = %q, want %q", tt.dish.Name, got.DishRole, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/models/ -run 'TestIsSoupName|TestInferDishTraitsSoupRole' -v`
Expected: FAIL — `undefined: IsSoupName`, and after that compiles, the role cases like 红烧排骨 return `"soup"`.

- [ ] **Step 3: Implement in `dish_traits.go`**

Three edits.

(a) Bump the version (line 8) — the inference algorithm changed, so cached auto traits are stale:

```go
const DishTraitVersion = 2
```

(b) In `InferDishTraits`, change the `inferDishRole` call (line 18):

```go
	role := inferDishRole(dish.Name, dish.Category, text, tags, proteins)
```

(c) Replace the `inferDishRole` function (lines 137-156) with:

```go
// IsSoupName reports whether a dish is a soup in the meal-slot sense (a
// drinkable accompaniment). Only the category and the name SUFFIX count:
// mid-name 汤 (酸汤鱼, 上汤娃娃菜, 汤圆), tags, remark, ingredient and step
// texts deliberately do not — those describe form or technique, not the
// dish's role on the table.
func IsSoupName(name, category string) bool {
	return strings.Contains(category, "汤") ||
		strings.HasSuffix(name, "汤") ||
		strings.HasSuffix(name, "羹")
}

func inferDishRole(name string, category string, text string, tags []string, proteins []string) string {
	if IsSoupName(name, category) {
		return "soup"
	}
	if containsAnyDishTrait(text, "主食", "米饭", "炒饭", "盖饭", "面条", "米线", "米粉", "冷面", "饺子", "馄饨", "馒头", "饼", "粥", "粉丝", "河粉") {
		return "staple"
	}
	if len(proteins) > 0 && !onlySoyProtein(proteins) {
		return "meat"
	}
	for _, tag := range tags {
		if tag == "素菜" || tag == "纯素" || tag == "蔬菜" || tag == "青菜" {
			return "veg"
		}
	}
	if containsAnyDishTrait(text, "青菜", "白菜", "生菜", "菠菜", "黄瓜", "茄子", "土豆", "番茄", "豆腐", "蘑菇", "木耳", "莲藕", "萝卜", "西兰花", "花菜") {
		return "veg"
	}
	return "side"
}
```

(All other branches keep using the full trait text — only the soup branch changed.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/models/ -run 'TestIsSoupName|TestInferDishTraitsSoupRole' -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Build the whole backend (other packages reference these symbols indirectly)**

Run: `cd backend && go build ./... && go test ./internal/models/`
Expected: build OK, models tests PASS. (`./internal/services` tests may fail at this point only if they pinned old behavior — they are fixed in Task 2; check the failure list mentions only `TestIsSoupDish`-related cases before proceeding. If other services tests fail, stop and investigate.)

- [ ] **Step 6: Commit**

```bash
git add backend/internal/models/dish_traits.go backend/internal/models/dish_traits_test.go
git commit -m "Tighten soup role inference to category/name-suffix only"
```

---

### Task 2: `isSoupDish` rewires to the same truth

**Files:**
- Modify: `backend/internal/services/week_plan_preferences.go:206-225`
- Modify: `backend/internal/services/plan_service_test.go` (TestIsSoupDish, around line 266)

- [ ] **Step 1: Update the test table**

In `plan_service_test.go`, replace the entire `TestIsSoupDish` test cases slice (keep the surrounding func) with:

```go
	tests := []struct {
		name string
		dish models.Dish
		want bool
	}{
		{
			name: "soup category",
			dish: models.Dish{Name: "番茄豆腐汤", Category: "汤品", Tags: `[]`, Ingredients: `[{"name":"番茄","amount":"1个"},{"name":"豆腐","amount":"1块"}]`},
			want: true,
		},
		{
			name: "soup tag no longer counts",
			dish: models.Dish{Name: "菌菇煲", Tags: `["汤品"]`, Ingredients: `[{"name":"香菇","amount":"50g"}]`},
			want: false,
		},
		{
			name: "name suffix",
			dish: models.Dish{Name: "紫菜蛋花汤", Tags: `[]`, Ingredients: `[{"name":"紫菜","amount":"适量"},{"name":"鸡蛋","amount":"1个"}]`},
			want: true,
		},
		{
			name: "mid-name 汤 no longer counts",
			dish: models.Dish{Name: "酸汤鱼", Category: "贵州菜", Tags: `[]`, Ingredients: `[{"name":"草鱼","amount":"1条"}]`},
			want: false,
		},
		{
			name: "name suffix 羹",
			dish: models.Dish{Name: "西湖牛肉羹", Category: "杭帮菜", Tags: `[]`, Ingredients: `[{"name":"牛肉","amount":"100克"}]`},
			want: true,
		},
		{
			name: "non soup egg dish",
			dish: models.Dish{Name: "番茄炒蛋", Tags: `[]`, Ingredients: `[{"name":"番茄","amount":"2个"},{"name":"鸡蛋","amount":"2个"}]`},
			want: false,
		},
	}
```

- [ ] **Step 2: Run to verify failures**

Run: `cd backend && go test ./internal/services/ -run TestIsSoupDish -v`
Expected: FAIL on "soup tag no longer counts" and "mid-name 汤 no longer counts" (old keyword logic returns true).

- [ ] **Step 3: Implement**

In `week_plan_preferences.go`, replace lines 206-225 (`isSoupDish`, `containsAnyKeyword`, `soupDishKeywords` — `containsAnyKeyword` has no other callers, verified) with:

```go
// isSoupDish mirrors models.IsSoupName so slot matching, theme bonuses and
// trait inference agree on what counts as a soup.
func isSoupDish(dish models.Dish) bool {
	return models.IsSoupName(dish.Name, dish.Category)
}
```

Then check whether `parseTags` and `strings` are still used elsewhere in this file; remove the `strings` import only if the compiler says it is unused.

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./internal/services/ -run TestIsSoupDish -v && go build ./...`
Expected: PASS, build OK.

- [ ] **Step 5: Run the full services suite to surface collateral pins**

Run: `cd backend && go test ./internal/services/`
Expected: PASS. If a light-profile or theme test fails because a fixture dish relied on tag/keyword soup detection, fix the fixture to use a 汤-suffixed name or 汤品 category (behavior change is intended; fixtures follow).

- [ ] **Step 6: Commit**

```bash
git add backend/internal/services/week_plan_preferences.go backend/internal/services/plan_service_test.go
git commit -m "Rewire isSoupDish to models.IsSoupName"
```

---

### Task 3: rule env — `prev_soups` + `countOverlapPrevSoup`

**Files:**
- Modify: `backend/internal/services/menu_rule_service.go`
- Modify: `backend/internal/services/plan_service.go:519,540` (mechanical, pass `nil`)
- Modify: `backend/internal/services/menu_rule_templates_test.go:59` (mechanical)
- Test: `backend/internal/services/menu_rule_templates_test.go`

- [ ] **Step 1: Write the failing test**

Append to `menu_rule_templates_test.go`:

```go
func TestCountOverlapPrevSoupHelper(t *testing.T) {
	prevSoups := []ruleDishEnv{{
		Name: "萝卜丝鲫鱼汤", DishRole: "soup",
		Ingredients: []string{"鲫鱼", "萝卜", "姜", "葱"},
	}}
	candidate := ruleDishEnv{Name: "鲫鱼豆腐汤", DishRole: "soup", Ingredients: []string{"鲫鱼", "豆腐", "姜"}}
	env := buildRuleEnv(candidate, nil, nil, nil, prevSoups, "balanced", MealQuota{}, false)

	fn, ok := env["countOverlapPrevSoup"].(func(string, []string) int)
	if !ok {
		t.Fatalf("countOverlapPrevSoup helper missing from env")
	}
	if got := fn("ingredients", candidate.Ingredients); got != 1 {
		t.Fatalf("countOverlapPrevSoup(鲫鱼豆腐汤) = %d, want 1 (shared 鲫鱼)", got)
	}

	// Shared 姜/葱 are aromatics — they must not count as a repeat.
	aromaticsOnly := []string{"番茄", "鸡蛋", "姜", "葱"}
	if got := fn("ingredients", aromaticsOnly); got != 0 {
		t.Fatalf("countOverlapPrevSoup(aromatics only) = %d, want 0", got)
	}

	// Empty prev_soups (Monday of a fresh week) is inert.
	emptyEnv := buildRuleEnv(candidate, nil, nil, nil, nil, "balanced", MealQuota{}, false)
	emptyFn := emptyEnv["countOverlapPrevSoup"].(func(string, []string) int)
	if got := emptyFn("ingredients", candidate.Ingredients); got != 0 {
		t.Fatalf("countOverlapPrevSoup(no prev soups) = %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./internal/services/ -run TestCountOverlapPrevSoupHelper -v`
Expected: COMPILE FAIL — `buildRuleEnv` takes 7 args, test passes 8.

- [ ] **Step 3: Implement in `menu_rule_service.go`**

(a) New signature + env entries — replace the `buildRuleEnv` opening (line 370) and add the two entries after `"is_weekend"`:

```go
func buildRuleEnv(candidate ruleDishEnv, meal []ruleDishEnv, day []ruleDishEnv, week []ruleDishEnv, prevSoups []ruleDishEnv, profile string, quota MealQuota, isWeekend bool) map[string]any {
	return map[string]any{
		"candidate":  candidate,
		"meal":       meal,
		"day":        day,
		"week":       week,
		"prev_soups": prevSoups,
		"profile":    normalizePlanProfile(profile),
		"is_weekend": isWeekend,
```

and alongside the other helper closures (after `countOverlapWeek`):

```go
		"countOverlapPrevSoup": func(field string, values []string) int {
			return countRuleDishOverlap(prevSoups, field, filterSoupAromatics(values))
		},
```

(b) `sampleRuleEnv` (line 367) gains the new arg:

```go
	return buildRuleEnv(dish, []ruleDishEnv{}, []ruleDishEnv{}, []ruleDishEnv{}, []ruleDishEnv{}, "balanced", MealQuota{MeatCount: 1}, false)
```

(c) `ruleDishFieldMatches` (line 475) gains an ingredients case before `default`:

```go
	case "ingredients":
		return containsString(dish.Ingredients, value)
```

(d) Blocklist + filter, placed right after `countRuleDishOverlap`:

```go
// soupAromaticsBlocklist lists base aromatics, seasonings and liquids that
// must not make two soups count as sharing a main ingredient.
var soupAromaticsBlocklist = map[string]bool{
	"葱": true, "小葱": true, "大葱": true, "葱花": true, "香葱": true, "葱段": true,
	"姜": true, "生姜": true, "姜片": true, "姜丝": true,
	"蒜": true, "大蒜": true, "蒜末": true, "蒜瓣": true,
	"香菜": true, "盐": true, "食盐": true, "糖": true, "白糖": true, "冰糖": true,
	"食用油": true, "油": true, "香油": true, "芝麻油": true, "猪油": true,
	"料酒": true, "生抽": true, "老抽": true, "醋": true,
	"胡椒": true, "白胡椒": true, "胡椒粉": true, "白胡椒粉": true,
	"鸡精": true, "味精": true, "枸杞": true, "红枣": true, "大枣": true,
	"八角": true, "花椒": true, "桂皮": true, "香叶": true,
	"清水": true, "热水": true, "温水": true, "开水": true, "高汤": true, "清汤": true,
}

func filterSoupAromatics(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if soupAromaticsBlocklist[strings.TrimSpace(value)] {
			continue
		}
		result = append(result, value)
	}
	return result
}
```

(e) Mechanical call-site updates so the package compiles (verified full list — these are the only other callers):
- `plan_service.go:519` and `:540`: insert `nil,` between the week arg and `dayCtx.profile`:
  `..., dishesRuleEnv(appendDishSlices(weekPicked, mealPicked), dayCtx.categoryFavorites), nil, dayCtx.profile, quota, dayCtx.isWeekend)` (Task 6 replaces the `nil` with real prev-soup data)
- `menu_rule_templates_test.go:59` (the local `buildEnv` helper): insert `[]ruleDishEnv{},` in the same position.

- [ ] **Step 4: Run tests**

Run: `cd backend && go build ./... && go test ./internal/services/ -run TestCountOverlapPrevSoupHelper -v`
Expected: build OK, PASS.

- [ ] **Step 5: Full services suite**

Run: `cd backend && go test ./internal/services/`
Expected: PASS (existing env tests unaffected — the new arg is additive).

- [ ] **Step 6: Commit**

```bash
git add backend/internal/services/menu_rule_service.go backend/internal/services/plan_service.go backend/internal/services/menu_rule_templates_test.go
git commit -m "Add prev_soups rule env with aromatics-filtered overlap helper"
```

---

### Task 4: template renderer — two new categories

**Files:**
- Modify: `backend/internal/services/menu_rule_templates.go:179-256`
- Test: `backend/internal/services/menu_rule_templates_test.go`

- [ ] **Step 1: Write the failing test**

Append to `menu_rule_templates_test.go`:

```go
func TestMenuRuleTemplateSoupSchedulingCategories(t *testing.T) {
	limitTpl := MenuRuleTemplate{Type: "limit", Scope: "week", Category: "slow_soup", N: 2, Points: 25, Strength: "prefer"}
	rendered, err := limitTpl.render()
	if err != nil {
		t.Fatalf("render(slow_soup limit) error = %v", err)
	}
	wantLimit := `candidate.dish_role == "soup" && candidate.cook_time > 45 && len(filter(week, .dish_role == "soup" && .cook_time > 45)) >= 2 ? -25 : 0`
	if rendered.Expression != wantLimit {
		t.Errorf("slow_soup limit expression:\ngot:  %s\nwant: %s", rendered.Expression, wantLimit)
	}
	if rendered.RuleKind != "score" || rendered.Severity != "soft" || !rendered.Relaxable || rendered.Scope != "week" {
		t.Errorf("slow_soup limit meta = %+v", rendered)
	}

	avoidTpl := MenuRuleTemplate{Type: "avoid", Category: "soup_ingredient_repeat", Points: 18, Strength: "prefer"}
	rendered, err = avoidTpl.render()
	if err != nil {
		t.Fatalf("render(soup_ingredient_repeat avoid) error = %v", err)
	}
	wantAvoid := `candidate.dish_role == "soup" && countOverlapPrevSoup("ingredients", candidate.ingredients) > 0 ? -18 : 0`
	if rendered.Expression != wantAvoid {
		t.Errorf("soup_ingredient_repeat expression:\ngot:  %s\nwant: %s", rendered.Expression, wantAvoid)
	}
	if rendered.RuleKind != "score" || rendered.Severity != "soft" || !rendered.Relaxable || rendered.Scope != "candidate" {
		t.Errorf("soup_ingredient_repeat meta = %+v", rendered)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./internal/services/ -run TestMenuRuleTemplateSoupSchedulingCategories -v`
Expected: FAIL — `规则模板类别无效: slow_soup`.

- [ ] **Step 3: Implement**

In `menuRuleCategoryPredicate`, after the `weekday_slow_soup` case (line 202):

```go
	case category == "slow_soup":
		return `candidate.dish_role == "soup" && candidate.cook_time > 45`, nil
	case category == "soup_ingredient_repeat":
		return `candidate.dish_role == "soup" && countOverlapPrevSoup("ingredients", candidate.ingredients) > 0`, nil
```

In `menuRuleCategoryCount`, after the `slow` case (line 238):

```go
	case category == "slow_soup":
		return fmt.Sprintf(`len(filter(%s, .dish_role == "soup" && .cook_time > 45))`, scope), nil
```

(`soup_ingredient_repeat` needs no count case — it is only used with the `avoid` type, which never calls `menuRuleCategoryCount`.)

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./internal/services/ -run 'TestMenuRuleTemplate' -v`
Expected: PASS (new test + all existing template tests).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/services/menu_rule_templates.go backend/internal/services/menu_rule_templates_test.go
git commit -m "Render slow_soup and soup_ingredient_repeat rule templates"
```

---

### Task 5: two default rules + seed v4 migration

**Files:**
- Modify: `backend/internal/models/menu_rules.go`
- Modify: `backend/internal/services/menu_rule_service.go:71`
- Modify/Test: `backend/internal/services/menu_rule_templates_test.go`

- [ ] **Step 1: Update existing tests + add the migration test (failing first)**

In `menu_rule_templates_test.go`:

(a) `TestDefaultMenuRuleTemplatesMatchExpressions` (line 268): change both occurrences of `12` to `14`:

```go
	if len(defaults) != 14 {
		t.Fatalf("DefaultMenuRules() has %d rules, want 14", len(defaults))
	}
```

(b) `TestEnsureDefaultMenuRulesMigratesV1Defaults`: append the two new codes to `wantCodes` (line 339):

```go
	wantCodes := []string{
		"avoid_double_egg", "limit_cold_dishes", "limit_staples",
		"avoid_same_primary_protein", "avoid_heavy_spicy_day",
		"spicy_meal_spread", "slow_meal_spread",
		"weekly_protein_variety", "weekly_fried_limit",
		"non_favorite_penalty", "unfamiliar_category_penalty", "weekday_slow_soup_penalty",
		"weekly_slow_soup_limit", "soup_ingredient_repeat_penalty",
	}
```

and update the version assertion (line 369):

```go
	if setting.Value != "4" {
		t.Errorf("seed version = %q, want 4", setting.Value)
	}
```

(c) `TestEnsureDefaultMenuRulesMigratesV2ToV3`: rename to `TestEnsureDefaultMenuRulesMigratesV2ToCurrent`; extend the seeding skip-list (line 392):

```go
		case "non_favorite_penalty", "unfamiliar_category_penalty", "weekday_slow_soup_penalty",
			"weekly_slow_soup_limit", "soup_ingredient_repeat_penalty":
			continue // v3/v4 additions don't exist in a v2 database
```

count assertion (line 425): `12` → `14` (message "after v3 migration" → "after migration"); the missing-codes loop (line 428) gains the two new codes:

```go
	for _, code := range []string{"non_favorite_penalty", "unfamiliar_category_penalty", "weekday_slow_soup_penalty", "weekly_slow_soup_limit", "soup_ingredient_repeat_penalty"} {
```

version assertion (line 461): `"3"` → `"4"`.

(d) Append the new focused v3→v4 test:

```go
func TestEnsureDefaultMenuRulesMigratesV3ToV4(t *testing.T) {
	setupPlanServiceTestDB(t)

	// Seed a v3-era database: all current defaults except the two v4
	// additions, with one user-modified rule and one rule toggled off.
	for _, rule := range models.DefaultMenuRules() {
		rule := rule
		switch rule.Code {
		case "weekly_slow_soup_limit", "soup_ingredient_repeat_penalty":
			continue // v4 additions don't exist in a v3 database
		}
		if rule.Code == "weekly_fried_limit" {
			rule.Expression = `has(candidate.cooking_methods, "deep_fry") && countWeek("cooking_methods", "deep_fry") >= 5 ? -20 : 0`
			rule.Template = ""
		}
		if err := database.DB.Create(&rule).Error; err != nil {
			t.Fatalf("seed v3 rule %s: %v", rule.Code, err)
		}
	}
	// Toggle a factory rule off the way the rules page does (column update);
	// Create alone would lose Enabled=false to the gorm default.
	if err := database.DB.Model(&models.MenuRule{}).Where("code = ?", "weekday_slow_soup_penalty").Update("enabled", false).Error; err != nil {
		t.Fatalf("disable weekday_slow_soup_penalty: %v", err)
	}
	if err := setSettingValue("menu_rules_seed_version", "3"); err != nil {
		t.Fatalf("set seed version: %v", err)
	}

	if err := EnsureDefaultMenuRules(); err != nil {
		t.Fatalf("EnsureDefaultMenuRules() error = %v", err)
	}

	var rules []models.MenuRule
	if err := database.DB.Find(&rules).Error; err != nil {
		t.Fatalf("load rules: %v", err)
	}
	got := menuRuleCodeSet(rules)
	if len(rules) != 14 {
		t.Fatalf("after v4 migration %d rules, want 14 (%v)", len(rules), got)
	}
	for _, code := range []string{"weekly_slow_soup_limit", "soup_ingredient_repeat_penalty"} {
		if _, ok := got[code]; !ok {
			t.Errorf("missing v4 rule %s after migration", code)
		}
	}
	wantModified := `has(candidate.cooking_methods, "deep_fry") && countWeek("cooking_methods", "deep_fry") >= 5 ? -20 : 0`
	if got["weekly_fried_limit"].Expression != wantModified {
		t.Errorf("user-modified rule overwritten:\ngot:  %s\nwant: %s", got["weekly_fried_limit"].Expression, wantModified)
	}
	if got["weekday_slow_soup_penalty"].Enabled {
		t.Errorf("user-disabled weekday_slow_soup_penalty was re-enabled by migration")
	}

	var setting models.Setting
	if err := database.DB.Where("`key` = ?", "menu_rules_seed_version").First(&setting).Error; err != nil {
		t.Fatalf("seed version setting missing: %v", err)
	}
	if setting.Value != "4" {
		t.Errorf("seed version = %q, want 4", setting.Value)
	}
}
```

- [ ] **Step 2: Run to verify failures**

Run: `cd backend && go test ./internal/services/ -run 'TestDefaultMenuRuleTemplatesMatchExpressions|TestEnsureDefaultMenuRules' -v`
Expected: FAIL — drift test wants 14 but gets 12; migration tests miss the new codes and find version "3".

- [ ] **Step 3: Implement**

(a) `backend/internal/models/menu_rules.go`: update the doc comment (line 3) `(seed version 3)` → `(seed version 4)`, then append two rules before the closing `}` of the slice (after `weekday_slow_soup_penalty`):

```go
		{
			Code:        "weekly_slow_soup_limit",
			Name:        "费时汤一周最多两道",
			Description: "一周菜单里炖煮超过 45 分钟的汤已有两道后，再排第三道会降分；大炖汤留给周末两天刚刚好。",
			Enabled:     true,
			Scope:       "week",
			RuleKind:    "score",
			Severity:    "soft",
			Relaxable:   true,
			Expression:  `candidate.dish_role == "soup" && candidate.cook_time > 45 && len(filter(week, .dish_role == "soup" && .cook_time > 45)) >= 2 ? -25 : 0`,
			Template:    `{"type":"limit","scope":"week","category":"slow_soup","n":2,"points":25,"strength":"prefer"}`,
			Priority:    290,
			Message:     "一周费时汤过多降分",
		},
		{
			Code:        "soup_ingredient_repeat_penalty",
			Name:        "今天的汤不和昨天撞主料",
			Description: "排今天的汤位时，候选汤和昨天的汤主料重叠（比如都是鲫鱼）会降分；葱姜蒜等基础配料不算主料。",
			Enabled:     true,
			Scope:       "candidate",
			RuleKind:    "score",
			Severity:    "soft",
			Relaxable:   true,
			Expression:  `candidate.dish_role == "soup" && countOverlapPrevSoup("ingredients", candidate.ingredients) > 0 ? -18 : 0`,
			Template:    `{"type":"avoid","scope":"meal","category":"soup_ingredient_repeat","n":1,"points":18,"strength":"prefer"}`,
			Priority:    300,
			Message:     "汤主料与昨天重复降分",
		},
```

(b) `menu_rule_service.go` line 71:

```go
	menuRulesSeedVersion    = 4
```

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./internal/services/ -v -run 'TestDefaultMenuRuleTemplatesMatchExpressions|TestEnsureDefaultMenuRules|TestMenuRuleTemplate'`
Expected: PASS — drift test proves both new expressions byte-match their rendered templates and compile (compile exercises `len(filter(week,...))` and `countOverlapPrevSoup` against `sampleRuleEnv`).

- [ ] **Step 5: Full backend suite**

Run: `cd backend && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/models/menu_rules.go backend/internal/services/menu_rule_service.go backend/internal/services/menu_rule_templates_test.go
git commit -m "Seed v4: weekly slow-soup cap and adjacent-day soup ingredient rules"
```

---

### Task 6: thread adjacent-day soups through the generator

**Files:**
- Modify: `backend/internal/services/plan_service.go`
- Test: `backend/internal/services/plan_service_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `plan_service_test.go`:

```go
func TestAdjacentPlannedSoups(t *testing.T) {
	soup := func(name string) models.Dish {
		return models.Dish{Name: name, Category: "汤品", DishRole: "soup"}
	}
	meat := models.Dish{Name: "红烧排骨", Category: "家常菜", DishRole: "meat"}
	days := []WeekDayPlan{
		{Lunch: []models.Dish{soup("紫菜蛋花汤")}, Dinner: []models.Dish{meat}},
		{Lunch: []models.Dish{meat}, Dinner: []models.Dish{soup("萝卜丝鲫鱼汤")}},
		{Lunch: []models.Dish{soup("冬瓜排骨汤")}, Dinner: []models.Dish{}},
	}

	got := adjacentPlannedSoups(days, 1)
	if len(got) != 2 || got[0].Name != "紫菜蛋花汤" || got[1].Name != "冬瓜排骨汤" {
		t.Fatalf("adjacentPlannedSoups(days, 1) returned %d soups (%+v), want 紫菜蛋花汤+冬瓜排骨汤", len(got), got)
	}
	if got := adjacentPlannedSoups(days, 0); len(got) != 1 || got[0].Name != "萝卜丝鲫鱼汤" {
		t.Fatalf("adjacentPlannedSoups(days, 0) returned %d soups, want only 萝卜丝鲫鱼汤", len(got))
	}
	if got := adjacentPlannedSoups(days, 2); len(got) != 1 || got[0].Name != "萝卜丝鲫鱼汤" {
		t.Fatalf("adjacentPlannedSoups(days, 2) returned %d soups, want only 萝卜丝鲫鱼汤", len(got))
	}
}

// TestSoupSchedulingRulesViaScoreEvaluation runs the two seeded v4 rules
// through the real evaluateScoreRules path: compiled expression + rule env.
func TestSoupSchedulingRulesViaScoreEvaluation(t *testing.T) {
	var slowSoupRule, repeatRule models.MenuRule
	for _, rule := range models.DefaultMenuRules() {
		switch rule.Code {
		case "weekly_slow_soup_limit":
			slowSoupRule = rule
		case "soup_ingredient_repeat_penalty":
			repeatRule = rule
		}
	}
	compiledSlow, err := compileMenuRule(slowSoupRule)
	if err != nil {
		t.Fatalf("compile weekly_slow_soup_limit: %v", err)
	}
	compiledRepeat, err := compileMenuRule(repeatRule)
	if err != nil {
		t.Fatalf("compile soup_ingredient_repeat_penalty: %v", err)
	}

	slowSoup := func(name string) models.Dish {
		return models.Dish{Name: name, Category: "汤品", DishRole: "soup", CookTime: 90,
			TraitSource: "manual", TraitVersion: models.DishTraitVersion}
	}
	candidate := slowSoup("猪肚鸡汤")
	dayCtx := weekPlanDayContext{profile: "balanced"}

	// Third slow soup of the week fights -25; the second is free.
	week := []models.Dish{slowSoup("莲藕排骨汤"), slowSoup("酸萝卜老鸭汤")}
	if got := evaluateScoreRules(candidate, dayCtx, MealQuota{}, nil, nil, week, []compiledMenuRule{compiledSlow}, true); got != -25 {
		t.Fatalf("third slow soup score = %v, want -25", got)
	}
	if got := evaluateScoreRules(candidate, dayCtx, MealQuota{}, nil, nil, week[:1], []compiledMenuRule{compiledSlow}, true); got != 0 {
		t.Fatalf("second slow soup score = %v, want 0", got)
	}

	// Yesterday's soup shared 鲫鱼 → -18; aromatics-only overlap (姜) → 0.
	yesterday := models.Dish{Name: "萝卜丝鲫鱼汤", Category: "汤品", DishRole: "soup",
		Ingredients: `[{"name":"鲫鱼","amount":"1条"},{"name":"萝卜","amount":"半根"},{"name":"姜","amount":"3片"}]`,
		TraitSource: "manual", TraitVersion: models.DishTraitVersion}
	repeatCtx := weekPlanDayContext{profile: "balanced", prevSoups: []models.Dish{yesterday}}
	todayRepeat := models.Dish{Name: "鲫鱼豆腐汤", Category: "汤品", DishRole: "soup",
		Ingredients: `[{"name":"鲫鱼","amount":"1条"},{"name":"豆腐","amount":"1块"},{"name":"姜","amount":"3片"}]`,
		TraitSource: "manual", TraitVersion: models.DishTraitVersion}
	if got := evaluateScoreRules(todayRepeat, repeatCtx, MealQuota{}, nil, nil, nil, []compiledMenuRule{compiledRepeat}, true); got != -18 {
		t.Fatalf("repeated soup main ingredient score = %v, want -18", got)
	}
	todayFresh := models.Dish{Name: "番茄蛋花汤", Category: "汤品", DishRole: "soup",
		Ingredients: `[{"name":"番茄","amount":"1个"},{"name":"鸡蛋","amount":"1个"},{"name":"姜","amount":"2片"}]`,
		TraitSource: "manual", TraitVersion: models.DishTraitVersion}
	if got := evaluateScoreRules(todayFresh, repeatCtx, MealQuota{}, nil, nil, nil, []compiledMenuRule{compiledRepeat}, true); got != 0 {
		t.Fatalf("aromatics-only overlap score = %v, want 0", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./internal/services/ -run 'TestAdjacentPlannedSoups|TestSoupSchedulingRulesViaScoreEvaluation' -v`
Expected: COMPILE FAIL — `undefined: adjacentPlannedSoups`, `unknown field prevSoups in struct literal`.

- [ ] **Step 3: Implement in `plan_service.go`**

(a) `weekPlanDayContext` (line 295) gains the field:

```go
type weekPlanDayContext struct {
	profile           string
	dayWant           []string
	weekWant          []string
	isWeekend         bool
	categoryFavorites map[string]int
	prevSoups         []models.Dish
}
```

(b) Helpers, placed right after `RegenerateWeekPlanDay`:

```go
// soupsFromDishes filters the dishes that occupy soup slots, using the same
// dual test (inferred role OR name/category) slot assignment uses.
func soupsFromDishes(dishes []models.Dish) []models.Dish {
	var result []models.Dish
	for _, dish := range dishes {
		if dish.DishRole == "soup" || isSoupDish(dish) {
			result = append(result, dish)
		}
	}
	return result
}

// adjacentPlannedSoups collects the soups already planned on the days next
// to dayIndex, so a regenerated day's soup avoids clashing with both
// neighbors (sequential generation only ever has the previous day).
func adjacentPlannedSoups(days []WeekDayPlan, dayIndex int) []models.Dish {
	var result []models.Dish
	for _, neighbor := range []int{dayIndex - 1, dayIndex + 1} {
		if neighbor < 0 || neighbor >= len(days) {
			continue
		}
		result = append(result, soupsFromDishes(appendDishSlices(days[neighbor].Lunch, days[neighbor].Dinner))...)
	}
	return result
}
```

(c) `generateDay` (line 176) gains a param and fills the context (after `dayCtx.categoryFavorites = ctx.categoryFavorites`):

```go
func (ctx *weekPlanGenContext) generateDay(dayIndex int, date time.Time, globalUsed map[uint]bool, weekPicked *[]models.Dish, prevSoups []models.Dish) WeekDayPlan {
	...
	dayCtx.categoryFavorites = ctx.categoryFavorites
	dayCtx.prevSoups = prevSoups
```

(d) `GenerateWeekPlan` loop (lines 218-223) threads yesterday's soups:

```go
	var days []WeekDayPlan
	globalUsed := make(map[uint]bool)
	var weekPicked []models.Dish
	var prevSoups []models.Dish
	for i := 0; i < 7; i++ {
		day := ctx.generateDay(i, monday.AddDate(0, 0, i), globalUsed, &weekPicked, prevSoups)
		days = append(days, day)
		prevSoups = soupsFromDishes(appendDishSlices(day.Lunch, day.Dinner))
	}
```

(e) `RegenerateWeekPlanDay` (line 274) passes both neighbors:

```go
	newPlan.Days[dayIndex] = ctx.generateDay(dayIndex, parsedDate, globalUsed, &weekPicked, adjacentPlannedSoups(newPlan.Days, dayIndex))
```

(f) Replace the `nil` placeholder from Task 3 in BOTH `evaluateConstraintRules` (line 519) and `evaluateScoreRules` (line 540):

```go
		env := buildRuleEnv(dishRuleEnv(candidate, dayCtx.categoryFavorites), dishesRuleEnv(mealPicked, dayCtx.categoryFavorites), dishesRuleEnv(appendDishSlices(dayPicked, mealPicked), dayCtx.categoryFavorites), dishesRuleEnv(appendDishSlices(weekPicked, mealPicked), dayCtx.categoryFavorites), dishesRuleEnv(dayCtx.prevSoups, dayCtx.categoryFavorites), dayCtx.profile, quota, dayCtx.isWeekend)
```

(g) Check for any other `generateDay(` callers: `grep -n "generateDay(" backend/internal/services/*.go` — only the two call sites above are expected; update any test caller the grep reveals by passing `nil`.

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./internal/services/ -run 'TestAdjacentPlannedSoups|TestSoupSchedulingRulesViaScoreEvaluation' -v`
Expected: PASS.

- [ ] **Step 5: Full backend suite**

Run: `cd backend && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/services/plan_service.go backend/internal/services/plan_service_test.go
git commit -m "Thread adjacent-day soups into rule evaluation"
```

---

### Task 7: frontend labels, changelog, live verification

**Files:**
- Modify: `frontend/src/lib/menuRuleTemplates.ts:47-51`
- Modify: `frontend/src/components/rules/PresetRuleCard.tsx:55-56`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Display labels**

In `menuRuleTemplates.ts`, extend `DISPLAY_ONLY_CATEGORY_LABELS`:

```ts
const DISPLAY_ONLY_CATEGORY_LABELS: Record<string, string> = {
  non_favorite: "没收藏的菜",
  unfamiliar_category: "一道收藏都没有的菜系",
  weekday_slow_soup: "工作日的费时汤",
  slow_soup: "费时汤（炖煮超45分钟）",
  soup_ingredient_repeat: "和昨天的汤撞主料的汤",
}
```

Resulting sentences (via the existing `describeTemplate`): 「一周里，费时汤（炖煮超45分钟）最多 2 道 · 尽量做到」 and 「尽量避开和昨天的汤撞主料的汤」.

- [ ] **Step 2: Points stepper for prefer-strength limit rules**

In `PresetRuleCard.tsx` (lines 55-56), the new weekly cap is a `limit` rule whose points should be tunable like the avoid rules (this also unlocks the stepper for the five existing prefer-strength limit rules — consistent with the v0.7.1 stepper feature):

```ts
  const hasPoints = tpl.type === "prefer" || tpl.type === "avoid" || (tpl.type === "limit" && tpl.strength !== "must")
  const pointsValue = tpl.points && tpl.points > 0 ? tpl.points : tpl.type === "prefer" ? 20 : 15 // 与后端 defaultTemplatePoints 保持一致（prefer 20 / limit·avoid 15）
```

- [ ] **Step 3: Frontend type-check/build**

Run: `cd frontend && npm run build`
Expected: build succeeds with no TS errors.

- [ ] **Step 4: Changelog**

Add at the top of `CHANGELOG.md` (above the v0.7.1 section):

```markdown
## v0.7.2 (2026-06-11)

### 修复

- **汤的识别大幅收紧** — 之前做法步骤里出现「汤汁」、标签带「汤品」、名字带「煲」都会被当成汤，全库 632 道菜里 198 道被标成汤（红烧排骨、水煮牛肉、酸汤鱼、汤圆都在内）。现在只认两条：分类带「汤」，或菜名以「汤/羹」结尾；重启后全库自动重新标注（手动改过角色的菜不动），汤位只会排真正的汤，红烧排骨这类硬菜回归荤位竞争

### 新增

- **费时汤一周最多两道（规则 seed v4）** — 一周菜单里炖煮超过 45 分钟的汤排满两道后，第三道起 -25 分；与「工作日少排费时汤」叠加，等于工作日压制、周末两天各一道大炖汤封顶。规则页可调次数与分值
- **今天的汤不和昨天撞主料** — 排汤位时与昨天的汤食材重叠（比如连着两天鲫鱼汤）-18 分，葱姜蒜等基础配料不算；单日重新生成时会同时避开前后两天的汤。规则页可调分值
- **「最多出现」类规则的分值也能调了** — 限次类预设规则（辣菜不扎堆、费时菜分散、油炸限两次等）的「降多少分」步进器补齐
```

- [ ] **Step 5: Restart backend and verify the live re-label + migration**

Kill the running backend (per project memory: the `go run` child is named `server`/`exe`, kill by port):

```bash
lsof -ti:8080 | xargs kill -9 2>/dev/null; cd backend && nohup go run cmd/server/main.go > /tmp/ninimenu-backend.log 2>&1 &
```

Wait ~5s for startup (backfill runs at boot), then verify against the live DB:

```bash
sqlite3 backend/data/ninimenu.db "
SELECT 'soup count: ' || count(*) FROM dishes WHERE deleted_at IS NULL AND dish_role='soup';
SELECT 'fake soups remaining: ' || count(*) FROM dishes WHERE deleted_at IS NULL AND dish_role='soup' AND name NOT LIKE '%汤' AND name NOT LIKE '%羹' AND category NOT LIKE '%汤%';
SELECT name || ' → ' || dish_role FROM dishes WHERE deleted_at IS NULL AND name IN ('红烧排骨','水煮牛肉','酸汤鱼','黑芝麻汤圆','莲藕花生排骨汤','开水白菜');
SELECT 'rules: ' || count(*) FROM menu_rules WHERE deleted_at IS NULL;
SELECT 'seed version: ' || value FROM settings WHERE key='menu_rules_seed_version';
"
```

Expected: soup count ≈ 60; fake soups remaining = 0; 红烧排骨/水煮牛肉/酸汤鱼 → meat, 黑芝麻汤圆 → staple, 莲藕花生排骨汤 → soup, 开水白菜 → veg; rules = 15 (14 defaults + 1 pre-existing user custom rule); seed version = 4.

Also confirm the server came up cleanly: `tail -5 /tmp/ninimenu-backend.log` should show the Gin startup banner / listening line and no panic. (Skip curl probes — API routes require an `X-App-Token` header; the log line plus the sqlite checks above are sufficient.)

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/menuRuleTemplates.ts frontend/src/components/rules/PresetRuleCard.tsx CHANGELOG.md
git commit -m "Frontend labels and changelog for soup scheduling controls"
```

---

## Final integration review checklist (for the closing reviewer)

1. Spec §3: `IsSoupName` is the only soup text-test left — `grep -rn '"煲"' backend/internal` must show no soup-detection use (the `inferCookingMethods` simmer keyword list legitimately keeps 煲/汤 — that is cooking METHOD, not role, and is out of scope).
2. Spec §4/§5: drift test green proves template↔expression byte-match for all 14 rules.
3. Double-count guard: the two new rules are week-plan only — `pickPenaltyAdjustment` in `pick_service.go` must NOT mention them (no week/prev-day state in the pick path; documented in spec §4/§5).
4. `weekday_slow_soup_penalty` (-30) now fires only for true soups — its expression is unchanged; the data fix did the work.
5. Live DB spot-check from Task 7 Step 5 matches expectations (≈60 soups, 0 fakes).
