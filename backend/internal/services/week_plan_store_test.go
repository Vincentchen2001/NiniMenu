package services

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"strings"
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

func TestMigrateWeekPlanStorageMovesCurrentWeekCache(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local)) // 周一=2026-06-15

	legacy := `{"days":[{"date":"2026-06-15","day_name":"周一","lunch":[],"dinner":[]}],"warnings":[]}`
	if err := database.DB.Create(&models.Setting{Key: "week_plan_cache", Value: legacy}).Error; err != nil {
		t.Fatalf("seed legacy cache: %v", err)
	}

	if err := MigrateWeekPlanStorage(); err != nil {
		t.Fatalf("MigrateWeekPlanStorage error = %v", err)
	}

	rec, ok := loadWeekPlanRecord("2026-06-15")
	if !ok || rec.PlanJSON != legacy {
		t.Fatalf("current-week cache must move into the row, ok=%v", ok)
	}
	var count int64
	database.DB.Model(&models.Setting{}).Where("`key` = ?", "week_plan_cache").Count(&count)
	if count != 0 {
		t.Fatalf("legacy cache key must be deleted, count=%d", count)
	}

	// 幂等：再跑一遍是 no-op。
	if err := MigrateWeekPlanStorage(); err != nil {
		t.Fatalf("second run error = %v", err)
	}
	rec, _ = loadWeekPlanRecord("2026-06-15")
	if rec.PlanJSON != legacy {
		t.Fatalf("second run must not change the row")
	}
}

func TestMigrateWeekPlanStorageDropsStaleCache(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local))

	stale := `{"days":[{"date":"2026-06-08","day_name":"周一","lunch":[],"dinner":[]}],"warnings":[]}`
	if err := database.DB.Create(&models.Setting{Key: "week_plan_cache", Value: stale}).Error; err != nil {
		t.Fatalf("seed stale cache: %v", err)
	}

	if err := MigrateWeekPlanStorage(); err != nil {
		t.Fatalf("MigrateWeekPlanStorage error = %v", err)
	}

	if _, ok := loadWeekPlanRecord("2026-06-15"); ok {
		t.Fatalf("stale cache must not create a row for the current week")
	}
	if _, ok := loadWeekPlanRecord("2026-06-08"); ok {
		t.Fatalf("stale cache must not backfill past weeks either (history starts at upgrade)")
	}
	var count int64
	database.DB.Model(&models.Setting{}).Where("`key` = ?", "week_plan_cache").Count(&count)
	if count != 0 {
		t.Fatalf("legacy cache key must be deleted even when stale, count=%d", count)
	}
}

func TestMigrateWeekPlanStorageSplitsLegacyPrefs(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local))

	legacyPrefs := `{"weekday":{"profile":"quick","lunch":{"meat_count":2,"veg_count":0,"soup_count":0},"dinner":{"meat_count":1,"veg_count":1,"soup_count":1}},"weekend":{"profile":"balanced","lunch":{"meat_count":1,"veg_count":1,"soup_count":0},"dinner":{"meat_count":2,"veg_count":1,"soup_count":1}},"week_want":["beef"],"days":{"wed":{"want":["seafood"]}}}`
	if err := database.DB.Create(&models.Setting{Key: "week_plan_preferences", Value: legacyPrefs}).Error; err != nil {
		t.Fatalf("seed legacy prefs: %v", err)
	}

	if err := MigrateWeekPlanStorage(); err != nil {
		t.Fatalf("MigrateWeekPlanStorage error = %v", err)
	}

	rec, ok := loadWeekPlanRecord("2026-06-15")
	if !ok || !strings.Contains(rec.PrefsJSON, "beef") || !strings.Contains(rec.PrefsJSON, "seafood") {
		t.Fatalf("one-off prefs must move to the week row, got %q", rec.PrefsJSON)
	}

	var setting models.Setting
	if err := database.DB.Where("`key` = ?", "week_plan_preferences").First(&setting).Error; err != nil {
		t.Fatalf("resident prefs setting must remain: %v", err)
	}
	if strings.Contains(setting.Value, "week_want") || strings.Contains(setting.Value, `"days"`) {
		t.Fatalf("one-off parts must be stripped from the setting: %s", setting.Value)
	}
	if !strings.Contains(setting.Value, "quick") {
		t.Fatalf("resident parts must survive: %s", setting.Value)
	}

	// 合成读取仍能看到迁移过来的一次性偏好。
	got := GetWeekPlanPreferences()
	if len(got.WeekWant) != 1 || got.WeekWant[0] != "beef" {
		t.Fatalf("composed prefs after migration = %+v", got)
	}
}
