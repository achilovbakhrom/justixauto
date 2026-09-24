package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"justixauto/internal/modules/inventory/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/jsonx"
	"justixauto/internal/pkg/validate"
)

type WarehouseProfileInput struct {
	Name    string       `json:"name"`
	Country jsonx.Label  `json:"country"`
	Region  *jsonx.Label `json:"region"`
	City    string       `json:"city"`
	Address string       `json:"address"`
}

// WarehouseInput is the contract's WarehouseInput.
type WarehouseInput struct {
	WarehouseProfileInput
	Capacity jsonx.Quantity `json:"capacity"`
	// BranchID makes the new warehouse the branch's main warehouse.
	BranchID *string `json:"branchId"`
}

func (in WarehouseProfileInput) apply(v *apperr.Validation, w *model.Warehouse) {
	w.Name = validate.Text(v, "name", in.Name, 1, 200)
	w.Country = validate.Text(v, "country", in.Country.Label, 1, 100)
	w.CountryKey = validate.Text(v, "country", in.Country.Key, 0, 50)
	w.Region, w.RegionKey = "", ""
	if in.Region != nil {
		w.Region = validate.Text(v, "region", in.Region.Label, 0, 100)
		w.RegionKey = validate.Text(v, "region", in.Region.Key, 0, 50)
	}
	w.City = validate.Text(v, "city", in.City, 1, 100)
	w.Address = validate.Text(v, "address", in.Address, 1, 500)
}

func capacity(v *apperr.Validation, q jsonx.Quantity) int {
	if q < 1 || q > 100_000 {
		v.Add("capacity", "must be a whole number from 1 to 100000")
	}
	return int(q)
}

// WarehouseView is a warehouse with its current occupancy.
type WarehouseView struct {
	Warehouse model.Warehouse
	Occupied  int
}

func (w WarehouseView) Free() int { return w.Warehouse.Capacity - w.Occupied }

// Warehouse is the warehouse and receipt batch capacity/placement service.
type Warehouse struct{ Deps }

func duplicateWarehouse(err error) error {
	if errors.Is(err, apperr.ErrConflict) {
		return apperr.New(apperr.ErrConflict, "warehouse_duplicate", "a warehouse with this name already exists")
	}
	return err
}

// Create adds a company-wide warehouse, or a branch's main warehouse when
// branchId is given.
func (s *Warehouse) Create(ctx context.Context, p *auth.Principal, in WarehouseInput) (*WarehouseView, error) {
	var v apperr.Validation
	now := s.clock()
	w := &model.Warehouse{ID: uuid.NewString(), CompanyID: p.CompanyID, Version: 1, CreatedAt: now, UpdatedAt: now}
	in.apply(&v, w)
	w.Capacity = capacity(&v, in.Capacity)
	if err := v.Err(); err != nil {
		return nil, err
	}
	err := s.store.InTx(ctx, func(st Store) error {
		if in.BranchID != nil {
			if err := s.checkBranch(ctx, st, p.CompanyID, *in.BranchID, ""); err != nil {
				return err
			}
			w.BranchID = in.BranchID
		}
		if err := st.Warehouses().Create(ctx, w); err != nil {
			return duplicateWarehouse(err)
		}
		return st.Facts().Append(ctx, s.fact(p, "warehouse.created", nil, &w.ID, nil, now, "", map[string]any{"name": w.Name, "capacity": w.Capacity}))
	})
	if err != nil {
		return nil, err
	}
	return &WarehouseView{Warehouse: *w}, nil
}

func (s *Warehouse) view(ctx context.Context, st Store, w *model.Warehouse) (*WarehouseView, error) {
	occ, err := st.Warehouses().Occupied(ctx, []string{w.ID})
	if err != nil {
		return nil, err
	}
	return &WarehouseView{Warehouse: *w, Occupied: occ[w.ID]}, nil
}

func (s *Warehouse) Get(ctx context.Context, p *auth.Principal, id string) (*WarehouseView, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	w, err := s.store.Warehouses().Get(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, s.store, w)
}

// List returns the company's warehouses in the session's branch scope.
func (s *Warehouse) List(ctx context.Context, p *auth.Principal) ([]WarehouseView, error) {
	f := model.WarehouseFilter{CompanyID: p.CompanyID}
	if p.BranchScope.Mode == "SELECTED" {
		f.BranchIDs = p.BranchScope.BranchIDs
	}
	ws, err := s.store.Warehouses().List(ctx, f)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(ws))
	for i, w := range ws {
		ids[i] = w.ID
	}
	occ, err := s.store.Warehouses().Occupied(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]WarehouseView, len(ws))
	for i, w := range ws {
		out[i] = WarehouseView{Warehouse: w, Occupied: occ[w.ID]}
	}
	return out, nil
}

// Update edits the profile only; capacity has its own reasoned command.
func (s *Warehouse) Update(ctx context.Context, p *auth.Principal, id string, expected int64, in WarehouseProfileInput) (*WarehouseView, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	var result *WarehouseView
	err := s.store.InTx(ctx, func(st Store) error {
		w, err := st.Warehouses().Lock(ctx, p.CompanyID, id)
		if err != nil {
			return err
		}
		if w.Version != expected {
			return apperr.ErrStale
		}
		var v apperr.Validation
		in.apply(&v, w)
		if err := v.Err(); err != nil {
			return err
		}
		w.UpdatedAt = s.clock()
		if err := st.Warehouses().Update(ctx, w, expected); err != nil {
			return duplicateWarehouse(err)
		}
		result, err = s.view(ctx, st, w)
		return err
	})
	return result, err
}

// checkBranch: the branch is the company's and has no other main warehouse.
func (s *Warehouse) checkBranch(ctx context.Context, st Store, companyID, branchID, warehouseID string) error {
	ok, err := s.branches.BranchOf(ctx, companyID, branchID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.FieldError("branchId", "not a branch of your company")
	}
	other, err := st.Warehouses().ByBranch(ctx, companyID, branchID)
	if err == nil && other.ID != warehouseID {
		return apperr.New(apperr.ErrConflict, "branch_has_warehouse", "the branch already has a main warehouse: "+other.Name)
	}
	if err != nil && !errors.Is(err, apperr.ErrNotFound) {
		return err
	}
	return nil
}

// AttachBranch makes the warehouse a branch's main warehouse, or (nil)
// detaches it; the warehouse keeps its capacity and stock either way.
func (s *Warehouse) AttachBranch(ctx context.Context, p *auth.Principal, id string, expected int64, branchID *string) (*WarehouseView, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	var result *WarehouseView
	err := s.store.InTx(ctx, func(st Store) error {
		w, err := st.Warehouses().Lock(ctx, p.CompanyID, id)
		if err != nil {
			return err
		}
		if w.Version != expected {
			return apperr.ErrStale
		}
		fact, details := "warehouse.branch_detached", map[string]any{"branchId": w.BranchID}
		if branchID != nil {
			if err := s.checkBranch(ctx, st, p.CompanyID, *branchID, w.ID); err != nil {
				return err
			}
			fact, details = "warehouse.branch_attached", map[string]any{"branchId": *branchID}
		} else if w.BranchID == nil {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "the warehouse is not attached to a branch")
		}
		w.BranchID, w.UpdatedAt = branchID, s.clock()
		if err := st.Warehouses().Update(ctx, w, expected); err != nil {
			if errors.Is(err, apperr.ErrConflict) { // a concurrent attachment won
				return apperr.New(apperr.ErrConflict, "branch_has_warehouse", "the branch already has a main warehouse")
			}
			return err
		}
		if err := st.Facts().Append(ctx, s.fact(p, fact, nil, &w.ID, nil, w.UpdatedAt, "", details)); err != nil {
			return err
		}
		result, err = s.view(ctx, st, w)
		return err
	})
	return result, err
}

// ChangeCapacity sets a new capacity; it can never drop below what is occupied.
func (s *Warehouse) ChangeCapacity(ctx context.Context, p *auth.Principal, id string, expected int64, q jsonx.Quantity, why string) (*WarehouseView, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	var v apperr.Validation
	newCapacity := capacity(&v, q)
	why = validate.Reason(&v, why)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *WarehouseView
	err := s.store.InTx(ctx, func(st Store) error {
		w, err := st.Warehouses().Lock(ctx, p.CompanyID, id)
		if err != nil {
			return err
		}
		if w.Version != expected {
			return apperr.ErrStale
		}
		view, err := s.view(ctx, st, w)
		if err != nil {
			return err
		}
		if newCapacity < view.Occupied {
			return apperr.New(apperr.ErrConflict, "capacity_below_occupied", "capacity cannot be lower than the vehicles already stored")
		}
		before := w.Capacity
		w.Capacity, w.UpdatedAt = newCapacity, s.clock()
		if err := st.Warehouses().Update(ctx, w, expected); err != nil {
			return err
		}
		if err := st.Facts().Append(ctx, s.fact(p, "warehouse.capacity_changed", nil, &w.ID, nil, w.UpdatedAt, why,
			map[string]any{"before": before, "after": newCapacity})); err != nil {
			return err
		}
		view.Warehouse = *w
		result = view
		return nil
	})
	return result, err
}

// WarehouseStock lists what is in a warehouse: identified vehicles and batches
// still waiting for VINs (never shown as fake vehicles).
type WarehouseStock struct {
	View         WarehouseView
	Vehicles     []model.VehicleRow
	Unidentified []model.ReceiptBatch
}

func (s *Warehouse) Stock(ctx context.Context, p *auth.Principal, id string) (*WarehouseStock, error) {
	view, err := s.Get(ctx, p, id)
	if err != nil {
		return nil, err
	}
	vehicles, err := s.store.Vehicles().InWarehouse(ctx, id)
	if err != nil {
		return nil, err
	}
	batches, err := s.store.Warehouses().OpenBatches(ctx, id)
	if err != nil {
		return nil, err
	}
	return &WarehouseStock{View: *view, Vehicles: vehicles, Unidentified: batches}, nil
}
