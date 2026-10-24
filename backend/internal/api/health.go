package api

import (
	"context"
	"math"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"homeops/internal/nutrition"
)

// Health profiles and nutrition. A profile belongs to a person and follows them across
// households; any member (not viewer) of a household they're in can see and edit it,
// so a couple can keep each other's details up to date.

type HealthProfile struct {
	UserID    string             `json:"userId" db:"user_id"`
	Sex       string             `json:"sex" db:"sex"`
	BirthYear int                `json:"birthYear" db:"birth_year"`
	HeightCm  float64            `json:"heightCm" db:"height_cm"`
	WeightKg  float64            `json:"weightKg" db:"weight_kg"`
	Activity  string             `json:"activity" db:"activity"`
	Goal      string             `json:"goal" db:"goal"`
	Avoid     []string           `json:"avoid" db:"avoid"`
	Overrides map[string]float64 `json:"overrides" db:"overrides"`
	UpdatedAt time.Time          `json:"updatedAt" db:"updated_at"`
	UpdatedBy *string            `json:"updatedBy" db:"updated_by"`
}

// Food groups a person can choose to avoid; recipes "contain" these from their ingredients.
var avoidGroups = []string{"fish", "seafood", "chicken", "mutton", "egg", "dairy"}

func (p *HealthProfile) age(now time.Time) int { return now.Year() - p.BirthYear }

func (p *HealthProfile) profile(now time.Time) nutrition.Profile {
	return nutrition.Profile{Sex: p.Sex, Age: p.age(now), HeightCm: p.HeightCm, WeightKg: p.WeightKg, Activity: p.Activity, Goal: p.Goal}
}

// targets are the computed daily targets with any hand-set overrides applied.
func (p *HealthProfile) targets(now time.Time) (computed, final nutrition.Targets) {
	computed = nutrition.DailyTargets(p.profile(now))
	final = computed
	set := map[string]*float64{"kcal": &final.Kcal, "protein": &final.Protein, "carbs": &final.Carbs, "fat": &final.Fat, "fibre": &final.Fibre,
		"iron": &final.Iron, "calcium": &final.Calcium, "vitC": &final.VitC, "vitA": &final.VitA, "folate": &final.Folate, "b12": &final.B12,
		"zinc": &final.Zinc, "magnesium": &final.Magnesium, "potassium": &final.Potassium, "sodium": &final.Sodium}
	for k, v := range p.Overrides {
		if ptr := set[k]; ptr != nil && v > 0 {
			*ptr = v
		}
	}
	return computed, final
}

const healthColumns = `user_id, sex, birth_year, height_cm, weight_kg, activity, goal, avoid, overrides, updated_at, updated_by`

type memberInfo struct {
	ID          string `db:"id"`
	DisplayName string `db:"display_name"`
	Color       string `db:"color"`
	Role        string `db:"role"`
}

type householdHealth struct {
	members  []memberInfo
	profiles map[string]*HealthProfile
}

func (s *Server) loadHouseholdHealth(ctx context.Context, householdID string) (*householdHealth, error) {
	rows, _ := s.db.Query(ctx, `SELECT id, display_name, color, role FROM members WHERE household_id = $1 ORDER BY created_at`, householdID)
	members, err := pgx.CollectRows(rows, pgx.RowToStructByName[memberInfo])
	if err != nil {
		return nil, err
	}
	rows, _ = s.db.Query(ctx, `SELECT `+healthColumns+` FROM health_profiles
		WHERE user_id IN (SELECT id FROM members WHERE household_id = $1)`, householdID)
	profiles, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[HealthProfile])
	if err != nil {
		return nil, err
	}
	hh := &householdHealth{members: members, profiles: map[string]*HealthProfile{}}
	for _, p := range profiles {
		hh.profiles[p.UserID] = p
	}
	return hh, nil
}

// defaultEaters: one serving for every member (viewers excepted, they're often the cook
// or a guest) whose avoid list doesn't clash with what the dish contains.
func (hh *householdHealth) defaultEaters(contains []string) map[string]float64 {
	out := map[string]float64{}
	for _, m := range hh.members {
		if m.Role == "viewer" || hh.avoids(m.ID, contains) {
			continue
		}
		out[m.ID] = 1
	}
	return out
}

func (hh *householdHealth) avoids(userID string, contains []string) bool {
	p := hh.profiles[userID]
	if p == nil {
		return false
	}
	for _, c := range contains {
		if slices.Contains(p.Avoid, c) {
			return true
		}
	}
	return false
}

// resolveEaters fills in each item's Eaters.
func (s *Server) resolveEaters(ctx context.Context, householdID string, items []MealItem) error {
	if len(items) == 0 {
		return nil
	}
	hh, err := s.loadHouseholdHealth(ctx, householdID)
	if err != nil {
		return err
	}
	var ids []string
	for _, m := range items {
		if m.RecipeID != nil && m.Portions == nil && !slices.Contains(ids, *m.RecipeID) {
			ids = append(ids, *m.RecipeID)
		}
	}
	contains := map[string][]string{}
	if len(ids) > 0 {
		recipes, err := s.queryRecipes(ctx, householdID, "AND r.id = ANY($2::uuid[])", ids)
		if err != nil {
			return err
		}
		for _, rc := range recipes {
			contains[rc.ID] = rc.Nutrition.Contains
		}
	}
	for i := range items {
		switch {
		case items[i].Portions != nil:
			items[i].Eaters = items[i].Portions
		case items[i].RecipeID != nil:
			items[i].Eaters = hh.defaultEaters(contains[*items[i].RecipeID])
		default:
			items[i].Eaters = hh.defaultEaters(nil)
		}
	}
	return nil
}

// cleanPortions checks that every key is a member of the household and rounds
// servings to quarters. nil stays nil (the default); {} means nobody (e.g. a guest dish).
func (s *Server) cleanPortions(ctx context.Context, householdID string, in map[string]float64) (map[string]float64, error) {
	if in == nil {
		return nil, nil
	}
	if len(in) > 50 {
		return nil, errBadRequest("Too many people")
	}
	var ids []string
	if err := s.db.QueryRow(ctx, `SELECT coalesce(array_agg(id::text), '{}') FROM members WHERE household_id = $1`, householdID).Scan(&ids); err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for id, v := range in {
		if !slices.Contains(ids, id) {
			return nil, errBadRequest("Portions can only be for household members")
		}
		if v < 0 || v > 10 || math.IsNaN(v) {
			return nil, errBadRequest("A portion must be 0–10 servings")
		}
		out[id] = math.Round(v*4) / 4
	}
	return out, nil
}

// ---- handlers ----

type memberHealthOut struct {
	UserID   string             `json:"userId"`
	Name     string             `json:"name"`
	Color    string             `json:"color"`
	Profile  *HealthProfile     `json:"profile"`
	Age      int                `json:"age,omitempty"`
	Computed *nutrition.Targets `json:"computed"` // from the profile alone
	Targets  *nutrition.Targets `json:"targets"`  // with overrides
}

func (s *Server) listHealth(w http.ResponseWriter, r *http.Request) error {
	hh, err := s.loadHouseholdHealth(r.Context(), currentUser(r).HouseholdID)
	if err != nil {
		return err
	}
	now := time.Now()
	out := []memberHealthOut{}
	for _, m := range hh.members {
		if m.Role == "viewer" {
			continue
		}
		o := memberHealthOut{UserID: m.ID, Name: m.DisplayName, Color: m.Color}
		if p := hh.profiles[m.ID]; p != nil {
			c, t := p.targets(now)
			o.Profile, o.Age, o.Computed, o.Targets = p, p.age(now), &c, &t
		}
		out = append(out, o)
	}
	return ok(w, map[string]any{"members": out, "avoidGroups": avoidGroups})
}

func (s *Server) saveHealth(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	userID := chi.URLParam(r, "userId")
	var member bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM members WHERE household_id = $1 AND id = $2)`, u.HouseholdID, userID).Scan(&member); err != nil {
		return err
	}
	if !member {
		return errNotFound("Not a member of this household")
	}
	var in HealthProfile
	if err := decode(r, &in); err != nil {
		return err
	}
	now := time.Now()
	if in.Activity == "" {
		in.Activity = "light"
	}
	if in.Goal == "" {
		in.Goal = "maintain"
	}
	in.HeightCm = math.Round(in.HeightCm*10) / 10
	in.WeightKg = math.Round(in.WeightKg*10) / 10
	if !in.profile(now).Valid() {
		return errBadRequest("Enter sex, age (14–100), height (100–230 cm) and weight (25–250 kg)")
	}
	if !slices.Contains([]string{"sedentary", "light", "moderate", "active"}, in.Activity) || !slices.Contains([]string{"lose", "maintain", "gain"}, in.Goal) {
		return errBadRequest("Unknown activity level or goal")
	}
	avoid := []string{}
	for _, a := range in.Avoid {
		if a = strings.ToLower(strings.TrimSpace(a)); slices.Contains(avoidGroups, a) && !slices.Contains(avoid, a) {
			avoid = append(avoid, a)
		}
	}
	overrides := map[string]float64{}
	for k, v := range in.Overrides {
		if !slices.Contains(nutrition.Keys, k) {
			return errBadRequest("Unknown nutrient " + k)
		}
		if v < 0 || v > 20000 || math.IsNaN(v) {
			return errBadRequest("Target out of range")
		}
		if v > 0 {
			overrides[k] = v
		}
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO health_profiles (user_id, sex, birth_year, height_cm, weight_kg, activity, goal, avoid, overrides, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id) DO UPDATE SET sex = $2, birth_year = $3, height_cm = $4, weight_kg = $5, activity = $6, goal = $7,
			avoid = $8, overrides = $9, updated_by = $10, updated_at = now()`,
		userID, in.Sex, in.BirthYear, in.HeightCm, in.WeightKg, in.Activity, in.Goal, avoid, overrides, u.ID); err != nil {
		return err
	}
	s.publishHealth(ctx, r, userID)
	return s.listHealth(w, r)
}

func (s *Server) deleteHealth(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	userID := chi.URLParam(r, "userId")
	if _, err := s.db.Exec(r.Context(), `DELETE FROM health_profiles WHERE user_id = $1
		AND user_id IN (SELECT id FROM members WHERE household_id = $2)`, userID, u.HouseholdID); err != nil {
		return err
	}
	s.publishHealth(r.Context(), r, userID)
	return noContent(w)
}

// A profile shows in every household the person is in.
func (s *Server) publishHealth(ctx context.Context, r *http.Request, userID string) {
	rows, err := s.db.Query(ctx, `SELECT household_id FROM memberships WHERE user_id = $1`, userID)
	if err != nil {
		return
	}
	hids, _ := pgx.CollectRows(rows, pgx.RowTo[string])
	for _, h := range hids {
		s.hub.Publish(h, eventMeals.from(r))
	}
}

func (s *Server) listFoods(w http.ResponseWriter, r *http.Request) error {
	return ok(w, nutrition.Foods())
}

// ---- report ----

type dayIntake struct {
	Date    string              `json:"date"`
	Totals  nutrition.Nutrients `json:"totals"`
	Dishes  int                 `json:"dishes"`
	Unknown []string            `json:"unknown"` // eaten, but no nutrition (eating out, free-text dish)
}

type nutrientIdea struct {
	RecipeID string  `json:"recipeId"`
	Title    string  `json:"title"`
	Amount   float64 `json:"amount"`  // per serving
	Percent  int     `json:"percent"` // of the daily target
}

type nutrientGap struct {
	Nutrient string         `json:"nutrient"`
	Kind     string         `json:"kind"`    // low | high
	Percent  int            `json:"percent"` // average as % of target
	Ideas    []nutrientIdea `json:"ideas"`
}

type personReport struct {
	UserID  string              `json:"userId"`
	Name    string              `json:"name"`
	Color   string              `json:"color"`
	Targets *nutrition.Targets  `json:"targets"`
	Days    []dayIntake         `json:"days"`
	Average nutrition.Nutrients `json:"average"` // per day, over the counted days
	Counted int                 `json:"counted"` // days with food planned and nothing unknown
	Partial int                 `json:"partial"` // days left out because something eaten had no nutrition
	Gaps    []nutrientGap       `json:"gaps"`
}

// Nutrients that drive "low" gaps, in priority order for Indian vegetarian-leaning diets.
var gapNutrients = []string{"protein", "fibre", "iron", "calcium", "b12", "folate", "vitC", "vitA", "zinc", "magnesium", "potassium"}

func (s *Server) nutritionReport(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	from, err := parseDate(r.URL.Query().Get("from"))
	if err != nil {
		return err
	}
	to, err := parseDate(r.URL.Query().Get("to"))
	if err != nil {
		return err
	}
	if to.Before(from) || to.Sub(from) > 62*24*time.Hour {
		return errBadRequest("Range must be at most two months")
	}
	rep, err := s.buildReport(ctx, u.HouseholdID, from, to)
	if err != nil {
		return err
	}
	return ok(w, rep)
}

func (s *Server) buildReport(ctx context.Context, householdID string, from, to time.Time) ([]personReport, error) {
	hh, err := s.loadHouseholdHealth(ctx, householdID)
	if err != nil {
		return nil, err
	}
	recipes, err := s.queryRecipes(ctx, householdID, "")
	if err != nil {
		return nil, err
	}
	byID := map[string]*Recipe{}
	for i := range recipes {
		byID[recipes[i].ID] = &recipes[i]
	}
	rows, _ := s.db.Query(ctx, `SELECT `+mealColumns+` FROM meal_plan_items WHERE household_id = $1 AND date BETWEEN $2 AND $3`, householdID, from, to)
	items, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[MealItem])
	if err != nil {
		return nil, err
	}
	for i := range items {
		m := &items[i]
		switch {
		case m.Portions != nil:
			m.Eaters = m.Portions
		case m.RecipeID != nil && byID[*m.RecipeID] != nil:
			m.Eaters = hh.defaultEaters(byID[*m.RecipeID].Nutrition.Contains)
		default:
			m.Eaters = hh.defaultEaters(nil)
		}
	}
	now := time.Now()
	out := []personReport{}
	for _, mem := range hh.members {
		if mem.Role == "viewer" {
			continue
		}
		pr := personReport{UserID: mem.ID, Name: mem.DisplayName, Color: mem.Color, Days: []dayIntake{}, Gaps: []nutrientGap{}}
		if p := hh.profiles[mem.ID]; p != nil {
			_, t := p.targets(now)
			pr.Targets = &t
		}
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			pr.Days = append(pr.Days, dayIntake{Date: d.Format(time.DateOnly), Unknown: []string{}})
		}
		days := map[string]*dayIntake{}
		for i := range pr.Days {
			days[pr.Days[i].Date] = &pr.Days[i]
		}
		for _, m := range items {
			portion := m.Eaters[mem.ID]
			day := days[m.Date]
			if portion <= 0 || day == nil {
				continue
			}
			var rc *Recipe
			if m.RecipeID != nil {
				rc = byID[*m.RecipeID]
			}
			if rc == nil || rc.Nutrition.Coverage < 0.5 {
				day.Unknown = append(day.Unknown, m.Title)
				continue
			}
			day.Dishes++
			day.Totals.Add(rc.Nutrition.PerServing, portion)
		}
		var sum nutrition.Nutrients
		for i := range pr.Days {
			d := &pr.Days[i]
			d.Totals = d.Totals.Round()
			switch {
			case len(d.Unknown) > 0:
				pr.Partial++
			case d.Dishes > 0:
				pr.Counted++
				sum.Add(d.Totals, 1)
			}
		}
		if pr.Counted > 0 {
			pr.Average = sum.Scale(1 / float64(pr.Counted)).Round()
			if pr.Targets != nil {
				pr.Gaps = findGaps(pr.Average, pr.Targets.Nutrients, recipes, hh, mem.ID)
			}
		}
		out = append(out, pr)
	}
	return out, nil
}

// findGaps lists nutrients averaging under 80% of target (or over a ceiling), each with
// the household's recipes that supply the most of it per calorie.
func findGaps(avg, target nutrition.Nutrients, recipes []Recipe, hh *householdHealth, userID string) []nutrientGap {
	gaps := []nutrientGap{}
	for _, k := range gapNutrients {
		t := target.Get(k)
		if t <= 0 {
			continue
		}
		pct := avg.Get(k) / t * 100
		if pct >= 80 {
			continue
		}
		gaps = append(gaps, nutrientGap{Nutrient: k, Kind: "low", Percent: int(math.Round(pct)), Ideas: ideasFor(k, t, target.Kcal, recipes, hh, userID)})
	}
	for _, k := range []string{"sodium", "fat"} {
		if t := target.Get(k); t > 0 && avg.Get(k) > t*1.1 {
			gaps = append(gaps, nutrientGap{Nutrient: k, Kind: "high", Percent: int(math.Round(avg.Get(k) / t * 100)), Ideas: []nutrientIdea{}})
		}
	}
	if t := target.Kcal; t > 0 && avg.Kcal > t*1.1 {
		gaps = append(gaps, nutrientGap{Nutrient: "kcal", Kind: "high", Percent: int(math.Round(avg.Kcal / t * 100)), Ideas: []nutrientIdea{}})
	}
	return gaps
}

func ideasFor(key string, target, kcalTarget float64, recipes []Recipe, hh *householdHealth, userID string) []nutrientIdea {
	type scored struct {
		idea  nutrientIdea
		score float64
	}
	var cands []scored
	for _, rc := range recipes {
		n := rc.Nutrition
		amt := n.PerServing.Get(key)
		if n.Coverage < 0.6 || amt <= 0 || n.PerServing.Kcal <= 0 || hh.avoids(userID, n.Contains) {
			continue
		}
		share := amt / target
		if share < 0.1 {
			continue
		}
		// Nutrient density: share of the day's need per share of the day's calories.
		density := share / (n.PerServing.Kcal / math.Max(kcalTarget, 1))
		cands = append(cands, scored{nutrientIdea{rc.ID, rc.Title, math.Round(amt*10) / 10, int(math.Round(share * 100))}, density + share})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	out := []nutrientIdea{}
	for i := 0; i < len(cands) && i < 4; i++ {
		out = append(out, cands[i].idea)
	}
	return out
}
