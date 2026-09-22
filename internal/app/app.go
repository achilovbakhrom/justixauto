// Package app is the composition root: it builds every module and wires the
// ports between them. cmd/api and the tests use the same wiring.
package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/modules/commerce"
	"justixauto/internal/modules/identity"
	"justixauto/internal/modules/inventory"
	"justixauto/internal/platform/httpx"
	"justixauto/internal/platform/idempotency"
)

type Config struct {
	Cookie  identity.CookieConfig
	Session identity.SessionConfig
	MFAKey  []byte
	Now     func() time.Time // nil = time.Now
	Log     *slog.Logger
}

// New returns the HTTP server with all modules mounted under /api/v1.
func New(db *gorm.DB, cfg Config) (*echo.Echo, *identity.Module, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	identity.RegisterPermissions(inventory.Permissions...)
	identity.RegisterPermissions(commerce.Permissions...)
	idm, err := identity.New(db, identity.Config{Cookie: cfg.Cookie, Session: cfg.Session, MFAKey: cfg.MFAKey, Now: cfg.Now})
	if err != nil {
		return nil, nil, err
	}
	e := httpx.NewServer(cfg.Log)
	// Every request: who is calling (session cookie, CSRF, Origin), then safe retries.
	api := e.Group("/api/v1", idm.Authenticate(), idempotency.Middleware(db, cfg.Now, "/identity/session/"))
	idm.Register(api)
	inv := inventory.New(db, cfg.Now)
	inv.Register(api)
	commerce.New(db, cfg.Now, directory{idm.Companies}, catalog{inv}).Register(api)
	return e, idm, nil
}

// directory adapts identity's company profiles to commerce's Directory port.
type directory struct{ companies *identity.CompanyService }

func (d directory) Company(ctx context.Context, id string) (*commerce.Company, error) {
	p, err := d.companies.CompanyProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	return &commerce.Company{ID: p.ID, Name: p.Name, Kind: string(p.Kind), Active: p.Access == identity.AccessActive,
		Country: p.Country}, nil
}

// catalog adapts inventory's model catalogue to commerce's Catalog port.
type catalog struct{ inventory *inventory.Module }

func (c catalog) Model(ctx context.Context, id string) (*commerce.Model, error) {
	m, err := c.inventory.Model(ctx, id)
	if err != nil {
		return nil, err
	}
	return &commerce.Model{ID: m.Model.ID, Name: m.Model.Make + " " + m.Model.Model + " " + m.Model.Variant}, nil
}
