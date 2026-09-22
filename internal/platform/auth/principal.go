// Package auth holds the authenticated caller for a request and the permission
// middleware every module uses. The identity module authenticates sessions and
// stores the Principal; other modules only read it.
package auth

import (
	"slices"
	"strconv"

	"github.com/labstack/echo/v4"

	"justixauto/internal/platform/apperr"
)

// BranchScope is the session's working branch filter inside the active company.
type BranchScope struct {
	Mode      string   `json:"mode"` // "ALL" or "SELECTED"
	BranchIDs []string `json:"branchIds"`
}

// Principal is the signed-in user as seen by the current request.
type Principal struct {
	UserID      string
	SessionID   string
	Permissions map[string]bool
	// CompanyID is the active company ("" if none); membership was checked
	// when the principal was loaded.
	CompanyID       string
	ContextRevision int64
	BranchScope     BranchScope
}

func (p *Principal) Can(permission string) bool { return p != nil && p.Permissions[permission] }

// PermissionList returns the permissions sorted, for responses.
func (p *Principal) PermissionList() []string {
	out := make([]string, 0, len(p.Permissions))
	for k := range p.Permissions {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

const principalKey = "auth.principal"

func Set(c echo.Context, p *Principal) { c.Set(principalKey, p) }

// Get returns the principal or nil for anonymous requests.
func Get(c echo.Context) *Principal {
	p, _ := c.Get(principalKey).(*Principal)
	return p
}

// MustGet returns the principal or an unauthenticated error.
func MustGet(c echo.Context) (*Principal, error) {
	if p := Get(c); p != nil {
		return p, nil
	}
	return nil, apperr.ErrUnauthenticated
}

// Require rejects anonymous callers (401) and callers lacking any of the
// permissions (403).
func Require(permissions ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p, err := MustGet(c)
			if err != nil {
				return err
			}
			for _, perm := range permissions {
				if !p.Can(perm) {
					return apperr.New(apperr.ErrForbidden, "permission_denied", "missing permission "+perm)
				}
			}
			return next(c)
		}
	}
}

// RequireCompany rejects requests without an active company and requests whose
// X-Context-Revision (when sent) no longer matches the session context.
func RequireCompany() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p, err := MustGet(c)
			if err != nil {
				return err
			}
			if p.CompanyID == "" {
				return apperr.New(apperr.ErrConflict, "no_active_company", "select a company first")
			}
			if raw := c.Request().Header.Get("X-Context-Revision"); raw != "" && raw != strconv.FormatInt(p.ContextRevision, 10) {
				return apperr.New(apperr.ErrConflict, "context_changed", "the working company or branch scope changed, reload")
			}
			return next(c)
		}
	}
}
