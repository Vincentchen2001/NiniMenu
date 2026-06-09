package services

import (
	"encoding/json"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"strings"
)

const (
	weekPlanPreferencesSettingKey = "week_plan_preferences"
	maxWeekPlanDishKindCount      = 5
)

type MealQuota struct {
	MeatCount int `json:"meat_count"`
	VegCount  int `json:"veg_count"`
	SoupCount int `json:"soup_count"`
}

type WeekPlanPeriodPreferences struct {
	Profile string    `json:"profile"`
	Lunch   MealQuota `json:"lunch"`
	Dinner  MealQuota `json:"dinner"`
}

type WeekPlanPreferences struct {
	Weekday WeekPlanPeriodPreferences `json:"weekday"`
	Weekend WeekPlanPeriodPreferences `json:"weekend"`
}

type dishProteinKind string

const (
	dishProteinMeat  dishProteinKind = "meat"
	dishProteinVeg   dishProteinKind = "veg"
	dishProteinOther dishProteinKind = "other"
)

func (q MealQuota) total() int {
	return q.MeatCount + q.VegCount + q.SoupCount
}

func GetWeekPlanPreferences() WeekPlanPreferences {
	var setting models.Setting
	if err := database.DB.Where("`key` = ?", weekPlanPreferencesSettingKey).First(&setting).Error; err == nil && setting.Value != "" {
		var prefs WeekPlanPreferences
		if json.Unmarshal([]byte(setting.Value), &prefs) == nil {
			return normalizeWeekPlanPreferences(prefs)
		}
	}
	return defaultWeekPlanPreferencesFromSettings()
}

func SaveWeekPlanPreferences(prefs WeekPlanPreferences) error {
	prefs = normalizeWeekPlanPreferences(prefs)
	data, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	if err := database.DB.Model(&models.Setting{}).
		Where("`key` = ?", weekPlanPreferencesSettingKey).
		Assign(models.Setting{Key: weekPlanPreferencesSettingKey, Value: string(data)}).
		FirstOrCreate(&models.Setting{}).Error; err != nil {
		return err
	}
	InvalidateWeekPlanCache()
	return nil
}

func defaultWeekPlanPreferencesFromSettings() WeekPlanPreferences {
	lunch := legacyQuotaFromCount(getSettingInt("lunch_dishes_per_day", 1))
	dinner := legacyQuotaFromCount(getSettingInt("dinner_dishes_per_day", 1))
	return normalizeWeekPlanPreferences(WeekPlanPreferences{
		Weekday: WeekPlanPeriodPreferences{
			Profile: "balanced",
			Lunch:   lunch,
			Dinner:  dinner,
		},
		Weekend: WeekPlanPeriodPreferences{
			Profile: "balanced",
			Lunch:   lunch,
			Dinner:  dinner,
		},
	})
}

func legacyQuotaFromCount(count int) MealQuota {
	if count <= 0 {
		return MealQuota{}
	}
	return MealQuota{MeatCount: count}
}

func normalizeWeekPlanPreferences(prefs WeekPlanPreferences) WeekPlanPreferences {
	prefs.Weekday = normalizeWeekPlanPeriodPreferences(prefs.Weekday)
	prefs.Weekend = normalizeWeekPlanPeriodPreferences(prefs.Weekend)
	return prefs
}

func normalizeWeekPlanPeriodPreferences(prefs WeekPlanPeriodPreferences) WeekPlanPeriodPreferences {
	prefs.Profile = normalizePlanProfile(prefs.Profile)
	prefs.Lunch = normalizeMealQuota(prefs.Lunch)
	prefs.Dinner = normalizeMealQuota(prefs.Dinner)
	return prefs
}

func normalizePlanProfile(profile string) string {
	switch strings.TrimSpace(profile) {
	case "quick", "light", "spicy", "favorite":
		return strings.TrimSpace(profile)
	default:
		return "balanced"
	}
}

func normalizeMealQuota(quota MealQuota) MealQuota {
	quota.MeatCount = clampInt(quota.MeatCount, 0, maxWeekPlanDishKindCount)
	quota.VegCount = clampInt(quota.VegCount, 0, maxWeekPlanDishKindCount)
	quota.SoupCount = clampInt(quota.SoupCount, 0, maxWeekPlanDishKindCount)
	return quota
}

func clampInt(value int, min int, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func classifyDishProteinKind(dish models.Dish) dishProteinKind {
	tags := parseTags(dish.Tags)
	textParts := []string{dish.Name, dish.Category, dish.Taste}
	textParts = append(textParts, tags...)
	textParts = append(textParts, ingredientNames(dish.Ingredients)...)
	text := strings.Join(textParts, " ")

	if containsAnyKeyword(text, meatDishKeywords) {
		return dishProteinMeat
	}
	if hasVegTag(tags) || containsAnyKeyword(text, vegDishKeywords) {
		return dishProteinVeg
	}
	return dishProteinOther
}

func isSoupDish(dish models.Dish) bool {
	tags := parseTags(dish.Tags)
	textParts := []string{dish.Name, dish.Category}
	textParts = append(textParts, tags...)
	text := strings.Join(textParts, " ")
	return containsAnyKeyword(text, soupDishKeywords)
}

func ingredientNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var named []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(raw), &named); err == nil {
		var result []string
		for _, item := range named {
			if item.Name != "" {
				result = append(result, item.Name)
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	var stringsOnly []string
	if err := json.Unmarshal([]byte(raw), &stringsOnly); err == nil {
		return stringsOnly
	}

	return []string{raw}
}

func hasVegTag(tags []string) bool {
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "素菜" || tag == "纯素" || tag == "蔬菜" || tag == "青菜" {
			return true
		}
	}
	return false
}

func containsAnyKeyword(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if keyword != "" && strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

var meatDishKeywords = []string{
	"肉", "猪", "牛", "羊", "鸡", "鸭", "鹅", "鱼", "虾", "蟹", "贝", "蛤",
	"鱿", "墨鱼", "章鱼", "蛋", "排骨", "五花", "培根", "腊肠", "香肠", "火腿",
	"肥肠", "猪肝", "牛腩", "鸡翅", "鸡腿", "鸡胸", "里脊",
}

var vegDishKeywords = []string{
	"青菜", "白菜", "生菜", "菠菜", "油麦", "西兰花", "花菜", "豆腐", "豆芽",
	"土豆", "番茄", "西红柿", "黄瓜", "茄子", "南瓜", "冬瓜", "丝瓜", "苦瓜",
	"萝卜", "胡萝卜", "蘑菇", "香菇", "金针菇", "平菇", "木耳", "芹菜", "莴笋",
	"藕", "玉米", "豌豆", "四季豆", "荷兰豆", "空心菜", "韭菜",
}

var soupDishKeywords = []string{
	"汤", "羹", "煲", "汤品", "蛋花汤", "例汤",
}
