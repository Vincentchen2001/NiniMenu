# 新鲜度衰减降分 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 出现过的菜在其后 14 天内背一个随时间线性消退的降分（出厂 -30 × (14−d)/14），作为规则页第 10 条预设打分规则交付，并镜像到明日推荐，打破周菜单跨周固定循环。

**Architecture:** 新文件 `week_plan_freshness.go` 提供 lastSeen 查询与天数换算；规则环境 `candidate` 增加 `days_since_last` 字段，经 `weekPlanDayContext` 注入（挑选器签名零改动）；种子规则 v4→v5 走既有 `syncDefaultMenuRules` 迁移；模板渲染器为 `stale_repeat` 类别加特殊分支，表达式与种子逐字节一致。

**Tech Stack:** Go (Gin + GORM + glebarez/sqlite)、expr-lang 规则引擎、React/TS 前端（仅一行 label）。

**Spec:** `docs/superpowers/specs/2026-06-12-freshness-decay-design.md`

**约定：** 所有后端命令在 `/Users/vincentchen/Documents/GitHub/NiniMenu/backend` 下执行。测试基建复用：`setupPlanServiceTestDB(t)`（内存库）、`withPlanNow(t, ...)`（钉时钟，week_plan_store_test.go）、`createDishForPlanTest(t, name, tags, ingredients)`。

---

### Task 1: 新鲜度数据层（week_plan_freshness.go）

**Files:**
- Create: `backend/internal/services/week_plan_freshness.go`
- Create: `backend/internal/services/week_plan_freshness_test.go`

- [ ] **Step 1: 写失败测试**

`week_plan_freshness_test.go` 全文：

```go
package services

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"testing"
	"time"
)

func TestLastSeenDishDatesMergesAndTrims(t *testing.T) {
	setupPlanServiceTestDB(t)
	now := time.Date(2026, 6, 8, 9, 0, 0, 0, time.Local) // today=2026-06-08, since=2026-05-26

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

	got := lastSeenDishDates(now)
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
	ref := time.Date(2026, 6, 8, 23, 30, 0, 0, time.Local) // 时分秒不应影响整天差
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
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/services/ -run 'TestLastSeenDishDates|TestDaysSinceFor' -v`
Expected: FAIL（undefined: lastSeenDishDates / daysSinceFor）

- [ ] **Step 3: 实现 week_plan_freshness.go**

```go
package services

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"time"
)

// Freshness decay: dishes seen within the last staleRepeatWindowDays carry a
// linearly fading score penalty (factory max staleRepeatDefaultPoints at d=0).
// The window is a factory constant by design — strength is tuned on the rule
// page (stale_repeat_penalty points); see the freshness-decay design spec.
const (
	staleRepeatWindowDays    = 14
	staleRepeatDefaultPoints = 30
)

// lastSeenDishDates returns each dish's most recent appearance date
// (meal_records ∪ dish_recommendations) within the decay window.
// Recommendation rows are capped at today: future rows are the current
// plan's tail, not something the family has eaten (same reasoning as
// recentDishIDMap). now is injectable: week-plan callers pass planNow(),
// the tomorrow pick passes time.Now().
func lastSeenDishDates(now time.Time) map[uint]string {
	today := now.Format("2006-01-02")
	since := now.AddDate(0, 0, -(staleRepeatWindowDays - 1)).Format("2006-01-02")

	type lastSeenRow struct {
		DishID   uint
		LastDate string
	}
	merge := func(result map[uint]string, rows []lastSeenRow) {
		for _, row := range rows {
			if row.DishID == 0 || row.LastDate == "" {
				continue
			}
			if existing, ok := result[row.DishID]; !ok || row.LastDate > existing {
				result[row.DishID] = row.LastDate
			}
		}
	}

	result := make(map[uint]string)
	var eaten []lastSeenRow
	database.DB.Model(&models.MealRecord{}).
		Select("dish_id, MAX(meal_date) AS last_date").
		Where("meal_date >= ?", since).
		Group("dish_id").
		Scan(&eaten)
	merge(result, eaten)

	var planned []lastSeenRow
	database.DB.Model(&models.DishRecommendation{}).
		Select("dish_id, MAX(planned_date) AS last_date").
		Where("planned_date >= ? AND planned_date <= ?", since, today).
		Group("dish_id").
		Scan(&planned)
	merge(result, planned)
	return result
}

// daysSinceFor converts lastSeen dates into whole-day distances relative to
// refDate. Unparsable or future dates map to -1 (= no penalty); -1 is the
// sentinel because a zero value would mean "seen today".
func daysSinceFor(refDate time.Time, lastSeen map[uint]string) map[uint]int {
	ref := time.Date(refDate.Year(), refDate.Month(), refDate.Day(), 0, 0, 0, 0, time.UTC)
	result := make(map[uint]int, len(lastSeen))
	for id, raw := range lastSeen {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			result[id] = -1
			continue
		}
		d := int(ref.Sub(parsed).Hours() / 24)
		if d < 0 {
			d = -1
		}
		result[id] = d
	}
	return result
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/services/ -run 'TestLastSeenDishDates|TestDaysSinceFor' -v`
Expected: PASS（2 个测试）

- [ ] **Step 5: 全量回归 + 提交**

Run: `go test ./... && go vet ./...`
Expected: 全绿（纯新增，不碰既有路径）

```bash
git add internal/services/week_plan_freshness.go internal/services/week_plan_freshness_test.go
git commit -m "feat: add freshness last-seen query and day-distance helpers"
```

---

### Task 2: 规则环境 days_since_last 字段与注入管线

**Files:**
- Modify: `backend/internal/services/menu_rule_service.go`（ruleDishEnv 结构体 ~line 22、dishRuleEnv ~line 416、sampleRuleEnv ~line 355）
- Modify: `backend/internal/services/plan_service.go`（weekPlanGenContext 结构体与 buildWeekPlanGenContext ~line 129、weekPlanDayContext ~line 448、generateDay ~line 184、evaluateScoreRules ~line 688、evaluateConstraintRules ~line 668）
- Test: `backend/internal/services/week_plan_freshness_test.go`（追加）

- [ ] **Step 1: 写失败测试（追加到 week_plan_freshness_test.go）**

```go
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
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/services/ -run 'TestEvaluateScoreRulesInjectsDaysSinceLast|TestEvaluateConstraintRulesSeesDaysSinceLast' -v`
Expected: FAIL（weekPlanDayContext 无 daysSince 字段，编译错误）

- [ ] **Step 3: 实现字段与注入**

menu_rule_service.go 三处：

```go
// ruleDishEnv 结构体，CategoryFavorites 之后加：
	DaysSinceLast int `expr:"days_since_last"`
```

```go
// dishRuleEnv() 返回字面量里加（零值 0 意为"今天见过"，必须显式 -1）：
		DaysSinceLast: -1,
```

```go
// sampleRuleEnv() 的样例菜字面量里加：
		DaysSinceLast: -1,
```

plan_service.go：

```go
// weekPlanGenContext 结构体加字段（与 recent 并列）：
	lastSeen map[uint]string

// buildWeekPlanGenContext() 返回字面量里，recent 行旁加：
		lastSeen: lastSeenDishDates(planNow()),

// weekPlanDayContext 结构体加字段：
	daysSince map[uint]int

// generateDay() 在 dayCtx.prevSoups = prevSoups 之后加：
	dayCtx.daysSince = daysSinceFor(date, ctx.lastSeen)
```

evaluateScoreRules 与 evaluateConstraintRules 同款改法——把内联的 `dishRuleEnv(candidate, dayCtx.categoryFavorites)` 拆出来覆盖字段后再传入：

```go
	candidateEnv := dishRuleEnv(candidate, dayCtx.categoryFavorites)
	if d, ok := dayCtx.daysSince[candidate.ID]; ok {
		candidateEnv.DaysSinceLast = d
	}
	env := buildRuleEnv(candidateEnv, dishesRuleEnv(mealPicked, dayCtx.categoryFavorites), ...其余参数不变...)
```

注意：env 构造在两个函数里各出现一次（循环体内）。`candidateEnv` 的提取放在 **for 循环外**（candidate 在整个函数内不变，避免每条规则重复构造）。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/services/ -run 'TestEvaluateScoreRulesInjectsDaysSinceLast|TestEvaluateConstraintRulesSeesDaysSinceLast' -v`
Expected: PASS

- [ ] **Step 5: 全量回归 + 提交**

Run: `go test ./... && go vet ./...`
Expected: 全绿——种子规则尚未引用新字段，纯管线改动零行为变化；若有失败说明注入改坏了 env 构造，立即排查。

```bash
git add internal/services/menu_rule_service.go internal/services/plan_service.go internal/services/week_plan_freshness_test.go
git commit -m "feat: expose days_since_last to the rule engine via day context"
```

---

### Task 3: 种子规则 v5 与模板渲染分支

**Files:**
- Modify: `backend/internal/models/menu_rules.go`（DefaultMenuRules 末尾追加第 15 条）
- Modify: `backend/internal/services/menu_rule_service.go`（menuRulesSeedVersion 4→5，~line 71）
- Modify: `backend/internal/services/menu_rule_templates.go`（renderPreference 拦截 + staleRepeatExpression）
- Modify: `backend/internal/services/menu_rule_templates_test.go`（新测试 + 既有计数 14→15）

- [ ] **Step 1: 写失败测试（追加到 menu_rule_templates_test.go）**

```go
func TestMenuRuleTemplateStaleRepeat(t *testing.T) {
	rule := models.MenuRule{Template: `{"type":"avoid","scope":"meal","category":"stale_repeat","n":1,"points":30,"strength":"prefer"}`}
	if err := ApplyMenuRuleTemplate(&rule); err != nil {
		t.Fatalf("ApplyMenuRuleTemplate() error = %v", err)
	}
	want := `candidate.days_since_last >= 0 && candidate.days_since_last < 14 ? -(30.0 * (14 - candidate.days_since_last) / 14.0) : 0`
	if rule.Expression != want {
		t.Errorf("expression = %s\nwant %s", rule.Expression, want)
	}
	if rule.RuleKind != "score" || rule.Severity != "soft" || !rule.Relaxable || rule.Scope != "candidate" {
		t.Errorf("rendered meta mismatch: kind=%s severity=%s relaxable=%v scope=%s", rule.RuleKind, rule.Severity, rule.Relaxable, rule.Scope)
	}

	custom := models.MenuRule{Template: `{"type":"avoid","scope":"meal","category":"stale_repeat","n":1,"points":18,"strength":"prefer"}`}
	if err := ApplyMenuRuleTemplate(&custom); err != nil {
		t.Fatalf("ApplyMenuRuleTemplate() points=18 error = %v", err)
	}
	want18 := `candidate.days_since_last >= 0 && candidate.days_since_last < 14 ? -(18.0 * (14 - candidate.days_since_last) / 14.0) : 0`
	if custom.Expression != want18 {
		t.Errorf("points=18 expression = %s\nwant %s", custom.Expression, want18)
	}

	for _, tpl := range []string{
		`{"type":"prefer","category":"stale_repeat"}`,
		`{"type":"limit","scope":"week","category":"stale_repeat","n":2}`,
	} {
		bad := models.MenuRule{Template: tpl}
		if err := ApplyMenuRuleTemplate(&bad); err == nil {
			t.Errorf("template %s should be rejected（display-only 类别只支持 avoid）", tpl)
		}
	}
}

func TestEnsureDefaultMenuRulesAddsStaleRepeatPenalty(t *testing.T) {
	setupPlanServiceTestDB(t)
	for _, rule := range models.DefaultMenuRules() {
		if rule.Code == "stale_repeat_penalty" {
			continue
		}
		rule := rule
		if rule.Code == "non_favorite_penalty" {
			rule.Expression = `!candidate.favorite ? -5 : 0` // 用户调过力度
			rule.Enabled = false
		}
		if err := database.DB.Create(&rule).Error; err != nil {
			t.Fatalf("seed v4 rule %s: %v", rule.Code, err)
		}
	}
	if err := setSettingValue(menuRulesSeedVersionKey, "4"); err != nil {
		t.Fatalf("set seed version: %v", err)
	}

	if err := EnsureDefaultMenuRules(); err != nil {
		t.Fatalf("EnsureDefaultMenuRules() error = %v", err)
	}

	var stale models.MenuRule
	if err := database.DB.Where("code = ?", "stale_repeat_penalty").First(&stale).Error; err != nil {
		t.Fatalf("stale_repeat_penalty not inserted: %v", err)
	}
	if !stale.Enabled || stale.RuleKind != "score" || stale.Priority != 310 {
		t.Errorf("unexpected stale rule row: enabled=%v kind=%s priority=%d", stale.Enabled, stale.RuleKind, stale.Priority)
	}
	if got := getSettingInt(menuRulesSeedVersionKey, 0); got != 5 {
		t.Errorf("seed version = %d, want 5", got)
	}
	var modified models.MenuRule
	if err := database.DB.Where("code = ?", "non_favorite_penalty").First(&modified).Error; err != nil {
		t.Fatalf("non_favorite_penalty missing: %v", err)
	}
	if modified.Expression != `!candidate.favorite ? -5 : 0` || modified.Enabled {
		t.Error("迁移动了用户改过的规则")
	}

	if err := EnsureDefaultMenuRules(); err != nil {
		t.Fatalf("EnsureDefaultMenuRules() second run error = %v", err)
	}
	var count int64
	database.DB.Model(&models.MenuRule{}).Where("code = ?", "stale_repeat_penalty").Count(&count)
	if count != 1 {
		t.Errorf("rerun 后 stale 规则数 = %d, want 1（幂等被破坏）", count)
	}
}
```

同时把 `TestDefaultMenuRuleTemplatesMatchExpressions` 开头的计数断言 `len(defaults) != 14` 改成 `!= 15`（错误信息里的 14 一并改 15）。该测试会自动对新规则做**模板渲染 == 种子表达式**的逐字节校验与编译校验——spec §4.3 的同步约束由它锁死。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/services/ -run 'TestMenuRuleTemplateStaleRepeat|TestEnsureDefaultMenuRulesAddsStaleRepeatPenalty|TestDefaultMenuRuleTemplatesMatchExpressions' -v`
Expected: FAIL（模板类别无效 / 规则缺失 / 计数 14≠15）

- [ ] **Step 3: 实现**

menu_rule_templates.go 加渲染函数（renderPreference 上方）：

```go
// staleRepeatExpression renders the freshness-decay penalty for a given
// strength. Must stay byte-identical to the stale_repeat_penalty seed in
// models.DefaultMenuRules — TestDefaultMenuRuleTemplatesMatchExpressions
// locks the sync.
func staleRepeatExpression(points int) string {
	w := staleRepeatWindowDays
	return fmt.Sprintf("candidate.days_since_last >= 0 && candidate.days_since_last < %d ? -(%d.0 * (%d - candidate.days_since_last) / %d.0) : 0", w, points, w, w)
}
```

renderPreference() 函数体最前面加拦截：

```go
	if t.Category == "stale_repeat" {
		if t.Type != "avoid" {
			return renderedMenuRule{}, fmt.Errorf("规则模板类别无效: %s", t.Category)
		}
		return renderedMenuRule{
			Expression: staleRepeatExpression(t.Points),
			RuleKind:   menuRuleKindScore,
			Severity:   "soft",
			Relaxable:  true,
			Scope:      "candidate",
		}, nil
	}
```

（limit/no_repeat 配 stale_repeat 走 menuRuleCategoryPredicate/Count 的 default 分支自然报错，无需改。）

models/menu_rules.go 在 soup_ingredient_repeat_penalty 之后追加：

```go
		// stale_repeat_penalty is the freshness decay: dishes seen (eaten or
		// planned) within the past 14 days fade back in linearly instead of
		// returning at full strength the moment the hard cooldown ends. The
		// expression must stay byte-identical to staleRepeatExpression(points)
		// in services/menu_rule_templates.go.
		{
			Code:        "stale_repeat_penalty",
			Name:        "最近吃过的菜降分",
			Description: "出了冷却的菜不立刻满血回归：距上次吃过或排进菜单越近降分越多，14 天后才完全恢复，让菜单跨周轮换不串台。",
			Enabled:     true,
			Scope:       "candidate",
			RuleKind:    "score",
			Severity:    "soft",
			Relaxable:   true,
			Expression:  `candidate.days_since_last >= 0 && candidate.days_since_last < 14 ? -(30.0 * (14 - candidate.days_since_last) / 14.0) : 0`,
			Template:    `{"type":"avoid","scope":"meal","category":"stale_repeat","n":1,"points":30,"strength":"prefer"}`,
			Priority:    310,
			Message:     "最近吃过的菜降分",
		},
```

menu_rule_service.go：`menuRulesSeedVersion = 4` 改为 `5`。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/services/ -run 'TestMenuRuleTemplateStaleRepeat|TestEnsureDefaultMenuRulesAddsStaleRepeatPenalty|TestDefaultMenuRuleTemplatesMatchExpressions' -v`
Expected: PASS（3 个测试）

- [ ] **Step 5: 全量回归 + 提交**

Run: `go test ./... && go vet ./...`
Expected: 全绿后才能提交。**注意**：从本步起种子规则生效，凡是自造了近 14 天 MealRecord/DishRecommendation 历史的既有测试可能受衰减影响——若失败，**当场**按 Task 4 Step 4 的决策规则处理（改 fixture 日期或更新期望，不许跳过/删除），处理后再提交；Task 4 的清扫步骤届时作为复核。与历史数据无关的失败则是本任务改坏了实现，修实现不修测试。

```bash
git add internal/models/menu_rules.go internal/services/menu_rule_service.go internal/services/menu_rule_templates.go internal/services/menu_rule_templates_test.go
git commit -m "feat: ship stale_repeat_penalty as seed rule v5 with template branch"
```

---

### Task 4: 周菜单衰减集成测试与既有测试回归清扫

**Files:**
- Test: `backend/internal/services/plan_service_test.go`（追加 3 个测试）
- Modify: 回归清扫中按需调整既有测试 fixture（见 Step 4 决策规则）

背景：`ListMenuRules` 在规则表为空时直接回落 `models.DefaultMenuRules()`（menu_rule_service.go:62-64），所以测试库不用跑迁移，衰减规则默认生效。

- [ ] **Step 1: 写失败测试（追加到 plan_service_test.go）**

```go
func TestGenerateWeekPlanAppliesFreshnessDecay(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 8, 9, 0, 0, 0, time.Local)) // 周一
	a := createDishForPlanTest(t, "决斗鸡块A", "", "鸡肉")
	b := createDishForPlanTest(t, "决斗鸡块B", "", "鸡肉")
	// A 五天前打卡：出了 3 天冷却（不被 recent 硬挡），但背 -19.3 衰减
	mustCreate(t, &models.MealRecord{DishID: a.ID, DishName: a.Name, MealType: "dinner", MealDate: "2026-06-03"})
	saveWeekPlanPreferenceForTest(t, WeekPlanPreferences{
		Weekday: WeekPlanPeriodPreferences{Profile: "balanced", Lunch: MealQuota{MeatCount: 1}},
		Weekend: WeekPlanPeriodPreferences{Profile: "balanced", Lunch: MealQuota{MeatCount: 1}},
	})

	plan, err := GenerateWeekPlan()
	if err != nil {
		t.Fatalf("GenerateWeekPlan() error = %v", err)
	}
	if len(plan.Days[0].Lunch) != 1 || plan.Days[0].Lunch[0].ID != b.ID {
		t.Errorf("周一午餐应选无历史的 B（A 背衰减分），got %+v", plan.Days[0].Lunch)
	}
}

func TestGenerateWeekPlanDecayExpiresAtWindow(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 8, 9, 0, 0, 0, time.Local)) // 周一
	a := createDishForPlanTest(t, "对照鸡块A", "", "鸡肉")
	b := createDishForPlanTest(t, "对照鸡块B", "", "鸡肉")
	// A 十四天前（2026-05-25）：已出窗口，零惩罚；B 五天前：-19.3
	mustCreate(t, &models.MealRecord{DishID: a.ID, DishName: a.Name, MealType: "dinner", MealDate: "2026-05-25"})
	mustCreate(t, &models.MealRecord{DishID: b.ID, DishName: b.Name, MealType: "dinner", MealDate: "2026-06-03"})
	saveWeekPlanPreferenceForTest(t, WeekPlanPreferences{
		Weekday: WeekPlanPeriodPreferences{Profile: "balanced", Lunch: MealQuota{MeatCount: 1}},
		Weekend: WeekPlanPeriodPreferences{Profile: "balanced", Lunch: MealQuota{MeatCount: 1}},
	})

	plan, err := GenerateWeekPlan()
	if err != nil {
		t.Fatalf("GenerateWeekPlan() error = %v", err)
	}
	if len(plan.Days[0].Lunch) != 1 || plan.Days[0].Lunch[0].ID != a.ID {
		t.Errorf("周一午餐应选已出窗口的 A，got %+v", plan.Days[0].Lunch)
	}
}

func TestDecayStillBitesWhenCooldownRelaxed(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 8, 9, 0, 0, 0, time.Local)) // 周一
	a := createDishForPlanTest(t, "放宽鸡块A", "", "鸡肉")
	b := createDishForPlanTest(t, "放宽鸡块B", "", "鸡肉")
	// 两道都在 3 天冷却内 → 第 1 级无候选 → 第 2 级放弃"最近避重"。
	// 旧行为：放宽后两道菜零惩罚平等竞争；新行为：衰减仍在，d=0 (-30) 输给 d=2 (-25.7)。
	mustCreate(t, &models.DishRecommendation{DishID: a.ID, DishName: a.Name, Source: "week_plan", MealType: "lunch", PlannedDate: "2026-06-08"})
	mustCreate(t, &models.DishRecommendation{DishID: b.ID, DishName: b.Name, Source: "week_plan", MealType: "lunch", PlannedDate: "2026-06-06"})
	saveWeekPlanPreferenceForTest(t, WeekPlanPreferences{
		Weekday: WeekPlanPeriodPreferences{Profile: "balanced", Lunch: MealQuota{MeatCount: 1}},
		Weekend: WeekPlanPeriodPreferences{Profile: "balanced", Lunch: MealQuota{MeatCount: 1}},
	})

	plan, err := GenerateWeekPlan()
	if err != nil {
		t.Fatalf("GenerateWeekPlan() error = %v", err)
	}
	if len(plan.Days[0].Lunch) != 1 || plan.Days[0].Lunch[0].ID != b.ID {
		t.Errorf("冷却放宽后应选 d=2 的 B 而非 d=0 的 A，got %+v", plan.Days[0].Lunch)
	}
}
```

- [ ] **Step 2: 跑测试确认失败基线**

Run: `go test ./internal/services/ -run 'TestGenerateWeekPlanAppliesFreshnessDecay|TestGenerateWeekPlanDecayExpiresAtWindow|TestDecayStillBitesWhenCooldownRelaxed' -v`
Expected: 若 Task 1-3 已正确实现则直接 PASS（功能已在）；若 FAIL 则定位实现缺口修复后再继续。这三个测试是行为锁，必须在本任务结束时稳定通过。

- [ ] **Step 3: 连跑 10 次防随机抖动**

Run: `go test ./internal/services/ -run 'TestGenerateWeekPlanAppliesFreshnessDecay|TestGenerateWeekPlanDecayExpiresAtWindow|TestDecayStillBitesWhenCooldownRelaxed' -count=10`
Expected: PASS ×10（设计的分差最小 4.3，远大于 0~1 随机微调；如有抖动说明分差计算与设计不符，回查实现）

- [ ] **Step 4: 全量回归清扫**

Run: `go test ./... 2>&1 | tail -30`

对每个失败的既有测试，按以下决策规则处理（不许跳过、不许删测试）：

1. **测试意图是冻结/手动菜/偏好/历史语义（非选菜打分）**，且 fixture 自造了近 14 天的 MealRecord / DishRecommendation 行 → 把 fixture 日期移到钉定时钟的 14 天之前，或给无关菜加历史使惩罚对称，保持原断言不变。
2. **测试断言具体选中哪道菜**，且衰减合法地改变了结果 → 确认新结果符合衰减语义后更新期望值，并在断言旁注释说明。
3. **与历史数据无关的失败** → 是本次改动引入的 bug，修实现不修测试。

已知风险名单（自造推荐行/打卡行的测试，逐一核对）：`TestSaveWeekPlanFreezesPastDaysAndTheirRecommendations`、`TestGenerateWeekPlanFreezesPastDays`、`TestRegenerateWeekPlanDayOnlyChangesTargetDay`、`TestRegenerateWeekPlanDayAfterPrefsSaveKeepsOtherDays`、`TestSaveWeekPlanMergesBackOmittedPastDays`；以及任何"先生成再重生成"两步式测试（第一步落的推荐行会进第二步的 lastSeen）。

Expected: 处理完后 `go test ./... && go vet ./...` 全绿。

- [ ] **Step 5: 提交**

```bash
git add internal/services/
git commit -m "test: lock freshness decay behavior in week plan generation"
```

---

### Task 5: 明日推荐镜像

**Files:**
- Modify: `backend/internal/services/pick_service.go`（pickPenaltyAdjustment ~line 205、sortTomorrowPool ~line 219、PickTomorrowDishes ~line 134、注释块 ~line 197）
- Modify: `backend/internal/models/menu_rules.go`（镜像注释块 ~line 134）
- Test: `backend/internal/services/pick_service_test.go`（更新 2 个既有 + 新增 1 个）

- [ ] **Step 1: 写失败测试**

pick_service_test.go 新增：

```go
func TestPickTomorrowDishesAppliesFreshnessDecay(t *testing.T) {
	setupPlanServiceTestDB(t)
	a := createDishForPlanTest(t, "明日鸡块A", "", "鸡肉")
	b := createDishForPlanTest(t, "明日鸡块B", "", "鸡肉")
	// A 五天前打卡（相对真实时钟；明天参照下 d=6 → 镜像扣 17 分）
	five := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	mustCreate(t, &models.MealRecord{DishID: a.ID, DishName: a.Name, MealType: "dinner", MealDate: five})

	picks, err := PickTomorrowDishes(TomorrowPickOptions{Count: 2})
	if err != nil {
		t.Fatalf("PickTomorrowDishes() error = %v", err)
	}
	if len(picks) != 2 || picks[0].ID != b.ID {
		t.Errorf("五天前吃过的 A 应排在 B 之后，got %+v", picks)
	}
}
```

更新既有测试以适配新签名（行为断言不变）：

- `TestPickPenaltyAdjustment`：所有既有调用补第 4 个实参 `-1`；并追加衰减断言——同一道菜在 `daysSinceLast` 为 `0 / 5 / 14 / -1` 时，调整值相对 `-1` 基线分别多扣 `30 / 19 / 0 / 0`（`round(30×9/14)=19`）。
- `TestSortTomorrowPoolPrefersFavorites`：`sortTomorrowPool` 调用补 `nil`（nil map 查询安全，等价全员无历史）。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/services/ -run 'TestPickTomorrowDishesAppliesFreshnessDecay|TestPickPenaltyAdjustment|TestSortTomorrowPoolPrefersFavorites' -v`
Expected: FAIL（签名不匹配，编译错误）

- [ ] **Step 3: 实现镜像**

pick_service.go——`pickPenaltyAdjustment` 加参数与镜像项（`import "math"`）：

```go
func pickPenaltyAdjustment(d models.Dish, categoryFavorites map[string]int, weekend bool, daysSinceLast int) int {
	adjust := 0
	if !d.Favorite {
		adjust -= 12
	}
	if d.Category != "" && categoryFavorites[d.Category] == 0 {
		adjust -= 25
	}
	if !weekend && d.DishRole == "soup" && (d.CookTime > 45 || d.Difficulty == "hard") {
		adjust -= 30
	}
	if daysSinceLast >= 0 && daysSinceLast < staleRepeatWindowDays {
		adjust -= int(math.Round(float64(staleRepeatDefaultPoints) * float64(staleRepeatWindowDays-daysSinceLast) / float64(staleRepeatWindowDays)))
	}
	return adjust
}
```

`sortTomorrowPool` 加 `daysSince map[uint]int` 参数，比较器内取值：

```go
func sortTomorrowPool(dishes []models.Dish, profile string, categoryFavorites map[string]int, weekend bool, daysSince map[uint]int) {
	daysSinceOf := func(id uint) int {
		if d, ok := daysSince[id]; ok {
			return d
		}
		return -1
	}
	// ...shuffle 不变；SliceStable 内两处 pickPenaltyAdjustment 调用补
	// daysSinceOf(dishes[i].ID) / daysSinceOf(dishes[j].ID)
}
```

`PickTomorrowDishes` 在 `tomorrow := ...` 行后构造映射并传入：

```go
	tomorrow := time.Now().AddDate(0, 0, 1)
	daysSince := daysSinceFor(tomorrow, lastSeenDishDates(time.Now()))
	sortTomorrowPool(pool, profile, favoriteCategoryCounts(), isWeekend(tomorrow), daysSince)
```

注释同步（两处，保持 keep-in-sync 提示准确）：

- pick_service.go ~line 197 注释块："mirrors the three v3 default score rules" → 改为说明镜像**三条 v3 降权 + v5 新鲜度衰减（stale_repeat_penalty，用出厂默认 30/14）**，其余措辞保留。
- models/menu_rules.go ~line 134 注释块：同步加一句 "stale_repeat_penalty (v5) is mirrored too"。

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/services/ -run 'TestPickTomorrowDishesAppliesFreshnessDecay|TestPickPenaltyAdjustment|TestSortTomorrowPoolPrefersFavorites' -v && go test ./... && go vet ./...`
Expected: 全 PASS

- [ ] **Step 5: 提交**

```bash
git add internal/services/pick_service.go internal/services/pick_service_test.go internal/models/menu_rules.go
git commit -m "feat: mirror freshness decay into the tomorrow pick"
```

---

### Task 6: 前端规则页类别文案

**Files:**
- Modify: `frontend/src/lib/menuRuleTemplates.ts`（DISPLAY_ONLY_CATEGORY_LABELS，~line 46-52）

- [ ] **Step 1: 加 label**

`DISPLAY_ONLY_CATEGORY_LABELS` 对象的 `soup_ingredient_repeat` 行之后加：

```ts
  stale_repeat: "最近吃过的菜",
```

卡片句子由 `describeTemplate` 的 avoid 分支自动渲染为"尽量避开最近吃过的菜"；points 步进器走既有机制，无需其他改动。

- [ ] **Step 2: 构建验证**

Run: `npm --prefix /Users/vincentchen/Documents/GitHub/NiniMenu/frontend run build`
Expected: 构建通过（既有 chunk 体积警告可忽略）

- [ ] **Step 3: 提交**

```bash
git add frontend/src/lib/menuRuleTemplates.ts
git commit -m "feat: label stale_repeat category on the rule page"
```

---

### Task 7: 文档、CHANGELOG 与上线验证

**Files:**
- Modify: `docs/推荐算法说明.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: 更新推荐算法说明.md**

逐处修改（保持文档"人话、无代码"的文风）：

1. 文档头：`> 更新日期：2026-06-11（对应 v0.7.1）` → `> 更新日期：2026-06-12（对应 v0.9.0）`。
2. §1 候选池表"最近吃过的进冷却期"行说明列，句末追加："出了冷却也不是立刻满血——14 天内仍背一个逐日消退的降分（见 3.2）"。
3. §3.2 减分表，"工作日少排费时汤"行之后加：

```markdown
| **最近吃过的菜降分** ★新 | 14 天内吃过或排进过菜单的菜，距今越近扣越多 | 最多 -30，每天消退约 2 分，14 天归零 |
```

4. §3.4 段位感段落末尾追加一句："新加的轮换降分同理：刚出冷却约 -21、一周前 -15，足以让位给两周没吃的同档菜，但拼不过位置和主题的硬需求。"
5. §5 放宽阶梯表，级别 2 行"人话"列追加："（轮换降分仍生效，刚吃过的菜放宽了也排队尾）"。
6. §6 入口差异表，"规则页的规则"行明日推荐列：`⚠️ 只镜像三条新降权的**默认分值**` → `⚠️ 只镜像四条降权规则的**默认分值**`。
7. §7 控制面表加一行：

```markdown
| 嫌菜单轮换太快/太慢 | 规则页 → 「最近吃过的菜降分」的「降多少分」步进器；关闭则回到只有 3 天冷却 |
```

8. §9 底层语义备忘加一条："**轮换记忆 = 打卡 ∪ 排进菜单**：新鲜度只看每道菜最后一次出现（两个来源取较近），未来天的计划不算；从未出现过的菜不背轮换分。"

- [ ] **Step 2: 更新 CHANGELOG.md**

文件顶部（上一版本节之前）插入：

```markdown
## v0.9.0 - 2026-06-12

### 新增

- **跨周轮换（新鲜度衰减）**：吃过或排进过菜单的菜，其后 14 天内背一个随时间线性消退的降分（出厂最多 -30），出冷却不再立刻满血回归，多周菜单不再收敛成固定循环。
- 规则页新增第 10 条预设规则「最近吃过的菜降分」，力度可调、可关闭；菜不够时随第 5 级放宽自动失效。
- 「明日推荐」按出厂默认值镜像同款降分，两个入口轮换体感一致。

### 修复

- 菜品库紧张触发"最近避重"放宽时，冷却期内的菜此前会零惩罚满血参赛；现在衰减降分仍然生效，刚吃过的菜放宽后也排在队尾。
```

- [ ] **Step 3: 全量验证**

```bash
cd /Users/vincentchen/Documents/GitHub/NiniMenu/backend && go test ./... && go vet ./...
npm --prefix /Users/vincentchen/Documents/GitHub/NiniMenu/frontend run build
```

Expected: 全绿。

- [ ] **Step 4: 重启后端并冒烟**

后端重启按既有惯例：`lsof -ti :8080 | xargs kill` 后台重启 `go run ./cmd/server`，等日志出现"NiniMenu 启动成功"且无迁移报错。鉴权 token 的取法与 v0.8.0 收尾冒烟相同（X-App-Token，值从后端配置/启动方式确认）。

冒烟三连：

1. `GET /menu-rules`（带 token）→ 响应里存在 `"code":"stale_repeat_penalty"` 且 `enabled:true`、points 模板含 30。
2. `sqlite3 backend/data/ninimenu.db "SELECT value FROM settings WHERE \`key\`='menu_rules_seed_version';"` → `5`。
3. `POST /week-plan/regenerate`（或前端等价入口）连续两次，对比今天的菜：大概率换批（d=0 惩罚生效的体感验证；非硬断言，记录观察结果即可）。

- [ ] **Step 5: 提交**

```bash
git add docs/推荐算法说明.md CHANGELOG.md
git commit -m "docs: document freshness decay in algorithm guide and changelog"
```

---

## 最终集成检查清单（执行完 7 个任务后逐项核对）

1. `go test ./...`、`go vet`、`npm run build` 全绿。
2. 升级路径 live 验证：现库重启后规则表出现 `stale_repeat_penalty`，seed version=5，重启幂等（连启两次无 dup）。
3. 衰减三测试（基本生效 / 窗口归零 / 冷却放宽仍生效）`-count=10` 稳定。
4. 规则页可见可调可关：GET /menu-rules 含新规则；模板渲染与种子表达式逐字节一致（TestDefaultMenuRuleTemplatesMatchExpressions 在）。
5. 明日推荐镜像生效（TestPickTomorrowDishesAppliesFreshnessDecay 在），转盘 PickDishes 路径零改动。
6. 关闭规则或 points=0 → 行为回到 v0.8.0 基线（手动验证一次：关规则后重生成，无衰减体感）。
7. 兼容回归：recentDishIDMap / 购物清单 / 回看端点 / week_plans 读写路径零改动（git diff 确认未触碰）。
8. 既有测试回归清扫只按 Task 4 决策规则改 fixture/期望，无删测试、无跳过。
9. spec 偏差核对：实现与 `2026-06-12-freshness-decay-design.md` 逐节一致；任何偏差记录在案并有理由。

## 计划自检记录

- **Spec 覆盖**：§2 数据源→Task 1；§4.1 env→Task 2；§4.2/4.3 种子与渲染→Task 3；§4.5/§5 管线与放宽→Task 2+4；§6 镜像→Task 5；§4.4 前端→Task 6；§8 文档→Task 7；§7 测试矩阵全部落入 Task 1-5；§9 验收→最终清单。无遗漏。
- **占位符扫描**：每个代码步骤均给出完整代码/精确编辑位置；Task 7 token 取法引用既有惯例属操作指引非占位。
- **类型一致性**：`lastSeenDishDates(now time.Time) map[uint]string`、`daysSinceFor(refDate time.Time, lastSeen map[uint]string) map[uint]int`、`dayCtx.daysSince map[uint]int`、`pickPenaltyAdjustment(..., daysSinceLast int)`、`sortTomorrowPool(..., daysSince map[uint]int)` 在各任务间引用一致；常量 `staleRepeatWindowDays`/`staleRepeatDefaultPoints` 定义于 Task 1、Task 3/5 引用。
- **数字一致性**：30×9/14=19.29→round 19（Task 5 断言）；30×12/14=25.71 vs 30 差 4.29>1（Task 4 第三测试）；d=5 → 19.29（周一测试用 -19.3 描述）。
