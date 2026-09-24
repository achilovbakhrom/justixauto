// Package handler holds financing's Echo routes, request/response DTOs and
// mappers. It imports service, model and internal/pkg only; it must never
// import repository or gorm.
package handler

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"justixauto/internal/modules/financing/model"
	"justixauto/internal/modules/financing/service"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/httpx"
)

// Handler mounts financing's Echo routes.
type Handler struct{ s *service.Service }

// New builds the financing handler.
func New(s *service.Service) *Handler { return &Handler{s} }

func (h *Handler) Routes(g *echo.Group) {
	c := g.Group("", auth.RequireCompany())
	c.GET("/programs", h.listPrograms, auth.Require(model.PermRead))
	c.GET("/programs/:id", h.getProgram, auth.Require(model.PermRead))
	c.POST("/programs", h.createProgram, auth.Require(model.PermPrograms))
	c.POST("/programs/:id/versions", h.addProgramVersion, auth.Require(model.PermPrograms))
	c.POST("/programs/:id/publish", h.publishProgram, auth.Require(model.PermPrograms))
	c.POST("/programs/:id/withdraw", h.withdrawProgram, auth.Require(model.PermPrograms))

	c.GET("/applications", h.list, auth.Require(model.PermRead))
	c.GET("/applications/:id", h.get, auth.Require(model.PermRead))
	c.POST("/applications", h.create, auth.Require(model.PermApply))
	c.PATCH("/applications/:id", h.update, auth.Require(model.PermApply))
	c.POST("/applications/:id/submit", h.submit, auth.Require(model.PermApply))
	c.POST("/applications/:id/responses", h.act("respond"), auth.Require(model.PermApply))
	c.POST("/applications/:id/counter", h.act("counter"), auth.Require(model.PermApply))
	c.POST("/applications/:id/agree", h.act("agree"), auth.Require(model.PermAgree))
	c.POST("/applications/:id/take", h.act("take"), auth.Require(model.PermReview))
	c.POST("/applications/:id/information-requests", h.act("request"), auth.Require(model.PermReview))
	c.POST("/applications/:id/terms", h.act("terms"), auth.Require(model.PermDecide))
	c.POST("/applications/:id/decline", h.act("decline"), auth.Require(model.PermDecide))

	c.GET("/applications/:id/document-requests", h.listDocuments, auth.Require(model.PermRead))
	c.POST("/applications/:id/document-requests", h.requestDocument, auth.Require(model.PermReview))
	c.GET("/document-requests/:id", h.getDocument, auth.Require(model.PermRead))
	c.POST("/document-requests/:id/submissions", h.submitDocument, auth.Require(model.PermApply))
	c.POST("/document-requests/:id/:action", h.decideDocument, auth.Require(model.PermDecide))
}

func (h *Handler) documentResponse(c echo.Context, status int, id string) error {
	v, err := h.s.DocumentRequestView(c.Request().Context(), auth.Get(c), id)
	if err != nil {
		return err
	}
	return httpx.Data(c, status, toDocumentRequest(v), v.Request.Version)
}

// listDocuments lists the document requests on an application.
//
//	@Summary	List document requests
//	@Tags		financing/documents
//	@Param		id			path		string	true	"application ID"
//	@Success	200			{object}	httpx.ListEnvelope[handler.documentRequestDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/financing/applications/{id}/document-requests [get]
func (h *Handler) listDocuments(c echo.Context) error {
	vs, err := h.s.DocumentRequests(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	out := make([]documentRequestDTO, len(vs))
	for i := range vs {
		out[i] = toDocumentRequest(&vs[i])
	}
	return httpx.List(c, out, nil)
}

// getDocument returns one document request with its submissions.
//
//	@Summary	Get document request
//	@Tags		financing/documents
//	@Param		id			path		string	true	"document request ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.documentRequestDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/financing/document-requests/{id} [get]
func (h *Handler) getDocument(c echo.Context) error {
	return h.documentResponse(c, http.StatusOK, c.Param("id"))
}

// requestDocument requests a supporting document on an application.
//
//	@Summary	Request document
//	@Tags		financing/documents
//	@Security	CSRF
//	@Param		id					path		string					true	"application ID"
//	@Param		body				body		requestDocumentRequest	true	"document request"
//	@Success	201					{object}	httpx.DataEnvelope[handler.documentRequestDTO]
//	@Failure	401,403,404,409,422	{object}	httpx.ErrorBody
//	@Router		/financing/applications/{id}/document-requests [post]
func (h *Handler) requestDocument(c echo.Context) error {
	var in requestDocumentRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	d, err := h.s.RequestDocument(c.Request().Context(), auth.Get(c), c.Param("id"), in.Title, in.Requirements)
	if err != nil {
		return err
	}
	return h.documentResponse(c, http.StatusCreated, d.ID)
}

// submitDocument submits a document for a requested attachment.
//
//	@Summary	Submit document
//	@Tags		financing/documents
//	@Security	CSRF
//	@Param		id							path		string					true	"document request ID"
//	@Param		If-Match					header		string					true	"revision"
//	@Param		body						body		submitDocumentRequest	true	"submission"
//	@Success	201							{object}	httpx.DataEnvelope[handler.documentRequestDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/financing/document-requests/{id}/submissions [post]
func (h *Handler) submitDocument(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in submitDocumentRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	d, err := h.s.SubmitDocument(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.AttachmentBindingID, in.Note)
	if err != nil {
		return err
	}
	return h.documentResponse(c, http.StatusCreated, d.ID)
}

// decideDocument accepts, returns or cancels a submitted document.
//
//	@Summary	Decide document
//	@Tags		financing/documents
//	@Security	CSRF
//	@Param		id							path		string					true	"document request ID"
//	@Param		action						path		string					true	"accept, return or cancel"
//	@Param		If-Match					header		string					true	"revision"
//	@Param		body						body		decideDocumentRequest	true	"decision"
//	@Success	200							{object}	httpx.DataEnvelope[handler.documentRequestDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/financing/document-requests/{id}/{action} [post]
func (h *Handler) decideDocument(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in decideDocumentRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	note := in.Note
	if c.Param("action") == "cancel" {
		note = in.Reason
	}
	d, err := h.s.DecideDocument(c.Request().Context(), auth.Get(c), c.Param("id"), expected, c.Param("action"), in.SubmissionVersion, in.Confirmation, note)
	if err != nil {
		return err
	}
	return h.documentResponse(c, http.StatusOK, d.ID)
}

func (h *Handler) programResponse(c echo.Context, status int, id string) error {
	v, err := h.s.Program(c.Request().Context(), auth.Get(c), id)
	if err != nil {
		return err
	}
	return httpx.Data(c, status, toProgram(v), v.Program.Version)
}

// listPrograms searches financing programs.
//
//	@Summary	List programs
//	@Tags		financing/programs
//	@Param		providerId	query		string	false	"provider company ID"
//	@Param		limit		query		int		false	"page size"
//	@Param		offset		query		int		false	"offset"
//	@Success	200			{object}	httpx.ListEnvelope[handler.programDTO]
//	@Failure	401,403,422	{object}	httpx.ErrorBody
//	@Router		/financing/programs [get]
func (h *Handler) listPrograms(c echo.Context) error {
	limit, err := httpx.IntQuery(c, "limit")
	if err != nil {
		return err
	}
	offset, err := httpx.IntQuery(c, "offset")
	if err != nil {
		return err
	}
	vs, err := h.s.Programs(c.Request().Context(), auth.Get(c), c.QueryParam("providerId"), limit, offset)
	if err != nil {
		return err
	}
	out := make([]programDTO, len(vs))
	for i := range vs {
		out[i] = toProgram(&vs[i])
	}
	return httpx.List(c, out, nil)
}

// getProgram returns one financing program with its versions.
//
//	@Summary	Get program
//	@Tags		financing/programs
//	@Param		id			path		string	true	"program ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.programDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/financing/programs/{id} [get]
func (h *Handler) getProgram(c echo.Context) error {
	return h.programResponse(c, http.StatusOK, c.Param("id"))
}

// createProgram creates a financing program with its first version.
//
//	@Summary	Create program
//	@Tags		financing/programs
//	@Security	CSRF
//	@Param		body			body		service.ProgramInput	true	"program"
//	@Success	201				{object}	httpx.DataEnvelope[handler.programDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/financing/programs [post]
func (h *Handler) createProgram(c echo.Context) error {
	var in service.ProgramInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	p, err := h.s.CreateProgram(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return h.programResponse(c, http.StatusCreated, p.ID)
}

// addProgramVersion adds a new version to a financing program.
//
//	@Summary	Add program version
//	@Tags		financing/programs
//	@Security	CSRF
//	@Param		id							path		string					true	"program ID"
//	@Param		If-Match					header		string					true	"revision"
//	@Param		body						body		service.ProgramInput	true	"program version"
//	@Success	201							{object}	httpx.DataEnvelope[handler.programDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/financing/programs/{id}/versions [post]
func (h *Handler) addProgramVersion(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in service.ProgramInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	p, err := h.s.AddProgramVersion(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return h.programResponse(c, http.StatusCreated, p.ID)
}

// publishProgram publishes a program version as the active one.
//
//	@Summary	Publish program
//	@Tags		financing/programs
//	@Security	CSRF
//	@Param		id							path		string					true	"program ID"
//	@Param		If-Match					header		string					true	"revision"
//	@Param		body						body		publishProgramRequest	true	"publication"
//	@Success	200							{object}	httpx.DataEnvelope[handler.programDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/financing/programs/{id}/publish [post]
func (h *Handler) publishProgram(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in publishProgramRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	p, err := h.s.PublishProgram(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.ProgramVersion)
	if err != nil {
		return err
	}
	return h.programResponse(c, http.StatusOK, p.ID)
}

// withdrawProgram withdraws a published financing program.
//
//	@Summary	Withdraw program
//	@Tags		financing/programs
//	@Security	CSRF
//	@Param		id							path		string					true	"program ID"
//	@Param		If-Match					header		string					true	"revision"
//	@Param		body						body		withdrawProgramRequest	true	"withdrawal"
//	@Success	200							{object}	httpx.DataEnvelope[handler.programDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/financing/programs/{id}/withdraw [post]
func (h *Handler) withdrawProgram(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in withdrawProgramRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	p, err := h.s.WithdrawProgram(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Reason)
	if err != nil {
		return err
	}
	return h.programResponse(c, http.StatusOK, p.ID)
}

func (h *Handler) respond(c echo.Context, status int, id string) error {
	p := auth.Get(c)
	v, err := h.s.Get(c.Request().Context(), p, id)
	if err != nil {
		return err
	}
	return httpx.Data(c, status, toApplication(p.CompanyID, v), v.Application.Version)
}

// list searches financing applications for the active company.
//
//	@Summary	List applications
//	@Tags		financing/applications
//	@Param		status		query		string	false	"application status"
//	@Param		limit		query		int		false	"page size"
//	@Param		offset		query		int		false	"offset"
//	@Success	200			{object}	httpx.ListEnvelope[handler.applicationDTO]
//	@Failure	401,403,422	{object}	httpx.ErrorBody
//	@Router		/financing/applications [get]
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
		out[i] = toApplication(p.CompanyID, &service.ApplicationView{Application: as[i]})
	}
	return httpx.List(c, out, nil)
}

// get returns one financing application with its terms and history.
//
//	@Summary	Get application
//	@Tags		financing/applications
//	@Param		id			path		string	true	"application ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.applicationDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/financing/applications/{id} [get]
func (h *Handler) get(c echo.Context) error { return h.respond(c, http.StatusOK, c.Param("id")) }

// create creates a draft financing application for a retail deal.
//
//	@Summary	Create application
//	@Tags		financing/applications
//	@Security	CSRF
//	@Param		body				body		service.ApplicationInput	true	"application"
//	@Success	201					{object}	httpx.DataEnvelope[handler.applicationDTO]
//	@Failure	401,403,404,409,422	{object}	httpx.ErrorBody
//	@Router		/financing/applications [post]
func (h *Handler) create(c echo.Context) error {
	var in service.ApplicationInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	a, err := h.s.Create(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return h.respond(c, http.StatusCreated, a.ID)
}

// update edits a draft financing application.
//
//	@Summary	Update application
//	@Tags		financing/applications
//	@Security	CSRF
//	@Param		id							path		string						true	"application ID"
//	@Param		If-Match					header		string						true	"revision"
//	@Param		body						body		service.ApplicationInput	true	"application"
//	@Success	200							{object}	httpx.DataEnvelope[handler.applicationDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/financing/applications/{id} [patch]
func (h *Handler) update(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in service.ApplicationInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	a, err := h.s.UpdateDraft(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return h.respond(c, http.StatusOK, a.ID)
}

// submit submits a draft financing application for review.
//
//	@Summary	Submit application
//	@Tags		financing/applications
//	@Security	CSRF
//	@Param		id							path		string						true	"application ID"
//	@Param		If-Match					header		string						true	"revision"
//	@Param		body						body		submitApplicationRequest	true	"submission"
//	@Success	200							{object}	httpx.DataEnvelope[handler.applicationDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/financing/applications/{id}/submit [post]
func (h *Handler) submit(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in submitApplicationRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	rev, _ := strconv.ParseInt(in.DealRevision, 10, 64)
	a, err := h.s.Submit(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Confirmation, rev, in.CalculationDigest)
	if err != nil {
		return err
	}
	return h.respond(c, http.StatusOK, a.ID)
}

// act performs a state transition on a financing application: the seller
// responds to an information request, counters or agrees to terms, or the
// provider takes/reviews it, requests information, issues terms or declines.
//
//	@Summary	Act on application
//	@Tags		financing/applications
//	@Security	CSRF
//	@Param		id							path		string				true	"application ID"
//	@Param		If-Match					header		string				true	"revision"
//	@Param		body						body		service.ActInput	true	"action"
//	@Success	200							{object}	httpx.DataEnvelope[handler.applicationDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/financing/applications/{id}/responses [post]
//	@Router		/financing/applications/{id}/counter [post]
//	@Router		/financing/applications/{id}/agree [post]
//	@Router		/financing/applications/{id}/take [post]
//	@Router		/financing/applications/{id}/information-requests [post]
//	@Router		/financing/applications/{id}/terms [post]
//	@Router		/financing/applications/{id}/decline [post]
func (h *Handler) act(action string) echo.HandlerFunc {
	return func(c echo.Context) error {
		expected, err := httpx.IfMatch(c)
		if err != nil {
			return err
		}
		var in service.ActInput
		if err := httpx.Bind(c, &in); err != nil {
			return err
		}
		a, err := h.s.Act(c.Request().Context(), auth.Get(c), c.Param("id"), expected, action, in)
		if err != nil {
			return err
		}
		return h.respond(c, http.StatusOK, a.ID)
	}
}
