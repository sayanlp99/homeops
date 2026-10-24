package api

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Recipe packs are ready-made recipe sets a household can add in one go.

//go:embed packs/*.json
var packFiles embed.FS

type recipePack struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Recipes     []Recipe `json:"recipes"`
}

func loadPacks() ([]recipePack, error) {
	entries, err := packFiles.ReadDir("packs")
	if err != nil {
		return nil, err
	}
	var out []recipePack
	for _, e := range entries {
		b, err := packFiles.ReadFile(path.Join("packs", e.Name()))
		if err != nil {
			return nil, err
		}
		var p recipePack
		if err := json.Unmarshal(b, &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Server) listPacks(w http.ResponseWriter, r *http.Request) error {
	packs, err := loadPacks()
	if err != nil {
		return err
	}
	type packOut struct {
		Name        string   `json:"name"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Count       int      `json:"count"`
		Titles      []string `json:"titles"`
	}
	out := []packOut{}
	for _, p := range packs {
		o := packOut{Name: p.Name, Title: p.Title, Description: p.Description, Count: len(p.Recipes)}
		for _, rc := range p.Recipes {
			o.Titles = append(o.Titles, rc.Title)
		}
		out = append(out, o)
	}
	return ok(w, out)
}

// importPack adds a pack's recipes, skipping any whose title the household already has.
func (s *Server) importPack(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	added, skipped, err := importPack(r.Context(), s.db, u, chi.URLParam(r, "name"))
	if err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventRecipes.from(r))
	return ok(w, map[string]any{"added": added, "skipped": skipped})
}

func importPack(ctx context.Context, db *pgxpool.Pool, u *User, name string) (int, []string, error) {
	packs, err := loadPacks()
	if err != nil {
		return 0, nil, err
	}
	var pack *recipePack
	for i := range packs {
		if packs[i].Name == name {
			pack = &packs[i]
		}
	}
	if pack == nil {
		return 0, nil, errNotFound("Unknown recipe pack")
	}
	existing := map[string]bool{}
	rows, err := db.Query(ctx, `SELECT lower(title) FROM recipes WHERE household_id = $1`, u.HouseholdID)
	if err != nil {
		return 0, nil, err
	}
	for rows.Next() {
		var t string
		rows.Scan(&t)
		existing[t] = true
	}
	rows.Close()

	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback(ctx)
	added, skipped := 0, []string{}
	for _, rc := range pack.Recipes {
		if existing[strings.ToLower(strings.TrimSpace(rc.Title))] {
			skipped = append(skipped, rc.Title)
			continue
		}
		if err := rc.validate(); err != nil {
			return 0, nil, err
		}
		if _, err := writeRecipe(ctx, tx, u, "", &rc); err != nil {
			return 0, nil, err
		}
		added++
	}
	return added, skipped, tx.Commit(ctx)
}

// ImportPack adds a recipe pack to a user's household from the command line, as that
// user. Someone in several households must name one.
func ImportPack(ctx context.Context, db *pgxpool.Pool, username, household, name string) (int, []string, error) {
	var u User
	rows, err := db.Query(ctx, `SELECT u.id, m.household_id, h.name FROM users u
		JOIN memberships m ON m.user_id = u.id JOIN households h ON h.id = m.household_id
		WHERE (u.username = $1 OR lower(u.email) = $1) AND m.role <> 'viewer'`, strings.ToLower(strings.TrimSpace(username)))
	if err != nil {
		return 0, nil, err
	}
	n := 0
	for rows.Next() {
		var id, hid, hname string
		if err := rows.Scan(&id, &hid, &hname); err != nil {
			return 0, nil, err
		}
		if household == "" || household == hid || strings.EqualFold(household, hname) {
			u.ID, u.HouseholdID = id, hid
			n++
		}
	}
	rows.Close()
	switch {
	case n == 0:
		return 0, nil, fmt.Errorf("no household where %q can add recipes", username)
	case n > 1:
		return 0, nil, fmt.Errorf("%q is in several households; name one", username)
	}
	return importPack(ctx, db, &u, name)
}
