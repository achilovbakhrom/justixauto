package repository

import (
	"context"

	"gorm.io/gorm"

	"justixauto/internal/modules/identity/model"
)

type CompanyRepository struct{ db *gorm.DB }

// live excludes soft-deleted companies: for callers they do not exist.
const live = "deleted_at IS NULL"

// liveOnly is a reusable session limited to live companies (a new session so
// the update and its follow-up count do not share accumulated conditions).
func (r *CompanyRepository) liveOnly(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Where(live).Session(&gorm.Session{})
}

func (r *CompanyRepository) Create(ctx context.Context, c *model.Company) error {
	return translate(r.db.WithContext(ctx).Create(c).Error)
}

// Get returns a live company; a soft-deleted one is ErrNotFound.
func (r *CompanyRepository) Get(ctx context.Context, id string) (*model.Company, error) {
	var c model.Company
	if err := r.db.WithContext(ctx).Where("id = ? AND "+live, id).Take(&c).Error; err != nil {
		return nil, translate(err)
	}
	return &c, nil
}

// GetIncludingDeleted also returns a soft-deleted company, so old records
// that reference it can still show its name.
func (r *CompanyRepository) GetIncludingDeleted(ctx context.Context, id string) (*model.Company, error) {
	var c model.Company
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&c).Error; err != nil {
		return nil, translate(err)
	}
	return &c, nil
}

func (r *CompanyRepository) GetMany(ctx context.Context, ids []string) ([]model.Company, error) {
	companies := []model.Company{}
	if len(ids) == 0 {
		return companies, nil
	}
	err := r.db.WithContext(ctx).Where("id IN ? AND "+live, ids).Order("name, id").Find(&companies).Error
	return companies, translate(err)
}

func (r *CompanyRepository) List(ctx context.Context, f model.CompanyFilter) ([]model.Company, error) {
	limit, offset := pageDefaults(f.Limit, f.Offset)
	q := r.db.WithContext(ctx).Where(live).Order("name, id").Limit(limit).Offset(offset)
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.Access != "" {
		q = q.Where("status = ?", f.Access)
	}
	if f.Query != "" {
		q = q.Where("name ILIKE ?", "%"+f.Query+"%")
	}
	companies := []model.Company{}
	if err := translate(q.Find(&companies).Error); err != nil {
		return nil, err
	}
	return companies, nil
}

// Update saves c if the stored version equals expected and sets c.Version.
func (r *CompanyRepository) Update(ctx context.Context, c *model.Company, expected int64) error {
	err := updateVersioned(r.liveOnly(ctx), &model.Company{}, c.ID, expected, map[string]any{
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

// SoftDelete marks c deleted if the stored version equals expected and it is
// not deleted yet; it sets c.Version.
func (r *CompanyRepository) SoftDelete(ctx context.Context, c *model.Company, expected int64) error {
	err := updateVersioned(r.liveOnly(ctx), &model.Company{}, c.ID, expected, map[string]any{
		"deleted_at": c.DeletedAt, "status_reason": c.StatusReason, "updated_at": c.UpdatedAt,
	})
	if err == nil {
		c.Version = expected + 1
	}
	return err
}
