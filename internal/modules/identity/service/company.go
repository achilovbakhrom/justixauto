package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"justixauto/internal/modules/identity/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
)

// NewCompany builds the company service.
func NewCompany(d Deps) *Company { return &Company{d} }

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
	Kind       model.CompanyKind `json:"kind"`
	Company    CompanyInput      `json:"company"`
	FirstAdmin FirstAdminInput   `json:"firstAdmin"`
}

// Company manages company registration, requisites and platform access.
type Company struct{ Deps }

func (in CompanyInput) apply(v *apperr.Validation, c *model.Company) {
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

func (s *Company) newCompany(v *apperr.Validation, kind model.CompanyKind, in CompanyInput) *model.Company {
	now := s.clock()
	c := &model.Company{ID: uuid.NewString(), Kind: kind, Status: model.AccessDraft, Version: 1, CreatedAt: now, UpdatedAt: now}
	in.apply(v, c)
	return c
}

// CreateSeller registers a seller company and makes the creator a member of
// all its branches. It does not grant roles or switch the working context.
func (s *Company) CreateSeller(ctx context.Context, actor *auth.Principal, in CompanyInput) (*model.Company, error) {
	var v apperr.Validation
	c := s.newCompany(&v, model.KindSeller, in)
	if err := v.Err(); err != nil {
		return nil, err
	}
	err := s.store.InTx(ctx, func(st Store) error {
		if err := st.Companies().Create(ctx, c); err != nil {
			return duplicateCompany(err)
		}
		m := &model.Membership{
			ID: uuid.NewString(), UserID: actor.UserID, CompanyID: c.ID, Status: model.MembershipActive,
			BranchAccess: model.AllBranches, Version: 1, CreatedAt: c.CreatedAt, UpdatedAt: c.CreatedAt,
		}
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
	Company    *model.Company
	Admin      *model.User
	Membership *model.Membership
}

// CreateProvider atomically creates a draft bank/MFO/insurance company, its
// first administrator with credentials, the company-admin role and membership.
// An existing login or email rejects everything; accounts are never linked.
func (s *Company) CreateProvider(ctx context.Context, actor *auth.Principal, in ProviderInput) (*ProvisionResult, error) {
	var v apperr.Validation
	if !in.Kind.Provider() {
		v.Add("kind", "must be one of bank, mfo, insurance")
	}
	return s.provision(ctx, actor, &v, in.Kind, in.Company, in.FirstAdmin)
}

// SellerInput creates a seller company together with its first administrator.
type SellerInput struct {
	Company    CompanyInput    `json:"company"`
	FirstAdmin FirstAdminInput `json:"firstAdmin"`
}

// CreateSellerWithAdmin onboards a seller company the same way as providers:
// draft company, first administrator with credentials, company-admin role and
// membership, all at once.
func (s *Company) CreateSellerWithAdmin(ctx context.Context, actor *auth.Principal, in SellerInput) (*ProvisionResult, error) {
	var v apperr.Validation
	return s.provision(ctx, actor, &v, model.KindSeller, in.Company, in.FirstAdmin)
}

func (s *Company) provision(ctx context.Context, actor *auth.Principal, v *apperr.Validation, kind model.CompanyKind, company CompanyInput, a FirstAdminInput) (*ProvisionResult, error) {
	c := s.newCompany(v, kind, company)
	login := validLogin(v, "firstAdmin.login", a.Login)
	now := c.CreatedAt
	u := &model.User{
		ID: uuid.NewString(), DisplayName: text(v, "firstAdmin.displayName", a.DisplayName, 1, 200),
		Email: email(v, "firstAdmin.email", a.Email), Login: &login, Status: model.UserActive,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	validatePassword(v, "firstAdmin.password", a.Password, a.PasswordConfirmation)
	if err := v.Err(); err != nil {
		return nil, err
	}
	hash, err := hashPassword(a.Password)
	if err != nil {
		return nil, err
	}
	u.PasswordHash = &hash
	m := &model.Membership{
		ID: uuid.NewString(), UserID: u.ID, CompanyID: c.ID, Status: model.MembershipActive,
		BranchAccess: model.AllBranches, Version: 1, CreatedAt: now, UpdatedAt: now,
	}

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
		if err := st.Roles().SetUserRoles(ctx, u.ID, []string{model.CompanyAdminRoleID}); err != nil {
			return err
		}
		if err := st.Memberships().Create(ctx, m); err != nil {
			return err
		}
		return s.audit(ctx, st, actor, "company."+string(provisionAction(kind)), "company", c.ID, &c.ID, "",
			map[string]any{"kind": c.Kind, "name": c.Name, "adminUserId": u.ID, "adminLogin": login})
	})
	if err != nil {
		return nil, err
	}
	return &ProvisionResult{Company: c, Admin: u, Membership: m}, nil
}

func provisionAction(kind model.CompanyKind) string {
	if kind == model.KindSeller {
		return "seller_provisioned"
	}
	return "provider_provisioned"
}

// canSee: members see their company; directory readers see all companies.
// Everyone else gets ErrNotFound so existence is not revealed.
func (s *Company) visible(ctx context.Context, actor *auth.Principal, id string) (*model.Company, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	if !actor.Can(model.PermPlatformDirectoryRead) {
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

func (s *Company) Get(ctx context.Context, actor *auth.Principal, id string) (*model.Company, error) {
	return s.visible(ctx, actor, id)
}

func (s *Company) List(ctx context.Context, f model.CompanyFilter) ([]model.Company, error) {
	var v apperr.Validation
	if f.Kind != "" && !f.Kind.Valid() {
		v.Add("kind", "unknown kind")
	}
	if f.Access != "" && f.Access != model.AccessDraft && f.Access != model.AccessActive && f.Access != model.AccessSuspended {
		v.Add("access", "unknown access state")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	return s.store.Companies().List(ctx, f)
}

// Update edits requisites: company editors who are members, or platform admins.
// The ID, kind, memberships and history stay the same.
func (s *Company) Update(ctx context.Context, actor *auth.Principal, id string, expected int64, in CompanyInput) (*model.Company, error) {
	c, err := s.visible(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if actor.Has(model.PermPlatformCompaniesAccess) {
		if err := actor.Allow(model.PermPlatformCompaniesAccess); err != nil {
			return nil, err
		}
	} else {
		member, err := s.isMember(ctx, s.store, actor.UserID, id)
		if err != nil {
			return nil, err
		}
		if !member {
			return nil, apperr.New(apperr.ErrForbidden, "permission_denied", "missing permission company.edit")
		}
		if err := actor.Allow(model.PermCompanyEdit); err != nil {
			return nil, err
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

var accessTransitions = map[string]struct{ from, to model.CompanyAccess }{
	ActionActivate: {model.AccessDraft, model.AccessActive},
	ActionSuspend:  {model.AccessActive, model.AccessSuspended},
	ActionRestore:  {model.AccessSuspended, model.AccessActive},
}

func (s *Company) SetAccess(ctx context.Context, actor *auth.Principal, id string, expected int64, action, why string) (*model.Company, error) {
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

// Profile is the public part of a company that other companies and modules
// may see: no contacts, registration or internal state beyond access.
type Profile struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	Kind    model.CompanyKind   `json:"kind"`
	Access  model.CompanyAccess `json:"access"`
	Country string              `json:"country"`
	Region  string              `json:"region"`
}

func profile(c *model.Company) Profile {
	return Profile{ID: c.ID, Name: c.Name, Kind: c.Kind, Access: c.Status, Country: c.Country, Region: c.Region}
}

// Directory lists active companies' public profiles, e.g. to find partners.
func (s *Company) Directory(ctx context.Context, f model.CompanyFilter) ([]Profile, error) {
	if f.Kind != "" && !f.Kind.Valid() {
		return nil, apperr.FieldError("kind", "unknown kind")
	}
	f.Access = model.AccessActive
	companies, err := s.store.Companies().List(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]Profile, len(companies))
	for i := range companies {
		out[i] = profile(&companies[i])
	}
	return out, nil
}

// CompanyProfile returns any company's public profile (for other modules).
func (s *Company) CompanyProfile(ctx context.Context, id string) (*Profile, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	c, err := s.store.Companies().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	p := profile(c)
	return &p, nil
}

// IsMember reports whether the user has an active membership in the company
// (for other modules, through their ports).
func (s *Company) IsMember(ctx context.Context, userID, companyID string) (bool, error) {
	if validID(userID, companyID) != nil {
		return false, nil
	}
	return s.isMember(ctx, s.store, userID, companyID)
}

// BranchOf reports whether the branch belongs to the company.
func (s *Company) BranchOf(ctx context.Context, companyID, branchID string) (bool, error) {
	if validID(companyID, branchID) != nil {
		return false, nil
	}
	n, err := s.store.Branches().CountInCompany(ctx, companyID, []string{branchID})
	return n == 1, err
}
