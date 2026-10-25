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
)

type MealItem struct {
	ID        string     `json:"id" db:"id"`
	Date      string     `json:"date" db:"date"`
	Slot      string     `json:"slot" db:"slot"`
	Position  float64    `json:"position" db:"position"`
	RecipeID  *string    `json:"recipeId" db:"recipe_id"`
	Title     string     `json:"title" db:"title"`
	Servings  int        `json:"servings" db:"servings"`
	Note      string     `json:"note" db:"note"`
	CookedAt  *time.Time `json:"cookedAt" db:"cooked_at"`
	CookedBy  *string    `json:"cookedBy" db:"cooked_by"`
	CreatedAt time.Time  `json:"createdAt" db:"created_at"`
	// Portions is who eats how many servings, as set by hand; nil = the default.
	Portions map[string]float64 `json:"portions" db:"portions"`
	Leftover bool               `json:"leftover" db:"leftover"`
	// Eaters is Portions, or the default: one serving for each member who eats this food.
	Eaters map[string]float64 `json:"eaters" db:"-"`
}

const mealColumns = `id, date::text AS date, slot, position, recipe_id, title, servings, note, cooked_at, cooked_by, created_at, portions, leftover`

func parseDate(s string) (time.Time, error) {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return d, errBadRequest("Dates must be YYYY-MM-DD")
	}
	return d, nil
}

func (s *Server) listMeals(w http.ResponseWriter, r *http.Request) error {
	from, err := parseDate(r.URL.Query().Get("from"))
	if err != nil {
		return err
	}
	to, err := parseDate(r.URL.Query().Get("to"))
	if err != nil {
		return err
	}
	if to.Sub(from) > 62*24*time.Hour || to.Before(from) {
		return errBadRequest("Range must be at most two months")
	}
	rows, _ := s.db.Query(r.Context(), `SELECT `+mealColumns+` FROM meal_plan_items
		WHERE household_id = $1 AND date BETWEEN $2 AND $3
		ORDER BY date, array_position(ARRAY['breakfast','lunch','snacks','dinner'], slot), position, created_at`, currentUser(r).HouseholdID, from, to)
	items, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[MealItem])
	if err != nil {
		return err
	}
	if err := s.resolveEaters(r.Context(), currentUser(r).HouseholdID, items); err != nil {
		return err
	}
	return ok(w, items)
}

func (s *Server) getMeal(ctx context.Context, q querier, householdID, id string, lock bool) (*MealItem, error) {
	sql := `SELECT ` + mealColumns + ` FROM meal_plan_items WHERE id = $1 AND household_id = $2`
	if lock {
		sql += ` FOR UPDATE`
	}
	rows, _ := q.Query(ctx, sql, id, householdID)
	m, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[MealItem])
	if err != nil {
		return nil, err
	}
	items := []MealItem{*m}
	if err := s.resolveEaters(ctx, householdID, items); err != nil {
		return nil, err
	}
	return &items[0], nil
}

func (s *Server) createMeal(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		ID       string             `json:"id"`
		Date     string             `json:"date"`
		Slot     string             `json:"slot"`
		RecipeID *string            `json:"recipeId"`
		Title    string             `json:"title"`
		Servings int                `json:"servings"`
		Note     string             `json:"note"`
		Position *float64           `json:"position"`
		Portions map[string]float64 `json:"portions"`
		Leftover bool               `json:"leftover"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	portions, err := s.cleanPortions(ctx, u.HouseholdID, in.Portions)
	if err != nil {
		return err
	}
	if in.ID != "" {
		if existing, err := s.getMeal(ctx, s.db, u.HouseholdID, in.ID, false); err == nil {
			return ok(w, existing) // offline retry
		}
	} else {
		in.ID = randomUUID()
	}
	if _, err := parseDate(in.Date); err != nil {
		return err
	}
	if !slices.Contains(mealSlots, in.Slot) {
		return errBadRequest("Unknown meal slot")
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.RecipeID != nil {
		var title string
		var servings int
		if err := s.db.QueryRow(ctx, `SELECT title, servings FROM recipes WHERE id = $1 AND household_id = $2`, *in.RecipeID, u.HouseholdID).
			Scan(&title, &servings); err != nil {
			return errBadRequest("Unknown recipe")
		}
		in.Title = title
		if in.Servings == 0 {
			in.Servings = servings
		}
	}
	if in.Title == "" || len(in.Title) > 150 {
		return errBadRequest("Dish name is required")
	}
	if in.Servings < 1 || in.Servings > 50 {
		in.Servings = 2
	}
	pos := 0.0
	if in.Position != nil {
		pos = *in.Position
	} else if err := s.db.QueryRow(ctx, `SELECT coalesce(max(position), 0) + 1 FROM meal_plan_items
		WHERE household_id = $1 AND date = $2 AND slot = $3`, u.HouseholdID, in.Date, in.Slot).Scan(&pos); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO meal_plan_items (id, household_id, date, slot, position, recipe_id, title, servings, note, portions, leftover)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		in.ID, u.HouseholdID, in.Date, in.Slot, pos, in.RecipeID, in.Title, in.Servings, truncate(in.Note, 300), portions, in.Leftover); err != nil {
		return err
	}
	item, err := s.getMeal(ctx, s.db, u.HouseholdID, in.ID, false)
	if err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventMeals.from(r))
	return writeJSON(w, http.StatusCreated, item)
}

func (s *Server) updateMeal(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		Date     *string         `json:"date"`
		Slot     *string         `json:"slot"`
		Servings *int            `json:"servings"`
		Note     *string         `json:"note"`
		Position *float64        `json:"position"`
		Cooked   *bool           `json:"cooked"`
		Leftover *bool           `json:"leftover"`
		Portions json.RawMessage `json:"portions"` // null resets to the default
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	setPortions := len(in.Portions) > 0
	var portions map[string]float64
	if setPortions {
		var raw map[string]float64
		if err := json.Unmarshal(in.Portions, &raw); err != nil {
			return errBadRequest("Portions must map members to servings")
		}
		var err error
		if portions, err = s.cleanPortions(ctx, u.HouseholdID, raw); err != nil {
			return err
		}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	m, err := s.getMeal(ctx, tx, u.HouseholdID, chi.URLParam(r, "id"), true)
	if err != nil {
		return err
	}
	prevDate := m.Date
	if in.Date != nil {
		if _, err := parseDate(*in.Date); err != nil {
			return err
		}
		m.Date = *in.Date
	}
	if in.Slot != nil {
		if !slices.Contains(mealSlots, *in.Slot) {
			return errBadRequest("Unknown meal slot")
		}
		m.Slot = *in.Slot
	}
	if in.Servings != nil {
		if *in.Servings < 1 || *in.Servings > 50 {
			return errBadRequest("Servings must be 1–50")
		}
		m.Servings = *in.Servings
	}
	if in.Note != nil {
		m.Note = truncate(*in.Note, 300)
	}
	if in.Position != nil {
		m.Position = *in.Position
	}
	if setPortions {
		m.Portions = portions
	}
	if in.Leftover != nil {
		m.Leftover = *in.Leftover
	}
	if _, err := tx.Exec(ctx, `UPDATE meal_plan_items SET date = $2, slot = $3, servings = $4, note = $5, position = $6, portions = $7, leftover = $8 WHERE id = $1`,
		m.ID, m.Date, m.Slot, m.Servings, m.Note, m.Position, m.Portions, m.Leftover); err != nil {
		return err
	}
	pantryChanged := false
	if in.Cooked != nil && *in.Cooked != (m.CookedAt != nil) {
		if *in.Cooked {
			err = s.markCooked(ctx, tx, u, m)
		} else {
			err = s.unmarkCooked(ctx, tx, m)
		}
		if err != nil {
			return err
		}
		pantryChanged = true
	}
	tasksMoved, err := moveCookingTasks(ctx, tx, m.ID, prevDate, m.Date)
	if err != nil {
		return err
	}
	updated, err := s.getMeal(ctx, tx, u.HouseholdID, m.ID, false)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventMeals.from(r))
	if tasksMoved {
		s.hub.Publish(u.HouseholdID, eventTasks.from(r))
	}
	if pantryChanged {
		s.hub.Publish(u.HouseholdID, eventPantry.from(r))
		s.hub.Publish(u.HouseholdID, eventRecipes.from(r)) // times cooked
	}
	return ok(w, updated)
}

type deduction struct {
	PantryID string  `json:"pantryId"`
	Amount   float64 `json:"amount"` // in the pantry item's unit
}

// markCooked deducts the recipe's ingredients (scaled to servings) from the pantry where
// the units are compatible, remembering exactly what was taken so it can be undone.
func (s *Server) markCooked(ctx context.Context, tx pgx.Tx, u *User, m *MealItem) error {
	deds := []deduction{}
	if m.RecipeID != nil && !m.Leftover { // a leftover was taken from the pantry when first cooked
		rows, err := tx.Query(ctx, `SELECT p.id, p.quantity, p.unit, ri.quantity, ri.unit, r.servings
			FROM recipe_ingredients ri
			JOIN recipes r ON r.id = ri.recipe_id
			JOIN pantry_items p ON p.ingredient_id = ri.ingredient_id AND p.household_id = r.household_id
			WHERE ri.recipe_id = $1 AND ri.quantity IS NOT NULL AND p.quantity IS NOT NULL
			FOR UPDATE OF p`, *m.RecipeID)
		if err != nil {
			return err
		}
		type need struct {
			pantryID string
			have     float64
			amount   float64
		}
		var needs []need
		for rows.Next() {
			var pid, punit, runit string
			var have, qty float64
			var servings int
			if err := rows.Scan(&pid, &have, &punit, &qty, &runit, &servings); err != nil {
				rows.Close()
				return err
			}
			amt, ok := convert(qty*float64(m.Servings)/float64(max(servings, 1)), runit, punit)
			if !ok || amt <= 0 {
				continue
			}
			needs = append(needs, need{pid, have, min(amt, have)})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, n := range needs {
			if n.amount <= 0 {
				continue
			}
			if _, err := tx.Exec(ctx, `UPDATE pantry_items SET quantity = greatest(quantity - $2, 0), updated_at = now() WHERE id = $1`, n.pantryID, n.amount); err != nil {
				return err
			}
			deds = append(deds, deduction{n.pantryID, n.amount})
		}
	}
	b, _ := json.Marshal(deds)
	_, err := tx.Exec(ctx, `UPDATE meal_plan_items SET cooked_at = now(), cooked_by = $2, deductions = $3 WHERE id = $1`, m.ID, u.ID, b)
	return err
}

func (s *Server) unmarkCooked(ctx context.Context, tx pgx.Tx, m *MealItem) error {
	var deds []deduction
	if err := tx.QueryRow(ctx, `SELECT deductions FROM meal_plan_items WHERE id = $1`, m.ID).Scan(&deds); err != nil {
		return err
	}
	for _, d := range deds {
		if _, err := tx.Exec(ctx, `UPDATE pantry_items SET quantity = quantity + $2, updated_at = now() WHERE id = $1`, d.PantryID, d.Amount); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `UPDATE meal_plan_items SET cooked_at = NULL, cooked_by = NULL, deductions = '[]' WHERE id = $1`, m.ID)
	return err
}

func (s *Server) deleteMeal(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	if _, err := s.db.Exec(r.Context(), `DELETE FROM meal_plan_items WHERE id = $1 AND household_id = $2`, chi.URLParam(r, "id"), u.HouseholdID); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventMeals.from(r))
	return noContent(w)
}

func (s *Server) updateMealTimes(ctx context.Context, householdID string, times map[string]string) error {
	clean := map[string]string{}
	for _, slot := range mealSlots {
		t, found := times[slot]
		if !found {
			continue
		}
		if _, err := parseClock(t); err != nil {
			return errBadRequest("Meal times must be HH:MM")
		}
		clean[slot] = t
	}
	b, _ := json.Marshal(clean)
	_, err := s.db.Exec(ctx, `UPDATE households SET meal_times = meal_times || $2::jsonb WHERE id = $1`, householdID, b)
	return err
}

// moveCookingTasks keeps a dish's open cooking tasks due on the dish's new day, and
// updates the day in their description ("Lunch on Mon 26 Oct").
func moveCookingTasks(ctx context.Context, q querier, mealID, from, to string) (bool, error) {
	if from == to {
		return false, nil
	}
	f, err1 := time.Parse(time.DateOnly, from)
	t, err2 := time.Parse(time.DateOnly, to)
	if err1 != nil || err2 != nil {
		return false, nil
	}
	tag, err := q.Exec(ctx, `UPDATE tasks SET due_date = $2::date, description = replace(description, $3, $4), updated_at = now()
		WHERE meal_id = $1 AND status <> 'done'`, mealID, to, " on "+f.Format("Mon 2 Jan"), " on "+t.Format("Mon 2 Jan"))
	return tag.RowsAffected() > 0, err
}
