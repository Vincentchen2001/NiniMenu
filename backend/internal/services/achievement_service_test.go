package services

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAchievementServiceTestDB(t *testing.T) {
	t.Helper()

	originalDB := database.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.AchievementEvent{},
		&models.Setting{},
	); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	database.DB = db
	cancelQueuedAutoAchievementSync()
	t.Cleanup(func() {
		cancelQueuedAutoAchievementSync()
		database.DB = originalDB
	})
}

func TestRecordAchievementEventDoesNotPersistWhenAchievementsDisabled(t *testing.T) {
	setupAchievementServiceTestDB(t)

	if err := database.DB.Create(&models.Setting{Key: "achievements_enabled", Value: "0"}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}

	RecordAchievementEvent("recommend_lunch", "")

	var count int64
	database.DB.Model(&models.AchievementEvent{}).Count(&count)
	if count != 0 {
		t.Fatalf("achievement event count = %d, want 0 when achievements are disabled", count)
	}
}
