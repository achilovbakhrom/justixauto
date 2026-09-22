package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/database"
	"justixauto/internal/platform/money"
	"justixauto/internal/platform/validate"
)

// PermPaymentsAccept decides on payment evidence: a sensitive action that
// needs a recent second factor.
const PermPaymentsAccept = "commerce.payments.accept"

type Invoice struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	OrderID           string `gorm:"type:uuid"`
	SupplierCompanyID string `gorm:"type:uuid"`
	BuyerCompanyID    string `gorm:"type:uuid"`
	TotalMinor        string
	Currency          string
	Schedule          []byte `gorm:"type:jsonb"`
	Status            string // issued | void
	VoidReason        string
	Version           int64
	CreatedBy         string `gorm:"type:uuid"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (Invoice) TableName() string { return "commerce.invoices" }

type Evidence struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	InvoiceID         string `gorm:"type:uuid"`
	AmountMinor       string
	Currency          string
	PaidOn            time.Time `gorm:"type:date"`
	ExternalReference string
	AttachmentIDs     []byte `gorm:"type:jsonb"`
	Seq               int64  `gorm:"->"`
	Status            string // submitted | accepted | rejected
	DecisionReason    string
	SubmittedBy       string  `gorm:"type:uuid"`
	DecidedBy         *string `gorm:"type:uuid"`
	Version           int64
	CreatedAt         time.Time
	DecidedAt         *time.Time
}

func (Evidence) TableName() string { return "commerce.payment_evidence" }

// ---- repository ----

type InvoiceRepository interface {
	Create(ctx context.Context, i *Invoice) error
	// Get returns an invoice the company is a party of.
	Get(ctx context.Context, companyID, id string) (*Invoice, error)
	ForOrder(ctx context.Context, orderID string) ([]Invoice, error)
	Update(ctx context.Context, i *Invoice, expected int64) error
	AddEvidence(ctx context.Context, e *Evidence) error
	Evidence(ctx context.Context, invoiceID string) ([]Evidence, error)
	GetEvidence(ctx context.Context, id string) (*Evidence, error)
	UpdateEvidence(ctx context.Context, e *Evidence, expected int64) error
}

type invoiceRepository struct{ db *gorm.DB }

func (r *invoiceRepository) Create(ctx context.Context, i *Invoice) error {
	return database.Translate(r.db.WithContext(ctx).Create(i).Error)
}

func (r *invoiceRepository) Get(ctx context.Context, companyID, id string) (*Invoice, error) {
	var i Invoice
	err := r.db.WithContext(ctx).Where("id = ? AND (supplier_company_id = ? OR buyer_company_id = ?)", id, companyID, companyID).Take(&i).Error
	if err != nil {
		return nil, database.Translate(err)
	}
	return &i, nil
}

func (r *invoiceRepository) ForOrder(ctx context.Context, orderID string) ([]Invoice, error) {
	is := []Invoice{}
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).Order("created_at").Find(&is).Error
	return is, database.Translate(err)
}

func (r *invoiceRepository) Update(ctx context.Context, i *Invoice, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Invoice{}, i.ID, expected,
		map[string]any{"status": i.Status, "void_reason": i.VoidReason, "updated_at": i.UpdatedAt})
	if err == nil {
		i.Version = expected + 1
	}
	return err
}

func (r *invoiceRepository) AddEvidence(ctx context.Context, e *Evidence) error {
	return database.Translate(r.db.WithContext(ctx).Create(e).Error)
}

func (r *invoiceRepository) Evidence(ctx context.Context, invoiceID string) ([]Evidence, error) {
	es := []Evidence{}
	err := r.db.WithContext(ctx).Where("invoice_id = ?", invoiceID).Order("seq").Find(&es).Error
	return es, database.Translate(err)
}

func (r *invoiceRepository) GetEvidence(ctx context.Context, id string) (*Evidence, error) {
	var e Evidence
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&e).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &e, nil
}

func (r *invoiceRepository) UpdateEvidence(ctx context.Context, e *Evidence, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Evidence{}, e.ID, expected, map[string]any{
		"status": e.Status, "decision_reason": e.DecisionReason, "decided_by": e.DecidedBy, "decided_at": e.DecidedAt})
	if err == nil {
		e.Version = expected + 1
	}
	return err
}

// ---- service ----

type InvoiceService struct{ deps }

// InvoiceView adds exact paid and outstanding amounts.
type InvoiceView struct {
	Invoice     Invoice
	Evidence    []Evidence
	Paid        money.Money
	Outstanding money.Money
	Pending     money.Money // submitted, not decided yet
}

func amount(s string) *big.Int {
	n, _ := new(big.Int).SetString(s, 10)
	if n == nil {
		return new(big.Int)
	}
	return n
}

func (s *InvoiceService) view(ctx context.Context, st Store, i *Invoice) (*InvoiceView, error) {
	es, err := st.Invoices().Evidence(ctx, i.ID)
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
	outstanding := new(big.Int).Sub(amount(i.TotalMinor), paid)
	return &InvoiceView{Invoice: *i, Evidence: es, Paid: money.Of(paid, i.Currency),
		Outstanding: money.Of(outstanding, i.Currency), Pending: money.Of(pending, i.Currency)}, nil
}

// Issue creates the supplier's invoice from the order's current terms. The
// schedule is copied; without one, the whole amount is due on dueDate.
func (s *InvoiceService) Issue(ctx context.Context, p *auth.Principal, orderID string, expected int64, dueDate string) (*InvoiceView, error) {
	var result *InvoiceView
	err := s.store.InTx(ctx, func(st Store) error {
		if err := validate.IDs(orderID); err != nil {
			return err
		}
		o, err := st.Deals().Order(ctx, p.CompanyID, orderID)
		if err != nil {
			return err
		}
		if o.Version != expected {
			return apperr.ErrStale
		}
		if o.Party(p.CompanyID) != "supplier" {
			return apperr.New(apperr.ErrForbidden, "wrong_party", "only the supplier issues invoices")
		}
		if o.Status == AwaitingSupplier || o.Status == OrderCancelled {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "invoices are issued for accepted orders")
		}
		terms := o.terms()
		total := termsTotal(terms)
		schedule := terms.PaymentSchedule
		if len(schedule) == 0 {
			if _, err := time.Parse(time.DateOnly, dueDate); err != nil {
				return apperr.FieldError("dueDate", "the order has no payment schedule: give a due date (YYYY-MM-DD)")
			}
			schedule = []Installment{{ID: uuid.NewString(), Amount: total, DueDate: dueDate}}
		}
		raw, _ := json.Marshal(schedule)
		now := s.clock()
		i := &Invoice{ID: uuid.NewString(), OrderID: o.ID, SupplierCompanyID: o.SupplierCompanyID, BuyerCompanyID: o.BuyerCompanyID,
			TotalMinor: total.AmountMinor, Currency: total.Currency, Schedule: raw, Status: "issued", Version: 1,
			CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now}
		if err := st.Invoices().Create(ctx, i); err != nil {
			if errors.Is(err, apperr.ErrConflict) {
				return apperr.New(apperr.ErrConflict, "invoice_exists", "the order already has an issued invoice; void it first")
			}
			return err
		}
		o.UpdatedAt = now
		if err := st.Deals().UpdateOrder(ctx, o, expected); err != nil {
			return err
		}
		if err := s.event(ctx, st, p, "order.invoice_issued", "order", o.ID, "", map[string]any{"invoiceId": i.ID, "total": total}); err != nil {
			return err
		}
		result, err = s.view(ctx, st, i)
		return err
	})
	return result, err
}

func (s *InvoiceService) invoice(ctx context.Context, st Store, p *auth.Principal, id string) (*Invoice, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	return st.Invoices().Get(ctx, p.CompanyID, id)
}

// Void withdraws an invoice that has no accepted payment, e.g. after an
// addendum changed the terms; a new invoice can then be issued.
func (s *InvoiceService) Void(ctx context.Context, p *auth.Principal, id string, expected int64, why string) (*InvoiceView, error) {
	var v apperr.Validation
	why = validate.Reason(&v, why)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *InvoiceView
	err := s.store.InTx(ctx, func(st Store) error {
		i, err := s.invoice(ctx, st, p, id)
		if err != nil {
			return err
		}
		if i.Version != expected {
			return apperr.ErrStale
		}
		if i.SupplierCompanyID != p.CompanyID {
			return apperr.New(apperr.ErrForbidden, "wrong_party", "only the supplier voids invoices")
		}
		view, err := s.view(ctx, st, i)
		if err != nil {
			return err
		}
		if i.Status != "issued" || view.Paid.AmountMinor != "0" || view.Pending.AmountMinor != "0" {
			return apperr.New(apperr.ErrConflict, "invoice_has_payments", "only an issued invoice without payments can be voided")
		}
		i.Status, i.VoidReason, i.UpdatedAt = "void", why, s.clock()
		if err := st.Invoices().Update(ctx, i, expected); err != nil {
			return err
		}
		if err := s.event(ctx, st, p, "order.invoice_voided", "order", i.OrderID, why, map[string]any{"invoiceId": i.ID}); err != nil {
			return err
		}
		result, err = s.view(ctx, st, i)
		return err
	})
	return result, err
}

type EvidenceInput struct {
	ClaimedAmount     money.Money `json:"claimedAmount"`
	PaidOn            string      `json:"paidOn"`
	ExternalReference string      `json:"externalReference"`
	AttachmentIDs     []string    `json:"attachmentBindingIds"`
}

// SubmitEvidence records the buyer's claim of an external payment. Claims can
// never exceed what is still outstanding (including undecided claims).
func (s *InvoiceService) SubmitEvidence(ctx context.Context, p *auth.Principal, invoiceID string, in EvidenceInput) (*InvoiceView, error) {
	var v apperr.Validation
	claimed, ok := in.ClaimedAmount.Parse()
	if !ok || claimed.Sign() == 0 {
		v.Add("claimedAmount", "must be a positive amount in minor units with a currency code")
	}
	paidOn, err := time.Parse(time.DateOnly, in.PaidOn)
	if err != nil || paidOn.After(s.clock()) {
		v.Add("paidOn", "a date (YYYY-MM-DD), not in the future")
	}
	ref := validate.Text(&v, "externalReference", in.ExternalReference, 1, 100)
	attachments := validate.UniqueIDs(&v, "attachmentBindingIds", in.AttachmentIDs)
	if len(attachments) > 10 {
		v.Add("attachmentBindingIds", "at most 10 files")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *InvoiceView
	err = s.store.InTx(ctx, func(st Store) error {
		i, err := s.invoice(ctx, st, p, invoiceID)
		if err != nil {
			return err
		}
		if i.BuyerCompanyID != p.CompanyID {
			return apperr.New(apperr.ErrForbidden, "wrong_party", "only the buyer submits payment evidence")
		}
		if i.Status != "issued" {
			return apperr.New(apperr.ErrConflict, "invoice_void", "the invoice was voided")
		}
		if in.ClaimedAmount.Currency != i.Currency {
			return apperr.FieldError("claimedAmount", "must be in "+i.Currency)
		}
		view, err := s.view(ctx, st, i)
		if err != nil {
			return err
		}
		open := new(big.Int).Sub(amount(view.Outstanding.AmountMinor), amount(view.Pending.AmountMinor))
		if claimed.Cmp(open) > 0 {
			return apperr.FieldError("claimedAmount", "exceeds the open amount "+open.String())
		}
		now := s.clock()
		rawIDs, _ := json.Marshal(attachments)
		e := &Evidence{ID: uuid.NewString(), InvoiceID: i.ID, AmountMinor: claimed.String(), Currency: i.Currency, PaidOn: paidOn,
			ExternalReference: ref, AttachmentIDs: rawIDs, Status: "submitted", SubmittedBy: p.UserID, Version: 1, CreatedAt: now}
		// The proof files become readable by the payee for this claim.
		for _, f := range attachments {
			if err := s.files.Share(st.Bind(ctx), p.CompanyID, f, i.SupplierCompanyID, "commerce.payment-evidence", e.ID); err != nil {
				if errors.Is(err, apperr.ErrNotFound) {
					return apperr.FieldError("attachmentBindingIds", "contains files that are not yours")
				}
				return err
			}
		}
		if err := st.Invoices().AddEvidence(ctx, e); err != nil {
			return err
		}
		if err := s.event(ctx, st, p, "order.payment_submitted", "order", i.OrderID, "", map[string]any{"evidenceId": e.ID, "amount": in.ClaimedAmount}); err != nil {
			return err
		}
		result, err = s.view(ctx, st, i)
		return err
	})
	return result, err
}

// DecideEvidence: the supplier's finance staff accept (with explicit
// confirmation) or reject (with a reason) a payment claim.
func (s *InvoiceService) DecideEvidence(ctx context.Context, p *auth.Principal, evidenceID string, expected int64, accept, confirmation bool, why string) (*InvoiceView, error) {
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
		if err := validate.IDs(evidenceID); err != nil {
			return err
		}
		e, err := st.Invoices().GetEvidence(ctx, evidenceID)
		if err != nil {
			return err
		}
		i, err := st.Invoices().Get(ctx, p.CompanyID, e.InvoiceID)
		if err != nil {
			return err
		}
		if i.SupplierCompanyID != p.CompanyID {
			return apperr.New(apperr.ErrForbidden, "wrong_party", "only the payee decides on payment evidence")
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
		if err := st.Invoices().UpdateEvidence(ctx, e, expected); err != nil {
			return err
		}
		if err := s.event(ctx, st, p, "order.payment_"+e.Status, "order", i.OrderID, why, map[string]any{"evidenceId": e.ID}); err != nil {
			return err
		}
		result, err = s.view(ctx, st, i)
		return err
	})
	return result, err
}

func (s *InvoiceService) Get(ctx context.Context, p *auth.Principal, id string) (*InvoiceView, error) {
	i, err := s.invoice(ctx, s.store, p, id)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, s.store, i)
}

func (s *InvoiceService) ForOrder(ctx context.Context, p *auth.Principal, orderID string) ([]InvoiceView, error) {
	if err := validate.IDs(orderID); err != nil {
		return nil, err
	}
	if _, err := s.store.Deals().Order(ctx, p.CompanyID, orderID); err != nil {
		return nil, err
	}
	is, err := s.store.Invoices().ForOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	out := make([]InvoiceView, 0, len(is))
	for i := range is {
		v, err := s.view(ctx, s.store, &is[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}

func init() {
	// Orders with accepted payments cannot simply be cancelled.
	cancellationChecks = append(cancellationChecks, func(ctx context.Context, st Store, o *Order) (string, error) {
		is, err := st.Invoices().ForOrder(ctx, o.ID)
		if err != nil {
			return "", err
		}
		for _, i := range is {
			es, err := st.Invoices().Evidence(ctx, i.ID)
			if err != nil {
				return "", err
			}
			for _, e := range es {
				if e.Status == "accepted" {
					return "the order has accepted payments", nil
				}
			}
		}
		return "", nil
	})
}
