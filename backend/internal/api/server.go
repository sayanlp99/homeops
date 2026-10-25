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

	"homeops/internal/blob"
	"homeops/internal/config"
	"homeops/internal/mail"
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
	files    blob.Store
	mailer   mail.Sender
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
	if cfg.S3Endpoint != "" {
		s.files, err = blob.NewS3(ctx, blob.S3Config{Endpoint: cfg.S3Endpoint, Bucket: cfg.S3Bucket, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, UseSSL: cfg.S3UseSSL})
	} else {
		s.files, err = blob.NewDir(cfg.DataDir)
	}
	if err != nil {
		return nil, err
	}
	slog.Info("file storage", "backend", s.files.Name())
	s.mailer, err = mail.New(mail.Config{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUser, Password: cfg.SMTPPass, From: cfg.MailFrom})
	if err != nil {
		return nil, err
	}
	slog.Info("email", "smtp", cfg.SMTPHost != "", "from", cfg.MailFrom)
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
		r.Method("POST", "/auth/password/forgot", handler(s.forgotPassword))
		r.Method("GET", "/auth/password/reset/{token}", handler(s.checkReset))
		r.Method("POST", "/auth/password/reset", handler(s.resetPassword))

		// Signed in, whether or not the person belongs to a household yet.
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)

			r.Method("PATCH", "/me", handler(s.updateMe))
			r.Method("POST", "/me/password", handler(s.changePassword))
			r.Method("GET", "/me/passkeys", handler(s.listPasskeys))
			r.Method("DELETE", "/me/passkeys/{id}", handler(s.deletePasskey))
			r.Method("POST", "/me/passkeys/begin", handler(s.passkeyRegisterBegin))
			r.Method("POST", "/me/passkeys/finish", handler(s.passkeyRegisterFinish))
			r.Method("GET", "/me/sessions", handler(s.listSessions))
			r.Method("DELETE", "/me/sessions/{id}", handler(s.revokeSession))
			r.Method("POST", "/me/household", handler(s.switchHousehold))
			r.Method("POST", "/invites/{code}/join", handler(s.joinWithInvite))

			r.Method("POST", "/push/subscribe", handler(s.pushSubscribe))
			r.Method("POST", "/push/unsubscribe", handler(s.pushUnsubscribe))
			r.Method("POST", "/push/test", handler(s.pushTest))
			r.Method("GET", "/push/devices", handler(s.pushDevices))

			r.Route("/admin", func(r chi.Router) {
				r.Use(platformAdminOnly)
				r.Method("GET", "/households", handler(s.adminListHouseholds))
				r.Method("POST", "/households", handler(s.adminCreateHousehold))
				r.Method("DELETE", "/households/{id}", handler(s.adminDeleteHousehold))
				r.Method("POST", "/households/{id}/invites", handler(s.adminInvite))
				r.Method("GET", "/users", handler(s.adminListUsers))
				r.Method("PATCH", "/users/{id}", handler(s.adminUpdateUser))
			})
		})

		// Household data. Viewers are limited to reads.
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth, s.requireHousehold)
			r.Method("GET", "/events", http.HandlerFunc(s.events))

			r.With(requireRole("admin")).Method("PATCH", "/household", handler(s.updateHousehold))
			r.With(requireRole("owner")).Method("DELETE", "/household", handler(s.deleteHousehold))
			r.With(requireRole("admin")).Method("GET", "/invites", handler(s.listInvites))
			r.With(requireRole("admin")).Method("POST", "/invites", handler(s.createInvite))
			r.With(requireRole("admin")).Method("DELETE", "/invites/{code}", handler(s.revokeInvite))
			r.With(requireRole("admin")).Method("PATCH", "/members/{id}", handler(s.updateMember))
			r.Method("DELETE", "/members/{id}", handler(s.removeMember))

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

			r.Method("POST", "/files", handler(s.uploadFile))
			r.Method("GET", "/files/{id}", http.HandlerFunc(s.serveFile))

			r.Method("GET", "/recipes", handler(s.listRecipes))
			r.Method("POST", "/recipes", handler(s.saveRecipe))
			r.Method("POST", "/recipes/import", handler(s.importRecipe))
			r.Method("GET", "/recipes/packs", handler(s.listPacks))
			r.Method("POST", "/recipes/packs/{name}", handler(s.importPack))
			r.Method("GET", "/recipes/{id}", handler(s.getRecipe))
			r.Method("PUT", "/recipes/{id}", handler(s.saveRecipe))
			r.Method("PATCH", "/recipes/{id}", handler(s.patchRecipe))
			r.Method("DELETE", "/recipes/{id}", handler(s.deleteRecipe))
			r.Method("GET", "/ingredients", handler(s.listIngredients))
			r.Method("PATCH", "/ingredients/{id}", handler(s.updateIngredient))
			r.Method("POST", "/ingredients/parse", handler(s.parseIngredients))

			r.With(requireRole("member")).Method("GET", "/nutrition/profiles", handler(s.listHealth))
			r.Method("PUT", "/nutrition/profiles/{userId}", handler(s.saveHealth))
			r.Method("DELETE", "/nutrition/profiles/{userId}", handler(s.deleteHealth))
			r.Method("GET", "/nutrition/foods", handler(s.listFoods))
			r.With(requireRole("member")).Method("GET", "/nutrition", handler(s.nutritionReport))

			r.Method("GET", "/meals", handler(s.listMeals))
			r.Method("POST", "/meals", handler(s.createMeal))
			r.Method("GET", "/meals/suggest", handler(s.suggestMeals))
			r.Method("POST", "/meals/autofill", handler(s.autofillMeals))
			r.Method("PATCH", "/meals/{id}", handler(s.updateMeal))
			r.Method("DELETE", "/meals/{id}", handler(s.deleteMeal))

			r.Method("GET", "/notes", handler(s.listNotes))
			r.Method("POST", "/notes", handler(s.createNote))
			r.Method("PATCH", "/notes/{id}", handler(s.updateNote))
			r.Method("PATCH", "/notes/{id}/items/{item}", handler(s.tickNoteItem))
			r.Method("DELETE", "/notes/{id}", handler(s.deleteNote))

			r.Method("GET", "/grocery", handler(s.listGrocery))
			r.Method("POST", "/grocery", handler(s.addGrocery))
			r.Method("DELETE", "/grocery", handler(s.clearGrocery))
			r.Method("POST", "/grocery/generate", handler(s.generateGrocery))
			r.Method("POST", "/grocery/complete", handler(s.completeShopping))
			r.Method("GET", "/grocery/trips", handler(s.listTrips))
			r.Method("PATCH", "/grocery/{id}", handler(s.updateGrocery))
			r.Method("DELETE", "/grocery/{id}", handler(s.deleteGrocery))

			r.Method("GET", "/pantry", handler(s.listPantry))
			r.Method("POST", "/pantry", handler(s.upsertPantry))
			r.Method("POST", "/pantry/restock", handler(s.restockPantry))
			r.Method("PATCH", "/pantry/{id}", handler(s.updatePantry))
			r.Method("DELETE", "/pantry/{id}", handler(s.deletePantry))

			r.Route("/money", func(r chi.Router) {
				r.Method("GET", "/categories", handler(s.listMoneyCategories))
				r.Method("POST", "/categories", handler(s.saveMoneyCategory))
				r.Method("PATCH", "/categories/{id}", handler(s.saveMoneyCategory))
				r.Method("PUT", "/categories/{id}/months/{month}", handler(s.setMonthBudget))
				r.Method("GET", "/accounts", handler(s.listAccounts))
				r.Method("POST", "/accounts", handler(s.saveAccount))
				r.Method("PATCH", "/accounts/{id}", handler(s.saveAccount))
				r.Method("GET", "/transactions", handler(s.listTransactions))
				r.Method("POST", "/transactions", handler(s.createTransaction))
				r.Method("PATCH", "/transactions/{id}", handler(s.updateTransaction))
				r.Method("DELETE", "/transactions/{id}", handler(s.deleteTransaction))
				r.Method("GET", "/summary", handler(s.moneySummary))
				r.Method("GET", "/classify", handler(s.classifyEndpoint))
				r.Method("GET", "/rules", handler(s.listRules))
				r.Method("DELETE", "/rules/{id}", handler(s.deleteRule))
				r.Method("POST", "/import/preview", handler(s.importPreview))
				r.Method("POST", "/import/commit", handler(s.importCommit))
				r.Method("GET", "/imports", handler(s.listImports))
				r.Method("POST", "/invoice/preview", handler(s.invoicePreview))
				r.Method("POST", "/invoice/commit", handler(s.invoiceCommit))
				r.Method("DELETE", "/imports/{id}", handler(s.undoImport))
				r.Method("GET", "/bills", handler(s.listBills))
				r.Method("POST", "/bills", handler(s.saveBill))
				r.Method("PATCH", "/bills/{id}", handler(s.saveBill))
				r.Method("DELETE", "/bills/{id}", handler(s.deleteBill))
				r.Method("POST", "/bills/{id}/pay", handler(s.payBill))
			})

			r.Method("GET", "/retros", handler(s.listRetros))
			r.Method("GET", "/retro", handler(s.getRetro))
			r.Method("POST", "/retros/{id}/cards", handler(s.addRetroCard))
			r.Method("POST", "/retros/{id}/reveal", handler(s.revealRetro))
			r.Method("POST", "/retros/{id}/close", handler(s.closeRetro))
			r.Method("PATCH", "/retro-cards/{id}", handler(s.updateRetroCard))
			r.Method("DELETE", "/retro-cards/{id}", handler(s.deleteRetroCard))
			r.Method("POST", "/retro-cards/{id}/vote", handler(s.voteRetroCard))
			r.Method("GET", "/metrics/burndown", handler(s.burndown))
			r.Method("GET", "/metrics/team", handler(s.teamMetrics))
			r.Method("POST", "/kudos", handler(s.giveKudos))

			r.Method("GET", "/notifications", handler(s.listNotifications))
			r.Method("POST", "/notifications/read", handler(s.markNotificationsRead))
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
