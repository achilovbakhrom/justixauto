package service

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/modules/identity/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
)

// NewUser builds the user service.
func NewUser(d Deps) *User { return &User{d} }

type CreateUserInput struct {
	DisplayName string   `json:"displayName"`
	Email       string   `json:"email"`
	RoleIDs     []string `json:"roleIds"`
}

type UpdateUserInput struct {
	DisplayName string   `json:"displayName"`
	RoleIDs     []string `json:"roleIds"`
}

// UserDetail is a user with its global roles.
type UserDetail struct {
	User  *model.User
	Roles []model.Role
}

// User manages platform user accounts.
type User struct{ Deps }

func (s *User) roles(ctx context.Context, st Store, v *apperr.Validation, ids []string) ([]string, error) {
	ids = uniqueIDs(v, "roleIds", ids)
	if len(ids) == 0 {
		return ids, nil
	}
	found, err := st.Roles().GetMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	if len(found) != len(ids) {
		v.Add("roleIds", "contains unknown roles")
	}
	return ids, nil
}

func (s *User) detail(ctx context.Context, st Store, u *model.User) (*UserDetail, error) {
	roles, err := st.Roles().UserRoles(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &UserDetail{User: u, Roles: roles}, nil
}

// Create registers a pending user. No password or enrollment is created here:
// credentials are set through a separate, security-approved flow.
func (s *User) Create(ctx context.Context, actor *auth.Principal, in CreateUserInput) (*UserDetail, error) {
	var v apperr.Validation
	now := s.clock()
	u := &model.User{
		ID: uuid.NewString(), DisplayName: text(&v, "displayName", in.DisplayName, 1, 200),
		Email: email(&v, "email", in.Email), Status: model.UserPending, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	var result *UserDetail
	err := s.store.InTx(ctx, func(st Store) error {
		roleIDs, err := s.roles(ctx, st, &v, in.RoleIDs)
		if err != nil {
			return err
		}
		if err := v.Err(); err != nil {
			return err
		}
		taken, err := st.Users().EmailOrLoginTaken(ctx, u.Email, nil)
		if err != nil {
			return err
		}
		if taken {
			return apperr.New(apperr.ErrConflict, "user_exists", "a user with this email already exists")
		}
		if err := st.Users().Create(ctx, u); err != nil {
			return err
		}
		if err := st.Roles().SetUserRoles(ctx, u.ID, roleIDs); err != nil {
			return err
		}
		if err := s.audit(ctx, st, actor, "user.created", "user", u.ID, nil, "", map[string]any{"email": u.Email, "roleIds": roleIDs}); err != nil {
			return err
		}
		result, err = s.detail(ctx, st, u)
		return err
	})
	return result, err
}

func (s *User) Get(ctx context.Context, id string) (*UserDetail, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	u, err := s.store.Users().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.detail(ctx, s.store, u)
}

func (s *User) List(ctx context.Context, limit, offset int) ([]UserDetail, error) {
	users, err := s.store.Users().List(ctx, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]UserDetail, 0, len(users))
	for i := range users {
		d, err := s.detail(ctx, s.store, &users[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, nil
}

func hasRole(roles []model.Role, id string) bool {
	return slices.ContainsFunc(roles, func(r model.Role) bool { return r.ID == id })
}

// guardAdminRemoval blocks self-lockout and removal of the last active
// platform admin. It must run inside the transaction that makes the change.
func (s *User) guardAdminRemoval(ctx context.Context, st Store, actor *auth.Principal, target *model.User, roles []model.Role) error {
	if target.Status != model.UserActive || !hasRole(roles, model.PlatformAdminRoleID) {
		return nil
	}
	if target.ID == actor.UserID {
		return apperr.New(apperr.ErrConflict, "self_lockout", "you cannot remove your own platform administration access")
	}
	if err := st.Roles().LockAdminGuard(ctx); err != nil {
		return err
	}
	n, err := st.Roles().CountActivePlatformAdmins(ctx, target.ID)
	if err != nil {
		return err
	}
	if n == 0 {
		return apperr.New(apperr.ErrConflict, "last_platform_admin", "the last active platform administrator cannot be removed")
	}
	return nil
}

// Update changes the display name and global roles.
func (s *User) Update(ctx context.Context, actor *auth.Principal, id string, expected int64, in UpdateUserInput) (*UserDetail, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	var result *UserDetail
	err := s.store.InTx(ctx, func(st Store) error {
		u, err := st.Users().Get(ctx, id)
		if err != nil {
			return err
		}
		if u.Version != expected {
			return apperr.ErrStale
		}
		var v apperr.Validation
		u.DisplayName = text(&v, "displayName", in.DisplayName, 1, 200)
		roleIDs, err := s.roles(ctx, st, &v, in.RoleIDs)
		if err != nil {
			return err
		}
		if err := v.Err(); err != nil {
			return err
		}
		current, err := st.Roles().UserRoles(ctx, id)
		if err != nil {
			return err
		}
		if !slices.Contains(roleIDs, model.PlatformAdminRoleID) {
			if err := s.guardAdminRemoval(ctx, st, actor, u, current); err != nil {
				return err
			}
		}
		u.UpdatedAt = s.clock()
		if err := st.Users().Update(ctx, u, expected); err != nil {
			return err
		}
		if err := st.Roles().SetUserRoles(ctx, id, roleIDs); err != nil {
			return err
		}
		before := make([]string, len(current))
		for i, r := range current {
			before[i] = r.ID
		}
		if err := s.audit(ctx, st, actor, "user.updated", "user", id, nil, "", map[string]any{"rolesBefore": before, "rolesAfter": roleIDs}); err != nil {
			return err
		}
		result, err = s.detail(ctx, st, u)
		return err
	})
	return result, err
}

// Suspend blocks sign-in and ends all sessions; history is kept.
func (s *User) Suspend(ctx context.Context, actor *auth.Principal, id string, expected int64, why string) (*UserDetail, error) {
	return s.setStatus(ctx, actor, id, expected, why, true)
}

// Restore re-enables a suspended user (pending again if it has no credential).
func (s *User) Restore(ctx context.Context, actor *auth.Principal, id string, expected int64, why string) (*UserDetail, error) {
	return s.setStatus(ctx, actor, id, expected, why, false)
}

// applySuspend transitions u into UserSuspended, guarding against re-suspending,
// self-lockout and removing the last administrator, and revokes its sessions.
func (s *User) applySuspend(ctx context.Context, st Store, actor *auth.Principal, u *model.User, roles []model.Role, now time.Time) error {
	if u.Status == model.UserSuspended {
		return apperr.New(apperr.ErrConflict, "invalid_transition", "the user is already suspended")
	}
	if u.ID == actor.UserID {
		return apperr.New(apperr.ErrConflict, "self_lockout", "you cannot suspend yourself")
	}
	if err := s.guardAdminRemoval(ctx, st, actor, u, roles); err != nil {
		return err
	}
	u.Status = model.UserSuspended
	return st.Sessions().RevokeUser(ctx, u.ID, "", now)
}

// applyRestore transitions a suspended u back to pending or active, depending
// on whether it already has a credential.
func applyRestore(u *model.User) error {
	if u.Status != model.UserSuspended {
		return apperr.New(apperr.ErrConflict, "invalid_transition", "only suspended users can be restored")
	}
	u.Status = model.UserPending
	if u.PasswordHash != nil {
		u.Status = model.UserActive
	}
	return nil
}

func (s *User) setStatus(ctx context.Context, actor *auth.Principal, id string, expected int64, why string, suspend bool) (*UserDetail, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	var v apperr.Validation
	why = reason(&v, why)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *UserDetail
	err := s.store.InTx(ctx, func(st Store) error {
		u, err := st.Users().Get(ctx, id)
		if err != nil {
			return err
		}
		if u.Version != expected {
			return apperr.ErrStale
		}
		roles, err := st.Roles().UserRoles(ctx, id)
		if err != nil {
			return err
		}
		before := u.Status
		now := s.clock()
		action := "user.restored"
		if suspend {
			action = "user.suspended"
			if err := s.applySuspend(ctx, st, actor, u, roles, now); err != nil {
				return err
			}
		} else if err := applyRestore(u); err != nil {
			return err
		}
		u.StatusReason, u.UpdatedAt = why, now
		if err := st.Users().Update(ctx, u, expected); err != nil {
			return err
		}
		if err := s.audit(ctx, st, actor, action, "user", id, nil, why, map[string]any{"before": before, "after": u.Status}); err != nil {
			return err
		}
		result = &UserDetail{User: u, Roles: roles}
		return nil
	})
	return result, err
}

// RevokeSessions signs the user out everywhere.
func (s *User) RevokeSessions(ctx context.Context, actor *auth.Principal, id, why string) error {
	if err := validID(id); err != nil {
		return err
	}
	var v apperr.Validation
	why = reason(&v, why)
	if err := v.Err(); err != nil {
		return err
	}
	return s.store.InTx(ctx, func(st Store) error {
		if _, err := st.Users().Get(ctx, id); err != nil {
			return err
		}
		if err := st.Sessions().RevokeUser(ctx, id, "", s.clock()); err != nil {
			return err
		}
		return s.audit(ctx, st, actor, "user.sessions_revoked", "user", id, nil, why, nil)
	})
}

// BootstrapInput creates the very first platform administrator.
type BootstrapInput struct {
	DisplayName, Login, Email, Password string
}

// Bootstrap creates the first active platform admin. It is single-use: once
// any active platform admin exists it refuses, so it can never reset access.
func (s *User) Bootstrap(ctx context.Context, in BootstrapInput) (*model.User, error) {
	var v apperr.Validation
	now := s.clock()
	login := text(&v, "login", in.Login, 3, 100)
	u := &model.User{
		ID: uuid.NewString(), DisplayName: text(&v, "displayName", in.DisplayName, 1, 200),
		Email: email(&v, "email", in.Email), Login: &login, Status: model.UserActive, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	validatePassword(&v, "password", in.Password, in.Password)
	if err := v.Err(); err != nil {
		return nil, err
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	u.PasswordHash = &hash
	err = s.store.InTx(ctx, func(st Store) error {
		if err := st.Roles().LockAdminGuard(ctx); err != nil {
			return err
		}
		n, err := st.Roles().CountActivePlatformAdmins(ctx, "")
		if err != nil {
			return err
		}
		if n > 0 {
			return apperr.New(apperr.ErrConflict, "already_bootstrapped", "a platform administrator already exists")
		}
		taken, err := st.Users().EmailOrLoginTaken(ctx, u.Email, u.Login)
		if err != nil {
			return err
		}
		if taken {
			return apperr.New(apperr.ErrConflict, "user_exists", "a user with this login or email already exists")
		}
		if err := st.Users().Create(ctx, u); err != nil {
			return err
		}
		if err := st.Roles().SetUserRoles(ctx, u.ID, []string{model.PlatformAdminRoleID}); err != nil {
			return err
		}
		return s.audit(ctx, st, nil, "user.bootstrapped", "user", u.ID, nil, "", map[string]any{"login": login})
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}

type SetPasswordInput struct {
	Login                string `json:"login"` // required if the user has none yet
	Password             string `json:"password"`
	PasswordConfirmation string `json:"passwordConfirmation"`
}

// SetPassword lets an administrator give a pending user credentials, or reset
// a forgotten password. The user must choose a new password at the next
// sign-in; existing sessions end. Two-factor settings are not touched, so this
// never bypasses MFA.
func (s *User) SetPassword(ctx context.Context, actor *auth.Principal, id string, expected int64, in SetPasswordInput) (*UserDetail, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	if id == actor.UserID {
		return nil, apperr.New(apperr.ErrConflict, "use_own_password_change", "change your own password in your session settings")
	}
	var result *UserDetail
	err := s.store.InTx(ctx, func(st Store) error {
		u, err := st.Users().Get(ctx, id)
		if err != nil {
			return err
		}
		if u.Version != expected {
			return apperr.ErrStale
		}
		if u.Status == model.UserSuspended {
			return apperr.New(apperr.ErrConflict, "user_suspended", "restore the user first")
		}
		var v apperr.Validation
		if u.Login == nil || in.Login != "" {
			login := validLogin(&v, "login", in.Login)
			u.Login = &login
		}
		validatePassword(&v, "password", in.Password, in.PasswordConfirmation)
		if err := v.Err(); err != nil {
			return err
		}
		if taken, err := st.Users().LoginTaken(ctx, *u.Login, u.ID); err != nil {
			return err
		} else if taken {
			return apperr.New(apperr.ErrConflict, "login_taken", "another user already has this login")
		}
		hash, err := hashPassword(in.Password)
		if err != nil {
			return err
		}
		now := s.clock()
		u.PasswordHash, u.PasswordChangeRequired, u.UpdatedAt = &hash, true, now
		if u.Status == model.UserPending {
			u.Status = model.UserActive
		}
		if err := st.Users().Update(ctx, u, expected); err != nil {
			return err
		}
		if err := st.Sessions().RevokeUser(ctx, id, "", now); err != nil {
			return err
		}
		if err := s.audit(ctx, st, actor, "user.password_set", "user", id, nil, "", map[string]any{"login": *u.Login}); err != nil {
			return err
		}
		result, err = s.detail(ctx, st, u)
		return err
	})
	return result, err
}
