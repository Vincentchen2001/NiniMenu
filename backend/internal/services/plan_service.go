package services

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

type WeekDayPlan struct {
	Date    string        `json:"date"`
	DayName string        `json:"day_name"`
	Lunch   []models.Dish `json:"lunch"`
	Dinner  []models.Dish `json:"dinner"`
	// Manual*IDs mark dishes the user added by hand (split per meal so a dish
	// added manually to lunch is not mis-marked when the system also picks it
	// for dinner). They survive regeneration: see generateDay's keep params.
	ManualLunchIDs  []uint `json:"manual_lunch_ids,omitempty"`
	ManualDinnerIDs []uint `json:"manual_dinner_ids,omitempty"`
}

type WeekPlan struct {
	Days     []WeekDayPlan `json:"days"`
	Warnings []string      `json:"warnings"`
}

var (
	cachedPlan    *WeekPlan
	cachedWeekKey string
	planMu        sync.RWMutex
)

func getCurrentWeekKey() string {
	now := planNow()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := now.AddDate(0, 0, 1-weekday)
	return monday.Format("2006-01-02")
}

func getSettingInt(key string, defaultVal int) int {
	var setting models.Setting
	if err := database.DB.Where("`key` = ?", key).First(&setting).Error; err == nil {
		if n, err := strconv.Atoi(setting.Value); err == nil && n >= 0 {
			return n
		}
	}
	return defaultVal
}

func GetCachedWeekPlan() *WeekPlan {
	weekKey := getCurrentWeekKey()

	planMu.RLock()
	if cachedPlan != nil && cachedWeekKey == weekKey {
		p := cachedPlan
		planMu.RUnlock()
		return p
	}
	planMu.RUnlock()

	if rec, ok := loadWeekPlanRecord(weekKey); ok && rec.PlanJSON != "" {
		var plan WeekPlan
		if json.Unmarshal([]byte(rec.PlanJSON), &plan) == nil && len(plan.Days) > 0 {
			planMu.Lock()
			cachedPlan = &plan
			cachedWeekKey = weekKey
			planMu.Unlock()
			return &plan
		}
		// 解析失败按"未生成"处理，重新生成覆盖。
		fmt.Printf("本周菜单存档解析失败，将重新生成（week_start=%s）\n", weekKey)
	}

	plan, _ := GenerateWeekPlan()
	_ = SaveWeekPlan(plan)
	return plan
}

func RegenerateWeekPlan() *WeekPlan {
	plan, _ := GenerateWeekPlan()
	_ = SaveWeekPlan(plan)
	return plan
}

// InvalidateWeekPlanCache clears the current week's generated plan so the
// next read regenerates it. The row itself stays (one-off prefs live on it),
// and past weeks are never touched.
func InvalidateWeekPlanCache() {
	database.DB.Model(&models.WeekPlanRecord{}).
		Where("user_id = ? AND week_start = ?", CurrentUserID, getCurrentWeekKey()).
		Update("plan_json", "")
	planMu.Lock()
	cachedPlan = nil
	cachedWeekKey = ""
	planMu.Unlock()
}

var weekPlanDayNames = []string{"周一", "周二", "周三", "周四", "周五", "周六", "周日"}

// weekPlanGenContext bundles the dish pools, preferences and compiled rules
// shared by full-week generation and single-day regeneration.
type weekPlanGenContext struct {
	lunchPool         []models.Dish
	dinnerPool        []models.Dish
	prefs             WeekPlanPreferences
	rules             []compiledMenuRule
	recent            map[uint]bool
	categoryFavorites map[string]int
	warnings          []string
	r                 *rand.Rand
}

// buildWeekPlanGenContext returns nil when no enabled dish survives the
// blocked-ingredient filter.
func buildWeekPlanGenContext() *weekPlanGenContext {
	var dishes []models.Dish
	database.DB.Where("enabled = ?", true).Find(&dishes)
	dishes = FilterBlockedDishes(dishes)
	if len(dishes) == 0 {
		return nil
	}
	for i := range dishes {
		dishes[i] = ensureDishTraits(dishes[i])
	}

	prefs := GetWeekPlanPreferences()
	rules, err := ListMenuRules()
	var warnings []string
	if err != nil {
		warnings = append(warnings, "推荐规则读取失败，已使用基础推荐逻辑")
	}
	compiledRules, ruleWarnings := compileMenuRules(rules)
	warnings = append(warnings, ruleWarnings...)

	var lunchPool, dinnerPool []models.Dish
	for _, d := range dishes {
		tags := parseTags(d.Tags)
		mt := d.MealType
		if mt == "all" || mt == "lunch" || containsTag(tags, "午餐") || containsTag(tags, "lunch") {
			lunchPool = append(lunchPool, d)
		}
		if mt == "all" || mt == "dinner" || containsTag(tags, "晚餐") || containsTag(tags, "dinner") {
			dinnerPool = append(dinnerPool, d)
		}
	}
	if len(lunchPool) == 0 {
		lunchPool = dishes
	}
	if len(dinnerPool) == 0 {
		dinnerPool = dishes
	}

	return &weekPlanGenContext{
		lunchPool:         lunchPool,
		dinnerPool:        dinnerPool,
		prefs:             prefs,
		rules:             compiledRules,
		recent:            recentDishIDMap(RecommendationCooldownDays()),
		categoryFavorites: favoriteCategoryCounts(),
		warnings:          warnings,
		r:                 rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// generateDay fills one weekday (0=Monday). globalUsed and weekPicked carry
// cross-day dedup state and are updated with the picked dishes. keepLunch /
// keepDinner are manually-added dishes that survive regeneration: they take
// their quota slots up front, join every dedup set and rule context, and the
// picker only fills what is left.
func (ctx *weekPlanGenContext) generateDay(dayIndex int, date time.Time, globalUsed map[uint]bool, weekPicked *[]models.Dish, prevSoups []models.Dish, keepLunch []models.Dish, keepDinner []models.Dish) WeekDayPlan {
	periodPrefs := ctx.prefs.Weekday
	if dayIndex >= 5 {
		periodPrefs = ctx.prefs.Weekend
	}
	dayCtx, lunchQuota, dinnerQuota := resolveWeekPlanDay(ctx.prefs, periodPrefs, dayIndex)
	dayCtx.categoryFavorites = ctx.categoryFavorites
	dayCtx.prevSoups = prevSoups
	dayPlan := WeekDayPlan{
		Date:   date.Format("2006-01-02"),
		Lunch:  []models.Dish{},
		Dinner: []models.Dish{},
	}
	if dayIndex >= 0 && dayIndex < len(weekPlanDayNames) {
		dayPlan.DayName = weekPlanDayNames[dayIndex]
	}
	for _, dish := range keepLunch {
		dayPlan.ManualLunchIDs = append(dayPlan.ManualLunchIDs, dish.ID)
	}
	for _, dish := range keepDinner {
		dayPlan.ManualDinnerIDs = append(dayPlan.ManualDinnerIDs, dish.ID)
	}

	usedThisDay := make(map[uint]bool)
	var dayPicked []models.Dish
	for _, dish := range appendDishSlices(keepLunch, keepDinner) {
		globalUsed[dish.ID] = true
		usedThisDay[dish.ID] = true
		dayPicked = append(dayPicked, dish)
		*weekPicked = append(*weekPicked, dish)
	}

	dayPlan.Lunch = pickQuotaDishes(ctx.lunchPool, reduceQuotaForKept(lunchQuota, keepLunch), dayCtx, "lunch", dayPlan.DayName, globalUsed, usedThisDay, ctx.recent, dayPicked, *weekPicked, ctx.rules, &ctx.warnings, ctx.r, keepLunch)
	for _, dish := range dayPlan.Lunch {
		if !containsDishID(keepLunch, dish.ID) {
			dayPicked = append(dayPicked, dish)
			*weekPicked = append(*weekPicked, dish)
		}
	}
	dayPlan.Dinner = pickQuotaDishes(ctx.dinnerPool, reduceQuotaForKept(dinnerQuota, keepDinner), dayCtx, "dinner", dayPlan.DayName, globalUsed, usedThisDay, ctx.recent, dayPicked, *weekPicked, ctx.rules, &ctx.warnings, ctx.r, keepDinner)
	for _, dish := range dayPlan.Dinner {
		if !containsDishID(keepDinner, dish.ID) {
			dayPicked = append(dayPicked, dish)
			*weekPicked = append(*weekPicked, dish)
		}
	}

	return dayPlan
}

// containsDishID reports whether dishes already contains id (used to avoid
// double-counting keep dishes that were pre-registered in the dedup sets).
func containsDishID(dishes []models.Dish, id uint) bool {
	for _, dish := range dishes {
		if dish.ID == id {
			return true
		}
	}
	return false
}

func GenerateWeekPlan() (*WeekPlan, error) {
	ctx := buildWeekPlanGenContext()
	if ctx == nil {
		return &WeekPlan{}, nil
	}

	now := planNow()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := now.AddDate(0, 0, 1-weekday)
	today := todayKey()

	// Freeze guard: days already gone are copied verbatim from the stored
	// snapshot (read-only history), and manual dishes on remaining days are
	// handed to generateDay so they survive the regenerate.
	var storedDays map[string]WeekDayPlan
	if rec, ok := loadWeekPlanRecord(getCurrentWeekKey()); ok && rec.PlanJSON != "" {
		var stored WeekPlan
		if json.Unmarshal([]byte(rec.PlanJSON), &stored) == nil {
			storedDays = make(map[string]WeekDayPlan, len(stored.Days))
			for _, day := range stored.Days {
				storedDays[day.Date] = day
			}
		}
	}

	var days []WeekDayPlan
	globalUsed := make(map[uint]bool)
	var weekPicked []models.Dish
	var prevSoups []models.Dish
	for i := 0; i < 7; i++ {
		date := monday.AddDate(0, 0, i)
		dateKey := date.Format("2006-01-02")
		storedDay, hasStored := storedDays[dateKey]

		if dateKey < today && hasStored {
			for _, dish := range appendDishSlices(storedDay.Lunch, storedDay.Dinner) {
				globalUsed[dish.ID] = true
				weekPicked = append(weekPicked, dish)
			}
			days = append(days, storedDay)
			prevSoups = soupsFromDishes(appendDishSlices(storedDay.Lunch, storedDay.Dinner))
			continue
		}

		var keepLunch, keepDinner []models.Dish
		if hasStored {
			keepLunch = manualDishesForMeal(storedDay.Lunch, storedDay.ManualLunchIDs)
			keepDinner = manualDishesForMeal(storedDay.Dinner, storedDay.ManualDinnerIDs)
		}
		day := ctx.generateDay(i, date, globalUsed, &weekPicked, prevSoups, keepLunch, keepDinner)
		days = append(days, day)
		prevSoups = soupsFromDishes(appendDishSlices(day.Lunch, day.Dinner))
	}

	return &WeekPlan{Days: days, Warnings: uniqueWarnings(ctx.warnings)}, nil
}

// RegenerateWeekPlanDay replaces a single day of the current week plan,
// keeping every other day untouched and avoiding their dishes.
func RegenerateWeekPlanDay(date string) (*WeekPlan, error) {
	date = strings.TrimSpace(date)
	if date < todayKey() {
		return nil, fmt.Errorf("过去的天不能重新生成")
	}
	current := GetCachedWeekPlan()
	if current == nil || len(current.Days) == 0 {
		return nil, fmt.Errorf("日期不在本周菜单内")
	}
	dayIndex := -1
	for i, day := range current.Days {
		if day.Date == date {
			dayIndex = i
			break
		}
	}
	if dayIndex < 0 {
		return nil, fmt.Errorf("日期不在本周菜单内")
	}
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("日期不在本周菜单内")
	}

	ctx := buildWeekPlanGenContext()
	if ctx == nil {
		return nil, fmt.Errorf("没有可用的菜品，请先在菜品库中添加")
	}

	// Work on a fresh Days slice so the cached plan pointer is never mutated.
	newPlan := &WeekPlan{
		Days:     append([]WeekDayPlan(nil), current.Days...),
		Warnings: current.Warnings,
	}

	globalUsed := make(map[uint]bool)
	var weekPicked []models.Dish
	for i, day := range newPlan.Days {
		if i == dayIndex {
			continue
		}
		for _, dish := range appendDishSlices(day.Lunch, day.Dinner) {
			globalUsed[dish.ID] = true
			weekPicked = append(weekPicked, dish)
		}
	}

	targetDay := current.Days[dayIndex]
	keepLunch := manualDishesForMeal(targetDay.Lunch, targetDay.ManualLunchIDs)
	keepDinner := manualDishesForMeal(targetDay.Dinner, targetDay.ManualDinnerIDs)
	newPlan.Days[dayIndex] = ctx.generateDay(dayIndex, parsedDate, globalUsed, &weekPicked, adjacentPlannedSoups(newPlan.Days, dayIndex), keepLunch, keepDinner)
	// Retire the target day's old warnings — they describe picks that no
	// longer exist; cross-day and rule-compile warnings stay.
	dayName := current.Days[dayIndex].DayName
	keptWarnings := make([]string, 0, len(current.Warnings))
	for _, warning := range current.Warnings {
		if dayName != "" && strings.HasPrefix(warning, dayName) {
			continue
		}
		keptWarnings = append(keptWarnings, warning)
	}
	newPlan.Warnings = uniqueWarnings(append(keptWarnings, ctx.warnings...))

	if err := SaveWeekPlan(newPlan); err != nil {
		return nil, err
	}
	return newPlan, nil
}

// soupsFromDishes filters the dishes that occupy soup slots, using the same
// dual test (inferred role OR name/category) slot assignment uses.
func soupsFromDishes(dishes []models.Dish) []models.Dish {
	var result []models.Dish
	for _, dish := range dishes {
		if dish.DishRole == "soup" || isSoupDish(dish) {
			result = append(result, dish)
		}
	}
	return result
}

// manualDishesForMeal returns the dishes in the slot that the user added by
// hand (ids listed in manualIDs). Stale ids — manual dishes since removed
// from the slot — drop out naturally.
func manualDishesForMeal(dishes []models.Dish, manualIDs []uint) []models.Dish {
	if len(manualIDs) == 0 {
		return nil
	}
	wanted := make(map[uint]bool, len(manualIDs))
	for _, id := range manualIDs {
		wanted[id] = true
	}
	var kept []models.Dish
	for _, dish := range dishes {
		if wanted[dish.ID] {
			kept = append(kept, dish)
		}
	}
	return kept
}

// reduceQuotaForKept subtracts the slots already occupied by kept (manual)
// dishes, matching each dish to the slot type it would fill with the same
// test matchesWeekPlanSlot uses (soup, veg, else meat).
func reduceQuotaForKept(quota MealQuota, kept []models.Dish) MealQuota {
	for _, dish := range kept {
		d := ensureDishTraits(dish)
		switch {
		case d.DishRole == "soup" || isSoupDish(d):
			if quota.SoupCount > 0 {
				quota.SoupCount--
			}
		case d.DishRole == "veg":
			if quota.VegCount > 0 {
				quota.VegCount--
			}
		default:
			if quota.MeatCount > 0 {
				quota.MeatCount--
			}
		}
	}
	return quota
}

// adjacentPlannedSoups collects the soups already planned on the days next
// to dayIndex, so a regenerated day's soup avoids clashing with both
// neighbors (sequential generation only ever has the previous day).
func adjacentPlannedSoups(days []WeekDayPlan, dayIndex int) []models.Dish {
	var result []models.Dish
	for _, neighbor := range []int{dayIndex - 1, dayIndex + 1} {
		if neighbor < 0 || neighbor >= len(days) {
			continue
		}
		result = append(result, soupsFromDishes(appendDishSlices(days[neighbor].Lunch, days[neighbor].Dinner))...)
	}
	return result
}

// weekPlanDayContext carries the effective taste profile and cravings for
// one generated day.
type weekPlanDayContext struct {
	profile           string
	dayWant           []string
	weekWant          []string
	isWeekend         bool
	categoryFavorites map[string]int
	prevSoups         []models.Dish
}

// resolveWeekPlanDay applies the per-day override for weekday index i
// (0=Monday) on top of the period defaults, and adds a temporary soup slot
// on 靓汤 days whose quotas have no soup at all.
func resolveWeekPlanDay(prefs WeekPlanPreferences, periodPrefs WeekPlanPeriodPreferences, i int) (weekPlanDayContext, MealQuota, MealQuota) {
	dayCtx := weekPlanDayContext{profile: periodPrefs.Profile, weekWant: prefs.WeekWant, isWeekend: i >= 5}
	if i >= 0 && i < len(weekPlanDayKeys) {
		if override, ok := prefs.Days[weekPlanDayKeys[i]]; ok {
			if override.Profile != "" {
				dayCtx.profile = override.Profile
			}
			dayCtx.dayWant = override.Want
		}
	}
	lunchQuota, dinnerQuota := periodPrefs.Lunch, periodPrefs.Dinner
	if dayCtx.profile == "soup" && lunchQuota.SoupCount+dinnerQuota.SoupCount == 0 {
		if dinnerQuota.total() > 0 {
			dinnerQuota.SoupCount = 1
		} else if lunchQuota.total() > 0 {
			lunchQuota.SoupCount = 1
		}
	}
	return dayCtx, lunchQuota, dinnerQuota
}

func wantBonus(dish models.Dish, want []string, points float64) float64 {
	if len(want) == 0 {
		return 0
	}
	proteins := parseJSONStrings(dish.ProteinSources)
	for _, key := range want {
		if containsString(proteins, key) {
			return points
		}
	}
	return 0
}

func pickQuotaDishes(pool []models.Dish, quota MealQuota, dayCtx weekPlanDayContext, mealType string, dayName string, globalUsed map[uint]bool, dayUsed map[uint]bool, recent map[uint]bool, dayPicked []models.Dish, weekPicked []models.Dish, rules []compiledMenuRule, warnings *[]string, r *rand.Rand, keep []models.Dish) []models.Dish {
	quota = normalizeMealQuota(quota)
	// Manual dishes occupy their slots up front: they sit in picked so every
	// same-meal rule context sees them, and quota was already reduced by the
	// caller (reduceQuotaForKept).
	picked := append([]models.Dish{}, keep...)
	if quota.total() <= 0 {
		return picked
	}
	for _, slot := range weekPlanSlots(quota) {
		dish, ok, relaxed := pickBestDishForSlot(pool, slot, dayCtx, quota, globalUsed, dayUsed, recent, picked, dayPicked, weekPicked, rules, r)
		if !ok {
			*warnings = append(*warnings, fmt.Sprintf("%s%s缺一道%s：可选的菜不够或都被规则挡住了，去菜品库加几道吧", dayName, mealLabelForWarning(mealType), slotLabelForWarning(slot)))
			continue
		}
		if relaxed != "" {
			*warnings = append(*warnings, relaxationWarning(dayName, mealLabelForWarning(mealType), slotLabelForWarning(slot), relaxed))
		}
		picked = append(picked, dish)
		globalUsed[dish.ID] = true
		dayUsed[dish.ID] = true
	}
	return picked
}

type weekPlanSlot string

const (
	weekPlanSlotMeat weekPlanSlot = "meat"
	weekPlanSlotVeg  weekPlanSlot = "veg"
	weekPlanSlotSoup weekPlanSlot = "soup"
)

type weekPlanRelaxationStage struct {
	label         string
	avoidRecent   bool
	strictProfile bool
	avoidGlobal   bool
	enforceSoft   bool
	strictRole    bool
}

type weekPlanCandidateScore struct {
	dish  models.Dish
	score float64
}

func weekPlanSlots(quota MealQuota) []weekPlanSlot {
	var slots []weekPlanSlot
	for i := 0; i < quota.MeatCount; i++ {
		slots = append(slots, weekPlanSlotMeat)
	}
	for i := 0; i < quota.VegCount; i++ {
		slots = append(slots, weekPlanSlotVeg)
	}
	for i := 0; i < quota.SoupCount; i++ {
		slots = append(slots, weekPlanSlotSoup)
	}
	return slots
}

func pickBestDishForSlot(pool []models.Dish, slot weekPlanSlot, dayCtx weekPlanDayContext, quota MealQuota, globalUsed map[uint]bool, dayUsed map[uint]bool, recent map[uint]bool, mealPicked []models.Dish, dayPicked []models.Dish, weekPicked []models.Dish, rules []compiledMenuRule, r *rand.Rand) (models.Dish, bool, string) {
	stages := []weekPlanRelaxationStage{
		{label: "", avoidRecent: true, strictProfile: true, avoidGlobal: true, enforceSoft: true, strictRole: true},
		{label: "最近避重", avoidRecent: false, strictProfile: true, avoidGlobal: true, enforceSoft: true, strictRole: true},
		{label: "口味画像", avoidRecent: false, strictProfile: false, avoidGlobal: true, enforceSoft: true, strictRole: true},
		{label: "全周唯一", avoidRecent: false, strictProfile: false, avoidGlobal: false, enforceSoft: true, strictRole: true},
		{label: "可降级规则", avoidRecent: false, strictProfile: false, avoidGlobal: false, enforceSoft: false, strictRole: true},
		{label: "菜品角色", avoidRecent: false, strictProfile: false, avoidGlobal: false, enforceSoft: false, strictRole: false},
	}

	for _, stage := range stages {
		candidates := scoreWeekPlanCandidates(pool, slot, dayCtx, quota, globalUsed, dayUsed, recent, mealPicked, dayPicked, weekPicked, rules, stage, r)
		if len(candidates) == 0 {
			continue
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].score != candidates[j].score {
				return candidates[i].score > candidates[j].score
			}
			return candidates[i].dish.ID < candidates[j].dish.ID
		})
		return candidates[0].dish, true, stage.label
	}
	return models.Dish{}, false, ""
}

func scoreWeekPlanCandidates(pool []models.Dish, slot weekPlanSlot, dayCtx weekPlanDayContext, quota MealQuota, globalUsed map[uint]bool, dayUsed map[uint]bool, recent map[uint]bool, mealPicked []models.Dish, dayPicked []models.Dish, weekPicked []models.Dish, rules []compiledMenuRule, stage weekPlanRelaxationStage, r *rand.Rand) []weekPlanCandidateScore {
	profile := normalizePlanProfile(dayCtx.profile)
	// 靓汤 themes are fulfilled by the dedicated soup slot; forcing meat/veg
	// slots to match a soup profile would only burn the strict stages and
	// emit misleading "口味画像" warnings on every soup day.
	strictProfile := stage.strictProfile && !(profile == "soup" && slot != weekPlanSlotSoup)
	var scores []weekPlanCandidateScore
	for _, dish := range pool {
		dish = ensureDishTraits(dish)
		if dayUsed[dish.ID] {
			continue
		}
		if stage.avoidGlobal && globalUsed[dish.ID] {
			continue
		}
		if stage.avoidRecent && recent[dish.ID] {
			continue
		}
		if !matchesWeekPlanSlot(dish, slot, stage.strictRole) {
			continue
		}
		if strictProfile && !matchesTomorrowProfile(dish, profile) {
			continue
		}
		allowed, _ := evaluateConstraintRules(dish, dayCtx, quota, mealPicked, dayPicked, weekPicked, rules, stage.enforceSoft)
		if !allowed {
			continue
		}
		score := baseWeekPlanDishScore(dish, slot, dayCtx, stage.strictRole)
		score += evaluateScoreRules(dish, dayCtx, quota, mealPicked, dayPicked, weekPicked, rules, stage.enforceSoft)
		score += r.Float64()
		scores = append(scores, weekPlanCandidateScore{dish: dish, score: score})
	}
	return scores
}

func matchesWeekPlanSlot(dish models.Dish, slot weekPlanSlot, strictRole bool) bool {
	dish = ensureDishTraits(dish)
	if strictRole {
		switch slot {
		case weekPlanSlotSoup:
			return dish.DishRole == "soup" || isSoupDish(dish)
		case weekPlanSlotVeg:
			return dish.DishRole == "veg"
		case weekPlanSlotMeat:
			return dish.DishRole == "meat"
		default:
			return true
		}
	}
	if slot == weekPlanSlotSoup {
		return dish.DishRole == "soup" || isSoupDish(dish)
	}
	return dish.DishRole != "soup"
}

func baseWeekPlanDishScore(dish models.Dish, slot weekPlanSlot, dayCtx weekPlanDayContext, strictRole bool) float64 {
	score := float64(tomorrowDishScore(dish, normalizePlanProfile(dayCtx.profile)))
	score += wantBonus(dish, dayCtx.dayWant, 25)
	score += wantBonus(dish, dayCtx.weekWant, 12)
	if strictRole {
		score += 40
	}
	switch slot {
	case weekPlanSlotMeat:
		if dish.DishRole == "meat" {
			score += 60
		}
	case weekPlanSlotVeg:
		if dish.DishRole == "veg" {
			score += 60
		}
	case weekPlanSlotSoup:
		if dish.DishRole == "soup" {
			score += 60
		}
	}
	if dish.TraitSource == "manual" {
		score += 4
	}
	return score
}

// evaluateConstraintRules rejects a candidate that violates any constraint
// rule. Relaxable rules are skipped once the relaxation ladder reaches the
// enforceSoft=false stages; non-relaxable rules block at every stage,
// regardless of severity.
func evaluateConstraintRules(candidate models.Dish, dayCtx weekPlanDayContext, quota MealQuota, mealPicked []models.Dish, dayPicked []models.Dish, weekPicked []models.Dish, rules []compiledMenuRule, enforceSoft bool) (bool, string) {
	for _, item := range rules {
		if item.rule.RuleKind != menuRuleKindConstraint {
			continue
		}
		if item.rule.Relaxable && !enforceSoft {
			continue
		}
		env := buildRuleEnv(dishRuleEnv(candidate, dayCtx.categoryFavorites), dishesRuleEnv(mealPicked, dayCtx.categoryFavorites), dishesRuleEnv(unionDishesByID(dayPicked, mealPicked), dayCtx.categoryFavorites), dishesRuleEnv(unionDishesByID(weekPicked, mealPicked), dayCtx.categoryFavorites), dishesRuleEnv(dayCtx.prevSoups, dayCtx.categoryFavorites), dayCtx.profile, quota, dayCtx.isWeekend)
		out, err := exprRunRule(item.program, env)
		if err != nil {
			continue
		}
		if ok, _ := out.(bool); !ok {
			return false, item.rule.Message
		}
	}
	return true, ""
}

func evaluateScoreRules(candidate models.Dish, dayCtx weekPlanDayContext, quota MealQuota, mealPicked []models.Dish, dayPicked []models.Dish, weekPicked []models.Dish, rules []compiledMenuRule, enforceSoft bool) float64 {
	if !enforceSoft {
		return 0
	}
	total := 0.0
	for _, item := range rules {
		if item.rule.RuleKind != menuRuleKindScore {
			continue
		}
		env := buildRuleEnv(dishRuleEnv(candidate, dayCtx.categoryFavorites), dishesRuleEnv(mealPicked, dayCtx.categoryFavorites), dishesRuleEnv(unionDishesByID(dayPicked, mealPicked), dayCtx.categoryFavorites), dishesRuleEnv(unionDishesByID(weekPicked, mealPicked), dayCtx.categoryFavorites), dishesRuleEnv(dayCtx.prevSoups, dayCtx.categoryFavorites), dayCtx.profile, quota, dayCtx.isWeekend)
		out, err := exprRunRule(item.program, env)
		if err != nil {
			continue
		}
		if value, ok := numericRuleOutput(out); ok {
			total += value
		}
	}
	return total
}

func exprRunRule(program *vm.Program, env map[string]any) (any, error) {
	return expr.Run(program, env)
}

// unionDishesByID returns base plus the extra dishes whose ID is not already
// present in base (order kept: base first, then new extras). The day/week
// rule envs use it to merge the in-progress meal picks into the day/week
// state without counting keep (manual) dishes twice — those sit in both sets
// by design while their own meal is being filled. ID 0 means an unsaved dish
// and never dedupes: distinct unsaved dishes must all count.
func unionDishesByID(base []models.Dish, extra []models.Dish) []models.Dish {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[uint]bool, len(base))
	for _, dish := range base {
		if dish.ID != 0 {
			seen[dish.ID] = true
		}
	}
	result := make([]models.Dish, 0, len(base)+len(extra))
	result = append(result, base...)
	for _, dish := range extra {
		if dish.ID != 0 && seen[dish.ID] {
			continue
		}
		result = append(result, dish)
	}
	return result
}

func appendDishSlices(slices ...[]models.Dish) []models.Dish {
	total := 0
	for _, values := range slices {
		total += len(values)
	}
	result := make([]models.Dish, 0, total)
	for _, values := range slices {
		result = append(result, values...)
	}
	return result
}

func uniqueWarnings(warnings []string) []string {
	seen := make(map[string]bool, len(warnings))
	result := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		warning = strings.TrimSpace(warning)
		if warning == "" || seen[warning] {
			continue
		}
		seen[warning] = true
		result = append(result, warning)
	}
	return result
}

// relaxationWarning turns a relaxation-stage label into a plain-Chinese
// explanation of what the planner compromised on to fill the slot.
func relaxationWarning(dayName string, mealLabel string, slotLabel string, stageLabel string) string {
	prefix := fmt.Sprintf("%s%s的%s", dayName, mealLabel, slotLabel)
	switch stageLabel {
	case "最近避重":
		return prefix + "不够选，安排了一道最近刚吃过的菜"
	case "口味画像":
		return prefix + "没有合当天口味的，放宽了口味要求"
	case "全周唯一":
		return prefix + "不够选，和本周其他天重复了一道"
	case "可降级规则":
		return prefix + "放宽了部分搭配规则才补上"
	case "菜品角色":
		return prefix + "不够，用其他类型的菜顶上了"
	default:
		return fmt.Sprintf("%s为补上做了让步（%s）", prefix, stageLabel)
	}
}

func mealLabelForWarning(mealType string) string {
	if mealType == "lunch" {
		return "午餐"
	}
	return "晚餐"
}

func slotLabelForWarning(slot weekPlanSlot) string {
	switch slot {
	case weekPlanSlotMeat:
		return "荤菜"
	case weekPlanSlotVeg:
		return "素菜"
	case weekPlanSlotSoup:
		return "汤"
	default:
		return "菜品"
	}
}

func parseTags(tagsStr string) []string {
	var tags []string
	json.Unmarshal([]byte(tagsStr), &tags)
	return tags
}

func containsTag(tags []string, target string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, target) {
			return true
		}
	}
	return false
}

type ShoppingCategory struct {
	Category string         `json:"category"`
	Items    []ShoppingItem `json:"items"`
}

type ShoppingItem struct {
	Name    string `json:"name"`
	Amount  string `json:"amount"`
	Checked bool   `json:"checked"`
	InStock bool   `json:"in_stock"`
}

type ShoppingCategoryOverride struct {
	ItemName string `json:"item_name"`
	Category string `json:"category"`
}

var shoppingCategoryOrder = map[string]int{
	"蔬菜": 0,
	"肉类": 1,
	"配料": 2,
	"其他": 3,
}

var shoppingCategoryExactTerms = map[string][]string{
	"蔬菜": {
		"番茄", "西红柿", "圣女果", "小番茄",
		"西兰花", "花菜", "菜花", "包菜", "卷心菜", "娃娃菜",
		"白菜", "大白菜", "小白菜", "青菜", "油菜", "油麦菜", "生菜",
		"菠菜", "空心菜", "苋菜", "茼蒿", "芹菜", "香菜", "韭菜",
		"葱", "小葱", "大葱", "葱花", "姜", "姜片", "姜丝",
		"蒜", "大蒜", "蒜瓣", "蒜末", "蒜苗", "蒜苔", "洋葱",
		"土豆", "马铃薯", "红薯", "地瓜", "山药", "芋头", "莲藕",
		"萝卜", "白萝卜", "胡萝卜", "莴笋", "竹笋", "冬笋", "春笋",
		"黄瓜", "丝瓜", "冬瓜", "南瓜", "苦瓜", "西葫芦", "茄子",
		"青椒", "红椒", "彩椒", "甜椒", "尖椒", "线椒", "小米辣",
		"蘑菇", "香菇", "平菇", "金针菇", "杏鲍菇", "口蘑", "蟹味菇",
		"木耳", "银耳", "海带", "紫菜", "裙带菜",
		"豆腐", "嫩豆腐", "老豆腐", "内酯豆腐", "油豆腐", "豆皮", "千张",
		"腐竹", "豆芽", "黄豆芽", "绿豆芽", "毛豆", "豌豆", "荷兰豆",
		"四季豆", "豆角", "豇豆", "玉米",
	},
	"肉类": {
		"猪肉", "猪肉末", "肉末", "肉丝", "肉片", "五花肉", "里脊肉",
		"排骨", "猪蹄", "猪肝", "猪肚", "腊肉", "火腿", "香肠", "午餐肉",
		"牛肉", "牛腩", "牛排", "肥牛", "牛肉丸",
		"羊肉", "羊排", "鸡肉", "鸡腿", "鸡翅", "鸡胸肉", "鸡爪",
		"鸭肉", "鸭腿", "鸭血", "鹅肉",
		"鸡蛋", "蛋液", "蛋清", "蛋白", "蛋黄", "鸭蛋", "鹌鹑蛋", "皮蛋", "咸鸭蛋",
		"鱼", "鱼片", "鲈鱼", "鲫鱼", "草鱼", "带鱼", "三文鱼", "鳕鱼",
		"虾", "虾仁", "基围虾", "明虾", "虾滑", "蟹", "螃蟹", "蟹棒",
		"蛤蜊", "花甲", "扇贝", "干贝", "鱿鱼", "墨鱼", "章鱼",
	},
	"配料": {
		"盐", "食盐", "糖", "白糖", "冰糖", "红糖", "蜂蜜",
		"生抽", "老抽", "酱油", "蚝油", "醋", "陈醋", "白醋", "米醋",
		"料酒", "黄酒", "白酒", "啤酒", "味淋",
		"油", "食用油", "花生油", "玉米油", "菜籽油", "橄榄油", "香油", "麻油", "辣椒油",
		"豆瓣酱", "郫县豆瓣酱", "黄豆酱", "甜面酱", "番茄酱", "沙茶酱", "芝麻酱",
		"辣椒酱", "蒜蓉辣酱", "剁椒酱", "老干妈", "火锅底料",
		"淀粉", "玉米淀粉", "土豆淀粉", "红薯淀粉", "生粉",
		"花椒", "花椒粉", "胡椒", "胡椒粉", "白胡椒粉", "黑胡椒", "黑胡椒粉",
		"辣椒粉", "五香粉", "孜然", "孜然粉", "椒盐", "十三香", "咖喱", "咖喱块", "咖喱粉",
		"八角", "桂皮", "香叶", "小茴香", "草果", "丁香", "豆蔻", "陈皮",
		"鸡精", "味精", "鸡粉", "高汤", "浓汤宝",
		"白芝麻", "黑芝麻", "芝麻", "豆豉", "腐乳", "虾皮", "海米", "鱼露", "蛋黄酱",
	},
	"其他": {
		"大米", "米", "糯米", "小米", "黑米", "燕麦", "面粉", "低筋面粉", "高筋面粉", "糯米粉",
		"面条", "挂面", "意面", "鸡蛋面", "方便面", "泡面", "粉丝", "粉条", "米粉", "河粉", "宽粉", "红薯粉", "土豆粉",
		"年糕", "馒头", "面包", "吐司", "饺子皮", "馄饨皮",
		"牛奶", "淡奶油", "黄油", "芝士", "奶酪", "可乐", "雪碧",
		"花生", "花生米", "腰果", "核桃", "杏仁", "葡萄干",
	},
}

var shoppingCategoryExact = buildShoppingCategoryExact(shoppingCategoryExactTerms)

func buildShoppingCategoryExact(terms map[string][]string) map[string]string {
	result := make(map[string]string)
	for category, names := range terms {
		if !isValidShoppingCategory(category) {
			continue
		}
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name != "" {
				result[name] = category
			}
		}
	}
	return result
}

var amountPattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)(.*)$`)

type numericAmountAccum struct {
	total float64
}

func compactShoppingAmounts(amounts []string) string {
	numericByUnit := make(map[string]*numericAmountAccum)
	textSeen := make(map[string]bool)

	for _, amount := range amounts {
		for _, part := range strings.Split(strings.ReplaceAll(amount, "＋", "+"), "+") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			if m := amountPattern.FindStringSubmatch(part); m != nil {
				num, err := strconv.ParseFloat(m[1], 64)
				if err == nil {
					unit := strings.TrimSpace(m[2])
					acc, exists := numericByUnit[unit]
					if !exists {
						acc = &numericAmountAccum{}
						numericByUnit[unit] = acc
					}
					acc.total += num
					continue
				}
			}

			if !textSeen[part] {
				textSeen[part] = true
			}
		}
	}

	if len(numericByUnit) == 0 && len(textSeen) == 0 {
		return "适量"
	}

	parts := make([]string, 0, len(textSeen)+len(numericByUnit))
	texts := make([]string, 0, len(textSeen))
	for text := range textSeen {
		texts = append(texts, text)
	}
	sort.Strings(texts)
	parts = append(parts, texts...)

	units := make([]string, 0, len(numericByUnit))
	for unit := range numericByUnit {
		units = append(units, unit)
	}
	sort.Strings(units)
	for _, unit := range units {
		parts = append(parts, formatAmountNumber(numericByUnit[unit].total)+unit)
	}

	if len(parts) == 0 {
		return "适量"
	}
	return strings.Join(parts, "+")
}

func formatAmountNumber(n float64) string {
	rounded := math.Round(n)
	if math.Abs(n-rounded) < 1e-9 {
		return strconv.FormatInt(int64(rounded), 10)
	}
	return strconv.FormatFloat(n, 'f', -1, 64)
}

func sortShoppingItems(items []ShoppingItem) {
	sort.Slice(items, func(i, j int) bool {
		if shoppingItemPriority(items[i]) != shoppingItemPriority(items[j]) {
			return shoppingItemPriority(items[i]) < shoppingItemPriority(items[j])
		}
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		if items[i].Amount != items[j].Amount {
			return items[i].Amount < items[j].Amount
		}
		return false
	})
}

func shoppingItemPriority(item ShoppingItem) int {
	switch {
	case item.InStock:
		return 2
	case item.Checked:
		return 1
	default:
		return 0
	}
}

func BuildShoppingList(dates []string) []ShoppingCategory {
	if len(dates) == 0 {
		return []ShoppingCategory{}
	}

	var checks []models.ShoppingCheck
	database.DB.Where("meal_date IN ?", dates).Find(&checks)
	if len(checks) == 0 {
		return []ShoppingCategory{}
	}

	type nameAccum struct {
		amounts []string
		checked bool
	}
	merged := make(map[string]*nameAccum)
	for _, ch := range checks {
		entry, exists := merged[ch.ItemName]
		if !exists {
			entry = &nameAccum{}
			merged[ch.ItemName] = entry
		}
		if ch.ItemAmount != "" {
			entry.amounts = append(entry.amounts, ch.ItemAmount)
		}
		if ch.Checked {
			entry.checked = true
		}
	}

	var inventory []models.HomeInventory
	database.DB.Where("in_stock = ?", true).Find(&inventory)
	inStockByName := make(map[string]bool, len(inventory))
	for _, inv := range inventory {
		inStockByName[inv.ItemName] = true
	}

	categoryByName := getShoppingCategoryOverrideMap()

	grouped := map[string][]ShoppingItem{
		"蔬菜": {},
		"肉类": {},
		"配料": {},
		"其他": {},
	}
	for name, entry := range merged {
		amountStr := compactShoppingAmounts(entry.amounts)
		item := ShoppingItem{Name: name, Amount: amountStr, Checked: entry.checked, InStock: inStockByName[name]}
		category := classifyShoppingItem(name, categoryByName)
		if _, ok := grouped[category]; !ok {
			category = "其他"
		}
		grouped[category] = append(grouped[category], item)
	}

	var result []ShoppingCategory
	categoryNames := []string{"蔬菜", "肉类", "配料", "其他"}
	for _, category := range categoryNames {
		items := grouped[category]
		if len(items) == 0 {
			continue
		}
		sortShoppingItems(items)
		result = append(result, ShoppingCategory{Category: category, Items: items})
	}

	return result
}

func classifyShoppingItem(name string, overrides map[string]string) string {
	if category := overrides[name]; isValidShoppingCategory(category) {
		return category
	}

	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "其他"
	}

	if category := shoppingCategoryExact[normalized]; isValidShoppingCategory(category) {
		return category
	}

	seasoningKw := []string{
		"盐", "糖", "酱", "油", "醋", "料酒", "生抽", "老抽", "蚝油", "淀粉", "生粉", "粉",
		"八角", "花椒", "胡椒", "孜然", "香叶", "桂皮", "辣椒粉", "豆瓣", "冰糖", "鸡精", "味精",
		"十三香", "五香", "椒盐", "香油", "麻油", "咖喱", "芡", "汁", "底料", "腐乳", "豆豉", "芝麻",
	}
	meatKw := []string{
		"肉", "排骨", "牛腩", "五花", "里脊", "肥牛", "猪蹄", "猪肝", "猪肚", "火腿", "香肠", "午餐肉",
		"鸡腿", "鸡翅", "鸡胸", "鸡爪", "鸭腿", "鸭血", "羊排", "牛排", "鱼片", "虾仁", "虾滑", "蟹",
		"蛤蜊", "花甲", "扇贝", "鱿鱼", "墨鱼", "章鱼",
	}
	vegetableKw := []string{
		"菜", "瓜", "豆", "葱", "姜", "蒜", "椒", "萝卜", "笋", "菇", "木耳", "银耳", "藕", "番茄",
		"西红柿", "西兰花", "土豆", "洋葱", "茄子", "芹菜", "香菜", "韭菜", "菠菜", "紫菜", "海带",
		"豆腐", "豆皮", "豆芽", "冬瓜", "南瓜", "山药", "芋头", "莲藕", "玉米", "菌", "蘑",
	}
	otherKw := []string{
		"米", "面", "粉丝", "粉条", "米粉", "河粉", "宽粉", "年糕", "馒头", "面包", "吐司", "饺子皮",
		"馄饨皮", "牛奶", "奶油", "黄油", "芝士", "奶酪", "可乐", "雪碧", "花生", "腰果", "核桃", "杏仁",
	}

	if hasKeyword(normalized, seasoningKw) {
		return "配料"
	}
	if hasKeyword(normalized, meatKw) {
		return "肉类"
	}
	if hasKeyword(normalized, vegetableKw) {
		return "蔬菜"
	}
	if hasKeyword(normalized, otherKw) {
		return "其他"
	}
	return "其他"
}

func getShoppingCategoryOverrideMap() map[string]string {
	var overrides []models.ShoppingItemCategory
	database.DB.Order("item_name ASC").Find(&overrides)

	result := make(map[string]string, len(overrides))
	for _, override := range overrides {
		if isValidShoppingCategory(override.Category) {
			result[override.ItemName] = override.Category
		}
	}
	return result
}

func isValidShoppingCategory(category string) bool {
	_, ok := shoppingCategoryOrder[category]
	return ok
}

func ListShoppingCategoryOverrides() []ShoppingCategoryOverride {
	var rows []models.ShoppingItemCategory
	database.DB.Order("category ASC, item_name ASC").Find(&rows)

	overrides := make([]ShoppingCategoryOverride, 0, len(rows))
	for _, row := range rows {
		if !isValidShoppingCategory(row.Category) {
			continue
		}
		overrides = append(overrides, ShoppingCategoryOverride{
			ItemName: row.ItemName,
			Category: row.Category,
		})
	}

	sort.SliceStable(overrides, func(i, j int) bool {
		if shoppingCategoryOrder[overrides[i].Category] != shoppingCategoryOrder[overrides[j].Category] {
			return shoppingCategoryOrder[overrides[i].Category] < shoppingCategoryOrder[overrides[j].Category]
		}
		return overrides[i].ItemName < overrides[j].ItemName
	})
	return overrides
}

func UpsertShoppingCategoryOverride(itemName, category string) bool {
	itemName = strings.TrimSpace(itemName)
	category = strings.TrimSpace(category)
	if itemName == "" || !isValidShoppingCategory(category) {
		return false
	}

	database.DB.Where("item_name = ?", itemName).
		Assign(models.ShoppingItemCategory{Category: category}).
		FirstOrCreate(&models.ShoppingItemCategory{ItemName: itemName})
	return true
}

func DeleteShoppingCategoryOverride(itemName string) {
	itemName = strings.TrimSpace(itemName)
	if itemName == "" {
		return
	}
	database.DB.Where("item_name = ?", itemName).Delete(&models.ShoppingItemCategory{})
}

func ToggleShoppingCheck(itemName string, mealDate string, checked bool) {
	database.DB.Model(&models.ShoppingCheck{}).
		Where("item_name = ?", itemName).
		Update("checked", checked)
}

func ToggleHomeInventory(itemName string, inStock bool) {
	itemName = strings.TrimSpace(itemName)
	if itemName == "" {
		return
	}

	if !inStock {
		database.DB.Where("item_name = ?", itemName).Delete(&models.HomeInventory{})
		return
	}

	database.DB.Where("item_name = ?", itemName).
		Assign(models.HomeInventory{InStock: true}).
		FirstOrCreate(&models.HomeInventory{ItemName: itemName})
}

func hasKeyword(s string, keywords []string) bool {
	for _, k := range keywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}
