package handler

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"justixauto/internal/modules/retail/model"
	"justixauto/internal/modules/retail/service"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/httpx"
	"justixauto/internal/pkg/money"
)

// Handler mounts retail's Echo routes.
type Handler struct {
	crm      *service.CRM
	listings *service.Listing
	deals    *service.Deal
}

// New builds the retail handler.
func New(crm *service.CRM, listings *service.Listing, deals *service.Deal) *Handler {
	return &Handler{crm: crm, listings: listings, deals: deals}
}

// assignLeadRequest assigns a lead to a user.
type assignLeadRequest struct {
	AssignedUserID string `json:"assignedUserId"`
}

// addContactRequest logs a customer contact on a lead.
type addContactRequest struct {
	Channel string `json:"channel"`
	Note    string `json:"note"`
}

// setStageRequest moves a lead to a new pipeline stage.
type setStageRequest struct {
	Stage  string `json:"stage"`
	Reason string `json:"reason"`
}

// updateListingRequest edits a marketplace listing's text and price.
type updateListingRequest struct {
	Text        string      `json:"text"`
	AskingPrice money.Money `json:"askingPrice"`
}

// recordContractRequest records a signed sale contract on a deal.
type recordContractRequest struct {
	SignedOn   string   `json:"signedOn"`
	Reference  string   `json:"reference"`
	BindingIDs []string `json:"bindingIds"`
}

// recordRegistrationRequest records vehicle registration on a deal.
type recordRegistrationRequest struct {
	RegisteredOn string `json:"registeredOn"`
	PlateNumber  string `json:"plateNumber"`
	Reference    string `json:"reference"`
}

// deliverRequest records vehicle delivery on a deal.
type deliverRequest struct {
	OccurredAt time.Time `json:"occurredAt"`
}

// cancelDealRequest cancels a reserved deal.
type cancelDealRequest struct {
	Reason string `json:"reason"`
}

// decideEvidenceRequest accepts or rejects submitted payment evidence.
type decideEvidenceRequest struct {
	Confirmation bool   `json:"confirmation"`
	Reason       string `json:"reason"`
}

func (h *Handler) Routes(g *echo.Group) {
	c := g.Group("", auth.RequireCompany())
	c.GET("/customers", h.listCustomers, auth.Require(model.PermRead))
	c.GET("/customers/:id", h.getCustomer, auth.Require(model.PermRead))
	c.POST("/customers", h.createCustomer, auth.Require(model.PermCRM))
	c.PATCH("/customers/:id", h.updateCustomer, auth.Require(model.PermCRM))

	c.GET("/leads", h.listLeads, auth.Require(model.PermRead))
	c.GET("/leads/:id", h.getLead, auth.Require(model.PermRead))
	c.POST("/leads", h.createLead, auth.Require(model.PermCRM))
	c.POST("/leads/:id/assign", h.assignLead, auth.Require(model.PermCRM))
	c.POST("/leads/:id/contacts", h.addContact, auth.Require(model.PermCRM))
	c.POST("/leads/:id/stage", h.setStage, auth.Require(model.PermCRM))

	c.GET("/tasks", h.listTasks, auth.Require(model.PermRead))
	c.POST("/tasks", h.createTask, auth.Require(model.PermCRM))
	c.POST("/tasks/:id/complete", h.completeTask, auth.Require(model.PermCRM))

	c.GET("/listings", h.listListings, auth.Require(model.PermRead))
	c.GET("/listings/:id", h.getListing, auth.Require(model.PermRead))
	c.POST("/listings", h.createListing, auth.Require(model.PermListings))
	c.PATCH("/listings/:id", h.updateListing, auth.Require(model.PermListings))
	c.POST("/listings/:id/publish", h.publishListing(true), auth.Require(model.PermListings))
	c.POST("/listings/:id/withdraw", h.publishListing(false), auth.Require(model.PermListings))

	c.GET("/deals", h.listDeals, auth.Require(model.PermRead))
	c.GET("/deals/:id", h.getDeal, auth.Require(model.PermRead))
	c.POST("/deals", h.createDeal, auth.Require(model.PermDeals))
	c.POST("/deals/:id/contract-records", h.recordContract, auth.Require(model.PermDeals))
	c.POST("/deals/:id/registration", h.recordRegistration, auth.Require(model.PermDeals))
	c.POST("/deals/:id/invoices", h.issueInvoice, auth.Require(model.PermDeals))
	c.POST("/deals/:id/deliveries", h.deliver, auth.Require(model.PermDeliver))
	c.POST("/deals/:id/cancel", h.cancelDeal, auth.Require(model.PermDeals))
	c.GET("/invoices/:id", h.getInvoice, auth.Require(model.PermRead))
	c.POST("/invoices/:id/evidence", h.submitEvidence, auth.Require(model.PermDeals))
	c.POST("/evidence/:id/accept", h.decideEvidence(true), auth.Require(model.PermPaymentsAccept))
	c.POST("/evidence/:id/reject", h.decideEvidence(false), auth.Require(model.PermPaymentsAccept))
}

func paging(c echo.Context) (int, int, error) {
	limit, err := httpx.IntQuery(c, "limit")
	if err != nil {
		return 0, 0, err
	}
	offset, err := httpx.IntQuery(c, "offset")
	return limit, offset, err
}

// listCustomers searches customers for the active company.
//
//	@Summary	List customers
//	@Tags		retail/crm
//	@Param		q			query		string	false	"name or phone search"
//	@Param		limit		query		int		false	"page size"
//	@Param		offset		query		int		false	"offset"
//	@Success	200			{object}	httpx.ListEnvelope[handler.customerDTO]
//	@Failure	401,403,422	{object}	httpx.ErrorBody
//	@Router		/retail/customers [get]
func (h *Handler) listCustomers(c echo.Context) error {
	limit, offset, err := paging(c)
	if err != nil {
		return err
	}
	cs, err := h.crm.Customers(c.Request().Context(), auth.Get(c), c.QueryParam("q"), limit, offset)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(cs, toCustomer), nil)
}

// getCustomer returns one customer.
//
//	@Summary	Get customer
//	@Tags		retail/crm
//	@Param		id			path		string	true	"customer ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.customerDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/retail/customers/{id} [get]
func (h *Handler) getCustomer(c echo.Context) error {
	cu, err := h.crm.Customer(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toCustomer(cu), cu.Version)
}

// createCustomer creates a customer for the active company.
//
//	@Summary	Create customer
//	@Tags		retail/crm
//	@Security	CSRF
//	@Param		body			body		service.CustomerInput	true	"customer"
//	@Success	201				{object}	httpx.DataEnvelope[handler.customerDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/retail/customers [post]
func (h *Handler) createCustomer(c echo.Context) error {
	var in service.CustomerInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	cu, err := h.crm.CreateCustomer(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toCustomer(cu), cu.Version)
}

// updateCustomer edits a customer.
//
//	@Summary	Update customer
//	@Tags		retail/crm
//	@Security	CSRF
//	@Param		id						path		string					true	"customer ID"
//	@Param		If-Match				header		string					true	"revision"
//	@Param		body					body		service.CustomerInput	true	"customer"
//	@Success	200						{object}	httpx.DataEnvelope[handler.customerDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/retail/customers/{id} [patch]
func (h *Handler) updateCustomer(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in service.CustomerInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	cu, err := h.crm.UpdateCustomer(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toCustomer(cu), cu.Version)
}

// listLeads searches leads for the active company.
//
//	@Summary	List leads
//	@Tags		retail/crm
//	@Param		stage		query		string	false	"lead stage"
//	@Param		limit		query		int		false	"page size"
//	@Param		offset		query		int		false	"offset"
//	@Success	200			{object}	httpx.ListEnvelope[handler.leadDTO]
//	@Failure	401,403,422	{object}	httpx.ErrorBody
//	@Router		/retail/leads [get]
func (h *Handler) listLeads(c echo.Context) error {
	limit, offset, err := paging(c)
	if err != nil {
		return err
	}
	ls, err := h.crm.Leads(c.Request().Context(), auth.Get(c), c.QueryParam("stage"), limit, offset)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(ls, toLead), nil)
}

func (h *Handler) leadResponse(c echo.Context, status int, id string) error {
	v, err := h.crm.Lead(c.Request().Context(), auth.Get(c), id)
	if err != nil {
		return err
	}
	return httpx.Data(c, status, toLeadView(v), v.Lead.Version)
}

// getLead returns one lead with its contacts and history.
//
//	@Summary	Get lead
//	@Tags		retail/crm
//	@Param		id			path		string	true	"lead ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.leadDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/retail/leads/{id} [get]
func (h *Handler) getLead(c echo.Context) error {
	return h.leadResponse(c, http.StatusOK, c.Param("id"))
}

// createLead creates a lead for the active company.
//
//	@Summary	Create lead
//	@Tags		retail/crm
//	@Security	CSRF
//	@Param		body			body		service.LeadInput	true	"lead"
//	@Success	201				{object}	httpx.DataEnvelope[handler.leadDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/retail/leads [post]
func (h *Handler) createLead(c echo.Context) error {
	var in service.LeadInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	l, err := h.crm.CreateLead(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return h.leadResponse(c, http.StatusCreated, l.ID)
}

// assignLead assigns a lead to a user.
//
//	@Summary	Assign lead
//	@Tags		retail/crm
//	@Security	CSRF
//	@Param		id							path		string				true	"lead ID"
//	@Param		If-Match					header		string				true	"revision"
//	@Param		body						body		assignLeadRequest	true	"assignment"
//	@Success	200							{object}	httpx.DataEnvelope[handler.leadDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/retail/leads/{id}/assign [post]
func (h *Handler) assignLead(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in assignLeadRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	l, err := h.crm.Assign(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.AssignedUserID)
	if err != nil {
		return err
	}
	return h.leadResponse(c, http.StatusOK, l.ID)
}

// addContact logs a customer contact on a lead.
//
//	@Summary	Add lead contact
//	@Tags		retail/crm
//	@Security	CSRF
//	@Param		id					path		string				true	"lead ID"
//	@Param		body				body		addContactRequest	true	"contact"
//	@Success	201					{object}	httpx.DataEnvelope[handler.leadDTO]
//	@Failure	401,403,404,409,422	{object}	httpx.ErrorBody
//	@Router		/retail/leads/{id}/contacts [post]
func (h *Handler) addContact(c echo.Context) error {
	var in addContactRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	l, err := h.crm.AddContact(c.Request().Context(), auth.Get(c), c.Param("id"), in.Channel, in.Note)
	if err != nil {
		return err
	}
	return h.leadResponse(c, http.StatusCreated, l.ID)
}

// setStage moves a lead to a new pipeline stage.
//
//	@Summary	Set lead stage
//	@Tags		retail/crm
//	@Security	CSRF
//	@Param		id							path		string			true	"lead ID"
//	@Param		If-Match					header		string			true	"revision"
//	@Param		body						body		setStageRequest	true	"stage"
//	@Success	200							{object}	httpx.DataEnvelope[handler.leadDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/retail/leads/{id}/stage [post]
func (h *Handler) setStage(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in setStageRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	l, err := h.crm.SetStage(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Stage, in.Reason)
	if err != nil {
		return err
	}
	return h.leadResponse(c, http.StatusOK, l.ID)
}

// listTasks searches tasks for the active company.
//
//	@Summary	List tasks
//	@Tags		retail/crm
//	@Param		owner		query		string	false	"owner user ID"
//	@Param		status		query		string	false	"task status"
//	@Param		limit		query		int		false	"page size"
//	@Param		offset		query		int		false	"offset"
//	@Success	200			{object}	httpx.ListEnvelope[handler.taskDTO]
//	@Failure	401,403,422	{object}	httpx.ErrorBody
//	@Router		/retail/tasks [get]
func (h *Handler) listTasks(c echo.Context) error {
	limit, offset, err := paging(c)
	if err != nil {
		return err
	}
	ts, err := h.crm.Tasks(c.Request().Context(), auth.Get(c), c.QueryParam("owner"), c.QueryParam("status"), limit, offset)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(ts, toTask), nil)
}

// createTask creates a follow-up task for the active company.
//
//	@Summary	Create task
//	@Tags		retail/crm
//	@Security	CSRF
//	@Param		body			body		service.TaskInput	true	"task"
//	@Success	201				{object}	httpx.DataEnvelope[handler.taskDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/retail/tasks [post]
func (h *Handler) createTask(c echo.Context) error {
	var in service.TaskInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	t, err := h.crm.CreateTask(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toTask(t), t.Version)
}

// completeTask marks a task done.
//
//	@Summary	Complete task
//	@Tags		retail/crm
//	@Security	CSRF
//	@Param		id						path		string	true	"task ID"
//	@Param		If-Match				header		string	true	"revision"
//	@Success	200						{object}	httpx.DataEnvelope[handler.taskDTO]
//	@Failure	401,403,404,409,412,428	{object}	httpx.ErrorBody
//	@Router		/retail/tasks/{id}/complete [post]
func (h *Handler) completeTask(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	t, err := h.crm.CompleteTask(c.Request().Context(), auth.Get(c), c.Param("id"), expected)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toTask(t), t.Version)
}

// listListings searches marketplace listings for the active company.
//
//	@Summary	List listings
//	@Tags		retail/listings
//	@Param		status		query		string	false	"listing status"
//	@Param		limit		query		int		false	"page size"
//	@Param		offset		query		int		false	"offset"
//	@Success	200			{object}	httpx.ListEnvelope[handler.listingDTO]
//	@Failure	401,403,422	{object}	httpx.ErrorBody
//	@Router		/retail/listings [get]
func (h *Handler) listListings(c echo.Context) error {
	limit, offset, err := paging(c)
	if err != nil {
		return err
	}
	ls, err := h.listings.List(c.Request().Context(), auth.Get(c), c.QueryParam("status"), limit, offset)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(ls, toListing), nil)
}

// getListing returns one listing.
//
//	@Summary	Get listing
//	@Tags		retail/listings
//	@Param		id			path		string	true	"listing ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.listingDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/retail/listings/{id} [get]
func (h *Handler) getListing(c echo.Context) error {
	l, err := h.listings.Get(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toListing(l), l.Version)
}

// createListing creates a marketplace listing for a vehicle.
//
//	@Summary	Create listing
//	@Tags		retail/listings
//	@Security	CSRF
//	@Param		body			body		service.ListingInput	true	"listing"
//	@Success	201				{object}	httpx.DataEnvelope[handler.listingDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/retail/listings [post]
func (h *Handler) createListing(c echo.Context) error {
	var in service.ListingInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	l, err := h.listings.Create(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toListing(l), l.Version)
}

// updateListing edits a listing's text and asking price.
//
//	@Summary	Update listing
//	@Tags		retail/listings
//	@Security	CSRF
//	@Param		id						path		string					true	"listing ID"
//	@Param		If-Match				header		string					true	"revision"
//	@Param		body					body		updateListingRequest	true	"listing"
//	@Success	200						{object}	httpx.DataEnvelope[handler.listingDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/retail/listings/{id} [patch]
func (h *Handler) updateListing(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in updateListingRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	l, err := h.listings.Update(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Text, in.AskingPrice)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toListing(l), l.Version)
}

// publishListing publishes or withdraws a listing.
//
//	@Summary	Publish or withdraw listing
//	@Tags		retail/listings
//	@Security	CSRF
//	@Param		id						path		string	true	"listing ID"
//	@Param		If-Match				header		string	true	"revision"
//	@Success	200						{object}	httpx.DataEnvelope[handler.listingDTO]
//	@Failure	401,403,404,409,412,428	{object}	httpx.ErrorBody
//	@Router		/retail/listings/{id}/publish [post]
//	@Router		/retail/listings/{id}/withdraw [post]
func (h *Handler) publishListing(publish bool) echo.HandlerFunc {
	return func(c echo.Context) error {
		expected, err := httpx.IfMatch(c)
		if err != nil {
			return err
		}
		l, err := h.listings.SetPublished(c.Request().Context(), auth.Get(c), c.Param("id"), expected, publish)
		if err != nil {
			return err
		}
		return httpx.Data(c, http.StatusOK, toListing(l), l.Version)
	}
}

// listDeals searches deals for the active company.
//
//	@Summary	List deals
//	@Tags		retail/deals
//	@Param		status		query		string	false	"deal status"
//	@Param		limit		query		int		false	"page size"
//	@Param		offset		query		int		false	"offset"
//	@Success	200			{object}	httpx.ListEnvelope[handler.dealDTO]
//	@Failure	401,403,422	{object}	httpx.ErrorBody
//	@Router		/retail/deals [get]
func (h *Handler) listDeals(c echo.Context) error {
	limit, offset, err := paging(c)
	if err != nil {
		return err
	}
	ds, err := h.deals.List(c.Request().Context(), auth.Get(c), c.QueryParam("status"), limit, offset)
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(ds, toDeal(false)), nil)
}

func (h *Handler) dealResponse(c echo.Context, status int, id string) error {
	v, err := h.deals.Get(c.Request().Context(), auth.Get(c), id)
	if err != nil {
		return err
	}
	return httpx.Data(c, status, toDeal(true)(v), v.Deal.Version)
}

// getDeal returns one deal with its invoices, checklist and history.
//
//	@Summary	Get deal
//	@Tags		retail/deals
//	@Param		id			path		string	true	"deal ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.dealDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/retail/deals/{id} [get]
func (h *Handler) getDeal(c echo.Context) error {
	return h.dealResponse(c, http.StatusOK, c.Param("id"))
}

// createDeal reserves a vehicle for a customer as a deal.
//
//	@Summary	Create deal
//	@Tags		retail/deals
//	@Security	CSRF
//	@Param		body			body		service.DealInput	true	"deal"
//	@Success	201				{object}	httpx.DataEnvelope[handler.dealDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/retail/deals [post]
func (h *Handler) createDeal(c echo.Context) error {
	var in service.DealInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	d, err := h.deals.Create(c.Request().Context(), auth.Get(c), in)
	if err != nil {
		return err
	}
	return h.dealResponse(c, http.StatusCreated, d.ID)
}

// recordContract records a signed sale contract on a deal.
//
//	@Summary	Record deal contract
//	@Tags		retail/deals
//	@Security	CSRF
//	@Param		id							path		string					true	"deal ID"
//	@Param		If-Match					header		string					true	"revision"
//	@Param		body						body		recordContractRequest	true	"contract"
//	@Success	200							{object}	httpx.DataEnvelope[handler.dealDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/retail/deals/{id}/contract-records [post]
func (h *Handler) recordContract(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in recordContractRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	d, err := h.deals.RecordContract(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.SignedOn, in.Reference, in.BindingIDs)
	if err != nil {
		return err
	}
	return h.dealResponse(c, http.StatusOK, d.ID)
}

// recordRegistration records vehicle registration on a deal.
//
//	@Summary	Record deal registration
//	@Tags		retail/deals
//	@Security	CSRF
//	@Param		id							path		string						true	"deal ID"
//	@Param		If-Match					header		string						true	"revision"
//	@Param		body						body		recordRegistrationRequest	true	"registration"
//	@Success	200							{object}	httpx.DataEnvelope[handler.dealDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/retail/deals/{id}/registration [post]
func (h *Handler) recordRegistration(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in recordRegistrationRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	d, err := h.deals.RecordRegistration(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.RegisteredOn, in.PlateNumber, in.Reference)
	if err != nil {
		return err
	}
	return h.dealResponse(c, http.StatusOK, d.ID)
}

// issueInvoice issues an invoice on a deal.
//
//	@Summary	Issue deal invoice
//	@Tags		retail/deals
//	@Security	CSRF
//	@Param		id							path		string					true	"deal ID"
//	@Param		If-Match					header		string					true	"revision"
//	@Param		body						body		service.InvoiceInput	true	"invoice"
//	@Success	201							{object}	httpx.DataEnvelope[handler.invoiceDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/retail/deals/{id}/invoices [post]
func (h *Handler) issueInvoice(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in service.InvoiceInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.deals.IssueInvoice(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toInvoice(v), v.Invoice.Version)
}

// deliver records vehicle delivery on a deal.
//
//	@Summary	Deliver deal
//	@Tags		retail/deals
//	@Security	CSRF
//	@Param		id							path		string			true	"deal ID"
//	@Param		If-Match					header		string			true	"revision"
//	@Param		body						body		deliverRequest	true	"delivery"
//	@Success	200							{object}	httpx.DataEnvelope[handler.dealDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/retail/deals/{id}/deliveries [post]
func (h *Handler) deliver(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in deliverRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	d, err := h.deals.Deliver(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.OccurredAt)
	if err != nil {
		return err
	}
	return h.dealResponse(c, http.StatusOK, d.ID)
}

// cancelDeal cancels a reserved deal.
//
//	@Summary	Cancel deal
//	@Tags		retail/deals
//	@Security	CSRF
//	@Param		id							path		string				true	"deal ID"
//	@Param		If-Match					header		string				true	"revision"
//	@Param		body						body		cancelDealRequest	true	"cancellation"
//	@Success	200							{object}	httpx.DataEnvelope[handler.dealDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/retail/deals/{id}/cancel [post]
func (h *Handler) cancelDeal(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in cancelDealRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	d, err := h.deals.Cancel(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Reason)
	if err != nil {
		return err
	}
	return h.dealResponse(c, http.StatusOK, d.ID)
}

// getInvoice returns one invoice with its payment evidence.
//
//	@Summary	Get invoice
//	@Tags		retail/deals
//	@Param		id			path		string	true	"invoice ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.invoiceDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/retail/invoices/{id} [get]
func (h *Handler) getInvoice(c echo.Context) error {
	v, err := h.deals.Invoice(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toInvoice(v), v.Invoice.Version)
}

// submitEvidence submits payment evidence for an invoice.
//
//	@Summary	Submit payment evidence
//	@Tags		retail/deals
//	@Security	CSRF
//	@Param		id					path		string					true	"invoice ID"
//	@Param		body				body		service.EvidenceInput	true	"evidence"
//	@Success	201					{object}	httpx.DataEnvelope[handler.invoiceDTO]
//	@Failure	401,403,404,409,422	{object}	httpx.ErrorBody
//	@Router		/retail/invoices/{id}/evidence [post]
func (h *Handler) submitEvidence(c echo.Context) error {
	var in service.EvidenceInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.deals.SubmitEvidence(c.Request().Context(), auth.Get(c), c.Param("id"), in)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, toInvoice(v), v.Invoice.Version)
}

// decideEvidence accepts or rejects submitted payment evidence.
//
//	@Summary	Decide payment evidence
//	@Tags		retail/deals
//	@Security	CSRF
//	@Param		id							path		string					true	"evidence ID"
//	@Param		If-Match					header		string					true	"revision"
//	@Param		body						body		decideEvidenceRequest	true	"decision"
//	@Success	200							{object}	httpx.DataEnvelope[handler.invoiceDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/retail/evidence/{id}/accept [post]
//	@Router		/retail/evidence/{id}/reject [post]
func (h *Handler) decideEvidence(accept bool) echo.HandlerFunc {
	return func(c echo.Context) error {
		expected, err := httpx.IfMatch(c)
		if err != nil {
			return err
		}
		var in decideEvidenceRequest
		if err := httpx.Bind(c, &in); err != nil {
			return err
		}
		v, err := h.deals.DecideEvidence(c.Request().Context(), auth.Get(c), c.Param("id"), expected, accept, in.Confirmation, in.Reason)
		if err != nil {
			return err
		}
		return httpx.Data(c, http.StatusOK, toInvoice(v), v.Invoice.Version)
	}
}
