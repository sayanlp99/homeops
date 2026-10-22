package blob

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
)

// The S3 backend is exercised against a real server when one is configured, e.g. a local
// SeaweedFS: HOMEOPS_TEST_S3_ENDPOINT=127.0.0.1:8333 HOMEOPS_TEST_S3_ACCESS_KEY=… go test ./...
func stores(t *testing.T) map[string]Store {
	out := map[string]Store{}
	d, err := NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out["dir"] = d
	if ep := os.Getenv("HOMEOPS_TEST_S3_ENDPOINT"); ep != "" {
		s, err := NewS3(context.Background(), S3Config{Endpoint: ep, Bucket: "homeops-test", AccessKey: os.Getenv("HOMEOPS_TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("HOMEOPS_TEST_S3_SECRET_KEY")})
		if err != nil {
			t.Fatal(err)
		}
		out["s3"] = s
	}
	return out
}

func TestStores(t *testing.T) {
	ctx := context.Background()
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			if err := s.Put(ctx, "recipes/a.jpg", []byte("hello"), "image/jpeg"); err != nil {
				t.Fatal(err)
			}
			f, err := s.Get(ctx, "recipes/a.jpg")
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(f)
			f.Close()
			if string(b) != "hello" {
				t.Fatalf("got %q", b)
			}
			if _, err := s.Get(ctx, "recipes/missing.jpg"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("missing key: %v", err)
			}
			objs, err := s.List(ctx, "recipes/")
			if err != nil || len(objs) == 0 {
				t.Fatalf("list: %v %v", objs, err)
			}
			if err := s.Delete(ctx, "recipes/a.jpg"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Get(ctx, "recipes/a.jpg"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("after delete: %v", err)
			}
		})
	}
}
