// Package commerce owns B2B trade between companies: partnerships, offers,
// quotations, orders, shipments and invoices. Other companies are known only
// through the Directory port.
//
// This package is the only one other code imports; internal/modules/commerce
// splits into model (GORM models, enums, value types), repository (GORM
// persistence), service (business rules) and handler (Echo routes) layers,
// wired together here.
package commerce

import (
	"context"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/modules/commerce/handler"
	"justixauto/internal/modules/commerce/model"
	"justixauto/internal/modules/commerce/repository"
	"justixauto/internal/modules/commerce/service"
)

const (
	PermRead               = model.PermRead
	PermPartnershipsManage = model.PermPartnershipsManage
	PermOffersManage       = model.PermOffersManage
	PermTrade              = model.PermTrade
	PermPaymentsAccept     = model.PermPaymentsAccept
)

// Permissions are registered in the identity catalog at startup.
var Permissions = model.Permissions

// Company is what commerce needs to know about another company.
type Company = service.Company

// Files shares uploaded files with another company (implemented by documents).
type Files = service.Files

// Directory looks up companies (implemented by the identity module).
type Directory = service.Directory

// Model is what commerce needs to know about a catalogue model.
type Model = service.Model

// Catalog looks up vehicle models (implemented by the inventory module).
type Catalog = service.Catalog

// StockVehicle is what commerce needs to know about a concrete vehicle.
type StockVehicle = service.StockVehicle

// Stock reserves and hands over vehicles (implemented by inventory).
type Stock = service.Stock

// storeAdapter bridges repository.Store (which cannot import service, since
// repository implements service's interfaces structurally) to service.Store,
// whose InTx is self-referential and so needs the exact service.Store type.
type storeAdapter struct{ r *repository.Store }

func (a storeAdapter) Partnerships() service.PartnershipRepository { return a.r.Partnerships() }
func (a storeAdapter) Offers() service.OfferRepository             { return a.r.Offers() }
func (a storeAdapter) Deals() service.DealRepository               { return a.r.Deals() }
func (a storeAdapter) Fulfilment() service.FulfilmentRepository    { return a.r.Fulfilment() }
func (a storeAdapter) Invoices() service.InvoiceRepository         { return a.r.Invoices() }
func (a storeAdapter) Events() service.EventRepository             { return a.r.Events() }
func (a storeAdapter) Bind(ctx context.Context) context.Context    { return a.r.Bind(ctx) }

func (a storeAdapter) InTx(ctx context.Context, fn func(service.Store) error) error {
	return a.r.InTx(ctx, func(rs *repository.Store) error { return fn(storeAdapter{rs}) })
}

// Module wires commerce's layers and exposes the ports other modules use.
type Module struct{ handler *handler.Handler }

// New builds the commerce module.
func New(db *gorm.DB, now func() time.Time, directory Directory, catalog Catalog, stock Stock, files Files) *Module {
	if now == nil {
		now = time.Now
	}
	d := service.NewDeps(storeAdapter{repository.NewStore(db)}, directory, catalog, stock, files, now)
	offers := service.NewOffer(d)
	h := handler.New(service.NewPartnership(d), offers, service.NewDeal(d, offers), service.NewFulfilment(d), service.NewInvoice(d))
	return &Module{handler: h}
}

// Register mounts the commerce routes under /api/v1/commerce.
func (m *Module) Register(api *echo.Group) { m.handler.Routes(api.Group("/commerce")) }
