package repository

import (
	"context"

	"gorm.io/gorm"

	"justixauto/internal/modules/retail/model"
	"justixauto/internal/pkg/database"
)

type EventRepository struct{ db *gorm.DB }

func (r *EventRepository) Append(ctx context.Context, e *model.Event) error {
	return database.Translate(r.db.WithContext(ctx).Create(e).Error)
}

func (r *EventRepository) For(ctx context.Context, resourceType, id string) ([]model.Event, error) {
	es := []model.Event{}
	err := r.db.WithContext(ctx).Where("resource_type = ? AND resource_id = ?", resourceType, id).Order("seq").Find(&es).Error
	return es, database.Translate(err)
}
