package services

import "ninimenu/internal/models"

func InferDishTraits(dish models.Dish) models.Dish {
	return models.InferDishTraits(dish)
}

func ApplyDishTraitsForSave(dish models.Dish, manual bool) models.Dish {
	if !manual {
		return models.InferDishTraits(dish)
	}
	dish.TraitSource = "manual"
	dish.TraitVersion = models.DishTraitVersion
	return normalizeManualDishTraits(dish)
}

func ensureDishTraits(dish models.Dish) models.Dish {
	if dish.TraitSource == "manual" && dish.TraitVersion >= models.DishTraitVersion {
		return normalizeManualDishTraits(dish)
	}
	if dish.TraitSource == "manual" {
		return normalizeManualDishTraits(dish)
	}
	if dish.DishRole == "" || dish.DishRole == "unknown" || dish.TraitVersion < models.DishTraitVersion {
		return models.InferDishTraits(dish)
	}
	return normalizeManualDishTraits(dish)
}

func normalizeManualDishTraits(dish models.Dish) models.Dish {
	if dish.DishRole == "" {
		dish.DishRole = "side"
	}
	if dish.ProteinSources == "" {
		dish.ProteinSources = "[]"
	}
	if dish.ServingTemperature == "" {
		dish.ServingTemperature = "hot"
	}
	if dish.CookingMethods == "" {
		dish.CookingMethods = "[]"
	}
	if dish.TraitSource == "" {
		dish.TraitSource = "auto"
	}
	if dish.TraitVersion == 0 {
		dish.TraitVersion = models.DishTraitVersion
	}
	return dish
}
