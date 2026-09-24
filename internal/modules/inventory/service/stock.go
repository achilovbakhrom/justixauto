package service

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/modules/inventory/model"
	"justixauto/internal/pkg/apperr"
)

// VehicleInfo is what other modules may learn about a vehicle.
type VehicleInfo struct {
	ID, VIN, ModelID string
	OwnerCompanyID   string
	WarehouseID      string // "" when outside any warehouse
}

var errUnavailable = apperr.New(apperr.ErrConflict, "vehicle_unavailable", "a vehicle is not available: unknown, not yours or already reserved")

// Handover is the physical and legal hand-over of held vehicles to another
// company, received into one of its warehouses.
type Handover struct {
	Holder        model.Holder
	VehicleIDs    []string
	ToCompanyID   string
	ToWarehouseID string
	ActorUserID   string
	At            time.Time
}

// Stock is used by other modules (through their ports) to reserve, release
// and hand over vehicles. Calls join the caller's transaction when ctx
// carries one (database.WithTx).
type Stock struct{ Deps }

// Vehicle returns a vehicle owned by companyID or stored in its warehouses.
func (s *Stock) Vehicle(ctx context.Context, companyID, id string) (*VehicleInfo, error) {
	if uuid.Validate(id) != nil {
		return nil, apperr.ErrNotFound
	}
	row, err := s.store.Vehicles().Get(ctx, companyID, id)
	if err != nil {
		return nil, err
	}
	info := &VehicleInfo{ID: row.ID, VIN: row.VIN, ModelID: row.ModelID}
	if row.OwnerCompanyID != nil {
		info.OwnerCompanyID = *row.OwnerCompanyID
	}
	if row.WarehouseID != nil {
		info.WarehouseID = *row.WarehouseID
	}
	return info, nil
}

// Reserve holds vehicles owned by companyID for the holder. Holding again
// for the same holder is a no-op; any other hold makes the call fail.
func (s *Stock) Reserve(ctx context.Context, companyID string, h model.Holder, vehicleIDs []string) error {
	ids := slices.Clone(vehicleIDs)
	slices.Sort(ids)
	return s.store.InTx(ctx, func(st Store) error {
		now := s.clock()
		for _, id := range ids {
			unit, err := st.Vehicles().LockOwned(ctx, id)
			if errors.Is(err, apperr.ErrNotFound) || (err == nil && (unit.OwnerCompanyID == nil || *unit.OwnerCompanyID != companyID)) {
				return errUnavailable
			} else if err != nil {
				return err
			}
			held, err := st.Reservations().HeldByVehicle(ctx, id)
			if err == nil {
				if held.HolderType == h.Type && held.HolderID == h.ID {
					continue
				}
				return errUnavailable
			} else if !errors.Is(err, apperr.ErrNotFound) {
				return err
			}
			r := &model.Reservation{
				ID: uuid.NewString(), VehicleID: id, CompanyID: companyID, HolderType: h.Type,
				HolderID: h.ID, Status: "held", CreatedAt: now,
			}
			if err := st.Reservations().Create(ctx, r); err != nil {
				if errors.Is(err, apperr.ErrConflict) {
					return errUnavailable
				}
				return err
			}
		}
		return nil
	})
}

// Release frees the holder's vehicles (nil = all of them).
func (s *Stock) Release(ctx context.Context, h model.Holder, vehicleIDs []string, reason string) error {
	return s.store.Reservations().Release(ctx, h.Type, h.ID, vehicleIDs, reason, s.clock())
}

// Held lists the vehicles currently held by the holder.
func (s *Stock) Held(ctx context.Context, h model.Holder) ([]string, error) {
	return s.store.Reservations().ListHeld(ctx, h.Type, h.ID)
}

// lockHandoverTarget locks the receiving warehouse, checks that every vehicle
// in the handover is currently held by t.Holder and that the warehouse has
// enough free capacity for the incoming vehicles.
func lockHandoverTarget(ctx context.Context, st Store, t Handover) (*model.Warehouse, error) {
	to, err := st.Warehouses().Lock(ctx, t.ToCompanyID, t.ToWarehouseID)
	if errors.Is(err, apperr.ErrNotFound) {
		return nil, apperr.FieldError("warehouseId", "not one of your warehouses")
	} else if err != nil {
		return nil, err
	}
	held, err := st.Reservations().CountHeld(ctx, t.Holder.Type, t.Holder.ID, t.VehicleIDs)
	if err != nil {
		return nil, err
	}
	if held != len(t.VehicleIDs) {
		return nil, errUnavailable
	}
	occ, err := st.Warehouses().Occupied(ctx, []string{to.ID})
	if err != nil {
		return nil, err
	}
	alreadyThere, err := st.Vehicles().CountPlaced(ctx, to.ID, t.VehicleIDs)
	if err != nil {
		return nil, err
	}
	if occ[to.ID]+len(t.VehicleIDs)-alreadyThere > to.Capacity {
		return nil, apperr.New(apperr.ErrConflict, "capacity_exceeded", "not enough free space in the receiving warehouse")
	}
	return to, nil
}

// transferVehicle moves one vehicle's ownership, custody and placement to the
// receiving warehouse and returns the fact recording the hand-over. Any
// warehouse the vehicle is leaving has its version bumped.
func transferVehicle(ctx context.Context, st Store, t Handover, to *model.Warehouse, id string, now time.Time) (model.Fact, error) {
	from, fromErr := st.Vehicles().PlacementOf(ctx, id)
	if err := st.Vehicles().TransferOwnership(ctx, id, t.ToCompanyID); err != nil {
		return model.Fact{}, err
	}
	placement := model.Placement{VehicleID: id, WarehouseID: to.ID, PlacedAt: t.At}
	if err := st.Vehicles().UpsertPlacement(ctx, placement); err != nil {
		return model.Fact{}, err
	}
	details, _ := json.Marshal(map[string]any{"holderType": t.Holder.Type, "holderId": t.Holder.ID})
	vid := id
	fact := model.Fact{
		ID: uuid.NewString(), CompanyID: t.ToCompanyID, FactType: "vehicle.handed_over",
		VehicleID: &vid, WarehouseID: &to.ID, ActorUserID: t.ActorUserID, OccurredAt: t.At, RecordedAt: now, Details: details,
	}
	if fromErr == nil {
		if err := st.Warehouses().BumpByID(ctx, from.WarehouseID, now); err != nil {
			return model.Fact{}, err
		}
	}
	return fact, nil
}

// Transfer completes a hand-over: the reservations are finalized, ownership
// and custody pass to the receiving company and the vehicles are placed in its
// warehouse, whose capacity is checked under a row lock.
func (s *Stock) Transfer(ctx context.Context, t Handover) error {
	return s.store.InTx(ctx, func(st Store) error {
		to, err := lockHandoverTarget(ctx, st, t)
		if err != nil {
			return err
		}
		now := s.clock()
		var facts []model.Fact
		for _, id := range t.VehicleIDs {
			fact, err := transferVehicle(ctx, st, t, to, id, now)
			if err != nil {
				return err
			}
			facts = append(facts, fact)
		}
		if err := st.Reservations().Finalize(ctx, t.Holder.Type, t.Holder.ID, t.VehicleIDs, now); err != nil {
			return err
		}
		if err := st.Warehouses().Touch(ctx, to, now); err != nil {
			return err
		}
		return st.Facts().Append(ctx, facts...)
	})
}

// Deliver hands a held vehicle to a retail customer (a natural person, not a
// company): the hold is finalized, the vehicle leaves its warehouse and no
// company owns or keeps it any more. The history stays.
func (s *Stock) Deliver(ctx context.Context, h model.Holder, vehicleID, actorID string, at time.Time) error {
	return s.store.InTx(ctx, func(st Store) error {
		r, err := st.Reservations().HeldFor(ctx, h.Type, h.ID, vehicleID)
		if errors.Is(err, apperr.ErrNotFound) {
			return errUnavailable
		} else if err != nil {
			return err
		}
		now := s.clock()
		if p, delErr := st.Vehicles().DeletePlacement(ctx, vehicleID); delErr == nil {
			if err := st.Warehouses().BumpByID(ctx, p.WarehouseID, now); err != nil {
				return err
			}
		}
		if err := st.Vehicles().ClearOwnership(ctx, vehicleID); err != nil {
			return err
		}
		if err := st.Reservations().FinalizeOne(ctx, r.ID, now); err != nil {
			return err
		}
		details, _ := json.Marshal(map[string]any{"holderType": h.Type, "holderId": h.ID})
		vid := vehicleID
		return st.Facts().Append(ctx, model.Fact{
			ID: uuid.NewString(), CompanyID: r.CompanyID, FactType: "vehicle.delivered_to_customer",
			VehicleID: &vid, ActorUserID: actorID, OccurredAt: at, RecordedAt: now, Details: details,
		})
	})
}
