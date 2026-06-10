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
	return buildRuleEnv(candidate, meal, day, week, "balanced", MealQuota{MeatCount: 1})
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
	env := dishRuleEnv(dish)
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

func TestDefaultMenuRuleTemplatesMatchExpressions(t *testing.T) {
	defaults := models.DefaultMenuRules()
	if len(defaults) != 9 {
		t.Fatalf("DefaultMenuRules() has %d rules, want 9", len(defaults))
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
	if setting.Value != "2" {
		t.Errorf("seed version = %q, want 2", setting.Value)
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
