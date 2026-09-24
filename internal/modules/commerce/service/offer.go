package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"justixauto/internal/modules/commerce/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/validate"
)

// Offer manages supplier offers: draft terms, versions, publishing.
type Offer struct{ Deps }

type OfferInput struct {
	Terms    model.Terms    `json:"terms"`
	Audience model.Audience `json:"audience"`
}

// OfferView is an offer as one company may see it. Suppliers get every
// version; buyers only the published one.
type OfferView struct {
	Offer     model.Offer
	Supplier  Company
	Published *model.OfferVersion
	Versions  []model.OfferVersion
	Own       bool
}

func (s *Offer) newVersion(ctx context.Context, p *auth.Principal, offerID string, in OfferInput) (*model.OfferVersion, error) {
	var v apperr.Validation
	terms := s.validateTerms(ctx, &v, in.Terms)
	audience := validateAudience(&v, p.CompanyID, in.Audience)
	if err := v.Err(); err != nil {
		return nil, err
	}
	rawTerms, _ := json.Marshal(terms)
	rawIDs, _ := json.Marshal(audience.PartnerCompanyIDs)
	return &model.OfferVersion{
		ID: uuid.NewString(), OfferID: offerID, Number: 1, Terms: rawTerms, AudienceMode: audience.Mode,
		AudienceIDs: rawIDs, CreatedBy: p.UserID, CreatedAt: s.clock(),
	}, nil
}

// Create starts a draft offer with version 1.
func (s *Offer) Create(ctx context.Context, p *auth.Principal, in OfferInput) (*OfferView, error) {
	supplier, err := s.tradingCompany(ctx, p.CompanyID, "companyId")
	if err != nil {
		return nil, err
	}
	now := s.clock()
	o := &model.Offer{ID: uuid.NewString(), SupplierCompanyID: p.CompanyID, Status: model.OfferDraft, Version: 1, CreatedAt: now, UpdatedAt: now}
	v, err := s.newVersion(ctx, p, o.ID, in)
	if err != nil {
		return nil, err
	}
	err = s.store.InTx(ctx, func(st Store) error {
		if err := st.Offers().Create(ctx, o, v); err != nil {
			return err
		}
		return s.event(ctx, st, p, "offer.created", "offer", o.ID, "", map[string]any{"versionId": v.ID})
	})
	if err != nil {
		return nil, err
	}
	return &OfferView{Offer: *o, Supplier: *supplier, Versions: []model.OfferVersion{*v}, Own: true}, nil
}

func (s *Offer) ownOpen(ctx context.Context, st Store, p *auth.Principal, id string, expected int64) (*model.Offer, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	o, err := st.Offers().GetOwn(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	if o.Version != expected {
		return nil, apperr.ErrStale
	}
	if o.Status == model.OfferWithdrawn {
		return nil, apperr.New(apperr.ErrConflict, "offer_withdrawn", "the offer was withdrawn; create a new one")
	}
	return o, nil
}

// AddVersion stores a new immutable version. It is not visible to partners
// until it is published; the currently published version stays in effect.
func (s *Offer) AddVersion(ctx context.Context, p *auth.Principal, id string, expected int64, in OfferInput) (*OfferView, error) {
	err := s.store.InTx(ctx, func(st Store) error {
		o, err := s.ownOpen(ctx, st, p, id, expected)
		if err != nil {
			return err
		}
		v, err := s.newVersion(ctx, p, o.ID, in)
		if err != nil {
			return err
		}
		// Bump the offer first: concurrent edits get 412, and the row lock
		// serializes version numbering.
		o.UpdatedAt = s.clock()
		if err := st.Offers().Update(ctx, o, expected); err != nil {
			return err
		}
		if err := st.Offers().AddVersion(ctx, v); err != nil {
			return err
		}
		return s.event(ctx, st, p, "offer.version_added", "offer", o.ID, "", map[string]any{"versionId": v.ID, "number": v.Number})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}

// Publish makes one exact version visible to its audience. A selected
// audience must consist of active partners. Publishing reserves no stock.
func (s *Offer) Publish(ctx context.Context, p *auth.Principal, id string, expected int64, versionID string) (*OfferView, error) {
	if uuid.Validate(versionID) != nil {
		return nil, apperr.FieldError("offerVersionId", "must be a valid ID")
	}
	if _, err := s.tradingCompany(ctx, p.CompanyID, "companyId"); err != nil {
		return nil, err
	}
	err := s.store.InTx(ctx, func(st Store) error {
		o, err := s.ownOpen(ctx, st, p, id, expected)
		if err != nil {
			return err
		}
		v, err := st.Offers().Version(ctx, o.ID, versionID)
		if errors.Is(err, apperr.ErrNotFound) {
			return apperr.FieldError("offerVersionId", "not a version of this offer")
		} else if err != nil {
			return err
		}
		_, audience := v.Decode()
		for _, partner := range audience.PartnerCompanyIDs {
			active, err := st.Partnerships().ActiveBetween(ctx, p.CompanyID, partner)
			if err != nil {
				return err
			}
			if !active {
				return apperr.New(apperr.ErrConflict, "partner_not_active", "every selected company must be an active partner")
			}
		}
		now := s.clock()
		if err := st.Offers().MarkPublished(ctx, v.ID, now); err != nil {
			return err
		}
		o.Status, o.PublishedVersionID, o.UpdatedAt = model.OfferPublished, &v.ID, now
		if err := st.Offers().Update(ctx, o, expected); err != nil {
			return err
		}
		return s.event(ctx, st, p, "offer.published", "offer", o.ID, "", map[string]any{"versionId": v.ID, "number": v.Number})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}

// Withdraw ends the offer for new orders; existing orders keep their terms.
func (s *Offer) Withdraw(ctx context.Context, p *auth.Principal, id string, expected int64, why string) (*OfferView, error) {
	var v apperr.Validation
	why = validate.Reason(&v, why)
	if err := v.Err(); err != nil {
		return nil, err
	}
	err := s.store.InTx(ctx, func(st Store) error {
		o, err := s.ownOpen(ctx, st, p, id, expected)
		if err != nil {
			return err
		}
		o.Status, o.StatusReason, o.UpdatedAt = model.OfferWithdrawn, why, s.clock()
		if err := st.Offers().Update(ctx, o, expected); err != nil {
			return err
		}
		return s.event(ctx, st, p, "offer.withdrawn", "offer", o.ID, why, nil)
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, p, id)
}

func (s *Offer) view(ctx context.Context, o *model.Offer, own bool) (*OfferView, error) {
	supplier, err := s.directory.Company(ctx, o.SupplierCompanyID)
	if err != nil {
		return nil, err
	}
	out := &OfferView{Offer: *o, Supplier: *supplier, Own: own}
	if own {
		if out.Versions, err = s.store.Offers().Versions(ctx, o.ID); err != nil {
			return nil, err
		}
	}
	if o.PublishedVersionID != nil {
		if out.Published, err = s.store.Offers().Version(ctx, o.ID, *o.PublishedVersionID); err != nil {
			return nil, err
		}
	}
	if !own && o.Status != model.OfferPublished {
		out.Published = nil
	}
	return out, nil
}

// Get returns the supplier's own offer, or a published offer visible to the
// buyer. Everything else is not found.
func (s *Offer) Get(ctx context.Context, p *auth.Principal, id string) (*OfferView, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	o, err := s.store.Offers().GetOwn(ctx, p.CompanyID, id)
	if err == nil {
		return s.view(ctx, o, true)
	}
	if !errors.Is(err, apperr.ErrNotFound) {
		return nil, err
	}
	o, err = s.store.Offers().GetVisible(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, o, false)
}

// List returns own offers (scope "own") or partners' offers available to the
// company (scope "available").
func (s *Offer) List(ctx context.Context, p *auth.Principal, scope string, limit, offset int) ([]OfferView, error) {
	var offers []model.Offer
	var err error
	switch scope {
	case "own":
		offers, err = s.store.Offers().ListOwn(ctx, p.CompanyID, limit, offset)
	case "available", "":
		offers, err = s.store.Offers().ListVisible(ctx, p.CompanyID, limit, offset)
	default:
		return nil, apperr.FieldError("scope", "must be own or available")
	}
	if err != nil {
		return nil, err
	}
	out := make([]OfferView, 0, len(offers))
	for i := range offers {
		v, err := s.view(ctx, &offers[i], scope == "own")
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}
