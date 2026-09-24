// Package repository holds insurance's GORM persistence. It implements the
// Repository interface declared in service/ports.go structurally: it must
// never import the service package.
package repository

import (
	"context"

	"gorm.io/gorm"

	"justixauto/internal/modules/insurance/model"
	"justixauto/internal/pkg/database"
)

// Store is insurance's GORM persistence.
type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db} }

func (r *Store) InTx(ctx context.Context, fn func(*Store) error) error {
	return database.Conn(ctx, r.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(&Store{tx}) })
}

func (r *Store) Create(ctx context.Context, a *model.Application) error {
	return database.Translate(r.db.WithContext(ctx).Create(a).Error)
}

const visible = "(seller_company_id = ? OR (insurer_company_id = ? AND status <> 'draft'))"

// Get returns an application visible to the company: the seller always, the
// addressed insurer once it was submitted.
func (r *Store) Get(ctx context.Context, companyID, id string) (*model.Application, error) {
	var a model.Application
	if err := r.db.WithContext(ctx).Where("id = ? AND "+visible, id, companyID, companyID).Take(&a).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &a, nil
}

func (r *Store) List(ctx context.Context, companyID, status string, limit, offset int) ([]model.Application, error) {
	limit, offset = database.Page(limit, offset)
	q := r.db.WithContext(ctx).Where(visible, companyID, companyID).Order("updated_at DESC, id").Limit(limit).Offset(offset)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	as := []model.Application{}
	if err := database.Translate(q.Find(&as).Error); err != nil {
		return nil, err
	}
	return as, nil
}

func (r *Store) ForDeal(ctx context.Context, sellerID, dealID string) (*model.Application, error) {
	var a model.Application
	if err := r.db.WithContext(ctx).Where("seller_company_id = ? AND deal_id = ?", sellerID, dealID).Take(&a).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &a, nil
}

func (r *Store) Update(ctx context.Context, a *model.Application, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.Application{}, a.ID, expected, map[string]any{
		"insurer_company_id": a.InsurerCompanyID, "status": a.Status, "note": a.Note, "snapshot": a.Snapshot,
		"updated_at": a.UpdatedAt, "submitted_at": a.SubmittedAt, "decided_at": a.DecidedAt,
	})
	if err == nil {
		a.Version = expected + 1
	}
	return err
}

func (r *Store) AddMessage(ctx context.Context, m *model.Message) error {
	return database.Translate(r.db.WithContext(ctx).Create(m).Error)
}

func (r *Store) Messages(ctx context.Context, applicationID string) ([]model.Message, error) {
	ms := []model.Message{}
	err := r.db.WithContext(ctx).Where("application_id = ?", applicationID).Order("seq").Find(&ms).Error
	return ms, database.Translate(err)
}
