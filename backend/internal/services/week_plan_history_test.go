package services

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"testing"
)

func TestWeekPlanHistoryByMonthGroupsByDate(t *testing.T) {
	setupPlanServiceTestDB(t)

	rows := []models.DishRecommendation{
		{DishID: 1, DishName: "周一午餐菜", Source: "week_plan", MealType: "lunch", PlannedDate: "2026-06-15"},
		{DishID: 2, DishName: "周一晚餐汤", Source: "week_plan", MealType: "dinner", PlannedDate: "2026-06-15"},
		{DishID: 3, DishName: "周二的菜", Source: "week_plan", MealType: "lunch", PlannedDate: "2026-06-16"},
		// 下月与异 source 都不得混入。
		{DishID: 4, DishName: "七月的菜", Source: "week_plan", MealType: "lunch", PlannedDate: "2026-07-01"},
		{DishID: 5, DishName: "别的来源", Source: "tomorrow_pick", MealType: "lunch", PlannedDate: "2026-06-15"},
		// BETWEEN 边界：6 月末日必须出现，5 月末日不得混入。
		{DishID: 6, DishName: "六月末日菜", Source: "week_plan", MealType: "lunch", PlannedDate: "2026-06-30"},
		{DishID: 7, DishName: "五月末日菜", Source: "week_plan", MealType: "lunch", PlannedDate: "2026-05-31"},
	}
	if err := database.DB.Create(&rows).Error; err != nil {
		t.Fatalf("seed rows: %v", err)
	}

	got, err := WeekPlanHistoryByMonth("2026-06")
	if err != nil {
		t.Fatalf("WeekPlanHistoryByMonth error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("dates = %d, want 3: %+v", len(got), got)
	}
	mon := got["2026-06-15"]
	if len(mon) != 2 || mon[0].DishName != "周一晚餐汤" || mon[1].DishName != "周一午餐菜" {
		// meal_type ASC: dinner < lunch（字典序），同日内先 dinner 后 lunch。
		t.Fatalf("monday entries = %+v", mon)
	}
	if len(got["2026-06-16"]) != 1 || got["2026-06-16"][0].DishID != 3 {
		t.Fatalf("tuesday entries = %+v", got["2026-06-16"])
	}
	if len(got["2026-06-30"]) != 1 || got["2026-06-30"][0].DishID != 6 {
		t.Fatalf("month-end entries = %+v", got["2026-06-30"])
	}
	if _, ok := got["2026-05-31"]; ok {
		t.Fatalf("previous-month boundary row leaked in: %+v", got["2026-05-31"])
	}
}

func TestWeekPlanHistoryByMonthValidatesMonth(t *testing.T) {
	setupPlanServiceTestDB(t)

	for _, bad := range []string{"2026-6", "202606", "2026-13", "abcd-ef", ""} {
		if _, err := WeekPlanHistoryByMonth(bad); err == nil {
			t.Fatalf("month %q must be rejected", bad)
		}
	}
}
