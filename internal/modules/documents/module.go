// Package documents stores uploaded files privately and controls who may read
// them. A file belongs to the uploading company; other modules share it with a
// counterparty for a specific resource. Stored bytes never change. There is no
// malware scanning yet: a stored file is not "clean" and not domain-accepted.
//
// This package is the only one other code imports; internal/modules/documents
// splits into model (GORM models, value types), repository (GORM persistence
// and byte storage backends), service (business rules) and handler (Echo
// routes) layers, wired together here.
package documents

import (
	"context"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/modules/documents/handler"
	"justixauto/internal/modules/documents/model"
	"justixauto/internal/modules/documents/repository"
	"justixauto/internal/modules/documents/service"
)

const (
	PermRead              = model.PermRead
	PermUpload            = model.PermUpload
	PermSensitiveDownload = model.PermSensitiveDownload
)

// Permissions are registered in the identity catalog at startup.
var Permissions = model.Permissions

// Storage keeps file bytes: S3 in production, a private directory locally.
// Keys are opaque and never leave the module.
type Storage = service.Storage

// DirStorage stores files in a private local directory (development, tests).
type DirStorage = repository.DirStorage

// S3Config selects the bucket for S3Storage.
type S3Config = repository.S3Config

// S3Storage keeps file bytes in a private S3 bucket.
type S3Storage = repository.S3Storage

// NewS3Storage builds an S3-backed Storage.
func NewS3Storage(ctx context.Context, cfg S3Config) (*S3Storage, error) {
	return repository.NewS3Storage(ctx, cfg)
}

// Service uploads, reads and shares files.
type Service = service.Service

// Module wires documents' layers.
type Module struct {
	handler *handler.Handler
	Service *Service
}

// New builds the documents module.
func New(db *gorm.DB, now func() time.Time, storage Storage) *Module {
	if now == nil {
		now = time.Now
	}
	s := service.New(repository.NewStore(db), storage, now)
	return &Module{handler: handler.New(s), Service: s}
}

// Register mounts the document routes under /api/v1/documents.
func (m *Module) Register(api *echo.Group) { m.handler.Routes(api.Group("/documents")) }
