package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/database"
	"justixauto/internal/pkg/jsonx"
	"justixauto/internal/pkg/money"
	"justixauto/internal/pkg/validate"
)

const PermOffersManage = "commerce.offers.manage"

// Catalog looks up vehicle models (implemented by the inventory module).
// It returns apperr.ErrNotFound for unknown IDs.
type Catalog interface {
	Model(ctx context.Context, id string) (*Model, error)
}

// Model is what commerce needs to know about a catalogue model.
type Model struct{ ID, Name string }

// ---- commercial terms (contract CommercialTerms) ----

type Line struct {
	LineID    string         `json:"lineId"`
	ModelID   string         `json:"modelId"`
	Quantity  jsonx.Quantity `json:"quantity"`
	UnitPrice money.Money    `json:"unitPrice"`
}

type Installment struct {
	ID      string      `json:"id"`
	Amount  money.Money `json:"amount"`
	DueDate string      `json:"dueDate"` // YYYY-MM-DD
}

type Terms struct {
	Lines           []Line        `json:"lines"`
	Route           string        `json:"route"` // factory | foreign-direct | in-transit | local
	DeliveryTerms   string        `json:"deliveryTerms"`
	PaymentSchedule []Installment `json:"paymentSchedule"`
	WarrantyTerms   string        `json:"warrantyTerms"`
	ServiceTerms    string        `json:"serviceTerms"`
}

var routes = []string{"factory", "foreign-direct", "in-transit", "local"}

// validateTerms normalizes terms and returns the exact total. All amounts
// share one currency (no conversion); a payment schedule must add up to the
// total exactly. Line and installment IDs are generated when missing.
func (d deps) validateTerms(ctx context.Context, v *apperr.Validation, in Terms) Terms {
	out := Terms{
		Route: in.Route, Lines: []Line{}, PaymentSchedule: []Installment{},
		DeliveryTerms: validate.Text(v, "terms.deliveryTerms", in.DeliveryTerms, 0, 2000),
		WarrantyTerms: validate.Text(v, "terms.warrantyTerms", in.WarrantyTerms, 0, 2000),
		ServiceTerms:  validate.Text(v, "terms.serviceTerms", in.ServiceTerms, 0, 2000),
	}
	if !slices.Contains(routes, in.Route) {
		v.Add("terms.route", "must be factory, foreign-direct, in-transit or local")
	}
	if len(in.Lines) == 0 || len(in.Lines) > 100 {
		v.Add("terms.lines", "list 1-100 lines")
	}
	currency := ""
	ids := map[string]bool{}
	total := d.validateLines(ctx, v, in.Lines, ids, &out.Lines, &currency)
	if !money.Fits(total) {
		v.Add("terms.lines", "total is too large")
	}
	if len(in.PaymentSchedule) > 60 {
		v.Add("terms.paymentSchedule", "at most 60 installments")
	}
	scheduled := validateSchedule(v, in.PaymentSchedule, ids, &out.PaymentSchedule, &currency)
	if len(in.PaymentSchedule) > 0 && scheduled.Cmp(total) != 0 {
		v.Add("terms.paymentSchedule", "installments must add up to the total "+total.String())
	}
	return out
}

// sameCurrency records the first currency seen and flags any later amount
// that uses a different one.
func sameCurrency(v *apperr.Validation, currency *string, field, c string) {
	if *currency == "" {
		*currency = c
	} else if c != *currency {
		v.Add(field, "all amounts must use one currency ("+*currency+")")
	}
}

// validateLines normalizes order lines, checks each against the catalog and
// returns the exact total (unit price * quantity, summed). Generated or
// duplicate line IDs are tracked in ids so the payment schedule can also
// reject collisions against them.
func (d deps) validateLines(ctx context.Context, v *apperr.Validation, lines []Line, ids map[string]bool, out *[]Line, currency *string) *big.Int {
	total := new(big.Int)
	for i, l := range lines {
		field := "terms.lines." + strconv.Itoa(i)
		if l.LineID == "" {
			l.LineID = uuid.NewString()
		}
		if uuid.Validate(l.LineID) != nil || ids[l.LineID] {
			v.Add(field+".lineId", "must be a unique ID")
		}
		ids[l.LineID] = true
		if l.Quantity < 1 || l.Quantity > 10_000 {
			v.Add(field+".quantity", "must be 1-10000")
		}
		price, ok := l.UnitPrice.Parse()
		if !ok || price.Sign() == 0 {
			v.Add(field+".unitPrice", "must be a positive amount in minor units with a currency code")
		} else {
			sameCurrency(v, currency, field+".unitPrice", l.UnitPrice.Currency)
			total.Add(total, new(big.Int).Mul(price, big.NewInt(int64(l.Quantity))))
		}
		if uuid.Validate(l.ModelID) != nil {
			v.Add(field+".modelId", "must be a valid ID")
		} else if _, err := d.catalog.Model(ctx, l.ModelID); err != nil {
			v.Add(field+".modelId", "unknown vehicle model")
		}
		*out = append(*out, l)
	}
	return total
}

// validateSchedule normalizes the payment schedule and returns the total
// amount scheduled. ids also tracks line IDs so schedule IDs cannot collide
// with them.
func validateSchedule(v *apperr.Validation, schedule []Installment, ids map[string]bool, out *[]Installment, currency *string) *big.Int {
	scheduled := new(big.Int)
	for i, p := range schedule {
		field := "terms.paymentSchedule." + strconv.Itoa(i)
		if p.ID == "" {
			p.ID = uuid.NewString()
		}
		if uuid.Validate(p.ID) != nil || ids[p.ID] {
			v.Add(field+".id", "must be a unique ID")
		}
		ids[p.ID] = true
		amount, ok := p.Amount.Parse()
		if !ok || amount.Sign() == 0 {
			v.Add(field+".amount", "must be a positive amount in minor units with a currency code")
		} else {
			sameCurrency(v, currency, field+".amount", p.Amount.Currency)
			scheduled.Add(scheduled, amount)
		}
		if _, err := time.Parse(time.DateOnly, p.DueDate); err != nil {
			v.Add(field+".dueDate", "must be a date (YYYY-MM-DD)")
		}
		*out = append(*out, p)
	}
	return scheduled
}

// Audience decides which partners see a published offer.
type Audience struct {
	Mode              string   `json:"mode"` // all-active | selected
	PartnerCompanyIDs []string `json:"partnerCompanyIds"`
}

func validateAudience(v *apperr.Validation, own string, in Audience) Audience {
	out := Audience{Mode: in.Mode, PartnerCompanyIDs: validate.UniqueIDs(v, "audience.partnerCompanyIds", in.PartnerCompanyIDs)}
	switch in.Mode {
	case "all-active":
		if len(out.PartnerCompanyIDs) > 0 {
			v.Add("audience.partnerCompanyIds", "must be empty for all-active")
		}
	case "selected":
		if len(out.PartnerCompanyIDs) == 0 || len(out.PartnerCompanyIDs) > 500 {
			v.Add("audience.partnerCompanyIds", "select 1-500 partners")
		}
		if slices.Contains(out.PartnerCompanyIDs, own) {
			v.Add("audience.partnerCompanyIds", "cannot include your own company")
		}
	default:
		v.Add("audience.mode", "must be all-active or selected")
	}
	return out
}

// ---- model ----

type OfferStatus string

const (
	OfferDraft     OfferStatus = "draft"
	OfferPublished OfferStatus = "published"
	OfferWithdrawn OfferStatus = "withdrawn"
)

type Offer struct {
	ID                 string `gorm:"primaryKey;type:uuid"`
	SupplierCompanyID  string `gorm:"type:uuid"`
	Status             OfferStatus
	PublishedVersionID *string `gorm:"type:uuid"`
	StatusReason       string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (Offer) TableName() string { return "commerce.offers" }

// OfferVersion is immutable once created.
type OfferVersion struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	OfferID      string `gorm:"type:uuid"`
	Number       int
	Terms        []byte `gorm:"type:jsonb"`
	AudienceMode string
	AudienceIDs  []byte `gorm:"type:jsonb"`
	CreatedBy    string `gorm:"type:uuid"`
	CreatedAt    time.Time
	PublishedAt  *time.Time
}

func (OfferVersion) TableName() string { return "commerce.offer_versions" }

func (v *OfferVersion) decode() (Terms, Audience) {
	var t Terms
	a := Audience{Mode: v.AudienceMode}
	_ = json.Unmarshal(v.Terms, &t)
	_ = json.Unmarshal(v.AudienceIDs, &a.PartnerCompanyIDs)
	return t, a
}

// ---- repository ----

type OfferRepository interface {
	Create(ctx context.Context, o *Offer, v *OfferVersion) error
	// Get returns the supplier's own offer, else ErrNotFound.
	GetOwn(ctx context.Context, supplierID, id string) (*Offer, error)
	// GetVisible returns a published offer the buyer may see, else ErrNotFound.
	GetVisible(ctx context.Context, buyerID, id string) (*Offer, error)
	// VisibleByPublishedVersion finds the visible offer whose published version is versionID.
	VisibleByPublishedVersion(ctx context.Context, buyerID, versionID string) (*Offer, error)
	ListOwn(ctx context.Context, supplierID string, limit, offset int) ([]Offer, error)
	ListVisible(ctx context.Context, buyerID string, limit, offset int) ([]Offer, error)
	Versions(ctx context.Context, offerID string) ([]OfferVersion, error)
	Version(ctx context.Context, offerID, versionID string) (*OfferVersion, error)
	AddVersion(ctx context.Context, v *OfferVersion) error
	Update(ctx context.Context, o *Offer, expected int64) error
	MarkPublished(ctx context.Context, versionID string, at time.Time) error
}

type offerRepository struct{ db *gorm.DB }

func (r *offerRepository) Create(ctx context.Context, o *Offer, v *OfferVersion) error {
	db := r.db.WithContext(ctx)
	if err := db.Create(o).Error; err != nil {
		return database.Translate(err)
	}
	return database.Translate(db.Create(v).Error)
}

func (r *offerRepository) GetOwn(ctx context.Context, supplierID, id string) (*Offer, error) {
	var o Offer
	if err := r.db.WithContext(ctx).Where("id = ? AND supplier_company_id = ?", id, supplierID).Take(&o).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &o, nil
}

// visible: published, supplier is an active partner of the buyer, and the
// published version's audience includes the buyer.
func (r *offerRepository) visible(ctx context.Context, buyerID string) *gorm.DB {
	return r.db.WithContext(ctx).Model(&Offer{}).
		Joins("JOIN commerce.offer_versions v ON v.id = offers.published_version_id").
		Where("offers.status = ? AND offers.supplier_company_id <> ?", OfferPublished, buyerID).
		Where(`EXISTS (SELECT 1 FROM commerce.partnerships p WHERE p.status = 'active' AND
			((p.requester_company_id = offers.supplier_company_id AND p.recipient_company_id = ?) OR
			 (p.recipient_company_id = offers.supplier_company_id AND p.requester_company_id = ?)))`, buyerID, buyerID).
		Where("(v.audience_mode = 'all-active' OR v.audience_ids @> jsonb_build_array(?::text))", buyerID)
}

func (r *offerRepository) GetVisible(ctx context.Context, buyerID, id string) (*Offer, error) {
	var o Offer
	if err := r.visible(ctx, buyerID).Where("offers.id = ?", id).Select("offers.*").Take(&o).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &o, nil
}

func (r *offerRepository) VisibleByPublishedVersion(ctx context.Context, buyerID, versionID string) (*Offer, error) {
	var o Offer
	err := r.visible(ctx, buyerID).Where("offers.published_version_id = ?", versionID).Select("offers.*").Take(&o).Error
	if err != nil {
		return nil, database.Translate(err)
	}
	return &o, nil
}

func (r *offerRepository) ListOwn(ctx context.Context, supplierID string, limit, offset int) ([]Offer, error) {
	limit, offset = database.Page(limit, offset)
	os := []Offer{}
	err := r.db.WithContext(ctx).Where("supplier_company_id = ?", supplierID).Order("updated_at DESC, id").
		Limit(limit).Offset(offset).Find(&os).Error
	return os, database.Translate(err)
}

func (r *offerRepository) ListVisible(ctx context.Context, buyerID string, limit, offset int) ([]Offer, error) {
	limit, offset = database.Page(limit, offset)
	os := []Offer{}
	err := r.visible(ctx, buyerID).Select("offers.*").Order("offers.updated_at DESC, offers.id").
		Limit(limit).Offset(offset).Find(&os).Error
	return os, database.Translate(err)
}

func (r *offerRepository) Versions(ctx context.Context, offerID string) ([]OfferVersion, error) {
	vs := []OfferVersion{}
	err := r.db.WithContext(ctx).Where("offer_id = ?", offerID).Order("number").Find(&vs).Error
	return vs, database.Translate(err)
}

func (r *offerRepository) Version(ctx context.Context, offerID, versionID string) (*OfferVersion, error) {
	var v OfferVersion
	if err := r.db.WithContext(ctx).Where("id = ? AND offer_id = ?", versionID, offerID).Take(&v).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &v, nil
}

func (r *offerRepository) AddVersion(ctx context.Context, v *OfferVersion) error {
	db := r.db.WithContext(ctx)
	if err := db.Model(&OfferVersion{}).Where("offer_id = ?", v.OfferID).Select("coalesce(max(number), 0) + 1").Scan(&v.Number).Error; err != nil {
		return database.Translate(err)
	}
	return database.Translate(db.Create(v).Error)
}

func (r *offerRepository) Update(ctx context.Context, o *Offer, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Offer{}, o.ID, expected, map[string]any{
		"status": o.Status, "published_version_id": o.PublishedVersionID, "status_reason": o.StatusReason, "updated_at": o.UpdatedAt,
	})
	if err == nil {
		o.Version = expected + 1
	}
	return err
}

func (r *offerRepository) MarkPublished(ctx context.Context, versionID string, at time.Time) error {
	return database.Translate(r.db.WithContext(ctx).Model(&OfferVersion{}).
		Where("id = ? AND published_at IS NULL", versionID).Update("published_at", at).Error)
}

// ---- service ----

type OfferInput struct {
	Terms    Terms    `json:"terms"`
	Audience Audience `json:"audience"`
}

// OfferView is an offer as one company may see it. Suppliers get every
// version; buyers only the published one.
type OfferView struct {
	Offer     Offer
	Supplier  Company
	Published *OfferVersion
	Versions  []OfferVersion
	Own       bool
}

type OfferService struct{ deps }

func (s *OfferService) newVersion(ctx context.Context, p *auth.Principal, offerID string, in OfferInput) (*OfferVersion, error) {
	var v apperr.Validation
	terms := s.validateTerms(ctx, &v, in.Terms)
	audience := validateAudience(&v, p.CompanyID, in.Audience)
	if err := v.Err(); err != nil {
		return nil, err
	}
	rawTerms, _ := json.Marshal(terms)
	rawIDs, _ := json.Marshal(audience.PartnerCompanyIDs)
	return &OfferVersion{
		ID: uuid.NewString(), OfferID: offerID, Number: 1, Terms: rawTerms, AudienceMode: audience.Mode,
		AudienceIDs: rawIDs, CreatedBy: p.UserID, CreatedAt: s.clock(),
	}, nil
}

// Create starts a draft offer with version 1.
func (s *OfferService) Create(ctx context.Context, p *auth.Principal, in OfferInput) (*OfferView, error) {
	supplier, err := s.tradingCompany(ctx, p.CompanyID, "companyId")
	if err != nil {
		return nil, err
	}
	now := s.clock()
	o := &Offer{ID: uuid.NewString(), SupplierCompanyID: p.CompanyID, Status: OfferDraft, Version: 1, CreatedAt: now, UpdatedAt: now}
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
	return &OfferView{Offer: *o, Supplier: *supplier, Versions: []OfferVersion{*v}, Own: true}, nil
}

func (s *OfferService) ownOpen(ctx context.Context, st Store, p *auth.Principal, id string, expected int64) (*Offer, error) {
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
	if o.Status == OfferWithdrawn {
		return nil, apperr.New(apperr.ErrConflict, "offer_withdrawn", "the offer was withdrawn; create a new one")
	}
	return o, nil
}

// AddVersion stores a new immutable version. It is not visible to partners
// until it is published; the currently published version stays in effect.
func (s *OfferService) AddVersion(ctx context.Context, p *auth.Principal, id string, expected int64, in OfferInput) (*OfferView, error) {
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
func (s *OfferService) Publish(ctx context.Context, p *auth.Principal, id string, expected int64, versionID string) (*OfferView, error) {
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
		_, audience := v.decode()
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
		o.Status, o.PublishedVersionID, o.UpdatedAt = OfferPublished, &v.ID, now
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
func (s *OfferService) Withdraw(ctx context.Context, p *auth.Principal, id string, expected int64, why string) (*OfferView, error) {
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
		o.Status, o.StatusReason, o.UpdatedAt = OfferWithdrawn, why, s.clock()
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

func (s *OfferService) view(ctx context.Context, o *Offer, own bool) (*OfferView, error) {
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
	if !own && o.Status != OfferPublished {
		out.Published = nil
	}
	return out, nil
}

// Get returns the supplier's own offer, or a published offer visible to the
// buyer. Everything else is not found.
func (s *OfferService) Get(ctx context.Context, p *auth.Principal, id string) (*OfferView, error) {
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
func (s *OfferService) List(ctx context.Context, p *auth.Principal, scope string, limit, offset int) ([]OfferView, error) {
	var offers []Offer
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

// termsTotal recomputes the exact total of stored terms.
func termsTotal(t Terms) money.Money {
	total, currency := new(big.Int), ""
	for _, l := range t.Lines {
		if price, ok := l.UnitPrice.Parse(); ok {
			total.Add(total, new(big.Int).Mul(price, big.NewInt(int64(l.Quantity))))
			currency = l.UnitPrice.Currency
		}
	}
	return money.Of(total, currency)
}
