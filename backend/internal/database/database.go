package database

import (
	"encoding/json"
	"ninimenu/internal/achievements"
	"ninimenu/internal/config"
	"ninimenu/internal/dishes"
	"ninimenu/internal/models"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Init() error {
	dir := filepath.Dir(config.C.DBPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	var err error
	DB, err = gorm.Open(sqlite.Open(config.C.DBPath), &gorm.Config{})
	if err != nil {
		return err
	}
	if err := configureSQLite(DB); err != nil {
		return err
	}

	if err := DB.AutoMigrate(
		&models.Dish{},
		&models.MealRecord{},
		&models.Favorite{},
		&models.Quote{},
		&models.Achievement{},
		&models.UserAchievement{},
		&models.AchievementEvent{},
		&models.BlindBox{},
		&models.Holiday{},
		&models.Setting{},
		&models.DishRecommendation{},
		&models.WeekPlanRecord{},
		&models.MenuRule{},
		&models.DayRating{},
		&models.ShoppingCheck{},
		&models.HomeInventory{},
		&models.ShoppingItemCategory{},
	); err != nil {
		return err
	}

	seedData()
	backfillDishTraits()
	return nil
}

func configureSQLite(db *gorm.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA temp_store = MEMORY",
	}
	for _, pragma := range pragmas {
		if err := db.Exec(pragma).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedData() {
	var count int64
	DB.Model(&models.Setting{}).Count(&count)
	if count == 0 {
		DB.Create(&models.Setting{Key: "repeat_days", Value: "3"})
		DB.Create(&models.Setting{Key: "pick_animation", Value: "wheel"})
		DB.Create(&models.Setting{Key: "blind_box_enabled", Value: "1"})
		DB.Create(&models.Setting{Key: "voice_enabled", Value: "1"})
	}

	DB.Model(&models.Quote{}).Count(&count)
	if count == 0 {
		quotes := []models.Quote{
			{Content: "中午来点硬菜，下午才有力气搬砖", Scene: "lunch"},
			{Content: "今晚吃顿好的，犒劳一下辛苦的你", Scene: "dinner"},
			{Content: "15分钟搞定，比外卖还快！", Scene: "quick"},
			{Content: "喝碗热汤，暖暖的，很贴心", Scene: "soup"},
			{Content: "你的心头好又来啦~", Scene: "favorite"},
			{Content: "今天辛苦了，吃点好的犒劳自己", Scene: "lunch"},
			{Content: "简单做做，好吃就行", Scene: "quick"},
			{Content: "换个口味，换种心情", Scene: "dinner"},
			{Content: "这道菜，吃了会开心的", Scene: "lunch"},
			{Content: "美食是最好的治愈", Scene: "dinner"},
			{Content: "尝尝这道，保准你满意", Scene: "lunch"},
			{Content: "下班了，来顿丰盛的", Scene: "dinner"},
			{Content: "好久没吃这个了吧？来一个！", Scene: "favorite"},
			{Content: "今天有点累？来道简单的", Scene: "quick"},
			{Content: "周末就该吃点好的", Scene: "dinner"},
		}
		DB.Create(&quotes)
	}

	for _, a := range achievements.DefaultAchievements() {
		var existing models.Achievement
		if err := DB.Where("code = ?", a.Code).First(&existing).Error; err == nil {
			if existing.Condition == "" || existing.Condition == "manual" {
				DB.Model(&existing).Update("condition", a.Condition)
			}
			continue
		}
		DB.Create(&a)
	}

	for _, dish := range dishes.DefaultDishes() {
		var existing models.Dish
		if err := DB.Where("name = ?", dish.Name).First(&existing).Error; err == nil {
			continue
		}
		DB.Create(&dish)
	}

	var dishesPerDayCount int64
	DB.Model(&models.Setting{}).Where("`key` = ?", "dishes_per_day").Count(&dishesPerDayCount)
	if dishesPerDayCount == 0 {
		DB.Create(&models.Setting{Key: "dishes_per_day", Value: "2"})
	}

	var lunchPerDayCount int64
	DB.Model(&models.Setting{}).Where("`key` = ?", "lunch_dishes_per_day").Count(&lunchPerDayCount)
	if lunchPerDayCount == 0 {
		DB.Create(&models.Setting{Key: "lunch_dishes_per_day", Value: "1"})
	}

	var dinnerPerDayCount int64
	DB.Model(&models.Setting{}).Where("`key` = ?", "dinner_dishes_per_day").Count(&dinnerPerDayCount)
	if dinnerPerDayCount == 0 {
		DB.Create(&models.Setting{Key: "dinner_dishes_per_day", Value: "1"})
	}

	var achievementsEnabledCount int64
	DB.Model(&models.Setting{}).Where("`key` = ?", "achievements_enabled").Count(&achievementsEnabledCount)
	if achievementsEnabledCount == 0 {
		DB.Create(&models.Setting{Key: "achievements_enabled", Value: "0"})
	}

	upsertManagedStringArraySetting("categories", dishes.DefaultCategories())

	upsertStringArraySetting("tastes", dishes.DefaultTastes())
}

func jsonArrayString(values []string) string {
	data, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func upsertStringArraySetting(key string, defaults []string) {
	var setting models.Setting
	if err := DB.Where("`key` = ?", key).First(&setting).Error; err != nil {
		DB.Create(&models.Setting{Key: key, Value: jsonArrayString(defaults)})
		return
	}

	existing := parseStringArray(setting.Value)
	merged := appendMissingStringValues(existing, defaults)
	if !sameStringValues(existing, merged) {
		DB.Model(&models.Setting{}).Where("`key` = ?", key).Update("value", jsonArrayString(merged))
	}
}

func backfillDishTraits() {
	var rows []models.Dish
	DB.Find(&rows)
	for _, dish := range rows {
		if dish.TraitSource == "manual" && dish.TraitVersion >= models.DishTraitVersion {
			continue
		}
		if dish.TraitSource == "manual" {
			continue
		}
		inferred := models.InferDishTraits(dish)
		DB.Model(&models.Dish{}).Where("id = ?", dish.ID).Updates(map[string]any{
			"dish_role":           inferred.DishRole,
			"protein_sources":     inferred.ProteinSources,
			"serving_temperature": inferred.ServingTemperature,
			"cooking_methods":     inferred.CookingMethods,
			"spice_level":         inferred.SpiceLevel,
			"richness_level":      inferred.RichnessLevel,
			"carb_level":          inferred.CarbLevel,
			"trait_source":        inferred.TraitSource,
			"trait_version":       inferred.TraitVersion,
		})
	}
}

func upsertManagedStringArraySetting(key string, defaults []string) {
	var setting models.Setting
	if err := DB.Where("`key` = ?", key).First(&setting).Error; err != nil {
		DB.Create(&models.Setting{Key: key, Value: jsonArrayString(defaults)})
		return
	}

	allowed := stringSet(defaults)
	existing := parseStringArray(setting.Value)
	managed := make([]string, 0, len(defaults))
	for _, value := range existing {
		if allowed[value] {
			managed = append(managed, value)
		}
	}
	managed = appendMissingStringValues(managed, defaults)
	if !sameStringValues(existing, managed) {
		DB.Model(&models.Setting{}).Where("`key` = ?", key).Update("value", jsonArrayString(managed))
	}
}

func parseStringArray(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	return values
}

func appendMissingStringValues(existing []string, defaults []string) []string {
	seen := make(map[string]bool, len(existing)+len(defaults))
	result := make([]string, 0, len(existing)+len(defaults))
	for _, value := range existing {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	for _, value := range defaults {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		if value != "" {
			result[value] = true
		}
	}
	return result
}

func sameStringValues(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
