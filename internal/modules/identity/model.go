// Package identity owns companies, branches, users, roles, memberships,
// sessions and the identity audit log. Other modules reference them by ID and
// read the signed-in caller through internal/platform/auth.
package identity

import "time"

// CompanyKind: a seller using the Realization app, or a provider (bank, MFO,
// insurance company) connected through Admin → Integrations.
type CompanyKind string

const (
	KindSeller    CompanyKind = "seller"
	KindBank      CompanyKind = "bank"
	KindMFO       CompanyKind = "mfo"
	KindInsurance CompanyKind = "insurance"
)

func (k CompanyKind) Valid() bool {
	switch k {
	case KindSeller, KindBank, KindMFO, KindInsurance:
		return true
	}
	return false
}

func (k CompanyKind) Provider() bool { return k == KindBank || k == KindMFO || k == KindInsurance }

// CompanyAccess is platform access, not legal/compliance verification.
type CompanyAccess string

const (
	AccessDraft     CompanyAccess = "draft"
	AccessActive    CompanyAccess = "active"
	AccessSuspended CompanyAccess = "suspended"
)

// Company is a registered organization. Requisites can change; the ID never does.
type Company struct {
	ID                 string `gorm:"primaryKey;type:uuid"`
	Kind               CompanyKind
	Name               string
	LegalName          string
	Country            string // label as entered
	CountryKey         string // catalogue key when chosen from the list
	Region             string
	RegionKey          string
	RegistrationNumber string
	Email              string
	Address            string
	Phone              string
	Status             CompanyAccess
	StatusReason       string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (Company) TableName() string { return "identity.companies" }

type UserStatus string

const (
	UserPending   UserStatus = "pending" // no credential yet, cannot sign in
	UserActive    UserStatus = "active"
	UserSuspended UserStatus = "suspended"
)

type User struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	DisplayName  string
	Email        string
	Login        *string
	PasswordHash *string
	// PasswordChangeRequired: an administrator set the password; the user must
	// choose their own before using anything else.
	PasswordChangeRequired bool
	Status                 UserStatus
	StatusReason           string
	FailedLogins           int
	LockedUntil            *time.Time
	// MFA fields change only through MFARepository, never through Update.
	MFASecretEnc   []byte     `gorm:"column:mfa_secret_enc"`
	MFAEnabledAt   *time.Time `gorm:"column:mfa_enabled_at"`
	MFALastCounter int64      `gorm:"column:mfa_last_counter"`
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (User) TableName() string { return "identity.users" }

// Role is global: it applies equally in every company the user is a member of.
type Role struct {
	ID          string  `gorm:"primaryKey;type:uuid"`
	SystemKey   *string // set for built-in roles; their permissions come from code
	Name        string
	Permissions []string `gorm:"-"`
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Role) TableName() string { return "identity.roles" }

func (r Role) System() bool { return r.SystemKey != nil }

type rolePermission struct {
	RoleID     string `gorm:"primaryKey;type:uuid"`
	Permission string `gorm:"primaryKey"`
}

func (rolePermission) TableName() string { return "identity.role_permissions" }

type userRole struct {
	UserID string `gorm:"primaryKey;type:uuid"`
	RoleID string `gorm:"primaryKey;type:uuid"`
}

func (userRole) TableName() string { return "identity.user_roles" }

type Branch struct {
	ID        string `gorm:"primaryKey;type:uuid"`
	CompanyID string `gorm:"type:uuid"`
	Name      string
	Address   string
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Branch) TableName() string { return "identity.branches" }

type MembershipStatus string

const (
	MembershipActive  MembershipStatus = "active"
	MembershipRevoked MembershipStatus = "revoked"
)

const (
	AllBranches      = "ALL_BRANCHES"
	SelectedBranches = "SELECTED_BRANCHES"
)

// Membership links a user to a company. It carries branch access, never a role.
type Membership struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	UserID       string `gorm:"type:uuid"`
	CompanyID    string `gorm:"type:uuid"`
	Status       MembershipStatus
	BranchAccess string
	BranchIDs    []string `gorm:"-"` // only for SELECTED_BRANCHES
	StatusReason string
	Version      int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (Membership) TableName() string { return "identity.memberships" }

type membershipBranch struct {
	MembershipID string `gorm:"primaryKey;type:uuid"`
	BranchID     string `gorm:"primaryKey;type:uuid"`
}

func (membershipBranch) TableName() string { return "identity.membership_branches" }

const (
	ScopeAll      = "ALL"
	ScopeSelected = "SELECTED"
)

// Session is a signed-in browser. Only the SHA-256 of the cookie token is stored.
type Session struct {
	ID              string `gorm:"primaryKey;type:uuid"`
	TokenHash       []byte
	CSRFToken       string  `gorm:"column:csrf_token"`
	UserID          string  `gorm:"type:uuid"`
	ActiveCompanyID *string `gorm:"type:uuid"`
	BranchScopeMode string
	BranchIDs       []string `gorm:"-"` // only for SELECTED scope
	ContextRevision int64
	// MFAAuthenticatedAt is the last successful second factor in this session.
	MFAAuthenticatedAt *time.Time `gorm:"column:mfa_authenticated_at"`
	CreatedAt          time.Time
	LastSeenAt         time.Time
	ExpiresAt          time.Time
	RevokedAt          *time.Time
}

func (Session) TableName() string { return "identity.sessions" }

type sessionBranch struct {
	SessionID string `gorm:"primaryKey;type:uuid"`
	BranchID  string `gorm:"primaryKey;type:uuid"`
}

func (sessionBranch) TableName() string { return "identity.session_branches" }

// AuditEvent is an append-only record of a sensitive change.
type AuditEvent struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	Seq          int64  `gorm:"->"` // assigned by the database; defines order
	OccurredAt   time.Time
	ActorUserID  *string `gorm:"type:uuid"`
	Action       string
	ResourceType string
	ResourceID   string  `gorm:"type:uuid"`
	CompanyID    *string `gorm:"type:uuid"`
	Reason       string
	Details      []byte `gorm:"type:jsonb"`
}

func (AuditEvent) TableName() string { return "identity.audit_events" }

type mfaEnrollment struct {
	ID          string `gorm:"primaryKey;type:uuid"`
	UserID      string `gorm:"type:uuid"`
	SecretEnc   []byte
	CreatedAt   time.Time
	ExpiresAt   time.Time
	ConfirmedAt *time.Time
}

func (mfaEnrollment) TableName() string { return "identity.mfa_enrollments" }

type mfaRecoveryCode struct {
	UserID   string `gorm:"primaryKey;type:uuid"`
	CodeHash []byte `gorm:"primaryKey"`
	UsedAt   *time.Time
}

func (mfaRecoveryCode) TableName() string { return "identity.mfa_recovery_codes" }

// mfaChallenge: the password was correct, the second factor is pending.
type mfaChallenge struct {
	ID         string `gorm:"primaryKey;type:uuid"`
	UserID     string `gorm:"type:uuid"`
	TokenHash  []byte
	Attempts   int
	CreatedAt  time.Time
	ExpiresAt  time.Time
	ConsumedAt *time.Time
}

func (mfaChallenge) TableName() string { return "identity.mfa_challenges" }
