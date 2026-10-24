package api

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"homeops/internal/nutrition"
)

// Suggestions are rule-based: every recipe gets a score for a given day and slot from
// its history (recency, frequency, weekday habits), what's in the pantry, what else is
// planned (variety), and which dishes you usually eat together (mix & match).

type Suggestion struct {
	RecipeID string   `json:"recipeId"`
	Title    string   `json:"title"`
	Photo    string   `json:"photo"`
	Category string   `json:"category"`
	Score    float64  `json:"score"`
	Reasons  []string `json:"reasons"`
}

type suggestCtx struct {
	hh       *householdHealth
	targets  map[string]*nutrition.Targets // members with a health profile
	weekDow  int                           // first day of the household's week
	recipes  []Recipe
	byID     map[string]*Recipe
	history  []MealItem // last 180 days + upcoming plan
	pantry   map[string]bool
	today    time.Time
	lastDate map[string]time.Time
	count    map[string]int
	weekday  map[string]map[time.Weekday]int
	pairs    map[[2]string]int // co-occurrence of two recipes in the same slot
}

func (s *Server) loadSuggestCtx(ctx context.Context, h *Household) (*suggestCtx, error) {
	recipes, err := s.queryRecipes(ctx, h.ID, "")
	if err != nil {
		return nil, err
	}
	today := localToday(h, time.Now())
	rows, err := s.db.Query(ctx, `SELECT `+mealColumns+` FROM meal_plan_items
		WHERE household_id = $1 AND date >= $2 AND date <= $3 ORDER BY date`, h.ID, today.AddDate(0, 0, -180), today.AddDate(0, 0, 30))
	if err != nil {
		return nil, err
	}
	var history []MealItem
	for rows.Next() {
		var m MealItem
		if err := rows.Scan(&m.ID, &m.Date, &m.Slot, &m.Position, &m.RecipeID, &m.Title, &m.Servings, &m.Note, &m.CookedAt, &m.CookedBy, &m.CreatedAt, &m.Portions, &m.Leftover); err != nil {
			rows.Close()
			return nil, err
		}
		history = append(history, m)
	}
	rows.Close()
	pantry := map[string]bool{}
	prow, err := s.db.Query(ctx, `SELECT ingredient_id FROM pantry_items WHERE household_id = $1 AND (quantity IS NULL OR quantity > 0)`, h.ID)
	if err != nil {
		return nil, err
	}
	for prow.Next() {
		var id string
		prow.Scan(&id)
		pantry[id] = true
	}
	prow.Close()

	hh, err := s.loadHouseholdHealth(ctx, h.ID)
	if err != nil {
		return nil, err
	}
	targets := map[string]*nutrition.Targets{}
	for _, m := range hh.members {
		if p := hh.profiles[m.ID]; p != nil && m.Role != "viewer" {
			_, t := p.targets(time.Now())
			targets[m.ID] = &t
		}
	}
	for i := range history {
		history[i].Eaters = nil // resolved lazily by eatersOf
	}
	c := &suggestCtx{hh: hh, targets: targets, weekDow: h.SprintStartDow, recipes: recipes, history: history, pantry: pantry, today: today,
		lastDate: map[string]time.Time{}, count: map[string]int{}, weekday: map[string]map[time.Weekday]int{}, pairs: map[[2]string]int{}}
	slotGroups := map[string][]string{}
	for _, m := range history {
		if m.RecipeID == nil {
			continue
		}
		d, _ := time.Parse(time.DateOnly, m.Date)
		if d.After(today) {
			continue // only the past counts as history
		}
		id := *m.RecipeID
		c.count[id]++
		if d.After(c.lastDate[id]) {
			c.lastDate[id] = d
		}
		if c.weekday[id] == nil {
			c.weekday[id] = map[time.Weekday]int{}
		}
		c.weekday[id][d.Weekday()]++
		key := m.Date + "/" + m.Slot
		slotGroups[key] = append(slotGroups[key], id)
	}
	for _, ids := range slotGroups {
		for i := range ids {
			for j := i + 1; j < len(ids); j++ {
				a, b := ids[i], ids[j]
				if a > b {
					a, b = b, a
				}
				c.pairs[[2]string{a, b}]++
			}
		}
	}
	return c, nil
}

func (c *suggestCtx) pairCount(a, b string) int {
	if a > b {
		a, b = b, a
	}
	return c.pairs[[2]string{a, b}]
}

// suggest ranks recipes for one slot. `planned` is the week's plan as it stands
// (including dishes the caller is about to add), used for variety and pairing.
func (c *suggestCtx) suggest(date time.Time, slot string, planned []MealItem, limit int) []Suggestion {
	dateStr := date.Format(time.DateOnly)
	inSlot, sameDay, sameWeek, adjacent := []string{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	prevDay, nextDay := date.AddDate(0, 0, -1).Format(time.DateOnly), date.AddDate(0, 0, 1).Format(time.DateOnly)
	catsInSlot, cuisinesToday := map[string]bool{}, map[string]bool{}
	byID := map[string]*Recipe{}
	for i := range c.recipes {
		byID[c.recipes[i].ID] = &c.recipes[i]
	}
	for _, m := range planned {
		if m.RecipeID == nil {
			continue
		}
		sameWeek[*m.RecipeID] = true
		if m.Date == prevDay || m.Date == nextDay {
			adjacent[*m.RecipeID] = true
		}
		if m.Date == dateStr {
			sameDay[*m.RecipeID] = true
			if r := byID[*m.RecipeID]; r != nil && r.Cuisine != "" {
				cuisinesToday[strings.ToLower(r.Cuisine)] = true
			}
			if m.Slot == slot {
				inSlot = append(inSlot, *m.RecipeID)
				if r := byID[*m.RecipeID]; r != nil {
					catsInSlot[r.Category] = true
				}
			}
		}
	}

	week := weekStart(date, c.weekDow)
	days := date.Sub(week).Hours()/24 + 1
	intake := c.intake(planned, week, date)

	var out []Suggestion
	for _, r := range c.recipes {
		if slices.Contains(inSlot, r.ID) {
			continue
		}
		if len(r.MealTypes) > 0 && !slices.Contains(r.MealTypes, slot) {
			continue
		}
		score := 1.0
		var reasons []string
		if len(r.MealTypes) > 0 {
			score += 1
		}
		if r.Favorite {
			score += 1.5
			reasons = append(reasons, "Favourite")
		}
		score += float64(r.Rating) * 0.3

		if len(r.Months) > 0 && !slices.Contains(r.Months, int(date.Month())) {
			score -= 6
			reasons = append(reasons, "Out of season")
		}
		if gap := r.minGapDays(); gap > 0 {
			if c.within(r.ID, date, gap, planned) {
				score -= 8
			} else if _, seen := c.lastDate[r.ID]; seen {
				score += 1
				reasons = append(reasons, "Due again")
			}
		}
		if eaters := c.hh.defaultEaters(r.Nutrition.Contains); len(eaters) == 0 && len(c.hh.members) > 0 {
			score -= 6 // nobody here eats it
		}
		if bonus, why := c.nutritionBonus(&r, c.hh.defaultEaters(r.Nutrition.Contains), intake, days); bonus != 0 {
			score += bonus
			if why != "" {
				reasons = append(reasons, why)
			}
		}

		if last, seen := c.lastDate[r.ID]; seen {
			days := int(date.Sub(last).Hours() / 24)
			switch {
			case days < 4:
				score -= 5
			case days < 8:
				score -= 2
			case days >= 21:
				score += math.Min(float64(days)/14, 3)
				reasons = append(reasons, fmt.Sprintf("Not had in %d weeks", days/7))
			}
		} else {
			score += 0.5
			reasons = append(reasons, "Not tried yet")
		}
		if n := c.count[r.ID]; n >= 3 {
			score += math.Log(float64(n)) * 0.6
			reasons = append(reasons, fmt.Sprintf("Cooked %d times", n))
		}
		if wd := c.weekday[r.ID][date.Weekday()]; wd >= 2 {
			score += 1.5 + float64(wd)*0.25
			reasons = append(reasons, "Usually on "+date.Weekday().String()+"s")
		}

		// Pantry coverage: prefer what you can cook without shopping.
		if len(r.Ingredients) > 0 {
			have := 0
			for _, ing := range r.Ingredients {
				if c.pantry[ing.IngredientID] {
					have++
				}
			}
			frac := float64(have) / float64(len(r.Ingredients))
			score += frac * 2
			if frac >= 0.75 {
				reasons = append(reasons, fmt.Sprintf("%d%% of ingredients at home", int(frac*100)))
			}
		}

		// Variety.
		if sameWeek[r.ID] {
			score -= 3
		}
		if sameDay[r.ID] {
			score -= 4
		}
		if adjacent[r.ID] {
			score -= 2.5 // nobody wants the same curry two nights running
		}
		if catsInSlot[r.Category] && r.Category != "side" && r.Category != "chutney" && r.Category != "salad" {
			score -= 2.5 // a second dal in the same lunch is unlikely
		}
		if r.Cuisine != "" && cuisinesToday[strings.ToLower(r.Cuisine)] && len(inSlot) == 0 {
			score -= 0.5
		}

		// Mix & match: what goes with the dishes already in this slot.
		best, bestWith := 0, ""
		for _, other := range inSlot {
			if n := c.pairCount(r.ID, other); n > best {
				best, bestWith = n, other
			}
		}
		if best > 0 {
			score += 1.5 + float64(best)*0.75
			if o := byID[bestWith]; o != nil {
				reasons = append(reasons, "Goes with "+o.Title)
			}
		} else if len(inSlot) > 0 && complements(catsInSlot, r.Category) {
			score += 1
			reasons = append(reasons, "Completes the meal")
		}

		out = append(out, Suggestion{RecipeID: r.ID, Title: r.Title, Photo: r.Photo, Category: r.Category, Score: round2(score), Reasons: reasons})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// complements: a classic Indian plate is a dal/curry + a sabzi + a rice or bread.
func complements(have map[string]bool, cat string) bool {
	switch cat {
	case "rice", "bread":
		return (have["dal"] || have["curry"] || have["sabzi"]) && !have["rice"] && !have["bread"]
	case "sabzi":
		return (have["dal"] || have["curry"]) && !have["sabzi"]
	case "dal", "curry":
		return (have["rice"] || have["bread"] || have["sabzi"]) && !have["dal"] && !have["curry"]
	case "salad", "chutney", "side":
		return len(have) >= 2
	}
	return false
}

func (s *Server) suggestMeals(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	date, err := parseDate(r.URL.Query().Get("date"))
	if err != nil {
		return err
	}
	slot := r.URL.Query().Get("slot")
	if !slices.Contains(mealSlots, slot) {
		return errBadRequest("Unknown meal slot")
	}
	h, err := s.getHousehold(r.Context(), u.HouseholdID)
	if err != nil {
		return err
	}
	c, err := s.loadSuggestCtx(r.Context(), h)
	if err != nil {
		return err
	}
	week := weekStart(date, h.SprintStartDow)
	planned := plannedBetween(c.history, week, week.AddDate(0, 0, 6))
	return ok(w, c.suggest(date, slot, planned, 8))
}

var (
	anchorCategories = map[string]bool{"main": true, "curry": true, "dal": true, "sabzi": true, "breakfast": true, "snack": true, "soup": true}
	accompaniments   = map[string]bool{"drink": true, "chutney": true, "side": true, "salad": true, "dessert": true}
)

// plate picks dishes for an empty slot: an anchor dish first (never a lone drink or
// chutney), then, for lunch and dinner, one dish that completes it or is usually eaten
// with it (dal → rice, curry → roti).
func (c *suggestCtx) plate(date time.Time, slot string, planned []MealItem) []Suggestion {
	cands := c.suggest(date, slot, planned, 20)
	var first *Suggestion
	for _, pass := range []func(Suggestion) bool{
		func(s Suggestion) bool { return anchorCategories[s.Category] },
		func(s Suggestion) bool { return !accompaniments[s.Category] },
	} {
		for i := range cands {
			if pass(cands[i]) {
				first = &cands[i]
				break
			}
		}
		if first != nil {
			break
		}
	}
	if first == nil {
		return nil
	}
	out := []Suggestion{*first}
	if slot != "lunch" && slot != "dinner" {
		return out
	}
	rid := first.RecipeID
	with := append(append([]MealItem{}, planned...), MealItem{Date: date.Format(time.DateOnly), Slot: slot, RecipeID: &rid})
	for _, s := range c.suggest(date, slot, with, 10) {
		for _, r := range s.Reasons {
			if strings.HasPrefix(r, "Goes with") || r == "Completes the meal" {
				return append(out, s)
			}
		}
	}
	return out
}

func plannedBetween(items []MealItem, from, to time.Time) []MealItem {
	var out []MealItem
	for _, m := range items {
		d, _ := time.Parse(time.DateOnly, m.Date)
		if !d.Before(from) && !d.After(to) {
			out = append(out, m)
		}
	}
	return out
}

// ---- nutrition-aware scoring ----

// minGapDays reads an "every:N" tag: cook at most once every N days (prawns weekly,
// khichdi fortnightly).
func (r *Recipe) minGapDays() int {
	for _, t := range r.Tags {
		if v, ok := strings.CutPrefix(t, "every:"); ok {
			var n int
			fmt.Sscanf(v, "%d", &n)
			return n
		}
	}
	return 0
}

func (r Recipe) hasTag(t string) bool { return slices.Contains(r.Tags, t) }

// within reports whether the recipe is planned or was eaten less than `days` days from date.
func (c *suggestCtx) within(recipeID string, date time.Time, days int, planned []MealItem) bool {
	check := func(items []MealItem) bool {
		for _, m := range items {
			if m.RecipeID == nil || *m.RecipeID != recipeID || m.Leftover {
				continue
			}
			d, _ := time.Parse(time.DateOnly, m.Date)
			if diff := math.Abs(d.Sub(date).Hours() / 24); diff < float64(days) && !d.Equal(date) {
				return true
			}
		}
		return false
	}
	return check(c.history) || check(planned)
}

func (c *suggestCtx) recipe(id string) *Recipe {
	if c.byID == nil {
		c.byID = map[string]*Recipe{}
		for i := range c.recipes {
			c.byID[c.recipes[i].ID] = &c.recipes[i]
		}
	}
	return c.byID[id]
}

func (c *suggestCtx) eatersOf(m MealItem) map[string]float64 {
	if m.Portions != nil {
		return m.Portions
	}
	if m.RecipeID != nil {
		if r := c.recipe(*m.RecipeID); r != nil {
			return c.hh.defaultEaters(r.Nutrition.Contains)
		}
	}
	return c.hh.defaultEaters(nil)
}

// intake sums what each person eats on the given days of the plan.
func (c *suggestCtx) intake(planned []MealItem, from, to time.Time) map[string]*nutrition.Nutrients {
	out := map[string]*nutrition.Nutrients{}
	for id := range c.targets {
		out[id] = &nutrition.Nutrients{}
	}
	for _, m := range plannedBetween(planned, from, to) {
		if m.RecipeID == nil {
			continue
		}
		r := c.recipe(*m.RecipeID)
		if r == nil {
			continue
		}
		for id, portion := range c.eatersOf(m) {
			if n := out[id]; n != nil {
				n.Add(r.Nutrition.PerServing, portion)
			}
		}
	}
	return out
}

// Weights for nutrients worth chasing; protein, fibre and iron are the usual shortfalls
// in Indian home diets (ICMR-NIN 2024 dietary guidelines).
var nutrientWeights = map[string]float64{"protein": 2, "fibre": 1.5, "iron": 1.5, "calcium": 1.2, "b12": 1, "folate": 0.8, "vitC": 0.6, "vitA": 0.6, "zinc": 0.6, "magnesium": 0.5, "potassium": 0.5}

var nutrientLabels = map[string]string{"protein": "protein", "fibre": "fibre", "iron": "iron", "calcium": "calcium", "b12": "vitamin B12", "folate": "folate", "vitC": "vitamin C", "vitA": "vitamin A", "zinc": "zinc", "magnesium": "magnesium", "potassium": "potassium"}

// nutritionBonus scores how much a dish closes the eaters' nutrient gaps for the week so
// far, and penalises calorie-dense dishes for anyone losing weight.
func (c *suggestCtx) nutritionBonus(r *Recipe, eaters map[string]float64, intake map[string]*nutrition.Nutrients, days float64) (float64, string) {
	if len(c.targets) == 0 || r.Nutrition.Coverage < 0.5 || r.Nutrition.PerServing.Kcal <= 0 {
		return 0, ""
	}
	ps := r.Nutrition.PerServing
	bonus, best, bestKey := 0.0, 0.0, ""
	for id, portion := range eaters {
		t := c.targets[id]
		if t == nil || portion <= 0 {
			continue
		}
		had := intake[id]
		for k, w := range nutrientWeights {
			target := t.Get(k)
			if target <= 0 {
				continue
			}
			deficit := math.Min(math.Max(0, target*days-had.Get(k))/target, 1.5)
			share := ps.Get(k) * portion / target
			gain := w * math.Min(share, deficit)
			bonus += gain
			if gain > best && share >= 0.2 {
				best, bestKey = gain, k
			}
		}
		if p := c.hh.profiles[id]; p != nil && p.Goal == "lose" && t.Kcal > 0 {
			// One dish shouldn't take more than ~30% of a reduced-calorie day.
			bonus -= math.Max(0, ps.Kcal*portion-0.3*t.Kcal) / t.Kcal * 6
			if ps.Kcal > 0 && ps.Fat*9/ps.Kcal > 0.5 {
				bonus -= 0.5
			}
		}
		if t.Sodium > 0 && had.Sodium > t.Sodium*days {
			bonus -= ps.Sodium * portion / t.Sodium
		}
	}
	why := ""
	if bestKey != "" {
		why = "Good source of " + nutrientLabels[bestKey]
	}
	return bonus * 2, why
}
