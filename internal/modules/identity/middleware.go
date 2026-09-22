package identity

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/labstack/echo/v4"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
)

// CookieConfig controls the session cookie. Secure must be true outside local
// HTTP development; the __Host- prefix is only valid for Secure cookies.
type CookieConfig struct {
	Secure         bool
	AllowedOrigins []string // extra origins allowed for state-changing requests
}

func (c CookieConfig) name() string {
	if c.Secure {
		return "__Host-justix_session"
	}
	return "justix_session"
}

func (c CookieConfig) set(ctx echo.Context, token string, maxAge int) {
	ctx.SetCookie(&http.Cookie{Name: c.name(), Value: token, Path: "/", MaxAge: maxAge,
		Secure: c.Secure, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func (c CookieConfig) token(ctx echo.Context) string { return cookieValue(ctx, c.name()) }

// The MFA challenge cookie holds no authority on its own; it only binds a
// pending challenge to the browser that entered the password.
func (c CookieConfig) challengeName() string {
	if c.Secure {
		return "__Host-justix_mfa_challenge"
	}
	return "justix_mfa_challenge"
}

func (c CookieConfig) setChallenge(ctx echo.Context, token string, maxAge int) {
	ctx.SetCookie(&http.Cookie{Name: c.challengeName(), Value: token, Path: "/", MaxAge: maxAge,
		Secure: c.Secure, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func (c CookieConfig) challengeToken(ctx echo.Context) string {
	return cookieValue(ctx, c.challengeName())
}

func cookieValue(ctx echo.Context, name string) string {
	cookie, err := ctx.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

const sessionKey = "identity.session"

// Authenticator loads the session from the cookie for every request. Anonymous
// requests pass through without a principal; routes decide with auth.Require.
// State-changing requests must come from an allowed Origin and, when signed
// in, carry the session's X-CSRF-Token.
type Authenticator struct {
	auth   *AuthService
	cookie CookieConfig
}

func unsafeMethod(m string) bool {
	return m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions
}

func (a *Authenticator) originAllowed(c echo.Context) bool {
	origin := c.Request().Header.Get("Origin")
	if origin == "" {
		return true // non-browser clients; the CSRF token still applies
	}
	if slices.Contains(a.cookie.AllowedOrigins, origin) {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == c.Request().Host
}

// Middleware authenticates requests. csrfExempt lists route suffixes that run
// before a session exists (login and MFA verification).
func (a *Authenticator) Middleware(csrfExempt ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			unsafe := unsafeMethod(c.Request().Method)
			if unsafe && !a.originAllowed(c) {
				return apperr.New(apperr.ErrForbidden, "origin_denied", "request origin is not allowed")
			}
			p, sess, err := a.auth.Authenticate(c.Request().Context(), a.cookie.token(c))
			switch {
			case errors.Is(err, apperr.ErrUnauthenticated):
				return next(c)
			case err != nil:
				return err
			}
			exempt := slices.ContainsFunc(csrfExempt, func(suffix string) bool { return strings.HasSuffix(c.Path(), suffix) })
			if unsafe && !exempt {
				sent := c.Request().Header.Get("X-CSRF-Token")
				if subtle.ConstantTimeCompare([]byte(sent), []byte(sess.CSRFToken)) != 1 {
					return apperr.New(apperr.ErrForbidden, "csrf_invalid", "missing or invalid X-CSRF-Token")
				}
			}
			if p.PasswordChangeRequired && !passwordChangeAllowed(c) {
				return apperr.New(apperr.ErrForbidden, "password_change_required", "choose a new password first")
			}
			auth.Set(c, p)
			c.Set(sessionKey, sess)
			return next(c)
		}
	}
}

// passwordChangeAllowed: with an administrator-set password, the session may
// only read itself, change the password or sign out.
func passwordChangeAllowed(c echo.Context) bool {
	path, method := c.Path(), c.Request().Method
	return (method == http.MethodGet && strings.HasSuffix(path, "/identity/session")) ||
		strings.HasSuffix(path, "/identity/session/password") || strings.HasSuffix(path, "/identity/session/logout")
}
