package services

import (
	"errors"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"time"

	"gorm.io/gorm"
)

// CurrentUserID is the single-user placeholder threaded through week-plan
// storage. The multi-user sub-project will replace call sites with an
// authenticated user id; the schema (user_id column + composite unique key)
// is already in place so those rows migrate for free.
const CurrentUserID uint = 1

// planNow is the clock behind every week-plan date decision (week key, the
// today freeze boundary, recommendation rewrites). Tests swap it to pin time.
var planNow = time.Now

func todayKey() string {
	return planNow().Format("2006-01-02")
}

// loadWeekPlanRecord returns the row for (CurrentUserID, weekStart), or
// (nil, false) when it does not exist.
func loadWeekPlanRecord(weekStart string) (*models.WeekPlanRecord, bool) {
	var rec models.WeekPlanRecord
	err := database.DB.Where("user_id = ? AND week_start = ?", CurrentUserID, weekStart).First(&rec).Error
	if err != nil {
		return nil, false
	}
	return &rec, true
}

// upsertWeekPlanRecordPlan writes plan_json for (CurrentUserID, weekStart)
// inside tx, creating the row when missing. prefs_json is left untouched so
// the two columns can be written independently.
func upsertWeekPlanRecordPlan(tx *gorm.DB, weekStart string, planJSON string) error {
	var rec models.WeekPlanRecord
	err := tx.Where("user_id = ? AND week_start = ?", CurrentUserID, weekStart).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&models.WeekPlanRecord{UserID: CurrentUserID, WeekStart: weekStart, PlanJSON: planJSON}).Error
	}
	if err != nil {
		return err
	}
	return tx.Model(&rec).Update("plan_json", planJSON).Error
}

// upsertWeekPlanRecordPrefs mirrors upsertWeekPlanRecordPlan for prefs_json.
// It runs outside any caller transaction because preference saves are
// standalone writes.
func upsertWeekPlanRecordPrefs(weekStart string, prefsJSON string) error {
	var rec models.WeekPlanRecord
	err := database.DB.Where("user_id = ? AND week_start = ?", CurrentUserID, weekStart).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return database.DB.Create(&models.WeekPlanRecord{UserID: CurrentUserID, WeekStart: weekStart, PrefsJSON: prefsJSON}).Error
	}
	if err != nil {
		return err
	}
	return database.DB.Model(&rec).Update("prefs_json", prefsJSON).Error
}
