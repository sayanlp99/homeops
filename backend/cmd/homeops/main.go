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
	// `homeops pack <username|email> <pack> [household]` adds a recipe pack to that
	// person's household, as if they had pressed Add under Recipes → Recipe packs.
	if len(os.Args) > 1 && os.Args[1] == "pack" {
		if err := pack(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	// `homeops photos [--dry-run]` brings photos saved before the current sizes up to date.
	if len(os.Args) > 1 && os.Args[1] == "photos" {
		if err := photos(os.Args[2:]); err != nil {
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

func pack(args []string) error {
	if len(args) < 2 || len(args) > 3 {
		return errors.New("usage: homeops pack <username or email> <pack name> [household]")
	}
	household := ""
	if len(args) == 3 {
		household = args[2]
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
	added, skipped, err := api.ImportPack(ctx, db, args[0], household, args[1])
	if err != nil {
		return err
	}
	fmt.Printf("Added %d recipes; %d already there.\n", added, len(skipped))
	return nil
}

func photos(args []string) error {
	dry := len(args) == 1 && args[0] == "--dry-run"
	if len(args) > 1 || (len(args) == 1 && !dry) {
		return errors.New("usage: homeops photos [--dry-run]")
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
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	return api.RefreshPhotos(ctx, cfg, db, dry, func(f string, a ...any) { fmt.Printf(f+"\n", a...) })
}
