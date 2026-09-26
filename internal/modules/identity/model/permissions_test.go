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

func TestCompanyAdminHoldsEveryCompanyPermission(t *testing.T) {
	RegisterPermissions(PermissionInfo{Key: "retail.read", Scope: "company", Assignable: true})
	key := RoleCompanyAdmin
	got := EffectivePermissions(Role{SystemKey: &key})
	for _, p := range Catalog {
		has := slices.Contains(got, p.Key)
		if p.Scope == "company" && !has {
			t.Errorf("company admin lacks %s", p.Key)
		}
		if p.Scope == "platform" && has {
			t.Errorf("company admin must not hold platform permission %s", p.Key)
		}
	}
	if !slices.Contains(got, "retail.read") {
		t.Error("permissions registered by other modules must be included")
	}
}
