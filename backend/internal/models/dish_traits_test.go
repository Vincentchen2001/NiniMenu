package models

import "testing"

func TestIsSoupName(t *testing.T) {
	tests := []struct {
		name     string
		dishName string
		category string
		want     bool
	}{
		{"category soup", "番茄豆腐汤", "汤品", true},
		{"name suffix 汤 in cuisine category", "莲藕花生排骨汤", "粤菜", true},
		{"name suffix 羹", "西湖牛肉羹", "杭帮菜", true},
		{"mid-name 汤 is not soup", "酸汤鱼", "贵州菜", false},
		{"mid-name 汤 staple", "黑芝麻汤圆", "主食", false},
		{"上汤 prefix veg dish", "上汤娃娃菜", "粤菜", false},
		{"煲 is not soup", "咸鱼鸡粒茄子煲", "粤菜", false},
		{"plain meat dish", "红烧排骨", "家常菜", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSoupName(tt.dishName, tt.category); got != tt.want {
				t.Fatalf("IsSoupName(%q, %q) = %v, want %v", tt.dishName, tt.category, got, tt.want)
			}
		})
	}
}

// TestInferDishTraitsSoupRole pins the boundary table from the design spec:
// step/remark/tag text and mid-name 汤 must stop producing role=soup, while
// suffix-named and 汤品-categorized soups keep it.
func TestInferDishTraitsSoupRole(t *testing.T) {
	tests := []struct {
		name string
		dish Dish
		want string
	}{
		{
			name: "step text 汤汁 no longer makes soup",
			dish: Dish{Name: "红烧排骨", Category: "家常菜", Taste: "酱香", Tags: `["家常菜","硬菜"]`,
				Ingredients: `[{"name":"猪肋排","amount":"600克"}]`,
				Steps:       `[{"text":"倒热水没过排骨，小火炖35分钟，开盖收浓汤汁。","time":35}]`},
			want: "meat",
		},
		{
			name: "remark 原汤 no longer makes soup",
			dish: Dish{Name: "清蒸鲈鱼", Category: "粤菜", Taste: "鲜", Remark: "蒸好后倒掉原汤，腥味更轻。",
				Ingredients: `[{"name":"鲈鱼","amount":"1条"}]`},
			want: "meat",
		},
		{
			name: "mid-name 汤 cuisine main",
			dish: Dish{Name: "酸汤鱼", Category: "贵州菜", Ingredients: `[{"name":"草鱼","amount":"1条"}]`},
			want: "meat",
		},
		{
			name: "汤圆 is staple",
			dish: Dish{Name: "黑芝麻汤圆", Category: "主食", Ingredients: `[{"name":"糯米粉","amount":"200克"}]`},
			want: "staple",
		},
		{
			name: "煲仔饭 is staple",
			dish: Dish{Name: "腊味煲仔饭", Category: "粤菜", Ingredients: `[{"name":"米饭","amount":"2碗"},{"name":"腊肠","amount":"2根"}]`},
			want: "staple",
		},
		{
			name: "tag 汤品 no longer makes soup",
			dish: Dish{Name: "开水白菜", Category: "川菜", Tags: `["川菜","晚餐","汤品"]`,
				Ingredients: `[{"name":"白菜","amount":"1棵"}]`},
			want: "veg",
		},
		{
			name: "tag 汤品 poultry main",
			dish: Dish{Name: "汽锅鸡", Category: "云南菜", Tags: `["云南菜","晚餐","汤品"]`,
				Ingredients: `[{"name":"鸡肉","amount":"500克"}]`},
			want: "meat",
		},
		{
			name: "name suffix 汤 stays soup",
			dish: Dish{Name: "莲藕花生排骨汤", Category: "粤菜",
				Ingredients: `[{"name":"排骨","amount":"300克"},{"name":"莲藕","amount":"1节"}]`},
			want: "soup",
		},
		{
			name: "category 汤品 stays soup",
			dish: Dish{Name: "紫菜蛋花汤", Category: "汤品",
				Ingredients: `[{"name":"紫菜","amount":"适量"},{"name":"鸡蛋","amount":"1个"}]`},
			want: "soup",
		},
		{
			name: "name suffix 羹 stays soup",
			dish: Dish{Name: "西湖牛肉羹", Category: "杭帮菜", Ingredients: `[{"name":"牛肉","amount":"100克"}]`},
			want: "soup",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferDishTraits(tt.dish)
			if got.DishRole != tt.want {
				t.Fatalf("InferDishTraits(%s).DishRole = %q, want %q", tt.dish.Name, got.DishRole, tt.want)
			}
		})
	}
}
