package service

import (
	"context"

	"justixauto/internal/modules/identity/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
)

// CompanyUser lets a company admin manage the company's own employees with
// roles prepared in Admin (user decisions 2026-09-26). An employee belongs to
// exactly one company; only company roles for this company's type can be
// assigned. Account operations reuse the User service after the checks here.
type CompanyUser struct {
	Deps
	users *User
}

// NewCompanyUser builds the company employee service.
func NewCompanyUser(d Deps, users *User) *CompanyUser { return &CompanyUser{Deps: d, users: users} }

// CompanyUserInput creates an employee who can sign in right away with a
// temporary password (changed at first sign-in). Email is optional.
type CompanyUserInput struct {
	DisplayName string   `json:"displayName"`
	Email       string   `json:"email" binding:"optional"`
	Login       string   `json:"login"`
	Password    string   `json:"password"`
	RoleIDs     []string `json:"roleIds"`
}

// UpdateCompanyUserInput changes an employee's name, email and roles.
type UpdateCompanyUserInput struct {
	DisplayName string   `json:"displayName"`
	Email       string   `json:"email" binding:"optional"`
	RoleIDs     []string `json:"roleIds"`
}

// company checks that the actor may manage users of companyID: an active
// member holding company.users.manage. Others see ErrNotFound/Forbidden.
func (s *CompanyUser) company(ctx context.Context, st Store, actor *auth.Principal, companyID string) (*model.Company, error) {
	if err := validID(companyID); err != nil {
		return nil, err
	}
	member, err := s.isMember(ctx, st, actor.UserID, companyID)
	if err != nil {
		return nil, err
	}
	if !member {
		return nil, apperr.ErrNotFound
	}
	if err := actor.Allow(model.PermCompanyUsersManage); err != nil {
		return nil, err
	}
	return st.Companies().Get(ctx, companyID)
}

// employee checks that userID is an active member of the company.
func (s *CompanyUser) employee(ctx context.Context, st Store, companyID, userID string) error {
	if err := validID(userID); err != nil {
		return err
	}
	member, err := s.isMember(ctx, st, userID, companyID)
	if err != nil {
		return err
	}
	if !member {
		return apperr.ErrNotFound
	}
	return nil
}

func assignableRoles(ctx context.Context, st Store, v *apperr.Validation, kind model.CompanyKind, ids []string) ([]string, error) {
	return checkRoles(ctx, st, v, ids, func(r model.Role) bool { return r.AssignableIn(kind) })
}

// Roles lists the prepared roles the company may give its employees.
func (s *CompanyUser) Roles(ctx context.Context, actor *auth.Principal, companyID string) ([]model.Role, error) {
	c, err := s.company(ctx, s.store, actor, companyID)
	if err != nil {
		return nil, err
	}
	return (&Role{s.Deps}).AssignableIn(ctx, c.Kind)
}

// List returns the company's employees.
func (s *CompanyUser) List(ctx context.Context, actor *auth.Principal, companyID string) ([]UserDetail, error) {
	if _, err := s.company(ctx, s.store, actor, companyID); err != nil {
		return nil, err
	}
	ms, err := s.store.Memberships().ActiveInCompany(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]UserDetail, 0, len(ms))
	for _, m := range ms {
		u, err := s.store.Users().Get(ctx, m.UserID)
		if err != nil {
			return nil, err
		}
		d, err := s.users.detail(ctx, s.store, u)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, nil
}

// Create adds an employee with login, temporary password and prepared roles.
func (s *CompanyUser) Create(ctx context.Context, actor *auth.Principal, companyID string, in CompanyUserInput) (*UserDetail, error) {
	var v apperr.Validation
	if in.Login == "" {
		v.Add("login", "required")
	}
	if in.Password == "" {
		v.Add("password", "required")
	}
	n := buildUser(&v, s.clock(), in.DisplayName, in.Email, in.Login, in.Password)
	n.companyID = companyID
	var result *UserDetail
	err := s.store.InTx(ctx, func(st Store) error {
		c, err := s.company(ctx, st, actor, companyID)
		if err != nil {
			return err
		}
		roleIDs, err := assignableRoles(ctx, st, &v, c.Kind, in.RoleIDs)
		if err != nil {
			return err
		}
		if err := v.Err(); err != nil {
			return err
		}
		result, err = s.create(ctx, st, actor, n, roleIDs)
		return err
	})
	return result, err
}

// Update changes an employee's name, email and roles. Admins cannot change
// their own roles, so a company cannot lock itself out by accident.
func (s *CompanyUser) Update(ctx context.Context, actor *auth.Principal, companyID, userID string, expected int64, in UpdateCompanyUserInput) (*UserDetail, error) {
	var result *UserDetail
	err := s.store.InTx(ctx, func(st Store) error {
		c, err := s.company(ctx, st, actor, companyID)
		if err != nil {
			return err
		}
		if err := s.employee(ctx, st, companyID, userID); err != nil {
			return err
		}
		u, err := st.Users().Get(ctx, userID)
		if err != nil {
			return err
		}
		if u.Version != expected {
			return apperr.ErrStale
		}
		var v apperr.Validation
		u.DisplayName = text(&v, "displayName", in.DisplayName, 1, 200)
		u.Email = optionalEmail(&v, "email", in.Email)
		roleIDs, err := assignableRoles(ctx, st, &v, c.Kind, in.RoleIDs)
		if err != nil {
			return err
		}
		if err := v.Err(); err != nil {
			return err
		}
		current, err := st.Roles().UserRoles(ctx, userID)
		if err != nil {
			return err
		}
		before := make([]string, len(current))
		for i, r := range current {
			before[i] = r.ID
		}
		if userID == actor.UserID && !sameIDs(before, roleIDs) {
			return apperr.New(apperr.ErrConflict, "own_roles", "you cannot change your own roles")
		}
		if u.Email != "" {
			taken, err := st.Users().EmailTakenByOther(ctx, u.Email, u.ID)
			if err != nil {
				return err
			}
			if taken {
				return apperr.New(apperr.ErrConflict, "user_exists", "a user with this email already exists")
			}
		}
		u.UpdatedAt = s.clock()
		if err := st.Users().Update(ctx, u, expected); err != nil {
			return err
		}
		if err := st.Roles().SetUserRoles(ctx, userID, roleIDs); err != nil {
			return err
		}
		if err := s.audit(ctx, st, actor, "user.updated", "user", userID, &companyID, "", map[string]any{"rolesBefore": before, "rolesAfter": roleIDs}); err != nil {
			return err
		}
		result, err = s.users.detail(ctx, st, u)
		return err
	})
	return result, err
}

// checkEmployee runs the company and employee checks for account operations.
func (s *CompanyUser) checkEmployee(ctx context.Context, actor *auth.Principal, companyID, userID string) error {
	if _, err := s.company(ctx, s.store, actor, companyID); err != nil {
		return err
	}
	return s.employee(ctx, s.store, companyID, userID)
}

// SetPassword gives an employee a new temporary password.
func (s *CompanyUser) SetPassword(ctx context.Context, actor *auth.Principal, companyID, userID string, expected int64, password string) (*UserDetail, error) {
	if err := s.checkEmployee(ctx, actor, companyID, userID); err != nil {
		return nil, err
	}
	return s.users.SetPassword(ctx, actor, userID, expected, SetPasswordInput{Password: password, PasswordConfirmation: password})
}

// SetStatus suspends or restores an employee; admins cannot suspend themselves.
func (s *CompanyUser) SetStatus(ctx context.Context, actor *auth.Principal, companyID, userID string, expected int64, why string, suspend bool) (*UserDetail, error) {
	if err := s.checkEmployee(ctx, actor, companyID, userID); err != nil {
		return nil, err
	}
	if suspend {
		if userID == actor.UserID {
			return nil, apperr.New(apperr.ErrConflict, "own_account", "you cannot suspend yourself")
		}
		return s.users.Suspend(ctx, actor, userID, expected, why)
	}
	return s.users.Restore(ctx, actor, userID, expected, why)
}

func sameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]bool{}
	for _, x := range a {
		seen[x] = true
	}
	for _, x := range b {
		if !seen[x] {
			return false
		}
	}
	return true
}
