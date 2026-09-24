package service

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"justixauto/internal/modules/financing/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/money"
	"justixauto/internal/pkg/validate"
)

// ProgramInput is one version of a program.
type ProgramInput struct {
	Name                     string             `json:"name"`
	Currency                 string             `json:"currency"`
	Terms                    model.ProgramTerms `json:"terms"`
	Eligibility              model.Eligibility  `json:"eligibility"`
	CalculationPolicyID      string             `json:"calculationPolicyId"`
	CalculationPolicyVersion int                `json:"calculationPolicyVersion"`
}

func (s *Service) newProgramVersion(p *auth.Principal, programID string, number int, in ProgramInput) (*model.ProgramVersion, error) {
	var v apperr.Validation
	name := validate.Text(&v, "name", in.Name, 1, 200)
	if _, ok := (money.Money{AmountMinor: "0", Currency: in.Currency}).Parse(); !ok {
		v.Add("currency", "a three-letter currency code")
	}
	if in.CalculationPolicyID != PolicyFixedMarkup || in.CalculationPolicyVersion != PolicyFixedMarkupVersion {
		v.Add("calculationPolicyId", "unsupported policy; available: fixed-markup version 1")
	}
	in.Terms.Validate(&v)
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
	return &model.ProgramVersion{
		ProgramID: programID, Number: number, Name: name, Currency: in.Currency, Terms: terms, Eligibility: elig,
		PolicyID: in.CalculationPolicyID, PolicyVersion: in.CalculationPolicyVersion, CreatedBy: p.UserID, CreatedAt: s.clock(),
	}, nil
}

// CreateProgram drafts a program for the provider (bank or MFO).
func (s *Service) CreateProgram(ctx context.Context, p *auth.Principal, in ProgramInput) (*model.Program, error) {
	if err := s.provider(ctx, p.CompanyID, "companyId"); err != nil {
		return nil, apperr.New(apperr.ErrForbidden, "not_a_provider", "only active banks and MFOs publish programs")
	}
	now := s.clock()
	prog := &model.Program{ID: uuid.NewString(), ProviderCompanyID: p.CompanyID, Status: "draft", Version: 1, CreatedAt: now, UpdatedAt: now}
	v, err := s.newProgramVersion(p, prog.ID, 1, in)
	if err != nil {
		return nil, err
	}
	return prog, s.repo.InTx(ctx, func(r Repository) error {
		if err := r.CreateProgram(ctx, prog); err != nil {
			return err
		}
		return r.CreateProgramVersion(ctx, v)
	})
}

func (s *Service) ownProgram(ctx context.Context, r Repository, p *auth.Principal, id string, expected int64) (*model.Program, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	prog, err := r.Program(ctx, id)
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
func (s *Service) AddProgramVersion(ctx context.Context, p *auth.Principal, id string, expected int64, in ProgramInput) (*model.Program, error) {
	var prog *model.Program
	err := s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if prog, err = s.ownProgram(ctx, r, p, id, expected); err != nil {
			return err
		}
		prog.UpdatedAt = s.clock()
		if err := r.UpdateProgram(ctx, prog, expected); err != nil {
			return err
		}
		n, err := r.NextProgramVersionNumber(ctx, prog.ID)
		if err != nil {
			return err
		}
		v, err := s.newProgramVersion(p, prog.ID, n, in)
		if err != nil {
			return err
		}
		return r.CreateProgramVersion(ctx, v)
	})
	return prog, err
}

// PublishProgram makes one version available to sellers; applications keep
// the version they were calculated with.
func (s *Service) PublishProgram(ctx context.Context, p *auth.Principal, id string, expected int64, number int) (*model.Program, error) {
	var prog *model.Program
	err := s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if prog, err = s.ownProgram(ctx, r, p, id, expected); err != nil {
			return err
		}
		if _, err := r.ProgramVersion(ctx, prog.ID, number); err != nil {
			return apperr.FieldError("programVersion", "not a version of this program")
		}
		prog.Status, prog.PublishedVersion, prog.UpdatedAt = "published", &number, s.clock()
		return r.UpdateProgram(ctx, prog, expected)
	})
	return prog, err
}

func (s *Service) WithdrawProgram(ctx context.Context, p *auth.Principal, id string, expected int64, why string) (*model.Program, error) {
	var v apperr.Validation
	why = validate.Reason(&v, why)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var prog *model.Program
	err := s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if prog, err = s.ownProgram(ctx, r, p, id, expected); err != nil {
			return err
		}
		prog.Status, prog.StatusReason, prog.UpdatedAt = "withdrawn", why, s.clock()
		return r.UpdateProgram(ctx, prog, expected)
	})
	return prog, err
}

type ProgramView struct {
	Program  model.Program
	Provider Company
	Versions []model.ProgramVersion // provider: all; sellers: the published one
}

func (s *Service) programView(ctx context.Context, p *auth.Principal, prog *model.Program) (*ProgramView, error) {
	c, err := s.directory.Company(ctx, prog.ProviderCompanyID)
	if err != nil {
		return nil, err
	}
	v := &ProgramView{Program: *prog, Provider: *c}
	if prog.ProviderCompanyID == p.CompanyID {
		v.Versions, err = s.repo.ProgramVersions(ctx, prog.ID)
		return v, err
	}
	pv, err := s.repo.ProgramVersion(ctx, prog.ID, *prog.PublishedVersion)
	if err != nil {
		return nil, err
	}
	v.Versions = []model.ProgramVersion{*pv}
	return v, nil
}

// Programs lists the provider's own programs, or (for sellers) the published
// programs of active banks and MFOs, optionally of one provider.
func (s *Service) Programs(ctx context.Context, p *auth.Principal, providerID string, limit, offset int) ([]ProgramView, error) {
	own := providerID == "" && s.provider(ctx, p.CompanyID, "x") == nil
	var ps []model.Program
	var err error
	if own {
		ps, err = s.repo.Programs(ctx, p.CompanyID, false, limit, offset)
	} else {
		ps, err = s.repo.Programs(ctx, providerID, true, limit, offset)
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
	prog, err := s.repo.Program(ctx, id)
	if err != nil {
		return nil, err
	}
	if prog.ProviderCompanyID != p.CompanyID && prog.Status != "published" {
		return nil, apperr.ErrNotFound
	}
	return s.programView(ctx, p, prog)
}
