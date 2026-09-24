package repository

import (
	"context"

	"gorm.io/gorm"

	"justixauto/internal/modules/commerce/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/database"
)

type DealRepository struct{ db *gorm.DB }

func (r *DealRepository) CreateRFQ(ctx context.Context, x *model.RFQ) error {
	return database.Translate(r.db.WithContext(ctx).Create(x).Error)
}

// RFQ returns an RFQ visible to the company: the buyer always, the
// supplier once it was sent.
func (r *DealRepository) RFQ(ctx context.Context, companyID, id string) (*model.RFQ, error) {
	var x model.RFQ
	err := r.db.WithContext(ctx).Where("id = ? AND (buyer_company_id = ? OR (supplier_company_id = ? AND status <> ?))",
		id, companyID, companyID, model.RFQDraft).Take(&x).Error
	if err != nil {
		return nil, database.Translate(err)
	}
	return &x, nil
}

func (r *DealRepository) RFQs(ctx context.Context, companyID string, limit, offset int) ([]model.RFQ, error) {
	limit, offset = database.Page(limit, offset)
	xs := []model.RFQ{}
	err := r.db.WithContext(ctx).Where("buyer_company_id = ? OR (supplier_company_id = ? AND status <> ?)", companyID, companyID, model.RFQDraft).
		Order("updated_at DESC, id").Limit(limit).Offset(offset).Find(&xs).Error
	return xs, database.Translate(err)
}

func (r *DealRepository) UpdateRFQ(ctx context.Context, x *model.RFQ, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.RFQ{}, x.ID, expected, map[string]any{
		"status": x.Status, "status_reason": x.StatusReason, "lines": x.Lines, "updated_at": x.UpdatedAt,
	})
	if err == nil {
		x.Version = expected + 1
	}
	return err
}

func (r *DealRepository) AddQuotation(ctx context.Context, q *model.Quotation) error {
	db := r.db.WithContext(ctx)
	if err := db.Model(&model.Quotation{}).Where("rfq_id = ?", q.RFQID).Select("coalesce(max(number), 0) + 1").Scan(&q.Number).Error; err != nil {
		return database.Translate(err)
	}
	return database.Translate(db.Create(q).Error)
}

func (r *DealRepository) Quotations(ctx context.Context, rfqID string) ([]model.Quotation, error) {
	qs := []model.Quotation{}
	err := r.db.WithContext(ctx).Where("rfq_id = ?", rfqID).Order("number").Find(&qs).Error
	return qs, database.Translate(err)
}

func (r *DealRepository) CreateOrder(ctx context.Context, o *model.Order) error {
	return database.Translate(r.db.WithContext(ctx).Create(o).Error)
}

// Order returns an order the company is a party of.
func (r *DealRepository) Order(ctx context.Context, companyID, id string) (*model.Order, error) {
	var o model.Order
	err := r.db.WithContext(ctx).Where("id = ? AND (buyer_company_id = ? OR supplier_company_id = ?)", id, companyID, companyID).Take(&o).Error
	if err != nil {
		return nil, database.Translate(err)
	}
	return &o, nil
}

func (r *DealRepository) Orders(ctx context.Context, companyID string, limit, offset int) ([]model.Order, error) {
	limit, offset = database.Page(limit, offset)
	os := []model.Order{}
	err := r.db.WithContext(ctx).Where("buyer_company_id = ? OR supplier_company_id = ?", companyID, companyID).
		Order("updated_at DESC, id").Limit(limit).Offset(offset).Find(&os).Error
	return os, database.Translate(err)
}

func (r *DealRepository) UpdateOrder(ctx context.Context, o *model.Order, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.Order{}, o.ID, expected, map[string]any{
		"status": o.Status, "status_reason": o.StatusReason, "terms": o.Terms, "updated_at": o.UpdatedAt,
	})
	if err == nil {
		o.Version = expected + 1
	}
	return err
}

func (r *DealRepository) AddAddendum(ctx context.Context, a *model.Addendum) error {
	db := r.db.WithContext(ctx)
	if err := db.Model(&model.Addendum{}).Where("order_id = ?", a.OrderID).Select("coalesce(max(number), 0) + 1").Scan(&a.Number).Error; err != nil {
		return database.Translate(err)
	}
	return database.Translate(db.Create(a).Error)
}

func (r *DealRepository) Addenda(ctx context.Context, orderID string) ([]model.Addendum, error) {
	as := []model.Addendum{}
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).Order("number").Find(&as).Error
	return as, database.Translate(err)
}

func (r *DealRepository) DecideAddendum(ctx context.Context, a *model.Addendum) error {
	res := r.db.WithContext(ctx).Model(&model.Addendum{}).Where("id = ? AND status = 'proposed'", a.ID).
		Updates(map[string]any{"status": a.Status, "decision_reason": a.DecisionReason, "decided_at": a.DecidedAt})
	if res.Error == nil && res.RowsAffected == 0 {
		return apperr.New(apperr.ErrConflict, "addendum_decided", "the addendum was already decided")
	}
	return database.Translate(res.Error)
}
