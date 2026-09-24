package service

import (
	"context"

	"justixauto/internal/modules/financing/model"
	"justixauto/internal/pkg/money"
)

// Repository is financing's persistence port.
type Repository interface {
	CreateProgram(ctx context.Context, p *model.Program) error
	Program(ctx context.Context, id string) (*model.Program, error)
	Programs(ctx context.Context, providerID string, publishedOnly bool, limit, offset int) ([]model.Program, error)
	UpdateProgram(ctx context.Context, p *model.Program, expected int64) error

	CreateProgramVersion(ctx context.Context, v *model.ProgramVersion) error
	ProgramVersion(ctx context.Context, id string, number int) (*model.ProgramVersion, error)
	ProgramVersions(ctx context.Context, id string) ([]model.ProgramVersion, error)
	NextProgramVersionNumber(ctx context.Context, programID string) (int, error)

	CreateApplication(ctx context.Context, a *model.Application) error
	Application(ctx context.Context, companyID, id string) (*model.Application, error)
	Applications(ctx context.Context, companyID, status string, limit, offset int) ([]model.Application, error)
	UpdateApplication(ctx context.Context, a *model.Application, expected int64) error

	CreateTermsVersion(ctx context.Context, t *model.TermsVersion) error
	TermsVersions(ctx context.Context, applicationID string) ([]model.TermsVersion, error)
	NextTermsVersionNumber(ctx context.Context, applicationID string) (int, error)

	CreateMessage(ctx context.Context, m *model.Message) error
	Messages(ctx context.Context, applicationID string) ([]model.Message, error)

	CreateDocumentRequest(ctx context.Context, d *model.DocumentRequest) error
	DocumentRequest(ctx context.Context, id string) (*model.DocumentRequest, error)
	DocumentRequests(ctx context.Context, applicationID string) ([]model.DocumentRequest, error)
	UpdateDocumentRequest(ctx context.Context, d *model.DocumentRequest, expected int64) error

	CreateSubmission(ctx context.Context, s *model.Submission) error
	NextSubmissionNumber(ctx context.Context, requestID string) (int, error)
	Submissions(ctx context.Context, requestID string) ([]model.Submission, error)

	InTx(ctx context.Context, fn func(Repository) error) error
	// Bind carries the repository's connection (its transaction, inside
	// InTx) in ctx, so a call into another module through a port joins it.
	Bind(ctx context.Context) context.Context
}

// Sale is what financing may know about a retail sale (from retail).
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

type Company struct {
	ID, Name, Kind string
	Active         bool
}

// Directory answers who a provider company is (implemented by identity).
type Directory interface {
	Company(ctx context.Context, id string) (*Company, error)
}

// Files shares an uploaded file with another company (implemented by the
// documents module). It fails with apperr.ErrNotFound if the file is not the
// owner's.
type Files interface {
	Share(ctx context.Context, ownerCompanyID, fileID, withCompanyID, resourceType, resourceID string) error
}
