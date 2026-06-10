package services

import (
	"math/rand"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"sort"
	"strings"
	"time"
)

type TomorrowPickOptions struct {
	MealType   string
	Profile    string
	Count      int
	ExcludeIDs []uint
}

func PickDishes(mealType string, count int, excludeRecent bool) ([]models.Dish, error) {
	var dishes []models.Dish
	query := database.DB.Where("enabled = ?", true)

	if mealType != "" {
		query = query.Where("meal_type IN ?", []string{mealType, "all"})
	}

	query.Find(&dishes)
	dishes = FilterBlockedDishes(dishes)

	if len(dishes) == 0 {
		return nil, nil
	}

	if excludeRecent {
		dishes = filterRecent(dishes, RecommendationCooldownDays())
	}

	if len(dishes) == 0 {
		var allDishes []models.Dish
		database.DB.Where("enabled = ?", true).Find(&allDishes)
		allDishes = FilterBlockedDishes(allDishes)
		if mealType != "" {
			var filtered []models.Dish
			for _, d := range allDishes {
				if d.MealType == mealType || d.MealType == "all" {
					filtered = append(filtered, d)
				}
			}
			dishes = filtered
		} else {
			dishes = allDishes
		}
	}

	if len(dishes) == 0 {
		return nil, nil
	}

	weighted := buildWeightedIndices(dishes)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(weighted), func(i, j int) {
		weighted[i], weighted[j] = weighted[j], weighted[i]
	})

	seen := make(map[int]bool)
	var result []models.Dish
	for _, idx := range weighted {
		if !seen[idx] {
			seen[idx] = true
			result = append(result, dishes[idx])
			if len(result) >= count {
				break
			}
		}
	}

	return result, nil
}

func PickTomorrowDishes(opts TomorrowPickOptions) ([]models.Dish, error) {
	count := opts.Count
	if count < 1 {
		count = 1
	}
	if count > 10 {
		count = 10
	}

	mealType := strings.TrimSpace(opts.MealType)
	if mealType != "lunch" && mealType != "dinner" {
		mealType = ""
	}

	profile := strings.TrimSpace(opts.Profile)
	if profile == "" {
		profile = "balanced"
	}

	var dishes []models.Dish
	query := database.DB.Where("enabled = ?", true)
	if mealType != "" {
		query = query.Where("meal_type IN ?", []string{mealType, "all", ""})
	}
	query.Find(&dishes)
	dishes = FilterBlockedDishes(dishes)

	if len(dishes) == 0 {
		return nil, nil
	}

	excluded := make(map[uint]bool, len(opts.ExcludeIDs))
	for _, id := range opts.ExcludeIDs {
		if id > 0 {
			excluded[id] = true
		}
	}

	recent := recentDishIDMap(RecommendationCooldownDays())
	pool := filterTomorrowPool(dishes, profile, excluded, recent, true)
	if len(pool) == 0 {
		pool = filterTomorrowPool(dishes, profile, excluded, recent, false)
	}
	if len(pool) == 0 {
		pool = filterTomorrowPool(dishes, "balanced", excluded, recent, false)
	}
	if len(pool) == 0 {
		pool = filterTomorrowPool(dishes, "balanced", map[uint]bool{}, recent, false)
	}

	sortTomorrowPool(pool, profile)
	if len(pool) > count {
		pool = pool[:count]
	}
	return pool, nil
}

func filterTomorrowPool(dishes []models.Dish, profile string, excluded map[uint]bool, recent map[uint]bool, strictProfile bool) []models.Dish {
	var result []models.Dish
	for _, d := range dishes {
		if excluded[d.ID] {
			continue
		}
		if recent[d.ID] {
			continue
		}
		if strictProfile && !matchesTomorrowProfile(d, profile) {
			continue
		}
		result = append(result, d)
	}
	if len(result) > 0 {
		return result
	}

	for _, d := range dishes {
		if excluded[d.ID] {
			continue
		}
		if strictProfile && !matchesTomorrowProfile(d, profile) {
			continue
		}
		result = append(result, d)
	}
	return result
}

func matchesTomorrowProfile(d models.Dish, profile string) bool {
	switch profile {
	case "quick":
		return d.Difficulty == "easy" || d.CookTime > 0 && d.CookTime <= 25 || strings.Contains(d.Category, "快手")
	case "light":
		return containsTaste(d.Taste, "清淡") || containsTaste(d.Taste, "鲜") || strings.Contains(d.Category, "汤")
	case "spicy":
		return containsTaste(d.Taste, "辣") || strings.Contains(d.Category, "川菜") || strings.Contains(d.Category, "湘菜") || strings.Contains(d.Category, "贵州菜") || strings.Contains(d.Name, "辣")
	case "favorite":
		return d.Favorite
	case "soup":
		return isSoupDish(d) || d.DishRole == "soup"
	default:
		return true
	}
}

func sortTomorrowPool(dishes []models.Dish, profile string) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(dishes), func(i, j int) {
		dishes[i], dishes[j] = dishes[j], dishes[i]
	})

	sort.SliceStable(dishes, func(i, j int) bool {
		a, b := tomorrowDishScore(dishes[i], profile), tomorrowDishScore(dishes[j], profile)
		if a != b {
			return a > b
		}
		return dishes[i].ID < dishes[j].ID
	})
}

func tomorrowDishScore(d models.Dish, profile string) int {
	score := 0
	if d.Favorite {
		score += 12
	}
	if matchesTomorrowProfile(d, profile) {
		score += 40
	}
	switch profile {
	case "quick":
		if d.Difficulty == "easy" {
			score += 18
		}
		if d.CookTime > 0 {
			score += maxInt(0, 35-d.CookTime)
		}
	case "light":
		if containsTaste(d.Taste, "清淡") {
			score += 18
		}
		if containsTaste(d.Taste, "鲜") {
			score += 10
		}
		if strings.Contains(d.Category, "汤") {
			score += 8
		}
	case "spicy":
		if containsTaste(d.Taste, "辣") {
			score += 18
		}
		if strings.Contains(d.Category, "川菜") || strings.Contains(d.Category, "湘菜") || strings.Contains(d.Category, "贵州菜") {
			score += 8
		}
	case "favorite":
		if d.Favorite {
			score += 30
		}
	case "soup":
		if isSoupDish(d) || d.DishRole == "soup" {
			score += 18
		}
		if containsString(parseJSONStrings(d.CookingMethods), "simmer") {
			score += 8
		}
	default:
		if d.Difficulty == "easy" {
			score += 6
		}
		if d.CookTime > 0 && d.CookTime <= 35 {
			score += 4
		}
	}
	return score
}

func recentDishIDMap(days int) map[uint]bool {
	if days <= 0 {
		return map[uint]bool{}
	}
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	// Pluck into separate slices: GORM resets the destination slice on each
	// query, so reusing one slice would drop the earlier results.
	var eatenIDs, recommendedIDs []uint
	database.DB.Model(&models.MealRecord{}).
		Where("meal_date >= ?", since).
		Pluck("dish_id", &eatenIDs)
	database.DB.Model(&models.DishRecommendation{}).
		Where("planned_date >= ?", since).
		Pluck("dish_id", &recommendedIDs)

	result := make(map[uint]bool, len(eatenIDs)+len(recommendedIDs))
	for _, id := range eatenIDs {
		result[id] = true
	}
	for _, id := range recommendedIDs {
		result[id] = true
	}
	return result
}

func RecommendationCooldownDays() int {
	return getSettingInt("repeat_days", 3)
}

func containsTaste(raw string, target string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || target == "" {
		return false
	}
	for _, sep := range []string{"，", "、", "/", "|", ";", "；", " "} {
		raw = strings.ReplaceAll(raw, sep, ",")
	}
	for _, part := range strings.Split(raw, ",") {
		if strings.Contains(strings.TrimSpace(part), target) {
			return true
		}
	}
	return strings.Contains(raw, target)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func filterRecent(dishes []models.Dish, days int) []models.Dish {
	recentMap := recentDishIDMap(days)

	var filtered []models.Dish
	for _, d := range dishes {
		if !recentMap[d.ID] {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

func buildWeightedIndices(dishes []models.Dish) []int {
	indices := make([]int, 0)
	for i, d := range dishes {
		w := 1
		if d.Favorite {
			w += 2
		}
		for j := 0; j < w; j++ {
			indices = append(indices, i)
		}
	}
	return indices
}

func GetRandomQuote(scene string) string {
	var quotes []models.Quote
	query := database.DB.Where("enabled = ?", true)
	if scene != "" {
		query = query.Where("scene = ?", scene)
	}
	query.Find(&quotes)

	if len(quotes) == 0 {
		database.DB.Where("enabled = ?", true).Find(&quotes)
	}

	if len(quotes) == 0 {
		return "今天吃点什么好呢？"
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return quotes[r.Intn(len(quotes))].Content
}
