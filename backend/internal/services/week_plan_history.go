package services

import (
	"fmt"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"regexp"
	"time"
)

// PlannedDishEntry is one dish of a stored day plan, served to the History
// page so it can show "planned vs eaten" for a day. DishName is a snapshot:
// it stays readable even after the dish is deleted from the library.
type PlannedDishEntry struct {
	DishID   uint   `json:"dish_id"`
	DishName string `json:"dish_name"`
	MealType string `json:"meal_type"`
}

var historyMonthPattern = regexp.MustCompile(`^\d{4}-\d{2}$`)

// WeekPlanHistoryByMonth returns the planned dishes recorded for each date of
// the month, keyed by YYYY-MM-DD. Rows come from dish_recommendations, whose
// past entries are immutable (SaveWeekPlan only rewrites today and later), so
// this is a faithful record of what was planned at the time.
func WeekPlanHistoryByMonth(month string) (map[string][]PlannedDishEntry, error) {
	if !historyMonthPattern.MatchString(month) {
		return nil, fmt.Errorf("月份格式无效，应为 YYYY-MM")
	}
	if _, err := time.Parse("2006-01", month); err != nil {
		return nil, fmt.Errorf("月份格式无效，应为 YYYY-MM")
	}

	var rows []models.DishRecommendation
	// 字符串 BETWEEN：月内任何日期 <= "-31"，对短月份同样安全。
	if err := database.DB.
		Where("source = ? AND planned_date BETWEEN ? AND ?", recommendationSourceWeekPlan, month+"-01", month+"-31").
		Order("planned_date ASC, meal_type ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[string][]PlannedDishEntry, len(rows))
	for _, row := range rows {
		result[row.PlannedDate] = append(result[row.PlannedDate], PlannedDishEntry{
			DishID:   row.DishID,
			DishName: row.DishName,
			MealType: row.MealType,
		})
	}
	return result, nil
}
