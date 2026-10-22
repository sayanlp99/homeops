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
	recipes  []Recipe
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
		if err := rows.Scan(&m.ID, &m.Date, &m.Slot, &m.Position, &m.RecipeID, &m.Title, &m.Servings, &m.Note, &m.CookedAt, &m.CookedBy, &m.CreatedAt); err != nil {
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

	c := &suggestCtx{recipes: recipes, history: history, pantry: pantry, today: today,
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

// autofillMeals plans the top suggestion into every empty slot in the range.
func (s *Server) autofillMeals(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		From  string   `json:"from"`
		To    string   `json:"to"`
		Slots []string `json:"slots"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	from, err := parseDate(in.From)
	if err != nil {
		return err
	}
	to, err := parseDate(in.To)
	if err != nil {
		return err
	}
	if to.Before(from) || to.Sub(from) > 14*24*time.Hour {
		return errBadRequest("Pick at most two weeks")
	}
	if len(in.Slots) == 0 {
		in.Slots = mealSlots
	}
	h, err := s.getHousehold(ctx, u.HouseholdID)
	if err != nil {
		return err
	}
	c, err := s.loadSuggestCtx(ctx, h)
	if err != nil {
		return err
	}
	planned := plannedBetween(c.history, weekStart(from, h.SprintStartDow), to.AddDate(0, 0, 6))
	filled := map[string]bool{}
	for _, m := range planned {
		filled[m.Date+"/"+m.Slot] = true
	}
	created := 0
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if d.Before(c.today) {
			continue
		}
		for _, slot := range mealSlots {
			key := d.Format(time.DateOnly) + "/" + slot
			if !slices.Contains(in.Slots, slot) || filled[key] {
				continue
			}
			picks := c.plate(d, slot, planned)
			for _, best := range picks {
				var servings int
				for _, rc := range c.recipes {
					if rc.ID == best.RecipeID {
						servings = rc.Servings
					}
				}
				m := MealItem{ID: randomUUID(), Date: d.Format(time.DateOnly), Slot: slot, RecipeID: &best.RecipeID, Title: best.Title, Servings: max(servings, 1), Position: float64(len(planned))}
				if _, err := s.db.Exec(ctx, `INSERT INTO meal_plan_items (id, household_id, date, slot, position, recipe_id, title, servings)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, m.ID, u.HouseholdID, m.Date, slot, m.Position, best.RecipeID, best.Title, m.Servings); err != nil {
					return err
				}
				planned = append(planned, m)
				created++
			}
		}
	}
	if created > 0 {
		s.hub.Publish(u.HouseholdID, eventMeals.from(r))
	}
	return ok(w, map[string]int{"created": created})
}
