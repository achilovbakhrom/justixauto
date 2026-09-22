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
	g.POST("/session/password", h.changePassword, auth.Require())
	g.POST("/session/mfa/verify", h.verifyMFA)
	g.POST("/session/mfa/enrollment", h.startEnrollment, auth.Require())
	g.POST("/session/mfa/enrollment/:id/confirm", h.confirmEnrollment, auth.Require())
	g.POST("/session/mfa/step-up", h.stepUp, auth.Require())
	g.PUT("/session/context", h.setContext, auth.Require())
	g.PUT("/session/branch-scope", h.setBranchScope, auth.Require())
}

func (h *SessionHandler) view(c echo.Context, p *auth.Principal, sess *Session) error {
	view, err := h.auth.View(c.Request().Context(), p, sess)
	if err != nil {
		return err
	}
	// The browser keeps the CSRF token in memory; it is re-issued on GET /session.
	c.Response().Header().Set("X-CSRF-Token", sess.CSRFToken)
	return httpx.Data(c, http.StatusOK, view, p.ContextRevision)
}

func (h *SessionHandler) get(c echo.Context) error {
	return h.view(c, auth.Get(c), c.Get(sessionKey).(*Session))
}

// signedIn sets the session cookie for a new token and returns the session view.
func (h *SessionHandler) signedIn(c echo.Context, token string) error {
	p, sess, err := h.auth.Authenticate(c.Request().Context(), token)
	if err != nil {
		return err
	}
	h.cookie.set(c, token, int(h.auth.cfg.AbsoluteTimeout.Seconds()))
	return h.view(c, p, sess)
}

func (h *SessionHandler) login(c echo.Context) error {
	var in struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	r, err := h.auth.Login(c.Request().Context(), in.Login, in.Password, h.cookie.token(c))
	if err != nil {
		return err
	}
	if r.ChallengeID != "" {
		// No session yet: only a short-lived cookie binding the challenge to this browser.
		h.cookie.setChallenge(c, r.ChallengeToken, int(mfaChallengeTTL.Seconds()))
		return httpx.Data(c, http.StatusOK, map[string]string{"challengeId": r.ChallengeID, "required": "mfa"}, 0)
	}
	return h.signedIn(c, r.Token)
}

func (h *SessionHandler) verifyMFA(c echo.Context) error {
	var in struct {
		ChallengeID string `json:"challengeId"`
		Code        string `json:"code"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	token, _, err := h.auth.VerifyChallenge(c.Request().Context(), in.ChallengeID, h.cookie.challengeToken(c), in.Code, h.cookie.token(c))
	if err != nil {
		return err
	}
	h.cookie.setChallenge(c, "", -1)
	return h.signedIn(c, token)
}

func (h *SessionHandler) startEnrollment(c echo.Context) error {
	e, err := h.auth.StartEnrollment(c.Request().Context(), auth.Get(c))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, e, 1)
}

func (h *SessionHandler) confirmEnrollment(c echo.Context) error {
	var in struct {
		Code string `json:"code"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	codes, err := h.auth.ConfirmEnrollment(c.Request().Context(), auth.Get(c), c.Param("id"), in.Code)
	if err != nil {
		return err
	}
	// Recovery codes are shown only in this response.
	return httpx.Data(c, http.StatusOK, map[string]any{"enrolled": true, "recoveryCodes": codes}, 1)
}

func (h *SessionHandler) stepUp(c echo.Context) error {
	var in struct {
		Code string `json:"code"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	ctx := c.Request().Context()
	if err := h.auth.StepUp(ctx, auth.Get(c), in.Code); err != nil {
		return err
	}
	p, sess, err := h.auth.Authenticate(ctx, h.cookie.token(c))
	if err != nil {
		return err
	}
	return h.view(c, p, sess)
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

func (h *SessionHandler) changePassword(c echo.Context) error {
	var in struct {
		CurrentPassword         string `json:"currentPassword"`
		NewPassword             string `json:"newPassword"`
		NewPasswordConfirmation string `json:"newPasswordConfirmation"`
	}
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
