package models

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

type Dish struct {
	ID                 uint           `json:"id" gorm:"primaryKey"`
	Name               string         `json:"name" gorm:"not null"`
	ImageURL           string         `json:"image_url"`
	Images             string         `json:"images" gorm:"default:'[]'"`
	VideoURL           string         `json:"video_url"`
	Category           string         `json:"category" gorm:"index"`
	MealType           string         `json:"meal_type" gorm:"default:'all';index;index:idx_dishes_pick,priority:2"`
	Taste              string         `json:"taste"`
	Ingredients        string         `json:"ingredients" gorm:"default:'[]'"`
	Seasonings         string         `json:"seasonings" gorm:"default:'[]'"`
	Steps              string         `json:"steps" gorm:"default:'[]'"`
	DishRole           string         `json:"dish_role" gorm:"default:'unknown';index"`
	ProteinSources     string         `json:"protein_sources" gorm:"default:'[]'"`
	ServingTemperature string         `json:"serving_temperature" gorm:"default:'unknown';index"`
	CookingMethods     string         `json:"cooking_methods" gorm:"default:'[]'"`
	SpiceLevel         int            `json:"spice_level" gorm:"default:0"`
	RichnessLevel      int            `json:"richness_level" gorm:"default:0"`
	CarbLevel          int            `json:"carb_level" gorm:"default:0"`
	TraitSource        string         `json:"trait_source" gorm:"default:'auto';index"`
	TraitVersion       int            `json:"trait_version" gorm:"default:0"`
	CookTime           int            `json:"cook_time" gorm:"default:0"`
	Difficulty         string         `json:"difficulty" gorm:"default:'easy'"`
	Remark             string         `json:"remark"`
	Favorite           bool           `json:"favorite" gorm:"default:false;index"`
	Enabled            bool           `json:"enabled" gorm:"default:true;index;index:idx_dishes_pick,priority:1"`
	Tags               string         `json:"tags" gorm:"default:'[]'"`
	SortOrder          int            `json:"sort_order" gorm:"default:0"`
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"index;index:idx_dishes_pick,priority:3"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

// jsonField 把数据库里存的 JSON 字符串转成可直接内联的 JSON 片段。
// 合法 JSON 原样输出（数组/对象不再被转义为字符串）；空值或非法内容回退为 []。
func jsonField(s string) json.RawMessage {
	if s == "" {
		return json.RawMessage("[]")
	}
	if json.Valid([]byte(s)) {
		return json.RawMessage(s)
	}
	return json.RawMessage("[]")
}

// UnmarshalJSON mirrors MarshalJSON: the plan JSON stores Images/Ingredients/…
// as real JSON arrays (via the custom marshal alias), so when we round-trip
// plan JSON back into Dish structs we must accept both a plain string and a
// JSON array for those fields and store it back as the string form GORM uses.
func (d *Dish) UnmarshalJSON(data []byte) error {
	type alias Dish
	aux := &struct {
		alias
		Images         json.RawMessage `json:"images"`
		Ingredients    json.RawMessage `json:"ingredients"`
		Seasonings     json.RawMessage `json:"seasonings"`
		Steps          json.RawMessage `json:"steps"`
		ProteinSources json.RawMessage `json:"protein_sources"`
		CookingMethods json.RawMessage `json:"cooking_methods"`
		Tags           json.RawMessage `json:"tags"`
	}{}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	*d = Dish(aux.alias)
	rawFieldStr := func(raw json.RawMessage) string {
		if len(raw) == 0 {
			return "[]"
		}
		return string(raw)
	}
	d.Images = rawFieldStr(aux.Images)
	d.Ingredients = rawFieldStr(aux.Ingredients)
	d.Seasonings = rawFieldStr(aux.Seasonings)
	d.Steps = rawFieldStr(aux.Steps)
	d.ProteinSources = rawFieldStr(aux.ProteinSources)
	d.CookingMethods = rawFieldStr(aux.CookingMethods)
	d.Tags = rawFieldStr(aux.Tags)
	return nil
}

// MarshalJSON 让 images/ingredients/seasonings/steps/tags 等 JSON 字符串字段在 API 响应中
// 输出为真正的 JSON 数组/对象，而不是被转义的字符串。数据库仍以 TEXT 存储，写入路径不变。
func (d Dish) MarshalJSON() ([]byte, error) {
	type alias Dish // 借助别名避免无限递归
	return json.Marshal(&struct {
		alias
		Images         json.RawMessage `json:"images"`
		Ingredients    json.RawMessage `json:"ingredients"`
		Seasonings     json.RawMessage `json:"seasonings"`
		Steps          json.RawMessage `json:"steps"`
		ProteinSources json.RawMessage `json:"protein_sources"`
		CookingMethods json.RawMessage `json:"cooking_methods"`
		Tags           json.RawMessage `json:"tags"`
	}{
		alias:          alias(d),
		Images:         jsonField(d.Images),
		Ingredients:    jsonField(d.Ingredients),
		Seasonings:     jsonField(d.Seasonings),
		Steps:          jsonField(d.Steps),
		ProteinSources: jsonField(d.ProteinSources),
		CookingMethods: jsonField(d.CookingMethods),
		Tags:           jsonField(d.Tags),
	})
}
