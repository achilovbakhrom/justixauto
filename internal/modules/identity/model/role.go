package model

import "time"

// Role scopes (user decisions 2026-09-26): platform roles are for JustixAuto
// staff and hold platform permissions; company roles are prepared in Admin and
// assigned by company admins to their employees.
const (
	RoleScopePlatform = "platform"
	RoleScopeCompany  = "company"
)

// Role is a named group of permissions prepared by the platform admin.
type Role struct {
	ID          string  `gorm:"primaryKey;type:uuid"`
	SystemKey   *string // set for built-in roles; their permissions come from code
	Name        string
	Scope       string       // RoleScopePlatform or RoleScopeCompany
	CompanyKind *CompanyKind // company roles only; nil = any company type
	Permissions []string     `gorm:"-"`
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Role) TableName() string { return "identity.roles" }

func (r Role) System() bool { return r.SystemKey != nil }

// AssignableIn reports whether company admins of a company of kind may assign r.
func (r Role) AssignableIn(kind CompanyKind) bool {
	return r.Scope == RoleScopeCompany && (r.CompanyKind == nil || *r.CompanyKind == kind)
}
