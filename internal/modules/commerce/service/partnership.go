package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"justixauto/internal/modules/commerce/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/validate"
)

// Partnership manages trading partnerships between companies.
type Partnership struct{ Deps }

// PartnershipView is a partnership with the other company's name.
type PartnershipView struct {
	Partnership  model.Partnership
	Counterparty Company
}

func (s *Partnership) view(ctx context.Context, companyID string, p *model.Partnership) (*PartnershipView, error) {
	c, err := s.directory.Company(ctx, p.Counterparty(companyID))
	if err != nil {
		return nil, err
	}
	return &PartnershipView{Partnership: *p, Counterparty: *c}, nil
}

// Request asks another seller company for a partnership. Both companies must
// be active sellers and may have only one open partnership with each other.
func (s *Partnership) Request(ctx context.Context, p *auth.Principal, counterpartyID string) (*PartnershipView, error) {
	if validate.IDs(counterpartyID) != nil {
		return nil, apperr.FieldError("counterpartyCompanyId", "must be a valid ID")
	}
	if counterpartyID == p.CompanyID {
		return nil, apperr.FieldError("counterpartyCompanyId", "cannot partner with your own company")
	}
	if _, err := s.tradingCompany(ctx, p.CompanyID, "companyId"); err != nil {
		return nil, err
	}
	other, err := s.tradingCompany(ctx, counterpartyID, "counterpartyCompanyId")
	if err != nil {
		return nil, err
	}
	now := s.clock()
	ps := &model.Partnership{
		ID: uuid.NewString(), RequesterCompanyID: p.CompanyID, RecipientCompanyID: counterpartyID,
		Status: model.Requested, RequestedBy: p.UserID, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	err = s.store.InTx(ctx, func(st Store) error {
		if err := st.Partnerships().Create(ctx, ps); err != nil {
			if errors.Is(err, apperr.ErrConflict) {
				return apperr.New(apperr.ErrConflict, "partnership_exists", "a partnership with this company is already requested or active")
			}
			return err
		}
		return s.event(ctx, st, p, "partnership.requested", "partnership", ps.ID, "", map[string]any{"counterpartyCompanyId": counterpartyID})
	})
	if err != nil {
		return nil, err
	}
	return &PartnershipView{Partnership: *ps, Counterparty: *other}, nil
}

func (s *Partnership) Get(ctx context.Context, p *auth.Principal, id string) (*PartnershipView, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	ps, err := s.store.Partnerships().Get(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, p.CompanyID, ps)
}

func (s *Partnership) List(ctx context.Context, p *auth.Principal, f model.PartnershipFilter) ([]PartnershipView, error) {
	switch f.Status {
	case "", model.Requested, model.Active, model.Declined, model.Withdrawn, model.Ended:
	default:
		return nil, apperr.FieldError("status", "unknown status")
	}
	f.CompanyID = p.CompanyID
	ps, err := s.store.Partnerships().List(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]PartnershipView, 0, len(ps))
	for i := range ps {
		v, err := s.view(ctx, p.CompanyID, &ps[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}

// Decide applies accept, decline, withdraw or end. Ending keeps all earlier
// contractual records; it only stops new offers and orders.
func (s *Partnership) Decide(ctx context.Context, p *auth.Principal, id string, expected int64, action, why string) (*PartnershipView, error) {
	t, ok := model.Transitions[action]
	if !ok {
		return nil, apperr.ErrNotFound
	}
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	var v apperr.Validation
	if t.Reason {
		why = validate.Reason(&v, why)
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *model.Partnership
	err := s.store.InTx(ctx, func(st Store) error {
		ps, err := st.Partnerships().Get(ctx, p.CompanyID, id)
		if err != nil {
			return err
		}
		if ps.Version != expected {
			return apperr.ErrStale
		}
		isRecipient := ps.RecipientCompanyID == p.CompanyID
		if !t.Any && t.Recipient != isRecipient {
			side := "the requesting company"
			if t.Recipient {
				side = "the invited company"
			}
			return apperr.New(apperr.ErrForbidden, "wrong_party", "only "+side+" can "+action)
		}
		if ps.Status != t.From {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "cannot "+action+" a partnership that is "+string(ps.Status))
		}
		now := s.clock()
		ps.Status, ps.StatusReason, ps.UpdatedAt = t.To, why, now
		if t.To == model.Active {
			ps.ActivatedAt = &now
		} else {
			ps.ClosedAt = &now
		}
		if err := st.Partnerships().Update(ctx, ps, expected); err != nil {
			return err
		}
		result = ps
		return s.event(ctx, st, p, "partnership."+string(t.To), "partnership", ps.ID, why, nil)
	})
	if err != nil {
		return nil, err
	}
	return s.view(ctx, p.CompanyID, result)
}
