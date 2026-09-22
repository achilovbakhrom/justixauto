package identity

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
)

// Label is a free-text value optionally chosen from a catalogue (country, region).
type Label struct {
	Key   string `json:"key,omitempty"`
	Label string `json:"label"`
}

// CompanyInput is the contract's CompanyInput.
type CompanyInput struct {
	Name         string `json:"name"`
	LegalName    string `json:"legalName"`
	Country      Label  `json:"country"`
	Region       *Label `json:"region"`
	Registration string `json:"registration"`
	Email        string `json:"email"`
	Address      string `json:"address"`
	Phone        string `json:"phone"`
}

type FirstAdminInput struct {
	DisplayName          string `json:"displayName"`
	Login                string `json:"login"`
	Email                string `json:"email"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"passwordConfirmation"`
}

type ProviderInput struct {
	Kind       CompanyKind     `json:"kind"`
	Company    CompanyInput    `json:"company"`
	FirstAdmin FirstAdminInput `json:"firstAdmin"`
}

type CompanyService struct{ deps }

func (in CompanyInput) apply(v *apperr.Validation, c *Company) {
	c.Name = text(v, "company.name", in.Name, 1, 200)
	c.LegalName = text(v, "company.legalName", in.LegalName, 0, 300)
	c.Country = text(v, "company.country", in.Country.Label, 1, 100)
	c.CountryKey = text(v, "company.country", in.Country.Key, 0, 50)
	c.Region, c.RegionKey = "", ""
	if in.Region != nil {
		c.Region = text(v, "company.region", in.Region.Label, 0, 100)
		c.RegionKey = text(v, "company.region", in.Region.Key, 0, 50)
		if c.Region != "" && c.Country == "" {
			v.Add("company.region", "requires a country")
		}
	}
	c.RegistrationNumber = text(v, "company.registration", in.Registration, 1, 64)
	c.Email = email(v, "company.email", in.Email)
	c.Address = text(v, "company.address", in.Address, 0, 500)
	c.Phone = text(v, "company.phone", in.Phone, 0, 50)
}

func duplicateCompany(err error) error {
	if errors.Is(err, apperr.ErrConflict) {
		return apperr.New(apperr.ErrConflict, "company_duplicate", "a company with this country and registration number already exists")
	}
	return err
}

func (s *CompanyService) newCompany(v *apperr.Validation, kind CompanyKind, in CompanyInput) *Company {
	now := s.clock()
	c := &Company{ID: uuid.NewString(), Kind: kind, Status: AccessDraft, Version: 1, CreatedAt: now, UpdatedAt: now}
	in.apply(v, c)
	return c
}

// CreateSeller registers a seller company and makes the creator a member of
// all its branches. It does not grant roles or switch the working context.
func (s *CompanyService) CreateSeller(ctx context.Context, actor *auth.Principal, in CompanyInput) (*Company, error) {
	var v apperr.Validation
	c := s.newCompany(&v, KindSeller, in)
	if err := v.Err(); err != nil {
		return nil, err
	}
	err := s.store.InTx(ctx, func(st Store) error {
		if err := st.Companies().Create(ctx, c); err != nil {
			return duplicateCompany(err)
		}
		m := &Membership{ID: uuid.NewString(), UserID: actor.UserID, CompanyID: c.ID, Status: MembershipActive,
			BranchAccess: AllBranches, Version: 1, CreatedAt: c.CreatedAt, UpdatedAt: c.CreatedAt}
		if err := st.Memberships().Create(ctx, m); err != nil {
			return err
		}
		return s.audit(ctx, st, actor, "company.created", "company", c.ID, &c.ID, "", map[string]any{"kind": c.Kind, "name": c.Name})
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// ProvisionResult lists what CreateProvider created.
type ProvisionResult struct {
	Company    *Company
	Admin      *User
	Membership *Membership
}

// CreateProvider atomically creates a draft bank/MFO/insurance company, its
// first administrator with credentials, the company-admin role and membership.
// An existing login or email rejects everything; accounts are never linked.
func (s *CompanyService) CreateProvider(ctx context.Context, actor *auth.Principal, in ProviderInput) (*ProvisionResult, error) {
	var v apperr.Validation
	if !in.Kind.Provider() {
		v.Add("kind", "must be one of bank, mfo, insurance")
	}
	c := s.newCompany(&v, in.Kind, in.Company)
	a := in.FirstAdmin
	login := strings.TrimSpace(a.Login)
	if n := len([]rune(login)); n < 3 || n > 100 || strings.ContainsAny(login, " \t\n") {
		v.Add("firstAdmin.login", "3-100 characters without spaces")
	}
	now := c.CreatedAt
	u := &User{ID: uuid.NewString(), DisplayName: text(&v, "firstAdmin.displayName", a.DisplayName, 1, 200),
		Email: email(&v, "firstAdmin.email", a.Email), Login: &login, Status: UserActive,
		Version: 1, CreatedAt: now, UpdatedAt: now}
	validatePassword(&v, "firstAdmin.password", a.Password, a.PasswordConfirmation)
	if err := v.Err(); err != nil {
		return nil, err
	}
	hash, err := hashPassword(a.Password)
	if err != nil {
		return nil, err
	}
	u.PasswordHash = &hash
	m := &Membership{ID: uuid.NewString(), UserID: u.ID, CompanyID: c.ID, Status: MembershipActive,
		BranchAccess: AllBranches, Version: 1, CreatedAt: now, UpdatedAt: now}

	err = s.store.InTx(ctx, func(st Store) error {
		taken, err := st.Users().EmailOrLoginTaken(ctx, u.Email, u.Login)
		if err != nil {
			return err
		}
		if taken {
			return apperr.New(apperr.ErrConflict, "user_exists", "a user with this login or email already exists")
		}
		if err := st.Companies().Create(ctx, c); err != nil {
			return duplicateCompany(err)
		}
		if err := st.Users().Create(ctx, u); err != nil {
			return err
		}
		if err := st.Roles().SetUserRoles(ctx, u.ID, []string{CompanyAdminRoleID}); err != nil {
			return err
		}
		if err := st.Memberships().Create(ctx, m); err != nil {
			return err
		}
		return s.audit(ctx, st, actor, "company.provider_provisioned", "company", c.ID, &c.ID, "",
			map[string]any{"kind": c.Kind, "name": c.Name, "adminUserId": u.ID, "adminLogin": login})
	})
	if err != nil {
		return nil, err
	}
	return &ProvisionResult{Company: c, Admin: u, Membership: m}, nil
}

// canSee: members see their company; directory readers see all companies.
// Everyone else gets ErrNotFound so existence is not revealed.
func (s *CompanyService) visible(ctx context.Context, actor *auth.Principal, id string) (*Company, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	if !actor.Can(PermPlatformDirectoryRead) {
		member, err := s.isMember(ctx, s.store, actor.UserID, id)
		if err != nil {
			return nil, err
		}
		if !member {
			return nil, apperr.ErrNotFound
		}
	}
	return s.store.Companies().Get(ctx, id)
}

func (s *CompanyService) Get(ctx context.Context, actor *auth.Principal, id string) (*Company, error) {
	return s.visible(ctx, actor, id)
}

func (s *CompanyService) List(ctx context.Context, f CompanyFilter) ([]Company, error) {
	var v apperr.Validation
	if f.Kind != "" && !f.Kind.Valid() {
		v.Add("kind", "unknown kind")
	}
	if f.Access != "" && f.Access != AccessDraft && f.Access != AccessActive && f.Access != AccessSuspended {
		v.Add("access", "unknown access state")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	return s.store.Companies().List(ctx, f)
}

// Update edits requisites: company editors who are members, or platform admins.
// The ID, kind, memberships and history stay the same.
func (s *CompanyService) Update(ctx context.Context, actor *auth.Principal, id string, expected int64, in CompanyInput) (*Company, error) {
	c, err := s.visible(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if !actor.Can(PermPlatformCompaniesAccess) {
		member, err := s.isMember(ctx, s.store, actor.UserID, id)
		if err != nil {
			return nil, err
		}
		if !member || !actor.Can(PermCompanyEdit) {
			return nil, apperr.New(apperr.ErrForbidden, "permission_denied", "missing permission company.edit")
		}
	}
	if c.Version != expected {
		return nil, apperr.ErrStale
	}
	var v apperr.Validation
	in.apply(&v, c)
	if err := v.Err(); err != nil {
		return nil, err
	}
	c.UpdatedAt = s.clock()
	err = s.store.InTx(ctx, func(st Store) error {
		if err := st.Companies().Update(ctx, c, expected); err != nil {
			return duplicateCompany(err)
		}
		return s.audit(ctx, st, actor, "company.updated", "company", c.ID, &c.ID, "", map[string]any{"name": c.Name, "registration": c.RegistrationNumber})
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Access actions of the Admin registry. Activation does not confirm licences,
// APIs or the right to sell any product.
const (
	ActionActivate = "activate"
	ActionSuspend  = "suspend"
	ActionRestore  = "restore"
)

var accessTransitions = map[string]struct{ from, to CompanyAccess }{
	ActionActivate: {AccessDraft, AccessActive},
	ActionSuspend:  {AccessActive, AccessSuspended},
	ActionRestore:  {AccessSuspended, AccessActive},
}

func (s *CompanyService) SetAccess(ctx context.Context, actor *auth.Principal, id string, expected int64, action, why string) (*Company, error) {
	t, ok := accessTransitions[action]
	if !ok {
		return nil, apperr.ErrNotFound
	}
	if err := validID(id); err != nil {
		return nil, err
	}
	c, err := s.store.Companies().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Version != expected {
		return nil, apperr.ErrStale
	}
	var v apperr.Validation
	why = reason(&v, why)
	if err := v.Err(); err != nil {
		return nil, err
	}
	if c.Status != t.from {
		return nil, apperr.New(apperr.ErrConflict, "invalid_transition", "cannot "+action+" a company that is "+string(c.Status))
	}
	before := c.Status
	c.Status, c.StatusReason, c.UpdatedAt = t.to, why, s.clock()
	err = s.store.InTx(ctx, func(st Store) error {
		if err := st.Companies().Update(ctx, c, expected); err != nil {
			return err
		}
		return s.audit(ctx, st, actor, "company."+action, "company", c.ID, &c.ID, why, map[string]any{"before": before, "after": c.Status})
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}
