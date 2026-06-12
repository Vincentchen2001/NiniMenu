package services

import (
	"encoding/json"
	"fmt"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"sort"
	"strconv"
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
	CategoryFavorites  int      `expr:"category_favorites"`
	DaysSinceLast      int      `expr:"days_since_last"`
	Tags               []string `expr:"tags"`
	Ingredients        []string `expr:"ingredients"`
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

const (
	menuRulesSeedVersionKey = "menu_rules_seed_version"
	menuRulesSeedVersion    = 4
)

// droppedMenuRuleExpressions lists pre-v2 default rules that no longer ship.
// Migration deletes them only while their expression is still the factory
// string; user-modified copies survive.
var droppedMenuRuleExpressions = map[string]string{
	"quick_profile_bonus":    `profile == "quick" && (candidate.difficulty == "easy" || candidate.cook_time <= 25) ? 24 : 0`,
	"light_profile_bonus":    `profile == "light" && candidate.spice_level == 0 && candidate.richness_level <= 1 ? 22 : 0`,
	"spicy_profile_bonus":    `profile == "spicy" && candidate.spice_level > 0 ? 24 : 0`,
	"favorite_profile_bonus": `profile == "favorite" && candidate.favorite ? 36 : 0`,
	"hot_cold_balance_bonus": `(candidate.serving_temperature == "cold" && countMeal("serving_temperature", "hot") > 0) || (candidate.serving_temperature == "hot" && countMeal("serving_temperature", "cold") > 0) ? 6 : 0`,
}

// legacyMenuRuleExpressions lists earlier factory variants of the kept codes;
// a row still matching one of these gets replaced with the v2 definition.
var legacyMenuRuleExpressions = map[string][]string{
	"avoid_double_egg":  {`!has(candidate.protein_sources, "egg") || countMeal("protein_sources", "egg") == 0`},
	"limit_cold_dishes": {`candidate.serving_temperature != "cold" || countMeal("serving_temperature", "cold") == 0`},
	"limit_staples":     {`candidate.dish_role != "staple" || countMeal("dish_role", "staple") == 0`},
	"avoid_same_primary_protein": {
		`countOverlapMeal("protein_sources", candidate.protein_sources) > 0 && !hasOnly(candidate.protein_sources, "soy") ? -14 : 0`,
		`countOverlapMeal("protein_sources", candidate.protein_sources) == 0 || hasOnly(candidate.protein_sources, "soy")`,
	},
	"avoid_heavy_spicy_day": {`candidate.spice_level >= 2 && candidate.richness_level >= 2 && countDay("heavy_spicy", "true") > 0 ? -18 : 0`},
}

func EnsureDefaultMenuRules() error {
	if database.DB == nil {
		return nil
	}
	if getSettingInt(menuRulesSeedVersionKey, 0) < menuRulesSeedVersion {
		if err := syncDefaultMenuRules(); err != nil {
			return err
		}
		return setSettingValue(menuRulesSeedVersionKey, strconv.Itoa(menuRulesSeedVersion))
	}

	// Seeded already: only repopulate a completely empty table (fresh DB
	// restored with settings); individual user deletions stay deleted.
	var count int64
	if err := database.DB.Model(&models.MenuRule{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	for _, rule := range models.DefaultMenuRules() {
		rule := rule
		if err := normalizeAndValidateMenuRule(&rule); err != nil {
			return err
		}
		if err := database.DB.Create(&rule).Error; err != nil {
			return err
		}
	}
	return nil
}

// syncDefaultMenuRules reconciles the rule table with the current factory
// set: dropped v1 rules still at their factory expression are deleted,
// missing default codes are inserted, and rows whose expression still
// matches a legacy factory variant (or lacks a template backfill) are
// refreshed to the current definition. Rules whose expression was edited
// by the user are never touched; rows refreshed from a legacy factory
// variant keep their enabled state, and rows already at the current
// definition are skipped entirely.
func syncDefaultMenuRules() error {
	for code, factoryExpression := range droppedMenuRuleExpressions {
		var existing models.MenuRule
		err := database.DB.Where("code = ?", code).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			continue
		}
		if err != nil {
			return err
		}
		if strings.TrimSpace(existing.Expression) == factoryExpression {
			if err := database.DB.Delete(&existing).Error; err != nil {
				return err
			}
		}
	}

	for _, rule := range models.DefaultMenuRules() {
		rule := rule
		if err := normalizeAndValidateMenuRule(&rule); err != nil {
			return err
		}
		var existing models.MenuRule
		err := database.DB.Where("code = ?", rule.Code).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			if err := database.DB.Create(&rule).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if isFactoryMenuRuleExpression(rule.Code, existing.Expression) {
			if strings.TrimSpace(existing.Expression) == strings.TrimSpace(rule.Expression) && existing.Template != "" {
				// Already at the current definition: leave the row (and the
				// user's enabled/name/priority tweaks) untouched.
				continue
			}
			rule.ID = existing.ID
			rule.CreatedAt = existing.CreatedAt
			rule.Enabled = existing.Enabled
			if err := database.DB.Save(&rule).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func isFactoryMenuRuleExpression(code string, expression string) bool {
	expression = strings.TrimSpace(expression)
	for _, legacy := range legacyMenuRuleExpressions[code] {
		if expression == legacy {
			return true
		}
	}
	for _, rule := range models.DefaultMenuRules() {
		if rule.Code == code {
			return expression == strings.TrimSpace(rule.Expression)
		}
	}
	return false
}

func setSettingValue(key string, value string) error {
	return database.DB.Model(&models.Setting{}).
		Where("`key` = ?", key).
		Assign(models.Setting{Key: key, Value: value}).
		FirstOrCreate(&models.Setting{}).Error
}

func SaveMenuRules(rules []models.MenuRule) ([]models.MenuRule, error) {
	// Settle the seed migration first so the saved list is the final state
	// instead of having ListMenuRules resurrect defaults afterwards.
	if err := EnsureDefaultMenuRules(); err != nil {
		return nil, err
	}
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
		codes := make([]string, 0, len(rules))
		for _, rule := range rules {
			codes = append(codes, rule.Code)
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
		query := tx
		if len(codes) > 0 {
			query = query.Where("code NOT IN ?", codes)
		}
		if err := query.Delete(&models.MenuRule{}).Error; err != nil {
			return err
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
	if err := ApplyMenuRuleTemplate(rule); err != nil {
		return err
	}
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
		DaysSinceLast:      -1,
		Tags:               []string{"家常菜"},
		Ingredients:        []string{"鸡蛋"},
	}
	return buildRuleEnv(dish, []ruleDishEnv{}, []ruleDishEnv{}, []ruleDishEnv{}, []ruleDishEnv{}, "balanced", MealQuota{MeatCount: 1}, false)
}

func buildRuleEnv(candidate ruleDishEnv, meal []ruleDishEnv, day []ruleDishEnv, week []ruleDishEnv, prevSoups []ruleDishEnv, profile string, quota MealQuota, isWeekend bool) map[string]any {
	return map[string]any{
		"candidate":  candidate,
		"meal":       meal,
		"day":        day,
		"week":       week,
		"prev_soups": prevSoups,
		"profile":    normalizePlanProfile(profile),
		"is_weekend": isWeekend,
		"quota":      ruleQuotaEnv{MeatCount: quota.MeatCount, VegCount: quota.VegCount, SoupCount: quota.SoupCount},
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
		"countOverlapDay": func(field string, values []string) int {
			return countRuleDishOverlap(day, field, values)
		},
		"countOverlapWeek": func(field string, values []string) int {
			return countRuleDishOverlap(week, field, values)
		},
		// Filtering the candidate side alone is complete: ruleDishFieldMatches
		// uses exact equality, so an unfiltered aromatic in prevSoups can never
		// match a candidate value that the filter removed.
		"countOverlapPrevSoup": func(field string, values []string) int {
			return countRuleDishOverlap(prevSoups, field, filterSoupAromatics(values))
		},
		"hasOnly": func(values []string, value string) bool {
			return len(values) == 1 && values[0] == value
		},
	}
}

func dishRuleEnv(dish models.Dish, categoryFavorites map[string]int) ruleDishEnv {
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
		CategoryFavorites:  categoryFavorites[dish.Category],
		DaysSinceLast:      -1,
		Tags:               parseTags(dish.Tags),
		Ingredients:        dishIngredientNames(dish.Ingredients),
	}
}

func dishesRuleEnv(dishes []models.Dish, categoryFavorites map[string]int) []ruleDishEnv {
	result := make([]ruleDishEnv, 0, len(dishes))
	for _, dish := range dishes {
		result = append(result, dishRuleEnv(dish, categoryFavorites))
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
	case "ingredients":
		return containsString(dish.Ingredients, value)
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
