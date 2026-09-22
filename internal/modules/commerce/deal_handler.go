package commerce

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/httpx"
	"justixauto/internal/platform/money"
)

func party(c Company) counterpartyDTO {
	return counterpartyDTO{ID: c.ID, Name: c.Name, Country: c.Country}
}

type quotationDTO struct {
	ID        string      `json:"id"`
	Number    int         `json:"number"`
	Terms     Terms       `json:"terms"`
	Total     money.Money `json:"total"`
	Digest    string      `json:"digest"`
	CreatedAt time.Time   `json:"createdAt"`
}

type rfqDTO struct {
	ID             string          `json:"id"`
	Buyer          counterpartyDTO `json:"buyer"`
	Supplier       counterpartyDTO `json:"supplier"`
	OfferVersionID *string         `json:"offerVersionId"`
	Lines          []RFQLine       `json:"lines"`
	Status         RFQStatus       `json:"status"`
	StatusReason   string          `json:"statusReason"`
	Quotations     []quotationDTO  `json:"quotations"`
	AllowedActions []string        `json:"allowedActions"`
	Revision       string          `json:"revision"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

func rfqActions(x *RFQ, companyID string) []string {
	buyer := x.BuyerCompanyID == companyID
	switch {
	case buyer && x.Status == RFQDraft:
		return []string{"send", "cancel"}
	case buyer && x.Status == RFQSent:
		return []string{"cancel"}
	case buyer && x.Status == RFQNegotiating:
		return []string{"accept", "cancel"}
	case !buyer && (x.Status == RFQSent || x.Status == RFQNegotiating):
		return []string{"quote", "decline"}
	}
	return []string{}
}

func toRFQ(companyID string) func(*RFQView) rfqDTO {
	return func(v *RFQView) rfqDTO {
		x := v.RFQ
		d := rfqDTO{ID: x.ID, Buyer: party(v.Buyer), Supplier: party(v.Supplier), OfferVersionID: x.OfferVersionID,
			Lines: x.lines(), Status: x.Status, StatusReason: x.StatusReason, Quotations: []quotationDTO{},
			AllowedActions: rfqActions(&x, companyID), Revision: httpx.Revision(x.Version), UpdatedAt: x.UpdatedAt}
		for _, q := range v.Quotations {
			var t Terms
			_ = json.Unmarshal(q.Terms, &t)
			d.Quotations = append(d.Quotations, quotationDTO{ID: q.ID, Number: q.Number, Terms: t, Total: termsTotal(t),
				Digest: q.Digest, CreatedAt: q.CreatedAt})
		}
		return d
	}
}

type addendumDTO struct {
	ID             string      `json:"id"`
	Number         int         `json:"number"`
	Terms          Terms       `json:"terms"`
	Total          money.Money `json:"total"`
	Reason         string      `json:"reason"`
	ProposedBy     string      `json:"proposedBy"` // buyer | supplier
	Status         string      `json:"status"`
	DecisionReason string      `json:"decisionReason"`
	CreatedAt      time.Time   `json:"createdAt"`
	DecidedAt      *time.Time  `json:"decidedAt"`
}

type historyDTO struct {
	Type       string    `json:"type"`
	ActorID    string    `json:"actorId"`
	OccurredAt time.Time `json:"occurredAt"`
	Reason     string    `json:"reason"`
}

type orderDTO struct {
	ID             string          `json:"id"`
	Party          string          `json:"party"` // buyer | supplier (the caller's side)
	Buyer          counterpartyDTO `json:"buyer"`
	Supplier       counterpartyDTO `json:"supplier"`
	Source         string          `json:"source"`
	RFQID          *string         `json:"rfqId"`
	QuotationID    *string         `json:"quotationId"`
	OfferVersionID *string         `json:"offerVersionId"`
	Terms          Terms           `json:"terms"`
	Total          money.Money     `json:"total"`
	Status         OrderStatus     `json:"status"`
	StatusReason   string          `json:"statusReason"`
	Addenda        []addendumDTO   `json:"addenda"`
	History        []historyDTO    `json:"history,omitempty"`
	AllowedActions []string        `json:"allowedActions"`
	Revision       string          `json:"revision"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

func orderActions(v *OrderView, companyID string) []string {
	o := v.Order
	actions := []string{}
	var open *Addendum
	for i := range v.Addenda {
		if v.Addenda[i].Status == "proposed" {
			open = &v.Addenda[i]
		}
	}
	switch o.Status {
	case AwaitingSupplier:
		if o.Party(companyID) == "supplier" {
			actions = append(actions, "confirm", "reject")
		}
		actions = append(actions, "cancel")
	case OrderAccepted, OrderFulfilling:
		if o.Status == OrderAccepted {
			actions = append(actions, "cancel")
		}
		switch {
		case open == nil:
			actions = append(actions, "propose-addendum")
		case open.ProposedByCompany != companyID:
			actions = append(actions, "accept-addendum", "reject-addendum")
		}
	}
	return actions
}

func toOrder(companyID string) func(*OrderView) orderDTO {
	return func(v *OrderView) orderDTO {
		o := v.Order
		t := o.terms()
		d := orderDTO{ID: o.ID, Party: o.Party(companyID), Buyer: party(v.Buyer), Supplier: party(v.Supplier),
			Source: o.Source, RFQID: o.RFQID, QuotationID: o.QuotationID, OfferVersionID: o.OfferVersionID,
			Terms: t, Total: termsTotal(t), Status: o.Status, StatusReason: o.StatusReason, Addenda: []addendumDTO{},
			AllowedActions: orderActions(v, companyID), Revision: httpx.Revision(o.Version), UpdatedAt: o.UpdatedAt}
		for _, a := range v.Addenda {
			var at Terms
			_ = json.Unmarshal(a.Terms, &at)
			by := "supplier"
			if a.ProposedByCompany == o.BuyerCompanyID {
				by = "buyer"
			}
			d.Addenda = append(d.Addenda, addendumDTO{ID: a.ID, Number: a.Number, Terms: at, Total: termsTotal(at), Reason: a.Reason,
				ProposedBy: by, Status: a.Status, DecisionReason: a.DecisionReason, CreatedAt: a.CreatedAt, DecidedAt: a.DecidedAt})
		}
		for _, e := range v.Events {
			d.History = append(d.History, historyDTO{Type: e.EventType, ActorID: e.ActorUserID, OccurredAt: e.OccurredAt, Reason: e.Reason})
		}
		return d
	}
}

func (h *Handler) dealRoutes(c *echo.Group) {
	c.GET("/rfqs", h.listRFQs, auth.Require(PermRead))
	c.GET("/rfqs/:id", h.getRFQ, auth.Require(PermRead))
	c.POST("/rfqs", h.createRFQ, auth.Require(PermTrade))
	c.POST("/rfqs/:id/quotation-versions", h.quote, auth.Require(PermTrade))
	c.POST("/rfqs/:id/accept", h.acceptRFQ, auth.Require(PermTrade))
	c.POST("/rfqs/:id/:action", h.rfqAction, auth.Require(PermTrade))

	c.GET("/orders", h.listOrders, auth.Require(PermRead))
	c.GET("/orders/:id", h.getOrder, auth.Require(PermRead))
	c.POST("/orders", h.orderFromOffer, auth.Require(PermTrade))
	c.POST("/orders/:id/supplier-confirmations", h.confirmOrder(true), auth.Require(PermTrade))
	c.POST("/orders/:id/supplier-rejections", h.confirmOrder(false), auth.Require(PermTrade))
	c.POST("/orders/:id/cancellations", h.cancelOrder, auth.Require(PermTrade))
	c.POST("/orders/:id/addenda", h.proposeAddendum, auth.Require(PermTrade))
	c.POST("/orders/:id/addenda/:addendumId/accept", h.decideAddendum(true), auth.Require(PermTrade))
	c.POST("/orders/:id/addenda/:addendumId/reject", h.decideAddendum(false), auth.Require(PermTrade))
}

func paging(c echo.Context) (int, int, error) {
	limit, err := httpx.IntQuery(c, "limit")
	if err != nil {
		return 0, 0, err
	}
	offset, err := httpx.IntQuery(c, "offset")
	return limit, offset, err
}

func (h *Handler) listRFQs(c echo.Context) error {
	limit, offset, err := paging(c)
	if err != nil {
		return err
	}
	p := auth.Get(c)
	vs, err := h.deals.ListRFQs(c.Request().Context(), p, limit, offset)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(vs, toRFQ(p.CompanyID)), nil)
}

func (h *Handler) rfqResponse(c echo.Context, status int, x *RFQ) error {
	p := auth.Get(c)
	v, err := h.deals.rfqView(c.Request().Context(), x)
	if err != nil {
		return err
	}
	return httpx.Data(c, status, toRFQ(p.CompanyID)(v), x.Version)
}

func (h *Handler) getRFQ(c echo.Context) error {
	p := auth.Get(c)
	v, err := h.deals.GetRFQ(c.Request().Context(), p, c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toRFQ(p.CompanyID)(v), v.RFQ.Version)
}

func (h *Handler) createRFQ(c echo.Context) error {
	var in RFQInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	x, err := h.deals.CreateRFQ(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return h.rfqResponse(c, http.StatusCreated, x)
}

func (h *Handler) rfqAction(c echo.Context) error {
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
	x, err := h.deals.RFQAction(c.Request().Context(), auth.Get(c), c.Param("id"), expected, c.Param("action"), in.Reason)
	if err != nil {
		return err
	}
	return h.rfqResponse(c, http.StatusOK, x)
}

func (h *Handler) quote(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		Terms Terms `json:"terms"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	x, err := h.deals.Quote(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Terms)
	if err != nil {
		return err
	}
	return h.rfqResponse(c, http.StatusCreated, x)
}

func (h *Handler) acceptRFQ(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		QuotationVersionID string `json:"quotationVersionId"`
		QuotationDigest    string `json:"quotationDigest"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	o, err := h.deals.Accept(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.QuotationVersionID, in.QuotationDigest)
	if err != nil {
		return err
	}
	return h.orderResponse(c, http.StatusCreated, o)
}

func (h *Handler) orderResponse(c echo.Context, status int, o *Order) error {
	p := auth.Get(c)
	v, err := h.deals.orderView(c.Request().Context(), o, true)
	if err != nil {
		return err
	}
	return httpx.Data(c, status, toOrder(p.CompanyID)(v), o.Version)
}

func (h *Handler) listOrders(c echo.Context) error {
	limit, offset, err := paging(c)
	if err != nil {
		return err
	}
	p := auth.Get(c)
	vs, err := h.deals.ListOrders(c.Request().Context(), p, limit, offset)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(vs, toOrder(p.CompanyID)), nil)
}

func (h *Handler) getOrder(c echo.Context) error {
	p := auth.Get(c)
	v, err := h.deals.GetOrder(c.Request().Context(), p, c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toOrder(p.CompanyID)(v), v.Order.Version)
}

func (h *Handler) orderFromOffer(c echo.Context) error {
	var in DirectOrderInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	o, err := h.deals.OrderFromOffer(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return h.orderResponse(c, http.StatusCreated, o)
}

func (h *Handler) confirmOrder(confirm bool) echo.HandlerFunc {
	return func(c echo.Context) error {
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
		o, err := h.deals.ConfirmOrder(c.Request().Context(), auth.Get(c), c.Param("id"), expected, confirm, in.Reason)
		if err != nil {
			return err
		}
		return h.orderResponse(c, http.StatusOK, o)
	}
}

func (h *Handler) cancelOrder(c echo.Context) error {
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
	o, err := h.deals.CancelOrder(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Reason)
	if err != nil {
		return err
	}
	return h.orderResponse(c, http.StatusOK, o)
}

func (h *Handler) proposeAddendum(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		Terms  Terms  `json:"terms"`
		Reason string `json:"reason"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	o, err := h.deals.ProposeAddendum(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Terms, in.Reason)
	if err != nil {
		return err
	}
	return h.orderResponse(c, http.StatusCreated, o)
}

func (h *Handler) decideAddendum(accept bool) echo.HandlerFunc {
	return func(c echo.Context) error {
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
		o, err := h.deals.DecideAddendum(c.Request().Context(), auth.Get(c), c.Param("id"), c.Param("addendumId"), expected, accept, in.Reason)
		if err != nil {
			return err
		}
		return h.orderResponse(c, http.StatusOK, o)
	}
}
