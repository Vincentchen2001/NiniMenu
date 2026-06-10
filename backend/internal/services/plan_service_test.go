package services

import (
	"encoding/json"
	"fmt"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupPlanServiceTestDB(t *testing.T) {
	t.Helper()

	originalDB := database.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.Dish{}, &models.Setting{}, &models.MealRecord{}, &models.DishRecommendation{}, &models.MenuRule{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	database.DB = db
	t.Cleanup(func() {
		database.DB = originalDB
	})
}

func hasJSONValue(raw string, want string) bool {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return false
	}
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func countTraitValue(dishes []models.Dish, field string, value string) int {
	count := 0
	for _, dish := range dishes {
		switch field {
		case "protein_sources":
			if hasJSONValue(dish.ProteinSources, value) {
				count++
			}
		case "cooking_methods":
			if hasJSONValue(dish.CookingMethods, value) {
				count++
			}
		case "serving_temperature":
			if dish.ServingTemperature == value {
				count++
			}
		case "dish_role":
			if dish.DishRole == value {
				count++
			}
		}
	}
	return count
}

func saveWeekPlanPreferenceForTest(t *testing.T, prefs WeekPlanPreferences) {
	t.Helper()
	if err := SaveWeekPlanPreferences(prefs); err != nil {
		t.Fatalf("SaveWeekPlanPreferences() error = %v", err)
	}
}

func createDishForPlanTest(t *testing.T, name string, tags string, ingredients string) models.Dish {
	t.Helper()
	dish := models.Dish{
		Name:        name,
		MealType:    "all",
		Tags:        tags,
		Ingredients: ingredients,
		Enabled:     true,
	}
	if err := database.DB.Create(&dish).Error; err != nil {
		t.Fatalf("create dish %s: %v", name, err)
	}
	return dish
}

// countDishKinds counts slot kinds with the same strict-role view the planner
// uses to fill quotas (matchesWeekPlanSlot with strictRole=true).
func countDishKinds(dishes []models.Dish) (meat int, veg int, soup int) {
	for _, dish := range dishes {
		dish = ensureDishTraits(dish)
		switch {
		case matchesWeekPlanSlot(dish, weekPlanSlotSoup, true):
			soup++
		case matchesWeekPlanSlot(dish, weekPlanSlotMeat, true):
			meat++
		case matchesWeekPlanSlot(dish, weekPlanSlotVeg, true):
			veg++
		}
	}
	return meat, veg, soup
}

func TestGenerateWeekPlanUsesWeekdayWeekendMeatVegSoupPreferences(t *testing.T) {
	setupPlanServiceTestDB(t)

	saveWeekPlanPreferenceForTest(t, WeekPlanPreferences{
		Weekday: WeekPlanPeriodPreferences{
			Profile: "balanced",
			Lunch:   MealQuota{MeatCount: 1, VegCount: 1, SoupCount: 1},
			Dinner:  MealQuota{MeatCount: 1, VegCount: 0, SoupCount: 1},
		},
		Weekend: WeekPlanPeriodPreferences{
			Profile: "favorite",
			Lunch:   MealQuota{MeatCount: 0, VegCount: 0},
			Dinner:  MealQuota{MeatCount: 0, VegCount: 2, SoupCount: 1},
		},
	})

	for i := 1; i <= 20; i++ {
		createDishForPlanTest(t, fmt.Sprintf("青菜%d", i), `["素菜"]`, `[{"name":"青菜","amount":"1把"}]`)
		createDishForPlanTest(t, fmt.Sprintf("鸡肉%d", i), `["家常菜"]`, `[{"name":"鸡肉","amount":"100g"}]`)
		createDishForPlanTest(t, fmt.Sprintf("菌菇汤%d", i), `["汤品"]`, `[{"name":"香菇","amount":"50g"},{"name":"金针菇","amount":"50g"}]`)
	}

	plan, err := GenerateWeekPlan()
	if err != nil {
		t.Fatalf("GenerateWeekPlan() error = %v", err)
	}
	if len(plan.Days) != 7 {
		t.Fatalf("plan day count = %d, want 7", len(plan.Days))
	}
	for i, day := range plan.Days {
		lunchMeat, lunchVeg, lunchSoup := countDishKinds(day.Lunch)
		dinnerMeat, dinnerVeg, dinnerSoup := countDishKinds(day.Dinner)
		if i < 5 {
			if lunchMeat != 1 || lunchVeg != 1 || lunchSoup != 1 {
				t.Fatalf("%s lunch meat/veg/soup = %d/%d/%d, want 1/1/1", day.DayName, lunchMeat, lunchVeg, lunchSoup)
			}
			if dinnerMeat != 1 || dinnerVeg != 0 || dinnerSoup != 1 {
				t.Fatalf("%s dinner meat/veg/soup = %d/%d/%d, want 1/0/1", day.DayName, dinnerMeat, dinnerVeg, dinnerSoup)
			}
			continue
		}
		if len(day.Lunch) != 0 {
			t.Fatalf("%s lunch count = %d, want skipped", day.DayName, len(day.Lunch))
		}
		if dinnerMeat != 0 || dinnerVeg != 2 || dinnerSoup != 1 {
			t.Fatalf("%s dinner meat/veg/soup = %d/%d/%d, want 0/2/1", day.DayName, dinnerMeat, dinnerVeg, dinnerSoup)
		}
	}
}

func TestSaveWeekPlanRecordsAndReplacesRecommendations(t *testing.T) {
	setupPlanServiceTestDB(t)

	first := createDishForPlanTest(t, "第一道菜", `["素菜"]`, `[{"name":"西兰花","amount":"1棵"}]`)
	second := createDishForPlanTest(t, "第二道菜", `["家常菜"]`, `[{"name":"牛肉","amount":"100g"}]`)

	plan := &WeekPlan{Days: []WeekDayPlan{{
		Date:    "2099-01-05",
		DayName: "周一",
		Lunch:   []models.Dish{first},
		Dinner:  []models.Dish{},
	}}}
	if err := SaveWeekPlan(plan); err != nil {
		t.Fatalf("SaveWeekPlan(first) error = %v", err)
	}

	plan.Days[0].Lunch = []models.Dish{second}
	if err := SaveWeekPlan(plan); err != nil {
		t.Fatalf("SaveWeekPlan(second) error = %v", err)
	}

	var records []models.DishRecommendation
	if err := database.DB.Order("dish_id ASC").Find(&records).Error; err != nil {
		t.Fatalf("list recommendations: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("recommendation count = %d, want 1: %+v", len(records), records)
	}
	if records[0].DishID != second.ID {
		t.Fatalf("recommendation dish_id = %d, want %d", records[0].DishID, second.ID)
	}
	if records[0].Source != recommendationSourceWeekPlan || records[0].MealType != "lunch" || records[0].PlannedDate != "2099-01-05" {
		t.Fatalf("recommendation metadata = %+v", records[0])
	}
}

func TestRecentDishIDMapUsesRecommendationHistoryAndSetting(t *testing.T) {
	setupPlanServiceTestDB(t)

	if err := database.DB.Create(&models.Setting{Key: "repeat_days", Value: "7"}).Error; err != nil {
		t.Fatalf("create repeat_days setting: %v", err)
	}
	dish := createDishForPlanTest(t, "最近推荐", `["素菜"]`, `[{"name":"豆腐","amount":"1块"}]`)
	if err := database.DB.Create(&models.DishRecommendation{
		DishID:      dish.ID,
		Source:      recommendationSourceWeekPlan,
		MealType:    "dinner",
		PlannedDate: time.Now().Format("2006-01-02"),
	}).Error; err != nil {
		t.Fatalf("create recommendation: %v", err)
	}

	if got := RecommendationCooldownDays(); got != 7 {
		t.Fatalf("RecommendationCooldownDays() = %d, want 7", got)
	}
	recent := recentDishIDMap(RecommendationCooldownDays())
	if !recent[dish.ID] {
		t.Fatalf("recentDishIDMap() should include dish recommendation id %d", dish.ID)
	}
}

func TestRecentDishIDMapIncludesMealRecordsAndRecommendations(t *testing.T) {
	setupPlanServiceTestDB(t)

	eaten := createDishForPlanTest(t, "最近吃过", `["家常菜"]`, `[{"name":"猪肉","amount":"100g"}]`)
	recommended := createDishForPlanTest(t, "最近推荐", `["素菜"]`, `[{"name":"豆腐","amount":"1块"}]`)

	today := time.Now().Format("2006-01-02")
	if err := database.DB.Create(&models.MealRecord{DishID: eaten.ID, MealDate: today}).Error; err != nil {
		t.Fatalf("create meal record: %v", err)
	}
	if err := database.DB.Create(&models.DishRecommendation{
		DishID:      recommended.ID,
		Source:      recommendationSourceWeekPlan,
		MealType:    "dinner",
		PlannedDate: today,
	}).Error; err != nil {
		t.Fatalf("create recommendation: %v", err)
	}

	// A recommendation planned for a future day (e.g. the tail of the
	// current week plan) must NOT count as "recently eaten" — otherwise
	// regenerating the plan excludes everything the previous plan picked.
	futurePlanned := createDishForPlanTest(t, "后天才排", `["素菜"]`, `[{"name":"青菜","amount":"1把"}]`)
	if err := database.DB.Create(&models.DishRecommendation{
		DishID:      futurePlanned.ID,
		Source:      recommendationSourceWeekPlan,
		MealType:    "dinner",
		PlannedDate: time.Now().AddDate(0, 0, 2).Format("2006-01-02"),
	}).Error; err != nil {
		t.Fatalf("create future recommendation: %v", err)
	}

	recent := recentDishIDMap(3)
	if !recent[recommended.ID] {
		t.Fatalf("recentDishIDMap() should include recommended dish id %d", recommended.ID)
	}
	if !recent[eaten.ID] {
		t.Fatalf("recentDishIDMap() should include recently eaten dish id %d", eaten.ID)
	}
	if recent[futurePlanned.ID] {
		t.Fatalf("recentDishIDMap() should ignore future-dated recommendation %d", futurePlanned.ID)
	}
}

func TestIsSoupDish(t *testing.T) {
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
			name: "soup tag",
			dish: models.Dish{Name: "菌菇煲", Tags: `["汤品"]`, Ingredients: `[{"name":"香菇","amount":"50g"}]`},
			want: true,
		},
		{
			name: "name keyword",
			dish: models.Dish{Name: "紫菜蛋花汤", Tags: `[]`, Ingredients: `[{"name":"紫菜","amount":"适量"},{"name":"鸡蛋","amount":"1个"}]`},
			want: true,
		},
		{
			name: "non soup egg dish",
			dish: models.Dish{Name: "番茄炒蛋", Tags: `[]`, Ingredients: `[{"name":"番茄","amount":"2个"},{"name":"鸡蛋","amount":"2个"}]`},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSoupDish(tt.dish); got != tt.want {
				t.Fatalf("isSoupDish() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInferDishTraitsForMenuRules(t *testing.T) {
	tests := []struct {
		name              string
		dish              models.Dish
		wantRole          string
		wantProtein       string
		wantTemperature   string
		wantCookingMethod string
		wantMinimumCarb   int
	}{
		{
			name:              "egg dish",
			dish:              models.Dish{Name: "番茄炒蛋", Category: "家常菜", Tags: `["素菜"]`, Ingredients: `[{"name":"鸡蛋","amount":"2个"},{"name":"番茄","amount":"2个"}]`, Steps: `[{"text":"热油炒蛋，再下番茄翻炒"}]`},
			wantRole:          "meat",
			wantProtein:       "egg",
			wantTemperature:   "hot",
			wantCookingMethod: "stir_fry",
		},
		{
			name:              "cold carb dish",
			dish:              models.Dish{Name: "凉拌米线", Category: "云南菜", Taste: "酸辣", Tags: `["午餐","凉菜"]`, Ingredients: `[{"name":"米线","amount":"350克"},{"name":"黄瓜","amount":"120克"}]`, Steps: `[{"text":"米线烫熟过凉，和配菜拌匀"}]`},
			wantRole:          "staple",
			wantTemperature:   "cold",
			wantCookingMethod: "cold_mix",
			wantMinimumCarb:   2,
		},
		{
			name:              "egg soup",
			dish:              models.Dish{Name: "紫菜蛋花汤", Category: "汤品", Tags: `["汤品"]`, Ingredients: `[{"name":"紫菜","amount":"适量"},{"name":"鸡蛋","amount":"1个"}]`, Steps: `[{"text":"汤煮开后淋入蛋液"}]`},
			wantRole:          "soup",
			wantProtein:       "egg",
			wantTemperature:   "hot",
			wantCookingMethod: "simmer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferDishTraits(tt.dish)
			if got.DishRole != tt.wantRole {
				t.Fatalf("DishRole = %q, want %q", got.DishRole, tt.wantRole)
			}
			if tt.wantProtein != "" && !hasJSONValue(got.ProteinSources, tt.wantProtein) {
				t.Fatalf("ProteinSources = %s, want %q", got.ProteinSources, tt.wantProtein)
			}
			if got.ServingTemperature != tt.wantTemperature {
				t.Fatalf("ServingTemperature = %q, want %q", got.ServingTemperature, tt.wantTemperature)
			}
			if tt.wantCookingMethod != "" && !hasJSONValue(got.CookingMethods, tt.wantCookingMethod) {
				t.Fatalf("CookingMethods = %s, want %q", got.CookingMethods, tt.wantCookingMethod)
			}
			if got.CarbLevel < tt.wantMinimumCarb {
				t.Fatalf("CarbLevel = %d, want >= %d", got.CarbLevel, tt.wantMinimumCarb)
			}
			if got.TraitSource != "auto" || got.TraitVersion == 0 {
				t.Fatalf("trait metadata = %q/%d, want auto with non-zero version", got.TraitSource, got.TraitVersion)
			}
		})
	}
}

func TestValidateMenuRuleExpression(t *testing.T) {
	valid := models.MenuRule{
		Code:       "avoid_double_egg",
		RuleKind:   "constraint",
		Expression: `!has(candidate.protein_sources, "egg") || countMeal("protein_sources", "egg") == 0`,
	}
	if err := ValidateMenuRuleExpression(valid); err != nil {
		t.Fatalf("ValidateMenuRuleExpression(valid) error = %v", err)
	}

	badVariable := valid
	badVariable.Expression = `unknown.value == 1`
	if err := ValidateMenuRuleExpression(badVariable); err == nil {
		t.Fatalf("ValidateMenuRuleExpression() should reject unknown variables")
	}

	badReturn := valid
	badReturn.Expression = `42`
	if err := ValidateMenuRuleExpression(badReturn); err == nil {
		t.Fatalf("ValidateMenuRuleExpression() should reject non-bool constraint rules")
	}

	scoreRule := models.MenuRule{
		Code:       "quick_bonus",
		RuleKind:   "score",
		Expression: `candidate.cook_time <= 20 ? 10 : 0`,
	}
	if err := ValidateMenuRuleExpression(scoreRule); err != nil {
		t.Fatalf("ValidateMenuRuleExpression(score) error = %v", err)
	}
}

func TestEvaluateConstraintRulesRelaxableSemantics(t *testing.T) {
	dish := models.Dish{Name: "测试菜"}
	quota := MealQuota{MeatCount: 1}

	tests := []struct {
		name        string
		severity    string
		relaxable   bool
		enforceSoft bool
		wantAllowed bool
	}{
		{name: "hard non-relaxable blocks in strict stage", severity: "hard", relaxable: false, enforceSoft: true, wantAllowed: false},
		{name: "hard non-relaxable blocks in relaxed stage", severity: "hard", relaxable: false, enforceSoft: false, wantAllowed: false},
		{name: "soft non-relaxable blocks in strict stage", severity: "soft", relaxable: false, enforceSoft: true, wantAllowed: false},
		{name: "soft non-relaxable blocks in relaxed stage", severity: "soft", relaxable: false, enforceSoft: false, wantAllowed: false},
		{name: "hard relaxable blocks in strict stage", severity: "hard", relaxable: true, enforceSoft: true, wantAllowed: false},
		{name: "hard relaxable passes in relaxed stage", severity: "hard", relaxable: true, enforceSoft: false, wantAllowed: true},
		{name: "soft relaxable blocks in strict stage", severity: "soft", relaxable: true, enforceSoft: true, wantAllowed: false},
		{name: "soft relaxable passes in relaxed stage", severity: "soft", relaxable: true, enforceSoft: false, wantAllowed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := models.MenuRule{
				Code:       "always_violated",
				Name:       "恒不满足的约束",
				Enabled:    true,
				Scope:      "meal",
				RuleKind:   "constraint",
				Severity:   tt.severity,
				Relaxable:  tt.relaxable,
				Expression: `false`,
				Message:    "测试约束拦截",
			}
			compiled, err := compileMenuRule(rule)
			if err != nil {
				t.Fatalf("compile rule: %v", err)
			}
			allowed, _ := evaluateConstraintRules(dish, "balanced", quota, nil, nil, nil, []compiledMenuRule{compiled}, tt.enforceSoft)
			if allowed != tt.wantAllowed {
				t.Fatalf("evaluateConstraintRules(severity=%s relaxable=%v enforceSoft=%v) allowed = %v, want %v", tt.severity, tt.relaxable, tt.enforceSoft, allowed, tt.wantAllowed)
			}
		})
	}
}

func TestDefaultSameProteinRuleIsSoftScore(t *testing.T) {
	var sameProtein models.MenuRule
	for _, rule := range models.DefaultMenuRules() {
		if rule.Code == "avoid_same_primary_protein" {
			sameProtein = rule
			break
		}
	}
	if sameProtein.Code == "" {
		t.Fatalf("DefaultMenuRules() missing avoid_same_primary_protein")
	}
	if sameProtein.RuleKind != "score" || sameProtein.Severity != "soft" || !sameProtein.Relaxable {
		t.Fatalf("avoid_same_primary_protein = kind %q severity %q relaxable %v, want soft score relaxable", sameProtein.RuleKind, sameProtein.Severity, sameProtein.Relaxable)
	}
	if err := ValidateMenuRuleExpression(sameProtein); err != nil {
		t.Fatalf("ValidateMenuRuleExpression(avoid_same_primary_protein) error = %v", err)
	}
}

func TestEnsureDefaultMenuRulesUpgradesLegacySameProteinRule(t *testing.T) {
	setupPlanServiceTestDB(t)

	legacy := models.MenuRule{
		Code:        "avoid_same_primary_protein",
		Name:        "同餐主蛋白不重复",
		Enabled:     true,
		Scope:       "meal",
		RuleKind:    "constraint",
		Severity:    "hard",
		Relaxable:   false,
		Expression:  `countOverlapMeal("protein_sources", candidate.protein_sources) == 0 || hasOnly(candidate.protein_sources, "soy")`,
		Priority:    40,
		Message:     "同餐主蛋白来源重复",
	}
	if err := database.DB.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy rule: %v", err)
	}

	if err := EnsureDefaultMenuRules(); err != nil {
		t.Fatalf("EnsureDefaultMenuRules() error = %v", err)
	}

	var got models.MenuRule
	if err := database.DB.Where("code = ?", "avoid_same_primary_protein").First(&got).Error; err != nil {
		t.Fatalf("load upgraded rule: %v", err)
	}
	if got.RuleKind != "score" || got.Severity != "soft" || !got.Relaxable {
		t.Fatalf("upgraded rule = kind %q severity %q relaxable %v, want soft score relaxable", got.RuleKind, got.Severity, got.Relaxable)
	}
}

func TestSaveMenuRulesDeletesRemovedRules(t *testing.T) {
	setupPlanServiceTestDB(t)

	kept := models.MenuRule{
		Code:       "kept_rule",
		Name:       "保留规则",
		Enabled:    true,
		Scope:      "meal",
		RuleKind:   "score",
		Severity:   "soft",
		Relaxable:  true,
		Expression: `0`,
		Priority:   1,
	}
	removed := models.MenuRule{
		Code:       "removed_rule",
		Name:       "移除规则",
		Enabled:    true,
		Scope:      "meal",
		RuleKind:   "score",
		Severity:   "soft",
		Relaxable:  true,
		Expression: `0`,
		Priority:   2,
	}
	if err := database.DB.Create(&kept).Error; err != nil {
		t.Fatalf("create kept rule: %v", err)
	}
	if err := database.DB.Create(&removed).Error; err != nil {
		t.Fatalf("create removed rule: %v", err)
	}

	rules, err := SaveMenuRules([]models.MenuRule{kept})
	if err != nil {
		t.Fatalf("SaveMenuRules() error = %v", err)
	}
	if len(rules) != 1 || rules[0].Code != "kept_rule" {
		t.Fatalf("rules after save = %+v, want only kept_rule", rules)
	}

	var count int64
	if err := database.DB.Model(&models.MenuRule{}).Where("code = ?", "removed_rule").Count(&count).Error; err != nil {
		t.Fatalf("count removed rule: %v", err)
	}
	if count != 0 {
		t.Fatalf("removed_rule count = %d, want 0", count)
	}
}

func TestGenerateWeekPlanHonorsHardMenuRules(t *testing.T) {
	setupPlanServiceTestDB(t)

	saveWeekPlanPreferenceForTest(t, WeekPlanPreferences{
		Weekday: WeekPlanPeriodPreferences{
			Profile: "balanced",
			Lunch:   MealQuota{MeatCount: 2, VegCount: 2, SoupCount: 0},
			Dinner:  MealQuota{MeatCount: 2, VegCount: 2, SoupCount: 0},
		},
		Weekend: WeekPlanPeriodPreferences{
			Profile: "balanced",
			Lunch:   MealQuota{MeatCount: 2, VegCount: 2, SoupCount: 0},
			Dinner:  MealQuota{MeatCount: 2, VegCount: 2, SoupCount: 0},
		},
	})

	for i := 1; i <= 20; i++ {
		createDishForPlanTest(t, fmt.Sprintf("番茄炒蛋%d", i), `["家常菜"]`, `[{"name":"鸡蛋","amount":"2个"},{"name":"番茄","amount":"2个"}]`)
		createDishForPlanTest(t, fmt.Sprintf("青椒肉丝%d", i), `["家常菜"]`, `[{"name":"猪肉","amount":"120克"},{"name":"青椒","amount":"1个"}]`)
		createDishForPlanTest(t, fmt.Sprintf("凉拌黄瓜%d", i), `["凉菜","素菜"]`, `[{"name":"黄瓜","amount":"1根"}]`)
		createDishForPlanTest(t, fmt.Sprintf("清炒青菜%d", i), `["素菜"]`, `[{"name":"青菜","amount":"1把"}]`)
		createDishForPlanTest(t, fmt.Sprintf("凉拌米线%d", i), `["凉菜","午餐"]`, `[{"name":"米线","amount":"250克"}]`)
	}

	plan, err := GenerateWeekPlan()
	if err != nil {
		t.Fatalf("GenerateWeekPlan() error = %v", err)
	}
	for _, day := range plan.Days {
		for mealName, dishes := range map[string][]models.Dish{"lunch": day.Lunch, "dinner": day.Dinner} {
			if eggs := countTraitValue(dishes, "protein_sources", "egg"); eggs > 1 {
				t.Fatalf("%s %s egg dishes = %d, want <= 1: %+v", day.DayName, mealName, eggs, dishes)
			}
			if cold := countTraitValue(dishes, "serving_temperature", "cold"); cold > 1 {
				t.Fatalf("%s %s cold dishes = %d, want <= 1: %+v", day.DayName, mealName, cold, dishes)
			}
			if staples := countTraitValue(dishes, "dish_role", "staple"); staples > 1 {
				t.Fatalf("%s %s staple dishes = %d, want <= 1: %+v", day.DayName, mealName, staples, dishes)
			}
		}
	}
}

func TestGenerateWeekPlanReturnsWarningsInsteadOfBreakingHardRules(t *testing.T) {
	setupPlanServiceTestDB(t)

	saveWeekPlanPreferenceForTest(t, WeekPlanPreferences{
		Weekday: WeekPlanPeriodPreferences{
			Profile: "balanced",
			Lunch:   MealQuota{MeatCount: 2},
			Dinner:  MealQuota{MeatCount: 0},
		},
		Weekend: WeekPlanPeriodPreferences{
			Profile: "balanced",
			Lunch:   MealQuota{MeatCount: 2},
			Dinner:  MealQuota{MeatCount: 0},
		},
	})
	createDishForPlanTest(t, "番茄炒蛋", `["家常菜"]`, `[{"name":"鸡蛋","amount":"2个"},{"name":"番茄","amount":"2个"}]`)
	createDishForPlanTest(t, "金钱蛋", `["家常菜"]`, `[{"name":"鸡蛋","amount":"5个"}]`)

	plan, err := GenerateWeekPlan()
	if err != nil {
		t.Fatalf("GenerateWeekPlan() error = %v", err)
	}
	if len(plan.Warnings) == 0 {
		t.Fatalf("GenerateWeekPlan() should return warnings when hard rules prevent filling quota")
	}
	for _, day := range plan.Days {
		if eggs := countTraitValue(day.Lunch, "protein_sources", "egg"); eggs > 1 {
			t.Fatalf("%s lunch egg dishes = %d, want <= 1", day.DayName, eggs)
		}
	}
}

func TestGenerateWeekPlanAllowsSkippingLunch(t *testing.T) {
	setupPlanServiceTestDB(t)

	if err := database.DB.Create(&models.Setting{Key: "lunch_dishes_per_day", Value: "0"}).Error; err != nil {
		t.Fatalf("create lunch setting: %v", err)
	}
	if err := database.DB.Create(&models.Setting{Key: "dinner_dishes_per_day", Value: "1"}).Error; err != nil {
		t.Fatalf("create dinner setting: %v", err)
	}
	for i := 1; i <= 3; i++ {
		dish := models.Dish{
			Name:     fmt.Sprintf("测试菜%d", i),
			MealType: "all",
			Enabled:  true,
		}
		if err := database.DB.Create(&dish).Error; err != nil {
			t.Fatalf("create dish %d: %v", i, err)
		}
	}

	plan, err := GenerateWeekPlan()
	if err != nil {
		t.Fatalf("GenerateWeekPlan() error = %v", err)
	}
	if len(plan.Days) != 7 {
		t.Fatalf("plan day count = %d, want 7", len(plan.Days))
	}
	for _, day := range plan.Days {
		if day.Lunch == nil {
			t.Fatalf("%s lunch should be an empty slice, not nil", day.DayName)
		}
		if len(day.Lunch) != 0 {
			t.Fatalf("%s lunch count = %d, want 0", day.DayName, len(day.Lunch))
		}
		if len(day.Dinner) != 1 {
			t.Fatalf("%s dinner count = %d, want 1", day.DayName, len(day.Dinner))
		}
	}
}

func TestGenerateWeekPlanAppliesDayProfileOverride(t *testing.T) {
	setupPlanServiceTestDB(t)

	saveWeekPlanPreferenceForTest(t, WeekPlanPreferences{
		Weekday: WeekPlanPeriodPreferences{Profile: "balanced", Lunch: MealQuota{MeatCount: 1}},
		Weekend: WeekPlanPeriodPreferences{Profile: "balanced", Lunch: MealQuota{MeatCount: 1}},
		Days:    map[string]DayOverride{"wed": {Profile: "spicy"}},
	})
	for i := 1; i <= 20; i++ {
		mild := models.Dish{
			Name:        fmt.Sprintf("白切鸡%d", i),
			MealType:    "all",
			Tags:        `["家常菜"]`,
			Ingredients: `[{"name":"鸡肉","amount":"200g"}]`,
			Favorite:    true,
			Enabled:     true,
		}
		if err := database.DB.Create(&mild).Error; err != nil {
			t.Fatalf("create mild dish: %v", err)
		}
		spicy := models.Dish{
			Name:        fmt.Sprintf("麻辣牛肉%d", i),
			MealType:    "all",
			Taste:       "麻辣",
			Tags:        `["家常菜"]`,
			Ingredients: `[{"name":"牛肉","amount":"200g"}]`,
			Enabled:     true,
		}
		if err := database.DB.Create(&spicy).Error; err != nil {
			t.Fatalf("create spicy dish: %v", err)
		}
	}

	plan, err := GenerateWeekPlan()
	if err != nil {
		t.Fatalf("GenerateWeekPlan() error = %v", err)
	}
	if len(plan.Days) != 7 {
		t.Fatalf("day count = %d", len(plan.Days))
	}
	monday := plan.Days[0].Lunch
	wednesday := plan.Days[2].Lunch
	if len(monday) != 1 || len(wednesday) != 1 {
		t.Fatalf("lunch counts = %d/%d, want 1/1", len(monday), len(wednesday))
	}
	if !strings.Contains(wednesday[0].Name, "麻辣") {
		t.Fatalf("wednesday should pick spicy dish, got %s", wednesday[0].Name)
	}
	if strings.Contains(monday[0].Name, "麻辣") {
		t.Fatalf("monday should keep favorite mild dish, got %s", monday[0].Name)
	}
}

func TestGenerateWeekPlanAppliesWantBonus(t *testing.T) {
	setupPlanServiceTestDB(t)

	saveWeekPlanPreferenceForTest(t, WeekPlanPreferences{
		Weekday: WeekPlanPeriodPreferences{Profile: "balanced", Lunch: MealQuota{MeatCount: 1}},
		Weekend: WeekPlanPeriodPreferences{Profile: "balanced", Lunch: MealQuota{MeatCount: 1}},
		Days:    map[string]DayOverride{"mon": {Want: []string{"poultry"}}},
	})
	for i := 1; i <= 20; i++ {
		pork := models.Dish{
			Name:        fmt.Sprintf("红烧猪蹄%d", i),
			MealType:    "all",
			Tags:        `["家常菜"]`,
			Ingredients: `[{"name":"猪蹄","amount":"300g"}]`,
			Favorite:    true,
			Enabled:     true,
		}
		if err := database.DB.Create(&pork).Error; err != nil {
			t.Fatalf("create pork dish: %v", err)
		}
		chicken := models.Dish{
			Name:        fmt.Sprintf("香煎鸡腿%d", i),
			MealType:    "all",
			Tags:        `["家常菜"]`,
			Ingredients: `[{"name":"鸡腿","amount":"2个"}]`,
			Enabled:     true,
		}
		if err := database.DB.Create(&chicken).Error; err != nil {
			t.Fatalf("create chicken dish: %v", err)
		}
	}

	plan, err := GenerateWeekPlan()
	if err != nil {
		t.Fatalf("GenerateWeekPlan() error = %v", err)
	}
	monday := plan.Days[0].Lunch
	tuesday := plan.Days[1].Lunch
	if len(monday) != 1 || len(tuesday) != 1 {
		t.Fatalf("lunch counts = %d/%d, want 1/1", len(monday), len(tuesday))
	}
	if !strings.Contains(monday[0].Name, "鸡腿") {
		t.Fatalf("monday should pick craved poultry dish, got %s", monday[0].Name)
	}
	if !strings.Contains(tuesday[0].Name, "猪蹄") {
		t.Fatalf("tuesday should pick favorite pork dish, got %s", tuesday[0].Name)
	}
}

func TestGenerateWeekPlanSoupDayAddsTempSoupSlot(t *testing.T) {
	setupPlanServiceTestDB(t)

	saveWeekPlanPreferenceForTest(t, WeekPlanPreferences{
		Weekday: WeekPlanPeriodPreferences{Profile: "balanced", Dinner: MealQuota{MeatCount: 1}},
		Weekend: WeekPlanPeriodPreferences{Profile: "balanced", Dinner: MealQuota{MeatCount: 1}},
		Days:    map[string]DayOverride{"sat": {Profile: "soup"}},
	})
	for i := 1; i <= 10; i++ {
		createDishForPlanTest(t, fmt.Sprintf("青椒肉丝%d", i), `["家常菜"]`, `[{"name":"猪肉","amount":"100g"}]`)
		createDishForPlanTest(t, fmt.Sprintf("菌菇汤%d", i), `["汤品"]`, `[{"name":"香菇","amount":"50g"}]`)
	}

	plan, err := GenerateWeekPlan()
	if err != nil {
		t.Fatalf("GenerateWeekPlan() error = %v", err)
	}
	for i, day := range plan.Days {
		_, _, soup := countDishKinds(day.Dinner)
		if i == 5 {
			if soup != 1 {
				t.Fatalf("saturday dinner soup = %d, want temp slot 1: %+v", soup, day.Dinner)
			}
			continue
		}
		if soup != 0 {
			t.Fatalf("%s dinner soup = %d, want 0", day.DayName, soup)
		}
	}
}

func TestInvalidateWeekPlanCacheClearsStoredAndInMemoryCache(t *testing.T) {
	setupPlanServiceTestDB(t)

	cachedPlan = &WeekPlan{Days: []WeekDayPlan{{Date: "2099-01-01", DayName: "周四"}}}
	cachedWeekKey = "2099-01-01"
	if err := database.DB.Create(&models.Setting{Key: "week_plan_cache", Value: `{"days":[]}`}).Error; err != nil {
		t.Fatalf("create cache setting: %v", err)
	}

	InvalidateWeekPlanCache()

	if cachedPlan != nil {
		t.Fatalf("cachedPlan should be nil after invalidation")
	}
	if cachedWeekKey != "" {
		t.Fatalf("cachedWeekKey = %q, want empty", cachedWeekKey)
	}
	var count int64
	database.DB.Model(&models.Setting{}).Where("`key` = ?", "week_plan_cache").Count(&count)
	if count != 0 {
		t.Fatalf("stored week_plan_cache count = %d, want 0", count)
	}
}

func TestCompactShoppingAmounts(t *testing.T) {
	tests := []struct {
		name    string
		amounts []string
		want    string
	}{
		{name: "same unit sums", amounts: []string{"1个", "2个"}, want: "3个"},
		{name: "different units stay separate", amounts: []string{"1个", "4颗"}, want: "1个+4颗"},
		{name: "different unit order is stable", amounts: []string{"4颗", "1个"}, want: "1个+4颗"},
		{name: "text amounts dedupe", amounts: []string{"适量", "适量", "适量"}, want: "适量"},
		{name: "mixed text and numeric", amounts: []string{"适量", "1块", "适量"}, want: "适量+1块"},
		{name: "existing plus values compact", amounts: []string{"3个+4个"}, want: "7个"},
		{name: "no unit numeric sums", amounts: []string{"1", "2"}, want: "3"},
		{name: "decimal values sum", amounts: []string{"0.5斤", "1斤"}, want: "1.5斤"},
		{name: "empty defaults to suitable amount", amounts: []string{"", "  "}, want: "适量"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compactShoppingAmounts(tt.amounts); got != tt.want {
				t.Fatalf("compactShoppingAmounts(%v) = %q, want %q", tt.amounts, got, tt.want)
			}
		})
	}
}

func TestSortShoppingItems(t *testing.T) {
	items := []ShoppingItem{
		{Name: "tomato", Amount: "2个"},
		{Name: "egg", Amount: "3个", Checked: true},
		{Name: "egg", Amount: "2个"},
		{Name: "apple", Amount: "1个", InStock: true},
	}

	sortShoppingItems(items)

	got := []string{
		items[0].Name + ":" + items[0].Amount,
		items[1].Name + ":" + items[1].Amount,
		items[2].Name + ":" + items[2].Amount,
		items[3].Name + ":" + items[3].Amount,
	}
	want := []string{"egg:2个", "tomato:2个", "egg:3个", "apple:1个"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted[%d] = %q, want %q; full order: %v", i, got[i], want[i], got)
		}
	}
}

func TestShoppingItemPriority(t *testing.T) {
	tests := []struct {
		item ShoppingItem
		want int
	}{
		{item: ShoppingItem{Name: "盐"}, want: 0},
		{item: ShoppingItem{Name: "盐", Checked: true}, want: 1},
		{item: ShoppingItem{Name: "盐", InStock: true}, want: 2},
		{item: ShoppingItem{Name: "盐", Checked: true, InStock: true}, want: 2},
	}

	for _, tt := range tests {
		if got := shoppingItemPriority(tt.item); got != tt.want {
			t.Fatalf("shoppingItemPriority(%+v) = %d, want %d", tt.item, got, tt.want)
		}
	}
}

func TestBuildShoppingCategoryExact(t *testing.T) {
	terms := map[string][]string{
		"蔬菜": {"番茄", "西兰花"},
		"肉类": {"鸡蛋"},
		"无效": {"不会加入"},
	}

	got := buildShoppingCategoryExact(terms)
	if got["番茄"] != "蔬菜" {
		t.Fatalf("番茄 category = %q, want 蔬菜", got["番茄"])
	}
	if got["鸡蛋"] != "肉类" {
		t.Fatalf("鸡蛋 category = %q, want 肉类", got["鸡蛋"])
	}
	if _, exists := got["不会加入"]; exists {
		t.Fatalf("invalid category term should be ignored")
	}
}

func TestClassifyShoppingItem(t *testing.T) {
	overrides := map[string]string{"紫菜": "其他"}
	tests := []struct {
		name string
		want string
	}{
		{name: "番茄", want: "蔬菜"},
		{name: "西红柿", want: "蔬菜"},
		{name: "西兰花", want: "蔬菜"},
		{name: "豆腐", want: "蔬菜"},
		{name: "青椒", want: "蔬菜"},
		{name: "小米辣", want: "蔬菜"},
		{name: "金针菇", want: "蔬菜"},
		{name: "土豆", want: "蔬菜"},
		{name: "玉米", want: "蔬菜"},
		{name: "猪肉末", want: "肉类"},
		{name: "鸡蛋", want: "肉类"},
		{name: "鸡翅", want: "肉类"},
		{name: "牛腩", want: "肉类"},
		{name: "虾仁", want: "肉类"},
		{name: "鱿鱼", want: "肉类"},
		{name: "花椒粉", want: "配料"},
		{name: "豆瓣酱", want: "配料"},
		{name: "辣椒粉", want: "配料"},
		{name: "生抽", want: "配料"},
		{name: "老抽", want: "配料"},
		{name: "料酒", want: "配料"},
		{name: "鸡精", want: "配料"},
		{name: "玉米淀粉", want: "配料"},
		{name: "番茄酱", want: "配料"},
		{name: "蛋黄酱", want: "配料"},
		{name: "面粉", want: "其他"},
		{name: "糯米粉", want: "其他"},
		{name: "粉丝", want: "其他"},
		{name: "鸡蛋面", want: "其他"},
		{name: "可乐", want: "其他"},
		{name: "紫菜", want: "其他"},
		{name: "黄瓜段", want: "蔬菜"},
		{name: "五花肉片", want: "肉类"},
		{name: "黑胡椒碎", want: "配料"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyShoppingItem(tt.name, overrides); got != tt.want {
				t.Fatalf("classifyShoppingItem(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func weekDayDishIDs(day WeekDayPlan) []uint {
	var ids []uint
	for _, dish := range day.Lunch {
		ids = append(ids, dish.ID)
	}
	for _, dish := range day.Dinner {
		ids = append(ids, dish.ID)
	}
	return ids
}

func equalUintSlices(a, b []uint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRegenerateWeekPlanDayOnlyChangesTargetDay(t *testing.T) {
	setupPlanServiceTestDB(t)
	t.Cleanup(InvalidateWeekPlanCache)

	quota := WeekPlanPeriodPreferences{
		Profile: "balanced",
		Lunch:   MealQuota{MeatCount: 1, VegCount: 1},
		Dinner:  MealQuota{MeatCount: 1},
	}
	saveWeekPlanPreferenceForTest(t, WeekPlanPreferences{Weekday: quota, Weekend: quota})

	for i := 1; i <= 30; i++ {
		createDishForPlanTest(t, fmt.Sprintf("红烧肉%d", i), `["家常菜"]`, `[{"name":"五花肉","amount":"200g"}]`)
		createDishForPlanTest(t, fmt.Sprintf("青菜%d", i), `["素菜"]`, `[{"name":"青菜","amount":"1把"}]`)
	}

	plan, err := GenerateWeekPlan()
	if err != nil {
		t.Fatalf("GenerateWeekPlan() error = %v", err)
	}
	if err := SaveWeekPlan(plan); err != nil {
		t.Fatalf("SaveWeekPlan() error = %v", err)
	}

	before := make([][]uint, len(plan.Days))
	for i, day := range plan.Days {
		before[i] = weekDayDishIDs(day)
	}
	target := plan.Days[2].Date

	updated, err := RegenerateWeekPlanDay(target)
	if err != nil {
		t.Fatalf("RegenerateWeekPlanDay(%q) error = %v", target, err)
	}
	if len(updated.Days) != 7 {
		t.Fatalf("updated plan has %d days, want 7", len(updated.Days))
	}

	otherIDs := make(map[uint]bool)
	for i, day := range updated.Days {
		if i == 2 {
			continue
		}
		got := weekDayDishIDs(day)
		if !equalUintSlices(got, before[i]) {
			t.Errorf("day %d changed: before %v, after %v", i, before[i], got)
		}
		for _, id := range got {
			otherIDs[id] = true
		}
	}

	newDay := updated.Days[2]
	if newDay.Date != target {
		t.Errorf("regenerated day date = %q, want %q", newDay.Date, target)
	}
	if len(newDay.Lunch) != 2 || len(newDay.Dinner) != 1 {
		t.Errorf("regenerated day quota: lunch %d (want 2), dinner %d (want 1)", len(newDay.Lunch), len(newDay.Dinner))
	}
	for _, id := range weekDayDishIDs(newDay) {
		if otherIDs[id] {
			t.Errorf("regenerated day reuses dish %d already planned on another day", id)
		}
	}

	var recs []models.DishRecommendation
	database.DB.Where("planned_date = ? AND source = ?", target, recommendationSourceWeekPlan).Find(&recs)
	newIDs := make(map[uint]bool)
	for _, id := range weekDayDishIDs(newDay) {
		newIDs[id] = true
	}
	if len(recs) != len(newIDs) {
		t.Errorf("recommendations for %s: got %d records, want %d", target, len(recs), len(newIDs))
	}
	for _, rec := range recs {
		if !newIDs[rec.DishID] {
			t.Errorf("stale recommendation for dish %d on %s", rec.DishID, target)
		}
	}

	if _, err := RegenerateWeekPlanDay("1999-01-01"); err == nil {
		t.Errorf("RegenerateWeekPlanDay with unknown date should fail")
	}
}
