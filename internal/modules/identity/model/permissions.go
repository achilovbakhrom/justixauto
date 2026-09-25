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
}

const (
	RolePlatformAdmin = "platform_admin"
	RoleCompanyAdmin  = "company_admin"

	PlatformAdminRoleID = "00000000-0000-4000-8000-000000000001"
	CompanyAdminRoleID  = "00000000-0000-4000-8000-000000000002"
)

// systemRolePermissions: administration powers never include financial or
// insurance decisions.
var systemRolePermissions = map[string][]string{
	RolePlatformAdmin: {
		PermPlatformCompaniesCreate, PermPlatformCompaniesAccess, PermPlatformUsersManage,
		PermPlatformMembershipsManage, PermPlatformRolesManage, PermPlatformDirectoryRead,
		PermPlatformAuditRead,
	},
	RoleCompanyAdmin: {PermCompanyEdit, PermBranchesCreate, PermBranchesEdit},
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
