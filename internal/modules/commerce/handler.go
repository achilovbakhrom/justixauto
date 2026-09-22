package commerce

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/httpx"
)

type counterpartyDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Country string `json:"country"`
}

type partnershipDTO struct {
	ID             string            `json:"id"`
	Direction      string            `json:"direction"` // outgoing: we asked; incoming: they asked
	Counterparty   counterpartyDTO   `json:"counterparty"`
	Status         PartnershipStatus `json:"status"`
	StatusReason   string            `json:"statusReason"`
	AllowedActions []string          `json:"allowedActions"`
	Revision       string            `json:"revision"`
	CreatedAt      time.Time         `json:"createdAt"`
	ActivatedAt    *time.Time        `json:"activatedAt"`
	ClosedAt       *time.Time        `json:"closedAt"`
}

func toPartnership(companyID string) func(*PartnershipView) partnershipDTO {
	return func(v *PartnershipView) partnershipDTO {
		p := v.Partnership
		dir := "incoming"
		if p.RequesterCompanyID == companyID {
			dir = "outgoing"
		}
		return partnershipDTO{ID: p.ID, Direction: dir,
			Counterparty: counterpartyDTO{ID: v.Counterparty.ID, Name: v.Counterparty.Name, Country: v.Counterparty.Country},
			Status:       p.Status, StatusReason: p.StatusReason, AllowedActions: p.AllowedActions(companyID),
			Revision: httpx.Revision(p.Version), CreatedAt: p.CreatedAt, ActivatedAt: p.ActivatedAt, ClosedAt: p.ClosedAt}
	}
}

func mapSlice[T, D any](items []T, f func(*T) D) []D {
	out := make([]D, len(items))
	for i := range items {
		out[i] = f(&items[i])
	}
	return out
}

type Handler struct{ partnerships *PartnershipService }

func (h *Handler) Routes(g *echo.Group) {
	c := g.Group("", auth.RequireCompany())
	c.GET("/partnerships", h.listPartnerships, auth.Require(PermRead))
	c.GET("/partnerships/:id", h.getPartnership, auth.Require(PermRead))
	c.POST("/partnerships", h.requestPartnership, auth.Require(PermPartnershipsManage))
	c.POST("/partnerships/:id/:action", h.decidePartnership, auth.Require(PermPartnershipsManage))
}

func (h *Handler) listPartnerships(c echo.Context) error {
	p := auth.Get(c)
	f := PartnershipFilter{Status: PartnershipStatus(c.QueryParam("status"))}
	var err error
	if f.Limit, err = httpx.IntQuery(c, "limit"); err != nil {
		return err
	}
	if f.Offset, err = httpx.IntQuery(c, "offset"); err != nil {
		return err
	}
	views, err := h.partnerships.List(c.Request().Context(), p, f)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(views, toPartnership(p.CompanyID)), nil)
}

func (h *Handler) getPartnership(c echo.Context) error {
	p := auth.Get(c)
	v, err := h.partnerships.Get(c.Request().Context(), p, c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toPartnership(p.CompanyID)(v), v.Partnership.Version)
}

func (h *Handler) requestPartnership(c echo.Context) error {
	p := auth.Get(c)
	var in struct {
		CounterpartyCompanyID string `json:"counterpartyCompanyId"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.partnerships.Request(c.Request().Context(), p, in.CounterpartyCompanyID)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toPartnership(p.CompanyID)(v), v.Partnership.Version)
}

func (h *Handler) decidePartnership(c echo.Context) error {
	p := auth.Get(c)
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
	v, err := h.partnerships.Decide(c.Request().Context(), p, c.Param("id"), expected, c.Param("action"), in.Reason)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toPartnership(p.CompanyID)(v), v.Partnership.Version)
}

type Module struct{ handler *Handler }

func New(db *gorm.DB, now func() time.Time, directory Directory) *Module {
	if now == nil {
		now = time.Now
	}
	d := deps{store: NewStore(db), directory: directory, now: now}
	return &Module{handler: &Handler{partnerships: &PartnershipService{d}}}
}

// Register mounts the commerce routes under /api/v1/commerce.
func (m *Module) Register(api *echo.Group) { m.handler.Routes(api.Group("/commerce")) }
