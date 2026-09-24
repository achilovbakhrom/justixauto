package service

import (
	"context"
	"time"

	"justixauto/internal/modules/inventory/model"
)

// Store gives services the inventory repositories and one-transaction runs.
type Store interface {
	Models() ModelRepository
	Warehouses() WarehouseRepository
	Vehicles() VehicleRepository
	Facts() FactRepository
	Reservations() ReservationRepository
	InTx(ctx context.Context, fn func(Store) error) error
}

// Branches answers whether a branch belongs to a company (implemented by identity).
type Branches interface {
	BranchOf(ctx context.Context, companyID, branchID string) (bool, error)
}

type ModelRepository interface {
	Create(ctx context.Context, m *model.VehicleModel, spec *model.Specification) error
	Get(ctx context.Context, id string) (*model.VehicleModel, error)
	List(ctx context.Context, f model.ModelFilter) ([]model.VehicleModel, error)
	Specs(ctx context.Context, modelID string) ([]model.Specification, error)
	Spec(ctx context.Context, modelID string, version int) (*model.Specification, error)
	// AddSpec stores a new version and makes it current if the model version matches.
	AddSpec(ctx context.Context, m *model.VehicleModel, expected int64, spec *model.Specification) error
}

type WarehouseRepository interface {
	Create(ctx context.Context, w *model.Warehouse) error
	Get(ctx context.Context, companyID, id string) (*model.Warehouse, error)
	// Lock reads and row-locks a company's warehouse until the transaction ends.
	Lock(ctx context.Context, companyID, id string) (*model.Warehouse, error)
	List(ctx context.Context, f model.WarehouseFilter) ([]model.Warehouse, error)
	Update(ctx context.Context, w *model.Warehouse, expected int64) error
	// ByBranch returns the branch's main warehouse, if any.
	ByBranch(ctx context.Context, companyID, branchID string) (*model.Warehouse, error)
	// Touch bumps the version after stock in the warehouse changed.
	Touch(ctx context.Context, w *model.Warehouse, at time.Time) error
	// BumpByID bumps a warehouse's version by ID, without a prior read or lock.
	BumpByID(ctx context.Context, id string, at time.Time) error
	// Occupied = placed vehicles + unidentified vehicles of active batches.
	Occupied(ctx context.Context, warehouseIDs []string) (map[string]int, error)

	CreateBatch(ctx context.Context, b *model.ReceiptBatch) error
	LockBatch(ctx context.Context, companyID, id string) (*model.ReceiptBatch, error)
	UpdateBatchCounts(ctx context.Context, b *model.ReceiptBatch) error
	OpenBatches(ctx context.Context, warehouseID string) ([]model.ReceiptBatch, error)
}

type VehicleRepository interface {
	// ExistingVINs returns which of the VINs are already registered (anywhere).
	ExistingVINs(ctx context.Context, vins []string) ([]string, error)
	Create(ctx context.Context, units []model.VehicleUnit, placements []model.Placement) error
	// Get returns a vehicle visible to the company: owned by it or stored in
	// one of its warehouses.
	Get(ctx context.Context, companyID, id string) (*model.VehicleRow, error)
	List(ctx context.Context, f model.VehicleFilter) ([]model.VehicleRow, error)
	InWarehouse(ctx context.Context, warehouseID string) ([]model.VehicleRow, error)
	// LockPlacement row-locks the vehicle's placement; ErrNotFound if outside.
	LockPlacement(ctx context.Context, vehicleID string) (*model.Placement, error)
	MovePlacement(ctx context.Context, vehicleID, toWarehouseID string, at time.Time) error

	// LockOwned row-locks a vehicle unit by ID; ErrNotFound if it does not exist.
	LockOwned(ctx context.Context, id string) (*model.VehicleUnit, error)
	// TransferOwnership sets a vehicle's owner and custodian to toCompanyID.
	TransferOwnership(ctx context.Context, id, toCompanyID string) error
	// ClearOwnership sets a vehicle's owner and custodian to none (delivered
	// to a retail customer, not a company).
	ClearOwnership(ctx context.Context, id string) error
	// PlacementOf reads a vehicle's placement without locking it.
	PlacementOf(ctx context.Context, vehicleID string) (*model.Placement, error)
	// UpsertPlacement creates or replaces a vehicle's placement.
	UpsertPlacement(ctx context.Context, p model.Placement) error
	// DeletePlacement removes a vehicle's placement, returning it if one existed.
	DeletePlacement(ctx context.Context, vehicleID string) (*model.Placement, error)
	// CountPlaced counts how many of vehicleIDs are placed in warehouseID.
	CountPlaced(ctx context.Context, warehouseID string, vehicleIDs []string) (int, error)
}

type FactRepository interface {
	Append(ctx context.Context, facts ...model.Fact) error
	ForVehicle(ctx context.Context, vehicleID string) ([]model.Fact, error)
}

// ReservationRepository is the persistence for vehicle holds; a hold with
// status "held" is the only kind that blocks another holder.
type ReservationRepository interface {
	// HeldByVehicle returns the active hold on a vehicle, if any.
	HeldByVehicle(ctx context.Context, vehicleID string) (*model.Reservation, error)
	// HeldFor returns the holder's active hold on a vehicle, if any.
	HeldFor(ctx context.Context, holderType, holderID, vehicleID string) (*model.Reservation, error)
	Create(ctx context.Context, r *model.Reservation) error
	// Release marks the holder's held reservations released (nil vehicleIDs = all).
	Release(ctx context.Context, holderType, holderID string, vehicleIDs []string, reason string, at time.Time) error
	// ListHeld returns the vehicle IDs the holder currently holds.
	ListHeld(ctx context.Context, holderType, holderID string) ([]string, error)
	// CountHeld counts how many of vehicleIDs the holder currently holds.
	CountHeld(ctx context.Context, holderType, holderID string, vehicleIDs []string) (int, error)
	// Finalize marks the holder's held reservations on vehicleIDs finalized.
	Finalize(ctx context.Context, holderType, holderID string, vehicleIDs []string, at time.Time) error
	// FinalizeOne finalizes a single reservation by ID.
	FinalizeOne(ctx context.Context, id string, at time.Time) error
}
