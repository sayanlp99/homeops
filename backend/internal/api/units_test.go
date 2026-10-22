package api

import "testing"

func TestParseIngredientLine(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		in   string
		qty  *float64
		unit string
		name string
		note string
	}{
		{"2 cups basmati rice, washed", f(2), "cup", "Basmati rice", "washed"},
		{"1½ kg chicken", f(1.5), "kg", "Chicken", ""},
		{"1 1/2 tsp turmeric powder", f(1.5), "tsp", "Turmeric powder", ""},
		{"½ tsp hing", f(0.5), "tsp", "Hing", ""},
		{"⅓ cup cream", f(1.0 / 3), "cup", "Cream", ""},
		{"200g paneer, cubed", f(200), "g", "Paneer", "cubed"},
		{"2-3 green chillies, slit", f(3), "", "Green chillies", "slit"},
		{"Salt to taste", nil, "", "Salt", "to taste"},
		{"a pinch of salt", nil, "pinch", "Salt", ""},
		{"1 (400 g) can chopped tomatoes", f(1), "can", "Chopped tomatoes", "400 g"},
		{"3 Tbsp ghee", f(3), "tbsp", "Ghee", ""},
		{"1 inch ginger", f(1), "inch", "Ginger", ""},
		{"4 cloves garlic", f(4), "clove", "Garlic", ""},
		{"1 bunch coriander leaves", f(1), "bunch", "Coriander leaves", ""},
		{"2 onions", f(2), "", "Onions", ""},
		{"Few curry leaves", nil, "", "Few curry leaves", ""},
		{"- 500 ml milk", f(500), "ml", "Milk", ""},
		{"1 litre milk", f(1), "l", "Milk", ""},
		// Real lines from a popular Indian recipe site.
		{"1 cup (200 grams) toor dal ((split pigeon peas or ¾ cup toor dal))", f(1), "cup", "Toor dal", "200 grams; split pigeon peas or ¾ cup toor dal"},
		{"½ teaspoon cumin seeds ((jeera))", f(0.5), "tsp", "Cumin seeds", "jeera"},
		{"1 teaspoon (4) garlic (cloves, fine chopped)", f(1), "tsp", "Garlic", "4; cloves, fine chopped"},
		{"1 medium (⅓ to ½ cup) onion ((fine chopped))", f(1), "", "Onion", "⅓ to ½ cup; fine chopped; medium"},
		{"2 medium (1 cup) tomatoes ((deseeded, fine chopped))", f(2), "", "Tomatoes", "1 cup; deseeded, fine chopped; medium"},
		{"1½ to 2 tablespoon oil", f(2), "tbsp", "Oil", ""},
		{"1 teaspoon salt ((more to adjust))", f(1), "tsp", "Salt", "more to adjust"},
	}
	for _, c := range cases {
		got := parseIngredientLine(c.in)
		okQty := (got.Quantity == nil && c.qty == nil) || (got.Quantity != nil && c.qty != nil && round2(*got.Quantity) == round2(*c.qty))
		if !okQty || got.Unit != c.unit || got.Name != c.name || got.Note != c.note {
			q := "nil"
			if got.Quantity != nil {
				q = formatFloat(*got.Quantity)
			}
			t.Errorf("%q → qty=%s unit=%q name=%q note=%q", c.in, q, got.Unit, got.Name, got.Note)
		}
	}
}

func TestClassifyAisle(t *testing.T) {
	cases := map[string]string{
		"Onions": "Vegetables", "Green chillies": "Vegetables", "Red chilli powder": "Spices & Masala",
		"Unsalted butter": "Dairy & Eggs", "Eggplant": "Vegetables", "Eggs": "Dairy & Eggs",
		"Toor dal": "Pulses & Lentils", "Basmati rice": "Grains & Flour", "Mustard oil": "Oils & Ghee",
		"Coriander leaves": "Vegetables", "Coriander powder": "Spices & Masala", "Chicken thighs": "Meat & Fish",
		"Paneer": "Dairy & Eggs", "Curry leaves": "Vegetables", "Tomatoes": "Vegetables", "Salt": "Spices & Masala",
		"Dishwash liquid": "Household", "Quinoa": "Grains & Flour", "Something odd": "Other",
	}
	for in, want := range cases {
		if got := classifyAisle(in); got != want {
			t.Errorf("%s: got %s, want %s", in, got, want)
		}
	}
}

func TestConvertAndHumanize(t *testing.T) {
	if v, ok := convert(2, "tbsp", "tsp"); !ok || v != 6 {
		t.Errorf("tbsp→tsp: %v %v", v, ok)
	}
	if _, ok := convert(1, "kg", "ml"); ok {
		t.Error("mass→volume should fail")
	}
	if q, u := humanize(1500, "g"); q != 1.5 || u != "kg" {
		t.Errorf("humanize 1500g: %v %s", q, u)
	}
	if q, u := humanize(750, "ml"); q != 750 || u != "ml" {
		t.Errorf("humanize 750ml: %v %s", q, u)
	}
}
