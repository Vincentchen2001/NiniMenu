package handlers

import (
	"math/rand"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type PickRequest struct {
	Mood string `json:"mood"`
}

type TomorrowPickRequest struct {
	MealType   string `json:"meal_type"`
	Profile    string `json:"profile"`
	Count      int    `json:"count"`
	ExcludeIDs []uint `json:"exclude_ids"`
}

func PickLunch(c *gin.Context) {
	count := getPickCount(c)
	dishes, err := services.PickDishes("lunch", count, true)
	if err != nil || len(dishes) == 0 {
		utils.NotFound(c, "没有可推荐的菜品")
		return
	}
	services.RecordAchievementEvent("recommend_lunch", "")
	quote := services.GetRandomQuote("lunch")
	utils.Success(c, gin.H{
		"dishes": dishes,
		"quote":  quote,
	})
}

func PickDinner(c *gin.Context) {
	count := getPickCount(c)
	dishes, err := services.PickDishes("dinner", count, true)
	if err != nil || len(dishes) == 0 {
		utils.NotFound(c, "没有可推荐的菜品")
		return
	}
	services.RecordAchievementEvent("recommend_dinner", "")
	quote := services.GetRandomQuote("dinner")
	utils.Success(c, gin.H{
		"dishes": dishes,
		"quote":  quote,
	})
}

func PickMood(c *gin.Context) {
	var req PickRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请选择心情")
		return
	}

	count := 3

	switch req.Mood {
	case "tired", "lazy":
		var dishes []models.Dish
		database.DB.Where("enabled = ? AND difficulty = ?", true, "easy").Find(&dishes)
		dishes = services.FilterBlockedDishes(dishes)
		if len(dishes) > 0 {
			shuffled := shuffleDishes(dishes)
			if len(shuffled) > count {
				shuffled = shuffled[:count]
			}
			services.RecordAchievementEvent("recommend_mood", "")
			quote := services.GetRandomQuote("quick")
			utils.Success(c, gin.H{"dishes": shuffled, "quote": quote})
			return
		}
	case "spicy":
		var dishes []models.Dish
		database.DB.Where("enabled = ? AND taste = ?", true, "辣").Find(&dishes)
		dishes = services.FilterBlockedDishes(dishes)
		if len(dishes) > 0 {
			shuffled := shuffleDishes(dishes)
			if len(shuffled) > count {
				shuffled = shuffled[:count]
			}
			services.RecordAchievementEvent("recommend_mood", "")
			quote := services.GetRandomQuote("lunch")
			utils.Success(c, gin.H{"dishes": shuffled, "quote": quote})
			return
		}
	case "healthy":
		var dishes []models.Dish
		database.DB.Where("enabled = ? AND taste IN ?", true, []string{"清淡", "鲜"}).Find(&dishes)
		dishes = services.FilterBlockedDishes(dishes)
		if len(dishes) > 0 {
			shuffled := shuffleDishes(dishes)
			if len(shuffled) > count {
				shuffled = shuffled[:count]
			}
			services.RecordAchievementEvent("recommend_mood", "")
			quote := services.GetRandomQuote("dinner")
			utils.Success(c, gin.H{"dishes": shuffled, "quote": quote})
			return
		}
	}

	dishes, err := services.PickDishes("", count, true)
	if err != nil || len(dishes) == 0 {
		utils.NotFound(c, "没有可推荐的菜品")
		return
	}
	services.RecordAchievementEvent("recommend_mood", "")
	quote := services.GetRandomQuote("")
	utils.Success(c, gin.H{"dishes": dishes, "quote": quote})
}

func PickTomorrow(c *gin.Context) {
	var req TomorrowPickRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}

	dishes, err := services.PickTomorrowDishes(services.TomorrowPickOptions{
		MealType:   req.MealType,
		Profile:    req.Profile,
		Count:      req.Count,
		ExcludeIDs: req.ExcludeIDs,
	})
	if err != nil || len(dishes) == 0 {
		utils.NotFound(c, "没有可推荐的菜品")
		return
	}

	services.RecordAchievementEvent("recommend_mood", req.Profile)
	utils.Success(c, gin.H{
		"dishes": dishes,
		"quote":  services.GetRandomQuote(req.Profile),
	})
}

func PickBlindBox(c *gin.Context) {
	var dishes []models.Dish
	database.DB.Where("enabled = ?", true).Find(&dishes)
	dishes = services.FilterBlockedDishes(dishes)
	if len(dishes) == 0 {
		utils.NotFound(c, "没有可推荐的菜品")
		return
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	dish := dishes[r.Intn(len(dishes))]

	hint := "点我揭晓今日惊喜~"
	var box models.BlindBox
	if err := database.DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Where("active = ?", true).First(&box).Error; err == nil && box.Hint != "" {
		hint = box.Hint
	}

	services.RecordAchievementEvent("blind_box", "")
	utils.Success(c, gin.H{
		"hint":  hint,
		"dish":  dish,
		"quote": services.GetRandomQuote(""),
	})
}

func getPickCount(c *gin.Context) int {
	count, err := strconv.Atoi(c.DefaultQuery("count", "3"))
	if err != nil || count < 1 {
		count = 3
	}
	if count > 10 {
		count = 10
	}
	return count
}

func shuffleDishes(dishes []models.Dish) []models.Dish {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	shuffled := make([]models.Dish, len(dishes))
	copy(shuffled, dishes)
	r.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})
	return shuffled
}
