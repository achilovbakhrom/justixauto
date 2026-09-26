package model

import "time"

// Role scopes (user decisions 2026-09-26), derived from a role's permissions:
// platform roles are for JustixAuto staff; company roles are prepared in Admin
// and assigned by company admins to their employees.
const (
	RoleScopePlatform = "platform"
	RoleScopeCompany  = "company"
)

// Role is a named group of permissions prepared by the platform admin.
type Role struct {
	ID          string  `gorm:"primaryKey;type:uuid"`
	SystemKey   *string // set for built-in roles; their permissions come from code
	Name        string
	Scope       string   // RoleScopePlatform or RoleScopeCompany
	Permissions []string `gorm:"-"`
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Role) TableName() string { return "identity.roles" }

func (r Role) System() bool { return r.SystemKey != nil }

// AssignableByCompany reports whether company admins may assign r: only
// company roles, never platform roles.
func (r Role) AssignableByCompany() bool { return r.Scope == RoleScopeCompany }
