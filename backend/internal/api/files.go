package api

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"homeops/internal/blob"
)

const maxUploadBytes = 15 << 20

var fileIDRe = regexp.MustCompile(`^[0-9a-f-]{36}$`)

func photoKey(id string, thumb bool) string {
	if thumb {
		return "recipes/" + id + "_thumb.jpg"
	}
	return "recipes/" + id + ".jpg"
}

// storeImage decodes any JPEG/PNG/WebP, then stores a ≤1600px JPEG and a 480px thumbnail
// in object storage. Re-encoding strips EXIF (including phone GPS data) and keeps
// camera photos small.
func (s *Server) storeImage(ctx context.Context, r io.Reader) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxUploadBytes+1))
	if err != nil {
		return "", err
	}
	if len(raw) > maxUploadBytes {
		return "", errBadRequest("Image is too large (max 15 MB)")
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", errBadRequest("Unsupported image. Use JPEG, PNG or WebP.")
	}
	id := uuid.NewString()
	for _, v := range []struct {
		thumb bool
		max   int
	}{{false, 1600}, {true, 480}} {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, resize(img, v.max), &jpeg.Options{Quality: 82}); err != nil {
			return "", err
		}
		if err := s.files.Put(ctx, photoKey(id, v.thumb), buf.Bytes(), "image/jpeg"); err != nil {
			return "", err
		}
	}
	return id, nil
}

func resize(img image.Image, max int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return img
	}
	if w >= h {
		h = h * max / w
		w = max
	} else {
		w = w * max / h
		h = max
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

func (s *Server) uploadFile(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1<<20)
	file, _, err := r.FormFile("file")
	if err != nil {
		return errBadRequest("No file uploaded")
	}
	defer file.Close()
	id, err := s.storeImage(r.Context(), file)
	if err != nil {
		return err
	}
	return ok(w, map[string]string{"id": id})
}

// serveFile streams an uploaded image through the app, so access stays behind sign-in
// and the bucket is never exposed. IDs never change, so browsers cache them forever.
func (s *Server) serveFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !fileIDRe.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	thumb := r.URL.Query().Get("thumb") == "1"
	f, err := s.files.Get(r.Context(), photoKey(id, thumb))
	if errors.Is(err, blob.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		logErr("serve file", err, "id", id)
		http.Error(w, "storage unavailable", http.StatusBadGateway)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(w, r, id+".jpg", time.Time{}, f)
}

// pruneOrphanFiles removes photos no recipe references, e.g. from an import that was
// never saved. Only objects older than a day are touched.
func (s *Server) pruneOrphanFiles(ctx context.Context) error {
	objs, err := s.files.List(ctx, "recipes/")
	if err != nil {
		return err
	}
	rows, err := s.db.Query(ctx, `SELECT photo FROM recipes WHERE photo <> ''`)
	if err != nil {
		return err
	}
	used := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return err
		}
		used[p] = true
	}
	rows.Close()
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, o := range objs {
		id := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(o.Key, "recipes/"), ".jpg"), "_thumb")
		if used[id] || o.Modified.After(cutoff) {
			continue
		}
		logErr("delete orphan file", s.files.Delete(ctx, o.Key), "key", o.Key)
	}
	return nil
}
