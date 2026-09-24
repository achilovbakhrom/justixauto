// Package service holds inventory's business rules. It imports model and
// internal/pkg only; it must never import echo, gorm, repository or handler.
package service

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/modules/inventory/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
)

type Deps struct {
	store    Store
	now      func() time.Time
	branches Branches
}

// NewDeps builds the shared dependencies every inventory service embeds.
func NewDeps(store Store, now func() time.Time, branches Branches) Deps {
	return Deps{store: store, now: now, branches: branches}
}

// NewModel builds the vehicle model catalogue service.
func NewModel(d Deps) *Model { return &Model{d} }

// NewWarehouse builds the warehouse and receipt batch capacity service.
func NewWarehouse(d Deps) *Warehouse { return &Warehouse{d} }

// NewReceipt builds the receiving and VIN-identification service.
func NewReceipt(d Deps) *Receipt { return &Receipt{d} }

// NewVehicle builds the vehicle unit listing and movement service.
func NewVehicle(d Deps) *Vehicle { return &Vehicle{d} }

// NewStock builds the reservation and hand-over service used by other modules.
func NewStock(d Deps) *Stock { return &Stock{d} }

func (d Deps) clock() time.Time { return d.now().UTC() }

func (d Deps) fact(p *auth.Principal, factType string, vehicleID, warehouseID, batchID *string, occurredAt time.Time, reason string, details map[string]any) model.Fact {
	raw, _ := json.Marshal(details)
	if details == nil {
		raw = []byte("{}")
	}
	return model.Fact{
		ID: uuid.NewString(), CompanyID: p.CompanyID, FactType: factType, VehicleID: vehicleID,
		WarehouseID: warehouseID, ReceiptBatchID: batchID, ActorUserID: p.UserID,
		OccurredAt: occurredAt, RecordedAt: d.clock(), Reason: reason, Details: raw,
	}
}

func ptr[T any](v T) *T { return &v }

func vinUnavailable(vins []string) error {
	return apperr.New(apperr.ErrConflict, "VIN_UNAVAILABLE", "VIN already registered: "+strings.Join(vins, ", "))
}
