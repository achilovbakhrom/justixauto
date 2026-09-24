// Package handler holds documents' Echo handlers, request/response DTOs and
// their mappers. It imports service, model and internal/pkg; it must never
// import repository or gorm.
package handler

import (
	"time"

	"justixauto/internal/modules/documents/model"
)

// FileView is a file's public metadata (never the storage key).
type FileView struct {
	ID         string    `json:"id"`
	FileName   string    `json:"fileName"`
	MIME       string    `json:"mime"`
	ByteLength int64     `json:"byteLength"`
	SHA256     string    `json:"sha256"`
	Purpose    string    `json:"purpose"`
	Sensitive  bool      `json:"sensitive"`
	ScanState  string    `json:"scanState"`
	CreatedAt  time.Time `json:"createdAt"`
}

// toFileView returns a file's public metadata.
func toFileView(f *model.File) FileView {
	return FileView{
		ID: f.ID, FileName: f.FileName, MIME: f.MIME, ByteLength: f.ByteLength, SHA256: f.SHA256,
		Purpose: f.Purpose, Sensitive: f.Sensitive, ScanState: "unscanned", CreatedAt: f.CreatedAt,
	}
}
