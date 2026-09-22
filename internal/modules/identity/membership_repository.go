package identity

import (
	"context"

	"gorm.io/gorm"
)

type BranchRepository interface {
	Create(ctx context.Context, b *Branch) error
	Get(ctx context.Context, companyID, id string) (*Branch, error)
	List(ctx context.Context, companyID string) ([]Branch, error)
	Update(ctx context.Context, b *Branch, expected int64) error
	// CountInCompany counts how many of ids are branches of the company.
	CountInCompany(ctx context.Context, companyID string, ids []string) (int64, error)
}

type branchRepository struct{ db *gorm.DB }

func (r *branchRepository) Create(ctx context.Context, b *Branch) error {
	return translate(r.db.WithContext(ctx).Create(b).Error)
}

func (r *branchRepository) Get(ctx context.Context, companyID, id string) (*Branch, error) {
	var b Branch
	if err := r.db.WithContext(ctx).Where("id = ? AND company_id = ?", id, companyID).Take(&b).Error; err != nil {
		return nil, translate(err)
	}
	return &b, nil
}

func (r *branchRepository) List(ctx context.Context, companyID string) ([]Branch, error) {
	branches := []Branch{}
	err := r.db.WithContext(ctx).Where("company_id = ?", companyID).Order("name, id").Find(&branches).Error
	return branches, translate(err)
}

func (r *branchRepository) Update(ctx context.Context, b *Branch, expected int64) error {
	err := updateVersioned(r.db.WithContext(ctx), &Branch{}, b.ID, expected, map[string]any{
		"name": b.Name, "address": b.Address, "updated_at": b.UpdatedAt,
	})
	if err == nil {
		b.Version = expected + 1
	}
	return err
}

func (r *branchRepository) CountInCompany(ctx context.Context, companyID string, ids []string) (int64, error) {
	var n int64
	if len(ids) == 0 {
		return 0, nil
	}
	err := r.db.WithContext(ctx).Model(&Branch{}).Where("company_id = ? AND id IN ?", companyID, ids).Count(&n).Error
	return n, translate(err)
}

type MembershipRepository interface {
	Create(ctx context.Context, m *Membership) error
	Get(ctx context.Context, id string) (*Membership, error)
	ListByUser(ctx context.Context, userID string) ([]Membership, error)
	// Active returns the user's active membership in the company, or ErrNotFound.
	Active(ctx context.Context, userID, companyID string) (*Membership, error)
	Update(ctx context.Context, m *Membership, expected int64) error
}

type membershipRepository struct{ db *gorm.DB }

func (r *membershipRepository) loadBranches(ctx context.Context, ms []Membership) ([]Membership, error) {
	if len(ms) == 0 {
		return ms, nil
	}
	ids := make([]string, len(ms))
	for i, m := range ms {
		ids[i] = m.ID
	}
	var rows []membershipBranch
	if err := r.db.WithContext(ctx).Where("membership_id IN ?", ids).Order("branch_id").Find(&rows).Error; err != nil {
		return nil, translate(err)
	}
	byID := map[string][]string{}
	for _, row := range rows {
		byID[row.MembershipID] = append(byID[row.MembershipID], row.BranchID)
	}
	for i := range ms {
		ms[i].BranchIDs = byID[ms[i].ID]
		if ms[i].BranchIDs == nil {
			ms[i].BranchIDs = []string{}
		}
	}
	return ms, nil
}

func (r *membershipRepository) saveBranches(ctx context.Context, m *Membership) error {
	db := r.db.WithContext(ctx)
	if err := db.Where("membership_id = ?", m.ID).Delete(&membershipBranch{}).Error; err != nil {
		return translate(err)
	}
	if len(m.BranchIDs) == 0 {
		return nil
	}
	rows := make([]membershipBranch, len(m.BranchIDs))
	for i, b := range m.BranchIDs {
		rows[i] = membershipBranch{MembershipID: m.ID, BranchID: b}
	}
	return translate(db.Create(&rows).Error)
}

func (r *membershipRepository) Create(ctx context.Context, m *Membership) error {
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return translate(err)
	}
	return r.saveBranches(ctx, m)
}

func (r *membershipRepository) find(ctx context.Context, query string, args ...any) ([]Membership, error) {
	ms := []Membership{}
	if err := r.db.WithContext(ctx).Where(query, args...).Order("created_at, id").Find(&ms).Error; err != nil {
		return nil, translate(err)
	}
	return r.loadBranches(ctx, ms)
}

func (r *membershipRepository) one(ctx context.Context, query string, args ...any) (*Membership, error) {
	ms, err := r.find(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if len(ms) == 0 {
		return nil, translate(gorm.ErrRecordNotFound)
	}
	return &ms[0], nil
}

func (r *membershipRepository) Get(ctx context.Context, id string) (*Membership, error) {
	return r.one(ctx, "id = ?", id)
}

func (r *membershipRepository) ListByUser(ctx context.Context, userID string) ([]Membership, error) {
	return r.find(ctx, "user_id = ?", userID)
}

func (r *membershipRepository) Active(ctx context.Context, userID, companyID string) (*Membership, error) {
	return r.one(ctx, "user_id = ? AND company_id = ? AND status = ?", userID, companyID, MembershipActive)
}

func (r *membershipRepository) Update(ctx context.Context, m *Membership, expected int64) error {
	err := updateVersioned(r.db.WithContext(ctx), &Membership{}, m.ID, expected, map[string]any{
		"status": m.Status, "branch_access": m.BranchAccess, "status_reason": m.StatusReason, "updated_at": m.UpdatedAt,
	})
	if err != nil {
		return err
	}
	m.Version = expected + 1
	return r.saveBranches(ctx, m)
}
