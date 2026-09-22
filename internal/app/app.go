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
	"justixauto/internal/modules/documents"
	"justixauto/internal/modules/financing"
	"justixauto/internal/modules/identity"
	"justixauto/internal/modules/insurance"
	"justixauto/internal/modules/inventory"
	"justixauto/internal/modules/retail"
	"justixauto/internal/platform/httpx"
	"justixauto/internal/platform/idempotency"
)

type Config struct {
	// Files stores uploaded documents (S3 in production, a directory locally).
	Files   documents.Storage
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
	identity.RegisterPermissions(insurance.Permissions...)
	identity.RegisterPermissions(financing.Permissions...)
	identity.RegisterPermissions(documents.Permissions...)
	idm, err := identity.New(db, identity.Config{Cookie: cfg.Cookie, Session: cfg.Session, MFAKey: cfg.MFAKey, Now: cfg.Now})
	if err != nil {
		return nil, nil, err
	}
	e := httpx.NewServer(cfg.Log)
	// Every request: who is calling (session cookie, CSRF, Origin), then safe retries.
	api := e.Group("/api/v1", idm.Authenticate(), idempotency.Middleware(db, cfg.Now, "/identity/session/"))
	idm.Register(api)
	docs := documents.New(db, cfg.Now, cfg.Files)
	docs.Register(api)
	files := fileShares{docs.Service}
	inv := inventory.New(db, cfg.Now, idm.Companies)
	inv.Register(api)
	commerce.New(db, cfg.Now, directory{idm.Companies}, catalog{inv}, commerceStock{inv.Stock()}, files).Register(api)
	// Retail and insurance each ask the other a question (approval / sale
	// facts); the approval adapter is bound once insurance exists.
	approvals := &insuranceApprovals{}
	ret := retail.New(db, cfg.Now, idm.Companies, retailStock{inv.Stock()}, approvals, files)
	ret.Register(api)
	ins := insurance.New(db, cfg.Now, insuranceSales{ret.Deals, inv}, insurerDirectory{idm.Companies})
	approvals.s = ins.Service
	ins.Register(api)
	financing.New(db, cfg.Now, financingSales{ret.Deals, inv}, providerDirectory{idm.Companies}, files).Register(api)
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

// insuranceApprovals adapts insurance decisions to retail's Insurance port.
type insuranceApprovals struct{ s *insurance.Service }

func (a *insuranceApprovals) Approved(ctx context.Context, companyID, dealID string) (bool, error) {
	return a.s.Approved(ctx, companyID, dealID)
}

// insuranceSales adapts retail sales to insurance's Sales port.
type insuranceSales struct {
	deals     *retail.DealService
	inventory *inventory.Module
}

func (a insuranceSales) Sale(ctx context.Context, companyID, dealID string) (*insurance.Sale, error) {
	d, err := a.deals.Info(ctx, companyID, dealID)
	if err != nil {
		return nil, err
	}
	v, err := saleVehicle(ctx, a.inventory, companyID, d.VehicleID)
	if err != nil {
		return nil, err
	}
	return &insurance.Sale{ID: d.ID, VehicleID: d.VehicleID, PaymentScheme: d.PaymentScheme, Status: d.Status, Price: d.Price, Revision: d.Revision,
		VIN: v.vin, Model: v.model, CustomerName: d.CustomerName}, nil
}

// insurerDirectory adapts identity company profiles to insurance's Directory port.
type insurerDirectory struct{ companies *identity.CompanyService }

func (a insurerDirectory) Company(ctx context.Context, id string) (*insurance.Company, error) {
	p, err := a.companies.CompanyProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	return &insurance.Company{ID: p.ID, Name: p.Name, Kind: string(p.Kind), Active: p.Access == identity.AccessActive}, nil
}

// financingSales adapts retail sales to financing's Sales port.
type financingSales struct {
	deals     *retail.DealService
	inventory *inventory.Module
}

func (a financingSales) Sale(ctx context.Context, companyID, dealID string) (*financing.Sale, error) {
	d, err := a.deals.Info(ctx, companyID, dealID)
	if err != nil {
		return nil, err
	}
	v, err := saleVehicle(ctx, a.inventory, companyID, d.VehicleID)
	if err != nil {
		return nil, err
	}
	return &financing.Sale{ID: d.ID, VehicleID: d.VehicleID, PaymentScheme: d.PaymentScheme, Status: d.Status, Price: d.Price, Revision: d.Revision,
		VIN: v.vin, Model: v.model, CustomerName: d.CustomerName}, nil
}

type vehicleFacts struct{ vin, model string }

// saleVehicle reads the VIN and model name of a sold vehicle for provider snapshots.
func saleVehicle(ctx context.Context, inv *inventory.Module, companyID, vehicleID string) (vehicleFacts, error) {
	v, err := inv.Stock().Vehicle(ctx, companyID, vehicleID)
	if err != nil {
		return vehicleFacts{}, err
	}
	m, err := inv.Model(ctx, v.ModelID)
	if err != nil {
		return vehicleFacts{}, err
	}
	return vehicleFacts{vin: v.VIN, model: m.Model.Make + " " + m.Model.Model + " " + m.Model.Variant}, nil
}

// providerDirectory adapts identity company profiles to financing's Directory port.
type providerDirectory struct{ companies *identity.CompanyService }

func (a providerDirectory) Company(ctx context.Context, id string) (*financing.Company, error) {
	p, err := a.companies.CompanyProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	return &financing.Company{ID: p.ID, Name: p.Name, Kind: string(p.Kind), Active: p.Access == identity.AccessActive}, nil
}

// fileShares adapts the documents module to other modules' Files ports.
type fileShares struct{ s *documents.Service }

func (a fileShares) Share(ctx context.Context, ownerCompanyID, fileID, withCompanyID, resourceType, resourceID string) error {
	_, err := a.s.Share(ctx, ownerCompanyID, fileID, withCompanyID, resourceType, resourceID)
	return err
}
