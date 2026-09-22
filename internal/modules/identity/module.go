package identity

import (
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type Config struct {
	Cookie  CookieConfig
	Session SessionConfig
	Now     func() time.Time // nil = time.Now
}

// Module wires the identity repository → service → handler chain.
type Module struct {
	Auth          *AuthService
	Companies     *CompanyService
	Users         *UserService
	authenticator *Authenticator
	session       *SessionHandler
	company       *CompanyHandler
	admin         *AdminHandler
}

func New(db *gorm.DB, cfg Config) *Module {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	d := deps{store: NewStore(db), now: cfg.Now}
	m := &Module{
		Auth:      &AuthService{deps: d, cfg: cfg.Session},
		Companies: &CompanyService{deps: d},
		Users:     &UserService{deps: d},
	}
	branches := &BranchService{deps: d}
	m.authenticator = &Authenticator{auth: m.Auth, cookie: cfg.Cookie}
	m.session = &SessionHandler{auth: m.Auth, cookie: cfg.Cookie}
	m.company = &CompanyHandler{companies: m.Companies, branches: branches}
	m.admin = &AdminHandler{companies: m.Companies, users: m.Users, roles: &RoleService{deps: d},
		memberships: &MembershipService{deps: d}, audit: &AuditService{deps: d}}
	return m
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
