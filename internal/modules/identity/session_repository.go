package identity

import (
	"context"
	"time"

	"gorm.io/gorm"

	"justixauto/internal/platform/apperr"
)

type SessionRepository interface {
	Create(ctx context.Context, s *Session) error
	// FindLive returns the unrevoked session with this token hash, or ErrNotFound.
	FindLive(ctx context.Context, tokenHash []byte) (*Session, error)
	Touch(ctx context.Context, id string, at time.Time) error
	// SetMFA records a successful second factor for the session.
	SetMFA(ctx context.Context, id string, at time.Time) error
	Revoke(ctx context.Context, id string, at time.Time) error
	// RevokeUser revokes all live sessions of a user except exceptID ("" = all).
	RevokeUser(ctx context.Context, userID, exceptID string, at time.Time) error
	// UpdateContext saves company/scope if the context revision still equals
	// expected, then increments it.
	UpdateContext(ctx context.Context, s *Session, expected int64) error
	// ResetCompanyContext clears the active company on the user's live sessions
	// that work in companyID (after membership revocation).
	ResetCompanyContext(ctx context.Context, userID, companyID string) error
}

type sessionRepository struct{ db *gorm.DB }

func (r *sessionRepository) Create(ctx context.Context, s *Session) error {
	return translate(r.db.WithContext(ctx).Create(s).Error)
}

func (r *sessionRepository) FindLive(ctx context.Context, tokenHash []byte) (*Session, error) {
	var s Session
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

func (r *sessionRepository) Touch(ctx context.Context, id string, at time.Time) error {
	return translate(r.db.WithContext(ctx).Model(&Session{}).Where("id = ?", id).Update("last_seen_at", at).Error)
}

func (r *sessionRepository) SetMFA(ctx context.Context, id string, at time.Time) error {
	return translate(r.db.WithContext(ctx).Model(&Session{}).Where("id = ?", id).Update("mfa_authenticated_at", at).Error)
}

func (r *sessionRepository) Revoke(ctx context.Context, id string, at time.Time) error {
	return translate(r.db.WithContext(ctx).Model(&Session{}).
		Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", at).Error)
}

func (r *sessionRepository) RevokeUser(ctx context.Context, userID, exceptID string, at time.Time) error {
	q := r.db.WithContext(ctx).Model(&Session{}).Where("user_id = ? AND revoked_at IS NULL", userID)
	if exceptID != "" {
		q = q.Where("id <> ?", exceptID)
	}
	return translate(q.Update("revoked_at", at).Error)
}

func (r *sessionRepository) UpdateContext(ctx context.Context, s *Session, expected int64) error {
	db := r.db.WithContext(ctx)
	res := db.Model(&Session{}).Where("id = ? AND context_revision = ?", s.ID, expected).Updates(map[string]any{
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

func (r *sessionRepository) ResetCompanyContext(ctx context.Context, userID, companyID string) error {
	db := r.db.WithContext(ctx)
	var ids []string
	err := db.Model(&Session{}).Where("user_id = ? AND active_company_id = ? AND revoked_at IS NULL", userID, companyID).
		Pluck("id", &ids).Error
	if err != nil || len(ids) == 0 {
		return translate(err)
	}
	if err := db.Where("session_id IN ?", ids).Delete(&sessionBranch{}).Error; err != nil {
		return translate(err)
	}
	return translate(db.Model(&Session{}).Where("id IN ?", ids).Updates(map[string]any{
		"active_company_id": nil, "branch_scope_mode": ScopeAll,
		"context_revision": gorm.Expr("context_revision + 1"),
	}).Error)
}
