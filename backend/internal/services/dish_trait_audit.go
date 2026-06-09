package services

import (
	"encoding/json"
	"fmt"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"sort"
	"strings"
)

type DishTraitAuditReport struct {
	Total        int                        `json:"total"`
	Passed       int                        `json:"passed"`
	IssueDishes  int                        `json:"issue_dishes"`
	IssueCount   int                        `json:"issue_count"`
	AutoTraits   int                        `json:"auto_traits"`
	ManualTraits int                        `json:"manual_traits"`
	FieldCounts  []DishTraitAuditFieldCount `json:"field_counts"`
	Items        []DishTraitAuditItem       `json:"items"`
}

type DishTraitAuditFieldCount struct {
	Field string `json:"field"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

type DishTraitAuditItem struct {
	DishID       uint                  `json:"dish_id"`
	DishName     string                `json:"dish_name"`
	Category     string                `json:"category"`
	TraitSource  string                `json:"trait_source"`
	TraitVersion int                   `json:"trait_version"`
	Issues       []DishTraitAuditIssue `json:"issues"`
}

type DishTraitAuditIssue struct {
	Field    string `json:"field"`
	Label    string `json:"label"`
	Severity string `json:"severity"`
	Current  string `json:"current"`
	Expected string `json:"expected"`
	Message  string `json:"message"`
}

func BuildDishTraitAudit() (DishTraitAuditReport, error) {
	var dishes []models.Dish
	if err := database.DB.Where("deleted_at IS NULL").Order("id ASC").Find(&dishes).Error; err != nil {
		return DishTraitAuditReport{}, err
	}
	return AuditDishTraits(dishes), nil
}

func AuditDishTraits(dishes []models.Dish) DishTraitAuditReport {
	report := DishTraitAuditReport{
		Total:       len(dishes),
		FieldCounts: []DishTraitAuditFieldCount{},
		Items:       []DishTraitAuditItem{},
	}
	fieldCounts := make(map[string]int)

	for _, dish := range dishes {
		if dish.TraitSource == "manual" {
			report.ManualTraits++
		} else {
			report.AutoTraits++
		}

		issues := auditDishTrait(dish)
		if len(issues) == 0 {
			report.Passed++
			continue
		}

		report.IssueDishes++
		report.IssueCount += len(issues)
		for _, issue := range issues {
			fieldCounts[issue.Field]++
		}
		report.Items = append(report.Items, DishTraitAuditItem{
			DishID:       dish.ID,
			DishName:     dish.Name,
			Category:     dish.Category,
			TraitSource:  normalizeAuditTraitSource(dish.TraitSource),
			TraitVersion: dish.TraitVersion,
			Issues:       issues,
		})
	}

	for field, count := range fieldCounts {
		report.FieldCounts = append(report.FieldCounts, DishTraitAuditFieldCount{
			Field: field,
			Label: dishTraitAuditFieldLabel(field),
			Count: count,
		})
	}
	sort.Slice(report.FieldCounts, func(i, j int) bool {
		if report.FieldCounts[i].Count != report.FieldCounts[j].Count {
			return report.FieldCounts[i].Count > report.FieldCounts[j].Count
		}
		return report.FieldCounts[i].Field < report.FieldCounts[j].Field
	})

	return report
}

func auditDishTrait(dish models.Dish) []DishTraitAuditIssue {
	inferred := models.InferDishTraits(dish)
	var issues []DishTraitAuditIssue

	if dish.TraitVersion < models.DishTraitVersion || dish.DishRole == "" || dish.DishRole == "unknown" {
		issues = append(issues, dishTraitAuditIssue("trait_version", "error", fmt.Sprintf("v%d", dish.TraitVersion), fmt.Sprintf("v%d", models.DishTraitVersion), "画像还没有完成升级回填"))
	}

	currentProteins, proteinsOK := parseAuditJSONStrings(dish.ProteinSources)
	inferredProteins := parseJSONStrings(inferred.ProteinSources)
	if !proteinsOK {
		issues = append(issues, dishTraitAuditIssue("protein_sources", "error", dish.ProteinSources, formatAuditList(inferredProteins), "主蛋白字段不是合法列表"))
	} else {
		for _, protein := range inferredProteins {
			if !containsString(currentProteins, protein) {
				issues = append(issues, dishTraitAuditIssue("protein_sources", "error", formatAuditList(currentProteins), formatAuditList(inferredProteins), fmt.Sprintf("疑似缺少%s标记", dishTraitAuditProteinLabel(protein))))
			}
		}
	}

	if shouldAuditDishRole(inferred.DishRole) && dish.DishRole != inferred.DishRole {
		issues = append(issues, dishTraitAuditIssue("dish_role", "error", dishTraitAuditRoleLabel(dish.DishRole), dishTraitAuditRoleLabel(inferred.DishRole), "菜名、分类或食材显示菜单角色可能不一致"))
	}

	if inferred.ServingTemperature != "hot" && dish.ServingTemperature != inferred.ServingTemperature {
		issues = append(issues, dishTraitAuditIssue("serving_temperature", "error", dishTraitAuditTemperatureLabel(dish.ServingTemperature), dishTraitAuditTemperatureLabel(inferred.ServingTemperature), "冷热形态可能不一致"))
	}

	currentMethods, methodsOK := parseAuditJSONStrings(dish.CookingMethods)
	inferredMethods := parseJSONStrings(inferred.CookingMethods)
	if !methodsOK {
		issues = append(issues, dishTraitAuditIssue("cooking_methods", "warning", dish.CookingMethods, formatAuditMethods(inferredMethods), "做法字段不是合法列表"))
	} else {
		for _, method := range inferredMethods {
			if method == "other" {
				continue
			}
			if !containsString(currentMethods, method) {
				issues = append(issues, dishTraitAuditIssue("cooking_methods", "warning", formatAuditMethods(currentMethods), formatAuditMethods(inferredMethods), fmt.Sprintf("疑似缺少%s做法", dishTraitAuditMethodLabel(method))))
			}
		}
	}

	if inferred.CarbLevel > dish.CarbLevel {
		severity := "warning"
		if inferred.CarbLevel >= 2 {
			severity = "error"
		}
		issues = append(issues, dishTraitAuditIssue("carb_level", severity, dishTraitAuditLevelLabel(dish.CarbLevel), dishTraitAuditLevelLabel(inferred.CarbLevel), "主食感可能偏低"))
	}

	if inferred.SpiceLevel > dish.SpiceLevel {
		issues = append(issues, dishTraitAuditIssue("spice_level", "warning", dishTraitAuditLevelLabel(dish.SpiceLevel), dishTraitAuditLevelLabel(inferred.SpiceLevel), "辣度可能偏低"))
	}

	return issues
}

func parseAuditJSONStrings(raw string) ([]string, bool) {
	if strings.TrimSpace(raw) == "" {
		return []string{}, true
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, false
	}
	return values, true
}

func dishTraitAuditIssue(field string, severity string, current string, expected string, message string) DishTraitAuditIssue {
	return DishTraitAuditIssue{
		Field:    field,
		Label:    dishTraitAuditFieldLabel(field),
		Severity: severity,
		Current:  current,
		Expected: expected,
		Message:  message,
	}
}

func normalizeAuditTraitSource(source string) string {
	if source == "manual" {
		return "manual"
	}
	return "auto"
}

func shouldAuditDishRole(role string) bool {
	switch role {
	case "meat", "veg", "soup", "staple":
		return true
	default:
		return false
	}
}

func dishTraitAuditFieldLabel(field string) string {
	switch field {
	case "trait_version":
		return "画像版本"
	case "dish_role":
		return "菜单角色"
	case "protein_sources":
		return "主蛋白来源"
	case "serving_temperature":
		return "冷热形态"
	case "cooking_methods":
		return "做法"
	case "carb_level":
		return "主食感"
	case "spice_level":
		return "辣度"
	default:
		return field
	}
}

func dishTraitAuditProteinLabel(value string) string {
	labels := map[string]string{
		"egg":     "蛋类",
		"pork":    "猪肉",
		"beef":    "牛肉",
		"lamb":    "羊肉",
		"poultry": "禽类",
		"seafood": "水产",
		"soy":     "豆制品",
	}
	if label, ok := labels[value]; ok {
		return label
	}
	return value
}

func dishTraitAuditRoleLabel(value string) string {
	labels := map[string]string{
		"meat":    "荤菜",
		"veg":     "素菜",
		"soup":    "汤",
		"staple":  "主食",
		"side":    "配菜",
		"unknown": "未识别",
		"":        "未填写",
	}
	if label, ok := labels[value]; ok {
		return label
	}
	return value
}

func dishTraitAuditTemperatureLabel(value string) string {
	labels := map[string]string{
		"hot":     "热食",
		"cold":    "冷/凉",
		"mixed":   "冷热均可",
		"unknown": "未识别",
		"":        "未填写",
	}
	if label, ok := labels[value]; ok {
		return label
	}
	return value
}

func dishTraitAuditMethodLabel(value string) string {
	labels := map[string]string{
		"stir_fry": "炒",
		"cold_mix": "凉拌",
		"simmer":   "煮/炖",
		"steam":    "蒸",
		"pan_fry":  "煎",
		"deep_fry": "炸",
		"braise":   "焖/卤",
		"roast":    "烤",
		"other":    "其他",
	}
	if label, ok := labels[value]; ok {
		return label
	}
	return value
}

func dishTraitAuditLevelLabel(value int) string {
	switch value {
	case 0:
		return "低"
	case 1:
		return "中"
	default:
		return "高"
	}
}

func formatAuditList(values []string) string {
	if len(values) == 0 {
		return "无"
	}
	labels := make([]string, 0, len(values))
	for _, value := range values {
		labels = append(labels, dishTraitAuditProteinLabel(value))
	}
	return strings.Join(labels, "、")
}

func formatAuditMethods(values []string) string {
	if len(values) == 0 {
		return "无"
	}
	labels := make([]string, 0, len(values))
	for _, value := range values {
		labels = append(labels, dishTraitAuditMethodLabel(value))
	}
	return strings.Join(labels, "、")
}
