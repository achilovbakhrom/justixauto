package repository

import (
	"context"

	"gorm.io/gorm"

	"justixauto/internal/modules/commerce/model"
	"justixauto/internal/pkg/database"
)

type FulfilmentRepository struct{ db *gorm.DB }

func (r *FulfilmentRepository) Allocations(ctx context.Context, orderID string) ([]model.Allocation, error) {
	as := []model.Allocation{}
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).Order("created_at, vin").Find(&as).Error
	return as, database.Translate(err)
}

func (r *FulfilmentRepository) AddAllocations(ctx context.Context, as []model.Allocation) error {
	return database.Translate(r.db.WithContext(ctx).Create(&as).Error)
}

func (r *FulfilmentRepository) SetAllocationStatus(ctx context.Context, orderID string, vehicleIDs []string, status string, shipmentID *string) error {
	fields := map[string]any{"status": status}
	if shipmentID != nil {
		fields["shipment_id"] = *shipmentID
	}
	q := r.db.WithContext(ctx).Model(&model.Allocation{}).Where("order_id = ? AND status IN ('allocated', 'shipped', 'delivered')", orderID)
	if vehicleIDs != nil {
		q = q.Where("vehicle_id IN ?", vehicleIDs)
	}
	return database.Translate(q.Updates(fields).Error)
}

func (r *FulfilmentRepository) CreateShipment(ctx context.Context, s *model.Shipment) error {
	return database.Translate(r.db.WithContext(ctx).Create(s).Error)
}

func (r *FulfilmentRepository) Shipment(ctx context.Context, id string) (*model.Shipment, error) {
	var s model.Shipment
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&s).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &s, nil
}

func (r *FulfilmentRepository) Shipments(ctx context.Context, orderID string) ([]model.Shipment, error) {
	ss := []model.Shipment{}
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).Order("created_at, id").Find(&ss).Error
	return ss, database.Translate(err)
}

func (r *FulfilmentRepository) UpdateShipment(ctx context.Context, s *model.Shipment, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.Shipment{}, s.ID, expected, map[string]any{"status": s.Status, "updated_at": s.UpdatedAt})
	if err == nil {
		s.Version = expected + 1
	}
	return err
}

func (r *FulfilmentRepository) AddMilestone(ctx context.Context, m *model.Milestone) error {
	return database.Translate(r.db.WithContext(ctx).Create(m).Error)
}

func (r *FulfilmentRepository) Milestones(ctx context.Context, shipmentID string) ([]model.Milestone, error) {
	ms := []model.Milestone{}
	err := r.db.WithContext(ctx).Where("shipment_id = ?", shipmentID).Order("occurred_at, recorded_at").Find(&ms).Error
	return ms, database.Translate(err)
}
