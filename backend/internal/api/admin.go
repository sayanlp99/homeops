package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// Platform administration: the person running this HomeOps installation creates
// households and onboards their owners. They don't see household data.

type adminHousehold struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Timezone       string    `json:"timezone"`
	CreatedAt      time.Time `json:"createdAt"`
	Members        int       `json:"members"`
	Owners         []string  `json:"owners"`
	PendingInvites int       `json:"pendingInvites"`
	IsMember       bool      `json:"isMember"`
}

func (s *Server) adminListHouseholds(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	rows, _ := s.db.Query(r.Context(), `SELECT h.id, h.name, h.timezone, h.created_at,
			(SELECT count(*) FROM memberships m WHERE m.household_id = h.id)::int,
			coalesce((SELECT array_agg(x.display_name ORDER BY x.created_at) FROM members x WHERE x.household_id = h.id AND x.role = 'owner'), '{}'),
			(SELECT count(*) FROM invites i WHERE i.household_id = h.id AND i.used_at IS NULL AND i.expires_at > now())::int,
			EXISTS (SELECT 1 FROM memberships m WHERE m.household_id = h.id AND m.user_id = $1)
		FROM households h ORDER BY h.created_at`, u.ID)
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (adminHousehold, error) {
		var h adminHousehold
		err := row.Scan(&h.ID, &h.Name, &h.Timezone, &h.CreatedAt, &h.Members, &h.Owners, &h.PendingInvites, &h.IsMember)
		return h, err
	})
	if err != nil {
		return err
	}
	return ok(w, out)
}

// adminCreateHousehold makes a household owned by the given email: yourself, an
// existing account (added straight away), or a new person (sent an owner invite).
func (s *Server) adminCreateHousehold(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in struct {
		Name       string `json:"name"`
		Timezone   string `json:"timezone"`
		OwnerEmail string `json:"ownerEmail"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 60 {
		return errBadRequest("Household name is required (max 60 characters)")
	}
	if in.Timezone == "" {
		in.Timezone = "Asia/Kolkata"
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return errBadRequest("Unknown time zone")
	}
	ctx := r.Context()
	owner, ownerID := u.Email, u.ID
	if strings.TrimSpace(in.OwnerEmail) != "" {
		var err error
		if owner, err = normalizeEmail(in.OwnerEmail); err != nil {
			return err
		}
		if owner != u.Email {
			// An existing account is added directly; otherwise ownerID stays empty and they're invited.
			ownerID = ""
			err := s.db.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = $1`, owner).Scan(&ownerID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	hid, err := createHousehold(ctx, tx, in.Name, in.Timezone)
	if err != nil {
		return err
	}
	if ownerID != "" {
		if err := addMember(ctx, tx, hid, ownerID, "owner"); err != nil {
			return err
		}
		if ownerID == u.ID {
			// Keep the admin where they are; they can switch to it from the menu.
			if _, err := tx.Exec(ctx, `UPDATE users SET last_household_id = coalesce($2::uuid, last_household_id) WHERE id = $1`,
				u.ID, nullIfEmpty(u.HouseholdID)); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if err := s.ensureSprints(ctx, hid); err != nil {
		return err
	}
	res := map[string]any{"id": hid}
	switch {
	case ownerID == u.ID:
		res["owner"] = "you"
	case ownerID != "":
		res["owner"] = "added"
		logErr("send added email", s.mailer.Send(ctx, addedEmail(owner, u.DisplayName, in.Name, s.cfg.PublicURL)), "to", owner)
	default:
		inv, err := s.invite(ctx, u, hid, in.Name, owner, "owner")
		if err != nil {
			return err
		}
		res["owner"] = "invited"
		res["invite"] = inv
	}
	return ok(w, res)
}

func (s *Server) adminDeleteHousehold(w http.ResponseWriter, r *http.Request) error {
	var in struct{ ConfirmName string }
	if err := decode(r, &in); err != nil {
		return err
	}
	return s.deleteHouseholdByID(w, r, chi.URLParam(r, "id"), in.ConfirmName)
}

// adminInvite sends an invite into any household, e.g. to onboard a second owner.
func (s *Server) adminInvite(w http.ResponseWriter, r *http.Request) error {
	var in struct{ Email, Role string }
	if err := decode(r, &in); err != nil {
		return err
	}
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return err
	}
	if !validRole(in.Role) {
		return errBadRequest("Unknown role")
	}
	h, err := s.getHousehold(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotFound("Household not found")
	}
	if err != nil {
		return err
	}
	res, err := s.invite(r.Context(), currentUser(r), h.ID, h.Name, email, in.Role)
	if err != nil {
		return err
	}
	return ok(w, res)
}

type adminUser struct {
	ID              string            `json:"id"`
	Username        string            `json:"username"`
	DisplayName     string            `json:"displayName"`
	Email           string            `json:"email"`
	Color           string            `json:"color"`
	IsPlatformAdmin bool              `json:"isPlatformAdmin"`
	CreatedAt       time.Time         `json:"createdAt"`
	LastSeenAt      *time.Time        `json:"lastSeenAt"`
	Households      []adminMembership `json:"households"`
}

type adminMembership struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request) error {
	rows, _ := s.db.Query(r.Context(), `SELECT u.id, u.username, u.display_name, coalesce(u.email, ''), u.color, u.is_platform_admin, u.created_at,
			(SELECT max(last_seen_at) FROM sessions s WHERE s.user_id = u.id),
			coalesce((SELECT json_agg(json_build_object('id', h.id, 'name', h.name, 'role', m.role) ORDER BY h.name)
				FROM memberships m JOIN households h ON h.id = m.household_id WHERE m.user_id = u.id), '[]')
		FROM users u ORDER BY u.created_at`)
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (adminUser, error) {
		var a adminUser
		err := row.Scan(&a.ID, &a.Username, &a.DisplayName, &a.Email, &a.Color, &a.IsPlatformAdmin, &a.CreatedAt, &a.LastSeenAt, &a.Households)
		return a, err
	})
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (s *Server) adminUpdateUser(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	id := chi.URLParam(r, "id")
	var in struct {
		Email           *string `json:"email"`
		IsPlatformAdmin *bool   `json:"isPlatformAdmin"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	if in.Email != nil {
		email, err := normalizeEmail(*in.Email)
		if err != nil {
			return err
		}
		if taken, err := s.emailTaken(ctx, email, id); err != nil {
			return err
		} else if taken {
			return errConflict("Another account already uses that email")
		}
		if _, err := s.db.Exec(ctx, `UPDATE users SET email = $2 WHERE id::text = $1`, id, email); err != nil {
			return err
		}
	}
	if in.IsPlatformAdmin != nil {
		if id == u.ID && !*in.IsPlatformAdmin {
			return errBadRequest("You can't remove your own administrator access")
		}
		if _, err := s.db.Exec(ctx, `UPDATE users SET is_platform_admin = $2 WHERE id::text = $1`, id, *in.IsPlatformAdmin); err != nil {
			return err
		}
	}
	return noContent(w)
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
