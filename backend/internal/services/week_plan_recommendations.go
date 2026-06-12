package services

import (
	"encoding/json"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"sort"

	"gorm.io/gorm"
)

const recommendationSourceWeekPlan = "week_plan"

// SaveWeekPlan persists the plan to the week-start row derived from the plan's
// own days (week routing by content) and refreshes the flat recommendation rows.
// Two freeze guards live here so EVERY write path (generation, regeneration,
// manual edit PUT) honours read-only history: restorePastDaysFromStored swaps
// days before today back to the stored version, and replaceWeekPlanRecommendations
// never touches past rows. The in-memory cache is only updated when the saved
// week matches the current week.
func SaveWeekPlan(plan *WeekPlan) error {
	if plan == nil {
		plan = &WeekPlan{Days: []WeekDayPlan{}}
	}
	normalizeWeekPlan(plan)
	weekKey := getCurrentWeekKey()
	if len(plan.Days) > 0 {
		if derived, err := mondayOf(plan.Days[0].Date); err == nil {
			weekKey = derived
		}
	}
	restorePastDaysFromStored(plan, weekKey)
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := upsertWeekPlanRecordPlan(tx, weekKey, string(data)); err != nil {
			return err
		}
		return replaceWeekPlanRecommendations(tx, plan)
	}); err != nil {
		return err
	}

	if weekKey == getCurrentWeekKey() {
		planMu.Lock()
		cachedPlan = plan
		cachedWeekKey = weekKey
		planMu.Unlock()
	}
	return nil
}

// restorePastDaysFromStored swaps any day before today back to the stored
// snapshot version (matched by date). History is read-only: a client PUT or
// a full regenerate cannot rewrite what was planned on days already gone.
// Past days the incoming plan omits entirely are merged back in (and the days
// re-sorted by date), so a partial PUT cannot silently drop them either.
// First-ever save of a week has no stored row, so nothing to restore.
func restorePastDaysFromStored(plan *WeekPlan, weekStart string) {
	today := todayKey()
	rec, ok := loadWeekPlanRecord(weekStart)
	if !ok || rec.PlanJSON == "" {
		return
	}
	var stored WeekPlan
	if json.Unmarshal([]byte(rec.PlanJSON), &stored) != nil {
		return
	}
	storedByDate := make(map[string]WeekDayPlan, len(stored.Days))
	for _, day := range stored.Days {
		storedByDate[day.Date] = day
	}
	planDates := make(map[string]bool, len(plan.Days))
	for i, day := range plan.Days {
		planDates[day.Date] = true
		if day.Date >= today {
			continue
		}
		if storedDay, exists := storedByDate[day.Date]; exists {
			plan.Days[i] = storedDay
		}
	}
	merged := false
	for _, day := range stored.Days {
		if day.Date == "" || day.Date >= today || planDates[day.Date] {
			continue
		}
		plan.Days = append(plan.Days, day)
		merged = true
	}
	if merged {
		sort.SliceStable(plan.Days, func(i, j int) bool {
			return plan.Days[i].Date < plan.Days[j].Date
		})
	}
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

// replaceWeekPlanRecommendations refreshes the flat rows for today and later
// only. Past rows are immutable history — the cooldown window and the History
// page rely on them recording what was actually planned at the time.
func replaceWeekPlanRecommendations(tx *gorm.DB, plan *WeekPlan) error {
	today := todayKey()
	dates := make([]string, 0, len(plan.Days))
	seenDates := make(map[string]bool, len(plan.Days))
	for _, day := range plan.Days {
		if day.Date == "" || day.Date < today || seenDates[day.Date] {
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
		if day.Date == "" || day.Date < today {
			continue
		}
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
