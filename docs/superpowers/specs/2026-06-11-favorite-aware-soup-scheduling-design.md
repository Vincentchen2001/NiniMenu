# Favorite-Aware Penalties & Soup Time-Awareness — Design

Date: 2026-06-11
Status: approved by user (interactive brainstorm)

## 1. Problem

Two recommendation-quality gaps reported by the user:

1. **Cuisines the household can't cook keep getting recommended.** The
   built-in catalog seeds ~50–60 dishes across 11 cuisine packs (新疆菜,
   云南菜, …) with `enabled=true`. Week-plan generation and tomorrow picks
   treat every enabled dish equally, so dishes nobody can cook surface
   regularly. For some cuisines the household can cook only one or two
   dishes; those should stay recommendable while the rest of the cuisine
   recedes.
2. **Soups are mis-handled twice.** (a) The light-profile matcher counts
   *any* dish whose category contains 汤 as 清淡, so a 浓香/hard 猪肚鸡汤
   matches a light day. (b) Soup-slot scoring ignores difficulty and cook
   time: ~2/3 of the built-in soups are 60+-minute stews, and only a ~10-pt
   base-score nudge separates an 8-minute 紫菜蛋花汤 from a 120-minute
   猪肚鸡汤, so weekday dinners routinely draw long stews. The only related
   rule (`slow_meal_spread`) merely prevents *two* slow dishes in one meal.

## 2. Confirmed product decisions

- **Favorite = can-cook.** No new dish fields. `Dish.Favorite` (kept in sync
  with the `favorites` table by `AddFavorite`/`RemoveFavorite`) is the
  capability signal. The user maintains it by favoriting dishes they cook.
- **Soft demotion, not exclusion.** Un-favorited dishes are heavily
  down-weighted but may still appear occasionally (a deliberate "learn a new
  dish" opening). No hard filtering.
- **Two-layer penalty.** Base layer: any un-favorited dish. Cuisine layer: an
  extra penalty when the dish's category contains *zero* favorites — this is
  what makes 新疆菜 (no favorites) recede as a whole while a cuisine with 1–2
  favorited dishes only suffers the base layer on its other dishes.
- **Slow soups: avoid on weekdays, free on weekends.** Mon–Fri the soup slot
  prefers quick soups; Sat/Sun long stews compete normally.
- **Rich soups stop counting as light.** A soup matches the light profile
  only when its inferred richness is low.
- New behavior ships as **built-in menu rules** wherever expressible, so the
  rules settings page shows them with toggles and adjustable numbers.

## 3. Part 1 — favorite-aware penalties

Two new default score rules (seed v3):

| Code | Expression (intent) | Default | Scope / kind / severity |
|---|---|---|---|
| `non_favorite_penalty` | `candidate.favorite ? 0 : -12` | -12 | candidate / score / soft, relaxable |
| `unfamiliar_category_penalty` | `candidate.category != "" && candidate.category_favorites == 0 ? -25 : 0` | -25 | candidate / score / soft, relaxable |

Behavior:

- A cuisine with zero favorites: every dish starts -37 relative to favorites.
  Profile/role bonuses (+40/+60) can still occasionally outweigh it when the
  pool runs dry or the dish uniquely fits a theme — the intended "偶尔学新菜"
  opening.
- A cuisine with 1–2 favorites: the favorited dishes carry no penalty at all;
  siblings get only the base -12.
- Dishes with an empty category only get the base layer (guard in the
  expression), so uncategorized user entries are not over-punished.
- Within a single-category pool (e.g. the soup slot when no soup is
  favorited) the penalties apply uniformly, so relative order inside the slot
  is unchanged — quotas still fill.

### Data plumbing

- `ruleDishEnv` gains `CategoryFavorites int` (`expr:"category_favorites"`).
- Favorite counts per category come from one aggregate query over all
  non-deleted dishes (`SELECT category, COUNT(*) … WHERE favorite = 1 GROUP BY
  category`), computed once per generation/pick and passed into the env
  builders. Counting all dishes (not just enabled ones) keeps a disabled
  favorited dish as capability evidence.

### Tomorrow-pick parity (no double counting)

`PickTomorrowDishes` does not run the rule engine, and the week-plan path
reuses `tomorrowDishScore` inside `baseWeekPlanDishScore`. Therefore the
penalties must NOT go into `tomorrowDishScore` (the week plan would then
apply them twice — once via Go, once via rules). Instead the pick path adds
the same two terms as a separate additive adjustment when sorting its pool.

## 4. Part 2 — soup fixes

### 4a. Rich soups stop matching "light"

In `matchesTomorrowProfile` (shared by both paths), the light branch drops
the unconditional `strings.Contains(category, "汤")` clause and instead
accepts a soup only when `RichnessLevel <= 1`:

```
light: 清淡 taste || 鲜 taste || (soup-ish && richness_level <= 1)
```

where soup-ish = `category contains 汤 || isSoupDish(dish)`. The `+8` light
bonus for 汤 categories in `tomorrowDishScore` gets the same richness guard.
Dishes are passed through `ensureDishTraits` before the check so inferred
richness is populated on both paths.

### 4b. Weekday slow-soup penalty

Third new default score rule:

| Code | Expression (intent) | Default |
|---|---|---|
| `weekday_slow_soup_penalty` | `!is_weekend && candidate.dish_role == "soup" && (candidate.cook_time > 45 || candidate.difficulty == "hard") ? -30 : 0` | -30 |

- `is_weekend` becomes a new env binding. Week plan: `weekPlanDayContext`
  carries `isWeekend` (day index 5/6, 0=Monday) and threads it into
  `evaluateConstraintRules` / `evaluateScoreRules` → `buildRuleEnv`.
- Tomorrow picks: weekend = `time.Now().AddDate(0,0,1)` falling on Sat/Sun;
  the same penalty is applied as a pick-side additive term (rule engine not
  involved), keeping both paths consistent.
- `cook_time = 0` (unknown) and non-hard difficulty pass untouched —
  user-entered soups without timing data are not punished.
- Soup-role detection relies on `dish_role == "soup"` (traits are ensured in
  `dishRuleEnv`); keyword-only soups get their role from trait inference.

## 5. Seed migration (v2 → v3)

- `menuRulesSeedVersion` 2 → 3. Migration inserts the three new rules only
  when their `code` is absent — user-modified or deleted existing rules are
  never touched (same philosophy as `migrateMenuRulesToV2`).
- Project invariant: every default rule carries a sentence template whose
  rendered expression matches the seeded expression (drift-tested). The
  template renderer (`menu_rule_templates.go`, types `limit | no_repeat |
  prefer | avoid`) is extended — either new categories on existing types or
  new types — so the three rules render proper sentences with adjustable
  numbers in the rules UI. The drift test gains the three new cases.
- Frontend rules page: render the new templates' sentences (reuse the
  existing preset-rule card mechanics; exact wording in implementation).

## 6. Touchpoints

| File | Change |
|---|---|
| `backend/internal/models/menu_rules.go` | 3 new default rules (+ templates) |
| `backend/internal/services/menu_rule_service.go` | `ruleDishEnv.CategoryFavorites`, `is_weekend` binding, seed v3 migration |
| `backend/internal/services/menu_rule_templates.go` | renderer support for the new rule shapes |
| `backend/internal/services/plan_service.go` | `weekPlanDayContext.isWeekend`, category-favorite counts in gen context, thread both into rule evaluation |
| `backend/internal/services/pick_service.go` | light-branch richness guard (match + score), pick-side additive penalties (favorites ×2, weekday slow soup) |
| `frontend/src/components/rules/PresetRuleCard.tsx`, `RuleSentenceBuilder.tsx` (rules UI under admin Settings) | sentence rendering for the 3 new rule templates |
| tests | template drift cases, env extension, weekday/weekend behavior, double-count guard |

## 7. Tuning & out of scope

- -12 / -25 / -30 / 45-minute threshold are starting values; all are
  user-adjustable on the rules page after shipping. Expect one tuning pass
  against real generations.
- Out of scope (deliberate): hard weekly caps for unfamiliar cuisines
  (add later as a constraint rule if soft scoring proves insufficient);
  weekday avoidance for slow *non-soup* dishes (user can clone the rule);
  any auto-inference of can-cook from meal records.
