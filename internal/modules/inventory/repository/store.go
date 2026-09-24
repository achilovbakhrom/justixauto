// Package repository holds inventory's GORM persistence. It implements the
// interfaces declared in service/ports.go structurally: it must never import
// the service package.
package repository

import (
	"context"

	"gorm.io/gorm"

	"justixauto/internal/pkg/database"
)

var translate = database.Translate

// conn resolves the connection to use for a call: the transaction carried by
// ctx (set by a caller in another module through database.WithTx) if there
// is one, otherwise db.
func conn(ctx context.Context, db *gorm.DB) *gorm.DB {
	return database.Conn(ctx, db).WithContext(ctx)
}

// Store gives services the inventory repositories and one-transaction runs.
type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) Models() *ModelRepository             { return &ModelRepository{s.db} }
func (s *Store) Warehouses() *WarehouseRepository     { return &WarehouseRepository{s.db} }
func (s *Store) Vehicles() *VehicleRepository         { return &VehicleRepository{s.db} }
func (s *Store) Facts() *FactRepository               { return &FactRepository{s.db} }
func (s *Store) Reservations() *ReservationRepository { return &ReservationRepository{s.db} }

// InTx runs fn in a transaction; inside a caller's ambient transaction
// (database.WithTx) it becomes a savepoint of that transaction.
func (s *Store) InTx(ctx context.Context, fn func(*Store) error) error {
	return conn(ctx, s.db).Transaction(func(tx *gorm.DB) error { return fn(&Store{db: tx}) })
}
