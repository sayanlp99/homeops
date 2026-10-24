package api

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"homeops/internal/config"
)

// RefreshPhotos brings photos saved before the 2048px/640px sizes up to date. The
// originals weren't kept, so a photo can only get sharper where a bigger copy still
// exists: a recipe imported from a link has its photo fetched again, but only when the
// source is larger and is the same picture (so a photo someone put on an imported recipe
// themselves is never replaced). Every other photo keeps its full image and gets a
// sharper 640px thumbnail. Refreshed photos get a new id, because browsers cache a
// photo forever under its id. With dryRun nothing is written.
func RefreshPhotos(ctx context.Context, cfg config.Config, db *pgxpool.Pool, dryRun bool, logf func(string, ...any)) error {
	s, err := New(ctx, cfg, db)
	if err != nil {
		return err
	}
	type photoRow struct{ table, column, id, title, photo, source string }
	var list []photoRow
	rows, err := db.Query(ctx, `
		SELECT 'recipes', 'photo', id::text, title, photo, source_url FROM recipes WHERE photo <> '' AND photo NOT LIKE '%.pdf'
		UNION ALL
		SELECT 'money_transactions', 'receipt', id::text, payee, receipt, '' FROM money_transactions WHERE receipt <> '' AND receipt NOT LIKE '%.pdf'`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p photoRow
		if err := rows.Scan(&p.table, &p.column, &p.id, &p.title, &p.photo, &p.source); err != nil {
			return err
		}
		list = append(list, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	var sharper, thumbs, current, failed int
	for _, p := range list {
		main, err := s.readFile(ctx, photoKey(p.photo, false))
		if err != nil {
			logf("✗ %s %q: can't read the photo: %v", p.table, p.title, err)
			failed++
			continue
		}
		cur, _, err := image.Decode(bytes.NewReader(main))
		if err != nil {
			logf("✗ %s %q: can't decode the photo: %v", p.table, p.title, err)
			failed++
			continue
		}
		var fresh []byte // a bigger copy of the same picture, from the recipe's source
		if p.source != "" {
			if b, why := s.sourcePhoto(ctx, p.source, cur); b != nil {
				fresh = b
			} else if why != "" {
				logf("  %q: keeping the saved photo (%s)", p.title, why)
			}
		}
		if fresh == nil && thumbCurrent(ctx, s, p.photo, cur) {
			current++
			continue
		}
		how := "sharper thumbnail"
		if fresh != nil {
			how = "full size again from " + hostOf(p.source)
		}
		if dryRun {
			logf("would refresh %s %q: %s", p.table, p.title, how)
			continue
		}
		var newID string
		if fresh != nil {
			newID, err = s.storeImage(ctx, bytes.NewReader(fresh))
		} else {
			newID, err = s.rethumb(ctx, main, cur)
		}
		if err != nil {
			logf("✗ %s %q: %v", p.table, p.title, err)
			failed++
			continue
		}
		tag, err := db.Exec(ctx, fmt.Sprintf(`UPDATE %s SET %s = $3 WHERE id = $1 AND %s = $2`, p.table, p.column, p.column), p.id, p.photo, newID)
		if err != nil || tag.RowsAffected() == 0 {
			s.deletePhoto(ctx, newID) // changed meanwhile; leave it be
			logf("✗ %s %q: not updated (%v)", p.table, p.title, err)
			failed++
			continue
		}
		var refs int
		db.QueryRow(ctx, `SELECT (SELECT count(*) FROM recipes WHERE photo = $1) + (SELECT count(*) FROM money_transactions WHERE receipt = $1)`, p.photo).Scan(&refs)
		if refs == 0 {
			s.deletePhoto(ctx, p.photo)
		}
		if fresh != nil {
			sharper++
		} else {
			thumbs++
		}
		logf("✔ %s %q: %s", p.table, p.title, how)
	}
	logf("%d photos: %d sharper from their source, %d with a sharper thumbnail, %d already current, %d failed", len(list), sharper, thumbs, current, failed)
	if failed > 0 {
		return fmt.Errorf("%d photos couldn't be refreshed", failed)
	}
	return nil
}

func (s *Server) readFile(ctx context.Context, key string) ([]byte, error) {
	f, err := s.files.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxUploadBytes))
}

func (s *Server) deletePhoto(ctx context.Context, id string) {
	s.files.Delete(ctx, photoKey(id, false))
	s.files.Delete(ctx, photoKey(id, true))
}

// rethumb keeps the full image byte for byte under a new id and adds a 640px thumbnail.
func (s *Server) rethumb(ctx context.Context, main []byte, img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resize(img, 640), &jpeg.Options{Quality: 80}); err != nil {
		return "", err
	}
	id := uuid.NewString()
	if err := s.files.Put(ctx, photoKey(id, false), main, "image/jpeg"); err != nil {
		return "", err
	}
	if err := s.files.Put(ctx, photoKey(id, true), buf.Bytes(), "image/jpeg"); err != nil {
		s.files.Delete(ctx, photoKey(id, false))
		return "", err
	}
	return id, nil
}

// thumbCurrent: the thumbnail is already as big as it can be (640px, or the photo itself
// is no bigger than the old 480px thumbnail).
func thumbCurrent(ctx context.Context, s *Server, id string, main image.Image) bool {
	b, err := s.readFile(ctx, photoKey(id, true))
	if err != nil {
		return false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return false
	}
	edge := max(cfg.Width, cfg.Height)
	mb := main.Bounds()
	return edge >= 640 || edge >= max(mb.Dx(), mb.Dy())
}

// sourcePhoto fetches the recipe page's photo again. It's used only when it's bigger
// than the saved one and looks like the same picture; otherwise it says why not ("" when
// there's simply nothing better).
func (s *Server) sourcePhoto(ctx context.Context, source string, saved image.Image) ([]byte, string) {
	u, err := url.Parse(source)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, ""
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	page, finalURL, err := fetch(ctx, u.String(), 5<<20)
	if err != nil {
		return nil, "source page unavailable"
	}
	final, _ := url.Parse(finalURL)
	rec := parseRecipePage(string(page), final)
	if rec.Photo == "" {
		return nil, "the source page has no photo"
	}
	imgURL, err := final.Parse(rec.Photo)
	if err != nil {
		return nil, "bad photo link on the source page"
	}
	data, _, err := fetch(ctx, imgURL.String(), maxUploadBytes)
	if err != nil {
		return nil, "source photo unavailable"
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "source photo unreadable"
	}
	sb, cb := saved.Bounds(), img.Bounds()
	if max(cb.Dx(), cb.Dy()) <= max(sb.Dx(), sb.Dy()) {
		return nil, "" // the saved photo is already the source's full size
	}
	if !samePicture(saved, img) {
		return nil, "the source photo is a different picture"
	}
	return data, ""
}

// samePicture compares two images at 16×16 grey: a photo re-saved or resized matches
// closely; a different photo doesn't.
func samePicture(a, b image.Image) bool {
	if math.Abs(float64(a.Bounds().Dx())/float64(a.Bounds().Dy())-float64(b.Bounds().Dx())/float64(b.Bounds().Dy())) > 0.03 {
		return false
	}
	ga, gb := tinyGrey(a), tinyGrey(b)
	if len(ga) != len(gb) {
		return false
	}
	diff := 0.0
	for i := range ga {
		diff += math.Abs(ga[i] - gb[i])
	}
	return diff/float64(len(ga)) < 12 // mean difference out of 255
}

func tinyGrey(img image.Image) []float64 {
	small := resize(img, 16)
	b := small.Bounds()
	out := make([]float64, 0, b.Dx()*b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := small.At(x, y).RGBA()
			out = append(out, (0.299*float64(r)+0.587*float64(g)+0.114*float64(bl))/257)
		}
	}
	return out
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return strings.TrimPrefix(u.Host, "www.")
	}
	return "the source"
}
