package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// FridgeNote is a sticky note on the household's home screen.
type FridgeNote struct {
	ID        string     `json:"id" db:"id"`
	AuthorID  *string    `json:"authorId" db:"author_id"`
	Text      string     `json:"text" db:"text"`
	Color     string     `json:"color" db:"color"`
	Pinned    bool       `json:"pinned" db:"pinned"`
	ExpiresOn *string    `json:"expiresOn" db:"expires_on"` // shown through this day, then removed
	Items     []NoteItem `json:"items" db:"items"`          // a checklist; Text is then its title
	CreatedAt time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time  `json:"updatedAt" db:"updated_at"`
}

// NoteItem is one line of a checklist note.
type NoteItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

const noteColumns = `id, author_id, text, color, pinned, expires_on::text, items, created_at, updated_at`

const maxNoteItems = 30

var noteColors = []string{"yellow", "pink", "blue", "green", "orange"}

func (s *Server) listNotes(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	today, err := s.householdToday(r.Context(), u.HouseholdID)
	if err != nil {
		return err
	}
	// Expired notes are hidden at once; the scheduler deletes them shortly after.
	rows, _ := s.db.Query(r.Context(), `SELECT `+noteColumns+` FROM fridge_notes
		WHERE household_id = $1 AND (expires_on IS NULL OR expires_on >= $2::date) ORDER BY pinned DESC, created_at DESC`,
		u.HouseholdID, today)
	notes, err := pgx.CollectRows(rows, pgx.RowToStructByName[FridgeNote])
	if err != nil {
		return err
	}
	return ok(w, notes)
}

func (s *Server) getNote(ctx context.Context, householdID, id string) (*FridgeNote, error) {
	rows, _ := s.db.Query(ctx, `SELECT `+noteColumns+` FROM fridge_notes WHERE id = $1 AND household_id = $2`, id, householdID)
	return pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByName[FridgeNote])
}

func (s *Server) householdToday(ctx context.Context, householdID string) (string, error) {
	h, err := s.getHousehold(ctx, householdID)
	if err != nil {
		return "", err
	}
	return localToday(h, time.Now()).Format(time.DateOnly), nil
}

// noteExpiry checks an optional expiry date: a real date, not already past.
func (s *Server) noteExpiry(ctx context.Context, householdID string, raw json.RawMessage) (*string, error) {
	var d *string
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, errBadRequest("Dates must be YYYY-MM-DD")
	}
	if d == nil || *d == "" {
		return nil, nil
	}
	t, err := parseDate(*d)
	if err != nil {
		return nil, err
	}
	today, err := s.householdToday(ctx, householdID)
	if err != nil {
		return nil, err
	}
	day := t.Format(time.DateOnly)
	if day < today {
		return nil, errBadRequest("The expiry date has already passed")
	}
	return &day, nil
}

// removeExpiredNotes deletes notes whose last day was before today.
func (s *Server) removeExpiredNotes(ctx context.Context, h *Household, today string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM fridge_notes WHERE household_id = $1 AND expires_on < $2::date`, h.ID, today)
	if err == nil && tag.RowsAffected() > 0 {
		s.hub.Publish(h.ID, eventNotes.from(nil))
	}
	return err
}

func noteText(t string) (string, error) {
	t = strings.TrimSpace(t)
	if len([]rune(t)) > 500 {
		return "", errBadRequest("Keep the note under 500 characters")
	}
	return t, nil
}

// noteItems tidies a checklist: blank lines dropped, ids kept (new ones assigned).
func noteItems(in []NoteItem) ([]NoteItem, error) {
	out := []NoteItem{}
	seen := map[string]bool{}
	for _, it := range in {
		it.Text = strings.TrimSpace(it.Text)
		if it.Text == "" {
			continue
		}
		if len([]rune(it.Text)) > 200 {
			return nil, errBadRequest("Keep each checklist item under 200 characters")
		}
		if it.ID == "" || len(it.ID) > 64 || seen[it.ID] {
			it.ID = randomUUID()
		}
		seen[it.ID] = true
		out = append(out, it)
	}
	if len(out) > maxNoteItems {
		return nil, errBadRequest(fmt.Sprintf("A checklist holds up to %d items", maxNoteItems))
	}
	return out, nil
}

func checkNoteFilled(text string, items []NoteItem) error {
	if text == "" && len(items) == 0 {
		return errBadRequest("Write something on the note")
	}
	return nil
}

// noteSummary is what a notification says: the text, or the checklist's items.
func noteSummary(text string, items []NoteItem) string {
	if len(items) == 0 {
		return text
	}
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.Text
	}
	list := strings.Join(names, ", ")
	if text == "" {
		return list
	}
	return text + ": " + list
}

// notePreview shortens a note for a notification without splitting a character.
func notePreview(t string, n int) string {
	if r := []rune(t); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return t
}

func (s *Server) createNote(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		ID        string          `json:"id"`
		Text      string          `json:"text"`
		Color     string          `json:"color"`
		Pinned    bool            `json:"pinned"`
		ExpiresOn json.RawMessage `json:"expiresOn"`
		Items     []NoteItem      `json:"items"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.ID == "" {
		in.ID = randomUUID()
	} else if existing, err := s.getNote(ctx, u.HouseholdID, in.ID); err == nil {
		return ok(w, existing) // offline retry
	}
	text, err := noteText(in.Text)
	if err != nil {
		return err
	}
	items, err := noteItems(in.Items)
	if err != nil {
		return err
	}
	if err := checkNoteFilled(text, items); err != nil {
		return err
	}
	if in.Color == "" {
		in.Color = "yellow"
	}
	if !slices.Contains(noteColors, in.Color) {
		return errBadRequest("Unknown colour")
	}
	expires, err := s.noteExpiry(ctx, u.HouseholdID, in.ExpiresOn)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO fridge_notes (id, household_id, author_id, text, color, pinned, expires_on, items) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		in.ID, u.HouseholdID, u.ID, text, in.Color, in.Pinned, expires, items); err != nil {
		return err
	}
	note, err := s.getNote(ctx, u.HouseholdID, in.ID)
	if err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventNotes.from(r))
	s.notifyPartners(ctx, u, notification{
		Kind: "notes", DedupeKey: "note:" + note.ID,
		Title: u.DisplayName + " left a note on the fridge", Body: notePreview(noteSummary(text, items), 140), URL: "/",
	})
	return writeJSON(w, http.StatusCreated, note)
}

func (s *Server) updateNote(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		Text      *string         `json:"text"`
		Color     *string         `json:"color"`
		Pinned    *bool           `json:"pinned"`
		ExpiresOn json.RawMessage `json:"expiresOn"` // null clears it
		Items     *[]NoteItem     `json:"items"`     // replaces the checklist; [] makes it a plain note
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	note, err := s.getNote(ctx, u.HouseholdID, chi.URLParam(r, "id"))
	if err != nil {
		return err
	}
	if in.Text != nil {
		if note.Text, err = noteText(*in.Text); err != nil {
			return err
		}
	}
	if in.Items != nil {
		if note.Items, err = noteItems(*in.Items); err != nil {
			return err
		}
	}
	if err := checkNoteFilled(note.Text, note.Items); err != nil {
		return err
	}
	if in.Color != nil {
		if !slices.Contains(noteColors, *in.Color) {
			return errBadRequest("Unknown colour")
		}
		note.Color = *in.Color
	}
	if in.Pinned != nil {
		note.Pinned = *in.Pinned
	}
	if len(in.ExpiresOn) > 0 {
		if note.ExpiresOn, err = s.noteExpiry(ctx, u.HouseholdID, in.ExpiresOn); err != nil {
			return err
		}
	}
	if err := s.db.QueryRow(ctx, `UPDATE fridge_notes SET text = $2, color = $3, pinned = $4, expires_on = $5, items = $6, updated_at = now() WHERE id = $1 RETURNING updated_at`,
		note.ID, note.Text, note.Color, note.Pinned, note.ExpiresOn, note.Items).Scan(&note.UpdatedAt); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventNotes.from(r))
	return ok(w, note)
}

// tickNoteItem ticks or unticks one checklist item in place, so two people ticking
// different items at once don't overwrite each other.
func (s *Server) tickNoteItem(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		Done bool `json:"done"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	rows, _ := s.db.Query(ctx, `UPDATE fridge_notes SET updated_at = now(), items = (
			SELECT jsonb_agg(CASE WHEN e->>'id' = $3 THEN jsonb_set(e, '{done}', to_jsonb($4::boolean)) ELSE e END ORDER BY n)
			FROM jsonb_array_elements(items) WITH ORDINALITY AS t(e, n))
		WHERE id = $1 AND household_id = $2 AND items @> jsonb_build_array(jsonb_build_object('id', $3::text))
		RETURNING `+noteColumns, chi.URLParam(r, "id"), u.HouseholdID, chi.URLParam(r, "item"), in.Done)
	note, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByName[FridgeNote])
	if err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventNotes.from(r))
	return ok(w, note)
}

func (s *Server) deleteNote(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	if _, err := s.db.Exec(r.Context(), `DELETE FROM fridge_notes WHERE id = $1 AND household_id = $2`, chi.URLParam(r, "id"), u.HouseholdID); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventNotes.from(r))
	return noContent(w)
}
