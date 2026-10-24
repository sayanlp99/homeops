package api

import (
	"embed"
	"encoding/json"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
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
	ctx := r.Context()
	packs, err := loadPacks()
	if err != nil {
		return err
	}
	var pack *recipePack
	for i := range packs {
		if packs[i].Name == chi.URLParam(r, "name") {
			pack = &packs[i]
		}
	}
	if pack == nil {
		return errNotFound("Unknown recipe pack")
	}
	existing := map[string]bool{}
	rows, err := s.db.Query(ctx, `SELECT lower(title) FROM recipes WHERE household_id = $1`, u.HouseholdID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var t string
		rows.Scan(&t)
		existing[t] = true
	}
	rows.Close()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	added, skipped := 0, []string{}
	for _, rc := range pack.Recipes {
		if existing[strings.ToLower(strings.TrimSpace(rc.Title))] {
			skipped = append(skipped, rc.Title)
			continue
		}
		if err := rc.validate(); err != nil {
			return err
		}
		if _, err := writeRecipe(ctx, tx, u, "", &rc); err != nil {
			return err
		}
		added++
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventRecipes.from(r))
	return ok(w, map[string]any{"added": added, "skipped": skipped})
}
