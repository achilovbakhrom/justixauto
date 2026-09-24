package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"justixauto/internal/modules/identity/model"
)

type MFARepository struct{ db *gorm.DB }

func (r *MFARepository) CreateEnrollment(ctx context.Context, e *model.MFAEnrollment) error {
	return translate(r.db.WithContext(ctx).Create(e).Error)
}

// Enrollment returns the user's unconfirmed enrollment, or ErrNotFound.
func (r *MFARepository) Enrollment(ctx context.Context, id, userID string) (*model.MFAEnrollment, error) {
	var e model.MFAEnrollment
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ? AND confirmed_at IS NULL", id, userID).Take(&e).Error
	if err != nil {
		return nil, translate(err)
	}
	return &e, nil
}

// Enable confirms the enrollment and stores the secret on the user.
func (r *MFARepository) Enable(ctx context.Context, e *model.MFAEnrollment, at time.Time, counter int64) error {
	db := r.db.WithContext(ctx)
	if err := db.Model(e).Update("confirmed_at", at).Error; err != nil {
		return translate(err)
	}
	return translate(db.Model(&model.User{}).Where("id = ?", e.UserID).Updates(map[string]any{
		"mfa_secret_enc": e.SecretEnc, "mfa_enabled_at": at, "mfa_last_counter": counter,
	}).Error)
}

// UseCounter records a TOTP step; false if it (or a later one) was used.
func (r *MFARepository) UseCounter(ctx context.Context, userID string, counter int64) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ? AND mfa_last_counter < ?", userID, counter).
		Update("mfa_last_counter", counter)
	return res.RowsAffected == 1, translate(res.Error)
}

func (r *MFARepository) ReplaceRecoveryCodes(ctx context.Context, userID string, hashes [][]byte) error {
	db := r.db.WithContext(ctx)
	if err := db.Where("user_id = ?", userID).Delete(&model.MFARecoveryCode{}).Error; err != nil {
		return translate(err)
	}
	rows := make([]model.MFARecoveryCode, len(hashes))
	for i, h := range hashes {
		rows[i] = model.MFARecoveryCode{UserID: userID, CodeHash: h}
	}
	return translate(db.Create(&rows).Error)
}

// UseRecoveryCode marks an unused code as used; false if none matched.
func (r *MFARepository) UseRecoveryCode(ctx context.Context, userID string, hash []byte, at time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.MFARecoveryCode{}).
		Where("user_id = ? AND code_hash = ? AND used_at IS NULL", userID, hash).Update("used_at", at)
	return res.RowsAffected == 1, translate(res.Error)
}

func (r *MFARepository) CreateChallenge(ctx context.Context, c *model.MFAChallenge) error {
	return translate(r.db.WithContext(ctx).Create(c).Error)
}

func (r *MFARepository) Challenge(ctx context.Context, id string) (*model.MFAChallenge, error) {
	var c model.MFAChallenge
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&c).Error; err != nil {
		return nil, translate(err)
	}
	return &c, nil
}

func (r *MFARepository) FailChallenge(ctx context.Context, id string) error {
	return translate(r.db.WithContext(ctx).Model(&model.MFAChallenge{}).Where("id = ?", id).
		Update("attempts", gorm.Expr("attempts + 1")).Error)
}

// ConsumeChallenge marks it used; false if it was already consumed.
func (r *MFARepository) ConsumeChallenge(ctx context.Context, id string, at time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.MFAChallenge{}).Where("id = ? AND consumed_at IS NULL", id).Update("consumed_at", at)
	return res.RowsAffected == 1, translate(res.Error)
}
