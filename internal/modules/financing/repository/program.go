package repository

import (
	"context"

	"justixauto/internal/modules/financing/model"
	"justixauto/internal/pkg/database"
)

func (r *Store) CreateProgram(ctx context.Context, p *model.Program) error { return r.create(ctx, p) }

func (r *Store) Program(ctx context.Context, id string) (*model.Program, error) {
	var p model.Program
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&p).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &p, nil
}

func (r *Store) Programs(ctx context.Context, providerID string, publishedOnly bool, limit, offset int) ([]model.Program, error) {
	limit, offset = database.Page(limit, offset)
	q := r.db.WithContext(ctx).Order("updated_at DESC, id").Limit(limit).Offset(offset)
	if providerID != "" {
		q = q.Where("provider_company_id = ?", providerID)
	}
	if publishedOnly {
		q = q.Where("status = 'published'")
	}
	ps := []model.Program{}
	if err := database.Translate(q.Find(&ps).Error); err != nil {
		return nil, err
	}
	return ps, nil
}

func (r *Store) UpdateProgram(ctx context.Context, p *model.Program, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.Program{}, p.ID, expected, map[string]any{
		"status": p.Status, "published_version": p.PublishedVersion, "status_reason": p.StatusReason, "updated_at": p.UpdatedAt,
	})
	if err == nil {
		p.Version = expected + 1
	}
	return err
}

func (r *Store) CreateProgramVersion(ctx context.Context, v *model.ProgramVersion) error {
	return r.create(ctx, v)
}

func (r *Store) ProgramVersion(ctx context.Context, id string, number int) (*model.ProgramVersion, error) {
	var v model.ProgramVersion
	if err := r.db.WithContext(ctx).Where("program_id = ? AND number = ?", id, number).Take(&v).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &v, nil
}

func (r *Store) ProgramVersions(ctx context.Context, id string) ([]model.ProgramVersion, error) {
	vs := []model.ProgramVersion{}
	err := r.db.WithContext(ctx).Where("program_id = ?", id).Order("number").Find(&vs).Error
	return vs, database.Translate(err)
}

func (r *Store) NextProgramVersionNumber(ctx context.Context, programID string) (int, error) {
	return r.nextNumber(ctx, &model.ProgramVersion{}, "program_id", programID)
}
