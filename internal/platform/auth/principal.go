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

// PermissionInfo is one entry of the permission catalog. Modules declare their
// own keys (`<module>.<resource>.<action>`); the identity module owns the
// catalog. Assignable=false keys can only be held through a system role.
// RequiresMFA marks sensitive actions that need a recent second factor.
type PermissionInfo struct {
	Key         string `json:"key"`
	Scope       string `json:"scope"` // "platform" or "company"
	RequiresMFA bool   `json:"requiresMfa"`
	Assignable  bool   `json:"assignable"`
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
	// MFA: permissions in MFARequired are usable only with a recent second factor.
	MFAEnrolled bool
	MFAFresh    bool
	MFARequired map[string]bool
	// PasswordChangeRequired limits the session to changing the password.
	PasswordChangeRequired bool
}

// Has reports whether the user's roles grant the permission.
func (p *Principal) Has(permission string) bool { return p != nil && p.Permissions[permission] }

// Can reports whether the permission is usable now: granted and, for
// sensitive permissions, backed by a recent second factor.
func (p *Principal) Can(permission string) bool {
	return p.Has(permission) && (!p.MFARequired[permission] || p.MFAFresh)
}

// Allow returns nil if the permission is usable now, otherwise the reason.
func (p *Principal) Allow(permission string) error {
	switch {
	case !p.Has(permission):
		return apperr.New(apperr.ErrForbidden, "permission_denied", "missing permission "+permission)
	case p.Can(permission):
		return nil
	case !p.MFAEnrolled:
		return apperr.New(apperr.ErrForbidden, "mfa_enrollment_required", "set up two-factor authentication to use "+permission)
	default:
		return apperr.New(apperr.ErrForbidden, "mfa_required", "confirm with your two-factor code to use "+permission)
	}
}

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

// Require rejects anonymous callers (401) and callers that cannot use all of
// the permissions now (403: permission_denied, mfa_enrollment_required, mfa_required).
func Require(permissions ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p, err := MustGet(c)
			if err != nil {
				return err
			}
			for _, perm := range permissions {
				if err := p.Allow(perm); err != nil {
					return err
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
