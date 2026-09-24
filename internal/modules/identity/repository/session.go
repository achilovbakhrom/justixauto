package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"justixauto/internal/modules/identity/model"
	"justixauto/internal/pkg/apperr"
)

type sessionBranch struct {
	SessionID string `gorm:"primaryKey;type:uuid"`
	BranchID  string `gorm:"primaryKey;type:uuid"`
}

func (sessionBranch) TableName() string { return "identity.session_branches" }

type SessionRepository struct{ db *gorm.DB }

func (r *SessionRepository) Create(ctx context.Context, s *model.Session) error {
	return translate(r.db.WithContext(ctx).Create(s).Error)
}

// FindLive returns the unrevoked session with this token hash, or ErrNotFound.
func (r *SessionRepository) FindLive(ctx context.Context, tokenHash []byte) (*model.Session, error) {
	var s model.Session
	db := r.db.WithContext(ctx)
	if err := db.Where("token_hash = ? AND revoked_at IS NULL", tokenHash).Take(&s).Error; err != nil {
		return nil, translate(err)
	}
	var rows []sessionBranch
	if err := db.Where("session_id = ?", s.ID).Order("branch_id").Find(&rows).Error; err != nil {
		return nil, translate(err)
	}
	s.BranchIDs = make([]string, len(rows))
	for i, row := range rows {
		s.BranchIDs[i] = row.BranchID
	}
	return &s, nil
}

func (r *SessionRepository) Touch(ctx context.Context, id string, at time.Time) error {
	return translate(r.db.WithContext(ctx).Model(&model.Session{}).Where("id = ?", id).Update("last_seen_at", at).Error)
}

// SetMFA records a successful second factor for the session.
func (r *SessionRepository) SetMFA(ctx context.Context, id string, at time.Time) error {
	return translate(r.db.WithContext(ctx).Model(&model.Session{}).Where("id = ?", id).Update("mfa_authenticated_at", at).Error)
}

func (r *SessionRepository) Revoke(ctx context.Context, id string, at time.Time) error {
	return translate(r.db.WithContext(ctx).Model(&model.Session{}).
		Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", at).Error)
}

// RevokeUser revokes all live sessions of a user except exceptID ("" = all).
func (r *SessionRepository) RevokeUser(ctx context.Context, userID, exceptID string, at time.Time) error {
	q := r.db.WithContext(ctx).Model(&model.Session{}).Where("user_id = ? AND revoked_at IS NULL", userID)
	if exceptID != "" {
		q = q.Where("id <> ?", exceptID)
	}
	return translate(q.Update("revoked_at", at).Error)
}

// UpdateContext saves company/scope if the context revision still equals
// expected, then increments it.
func (r *SessionRepository) UpdateContext(ctx context.Context, s *model.Session, expected int64) error {
	db := r.db.WithContext(ctx)
	res := db.Model(&model.Session{}).Where("id = ? AND context_revision = ?", s.ID, expected).Updates(map[string]any{
		"active_company_id": s.ActiveCompanyID, "branch_scope_mode": s.BranchScopeMode,
		"context_revision": expected + 1,
	})
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.ErrStale
	}
	s.ContextRevision = expected + 1
	if err := db.Where("session_id = ?", s.ID).Delete(&sessionBranch{}).Error; err != nil {
		return translate(err)
	}
	if len(s.BranchIDs) == 0 {
		return nil
	}
	rows := make([]sessionBranch, len(s.BranchIDs))
	for i, b := range s.BranchIDs {
		rows[i] = sessionBranch{SessionID: s.ID, BranchID: b}
	}
	return translate(db.Create(&rows).Error)
}

// ResetCompanyContext clears the active company on the user's live sessions
// that work in companyID (after membership revocation).
func (r *SessionRepository) ResetCompanyContext(ctx context.Context, userID, companyID string) error {
	db := r.db.WithContext(ctx)
	var ids []string
	err := db.Model(&model.Session{}).Where("user_id = ? AND active_company_id = ? AND revoked_at IS NULL", userID, companyID).
		Pluck("id", &ids).Error
	if err != nil || len(ids) == 0 {
		return translate(err)
	}
	if err := db.Where("session_id IN ?", ids).Delete(&sessionBranch{}).Error; err != nil {
		return translate(err)
	}
	return translate(db.Model(&model.Session{}).Where("id IN ?", ids).Updates(map[string]any{
		"active_company_id": nil, "branch_scope_mode": model.ScopeAll,
		"context_revision": gorm.Expr("context_revision + 1"),
	}).Error)
}
