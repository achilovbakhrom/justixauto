package commerce

import (
	"encoding/json"
	"net/http"
	"slices"
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

type allocationDTO struct {
	OrderLineID string  `json:"orderLineId"`
	VehicleID   string  `json:"vehicleId"`
	VIN         string  `json:"vin"`
	Status      string  `json:"status"`
	ShipmentID  *string `json:"shipmentId"`
}

type shipmentRef struct {
	ID     string `json:"id"`
	Route  string `json:"route"`
	Status string `json:"status"`
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
	Allocations    []allocationDTO `json:"allocations"`
	Shipments      []shipmentRef   `json:"shipments"`
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
		if o.Party(companyID) == "supplier" {
			actions = append(actions, "allocate")
			if slices.ContainsFunc(v.Allocations, func(a Allocation) bool { return a.Status == "allocated" }) {
				actions = append(actions, "ship")
			}
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
		d.Allocations, d.Shipments = []allocationDTO{}, []shipmentRef{}
		for _, a := range v.Allocations {
			d.Allocations = append(d.Allocations, allocationDTO{OrderLineID: a.LineID, VehicleID: a.VehicleID, VIN: a.VIN, Status: a.Status, ShipmentID: a.ShipmentID})
		}
		for _, sh := range v.Shipments {
			d.Shipments = append(d.Shipments, shipmentRef{ID: sh.ID, Route: sh.Route, Status: sh.Status})
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
	c.POST("/orders/:id/allocations", h.allocate, auth.Require(PermTrade))
	c.POST("/orders/:id/shipments", h.ship, auth.Require(PermTrade))
	c.GET("/shipments/:id", h.getShipment, auth.Require(PermRead))
	c.POST("/shipments/:id/milestones", h.addMilestone, auth.Require(PermTrade))
	c.POST("/shipments/:id/receipt-decisions", h.decideReceipt, auth.Require(PermTrade))
}

// rfqActionRequest decides a generic RFQ transition (path action).
type rfqActionRequest struct {
	Reason string `json:"reason"`
}

// quoteRequest adds a quotation version to an RFQ.
type quoteRequest struct {
	Terms Terms `json:"terms"`
}

// acceptRFQRequest accepts a quotation and creates the order.
type acceptRFQRequest struct {
	QuotationVersionID string `json:"quotationVersionId"`
	QuotationDigest    string `json:"quotationDigest"`
}

// orderConfirmRequest confirms or rejects an order as the supplier.
type orderConfirmRequest struct {
	Reason string `json:"reason"`
}

// orderCancelRequest cancels an order.
type orderCancelRequest struct {
	Reason string `json:"reason"`
}

// addendumProposeRequest proposes a change to an accepted order's terms.
type addendumProposeRequest struct {
	Terms  Terms  `json:"terms"`
	Reason string `json:"reason"`
}

// addendumDecisionRequest accepts or rejects a proposed addendum.
type addendumDecisionRequest struct {
	Reason string `json:"reason"`
}

// allocateRequest allocates vehicle units to an order's lines.
type allocateRequest struct {
	Items []AllocationItem `json:"items"`
}

type milestoneDTO struct {
	Type       string    `json:"milestoneType"`
	OccurredAt time.Time `json:"occurredAt"`
	Location   string    `json:"location"`
	Note       string    `json:"note"`
	RecordedBy string    `json:"recordedBy"`
}

type shipmentDTO struct {
	ID         string          `json:"id"`
	OrderID    string          `json:"orderId"`
	Route      string          `json:"route"`
	Status     string          `json:"status"`
	Vehicles   []allocationDTO `json:"vehicles"`
	Milestones []milestoneDTO  `json:"milestones"`
	Revision   string          `json:"revision"`
}

func toShipment(v *ShipmentView) shipmentDTO {
	d := shipmentDTO{ID: v.Shipment.ID, OrderID: v.Shipment.OrderID, Route: v.Shipment.Route, Status: v.Shipment.Status,
		Vehicles: []allocationDTO{}, Milestones: []milestoneDTO{}, Revision: httpx.Revision(v.Shipment.Version)}
	for _, a := range v.Vehicles {
		d.Vehicles = append(d.Vehicles, allocationDTO{OrderLineID: a.LineID, VehicleID: a.VehicleID, VIN: a.VIN, Status: a.Status, ShipmentID: a.ShipmentID})
	}
	for _, m := range v.Milestones {
		d.Milestones = append(d.Milestones, milestoneDTO{Type: m.MilestoneType, OccurredAt: m.OccurredAt, Location: m.Location, Note: m.Note, RecordedBy: m.RecordedBy})
	}
	return d
}

// allocate allocates vehicle units to an accepted order's lines.
//
//	@Summary	Allocate order vehicles
//	@Tags		commerce/orders
//	@Security	CSRF
//	@Param		id							path		string			true	"order ID"
//	@Param		If-Match					header		string			true	"revision"
//	@Param		body						body		allocateRequest	true	"allocation items"
//	@Param		Idempotency-Key				header		string			true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[commerce.orderDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/orders/{id}/allocations [post]
func (h *Handler) allocate(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in allocateRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	o, err := h.fulfilment.Allocate(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Items)
	if err != nil {
		return err
	}
	return h.orderResponse(c, http.StatusOK, o)
}

// ship creates a shipment for an order's allocated vehicles.
//
//	@Summary	Ship order
//	@Tags		commerce/orders
//	@Security	CSRF
//	@Param		id							path		string			true	"order ID"
//	@Param		If-Match					header		string			true	"revision"
//	@Param		body						body		ShipmentInput	true	"shipment"
//	@Param		Idempotency-Key				header		string			true	"retry key"
//	@Success	201							{object}	httpx.DataEnvelope[commerce.shipmentDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/orders/{id}/shipments [post]
func (h *Handler) ship(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in ShipmentInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	sh, err := h.fulfilment.Ship(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	v, err := h.fulfilment.GetShipment(c.Request().Context(), auth.Get(c), sh.ID)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toShipment(v), v.Shipment.Version)
}

// getShipment returns one shipment.
//
//	@Summary	Get shipment
//	@Tags		commerce/orders
//	@Param		id			path		string	true	"shipment ID"
//	@Success	200			{object}	httpx.DataEnvelope[commerce.shipmentDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/commerce/shipments/{id} [get]
func (h *Handler) getShipment(c echo.Context) error {
	v, err := h.fulfilment.GetShipment(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toShipment(v), v.Shipment.Version)
}

// addMilestone records a tracking milestone for a shipment.
//
//	@Summary	Add shipment milestone
//	@Tags		commerce/orders
//	@Security	CSRF
//	@Param		id				path		string			true	"shipment ID"
//	@Param		Idempotency-Key	header		string			true	"retry key"
//	@Param		body			body		MilestoneInput	true	"milestone"
//	@Success	201				{object}	httpx.DataEnvelope[commerce.shipmentDTO]
//	@Failure	401,403,404,422	{object}	httpx.ErrorBody
//	@Router		/commerce/shipments/{id}/milestones [post]
func (h *Handler) addMilestone(c echo.Context) error {
	var in MilestoneInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.fulfilment.AddMilestone(c.Request().Context(), auth.Get(c), c.Param("id"), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toShipment(v), v.Shipment.Version)
}

// decideReceipt records the buyer's decision on a received shipment.
//
//	@Summary	Decide shipment receipt
//	@Tags		commerce/orders
//	@Security	CSRF
//	@Param		id							path		string			true	"shipment ID"
//	@Param		If-Match					header		string			true	"revision"
//	@Param		body						body		ReceiptDecision	true	"receipt decision"
//	@Param		Idempotency-Key				header		string			true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[commerce.shipmentDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/shipments/{id}/receipt-decisions [post]
func (h *Handler) decideReceipt(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in ReceiptDecision
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.fulfilment.DecideReceipt(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toShipment(v), v.Shipment.Version)
}

func paging(c echo.Context) (int, int, error) {
	limit, err := httpx.IntQuery(c, "limit")
	if err != nil {
		return 0, 0, err
	}
	offset, err := httpx.IntQuery(c, "offset")
	return limit, offset, err
}

// listRFQs lists RFQs for the active company.
//
//	@Summary	List RFQs
//	@Tags		commerce/rfqs
//	@Param		limit		query		int	false	"page size"
//	@Param		offset		query		int	false	"offset"
//	@Success	200			{object}	httpx.ListEnvelope[commerce.rfqDTO]
//	@Failure	401,403,422	{object}	httpx.ErrorBody
//	@Router		/commerce/rfqs [get]
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

// getRFQ returns one RFQ with its quotations.
//
//	@Summary	Get RFQ
//	@Tags		commerce/rfqs
//	@Param		id			path		string	true	"RFQ ID"
//	@Success	200			{object}	httpx.DataEnvelope[commerce.rfqDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/commerce/rfqs/{id} [get]
func (h *Handler) getRFQ(c echo.Context) error {
	p := auth.Get(c)
	v, err := h.deals.GetRFQ(c.Request().Context(), p, c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toRFQ(p.CompanyID)(v), v.RFQ.Version)
}

// createRFQ creates a request for quotation to a supplier.
//
//	@Summary	Create RFQ
//	@Tags		commerce/rfqs
//	@Security	CSRF
//	@Param		Idempotency-Key	header		string		true	"retry key"
//	@Param		body			body		RFQInput	true	"RFQ"
//	@Success	201				{object}	httpx.DataEnvelope[commerce.rfqDTO]
//	@Failure	401,403,404,422	{object}	httpx.ErrorBody
//	@Router		/commerce/rfqs [post]
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

// rfqAction decides a generic RFQ transition (path action, e.g. cancel/decline).
//
//	@Summary	Decide RFQ
//	@Tags		commerce/rfqs
//	@Security	CSRF
//	@Param		id							path		string				true	"RFQ ID"
//	@Param		action						path		string				true	"decision action"
//	@Param		If-Match					header		string				true	"revision"
//	@Param		body						body		rfqActionRequest	true	"reason"
//	@Param		Idempotency-Key				header		string				true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[commerce.rfqDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/rfqs/{id}/{action} [post]
func (h *Handler) rfqAction(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in rfqActionRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	x, err := h.deals.RFQAction(c.Request().Context(), auth.Get(c), c.Param("id"), expected, c.Param("action"), in.Reason)
	if err != nil {
		return err
	}
	return h.rfqResponse(c, http.StatusOK, x)
}

// quote adds a quotation version to an RFQ.
//
//	@Summary	Quote RFQ
//	@Tags		commerce/rfqs
//	@Security	CSRF
//	@Param		id							path		string			true	"RFQ ID"
//	@Param		If-Match					header		string			true	"revision"
//	@Param		body						body		quoteRequest	true	"quotation terms"
//	@Param		Idempotency-Key				header		string			true	"retry key"
//	@Success	201							{object}	httpx.DataEnvelope[commerce.rfqDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/rfqs/{id}/quotation-versions [post]
func (h *Handler) quote(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in quoteRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	x, err := h.deals.Quote(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Terms)
	if err != nil {
		return err
	}
	return h.rfqResponse(c, http.StatusCreated, x)
}

// acceptRFQ accepts a quotation and creates the resulting order.
//
//	@Summary	Accept RFQ quotation
//	@Tags		commerce/rfqs
//	@Security	CSRF
//	@Param		id							path		string				true	"RFQ ID"
//	@Param		If-Match					header		string				true	"revision"
//	@Param		body						body		acceptRFQRequest	true	"accepted quotation"
//	@Param		Idempotency-Key				header		string				true	"retry key"
//	@Success	201							{object}	httpx.DataEnvelope[commerce.orderDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/rfqs/{id}/accept [post]
func (h *Handler) acceptRFQ(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in acceptRFQRequest
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

// listOrders lists orders for the active company.
//
//	@Summary	List orders
//	@Tags		commerce/orders
//	@Param		limit		query		int	false	"page size"
//	@Param		offset		query		int	false	"offset"
//	@Success	200			{object}	httpx.ListEnvelope[commerce.orderDTO]
//	@Failure	401,403,422	{object}	httpx.ErrorBody
//	@Router		/commerce/orders [get]
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

// getOrder returns one order.
//
//	@Summary	Get order
//	@Tags		commerce/orders
//	@Param		id			path		string	true	"order ID"
//	@Success	200			{object}	httpx.DataEnvelope[commerce.orderDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/commerce/orders/{id} [get]
func (h *Handler) getOrder(c echo.Context) error {
	p := auth.Get(c)
	v, err := h.deals.GetOrder(c.Request().Context(), p, c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toOrder(p.CompanyID)(v), v.Order.Version)
}

// orderFromOffer creates an order directly from a published offer version.
//
//	@Summary	Create order from offer
//	@Tags		commerce/orders
//	@Security	CSRF
//	@Param		Idempotency-Key		header		string				true	"retry key"
//	@Param		body				body		DirectOrderInput	true	"order"
//	@Success	201					{object}	httpx.DataEnvelope[commerce.orderDTO]
//	@Failure	401,403,404,409,422	{object}	httpx.ErrorBody
//	@Router		/commerce/orders [post]
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

// confirmOrder confirms or rejects an order as the supplier.
//
//	@Summary	Confirm or reject order
//	@Tags		commerce/orders
//	@Security	CSRF
//	@Param		id							path		string				true	"order ID"
//	@Param		If-Match					header		string				true	"revision"
//	@Param		body						body		orderConfirmRequest	true	"reason"
//	@Param		Idempotency-Key				header		string				true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[commerce.orderDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/orders/{id}/supplier-confirmations [post]
//	@Router		/commerce/orders/{id}/supplier-rejections [post]
func (h *Handler) confirmOrder(confirm bool) echo.HandlerFunc {
	return func(c echo.Context) error {
		expected, err := httpx.IfMatch(c)
		if err != nil {
			return err
		}
		var in orderConfirmRequest
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

// cancelOrder cancels an order.
//
//	@Summary	Cancel order
//	@Tags		commerce/orders
//	@Security	CSRF
//	@Param		id							path		string				true	"order ID"
//	@Param		If-Match					header		string				true	"revision"
//	@Param		body						body		orderCancelRequest	true	"reason"
//	@Param		Idempotency-Key				header		string				true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[commerce.orderDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/orders/{id}/cancellations [post]
func (h *Handler) cancelOrder(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in orderCancelRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	o, err := h.deals.CancelOrder(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Reason)
	if err != nil {
		return err
	}
	return h.orderResponse(c, http.StatusOK, o)
}

// proposeAddendum proposes a change to an accepted order's terms.
//
//	@Summary	Propose order addendum
//	@Tags		commerce/orders
//	@Security	CSRF
//	@Param		id							path		string					true	"order ID"
//	@Param		If-Match					header		string					true	"revision"
//	@Param		body						body		addendumProposeRequest	true	"addendum"
//	@Param		Idempotency-Key				header		string					true	"retry key"
//	@Success	201							{object}	httpx.DataEnvelope[commerce.orderDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/orders/{id}/addenda [post]
func (h *Handler) proposeAddendum(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in addendumProposeRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	o, err := h.deals.ProposeAddendum(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Terms, in.Reason)
	if err != nil {
		return err
	}
	return h.orderResponse(c, http.StatusCreated, o)
}

// decideAddendum accepts or rejects a proposed addendum.
//
//	@Summary	Decide order addendum
//	@Tags		commerce/orders
//	@Security	CSRF
//	@Param		id							path		string					true	"order ID"
//	@Param		addendumId					path		string					true	"addendum ID"
//	@Param		If-Match					header		string					true	"revision"
//	@Param		body						body		addendumDecisionRequest	true	"reason"
//	@Param		Idempotency-Key				header		string					true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[commerce.orderDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/orders/{id}/addenda/{addendumId}/accept [post]
//	@Router		/commerce/orders/{id}/addenda/{addendumId}/reject [post]
func (h *Handler) decideAddendum(accept bool) echo.HandlerFunc {
	return func(c echo.Context) error {
		expected, err := httpx.IfMatch(c)
		if err != nil {
			return err
		}
		var in addendumDecisionRequest
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
