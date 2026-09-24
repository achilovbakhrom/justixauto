package service

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"slices"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/modules/retail/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/money"
	"justixauto/internal/pkg/validate"
)

// Deal manages retail sales: reservation, contract, invoices, payment
// evidence, registration and delivery.
type Deal struct {
	Deps
	insurance Insurance
}

type DealInput struct {
	CustomerID    string      `json:"customerId"`
	LeadID        *string     `json:"leadId"`
	VehicleID     string      `json:"vehicleId"`
	BranchID      string      `json:"branchId"`
	PaymentScheme string      `json:"paymentScheme"`
	Price         money.Money `json:"price"`
}

// validateDealIDs validates the referenced IDs of a Create call.
func validateDealIDs(v *apperr.Validation, in DealInput) {
	for field, id := range map[string]string{"customerId": in.CustomerID, "vehicleId": in.VehicleID, "branchId": in.BranchID} {
		if validate.IDs(id) != nil {
			v.Add(field, "must be a valid ID")
		}
	}
}

// resolveDealLead loads and validates the optional lead a Create call links
// to the new deal: it must belong to the same customer, be open, qualified
// and not already tied to another active sale.
func resolveDealLead(ctx context.Context, st Store, p *auth.Principal, customerID string, leadID *string) (*model.Lead, error) {
	if leadID == nil {
		return nil, nil
	}
	l, err := st.CRM().Lead(ctx, p.CompanyID, *leadID)
	if err != nil || l.CustomerID != customerID {
		return nil, apperr.FieldError("leadId", "not a lead of this customer")
	}
	if !l.Open() || !l.Qualified() || l.DealID != nil {
		return nil, apperr.New(apperr.ErrConflict, "lead_not_eligible", "the lead must be open, qualified and without an active sale")
	}
	return l, nil
}

// checkDealVehicle rejects Create when the vehicle is not one the company
// owns.
func (s *Deal) checkDealVehicle(ctx context.Context, st Store, p *auth.Principal, vehicleID string) error {
	vehicle, err := s.stock.Vehicle(st.Bind(ctx), p.CompanyID, vehicleID)
	if err != nil || !vehicle.Owned {
		return apperr.FieldError("vehicleId", "not a vehicle your company owns")
	}
	return nil
}

// Create starts a sale: the vehicle is reserved at once (one active sale per
// VIN across wholesale and retail) and an optional qualified lead is linked.
func (s *Deal) Create(ctx context.Context, p *auth.Principal, in DealInput) (*model.Deal, error) {
	var v apperr.Validation
	if !slices.Contains(model.Schemes, in.PaymentScheme) {
		v.Add("paymentScheme", "must be cash, own-installment or partner-finance")
	}
	price(&v, "price", in.Price)
	validateDealIDs(&v, in)
	if err := v.Err(); err != nil {
		return nil, err
	}
	if err := s.branch(ctx, p, "branchId", in.BranchID); err != nil {
		return nil, err
	}
	now := s.clock()
	d := &model.Deal{
		ID: uuid.NewString(), ContractFileIDs: []byte("[]"), CompanyID: p.CompanyID, BranchID: in.BranchID, CustomerID: in.CustomerID, LeadID: in.LeadID,
		VehicleID: in.VehicleID, PaymentScheme: in.PaymentScheme, PriceMinor: in.Price.AmountMinor, Currency: in.Price.Currency,
		Status: "reserved", Version: 1, CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now,
	}
	err := s.store.InTx(ctx, func(st Store) error {
		if _, err := st.CRM().Customer(ctx, p.CompanyID, in.CustomerID); err != nil {
			return apperr.FieldError("customerId", "unknown customer")
		}
		lead, err := resolveDealLead(ctx, st, p, in.CustomerID, in.LeadID)
		if err != nil {
			return err
		}
		if err := s.checkDealVehicle(ctx, st, p, in.VehicleID); err != nil {
			return err
		}
		if err := st.Deals().Create(ctx, d); err != nil {
			if errors.Is(err, apperr.ErrConflict) {
				return apperr.New(apperr.ErrConflict, "vehicle_unavailable", "the vehicle or lead already has an active sale")
			}
			return err
		}
		if err := s.stock.Reserve(st.Bind(ctx), p.CompanyID, d.ID, d.VehicleID); err != nil {
			return err
		}
		if lead != nil {
			lead.DealID, lead.UpdatedAt = &d.ID, now
			if err := st.CRM().UpdateLead(ctx, lead, lead.Version); err != nil {
				return err
			}
		}
		return s.event(ctx, st, p, "deal.reserved", "deal", d.ID, "", map[string]any{"vehicleId": d.VehicleID, "scheme": d.PaymentScheme})
	})
	return d, err
}

func (s *Deal) deal(ctx context.Context, st Store, p *auth.Principal, id string, expected int64) (*model.Deal, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	d, err := st.Deals().Deal(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	if !inScope(p, d.BranchID) {
		return nil, apperr.ErrNotFound
	}
	if expected >= 0 && d.Version != expected {
		return nil, apperr.ErrStale
	}
	if expected >= 0 && d.Status != "reserved" {
		return nil, apperr.New(apperr.ErrConflict, "deal_closed", "the sale is "+d.Status)
	}
	return d, nil
}

func date(v *apperr.Validation, field, s string, now time.Time) *time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil || t.After(now) {
		v.Add(field, "a date (YYYY-MM-DD), not in the future")
		return nil
	}
	return &t
}

// RecordContract stores the external contract fact (signed date, reference)
// and the uploaded scans; recording again replaces the previous record.
func (s *Deal) RecordContract(ctx context.Context, p *auth.Principal, id string, expected int64, signedOn, reference string, fileIDs []string) (*model.Deal, error) {
	var files []string
	return s.update(ctx, p, id, expected, "deal.contract_recorded", func(v *apperr.Validation, d *model.Deal) {
		d.ContractSignedOn = date(v, "signedOn", signedOn, s.clock())
		d.ContractReference = validate.Text(v, "reference", reference, 1, 100)
		files = validate.UniqueIDs(v, "bindingIds", fileIDs)
		if len(files) > 10 {
			v.Add("bindingIds", "at most 10 files")
		}
		d.ContractFileIDs, _ = json.Marshal(files)
	}, func(ctx context.Context, st Store, d *model.Deal) error {
		for _, f := range files { // must be the company's own files
			if err := s.files.Share(st.Bind(ctx), p.CompanyID, f, p.CompanyID, "retail.contract", d.ID); err != nil {
				if errors.Is(err, apperr.ErrNotFound) {
					return apperr.FieldError("bindingIds", "contains files that are not yours")
				}
				return err
			}
		}
		return nil
	})
}

// RecordRegistration stores the vehicle registration facts; the registration
// invoice must be paid first.
func (s *Deal) RecordRegistration(ctx context.Context, p *auth.Principal, id string, expected int64, registeredOn, plate, reference string) (*model.Deal, error) {
	return s.update(ctx, p, id, expected, "deal.registered", func(v *apperr.Validation, d *model.Deal) {
		d.RegisteredOn = date(v, "registeredOn", registeredOn, s.clock())
		d.PlateNumber = validate.Text(v, "plateNumber", plate, 1, 20)
		d.RegistrationReference = validate.Text(v, "reference", reference, 0, 100)
	}, func(ctx context.Context, st Store, d *model.Deal) error {
		paid, err := s.paid(ctx, st, d, "registration")
		if err != nil {
			return err
		}
		if !paid {
			return apperr.New(apperr.ErrConflict, "registration_unpaid", "the registration invoice must be paid first")
		}
		return nil
	})
}

func (s *Deal) update(ctx context.Context, p *auth.Principal, id string, expected int64, event string,
	apply func(*apperr.Validation, *model.Deal), checks ...func(context.Context, Store, *model.Deal) error,
) (*model.Deal, error) {
	var d *model.Deal
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if d, err = s.deal(ctx, st, p, id, expected); err != nil {
			return err
		}
		var v apperr.Validation
		apply(&v, d)
		if err := v.Err(); err != nil {
			return err
		}
		for _, check := range checks {
			if err := check(ctx, st, d); err != nil {
				return err
			}
		}
		d.UpdatedAt = s.clock()
		if err := st.Deals().Update(ctx, d, expected); err != nil {
			return err
		}
		return s.event(ctx, st, p, event, "deal", d.ID, "", nil)
	})
	return d, err
}

// paid reports whether the deal's issued invoice for purpose is fully paid
// by accepted evidence.
func (s *Deal) paid(ctx context.Context, st Store, d *model.Deal, purpose string) (bool, error) {
	is, err := st.Deals().Invoices(ctx, d.ID)
	if err != nil {
		return false, err
	}
	for _, i := range is {
		if i.Purpose != purpose || i.Status != "issued" {
			continue
		}
		v, err := s.invoiceView(ctx, st, &i)
		if err != nil {
			return false, err
		}
		return v.Outstanding.AmountMinor == "0", nil
	}
	return false, nil
}

type InvoiceInput struct {
	Purpose           string      `json:"purpose"`
	Amount            money.Money `json:"amount"`
	RecipientSnapshot string      `json:"recipientSnapshot"`
	DueDate           string      `json:"dueDate"`
}

// IssueInvoice records a deal invoice for a purpose the payment scheme uses.
// Amounts and recipients are entered by the seller; Justix does not infer
// registration fees or legal obligations.
func (s *Deal) IssueInvoice(ctx context.Context, p *auth.Principal, dealID string, expected int64, in InvoiceInput) (*InvoiceView, error) {
	var v apperr.Validation
	price(&v, "amount", in.Amount)
	recipient := validate.Text(&v, "recipientSnapshot", in.RecipientSnapshot, 1, 500)
	var due *time.Time
	if in.DueDate != "" {
		t, err := time.Parse(time.DateOnly, in.DueDate)
		if err != nil {
			v.Add("dueDate", "a date (YYYY-MM-DD)")
		}
		due = &t
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *InvoiceView
	err := s.store.InTx(ctx, func(st Store) error {
		d, err := s.deal(ctx, st, p, dealID, expected)
		if err != nil {
			return err
		}
		if !slices.Contains(model.Purposes[d.PaymentScheme], in.Purpose) {
			return apperr.FieldError("purpose", "not used by a "+d.PaymentScheme+" sale")
		}
		i := &model.Invoice{
			ID: uuid.NewString(), DealID: d.ID, CompanyID: d.CompanyID, Purpose: in.Purpose, AmountMinor: in.Amount.AmountMinor,
			Currency: in.Amount.Currency, RecipientSnapshot: recipient, DueDate: due, Status: "issued", Version: 1, CreatedAt: s.clock(),
		}
		if err := st.Deals().CreateInvoice(ctx, i); err != nil {
			if errors.Is(err, apperr.ErrConflict) {
				return apperr.New(apperr.ErrConflict, "invoice_exists", "an invoice for this purpose already exists")
			}
			return err
		}
		d.UpdatedAt = i.CreatedAt
		if err := st.Deals().Update(ctx, d, expected); err != nil {
			return err
		}
		if err := s.event(ctx, st, p, "deal.invoice_issued", "deal", d.ID, "", map[string]any{"invoiceId": i.ID, "purpose": i.Purpose}); err != nil {
			return err
		}
		result, err = s.invoiceView(ctx, st, i)
		return err
	})
	return result, err
}

type InvoiceView struct {
	Invoice     model.Invoice
	Evidence    []model.Evidence
	Paid        money.Money
	Pending     money.Money
	Outstanding money.Money
}

func amount(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return new(big.Int)
	}
	return n
}

func (s *Deal) invoiceView(ctx context.Context, st Store, i *model.Invoice) (*InvoiceView, error) {
	es, err := st.Deals().Evidence(ctx, i.ID)
	if err != nil {
		return nil, err
	}
	paid, pending := new(big.Int), new(big.Int)
	for _, e := range es {
		switch e.Status {
		case "accepted":
			paid.Add(paid, amount(e.AmountMinor))
		case "submitted":
			pending.Add(pending, amount(e.AmountMinor))
		}
	}
	out := new(big.Int).Sub(amount(i.AmountMinor), paid)
	if out.Sign() < 0 {
		out.SetInt64(0)
	}
	return &InvoiceView{
		Invoice: *i, Evidence: es, Paid: money.Of(paid, i.Currency), Pending: money.Of(pending, i.Currency),
		Outstanding: money.Of(out, i.Currency),
	}, nil
}

type EvidenceInput struct {
	ClaimedAmount     money.Money `json:"claimedAmount"`
	PaidOn            string      `json:"paidOn"`
	ExternalReference string      `json:"externalReference"`
	AttachmentIDs     []string    `json:"attachmentBindingIds"`
}

// checkEvidenceInvoice loads the invoice for a SubmitEvidence call and
// validates it is still open, in the claim's currency and that the claim
// does not exceed the amount still open (net of evidence already pending).
func (s *Deal) checkEvidenceInvoice(ctx context.Context, st Store, p *auth.Principal, invoiceID string, in EvidenceInput, claimed *big.Int) (*model.Invoice, error) {
	if err := validate.IDs(invoiceID); err != nil {
		return nil, err
	}
	i, err := st.Deals().Invoice(ctx, p.CompanyID, invoiceID)
	if err != nil {
		return nil, err
	}
	if _, err := s.deal(ctx, st, p, i.DealID, -1); err != nil {
		return nil, err
	}
	if i.Status != "issued" {
		return nil, apperr.New(apperr.ErrConflict, "invoice_void", "the invoice was voided")
	}
	if in.ClaimedAmount.Currency != i.Currency {
		return nil, apperr.FieldError("claimedAmount", "must be in "+i.Currency)
	}
	view, err := s.invoiceView(ctx, st, i)
	if err != nil {
		return nil, err
	}
	if claimed.Cmp(new(big.Int).Sub(amount(view.Outstanding.AmountMinor), amount(view.Pending.AmountMinor))) > 0 {
		return nil, apperr.FieldError("claimedAmount", "exceeds the open amount")
	}
	return i, nil
}

// shareEvidenceAttachments shares each attachment (which must be the
// company's own file) with itself, scoped to the evidence record.
func (s *Deal) shareEvidenceAttachments(ctx context.Context, st Store, p *auth.Principal, attachments []string, evidenceID string) error {
	for _, f := range attachments {
		if err := s.files.Share(st.Bind(ctx), p.CompanyID, f, p.CompanyID, "retail.payment-evidence", evidenceID); err != nil {
			if errors.Is(err, apperr.ErrNotFound) {
				return apperr.FieldError("attachmentBindingIds", "contains files that are not yours")
			}
			return err
		}
	}
	return nil
}

// SubmitEvidence records the customer's external payment as reported to the
// seller's staff; it is reviewed separately.
func (s *Deal) SubmitEvidence(ctx context.Context, p *auth.Principal, invoiceID string, in EvidenceInput) (*InvoiceView, error) {
	var v apperr.Validation
	claimed, ok := in.ClaimedAmount.Parse()
	if !ok || claimed.Sign() == 0 {
		v.Add("claimedAmount", "must be a positive amount in minor units with a currency code")
	}
	paidOn := date(&v, "paidOn", in.PaidOn, s.clock())
	ref := validate.Text(&v, "externalReference", in.ExternalReference, 1, 100)
	attachments := validate.UniqueIDs(&v, "attachmentBindingIds", in.AttachmentIDs)
	if len(attachments) > 10 {
		v.Add("attachmentBindingIds", "at most 10 files")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *InvoiceView
	err := s.store.InTx(ctx, func(st Store) error {
		i, err := s.checkEvidenceInvoice(ctx, st, p, invoiceID, in, claimed)
		if err != nil {
			return err
		}
		rawIDs, _ := json.Marshal(attachments)
		e := &model.Evidence{
			ID: uuid.NewString(), InvoiceID: i.ID, AmountMinor: claimed.String(), Currency: i.Currency, PaidOn: *paidOn,
			ExternalReference: ref, AttachmentIDs: rawIDs, Status: "submitted", SubmittedBy: p.UserID, Version: 1, CreatedAt: s.clock(),
		}
		if err := s.shareEvidenceAttachments(ctx, st, p, attachments, e.ID); err != nil { // must be the company's own files
			return err
		}
		if err := st.Deals().AddEvidence(ctx, e); err != nil {
			return err
		}
		if err := s.event(ctx, st, p, "deal.payment_submitted", "deal", i.DealID, "", map[string]any{"evidenceId": e.ID}); err != nil {
			return err
		}
		result, err = s.invoiceView(ctx, st, i)
		return err
	})
	return result, err
}

// DecideEvidence is the factual review of a payment claim: accept (with
// confirmation) or reject (with a reason). No schedule allocation is implied.
func (s *Deal) DecideEvidence(ctx context.Context, p *auth.Principal, id string, expected int64, accept, confirmation bool, why string) (*InvoiceView, error) {
	var v apperr.Validation
	if accept && !confirmation {
		v.Add("confirmation", "confirm that the payment was received")
	}
	if !accept {
		why = validate.Reason(&v, why)
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *InvoiceView
	err := s.store.InTx(ctx, func(st Store) error {
		if err := validate.IDs(id); err != nil {
			return err
		}
		e, err := st.Deals().GetEvidence(ctx, id)
		if err != nil {
			return err
		}
		i, err := st.Deals().Invoice(ctx, p.CompanyID, e.InvoiceID)
		if err != nil {
			return err
		}
		if _, err := s.deal(ctx, st, p, i.DealID, -1); err != nil {
			return err
		}
		if e.Version != expected {
			return apperr.ErrStale
		}
		if e.Status != "submitted" {
			return apperr.New(apperr.ErrConflict, "evidence_decided", "the evidence was already decided")
		}
		now := s.clock()
		e.Status, e.DecisionReason, e.DecidedBy, e.DecidedAt = "rejected", why, &p.UserID, &now
		if accept {
			e.Status = "accepted"
		}
		if err := st.Deals().UpdateEvidence(ctx, e, expected); err != nil {
			return err
		}
		if err := s.event(ctx, st, p, "deal.payment_"+e.Status, "deal", i.DealID, why, map[string]any{"evidenceId": e.ID}); err != nil {
			return err
		}
		result, err = s.invoiceView(ctx, st, i)
		return err
	})
	return result, err
}

// Checklist shows which delivery prerequisites are met.
type Checklist struct {
	Contract          bool  `json:"contract"`
	VehiclePayment    *bool `json:"vehiclePayment,omitempty"`
	InsuranceApproved *bool `json:"insuranceApproved,omitempty"`
	FirstInstallment  *bool `json:"firstInstallment,omitempty"`
	RegistrationPaid  bool  `json:"registrationPaid"`
	Registered        bool  `json:"registered"`
	PolicyResolved    bool  `json:"policyResolved"`
}

func (c Checklist) Ready() bool {
	ok := func(b *bool) bool { return b == nil || *b }
	return c.PolicyResolved && c.Contract && ok(c.VehiclePayment) && ok(c.InsuranceApproved) && ok(c.FirstInstallment) && c.RegistrationPaid && c.Registered
}

func (s *Deal) checklist(ctx context.Context, st Store, d *model.Deal) (Checklist, error) {
	c := Checklist{Contract: d.ContractSignedOn != nil, Registered: d.RegisteredOn != nil, PolicyResolved: d.PaymentScheme != "partner-finance"}
	var err error
	if c.RegistrationPaid, err = s.paid(ctx, st, d, "registration"); err != nil {
		return c, err
	}
	switch d.PaymentScheme {
	case "cash":
		paid, err := s.paid(ctx, st, d, "vehicle-payment")
		if err != nil {
			return c, err
		}
		c.VehiclePayment = &paid
	case "own-installment":
		approved, err := s.insurance.Approved(st.Bind(ctx), d.CompanyID, d.ID)
		if err != nil {
			return c, err
		}
		paid, err := s.paid(ctx, st, d, "first-installment")
		if err != nil {
			return c, err
		}
		c.InsuranceApproved, c.FirstInstallment = &approved, &paid
	}
	return c, nil
}

// Deliver hands the vehicle to the customer once every prerequisite of the
// payment scheme is met. The vehicle leaves stock, its listings close and the
// linked lead is won. Partner-finance deliveries wait for a product decision
// on the money flow (OD-01). For own installments, servicing of the debt is
// not started here.
func (s *Deal) Deliver(ctx context.Context, p *auth.Principal, id string, expected int64, occurredAt time.Time) (*model.Deal, error) {
	if occurredAt.IsZero() || occurredAt.After(s.clock().Add(5*time.Minute)) {
		return nil, apperr.FieldError("occurredAt", "required and not in the future")
	}
	var d *model.Deal
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if d, err = s.deal(ctx, st, p, id, expected); err != nil {
			return err
		}
		c, err := s.checklist(ctx, st, d)
		if err != nil {
			return err
		}
		if !c.PolicyResolved {
			return apperr.New(apperr.ErrForbidden, "policy_unresolved", "delivery for partner financing awaits an approved money-flow policy")
		}
		if !c.Ready() {
			return apperr.New(apperr.ErrConflict, "delivery_not_ready", "complete the sale steps first (see checklist)")
		}
		at := occurredAt.UTC()
		if err := s.stock.Deliver(st.Bind(ctx), d.ID, d.VehicleID, p.UserID, at); err != nil {
			return err
		}
		d.Status, d.DeliveredAt, d.UpdatedAt = "delivered", &at, s.clock()
		if err := st.Deals().Update(ctx, d, expected); err != nil {
			return err
		}
		if err := st.Listings().WithdrawForVehicle(ctx, d.VehicleID, d.UpdatedAt); err != nil {
			return err
		}
		if d.LeadID != nil {
			l, err := st.CRM().Lead(ctx, p.CompanyID, *d.LeadID)
			if err != nil {
				return err
			}
			l.Stage, l.UpdatedAt = "won", d.UpdatedAt
			if err := st.CRM().UpdateLead(ctx, l, l.Version); err != nil {
				return err
			}
			if err := s.event(ctx, st, p, "lead.won", "lead", l.ID, "", map[string]any{"dealId": d.ID}); err != nil {
				return err
			}
		}
		return s.event(ctx, st, p, "deal.delivered", "deal", d.ID, "", nil)
	})
	return d, err
}

// Cancel ends a sale before delivery and releases the vehicle. A sale with
// accepted payments needs a refund policy first and cannot be cancelled here.
func (s *Deal) Cancel(ctx context.Context, p *auth.Principal, id string, expected int64, why string) (*model.Deal, error) {
	var v apperr.Validation
	why = validate.Reason(&v, why)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var d *model.Deal
	err := s.store.InTx(ctx, func(st Store) error {
		var err error
		if d, err = s.deal(ctx, st, p, id, expected); err != nil {
			return err
		}
		is, err := st.Deals().Invoices(ctx, d.ID)
		if err != nil {
			return err
		}
		for i := range is {
			view, err := s.invoiceView(ctx, st, &is[i])
			if err != nil {
				return err
			}
			if view.Paid.AmountMinor != "0" {
				return apperr.New(apperr.ErrConflict, "cancellation_blocked", "the sale has accepted payments")
			}
		}
		if err := s.stock.Release(st.Bind(ctx), d.ID, "sale cancelled: "+why); err != nil {
			return err
		}
		d.Status, d.StatusReason, d.UpdatedAt = "cancelled", why, s.clock()
		if err := st.Deals().Update(ctx, d, expected); err != nil {
			return err
		}
		if d.LeadID != nil {
			l, err := st.CRM().Lead(ctx, p.CompanyID, *d.LeadID)
			if err != nil {
				return err
			}
			l.DealID, l.UpdatedAt = nil, d.UpdatedAt
			if err := st.CRM().UpdateLead(ctx, l, l.Version); err != nil {
				return err
			}
		}
		return s.event(ctx, st, p, "deal.cancelled", "deal", d.ID, why, nil)
	})
	return d, err
}

type DealView struct {
	Deal      model.Deal
	Customer  model.Customer
	Invoices  []InvoiceView
	Checklist Checklist
	History   []model.Event
}

func (s *Deal) view(ctx context.Context, d *model.Deal, full bool) (*DealView, error) {
	c, err := s.store.CRM().Customer(ctx, d.CompanyID, d.CustomerID)
	if err != nil {
		return nil, err
	}
	v := &DealView{Deal: *d, Customer: *c, Invoices: []InvoiceView{}}
	if !full {
		return v, nil
	}
	is, err := s.store.Deals().Invoices(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	for i := range is {
		iv, err := s.invoiceView(ctx, s.store, &is[i])
		if err != nil {
			return nil, err
		}
		v.Invoices = append(v.Invoices, *iv)
	}
	if v.Checklist, err = s.checklist(ctx, s.store, d); err != nil {
		return nil, err
	}
	v.History, err = s.store.Events().For(ctx, "deal", d.ID)
	return v, err
}

func (s *Deal) Get(ctx context.Context, p *auth.Principal, id string) (*DealView, error) {
	d, err := s.deal(ctx, s.store, p, id, -1)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, d, true)
}

func (s *Deal) List(ctx context.Context, p *auth.Principal, status string, limit, offset int) ([]DealView, error) {
	ds, err := s.store.Deals().Deals(ctx, p.CompanyID, scopeBranches(p), status, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]DealView, 0, len(ds))
	for i := range ds {
		v, err := s.view(ctx, &ds[i], false)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}

// Invoice returns one invoice with its payment evidence, checking access
// through the deal it belongs to.
func (s *Deal) Invoice(ctx context.Context, p *auth.Principal, id string) (*InvoiceView, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	i, err := s.store.Deals().Invoice(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	if _, err := s.deal(ctx, s.store, p, i.DealID, -1); err != nil {
		return nil, err
	}
	return s.invoiceView(ctx, s.store, i)
}

// DealInfo is what finance and insurance may know about a sale.
type DealInfo struct {
	ID, CompanyID, VehicleID, CustomerID, PaymentScheme, Status string
	CustomerName                                                string
	Price                                                       money.Money
	Revision                                                    int64
}

// Info returns a sale of the company for other modules (through their ports).
func (s *Deal) Info(ctx context.Context, companyID, id string) (*DealInfo, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	d, err := s.store.Deals().Deal(ctx, companyID, id)
	if err != nil {
		return nil, err
	}
	c, err := s.store.CRM().Customer(ctx, companyID, d.CustomerID)
	if err != nil {
		return nil, err
	}
	return &DealInfo{
		ID: d.ID, CompanyID: d.CompanyID, VehicleID: d.VehicleID, CustomerID: d.CustomerID, CustomerName: c.DisplayName,
		PaymentScheme: d.PaymentScheme, Status: d.Status, Price: d.Price(), Revision: d.Version,
	}, nil
}
