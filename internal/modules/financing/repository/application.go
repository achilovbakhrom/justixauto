package repository

import (
	"context"

	"justixauto/internal/modules/financing/model"
	"justixauto/internal/pkg/database"
)

const visible = "(seller_company_id = ? OR (provider_company_id = ? AND status <> 'draft'))"

func (r *Store) CreateApplication(ctx context.Context, a *model.Application) error {
	return r.create(ctx, a)
}

func (r *Store) Application(ctx context.Context, companyID, id string) (*model.Application, error) {
	var a model.Application
	if err := r.db.WithContext(ctx).Where("id = ? AND "+visible, id, companyID, companyID).Take(&a).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &a, nil
}

func (r *Store) Applications(ctx context.Context, companyID, status string, limit, offset int) ([]model.Application, error) {
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

func (r *Store) UpdateApplication(ctx context.Context, a *model.Application, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.Application{}, a.ID, expected, map[string]any{
		"provider_company_id": a.ProviderCompanyID, "program_id": a.ProgramID, "program_version": a.ProgramVersion,
		"calculation": a.Calculation, "calculation_digest": a.CalculationDigest, "status": a.Status, "snapshot": a.Snapshot,
		"current_terms_version": a.CurrentTermsVersion, "updated_at": a.UpdatedAt, "submitted_at": a.SubmittedAt,
	})
	if err == nil {
		a.Version = expected + 1
	}
	return err
}

func (r *Store) CreateTermsVersion(ctx context.Context, t *model.TermsVersion) error {
	return r.create(ctx, t)
}

func (r *Store) TermsVersions(ctx context.Context, applicationID string) ([]model.TermsVersion, error) {
	ts := []model.TermsVersion{}
	err := r.db.WithContext(ctx).Where("application_id = ?", applicationID).Order("number").Find(&ts).Error
	return ts, database.Translate(err)
}

func (r *Store) NextTermsVersionNumber(ctx context.Context, applicationID string) (int, error) {
	return r.nextNumber(ctx, &model.TermsVersion{}, "application_id", applicationID)
}

func (r *Store) CreateMessage(ctx context.Context, m *model.Message) error { return r.create(ctx, m) }

func (r *Store) Messages(ctx context.Context, applicationID string) ([]model.Message, error) {
	ms := []model.Message{}
	err := r.db.WithContext(ctx).Where("application_id = ?", applicationID).Order("seq").Find(&ms).Error
	return ms, database.Translate(err)
}
