package service

import (
	"context"

	"justixauto/internal/modules/insurance/model"
	"justixauto/internal/pkg/money"
)

// Repository is insurance's persistence port.
type Repository interface {
	Create(ctx context.Context, a *model.Application) error
	// Get returns an application visible to the company: the seller always,
	// the addressed insurer once it was submitted.
	Get(ctx context.Context, companyID, id string) (*model.Application, error)
	List(ctx context.Context, companyID, status string, limit, offset int) ([]model.Application, error)
	ForDeal(ctx context.Context, sellerID, dealID string) (*model.Application, error)
	Update(ctx context.Context, a *model.Application, expected int64) error
	AddMessage(ctx context.Context, m *model.Message) error
	Messages(ctx context.Context, applicationID string) ([]model.Message, error)
	InTx(ctx context.Context, fn func(Repository) error) error
}

// Sale is what insurance may know about a retail sale (from retail).
type Sale struct {
	ID, VehicleID, PaymentScheme, Status string
	// What the provider sees about the sale: vehicle and client (OD-10 decides
	// any further personal data).
	VIN, Model, CustomerName string
	Price                    money.Money
	Revision                 int64
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

// Directory answers who an insurer company is (implemented by identity).
type Directory interface {
	Company(ctx context.Context, id string) (*Company, error)
}
