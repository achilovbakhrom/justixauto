package identity

import (
	"context"

	"gorm.io/gorm"
)

type AuditFilter struct {
	ResourceType string
	ResourceID   string
	ActorID      string
	Limit        int
	Offset       int
}

type AuditRepository interface {
	Append(ctx context.Context, e *AuditEvent) error
	List(ctx context.Context, f AuditFilter) ([]AuditEvent, error)
}

type auditRepository struct{ db *gorm.DB }

func (r *auditRepository) Append(ctx context.Context, e *AuditEvent) error {
	return translate(r.db.WithContext(ctx).Create(e).Error)
}

func (r *auditRepository) List(ctx context.Context, f AuditFilter) ([]AuditEvent, error) {
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
	events := []AuditEvent{}
	return events, translate(q.Find(&events).Error)
}
