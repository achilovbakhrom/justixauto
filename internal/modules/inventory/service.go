package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/jsonx"
	"justixauto/internal/platform/validate"
)

type deps struct {
	store Store
	now   func() time.Time
}

func (d deps) clock() time.Time { return d.now().UTC() }

func (d deps) fact(p *auth.Principal, factType string, vehicleID, warehouseID, batchID *string, occurredAt time.Time, reason string, details map[string]any) Fact {
	raw, _ := json.Marshal(details)
	if details == nil {
		raw = []byte("{}")
	}
	return Fact{ID: uuid.NewString(), CompanyID: p.CompanyID, FactType: factType, VehicleID: vehicleID,
		WarehouseID: warehouseID, ReceiptBatchID: batchID, ActorUserID: p.UserID,
		OccurredAt: occurredAt, RecordedAt: d.clock(), Reason: reason, Details: raw}
}

func ptr[T any](v T) *T { return &v }

func vinUnavailable(vins []string) error {
	return apperr.New(apperr.ErrConflict, "VIN_UNAVAILABLE", "VIN already registered: "+strings.Join(vins, ", "))
}

// ================= vehicle model catalogue =================

// SpecInput is the contract's VehicleSpecification: nine separate fields.
type SpecInput struct {
	Make          string `json:"make"`
	Model         string `json:"model"`
	Variant       string `json:"variant"`
	Year          int    `json:"year"`
	BodyType      string `json:"bodyType"`
	ExteriorColor string `json:"exteriorColor"`
	InteriorColor string `json:"interiorColor"`
	Powertrain    string `json:"powertrain"`
	Drivetrain    string `json:"drivetrain"`
}

func (in SpecInput) validate(v *apperr.Validation) (make_, model, variant string, spec Specification) {
	make_ = validate.Text(v, "specification.make", in.Make, 1, 100)
	model = validate.Text(v, "specification.model", in.Model, 1, 100)
	variant = validate.Text(v, "specification.variant", in.Variant, 1, 100)
	if in.Year < 1900 || in.Year > 2100 {
		v.Add("specification.year", "must be a year between 1900 and 2100")
	}
	spec = Specification{Year: in.Year,
		BodyType:      validate.Text(v, "specification.bodyType", in.BodyType, 1, 50),
		ExteriorColor: validate.Text(v, "specification.exteriorColor", in.ExteriorColor, 1, 50),
		InteriorColor: validate.Text(v, "specification.interiorColor", in.InteriorColor, 1, 50),
		Powertrain:    validate.Text(v, "specification.powertrain", in.Powertrain, 1, 50),
		Drivetrain:    validate.Text(v, "specification.drivetrain", in.Drivetrain, 1, 50)}
	return
}

// ModelDetail is a model with its current and (optionally) all specifications.
type ModelDetail struct {
	Model    VehicleModel
	Current  Specification
	Versions []Specification
}

type ModelService struct{ deps }

// Create adds a make/model/variant with specification version 1.
func (s *ModelService) Create(ctx context.Context, p *auth.Principal, in SpecInput) (*ModelDetail, error) {
	var v apperr.Validation
	make_, model, variant, spec := in.validate(&v)
	if err := v.Err(); err != nil {
		return nil, err
	}
	now := s.clock()
	m := &VehicleModel{ID: uuid.NewString(), Make: make_, Model: model, Variant: variant, CurrentSpecVersion: 1,
		Version: 1, CreatedAt: now, UpdatedAt: now}
	spec.ModelID, spec.SpecVersion, spec.CreatedAt, spec.CreatedBy = m.ID, 1, now, p.UserID
	if err := s.store.Models().Create(ctx, m, &spec); err != nil {
		if errors.Is(err, apperr.ErrConflict) {
			return nil, apperr.New(apperr.ErrConflict, "model_duplicate", "this make, model and variant already exist")
		}
		return nil, err
	}
	return &ModelDetail{Model: *m, Current: spec, Versions: []Specification{spec}}, nil
}

// AddVersion records a new immutable specification; existing vehicles keep
// the version they were registered with. Make, model and variant are fixed.
func (s *ModelService) AddVersion(ctx context.Context, p *auth.Principal, id string, expected int64, in SpecInput) (*ModelDetail, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	var v apperr.Validation
	make_, model, variant, spec := in.validate(&v)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *ModelDetail
	err := s.store.InTx(ctx, func(st Store) error {
		m, err := st.Models().Get(ctx, id)
		if err != nil {
			return err
		}
		if m.Version != expected {
			return apperr.ErrStale
		}
		if !strings.EqualFold(make_, m.Make) || !strings.EqualFold(model, m.Model) || !strings.EqualFold(variant, m.Variant) {
			return apperr.FieldError("specification", "make, model and variant cannot change; create a new model instead")
		}
		now := s.clock()
		spec.ModelID, spec.SpecVersion, spec.CreatedAt, spec.CreatedBy = m.ID, m.CurrentSpecVersion+1, now, p.UserID
		m.UpdatedAt = now
		if err := st.Models().AddSpec(ctx, m, expected, &spec); err != nil {
			return err
		}
		versions, err := st.Models().Specs(ctx, m.ID)
		if err != nil {
			return err
		}
		result = &ModelDetail{Model: *m, Current: spec, Versions: versions}
		return nil
	})
	return result, err
}

func (s *ModelService) Get(ctx context.Context, id string) (*ModelDetail, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	m, err := s.store.Models().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	versions, err := s.store.Models().Specs(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &ModelDetail{Model: *m, Versions: versions}
	for _, spec := range versions {
		if spec.SpecVersion == m.CurrentSpecVersion {
			d.Current = spec
		}
	}
	return d, nil
}

func (s *ModelService) List(ctx context.Context, f ModelFilter) ([]ModelDetail, error) {
	f.Query = strings.TrimSpace(f.Query)
	models, err := s.store.Models().List(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]ModelDetail, 0, len(models))
	for _, m := range models {
		spec, err := s.store.Models().Spec(ctx, m.ID, m.CurrentSpecVersion)
		if err != nil {
			return nil, err
		}
		out = append(out, ModelDetail{Model: m, Current: *spec})
	}
	return out, nil
}

// ================= warehouses =================

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
}

func (in WarehouseProfileInput) apply(v *apperr.Validation, w *Warehouse) {
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
	Warehouse Warehouse
	Occupied  int
}

func (w WarehouseView) Free() int { return w.Warehouse.Capacity - w.Occupied }

type WarehouseService struct{ deps }

func duplicateWarehouse(err error) error {
	if errors.Is(err, apperr.ErrConflict) {
		return apperr.New(apperr.ErrConflict, "warehouse_duplicate", "a warehouse with this name already exists")
	}
	return err
}

// Create adds a company-wide warehouse; attaching it to a branch is a separate step.
func (s *WarehouseService) Create(ctx context.Context, p *auth.Principal, in WarehouseInput) (*WarehouseView, error) {
	var v apperr.Validation
	now := s.clock()
	w := &Warehouse{ID: uuid.NewString(), CompanyID: p.CompanyID, Version: 1, CreatedAt: now, UpdatedAt: now}
	in.apply(&v, w)
	w.Capacity = capacity(&v, in.Capacity)
	if err := v.Err(); err != nil {
		return nil, err
	}
	err := s.store.InTx(ctx, func(st Store) error {
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

func (s *WarehouseService) view(ctx context.Context, st Store, w *Warehouse) (*WarehouseView, error) {
	occ, err := st.Warehouses().Occupied(ctx, []string{w.ID})
	if err != nil {
		return nil, err
	}
	return &WarehouseView{Warehouse: *w, Occupied: occ[w.ID]}, nil
}

func (s *WarehouseService) Get(ctx context.Context, p *auth.Principal, id string) (*WarehouseView, error) {
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
func (s *WarehouseService) List(ctx context.Context, p *auth.Principal) ([]WarehouseView, error) {
	f := WarehouseFilter{CompanyID: p.CompanyID}
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
func (s *WarehouseService) Update(ctx context.Context, p *auth.Principal, id string, expected int64, in WarehouseProfileInput) (*WarehouseView, error) {
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

// ChangeCapacity sets a new capacity; it can never drop below what is occupied.
func (s *WarehouseService) ChangeCapacity(ctx context.Context, p *auth.Principal, id string, expected int64, q jsonx.Quantity, why string) (*WarehouseView, error) {
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
	Vehicles     []VehicleRow
	Unidentified []ReceiptBatch
}

func (s *WarehouseService) Stock(ctx context.Context, p *auth.Principal, id string) (*WarehouseStock, error) {
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

// ================= receipts =================

type StockInput struct {
	Mode     string         `json:"mode"` // "unidentified" or "identified"
	Quantity jsonx.Quantity `json:"quantity"`
	VINs     []string       `json:"vins"`
}

type ReceiptInput struct {
	ModelID                   string         `json:"modelId"`
	ModelSpecificationVersion jsonx.Quantity `json:"modelSpecificationVersion"`
	Stock                     StockInput     `json:"stock"`
	ReceivedAt                time.Time      `json:"receivedAt"`
	EvidenceBindingIDs        []string       `json:"evidenceBindingIds"`
}

type ReceiptResult struct {
	Batch     ReceiptBatch
	Warehouse WarehouseView
	Vehicles  []VehicleUnit
}

type ReceiptService struct{ deps }

// vins normalizes VINs and rejects malformed or repeated ones.
func vins(v *apperr.Validation, field string, raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		vin, ok := normalizeVIN(r)
		switch {
		case !ok:
			v.Add(field, "invalid VIN "+strings.TrimSpace(r)+": 17 characters, no I, O or Q")
		case slices.Contains(out, vin):
			v.Add(field, "VIN "+vin+" is listed twice")
		default:
			out = append(out, vin)
		}
	}
	return out
}

func (s *ReceiptService) checkOccurred(v *apperr.Validation, field string, at time.Time) {
	if at.IsZero() || at.After(s.clock().Add(5*time.Minute)) {
		v.Add(field, "required and not in the future")
	}
}

func newUnits(p *auth.Principal, vins []string, modelID string, specVersion int, warehouseID, batchID string, at time.Time) ([]VehicleUnit, []Placement) {
	units := make([]VehicleUnit, len(vins))
	placements := make([]Placement, len(vins))
	for i, vin := range vins {
		units[i] = VehicleUnit{ID: uuid.NewString(), VIN: vin, ModelID: modelID, SpecVersion: specVersion,
			OwnerCompanyID: ptr(p.CompanyID), CustodianCompanyID: ptr(p.CompanyID), Version: 1, CreatedAt: at}
		placements[i] = Placement{VehicleID: units[i].ID, WarehouseID: warehouseID, ReceiptBatchID: ptr(batchID), PlacedAt: at}
	}
	return units, placements
}

// Receive records vehicles arriving at a warehouse: either known VINs or a
// quantity whose VINs come later. Everything commits together or not at all,
// and the warehouse capacity is checked under a row lock.
func (s *ReceiptService) Receive(ctx context.Context, p *auth.Principal, warehouseID string, expected int64, in ReceiptInput) (*ReceiptResult, error) {
	if err := validate.IDs(warehouseID); err != nil {
		return nil, err
	}
	var v apperr.Validation
	if uuid.Validate(in.ModelID) != nil {
		v.Add("modelId", "must be a valid ID")
	}
	var quantity int
	var list []string
	switch in.Stock.Mode {
	case "unidentified":
		if in.Stock.Quantity < 1 || in.Stock.Quantity > 10_000 || len(in.Stock.VINs) > 0 {
			v.Add("stock.quantity", "must be 1-10000, without VINs")
		}
		quantity = int(in.Stock.Quantity)
	case "identified":
		list = vins(&v, "stock.vins", in.Stock.VINs)
		if len(in.Stock.VINs) == 0 || len(in.Stock.VINs) > 1000 {
			v.Add("stock.vins", "list 1-1000 VINs")
		}
		quantity = len(list)
	default:
		v.Add("stock.mode", "must be unidentified or identified")
	}
	s.checkOccurred(&v, "receivedAt", in.ReceivedAt)
	if len(in.EvidenceBindingIDs) > 0 {
		v.Add("evidenceBindingIds", "evidence attachments are not supported yet")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *ReceiptResult
	err := s.store.InTx(ctx, func(st Store) error {
		if _, err := st.Models().Spec(ctx, in.ModelID, int(in.ModelSpecificationVersion)); errors.Is(err, apperr.ErrNotFound) {
			return apperr.FieldError("modelSpecificationVersion", "unknown model or specification version")
		} else if err != nil {
			return err
		}
		w, err := st.Warehouses().Lock(ctx, p.CompanyID, warehouseID)
		if err != nil {
			return err
		}
		if w.Version != expected {
			return apperr.ErrStale
		}
		occ, err := st.Warehouses().Occupied(ctx, []string{w.ID})
		if err != nil {
			return err
		}
		if occ[w.ID]+quantity > w.Capacity {
			return apperr.New(apperr.ErrConflict, "capacity_exceeded", "not enough free space in the warehouse")
		}
		if taken, err := st.Vehicles().ExistingVINs(ctx, list); err != nil {
			return err
		} else if len(taken) > 0 {
			return vinUnavailable(taken)
		}
		now := s.clock()
		received := in.ReceivedAt.UTC()
		b := &ReceiptBatch{ID: uuid.NewString(), CompanyID: p.CompanyID, WarehouseID: w.ID, ModelID: in.ModelID,
			SpecVersion: int(in.ModelSpecificationVersion), ConfirmedQuantity: quantity, IdentifiedCount: len(list),
			UnidentifiedCount: quantity - len(list), ReceivedAt: received, CreatedBy: p.UserID, Version: 1, CreatedAt: now}
		if err := st.Warehouses().CreateBatch(ctx, b); err != nil {
			return err
		}
		units, placements := newUnits(p, list, b.ModelID, b.SpecVersion, w.ID, b.ID, received)
		if err := st.Vehicles().Create(ctx, units, placements); err != nil {
			if errors.Is(err, apperr.ErrConflict) {
				return vinUnavailable(list) // a concurrent receipt registered one of them
			}
			return err
		}
		if err := st.Warehouses().Touch(ctx, w, now); err != nil {
			return err
		}
		facts := []Fact{s.fact(p, "receipt.recorded", nil, &w.ID, &b.ID, received, "",
			map[string]any{"mode": in.Stock.Mode, "quantity": quantity, "modelId": b.ModelID, "specVersion": b.SpecVersion})}
		for i := range units {
			facts = append(facts, s.fact(p, "vehicle.received", &units[i].ID, &w.ID, &b.ID, received, "", map[string]any{"vin": units[i].VIN}))
		}
		if err := st.Facts().Append(ctx, facts...); err != nil {
			return err
		}
		result = &ReceiptResult{Batch: *b, Warehouse: WarehouseView{Warehouse: *w, Occupied: occ[w.ID] + quantity}, Vehicles: units}
		return nil
	})
	return result, err
}

type IdentifyInput struct {
	Items []struct {
		VIN     string `json:"vin"`
		ModelID string `json:"modelId"`
	} `json:"items"`
	Atomic bool `json:"atomic"`
}

// Identify turns unidentified stock of a batch into vehicles with VINs, one
// unit each. Occupancy does not change. All items succeed or none do.
func (s *ReceiptService) Identify(ctx context.Context, p *auth.Principal, batchID string, in IdentifyInput) (*ReceiptResult, error) {
	if err := validate.IDs(batchID); err != nil {
		return nil, err
	}
	var v apperr.Validation
	if !in.Atomic {
		v.Add("atomic", "only atomic identification is supported")
	}
	raw := make([]string, len(in.Items))
	for i, item := range in.Items {
		raw[i] = item.VIN
	}
	list := vins(&v, "items", raw)
	if len(in.Items) == 0 || len(in.Items) > 1000 {
		v.Add("items", "list 1-1000 vehicles")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *ReceiptResult
	err := s.store.InTx(ctx, func(st Store) error {
		b, err := st.Warehouses().LockBatch(ctx, p.CompanyID, batchID)
		if err != nil {
			return err
		}
		for _, item := range in.Items {
			if item.ModelID != b.ModelID {
				return apperr.FieldError("items", "every vehicle must match the batch model")
			}
		}
		if len(list) > b.UnidentifiedCount {
			return apperr.New(apperr.ErrConflict, "exceeds_unidentified", "the batch has fewer vehicles waiting for a VIN")
		}
		w, err := st.Warehouses().Lock(ctx, p.CompanyID, b.WarehouseID)
		if err != nil {
			return err
		}
		if taken, err := st.Vehicles().ExistingVINs(ctx, list); err != nil {
			return err
		} else if len(taken) > 0 {
			return vinUnavailable(taken)
		}
		now := s.clock()
		units, placements := newUnits(p, list, b.ModelID, b.SpecVersion, w.ID, b.ID, now)
		if err := st.Vehicles().Create(ctx, units, placements); err != nil {
			if errors.Is(err, apperr.ErrConflict) {
				return vinUnavailable(list)
			}
			return err
		}
		b.IdentifiedCount += len(list)
		b.UnidentifiedCount -= len(list)
		if err := st.Warehouses().UpdateBatchCounts(ctx, b); err != nil {
			return err
		}
		if err := st.Warehouses().Touch(ctx, w, now); err != nil {
			return err
		}
		facts := make([]Fact, len(units))
		for i := range units {
			facts[i] = s.fact(p, "vehicle.identified", &units[i].ID, &w.ID, &b.ID, now, "", map[string]any{"vin": units[i].VIN})
		}
		if err := st.Facts().Append(ctx, facts...); err != nil {
			return err
		}
		occ, err := st.Warehouses().Occupied(ctx, []string{w.ID})
		if err != nil {
			return err
		}
		result = &ReceiptResult{Batch: *b, Warehouse: WarehouseView{Warehouse: *w, Occupied: occ[w.ID]}, Vehicles: units}
		return nil
	})
	return result, err
}

// CorrectQuantity fixes the confirmed quantity of a batch after a recount. It
// is a separate audited action (business-logic §5.7): a reason is required,
// the quantity cannot drop below the vehicles already identified, and an
// increase must fit the warehouse.
func (s *ReceiptService) CorrectQuantity(ctx context.Context, p *auth.Principal, batchID string, expected int64, quantity int, reason string) (*ReceiptResult, error) {
	if err := validate.IDs(batchID); err != nil {
		return nil, err
	}
	var v apperr.Validation
	if quantity < 1 || quantity > 10000 {
		v.Add("quantity", "must be 1-10000")
	}
	why := validate.Reason(&v, reason)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *ReceiptResult
	err := s.store.InTx(ctx, func(st Store) error {
		b, err := st.Warehouses().LockBatch(ctx, p.CompanyID, batchID)
		if err != nil {
			return err
		}
		if b.Version != expected {
			return apperr.ErrStale
		}
		if quantity == b.ConfirmedQuantity {
			return apperr.FieldError("quantity", "equals the current quantity")
		}
		if quantity < b.IdentifiedCount {
			return apperr.FieldError("quantity", "cannot be below the vehicles already identified")
		}
		w, err := st.Warehouses().Lock(ctx, p.CompanyID, b.WarehouseID)
		if err != nil {
			return err
		}
		occ, err := st.Warehouses().Occupied(ctx, []string{w.ID})
		if err != nil {
			return err
		}
		delta := quantity - b.ConfirmedQuantity
		if delta > 0 && occ[w.ID]+delta > w.Capacity {
			return apperr.New(apperr.ErrConflict, "capacity_exceeded", "not enough free space in the warehouse")
		}
		previous := b.ConfirmedQuantity
		b.ConfirmedQuantity, b.UnidentifiedCount = quantity, quantity-b.IdentifiedCount
		if err := st.Warehouses().UpdateBatchCounts(ctx, b); err != nil {
			return err
		}
		now := s.clock()
		if err := st.Warehouses().Touch(ctx, w, now); err != nil {
			return err
		}
		if err := st.Facts().Append(ctx, s.fact(p, "receipt.quantity_corrected", nil, &w.ID, &b.ID, now, why,
			map[string]any{"from": previous, "to": quantity})); err != nil {
			return err
		}
		result = &ReceiptResult{Batch: *b, Warehouse: WarehouseView{Warehouse: *w, Occupied: occ[w.ID] + delta}}
		return nil
	})
	return result, err
}

// ================= vehicles =================

type VehicleDetail struct {
	Vehicle VehicleRow
	Model   VehicleModel
	Spec    Specification
	History []Fact
}

type MoveInput struct {
	FromWarehouseID    string    `json:"fromWarehouseId"`
	ToWarehouseID      string    `json:"toWarehouseId"`
	OccurredAt         time.Time `json:"occurredAt"`
	EvidenceBindingIDs []string  `json:"evidenceBindingIds"`
}

type VehicleService struct{ deps }

func (s *VehicleService) Get(ctx context.Context, p *auth.Principal, id string) (*VehicleDetail, error) {
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

func (s *VehicleService) List(ctx context.Context, p *auth.Principal, f VehicleFilter) ([]VehicleRow, error) {
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

// Move relocates a vehicle between two warehouses of the same company. Both
// warehouses are locked in ID order (no deadlocks) and the destination
// capacity is checked. Ownership does not change.
func (s *VehicleService) Move(ctx context.Context, p *auth.Principal, id string, in MoveInput) (*VehicleRow, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	var v apperr.Validation
	if uuid.Validate(in.FromWarehouseID) != nil {
		v.Add("fromWarehouseId", "must be a valid ID")
	}
	if uuid.Validate(in.ToWarehouseID) != nil || in.ToWarehouseID == in.FromWarehouseID {
		v.Add("toWarehouseId", "must be a different warehouse")
	}
	if in.OccurredAt.IsZero() || in.OccurredAt.After(s.clock().Add(5*time.Minute)) {
		v.Add("occurredAt", "required and not in the future")
	}
	if len(in.EvidenceBindingIDs) > 0 {
		v.Add("evidenceBindingIds", "evidence attachments are not supported yet")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *VehicleRow
	err := s.store.InTx(ctx, func(st Store) error {
		if _, err := st.Vehicles().Get(ctx, p.CompanyID, id); err != nil {
			return err
		}
		ids := []string{in.FromWarehouseID, in.ToWarehouseID}
		slices.Sort(ids)
		locked := map[string]*Warehouse{}
		for _, wid := range ids {
			w, err := st.Warehouses().Lock(ctx, p.CompanyID, wid)
			if errors.Is(err, apperr.ErrNotFound) {
				return apperr.FieldError("toWarehouseId", "both warehouses must belong to your company")
			} else if err != nil {
				return err
			}
			locked[wid] = w
		}
		placement, err := st.Vehicles().LockPlacement(ctx, id)
		if errors.Is(err, apperr.ErrNotFound) {
			return apperr.New(apperr.ErrConflict, "not_in_warehouse", "the vehicle is not in a warehouse")
		} else if err != nil {
			return err
		}
		if placement.WarehouseID != in.FromWarehouseID {
			return apperr.New(apperr.ErrConflict, "placement_changed", "the vehicle is no longer in the source warehouse, reload")
		}
		to := locked[in.ToWarehouseID]
		occ, err := st.Warehouses().Occupied(ctx, []string{to.ID})
		if err != nil {
			return err
		}
		if occ[to.ID]+1 > to.Capacity {
			return apperr.New(apperr.ErrConflict, "capacity_exceeded", "not enough free space in the destination warehouse")
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
