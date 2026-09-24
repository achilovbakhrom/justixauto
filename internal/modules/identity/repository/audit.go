package repository

import (
	"context"

	"gorm.io/gorm"

	"justixauto/internal/modules/identity/model"
)

type AuditRepository struct{ db *gorm.DB }

func (r *AuditRepository) Append(ctx context.Context, e *model.AuditEvent) error {
	return translate(r.db.WithContext(ctx).Create(e).Error)
}

func (r *AuditRepository) List(ctx context.Context, f model.AuditFilter) ([]model.AuditEvent, error) {
	limit, offset := pageDefaults(f.Limit, f.Offset)
	q := r.db.WithContext(ctx).Order("seq DESC").Limit(limit).Offset(offset)
	if f.ResourceType != "" {
		q = q.Where("resource_type = ?", f.ResourceType)
	}
	if f.ResourceID != "" {
		q = q.Where("resource_id = ?", f.ResourceID)
	}
	if f.ActorID != "" {
		q = q.Where("actor_user_id = ?", f.ActorID)
	}
	events := []model.AuditEvent{}
	if err := translate(q.Find(&events).Error); err != nil {
		return nil, err
	}
	return events, nil
}
