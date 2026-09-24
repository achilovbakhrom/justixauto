package model

import "time"

type UserStatus string

const (
	UserPending   UserStatus = "pending" // no credential yet, cannot sign in
	UserActive    UserStatus = "active"
	UserSuspended UserStatus = "suspended"
)

type User struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	DisplayName  string
	Email        string
	Login        *string
	PasswordHash *string
	// PasswordChangeRequired: an administrator set the password; the user must
	// choose their own before using anything else.
	PasswordChangeRequired bool
	Status                 UserStatus
	StatusReason           string
	FailedLogins           int
	LockedUntil            *time.Time
	// MFA fields change only through MFARepository, never through Update.
	MFASecretEnc   []byte     `gorm:"column:mfa_secret_enc"`
	MFAEnabledAt   *time.Time `gorm:"column:mfa_enabled_at"`
	MFALastCounter int64      `gorm:"column:mfa_last_counter"`
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (User) TableName() string { return "identity.users" }
