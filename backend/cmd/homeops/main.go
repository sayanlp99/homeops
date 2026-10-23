package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	// `homeops passwd <username|email>` reads a new password from stdin, for when
	// someone is locked out and password-reset email isn't set up.
	if len(os.Args) > 1 && os.Args[1] == "passwd" {
		if err := passwd(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
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

func passwd(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: homeops passwd <username or email>  (new password on stdin)")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return errors.New("no password given on stdin")
	}
	if err := api.ResetPassword(ctx, db, args[0], strings.TrimRight(line, "\r\n")); err != nil {
		return err
	}
	fmt.Printf("Password changed for %s; they've been signed out everywhere.\n", args[0])
	return nil
}
