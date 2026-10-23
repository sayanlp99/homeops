package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type GroceryItem struct {
	ID           string     `json:"id" db:"id"`
	IngredientID *string    `json:"ingredientId" db:"ingredient_id"`
	Name         string     `json:"name" db:"name"`
	Quantity     *float64   `json:"quantity" db:"quantity"`
	Unit         string     `json:"unit" db:"unit"`
	Aisle        string     `json:"aisle" db:"aisle"`
	Note         string     `json:"note" db:"note"`
	Source       string     `json:"source" db:"source"`
	CheckedAt    *time.Time `json:"checkedAt" db:"checked_at"`
	CheckedBy    *string    `json:"checkedBy" db:"checked_by"`
	CreatedBy    *string    `json:"createdBy" db:"created_by"`
	CreatedAt    time.Time  `json:"createdAt" db:"created_at"`
}

const groceryColumns = `id, ingredient_id, name, quantity, unit, aisle, note, source, checked_at, checked_by, created_by, created_at`

func (s *Server) listGrocery(w http.ResponseWriter, r *http.Request) error {
	rows, _ := s.db.Query(r.Context(), `SELECT `+groceryColumns+` FROM grocery_items WHERE household_id = $1 ORDER BY aisle, lower(name)`,
		currentUser(r).HouseholdID)
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[GroceryItem])
	if err != nil {
		return err
	}
	return ok(w, items)
}

func (s *Server) getGrocery(ctx context.Context, householdID, id string) (*GroceryItem, error) {
	rows, _ := s.db.Query(ctx, `SELECT `+groceryColumns+` FROM grocery_items WHERE id = $1 AND household_id = $2`, id, householdID)
	return pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByName[GroceryItem])
}

// addGrocery accepts either structured fields or free text like "2 kg onions".
func (s *Server) addGrocery(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		ID       string   `json:"id"`
		Text     string   `json:"text"`
		Name     string   `json:"name"`
		Quantity *float64 `json:"quantity"`
		Unit     string   `json:"unit"`
		Note     string   `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.ID == "" {
		in.ID = randomUUID()
	} else if existing, err := s.getGrocery(ctx, u.HouseholdID, in.ID); err == nil {
		return ok(w, existing) // offline retry
	}
	if in.Text != "" {
		p := parseIngredientLine(in.Text)
		in.Name, in.Quantity, in.Unit = p.Name, p.Quantity, p.Unit
		if in.Note == "" {
			in.Note = p.Note
		}
	}
	iid, aisle, err := ensureIngredient(ctx, s.db, u.HouseholdID, in.Name)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO grocery_items (id, household_id, ingredient_id, name, quantity, unit, aisle, note, source, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'manual', $9)`,
		in.ID, u.HouseholdID, iid, cleanName(in.Name), in.Quantity, normalizeUnit(in.Unit), aisle, truncate(in.Note, 200), u.ID); err != nil {
		return err
	}
	item, err := s.getGrocery(ctx, u.HouseholdID, in.ID)
	if err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventGrocery.from(r))
	return writeJSON(w, http.StatusCreated, item)
}

func (s *Server) updateGrocery(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		Checked  *bool           `json:"checked"`
		Quantity json.RawMessage `json:"quantity"`
		Unit     *string         `json:"unit"`
		Note     *string         `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	id := chi.URLParam(r, "id")
	item, err := s.getGrocery(ctx, u.HouseholdID, id)
	if err != nil {
		return err
	}
	if in.Checked != nil {
		if *in.Checked && item.CheckedAt == nil {
			now := time.Now()
			item.CheckedAt, item.CheckedBy = &now, &u.ID
		} else if !*in.Checked {
			item.CheckedAt, item.CheckedBy = nil, nil
		}
	}
	if len(in.Quantity) > 0 {
		if string(in.Quantity) == "null" {
			item.Quantity = nil
		} else if err := json.Unmarshal(in.Quantity, &item.Quantity); err != nil {
			return errBadRequest("Invalid quantity")
		}
	}
	if in.Unit != nil {
		item.Unit = normalizeUnit(*in.Unit)
	}
	if in.Note != nil {
		item.Note = truncate(*in.Note, 200)
	}
	if _, err := s.db.Exec(ctx, `UPDATE grocery_items SET checked_at = $2, checked_by = $3, quantity = $4, unit = $5, note = $6 WHERE id = $1`,
		id, item.CheckedAt, item.CheckedBy, item.Quantity, item.Unit, item.Note); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventGrocery.from(r))
	return ok(w, item)
}

func (s *Server) deleteGrocery(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	if _, err := s.db.Exec(r.Context(), `DELETE FROM grocery_items WHERE id = $1 AND household_id = $2`, chi.URLParam(r, "id"), u.HouseholdID); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventGrocery.from(r))
	return noContent(w)
}

type need struct {
	ingredientID, name, aisle, unit string
	qty                             *float64 // nil = "to taste": buy only if not in pantry
	recipes                         []string
}

// generateGrocery builds the shopping list for planned, not-yet-cooked meals in a date
// range: ingredients are summed per unit dimension, scaled to servings, and reduced by
// what the pantry already holds. Previously generated unchecked items are replaced.
func (s *Server) generateGrocery(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		From           string `json:"from"`
		To             string `json:"to"`
		SubtractPantry *bool  `json:"subtractPantry"`
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
	if to.Before(from) || to.Sub(from) > 31*24*time.Hour {
		return errBadRequest("Pick a range of up to a month")
	}
	subtract := in.SubtractPantry == nil || *in.SubtractPantry

	rows, err := s.db.Query(ctx, `SELECT i.id, i.name, i.aisle, ri.quantity, ri.unit, m.servings, r.servings, r.title
		FROM meal_plan_items m
		JOIN recipes r ON r.id = m.recipe_id
		JOIN recipe_ingredients ri ON ri.recipe_id = r.id
		JOIN ingredients i ON i.id = ri.ingredient_id
		WHERE m.household_id = $1 AND m.date BETWEEN $2 AND $3 AND m.cooked_at IS NULL`, u.HouseholdID, from, to)
	if err != nil {
		return err
	}
	needs := map[string]*need{} // key: ingredient + unit dimension
	var order []string
	hasAmount := map[string]bool{}
	for rows.Next() {
		var iid, name, aisle, unit, title string
		var qty *float64
		var planned, base int
		if err := rows.Scan(&iid, &name, &aisle, &qty, &unit, &planned, &base, &title); err != nil {
			rows.Close()
			return err
		}
		if notShopped(name) {
			continue
		}
		key := iid + "|" + unitDim(unit)
		if qty == nil {
			key = iid + "|taste"
		} else {
			hasAmount[iid] = true
		}
		n := needs[key]
		if n == nil {
			n = &need{ingredientID: iid, name: name, aisle: aisle, unit: unit}
			needs[key] = n
			order = append(order, key)
		}
		if !slices.Contains(n.recipes, title) {
			n.recipes = append(n.recipes, title)
		}
		if qty != nil {
			scaled := *qty * float64(planned) / float64(max(base, 1))
			conv, _ := convert(scaled, unit, n.unit)
			if n.qty == nil {
				n.qty = new(float64)
			}
			*n.qty += conv
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	type stock struct {
		qty  *float64
		unit string
	}
	pantry := map[string]stock{}
	if subtract {
		prow, err := s.db.Query(ctx, `SELECT ingredient_id, quantity, unit FROM pantry_items WHERE household_id = $1`, u.HouseholdID)
		if err != nil {
			return err
		}
		for prow.Next() {
			var id, unit string
			var q *float64
			if err := prow.Scan(&id, &q, &unit); err != nil {
				prow.Close()
				return err
			}
			pantry[id] = stock{q, unit}
		}
		prow.Close()
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM grocery_items WHERE household_id = $1 AND source = 'plan' AND checked_at IS NULL`, u.HouseholdID); err != nil {
		return err
	}
	added, covered := 0, 0
	for _, key := range order {
		n := needs[key]
		if n.qty == nil && hasAmount[n.ingredientID] {
			continue // "salt to taste" is covered by "1 tsp salt" from another dish
		}
		qty, unit := n.qty, n.unit
		if st, have := pantry[n.ingredientID]; have {
			switch {
			case st.qty == nil: // in stock, amount untracked: assume enough
				covered++
				continue
			case qty == nil:
				if *st.qty > 0 {
					covered++
					continue
				}
			default:
				inStock, ok := convert(*st.qty, st.unit, unit)
				if !ok {
					// Cups of rice vs a 2 kg bag: can't compare, so trust that stock on hand is enough.
					if *st.qty > 0 {
						covered++
						continue
					}
					break
				}
				left := *qty - inStock
				if left <= 0.0001 {
					covered++
					continue
				}
				qty = &left
			}
		}
		if qty != nil {
			q, hu := humanize(*qty, unit)
			q = roundUpForShopping(q, hu)
			qty, unit = &q, hu
		}
		note := "For " + strings.Join(n.recipes, ", ")
		if len(n.recipes) > 2 {
			note = fmt.Sprintf("For %s and %d more", strings.Join(n.recipes[:2], ", "), len(n.recipes)-2)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO grocery_items (id, household_id, ingredient_id, name, quantity, unit, aisle, note, source, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'plan', $9)`,
			randomUUID(), u.HouseholdID, n.ingredientID, n.name, qty, unit, n.aisle, truncate(note, 200), u.ID); err != nil {
			return err
		}
		added++
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventGrocery.from(r))
	return ok(w, map[string]int{"added": added, "covered": covered})
}

// completeShopping records a trip, optionally stocks the pantry with what was bought,
// and clears checked items from the list.
func (s *Server) completeShopping(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		AddToPantry *bool    `json:"addToPantry"`
		Total       *float64 `json:"total"`
		Store       string   `json:"store"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Total != nil && (*in.Total < 0 || *in.Total > 1e7) {
		return errBadRequest("Invalid total")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, _ := tx.Query(ctx, `SELECT `+groceryColumns+` FROM grocery_items WHERE household_id = $1 AND checked_at IS NOT NULL FOR UPDATE`, u.HouseholdID)
	bought, err := pgx.CollectRows(rows, pgx.RowToStructByName[GroceryItem])
	if err != nil {
		return err
	}
	if len(bought) == 0 {
		return errBadRequest("Tick off what you bought first")
	}
	if in.AddToPantry == nil || *in.AddToPantry {
		for _, it := range bought {
			if it.IngredientID == nil {
				continue
			}
			if err := addToPantry(ctx, tx, u.HouseholdID, *it.IngredientID, it.Quantity, it.Unit, it.Aisle); err != nil {
				return err
			}
		}
	}
	summary := make([]map[string]any, len(bought))
	for i, it := range bought {
		summary[i] = map[string]any{"name": it.Name, "quantity": it.Quantity, "unit": it.Unit}
	}
	items, _ := json.Marshal(summary)
	var tripID string
	if err := tx.QueryRow(ctx, `INSERT INTO grocery_trips (household_id, completed_by, item_count, items, total_amount, store)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, u.HouseholdID, u.ID, len(bought), items, in.Total, truncate(strings.TrimSpace(in.Store), 60)).Scan(&tripID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM grocery_items WHERE household_id = $1 AND checked_at IS NOT NULL`, u.HouseholdID); err != nil {
		return err
	}
	// The bill becomes a shared Groceries expense, so the budget is up to date.
	var groceryCat *string
	if in.Total != nil && *in.Total > 0 {
		if err := s.ensureMoneyDefaults(ctx, u.HouseholdID); err != nil {
			return err
		}
		var cid string
		if err := tx.QueryRow(ctx, `SELECT id FROM money_categories WHERE household_id = $1 AND owner_id IS NULL AND lower(name) = 'groceries' AND NOT archived`,
			u.HouseholdID).Scan(&cid); err == nil {
			groceryCat = &cid
		}
		payee := strings.TrimSpace(in.Store)
		if payee == "" {
			payee = "Groceries"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO money_transactions (id, household_id, kind, amount, date, payee, note, category_id, paid_by, source,
			grocery_trip_id, created_by) VALUES ($1, $2, 'expense', $3, current_date, $4, $5, $6, $7, 'grocery', $8, $7)`,
			randomUUID(), u.HouseholdID, int64(math.Round(*in.Total*100)), truncate(payee, 80), fmt.Sprintf("Shopping trip · %d items", len(bought)),
			groceryCat, u.ID, tripID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventGrocery.from(r))
	s.hub.Publish(u.HouseholdID, eventPantry.from(r))
	if groceryCat != nil {
		s.hub.Publish(u.HouseholdID, eventMoney.from(r))
		s.checkBudgetAlerts(ctx, u.HouseholdID, groceryCat, time.Now().Format(time.DateOnly))
	}
	return ok(w, map[string]any{"tripId": tripID, "items": len(bought)})
}

// addToPantry increases stock, converting units when compatible; otherwise the newer
// purchase's unit wins (e.g. switching from "1 packet" to "500 g").
func addToPantry(ctx context.Context, tx pgx.Tx, householdID, ingredientID string, qty *float64, unit, aisle string) error {
	var id string
	var have *float64
	var haveUnit string
	err := tx.QueryRow(ctx, `SELECT id, quantity, unit FROM pantry_items WHERE household_id = $1 AND ingredient_id = $2 FOR UPDATE`,
		householdID, ingredientID).Scan(&id, &have, &haveUnit)
	location := "pantry"
	if aisle == "Dairy & Eggs" || aisle == "Meat & Fish" || aisle == "Vegetables" || aisle == "Fruits & Nuts" {
		location = "fridge"
	} else if aisle == "Frozen" {
		location = "freezer"
	}
	switch {
	case err == pgx.ErrNoRows:
		_, err = tx.Exec(ctx, `INSERT INTO pantry_items (household_id, ingredient_id, quantity, unit, location) VALUES ($1, $2, $3, $4, $5)`,
			householdID, ingredientID, qty, unit, location)
		return err
	case err != nil:
		return err
	}
	newQty, newUnit := qty, unit
	if qty != nil && have != nil {
		if conv, ok := convert(*qty, unit, haveUnit); ok {
			sum := *have + conv
			newQty, newUnit = &sum, haveUnit
		}
	} else if qty == nil {
		newQty, newUnit = have, haveUnit
	}
	_, err = tx.Exec(ctx, `UPDATE pantry_items SET quantity = $2, unit = $3, updated_at = now() WHERE id = $1`, id, newQty, newUnit)
	return err
}

type GroceryTrip struct {
	ID          string          `json:"id" db:"id"`
	CompletedBy *string         `json:"completedBy" db:"completed_by"`
	CompletedAt time.Time       `json:"completedAt" db:"completed_at"`
	ItemCount   int             `json:"itemCount" db:"item_count"`
	Items       json.RawMessage `json:"items" db:"items"`
	Total       *float64        `json:"total" db:"total_amount"`
	Store       string          `json:"store" db:"store"`
}

func (s *Server) listTrips(w http.ResponseWriter, r *http.Request) error {
	rows, _ := s.db.Query(r.Context(), `SELECT id, completed_by, completed_at, item_count, items, total_amount, store
		FROM grocery_trips WHERE household_id = $1 ORDER BY completed_at DESC LIMIT 30`, currentUser(r).HouseholdID)
	trips, err := pgx.CollectRows(rows, pgx.RowToStructByName[GroceryTrip])
	if err != nil {
		return err
	}
	return ok(w, trips)
}

var notShoppedRe = regexp.MustCompile(`(?i)^(hot |warm |cold |boiling |lukewarm |chilled )?(water|ice|ice cubes)$`)

// notShopped filters out things that never belong on a shopping list.
func notShopped(name string) bool { return notShoppedRe.MatchString(strings.TrimSpace(name)) }

// roundUpForShopping turns 0.92 cup into 1 cup and 3.2 onions into 4: you buy whole things.
func roundUpForShopping(q float64, unit string) float64 {
	step := 1.0
	switch unitDim(unit) {
	case "mass":
		if unit == "g" {
			step = 50
			if q < 100 {
				step = 10
			}
		} else {
			step = 0.25
		}
	case "volume":
		switch unit {
		case "ml":
			step = 50
		case "l":
			step = 0.25
		default: // tsp, tbsp, cup
			step = 0.25
		}
	}
	return math.Ceil(q/step-1e-9) * step
}
