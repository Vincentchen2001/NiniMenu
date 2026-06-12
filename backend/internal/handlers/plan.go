package handlers

import (
	"errors"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/utils"
	"time"

	"github.com/gin-gonic/gin"
)

func GetWeekPlan(c *gin.Context) {
	plan := services.GetCachedWeekPlan()
	if plan != nil && len(plan.Days) > 0 {
		services.RecordUniqueAchievementEvent("week_plan", plan.Days[0].Date)
	}
	utils.Success(c, plan)
}

func RegenerateWeekPlanHandler(c *gin.Context) {
	plan := services.RegenerateWeekPlan()
	services.RecordAchievementEvent("week_plan", "")
	utils.Success(c, plan)
}

type RegenerateWeekPlanDayRequest struct {
	Date string `json:"date" binding:"required"`
}

func RegenerateWeekPlanDayHandler(c *gin.Context) {
	var req RegenerateWeekPlanDayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数无效")
		return
	}
	plan, err := services.RegenerateWeekPlanDay(req.Date)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	services.RecordAchievementEvent("week_plan", "")
	utils.Success(c, plan)
}

func GetWeekPlanHistoryHandler(c *gin.Context) {
	history, err := services.WeekPlanHistoryByMonth(c.Query("month"))
	if errors.Is(err, services.ErrInvalidHistoryMonth) {
		utils.BadRequest(c, err.Error())
		return
	}
	if err != nil {
		utils.InternalError(c, "读取菜单历史失败")
		return
	}
	utils.Success(c, history)
}

func SaveWeekPlanHandler(c *gin.Context) {
	var plan services.WeekPlan
	if err := c.ShouldBindJSON(&plan); err != nil {
		utils.BadRequest(c, "参数无效")
		return
	}
	if err := services.SaveWeekPlan(&plan); err != nil {
		utils.InternalError(c, "保存菜单失败")
		return
	}
	utils.Success(c, &plan)
}

func GetWeekPlanPreferencesHandler(c *gin.Context) {
	utils.Success(c, services.GetWeekPlanPreferences())
}

func UpdateWeekPlanPreferencesHandler(c *gin.Context) {
	var prefs services.WeekPlanPreferences
	if err := c.ShouldBindJSON(&prefs); err != nil {
		utils.BadRequest(c, "参数无效")
		return
	}
	if err := services.SaveWeekPlanPreferences(prefs); err != nil {
		utils.InternalError(c, "保存设置失败")
		return
	}
	utils.Success(c, services.GetWeekPlanPreferences())
}

func GetWeekPlanRulesHandler(c *gin.Context) {
	rules, err := services.ListMenuRules()
	if err != nil {
		utils.InternalError(c, "读取推荐规则失败")
		return
	}
	utils.Success(c, rules)
}

type UpdateWeekPlanRulesRequest struct {
	Rules []models.MenuRule `json:"rules" binding:"required"`
}

func UpdateWeekPlanRulesHandler(c *gin.Context) {
	var req UpdateWeekPlanRulesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数无效")
		return
	}
	rules, err := services.SaveMenuRules(req.Rules)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, rules)
}

func ValidateWeekPlanRuleHandler(c *gin.Context) {
	var rule models.MenuRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		utils.BadRequest(c, "参数无效")
		return
	}
	if err := services.ValidateMenuRuleExpression(rule); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.SuccessMsg(c, "规则表达式有效")
}

func GetShoppingList(c *gin.Context) {
	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	dates := []string{today, tomorrow}

	list := services.BuildShoppingList(dates)
	utils.Success(c, list)
}

type ToggleShoppingCheckRequest struct {
	ItemName string `json:"item_name" binding:"required"`
	MealDate string `json:"meal_date" binding:"required"`
	Checked  bool   `json:"checked"`
}

func ToggleShoppingCheckHandler(c *gin.Context) {
	var req ToggleShoppingCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数无效")
		return
	}
	services.ToggleShoppingCheck(req.ItemName, req.MealDate, req.Checked)
	if req.Checked {
		services.QueueAutoAchievementSync()
	}
	utils.SuccessMsg(c, "已更新")
}

type ToggleHomeInventoryRequest struct {
	ItemName string `json:"item_name" binding:"required"`
	InStock  bool   `json:"in_stock"`
}

func ToggleHomeInventoryHandler(c *gin.Context) {
	var req ToggleHomeInventoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数无效")
		return
	}
	services.ToggleHomeInventory(req.ItemName, req.InStock)
	services.QueueAutoAchievementSync()
	utils.SuccessMsg(c, "已更新")
}

func GetShoppingCategories(c *gin.Context) {
	utils.Success(c, services.ListShoppingCategoryOverrides())
}

type UpsertShoppingCategoryRequest struct {
	ItemName string `json:"item_name" binding:"required"`
	Category string `json:"category" binding:"required"`
}

func UpsertShoppingCategoryHandler(c *gin.Context) {
	var req UpsertShoppingCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数无效")
		return
	}
	if !services.UpsertShoppingCategoryOverride(req.ItemName, req.Category) {
		utils.BadRequest(c, "分类无效")
		return
	}
	utils.SuccessMsg(c, "已更新")
}

func DeleteShoppingCategoryHandler(c *gin.Context) {
	itemName := c.Param("itemName")
	services.DeleteShoppingCategoryOverride(itemName)
	utils.SuccessMsg(c, "已删除")
}

func GetUpcomingHolidays(c *gin.Context) {
	var holidays []models.Holiday
	now := time.Now().Format("2006-01-02")
	database.DB.Where("date >= ?", now).Order("date ASC").Limit(5).Find(&holidays)
	utils.Success(c, holidays)
}
