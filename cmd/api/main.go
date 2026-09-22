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

	"justixauto/internal/app"
	"justixauto/internal/config"
	"justixauto/internal/modules/documents"
	"justixauto/internal/modules/identity"
	"justixauto/internal/platform/database"
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

	var files documents.Storage = documents.DirStorage{Root: cfg.DocumentsDir}
	if cfg.FileStorage == "s3" {
		files, err = documents.NewS3Storage(context.Background(), documents.S3Config{Bucket: cfg.S3.Bucket, Region: cfg.S3.Region,
			Prefix: cfg.S3.Prefix, Endpoint: cfg.S3.Endpoint, PathStyle: cfg.S3.PathStyle, SSE: cfg.S3.SSE})
		if err != nil {
			return err
		}
	}
	e, _, err := app.New(db, app.Config{
		Files:   files,
		Cookie:  identity.CookieConfig{Secure: cfg.CookieSecure, AllowedOrigins: cfg.AllowedOrigins},
		Session: identity.DefaultSessionConfig,
		MFAKey:  cfg.MFAKey,
		Log:     log,
	})
	if err != nil {
		return err
	}

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
