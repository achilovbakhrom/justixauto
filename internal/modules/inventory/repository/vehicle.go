package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"justixauto/internal/modules/inventory/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/database"
)

type VehicleRepository struct{ db *gorm.DB }

func (r *VehicleRepository) ExistingVINs(ctx context.Context, vins []string) ([]string, error) {
	found := []string{}
	if len(vins) == 0 {
		return found, nil
	}
	err := conn(ctx, r.db).Model(&model.VehicleUnit{}).Where("vin IN ?", vins).Order("vin").Pluck("vin", &found).Error
	return found, translate(err)
}

func (r *VehicleRepository) Create(ctx context.Context, units []model.VehicleUnit, placements []model.Placement) error {
	if len(units) == 0 {
		return nil
	}
	db := conn(ctx, r.db)
	if err := db.Create(&units).Error; err != nil {
		return translate(err)
	}
	return translate(db.Create(&placements).Error)
}

const vehicleSelect = `vehicle_units.*, p.warehouse_id, p.receipt_batch_id, p.placed_at, ` +
	`EXISTS (SELECT 1 FROM inventory.reservations r WHERE r.vehicle_id = vehicle_units.id AND r.status = 'held') AS reserved`

func (r *VehicleRepository) visible(ctx context.Context, companyID string) *gorm.DB {
	return conn(ctx, r.db).Model(&model.VehicleUnit{}).Select(vehicleSelect).
		Joins("LEFT JOIN inventory.placements p ON p.vehicle_id = vehicle_units.id").
		Joins("LEFT JOIN inventory.warehouses w ON w.id = p.warehouse_id").
		Where("(vehicle_units.owner_company_id = ? OR w.company_id = ?)", companyID, companyID)
}

func (r *VehicleRepository) Get(ctx context.Context, companyID, id string) (*model.VehicleRow, error) {
	var rows []model.VehicleRow
	if err := r.visible(ctx, companyID).Where("vehicle_units.id = ?", id).Scan(&rows).Error; err != nil {
		return nil, translate(err)
	}
	if len(rows) == 0 {
		return nil, apperr.ErrNotFound
	}
	return &rows[0], nil
}

func (r *VehicleRepository) List(ctx context.Context, f model.VehicleFilter) ([]model.VehicleRow, error) {
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
	rows := []model.VehicleRow{}
	if err := translate(q.Scan(&rows).Error); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *VehicleRepository) InWarehouse(ctx context.Context, warehouseID string) ([]model.VehicleRow, error) {
	rows := []model.VehicleRow{}
	err := conn(ctx, r.db).Model(&model.VehicleUnit{}).Select(vehicleSelect).
		Joins("JOIN inventory.placements p ON p.vehicle_id = vehicle_units.id AND p.warehouse_id = ?", warehouseID).
		Order("vehicle_units.vin").Scan(&rows).Error
	return rows, translate(err)
}

func (r *VehicleRepository) LockPlacement(ctx context.Context, vehicleID string) (*model.Placement, error) {
	var p model.Placement
	err := conn(ctx, r.db).Clauses(clause.Locking{Strength: "UPDATE"}).Where("vehicle_id = ?", vehicleID).Take(&p).Error
	if err != nil {
		return nil, translate(err)
	}
	return &p, nil
}

func (r *VehicleRepository) MovePlacement(ctx context.Context, vehicleID, toWarehouseID string, at time.Time) error {
	return translate(conn(ctx, r.db).Model(&model.Placement{}).Where("vehicle_id = ?", vehicleID).
		Updates(map[string]any{"warehouse_id": toWarehouseID, "receipt_batch_id": nil, "placed_at": at}).Error)
}

// LockOwned row-locks a vehicle unit by ID; ErrNotFound if it does not exist.
func (r *VehicleRepository) LockOwned(ctx context.Context, id string) (*model.VehicleUnit, error) {
	var u model.VehicleUnit
	err := conn(ctx, r.db).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Take(&u).Error
	if err != nil {
		return nil, translate(err)
	}
	return &u, nil
}

// TransferOwnership sets a vehicle's owner and custodian to toCompanyID.
func (r *VehicleRepository) TransferOwnership(ctx context.Context, id, toCompanyID string) error {
	err := conn(ctx, r.db).Model(&model.VehicleUnit{}).Where("id = ?", id).Updates(map[string]any{
		"owner_company_id": toCompanyID, "custodian_company_id": toCompanyID, "version": gorm.Expr("version + 1"),
	}).Error
	return translate(err)
}

// ClearOwnership sets a vehicle's owner and custodian to none.
func (r *VehicleRepository) ClearOwnership(ctx context.Context, id string) error {
	err := conn(ctx, r.db).Model(&model.VehicleUnit{}).Where("id = ?", id).Updates(map[string]any{
		"owner_company_id": nil, "custodian_company_id": nil, "version": gorm.Expr("version + 1"),
	}).Error
	return translate(err)
}

// PlacementOf reads a vehicle's placement without locking it.
func (r *VehicleRepository) PlacementOf(ctx context.Context, vehicleID string) (*model.Placement, error) {
	var p model.Placement
	if err := conn(ctx, r.db).Where("vehicle_id = ?", vehicleID).Take(&p).Error; err != nil {
		return nil, translate(err)
	}
	return &p, nil
}

// UpsertPlacement creates or replaces a vehicle's placement.
func (r *VehicleRepository) UpsertPlacement(ctx context.Context, p model.Placement) error {
	err := conn(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "vehicle_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"warehouse_id", "receipt_batch_id", "placed_at"}),
	}).Create(&p).Error
	return translate(err)
}

// DeletePlacement removes a vehicle's placement, returning it if one existed.
func (r *VehicleRepository) DeletePlacement(ctx context.Context, vehicleID string) (*model.Placement, error) {
	var p model.Placement
	if err := conn(ctx, r.db).Where("vehicle_id = ?", vehicleID).Take(&p).Error; err != nil {
		return nil, translate(err)
	}
	if err := conn(ctx, r.db).Delete(&model.Placement{}, "vehicle_id = ?", vehicleID).Error; err != nil {
		return nil, translate(err)
	}
	return &p, nil
}

// CountPlaced counts how many of vehicleIDs are placed in warehouseID.
func (r *VehicleRepository) CountPlaced(ctx context.Context, warehouseID string, vehicleIDs []string) (int, error) {
	var n int64
	err := conn(ctx, r.db).Model(&model.Placement{}).Where("warehouse_id = ? AND vehicle_id IN ?", warehouseID, vehicleIDs).Count(&n).Error
	return int(n), translate(err)
}
