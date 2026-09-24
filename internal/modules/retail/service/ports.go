package service

import (
	"context"
	"time"

	"justixauto/internal/modules/retail/model"
)

// Store gives services the retail repositories and one-transaction runs.
type Store interface {
	CRM() CRMRepository
	Listings() ListingRepository
	Deals() DealRepository
	Events() EventRepository
	InTx(ctx context.Context, fn func(Store) error) error
	// Bind carries the store's connection in ctx, so a call into another
	// module through a port joins it.
	Bind(ctx context.Context) context.Context
}

type CRMRepository interface {
	CreateCustomer(ctx context.Context, c *model.Customer) error
	Customer(ctx context.Context, companyID, id string) (*model.Customer, error)
	Customers(ctx context.Context, companyID, query string, limit, offset int) ([]model.Customer, error)
	UpdateCustomer(ctx context.Context, c *model.Customer, expected int64) error
	CreateLead(ctx context.Context, l *model.Lead) error
	Lead(ctx context.Context, companyID, id string) (*model.Lead, error)
	Leads(ctx context.Context, f model.LeadFilter) ([]model.Lead, error)
	UpdateLead(ctx context.Context, l *model.Lead, expected int64) error
	AddContact(ctx context.Context, c *model.Contact) error
	Contacts(ctx context.Context, leadID string) ([]model.Contact, error)
	CreateTask(ctx context.Context, t *model.Task) error
	Task(ctx context.Context, companyID, id string) (*model.Task, error)
	Tasks(ctx context.Context, companyID, ownerID, status string, limit, offset int) ([]model.Task, error)
	UpdateTask(ctx context.Context, t *model.Task, expected int64) error
}

type ListingRepository interface {
	Create(ctx context.Context, l *model.Listing) error
	Get(ctx context.Context, companyID, id string) (*model.Listing, error)
	List(ctx context.Context, companyID, status string, limit, offset int) ([]model.Listing, error)
	Update(ctx context.Context, l *model.Listing, expected int64) error
	// WithdrawForVehicle closes open listings of a delivered vehicle.
	WithdrawForVehicle(ctx context.Context, vehicleID string, at time.Time) error
}

type DealRepository interface {
	Create(ctx context.Context, d *model.Deal) error
	Deal(ctx context.Context, companyID, id string) (*model.Deal, error)
	Deals(ctx context.Context, companyID string, branchIDs []string, status string, limit, offset int) ([]model.Deal, error)
	Update(ctx context.Context, d *model.Deal, expected int64) error
	CreateInvoice(ctx context.Context, i *model.Invoice) error
	Invoice(ctx context.Context, companyID, id string) (*model.Invoice, error)
	Invoices(ctx context.Context, dealID string) ([]model.Invoice, error)
	AddEvidence(ctx context.Context, e *model.Evidence) error
	Evidence(ctx context.Context, invoiceID string) ([]model.Evidence, error)
	GetEvidence(ctx context.Context, id string) (*model.Evidence, error)
	UpdateEvidence(ctx context.Context, e *model.Evidence, expected int64) error
}

type EventRepository interface {
	Append(ctx context.Context, e *model.Event) error
	For(ctx context.Context, resourceType, id string) ([]model.Event, error)
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

// Files checks and shares uploaded files (implemented by documents).
type Files interface {
	Share(ctx context.Context, ownerCompanyID, fileID, withCompanyID, resourceType, resourceID string) error
}

// Insurance tells whether the insurer approved the deal's application
// (implemented by the insurance module).
type Insurance interface {
	Approved(ctx context.Context, companyID, dealID string) (bool, error)
}
