package services

import (
	"encoding/json"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"strings"
)

const blockedIngredientsSettingKey = "blocked_ingredients"

func GetBlockedIngredients() []string {
	var setting models.Setting
	if err := database.DB.Where("`key` = ?", blockedIngredientsSettingKey).First(&setting).Error; err != nil {
		return nil
	}
	var words []string
	if json.Unmarshal([]byte(setting.Value), &words) != nil {
		return nil
	}
	result := make([]string, 0, len(words))
	for _, word := range words {
		word = strings.TrimSpace(word)
		if word != "" {
			result = append(result, word)
		}
	}
	return result
}

// FilterBlockedDishes removes dishes containing any blocked ingredient word
// in their name, ingredients or seasonings. Shared by every recommendation
// entry point so 忌口 applies app-wide.
func FilterBlockedDishes(dishes []models.Dish) []models.Dish {
	blocked := GetBlockedIngredients()
	if len(blocked) == 0 {
		return dishes
	}
	result := make([]models.Dish, 0, len(dishes))
	for _, dish := range dishes {
		if !dishContainsBlockedWord(dish, blocked) {
			result = append(result, dish)
		}
	}
	return result
}

func dishContainsBlockedWord(dish models.Dish, blocked []string) bool {
	parts := []string{dish.Name}
	parts = append(parts, dishIngredientNames(dish.Ingredients)...)
	parts = append(parts, dishIngredientNames(dish.Seasonings)...)
	text := strings.Join(parts, " ")
	for _, word := range blocked {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func dishIngredientNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var named []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(raw), &named); err == nil {
		result := make([]string, 0, len(named))
		for _, item := range named {
			if item.Name != "" {
				result = append(result, item.Name)
			}
		}
		return result
	}
	var plain []string
	if err := json.Unmarshal([]byte(raw), &plain); err == nil {
		return plain
	}
	return nil
}
