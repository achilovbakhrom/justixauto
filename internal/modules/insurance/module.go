// Package insurance lets sellers send own-installment sales to an insurance
// company, which reviews them and approves or declines. A decision is only a
// decision: no policy, premium, coverage, payment or hand-over follows.
//
// This package is the only one other code imports; internal/modules/insurance
// splits into model (GORM models), repository (GORM persistence), service
// (business rules) and handler (Echo routes) layers, wired together here.
package insurance

import (
	"context"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/modules/insurance/handler"
	"justixauto/internal/modules/insurance/model"
	"justixauto/internal/modules/insurance/repository"
	"justixauto/internal/modules/insurance/service"
)

const (
	PermRead   = model.PermRead
	PermApply  = model.PermApply
	PermReview = model.PermReview
	PermDecide = model.PermDecide
)

// Permissions are registered in the identity catalog at startup.
var Permissions = model.Permissions

// Sale is what insurance may know about a retail sale (from retail).
type Sale = service.Sale

// Sales reads the seller's sales (implemented by retail).
type Sales = service.Sales

// Company is the insurer's public profile (from identity).
type Company = service.Company

// Directory answers who an insurer company is (implemented by identity).
type Directory = service.Directory

// Service reviews sellers' insurance applications on their sales.
type Service = service.Service

// storeAdapter bridges repository.Store (which cannot import service, since
// repository implements service's interfaces structurally) to
// service.Repository, whose InTx is self-referential and so needs the exact
// service.Repository type.
type storeAdapter struct{ r *repository.Store }

func (a storeAdapter) Create(ctx context.Context, ap *model.Application) error {
	return a.r.Create(ctx, ap)
}

func (a storeAdapter) Get(ctx context.Context, companyID, id string) (*model.Application, error) {
	return a.r.Get(ctx, companyID, id)
}

func (a storeAdapter) List(ctx context.Context, companyID, status string, limit, offset int) ([]model.Application, error) {
	return a.r.List(ctx, companyID, status, limit, offset)
}

func (a storeAdapter) ForDeal(ctx context.Context, sellerID, dealID string) (*model.Application, error) {
	return a.r.ForDeal(ctx, sellerID, dealID)
}

func (a storeAdapter) Update(ctx context.Context, ap *model.Application, expected int64) error {
	return a.r.Update(ctx, ap, expected)
}

func (a storeAdapter) AddMessage(ctx context.Context, m *model.Message) error {
	return a.r.AddMessage(ctx, m)
}

func (a storeAdapter) Messages(ctx context.Context, applicationID string) ([]model.Message, error) {
	return a.r.Messages(ctx, applicationID)
}

func (a storeAdapter) InTx(ctx context.Context, fn func(service.Repository) error) error {
	return a.r.InTx(ctx, func(rs *repository.Store) error { return fn(storeAdapter{rs}) })
}

// Module wires insurance's layers.
type Module struct {
	handler *handler.Handler
	Service *Service
}

// New builds the insurance module.
func New(db *gorm.DB, now func() time.Time, sales Sales, directory Directory) *Module {
	if now == nil {
		now = time.Now
	}
	s := service.New(storeAdapter{repository.NewStore(db)}, sales, directory, now)
	return &Module{handler: handler.New(s), Service: s}
}

// Register mounts the insurance routes under /api/v1/insurance.
func (m *Module) Register(api *echo.Group) { m.handler.Routes(api.Group("/insurance")) }
