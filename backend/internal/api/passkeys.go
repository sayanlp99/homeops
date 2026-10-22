package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
)

var jsonUnmarshal = json.Unmarshal

// waUser adapts a user row to webauthn.User. The user handle is the UUID string.
type waUser struct {
	id, username, displayName string
	creds                     []webauthn.Credential
}

func (u *waUser) WebAuthnID() []byte                         { return []byte(u.id) }
func (u *waUser) WebAuthnName() string                       { return u.username }
func (u *waUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func (s *Server) loadWAUser(ctx context.Context, userID string) (*waUser, error) {
	u := &waUser{id: userID}
	if err := s.db.QueryRow(ctx, `SELECT username, display_name FROM users WHERE id = $1`, userID).
		Scan(&u.username, &u.displayName); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT data FROM passkeys WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c webauthn.Credential
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		u.creds = append(u.creds, c)
	}
	return u, rows.Err()
}

func (s *Server) passkeyRegisterBegin(w http.ResponseWriter, r *http.Request) error {
	u, err := s.loadWAUser(r.Context(), currentUser(r).ID)
	if err != nil {
		return err
	}
	exclude := make([]protocol.CredentialDescriptor, 0, len(u.creds))
	for _, c := range u.creds {
		exclude = append(exclude, c.Descriptor())
	}
	opts, sess, err := s.wa.BeginRegistration(u,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(exclude),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationPreferred,
		}),
	)
	if err != nil {
		return err
	}
	s.ceremony.Put("reg:"+u.id, *sess)
	return ok(w, opts)
}

func (s *Server) passkeyRegisterFinish(w http.ResponseWriter, r *http.Request) error {
	cu := currentUser(r)
	sess, found := s.ceremony.Take("reg:" + cu.ID)
	if !found {
		return errBadRequest("Passkey setup timed out, please try again")
	}
	u, err := s.loadWAUser(r.Context(), cu.ID)
	if err != nil {
		return err
	}
	cred, err := s.wa.FinishRegistration(u, sess, r)
	if err != nil {
		return errBadRequest("Passkey could not be verified")
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		name = deviceName(r.UserAgent())
	}
	data, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO passkeys (credential_id, user_id, name, data) VALUES ($1, $2, $3, $4)`,
		cred.ID, cu.ID, truncate(name, 60), data); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) passkeyLoginBegin(w http.ResponseWriter, r *http.Request) error {
	if s.limiter.Blocked("login:" + clientIP(r)) {
		return errTooManyAttempts
	}
	opts, sess, err := s.wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationPreferred))
	if err != nil {
		return err
	}
	id := randomToken(16)
	s.ceremony.Put("login:"+id, *sess)
	return ok(w, map[string]any{"ceremonyId": id, "options": opts})
}

func (s *Server) passkeyLoginFinish(w http.ResponseWriter, r *http.Request) error {
	sess, found := s.ceremony.Take("login:" + r.URL.Query().Get("ceremony"))
	if !found {
		return errBadRequest("Sign-in timed out, please try again")
	}
	ctx := r.Context()
	var owner string
	lookup := func(rawID, userHandle []byte) (webauthn.User, error) {
		var uid string
		if err := s.db.QueryRow(ctx, `SELECT user_id FROM passkeys WHERE credential_id = $1`, rawID).Scan(&uid); err != nil {
			return nil, errors.New("unknown credential")
		}
		if !bytes.Equal([]byte(uid), userHandle) {
			return nil, errors.New("user handle mismatch")
		}
		owner = uid
		return s.loadWAUser(ctx, uid)
	}
	_, cred, err := s.wa.FinishPasskeyLogin(lookup, sess, r)
	if err != nil {
		s.limiter.Fail("login:" + clientIP(r))
		return errUnauthorized("Passkey not recognized")
	}
	// Persist the updated sign counter / flags.
	if data, err := json.Marshal(cred); err == nil {
		_, err = s.db.Exec(ctx, `UPDATE passkeys SET data = $2, last_used_at = now() WHERE credential_id = $1`, cred.ID, data)
		logErr("update passkey", err)
	}
	if err := s.startSession(ctx, w, r, owner); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) listPasskeys(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.db.Query(r.Context(), `SELECT credential_id, name, created_at, last_used_at FROM passkeys
		WHERE user_id = $1 ORDER BY created_at`, currentUser(r).ID)
	if err != nil {
		return err
	}
	type passkey struct {
		ID         string     `json:"id"`
		Name       string     `json:"name"`
		CreatedAt  time.Time  `json:"createdAt"`
		LastUsedAt *time.Time `json:"lastUsedAt"`
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (passkey, error) {
		var p passkey
		var id []byte
		err := row.Scan(&id, &p.Name, &p.CreatedAt, &p.LastUsedAt)
		p.ID = base64.RawURLEncoding.EncodeToString(id)
		return p, err
	})
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (s *Server) deletePasskey(w http.ResponseWriter, r *http.Request) error {
	id, err := base64.RawURLEncoding.DecodeString(chi.URLParam(r, "id"))
	if err != nil {
		return errBadRequest("Invalid passkey id")
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM passkeys WHERE credential_id = $1 AND user_id = $2`, id, currentUser(r).ID); err != nil {
		return err
	}
	return noContent(w)
}

// deviceName gives a passkey a recognizable default label from the user agent.
func deviceName(ua string) string {
	switch {
	case strings.Contains(ua, "iPhone"):
		return "iPhone"
	case strings.Contains(ua, "iPad"):
		return "iPad"
	case strings.Contains(ua, "Android"):
		return "Android phone"
	case strings.Contains(ua, "Macintosh"):
		return "Mac"
	case strings.Contains(ua, "Windows"):
		return "Windows PC"
	case strings.Contains(ua, "Linux"):
		return "Linux PC"
	}
	return "Passkey"
}
