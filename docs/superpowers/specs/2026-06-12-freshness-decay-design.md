# 新鲜度衰减降分设计（子项目 2）

> 日期：2026-06-12。前置：v0.8.0 周菜单持久化重构（spec `2026-06-12-week-plan-persistence-design.md`）。
> 本文是实施依据；实现计划另见 plans 目录。

## 0. 背景与问题

v0.8.0 之后，周菜单按周快照永久保留，推荐历史行（`dish_recommendations`）过去天不可变，打卡记录（`meal_records`）一直都在——回看地基已就位。但推荐算法的跨周记忆仍只有一个 **3 天硬冷却**（`repeat_days`，二值）：

- 冷却内：整道菜从候选池排除（放宽阶梯第 2 级才解禁）；
- 冷却外：**悬崖式清零**——"4 天前刚吃过"和"两个月没吃"在打分里完全平等。

打分主体（位置对口 +60、主题 +40、收藏 +12、三条降权）对同一菜品库是基本确定的，随机项只有 0~1 的平手微调。结果是高分菜每周都赢，**多周后菜单收敛成固定循环**（下周 ≈ 本周去掉周末尾巴后的重排）。

本设计引入**时间衰减的新鲜度降分**（前期讨论的方案 e）：出现过的菜在其后 14 天内背一个随时间线性消退的降分，让"最近上过桌"成为连续的负信号而不是二值开关。

## 1. 目标与非目标

### 1.1 目标

1. 跨周轮换：上周吃过的菜本周在势均力敌时让位给更久没吃的菜，约两周完成一轮轮换。
2. 力度可控：作为规则页第 10 条预设打分规则交付（可调分值、可关闭），与现有 9 条同一控制面。
3. 菜荒不挡路：随放宽阶梯第 5 级（可降级规则）自动失效。
4. 入口一致：明日推荐按出厂默认值镜像同款降分（先例：三条 v3 降权的镜像机制）。
5. 既有行为零回归：无历史数据的菜零惩罚；硬冷却、全周唯一、购物清单、回看端点均不动。

### 1.2 非目标（明确不做）

- 衰减**窗口天数不开放配置**（出厂常量 14 天；力度走规则页 points 调）。
- **不算频次**——只看最近一次出现，不看 30 天内吃了几次（recency 已足够打破循环，将来不满意再叠加）。
- 「今晚吃什么」转盘照旧纯随机，不接衰减。
- 不引入新表、不动 `week_plans` / `dish_recommendations` 的写路径，纯读侧消费。
- 前端除规则页一行类别文案外无 UI 改动。

## 2. 数据源与口径

### 2.1 lastSeen 映射

每次生成（周菜单一次、明日推荐一次）查询"每道菜最后一次出现的日期" `map[dishID]lastDate`，两个来源取**较近者**：

| 来源 | 查询 | 语义 |
|---|---|---|
| 打卡吃过 | `meal_records`：按 dish_id 取 `MAX(meal_date)` | 真实吃过，含菜单外随手做的 |
| 排进过菜单 | `dish_recommendations`：按 dish_id 取 `MAX(planned_date)`，**仅 `planned_date <= today`** | 计划口径；未来尾巴不算（沿用 `recentDishIDMap` 的 cap-at-today 语义与注释理由） |

两条查询都加 `>= today − 13天` 的下界：窗口外惩罚必然归零，没必要读。这同时保证 map 恒小（最多 14 天 × 每天几道），`dish_recommendations` 多年增长不影响生成性能。

> 下界证明：可重排的参照日 refDay ≥ today（过去天被冻结或不参与重挑）。`d = refDay − lastDate < 14` 要求 `lastDate > refDay − 14 ≥ today − 14`，即 `lastDate ≥ today − 13`。

实现放在新文件 `backend/internal/services/week_plan_freshness.go`：

```go
const staleRepeatWindowDays = 14
const staleRepeatDefaultPoints = 30

// lastSeenDishDates returns each dish's most recent appearance
// (meal_records ∪ dish_recommendations capped at today) within the
// decay window. now is injectable: week-plan callers pass planNow(),
// the tomorrow pick passes time.Now().
func lastSeenDishDates(now time.Time) map[uint]string
```

周菜单路径传 `planNow()`（可注入时钟，测试用 `withPlanNow` 钉死）；明日推荐路径传 `time.Now()`（与该文件其余代码一致，预存在缝不在本次扩大）。

### 2.2 边界语义

- **从未出现的菜**：不在 map → `days_since_last = -1` → 零惩罚。新菜的压制由"陌生菜系降分"负责，两机制独立。
- **今天的菜**（今日推荐行或今日打卡）：`d = 0` → 满额惩罚。含义：重新生成（整周或单天）会主动避开当前计划里今天的菜——"重新生成 = 给我换一批"，符合预期；该行为也覆盖设置变更触发的静默重建，属可接受的一致语义。
- **负 d**（参照日早于 lastSeen，仅在失效后重建的过去天填充时可能出现）：按 -1 处理，零惩罚。过去天填充本就是展示态。

## 3. 衰减函数

### 3.1 公式

```
penalty(d) = -P × (14 − d) / 14    当 0 ≤ d < 14
penalty(d) = 0                      其他（含 d = -1）
```

- `P` = 规则 points，出厂默认 **30**，规则页步进器可调、可整条关闭。
- 窗口 14 天为出厂常量（`staleRepeatWindowDays`），不开放配置。
- **参照日期 = 菜单上那一天**（不是生成那天）：周日的菜距上次吃更远、惩罚更小，与"周末判定按菜单上那天是周几"先例一致。明日推荐参照"明天"。

### 3.2 体感数字（P=30）

| 距上次出现 | 降分 | 处境 |
|---|---|---|
| 0~3 天 | -30 ~ -23.6 | 被 3 天硬冷却盖住，平时无感；菜荒触发第 2 级放宽时仍垫底 |
| 4 天（刚出冷却） | **-21.4** | 用户问答敲定的"中等"档体感 |
| 7 天（上周同期） | -15 | 与费时菜同餐扣分同段位 |
| 10 天 | -8.6 | 接近收藏分一半 |
| ≥14 天 | 0 | 满血回归 |

段位感：惩罚拼不过位置对口（+60）+ 主题（+40）的硬需求，但足以在势均力敌的菜之间分出胜负——与三条 v3 降权同一设计哲学（软降权、不赶尽杀绝）。

### 3.3 为什么从 d=0 起定义（而非出冷却才开始）

冷却内的 d=0~3 平时被硬冷却整段盖住，公式在此区间是否有值似乎无所谓；但**菜荒触发第 2 级放宽（放弃"最近避重"）时，冷却内的菜会重新参赛**。旧讨论稿"出冷却才计衰减"会让昨天刚吃的菜此时**零惩罚**满血竞争；从 d=0 定义后它仍背着约 -28，放宽了也排队尾。副作用仅是规则卡片显示的 30 是 d=0 的理论最大值，刚出冷却的实际体感约 -21，文档说明即可。

## 4. 规则引擎接入

### 4.1 规则环境新字段

`ruleDishEnv`（menu_rule_service.go）追加：

```go
DaysSinceLast int `expr:"days_since_last"`
```

- `dishRuleEnv()` 一律填 **-1**（零值 0 意为"今天见过"，有歧义，必须显式 -1）；
- 仅 `evaluateScoreRules` / `evaluateConstraintRules` 两处在构造 candidate env 后按当天上下文覆盖真实值；meal/day/week/prevSoups 列表里的菜保持 -1（该字段只对 candidate 有意义）；
- `sampleRuleEnv()` 的样例菜显式 `DaysSinceLast: -1`（编译校验路径走零惩罚分支，输出 0 仍为数字，校验通过）。

### 4.2 种子规则（v4 → v5）

`models.DefaultMenuRules()` 追加第 10 条；`menuRulesSeedVersion` 由 4 升 5。既有迁移机制保证：缺失 code 自动补插、用户改过表达式的行永不触碰、重复执行幂等（`syncDefaultMenuRules` 现状即可，无需改动）。

```go
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
}
```

### 4.3 模板渲染特殊分支

`renderPreference()`（menu_rule_templates.go）在调用 `menuRuleCategoryPredicate` 前拦截：

- `category == "stale_repeat"` 且 `type == "avoid"` → 输出衰减表达式（points 内嵌、窗口用 `staleRepeatWindowDays` 常量），`RuleKind=score / Severity=soft / Relaxable=true / Scope=candidate`；
- `type == "prefer"` 或 limit/no_repeat 配此类别 → 报"规则模板类别无效"（display-only 类别只支持既定形状，与现有 5 个 display-only 类别一致）。

**同步约束**：渲染器以 points=30 产出的表达式必须与种子表达式**逐字节一致**（用户在规则页保存未改动的卡片时不会产生伪 diff，`isFactoryMenuRuleExpression` 判定不受干扰）。用测试锁死。

### 4.4 前端规则页

`frontend/src/lib/menuRuleTemplates.ts` 的 `DISPLAY_ONLY_CATEGORY_LABELS` 加一行 `stale_repeat: "最近吃过的菜"`。卡片句子渲染为"尽量避开最近吃过的菜 · 尽量做到"，points 步进器走既有机制，无其他前端改动。

### 4.5 放宽阶梯交互

作为 Relaxable score 规则，第 5 级"可降级规则"（`enforceSoft=false`）时 `evaluateScoreRules` 返回 0，衰减自动失效——无需任何专门代码。第 1~4 级正常生效，其中第 2 级"最近避重"放宽后衰减是唯一还在压制近期菜的力量（见 §3.3）。

## 5. 周菜单生成管线

### 5.1 上下文装配

- `weekPlanGenContext` 增加 `lastSeen map[uint]string`，`buildWeekPlanGenContext()` 里调 `lastSeenDishDates(planNow())` 一次性装好（与 `recent`、`categoryFavorites` 并列）。
- `weekPlanDayContext` 增加 `daysSince map[uint]int`；`generateDay()` 按当天日期把 `ctx.lastSeen` 换算成天数差填入：

```go
// week_plan_freshness.go
// daysSinceFor converts lastSeen dates into whole-day distances relative
// to refDate; entries that parse badly or land in the future map to -1.
func daysSinceFor(refDate time.Time, lastSeen map[uint]string) map[uint]int
```

- `evaluateScoreRules` / `evaluateConstraintRules` 构造 candidate env 后：

```go
candidateEnv := dishRuleEnv(candidate, dayCtx.categoryFavorites)
if d, ok := dayCtx.daysSince[candidate.ID]; ok {
    candidateEnv.DaysSinceLast = d
}
```

（map 未命中保持 -1；`dayCtx.daysSince == nil` 时同样安全。）

### 5.2 路径覆盖确认

`GenerateWeekPlan`、`RegenerateWeekPlan`、`RegenerateWeekPlanDay`、失效后 `GetCachedWeekPlan` 重建，全部经由 `buildWeekPlanGenContext` + `generateDay`，自动获得衰减；冻结复制的过去天不经过挑选器，不受影响。同周内已排的菜照旧由 `globalUsed`（全周唯一）硬挡——**衰减的主战场是跨周**。

## 6. 明日推荐镜像

`pickPenaltyAdjustment`（pick_service.go）追加第 4 项，按出厂默认值硬编码（先例：三条 v3 降权镜像，规则页改分不影响此路径）：

```go
// 签名追加 daysSinceLast int 参数（调用方查不到时传 -1）
if daysSinceLast >= 0 && daysSinceLast < staleRepeatWindowDays {
    adjust -= int(math.Round(staleRepeatDefaultPoints * float64(staleRepeatWindowDays-daysSinceLast) / float64(staleRepeatWindowDays)))
}
```

- `PickTomorrowDishes` 调 `lastSeenDishDates(time.Now())` + `daysSinceFor(明天)` 一次，经 `sortTomorrowPool` 透传（两函数签名各加一个参数）。
- 与规则引擎的 float 计算相比此处四舍五入到整数（`adjust` 本为 int），偏差 <1 分，镜像本就只承诺默认值近似，可接受。
- `menu_rules.go` 与 `pick_service.go` 两处"three v3 penalty rules mirrored"注释改为四条，保持 keep-in-sync 提示准确。

## 7. 测试策略

| 层 | 用例 | 关键断言 |
|---|---|---|
| freshness 单元 | `lastSeenDishDates`：两表合并取较近、`planned_date` cap-at-today、窗口下界裁剪 | map 内容精确匹配；窗口外/未来行不出现 |
| freshness 单元 | `daysSinceFor`：正常差值、未来日期 → -1、坏日期串 → -1 | 整数天差 |
| 模板渲染 | `stale_repeat` avoid + points 30 渲染 | 与种子 Expression **逐字节相等**（同步约束锁死）；prefer/limit 配此类别报错 |
| 种子迁移 | v4 状态（无第 10 条）跑 `EnsureDefaultMenuRules` | 新规则补插、版本升 5、用户改过的规则原样、重跑幂等 |
| 规则求值 | 编译种子规则后对 days=7 / days=-1 / enforceSoft=false 求值 | ≈ -15（容差 0.01）/ 0 / 0 |
| 周菜单集成 | `withPlanNow` 钉周一；同形菜 A、B，A 五天前打卡 | 该位置选 B；A 十四天前时 A/B 凭随机平手竞争（衰减归零） |
| 周菜单回归 | 现有全部测试 | 无历史数据 → 全员 -1 → 零惩罚，不许有行为漂移；自造推荐行的冻结测试逐一核对（衰减可能改变非冻结天的选菜，必要时补钉历史或放宽断言——计划阶段排查） |
| 明日推荐 | A 五天前打卡、B 无记录，其余同形 | B 排在 A 前 |

## 8. 文档与发布

- `docs/推荐算法说明.md`：§1 候选池表补一句"出冷却仍有 14 天衰减降分"；§3.2 减分表加一行（★新 最近吃过的菜降分，最多 -30 线性消退）；§3.4 段位感补充；§5 放宽阶梯第 2 级备注衰减仍生效；§6 入口差异表镜像行更新为四条；§7 控制面加"调轮换力度"；版本头改 v0.9.0。
- `CHANGELOG.md`：v0.9.0 新增条目（跨周轮换 + 规则页第 10 条 + 明日推荐镜像）。
- 规则页卡片文案由种子 Name/Description 自带，无需另写。

## 9. 验收清单（实施完成后逐项核对）

1. `go test ./...`、`go vet`、`npm run build` 全绿。
2. 升级路径：带 v4 种子的现库启动后规则表出现 `stale_repeat_penalty`，规则页可见可调可关；重启幂等。
3. 钉时钟手测：周一生成本周，上周中吃过的菜显著少于无衰减基线（同库对照）；连续两次"重新生成"今天的菜大概率换批。
4. 规则页把 points 调到 0 或关闭规则 → 行为回到 v0.8.0 基线。
5. 菜荒库（候选 < 配额）生成不开天窗，放宽提示正常。
6. 明日推荐：昨天打卡的菜不出现在前排（冷却挡）；五天前打卡的菜排序明显靠后。
7. 转盘、购物清单、回看端点行为不变。
8. 渲染同步测试在（模板渲染 == 种子表达式），防止规则页保存产生伪 diff。

## 10. 将来扩展挂点（本次不做）

- **频次叠加**：`lastSeenDishDates` 改返回 `(lastDate, count30d)`，公式乘频次因子——查询与公式都集中在 week_plan_freshness.go，单点扩展。
- **窗口可配置**：`staleRepeatWindowDays` 改读 Setting + 模板渲染器带入 N 字段（需放开 N 的 [0,10] 钳制对此类别的例外）。
- **多用户**：lastSeen 查询天然按表查，将来 `meal_records` / `dish_recommendations` 带 user_id 后加 WHERE 即可。
