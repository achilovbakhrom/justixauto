package identity

import "slices"

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

// PermissionInfo describes one catalog entry. Assignable=false keys can only be
// held through a system role, which keeps the last-admin guard simple.
// RequiresMFA marks sensitive actions; enforcement arrives with MFA.
type PermissionInfo struct {
	Key         string `json:"key"`
	Scope       string `json:"scope"` // "platform" or "company"
	RequiresMFA bool   `json:"requiresMfa"`
	Assignable  bool   `json:"assignable"`
}

var Catalog = []PermissionInfo{
	{PermPlatformCompaniesCreate, "platform", true, false},
	{PermPlatformCompaniesAccess, "platform", true, false},
	{PermPlatformUsersManage, "platform", true, false},
	{PermPlatformMembershipsManage, "platform", true, false},
	{PermPlatformRolesManage, "platform", true, false},
	{PermPlatformDirectoryRead, "platform", false, true},
	{PermPlatformAuditRead, "platform", false, true},
	{PermCompanyCreate, "company", false, true},
	{PermCompanyEdit, "company", false, true},
	{PermBranchesCreate, "company", false, true},
	{PermBranchesEdit, "company", false, true},
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

func permissionInfo(key string) (PermissionInfo, bool) {
	i := slices.IndexFunc(Catalog, func(p PermissionInfo) bool { return p.Key == key })
	if i < 0 {
		return PermissionInfo{}, false
	}
	return Catalog[i], true
}

// effectivePermissions returns the permissions granted by a role.
func effectivePermissions(r Role) []string {
	if r.SystemKey != nil {
		return systemRolePermissions[*r.SystemKey]
	}
	return r.Permissions
}
