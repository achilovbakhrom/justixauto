package model

import "time"

type MembershipStatus string

const (
	MembershipActive  MembershipStatus = "active"
	MembershipRevoked MembershipStatus = "revoked"
)

const (
	AllBranches      = "ALL_BRANCHES"
	SelectedBranches = "SELECTED_BRANCHES"
)

// Membership links a user to a company. It carries branch access, never a role.
type Membership struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	UserID       string `gorm:"type:uuid"`
	CompanyID    string `gorm:"type:uuid"`
	Status       MembershipStatus
	BranchAccess string
	BranchIDs    []string `gorm:"-"` // only for SELECTED_BRANCHES
	StatusReason string
	Version      int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (Membership) TableName() string { return "identity.memberships" }
