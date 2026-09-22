// Package financing connects sellers with banks and MFOs: providers publish
// programs; sellers send a partner-finance sale with a calculation; the
// provider reviews, proposes terms and the seller agrees. Agreement is not a
// contract, signature, funding or permission to deliver.
package financing

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/database"
	"justixauto/internal/platform/money"
	"justixauto/internal/platform/validate"
)

const (
	PermRead     = "financing.read"
	PermPrograms = "financing.programs.manage"     // provider
	PermApply    = "financing.applications.manage" // seller
	PermAgree    = "financing.applications.agree"  // seller agrees to terms (sensitive)
	PermReview   = "financing.applications.review" // provider: take, request information
	PermDecide   = "financing.applications.decide" // provider: terms, decline (sensitive)
)

var Permissions = []auth.PermissionInfo{
	{Key: PermRead, Scope: "company", Assignable: true},
	{Key: PermPrograms, Scope: "company", Assignable: true},
	{Key: PermApply, Scope: "company", Assignable: true},
	{Key: PermAgree, Scope: "company", RequiresMFA: true, Assignable: true},
	{Key: PermReview, Scope: "company", Assignable: true},
	{Key: PermDecide, Scope: "company", RequiresMFA: true, Assignable: true},
}

// Sale is what financing may know about a retail sale (from retail).
type Sale struct {
	ID, VehicleID, PaymentScheme, Status string
	Price                                money.Money
	Revision                             int64
}

type Sales interface {
	Sale(ctx context.Context, companyID, dealID string) (*Sale, error)
}

type Company struct {
	ID, Name, Kind string
	Active         bool
}

type Directory interface {
	Company(ctx context.Context, id string) (*Company, error)
}

// ---- model ----

type Program struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	ProviderCompanyID string `gorm:"type:uuid"`
	Status            string
	PublishedVersion  *int
	StatusReason      string
	Version           int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (Program) TableName() string { return "financing.programs" }

type ProgramVersion struct {
	ProgramID     string `gorm:"primaryKey;type:uuid"`
	Number        int    `gorm:"primaryKey"`
	Name          string
	Currency      string
	Terms         []byte `gorm:"type:jsonb"`
	Eligibility   []byte `gorm:"type:jsonb"`
	PolicyID      string
	PolicyVersion int
	CreatedBy     string `gorm:"type:uuid"`
	CreatedAt     time.Time
}

func (ProgramVersion) TableName() string { return "financing.program_versions" }

func (v *ProgramVersion) decode() (ProgramTerms, Eligibility) {
	var t ProgramTerms
	var e Eligibility
	_ = json.Unmarshal(v.Terms, &t)
	_ = json.Unmarshal(v.Eligibility, &e)
	return t, e
}

type Application struct {
	ID                  string  `gorm:"primaryKey;type:uuid"`
	SellerCompanyID     string  `gorm:"type:uuid"`
	ProviderCompanyID   string  `gorm:"type:uuid"`
	DealID              string  `gorm:"type:uuid"`
	ProgramID           *string `gorm:"type:uuid"`
	ProgramVersion      *int
	Calculation         []byte `gorm:"type:jsonb"`
	CalculationDigest   string
	Status              string
	Snapshot            []byte `gorm:"type:jsonb"`
	CurrentTermsVersion *int
	Version             int64
	CreatedBy           string `gorm:"type:uuid"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
	SubmittedAt         *time.Time
}

func (Application) TableName() string { return "financing.applications" }

type TermsVersion struct {
	ApplicationID string `gorm:"primaryKey;type:uuid"`
	Number        int    `gorm:"primaryKey"`
	Calculation   []byte `gorm:"type:jsonb"`
	Note          string
	CreatedBy     string `gorm:"type:uuid"`
	CreatedAt     time.Time
}

func (TermsVersion) TableName() string { return "financing.terms_versions" }

type Message struct {
	ID            string `gorm:"primaryKey;type:uuid"`
	Seq           int64  `gorm:"->"`
	ApplicationID string `gorm:"type:uuid"`
	Kind          string
	RequestID     *string `gorm:"type:uuid"`
	Note          string
	TermsVersion  *int
	CompanyID     string `gorm:"type:uuid"`
	ActorUserID   string `gorm:"type:uuid"`
	CreatedAt     time.Time
}

func (Message) TableName() string { return "financing.messages" }

// ---- repository ----

type repo struct{ db *gorm.DB }

func (r *repo) tx(ctx context.Context, fn func(*repo) error) error {
	return database.Conn(ctx, r.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(&repo{tx}) })
}

func (r *repo) create(ctx context.Context, v any) error {
	return database.Translate(r.db.WithContext(ctx).Create(v).Error)
}

func (r *repo) program(ctx context.Context, id string) (*Program, error) {
	var p Program
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&p).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &p, nil
}

func (r *repo) programs(ctx context.Context, providerID string, publishedOnly bool, limit, offset int) ([]Program, error) {
	limit, offset = database.Page(limit, offset)
	q := r.db.WithContext(ctx).Order("updated_at DESC, id").Limit(limit).Offset(offset)
	if providerID != "" {
		q = q.Where("provider_company_id = ?", providerID)
	}
	if publishedOnly {
		q = q.Where("status = 'published'")
	}
	ps := []Program{}
	return ps, database.Translate(q.Find(&ps).Error)
}

func (r *repo) programVersion(ctx context.Context, id string, number int) (*ProgramVersion, error) {
	var v ProgramVersion
	if err := r.db.WithContext(ctx).Where("program_id = ? AND number = ?", id, number).Take(&v).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &v, nil
}

func (r *repo) programVersions(ctx context.Context, id string) ([]ProgramVersion, error) {
	vs := []ProgramVersion{}
	err := r.db.WithContext(ctx).Where("program_id = ?", id).Order("number").Find(&vs).Error
	return vs, database.Translate(err)
}

func (r *repo) nextNumber(ctx context.Context, model any, column, id string) (int, error) {
	var n int
	err := r.db.WithContext(ctx).Model(model).Where(column+" = ?", id).Select("coalesce(max(number), 0) + 1").Scan(&n).Error
	return n, database.Translate(err)
}

func (r *repo) updateProgram(ctx context.Context, p *Program, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Program{}, p.ID, expected, map[string]any{
		"status": p.Status, "published_version": p.PublishedVersion, "status_reason": p.StatusReason, "updated_at": p.UpdatedAt})
	if err == nil {
		p.Version = expected + 1
	}
	return err
}

const visible = "(seller_company_id = ? OR (provider_company_id = ? AND status <> 'draft'))"

func (r *repo) application(ctx context.Context, companyID, id string) (*Application, error) {
	var a Application
	if err := r.db.WithContext(ctx).Where("id = ? AND "+visible, id, companyID, companyID).Take(&a).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &a, nil
}

func (r *repo) applications(ctx context.Context, companyID, status string, limit, offset int) ([]Application, error) {
	limit, offset = database.Page(limit, offset)
	q := r.db.WithContext(ctx).Where(visible, companyID, companyID).Order("updated_at DESC, id").Limit(limit).Offset(offset)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	as := []Application{}
	return as, database.Translate(q.Find(&as).Error)
}

func (r *repo) updateApplication(ctx context.Context, a *Application, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Application{}, a.ID, expected, map[string]any{
		"provider_company_id": a.ProviderCompanyID, "program_id": a.ProgramID, "program_version": a.ProgramVersion,
		"calculation": a.Calculation, "calculation_digest": a.CalculationDigest, "status": a.Status, "snapshot": a.Snapshot,
		"current_terms_version": a.CurrentTermsVersion, "updated_at": a.UpdatedAt, "submitted_at": a.SubmittedAt})
	if err == nil {
		a.Version = expected + 1
	}
	return err
}

func (r *repo) termsVersions(ctx context.Context, id string) ([]TermsVersion, error) {
	ts := []TermsVersion{}
	err := r.db.WithContext(ctx).Where("application_id = ?", id).Order("number").Find(&ts).Error
	return ts, database.Translate(err)
}

func (r *repo) messages(ctx context.Context, id string) ([]Message, error) {
	ms := []Message{}
	err := r.db.WithContext(ctx).Where("application_id = ?", id).Order("seq").Find(&ms).Error
	return ms, database.Translate(err)
}

// ---- service ----

type Service struct {
	r         *repo
	sales     Sales
	directory Directory
	files     Files
	now       func() time.Time
}

func (s *Service) clock() time.Time { return s.now().UTC() }

func (s *Service) provider(ctx context.Context, id, field string) error {
	if validate.IDs(id) != nil {
		return apperr.FieldError(field, "must be a valid ID")
	}
	c, err := s.directory.Company(ctx, id)
	if errors.Is(err, apperr.ErrNotFound) || (err == nil && ((c.Kind != "bank" && c.Kind != "mfo") || !c.Active)) {
		return apperr.FieldError(field, "not an active bank or MFO")
	}
	return err
}

// ProgramInput is one version of a program.
type ProgramInput struct {
	Name                     string       `json:"name"`
	Currency                 string       `json:"currency"`
	Terms                    ProgramTerms `json:"terms"`
	Eligibility              Eligibility  `json:"eligibility"`
	CalculationPolicyID      string       `json:"calculationPolicyId"`
	CalculationPolicyVersion int          `json:"calculationPolicyVersion"`
}

func (s *Service) newProgramVersion(p *auth.Principal, programID string, number int, in ProgramInput) (*ProgramVersion, error) {
	var v apperr.Validation
	name := validate.Text(&v, "name", in.Name, 1, 200)
	if _, ok := (money.Money{AmountMinor: "0", Currency: in.Currency}).Parse(); !ok {
		v.Add("currency", "a three-letter currency code")
	}
	if in.CalculationPolicyID != PolicyFixedMarkup || in.CalculationPolicyVersion != PolicyFixedMarkupVersion {
		v.Add("calculationPolicyId", "unsupported policy; available: fixed-markup version 1")
	}
	in.Terms.validate(&v)
	for field, a := range map[string]string{"eligibility.minPriceMinor": in.Eligibility.MinPriceMinor, "eligibility.maxPriceMinor": in.Eligibility.MaxPriceMinor} {
		if _, ok := (money.Money{AmountMinor: a, Currency: "USD"}).Parse(); a != "" && !ok {
			v.Add(field, "an amount in minor units")
		}
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	terms, _ := json.Marshal(in.Terms)
	elig, _ := json.Marshal(in.Eligibility)
	return &ProgramVersion{ProgramID: programID, Number: number, Name: name, Currency: in.Currency, Terms: terms, Eligibility: elig,
		PolicyID: in.CalculationPolicyID, PolicyVersion: in.CalculationPolicyVersion, CreatedBy: p.UserID, CreatedAt: s.clock()}, nil
}

// CreateProgram drafts a program for the provider (bank or MFO).
func (s *Service) CreateProgram(ctx context.Context, p *auth.Principal, in ProgramInput) (*Program, error) {
	if err := s.provider(ctx, p.CompanyID, "companyId"); err != nil {
		return nil, apperr.New(apperr.ErrForbidden, "not_a_provider", "only active banks and MFOs publish programs")
	}
	now := s.clock()
	prog := &Program{ID: uuid.NewString(), ProviderCompanyID: p.CompanyID, Status: "draft", Version: 1, CreatedAt: now, UpdatedAt: now}
	v, err := s.newProgramVersion(p, prog.ID, 1, in)
	if err != nil {
		return nil, err
	}
	return prog, s.r.tx(ctx, func(r *repo) error {
		if err := r.create(ctx, prog); err != nil {
			return err
		}
		return r.create(ctx, v)
	})
}

func (s *Service) ownProgram(ctx context.Context, r *repo, p *auth.Principal, id string, expected int64) (*Program, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	prog, err := r.program(ctx, id)
	if err != nil || prog.ProviderCompanyID != p.CompanyID {
		return nil, apperr.ErrNotFound
	}
	if prog.Version != expected {
		return nil, apperr.ErrStale
	}
	if prog.Status == "withdrawn" {
		return nil, apperr.New(apperr.ErrConflict, "program_withdrawn", "the program was withdrawn")
	}
	return prog, nil
}

// AddProgramVersion stores a new version; the published one stays in effect.
func (s *Service) AddProgramVersion(ctx context.Context, p *auth.Principal, id string, expected int64, in ProgramInput) (*Program, error) {
	var prog *Program
	err := s.r.tx(ctx, func(r *repo) error {
		var err error
		if prog, err = s.ownProgram(ctx, r, p, id, expected); err != nil {
			return err
		}
		prog.UpdatedAt = s.clock()
		if err := r.updateProgram(ctx, prog, expected); err != nil {
			return err
		}
		n, err := r.nextNumber(ctx, &ProgramVersion{}, "program_id", prog.ID)
		if err != nil {
			return err
		}
		v, err := s.newProgramVersion(p, prog.ID, n, in)
		if err != nil {
			return err
		}
		return r.create(ctx, v)
	})
	return prog, err
}

// PublishProgram makes one version available to sellers; applications keep
// the version they were calculated with.
func (s *Service) PublishProgram(ctx context.Context, p *auth.Principal, id string, expected int64, number int) (*Program, error) {
	var prog *Program
	err := s.r.tx(ctx, func(r *repo) error {
		var err error
		if prog, err = s.ownProgram(ctx, r, p, id, expected); err != nil {
			return err
		}
		if _, err := r.programVersion(ctx, prog.ID, number); err != nil {
			return apperr.FieldError("programVersion", "not a version of this program")
		}
		prog.Status, prog.PublishedVersion, prog.UpdatedAt = "published", &number, s.clock()
		return r.updateProgram(ctx, prog, expected)
	})
	return prog, err
}

func (s *Service) WithdrawProgram(ctx context.Context, p *auth.Principal, id string, expected int64, why string) (*Program, error) {
	var v apperr.Validation
	why = validate.Reason(&v, why)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var prog *Program
	err := s.r.tx(ctx, func(r *repo) error {
		var err error
		if prog, err = s.ownProgram(ctx, r, p, id, expected); err != nil {
			return err
		}
		prog.Status, prog.StatusReason, prog.UpdatedAt = "withdrawn", why, s.clock()
		return r.updateProgram(ctx, prog, expected)
	})
	return prog, err
}

type ProgramView struct {
	Program  Program
	Provider Company
	Versions []ProgramVersion // provider: all; sellers: the published one
}

func (s *Service) programView(ctx context.Context, p *auth.Principal, prog *Program) (*ProgramView, error) {
	c, err := s.directory.Company(ctx, prog.ProviderCompanyID)
	if err != nil {
		return nil, err
	}
	v := &ProgramView{Program: *prog, Provider: *c}
	if prog.ProviderCompanyID == p.CompanyID {
		v.Versions, err = s.r.programVersions(ctx, prog.ID)
		return v, err
	}
	pv, err := s.r.programVersion(ctx, prog.ID, *prog.PublishedVersion)
	if err != nil {
		return nil, err
	}
	v.Versions = []ProgramVersion{*pv}
	return v, nil
}

// Programs lists the provider's own programs, or (for sellers) the published
// programs of active banks and MFOs, optionally of one provider.
func (s *Service) Programs(ctx context.Context, p *auth.Principal, providerID string, limit, offset int) ([]ProgramView, error) {
	own := providerID == "" && s.provider(ctx, p.CompanyID, "x") == nil
	var ps []Program
	var err error
	if own {
		ps, err = s.r.programs(ctx, p.CompanyID, false, limit, offset)
	} else {
		ps, err = s.r.programs(ctx, providerID, true, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	out := []ProgramView{}
	for i := range ps {
		if !own {
			if c, err := s.directory.Company(ctx, ps[i].ProviderCompanyID); err != nil || !c.Active {
				continue
			}
		}
		v, err := s.programView(ctx, p, &ps[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}

func (s *Service) Program(ctx context.Context, p *auth.Principal, id string) (*ProgramView, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	prog, err := s.r.program(ctx, id)
	if err != nil {
		return nil, err
	}
	if prog.ProviderCompanyID != p.CompanyID && prog.Status != "published" {
		return nil, apperr.ErrNotFound
	}
	return s.programView(ctx, p, prog)
}
