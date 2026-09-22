package identity

import (
	"context"

	"gorm.io/gorm"
)

// CompanyFilter narrows List; zero values mean "any".
type CompanyFilter struct {
	Kind   CompanyKind
	Access CompanyAccess
	Limit  int
	Offset int
}

type CompanyRepository interface {
	Create(ctx context.Context, c *Company) error
	Get(ctx context.Context, id string) (*Company, error)
	GetMany(ctx context.Context, ids []string) ([]Company, error)
	List(ctx context.Context, f CompanyFilter) ([]Company, error)
	// Update saves c if the stored version equals expected and sets c.Version.
	Update(ctx context.Context, c *Company, expected int64) error
}

type companyRepository struct{ db *gorm.DB }

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

func (r *companyRepository) GetMany(ctx context.Context, ids []string) ([]Company, error) {
	companies := []Company{}
	if len(ids) == 0 {
		return companies, nil
	}
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Order("name, id").Find(&companies).Error
	return companies, translate(err)
}

func (r *companyRepository) List(ctx context.Context, f CompanyFilter) ([]Company, error) {
	limit, offset := pageDefaults(f.Limit, f.Offset)
	q := r.db.WithContext(ctx).Order("name, id").Limit(limit).Offset(offset)
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.Access != "" {
		q = q.Where("status = ?", f.Access)
	}
	companies := []Company{}
	return companies, translate(q.Find(&companies).Error)
}

func (r *companyRepository) Update(ctx context.Context, c *Company, expected int64) error {
	err := updateVersioned(r.db.WithContext(ctx), &Company{}, c.ID, expected, map[string]any{
		"name": c.Name, "legal_name": c.LegalName, "country": c.Country, "country_key": c.CountryKey,
		"region": c.Region, "region_key": c.RegionKey, "registration_number": c.RegistrationNumber,
		"email": c.Email, "address": c.Address, "phone": c.Phone,
		"status": c.Status, "status_reason": c.StatusReason, "updated_at": c.UpdatedAt,
	})
	if err == nil {
		c.Version = expected + 1
	}
	return err
}
