package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupPickHandlerTestDB(t *testing.T) {
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
		&models.Quote{},
		&models.BlindBox{},
		&models.Achievement{},
		&models.UserAchievement{},
		&models.AchievementEvent{},
		&models.MealRecord{},
		&models.DishRecommendation{},
	); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	database.DB = db
	t.Cleanup(func() {
		database.DB = originalDB
	})
}

func blockIngredientForPickTest(t *testing.T, word string) {
	t.Helper()
	if err := database.DB.Create(&models.Setting{
		Key:   "blocked_ingredients",
		Value: `["` + word + `"]`,
	}).Error; err != nil {
		t.Fatalf("create blocked_ingredients setting: %v", err)
	}
}

type pickDishesResponse struct {
	Data struct {
		Dishes []struct {
			Name string `json:"name"`
		} `json:"dishes"`
	} `json:"data"`
}

// 忌口 must hold on the mood shortcut branches too, not just PickDishes.
func TestPickMoodExcludesBlockedDishes(t *testing.T) {
	setupPickHandlerTestDB(t)
	gin.SetMode(gin.TestMode)

	for _, dish := range []models.Dish{
		{Name: "香菜拌牛肉", MealType: "all", Difficulty: "easy", Enabled: true},
		{Name: "凉拌黄瓜", MealType: "all", Difficulty: "easy", Enabled: true},
	} {
		if err := database.DB.Create(&dish).Error; err != nil {
			t.Fatalf("create dish: %v", err)
		}
	}
	blockIngredientForPickTest(t, "香菜")

	body, _ := json.Marshal(map[string]string{"mood": "tired"})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/pick/mood", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	PickMood(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d (%s)", recorder.Code, recorder.Body.String())
	}
	var resp pickDishesResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Data.Dishes) == 0 {
		t.Fatalf("expected at least one dish")
	}
	for _, dish := range resp.Data.Dishes {
		if dish.Name == "香菜拌牛肉" {
			t.Errorf("mood pick returned blocked dish %s", dish.Name)
		}
	}
}

// When every dish is blocked, the blind box must come up empty rather than
// recommend a blocked dish.
func TestPickBlindBoxSkipsBlockedDishes(t *testing.T) {
	setupPickHandlerTestDB(t)
	gin.SetMode(gin.TestMode)

	if err := database.DB.Create(&models.Dish{
		Name: "香菜牛肉羹", MealType: "all", Enabled: true,
	}).Error; err != nil {
		t.Fatalf("create dish: %v", err)
	}
	blockIngredientForPickTest(t, "香菜")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/pick/blind-box", nil)
	PickBlindBox(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("blind box with only blocked dishes should 404, got %d (%s)", recorder.Code, recorder.Body.String())
	}
}
