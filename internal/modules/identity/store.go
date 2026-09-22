package identity

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"justixauto/internal/platform/apperr"
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

// translate maps GORM errors to apperr kinds. Services add specific codes.
func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return apperr.ErrNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return apperr.New(apperr.ErrConflict, "duplicate", "a record with the same unique values already exists")
	case errors.Is(err, gorm.ErrForeignKeyViolated):
		return apperr.New(apperr.ErrConflict, "reference_missing", "a referenced record does not exist")
	}
	return err
}

// updateVersioned applies fields only if the row still has the expected
// version, and bumps the version. It reports ErrNotFound or ErrStale otherwise.
func updateVersioned(db *gorm.DB, model any, id string, expected int64, fields map[string]any) error {
	fields["version"] = expected + 1
	res := db.Model(model).Where("id = ? AND version = ?", id, expected).Updates(fields)
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 1 {
		return nil
	}
	var n int64
	if err := db.Model(model).Where("id = ?", id).Count(&n).Error; err != nil {
		return translate(err)
	}
	if n == 0 {
		return apperr.ErrNotFound
	}
	return apperr.ErrStale
}

func pageDefaults(limit, offset int) (int, int) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
