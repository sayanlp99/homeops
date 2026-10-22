package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // household time zones work even in a minimal container

	"homeops/internal/api"
	"homeops/internal/config"
	"homeops/internal/store"
)

func main() {
	// `homeops healthcheck` is used by Docker, since the runtime image has no curl.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		resp, err := (&http.Client{Timeout: 3 * time.Second}).Get("http://127.0.0.1:8080/api/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := store.Migrate(ctx, db); err != nil {
		return err
	}

	srv, err := api.New(ctx, cfg, db)
	if err != nil {
		return err
	}
	srv.Run(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdownCtx)
	}()
	slog.Info("HomeOps listening", "addr", cfg.Addr, "publicURL", cfg.PublicURL)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
