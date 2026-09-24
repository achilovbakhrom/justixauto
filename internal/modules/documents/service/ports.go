package service

import (
	"context"
	"io"

	"justixauto/internal/modules/documents/model"
)

// Store gives the service the documents repositories.
type Store interface {
	Create(ctx context.Context, f *model.File) error
	// ByContent returns the company's file with this purpose and content hash.
	ByContent(ctx context.Context, companyID, purpose, sha256 string) (*model.File, error)
	// Readable returns the file if the company owns it or it was shared with it.
	Readable(ctx context.Context, companyID, id string) (*model.File, error)
	// Share joins the caller's transaction when ctx carries one.
	Share(ctx context.Context, ownerCompanyID, fileID string, sh *model.Share) (*model.File, error)
}

// Storage keeps file bytes: S3 in production, a private directory locally.
// Keys are opaque and never leave the module.
type Storage interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}
