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

// RoleInput prepares a role: a name and a set of permissions (user
// decisions 2026-09-26). Its scope follows from the permissions: company
// permissions make a role company admins may assign to employees; platform
// permissions make a role for JustixAuto staff. Mixing both is rejected.
type RoleInput struct {
	Name           string   `json:"name"`
	PermissionKeys []string `json:"permissionKeys"`
}

// Role manages prepared roles and their permission grants.
type Role struct{ Deps }

// Assignable lists the roles company admins may give their employees:
// company roles, including the built-in company administrator.
func (s *Role) Assignable(ctx context.Context) ([]model.Role, error) {
	roles, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	out := []model.Role{}
	for _, r := range roles {
		if r.AssignableByCompany() {
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

// permissions validates keys (unknown and non-assignable keys are rejected)
// and derives the role's scope from them: platform if they are platform
// permissions, otherwise company. A role cannot mix both scopes.
func permissions(v *apperr.Validation, keys []string) ([]string, string) {
	out := []string{}
	scopes := map[string]bool{}
	for _, k := range keys {
		info, ok := model.LookupPermission(k)
		if !ok || !info.Assignable {
			v.Add("permissionKeys", "unknown or non-assignable permission "+k)
			continue
		}
		scopes[info.Scope] = true
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	if scopes[model.RoleScopePlatform] && scopes[model.RoleScopeCompany] {
		v.Add("permissionKeys", "a role cannot mix platform and company permissions")
	}
	if scopes[model.RoleScopePlatform] {
		return out, model.RoleScopePlatform
	}
	return out, model.RoleScopeCompany
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
	perms, scope := permissions(&v, in.PermissionKeys)
	r := &model.Role{
		ID: uuid.NewString(), Name: text(&v, "name", in.Name, 1, 100), Scope: scope,
		Permissions: perms, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	err := s.store.InTx(ctx, func(st Store) error {
		if err := st.Roles().Create(ctx, r); err != nil {
			return duplicateRole(err)
		}
		return s.audit(ctx, st, actor, "role.created", "role", r.ID, nil, "", map[string]any{"name": r.Name, "scope": r.Scope, "permissions": r.Permissions})
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
		r.Permissions, r.Scope = permissions(&v, in.PermissionKeys)
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
