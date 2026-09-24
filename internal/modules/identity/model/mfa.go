package model

import "time"

// MFAEnrollment is a pending TOTP secret, not yet confirmed.
type MFAEnrollment struct {
	ID          string `gorm:"primaryKey;type:uuid"`
	UserID      string `gorm:"type:uuid"`
	SecretEnc   []byte
	CreatedAt   time.Time
	ExpiresAt   time.Time
	ConfirmedAt *time.Time
}

func (MFAEnrollment) TableName() string { return "identity.mfa_enrollments" }

// MFARecoveryCode is a single-use code replacing the authenticator.
type MFARecoveryCode struct {
	UserID   string `gorm:"primaryKey;type:uuid"`
	CodeHash []byte `gorm:"primaryKey"`
	UsedAt   *time.Time
}

func (MFARecoveryCode) TableName() string { return "identity.mfa_recovery_codes" }

// MFAChallenge: the password was correct, the second factor is pending.
type MFAChallenge struct {
	ID         string `gorm:"primaryKey;type:uuid"`
	UserID     string `gorm:"type:uuid"`
	TokenHash  []byte
	Attempts   int
	CreatedAt  time.Time
	ExpiresAt  time.Time
	ConsumedAt *time.Time
}

func (MFAChallenge) TableName() string { return "identity.mfa_challenges" }
