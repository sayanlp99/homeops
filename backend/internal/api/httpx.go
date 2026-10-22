package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type apiError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *apiError) Error() string { return e.Message }

func errBadRequest(msg string) error   { return &apiError{http.StatusBadRequest, "bad_request", msg} }
func errUnauthorized(msg string) error { return &apiError{http.StatusUnauthorized, "unauthorized", msg} }
func errForbidden(msg string) error    { return &apiError{http.StatusForbidden, "forbidden", msg} }
func errNotFound(msg string) error     { return &apiError{http.StatusNotFound, "not_found", msg} }
func errConflict(msg string) error     { return &apiError{http.StatusConflict, "conflict", msg} }

// handler lets endpoints return errors instead of writing them by hand.
type handler func(w http.ResponseWriter, r *http.Request) error

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := h(w, r)
	if err == nil {
		return
	}
	var ae *apiError
	switch {
	case errors.As(err, &ae):
	case errors.Is(err, pgx.ErrNoRows):
		ae = &apiError{http.StatusNotFound, "not_found", "Not found"}
	default:
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			ae = &apiError{http.StatusConflict, "conflict", "That already exists"}
		} else if errors.As(err, &pgErr) && (pgErr.Code == "23503" || pgErr.Code == "23514" || pgErr.Code == "22P02") {
			ae = &apiError{http.StatusBadRequest, "bad_request", "Invalid value"}
		} else {
			slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
			ae = &apiError{http.StatusInternalServerError, "internal", "Something went wrong"}
		}
	}
	writeJSON(w, ae.Status, ae)
}

func writeJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, v any) error { return writeJSON(w, http.StatusOK, v) }

func noContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return errBadRequest("Invalid JSON body")
	}
	return nil
}
