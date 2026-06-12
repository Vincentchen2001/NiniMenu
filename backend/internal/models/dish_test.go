package models

import (
	"encoding/json"
	"testing"
)

// TestDishUnmarshalJSONNormalizesTextFields pins normalizeJSONFieldText's
// contract for every shape a dish JSON field can arrive in: MarshalJSON's
// array/object output stays verbatim, legacy quoted strings wrapping such
// text are unwrapped, and everything else falls back to "[]".
func TestDishUnmarshalJSONNormalizesTextFields(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string // expected Ingredients TEXT value after unmarshal
	}{
		{
			name: "array form kept verbatim",
			json: `{"name":"a","ingredients":["土豆","番茄"]}`,
			want: `["土豆","番茄"]`,
		},
		{
			name: "object array form kept verbatim",
			json: `{"name":"a","ingredients":[{"name":"土豆","amount":"1个"}]}`,
			want: `[{"name":"土豆","amount":"1个"}]`,
		},
		{
			name: "quoted array string unwraps",
			json: `{"name":"a","ingredients":"[]"}`,
			want: `[]`,
		},
		{
			name: "quoted object array string unwraps",
			json: `{"name":"a","ingredients":"[{\"name\":\"土豆\",\"amount\":\"1个\"}]"}`,
			want: `[{"name":"土豆","amount":"1个"}]`,
		},
		{
			name: "quoted garbage falls back",
			json: `{"name":"a","ingredients":"not json"}`,
			want: `[]`,
		},
		{
			name: "null falls back",
			json: `{"name":"a","ingredients":null}`,
			want: `[]`,
		},
		{
			name: "missing field falls back",
			json: `{"name":"a"}`,
			want: `[]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Dish
			if err := json.Unmarshal([]byte(tt.json), &d); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if d.Ingredients != tt.want {
				t.Fatalf("Ingredients = %q, want %q", d.Ingredients, tt.want)
			}
		})
	}
}

// TestDishJSONRoundTrip ensures MarshalJSON output survives a round trip
// through UnmarshalJSON with the TEXT fields intact — the property the
// week-plan snapshot storage depends on.
func TestDishJSONRoundTrip(t *testing.T) {
	src := Dish{
		Name:        "回锅肉",
		Images:      `["a.jpg"]`,
		Ingredients: `[{"name":"五花肉","amount":"200g"}]`,
		Tags:        `["家常菜"]`,
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Dish
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Images != src.Images || back.Ingredients != src.Ingredients || back.Tags != src.Tags {
		t.Fatalf("round trip changed fields: images %q ingredients %q tags %q", back.Images, back.Ingredients, back.Tags)
	}
}
