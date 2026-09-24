package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"justixauto/internal/modules/financing/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/money"
	"justixauto/internal/pkg/validate"
)

type ApplicationInput struct {
	RetailDealID      string                  `json:"retailDealId"`
	ProviderCompanyID string                  `json:"providerCompanyId"`
	ProgramID         *string                 `json:"programId"`
	ProgramVersion    *int                    `json:"programVersion"`
	CalculationInputs *model.CalculationInput `json:"calculationInputs"`
}

// eligibleSale: the seller's own active partner-finance sale.
func (s *Service) eligibleSale(ctx context.Context, companyID, dealID string) (*Sale, error) {
	sale, err := s.sales.Sale(ctx, companyID, dealID)
	if errors.Is(err, apperr.ErrNotFound) {
		return nil, apperr.FieldError("retailDealId", "not a sale of your company")
	}
	if err != nil {
		return nil, err
	}
	if sale.PaymentScheme != "partner-finance" || sale.Status != "reserved" {
		return nil, apperr.New(apperr.ErrConflict, "sale_not_eligible", "only active partner-finance sales can be financed")
	}
	return sale, nil
}

// chosenProgram returns the provider's published program version the seller chose.
func (s *Service) chosenProgram(ctx context.Context, r Repository, providerID string, programID *string, number *int) (*model.ProgramVersion, error) {
	if programID == nil {
		return nil, nil
	}
	if validate.IDs(*programID) != nil || number == nil {
		return nil, apperr.FieldError("programId", "choose a program and its version")
	}
	prog, err := r.Program(ctx, *programID)
	if err != nil || prog.ProviderCompanyID != providerID || prog.Status != "published" || prog.PublishedVersion == nil || *prog.PublishedVersion != *number {
		return nil, apperr.FieldError("programId", "not a published program version of this provider")
	}
	return r.ProgramVersion(ctx, prog.ID, *number)
}

// apply sets provider, program and calculation of a draft. Changing the
// provider clears the earlier program and calculation.
func (s *Service) apply(ctx context.Context, r Repository, a *model.Application, sale *Sale, in ApplicationInput) error {
	if err := s.provider(ctx, in.ProviderCompanyID, "providerCompanyId"); err != nil {
		return err
	}
	if in.ProviderCompanyID != a.ProviderCompanyID {
		a.ProgramID, a.ProgramVersion, a.Calculation, a.CalculationDigest = nil, nil, nil, ""
	}
	a.ProviderCompanyID = in.ProviderCompanyID
	pv, err := s.chosenProgram(ctx, r, in.ProviderCompanyID, in.ProgramID, in.ProgramVersion)
	if err != nil {
		return err
	}
	if pv == nil {
		a.ProgramID, a.ProgramVersion, a.Calculation, a.CalculationDigest = nil, nil, nil, ""
		return nil
	}
	a.ProgramID, a.ProgramVersion = &pv.ProgramID, &pv.Number
	a.Calculation, a.CalculationDigest = nil, ""
	if in.CalculationInputs != nil {
		terms, elig := pv.Decode()
		c, err := calculate(sale.Price, pv.ProgramID, pv.Number, pv.Currency, terms, elig, *in.CalculationInputs, s.clock())
		if err != nil {
			return err
		}
		a.Calculation, _ = json.Marshal(c)
		a.CalculationDigest = c.Digest()
	}
	return nil
}

// Create drafts an application; the provider does not see drafts.
func (s *Service) Create(ctx context.Context, p *auth.Principal, in ApplicationInput) (*model.Application, error) {
	sale, err := s.eligibleSale(ctx, p.CompanyID, in.RetailDealID)
	if err != nil {
		return nil, err
	}
	now := s.clock()
	a := &model.Application{
		ID: uuid.NewString(), SellerCompanyID: p.CompanyID, DealID: sale.ID, Status: "draft", Version: 1,
		CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now,
	}
	err = s.repo.InTx(ctx, func(r Repository) error {
		if err := s.apply(ctx, r, a, sale, in); err != nil {
			return err
		}
		if err := r.CreateApplication(ctx, a); err != nil {
			if errors.Is(err, apperr.ErrConflict) {
				return apperr.New(apperr.ErrConflict, "application_exists", "this sale already has an open financing application")
			}
			return err
		}
		return nil
	})
	return a, err
}

func (s *Service) load(ctx context.Context, r Repository, p *auth.Principal, id string, expected int64, side string) (*model.Application, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	a, err := r.Application(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	if a.Version != expected {
		return nil, apperr.ErrStale
	}
	if (side == "seller") != (a.SellerCompanyID == p.CompanyID) {
		return nil, apperr.New(apperr.ErrForbidden, "wrong_party", "only the "+side+" can do this")
	}
	return a, nil
}

// UpdateDraft edits provider, program and calculation of a draft.
func (s *Service) UpdateDraft(ctx context.Context, p *auth.Principal, id string, expected int64, in ApplicationInput) (*model.Application, error) {
	var a *model.Application
	err := s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if a, err = s.load(ctx, r, p, id, expected, "seller"); err != nil {
			return err
		}
		if a.Status != "draft" {
			return apperr.New(apperr.ErrConflict, "not_draft", "only drafts can be edited")
		}
		sale, err := s.eligibleSale(ctx, p.CompanyID, a.DealID)
		if err != nil {
			return err
		}
		if err := s.apply(ctx, r, a, sale, in); err != nil {
			return err
		}
		a.UpdatedAt = s.clock()
		return r.UpdateApplication(ctx, a, expected)
	})
	return a, err
}

func (s *Service) message(ctx context.Context, r Repository, p *auth.Principal, a *model.Application, kind string, requestID *string, note string, terms *int) error {
	return r.CreateMessage(ctx, &model.Message{
		ID: uuid.NewString(), ApplicationID: a.ID, Kind: kind, RequestID: requestID, Note: note,
		TermsVersion: terms, CompanyID: p.CompanyID, ActorUserID: p.UserID, CreatedAt: s.clock(),
	})
}

// Submit sends the application with an immutable snapshot of the sale and
// the exact calculation the seller reviewed (calculationDigest).
func (s *Service) Submit(ctx context.Context, p *auth.Principal, id string, expected int64, confirmation bool, dealRevision int64, calculationDigest string) (*model.Application, error) {
	if !confirmation {
		return nil, apperr.FieldError("confirmation", "confirm the data sent to the provider")
	}
	var a *model.Application
	err := s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if a, err = s.load(ctx, r, p, id, expected, "seller"); err != nil {
			return err
		}
		if a.Status != "draft" {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "the application was already submitted")
		}
		if a.Calculation == nil {
			return apperr.New(apperr.ErrConflict, "calculation_missing", "choose a program and calculate first")
		}
		if a.CalculationDigest != calculationDigest {
			return apperr.New(apperr.ErrConflict, "calculation_changed", "the calculation changed; review it and submit again")
		}
		sale, err := s.eligibleSale(ctx, p.CompanyID, a.DealID)
		if err != nil {
			return err
		}
		if sale.Revision != dealRevision {
			return apperr.New(apperr.ErrConflict, "sale_changed", "the sale changed; review it and submit again")
		}
		if _, err := s.chosenProgram(ctx, r, a.ProviderCompanyID, a.ProgramID, a.ProgramVersion); err != nil {
			return apperr.New(apperr.ErrConflict, "program_changed", "the program version is no longer published; recalculate")
		}
		if err := s.provider(ctx, a.ProviderCompanyID, "providerCompanyId"); err != nil {
			return err
		}
		now := s.clock()
		a.Snapshot, _ = json.Marshal(map[string]any{
			"dealId": sale.ID, "vehicleId": sale.VehicleID, "price": sale.Price,
			"vehicle": map[string]string{"vin": sale.VIN, "model": sale.Model}, "customer": map[string]string{"name": sale.CustomerName},
			"dealRevision": sale.Revision, "calculation": json.RawMessage(a.Calculation),
		})
		a.Status, a.SubmittedAt, a.UpdatedAt = "submitted", &now, now
		if err := r.UpdateApplication(ctx, a, expected); err != nil {
			return err
		}
		return s.message(ctx, r, p, a, "submitted", nil, "", nil)
	})
	return a, err
}

// ActInput carries the optional fields of a workflow step.
type ActInput struct {
	Note              string                  `json:"note"`
	Reason            string                  `json:"reason"`
	TermsVersion      *int                    `json:"termsVersion"`
	Confirmation      bool                    `json:"confirmation"`
	CalculationInputs *model.CalculationInput `json:"calculationInputs"`
}

// Act applies a workflow step:
//
//	take     provider submitted  → review
//	request  provider review     → needs-info  (note)
//	respond  seller   needs-info → review      (note, answers the open request)
//	terms    provider review     → terms       (note + recalculated, numbered terms version)
//	counter  seller   terms      → review      (note, on the current terms version)
//	agree    seller   terms      → agreed      (confirmation, on the current terms version)
//	decline  provider review     → declined    (reason; final in this release)
func (s *Service) Act(ctx context.Context, p *auth.Principal, id string, expected int64, action string, in ActInput) (*model.Application, error) {
	type step struct{ side, from, to string }
	steps := map[string]step{
		"take": {"provider", "submitted", "review"}, "request": {"provider", "review", "needs-info"},
		"respond": {"seller", "needs-info", "review"}, "terms": {"provider", "review", "terms"},
		"counter": {"seller", "terms", "review"}, "agree": {"seller", "terms", "agreed"},
		"decline": {"provider", "review", "declined"},
	}
	st, ok := steps[action]
	if !ok {
		return nil, apperr.ErrNotFound
	}
	var v apperr.Validation
	note := validateActInput(&v, action, in)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var a *model.Application
	err := s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if a, err = s.load(ctx, r, p, id, expected, st.side); err != nil {
			return err
		}
		if a.Status != st.from {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "cannot "+action+" an application that is "+a.Status)
		}
		requestID, termsNumber, err := s.stepEffects(ctx, r, p, a, action, in, note)
		if err != nil {
			return err
		}
		a.Status, a.UpdatedAt = st.to, s.clock()
		if err := r.UpdateApplication(ctx, a, expected); err != nil {
			return err
		}
		return s.message(ctx, r, p, a, action, requestID, note, termsNumber)
	})
	return a, err
}

// validateActInput checks the fields an Act action needs, beyond the
// from/to transition itself, preserving the original validation order.
func validateActInput(v *apperr.Validation, action string, in ActInput) string {
	note := ""
	switch action {
	case "request", "respond", "terms", "counter":
		note = validate.Text(v, "note", in.Note, 1, 2000)
	case "decline":
		note = validate.Reason(v, in.Reason)
	case "agree":
		if !in.Confirmation {
			v.Add("confirmation", "confirm the agreement with the customer")
		}
	}
	if (action == "counter" || action == "agree") && in.TermsVersion == nil {
		v.Add("termsVersion", "the terms version you reviewed")
	}
	if action == "terms" && in.CalculationInputs == nil {
		v.Add("calculationInputs", "required")
	}
	return note
}

// stepEffects performs the action-specific side effects of Act (finding the
// open request to answer, pinning the reviewed terms version, or
// recalculating and recording a new terms version) before the generic
// status transition and message are written.
func (s *Service) stepEffects(ctx context.Context, r Repository, p *auth.Principal, a *model.Application, action string, in ActInput, note string) (requestID *string, termsNumber *int, err error) {
	switch action {
	case "respond":
		ms, err := r.Messages(ctx, a.ID)
		if err != nil {
			return nil, nil, err
		}
		for i := len(ms) - 1; i >= 0 && requestID == nil; i-- {
			if ms[i].Kind == "request" {
				requestID = &ms[i].ID
			}
		}
	case "counter", "agree":
		if a.CurrentTermsVersion == nil || *in.TermsVersion != *a.CurrentTermsVersion {
			return nil, nil, apperr.New(apperr.ErrConflict, "terms_changed", "these are not the current terms; reload")
		}
		termsNumber = a.CurrentTermsVersion
	case "terms":
		n, err := s.recordTerms(ctx, r, p, a, *in.CalculationInputs, note)
		if err != nil {
			return nil, nil, err
		}
		a.CurrentTermsVersion, termsNumber = &n, &n
	}
	return requestID, termsNumber, nil
}

// recordTerms recalculates the application's program terms and stores the
// next numbered terms version.
func (s *Service) recordTerms(ctx context.Context, r Repository, p *auth.Principal, a *model.Application, inputs model.CalculationInput, note string) (int, error) {
	pv, err := r.ProgramVersion(ctx, *a.ProgramID, *a.ProgramVersion)
	if err != nil {
		return 0, err
	}
	var snap struct {
		Price money.Money `json:"price"`
	}
	_ = json.Unmarshal(a.Snapshot, &snap)
	terms, elig := pv.Decode()
	c, err := calculate(snap.Price, pv.ProgramID, pv.Number, pv.Currency, terms, elig, inputs, s.clock())
	if err != nil {
		return 0, err
	}
	raw, _ := json.Marshal(c)
	n, err := r.NextTermsVersionNumber(ctx, a.ID)
	if err != nil {
		return 0, err
	}
	if err := r.CreateTermsVersion(ctx, &model.TermsVersion{ApplicationID: a.ID, Number: n, Calculation: raw, Note: note, CreatedBy: p.UserID, CreatedAt: s.clock()}); err != nil {
		return 0, err
	}
	return n, nil
}

type ApplicationView struct {
	Application model.Application
	Terms       []model.TermsVersion
	History     []model.Message
}

func (s *Service) Get(ctx context.Context, p *auth.Principal, id string) (*ApplicationView, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	a, err := s.repo.Application(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	ts, err := s.repo.TermsVersions(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	ms, err := s.repo.Messages(ctx, a.ID)
	return &ApplicationView{Application: *a, Terms: ts, History: ms}, err
}

func (s *Service) List(ctx context.Context, p *auth.Principal, status string, limit, offset int) ([]model.Application, error) {
	return s.repo.Applications(ctx, p.CompanyID, status, limit, offset)
}
