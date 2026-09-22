package retail

import (
	"context"
	"errors"
	"math/big"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/database"
	"justixauto/internal/platform/money"
	"justixauto/internal/platform/validate"
)

// Insurance tells whether the insurer approved the deal's application
// (implemented by the insurance module).
type Insurance interface {
	Approved(ctx context.Context, companyID, dealID string) (bool, error)
}

// ---- model ----

type Deal struct {
	ID                    string  `gorm:"primaryKey;type:uuid"`
	CompanyID             string  `gorm:"type:uuid"`
	BranchID              string  `gorm:"type:uuid"`
	CustomerID            string  `gorm:"type:uuid"`
	LeadID                *string `gorm:"type:uuid"`
	VehicleID             string  `gorm:"type:uuid"`
	PaymentScheme         string
	PriceMinor            string
	Currency              string
	Status                string     // reserved | delivered | cancelled
	ContractSignedOn      *time.Time `gorm:"type:date"`
	ContractReference     string
	RegisteredOn          *time.Time `gorm:"type:date"`
	PlateNumber           string
	RegistrationReference string
	DeliveredAt           *time.Time
	StatusReason          string
	Version               int64
	CreatedBy             string `gorm:"type:uuid"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (Deal) TableName() string { return "retail.deals" }

func (d *Deal) Price() money.Money {
	return money.Money{AmountMinor: d.PriceMinor, Currency: d.Currency}
}

type Invoice struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	DealID            string `gorm:"type:uuid"`
	CompanyID         string `gorm:"type:uuid"`
	Purpose           string
	AmountMinor       string
	Currency          string
	RecipientSnapshot string
	DueDate           *time.Time `gorm:"type:date"`
	Status            string
	Version           int64
	CreatedAt         time.Time
}

func (Invoice) TableName() string { return "retail.invoices" }

type Evidence struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	InvoiceID         string `gorm:"type:uuid"`
	AmountMinor       string
	Currency          string
	PaidOn            time.Time `gorm:"type:date"`
	ExternalReference string
	Status            string
	DecisionReason    string
	SubmittedBy       string  `gorm:"type:uuid"`
	DecidedBy         *string `gorm:"type:uuid"`
	Version           int64
	CreatedAt         time.Time
	DecidedAt         *time.Time
}

func (Evidence) TableName() string { return "retail.payment_evidence" }

// ---- repository ----

type DealRepository interface {
	Create(ctx context.Context, d *Deal) error
	Deal(ctx context.Context, companyID, id string) (*Deal, error)
	Deals(ctx context.Context, companyID string, branchIDs []string, status string, limit, offset int) ([]Deal, error)
	Update(ctx context.Context, d *Deal, expected int64) error
	CreateInvoice(ctx context.Context, i *Invoice) error
	Invoice(ctx context.Context, companyID, id string) (*Invoice, error)
	Invoices(ctx context.Context, dealID string) ([]Invoice, error)
	AddEvidence(ctx context.Context, e *Evidence) error
	Evidence(ctx context.Context, invoiceID string) ([]Evidence, error)
	GetEvidence(ctx context.Context, id string) (*Evidence, error)
	UpdateEvidence(ctx context.Context, e *Evidence, expected int64) error
}

type dealRepository struct{ db *gorm.DB }

func (r *dealRepository) Create(ctx context.Context, d *Deal) error {
	return database.Translate(r.db.WithContext(ctx).Create(d).Error)
}

func (r *dealRepository) Deal(ctx context.Context, companyID, id string) (*Deal, error) {
	var d Deal
	if err := r.db.WithContext(ctx).Where("id = ? AND company_id = ?", id, companyID).Take(&d).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &d, nil
}

func (r *dealRepository) Deals(ctx context.Context, companyID string, branchIDs []string, status string, limit, offset int) ([]Deal, error) {
	limit, offset = database.Page(limit, offset)
	q := r.db.WithContext(ctx).Where("company_id = ?", companyID).Order("updated_at DESC, id").Limit(limit).Offset(offset)
	if branchIDs != nil {
		q = q.Where("branch_id IN ?", append(branchIDs, uuid.Nil.String()))
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	ds := []Deal{}
	return ds, database.Translate(q.Find(&ds).Error)
}

func (r *dealRepository) Update(ctx context.Context, d *Deal, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Deal{}, d.ID, expected, map[string]any{
		"status": d.Status, "contract_signed_on": d.ContractSignedOn, "contract_reference": d.ContractReference,
		"registered_on": d.RegisteredOn, "plate_number": d.PlateNumber, "registration_reference": d.RegistrationReference,
		"delivered_at": d.DeliveredAt, "status_reason": d.StatusReason, "updated_at": d.UpdatedAt})
	if err == nil {
		d.Version = expected + 1
	}
	return err
}

func (r *dealRepository) CreateInvoice(ctx context.Context, i *Invoice) error {
	return database.Translate(r.db.WithContext(ctx).Create(i).Error)
}

func (r *dealRepository) Invoice(ctx context.Context, companyID, id string) (*Invoice, error) {
	var i Invoice
	if err := r.db.WithContext(ctx).Where("id = ? AND company_id = ?", id, companyID).Take(&i).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &i, nil
}

func (r *dealRepository) Invoices(ctx context.Context, dealID string) ([]Invoice, error) {
	is := []Invoice{}
	err := r.db.WithContext(ctx).Where("deal_id = ?", dealID).Order("created_at").Find(&is).Error
	return is, database.Translate(err)
}

func (r *dealRepository) AddEvidence(ctx context.Context, e *Evidence) error {
	return database.Translate(r.db.WithContext(ctx).Create(e).Error)
}

func (r *dealRepository) Evidence(ctx context.Context, invoiceID string) ([]Evidence, error) {
	es := []Evidence{}
	err := r.db.WithContext(ctx).Where("invoice_id = ?", invoiceID).Order("created_at, id").Find(&es).Error
	return es, database.Translate(err)
}

func (r *dealRepository) GetEvidence(ctx context.Context, id string) (*Evidence, error) {
	var e Evidence
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&e).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &e, nil
}

func (r *dealRepository) UpdateEvidence(ctx context.Context, e *Evidence, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Evidence{}, e.ID, expected, map[string]any{
		"status": e.Status, "decision_reason": e.DecisionReason, "decided_by": e.DecidedBy, "decided_at": e.DecidedAt})
	if err == nil {
		e.Version = expected + 1
	}
	return err
}

// ---- service ----

type DealService struct {
	deps
	insurance Insurance
}

var schemes = []string{"cash", "own-installment", "partner-finance"}

// purposes lists which invoices a payment scheme uses.
var purposes = map[string][]string{
	"cash":            {"vehicle-payment", "registration"},
	"own-installment": {"first-installment", "registration"},
	"partner-finance": {"registration"},
}

type DealInput struct {
	CustomerID    string      `json:"customerId"`
	LeadID        *string     `json:"leadId"`
	VehicleID     string      `json:"vehicleId"`
	BranchID      string      `json:"branchId"`
	PaymentScheme string      `json:"paymentScheme"`
	Price         money.Money `json:"price"`
}

// Create starts a sale: the vehicle is reserved at once (one active sale per
// VIN across wholesale and retail) and an optional qualified lead is linked.
func (s *DealService) Create(ctx context.Context, p *auth.Principal, in DealInput) (*Deal, error) {
	var v apperr.Validation
	if !slices.Contains(schemes, in.PaymentScheme) {
		v.Add("paymentScheme", "must be cash, own-installment or partner-finance")
	}
	price(&v, "price", in.Price)
	for field, id := range map[string]string{"customerId": in.CustomerID, "vehicleId": in.VehicleID, "branchId": in.BranchID} {
		if validate.IDs(id) != nil {
			v.Add(field, "must be a valid ID")
		}
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	if err := s.branch(ctx, p, "branchId", in.BranchID); err != nil {
		return nil, err
	}
	now := s.clock()
	d := &Deal{ID: uuid.NewString(), CompanyID: p.CompanyID, BranchID: in.BranchID, CustomerID: in.CustomerID, LeadID: in.LeadID,
		VehicleID: in.VehicleID, PaymentScheme: in.PaymentScheme, PriceMinor: in.Price.AmountMinor, Currency: in.Price.Currency,
		Status: "reserved", Version: 1, CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now}
	err := s.store.InTx(ctx, func(st Store) error {
		if _, err := st.CRM().Customer(ctx, p.CompanyID, in.CustomerID); err != nil {
			return apperr.FieldError("customerId", "unknown customer")
		}
		var lead *Lead
		if in.LeadID != nil {
			l, err := st.CRM().Lead(ctx, p.CompanyID, *in.LeadID)
			if err != nil || l.CustomerID != in.CustomerID {
				return apperr.FieldError("leadId", "not a lead of this customer")
			}
			if !l.Open() || !l.Qualified() || l.DealID != nil {
				return apperr.New(apperr.ErrConflict, "lead_not_eligible", "the lead must be open, qualified and without an active sale")
			}
			lead = l
		}
		vehicle, err := s.stock.Vehicle(st.Bind(ctx), p.CompanyID, in.VehicleID)
		if err != nil || !vehicle.Owned {
			return apperr.FieldError("vehicleId", "not a vehicle your company owns")
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

func (s *DealService) deal(ctx context.Context, st Store, p *auth.Principal, id string, expected int64) (*Deal, error) {
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

// RecordContract stores the external contract fact (signed date, reference).
func (s *DealService) RecordContract(ctx context.Context, p *auth.Principal, id string, expected int64, signedOn, reference string) (*Deal, error) {
	return s.update(ctx, p, id, expected, "deal.contract_recorded", func(v *apperr.Validation, d *Deal) {
		d.ContractSignedOn = date(v, "signedOn", signedOn, s.clock())
		d.ContractReference = validate.Text(v, "reference", reference, 1, 100)
	})
}

// RecordRegistration stores the vehicle registration facts; the registration
// invoice must be paid first.
func (s *DealService) RecordRegistration(ctx context.Context, p *auth.Principal, id string, expected int64, registeredOn, plate, reference string) (*Deal, error) {
	return s.update(ctx, p, id, expected, "deal.registered", func(v *apperr.Validation, d *Deal) {
		d.RegisteredOn = date(v, "registeredOn", registeredOn, s.clock())
		d.PlateNumber = validate.Text(v, "plateNumber", plate, 1, 20)
		d.RegistrationReference = validate.Text(v, "reference", reference, 0, 100)
	}, func(ctx context.Context, st Store, d *Deal) error {
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

func (s *DealService) update(ctx context.Context, p *auth.Principal, id string, expected int64, event string,
	apply func(*apperr.Validation, *Deal), checks ...func(context.Context, Store, *Deal) error) (*Deal, error) {
	var d *Deal
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
func (s *DealService) paid(ctx context.Context, st Store, d *Deal, purpose string) (bool, error) {
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
func (s *DealService) IssueInvoice(ctx context.Context, p *auth.Principal, dealID string, expected int64, in InvoiceInput) (*InvoiceView, error) {
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
		if !slices.Contains(purposes[d.PaymentScheme], in.Purpose) {
			return apperr.FieldError("purpose", "not used by a "+d.PaymentScheme+" sale")
		}
		i := &Invoice{ID: uuid.NewString(), DealID: d.ID, CompanyID: d.CompanyID, Purpose: in.Purpose, AmountMinor: in.Amount.AmountMinor,
			Currency: in.Amount.Currency, RecipientSnapshot: recipient, DueDate: due, Status: "issued", Version: 1, CreatedAt: s.clock()}
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
	Invoice     Invoice
	Evidence    []Evidence
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

func (s *DealService) invoiceView(ctx context.Context, st Store, i *Invoice) (*InvoiceView, error) {
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
	return &InvoiceView{Invoice: *i, Evidence: es, Paid: money.Of(paid, i.Currency), Pending: money.Of(pending, i.Currency),
		Outstanding: money.Of(out, i.Currency)}, nil
}

type EvidenceInput struct {
	ClaimedAmount     money.Money `json:"claimedAmount"`
	PaidOn            string      `json:"paidOn"`
	ExternalReference string      `json:"externalReference"`
	AttachmentIDs     []string    `json:"attachmentBindingIds"`
}

// SubmitEvidence records the customer's external payment as reported to the
// seller's staff; it is reviewed separately.
func (s *DealService) SubmitEvidence(ctx context.Context, p *auth.Principal, invoiceID string, in EvidenceInput) (*InvoiceView, error) {
	var v apperr.Validation
	claimed, ok := in.ClaimedAmount.Parse()
	if !ok || claimed.Sign() == 0 {
		v.Add("claimedAmount", "must be a positive amount in minor units with a currency code")
	}
	paidOn := date(&v, "paidOn", in.PaidOn, s.clock())
	ref := validate.Text(&v, "externalReference", in.ExternalReference, 1, 100)
	if len(in.AttachmentIDs) > 0 {
		v.Add("attachmentBindingIds", "attachments are not supported yet")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *InvoiceView
	err := s.store.InTx(ctx, func(st Store) error {
		if err := validate.IDs(invoiceID); err != nil {
			return err
		}
		i, err := st.Deals().Invoice(ctx, p.CompanyID, invoiceID)
		if err != nil {
			return err
		}
		if _, err := s.deal(ctx, st, p, i.DealID, -1); err != nil {
			return err
		}
		if i.Status != "issued" {
			return apperr.New(apperr.ErrConflict, "invoice_void", "the invoice was voided")
		}
		if in.ClaimedAmount.Currency != i.Currency {
			return apperr.FieldError("claimedAmount", "must be in "+i.Currency)
		}
		view, err := s.invoiceView(ctx, st, i)
		if err != nil {
			return err
		}
		if claimed.Cmp(new(big.Int).Sub(amount(view.Outstanding.AmountMinor), amount(view.Pending.AmountMinor))) > 0 {
			return apperr.FieldError("claimedAmount", "exceeds the open amount")
		}
		e := &Evidence{ID: uuid.NewString(), InvoiceID: i.ID, AmountMinor: claimed.String(), Currency: i.Currency, PaidOn: *paidOn,
			ExternalReference: ref, Status: "submitted", SubmittedBy: p.UserID, Version: 1, CreatedAt: s.clock()}
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
func (s *DealService) DecideEvidence(ctx context.Context, p *auth.Principal, id string, expected int64, accept, confirmation bool, why string) (*InvoiceView, error) {
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

func (s *DealService) checklist(ctx context.Context, st Store, d *Deal) (Checklist, error) {
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
func (s *DealService) Deliver(ctx context.Context, p *auth.Principal, id string, expected int64, occurredAt time.Time) (*Deal, error) {
	if occurredAt.IsZero() || occurredAt.After(s.clock().Add(5*time.Minute)) {
		return nil, apperr.FieldError("occurredAt", "required and not in the future")
	}
	var d *Deal
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
func (s *DealService) Cancel(ctx context.Context, p *auth.Principal, id string, expected int64, why string) (*Deal, error) {
	var v apperr.Validation
	why = validate.Reason(&v, why)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var d *Deal
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
	Deal      Deal
	Customer  Customer
	Invoices  []InvoiceView
	Checklist Checklist
	History   []Event
}

func (s *DealService) view(ctx context.Context, d *Deal, full bool) (*DealView, error) {
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

func (s *DealService) Get(ctx context.Context, p *auth.Principal, id string) (*DealView, error) {
	d, err := s.deal(ctx, s.store, p, id, -1)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, d, true)
}

func (s *DealService) List(ctx context.Context, p *auth.Principal, status string, limit, offset int) ([]DealView, error) {
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

// DealInfo is what finance and insurance may know about a sale.
type DealInfo struct {
	ID, CompanyID, VehicleID, CustomerID, PaymentScheme, Status string
	Price                                                       money.Money
	Revision                                                    int64
}

// Info returns a sale of the company for other modules (through their ports).
func (s *DealService) Info(ctx context.Context, companyID, id string) (*DealInfo, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	d, err := s.store.Deals().Deal(ctx, companyID, id)
	if err != nil {
		return nil, err
	}
	return &DealInfo{ID: d.ID, CompanyID: d.CompanyID, VehicleID: d.VehicleID, CustomerID: d.CustomerID,
		PaymentScheme: d.PaymentScheme, Status: d.Status, Price: d.Price(), Revision: d.Version}, nil
}
