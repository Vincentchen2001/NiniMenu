# 周菜单持久化重构设计（子项目 1：周菜单一等公民化）

日期：2026-06-12
状态：已批准（方案 A：按周快照表）
前置讨论：伪随机现象与跨周重复诊断（2026-06-11 会话）
后续子项目：子项目 2「新鲜度衰减」消费本次建立的可靠历史；子项目 3「多用户化」消费本次预留的 user_id 缝。

## 1. 背景与问题

当前周菜单在数据层不是一等公民，有四个硬伤：

1. **整周计划只是 Setting 表里的一个 JSON 字符串**（key=`week_plan_cache`）。单槽位，只装"当前周"，本质是缓存不是数据。
2. **翻篇即丢**。下周一打开页面，旧 JSON 因日期检查失效（`GetCachedWeekPlan` 检查 `plan.Days[0].Date >= weekKey`），新计划一保存直接覆盖。上周菜单的结构化形态永久消失。
3. **周中重新生成会改写历史**。`GenerateWeekPlan` 永远从本周一生成整周 7 天（含已过去的天）；`SaveWeekPlan` 的 `replaceWeekPlanRecommendations` 把本周日期范围内 source=week_plan 的旧推荐记录整批删掉重写。周四点"重新生成"，周一~三"当时排了什么"被顶掉；用户手动调过的菜也被冲掉。
4. **计划态与事实态没有衔接**。唯一跨周留存的是扁平 `DishRecommendation` 行，但受第 3 条改写影响；系统分不清"排了且吃了 / 排了没吃 / 被换掉了"。

后果：跨周记忆只有 N 天冷却（默认 3 天）可依赖，且地基会被改写。子项目 2 的新鲜度衰减算法必须先有可靠历史。

## 2. 产品决策（已与用户敲定）

| 决策点 | 结论 |
|---|---|
| 历史用途 | **轻量回看**：History 页（打卡日历）顺带显示"这天当时排了什么"，与实际吃的对照。不做整周历史浏览 UI。 |
| 重排边界 | **今天及以后可重排**；昨天及更早冻结。按天切，不按餐次细分。 |
| 手动编辑幸存 | **手动菜固定**：每道菜记录来源；重生成只重排系统排的，手动加的原位保留、配额扣减。 |
| 一次性偏好 | 「每天主题 / 本周想吃」**只管这一周**，下周自动清空；常驻配额/档位仍是全局设置。 |
| 多用户 | **留缝不开门**：新表带 `user_id` 列与复合唯一键，单用户阶段恒为常量 1；认证体系与存量表迁移立为独立子项目。 |
| 存储模型 | **方案 A：按周快照表**。`week_plans` 一用户一周一行（计划 JSON + 本周偏好 JSON）；`DishRecommendation` 保持冷却/回看索引职责。 |
| 历史保留时长 | 永久（每周约一行，无清理逻辑，YAGNI）。 |

落选方案：B 完整规范化（week_plan_entries 每菜一行，DishRecommendation 退役）——最干净但迁移面最大，对单用户 SQLite 应用解决的是不存在的问题；C 升级推荐表为唯一真相源——计划级元数据（warnings、当时菜品快照）无处安放。

## 3. 数据模型

### 3.1 新表 `week_plans`

```go
// internal/models/week_plan_record.go
type WeekPlanRecord struct {
    ID        uint      `json:"id" gorm:"primaryKey"`
    UserID    uint      `json:"user_id" gorm:"not null;default:1;uniqueIndex:idx_week_plans_user_week,priority:1"`
    WeekStart string    `json:"week_start" gorm:"not null;uniqueIndex:idx_week_plans_user_week,priority:2"` // 周一 YYYY-MM-DD
    PlanJSON  string    `json:"plan_json"`  // services.WeekPlan 序列化；空串 = 本周尚未生成计划
    PrefsJSON string    `json:"prefs_json"` // 本周一次性偏好（WeekWant + Days）；空串 = 无
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

- `PlanJSON` 允许空字符串：偏好先于计划写入时（用户先设"本周想吃"再打开周计划页），行先建、计划后补。读到空 `PlanJSON` 视为"本周未生成"，走生成路径。
- 单用户常量：`services` 包定义 `const CurrentUserID uint = 1`，所有读写传它；将来多用户从认证上下文注入，新表零迁移。
- 注册进 `database.AutoMigrate`。

### 3.2 `WeekDayPlan` 扩展（手动菜标记）

```go
type WeekDayPlan struct {
    Date            string        `json:"date"`
    DayName         string        `json:"day_name"`
    Lunch           []models.Dish `json:"lunch"`
    Dinner          []models.Dish `json:"dinner"`
    ManualLunchIDs  []uint        `json:"manual_lunch_ids,omitempty"`
    ManualDinnerIDs []uint        `json:"manual_dinner_ids,omitempty"`
}
```

- 午/晚分开两个数组，避免"手动加进午餐、系统排进晚餐"误标。
- 字段可选（omitempty），前端旧数据/旧请求不带它们也合法——零破坏。
- 前端在加菜操作时把菜 ID push 进对应数组、删菜时移除；后端原样存储并在重生成时消费。单用户本地应用，信任前端标记。

### 3.3 `DishRecommendation`（无 schema 变化）

表结构不动。写侧语义变化见 §5.6；读侧（冷却 `recentDishIDMap`、新回看端点）不变或新增。

## 4. 偏好拆分：常驻 vs 本周

`WeekPlanPreferences` 结构体与 API 形状**不变**，存储拆两处：

| 部分 | 字段 | 存储位置 |
|---|---|---|
| 常驻 | `Weekday` / `Weekend`（配额 + 档位） | 原 Setting key `week_plan_preferences` |
| 本周一次性 | `WeekWant` + `Days`（每天主题/想吃） | 当前周 `week_plans.PrefsJSON` |

- `GET /week-plan/preferences`：返回合成视图（Setting 的常驻部分 + 当前周行的一次性部分）。
- `PUT /week-plan/preferences`：拆开写——常驻部分写 Setting，一次性部分 upsert 当前周行的 `PrefsJSON`（行不存在则创建，`PlanJSON` 留空）。
- 翻篇效果：新周行不存在 → 一次性部分读出来为空 → "想吃/主题"自动清零。
- `PrefsJSON` 内容结构：`{"week_want":[...],"days":{"mon":{...},...}}`，复用现有 `DayOverride` 类型。

## 5. 行为语义

约定："今天" = 服务器本地时区 `time.Now().Format("2006-01-02")`；日期比较一律字符串比较（项目现状惯例）。

### 5.1 读当前周（GetWeekPlan）

查 `(user_id=CurrentUserID, week_start=本周一)` 行：

- 命中且 `PlanJSON` 非空 → 反序列化返回；
- 未命中或 `PlanJSON` 为空 → 生成整周 → 写入该行（upsert，保留已有 `PrefsJSON`）→ 返回。

内存缓存 `cachedPlan`/`cachedWeekKey` 机制保留，只是落库目标从 Setting 换成 `week_plans` 行。现有两条写路径（`saveWeekPlanCache` 与 `SaveWeekPlan`）统一为一条。

### 5.2 整周重生成（RegenerateWeekPlan）

1. 读存量行；昨天及更早的天**原样复制**进新计划（含其 manual ids）。
2. 今天及以后的天重排，但每天 manual ids 中仍存在于对应槽位的菜**保留原位**，配额扣减后只补系统位（例：午餐荤菜配额 2、手动占 1 → 系统补 1）。
3. 冻结天与手动菜全部计入生成上下文：`globalUsed`（周内去重）、`weekPicked`（周级规则如蛋白多样性/费时汤上限）、`prevSoups`（汤主料相邻去重）从冻结天和手动菜接续，保证新排的天不和已固定的内容撞车。
4. 写回同一行（UPDATE PlanJSON）。

### 5.3 单天重生成（RegenerateWeekPlanDay）

目标天 < 今天 → 拒绝，400，消息"过去的天不能重新生成"。其余沿用现有逻辑（已只动一天、已传相邻汤上下文），该天的 manual 菜同样按 §5.2 第 2 条保留。

### 5.4 手动编辑保存（PUT /week-plan）

前端发整份计划（含 manual ids）。服务端**对过去的天以存量行覆盖请求值**——历史只读对所有写路径一致；修正"那天实际吃了什么"走打卡（MealRecord），不改计划。今天及以后的天按请求保存。

### 5.5 翻篇

下周一首次访问，`(user, 新周一)` 行不存在 → 自动生成新周行。上周行原地留存，永不改写。

### 5.6 推荐记录写侧（replaceWeekPlanRecommendations）

改为只删、只写**今天及以后**的行：删除范围 = 计划日期 ∩ [今天, +∞)，插入同范围。过去的 `DishRecommendation` 行从此不可变——冷却（3 天窗口正好横跨周界）与回看的数据地基变可靠。读侧零改动。

## 6. 轻量回看

- 新端点：`GET /week-plan/history?month=YYYY-MM`（month 格式校验），查询 `DishRecommendation` WHERE `source='week_plan' AND planned_date BETWEEN 'YYYY-MM-01' AND 'YYYY-MM-31'`，返回：

```json
{ "2026-06-10": [ {"dish_id": 12, "dish_name": "鲫鱼豆腐汤", "meal_type": "lunch"}, ... ], ... }
```

- History 页（打卡日历）选中某天时，在打卡记录旁显示"当天计划了什么"；菜被删后用 `dish_name` 快照兜底显示。排了没吃一眼可见。
- 仅展示，不提供编辑。

## 7. 迁移与兼容

### 7.1 启动迁移（一次性）

1. AutoMigrate 建 `week_plans` 表。
2. 读旧 Setting `week_plan_cache`：
   - 存在且 `Days[0].Date == 本周一` → 搬入 `week_plans` 行（upsert）；
   - 存在但已过期 → 不搬；
   - 处理完删除该 Setting key。
3. 历史从升级日开始积累，旧周数据无从恢复（已被原设计丢弃），明确不做。
4. 一次性偏好迁移：现有 Setting `week_plan_preferences` 里的 `WeekWant`/`Days` 视为属于当前周，搬入当前周行 `PrefsJSON`，并从 Setting 值中剔除（常驻部分留下）。

### 7.2 不变的路径

- 购物清单：数据源是打卡时生成的 ShoppingCheck 行（与周计划存储无耦合，实施中核实），零改动。
- 冷却 `recentDishIDMap`：继续读 `DishRecommendation`，零改动。
- 前端 API 形状：GET/PUT /week-plan、preferences 端点的 JSON 结构向后兼容（新字段全部可选）。

### 7.3 前端改动（收敛两处）

1. WeekPlan 页：加菜/删菜时维护 `manual_lunch_ids`/`manual_dinner_ids`；类型定义补两个可选字段。
2. History 页：调用新回看端点，按天展示计划与打卡对照。

## 8. 错误处理

- `PlanJSON` 解析失败 → 视为"本周未生成"，重新生成覆盖，log 警告；不崩溃。
- 过去天的单天重排请求 → 400 友好消息（§5.3）。
- 翻篇竞态（两请求同时首访新周）：upsert（ON CONFLICT 更新）+ 现有 `planMu` 互斥已序列化同进程请求。
- 回看端点 month 参数非法 → 400。

## 9. 测试清单

1. **迁移**：旧 cache 是本周 → 搬入新表且旧 key 删除；旧 cache 过期 → 只删不搬；偏好一次性部分搬入当前周行、常驻部分留 Setting。
2. **翻篇留存**：模拟跨周（注入周键），新周生成新行，上周行内容不变。
3. **冻结边界**：周中（mock 今天=周四）整周重生成 → 周一~三的天与对应 `DishRecommendation` 行逐字节不变；周四~日重排。
4. **过去天保护**：单天重排过去天 → 400；保存端点篡改过去天 → 服务端以存量覆盖。
5. **手动菜幸存**：手动加菜 → 整周重生成 → 该菜原位保留、该槽系统补位数=配额−1；手动菜计入周内去重与周级规则计数。
6. **偏好按周**：设置 WeekWant/Days → 翻篇 → 新周读出为空；常驻配额不受影响；偏好先于计划写入（行先建、PlanJSON 空）→ 后续生成正常落位。
7. **回看端点**：按月返回计划行；删菜后 dish_name 兜底；month 非法 400。
8. **回归**：冷却窗口跨周界时正确避开上周末的菜（读侧零改动，由现有 recentDishIDMap 测试覆盖）；`go test ./...` 与 `npm run build` 全绿。

## 10. 范围外

- 多用户认证、存量表（MealRecord/Setting/Dish/MenuRule/购物状态）加 user_id 迁移、菜品库归属模型——独立子项目 3。
- 新鲜度衰减算法（重新生成求变、跨周软降权、防多周收敛循环）——子项目 2，消费本次历史。
- 整周历史浏览 UI（按周翻页回看）。
- Dish 完整快照（历史里 join 不到的菜只兜底显示名字，不还原完整详情）。
- 按餐次细分的冻结边界（"下午 3 点重生成时今天午餐冻结"）。
