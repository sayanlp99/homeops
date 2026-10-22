// Package api implements the HomeOps HTTP API, background scheduler and push delivery.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5/pgxpool"

	"homeops/internal/config"
	"homeops/internal/realtime"
)

type Server struct {
	cfg      config.Config
	db       *pgxpool.Pool
	hub      *realtime.Hub
	wa       *webauthn.WebAuthn
	ceremony *ttlStore[webauthn.SessionData]
	limiter  *rateLimiter
	push     *pusher
	stopping <-chan struct{} // closed on shutdown so long-lived SSE streams end
}

func New(ctx context.Context, cfg config.Config, db *pgxpool.Pool) (*Server, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.RPID(),
		RPDisplayName: "HomeOps",
		RPOrigins:     []string{cfg.PublicURL},
	})
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:      cfg,
		db:       db,
		hub:      realtime.NewHub(),
		wa:       wa,
		ceremony: newTTLStore[webauthn.SessionData](5 * time.Minute),
		limiter:  newRateLimiter(10, 15*time.Minute),
	}
	s.push, err = newPusher(ctx, db, cfg.VAPIDEmail)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP, middleware.Recoverer, securityHeaders)

	r.Route("/api", func(r chi.Router) {
		r.Use(requireCSRFHeader)
		r.Method("GET", "/health", handler(func(w http.ResponseWriter, r *http.Request) error {
			return ok(w, map[string]string{"status": "ok"})
		}))
		r.Method("GET", "/bootstrap", s.optionalAuth(handler(s.bootstrap)))
		r.Method("POST", "/setup", handler(s.setup))
		r.Method("POST", "/auth/login", handler(s.login))
		r.Method("POST", "/auth/logout", handler(s.logout))
		r.Method("POST", "/auth/invite/accept", handler(s.acceptInvite))
		r.Method("GET", "/auth/invite/{code}", handler(s.checkInvite))
		r.Method("POST", "/auth/passkey/login/begin", handler(s.passkeyLoginBegin))
		r.Method("POST", "/auth/passkey/login/finish", handler(s.passkeyLoginFinish))

		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Method("GET", "/events", http.HandlerFunc(s.events))

			r.Method("PATCH", "/me", handler(s.updateMe))
			r.Method("POST", "/me/password", handler(s.changePassword))
			r.Method("GET", "/me/passkeys", handler(s.listPasskeys))
			r.Method("DELETE", "/me/passkeys/{id}", handler(s.deletePasskey))
			r.Method("POST", "/me/passkeys/begin", handler(s.passkeyRegisterBegin))
			r.Method("POST", "/me/passkeys/finish", handler(s.passkeyRegisterFinish))
			r.Method("GET", "/me/sessions", handler(s.listSessions))
			r.Method("DELETE", "/me/sessions/{id}", handler(s.revokeSession))

			r.Method("PATCH", "/household", s.adminOnly(handler(s.updateHousehold)))
			r.Method("POST", "/invites", s.adminOnly(handler(s.createInvite)))

			r.Method("GET", "/labels", handler(s.listLabels))
			r.Method("POST", "/labels", handler(s.createLabel))
			r.Method("PATCH", "/labels/{id}", handler(s.updateLabel))
			r.Method("DELETE", "/labels/{id}", handler(s.deleteLabel))

			r.Method("GET", "/tasks", handler(s.listTasks))
			r.Method("POST", "/tasks", handler(s.createTask))
			r.Method("PATCH", "/tasks/{id}", handler(s.updateTask))
			r.Method("DELETE", "/tasks/{id}", handler(s.deleteTask))

			r.Method("GET", "/sprints", handler(s.listSprints))
			r.Method("PATCH", "/sprints/{id}", handler(s.updateSprint))
			r.Method("POST", "/sprints/{id}/start", handler(s.startSprint))
			r.Method("POST", "/sprints/{id}/complete", handler(s.completeSprint))

			r.Method("GET", "/recurring", handler(s.listRecurring))
			r.Method("POST", "/recurring", handler(s.createRecurring))
			r.Method("PATCH", "/recurring/{id}", handler(s.updateRecurring))
			r.Method("DELETE", "/recurring/{id}", handler(s.deleteRecurring))

			r.Method("GET", "/notifications", handler(s.listNotifications))
			r.Method("POST", "/notifications/read", handler(s.markNotificationsRead))
			r.Method("POST", "/push/subscribe", handler(s.pushSubscribe))
			r.Method("POST", "/push/unsubscribe", handler(s.pushUnsubscribe))
			r.Method("POST", "/push/test", handler(s.pushTest))
		})

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusNotFound, &apiError{Code: "not_found", Message: "No such endpoint"})
		})
	})

	if s.cfg.StaticDir != "" {
		r.Handle("/*", spaHandler(s.cfg.StaticDir))
	}
	return r
}

// Run starts background workers; they stop when ctx is cancelled.
func (s *Server) Run(ctx context.Context) {
	s.stopping = ctx.Done()
	go s.push.run(ctx, s)
	go s.runScheduler(ctx)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// requireCSRFHeader rejects cross-site writes: browsers can't send custom headers
// cross-origin without a CORS preflight, which we never approve.
func requireCSRFHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-Requested-With") != "homeops" {
			writeJSON(w, http.StatusForbidden, &apiError{Code: "csrf", Message: "Missing request header"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP prefers Cloudflare's header since all traffic arrives via the tunnel.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}

func logErr(msg string, err error, args ...any) {
	if err != nil {
		slog.Error(msg, append(args, "err", err)...)
	}
}
