package inventory

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/database"
)

// Store gives services the inventory repositories and one-transaction runs.
type Store interface {
	Models() ModelRepository
	Warehouses() WarehouseRepository
	Vehicles() VehicleRepository
	Facts() FactRepository
	InTx(ctx context.Context, fn func(Store) error) error
}

type gormStore struct{ db *gorm.DB }

func NewStore(db *gorm.DB) Store { return &gormStore{db: db} }

func (s *gormStore) Models() ModelRepository         { return &modelRepository{s.db} }
func (s *gormStore) Warehouses() WarehouseRepository { return &warehouseRepository{s.db} }
func (s *gormStore) Vehicles() VehicleRepository     { return &vehicleRepository{s.db} }
func (s *gormStore) Facts() FactRepository           { return &factRepository{s.db} }

// InTx runs fn in a transaction; inside a caller's ambient transaction
// (database.WithTx) it becomes a savepoint of that transaction.
func (s *gormStore) InTx(ctx context.Context, fn func(Store) error) error {
	return database.Conn(ctx, s.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(&gormStore{db: tx}) })
}

var translate = database.Translate

// ---- vehicle models ----

type ModelFilter struct {
	Query         string // matches make, model or variant
	Limit, Offset int
}

type ModelRepository interface {
	Create(ctx context.Context, m *VehicleModel, spec *Specification) error
	Get(ctx context.Context, id string) (*VehicleModel, error)
	List(ctx context.Context, f ModelFilter) ([]VehicleModel, error)
	Specs(ctx context.Context, modelID string) ([]Specification, error)
	Spec(ctx context.Context, modelID string, version int) (*Specification, error)
	// AddSpec stores a new version and makes it current if the model version matches.
	AddSpec(ctx context.Context, m *VehicleModel, expected int64, spec *Specification) error
}

type modelRepository struct{ db *gorm.DB }

func (r *modelRepository) Create(ctx context.Context, m *VehicleModel, spec *Specification) error {
	db := r.db.WithContext(ctx)
	if err := db.Create(m).Error; err != nil {
		return translate(err)
	}
	return translate(db.Create(spec).Error)
}

func (r *modelRepository) Get(ctx context.Context, id string) (*VehicleModel, error) {
	var m VehicleModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&m).Error; err != nil {
		return nil, translate(err)
	}
	return &m, nil
}

func (r *modelRepository) List(ctx context.Context, f ModelFilter) ([]VehicleModel, error) {
	limit, offset := database.Page(f.Limit, f.Offset)
	q := r.db.WithContext(ctx).Order("make, model, variant, id").Limit(limit).Offset(offset)
	if f.Query != "" {
		like := "%" + f.Query + "%"
		q = q.Where("(make ILIKE ? OR model ILIKE ? OR variant ILIKE ?)", like, like, like)
	}
	models := []VehicleModel{}
	return models, translate(q.Find(&models).Error)
}

func (r *modelRepository) Specs(ctx context.Context, modelID string) ([]Specification, error) {
	specs := []Specification{}
	err := r.db.WithContext(ctx).Where("model_id = ?", modelID).Order("spec_version").Find(&specs).Error
	return specs, translate(err)
}

func (r *modelRepository) Spec(ctx context.Context, modelID string, version int) (*Specification, error) {
	var s Specification
	if err := r.db.WithContext(ctx).Where("model_id = ? AND spec_version = ?", modelID, version).Take(&s).Error; err != nil {
		return nil, translate(err)
	}
	return &s, nil
}

func (r *modelRepository) AddSpec(ctx context.Context, m *VehicleModel, expected int64, spec *Specification) error {
	db := r.db.WithContext(ctx)
	err := database.UpdateVersioned(db, &VehicleModel{}, m.ID, expected, map[string]any{
		"current_spec_version": spec.SpecVersion, "updated_at": m.UpdatedAt,
	})
	if err != nil {
		return err
	}
	m.Version, m.CurrentSpecVersion = expected+1, spec.SpecVersion
	return translate(db.Create(spec).Error)
}

// ---- warehouses and receipt batches ----

type WarehouseFilter struct {
	CompanyID string
	// BranchIDs limits branch-bound warehouses (nil = all); company-wide
	// warehouses (no branch) are always included.
	BranchIDs []string
}

type WarehouseRepository interface {
	Create(ctx context.Context, w *Warehouse) error
	Get(ctx context.Context, companyID, id string) (*Warehouse, error)
	// Lock reads and row-locks a company's warehouse until the transaction ends.
	Lock(ctx context.Context, companyID, id string) (*Warehouse, error)
	List(ctx context.Context, f WarehouseFilter) ([]Warehouse, error)
	Update(ctx context.Context, w *Warehouse, expected int64) error
	// Touch bumps the version after stock in the warehouse changed.
	Touch(ctx context.Context, w *Warehouse, at time.Time) error
	// Occupied = placed vehicles + unidentified vehicles of active batches.
	Occupied(ctx context.Context, warehouseIDs []string) (map[string]int, error)

	CreateBatch(ctx context.Context, b *ReceiptBatch) error
	LockBatch(ctx context.Context, companyID, id string) (*ReceiptBatch, error)
	UpdateBatchCounts(ctx context.Context, b *ReceiptBatch) error
	OpenBatches(ctx context.Context, warehouseID string) ([]ReceiptBatch, error)
}

type warehouseRepository struct{ db *gorm.DB }

func (r *warehouseRepository) Create(ctx context.Context, w *Warehouse) error {
	return translate(r.db.WithContext(ctx).Create(w).Error)
}

func (r *warehouseRepository) Get(ctx context.Context, companyID, id string) (*Warehouse, error) {
	var w Warehouse
	if err := r.db.WithContext(ctx).Where("id = ? AND company_id = ?", id, companyID).Take(&w).Error; err != nil {
		return nil, translate(err)
	}
	return &w, nil
}

func (r *warehouseRepository) Lock(ctx context.Context, companyID, id string) (*Warehouse, error) {
	var w Warehouse
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND company_id = ?", id, companyID).Take(&w).Error
	if err != nil {
		return nil, translate(err)
	}
	return &w, nil
}

func (r *warehouseRepository) List(ctx context.Context, f WarehouseFilter) ([]Warehouse, error) {
	q := r.db.WithContext(ctx).Where("company_id = ?", f.CompanyID).Order("name, id")
	if f.BranchIDs != nil {
		q = q.Where("(branch_id IS NULL OR branch_id IN ?)", append(f.BranchIDs, "00000000-0000-0000-0000-000000000000"))
	}
	ws := []Warehouse{}
	return ws, translate(q.Find(&ws).Error)
}

func (r *warehouseRepository) Update(ctx context.Context, w *Warehouse, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Warehouse{}, w.ID, expected, map[string]any{
		"name": w.Name, "country": w.Country, "country_key": w.CountryKey, "region": w.Region,
		"region_key": w.RegionKey, "city": w.City, "address": w.Address, "capacity": w.Capacity,
		"updated_at": w.UpdatedAt,
	})
	if err == nil {
		w.Version = expected + 1
	}
	return err
}

func (r *warehouseRepository) Touch(ctx context.Context, w *Warehouse, at time.Time) error {
	err := r.db.WithContext(ctx).Model(&Warehouse{}).Where("id = ?", w.ID).
		Updates(map[string]any{"version": gorm.Expr("version + 1"), "updated_at": at}).Error
	if err == nil {
		w.Version++
		w.UpdatedAt = at
	}
	return translate(err)
}

func (r *warehouseRepository) Occupied(ctx context.Context, ids []string) (map[string]int, error) {
	out := make(map[string]int, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		WarehouseID string
		Occupied    int
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT warehouse_id, sum(n)::int AS occupied FROM (
			SELECT warehouse_id, count(*) AS n FROM inventory.placements WHERE warehouse_id IN ? GROUP BY warehouse_id
			UNION ALL
			SELECT warehouse_id, sum(unidentified_count) FROM inventory.receipt_batches WHERE warehouse_id IN ? GROUP BY warehouse_id
		) t GROUP BY warehouse_id`, ids, ids).Scan(&rows).Error
	if err != nil {
		return nil, translate(err)
	}
	for _, row := range rows {
		out[row.WarehouseID] = row.Occupied
	}
	return out, nil
}

func (r *warehouseRepository) CreateBatch(ctx context.Context, b *ReceiptBatch) error {
	return translate(r.db.WithContext(ctx).Create(b).Error)
}

func (r *warehouseRepository) LockBatch(ctx context.Context, companyID, id string) (*ReceiptBatch, error) {
	var b ReceiptBatch
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND company_id = ?", id, companyID).Take(&b).Error
	if err != nil {
		return nil, translate(err)
	}
	return &b, nil
}

func (r *warehouseRepository) UpdateBatchCounts(ctx context.Context, b *ReceiptBatch) error {
	res := r.db.WithContext(ctx).Model(&ReceiptBatch{}).Where("id = ?", b.ID).Updates(map[string]any{
		"confirmed_quantity": b.ConfirmedQuantity, "identified_count": b.IdentifiedCount, "unidentified_count": b.UnidentifiedCount, "version": b.Version + 1,
	})
	if res.Error == nil {
		b.Version++
	}
	return translate(res.Error)
}

func (r *warehouseRepository) OpenBatches(ctx context.Context, warehouseID string) ([]ReceiptBatch, error) {
	bs := []ReceiptBatch{}
	err := r.db.WithContext(ctx).Where("warehouse_id = ? AND unidentified_count > 0", warehouseID).
		Order("received_at, id").Find(&bs).Error
	return bs, translate(err)
}

// ---- vehicles and placements ----

type VehicleFilter struct {
	CompanyID   string
	Placement   string // "warehouse", "outside" or "any"
	WarehouseID string
	Limit       int
	Offset      int
}

// VehicleRow is a vehicle with its current placement (if any).
type VehicleRow struct {
	VehicleUnit
	WarehouseID    *string
	ReceiptBatchID *string
	PlacedAt       *time.Time
}

type VehicleRepository interface {
	// ExistingVINs returns which of the VINs are already registered (anywhere).
	ExistingVINs(ctx context.Context, vins []string) ([]string, error)
	Create(ctx context.Context, units []VehicleUnit, placements []Placement) error
	// Get returns a vehicle visible to the company: owned by it or stored in
	// one of its warehouses.
	Get(ctx context.Context, companyID, id string) (*VehicleRow, error)
	List(ctx context.Context, f VehicleFilter) ([]VehicleRow, error)
	InWarehouse(ctx context.Context, warehouseID string) ([]VehicleRow, error)
	// LockPlacement row-locks the vehicle's placement; ErrNotFound if outside.
	LockPlacement(ctx context.Context, vehicleID string) (*Placement, error)
	MovePlacement(ctx context.Context, vehicleID, toWarehouseID string, at time.Time) error
}

type vehicleRepository struct{ db *gorm.DB }

func (r *vehicleRepository) ExistingVINs(ctx context.Context, vins []string) ([]string, error) {
	found := []string{}
	if len(vins) == 0 {
		return found, nil
	}
	err := r.db.WithContext(ctx).Model(&VehicleUnit{}).Where("vin IN ?", vins).Order("vin").Pluck("vin", &found).Error
	return found, translate(err)
}

func (r *vehicleRepository) Create(ctx context.Context, units []VehicleUnit, placements []Placement) error {
	if len(units) == 0 {
		return nil
	}
	db := r.db.WithContext(ctx)
	if err := db.Create(&units).Error; err != nil {
		return translate(err)
	}
	return translate(db.Create(&placements).Error)
}

const vehicleSelect = `vehicle_units.*, p.warehouse_id, p.receipt_batch_id, p.placed_at`

func (r *vehicleRepository) visible(ctx context.Context, companyID string) *gorm.DB {
	return r.db.WithContext(ctx).Model(&VehicleUnit{}).Select(vehicleSelect).
		Joins("LEFT JOIN inventory.placements p ON p.vehicle_id = vehicle_units.id").
		Joins("LEFT JOIN inventory.warehouses w ON w.id = p.warehouse_id").
		Where("(vehicle_units.owner_company_id = ? OR w.company_id = ?)", companyID, companyID)
}

func (r *vehicleRepository) Get(ctx context.Context, companyID, id string) (*VehicleRow, error) {
	var rows []VehicleRow
	if err := r.visible(ctx, companyID).Where("vehicle_units.id = ?", id).Scan(&rows).Error; err != nil {
		return nil, translate(err)
	}
	if len(rows) == 0 {
		return nil, apperr.ErrNotFound
	}
	return &rows[0], nil
}

func (r *vehicleRepository) List(ctx context.Context, f VehicleFilter) ([]VehicleRow, error) {
	limit, offset := database.Page(f.Limit, f.Offset)
	q := r.visible(ctx, f.CompanyID).Order("vehicle_units.vin").Limit(limit).Offset(offset)
	switch f.Placement {
	case "warehouse":
		q = q.Where("p.vehicle_id IS NOT NULL")
	case "outside":
		q = q.Where("p.vehicle_id IS NULL")
	}
	if f.WarehouseID != "" {
		q = q.Where("p.warehouse_id = ?", f.WarehouseID)
	}
	rows := []VehicleRow{}
	return rows, translate(q.Scan(&rows).Error)
}

func (r *vehicleRepository) InWarehouse(ctx context.Context, warehouseID string) ([]VehicleRow, error) {
	rows := []VehicleRow{}
	err := r.db.WithContext(ctx).Model(&VehicleUnit{}).Select(vehicleSelect).
		Joins("JOIN inventory.placements p ON p.vehicle_id = vehicle_units.id AND p.warehouse_id = ?", warehouseID).
		Order("vehicle_units.vin").Scan(&rows).Error
	return rows, translate(err)
}

func (r *vehicleRepository) LockPlacement(ctx context.Context, vehicleID string) (*Placement, error) {
	var p Placement
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("vehicle_id = ?", vehicleID).Take(&p).Error
	if err != nil {
		return nil, translate(err)
	}
	return &p, nil
}

func (r *vehicleRepository) MovePlacement(ctx context.Context, vehicleID, toWarehouseID string, at time.Time) error {
	return translate(r.db.WithContext(ctx).Model(&Placement{}).Where("vehicle_id = ?", vehicleID).
		Updates(map[string]any{"warehouse_id": toWarehouseID, "receipt_batch_id": nil, "placed_at": at}).Error)
}

// ---- facts ----

type FactRepository interface {
	Append(ctx context.Context, facts ...Fact) error
	ForVehicle(ctx context.Context, vehicleID string) ([]Fact, error)
}

type factRepository struct{ db *gorm.DB }

func (r *factRepository) Append(ctx context.Context, facts ...Fact) error {
	if len(facts) == 0 {
		return nil
	}
	return translate(r.db.WithContext(ctx).Create(&facts).Error)
}

func (r *factRepository) ForVehicle(ctx context.Context, vehicleID string) ([]Fact, error) {
	facts := []Fact{}
	err := r.db.WithContext(ctx).Where("vehicle_id = ?", vehicleID).Order("seq").Find(&facts).Error
	return facts, translate(err)
}
