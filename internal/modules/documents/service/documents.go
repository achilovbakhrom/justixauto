// Package service holds documents' business rules: uploading, reading and
// sharing files. It imports model and internal/pkg only; it must never
// import echo, gorm, repository or handler.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/modules/documents/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/validate"
)

// MaxBytes limits one upload (10 MiB).
const MaxBytes = 10 << 20

var allowedMIME = []string{"application/pdf", "image/jpeg", "image/png"}

// purposes decide sensitivity: finance and identity documents are sensitive.
var purposes = map[string]bool{
	"finance-document": true, "payment-evidence": false, "vehicle-photo": false, "deal-document": true, "other": false,
}

// Service uploads, reads and shares files.
type Service struct {
	store   Store
	storage Storage
	now     func() time.Time
}

// New builds the documents service.
func New(store Store, storage Storage, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, storage: storage, now: now}
}

// Upload stores a file for the active company. The content type is detected
// from the bytes (the client's claim is ignored) and must be PDF, JPEG or PNG.
func (s *Service) Upload(ctx context.Context, p *auth.Principal, purpose, name string, r io.Reader) (*model.File, error) {
	var v apperr.Validation
	sensitive, ok := purposes[purpose]
	if !ok {
		v.Add("purpose", "unknown purpose")
	}
	name = validate.Text(&v, "fileName", filepath.Base(strings.ReplaceAll(name, "\\", "/")), 1, 255)
	if err := v.Err(); err != nil {
		return nil, err
	}
	// Uploads are small (10 MiB): read them fully, check them, then store.
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, apperr.FieldError("file", "the file is empty")
	}
	if len(data) > MaxBytes {
		return nil, apperr.FieldError("file", "at most 10 MiB")
	}
	mime := strings.Split(http.DetectContentType(data), ";")[0]
	if !slices.Contains(allowedMIME, mime) {
		return nil, apperr.FieldError("file", "only PDF, JPEG or PNG files")
	}
	return s.upload(ctx, p, purpose, name, mime, sensitive, data)
}

// upload stores the file once per company, purpose and content: uploading
// the same bytes again (a retry, a double click) returns the existing file.
func (s *Service) upload(ctx context.Context, p *auth.Principal, purpose, name, mime string, sensitive bool, data []byte) (*model.File, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	if f, err := s.store.ByContent(ctx, p.CompanyID, purpose, hash); err == nil {
		return f, nil
	} else if !errors.Is(err, apperr.ErrNotFound) {
		return nil, err
	}
	key := strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := s.storage.Put(ctx, key, data, mime); err != nil {
		return nil, err
	}
	f := &model.File{
		ID: uuid.NewString(), CompanyID: p.CompanyID, Purpose: purpose, FileName: name, MIME: mime, ByteLength: int64(len(data)),
		SHA256: hash, StorageKey: key, Sensitive: sensitive, CreatedBy: p.UserID, CreatedAt: s.now().UTC(),
	}
	if err := s.store.Create(ctx, f); err != nil {
		_ = s.storage.Delete(ctx, key)
		if errors.Is(err, apperr.ErrConflict) { // lost the race to an identical upload
			return s.store.ByContent(ctx, p.CompanyID, purpose, hash)
		}
		return nil, err
	}
	return f, nil
}

func (s *Service) readable(ctx context.Context, companyID, id string) (*model.File, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	return s.store.Readable(ctx, companyID, id)
}

// Get returns a file visible to the caller: owned by its company or shared with it.
func (s *Service) Get(ctx context.Context, p *auth.Principal, id string) (*model.File, error) {
	return s.readable(ctx, p.CompanyID, id)
}

// Open returns the bytes; sensitive files need the sensitive-download
// permission with a fresh second factor.
func (s *Service) Open(ctx context.Context, p *auth.Principal, id string) (*model.File, io.ReadCloser, error) {
	f, err := s.readable(ctx, p.CompanyID, id)
	if err != nil {
		return nil, nil, err
	}
	if f.Sensitive {
		if err := p.Allow(model.PermSensitiveDownload); err != nil {
			return nil, nil, err
		}
	}
	r, err := s.storage.Open(ctx, f.StorageKey)
	return f, r, err
}

// Share grants companyID read access for a resource. The file must belong to
// ownerCompanyID. Used by other modules through their ports; joins the
// caller's transaction when ctx carries one.
func (s *Service) Share(ctx context.Context, ownerCompanyID, fileID, companyID, resourceType, resourceID string) (*model.File, error) {
	if validate.IDs(fileID) != nil {
		return nil, apperr.ErrNotFound
	}
	return s.store.Share(ctx, ownerCompanyID, fileID, &model.Share{
		FileID: fileID, CompanyID: companyID,
		ResourceType: resourceType, ResourceID: resourceID, CreatedAt: s.now().UTC(),
	})
}
