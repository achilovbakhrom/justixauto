package repository

import (
	"context"

	"gorm.io/gorm"

	"justixauto/internal/modules/commerce/model"
	"justixauto/internal/pkg/database"
)

type InvoiceRepository struct{ db *gorm.DB }

func (r *InvoiceRepository) Create(ctx context.Context, i *model.Invoice) error {
	return database.Translate(r.db.WithContext(ctx).Create(i).Error)
}

// Get returns an invoice the company is a party of.
func (r *InvoiceRepository) Get(ctx context.Context, companyID, id string) (*model.Invoice, error) {
	var i model.Invoice
	err := r.db.WithContext(ctx).Where("id = ? AND (supplier_company_id = ? OR buyer_company_id = ?)", id, companyID, companyID).Take(&i).Error
	if err != nil {
		return nil, database.Translate(err)
	}
	return &i, nil
}

func (r *InvoiceRepository) ForOrder(ctx context.Context, orderID string) ([]model.Invoice, error) {
	is := []model.Invoice{}
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).Order("created_at").Find(&is).Error
	return is, database.Translate(err)
}

func (r *InvoiceRepository) Update(ctx context.Context, i *model.Invoice, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.Invoice{}, i.ID, expected,
		map[string]any{"status": i.Status, "void_reason": i.VoidReason, "updated_at": i.UpdatedAt})
	if err == nil {
		i.Version = expected + 1
	}
	return err
}

func (r *InvoiceRepository) AddEvidence(ctx context.Context, e *model.Evidence) error {
	return database.Translate(r.db.WithContext(ctx).Create(e).Error)
}

func (r *InvoiceRepository) Evidence(ctx context.Context, invoiceID string) ([]model.Evidence, error) {
	es := []model.Evidence{}
	err := r.db.WithContext(ctx).Where("invoice_id = ?", invoiceID).Order("seq").Find(&es).Error
	return es, database.Translate(err)
}

func (r *InvoiceRepository) GetEvidence(ctx context.Context, id string) (*model.Evidence, error) {
	var e model.Evidence
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&e).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &e, nil
}

func (r *InvoiceRepository) UpdateEvidence(ctx context.Context, e *model.Evidence, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.Evidence{}, e.ID, expected, map[string]any{
		"status": e.Status, "decision_reason": e.DecisionReason, "decided_by": e.DecidedBy, "decided_at": e.DecidedAt,
	})
	if err == nil {
		e.Version = expected + 1
	}
	return err
}
