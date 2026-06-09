package services

import (
	"encoding/json"
	"ninimenu/internal/database"
	"ninimenu/internal/models"

	"gorm.io/gorm"
)

const recommendationSourceWeekPlan = "week_plan"

func SaveWeekPlan(plan *WeekPlan) error {
	if plan == nil {
		plan = &WeekPlan{Days: []WeekDayPlan{}}
	}
	normalizeWeekPlan(plan)
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Setting{}).
			Where("`key` = ?", "week_plan_cache").
			Assign(models.Setting{Key: "week_plan_cache", Value: string(data)}).
			FirstOrCreate(&models.Setting{}).Error; err != nil {
			return err
		}
		return replaceWeekPlanRecommendations(tx, plan)
	}); err != nil {
		return err
	}

	planMu.Lock()
	cachedPlan = plan
	cachedWeekKey = getCurrentWeekKey()
	planMu.Unlock()
	return nil
}

func normalizeWeekPlan(plan *WeekPlan) {
	for i := range plan.Days {
		if plan.Days[i].Lunch == nil {
			plan.Days[i].Lunch = []models.Dish{}
		}
		if plan.Days[i].Dinner == nil {
			plan.Days[i].Dinner = []models.Dish{}
		}
	}
}

func replaceWeekPlanRecommendations(tx *gorm.DB, plan *WeekPlan) error {
	dates := make([]string, 0, len(plan.Days))
	seenDates := make(map[string]bool, len(plan.Days))
	for _, day := range plan.Days {
		if day.Date == "" || seenDates[day.Date] {
			continue
		}
		seenDates[day.Date] = true
		dates = append(dates, day.Date)
	}

	if len(dates) > 0 {
		if err := tx.Where("source = ? AND planned_date IN ?", recommendationSourceWeekPlan, dates).
			Delete(&models.DishRecommendation{}).Error; err != nil {
			return err
		}
	}

	var records []models.DishRecommendation
	for _, day := range plan.Days {
		records = append(records, dishRecommendationsForMeal(day.Date, "lunch", day.Lunch)...)
		records = append(records, dishRecommendationsForMeal(day.Date, "dinner", day.Dinner)...)
	}
	if len(records) == 0 {
		return nil
	}
	return tx.Create(&records).Error
}

func dishRecommendationsForMeal(plannedDate string, mealType string, dishes []models.Dish) []models.DishRecommendation {
	if plannedDate == "" || mealType == "" || len(dishes) == 0 {
		return nil
	}
	records := make([]models.DishRecommendation, 0, len(dishes))
	for _, dish := range dishes {
		if dish.ID == 0 {
			continue
		}
		records = append(records, models.DishRecommendation{
			DishID:      dish.ID,
			DishName:    dish.Name,
			Source:      recommendationSourceWeekPlan,
			MealType:    mealType,
			PlannedDate: plannedDate,
		})
	}
	return records
}
