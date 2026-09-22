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
}

func (h *Handler) programResponse(c echo.Context, status int, id string) error {
	v, err := h.s.Program(c.Request().Context(), auth.Get(c), id)
	if err != nil {
		return err
	}
	return httpx.Data(c, status, toProgram(v), v.Program.Version)
}

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

func (h *Handler) getProgram(c echo.Context) error {
	return h.programResponse(c, http.StatusOK, c.Param("id"))
}

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

func (h *Handler) publishProgram(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		ProgramVersion int `json:"programVersion"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	p, err := h.s.PublishProgram(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.ProgramVersion)
	if err != nil {
		return err
	}
	return h.programResponse(c, http.StatusOK, p.ID)
}

func (h *Handler) withdrawProgram(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		Reason string `json:"reason"`
	}
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

func (h *Handler) get(c echo.Context) error { return h.respond(c, http.StatusOK, c.Param("id")) }

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

func (h *Handler) submit(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		Confirmation      bool   `json:"confirmation"`
		DealRevision      string `json:"dealRevision"`
		CalculationDigest string `json:"calculationDigest"`
	}
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

func New(db *gorm.DB, now func() time.Time, sales Sales, directory Directory) *Module {
	if now == nil {
		now = time.Now
	}
	return &Module{handler: &Handler{&Service{r: &repo{db}, sales: sales, directory: directory, now: now}}}
}

// Register mounts the financing routes under /api/v1/financing.
func (m *Module) Register(api *echo.Group) { m.handler.Routes(api.Group("/financing")) }
