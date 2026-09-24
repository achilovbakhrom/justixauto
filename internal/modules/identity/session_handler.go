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

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// loginChallengeResponse is returned instead of a session when MFA is required.
type loginChallengeResponse struct {
	ChallengeID string `json:"challengeId"`
	Required    string `json:"required"`
}

type verifyMFARequest struct {
	ChallengeID string `json:"challengeId"`
	Code        string `json:"code"`
}

type mfaCodeRequest struct {
	Code string `json:"code"`
}

// confirmEnrollmentResponse shows the recovery codes once, at enrollment time.
type confirmEnrollmentResponse struct {
	Enrolled      bool     `json:"enrolled"`
	RecoveryCodes []string `json:"recoveryCodes"`
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

// get returns the signed-in session view.
//
//	@Summary	Get session
//	@Tags		identity/session
//	@Success	200	{object}	httpx.DataEnvelope[identity.SessionView]
//	@Failure	401	{object}	httpx.ErrorBody
//	@Router		/identity/session [get]
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

// login authenticates with login/password, starting an MFA challenge if enrolled.
//
//	@Summary	Log in
//	@Tags		identity/session
//	@Param		body		body		loginRequest								true	"credentials"
//	@Success	200			{object}	httpx.DataEnvelope[identity.SessionView]	"signed in; when MFA is required, data is {challengeId, required: mfa} instead (then POST /identity/session/mfa/verify)"
//	@Failure	401,422,429	{object}	httpx.ErrorBody
//	@Router		/identity/session/login [post]
func (h *SessionHandler) login(c echo.Context) error {
	var in loginRequest
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
		return httpx.Data(c, http.StatusOK, loginChallengeResponse{ChallengeID: r.ChallengeID, Required: "mfa"}, 0)
	}
	return h.signedIn(c, r.Token)
}

// verifyMFA completes login by verifying the second factor for a pending challenge.
//
//	@Summary	Verify MFA challenge
//	@Tags		identity/session
//	@Param		body		body		verifyMFARequest	true	"challenge response"
//	@Success	200			{object}	httpx.DataEnvelope[identity.SessionView]
//	@Failure	401,422,429	{object}	httpx.ErrorBody
//	@Router		/identity/session/mfa/verify [post]
func (h *SessionHandler) verifyMFA(c echo.Context) error {
	var in verifyMFARequest
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

// startEnrollment creates a pending TOTP secret for the signed-in user.
//
//	@Summary	Start MFA enrollment
//	@Tags		identity/session
//	@Security	CSRF
//	@Success	201		{object}	httpx.DataEnvelope[identity.Enrollment]
//	@Failure	401,409	{object}	httpx.ErrorBody
//	@Router		/identity/session/mfa/enrollment [post]
func (h *SessionHandler) startEnrollment(c echo.Context) error {
	e, err := h.auth.StartEnrollment(c.Request().Context(), auth.Get(c))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, e, 1)
}

// confirmEnrollment confirms a pending TOTP secret with the first code.
//
//	@Summary	Confirm MFA enrollment
//	@Tags		identity/session
//	@Security	CSRF
//	@Param		id			path		string			true	"enrollment ID"
//	@Param		body		body		mfaCodeRequest	true	"code"
//	@Success	200			{object}	httpx.DataEnvelope[identity.confirmEnrollmentResponse]
//	@Failure	401,404,422	{object}	httpx.ErrorBody
//	@Router		/identity/session/mfa/enrollment/{id}/confirm [post]
func (h *SessionHandler) confirmEnrollment(c echo.Context) error {
	var in mfaCodeRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	codes, err := h.auth.ConfirmEnrollment(c.Request().Context(), auth.Get(c), c.Param("id"), in.Code)
	if err != nil {
		return err
	}
	// Recovery codes are shown only in this response.
	return httpx.Data(c, http.StatusOK, confirmEnrollmentResponse{Enrolled: true, RecoveryCodes: codes}, 1)
}

// stepUp re-verifies the second factor for a sensitive action in the current session.
//
//	@Summary	Step-up MFA
//	@Tags		identity/session
//	@Security	CSRF
//	@Param		body		body		mfaCodeRequest	true	"code"
//	@Success	200			{object}	httpx.DataEnvelope[identity.SessionView]
//	@Failure	401,422,429	{object}	httpx.ErrorBody
//	@Router		/identity/session/mfa/step-up [post]
func (h *SessionHandler) stepUp(c echo.Context) error {
	var in mfaCodeRequest
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
//	@Success	200					{object}	httpx.DataEnvelope[identity.ContextView]
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
	return httpx.Data(c, http.StatusOK, ContextView{Revision: revision(expected + 1), CompanyID: in.CompanyID,
		BranchScope: auth.BranchScope{Mode: ScopeAll, BranchIDs: []string{}}}, expected+1)
}

// setBranchScope narrows the signed-in membership's branch scope.
//
//	@Summary	Set session branch scope
//	@Tags		identity/session
//	@Security	CSRF
//	@Param		If-Match			header		string				true	"revision"
//	@Param		body				body		auth.BranchScope	true	"branch scope"
//	@Success	200					{object}	httpx.DataEnvelope[identity.ContextView]
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
	return httpx.Data(c, http.StatusOK, ContextView{Revision: revision(expected + 1), CompanyID: &company, BranchScope: scope}, expected+1)
}

// changePassword sets a new password for the signed-in user.
//
//	@Summary	Change password
//	@Tags		identity/session
//	@Security	CSRF
//	@Param		body	body		changePasswordRequest	true	"passwords"
//	@Success	200		{object}	httpx.DataEnvelope[identity.SessionView]
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
