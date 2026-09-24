package handler

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"justixauto/internal/modules/insurance/model"
	"justixauto/internal/modules/insurance/service"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/httpx"
)

// Handler holds the service behind the insurance HTTP routes.
type Handler struct{ s *service.Service }

// New builds the insurance handler from its service.
func New(s *service.Service) *Handler { return &Handler{s} }

func (h *Handler) Routes(g *echo.Group) {
	c := g.Group("", auth.RequireCompany())
	c.GET("/applications", h.list, auth.Require(model.PermRead))
	c.GET("/applications/:id", h.get, auth.Require(model.PermRead))
	c.POST("/applications", h.create, auth.Require(model.PermApply))
	c.PATCH("/applications/:id", h.update, auth.Require(model.PermApply))
	c.POST("/applications/:id/submit", h.submit, auth.Require(model.PermApply))
	c.POST("/applications/:id/responses", h.act("respond"), auth.Require(model.PermApply))
	c.POST("/applications/:id/take", h.act("take"), auth.Require(model.PermReview))
	c.POST("/applications/:id/information-requests", h.act("request"), auth.Require(model.PermReview))
	c.POST("/applications/:id/approve", h.act("approve"), auth.Require(model.PermDecide))
	c.POST("/applications/:id/decline", h.act("decline"), auth.Require(model.PermDecide))
}

// updateDraftRequest edits a draft insurance application.
type updateDraftRequest struct {
	InsurerCompanyID string `json:"insurerCompanyId"`
	Note             string `json:"note"`
}

// submitRequest submits a draft insurance application for review.
type submitRequest struct {
	Confirmation bool   `json:"confirmation"`
	DealRevision string `json:"dealRevision"`
}

// actRequest carries an optional note for an insurance application transition.
type actRequest struct {
	Note string `json:"note"`
}

func (h *Handler) respond(c echo.Context, status int, id string) error {
	p := auth.Get(c)
	a, ms, err := h.s.Get(c.Request().Context(), p, id)
	if err != nil {
		return err
	}
	return httpx.Data(c, status, toApplication(p.CompanyID, a, ms), a.Version)
}

// list lists insurance applications for the active company.
//
//	@Summary	List insurance applications
//	@Tags		insurance/applications
//	@Param		status		query		string	false	"application status"
//	@Param		limit		query		int		false	"page size"
//	@Param		offset		query		int		false	"offset"
//	@Success	200			{object}	httpx.ListEnvelope[handler.applicationDTO]
//	@Failure	401,403,422	{object}	httpx.ErrorBody
//	@Router		/insurance/applications [get]
func (h *Handler) list(c echo.Context) error {
	limit, err := httpx.IntQuery(c, "limit")
	if err != nil {
		return err
	}
	offset, err := httpx.IntQuery(c, "offset")
	if err != nil {
		return err
	}
	p := auth.Get(c)
	as, err := h.s.List(c.Request().Context(), p, c.QueryParam("status"), limit, offset)
	if err != nil {
		return err
	}
	out := make([]applicationDTO, len(as))
	for i := range as {
		out[i] = toApplication(p.CompanyID, &as[i], nil)
	}
	return httpx.List(c, out, nil)
}

// get returns one insurance application.
//
//	@Summary	Get insurance application
//	@Tags		insurance/applications
//	@Param		id			path		string	true	"application ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.applicationDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/insurance/applications/{id} [get]
func (h *Handler) get(c echo.Context) error { return h.respond(c, http.StatusOK, c.Param("id")) }

// create creates a draft insurance application for an installment sale.
//
//	@Summary	Create insurance application
//	@Tags		insurance/applications
//	@Security	CSRF
//	@Param		body				body		service.CreateInput	true	"application"
//	@Success	201					{object}	httpx.DataEnvelope[handler.applicationDTO]
//	@Failure	401,403,404,409,422	{object}	httpx.ErrorBody
//	@Router		/insurance/applications [post]
func (h *Handler) create(c echo.Context) error {
	var in service.CreateInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	a, err := h.s.Create(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return h.respond(c, http.StatusCreated, a.ID)
}

// update edits a draft insurance application.
//
//	@Summary	Update insurance application
//	@Tags		insurance/applications
//	@Security	CSRF
//	@Param		id							path		string				true	"application ID"
//	@Param		If-Match					header		string				true	"revision"
//	@Param		body						body		updateDraftRequest	true	"application"
//	@Success	200							{object}	httpx.DataEnvelope[handler.applicationDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/insurance/applications/{id} [patch]
func (h *Handler) update(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in updateDraftRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	a, err := h.s.UpdateDraft(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.InsurerCompanyID, in.Note)
	if err != nil {
		return err
	}
	return h.respond(c, http.StatusOK, a.ID)
}

// submit submits a draft insurance application for the insurer's review.
//
//	@Summary	Submit insurance application
//	@Tags		insurance/applications
//	@Security	CSRF
//	@Param		id							path		string			true	"application ID"
//	@Param		If-Match					header		string			true	"revision"
//	@Param		body						body		submitRequest	true	"submission"
//	@Success	200							{object}	httpx.DataEnvelope[handler.applicationDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/insurance/applications/{id}/submit [post]
func (h *Handler) submit(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in submitRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	var rev int64
	_ = json.Unmarshal([]byte(in.DealRevision), &rev)
	a, err := h.s.Submit(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Confirmation, rev)
	if err != nil {
		return err
	}
	return h.respond(c, http.StatusOK, a.ID)
}

// act performs a state transition on an insurance application: the seller
// responds to an information request, the insurer takes/reviews it, or the
// insurer requests information, approves or declines it.
//
//	@Summary	Act on insurance application
//	@Tags		insurance/applications
//	@Security	CSRF
//	@Param		id							path		string		true	"application ID"
//	@Param		If-Match					header		string		true	"revision"
//	@Param		body						body		actRequest	true	"note"
//	@Success	200							{object}	httpx.DataEnvelope[handler.applicationDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/insurance/applications/{id}/responses [post]
//	@Router		/insurance/applications/{id}/take [post]
//	@Router		/insurance/applications/{id}/information-requests [post]
//	@Router		/insurance/applications/{id}/approve [post]
//	@Router		/insurance/applications/{id}/decline [post]
func (h *Handler) act(action string) echo.HandlerFunc {
	return func(c echo.Context) error {
		expected, err := httpx.IfMatch(c)
		if err != nil {
			return err
		}
		var in actRequest
		if err := httpx.Bind(c, &in); err != nil {
			return err
		}
		a, err := h.s.Act(c.Request().Context(), auth.Get(c), c.Param("id"), expected, action, in.Note)
		if err != nil {
			return err
		}
		return h.respond(c, http.StatusOK, a.ID)
	}
}
