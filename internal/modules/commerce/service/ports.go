package service

import (
	"context"
	"time"

	"justixauto/internal/modules/commerce/model"
)

// Store gives services the commerce repositories and one-transaction runs.
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

type PartnershipRepository interface {
	Create(ctx context.Context, p *model.Partnership) error
	// Get returns a partnership the company takes part in, else ErrNotFound.
	Get(ctx context.Context, companyID, id string) (*model.Partnership, error)
	List(ctx context.Context, f model.PartnershipFilter) ([]model.Partnership, error)
	Update(ctx context.Context, p *model.Partnership, expected int64) error
	// ActiveBetween reports whether the two companies have an active partnership.
	ActiveBetween(ctx context.Context, a, b string) (bool, error)
}

type EventRepository interface {
	Append(ctx context.Context, e *model.Event) error
	ForResource(ctx context.Context, resourceType, id string) ([]model.Event, error)
}

type OfferRepository interface {
	Create(ctx context.Context, o *model.Offer, v *model.OfferVersion) error
	// GetOwn returns the supplier's own offer, else ErrNotFound.
	GetOwn(ctx context.Context, supplierID, id string) (*model.Offer, error)
	// GetVisible returns a published offer the buyer may see, else ErrNotFound.
	GetVisible(ctx context.Context, buyerID, id string) (*model.Offer, error)
	// VisibleByPublishedVersion finds the visible offer whose published version is versionID.
	VisibleByPublishedVersion(ctx context.Context, buyerID, versionID string) (*model.Offer, error)
	ListOwn(ctx context.Context, supplierID string, limit, offset int) ([]model.Offer, error)
	ListVisible(ctx context.Context, buyerID string, limit, offset int) ([]model.Offer, error)
	Versions(ctx context.Context, offerID string) ([]model.OfferVersion, error)
	Version(ctx context.Context, offerID, versionID string) (*model.OfferVersion, error)
	AddVersion(ctx context.Context, v *model.OfferVersion) error
	Update(ctx context.Context, o *model.Offer, expected int64) error
	MarkPublished(ctx context.Context, versionID string, at time.Time) error
}

type DealRepository interface {
	CreateRFQ(ctx context.Context, r *model.RFQ) error
	// RFQ returns an RFQ visible to the company: the buyer always, the
	// supplier once it was sent.
	RFQ(ctx context.Context, companyID, id string) (*model.RFQ, error)
	RFQs(ctx context.Context, companyID string, limit, offset int) ([]model.RFQ, error)
	UpdateRFQ(ctx context.Context, r *model.RFQ, expected int64) error
	AddQuotation(ctx context.Context, q *model.Quotation) error
	Quotations(ctx context.Context, rfqID string) ([]model.Quotation, error)

	CreateOrder(ctx context.Context, o *model.Order) error
	// Order returns an order the company is a party of.
	Order(ctx context.Context, companyID, id string) (*model.Order, error)
	Orders(ctx context.Context, companyID string, limit, offset int) ([]model.Order, error)
	UpdateOrder(ctx context.Context, o *model.Order, expected int64) error
	AddAddendum(ctx context.Context, a *model.Addendum) error
	Addenda(ctx context.Context, orderID string) ([]model.Addendum, error)
	DecideAddendum(ctx context.Context, a *model.Addendum) error
}

type FulfilmentRepository interface {
	Allocations(ctx context.Context, orderID string) ([]model.Allocation, error)
	AddAllocations(ctx context.Context, as []model.Allocation) error
	SetAllocationStatus(ctx context.Context, orderID string, vehicleIDs []string, status string, shipmentID *string) error
	CreateShipment(ctx context.Context, s *model.Shipment) error
	Shipment(ctx context.Context, id string) (*model.Shipment, error)
	Shipments(ctx context.Context, orderID string) ([]model.Shipment, error)
	UpdateShipment(ctx context.Context, s *model.Shipment, expected int64) error
	AddMilestone(ctx context.Context, m *model.Milestone) error
	Milestones(ctx context.Context, shipmentID string) ([]model.Milestone, error)
}

type InvoiceRepository interface {
	Create(ctx context.Context, i *model.Invoice) error
	// Get returns an invoice the company is a party of.
	Get(ctx context.Context, companyID, id string) (*model.Invoice, error)
	ForOrder(ctx context.Context, orderID string) ([]model.Invoice, error)
	Update(ctx context.Context, i *model.Invoice, expected int64) error
	AddEvidence(ctx context.Context, e *model.Evidence) error
	Evidence(ctx context.Context, invoiceID string) ([]model.Evidence, error)
	GetEvidence(ctx context.Context, id string) (*model.Evidence, error)
	UpdateEvidence(ctx context.Context, e *model.Evidence, expected int64) error
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

// Model is what commerce needs to know about a catalogue model.
type Model struct{ ID, Name string }

// Catalog looks up vehicle models (implemented by the inventory module).
// It returns apperr.ErrNotFound for unknown IDs.
type Catalog interface {
	Model(ctx context.Context, id string) (*Model, error)
}

// StockVehicle is what commerce needs to know about a concrete vehicle.
type StockVehicle struct{ ID, VIN, ModelID string }

// Stock reserves and hands over vehicles (implemented by inventory). Calls
// made with a ctx from Store.Bind join the commerce transaction.
type Stock interface {
	Vehicle(ctx context.Context, companyID, id string) (*StockVehicle, error)
	Reserve(ctx context.Context, companyID, orderID string, vehicleIDs []string) error
	Release(ctx context.Context, orderID string, vehicleIDs []string, reason string) error
	Transfer(ctx context.Context, orderID string, vehicleIDs []string, toCompanyID, toWarehouseID, actorID string, at time.Time) error
}
