package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"justixauto/internal/modules/retail/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/money"
	"justixauto/internal/pkg/validate"
)

// Listing manages marketplace listings of vehicles.
type Listing struct{ Deps }

type ListingInput struct {
	VehicleID   string      `json:"vehicleId"`
	Text        string      `json:"text"`
	AskingPrice money.Money `json:"askingPrice"`
}

func price(v *apperr.Validation, field string, m money.Money) {
	if n, ok := m.Parse(); !ok || n.Sign() == 0 {
		v.Add(field, "must be a positive amount in minor units with a currency code")
	}
}

// eligible checks the vehicle belongs to the company. Listings never change
// vehicle facts (VIN, customs, documents).
func (s *Listing) eligible(ctx context.Context, p *auth.Principal, vehicleID string) error {
	v, err := s.stock.Vehicle(ctx, p.CompanyID, vehicleID)
	if errors.Is(err, apperr.ErrNotFound) || (err == nil && !v.Owned) {
		return apperr.FieldError("vehicleId", "not a vehicle your company owns")
	}
	return err
}

func (s *Listing) Create(ctx context.Context, p *auth.Principal, in ListingInput) (*model.Listing, error) {
	var v apperr.Validation
	text := validate.Text(&v, "text", in.Text, 0, 5000)
	price(&v, "askingPrice", in.AskingPrice)
	if validate.IDs(in.VehicleID) != nil {
		v.Add("vehicleId", "must be a valid ID")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	if err := s.eligible(ctx, p, in.VehicleID); err != nil {
		return nil, err
	}
	now := s.clock()
	l := &model.Listing{
		ID: uuid.NewString(), CompanyID: p.CompanyID, VehicleID: in.VehicleID, Text: text,
		AskingPriceMinor: in.AskingPrice.AmountMinor, Currency: in.AskingPrice.Currency, Status: "draft", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	err := s.store.InTx(ctx, func(st Store) error {
		if err := st.Listings().Create(ctx, l); err != nil {
			if errors.Is(err, apperr.ErrConflict) {
				return apperr.New(apperr.ErrConflict, "listing_exists", "the vehicle already has an open listing")
			}
			return err
		}
		return s.event(ctx, st, p, "listing.created", "listing", l.ID, "", nil)
	})
	return l, err
}

func (s *Listing) get(ctx context.Context, st Store, p *auth.Principal, id string, expected int64) (*model.Listing, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	l, err := st.Listings().Get(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	if expected >= 0 && l.Version != expected {
		return nil, apperr.ErrStale
	}
	return l, nil
}

// Update changes the text and asking price of an open listing.
func (s *Listing) Update(ctx context.Context, p *auth.Principal, id string, expected int64, text string, askingPrice money.Money) (*model.Listing, error) {
	var v apperr.Validation
	text = validate.Text(&v, "text", text, 0, 5000)
	price(&v, "askingPrice", askingPrice)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var l *model.Listing
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if l, err = s.get(ctx, st, p, id, expected); err != nil {
			return err
		}
		if l.Status == "withdrawn" {
			return apperr.New(apperr.ErrConflict, "listing_withdrawn", "the listing was withdrawn")
		}
		l.Text, l.AskingPriceMinor, l.Currency, l.UpdatedAt = text, askingPrice.AmountMinor, askingPrice.Currency, s.clock()
		if err := st.Listings().Update(ctx, l, expected); err != nil {
			return err
		}
		return s.event(ctx, st, p, "listing.updated", "listing", l.ID, "", nil)
	})
	return l, err
}

// SetPublished publishes (after re-checking the vehicle) or withdraws a listing.
func (s *Listing) SetPublished(ctx context.Context, p *auth.Principal, id string, expected int64, publish bool) (*model.Listing, error) {
	var l *model.Listing
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if l, err = s.get(ctx, st, p, id, expected); err != nil {
			return err
		}
		switch {
		case publish && l.Status != "draft":
			return apperr.New(apperr.ErrConflict, "invalid_transition", "only a draft listing can be published")
		case !publish && l.Status == "withdrawn":
			return apperr.New(apperr.ErrConflict, "invalid_transition", "the listing was already withdrawn")
		}
		if publish {
			if err := s.eligible(st.Bind(ctx), p, l.VehicleID); err != nil {
				return err
			}
			l.Status = "published"
		} else {
			l.Status = "withdrawn"
		}
		l.UpdatedAt = s.clock()
		if err := st.Listings().Update(ctx, l, expected); err != nil {
			return err
		}
		return s.event(ctx, st, p, "listing."+l.Status, "listing", l.ID, "", nil)
	})
	return l, err
}

func (s *Listing) List(ctx context.Context, p *auth.Principal, status string, limit, offset int) ([]model.Listing, error) {
	return s.store.Listings().List(ctx, p.CompanyID, status, limit, offset)
}

func (s *Listing) Get(ctx context.Context, p *auth.Principal, id string) (*model.Listing, error) {
	return s.get(ctx, s.store, p, id, -1)
}
