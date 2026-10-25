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

// FridgeNote is a sticky note on the household's home screen.
type FridgeNote struct {
	ID        string    `json:"id" db:"id"`
	AuthorID  *string   `json:"authorId" db:"author_id"`
	Text      string    `json:"text" db:"text"`
	Color     string    `json:"color" db:"color"`
	Pinned    bool      `json:"pinned" db:"pinned"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}

const noteColumns = `id, author_id, text, color, pinned, created_at, updated_at`

var noteColors = []string{"yellow", "pink", "blue", "green", "orange"}

func (s *Server) listNotes(w http.ResponseWriter, r *http.Request) error {
	rows, _ := s.db.Query(r.Context(), `SELECT `+noteColumns+` FROM fridge_notes WHERE household_id = $1 ORDER BY pinned DESC, created_at DESC`,
		currentUser(r).HouseholdID)
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

func noteText(t string) (string, error) {
	t = strings.TrimSpace(t)
	if t == "" {
		return "", errBadRequest("Write something on the note")
	}
	if len([]rune(t)) > 500 {
		return "", errBadRequest("Keep the note under 500 characters")
	}
	return t, nil
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
		ID     string `json:"id"`
		Text   string `json:"text"`
		Color  string `json:"color"`
		Pinned bool   `json:"pinned"`
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
	if in.Color == "" {
		in.Color = "yellow"
	}
	if !slices.Contains(noteColors, in.Color) {
		return errBadRequest("Unknown colour")
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO fridge_notes (id, household_id, author_id, text, color, pinned) VALUES ($1, $2, $3, $4, $5, $6)`,
		in.ID, u.HouseholdID, u.ID, text, in.Color, in.Pinned); err != nil {
		return err
	}
	note, err := s.getNote(ctx, u.HouseholdID, in.ID)
	if err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventNotes.from(r))
	s.notifyPartners(ctx, u, notification{
		Kind: "notes", DedupeKey: "note:" + note.ID,
		Title: u.DisplayName + " left a note on the fridge", Body: notePreview(text, 140), URL: "/",
	})
	return writeJSON(w, http.StatusCreated, note)
}

func (s *Server) updateNote(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		Text   *string `json:"text"`
		Color  *string `json:"color"`
		Pinned *bool   `json:"pinned"`
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
	if in.Color != nil {
		if !slices.Contains(noteColors, *in.Color) {
			return errBadRequest("Unknown colour")
		}
		note.Color = *in.Color
	}
	if in.Pinned != nil {
		note.Pinned = *in.Pinned
	}
	if err := s.db.QueryRow(ctx, `UPDATE fridge_notes SET text = $2, color = $3, pinned = $4, updated_at = now() WHERE id = $1 RETURNING updated_at`,
		note.ID, note.Text, note.Color, note.Pinned).Scan(&note.UpdatedAt); err != nil {
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
