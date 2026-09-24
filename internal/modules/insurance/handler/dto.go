// Package handler holds insurance's Echo handlers, request/response DTOs and
// their mappers. It imports service, model and internal/pkg; it must never
// import repository or gorm.
package handler

import (
	"encoding/json"
	"time"

	"justixauto/internal/modules/insurance/model"
	"justixauto/internal/pkg/httpx"
)

type messageDTO struct {
	Kind       string    `json:"kind"`
	RequestID  *string   `json:"requestId"`
	Note       string    `json:"note"`
	Side       string    `json:"side"` // seller | insurer
	ActorID    string    `json:"actorId"`
	OccurredAt time.Time `json:"occurredAt"`
}

type applicationDTO struct {
	ID               string          `json:"id"`
	Side             string          `json:"side"` // the caller's side
	SellerCompanyID  string          `json:"sellerCompanyId"`
	InsurerCompanyID string          `json:"insurerCompanyId"`
	RetailDealID     string          `json:"retailDealId"`
	Status           string          `json:"status"`
	Note             string          `json:"note"`
	Snapshot         json.RawMessage `json:"snapshot"`
	History          []messageDTO    `json:"history,omitempty"`
	AllowedActions   []string        `json:"allowedActions"`
	Revision         string          `json:"revision"`
	SubmittedAt      *time.Time      `json:"submittedAt"`
	DecidedAt        *time.Time      `json:"decidedAt"`
}

func actions(a *model.Application, seller bool) []string {
	switch {
	case seller && a.Status == "draft":
		return []string{"edit", "submit"}
	case seller && a.Status == "needs-info":
		return []string{"respond"}
	case !seller && a.Status == "submitted":
		return []string{"take"}
	case !seller && a.Status == "review":
		return []string{"request", "approve", "decline"}
	}
	return []string{}
}

func toApplication(companyID string, a *model.Application, ms []model.Message) applicationDTO {
	seller := a.SellerCompanyID == companyID
	side := "insurer"
	if seller {
		side = "seller"
	}
	snap := json.RawMessage("null")
	if len(a.Snapshot) > 0 {
		snap = a.Snapshot
	}
	d := applicationDTO{
		ID: a.ID, Side: side, SellerCompanyID: a.SellerCompanyID, InsurerCompanyID: a.InsurerCompanyID,
		RetailDealID: a.DealID, Status: a.Status, Note: a.Note, Snapshot: snap, AllowedActions: actions(a, seller),
		Revision: httpx.Revision(a.Version), SubmittedAt: a.SubmittedAt, DecidedAt: a.DecidedAt,
	}
	if ms != nil {
		d.History = []messageDTO{}
		for _, m := range ms {
			s := "insurer"
			if m.CompanyID == a.SellerCompanyID {
				s = "seller"
			}
			d.History = append(d.History, messageDTO{Kind: m.Kind, RequestID: m.RequestID, Note: m.Note, Side: s, ActorID: m.ActorUserID, OccurredAt: m.CreatedAt})
		}
	}
	return d
}
