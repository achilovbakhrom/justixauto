package commerce

import (
	"encoding/json"
	"math/big"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/httpx"
	"justixauto/internal/pkg/money"
)

type evidenceDTO struct {
	ID                string      `json:"id"`
	Amount            money.Money `json:"amount"`
	PaidOn            string      `json:"paidOn"`
	ExternalReference string      `json:"externalReference"`
	AttachmentIDs     []string    `json:"attachmentIds"`
	Status            string      `json:"status"`
	DecisionReason    string      `json:"decisionReason"`
	AllowedActions    []string    `json:"allowedActions"`
	Revision          string      `json:"revision"`
	CreatedAt         time.Time   `json:"createdAt"`
	DecidedAt         *time.Time  `json:"decidedAt"`
}

type invoiceDTO struct {
	ID             string        `json:"id"`
	OrderID        string        `json:"orderId"`
	Total          money.Money   `json:"total"`
	Schedule       []Installment `json:"schedule"`
	Status         string        `json:"status"`
	VoidReason     string        `json:"voidReason"`
	Paid           money.Money   `json:"paid"`
	Pending        money.Money   `json:"pending"`
	Outstanding    money.Money   `json:"outstanding"`
	Evidence       []evidenceDTO `json:"paymentEvidence"`
	AllowedActions []string      `json:"allowedActions"`
	Revision       string        `json:"revision"`
}

func toInvoice(companyID string) func(*InvoiceView) invoiceDTO {
	return func(v *InvoiceView) invoiceDTO {
		i := v.Invoice
		var schedule []Installment
		_ = json.Unmarshal(i.Schedule, &schedule)
		supplier := i.SupplierCompanyID == companyID
		d := invoiceDTO{
			ID: i.ID, OrderID: i.OrderID, Total: money.Money{AmountMinor: i.TotalMinor, Currency: i.Currency},
			Schedule: schedule, Status: i.Status, VoidReason: i.VoidReason, Paid: v.Paid, Pending: v.Pending,
			Outstanding: v.Outstanding, Evidence: []evidenceDTO{}, AllowedActions: []string{}, Revision: httpx.Revision(i.Version),
		}
		open := new(big.Int).Sub(amount(v.Outstanding.AmountMinor), amount(v.Pending.AmountMinor))
		switch {
		case i.Status == "issued" && !supplier && open.Sign() > 0:
			d.AllowedActions = append(d.AllowedActions, "submit-payment")
		case i.Status == "issued" && supplier && v.Paid.AmountMinor == "0" && v.Pending.AmountMinor == "0":
			d.AllowedActions = append(d.AllowedActions, "void")
		}
		for _, e := range v.Evidence {
			ed := evidenceDTO{
				ID: e.ID, Amount: money.Money{AmountMinor: e.AmountMinor, Currency: e.Currency},
				PaidOn: e.PaidOn.Format(time.DateOnly), ExternalReference: e.ExternalReference, AttachmentIDs: ids(e.AttachmentIDs), Status: e.Status,
				DecisionReason: e.DecisionReason, AllowedActions: []string{}, Revision: httpx.Revision(e.Version),
				CreatedAt: e.CreatedAt, DecidedAt: e.DecidedAt,
			}
			if supplier && e.Status == "submitted" {
				ed.AllowedActions = []string{"accept", "reject"}
			}
			d.Evidence = append(d.Evidence, ed)
		}
		return d
	}
}

func (h *Handler) invoiceRoutes(c *echo.Group) {
	c.GET("/orders/:id/invoices", h.orderInvoices, auth.Require(PermRead))
	c.POST("/orders/:id/invoices", h.issueInvoice, auth.Require(PermTrade))
	c.GET("/invoices/:id", h.getInvoice, auth.Require(PermRead))
	c.POST("/invoices/:id/void", h.voidInvoice, auth.Require(PermTrade))
	c.POST("/invoices/:id/payment-evidence", h.submitEvidence, auth.Require(PermTrade))
	c.POST("/payment-evidence/:id/accept", h.decideEvidence(true), auth.Require(PermPaymentsAccept))
	c.POST("/payment-evidence/:id/reject", h.decideEvidence(false), auth.Require(PermPaymentsAccept))
}

// issueInvoiceRequest issues an invoice for an accepted order.
type issueInvoiceRequest struct {
	DueDate string `json:"dueDate"`
}

// invoiceVoidRequest voids an unpaid issued invoice.
type invoiceVoidRequest struct {
	Reason string `json:"reason"`
}

// evidenceDecisionRequest accepts or rejects submitted payment evidence.
type evidenceDecisionRequest struct {
	Confirmation bool   `json:"confirmation"`
	Reason       string `json:"reason"`
}

func (h *Handler) invoiceResponse(c echo.Context, status int, v *InvoiceView) error {
	return httpx.Data(c, status, toInvoice(auth.Get(c).CompanyID)(v), v.Invoice.Version)
}

// orderInvoices lists invoices issued for an order.
//
//	@Summary	List order invoices
//	@Tags		commerce/invoices
//	@Param		id			path		string	true	"order ID"
//	@Success	200			{object}	httpx.ListEnvelope[commerce.invoiceDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/commerce/orders/{id}/invoices [get]
func (h *Handler) orderInvoices(c echo.Context) error {
	vs, err := h.invoices.ForOrder(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(vs, toInvoice(auth.Get(c).CompanyID)), nil)
}

// issueInvoice issues an invoice for an accepted order.
//
//	@Summary	Issue invoice
//	@Tags		commerce/invoices
//	@Security	CSRF
//	@Param		id							path		string				true	"order ID"
//	@Param		If-Match					header		string				true	"revision"
//	@Param		body						body		issueInvoiceRequest	true	"due date"
//	@Success	201							{object}	httpx.DataEnvelope[commerce.invoiceDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/orders/{id}/invoices [post]
func (h *Handler) issueInvoice(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in issueInvoiceRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.invoices.Issue(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.DueDate)
	if err != nil {
		return err
	}
	return h.invoiceResponse(c, http.StatusCreated, v)
}

// getInvoice returns one invoice.
//
//	@Summary	Get invoice
//	@Tags		commerce/invoices
//	@Param		id			path		string	true	"invoice ID"
//	@Success	200			{object}	httpx.DataEnvelope[commerce.invoiceDTO]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/commerce/invoices/{id} [get]
func (h *Handler) getInvoice(c echo.Context) error {
	v, err := h.invoices.Get(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return h.invoiceResponse(c, http.StatusOK, v)
}

// voidInvoice voids an issued invoice that has no payments yet.
//
//	@Summary	Void invoice
//	@Tags		commerce/invoices
//	@Security	CSRF
//	@Param		id							path		string				true	"invoice ID"
//	@Param		If-Match					header		string				true	"revision"
//	@Param		body						body		invoiceVoidRequest	true	"reason"
//	@Success	200							{object}	httpx.DataEnvelope[commerce.invoiceDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/invoices/{id}/void [post]
func (h *Handler) voidInvoice(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in invoiceVoidRequest
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.invoices.Void(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Reason)
	if err != nil {
		return err
	}
	return h.invoiceResponse(c, http.StatusOK, v)
}

// submitEvidence submits payment evidence for an issued invoice.
//
//	@Summary	Submit payment evidence
//	@Tags		commerce/invoices
//	@Security	CSRF
//	@Param		id					path		string			true	"invoice ID"
//	@Param		body				body		EvidenceInput	true	"payment evidence"
//	@Success	201					{object}	httpx.DataEnvelope[commerce.invoiceDTO]
//	@Failure	401,403,404,409,422	{object}	httpx.ErrorBody
//	@Router		/commerce/invoices/{id}/payment-evidence [post]
func (h *Handler) submitEvidence(c echo.Context) error {
	var in EvidenceInput
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.invoices.SubmitEvidence(c.Request().Context(), auth.Get(c), c.Param("id"), in)
	if err != nil {
		return err
	}
	return h.invoiceResponse(c, http.StatusCreated, v)
}

// decideEvidence accepts or rejects submitted payment evidence.
//
//	@Summary	Decide payment evidence
//	@Tags		commerce/invoices
//	@Security	CSRF
//	@Param		id							path		string					true	"payment evidence ID"
//	@Param		If-Match					header		string					true	"revision"
//	@Param		body						body		evidenceDecisionRequest	true	"decision"
//	@Success	200							{object}	httpx.DataEnvelope[commerce.invoiceDTO]
//	@Failure	401,403,404,409,412,422,428	{object}	httpx.ErrorBody
//	@Router		/commerce/payment-evidence/{id}/accept [post]
//	@Router		/commerce/payment-evidence/{id}/reject [post]
func (h *Handler) decideEvidence(accept bool) echo.HandlerFunc {
	return func(c echo.Context) error {
		expected, err := httpx.IfMatch(c)
		if err != nil {
			return err
		}
		var in evidenceDecisionRequest
		if err := httpx.Bind(c, &in); err != nil {
			return err
		}
		v, err := h.invoices.DecideEvidence(c.Request().Context(), auth.Get(c), c.Param("id"), expected, accept, in.Confirmation, in.Reason)
		if err != nil {
			return err
		}
		return h.invoiceResponse(c, http.StatusOK, v)
	}
}

func ids(raw []byte) []string {
	out := []string{}
	_ = json.Unmarshal(raw, &out)
	return out
}
