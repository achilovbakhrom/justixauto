package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"justixauto/internal/modules/inventory/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/validate"
)

// SpecInput is the contract's VehicleSpecification: nine separate fields.
type SpecInput struct {
	Make          string `json:"make"`
	Model         string `json:"model"`
	Variant       string `json:"variant"`
	Year          int    `json:"year"`
	BodyType      string `json:"bodyType"`
	ExteriorColor string `json:"exteriorColor"`
	InteriorColor string `json:"interiorColor"`
	Powertrain    string `json:"powertrain"`
	Drivetrain    string `json:"drivetrain"`
}

func (in SpecInput) validate(v *apperr.Validation) (brand, m, variant string, spec model.Specification) {
	brand = validate.Text(v, "specification.make", in.Make, 1, 100)
	m = validate.Text(v, "specification.model", in.Model, 1, 100)
	variant = validate.Text(v, "specification.variant", in.Variant, 1, 100)
	if in.Year < 1900 || in.Year > 2100 {
		v.Add("specification.year", "must be a year between 1900 and 2100")
	}
	spec = model.Specification{
		Year:          in.Year,
		BodyType:      validate.Text(v, "specification.bodyType", in.BodyType, 1, 50),
		ExteriorColor: validate.Text(v, "specification.exteriorColor", in.ExteriorColor, 1, 50),
		InteriorColor: validate.Text(v, "specification.interiorColor", in.InteriorColor, 1, 50),
		Powertrain:    validate.Text(v, "specification.powertrain", in.Powertrain, 1, 50),
		Drivetrain:    validate.Text(v, "specification.drivetrain", in.Drivetrain, 1, 50),
	}
	return
}

// ModelDetail is a model with its current and (optionally) all specifications.
type ModelDetail struct {
	Model    model.VehicleModel
	Current  model.Specification
	Versions []model.Specification
}

// Model is the vehicle model catalogue service.
type Model struct{ Deps }

// Create adds a make/model/variant with specification version 1.
func (s *Model) Create(ctx context.Context, p *auth.Principal, in SpecInput) (*ModelDetail, error) {
	var v apperr.Validation
	brand, m, variant, spec := in.validate(&v)
	if err := v.Err(); err != nil {
		return nil, err
	}
	now := s.clock()
	vm := &model.VehicleModel{
		ID: uuid.NewString(), Make: brand, Model: m, Variant: variant, CurrentSpecVersion: 1,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	spec.ModelID, spec.SpecVersion, spec.CreatedAt, spec.CreatedBy = vm.ID, 1, now, p.UserID
	if err := s.store.Models().Create(ctx, vm, &spec); err != nil {
		if errors.Is(err, apperr.ErrConflict) {
			return nil, apperr.New(apperr.ErrConflict, "model_duplicate", "this make, model and variant already exist")
		}
		return nil, err
	}
	return &ModelDetail{Model: *vm, Current: spec, Versions: []model.Specification{spec}}, nil
}

// AddVersion records a new immutable specification; existing vehicles keep
// the version they were registered with. Make, model and variant are fixed.
func (s *Model) AddVersion(ctx context.Context, p *auth.Principal, id string, expected int64, in SpecInput) (*ModelDetail, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	var v apperr.Validation
	brand, m, variant, spec := in.validate(&v)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *ModelDetail
	err := s.store.InTx(ctx, func(st Store) error {
		vm, err := st.Models().Get(ctx, id)
		if err != nil {
			return err
		}
		if vm.Version != expected {
			return apperr.ErrStale
		}
		if !strings.EqualFold(brand, vm.Make) || !strings.EqualFold(m, vm.Model) || !strings.EqualFold(variant, vm.Variant) {
			return apperr.FieldError("specification", "make, model and variant cannot change; create a new model instead")
		}
		now := s.clock()
		spec.ModelID, spec.SpecVersion, spec.CreatedAt, spec.CreatedBy = vm.ID, vm.CurrentSpecVersion+1, now, p.UserID
		vm.UpdatedAt = now
		if err := st.Models().AddSpec(ctx, vm, expected, &spec); err != nil {
			return err
		}
		versions, err := st.Models().Specs(ctx, vm.ID)
		if err != nil {
			return err
		}
		result = &ModelDetail{Model: *vm, Current: spec, Versions: versions}
		return nil
	})
	return result, err
}

func (s *Model) Get(ctx context.Context, id string) (*ModelDetail, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	m, err := s.store.Models().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	versions, err := s.store.Models().Specs(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &ModelDetail{Model: *m, Versions: versions}
	for _, spec := range versions {
		if spec.SpecVersion == m.CurrentSpecVersion {
			d.Current = spec
		}
	}
	return d, nil
}

func (s *Model) List(ctx context.Context, f model.ModelFilter) ([]ModelDetail, error) {
	f.Query = strings.TrimSpace(f.Query)
	models, err := s.store.Models().List(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]ModelDetail, 0, len(models))
	for _, m := range models {
		spec, err := s.store.Models().Spec(ctx, m.ID, m.CurrentSpecVersion)
		if err != nil {
			return nil, err
		}
		out = append(out, ModelDetail{Model: m, Current: *spec})
	}
	return out, nil
}
