# Week Plan Summary Card — Design

Date: 2026-06-10
Status: approved by user (interactive brainstorm)

## 1. Problem

The week-plan page's top card shows three numbers: 总菜数 (total dishes),
跳过的餐 (skipped meals), 状态 (sync status). The user rejected the first two
as meaningless. Both merely echo the user's own quota configuration: the total
is the sum of configured per-meal quotas, and the skipped count is the number
of meals the user set to 0. The only informative deviation — a slot that could
not be filled — is already surfaced by the warnings area below the buttons.

## 2. Confirmed product decisions (user interview)

- **Usage moments**: start-of-week planning workbench, plus showing the week
  to family for taste confirmation. NOT used for daily "what's for dinner"
  (Home page covers that) or for shopping prep.
- **Signals that actually trigger manual adjustment** (regenerate / swap):
  meat-veg balance, and too few favorite dishes. Nothing else.
- **Chosen approach**: badge row (option B), over re-filled three-cell
  numbers (A) and a ✓/✗ checklist "health report" (C, rejected as
  threshold-heavy and space-hungry).

## 3. Design

### 3.1 Content & computation (frontend-only)

Replace the three stat cells with one wrapping badge row computed from the
current draft plan (lunch + dinner across all 7 days), updating live as the
user swaps dishes or regenerates:

| Badge | Computation |
|---|---|
| 🥩 荤 N | dishes with `dish_role === "meat"` |
| 🥬 素 N | dishes with `dish_role === "veg"` |
| 🍲 汤 N | dishes with `dish_role === "soup"` |
| ❤️ 收藏 N | dishes with `favorite === true` |
| status (row end) | unsaved changes (`dirtyPlan \|\| dirtyPrefs`) → `● 未保存`; else `✓ 已同步` |

- Staples and sides are not shown — not decision-relevant for this user.
- Duplicate dishes across the week each count once per occurrence.
- Dishes with empty/other `dish_role` are simply not counted, so the three
  role badges need not sum to the dish total (which is no longer shown).
- `dishCount()` / `skippedMealCount()` helpers and their cells are deleted
  (grep confirmed no usage outside `WeekPlan.tsx`).

### 3.2 Attention coloring (orange = worth a look, not an error)

- Any of 荤 / 素 / 汤 at **0** → that badge turns orange.
- 收藏 < `FAVORITE_LOW_THRESHOLD` (named constant, value 3) → favorite badge
  turns orange.
- The unsaved status badge uses the same warm accent; the synced state is
  neutral.

### 3.3 Layout & style

- Badge row: `flex flex-wrap gap-2` so narrow screens wrap naturally.
- Normal badge: neutral (`bg-bg`, `text-text2`). Attention badge: existing
  warm accent (`bg-primary-light`, `text-primary`) — same visual language as
  the warnings area.
- Badges are display-only, not clickable. (Future idea, out of scope: tapping
  the favorite badge highlights favorite dishes in the day cards.)
- Everything else on the card is unchanged: 推荐设置 / 重生成 buttons,
  warnings area. Day theme strip and day cards untouched.

## 4. Implementation notes

- Single file: `frontend/src/pages/WeekPlan.tsx`.
  - Replace the `grid grid-cols-3` stats block inside the summary card
    (currently ~lines 911–924).
  - Add a pure helper `weekBadgeCounts(plan)` returning
    `{ meat, veg, soup, favorite }` in one pass over each day's
    lunch + dinner arrays; memoize with `useMemo` on the draft plan.
  - Delete `dishCount` / `skippedMealCount`.
- No backend, API, or type changes.

## 5. Verification

- `npm run build` passes; no new lint errors in the touched file.
- Manual checks: regenerate → counts change; toggling a dish's favorite and
  re-opening the plan updates ❤️; a week with zero soup shows an orange 🍲 0;
  saving flips the status badge to ✓ 已同步.

## 6. Non-goals

- Shopping/ingredient aggregation on this page.
- Freshness / repeat-vs-last-week stats (needs backend queries; repetition was
  not one of the user's adjustment triggers).
- Per-day balance flags or tunable "health" scoring.
