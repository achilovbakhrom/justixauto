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

	"github.com/labstack/echo/v4"

	"justixauto/internal/app"
	"justixauto/internal/config"
	"justixauto/internal/modules/documents"
	"justixauto/internal/modules/identity"
	"justixauto/internal/pkg/apidocs"
	"justixauto/internal/pkg/database"
	"justixauto/internal/pkg/httpx"
	"justixauto/internal/pkg/telemetry"
	"justixauto/internal/pkg/webui"
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "dev"

// OpenAPI general info for swag (make openapi); handlers carry per-route annotations.
//
//	@title						JustixAuto API
//	@version					1
//	@description				Envelopes and errors: docs/justix-auto/contracts/http-domain.md. Sign in with POST /identity/session/login (session cookie); state-changing requests also send the X-CSRF-Token returned by GET /identity/session, and authenticated POSTs an Idempotency-Key.
//	@BasePath					/api/v1
//	@securityDefinitions.apikey	CSRF
//	@in							header
//	@name						X-CSRF-Token
func main() {
	log := telemetry.Logger()
	slog.SetDefault(log)
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
	var drain httpx.Drain
	stopTelemetry, err := telemetry.Setup(context.Background(), version)
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = stopTelemetry(ctx)
	}()
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
		files, err = documents.NewS3Storage(context.Background(), documents.S3Config{
			Bucket: cfg.S3.Bucket, Region: cfg.S3.Region,
			Prefix: cfg.S3.Prefix, Endpoint: cfg.S3.Endpoint, PathStyle: cfg.S3.PathStyle, SSE: cfg.S3.SSE,
		})
		if err != nil {
			return err
		}
	}
	e, _, err := app.New(db, app.Config{
		Files:       files,
		Cookie:      identity.CookieConfig{Secure: cfg.CookieSecure, AllowedOrigins: cfg.AllowedOrigins},
		Session:     identity.DefaultSessionConfig,
		MFAKey:      cfg.MFAKey,
		MFADisabled: cfg.MFADisabled,
		Log:         log,
		Middleware:  append([]echo.MiddlewareFunc{drain.Middleware}, telemetry.HTTP(log)...),
	})
	if err != nil {
		return err
	}
	if cfg.APIDocs {
		apidocs.Mount(e)
	}
	if cfg.WebDir != "" {
		webui.Mount(e, webui.Apps(cfg.WebDir))
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
	// Kubernetes stops routing to a terminating pod a moment after SIGTERM;
	// keep serving briefly so no request hits a closed listener.
	log.Info("shutting down", "drain", cfg.ShutdownDrain)
	drain.Start()
	time.Sleep(cfg.ShutdownDrain)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return e.Shutdown(shutdownCtx)
}
