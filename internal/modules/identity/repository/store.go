// Package repository holds identity's GORM persistence. It implements the
// interfaces declared in service/ports.go structurally: it must never
// import the service package.
package repository

import (
	"context"

	"gorm.io/gorm"

	"justixauto/internal/pkg/database"
)

// Store gives services access to all identity repositories and lets them run
// several repository calls in one database transaction.
type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) Companies() *CompanyRepository      { return &CompanyRepository{s.db} }
func (s *Store) Users() *UserRepository             { return &UserRepository{s.db} }
func (s *Store) Roles() *RoleRepository             { return &RoleRepository{s.db} }
func (s *Store) Branches() *BranchRepository        { return &BranchRepository{s.db} }
func (s *Store) Memberships() *MembershipRepository { return &MembershipRepository{s.db} }
func (s *Store) Sessions() *SessionRepository       { return &SessionRepository{s.db} }
func (s *Store) Audit() *AuditRepository            { return &AuditRepository{s.db} }
func (s *Store) MFA() *MFARepository                { return &MFARepository{s.db} }

// InTx runs fn with a Store bound to one transaction; any error rolls back.
func (s *Store) InTx(ctx context.Context, fn func(*Store) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Store{db: tx})
	})
}

// Shared persistence helpers (see internal/pkg/database).
var (
	translate       = database.Translate
	updateVersioned = database.UpdateVersioned
	pageDefaults    = database.Page
)
