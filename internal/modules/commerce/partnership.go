// Package commerce owns B2B trade between companies: partnerships, offers,
// quotations, orders, shipments and invoices. Other companies are known only
// through the Directory port.
package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/database"
	"justixauto/internal/pkg/validate"
)

const (
	PermRead               = "commerce.read"
	PermPartnershipsManage = "commerce.partnerships.manage"
)

// Permissions are registered in the identity catalog at startup.
var Permissions = []auth.PermissionInfo{
	{Key: PermRead, Scope: "company", Assignable: true},
	{Key: PermPartnershipsManage, Scope: "company", Assignable: true},
	{Key: PermOffersManage, Scope: "company", Assignable: true},
	{Key: PermTrade, Scope: "company", Assignable: true},
	{Key: PermPaymentsAccept, Scope: "company", RequiresMFA: true, Assignable: true},
}

// Company is what commerce needs to know about another company.
type Company struct {
	ID, Name, Kind, Country string
	Active                  bool
}

// Files shares uploaded files with another company (implemented by documents).
type Files interface {
	Share(ctx context.Context, ownerCompanyID, fileID, withCompanyID, resourceType, resourceID string) error
}

// Directory looks up companies (implemented by the identity module).
// It returns apperr.ErrNotFound for unknown IDs.
type Directory interface {
	Company(ctx context.Context, id string) (*Company, error)
}

// ---- model ----

type PartnershipStatus string

const (
	Requested PartnershipStatus = "requested"
	Active    PartnershipStatus = "active"
	Declined  PartnershipStatus = "declined"
	Withdrawn PartnershipStatus = "withdrawn"
	Ended     PartnershipStatus = "ended"
)

type Partnership struct {
	ID                 string `gorm:"primaryKey;type:uuid"`
	RequesterCompanyID string `gorm:"type:uuid"`
	RecipientCompanyID string `gorm:"type:uuid"`
	Status             PartnershipStatus
	StatusReason       string
	RequestedBy        string `gorm:"type:uuid"`
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ActivatedAt        *time.Time
	ClosedAt           *time.Time
}

func (Partnership) TableName() string { return "commerce.partnerships" }

// Counterparty returns the other company from companyID's point of view.
func (p *Partnership) Counterparty(companyID string) string {
	if p.RequesterCompanyID == companyID {
		return p.RecipientCompanyID
	}
	return p.RequesterCompanyID
}

// AllowedActions lists what companyID may do with the partnership now.
func (p *Partnership) AllowedActions(companyID string) []string {
	switch {
	case p.Status == Requested && p.RecipientCompanyID == companyID:
		return []string{"accept", "decline"}
	case p.Status == Requested:
		return []string{"withdraw"}
	case p.Status == Active:
		return []string{"end"}
	}
	return []string{}
}

type Event struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	Seq          int64  `gorm:"->"`
	CompanyID    string `gorm:"type:uuid"`
	EventType    string
	ResourceType string
	ResourceID   string `gorm:"type:uuid"`
	ActorUserID  string `gorm:"type:uuid"`
	OccurredAt   time.Time
	Reason       string
	Details      []byte `gorm:"type:jsonb"`
}

func (Event) TableName() string { return "commerce.events" }

// ---- repository ----

type Store interface {
	Partnerships() PartnershipRepository
	Offers() OfferRepository
	Deals() DealRepository
	Fulfilment() FulfilmentRepository
	Invoices() InvoiceRepository
	Events() EventRepository
	InTx(ctx context.Context, fn func(Store) error) error
	// Bind returns ctx carrying this store's transaction, so other modules
	// called through ports commit or roll back together with it.
	Bind(ctx context.Context) context.Context
}

type gormStore struct{ db *gorm.DB }

func NewStore(db *gorm.DB) Store { return &gormStore{db: db} }

func (s *gormStore) Partnerships() PartnershipRepository { return &partnershipRepository{s.db} }
func (s *gormStore) Events() EventRepository             { return &eventRepository{s.db} }
func (s *gormStore) Offers() OfferRepository             { return &offerRepository{s.db} }
func (s *gormStore) Deals() DealRepository               { return &dealRepository{s.db} }
func (s *gormStore) Fulfilment() FulfilmentRepository    { return &fulfilmentRepository{s.db} }
func (s *gormStore) Invoices() InvoiceRepository         { return &invoiceRepository{s.db} }
func (s *gormStore) InTx(ctx context.Context, fn func(Store) error) error {
	return database.Conn(ctx, s.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(&gormStore{db: tx}) })
}

func (s *gormStore) Bind(ctx context.Context) context.Context { return database.WithTx(ctx, s.db) }

type PartnershipFilter struct {
	CompanyID string
	Status    PartnershipStatus
	Limit     int
	Offset    int
}

type PartnershipRepository interface {
	Create(ctx context.Context, p *Partnership) error
	// Get returns a partnership the company takes part in, else ErrNotFound.
	Get(ctx context.Context, companyID, id string) (*Partnership, error)
	List(ctx context.Context, f PartnershipFilter) ([]Partnership, error)
	Update(ctx context.Context, p *Partnership, expected int64) error
	// ActiveBetween reports whether the two companies have an active partnership.
	ActiveBetween(ctx context.Context, a, b string) (bool, error)
}

type partnershipRepository struct{ db *gorm.DB }

func (r *partnershipRepository) Create(ctx context.Context, p *Partnership) error {
	return database.Translate(r.db.WithContext(ctx).Create(p).Error)
}

func (r *partnershipRepository) involving(ctx context.Context, companyID string) *gorm.DB {
	return r.db.WithContext(ctx).Where("(requester_company_id = ? OR recipient_company_id = ?)", companyID, companyID)
}

func (r *partnershipRepository) Get(ctx context.Context, companyID, id string) (*Partnership, error) {
	var p Partnership
	if err := r.involving(ctx, companyID).Where("id = ?", id).Take(&p).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &p, nil
}

func (r *partnershipRepository) List(ctx context.Context, f PartnershipFilter) ([]Partnership, error) {
	limit, offset := database.Page(f.Limit, f.Offset)
	q := r.involving(ctx, f.CompanyID).Order("updated_at DESC, id").Limit(limit).Offset(offset)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	ps := []Partnership{}
	if err := database.Translate(q.Find(&ps).Error); err != nil {
		return nil, err
	}
	return ps, nil
}

func (r *partnershipRepository) Update(ctx context.Context, p *Partnership, expected int64) error {
	err := database.UpdateVersioned(r.db.WithContext(ctx), &Partnership{}, p.ID, expected, map[string]any{
		"status": p.Status, "status_reason": p.StatusReason, "updated_at": p.UpdatedAt,
		"activated_at": p.ActivatedAt, "closed_at": p.ClosedAt,
	})
	if err == nil {
		p.Version = expected + 1
	}
	return err
}

func (r *partnershipRepository) ActiveBetween(ctx context.Context, a, b string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&Partnership{}).Where(
		"status = ? AND ((requester_company_id = ? AND recipient_company_id = ?) OR (requester_company_id = ? AND recipient_company_id = ?))",
		Active, a, b, b, a).Count(&n).Error
	return n > 0, database.Translate(err)
}

type EventRepository interface {
	Append(ctx context.Context, e *Event) error
	ForResource(ctx context.Context, resourceType, id string) ([]Event, error)
}

type eventRepository struct{ db *gorm.DB }

func (r *eventRepository) Append(ctx context.Context, e *Event) error {
	return database.Translate(r.db.WithContext(ctx).Create(e).Error)
}

func (r *eventRepository) ForResource(ctx context.Context, resourceType, id string) ([]Event, error) {
	es := []Event{}
	err := r.db.WithContext(ctx).Where("resource_type = ? AND resource_id = ?", resourceType, id).Order("seq").Find(&es).Error
	return es, database.Translate(err)
}

// ---- service ----

type deps struct {
	store     Store
	directory Directory
	catalog   Catalog
	stock     Stock
	files     Files
	now       func() time.Time
}

func (d deps) clock() time.Time { return d.now().UTC() }

func (d deps) event(ctx context.Context, st Store, p *auth.Principal, eventType, resourceType, resourceID, reason string, details map[string]any) error {
	raw := []byte("{}")
	if details != nil {
		raw, _ = json.Marshal(details)
	}
	return st.Events().Append(ctx, &Event{
		ID: uuid.NewString(), CompanyID: p.CompanyID, EventType: eventType,
		ResourceType: resourceType, ResourceID: resourceID, ActorUserID: p.UserID, OccurredAt: d.clock(),
		Reason: reason, Details: raw,
	})
}

// tradingCompany checks that a company exists, sells vehicles and has active
// platform access. Unknown companies surface as field errors, not 404s.
func (d deps) tradingCompany(ctx context.Context, id, field string) (*Company, error) {
	c, err := d.directory.Company(ctx, id)
	if errors.Is(err, apperr.ErrNotFound) {
		return nil, apperr.FieldError(field, "company not found")
	}
	if err != nil {
		return nil, err
	}
	if c.Kind != "seller" || !c.Active {
		return nil, apperr.New(apperr.ErrConflict, "company_not_trading", c.Name+" cannot trade: it must be an active seller company")
	}
	return c, nil
}

type PartnershipService struct{ deps }

// PartnershipView is a partnership with the other company's name.
type PartnershipView struct {
	Partnership  Partnership
	Counterparty Company
}

func (s *PartnershipService) view(ctx context.Context, companyID string, p *Partnership) (*PartnershipView, error) {
	c, err := s.directory.Company(ctx, p.Counterparty(companyID))
	if err != nil {
		return nil, err
	}
	return &PartnershipView{Partnership: *p, Counterparty: *c}, nil
}

// Request asks another seller company for a partnership. Both companies must
// be active sellers and may have only one open partnership with each other.
func (s *PartnershipService) Request(ctx context.Context, p *auth.Principal, counterpartyID string) (*PartnershipView, error) {
	if validate.IDs(counterpartyID) != nil {
		return nil, apperr.FieldError("counterpartyCompanyId", "must be a valid ID")
	}
	if counterpartyID == p.CompanyID {
		return nil, apperr.FieldError("counterpartyCompanyId", "cannot partner with your own company")
	}
	if _, err := s.tradingCompany(ctx, p.CompanyID, "companyId"); err != nil {
		return nil, err
	}
	other, err := s.tradingCompany(ctx, counterpartyID, "counterpartyCompanyId")
	if err != nil {
		return nil, err
	}
	now := s.clock()
	ps := &Partnership{
		ID: uuid.NewString(), RequesterCompanyID: p.CompanyID, RecipientCompanyID: counterpartyID,
		Status: Requested, RequestedBy: p.UserID, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	err = s.store.InTx(ctx, func(st Store) error {
		if err := st.Partnerships().Create(ctx, ps); err != nil {
			if errors.Is(err, apperr.ErrConflict) {
				return apperr.New(apperr.ErrConflict, "partnership_exists", "a partnership with this company is already requested or active")
			}
			return err
		}
		return s.event(ctx, st, p, "partnership.requested", "partnership", ps.ID, "", map[string]any{"counterpartyCompanyId": counterpartyID})
	})
	if err != nil {
		return nil, err
	}
	return &PartnershipView{Partnership: *ps, Counterparty: *other}, nil
}

func (s *PartnershipService) Get(ctx context.Context, p *auth.Principal, id string) (*PartnershipView, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	ps, err := s.store.Partnerships().Get(ctx, p.CompanyID, id)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, p.CompanyID, ps)
}

func (s *PartnershipService) List(ctx context.Context, p *auth.Principal, f PartnershipFilter) ([]PartnershipView, error) {
	switch f.Status {
	case "", Requested, Active, Declined, Withdrawn, Ended:
	default:
		return nil, apperr.FieldError("status", "unknown status")
	}
	f.CompanyID = p.CompanyID
	ps, err := s.store.Partnerships().List(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]PartnershipView, 0, len(ps))
	for i := range ps {
		v, err := s.view(ctx, p.CompanyID, &ps[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}

// transition describes who may move a partnership from one state to another.
type transition struct {
	from, to  PartnershipStatus
	recipient bool // only the recipient (else only the requester, unless any)
	any       bool // either company
	reason    bool // reason required
}

var transitions = map[string]transition{
	"accept":   {from: Requested, to: Active, recipient: true},
	"decline":  {from: Requested, to: Declined, recipient: true, reason: true},
	"withdraw": {from: Requested, to: Withdrawn, reason: true},
	"end":      {from: Active, to: Ended, any: true, reason: true},
}

// Decide applies accept, decline, withdraw or end. Ending keeps all earlier
// contractual records; it only stops new offers and orders.
func (s *PartnershipService) Decide(ctx context.Context, p *auth.Principal, id string, expected int64, action, why string) (*PartnershipView, error) {
	t, ok := transitions[action]
	if !ok {
		return nil, apperr.ErrNotFound
	}
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	var v apperr.Validation
	if t.reason {
		why = validate.Reason(&v, why)
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *Partnership
	err := s.store.InTx(ctx, func(st Store) error {
		ps, err := st.Partnerships().Get(ctx, p.CompanyID, id)
		if err != nil {
			return err
		}
		if ps.Version != expected {
			return apperr.ErrStale
		}
		isRecipient := ps.RecipientCompanyID == p.CompanyID
		if !t.any && t.recipient != isRecipient {
			side := "the requesting company"
			if t.recipient {
				side = "the invited company"
			}
			return apperr.New(apperr.ErrForbidden, "wrong_party", "only "+side+" can "+action)
		}
		if ps.Status != t.from {
			return apperr.New(apperr.ErrConflict, "invalid_transition", "cannot "+action+" a partnership that is "+string(ps.Status))
		}
		now := s.clock()
		ps.Status, ps.StatusReason, ps.UpdatedAt = t.to, why, now
		if t.to == Active {
			ps.ActivatedAt = &now
		} else {
			ps.ClosedAt = &now
		}
		if err := st.Partnerships().Update(ctx, ps, expected); err != nil {
			return err
		}
		result = ps
		return s.event(ctx, st, p, "partnership."+string(t.to), "partnership", ps.ID, why, nil)
	})
	if err != nil {
		return nil, err
	}
	return s.view(ctx, p.CompanyID, result)
}
