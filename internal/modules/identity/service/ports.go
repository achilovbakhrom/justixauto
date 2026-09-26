package service

import (
	"context"
	"time"

	"justixauto/internal/modules/identity/model"
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
	// InTx runs fn with a Store bound to one transaction; any error rolls back.
	InTx(ctx context.Context, fn func(Store) error) error
}

type CompanyRepository interface {
	Create(ctx context.Context, c *model.Company) error
	// Get, GetMany and List see live companies only; a soft-deleted company
	// is ErrNotFound / absent.
	Get(ctx context.Context, id string) (*model.Company, error)
	// GetIncludingDeleted also returns a soft-deleted company (history only).
	GetIncludingDeleted(ctx context.Context, id string) (*model.Company, error)
	GetMany(ctx context.Context, ids []string) ([]model.Company, error)
	List(ctx context.Context, f model.CompanyFilter) ([]model.Company, error)
	// Update saves c if the stored version equals expected and sets c.Version.
	Update(ctx context.Context, c *model.Company, expected int64) error
	// SoftDelete marks c deleted if the stored version equals expected.
	SoftDelete(ctx context.Context, c *model.Company, expected int64) error
}

type UserRepository interface {
	Create(ctx context.Context, u *model.User) error
	Get(ctx context.Context, id string) (*model.User, error)
	FindByLogin(ctx context.Context, login string) (*model.User, error)
	// EmailOrLoginTaken reports whether another user already uses the email or login.
	EmailOrLoginTaken(ctx context.Context, email string, login *string) (bool, error)
	// LoginTaken reports whether another user already signs in with login.
	LoginTaken(ctx context.Context, login, exceptUserID string) (bool, error)
	// EmailTakenByOther reports whether another user already uses the email.
	EmailTakenByOther(ctx context.Context, email, exceptUserID string) (bool, error)
	// ListStaff lists users who are not employees of any live company.
	ListStaff(ctx context.Context, limit, offset int) ([]model.User, error)
	Update(ctx context.Context, u *model.User, expected int64) error
	// SetLoginState records failed attempts and lockouts without bumping the
	// version, so sign-in attempts never make an admin's edit stale.
	SetLoginState(ctx context.Context, id string, failed int, lockedUntil *time.Time) error
	// RecordFailure counts one failed attempt atomically (safe across
	// replicas); at the threshold it locks the account and resets the count.
	RecordFailure(ctx context.Context, id string, threshold int, lockUntil time.Time) error
}

type RoleRepository interface {
	List(ctx context.Context) ([]model.Role, error)
	Get(ctx context.Context, id string) (*model.Role, error)
	GetMany(ctx context.Context, ids []string) ([]model.Role, error)
	Create(ctx context.Context, r *model.Role) error
	Update(ctx context.Context, r *model.Role, expected int64) error
	UserRoles(ctx context.Context, userID string) ([]model.Role, error)
	SetUserRoles(ctx context.Context, userID string, roleIDs []string) error
	// LockAdminGuard serializes every change that could remove the last
	// platform admin (row lock on the platform_admin role, inside a transaction).
	LockAdminGuard(ctx context.Context) error
	// CountActivePlatformAdmins counts active users holding platform_admin,
	// ignoring excludeUserID.
	CountActivePlatformAdmins(ctx context.Context, excludeUserID string) (int64, error)
}

type BranchRepository interface {
	Create(ctx context.Context, b *model.Branch) error
	Get(ctx context.Context, companyID, id string) (*model.Branch, error)
	List(ctx context.Context, companyID string) ([]model.Branch, error)
	Update(ctx context.Context, b *model.Branch, expected int64) error
	// CountInCompany counts how many of ids are branches of the company.
	CountInCompany(ctx context.Context, companyID string, ids []string) (int64, error)
}

type MembershipRepository interface {
	Create(ctx context.Context, m *model.Membership) error
	Get(ctx context.Context, id string) (*model.Membership, error)
	ListByUser(ctx context.Context, userID string) ([]model.Membership, error)
	// ActiveInCompany lists the company's active memberships.
	ActiveInCompany(ctx context.Context, companyID string) ([]model.Membership, error)
	// Active returns the user's active membership in the company, or ErrNotFound.
	Active(ctx context.Context, userID, companyID string) (*model.Membership, error)
	Update(ctx context.Context, m *model.Membership, expected int64) error
}

type SessionRepository interface {
	Create(ctx context.Context, s *model.Session) error
	// FindLive returns the unrevoked session with this token hash, or ErrNotFound.
	FindLive(ctx context.Context, tokenHash []byte) (*model.Session, error)
	Touch(ctx context.Context, id string, at time.Time) error
	Revoke(ctx context.Context, id string, at time.Time) error
	// RevokeUser revokes all live sessions of a user except exceptID ("" = all).
	RevokeUser(ctx context.Context, userID, exceptID string, at time.Time) error
	// UpdateContext saves company/scope if the context revision still equals
	// expected, then increments it.
	UpdateContext(ctx context.Context, s *model.Session, expected int64) error
	// ResetCompanyContext clears the active company on the user's live sessions
	// that work in companyID (after membership revocation).
	ResetCompanyContext(ctx context.Context, userID, companyID string) error
}

type AuditRepository interface {
	Append(ctx context.Context, e *model.AuditEvent) error
	List(ctx context.Context, f model.AuditFilter) ([]model.AuditEvent, error)
}
