// Package repository holds documents' persistence: GORM access to files and
// shares, plus the byte storage backends (a private local directory and S3).
// It implements the interfaces declared in service/ports.go structurally: it
// must never import the service package.
package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"justixauto/internal/modules/documents/model"
	"justixauto/internal/pkg/database"
)

// Store holds all database access of the module.
type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db} }

func (r *Store) Create(ctx context.Context, f *model.File) error {
	return database.Translate(r.db.WithContext(ctx).Create(f).Error)
}

// Readable returns the file if the company owns it or it was shared with it.
func (r *Store) Readable(ctx context.Context, companyID, id string) (*model.File, error) {
	var f model.File
	err := r.db.WithContext(ctx).Where(`id = ? AND (company_id = ? OR EXISTS (SELECT 1 FROM documents.shares s
		WHERE s.file_id = files.id AND s.company_id = ?))`, id, companyID, companyID).Take(&f).Error
	if err != nil {
		return nil, database.Translate(err)
	}
	return &f, nil
}

// Share joins the caller's transaction when ctx carries one.
func (r *Store) Share(ctx context.Context, ownerCompanyID, fileID string, sh *model.Share) (*model.File, error) {
	db := database.Conn(ctx, r.db).WithContext(ctx)
	var f model.File
	if err := db.Where("id = ? AND company_id = ?", fileID, ownerCompanyID).Take(&f).Error; err != nil {
		return nil, database.Translate(err)
	}
	return &f, database.Translate(db.Clauses(clause.OnConflict{DoNothing: true}).Create(sh).Error)
}
