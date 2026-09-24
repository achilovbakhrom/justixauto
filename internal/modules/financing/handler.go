package financing

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/httpx"
)

type programVersionDTO struct {
	Number                   string       `json:"number"`
	Name                     string       `json:"name"`
	Currency                 string       `json:"currency"`
	Terms                    ProgramTerms `json:"terms"`
	Eligibility              Eligibility  `json:"eligibility"`
	CalculationPolicyID      string       `json:"calculationPolicyId"`
	CalculationPolicyVersion int          `json:"calculationPolicyVersion"`
	CreatedAt                time.Time    `json:"createdAt"`
}

type programDTO struct {
	ID               string              `json:"id"`
	Provider         map[string]string   `json:"provider"`
	Status           string              `json:"status"`
	StatusReason     string              `json:"statusReason"`
	PublishedVersion *int                `json:"publishedVersion"`
	Versions         []programVersionDTO `json:"versions"`
	Revision         string              `json:"revision"`
}

func toProgram(v *ProgramView) programDTO {
	d := programDTO{ID: v.Program.ID, Provider: map[string]string{"id": v.Provider.ID, "name": v.Provider.Name, "kind": v.Provider.Kind},
		Status: v.Program.Status, StatusReason: v.Program.StatusReason, PublishedVersion: v.Program.PublishedVersion,
		Versions: []programVersionDTO{}, Revision: httpx.Revision(v.Program.Version)}
	for i := range v.Versions {
		pv := &v.Versions[i]
		t, e := pv.decode()
		d.Versions = append(d.Versions, programVersionDTO{Number: strconv.Itoa(pv.Number), Name: pv.Name, Currency: pv.Currency,
			Terms: t, Eligibility: e, CalculationPolicyID: pv.PolicyID, CalculationPolicyVersion: pv.PolicyVersion, CreatedAt: pv.CreatedAt})
	}
	return d
}

type termsDTO struct {
	Number      int             `json:"number"`
	Calculation json.RawMessage `json:"calculation"`
	Note        string          `json:"note"`
	CreatedAt   time.Time       `json:"createdAt"`
}

type messageDTO struct {
	Kind         string    `json:"kind"`
	RequestID    *string   `json:"requestId"`
	TermsVersion *int      `json:"termsVersion"`
	Note         string    `json:"note"`
	Side         string    `json:"side"`
	ActorID      string    `json:"actorId"`
	OccurredAt   time.Time `json:"occurredAt"`
}

type applicationDTO struct {
	ID                  string          `json:"id"`
	Side                string          `json:"side"`
	SellerCompanyID     string          `json:"sellerCompanyId"`
	ProviderCompanyID   string          `json:"providerCompanyId"`
	RetailDealID        string          `json:"retailDealId"`
	ProgramID           *string         `json:"programId"`
	ProgramVersion      *int            `json:"programVersion"`
	Calculation         json.RawMessage `json:"calculation"`
	CalculationDigest   string          `json:"calculationDigest"`
	Status              string          `json:"status"`
	Snapshot            json.RawMessage `json:"snapshot"`
	CurrentTermsVersion *int            `json:"currentTermsVersion"`
	Terms               []termsDTO      `json:"terms,omitempty"`
	History             []messageDTO    `json:"history,omitempty"`
	AllowedActions      []string        `json:"allowedActions"`
	Revision            string          `json:"revision"`
}

func raw(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("null")
	}
	return b
}

func toApplication(companyID string, v *ApplicationView) applicationDTO {
	a := v.Application
	seller := a.SellerCompanyID == companyID
	side := map[bool]string{true: "seller", false: "provider"}[seller]
	d := applicationDTO{ID: a.ID, Side: side, SellerCompanyID: a.SellerCompanyID, ProviderCompanyID: a.ProviderCompanyID,
		RetailDealID: a.DealID, ProgramID: a.ProgramID, ProgramVersion: a.ProgramVersion, Calculation: raw(a.Calculation),
		CalculationDigest: a.CalculationDigest, Status: a.Status, Snapshot: raw(a.Snapshot), CurrentTermsVersion: a.CurrentTermsVersion,
		AllowedActions: []string{}, Revision: httpx.Revision(a.Version)}
	switch {
	case seller && a.Status == "draft":
		d.AllowedActions = []string{"edit", "submit"}
	case seller && a.Status == "needs-info":
		d.AllowedActions = []string{"respond"}
	case seller && a.Status == "terms":
		d.AllowedActions = []string{"agree", "counter"}
	case !seller && a.Status == "submitted":
		d.AllowedActions = []string{"take"}
	case !seller && a.Status == "review":
		d.AllowedActions = []string{"request", "terms", "decline"}
	}
	if v.Terms != nil {
		d.Terms = []termsDTO{}
		for _, t := range v.Terms {
			d.Terms = append(d.Terms, termsDTO{Number: t.Number, Calculation: t.Calculation, Note: t.Note, CreatedAt: t.CreatedAt})
		}
		d.History = []messageDTO{}
		for _, m := range v.History {
			d.History = append(d.History, messageDTO{Kind: m.Kind, RequestID: m.RequestID, TermsVersion: m.TermsVersion, Note: m.Note,
				Side: map[bool]string{true: "seller", false: "provider"}[m.CompanyID == a.SellerCompanyID], ActorID: m.ActorUserID, OccurredAt: m.CreatedAt})
		}
	}
	return d
}

type Handler struct{ s *Service }

// publishProgramRequest publishes a financing program version.
type publishProgramRequest struct {
	ProgramVersion int `json:"programVersion"`
}

// withdrawProgramRequest withdraws a published financing program.
type withdrawProgramRequest struct {
	Reason string `json:"reason"`
}

// submitApplicationRequest submits a draft financing application for review.
type submitApplicationRequest struct {
	Confirmation      bool   `json:"confirmation"`
	DealRevision      string `json:"dealRevision"`
	CalculationDigest string `json:"calculationDigest"`
}

// requestDocumentRequest requests a supporting document on a financing application.
type requestDocumentRequest struct {
	Title        string `json:"title"`
	Requirements string `json:"requirements"`
}

// submitDocumentRequest submits a document for a requested attachment.
type submitDocumentRequest struct {
	AttachmentBindingID string `json:"attachmentBindingId"`
	Note                string `json:"note"`
}

// decideDocumentRequest accepts, returns or cancels a document submission.
type decideDocumentRequest struct {
	SubmissionVersion int    `json:"submissionVersion"`
	Confirmation      bool   `json:"confirmation"`
	Note              string `json:"note"`
	Reason            string `json:"reason"`
}

func (h *Handler) Routes(g *echo.Group) {
	c := g.Group("", auth.RequireCompany())
	c.GET("/programs", h.listPrograms, auth.Require(PermRead))
	c.GET("/programs/:id", h.getProgram, auth.Require(PermRead))
	c.POST("/programs", h.createProgram, auth.Require(PermPrograms))
	c.POST("/programs/:id/versions", h.addProgramVersion, auth.Require(PermPrograms))
	c.POST("/programs/:id/publish", h.publishProgram, auth.Require(PermPrograms))
	c.POST("/programs/:id/withdraw", h.withdrawProgram, auth.Require(PermPrograms))

	c.GET("/applications", h.list, auth.Require(PermRead))
	c.GET("/applications/:id", h.get, auth.Require(PermRead))
	c.POST("/applications", h.create, auth.Require(PermApply))
	c.PATCH("/applications/:id", h.update, auth.Require(PermApply))
	c.POST("/applications/:id/submit", h.submit, auth.Require(PermApply))
	c.POST("/applications/:id/responses", h.act("respond"), auth.Require(PermApply))
	c.POST("/applications/:id/counter", h.act("counter"), auth.Require(PermApply))
	c.POST("/applications/:id/agree", h.act("agree"), auth.Require(PermAgree))
	c.POST("/applications/:id/take", h.act("take"), auth.Require(PermReview))
	c.POST("/applications/:id/information-requests", h.act("request"), auth.Require(PermReview))
	c.POST("/applications/:id/terms", h.act("terms"), auth.Require(PermDecide))
	c.POST("/applications/:id/decline", h.act("decline"), auth.Require(PermDecide))

	c.GET("/applications/:id/document-requests", h.listDocuments, auth.Require(PermRead))
	c.POST("/applications/:id/document-requests", h.requestDocument, auth.Require(PermReview))
	c.GET("/document-requests/:id", h.getDocument, auth.Require(PermRead))
	c.POST("/document-requests/:id/submissions", h.submitDocument, auth.Require(PermApply))
	c.POST("/document-requests/:id/:action", h.decideDocument, auth.Require(PermDecide))
}

type submissionDTO struct {
	Version   int       `json:"version"`
	FileID    string    `json:"fileId"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
}

type documentRequestDTO struct {
	ID            string          `json:"id"`
	ApplicationID string          `json:"applicationId"`
	Title         string          `json:"title"`
	Requirements  string          `json:"requirements"`
	Status        string          `json:"status"`
	StatusNote    string          `json:"statusNote"`
	Submissions   []submissionDTO `json:"submissions"`
	Revision      string          `json:"revision"`
}

func toDocumentRequest(v *DocumentRequestView) documentRequestDTO {
	d := documentRequestDTO{ID: v.Request.ID, ApplicationID: v.Request.ApplicationID, Title: v.Request.Title,
		Requirements: v.Request.Requirements, Status: v.Request.Status, StatusNote: v.Request.StatusNote,
		Submissions: []submissionDTO{}, Revision: httpx.Revision(v.Request.Version)}
	for _, s := range v.Submissions {
		d.Submissions = append(d.Submissions, submissionDTO{Version: s.Number, FileID: s.FileID, Note: s.Note, CreatedAt: s.CreatedAt})
	}
	return d
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
//	@Success	200			{object}	httpx.ListEnvelope[financing.documentRequestDTO]
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
//	@Success	200			{object}	httpx.DataEnvelope[financing.documentRequestDTO]
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
//	@Param		Idempotency-Key		header		string					true	"retry key"
//	@Param		body				body		requestDocumentRequest	true	"document request"
//	@Success	201					{object}	httpx.DataEnvelope[financing.documentRequestDTO]
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
//	@Param		Idempotency-Key				header		string					true	"retry key"
//	@Success	201							{object}	httpx.DataEnvelope[financing.documentRequestDTO]
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
//	@Param		Idempotency-Key				header		string					true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[financing.documentRequestDTO]
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
//	@Success	200			{object}	httpx.ListEnvelope[financing.programDTO]
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
//	@Success	200			{object}	httpx.DataEnvelope[financing.programDTO]
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
//	@Param		Idempotency-Key	header		string			true	"retry key"
//	@Param		body			body		ProgramInput	true	"program"
//	@Success	201				{object}	httpx.DataEnvelope[financing.programDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/financing/programs [post]
func (h *Handler) createProgram(c echo.Context) error {
	var in ProgramInput
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
//	@Param		id							path		string			true	"program ID"
//	@Param		If-Match					header		string			true	"revision"
//	@Param		body						body		ProgramInput	true	"program version"
//	@Param		Idempotency-Key				header		string			true	"retry key"
//	@Success	201							{object}	httpx.DataEnvelope[financing.programDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/financing/programs/{id}/versions [post]
func (h *Handler) addProgramVersion(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in ProgramInput
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
//	@Param		Idempotency-Key				header		string					true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[financing.programDTO]
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
//	@Param		Idempotency-Key				header		string					true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[financing.programDTO]
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
//	@Success	200			{object}	httpx.ListEnvelope[financing.applicationDTO]
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
		out[i] = toApplication(p.CompanyID, &ApplicationView{Application: as[i]})
	}
	return httpx.List(c, out, nil)
}

// get returns one financing application with its terms and history.
//
//	@Summary	Get application
//	@Tags		financing/applications
//	@Param		id			path		string	true	"application ID"
//	@Success	200			{object}	httpx.DataEnvelope[financing.applicationDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/financing/applications/{id} [get]
func (h *Handler) get(c echo.Context) error { return h.respond(c, http.StatusOK, c.Param("id")) }

// create creates a draft financing application for a retail deal.
//
//	@Summary	Create application
//	@Tags		financing/applications
//	@Security	CSRF
//	@Param		Idempotency-Key		header		string				true	"retry key"
//	@Param		body				body		ApplicationInput	true	"application"
//	@Success	201					{object}	httpx.DataEnvelope[financing.applicationDTO]
//	@Failure	401,403,404,409,422	{object}	httpx.ErrorBody
//	@Router		/financing/applications [post]
func (h *Handler) create(c echo.Context) error {
	var in ApplicationInput
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
//	@Param		id							path		string				true	"application ID"
//	@Param		If-Match					header		string				true	"revision"
//	@Param		body						body		ApplicationInput	true	"application"
//	@Success	200							{object}	httpx.DataEnvelope[financing.applicationDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/financing/applications/{id} [patch]
func (h *Handler) update(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in ApplicationInput
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
//	@Param		Idempotency-Key				header		string						true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[financing.applicationDTO]
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
//	@Param		id							path		string		true	"application ID"
//	@Param		If-Match					header		string		true	"revision"
//	@Param		body						body		ActInput	true	"action"
//	@Param		Idempotency-Key				header		string		true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[financing.applicationDTO]
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
		var in ActInput
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

type Module struct{ handler *Handler }

func New(db *gorm.DB, now func() time.Time, sales Sales, directory Directory, files Files) *Module {
	if now == nil {
		now = time.Now
	}
	return &Module{handler: &Handler{&Service{r: &repo{db}, sales: sales, directory: directory, files: files, now: now}}}
}

// Register mounts the financing routes under /api/v1/financing.
func (m *Module) Register(api *echo.Group) { m.handler.Routes(api.Group("/financing")) }
