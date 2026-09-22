package identity

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"justixauto/internal/platform/apperr"
)

// CompanyFilter narrows List; zero values mean "any".
type CompanyFilter struct {
	Kind   CompanyKind
	Status CompanyStatus
	Limit  int
	Offset int
}

type CompanyRepository interface {
	Create(ctx context.Context, c *Company) error
	Get(ctx context.Context, id string) (*Company, error)
	List(ctx context.Context, f CompanyFilter) ([]Company, error)
	// Update saves c only if the stored version still equals expectedVersion,
	// then sets c.Version to the new version.
	Update(ctx context.Context, c *Company, expectedVersion int64) error
}

type companyRepository struct{ db *gorm.DB }

func NewCompanyRepository(db *gorm.DB) CompanyRepository {
	return &companyRepository{db: db}
}

func (r *companyRepository) Create(ctx context.Context, c *Company) error {
	return translate(r.db.WithContext(ctx).Create(c).Error)
}

func (r *companyRepository) Get(ctx context.Context, id string) (*Company, error) {
	var c Company
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&c).Error; err != nil {
		return nil, translate(err)
	}
	return &c, nil
}

func (r *companyRepository) List(ctx context.Context, f CompanyFilter) ([]Company, error) {
	q := r.db.WithContext(ctx).Order("name, id").Limit(f.Limit).Offset(f.Offset)
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	companies := []Company{}
	if err := q.Find(&companies).Error; err != nil {
		return nil, translate(err)
	}
	return companies, nil
}

func (r *companyRepository) Update(ctx context.Context, c *Company, expectedVersion int64) error {
	res := r.db.WithContext(ctx).Model(&Company{}).
		Where("id = ? AND version = ?", c.ID, expectedVersion).
		Updates(map[string]any{
			"name":                c.Name,
			"country":             c.Country,
			"region":              c.Region,
			"registration_number": c.RegistrationNumber,
			"status":              c.Status,
			"status_reason":       c.StatusReason,
			"version":             expectedVersion + 1,
			"updated_at":          c.UpdatedAt,
		})
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		// Either the row is gone or someone else updated it first.
		if _, err := r.Get(ctx, c.ID); err != nil {
			return err
		}
		return apperr.ErrStale
	}
	c.Version = expectedVersion + 1
	return nil
}

func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return apperr.ErrNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey):
		// The only unique constraint besides the primary key.
		return fmt.Errorf("%w: company with this country and registration number already exists", apperr.ErrConflict)
	}
	return err
}
