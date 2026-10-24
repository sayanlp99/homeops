package api

import (
	"testing"

	"homeops/internal/nutrition"
)

func TestStapleSteps(t *testing.T) {
	roti := &Recipe{Nutrition: nutrition.Result{PerServing: nutrition.Nutrients{Kcal: 64}}}
	if step, floor, most := stapleSteps(roti); step != 1 || floor != 1 || most != 10 {
		t.Fatalf("roti: %v %v %v", step, floor, most)
	}
	rice := &Recipe{Nutrition: nutrition.Result{PerServing: nutrition.Nutrients{Kcal: 356}}}
	if step, floor, most := stapleSteps(rice); step != 0.25 || floor != 0.5 || most != 3 {
		t.Fatalf("rice: %v %v %v", step, floor, most)
	}
}
