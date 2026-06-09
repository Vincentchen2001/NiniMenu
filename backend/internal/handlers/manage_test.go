package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupManageHandlerTestDB(t *testing.T) {
	t.Helper()

	originalDB := database.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Achievement{},
		&models.UserAchievement{},
		&models.Setting{},
	); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	database.DB = db
	t.Cleanup(func() {
		database.DB = originalDB
	})
}

func TestGetAchievementsTreatsUnlockedAchievementsAsLockedWhenDisabled(t *testing.T) {
	setupManageHandlerTestDB(t)
	gin.SetMode(gin.TestMode)

	achievement := models.Achievement{
		Code:        "first_pick",
		Name:        "第一次记录",
		Description: "记录一次吃饭",
		Condition:   "auto",
	}
	if err := database.DB.Create(&achievement).Error; err != nil {
		t.Fatalf("create achievement: %v", err)
	}
	if err := database.DB.Create(&models.UserAchievement{
		AchievementID: achievement.ID,
		UnlockedAt:    time.Now(),
	}).Error; err != nil {
		t.Fatalf("create user achievement: %v", err)
	}
	if err := database.DB.Create(&models.Setting{Key: "achievements_enabled", Value: "0"}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/achievements", nil)

	GetAchievements(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var body struct {
		Code int `json:"code"`
		Data []struct {
			Code       string `json:"code"`
			IsUnlocked bool   `json:"is_unlocked"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v; body: %s", err, recorder.Body.String())
	}
	if len(body.Data) != 1 {
		t.Fatalf("achievement count = %d, want 1", len(body.Data))
	}
	if body.Data[0].IsUnlocked {
		t.Fatalf("%s should be locked when achievements_enabled is 0", body.Data[0].Code)
	}
}
