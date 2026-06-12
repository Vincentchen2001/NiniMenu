package services

import (
	"testing"
	"time"

	"ninimenu/internal/models"
)

func TestTomorrowProfileSoup(t *testing.T) {
	soup := models.Dish{Name: "玉米排骨汤", Category: "汤品", Tags: `["汤品"]`, Steps: `[{"text":"冷水下排骨，炖一小时"}]`}
	soup = InferDishTraits(soup)
	stirFry := models.Dish{Name: "青椒肉丝", Tags: `["家常菜"]`, Ingredients: `[{"name":"猪肉","amount":"100g"}]`, Steps: `[{"text":"热油快炒"}]`}
	stirFry = InferDishTraits(stirFry)

	if normalizePlanProfile("soup") != "soup" {
		t.Fatalf("normalizePlanProfile(soup) = %q, want soup", normalizePlanProfile("soup"))
	}
	if !matchesTomorrowProfile(soup, "soup") {
		t.Fatalf("soup dish should match soup profile")
	}
	if matchesTomorrowProfile(stirFry, "soup") {
		t.Fatalf("stir-fry should not match soup profile")
	}
	if tomorrowDishScore(soup, "soup") <= tomorrowDishScore(stirFry, "soup") {
		t.Fatalf("soup dish should outscore stir-fry under soup profile: %d vs %d",
			tomorrowDishScore(soup, "soup"), tomorrowDishScore(stirFry, "soup"))
	}
}

func TestLightProfileSoupRespectsRichness(t *testing.T) {
	richSoup := models.Dish{Name: "猪肚鸡汤", Category: "汤品", Taste: "浓香", RichnessLevel: 2, DishRole: "soup", TraitSource: "manual", TraitVersion: models.DishTraitVersion}
	// 酸甜 taste matches no light keyword — only the soup clause can let it in.
	clearSoup := models.Dish{Name: "番茄蛋花汤", Category: "汤品", Taste: "酸甜", RichnessLevel: 1, DishRole: "soup", TraitSource: "manual", TraitVersion: models.DishTraitVersion}

	if matchesTomorrowProfile(richSoup, "light") {
		t.Errorf("rich soup (richness 2) should not match light profile")
	}
	if !matchesTomorrowProfile(clearSoup, "light") {
		t.Errorf("clear soup (richness 0) should match light profile")
	}
	if got, want := tomorrowDishScore(clearSoup, "light"), tomorrowDishScore(richSoup, "light"); got <= want {
		t.Errorf("light score clear=%d rich=%d, want clear > rich", got, want)
	}

	// Factory-realistic 猪肚鸡汤: seeded soups carry no explicit richness
	// (inference yields 0) — the taste guard alone must exclude it.
	factoryRichSoup := models.Dish{Name: "猪肚鸡汤", Category: "汤品", Taste: "浓香", RichnessLevel: 0, DishRole: "soup", TraitSource: "manual", TraitVersion: models.DishTraitVersion}
	if matchesTomorrowProfile(factoryRichSoup, "light") {
		t.Errorf("factory rich soup (浓香, richness 0) should not match light profile")
	}
}

func TestPickPenaltyAdjustment(t *testing.T) {
	favs := map[string]int{"家常菜": 2}

	fav := models.Dish{Name: "红烧肉", Category: "家常菜", Favorite: true, DishRole: "meat"}
	if got := pickPenaltyAdjustment(fav, favs, false, -1); got != 0 {
		t.Errorf("favorite dish = %d, want 0", got)
	}
	plain := models.Dish{Name: "清炒时蔬", Category: "家常菜", DishRole: "veg"}
	if got := pickPenaltyAdjustment(plain, favs, false, -1); got != -12 {
		t.Errorf("non-favorite = %d, want -12", got)
	}
	unfamiliar := models.Dish{Name: "大盘鸡", Category: "新疆菜", DishRole: "meat"}
	if got := pickPenaltyAdjustment(unfamiliar, favs, false, -1); got != -37 {
		t.Errorf("unfamiliar category = %d, want -37", got)
	}
	uncategorized := models.Dish{Name: "随手菜", Category: "", DishRole: "veg"}
	if got := pickPenaltyAdjustment(uncategorized, favs, false, -1); got != -12 {
		t.Errorf("empty category = %d, want -12 (no cuisine layer)", got)
	}
	slowSoup := models.Dish{Name: "莲藕排骨汤", Category: "家常菜", DishRole: "soup", CookTime: 90}
	if got := pickPenaltyAdjustment(slowSoup, favs, false, -1); got != -42 {
		t.Errorf("weekday slow soup = %d, want -42 (-12 -30)", got)
	}
	if got := pickPenaltyAdjustment(slowSoup, favs, true, -1); got != -12 {
		t.Errorf("weekend slow soup = %d, want -12", got)
	}
	quickSoup := models.Dish{Name: "紫菜蛋花汤", Category: "家常菜", DishRole: "soup", CookTime: 8}
	if got := pickPenaltyAdjustment(quickSoup, favs, false, -1); got != -12 {
		t.Errorf("weekday quick soup = %d, want -12 (no slow penalty)", got)
	}
	hardSoup := models.Dish{Name: "佛跳墙", Category: "家常菜", DishRole: "soup", CookTime: 30, Difficulty: "hard"}
	if got := pickPenaltyAdjustment(hardSoup, favs, false, -1); got != -42 {
		t.Errorf("weekday hard soup = %d, want -42 (-12 -30 via difficulty)", got)
	}

	// Freshness decay cases — delta relative to daysSinceLast=-1 (baseline, no decay).
	// round(30×(14-d)/14): d=0 → round(30)=30; d=5 → round(30×9/14)=round(19.2857)=19;
	// d=14 → round(0)=0.
	base := models.Dish{Name: "红烧肉", Category: "家常菜", Favorite: true, DishRole: "meat"}
	baseline := pickPenaltyAdjustment(base, favs, false, -1)
	if got := pickPenaltyAdjustment(base, favs, false, 0); got != baseline-30 {
		t.Errorf("daysSinceLast=0 decay = %d, want baseline-30 (%d)", got, baseline-30)
	}
	if got := pickPenaltyAdjustment(base, favs, false, 5); got != baseline-19 {
		t.Errorf("daysSinceLast=5 decay = %d, want baseline-19 (%d)", got, baseline-19)
	}
	if got := pickPenaltyAdjustment(base, favs, false, 14); got != baseline {
		t.Errorf("daysSinceLast=14 decay = %d, want baseline (%d, no penalty at window edge)", got, baseline)
	}
	if got := pickPenaltyAdjustment(base, favs, false, -1); got != baseline {
		t.Errorf("daysSinceLast=-1 decay = %d, want baseline (%d, no decay)", got, baseline)
	}
}

// tomorrowDishScore is shared by the week plan (which applies the v3
// penalties via menu rules) and the pick path (which applies them via
// pickPenaltyAdjustment). If a penalty term ever leaks into this function,
// the week plan double-counts — pin favorite's only influence to the
// long-standing +12.
func TestTomorrowDishScoreHasNoPenaltyTerms(t *testing.T) {
	base := models.Dish{Name: "青椒肉丝", Category: "新疆菜", DishRole: "meat", TraitSource: "manual", TraitVersion: models.DishTraitVersion}
	fav := base
	fav.Favorite = true
	if got := tomorrowDishScore(fav, "balanced") - tomorrowDishScore(base, "balanced"); got != 12 {
		t.Errorf("favorite delta inside tomorrowDishScore = %d, want exactly 12 (no penalty terms)", got)
	}
}

func TestSortTomorrowPoolPrefersFavorites(t *testing.T) {
	favs := map[string]int{"家常菜": 1}
	pool := []models.Dish{
		{ID: 1, Name: "大盘鸡", Category: "新疆菜", DishRole: "meat", TraitSource: "manual", TraitVersion: models.DishTraitVersion},
		{ID: 2, Name: "红烧肉", Category: "家常菜", Favorite: true, DishRole: "meat", TraitSource: "manual", TraitVersion: models.DishTraitVersion},
	}
	sortTomorrowPool(pool, "balanced", favs, false, nil)
	if pool[0].Name != "红烧肉" {
		t.Fatalf("pool[0] = %s, want 红烧肉 (favorite of a familiar category first)", pool[0].Name)
	}
}

func TestPickTomorrowDishesAppliesFreshnessDecay(t *testing.T) {
	setupPlanServiceTestDB(t)
	// Pin the injectable clock: PickTomorrowDishes sits entirely on planNow,
	// so fixture and pick share one instant — no midnight-crossing flake
	// between fixture creation and the planNow() read.
	now := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	withPlanNow(t, now)
	a := createDishForPlanTest(t, "明日鸡块A", "", "鸡肉")
	b := createDishForPlanTest(t, "明日鸡块B", "", "鸡肉")
	// A 五天前（2026-06-03）打卡；明天（06-09）参照下 d=6 → 镜像扣 round(30×8/14)=17 分
	five := now.AddDate(0, 0, -5).Format("2006-01-02")
	mustCreate(t, &models.MealRecord{DishID: a.ID, DishName: a.Name, MealType: "dinner", MealDate: five})

	picks, err := PickTomorrowDishes(TomorrowPickOptions{Count: 2})
	if err != nil {
		t.Fatalf("PickTomorrowDishes() error = %v", err)
	}
	if len(picks) != 2 || picks[0].ID != b.ID {
		t.Errorf("五天前吃过的 A 应排在 B 之后，got %+v", picks)
	}
}
