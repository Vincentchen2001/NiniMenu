package services

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"strings"
	"testing"
	"time"
)

func TestWeekPlanPreferencesDaysNormalization(t *testing.T) {
	prefs := normalizeWeekPlanPreferences(WeekPlanPreferences{
		WeekWant: []string{"poultry", "bogus", "poultry"},
		Days: map[string]DayOverride{
			"mon": {Profile: "spicy", Want: []string{"beef", "junk"}},
			"xyz": {Profile: "light"},
			"tue": {Profile: "weird"},
			"wed": {},
		},
	})
	if len(prefs.WeekWant) != 1 || prefs.WeekWant[0] != "poultry" {
		t.Fatalf("week want = %v, want [poultry]", prefs.WeekWant)
	}
	if _, ok := prefs.Days["xyz"]; ok {
		t.Fatalf("invalid day key should be dropped")
	}
	if prefs.Days["mon"].Profile != "spicy" || len(prefs.Days["mon"].Want) != 1 || prefs.Days["mon"].Want[0] != "beef" {
		t.Fatalf("mon override = %+v", prefs.Days["mon"])
	}
	if _, ok := prefs.Days["tue"]; ok {
		t.Fatalf("override with invalid profile and no want should be dropped, got %+v", prefs.Days["tue"])
	}
	if _, ok := prefs.Days["wed"]; ok {
		t.Fatalf("empty override should be dropped")
	}
}

func TestWeekPlanPreferencesLegacyJSONLoads(t *testing.T) {
	setupPlanServiceTestDB(t)
	legacy := `{"weekday":{"profile":"balanced","lunch":{"meat_count":1,"veg_count":1,"soup_count":0},"dinner":{"meat_count":1,"veg_count":0,"soup_count":1}},"weekend":{"profile":"favorite","lunch":{"meat_count":0,"veg_count":0,"soup_count":0},"dinner":{"meat_count":2,"veg_count":1,"soup_count":1}}}`
	if err := database.DB.Create(&models.Setting{Key: "week_plan_preferences", Value: legacy}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	prefs := GetWeekPlanPreferences()
	if prefs.Weekend.Profile != "favorite" {
		t.Fatalf("legacy weekend profile = %q, want favorite", prefs.Weekend.Profile)
	}
	if len(prefs.WeekWant) != 0 || len(prefs.Days) != 0 {
		t.Fatalf("legacy JSON should have no want/days, got %v / %v", prefs.WeekWant, prefs.Days)
	}
}

func TestSaveWeekPlanPreferencesRoundTripsDays(t *testing.T) {
	setupPlanServiceTestDB(t)
	saveWeekPlanPreferenceForTest(t, WeekPlanPreferences{
		Weekday:  WeekPlanPeriodPreferences{Profile: "balanced", Lunch: MealQuota{MeatCount: 1}},
		Weekend:  WeekPlanPeriodPreferences{Profile: "balanced", Dinner: MealQuota{MeatCount: 1}},
		WeekWant: []string{"seafood"},
		Days:     map[string]DayOverride{"fri": {Profile: "favorite", Want: []string{"poultry"}}},
	})
	prefs := GetWeekPlanPreferences()
	if len(prefs.WeekWant) != 1 || prefs.WeekWant[0] != "seafood" {
		t.Fatalf("week want lost: %v", prefs.WeekWant)
	}
	if prefs.Days["fri"].Profile != "favorite" || len(prefs.Days["fri"].Want) != 1 {
		t.Fatalf("friday override lost: %+v", prefs.Days["fri"])
	}
}

func TestWeekPlanPreferencesOneOffPartsLiveOnWeekRow(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local)) // 周一=2026-06-15

	prefs := WeekPlanPreferences{
		Weekday:  WeekPlanPeriodPreferences{Profile: "quick", Lunch: MealQuota{MeatCount: 2}, Dinner: MealQuota{MeatCount: 1, SoupCount: 1}},
		Weekend:  WeekPlanPeriodPreferences{Profile: "balanced", Lunch: MealQuota{MeatCount: 1}, Dinner: MealQuota{MeatCount: 2}},
		WeekWant: []string{"beef"},
		Days:     map[string]DayOverride{"wed": {Want: []string{"seafood"}}},
	}
	if err := SaveWeekPlanPreferences(prefs); err != nil {
		t.Fatalf("SaveWeekPlanPreferences error = %v", err)
	}

	// 常驻 Setting 里不得残留一次性字段。
	var setting models.Setting
	if err := database.DB.Where("`key` = ?", "week_plan_preferences").First(&setting).Error; err != nil {
		t.Fatalf("resident setting missing: %v", err)
	}
	if strings.Contains(setting.Value, "week_want") || strings.Contains(setting.Value, `"days"`) {
		t.Fatalf("resident setting must not contain one-off parts: %s", setting.Value)
	}

	// 一次性部分落在本周行。
	rec, ok := loadWeekPlanRecord("2026-06-15")
	if !ok || rec.PrefsJSON == "" {
		t.Fatalf("one-off prefs must land on the week row, ok=%v", ok)
	}
	if !strings.Contains(rec.PrefsJSON, "beef") || !strings.Contains(rec.PrefsJSON, "seafood") {
		t.Fatalf("PrefsJSON = %s, want want-lists", rec.PrefsJSON)
	}

	// 合成视图完整 round-trip。
	got := GetWeekPlanPreferences()
	if got.Weekday.Profile != "quick" || len(got.WeekWant) != 1 || got.WeekWant[0] != "beef" {
		t.Fatalf("composed prefs = %+v", got)
	}
	if got.Days["wed"].Want[0] != "seafood" {
		t.Fatalf("day override lost: %+v", got.Days)
	}
}

func TestWeekPlanPreferencesOneOffPartsResetOnRollover(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local))

	prefs := WeekPlanPreferences{
		Weekday:  WeekPlanPeriodPreferences{Profile: "light", Lunch: MealQuota{VegCount: 2}, Dinner: MealQuota{MeatCount: 1}},
		Weekend:  WeekPlanPeriodPreferences{Profile: "balanced", Lunch: MealQuota{MeatCount: 1}, Dinner: MealQuota{MeatCount: 1}},
		WeekWant: []string{"pork"},
		Days:     map[string]DayOverride{"fri": {Profile: "soup"}},
	}
	if err := SaveWeekPlanPreferences(prefs); err != nil {
		t.Fatalf("SaveWeekPlanPreferences error = %v", err)
	}

	// 翻篇到下周。
	withPlanNow(t, time.Date(2026, 6, 23, 9, 0, 0, 0, time.Local))

	got := GetWeekPlanPreferences()
	if len(got.WeekWant) != 0 {
		t.Fatalf("WeekWant must reset on rollover, got %v", got.WeekWant)
	}
	if len(got.Days) != 0 {
		t.Fatalf("Days overrides must reset on rollover, got %+v", got.Days)
	}
	if got.Weekday.Profile != "light" || got.Weekday.Lunch.VegCount != 2 {
		t.Fatalf("resident parts must survive rollover, got %+v", got.Weekday)
	}
}
