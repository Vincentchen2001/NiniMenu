package services

import (
	"testing"

	"ninimenu/internal/database"
	"ninimenu/internal/models"
)

func compileTemplateRule(t *testing.T, template string) compiledMenuRule {
	t.Helper()
	rule := models.MenuRule{Code: "tpl_test", Name: "模板规则", Enabled: true, Template: template}
	if err := ApplyMenuRuleTemplate(&rule); err != nil {
		t.Fatalf("ApplyMenuRuleTemplate(%s) error = %v", template, err)
	}
	compiled, err := compileMenuRule(rule)
	if err != nil {
		t.Fatalf("compileMenuRule(%s -> %s) error = %v", template, rule.Expression, err)
	}
	return compiled
}

func runTemplateScore(t *testing.T, compiled compiledMenuRule, env map[string]any) float64 {
	t.Helper()
	out, err := exprRunRule(compiled.program, env)
	if err != nil {
		t.Fatalf("run rule %q: %v", compiled.rule.Expression, err)
	}
	value, ok := numericRuleOutput(out)
	if !ok {
		t.Fatalf("rule %q returned non-numeric %#v", compiled.rule.Expression, out)
	}
	return value
}

func runTemplateConstraint(t *testing.T, compiled compiledMenuRule, env map[string]any) bool {
	t.Helper()
	out, err := exprRunRule(compiled.program, env)
	if err != nil {
		t.Fatalf("run rule %q: %v", compiled.rule.Expression, err)
	}
	allowed, ok := out.(bool)
	if !ok {
		t.Fatalf("rule %q returned non-bool %#v", compiled.rule.Expression, out)
	}
	return allowed
}

func templateRuleEnv(candidate ruleDishEnv, meal []ruleDishEnv, day []ruleDishEnv, week []ruleDishEnv) map[string]any {
	if meal == nil {
		meal = []ruleDishEnv{}
	}
	if day == nil {
		day = []ruleDishEnv{}
	}
	if week == nil {
		week = []ruleDishEnv{}
	}
	return buildRuleEnv(candidate, meal, day, week, []ruleDishEnv{}, "balanced", MealQuota{MeatCount: 1}, false)
}

func TestMenuRuleTemplateLimitPrefer(t *testing.T) {
	compiled := compileTemplateRule(t, `{"type":"limit","scope":"meal","category":"spicy","n":1,"points":12}`)
	if compiled.rule.RuleKind != menuRuleKindScore {
		t.Errorf("limit/prefer should render a score rule, got %s", compiled.rule.RuleKind)
	}
	if !compiled.rule.Relaxable || compiled.rule.Severity != "soft" {
		t.Errorf("limit/prefer should be soft+relaxable, got severity=%s relaxable=%v", compiled.rule.Severity, compiled.rule.Relaxable)
	}
	if compiled.rule.Scope != "meal" {
		t.Errorf("limit scope should stay meal, got %s", compiled.rule.Scope)
	}

	spicy := ruleDishEnv{Name: "辣子鸡", SpiceLevel: 2}
	mild := ruleDishEnv{Name: "白灼菜心"}
	mealWithSpicy := []ruleDishEnv{{Name: "麻婆豆腐", SpiceLevel: 2}}

	if got := runTemplateScore(t, compiled, templateRuleEnv(spicy, mealWithSpicy, nil, nil)); got != -12 {
		t.Errorf("spicy candidate with spicy meal = %v, want -12", got)
	}
	if got := runTemplateScore(t, compiled, templateRuleEnv(spicy, nil, nil, nil)); got != 0 {
		t.Errorf("spicy candidate with empty meal = %v, want 0", got)
	}
	if got := runTemplateScore(t, compiled, templateRuleEnv(mild, mealWithSpicy, nil, nil)); got != 0 {
		t.Errorf("mild candidate = %v, want 0", got)
	}
}

func TestMenuRuleTemplateLimitMust(t *testing.T) {
	compiled := compileTemplateRule(t, `{"type":"limit","scope":"day","category":"egg","strength":"must","n":1}`)
	if compiled.rule.RuleKind != menuRuleKindConstraint {
		t.Errorf("limit/must should render a constraint, got %s", compiled.rule.RuleKind)
	}
	if compiled.rule.Relaxable || compiled.rule.Severity != "hard" {
		t.Errorf("limit/must should be hard+non-relaxable, got severity=%s relaxable=%v", compiled.rule.Severity, compiled.rule.Relaxable)
	}

	egg := ruleDishEnv{Name: "番茄炒蛋", ProteinSources: []string{"egg"}}
	dayWithEgg := []ruleDishEnv{{Name: "蒸水蛋", ProteinSources: []string{"egg"}}}

	if runTemplateConstraint(t, compiled, templateRuleEnv(egg, nil, dayWithEgg, nil)) {
		t.Errorf("second egg dish of the day should be blocked")
	}
	if !runTemplateConstraint(t, compiled, templateRuleEnv(egg, nil, nil, nil)) {
		t.Errorf("first egg dish of the day should pass")
	}
	if !runTemplateConstraint(t, compiled, templateRuleEnv(ruleDishEnv{Name: "清炒西兰花"}, nil, dayWithEgg, nil)) {
		t.Errorf("non-egg candidate should pass")
	}
}

func TestMenuRuleTemplateNoRepeat(t *testing.T) {
	prefer := compileTemplateRule(t, `{"type":"no_repeat","scope":"meal","n":1,"points":14}`)
	pork := ruleDishEnv{Name: "回锅肉", ProteinSources: []string{"pork"}}
	soy := ruleDishEnv{Name: "家常豆腐", ProteinSources: []string{"soy"}}
	mealWithPork := []ruleDishEnv{{Name: "红烧肉", ProteinSources: []string{"pork"}}}
	mealWithSoy := []ruleDishEnv{{Name: "麻婆豆腐", ProteinSources: []string{"soy"}}}

	if got := runTemplateScore(t, prefer, templateRuleEnv(pork, mealWithPork, nil, nil)); got != -14 {
		t.Errorf("pork-on-pork = %v, want -14", got)
	}
	if got := runTemplateScore(t, prefer, templateRuleEnv(pork, nil, nil, nil)); got != 0 {
		t.Errorf("pork with empty meal = %v, want 0", got)
	}
	if got := runTemplateScore(t, prefer, templateRuleEnv(soy, mealWithSoy, nil, nil)); got != 0 {
		t.Errorf("soy-only candidate should be exempt, got %v", got)
	}

	must := compileTemplateRule(t, `{"type":"no_repeat","scope":"meal","strength":"must","n":1}`)
	if must.rule.RuleKind != menuRuleKindConstraint {
		t.Errorf("no_repeat/must should render a constraint, got %s", must.rule.RuleKind)
	}
	if runTemplateConstraint(t, must, templateRuleEnv(pork, mealWithPork, nil, nil)) {
		t.Errorf("pork-on-pork should be blocked by must variant")
	}
	if !runTemplateConstraint(t, must, templateRuleEnv(soy, mealWithSoy, nil, nil)) {
		t.Errorf("soy-only candidate should pass must variant")
	}
}

func TestMenuRuleTemplatePreferAndAvoid(t *testing.T) {
	prefer := compileTemplateRule(t, `{"type":"prefer","category":"protein:poultry","points":20}`)
	if prefer.rule.Scope != "candidate" {
		t.Errorf("prefer scope should be candidate, got %s", prefer.rule.Scope)
	}
	poultry := ruleDishEnv{Name: "香煎鸡腿", ProteinSources: []string{"poultry"}}
	beef := ruleDishEnv{Name: "黑椒牛柳", ProteinSources: []string{"beef"}}
	if got := runTemplateScore(t, prefer, templateRuleEnv(poultry, nil, nil, nil)); got != 20 {
		t.Errorf("poultry candidate = %v, want 20", got)
	}
	if got := runTemplateScore(t, prefer, templateRuleEnv(beef, nil, nil, nil)); got != 0 {
		t.Errorf("beef candidate = %v, want 0", got)
	}

	avoid := compileTemplateRule(t, `{"type":"avoid","category":"ingredient:香菜","points":10}`)
	byName := ruleDishEnv{Name: "香菜牛肉"}
	byIngredient := ruleDishEnv{Name: "凉拌牛肉", Ingredients: []string{"牛肉", "香菜"}}
	clean := ruleDishEnv{Name: "清蒸鲈鱼", Ingredients: []string{"鲈鱼"}}
	if got := runTemplateScore(t, avoid, templateRuleEnv(byName, nil, nil, nil)); got != -10 {
		t.Errorf("name match = %v, want -10", got)
	}
	if got := runTemplateScore(t, avoid, templateRuleEnv(byIngredient, nil, nil, nil)); got != -10 {
		t.Errorf("ingredient match = %v, want -10", got)
	}
	if got := runTemplateScore(t, avoid, templateRuleEnv(clean, nil, nil, nil)); got != 0 {
		t.Errorf("clean candidate = %v, want 0", got)
	}
}

func TestMenuRuleTemplateRejectsInvalidInput(t *testing.T) {
	invalid := []string{
		`{"type":"explode"}`,
		`{"type":"limit","category":"nonsense"}`,
		`{"type":"prefer","category":"protein:unobtainium"}`,
		`{"type":"avoid","category":"ingredient:"}`,
		`{"type":"avoid","category":"ingredient:bad\"quote"}`,
		`not json`,
	}
	for _, template := range invalid {
		rule := models.MenuRule{Code: "tpl_bad", Name: "bad", Template: template}
		if err := ApplyMenuRuleTemplate(&rule); err == nil {
			t.Errorf("ApplyMenuRuleTemplate(%s) should fail", template)
		}
	}

	blank := models.MenuRule{Code: "manual", Name: "manual", Expression: "candidate.favorite ? 5 : 0", RuleKind: menuRuleKindScore}
	if err := ApplyMenuRuleTemplate(&blank); err != nil {
		t.Errorf("blank template should be a no-op, got %v", err)
	}
	if blank.Expression != "candidate.favorite ? 5 : 0" {
		t.Errorf("blank template must not touch expression, got %q", blank.Expression)
	}
}

func TestDishRuleEnvIncludesIngredients(t *testing.T) {
	dish := models.Dish{
		Name:        "凉拌牛肉",
		Ingredients: `[{"name":"牛肉","amount":"200g"},{"name":"香菜","amount":"1把"}]`,
	}
	env := dishRuleEnv(dish, nil)
	if !containsString(env.Ingredients, "香菜") || !containsString(env.Ingredients, "牛肉") {
		t.Fatalf("dishRuleEnv ingredients = %v, want 牛肉+香菜", env.Ingredients)
	}
}

func TestCountOverlapDayAndWeekHelpers(t *testing.T) {
	compiled := compileTemplateRule(t, `{"type":"no_repeat","scope":"week","n":2,"points":20}`)
	pork := ruleDishEnv{Name: "回锅肉", ProteinSources: []string{"pork"}}
	weekWithTwoPork := []ruleDishEnv{
		{Name: "红烧肉", ProteinSources: []string{"pork"}},
		{Name: "糖醋排骨", ProteinSources: []string{"pork"}},
	}

	if got := runTemplateScore(t, compiled, templateRuleEnv(pork, nil, nil, weekWithTwoPork)); got != -20 {
		t.Errorf("third pork dish of the week = %v, want -20", got)
	}
	if got := runTemplateScore(t, compiled, templateRuleEnv(pork, nil, nil, nil)); got != 0 {
		t.Errorf("pork with empty week = %v, want 0", got)
	}

	dayRule := models.MenuRule{
		Code:       "overlap_day_probe",
		Name:       "day overlap",
		RuleKind:   menuRuleKindScore,
		Expression: `countOverlapDay("protein_sources", candidate.protein_sources) >= 1 ? -9 : 0`,
	}
	compiledDay, err := compileMenuRule(dayRule)
	if err != nil {
		t.Fatalf("compile countOverlapDay rule: %v", err)
	}
	dayWithPork := []ruleDishEnv{{Name: "卤肉饭", ProteinSources: []string{"pork"}}}
	if got := runTemplateScore(t, compiledDay, templateRuleEnv(pork, nil, dayWithPork, nil)); got != -9 {
		t.Errorf("countOverlapDay with pork day = %v, want -9", got)
	}
	if got := runTemplateScore(t, compiledDay, templateRuleEnv(pork, nil, nil, nil)); got != 0 {
		t.Errorf("countOverlapDay with empty day = %v, want 0", got)
	}
}

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

func TestDefaultMenuRuleTemplatesMatchExpressions(t *testing.T) {
	defaults := models.DefaultMenuRules()
	if len(defaults) != 14 {
		t.Fatalf("DefaultMenuRules() has %d rules, want 14", len(defaults))
	}
	for _, rule := range defaults {
		if rule.Template == "" {
			t.Errorf("rule %s has no template", rule.Code)
			continue
		}
		clone := rule
		clone.Expression = ""
		if err := ApplyMenuRuleTemplate(&clone); err != nil {
			t.Errorf("rule %s template render failed: %v", rule.Code, err)
			continue
		}
		if clone.Expression != rule.Expression {
			t.Errorf("rule %s expression drift:\nstored:   %s\nrendered: %s", rule.Code, rule.Expression, clone.Expression)
		}
		if clone.RuleKind != rule.RuleKind || clone.Severity != rule.Severity || clone.Relaxable != rule.Relaxable || clone.Scope != rule.Scope {
			t.Errorf("rule %s rendered meta mismatch: kind=%s/%s severity=%s/%s relaxable=%v/%v scope=%s/%s",
				rule.Code, clone.RuleKind, rule.RuleKind, clone.Severity, rule.Severity, clone.Relaxable, rule.Relaxable, clone.Scope, rule.Scope)
		}
		if _, err := compileMenuRule(rule); err != nil {
			t.Errorf("rule %s does not compile: %v", rule.Code, err)
		}
	}
}

// legacyV1MenuRulesForTest is the verbatim pre-migration factory rule set.
func legacyV1MenuRulesForTest() []models.MenuRule {
	return []models.MenuRule{
		{Code: "avoid_double_egg", Name: "同餐蛋类最多一份", Enabled: true, Scope: "meal", RuleKind: "constraint", Severity: "hard", Relaxable: false, Expression: `!has(candidate.protein_sources, "egg") || countMeal("protein_sources", "egg") == 0`, Priority: 10},
		{Code: "limit_cold_dishes", Name: "同餐凉菜最多一份", Enabled: true, Scope: "meal", RuleKind: "constraint", Severity: "hard", Relaxable: false, Expression: `candidate.serving_temperature != "cold" || countMeal("serving_temperature", "cold") == 0`, Priority: 20},
		{Code: "limit_staples", Name: "同餐主食最多一份", Enabled: true, Scope: "meal", RuleKind: "constraint", Severity: "hard", Relaxable: false, Expression: `candidate.dish_role != "staple" || countMeal("dish_role", "staple") == 0`, Priority: 30},
		{Code: "avoid_same_primary_protein", Name: "同餐主蛋白重复降分", Enabled: true, Scope: "meal", RuleKind: "score", Severity: "soft", Relaxable: true, Expression: `countOverlapMeal("protein_sources", candidate.protein_sources) > 0 && !hasOnly(candidate.protein_sources, "soy") ? -14 : 0`, Priority: 40},
		{Code: "quick_profile_bonus", Name: "快手画像加分", Enabled: true, Scope: "candidate", RuleKind: "score", Severity: "soft", Relaxable: true, Expression: `profile == "quick" && (candidate.difficulty == "easy" || candidate.cook_time <= 25) ? 24 : 0`, Priority: 100},
		{Code: "light_profile_bonus", Name: "清淡画像加分", Enabled: true, Scope: "candidate", RuleKind: "score", Severity: "soft", Relaxable: true, Expression: `profile == "light" && candidate.spice_level == 0 && candidate.richness_level <= 1 ? 22 : 0`, Priority: 110},
		{Code: "spicy_profile_bonus", Name: "辣味画像加分", Enabled: true, Scope: "candidate", RuleKind: "score", Severity: "soft", Relaxable: true, Expression: `profile == "spicy" && candidate.spice_level > 0 ? 24 : 0`, Priority: 120},
		{Code: "favorite_profile_bonus", Name: "收藏画像加分", Enabled: true, Scope: "candidate", RuleKind: "score", Severity: "soft", Relaxable: true, Expression: `profile == "favorite" && candidate.favorite ? 36 : 0`, Priority: 130},
		{Code: "hot_cold_balance_bonus", Name: "冷热搭配加分", Enabled: true, Scope: "meal", RuleKind: "score", Severity: "soft", Relaxable: true, Expression: `(candidate.serving_temperature == "cold" && countMeal("serving_temperature", "hot") > 0) || (candidate.serving_temperature == "hot" && countMeal("serving_temperature", "cold") > 0) ? 6 : 0`, Priority: 200},
		{Code: "avoid_heavy_spicy_day", Name: "重油重辣不连续", Enabled: true, Scope: "day", RuleKind: "score", Severity: "soft", Relaxable: true, Expression: `candidate.spice_level >= 2 && candidate.richness_level >= 2 && countDay("heavy_spicy", "true") > 0 ? -18 : 0`, Priority: 210},
	}
}

func menuRuleCodeSet(rules []models.MenuRule) map[string]models.MenuRule {
	result := make(map[string]models.MenuRule, len(rules))
	for _, rule := range rules {
		result[rule.Code] = rule
	}
	return result
}

func TestEnsureDefaultMenuRulesMigratesV1Defaults(t *testing.T) {
	setupPlanServiceTestDB(t)

	for _, rule := range legacyV1MenuRulesForTest() {
		rule := rule
		if err := database.DB.Create(&rule).Error; err != nil {
			t.Fatalf("seed legacy rule %s: %v", rule.Code, err)
		}
	}

	if err := EnsureDefaultMenuRules(); err != nil {
		t.Fatalf("EnsureDefaultMenuRules() error = %v", err)
	}

	var rules []models.MenuRule
	if err := database.DB.Find(&rules).Error; err != nil {
		t.Fatalf("load rules: %v", err)
	}
	got := menuRuleCodeSet(rules)

	wantCodes := []string{
		"avoid_double_egg", "limit_cold_dishes", "limit_staples",
		"avoid_same_primary_protein", "avoid_heavy_spicy_day",
		"spicy_meal_spread", "slow_meal_spread",
		"weekly_protein_variety", "weekly_fried_limit",
		"non_favorite_penalty", "unfamiliar_category_penalty", "weekday_slow_soup_penalty",
		"weekly_slow_soup_limit", "soup_ingredient_repeat_penalty",
	}
	if len(rules) != len(wantCodes) {
		t.Errorf("after migration %d rules, want %d (%v)", len(rules), len(wantCodes), got)
	}
	for _, code := range wantCodes {
		rule, ok := got[code]
		if !ok {
			t.Errorf("missing rule %s after migration", code)
			continue
		}
		if rule.Template == "" {
			t.Errorf("rule %s has no template after migration", code)
		}
	}
	for _, code := range []string{"quick_profile_bonus", "light_profile_bonus", "spicy_profile_bonus", "favorite_profile_bonus", "hot_cold_balance_bonus"} {
		if _, ok := got[code]; ok {
			t.Errorf("dropped rule %s still present after migration", code)
		}
	}

	var setting models.Setting
	if err := database.DB.Where("`key` = ?", "menu_rules_seed_version").First(&setting).Error; err != nil {
		t.Fatalf("seed version setting missing: %v", err)
	}
	if setting.Value != "4" {
		t.Errorf("seed version = %q, want 4", setting.Value)
	}

	if err := EnsureDefaultMenuRules(); err != nil {
		t.Fatalf("second EnsureDefaultMenuRules() error = %v", err)
	}
	var countAfter int64
	database.DB.Model(&models.MenuRule{}).Count(&countAfter)
	if countAfter != int64(len(wantCodes)) {
		t.Errorf("second ensure changed rule count to %d", countAfter)
	}
}

func TestEnsureDefaultMenuRulesMigratesV2ToCurrent(t *testing.T) {
	setupPlanServiceTestDB(t)

	// Seed a v2-era database: the 9 v2 factory rules, one user-modified and
	// one toggled off in the UI (factory expression/template, Enabled=false).
	var factoryDoubleEggExpression, factoryDoubleEggTemplate string
	for _, rule := range models.DefaultMenuRules() {
		rule := rule
		switch rule.Code {
		case "non_favorite_penalty", "unfamiliar_category_penalty", "weekday_slow_soup_penalty",
			"weekly_slow_soup_limit", "soup_ingredient_repeat_penalty":
			continue // v3/v4 additions don't exist in a v2 database
		}
		if rule.Code == "weekly_fried_limit" {
			rule.Expression = `has(candidate.cooking_methods, "deep_fry") && countWeek("cooking_methods", "deep_fry") >= 5 ? -20 : 0`
			rule.Template = ""
		}
		if rule.Code == "avoid_double_egg" {
			factoryDoubleEggExpression = rule.Expression
			factoryDoubleEggTemplate = rule.Template
		}
		if err := database.DB.Create(&rule).Error; err != nil {
			t.Fatalf("seed v2 rule %s: %v", rule.Code, err)
		}
	}
	// Toggle one factory rule off the way the rules page does (column
	// update); Create alone would lose Enabled=false to the gorm default.
	if err := database.DB.Model(&models.MenuRule{}).Where("code = ?", "avoid_double_egg").Update("enabled", false).Error; err != nil {
		t.Fatalf("disable avoid_double_egg: %v", err)
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
	if len(rules) != 14 {
		t.Fatalf("after migration %d rules, want 14 (%v)", len(rules), got)
	}
	for _, code := range []string{"non_favorite_penalty", "unfamiliar_category_penalty", "weekday_slow_soup_penalty", "weekly_slow_soup_limit", "soup_ingredient_repeat_penalty"} {
		if _, ok := got[code]; !ok {
			t.Errorf("missing v3 rule %s after migration", code)
		}
	}
	wantModified := `has(candidate.cooking_methods, "deep_fry") && countWeek("cooking_methods", "deep_fry") >= 5 ? -20 : 0`
	if got["weekly_fried_limit"].Expression != wantModified {
		t.Errorf("user-modified rule overwritten:\ngot:  %s\nwant: %s", got["weekly_fried_limit"].Expression, wantModified)
	}

	// The rule the user toggled off must survive the sync untouched.
	var doubleEggCount int64
	if err := database.DB.Model(&models.MenuRule{}).Where("code = ?", "avoid_double_egg").Count(&doubleEggCount).Error; err != nil {
		t.Fatalf("count avoid_double_egg rows: %v", err)
	}
	if doubleEggCount != 1 {
		t.Errorf("avoid_double_egg row count = %d, want 1", doubleEggCount)
	}
	disabled := got["avoid_double_egg"]
	if disabled.Enabled {
		t.Errorf("user-disabled avoid_double_egg was re-enabled by migration")
	}
	if disabled.Expression != factoryDoubleEggExpression {
		t.Errorf("disabled rule expression changed:\ngot:  %s\nwant: %s", disabled.Expression, factoryDoubleEggExpression)
	}
	if disabled.Template != factoryDoubleEggTemplate {
		t.Errorf("disabled rule template changed:\ngot:  %s\nwant: %s", disabled.Template, factoryDoubleEggTemplate)
	}

	var setting models.Setting
	if err := database.DB.Where("`key` = ?", "menu_rules_seed_version").First(&setting).Error; err != nil {
		t.Fatalf("seed version setting missing: %v", err)
	}
	if setting.Value != "4" {
		t.Errorf("seed version = %q, want 4", setting.Value)
	}
}

func TestEnsureDefaultMenuRulesKeepsUserModifiedRules(t *testing.T) {
	setupPlanServiceTestDB(t)

	customDropped := models.MenuRule{
		Code: "quick_profile_bonus", Name: "我的快手规则", Enabled: true,
		Scope: "candidate", RuleKind: "score", Severity: "soft", Relaxable: true,
		Expression: `profile == "quick" ? 50 : 0`,
	}
	customKept := models.MenuRule{
		Code: "avoid_double_egg", Name: "我的蛋类规则", Enabled: true,
		Scope: "meal", RuleKind: "constraint", Severity: "hard", Relaxable: false,
		Expression: `!has(candidate.protein_sources, "egg") || countMeal("protein_sources", "egg") < 2`,
	}
	for _, rule := range []models.MenuRule{customDropped, customKept} {
		rule := rule
		if err := database.DB.Create(&rule).Error; err != nil {
			t.Fatalf("seed custom rule %s: %v", rule.Code, err)
		}
	}

	if err := EnsureDefaultMenuRules(); err != nil {
		t.Fatalf("EnsureDefaultMenuRules() error = %v", err)
	}

	var dropped models.MenuRule
	if err := database.DB.Where("code = ?", "quick_profile_bonus").First(&dropped).Error; err != nil {
		t.Fatalf("user-modified dropped rule should survive migration: %v", err)
	}
	if dropped.Expression != `profile == "quick" ? 50 : 0` {
		t.Errorf("user-modified dropped rule expression changed: %q", dropped.Expression)
	}

	var kept models.MenuRule
	if err := database.DB.Where("code = ?", "avoid_double_egg").First(&kept).Error; err != nil {
		t.Fatalf("kept rule missing: %v", err)
	}
	if kept.Expression != `!has(candidate.protein_sources, "egg") || countMeal("protein_sources", "egg") < 2` {
		t.Errorf("user-modified kept rule was overwritten: %q", kept.Expression)
	}
	if kept.Name != "我的蛋类规则" {
		t.Errorf("user-modified kept rule renamed: %q", kept.Name)
	}
}

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
