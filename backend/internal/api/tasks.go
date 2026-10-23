package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type Task struct {
	ID          string     `json:"id" db:"id"`
	Number      int        `json:"number" db:"number"`
	Title       string     `json:"title" db:"title"`
	Description string     `json:"description" db:"description"`
	Status      string     `json:"status" db:"status"`
	Priority    int        `json:"priority" db:"priority"`
	Points      *int       `json:"points" db:"points"`
	Labels      []string   `json:"labels" db:"labels"`
	AssigneeID  *string    `json:"assigneeId" db:"assignee_id"`
	ReporterID  *string    `json:"reporterId" db:"reporter_id"`
	SprintID    *string    `json:"sprintId" db:"sprint_id"`
	DueDate     *string    `json:"dueDate" db:"due_date"`
	Rank        float64    `json:"rank" db:"rank"`
	RecurringID *string    `json:"recurringId" db:"recurring_id"`
	CompletedAt *time.Time `json:"completedAt" db:"completed_at"`
	CompletedBy *string    `json:"completedBy" db:"completed_by"`
	CreatedAt   time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time  `json:"updatedAt" db:"updated_at"`
}

const taskColumns = `id, number, title, description, status, priority, points, labels, assignee_id, reporter_id,
	sprint_id, due_date::text AS due_date, rank, recurring_id, completed_at, completed_by, created_at, updated_at`

var taskStatuses = map[string]bool{"backlog": true, "todo": true, "in_progress": true, "done": true}

func (s *Server) queryTasks(ctx context.Context, where string, args ...any) ([]Task, error) {
	rows, _ := s.db.Query(ctx, `SELECT `+taskColumns+` FROM tasks WHERE `+where+` ORDER BY rank, number`, args...)
	return pgx.CollectRows(rows, pgx.RowToStructByName[Task])
}

func (s *Server) getTask(ctx context.Context, q querier, householdID, id string, lock bool) (*Task, error) {
	sql := `SELECT ` + taskColumns + ` FROM tasks WHERE id = $1 AND household_id = $2`
	if lock {
		sql += ` FOR UPDATE`
	}
	rows, _ := q.Query(ctx, sql, id, householdID)
	return pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByName[Task])
}

// listTasks returns the working set: everything open, everything in current/upcoming
// sprints, and what was finished in the last 60 days. ?sprint=<id> fetches one sprint.
func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var tasks []Task
	var err error
	if sid := r.URL.Query().Get("sprint"); sid != "" {
		tasks, err = s.queryTasks(r.Context(), `household_id = $1 AND sprint_id = $2`, u.HouseholdID, sid)
	} else {
		tasks, err = s.queryTasks(r.Context(), `household_id = $1 AND (status <> 'done'
			OR completed_at > now() - interval '60 days'
			OR sprint_id IN (SELECT id FROM sprints WHERE household_id = $1 AND status <> 'completed'))`, u.HouseholdID)
	}
	if err != nil {
		return err
	}
	return ok(w, tasks)
}

// taskInput is a PATCH-style body: absent keys are left alone, null clears.
type taskInput map[string]json.RawMessage

func (in taskInput) has(k string) bool { _, ok := in[k]; return ok }

func (in taskInput) isNull(k string) bool { return string(in[k]) == "null" }

func (in taskInput) get(k string, dst any) error {
	if err := json.Unmarshal(in[k], dst); err != nil {
		return errBadRequest(fmt.Sprintf("Invalid value for %s", k))
	}
	return nil
}

// apply validates and copies fields from the input onto t.
func (s *Server) applyTaskInput(ctx context.Context, q querier, householdID string, t *Task, in taskInput) error {
	if in.has("title") {
		if err := in.get("title", &t.Title); err != nil {
			return err
		}
		t.Title = strings.TrimSpace(t.Title)
		if t.Title == "" || len(t.Title) > 200 {
			return errBadRequest("Title is required (max 200 characters)")
		}
	}
	if in.has("description") {
		if err := in.get("description", &t.Description); err != nil {
			return err
		}
		if len(t.Description) > 20000 {
			return errBadRequest("Description is too long")
		}
	}
	if in.has("status") {
		if err := in.get("status", &t.Status); err != nil {
			return err
		}
		if !taskStatuses[t.Status] {
			return errBadRequest("Invalid status")
		}
	}
	if in.has("priority") {
		if err := in.get("priority", &t.Priority); err != nil {
			return err
		}
		if t.Priority < 0 || t.Priority > 4 {
			return errBadRequest("Priority must be 0–4")
		}
	}
	if in.has("points") {
		if in.isNull("points") {
			t.Points = nil
		} else if err := in.get("points", &t.Points); err != nil {
			return err
		} else if *t.Points < 0 || *t.Points > 100 {
			return errBadRequest("Points must be 0–100")
		}
	}
	if in.has("rank") {
		if err := in.get("rank", &t.Rank); err != nil {
			return err
		}
	}
	if in.has("dueDate") {
		if in.isNull("dueDate") {
			t.DueDate = nil
		} else {
			var d string
			if err := in.get("dueDate", &d); err != nil {
				return err
			}
			if _, err := time.Parse(time.DateOnly, d); err != nil {
				return errBadRequest("Due date must be YYYY-MM-DD")
			}
			t.DueDate = &d
		}
	}
	if in.has("labels") {
		var labels []string
		if err := in.get("labels", &labels); err != nil {
			return err
		}
		if labels == nil {
			labels = []string{}
		}
		if len(labels) > 0 {
			var n int
			if err := q.QueryRow(ctx, `SELECT count(*) FROM labels WHERE household_id = $1 AND id = ANY($2::uuid[])`,
				householdID, labels).Scan(&n); err != nil {
				return err
			}
			if n != len(labels) {
				return errBadRequest("Unknown label")
			}
		}
		t.Labels = labels
	}
	for _, ref := range []struct {
		key, table string
		dst        **string
	}{{"assigneeId", "members", &t.AssigneeID}, {"sprintId", "sprints", &t.SprintID}} {
		if !in.has(ref.key) {
			continue
		}
		if in.isNull(ref.key) {
			*ref.dst = nil
			continue
		}
		var id string
		if err := in.get(ref.key, &id); err != nil {
			return err
		}
		var exists bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM `+ref.table+` WHERE id = $1::uuid AND household_id = $2)`,
			id, householdID).Scan(&exists); err != nil {
			return errBadRequest("Invalid " + ref.key)
		}
		if !exists {
			return errBadRequest("Unknown " + ref.key)
		}
		*ref.dst = &id
	}
	return nil
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in taskInput
	if err := decode(r, &in); err != nil {
		return err
	}
	t := Task{Status: "todo", Priority: 2, Labels: []string{}, ReporterID: &u.ID}
	if in.has("id") {
		if err := in.get("id", &t.ID); err != nil {
			return err
		}
		// Offline clients retry creates; make them idempotent.
		if existing, err := s.getTask(ctx, s.db, u.HouseholdID, t.ID, false); err == nil {
			return ok(w, existing)
		}
	}
	if !in.has("title") {
		return errBadRequest("Title is required")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.applyTaskInput(ctx, tx, u.HouseholdID, &t, in); err != nil {
		return err
	}
	if !in.has("rank") {
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(rank), 0) + 1024 FROM tasks WHERE household_id = $1`, u.HouseholdID).Scan(&t.Rank); err != nil {
			return err
		}
	}
	if err := s.insertTask(ctx, tx, u.HouseholdID, &t, u.ID); err != nil {
		return err
	}
	created, err := s.getTask(ctx, tx, u.HouseholdID, t.ID, false)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventTasks.from(r))
	if created.AssigneeID != nil && *created.AssigneeID != u.ID {
		s.notifyAssigned(ctx, u, created)
	}
	return writeJSON(w, http.StatusCreated, created)
}

// insertTask allocates the next human-readable number (HO-123) and inserts t.
func (s *Server) insertTask(ctx context.Context, q querier, householdID string, t *Task, actorID string) error {
	if err := q.QueryRow(ctx, `UPDATE households SET next_task_number = next_task_number + 1 WHERE id = $1
		RETURNING next_task_number - 1`, householdID).Scan(&t.Number); err != nil {
		return err
	}
	if t.ID == "" {
		if err := q.QueryRow(ctx, `SELECT gen_random_uuid()`).Scan(&t.ID); err != nil {
			return err
		}
	}
	var completedAt *time.Time
	var completedBy *string
	if t.Status == "done" {
		now := time.Now()
		completedAt, completedBy = &now, &actorID
	}
	_, err := q.Exec(ctx, `INSERT INTO tasks (id, household_id, number, title, description, status, priority, points, labels,
		assignee_id, reporter_id, sprint_id, due_date, rank, recurring_id, completed_at, completed_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::date, $14, $15, $16, $17)`,
		t.ID, householdID, t.Number, t.Title, t.Description, t.Status, t.Priority, t.Points, t.Labels,
		t.AssigneeID, t.ReporterID, t.SprintID, t.DueDate, t.Rank, t.RecurringID, completedAt, completedBy)
	return err
}

func (s *Server) updateTask(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in taskInput
	if err := decode(r, &in); err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	t, err := s.getTask(ctx, tx, u.HouseholdID, chi.URLParam(r, "id"), true)
	if err != nil {
		return err
	}
	prevStatus, prevAssignee := t.Status, t.AssigneeID
	if err := s.applyTaskInput(ctx, tx, u.HouseholdID, t, in); err != nil {
		return err
	}
	switch {
	case t.Status == "done" && prevStatus != "done":
		now := time.Now()
		t.CompletedAt, t.CompletedBy = &now, &u.ID
	case t.Status != "done":
		t.CompletedAt, t.CompletedBy = nil, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE tasks SET title = $2, description = $3, status = $4, priority = $5, points = $6,
		labels = $7, assignee_id = $8, sprint_id = $9, due_date = $10::date, rank = $11, completed_at = $12,
		completed_by = $13, updated_at = now() WHERE id = $1`,
		t.ID, t.Title, t.Description, t.Status, t.Priority, t.Points, t.Labels, t.AssigneeID, t.SprintID,
		t.DueDate, t.Rank, t.CompletedAt, t.CompletedBy); err != nil {
		return err
	}
	updated, err := s.getTask(ctx, tx, u.HouseholdID, t.ID, false)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventTasks.from(r))
	if updated.AssigneeID != nil && *updated.AssigneeID != u.ID && !sameID(prevAssignee, updated.AssigneeID) {
		s.notifyAssigned(ctx, u, updated)
	}
	return ok(w, updated)
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	if _, err := s.db.Exec(r.Context(), `DELETE FROM tasks WHERE id = $1 AND household_id = $2`,
		chi.URLParam(r, "id"), u.HouseholdID); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventTasks.from(r))
	return noContent(w)
}

func (s *Server) notifyAssigned(ctx context.Context, actor *User, t *Task) {
	err := s.notify(ctx, notification{
		HouseholdID: actor.HouseholdID,
		UserID:      *t.AssigneeID,
		Kind:        "task_assigned",
		DedupeKey:   fmt.Sprintf("assigned:%s:%d", t.ID, time.Now().Unix()),
		Title:       fmt.Sprintf("%s assigned you HO-%d", actor.DisplayName, t.Number),
		Body:        t.Title,
		URL:         fmt.Sprintf("/tasks/%d", t.Number),
	})
	logErr("notify assigned", err)
}

func sameID(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
