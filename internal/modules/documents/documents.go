// Package documents stores uploaded files privately and controls who may read
// them. A file belongs to the uploading company; other modules share it with a
// counterparty for a specific resource. Stored bytes never change. There is no
// malware scanning yet: a stored file is not "clean" and not domain-accepted.
package documents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/database"
	"justixauto/internal/platform/validate"
)

const (
	PermRead              = "documents.read"
	PermUpload            = "documents.upload"
	PermSensitiveDownload = "documents.sensitive.download" // needs fresh MFA
)

var Permissions = []auth.PermissionInfo{
	{Key: PermRead, Scope: "company", Assignable: true},
	{Key: PermUpload, Scope: "company", Assignable: true},
	{Key: PermSensitiveDownload, Scope: "company", RequiresMFA: true, Assignable: true},
}

// MaxBytes limits one upload (10 MiB).
const MaxBytes = 10 << 20

var allowedMIME = []string{"application/pdf", "image/jpeg", "image/png"}

// Purposes decide sensitivity: finance and identity documents are sensitive.
var purposes = map[string]bool{
	"finance-document": true, "payment-evidence": false, "vehicle-photo": false, "deal-document": true, "other": false,
}

type File struct {
	ID         string `gorm:"primaryKey;type:uuid"`
	CompanyID  string `gorm:"type:uuid"`
	Purpose    string
	FileName   string
	MIME       string `gorm:"column:mime"`
	ByteLength int64
	SHA256     string `gorm:"column:sha256"`
	StorageKey string
	Sensitive  bool
	CreatedBy  string `gorm:"type:uuid"`
	CreatedAt  time.Time
}

func (File) TableName() string { return "documents.files" }

type Share struct {
	FileID       string `gorm:"primaryKey;type:uuid"`
	CompanyID    string `gorm:"primaryKey;type:uuid"`
	ResourceType string
	ResourceID   string `gorm:"primaryKey;type:uuid"`
	CreatedAt    time.Time
}

func (Share) TableName() string { return "documents.shares" }

// Storage keeps file bytes: S3 in production, a private directory locally.
// Keys are opaque and never leave the module.
type Storage interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// DirStorage stores files in a private local directory (development, tests).
type DirStorage struct{ Root string }

func (d DirStorage) path(key string) string { return filepath.Join(d.Root, key[:2], key) }

func (d DirStorage) Put(_ context.Context, key string, data []byte, _ string) error {
	p := d.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func (d DirStorage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	return os.Open(d.path(key))
}
func (d DirStorage) Delete(_ context.Context, key string) error { return os.Remove(d.path(key)) }

// repository holds all database access of the module.
type repository struct{ db *gorm.DB }

func (r *repository) create(ctx context.Context, f *File) error {
	return database.Translate(r.db.WithContext(ctx).Create(f).Error)
}

// readable returns the file if the company owns it or it was shared with it.
func (r *repository) readable(ctx context.Context, companyID, id string) (*File, error) {
	var f File
	err := r.db.WithContext(ctx).Where(`id = ? AND (company_id = ? OR EXISTS (SELECT 1 FROM documents.shares s
		WHERE s.file_id = files.id AND s.company_id = ?))`, id, companyID, companyID).Take(&f).Error
	if err != nil {
		return nil, database.Translate(err)
	}
	return &f, nil
}

// share joins the caller's transaction when ctx carries one.
func (r *repository) share(ctx context.Context, ownerCompanyID, fileID string, sh *Share) (*File, error) {
	db := database.Conn(ctx, r.db).WithContext(ctx)
	var f File
	if err := db.Where("id = ? AND company_id = ?", fileID, ownerCompanyID).Take(&f).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &f, database.Translate(db.Clauses(clause.OnConflict{DoNothing: true}).Create(sh).Error)
}

type Service struct {
	repo    *repository
	storage Storage
	now     func() time.Time
}

// Upload stores a file for the active company. The content type is detected
// from the bytes (the client's claim is ignored) and must be PDF, JPEG or PNG.
func (s *Service) Upload(ctx context.Context, p *auth.Principal, purpose, name string, r io.Reader) (*File, error) {
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
	sum := sha256.Sum256(data)
	key := strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := s.storage.Put(ctx, key, data, mime); err != nil {
		return nil, err
	}
	f := &File{ID: uuid.NewString(), CompanyID: p.CompanyID, Purpose: purpose, FileName: name, MIME: mime, ByteLength: int64(len(data)),
		SHA256: hex.EncodeToString(sum[:]), StorageKey: key, Sensitive: sensitive, CreatedBy: p.UserID, CreatedAt: s.now().UTC()}
	if err := s.repo.create(ctx, f); err != nil {
		_ = s.storage.Delete(ctx, key)
		return nil, err
	}
	return f, nil
}

func (s *Service) readable(ctx context.Context, companyID, id string) (*File, error) {
	if err := validate.IDs(id); err != nil {
		return nil, err
	}
	return s.repo.readable(ctx, companyID, id)
}

func (s *Service) Get(ctx context.Context, p *auth.Principal, id string) (*File, error) {
	return s.readable(ctx, p.CompanyID, id)
}

// Open returns the bytes; sensitive files need the sensitive-download
// permission with a fresh second factor.
func (s *Service) Open(ctx context.Context, p *auth.Principal, id string) (*File, io.ReadCloser, error) {
	f, err := s.readable(ctx, p.CompanyID, id)
	if err != nil {
		return nil, nil, err
	}
	if f.Sensitive {
		if err := p.Allow(PermSensitiveDownload); err != nil {
			return nil, nil, err
		}
	}
	r, err := s.storage.Open(ctx, f.StorageKey)
	return f, r, err
}

// Share grants companyID read access for a resource. The file must belong to
// ownerCompanyID. Used by other modules through their ports; joins the
// caller's transaction when ctx carries one.
func (s *Service) Share(ctx context.Context, ownerCompanyID, fileID, companyID, resourceType, resourceID string) (*File, error) {
	if validate.IDs(fileID) != nil {
		return nil, apperr.ErrNotFound
	}
	return s.repo.share(ctx, ownerCompanyID, fileID, &Share{FileID: fileID, CompanyID: companyID,
		ResourceType: resourceType, ResourceID: resourceID, CreatedAt: s.now().UTC()})
}

// FileInfo is a file's public metadata (never the storage key).
func FileInfo(f *File) map[string]any {
	return map[string]any{"id": f.ID, "fileName": f.FileName, "mime": f.MIME, "byteLength": f.ByteLength, "sha256": f.SHA256,
		"purpose": f.Purpose, "sensitive": f.Sensitive, "scanState": "unscanned", "createdAt": f.CreatedAt}
}
