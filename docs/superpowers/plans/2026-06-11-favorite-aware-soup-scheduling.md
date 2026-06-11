# Favorite-Aware Penalties & Soup Time-Awareness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Down-weight dishes the household can't cook (favorite = can-cook signal, two-layer penalty), stop rich soups from matching the 清淡 profile, and keep slow soups off weekday menus — all shipped as seed-v3 menu rules plus mirrored tomorrow-pick scoring.

**Architecture:** Three new default score rules (rendered from sentence templates, seed v2→v3 migration reuses the existing "sync defaults, preserve user edits" function). The rule env gains `candidate.category_favorites` and `is_weekend`. The tomorrow-pick path doesn't run the rule engine, so it gets an additive `pickPenaltyAdjustment` mirroring the three rules — penalties deliberately do NOT go into `tomorrowDishScore`, because the week plan reuses that function and would double-count.

**Tech Stack:** Go (Gin, GORM, expr-lang), SQLite, React + TypeScript (Vite). Spec: `docs/superpowers/specs/2026-06-11-favorite-aware-soup-scheduling-design.md`.

**Conventions (from AGENTS.md):** work on `main`, check `git status -sb` before committing, never commit local DB/log artifacts. Backend tests: `cd backend && go test ./...`. Frontend check: `cd frontend && npm run build`.

---

### Task 1: Rule env extension — `category_favorites` + `is_weekend`

The rule engine env and the week-plan day context learn about favorites-per-category and weekday/weekend. All signature changes land together so the package compiles at every commit.

**Files:**
- Modify: `backend/internal/services/menu_rule_service.go` (ruleDishEnv struct ~line 22, buildRuleEnv ~line 355, dishRuleEnv ~line 393, dishesRuleEnv ~line 416, sampleRuleEnv ~line 340)
- Modify: `backend/internal/services/plan_service.go` (weekPlanGenContext ~line 111, buildWeekPlanGenContext ~line 123, generateDay ~line 174, weekPlanDayContext ~line 292, resolveWeekPlanDay ~line 301, scoreWeekPlanCandidates lines 443/448, evaluateConstraintRules ~line 506, evaluateScoreRules ~line 526)
- Modify: `backend/internal/services/pick_service.go` (new helper `favoriteCategoryCounts`, place near `recentDishIDMap`)
- Modify: `backend/internal/services/plan_service_test.go` (existing call site line 434 + new tests)

- [ ] **Step 1: Write the failing tests**

Append to `backend/internal/services/plan_service_test.go`:

```go
func TestRuleEnvCategoryFavoritesAndWeekend(t *testing.T) {
	rule := models.MenuRule{
		Code:       "env_probe",
		Name:       "env probe",
		Enabled:    true,
		Scope:      "candidate",
		RuleKind:   "score",
		Severity:   "soft",
		Relaxable:  true,
		Expression: `(is_weekend ? 100 : 0) + (candidate.category_favorites == 0 ? -25 : 0)`,
	}
	compiled, err := compileMenuRule(rule)
	if err != nil {
		t.Fatalf("compileMenuRule() error = %v", err)
	}
	dish := models.Dish{Name: "测试菜", Category: "新疆菜", Enabled: true}

	weekday := weekPlanDayContext{profile: "balanced", categoryFavorites: map[string]int{"川菜": 2}}
	got := evaluateScoreRules(dish, weekday, MealQuota{MeatCount: 1}, nil, nil, nil, []compiledMenuRule{compiled}, true)
	if got != -25 {
		t.Fatalf("weekday unfamiliar-category score = %v, want -25", got)
	}

	weekend := weekPlanDayContext{profile: "balanced", isWeekend: true, categoryFavorites: map[string]int{"新疆菜": 1}}
	got = evaluateScoreRules(dish, weekend, MealQuota{MeatCount: 1}, nil, nil, nil, []compiledMenuRule{compiled}, true)
	if got != 100 {
		t.Fatalf("weekend familiar-category score = %v, want 100", got)
	}
}

func TestFavoriteCategoryCounts(t *testing.T) {
	setupPlanServiceTestDB(t)
	seed := []models.Dish{
		{Name: "大盘鸡", Category: "新疆菜", Favorite: true, Enabled: false},
		{Name: "红烧肉", Category: "家常菜", Favorite: true, Enabled: true},
		{Name: "清蒸鱼", Category: "家常菜", Enabled: true},
	}
	for i := range seed {
		if err := database.DB.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed dish: %v", err)
		}
	}
	got := favoriteCategoryCounts()
	if got["新疆菜"] != 1 || got["家常菜"] != 1 || got["汤品"] != 0 {
		t.Fatalf("favoriteCategoryCounts() = %v", got)
	}
}
```

Note: `TestFavoriteCategoryCounts` deliberately marks the favorited 大盘鸡 as `Enabled: false` — a disabled favorite still proves the household can cook that cuisine.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/services/ -run 'TestRuleEnvCategoryFavoritesAndWeekend|TestFavoriteCategoryCounts' -v`
Expected: compile errors — `unknown field isWeekend/categoryFavorites`, `undefined: favoriteCategoryCounts`, wrong arg count for `evaluateScoreRules`. A compile failure is this step's "red".

- [ ] **Step 3: Implement the env extension**

In `backend/internal/services/menu_rule_service.go`:

(a) Add the field to `ruleDishEnv` after `Favorite`:

```go
	Favorite           bool     `expr:"favorite"`
	CategoryFavorites  int      `expr:"category_favorites"`
```

(b) Change `dishRuleEnv` / `dishesRuleEnv` to take the counts map:

```go
func dishRuleEnv(dish models.Dish, categoryFavorites map[string]int) ruleDishEnv {
	dish = ensureDishTraits(dish)
	return ruleDishEnv{
		// ... existing fields unchanged ...
		Favorite:          dish.Favorite,
		CategoryFavorites: categoryFavorites[dish.Category],
		// ... existing fields unchanged ...
	}
}

func dishesRuleEnv(dishes []models.Dish, categoryFavorites map[string]int) []ruleDishEnv {
	result := make([]ruleDishEnv, 0, len(dishes))
	for _, dish := range dishes {
		result = append(result, dishRuleEnv(dish, categoryFavorites))
	}
	return result
}
```

(c) Add `isWeekend` to `buildRuleEnv` (last parameter) and bind it:

```go
func buildRuleEnv(candidate ruleDishEnv, meal []ruleDishEnv, day []ruleDishEnv, week []ruleDishEnv, profile string, quota MealQuota, isWeekend bool) map[string]any {
	return map[string]any{
		"candidate":  candidate,
		"meal":       meal,
		"day":        day,
		"week":       week,
		"profile":    normalizePlanProfile(profile),
		"is_weekend": isWeekend,
		// ... rest unchanged ...
```

(d) Update `sampleRuleEnv` (compile-time validation env must know every binding):

```go
	return buildRuleEnv(dish, []ruleDishEnv{}, []ruleDishEnv{}, []ruleDishEnv{}, "balanced", MealQuota{MeatCount: 1}, false)
```

In `backend/internal/services/pick_service.go`, add near `recentDishIDMap`:

```go
// favoriteCategoryCounts maps each category to its favorited-dish count.
// Disabled dishes count too: a favorited dish that is temporarily disabled
// still proves the household can cook that cuisine.
func favoriteCategoryCounts() map[string]int {
	type favoriteCategoryRow struct {
		Category string
		Count    int
	}
	var rows []favoriteCategoryRow
	database.DB.Model(&models.Dish{}).
		Select("category, count(*) as count").
		Where("favorite = ?", true).
		Group("category").
		Scan(&rows)
	result := make(map[string]int, len(rows))
	for _, row := range rows {
		result[row.Category] = row.Count
	}
	return result
}
```

In `backend/internal/services/plan_service.go`:

(e) Extend the day context and gen context:

```go
// weekPlanDayContext carries the effective taste profile and cravings for
// one generated day.
type weekPlanDayContext struct {
	profile           string
	dayWant           []string
	weekWant          []string
	isWeekend         bool
	categoryFavorites map[string]int
}
```

```go
type weekPlanGenContext struct {
	lunchPool         []models.Dish
	dinnerPool        []models.Dish
	prefs             WeekPlanPreferences
	rules             []compiledMenuRule
	recent            map[uint]bool
	categoryFavorites map[string]int
	warnings          []string
	r                 *rand.Rand
}
```

(f) In `buildWeekPlanGenContext`, fill the new field in the returned struct:

```go
	return &weekPlanGenContext{
		lunchPool:         lunchPool,
		dinnerPool:        dinnerPool,
		prefs:             prefs,
		rules:             compiledRules,
		recent:            recentDishIDMap(RecommendationCooldownDays()),
		categoryFavorites: favoriteCategoryCounts(),
		warnings:          warnings,
		r:                 rand.New(rand.NewSource(time.Now().UnixNano())),
	}
```

(g) In `resolveWeekPlanDay`, set the weekend flag (index 5/6 = Sat/Sun, 0=Monday):

```go
	dayCtx := weekPlanDayContext{profile: periodPrefs.Profile, weekWant: prefs.WeekWant, isWeekend: i >= 5}
```

(h) In `generateDay`, copy the counts onto the day context right after `resolveWeekPlanDay`:

```go
	dayCtx, lunchQuota, dinnerQuota := resolveWeekPlanDay(ctx.prefs, periodPrefs, dayIndex)
	dayCtx.categoryFavorites = ctx.categoryFavorites
```

(i) Change `evaluateConstraintRules` / `evaluateScoreRules` to take the day context instead of the bare profile:

```go
func evaluateConstraintRules(candidate models.Dish, dayCtx weekPlanDayContext, quota MealQuota, mealPicked []models.Dish, dayPicked []models.Dish, weekPicked []models.Dish, rules []compiledMenuRule, enforceSoft bool) (bool, string) {
	for _, item := range rules {
		if item.rule.RuleKind != menuRuleKindConstraint {
			continue
		}
		if item.rule.Relaxable && !enforceSoft {
			continue
		}
		env := buildRuleEnv(dishRuleEnv(candidate, dayCtx.categoryFavorites), dishesRuleEnv(mealPicked, dayCtx.categoryFavorites), dishesRuleEnv(appendDishSlices(dayPicked, mealPicked), dayCtx.categoryFavorites), dishesRuleEnv(appendDishSlices(weekPicked, mealPicked), dayCtx.categoryFavorites), dayCtx.profile, quota, dayCtx.isWeekend)
		// ... rest unchanged ...
```

```go
func evaluateScoreRules(candidate models.Dish, dayCtx weekPlanDayContext, quota MealQuota, mealPicked []models.Dish, dayPicked []models.Dish, weekPicked []models.Dish, rules []compiledMenuRule, enforceSoft bool) float64 {
	if !enforceSoft {
		return 0
	}
	total := 0.0
	for _, item := range rules {
		if item.rule.RuleKind != menuRuleKindScore {
			continue
		}
		env := buildRuleEnv(dishRuleEnv(candidate, dayCtx.categoryFavorites), dishesRuleEnv(mealPicked, dayCtx.categoryFavorites), dishesRuleEnv(appendDishSlices(dayPicked, mealPicked), dayCtx.categoryFavorites), dishesRuleEnv(appendDishSlices(weekPicked, mealPicked), dayCtx.categoryFavorites), dayCtx.profile, quota, dayCtx.isWeekend)
		// ... rest unchanged ...
```

(j) Update the two call sites in `scoreWeekPlanCandidates` (lines 443/448): pass `dayCtx` instead of `dayCtx.profile`:

```go
		allowed, _ := evaluateConstraintRules(dish, dayCtx, quota, mealPicked, dayPicked, weekPicked, rules, stage.enforceSoft)
```
```go
		score += evaluateScoreRules(dish, dayCtx, quota, mealPicked, dayPicked, weekPicked, rules, stage.enforceSoft)
```

(k) Update the existing test call site `plan_service_test.go:434`:

```go
			allowed, _ := evaluateConstraintRules(dish, weekPlanDayContext{profile: "balanced"}, quota, nil, nil, nil, []compiledMenuRule{compiled}, tt.enforceSoft)
```

(l) Compile the whole package and fix any call site the grep below still finds (there should be none):

Run: `cd backend && go build ./... && grep -rn "dishRuleEnv(\|dishesRuleEnv(\|buildRuleEnv(" internal/ | grep -v "_test" | grep -v "func "`
Expected: only the updated call sites inside `evaluateConstraintRules`/`evaluateScoreRules` and `sampleRuleEnv`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/services/ -run 'TestRuleEnvCategoryFavoritesAndWeekend|TestFavoriteCategoryCounts' -v`
Expected: PASS ×2

- [ ] **Step 5: Run the full backend suite**

Run: `cd backend && go test ./...`
Expected: all PASS (existing rule tests still compile against the sample env)

- [ ] **Step 6: Commit**

```bash
git add backend/internal/services/menu_rule_service.go backend/internal/services/plan_service.go backend/internal/services/pick_service.go backend/internal/services/plan_service_test.go
git commit -m "Add category-favorite and weekend bindings to rule env"
```

---

### Task 2: Seed v3 — three new default rules, templates, migration

Adds the template categories, the three default rules, and bumps the seed version. The existing `migrateMenuRulesToV2` already implements "insert missing codes, refresh factory-expression rows, keep user edits" — it gets renamed to `syncDefaultMenuRules` and reused.

**Files:**
- Modify: `backend/internal/services/menu_rule_templates.go` (menuRuleCategoryPredicate ~line 179)
- Modify: `backend/internal/models/menu_rules.go` (DefaultMenuRules + file comment)
- Modify: `backend/internal/services/menu_rule_service.go` (menuRulesSeedVersion ~line 70, rename migrateMenuRulesToV2 ~line 129)
- Modify: `backend/internal/services/menu_rule_templates_test.go` (drift test count line 242, v1 migration test lines 313-344)

- [ ] **Step 1: Write the failing tests**

In `backend/internal/services/menu_rule_templates_test.go`:

(a) In `TestDefaultMenuRuleTemplatesMatchExpressions` change the count assertion:

```go
	if len(defaults) != 12 {
		t.Fatalf("DefaultMenuRules() has %d rules, want 12", len(defaults))
	}
```

(b) Add a render unit test for the new categories:

```go
func TestMenuRuleTemplateNewCategoryPredicates(t *testing.T) {
	cases := []struct {
		category string
		points   int
		want     string
	}{
		{"non_favorite", 12, `!candidate.favorite ? -12 : 0`},
		{"unfamiliar_category", 25, `candidate.category != "" && candidate.category_favorites == 0 ? -25 : 0`},
		{"weekday_slow_soup", 30, `!is_weekend && candidate.dish_role == "soup" && (candidate.cook_time > 45 || candidate.difficulty == "hard") ? -30 : 0`},
	}
	for _, tc := range cases {
		tpl := MenuRuleTemplate{Type: "avoid", Category: tc.category, Points: tc.points, Strength: "prefer"}
		rendered, err := tpl.render()
		if err != nil {
			t.Errorf("render(%s) error = %v", tc.category, err)
			continue
		}
		if rendered.Expression != tc.want {
			t.Errorf("render(%s) expression:\ngot:  %s\nwant: %s", tc.category, rendered.Expression, tc.want)
		}
		if rendered.RuleKind != "score" || rendered.Severity != "soft" || !rendered.Relaxable || rendered.Scope != "candidate" {
			t.Errorf("render(%s) meta = %+v", tc.category, rendered)
		}
	}
}
```

(c) Add the v2→v3 migration test:

```go
func TestEnsureDefaultMenuRulesMigratesV2ToV3(t *testing.T) {
	setupPlanServiceTestDB(t)

	// Seed a v2-era database: the 9 v2 factory rules, one user-modified.
	for _, rule := range models.DefaultMenuRules() {
		rule := rule
		switch rule.Code {
		case "non_favorite_penalty", "unfamiliar_category_penalty", "weekday_slow_soup_penalty":
			continue // v3 additions don't exist in a v2 database
		}
		if rule.Code == "weekly_fried_limit" {
			rule.Expression = `has(candidate.cooking_methods, "deep_fry") && countWeek("cooking_methods", "deep_fry") >= 5 ? -20 : 0`
			rule.Template = ""
		}
		if err := database.DB.Create(&rule).Error; err != nil {
			t.Fatalf("seed v2 rule %s: %v", rule.Code, err)
		}
	}
	if err := setSettingValue("menu_rules_seed_version", "2"); err != nil {
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
	if len(rules) != 12 {
		t.Fatalf("after v3 migration %d rules, want 12 (%v)", len(rules), got)
	}
	for _, code := range []string{"non_favorite_penalty", "unfamiliar_category_penalty", "weekday_slow_soup_penalty"} {
		if _, ok := got[code]; !ok {
			t.Errorf("missing v3 rule %s after migration", code)
		}
	}
	wantModified := `has(candidate.cooking_methods, "deep_fry") && countWeek("cooking_methods", "deep_fry") >= 5 ? -20 : 0`
	if got["weekly_fried_limit"].Expression != wantModified {
		t.Errorf("user-modified rule overwritten:\ngot:  %s\nwant: %s", got["weekly_fried_limit"].Expression, wantModified)
	}
	var setting models.Setting
	if err := database.DB.Where("`key` = ?", "menu_rules_seed_version").First(&setting).Error; err != nil {
		t.Fatalf("seed version setting missing: %v", err)
	}
	if setting.Value != "3" {
		t.Errorf("seed version = %q, want 3", setting.Value)
	}
}
```

(d) Update `TestEnsureDefaultMenuRulesMigratesV1Defaults`: extend `wantCodes` and the version assertion:

```go
	wantCodes := []string{
		"avoid_double_egg", "limit_cold_dishes", "limit_staples",
		"avoid_same_primary_protein", "avoid_heavy_spicy_day",
		"spicy_meal_spread", "slow_meal_spread",
		"weekly_protein_variety", "weekly_fried_limit",
		"non_favorite_penalty", "unfamiliar_category_penalty", "weekday_slow_soup_penalty",
	}
```

and

```go
	if setting.Value != "3" {
		t.Errorf("seed version = %q, want 3", setting.Value)
	}
```

(e) Add the end-to-end behavior test (compiled defaults scored through the env) in `plan_service_test.go`:

```go
func TestV3DefaultRulesScoreBehavior(t *testing.T) {
	compiled, warnings := compileMenuRules(models.DefaultMenuRules())
	if len(warnings) != 0 {
		t.Fatalf("default rules compile warnings: %v", warnings)
	}

	slowSoup := models.Dish{Name: "莲藕排骨汤", Category: "汤品", CookTime: 90, Difficulty: "medium", DishRole: "soup", TraitSource: "manual", TraitVersion: models.DishTraitVersion}
	favs := map[string]int{"家常菜": 3}
	weekday := weekPlanDayContext{profile: "balanced", categoryFavorites: favs}
	weekend := weekPlanDayContext{profile: "balanced", isWeekend: true, categoryFavorites: favs}

	wd := evaluateScoreRules(slowSoup, weekday, MealQuota{SoupCount: 1}, nil, nil, nil, compiled, true)
	we := evaluateScoreRules(slowSoup, weekend, MealQuota{SoupCount: 1}, nil, nil, nil, compiled, true)
	if wd-we != -30 {
		t.Fatalf("weekday slow-soup delta = %v, want -30", wd-we)
	}

	favored := slowSoup
	favored.Favorite = true
	favored.Category = "家常菜"
	wdFav := evaluateScoreRules(favored, weekday, MealQuota{SoupCount: 1}, nil, nil, nil, compiled, true)
	if wdFav-wd != 37 {
		t.Fatalf("favorite escape delta = %v, want +37 (-12 and -25 both lifted)", wdFav-wd)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/services/ -run 'TestDefaultMenuRuleTemplatesMatchExpressions|TestMenuRuleTemplateNewCategoryPredicates|TestEnsureDefaultMenuRulesMigratesV2ToV3|TestEnsureDefaultMenuRulesMigratesV1Defaults|TestV3DefaultRulesScoreBehavior' -v`
Expected: FAIL — count is 9 not 12, `规则模板类别无效: non_favorite`, missing v3 codes, seed version "2".

- [ ] **Step 3: Implement templates, rules, migration**

(a) `backend/internal/services/menu_rule_templates.go` — add three cases to `menuRuleCategoryPredicate` (before the `protein:` case). Do NOT add them to `menuRuleCategoryCount`: they are display-only categories used by avoid-type presets, never offered for limit rules.

```go
	case category == "non_favorite":
		return `!candidate.favorite`, nil
	case category == "unfamiliar_category":
		return `candidate.category != "" && candidate.category_favorites == 0`, nil
	case category == "weekday_slow_soup":
		return `!is_weekend && candidate.dish_role == "soup" && (candidate.cook_time > 45 || candidate.difficulty == "hard")`, nil
```

(b) `backend/internal/models/menu_rules.go` — update the file comment (`seed version 2` → `seed version 3`) and append three rules at the end of the slice in `DefaultMenuRules()`:

```go
		{
			Code:        "non_favorite_penalty",
			Name:        "未收藏的菜降分",
			Description: "收藏过的菜代表家里会做；没收藏的菜推荐时降一点分，但不会完全消失。",
			Enabled:     true,
			Scope:       "candidate",
			RuleKind:    "score",
			Severity:    "soft",
			Relaxable:   true,
			Expression:  `!candidate.favorite ? -12 : 0`,
			Template:    `{"type":"avoid","scope":"meal","category":"non_favorite","n":1,"points":12,"strength":"prefer"}`,
			Priority:    260,
			Message:     "未收藏的菜降分",
		},
		{
			Code:        "unfamiliar_category_penalty",
			Name:        "陌生菜系降分",
			Description: "一道收藏都没有的菜系视为基本不会做，整体大幅降分；偶尔出现就当学新菜的机会。",
			Enabled:     true,
			Scope:       "candidate",
			RuleKind:    "score",
			Severity:    "soft",
			Relaxable:   true,
			Expression:  `candidate.category != "" && candidate.category_favorites == 0 ? -25 : 0`,
			Template:    `{"type":"avoid","scope":"meal","category":"unfamiliar_category","n":1,"points":25,"strength":"prefer"}`,
			Priority:    270,
			Message:     "陌生菜系降分",
		},
		{
			Code:        "weekday_slow_soup_penalty",
			Name:        "工作日少排费时汤",
			Description: "周一到周五的汤位优先快手汤；炖煮超过 45 分钟或难度高的汤留给周末。",
			Enabled:     true,
			Scope:       "candidate",
			RuleKind:    "score",
			Severity:    "soft",
			Relaxable:   true,
			Expression:  `!is_weekend && candidate.dish_role == "soup" && (candidate.cook_time > 45 || candidate.difficulty == "hard") ? -30 : 0`,
			Template:    `{"type":"avoid","scope":"meal","category":"weekday_slow_soup","n":1,"points":30,"strength":"prefer"}`,
			Priority:    280,
			Message:     "工作日费时汤降分",
		},
```

The `Expression` strings must byte-match what the template renders (the drift test enforces it). `renderPreference` emits `<predicate> ? -<points> : 0` with `Scope: "candidate"`.

(c) `backend/internal/services/menu_rule_service.go` — bump the version and rename the migration:

```go
	menuRulesSeedVersion    = 3
```

Rename `migrateMenuRulesToV2` → `syncDefaultMenuRules` and replace its doc comment:

```go
// syncDefaultMenuRules reconciles the rule table with the current factory
// set: dropped v1 rules still at their factory expression are deleted,
// missing default codes are inserted, and rows whose expression still
// matches a factory variant are refreshed to the current definition.
// User-modified rules are never touched.
func syncDefaultMenuRules() error {
```

Update the one call site in `EnsureDefaultMenuRules`:

```go
	if getSettingInt(menuRulesSeedVersionKey, 0) < menuRulesSeedVersion {
		if err := syncDefaultMenuRules(); err != nil {
			return err
		}
		return setSettingValue(menuRulesSeedVersionKey, strconv.Itoa(menuRulesSeedVersion))
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/services/ -run 'TestDefaultMenuRuleTemplatesMatchExpressions|TestMenuRuleTemplateNewCategoryPredicates|TestEnsureDefaultMenuRulesMigratesV2ToV3|TestEnsureDefaultMenuRulesMigratesV1Defaults|TestV3DefaultRulesScoreBehavior' -v`
Expected: PASS ×5

- [ ] **Step 5: Run the full backend suite**

Run: `cd backend && go test ./...`
Expected: all PASS. If another test asserts on rule counts or seed version, update it to the v3 reality (grep: `grep -rn "seed_version\|DefaultMenuRules()" backend/internal --include=*_test.go`).

- [ ] **Step 6: Commit**

```bash
git add backend/internal/models/menu_rules.go backend/internal/services/menu_rule_service.go backend/internal/services/menu_rule_templates.go backend/internal/services/menu_rule_templates_test.go backend/internal/services/plan_service_test.go
git commit -m "Seed v3 menu rules: favorite-aware and weekday slow-soup penalties"
```

---

### Task 3: Rich soups stop matching the 清淡 profile

`matchesTomorrowProfile`'s light branch currently accepts any 汤-category dish. Both the matcher and the +8 light bonus gain a richness guard, shared via a helper.

**Files:**
- Modify: `backend/internal/services/pick_service.go` (matchesTomorrowProfile light case ~line 171, tomorrowDishScore light case ~line 222, new helper `isLightSoup`)
- Modify: `backend/internal/services/pick_service_test.go` (new tests)

- [ ] **Step 1: Write the failing tests**

Append to `backend/internal/services/pick_service_test.go`:

```go
func TestLightProfileSoupRespectsRichness(t *testing.T) {
	richSoup := models.Dish{Name: "猪肚鸡汤", Category: "汤品", Taste: "浓香", RichnessLevel: 2, DishRole: "soup", TraitSource: "manual", TraitVersion: models.DishTraitVersion}
	// 酸甜 taste matches no light keyword — only the soup clause can let it in.
	clearSoup := models.Dish{Name: "番茄蛋花汤", Category: "汤品", Taste: "酸甜", RichnessLevel: 0, DishRole: "soup", TraitSource: "manual", TraitVersion: models.DishTraitVersion}

	if matchesTomorrowProfile(richSoup, "light") {
		t.Errorf("rich soup (richness 2) should not match light profile")
	}
	if !matchesTomorrowProfile(clearSoup, "light") {
		t.Errorf("clear soup (richness 0) should match light profile")
	}
	if got, want := tomorrowDishScore(clearSoup, "light"), tomorrowDishScore(richSoup, "light"); got <= want {
		t.Errorf("light score clear=%d rich=%d, want clear > rich", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/services/ -run TestLightProfileSoupRespectsRichness -v`
Expected: FAIL — rich soup currently matches via `strings.Contains(d.Category, "汤")`.

- [ ] **Step 3: Implement the richness guard**

In `backend/internal/services/pick_service.go`:

(a) Add the helper next to `matchesTomorrowProfile`:

```go
// isLightSoup reports whether a dish is a soup light enough for the 清淡
// profile — rich stews (richness_level >= 2) no longer pass just because
// their category contains 汤.
func isLightSoup(d models.Dish) bool {
	return (strings.Contains(d.Category, "汤") || isSoupDish(d)) && d.RichnessLevel <= 1
}
```

(b) In `matchesTomorrowProfile`, replace the light case:

```go
	case "light":
		return containsTaste(d.Taste, "清淡") || containsTaste(d.Taste, "鲜") || isLightSoup(d)
```

(c) In `tomorrowDishScore`'s `case "light":`, replace the 汤 bonus condition:

```go
		if isLightSoup(d) {
			score += 8
		}
```

(was `if strings.Contains(d.Category, "汤")`).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/services/ -run TestLightProfileSoupRespectsRichness -v`
Expected: PASS

- [ ] **Step 5: Run the full backend suite**

Run: `cd backend && go test ./...`
Expected: all PASS (the week-plan light profile path uses the same function; existing tests with 清淡-taste soups keep matching via the taste clause)

- [ ] **Step 6: Commit**

```bash
git add backend/internal/services/pick_service.go backend/internal/services/pick_service_test.go
git commit -m "Stop counting rich soups as light-profile matches"
```

---

### Task 4: Mirror the three penalties in tomorrow picks

The pick path doesn't run the rule engine. Penalties go in as a separate additive term during pool sorting — NOT inside `tomorrowDishScore`, which the week plan reuses (rules already cover it there; adding here would double-count).

**Files:**
- Modify: `backend/internal/services/pick_service.go` (PickTomorrowDishes ~line 80, sortTomorrowPool ~line 184, new func `pickPenaltyAdjustment`)
- Modify: `backend/internal/services/pick_service_test.go` (new tests)

- [ ] **Step 1: Write the failing tests**

Append to `backend/internal/services/pick_service_test.go`:

```go
func TestPickPenaltyAdjustment(t *testing.T) {
	favs := map[string]int{"家常菜": 2}

	fav := models.Dish{Name: "红烧肉", Category: "家常菜", Favorite: true, DishRole: "meat"}
	if got := pickPenaltyAdjustment(fav, favs, false); got != 0 {
		t.Errorf("favorite dish = %d, want 0", got)
	}
	plain := models.Dish{Name: "清炒时蔬", Category: "家常菜", DishRole: "veg"}
	if got := pickPenaltyAdjustment(plain, favs, false); got != -12 {
		t.Errorf("non-favorite = %d, want -12", got)
	}
	unfamiliar := models.Dish{Name: "大盘鸡", Category: "新疆菜", DishRole: "meat"}
	if got := pickPenaltyAdjustment(unfamiliar, favs, false); got != -37 {
		t.Errorf("unfamiliar category = %d, want -37", got)
	}
	uncategorized := models.Dish{Name: "随手菜", Category: "", DishRole: "veg"}
	if got := pickPenaltyAdjustment(uncategorized, favs, false); got != -12 {
		t.Errorf("empty category = %d, want -12 (no cuisine layer)", got)
	}
	slowSoup := models.Dish{Name: "莲藕排骨汤", Category: "家常菜", DishRole: "soup", CookTime: 90}
	if got := pickPenaltyAdjustment(slowSoup, favs, false); got != -42 {
		t.Errorf("weekday slow soup = %d, want -42 (-12 -30)", got)
	}
	if got := pickPenaltyAdjustment(slowSoup, favs, true); got != -12 {
		t.Errorf("weekend slow soup = %d, want -12", got)
	}
	quickSoup := models.Dish{Name: "紫菜蛋花汤", Category: "家常菜", DishRole: "soup", CookTime: 8}
	if got := pickPenaltyAdjustment(quickSoup, favs, false); got != -12 {
		t.Errorf("weekday quick soup = %d, want -12 (no slow penalty)", got)
	}
}

func TestSortTomorrowPoolPrefersFavorites(t *testing.T) {
	favs := map[string]int{"家常菜": 1}
	pool := []models.Dish{
		{ID: 1, Name: "大盘鸡", Category: "新疆菜", DishRole: "meat", TraitSource: "manual", TraitVersion: models.DishTraitVersion},
		{ID: 2, Name: "红烧肉", Category: "家常菜", Favorite: true, DishRole: "meat", TraitSource: "manual", TraitVersion: models.DishTraitVersion},
	}
	sortTomorrowPool(pool, "balanced", favs, false)
	if pool[0].Name != "红烧肉" {
		t.Fatalf("pool[0] = %s, want 红烧肉 (favorite of a familiar category first)", pool[0].Name)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/services/ -run 'TestPickPenaltyAdjustment|TestSortTomorrowPoolPrefersFavorites' -v`
Expected: compile error — `undefined: pickPenaltyAdjustment`, wrong arg count for `sortTomorrowPool`.

- [ ] **Step 3: Implement the pick-side mirror**

In `backend/internal/services/pick_service.go`:

(a) Add next to `tomorrowDishScore`:

```go
// pickPenaltyAdjustment mirrors the three v3 default score rules
// (non_favorite_penalty / unfamiliar_category_penalty /
// weekday_slow_soup_penalty) for the tomorrow-pick path, which doesn't run
// the rule engine. Keep the numbers in sync with models.DefaultMenuRules;
// rule-page tuning only reaches the week plan.
func pickPenaltyAdjustment(d models.Dish, categoryFavorites map[string]int, isWeekend bool) int {
	adjust := 0
	if !d.Favorite {
		adjust -= 12
	}
	if d.Category != "" && categoryFavorites[d.Category] == 0 {
		adjust -= 25
	}
	if !isWeekend && d.DishRole == "soup" && (d.CookTime > 45 || d.Difficulty == "hard") {
		adjust -= 30
	}
	return adjust
}
```

(b) Extend `sortTomorrowPool`:

```go
func sortTomorrowPool(dishes []models.Dish, profile string, categoryFavorites map[string]int, isWeekend bool) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(dishes), func(i, j int) {
		dishes[i], dishes[j] = dishes[j], dishes[i]
	})

	sort.SliceStable(dishes, func(i, j int) bool {
		a := tomorrowDishScore(dishes[i], profile) + pickPenaltyAdjustment(dishes[i], categoryFavorites, isWeekend)
		b := tomorrowDishScore(dishes[j], profile) + pickPenaltyAdjustment(dishes[j], categoryFavorites, isWeekend)
		if a != b {
			return a > b
		}
		return dishes[i].ID < dishes[j].ID
	})
}
```

(c) In `PickTomorrowDishes`, ensure traits after the blocked filter (so `RichnessLevel`/`DishRole` are inferred for the light guard and soup penalty), and pass the new context when sorting:

After `dishes = FilterBlockedDishes(dishes)` / the empty check, add:

```go
	for i := range dishes {
		dishes[i] = ensureDishTraits(dishes[i])
	}
```

Replace `sortTomorrowPool(pool, profile)` with:

```go
	tomorrow := time.Now().AddDate(0, 0, 1)
	isWeekend := tomorrow.Weekday() == time.Saturday || tomorrow.Weekday() == time.Sunday
	sortTomorrowPool(pool, profile, favoriteCategoryCounts(), isWeekend)
```

(d) Check for other `sortTomorrowPool` callers (there should be none besides PickTomorrowDishes):

Run: `grep -rn "sortTomorrowPool(" backend/internal --include=*.go | grep -v "func "`

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/services/ -run 'TestPickPenaltyAdjustment|TestSortTomorrowPoolPrefersFavorites' -v`
Expected: PASS ×2

- [ ] **Step 5: Run the full backend suite**

Run: `cd backend && go test ./...`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/services/pick_service.go backend/internal/services/pick_service_test.go
git commit -m "Mirror v3 penalties in tomorrow picks"
```

---

### Task 5: Frontend — sentences for the new rules + points stepper

The three rules arrive with templates, so they already render as preset cards. Two gaps: their category keys need Chinese labels (display-only — NOT in the sentence-builder dropdown, they have no count expression for limit rules), and avoid/prefer cards need a points stepper so the penalty numbers are tunable.

**Files:**
- Modify: `frontend/src/lib/menuRuleTemplates.ts` (labels ~line 33, categoryLabel ~line 49)
- Modify: `frontend/src/components/rules/PresetRuleCard.tsx` (NStepper bounds, points row)

- [ ] **Step 1: Add display-only labels**

In `frontend/src/lib/menuRuleTemplates.ts`, after `FIXED_CATEGORY_LABELS`:

```ts
// Display-only categories used by the v3 preset rules. Not offered in the
// sentence builder: they have no count expression, so a limit-type rule
// built from them would fail backend validation.
const DISPLAY_ONLY_CATEGORY_LABELS: Record<string, string> = {
  non_favorite: "没收藏的菜",
  unfamiliar_category: "一道收藏都没有的菜系",
  weekday_slow_soup: "工作日的费时汤",
}
```

In `categoryLabel`, change the final fallback line:

```ts
  return FIXED_CATEGORY_LABELS[category] || DISPLAY_ONLY_CATEGORY_LABELS[category] || category
```

(`CATEGORY_OPTIONS` stays built from `FIXED_CATEGORY_LABELS` only — that keeps the builder dropdown unchanged.)

- [ ] **Step 2: Add the points stepper to avoid/prefer preset cards**

In `frontend/src/components/rules/PresetRuleCard.tsx`:

(a) Give `NStepper` configurable bounds (defaults preserve current behavior):

```tsx
function NStepper({
  value,
  disabled,
  onChange,
  min = 1,
  max = 10,
}: {
  value: number
  disabled: boolean
  onChange: (next: number) => void
  min?: number
  max?: number
}) {
```

and update the two disabled conditions to `value <= min` / `value >= max`.

(b) In the component body, alongside `hasStepper`:

```tsx
  const hasPoints = tpl.type === "prefer" || tpl.type === "avoid"
  const pointsValue = tpl.points && tpl.points > 0 ? tpl.points : tpl.type === "prefer" ? 20 : 15

  function updatePoints(next: number) {
    const clamped = Math.max(1, Math.min(99, next))
    onChange({ template: withTemplate(rule, { ...tpl, points: clamped }).template })
  }
```

(The 20/15 fallbacks mirror the backend's `defaultTemplatePoints`.)

(c) After the existing `{hasStepper && (...)}` block, add:

```tsx
      {hasPoints && (
        <div className="mt-2 flex items-center justify-between gap-2 rounded-xl bg-card px-3 py-2">
          <span className="text-[11px] font-bold text-text3">{tpl.type === "avoid" ? "降多少分" : "加多少分"}</span>
          <NStepper value={pointsValue} disabled={!canEdit || !rule.enabled} onChange={updatePoints} min={1} max={99} />
        </div>
      )}
```

- [ ] **Step 3: Type-check and build**

Run: `cd frontend && npm run build`
Expected: tsc + vite build succeed with no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/menuRuleTemplates.ts frontend/src/components/rules/PresetRuleCard.tsx
git commit -m "Render v3 rule sentences and points stepper in rules UI"
```

---

### Task 6: Changelog + final verification

**Files:**
- Modify: `CHANGELOG.md` (top of file, follow the existing v0.7.0 section format)

- [ ] **Step 1: Add changelog entries**

Read the top of `CHANGELOG.md`. If an unreleased/v0.7.x section for 2026-06-11 already exists, append there; otherwise add a new section above `## v0.7.0` following the same format:

```markdown
## v0.7.1 (2026-06-11)

### 新增

- **会做感知推荐（规则 seed v3）** — 收藏=会做：未收藏的菜 -12 分，一道收藏都没有的菜系再 -25 分；收藏过的菜不受影响，陌生菜系只在候选不足时偶尔出现（留学新菜的口子）。明日推荐同步同样降权
- **工作日少排费时汤** — 周一到周五汤位对炖煮超过 45 分钟或难度 hard 的汤 -30 分，周末不罚；三条新规则都在规则页可见、可调分值、可单独关闭
- **规则卡片分值步进器** — 优先安排/尽量避开类预设规则新增「加/降多少分」调节

### 优化

- **浓汤不再算清淡** — 清淡主题只接受清汤（richness ≤ 1），浓香炖汤（如猪肚鸡汤）不再混进清淡日；汤的清淡加分同步收紧
```

- [ ] **Step 2: Full verification**

```bash
cd backend && go build ./... && go test ./...
cd ../frontend && npm run build
git status -sb
```

Expected: backend suite green, frontend builds, no stray untracked runtime files staged.

- [ ] **Step 3: Commit**

```bash
git add CHANGELOG.md
git commit -m "Document favorite-aware recommendation and soup scheduling in changelog"
```

- [ ] **Step 4: Manual smoke (optional but recommended)**

Restart the backend (kill the `exe/server` process on port 8080 via `lsof -ti:8080 | xargs kill`, then `cd backend && go run ./cmd/server`), then:

```bash
curl -s -X POST http://localhost:8080/api/week-plan/regenerate -H "X-App-Token: <token>" | head -c 2000
```

Check: a weekday with a soup quota picks a quick soup (紫菜蛋花汤/番茄蛋花汤-class) instead of a 90-minute stew; Saturday/Sunday may still pick stews. Open the rules settings page: the three new rules show as Chinese sentence cards with a 降多少分 stepper.

---

## Self-review notes

- **Spec coverage:** §3 two favorite rules → Task 2; env/data plumbing → Task 1; pick parity without double counting → Task 4 (penalties outside `tomorrowDishScore`); §4a richness guard both paths → Task 3 (+ trait-ensure in Task 4); §4b weekday rule + `is_weekend` → Tasks 1/2; §5 migration v3 + template invariant + drift test → Task 2; frontend sentences → Task 5; tuning expectations → changelog + rule-page steppers.
- **Known deliberate limits:** pick-path penalty numbers are constants mirroring the rule defaults — tuning a rule's points on the rules page affects the week plan only (documented in code comment). Display-only categories are excluded from the sentence builder because they lack count expressions.
- **Type consistency:** `weekPlanDayContext{profile, dayWant, weekWant, isWeekend, categoryFavorites}`, `evaluateScoreRules(candidate, dayCtx, quota, ...)`, `buildRuleEnv(..., quota, isWeekend)`, `dishRuleEnv(dish, categoryFavorites)`, `favoriteCategoryCounts() map[string]int`, `pickPenaltyAdjustment(dish, categoryFavorites, isWeekend)`, `sortTomorrowPool(dishes, profile, categoryFavorites, isWeekend)` — used identically across Tasks 1–4.
