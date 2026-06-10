# Week Plan Summary Badges Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the week-plan summary card's config-echo stats (总菜数 / 跳过的餐 / 状态 cells) with a live badge row: 🥩荤 / 🥬素 / 🍲汤 / ❤️收藏 counts plus a row-end sync-status pill, with orange attention coloring.

**Architecture:** Single-file frontend change in `frontend/src/pages/WeekPlan.tsx`. A pure module-level helper `weekBadgeCounts(plan)` does one pass over the draft plan's lunch+dinner arrays; a `badgeClass(attention)` helper returns pill classes; the component memoizes counts on `draftPlan`. The old `dishCount` / `skippedMealCount` helpers are deleted (grep-confirmed unused elsewhere). Spec: `docs/superpowers/specs/2026-06-10-week-plan-summary-card-design.md`.

**Tech Stack:** React 18 + TypeScript, Tailwind utility classes. No new dependencies, no backend/API/type changes.

**Verification note:** The frontend has NO test runner (package.json scripts: dev/build/lint/preview only — verified). Do not add one for this. Each task verifies via `npm run build` (runs `tsc -b`) and `npx eslint` on the touched file only (the repo has 17 pre-existing lint errors in other files — do not try to fix them). Final visual check is manual in the running dev server.

---

### Task 1: Badge counts helper + badge row UI

**Files:**
- Modify: `frontend/src/pages/WeekPlan.tsx` (three regions: module helpers ~line 179, component body ~line 690, summary-card JSX ~line 911)

- [ ] **Step 1: Replace the two dead stat helpers with badge helpers**

In `frontend/src/pages/WeekPlan.tsx`, find this exact block (module scope, after `quotaTotal`):

```tsx
function dishCount(plan: WeekPlanType) {
  return plan.days.reduce((sum, day) => sum + day.lunch.length + day.dinner.length, 0)
}

function skippedMealCount(plan: WeekPlanType) {
  return plan.days.reduce((sum, day) => sum + (day.lunch.length === 0 ? 1 : 0) + (day.dinner.length === 0 ? 1 : 0), 0)
}
```

Replace it with:

```tsx
const FAVORITE_LOW_THRESHOLD = 3

type WeekBadgeCounts = { meat: number; veg: number; soup: number; favorite: number }

function weekBadgeCounts(plan: WeekPlanType): WeekBadgeCounts {
  const counts: WeekBadgeCounts = { meat: 0, veg: 0, soup: 0, favorite: 0 }
  for (const day of plan.days) {
    for (const dish of [...day.lunch, ...day.dinner]) {
      if (dish.dish_role === "meat") counts.meat += 1
      else if (dish.dish_role === "veg") counts.veg += 1
      else if (dish.dish_role === "soup") counts.soup += 1
      if (dish.favorite) counts.favorite += 1
    }
  }
  return counts
}

function badgeClass(attention: boolean) {
  return `inline-flex h-8 items-center gap-1 rounded-full px-3 text-[12px] font-bold ${
    attention ? "bg-primary-light text-primary" : "bg-bg text-text2"
  }`
}
```

Notes for the implementer: `Dish.dish_role` is a plain `string` (`"meat" | "veg" | "soup" | "staple" | "side"` in practice); staples/sides are intentionally not counted, so the three role badges need not sum to the total. Duplicate dishes count once per occurrence. `favorite` is a `boolean` on `Dish`.

- [ ] **Step 2: Memoize the counts in the component**

In the `WeekPlan` component body, find this exact block (the `selectedIds` memo):

```tsx
  const selectedIds = useMemo(() => {
    const set = new Set<number>()
    if (!activePickerDay) return set
    activePickerDay.lunch.forEach((dish) => set.add(dish.id))
    activePickerDay.dinner.forEach((dish) => set.add(dish.id))
    return set
  }, [activePickerDay])
```

Insert directly after it:

```tsx
  const badges = useMemo(() => weekBadgeCounts(draftPlan), [draftPlan])
```

(`draftPlan` is replaced immutably on every edit/regenerate, so identity-based memoization is correct. `useMemo` is already imported at the top of the file.)

- [ ] **Step 3: Replace the three-cell stats grid with the badge row**

In the summary-card JSX, find this exact block:

```tsx
          <div className="grid grid-cols-3 gap-2">
            <div className="rounded-2xl bg-primary-light px-3 py-3">
              <div className="text-[11px] font-bold text-primary/80">总菜数</div>
              <div className="mt-1 text-xl font-extrabold text-primary">{dishCount(draftPlan)}</div>
            </div>
            <div className="rounded-2xl bg-mint-light px-3 py-3" title="数量设为 0 或没排上菜的餐次">
              <div className="text-[11px] font-bold text-mint/80">跳过的餐</div>
              <div className="mt-1 text-xl font-extrabold text-mint">{skippedMealCount(draftPlan)}</div>
            </div>
            <div className="rounded-2xl bg-bg px-3 py-3">
              <div className="text-[11px] font-bold text-text3">状态</div>
              <div className="mt-1 truncate text-[13px] font-extrabold text-text">{dirtyPlan || dirtyPrefs ? "有改动" : "已同步"}</div>
            </div>
          </div>
```

Replace it with:

```tsx
          <div className="flex flex-wrap items-center gap-2">
            <span className={badgeClass(badges.meat === 0)}>🥩 荤 {badges.meat}</span>
            <span className={badgeClass(badges.veg === 0)}>🥬 素 {badges.veg}</span>
            <span className={badgeClass(badges.soup === 0)}>🍲 汤 {badges.soup}</span>
            <span className={badgeClass(badges.favorite < FAVORITE_LOW_THRESHOLD)}>❤️ 收藏 {badges.favorite}</span>
            <span className={`ml-auto ${badgeClass(dirtyPlan || dirtyPrefs)}`}>
              {dirtyPlan || dirtyPrefs ? "● 未保存" : "✓ 已同步"}
            </span>
          </div>
```

Everything below the block stays untouched: the `mt-3 grid grid-cols-2` buttons row (推荐设置 / 重生成) and the warnings area. `dirtyPlan` / `dirtyPrefs` are component state already in scope here.

- [ ] **Step 4: Build to verify types and bundling**

Run: `cd /Users/vincentchen/Documents/GitHub/NiniMenu/frontend && npm run build`
Expected: ends with `✓ built in …`. A TS error like `Cannot find name 'dishCount'` means a stale reference survived Step 3 — fix before proceeding.

- [ ] **Step 5: Lint only the touched file**

Run: `cd /Users/vincentchen/Documents/GitHub/NiniMenu/frontend && npx eslint src/pages/WeekPlan.tsx`
Expected: no output, exit code 0.

- [ ] **Step 6: Commit**

```bash
cd /Users/vincentchen/Documents/GitHub/NiniMenu && git add frontend/src/pages/WeekPlan.tsx && git commit -m "$(cat <<'EOF'
Replace week plan stats with role/favorite badge row

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: CHANGELOG entry

**Files:**
- Modify: `CHANGELOG.md` (v0.7.0 → `### 优化` section, currently ends at the 修复推荐去重失效 bullet)

- [ ] **Step 1: Append a bullet to the v0.7.0 优化 section**

In `CHANGELOG.md`, find this exact line:

```markdown
- **修复推荐去重失效** — GORM `Pluck` 会重置目标切片，导致已吃记录被推荐记录覆盖；冷却窗口内的菜可能被重复推荐
```

Insert directly after it:

```markdown
- **周菜单概览徽章** — 顶部卡片的 总菜数/跳过的餐/状态 三格改为 荤/素/汤/收藏 徽章行，某类为 0 或收藏少于 3 道时变橙提醒；状态收为行尾小胶囊
```

- [ ] **Step 2: Commit**

```bash
cd /Users/vincentchen/Documents/GitHub/NiniMenu && git add CHANGELOG.md && git commit -m "$(cat <<'EOF'
Document week plan summary badges in changelog

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
```

---

### Final manual smoke check (running dev server, user-visible)

Dev servers are already running (backend :8080, frontend :5173 with HMR). In the browser on 一周菜单:

1. Badge row shows 荤/素/汤/收藏 counts matching the visible plan; 总菜数/跳过的餐 cells are gone.
2. With current data (soup quota > 0, favorites ≥ 3) all four badges are neutral; status pill reads `✓ 已同步`.
3. Swap or remove a dish → counts update immediately and status pill flips to orange `● 未保存`; saving flips it back.
4. 推荐设置 / 重生成 buttons and the warnings area render unchanged below the badges.
