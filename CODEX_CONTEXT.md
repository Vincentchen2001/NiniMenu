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

Current custom commit:

```text
2311591 Allow lunch dish count to be zero
```

The fork was verified through GitHub API as a fork of `TryHarder-L/NiniMenu`.

## What Was Changed

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

