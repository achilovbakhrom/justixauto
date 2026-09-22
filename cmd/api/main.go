// Command api runs the JustixAuto HTTP API. It is the composition root: all
// modules are constructed and wired here.
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

	"justixauto/internal/config"
	"justixauto/internal/modules/identity"
	"justixauto/internal/platform/database"
	"justixauto/internal/platform/httpx"
	"justixauto/internal/platform/idempotency"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("api stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	idm, err := identity.New(db, identity.Config{
		Cookie:  identity.CookieConfig{Secure: cfg.CookieSecure, AllowedOrigins: cfg.AllowedOrigins},
		Session: identity.DefaultSessionConfig,
		MFAKey:  cfg.MFAKey,
	})
	if err != nil {
		return err
	}
	e := httpx.NewServer(log)
	// Every request: who is calling (session cookie, CSRF, Origin), then safe retries.
	api := e.Group("/api/v1", idm.Authenticate(), idempotency.Middleware(db, time.Now, "/identity/session/"))
	idm.Register(api)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serveErr := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr)
		serveErr <- e.Start(cfg.HTTPAddr)
	}()
	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return e.Shutdown(shutdownCtx)
}
