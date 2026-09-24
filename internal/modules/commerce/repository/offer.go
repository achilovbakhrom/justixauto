package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"justixauto/internal/modules/commerce/model"
	"justixauto/internal/pkg/database"
)

type OfferRepository struct{ db *gorm.DB }

func (r *OfferRepository) Create(ctx context.Context, o *model.Offer, v *model.OfferVersion) error {
	db := r.db.WithContext(ctx)
	if err := db.Create(o).Error; err != nil {
		return database.Translate(err)
	}
	return database.Translate(db.Create(v).Error)
}

// GetOwn returns the supplier's own offer, else ErrNotFound.
func (r *OfferRepository) GetOwn(ctx context.Context, supplierID, id string) (*model.Offer, error) {
	var o model.Offer
	if err := r.db.WithContext(ctx).Where("id = ? AND supplier_company_id = ?", id, supplierID).Take(&o).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &o, nil
}

// visible: published, supplier is an active partner of the buyer, and the
// published version's audience includes the buyer.
func (r *OfferRepository) visible(ctx context.Context, buyerID string) *gorm.DB {
	return r.db.WithContext(ctx).Model(&model.Offer{}).
		Joins("JOIN commerce.offer_versions v ON v.id = offers.published_version_id").
		Where("offers.status = ? AND offers.supplier_company_id <> ?", model.OfferPublished, buyerID).
		Where(`EXISTS (SELECT 1 FROM commerce.partnerships p WHERE p.status = 'active' AND
			((p.requester_company_id = offers.supplier_company_id AND p.recipient_company_id = ?) OR
			 (p.recipient_company_id = offers.supplier_company_id AND p.requester_company_id = ?)))`, buyerID, buyerID).
		Where("(v.audience_mode = 'all-active' OR v.audience_ids @> jsonb_build_array(?::text))", buyerID)
}

// GetVisible returns a published offer the buyer may see, else ErrNotFound.
func (r *OfferRepository) GetVisible(ctx context.Context, buyerID, id string) (*model.Offer, error) {
	var o model.Offer
	if err := r.visible(ctx, buyerID).Where("offers.id = ?", id).Select("offers.*").Take(&o).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &o, nil
}

// VisibleByPublishedVersion finds the visible offer whose published version is versionID.
func (r *OfferRepository) VisibleByPublishedVersion(ctx context.Context, buyerID, versionID string) (*model.Offer, error) {
	var o model.Offer
	err := r.visible(ctx, buyerID).Where("offers.published_version_id = ?", versionID).Select("offers.*").Take(&o).Error
	if err != nil {
		return nil, database.Translate(err)
	}
	return &o, nil
}

func (r *OfferRepository) ListOwn(ctx context.Context, supplierID string, limit, offset int) ([]model.Offer, error) {
	limit, offset = database.Page(limit, offset)
	os := []model.Offer{}
	err := r.db.WithContext(ctx).Where("supplier_company_id = ?", supplierID).Order("updated_at DESC, id").
		Limit(limit).Offset(offset).Find(&os).Error
	return os, database.Translate(err)
}

func (r *OfferRepository) ListVisible(ctx context.Context, buyerID string, limit, offset int) ([]model.Offer, error) {
	limit, offset = database.Page(limit, offset)
	os := []model.Offer{}
	err := r.visible(ctx, buyerID).Select("offers.*").Order("offers.updated_at DESC, offers.id").
		Limit(limit).Offset(offset).Find(&os).Error
	return os, database.Translate(err)
}

func (r *OfferRepository) Versions(ctx context.Context, offerID string) ([]model.OfferVersion, error) {
	vs := []model.OfferVersion{}
	err := r.db.WithContext(ctx).Where("offer_id = ?", offerID).Order("number").Find(&vs).Error
	return vs, database.Translate(err)
}

func (r *OfferRepository) Version(ctx context.Context, offerID, versionID string) (*model.OfferVersion, error) {
	var v model.OfferVersion
	if err := r.db.WithContext(ctx).Where("id = ? AND offer_id = ?", versionID, offerID).Take(&v).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &v, nil
}

func (r *OfferRepository) AddVersion(ctx context.Context, v *model.OfferVersion) error {
	db := r.db.WithContext(ctx)
	if err := db.Model(&model.OfferVersion{}).Where("offer_id = ?", v.OfferID).Select("coalesce(max(number), 0) + 1").Scan(&v.Number).Error; err != nil {
		return database.Translate(err)
	}
	return database.Translate(db.Create(v).Error)
}

func (r *OfferRepository) Update(ctx context.Context, o *model.Offer, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.Offer{}, o.ID, expected, map[string]any{
		"status": o.Status, "published_version_id": o.PublishedVersionID, "status_reason": o.StatusReason, "updated_at": o.UpdatedAt,
	})
	if err == nil {
		o.Version = expected + 1
	}
	return err
}

func (r *OfferRepository) MarkPublished(ctx context.Context, versionID string, at time.Time) error {
	return database.Translate(r.db.WithContext(ctx).Model(&model.OfferVersion{}).
		Where("id = ? AND published_at IS NULL", versionID).Update("published_at", at).Error)
}
