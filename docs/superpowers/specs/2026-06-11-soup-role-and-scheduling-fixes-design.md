# Soup Role Tightening & Soup Scheduling Controls — Design

Date: 2026-06-11
Status: approved by user (interactive diagnosis + brainstorm)
Follow-up to: 2026-06-11-favorite-aware-soup-scheduling-design.md (v0.7.1)

## 1. Problem

Live-DB diagnosis (632 dishes, all `trait_source=auto`, 84 favorites) of the
user complaint "推荐很难做的汤还是没解决" found four distinct issues:

1. **`dish_role=soup` is massively over-assigned.** 198 dishes carry
   role=soup; 121 of them have no 汤/羹 in name or category (红烧排骨,
   水煮牛肉, 可乐鸡翅, 清蒸鲈鱼, 螺蛳粉, 香辣猪脚, 火锅…). Mechanism:
   `inferDishRole` scans the FULL trait text (name + category + taste +
   remark + tags + ingredient names + **step texts**) for any of
   `汤品/蛋花汤/例汤/羹/煲/汤`, and the soup branch runs before the protein
   branch — so a step like "收浓汤汁" or a remark "蒸好后倒掉原汤" turns a
   meat dish into a soup. Separately, `isSoupDish`
   (week_plan_preferences.go) scans name+category+**tags** for
   `汤/羹/煲/汤品/蛋花汤/例汤`, and soup-slot matching
   (plan_service.go:465,475) is `DishRole=="soup" || isSoupDish(dish)` — a
   two-channel leak. Consequences: soup-slot pool is ~1/3 fake soups, the
   meat pool is missing ~100 real meat dishes, `weekday_slow_soup_penalty`
   (-30) mis-fires on stew-style mains, and the 靓汤 theme rewards fakes.
2. **Weekend long-stew fatigue.** The user favorites 14 soups with
   cook_time > 45 min (猪肚鸡汤 120, 酸萝卜老鸭汤 100, …). After v0.7.1,
   weekdays are suppressed (-30) but weekends have zero limits, so both
   weekend days routinely draw long stews.
3. **Weekday slow soups still appear.** Diagnosed as a side effect of (1):
   the "90-minute soups" seen on weekdays are mostly fake soups (香辣猪脚).
   Decision: fix (1) first, observe, no extra mechanism now.
4. **Adjacent-day soup main-ingredient repetition.** 萝卜丝鲫鱼汤 →
   鲫鱼豆腐汤 on consecutive days is fully legal today: existing rules only
   cover same-meal protein (-14) and 4th-time-in-week protein (-20). No
   adjacent-day control exists.

## 2. Confirmed product decisions

- **Soup = table role, not dish form.** 开水白菜, 汽锅鸡, 跷脚牛肉 (tagged
  "汤品" by the original author) are mains, not soups (user call). A dish is
  a soup iff **category contains 汤** OR **name ends with 汤 or 羹**. Tags,
  remark, steps, ingredients, the 煲 keyword, and mid-name 汤 (酸汤鱼,
  上汤娃娃菜, 汤圆, 汤饭) all stop triggering.
  - Verified against live DB: 198 → **60 soups** (54 in 汤品 category + 6
    suffix-named: 连锅汤, 莲藕花生排骨汤, 冬瓜薏米老鸭汤, 粉汤, 羊杂碎汤,
    豆米腊肉汤). 煲-named dishes are rice/mains. No 蛋羹 dishes exist, so no
    suffix exception is needed.
- **Slow soups (>45 min, real soups) max 2 per week**; from the 3rd one each
  costs -25 (user picked 2).
- **Adjacent-day main-ingredient control applies between soup slots only**
  (user picked the minimal scope): today's soup vs yesterday's soup, -18 on
  overlap, ignoring base aromatics (葱姜蒜 etc.).
- Unchanged this round: `difficulty` semantics (low discrimination noted,
  deferred), the wheel (今晚吃什么) stays penalty-free, weekday -30 stays
  as-is.
- New behavior ships as default menu rules (visible/tunable/toggleable on
  the rules page) wherever the rule engine can express it.

## 3. Part 1 — unified soup detection (bug fix)

Single source of truth in `models`:

```go
// IsSoupName reports whether a dish is a soup in the meal-slot sense
// (a drinkable accompaniment), judged ONLY by name suffix and category.
func IsSoupName(name, category string) bool {
    return strings.Contains(category, "汤") ||
        strings.HasSuffix(name, "汤") ||
        strings.HasSuffix(name, "羹")
}
```

- `inferDishRole` (models/dish_traits.go): the soup branch stops scanning
  the full trait text and uses `IsSoupName(dish.Name, dish.Category)`
  instead. This needs name/category available — change the signature (or
  hoist the soup check into `InferDishTraits` before the text-based
  branches). **All other branches (staple/veg/side keywords, protein check)
  keep using the existing full text unchanged.** Order note: the soup check
  stays first; 汤-suffixed staples like 疙瘩汤 (none in current DB worth
  special-casing) would classify as soup, accepted.
- `isSoupDish` (services/week_plan_preferences.go) becomes a thin wrapper
  over `models.IsSoupName`; `soupDishKeywords` and its `containsAnyKeyword`
  use are deleted. Soup-slot matching keeps the existing
  `DishRole=="soup" || isSoupDish(dish)` structure (manual role overrides
  still win; both channels now agree).
- Re-labeling: `backfillDishTraits` (database.go) already re-infers every
  `trait_source=auto` row on startup — all 632 rows are auto — so a backend
  restart re-labels the whole library. No migration code.
- Expected outcomes: 红烧排骨/水煮牛肉/可乐鸡翅 → meat; 黑芝麻汤圆/酸汤水饺/
  螺蛳粉 → staple; 上汤娃娃菜 → veg; 开水白菜/汽锅鸡/跷脚牛肉 → by protein;
  莲藕花生排骨汤/西湖牛肉羹/紫菜蛋花汤 → soup.
- Knock-on benefits (no extra code): soup slots only host real soups, meat
  slots regain ~100 dishes, weekday -30 and the 靓汤 theme stop mis-firing,
  `weekday_slow_soup_penalty` semantics become honest.

## 4. Part 2 — weekly slow-soup cap (seed v4, rule A)

| Field | Value |
|---|---|
| Code | `weekly_slow_soup_limit` |
| Name | 费时汤一周最多两道 |
| Expression | `candidate.dish_role == "soup" && candidate.cook_time > 45 && countWeekSlowSoup(45) >= 2 ? -25 : 0` |
| Template | `{"type":"limit","scope":"week","category":"slow_soup","n":2,"points":25,"strength":"prefer"}` |
| Kind / severity | score / soft, relaxable, scope `week` |

- New env helper `countWeekSlowSoup(minutes int) int`: count of already-
  planned week dishes with `dish_role=="soup" && cook_time > minutes`.
  (Existing `countWeek(field, value)` only does equality counts.)
- `n` (the 2) and `points` (the 25) are tunable on the rules page via the
  existing limit-template stepper; the 45-minute threshold stays a fixed
  part of the seeded expression (consistent with `slow_meal_spread`).
- Week-plan only. The tomorrow-pick and wheel paths have no week state, so
  this rule is **not mirrored** there (unlike the three v3 per-dish
  penalties, which are mirrored in `pickPenaltyAdjustment`).
- Interaction: stacks with weekday -30. Net effect: weekdays suppressed,
  weekends allow one long stew each, a 3rd long stew that week fights a -25.

## 5. Part 3 — adjacent-day soup ingredient repetition (seed v4, rule B)

| Field | Value |
|---|---|
| Code | `soup_ingredient_repeat_penalty` |
| Name | 今天的汤不和昨天撞主料 |
| Expression | `candidate.dish_role == "soup" && countOverlapPrevSoup("ingredients", candidate.ingredients) > 0 ? -18 : 0` |
| Template | `{"type":"no_repeat","scope":"prev_day","category":"soup_main_ingredient","points":18,"strength":"prefer"}` |
| Kind / severity | score / soft, relaxable, scope `candidate` |

- New env entry `prev_soups []ruleDishEnv`: the soup-slot dishes of the
  **adjacent day(s)** — sequential generation passes the previous day;
  single-day regeneration (`regenerate-day`) passes BOTH the previous and
  the next day (their menus already exist; lesson from the earlier
  regenerate-day audit: that path must not be forgotten).
- New helper `countOverlapPrevSoup(field, values)`: overlap count against
  `prev_soups`, skipping a base-aromatics blocklist so 葱/姜 overlap doesn't
  trigger. Blocklist (finalized at implementation, approximately): 葱, 小葱,
  大葱, 葱花, 姜, 生姜, 蒜, 大蒜, 香菜, 盐, 食用油, 料酒, 生抽, 老抽, 枸杞,
  红枣, 八角, 花椒, 胡椒, 白胡椒, 鸡精, 味精, 糖, 冰糖, 清水, 热水, 高汤.
- Sentence: 「今天的汤不和昨天的汤用同一主料，撞了降 18 分」; `points`
  tunable via the existing card mechanics.
- Day-0 (Monday) of a fresh generation has no previous day → `prev_soups`
  empty → rule inert. Soup-scarce libraries degrade gracefully: soft score,
  relaxable at ladder stage 5.
- Week-plan only (tomorrow pick has no "yesterday's plan" context; meal
  records are a different signal — out of scope).

## 6. Seed migration (v3 → v4)

- `menuRulesSeedVersion` 3 → 4. `syncDefaultMenuRules` machinery is reused
  unchanged: missing codes inserted, factory-expression rows refreshed,
  user-edited rows and Enabled state preserved.
- Default rules 12 → 14. Template drift test extended to 14 (byte-match
  invariant), plus migration test v3 → v4 (user-modified rule survives,
  disabled rule stays disabled).
- `menuRuleCategoryPredicate` / template renderer gain the two new
  categories; `limit` type already renders `countWeek`-style sentences, the
  `no_repeat` type gains the `prev_day` scope wording.
- Frontend `menuRuleTemplates.ts`: `DISPLAY_ONLY_CATEGORY_LABELS` +
  `{slow_soup: "费时汤（炖煮超45分钟）", soup_main_ingredient: "昨天汤的主料"}`;
  PresetRuleCard needs no structural change (limit/no_repeat cards exist).

## 7. Touchpoints

| File | Change |
|---|---|
| `backend/internal/models/dish_traits.go` | `IsSoupName`, soup branch rewiring in role inference |
| `backend/internal/services/week_plan_preferences.go` | `isSoupDish` → wrapper; delete `soupDishKeywords` |
| `backend/internal/models/menu_rules.go` | 2 new default rules, seed version comment |
| `backend/internal/services/menu_rule_service.go` | seed v4, `prev_soups` env, `countWeekSlowSoup` + `countOverlapPrevSoup` helpers |
| `backend/internal/services/menu_rule_templates.go` | renderer cases for `slow_soup` (limit/week) and `soup_main_ingredient` (no_repeat/prev_day) |
| `backend/internal/services/plan_service.go` | thread adjacent-day soups into `weekPlanDayContext` for generate + regenerate-day |
| `frontend/src/lib/menuRuleTemplates.ts` | 2 display labels |
| tests | role-inference boundary table, isSoupDish table update, drift 14, migration v3→v4, slow-soup week cap, prev-soup overlap (blocklist + regenerate both-neighbors) |

## 8. Test boundaries (role inference)

| Dish | Trigger text | Expected role |
|---|---|---|
| 红烧排骨 | step "收浓汤汁" | meat (was soup) |
| 清蒸鲈鱼 | remark "倒掉原汤" | meat (was soup) |
| 酸汤鱼 | mid-name 汤 | meat (was soup) |
| 黑芝麻汤圆 | mid-name 汤 | staple (was soup) |
| 上汤娃娃菜 | mid-name 汤 | veg (was soup) |
| 开水白菜 / 汽锅鸡 | tag "汤品" | non-soup (was soup) |
| 腊味煲仔饭 / 咸鱼鸡粒茄子煲 | 煲 | staple / meat (was soup) |
| 莲藕花生排骨汤 | name suffix 汤, category 粤菜 | soup |
| 西湖牛肉羹 | name suffix 羹 | soup |
| 紫菜蛋花汤 | category 汤品 | soup |

## 9. Out of scope (deliberate)

- `difficulty` re-inference (low discrimination acknowledged; cook_time
  carries the load after the fake-soup fix).
- Wheel (今晚吃什么) time penalties.
- Weekday -30 magnitude changes — observe after Part 1 lands.
- Mirroring the two new rules into the tomorrow-pick path (no week/prev-day
  state there).
- Cross-day repetition control for non-soup dishes.

## 10. Expected observable effects

- "汤" in the system: 198 → 60, all genuine table soups.
- Soup slots only serve real soups; 红烧排骨-class dishes compete as meat.
- Long stews: weekdays suppressed (-30), ≤2 per week (3rd fights -25) —
  i.e. one per weekend day as the steady state.
- No 鲫鱼汤 → 鲫鱼豆腐汤 back-to-back days (soft -18).
