package models

import (
	"encoding/json"
	"strings"
)

const DishTraitVersion = 2

func InferDishTraits(dish Dish) Dish {
	text := dishTraitText(dish)
	tags := dishTraitTags(dish.Tags)
	ingredients := dishTraitIngredientNames(dish.Ingredients)
	ingredientText := strings.Join(ingredients, " ")

	proteins := inferProteinSources(text + " " + ingredientText)
	methods := inferCookingMethods(text)
	role := inferDishRole(dish.Name, dish.Category, text, tags, proteins)
	temperature := inferServingTemperature(text, methods)

	dish.DishRole = role
	dish.ProteinSources = jsonStringList(proteins)
	dish.ServingTemperature = temperature
	dish.CookingMethods = jsonStringList(methods)
	dish.SpiceLevel = inferSpiceLevel(text)
	dish.RichnessLevel = inferRichnessLevel(text, proteins, methods)
	dish.CarbLevel = inferCarbLevel(text + " " + ingredientText)
	dish.TraitSource = "auto"
	dish.TraitVersion = DishTraitVersion
	return dish
}

func dishTraitText(dish Dish) string {
	parts := []string{dish.Name, dish.Category, dish.Taste, dish.Remark}
	parts = append(parts, dishTraitTags(dish.Tags)...)
	parts = append(parts, dishTraitIngredientNames(dish.Ingredients)...)
	parts = append(parts, dishTraitStepTexts(dish.Steps)...)
	return strings.Join(parts, " ")
}

func dishTraitTags(raw string) []string {
	var tags []string
	_ = json.Unmarshal([]byte(raw), &tags)
	return tags
}

func dishTraitIngredientNames(raw string) []string {
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
		if len(result) > 0 {
			return result
		}
	}
	var stringsOnly []string
	if err := json.Unmarshal([]byte(raw), &stringsOnly); err == nil {
		return stringsOnly
	}
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return []string{raw}
}

func dishTraitStepTexts(raw string) []string {
	var steps []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(raw), &steps); err != nil {
		return nil
	}
	result := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.Text != "" {
			result = append(result, step.Text)
		}
	}
	return result
}

func jsonStringList(values []string) string {
	values = uniqueNonEmptyStrings(values)
	data, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func inferProteinSources(text string) []string {
	var result []string
	if containsAnyDishTrait(text, "鸡蛋", "鸭蛋", "鹌鹑蛋", "皮蛋", "咸鸭蛋", "蛋花", "炒蛋", "滑蛋", "金钱蛋", "荷包蛋") {
		result = append(result, "egg")
	}
	if containsAnyDishTrait(text, "猪肉", "肉末", "肉丝", "肉片", "五花肉", "排骨", "猪蹄", "猪肝", "肥肠", "里脊") {
		result = append(result, "pork")
	}
	if containsAnyDishTrait(text, "牛肉", "牛腩", "肥牛", "牛排", "牛腱") {
		result = append(result, "beef")
	}
	if containsAnyDishTrait(text, "羊肉", "羊排") {
		result = append(result, "lamb")
	}
	if containsAnyDishTrait(text, "鸡肉", "鸡腿", "鸡翅", "鸡胸", "鸡爪", "鸡架", "鸭肉", "鸭腿", "鹅肉") {
		result = append(result, "poultry")
	}
	if containsAnyDishTrait(text, "鱼", "虾", "蟹", "蛤", "贝", "鱿", "墨鱼", "章鱼", "海鲜", "花甲", "干贝") {
		result = append(result, "seafood")
	}
	if containsAnyDishTrait(text, "豆腐", "豆皮", "千张", "腐竹", "豆干", "豆泡", "黄豆", "毛豆") {
		result = append(result, "soy")
	}
	return uniqueNonEmptyStrings(result)
}

// IsSoupName reports whether a dish is a soup in the meal-slot sense (a
// drinkable accompaniment). Only the category and the name SUFFIX count:
// mid-name 汤 (酸汤鱼, 上汤娃娃菜, 汤圆), tags, remark, ingredient and step
// texts deliberately do not — those describe form or technique, not the
// dish's role on the table.
func IsSoupName(name, category string) bool {
	return strings.Contains(category, "汤") ||
		strings.HasSuffix(name, "汤") ||
		strings.HasSuffix(name, "羹")
}

func inferDishRole(name string, category string, text string, tags []string, proteins []string) string {
	if IsSoupName(name, category) {
		return "soup"
	}
	if containsAnyDishTrait(text, "主食", "米饭", "炒饭", "盖饭", "面条", "米线", "米粉", "冷面", "饺子", "馄饨", "馒头", "饼", "粥", "粉丝", "河粉") {
		return "staple"
	}
	if len(proteins) > 0 && !onlySoyProtein(proteins) {
		return "meat"
	}
	for _, tag := range tags {
		if tag == "素菜" || tag == "纯素" || tag == "蔬菜" || tag == "青菜" {
			return "veg"
		}
	}
	if containsAnyDishTrait(text, "青菜", "白菜", "生菜", "菠菜", "黄瓜", "茄子", "土豆", "番茄", "豆腐", "蘑菇", "木耳", "莲藕", "萝卜", "西兰花", "花菜") {
		return "veg"
	}
	return "side"
}

func onlySoyProtein(proteins []string) bool {
	return len(proteins) == 1 && proteins[0] == "soy"
}

func inferServingTemperature(text string, methods []string) string {
	if containsStringValue(methods, "cold_mix") || containsAnyDishTrait(text, "凉菜", "凉拌", "冷食", "冷面", "过凉", "冰水", "放凉", "冷片") {
		return "cold"
	}
	if containsAnyDishTrait(text, "冷食或温食") {
		return "mixed"
	}
	return "hot"
}

func inferCookingMethods(text string) []string {
	var result []string
	if containsAnyDishTrait(text, "凉拌", "凉菜", "拌匀", "蘸酱", "冷食") {
		result = append(result, "cold_mix")
	}
	if containsAnyDishTrait(text, "炒", "煸", "爆香") {
		result = append(result, "stir_fry")
	}
	if containsAnyDishTrait(text, "汤", "炖", "煮", "煲", "羹", "例汤") {
		result = append(result, "simmer")
	}
	if containsAnyDishTrait(text, "蒸") {
		result = append(result, "steam")
	}
	if containsAnyDishTrait(text, "炸", "油炸") {
		result = append(result, "deep_fry")
	}
	if containsAnyDishTrait(text, "煎") {
		result = append(result, "pan_fry")
	}
	if containsAnyDishTrait(text, "烤", "焗") {
		result = append(result, "roast")
	}
	if containsAnyDishTrait(text, "卤", "红烧", "焖") {
		result = append(result, "braise")
	}
	if len(result) == 0 {
		result = append(result, "other")
	}
	return uniqueNonEmptyStrings(result)
}

func inferSpiceLevel(text string) int {
	if containsAnyDishTrait(text, "麻辣", "香辣", "酸辣", "鲜辣", "小米辣", "干辣椒", "辣椒油", "剁椒") {
		return 2
	}
	if containsAnyDishTrait(text, "辣", "椒") {
		return 1
	}
	return 0
}

func inferRichnessLevel(text string, proteins []string, methods []string) int {
	if containsStringValue(methods, "deep_fry") || containsAnyDishTrait(text, "干锅", "红烧", "肥肠", "五花肉", "猪蹄", "油炸", "重油") {
		return 2
	}
	if len(proteins) > 0 || containsStringValue(methods, "pan_fry") || containsStringValue(methods, "braise") {
		return 1
	}
	return 0
}

func inferCarbLevel(text string) int {
	if containsAnyDishTrait(text, "米饭", "炒饭", "盖饭", "面条", "米线", "米粉", "冷面", "馒头", "饼", "粥", "粉丝", "河粉", "饺子", "馄饨") {
		return 2
	}
	if containsAnyDishTrait(text, "土豆", "红薯", "玉米", "南瓜", "年糕", "粉条") {
		return 1
	}
	return 0
}

func containsAnyDishTrait(text string, keywords ...string) bool {
	for _, keyword := range keywords {
		if keyword != "" && strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func containsStringValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
