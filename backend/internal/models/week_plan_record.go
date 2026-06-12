package models

import "time"

// WeekPlanRecord persists one user's week plan as a per-week snapshot row.
// PlanJSON serializes services.WeekPlan; empty string means "this week's plan
// not generated yet" (a row may be created early to hold prefs only).
// PrefsJSON holds the week's one-off preferences (week_want + day overrides),
// which must NOT survive into the next week. Rows are kept forever — one row
// per week is negligible. UserID is reserved for the future multi-user
// sub-project and stays the constant 1 until then.
type WeekPlanRecord struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"not null;default:1;uniqueIndex:idx_week_plans_user_week,priority:1"`
	WeekStart string    `json:"week_start" gorm:"not null;uniqueIndex:idx_week_plans_user_week,priority:2"`
	PlanJSON  string    `json:"plan_json"`
	PrefsJSON string    `json:"prefs_json"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName keeps the table name in line with the spec (week_plans).
func (WeekPlanRecord) TableName() string {
	return "week_plans"
}
