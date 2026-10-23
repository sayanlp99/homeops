package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
)

const (
	sessionCookie   = "ho_session"
	sessionLifetime = 90 * 24 * time.Hour
)

// User is the signed-in person. HouseholdID and Role describe the household this
// session is currently in; both are empty when the person belongs to none yet.
type User struct {
	ID              string          `json:"id"`
	HouseholdID     string          `json:"householdId"`
	Role            string          `json:"role"`
	Username        string          `json:"username"`
	DisplayName     string          `json:"displayName"`
	Email           string          `json:"email"`
	IsPlatformAdmin bool            `json:"isPlatformAdmin"`
	Color           string          `json:"color"`
	NotifyPrefs     map[string]bool `json:"notifyPrefs"`
	QuietStart      *string         `json:"quietStart"`
	QuietEnd        *string         `json:"quietEnd"`
}

// Household roles, lowest to highest.
var roleRank = map[string]int{"viewer": 1, "member": 2, "admin": 3, "owner": 4}

func (u *User) atLeast(role string) bool { return roleRank[u.Role] >= roleRank[role] }

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

func normalizeEmail(e string) (string, error) {
	e = strings.ToLower(strings.TrimSpace(e))
	a, err := mail.ParseAddress(e)
	if err != nil || a.Address != e || a.Name != "" || len(e) > 254 || !strings.Contains(e[strings.LastIndex(e, "@")+1:], ".") {
		return "", errBadRequest("Enter a valid email address")
	}
	return e, nil
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
	// Resume the household used last; authenticate falls back to any other membership.
	_, err := s.db.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, user_agent, expires_at, household_id)
		SELECT $1, $2, $3, $4, last_household_id FROM users WHERE id = $2`,
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
	var sessionHousehold *string
	// The session's household, or — if the person was removed from it — their oldest membership.
	err = s.db.QueryRow(r.Context(), `
		SELECT u.id, u.username, u.display_name, coalesce(u.email, ''), u.is_platform_admin, u.color, u.notify_prefs,
			u.quiet_start, u.quiet_end, s.last_seen_at, s.household_id::text,
			coalesce(m.household_id::text, ''), coalesce(m.role, '')
		FROM sessions s JOIN users u ON u.id = s.user_id
		LEFT JOIN LATERAL (SELECT household_id, role FROM memberships WHERE user_id = u.id
			ORDER BY household_id = s.household_id DESC NULLS LAST, created_at LIMIT 1) m ON true
		WHERE s.token_hash = $1 AND s.expires_at > now()`, th).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.IsPlatformAdmin, &u.Color, &u.NotifyPrefs,
			&u.QuietStart, &u.QuietEnd, &lastSeen, &sessionHousehold, &u.HouseholdID, &u.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if u.HouseholdID != "" && (sessionHousehold == nil || *sessionHousehold != u.HouseholdID) {
		_, err := s.db.Exec(r.Context(), `UPDATE sessions SET household_id = $2 WHERE token_hash = $1`, th, u.HouseholdID)
		logErr("move session household", err)
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

// requireHousehold guards household data: the person must belong to one, and
// viewers may only read.
func (s *Server) requireHousehold(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u.HouseholdID == "" {
			writeJSON(w, http.StatusForbidden, &apiError{Code: "no_household", Message: "You're not part of a household yet"})
			return
		}
		if u.Role == "viewer" && r.Method != http.MethodGet && r.Method != http.MethodHead && !viewerWritable[r.URL.Path] &&
			!(r.Method == http.MethodDelete && r.URL.Path == "/api/members/"+u.ID) {
			writeJSON(w, http.StatusForbidden, &apiError{Code: "read_only", Message: "You have view-only access to this household"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Writes a viewer may still make: they only touch the viewer's own state.
var viewerWritable = map[string]bool{"/api/notifications/read": true}

// requireRole limits a route to people with at least this role in the current household.
func requireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !currentUser(r).atLeast(role) {
				writeJSON(w, http.StatusForbidden, &apiError{Code: "forbidden", Message: "You need to be a household " + role + " to do that"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func platformAdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).IsPlatformAdmin {
			writeJSON(w, http.StatusForbidden, &apiError{Code: "forbidden", Message: "Only the HomeOps administrator can do that"})
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
		Email         string `json:"email"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	var err error
	if in.Email, err = normalizeEmail(in.Email); err != nil {
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
	// The first person administers the whole installation.
	var uid string
	if err := tx.QueryRow(ctx, `INSERT INTO users (username, display_name, email, password_hash, color, is_platform_admin)
		VALUES ($1, $2, $3, $4, '#2563eb', true) RETURNING id`,
		in.Username, in.DisplayName, in.Email, hashPassword(in.Password)).Scan(&uid); err != nil {
		return err
	}
	hid, err := createHousehold(ctx, tx, in.HouseholdName, in.Timezone)
	if err != nil {
		return err
	}
	if err := addMember(ctx, tx, hid, uid, "owner"); err != nil {
		return err
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
	err := s.db.QueryRow(r.Context(), `SELECT id, password_hash FROM users WHERE username = $1 OR lower(email) = $1`,
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

// ---- profile & sessions ----

var notifyKinds = []string{"task_assigned", "task_due", "task_overdue", "ceremony", "meals", "budget", "kudos"}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in struct {
		DisplayName *string         `json:"displayName"`
		Color       *string         `json:"color"`
		NotifyPrefs map[string]bool `json:"notifyPrefs"`
		QuietStart  *optionalString `json:"quietStart"`
		QuietEnd    *optionalString `json:"quietEnd"`
		Email       *string         `json:"email"`
		Password    string          `json:"password"` // required to change the email
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Email != nil {
		email, err := normalizeEmail(*in.Email)
		if err != nil {
			return err
		}
		if email != u.Email {
			// The email can reset the password, so changing it needs the password.
			var hash string
			if err := s.db.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id = $1`, u.ID).Scan(&hash); err != nil {
				return err
			}
			if !checkPassword(hash, in.Password) {
				return errBadRequest("Enter your current password to change your email")
			}
			if taken, err := s.emailTaken(r.Context(), email, u.ID); err != nil {
				return err
			} else if taken {
				return errConflict("Another account already uses that email")
			}
			u.Email = email
		}
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
	if _, err := s.db.Exec(r.Context(), `UPDATE users SET display_name = $2, color = $3, notify_prefs = $4, quiet_start = $5, quiet_end = $6,
		email = nullif($7, '') WHERE id = $1`,
		u.ID, u.DisplayName, u.Color, u.NotifyPrefs, u.QuietStart, u.QuietEnd, u.Email); err != nil {
		return err
	}
	s.publishToUserHouseholds(r, u.ID, eventMembers)
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

// ResetPassword sets a new password for a user and signs them out everywhere. It is
// the server-side fallback (`homeops passwd`) when password-reset email isn't set up.
func ResetPassword(ctx context.Context, db *pgxpool.Pool, username, password string) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `UPDATE users SET password_hash = $2 WHERE username = $1 OR lower(email) = $1 RETURNING id`,
		strings.ToLower(strings.TrimSpace(username)), hashPassword(password)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("no user %q", username)
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM password_resets WHERE user_id = $1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
