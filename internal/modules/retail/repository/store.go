// Package repository holds retail's GORM persistence. It implements the
// interfaces declared in service/ports.go structurally: it must never
// import the service package.
package repository

import (
	"context"

	"gorm.io/gorm"

	"justixauto/internal/pkg/database"
)

// Store gives services the retail repositories and one-transaction runs.
type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) CRM() *CRMRepository          { return &CRMRepository{s.db} }
func (s *Store) Listings() *ListingRepository { return &ListingRepository{s.db} }
func (s *Store) Deals() *DealRepository       { return &DealRepository{s.db} }
func (s *Store) Events() *EventRepository     { return &EventRepository{s.db} }

// Bind carries the store's connection in ctx, so a call into another
// module through a port joins it.
func (s *Store) Bind(ctx context.Context) context.Context { return database.WithTx(ctx, s.db) }

// InTx runs fn in a transaction; inside a caller's ambient transaction
// (database.WithTx) it becomes a savepoint of that transaction.
func (s *Store) InTx(ctx context.Context, fn func(*Store) error) error {
	return database.Conn(ctx, s.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(&Store{db: tx}) })
}
