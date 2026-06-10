package handlers

import (
	"encoding/json"
	"math/rand"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func GetDishes(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	// pageSize=0 disables pagination: the week-plan/tomorrow dish pickers
	// need the full eligible list so their client-side search and category
	// chips cover every dish, not just the first page.
	fetchAll := pageSize == 0
	if !fetchAll && (pageSize < 1 || pageSize > 100) {
		pageSize = 20
	}

	query := database.DB.Model(&models.Dish{})

	if category := c.Query("category"); category != "" {
		query = query.Where("category = ?", category)
	}
	if mealType := c.Query("meal_type"); mealType != "" {
		query = query.Where("meal_type IN ?", []string{mealType, "all"})
	}
	if search := c.Query("search"); search != "" {
		query = query.Where("name LIKE ?", "%"+search+"%")
	}
	if enabled := c.Query("enabled"); enabled != "" {
		query = query.Where("enabled = ?", enabled == "1" || enabled == "true")
	}
	if taste := c.Query("taste"); taste != "" {
		query = query.Where("taste LIKE ?", "%"+taste+"%")
	}
	if difficulty := c.Query("difficulty"); difficulty != "" {
		query = query.Where("difficulty = ?", difficulty)
	}

	sort := c.DefaultQuery("sort", "created_at")
	order := c.DefaultQuery("order", "desc")
	isRandom := sort == "random"
	allowedSorts := map[string]bool{"created_at": true, "name": true, "cook_time": true, "difficulty": true, "sort_order": true, "random": true}
	if !allowedSorts[sort] {
		sort = "created_at"
	}
	if !isRandom {
		if order != "asc" && order != "desc" {
			order = "desc"
		}
		query = query.Order(sort + " " + order)
	}

	var total int64
	query.Count(&total)

	var dishes []models.Dish
	if isRandom {
		query.Find(&dishes)
		rand.Shuffle(len(dishes), func(i, j int) { dishes[i], dishes[j] = dishes[j], dishes[i] })
		if !fetchAll && len(dishes) > pageSize {
			dishes = dishes[:pageSize]
		}
	} else if fetchAll {
		query.Find(&dishes)
	} else {
		offset := (page - 1) * pageSize
		query.Offset(offset).Limit(pageSize).Find(&dishes)
	}

	utils.SuccessPaginated(c, dishes, total, page, pageSize)
}

func GetDish(c *gin.Context) {
	id := c.Param("id")
	var dish models.Dish
	if err := database.DB.First(&dish, id).Error; err != nil {
		utils.NotFound(c, "菜品不存在")
		return
	}
	utils.Success(c, dish)
}

func GetDishTraitAudit(c *gin.Context) {
	report, err := services.BuildDishTraitAudit()
	if err != nil {
		utils.InternalError(c, "菜品画像体检失败")
		return
	}
	utils.Success(c, report)
}

type CreateDishRequest struct {
	Name               string `json:"name" binding:"required"`
	ImageURL           string `json:"image_url"`
	Images             string `json:"images"`
	VideoURL           string `json:"video_url"`
	Category           string `json:"category"`
	MealType           string `json:"meal_type"`
	Taste              string `json:"taste"`
	Ingredients        string `json:"ingredients"`
	Seasonings         string `json:"seasonings"`
	Steps              string `json:"steps"`
	DishRole           string `json:"dish_role"`
	ProteinSources     string `json:"protein_sources"`
	ServingTemperature string `json:"serving_temperature"`
	CookingMethods     string `json:"cooking_methods"`
	SpiceLevel         int    `json:"spice_level"`
	RichnessLevel      int    `json:"richness_level"`
	CarbLevel          int    `json:"carb_level"`
	TraitSource        string `json:"trait_source"`
	TraitVersion       int    `json:"trait_version"`
	CookTime           int    `json:"cook_time"`
	Difficulty         string `json:"difficulty"`
	Remark             string `json:"remark"`
	Tags               string `json:"tags"`
	SortOrder          int    `json:"sort_order"`
}

func CreateDish(c *gin.Context) {
	var req CreateDishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "菜名不能为空")
		return
	}

	dish := models.Dish{
		Name:               req.Name,
		ImageURL:           req.ImageURL,
		Images:             req.Images,
		VideoURL:           req.VideoURL,
		Category:           req.Category,
		MealType:           req.MealType,
		Taste:              req.Taste,
		Ingredients:        req.Ingredients,
		Seasonings:         req.Seasonings,
		Steps:              req.Steps,
		DishRole:           req.DishRole,
		ProteinSources:     req.ProteinSources,
		ServingTemperature: req.ServingTemperature,
		CookingMethods:     req.CookingMethods,
		SpiceLevel:         req.SpiceLevel,
		RichnessLevel:      req.RichnessLevel,
		CarbLevel:          req.CarbLevel,
		TraitSource:        req.TraitSource,
		TraitVersion:       req.TraitVersion,
		CookTime:           req.CookTime,
		Difficulty:         req.Difficulty,
		Remark:             req.Remark,
		Tags:               req.Tags,
		SortOrder:          req.SortOrder,
		Enabled:            true,
	}

	if dish.MealType == "" {
		dish.MealType = "all"
	}
	if dish.Difficulty == "" {
		dish.Difficulty = "easy"
	}
	if dish.Images == "" {
		dish.Images = "[]"
	}
	if dish.Ingredients == "" {
		dish.Ingredients = "[]"
	}
	if dish.Seasonings == "" {
		dish.Seasonings = "[]"
	}
	if dish.Steps == "" {
		dish.Steps = "[]"
	}
	if dish.Tags == "" {
		dish.Tags = "[]"
	}
	dish = services.ApplyDishTraitsForSave(dish, requestHasDishTraits(req))

	if err := database.DB.Create(&dish).Error; err != nil {
		utils.InternalError(c, "创建菜品失败")
		return
	}

	services.QueueAutoAchievementSync()
	utils.Success(c, dish)
}

func UpdateDish(c *gin.Context) {
	id := c.Param("id")
	var dish models.Dish
	if err := database.DB.First(&dish, id).Error; err != nil {
		utils.NotFound(c, "菜品不存在")
		return
	}

	var req CreateDishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "请求数据无效")
		return
	}

	dish.Name = req.Name
	dish.ImageURL = req.ImageURL
	dish.Images = req.Images
	dish.VideoURL = req.VideoURL
	dish.Category = req.Category
	dish.MealType = req.MealType
	dish.Taste = req.Taste
	dish.Ingredients = req.Ingredients
	dish.Seasonings = req.Seasonings
	dish.Steps = req.Steps
	dish.CookTime = req.CookTime
	dish.Difficulty = req.Difficulty
	dish.Remark = req.Remark
	dish.Tags = req.Tags
	dish.SortOrder = req.SortOrder
	if requestHasDishTraits(req) {
		dish.DishRole = req.DishRole
		dish.ProteinSources = req.ProteinSources
		dish.ServingTemperature = req.ServingTemperature
		dish.CookingMethods = req.CookingMethods
		dish.SpiceLevel = req.SpiceLevel
		dish.RichnessLevel = req.RichnessLevel
		dish.CarbLevel = req.CarbLevel
		dish.TraitSource = req.TraitSource
		dish.TraitVersion = req.TraitVersion
		dish = services.ApplyDishTraitsForSave(dish, true)
	} else if dish.TraitSource != "manual" {
		dish = services.ApplyDishTraitsForSave(dish, false)
	}

	if err := database.DB.Save(&dish).Error; err != nil {
		utils.InternalError(c, "更新菜品失败")
		return
	}

	services.QueueAutoAchievementSync()
	utils.Success(c, dish)
}

func requestHasDishTraits(req CreateDishRequest) bool {
	return req.DishRole != "" ||
		req.ProteinSources != "" ||
		req.ServingTemperature != "" ||
		req.CookingMethods != "" ||
		req.SpiceLevel != 0 ||
		req.RichnessLevel != 0 ||
		req.CarbLevel != 0 ||
		req.TraitSource == "manual"
}

func DeleteDish(c *gin.Context) {
	id := c.Param("id")
	if err := database.DB.Delete(&models.Dish{}, id).Error; err != nil {
		utils.NotFound(c, "菜品不存在")
		return
	}
	services.QueueAutoAchievementSync()
	utils.SuccessMsg(c, "删除成功")
}

func ToggleDish(c *gin.Context) {
	id := c.Param("id")
	var dish models.Dish
	if err := database.DB.First(&dish, id).Error; err != nil {
		utils.NotFound(c, "菜品不存在")
		return
	}
	dish.Enabled = !dish.Enabled
	database.DB.Save(&dish)
	services.QueueAutoAchievementSync()
	utils.Success(c, dish)
}

func CloneDish(c *gin.Context) {
	id := c.Param("id")
	var dish models.Dish
	if err := database.DB.First(&dish, id).Error; err != nil {
		utils.NotFound(c, "菜品不存在")
		return
	}

	newDish := dish
	newDish.ID = 0
	newDish.Name = dish.Name + " (副本)"
	if err := database.DB.Create(&newDish).Error; err != nil {
		utils.InternalError(c, "克隆菜品失败")
		return
	}

	services.QueueAutoAchievementSync()
	utils.Success(c, newDish)
}

type CategoryCount struct {
	Category string `json:"category"`
	Count    int64  `json:"count"`
}

func GetDishCategoryCounts(c *gin.Context) {
	var counts []CategoryCount
	database.DB.Model(&models.Dish{}).
		Select("category, count(*) as count").
		Where("enabled = ? AND deleted_at IS NULL", true).
		Group("category").
		Order("count DESC").
		Find(&counts)

	var total int64
	database.DB.Model(&models.Dish{}).Where("enabled = ? AND deleted_at IS NULL", true).Count(&total)

	utils.Success(c, gin.H{
		"total":      total,
		"categories": counts,
	})
}

type BatchToggleRequest struct {
	IDs     []uint `json:"ids" binding:"required"`
	Enabled bool   `json:"enabled"`
}

func BatchToggleDishes(c *gin.Context) {
	var req BatchToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		utils.BadRequest(c, "请选择菜品")
		return
	}
	database.DB.Model(&models.Dish{}).Where("id IN ?", req.IDs).Update("enabled", req.Enabled)
	services.QueueAutoAchievementSync()
	utils.SuccessMsg(c, "批量操作成功")
}

type BatchDeleteRequest struct {
	IDs []uint `json:"ids" binding:"required"`
}

func BatchDeleteDishes(c *gin.Context) {
	var req BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		utils.BadRequest(c, "请选择菜品")
		return
	}
	database.DB.Where("id IN ?", req.IDs).Delete(&models.Dish{})
	services.QueueAutoAchievementSync()
	utils.SuccessMsg(c, "批量删除成功")
}

type BatchCategoryRequest struct {
	IDs      []uint `json:"ids" binding:"required"`
	Category string `json:"category" binding:"required"`
}

func BatchUpdateCategory(c *gin.Context) {
	var req BatchCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		utils.BadRequest(c, "请选择菜品并指定分类")
		return
	}
	database.DB.Model(&models.Dish{}).Where("id IN ?", req.IDs).Update("category", req.Category)
	utils.SuccessMsg(c, "批量修改分类成功")
}

type DishRecordWithDay struct {
	ID        uint     `json:"id"`
	MealType  string   `json:"meal_type"`
	MealDate  string   `json:"meal_date"`
	Rating    int      `json:"rating"`
	Remark    string   `json:"remark"`
	Mood      string   `json:"mood"`
	Photo     string   `json:"photo"`
	HomeMood  string   `json:"home_mood"`
	DayMood   string   `json:"day_mood"`
	DayRemark string   `json:"day_remark"`
	Photos    []string `json:"photos"`
}

type DishRecordsStats struct {
	TotalCount  int     `json:"total_count"`
	LunchCount  int     `json:"lunch_count"`
	DinnerCount int     `json:"dinner_count"`
	YumPercent  int     `json:"yum_percent"`
	OkPercent   int     `json:"ok_percent"`
	NoPercent   int     `json:"no_percent"`
	AvgRating   float64 `json:"avg_rating"`
	LastDate    string  `json:"last_date"`
	AvgInterval int     `json:"avg_interval"`
}

type DishRecordsResponse struct {
	Records []DishRecordWithDay `json:"records"`
	Stats   DishRecordsStats    `json:"stats"`
}

func GetDishRecords(c *gin.Context) {
	id := c.Param("id")
	var dish models.Dish
	if err := database.DB.First(&dish, id).Error; err != nil {
		utils.NotFound(c, "菜品不存在")
		return
	}

	var records []models.MealRecord
	database.DB.Where("dish_id = ?", id).Order("meal_date DESC, created_at DESC").Find(&records)

	if len(records) == 0 {
		utils.Success(c, DishRecordsResponse{
			Records: []DishRecordWithDay{},
			Stats:   DishRecordsStats{},
		})
		return
	}

	dates := make([]string, 0, len(records))
	for _, r := range records {
		dates = append(dates, r.MealDate)
	}

	dayRatingMap := make(map[string]models.DayRating)
	if len(dates) > 0 {
		var dayRatings []models.DayRating
		database.DB.Where("meal_date IN ?", dates).Find(&dayRatings)
		for _, dr := range dayRatings {
			dayRatingMap[dr.MealDate] = dr
		}
	}

	result := make([]DishRecordWithDay, 0, len(records))
	yumCount := 0
	okCount := 0
	noCount := 0
	totalRating := 0
	ratingCount := 0
	lunchCount := 0

	for _, r := range records {
		var photos []string
		var homeMood, dayMood, dayRemark string
		if dr, ok := dayRatingMap[r.MealDate]; ok {
			homeMood = dr.HomeMood
			dayMood = dr.Mood
			dayRemark = dr.Remark
			if dr.Photos != "" {
				json.Unmarshal([]byte(dr.Photos), &photos)
			}
		}
		if photos == nil {
			photos = []string{}
		}

		switch r.Mood {
		case "yum", "great":
			yumCount++
		case "ok":
			okCount++
		case "no", "meh":
			noCount++
		}
		if r.Rating > 0 {
			totalRating += r.Rating
			ratingCount++
		}
		if r.MealType == "lunch" {
			lunchCount++
		}

		result = append(result, DishRecordWithDay{
			ID:        r.ID,
			MealType:  r.MealType,
			MealDate:  r.MealDate,
			Rating:    r.Rating,
			Remark:    r.Remark,
			Mood:      r.Mood,
			Photo:     r.Photo,
			HomeMood:  homeMood,
			DayMood:   dayMood,
			DayRemark: dayRemark,
			Photos:    photos,
		})
	}

	total := len(records)
	yumPct := 0
	okPct := 0
	noPct := 0
	moodTotal := yumCount + okCount + noCount
	if moodTotal > 0 {
		yumPct = yumCount * 100 / moodTotal
		okPct = okCount * 100 / moodTotal
		noPct = noCount * 100 / moodTotal
	}

	avgRating := 0.0
	if ratingCount > 0 {
		avgRating = float64(totalRating) / float64(ratingCount)
	}

	lastDate := records[0].MealDate

	var avgInterval int
	if total >= 2 {
		sorted := make([]string, len(records))
		for i, r := range records {
			sorted[i] = r.MealDate
		}
		totalDays := 0
		for i := 0; i < len(sorted)-1; i++ {
			d1, e1 := time.Parse("2006-01-02", sorted[i])
			d2, e2 := time.Parse("2006-01-02", sorted[i+1])
			if e1 == nil && e2 == nil {
				diff := int(d1.Sub(d2).Hours() / 24)
				if diff > 0 {
					totalDays += diff
				}
			}
		}
		avgInterval = totalDays / (len(sorted) - 1)
	}

	utils.Success(c, DishRecordsResponse{
		Records: result,
		Stats: DishRecordsStats{
			TotalCount:  total,
			LunchCount:  lunchCount,
			DinnerCount: total - lunchCount,
			YumPercent:  yumPct,
			OkPercent:   okPct,
			NoPercent:   noPct,
			AvgRating:   avgRating,
			LastDate:    lastDate,
			AvgInterval: avgInterval,
		},
	})
}
