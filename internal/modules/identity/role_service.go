package identity

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
)

type RoleInput struct {
	Name           string   `json:"name"`
	PermissionKeys []string `json:"permissionKeys"`
}

type RoleService struct{ deps }

func (s *RoleService) List(ctx context.Context) ([]Role, error) {
	roles, err := s.store.Roles().List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		roles[i].Permissions = effectivePermissions(roles[i])
	}
	return roles, nil
}

// permissions validates keys: unknown and non-assignable keys are rejected.
func permissions(v *apperr.Validation, keys []string) []string {
	out := []string{}
	for _, k := range keys {
		info, ok := permissionInfo(k)
		if !ok || !info.Assignable {
			v.Add("permissionKeys", "unknown or non-assignable permission "+k)
			continue
		}
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

func duplicateRole(err error) error {
	if err != nil && isConflict(err) {
		return apperr.New(apperr.ErrConflict, "role_duplicate", "a role with this name already exists")
	}
	return err
}

func (s *RoleService) Create(ctx context.Context, actor *auth.Principal, in RoleInput) (*Role, error) {
	var v apperr.Validation
	now := s.clock()
	r := &Role{ID: uuid.NewString(), Name: text(&v, "name", in.Name, 1, 100),
		Permissions: permissions(&v, in.PermissionKeys), Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := v.Err(); err != nil {
		return nil, err
	}
	err := s.store.InTx(ctx, func(st Store) error {
		if err := st.Roles().Create(ctx, r); err != nil {
			return duplicateRole(err)
		}
		return s.audit(ctx, st, actor, "role.created", "role", r.ID, nil, "", map[string]any{"name": r.Name, "permissions": r.Permissions})
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Update replaces a custom role's name and permissions. System roles are fixed.
func (s *RoleService) Update(ctx context.Context, actor *auth.Principal, id string, expected int64, in RoleInput) (*Role, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	var result *Role
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
		r.Permissions = permissions(&v, in.PermissionKeys)
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
