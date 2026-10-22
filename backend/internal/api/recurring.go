package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type Recurring struct {
	ID          string   `json:"id" db:"id"`
	Title       string   `json:"title" db:"title"`
	Description string   `json:"description" db:"description"`
	Priority    int      `json:"priority" db:"priority"`
	Points      *int     `json:"points" db:"points"`
	Labels      []string `json:"labels" db:"labels"`
	AssigneeID  *string  `json:"assigneeId" db:"assignee_id"`
	Rotate      bool     `json:"rotate" db:"rotate"`
	Freq        string   `json:"freq" db:"freq"`
	Interval    int      `json:"interval" db:"interval"`
	Weekdays    []int    `json:"weekdays" db:"weekdays"`
	MonthDay    *int     `json:"monthDay" db:"month_day"`
	AnchorDate  string   `json:"anchorDate" db:"anchor_date"`
	Active      bool     `json:"active" db:"active"`
	RotateIndex int      `json:"-" db:"rotate_index"`
}

const recurringColumns = `id, title, description, priority, points, labels, assignee_id, rotate, freq, interval,
	weekdays, month_day, anchor_date::text AS anchor_date, active, rotate_index`

func (s *Server) listRecurring(w http.ResponseWriter, r *http.Request) error {
	rows, _ := s.db.Query(r.Context(), `SELECT `+recurringColumns+` FROM recurring_tasks WHERE household_id = $1 ORDER BY title`,
		currentUser(r).HouseholdID)
	out, err := pgx.CollectRows(rows, pgx.RowToStructByName[Recurring])
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (in *Recurring) validate() error {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > 200 {
		return errBadRequest("Title is required (max 200 characters)")
	}
	if in.Priority < 0 || in.Priority > 4 {
		return errBadRequest("Priority must be 0–4")
	}
	if in.Interval < 1 || in.Interval > 52 {
		return errBadRequest("Repeat interval must be 1–52")
	}
	switch in.Freq {
	case "daily":
	case "weekly":
		if len(in.Weekdays) == 0 {
			return errBadRequest("Pick at least one weekday")
		}
		for _, d := range in.Weekdays {
			if d < 0 || d > 6 {
				return errBadRequest("Weekdays must be 0–6")
			}
		}
	case "monthly":
		if in.MonthDay == nil || *in.MonthDay < 1 || *in.MonthDay > 31 {
			return errBadRequest("Pick a day of the month (1–31)")
		}
	default:
		return errBadRequest("Frequency must be daily, weekly or monthly")
	}
	if in.Labels == nil {
		in.Labels = []string{}
	}
	if in.Weekdays == nil {
		in.Weekdays = []int{}
	}
	if in.AnchorDate != "" {
		if _, err := time.Parse(time.DateOnly, in.AnchorDate); err != nil {
			return errBadRequest("Start date must be YYYY-MM-DD")
		}
	}
	if in.Rotate {
		in.AssigneeID = nil
	}
	return nil
}

func (s *Server) createRecurring(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	in := Recurring{Priority: 2, Interval: 1, Active: true}
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	if in.AnchorDate == "" {
		h, err := s.getHousehold(r.Context(), u.HouseholdID)
		if err != nil {
			return err
		}
		in.AnchorDate = localToday(h, time.Now()).Format(time.DateOnly)
	}
	if err := s.db.QueryRow(r.Context(), `INSERT INTO recurring_tasks (household_id, title, description, priority, points,
		labels, assignee_id, rotate, freq, interval, weekdays, month_day, anchor_date, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::date, $14) RETURNING id`,
		u.HouseholdID, in.Title, in.Description, in.Priority, in.Points, in.Labels, in.AssigneeID, in.Rotate,
		in.Freq, in.Interval, in.Weekdays, in.MonthDay, in.AnchorDate, in.Active).Scan(&in.ID); err != nil {
		return err
	}
	return s.afterRecurringChange(w, r, in)
}

func (s *Server) updateRecurring(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in Recurring
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	in.ID = chi.URLParam(r, "id")
	tag, err := s.db.Exec(r.Context(), `UPDATE recurring_tasks SET title = $3, description = $4, priority = $5, points = $6,
		labels = $7, assignee_id = $8, rotate = $9, freq = $10, interval = $11, weekdays = $12, month_day = $13,
		anchor_date = coalesce($14::date, anchor_date), active = $15
		WHERE id = $1 AND household_id = $2`,
		in.ID, u.HouseholdID, in.Title, in.Description, in.Priority, in.Points, in.Labels, in.AssigneeID, in.Rotate,
		in.Freq, in.Interval, in.Weekdays, in.MonthDay, nilIfEmpty(in.AnchorDate), in.Active)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNotFound("Recurring chore not found")
	}
	return s.afterRecurringChange(w, r, in)
}

func (s *Server) afterRecurringChange(w http.ResponseWriter, r *http.Request, in Recurring) error {
	u := currentUser(r)
	if err := s.ensureSprints(r.Context(), u.HouseholdID); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventRecurring.from(r))
	rows, _ := s.db.Query(r.Context(), `SELECT `+recurringColumns+` FROM recurring_tasks WHERE id = $1`, in.ID)
	out, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Recurring])
	if err != nil {
		return err
	}
	return ok(w, out)
}

// deleteRecurring removes the template and its not-yet-started future occurrences.
func (s *Server) deleteRecurring(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	id := chi.URLParam(r, "id")
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM tasks WHERE recurring_id = $1 AND household_id = $2 AND status IN ('backlog', 'todo')
		AND due_date >= current_date`, id, u.HouseholdID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM recurring_tasks WHERE id = $1 AND household_id = $2`, id, u.HouseholdID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventRecurring.from(r))
	s.hub.Publish(u.HouseholdID, eventTasks.from(r))
	return noContent(w)
}

// generateRecurring creates tasks for every occurrence that falls inside the active or
// planned sprints and hasn't been generated before. Returns the number of tasks created.
func (s *Server) generateRecurring(ctx context.Context, tx pgx.Tx, h *Household, today time.Time) (int, error) {
	rows, _ := tx.Query(ctx, `SELECT `+recurringColumns+` FROM recurring_tasks WHERE household_id = $1 AND active FOR UPDATE`, h.ID)
	templates, err := pgx.CollectRows(rows, pgx.RowToStructByName[Recurring])
	if err != nil || len(templates) == 0 {
		return 0, err
	}
	type window struct {
		id         string
		start, end time.Time
	}
	rows, _ = tx.Query(ctx, `SELECT id, starts_on, ends_on FROM sprints WHERE household_id = $1 AND status IN ('active', 'planned')
		ORDER BY starts_on`, h.ID)
	windows, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (window, error) {
		var w window
		err := row.Scan(&w.id, &w.start, &w.end)
		return w, err
	})
	if err != nil || len(windows) == 0 {
		return 0, err
	}
	var members []string
	rows, _ = tx.Query(ctx, `SELECT id FROM users WHERE household_id = $1 ORDER BY created_at`, h.ID)
	if members, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		return 0, err
	}

	created := 0
	for _, tpl := range templates {
		anchor, err := time.Parse(time.DateOnly, tpl.AnchorDate)
		if err != nil {
			continue
		}
		rotateIndex := tpl.RotateIndex
		for _, win := range windows {
			// Never backfill days that have already passed.
			from := maxDate(win.start, anchor, today)
			for d := from; !d.After(win.end); d = d.AddDate(0, 0, 1) {
				if !occursOn(&tpl, anchor, d) {
					continue
				}
				tag, err := tx.Exec(ctx, `INSERT INTO recurring_occurrences (recurring_id, occurs_on) VALUES ($1, $2)
					ON CONFLICT DO NOTHING`, tpl.ID, d)
				if err != nil {
					return 0, err
				}
				if tag.RowsAffected() == 0 {
					continue
				}
				due := d.Format(time.DateOnly)
				t := Task{
					Title: tpl.Title, Description: tpl.Description, Status: "todo", Priority: tpl.Priority,
					Points: tpl.Points, Labels: tpl.Labels, AssigneeID: tpl.AssigneeID, SprintID: &win.id,
					DueDate: &due, RecurringID: &tpl.ID, Rank: float64(d.Unix()) / 1e6,
				}
				if tpl.Rotate && len(members) > 0 {
					t.AssigneeID = &members[rotateIndex%len(members)]
					rotateIndex++
				}
				if err := s.insertTask(ctx, tx, h.ID, &t, ""); err != nil {
					return 0, err
				}
				created++
			}
		}
		if rotateIndex != tpl.RotateIndex {
			if _, err := tx.Exec(ctx, `UPDATE recurring_tasks SET rotate_index = $2 WHERE id = $1`, tpl.ID, rotateIndex); err != nil {
				return 0, err
			}
		}
	}
	return created, nil
}

// occursOn reports whether the template has an occurrence on date d (UTC midnight).
func occursOn(t *Recurring, anchor, d time.Time) bool {
	if d.Before(anchor) {
		return false
	}
	switch t.Freq {
	case "daily":
		return int(d.Sub(anchor).Hours()/24)%t.Interval == 0
	case "weekly":
		if !slices.Contains(t.Weekdays, int(d.Weekday())) {
			return false
		}
		weeks := int(weekStart(d, 1).Sub(weekStart(anchor, 1)).Hours() / (24 * 7))
		return weeks%t.Interval == 0
	case "monthly":
		if t.MonthDay == nil {
			return false
		}
		lastDay := time.Date(d.Year(), d.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if d.Day() != min(*t.MonthDay, lastDay) {
			return false
		}
		months := (d.Year()-anchor.Year())*12 + int(d.Month()) - int(anchor.Month())
		return months%t.Interval == 0
	}
	return false
}

func maxDate(ds ...time.Time) time.Time {
	m := ds[0]
	for _, d := range ds[1:] {
		if d.After(m) {
			m = d
		}
	}
	return m
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
