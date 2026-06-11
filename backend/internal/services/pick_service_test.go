package services

import (
	"testing"

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
	clearSoup := models.Dish{Name: "番茄蛋花汤", Category: "汤品", Taste: "酸甜", RichnessLevel: 0, DishRole: "soup", TraitSource: "manual", TraitVersion: models.DishTraitVersion}

	if matchesTomorrowProfile(richSoup, "light") {
		t.Errorf("rich soup (richness 2) should not match light profile")
	}
	if !matchesTomorrowProfile(clearSoup, "light") {
		t.Errorf("clear soup (richness 0) should match light profile")
	}
	if got, want := tomorrowDishScore(clearSoup, "light"), tomorrowDishScore(richSoup, "light"); got <= want {
		t.Errorf("light score clear=%d rich=%d, want clear > rich", got, want)
	}
}
