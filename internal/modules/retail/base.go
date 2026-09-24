// Package retail owns the seller's retail business: customers, the CRM
// (leads, contacts, tasks), vehicle listings and retail sales (deals) with
// their invoices, payment evidence, registration and delivery.
package retail

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/database"
)

const (
	PermRead           = "retail.read"
	PermCRM            = "retail.crm.manage"
	PermListings       = "retail.listings.manage"
	PermDeals          = "retail.deals.manage"
	PermPaymentsAccept = "retail.payments.accept" // sensitive: needs fresh MFA
	PermDeliver        = "retail.deals.deliver"   // sensitive: needs fresh MFA
)

var Permissions = []auth.PermissionInfo{
	{Key: PermRead, Scope: "company", Assignable: true},
	{Key: PermCRM, Scope: "company", Assignable: true},
	{Key: PermListings, Scope: "company", Assignable: true},
	{Key: PermDeals, Scope: "company", Assignable: true},
	{Key: PermPaymentsAccept, Scope: "company", RequiresMFA: true, Assignable: true},
	{Key: PermDeliver, Scope: "company", RequiresMFA: true, Assignable: true},
}

// Company answers identity questions (implemented by identity).
type Company interface {
	IsMember(ctx context.Context, userID, companyID string) (bool, error)
	BranchOf(ctx context.Context, companyID, branchID string) (bool, error)
}

// Vehicle is what retail may know about a vehicle.
type Vehicle struct {
	ID, VIN, ModelID string
	Owned            bool // owned by the asking company
	InWarehouse      bool
}

// Stock reserves and delivers vehicles (implemented by inventory). Calls with
// a ctx from Store.Bind join the retail transaction.
type Stock interface {
	Vehicle(ctx context.Context, companyID, id string) (*Vehicle, error)
	Reserve(ctx context.Context, companyID, dealID, vehicleID string) error
	Release(ctx context.Context, dealID, reason string) error
	// Deliver hands the vehicle to the retail customer: it leaves stock.
	Deliver(ctx context.Context, dealID, vehicleID, actorID string, at time.Time) error
}

// Event is an append-only history entry of a retail resource.
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

func (Event) TableName() string { return "retail.events" }

// Store: retail repositories plus transactions.
type Store interface {
	CRM() CRMRepository
	Listings() ListingRepository
	Deals() DealRepository
	Events() EventRepository
	InTx(ctx context.Context, fn func(Store) error) error
	Bind(ctx context.Context) context.Context
}

type gormStore struct{ db *gorm.DB }

func NewStore(db *gorm.DB) Store { return &gormStore{db: db} }

func (s *gormStore) CRM() CRMRepository                       { return &crmRepository{s.db} }
func (s *gormStore) Listings() ListingRepository              { return &listingRepository{s.db} }
func (s *gormStore) Deals() DealRepository                    { return &dealRepository{s.db} }
func (s *gormStore) Events() EventRepository                  { return &eventRepository{s.db} }
func (s *gormStore) Bind(ctx context.Context) context.Context { return database.WithTx(ctx, s.db) }
func (s *gormStore) InTx(ctx context.Context, fn func(Store) error) error {
	return database.Conn(ctx, s.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(&gormStore{db: tx}) })
}

type EventRepository interface {
	Append(ctx context.Context, e *Event) error
	For(ctx context.Context, resourceType, id string) ([]Event, error)
}

type eventRepository struct{ db *gorm.DB }

func (r *eventRepository) Append(ctx context.Context, e *Event) error {
	return database.Translate(r.db.WithContext(ctx).Create(e).Error)
}

func (r *eventRepository) For(ctx context.Context, resourceType, id string) ([]Event, error) {
	es := []Event{}
	err := r.db.WithContext(ctx).Where("resource_type = ? AND resource_id = ?", resourceType, id).Order("seq").Find(&es).Error
	return es, database.Translate(err)
}

// Files checks and shares uploaded files (implemented by documents).
type Files interface {
	Share(ctx context.Context, ownerCompanyID, fileID, withCompanyID, resourceType, resourceID string) error
}

type deps struct {
	store   Store
	company Company
	stock   Stock
	files   Files
	now     func() time.Time
}

func (d deps) clock() time.Time { return d.now().UTC() }

func (d deps) event(ctx context.Context, st Store, p *auth.Principal, eventType, resourceType, id, reason string, details map[string]any) error {
	raw := []byte("{}")
	if details != nil {
		raw, _ = json.Marshal(details)
	}
	return st.Events().Append(ctx, &Event{
		ID: uuid.NewString(), CompanyID: p.CompanyID, EventType: eventType,
		ResourceType: resourceType, ResourceID: id, ActorUserID: p.UserID, OccurredAt: d.clock(), Reason: reason, Details: raw,
	})
}

// branch checks that a branch belongs to the active company and is inside the
// session's branch scope.
func (d deps) branch(ctx context.Context, p *auth.Principal, field, branchID string) error {
	ok, err := d.company.BranchOf(ctx, p.CompanyID, branchID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.FieldError(field, "not a branch of your company")
	}
	if !inScope(p, branchID) {
		return apperr.FieldError(field, "outside your selected branches")
	}
	return nil
}

// inScope applies the session's branch scope (ALL or SELECTED).
func inScope(p *auth.Principal, branchID string) bool {
	if p.BranchScope.Mode != "SELECTED" {
		return true
	}
	for _, b := range p.BranchScope.BranchIDs {
		if b == branchID {
			return true
		}
	}
	return false
}

// scopeBranches returns the branch filter for lists (nil = all).
func scopeBranches(p *auth.Principal) []string {
	if p.BranchScope.Mode == "SELECTED" {
		return p.BranchScope.BranchIDs
	}
	return nil
}

func (d deps) member(ctx context.Context, p *auth.Principal, field, userID string) error {
	ok, err := d.company.IsMember(ctx, userID, p.CompanyID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.FieldError(field, "not a member of your company")
	}
	return nil
}
