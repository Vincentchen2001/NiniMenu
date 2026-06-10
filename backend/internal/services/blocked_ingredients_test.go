package services

import (
	"fmt"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"strings"
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

func TestGenerateWeekPlanExcludesBlockedIngredients(t *testing.T) {
	setupPlanServiceTestDB(t)
	if err := database.DB.Create(&models.Setting{Key: "blocked_ingredients", Value: `["香菜"]`}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	saveWeekPlanPreferenceForTest(t, WeekPlanPreferences{
		Weekday: WeekPlanPeriodPreferences{
			Profile: "balanced",
			Lunch:   MealQuota{VegCount: 1},
			Dinner:  MealQuota{VegCount: 1},
		},
		Weekend: WeekPlanPeriodPreferences{
			Profile: "balanced",
			Lunch:   MealQuota{VegCount: 1},
			Dinner:  MealQuota{VegCount: 1},
		},
	})
	for i := 1; i <= 10; i++ {
		createDishForPlanTest(t, fmt.Sprintf("香菜豆腐%d", i), `["素菜"]`, `[{"name":"香菜","amount":"1把"},{"name":"豆腐","amount":"1块"}]`)
		createDishForPlanTest(t, fmt.Sprintf("清炒青菜%d", i), `["素菜"]`, `[{"name":"青菜","amount":"1把"}]`)
	}

	plan, err := GenerateWeekPlan()
	if err != nil {
		t.Fatalf("GenerateWeekPlan() error = %v", err)
	}
	for _, day := range plan.Days {
		for _, dish := range append(append([]models.Dish{}, day.Lunch...), day.Dinner...) {
			if strings.Contains(dish.Name, "香菜") {
				t.Fatalf("%s contains blocked dish %s", day.DayName, dish.Name)
			}
		}
	}
}

func TestPickTomorrowDishesExcludesBlocked(t *testing.T) {
	setupPlanServiceTestDB(t)
	if err := database.DB.Create(&models.Setting{Key: "blocked_ingredients", Value: `["虾"]`}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	createDishForPlanTest(t, "油焖大虾", `["家常菜"]`, `[{"name":"大虾","amount":"300g"}]`)
	createDishForPlanTest(t, "清炒时蔬", `["素菜"]`, `[{"name":"青菜","amount":"1把"}]`)

	dishes, err := PickTomorrowDishes(TomorrowPickOptions{Count: 5})
	if err != nil {
		t.Fatalf("PickTomorrowDishes() error = %v", err)
	}
	if len(dishes) == 0 {
		t.Fatalf("PickTomorrowDishes() returned nothing")
	}
	for _, dish := range dishes {
		if strings.Contains(dish.Name, "虾") {
			t.Fatalf("blocked dish recommended: %s", dish.Name)
		}
	}
}
