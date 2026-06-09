package services

import (
	"fmt"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupPlanServiceTestDB(t *testing.T) {
	t.Helper()

	originalDB := database.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.Dish{}, &models.Setting{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	database.DB = db
	t.Cleanup(func() {
		database.DB = originalDB
	})
}

func TestGenerateWeekPlanAllowsSkippingLunch(t *testing.T) {
	setupPlanServiceTestDB(t)

	if err := database.DB.Create(&models.Setting{Key: "lunch_dishes_per_day", Value: "0"}).Error; err != nil {
		t.Fatalf("create lunch setting: %v", err)
	}
	if err := database.DB.Create(&models.Setting{Key: "dinner_dishes_per_day", Value: "1"}).Error; err != nil {
		t.Fatalf("create dinner setting: %v", err)
	}
	for i := 1; i <= 3; i++ {
		dish := models.Dish{
			Name:     fmt.Sprintf("测试菜%d", i),
			MealType: "all",
			Enabled:  true,
		}
		if err := database.DB.Create(&dish).Error; err != nil {
			t.Fatalf("create dish %d: %v", i, err)
		}
	}

	plan, err := GenerateWeekPlan()
	if err != nil {
		t.Fatalf("GenerateWeekPlan() error = %v", err)
	}
	if len(plan.Days) != 7 {
		t.Fatalf("plan day count = %d, want 7", len(plan.Days))
	}
	for _, day := range plan.Days {
		if day.Lunch == nil {
			t.Fatalf("%s lunch should be an empty slice, not nil", day.DayName)
		}
		if len(day.Lunch) != 0 {
			t.Fatalf("%s lunch count = %d, want 0", day.DayName, len(day.Lunch))
		}
		if len(day.Dinner) != 1 {
			t.Fatalf("%s dinner count = %d, want 1", day.DayName, len(day.Dinner))
		}
	}
}

func TestInvalidateWeekPlanCacheClearsStoredAndInMemoryCache(t *testing.T) {
	setupPlanServiceTestDB(t)

	cachedPlan = &WeekPlan{Days: []WeekDayPlan{{Date: "2099-01-01", DayName: "周四"}}}
	cachedWeekKey = "2099-01-01"
	if err := database.DB.Create(&models.Setting{Key: "week_plan_cache", Value: `{"days":[]}`}).Error; err != nil {
		t.Fatalf("create cache setting: %v", err)
	}

	InvalidateWeekPlanCache()

	if cachedPlan != nil {
		t.Fatalf("cachedPlan should be nil after invalidation")
	}
	if cachedWeekKey != "" {
		t.Fatalf("cachedWeekKey = %q, want empty", cachedWeekKey)
	}
	var count int64
	database.DB.Model(&models.Setting{}).Where("`key` = ?", "week_plan_cache").Count(&count)
	if count != 0 {
		t.Fatalf("stored week_plan_cache count = %d, want 0", count)
	}
}

func TestCompactShoppingAmounts(t *testing.T) {
	tests := []struct {
		name    string
		amounts []string
		want    string
	}{
		{name: "same unit sums", amounts: []string{"1个", "2个"}, want: "3个"},
		{name: "different units stay separate", amounts: []string{"1个", "4颗"}, want: "1个+4颗"},
		{name: "different unit order is stable", amounts: []string{"4颗", "1个"}, want: "1个+4颗"},
		{name: "text amounts dedupe", amounts: []string{"适量", "适量", "适量"}, want: "适量"},
		{name: "mixed text and numeric", amounts: []string{"适量", "1块", "适量"}, want: "适量+1块"},
		{name: "existing plus values compact", amounts: []string{"3个+4个"}, want: "7个"},
		{name: "no unit numeric sums", amounts: []string{"1", "2"}, want: "3"},
		{name: "decimal values sum", amounts: []string{"0.5斤", "1斤"}, want: "1.5斤"},
		{name: "empty defaults to suitable amount", amounts: []string{"", "  "}, want: "适量"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compactShoppingAmounts(tt.amounts); got != tt.want {
				t.Fatalf("compactShoppingAmounts(%v) = %q, want %q", tt.amounts, got, tt.want)
			}
		})
	}
}

func TestSortShoppingItems(t *testing.T) {
	items := []ShoppingItem{
		{Name: "tomato", Amount: "2个"},
		{Name: "egg", Amount: "3个", Checked: true},
		{Name: "egg", Amount: "2个"},
		{Name: "apple", Amount: "1个", InStock: true},
	}

	sortShoppingItems(items)

	got := []string{
		items[0].Name + ":" + items[0].Amount,
		items[1].Name + ":" + items[1].Amount,
		items[2].Name + ":" + items[2].Amount,
		items[3].Name + ":" + items[3].Amount,
	}
	want := []string{"egg:2个", "tomato:2个", "egg:3个", "apple:1个"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted[%d] = %q, want %q; full order: %v", i, got[i], want[i], got)
		}
	}
}

func TestShoppingItemPriority(t *testing.T) {
	tests := []struct {
		item ShoppingItem
		want int
	}{
		{item: ShoppingItem{Name: "盐"}, want: 0},
		{item: ShoppingItem{Name: "盐", Checked: true}, want: 1},
		{item: ShoppingItem{Name: "盐", InStock: true}, want: 2},
		{item: ShoppingItem{Name: "盐", Checked: true, InStock: true}, want: 2},
	}

	for _, tt := range tests {
		if got := shoppingItemPriority(tt.item); got != tt.want {
			t.Fatalf("shoppingItemPriority(%+v) = %d, want %d", tt.item, got, tt.want)
		}
	}
}

func TestBuildShoppingCategoryExact(t *testing.T) {
	terms := map[string][]string{
		"蔬菜": {"番茄", "西兰花"},
		"肉类": {"鸡蛋"},
		"无效": {"不会加入"},
	}

	got := buildShoppingCategoryExact(terms)
	if got["番茄"] != "蔬菜" {
		t.Fatalf("番茄 category = %q, want 蔬菜", got["番茄"])
	}
	if got["鸡蛋"] != "肉类" {
		t.Fatalf("鸡蛋 category = %q, want 肉类", got["鸡蛋"])
	}
	if _, exists := got["不会加入"]; exists {
		t.Fatalf("invalid category term should be ignored")
	}
}

func TestClassifyShoppingItem(t *testing.T) {
	overrides := map[string]string{"紫菜": "其他"}
	tests := []struct {
		name string
		want string
	}{
		{name: "番茄", want: "蔬菜"},
		{name: "西红柿", want: "蔬菜"},
		{name: "西兰花", want: "蔬菜"},
		{name: "豆腐", want: "蔬菜"},
		{name: "青椒", want: "蔬菜"},
		{name: "小米辣", want: "蔬菜"},
		{name: "金针菇", want: "蔬菜"},
		{name: "土豆", want: "蔬菜"},
		{name: "玉米", want: "蔬菜"},
		{name: "猪肉末", want: "肉类"},
		{name: "鸡蛋", want: "肉类"},
		{name: "鸡翅", want: "肉类"},
		{name: "牛腩", want: "肉类"},
		{name: "虾仁", want: "肉类"},
		{name: "鱿鱼", want: "肉类"},
		{name: "花椒粉", want: "配料"},
		{name: "豆瓣酱", want: "配料"},
		{name: "辣椒粉", want: "配料"},
		{name: "生抽", want: "配料"},
		{name: "老抽", want: "配料"},
		{name: "料酒", want: "配料"},
		{name: "鸡精", want: "配料"},
		{name: "玉米淀粉", want: "配料"},
		{name: "番茄酱", want: "配料"},
		{name: "蛋黄酱", want: "配料"},
		{name: "面粉", want: "其他"},
		{name: "糯米粉", want: "其他"},
		{name: "粉丝", want: "其他"},
		{name: "鸡蛋面", want: "其他"},
		{name: "可乐", want: "其他"},
		{name: "紫菜", want: "其他"},
		{name: "黄瓜段", want: "蔬菜"},
		{name: "五花肉片", want: "肉类"},
		{name: "黑胡椒碎", want: "配料"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyShoppingItem(tt.name, overrides); got != tt.want {
				t.Fatalf("classifyShoppingItem(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
