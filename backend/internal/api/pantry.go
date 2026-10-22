package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type PantryItem struct {
	ID           string    `json:"id" db:"id"`
	IngredientID string    `json:"ingredientId" db:"ingredient_id"`
	Name         string    `json:"name" db:"name"`
	Aisle        string    `json:"aisle" db:"aisle"`
	Quantity     *float64  `json:"quantity" db:"quantity"`
	Unit         string    `json:"unit" db:"unit"`
	LowThreshold *float64  `json:"lowThreshold" db:"low_threshold"`
	Location     string    `json:"location" db:"location"`
	ExpiresOn    *string   `json:"expiresOn" db:"expires_on"`
	UpdatedAt    time.Time `json:"updatedAt" db:"updated_at"`
}

const pantrySelect = `SELECT p.id, p.ingredient_id, i.name, i.aisle, p.quantity, p.unit, p.low_threshold, p.location,
	p.expires_on::text AS expires_on, p.updated_at
	FROM pantry_items p JOIN ingredients i ON i.id = p.ingredient_id`

var pantryLocations = []string{"pantry", "fridge", "freezer"}

func (s *Server) listPantry(w http.ResponseWriter, r *http.Request) error {
	rows, _ := s.db.Query(r.Context(), pantrySelect+` WHERE p.household_id = $1 ORDER BY lower(i.name)`, currentUser(r).HouseholdID)
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[PantryItem])
	if err != nil {
		return err
	}
	return ok(w, items)
}

func (s *Server) upsertPantry(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		Name         string   `json:"name"`
		Quantity     *float64 `json:"quantity"`
		Unit         string   `json:"unit"`
		LowThreshold *float64 `json:"lowThreshold"`
		Location     string   `json:"location"`
		ExpiresOn    *string  `json:"expiresOn"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Location == "" {
		in.Location = "pantry"
	}
	if !slices.Contains(pantryLocations, in.Location) {
		return errBadRequest("Location must be pantry, fridge or freezer")
	}
	if in.ExpiresOn != nil && *in.ExpiresOn == "" {
		in.ExpiresOn = nil
	}
	if in.ExpiresOn != nil {
		if _, err := parseDate(*in.ExpiresOn); err != nil {
			return err
		}
	}
	if in.Quantity != nil && *in.Quantity < 0 {
		return errBadRequest("Quantity can't be negative")
	}
	iid, _, err := ensureIngredient(ctx, s.db, u.HouseholdID, in.Name)
	if err != nil {
		return err
	}
	var id string
	if err := s.db.QueryRow(ctx, `INSERT INTO pantry_items (household_id, ingredient_id, quantity, unit, low_threshold, location, expires_on)
		VALUES ($1, $2, $3, $4, $5, $6, $7::date)
		ON CONFLICT (household_id, ingredient_id) DO UPDATE SET quantity = EXCLUDED.quantity, unit = EXCLUDED.unit,
			low_threshold = EXCLUDED.low_threshold, location = EXCLUDED.location, expires_on = EXCLUDED.expires_on, updated_at = now()
		RETURNING id`, u.HouseholdID, iid, in.Quantity, normalizeUnit(in.Unit), in.LowThreshold, in.Location, in.ExpiresOn).Scan(&id); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventPantry.from(r))
	rows, _ := s.db.Query(ctx, pantrySelect+` WHERE p.id = $1`, id)
	item, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[PantryItem])
	if err != nil {
		return err
	}
	return ok(w, item)
}

// updatePantry is a PATCH: absent keys stay, null clears.
func (s *Server) updatePantry(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in map[string]json.RawMessage
	if err := decode(r, &in); err != nil {
		return err
	}
	id := chi.URLParam(r, "id")
	rows, _ := s.db.Query(ctx, pantrySelect+` WHERE p.id = $1 AND p.household_id = $2`, id, u.HouseholdID)
	p, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[PantryItem])
	if err != nil {
		return err
	}
	field := func(k string, dst any) error {
		if err := json.Unmarshal(in[k], dst); err != nil {
			return errBadRequest("Invalid " + k)
		}
		return nil
	}
	for k := range in {
		var err error
		switch k {
		case "quantity":
			err = field(k, &p.Quantity)
			if err == nil && p.Quantity != nil && *p.Quantity < 0 {
				err = errBadRequest("Quantity can't be negative")
			}
		case "unit":
			err = field(k, &p.Unit)
			p.Unit = normalizeUnit(p.Unit)
		case "lowThreshold":
			err = field(k, &p.LowThreshold)
		case "location":
			err = field(k, &p.Location)
			if err == nil && !slices.Contains(pantryLocations, p.Location) {
				err = errBadRequest("Location must be pantry, fridge or freezer")
			}
		case "expiresOn":
			err = field(k, &p.ExpiresOn)
			if err == nil && p.ExpiresOn != nil {
				if *p.ExpiresOn == "" {
					p.ExpiresOn = nil
				} else {
					_, err = parseDate(*p.ExpiresOn)
				}
			}
		}
		if err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(ctx, `UPDATE pantry_items SET quantity = $2, unit = $3, low_threshold = $4, location = $5,
		expires_on = $6::date, updated_at = now() WHERE id = $1`, id, p.Quantity, p.Unit, p.LowThreshold, p.Location, p.ExpiresOn); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventPantry.from(r))
	return ok(w, p)
}

func (s *Server) deletePantry(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	if _, err := s.db.Exec(r.Context(), `DELETE FROM pantry_items WHERE id = $1 AND household_id = $2`, chi.URLParam(r, "id"), u.HouseholdID); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventPantry.from(r))
	return noContent(w)
}

// restockPantry puts every low or empty pantry item on the shopping list (once).
func (s *Server) restockPantry(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	tag, err := s.db.Exec(ctx, `INSERT INTO grocery_items (id, household_id, ingredient_id, name, quantity, unit, aisle, note, source, created_by)
		SELECT gen_random_uuid(), p.household_id, p.ingredient_id, i.name,
			CASE WHEN p.low_threshold IS NOT NULL AND p.low_threshold > coalesce(p.quantity, 0) THEN p.low_threshold - coalesce(p.quantity, 0) END,
			p.unit, i.aisle, 'Restock', 'pantry', $2
		FROM pantry_items p JOIN ingredients i ON i.id = p.ingredient_id
		WHERE p.household_id = $1
			AND (p.quantity = 0 OR (p.low_threshold IS NOT NULL AND p.quantity <= p.low_threshold))
			AND NOT EXISTS (SELECT 1 FROM grocery_items g WHERE g.household_id = p.household_id AND g.ingredient_id = p.ingredient_id AND g.checked_at IS NULL)`,
		u.HouseholdID, u.ID)
	if err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventGrocery.from(r))
	return ok(w, map[string]int64{"added": tag.RowsAffected()})
}

func expiringSummary(names []string, days []int) string {
	parts := make([]string, len(names))
	for i, n := range names {
		switch {
		case days[i] < 0:
			parts[i] = n + " (expired)"
		case days[i] == 0:
			parts[i] = n + " (today)"
		case days[i] == 1:
			parts[i] = n + " (tomorrow)"
		default:
			parts[i] = n
		}
	}
	return strings.Join(parts, " · ")
}
