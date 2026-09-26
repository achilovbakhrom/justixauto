package model

import (
	"slices"

	"justixauto/internal/pkg/auth"
)

// Permission keys from the HTTP contract. Other modules add their own keys to
// the catalog as they are built (`<module>.<resource>.<action>`).
const (
	PermPlatformCompaniesCreate   = "platform.companies.create"
	PermPlatformCompaniesAccess   = "platform.companies.access"
	PermPlatformUsersManage       = "platform.users.manage"
	PermPlatformMembershipsManage = "platform.memberships.manage"
	PermPlatformRolesManage       = "platform.roles.manage"
	PermPlatformDirectoryRead     = "platform.directory.read"
	PermPlatformAuditRead         = "platform.audit.read"
	PermCompanyCreate             = "company.create"
	PermCompanyEdit               = "company.edit"
	PermBranchesCreate            = "branches.create"
	PermBranchesEdit              = "branches.edit"
	// PermCompanyUsersManage lets a company admin manage the company's own
	// employees with roles prepared in Admin (user decision 2026-09-26).
	PermCompanyUsersManage = "company.users.manage"
)

// PermissionInfo describes one catalog entry (see auth.PermissionInfo).
type PermissionInfo = auth.PermissionInfo

var Catalog = []PermissionInfo{
	{Key: PermPlatformCompaniesCreate, Scope: "platform", Assignable: false},
	{Key: PermPlatformCompaniesAccess, Scope: "platform", Assignable: false},
	{Key: PermPlatformUsersManage, Scope: "platform", Assignable: false},
	{Key: PermPlatformMembershipsManage, Scope: "platform", Assignable: false},
	{Key: PermPlatformRolesManage, Scope: "platform", Assignable: false},
	{Key: PermPlatformDirectoryRead, Scope: "platform", Assignable: true},
	{Key: PermPlatformAuditRead, Scope: "platform", Assignable: true},
	{Key: PermCompanyCreate, Scope: "company", Assignable: true},
	{Key: PermCompanyEdit, Scope: "company", Assignable: true},
	{Key: PermBranchesCreate, Scope: "company", Assignable: true},
	{Key: PermBranchesEdit, Scope: "company", Assignable: true},
	{Key: PermCompanyUsersManage, Scope: "company", Assignable: true},
}

const (
	RolePlatformAdmin = "platform_admin"
	RoleCompanyAdmin  = "company_admin"

	PlatformAdminRoleID = "00000000-0000-4000-8000-000000000001"
	CompanyAdminRoleID  = "00000000-0000-4000-8000-000000000002"
)

// systemRolePermissions: fixed grants of built-in roles. Platform
// administration never includes company business permissions. The company
// administrator is not listed: it is computed, see companyScopePermissions.
var systemRolePermissions = map[string][]string{
	RolePlatformAdmin: {
		PermPlatformCompaniesCreate, PermPlatformCompaniesAccess, PermPlatformUsersManage,
		PermPlatformMembershipsManage, PermPlatformRolesManage, PermPlatformDirectoryRead,
		PermPlatformAuditRead,
	},
}

// companyScopePermissions lists every company-scoped permission in the
// catalog, including those other modules register at startup. User decision
// 2026-09-26: a company administrator holds all of them by default; platform
// permissions are never included. Organization capabilities still apply, so
// e.g. a bank cannot sell retail just because its admin holds retail keys.
func companyScopePermissions() []string {
	var out []string
	for _, p := range Catalog {
		if p.Scope == "company" {
			out = append(out, p.Key)
		}
	}
	return out
}

// LookupPermission returns the catalog entry for key, if any.
func LookupPermission(key string) (PermissionInfo, bool) {
	i := slices.IndexFunc(Catalog, func(p PermissionInfo) bool { return p.Key == key })
	if i < 0 {
		return PermissionInfo{}, false
	}
	return Catalog[i], true
}

// EffectivePermissions returns the permissions granted by a role.
func EffectivePermissions(r Role) []string {
	if r.SystemKey != nil {
		if *r.SystemKey == RoleCompanyAdmin {
			return companyScopePermissions()
		}
		return systemRolePermissions[*r.SystemKey]
	}
	return r.Permissions
}

// RegisterPermissions adds another module's permission keys to the catalog.
// Call it at startup, before serving requests; duplicates are ignored.
func RegisterPermissions(perms ...PermissionInfo) {
	for _, p := range perms {
		if _, exists := LookupPermission(p.Key); !exists {
			Catalog = append(Catalog, p)
		}
	}
}
