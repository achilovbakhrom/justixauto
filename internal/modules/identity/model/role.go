package model

import "time"

// Role is global: it applies equally in every company the user is a member of.
type Role struct {
	ID          string  `gorm:"primaryKey;type:uuid"`
	SystemKey   *string // set for built-in roles; their permissions come from code
	Name        string
	Permissions []string `gorm:"-"`
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Role) TableName() string { return "identity.roles" }

func (r Role) System() bool { return r.SystemKey != nil }
