package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"justixauto/internal/modules/identity/model"
	"justixauto/internal/modules/identity/service"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/httpx"
)

// SessionHandler serves the signed-in session's routes: login, password, context.
type SessionHandler struct {
	auth   *service.Auth
	cookie CookieConfig
}

// NewSessionHandler builds the session handler.
func NewSessionHandler(a *service.Auth, cookie CookieConfig) *SessionHandler {
	return &SessionHandler{auth: a, cookie: cookie}
}

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type setContextRequest struct {
	CompanyID *string `json:"companyId"`
}

type changePasswordRequest struct {
	CurrentPassword         string `json:"currentPassword"`
	NewPassword             string `json:"newPassword"`
	NewPasswordConfirmation string `json:"newPasswordConfirmation"`
}

func (h *SessionHandler) Routes(g *echo.Group) {
	g.GET("/session", h.get, auth.Require())
	g.POST("/session/login", h.login)
	g.POST("/session/logout", h.logout, auth.Require())
	g.POST("/session/password", h.changePassword, auth.Require())
	g.PUT("/session/context", h.setContext, auth.Require())
	g.PUT("/session/branch-scope", h.setBranchScope, auth.Require())
}

func (h *SessionHandler) view(c echo.Context, p *auth.Principal, sess *model.Session) error {
	view, err := h.auth.View(c.Request().Context(), p)
	if err != nil {
		return err
	}
	// The browser keeps the CSRF token in memory; it is re-issued on GET /session.
	c.Response().Header().Set("X-CSRF-Token", sess.CSRFToken)
	return httpx.Data(c, http.StatusOK, view, p.ContextRevision)
}

// get returns the signed-in session view.
//
//	@Summary	Get session
//	@Tags		identity/session
//	@Success	200	{object}	httpx.DataEnvelope[service.SessionView]
//	@Failure	401	{object}	httpx.ErrorBody
//	@Router		/identity/session [get]
func (h *SessionHandler) get(c echo.Context) error {
	return h.view(c, auth.Get(c), c.Get(sessionKey).(*model.Session))
}

// signedIn sets the session cookie for a new token and returns the session view.
func (h *SessionHandler) signedIn(c echo.Context, token string) error {
	p, sess, err := h.auth.Authenticate(c.Request().Context(), token)
	if err != nil {
		return err
	}
	h.cookie.set(c, token, int(h.auth.AbsoluteTimeout().Seconds()))
	return h.view(c, p, sess)
}

// login authenticates with login/password and starts a session.
//
//	@Summary	Log in
//	@Tags		identity/session
//	@Param		body		body		loginRequest	true	"credentials"
//	@Success	200			{object}	httpx.DataEnvelope[service.SessionView]
//	@Failure	401,422,429	{object}	httpx.ErrorBody
//	@Router		/identity/session/login [post]
func (h *SessionHandler) login(c echo.Context) error {
	var in loginRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	token, err := h.auth.Login(c.Request().Context(), in.Login, in.Password, h.cookie.token(c))
	if err != nil {
		return err
	}
	return h.signedIn(c, token)
}

// logout revokes the current session.
//
//	@Summary	Log out
//	@Tags		identity/session
//	@Security	CSRF
//	@Success	204	"no content"
//	@Failure	401	{object}	httpx.ErrorBody
//	@Router		/identity/session/logout [post]
func (h *SessionHandler) logout(c echo.Context) error {
	if err := h.auth.Logout(c.Request().Context(), auth.Get(c).SessionID); err != nil {
		return err
	}
	h.cookie.set(c, "", -1)
	return c.NoContent(http.StatusNoContent)
}

// setContext switches the active company for the signed-in user.
//
//	@Summary	Set session company context
//	@Tags		identity/session
//	@Security	CSRF
//	@Param		If-Match			header		string				true	"revision"
//	@Param		body				body		setContextRequest	true	"company context"
//	@Success	200					{object}	httpx.DataEnvelope[service.ContextView]
//	@Failure	401,403,412,422,428	{object}	httpx.ErrorBody
//	@Router		/identity/session/context [put]
func (h *SessionHandler) setContext(c echo.Context) error {
	p := auth.Get(c)
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in setContextRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	if err := h.auth.SetCompany(c.Request().Context(), p, expected, in.CompanyID); err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, service.ContextView{
		Revision: revision(expected + 1), CompanyID: in.CompanyID,
		BranchScope: auth.BranchScope{Mode: model.ScopeAll, BranchIDs: []string{}},
	}, expected+1)
}

// setBranchScope narrows the signed-in membership's branch scope.
//
//	@Summary	Set session branch scope
//	@Tags		identity/session
//	@Security	CSRF
//	@Param		If-Match			header		string				true	"revision"
//	@Param		body				body		auth.BranchScope	true	"branch scope"
//	@Success	200					{object}	httpx.DataEnvelope[service.ContextView]
//	@Failure	401,403,412,422,428	{object}	httpx.ErrorBody
//	@Router		/identity/session/branch-scope [put]
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
	return httpx.Data(c, http.StatusOK, service.ContextView{Revision: revision(expected + 1), CompanyID: &company, BranchScope: scope}, expected+1)
}

// changePassword sets a new password for the signed-in user.
//
//	@Summary	Change password
//	@Tags		identity/session
//	@Security	CSRF
//	@Param		body	body		changePasswordRequest	true	"passwords"
//	@Success	200		{object}	httpx.DataEnvelope[service.SessionView]
//	@Failure	401,422	{object}	httpx.ErrorBody
//	@Router		/identity/session/password [post]
func (h *SessionHandler) changePassword(c echo.Context) error {
	var in changePasswordRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	ctx := c.Request().Context()
	if err := h.auth.ChangePassword(ctx, auth.Get(c), in.CurrentPassword, in.NewPassword, in.NewPasswordConfirmation); err != nil {
		return err
	}
	p, sess, err := h.auth.Authenticate(ctx, h.cookie.token(c))
	if err != nil {
		return err
	}
	return h.view(c, p, sess)
}
