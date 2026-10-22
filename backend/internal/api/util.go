package api

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ttlStore holds short-lived values such as WebAuthn ceremony state.
type ttlStore[T any] struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]ttlItem[T]
}

type ttlItem[T any] struct {
	v   T
	exp time.Time
}

func newTTLStore[T any](ttl time.Duration) *ttlStore[T] {
	return &ttlStore[T]{ttl: ttl, items: make(map[string]ttlItem[T])}
}

func (s *ttlStore[T]) Put(key string, v T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, it := range s.items {
		if now.After(it.exp) {
			delete(s.items, k)
		}
	}
	s.items[key] = ttlItem[T]{v, now.Add(s.ttl)}
}

// Take returns and removes the value, so each ceremony can be finished once.
func (s *ttlStore[T]) Take(key string) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, found := s.items[key]
	delete(s.items, key)
	if !found || time.Now().After(it.exp) {
		var zero T
		return zero, false
	}
	return it.v, true
}

// rateLimiter counts failures per key (e.g. client IP) in a fixed window. Only failed
// attempts count, so normal use from a shared home IP never trips it.
type rateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*window
}

type window struct {
	start time.Time
	count int
}

func newRateLimiter(limit int, w time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: w, hits: make(map[string]*window)}
}

func (l *rateLimiter) current(key string) *window {
	w := l.hits[key]
	if w == nil || time.Since(w.start) > l.window {
		if len(l.hits) > 10000 {
			l.hits = make(map[string]*window)
		}
		w = &window{start: time.Now()}
		l.hits[key] = w
	}
	return w
}

// Blocked reports whether the key has used up its failures for this window.
func (l *rateLimiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.current(key).count >= l.limit
}

func (l *rateLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.current(key).count++
}

func randomToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func randomCode(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)[:n]
}

// querier is satisfied by both the pool and a transaction.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// parseClock parses "HH:MM" into minutes after midnight.
func parseClock(s string) (int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}

func randomUUID() string { return uuid.NewString() }
