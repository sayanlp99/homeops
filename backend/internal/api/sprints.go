package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type Sprint struct {
	ID          string     `json:"id" db:"id"`
	Number      int        `json:"number" db:"number"`
	Goal        string     `json:"goal" db:"goal"`
	StartsOn    string     `json:"startsOn" db:"starts_on"`
	EndsOn      string     `json:"endsOn" db:"ends_on"`
	Status      string     `json:"status" db:"status"`
	StartedAt   *time.Time `json:"startedAt" db:"started_at"`
	CompletedAt *time.Time `json:"completedAt" db:"completed_at"`
	TaskCount   int        `json:"taskCount" db:"task_count"`
	DoneCount   int        `json:"doneCount" db:"done_count"`
	Points      int        `json:"points" db:"points"`
	DonePoints  int        `json:"donePoints" db:"done_points"`
}

const sprintSelect = `SELECT s.id, s.number, s.goal, s.starts_on::text AS starts_on, s.ends_on::text AS ends_on, s.status,
	s.started_at, s.completed_at,
	count(t.id)::int AS task_count,
	count(t.id) FILTER (WHERE t.status = 'done')::int AS done_count,
	coalesce(sum(t.points), 0)::int AS points,
	coalesce(sum(t.points) FILTER (WHERE t.status = 'done'), 0)::int AS done_points
	FROM sprints s LEFT JOIN tasks t ON t.sprint_id = s.id`

func (s *Server) listSprints(w http.ResponseWriter, r *http.Request) error {
	rows, _ := s.db.Query(r.Context(), sprintSelect+` WHERE s.household_id = $1 GROUP BY s.id ORDER BY s.number DESC LIMIT 60`,
		currentUser(r).HouseholdID)
	sprints, err := pgx.CollectRows(rows, pgx.RowToStructByName[Sprint])
	if err != nil {
		return err
	}
	return ok(w, sprints)
}

func (s *Server) getSprint(ctx context.Context, householdID, id string) (*Sprint, error) {
	rows, _ := s.db.Query(ctx, sprintSelect+` WHERE s.household_id = $1 AND s.id = $2 GROUP BY s.id`, householdID, id)
	return pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByName[Sprint])
}

func (s *Server) updateSprint(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in struct {
		Goal *string `json:"goal"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Goal != nil {
		if _, err := s.db.Exec(r.Context(), `UPDATE sprints SET goal = $3 WHERE id = $1 AND household_id = $2`,
			chi.URLParam(r, "id"), u.HouseholdID, truncate(strings.TrimSpace(*in.Goal), 300)); err != nil {
			return err
		}
	}
	sp, err := s.getSprint(r.Context(), u.HouseholdID, chi.URLParam(r, "id"))
	if err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventSprints.from(r))
	return ok(w, sp)
}

func (s *Server) startSprint(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockHousehold(ctx, tx, u.HouseholdID); err != nil {
		return err
	}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sprints WHERE household_id = $1 AND status = 'active')`, u.HouseholdID).Scan(&active); err != nil {
		return err
	}
	if active {
		return errConflict("Complete the current sprint before starting the next one")
	}
	tag, err := tx.Exec(ctx, `UPDATE sprints SET status = 'active', started_at = now() WHERE id = $1 AND household_id = $2 AND status = 'planned'`,
		chi.URLParam(r, "id"), u.HouseholdID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errBadRequest("Only a planned sprint can be started")
	}
	// Backlog-status tasks pulled into the sprint become To Do once it starts.
	if _, err := tx.Exec(ctx, `UPDATE tasks SET status = 'todo', updated_at = now() WHERE sprint_id = $1 AND status = 'backlog'`, chi.URLParam(r, "id")); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if err := s.ensureSprints(ctx, u.HouseholdID); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventSprints.from(r))
	s.hub.Publish(u.HouseholdID, eventTasks.from(r))
	return noContent(w)
}

func (s *Server) completeSprint(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in struct {
		MoveTo string `json:"moveTo"` // "next" (default) or "backlog"
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockHousehold(ctx, tx, u.HouseholdID); err != nil {
		return err
	}
	var endsOn time.Time
	var status string
	if err := tx.QueryRow(ctx, `SELECT ends_on, status FROM sprints WHERE id = $1 AND household_id = $2`,
		chi.URLParam(r, "id"), u.HouseholdID).Scan(&endsOn, &status); err != nil {
		return err
	}
	if status != "active" {
		return errBadRequest("Only the active sprint can be completed")
	}
	h, err := s.getHousehold(ctx, u.HouseholdID)
	if err != nil {
		return err
	}
	if err := completeSprintTx(ctx, tx, h, chi.URLParam(r, "id"), endsOn, in.MoveTo == "backlog"); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventSprints.from(r))
	s.hub.Publish(u.HouseholdID, eventTasks.from(r))
	return noContent(w)
}

// completeSprintTx closes a sprint and carries unfinished work to the next sprint
// (created if needed) or back to the backlog.
func completeSprintTx(ctx context.Context, tx pgx.Tx, h *Household, sprintID string, endsOn time.Time, toBacklog bool) error {
	if _, err := tx.Exec(ctx, `UPDATE sprints SET status = 'completed', completed_at = now() WHERE id = $1`, sprintID); err != nil {
		return err
	}
	if toBacklog {
		_, err := tx.Exec(ctx, `UPDATE tasks SET sprint_id = NULL, status = 'backlog', updated_at = now()
			WHERE sprint_id = $1 AND status <> 'done'`, sprintID)
		return err
	}
	next, err := nextSprintAfter(ctx, tx, h, endsOn)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE tasks SET sprint_id = $2, updated_at = now() WHERE sprint_id = $1 AND status <> 'done'`, sprintID, next)
	return err
}

// nextSprintAfter returns the planned sprint starting after the given date, creating it if needed.
func nextSprintAfter(ctx context.Context, q querier, h *Household, after time.Time) (string, error) {
	var id string
	err := q.QueryRow(ctx, `SELECT id FROM sprints WHERE household_id = $1 AND status = 'planned' AND starts_on > $2
		ORDER BY starts_on LIMIT 1`, h.ID, after).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	start := after.AddDate(0, 0, 1)
	return createSprint(ctx, q, h.ID, start, "planned")
}

func createSprint(ctx context.Context, q querier, householdID string, start time.Time, status string) (string, error) {
	var id string
	var startedAt *time.Time
	if status == "active" {
		now := time.Now()
		startedAt = &now
	}
	err := q.QueryRow(ctx, `INSERT INTO sprints (household_id, number, starts_on, ends_on, status, started_at)
		VALUES ($1, (SELECT coalesce(max(number), 0) + 1 FROM sprints WHERE household_id = $1), $2, $3, $4, $5) RETURNING id`,
		householdID, start, start.AddDate(0, 0, 6), status, startedAt).Scan(&id)
	return id, err
}

// lockHousehold serializes sprint lifecycle changes for one household within a transaction.
func lockHousehold(ctx context.Context, tx pgx.Tx, householdID string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "sprints:"+householdID)
	return err
}

// weekStart returns the most recent date (at or before d) that falls on startDow.
func weekStart(d time.Time, startDow int) time.Time {
	offset := (int(d.Weekday()) - startDow + 7) % 7
	return time.Date(d.Year(), d.Month(), d.Day()-offset, 0, 0, 0, 0, time.UTC)
}

// localToday returns today's date in the household's time zone, as a UTC-midnight value
// so date arithmetic is free of DST surprises.
func localToday(h *Household, now time.Time) time.Time {
	t := now.In(h.Location())
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// ensureSprints keeps the cadence going: there is always a sprint for this week (when
// auto-roll is on, or on first run), expired sprints roll over, and the next week's sprint
// exists for planning. It also materializes recurring chores into those sprints.
func (s *Server) ensureSprints(ctx context.Context, householdID string) error {
	h, err := s.getHousehold(ctx, householdID)
	if err != nil {
		return err
	}
	today := localToday(h, time.Now())

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockHousehold(ctx, tx, h.ID); err != nil {
		return err
	}

	var events []string
	var activeID string
	var activeEnds time.Time
	err = tx.QueryRow(ctx, `SELECT id, ends_on FROM sprints WHERE household_id = $1 AND status = 'active'`, h.ID).Scan(&activeID, &activeEnds)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if activeID != "" && activeEnds.Before(today) && h.AutoRoll {
		if err := completeSprintTx(ctx, tx, h, activeID, activeEnds, false); err != nil {
			return err
		}
		activeID = ""
		events = append(events, "rolled")
	}

	if activeID == "" {
		var total int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM sprints WHERE household_id = $1`, h.ID).Scan(&total); err != nil {
			return err
		}
		if total == 0 || h.AutoRoll {
			// Start the planned sprint whose window has begun, or create one for this week.
			var plannedID string
			err := tx.QueryRow(ctx, `SELECT id FROM sprints WHERE household_id = $1 AND status = 'planned' AND starts_on <= $2
				ORDER BY starts_on DESC LIMIT 1`, h.ID, today).Scan(&plannedID)
			switch {
			case err == nil:
				if _, err := tx.Exec(ctx, `UPDATE sprints SET status = 'active', started_at = now() WHERE id = $1`, plannedID); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE tasks SET status = 'todo' WHERE sprint_id = $1 AND status = 'backlog'`, plannedID); err != nil {
					return err
				}
				activeID = plannedID
				events = append(events, "started")
			case errors.Is(err, pgx.ErrNoRows):
				var covered bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sprints WHERE household_id = $1 AND starts_on <= $2 AND ends_on >= $2)`,
					h.ID, today).Scan(&covered); err != nil {
					return err
				}
				if !covered {
					if activeID, err = createSprint(ctx, tx, h.ID, weekStart(today, h.SprintStartDow), "active"); err != nil {
						return err
					}
					events = append(events, "started")
				}
			default:
				return err
			}
		}
	}

	// Always have the next sprint ready to plan into.
	var lastEnd time.Time
	var lastStatus string
	if err := tx.QueryRow(ctx, `SELECT ends_on, status FROM sprints WHERE household_id = $1 ORDER BY starts_on DESC LIMIT 1`,
		h.ID).Scan(&lastEnd, &lastStatus); err == nil && lastStatus != "planned" {
		if _, err := createSprint(ctx, tx, h.ID, lastEnd.AddDate(0, 0, 1), "planned"); err != nil {
			return err
		}
		events = append(events, "planned")
	}

	generated, err := s.generateRecurring(ctx, tx, h, today)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if len(events) > 0 {
		s.hub.Publish(h.ID, eventSprints.from(nil))
		s.hub.Publish(h.ID, eventTasks.from(nil))
	}
	if generated > 0 {
		s.hub.Publish(h.ID, eventTasks.from(nil))
	}
	for _, e := range events {
		if e == "started" && activeID != "" {
			s.announceSprintStart(ctx, h, activeID)
		}
	}
	return nil
}

func (s *Server) announceSprintStart(ctx context.Context, h *Household, sprintID string) {
	sp, err := s.getSprint(ctx, h.ID, sprintID)
	if err != nil {
		logErr("load sprint", err)
		return
	}
	err = s.notifyHousehold(ctx, h.ID, notification{
		Kind:      "ceremony",
		DedupeKey: "sprint-start:" + sp.ID,
		Title:     fmt.Sprintf("Sprint %d has started", sp.Number),
		Body:      fmt.Sprintf("%d tasks · %d points planned. Have a good week!", sp.TaskCount, sp.Points),
		URL:       "/board",
	})
	logErr("announce sprint", err)
}
