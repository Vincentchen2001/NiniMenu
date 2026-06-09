# Codex Development Context

Last updated: 2026-06-09

This file is a handoff note for future Codex conversations. Open this repository
from the stable project folder:

```text
/Users/vincentchen/Documents/GitHub/NiniMenu
```

## Repository Setup

The user's fork is the primary writable remote:

```text
origin   https://github.com/Vincentchen2001/NiniMenu.git
```

The original project is kept as upstream for future updates:

```text
upstream https://github.com/TryHarder-L/NiniMenu.git
```

Recent custom commits:

```text
2311591 Allow lunch dish count to be zero
629f977 Disable automatic achievements by default
9c0f78b Refactor weekly menu planner
e94be21 Improve weekly menu export image
```

The fork was verified through GitHub API as a fork of `TryHarder-L/NiniMenu`.

## What Was Changed

### Lunch Count Can Be Zero

The user does not always need lunch. The app previously forced lunch generation
because `lunch_dishes_per_day=0` was treated as invalid and fell back to a
positive default.

Implemented behavior:

- Admin settings now allow `lunch_dishes_per_day` to be `0`.
- Backend settings parsing accepts zero instead of falling back to the default.
- Week plan generation returns an empty lunch array when lunch count is zero.
- Empty meal lists are serialized as `[]`, not `null`.
- Week plan cache is invalidated when lunch or dinner dish counts change.
- Tomorrow page reads global meal-count settings instead of hard-coded defaults.
- Tomorrow page can show and generate `0` lunch dishes while still generating dinner.

Main files changed:

```text
backend/internal/handlers/manage.go
backend/internal/services/plan_service.go
backend/internal/services/plan_service_test.go
frontend/src/pages/Tomorrow.tsx
frontend/src/pages/admin/Settings.tsx
```

### Achievements Default To Locked

The user did not want achievement unlock popups appearing repeatedly. Automatic
achievements now default to disabled/locked unless explicitly configured.

Implemented behavior:

- Achievement auto-unlock defaults are disabled at database/settings level.
- Admin settings expose the achievement lock/default behavior.
- Backend handler and service tests cover the new achievement default behavior.
- Local runtime data is ignored via root `.gitignore`:

```gitignore
/backend/data/
```

Main files changed:

```text
.gitignore
backend/internal/database/database.go
backend/internal/handlers/manage.go
backend/internal/handlers/manage_test.go
backend/internal/services/achievement_service.go
backend/internal/services/achievement_service_test.go
frontend/src/pages/admin/Settings.tsx
```

### Weekly Menu Planner

The bottom navigation "照片墙" entry was replaced by a first-class "菜单" entry.
The old photo wall page still exists as a route, but it is no longer the main
bottom-tab entry.

Implemented behavior:

- `/week-plan` is the dedicated weekly menu page.
- Weekly menu preferences are stored through backend APIs:
  - `GET /api/week-plan/preferences`
  - `PUT /api/week-plan/preferences`
  - `PUT /api/week-plan`
- Workday/weekend preferences can be configured independently.
- Each meal supports separate `meat_count`, `veg_count`, and `soup_count`.
- Lunch or dinner can be set to `0`; empty meal sections are hidden in the
  weekly menu display and PNG export.
- Recommendation profiles include balanced, quick, light, spicy, and favorite.
- Soup dishes are recognized separately from meat/vegetable quotas. Soup
  matching checks category, tags, and name for soup-related terms.
- Weekly recommendations use recent history to reduce repetition.
- Settings are collapsible so they do not dominate the main page.
- Users can manually add/remove dishes from a weekly menu draft and save it.
- Weekly menus can be exported as PNG.

Main files changed:

```text
backend/internal/handlers/plan.go
backend/internal/models/other.go
backend/internal/routes/routes.go
backend/internal/services/pick_service.go
backend/internal/services/plan_service.go
backend/internal/services/plan_service_test.go
backend/internal/services/week_plan_preferences.go
backend/internal/services/week_plan_recommendations.go
frontend/src/App.tsx
frontend/src/api/index.ts
frontend/src/layouts/MainLayout.tsx
frontend/src/lib/weekPlanExport.ts
frontend/src/pages/More.tsx
frontend/src/pages/WeekPlan.tsx
frontend/src/types/index.ts
```

### Weekly Menu PNG Export

The first PNG export was readable but too sparse and plain. It was redesigned
as a two-column menu poster.

Implemented behavior:

- Exported file names include the week start date and current `HHmm`, for
  example `ninimenu-week-plan-2026-06-08-1415.png`, so repeated exports do not
  make it easy to inspect an old file by mistake.
- The toast now includes the exact file name:
  `已开始下载：<file>.png`.
- The canvas export uses a two-column poster layout with day cards, meal tags,
  dish chips, and a compact header.
- Empty lunch/dinner sections are omitted from export.
- A visual smoke check of `~/Downloads/ninimenu-week-plan-2026-06-08-1415.png`
  found that the header date was overlapped by preference cards. The header was
  then expanded and preference cards were moved lower to avoid overlap.

Main files changed:

```text
frontend/src/lib/weekPlanExport.ts
frontend/src/pages/WeekPlan.tsx
```

## Verification Already Done

In the original working copy, before moving to the stable folder:

```bash
GOCACHE=/private/tmp/ninimenu-gocache go test ./...
npm run build
```

Both passed. `npm run lint` was also tried, but the upstream project already has
repo-wide lint issues unrelated to this change, so lint is not currently clean.

After migrating to the stable folder:

```bash
GOCACHE=/private/tmp/ninimenu-gocache go -C /Users/vincentchen/Documents/GitHub/NiniMenu/backend test ./...
git -C /Users/vincentchen/Documents/GitHub/NiniMenu diff --check
```

Both passed.

The local UI was smoke-tested with lunch count set to `0`. The Tomorrow page
showed lunch as `0` dishes and still generated dinner normally.

For the achievement lock change:

```bash
git diff --check
GOCACHE=/private/tmp/ninimenu-gocache go -C backend test ./...
npm --prefix frontend run build
```

All passed before commit `629f977`.

For the weekly menu refactor:

```bash
git diff --check
GOCACHE=/private/tmp/ninimenu-gocache go -C backend test ./...
npm --prefix frontend run build
```

All passed before commit `9c0f78b`. The in-app browser also verified that when
weekday lunch is set to `0`, weekday cards render only dinner rather than an
empty lunch block.

For the PNG export redesign:

```bash
git diff --check
npm --prefix frontend run build
```

Both passed before commit `e94be21`. The in-app browser triggered exports and
showed the new exact-filename toast. The `1415` export image was inspected and
the header overlap found there was fixed afterward; re-export manually if more
visual polish is needed.

## Local Run Notes

Backend:

```bash
cd /Users/vincentchen/Documents/GitHub/NiniMenu/backend
go run cmd/server/main.go
```

Frontend:

```bash
cd /Users/vincentchen/Documents/GitHub/NiniMenu/frontend
npm install
npm run dev
```

Typical local URLs:

```text
Backend:  http://localhost:8080
Frontend: http://localhost:5173
```

The default app/admin password from the upstream docs was `nini123` during the
initial setup.

## Updating From Upstream

Use this when the original project releases updates:

```bash
cd /Users/vincentchen/Documents/GitHub/NiniMenu
git fetch upstream
git merge upstream/main
git push
```

If conflicts happen, expect them most likely around the files listed above.

## Runtime Files

Do not accidentally commit runtime data such as local databases, temporary
server logs, or generated dev artifacts. If the app is run from the stable
folder, check `git status` before committing.

Good status check:

```bash
git status -sb
```
