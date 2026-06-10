# Week Plan Customization & Friendly Rules — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> This plan is executed inline in the authoring session (user-approved); backend
> tasks carry full code, frontend tasks carry exact component contracts +
> structure specs. Spec: `docs/superpowers/specs/2026-06-10-week-plan-ux-design.md`.

**Goal:** Per-day taste themes + cravings + combo packs on the week-plan page; blocked ingredients, plain-language preset rules and a sentence builder on the rules page.

**Architecture:** Blocked ingredients filter the candidate pool for every pick path; day overrides/cravings live in `week_plan_preferences` and bias the planner's scoring; rules gain a `template` field rendered to expr expressions server-side (expression stays the execution truth).

**Tech Stack:** Go 1.x + gin + GORM + expr-lang v1.17 (verified: `filter(meal, .cook_time > 45)`, `candidate.name contains "X"`, `any(...)` all compile); React + TS + @tanstack/react-query + Tailwind.

**Verified facts:** pick entry points = `handlers/pick.go:30,45,109` (PickDishes incl. blind box) + `:126` (PickTomorrowDishes); routes in `internal/routes/routes.go` app group (line ~67) + admin group (~99); `MenuRule` model at `internal/models/other.go:80`; frontend types at `frontend/src/types/index.ts:193+`, API at `frontend/src/api/index.ts:88+`.

Run backend commands from `backend/`; frontend from `frontend/`.

---

## Task 1: Blocked-ingredients service

**Files:**
- Create: `backend/internal/services/blocked_ingredients.go`
- Test: `backend/internal/services/blocked_ingredients_test.go`

- [ ] **Step 1: failing test**

```go
package services

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"testing"
)

func TestFilterBlockedDishes(t *testing.T) {
	setupPlanServiceTestDB(t)
	if err := database.DB.Create(&models.Setting{Key: "blocked_ingredients", Value: `["香菜","虾"]`}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	dishes := []models.Dish{
		{Name: "香菜牛肉", Ingredients: `[{"name":"牛肉","amount":"100g"}]`},
		{Name: "白灼基围虾", Ingredients: `[{"name":"基围虾","amount":"200g"}]`},
		{Name: "清炒西兰花", Ingredients: `[{"name":"西兰花","amount":"1棵"}]`},
		{Name: "凉拌黄瓜", Ingredients: `[{"name":"黄瓜","amount":"1根"}]`, Seasonings: `[{"name":"香菜","amount":"少许"}]`},
	}
	got := FilterBlockedDishes(dishes)
	if len(got) != 1 || got[0].Name != "清炒西兰花" {
		t.Fatalf("FilterBlockedDishes() = %+v, want only 清炒西兰花", got)
	}
}

func TestFilterBlockedDishesNoSetting(t *testing.T) {
	setupPlanServiceTestDB(t)
	dishes := []models.Dish{{Name: "香菜牛肉"}}
	if got := FilterBlockedDishes(dishes); len(got) != 1 {
		t.Fatalf("no blocked setting should pass dishes through, got %+v", got)
	}
}
```

- [ ] **Step 2: run** `go test ./internal/services/ -run TestFilterBlockedDishes -v` → FAIL (undefined: FilterBlockedDishes)
- [ ] **Step 3: implement**

```go
package services

import (
	"encoding/json"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"strings"
)

const blockedIngredientsSettingKey = "blocked_ingredients"

func GetBlockedIngredients() []string {
	var setting models.Setting
	if err := database.DB.Where("`key` = ?", blockedIngredientsSettingKey).First(&setting).Error; err != nil {
		return nil
	}
	var words []string
	if json.Unmarshal([]byte(setting.Value), &words) != nil {
		return nil
	}
	result := make([]string, 0, len(words))
	for _, word := range words {
		word = strings.TrimSpace(word)
		if word != "" {
			result = append(result, word)
		}
	}
	return result
}

// FilterBlockedDishes removes dishes containing any blocked ingredient word
// in their name, ingredients or seasonings. Shared by every recommendation
// entry point so 忌口 applies app-wide.
func FilterBlockedDishes(dishes []models.Dish) []models.Dish {
	blocked := GetBlockedIngredients()
	if len(blocked) == 0 {
		return dishes
	}
	result := make([]models.Dish, 0, len(dishes))
	for _, dish := range dishes {
		if !dishContainsBlockedWord(dish, blocked) {
			result = append(result, dish)
		}
	}
	return result
}

func dishContainsBlockedWord(dish models.Dish, blocked []string) bool {
	parts := []string{dish.Name}
	parts = append(parts, dishIngredientNames(dish.Ingredients)...)
	parts = append(parts, dishIngredientNames(dish.Seasonings)...)
	text := strings.Join(parts, " ")
	for _, word := range blocked {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func dishIngredientNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var named []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(raw), &named); err == nil {
		result := make([]string, 0, len(named))
		for _, item := range named {
			if item.Name != "" {
				result = append(result, item.Name)
			}
		}
		return result
	}
	var plain []string
	if err := json.Unmarshal([]byte(raw), &plain); err == nil {
		return plain
	}
	return nil
}
```

- [ ] **Step 4: run** same test → PASS
- [ ] **Step 5: commit** `git add backend/internal/services/blocked_ingredients*.go && git commit -m "Add blocked-ingredients filter service"`

## Task 2: Wire the filter into all pick paths

**Files:**
- Modify: `backend/internal/services/plan_service.go` (GenerateWeekPlan, after the enabled query)
- Modify: `backend/internal/services/pick_service.go` (PickDishes ×2 query sites, PickTomorrowDishes)
- Test: `backend/internal/services/blocked_ingredients_test.go`

- [ ] **Step 1: failing tests** — `TestGenerateWeekPlanExcludesBlockedIngredients` (create 香菜豆腐 + 清炒青菜 dishes, blocked=香菜, prefs 1 veg lunch/dinner; assert no plan dish name contains 香菜) and `TestPickTomorrowDishesExcludesBlocked` (two dishes, one blocked; `PickTomorrowDishes(TomorrowPickOptions{Count: 5})` returns only the clean one).
- [ ] **Step 2: run** → FAIL (blocked dish appears)
- [ ] **Step 3: implement** — insert `dishes = FilterBlockedDishes(dishes)` after each `Find(&dishes)`/fallback re-query in the three functions (4 insertion points: plan_service GenerateWeekPlan; pick_service PickDishes main + fallback, PickTomorrowDishes).
- [ ] **Step 4: run** `go test ./internal/services/` → PASS
- [ ] **Step 5: commit** `"Apply blocked ingredients to all recommendation paths"`

## Task 3: Preferences data model — Days/WeekWant

**Files:**
- Modify: `backend/internal/services/week_plan_preferences.go`
- Test: `backend/internal/services/week_plan_preferences_test.go` (create)

- [ ] **Step 1: failing tests**

```go
func TestWeekPlanPreferencesDaysNormalization(t *testing.T) {
	prefs := normalizeWeekPlanPreferences(WeekPlanPreferences{
		WeekWant: []string{"poultry", "bogus", "poultry"},
		Days: map[string]DayOverride{
			"mon":  {Profile: "spicy", Want: []string{"beef", "junk"}},
			"xyz":  {Profile: "light"},
			"tue":  {Profile: "weird"},
			"wed":  {},
		},
	})
	if len(prefs.WeekWant) != 1 || prefs.WeekWant[0] != "poultry" { t.Fatalf("week want = %v", prefs.WeekWant) }
	if _, ok := prefs.Days["xyz"]; ok { t.Fatalf("invalid day key kept") }
	if prefs.Days["mon"].Profile != "spicy" || len(prefs.Days["mon"].Want) != 1 { t.Fatalf("mon = %+v", prefs.Days["mon"]) }
	if prefs.Days["tue"].Profile != "" { t.Fatalf("invalid profile should clear, got %q", prefs.Days["tue"].Profile) }
	if _, ok := prefs.Days["wed"]; ok { t.Fatalf("empty override should be dropped") }
}

func TestWeekPlanPreferencesLegacyJSONLoads(t *testing.T) {
	setupPlanServiceTestDB(t)
	legacy := `{"weekday":{"profile":"balanced","lunch":{"meat_count":1,"veg_count":1,"soup_count":0},"dinner":{"meat_count":1,"veg_count":0,"soup_count":1}},"weekend":{"profile":"favorite","lunch":{"meat_count":0,"veg_count":0,"soup_count":0},"dinner":{"meat_count":2,"veg_count":1,"soup_count":1}}}`
	if err := database.DB.Create(&models.Setting{Key: "week_plan_preferences", Value: legacy}).Error; err != nil { t.Fatal(err) }
	prefs := GetWeekPlanPreferences()
	if prefs.Weekend.Profile != "favorite" { t.Fatalf("legacy profile lost") }
	if prefs.Days == nil { /* nil map fine */ }
	if len(prefs.WeekWant) != 0 { t.Fatalf("unexpected week want") }
}
```

- [ ] **Step 2: run** → FAIL (no DayOverride type)
- [ ] **Step 3: implement** — add to `week_plan_preferences.go`:

```go
type DayOverride struct {
	Profile string   `json:"profile,omitempty"`
	Want    []string `json:"want,omitempty"`
}
// WeekPlanPreferences gains:
//   WeekWant []string               `json:"week_want,omitempty"`
//   Days     map[string]DayOverride `json:"days,omitempty"`

var weekPlanDayKeys = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

var weekPlanProteinKeys = map[string]bool{
	"pork": true, "beef": true, "lamb": true, "poultry": true,
	"seafood": true, "egg": true, "soy": true,
}

func normalizeOptionalPlanProfile(profile string) string {
	profile = strings.TrimSpace(profile)
	if profile == "" { return "" }
	if normalizePlanProfile(profile) == profile { return profile }
	return ""
}

func normalizeWantList(want []string) []string {
	result := make([]string, 0, len(want))
	seen := map[string]bool{}
	for _, key := range want {
		key = strings.TrimSpace(key)
		if weekPlanProteinKeys[key] && !seen[key] { seen[key] = true; result = append(result, key) }
	}
	return result
}
```

Extend `normalizeWeekPlanPreferences` to normalize `WeekWant` and rebuild `Days` keeping only valid day keys; drop entries where both normalized Profile=="" and Want empty. Note `normalizePlanProfile` returns "balanced" for unknown — optional variant must clear instead.

- [ ] **Step 4: run** `go test ./internal/services/ -run TestWeekPlanPreferences -v` → PASS; full suite PASS
- [ ] **Step 5: commit** `"Add per-day overrides and week cravings to plan preferences"`

## Task 4: Soup profile (靓汤)

**Files:**
- Modify: `backend/internal/services/week_plan_preferences.go` (`normalizePlanProfile` accepts "soup")
- Modify: `backend/internal/services/pick_service.go` (`matchesTomorrowProfile`, `tomorrowDishScore` gain soup branch)
- Test: `backend/internal/services/pick_service_test.go` (create or extend)

- [ ] **Step 1: failing test** — `TestTomorrowProfileSoup`: soup dish (紫菜蛋花汤, tags 汤品) matches profile "soup" and outscores a stir-fry dish under profile "soup"; non-soup doesn't match.
- [ ] **Step 2: run** → FAIL ("soup" normalizes to balanced)
- [ ] **Step 3: implement** — add `"soup"` to `normalizePlanProfile` allow-list; `matchesTomorrowProfile` case `"soup"`: `return isSoupDish(d) || d.DishRole == "soup"`; `tomorrowDishScore` case `"soup"`: `if isSoupDish(d) || d.DishRole == "soup" { score += 18 }; if containsString(parseJSONStrings(d.CookingMethods), "simmer") { score += 8 }`.
- [ ] **Step 4: run** → PASS
- [ ] **Step 5: commit** `"Add soup taste profile"`

## Task 5: Planner consumes day overrides, cravings, temp soup slot

**Files:**
- Modify: `backend/internal/services/plan_service.go`
- Test: `backend/internal/services/plan_service_test.go`

Behavior spec:
- New `weekPlanDayContext{profile string; dayWant, weekWant []string}` replaces the bare `profile string` parameter through `pickQuotaDishes` → `pickBestDishForSlot` → `scoreWeekPlanCandidates` → `baseWeekPlanDishScore` and the two rule-eval calls (which still receive `dayCtx.profile`).
- In `GenerateWeekPlan`'s day loop (`i` 0..6): `key := weekPlanDayKeys[i]`; start from period prefs; apply `prefs.Days[key]` (non-empty Profile overrides; Want becomes dayWant); `weekWant` = prefs.WeekWant.
- Temp soup slot: after resolving quotas, `if dayCtx.profile == "soup" && lunch.SoupCount+dinner.SoupCount == 0` → add 1 soup to dinner if dinner.total()>0 else to lunch if lunch.total()>0.
- Scoring: in `baseWeekPlanDishScore` add `score += wantBonus(dish, dayCtx.dayWant, 25) + wantBonus(dish, dayCtx.weekWant, 12)` where wantBonus checks `parseJSONStrings(dish.ProteinSources)` against the want keys (first match wins, no stacking).

- [ ] **Step 1: failing tests**
  - `TestGenerateWeekPlanAppliesDayProfileOverride`: prefs balanced everywhere, `Days: {"wed": {Profile: "spicy"}}`; pool = 20 favorite mild meat dishes (favorite=true) + 20 non-favorite spicy meat dishes (taste 辣); quotas 1 meat lunch only. Wednesday (index 2) must pick a spicy dish (profile +40 beats favorite +12); Monday must pick favorite mild.
  - `TestGenerateWeekPlanAppliesWantBonus`: all-balanced, `Days: {"mon": {Want: ["poultry"]}}`; pool favorite pork dishes + plain chicken dishes; Monday picks chicken (+25 > +12), Tuesday picks pork favorite.
  - `TestGenerateWeekPlanSoupDayAddsTempSoupSlot`: quotas with zero soup everywhere, `Days: {"sat": {Profile: "soup"}}`; pool has soups + others; Saturday dinner contains exactly 1 soup, other days 0.
- [ ] **Step 2: run** → FAIL
- [ ] **Step 3: implement** per spec above (mechanical signature threading; keep existing behavior when Days empty)
- [ ] **Step 4: run** full `go test ./internal/services/` → PASS
- [ ] **Step 5: commit** `"Planner honors day overrides, cravings and soup days"`

## Task 6: Single-day regeneration

**Files:**
- Modify: `backend/internal/services/plan_service.go` (extract gen context + day generator, add `RegenerateWeekPlanDay`)
- Modify: `backend/internal/handlers/plan.go` (handler), `backend/internal/routes/routes.go` (route in app group)
- Test: `backend/internal/services/plan_service_test.go`

Refactor spec (no behavior change for full generation):
```go
type weekPlanGenContext struct {
	lunchPool, dinnerPool []models.Dish
	prefs                 WeekPlanPreferences
	rules                 []compiledMenuRule
	recent                map[uint]bool
	warnings              []string
	r                     *rand.Rand
}
func buildWeekPlanGenContext() *weekPlanGenContext // nil when no enabled dishes
func (ctx *weekPlanGenContext) generateDay(dayIndex int, date time.Time, globalUsed map[uint]bool, weekPicked *[]models.Dish) WeekDayPlan
```
`GenerateWeekPlan` = build context + loop calling generateDay. `RegenerateWeekPlanDay(date string)`:
copy the cached plan (fresh Days slice — never mutate the cached pointer), locate the day by date (error 日期不在本周菜单内), seed globalUsed/weekPicked from the OTHER six days, regenerate that day with its weekday index (weekend = index>=5), merge warnings, `SaveWeekPlan`, return plan.

Handler: `RegenerateWeekPlanDayHandler` binds `{date string required}`, 400 on bad input/unknown date, success returns plan; achievements event like full regenerate. Route: `app.POST("/week-plan/regenerate-day", ...)`.

- [ ] **Step 1: failing test** — `TestRegenerateWeekPlanDayOnlyChangesTargetDay`: seed 30 dishes; generate + save full plan; capture day dish IDs; call `RegenerateWeekPlanDay(plan.Days[2].Date)`; assert days 0,1,3..6 identical IDs, day 2 valid (meets quota, no overlap with other days' IDs), cache/save updated (recommendations for that date replaced).
- [ ] **Step 2: run** → FAIL (undefined)
- [ ] **Step 3: implement** refactor + function + handler + route
- [ ] **Step 4: run** full suite + `go build ./...` → PASS
- [ ] **Step 5: commit** `"Support regenerating a single week-plan day"`

## Task 7: Rule templates — model field + renderer

**Files:**
- Modify: `backend/internal/models/other.go` (MenuRule + `Template string \`json:"template"\``)
- Create: `backend/internal/services/menu_rule_templates.go`
- Modify: `backend/internal/services/menu_rule_service.go` (`normalizeAndValidateMenuRule` applies template first; `dishRuleEnv` gains `Ingredients []string` via `dishIngredientNames`; `ruleDishEnv` struct + `sampleRuleEnv` updated)
- Test: `backend/internal/services/menu_rule_templates_test.go`

Template schema and rendering:

```go
type MenuRuleTemplate struct {
	Type     string `json:"type"`     // limit | no_repeat | prefer | avoid
	Scope    string `json:"scope"`    // meal | day | week (limit/no_repeat)
	Category string `json:"category"` // see below (limit/prefer/avoid)
	N        int    `json:"n"`        // limit threshold (default 1) / no_repeat overlap threshold
	Points   int    `json:"points"`   // score magnitude (defaults: limit 15, no_repeat 14, prefer 20, avoid 15)
	Strength string `json:"strength"` // must | prefer (default prefer)
}
```

Categories → candidate predicate / scope count expression (`coll` ∈ meal|day|week):
| category | predicate | count |
|---|---|---|
| egg | `has(candidate.protein_sources, "egg")` | `countMeal("protein_sources", "egg")` (countDay/countWeek per scope) |
| cold | `candidate.serving_temperature == "cold"` | `count<Scope>("serving_temperature", "cold")` |
| staple | `candidate.dish_role == "staple"` | `count<Scope>("dish_role", "staple")` |
| soup | `candidate.dish_role == "soup"` | `count<Scope>("dish_role", "soup")` |
| spicy | `candidate.spice_level >= 1` | `len(filter(<coll>, .spice_level >= 1))` |
| heavy_spicy | `candidate.spice_level >= 2 && candidate.richness_level >= 2` | `count<Scope>("heavy_spicy", "true")` |
| deep_fry | `has(candidate.cooking_methods, "deep_fry")` | `count<Scope>("cooking_methods", "deep_fry")` |
| slow | `candidate.cook_time > 45` | `len(filter(<coll>, .cook_time > 45))` |
| protein:K | `has(candidate.protein_sources, "K")` | `len(filter(<coll>, has(.protein_sources, "K")))` — NOTE: `has()` is env-level, not usable inside filter; use `countOverlap<Scope>("protein_sources", candidate.protein_sources)`-style only for no_repeat; for protein limit use `count<Scope>("protein_sources", "K")` |
| ingredient:W | `(candidate.name contains "W" \|\| any(candidate.ingredients, # contains "W"))` | name-only: `len(filter(<coll>, .name contains "W"))` (v1 limitation, documented) |

Render rules:
- `limit` + `must` → constraint, hard, !relaxable, expr `!(<pred>) || <count> < <n>`
- `limit` + `prefer` → score, soft, relaxable, expr `<pred> && <count> >= <n> ? -<points> : 0`
- `no_repeat` (+ scope, n, points) → score `countOverlap<Scope>("protein_sources", candidate.protein_sources) >= <n> && !hasOnly(candidate.protein_sources, "soy") ? -<points> : 0`; `must` variant → constraint `countOverlap<Scope>(...) < <n> || hasOnly(candidate.protein_sources, "soy")`
- `prefer` → score `<pred> ? <points> : 0` (scope candidate)
- `avoid` → score `<pred> ? -<points> : 0`

`ApplyMenuRuleTemplate(rule *models.MenuRule) error`: no-op when Template blank; else parse → render → overwrite Expression/RuleKind/Severity/Relaxable/Scope. Called at the top of `normalizeAndValidateMenuRule` so existing compile validation guards every render.

- [ ] **Step 1: failing tests** — table test: each (type, strength) combo renders, compiles via `compileMenuRule`, and behaves (run with crafted envs: e.g. limit/prefer spicy n=1 returns −12 when meal already has a spicy dish, 0 otherwise; limit/must egg blocks via constraint false). Plus `TestDishRuleEnvIncludesIngredients`.
- [ ] **Step 2: run** → FAIL
- [ ] **Step 3: implement** renderer + model field + env ingredients
- [ ] **Step 4: run** suite → PASS
- [ ] **Step 5: commit** `"Add sentence-template rendering for menu rules"`

## Task 8: countOverlapDay / countOverlapWeek env helpers

**Files:**
- Modify: `backend/internal/services/menu_rule_service.go` (buildRuleEnv)
- Test: `backend/internal/services/menu_rule_templates_test.go`

- [ ] **Step 1: failing test** — expression `countOverlapWeek("protein_sources", candidate.protein_sources) >= 2 ? -20 : 0` compiles and returns −20 with two pork dishes in week env, 0 with none.
- [ ] **Step 2: run** → FAIL (unknown name)
- [ ] **Step 3: implement** — mirror `countOverlapMeal` for day/week collections.
- [ ] **Step 4: run** → PASS
- [ ] **Step 5: commit** `"Add day/week protein-overlap rule helpers"`

## Task 9: New default rule set + seed migration

**Files:**
- Modify: `backend/internal/models/menu_rules.go` (new 9-rule set with Template values)
- Modify: `backend/internal/services/menu_rule_service.go` (seed-version migration replaces `shouldRefreshDefaultMenuRule` logic)
- Test: `backend/internal/services/plan_service_test.go` / `menu_rule_templates_test.go`

New defaults (codes): keep `avoid_double_egg`, `limit_cold_dishes`, `limit_staples` (limit/must, n=1), `avoid_same_primary_protein` (no_repeat meal n=1 pts14), `avoid_heavy_spicy_day` (limit/prefer day heavy_spicy n=1 pts18); add `spicy_meal_spread` (limit/prefer meal spicy n=1 pts12), `slow_meal_spread` (limit/prefer meal slow n=1 pts15), `weekly_protein_variety` (no_repeat week n=4 pts20), `weekly_fried_limit` (limit/prefer week deep_fry n=2 pts20). All Expression values produced by the Task-7 renderer (hardcode the rendered strings in DefaultMenuRules; add a test asserting render(template)==stored expression for each).

Migration via `Setting` key `menu_rules_seed_version` (target 2) inside `EnsureDefaultMenuRules`:
- version < 2 → (a) delete the 5 dropped codes (`quick_profile_bonus`, `light_profile_bonus`, `spicy_profile_bonus`, `favorite_profile_bonus`, `hot_cold_balance_bonus`) **only when** their Expression still equals the factory string (keep a `droppedFactoryExpressions` map); (b) for each new-set code: missing → create; present with Expression equal to any known factory variant (old v1 strings incl. the old hard same-protein rule) → replace with new definition (ID/CreatedAt preserved); otherwise leave user-modified rows untouched; (c) set version 2.
- version ≥ 2 → only create rules for a completely empty table (fresh DB path), preserving user deletions otherwise.

- [ ] **Step 1: failing tests** — `TestEnsureDefaultMenuRulesMigratesV1Defaults`: seed full old 10-rule set verbatim → Ensure → expect exactly the 9 new codes (+ none of dropped), templates attached, version=2, second Ensure idempotent. `TestEnsureDefaultMenuRulesKeepsUserModifiedRules`: seed `quick_profile_bonus` with custom expression → Ensure → rule survives.
- [ ] **Step 2: run** → FAIL
- [ ] **Step 3: implement**
- [ ] **Step 4: run** full backend suite + `go vet ./...` → PASS
- [ ] **Step 5: commit** `"Rework default menu rules and add seed migration"`

## Task 10: Frontend types & API

**Files:**
- Modify: `frontend/src/types/index.ts` — `PlanProfile` += `"soup"`; `interface DayOverride {profile?: PlanProfile | ""; want?: string[]}`; `WeekPlanPreferences` += `week_want?: string[]; days?: Record<string, DayOverride>`; `MenuRule` += `template?: string`.
- Modify: `frontend/src/api/index.ts` — `weekPlanApi.regenerateDay: (date: string) => api<WeekPlan>("POST", "/week-plan/regenerate-day", { date })`; locate the existing admin settings update function (used by `pages/admin/Settings.tsx` `updateMut`) and reuse it for `blocked_ingredients` (JSON-stringified array value); add a constants module export `PROTEIN_OPTIONS: {key,label}[]` (pork 猪肉 / beef 牛肉 / lamb 羊肉 / poultry 鸡鸭 / seafood 鱼虾 / egg 蛋类 / soy 豆制品) in `frontend/src/lib/weekPlanCombos.ts` (Task 11 file).

- [ ] Steps: implement → `npm run build` (type check) → PASS → commit `"Extend frontend types and APIs for week plan customization"` (may be folded into Task 11's commit if trivially small).

## Task 11: Week plan page — theme strip, day sheet, combos

**Files:**
- Create: `frontend/src/lib/weekPlanCombos.ts` — `PROTEIN_OPTIONS`; `PROFILE_EMOJI: Record<PlanProfile,string>` (均衡 ✨ / 快手 ⚡ / 清淡 🍃 / 想吃辣 🌶 / 收藏 ❤️ / 靓汤 🍲); `WEEKDAY_PACKS` & `WEEKEND_PACKS`: `{key,label,desc,days: Record<string,DayOverride>}` per spec §7.1 (busy_worker, homely_rhythm, light_fit, craving / family_kids, treat_self, soup_week, easy_rest); `applyPacks(prefs, weekdayPack, weekendPack): WeekPlanPreferences` (merges into `days`, weekday packs fill mon–fri keys, weekend packs sat–sun); `MEAL_COUNT_PRESETS` (二人简餐/三口之家/人多聚餐 quota sets per spec).
- Create: `frontend/src/components/weekplan/DayThemeStrip.tsx` — props `{days: WeekPlanType["days"]; overrides: Record<string, DayOverride>; defaults: {weekday: PlanProfile; weekend: PlanProfile}; onPick(dayKey: string): void; onOpenCombos(): void}`; renders 组合 button + 7 cells (一..日, emoji of effective profile, dot when following default; today highlighted).
- Create: `frontend/src/components/weekplan/DayThemeSheet.tsx` — bottom sheet (reuse DishPickerModal's overlay pattern); props `{dayKey, dayLabel, value: DayOverride, busy, onChange(next: DayOverride), onRegenerateDay(): void, onClose(): void}`; sections: 6 profile chips (跟随默认 option clears profile), 今天想吃 chips from PROTEIN_OPTIONS (multi-toggle), footer buttons 仅这天重生成 / 完成.
- Create: `frontend/src/components/weekplan/ComboSheet.tsx` — props `{onApply(weekdayKey: string|null, weekendKey: string|null): void; onClose(): void}`; two labelled rows of pack cards, preview line of resulting 7-day emoji spread, confirm button (disabled until ≥1 pack chosen).
- Modify: `frontend/src/pages/WeekPlan.tsx` — add `soup` to `profileOptions` (icon `Soup` from lucide); state `themeSheet: string|null`, `comboOpen: boolean`; `draftPrefs` normalization extended for days/week_want; render `<DayThemeStrip>` between the stats card and 菜单草稿 header; day-sheet edits update `draftPrefs.days[key]` and set `dirtyPrefs`; 仅这天重生成 = save prefs if dirty then `weekPlanApi.regenerateDay(date)` mutation updating draft/cache; combo apply = `applyPacks` + toast 已套用组合，点击应用设置生效; preferences drawer gains 本周想多吃 chip row (week_want) and three MEAL_COUNT_PRESETS buttons above the period cards.

- [ ] **Step 1: implement files** (no component unit-test infra in repo; verification is lint+build+manual)
- [ ] **Step 2: run** `npm run lint && npm run build` → PASS
- [ ] **Step 3: commit** `"Add day themes, cravings and combo packs to week plan page"`

## Task 12: Rules page — three friendly sections

**Files:**
- Create: `frontend/src/lib/menuRuleTemplates.ts` — `parseTemplate(rule: MenuRule): MenuRuleTemplate|null`; `withTemplate(rule, tpl): MenuRule` (serializes template, leaves expression untouched — backend re-renders); `describeTemplate(tpl): string` (中文造句：在同一餐里，蛋类菜最多 1 道 · 必须遵守); option lists `SCOPE_OPTIONS/CATEGORY_OPTIONS/STRENGTH_OPTIONS` (labels per mockup).
- Create: `frontend/src/components/rules/BlockedIngredientsCard.tsx` — props `{value: string[]; canEdit: boolean; busy: boolean; onSave(next: string[]): void}`; chip list with ×, input + 添加 (splits on 空格/，/、), save-on-change with toast 已更新忌口，重新生成菜单后生效.
- Create: `frontend/src/components/rules/PresetRuleCard.tsx` — props `{rule: MenuRule; tpl: MenuRuleTemplate; canEdit: boolean; onChange(patch: Partial<MenuRule>): void}`; renders describeTemplate sentence with inline `CountStepper`-style control bound to `tpl.n` (re-serialized via withTemplate), enable toggle, description line.
- Create: `frontend/src/components/rules/RuleSentenceBuilder.tsx` — props `{onCreate(rule: MenuRule): void; onClose(): void}`; four selects + number input mirroring the template fields; builds a MenuRule (code `custom_rule_${Date.now()}`, name from sentence, template JSON, expression "" — backend renders), 校验 via existing validate API before onCreate.
- Modify: `frontend/src/pages/admin/Settings.tsx` — rules section becomes: BlockedIngredientsCard (value from settings `blocked_ingredients`, saved via existing settings mutation) → 搭配习惯 list (draftRules with parseable template) using PresetRuleCard → 我的规则 (templated custom rules + 新建规则 button opening RuleSentenceBuilder) → 专家模式 collapsible hosting the existing `MenuRuleEditor` unchanged (all rules). Save/validate flows reuse existing mutations.

- [ ] **Step 1: implement files**
- [ ] **Step 2: run** `npm run lint && npm run build` → PASS
- [ ] **Step 3: commit** `"Rebuild rules settings with blocked ingredients, preset cards and sentence builder"`

## Task 13: Regression & docs

- [ ] `cd backend && go build ./... && go vet ./... && go test ./...` → all PASS
- [ ] `cd frontend && npm run lint && npm run build` → PASS
- [ ] Append CHANGELOG.md entry (per-day themes, cravings, combo packs, blocked ingredients, friendly rules, single-day regenerate)
- [ ] Final commit `"Document week plan customization release"`

## Self-review notes

- Spec coverage: §3.1→T3, §3.2→T1/2/12, §3.3→T7, §4→T9 (+T8 helpers), §5→T2/5, §6→T6, §7.1→T10/11, §7.2→T12, §8 tests distributed per task, §9 milestones≈task order. `protein:K` filter-with-`has` pitfall documented in the category table (env funcs unavailable inside closures — use count-based forms).
- No "TBD" remains; ingredient-category count is an explicit documented v1 limitation (name-only), not a placeholder.
- Type consistency: `weekPlanDayContext` (T5) is the same struct consumed in T6's `generateDay`; `DayOverride` JSON shape identical Go (T3) ↔ TS (T10); template JSON field names match Go struct tags (T7) ↔ TS parser (T12).
