// Package model holds documents' GORM models and value types. It imports
// internal/pkg only; it must never import service, repository or handler.
package model

import "time"

// File is a stored upload's metadata. Stored bytes never change.
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

// Share grants a company read access to a file for a specific resource.
type Share struct {
	FileID       string `gorm:"primaryKey;type:uuid"`
	CompanyID    string `gorm:"primaryKey;type:uuid"`
	ResourceType string
	ResourceID   string `gorm:"primaryKey;type:uuid"`
	CreatedAt    time.Time
}

func (Share) TableName() string { return "documents.shares" }
