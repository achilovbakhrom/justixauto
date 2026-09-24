// Package financing connects sellers with banks and MFOs: providers publish
// programs; sellers send a partner-finance sale with a calculation; the
// provider reviews, proposes terms and the seller agrees. Agreement is not a
// contract, signature, funding or permission to deliver.
//
// This package is the only one other code imports; internal/modules/financing
// splits into model (GORM models, value types), repository (GORM
// persistence), service (business rules) and handler (Echo routes) layers,
// wired together here.
package financing

import (
	"context"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/modules/financing/handler"
	"justixauto/internal/modules/financing/model"
	"justixauto/internal/modules/financing/repository"
	"justixauto/internal/modules/financing/service"
)

const (
	PermRead     = model.PermRead
	PermPrograms = model.PermPrograms
	PermApply    = model.PermApply
	PermAgree    = model.PermAgree
	PermReview   = model.PermReview
	PermDecide   = model.PermDecide
)

// Permissions are registered in the identity catalog at startup.
var Permissions = model.Permissions

// Sale is what financing may know about a retail sale (from retail).
type Sale = service.Sale

// Sales reads the seller's sales (implemented by retail).
type Sales = service.Sales

// Company is a provider's public profile (from identity).
type Company = service.Company

// Directory answers who a provider company is (implemented by identity).
type Directory = service.Directory

// Files shares an uploaded file with another company (implemented by the
// documents module).
type Files = service.Files

// storeAdapter bridges repository.Store (which cannot import service, since
// repository implements service's interfaces structurally) to
// service.Repository, whose InTx is self-referential and so needs the exact
// service.Repository type.
type storeAdapter struct{ r *repository.Store }

func (a storeAdapter) CreateProgram(ctx context.Context, p *model.Program) error {
	return a.r.CreateProgram(ctx, p)
}

func (a storeAdapter) Program(ctx context.Context, id string) (*model.Program, error) {
	return a.r.Program(ctx, id)
}

func (a storeAdapter) Programs(ctx context.Context, providerID string, publishedOnly bool, limit, offset int) ([]model.Program, error) {
	return a.r.Programs(ctx, providerID, publishedOnly, limit, offset)
}

func (a storeAdapter) UpdateProgram(ctx context.Context, p *model.Program, expected int64) error {
	return a.r.UpdateProgram(ctx, p, expected)
}

func (a storeAdapter) CreateProgramVersion(ctx context.Context, v *model.ProgramVersion) error {
	return a.r.CreateProgramVersion(ctx, v)
}

func (a storeAdapter) ProgramVersion(ctx context.Context, id string, number int) (*model.ProgramVersion, error) {
	return a.r.ProgramVersion(ctx, id, number)
}

func (a storeAdapter) ProgramVersions(ctx context.Context, id string) ([]model.ProgramVersion, error) {
	return a.r.ProgramVersions(ctx, id)
}

func (a storeAdapter) NextProgramVersionNumber(ctx context.Context, programID string) (int, error) {
	return a.r.NextProgramVersionNumber(ctx, programID)
}

func (a storeAdapter) CreateApplication(ctx context.Context, ap *model.Application) error {
	return a.r.CreateApplication(ctx, ap)
}

func (a storeAdapter) Application(ctx context.Context, companyID, id string) (*model.Application, error) {
	return a.r.Application(ctx, companyID, id)
}

func (a storeAdapter) Applications(ctx context.Context, companyID, status string, limit, offset int) ([]model.Application, error) {
	return a.r.Applications(ctx, companyID, status, limit, offset)
}

func (a storeAdapter) UpdateApplication(ctx context.Context, ap *model.Application, expected int64) error {
	return a.r.UpdateApplication(ctx, ap, expected)
}

func (a storeAdapter) CreateTermsVersion(ctx context.Context, t *model.TermsVersion) error {
	return a.r.CreateTermsVersion(ctx, t)
}

func (a storeAdapter) TermsVersions(ctx context.Context, applicationID string) ([]model.TermsVersion, error) {
	return a.r.TermsVersions(ctx, applicationID)
}

func (a storeAdapter) NextTermsVersionNumber(ctx context.Context, applicationID string) (int, error) {
	return a.r.NextTermsVersionNumber(ctx, applicationID)
}

func (a storeAdapter) CreateMessage(ctx context.Context, m *model.Message) error {
	return a.r.CreateMessage(ctx, m)
}

func (a storeAdapter) Messages(ctx context.Context, applicationID string) ([]model.Message, error) {
	return a.r.Messages(ctx, applicationID)
}

func (a storeAdapter) CreateDocumentRequest(ctx context.Context, d *model.DocumentRequest) error {
	return a.r.CreateDocumentRequest(ctx, d)
}

func (a storeAdapter) DocumentRequest(ctx context.Context, id string) (*model.DocumentRequest, error) {
	return a.r.DocumentRequest(ctx, id)
}

func (a storeAdapter) DocumentRequests(ctx context.Context, applicationID string) ([]model.DocumentRequest, error) {
	return a.r.DocumentRequests(ctx, applicationID)
}

func (a storeAdapter) UpdateDocumentRequest(ctx context.Context, d *model.DocumentRequest, expected int64) error {
	return a.r.UpdateDocumentRequest(ctx, d, expected)
}

func (a storeAdapter) CreateSubmission(ctx context.Context, s *model.Submission) error {
	return a.r.CreateSubmission(ctx, s)
}

func (a storeAdapter) NextSubmissionNumber(ctx context.Context, requestID string) (int, error) {
	return a.r.NextSubmissionNumber(ctx, requestID)
}

func (a storeAdapter) Submissions(ctx context.Context, requestID string) ([]model.Submission, error) {
	return a.r.Submissions(ctx, requestID)
}

func (a storeAdapter) Bind(ctx context.Context) context.Context { return a.r.Bind(ctx) }

func (a storeAdapter) InTx(ctx context.Context, fn func(service.Repository) error) error {
	return a.r.InTx(ctx, func(rs *repository.Store) error { return fn(storeAdapter{rs}) })
}

// Module wires financing's layers.
type Module struct {
	handler *handler.Handler
	Service *service.Service
}

// New builds the financing module.
func New(db *gorm.DB, now func() time.Time, sales Sales, directory Directory, files Files) *Module {
	if now == nil {
		now = time.Now
	}
	s := service.New(storeAdapter{repository.NewStore(db)}, sales, directory, files, now)
	return &Module{handler: handler.New(s), Service: s}
}

// Register mounts the financing routes under /api/v1/financing.
func (m *Module) Register(api *echo.Group) { m.handler.Routes(api.Group("/financing")) }
