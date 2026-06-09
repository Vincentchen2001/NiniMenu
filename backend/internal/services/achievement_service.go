package services

import (
	"encoding/json"
	"ninimenu/internal/achievements"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	achievementSyncDelay  = 300 * time.Millisecond
	achievementEnabledKey = "achievements_enabled"
)

var (
	defaultAchievementsMu      sync.Mutex
	defaultAchievementsEnsured bool
	achievementSyncMu          sync.Mutex
	achievementSyncTimer       *time.Timer
	achievementRunMu           sync.Mutex
)

type achievementSnapshot struct {
	totalRecords        int
	distinctDishes      map[uint]bool
	recordDates         map[string]bool
	mealCounts          map[string]int
	fullDayCount        int
	categoryCounts      map[string]int
	recordCategories    map[string]bool
	difficultyCounts    map[string]int
	quickCount          int
	fastCount           int
	slowCount           int
	weekendHard         bool
	weekendRecords      int
	hasMidnightRecord   bool
	hasBreakfastRecord  bool
	seasonSet           map[string]bool
	tasteSet            map[string]bool
	spicyCount          int
	sweetCount          int
	sourCount           int
	umamiCount          int
	lightCount          int
	healthyCount        int
	recordMoodCounts    map[string]int
	recordRemarkCount   int
	homeMoodCounts      map[string]int
	dayRatingCount      int
	dayRemarkCount      int
	photoDays           int
	photoCount          int
	favoriteCount       int
	shoppingChecked     int
	inventoryCount      int
	eventCounts         map[string]int
	dishLibraryCount    int
	imageDishCount      int
	videoDishCount      int
	ingredientDishCount int
	stepDishCount       int
	libraryCategorySet  map[string]bool
	topDishCount        int
	maxMonthRecords     int
	maxSeasonRecords    int
	consecutiveQuick    int
	consecutiveSpicy    int
	longestDateStreak   int
}

type achievementRecordInfo struct {
	record  models.MealRecord
	dish    *models.Dish
	date    time.Time
	hasDate bool
}

func AutoAchievementsEnabled() bool {
	if database.DB == nil {
		return false
	}

	var setting models.Setting
	if err := database.DB.Where("`key` = ?", achievementEnabledKey).First(&setting).Error; err != nil {
		return false
	}

	switch strings.ToLower(strings.TrimSpace(setting.Value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func EnsureDefaultAchievements() {
	defaultAchievementsMu.Lock()
	defer defaultAchievementsMu.Unlock()
	if defaultAchievementsEnsured {
		return
	}

	defaults := achievements.DefaultAchievements()
	codes := make([]string, 0, len(defaults))
	for _, a := range defaults {
		codes = append(codes, a.Code)
	}

	var existing []models.Achievement
	database.DB.Where("code IN ?", codes).Find(&existing)
	existingByCode := make(map[string]models.Achievement, len(existing))
	for _, a := range existing {
		existingByCode[a.Code] = a
		if a.Condition == "" || a.Condition == "manual" {
			database.DB.Model(&a).Update("condition", "auto")
		}
	}

	missing := make([]models.Achievement, 0, len(defaults)-len(existingByCode))
	for _, a := range defaults {
		if _, ok := existingByCode[a.Code]; ok {
			continue
		}
		missing = append(missing, a)
	}
	if len(missing) > 0 {
		database.DB.Create(&missing)
	}

	defaultAchievementsEnsured = true
}

func QueueAutoAchievementSync() {
	if !AutoAchievementsEnabled() {
		return
	}

	achievementSyncMu.Lock()
	defer achievementSyncMu.Unlock()

	if achievementSyncTimer != nil {
		achievementSyncTimer.Reset(achievementSyncDelay)
		return
	}

	achievementSyncTimer = time.AfterFunc(achievementSyncDelay, func() {
		achievementSyncMu.Lock()
		achievementSyncTimer = nil
		achievementSyncMu.Unlock()
		SyncAutoAchievements()
	})
}

func cancelQueuedAutoAchievementSync() {
	achievementSyncMu.Lock()
	defer achievementSyncMu.Unlock()

	if achievementSyncTimer != nil {
		achievementSyncTimer.Stop()
		achievementSyncTimer = nil
	}
}

func RecordAchievementEvent(eventType string, refKey string) {
	eventType = strings.TrimSpace(eventType)
	refKey = strings.TrimSpace(refKey)
	if eventType == "" {
		return
	}
	if !AutoAchievementsEnabled() {
		return
	}

	event := models.AchievementEvent{EventType: eventType, RefKey: refKey}
	database.DB.Create(&event)

	QueueAutoAchievementSync()
}

func RecordUniqueAchievementEvent(eventType string, refKey string) {
	eventType = strings.TrimSpace(eventType)
	refKey = strings.TrimSpace(refKey)
	if eventType == "" || refKey == "" {
		return
	}
	if !AutoAchievementsEnabled() {
		return
	}

	event := models.AchievementEvent{EventType: eventType, RefKey: refKey}
	database.DB.Where("event_type = ? AND ref_key = ?", eventType, refKey).
		FirstOrCreate(&event)

	QueueAutoAchievementSync()
}

func SyncAutoAchievements() {
	cancelQueuedAutoAchievementSync()
	if !AutoAchievementsEnabled() {
		return
	}

	achievementRunMu.Lock()
	defer achievementRunMu.Unlock()

	EnsureDefaultAchievements()

	unlockable := evaluateAchievementCodes(buildAchievementSnapshot())
	if len(unlockable) == 0 {
		return
	}

	codes := make([]string, 0, len(unlockable))
	for code := range unlockable {
		codes = append(codes, code)
	}

	var autoAchievements []models.Achievement
	database.DB.Where("condition = ? AND code IN ?", "auto", codes).Find(&autoAchievements)
	if len(autoAchievements) == 0 {
		return
	}

	achievementIDs := make([]uint, 0, len(autoAchievements))
	for _, a := range autoAchievements {
		achievementIDs = append(achievementIDs, a.ID)
	}

	var unlocked []models.UserAchievement
	database.DB.Where("achievement_id IN ?", achievementIDs).Find(&unlocked)
	unlockedMap := make(map[uint]bool, len(unlocked))
	for _, u := range unlocked {
		unlockedMap[u.AchievementID] = true
	}

	now := time.Now()
	newUnlocks := make([]models.UserAchievement, 0)
	for _, a := range autoAchievements {
		if !unlockable[a.Code] || unlockedMap[a.ID] {
			continue
		}
		newUnlocks = append(newUnlocks, models.UserAchievement{
			AchievementID: a.ID,
			UnlockedAt:    now,
		})
		unlockedMap[a.ID] = true
	}
	if len(newUnlocks) > 0 {
		database.DB.Create(&newUnlocks)
	}
}

func buildAchievementSnapshot() achievementSnapshot {
	s := achievementSnapshot{
		distinctDishes:     map[uint]bool{},
		recordDates:        map[string]bool{},
		mealCounts:         map[string]int{},
		categoryCounts:     map[string]int{},
		recordCategories:   map[string]bool{},
		difficultyCounts:   map[string]int{},
		seasonSet:          map[string]bool{},
		tasteSet:           map[string]bool{},
		recordMoodCounts:   map[string]int{},
		homeMoodCounts:     map[string]int{},
		eventCounts:        map[string]int{},
		libraryCategorySet: map[string]bool{},
	}

	var allDishes []models.Dish
	database.DB.Unscoped().
		Select("id", "name", "image_url", "images", "video_url", "category", "taste", "ingredients", "steps", "cook_time", "difficulty", "deleted_at").
		Find(&allDishes)
	dishByID := make(map[uint]models.Dish, len(allDishes))
	for _, d := range allDishes {
		dishByID[d.ID] = d
		if !d.DeletedAt.Valid {
			s.dishLibraryCount++
			if normalized := strings.TrimSpace(d.Category); normalized != "" {
				s.libraryCategorySet[normalized] = true
			}
			if hasDishImage(d) {
				s.imageDishCount++
			}
			if strings.TrimSpace(d.VideoURL) != "" {
				s.videoDishCount++
			}
			if jsonArrayLen(d.Ingredients) > 0 {
				s.ingredientDishCount++
			}
			if jsonArrayLen(d.Steps) > 0 {
				s.stepDishCount++
			}
		}
	}

	var records []models.MealRecord
	database.DB.
		Select("id", "dish_id", "meal_type", "meal_date", "remark", "mood", "created_at").
		Order("created_at ASC, id ASC").
		Find(&records)
	s.totalRecords = len(records)

	mealByDate := map[string]map[string]bool{}
	dishRecordCounts := map[uint]int{}
	monthCounts := map[string]int{}
	seasonCounts := map[string]int{}
	recordInfos := make([]achievementRecordInfo, 0, len(records))

	for _, r := range records {
		s.distinctDishes[r.DishID] = true
		s.recordDates[r.MealDate] = true
		s.mealCounts[r.MealType]++
		if mealByDate[r.MealDate] == nil {
			mealByDate[r.MealDate] = map[string]bool{}
		}
		mealByDate[r.MealDate][r.MealType] = true
		dishRecordCounts[r.DishID]++
		if dishRecordCounts[r.DishID] > s.topDishCount {
			s.topDishCount = dishRecordCounts[r.DishID]
		}

		if r.Mood != "" {
			s.recordMoodCounts[r.Mood]++
		}
		if strings.TrimSpace(r.Remark) != "" {
			s.recordRemarkCount++
		}

		var dish *models.Dish
		if d, ok := dishByID[r.DishID]; ok {
			dish = &d
			collectDishRecordStats(&s, d)
		}

		info := achievementRecordInfo{record: r, dish: dish}
		if parsed, ok := parseMealDate(r.MealDate); ok {
			info.date = parsed
			info.hasDate = true
			if isWeekend(parsed) {
				s.weekendRecords++
				if dish != nil && dish.Difficulty == "hard" {
					s.weekendHard = true
				}
			}
			season := seasonOf(parsed)
			s.seasonSet[season] = true
			seasonCounts[season]++
			monthCounts[parsed.Format("2006-01")]++
		}
		hour := r.CreatedAt.Hour()
		if hour >= 22 || hour < 5 {
			s.hasMidnightRecord = true
		}
		if hour >= 5 && hour < 10 {
			s.hasBreakfastRecord = true
		}
		recordInfos = append(recordInfos, info)
	}

	for _, meals := range mealByDate {
		if meals["lunch"] && meals["dinner"] {
			s.fullDayCount++
		}
	}
	for _, count := range monthCounts {
		if count > s.maxMonthRecords {
			s.maxMonthRecords = count
		}
	}
	for _, count := range seasonCounts {
		if count > s.maxSeasonRecords {
			s.maxSeasonRecords = count
		}
	}
	s.consecutiveQuick = longestRecordRun(recordInfos, func(info achievementRecordInfo) bool {
		return info.dish != nil && isQuickDish(*info.dish)
	})
	s.consecutiveSpicy = longestRecordRun(recordInfos, func(info achievementRecordInfo) bool {
		return info.dish != nil && isSpicyDish(*info.dish)
	})
	s.longestDateStreak = longestDateStreak(s.recordDates)

	var ratings []models.DayRating
	database.DB.Select("home_mood", "mood", "remark", "photos").Find(&ratings)
	for _, r := range ratings {
		if strings.TrimSpace(r.Mood) != "" {
			s.dayRatingCount++
		}
		if strings.TrimSpace(r.Remark) != "" {
			s.dayRemarkCount++
		}
		if strings.TrimSpace(r.HomeMood) != "" {
			s.homeMoodCounts[r.HomeMood]++
		}
		photos := parseStringArray(r.Photos)
		if len(photos) > 0 {
			s.photoDays++
			s.photoCount += len(photos)
		}
	}

	var n int64
	database.DB.Model(&models.Favorite{}).Count(&n)
	s.favoriteCount = int(n)
	database.DB.Model(&models.ShoppingCheck{}).Where("checked = ?", true).Count(&n)
	s.shoppingChecked = int(n)
	database.DB.Model(&models.HomeInventory{}).Where("in_stock = ?", true).Count(&n)
	s.inventoryCount = int(n)

	type eventCount struct {
		EventType string
		Count     int
	}
	var eventCounts []eventCount
	database.DB.Model(&models.AchievementEvent{}).
		Select("event_type, count(*) as count").
		Group("event_type").
		Find(&eventCounts)
	for _, event := range eventCounts {
		s.eventCounts[event.EventType] = event.Count
	}

	return s
}

func collectDishRecordStats(s *achievementSnapshot, d models.Dish) {
	category := strings.TrimSpace(d.Category)
	if category != "" {
		s.categoryCounts[category]++
		s.recordCategories[category] = true
	}
	if d.Difficulty != "" {
		s.difficultyCounts[d.Difficulty]++
	}
	if isQuickDish(d) {
		s.quickCount++
	}
	if d.CookTime > 0 && d.CookTime <= 15 {
		s.fastCount++
	}
	if d.CookTime >= 45 {
		s.slowCount++
	}
	if isHealthyDish(d) {
		s.healthyCount++
	}

	for _, taste := range tasteTokens(d.Taste) {
		s.tasteSet[taste] = true
		if strings.Contains(taste, "辣") {
			s.spicyCount++
		}
		if strings.Contains(taste, "甜") {
			s.sweetCount++
		}
		if strings.Contains(taste, "酸") {
			s.sourCount++
		}
		if strings.Contains(taste, "鲜") {
			s.umamiCount++
		}
		if strings.Contains(taste, "清淡") {
			s.lightCount++
		}
	}
}

func evaluateAchievementCodes(s achievementSnapshot) map[string]bool {
	codes := map[string]bool{}
	unlock := func(code string, ok bool) {
		if ok {
			codes[code] = true
		}
	}

	unlock("first_pick", s.totalRecords >= 1)
	unlock("record_3", s.totalRecords >= 3)
	unlock("record_10", s.totalRecords >= 10)
	unlock("record_20", s.totalRecords >= 20)
	unlock("record_50", s.totalRecords >= 50)
	unlock("record_100", s.totalRecords >= 100)
	unlock("record_200", s.totalRecords >= 200)

	unlock("distinct_3", len(s.distinctDishes) >= 3)
	unlock("distinct_10", len(s.distinctDishes) >= 10)
	unlock("distinct_30", len(s.distinctDishes) >= 30)
	unlock("distinct_50", len(s.distinctDishes) >= 50)
	unlock("hundred_dishes", len(s.distinctDishes) >= 100)

	unlock("streak_3", s.longestDateStreak >= 3)
	unlock("seven_days", s.longestDateStreak >= 7)
	unlock("streak_14", s.longestDateStreak >= 14)
	unlock("streak_30", s.longestDateStreak >= 30)

	unlock("lunch_5", s.mealCounts["lunch"] >= 5)
	unlock("lunch_20", s.mealCounts["lunch"] >= 20)
	unlock("lunch_50", s.mealCounts["lunch"] >= 50)
	unlock("dinner_5", s.mealCounts["dinner"] >= 5)
	unlock("dinner_20", s.mealCounts["dinner"] >= 20)
	unlock("dinner_50", s.mealCounts["dinner"] >= 50)
	unlock("full_day_once", s.fullDayCount >= 1)
	unlock("full_day_3", s.fullDayCount >= 3)
	unlock("full_day_10", s.fullDayCount >= 10)

	unlock("home_food_5", s.categoryCounts["家常菜"] >= 5)
	unlock("home_chef", s.categoryCounts["家常菜"] >= 20)
	unlock("sichuan_rookie", s.categoryCounts["川菜"] >= 5)
	unlock("sichuan_master", s.categoryCounts["川菜"] >= 10)
	unlock("cantonese_rookie", s.categoryCounts["粤菜"] >= 5)
	unlock("cantonese_master", s.categoryCounts["粤菜"] >= 10)
	unlock("soup_lover", s.categoryCounts["汤品"] >= 5)
	unlock("soup_master", s.categoryCounts["汤品"] >= 15)
	unlock("quick_cook", s.consecutiveQuick >= 3)
	unlock("quick_master", s.quickCount >= 20)
	unlock("staple_runner", s.categoryCounts["主食"] >= 10)
	unlock("snack_collector", s.categoryCounts["小食"] >= 5)
	unlock("foodie_explorer", len(s.recordCategories) >= 5)
	unlock("category_collector", len(s.recordCategories) >= 7)

	unlock("easy_10", s.difficultyCounts["easy"] >= 10)
	unlock("medium_10", s.difficultyCounts["medium"] >= 10)
	unlock("hard_first", s.difficultyCounts["hard"] >= 1)
	unlock("hard_10", s.difficultyCounts["hard"] >= 10)
	unlock("weekend_chef", s.weekendHard)
	unlock("sunday_feast", s.weekendRecords >= 3)

	unlock("lightning_10", s.fastCount >= 10)
	unlock("slow_cook_5", s.slowCount >= 5)
	unlock("midnight_snack", s.hasMidnightRecord)
	unlock("breakfast_king", s.hasBreakfastRecord)

	unlock("spicy_life", s.consecutiveSpicy >= 3)
	unlock("spicy_master_10", s.spicyCount >= 10)
	unlock("sweet_tooth_5", s.sweetCount >= 5)
	unlock("sour_explorer_5", s.sourCount >= 5)
	unlock("umami_10", s.umamiCount >= 10)
	unlock("light_diet_10", s.lightCount >= 10)
	unlock("healthy_life", s.healthyCount >= 10)
	unlock("taste_master", len(s.tasteSet) >= 6)

	unlock("first_review", len(s.recordMoodCounts) > 0 || s.recordRemarkCount > 0)
	unlock("review_5", countMapValues(s.recordMoodCounts) >= 5)
	unlock("review_20", countMapValues(s.recordMoodCounts) >= 20)
	unlock("meal_remark_10", s.recordRemarkCount >= 10)
	unlock("yummy_10", s.recordMoodCounts["yum"] >= 10)
	unlock("not_again_3", s.recordMoodCounts["no"] >= 3)
	unlock("first_day_rating", s.dayRatingCount >= 1)
	unlock("day_rating_7", s.dayRatingCount >= 7)

	unlock("happy_home_5", s.homeMoodCounts["happy"] >= 5)
	unlock("tired_care_3", s.homeMoodCounts["tired"] >= 3)
	unlock("lazy_easy_3", s.homeMoodCounts["lazy"] >= 3)
	unlock("spicy_mood_5", s.homeMoodCounts["spicy"] >= 5)
	unlock("healthy_mood_5", s.homeMoodCounts["healthy"] >= 5)
	unlock("seasonal_eater", len(s.seasonSet) >= 4)

	unlock("first_photo", s.photoCount >= 1)
	unlock("photo_wall_3_days", s.photoDays >= 3)
	unlock("photo_collector_10", s.photoCount >= 10)
	unlock("photo_album_30", s.photoCount >= 30)

	unlock("first_favorite", s.favoriteCount >= 1)
	unlock("favorite_5", s.favoriteCount >= 5)
	unlock("favorite_10", s.favoriteCount >= 10)

	unlock("first_shopping_check", s.shoppingChecked >= 1)
	unlock("shopping_10", s.shoppingChecked >= 10)
	unlock("shopping_50", s.shoppingChecked >= 50)
	unlock("pantry_first", s.inventoryCount >= 1)
	unlock("pantry_10", s.inventoryCount >= 10)

	recommendCount := s.eventCounts["recommend_lunch"] + s.eventCounts["recommend_dinner"] + s.eventCounts["recommend_mood"] + s.eventCounts["blind_box"]
	unlock("first_recommend", recommendCount >= 1)
	unlock("recommend_10", recommendCount >= 10)
	unlock("recommend_50", recommendCount >= 50)
	unlock("lunch_recommend_10", s.eventCounts["recommend_lunch"] >= 10)
	unlock("dinner_recommend_10", s.eventCounts["recommend_dinner"] >= 10)
	unlock("mood_recommend_5", s.eventCounts["recommend_mood"] >= 5)
	unlock("blind_box_first", s.eventCounts["blind_box"] >= 1)
	unlock("blind_box_10", s.eventCounts["blind_box"] >= 10)
	unlock("week_plan_first", s.eventCounts["week_plan"] >= 1)
	unlock("week_plan_5", s.eventCounts["week_plan"] >= 5)

	unlock("dish_library_10", s.dishLibraryCount >= 10)
	unlock("dish_library_30", s.dishLibraryCount >= 30)
	unlock("dish_library_50", s.dishLibraryCount >= 50)
	unlock("image_library_10", s.imageDishCount >= 10)
	unlock("video_recipe_1", s.videoDishCount >= 1)
	unlock("ingredient_ready_10", s.ingredientDishCount >= 10)
	unlock("rich_recipe_10", s.stepDishCount >= 10)
	unlock("category_builder_5", len(s.libraryCategorySet) >= 5)

	unlock("top_dish_5", s.topDishCount >= 5)
	unlock("top_dish_10", s.topDishCount >= 10)
	unlock("month_regular_10", s.maxMonthRecords >= 10)
	unlock("season_regular_30", s.maxSeasonRecords >= 30)

	return codes
}

func hasDishImage(d models.Dish) bool {
	if strings.TrimSpace(d.ImageURL) != "" {
		return true
	}
	return jsonArrayLen(d.Images) > 0
}

func jsonArrayLen(raw string) int {
	var arr []any
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return len(arr)
	}
	return 0
}

func parseStringArray(raw string) []string {
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return arr
	}
	return []string{}
}

func tasteTokens(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	for _, sep := range []string{"，", "、", "/", "|", ";", "；", " "} {
		raw = strings.ReplaceAll(raw, sep, ",")
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		result = append(result, part)
	}
	return result
}

func isQuickDish(d models.Dish) bool {
	return strings.Contains(d.Category, "快手") || (d.CookTime > 0 && d.CookTime <= 15)
}

func isSpicyDish(d models.Dish) bool {
	for _, taste := range tasteTokens(d.Taste) {
		if strings.Contains(taste, "辣") {
			return true
		}
	}
	return strings.Contains(d.Name, "辣")
}

func isHealthyDish(d models.Dish) bool {
	if strings.Contains(d.Category, "汤") {
		return true
	}
	for _, taste := range tasteTokens(d.Taste) {
		if strings.Contains(taste, "清淡") || strings.Contains(taste, "鲜") {
			return true
		}
	}
	return false
}

func parseMealDate(raw string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", raw)
	return t, err == nil
}

func isWeekend(t time.Time) bool {
	return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
}

func seasonOf(t time.Time) string {
	switch t.Month() {
	case time.March, time.April, time.May:
		return "spring"
	case time.June, time.July, time.August:
		return "summer"
	case time.September, time.October, time.November:
		return "autumn"
	default:
		return "winter"
	}
}

func longestRecordRun(records []achievementRecordInfo, match func(achievementRecordInfo) bool) int {
	best := 0
	current := 0
	for _, record := range records {
		if match(record) {
			current++
			if current > best {
				best = current
			}
		} else {
			current = 0
		}
	}
	return best
}

func longestDateStreak(dateSet map[string]bool) int {
	if len(dateSet) == 0 {
		return 0
	}

	dates := make([]time.Time, 0, len(dateSet))
	for raw := range dateSet {
		if parsed, ok := parseMealDate(raw); ok {
			dates = append(dates, parsed)
		}
	}
	if len(dates) == 0 {
		return 0
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	best := 1
	current := 1
	for i := 1; i < len(dates); i++ {
		days := int(dates[i].Sub(dates[i-1]).Hours() / 24)
		if days == 0 {
			continue
		}
		if days == 1 {
			current++
		} else {
			current = 1
		}
		if current > best {
			best = current
		}
	}
	return best
}

func countMapValues(m map[string]int) int {
	total := 0
	for _, n := range m {
		total += n
	}
	return total
}
