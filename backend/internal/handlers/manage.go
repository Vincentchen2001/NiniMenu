package handlers

import (
	"encoding/json"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func GetQuotes(c *gin.Context) {
	var quotes []models.Quote
	database.DB.Order("created_at DESC").Find(&quotes)
	utils.Success(c, quotes)
}

type CreateQuoteRequest struct {
	Content string `json:"content" binding:"required"`
	Scene   string `json:"scene"`
}

func CreateQuote(c *gin.Context) {
	var req CreateQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "推荐语内容不能为空")
		return
	}

	quote := models.Quote{Content: req.Content, Scene: req.Scene, Enabled: true}
	database.DB.Create(&quote)
	utils.Success(c, quote)
}

func UpdateQuote(c *gin.Context) {
	id := c.Param("id")
	var quote models.Quote
	if err := database.DB.First(&quote, id).Error; err != nil {
		utils.NotFound(c, "推荐语不存在")
		return
	}

	var req CreateQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}

	quote.Content = req.Content
	quote.Scene = req.Scene
	database.DB.Save(&quote)
	utils.Success(c, quote)
}

func DeleteQuote(c *gin.Context) {
	id := c.Param("id")
	database.DB.Delete(&models.Quote{}, id)
	utils.SuccessMsg(c, "删除成功")
}

func GetAchievements(c *gin.Context) {
	achievementsEnabled := services.AutoAchievementsEnabled()
	if achievementsEnabled {
		services.SyncAutoAchievements()
	}

	var achievements []models.Achievement
	database.DB.Order("id ASC").Find(&achievements)

	var unlocked []models.UserAchievement
	if achievementsEnabled {
		database.DB.Find(&unlocked)
	}

	unlockedMap := make(map[uint]bool)
	unlockedAtMap := make(map[uint]time.Time)
	for _, u := range unlocked {
		unlockedMap[u.AchievementID] = true
		unlockedAtMap[u.AchievementID] = u.UnlockedAt
	}

	type AchievementWithStatus struct {
		models.Achievement
		IsUnlocked bool `json:"is_unlocked"`
	}

	var result []AchievementWithStatus
	for _, a := range achievements {
		if unlockedAt, ok := unlockedAtMap[a.ID]; ok {
			a.UnlockedAt = &unlockedAt
		}
		result = append(result, AchievementWithStatus{
			Achievement: a,
			IsUnlocked:  unlockedMap[a.ID],
		})
	}

	utils.Success(c, result)
}

func UnlockAchievement(c *gin.Context) {
	id := c.Param("id")
	var achievement models.Achievement
	if err := database.DB.First(&achievement, id).Error; err != nil {
		utils.NotFound(c, "成就不存在")
		return
	}

	var unlocked models.UserAchievement
	result := database.DB.Where("achievement_id = ?", achievement.ID).First(&unlocked)
	if result.Error == nil {
		utils.SuccessMsg(c, "已解锁")
		return
	}

	now := time.Now()
	database.DB.Create(&models.UserAchievement{
		AchievementID: achievement.ID,
		UnlockedAt:    now,
	})
	utils.SuccessMsg(c, "解锁成功")
}

func ToggleAchievement(c *gin.Context) {
	id := c.Param("id")
	var achievement models.Achievement
	if err := database.DB.First(&achievement, id).Error; err != nil {
		utils.NotFound(c, "成就不存在")
		return
	}

	var unlocked models.UserAchievement
	result := database.DB.Where("achievement_id = ?", achievement.ID).First(&unlocked)
	if result.Error == nil {
		database.DB.Delete(&unlocked)
		utils.SuccessMsg(c, "已关闭")
		return
	}

	now := time.Now()
	database.DB.Create(&models.UserAchievement{
		AchievementID: achievement.ID,
		UnlockedAt:    now,
	})
	utils.SuccessMsg(c, "已激活")
}

type CreateAchievementRequest struct {
	Code        string `json:"code" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description" binding:"required"`
	Icon        string `json:"icon"`
	Condition   string `json:"condition"`
}

func CreateAchievement(c *gin.Context) {
	var req CreateAchievementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "成就名称和描述不能为空")
		return
	}
	if req.Condition == "" {
		req.Condition = "manual"
	}

	achievement := models.Achievement{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		Icon:        req.Icon,
		Condition:   req.Condition,
	}
	database.DB.Create(&achievement)
	utils.Success(c, achievement)
}

func UpdateAchievement(c *gin.Context) {
	id := c.Param("id")
	var achievement models.Achievement
	if err := database.DB.First(&achievement, id).Error; err != nil {
		utils.NotFound(c, "成就不存在")
		return
	}

	var req CreateAchievementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}

	achievement.Code = req.Code
	achievement.Name = req.Name
	achievement.Description = req.Description
	achievement.Icon = req.Icon
	achievement.Condition = req.Condition
	if achievement.Condition == "" {
		achievement.Condition = "manual"
	}
	database.DB.Save(&achievement)
	utils.Success(c, achievement)
}

func DeleteAchievement(c *gin.Context) {
	id := c.Param("id")
	database.DB.Where("achievement_id = ?", id).Delete(&models.UserAchievement{})
	database.DB.Delete(&models.Achievement{}, id)
	utils.SuccessMsg(c, "删除成功")
}

func GetAppInfo(c *gin.Context) {
	var setting models.Setting
	appName := "NiniMenu"
	if err := database.DB.Where("`key` = ?", "app_name").First(&setting).Error; err == nil {
		if v := strings.TrimSpace(setting.Value); v != "" {
			appName = v
		}
	}
	utils.Success(c, gin.H{"app_name": appName})
}

func GetSettings(c *gin.Context) {
	var settings []models.Setting
	database.DB.Find(&settings)

	// 值如果是合法的 JSON 数组/对象（如 categories、tastes），内联成真正的 JSON，
	// 避免前端拿到被转义的字符串；其余保持原字符串。
	// week_plan_cache 是后端内部缓存（体积大、前端从 /week-plan 获取），不对外暴露。
	m := make(map[string]any)
	for _, s := range settings {
		if s.Key == "admin_password" || s.Key == "app_password" || s.Key == "week_plan_cache" {
			continue
		}
		v := strings.TrimSpace(s.Value)
		if (strings.HasPrefix(v, "[") || strings.HasPrefix(v, "{")) && json.Valid([]byte(v)) {
			m[s.Key] = json.RawMessage(v)
		} else {
			m[s.Key] = s.Value
		}
	}

	utils.Success(c, m)
}

type UpdateSettingsRequest struct {
	Settings map[string]string `json:"settings" binding:"required"`
}

func UpdateSettings(c *gin.Context) {
	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请提供设置数据")
		return
	}

	shouldInvalidateWeekPlan := false
	for key, value := range req.Settings {
		if key == "admin_password" || key == "app_password" {
			continue
		}
		if key == "lunch_dishes_per_day" || key == "dinner_dishes_per_day" {
			shouldInvalidateWeekPlan = true
		}
		var existing models.Setting
		if err := database.DB.Where("`key` = ?", key).First(&existing).Error; err == nil {
			database.DB.Model(&models.Setting{}).Where("`key` = ?", key).Update("value", value)
		} else {
			database.DB.Create(&models.Setting{Key: key, Value: value})
		}
	}
	if shouldInvalidateWeekPlan {
		services.InvalidateWeekPlanCache()
	}

	utils.SuccessMsg(c, "更新成功")
}

func GetDashboard(c *gin.Context) {
	var totalDishes int64
	database.DB.Model(&models.Dish{}).Count(&totalDishes)

	var enabledDishes int64
	database.DB.Model(&models.Dish{}).Where("enabled = ?", true).Count(&enabledDishes)

	var totalRecords int64
	database.DB.Model(&models.MealRecord{}).Count(&totalRecords)

	var todayRecords int64
	database.DB.Model(&models.MealRecord{}).Where("meal_date = ?", todayStr()).Count(&todayRecords)

	var favoriteCount int64
	database.DB.Model(&models.Favorite{}).Count(&favoriteCount)

	var disabledDishes int64
	database.DB.Model(&models.Dish{}).Where("enabled = ?", false).Count(&disabledDishes)

	type CategoryCount struct {
		Category string `json:"category"`
		Count    int64  `json:"count"`
	}
	var categoryCounts []CategoryCount
	database.DB.Model(&models.Dish{}).Select("category, count(*) as count").Where("enabled = ?", true).Group("category").Order("count DESC").Find(&categoryCounts)

	type TopDish struct {
		DishID   uint   `json:"dish_id"`
		DishName string `json:"dish_name"`
		Count    int64  `json:"count"`
	}
	var topDishes []TopDish
	database.DB.Model(&models.MealRecord{}).
		Select("dish_id, dish_name, count(*) as count").
		Group("dish_id, dish_name").
		Order("count DESC").
		Limit(8).
		Find(&topDishes)

	type RecentRecord struct {
		ID       uint   `json:"id"`
		DishID   uint   `json:"dish_id"`
		DishName string `json:"dish_name"`
		MealType string `json:"meal_type"`
		MealDate string `json:"meal_date"`
		Mood     string `json:"mood"`
		Rating   int    `json:"rating"`
	}
	var recentRecords []RecentRecord
	database.DB.Model(&models.MealRecord{}).
		Select("id, dish_id, dish_name, meal_type, meal_date, mood, rating").
		Order("meal_date DESC, created_at DESC").
		Limit(10).
		Find(&recentRecords)

	type DayCount struct {
		Date  string `json:"date"`
		Count int64  `json:"count"`
	}
	var weekTrend []DayCount
	for i := 6; i >= 0; i-- {
		d := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		var cnt int64
		database.DB.Model(&models.MealRecord{}).Where("meal_date = ?", d).Count(&cnt)
		weekTrend = append(weekTrend, DayCount{Date: d, Count: cnt})
	}

	type DifficultyCount struct {
		Difficulty string `json:"difficulty"`
		Count      int64  `json:"count"`
	}
	var difficultyCounts []DifficultyCount
	database.DB.Model(&models.Dish{}).Select("difficulty, count(*) as count").Where("enabled = ?", true).Group("difficulty").Find(&difficultyCounts)

	utils.Success(c, gin.H{
		"total_dishes":      totalDishes,
		"enabled_dishes":    enabledDishes,
		"disabled_dishes":   disabledDishes,
		"total_records":     totalRecords,
		"today_records":     todayRecords,
		"favorite_count":    favoriteCount,
		"category_counts":   categoryCounts,
		"top_dishes":        topDishes,
		"recent_records":    recentRecords,
		"week_trend":        weekTrend,
		"difficulty_counts": difficultyCounts,
	})
}

func todayStr() string {
	return time.Now().Format("2006-01-02")
}
