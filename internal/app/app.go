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
	"justixauto/internal/modules/retail"
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
	identity.RegisterPermissions(retail.Permissions...)
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
	commerce.New(db, cfg.Now, directory{idm.Companies}, catalog{inv}, commerceStock{inv.Stock()}).Register(api)
	ret := retail.New(db, cfg.Now, idm.Companies, retailStock{inv.Stock()}, pendingInsurance{})
	ret.Register(api)
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

// commerceStock adapts inventory reservations to commerce's Stock port;
// commerce orders hold vehicles as "commerce-order".
type commerceStock struct{ s *inventory.StockService }

func (a commerceStock) holder(orderID string) inventory.Holder {
	return inventory.Holder{Type: "commerce-order", ID: orderID}
}

func (a commerceStock) Vehicle(ctx context.Context, companyID, id string) (*commerce.StockVehicle, error) {
	v, err := a.s.Vehicle(ctx, companyID, id)
	if err != nil {
		return nil, err
	}
	return &commerce.StockVehicle{ID: v.ID, VIN: v.VIN, ModelID: v.ModelID}, nil
}

func (a commerceStock) Reserve(ctx context.Context, companyID, orderID string, vehicleIDs []string) error {
	return a.s.Reserve(ctx, companyID, a.holder(orderID), vehicleIDs)
}

func (a commerceStock) Release(ctx context.Context, orderID string, vehicleIDs []string, reason string) error {
	return a.s.Release(ctx, a.holder(orderID), vehicleIDs, reason)
}

func (a commerceStock) Transfer(ctx context.Context, orderID string, vehicleIDs []string, toCompanyID, toWarehouseID, actorID string, at time.Time) error {
	return a.s.Transfer(ctx, inventory.Handover{Holder: a.holder(orderID), VehicleIDs: vehicleIDs,
		ToCompanyID: toCompanyID, ToWarehouseID: toWarehouseID, ActorUserID: actorID, At: at})
}

// retailStock adapts inventory to retail's Stock port; sales hold vehicles
// as "retail-deal".
type retailStock struct{ s *inventory.StockService }

func (a retailStock) holder(dealID string) inventory.Holder {
	return inventory.Holder{Type: "retail-deal", ID: dealID}
}

func (a retailStock) Vehicle(ctx context.Context, companyID, id string) (*retail.Vehicle, error) {
	v, err := a.s.Vehicle(ctx, companyID, id)
	if err != nil {
		return nil, err
	}
	return &retail.Vehicle{ID: v.ID, VIN: v.VIN, ModelID: v.ModelID, Owned: v.OwnerCompanyID == companyID, InWarehouse: v.WarehouseID != ""}, nil
}

func (a retailStock) Reserve(ctx context.Context, companyID, dealID, vehicleID string) error {
	return a.s.Reserve(ctx, companyID, a.holder(dealID), []string{vehicleID})
}

func (a retailStock) Release(ctx context.Context, dealID, reason string) error {
	return a.s.Release(ctx, a.holder(dealID), nil, reason)
}

func (a retailStock) Deliver(ctx context.Context, dealID, vehicleID, actorID string, at time.Time) error {
	return a.s.Deliver(ctx, a.holder(dealID), vehicleID, actorID, at)
}

// pendingInsurance stands in until the insurance module exists: no deal is
// approved, so own-installment sales cannot be delivered yet.
type pendingInsurance struct{}

func (pendingInsurance) Approved(context.Context, string, string) (bool, error) { return false, nil }
