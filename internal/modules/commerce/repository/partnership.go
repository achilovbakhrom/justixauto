package repository

import (
	"context"

	"gorm.io/gorm"

	"justixauto/internal/modules/commerce/model"
	"justixauto/internal/pkg/database"
)

type PartnershipRepository struct{ db *gorm.DB }

func (r *PartnershipRepository) Create(ctx context.Context, p *model.Partnership) error {
	return database.Translate(r.db.WithContext(ctx).Create(p).Error)
}

func (r *PartnershipRepository) involving(ctx context.Context, companyID string) *gorm.DB {
	return r.db.WithContext(ctx).Where("(requester_company_id = ? OR recipient_company_id = ?)", companyID, companyID)
}

// Get returns a partnership the company takes part in, else ErrNotFound.
func (r *PartnershipRepository) Get(ctx context.Context, companyID, id string) (*model.Partnership, error) {
	var p model.Partnership
	if err := r.involving(ctx, companyID).Where("id = ?", id).Take(&p).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &p, nil
}

func (r *PartnershipRepository) List(ctx context.Context, f model.PartnershipFilter) ([]model.Partnership, error) {
	limit, offset := database.Page(f.Limit, f.Offset)
	q := r.involving(ctx, f.CompanyID).Order("updated_at DESC, id").Limit(limit).Offset(offset)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	ps := []model.Partnership{}
	if err := database.Translate(q.Find(&ps).Error); err != nil {
		return nil, err
	}
	return ps, nil
}

func (r *PartnershipRepository) Update(ctx context.Context, p *model.Partnership, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.Partnership{}, p.ID, expected, map[string]any{
		"status": p.Status, "status_reason": p.StatusReason, "updated_at": p.UpdatedAt,
		"activated_at": p.ActivatedAt, "closed_at": p.ClosedAt,
	})
	if err == nil {
		p.Version = expected + 1
	}
	return err
}

// ActiveBetween reports whether the two companies have an active partnership.
func (r *PartnershipRepository) ActiveBetween(ctx context.Context, a, b string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Partnership{}).Where(
		"status = ? AND ((requester_company_id = ? AND recipient_company_id = ?) OR (requester_company_id = ? AND recipient_company_id = ?))",
		model.Active, a, b, b, a).Count(&n).Error
	return n > 0, database.Translate(err)
}

type EventRepository struct{ db *gorm.DB }

func (r *EventRepository) Append(ctx context.Context, e *model.Event) error {
	return database.Translate(r.db.WithContext(ctx).Create(e).Error)
}

func (r *EventRepository) ForResource(ctx context.Context, resourceType, id string) ([]model.Event, error) {
	es := []model.Event{}
	err := r.db.WithContext(ctx).Where("resource_type = ? AND resource_id = ?", resourceType, id).Order("seq").Find(&es).Error
	return es, database.Translate(err)
}
