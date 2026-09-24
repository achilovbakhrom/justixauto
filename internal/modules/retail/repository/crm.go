package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/modules/retail/model"
	"justixauto/internal/pkg/database"
)

type CRMRepository struct{ db *gorm.DB }

func (r *CRMRepository) CreateCustomer(ctx context.Context, c *model.Customer) error {
	return database.Translate(r.db.WithContext(ctx).Create(c).Error)
}

func (r *CRMRepository) Customer(ctx context.Context, companyID, id string) (*model.Customer, error) {
	var c model.Customer
	if err := r.db.WithContext(ctx).Where("id = ? AND company_id = ?", id, companyID).Take(&c).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &c, nil
}

func (r *CRMRepository) Customers(ctx context.Context, companyID, query string, limit, offset int) ([]model.Customer, error) {
	limit, offset = database.Page(limit, offset)
	q := r.db.WithContext(ctx).Where("company_id = ?", companyID).Order("display_name, id").Limit(limit).Offset(offset)
	if query != "" {
		q = q.Where("(display_name ILIKE ? OR phone ILIKE ?)", "%"+query+"%", "%"+query+"%")
	}
	cs := []model.Customer{}
	if err := database.Translate(q.Find(&cs).Error); err != nil {
		return nil, err
	}
	return cs, nil
}

func (r *CRMRepository) UpdateCustomer(ctx context.Context, c *model.Customer, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.Customer{}, c.ID, expected,
		map[string]any{"display_name": c.DisplayName, "phone": c.Phone, "updated_at": c.UpdatedAt})
	if err == nil {
		c.Version = expected + 1
	}
	return err
}

func (r *CRMRepository) CreateLead(ctx context.Context, l *model.Lead) error {
	return database.Translate(r.db.WithContext(ctx).Create(l).Error)
}

func (r *CRMRepository) Lead(ctx context.Context, companyID, id string) (*model.Lead, error) {
	var l model.Lead
	if err := r.db.WithContext(ctx).Where("id = ? AND company_id = ?", id, companyID).Take(&l).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &l, nil
}

func (r *CRMRepository) Leads(ctx context.Context, f model.LeadFilter) ([]model.Lead, error) {
	limit, offset := database.Page(f.Limit, f.Offset)
	q := r.db.WithContext(ctx).Where("company_id = ?", f.CompanyID).Order("updated_at DESC, id").Limit(limit).Offset(offset)
	if f.BranchIDs != nil {
		q = q.Where("branch_id IN ?", append(f.BranchIDs, uuid.Nil.String()))
	}
	if f.Stage != "" {
		q = q.Where("stage = ?", f.Stage)
	}
	ls := []model.Lead{}
	if err := database.Translate(q.Find(&ls).Error); err != nil {
		return nil, err
	}
	return ls, nil
}

func (r *CRMRepository) UpdateLead(ctx context.Context, l *model.Lead, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.Lead{}, l.ID, expected, map[string]any{
		"stage": l.Stage, "assigned_user_id": l.AssignedUserID, "lost_reason": l.LostReason, "deal_id": l.DealID, "updated_at": l.UpdatedAt,
	})
	if err == nil {
		l.Version = expected + 1
	}
	return err
}

func (r *CRMRepository) AddContact(ctx context.Context, c *model.Contact) error {
	return database.Translate(r.db.WithContext(ctx).Create(c).Error)
}

func (r *CRMRepository) Contacts(ctx context.Context, leadID string) ([]model.Contact, error) {
	cs := []model.Contact{}
	err := r.db.WithContext(ctx).Where("lead_id = ?", leadID).Order("occurred_at, id").Find(&cs).Error
	return cs, database.Translate(err)
}

func (r *CRMRepository) CreateTask(ctx context.Context, t *model.Task) error {
	return database.Translate(r.db.WithContext(ctx).Create(t).Error)
}

func (r *CRMRepository) Task(ctx context.Context, companyID, id string) (*model.Task, error) {
	var t model.Task
	if err := r.db.WithContext(ctx).Where("id = ? AND company_id = ?", id, companyID).Take(&t).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &t, nil
}

func (r *CRMRepository) Tasks(ctx context.Context, companyID, ownerID, status string, limit, offset int) ([]model.Task, error) {
	limit, offset = database.Page(limit, offset)
	q := r.db.WithContext(ctx).Where("company_id = ?", companyID).Order("due_at, id").Limit(limit).Offset(offset)
	if ownerID != "" {
		q = q.Where("owner_user_id = ?", ownerID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	ts := []model.Task{}
	if err := database.Translate(q.Find(&ts).Error); err != nil {
		return nil, err
	}
	return ts, nil
}

func (r *CRMRepository) UpdateTask(ctx context.Context, t *model.Task, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &model.Task{}, t.ID, expected,
		map[string]any{"status": t.Status, "completed_at": t.CompletedAt, "completed_by": t.CompletedBy})
	if err == nil {
		t.Version = expected + 1
	}
	return err
}
