package api

import (
	"fmt"
	"os"
	"testing"
)

func TestPacksValidAndCovered(t *testing.T) {
	packs, err := loadPacks()
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) == 0 {
		t.Fatal("no packs")
	}
	for _, p := range packs {
		seen := map[string]bool{}
		for _, rc := range p.Recipes {
			if seen[rc.Title] {
				t.Errorf("%s: duplicate %q", p.Name, rc.Title)
			}
			seen[rc.Title] = true
			if err := rc.validate(); err != nil {
				t.Errorf("%s: %q invalid: %v", p.Name, rc.Title, err)
			}
			for i := range rc.Ingredients {
				rc.Ingredients[i].Unit = normalizeUnit(rc.Ingredients[i].Unit)
			}
			rc.computeNutrition()
			if rc.Nutrition.Coverage < 1 {
				t.Errorf("%s: %q missing nutrition for %v", p.Name, rc.Title, rc.Nutrition.Missing)
			}
			if os.Getenv("PACK_REPORT") != "" {
				n := rc.Nutrition.PerServing
				fmt.Printf("%-40s %5.0f kcal P%5.1f C%5.1f F%5.1f Fb%4.1f Fe%4.1f Ca%5.0f Na%5.0f  %v %s\n", rc.Title, n.Kcal, n.Protein, n.Carbs, n.Fat, n.Fibre, n.Iron, n.Calcium, n.Sodium, rc.Nutrition.Contains, rc.role())
			}
		}
	}
}
