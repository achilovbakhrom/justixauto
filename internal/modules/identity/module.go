// Package identity owns companies, branches, users, roles, memberships,
// sessions and the identity audit log. Other modules reference them by ID and
// read the signed-in caller through internal/pkg/auth.
//
// This package is the only one other code imports; internal/modules/identity
// splits into model (GORM models, enums, the permission catalog), repository
// (GORM persistence), service (business rules, sessions) and handler
// (Echo routes, the authentication middleware) layers, wired together here.
package identity

import (
	"context"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/modules/identity/handler"
	"justixauto/internal/modules/identity/model"
	"justixauto/internal/modules/identity/repository"
	"justixauto/internal/modules/identity/service"
	"justixauto/internal/pkg/auth"
)

// CookieConfig controls the session cookie.
type CookieConfig = handler.CookieConfig

// SessionConfig controls session lifetimes and login lockout.
type SessionConfig = service.SessionConfig

// DefaultSessionConfig is the recommended SessionConfig.
var DefaultSessionConfig = service.DefaultSessionConfig

// CompanyService manages company registration, requisites and platform
// access. Other modules read company facts through it (see internal/app's
// Directory adapters).
type CompanyService = service.Company

// BootstrapInput creates the very first platform administrator.
type BootstrapInput = service.BootstrapInput

const (
	// CompanyAdminRoleID is the built-in role granted to a company's first admin.
	CompanyAdminRoleID = model.CompanyAdminRoleID
	// AccessActive is the CompanyAccess value for a company with platform access.
	AccessActive = model.AccessActive
)

// RegisterPermissions adds another module's permission keys to the identity
// catalog. Call it at startup, before serving requests.
func RegisterPermissions(perms ...auth.PermissionInfo) { model.RegisterPermissions(perms...) }

type Config struct {
	Cookie  CookieConfig
	Session SessionConfig
	Now     func() time.Time // nil = time.Now
}

// storeAdapter bridges repository.Store (which cannot import service, since
// repository implements service's interfaces structurally) to service.Store,
// whose InTx is self-referential and so needs the exact service.Store type.
type storeAdapter struct{ r *repository.Store }

func (a storeAdapter) Companies() service.CompanyRepository      { return a.r.Companies() }
func (a storeAdapter) Users() service.UserRepository             { return a.r.Users() }
func (a storeAdapter) Roles() service.RoleRepository             { return a.r.Roles() }
func (a storeAdapter) Branches() service.BranchRepository        { return a.r.Branches() }
func (a storeAdapter) Memberships() service.MembershipRepository { return a.r.Memberships() }
func (a storeAdapter) Sessions() service.SessionRepository       { return a.r.Sessions() }
func (a storeAdapter) Audit() service.AuditRepository            { return a.r.Audit() }

func (a storeAdapter) InTx(ctx context.Context, fn func(service.Store) error) error {
	return a.r.InTx(ctx, func(rs *repository.Store) error { return fn(storeAdapter{rs}) })
}

// Module wires the identity repository → service → handler chain.
type Module struct {
	Auth          *service.Auth
	Companies     *CompanyService
	Users         *service.User
	authenticator *handler.Authenticator
	session       *handler.SessionHandler
	company       *handler.CompanyHandler
	admin         *handler.AdminHandler
}

func New(db *gorm.DB, cfg Config) (*Module, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	d := service.NewDeps(storeAdapter{repository.NewStore(db)}, cfg.Now)
	m := &Module{
		Auth:      service.NewAuth(d, cfg.Session),
		Companies: service.NewCompany(d),
		Users:     service.NewUser(d),
	}
	branches := service.NewBranch(d)
	m.authenticator = handler.NewAuthenticator(m.Auth, cfg.Cookie)
	m.session = handler.NewSessionHandler(m.Auth, cfg.Cookie)
	m.company = handler.NewCompanyHandler(m.Companies, branches)
	m.admin = handler.NewAdminHandler(m.Companies, m.Users, service.NewRole(d), service.NewMembership(d), service.NewAudit(d))
	return m, nil
}

// Authenticate must wrap the whole /api/v1 group so every module sees the
// signed-in principal (auth.Get) and CSRF/Origin checks apply everywhere.
func (m *Module) Authenticate() echo.MiddlewareFunc {
	return m.authenticator.Middleware("/identity/session/login")
}

// Register mounts the identity routes under /api/v1/identity.
func (m *Module) Register(api *echo.Group) {
	g := api.Group("/identity")
	m.session.Routes(g)
	m.company.Routes(g)
	m.admin.Routes(g)
}
