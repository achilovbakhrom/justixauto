// Package inventory owns the vehicle model catalogue, physical vehicles
// (one per globally unique VIN), warehouses, receipt batches and placements.
// Companies and users are referenced by ID only.
//
// This package is the only one other code imports; internal/modules/inventory
// splits into model (GORM models, filters, value types), repository (GORM
// persistence), service (business rules) and handler (Echo routes) layers,
// wired together here.
package inventory

import (
	"context"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/modules/inventory/handler"
	"justixauto/internal/modules/inventory/model"
	"justixauto/internal/modules/inventory/repository"
	"justixauto/internal/modules/inventory/service"
)

// Holder identifies the deal that holds vehicles.
type Holder = model.Holder

// Handover is the physical and legal hand-over of held vehicles to another
// company, received into one of its warehouses.
type Handover = service.Handover

// ModelDetail is a model with its current and (optionally) all specifications.
type ModelDetail = service.ModelDetail

// StockService is used by other modules (through their ports) to reserve,
// release and hand over vehicles.
type StockService = service.Stock

// Branches answers whether a branch belongs to a company (implemented by identity).
type Branches = service.Branches

// storeAdapter bridges repository.Store (which cannot import service, since
// repository implements service's interfaces structurally) to service.Store,
// whose InTx is self-referential and so needs the exact service.Store type.
type storeAdapter struct{ r *repository.Store }

func (a storeAdapter) Models() service.ModelRepository             { return a.r.Models() }
func (a storeAdapter) Warehouses() service.WarehouseRepository     { return a.r.Warehouses() }
func (a storeAdapter) Vehicles() service.VehicleRepository         { return a.r.Vehicles() }
func (a storeAdapter) Facts() service.FactRepository               { return a.r.Facts() }
func (a storeAdapter) Reservations() service.ReservationRepository { return a.r.Reservations() }

func (a storeAdapter) InTx(ctx context.Context, fn func(service.Store) error) error {
	return a.r.InTx(ctx, func(rs *repository.Store) error { return fn(storeAdapter{rs}) })
}

// Module wires inventory's layers and exposes the ports other modules use.
type Module struct {
	handler *handler.Handler
	models  *service.Model
	stock   *StockService
}

// New builds the inventory module. branches answers whether a branch
// belongs to a company (implemented by identity).
func New(db *gorm.DB, now func() time.Time, branches Branches) *Module {
	if now == nil {
		now = time.Now
	}
	st := storeAdapter{repository.NewStore(db)}
	d := service.NewDeps(st, now, branches)
	models, warehouses := service.NewModel(d), service.NewWarehouse(d)
	receipts, vehicles := service.NewReceipt(d), service.NewVehicle(d)
	return &Module{
		handler: handler.New(models, warehouses, receipts, vehicles),
		models:  models,
		stock:   service.NewStock(d),
	}
}

// Register mounts the inventory routes under /api/v1/inventory.
func (m *Module) Register(api *echo.Group) { m.handler.Routes(api.Group("/inventory")) }

// Model returns a catalogue model with its current specification (for other
// modules, through their own ports). Unknown IDs return apperr.ErrNotFound.
func (m *Module) Model(ctx context.Context, id string) (*ModelDetail, error) {
	return m.models.Get(ctx, id)
}

// Stock is the reservation and hand-over service for other modules.
func (m *Module) Stock() *StockService { return m.stock }
