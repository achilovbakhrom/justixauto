package retail

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/httpx"
	"justixauto/internal/platform/money"
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
	return leadDTO{ID: l.ID, CustomerID: l.CustomerID, BranchID: l.BranchID, Source: l.Source, Stage: l.Stage,
		AssignedUserID: l.AssignedUserID, LostReason: l.LostReason, DealID: l.DealID, Revision: httpx.Revision(l.Version), UpdatedAt: l.UpdatedAt}
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
	return taskDTO{ID: t.ID, CustomerID: t.CustomerID, LeadID: t.LeadID, DealID: t.DealID, OwnerUserID: t.OwnerUserID,
		DueAt: t.DueAt, Title: t.Title, Status: t.Status, CompletedAt: t.CompletedAt, Revision: httpx.Revision(t.Version)}
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
	return listingDTO{ID: l.ID, VehicleID: l.VehicleID, Text: l.Text, AskingPrice: l.Price(), Status: l.Status,
		Revision: httpx.Revision(l.Version), UpdatedAt: l.UpdatedAt}
}

type evidenceDTO struct {
	ID                string      `json:"id"`
	Amount            money.Money `json:"amount"`
	PaidOn            string      `json:"paidOn"`
	ExternalReference string      `json:"externalReference"`
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
	d := invoiceDTO{ID: i.ID, DealID: i.DealID, Purpose: i.Purpose, Amount: money.Money{AmountMinor: i.AmountMinor, Currency: i.Currency},
		RecipientSnapshot: i.RecipientSnapshot, Status: i.Status, Paid: v.Paid, Pending: v.Pending, Outstanding: v.Outstanding,
		Evidence: []evidenceDTO{}, Revision: httpx.Revision(i.Version)}
	if i.DueDate != nil {
		s := i.DueDate.Format(time.DateOnly)
		d.DueDate = &s
	}
	for _, e := range v.Evidence {
		d.Evidence = append(d.Evidence, evidenceDTO{ID: e.ID, Amount: money.Money{AmountMinor: e.AmountMinor, Currency: e.Currency},
			PaidOn: e.PaidOn.Format(time.DateOnly), ExternalReference: e.ExternalReference, Status: e.Status,
			DecisionReason: e.DecisionReason, Revision: httpx.Revision(e.Version)})
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
		out := dealDTO{ID: d.ID, BranchID: d.BranchID, Customer: toCustomer(&v.Customer), LeadID: d.LeadID, VehicleID: d.VehicleID,
			PaymentScheme: d.PaymentScheme, Price: d.Price(), Status: d.Status, StatusReason: d.StatusReason,
			ContractSignedOn: dateString(d.ContractSignedOn), ContractReference: d.ContractReference,
			RegisteredOn: dateString(d.RegisteredOn), PlateNumber: d.PlateNumber, RegistrationReference: d.RegistrationReference,
			DeliveredAt: d.DeliveredAt, AllowedActions: []string{}, Revision: httpx.Revision(d.Version), UpdatedAt: d.UpdatedAt}
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

func (h *Handler) getCustomer(c echo.Context) error {
	cu, err := h.crm.Customer(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toCustomer(cu), cu.Version)
}

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

func (h *Handler) getLead(c echo.Context) error {
	return h.leadResponse(c, http.StatusOK, c.Param("id"))
}

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

func (h *Handler) assignLead(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		AssignedUserID string `json:"assignedUserId"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	l, err := h.crm.Assign(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.AssignedUserID)
	if err != nil {
		return err
	}
	return h.leadResponse(c, http.StatusOK, l.ID)
}

func (h *Handler) addContact(c echo.Context) error {
	var in struct {
		Channel string `json:"channel"`
		Note    string `json:"note"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	l, err := h.crm.AddContact(c.Request().Context(), auth.Get(c), c.Param("id"), in.Channel, in.Note)
	if err != nil {
		return err
	}
	return h.leadResponse(c, http.StatusCreated, l.ID)
}

func (h *Handler) setStage(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		Stage  string `json:"stage"`
		Reason string `json:"reason"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	l, err := h.crm.SetStage(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Stage, in.Reason)
	if err != nil {
		return err
	}
	return h.leadResponse(c, http.StatusOK, l.ID)
}

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

func (h *Handler) getListing(c echo.Context) error {
	l, err := h.listings.Get(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toListing(l), l.Version)
}

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

func (h *Handler) updateListing(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		Text        string      `json:"text"`
		AskingPrice money.Money `json:"askingPrice"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	l, err := h.listings.Update(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Text, in.AskingPrice)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toListing(l), l.Version)
}

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

func (h *Handler) getDeal(c echo.Context) error {
	return h.dealResponse(c, http.StatusOK, c.Param("id"))
}

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

func (h *Handler) recordContract(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		SignedOn   string   `json:"signedOn"`
		Reference  string   `json:"reference"`
		BindingIDs []string `json:"bindingIds"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	d, err := h.deals.RecordContract(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.SignedOn, in.Reference)
	if err != nil {
		return err
	}
	return h.dealResponse(c, http.StatusOK, d.ID)
}

func (h *Handler) recordRegistration(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		RegisteredOn string `json:"registeredOn"`
		PlateNumber  string `json:"plateNumber"`
		Reference    string `json:"reference"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	d, err := h.deals.RecordRegistration(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.RegisteredOn, in.PlateNumber, in.Reference)
	if err != nil {
		return err
	}
	return h.dealResponse(c, http.StatusOK, d.ID)
}

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

func (h *Handler) deliver(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		OccurredAt time.Time `json:"occurredAt"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	d, err := h.deals.Deliver(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.OccurredAt)
	if err != nil {
		return err
	}
	return h.dealResponse(c, http.StatusOK, d.ID)
}

func (h *Handler) cancelDeal(c echo.Context) error {
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
	d, err := h.deals.Cancel(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Reason)
	if err != nil {
		return err
	}
	return h.dealResponse(c, http.StatusOK, d.ID)
}

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

func (h *Handler) decideEvidence(accept bool) echo.HandlerFunc {
	return func(c echo.Context) error {
		expected, err := httpx.IfMatch(c)
		if err != nil {
			return err
		}
		var in struct {
			Confirmation bool   `json:"confirmation"`
			Reason       string `json:"reason"`
		}
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

func New(db *gorm.DB, now func() time.Time, company Company, stock Stock, insurance Insurance) *Module {
	if now == nil {
		now = time.Now
	}
	d := deps{store: NewStore(db), company: company, stock: stock, now: now}
	deals := &DealService{deps: d, insurance: insurance}
	return &Module{handler: &Handler{crm: &CRMService{d}, listings: &ListingService{d}, deals: deals}, Deals: deals}
}

// Register mounts the retail routes under /api/v1/retail.
func (m *Module) Register(api *echo.Group) { m.handler.Routes(api.Group("/retail")) }
