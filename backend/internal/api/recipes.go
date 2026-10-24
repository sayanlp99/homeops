package api

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"homeops/internal/nutrition"
)

type RecipeIngredient struct {
	IngredientID string   `json:"ingredientId"`
	Name         string   `json:"name"`
	Aisle        string   `json:"aisle"`
	Quantity     *float64 `json:"quantity"`
	Unit         string   `json:"unit"`
	Note         string   `json:"note"`
	FoodKey      string   `json:"foodKey"` // chosen food-table entry: "" = by name, "-" = left out
	Food         string   `json:"food"`    // the entry used for nutrition, "" if none
}

type Recipe struct {
	ID            string             `json:"id" db:"id"`
	Title         string             `json:"title" db:"title"`
	Description   string             `json:"description" db:"description"`
	Category      string             `json:"category" db:"category"`
	MealTypes     []string           `json:"mealTypes" db:"meal_types"`
	Diet          string             `json:"diet" db:"diet"`
	Cuisine       string             `json:"cuisine" db:"cuisine"`
	Tags          []string           `json:"tags" db:"tags"`
	Servings      int                `json:"servings" db:"servings"`
	PrepMinutes   *int               `json:"prepMinutes" db:"prep_minutes"`
	CookMinutes   *int               `json:"cookMinutes" db:"cook_minutes"`
	PrepNote      string             `json:"prepNote" db:"prep_note"`
	PrepLeadHours int                `json:"prepLeadHours" db:"prep_lead_hours"`
	Steps         []string           `json:"steps" db:"steps"`
	SourceURL     string             `json:"sourceUrl" db:"source_url"`
	Photo         string             `json:"photo" db:"photo"`
	Favorite      bool               `json:"favorite" db:"favorite"`
	Rating        int                `json:"rating" db:"rating"`
	CreatedAt     time.Time          `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time          `json:"updatedAt" db:"updated_at"`
	TimesCooked   int                `json:"timesCooked" db:"times_cooked"`
	LastCooked    *string            `json:"lastCooked" db:"last_cooked"`
	Months        []int              `json:"months" db:"months"`
	Ingredients   []RecipeIngredient `json:"ingredients" db:"-"`
	Nutrition     nutrition.Result   `json:"nutrition" db:"-"`
}

var (
	recipeCategories = []string{"main", "curry", "dal", "sabzi", "rice", "bread", "side", "salad", "snack", "breakfast", "dessert", "drink", "soup", "chutney"}
	mealSlots        = []string{"breakfast", "lunch", "snacks", "dinner"}
	diets            = []string{"veg", "egg", "nonveg", "vegan"}
)

// "Cooked" means marked cooked, or planned on a day that has passed.
const recipeSelect = `SELECT r.id, r.title, r.description, r.category, r.meal_types, r.diet, r.cuisine, r.tags, r.servings,
	r.prep_minutes, r.cook_minutes, r.prep_note, r.prep_lead_hours, r.steps, r.source_url, r.photo, r.favorite, r.rating,
	r.created_at, r.updated_at, r.months::int[] AS months,
	(SELECT count(*)::int FROM meal_plan_items m WHERE m.recipe_id = r.id AND (m.cooked_at IS NOT NULL OR m.date < current_date)) AS times_cooked,
	(SELECT max(m.date)::text FROM meal_plan_items m WHERE m.recipe_id = r.id AND (m.cooked_at IS NOT NULL OR m.date < current_date)) AS last_cooked
	FROM recipes r`

func (s *Server) queryRecipes(ctx context.Context, householdID string, where string, args ...any) ([]Recipe, error) {
	rows, _ := s.db.Query(ctx, recipeSelect+` WHERE r.household_id = $1 `+where+` ORDER BY lower(r.title)`, append([]any{householdID}, args...)...)
	recipes, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[Recipe])
	if err != nil || len(recipes) == 0 {
		return recipes, err
	}
	ids := make([]string, len(recipes))
	byID := make(map[string]*Recipe, len(recipes))
	for i := range recipes {
		ids[i] = recipes[i].ID
		recipes[i].Ingredients = []RecipeIngredient{}
		byID[recipes[i].ID] = &recipes[i]
	}
	rows, err = s.db.Query(ctx, `SELECT ri.recipe_id, i.id, i.name, i.aisle, ri.quantity, ri.unit, ri.note, i.food_key
		FROM recipe_ingredients ri JOIN ingredients i ON i.id = ri.ingredient_id
		WHERE ri.recipe_id = ANY($1::uuid[]) ORDER BY ri.position`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rid string
		var ing RecipeIngredient
		if err := rows.Scan(&rid, &ing.IngredientID, &ing.Name, &ing.Aisle, &ing.Quantity, &ing.Unit, &ing.Note, &ing.FoodKey); err != nil {
			return nil, err
		}
		byID[rid].Ingredients = append(byID[rid].Ingredients, ing)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range recipes {
		recipes[i].computeNutrition()
	}
	return recipes, nil
}

// computeNutrition fills in per-serving nutrition and the food groups the dish contains.
func (rc *Recipe) computeNutrition() {
	lines := make([]nutrition.Line, len(rc.Ingredients))
	for i, ing := range rc.Ingredients {
		lines[i] = nutrition.Line{Name: ing.Name, FoodKey: ing.FoodKey, Quantity: ing.Quantity, Unit: ing.Unit}
		if f := lines[i].Resolve(); f != nil {
			rc.Ingredients[i].Food = f.Key
		}
	}
	rc.Nutrition = nutrition.Calc(lines, rc.Servings)
}

func (s *Server) listRecipes(w http.ResponseWriter, r *http.Request) error {
	recipes, err := s.queryRecipes(r.Context(), currentUser(r).HouseholdID, "")
	if err != nil {
		return err
	}
	return ok(w, recipes)
}

func (s *Server) getRecipe(w http.ResponseWriter, r *http.Request) error {
	recipes, err := s.queryRecipes(r.Context(), currentUser(r).HouseholdID, "AND r.id = $2", chi.URLParam(r, "id"))
	if err != nil {
		return err
	}
	if len(recipes) == 0 {
		return errNotFound("Recipe not found")
	}
	return ok(w, recipes[0])
}

func (in *Recipe) validate() error {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > 150 {
		return errBadRequest("Recipe name is required (max 150 characters)")
	}
	if !slices.Contains(recipeCategories, in.Category) {
		in.Category = "main"
	}
	if !slices.Contains(diets, in.Diet) {
		return errBadRequest("Diet must be veg, egg, nonveg or vegan")
	}
	types := []string{}
	for _, t := range in.MealTypes {
		if slices.Contains(mealSlots, t) && !slices.Contains(types, t) {
			types = append(types, t)
		}
	}
	in.MealTypes = types
	if in.Servings < 1 || in.Servings > 50 {
		in.Servings = 2
	}
	if in.Rating < 0 || in.Rating > 5 {
		in.Rating = 0
	}
	if in.PrepLeadHours < 0 || in.PrepLeadHours > 72 {
		return errBadRequest("Prep reminder must be 0–72 hours ahead")
	}
	if in.Photo != "" && !fileIDRe.MatchString(in.Photo) {
		return errBadRequest("Invalid photo")
	}
	steps := []string{}
	for _, st := range in.Steps {
		if st = strings.TrimSpace(st); st != "" {
			steps = append(steps, st)
		}
	}
	in.Steps = steps
	tags := []string{}
	for _, t := range in.Tags {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" && !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	in.Tags = tags
	in.Cuisine = strings.TrimSpace(in.Cuisine)
	months := []int{}
	for _, m := range in.Months {
		if m >= 1 && m <= 12 && !slices.Contains(months, m) {
			months = append(months, m)
		}
	}
	slices.Sort(months)
	if len(months) == 12 {
		months = []int{}
	}
	in.Months = months
	if len(in.Ingredients) > 80 || len(in.Steps) > 60 {
		return errBadRequest("Too many ingredients or steps")
	}
	return nil
}

// ensureIngredient finds or creates a catalog ingredient by name (case-insensitive).
func ensureIngredient(ctx context.Context, q querier, householdID, name string) (id, aisle string, err error) {
	name = cleanName(name)
	if name == "" {
		return "", "", errBadRequest("Ingredient name is required")
	}
	// Singular and plural are the same thing on a shopping list: "Onion" = "Onions".
	l := strings.ToLower(name)
	variants := []string{l, l + "s", l + "es", strings.TrimSuffix(l, "s"), strings.TrimSuffix(l, "es")}
	err = q.QueryRow(ctx, `SELECT id, aisle FROM ingredients WHERE household_id = $1 AND lower(name) = ANY($2)
		ORDER BY lower(name) = $3 DESC, created_at LIMIT 1`, householdID, variants, l).Scan(&id, &aisle)
	if err == nil {
		return id, aisle, nil
	}
	err = q.QueryRow(ctx, `INSERT INTO ingredients (household_id, name, aisle) VALUES ($1, $2, $3)
		ON CONFLICT (household_id, lower(name)) DO UPDATE SET name = ingredients.name
		RETURNING id, aisle`, householdID, truncate(name, 80), classifyAisle(name)).Scan(&id, &aisle)
	return id, aisle, err
}

func (s *Server) saveRecipe(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in Recipe
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	id, err := writeRecipe(ctx, tx, u, chi.URLParam(r, "id"), &in)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventRecipes.from(r))
	s.hub.Publish(u.HouseholdID, eventMeals.from(r))
	recipes, err := s.queryRecipes(ctx, u.HouseholdID, "AND r.id = $2", id)
	if err != nil {
		return err
	}
	return ok(w, recipes[0])
}

// writeRecipe inserts (id "") or replaces a validated recipe and its ingredients.
func writeRecipe(ctx context.Context, tx pgx.Tx, u *User, id string, in *Recipe) (string, error) {
	steps, _ := json.Marshal(in.Steps)
	var err error
	if id == "" {
		err = tx.QueryRow(ctx, `INSERT INTO recipes (household_id, title, description, category, meal_types, diet, cuisine, tags,
			servings, prep_minutes, cook_minutes, prep_note, prep_lead_hours, steps, source_url, photo, favorite, rating, created_by, months)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20) RETURNING id`,
			u.HouseholdID, in.Title, in.Description, in.Category, in.MealTypes, in.Diet, in.Cuisine, in.Tags, in.Servings,
			in.PrepMinutes, in.CookMinutes, in.PrepNote, in.PrepLeadHours, steps, in.SourceURL, in.Photo, in.Favorite, in.Rating, u.ID, in.Months).Scan(&id)
	} else {
		var tag interface{ RowsAffected() int64 }
		tag, err = tx.Exec(ctx, `UPDATE recipes SET title = $3, description = $4, category = $5, meal_types = $6, diet = $7,
			cuisine = $8, tags = $9, servings = $10, prep_minutes = $11, cook_minutes = $12, prep_note = $13, prep_lead_hours = $14,
			steps = $15, source_url = $16, photo = $17, favorite = $18, rating = $19, months = $20, updated_at = now()
			WHERE id = $1 AND household_id = $2`,
			id, u.HouseholdID, in.Title, in.Description, in.Category, in.MealTypes, in.Diet, in.Cuisine, in.Tags, in.Servings,
			in.PrepMinutes, in.CookMinutes, in.PrepNote, in.PrepLeadHours, steps, in.SourceURL, in.Photo, in.Favorite, in.Rating, in.Months)
		if err == nil && tag.RowsAffected() == 0 {
			return "", errNotFound("Recipe not found")
		}
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM recipe_ingredients WHERE recipe_id = $1`, id)
		}
	}
	if err != nil {
		return "", err
	}
	for i, ing := range in.Ingredients {
		if strings.TrimSpace(ing.Name) == "" {
			continue
		}
		iid, _, err := ensureIngredient(ctx, tx, u.HouseholdID, ing.Name)
		if err != nil {
			return "", err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO recipe_ingredients (recipe_id, position, ingredient_id, quantity, unit, note)
			VALUES ($1, $2, $3, $4, $5, $6)`, id, i, iid, ing.Quantity, normalizeUnit(ing.Unit), truncate(strings.TrimSpace(ing.Note), 200)); err != nil {
			return "", err
		}
	}
	// Keep plan entries' titles in sync with a renamed recipe.
	if _, err := tx.Exec(ctx, `UPDATE meal_plan_items SET title = $2 WHERE recipe_id = $1 AND title <> $2`, id, in.Title); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Server) patchRecipe(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in struct {
		Favorite *bool `json:"favorite"`
		Rating   *int  `json:"rating"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Rating != nil && (*in.Rating < 0 || *in.Rating > 5) {
		return errBadRequest("Rating must be 0–5")
	}
	tag, err := s.db.Exec(r.Context(), `UPDATE recipes SET favorite = coalesce($3, favorite), rating = coalesce($4, rating)
		WHERE id = $1 AND household_id = $2`, chi.URLParam(r, "id"), u.HouseholdID, in.Favorite, in.Rating)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNotFound("Recipe not found")
	}
	s.hub.Publish(u.HouseholdID, eventRecipes.from(r))
	return noContent(w)
}

func (s *Server) deleteRecipe(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	// Past plan entries keep their title as free text; history stays intact.
	if _, err := s.db.Exec(r.Context(), `DELETE FROM recipes WHERE id = $1 AND household_id = $2`, chi.URLParam(r, "id"), u.HouseholdID); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventRecipes.from(r))
	s.hub.Publish(u.HouseholdID, eventMeals.from(r))
	return noContent(w)
}

// ---- ingredients ----

type Ingredient struct {
	ID      string `json:"id" db:"id"`
	Name    string `json:"name" db:"name"`
	Aisle   string `json:"aisle" db:"aisle"`
	FoodKey string `json:"foodKey" db:"food_key"`
}

func (s *Server) listIngredients(w http.ResponseWriter, r *http.Request) error {
	rows, _ := s.db.Query(r.Context(), `SELECT id, name, aisle, food_key FROM ingredients WHERE household_id = $1 ORDER BY lower(name)`, currentUser(r).HouseholdID)
	out, err := pgx.CollectRows(rows, pgx.RowToStructByName[Ingredient])
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"ingredients": out, "aisles": aisles})
}

func (s *Server) updateIngredient(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in struct {
		Aisle   *string `json:"aisle"`
		FoodKey *string `json:"foodKey"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	id := chi.URLParam(r, "id")
	if in.FoodKey != nil {
		if k := *in.FoodKey; k != "" && k != "-" && nutrition.ByKey(k) == nil {
			return errBadRequest("Unknown food")
		}
		if _, err := s.db.Exec(r.Context(), `UPDATE ingredients SET food_key = $3 WHERE id = $1 AND household_id = $2`, id, u.HouseholdID, *in.FoodKey); err != nil {
			return err
		}
		s.hub.Publish(u.HouseholdID, eventRecipes.from(r))
		if in.Aisle == nil {
			return noContent(w)
		}
	}
	if in.Aisle == nil || !isAisle(*in.Aisle) {
		return errBadRequest("Unknown aisle")
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE ingredients SET aisle = $3 WHERE id = $1 AND household_id = $2`, id, u.HouseholdID, *in.Aisle); err != nil {
		return err
	}
	// Move it on the current shopping list too.
	if _, err := s.db.Exec(r.Context(), `UPDATE grocery_items SET aisle = $3 WHERE ingredient_id = $1 AND household_id = $2`, id, u.HouseholdID, *in.Aisle); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventGrocery.from(r))
	s.hub.Publish(u.HouseholdID, eventRecipes.from(r))
	return noContent(w)
}

// parseIngredients turns pasted text (one ingredient per line) into structured rows.
func (s *Server) parseIngredients(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Lines []string `json:"lines"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	out := []map[string]any{}
	for _, l := range in.Lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		p := parseIngredientLine(l)
		out = append(out, map[string]any{"quantity": p.Quantity, "unit": p.Unit, "name": p.Name, "note": p.Note, "aisle": classifyAisle(p.Name)})
	}
	return ok(w, out)
}
