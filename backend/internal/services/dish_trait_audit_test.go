package services

import (
	"ninimenu/internal/models"
	"testing"
)

func TestAuditDishTraitsFlagsMissingEggProtein(t *testing.T) {
	dishes := []models.Dish{{
		ID:                 1,
		Name:               "番茄炒蛋",
		Category:           "家常菜",
		Ingredients:        `[{"name":"番茄","amount":"2个"},{"name":"鸡蛋","amount":"3个"}]`,
		DishRole:           "veg",
		ProteinSources:     `[]`,
		ServingTemperature: "hot",
		CookingMethods:     `["stir_fry"]`,
		TraitSource:        "manual",
		TraitVersion:       models.DishTraitVersion,
	}}

	report := AuditDishTraits(dishes)

	if report.IssueDishes != 1 || report.IssueCount == 0 {
		t.Fatalf("AuditDishTraits() issue summary = %+v, want missing egg issue", report)
	}
	if len(report.Items) != 1 {
		t.Fatalf("AuditDishTraits() item count = %d, want 1", len(report.Items))
	}
	if !auditItemHasField(report.Items[0], "protein_sources") {
		t.Fatalf("AuditDishTraits() issues = %+v, want protein_sources issue", report.Items[0].Issues)
	}
}

func TestAuditDishTraitsPassesCorrectTomatoEgg(t *testing.T) {
	dishes := []models.Dish{models.InferDishTraits(models.Dish{
		ID:          1,
		Name:        "番茄炒蛋",
		Category:    "家常菜",
		Ingredients: `[{"name":"番茄","amount":"2个"},{"name":"鸡蛋","amount":"3个"}]`,
		Steps:       `[{"text":"鸡蛋炒散，番茄炒出汁后回锅"}]`,
	})}

	report := AuditDishTraits(dishes)

	if report.IssueCount != 0 || len(report.Items) != 0 {
		t.Fatalf("AuditDishTraits() = %+v, want no issues", report)
	}
	if report.Passed != 1 {
		t.Fatalf("AuditDishTraits() passed = %d, want 1", report.Passed)
	}
}

func TestAuditDishTraitsFlagsSoupColdAndStapleMismatches(t *testing.T) {
	dishes := []models.Dish{
		{
			ID:                 1,
			Name:               "紫菜蛋花汤",
			Category:           "汤品",
			Ingredients:        `[{"name":"紫菜","amount":"适量"},{"name":"鸡蛋","amount":"1个"}]`,
			DishRole:           "side",
			ProteinSources:     `[]`,
			ServingTemperature: "hot",
			CookingMethods:     `["other"]`,
			TraitSource:        "manual",
			TraitVersion:       models.DishTraitVersion,
		},
		{
			ID:                 2,
			Name:               "凉拌米线",
			Category:           "主食",
			Ingredients:        `[{"name":"米线","amount":"200克"}]`,
			DishRole:           "veg",
			ProteinSources:     `[]`,
			ServingTemperature: "hot",
			CookingMethods:     `["other"]`,
			CarbLevel:          0,
			TraitSource:        "manual",
			TraitVersion:       models.DishTraitVersion,
		},
	}

	report := AuditDishTraits(dishes)

	if report.IssueDishes != 2 {
		t.Fatalf("AuditDishTraits() issue dishes = %d, want 2: %+v", report.IssueDishes, report.Items)
	}
	if !auditItemHasField(report.Items[0], "dish_role") || !auditItemHasField(report.Items[0], "protein_sources") {
		t.Fatalf("soup issues = %+v, want dish_role and protein_sources", report.Items[0].Issues)
	}
	if !auditItemHasField(report.Items[1], "serving_temperature") || !auditItemHasField(report.Items[1], "carb_level") {
		t.Fatalf("staple/cold issues = %+v, want serving_temperature and carb_level", report.Items[1].Issues)
	}
}

func auditItemHasField(item DishTraitAuditItem, field string) bool {
	for _, issue := range item.Issues {
		if issue.Field == field {
			return true
		}
	}
	return false
}
