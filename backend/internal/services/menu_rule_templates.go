package services

import (
	"encoding/json"
	"fmt"
	"strings"

	"ninimenu/internal/models"
)

// MenuRuleTemplate is the sentence-builder representation of a rule. The
// rendered expression stays the execution truth; the template only drives
// rendering and the friendly UI.
type MenuRuleTemplate struct {
	Type     string `json:"type"`     // limit | no_repeat | prefer | avoid
	Scope    string `json:"scope"`    // meal | day | week (limit / no_repeat)
	Category string `json:"category"` // egg/cold/staple/soup/spicy/heavy_spicy/deep_fry/slow/protein:K/ingredient:W
	N        int    `json:"n"`        // limit threshold / no_repeat overlap threshold
	Points   int    `json:"points"`   // score magnitude
	Strength string `json:"strength"` // must | prefer
}

type renderedMenuRule struct {
	Expression string
	RuleKind   string
	Severity   string
	Relaxable  bool
	Scope      string
}

// ApplyMenuRuleTemplate renders rule.Template (when present) into the
// executable fields, overwriting Expression/RuleKind/Severity/Relaxable/Scope.
func ApplyMenuRuleTemplate(rule *models.MenuRule) error {
	raw := strings.TrimSpace(rule.Template)
	if raw == "" {
		return nil
	}
	var template MenuRuleTemplate
	if err := json.Unmarshal([]byte(raw), &template); err != nil {
		return fmt.Errorf("规则模板格式无效: %v", err)
	}
	rendered, err := template.render()
	if err != nil {
		return err
	}
	rule.Expression = rendered.Expression
	rule.RuleKind = rendered.RuleKind
	rule.Severity = rendered.Severity
	rule.Relaxable = rendered.Relaxable
	rule.Scope = rendered.Scope
	return nil
}

func (t MenuRuleTemplate) render() (renderedMenuRule, error) {
	t.Type = strings.ToLower(strings.TrimSpace(t.Type))
	t.Strength = strings.ToLower(strings.TrimSpace(t.Strength))
	if t.Strength == "" {
		t.Strength = "prefer"
	}
	if t.Strength != "must" && t.Strength != "prefer" {
		return renderedMenuRule{}, fmt.Errorf("规则模板强度无效: %s", t.Strength)
	}
	t.Scope = strings.ToLower(strings.TrimSpace(t.Scope))
	if t.Scope == "" {
		t.Scope = "meal"
	}
	if t.Scope != "meal" && t.Scope != "day" && t.Scope != "week" {
		return renderedMenuRule{}, fmt.Errorf("规则模板范围无效: %s", t.Scope)
	}
	t.Category = strings.TrimSpace(t.Category)
	t.N = clampInt(t.N, 0, 10)
	if t.N == 0 {
		t.N = 1
	}
	t.Points = clampInt(t.Points, 0, 100)
	if t.Points == 0 {
		t.Points = defaultTemplatePoints(t.Type)
	}

	switch t.Type {
	case "limit":
		return t.renderLimit()
	case "no_repeat":
		return t.renderNoRepeat()
	case "prefer", "avoid":
		return t.renderPreference()
	default:
		return renderedMenuRule{}, fmt.Errorf("规则模板类型无效: %s", t.Type)
	}
}

func defaultTemplatePoints(templateType string) int {
	switch templateType {
	case "no_repeat":
		return 14
	case "prefer":
		return 20
	default: // limit, avoid
		return 15
	}
}

func (t MenuRuleTemplate) renderLimit() (renderedMenuRule, error) {
	predicate, err := menuRuleCategoryPredicate(t.Category)
	if err != nil {
		return renderedMenuRule{}, err
	}
	count, err := menuRuleCategoryCount(t.Category, t.Scope)
	if err != nil {
		return renderedMenuRule{}, err
	}
	if t.Strength == "must" {
		return renderedMenuRule{
			Expression: fmt.Sprintf("!(%s) || %s < %d", predicate, count, t.N),
			RuleKind:   menuRuleKindConstraint,
			Severity:   "hard",
			Relaxable:  false,
			Scope:      t.Scope,
		}, nil
	}
	return renderedMenuRule{
		Expression: fmt.Sprintf("%s && %s >= %d ? -%d : 0", predicate, count, t.N, t.Points),
		RuleKind:   menuRuleKindScore,
		Severity:   "soft",
		Relaxable:  true,
		Scope:      t.Scope,
	}, nil
}

func (t MenuRuleTemplate) renderNoRepeat() (renderedMenuRule, error) {
	overlap := fmt.Sprintf(`countOverlap%s("protein_sources", candidate.protein_sources)`, templateScopeSuffix(t.Scope))
	if t.Strength == "must" {
		return renderedMenuRule{
			Expression: fmt.Sprintf(`%s < %d || hasOnly(candidate.protein_sources, "soy")`, overlap, t.N),
			RuleKind:   menuRuleKindConstraint,
			Severity:   "hard",
			Relaxable:  false,
			Scope:      t.Scope,
		}, nil
	}
	return renderedMenuRule{
		Expression: fmt.Sprintf(`%s >= %d && !hasOnly(candidate.protein_sources, "soy") ? -%d : 0`, overlap, t.N, t.Points),
		RuleKind:   menuRuleKindScore,
		Severity:   "soft",
		Relaxable:  true,
		Scope:      t.Scope,
	}, nil
}

func (t MenuRuleTemplate) renderPreference() (renderedMenuRule, error) {
	predicate, err := menuRuleCategoryPredicate(t.Category)
	if err != nil {
		return renderedMenuRule{}, err
	}
	points := t.Points
	if t.Type == "avoid" {
		points = -points
	}
	return renderedMenuRule{
		Expression: fmt.Sprintf("%s ? %d : 0", predicate, points),
		RuleKind:   menuRuleKindScore,
		Severity:   "soft",
		Relaxable:  true,
		Scope:      "candidate",
	}, nil
}

func templateScopeSuffix(scope string) string {
	switch scope {
	case "day":
		return "Day"
	case "week":
		return "Week"
	default:
		return "Meal"
	}
}

func menuRuleCategoryPredicate(category string) (string, error) {
	switch {
	case category == "egg":
		return `has(candidate.protein_sources, "egg")`, nil
	case category == "cold":
		return `candidate.serving_temperature == "cold"`, nil
	case category == "staple":
		return `candidate.dish_role == "staple"`, nil
	case category == "soup":
		return `candidate.dish_role == "soup"`, nil
	case category == "spicy":
		return `candidate.spice_level >= 1`, nil
	case category == "heavy_spicy":
		return `candidate.spice_level >= 2 && candidate.richness_level >= 2`, nil
	case category == "deep_fry":
		return `has(candidate.cooking_methods, "deep_fry")`, nil
	case category == "slow":
		return `candidate.cook_time > 45`, nil
	case category == "non_favorite":
		return `!candidate.favorite`, nil
	case category == "unfamiliar_category":
		return `candidate.category != "" && candidate.category_favorites == 0`, nil
	case category == "weekday_slow_soup":
		return `!is_weekend && candidate.dish_role == "soup" && (candidate.cook_time > 45 || candidate.difficulty == "hard")`, nil
	case category == "slow_soup":
		return `candidate.dish_role == "soup" && candidate.cook_time > 45`, nil
	case category == "soup_ingredient_repeat":
		return `candidate.dish_role == "soup" && countOverlapPrevSoup("ingredients", candidate.ingredients) > 0`, nil
	case strings.HasPrefix(category, "protein:"):
		key, err := templateProteinKey(category)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`has(candidate.protein_sources, %q)`, key), nil
	case strings.HasPrefix(category, "ingredient:"):
		word, err := templateIngredientWord(category)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`(candidate.name contains %q || any(candidate.ingredients, # contains %q))`, word, word), nil
	default:
		return "", fmt.Errorf("规则模板类别无效: %s", category)
	}
}

func menuRuleCategoryCount(category string, scope string) (string, error) {
	suffix := templateScopeSuffix(scope)
	switch {
	case category == "egg":
		return fmt.Sprintf(`count%s("protein_sources", "egg")`, suffix), nil
	case category == "cold":
		return fmt.Sprintf(`count%s("serving_temperature", "cold")`, suffix), nil
	case category == "staple":
		return fmt.Sprintf(`count%s("dish_role", "staple")`, suffix), nil
	case category == "soup":
		return fmt.Sprintf(`count%s("dish_role", "soup")`, suffix), nil
	case category == "spicy":
		return fmt.Sprintf(`len(filter(%s, .spice_level >= 1))`, scope), nil
	case category == "heavy_spicy":
		return fmt.Sprintf(`count%s("heavy_spicy", "true")`, suffix), nil
	case category == "deep_fry":
		return fmt.Sprintf(`count%s("cooking_methods", "deep_fry")`, suffix), nil
	case category == "slow":
		return fmt.Sprintf(`len(filter(%s, .cook_time > 45))`, scope), nil
	case category == "slow_soup":
		return fmt.Sprintf(`len(filter(%s, .dish_role == "soup" && .cook_time > 45))`, scope), nil
	case strings.HasPrefix(category, "protein:"):
		key, err := templateProteinKey(category)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`count%s("protein_sources", %q)`, suffix, key), nil
	case strings.HasPrefix(category, "ingredient:"):
		// v1 limitation: scope counting matches dish names only; the
		// candidate predicate still checks ingredients.
		word, err := templateIngredientWord(category)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`len(filter(%s, .name contains %q))`, scope, word), nil
	default:
		return "", fmt.Errorf("规则模板类别无效: %s", category)
	}
}

func templateProteinKey(category string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(category, "protein:")))
	if !weekPlanProteinKeys[key] {
		return "", fmt.Errorf("规则模板蛋白类别无效: %s", category)
	}
	return key, nil
}

func templateIngredientWord(category string) (string, error) {
	word := strings.TrimSpace(strings.TrimPrefix(category, "ingredient:"))
	if word == "" {
		return "", fmt.Errorf("规则模板食材关键词不能为空")
	}
	if strings.ContainsAny(word, "\"\\`") {
		return "", fmt.Errorf("规则模板食材关键词包含非法字符")
	}
	return word, nil
}
