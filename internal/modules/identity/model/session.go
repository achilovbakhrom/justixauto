package model

import "time"

const (
	ScopeAll      = "ALL"
	ScopeSelected = "SELECTED"
)

// Session is a signed-in browser. Only the SHA-256 of the cookie token is stored.
type Session struct {
	ID              string `gorm:"primaryKey;type:uuid"`
	TokenHash       []byte
	CSRFToken       string  `gorm:"column:csrf_token"`
	UserID          string  `gorm:"type:uuid"`
	ActiveCompanyID *string `gorm:"type:uuid"`
	BranchScopeMode string
	BranchIDs       []string `gorm:"-"` // only for SELECTED scope
	ContextRevision int64
	CreatedAt       time.Time
	LastSeenAt      time.Time
	ExpiresAt       time.Time
	RevokedAt       *time.Time
}

func (Session) TableName() string { return "identity.sessions" }
