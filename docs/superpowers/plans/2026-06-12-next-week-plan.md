# Next-Week Plan (下周菜单提前生成) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the family generate and edit next week's menu ahead of time (typically on the weekend, to shop for ingredients), without disturbing the current week.

**Architecture:** The `week_plans` table is already keyed by `(user_id, week_start)` — multi-week storage exists. This plan parameterizes the service layer by week-start key (current code hardcodes `getCurrentWeekKey()`), adds a `?week=next` query param to the four week-addressable endpoints, and adds a 本周/下周 tab to the WeekPlan page with an empty-state "生成下周菜单" button. Next week is generated only on explicit request, never as a read side effect.

**Tech Stack:** Go (Gin + GORM + sqlite), React/TS (@tanstack/react-query). Backend tests via `go test ./...` in `backend/`; frontend verified by `npm run build`.

---

## Design decisions (locked — user said "按方案1直接完成", no further questions)

1. **Week addressing:** query param `week` ∈ {absent, `current`, `next`}. Anything else → 400 `week 参数无效（可选 current/next）`. No arbitrary-date addressing (YAGNI; History page already covers the past).
2. **No auto-generation for next week:** `GET /week-plan?week=next` returns `{days: []}` when no snapshot exists. Generation happens only via `POST /week-plan/regenerate?week=next` (frontend empty-state button). Current week keeps its auto-generate-on-read behavior unchanged.
3. **Freshness cap extension:** `lastSeenDishDates` gains a `weekStart` param; the recommendation-row cap becomes `max(today, weekStart-1)`. For the current week `weekStart-1` is last Sunday ≤ today, so cap = today — behavior unchanged. For next week, cap = this Sunday, so dishes planned for the rest of THIS week count as "recently seen" when generating next week (prevents this-Sunday → next-Monday repeats). A week's own rows are never included when generating it (cap < its first day).
4. **Recent-cooldown stage (3-day hard exclude) stays today-based.** Next-week days are ≥3 days out for most of the week anyway; the decay penalty (point 3) covers grading. Documented, not changed.
5. **Per-week one-off preferences:** `GetWeekPlanPreferences`/`SaveWeekPlanPreferences` get `ForWeek` variants reading/writing THAT week's row `prefs_json`. Resident preferences (the Settings row) remain shared across weeks. Generating next week consumes next week's one-off prefs.
6. **SaveWeekPlan routes by content:** week key derived from `mondayOf(plan.Days[0].Date)` (fallback: current week when days empty/unparsable). The in-memory cache is only updated when the derived key equals the current week. `restorePastDaysFromStored` / `replaceWeekPlanRecommendations` need NO changes: next-week days are all ≥ today, and the recommendation delete is scoped `planned_date IN (plan's own dates)`.
7. **Invalidation widens to future weeks:** `InvalidateWeekPlanCache` clears `plan_json` for `week_start >= current` (a pre-generated next week goes stale on the same triggers: dish disabled, rules changed, …). Past weeks stay untouched.
8. **Day regeneration generalizes by date:** `RegenerateWeekPlanDay(date)` derives the week from the date. Past dates still rejected; a next-week date errors with `该周菜单尚未生成，请先生成` if no snapshot exists.
9. **Empty-generation guard:** `RegenerateWeekPlanForWeek` skips `SaveWeekPlan` when generation produced 0 days (no enabled dishes). Previously an empty plan was saved to the current row — pointless (the read path treats empty as "not generated"), and for next week it would mis-route to the current row via the days[0] fallback.
10. **Frontend:** single WeekPlan page with a 本周/下周 segmented tab. Query keys gain the week dimension. Switching tabs while `dirtyPlan || dirtyPrefs` is blocked with a toast (no cross-week draft bleed). Day-theme sheet, manual add/remove, save, per-day regenerate all operate on the visible week unchanged (they're date/body-driven).
11. **Out of scope:** shopping list date range (it reads `ShoppingCheck` rows for today+tomorrow only — pre-existing limitation, separate feature), auto-rolling generation (cron), viewing past weeks via tabs (History page exists), achievement-event changes (kept as-is for both weeks).

**Versioning:** CHANGELOG `v0.10.0`.

---

## File map

| File | Change |
|---|---|
| `backend/internal/services/plan_service.go` | `weekKeyWithOffset`, `mondayOf`, exported `CurrentWeekStart`/`NextWeekStart`; parameterize `buildWeekPlanGenContext`, `GenerateWeekPlanForWeek`, `GetWeekPlanForWeek`, `RegenerateWeekPlanForWeek`, `RegenerateWeekPlanDay`; widen `InvalidateWeekPlanCache` |
| `backend/internal/services/week_plan_freshness.go` | `lastSeenDishDates(now, weekStart)` cap = max(today, weekStart−1) |
| `backend/internal/services/pick_service.go` | update `lastSeenDishDates` call site |
| `backend/internal/services/week_plan_recommendations.go` | `SaveWeekPlan` derives week from days; cache guard |
| `backend/internal/services/week_plan_preferences.go` | `ForWeek` variants |
| `backend/internal/handlers/plan.go` | `resolveWeekParam` + 4 handlers accept `?week=` |
| `frontend/src/api/index.ts` | `weekPlanApi` week param |
| `frontend/src/pages/WeekPlan.tsx` | tab, keyed queries, empty state, dirty guard |
| `CHANGELOG.md`, `docs/推荐算法说明.md` | v0.10.0 + freshness-cap note |

Tests: `backend/internal/services/week_plan_freshness_test.go`, `backend/internal/services/plan_service_test.go` (new cases follow existing `withPlanNow` + fixture patterns).

---

### Task 1: Week-key helpers + freshness cap parameterization

**Files:**
- Modify: `backend/internal/services/plan_service.go:44-52` (getCurrentWeekKey → helpers)
- Modify: `backend/internal/services/week_plan_freshness.go:18-61` (lastSeenDishDates signature + cap)
- Modify: `backend/internal/services/pick_service.go` (one `lastSeenDishDates(planNow())` call site)
- Modify: `backend/internal/services/plan_service.go:174` (buildWeekPlanGenContext call site — keep zero-arg signature in this task, pass current week explicitly)
- Test: `backend/internal/services/week_plan_freshness_test.go`, `backend/internal/services/plan_service_test.go`

Existing test conventions to reuse (read the test files first): `withPlanNow(t, time.Date(...))` pins `planNow`; `mustCreate(t, &models....)` inserts fixtures; each test sets up the shared sqlite test DB via the package's existing setup helper (see how `TestLastSeenDishDates...` tests in `week_plan_freshness_test.go` open the DB — copy that pattern exactly).

- [ ] **Step 1: Write failing tests for the new helpers**

In `plan_service_test.go` (place near other week-key/decay tests):

```go
func TestWeekKeyWithOffset(t *testing.T) {
	// Wednesday 2026-06-10 → this Monday 2026-06-08, next Monday 2026-06-15.
	withPlanNow(t, time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC))
	if got := weekKeyWithOffset(0); got != "2026-06-08" {
		t.Fatalf("offset 0: got %s", got)
	}
	if got := weekKeyWithOffset(1); got != "2026-06-15" {
		t.Fatalf("offset 1: got %s", got)
	}
}

func TestWeekKeyWithOffsetSundayEdge(t *testing.T) {
	// Sunday 2026-06-14 still belongs to the 06-08 week.
	withPlanNow(t, time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC))
	if got := weekKeyWithOffset(0); got != "2026-06-08" {
		t.Fatalf("offset 0 on Sunday: got %s", got)
	}
	if got := weekKeyWithOffset(1); got != "2026-06-15" {
		t.Fatalf("offset 1 on Sunday: got %s", got)
	}
}

func TestMondayOf(t *testing.T) {
	cases := map[string]string{
		"2026-06-15": "2026-06-15", // Monday → itself
		"2026-06-21": "2026-06-15", // Sunday → that week's Monday
		"2026-06-10": "2026-06-08",
	}
	for date, want := range cases {
		got, err := mondayOf(date)
		if err != nil || got != want {
			t.Fatalf("mondayOf(%s) = %s, %v; want %s", date, got, err, want)
		}
	}
	if _, err := mondayOf("not-a-date"); err == nil {
		t.Fatal("mondayOf should reject garbage")
	}
}
```

In `week_plan_freshness_test.go` (reuse the file's existing DB-setup + mustCreate pattern):

```go
func TestLastSeenCapExtendsToWeekStartMinusOne(t *testing.T) {
	// (existing DB setup pattern here)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) // Wednesday
	// Dish 1 planned for this Sunday — future relative to today.
	mustCreate(t, &models.DishRecommendation{DishID: 1, DishName: "周日菜", Source: "week_plan", MealType: "dinner", PlannedDate: "2026-06-14"})

	// Current week: cap = today → future tail invisible (unchanged behavior).
	if got := lastSeenDishDates(now, "2026-06-08"); got[1] != "" {
		t.Fatalf("current-week cap should exclude future rows, got %v", got)
	}
	// Next week: cap = 2026-06-14 (weekStart-1) → this week's tail counts.
	if got := lastSeenDishDates(now, "2026-06-15"); got[1] != "2026-06-14" {
		t.Fatalf("next-week cap should include this week's tail, got %v", got)
	}
}
```

Also update every existing `lastSeenDishDates(now)` call in tests to `lastSeenDishDates(now, <current week key for the pinned now>)` — compute the literal Monday from each test's pinned date (do NOT call getCurrentWeekKey() in tests that don't pin planNow).

- [ ] **Step 2: Run tests, verify the new ones fail to compile/fail**

Run: `cd backend && go test ./internal/services/ -run 'TestWeekKeyWithOffset|TestMondayOf|TestLastSeenCap' -v`
Expected: compile error (`weekKeyWithOffset` undefined) — that counts as the failing state.

- [ ] **Step 3: Implement the helpers**

`plan_service.go` — replace `getCurrentWeekKey` (lines 44-52) with:

```go
func getCurrentWeekKey() string {
	return weekKeyWithOffset(0)
}

// weekKeyWithOffset returns the Monday key of the week `weeks` after the
// current one (0 = this week, 1 = next week).
func weekKeyWithOffset(weeks int) string {
	now := planNow()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return now.AddDate(0, 0, 1-weekday+7*weeks).Format("2006-01-02")
}

// mondayOf returns the Monday key of the week containing date.
func mondayOf(date string) (string, error) {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "", err
	}
	weekday := int(parsed.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return parsed.AddDate(0, 0, 1-weekday).Format("2006-01-02"), nil
}

// CurrentWeekStart / NextWeekStart are the handler-facing week addresses.
func CurrentWeekStart() string { return weekKeyWithOffset(0) }
func NextWeekStart() string    { return weekKeyWithOffset(1) }
```

`week_plan_freshness.go` — change signature and cap (the doc comment must explain the cap rule):

```go
// lastSeenDishDates returns each dish's most recent appearance date
// (meal_records ∪ dish_recommendations) within the decay window.
// Recommendation rows are capped at max(today, weekStart-1): when generating
// the CURRENT week that is today (future rows are this very plan's tail,
// not something the family has eaten), but when generating a FUTURE week the
// current week's remaining days ARE prior context — without them, a dish
// planned for this Sunday would carry zero penalty next Monday. A week's own
// rows are never included when generating it (cap < its first day).
// now is injectable via planNow(); tests pin it with withPlanNow.
func lastSeenDishDates(now time.Time, weekStart string) map[uint]string {
	today := now.Format("2006-01-02")
	capDate := today
	if ws, err := time.Parse("2006-01-02", weekStart); err == nil {
		if dayBefore := ws.AddDate(0, 0, -1).Format("2006-01-02"); dayBefore > capDate {
			capDate = dayBefore
		}
	}
	since := now.AddDate(0, 0, -(staleRepeatWindowDays - 1)).Format("2006-01-02")
	// ... body unchanged except the planned query:
	//   Where("planned_date >= ? AND planned_date <= ?", since, capDate)
}
```

(MealRecord query stays `meal_date >= ?, since` — eaten records cannot be in the future.)

Call sites:
- `plan_service.go:174`: `lastSeen: lastSeenDishDates(planNow(), getCurrentWeekKey()),`
- `pick_service.go` (grep `lastSeenDishDates`): `lastSeenDishDates(planNow(), getCurrentWeekKey())` — cap stays today for the tomorrow-pick (current Monday−1 is always ≤ today).

- [ ] **Step 4: Run the full service test suite**

Run: `cd backend && go test ./internal/services/ && go vet ./...`
Expected: PASS (all existing freshness/decay/pick tests still green with updated call sites).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/services/
git commit -m "feat: week-key helpers and week-aware freshness cap"
```

### Task 2: Service-layer week parameterization

**Files:**
- Modify: `backend/internal/services/plan_service.go` (`buildWeekPlanGenContext`, `GenerateWeekPlan` 248-306, `GetCachedWeekPlan` 64-91 untouched + new `GetWeekPlanForWeek`, `RegenerateWeekPlan` 93-97, `InvalidateWeekPlanCache` 102-110, `RegenerateWeekPlanDay` 310-377)
- Modify: `backend/internal/services/week_plan_recommendations.go:19-45` (`SaveWeekPlan`)
- Modify: `backend/internal/services/week_plan_preferences.go:61-113` (`ForWeek` variants)
- Test: `backend/internal/services/plan_service_test.go`

**Scene:** Task 1 added `weekKeyWithOffset`/`mondayOf` and the week-aware freshness cap. This task makes every read/write/generate path addressable by week-start key, keeping the existing zero-arg functions as current-week wrappers so all existing callers and tests stay green.

- [ ] **Step 1: Write failing tests**

Add to `plan_service_test.go`, following the file's existing patterns (`withPlanNow`, fixture dish creation as done by the decay tests around line 1828, `models.WeekPlanRecord` lookups):

```go
func TestGenerateWeekPlanForWeekCoversRequestedWeek(t *testing.T) {
	// (existing DB setup + a few enabled dishes)
	withPlanNow(t, time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)) // Wednesday
	plan, err := GenerateWeekPlanForWeek("2026-06-15")
	if err != nil || len(plan.Days) != 7 {
		t.Fatalf("want 7 days, got %d (err %v)", len(plan.Days), err)
	}
	if plan.Days[0].Date != "2026-06-15" || plan.Days[6].Date != "2026-06-21" {
		t.Fatalf("wrong span: %s..%s", plan.Days[0].Date, plan.Days[6].Date)
	}
	if plan.Days[0].DayName != "周一" || plan.Days[6].DayName != "周日" {
		t.Fatalf("wrong day names: %s..%s", plan.Days[0].DayName, plan.Days[6].DayName)
	}
}

func TestNextWeekGenerationPenalizesThisWeekTail(t *testing.T) {
	// Two interchangeable dishes; dish A is planned for THIS Sunday. Generating
	// next week must prefer B on Monday (A carries the d=1 decay penalty via
	// the weekStart-1 cap from Task 1). Mirror the fixture style of the
	// existing decay tests (same category/traits so only the penalty differs,
	// quota forced to 1 lunch / 1 dinner via prefs or settings as those tests do).
	withPlanNow(t, time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)) // Saturday
	// fixtures: dish A, dish B enabled; recommendation row: A @ 2026-06-14 (Sunday)
	// generate: plan, _ := GenerateWeekPlanForWeek("2026-06-15")
	// assert: plan.Days[0] (Monday 06-15) does not contain dish A
}

func TestSaveWeekPlanRoutesToWeekOfDays(t *testing.T) {
	withPlanNow(t, time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC))
	plan := &WeekPlan{Days: []WeekDayPlan{{Date: "2026-06-15", DayName: "周一"}}}
	if err := SaveWeekPlan(plan); err != nil {
		t.Fatal(err)
	}
	var rec models.WeekPlanRecord
	if err := database.DB.Where("week_start = ?", "2026-06-15").First(&rec).Error; err != nil || rec.PlanJSON == "" {
		t.Fatalf("next-week row missing: %v", err)
	}
	planMu.RLock()
	key := cachedWeekKey
	planMu.RUnlock()
	if key == "2026-06-15" {
		t.Fatal("saving next week must not hijack the current-week cache")
	}
}

func TestGetWeekPlanForWeekDoesNotAutoGenerate(t *testing.T) {
	withPlanNow(t, time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC))
	plan := GetWeekPlanForWeek("2026-06-15")
	if len(plan.Days) != 0 {
		t.Fatalf("next week must stay empty until explicitly generated, got %d days", len(plan.Days))
	}
	var count int64
	database.DB.Model(&models.WeekPlanRecord{}).Where("week_start = ?", "2026-06-15").Count(&count)
	if count != 0 {
		t.Fatal("read must not create a row")
	}
}

func TestInvalidateWeekPlanCacheClearsFutureWeeks(t *testing.T) {
	withPlanNow(t, time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC))
	// seed three rows with non-empty plan_json: 2026-06-01 (past),
	// 2026-06-08 (current), 2026-06-15 (next) — use upsertWeekPlanRecordPlan
	// or direct Create, matching existing test style.
	InvalidateWeekPlanCache()
	// assert: past row plan_json untouched; current + next rows plan_json == ""
}

func TestWeekPlanPreferencesPerWeek(t *testing.T) {
	withPlanNow(t, time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC))
	prefs := GetWeekPlanPreferencesForWeek("2026-06-15")
	prefs.WeekWant = []string{"饺子"}
	if err := SaveWeekPlanPreferencesForWeek(prefs, "2026-06-15"); err != nil {
		t.Fatal(err)
	}
	if got := GetWeekPlanPreferencesForWeek("2026-06-15"); len(got.WeekWant) != 1 || got.WeekWant[0] != "饺子" {
		t.Fatalf("next-week one-off lost: %v", got.WeekWant)
	}
	if got := GetWeekPlanPreferencesForWeek("2026-06-08"); len(got.WeekWant) != 0 {
		t.Fatalf("current week must not see next week's one-off: %v", got.WeekWant)
	}
}

func TestRegenerateWeekPlanDayNextWeekRequiresPlan(t *testing.T) {
	withPlanNow(t, time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC))
	// no next-week row yet:
	if _, err := RegenerateWeekPlanDay("2026-06-16"); err == nil {
		t.Fatal("day-regen on an ungenerated week must error")
	}
	// after generating next week it works:
	// RegenerateWeekPlanForWeek("2026-06-15") (needs dish fixtures)
	// plan, err := RegenerateWeekPlanDay("2026-06-16") → err == nil, 7 days, Days[1].Date == "2026-06-16"
}
```

Note for the WeekWant field name: check `weekPlanOneOffPrefs` / `WeekPlanPreferences` in `week_plan_preferences.go` for the exact field (it is the "本周想吃" list; adjust the test to the real field name and type if it differs).

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./internal/services/ -run 'ForWeek|RoutesToWeek|ClearsFutureWeeks|PerWeek|NextWeekRequiresPlan|PenalizesThisWeekTail' -v`
Expected: compile errors (`GenerateWeekPlanForWeek` undefined etc.).

- [ ] **Step 3: Implement**

`week_plan_preferences.go` — split into ForWeek + wrappers (resident part stays shared):

```go
func GetWeekPlanPreferences() WeekPlanPreferences {
	return GetWeekPlanPreferencesForWeek(getCurrentWeekKey())
}

// GetWeekPlanPreferencesForWeek: resident prefs come from the shared Setting
// row; one-off parts (week want / day overrides) live on the addressed week's
// row and reset when that week rolls out of scope.
func GetWeekPlanPreferencesForWeek(weekStart string) WeekPlanPreferences {
	// identical body to the old GetWeekPlanPreferences, except:
	//   loadWeekPlanRecord(weekStart)
}

func SaveWeekPlanPreferences(prefs WeekPlanPreferences) error {
	return SaveWeekPlanPreferencesForWeek(prefs, getCurrentWeekKey())
}

func SaveWeekPlanPreferencesForWeek(prefs WeekPlanPreferences, weekStart string) error {
	// identical body, except: upsertWeekPlanRecordPrefs(weekStart, string(oneOffData))
	// The "deliberately NOT invalidating" comment and behavior stay as-is.
}
```

`plan_service.go`:

```go
// buildWeekPlanGenContext gets the week parameter (one-off prefs and the
// freshness cap are per-week):
func buildWeekPlanGenContext(weekStart string) *weekPlanGenContext {
	// body unchanged except:
	//   prefs := GetWeekPlanPreferencesForWeek(weekStart)
	//   lastSeen: lastSeenDishDates(planNow(), weekStart),
}

func GenerateWeekPlan() (*WeekPlan, error) {
	return GenerateWeekPlanForWeek(getCurrentWeekKey())
}

func GenerateWeekPlanForWeek(weekStart string) (*WeekPlan, error) {
	monday, err := time.Parse("2006-01-02", weekStart)
	if err != nil {
		return &WeekPlan{}, fmt.Errorf("无效的周起始日期")
	}
	ctx := buildWeekPlanGenContext(weekStart)
	if ctx == nil {
		return &WeekPlan{}, nil
	}
	today := todayKey()
	// Freeze guard unchanged in spirit; the stored snapshot now comes from the
	// addressed week's row. For a future week no day is < today, so nothing
	// freezes — but manual dishes on its days still survive via keepLunch/keepDinner.
	var storedDays map[string]WeekDayPlan
	if rec, ok := loadWeekPlanRecord(weekStart); ok && rec.PlanJSON != "" {
		// ... unchanged unmarshal into storedDays ...
	}
	// ... 7-day loop unchanged (date := monday.AddDate(0, 0, i)) ...
}
```

(The old lines 254-259 deriving monday from `planNow()` are deleted — monday now comes from the parsed weekStart.)

```go
// GetWeekPlanForWeek reads the addressed week. The current week keeps its
// generate-on-first-read behavior (GetCachedWeekPlan); other weeks are
// generated only on explicit request, never as a read side effect.
func GetWeekPlanForWeek(weekStart string) *WeekPlan {
	if weekStart == getCurrentWeekKey() {
		return GetCachedWeekPlan()
	}
	if rec, ok := loadWeekPlanRecord(weekStart); ok && rec.PlanJSON != "" {
		var plan WeekPlan
		if json.Unmarshal([]byte(rec.PlanJSON), &plan) == nil && len(plan.Days) > 0 {
			return &plan
		}
		fmt.Printf("菜单存档解析失败（week_start=%s）\n", weekStart)
	}
	return &WeekPlan{Days: []WeekDayPlan{}}
}

func RegenerateWeekPlan() *WeekPlan {
	return RegenerateWeekPlanForWeek(getCurrentWeekKey())
}

func RegenerateWeekPlanForWeek(weekStart string) *WeekPlan {
	plan, _ := GenerateWeekPlanForWeek(weekStart)
	// An empty generation (no enabled dishes) is not a snapshot worth saving —
	// and SaveWeekPlan's days[0]-based routing could not address the right
	// week for it anyway.
	if len(plan.Days) == 0 {
		return plan
	}
	_ = SaveWeekPlan(plan)
	return plan
}
```

`InvalidateWeekPlanCache` — widen the WHERE and the doc comment:

```go
// InvalidateWeekPlanCache clears the current AND any future week's generated
// plan so the next read (current) or the next explicit generate (future)
// rebuilds it — a pre-generated next week goes stale on the same triggers
// (dish disabled, rules changed, ...). Rows stay (one-off prefs live on
// them); past weeks are never touched.
func InvalidateWeekPlanCache() {
	database.DB.Model(&models.WeekPlanRecord{}).
		Where("user_id = ? AND week_start >= ?", CurrentUserID, getCurrentWeekKey()).
		Update("plan_json", "")
	// in-memory part unchanged
}
```

`RegenerateWeekPlanDay` — derive the week from the date (lines 310-318):

```go
	date = strings.TrimSpace(date)
	if date < todayKey() {
		return nil, fmt.Errorf("过去的天不能重新生成")
	}
	weekKey, err := mondayOf(date)
	if err != nil {
		return nil, fmt.Errorf("日期不在本周菜单内")
	}
	current := GetWeekPlanForWeek(weekKey)
	if current == nil || len(current.Days) == 0 {
		return nil, fmt.Errorf("该周菜单尚未生成，请先生成")
	}
	// ... dayIndex lookup unchanged ...
	ctx := buildWeekPlanGenContext(weekKey)
```

Keep the existing error text `日期不在本周菜单内` for the dayIndex-miss and parse branches (existing tests may assert it); the new text only covers the missing-week case. Update the function doc comment ("current week" → "the week containing date").

`week_plan_recommendations.go` `SaveWeekPlan` (lines 19-45):

```go
	normalizeWeekPlan(plan)
	// The plan addresses its week by content: a snapshot of next week's days
	// lands on next week's row. Empty/garbled days fall back to the current week.
	weekKey := getCurrentWeekKey()
	if len(plan.Days) > 0 {
		if derived, err := mondayOf(plan.Days[0].Date); err == nil {
			weekKey = derived
		}
	}
	restorePastDaysFromStored(plan, weekKey)
	// ... transaction unchanged ...
	// The in-memory cache only ever mirrors the current week:
	if weekKey == getCurrentWeekKey() {
		planMu.Lock()
		cachedPlan = plan
		cachedWeekKey = weekKey
		planMu.Unlock()
	}
	return nil
```

Update the SaveWeekPlan doc comment to mention week routing. `restorePastDaysFromStored` and `replaceWeekPlanRecommendations` are untouched (today-guards are week-agnostic; the delete is scoped to the plan's own dates).

- [ ] **Step 4: Run the full backend suite**

Run: `cd backend && go test ./... && go vet ./...`
Expected: PASS — all new tests green, all existing generation/freeze/decay/manual-dish tests green via the wrappers.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/services/
git commit -m "feat: address week plans by week-start key across the service layer"
```

### Task 3: HTTP layer — `?week=` parameter

**Files:**
- Modify: `backend/internal/handlers/plan.go:14-26, 73-88`
- Test: handler-level test only if `backend/internal/handlers/` already has a `*_test.go` precedent with a gin/httptest pattern — check first. If none exists, do NOT invent a new harness; param validation is covered by the curl smoke in Task 5.

**Scene:** Routes don't change (query string needs no registration). Four handlers become week-addressable; PUT /week-plan and POST /week-plan/regenerate-day stay as-is (they derive the week from their payload dates via Task 2).

- [ ] **Step 1: Implement `resolveWeekParam` + handler changes**

Add to `handlers/plan.go`:

```go
// resolveWeekParam maps ?week= to a week-start key. Absent/current → this
// week, next → next week; anything else is a client error (false return,
// response already written).
func resolveWeekParam(c *gin.Context) (string, bool) {
	switch c.Query("week") {
	case "", "current":
		return services.CurrentWeekStart(), true
	case "next":
		return services.NextWeekStart(), true
	default:
		utils.BadRequest(c, "week 参数无效（可选 current/next）")
		return "", false
	}
}
```

Rewire the four handlers:

```go
func GetWeekPlan(c *gin.Context) {
	weekStart, ok := resolveWeekParam(c)
	if !ok {
		return
	}
	plan := services.GetWeekPlanForWeek(weekStart)
	if plan != nil && len(plan.Days) > 0 {
		services.RecordUniqueAchievementEvent("week_plan", plan.Days[0].Date)
	}
	utils.Success(c, plan)
}

func RegenerateWeekPlanHandler(c *gin.Context) {
	weekStart, ok := resolveWeekParam(c)
	if !ok {
		return
	}
	plan := services.RegenerateWeekPlanForWeek(weekStart)
	services.RecordAchievementEvent("week_plan", "")
	utils.Success(c, plan)
}

func GetWeekPlanPreferencesHandler(c *gin.Context) {
	weekStart, ok := resolveWeekParam(c)
	if !ok {
		return
	}
	utils.Success(c, services.GetWeekPlanPreferencesForWeek(weekStart))
}

func UpdateWeekPlanPreferencesHandler(c *gin.Context) {
	weekStart, ok := resolveWeekParam(c)
	if !ok {
		return
	}
	var prefs services.WeekPlanPreferences
	if err := c.ShouldBindJSON(&prefs); err != nil {
		utils.BadRequest(c, "参数无效")
		return
	}
	if err := services.SaveWeekPlanPreferencesForWeek(prefs, weekStart); err != nil {
		utils.InternalError(c, "保存设置失败")
		return
	}
	utils.Success(c, services.GetWeekPlanPreferencesForWeek(weekStart))
}
```

- [ ] **Step 2: Verify**

Run: `cd backend && go build ./... && go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/plan.go
git commit -m "feat: week query param on week-plan endpoints"
```

---

### Task 4: Frontend — 本周/下周 tab, keyed queries, next-week empty state

**Files:**
- Modify: `frontend/src/api/index.ts:88-99` (`weekPlanApi`)
- Modify: `frontend/src/pages/WeekPlan.tsx` (state ~658, queries 667-674, mutations 709-780, header/tab UI, empty state)

**Scene:** Single page, a `viewWeek` dimension threaded through queries and mutations. The page's draft/dirty state machine (draftPlan/dirtyPlan, draftPrefs/dirtyPrefs) is reused untouched; the only new rule is that switching weeks with unsaved changes is blocked. Read the full component before editing — the line numbers are anchors, not exact targets.

- [ ] **Step 1: API layer**

```ts
export type WeekPlanWeek = "current" | "next"
const weekQuery = (week?: WeekPlanWeek) => (week === "next" ? "?week=next" : "")

export const weekPlanApi = {
  get: (week?: WeekPlanWeek) => api<WeekPlan>("GET", `/week-plan${weekQuery(week)}`),
  save: (data: WeekPlan) => api<WeekPlan>("PUT", "/week-plan", data),
  preferences: (week?: WeekPlanWeek) =>
    api<WeekPlanPreferences>("GET", `/week-plan/preferences${weekQuery(week)}`),
  updatePreferences: (data: WeekPlanPreferences, week?: WeekPlanWeek) =>
    api<WeekPlanPreferences>("PUT", `/week-plan/preferences${weekQuery(week)}`, data),
  rules: ...unchanged,
  regenerate: (week?: WeekPlanWeek) =>
    api<WeekPlan>("POST", `/week-plan/regenerate${weekQuery(week)}`),
  regenerateDay: ...unchanged,
  history: ...unchanged,
}
```

- [ ] **Step 2: WeekPlan.tsx — week dimension**

1. State near the other useState calls: `const [viewWeek, setViewWeek] = useState<WeekPlanWeek>("current")`.
2. Queries:
   - plan: `queryKey: ["week-plan", viewWeek]`, `queryFn: () => weekPlanApi.get(viewWeek)`
   - prefs: `queryKey: ["week-plan", "preferences", viewWeek]`, `queryFn: () => weekPlanApi.preferences(viewWeek)`
3. Mutations: regenerate → `weekPlanApi.regenerate(viewWeek)`; apply-prefs → `weekPlanApi.updatePreferences(draftPrefs, viewWeek)` (and its follow-up regenerate uses viewWeek); save & per-day regenerate unchanged (payload-driven).
4. **Audit every `setQueryData`/`invalidateQueries` touching `["week-plan"...]` in this file AND grep the rest of `frontend/src` for the same keys.** Prefix `invalidateQueries({ queryKey: ["week-plan"] })` calls elsewhere still match keyed queries and stay as-is; exact-key `setQueryData(["week-plan"], ...)` calls inside the page must become `["week-plan", viewWeek]` (and the prefs key gains viewWeek likewise).
5. Tab switcher in the header area (above or beside the existing 本周菜单 title/actions), matching the page's existing button styling:

```tsx
const switchWeek = (week: WeekPlanWeek) => {
  if (week === viewWeek) return
  if (dirtyPlan || dirtyPrefs) {
    toast("请先保存或放弃当前修改，再切换周")
    return
  }
  setViewWeek(week)
}
```

(Use the page's existing toast/notification mechanism — grep how other warnings are surfaced in this file and reuse it.)

6. Next-week empty state: when `viewWeek === "next" && !planLoading && (draftPlan?.days?.length ?? 0) === 0`, render instead of the day grid a centered card: 标题「下周菜单还没生成」, 说明「生成后即可提前调整、周末照着买菜」, button 「生成下周菜单」 firing the regenerate mutation. The regular header regenerate button label becomes 「重新生成下周」 when `viewWeek === "next"` (current week label unchanged).
7. The draft-reset effects (677/688) already key off serverPlan/serverPrefs identity — switching weeks refetches and resets drafts because the dirty guard guarantees `!dirtyPlan && !dirtyPrefs` at switch time. No change needed; verify by reading.

- [ ] **Step 3: Build**

Run: `cd frontend && npm run build`
Expected: clean build, no TS errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/api/index.ts frontend/src/pages/WeekPlan.tsx
git commit -m "feat: next-week tab with explicit generation on the week plan page"
```

---

### Task 5: Docs + live verification

**Files:**
- Modify: `CHANGELOG.md` (new v0.10.0 section at top, mirroring v0.9.0's structure)
- Modify: `docs/推荐算法说明.md` (freshness/decay section: one short paragraph)

- [ ] **Step 1: CHANGELOG v0.10.0**

新增（措辞贴合既有条目风格，中文）：下周菜单提前生成——周菜单页新增 本周/下周 切换；下周不会自动生成，空态一键「生成下周菜单」；生成下周时会把本周剩余几天计入"最近吃过"（周日的菜下周一不会立刻重复）；下周可手动增删、单天重抽、单独设置当周想吃；菜品/规则变更会让已生成的下周菜单一并失效待重新生成。

- [ ] **Step 2: 推荐算法说明 — freshness cap note**

在新鲜度衰减小节末尾补一段（中文，口吻与全文一致）：生成「下周」菜单时，"最近一次吃过"的参照上限从今天延伸到下周一的前一天——也就是本周剩下几天已计划的菜也算"刚出现过"，避免本周日的菜下周一原样再来；生成本周时行为不变。

- [ ] **Step 3: Full verification + restart + smoke**

```bash
cd backend && go test ./... && go vet ./... && go build ./...
cd ../frontend && npm run build
```

Restart backend (kill by port, not by name):

```bash
lsof -ti tcp:8080 | xargs kill -9 2>/dev/null
cd backend && nohup go run ./cmd/server > /tmp/ninimenu-server.log 2>&1 &
sleep 3
```

Smoke (token header required):

```bash
# next week before generation → days: []
curl -s -H 'X-App-Token: nini123' 'http://localhost:8080/api/week-plan?week=next'
# bad param → 400
curl -s -o /dev/null -w '%{http_code}' -H 'X-App-Token: nini123' 'http://localhost:8080/api/week-plan?week=foo'
# generate next week → 7 days starting next Monday
curl -s -X POST -H 'X-App-Token: nini123' 'http://localhost:8080/api/week-plan/regenerate?week=next'
# read back persisted; current week unchanged by the above
curl -s -H 'X-App-Token: nini123' 'http://localhost:8080/api/week-plan?week=next'
curl -s -H 'X-App-Token: nini123' 'http://localhost:8080/api/week-plan'
```

Expected: empty days → 400 → 7 days (next Monday..Sunday) → same 7 days persisted → current week's dates unchanged.

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md docs/推荐算法说明.md
git commit -m "docs: changelog and algorithm note for next-week generation"
```

---

## Self-review checklist (done at plan time)

- Spec coverage: all 11 locked decisions map to Tasks 1-5 (1→T1, 2→T2/T3, 3→T1, 4→doc note T5, 5→T2/T3, 6→T2, 7→T2, 8→T2, 9→T2, 10→T4, 11→explicitly excluded).
- Type/name consistency: `weekKeyWithOffset`/`mondayOf`/`CurrentWeekStart`/`NextWeekStart` (T1) are exactly what T2/T3 call; `GenerateWeekPlanForWeek`/`GetWeekPlanForWeek`/`RegenerateWeekPlanForWeek`/`*PreferencesForWeek` (T2) are exactly what T3 calls; `WeekPlanWeek` (T4 api) is what WeekPlan.tsx imports.
- Known judgment calls delegated to implementers with explicit instructions: exact one-off prefs field name (T2 Step 1 note), handler-test precedent check (T3), toast mechanism + setQueryData audit (T4).


