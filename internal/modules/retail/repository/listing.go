package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"justixauto/internal/modules/retail/model"
	"justixauto/internal/pkg/database"
)

type ListingRepository struct{ db *gorm.DB }

func (r *ListingRepository) Create(ctx context.Context, l *model.Listing) error {
	return database.Translate(r.db.WithContext(ctx).Create(l).Error)
}

func (r *ListingRepository) Get(ctx context.Context, companyID, id string) (*model.Listing, error) {
	var l model.Listing
	if err := r.db.WithContext(ctx).Where("id = ? AND company_id = ?", id, companyID).Take(&l).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &l, nil
}

func (r *ListingRepository) List(ctx context.Context, companyID, status string, limit, offset int) ([]model.Listing, error) {
	limit, offset = database.Page(limit, offset)
	q := r.db.WithContext(ctx).Where("company_id = ?", companyID).Order("updated_at DESC, id").Limit(limit).Offset(offset)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	ls := []model.Listing{}
	if err := database.Translate(q.Find(&ls).Error); err != nil {
		return nil, err
	}
	return ls, nil
}

func (r *ListingRepository) Update(ctx context.Context, l *model.Listing, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.Listing{}, l.ID, expected, map[string]any{
		"text": l.Text, "asking_price_minor": l.AskingPriceMinor, "currency": l.Currency, "status": l.Status, "updated_at": l.UpdatedAt,
	})
	if err == nil {
		l.Version = expected + 1
	}
	return err
}

// WithdrawForVehicle closes open listings of a delivered vehicle.
func (r *ListingRepository) WithdrawForVehicle(ctx context.Context, vehicleID string, at time.Time) error {
	return database.Translate(r.db.WithContext(ctx).Model(&model.Listing{}).Where("vehicle_id = ? AND status <> 'withdrawn'", vehicleID).
		Updates(map[string]any{"status": "withdrawn", "updated_at": at, "version": gorm.Expr("version + 1")}).Error)
}
