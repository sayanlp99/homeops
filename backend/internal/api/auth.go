package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/argon2"
)

const (
	sessionCookie   = "ho_session"
	sessionLifetime = 90 * 24 * time.Hour
)

type User struct {
	ID          string          `json:"id" db:"id"`
	HouseholdID string          `json:"householdId" db:"household_id"`
	Username    string          `json:"username" db:"username"`
	DisplayName string          `json:"displayName" db:"display_name"`
	Role        string          `json:"role" db:"role"`
	Color       string          `json:"color" db:"color"`
	NotifyPrefs map[string]bool `json:"notifyPrefs" db:"notify_prefs"`
	QuietStart  *string         `json:"quietStart" db:"quiet_start"`
	QuietEnd    *string         `json:"quietEnd" db:"quiet_end"`
}

const userColumns = `id, household_id, username, display_name, role, color, notify_prefs, quiet_start, quiet_end`

type ctxKey int

const (
	userKey ctxKey = iota
	sessionKey
)

func currentUser(r *http.Request) *User {
	u, _ := r.Context().Value(userKey).(*User)
	return u
}

// clientID identifies the browser tab that made a change, so its own SSE echo is ignored.
func clientID(r *http.Request) string { return r.Header.Get("X-Client-Id") }

// ---- passwords ----

const (
	argonTime    = 2
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
)

func hashPassword(pw string) string {
	salt := []byte(randomToken(16))
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

func checkPassword(hash, pw string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func validatePassword(pw string) error {
	if len(pw) < 8 {
		return errBadRequest("Password must be at least 8 characters")
	}
	return nil
}

func normalizeUsername(u string) (string, error) {
	u = strings.ToLower(strings.TrimSpace(u))
	if len(u) < 2 || len(u) > 32 {
		return "", errBadRequest("Username must be 2–32 characters")
	}
	for _, c := range u {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return "", errBadRequest("Username may only contain letters, digits, . _ -")
		}
	}
	return u, nil
}

// ---- sessions ----

func hashToken(tok string) []byte {
	h := sha256.Sum256([]byte(tok))
	return h[:]
}

func (s *Server) startSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID string) error {
	tok := randomToken(32)
	_, err := s.db.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, user_agent, expires_at) VALUES ($1, $2, $3, $4)`,
		hashToken(tok), userID, truncate(r.UserAgent(), 300), time.Now().Add(sessionLifetime))
	if err != nil {
		return err
	}
	s.setSessionCookie(w, tok, sessionLifetime)
	return nil
}

func (s *Server) setSessionCookie(w http.ResponseWriter, tok string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    tok,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies(),
		SameSite: http.SameSiteLaxMode,
	})
}

// authenticate resolves the session cookie, sliding its expiry at most once an hour.
func (s *Server) authenticate(r *http.Request) (*User, []byte, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil, nil, nil
	}
	th := hashToken(c.Value)
	var u User
	var lastSeen time.Time
	err = s.db.QueryRow(r.Context(), `
		SELECT u.id, u.household_id, u.username, u.display_name, u.role, u.color, u.notify_prefs, u.quiet_start, u.quiet_end, s.last_seen_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, th).
		Scan(&u.ID, &u.HouseholdID, &u.Username, &u.DisplayName, &u.Role, &u.Color, &u.NotifyPrefs, &u.QuietStart, &u.QuietEnd, &lastSeen)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if time.Since(lastSeen) > time.Hour {
		_, err := s.db.Exec(r.Context(), `UPDATE sessions SET last_seen_at = now(), expires_at = $2 WHERE token_hash = $1`,
			th, time.Now().Add(sessionLifetime))
		logErr("slide session", err)
	}
	return &u, th, nil
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, th, err := s.authenticate(r)
		if err != nil {
			handler(func(http.ResponseWriter, *http.Request) error { return err }).ServeHTTP(w, r)
			return
		}
		if u == nil {
			writeJSON(w, http.StatusUnauthorized, &apiError{Code: "unauthorized", Message: "Please sign in"})
			return
		}
		ctx := context.WithValue(r.Context(), userKey, u)
		ctx = context.WithValue(ctx, sessionKey, th)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) optionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _, err := s.authenticate(r)
		if err == nil && u != nil {
			r = r.WithContext(context.WithValue(r.Context(), userKey, u))
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := currentUser(r); u == nil || u.Role != "admin" {
			writeJSON(w, http.StatusForbidden, &apiError{Code: "forbidden", Message: "Only the household admin can do that"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- setup, login, invites ----

var errTooManyAttempts = &apiError{http.StatusTooManyRequests, "rate_limited", "Too many failed attempts. Try again in a few minutes."}

var defaultLabels = []struct{ name, color string }{
	{"Cleaning", "#0ea5e9"}, {"Laundry", "#8b5cf6"}, {"Shopping", "#f59e0b"}, {"Bills", "#ef4444"},
	{"Repairs", "#64748b"}, {"Errands", "#10b981"}, {"Admin/Paperwork", "#ec4899"},
}

type newUserInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

func (in *newUserInput) validate() error {
	var err error
	if in.Username, err = normalizeUsername(in.Username); err != nil {
		return err
	}
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.DisplayName == "" || len(in.DisplayName) > 40 {
		return errBadRequest("Display name is required (max 40 characters)")
	}
	return validatePassword(in.Password)
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		newUserInput
		HouseholdName string `json:"householdName"`
		Timezone      string `json:"timezone"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	in.HouseholdName = strings.TrimSpace(in.HouseholdName)
	if in.HouseholdName == "" {
		in.HouseholdName = "Our Home"
	}
	if in.Timezone == "" {
		in.Timezone = "Asia/Kolkata"
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return errBadRequest("Unknown time zone")
	}

	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Lock so two simultaneous setup requests can't both succeed.
	if _, err := tx.Exec(ctx, `LOCK TABLE users IN EXCLUSIVE MODE`); err != nil {
		return err
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return errConflict("Setup has already been completed")
	}
	var hid, uid string
	if err := tx.QueryRow(ctx, `INSERT INTO households (name, timezone) VALUES ($1, $2) RETURNING id`,
		in.HouseholdName, in.Timezone).Scan(&hid); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `INSERT INTO users (household_id, username, display_name, password_hash, role, color)
		VALUES ($1, $2, $3, $4, 'admin', '#2563eb') RETURNING id`,
		hid, in.Username, in.DisplayName, hashPassword(in.Password)).Scan(&uid); err != nil {
		return err
	}
	for _, l := range defaultLabels {
		if _, err := tx.Exec(ctx, `INSERT INTO labels (household_id, name, color) VALUES ($1, $2, $3)`, hid, l.name, l.color); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if err := s.ensureSprints(ctx, hid); err != nil {
		return err
	}
	if err := s.startSession(ctx, w, r, uid); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	key := "login:" + clientIP(r)
	if s.limiter.Blocked(key) {
		return errTooManyAttempts
	}
	var in struct{ Username, Password string }
	if err := decode(r, &in); err != nil {
		return err
	}
	var id, hash string
	err := s.db.QueryRow(r.Context(), `SELECT id, password_hash FROM users WHERE username = $1`,
		strings.ToLower(strings.TrimSpace(in.Username))).Scan(&id, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		hashPassword(in.Password) // equalize timing
		s.limiter.Fail(key)
		return errUnauthorized("Wrong username or password")
	}
	if err != nil {
		return err
	}
	if !checkPassword(hash, in.Password) {
		s.limiter.Fail(key)
		return errUnauthorized("Wrong username or password")
	}
	if err := s.startSession(r.Context(), w, r, id); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) error {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_, err := s.db.Exec(r.Context(), `DELETE FROM sessions WHERE token_hash = $1`, hashToken(c.Value))
		logErr("delete session", err)
	}
	s.setSessionCookie(w, "", -time.Second)
	return noContent(w)
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	code := randomCode(10)
	exp := time.Now().Add(7 * 24 * time.Hour)
	if _, err := s.db.Exec(r.Context(), `INSERT INTO invites (code, household_id, created_by, expires_at) VALUES ($1, $2, $3, $4)`,
		code, u.HouseholdID, u.ID, exp); err != nil {
		return err
	}
	return ok(w, map[string]any{"code": code, "url": s.cfg.PublicURL + "/join/" + code, "expiresAt": exp})
}

func (s *Server) checkInvite(w http.ResponseWriter, r *http.Request) error {
	key := "invite:" + clientIP(r)
	if s.limiter.Blocked(key) {
		return errTooManyAttempts
	}
	var name, inviter string
	err := s.db.QueryRow(r.Context(), `
		SELECT h.name, u.display_name FROM invites i
		JOIN households h ON h.id = i.household_id JOIN users u ON u.id = i.created_by
		WHERE i.code = $1 AND i.used_at IS NULL AND i.expires_at > now()`, strings.ToUpper(chi.URLParam(r, "code"))).Scan(&name, &inviter)
	if errors.Is(err, pgx.ErrNoRows) {
		s.limiter.Fail(key)
		return errNotFound("This invite link is invalid or has expired")
	}
	if err != nil {
		return err
	}
	return ok(w, map[string]string{"householdName": name, "invitedBy": inviter})
}

func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request) error {
	key := "invite:" + clientIP(r)
	if s.limiter.Blocked(key) {
		return errTooManyAttempts
	}
	var in struct {
		newUserInput
		Code string `json:"code"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var hid string
	err = tx.QueryRow(ctx, `UPDATE invites SET used_at = now()
		WHERE code = $1 AND used_at IS NULL AND expires_at > now() RETURNING household_id`, strings.ToUpper(in.Code)).Scan(&hid)
	if errors.Is(err, pgx.ErrNoRows) {
		s.limiter.Fail(key)
		return errNotFound("This invite link is invalid or has expired")
	}
	if err != nil {
		return err
	}
	var uid string
	if err := tx.QueryRow(ctx, `INSERT INTO users (household_id, username, display_name, password_hash, role, color)
		VALUES ($1, $2, $3, $4, 'member', '#db2777') RETURNING id`,
		hid, in.Username, in.DisplayName, hashPassword(in.Password)).Scan(&uid); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.hub.Publish(hid, eventMembers.from(r))
	if err := s.startSession(ctx, w, r, uid); err != nil {
		return err
	}
	return noContent(w)
}

// ---- profile & sessions ----

var notifyKinds = []string{"task_assigned", "task_due", "task_overdue", "ceremony", "meals", "budget", "kudos"}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in struct {
		DisplayName *string          `json:"displayName"`
		Color       *string          `json:"color"`
		NotifyPrefs map[string]bool  `json:"notifyPrefs"`
		QuietStart  *optionalString  `json:"quietStart"`
		QuietEnd    *optionalString  `json:"quietEnd"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.DisplayName != nil {
		name := strings.TrimSpace(*in.DisplayName)
		if name == "" || len(name) > 40 {
			return errBadRequest("Display name is required (max 40 characters)")
		}
		u.DisplayName = name
	}
	if in.Color != nil {
		if !isHexColor(*in.Color) {
			return errBadRequest("Invalid color")
		}
		u.Color = *in.Color
	}
	if in.NotifyPrefs != nil {
		if u.NotifyPrefs == nil {
			u.NotifyPrefs = map[string]bool{}
		}
		for _, k := range notifyKinds {
			if v, ok := in.NotifyPrefs[k]; ok {
				u.NotifyPrefs[k] = v
			}
		}
	}
	for _, f := range []struct {
		in  *optionalString
		dst **string
	}{{in.QuietStart, &u.QuietStart}, {in.QuietEnd, &u.QuietEnd}} {
		if f.in == nil {
			continue
		}
		if f.in.v != nil {
			if _, err := parseClock(*f.in.v); err != nil {
				return errBadRequest("Quiet hours must be HH:MM")
			}
		}
		*f.dst = f.in.v
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE users SET display_name = $2, color = $3, notify_prefs = $4, quiet_start = $5, quiet_end = $6 WHERE id = $1`,
		u.ID, u.DisplayName, u.Color, u.NotifyPrefs, u.QuietStart, u.QuietEnd); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventMembers.from(r))
	return ok(w, u)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in struct{ Current, New string }
	if err := decode(r, &in); err != nil {
		return err
	}
	var hash string
	if err := s.db.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id = $1`, u.ID).Scan(&hash); err != nil {
		return err
	}
	if !checkPassword(hash, in.Current) {
		return errBadRequest("Current password is wrong")
	}
	if err := validatePassword(in.New); err != nil {
		return err
	}
	th, _ := r.Context().Value(sessionKey).([]byte)
	// Changing the password signs out every other device.
	if _, err := s.db.Exec(r.Context(), `UPDATE users SET password_hash = $2 WHERE id = $1`, u.ID, hashPassword(in.New)); err != nil {
		return err
	}
	_, err := s.db.Exec(r.Context(), `DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2`, u.ID, th)
	if err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	th, _ := r.Context().Value(sessionKey).([]byte)
	rows, err := s.db.Query(r.Context(), `SELECT token_hash, user_agent, created_at, last_seen_at FROM sessions
		WHERE user_id = $1 AND expires_at > now() ORDER BY last_seen_at DESC`, u.ID)
	if err != nil {
		return err
	}
	type session struct {
		ID         string    `json:"id"`
		UserAgent  string    `json:"userAgent"`
		CreatedAt  time.Time `json:"createdAt"`
		LastSeenAt time.Time `json:"lastSeenAt"`
		Current    bool      `json:"current"`
	}
	out := []session{}
	for rows.Next() {
		var h []byte
		var ss session
		if err := rows.Scan(&h, &ss.UserAgent, &ss.CreatedAt, &ss.LastSeenAt); err != nil {
			return err
		}
		ss.ID = base64.RawURLEncoding.EncodeToString(h)
		ss.Current = subtle.ConstantTimeCompare(h, th) == 1
		out = append(out, ss)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return ok(w, out)
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) error {
	h, err := base64.RawURLEncoding.DecodeString(chi.URLParam(r, "id"))
	if err != nil {
		return errBadRequest("Invalid session id")
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM sessions WHERE token_hash = $1 AND user_id = $2`, h, currentUser(r).ID); err != nil {
		return err
	}
	return noContent(w)
}

// optionalString distinguishes "absent" (nil pointer) from explicit null in PATCH bodies.
type optionalString struct{ v *string }

func (o *optionalString) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		o.v = nil
		return nil
	}
	var s string
	if err := jsonUnmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		o.v = nil
	} else {
		o.v = &s
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func isHexColor(c string) bool {
	if len(c) != 7 || c[0] != '#' {
		return false
	}
	for _, ch := range strings.ToLower(c[1:]) {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	return true
}
