package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/database"
)

// Reservation holds a vehicle for one deal. At most one hold per vehicle is
// active, whichever module (commerce order, retail deal) asks for it.
type Reservation struct {
	ID         string `gorm:"primaryKey;type:uuid"`
	VehicleID  string `gorm:"type:uuid"`
	CompanyID  string `gorm:"type:uuid"`
	HolderType string
	HolderID   string `gorm:"type:uuid"`
	Status     string // held | released | finalized
	Reason     string
	CreatedAt  time.Time
	ClosedAt   *time.Time
}

func (Reservation) TableName() string { return "inventory.reservations" }

// Holder identifies the deal that holds vehicles.
type Holder struct {
	Type string // commerce-order | retail-deal
	ID   string
}

// VehicleInfo is what other modules may learn about a vehicle.
type VehicleInfo struct {
	ID, VIN, ModelID string
	OwnerCompanyID   string
	WarehouseID      string // "" when outside any warehouse
}

var errUnavailable = apperr.New(apperr.ErrConflict, "vehicle_unavailable", "a vehicle is not available: unknown, not yours or already reserved")

// StockService is used by other modules (through their ports) to reserve,
// release and hand over vehicles. Calls join the caller's transaction when
// ctx carries one (database.WithTx).
type StockService struct{ deps }

func (s *StockService) db(ctx context.Context) *gorm.DB {
	return database.Conn(ctx, s.store.(*gormStore).db).WithContext(ctx)
}

// Vehicle returns a vehicle owned by companyID or stored in its warehouses.
func (s *StockService) Vehicle(ctx context.Context, companyID, id string) (*VehicleInfo, error) {
	if uuid.Validate(id) != nil {
		return nil, apperr.ErrNotFound
	}
	row, err := (&vehicleRepository{s.db(ctx)}).Get(ctx, companyID, id)
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
func (s *StockService) Reserve(ctx context.Context, companyID string, h Holder, vehicleIDs []string) error {
	ids := slices.Clone(vehicleIDs)
	slices.Sort(ids)
	return s.store.InTx(ctx, func(st Store) error {
		db := st.(*gormStore).db
		now := s.clock()
		for _, id := range ids {
			var unit VehicleUnit
			err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Take(&unit).Error
			if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && (unit.OwnerCompanyID == nil || *unit.OwnerCompanyID != companyID)) {
				return errUnavailable
			} else if err != nil {
				return database.Translate(err)
			}
			var held Reservation
			err = db.Where("vehicle_id = ? AND status = 'held'", id).Take(&held).Error
			if err == nil {
				if held.HolderType == h.Type && held.HolderID == h.ID {
					continue
				}
				return errUnavailable
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return database.Translate(err)
			}
			r := Reservation{
				ID: uuid.NewString(), VehicleID: id, CompanyID: companyID, HolderType: h.Type,
				HolderID: h.ID, Status: "held", CreatedAt: now,
			}
			if err := db.Create(&r).Error; err != nil {
				if errors.Is(database.Translate(err), apperr.ErrConflict) {
					return errUnavailable
				}
				return database.Translate(err)
			}
		}
		return nil
	})
}

// Release frees the holder's vehicles (nil = all of them).
func (s *StockService) Release(ctx context.Context, h Holder, vehicleIDs []string, reason string) error {
	q := s.db(ctx).Model(&Reservation{}).Where("holder_type = ? AND holder_id = ? AND status = 'held'", h.Type, h.ID)
	if vehicleIDs != nil {
		q = q.Where("vehicle_id IN ?", vehicleIDs)
	}
	return database.Translate(q.Updates(map[string]any{"status": "released", "reason": reason, "closed_at": s.clock()}).Error)
}

// Held lists the vehicles currently held by the holder.
func (s *StockService) Held(ctx context.Context, h Holder) ([]string, error) {
	var ids []string
	err := s.db(ctx).Model(&Reservation{}).Where("holder_type = ? AND holder_id = ? AND status = 'held'", h.Type, h.ID).
		Order("vehicle_id").Pluck("vehicle_id", &ids).Error
	return ids, database.Translate(err)
}

// Handover is the physical and legal hand-over of held vehicles to another
// company, received into one of its warehouses.
type Handover struct {
	Holder        Holder
	VehicleIDs    []string
	ToCompanyID   string
	ToWarehouseID string
	ActorUserID   string
	At            time.Time
}

// lockHandoverTarget locks the receiving warehouse, checks that every vehicle
// in the handover is currently held by t.Holder and that the warehouse has
// enough free capacity for the incoming vehicles.
func lockHandoverTarget(ctx context.Context, st Store, db *gorm.DB, t Handover) (*Warehouse, error) {
	to, err := st.Warehouses().Lock(ctx, t.ToCompanyID, t.ToWarehouseID)
	if errors.Is(err, apperr.ErrNotFound) {
		return nil, apperr.FieldError("warehouseId", "not one of your warehouses")
	} else if err != nil {
		return nil, err
	}
	var held int64
	if err := db.Model(&Reservation{}).Where("holder_type = ? AND holder_id = ? AND status = 'held' AND vehicle_id IN ?",
		t.Holder.Type, t.Holder.ID, t.VehicleIDs).Count(&held).Error; err != nil {
		return nil, database.Translate(err)
	}
	if int(held) != len(t.VehicleIDs) {
		return nil, errUnavailable
	}
	occ, err := st.Warehouses().Occupied(ctx, []string{to.ID})
	if err != nil {
		return nil, err
	}
	var alreadyThere int64
	if err := db.Model(&Placement{}).Where("warehouse_id = ? AND vehicle_id IN ?", to.ID, t.VehicleIDs).Count(&alreadyThere).Error; err != nil {
		return nil, database.Translate(err)
	}
	if occ[to.ID]+len(t.VehicleIDs)-int(alreadyThere) > to.Capacity {
		return nil, apperr.New(apperr.ErrConflict, "capacity_exceeded", "not enough free space in the receiving warehouse")
	}
	return to, nil
}

// transferVehicle moves one vehicle's ownership, custody and placement to the
// receiving warehouse and returns the fact recording the hand-over. Any
// warehouse the vehicle is leaving has its version bumped.
func transferVehicle(db *gorm.DB, t Handover, to *Warehouse, id string, now time.Time) (Fact, error) {
	var from Placement
	fromErr := db.Where("vehicle_id = ?", id).Take(&from).Error
	if err := db.Model(&VehicleUnit{}).Where("id = ?", id).Updates(map[string]any{
		"owner_company_id": t.ToCompanyID, "custodian_company_id": t.ToCompanyID, "version": gorm.Expr("version + 1"),
	}).Error; err != nil {
		return Fact{}, database.Translate(err)
	}
	placement := Placement{VehicleID: id, WarehouseID: to.ID, PlacedAt: t.At}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "vehicle_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"warehouse_id", "receipt_batch_id", "placed_at"}),
	}).Create(&placement).Error; err != nil {
		return Fact{}, database.Translate(err)
	}
	details, _ := json.Marshal(map[string]any{"holderType": t.Holder.Type, "holderId": t.Holder.ID})
	vid := id
	fact := Fact{
		ID: uuid.NewString(), CompanyID: t.ToCompanyID, FactType: "vehicle.handed_over",
		VehicleID: &vid, WarehouseID: &to.ID, ActorUserID: t.ActorUserID, OccurredAt: t.At, RecordedAt: now, Details: details,
	}
	if fromErr == nil {
		if err := db.Model(&Warehouse{}).Where("id = ?", from.WarehouseID).
			Updates(map[string]any{"version": gorm.Expr("version + 1"), "updated_at": now}).Error; err != nil {
			return Fact{}, database.Translate(err)
		}
	}
	return fact, nil
}

// Transfer completes a hand-over: the reservations are finalized, ownership
// and custody pass to the receiving company and the vehicles are placed in its
// warehouse, whose capacity is checked under a row lock.
func (s *StockService) Transfer(ctx context.Context, t Handover) error {
	return s.store.InTx(ctx, func(st Store) error {
		db := st.(*gormStore).db
		to, err := lockHandoverTarget(ctx, st, db, t)
		if err != nil {
			return err
		}
		now := s.clock()
		var facts []Fact
		for _, id := range t.VehicleIDs {
			fact, err := transferVehicle(db, t, to, id, now)
			if err != nil {
				return err
			}
			facts = append(facts, fact)
		}
		if err := db.Model(&Reservation{}).Where("holder_type = ? AND holder_id = ? AND status = 'held' AND vehicle_id IN ?",
			t.Holder.Type, t.Holder.ID, t.VehicleIDs).Updates(map[string]any{"status": "finalized", "closed_at": now}).Error; err != nil {
			return database.Translate(err)
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
func (s *StockService) Deliver(ctx context.Context, h Holder, vehicleID, actorID string, at time.Time) error {
	return s.store.InTx(ctx, func(st Store) error {
		db := st.(*gormStore).db
		var r Reservation
		err := db.Where("holder_type = ? AND holder_id = ? AND vehicle_id = ? AND status = 'held'", h.Type, h.ID, vehicleID).Take(&r).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errUnavailable
		} else if err != nil {
			return database.Translate(err)
		}
		now := s.clock()
		var p Placement
		if err := db.Where("vehicle_id = ?", vehicleID).Take(&p).Error; err == nil {
			if err := db.Delete(&Placement{}, "vehicle_id = ?", vehicleID).Error; err != nil {
				return database.Translate(err)
			}
			if err := db.Model(&Warehouse{}).Where("id = ?", p.WarehouseID).
				Updates(map[string]any{"version": gorm.Expr("version + 1"), "updated_at": now}).Error; err != nil {
				return database.Translate(err)
			}
		}
		if err := db.Model(&VehicleUnit{}).Where("id = ?", vehicleID).Updates(map[string]any{
			"owner_company_id": nil, "custodian_company_id": nil, "version": gorm.Expr("version + 1"),
		}).Error; err != nil {
			return database.Translate(err)
		}
		if err := db.Model(&r).Updates(map[string]any{"status": "finalized", "closed_at": now}).Error; err != nil {
			return database.Translate(err)
		}
		details, _ := json.Marshal(map[string]any{"holderType": h.Type, "holderId": h.ID})
		vid := vehicleID
		return st.Facts().Append(ctx, Fact{
			ID: uuid.NewString(), CompanyID: r.CompanyID, FactType: "vehicle.delivered_to_customer",
			VehicleID: &vid, ActorUserID: actorID, OccurredAt: at, RecordedAt: now, Details: details,
		})
	})
}
