package insurance

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/httpx"
)

type messageDTO struct {
	Kind       string    `json:"kind"`
	RequestID  *string   `json:"requestId"`
	Note       string    `json:"note"`
	Side       string    `json:"side"` // seller | insurer
	ActorID    string    `json:"actorId"`
	OccurredAt time.Time `json:"occurredAt"`
}

type applicationDTO struct {
	ID               string          `json:"id"`
	Side             string          `json:"side"` // the caller's side
	SellerCompanyID  string          `json:"sellerCompanyId"`
	InsurerCompanyID string          `json:"insurerCompanyId"`
	RetailDealID     string          `json:"retailDealId"`
	Status           string          `json:"status"`
	Note             string          `json:"note"`
	Snapshot         json.RawMessage `json:"snapshot"`
	History          []messageDTO    `json:"history,omitempty"`
	AllowedActions   []string        `json:"allowedActions"`
	Revision         string          `json:"revision"`
	SubmittedAt      *time.Time      `json:"submittedAt"`
	DecidedAt        *time.Time      `json:"decidedAt"`
}

func actions(a *Application, seller bool) []string {
	switch {
	case seller && a.Status == "draft":
		return []string{"edit", "submit"}
	case seller && a.Status == "needs-info":
		return []string{"respond"}
	case !seller && a.Status == "submitted":
		return []string{"take"}
	case !seller && a.Status == "review":
		return []string{"request", "approve", "decline"}
	}
	return []string{}
}

func toApplication(companyID string, a *Application, ms []Message) applicationDTO {
	seller := a.SellerCompanyID == companyID
	side := "insurer"
	if seller {
		side = "seller"
	}
	snap := json.RawMessage("null")
	if len(a.Snapshot) > 0 {
		snap = a.Snapshot
	}
	d := applicationDTO{
		ID: a.ID, Side: side, SellerCompanyID: a.SellerCompanyID, InsurerCompanyID: a.InsurerCompanyID,
		RetailDealID: a.DealID, Status: a.Status, Note: a.Note, Snapshot: snap, AllowedActions: actions(a, seller),
		Revision: httpx.Revision(a.Version), SubmittedAt: a.SubmittedAt, DecidedAt: a.DecidedAt,
	}
	if ms != nil {
		d.History = []messageDTO{}
		for _, m := range ms {
			s := "insurer"
			if m.CompanyID == a.SellerCompanyID {
				s = "seller"
			}
			d.History = append(d.History, messageDTO{Kind: m.Kind, RequestID: m.RequestID, Note: m.Note, Side: s, ActorID: m.ActorUserID, OccurredAt: m.CreatedAt})
		}
	}
	return d
}

type Handler struct{ s *Service }

func (h *Handler) Routes(g *echo.Group) {
	c := g.Group("", auth.RequireCompany())
	c.GET("/applications", h.list, auth.Require(PermRead))
	c.GET("/applications/:id", h.get, auth.Require(PermRead))
	c.POST("/applications", h.create, auth.Require(PermApply))
	c.PATCH("/applications/:id", h.update, auth.Require(PermApply))
	c.POST("/applications/:id/submit", h.submit, auth.Require(PermApply))
	c.POST("/applications/:id/responses", h.act("respond"), auth.Require(PermApply))
	c.POST("/applications/:id/take", h.act("take"), auth.Require(PermReview))
	c.POST("/applications/:id/information-requests", h.act("request"), auth.Require(PermReview))
	c.POST("/applications/:id/approve", h.act("approve"), auth.Require(PermDecide))
	c.POST("/applications/:id/decline", h.act("decline"), auth.Require(PermDecide))
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
//	@Success	200			{object}	httpx.ListEnvelope[insurance.applicationDTO]
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
//	@Success	200			{object}	httpx.DataEnvelope[insurance.applicationDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/insurance/applications/{id} [get]
func (h *Handler) get(c echo.Context) error { return h.respond(c, http.StatusOK, c.Param("id")) }

// create creates a draft insurance application for an installment sale.
//
//	@Summary	Create insurance application
//	@Tags		insurance/applications
//	@Security	CSRF
//	@Param		Idempotency-Key		header		string		true	"retry key"
//	@Param		body				body		CreateInput	true	"application"
//	@Success	201					{object}	httpx.DataEnvelope[insurance.applicationDTO]
//	@Failure	401,403,404,409,422	{object}	httpx.ErrorBody
//	@Router		/insurance/applications [post]
func (h *Handler) create(c echo.Context) error {
	var in CreateInput
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
//	@Success	200							{object}	httpx.DataEnvelope[insurance.applicationDTO]
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
//	@Param		Idempotency-Key				header		string			true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[insurance.applicationDTO]
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
//	@Param		Idempotency-Key				header		string		true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[insurance.applicationDTO]
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

type Module struct {
	handler *Handler
	Service *Service
}

func New(db *gorm.DB, now func() time.Time, sales Sales, directory Directory) *Module {
	if now == nil {
		now = time.Now
	}
	s := &Service{repo: &repository{db}, sales: sales, directory: directory, now: now}
	return &Module{handler: &Handler{s}, Service: s}
}

// Register mounts the insurance routes under /api/v1/insurance.
func (m *Module) Register(api *echo.Group) { m.handler.Routes(api.Group("/insurance")) }
