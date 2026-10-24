package api

import (
	"context"
	"math"
	"net/http"
	"slices"
	"time"
)

// Auto-fill. When the recipe box describes a daily routine (a dish tagged "staple" for
// rice), each day is built the way a Bengali home kitchen runs:
//
//	breakfast  one breakfast dish
//	lunch      rice + dal + a vegetable + protein for everyone; fish goes to whoever
//	           eats it and the others get their own protein. One quick day a week is a
//	           one-pot dal-with-vegetables plus a bhaja. Saturday lunch is khichdi when
//	           it's due; Sunday lunch is the weekend special (pulao set) when it's due.
//	snacks     one seasonal fruit per person
//	dairy      one a day for calcium (curd or buttermilk at lunch, milk at breakfast),
//	           never the same one two days running
//	dinner     chapati + a simple curry, leftovers of lunch two nights, a special one
//	           night, eating out one night
//
// Dishes are scored with the usual suggestion rules (variety, season, "every:N"
// spacing, history) plus nutrition: each pick closes the eaters' gaps for the week.
// Finally each person's rice/chapati portion is sized so the day lands near their
// calorie target. Without a staple, it falls back to filling each slot with a plate.

// Dinner pattern by weekday; the user can change any night afterwards.
var dinnerPattern = map[time.Weekday]string{
	time.Monday: "cook", time.Tuesday: "leftover", time.Wednesday: "cook", time.Thursday: "leftover",
	time.Friday: "special", time.Saturday: "out", time.Sunday: "cook",
}

const (
	quickLunchDay   = time.Wednesday
	khichdiDay      = time.Saturday
	weekendSpecDay  = time.Sunday
	maxFriedPerWeek = 3
	eatingOutTitle  = "Eating out / order in"
)

// role classifies a recipe for the routine.
func (r Recipe) role() string {
	switch {
	case r.hasTag("fruit"):
		return "fruit"
	case r.hasTag("dairy"):
		return "dairy"
	case r.hasTag("staple") && r.Category == "rice":
		return "rice"
	case r.hasTag("staple") && r.Category == "bread":
		return "bread"
	case r.hasTag("weekend-special"):
		return "weekend"
	case r.hasTag("dinner-special"):
		return "special"
	case r.hasTag("khichdi"):
		return "khichdi"
	case r.Category == "dal" && r.hasTag("one-pot"):
		return "dalveg"
	case r.Category == "dal":
		return "dal"
	case r.isProtein():
		return "protein"
	case r.Category == "breakfast":
		return "breakfast"
	case r.hasTag("fried") || r.Category == "side":
		return "side"
	case r.Category == "sabzi" || r.Category == "curry":
		return "veg"
	}
	return ""
}

func (r Recipe) isProtein() bool {
	if r.hasTag("protein") {
		return true
	}
	if r.Category == "breakfast" || r.Category == "rice" || r.Category == "bread" {
		return false
	}
	for _, g := range r.Nutrition.Contains {
		if g == "fish" || g == "seafood" || g == "chicken" || g == "mutton" || g == "egg" {
			return true
		}
	}
	return false
}

type plannedDish struct {
	recipe   *Recipe
	title    string
	slot     string
	portions map[string]float64 // nil = default eaters
	leftover bool
	from     int // index of the dish a leftover comes from, -1 otherwise
	staple   bool
}

type planner struct {
	s       *Server
	c       *suggestCtx
	ctx     context.Context
	hid     string
	planned []MealItem
	created int
}

func (p *planner) eaters(d plannedDish) map[string]float64 {
	if d.portions != nil {
		return d.portions
	}
	if d.recipe == nil {
		return p.c.hh.defaultEaters(nil)
	}
	return p.c.hh.defaultEaters(d.recipe.Nutrition.Contains)
}

// pick returns the best-scoring recipe for the slot that passes ok.
func (p *planner) pick(date time.Time, slot string, extra []MealItem, ok func(*Recipe) bool) *Recipe {
	for _, sg := range p.c.suggest(date, slot, append(append([]MealItem{}, p.planned...), extra...), 200) {
		r := p.c.recipe(sg.RecipeID)
		if r == nil || !ok(r) {
			continue
		}
		if len(r.Months) > 0 && !slices.Contains(r.Months, int(date.Month())) {
			continue
		}
		if gap := r.minGapDays(); gap > 0 && p.c.within(r.ID, date, gap, append(append([]MealItem{}, p.planned...), extra...)) {
			continue
		}
		return r
	}
	return nil
}

func (p *planner) plannedOn(recipeID string, date time.Time) bool {
	ds := date.Format(time.DateOnly)
	for _, m := range p.planned {
		if m.Date == ds && m.RecipeID != nil && *m.RecipeID == recipeID {
			return true
		}
	}
	return false
}

// lastPlanned is the latest day before date the recipe was planned (zero if never).
func (p *planner) lastPlanned(recipeID string, date time.Time) time.Time {
	var last time.Time
	for _, m := range p.planned {
		if m.RecipeID == nil || *m.RecipeID != recipeID {
			continue
		}
		if d, err := time.Parse(time.DateOnly, m.Date); err == nil && d.Before(date) && d.After(last) {
			last = d
		}
	}
	return last
}

func hasRole(role string) func(*Recipe) bool { return func(r *Recipe) bool { return r.role() == role } }

// asItems turns the day's draft into plan items so scoring sees them.
func asItems(date time.Time, dishes []plannedDish) []MealItem {
	out := make([]MealItem, 0, len(dishes))
	for _, d := range dishes {
		m := MealItem{Date: date.Format(time.DateOnly), Slot: d.slot, Title: d.title, Portions: d.portions, Leftover: d.leftover}
		if d.recipe != nil {
			id := d.recipe.ID
			m.RecipeID = &id
		}
		out = append(out, m)
	}
	return out
}

func (p *planner) friedThisWeek(date time.Time, draft []MealItem) int {
	week := weekStart(date, p.c.weekDow)
	n := 0
	for _, m := range plannedBetween(append(append([]MealItem{}, p.planned...), draft...), week, week.AddDate(0, 0, 6)) {
		if m.RecipeID != nil && !m.Leftover {
			if r := p.c.recipe(*m.RecipeID); r != nil && r.hasTag("fried") {
				n++
			}
		}
	}
	return n
}

func (p *planner) planDay(date time.Time, slots []string, filled map[string]bool) []plannedDish {
	var day []plannedDish
	add := func(r *Recipe, slot string) int {
		if r == nil {
			return -1
		}
		day = append(day, plannedDish{recipe: r, title: r.Title, slot: slot, from: -1, staple: r.role() == "rice" || r.role() == "bread"})
		return len(day) - 1
	}
	want := func(slot string) bool { return slices.Contains(slots, slot) && !filled[slot] }
	draft := func() []MealItem { return asItems(date, day) }
	shared := func(r *Recipe) bool { // everyone at the table eats it
		return len(p.c.hh.defaultEaters(r.Nutrition.Contains)) == len(p.c.hh.defaultEaters(nil))
	}

	if want("breakfast") {
		add(p.pick(date, "breakfast", draft(), func(r *Recipe) bool { return r.Category == "breakfast" && !r.hasTag("fruit") }), "breakfast")
	}

	if want("lunch") {
		switch {
		case date.Weekday() == weekendSpecDay && p.pick(date, "lunch", draft(), hasRole("weekend")) != nil:
			for i := range p.c.recipes {
				if r := &p.c.recipes[i]; r.role() == "weekend" {
					add(r, "lunch")
				}
			}
		case date.Weekday() == khichdiDay && p.pick(date, "lunch", draft(), hasRole("khichdi")) != nil:
			add(p.pick(date, "lunch", draft(), hasRole("khichdi")), "lunch")
			// The khichdi companion (palong shaak ghonto) at most once a month.
			if g := p.pick(date, "lunch", draft(), func(r *Recipe) bool { return r.hasTag("with-khichdi") && !p.c.within(r.ID, date, 28, p.planned) }); g != nil {
				add(g, "lunch")
			}
			if p.friedThisWeek(date, draft()) < maxFriedPerWeek {
				add(p.pick(date, "lunch", draft(), func(r *Recipe) bool { return r.role() == "side" && !r.hasTag("with-khichdi") }), "lunch")
			}
			p.addProteins(date, "lunch", &day, true) // khichdi has dal protein; top up only if the day needs it
		default:
			add(p.pick(date, "lunch", draft(), hasRole("rice")), "lunch")
			if date.Weekday() == quickLunchDay {
				add(p.pick(date, "lunch", draft(), func(r *Recipe) bool { return r.role() == "dalveg" && shared(r) }), "lunch")
				side := func(r *Recipe) bool {
					return r.role() == "side" && shared(r) && (!r.hasTag("fried") || p.friedThisWeek(date, draft()) < maxFriedPerWeek)
				}
				add(p.pick(date, "lunch", draft(), side), "lunch")
			} else {
				add(p.pick(date, "lunch", draft(), func(r *Recipe) bool { return r.role() == "dal" && shared(r) }), "lunch")
				add(p.pick(date, "lunch", draft(), func(r *Recipe) bool { return r.role() == "veg" && shared(r) }), "lunch")
			}
			p.addProteins(date, "lunch", &day, false)
		}
	}

	if want("snacks") {
		add(p.pick(date, "snacks", draft(), hasRole("fruit")), "snacks")
	}

	// One dairy item a day, a different one from yesterday's.
	var dairy *Recipe
	dairySlot := ""
	for _, slot := range []string{"lunch", "breakfast"} {
		if !want(slot) {
			continue
		}
		r := p.pick(date, slot, draft(), func(r *Recipe) bool {
			return r.role() == "dairy" && slices.Contains(r.MealTypes, slot) && !p.plannedOn(r.ID, date.AddDate(0, 0, -1))
		})
		if r != nil && (dairy == nil || p.lastPlanned(r.ID, date).Before(p.lastPlanned(dairy.ID, date))) {
			dairy, dairySlot = r, slot
		}
	}
	add(dairy, dairySlot)

	if want("dinner") {
		switch dinnerPattern[date.Weekday()] {
		case "out":
			day = append(day, plannedDish{title: eatingOutTitle, slot: "dinner", from: -1})
		case "leftover":
			n := len(day)
			for i := 0; i < n; i++ {
				if day[i].slot == "lunch" && day[i].recipe != nil && day[i].recipe.role() != "dairy" {
					d := day[i]
					d.slot, d.leftover, d.from = "dinner", true, i
					if d.portions != nil {
						d.portions = cloneMap(d.portions)
					}
					day = append(day, d)
				}
			}
			if len(day) > n {
				break
			}
			fallthrough // nothing at lunch to eat again
		case "special":
			if dinnerPattern[date.Weekday()] == "special" {
				if i := add(p.pick(date, "dinner", draft(), func(r *Recipe) bool { return r.hasTag("dinner-special") }), "dinner"); i >= 0 {
					p.addProteins(date, "dinner", &day, true)
					break
				}
			}
			fallthrough
		default:
			add(p.pick(date, "dinner", draft(), hasRole("bread")), "dinner")
			// Her list of simple dinners first, then anything light that suits dinner.
			listed := func(r *Recipe) bool { return shared(r) && r.hasTag("simple-dinner") }
			simple := func(r *Recipe) bool {
				role := r.role()
				return shared(r) && slices.Contains(r.MealTypes, "dinner") && (role == "veg" || role == "dal")
			}
			if c := p.pick(date, "dinner", draft(), listed); c != nil {
				add(c, "dinner")
			} else {
				add(p.pick(date, "dinner", draft(), simple), "dinner")
			}
			p.addProteins(date, "dinner", &day, true)
		}
	}
	return day
}

// addProteins makes sure everyone gets protein in this meal: the best protein dish for
// those who eat it, then, for anyone it doesn't suit (fish for a non-fish-eater) or who
// is short on protein today, a dish of their own. With onlyIfShort, only people still
// short on protein for the day get one.
func (p *planner) addProteins(date time.Time, slot string, day *[]plannedDish, onlyIfShort bool) {
	covered := map[string]bool{}
	for _, d := range *day {
		if d.slot == slot && d.recipe != nil && d.recipe.isProtein() {
			for id, v := range p.eaters(d) {
				if v > 0 {
					covered[id] = true
				}
			}
		}
	}
	everyone := p.c.hh.defaultEaters(nil)
	need := func() []string {
		var out []string
		for id := range everyone {
			if covered[id] {
				continue
			}
			if onlyIfShort && !p.shortOnProtein(date, id, *day) {
				continue
			}
			out = append(out, id)
		}
		slices.Sort(out)
		return out
	}
	for range 3 {
		ids := need()
		if len(ids) == 0 {
			return
		}
		draft := asItems(date, *day)
		r := p.pick(date, slot, draft, func(r *Recipe) bool {
			if r.role() != "protein" || r.hasTag("weekend-special") || (slot == "dinner" && !slices.Contains(r.MealTypes, "dinner") && len(r.MealTypes) > 0) {
				return false
			}
			eats := p.c.hh.defaultEaters(r.Nutrition.Contains)
			for _, id := range ids {
				if eats[id] == 0 {
					return false
				}
			}
			return true
		})
		if r == nil {
			return
		}
		d := plannedDish{recipe: r, title: r.Title, slot: slot, from: -1}
		eats := p.c.hh.defaultEaters(r.Nutrition.Contains)
		if len(ids) < len(eats) {
			d.portions = map[string]float64{}
			for id := range eats {
				d.portions[id] = 0
			}
			for _, id := range ids {
				d.portions[id] = 1
			}
		}
		*day = append(*day, d)
		for _, id := range ids {
			covered[id] = true
		}
	}
}

func (p *planner) shortOnProtein(date time.Time, userID string, day []plannedDish) bool {
	t := p.c.targets[userID]
	if t == nil {
		return false
	}
	got := p.c.intake(append(append([]MealItem{}, p.planned...), asItems(date, day)...), date, date)[userID]
	return got != nil && got.Protein < t.Protein*0.85
}

// balance sizes each person's portions so the day lands near their calorie target.
// Over target, portions shrink in this order: rice/chapati (to ½), then fried sides,
// specials and breakfast (to ½), then dal and vegetables (to ½), and protein last (to ¾,
// only while protein is above target). Under target, only rice/chapati grow (up to 3).
// Fruit and dairy are never cut. A meal out is budgeted at about a third of the day.
func (p *planner) balance(date time.Time, day []plannedDish) {
	eatingOut := false
	for _, d := range day {
		eatingOut = eatingOut || (d.recipe == nil && d.title == eatingOutTitle)
	}
	tier := func(d plannedDish) (int, float64) {
		switch role := d.recipe.role(); {
		case d.staple:
			return 1, 0.5
		case role == "fruit" || role == "dairy":
			return 0, 0
		case role == "side" || role == "special" || role == "weekend" && !d.recipe.isProtein() || d.recipe.Category == "breakfast" || role == "khichdi":
			return 2, 0.5
		case d.recipe.isProtein():
			return 4, 0.75
		default:
			return 3, 0.5
		}
	}
	for id, t := range p.c.targets {
		var kcal, protein float64
		if had := p.c.intake(p.planned, date, date)[id]; had != nil { // planned outside this run
			kcal, protein = had.Kcal, had.Protein
		}
		if eatingOut {
			kcal += t.Kcal * 0.35 // leave room for the meal out
		}
		for i := range day {
			d := &day[i]
			if d.recipe == nil {
				continue
			}
			portion := p.eaters(*d)[id]
			kcal += d.recipe.Nutrition.PerServing.Kcal * portion
			protein += d.recipe.Nutrition.PerServing.Protein * portion
		}
		if kcal == 0 || math.Abs(t.Kcal-kcal) < t.Kcal*0.05 {
			continue
		}
		setPortion := func(i int, v float64) {
			d := &day[i]
			if d.portions == nil {
				d.portions = cloneMap(p.c.hh.defaultEaters(d.recipe.Nutrition.Contains))
			}
			old := d.portions[id]
			d.portions[id] = v
			kcal += (v - old) * d.recipe.Nutrition.PerServing.Kcal
			protein += (v - old) * d.recipe.Nutrition.PerServing.Protein
		}
		if kcal < t.Kcal { // grow the staples
			var staples []int
			for i, d := range day {
				if d.recipe != nil && d.staple && p.eaters(d)[id] > 0 && d.recipe.Nutrition.PerServing.Kcal > 0 {
					staples = append(staples, i)
				}
			}
			for _, i := range staples {
				d := day[i]
				cur := p.eaters(d)[id]
				next := cur + (t.Kcal-kcal)/float64(len(staples))/d.recipe.Nutrition.PerServing.Kcal
				setPortion(i, math.Min(math.Round(next*4)/4, 3))
			}
			continue
		}
		for level := 1; level <= 4 && kcal > t.Kcal*1.03; level++ {
			if level == 4 && protein <= t.Protein {
				break
			}
			// How much this tier can give up, then shrink everything in it proportionally.
			var idx []int
			room := 0.0
			for i, d := range day {
				if d.recipe == nil {
					continue
				}
				lv, floor := tier(d)
				if cur := p.eaters(d)[id]; lv == level && cur > floor {
					idx = append(idx, i)
					room += (cur - floor) * d.recipe.Nutrition.PerServing.Kcal
				}
			}
			if room <= 0 {
				continue
			}
			f := math.Min(1, (kcal-t.Kcal)/room)
			for _, i := range idx {
				d := day[i]
				_, floor := tier(d)
				cur := p.eaters(d)[id]
				next := math.Max(math.Round((cur-f*(cur-floor))*4)/4, floor)
				setPortion(i, next)
			}
		}
	}
}

func cloneMap(m map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// servingsFor: enough for everyone eating it now and later as leftovers, never less than
// the recipe makes.
func (p *planner) servingsFor(day []plannedDish, i int) int {
	d := day[i]
	if d.recipe == nil {
		return max(len(p.c.hh.defaultEaters(nil)), 1)
	}
	if d.leftover {
		return max(int(math.Ceil(sum(p.eaters(d)))), 1)
	}
	total := sum(p.eaters(d))
	for _, o := range day {
		if o.leftover && o.from == i {
			total += sum(p.eaters(o))
		}
	}
	return min(max(int(math.Ceil(total-1e-9)), d.recipe.Servings, 1), 50)
}

func sum(m map[string]float64) float64 {
	t := 0.0
	for _, v := range m {
		t += v
	}
	return t
}

func (p *planner) save(date time.Time, day []plannedDish) error {
	pos := map[string]float64{}
	for i, d := range day {
		m := MealItem{ID: randomUUID(), Date: date.Format(time.DateOnly), Slot: d.slot, Title: d.title, Servings: p.servingsFor(day, i), Portions: d.portions, Leftover: d.leftover}
		pos[d.slot]++
		m.Position = pos[d.slot]
		var rid *string
		if d.recipe != nil {
			id := d.recipe.ID
			rid = &id
		}
		m.RecipeID = rid
		if d.leftover {
			m.Note = "Leftover from lunch"
		}
		if _, err := p.s.db.Exec(p.ctx, `INSERT INTO meal_plan_items (id, household_id, date, slot, position, recipe_id, title, servings, note, portions, leftover)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			m.ID, p.hid, m.Date, m.Slot, m.Position, rid, m.Title, m.Servings, m.Note, m.Portions, m.Leftover); err != nil {
			return err
		}
		p.planned = append(p.planned, m)
		p.created++
	}
	return nil
}

// autofillMeals plans every empty slot in the range.
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
	p := &planner{s: s, c: c, ctx: ctx, hid: u.HouseholdID, planned: plannedBetween(c.history, weekStart(from, h.SprintStartDow), to.AddDate(0, 0, 6))}
	routine := false
	for i := range c.recipes {
		if c.recipes[i].role() == "rice" {
			routine = true
		}
	}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if d.Before(c.today) {
			continue
		}
		filled := map[string]bool{}
		for _, m := range p.planned {
			if m.Date == d.Format(time.DateOnly) {
				filled[m.Slot] = true
			}
		}
		if routine {
			day := p.planDay(d, in.Slots, filled)
			p.balance(d, day)
			if err := p.save(d, day); err != nil {
				return err
			}
			continue
		}
		for _, slot := range mealSlots {
			if !slices.Contains(in.Slots, slot) || filled[slot] {
				continue
			}
			var day []plannedDish
			for _, best := range c.plate(d, slot, p.planned) {
				if rc := c.recipe(best.RecipeID); rc != nil {
					day = append(day, plannedDish{recipe: rc, title: rc.Title, slot: slot, from: -1})
				}
			}
			if err := p.save(d, day); err != nil {
				return err
			}
		}
	}
	if p.created > 0 {
		s.hub.Publish(u.HouseholdID, eventMeals.from(r))
	}
	return ok(w, map[string]int{"created": p.created})
}
