package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"homeops/internal/gemini"
	"homeops/internal/nutrition"
)

// AI helpers draft things for people to check: a recipe from a dish name, grocery items
// from a messy list, a task from a sentence. Nothing is saved here. On Google's free tier
// prompts may be read by Google, so only kitchen and chore text is ever sent — never money data.

func (s *Server) askAI(r *http.Request, system, prompt string, schema map[string]any, out any) error {
	if s.ai == nil {
		return &apiError{http.StatusNotImplemented, "ai_off", "AI helpers aren't set up on this server"}
	}
	key := currentUser(r).HouseholdID
	if s.aiLimit.Blocked(key) {
		return &apiError{http.StatusTooManyRequests, "ai_limit", "Your household has used today's AI requests. Try again tomorrow."}
	}
	s.aiLimit.Fail(key)
	start := time.Now()
	err := s.ai.JSON(r.Context(), system, prompt, schema, out)
	switch {
	case err == nil:
		slog.Info("ai request", "path", r.URL.Path, "took", time.Since(start).Round(time.Millisecond))
		return nil
	case errors.Is(err, gemini.ErrQuota):
		slog.Warn("ai quota exceeded", "path", r.URL.Path)
		return &apiError{http.StatusTooManyRequests, "ai_limit", "The free AI limit is used up for now. Try again in a few minutes."}
	default:
		slog.Warn("ai request failed", "path", r.URL.Path, "err", err)
		return &apiError{http.StatusBadGateway, "ai_failed", "The AI didn't answer. Try again."}
	}
}

func aiText(s string, max int, what string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errBadRequest(what + " is required")
	}
	if len(s) > max {
		return "", errBadRequest(fmt.Sprintf("%s is too long (max %d characters)", what, max))
	}
	return s, nil
}

// Schema helpers keep the shapes below readable.
func obj(props map[string]any) map[string]any {
	req := make([]string, 0, len(props))
	for k := range props {
		req = append(req, k)
	}
	slices.Sort(req)
	return map[string]any{"type": "object", "properties": props, "required": req}
}
func arr(item map[string]any) map[string]any { return map[string]any{"type": "array", "items": item} }
func enum(vals ...string) map[string]any     { return map[string]any{"type": "string", "enum": vals} }

var (
	jsonStr        = map[string]any{"type": "string"}
	jsonInt        = map[string]any{"type": "integer"}
	jsonNum        = map[string]any{"type": []string{"number", "null"}}
	ingredientJSON = obj(map[string]any{"name": jsonStr, "quantity": jsonNum, "unit": jsonStr, "note": jsonStr})
)

const unitsHint = "Units: g, kg, ml, l, tsp, tbsp, cup, pcs, clove, bunch, packet, pinch, inch, sprig, dozen, bottle, handful, leaf, pod — or empty when there is no amount."

// foodNames lists the foods HomeOps has nutrition data for, so drafted ingredient
// names match them and the recipe gets nutrition without fixing names by hand.
var foodNames = func() string {
	names := []string{}
	for _, f := range nutrition.Foods() {
		names = append(names, f.Name)
	}
	return strings.Join(names, "; ")
}()

type aiRecipeOut struct {
	Known         bool               `json:"known"`
	Title         string             `json:"title"`
	Description   string             `json:"description"`
	Category      string             `json:"category"`
	MealTypes     []string           `json:"mealTypes"`
	Diet          string             `json:"diet"`
	Cuisine       string             `json:"cuisine"`
	Tags          []string           `json:"tags"`
	Servings      int                `json:"servings"`
	PrepMinutes   int                `json:"prepMinutes"`
	CookMinutes   int                `json:"cookMinutes"`
	PrepNote      string             `json:"prepNote"`
	PrepLeadHours int                `json:"prepLeadHours"`
	Ingredients   []RecipeIngredient `json:"ingredients"`
	Steps         []string           `json:"steps"`
}

var aiRecipeSchema = obj(map[string]any{
	"known":         map[string]any{"type": "boolean"},
	"title":         jsonStr,
	"description":   jsonStr,
	"category":      enum(recipeCategories...),
	"mealTypes":     arr(enum(mealSlots...)),
	"diet":          enum(diets...),
	"cuisine":       jsonStr,
	"tags":          arr(jsonStr),
	"servings":      jsonInt,
	"prepMinutes":   jsonInt,
	"cookMinutes":   jsonInt,
	"prepNote":      jsonStr,
	"prepLeadHours": jsonInt,
	"ingredients":   arr(ingredientJSON),
	"steps":         arr(jsonStr),
})

// aiRecipe drafts a full recipe from a dish name. It goes to the editor like an import.
func (s *Server) aiRecipe(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Name     string `json:"name"`
		Servings int    `json:"servings"`
		Notes    string `json:"notes"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	name, err := aiText(in.Name, 120, "Dish name")
	if err != nil {
		return err
	}
	notes, _ := aiText(in.Notes, 500, "Notes")
	if in.Servings < 1 || in.Servings > 20 {
		in.Servings = 2
	}
	system := `You write recipes for an Indian home kitchen, the way a good home cook makes the dish — not restaurant style.
Use the dish's own regional style (Bengali dishes cooked the Bengali way, and so on).
Quantities must be realistic for the servings asked, in metric or spoon/cup measures. ` + unitsHint + `
Ingredient names are plain English singular nouns without amounts or prep ("potato", "mustard oil", "green chilli"); put prep like "finely chopped" in note.
List every spice and whole spice separately with its own amount (turmeric powder, cumin seeds, bay leaf…); never group them.
When an ingredient is one of these foods, use exactly this name: ` + foodNames + `
title is the dish's usual name in Title Case.
Steps are short, one action each, in order, without numbering. The description is one plain sentence.
prepNote is advance work needed before cooking day (soaking, marinating, fermenting), else empty; prepLeadHours is how many hours ahead it must start, else 0.
mealTypes are the meals it suits. tags are 0-4 lowercase words (e.g. "quick", "festive").
If the name isn't a real dish you know, set known to false and leave the rest empty.`
	prompt := fmt.Sprintf("Dish: %s\nServings: %d", name, in.Servings)
	if notes != "" {
		prompt += "\nThe cook's notes: " + notes
	}
	var got aiRecipeOut
	if err := s.askAI(r, system, prompt, aiRecipeSchema, &got); err != nil {
		return err
	}
	if !got.Known || strings.TrimSpace(got.Title) == "" {
		return errBadRequest("The AI didn't recognise that dish. Try its usual name, or add a few notes.")
	}
	out := importedRecipe{Recipe: Recipe{
		Title: got.Title, Description: got.Description, Category: got.Category, MealTypes: got.MealTypes, Diet: got.Diet,
		Cuisine: got.Cuisine, Tags: got.Tags, Servings: got.Servings, PrepNote: got.PrepNote, PrepLeadHours: got.PrepLeadHours,
		Steps: got.Steps, Months: []int{}, Ingredients: []RecipeIngredient{},
	}}
	if got.PrepMinutes > 0 {
		out.PrepMinutes = &got.PrepMinutes
	}
	if got.CookMinutes > 0 {
		out.CookMinutes = &got.CookMinutes
	}
	for _, ing := range got.Ingredients {
		if n := cleanName(ing.Name); n != "" {
			out.Ingredients = append(out.Ingredients, RecipeIngredient{Name: n, Quantity: positive(ing.Quantity), Unit: normalizeUnit(ing.Unit),
				Note: truncate(strings.TrimSpace(ing.Note), 200), Aisle: classifyAisle(n)})
		}
	}
	if out.PrepLeadHours < 0 || out.PrepLeadHours > 72 {
		out.PrepLeadHours = 0
	}
	if err := out.validate(); err != nil {
		slog.Warn("ai recipe invalid", "err", err)
		return &apiError{http.StatusBadGateway, "ai_failed", "The AI's recipe didn't make sense. Try again."}
	}
	out.computeNutrition()
	out.Warnings = []string{"Drafted by AI: check the quantities and steps before saving."}
	return ok(w, out)
}

func positive(q *float64) *float64 {
	if q == nil || *q <= 0 {
		return nil
	}
	return q
}

type aiGroceryItem struct {
	Name     string   `json:"name"`
	Quantity *float64 `json:"quantity"`
	Unit     string   `json:"unit"`
	Note     string   `json:"note"`
	Aisle    string   `json:"aisle"`
}

var aiGrocerySchema = obj(map[string]any{"items": arr(ingredientJSON)})

// aiGrocery turns a pasted or typed list ("2 kg aloo, dhania, 1 dozen eggs") into items.
func (s *Server) aiGrocery(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in struct {
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	text, err := aiText(in.Text, 3000, "The list")
	if err != nil {
		return err
	}
	// The household's own names, so "aloo" lands on the "Potato" they already buy.
	rows, err := s.db.Query(r.Context(), `SELECT name, aisle FROM ingredients WHERE household_id = $1 ORDER BY name LIMIT 600`, u.HouseholdID)
	if err != nil {
		return err
	}
	aisles := map[string]string{}
	known := []string{}
	for rows.Next() {
		var n, a string
		if err := rows.Scan(&n, &a); err != nil {
			return err
		}
		aisles[strings.ToLower(n)] = a
		known = append(known, n)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	system := `You turn a household's shopping list into separate grocery items. The list may be in English, Hindi, Bengali or a mix, written any way (commas, lines, voice-typed sentences).
One item per thing to buy. Keep the amount the person asked for; leave quantity null and unit empty when they gave none. Don't add items they didn't ask for.
` + unitsHint + `
Name items in plain English singular ("potato", "coriander leaves", "egg"). When an item is one of the household's usual items below, use exactly that name.
note holds anything else they said about the item (brand, size, "ripe", "for Sunday"), else empty.
Usual items: ` + strings.Join(known, "; ")
	var got struct {
		Items []aiGroceryItem `json:"items"`
	}
	if err := s.askAI(r, system, text, aiGrocerySchema, &got); err != nil {
		return err
	}
	out := []aiGroceryItem{}
	for _, it := range got.Items {
		n := cleanName(it.Name)
		if n == "" || len(out) >= 100 {
			continue
		}
		aisle, ok := aisles[strings.ToLower(n)]
		if !ok {
			aisle = classifyAisle(n)
		}
		out = append(out, aiGroceryItem{Name: n, Quantity: positive(it.Quantity), Unit: normalizeUnit(it.Unit), Note: truncate(strings.TrimSpace(it.Note), 200), Aisle: aisle})
	}
	if len(out) == 0 {
		return errBadRequest("No grocery items found in that text")
	}
	return ok(w, out)
}

type aiTaskOut struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	AssigneeID  *string  `json:"assigneeId"`
	DueDate     *string  `json:"dueDate"`
	Priority    int      `json:"priority"`
	Labels      []string `json:"labels"`
	Repeat      *struct {
		Freq     string `json:"freq"`
		Interval int    `json:"interval"`
		Weekdays []int  `json:"weekdays"`
		MonthDay *int   `json:"monthDay"`
	} `json:"repeat"`
}

// aiTask reads a sentence ("remind Sangita to pay the maid every 5th") into a task, or a
// recurring chore when it repeats. The app fills its New task form with the result.
func (s *Server) aiTask(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	text, err := aiText(in.Text, 1000, "The sentence")
	if err != nil {
		return err
	}
	h, err := s.getHousehold(ctx, u.HouseholdID)
	if err != nil {
		return err
	}
	var members []struct{ ID, Name string }
	rows, _ := s.db.Query(ctx, `SELECT id, display_name FROM members WHERE household_id = $1 AND role <> 'viewer' ORDER BY created_at`, u.HouseholdID)
	for rows.Next() {
		var m struct{ ID, Name string }
		if err := rows.Scan(&m.ID, &m.Name); err != nil {
			return err
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	labels, err := s.labels(ctx, u.HouseholdID)
	if err != nil {
		return err
	}
	memberIDs, labelIDs := []string{}, []string{}
	var people, tags strings.Builder
	me := ""
	for _, m := range members {
		memberIDs = append(memberIDs, m.ID)
		fmt.Fprintf(&people, "- %s: id %s\n", m.Name, m.ID)
		if m.ID == u.ID {
			me = m.Name
		}
	}
	for _, l := range labels {
		labelIDs = append(labelIDs, l.ID)
		fmt.Fprintf(&tags, "- %s: id %s\n", l.Name, l.ID)
	}
	if len(labelIDs) == 0 {
		labelIDs = []string{"none"}
	}
	today := localToday(h, time.Now())
	schema := obj(map[string]any{
		"title":       jsonStr,
		"description": jsonStr,
		"assigneeId":  map[string]any{"type": []string{"string", "null"}, "enum": append(slices.Clone(memberIDs), "")},
		"dueDate":     map[string]any{"type": []string{"string", "null"}, "description": "YYYY-MM-DD"},
		"priority":    map[string]any{"type": "integer", "minimum": 0, "maximum": 4},
		"labels":      arr(enum(labelIDs...)),
		"repeat": map[string]any{"type": []string{"object", "null"}, "properties": map[string]any{
			"freq": enum("daily", "weekly", "monthly"), "interval": jsonInt, "weekdays": arr(map[string]any{"type": "integer", "minimum": 0, "maximum": 6}),
			"monthDay": map[string]any{"type": []string{"integer", "null"}},
		}, "required": []string{"freq", "interval", "weekdays", "monthDay"}},
	})
	system := fmt.Sprintf(`You turn one sentence from a household member into a chore for their shared task list.
Today is %s (%s). The person writing is %s; "me", "I" and "my" mean them.
title: short imperative, capitalised, without the person, date or schedule ("Pay the maid", "Clean the fridge").
description: extra details from the sentence, else empty.
assigneeId: the member named or clearly meant, using the ids below; the writer when they say "me" or name no one; empty only when they say anyone/whoever.
dueDate: the day it's due if a one-off date or day is given ("tomorrow", "Friday" = the next Friday, "by the 10th"), else null.
priority: 0 none, 1 low, 2 medium (default), 3 high, 4 urgent ("urgent", "asap", "today").
labels: ids of labels that clearly fit, else none.
repeat: null for a one-off task. For a repeating chore, freq daily/weekly/monthly with interval (2 = every other); weekdays 0=Sunday…6=Saturday for weekly (the day given, else today's); monthDay 1-31 for monthly. When it repeats, dueDate is null.
Members:
%sLabels:
%s`, today.Format(time.DateOnly), today.Weekday(), me, people.String(), tags.String())
	var got aiTaskOut
	if err := s.askAI(r, system, text, schema, &got); err != nil {
		return err
	}
	got.Title = truncate(strings.TrimSpace(got.Title), 200)
	if got.Title == "" {
		return errBadRequest("Couldn't find a task in that sentence")
	}
	got.Description = truncate(strings.TrimSpace(got.Description), 2000)
	if got.AssigneeID != nil && !slices.Contains(memberIDs, *got.AssigneeID) {
		got.AssigneeID = nil
	}
	if got.DueDate != nil {
		if _, err := time.Parse(time.DateOnly, *got.DueDate); err != nil {
			got.DueDate = nil
		}
	}
	got.Priority = min(max(got.Priority, 0), 4)
	kept := []string{}
	for _, l := range got.Labels {
		if slices.Contains(labelIDs, l) && l != "none" && !slices.Contains(kept, l) {
			kept = append(kept, l)
		}
	}
	got.Labels = kept
	if rp := got.Repeat; rp != nil {
		rp.Interval = min(max(rp.Interval, 1), 52)
		days := []int{}
		for _, d := range rp.Weekdays {
			if d >= 0 && d <= 6 && !slices.Contains(days, d) {
				days = append(days, d)
			}
		}
		slices.Sort(days)
		rp.Weekdays = days
		switch rp.Freq {
		case "weekly":
			if len(rp.Weekdays) == 0 {
				rp.Weekdays = []int{int(today.Weekday())}
			}
			rp.MonthDay = nil
		case "monthly":
			if rp.MonthDay == nil || *rp.MonthDay < 1 || *rp.MonthDay > 31 {
				d := today.Day()
				rp.MonthDay = &d
			}
			rp.Weekdays = []int{}
		case "daily":
			rp.Weekdays, rp.MonthDay = []int{}, nil
		default:
			got.Repeat = nil
		}
		if got.Repeat != nil {
			got.DueDate = nil
		}
	}
	return ok(w, got)
}
