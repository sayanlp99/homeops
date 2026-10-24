package api

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func testPhoto(w, h int, shade uint8) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x * 255 / w), shade, uint8(y * 255 / h), 255})
		}
	}
	return img
}

func TestSamePicture(t *testing.T) {
	a := testPhoto(1600, 1200, 40)
	if !samePicture(a, resize(testPhoto(2400, 1800, 40), 2400)) {
		t.Error("a bigger copy of the same photo should match")
	}
	if samePicture(a, testPhoto(1600, 1200, 200)) {
		t.Error("a different photo matched")
	}
	if samePicture(a, testPhoto(1200, 1200, 40)) {
		t.Error("a different shape matched")
	}
}

func TestRefreshPhotos(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	s, ts, _ := startTestServer(t, 54335)
	c := newClient(t, ts.URL)
	c.do("POST", "/api/setup", map[string]any{"householdName": "Home", "email": "a@example.com", "username": "asha", "displayName": "Asha", "password": "hunter2hunter2"}, 204, nil)

	// A photo saved the old way: 1600px main, 480px thumbnail.
	ctx := t.Context()
	old := "11111111-1111-4111-8111-111111111111"
	enc := func(img image.Image) []byte {
		var b bytes.Buffer
		jpeg.Encode(&b, img, &jpeg.Options{Quality: 82})
		return b.Bytes()
	}
	main := enc(testPhoto(1600, 1200, 40))
	s.files.Put(ctx, photoKey(old, false), main, "image/jpeg")
	s.files.Put(ctx, photoKey(old, true), enc(resize(testPhoto(1600, 1200, 40), 480)), "image/jpeg")
	var r Recipe
	c.do("POST", "/api/recipes", map[string]any{"title": "Shukto", "diet": "veg", "servings": 2, "photo": old}, 200, &r)

	var log []string
	logf := func(f string, a ...any) { log = append(log, f) }
	if err := RefreshPhotos(ctx, s.cfg, s.db, true, logf); err != nil {
		t.Fatal(err)
	}
	c.do("GET", "/api/recipes/"+r.ID, nil, 200, &r)
	if r.Photo != old {
		t.Fatal("a dry run changed the photo")
	}
	if err := RefreshPhotos(ctx, s.cfg, s.db, false, logf); err != nil {
		t.Fatal(err)
	}
	c.do("GET", "/api/recipes/"+r.ID, nil, 200, &r)
	if r.Photo == old || r.Photo == "" {
		t.Fatalf("photo not refreshed: %q", r.Photo)
	}
	got, err := s.readFile(ctx, photoKey(r.Photo, false))
	if err != nil || !bytes.Equal(got, main) {
		t.Fatal("the full image should be kept byte for byte")
	}
	thumb, _ := s.readFile(ctx, photoKey(r.Photo, true))
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(thumb)); err != nil || cfg.Width != 640 {
		t.Fatalf("thumbnail %+v %v", cfg, err)
	}
	if _, err := s.readFile(ctx, photoKey(old, false)); err == nil {
		t.Error("the old photo wasn't removed")
	}
	// Re-running finds nothing to do.
	before := r.Photo
	if err := RefreshPhotos(ctx, s.cfg, s.db, false, logf); err != nil {
		t.Fatal(err)
	}
	c.do("GET", "/api/recipes/"+r.ID, nil, 200, &r)
	if r.Photo != before {
		t.Error("a second run changed the photo again")
	}
}
