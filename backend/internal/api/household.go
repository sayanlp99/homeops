package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type Household struct {
	ID             string            `json:"id" db:"id"`
	Name           string            `json:"name" db:"name"`
	Timezone       string            `json:"timezone" db:"timezone"`
	SprintStartDow int               `json:"sprintStartDow" db:"sprint_start_dow"`
	PlanningDow    int               `json:"planningDow" db:"planning_dow"`
	PlanningTime   string            `json:"planningTime" db:"planning_time"`
	AutoRoll       bool              `json:"autoRoll" db:"auto_roll"`
	MealTimes      map[string]string `json:"mealTimes" db:"meal_times"`
}

const householdColumns = `id, name, timezone, sprint_start_dow, planning_dow, planning_time, auto_roll, meal_times`

type Member struct {
	ID          string `json:"id" db:"id"`
	Username    string `json:"username" db:"username"`
	DisplayName string `json:"displayName" db:"display_name"`
	Email       string `json:"email" db:"email"`
	Role        string `json:"role" db:"role"`
	Color       string `json:"color" db:"color"`
}

type Label struct {
	ID    string `json:"id" db:"id"`
	Name  string `json:"name" db:"name"`
	Color string `json:"color" db:"color"`
}

func (s *Server) getHousehold(ctx context.Context, id string) (*Household, error) {
	rows, _ := s.db.Query(ctx, `SELECT `+householdColumns+` FROM households WHERE id = $1`, id)
	return pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByName[Household])
}

func (h *Household) Location() *time.Location {
	loc, err := time.LoadLocation(h.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// bootstrap returns everything the app shell needs in one round trip.
func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	u := currentUser(r)
	if u == nil {
		var hasUsers bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users)`).Scan(&hasUsers); err != nil {
			return err
		}
		return ok(w, map[string]any{"setupRequired": !hasUsers, "user": nil})
	}
	households, err := s.myHouseholds(ctx, u.ID)
	if err != nil {
		return err
	}
	if u.HouseholdID == "" {
		return ok(w, map[string]any{
			"setupRequired":  false,
			"user":           u,
			"households":     households,
			"mailEnabled":    s.mailer.Enabled(),
			"vapidPublicKey": s.push.publicKey,
			"serverTime":     time.Now(),
		})
	}
	h, err := s.getHousehold(ctx, u.HouseholdID)
	if err != nil {
		return err
	}
	rows, _ := s.db.Query(ctx, `SELECT id, username, display_name, coalesce(email, '') AS email, role, color
		FROM members WHERE household_id = $1 ORDER BY created_at`, u.HouseholdID)
	members, err := pgx.CollectRows(rows, pgx.RowToStructByName[Member])
	if err != nil {
		return err
	}
	labels, err := s.labels(ctx, u.HouseholdID)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{
		"setupRequired":  false,
		"user":           u,
		"household":      h,
		"members":        members,
		"labels":         labels,
		"households":     households,
		"mailEnabled":    s.mailer.Enabled(),
		"vapidPublicKey": s.push.publicKey,
		"serverTime":     time.Now(),
	})
}

func (s *Server) updateHousehold(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	h, err := s.getHousehold(r.Context(), u.HouseholdID)
	if err != nil {
		return err
	}
	var in struct {
		Name           *string           `json:"name"`
		Timezone       *string           `json:"timezone"`
		SprintStartDow *int              `json:"sprintStartDow"`
		PlanningDow    *int              `json:"planningDow"`
		PlanningTime   *string           `json:"planningTime"`
		AutoRoll       *bool             `json:"autoRoll"`
		MealTimes      map[string]string `json:"mealTimes"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		h.Name = strings.TrimSpace(*in.Name)
	}
	if in.Timezone != nil {
		if _, err := time.LoadLocation(*in.Timezone); err != nil {
			return errBadRequest("Unknown time zone")
		}
		h.Timezone = *in.Timezone
	}
	for _, d := range []*int{in.SprintStartDow, in.PlanningDow} {
		if d != nil && (*d < 0 || *d > 6) {
			return errBadRequest("Day of week must be 0–6")
		}
	}
	if in.SprintStartDow != nil {
		h.SprintStartDow = *in.SprintStartDow
	}
	if in.PlanningDow != nil {
		h.PlanningDow = *in.PlanningDow
	}
	if in.PlanningTime != nil {
		if _, err := parseClock(*in.PlanningTime); err != nil {
			return errBadRequest("Planning time must be HH:MM")
		}
		h.PlanningTime = *in.PlanningTime
	}
	if in.AutoRoll != nil {
		h.AutoRoll = *in.AutoRoll
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE households SET name = $2, timezone = $3, sprint_start_dow = $4,
		planning_dow = $5, planning_time = $6, auto_roll = $7 WHERE id = $1`,
		h.ID, h.Name, h.Timezone, h.SprintStartDow, h.PlanningDow, h.PlanningTime, h.AutoRoll); err != nil {
		return err
	}
	if in.MealTimes != nil {
		if err := s.updateMealTimes(r.Context(), h.ID, in.MealTimes); err != nil {
			return err
		}
		if h, err = s.getHousehold(r.Context(), h.ID); err != nil {
			return err
		}
	}
	// A changed sprint start day only affects sprints that haven't started yet.
	if in.SprintStartDow != nil {
		if _, err := s.db.Exec(r.Context(), `DELETE FROM sprints WHERE household_id = $1 AND status = 'planned'
			AND NOT EXISTS (SELECT 1 FROM tasks WHERE sprint_id = sprints.id)`, h.ID); err != nil {
			return err
		}
		if err := s.ensureSprints(r.Context(), h.ID); err != nil {
			return err
		}
	}
	s.hub.Publish(h.ID, eventHousehold.from(r))
	return ok(w, h)
}

// ---- labels ----

func (s *Server) labels(ctx context.Context, householdID string) ([]Label, error) {
	rows, _ := s.db.Query(ctx, `SELECT id, name, color FROM labels WHERE household_id = $1 ORDER BY name`, householdID)
	return pgx.CollectRows(rows, pgx.RowToStructByName[Label])
}

func (s *Server) listLabels(w http.ResponseWriter, r *http.Request) error {
	l, err := s.labels(r.Context(), currentUser(r).HouseholdID)
	if err != nil {
		return err
	}
	return ok(w, l)
}

func (in *Label) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 30 {
		return errBadRequest("Label name is required (max 30 characters)")
	}
	if !isHexColor(in.Color) {
		return errBadRequest("Invalid color")
	}
	return nil
}

func (s *Server) createLabel(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in Label
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	if err := s.db.QueryRow(r.Context(), `INSERT INTO labels (household_id, name, color) VALUES ($1, $2, $3) RETURNING id`,
		u.HouseholdID, in.Name, in.Color).Scan(&in.ID); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventLabels.from(r))
	return ok(w, in)
}

func (s *Server) updateLabel(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in Label
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	in.ID = chi.URLParam(r, "id")
	tag, err := s.db.Exec(r.Context(), `UPDATE labels SET name = $3, color = $4 WHERE id = $1 AND household_id = $2`,
		in.ID, u.HouseholdID, in.Name, in.Color)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNotFound("Label not found")
	}
	s.hub.Publish(u.HouseholdID, eventLabels.from(r))
	return ok(w, in)
}

func (s *Server) deleteLabel(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	id := chi.URLParam(r, "id")
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), `DELETE FROM labels WHERE id = $1 AND household_id = $2`, id, u.HouseholdID); err != nil {
		return err
	}
	for _, table := range []string{"tasks", "recurring_tasks"} {
		if _, err := tx.Exec(r.Context(), `UPDATE `+table+` SET labels = array_remove(labels, $1::uuid)
			WHERE household_id = $2 AND $1::uuid = ANY(labels)`, id, u.HouseholdID); err != nil {
			return err
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventLabels.from(r))
	s.hub.Publish(u.HouseholdID, eventTasks.from(r))
	return noContent(w)
}
