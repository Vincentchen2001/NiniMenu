package services

import (
	"testing"

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
