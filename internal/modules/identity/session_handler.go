package identity

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/httpx"
)

type SessionHandler struct {
	auth   *AuthService
	cookie CookieConfig
}

func (h *SessionHandler) Routes(g *echo.Group) {
	g.GET("/session", h.get, auth.Require())
	g.POST("/session/login", h.login)
	g.POST("/session/logout", h.logout, auth.Require())
	g.PUT("/session/context", h.setContext, auth.Require())
	g.PUT("/session/branch-scope", h.setBranchScope, auth.Require())
}

func (h *SessionHandler) view(c echo.Context, p *auth.Principal, csrf string, status int) error {
	view, err := h.auth.View(c.Request().Context(), p)
	if err != nil {
		return err
	}
	// The browser keeps the CSRF token in memory; it is re-issued on GET /session.
	c.Response().Header().Set("X-CSRF-Token", csrf)
	return httpx.Data(c, status, view, p.ContextRevision)
}

func (h *SessionHandler) get(c echo.Context) error {
	sess := c.Get(sessionKey).(*Session)
	return h.view(c, auth.Get(c), sess.CSRFToken, http.StatusOK)
}

func (h *SessionHandler) login(c echo.Context) error {
	var in struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	ctx := c.Request().Context()
	token, _, err := h.auth.Login(ctx, in.Login, in.Password, h.cookie.token(c))
	if err != nil {
		return err
	}
	p, sess, err := h.auth.Authenticate(ctx, token)
	if err != nil {
		return err
	}
	h.cookie.set(c, token, int(h.auth.cfg.AbsoluteTimeout.Seconds()))
	return h.view(c, p, sess.CSRFToken, http.StatusOK)
}

func (h *SessionHandler) logout(c echo.Context) error {
	if err := h.auth.Logout(c.Request().Context(), auth.Get(c).SessionID); err != nil {
		return err
	}
	h.cookie.set(c, "", -1)
	return c.NoContent(http.StatusNoContent)
}

func (h *SessionHandler) setContext(c echo.Context) error {
	p := auth.Get(c)
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		CompanyID *string `json:"companyId"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if err := h.auth.SetCompany(c.Request().Context(), p, expected, in.CompanyID); err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, ContextView{Revision: revision(expected + 1), CompanyID: in.CompanyID,
		BranchScope: auth.BranchScope{Mode: ScopeAll, BranchIDs: []string{}}}, expected+1)
}

func (h *SessionHandler) setBranchScope(c echo.Context) error {
	p := auth.Get(c)
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in auth.BranchScope
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	scope, err := h.auth.SetBranchScope(c.Request().Context(), p, expected, in)
	if err != nil {
		return err
	}
	company := p.CompanyID
	return httpx.Data(c, http.StatusOK, ContextView{Revision: revision(expected + 1), CompanyID: &company, BranchScope: scope}, expected+1)
}
