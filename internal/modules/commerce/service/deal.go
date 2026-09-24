package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/google/uuid"

	"justixauto/internal/modules/commerce/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/jsonx"
	"justixauto/internal/pkg/validate"
)

// Deal manages RFQs, quotations, orders and addenda.
type Deal struct {
	Deps
	offers *Offer
}

func (s *Deal) requirePartner(ctx context.Context, st Store, a, b string) error {
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
	SupplierCompanyID string          `json:"supplierCompanyId"`
	OfferVersionID    *string         `json:"offerVersionId"`
	Lines             []model.RFQLine `json:"lines"`
}

func (s *Deal) validateRFQLines(ctx context.Context, v *apperr.Validation, in []model.RFQLine) []model.RFQLine {
	out := []model.RFQLine{}
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
func (s *Deal) CreateRFQ(ctx context.Context, p *auth.Principal, in RFQInput) (*model.RFQ, error) {
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
	x := &model.RFQ{
		ID: uuid.NewString(), BuyerCompanyID: p.CompanyID, SupplierCompanyID: in.SupplierCompanyID,
		OfferVersionID: in.OfferVersionID, Lines: raw, Status: model.RFQDraft, Version: 1, CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now,
	}
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

func (s *Deal) rfq(ctx context.Context, st Store, p *auth.Principal, id string, expected int64, party string) (*model.RFQ, error) {
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
func (s *Deal) RFQAction(ctx context.Context, p *auth.Principal, id string, expected int64, action, why string) (*model.RFQ, error) {
	type rule struct {
		party  string
		from   []model.RFQStatus
		to     model.RFQStatus
		reason bool
	}
	rules := map[string]rule{
		"send":    {"buyer", []model.RFQStatus{model.RFQDraft}, model.RFQSent, false},
		"cancel":  {"buyer", []model.RFQStatus{model.RFQDraft, model.RFQSent, model.RFQNegotiating}, model.RFQCancelled, true},
		"decline": {"supplier", []model.RFQStatus{model.RFQSent, model.RFQNegotiating}, model.RFQDeclined, true},
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
	var x *model.RFQ
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
func (s *Deal) Quote(ctx context.Context, p *auth.Principal, id string, expected int64, in model.Terms) (*model.RFQ, error) {
	var v apperr.Validation
	terms := s.validateTerms(ctx, &v, in)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var x *model.RFQ
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if x, err = s.rfq(ctx, st, p, id, expected, "supplier"); err != nil {
			return err
		}
		if x.Status != model.RFQSent && x.Status != model.RFQNegotiating {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "cannot quote an RFQ that is "+string(x.Status))
		}
		if err := s.requirePartner(ctx, st, x.BuyerCompanyID, x.SupplierCompanyID); err != nil {
			return err
		}
		x.Status, x.UpdatedAt = model.RFQNegotiating, s.clock()
		if err := st.Deals().UpdateRFQ(ctx, x, expected); err != nil {
			return err
		}
		raw, _ := json.Marshal(terms)
		q := &model.Quotation{ID: uuid.NewString(), RFQID: x.ID, Terms: raw, Digest: model.Digest(raw), CreatedBy: p.UserID, CreatedAt: x.UpdatedAt}
		if err := st.Deals().AddQuotation(ctx, q); err != nil {
			return err
		}
		return s.event(ctx, st, p, "rfq.quoted", "rfq", x.ID, "", map[string]any{"quotationId": q.ID, "number": q.Number})
	})
	return x, err
}

// Accept accepts the exact latest quotation (identified by ID and digest) and
// creates the order in the same transaction. One quotation → one order.
func (s *Deal) Accept(ctx context.Context, p *auth.Principal, id string, expected int64, quotationID, quotationDigest string) (*model.Order, error) {
	var order *model.Order
	err := s.store.InTx(ctx, func(st Store) error {
		x, err := s.rfq(ctx, st, p, id, expected, "buyer")
		if err != nil {
			return err
		}
		if x.Status != model.RFQNegotiating {
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
		x.Status, x.UpdatedAt = model.RFQAccepted, now
		if err := st.Deals().UpdateRFQ(ctx, x, expected); err != nil {
			return err
		}
		order = &model.Order{
			ID: uuid.NewString(), BuyerCompanyID: x.BuyerCompanyID, SupplierCompanyID: x.SupplierCompanyID,
			Source: "rfq", RFQID: &x.ID, QuotationID: &latest.ID, Terms: latest.Terms, Status: model.OrderAccepted,
			Version: 1, CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now,
		}
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
	RFQ        model.RFQ
	Quotations []model.Quotation
	Buyer      Company
	Supplier   Company
}

func (s *Deal) RFQView(ctx context.Context, x *model.RFQ) (*RFQView, error) {
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

func (s *Deal) GetRFQ(ctx context.Context, p *auth.Principal, id string) (*RFQView, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	x, err := s.store.Deals().RFQ(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	return s.RFQView(ctx, x)
}

func (s *Deal) ListRFQs(ctx context.Context, p *auth.Principal, limit, offset int) ([]RFQView, error) {
	xs, err := s.store.Deals().RFQs(ctx, p.CompanyID, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]RFQView, 0, len(xs))
	for i := range xs {
		v, err := s.RFQView(ctx, &xs[i])
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

// selectOrderLines validates the requested offer lines/quantities against
// the offered terms and returns the resulting order terms. The payment
// schedule is dropped unless the whole offer (every line, full quantity) was
// ordered.
func selectOrderLines(v *apperr.Validation, offered model.Terms, in []struct {
	OfferLineID string         `json:"offerLineId"`
	Quantity    jsonx.Quantity `json:"quantity"`
},
) (model.Terms, error) {
	if len(in) == 0 {
		v.Add("lines", "order at least one line")
	}
	chosen := map[string]jsonx.Quantity{}
	for i, l := range in {
		field := "lines." + strconv.Itoa(i)
		if _, dup := chosen[l.OfferLineID]; dup {
			v.Add(field+".offerLineId", "listed twice")
		}
		chosen[l.OfferLineID] = l.Quantity
	}
	terms := offered
	terms.Lines = []model.Line{}
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
		return model.Terms{}, err
	}
	if !whole {
		terms.PaymentSchedule = []model.Installment{}
	}
	return terms, nil
}

// OrderFromOffer creates an order from the currently published offer version.
// It waits for the supplier's confirmation and grants no early access to VINs.
// The payment schedule is kept only when the whole offer is ordered;
// otherwise payments are agreed later by an addendum.
func (s *Deal) OrderFromOffer(ctx context.Context, p *auth.Principal, in DirectOrderInput) (*model.Order, error) {
	if uuid.Validate(in.OfferVersionID) != nil {
		return nil, apperr.FieldError("offerVersionId", "must be a valid ID")
	}
	var order *model.Order
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
		offered, _ := version.Decode()
		var v apperr.Validation
		terms, err := selectOrderLines(&v, offered, in.Lines)
		if err != nil {
			return err
		}
		if err := s.requirePartner(ctx, st, p.CompanyID, offer.SupplierCompanyID); err != nil {
			return err
		}
		raw, _ := json.Marshal(terms)
		now := s.clock()
		order = &model.Order{
			ID: uuid.NewString(), BuyerCompanyID: p.CompanyID, SupplierCompanyID: offer.SupplierCompanyID,
			Source: "offer", OfferVersionID: &version.ID, Terms: raw, Status: model.AwaitingSupplier, Version: 1,
			CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now,
		}
		if err := st.Deals().CreateOrder(ctx, order); err != nil {
			return err
		}
		return s.event(ctx, st, p, "order.created", "order", order.ID, "", map[string]any{"source": "offer", "offerVersionId": version.ID})
	})
	return order, err
}

func (s *Deal) order(ctx context.Context, st Store, p *auth.Principal, id string, expected int64) (*model.Order, error) {
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
func (s *Deal) ConfirmOrder(ctx context.Context, p *auth.Principal, id string, expected int64, confirm bool, why string) (*model.Order, error) {
	var v apperr.Validation
	if !confirm {
		why = validate.Reason(&v, why)
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var o *model.Order
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if o, err = s.order(ctx, st, p, id, expected); err != nil {
			return err
		}
		if o.Party(p.CompanyID) != "supplier" {
			return apperr.New(apperr.ErrForbidden, "wrong_party", "only the supplier can confirm")
		}
		if o.Status != model.AwaitingSupplier {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "the order is "+string(o.Status))
		}
		event := "order.confirmed"
		o.Status, o.StatusReason, o.UpdatedAt = model.OrderAccepted, "", s.clock()
		if !confirm {
			event = "order.rejected"
			o.Status, o.StatusReason = model.OrderCancelled, why
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
func (s *Deal) CancelOrder(ctx context.Context, p *auth.Principal, id string, expected int64, why string) (*model.Order, error) {
	var v apperr.Validation
	why = validate.Reason(&v, why)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var o *model.Order
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if o, err = s.order(ctx, st, p, id, expected); err != nil {
			return err
		}
		if o.Status != model.AwaitingSupplier && o.Status != model.OrderAccepted {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "an order that is "+string(o.Status)+" cannot be cancelled")
		}
		if blocked, err := s.cancellationBlocked(ctx, st, o); err != nil {
			return err
		} else if blocked != "" {
			return apperr.New(apperr.ErrConflict, "cancellation_blocked", blocked)
		}
		o.Status, o.StatusReason, o.UpdatedAt = model.OrderCancelled, why, s.clock()
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

// cancellationChecks is extended by fulfilment features (allocations, payments).
var cancellationChecks []func(ctx context.Context, st Store, o *model.Order) (string, error)

func (s *Deal) cancellationBlocked(ctx context.Context, st Store, o *model.Order) (string, error) {
	for _, check := range cancellationChecks {
		if reason, err := check(ctx, st, o); err != nil || reason != "" {
			return reason, err
		}
	}
	return "", nil
}

// ProposeAddendum proposes new terms for an accepted order. Only one proposal
// can be open; the other party accepts or rejects it.
func (s *Deal) ProposeAddendum(ctx context.Context, p *auth.Principal, id string, expected int64, in model.Terms, why string) (*model.Order, error) {
	var v apperr.Validation
	terms := s.validateTerms(ctx, &v, in)
	why = validate.Reason(&v, why)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var o *model.Order
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if o, err = s.order(ctx, st, p, id, expected); err != nil {
			return err
		}
		if o.Status != model.OrderAccepted && o.Status != model.OrderFulfilling {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "addenda apply to accepted orders")
		}
		raw, _ := json.Marshal(terms)
		a := &model.Addendum{
			ID: uuid.NewString(), OrderID: o.ID, Terms: raw, Reason: why, ProposedByCompany: p.CompanyID,
			Status: "proposed", CreatedAt: s.clock(),
		}
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
func (s *Deal) DecideAddendum(ctx context.Context, p *auth.Principal, id, addendumID string, expected int64, accept bool, why string) (*model.Order, error) {
	var v apperr.Validation
	if !accept {
		why = validate.Reason(&v, why)
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var o *model.Order
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if o, err = s.order(ctx, st, p, id, expected); err != nil {
			return err
		}
		as, err := st.Deals().Addenda(ctx, o.ID)
		if err != nil {
			return err
		}
		var a *model.Addendum
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
	Order       model.Order
	Addenda     []model.Addendum
	Allocations []model.Allocation
	Shipments   []model.Shipment
	Buyer       Company
	Supplier    Company
	Events      []model.Event
}

func (s *Deal) OrderView(ctx context.Context, o *model.Order, withHistory bool) (*OrderView, error) {
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

func (s *Deal) GetOrder(ctx context.Context, p *auth.Principal, id string) (*OrderView, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	o, err := s.store.Deals().Order(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	return s.OrderView(ctx, o, true)
}

func (s *Deal) ListOrders(ctx context.Context, p *auth.Principal, limit, offset int) ([]OrderView, error) {
	os, err := s.store.Deals().Orders(ctx, p.CompanyID, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]OrderView, 0, len(os))
	for i := range os {
		v, err := s.OrderView(ctx, &os[i], false)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}
