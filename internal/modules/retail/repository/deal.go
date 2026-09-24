package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/modules/retail/model"
	"justixauto/internal/pkg/database"
)

type DealRepository struct{ db *gorm.DB }

func (r *DealRepository) Create(ctx context.Context, d *model.Deal) error {
	return database.Translate(r.db.WithContext(ctx).Create(d).Error)
}

func (r *DealRepository) Deal(ctx context.Context, companyID, id string) (*model.Deal, error) {
	var d model.Deal
	if err := r.db.WithContext(ctx).Where("id = ? AND company_id = ?", id, companyID).Take(&d).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &d, nil
}

func (r *DealRepository) Deals(ctx context.Context, companyID string, branchIDs []string, status string, limit, offset int) ([]model.Deal, error) {
	limit, offset = database.Page(limit, offset)
	q := r.db.WithContext(ctx).Where("company_id = ?", companyID).Order("updated_at DESC, id").Limit(limit).Offset(offset)
	if branchIDs != nil {
		q = q.Where("branch_id IN ?", append(branchIDs, uuid.Nil.String()))
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	ds := []model.Deal{}
	if err := database.Translate(q.Find(&ds).Error); err != nil {
		return nil, err
	}
	return ds, nil
}

func (r *DealRepository) Update(ctx context.Context, d *model.Deal, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.Deal{}, d.ID, expected, map[string]any{
		"status": d.Status, "contract_signed_on": d.ContractSignedOn, "contract_reference": d.ContractReference, "contract_file_ids": d.ContractFileIDs,
		"registered_on": d.RegisteredOn, "plate_number": d.PlateNumber, "registration_reference": d.RegistrationReference,
		"delivered_at": d.DeliveredAt, "status_reason": d.StatusReason, "updated_at": d.UpdatedAt,
	})
	if err == nil {
		d.Version = expected + 1
	}
	return err
}

func (r *DealRepository) CreateInvoice(ctx context.Context, i *model.Invoice) error {
	return database.Translate(r.db.WithContext(ctx).Create(i).Error)
}

func (r *DealRepository) Invoice(ctx context.Context, companyID, id string) (*model.Invoice, error) {
	var i model.Invoice
	if err := r.db.WithContext(ctx).Where("id = ? AND company_id = ?", id, companyID).Take(&i).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &i, nil
}

func (r *DealRepository) Invoices(ctx context.Context, dealID string) ([]model.Invoice, error) {
	is := []model.Invoice{}
	err := r.db.WithContext(ctx).Where("deal_id = ?", dealID).Order("created_at").Find(&is).Error
	return is, database.Translate(err)
}

func (r *DealRepository) AddEvidence(ctx context.Context, e *model.Evidence) error {
	return database.Translate(r.db.WithContext(ctx).Create(e).Error)
}

func (r *DealRepository) Evidence(ctx context.Context, invoiceID string) ([]model.Evidence, error) {
	es := []model.Evidence{}
	err := r.db.WithContext(ctx).Where("invoice_id = ?", invoiceID).Order("seq").Find(&es).Error
	return es, database.Translate(err)
}

func (r *DealRepository) GetEvidence(ctx context.Context, id string) (*model.Evidence, error) {
	var e model.Evidence
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&e).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &e, nil
}

func (r *DealRepository) UpdateEvidence(ctx context.Context, e *model.Evidence, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.Evidence{}, e.ID, expected, map[string]any{
		"status": e.Status, "decision_reason": e.DecisionReason, "decided_by": e.DecidedBy, "decided_at": e.DecidedAt,
	})
	if err == nil {
		e.Version = expected + 1
	}
	return err
}
