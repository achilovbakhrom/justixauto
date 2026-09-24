// Package retail owns the seller's retail business: customers, the CRM
// (leads, contacts, tasks), vehicle listings and retail sales (deals) with
// their invoices, payment evidence, registration and delivery.
//
// This package is the only one other code imports; internal/modules/retail
// splits into model (GORM models, enums, value types), repository (GORM
// persistence), service (business rules) and handler (Echo routes) layers,
// wired together here.
package retail

import (
	"context"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/modules/retail/handler"
	"justixauto/internal/modules/retail/model"
	"justixauto/internal/modules/retail/repository"
	"justixauto/internal/modules/retail/service"
)

const (
	PermRead           = model.PermRead
	PermCRM            = model.PermCRM
	PermListings       = model.PermListings
	PermDeals          = model.PermDeals
	PermPaymentsAccept = model.PermPaymentsAccept
	PermDeliver        = model.PermDeliver
)

// Permissions are registered in the identity catalog at startup.
var Permissions = model.Permissions

// Company answers identity questions (implemented by identity).
type Company = service.Company

// Vehicle is what retail may know about a vehicle.
type Vehicle = service.Vehicle

// Stock reserves and delivers vehicles (implemented by inventory).
type Stock = service.Stock

// Files checks and shares uploaded files (implemented by documents).
type Files = service.Files

// Insurance tells whether the insurer approved the deal's application
// (implemented by the insurance module).
type Insurance = service.Insurance

// DealService manages retail sales; other modules read sale facts through
// its Info method (through their own ports).
type DealService = service.Deal

// storeAdapter bridges repository.Store (which cannot import service, since
// repository implements service's interfaces structurally) to service.Store,
// whose InTx is self-referential and so needs the exact service.Store type.
type storeAdapter struct{ r *repository.Store }

func (a storeAdapter) CRM() service.CRMRepository               { return a.r.CRM() }
func (a storeAdapter) Listings() service.ListingRepository      { return a.r.Listings() }
func (a storeAdapter) Deals() service.DealRepository            { return a.r.Deals() }
func (a storeAdapter) Events() service.EventRepository          { return a.r.Events() }
func (a storeAdapter) Bind(ctx context.Context) context.Context { return a.r.Bind(ctx) }

func (a storeAdapter) InTx(ctx context.Context, fn func(service.Store) error) error {
	return a.r.InTx(ctx, func(rs *repository.Store) error { return fn(storeAdapter{rs}) })
}

// Module wires retail's layers and exposes the ports other modules use.
type Module struct {
	handler *handler.Handler
	Deals   *DealService
}

// New builds the retail module.
func New(db *gorm.DB, now func() time.Time, company Company, stock Stock, insurance Insurance, files Files) *Module {
	if now == nil {
		now = time.Now
	}
	d := service.NewDeps(storeAdapter{repository.NewStore(db)}, company, stock, files, now)
	crm, listings, deals := service.NewCRM(d), service.NewListing(d), service.NewDeal(d, insurance)
	return &Module{handler: handler.New(crm, listings, deals), Deals: deals}
}

// Register mounts the retail routes under /api/v1/retail.
func (m *Module) Register(api *echo.Group) { m.handler.Routes(api.Group("/retail")) }
