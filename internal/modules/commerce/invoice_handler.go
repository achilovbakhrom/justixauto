package commerce

import (
	"encoding/json"
	"math/big"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/httpx"
	"justixauto/internal/platform/money"
)

type evidenceDTO struct {
	ID                string      `json:"id"`
	Amount            money.Money `json:"amount"`
	PaidOn            string      `json:"paidOn"`
	ExternalReference string      `json:"externalReference"`
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
		d := invoiceDTO{ID: i.ID, OrderID: i.OrderID, Total: money.Money{AmountMinor: i.TotalMinor, Currency: i.Currency},
			Schedule: schedule, Status: i.Status, VoidReason: i.VoidReason, Paid: v.Paid, Pending: v.Pending,
			Outstanding: v.Outstanding, Evidence: []evidenceDTO{}, AllowedActions: []string{}, Revision: httpx.Revision(i.Version)}
		open := new(big.Int).Sub(amount(v.Outstanding.AmountMinor), amount(v.Pending.AmountMinor))
		switch {
		case i.Status == "issued" && !supplier && open.Sign() > 0:
			d.AllowedActions = append(d.AllowedActions, "submit-payment")
		case i.Status == "issued" && supplier && v.Paid.AmountMinor == "0" && v.Pending.AmountMinor == "0":
			d.AllowedActions = append(d.AllowedActions, "void")
		}
		for _, e := range v.Evidence {
			ed := evidenceDTO{ID: e.ID, Amount: money.Money{AmountMinor: e.AmountMinor, Currency: e.Currency},
				PaidOn: e.PaidOn.Format(time.DateOnly), ExternalReference: e.ExternalReference, Status: e.Status,
				DecisionReason: e.DecisionReason, AllowedActions: []string{}, Revision: httpx.Revision(e.Version),
				CreatedAt: e.CreatedAt, DecidedAt: e.DecidedAt}
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

func (h *Handler) invoiceResponse(c echo.Context, status int, v *InvoiceView) error {
	return httpx.Data(c, status, toInvoice(auth.Get(c).CompanyID)(v), v.Invoice.Version)
}

func (h *Handler) orderInvoices(c echo.Context) error {
	vs, err := h.invoices.ForOrder(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.List(c, mapSlice(vs, toInvoice(auth.Get(c).CompanyID)), nil)
}

func (h *Handler) issueInvoice(c echo.Context) error {
	expected, err := httpx.IfMatch(c)
	if err != nil {
		return err
	}
	var in struct {
		DueDate string `json:"dueDate"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		return err
	}
	v, err := h.invoices.Issue(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.DueDate)
	if err != nil {
		return err
	}
	return h.invoiceResponse(c, http.StatusCreated, v)
}

func (h *Handler) getInvoice(c echo.Context) error {
	v, err := h.invoices.Get(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return h.invoiceResponse(c, http.StatusOK, v)
}

func (h *Handler) voidInvoice(c echo.Context) error {
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
	v, err := h.invoices.Void(c.Request().Context(), auth.Get(c), c.Param("id"), expected, in.Reason)
	if err != nil {
		return err
	}
	return h.invoiceResponse(c, http.StatusOK, v)
}

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
		v, err := h.invoices.DecideEvidence(c.Request().Context(), auth.Get(c), c.Param("id"), expected, accept, in.Confirmation, in.Reason)
		if err != nil {
			return err
		}
		return h.invoiceResponse(c, http.StatusOK, v)
	}
}
