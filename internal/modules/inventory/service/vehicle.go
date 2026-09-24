package service

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/modules/inventory/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/validate"
)

type VehicleDetail struct {
	Vehicle model.VehicleRow
	Model   model.VehicleModel
	Spec    model.Specification
	History []model.Fact
}

type MoveInput struct {
	FromWarehouseID    string    `json:"fromWarehouseId"`
	ToWarehouseID      string    `json:"toWarehouseId"`
	OccurredAt         time.Time `json:"occurredAt"`
	EvidenceBindingIDs []string  `json:"evidenceBindingIds"`
}

// Vehicle is the vehicle unit listing and movement service.
type Vehicle struct{ Deps }

func (s *Vehicle) Get(ctx context.Context, p *auth.Principal, id string) (*VehicleDetail, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	row, err := s.store.Vehicles().Get(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	m, err := s.store.Models().Get(ctx, row.ModelID)
	if err != nil {
		return nil, err
	}
	spec, err := s.store.Models().Spec(ctx, row.ModelID, row.SpecVersion)
	if err != nil {
		return nil, err
	}
	history, err := s.store.Facts().ForVehicle(ctx, id)
	if err != nil {
		return nil, err
	}
	return &VehicleDetail{Vehicle: *row, Model: *m, Spec: *spec, History: history}, nil
}

func (s *Vehicle) List(ctx context.Context, p *auth.Principal, f model.VehicleFilter) ([]model.VehicleRow, error) {
	var v apperr.Validation
	if f.Placement == "" {
		f.Placement = "any"
	}
	if !slices.Contains([]string{"warehouse", "outside", "any"}, f.Placement) {
		v.Add("placement", "must be warehouse, outside or any")
	}
	if f.WarehouseID != "" && uuid.Validate(f.WarehouseID) != nil {
		v.Add("warehouseId", "must be a valid ID")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	f.CompanyID = p.CompanyID
	return s.store.Vehicles().List(ctx, f)
}

// validateMoveInput validates a Move call against the current time.
func validateMoveInput(v *apperr.Validation, in MoveInput, now time.Time) {
	if uuid.Validate(in.FromWarehouseID) != nil {
		v.Add("fromWarehouseId", "must be a valid ID")
	}
	if uuid.Validate(in.ToWarehouseID) != nil || in.ToWarehouseID == in.FromWarehouseID {
		v.Add("toWarehouseId", "must be a different warehouse")
	}
	if in.OccurredAt.IsZero() || in.OccurredAt.After(now.Add(5*time.Minute)) {
		v.Add("occurredAt", "required and not in the future")
	}
	if len(in.EvidenceBindingIDs) > 0 {
		v.Add("evidenceBindingIds", "evidence attachments are not supported yet")
	}
}

// lockMoveWarehouses locks both warehouses of a Move (in ID order, to avoid
// deadlocks), checks the vehicle is still placed in the source warehouse and
// that the destination has room for it.
func lockMoveWarehouses(ctx context.Context, st Store, p *auth.Principal, id string, in MoveInput) (map[string]*model.Warehouse, *model.Warehouse, error) {
	ids := []string{in.FromWarehouseID, in.ToWarehouseID}
	slices.Sort(ids)
	locked := map[string]*model.Warehouse{}
	for _, wid := range ids {
		w, err := st.Warehouses().Lock(ctx, p.CompanyID, wid)
		if errors.Is(err, apperr.ErrNotFound) {
			return nil, nil, apperr.FieldError("toWarehouseId", "both warehouses must belong to your company")
		} else if err != nil {
			return nil, nil, err
		}
		locked[wid] = w
	}
	placement, err := st.Vehicles().LockPlacement(ctx, id)
	if errors.Is(err, apperr.ErrNotFound) {
		return nil, nil, apperr.New(apperr.ErrConflict, "not_in_warehouse", "the vehicle is not in a warehouse")
	} else if err != nil {
		return nil, nil, err
	}
	if placement.WarehouseID != in.FromWarehouseID {
		return nil, nil, apperr.New(apperr.ErrConflict, "placement_changed", "the vehicle is no longer in the source warehouse, reload")
	}
	to := locked[in.ToWarehouseID]
	occ, err := st.Warehouses().Occupied(ctx, []string{to.ID})
	if err != nil {
		return nil, nil, err
	}
	if occ[to.ID]+1 > to.Capacity {
		return nil, nil, apperr.New(apperr.ErrConflict, "capacity_exceeded", "not enough free space in the destination warehouse")
	}
	return locked, to, nil
}

// Move relocates a vehicle between two warehouses of the same company. Both
// warehouses are locked in ID order (no deadlocks) and the destination
// capacity is checked. Ownership does not change.
func (s *Vehicle) Move(ctx context.Context, p *auth.Principal, id string, in MoveInput) (*model.VehicleRow, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	var v apperr.Validation
	validateMoveInput(&v, in, s.clock())
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *model.VehicleRow
	err := s.store.InTx(ctx, func(st Store) error {
		if _, err := st.Vehicles().Get(ctx, p.CompanyID, id); err != nil {
			return err
		}
		locked, to, err := lockMoveWarehouses(ctx, st, p, id, in)
		if err != nil {
			return err
		}
		at := in.OccurredAt.UTC()
		if err := st.Vehicles().MovePlacement(ctx, id, to.ID, at); err != nil {
			return err
		}
		now := s.clock()
		for _, w := range locked {
			if err := st.Warehouses().Touch(ctx, w, now); err != nil {
				return err
			}
		}
		if err := st.Facts().Append(ctx, s.fact(p, "vehicle.moved", &id, &to.ID, nil, at, "",
			map[string]any{"from": in.FromWarehouseID, "to": in.ToWarehouseID})); err != nil {
			return err
		}
		result, err = st.Vehicles().Get(ctx, p.CompanyID, id)
		return err
	})
	return result, err
}
