package model

import (
	"slices"
	"testing"
)

func TestSystemRolePermissionsAreCatalogued(t *testing.T) {
	for role, perms := range systemRolePermissions {
		for _, p := range perms {
			if _, ok := LookupPermission(p); !ok {
				t.Fatalf("%s grants uncatalogued permission %s", role, p)
			}
		}
	}
	// Administration powers never include company business permissions.
	if slices.Contains(systemRolePermissions[RolePlatformAdmin], PermCompanyEdit) {
		t.Fatal("platform admin must not implicitly edit companies as a member")
	}
}
