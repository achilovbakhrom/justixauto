package retail

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/database"
	"justixauto/internal/pkg/money"
	"justixauto/internal/pkg/validate"
)

type Listing struct {
	ID               string `gorm:"primaryKey;type:uuid"`
	CompanyID        string `gorm:"type:uuid"`
	VehicleID        string `gorm:"type:uuid"`
	Text             string
	AskingPriceMinor string
	Currency         string
	Status           string // draft | published | withdrawn
	Version          int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (Listing) TableName() string { return "retail.listings" }

func (l *Listing) Price() money.Money {
	return money.Money{AmountMinor: l.AskingPriceMinor, Currency: l.Currency}
}

type ListingRepository interface {
	Create(ctx context.Context, l *Listing) error
	Get(ctx context.Context, companyID, id string) (*Listing, error)
	List(ctx context.Context, companyID, status string, limit, offset int) ([]Listing, error)
	Update(ctx context.Context, l *Listing, expected int64) error
	// WithdrawForVehicle closes open listings of a delivered vehicle.
	WithdrawForVehicle(ctx context.Context, vehicleID string, at time.Time) error
}

type listingRepository struct{ db *gorm.DB }

func (r *listingRepository) Create(ctx context.Context, l *Listing) error {
	return database.Translate(r.db.WithContext(ctx).Create(l).Error)
}

func (r *listingRepository) Get(ctx context.Context, companyID, id string) (*Listing, error) {
	var l Listing
	if err := r.db.WithContext(ctx).Where("id = ? AND company_id = ?", id, companyID).Take(&l).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &l, nil
}

func (r *listingRepository) List(ctx context.Context, companyID, status string, limit, offset int) ([]Listing, error) {
	limit, offset = database.Page(limit, offset)
	q := r.db.WithContext(ctx).Where("company_id = ?", companyID).Order("updated_at DESC, id").Limit(limit).Offset(offset)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	ls := []Listing{}
	if err := database.Translate(q.Find(&ls).Error); err != nil {
		return nil, err
	}
	return ls, nil
}

func (r *listingRepository) Update(ctx context.Context, l *Listing, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Listing{}, l.ID, expected, map[string]any{
		"text": l.Text, "asking_price_minor": l.AskingPriceMinor, "currency": l.Currency, "status": l.Status, "updated_at": l.UpdatedAt,
	})
	if err == nil {
		l.Version = expected + 1
	}
	return err
}

func (r *listingRepository) WithdrawForVehicle(ctx context.Context, vehicleID string, at time.Time) error {
	return database.Translate(r.db.WithContext(ctx).Model(&Listing{}).Where("vehicle_id = ? AND status <> 'withdrawn'", vehicleID).
		Updates(map[string]any{"status": "withdrawn", "updated_at": at, "version": gorm.Expr("version + 1")}).Error)
}

type ListingService struct{ deps }

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
func (s *ListingService) eligible(ctx context.Context, p *auth.Principal, vehicleID string) error {
	v, err := s.stock.Vehicle(ctx, p.CompanyID, vehicleID)
	if errors.Is(err, apperr.ErrNotFound) || (err == nil && !v.Owned) {
		return apperr.FieldError("vehicleId", "not a vehicle your company owns")
	}
	return err
}

func (s *ListingService) Create(ctx context.Context, p *auth.Principal, in ListingInput) (*Listing, error) {
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
	l := &Listing{
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

func (s *ListingService) get(ctx context.Context, st Store, p *auth.Principal, id string, expected int64) (*Listing, error) {
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
func (s *ListingService) Update(ctx context.Context, p *auth.Principal, id string, expected int64, text string, askingPrice money.Money) (*Listing, error) {
	var v apperr.Validation
	text = validate.Text(&v, "text", text, 0, 5000)
	price(&v, "askingPrice", askingPrice)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var l *Listing
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
func (s *ListingService) SetPublished(ctx context.Context, p *auth.Principal, id string, expected int64, publish bool) (*Listing, error) {
	var l *Listing
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

func (s *ListingService) List(ctx context.Context, p *auth.Principal, status string, limit, offset int) ([]Listing, error) {
	return s.store.Listings().List(ctx, p.CompanyID, status, limit, offset)
}

func (s *ListingService) Get(ctx context.Context, p *auth.Principal, id string) (*Listing, error) {
	return s.get(ctx, s.store, p, id, -1)
}
