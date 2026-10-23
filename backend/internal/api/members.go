package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// createHousehold inserts a household with the default labels. Sprints are created
// by ensureSprints once the transaction commits.
func createHousehold(ctx context.Context, tx pgx.Tx, name, tz string) (string, error) {
	var hid string
	if err := tx.QueryRow(ctx, `INSERT INTO households (name, timezone) VALUES ($1, $2) RETURNING id`, name, tz).Scan(&hid); err != nil {
		return "", err
	}
	for _, l := range defaultLabels {
		if _, err := tx.Exec(ctx, `INSERT INTO labels (household_id, name, color) VALUES ($1, $2, $3)`, hid, l.name, l.color); err != nil {
			return "", err
		}
	}
	return hid, nil
}

// addMember adds (or re-roles) someone and makes it the household they land in next.
func addMember(ctx context.Context, tx pgx.Tx, householdID, userID, role string) error {
	if _, err := tx.Exec(ctx, `INSERT INTO memberships (household_id, user_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (household_id, user_id) DO UPDATE SET role = EXCLUDED.role`, householdID, userID, role); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE users SET last_household_id = $2 WHERE id = $1`, userID, householdID)
	return err
}

func (s *Server) emailTaken(ctx context.Context, email, exceptUserID string) (bool, error) {
	var taken bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = $1 AND id::text <> $2)`, email, exceptUserID).Scan(&taken)
	return taken, err
}

// publishToUserHouseholds tells every household someone belongs to that they changed
// (their name or color shows in all of them).
func (s *Server) publishToUserHouseholds(r *http.Request, userID string, kind topic) {
	rows, err := s.db.Query(r.Context(), `SELECT household_id::text FROM memberships WHERE user_id = $1`, userID)
	if err != nil {
		logErr("list memberships", err)
		return
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	logErr("list memberships", err)
	for _, id := range ids {
		s.hub.Publish(id, kind.from(r))
	}
}

func validRole(role string) bool { _, ok := roleRank[role]; return ok }

// ---- my households ----

type myHousehold struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Members int    `json:"members"`
	Unread  int    `json:"unread"`
}

func (s *Server) myHouseholds(ctx context.Context, userID string) ([]myHousehold, error) {
	rows, _ := s.db.Query(ctx, `SELECT h.id, h.name, m.role,
			(SELECT count(*) FROM memberships x WHERE x.household_id = h.id)::int,
			(SELECT count(*) FROM notifications n WHERE n.user_id = m.user_id AND n.household_id = h.id AND n.read_at IS NULL)::int
		FROM memberships m JOIN households h ON h.id = m.household_id
		WHERE m.user_id = $1 ORDER BY h.name`, userID)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (myHousehold, error) {
		var h myHousehold
		err := row.Scan(&h.ID, &h.Name, &h.Role, &h.Members, &h.Unread)
		return h, err
	})
}

func (s *Server) switchHousehold(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in struct {
		HouseholdID string `json:"householdId"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	var member bool
	if err := s.db.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM memberships WHERE user_id = $1 AND household_id::text = $2)`,
		u.ID, in.HouseholdID).Scan(&member); err != nil {
		return err
	}
	if !member {
		return errNotFound("You're not a member of that household")
	}
	th, _ := r.Context().Value(sessionKey).([]byte)
	if _, err := s.db.Exec(r.Context(), `UPDATE sessions SET household_id = $2 WHERE token_hash = $1`, th, in.HouseholdID); err != nil {
		return err
	}
	_, err := s.db.Exec(r.Context(), `UPDATE users SET last_household_id = $2 WHERE id = $1`, u.ID, in.HouseholdID)
	if err != nil {
		return err
	}
	return noContent(w)
}

// ---- members & invites (household admins) ----

type pendingInvite struct {
	Code      string    `json:"code"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	InvitedBy string    `json:"invitedBy"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	URL       string    `json:"url"`
}

func (s *Server) listInvites(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	rows, _ := s.db.Query(r.Context(), `SELECT i.code, i.email, i.role, coalesce(u.display_name, ''), i.created_at, i.expires_at
		FROM invites i LEFT JOIN users u ON u.id = i.created_by
		WHERE i.household_id = $1 AND i.used_at IS NULL AND i.expires_at > now() ORDER BY i.created_at DESC`, u.HouseholdID)
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (pendingInvite, error) {
		var p pendingInvite
		err := row.Scan(&p.Code, &p.Email, &p.Role, &p.InvitedBy, &p.CreatedAt, &p.ExpiresAt)
		p.URL = s.cfg.PublicURL + "/join/" + p.Code
		return p, err
	})
	if err != nil {
		return err
	}
	return ok(w, out)
}

// invite creates an email invite and sends it. If the person already has an account
// the email still goes out: they accept by signing in, which keeps consent explicit.
func (s *Server) invite(ctx context.Context, inviter *User, householdID, householdName, email, role string) (map[string]any, error) {
	var already bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM members WHERE household_id = $1 AND lower(email) = $2)`,
		householdID, email).Scan(&already); err != nil {
		return nil, err
	}
	if already {
		return nil, errConflict(email + " is already a member")
	}
	// Re-inviting the same address replaces the earlier link.
	if _, err := s.db.Exec(ctx, `DELETE FROM invites WHERE household_id = $1 AND lower(email) = $2 AND used_at IS NULL`, householdID, email); err != nil {
		return nil, err
	}
	code := randomCode(12)
	exp := time.Now().Add(7 * 24 * time.Hour)
	var createdBy *string
	inviterName := "The HomeOps administrator"
	if inviter != nil {
		createdBy, inviterName = &inviter.ID, inviter.DisplayName
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO invites (code, household_id, created_by, expires_at, email, role) VALUES ($1, $2, $3, $4, $5, $6)`,
		code, householdID, createdBy, exp, email, role); err != nil {
		return nil, err
	}
	url := s.cfg.PublicURL + "/join/" + code
	err := s.mailer.Send(ctx, inviteEmail(email, inviterName, householdName, role, url))
	logErr("send invite email", err, "to", email)
	return map[string]any{"code": code, "url": url, "expiresAt": exp, "emailSent": err == nil && s.mailer.Enabled()}, nil
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in struct{ Email, Role string }
	if err := decode(r, &in); err != nil {
		return err
	}
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return err
	}
	if in.Role == "" {
		in.Role = "member"
	}
	if !validRole(in.Role) {
		return errBadRequest("Unknown role")
	}
	if !u.atLeast(in.Role) {
		return errForbidden("You can't invite someone with a higher role than yours")
	}
	h, err := s.getHousehold(r.Context(), u.HouseholdID)
	if err != nil {
		return err
	}
	res, err := s.invite(r.Context(), u, h.ID, h.Name, email, in.Role)
	if err != nil {
		return err
	}
	return ok(w, res)
}

func (s *Server) revokeInvite(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.db.Exec(r.Context(), `DELETE FROM invites WHERE code = $1 AND household_id = $2 AND used_at IS NULL`,
		chi.URLParam(r, "code"), currentUser(r).HouseholdID); err != nil {
		return err
	}
	return noContent(w)
}

// memberTarget loads someone else's role and checks the actor may manage them.
func (s *Server) memberTarget(ctx context.Context, actor *User, id string) (string, error) {
	var role string
	err := s.db.QueryRow(ctx, `SELECT role FROM memberships WHERE household_id = $1 AND user_id::text = $2`, actor.HouseholdID, id).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errNotFound("Member not found")
	}
	if err != nil {
		return "", err
	}
	if id != actor.ID && roleRank[role] > roleRank[actor.Role] {
		return "", errForbidden("Only an owner can change another owner")
	}
	return role, nil
}

// lastOwnerCheck refuses changes that would leave the household without an owner.
func (s *Server) lastOwnerCheck(ctx context.Context, householdID, userID, currentRole string) error {
	if currentRole != "owner" {
		return nil
	}
	var owners int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE household_id = $1 AND role = 'owner'`, householdID).Scan(&owners); err != nil {
		return err
	}
	if owners <= 1 {
		return errBadRequest("A household needs at least one owner. Make someone else an owner first.")
	}
	return nil
}

func (s *Server) updateMember(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	id := chi.URLParam(r, "id")
	var in struct{ Role string }
	if err := decode(r, &in); err != nil {
		return err
	}
	if !validRole(in.Role) {
		return errBadRequest("Unknown role")
	}
	if !u.atLeast(in.Role) {
		return errForbidden("You can't give a role higher than your own")
	}
	current, err := s.memberTarget(r.Context(), u, id)
	if err != nil {
		return err
	}
	if in.Role != "owner" {
		if err := s.lastOwnerCheck(r.Context(), u.HouseholdID, id, current); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE memberships SET role = $3 WHERE household_id = $1 AND user_id::text = $2`, u.HouseholdID, id, in.Role); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventMembers.from(r))
	return noContent(w)
}

// removeMember removes someone; anyone may remove themselves (leave).
func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	id := chi.URLParam(r, "id")
	if id != u.ID && !u.atLeast("admin") {
		return errForbidden("Only household admins can remove members")
	}
	current, err := s.memberTarget(r.Context(), u, id)
	if err != nil {
		return err
	}
	if err := s.lastOwnerCheck(r.Context(), u.HouseholdID, id, current); err != nil {
		return err
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM memberships WHERE household_id = $1 AND user_id::text = $2`, u.HouseholdID, id); err != nil {
		return err
	}
	// Their open work goes back to the pool; history keeps their name.
	if _, err := tx.Exec(ctx, `UPDATE tasks SET assignee_id = NULL WHERE household_id = $1 AND assignee_id::text = $2 AND status <> 'done'`, u.HouseholdID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE recurring_tasks SET assignee_id = NULL WHERE household_id = $1 AND assignee_id::text = $2`, u.HouseholdID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM notifications WHERE household_id = $1 AND user_id::text = $2`, u.HouseholdID, id); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.hub.Publish(u.HouseholdID, eventMembers.from(r))
	s.hub.Publish(u.HouseholdID, eventTasks.from(r))
	return noContent(w)
}

func (s *Server) deleteHousehold(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in struct{ ConfirmName string }
	if err := decode(r, &in); err != nil {
		return err
	}
	return s.deleteHouseholdByID(w, r, u.HouseholdID, in.ConfirmName)
}

func (s *Server) deleteHouseholdByID(w http.ResponseWriter, r *http.Request, id, confirm string) error {
	h, err := s.getHousehold(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotFound("Household not found")
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(confirm) != h.Name {
		return errBadRequest("Type the household's name to confirm")
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM households WHERE id = $1`, id); err != nil {
		return err
	}
	s.hub.Publish(id, eventMembers.from(r))
	return noContent(w)
}

// ---- accepting invites ----

type inviteInfo struct {
	householdID, householdName, inviter, email, role string
}

func (s *Server) lookupInvite(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, code string, lock bool) (*inviteInfo, error) {
	sql := `SELECT i.household_id, h.name, coalesce(u.display_name, 'The HomeOps administrator'), i.email, i.role
		FROM invites i JOIN households h ON h.id = i.household_id LEFT JOIN users u ON u.id = i.created_by
		WHERE i.code = $1 AND i.used_at IS NULL AND i.expires_at > now()`
	if lock {
		sql += ` FOR UPDATE OF i`
	}
	var in inviteInfo
	err := q.QueryRow(ctx, sql, strings.ToUpper(strings.TrimSpace(code))).Scan(&in.householdID, &in.householdName, &in.inviter, &in.email, &in.role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errNotFound("This invite link is invalid or has expired")
	}
	return &in, err
}

func (s *Server) checkInvite(w http.ResponseWriter, r *http.Request) error {
	key := "invite:" + clientIP(r)
	if s.limiter.Blocked(key) {
		return errTooManyAttempts
	}
	inv, err := s.lookupInvite(r.Context(), s.db, chi.URLParam(r, "code"), false)
	if err != nil {
		s.limiter.Fail(key)
		return err
	}
	var exists bool
	if err := s.db.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = $1)`, inv.email).Scan(&exists); err != nil {
		return err
	}
	return ok(w, map[string]any{"householdName": inv.householdName, "invitedBy": inv.inviter, "email": inv.email,
		"role": inv.role, "accountExists": exists})
}

func (s *Server) useInvite(ctx context.Context, tx pgx.Tx, code, userID string) (*inviteInfo, error) {
	inv, err := s.lookupInvite(ctx, tx, code, true)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE invites SET used_at = now(), used_by = $2 WHERE code = $1`, strings.ToUpper(strings.TrimSpace(code)), userID); err != nil {
		return nil, err
	}
	return inv, addMember(ctx, tx, inv.householdID, userID, inv.role)
}

// acceptInvite creates a new account from an invite. The account gets the invited email.
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
	inv, err := s.lookupInvite(ctx, tx, in.Code, true)
	if err != nil {
		s.limiter.Fail(key)
		return err
	}
	if taken, err := s.emailTaken(ctx, inv.email, ""); err != nil {
		return err
	} else if taken {
		return errConflict("You already have an account. Sign in to accept this invite.")
	}
	var taken bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE username = $1)`, in.Username).Scan(&taken); err != nil {
		return err
	}
	if taken {
		return errConflict("That username is taken")
	}
	var uid string
	if err := tx.QueryRow(ctx, `INSERT INTO users (username, display_name, email, password_hash, color)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		in.Username, in.DisplayName, inv.email, hashPassword(in.Password), memberColor(in.Username)).Scan(&uid); err != nil {
		return err
	}
	if _, err := s.useInvite(ctx, tx, in.Code, uid); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.hub.Publish(inv.householdID, eventMembers.from(r))
	if err := s.startSession(ctx, w, r, uid); err != nil {
		return err
	}
	return noContent(w)
}

// joinWithInvite adds a signed-in person to the invite's household. The invite must
// have been sent to their email, so a forwarded link can't be used by someone else.
func (s *Server) joinWithInvite(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	inv, err := s.lookupInvite(ctx, tx, chi.URLParam(r, "code"), true)
	if err != nil {
		return err
	}
	if !strings.EqualFold(inv.email, u.Email) {
		return errForbidden("This invite was sent to " + inv.email + ". Sign in with that account to accept it.")
	}
	var role string
	err = tx.QueryRow(ctx, `SELECT role FROM memberships WHERE household_id = $1 AND user_id = $2`, inv.householdID, u.ID).Scan(&role)
	if err == nil && roleRank[role] >= roleRank[inv.role] {
		inv.role = role // never downgrade someone through an invite
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE invites SET used_at = now(), used_by = $2 WHERE code = $1`, strings.ToUpper(chi.URLParam(r, "code")), u.ID); err != nil {
		return err
	}
	if err := addMember(ctx, tx, inv.householdID, u.ID, inv.role); err != nil {
		return err
	}
	th, _ := r.Context().Value(sessionKey).([]byte)
	if _, err := tx.Exec(ctx, `UPDATE sessions SET household_id = $2 WHERE token_hash = $1`, th, inv.householdID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.hub.Publish(inv.householdID, eventMembers.from(r))
	return ok(w, map[string]string{"householdId": inv.householdID})
}

var memberColors = []string{"#2563eb", "#db2777", "#059669", "#d97706", "#7c3aed", "#0891b2", "#dc2626", "#4f46e5"}

func memberColor(seed string) string {
	h := 0
	for _, c := range seed {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return memberColors[h%len(memberColors)]
}

// ---- password reset ----

const resetLifetime = time.Hour

func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) error {
	key := "forgot:" + clientIP(r)
	if s.limiter.Blocked(key) {
		return errTooManyAttempts
	}
	// Every request counts, so the endpoint can't be used to spam someone's inbox.
	s.limiter.Fail(key)
	var in struct{ Email string }
	if err := decode(r, &in); err != nil {
		return err
	}
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return err
	}
	var uid, name string
	err = s.db.QueryRow(r.Context(), `SELECT id, display_name FROM users WHERE lower(email) = $1`, email).Scan(&uid, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return noContent(w) // don't reveal which emails have accounts
	}
	if err != nil {
		return err
	}
	tok := randomToken(32)
	if _, err := s.db.Exec(r.Context(), `INSERT INTO password_resets (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		hashToken(tok), uid, time.Now().Add(resetLifetime)); err != nil {
		return err
	}
	// Send in the background so response time doesn't reveal whether the account exists.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		logErr("send reset email", s.mailer.Send(ctx, resetEmail(email, name, s.cfg.PublicURL+"/reset/"+tok)), "to", email)
	}()
	return noContent(w)
}

func (s *Server) resetLookup(ctx context.Context, tok string) (string, string, error) {
	var uid, username string
	err := s.db.QueryRow(ctx, `SELECT u.id, u.username FROM password_resets p JOIN users u ON u.id = p.user_id
		WHERE p.token_hash = $1 AND p.used_at IS NULL AND p.expires_at > now()`, hashToken(tok)).Scan(&uid, &username)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", errNotFound("This reset link is invalid or has expired. Request a new one.")
	}
	return uid, username, err
}

func (s *Server) checkReset(w http.ResponseWriter, r *http.Request) error {
	key := "reset:" + clientIP(r)
	if s.limiter.Blocked(key) {
		return errTooManyAttempts
	}
	_, username, err := s.resetLookup(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		s.limiter.Fail(key)
		return err
	}
	return ok(w, map[string]string{"username": username})
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) error {
	key := "reset:" + clientIP(r)
	if s.limiter.Blocked(key) {
		return errTooManyAttempts
	}
	var in struct{ Token, Password string }
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := validatePassword(in.Password); err != nil {
		return err
	}
	uid, _, err := s.resetLookup(r.Context(), in.Token)
	if err != nil {
		s.limiter.Fail(key)
		return err
	}
	ctx := r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE password_resets SET used_at = now() WHERE token_hash = $1 AND used_at IS NULL`, hashToken(in.Token))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNotFound("This reset link has already been used")
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, uid, hashPassword(in.Password)); err != nil {
		return err
	}
	// Whoever might have had the old password is signed out everywhere.
	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, uid); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM password_resets WHERE user_id = $1 AND used_at IS NULL`, uid); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if err := s.startSession(ctx, w, r, uid); err != nil {
		return err
	}
	return noContent(w)
}
