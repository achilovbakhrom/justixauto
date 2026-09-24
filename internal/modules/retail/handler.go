package retail

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/httpx"
	"justixauto/internal/pkg/money"
)

// ---- DTOs ----

type customerDTO struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	Phone       string    `json:"phone"`
	Revision    string    `json:"revision"`
	CreatedAt   time.Time `json:"createdAt"`
}

func toCustomer(c *Customer) customerDTO {
	return customerDTO{ID: c.ID, DisplayName: c.DisplayName, Phone: c.Phone, Revision: httpx.Revision(c.Version), CreatedAt: c.CreatedAt}
}

type contactDTO struct {
	Channel    string    `json:"channel"`
	Note       string    `json:"note"`
	ActorID    string    `json:"actorId"`
	OccurredAt time.Time `json:"occurredAt"`
}

type historyDTO struct {
	Type       string          `json:"type"`
	ActorID    string          `json:"actorId"`
	OccurredAt time.Time       `json:"occurredAt"`
	Reason     string          `json:"reason"`
	Details    json.RawMessage `json:"details"`
}

func toHistory(es []Event) []historyDTO {
	out := []historyDTO{}
	for _, e := range es {
		out = append(out, historyDTO{Type: e.EventType, ActorID: e.ActorUserID, OccurredAt: e.OccurredAt, Reason: e.Reason, Details: e.Details})
	}
	return out
}

type leadDTO struct {
	ID             string       `json:"id"`
	CustomerID     string       `json:"customerId"`
	Customer       *customerDTO `json:"customer,omitempty"`
	BranchID       string       `json:"branchId"`
	Source         string       `json:"source"`
	Stage          string       `json:"stage"`
	AssignedUserID *string      `json:"assignedUserId"`
	LostReason     string       `json:"lostReason"`
	DealID         *string      `json:"dealId"`
	Contacts       []contactDTO `json:"contacts,omitempty"`
	History        []historyDTO `json:"history,omitempty"`
	Revision       string       `json:"revision"`
	UpdatedAt      time.Time    `json:"updatedAt"`
}

func toLead(l *Lead) leadDTO {
	return leadDTO{
		ID: l.ID, CustomerID: l.CustomerID, BranchID: l.BranchID, Source: l.Source, Stage: l.Stage,
		AssignedUserID: l.AssignedUserID, LostReason: l.LostReason, DealID: l.DealID, Revision: httpx.Revision(l.Version), UpdatedAt: l.UpdatedAt,
	}
}

func toLeadView(v *LeadView) leadDTO {
	d := toLead(&v.Lead)
	c := toCustomer(&v.Customer)
	d.Customer, d.Contacts, d.History = &c, []contactDTO{}, toHistory(v.History)
	for _, ct := range v.Contacts {
		d.Contacts = append(d.Contacts, contactDTO{Channel: ct.Channel, Note: ct.Note, ActorID: ct.ActorID, OccurredAt: ct.OccurredAt})
	}
	return d
}

type taskDTO struct {
	ID          string     `json:"id"`
	CustomerID  string     `json:"customerId"`
	LeadID      *string    `json:"leadId"`
	DealID      *string    `json:"dealId"`
	OwnerUserID string     `json:"ownerUserId"`
	DueAt       time.Time  `json:"dueAt"`
	Title       string     `json:"title"`
	Status      string     `json:"status"`
	CompletedAt *time.Time `json:"completedAt"`
	Revision    string     `json:"revision"`
}

func toTask(t *Task) taskDTO {
	return taskDTO{
		ID: t.ID, CustomerID: t.CustomerID, LeadID: t.LeadID, DealID: t.DealID, OwnerUserID: t.OwnerUserID,
		DueAt: t.DueAt, Title: t.Title, Status: t.Status, CompletedAt: t.CompletedAt, Revision: httpx.Revision(t.Version),
	}
}

type listingDTO struct {
	ID          string      `json:"id"`
	VehicleID   string      `json:"vehicleId"`
	Text        string      `json:"text"`
	AskingPrice money.Money `json:"askingPrice"`
	Status      string      `json:"status"`
	Revision    string      `json:"revision"`
	UpdatedAt   time.Time   `json:"updatedAt"`
}

func toListing(l *Listing) listingDTO {
	return listingDTO{
		ID: l.ID, VehicleID: l.VehicleID, Text: l.Text, AskingPrice: l.Price(), Status: l.Status,
		Revision: httpx.Revision(l.Version), UpdatedAt: l.UpdatedAt,
	}
}

type evidenceDTO struct {
	ID                string      `json:"id"`
	Amount            money.Money `json:"amount"`
	PaidOn            string      `json:"paidOn"`
	ExternalReference string      `json:"externalReference"`
	AttachmentIDs     []string    `json:"attachmentIds"`
	Status            string      `json:"status"`
	DecisionReason    string      `json:"decisionReason"`
	Revision          string      `json:"revision"`
}

type invoiceDTO struct {
	ID                string        `json:"id"`
	DealID            string        `json:"dealId"`
	Purpose           string        `json:"purpose"`
	Amount            money.Money   `json:"amount"`
	RecipientSnapshot string        `json:"recipientSnapshot"`
	DueDate           *string       `json:"dueDate"`
	Status            string        `json:"status"`
	Paid              money.Money   `json:"paid"`
	Pending           money.Money   `json:"pending"`
	Outstanding       money.Money   `json:"outstanding"`
	Evidence          []evidenceDTO `json:"paymentEvidence"`
	Revision          string        `json:"revision"`
}

func toInvoice(v *InvoiceView) invoiceDTO {
	i := v.Invoice
	d := invoiceDTO{
		ID: i.ID, DealID: i.DealID, Purpose: i.Purpose, Amount: money.Money{AmountMinor: i.AmountMinor, Currency: i.Currency},
		RecipientSnapshot: i.RecipientSnapshot, Status: i.Status, Paid: v.Paid, Pending: v.Pending, Outstanding: v.Outstanding,
		Evidence: []evidenceDTO{}, Revision: httpx.Revision(i.Version),
	}
	if i.DueDate != nil {
		s := i.DueDate.Format(time.DateOnly)
		d.DueDate = &s
	}
	for _, e := range v.Evidence {
		d.Evidence = append(d.Evidence, evidenceDTO{
			ID: e.ID, Amount: money.Money{AmountMinor: e.AmountMinor, Currency: e.Currency},
			PaidOn: e.PaidOn.Format(time.DateOnly), ExternalReference: e.ExternalReference, AttachmentIDs: ids(e.AttachmentIDs), Status: e.Status,
			DecisionReason: e.DecisionReason, Revision: httpx.Revision(e.Version),
		})
	}
	return d
}

func dateString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.DateOnly)
	return &s
}

type dealDTO struct {
	ID                    string       `json:"id"`
	BranchID              string       `json:"branchId"`
	Customer              customerDTO  `json:"customer"`
	LeadID                *string      `json:"leadId"`
	VehicleID             string       `json:"vehicleId"`
	PaymentScheme         string       `json:"paymentScheme"`
	Price                 money.Money  `json:"price"`
	Status                string       `json:"status"`
	StatusReason          string       `json:"statusReason"`
	ContractSignedOn      *string      `json:"contractSignedOn"`
	ContractReference     string       `json:"contractReference"`
	ContractFileIDs       []string     `json:"contractFileIds"`
	RegisteredOn          *string      `json:"registeredOn"`
	PlateNumber           string       `json:"plateNumber"`
	RegistrationReference string       `json:"registrationReference"`
	DeliveredAt           *time.Time   `json:"deliveredAt"`
	Invoices              []invoiceDTO `json:"invoices,omitempty"`
	Checklist             *Checklist   `json:"checklist,omitempty"`
	AllowedActions        []string     `json:"allowedActions"`
	History               []historyDTO `json:"history,omitempty"`
	Revision              string       `json:"revision"`
	UpdatedAt             time.Time    `json:"updatedAt"`
}

func toDeal(full bool) func(*DealView) dealDTO {
	return func(v *DealView) dealDTO {
		d := v.Deal
		out := dealDTO{
			ID: d.ID, BranchID: d.BranchID, Customer: toCustomer(&v.Customer), LeadID: d.LeadID, VehicleID: d.VehicleID,
			PaymentScheme: d.PaymentScheme, Price: d.Price(), Status: d.Status, StatusReason: d.StatusReason,
			ContractSignedOn: dateString(d.ContractSignedOn), ContractReference: d.ContractReference, ContractFileIDs: ids(d.ContractFileIDs),
			RegisteredOn: dateString(d.RegisteredOn), PlateNumber: d.PlateNumber, RegistrationReference: d.RegistrationReference,
			DeliveredAt: d.DeliveredAt, AllowedActions: []string{}, Revision: httpx.Revision(d.Version), UpdatedAt: d.UpdatedAt,
		}
		if full {
			out.Invoices = []invoiceDTO{}
			for i := range v.Invoices {
				out.Invoices = append(out.Invoices, toInvoice(&v.Invoices[i]))
			}
			c := v.Checklist
			out.Checklist, out.History = &c, toHistory(v.History)
			if d.Status == "reserved" {
				out.AllowedActions = []string{"record-contract", "issue-invoice", "record-registration", "cancel"}
				if c.Ready() {
					out.AllowedActions = append(out.AllowedActions, "deliver")
				}
			}
		}
		return out
	}
}

func mapSlice[T, D any](items []T, f func(*T) D) []D {
	out := make([]D, len(items))
	for i := range items {
		out[i] = f(&items[i])
	}
	return out
}

// ---- handlers ----

type Handler struct {
	crm      *CRMService
	listings *ListingService
	deals    *DealService
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
	c.GET("/customers", h.listCustomers, auth.Require(PermRead))
	c.GET("/customers/:id", h.getCustomer, auth.Require(PermRead))
	c.POST("/customers", h.createCustomer, auth.Require(PermCRM))
	c.PATCH("/customers/:id", h.updateCustomer, auth.Require(PermCRM))

	c.GET("/leads", h.listLeads, auth.Require(PermRead))
	c.GET("/leads/:id", h.getLead, auth.Require(PermRead))
	c.POST("/leads", h.createLead, auth.Require(PermCRM))
	c.POST("/leads/:id/assign", h.assignLead, auth.Require(PermCRM))
	c.POST("/leads/:id/contacts", h.addContact, auth.Require(PermCRM))
	c.POST("/leads/:id/stage", h.setStage, auth.Require(PermCRM))

	c.GET("/tasks", h.listTasks, auth.Require(PermRead))
	c.POST("/tasks", h.createTask, auth.Require(PermCRM))
	c.POST("/tasks/:id/complete", h.completeTask, auth.Require(PermCRM))

	c.GET("/listings", h.listListings, auth.Require(PermRead))
	c.GET("/listings/:id", h.getListing, auth.Require(PermRead))
	c.POST("/listings", h.createListing, auth.Require(PermListings))
	c.PATCH("/listings/:id", h.updateListing, auth.Require(PermListings))
	c.POST("/listings/:id/publish", h.publishListing(true), auth.Require(PermListings))
	c.POST("/listings/:id/withdraw", h.publishListing(false), auth.Require(PermListings))

	c.GET("/deals", h.listDeals, auth.Require(PermRead))
	c.GET("/deals/:id", h.getDeal, auth.Require(PermRead))
	c.POST("/deals", h.createDeal, auth.Require(PermDeals))
	c.POST("/deals/:id/contract-records", h.recordContract, auth.Require(PermDeals))
	c.POST("/deals/:id/registration", h.recordRegistration, auth.Require(PermDeals))
	c.POST("/deals/:id/invoices", h.issueInvoice, auth.Require(PermDeals))
	c.POST("/deals/:id/deliveries", h.deliver, auth.Require(PermDeliver))
	c.POST("/deals/:id/cancel", h.cancelDeal, auth.Require(PermDeals))
	c.GET("/invoices/:id", h.getInvoice, auth.Require(PermRead))
	c.POST("/invoices/:id/evidence", h.submitEvidence, auth.Require(PermDeals))
	c.POST("/evidence/:id/accept", h.decideEvidence(true), auth.Require(PermPaymentsAccept))
	c.POST("/evidence/:id/reject", h.decideEvidence(false), auth.Require(PermPaymentsAccept))
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
//	@Success	200			{object}	httpx.ListEnvelope[retail.customerDTO]
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
//	@Success	200			{object}	httpx.DataEnvelope[retail.customerDTO]
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
//	@Param		Idempotency-Key	header		string			true	"retry key"
//	@Param		body			body		CustomerInput	true	"customer"
//	@Success	201				{object}	httpx.DataEnvelope[retail.customerDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/retail/customers [post]
func (h *Handler) createCustomer(c echo.Context) error {
	var in CustomerInput
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
//	@Param		id						path		string			true	"customer ID"
//	@Param		If-Match				header		string			true	"revision"
//	@Param		body					body		CustomerInput	true	"customer"
//	@Success	200						{object}	httpx.DataEnvelope[retail.customerDTO]
//	@Failure	401,403,404,412,422,428	{object}	httpx.ErrorBody
//	@Router		/retail/customers/{id} [patch]
func (h *Handler) updateCustomer(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in CustomerInput
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
//	@Success	200			{object}	httpx.ListEnvelope[retail.leadDTO]
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
//	@Success	200			{object}	httpx.DataEnvelope[retail.leadDTO]
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
//	@Param		Idempotency-Key	header		string		true	"retry key"
//	@Param		body			body		LeadInput	true	"lead"
//	@Success	201				{object}	httpx.DataEnvelope[retail.leadDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/retail/leads [post]
func (h *Handler) createLead(c echo.Context) error {
	var in LeadInput
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
//	@Param		Idempotency-Key				header		string				true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[retail.leadDTO]
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
//	@Param		Idempotency-Key		header		string				true	"retry key"
//	@Param		body				body		addContactRequest	true	"contact"
//	@Success	201					{object}	httpx.DataEnvelope[retail.leadDTO]
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
//	@Param		Idempotency-Key				header		string			true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[retail.leadDTO]
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
//	@Success	200			{object}	httpx.ListEnvelope[retail.taskDTO]
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
//	@Param		Idempotency-Key	header		string		true	"retry key"
//	@Param		body			body		TaskInput	true	"task"
//	@Success	201				{object}	httpx.DataEnvelope[retail.taskDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/retail/tasks [post]
func (h *Handler) createTask(c echo.Context) error {
	var in TaskInput
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
//	@Param		Idempotency-Key			header		string	true	"retry key"
//	@Success	200						{object}	httpx.DataEnvelope[retail.taskDTO]
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
//	@Success	200			{object}	httpx.ListEnvelope[retail.listingDTO]
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
//	@Success	200			{object}	httpx.DataEnvelope[retail.listingDTO]
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
//	@Param		Idempotency-Key	header		string			true	"retry key"
//	@Param		body			body		ListingInput	true	"listing"
//	@Success	201				{object}	httpx.DataEnvelope[retail.listingDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/retail/listings [post]
func (h *Handler) createListing(c echo.Context) error {
	var in ListingInput
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
//	@Success	200						{object}	httpx.DataEnvelope[retail.listingDTO]
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
//	@Param		Idempotency-Key			header		string	true	"retry key"
//	@Success	200						{object}	httpx.DataEnvelope[retail.listingDTO]
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
//	@Success	200			{object}	httpx.ListEnvelope[retail.dealDTO]
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
//	@Success	200			{object}	httpx.DataEnvelope[retail.dealDTO]
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
//	@Param		Idempotency-Key	header		string		true	"retry key"
//	@Param		body			body		DealInput	true	"deal"
//	@Success	201				{object}	httpx.DataEnvelope[retail.dealDTO]
//	@Failure	401,403,409,422	{object}	httpx.ErrorBody
//	@Router		/retail/deals [post]
func (h *Handler) createDeal(c echo.Context) error {
	var in DealInput
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
//	@Param		Idempotency-Key				header		string					true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[retail.dealDTO]
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
//	@Param		Idempotency-Key				header		string						true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[retail.dealDTO]
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
//	@Param		id							path		string			true	"deal ID"
//	@Param		If-Match					header		string			true	"revision"
//	@Param		body						body		InvoiceInput	true	"invoice"
//	@Param		Idempotency-Key				header		string			true	"retry key"
//	@Success	201							{object}	httpx.DataEnvelope[retail.invoiceDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/retail/deals/{id}/invoices [post]
func (h *Handler) issueInvoice(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in InvoiceInput
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
//	@Param		Idempotency-Key				header		string			true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[retail.dealDTO]
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
//	@Param		Idempotency-Key				header		string				true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[retail.dealDTO]
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
//	@Success	200			{object}	httpx.DataEnvelope[retail.invoiceDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/retail/invoices/{id} [get]
func (h *Handler) getInvoice(c echo.Context) error {
	p := auth.Get(c)
	i, err := h.deals.store.Deals().Invoice(c.Request().Context(), p.CompanyID, c.Param("id"))
	if err != nil {
		return err
	}
	if _, err := h.deals.deal(c.Request().Context(), h.deals.store, p, i.DealID, -1); err != nil {
		return err
	}
	v, err := h.deals.invoiceView(c.Request().Context(), h.deals.store, i)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toInvoice(v), i.Version)
}

// submitEvidence submits payment evidence for an invoice.
//
//	@Summary	Submit payment evidence
//	@Tags		retail/deals
//	@Security	CSRF
//	@Param		id					path		string			true	"invoice ID"
//	@Param		Idempotency-Key		header		string			true	"retry key"
//	@Param		body				body		EvidenceInput	true	"evidence"
//	@Success	201					{object}	httpx.DataEnvelope[retail.invoiceDTO]
//	@Failure	401,403,404,409,422	{object}	httpx.ErrorBody
//	@Router		/retail/invoices/{id}/evidence [post]
func (h *Handler) submitEvidence(c echo.Context) error {
	var in EvidenceInput
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
//	@Param		Idempotency-Key				header		string					true	"retry key"
//	@Success	200							{object}	httpx.DataEnvelope[retail.invoiceDTO]
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

// ---- module ----

type Module struct {
	handler *Handler
	Deals   *DealService
}

func New(db *gorm.DB, now func() time.Time, company Company, stock Stock, insurance Insurance, files Files) *Module {
	if now == nil {
		now = time.Now
	}
	d := deps{store: NewStore(db), company: company, stock: stock, files: files, now: now}
	deals := &DealService{deps: d, insurance: insurance}
	return &Module{handler: &Handler{crm: &CRMService{d}, listings: &ListingService{d}, deals: deals}, Deals: deals}
}

// Register mounts the retail routes under /api/v1/retail.
func (m *Module) Register(api *echo.Group) { m.handler.Routes(api.Group("/retail")) }

func ids(raw []byte) []string {
	out := []string{}
	_ = json.Unmarshal(raw, &out)
	return out
}
