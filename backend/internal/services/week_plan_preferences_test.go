package services

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"testing"
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
