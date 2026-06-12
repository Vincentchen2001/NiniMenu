package services

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"testing"
	"time"
)

func TestLastSeenDishDatesMergesAndTrims(t *testing.T) {
	setupPlanServiceTestDB(t)
	now := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC) // today=2026-06-08, since=2026-05-26

	// dish 1: 两表都有，取较近（推荐 06-05 > 打卡 06-01）
	mustCreate(t, &models.MealRecord{DishID: 1, DishName: "A", MealType: "dinner", MealDate: "2026-06-01"})
	mustCreate(t, &models.DishRecommendation{DishID: 1, DishName: "A", Source: "week_plan", MealType: "lunch", PlannedDate: "2026-06-05"})
	// dish 2: 只有未来推荐行（本周计划尾巴），必须排除
	mustCreate(t, &models.DishRecommendation{DishID: 2, DishName: "B", Source: "week_plan", MealType: "lunch", PlannedDate: "2026-06-10"})
	// dish 3: 打卡在窗口下界之前，必须排除
	mustCreate(t, &models.MealRecord{DishID: 3, DishName: "C", MealType: "lunch", MealDate: "2026-05-20"})
	// dish 4: 推荐行恰在窗口下界（today-13），必须包含
	mustCreate(t, &models.DishRecommendation{DishID: 4, DishName: "D", Source: "week_plan", MealType: "dinner", PlannedDate: "2026-05-26"})
	// dish 5: 打卡在今天当天，必须包含（d=0 语义）
	mustCreate(t, &models.MealRecord{DishID: 5, DishName: "E", MealType: "lunch", MealDate: "2026-06-08"})

	got := lastSeenDishDates(now, "2026-06-08")
	want := map[uint]string{1: "2026-06-05", 4: "2026-05-26", 5: "2026-06-08"}
	if len(got) != len(want) {
		t.Fatalf("lastSeenDishDates() = %v, want %v", got, want)
	}
	for id, date := range want {
		if got[id] != date {
			t.Errorf("lastSeenDishDates()[%d] = %q, want %q", id, got[id], date)
		}
	}
}

func TestDaysSinceFor(t *testing.T) {
	ref := time.Date(2026, 6, 8, 23, 30, 0, 0, time.UTC) // 时分秒不应影响整天差
	lastSeen := map[uint]string{
		1: "2026-06-03", // 5 天前
		2: "2026-06-08", // 今天
		3: "2026-06-09", // 未来 → -1
		4: "not-a-date", // 坏数据 → -1
	}
	got := daysSinceFor(ref, lastSeen)
	want := map[uint]int{1: 5, 2: 0, 3: -1, 4: -1}
	for id, d := range want {
		if got[id] != d {
			t.Errorf("daysSinceFor()[%d] = %d, want %d", id, got[id], d)
		}
	}
}

func mustCreate(t *testing.T, value any) {
	t.Helper()
	if err := database.DB.Create(value).Error; err != nil {
		t.Fatalf("create fixture %T: %v", value, err)
	}
}

func TestLastSeenCapExtendsToWeekStartMinusOne(t *testing.T) {
	setupPlanServiceTestDB(t)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) // Wednesday

	// Dish 1 planned for this Sunday — future relative to today.
	mustCreate(t, &models.DishRecommendation{DishID: 1, DishName: "周日菜", Source: "week_plan", MealType: "dinner", PlannedDate: "2026-06-14"})

	// Current week: cap = today → future tail invisible (unchanged behavior).
	if got := lastSeenDishDates(now, "2026-06-08"); got[1] != "" {
		t.Fatalf("current-week cap should exclude future rows, got %v", got)
	}
	// Next week: cap = 2026-06-14 (weekStart-1) → this week's tail counts.
	if got := lastSeenDishDates(now, "2026-06-15"); got[1] != "2026-06-14" {
		t.Fatalf("next-week cap should include this week's tail, got %v", got)
	}
}

func TestEvaluateScoreRulesInjectsDaysSinceLast(t *testing.T) {
	setupPlanServiceTestDB(t)
	rule := models.MenuRule{
		Code: "test_days_since", Enabled: true, Scope: "candidate",
		RuleKind: "score", Severity: "soft", Relaxable: true,
		Expression: `candidate.days_since_last >= 0 && candidate.days_since_last < 14 ? -(28.0 * (14 - candidate.days_since_last) / 14.0) : 0`,
	}
	compiled, err := compileMenuRule(rule)
	if err != nil {
		t.Fatalf("compileMenuRule() error = %v", err)
	}
	rules := []compiledMenuRule{compiled}
	dish := models.Dish{ID: 7, Name: "测试肉菜", Ingredients: "猪肉"}
	dayCtx := weekPlanDayContext{daysSince: map[uint]int{7: 7}}

	if got := evaluateScoreRules(dish, dayCtx, MealQuota{}, nil, nil, nil, rules, true); got != -14.0 {
		t.Errorf("days=7 score = %v, want -14.0", got) // 28*(14-7)/14
	}
	if got := evaluateScoreRules(models.Dish{ID: 8, Name: "无记录菜", Ingredients: "牛肉"}, dayCtx, MealQuota{}, nil, nil, nil, rules, true); got != 0 {
		t.Errorf("absent dish score = %v, want 0（env 默认 -1）", got)
	}
	if got := evaluateScoreRules(dish, dayCtx, MealQuota{}, nil, nil, nil, rules, false); got != 0 {
		t.Errorf("enforceSoft=false score = %v, want 0（第 5 级放宽失效）", got)
	}
	if got := evaluateScoreRules(dish, weekPlanDayContext{}, MealQuota{}, nil, nil, nil, rules, true); got != 0 {
		t.Errorf("nil daysSince map score = %v, want 0", got)
	}
}

func TestEvaluateConstraintRulesSeesDaysSinceLast(t *testing.T) {
	setupPlanServiceTestDB(t)
	rule := models.MenuRule{
		Code: "test_days_constraint", Enabled: true, Scope: "candidate",
		RuleKind: "constraint", Severity: "hard", Relaxable: false,
		Expression: `candidate.days_since_last != 3`,
	}
	compiled, err := compileMenuRule(rule)
	if err != nil {
		t.Fatalf("compileMenuRule() error = %v", err)
	}
	dish := models.Dish{ID: 9, Name: "约束肉菜", Ingredients: "鸡肉"}
	dayCtx := weekPlanDayContext{daysSince: map[uint]int{9: 3}}
	if allowed, _ := evaluateConstraintRules(dish, dayCtx, MealQuota{}, nil, nil, nil, []compiledMenuRule{compiled}, true); allowed {
		t.Error("d=3 的菜应被该约束拒绝（注入未生效）")
	}
}
