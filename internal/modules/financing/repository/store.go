// Package repository holds financing's GORM persistence. It implements the
// Repository interface declared in service/ports.go structurally: it must
// never import the service package.
package repository

import (
	"context"

	"gorm.io/gorm"

	"justixauto/internal/pkg/database"
)

// Store is financing's GORM persistence.
type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db} }

// InTx runs fn in a transaction; inside a caller's ambient transaction
// (database.WithTx) it becomes a savepoint of that transaction.
func (r *Store) InTx(ctx context.Context, fn func(*Store) error) error {
	return database.Conn(ctx, r.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(&Store{tx}) })
}

// Bind carries the store's connection in ctx, so a call into another
// module through a port joins it.
func (r *Store) Bind(ctx context.Context) context.Context { return database.WithTx(ctx, r.db) }

func (r *Store) create(ctx context.Context, v any) error {
	return database.Translate(r.db.WithContext(ctx).Create(v).Error)
}

// nextNumber computes the next 1-based number of a per-parent numbered
// sequence (program versions, terms versions, document submissions).
func (r *Store) nextNumber(ctx context.Context, model any, column, id string) (int, error) {
	var n int
	err := r.db.WithContext(ctx).Model(model).Where(column+" = ?", id).Select("coalesce(max(number), 0) + 1").Scan(&n).Error
	return n, database.Translate(err)
}
