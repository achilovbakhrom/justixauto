package repository

import (
	"context"

	"gorm.io/gorm"

	"justixauto/internal/modules/inventory/model"
)

type FactRepository struct{ db *gorm.DB }

func (r *FactRepository) Append(ctx context.Context, facts ...model.Fact) error {
	if len(facts) == 0 {
		return nil
	}
	return translate(conn(ctx, r.db).Create(&facts).Error)
}

func (r *FactRepository) ForVehicle(ctx context.Context, vehicleID string) ([]model.Fact, error) {
	facts := []model.Fact{}
	err := conn(ctx, r.db).Where("vehicle_id = ?", vehicleID).Order("seq").Find(&facts).Error
	return facts, translate(err)
}
