package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"justixauto/internal/modules/inventory/model"
)

// ReservationRepository is the persistence for vehicle holds.
type ReservationRepository struct{ db *gorm.DB }

// HeldByVehicle returns the active hold on a vehicle, if any.
func (r *ReservationRepository) HeldByVehicle(ctx context.Context, vehicleID string) (*model.Reservation, error) {
	var res model.Reservation
	err := conn(ctx, r.db).Where("vehicle_id = ? AND status = 'held'", vehicleID).Take(&res).Error
	if err != nil {
		return nil, translate(err)
	}
	return &res, nil
}

// HeldFor returns the holder's active hold on a vehicle, if any.
func (r *ReservationRepository) HeldFor(ctx context.Context, holderType, holderID, vehicleID string) (*model.Reservation, error) {
	var res model.Reservation
	err := conn(ctx, r.db).Where("holder_type = ? AND holder_id = ? AND vehicle_id = ? AND status = 'held'", holderType, holderID, vehicleID).Take(&res).Error
	if err != nil {
		return nil, translate(err)
	}
	return &res, nil
}

func (r *ReservationRepository) Create(ctx context.Context, res *model.Reservation) error {
	return translate(conn(ctx, r.db).Create(res).Error)
}

// Release marks the holder's held reservations released (nil vehicleIDs = all).
func (r *ReservationRepository) Release(ctx context.Context, holderType, holderID string, vehicleIDs []string, reason string, at time.Time) error {
	q := conn(ctx, r.db).Model(&model.Reservation{}).Where("holder_type = ? AND holder_id = ? AND status = 'held'", holderType, holderID)
	if vehicleIDs != nil {
		q = q.Where("vehicle_id IN ?", vehicleIDs)
	}
	return translate(q.Updates(map[string]any{"status": "released", "reason": reason, "closed_at": at}).Error)
}

// ListHeld returns the vehicle IDs the holder currently holds.
func (r *ReservationRepository) ListHeld(ctx context.Context, holderType, holderID string) ([]string, error) {
	var ids []string
	err := conn(ctx, r.db).Model(&model.Reservation{}).Where("holder_type = ? AND holder_id = ? AND status = 'held'", holderType, holderID).
		Order("vehicle_id").Pluck("vehicle_id", &ids).Error
	return ids, translate(err)
}

// CountHeld counts how many of vehicleIDs the holder currently holds.
func (r *ReservationRepository) CountHeld(ctx context.Context, holderType, holderID string, vehicleIDs []string) (int, error) {
	var n int64
	err := conn(ctx, r.db).Model(&model.Reservation{}).
		Where("holder_type = ? AND holder_id = ? AND status = 'held' AND vehicle_id IN ?", holderType, holderID, vehicleIDs).Count(&n).Error
	return int(n), translate(err)
}

// Finalize marks the holder's held reservations on vehicleIDs finalized.
func (r *ReservationRepository) Finalize(ctx context.Context, holderType, holderID string, vehicleIDs []string, at time.Time) error {
	err := conn(ctx, r.db).Model(&model.Reservation{}).
		Where("holder_type = ? AND holder_id = ? AND status = 'held' AND vehicle_id IN ?", holderType, holderID, vehicleIDs).
		Updates(map[string]any{"status": "finalized", "closed_at": at}).Error
	return translate(err)
}

// FinalizeOne finalizes a single reservation by ID.
func (r *ReservationRepository) FinalizeOne(ctx context.Context, id string, at time.Time) error {
	err := conn(ctx, r.db).Model(&model.Reservation{}).Where("id = ?", id).
		Updates(map[string]any{"status": "finalized", "closed_at": at}).Error
	return translate(err)
}
