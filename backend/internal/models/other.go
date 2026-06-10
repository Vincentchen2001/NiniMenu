package models

import "time"

type Favorite struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	DishID    uint      `json:"dish_id" gorm:"not null;unique"`
	CreatedAt time.Time `json:"created_at"`
}

type Quote struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Content   string    `json:"content" gorm:"not null"`
	Scene     string    `json:"scene"`
	Enabled   bool      `json:"enabled" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
}

type Achievement struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	Code        string     `json:"code" gorm:"not null;unique"`
	Name        string     `json:"name" gorm:"not null"`
	Description string     `json:"description" gorm:"not null"`
	Icon        string     `json:"icon"`
	Condition   string     `json:"condition" gorm:"not null"`
	UnlockedAt  *time.Time `json:"unlocked_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

type UserAchievement struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	AchievementID uint      `json:"achievement_id" gorm:"not null;index"`
	UnlockedAt    time.Time `json:"unlocked_at"`
}

type AchievementEvent struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	EventType string    `json:"event_type" gorm:"not null;index"`
	RefKey    string    `json:"ref_key" gorm:"index"`
	CreatedAt time.Time `json:"created_at" gorm:"index"`
}

type BlindBox struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	DishID    uint      `json:"dish_id" gorm:"not null"`
	Hint      string    `json:"hint"`
	Active    bool      `json:"active" gorm:"default:true"`
	StartDate string    `json:"start_date"`
	EndDate   string    `json:"end_date"`
	CreatedAt time.Time `json:"created_at"`
}

type Holiday struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"not null"`
	Date      string    `json:"date" gorm:"not null"`
	DishIDs   string    `json:"dish_ids" gorm:"default:'[]'"`
	Greeting  string    `json:"greeting"`
	CreatedAt time.Time `json:"created_at"`
}

type Setting struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Key       string    `json:"key" gorm:"not null;unique"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DishRecommendation struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	DishID      uint      `json:"dish_id" gorm:"not null;index;index:idx_dish_recommendations_lookup,priority:3"`
	DishName    string    `json:"dish_name"`
	Source      string    `json:"source" gorm:"not null;index;index:idx_dish_recommendations_lookup,priority:1"`
	MealType    string    `json:"meal_type" gorm:"not null;index"`
	PlannedDate string    `json:"planned_date" gorm:"not null;index;index:idx_dish_recommendations_lookup,priority:2"`
	CreatedAt   time.Time `json:"created_at" gorm:"index"`
}

type MenuRule struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Code        string    `json:"code" gorm:"not null;uniqueIndex"`
	Name        string    `json:"name" gorm:"not null"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled" gorm:"default:true;index"`
	Scope       string    `json:"scope" gorm:"default:'meal';index"`
	RuleKind    string    `json:"rule_kind" gorm:"default:'constraint';index"`
	Severity    string    `json:"severity" gorm:"default:'hard';index"`
	Relaxable   bool      `json:"relaxable" gorm:"default:false;index"`
	Expression  string    `json:"expression" gorm:"not null"`
	Template    string    `json:"template"`
	Priority    int       `json:"priority" gorm:"default:0;index"`
	Message     string    `json:"message"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type DayRating struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	MealDate  string    `json:"meal_date" gorm:"not null;uniqueIndex"`
	HomeMood  string    `json:"home_mood" gorm:"index"`
	Mood      string    `json:"mood" gorm:"index"`
	Remark    string    `json:"remark"`
	Photos    string    `json:"photos" gorm:"default:'[]'"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ShoppingCheck struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	MealDate   string    `json:"meal_date" gorm:"not null;index;index:idx_shopping_checks_date_checked,priority:1"`
	MealType   string    `json:"meal_type" gorm:"not null"`
	DishID     uint      `json:"dish_id" gorm:"not null"`
	DishName   string    `json:"dish_name"`
	ItemName   string    `json:"item_name" gorm:"not null"`
	ItemAmount string    `json:"item_amount"`
	Checked    bool      `json:"checked" gorm:"default:false;index;index:idx_shopping_checks_date_checked,priority:2"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type HomeInventory struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	ItemName  string    `json:"item_name" gorm:"not null;uniqueIndex"`
	InStock   bool      `json:"in_stock" gorm:"default:true;index"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ShoppingItemCategory struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	ItemName  string    `json:"item_name" gorm:"not null;uniqueIndex"`
	Category  string    `json:"category" gorm:"not null"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
