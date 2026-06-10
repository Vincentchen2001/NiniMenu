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
