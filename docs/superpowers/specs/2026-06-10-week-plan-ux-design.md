# Week Plan Customization & Friendly Rules — Design

Date: 2026-06-10
Status: approved by user (interactive brainstorm, mockups in `.superpowers/brainstorm/`)

## 1. Problem

Two UX gaps reported by the user:

1. **Week plan page** has no way to customize the *shape* of a week — e.g.
   "spicy Monday, light Tuesday, a good soup on Saturday, more chicken this
   week". Today only a weekday/weekend profile + meat/veg/soup counts exist.
2. **Rules settings page** (admin Settings → 高级推荐规则) exposes raw expr
   expressions, severity/priority/scope/relaxable fields — unusable for
   non-technical users. The default rule set itself was also written ad-hoc
   (4 of 10 rules duplicate internal profile scoring; useful household rules
   are missing).

## 2. Confirmed product decisions

- Per-day customization granularity: **one taste theme per day** (lunch and
  dinner follow it). Counts stay on the weekday/weekend level.
- Ingredient cravings: **week-level "want more X" + per-day override**.
- Rules page must support: **blocked-ingredients management (忌口/过敏)**,
  **human-readable preset rules with toggles & adjustable numbers**, and
  **sentence-builder for custom rules**. Expression editing survives as a
  collapsed expert mode.
- Architecture: option A — preferences carry transient lifestyle choices,
  blocked ingredients are a hard pool filter shared by *all* recommendation
  paths, the rule table keeps only durable household habits.
- Week-plan page layout: option C — persistent 7-day theme strip above the
  menu; tapping a day opens a bottom sheet (theme + cravings + regenerate
  this day only).
- Preset combos are **split into weekday packs × weekend packs** (4 × 4),
  applied as a starting point that users can still edit per day.

## 3. Data model

### 3.1 WeekPlanPreferences (extended, stored in `Setting` key `week_plan_preferences`)

```go
type WeekPlanPreferences struct {
    Weekday  WeekPlanPeriodPreferences `json:"weekday"`
    Weekend  WeekPlanPeriodPreferences `json:"weekend"`
    WeekWant []string                  `json:"week_want,omitempty"` // protein keys
    Days     map[string]DayOverride    `json:"days,omitempty"`      // "mon".."sun"
}

type DayOverride struct {
    Profile string   `json:"profile,omitempty"` // "" = follow period default
    Want    []string `json:"want,omitempty"`    // protein keys, e.g. ["poultry"]
}
```

- Keys are weekday names (`mon`..`sun`), **not dates**: the rhythm repeats
  weekly and never expires. Changing one week's craving means editing the
  always-visible setting — acceptable, avoids one-shot/date cleanup logic.
- `want` values use the existing protein-source keys
  (`pork|beef|lamb|poultry|seafood|egg|soy`), matched against
  `dish.ProteinSources` — far more reliable than free-text ingredient match.
  Chinese labels: 猪肉/牛肉/羊肉/鸡鸭/鱼虾/蛋类/豆制品.
- Unknown/missing fields default to today's behavior (backward compatible
  with stored JSON lacking `days`/`week_want`).

### 3.2 Blocked ingredients (new `Setting` key `blocked_ingredients`)

JSON string array of single ingredient words (e.g. `["香菜","虾","蟹"]`).
Stored independently from rules; edited on the rules settings page via the
existing manage-settings key/value channel. Saving does **not** silently
invalidate the cached week plan (that would discard manual edits); the UI
shows a toast suggesting 重新生成 so the filter takes effect. New
generations and tomorrow-picks apply the filter immediately.

### 3.3 MenuRule template field (new column `template`, default "")

JSON describing the sentence-builder parameters; empty = expert rule.

```json
{"type":"limit_count|discourage_repeat|prefer|avoid",
 "scope":"meal|day|week",
 "category":"egg|cold|staple|soup|spicy|heavy_spicy|deep_fry|slow|protein:<key>|ingredient:<word>",
 "n":1, "points":15, "strength":"must|prefer"}
```

The **expression remains the single execution truth**. A Go renderer
(`menu_rule_templates.go`) converts template → expression (+ rule_kind,
severity, relaxable, scope, message). Editing a parameter re-renders the
expression and runs the existing compile/validate path. Rules without a
template render in expert mode only.

## 4. New default rule set (keep 5 / drop 5 / add 4)

Keep (templated, numbers adjustable):
1. 同餐蛋类最多 1 道 — hard limit_count(meal, egg, 1)
2. 同餐凉菜最多 1 道 — hard limit_count(meal, cold, 1)
3. 同餐主食最多 1 道 — hard limit_count(meal, staple, 1)
4. 同餐主蛋白重复扣分 (soy exempt) — soft, −14
5. 同日重油重辣不连续 — soft, −18

Drop (one-time migration): `quick/light/spicy/favorite_profile_bonus`
(duplicate the planner's internal profile scoring — implementation detail
leaked into user-visible rules) and `hot_cold_balance_bonus` (+6 is noise).

Add (all soft/prefer by default):
6. 同餐辣菜别扎堆 — meal already has spice_level≥1 → spicy candidate −12.
   Deliberately weaker than the spicy-theme bonus so a 想吃辣 day can still
   stack spicy dishes.
7. 同餐费时菜别扎堆 — meal already has cook_time>45 → slow candidate −15.
   Models the cook's real time budget.
8. 同种主蛋白整周最多 4 道 — candidate protein already ≥4 times this week
   → −20. Weekly variety backstop.
9. 整周油炸最多 2 道 — week already has ≥2 deep_fry → fried candidate −20.

Engine support: add `countOverlapWeek` / `countOverlapDay` env helpers
(meal-level overlap already exists). Numeric conditions (cook_time>45,
spice_level≥1 over meal/day/week arrays) use expr-lang's native
`filter`/`len` if workable; otherwise add a small `countMealWhere`-style
helper. Decide during implementation.

**Migration** (extend `EnsureDefaultMenuRules`): delete the 5 dropped rules
only when their expression still equals the factory value (user-modified
copies survive); create the 4 new rules if their codes are absent; attach
`template` to the 5 kept rules when their expression equals factory value.

## 5. Generator behavior

- Pool order: `enabled` → **blocked-ingredient filter** → meal_type split.
  `FilterBlockedDishes` matches dish name + ingredient/seasoning names by
  substring; wire into `GenerateWeekPlan`, `PickDishes`,
  `PickTomorrowDishes`, and any other recommendation entry points found via
  grep (wheel/blind-box paths).
- Per-day profile: after picking `periodPrefs`, apply
  `prefs.Days[weekdayKey]` override (profile + want).
- New profile **`soup` (靓汤)**: `normalizePlanProfile` accepts it;
  `matchesTomorrowProfile`/`tomorrowDishScore` gain a soup branch (soup role
  or simmer/braise methods score up); if a soup-day's quota has
  `SoupCount==0`, temporarily add 1 soup slot for that day.
- Scoring weights: day want **+25**, week want **+12**, unchanged profile
  match +40 and favorite +12. Rationale: a day craving beats favorites but
  not the day's theme; week-level cravings sit at favorite level.

## 6. Single-day regeneration

`POST /api/week-plan/regenerate-day {date}` — load cached plan, rebuild only
that day (its quota + day override), seeding dedup context
(`globalUsed`/`weekPicked`) from the other six days, then persist via
`SaveWeekPlan` (recommendation records refresh as today). Returns the full
plan. Extract the per-day section of `GenerateWeekPlan` into a shared
helper to avoid duplication.

## 7. Frontend

### 7.1 Week plan page (`WeekPlan.tsx`)

- **Theme strip**: 7 cells (一..日) above the menu list; each shows the
  day's theme emoji (· when following default). Tap → bottom sheet:
  - theme picker (6 options: 均衡/快手/清淡/想吃辣/收藏/靓汤)
  - 今天想吃 chips (protein labels, multi-select)
  - actions: 仅这天重生成 / 保存
- **组合 (combos) button** beside the strip → sheet with two steps:
  weekday pack + weekend pack → preview of the 7-day spread → apply
  (confirmation toast warns it overwrites current day overrides; suggests
  regenerate).
- **本周想多吃** chips added to the existing preferences drawer.
- Existing drawer (counts + period profile) stays; period profile is now
  described as the **default taste** used by days without an override.
- Meal-count quick presets in the drawer: 二人简餐 / 三口之家 / 人多聚餐
  (fills the 12 steppers in one tap).

Combos are a frontend constant (`weekPlanCombos.ts`), writing only
`days` (+`want`). **v1 does not touch rule parameters** (e.g. 亲子时光
tightening the spicy rule) — the family pack approximates "less spicy" via
light/soup themes. Rule-parameter linkage is v2.

Weekday packs: 上班族快手 (mon–fri quick) · 家常有节奏 (mon light, wed
spicy, fri favorite) · 清爽减脂 (all light; wed want seafood, fri want
poultry) · 解馋重口 (tue+fri spicy).
Weekend packs: 亲子时光 (sat soup, sun light) · 犒劳自己 (sat favorite,
sun spicy) · 靓汤滋补 (sat+sun soup) · 简单休息 (sat+sun quick).

### 7.2 Rules settings page (admin Settings → rules section)

Three sections replacing the current editor (which becomes expert mode):

1. **🚫 家庭忌口** — chip list + add input (splits multi-word input);
   explainer: applies to every recommendation feature.
2. **🍽 搭配习惯** — preset rules as cards: toggle + inline number stepper
   (renders from `template.n`), one-line plain-Chinese explanation. No
   code/severity/priority/scope/relaxable visible.
3. **✏️ 我的规则** — templated custom rules listed like presets; "新建规则"
   opens the sentence builder (在 [范围] 里，[类别] [动作] [N] 道，[力度]).
   Saves via template render → existing validate/save APIs.
4. **⚙️ 专家模式** (collapsed) — current expression editor, unchanged
   capabilities, hosts non-templated rules.

## 8. Testing

- Day override changes that day's candidate ranking (e.g. wed=spicy).
- Day want +25 outranks favorite; week want +12 applies all week.
- Blocked ingredient excluded from week plan AND tomorrow picks.
- Soup day with SoupCount==0 gains exactly one soup slot.
- regenerate-day: other six days byte-identical, target day re-picked, no
  duplicate dishes across the week, recommendations table updated.
- Template renderer: each template type → expected expression; round-trip
  via existing compile validation; n/points edits re-render.
- Default-rule migration: drops only unmodified factory rules; adds 4 new;
  user-modified rules untouched; idempotent on second boot.
- Old preferences JSON without days/week_want loads with current behavior.

## 9. Milestones

1. Backend: blocked-ingredients filter across all pick paths.
2. Backend: days/week_want in preferences + generator override + soup
   profile.
3. Backend: regenerate-day API.
4. Backend: rule template field + renderer + new default set + migration +
   countOverlap helpers.
5. Frontend: theme strip + day sheet + combos + drawer additions.
6. Frontend: rules page three-section rebuild with expert-mode fold.
7. Regression pass (backend tests, frontend lint/build), docs update.

## 10. Out of scope (v2 candidates)

- Combo packs adjusting rule parameters (亲子时光 → spicy limit 0).
- Date-specific one-shot overrides ("only this Wednesday").
- Week-level "eat less X" (avoid list) — v1 ships want-only.
- Multi-member preference merging (per-person spice tolerance).
