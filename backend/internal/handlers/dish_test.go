package handlers

import (
	"encoding/json"
	"fmt"
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

func setupDishHandlerTestDB(t *testing.T) {
	t.Helper()

	originalDB := database.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.Dish{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	database.DB = db
	t.Cleanup(func() {
		database.DB = originalDB
	})
}

type dishListResponse struct {
	Data struct {
		// Raw items: models.Dish marshals images as an array, so it cannot
		// round-trip through itself; the tests only count entries.
		Items []json.RawMessage `json:"items"`
		Total int64             `json:"total"`
	} `json:"data"`
}

func getDishesResponse(t *testing.T, rawQuery string) dishListResponse {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/dishes?"+rawQuery, nil)
	GetDishes(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	var resp dishListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp
}

func createTestDishes(t *testing.T, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		dish := models.Dish{
			Name:     fmt.Sprintf("测试菜%d", i),
			Category: "家常菜",
			MealType: "all",
			Enabled:  true,
		}
		if err := database.DB.Create(&dish).Error; err != nil {
			t.Fatalf("create dish: %v", err)
		}
	}
}

// The week-plan / tomorrow dish pickers need the full eligible list so their
// client-side search and category chips cover every dish, not just page one.
func TestGetDishesPageSizeZeroReturnsAllDishes(t *testing.T) {
	setupDishHandlerTestDB(t)
	gin.SetMode(gin.TestMode)
	createTestDishes(t, 150)

	resp := getDishesResponse(t, "enabled=true&pageSize=0")

	if len(resp.Data.Items) != 150 {
		t.Fatalf("pageSize=0 should return all 150 dishes, got %d", len(resp.Data.Items))
	}
	if resp.Data.Total != 150 {
		t.Fatalf("total should be 150, got %d", resp.Data.Total)
	}
}

func TestGetDishesOversizePageSizeStillClampsToDefault(t *testing.T) {
	setupDishHandlerTestDB(t)
	gin.SetMode(gin.TestMode)
	createTestDishes(t, 30)

	resp := getDishesResponse(t, "pageSize=500")

	if len(resp.Data.Items) != 20 {
		t.Fatalf("oversize pageSize should clamp to default 20 items, got %d", len(resp.Data.Items))
	}
	if resp.Data.Total != 30 {
		t.Fatalf("total should still count all 30 dishes, got %d", resp.Data.Total)
	}
}
