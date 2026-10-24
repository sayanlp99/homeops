package nutrition

import (
	"math"
	"testing"
)

func q(f float64) *float64 { return &f }

func TestMatch(t *testing.T) {
	cases := map[string]string{
		"Mustard oil":                "oil",
		"Shorshe bata":               "mustard-seed",
		"Sweet potato":               "sweet-potato",
		"Potatoes":                   "potato",
		"Green chillies":             "green-chilli",
		"Kashmiri red chilli powder": "chilli-powder",
		"Rui maach":                  "fish",
		"Chicken breast":             "chicken-boneless",
		"Chicken":                    "chicken",
		"Chicken masala":             "spice-mix",
		"Eggs":                       "egg",
		"Duck or chicken eggs":       "egg",
		"Egg whites":                 "egg-white",
		"Ilish maach":                "hilsa",
		"Eggplant":                   "brinjal",
		"Moong dal":                  "moong-dal",
		"Green moong dal":            "green-moong",
		"Chholar dal":                "chana-dal",
		"Coconut oil":                "oil",
		"Grated coconut":             "coconut",
		"Palong shaak":               "spinach",
		"Koraishuti":                 "green-peas",
		"Dried white peas":           "white-peas",
		"Gobindobhog rice":           "rice",
		"Poha":                       "poha",
		"Bell pepper":                "capsicum",
		"Posto":                      "poppy",
		"Lau":                        "bottle-gourd",
		"Protein bread":              "protein-bread",
		"Water":                      "water",
	}
	for name, want := range cases {
		f := Match(name)
		if f == nil || f.Key != want {
			got := "<nil>"
			if f != nil {
				got = f.Key
			}
			t.Errorf("Match(%q) = %s, want %s", name, got, want)
		}
	}
	if f := Match("Unobtainium"); f != nil {
		t.Errorf("unexpected match %s", f.Key)
	}
}

func TestGrams(t *testing.T) {
	cases := []struct {
		food string
		qty  float64
		unit string
		want float64
	}{
		{"oil", 1, "tbsp", 13.8},
		{"rice", 1, "cup", 199.2},
		{"onion", 2, "pcs", 220},
		{"onion", 2, "", 220},
		{"garlic", 4, "clove", 16},
		{"egg", 2, "pcs", 100},
		{"chicken", 500, "g", 375}, // 75% edible
		{"raw-banana", 1, "pcs", 120},
		{"spinach", 1, "bunch", 250},
	}
	for _, c := range cases {
		g, ok := Grams(ByKey(c.food), c.qty, c.unit)
		if !ok || math.Abs(g-c.want) > 0.5 {
			t.Errorf("Grams(%s, %v %s) = %v %v, want %v", c.food, c.qty, c.unit, g, ok, c.want)
		}
	}
	if _, ok := Grams(ByKey("chicken"), 1, "packet"); ok {
		t.Error("a packet of chicken has no known weight")
	}
}

func TestCalc(t *testing.T) {
	// Plain masoor dal for 2.
	r := Calc([]Line{
		{Name: "Masoor dal", Quantity: q(100), Unit: "g"},
		{Name: "Mustard oil", Quantity: q(1), Unit: "tbsp"},
		{Name: "Salt", Quantity: nil},
		{Name: "Mystery spice", Quantity: q(1), Unit: "tsp"},
		{Name: "Water", Quantity: q(3), Unit: "cup"},
	}, 2)
	// 343 kcal + 13.8 g oil × 8.84 = 465 → 232.5 per serving
	if math.Abs(r.PerServing.Kcal-232.5) > 1 {
		t.Errorf("kcal = %v", r.PerServing.Kcal)
	}
	if math.Abs(r.PerServing.Protein-12.2) > 0.1 {
		t.Errorf("protein = %v", r.PerServing.Protein)
	}
	if r.Coverage != 0.75 || len(r.Missing) != 1 || r.Missing[0] != "Mystery spice" {
		t.Errorf("coverage = %v missing = %v", r.Coverage, r.Missing)
	}
	r = Calc([]Line{{Name: "Rui maach", Quantity: q(4), Unit: "pcs"}, {Name: "Eggs", Quantity: q(2)}}, 2)
	if len(r.Contains) != 2 || r.Contains[0] != "egg" || r.Contains[1] != "fish" {
		t.Errorf("contains = %v", r.Contains)
	}
}

func TestTargets(t *testing.T) {
	// 29 y woman, 155 cm, 59 kg, light activity, gentle loss.
	f := DailyTargets(Profile{Sex: "female", Age: 29, HeightCm: 154.9, WeightKg: 59, Activity: "light", Goal: "lose"})
	if f.BMR != 1252 || f.Kcal < 1400 || f.Kcal > 1500 || f.Protein != 71 || f.Iron != 29 {
		t.Errorf("female targets = %+v", f)
	}
	m := DailyTargets(Profile{Sex: "male", Age: 27, HeightCm: 170, WeightKg: 68, Activity: "light", Goal: "maintain"})
	if m.Kcal < 2150 || m.Kcal > 2250 || m.Protein != 75 || m.Iron != 19 {
		t.Errorf("male targets = %+v", m)
	}
}
