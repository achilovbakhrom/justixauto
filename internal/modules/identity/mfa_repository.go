package identity

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type MFARepository interface {
	CreateEnrollment(ctx context.Context, e *mfaEnrollment) error
	// Enrollment returns the user's unconfirmed enrollment, or ErrNotFound.
	Enrollment(ctx context.Context, id, userID string) (*mfaEnrollment, error)
	// Enable confirms the enrollment and stores the secret on the user.
	Enable(ctx context.Context, e *mfaEnrollment, at time.Time, counter int64) error
	// UseCounter records a TOTP step; false if it (or a later one) was used.
	UseCounter(ctx context.Context, userID string, counter int64) (bool, error)
	ReplaceRecoveryCodes(ctx context.Context, userID string, hashes [][]byte) error
	// UseRecoveryCode marks an unused code as used; false if none matched.
	UseRecoveryCode(ctx context.Context, userID string, hash []byte, at time.Time) (bool, error)
	CreateChallenge(ctx context.Context, c *mfaChallenge) error
	Challenge(ctx context.Context, id string) (*mfaChallenge, error)
	FailChallenge(ctx context.Context, id string) error
	// ConsumeChallenge marks it used; false if it was already consumed.
	ConsumeChallenge(ctx context.Context, id string, at time.Time) (bool, error)
}

type mfaRepository struct{ db *gorm.DB }

func (r *mfaRepository) CreateEnrollment(ctx context.Context, e *mfaEnrollment) error {
	return translate(r.db.WithContext(ctx).Create(e).Error)
}

func (r *mfaRepository) Enrollment(ctx context.Context, id, userID string) (*mfaEnrollment, error) {
	var e mfaEnrollment
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ? AND confirmed_at IS NULL", id, userID).Take(&e).Error
	if err != nil {
		return nil, translate(err)
	}
	return &e, nil
}

func (r *mfaRepository) Enable(ctx context.Context, e *mfaEnrollment, at time.Time, counter int64) error {
	db := r.db.WithContext(ctx)
	if err := db.Model(e).Update("confirmed_at", at).Error; err != nil {
		return translate(err)
	}
	return translate(db.Model(&User{}).Where("id = ?", e.UserID).Updates(map[string]any{
		"mfa_secret_enc": e.SecretEnc, "mfa_enabled_at": at, "mfa_last_counter": counter,
	}).Error)
}

func (r *mfaRepository) UseCounter(ctx context.Context, userID string, counter int64) (bool, error) {
	res := r.db.WithContext(ctx).Model(&User{}).Where("id = ? AND mfa_last_counter < ?", userID, counter).
		Update("mfa_last_counter", counter)
	return res.RowsAffected == 1, translate(res.Error)
}

func (r *mfaRepository) ReplaceRecoveryCodes(ctx context.Context, userID string, hashes [][]byte) error {
	db := r.db.WithContext(ctx)
	if err := db.Where("user_id = ?", userID).Delete(&mfaRecoveryCode{}).Error; err != nil {
		return translate(err)
	}
	rows := make([]mfaRecoveryCode, len(hashes))
	for i, h := range hashes {
		rows[i] = mfaRecoveryCode{UserID: userID, CodeHash: h}
	}
	return translate(db.Create(&rows).Error)
}

func (r *mfaRepository) UseRecoveryCode(ctx context.Context, userID string, hash []byte, at time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Model(&mfaRecoveryCode{}).
		Where("user_id = ? AND code_hash = ? AND used_at IS NULL", userID, hash).Update("used_at", at)
	return res.RowsAffected == 1, translate(res.Error)
}

func (r *mfaRepository) CreateChallenge(ctx context.Context, c *mfaChallenge) error {
	return translate(r.db.WithContext(ctx).Create(c).Error)
}

func (r *mfaRepository) Challenge(ctx context.Context, id string) (*mfaChallenge, error) {
	var c mfaChallenge
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&c).Error; err != nil {
		return nil, translate(err)
	}
	return &c, nil
}

func (r *mfaRepository) FailChallenge(ctx context.Context, id string) error {
	return translate(r.db.WithContext(ctx).Model(&mfaChallenge{}).Where("id = ?", id).
		Update("attempts", gorm.Expr("attempts + 1")).Error)
}

func (r *mfaRepository) ConsumeChallenge(ctx context.Context, id string, at time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Model(&mfaChallenge{}).Where("id = ? AND consumed_at IS NULL", id).Update("consumed_at", at)
	return res.RowsAffected == 1, translate(res.Error)
}
