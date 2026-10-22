// Package blob stores uploaded files: in an S3 bucket (SeaweedFS in production) or,
// when no S3 endpoint is configured, in a local directory for development and tests.
package blob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var ErrNotFound = errors.New("blob not found")

type Object struct {
	Key      string
	Modified time.Time
}

type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) (io.ReadSeekCloser, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]Object, error)
	Name() string
}

type S3Config struct {
	Endpoint  string // host:port, e.g. "seaweedfs:8333"
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

// ---- S3 ----

type s3Store struct {
	c      *minio.Client
	bucket string
}

// NewS3 connects and creates the bucket if needed, retrying while the server starts up.
func NewS3(ctx context.Context, cfg S3Config) (Store, error) {
	c, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       cfg.UseSSL,
		BucketLookup: minio.BucketLookupPath, // SeaweedFS uses path-style addressing
		Region:       "us-east-1",
	})
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		exists, err := c.BucketExists(ctx, cfg.Bucket)
		if err == nil && !exists {
			err = c.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: "us-east-1"})
			if err != nil && minio.ToErrorResponse(err).Code == "BucketAlreadyOwnedByYou" {
				err = nil
			}
		}
		if err == nil {
			return &s3Store{c, cfg.Bucket}, nil
		}
		if attempt >= 30 {
			return nil, fmt.Errorf("object storage at %s: %w", cfg.Endpoint, err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func (s *s3Store) Name() string { return "s3://" + s.bucket }

func (s *s3Store) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.c.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *s3Store) Get(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	obj, err := s.c.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	// GetObject is lazy; Stat surfaces a missing key now rather than mid-response.
	if _, err := obj.Stat(); err != nil {
		obj.Close()
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return obj, nil
}

func (s *s3Store) Delete(ctx context.Context, key string) error {
	return s.c.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func (s *s3Store) List(ctx context.Context, prefix string) ([]Object, error) {
	var out []Object
	for o := range s.c.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if o.Err != nil {
			return nil, o.Err
		}
		out = append(out, Object{Key: o.Key, Modified: o.LastModified})
	}
	return out, nil
}

// ---- local directory ----

type dirStore struct{ root string }

func NewDir(root string) (Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &dirStore{root}, nil
}

func (d *dirStore) Name() string { return "dir://" + d.root }

func (d *dirStore) path(key string) (string, error) {
	clean := filepath.Clean("/" + key)
	if strings.Contains(key, "..") {
		return "", errors.New("invalid key")
	}
	return filepath.Join(d.root, clean), nil
}

func (d *dirStore) Put(_ context.Context, key string, data []byte, _ string) error {
	p, err := d.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (d *dirStore) Get(_ context.Context, key string) (io.ReadSeekCloser, error) {
	p, err := d.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

func (d *dirStore) Delete(_ context.Context, key string) error {
	p, err := d.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (d *dirStore) List(_ context.Context, prefix string) ([]Object, error) {
	var out []Object
	err := filepath.WalkDir(d.root, func(p string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() || strings.HasSuffix(p, ".tmp") {
			return err
		}
		key := filepath.ToSlash(strings.TrimPrefix(p, d.root+string(filepath.Separator)))
		if !strings.HasPrefix(key, prefix) {
			return nil
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		out = append(out, Object{Key: key, Modified: info.ModTime()})
		return nil
	})
	return out, err
}
