package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type Retro struct {
	ID         string     `json:"id" db:"id"`
	SprintID   string     `json:"sprintId" db:"sprint_id"`
	Status     string     `json:"status" db:"status"`
	RevealedAt *time.Time `json:"revealedAt" db:"revealed_at"`
	ClosedAt   *time.Time `json:"closedAt" db:"closed_at"`
	CreatedAt  time.Time  `json:"createdAt" db:"created_at"`
}

type RetroCard struct {
	ID         string    `json:"id"`
	Col        string    `json:"col"`
	Text       string    `json:"text"`
	AuthorID   string    `json:"authorId"`
	AssigneeID *string   `json:"assigneeId"`
	TaskID     *string   `json:"taskId"`
	Votes      int       `json:"votes"`
	Voters     []string  `json:"voters"`
	CreatedAt  time.Time `json:"createdAt"`
}

var retroCols = []string{"went_well", "improve", "action"}

const retroColumns = `id, sprint_id, status, revealed_at, closed_at, created_at`

func (s *Server) loadRetro(ctx context.Context, householdID, id string) (*Retro, error) {
	rows, _ := s.db.Query(ctx, `SELECT `+retroColumns+` FROM retros WHERE id = $1 AND household_id = $2`, id, householdID)
	return pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByName[Retro])
}

// getRetro returns a sprint's retro, creating it on first open. While cards are being
// collected, each person sees only their own cards plus a count of the other's, so
// nobody is anchored by what the other wrote.
func (s *Server) getRetro(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	sprintID := r.URL.Query().Get("sprint")
	var status string
	if err := s.db.QueryRow(ctx, `SELECT status FROM sprints WHERE id = $1 AND household_id = $2`, sprintID, u.HouseholdID).Scan(&status); err != nil {
		return errNotFound("Sprint not found")
	}
	if status == "planned" {
		return errBadRequest("A retro is for a sprint that has started")
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO retros (household_id, sprint_id) VALUES ($1, $2) ON CONFLICT (sprint_id) DO NOTHING`, u.HouseholdID, sprintID); err != nil {
		return err
	}
	rows, _ := s.db.Query(ctx, `SELECT `+retroColumns+` FROM retros WHERE sprint_id = $1`, sprintID)
	retro, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByName[Retro])
	if err != nil {
		return err
	}
	cards, hidden, err := s.retroCards(ctx, retro, u.ID)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"retro": retro, "cards": cards, "hidden": hidden})
}

func (s *Server) retroCards(ctx context.Context, retro *Retro, userID string) ([]RetroCard, map[string]int, error) {
	rows, err := s.db.Query(ctx, `SELECT c.id, c.col, c.text, c.author_id, c.assignee_id, c.task_id, c.created_at,
		coalesce(array_agg(v.user_id::text) FILTER (WHERE v.user_id IS NOT NULL), '{}')
		FROM retro_cards c LEFT JOIN retro_votes v ON v.card_id = c.id
		WHERE c.retro_id = $1 GROUP BY c.id ORDER BY c.created_at`, retro.ID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	cards := []RetroCard{}
	hidden := map[string]int{}
	for rows.Next() {
		var c RetroCard
		if err := rows.Scan(&c.ID, &c.Col, &c.Text, &c.AuthorID, &c.AssigneeID, &c.TaskID, &c.CreatedAt, &c.Voters); err != nil {
			return nil, nil, err
		}
		c.Votes = len(c.Voters)
		if retro.Status == "collecting" && c.AuthorID != userID {
			hidden[c.AuthorID]++
			continue
		}
		cards = append(cards, c)
	}
	return cards, hidden, rows.Err()
}

func (s *Server) listRetros(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	rows, err := s.db.Query(r.Context(), `SELECT r.id, r.sprint_id, s.number, r.status, r.created_at,
		(SELECT count(*) FROM retro_cards c WHERE c.retro_id = r.id)::int,
		(SELECT count(*) FROM retro_cards c WHERE c.retro_id = r.id AND c.col = 'action')::int
		FROM retros r JOIN sprints s ON s.id = r.sprint_id WHERE r.household_id = $1 ORDER BY s.number DESC LIMIT 30`, u.HouseholdID)
	if err != nil {
		return err
	}
	type item struct {
		ID           string    `json:"id"`
		SprintID     string    `json:"sprintId"`
		SprintNumber int       `json:"sprintNumber"`
		Status       string    `json:"status"`
		CreatedAt    time.Time `json:"createdAt"`
		Cards        int       `json:"cards"`
		Actions      int       `json:"actions"`
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (item, error) {
		var i item
		err := row.Scan(&i.ID, &i.SprintID, &i.SprintNumber, &i.Status, &i.CreatedAt, &i.Cards, &i.Actions)
		return i, err
	})
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (s *Server) addRetroCard(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	retro, err := s.loadRetro(ctx, u.HouseholdID, chi.URLParam(r, "id"))
	if err != nil {
		return err
	}
	if retro.Status == "closed" {
		return errBadRequest("This retro is closed")
	}
	var in struct {
		Col        string  `json:"col"`
		Text       string  `json:"text"`
		AssigneeID *string `json:"assigneeId"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Text = strings.TrimSpace(in.Text)
	if !slices.Contains(retroCols, in.Col) {
		return errBadRequest("Unknown column")
	}
	if in.Text == "" || len(in.Text) > 500 {
		return errBadRequest("Write something (max 500 characters)")
	}
	if err := s.checkMember(ctx, u.HouseholdID, in.AssigneeID); err != nil {
		return err
	}
	var id string
	if err := s.db.QueryRow(ctx, `INSERT INTO retro_cards (retro_id, author_id, col, text, assignee_id) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		retro.ID, u.ID, in.Col, in.Text, in.AssigneeID).Scan(&id); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventRetro.from(r))
	return ok(w, map[string]string{"id": id})
}

func (s *Server) checkMember(ctx context.Context, householdID string, id *string) error {
	if id == nil || *id == "" {
		return nil
	}
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM members WHERE id = $1 AND household_id = $2)`, *id, householdID).Scan(&exists); err != nil || !exists {
		return errBadRequest("Unknown member")
	}
	return nil
}

// cardForUpdate loads a card with its retro, scoped to the household.
func (s *Server) cardForUpdate(ctx context.Context, householdID, cardID string) (*RetroCard, *Retro, error) {
	var c RetroCard
	var retroID string
	err := s.db.QueryRow(ctx, `SELECT c.id, c.col, c.text, c.author_id, c.assignee_id, c.task_id, c.retro_id
		FROM retro_cards c JOIN retros r ON r.id = c.retro_id WHERE c.id = $1 AND r.household_id = $2`, cardID, householdID).
		Scan(&c.ID, &c.Col, &c.Text, &c.AuthorID, &c.AssigneeID, &c.TaskID, &retroID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, errNotFound("Card not found")
	}
	if err != nil {
		return nil, nil, err
	}
	retro, err := s.loadRetro(ctx, householdID, retroID)
	return &c, retro, err
}

func (s *Server) updateRetroCard(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	c, retro, err := s.cardForUpdate(ctx, u.HouseholdID, chi.URLParam(r, "id"))
	if err != nil {
		return err
	}
	if retro.Status == "closed" {
		return errBadRequest("This retro is closed")
	}
	var in struct {
		Text       *string        `json:"text"`
		Col        *string        `json:"col"`
		AssigneeID optionalString `json:"assigneeId"`
	}
	in.AssigneeID.v = c.AssigneeID
	if err := decode(r, &in); err != nil {
		return err
	}
	// Before the reveal nobody can see (or touch) the other person's cards.
	if c.AuthorID != u.ID && (retro.Status == "collecting" || in.Text != nil || in.Col != nil) {
		return errForbidden("Only the author can edit this card")
	}
	if in.Text != nil {
		t := strings.TrimSpace(*in.Text)
		if t == "" || len(t) > 500 {
			return errBadRequest("Write something (max 500 characters)")
		}
		c.Text = t
	}
	if in.Col != nil {
		if !slices.Contains(retroCols, *in.Col) {
			return errBadRequest("Unknown column")
		}
		c.Col = *in.Col
	}
	if err := s.checkMember(ctx, u.HouseholdID, in.AssigneeID.v); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `UPDATE retro_cards SET text = $2, col = $3, assignee_id = $4 WHERE id = $1`, c.ID, c.Text, c.Col, in.AssigneeID.v); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventRetro.from(r))
	return noContent(w)
}

func (s *Server) deleteRetroCard(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	c, retro, err := s.cardForUpdate(r.Context(), u.HouseholdID, chi.URLParam(r, "id"))
	if err != nil {
		return err
	}
	if c.AuthorID != u.ID {
		return errForbidden("Only the author can delete this card")
	}
	if retro.Status == "closed" {
		return errBadRequest("This retro is closed")
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM retro_cards WHERE id = $1`, c.ID); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventRetro.from(r))
	return noContent(w)
}

func (s *Server) voteRetroCard(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	c, retro, err := s.cardForUpdate(ctx, u.HouseholdID, chi.URLParam(r, "id"))
	if err != nil {
		return err
	}
	if retro.Status != "revealed" {
		return errBadRequest("Voting opens once the cards are revealed")
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM retro_votes WHERE card_id = $1 AND user_id = $2`, c.ID, u.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := s.db.Exec(ctx, `INSERT INTO retro_votes (card_id, user_id) VALUES ($1, $2)`, c.ID, u.ID); err != nil {
			return err
		}
	}
	s.hub.Publish(u.HouseholdID, eventRetro.from(r))
	return noContent(w)
}

func (s *Server) revealRetro(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	tag, err := s.db.Exec(ctx, `UPDATE retros SET status = 'revealed', revealed_at = now() WHERE id = $1 AND household_id = $2 AND status = 'collecting'`,
		chi.URLParam(r, "id"), u.HouseholdID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errBadRequest("Already revealed")
	}
	s.hub.Publish(u.HouseholdID, eventRetro.from(r))
	s.notifyPartners(ctx, u, notification{
		Kind: "ceremony", DedupeKey: "retro-reveal:" + chi.URLParam(r, "id"),
		Title: u.DisplayName + " revealed the retro", Body: "See each other's cards and vote on what matters most.", URL: "/retro",
	})
	return noContent(w)
}

// closeRetro turns every action item into a task in the next sprint, then locks the retro.
func (s *Server) closeRetro(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	retro, err := s.loadRetro(ctx, u.HouseholdID, chi.URLParam(r, "id"))
	if err != nil {
		return err
	}
	if retro.Status != "revealed" {
		return errBadRequest("Reveal the cards before closing the retro")
	}
	h, err := s.getHousehold(ctx, u.HouseholdID)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockHousehold(ctx, tx, h.ID); err != nil {
		return err
	}
	var number int
	var endsOn time.Time
	if err := tx.QueryRow(ctx, `SELECT number, ends_on FROM sprints WHERE id = $1`, retro.SprintID).Scan(&number, &endsOn); err != nil {
		return err
	}
	next, err := nextSprintAfter(ctx, tx, h, endsOn)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT c.id, c.text, c.assignee_id, (SELECT count(*) FROM retro_votes v WHERE v.card_id = c.id)
		FROM retro_cards c WHERE c.retro_id = $1 AND c.col = 'action' AND c.task_id IS NULL ORDER BY c.created_at`, retro.ID)
	if err != nil {
		return err
	}
	type action struct {
		id, text string
		assignee *string
		votes    int
	}
	var actions []action
	for rows.Next() {
		var a action
		if err := rows.Scan(&a.id, &a.text, &a.assignee, &a.votes); err != nil {
			rows.Close()
			return err
		}
		actions = append(actions, a)
	}
	rows.Close()
	var rank float64
	if err := tx.QueryRow(ctx, `SELECT coalesce(min(rank), 1024) - 1024 FROM tasks WHERE household_id = $1`, h.ID).Scan(&rank); err != nil {
		return err
	}
	for i, a := range actions {
		title := a.text
		if len(title) > 200 {
			title = title[:200]
		}
		// Voted-for actions start as high priority.
		prio := 2
		if a.votes >= 2 {
			prio = 3
		}
		t := Task{Title: title, Description: fmt.Sprintf("Action item from the Sprint %d retro.", number), Status: "todo", Priority: prio,
			Labels: []string{}, AssigneeID: a.assignee, ReporterID: &u.ID, SprintID: &next, Rank: rank - float64(len(actions)-i)}
		if err := s.insertTask(ctx, tx, h.ID, &t, u.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE retro_cards SET task_id = $2 WHERE id = $1`, a.id, t.ID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE retros SET status = 'closed', closed_at = now() WHERE id = $1`, retro.ID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventRetro.from(r))
	s.hub.Publish(u.HouseholdID, eventTasks.from(r))
	return ok(w, map[string]int{"tasksCreated": len(actions)})
}

// notifyPartners notifies every household member except the actor.
func (s *Server) notifyPartners(ctx context.Context, actor *User, n notification) {
	rows, err := s.db.Query(ctx, `SELECT id FROM members WHERE household_id = $1 AND id <> $2`, actor.HouseholdID, actor.ID)
	if err != nil {
		logErr("notify partners", err)
		return
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		logErr("notify partners", err)
		return
	}
	n.HouseholdID = actor.HouseholdID
	for _, id := range ids {
		n.UserID = id
		logErr("notify partner", s.notify(ctx, n))
	}
}
