package api

import (
	"slices"
	"sort"
	"testing"
	"time"
)

func TestNutritionFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	s, ts, _ := startTestServer(t, 54334)
	she := newClient(t, ts.URL)
	she.do("POST", "/api/setup", map[string]any{"householdName": "Home", "email": "she@example.com", "username": "sangita", "displayName": "Sangita", "password": "hunter2hunter2"}, 204, nil)
	join := func(email, username, role string) *testClient {
		var inv map[string]any
		she.do("POST", "/api/invites", map[string]string{"email": email, "role": role}, 200, &inv)
		c := newClient(t, ts.URL)
		c.do("POST", "/api/auth/invite/accept", map[string]any{"code": inv["code"], "username": username, "displayName": username, "password": "correcthorse"}, 204, nil)
		return c
	}
	he := join("he@example.com", "sayan", "member")
	cook := join("cook@example.com", "cook", "viewer")

	type healthList struct {
		Members []memberHealthOut `json:"members"`
	}
	var hl healthList
	she.do("GET", "/api/nutrition/profiles", nil, 200, &hl)
	if len(hl.Members) != 2 || hl.Members[0].Profile != nil { // the viewer isn't listed
		t.Fatalf("health: %+v", hl)
	}
	sheID, heID := hl.Members[0].UserID, hl.Members[1].UserID
	cook.do("GET", "/api/nutrition/profiles", nil, 403, nil)
	newClient(t, ts.URL).do("GET", "/api/health", nil, 200, nil) // the public liveness check isn't shadowed

	year := time.Now().Year()
	// Each can edit the other's details; a viewer can't.
	she.do("PUT", "/api/nutrition/profiles/"+heID, map[string]any{"sex": "male", "birthYear": year - 27, "heightCm": 170, "weightKg": 68, "goal": "maintain", "avoid": []string{"fish", "bogus"}}, 200, nil)
	he.do("PUT", "/api/nutrition/profiles/"+sheID, map[string]any{"sex": "female", "birthYear": year - 29, "heightCm": 154.9, "weightKg": 59, "goal": "lose"}, 200, &hl)
	cook.do("PUT", "/api/nutrition/profiles/"+sheID, map[string]any{"sex": "female", "birthYear": year - 29, "heightCm": 155, "weightKg": 50}, 403, nil)
	she.do("PUT", "/api/nutrition/profiles/"+sheID, map[string]any{"sex": "female", "birthYear": year - 29, "heightCm": 20, "weightKg": 59}, 400, nil)
	if p := hl.Members[1].Profile; p == nil || !slices.Equal(p.Avoid, []string{"fish"}) || hl.Members[1].Targets.Kcal < 2100 {
		t.Fatalf("his profile: %+v %+v", p, hl.Members[1].Targets)
	}
	if tg := hl.Members[0].Targets; tg.Kcal < 1350 || tg.Kcal > 1550 || tg.Protein != 71 || tg.Iron != 29 {
		t.Fatalf("her targets: %+v", tg)
	}
	// Overrides replace a computed target.
	she.do("PUT", "/api/nutrition/profiles/"+sheID, map[string]any{"sex": "female", "birthYear": year - 29, "heightCm": 154.9, "weightKg": 59, "goal": "lose", "overrides": map[string]float64{"protein": 80}}, 200, &hl)
	if hl.Members[0].Targets.Protein != 80 || hl.Members[0].Computed.Protein != 71 {
		t.Fatalf("override: %+v", hl.Members[0])
	}

	// The recipe pack imports once.
	var res struct {
		Added   int      `json:"added"`
		Skipped []string `json:"skipped"`
	}
	she.do("POST", "/api/recipes/packs/bengali-home", nil, 200, &res)
	if res.Added != 87 {
		t.Fatalf("pack: %+v", res)
	}
	she.do("POST", "/api/recipes/packs/bengali-home", nil, 200, &res)
	if res.Added != 0 || len(res.Skipped) != 87 {
		t.Fatalf("re-import: %+v", res)
	}
	// The command-line import uses the same path, as a household member.
	if n, sk, err := ImportPack(t.Context(), s.db, "sayan", "", "bengali-home"); err != nil || n != 0 || len(sk) != 87 {
		t.Fatalf("cli import: %d %d %v", n, len(sk), err)
	}
	if _, _, err := ImportPack(t.Context(), s.db, "cook", "", "bengali-home"); err == nil {
		t.Fatal("a viewer imported a pack")
	}
	var recipes []Recipe
	she.do("GET", "/api/recipes", nil, 200, &recipes)
	byTitle := map[string]Recipe{}
	for _, r := range recipes {
		byTitle[r.Title] = r
	}
	fish := byTitle["Macher jhol (rui)"]
	if fish.Nutrition.Coverage != 1 || fish.Nutrition.PerServing.Protein < 15 || !slices.Contains(fish.Nutrition.Contains, "fish") {
		t.Fatalf("fish nutrition: %+v", fish.Nutrition)
	}

	// Fish defaults to the fish-eater; portions can be set and reset.
	h, _ := s.getHousehold(t.Context(), fishHousehold(t, s, fish.ID))
	today := localToday(h, time.Now())
	d := today.Format(time.DateOnly)
	var m MealItem
	she.do("POST", "/api/meals", map[string]any{"date": d, "slot": "lunch", "recipeId": fish.ID}, 201, &m)
	if len(m.Eaters) != 1 || m.Eaters[sheID] != 1 {
		t.Fatalf("fish eaters: %+v", m.Eaters)
	}
	she.do("PATCH", "/api/meals/"+m.ID, map[string]any{"portions": map[string]float64{sheID: 1.3, heID: 0}}, 200, &m)
	if m.Eaters[sheID] != 1.25 || m.Eaters[heID] != 0 {
		t.Fatalf("portions: %+v", m.Eaters)
	}
	she.do("PATCH", "/api/meals/"+m.ID, map[string]any{"portions": map[string]float64{"00000000-0000-4000-8000-000000000000": 1}}, 400, nil)
	she.do("PATCH", "/api/meals/"+m.ID, map[string]any{"portions": nil}, 200, &m)
	if m.Portions != nil || m.Eaters[sheID] != 1 {
		t.Fatalf("reset: %+v", m)
	}
	she.do("DELETE", "/api/meals/"+m.ID, nil, 204, nil)

	// Auto-fill a week from today with the routine.
	to := today.AddDate(0, 0, 6)
	var fill map[string]int
	she.do("POST", "/api/meals/autofill", map[string]any{"from": d, "to": to.Format(time.DateOnly)}, 200, &fill)
	var meals []MealItem
	she.do("GET", "/api/meals?from="+d+"&to="+to.Format(time.DateOnly), nil, 200, &meals)
	if fill["created"] != len(meals) || len(meals) < 40 {
		t.Fatalf("autofill created %v, plan has %d", fill, len(meals))
	}
	rid := map[string]Recipe{}
	for _, r := range recipes {
		rid[r.ID] = r
	}
	prevDairy := ""
	for day := today; !day.After(to); day = day.AddDate(0, 0, 1) {
		ds := day.Format(time.DateOnly)
		var lunch, dinner, snacks []MealItem
		var dairy []string
		for _, mi := range meals {
			if mi.Date != ds {
				continue
			}
			switch mi.Slot {
			case "lunch":
				lunch = append(lunch, mi)
			case "dinner":
				dinner = append(dinner, mi)
			case "snacks":
				snacks = append(snacks, mi)
			}
			if mi.RecipeID != nil {
				r := rid[*mi.RecipeID]
				if slices.Contains(r.Nutrition.Contains, "fish") && mi.Eaters[heID] > 0 {
					t.Errorf("%s: fish served to a non-fish-eater: %s %v", ds, mi.Title, mi.Eaters)
				}
				if len(r.Months) > 0 && !slices.Contains(r.Months, int(day.Month())) {
					t.Errorf("%s: out of season: %s", ds, r.Title)
				}
				if r.role() == "dairy" {
					dairy = append(dairy, r.Title)
				}
			}
		}
		if len(lunch) < 2 || len(snacks) != 1 || rid[*snacks[0].RecipeID].role() != "fruit" {
			t.Errorf("%s: lunch %v, snacks %v", ds, titles(lunch), titles(snacks))
		}
		// One dairy item a day, never the same two days running.
		if len(dairy) != 1 || dairy[0] == prevDairy {
			t.Errorf("%s: dairy %v after %q", ds, dairy, prevDairy)
		} else {
			prevDairy = dairy[0]
		}
		// Everyone gets protein at an everyday (rice) lunch.
		everyday := false
		for _, mi := range lunch {
			everyday = everyday || (mi.RecipeID != nil && rid[*mi.RecipeID].role() == "rice")
		}
		got := map[string]bool{sheID: !everyday, heID: !everyday}
		for _, mi := range lunch {
			if mi.RecipeID != nil && rid[*mi.RecipeID].isProtein() {
				for id, v := range mi.Eaters {
					got[id] = got[id] || v > 0
				}
			}
		}
		if !got[sheID] || !got[heID] {
			t.Errorf("%s: protein not covered at lunch: %v (%v)", ds, got, titles(lunch))
		}
		switch dinnerPattern[day.Weekday()] {
		case "out":
			if len(dinner) != 1 || dinner[0].Title != eatingOutTitle {
				t.Errorf("%s: expected eating out, got %v", ds, titles(dinner))
			}
		}
		// Cooked once for two days: a meal cooked today comes back reheated tomorrow (same
		// dishes, made in enough quantity for both days), then something fresh is cooked.
		for _, slot := range []string{"breakfast", "lunch", "dinner"} {
			cooked, again := cookedFor(meals, rid, day, slot), cookedFor(meals, rid, day.AddDate(0, 0, 1), slot)
			yday := cookedFor(meals, rid, day.AddDate(0, 0, -1), slot)
			if len(cooked.fresh) > 0 && len(cooked.reheated) > 0 {
				t.Errorf("%s %s: cooked and reheated dishes mixed: %v / %v", ds, slot, titles(cooked.fresh), titles(cooked.reheated))
			}
			if len(cooked.reheated) > 0 && len(yday.reheated) > 0 {
				t.Errorf("%s %s: reheated a third day", ds, slot)
			}
			for _, mi := range meals {
				if mi.Date == ds && mi.Slot == slot && mi.RecipeID != nil && rid[*mi.RecipeID].role() == "bread" && mi.Leftover {
					t.Errorf("%s %s: rotis reheated, not made fresh", ds, slot)
				}
			}
			oneNight := slot == "dinner" && (dinnerPattern[day.Weekday()] != "cook" || dinnerPattern[day.AddDate(0, 0, 1).Weekday()] != "cook")
			if day.Before(to) && len(cooked.fresh) > 0 && !oneNight {
				if len(again.reheated) != len(cooked.fresh) {
					t.Errorf("%s %s: cooked %v but next day has %v", ds, slot, titles(cooked.fresh), titles(again.reheated))
					continue
				}
				for i, mi := range cooked.fresh {
					next := again.reheated[i]
					if next.Title != mi.Title || float64(mi.Servings) < sum(mi.Eaters)+sum(next.Eaters)-1e-9 {
						t.Errorf("%s %s: %s ×%d doesn't cover %v + next day's %s %v", ds, slot, mi.Title, mi.Servings, mi.Eaters, next.Title, next.Eaters)
					}
				}
			}
		}
		// Portions: the rice is sized per person, and she eats less of it than he does.
		for _, mi := range lunch {
			if mi.RecipeID != nil && rid[*mi.RecipeID].role() == "rice" && mi.Portions != nil && mi.Eaters[sheID] > mi.Eaters[heID] {
				t.Errorf("%s: rice portions %v", ds, mi.Eaters)
			}
		}
	}

	// Leftovers don't add to the grocery list a second time.
	var gen map[string]int
	she.do("POST", "/api/grocery/generate", map[string]any{"from": d, "to": to.Format(time.DateOnly)}, 200, &gen)

	// The report: both people, near their targets on counted days.
	var rep []personReport
	she.do("GET", "/api/nutrition?from="+d+"&to="+to.Format(time.DateOnly), nil, 200, &rep)
	if len(rep) != 2 || rep[0].Targets == nil || rep[0].Counted == 0 {
		t.Fatalf("report: %+v", rep)
	}
	for _, p := range rep {
		t.Logf("%s: %d counted, %d partial; avg %.0f kcal (target %.0f), protein %.0f/%.0f, fibre %.0f, iron %.1f/%.0f, calcium %.0f, sodium %.0f; gaps %d",
			p.Name, p.Counted, p.Partial, p.Average.Kcal, p.Targets.Kcal, p.Average.Protein, p.Targets.Protein, p.Average.Fibre, p.Average.Iron, p.Targets.Iron, p.Average.Calcium, p.Average.Sodium, len(p.Gaps))
		if r := p.Average.Kcal / p.Targets.Kcal; r < 0.75 || r > 1.25 {
			t.Errorf("%s: average %.0f kcal is far from target %.0f", p.Name, p.Average.Kcal, p.Targets.Kcal)
		}
		if p.Average.Protein < p.Targets.Protein*0.75 {
			t.Errorf("%s: protein %.0f g under target %.0f", p.Name, p.Average.Protein, p.Targets.Protein)
		}
	}
	cook.do("GET", "/api/nutrition?from="+d+"&to="+d, nil, 403, nil)
}

func fishHousehold(t *testing.T, s *Server, recipeID string) string {
	var hid string
	if err := s.db.QueryRow(t.Context(), `SELECT household_id FROM recipes WHERE id = $1`, recipeID).Scan(&hid); err != nil {
		t.Fatal(err)
	}
	return hid
}

type slotMeals struct{ fresh, reheated []MealItem }

// cookedFor: a day's cooked dishes in a slot (not fruit, dairy or rotis, made fresh daily).
func cookedFor(meals []MealItem, rid map[string]Recipe, day time.Time, slot string) slotMeals {
	var out slotMeals
	for _, mi := range meals {
		if mi.Date != day.Format(time.DateOnly) || mi.Slot != slot || mi.RecipeID == nil {
			continue
		}
		if role := rid[*mi.RecipeID].role(); role == "fruit" || role == "dairy" || role == "bread" {
			continue // fresh every day (rotis from dough made for two days)
		}
		if mi.Leftover {
			out.reheated = append(out.reheated, mi)
		} else {
			out.fresh = append(out.fresh, mi)
		}
	}
	sort.SliceStable(out.fresh, func(i, j int) bool { return out.fresh[i].Title < out.fresh[j].Title })
	sort.SliceStable(out.reheated, func(i, j int) bool { return out.reheated[i].Title < out.reheated[j].Title })
	return out
}

func titles(ms []MealItem) []string {
	out := []string{}
	for _, m := range ms {
		out = append(out, m.Title)
	}
	return out
}
