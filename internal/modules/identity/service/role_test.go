package service

import (
	"testing"

	"justixauto/internal/modules/identity/model"
	"justixauto/internal/pkg/apperr"
)

func TestRoleScopeRules(t *testing.T) {
	cases := []struct {
		name    string
		in      RoleInput
		wantErr bool
		kind    string
	}{
		{"company any type", RoleInput{Scope: "company"}, false, ""},
		{"company seller", RoleInput{Scope: "company", CompanyKind: "seller"}, false, "seller"},
		{"platform", RoleInput{Scope: "platform"}, false, ""},
		{"platform with type", RoleInput{Scope: "platform", CompanyKind: "bank"}, true, ""},
		{"unknown type", RoleInput{Scope: "company", CompanyKind: "shop"}, true, ""},
		{"missing scope defaults to company", RoleInput{CompanyKind: "bank"}, false, "bank"},
		{"unknown scope", RoleInput{Scope: "global"}, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var v apperr.Validation
			_, kind := roleScope(&v, c.in)
			if (v.Err() != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", v.Err(), c.wantErr)
			}
			if !c.wantErr && c.kind != "" && (kind == nil || string(*kind) != c.kind) {
				t.Fatalf("kind = %v, want %s", kind, c.kind)
			}
		})
	}
}

func TestPermissionsMustMatchRoleScope(t *testing.T) {
	var v apperr.Validation
	got := permissions(&v, model.RoleScopeCompany, []string{model.PermCompanyEdit, model.PermCompanyEdit})
	if v.Err() != nil || len(got) != 1 {
		t.Fatalf("company permission in company role: got %v, err %v", got, v.Err())
	}
	v = apperr.Validation{}
	permissions(&v, model.RoleScopeCompany, []string{model.PermPlatformAuditRead})
	if v.Err() == nil {
		t.Fatal("platform permission must be rejected in a company role")
	}
	v = apperr.Validation{}
	permissions(&v, model.RoleScopePlatform, []string{model.PermCompanyEdit})
	if v.Err() == nil {
		t.Fatal("company permission must be rejected in a platform role")
	}
}

func TestRoleAssignableIn(t *testing.T) {
	seller := model.KindSeller
	any := model.Role{Scope: model.RoleScopeCompany}
	sellerOnly := model.Role{Scope: model.RoleScopeCompany, CompanyKind: &seller}
	staff := model.Role{Scope: model.RoleScopePlatform}
	if !any.AssignableIn(model.KindBank) || !sellerOnly.AssignableIn(model.KindSeller) {
		t.Fatal("company roles must be assignable in matching companies")
	}
	if sellerOnly.AssignableIn(model.KindBank) || staff.AssignableIn(model.KindSeller) {
		t.Fatal("seller-only and platform roles must not be assignable in a bank")
	}
}
