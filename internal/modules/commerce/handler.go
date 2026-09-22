package commerce

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/httpx"
	"justixauto/internal/platform/money"
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

type Handler struct {
	partnerships *PartnershipService
	offers       *OfferService
	deals        *DealService
	fulfilment   *FulfilmentService
	invoices     *InvoiceService
}

func (h *Handler) Routes(g *echo.Group) {
	c := g.Group("", auth.RequireCompany())
	c.GET("/partnerships", h.listPartnerships, auth.Require(PermRead))
	c.GET("/partnerships/:id", h.getPartnership, auth.Require(PermRead))
	c.POST("/partnerships", h.requestPartnership, auth.Require(PermPartnershipsManage))
	c.POST("/partnerships/:id/:action", h.decidePartnership, auth.Require(PermPartnershipsManage))

	c.GET("/offers", h.listOffers, auth.Require(PermRead))
	c.GET("/offers/:id", h.getOffer, auth.Require(PermRead))
	c.POST("/offers", h.createOffer, auth.Require(PermOffersManage))
	c.POST("/offers/:id/versions", h.addOfferVersion, auth.Require(PermOffersManage))
	c.POST("/offers/:id/publish", h.publishOffer, auth.Require(PermOffersManage))
	c.POST("/offers/:id/withdraw", h.withdrawOffer, auth.Require(PermOffersManage))

	h.dealRoutes(c)
	h.invoiceRoutes(c)
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

func New(db *gorm.DB, now func() time.Time, directory Directory, catalog Catalog, stock Stock, files Files) *Module {
	if now == nil {
		now = time.Now
	}
	d := deps{store: NewStore(db), directory: directory, catalog: catalog, stock: stock, files: files, now: now}
	offers := &OfferService{d}
	return &Module{handler: &Handler{partnerships: &PartnershipService{d}, offers: offers, deals: &DealService{deps: d, offers: offers}, fulfilment: &FulfilmentService{d}, invoices: &InvoiceService{d}}}
}

// Register mounts the commerce routes under /api/v1/commerce.
func (m *Module) Register(api *echo.Group) { m.handler.Routes(api.Group("/commerce")) }

// ---- offers ----

type offerVersionDTO struct {
	ID          string      `json:"id"`
	Number      int         `json:"number"`
	Terms       Terms       `json:"terms"`
	Total       money.Money `json:"total"`
	Audience    *Audience   `json:"audience,omitempty"` // supplier only
	CreatedAt   time.Time   `json:"createdAt"`
	PublishedAt *time.Time  `json:"publishedAt"`
}

func toOfferVersion(v *OfferVersion, own bool) offerVersionDTO {
	terms, audience := v.decode()
	d := offerVersionDTO{ID: v.ID, Number: v.Number, Terms: terms, Total: termsTotal(terms), CreatedAt: v.CreatedAt, PublishedAt: v.PublishedAt}
	if own {
		if audience.PartnerCompanyIDs == nil {
			audience.PartnerCompanyIDs = []string{}
		}
		d.Audience = &audience
	}
	return d
}

type offerDTO struct {
	ID               string            `json:"id"`
	Supplier         counterpartyDTO   `json:"supplier"`
	Status           OfferStatus       `json:"status"`
	StatusReason     string            `json:"statusReason"`
	PublishedVersion *offerVersionDTO  `json:"publishedVersion"`
	Versions         []offerVersionDTO `json:"versions,omitempty"` // supplier only
	AllowedActions   []string          `json:"allowedActions"`
	Revision         string            `json:"revision"`
	UpdatedAt        time.Time         `json:"updatedAt"`
}

func toOffer(v *OfferView) offerDTO {
	o := v.Offer
	d := offerDTO{ID: o.ID, Supplier: counterpartyDTO{ID: v.Supplier.ID, Name: v.Supplier.Name, Country: v.Supplier.Country},
		Status: o.Status, StatusReason: o.StatusReason, AllowedActions: []string{}, Revision: httpx.Revision(o.Version), UpdatedAt: o.UpdatedAt}
	if v.Published != nil {
		pv := toOfferVersion(v.Published, v.Own)
		d.PublishedVersion = &pv
	}
	for i := range v.Versions {
		d.Versions = append(d.Versions, toOfferVersion(&v.Versions[i], true))
	}
	if v.Own && o.Status != OfferWithdrawn {
		d.AllowedActions = []string{"add-version", "publish", "withdraw"}
	}
	return d
}

func (h *Handler) listOffers(c echo.Context) error {
	limit, err := httpx.IntQuery(c, "limit")
	if err != nil {
		return err
	}
	offset, err := httpx.IntQuery(c, "offset")
	if err != nil {
		return err
	}
	views, err := h.offers.List(c.Request().Context(), auth.Get(c), c.QueryParam("scope"), limit, offset)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(views, toOffer), nil)
}

func (h *Handler) getOffer(c echo.Context) error {
	v, err := h.offers.Get(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toOffer(v), v.Offer.Version)
}

func (h *Handler) createOffer(c echo.Context) error {
	var in OfferInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.offers.Create(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toOffer(v), v.Offer.Version)
}

func (h *Handler) addOfferVersion(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in OfferInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.offers.AddVersion(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toOffer(v), v.Offer.Version)
}

func (h *Handler) publishOffer(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		OfferVersionID string `json:"offerVersionId"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.offers.Publish(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.OfferVersionID)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toOffer(v), v.Offer.Version)
}

func (h *Handler) withdrawOffer(c echo.Context) error {
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
	v, err := h.offers.Withdraw(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Reason)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toOffer(v), v.Offer.Version)
}
