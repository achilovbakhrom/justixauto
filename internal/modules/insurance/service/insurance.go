// Package service holds insurance's business rules. It imports model and
// internal/pkg only; it must never import echo, gorm, repository or handler.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/modules/insurance/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/validate"
)

// Service reviews sellers' insurance applications on their sales.
type Service struct {
	repo      Repository
	sales     Sales
	directory Directory
	now       func() time.Time
}

// New builds the insurance service.
func New(repo Repository, sales Sales, directory Directory, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, sales: sales, directory: directory, now: now}
}

func (s *Service) clock() time.Time { return s.now().UTC() }

func (s *Service) insurer(ctx context.Context, id string) error {
	if validate.IDs(id) != nil {
		return apperr.FieldError("insurerCompanyId", "must be a valid ID")
	}
	c, err := s.directory.Company(ctx, id)
	if errors.Is(err, apperr.ErrNotFound) || (err == nil && (c.Kind != "insurance" || !c.Active)) {
		return apperr.FieldError("insurerCompanyId", "not an active insurance company")
	}
	return err
}

// eligibleSale: the seller's own, undelivered own-installment sale.
func (s *Service) eligibleSale(ctx context.Context, companyID, dealID string) (*Sale, error) {
	sale, err := s.sales.Sale(ctx, companyID, dealID)
	if errors.Is(err, apperr.ErrNotFound) {
		return nil, apperr.FieldError("retailDealId", "not a sale of your company")
	}
	if err != nil {
		return nil, err
	}
	if sale.PaymentScheme != "own-installment" || sale.Status != "reserved" {
		return nil, apperr.New(apperr.ErrConflict, "sale_not_eligible", "only active own-installment sales can be insured")
	}
	return sale, nil
}

// CreateInput is a new insurance application's data.
type CreateInput struct {
	RetailDealID     string `json:"retailDealId"`
	InsurerCompanyID string `json:"insurerCompanyId"`
	Note             string `json:"note"`
}

// Create drafts the application; the insurer does not see drafts.
func (s *Service) Create(ctx context.Context, p *auth.Principal, in CreateInput) (*model.Application, error) {
	var v apperr.Validation
	note := validate.Text(&v, "note", in.Note, 0, 2000)
	if err := v.Err(); err != nil {
		return nil, err
	}
	if _, err := s.eligibleSale(ctx, p.CompanyID, in.RetailDealID); err != nil {
		return nil, err
	}
	if err := s.insurer(ctx, in.InsurerCompanyID); err != nil {
		return nil, err
	}
	now := s.clock()
	a := &model.Application{
		ID: uuid.NewString(), SellerCompanyID: p.CompanyID, InsurerCompanyID: in.InsurerCompanyID, DealID: in.RetailDealID,
		Status: "draft", Note: note, Version: 1, CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.Create(ctx, a); err != nil {
		if errors.Is(err, apperr.ErrConflict) {
			return nil, apperr.New(apperr.ErrConflict, "application_exists", "this sale already has an insurance application")
		}
		return nil, err
	}
	return a, nil
}

func (s *Service) load(ctx context.Context, r Repository, p *auth.Principal, id string, expected int64, side string) (*model.Application, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	a, err := r.Get(ctx, p.CompanyID, id)
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

// UpdateDraft changes the insurer or note while the application is a draft.
func (s *Service) UpdateDraft(ctx context.Context, p *auth.Principal, id string, expected int64, insurerID, note string) (*model.Application, error) {
	var v apperr.Validation
	note = validate.Text(&v, "note", note, 0, 2000)
	if err := v.Err(); err != nil {
		return nil, err
	}
	if err := s.insurer(ctx, insurerID); err != nil {
		return nil, err
	}
	var a *model.Application
	err := s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if a, err = s.load(ctx, r, p, id, expected, "seller"); err != nil {
			return err
		}
		if a.Status != "draft" {
			return apperr.New(apperr.ErrConflict, "not_draft", "only drafts can be edited")
		}
		a.InsurerCompanyID, a.Note, a.UpdatedAt = insurerID, note, s.clock()
		return r.Update(ctx, a, expected)
	})
	return a, err
}

// Submit freezes the sale facts and sends the application to the insurer.
// dealRevision must match the sale the seller reviewed.
func (s *Service) Submit(ctx context.Context, p *auth.Principal, id string, expected int64, confirmation bool, dealRevision int64) (*model.Application, error) {
	if !confirmation {
		return nil, apperr.FieldError("confirmation", "confirm the data sent to the insurer")
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
		sale, err := s.eligibleSale(ctx, p.CompanyID, a.DealID)
		if err != nil {
			return err
		}
		if sale.Revision != dealRevision {
			return apperr.New(apperr.ErrConflict, "sale_changed", "the sale changed; review it and submit again")
		}
		if err := s.insurer(ctx, a.InsurerCompanyID); err != nil {
			return err
		}
		now := s.clock()
		a.Snapshot, _ = json.Marshal(map[string]any{
			"dealId": sale.ID, "vehicleId": sale.VehicleID, "price": sale.Price,
			"vehicle": map[string]string{"vin": sale.VIN, "model": sale.Model}, "customer": map[string]string{"name": sale.CustomerName},
			"paymentScheme": sale.PaymentScheme, "dealRevision": sale.Revision, "note": a.Note,
		})
		a.Status, a.SubmittedAt, a.UpdatedAt = "submitted", &now, now
		if err := r.Update(ctx, a, expected); err != nil {
			return err
		}
		return s.message(ctx, r, p, a, "submitted", nil, a.Note)
	})
	return a, err
}

func (s *Service) message(ctx context.Context, r Repository, p *auth.Principal, a *model.Application, kind string, requestID *string, note string) error {
	return r.AddMessage(ctx, &model.Message{
		ID: uuid.NewString(), ApplicationID: a.ID, Kind: kind, RequestID: requestID, Note: note,
		CompanyID: p.CompanyID, ActorUserID: p.UserID, CreatedAt: s.clock(),
	})
}

// Act applies an insurer or seller step:
//
//	take     insurer  submitted  → review
//	request  insurer  review     → needs-info   (note required)
//	respond  seller   needs-info → review       (note required, answers the open request)
//	approve  insurer  review     → approved     (note required)
//	decline  insurer  review     → declined     (note required)
func (s *Service) Act(ctx context.Context, p *auth.Principal, id string, expected int64, action, note string) (*model.Application, error) {
	type step struct {
		side, from, to, kind string
		note                 bool
	}
	steps := map[string]step{
		"take":    {"insurer", "submitted", "review", "taken", false},
		"request": {"insurer", "review", "needs-info", "request", true},
		"respond": {"seller", "needs-info", "review", "response", true},
		"approve": {"insurer", "review", "approved", "approved", true},
		"decline": {"insurer", "review", "declined", "declined", true},
	}
	st, ok := steps[action]
	if !ok {
		return nil, apperr.ErrNotFound
	}
	var v apperr.Validation
	if st.note {
		note = validate.Text(&v, "note", note, 1, 2000)
	}
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
		var requestID *string
		if action == "respond" {
			ms, err := r.Messages(ctx, a.ID)
			if err != nil {
				return err
			}
			for i := len(ms) - 1; i >= 0; i-- {
				if ms[i].Kind == "request" {
					requestID = &ms[i].ID
					break
				}
			}
		}
		now := s.clock()
		a.Status, a.UpdatedAt = st.to, now
		if st.to == "approved" || st.to == "declined" {
			a.DecidedAt = &now
		}
		if err := r.Update(ctx, a, expected); err != nil {
			return err
		}
		return s.message(ctx, r, p, a, st.kind, requestID, note)
	})
	return a, err
}

func (s *Service) Get(ctx context.Context, p *auth.Principal, id string) (*model.Application, []model.Message, error) {
	if err := validate.IDs(id); err != nil {
		return nil, nil, err
	}
	a, err := s.repo.Get(ctx, p.CompanyID, id)
	if err != nil {
		return nil, nil, err
	}
	ms, err := s.repo.Messages(ctx, a.ID)
	return a, ms, err
}

func (s *Service) List(ctx context.Context, p *auth.Principal, status string, limit, offset int) ([]model.Application, error) {
	return s.repo.List(ctx, p.CompanyID, status, limit, offset)
}

// Approved reports whether the insurer approved the seller's sale (for retail).
func (s *Service) Approved(ctx context.Context, companyID, dealID string) (bool, error) {
	a, err := s.repo.ForDeal(ctx, companyID, dealID)
	if errors.Is(err, apperr.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return a.Status == "approved", nil
}
