package service

import (
	"testing"

	"justixauto/internal/modules/identity/model"
	"justixauto/internal/pkg/apperr"
)

func TestPermissionsDeriveRoleScope(t *testing.T) {
	var v apperr.Validation
	perms, scope := permissions(&v, []string{model.PermCompanyEdit, model.PermCompanyEdit})
	if v.Err() != nil || len(perms) != 1 || scope != model.RoleScopeCompany {
		t.Fatalf("company permissions: got %v %s, err %v", perms, scope, v.Err())
	}
	v = apperr.Validation{}
	if _, scope = permissions(&v, []string{model.PermPlatformAuditRead}); v.Err() != nil || scope != model.RoleScopePlatform {
		t.Fatalf("platform permission: scope %s, err %v", scope, v.Err())
	}
	v = apperr.Validation{}
	if _, scope = permissions(&v, nil); v.Err() != nil || scope != model.RoleScopeCompany {
		t.Fatalf("empty role: scope %s, err %v", scope, v.Err())
	}
	v = apperr.Validation{}
	permissions(&v, []string{model.PermCompanyEdit, model.PermPlatformAuditRead})
	if v.Err() == nil {
		t.Fatal("a role mixing platform and company permissions must be rejected")
	}
	v = apperr.Validation{}
	permissions(&v, []string{model.PermPlatformUsersManage})
	if v.Err() == nil {
		t.Fatal("non-assignable permissions must be rejected")
	}
}

func TestOnlyCompanyRolesAreAssignableByCompanies(t *testing.T) {
	if !(model.Role{Scope: model.RoleScopeCompany}).AssignableByCompany() {
		t.Fatal("company roles must be assignable by company admins")
	}
	if (model.Role{Scope: model.RoleScopePlatform}).AssignableByCompany() {
		t.Fatal("platform roles must not be assignable by company admins")
	}
}
