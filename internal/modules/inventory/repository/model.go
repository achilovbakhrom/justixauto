package repository

import (
	"context"

	"gorm.io/gorm"

	"justixauto/internal/modules/inventory/model"
	"justixauto/internal/pkg/database"
)

type ModelRepository struct{ db *gorm.DB }

func (r *ModelRepository) Create(ctx context.Context, m *model.VehicleModel, spec *model.Specification) error {
	db := conn(ctx, r.db)
	if err := db.Create(m).Error; err != nil {
		return translate(err)
	}
	return translate(db.Create(spec).Error)
}

func (r *ModelRepository) Get(ctx context.Context, id string) (*model.VehicleModel, error) {
	var m model.VehicleModel
	if err := conn(ctx, r.db).Where("id = ?", id).Take(&m).Error; err != nil {
		return nil, translate(err)
	}
	return &m, nil
}

func (r *ModelRepository) List(ctx context.Context, f model.ModelFilter) ([]model.VehicleModel, error) {
	limit, offset := database.Page(f.Limit, f.Offset)
	q := conn(ctx, r.db).Order("make, model, variant, id").Limit(limit).Offset(offset)
	if f.Query != "" {
		like := "%" + f.Query + "%"
		q = q.Where("(make ILIKE ? OR model ILIKE ? OR variant ILIKE ?)", like, like, like)
	}
	models := []model.VehicleModel{}
	if err := translate(q.Find(&models).Error); err != nil {
		return nil, err
	}
	return models, nil
}

func (r *ModelRepository) Specs(ctx context.Context, modelID string) ([]model.Specification, error) {
	specs := []model.Specification{}
	err := conn(ctx, r.db).Where("model_id = ?", modelID).Order("spec_version").Find(&specs).Error
	return specs, translate(err)
}

func (r *ModelRepository) Spec(ctx context.Context, modelID string, version int) (*model.Specification, error) {
	var s model.Specification
	if err := conn(ctx, r.db).Where("model_id = ? AND spec_version = ?", modelID, version).Take(&s).Error; err != nil {
		return nil, translate(err)
	}
	return &s, nil
}

func (r *ModelRepository) AddSpec(ctx context.Context, m *model.VehicleModel, expected int64, spec *model.Specification) error {
	db := conn(ctx, r.db)
	err := database.UpdateVersioned(db, &model.VehicleModel{}, m.ID, expected, map[string]any{
		"current_spec_version": spec.SpecVersion, "updated_at": m.UpdatedAt,
	})
	if err != nil {
		return err
	}
	m.Version, m.CurrentSpecVersion = expected+1, spec.SpecVersion
	return translate(db.Create(spec).Error)
}
