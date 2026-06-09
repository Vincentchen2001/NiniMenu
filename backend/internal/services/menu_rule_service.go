package services

import (
	"encoding/json"
	"fmt"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"sort"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"gorm.io/gorm"
)

const (
	menuRuleKindConstraint = "constraint"
	menuRuleKindScore      = "score"
)

type ruleDishEnv struct {
	ID                 uint     `expr:"id"`
	Name               string   `expr:"name"`
	Category           string   `expr:"category"`
	MealType           string   `expr:"meal_type"`
	Taste              string   `expr:"taste"`
	DishRole           string   `expr:"dish_role"`
	ProteinSources     []string `expr:"protein_sources"`
	ServingTemperature string   `expr:"serving_temperature"`
	CookingMethods     []string `expr:"cooking_methods"`
	SpiceLevel         int      `expr:"spice_level"`
	RichnessLevel      int      `expr:"richness_level"`
	CarbLevel          int      `expr:"carb_level"`
	CookTime           int      `expr:"cook_time"`
	Difficulty         string   `expr:"difficulty"`
	Favorite           bool     `expr:"favorite"`
	Tags               []string `expr:"tags"`
}

type ruleQuotaEnv struct {
	MeatCount int `expr:"meat_count"`
	VegCount  int `expr:"veg_count"`
	SoupCount int `expr:"soup_count"`
}

type compiledMenuRule struct {
	rule    models.MenuRule
	program *vm.Program
}

func ListMenuRules() ([]models.MenuRule, error) {
	if err := EnsureDefaultMenuRules(); err != nil {
		return nil, err
	}
	var rules []models.MenuRule
	if err := database.DB.Order("priority ASC, id ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		rules = models.DefaultMenuRules()
	}
	sortMenuRules(rules)
	return rules, nil
}

func EnsureDefaultMenuRules() error {
	if database.DB == nil {
		return nil
	}
	for _, rule := range models.DefaultMenuRules() {
		if err := normalizeAndValidateMenuRule(&rule); err != nil {
			return err
		}
		var existing models.MenuRule
		if err := database.DB.Where("code = ?", rule.Code).First(&existing).Error; err == nil {
			continue
		} else if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		if err := database.DB.Create(&rule).Error; err != nil {
			return err
		}
	}
	return nil
}

func SaveMenuRules(rules []models.MenuRule) ([]models.MenuRule, error) {
	seen := make(map[string]bool, len(rules))
	for i := range rules {
		if err := normalizeAndValidateMenuRule(&rules[i]); err != nil {
			return nil, err
		}
		if seen[rules[i].Code] {
			return nil, fmt.Errorf("规则编码重复: %s", rules[i].Code)
		}
		seen[rules[i].Code] = true
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		for _, rule := range rules {
			var existing models.MenuRule
			err := tx.Where("code = ?", rule.Code).First(&existing).Error
			if err == nil {
				rule.ID = existing.ID
				rule.CreatedAt = existing.CreatedAt
				if err := tx.Save(&rule).Error; err != nil {
					return err
				}
				continue
			}
			if err != gorm.ErrRecordNotFound {
				return err
			}
			if err := tx.Create(&rule).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	InvalidateWeekPlanCache()
	return ListMenuRules()
}

func ValidateMenuRuleExpression(rule models.MenuRule) error {
	if err := normalizeAndValidateMenuRule(&rule); err != nil {
		return err
	}
	return nil
}

func normalizeAndValidateMenuRule(rule *models.MenuRule) error {
	rule.Code = strings.TrimSpace(rule.Code)
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Scope = normalizeMenuRuleScope(rule.Scope)
	rule.RuleKind = normalizeMenuRuleKind(rule.RuleKind)
	rule.Severity = normalizeMenuRuleSeverity(rule.Severity)
	rule.Expression = strings.TrimSpace(rule.Expression)
	if rule.Code == "" {
		return fmt.Errorf("规则编码不能为空")
	}
	if rule.Name == "" {
		rule.Name = rule.Code
	}
	if rule.Expression == "" {
		return fmt.Errorf("规则表达式不能为空")
	}
	_, err := compileMenuRule(*rule)
	return err
}

func normalizeMenuRuleScope(scope string) string {
	switch strings.TrimSpace(scope) {
	case "candidate", "meal", "day", "week":
		return strings.TrimSpace(scope)
	default:
		return "meal"
	}
}

func normalizeMenuRuleKind(kind string) string {
	if strings.TrimSpace(kind) == menuRuleKindScore {
		return menuRuleKindScore
	}
	return menuRuleKindConstraint
}

func normalizeMenuRuleSeverity(severity string) string {
	if strings.TrimSpace(severity) == "soft" {
		return "soft"
	}
	return "hard"
}

func compileMenuRules(rules []models.MenuRule) ([]compiledMenuRule, []string) {
	var compiled []compiledMenuRule
	var warnings []string
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		item, err := compileMenuRule(rule)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("规则 %s 编译失败: %v", rule.Code, err))
			continue
		}
		compiled = append(compiled, item)
	}
	return compiled, warnings
}

func compileMenuRule(rule models.MenuRule) (compiledMenuRule, error) {
	options := []expr.Option{expr.Env(sampleRuleEnv())}
	if normalizeMenuRuleKind(rule.RuleKind) == menuRuleKindConstraint {
		options = append(options, expr.AsBool())
	}
	program, err := expr.Compile(rule.Expression, options...)
	if err != nil {
		return compiledMenuRule{}, err
	}
	if normalizeMenuRuleKind(rule.RuleKind) == menuRuleKindScore {
		out, err := expr.Run(program, sampleRuleEnv())
		if err != nil {
			return compiledMenuRule{}, err
		}
		if _, ok := numericRuleOutput(out); !ok {
			return compiledMenuRule{}, fmt.Errorf("score 规则必须返回数字")
		}
	}
	return compiledMenuRule{rule: rule, program: program}, nil
}

func sampleRuleEnv() map[string]any {
	dish := ruleDishEnv{
		Name:               "样例菜",
		DishRole:           "meat",
		ProteinSources:     []string{"egg"},
		ServingTemperature: "hot",
		CookingMethods:     []string{"stir_fry"},
		Difficulty:         "easy",
		CookTime:           20,
		Tags:               []string{"家常菜"},
	}
	return buildRuleEnv(dish, []ruleDishEnv{}, []ruleDishEnv{}, []ruleDishEnv{}, "balanced", MealQuota{MeatCount: 1})
}

func buildRuleEnv(candidate ruleDishEnv, meal []ruleDishEnv, day []ruleDishEnv, week []ruleDishEnv, profile string, quota MealQuota) map[string]any {
	return map[string]any{
		"candidate": candidate,
		"meal":      meal,
		"day":       day,
		"week":      week,
		"profile":   normalizePlanProfile(profile),
		"quota":     ruleQuotaEnv{MeatCount: quota.MeatCount, VegCount: quota.VegCount, SoupCount: quota.SoupCount},
		"has": func(values []string, value string) bool {
			return containsString(values, value)
		},
		"contains": func(text string, value string) bool {
			return value != "" && strings.Contains(text, value)
		},
		"countMeal": func(field string, value string) int {
			return countRuleDishes(meal, field, value)
		},
		"countDay": func(field string, value string) int {
			return countRuleDishes(day, field, value)
		},
		"countWeek": func(field string, value string) int {
			return countRuleDishes(week, field, value)
		},
		"countOverlapMeal": func(field string, values []string) int {
			return countRuleDishOverlap(meal, field, values)
		},
		"hasOnly": func(values []string, value string) bool {
			return len(values) == 1 && values[0] == value
		},
	}
}

func dishRuleEnv(dish models.Dish) ruleDishEnv {
	dish = ensureDishTraits(dish)
	return ruleDishEnv{
		ID:                 dish.ID,
		Name:               dish.Name,
		Category:           dish.Category,
		MealType:           dish.MealType,
		Taste:              dish.Taste,
		DishRole:           dish.DishRole,
		ProteinSources:     parseJSONStrings(dish.ProteinSources),
		ServingTemperature: dish.ServingTemperature,
		CookingMethods:     parseJSONStrings(dish.CookingMethods),
		SpiceLevel:         dish.SpiceLevel,
		RichnessLevel:      dish.RichnessLevel,
		CarbLevel:          dish.CarbLevel,
		CookTime:           dish.CookTime,
		Difficulty:         dish.Difficulty,
		Favorite:           dish.Favorite,
		Tags:               parseTags(dish.Tags),
	}
}

func dishesRuleEnv(dishes []models.Dish) []ruleDishEnv {
	result := make([]ruleDishEnv, 0, len(dishes))
	for _, dish := range dishes {
		result = append(result, dishRuleEnv(dish))
	}
	return result
}

func parseJSONStrings(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}

func countRuleDishes(dishes []ruleDishEnv, field string, value string) int {
	count := 0
	for _, dish := range dishes {
		if ruleDishFieldMatches(dish, field, value) {
			count++
		}
	}
	return count
}

func countRuleDishOverlap(dishes []ruleDishEnv, field string, values []string) int {
	if len(values) == 0 {
		return 0
	}
	count := 0
	for _, dish := range dishes {
		for _, value := range values {
			if ruleDishFieldMatches(dish, field, value) {
				count++
				break
			}
		}
	}
	return count
}

func ruleDishFieldMatches(dish ruleDishEnv, field string, value string) bool {
	switch field {
	case "protein_sources":
		return containsString(dish.ProteinSources, value)
	case "cooking_methods":
		return containsString(dish.CookingMethods, value)
	case "serving_temperature":
		return dish.ServingTemperature == value
	case "dish_role":
		return dish.DishRole == value
	case "heavy_spicy":
		return value == "true" && dish.SpiceLevel >= 2 && dish.RichnessLevel >= 2
	default:
		return false
	}
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func numericRuleOutput(out any) (float64, bool) {
	switch value := out.(type) {
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case float64:
		return value, true
	case float32:
		return float64(value), true
	default:
		return 0, false
	}
}

func sortMenuRules(rules []models.MenuRule) {
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority < rules[j].Priority
		}
		return rules[i].ID < rules[j].ID
	})
}
