package service

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"justixauto/internal/modules/identity/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
)

// NewRole builds the role service.
func NewRole(d Deps) *Role { return &Role{d} }

// RoleInput prepares a role (user decisions 2026-09-26): scope "platform"
// (JustixAuto staff, platform permissions) or "company" (assigned by company
// admins, company permissions), and for company roles an optional company
// type that limits which companies may assign it.
type RoleInput struct {
	Name           string   `json:"name"`
	Scope          string   `json:"scope" binding:"optional"` // default "company"
	CompanyKind    string   `json:"companyKind" binding:"optional"`
	PermissionKeys []string `json:"permissionKeys"`
}

// Role manages prepared roles and their permission grants.
type Role struct{ Deps }

// AssignableIn lists the company roles a company of kind may give its
// employees: company-scope roles for any type or exactly this type,
// including the built-in company administrator.
func (s *Role) AssignableIn(ctx context.Context, kind model.CompanyKind) ([]model.Role, error) {
	roles, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	out := []model.Role{}
	for _, r := range roles {
		if r.AssignableIn(kind) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Role) List(ctx context.Context) ([]model.Role, error) {
	roles, err := s.store.Roles().List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		roles[i].Permissions = model.EffectivePermissions(roles[i])
	}
	return roles, nil
}

// permissions validates keys: unknown, non-assignable and keys of another
// scope than the role's are rejected.
func permissions(v *apperr.Validation, scope string, keys []string) []string {
	out := []string{}
	for _, k := range keys {
		info, ok := model.LookupPermission(k)
		if !ok || !info.Assignable {
			v.Add("permissionKeys", "unknown or non-assignable permission "+k)
			continue
		}
		if info.Scope != scope {
			v.Add("permissionKeys", "permission "+k+" does not belong to a "+scope+" role")
			continue
		}
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

// roleScope validates the scope and company type of a prepared role.
func roleScope(v *apperr.Validation, in RoleInput) (string, *model.CompanyKind) {
	if in.Scope == "" {
		in.Scope = model.RoleScopeCompany // most prepared roles are company roles
	}
	if in.Scope != model.RoleScopePlatform && in.Scope != model.RoleScopeCompany {
		v.Add("scope", "must be platform or company")
		return in.Scope, nil
	}
	if in.CompanyKind == "" {
		return in.Scope, nil
	}
	kind := model.CompanyKind(in.CompanyKind)
	switch {
	case in.Scope != model.RoleScopeCompany:
		v.Add("companyKind", "only company roles have a company type")
	case !kind.Valid():
		v.Add("companyKind", "unknown company type")
	}
	return in.Scope, &kind
}

func duplicateRole(err error) error {
	if err != nil && isConflict(err) {
		return apperr.New(apperr.ErrConflict, "role_duplicate", "a role with this name already exists")
	}
	return err
}

func (s *Role) Create(ctx context.Context, actor *auth.Principal, in RoleInput) (*model.Role, error) {
	var v apperr.Validation
	now := s.clock()
	scope, kind := roleScope(&v, in)
	r := &model.Role{
		ID: uuid.NewString(), Name: text(&v, "name", in.Name, 1, 100), Scope: scope, CompanyKind: kind,
		Permissions: permissions(&v, scope, in.PermissionKeys), Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	err := s.store.InTx(ctx, func(st Store) error {
		if err := st.Roles().Create(ctx, r); err != nil {
			return duplicateRole(err)
		}
		return s.audit(ctx, st, actor, "role.created", "role", r.ID, nil, "", map[string]any{"name": r.Name, "scope": r.Scope, "companyKind": r.CompanyKind, "permissions": r.Permissions})
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Update replaces a custom role's name and permissions. System roles are fixed.
func (s *Role) Update(ctx context.Context, actor *auth.Principal, id string, expected int64, in RoleInput) (*model.Role, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	var result *model.Role
	err := s.store.InTx(ctx, func(st Store) error {
		r, err := st.Roles().Get(ctx, id)
		if err != nil {
			return err
		}
		if r.System() {
			return apperr.New(apperr.ErrConflict, "system_role", "system roles cannot be changed")
		}
		if r.Version != expected {
			return apperr.ErrStale
		}
		var v apperr.Validation
		before := r.Permissions
		r.Name = text(&v, "name", in.Name, 1, 100)
		r.Scope, r.CompanyKind = roleScope(&v, in)
		r.Permissions = permissions(&v, r.Scope, in.PermissionKeys)
		if err := v.Err(); err != nil {
			return err
		}
		r.UpdatedAt = s.clock()
		if err := st.Roles().Update(ctx, r, expected); err != nil {
			return duplicateRole(err)
		}
		result = r
		return s.audit(ctx, st, actor, "role.updated", "role", r.ID, nil, "", map[string]any{"permissionsBefore": before, "permissionsAfter": r.Permissions})
	})
	return result, err
}
