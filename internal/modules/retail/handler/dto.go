// Package handler holds retail's Echo routes, request/response DTOs and
// mappers. It imports service, model and internal/pkg only; it must never
// import repository or gorm.
package handler

import (
	"encoding/json"
	"time"

	"justixauto/internal/modules/retail/model"
	"justixauto/internal/modules/retail/service"
	"justixauto/internal/pkg/httpx"
	"justixauto/internal/pkg/money"
)

type customerDTO struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	Phone       string    `json:"phone"`
	Revision    string    `json:"revision"`
	CreatedAt   time.Time `json:"createdAt"`
}

func toCustomer(c *model.Customer) customerDTO {
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

func toHistory(es []model.Event) []historyDTO {
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

func toLead(l *model.Lead) leadDTO {
	return leadDTO{
		ID: l.ID, CustomerID: l.CustomerID, BranchID: l.BranchID, Source: l.Source, Stage: l.Stage,
		AssignedUserID: l.AssignedUserID, LostReason: l.LostReason, DealID: l.DealID, Revision: httpx.Revision(l.Version), UpdatedAt: l.UpdatedAt,
	}
}

func toLeadView(v *service.LeadView) leadDTO {
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

func toTask(t *model.Task) taskDTO {
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

func toListing(l *model.Listing) listingDTO {
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

func toInvoice(v *service.InvoiceView) invoiceDTO {
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
	ID                    string             `json:"id"`
	BranchID              string             `json:"branchId"`
	Customer              customerDTO        `json:"customer"`
	LeadID                *string            `json:"leadId"`
	VehicleID             string             `json:"vehicleId"`
	PaymentScheme         string             `json:"paymentScheme"`
	Price                 money.Money        `json:"price"`
	Status                string             `json:"status"`
	StatusReason          string             `json:"statusReason"`
	ContractSignedOn      *string            `json:"contractSignedOn"`
	ContractReference     string             `json:"contractReference"`
	ContractFileIDs       []string           `json:"contractFileIds"`
	RegisteredOn          *string            `json:"registeredOn"`
	PlateNumber           string             `json:"plateNumber"`
	RegistrationReference string             `json:"registrationReference"`
	DeliveredAt           *time.Time         `json:"deliveredAt"`
	Invoices              []invoiceDTO       `json:"invoices,omitempty"`
	Checklist             *service.Checklist `json:"checklist,omitempty"`
	AllowedActions        []string           `json:"allowedActions"`
	History               []historyDTO       `json:"history,omitempty"`
	Revision              string             `json:"revision"`
	UpdatedAt             time.Time          `json:"updatedAt"`
}

func toDeal(full bool) func(*service.DealView) dealDTO {
	return func(v *service.DealView) dealDTO {
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

func ids(raw []byte) []string {
	out := []string{}
	_ = json.Unmarshal(raw, &out)
	return out
}
