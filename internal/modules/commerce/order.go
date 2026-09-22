package commerce

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/database"
	"justixauto/internal/platform/jsonx"
	"justixauto/internal/platform/validate"
)

// PermTrade covers the deal flow: RFQs, quotations, orders and addenda.
const PermTrade = "commerce.trade"

// ---- model ----

type RFQLine struct {
	LineID   string         `json:"lineId"`
	ModelID  string         `json:"modelId"`
	Quantity jsonx.Quantity `json:"quantity"`
}

type RFQStatus string

const (
	RFQDraft       RFQStatus = "draft"
	RFQSent        RFQStatus = "sent"
	RFQNegotiating RFQStatus = "negotiating"
	RFQAccepted    RFQStatus = "accepted"
	RFQDeclined    RFQStatus = "declined"
	RFQCancelled   RFQStatus = "cancelled"
)

type RFQ struct {
	ID                string  `gorm:"primaryKey;type:uuid"`
	BuyerCompanyID    string  `gorm:"type:uuid"`
	SupplierCompanyID string  `gorm:"type:uuid"`
	OfferVersionID    *string `gorm:"type:uuid"`
	Lines             []byte  `gorm:"type:jsonb"`
	Status            RFQStatus
	StatusReason      string
	Version           int64
	CreatedBy         string `gorm:"type:uuid"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (RFQ) TableName() string { return "commerce.rfqs" }

func (r *RFQ) lines() []RFQLine {
	var ls []RFQLine
	_ = json.Unmarshal(r.Lines, &ls)
	return ls
}

type Quotation struct {
	ID        string `gorm:"primaryKey;type:uuid"`
	RFQID     string `gorm:"column:rfq_id;type:uuid"`
	Number    int
	Terms     []byte `gorm:"type:jsonb"`
	Digest    string
	CreatedBy string `gorm:"type:uuid"`
	CreatedAt time.Time
}

func (Quotation) TableName() string { return "commerce.quotations" }

type OrderStatus string

const (
	AwaitingSupplier OrderStatus = "awaiting-supplier"
	OrderAccepted    OrderStatus = "accepted"
	OrderFulfilling  OrderStatus = "fulfilling"
	OrderCompleted   OrderStatus = "completed"
	OrderCancelled   OrderStatus = "cancelled"
)

type Order struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	BuyerCompanyID    string `gorm:"type:uuid"`
	SupplierCompanyID string `gorm:"type:uuid"`
	Source            string
	RFQID             *string `gorm:"column:rfq_id;type:uuid"`
	QuotationID       *string `gorm:"type:uuid"`
	OfferVersionID    *string `gorm:"type:uuid"`
	Terms             []byte  `gorm:"type:jsonb"`
	Status            OrderStatus
	StatusReason      string
	Version           int64
	CreatedBy         string `gorm:"type:uuid"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (Order) TableName() string { return "commerce.orders" }

func (o *Order) terms() Terms {
	var t Terms
	_ = json.Unmarshal(o.Terms, &t)
	return t
}

// Party returns "buyer", "supplier" or "" for companyID.
func (o *Order) Party(companyID string) string {
	switch companyID {
	case o.BuyerCompanyID:
		return "buyer"
	case o.SupplierCompanyID:
		return "supplier"
	}
	return ""
}

type Addendum struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	OrderID           string `gorm:"type:uuid"`
	Number            int
	Terms             []byte `gorm:"type:jsonb"`
	Reason            string
	ProposedByCompany string `gorm:"type:uuid"`
	Status            string // proposed | accepted | rejected
	DecisionReason    string
	CreatedAt         time.Time
	DecidedAt         *time.Time
}

func (Addendum) TableName() string { return "commerce.order_addenda" }

// digest pins exact terms: accepting a quotation requires the digest the
// buyer reviewed, so a quietly changed quotation cannot be accepted.
func digest(raw []byte) string {
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

// ---- repository ----

type DealRepository interface {
	CreateRFQ(ctx context.Context, r *RFQ) error
	// RFQ returns an RFQ visible to the company: the buyer always, the
	// supplier once it was sent.
	RFQ(ctx context.Context, companyID, id string) (*RFQ, error)
	RFQs(ctx context.Context, companyID string, limit, offset int) ([]RFQ, error)
	UpdateRFQ(ctx context.Context, r *RFQ, expected int64) error
	AddQuotation(ctx context.Context, q *Quotation) error
	Quotations(ctx context.Context, rfqID string) ([]Quotation, error)

	CreateOrder(ctx context.Context, o *Order) error
	// Order returns an order the company is a party of.
	Order(ctx context.Context, companyID, id string) (*Order, error)
	Orders(ctx context.Context, companyID string, limit, offset int) ([]Order, error)
	UpdateOrder(ctx context.Context, o *Order, expected int64) error
	AddAddendum(ctx context.Context, a *Addendum) error
	Addenda(ctx context.Context, orderID string) ([]Addendum, error)
	DecideAddendum(ctx context.Context, a *Addendum) error
}

type dealRepository struct{ db *gorm.DB }

func (r *dealRepository) CreateRFQ(ctx context.Context, x *RFQ) error {
	return database.Translate(r.db.WithContext(ctx).Create(x).Error)
}

func (r *dealRepository) RFQ(ctx context.Context, companyID, id string) (*RFQ, error) {
	var x RFQ
	err := r.db.WithContext(ctx).Where("id = ? AND (buyer_company_id = ? OR (supplier_company_id = ? AND status <> ?))",
		id, companyID, companyID, RFQDraft).Take(&x).Error
	if err != nil {
		return nil, database.Translate(err)
	}
	return &x, nil
}

func (r *dealRepository) RFQs(ctx context.Context, companyID string, limit, offset int) ([]RFQ, error) {
	limit, offset = database.Page(limit, offset)
	xs := []RFQ{}
	err := r.db.WithContext(ctx).Where("buyer_company_id = ? OR (supplier_company_id = ? AND status <> ?)", companyID, companyID, RFQDraft).
		Order("updated_at DESC, id").Limit(limit).Offset(offset).Find(&xs).Error
	return xs, database.Translate(err)
}

func (r *dealRepository) UpdateRFQ(ctx context.Context, x *RFQ, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &RFQ{}, x.ID, expected, map[string]any{
		"status": x.Status, "status_reason": x.StatusReason, "lines": x.Lines, "updated_at": x.UpdatedAt})
	if err == nil {
		x.Version = expected + 1
	}
	return err
}

func (r *dealRepository) AddQuotation(ctx context.Context, q *Quotation) error {
	db := r.db.WithContext(ctx)
	if err := db.Model(&Quotation{}).Where("rfq_id = ?", q.RFQID).Select("coalesce(max(number), 0) + 1").Scan(&q.Number).Error; err != nil {
		return database.Translate(err)
	}
	return database.Translate(db.Create(q).Error)
}

func (r *dealRepository) Quotations(ctx context.Context, rfqID string) ([]Quotation, error) {
	qs := []Quotation{}
	err := r.db.WithContext(ctx).Where("rfq_id = ?", rfqID).Order("number").Find(&qs).Error
	return qs, database.Translate(err)
}

func (r *dealRepository) CreateOrder(ctx context.Context, o *Order) error {
	return database.Translate(r.db.WithContext(ctx).Create(o).Error)
}

func (r *dealRepository) Order(ctx context.Context, companyID, id string) (*Order, error) {
	var o Order
	err := r.db.WithContext(ctx).Where("id = ? AND (buyer_company_id = ? OR supplier_company_id = ?)", id, companyID, companyID).Take(&o).Error
	if err != nil {
		return nil, database.Translate(err)
	}
	return &o, nil
}

func (r *dealRepository) Orders(ctx context.Context, companyID string, limit, offset int) ([]Order, error) {
	limit, offset = database.Page(limit, offset)
	os := []Order{}
	err := r.db.WithContext(ctx).Where("buyer_company_id = ? OR supplier_company_id = ?", companyID, companyID).
		Order("updated_at DESC, id").Limit(limit).Offset(offset).Find(&os).Error
	return os, database.Translate(err)
}

func (r *dealRepository) UpdateOrder(ctx context.Context, o *Order, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Order{}, o.ID, expected, map[string]any{
		"status": o.Status, "status_reason": o.StatusReason, "terms": o.Terms, "updated_at": o.UpdatedAt})
	if err == nil {
		o.Version = expected + 1
	}
	return err
}

func (r *dealRepository) AddAddendum(ctx context.Context, a *Addendum) error {
	db := r.db.WithContext(ctx)
	if err := db.Model(&Addendum{}).Where("order_id = ?", a.OrderID).Select("coalesce(max(number), 0) + 1").Scan(&a.Number).Error; err != nil {
		return database.Translate(err)
	}
	return database.Translate(db.Create(a).Error)
}

func (r *dealRepository) Addenda(ctx context.Context, orderID string) ([]Addendum, error) {
	as := []Addendum{}
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).Order("number").Find(&as).Error
	return as, database.Translate(err)
}

func (r *dealRepository) DecideAddendum(ctx context.Context, a *Addendum) error {
	res := r.db.WithContext(ctx).Model(&Addendum{}).Where("id = ? AND status = 'proposed'", a.ID).
		Updates(map[string]any{"status": a.Status, "decision_reason": a.DecisionReason, "decided_at": a.DecidedAt})
	if res.Error == nil && res.RowsAffected == 0 {
		return apperr.New(apperr.ErrConflict, "addendum_decided", "the addendum was already decided")
	}
	return database.Translate(res.Error)
}

// ---- service ----

type DealService struct {
	deps
	offers *OfferService
}

func (s *DealService) requirePartner(ctx context.Context, st Store, a, b string) error {
	active, err := st.Partnerships().ActiveBetween(ctx, a, b)
	if err != nil {
		return err
	}
	if !active {
		return apperr.New(apperr.ErrConflict, "partnership_required", "an active partnership is required for new deals")
	}
	return nil
}

type RFQInput struct {
	SupplierCompanyID string    `json:"supplierCompanyId"`
	OfferVersionID    *string   `json:"offerVersionId"`
	Lines             []RFQLine `json:"lines"`
}

func (s *DealService) validateRFQLines(ctx context.Context, v *apperr.Validation, in []RFQLine) []RFQLine {
	out := []RFQLine{}
	if len(in) == 0 || len(in) > 100 {
		v.Add("lines", "list 1-100 lines")
	}
	for i, l := range in {
		field := "lines." + strconv.Itoa(i)
		if l.LineID == "" {
			l.LineID = uuid.NewString()
		}
		if uuid.Validate(l.LineID) != nil {
			v.Add(field+".lineId", "must be a valid ID")
		}
		if l.Quantity < 1 || l.Quantity > 10_000 {
			v.Add(field+".quantity", "must be 1-10000")
		}
		if uuid.Validate(l.ModelID) != nil {
			v.Add(field+".modelId", "must be a valid ID")
		} else if _, err := s.catalog.Model(ctx, l.ModelID); err != nil {
			v.Add(field+".modelId", "unknown vehicle model")
		}
		out = append(out, l)
	}
	return out
}

// CreateRFQ drafts a request to a partner supplier.
func (s *DealService) CreateRFQ(ctx context.Context, p *auth.Principal, in RFQInput) (*RFQ, error) {
	if uuid.Validate(in.SupplierCompanyID) != nil || in.SupplierCompanyID == p.CompanyID {
		return nil, apperr.FieldError("supplierCompanyId", "must be another company")
	}
	var v apperr.Validation
	lines := s.validateRFQLines(ctx, &v, in.Lines)
	if in.OfferVersionID != nil && uuid.Validate(*in.OfferVersionID) != nil {
		v.Add("offerVersionId", "must be a valid ID")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	if _, err := s.tradingCompany(ctx, in.SupplierCompanyID, "supplierCompanyId"); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(lines)
	now := s.clock()
	x := &RFQ{ID: uuid.NewString(), BuyerCompanyID: p.CompanyID, SupplierCompanyID: in.SupplierCompanyID,
		OfferVersionID: in.OfferVersionID, Lines: raw, Status: RFQDraft, Version: 1, CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now}
	err := s.store.InTx(ctx, func(st Store) error {
		if err := s.requirePartner(ctx, st, p.CompanyID, in.SupplierCompanyID); err != nil {
			return err
		}
		if err := st.Deals().CreateRFQ(ctx, x); err != nil {
			return err
		}
		return s.event(ctx, st, p, "rfq.created", "rfq", x.ID, "", nil)
	})
	return x, err
}

func (s *DealService) rfq(ctx context.Context, st Store, p *auth.Principal, id string, expected int64, party string) (*RFQ, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	x, err := st.Deals().RFQ(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	if x.Version != expected {
		return nil, apperr.ErrStale
	}
	if (party == "buyer") != (x.BuyerCompanyID == p.CompanyID) {
		return nil, apperr.New(apperr.ErrForbidden, "wrong_party", "only the "+party+" can do this")
	}
	return x, nil
}

// RFQAction applies send (buyer), cancel (buyer) or decline (supplier).
func (s *DealService) RFQAction(ctx context.Context, p *auth.Principal, id string, expected int64, action, why string) (*RFQ, error) {
	type rule struct {
		party  string
		from   []RFQStatus
		to     RFQStatus
		reason bool
	}
	rules := map[string]rule{
		"send":    {"buyer", []RFQStatus{RFQDraft}, RFQSent, false},
		"cancel":  {"buyer", []RFQStatus{RFQDraft, RFQSent, RFQNegotiating}, RFQCancelled, true},
		"decline": {"supplier", []RFQStatus{RFQSent, RFQNegotiating}, RFQDeclined, true},
	}
	r, ok := rules[action]
	if !ok {
		return nil, apperr.ErrNotFound
	}
	var v apperr.Validation
	if r.reason {
		why = validate.Reason(&v, why)
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var x *RFQ
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if x, err = s.rfq(ctx, st, p, id, expected, r.party); err != nil {
			return err
		}
		allowed := false
		for _, f := range r.from {
			allowed = allowed || x.Status == f
		}
		if !allowed {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "cannot "+action+" an RFQ that is "+string(x.Status))
		}
		if action == "send" {
			if err := s.requirePartner(ctx, st, x.BuyerCompanyID, x.SupplierCompanyID); err != nil {
				return err
			}
		}
		x.Status, x.StatusReason, x.UpdatedAt = r.to, why, s.clock()
		if err := st.Deals().UpdateRFQ(ctx, x, expected); err != nil {
			return err
		}
		return s.event(ctx, st, p, "rfq."+string(r.to), "rfq", x.ID, why, nil)
	})
	return x, err
}

// Quote adds the supplier's next numbered, immutable quotation.
func (s *DealService) Quote(ctx context.Context, p *auth.Principal, id string, expected int64, in Terms) (*RFQ, error) {
	var v apperr.Validation
	terms, _ := s.validateTerms(ctx, &v, in)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var x *RFQ
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if x, err = s.rfq(ctx, st, p, id, expected, "supplier"); err != nil {
			return err
		}
		if x.Status != RFQSent && x.Status != RFQNegotiating {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "cannot quote an RFQ that is "+string(x.Status))
		}
		if err := s.requirePartner(ctx, st, x.BuyerCompanyID, x.SupplierCompanyID); err != nil {
			return err
		}
		x.Status, x.UpdatedAt = RFQNegotiating, s.clock()
		if err := st.Deals().UpdateRFQ(ctx, x, expected); err != nil {
			return err
		}
		raw, _ := json.Marshal(terms)
		q := &Quotation{ID: uuid.NewString(), RFQID: x.ID, Terms: raw, Digest: digest(raw), CreatedBy: p.UserID, CreatedAt: x.UpdatedAt}
		if err := st.Deals().AddQuotation(ctx, q); err != nil {
			return err
		}
		return s.event(ctx, st, p, "rfq.quoted", "rfq", x.ID, "", map[string]any{"quotationId": q.ID, "number": q.Number})
	})
	return x, err
}

// Accept accepts the exact latest quotation (identified by ID and digest) and
// creates the order in the same transaction. One quotation → one order.
func (s *DealService) Accept(ctx context.Context, p *auth.Principal, id string, expected int64, quotationID, quotationDigest string) (*Order, error) {
	var order *Order
	err := s.store.InTx(ctx, func(st Store) error {
		x, err := s.rfq(ctx, st, p, id, expected, "buyer")
		if err != nil {
			return err
		}
		if x.Status != RFQNegotiating {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "there is no quotation to accept")
		}
		qs, err := st.Deals().Quotations(ctx, x.ID)
		if err != nil {
			return err
		}
		latest := qs[len(qs)-1]
		if latest.ID != quotationID || latest.Digest != quotationDigest {
			return apperr.New(apperr.ErrConflict, "quotation_changed", "accept the latest quotation exactly as shown; reload")
		}
		if err := s.requirePartner(ctx, st, x.BuyerCompanyID, x.SupplierCompanyID); err != nil {
			return err
		}
		now := s.clock()
		x.Status, x.UpdatedAt = RFQAccepted, now
		if err := st.Deals().UpdateRFQ(ctx, x, expected); err != nil {
			return err
		}
		order = &Order{ID: uuid.NewString(), BuyerCompanyID: x.BuyerCompanyID, SupplierCompanyID: x.SupplierCompanyID,
			Source: "rfq", RFQID: &x.ID, QuotationID: &latest.ID, Terms: latest.Terms, Status: OrderAccepted,
			Version: 1, CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now}
		if err := st.Deals().CreateOrder(ctx, order); err != nil {
			return err
		}
		if err := s.event(ctx, st, p, "rfq.accepted", "rfq", x.ID, "", map[string]any{"quotationId": latest.ID, "orderId": order.ID}); err != nil {
			return err
		}
		return s.event(ctx, st, p, "order.created", "order", order.ID, "", map[string]any{"source": "rfq", "quotationId": latest.ID})
	})
	return order, err
}

// RFQView is an RFQ with its quotations.
type RFQView struct {
	RFQ        RFQ
	Quotations []Quotation
	Buyer      Company
	Supplier   Company
}

func (s *DealService) rfqView(ctx context.Context, x *RFQ) (*RFQView, error) {
	qs, err := s.store.Deals().Quotations(ctx, x.ID)
	if err != nil {
		return nil, err
	}
	buyer, err := s.directory.Company(ctx, x.BuyerCompanyID)
	if err != nil {
		return nil, err
	}
	supplier, err := s.directory.Company(ctx, x.SupplierCompanyID)
	if err != nil {
		return nil, err
	}
	return &RFQView{RFQ: *x, Quotations: qs, Buyer: *buyer, Supplier: *supplier}, nil
}

func (s *DealService) GetRFQ(ctx context.Context, p *auth.Principal, id string) (*RFQView, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	x, err := s.store.Deals().RFQ(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	return s.rfqView(ctx, x)
}

func (s *DealService) ListRFQs(ctx context.Context, p *auth.Principal, limit, offset int) ([]RFQView, error) {
	xs, err := s.store.Deals().RFQs(ctx, p.CompanyID, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]RFQView, 0, len(xs))
	for i := range xs {
		v, err := s.rfqView(ctx, &xs[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}

// DirectOrderInput orders from a published offer: offer lines and quantities.
type DirectOrderInput struct {
	OfferVersionID string `json:"offerVersionId"`
	Lines          []struct {
		OfferLineID string         `json:"offerLineId"`
		Quantity    jsonx.Quantity `json:"quantity"`
	} `json:"lines"`
}

// OrderFromOffer creates an order from the currently published offer version.
// It waits for the supplier's confirmation and grants no early access to VINs.
// The payment schedule is kept only when the whole offer is ordered;
// otherwise payments are agreed later by an addendum.
func (s *DealService) OrderFromOffer(ctx context.Context, p *auth.Principal, in DirectOrderInput) (*Order, error) {
	if uuid.Validate(in.OfferVersionID) != nil {
		return nil, apperr.FieldError("offerVersionId", "must be a valid ID")
	}
	var order *Order
	err := s.store.InTx(ctx, func(st Store) error {
		offer, err := st.Offers().VisibleByPublishedVersion(ctx, p.CompanyID, in.OfferVersionID)
		if errors.Is(err, apperr.ErrNotFound) {
			return apperr.FieldError("offerVersionId", "not the published version of an offer available to you")
		} else if err != nil {
			return err
		}
		version, err := st.Offers().Version(ctx, offer.ID, in.OfferVersionID)
		if err != nil {
			return err
		}
		offered, _ := version.decode()
		var v apperr.Validation
		if len(in.Lines) == 0 {
			v.Add("lines", "order at least one line")
		}
		chosen := map[string]jsonx.Quantity{}
		for i, l := range in.Lines {
			field := "lines." + strconv.Itoa(i)
			if _, dup := chosen[l.OfferLineID]; dup {
				v.Add(field+".offerLineId", "listed twice")
			}
			chosen[l.OfferLineID] = l.Quantity
		}
		terms := offered
		terms.Lines = []Line{}
		whole := len(chosen) == len(offered.Lines)
		for _, ol := range offered.Lines {
			q, ok := chosen[ol.LineID]
			if !ok {
				continue
			}
			delete(chosen, ol.LineID)
			if q < 1 || q > ol.Quantity {
				v.Add("lines", "quantity for line "+ol.LineID+" must be 1-"+strconv.Itoa(int(ol.Quantity)))
			}
			whole = whole && q == ol.Quantity
			ol.Quantity = q
			terms.Lines = append(terms.Lines, ol)
		}
		if len(chosen) > 0 {
			v.Add("lines", "contains lines that are not in the offer")
		}
		if err := v.Err(); err != nil {
			return err
		}
		if !whole {
			terms.PaymentSchedule = []Installment{}
		}
		if err := s.requirePartner(ctx, st, p.CompanyID, offer.SupplierCompanyID); err != nil {
			return err
		}
		raw, _ := json.Marshal(terms)
		now := s.clock()
		order = &Order{ID: uuid.NewString(), BuyerCompanyID: p.CompanyID, SupplierCompanyID: offer.SupplierCompanyID,
			Source: "offer", OfferVersionID: &version.ID, Terms: raw, Status: AwaitingSupplier, Version: 1,
			CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now}
		if err := st.Deals().CreateOrder(ctx, order); err != nil {
			return err
		}
		return s.event(ctx, st, p, "order.created", "order", order.ID, "", map[string]any{"source": "offer", "offerVersionId": version.ID})
	})
	return order, err
}

func (s *DealService) order(ctx context.Context, st Store, p *auth.Principal, id string, expected int64) (*Order, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	o, err := st.Deals().Order(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	if o.Version != expected {
		return nil, apperr.ErrStale
	}
	return o, nil
}

// ConfirmOrder: the supplier confirms a direct order (awaiting → accepted),
// or rejects it with a reason (→ cancelled).
func (s *DealService) ConfirmOrder(ctx context.Context, p *auth.Principal, id string, expected int64, confirm bool, why string) (*Order, error) {
	var v apperr.Validation
	if !confirm {
		why = validate.Reason(&v, why)
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var o *Order
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if o, err = s.order(ctx, st, p, id, expected); err != nil {
			return err
		}
		if o.Party(p.CompanyID) != "supplier" {
			return apperr.New(apperr.ErrForbidden, "wrong_party", "only the supplier can confirm")
		}
		if o.Status != AwaitingSupplier {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "the order is "+string(o.Status))
		}
		event := "order.confirmed"
		o.Status, o.StatusReason, o.UpdatedAt = OrderAccepted, "", s.clock()
		if !confirm {
			event = "order.rejected"
			o.Status, o.StatusReason = OrderCancelled, why
		}
		if err := st.Deals().UpdateOrder(ctx, o, expected); err != nil {
			return err
		}
		return s.event(ctx, st, p, event, "order", o.ID, why, nil)
	})
	return o, err
}

// CancelOrder lets either party cancel an order that has not started
// fulfilment. Fulfilment steps (allocation, payments) block cancellation.
func (s *DealService) CancelOrder(ctx context.Context, p *auth.Principal, id string, expected int64, why string) (*Order, error) {
	var v apperr.Validation
	why = validate.Reason(&v, why)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var o *Order
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if o, err = s.order(ctx, st, p, id, expected); err != nil {
			return err
		}
		if o.Status != AwaitingSupplier && o.Status != OrderAccepted {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "an order that is "+string(o.Status)+" cannot be cancelled")
		}
		if blocked, err := s.cancellationBlocked(ctx, st, o); err != nil {
			return err
		} else if blocked != "" {
			return apperr.New(apperr.ErrConflict, "cancellation_blocked", blocked)
		}
		o.Status, o.StatusReason, o.UpdatedAt = OrderCancelled, why, s.clock()
		if err := st.Deals().UpdateOrder(ctx, o, expected); err != nil {
			return err
		}
		// Cancellation is final only together with the exact inventory release.
		if err := s.stock.Release(st.Bind(ctx), o.ID, nil, "order cancelled: "+why); err != nil {
			return err
		}
		if err := st.Fulfilment().SetAllocationStatus(ctx, o.ID, nil, "released", nil); err != nil {
			return err
		}
		return s.event(ctx, st, p, "order.cancelled", "order", o.ID, why, nil)
	})
	return o, err
}

// cancellationBlocked is extended by fulfilment features (allocations, payments).
var cancellationChecks []func(ctx context.Context, st Store, o *Order) (string, error)

func (s *DealService) cancellationBlocked(ctx context.Context, st Store, o *Order) (string, error) {
	for _, check := range cancellationChecks {
		if reason, err := check(ctx, st, o); err != nil || reason != "" {
			return reason, err
		}
	}
	return "", nil
}

// ProposeAddendum proposes new terms for an accepted order. Only one proposal
// can be open; the other party accepts or rejects it.
func (s *DealService) ProposeAddendum(ctx context.Context, p *auth.Principal, id string, expected int64, in Terms, why string) (*Order, error) {
	var v apperr.Validation
	terms, _ := s.validateTerms(ctx, &v, in)
	why = validate.Reason(&v, why)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var o *Order
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if o, err = s.order(ctx, st, p, id, expected); err != nil {
			return err
		}
		if o.Status != OrderAccepted && o.Status != OrderFulfilling {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "addenda apply to accepted orders")
		}
		raw, _ := json.Marshal(terms)
		a := &Addendum{ID: uuid.NewString(), OrderID: o.ID, Terms: raw, Reason: why, ProposedByCompany: p.CompanyID,
			Status: "proposed", CreatedAt: s.clock()}
		if err := st.Deals().AddAddendum(ctx, a); err != nil {
			if errors.Is(err, apperr.ErrConflict) {
				return apperr.New(apperr.ErrConflict, "addendum_open", "another addendum is waiting for a decision")
			}
			return err
		}
		o.UpdatedAt = a.CreatedAt
		if err := st.Deals().UpdateOrder(ctx, o, expected); err != nil {
			return err
		}
		return s.event(ctx, st, p, "order.addendum_proposed", "order", o.ID, why, map[string]any{"addendumId": a.ID, "number": a.Number})
	})
	return o, err
}

// DecideAddendum: the other party accepts (the order terms change) or
// rejects (with a reason) the open proposal.
func (s *DealService) DecideAddendum(ctx context.Context, p *auth.Principal, id, addendumID string, expected int64, accept bool, why string) (*Order, error) {
	var v apperr.Validation
	if !accept {
		why = validate.Reason(&v, why)
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var o *Order
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if o, err = s.order(ctx, st, p, id, expected); err != nil {
			return err
		}
		as, err := st.Deals().Addenda(ctx, o.ID)
		if err != nil {
			return err
		}
		var a *Addendum
		for i := range as {
			if as[i].ID == addendumID {
				a = &as[i]
			}
		}
		if a == nil {
			return apperr.ErrNotFound
		}
		if a.ProposedByCompany == p.CompanyID {
			return apperr.New(apperr.ErrForbidden, "wrong_party", "the other party decides on an addendum")
		}
		if a.Status != "proposed" {
			return apperr.New(apperr.ErrConflict, "addendum_decided", "the addendum was already decided")
		}
		now := s.clock()
		a.Status, a.DecisionReason, a.DecidedAt = "rejected", why, &now
		if accept {
			a.Status = "accepted"
			o.Terms = a.Terms
		}
		if err := st.Deals().DecideAddendum(ctx, a); err != nil {
			return err
		}
		o.UpdatedAt = now
		if err := st.Deals().UpdateOrder(ctx, o, expected); err != nil {
			return err
		}
		return s.event(ctx, st, p, "order.addendum_"+a.Status, "order", o.ID, why, map[string]any{"addendumId": a.ID})
	})
	return o, err
}

// OrderView is an order with its addenda and both parties.
type OrderView struct {
	Order       Order
	Addenda     []Addendum
	Allocations []Allocation
	Shipments   []Shipment
	Buyer       Company
	Supplier    Company
	Events      []Event
}

func (s *DealService) orderView(ctx context.Context, o *Order, withHistory bool) (*OrderView, error) {
	as, err := s.store.Deals().Addenda(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	buyer, err := s.directory.Company(ctx, o.BuyerCompanyID)
	if err != nil {
		return nil, err
	}
	supplier, err := s.directory.Company(ctx, o.SupplierCompanyID)
	if err != nil {
		return nil, err
	}
	v := &OrderView{Order: *o, Addenda: as, Buyer: *buyer, Supplier: *supplier}
	if v.Allocations, err = s.store.Fulfilment().Allocations(ctx, o.ID); err != nil {
		return nil, err
	}
	if v.Shipments, err = s.store.Fulfilment().Shipments(ctx, o.ID); err != nil {
		return nil, err
	}
	if withHistory {
		if v.Events, err = s.store.Events().ForResource(ctx, "order", o.ID); err != nil {
			return nil, err
		}
	}
	return v, nil
}

func (s *DealService) GetOrder(ctx context.Context, p *auth.Principal, id string) (*OrderView, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	o, err := s.store.Deals().Order(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	return s.orderView(ctx, o, true)
}

func (s *DealService) ListOrders(ctx context.Context, p *auth.Principal, limit, offset int) ([]OrderView, error) {
	os, err := s.store.Deals().Orders(ctx, p.CompanyID, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]OrderView, 0, len(os))
	for i := range os {
		v, err := s.orderView(ctx, &os[i], false)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}
