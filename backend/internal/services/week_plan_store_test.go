package services

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"testing"
	"time"
)

// withPlanNow pins the week-plan clock to a fixed instant for the test.
func withPlanNow(t *testing.T, fixed time.Time) {
	t.Helper()
	original := planNow
	planNow = func() time.Time { return fixed }
	t.Cleanup(func() { planNow = original })
}

func TestTodayKeyAndWeekKeyUsePlanNow(t *testing.T) {
	// 2026-06-18 is a Thursday; its Monday is 2026-06-15.
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local))

	if got := todayKey(); got != "2026-06-18" {
		t.Fatalf("todayKey() = %q, want 2026-06-18", got)
	}
	if got := getCurrentWeekKey(); got != "2026-06-15" {
		t.Fatalf("getCurrentWeekKey() = %q, want 2026-06-15", got)
	}
}

func TestWeekPlanRecordUpsertsPreserveSiblingColumns(t *testing.T) {
	setupPlanServiceTestDB(t)

	if err := upsertWeekPlanRecordPrefs("2026-06-15", `{"week_want":["beef"]}`); err != nil {
		t.Fatalf("upsert prefs (create) error = %v", err)
	}
	if err := upsertWeekPlanRecordPlan(database.DB, "2026-06-15", `{"days":[]}`); err != nil {
		t.Fatalf("upsert plan (update) error = %v", err)
	}

	rec, ok := loadWeekPlanRecord("2026-06-15")
	if !ok {
		t.Fatalf("loadWeekPlanRecord should find the row")
	}
	if rec.PrefsJSON != `{"week_want":["beef"]}` {
		t.Fatalf("PrefsJSON = %q, want preserved", rec.PrefsJSON)
	}
	if rec.PlanJSON != `{"days":[]}` {
		t.Fatalf("PlanJSON = %q, want written", rec.PlanJSON)
	}

	if err := upsertWeekPlanRecordPrefs("2026-06-15", `{"week_want":["pork"]}`); err != nil {
		t.Fatalf("upsert prefs (update) error = %v", err)
	}
	rec, _ = loadWeekPlanRecord("2026-06-15")
	if rec.PlanJSON != `{"days":[]}` {
		t.Fatalf("PlanJSON = %q, must survive prefs update", rec.PlanJSON)
	}
	if rec.PrefsJSON != `{"week_want":["pork"]}` {
		t.Fatalf("PrefsJSON = %q, want updated", rec.PrefsJSON)
	}

	var count int64
	database.DB.Model(&models.WeekPlanRecord{}).Count(&count)
	if count != 1 {
		t.Fatalf("week_plans row count = %d, want 1 (upsert must not duplicate)", count)
	}

	if _, ok := loadWeekPlanRecord("2026-06-08"); ok {
		t.Fatalf("loadWeekPlanRecord(other week) should miss")
	}
}
