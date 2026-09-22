// Package insurance lets sellers send own-installment sales to an insurance
// company, which reviews them and approves or declines. A decision is only a
// decision: no policy, premium, coverage, payment or hand-over follows.
package insurance

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/database"
	"justixauto/internal/platform/money"
	"justixauto/internal/platform/validate"
)

const (
	PermRead   = "insurance.read"
	PermApply  = "insurance.applications.manage" // seller side
	PermReview = "insurance.applications.review" // insurer: take, request information
	PermDecide = "insurance.applications.decide" // insurer: approve/decline (sensitive)
)

var Permissions = []auth.PermissionInfo{
	{Key: PermRead, Scope: "company", Assignable: true},
	{Key: PermApply, Scope: "company", Assignable: true},
	{Key: PermReview, Scope: "company", Assignable: true},
	{Key: PermDecide, Scope: "company", RequiresMFA: true, Assignable: true},
}

// Sale is what insurance may know about a retail sale (from retail).
type Sale struct {
	ID, VehicleID, PaymentScheme, Status string
	Price                                money.Money
	Revision                             int64
}

// Sales reads the seller's sales (implemented by retail).
type Sales interface {
	Sale(ctx context.Context, companyID, dealID string) (*Sale, error)
}

// Company is the insurer's public profile (from identity).
type Company struct {
	ID, Name, Kind string
	Active         bool
}

type Directory interface {
	Company(ctx context.Context, id string) (*Company, error)
}

// ---- model ----

type Application struct {
	ID               string `gorm:"primaryKey;type:uuid"`
	SellerCompanyID  string `gorm:"type:uuid"`
	InsurerCompanyID string `gorm:"type:uuid"`
	DealID           string `gorm:"type:uuid"`
	Status           string
	Note             string
	Snapshot         []byte `gorm:"type:jsonb"`
	Version          int64
	CreatedBy        string `gorm:"type:uuid"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	SubmittedAt      *time.Time
	DecidedAt        *time.Time
}

func (Application) TableName() string { return "insurance.applications" }

type Message struct {
	ID            string `gorm:"primaryKey;type:uuid"`
	Seq           int64  `gorm:"->"`
	ApplicationID string `gorm:"type:uuid"`
	Kind          string
	RequestID     *string `gorm:"type:uuid"`
	Note          string
	CompanyID     string `gorm:"type:uuid"`
	ActorUserID   string `gorm:"type:uuid"`
	CreatedAt     time.Time
}

func (Message) TableName() string { return "insurance.messages" }

// ---- repository ----

type Repository interface {
	Create(ctx context.Context, a *Application) error
	// Get returns an application visible to the company: the seller always,
	// the addressed insurer once it was submitted.
	Get(ctx context.Context, companyID, id string) (*Application, error)
	List(ctx context.Context, companyID, status string, limit, offset int) ([]Application, error)
	ForDeal(ctx context.Context, sellerID, dealID string) (*Application, error)
	Update(ctx context.Context, a *Application, expected int64) error
	AddMessage(ctx context.Context, m *Message) error
	Messages(ctx context.Context, applicationID string) ([]Message, error)
	InTx(ctx context.Context, fn func(Repository) error) error
}

type repository struct{ db *gorm.DB }

func (r *repository) InTx(ctx context.Context, fn func(Repository) error) error {
	return database.Conn(ctx, r.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(&repository{tx}) })
}

func (r *repository) Create(ctx context.Context, a *Application) error {
	return database.Translate(r.db.WithContext(ctx).Create(a).Error)
}

const visible = "(seller_company_id = ? OR (insurer_company_id = ? AND status <> 'draft'))"

func (r *repository) Get(ctx context.Context, companyID, id string) (*Application, error) {
	var a Application
	if err := r.db.WithContext(ctx).Where("id = ? AND "+visible, id, companyID, companyID).Take(&a).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &a, nil
}

func (r *repository) List(ctx context.Context, companyID, status string, limit, offset int) ([]Application, error) {
	limit, offset = database.Page(limit, offset)
	q := r.db.WithContext(ctx).Where(visible, companyID, companyID).Order("updated_at DESC, id").Limit(limit).Offset(offset)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	as := []Application{}
	return as, database.Translate(q.Find(&as).Error)
}

func (r *repository) ForDeal(ctx context.Context, sellerID, dealID string) (*Application, error) {
	var a Application
	if err := r.db.WithContext(ctx).Where("seller_company_id = ? AND deal_id = ?", sellerID, dealID).Take(&a).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &a, nil
}

func (r *repository) Update(ctx context.Context, a *Application, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Application{}, a.ID, expected, map[string]any{
		"insurer_company_id": a.InsurerCompanyID, "status": a.Status, "note": a.Note, "snapshot": a.Snapshot,
		"updated_at": a.UpdatedAt, "submitted_at": a.SubmittedAt, "decided_at": a.DecidedAt})
	if err == nil {
		a.Version = expected + 1
	}
	return err
}

func (r *repository) AddMessage(ctx context.Context, m *Message) error {
	return database.Translate(r.db.WithContext(ctx).Create(m).Error)
}

func (r *repository) Messages(ctx context.Context, applicationID string) ([]Message, error) {
	ms := []Message{}
	err := r.db.WithContext(ctx).Where("application_id = ?", applicationID).Order("seq").Find(&ms).Error
	return ms, database.Translate(err)
}

// ---- service ----

type Service struct {
	repo      Repository
	sales     Sales
	directory Directory
	now       func() time.Time
}

func (s *Service) clock() time.Time { return s.now().UTC() }

func (s *Service) insurer(ctx context.Context, id string) error {
	if validate.IDs(id) != nil {
		return apperr.FieldError("insurerCompanyId", "must be a valid ID")
	}
	c, err := s.directory.Company(ctx, id)
	if errors.Is(err, apperr.ErrNotFound) || (err == nil && (c.Kind != "insurance" || !c.Active)) {
		return apperr.FieldError("insurerCompanyId", "not an active insurance company")
	}
	return err
}

// eligibleSale: the seller's own, undelivered own-installment sale.
func (s *Service) eligibleSale(ctx context.Context, companyID, dealID string) (*Sale, error) {
	sale, err := s.sales.Sale(ctx, companyID, dealID)
	if errors.Is(err, apperr.ErrNotFound) {
		return nil, apperr.FieldError("retailDealId", "not a sale of your company")
	}
	if err != nil {
		return nil, err
	}
	if sale.PaymentScheme != "own-installment" || sale.Status != "reserved" {
		return nil, apperr.New(apperr.ErrConflict, "sale_not_eligible", "only active own-installment sales can be insured")
	}
	return sale, nil
}

type CreateInput struct {
	RetailDealID     string `json:"retailDealId"`
	InsurerCompanyID string `json:"insurerCompanyId"`
	Note             string `json:"note"`
}

// Create drafts the application; the insurer does not see drafts.
func (s *Service) Create(ctx context.Context, p *auth.Principal, in CreateInput) (*Application, error) {
	var v apperr.Validation
	note := validate.Text(&v, "note", in.Note, 0, 2000)
	if err := v.Err(); err != nil {
		return nil, err
	}
	if _, err := s.eligibleSale(ctx, p.CompanyID, in.RetailDealID); err != nil {
		return nil, err
	}
	if err := s.insurer(ctx, in.InsurerCompanyID); err != nil {
		return nil, err
	}
	now := s.clock()
	a := &Application{ID: uuid.NewString(), SellerCompanyID: p.CompanyID, InsurerCompanyID: in.InsurerCompanyID, DealID: in.RetailDealID,
		Status: "draft", Note: note, Version: 1, CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.Create(ctx, a); err != nil {
		if errors.Is(err, apperr.ErrConflict) {
			return nil, apperr.New(apperr.ErrConflict, "application_exists", "this sale already has an insurance application")
		}
		return nil, err
	}
	return a, nil
}

func (s *Service) load(ctx context.Context, r Repository, p *auth.Principal, id string, expected int64, side string) (*Application, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	a, err := r.Get(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	if a.Version != expected {
		return nil, apperr.ErrStale
	}
	if (side == "seller") != (a.SellerCompanyID == p.CompanyID) {
		return nil, apperr.New(apperr.ErrForbidden, "wrong_party", "only the "+side+" can do this")
	}
	return a, nil
}

// UpdateDraft changes the insurer or note while the application is a draft.
func (s *Service) UpdateDraft(ctx context.Context, p *auth.Principal, id string, expected int64, insurerID, note string) (*Application, error) {
	var v apperr.Validation
	note = validate.Text(&v, "note", note, 0, 2000)
	if err := v.Err(); err != nil {
		return nil, err
	}
	if err := s.insurer(ctx, insurerID); err != nil {
		return nil, err
	}
	var a *Application
	err := s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if a, err = s.load(ctx, r, p, id, expected, "seller"); err != nil {
			return err
		}
		if a.Status != "draft" {
			return apperr.New(apperr.ErrConflict, "not_draft", "only drafts can be edited")
		}
		a.InsurerCompanyID, a.Note, a.UpdatedAt = insurerID, note, s.clock()
		return r.Update(ctx, a, expected)
	})
	return a, err
}

// Submit freezes the sale facts and sends the application to the insurer.
// dealRevision must match the sale the seller reviewed.
func (s *Service) Submit(ctx context.Context, p *auth.Principal, id string, expected int64, confirmation bool, dealRevision int64) (*Application, error) {
	if !confirmation {
		return nil, apperr.FieldError("confirmation", "confirm the data sent to the insurer")
	}
	var a *Application
	err := s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if a, err = s.load(ctx, r, p, id, expected, "seller"); err != nil {
			return err
		}
		if a.Status != "draft" {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "the application was already submitted")
		}
		sale, err := s.eligibleSale(ctx, p.CompanyID, a.DealID)
		if err != nil {
			return err
		}
		if sale.Revision != dealRevision {
			return apperr.New(apperr.ErrConflict, "sale_changed", "the sale changed; review it and submit again")
		}
		if err := s.insurer(ctx, a.InsurerCompanyID); err != nil {
			return err
		}
		now := s.clock()
		a.Snapshot, _ = json.Marshal(map[string]any{"dealId": sale.ID, "vehicleId": sale.VehicleID, "price": sale.Price,
			"paymentScheme": sale.PaymentScheme, "dealRevision": sale.Revision, "note": a.Note})
		a.Status, a.SubmittedAt, a.UpdatedAt = "submitted", &now, now
		if err := r.Update(ctx, a, expected); err != nil {
			return err
		}
		return s.message(ctx, r, p, a, "submitted", nil, a.Note)
	})
	return a, err
}

func (s *Service) message(ctx context.Context, r Repository, p *auth.Principal, a *Application, kind string, requestID *string, note string) error {
	return r.AddMessage(ctx, &Message{ID: uuid.NewString(), ApplicationID: a.ID, Kind: kind, RequestID: requestID, Note: note,
		CompanyID: p.CompanyID, ActorUserID: p.UserID, CreatedAt: s.clock()})
}

// Act applies an insurer or seller step:
//
//	take     insurer  submitted  → review
//	request  insurer  review     → needs-info   (note required)
//	respond  seller   needs-info → review       (note required, answers the open request)
//	approve  insurer  review     → approved     (note required)
//	decline  insurer  review     → declined     (note required)
func (s *Service) Act(ctx context.Context, p *auth.Principal, id string, expected int64, action, note string) (*Application, error) {
	type step struct {
		side, from, to, kind string
		note                 bool
	}
	steps := map[string]step{
		"take":    {"insurer", "submitted", "review", "taken", false},
		"request": {"insurer", "review", "needs-info", "request", true},
		"respond": {"seller", "needs-info", "review", "response", true},
		"approve": {"insurer", "review", "approved", "approved", true},
		"decline": {"insurer", "review", "declined", "declined", true},
	}
	st, ok := steps[action]
	if !ok {
		return nil, apperr.ErrNotFound
	}
	var v apperr.Validation
	if st.note {
		note = validate.Text(&v, "note", note, 1, 2000)
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var a *Application
	err := s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if a, err = s.load(ctx, r, p, id, expected, st.side); err != nil {
			return err
		}
		if a.Status != st.from {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "cannot "+action+" an application that is "+a.Status)
		}
		var requestID *string
		if action == "respond" {
			ms, err := r.Messages(ctx, a.ID)
			if err != nil {
				return err
			}
			for i := len(ms) - 1; i >= 0; i-- {
				if ms[i].Kind == "request" {
					requestID = &ms[i].ID
					break
				}
			}
		}
		now := s.clock()
		a.Status, a.UpdatedAt = st.to, now
		if st.to == "approved" || st.to == "declined" {
			a.DecidedAt = &now
		}
		if err := r.Update(ctx, a, expected); err != nil {
			return err
		}
		return s.message(ctx, r, p, a, st.kind, requestID, note)
	})
	return a, err
}

func (s *Service) Get(ctx context.Context, p *auth.Principal, id string) (*Application, []Message, error) {
	if err := validate.IDs(id); err != nil {
		return nil, nil, err
	}
	a, err := s.repo.Get(ctx, p.CompanyID, id)
	if err != nil {
		return nil, nil, err
	}
	ms, err := s.repo.Messages(ctx, a.ID)
	return a, ms, err
}

func (s *Service) List(ctx context.Context, p *auth.Principal, status string, limit, offset int) ([]Application, error) {
	return s.repo.List(ctx, p.CompanyID, status, limit, offset)
}

// Approved reports whether the insurer approved the seller's sale (for retail).
func (s *Service) Approved(ctx context.Context, companyID, dealID string) (bool, error) {
	a, err := s.repo.ForDeal(ctx, companyID, dealID)
	if errors.Is(err, apperr.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return a.Status == "approved", nil
}
