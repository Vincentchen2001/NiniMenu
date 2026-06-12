package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ninimenu/internal/database"
	"ninimenu/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestValidateWeekPlanRuleHandlerAcceptsValidExpression(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/week-plan/rules/validate", strings.NewReader(`{
		"code":"test_rule",
		"name":"测试规则",
		"enabled":true,
		"scope":"meal",
		"rule_kind":"constraint",
		"severity":"hard",
		"relaxable":false,
		"expression":"!has(candidate.protein_sources, \"egg\") || countMeal(\"protein_sources\", \"egg\") == 0",
		"priority":10,
		"message":"同餐蛋类已达到上限"
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ValidateWeekPlanRuleHandler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

func TestValidateWeekPlanRuleHandlerRejectsBadExpression(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/week-plan/rules/validate", strings.NewReader(`{
		"code":"bad_rule",
		"name":"错误规则",
		"enabled":true,
		"scope":"meal",
		"rule_kind":"constraint",
		"severity":"hard",
		"relaxable":false,
		"expression":"unknown_value > 0",
		"priority":10,
		"message":""
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ValidateWeekPlanRuleHandler(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func setupPlanHandlerTestDB(t *testing.T) {
	t.Helper()

	originalDB := database.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Dish{},
		&models.Setting{},
		&models.MealRecord{},
		&models.DishRecommendation{},
		&models.WeekPlanRecord{},
		&models.MenuRule{},
		&models.Achievement{},
		&models.UserAchievement{},
		&models.AchievementEvent{},
	); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	database.DB = db
	t.Cleanup(func() {
		database.DB = originalDB
	})
}

// week=foo must return 400.
func TestResolveWeekParamRejectsUnknownValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/week-plan?week=foo", nil)

	GetWeekPlan(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

// week=next on a fresh DB (no snapshot) must return 200 with an empty days list.
func TestGetWeekPlanNextWeekReturnEmptyOnFreshDB(t *testing.T) {
	setupPlanHandlerTestDB(t)
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/week-plan?week=next", nil)

	GetWeekPlan(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var body struct {
		Data struct {
			Days []interface{} `json:"days"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v; body: %s", err, recorder.Body.String())
	}
	if len(body.Data.Days) != 0 {
		t.Fatalf("expected empty days for next week on fresh DB, got %d days", len(body.Data.Days))
	}
}
