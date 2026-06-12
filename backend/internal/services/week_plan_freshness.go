package services

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"time"
)

// Freshness decay: dishes seen within the last staleRepeatWindowDays carry a
// linearly fading score penalty (factory max staleRepeatDefaultPoints at d=0).
// The window is a factory constant by design — strength is tuned on the rule
// page (stale_repeat_penalty points); see the freshness-decay design spec.
const (
	staleRepeatWindowDays    = 14
	staleRepeatDefaultPoints = 30
)

// lastSeenDishDates returns each dish's most recent appearance date
// (meal_records ∪ dish_recommendations) within the decay window.
//
// The weekStart parameter controls the recommendation cap:
//   - cap = max(today, weekStart-1)
//   - Current-week generation keeps cap=today: future rows are the plan's own
//     tail, not food the family has eaten; excluding them prevents the planner
//     from penalising dishes it just scheduled.
//   - Future-week generation raises the cap to weekStart-1: dishes planned for
//     the current week's remaining days count as prior context, so a dish
//     scheduled for this Sunday still carries a freshness penalty on next Monday.
//   - A week's own rows are never included when generating it, because
//     weekStart-1 < weekStart (the first day of that week).
//
// now is injectable: callers pass planNow(); tests pin it via withPlanNow.
func lastSeenDishDates(now time.Time, weekStart string) map[uint]string {
	today := now.Format("2006-01-02")
	since := now.AddDate(0, 0, -(staleRepeatWindowDays - 1)).Format("2006-01-02")

	// Raise the cap to weekStart-1 when generating for a future week so that
	// the current week's planned tail counts as prior context.
	capDate := today
	if parsed, err := time.Parse("2006-01-02", weekStart); err == nil {
		dayBefore := parsed.AddDate(0, 0, -1).Format("2006-01-02")
		if dayBefore > today {
			capDate = dayBefore
		}
	}

	type lastSeenRow struct {
		DishID   uint
		LastDate string
	}
	merge := func(result map[uint]string, rows []lastSeenRow) {
		for _, row := range rows {
			if row.DishID == 0 || row.LastDate == "" {
				continue
			}
			if existing, ok := result[row.DishID]; !ok || row.LastDate > existing {
				result[row.DishID] = row.LastDate
			}
		}
	}

	result := make(map[uint]string)
	var eaten []lastSeenRow
	database.DB.Model(&models.MealRecord{}).
		Select("dish_id, MAX(meal_date) AS last_date").
		Where("meal_date >= ?", since).
		Group("dish_id").
		Scan(&eaten)
	merge(result, eaten)

	var planned []lastSeenRow
	database.DB.Model(&models.DishRecommendation{}).
		Select("dish_id, MAX(planned_date) AS last_date").
		Where("planned_date >= ? AND planned_date <= ?", since, capDate).
		Group("dish_id").
		Scan(&planned)
	merge(result, planned)
	return result
}

// daysSinceFor converts lastSeen dates into whole-day distances relative to
// refDate. Unparsable or future dates map to -1 (= no penalty); -1 is the
// sentinel because a zero value would mean "seen today".
func daysSinceFor(refDate time.Time, lastSeen map[uint]string) map[uint]int {
	ref := time.Date(refDate.Year(), refDate.Month(), refDate.Day(), 0, 0, 0, 0, time.UTC)
	result := make(map[uint]int, len(lastSeen))
	for id, raw := range lastSeen {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			result[id] = -1
			continue
		}
		d := int(ref.Sub(parsed) / (24 * time.Hour))
		if d < 0 {
			d = -1
		}
		result[id] = d
	}
	return result
}
