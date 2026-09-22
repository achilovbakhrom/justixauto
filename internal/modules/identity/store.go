package identity

import (
	"context"

	"gorm.io/gorm"

	"justixauto/internal/platform/database"
)

// Store gives services access to all identity repositories and lets them run
// several repository calls in one database transaction.
type Store interface {
	Companies() CompanyRepository
	Users() UserRepository
	Roles() RoleRepository
	Branches() BranchRepository
	Memberships() MembershipRepository
	Sessions() SessionRepository
	Audit() AuditRepository
	MFA() MFARepository
	// InTx runs fn with a Store bound to one transaction; any error rolls back.
	InTx(ctx context.Context, fn func(Store) error) error
}

type gormStore struct{ db *gorm.DB }

func NewStore(db *gorm.DB) Store { return &gormStore{db: db} }

func (s *gormStore) Companies() CompanyRepository      { return &companyRepository{s.db} }
func (s *gormStore) Users() UserRepository             { return &userRepository{s.db} }
func (s *gormStore) Roles() RoleRepository             { return &roleRepository{s.db} }
func (s *gormStore) Branches() BranchRepository        { return &branchRepository{s.db} }
func (s *gormStore) Memberships() MembershipRepository { return &membershipRepository{s.db} }
func (s *gormStore) Sessions() SessionRepository       { return &sessionRepository{s.db} }
func (s *gormStore) Audit() AuditRepository            { return &auditRepository{s.db} }
func (s *gormStore) MFA() MFARepository                { return &mfaRepository{s.db} }

func (s *gormStore) InTx(ctx context.Context, fn func(Store) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&gormStore{db: tx})
	})
}

// Shared persistence helpers (see internal/platform/database).
var (
	translate       = database.Translate
	updateVersioned = database.UpdateVersioned
	pageDefaults    = database.Page
)
