# 周菜单持久化重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把周菜单从 Setting 单槽缓存升级为按周持久化的一等公民：新表 `week_plans`（user_id 预留）、冻结过去天、手动菜在重生成中幸存、一次性偏好按周清空、推荐记录历史不可变、History 页轻量回看。

**Architecture:** 方案 A（按周快照表）。`week_plans` 一用户一周一行存整周计划 JSON + 本周一次性偏好 JSON；`DishRecommendation` 扁平行保持冷却/回看索引职责，写侧收紧为只动今天及以后。所有"今天"判定走可注入时钟 `planNow`，测试可控。统一写路径 `SaveWeekPlan` 内置过去天保护，生成器冻结复制过去天并保留手动菜。

**Tech Stack:** Go 1.x + Gin + GORM + SQLite（glebarez/sqlite，纯 Go）；前端 React + TS + Vite + @tanstack/react-query。

**Spec:** `docs/superpowers/specs/2026-06-12-week-plan-persistence-design.md`（已批准）。

---

## 改动前的关键事实（实施者必读）

实施前不需要重新调研，以下事实已核实（行号为改动前）：

1. **现存储**：`internal/services/plan_service.go:59-105` —— `GetCachedWeekPlan` 读 Setting key `week_plan_cache`（检查 `plan.Days[0].Date >= weekKey`），`RegenerateWeekPlan` 经 `saveWeekPlanCache` 调 `SaveWeekPlan`，`InvalidateWeekPlanCache` 删 Setting key + 清内存缓存（`cachedPlan`/`cachedWeekKey`/`planMu`，plan_service.go:33-37）。
2. **SaveWeekPlan**：`internal/services/week_plan_recommendations.go:13-40` —— normalize → marshal → 事务（Setting upsert + `replaceWeekPlanRecommendations`）→ 更新内存缓存。`replaceWeekPlanRecommendations`（:53-80）删除计划**全部日期**的 source=week_plan 行后整批重写——这是要修的"改写历史"。
3. **生成器**：`plan_service.go:206-230` `GenerateWeekPlan` 永远从本周一生成 7 天；`generateDay`（:176-204）每天先午餐后晚餐调 `pickQuotaDishes`；`pickQuotaDishes`（:371-）内部 `var picked []models.Dish` 从空累计，按 `weekPlanSlots(quota)`（meat/veg/soup 槽）逐槽 pick。`RegenerateWeekPlanDay`（:234-295）已只动一天、已传 `adjacentPlannedSoups`。
4. **时间**：`getCurrentWeekKey`（:39-47）和 `GenerateWeekPlan` 用 `time.Now()` 算周一；无时钟注入。
5. **偏好**：`internal/services/week_plan_preferences.go` —— `WeekPlanPreferences{Weekday, Weekend, WeekWant, Days}` 整体存 Setting key `week_plan_preferences`；`SaveWeekPlanPreferences`（:63-80）刻意**不**失效周计划缓存（注释说明，保留此行为）；`normalizeWeekPlanPreferences`（:106-112）规整全部字段。
6. **路由**：`internal/routes/routes.go:67-73` week-plan 端点在 `app` 组（AppAuthMiddleware）。handler 在 `internal/handlers/plan.go`，统一 `utils.Success/BadRequest/InternalError`。
7. **启动顺序**：`cmd/server/main.go` —— `database.Init()`（AutoMigrate 列表在 `internal/database/database.go:33-51`）→ `services.EnsureDefaultMenuRules()`（失败仅 warn）。新迁移函数照此模式挂在 main。
8. **购物清单与周计划无耦合**：`BuildShoppingList`（plan_service.go:860）读 `ShoppingCheck` 行；这些行由**打卡**（`internal/handlers/record.go:21-64` `addShoppingItems`）从菜的食材生成。本次改动完全不触及购物清单（spec §7.2 原文有误，Task 8 修正 spec）。
9. **冷却读侧**：`recentDishIDMap`（pick_service.go:311-336）读 MealRecord + DishRecommendation（`planned_date BETWEEN since AND today`），零改动。
10. **测试基建**：`plan_service_test.go:16-32` `setupPlanServiceTestDB` 用 `:memory:` SQLite 换 `database.DB` + cleanup；fixture helper `createDishForPlanTest(t, name, tags, ingredients)`。受影响的现有测试：`TestSaveWeekPlanRecordsAndReplacesRecommendations`（:161，用未来日期 2099-01-05，新行为下应继续绿）、`TestInvalidateWeekPlanCacheClearsStoredAndInMemoryCache`（:806，断言要改写）、`week_plan_preferences_test.go:51` `TestSaveWeekPlanPreferencesRoundTripsDays`（round-trip 语义不变，应继续绿）。
11. **前端**：`frontend/src/pages/WeekPlan.tsx` —— `addDish`/`removeDish`（:852-872）改 `draftPlan` 后置 `dirtyPlan`；`normalizePlan`（:107-116）对 day 用 spread，新字段自动透传无须改；保存 `weekPlanApi.save(draftPlan)`。类型在 `frontend/src/types/index.ts:181-191`；API 客户端 `frontend/src/api/index.ts:88-98`。History 页 `frontend/src/pages/History.tsx`：`calYear`/`calMonth`/`selectedDay` 状态，`selectedDateKey`（:160），月度查询先例 `dayRatings`（:171-174）。
12. **手动菜与槽位**：荤/素/汤槽判定依据 `matchesWeekPlanSlot`（plan_service.go:491-509）：soup = `DishRole=="soup" || isSoupDish(dish)`，veg = `DishRole=="veg"`，meat = 其余。手动菜配额扣减复用同一判定逻辑。

## 文件结构

| 文件 | 操作 | 职责 |
|---|---|---|
| `backend/internal/models/week_plan_record.go` | 新建 | `WeekPlanRecord` GORM 模型（表名 week_plans） |
| `backend/internal/database/database.go` | 修改 | AutoMigrate 注册新模型 |
| `backend/internal/services/week_plan_store.go` | 新建 | 存取层：`CurrentUserID`、`planNow` 时钟、`todayKey`、行 load/upsert、启动迁移 `MigrateWeekPlanStorage` |
| `backend/internal/services/week_plan_store_test.go` | 新建 | 存取层 + 迁移测试 |
| `backend/internal/services/plan_service.go` | 修改 | `WeekDayPlan` 加 manual 字段；GetCached/Regenerate/Invalidate 切换新表；生成器冻结 + 手动菜保留；单天重排拒绝过去 |
| `backend/internal/services/week_plan_recommendations.go` | 修改 | `SaveWeekPlan` 写新表 + 过去天保护；`replaceWeekPlanRecommendations` 只动今天及以后 |
| `backend/internal/services/week_plan_preferences.go` | 修改 | Get/Save 拆分常驻（Setting）与本周一次性（行 PrefsJSON） |
| `backend/internal/services/week_plan_history.go` | 新建 | `WeekPlanHistoryByMonth` 回看查询 |
| `backend/internal/services/week_plan_history_test.go` | 新建 | 回看查询测试 |
| `backend/internal/handlers/plan.go` | 修改 | `GetWeekPlanHistoryHandler` |
| `backend/internal/routes/routes.go` | 修改 | `GET /week-plan/history` |
| `backend/cmd/server/main.go` | 修改 | 启动调 `MigrateWeekPlanStorage` |
| `backend/internal/services/plan_service_test.go` | 修改 | 适配 + 新增冻结/手动菜/拒绝过去测试 |
| `backend/internal/services/week_plan_preferences_test.go` | 修改 | 新增拆分/翻篇清零测试 |
| `frontend/src/types/index.ts` | 修改 | `WeekDayPlan` manual 字段；`PlannedDishEntry` |
| `frontend/src/api/index.ts` | 修改 | `weekPlanApi.history` |
| `frontend/src/pages/WeekPlan.tsx` | 修改 | addDish/removeDish 维护 manual ids |
| `frontend/src/pages/History.tsx` | 修改 | 选中日显示"当天排了什么" |
| `docs/superpowers/specs/2026-06-12-week-plan-persistence-design.md` | 修改 | 修正购物清单耦合描述（Task 8） |
| `CHANGELOG.md` | 修改 | v0.8.0 条目 |

任务顺序：Task 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8（线性执行；5 依赖 4 的类型，7 依赖 6 的端点）。

---

### Task 1: WeekPlanRecord 模型与存取层（含可注入时钟）

**Files:**
- Create: `backend/internal/models/week_plan_record.go`
- Modify: `backend/internal/database/database.go:33-51`（AutoMigrate 列表）
- Modify: `backend/internal/services/plan_service.go:39-47`（getCurrentWeekKey 用 planNow）、`:212`（GenerateWeekPlan 的 now）
- Create: `backend/internal/services/week_plan_store.go`
- Modify: `backend/internal/services/plan_service_test.go:24`（setup 的 AutoMigrate 加 WeekPlanRecord）
- Create: `backend/internal/services/week_plan_store_test.go`

- [ ] **Step 1: 写模型文件**

```go
// backend/internal/models/week_plan_record.go
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
```

- [ ] **Step 2: 注册 AutoMigrate**

`backend/internal/database/database.go` 的 AutoMigrate 调用里，在 `&models.DishRecommendation{},` 之后插一行：

```go
		&models.WeekPlanRecord{},
```

- [ ] **Step 3: 写失败测试（存取层）**

新建 `backend/internal/services/week_plan_store_test.go`：

```go
package services

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"testing"
	"time"
)

// withPlanNow pins the week-plan clock to a fixed instant for the test.
func withPlanNow(t *testing.T, fixed time.Time) {
	t.Helper()
	original := planNow
	planNow = func() time.Time { return fixed }
	t.Cleanup(func() { planNow = original })
}

func TestTodayKeyAndWeekKeyUsePlanNow(t *testing.T) {
	// 2026-06-18 is a Thursday; its Monday is 2026-06-15.
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local))

	if got := todayKey(); got != "2026-06-18" {
		t.Fatalf("todayKey() = %q, want 2026-06-18", got)
	}
	if got := getCurrentWeekKey(); got != "2026-06-15" {
		t.Fatalf("getCurrentWeekKey() = %q, want 2026-06-15", got)
	}
}

func TestWeekPlanRecordUpsertsPreserveSiblingColumns(t *testing.T) {
	setupPlanServiceTestDB(t)

	if err := upsertWeekPlanRecordPrefs("2026-06-15", `{"week_want":["beef"]}`); err != nil {
		t.Fatalf("upsert prefs (create) error = %v", err)
	}
	if err := upsertWeekPlanRecordPlan(database.DB, "2026-06-15", `{"days":[]}`); err != nil {
		t.Fatalf("upsert plan (update) error = %v", err)
	}

	rec, ok := loadWeekPlanRecord("2026-06-15")
	if !ok {
		t.Fatalf("loadWeekPlanRecord should find the row")
	}
	if rec.PrefsJSON != `{"week_want":["beef"]}` {
		t.Fatalf("PrefsJSON = %q, want preserved", rec.PrefsJSON)
	}
	if rec.PlanJSON != `{"days":[]}` {
		t.Fatalf("PlanJSON = %q, want written", rec.PlanJSON)
	}

	if err := upsertWeekPlanRecordPrefs("2026-06-15", `{"week_want":["pork"]}`); err != nil {
		t.Fatalf("upsert prefs (update) error = %v", err)
	}
	rec, _ = loadWeekPlanRecord("2026-06-15")
	if rec.PlanJSON != `{"days":[]}` {
		t.Fatalf("PlanJSON = %q, must survive prefs update", rec.PlanJSON)
	}
	if rec.PrefsJSON != `{"week_want":["pork"]}` {
		t.Fatalf("PrefsJSON = %q, want updated", rec.PrefsJSON)
	}

	var count int64
	database.DB.Model(&models.WeekPlanRecord{}).Count(&count)
	if count != 1 {
		t.Fatalf("week_plans row count = %d, want 1 (upsert must not duplicate)", count)
	}

	if _, ok := loadWeekPlanRecord("2026-06-08"); ok {
		t.Fatalf("loadWeekPlanRecord(other week) should miss")
	}
}
```

同时在 `plan_service_test.go:24` 的 `db.AutoMigrate(...)` 列表里加 `&models.WeekPlanRecord{}`（放 `&models.DishRecommendation{}` 之后）。

- [ ] **Step 4: 跑测试确认失败**

Run: `cd backend && go test ./internal/services/ -run 'TestTodayKey|TestWeekPlanRecordUpserts' -v`
Expected: 编译错误 `undefined: planNow` / `undefined: todayKey` 等。

- [ ] **Step 5: 写存取层实现**

新建 `backend/internal/services/week_plan_store.go`：

```go
package services

import (
	"errors"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"time"

	"gorm.io/gorm"
)

// CurrentUserID is the single-user placeholder threaded through week-plan
// storage. The multi-user sub-project will replace call sites with an
// authenticated user id; the schema (user_id column + composite unique key)
// is already in place so those rows migrate for free.
const CurrentUserID uint = 1

// planNow is the clock behind every week-plan date decision (week key, the
// today freeze boundary, recommendation rewrites). Tests swap it to pin time.
var planNow = time.Now

func todayKey() string {
	return planNow().Format("2006-01-02")
}

// loadWeekPlanRecord returns the row for (CurrentUserID, weekStart), or
// (nil, false) when it does not exist.
func loadWeekPlanRecord(weekStart string) (*models.WeekPlanRecord, bool) {
	var rec models.WeekPlanRecord
	err := database.DB.Where("user_id = ? AND week_start = ?", CurrentUserID, weekStart).First(&rec).Error
	if err != nil {
		return nil, false
	}
	return &rec, true
}

// upsertWeekPlanRecordPlan writes plan_json for (CurrentUserID, weekStart)
// inside tx, creating the row when missing. prefs_json is left untouched so
// the two columns can be written independently.
func upsertWeekPlanRecordPlan(tx *gorm.DB, weekStart string, planJSON string) error {
	var rec models.WeekPlanRecord
	err := tx.Where("user_id = ? AND week_start = ?", CurrentUserID, weekStart).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&models.WeekPlanRecord{UserID: CurrentUserID, WeekStart: weekStart, PlanJSON: planJSON}).Error
	}
	if err != nil {
		return err
	}
	return tx.Model(&rec).Update("plan_json", planJSON).Error
}

// upsertWeekPlanRecordPrefs mirrors upsertWeekPlanRecordPlan for prefs_json.
// It runs outside any caller transaction because preference saves are
// standalone writes.
func upsertWeekPlanRecordPrefs(weekStart string, prefsJSON string) error {
	var rec models.WeekPlanRecord
	err := database.DB.Where("user_id = ? AND week_start = ?", CurrentUserID, weekStart).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return database.DB.Create(&models.WeekPlanRecord{UserID: CurrentUserID, WeekStart: weekStart, PrefsJSON: prefsJSON}).Error
	}
	if err != nil {
		return err
	}
	return database.DB.Model(&rec).Update("prefs_json", prefsJSON).Error
}
```

- [ ] **Step 6: 接入时钟**

`backend/internal/services/plan_service.go`：

`getCurrentWeekKey`（:39-47）首行 `now := time.Now()` 改为：

```go
	now := planNow()
```

`GenerateWeekPlan`（:212）`now := time.Now()` 同样改为：

```go
	now := planNow()
```

注意：`buildWeekPlanGenContext` 里的 `rand.NewSource(time.Now().UnixNano())` **不改**（随机种子无须注入）。

- [ ] **Step 7: 跑测试确认通过**

Run: `cd backend && go test ./internal/services/ -run 'TestTodayKey|TestWeekPlanRecordUpserts' -v`
Expected: 2 个测试 PASS。

- [ ] **Step 8: 全量回归 + 提交**

Run: `cd backend && go test ./...`
Expected: 全绿（行为未变，只加了模型与未被调用的存取层）。

```bash
git add backend/internal/models/week_plan_record.go backend/internal/database/database.go backend/internal/services/week_plan_store.go backend/internal/services/week_plan_store_test.go backend/internal/services/plan_service.go backend/internal/services/plan_service_test.go
git commit -m "feat: add week_plans snapshot table and injectable plan clock"
```

### Task 2: SaveWeekPlan 切换新表 + 历史只读保护

**Files:**
- Modify: `backend/internal/services/week_plan_recommendations.go:13-80`（SaveWeekPlan、replaceWeekPlanRecommendations）
- Modify: `backend/internal/services/plan_service.go:59-105`（GetCachedWeekPlan、RegenerateWeekPlan、saveWeekPlanCache 删除、InvalidateWeekPlanCache）
- Modify: `backend/internal/services/plan_service_test.go`（改写 TestInvalidateWeekPlanCacheClearsStoredAndInMemoryCache；新增 3 个测试）

- [ ] **Step 1: 写失败测试**

在 `backend/internal/services/plan_service_test.go` 中，**改写** `TestInvalidateWeekPlanCacheClearsStoredAndInMemoryCache`（:806-829）为：

```go
func TestInvalidateWeekPlanCacheClearsStoredAndInMemoryCache(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local)) // 周四，周一=2026-06-15

	cachedPlan = &WeekPlan{Days: []WeekDayPlan{{Date: "2026-06-15", DayName: "周一"}}}
	cachedWeekKey = "2026-06-15"
	if err := database.DB.Create(&models.WeekPlanRecord{
		UserID:    CurrentUserID,
		WeekStart: "2026-06-15",
		PlanJSON:  `{"days":[]}`,
		PrefsJSON: `{"week_want":["beef"]}`,
	}).Error; err != nil {
		t.Fatalf("create week plan record: %v", err)
	}

	InvalidateWeekPlanCache()

	if cachedPlan != nil {
		t.Fatalf("cachedPlan should be nil after invalidation")
	}
	if cachedWeekKey != "" {
		t.Fatalf("cachedWeekKey = %q, want empty", cachedWeekKey)
	}
	rec, ok := loadWeekPlanRecord("2026-06-15")
	if !ok {
		t.Fatalf("invalidation must keep the row (prefs live on it)")
	}
	if rec.PlanJSON != "" {
		t.Fatalf("PlanJSON = %q, want cleared", rec.PlanJSON)
	}
	if rec.PrefsJSON != `{"week_want":["beef"]}` {
		t.Fatalf("PrefsJSON = %q, must survive invalidation", rec.PrefsJSON)
	}
}
```

**新增** 3 个测试（追加到同文件）：

```go
func TestSaveWeekPlanWritesWeekRowAndSkipsPastRecommendations(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local)) // 周四

	monDish := createDishForPlanTest(t, "周一的菜", `["家常菜"]`, `[{"name":"土豆","amount":"1个"}]`)
	thuDish := createDishForPlanTest(t, "周四的菜", `["家常菜"]`, `[{"name":"番茄","amount":"2个"}]`)

	plan := &WeekPlan{Days: []WeekDayPlan{
		{Date: "2026-06-15", DayName: "周一", Lunch: []models.Dish{monDish}, Dinner: []models.Dish{}},
		{Date: "2026-06-18", DayName: "周四", Lunch: []models.Dish{thuDish}, Dinner: []models.Dish{}},
	}}
	if err := SaveWeekPlan(plan); err != nil {
		t.Fatalf("SaveWeekPlan error = %v", err)
	}

	rec, ok := loadWeekPlanRecord("2026-06-15")
	if !ok || rec.PlanJSON == "" {
		t.Fatalf("SaveWeekPlan must write the week row, got ok=%v", ok)
	}
	var setting models.Setting
	if err := database.DB.Where("`key` = ?", "week_plan_cache").First(&setting).Error; err == nil {
		t.Fatalf("SaveWeekPlan must not write the legacy Setting key any more")
	}

	var recs []models.DishRecommendation
	database.DB.Find(&recs)
	if len(recs) != 1 || recs[0].PlannedDate != "2026-06-18" {
		t.Fatalf("only today-and-later days get recommendation rows, got %+v", recs)
	}
}

func TestSaveWeekPlanFreezesPastDaysAndTheirRecommendations(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local)) // 周四

	origMon := createDishForPlanTest(t, "原本周一", `["家常菜"]`, `[{"name":"白菜","amount":"1棵"}]`)
	newMon := createDishForPlanTest(t, "篡改周一", `["家常菜"]`, `[{"name":"萝卜","amount":"1根"}]`)
	thuDish := createDishForPlanTest(t, "周四新菜", `["家常菜"]`, `[{"name":"豆腐","amount":"1块"}]`)

	// 直接预置存量行与周一的历史推荐行，模拟"周一当时排的"。
	storedPlan := WeekPlan{Days: []WeekDayPlan{
		{Date: "2026-06-15", DayName: "周一", Lunch: []models.Dish{origMon}, Dinner: []models.Dish{}},
		{Date: "2026-06-18", DayName: "周四", Lunch: []models.Dish{}, Dinner: []models.Dish{}},
	}}
	storedJSON, _ := json.Marshal(storedPlan)
	if err := database.DB.Create(&models.WeekPlanRecord{
		UserID: CurrentUserID, WeekStart: "2026-06-15", PlanJSON: string(storedJSON),
	}).Error; err != nil {
		t.Fatalf("seed week row: %v", err)
	}
	if err := database.DB.Create(&models.DishRecommendation{
		DishID: origMon.ID, DishName: origMon.Name,
		Source: recommendationSourceWeekPlan, MealType: "lunch", PlannedDate: "2026-06-15",
	}).Error; err != nil {
		t.Fatalf("seed past recommendation: %v", err)
	}

	// 客户端试图改写周一（过去）并更新周四（未来）。
	edited := &WeekPlan{Days: []WeekDayPlan{
		{Date: "2026-06-15", DayName: "周一", Lunch: []models.Dish{newMon}, Dinner: []models.Dish{}},
		{Date: "2026-06-18", DayName: "周四", Lunch: []models.Dish{thuDish}, Dinner: []models.Dish{}},
	}}
	if err := SaveWeekPlan(edited); err != nil {
		t.Fatalf("SaveWeekPlan error = %v", err)
	}

	// 内存返回值与落库 JSON 的周一都必须还原为存量版本。
	if got := edited.Days[0].Lunch[0].ID; got != origMon.ID {
		t.Fatalf("past day in returned plan = dish %d, want stored %d", got, origMon.ID)
	}
	rec, _ := loadWeekPlanRecord("2026-06-15")
	var persisted WeekPlan
	if err := json.Unmarshal([]byte(rec.PlanJSON), &persisted); err != nil {
		t.Fatalf("unmarshal persisted plan: %v", err)
	}
	if persisted.Days[0].Lunch[0].ID != origMon.ID {
		t.Fatalf("past day rewritten in storage: got dish %d", persisted.Days[0].Lunch[0].ID)
	}
	if persisted.Days[1].Lunch[0].ID != thuDish.ID {
		t.Fatalf("today's edit must be saved, got %+v", persisted.Days[1].Lunch)
	}

	// 周一的历史推荐行原样保留；周四的行被重写。
	var monRecs []models.DishRecommendation
	database.DB.Where("planned_date = ?", "2026-06-15").Find(&monRecs)
	if len(monRecs) != 1 || monRecs[0].DishID != origMon.ID {
		t.Fatalf("past recommendation rows must be immutable, got %+v", monRecs)
	}
	var thuRecs []models.DishRecommendation
	database.DB.Where("planned_date = ?", "2026-06-18").Find(&thuRecs)
	if len(thuRecs) != 1 || thuRecs[0].DishID != thuDish.ID {
		t.Fatalf("today's recommendation rows must be rewritten, got %+v", thuRecs)
	}
}

func TestGetCachedWeekPlanReadsRowAndRollsOver(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local)) // 本周一 2026-06-15

	dish := createDishForPlanTest(t, "存档里的菜", `["家常菜"]`, `[{"name":"鸡蛋","amount":"2个"}]`)
	storedPlan := WeekPlan{Days: []WeekDayPlan{
		{Date: "2026-06-15", DayName: "周一", Lunch: []models.Dish{dish}, Dinner: []models.Dish{}},
	}}
	storedJSON, _ := json.Marshal(storedPlan)
	if err := database.DB.Create(&models.WeekPlanRecord{
		UserID: CurrentUserID, WeekStart: "2026-06-15", PlanJSON: string(storedJSON),
	}).Error; err != nil {
		t.Fatalf("seed week row: %v", err)
	}

	got := GetCachedWeekPlan()
	if len(got.Days) != 1 || got.Days[0].Lunch[0].ID != dish.ID {
		t.Fatalf("GetCachedWeekPlan should read the week row, got %+v", got.Days)
	}

	// 翻篇：时间推进到下周二，旧行必须原样留存，新周生成新行。
	withPlanNow(t, time.Date(2026, 6, 23, 9, 0, 0, 0, time.Local)) // 下周二，周一=2026-06-22
	planMu.Lock()
	cachedPlan = nil
	cachedWeekKey = ""
	planMu.Unlock()

	_ = GetCachedWeekPlan()

	oldRec, ok := loadWeekPlanRecord("2026-06-15")
	if !ok || oldRec.PlanJSON != string(storedJSON) {
		t.Fatalf("last week's row must survive rollover untouched")
	}
	if _, ok := loadWeekPlanRecord("2026-06-22"); !ok {
		t.Fatalf("rollover must create the new week's row")
	}
}
```

说明：`withPlanNow` 嵌套调用没问题——`t.Cleanup` 按 LIFO 恢复，最终回到真实时钟。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/services/ -run 'TestInvalidateWeekPlan|TestSaveWeekPlanWrites|TestSaveWeekPlanFreezes|TestGetCachedWeekPlanReads' -v`
Expected: FAIL（旧实现写 Setting、改写过去行、Invalidate 删 Setting）。

- [ ] **Step 3: 重写 week_plan_recommendations.go 的写路径**

`backend/internal/services/week_plan_recommendations.go` 中 `SaveWeekPlan`（:13-40）替换为：

```go
// SaveWeekPlan persists the plan as this week's snapshot row and refreshes
// the flat recommendation rows. Two freeze guards live here so EVERY write
// path (generation, regeneration, manual edit PUT) honours read-only history:
// restorePastDaysFromStored swaps days before today back to the stored
// version, and replaceWeekPlanRecommendations never touches past rows.
func SaveWeekPlan(plan *WeekPlan) error {
	if plan == nil {
		plan = &WeekPlan{Days: []WeekDayPlan{}}
	}
	normalizeWeekPlan(plan)
	weekKey := getCurrentWeekKey()
	restorePastDaysFromStored(plan, weekKey)
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := upsertWeekPlanRecordPlan(tx, weekKey, string(data)); err != nil {
			return err
		}
		return replaceWeekPlanRecommendations(tx, plan)
	}); err != nil {
		return err
	}

	planMu.Lock()
	cachedPlan = plan
	cachedWeekKey = weekKey
	planMu.Unlock()
	return nil
}

// restorePastDaysFromStored swaps any day before today back to the stored
// snapshot version (matched by date). History is read-only: a client PUT or
// a full regenerate cannot rewrite what was planned on days already gone.
// First-ever save of a week has no stored row, so nothing to restore.
func restorePastDaysFromStored(plan *WeekPlan, weekStart string) {
	today := todayKey()
	rec, ok := loadWeekPlanRecord(weekStart)
	if !ok || rec.PlanJSON == "" {
		return
	}
	var stored WeekPlan
	if json.Unmarshal([]byte(rec.PlanJSON), &stored) != nil {
		return
	}
	storedByDate := make(map[string]WeekDayPlan, len(stored.Days))
	for _, day := range stored.Days {
		storedByDate[day.Date] = day
	}
	for i, day := range plan.Days {
		if day.Date >= today {
			continue
		}
		if storedDay, exists := storedByDate[day.Date]; exists {
			plan.Days[i] = storedDay
		}
	}
}
```

`replaceWeekPlanRecommendations`（:53-80）替换为：

```go
// replaceWeekPlanRecommendations refreshes the flat rows for today and later
// only. Past rows are immutable history — the cooldown window and the History
// page rely on them recording what was actually planned at the time.
func replaceWeekPlanRecommendations(tx *gorm.DB, plan *WeekPlan) error {
	today := todayKey()
	dates := make([]string, 0, len(plan.Days))
	seenDates := make(map[string]bool, len(plan.Days))
	for _, day := range plan.Days {
		if day.Date == "" || day.Date < today || seenDates[day.Date] {
			continue
		}
		seenDates[day.Date] = true
		dates = append(dates, day.Date)
	}

	if len(dates) > 0 {
		if err := tx.Where("source = ? AND planned_date IN ?", recommendationSourceWeekPlan, dates).
			Delete(&models.DishRecommendation{}).Error; err != nil {
			return err
		}
	}

	var records []models.DishRecommendation
	for _, day := range plan.Days {
		if day.Date == "" || day.Date < today {
			continue
		}
		records = append(records, dishRecommendationsForMeal(day.Date, "lunch", day.Lunch)...)
		records = append(records, dishRecommendationsForMeal(day.Date, "dinner", day.Dinner)...)
	}
	if len(records) == 0 {
		return nil
	}
	return tx.Create(&records).Error
}
```

- [ ] **Step 4: 切换 plan_service.go 的读路径**

`GetCachedWeekPlan`（:59-87）替换为：

```go
func GetCachedWeekPlan() *WeekPlan {
	weekKey := getCurrentWeekKey()

	planMu.RLock()
	if cachedPlan != nil && cachedWeekKey == weekKey {
		p := cachedPlan
		planMu.RUnlock()
		return p
	}
	planMu.RUnlock()

	if rec, ok := loadWeekPlanRecord(weekKey); ok && rec.PlanJSON != "" {
		var plan WeekPlan
		if json.Unmarshal([]byte(rec.PlanJSON), &plan) == nil && len(plan.Days) > 0 {
			planMu.Lock()
			cachedPlan = &plan
			cachedWeekKey = weekKey
			planMu.Unlock()
			return &plan
		}
		// 解析失败按"未生成"处理，重新生成覆盖。
		fmt.Printf("本周菜单存档解析失败，将重新生成（week_start=%s）\n", weekKey)
	}

	plan, _ := GenerateWeekPlan()
	_ = SaveWeekPlan(plan)
	return plan
}
```

`RegenerateWeekPlan` 与 `saveWeekPlanCache`（:89-97）替换为（helper 删除，统一走 SaveWeekPlan）：

```go
func RegenerateWeekPlan() *WeekPlan {
	plan, _ := GenerateWeekPlan()
	_ = SaveWeekPlan(plan)
	return plan
}
```

`GetCachedWeekPlan` 原 `:84-85` 的 `saveWeekPlanCache(plan)` 调用已由上面替换覆盖。

`InvalidateWeekPlanCache`（:99-105）替换为：

```go
// InvalidateWeekPlanCache clears the current week's generated plan so the
// next read regenerates it. The row itself stays (one-off prefs live on it),
// and past weeks are never touched.
func InvalidateWeekPlanCache() {
	database.DB.Model(&models.WeekPlanRecord{}).
		Where("user_id = ? AND week_start = ?", CurrentUserID, getCurrentWeekKey()).
		Update("plan_json", "")
	planMu.Lock()
	cachedPlan = nil
	cachedWeekKey = ""
	planMu.Unlock()
}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `cd backend && go test ./internal/services/ -run 'TestInvalidateWeekPlan|TestSaveWeekPlanWrites|TestSaveWeekPlanFreezes|TestGetCachedWeekPlanReads|TestSaveWeekPlanRecordsAndReplaces' -v`
Expected: 全部 PASS（含未改动的 `TestSaveWeekPlanRecordsAndReplacesRecommendations`——其日期 2099-01-05 恒在未来，新写侧照常替换）。

- [ ] **Step 6: 全量回归 + 提交**

Run: `cd backend && go test ./...`
Expected: 全绿。若 `week_plan_preferences_test.go` 或其它测试因缺 `WeekPlanRecord` 表报 `no such table: week_plans`，把该测试文件 setup 的 AutoMigrate 列表补上 `&models.WeekPlanRecord{}`。

```bash
git add backend/internal/services/week_plan_recommendations.go backend/internal/services/plan_service.go backend/internal/services/plan_service_test.go
git commit -m "feat: store week plan in per-week rows with read-only past days"
```

### Task 3: 生成器冻结过去天 + 手动菜在重生成中幸存

**Files:**
- Modify: `backend/internal/services/plan_service.go`：`WeekDayPlan`（:21-26）、`generateDay`（:176-204）、`GenerateWeekPlan`（:206-230）、`RegenerateWeekPlanDay`（:234-295）、`pickQuotaDishes`（:371-391）
- Modify: `backend/internal/services/plan_service_test.go`（新增 5 个测试）

**背景约束（来自 spec §5.2/§5.3）**：冻结天与手动菜必须计入生成上下文——`globalUsed`（周内去重）、`weekPicked`（周级规则）、`prevSoups`（汤相邻去重）、同餐/同日规则上下文（`mealPicked`/`dayPicked`），否则新排的天会和已固定内容撞车。

- [ ] **Step 1: 写失败测试**

追加到 `backend/internal/services/plan_service_test.go`：

```go
func TestManualDishesForMeal(t *testing.T) {
	a := models.Dish{ID: 1, Name: "甲"}
	b := models.Dish{ID: 2, Name: "乙"}

	kept := manualDishesForMeal([]models.Dish{a, b}, []uint{2, 99})
	if len(kept) != 1 || kept[0].ID != 2 {
		t.Fatalf("manualDishesForMeal = %+v, want only dish 2 (stale id 99 dropped)", kept)
	}
	if got := manualDishesForMeal([]models.Dish{a}, nil); got != nil {
		t.Fatalf("nil manual ids should return nil, got %+v", got)
	}
}

func TestReduceQuotaForKept(t *testing.T) {
	soup := models.Dish{ID: 1, Name: "冬瓜汤", DishRole: "soup", TraitSource: "manual", TraitVersion: models.DishTraitVersion}
	veg := models.Dish{ID: 2, Name: "清炒时蔬", DishRole: "veg", TraitSource: "manual", TraitVersion: models.DishTraitVersion}
	meat := models.Dish{ID: 3, Name: "红烧肉", DishRole: "meat", TraitSource: "manual", TraitVersion: models.DishTraitVersion}

	quota := reduceQuotaForKept(MealQuota{MeatCount: 2, VegCount: 1, SoupCount: 1}, []models.Dish{soup, veg, meat})
	if quota.MeatCount != 1 || quota.VegCount != 0 || quota.SoupCount != 0 {
		t.Fatalf("quota = %+v, want {1 0 0}", quota)
	}

	// 超量保留不把配额扣成负数。
	quota = reduceQuotaForKept(MealQuota{MeatCount: 1}, []models.Dish{meat, meat})
	if quota.MeatCount != 0 {
		t.Fatalf("MeatCount = %d, want 0 (clamped)", quota.MeatCount)
	}
}

func TestGenerateWeekPlanFreezesPastDays(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local)) // 周四

	// 库里只有这些菜可选；冻结天的菜刻意不在库里（菜删了历史也不能变）。
	for i := 0; i < 14; i++ {
		createDishForPlanTest(t, fmt.Sprintf("候选菜%d", i), `["家常菜"]`, `[{"name":"食材","amount":"适量"}]`)
	}
	frozen := models.Dish{ID: 9001, Name: "周一历史菜", DishRole: "meat", TraitSource: "manual", TraitVersion: models.DishTraitVersion}
	storedPlan := WeekPlan{Days: []WeekDayPlan{
		{Date: "2026-06-15", DayName: "周一", Lunch: []models.Dish{frozen}, Dinner: []models.Dish{}},
		{Date: "2026-06-16", DayName: "周二", Lunch: []models.Dish{}, Dinner: []models.Dish{}},
		{Date: "2026-06-17", DayName: "周三", Lunch: []models.Dish{}, Dinner: []models.Dish{}},
	}}
	storedJSON, _ := json.Marshal(storedPlan)
	if err := database.DB.Create(&models.WeekPlanRecord{
		UserID: CurrentUserID, WeekStart: "2026-06-15", PlanJSON: string(storedJSON),
	}).Error; err != nil {
		t.Fatalf("seed week row: %v", err)
	}

	plan, err := GenerateWeekPlan()
	if err != nil {
		t.Fatalf("GenerateWeekPlan error = %v", err)
	}
	if len(plan.Days) != 7 {
		t.Fatalf("days = %d, want 7", len(plan.Days))
	}
	// 周一~三逐字节等于存量（含不在库里的菜）。
	if plan.Days[0].Lunch[0].ID != frozen.ID || plan.Days[0].Lunch[0].Name != "周一历史菜" {
		t.Fatalf("Monday must be copied verbatim, got %+v", plan.Days[0].Lunch)
	}
	if len(plan.Days[1].Lunch) != 0 || len(plan.Days[2].Lunch) != 0 {
		t.Fatalf("Tue/Wed (stored empty) must stay empty, got %+v / %+v", plan.Days[1].Lunch, plan.Days[2].Lunch)
	}
	// 周四起正常生成（库里有菜就该排出来）。
	if len(plan.Days[3].Lunch)+len(plan.Days[3].Dinner) == 0 {
		t.Fatalf("Thursday onward must be generated")
	}
	// 冻结菜计入周内去重：新排的天不得再出现它。
	for i := 3; i < 7; i++ {
		for _, dish := range appendDishSlices(plan.Days[i].Lunch, plan.Days[i].Dinner) {
			if dish.ID == frozen.ID {
				t.Fatalf("frozen dish leaked into generated day %d", i)
			}
		}
	}
}

func TestGenerateWeekPlanKeepsManualDishes(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local)) // 周四

	manual := createDishForPlanTest(t, "点名要吃", `["家常菜"]`, `[{"name":"牛腩","amount":"500g"}]`)
	for i := 0; i < 14; i++ {
		createDishForPlanTest(t, fmt.Sprintf("普通菜%d", i), `["家常菜"]`, `[{"name":"食材","amount":"适量"}]`)
	}
	storedPlan := WeekPlan{Days: []WeekDayPlan{
		{Date: "2026-06-19", DayName: "周五",
			Lunch:          []models.Dish{manual},
			Dinner:         []models.Dish{},
			ManualLunchIDs: []uint{manual.ID}},
	}}
	storedJSON, _ := json.Marshal(storedPlan)
	if err := database.DB.Create(&models.WeekPlanRecord{
		UserID: CurrentUserID, WeekStart: "2026-06-15", PlanJSON: string(storedJSON),
	}).Error; err != nil {
		t.Fatalf("seed week row: %v", err)
	}

	plan, err := GenerateWeekPlan()
	if err != nil {
		t.Fatalf("GenerateWeekPlan error = %v", err)
	}
	friday := plan.Days[4] // 0=周一
	if friday.Date != "2026-06-19" {
		t.Fatalf("day index mismatch: %s", friday.Date)
	}
	foundManual := false
	for _, dish := range friday.Lunch {
		if dish.ID == manual.ID {
			foundManual = true
		}
	}
	if !foundManual {
		t.Fatalf("manual dish must survive full regenerate, lunch = %+v", friday.Lunch)
	}
	if len(friday.ManualLunchIDs) != 1 || friday.ManualLunchIDs[0] != manual.ID {
		t.Fatalf("ManualLunchIDs must be carried, got %v", friday.ManualLunchIDs)
	}
}

func TestRegenerateWeekPlanDayRejectsPastDate(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local)) // 周四

	dish := createDishForPlanTest(t, "占位菜", `["家常菜"]`, `[{"name":"食材","amount":"适量"}]`)
	storedPlan := WeekPlan{Days: []WeekDayPlan{
		{Date: "2026-06-16", DayName: "周二", Lunch: []models.Dish{dish}, Dinner: []models.Dish{}},
	}}
	storedJSON, _ := json.Marshal(storedPlan)
	if err := database.DB.Create(&models.WeekPlanRecord{
		UserID: CurrentUserID, WeekStart: "2026-06-15", PlanJSON: string(storedJSON),
	}).Error; err != nil {
		t.Fatalf("seed week row: %v", err)
	}

	if _, err := RegenerateWeekPlanDay("2026-06-16"); err == nil {
		t.Fatalf("regenerating a past day must fail")
	} else if !strings.Contains(err.Error(), "过去的天") {
		t.Fatalf("error = %q, want 过去的天 message", err.Error())
	}
}
```

说明：`fmt`、`strings`、`json`、`time` 在该测试文件均已 import（见文件头 :1-14）。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/services/ -run 'TestManualDishesForMeal|TestReduceQuotaForKept|TestGenerateWeekPlanFreezes|TestGenerateWeekPlanKeepsManual|TestRegenerateWeekPlanDayRejects' -v`
Expected: 编译错误 `undefined: manualDishesForMeal` 等。

- [ ] **Step 3: WeekDayPlan 加字段 + 两个 helper**

`plan_service.go` 的 `WeekDayPlan`（:21-26）替换为：

```go
type WeekDayPlan struct {
	Date    string        `json:"date"`
	DayName string        `json:"day_name"`
	Lunch   []models.Dish `json:"lunch"`
	Dinner  []models.Dish `json:"dinner"`
	// Manual*IDs mark dishes the user added by hand (split per meal so a dish
	// added manually to lunch is not mis-marked when the system also picks it
	// for dinner). They survive regeneration: see generateDay's keep params.
	ManualLunchIDs  []uint `json:"manual_lunch_ids,omitempty"`
	ManualDinnerIDs []uint `json:"manual_dinner_ids,omitempty"`
}
```

在 `soupsFromDishes`（:299 附近）旁追加两个 helper：

```go
// manualDishesForMeal returns the dishes in the slot that the user added by
// hand (ids listed in manualIDs). Stale ids — manual dishes since removed
// from the slot — drop out naturally.
func manualDishesForMeal(dishes []models.Dish, manualIDs []uint) []models.Dish {
	if len(manualIDs) == 0 {
		return nil
	}
	wanted := make(map[uint]bool, len(manualIDs))
	for _, id := range manualIDs {
		wanted[id] = true
	}
	var kept []models.Dish
	for _, dish := range dishes {
		if wanted[dish.ID] {
			kept = append(kept, dish)
		}
	}
	return kept
}

// reduceQuotaForKept subtracts the slots already occupied by kept (manual)
// dishes, matching each dish to the slot type it would fill with the same
// test matchesWeekPlanSlot uses (soup, veg, else meat).
func reduceQuotaForKept(quota MealQuota, kept []models.Dish) MealQuota {
	for _, dish := range kept {
		d := ensureDishTraits(dish)
		switch {
		case d.DishRole == "soup" || isSoupDish(d):
			if quota.SoupCount > 0 {
				quota.SoupCount--
			}
		case d.DishRole == "veg":
			if quota.VegCount > 0 {
				quota.VegCount--
			}
		default:
			if quota.MeatCount > 0 {
				quota.MeatCount--
			}
		}
	}
	return quota
}
```

- [ ] **Step 4: pickQuotaDishes 接收预占菜**

`pickQuotaDishes`（:371）签名加最后一个参数 `keep []models.Dish`，并把内部 `picked` 的初始化与返回语义改为"含 keep 的完整餐列表"。改动后函数开头：

```go
func pickQuotaDishes(pool []models.Dish, quota MealQuota, dayCtx weekPlanDayContext, mealType string, dayName string, globalUsed map[uint]bool, dayUsed map[uint]bool, recent map[uint]bool, dayPicked []models.Dish, weekPicked []models.Dish, rules []compiledMenuRule, warnings *[]string, r *rand.Rand, keep []models.Dish) []models.Dish {
	quota = normalizeMealQuota(quota)
	// Manual dishes occupy their slots up front: they sit in picked so every
	// same-meal rule context sees them, and quota was already reduced by the
	// caller (reduceQuotaForKept).
	picked := append([]models.Dish(nil), keep...)
	if quota.total() <= 0 {
		return picked
	}
	for _, slot := range weekPlanSlots(quota) {
```

（原 `var picked []models.Dish` 删除；原 `if quota.total() <= 0 { return []models.Dish{} }` 由上面的带 keep 返回替代；函数其余部分——循环体、warning 逻辑、`return picked`——不变。）

- [ ] **Step 5: generateDay 接收 keep 并接入上下文**

`generateDay`（:176-204）整体替换为：

```go
// generateDay fills one weekday (0=Monday). globalUsed and weekPicked carry
// cross-day dedup state and are updated with the picked dishes. keepLunch /
// keepDinner are manually-added dishes that survive regeneration: they take
// their quota slots up front, join every dedup set and rule context, and the
// picker only fills what is left.
func (ctx *weekPlanGenContext) generateDay(dayIndex int, date time.Time, globalUsed map[uint]bool, weekPicked *[]models.Dish, prevSoups []models.Dish, keepLunch []models.Dish, keepDinner []models.Dish) WeekDayPlan {
	periodPrefs := ctx.prefs.Weekday
	if dayIndex >= 5 {
		periodPrefs = ctx.prefs.Weekend
	}
	dayCtx, lunchQuota, dinnerQuota := resolveWeekPlanDay(ctx.prefs, periodPrefs, dayIndex)
	dayCtx.categoryFavorites = ctx.categoryFavorites
	dayCtx.prevSoups = prevSoups
	dayPlan := WeekDayPlan{
		Date:   date.Format("2006-01-02"),
		Lunch:  []models.Dish{},
		Dinner: []models.Dish{},
	}
	if dayIndex >= 0 && dayIndex < len(weekPlanDayNames) {
		dayPlan.DayName = weekPlanDayNames[dayIndex]
	}
	for _, dish := range keepLunch {
		dayPlan.ManualLunchIDs = append(dayPlan.ManualLunchIDs, dish.ID)
	}
	for _, dish := range keepDinner {
		dayPlan.ManualDinnerIDs = append(dayPlan.ManualDinnerIDs, dish.ID)
	}

	usedThisDay := make(map[uint]bool)
	var dayPicked []models.Dish
	for _, dish := range appendDishSlices(keepLunch, keepDinner) {
		globalUsed[dish.ID] = true
		usedThisDay[dish.ID] = true
		dayPicked = append(dayPicked, dish)
		*weekPicked = append(*weekPicked, dish)
	}

	dayPlan.Lunch = pickQuotaDishes(ctx.lunchPool, reduceQuotaForKept(lunchQuota, keepLunch), dayCtx, "lunch", dayPlan.DayName, globalUsed, usedThisDay, ctx.recent, dayPicked, *weekPicked, ctx.rules, &ctx.warnings, ctx.r, keepLunch)
	for _, dish := range dayPlan.Lunch {
		if !containsDishID(keepLunch, dish.ID) {
			dayPicked = append(dayPicked, dish)
			*weekPicked = append(*weekPicked, dish)
		}
	}
	dayPlan.Dinner = pickQuotaDishes(ctx.dinnerPool, reduceQuotaForKept(dinnerQuota, keepDinner), dayCtx, "dinner", dayPlan.DayName, globalUsed, usedThisDay, ctx.recent, dayPicked, *weekPicked, ctx.rules, &ctx.warnings, ctx.r, keepDinner)
	for _, dish := range dayPlan.Dinner {
		if !containsDishID(keepDinner, dish.ID) {
			dayPicked = append(dayPicked, dish)
			*weekPicked = append(*weekPicked, dish)
		}
	}

	return dayPlan
}

// containsDishID reports whether dishes already contains id (used to avoid
// double-counting keep dishes that were pre-registered in the dedup sets).
func containsDishID(dishes []models.Dish, id uint) bool {
	for _, dish := range dishes {
		if dish.ID == id {
			return true
		}
	}
	return false
}
```

- [ ] **Step 6: GenerateWeekPlan 冻结复制**

`GenerateWeekPlan`（:206-230）替换为：

```go
func GenerateWeekPlan() (*WeekPlan, error) {
	ctx := buildWeekPlanGenContext()
	if ctx == nil {
		return &WeekPlan{}, nil
	}

	now := planNow()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := now.AddDate(0, 0, 1-weekday)
	today := todayKey()

	// Freeze guard: days already gone are copied verbatim from the stored
	// snapshot (read-only history), and manual dishes on remaining days are
	// handed to generateDay so they survive the regenerate.
	var storedDays map[string]WeekDayPlan
	if rec, ok := loadWeekPlanRecord(getCurrentWeekKey()); ok && rec.PlanJSON != "" {
		var stored WeekPlan
		if json.Unmarshal([]byte(rec.PlanJSON), &stored) == nil {
			storedDays = make(map[string]WeekDayPlan, len(stored.Days))
			for _, day := range stored.Days {
				storedDays[day.Date] = day
			}
		}
	}

	var days []WeekDayPlan
	globalUsed := make(map[uint]bool)
	var weekPicked []models.Dish
	var prevSoups []models.Dish
	for i := 0; i < 7; i++ {
		date := monday.AddDate(0, 0, i)
		dateKey := date.Format("2006-01-02")
		storedDay, hasStored := storedDays[dateKey]

		if dateKey < today && hasStored {
			for _, dish := range appendDishSlices(storedDay.Lunch, storedDay.Dinner) {
				globalUsed[dish.ID] = true
				weekPicked = append(weekPicked, dish)
			}
			days = append(days, storedDay)
			prevSoups = soupsFromDishes(appendDishSlices(storedDay.Lunch, storedDay.Dinner))
			continue
		}

		var keepLunch, keepDinner []models.Dish
		if hasStored {
			keepLunch = manualDishesForMeal(storedDay.Lunch, storedDay.ManualLunchIDs)
			keepDinner = manualDishesForMeal(storedDay.Dinner, storedDay.ManualDinnerIDs)
		}
		day := ctx.generateDay(i, date, globalUsed, &weekPicked, prevSoups, keepLunch, keepDinner)
		days = append(days, day)
		prevSoups = soupsFromDishes(appendDishSlices(day.Lunch, day.Dinner))
	}

	return &WeekPlan{Days: days, Warnings: uniqueWarnings(ctx.warnings)}, nil
}
```

- [ ] **Step 7: RegenerateWeekPlanDay 拒绝过去 + 保留手动菜**

`RegenerateWeekPlanDay`（:234-295）做两处修改。

在 `date = strings.TrimSpace(date)` 之后、`current := GetCachedWeekPlan()` 之前插入：

```go
	if date < todayKey() {
		return nil, fmt.Errorf("过去的天不能重新生成")
	}
```

原 `:278` 的生成调用替换为：

```go
	targetDay := current.Days[dayIndex]
	keepLunch := manualDishesForMeal(targetDay.Lunch, targetDay.ManualLunchIDs)
	keepDinner := manualDishesForMeal(targetDay.Dinner, targetDay.ManualDinnerIDs)
	newPlan.Days[dayIndex] = ctx.generateDay(dayIndex, parsedDate, globalUsed, &weekPicked, adjacentPlannedSoups(newPlan.Days, dayIndex), keepLunch, keepDinner)
```

- [ ] **Step 8: 跑测试确认通过**

Run: `cd backend && go test ./internal/services/ -run 'TestManualDishesForMeal|TestReduceQuotaForKept|TestGenerateWeekPlanFreezes|TestGenerateWeekPlanKeepsManual|TestRegenerateWeekPlanDayRejects' -v`
Expected: 5 个测试 PASS。

- [ ] **Step 9: 全量回归 + 提交**

Run: `cd backend && go test ./...`
Expected: 全绿。注意 `TestSoupSchedulingRulesViaScoreEvaluation` 等既有生成测试不应受影响（generateDay 新参数传 nil/nil 即旧行为）。

```bash
git add backend/internal/services/plan_service.go backend/internal/services/plan_service_test.go
git commit -m "feat: freeze past days and keep manual dishes through regeneration"
```

### Task 4: 偏好拆分——常驻进 Setting、本周一次性进周行

**Files:**
- Modify: `backend/internal/services/week_plan_preferences.go:52-80`（GetWeekPlanPreferences、SaveWeekPlanPreferences，新增 weekPlanOneOffPrefs 类型）
- Modify: `backend/internal/services/week_plan_preferences_test.go`（新增 2 个测试）

**API 形状不变**：`WeekPlanPreferences` 结构体、GET/PUT 端点 JSON 完全不动；只拆存储。

- [ ] **Step 1: 写失败测试**

追加到 `backend/internal/services/week_plan_preferences_test.go`（若该文件 setup 用独立的 AutoMigrate 列表且缺 `WeekPlanRecord`，先补上；若复用 `setupPlanServiceTestDB` 则 Task 1 已覆盖）：

```go
func TestWeekPlanPreferencesOneOffPartsLiveOnWeekRow(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local)) // 周一=2026-06-15

	prefs := WeekPlanPreferences{
		Weekday:  WeekPlanPeriodPreferences{Profile: "quick", Lunch: MealQuota{MeatCount: 2}, Dinner: MealQuota{MeatCount: 1, SoupCount: 1}},
		Weekend:  WeekPlanPeriodPreferences{Profile: "balanced", Lunch: MealQuota{MeatCount: 1}, Dinner: MealQuota{MeatCount: 2}},
		WeekWant: []string{"beef"},
		Days:     map[string]DayOverride{"wed": {Want: []string{"seafood"}}},
	}
	if err := SaveWeekPlanPreferences(prefs); err != nil {
		t.Fatalf("SaveWeekPlanPreferences error = %v", err)
	}

	// 常驻 Setting 里不得残留一次性字段。
	var setting models.Setting
	if err := database.DB.Where("`key` = ?", "week_plan_preferences").First(&setting).Error; err != nil {
		t.Fatalf("resident setting missing: %v", err)
	}
	if strings.Contains(setting.Value, "week_want") || strings.Contains(setting.Value, `"days"`) {
		t.Fatalf("resident setting must not contain one-off parts: %s", setting.Value)
	}

	// 一次性部分落在本周行。
	rec, ok := loadWeekPlanRecord("2026-06-15")
	if !ok || rec.PrefsJSON == "" {
		t.Fatalf("one-off prefs must land on the week row, ok=%v", ok)
	}
	if !strings.Contains(rec.PrefsJSON, "beef") || !strings.Contains(rec.PrefsJSON, "seafood") {
		t.Fatalf("PrefsJSON = %s, want want-lists", rec.PrefsJSON)
	}

	// 合成视图完整 round-trip。
	got := GetWeekPlanPreferences()
	if got.Weekday.Profile != "quick" || len(got.WeekWant) != 1 || got.WeekWant[0] != "beef" {
		t.Fatalf("composed prefs = %+v", got)
	}
	if got.Days["wed"].Want[0] != "seafood" {
		t.Fatalf("day override lost: %+v", got.Days)
	}
}

func TestWeekPlanPreferencesOneOffPartsResetOnRollover(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local))

	prefs := WeekPlanPreferences{
		Weekday:  WeekPlanPeriodPreferences{Profile: "light", Lunch: MealQuota{VegCount: 2}, Dinner: MealQuota{MeatCount: 1}},
		Weekend:  WeekPlanPeriodPreferences{Profile: "balanced", Lunch: MealQuota{MeatCount: 1}, Dinner: MealQuota{MeatCount: 1}},
		WeekWant: []string{"pork"},
		Days:     map[string]DayOverride{"fri": {Profile: "soup"}},
	}
	if err := SaveWeekPlanPreferences(prefs); err != nil {
		t.Fatalf("SaveWeekPlanPreferences error = %v", err)
	}

	// 翻篇到下周。
	withPlanNow(t, time.Date(2026, 6, 23, 9, 0, 0, 0, time.Local))

	got := GetWeekPlanPreferences()
	if len(got.WeekWant) != 0 {
		t.Fatalf("WeekWant must reset on rollover, got %v", got.WeekWant)
	}
	if len(got.Days) != 0 {
		t.Fatalf("Days overrides must reset on rollover, got %+v", got.Days)
	}
	if got.Weekday.Profile != "light" || got.Weekday.Lunch.VegCount != 2 {
		t.Fatalf("resident parts must survive rollover, got %+v", got.Weekday)
	}
}
```

说明：该测试文件需要 import `strings`、`time`、`ninimenu/internal/database`、`ninimenu/internal/models`（按现有 import 块补缺）。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/services/ -run 'TestWeekPlanPreferencesOneOff' -v`
Expected: FAIL——现实现把整份 prefs 存 Setting，一次性字段残留且跨周不清零。

- [ ] **Step 3: 实现拆分**

`backend/internal/services/week_plan_preferences.go`：

在 `WeekPlanPreferences` 类型定义后追加：

```go
// weekPlanOneOffPrefs is the per-week slice of the preferences: the "want"
// wishes and day overrides only apply to one week and live on that week's
// row (week_plans.prefs_json), so they reset naturally on rollover. The
// resident quotas/profiles stay in the Setting row.
type weekPlanOneOffPrefs struct {
	WeekWant []string               `json:"week_want,omitempty"`
	Days     map[string]DayOverride `json:"days,omitempty"`
}
```

`GetWeekPlanPreferences`（:52-61）替换为：

```go
func GetWeekPlanPreferences() WeekPlanPreferences {
	prefs := defaultWeekPlanPreferencesFromSettings()
	var setting models.Setting
	if err := database.DB.Where("`key` = ?", weekPlanPreferencesSettingKey).First(&setting).Error; err == nil && setting.Value != "" {
		var stored WeekPlanPreferences
		if json.Unmarshal([]byte(setting.Value), &stored) == nil {
			prefs = stored
		}
	}
	// One-off parts never come from the resident setting; they live on this
	// week's row and reset when the week rolls over.
	prefs.WeekWant = nil
	prefs.Days = nil
	if rec, ok := loadWeekPlanRecord(getCurrentWeekKey()); ok && rec.PrefsJSON != "" {
		var oneOff weekPlanOneOffPrefs
		if json.Unmarshal([]byte(rec.PrefsJSON), &oneOff) == nil {
			prefs.WeekWant = oneOff.WeekWant
			prefs.Days = oneOff.Days
		}
	}
	return normalizeWeekPlanPreferences(prefs)
}
```

`SaveWeekPlanPreferences`（:63-80）替换为（**保留**原"不失效缓存"注释）：

```go
func SaveWeekPlanPreferences(prefs WeekPlanPreferences) error {
	prefs = normalizeWeekPlanPreferences(prefs)

	oneOffData, err := json.Marshal(weekPlanOneOffPrefs{WeekWant: prefs.WeekWant, Days: prefs.Days})
	if err != nil {
		return err
	}

	resident := prefs
	resident.WeekWant = nil
	resident.Days = nil
	data, err := json.Marshal(resident)
	if err != nil {
		return err
	}
	if err := database.DB.Model(&models.Setting{}).
		Where("`key` = ?", weekPlanPreferencesSettingKey).
		Assign(models.Setting{Key: weekPlanPreferencesSettingKey, Value: string(data)}).
		FirstOrCreate(&models.Setting{}).Error; err != nil {
		return err
	}
	if err := upsertWeekPlanRecordPrefs(getCurrentWeekKey(), string(oneOffData)); err != nil {
		return err
	}
	// Deliberately NOT invalidating the cached week plan here: preference
	// changes take effect on the next explicit regenerate (应用设置/重新生成),
	// and 仅这天重生成 right after a day-theme save must keep working off
	// the existing plan instead of silently rebuilding all seven days.
	return nil
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd backend && go test ./internal/services/ -run 'TestWeekPlanPreferences' -v`
Expected: 新旧测试全 PASS（含既有 `TestSaveWeekPlanPreferencesRoundTripsDays`——Save→Get round-trip 语义未变，只是存储位置拆了）。

- [ ] **Step 5: 全量回归 + 提交**

Run: `cd backend && go test ./...`
Expected: 全绿。

```bash
git add backend/internal/services/week_plan_preferences.go backend/internal/services/week_plan_preferences_test.go
git commit -m "feat: store one-off week preferences on the week row"
```

---

### Task 5: 启动迁移 MigrateWeekPlanStorage

**Files:**
- Modify: `backend/internal/services/week_plan_store.go`（追加迁移函数）
- Modify: `backend/cmd/server/main.go`（启动调用）
- Modify: `backend/internal/services/week_plan_store_test.go`（新增 3 个测试）

依赖：Task 4 的 `weekPlanOneOffPrefs` 类型。

- [ ] **Step 1: 写失败测试**

追加到 `backend/internal/services/week_plan_store_test.go`：

```go
func TestMigrateWeekPlanStorageMovesCurrentWeekCache(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local)) // 周一=2026-06-15

	legacy := `{"days":[{"date":"2026-06-15","day_name":"周一","lunch":[],"dinner":[]}],"warnings":[]}`
	if err := database.DB.Create(&models.Setting{Key: "week_plan_cache", Value: legacy}).Error; err != nil {
		t.Fatalf("seed legacy cache: %v", err)
	}

	if err := MigrateWeekPlanStorage(); err != nil {
		t.Fatalf("MigrateWeekPlanStorage error = %v", err)
	}

	rec, ok := loadWeekPlanRecord("2026-06-15")
	if !ok || rec.PlanJSON != legacy {
		t.Fatalf("current-week cache must move into the row, ok=%v", ok)
	}
	var count int64
	database.DB.Model(&models.Setting{}).Where("`key` = ?", "week_plan_cache").Count(&count)
	if count != 0 {
		t.Fatalf("legacy cache key must be deleted, count=%d", count)
	}

	// 幂等：再跑一遍是 no-op。
	if err := MigrateWeekPlanStorage(); err != nil {
		t.Fatalf("second run error = %v", err)
	}
	rec, _ = loadWeekPlanRecord("2026-06-15")
	if rec.PlanJSON != legacy {
		t.Fatalf("second run must not change the row")
	}
}

func TestMigrateWeekPlanStorageDropsStaleCache(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local))

	stale := `{"days":[{"date":"2026-06-08","day_name":"周一","lunch":[],"dinner":[]}],"warnings":[]}`
	if err := database.DB.Create(&models.Setting{Key: "week_plan_cache", Value: stale}).Error; err != nil {
		t.Fatalf("seed stale cache: %v", err)
	}

	if err := MigrateWeekPlanStorage(); err != nil {
		t.Fatalf("MigrateWeekPlanStorage error = %v", err)
	}

	if _, ok := loadWeekPlanRecord("2026-06-15"); ok {
		t.Fatalf("stale cache must not create a row for the current week")
	}
	if _, ok := loadWeekPlanRecord("2026-06-08"); ok {
		t.Fatalf("stale cache must not backfill past weeks either (history starts at upgrade)")
	}
	var count int64
	database.DB.Model(&models.Setting{}).Where("`key` = ?", "week_plan_cache").Count(&count)
	if count != 0 {
		t.Fatalf("legacy cache key must be deleted even when stale, count=%d", count)
	}
}

func TestMigrateWeekPlanStorageSplitsLegacyPrefs(t *testing.T) {
	setupPlanServiceTestDB(t)
	withPlanNow(t, time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local))

	legacyPrefs := `{"weekday":{"profile":"quick","lunch":{"meat_count":2,"veg_count":0,"soup_count":0},"dinner":{"meat_count":1,"veg_count":1,"soup_count":1}},"weekend":{"profile":"balanced","lunch":{"meat_count":1,"veg_count":1,"soup_count":0},"dinner":{"meat_count":2,"veg_count":1,"soup_count":1}},"week_want":["beef"],"days":{"wed":{"want":["seafood"]}}}`
	if err := database.DB.Create(&models.Setting{Key: "week_plan_preferences", Value: legacyPrefs}).Error; err != nil {
		t.Fatalf("seed legacy prefs: %v", err)
	}

	if err := MigrateWeekPlanStorage(); err != nil {
		t.Fatalf("MigrateWeekPlanStorage error = %v", err)
	}

	rec, ok := loadWeekPlanRecord("2026-06-15")
	if !ok || !strings.Contains(rec.PrefsJSON, "beef") || !strings.Contains(rec.PrefsJSON, "seafood") {
		t.Fatalf("one-off prefs must move to the week row, got %q", rec.PrefsJSON)
	}

	var setting models.Setting
	if err := database.DB.Where("`key` = ?", "week_plan_preferences").First(&setting).Error; err != nil {
		t.Fatalf("resident prefs setting must remain: %v", err)
	}
	if strings.Contains(setting.Value, "week_want") || strings.Contains(setting.Value, `"days"`) {
		t.Fatalf("one-off parts must be stripped from the setting: %s", setting.Value)
	}
	if !strings.Contains(setting.Value, "quick") {
		t.Fatalf("resident parts must survive: %s", setting.Value)
	}

	// 合成读取仍能看到迁移过来的一次性偏好。
	got := GetWeekPlanPreferences()
	if len(got.WeekWant) != 1 || got.WeekWant[0] != "beef" {
		t.Fatalf("composed prefs after migration = %+v", got)
	}
}
```

`week_plan_store_test.go` 需要补 import `strings`。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/services/ -run 'TestMigrateWeekPlanStorage' -v`
Expected: 编译错误 `undefined: MigrateWeekPlanStorage`。

- [ ] **Step 3: 实现迁移函数**

追加到 `backend/internal/services/week_plan_store.go`（import 块补 `encoding/json`）：

```go
// MigrateWeekPlanStorage moves the legacy single-slot Setting rows into the
// per-week week_plans table. Idempotent: each legacy key is deleted (cache)
// or stripped (prefs) after handling, so reruns are no-ops. Old weeks are NOT
// backfilled — the legacy design already discarded them, so history starts
// at the upgrade. Called from main at boot; a failure only warns because the
// worst case is regenerating the current week.
func MigrateWeekPlanStorage() error {
	weekKey := getCurrentWeekKey()

	var cacheSetting models.Setting
	if err := database.DB.Where("`key` = ?", "week_plan_cache").First(&cacheSetting).Error; err == nil {
		var plan WeekPlan
		if cacheSetting.Value != "" && json.Unmarshal([]byte(cacheSetting.Value), &plan) == nil &&
			len(plan.Days) > 0 && plan.Days[0].Date == weekKey {
			if err := upsertWeekPlanRecordPlan(database.DB, weekKey, cacheSetting.Value); err != nil {
				return err
			}
		}
		if err := database.DB.Where("`key` = ?", "week_plan_cache").Delete(&models.Setting{}).Error; err != nil {
			return err
		}
	}

	var prefsSetting models.Setting
	if err := database.DB.Where("`key` = ?", weekPlanPreferencesSettingKey).First(&prefsSetting).Error; err == nil && prefsSetting.Value != "" {
		var prefs WeekPlanPreferences
		if json.Unmarshal([]byte(prefsSetting.Value), &prefs) == nil &&
			(len(prefs.WeekWant) > 0 || len(prefs.Days) > 0) {
			oneOffData, err := json.Marshal(weekPlanOneOffPrefs{WeekWant: prefs.WeekWant, Days: prefs.Days})
			if err != nil {
				return err
			}
			if err := upsertWeekPlanRecordPrefs(weekKey, string(oneOffData)); err != nil {
				return err
			}
			prefs.WeekWant = nil
			prefs.Days = nil
			residentData, err := json.Marshal(prefs)
			if err != nil {
				return err
			}
			if err := database.DB.Model(&models.Setting{}).
				Where("`key` = ?", weekPlanPreferencesSettingKey).
				Update("value", string(residentData)).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: 挂到启动**

`backend/cmd/server/main.go`，在 `EnsureDefaultMenuRules` 块之后插入：

```go
	// Moves the legacy week-plan cache/prefs Settings into week_plans rows;
	// idempotent, worst failure case is regenerating the current week.
	if err := services.MigrateWeekPlanStorage(); err != nil {
		fmt.Printf("周菜单存储迁移失败: %v\n", err)
	}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `cd backend && go test ./internal/services/ -run 'TestMigrateWeekPlanStorage' -v`
Expected: 3 个测试 PASS。

- [ ] **Step 6: 全量回归 + 提交**

Run: `cd backend && go test ./... && go build ./...`
Expected: 全绿、可编译。

```bash
git add backend/internal/services/week_plan_store.go backend/internal/services/week_plan_store_test.go backend/cmd/server/main.go
git commit -m "feat: migrate legacy week-plan settings into week_plans rows at boot"
```

### Task 6: 回看查询端点 GET /week-plan/history

**Files:**
- Create: `backend/internal/services/week_plan_history.go`
- Create: `backend/internal/services/week_plan_history_test.go`
- Modify: `backend/internal/handlers/plan.go`（追加 handler）
- Modify: `backend/internal/routes/routes.go:73`（追加路由）

- [ ] **Step 1: 写失败测试**

新建 `backend/internal/services/week_plan_history_test.go`：

```go
package services

import (
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"testing"
)

func TestWeekPlanHistoryByMonthGroupsByDate(t *testing.T) {
	setupPlanServiceTestDB(t)

	rows := []models.DishRecommendation{
		{DishID: 1, DishName: "周一午餐菜", Source: "week_plan", MealType: "lunch", PlannedDate: "2026-06-15"},
		{DishID: 2, DishName: "周一晚餐汤", Source: "week_plan", MealType: "dinner", PlannedDate: "2026-06-15"},
		{DishID: 3, DishName: "周二的菜", Source: "week_plan", MealType: "lunch", PlannedDate: "2026-06-16"},
		// 下月与异 source 都不得混入。
		{DishID: 4, DishName: "七月的菜", Source: "week_plan", MealType: "lunch", PlannedDate: "2026-07-01"},
		{DishID: 5, DishName: "别的来源", Source: "tomorrow_pick", MealType: "lunch", PlannedDate: "2026-06-15"},
	}
	if err := database.DB.Create(&rows).Error; err != nil {
		t.Fatalf("seed rows: %v", err)
	}

	got, err := WeekPlanHistoryByMonth("2026-06")
	if err != nil {
		t.Fatalf("WeekPlanHistoryByMonth error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("dates = %d, want 2: %+v", len(got), got)
	}
	mon := got["2026-06-15"]
	if len(mon) != 2 || mon[0].DishName != "周一晚餐汤" || mon[1].DishName != "周一午餐菜" {
		// meal_type ASC: dinner < lunch（字典序），同日内先 dinner 后 lunch。
		t.Fatalf("monday entries = %+v", mon)
	}
	if len(got["2026-06-16"]) != 1 || got["2026-06-16"][0].DishID != 3 {
		t.Fatalf("tuesday entries = %+v", got["2026-06-16"])
	}
}

func TestWeekPlanHistoryByMonthValidatesMonth(t *testing.T) {
	setupPlanServiceTestDB(t)

	for _, bad := range []string{"2026-6", "202606", "2026-13", "abcd-ef", ""} {
		if _, err := WeekPlanHistoryByMonth(bad); err == nil {
			t.Fatalf("month %q must be rejected", bad)
		}
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd backend && go test ./internal/services/ -run 'TestWeekPlanHistoryByMonth' -v`
Expected: 编译错误 `undefined: WeekPlanHistoryByMonth`。

- [ ] **Step 3: 实现查询**

新建 `backend/internal/services/week_plan_history.go`：

```go
package services

import (
	"fmt"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"regexp"
	"time"
)

// PlannedDishEntry is one dish of a stored day plan, served to the History
// page so it can show "planned vs eaten" for a day. DishName is a snapshot:
// it stays readable even after the dish is deleted from the library.
type PlannedDishEntry struct {
	DishID   uint   `json:"dish_id"`
	DishName string `json:"dish_name"`
	MealType string `json:"meal_type"`
}

var historyMonthPattern = regexp.MustCompile(`^\d{4}-\d{2}$`)

// WeekPlanHistoryByMonth returns the planned dishes recorded for each date of
// the month, keyed by YYYY-MM-DD. Rows come from dish_recommendations, whose
// past entries are immutable (SaveWeekPlan only rewrites today and later), so
// this is a faithful record of what was planned at the time.
func WeekPlanHistoryByMonth(month string) (map[string][]PlannedDishEntry, error) {
	if !historyMonthPattern.MatchString(month) {
		return nil, fmt.Errorf("月份格式无效，应为 YYYY-MM")
	}
	if _, err := time.Parse("2006-01", month); err != nil {
		return nil, fmt.Errorf("月份格式无效，应为 YYYY-MM")
	}

	var rows []models.DishRecommendation
	// 字符串 BETWEEN：月内任何日期 <= "-31"，对短月份同样安全。
	if err := database.DB.
		Where("source = ? AND planned_date BETWEEN ? AND ?", recommendationSourceWeekPlan, month+"-01", month+"-31").
		Order("planned_date ASC, meal_type ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[string][]PlannedDishEntry, len(rows))
	for _, row := range rows {
		result[row.PlannedDate] = append(result[row.PlannedDate], PlannedDishEntry{
			DishID:   row.DishID,
			DishName: row.DishName,
			MealType: row.MealType,
		})
	}
	return result, nil
}
```

- [ ] **Step 4: handler 与路由**

`backend/internal/handlers/plan.go`，在 `GetWeekPlanHistoryHandler` 命名下追加（放 `RegenerateWeekPlanDayHandler` 之后）：

```go
func GetWeekPlanHistoryHandler(c *gin.Context) {
	history, err := services.WeekPlanHistoryByMonth(c.Query("month"))
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, history)
}
```

`backend/internal/routes/routes.go`，在 `app.POST("/week-plan/regenerate-day", ...)` 之后追加：

```go
				app.GET("/week-plan/history", handlers.GetWeekPlanHistoryHandler)
```

- [ ] **Step 5: 跑测试确认通过 + 提交**

Run: `cd backend && go test ./internal/services/ -run 'TestWeekPlanHistoryByMonth' -v && go build ./...`
Expected: 2 个测试 PASS，编译通过。

```bash
git add backend/internal/services/week_plan_history.go backend/internal/services/week_plan_history_test.go backend/internal/handlers/plan.go backend/internal/routes/routes.go
git commit -m "feat: add week-plan history endpoint for the History page"
```

---

### Task 7: 前端——手动菜标记 + History 页回看

**Files:**
- Modify: `frontend/src/types/index.ts:181-191`（WeekDayPlan 加字段；新增 PlannedDishEntry）
- Modify: `frontend/src/api/index.ts:88-98`（weekPlanApi.history）
- Modify: `frontend/src/pages/WeekPlan.tsx:852-872`（addDish/removeDish 维护 manual ids）
- Modify: `frontend/src/pages/History.tsx`（选中日显示当天计划）

前端无单测基建，验证方式为 `npm run build`（类型检查）+ Task 8 的手工冒烟。

- [ ] **Step 1: 类型**

`frontend/src/types/index.ts` 的 `WeekDayPlan`（:181-186）替换为：

```ts
export interface WeekDayPlan {
  date: string
  day_name: string
  lunch: Dish[]
  dinner: Dish[]
  // 手动加进该餐的菜 ID；重生成时这些菜留在原位（后端 spec §5.2）。
  manual_lunch_ids?: number[]
  manual_dinner_ids?: number[]
}
```

在 `WeekPlan` 接口之后追加：

```ts
export interface PlannedDishEntry {
  dish_id: number
  dish_name: string
  meal_type: string
}
```

- [ ] **Step 2: API 客户端**

`frontend/src/api/index.ts` 的 `weekPlanApi` 对象末尾（`regenerateDay` 行后）追加：

```ts
  history: (month: string) => api<Record<string, PlannedDishEntry[]>>("GET", `/week-plan/history?month=${month}`),
```

并把 `PlannedDishEntry` 加进该文件顶部的类型 import。

- [ ] **Step 3: WeekPlan 页维护 manual ids**

`frontend/src/pages/WeekPlan.tsx` 的 `addDish` / `removeDish`（:852-872）替换为：

```ts
  function addDish(date: string, meal: MealType, dish: Dish) {
    const manualKey = meal === "lunch" ? "manual_lunch_ids" : "manual_dinner_ids"
    setDraftPlan((prev) => ({
      warnings: prev.warnings,
      days: prev.days.map((day) => {
        if (day.date !== date) return day
        const manualIds = day[manualKey] || []
        return {
          ...day,
          [meal]: uniqueById([...day[meal], dish]),
          [manualKey]: manualIds.includes(dish.id) ? manualIds : [...manualIds, dish.id],
        }
      }),
    }))
    setDirtyPlan(true)
  }

  function removeDish(date: string, meal: MealType, dishId: number) {
    const manualKey = meal === "lunch" ? "manual_lunch_ids" : "manual_dinner_ids"
    setDraftPlan((prev) => ({
      warnings: prev.warnings,
      days: prev.days.map((day) => {
        if (day.date !== date) return day
        return {
          ...day,
          [meal]: day[meal].filter((dish) => dish.id !== dishId),
          [manualKey]: (day[manualKey] || []).filter((id) => id !== dishId),
        }
      }),
    }))
    setDirtyPlan(true)
  }
```

`normalizePlan`（:107-116）对 day 用 spread，新字段自动透传，**无须改**。

- [ ] **Step 4: History 页显示当天计划**

`frontend/src/pages/History.tsx`：

1. import 区补：`weekPlanApi` 加入现有 `@/api` import；`PlannedDishEntry` 加入现有 `@/types` import。
2. 在月度查询 `dayRatings`（:171-174）之后追加月度计划查询（`calYear`/`calMonth` 已有）：

```ts
  const planMonthKey = `${calYear}-${String(calMonth + 1).padStart(2, "0")}`
  const { data: plannedByDate = {} } = useQuery({
    queryKey: ["week-plan-history", planMonthKey],
    queryFn: () => weekPlanApi.history(planMonthKey),
  })
```

3. 在选中日详情区域（用 `selectedDateKey` 渲染打卡记录列表的位置——搜索 `selectedDateKey` 的渲染分支），打卡记录列表**之前**插入"当天排了什么"卡片；无数据不渲染：

```tsx
  {selectedDateKey && (plannedByDate[selectedDateKey]?.length ?? 0) > 0 && (
    <div className="rounded-2xl bg-card p-4">
      <div className="mb-2 text-sm font-medium text-text2">当天排的菜单</div>
      <div className="flex flex-wrap gap-1.5">
        {plannedByDate[selectedDateKey].map((entry, i) => (
          <span
            key={`${entry.dish_id}-${i}`}
            className="rounded-full bg-bg px-2.5 py-1 text-xs text-text2"
          >
            {entry.meal_type === "lunch" ? "午" : "晚"} · {entry.dish_name}
          </span>
        ))}
      </div>
    </div>
  )}
```

样式按 History 页现有卡片惯例微调（实施者对齐周边代码的 className 风格，如 `bg-card`/`text-text2` 的实际用法）；要求：仅展示、午/晚徽标 + 菜名、与打卡记录形成"排了 vs 吃了"对照。

- [ ] **Step 5: 构建验证 + 提交**

Run: `cd frontend && npm run build`
Expected: 类型检查与构建通过。

```bash
git add frontend/src/types/index.ts frontend/src/api/index.ts frontend/src/pages/WeekPlan.tsx frontend/src/pages/History.tsx
git commit -m "feat: manual-dish marking and planned-history view in frontend"
```

### Task 8: 收尾——spec 修正、CHANGELOG、全量验证与重启冒烟

**Files:**
- Modify: `docs/superpowers/specs/2026-06-12-week-plan-persistence-design.md`（§7.2 与 §9.8 购物清单描述修正）
- Modify: `CHANGELOG.md`（v0.8.0 条目，插在文件最上方现有版本之前）

- [ ] **Step 1: 修正 spec 的购物清单描述**

实施中核实：购物清单读 `ShoppingCheck` 行（由打卡 `addShoppingItems` 从菜的食材生成，`internal/handlers/record.go:21`），与周计划存储**无耦合**。spec 两处与事实不符，修正：

§7.2 中 `- 购物清单：继续从当前周计划取数（读取入口同 §5.1）。` 替换为：

```markdown
- 购物清单：数据源是打卡时生成的 ShoppingCheck 行（与周计划存储无耦合，实施中核实），零改动。
```

§9 测试清单第 8 条 `**回归**：购物清单仍出数；冷却窗口跨周界时正确避开上周末的菜；` 替换为：

```markdown
8. **回归**：冷却窗口跨周界时正确避开上周末的菜（读侧零改动，由现有 recentDishIDMap 测试覆盖）；
```

- [ ] **Step 2: CHANGELOG**

`CHANGELOG.md` 文件顶部（现有最新版本节之前）插入：

```markdown
## v0.8.0 - 周菜单变成"有记忆的"

### 新增
- **每周菜单永久存档**：周菜单不再是一次性缓存——每周一份独立保存，下周自动开新篇，上周的菜单原样留存。
- **历史页能看"当天排了什么"**：历史日历选中某天，会显示那天当时排的菜单，和实际打卡对照，排了没吃一眼看出。
- **手动加的菜不再被冲掉**：自己加进菜单的菜，点"重新生成"时会留在原位，系统只重排它自己排的部分。
- **"本周想吃 / 每天主题"只管这一周**：下周自动清空，不再永久残留到每个周三都想吃牛肉。

### 修复
- **重新生成不再改写过去**：周中点"重新生成整周"只重排今天及以后；昨天以前排过什么保持原样，推荐历史不再被悄悄顶掉。过去的天也不能再被单天重生成。
```

- [ ] **Step 3: 全量验证**

```bash
cd backend && go test ./... && go vet ./...
cd ../frontend && npm run build
```

Expected: 全绿。

- [ ] **Step 4: 重启后端并冒烟**

按项目惯例（go run 子进程名是 exe/server，按端口杀）：

```bash
lsof -ti:8080 | xargs kill 2>/dev/null; sleep 1
cd backend && nohup go run ./cmd/server > /tmp/ninimenu-server.log 2>&1 &
sleep 5 && tail -5 /tmp/ninimenu-server.log
```

Expected: 日志出现 `NiniMenu 启动成功`，且**无** `周菜单存储迁移失败`。

带 token 冒烟（token 在 `backend/.env` 的 APP_TOKEN；若变量名不同以 `internal/config/config.go` 为准）：

```bash
TOKEN=$(grep -E '^APP_TOKEN=' backend/.env | cut -d= -f2)
curl -s -H "X-App-Token: $TOKEN" 'http://localhost:8080/api/week-plan' | head -c 300; echo
curl -s -H "X-App-Token: $TOKEN" "http://localhost:8080/api/week-plan/history?month=$(date +%Y-%m)" | head -c 300; echo
curl -s -H "X-App-Token: $TOKEN" 'http://localhost:8080/api/week-plan/history?month=bad' | head -c 200; echo
```

Expected: 第 1 条返回本周 7 天计划；第 2 条返回本月已计划日期映射（今天及以后的本周日期应在内）；第 3 条返回 400 与"月份格式无效"。

数据库冒烟：

```bash
sqlite3 backend/data/ninimenu.db "SELECT user_id, week_start, length(plan_json), prefs_json FROM week_plans;"
sqlite3 backend/data/ninimenu.db "SELECT COUNT(*) FROM settings WHERE \`key\` = 'week_plan_cache';"
```

Expected: 第 1 条至少一行（user_id=1、week_start=本周一、plan_json 非空）；第 2 条为 0（旧 key 已被迁移删除）。

- [ ] **Step 5: 提交**

```bash
git add docs/superpowers/specs/2026-06-12-week-plan-persistence-design.md CHANGELOG.md
git commit -m "docs: fix shopping-list coupling note and add v0.8.0 changelog"
```

---

## 最终集成检查清单（执行完 8 个任务后逐项核对）

1. `go test ./...` 与 `npm run build` 全绿；`go vet ./...` 无告警。
2. 新表生效：`week_plans` 有本周行（user_id=1），`week_plan_cache` Setting 已消失。
3. 冻结语义：mock 时间的测试覆盖了"整周重生成不动过去天 + 过去推荐行不可变 + 保存端点篡改过去无效 + 单天重排过去被拒"四个面。
4. 手动菜：加菜→保存→整周重生成后菜还在原餐，`manual_*_ids` 随行保存且前端维护。
5. 偏好：PUT 后 Setting 无 `week_want`/`days` 残留、本周行 PrefsJSON 有；mock 翻篇后 Get 返回空一次性偏好、常驻配额完好。
6. 迁移幂等：`MigrateWeekPlanStorage` 重复执行无副作用（测试覆盖）。
7. 回看：`/week-plan/history` 按月分组、month 校验、异 source 不混入；History 页选中日出现"当天排的菜单"卡片。
8. 兼容回归：`recentDishIDMap`（冷却）与购物清单代码零改动；前端 GET/PUT /week-plan 旧形状字段不变（新字段全可选）。
9. spec 偏差记录：唯一一处主动偏差是 Task 8 对 spec §7.2/§9.8 购物清单描述的事实性修正（实施中发现 spec 笔误），无行为偏差。

## 计划自检记录（writing-plans Self-Review）

- **Spec 覆盖**：§3→Task 1/3，§4→Task 4，§5.1/5.4/5.5/5.6→Task 2，§5.2/5.3→Task 3，§6→Task 6/7，§7.1→Task 5，§7.3→Task 7，§8 各项散落于对应任务实现，§9 测试清单逐条映射（见最终检查清单 3-7）。无缺口。
- **占位符扫描**：无 TBD/TODO/“类似 Task N”；所有代码步骤含完整代码；唯一的实施者自由度是 Task 7 Step 4 History 卡片的 className 微调（功能要求已写死）。
- **类型一致性**：`withPlanNow`（Task 1 定义，2/3/4/5 使用）、`todayKey`/`CurrentUserID`/`loadWeekPlanRecord`/`upsertWeekPlanRecordPlan`/`upsertWeekPlanRecordPrefs`（Task 1 定义，2/4/5 使用）、`weekPlanOneOffPrefs`（Task 4 定义，5 使用，依赖已标注）、`manualDishesForMeal`/`reduceQuotaForKept`/`containsDishID`（Task 3 定义并使用）、`PlannedDishEntry`（Task 6 后端 / Task 7 前端各自定义，JSON 字段名一致）——签名与用法逐一核对无冲突。

