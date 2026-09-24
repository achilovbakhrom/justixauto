// Package service holds commerce's business rules. It imports model and
// internal/pkg only; it must never import echo, gorm, repository or handler.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/modules/commerce/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
)

// Deps are the dependencies every commerce service embeds.
type Deps struct {
	store     Store
	directory Directory
	catalog   Catalog
	stock     Stock
	files     Files
	now       func() time.Time
}

// NewDeps builds the shared dependencies every commerce service embeds.
func NewDeps(store Store, directory Directory, catalog Catalog, stock Stock, files Files, now func() time.Time) Deps {
	return Deps{store: store, directory: directory, catalog: catalog, stock: stock, files: files, now: now}
}

// NewPartnership builds the partnership service.
func NewPartnership(d Deps) *Partnership { return &Partnership{d} }

// NewOffer builds the offer service.
func NewOffer(d Deps) *Offer { return &Offer{d} }

// NewDeal builds the RFQ/order service.
func NewDeal(d Deps, offers *Offer) *Deal { return &Deal{Deps: d, offers: offers} }

// NewFulfilment builds the allocation/shipment service.
func NewFulfilment(d Deps) *Fulfilment { return &Fulfilment{d} }

// NewInvoice builds the invoice/payment evidence service.
func NewInvoice(d Deps) *Invoice { return &Invoice{d} }

func (d Deps) clock() time.Time { return d.now().UTC() }

func (d Deps) event(ctx context.Context, st Store, p *auth.Principal, eventType, resourceType, resourceID, reason string, details map[string]any) error {
	raw := []byte("{}")
	if details != nil {
		raw, _ = json.Marshal(details)
	}
	return st.Events().Append(ctx, &model.Event{
		ID: uuid.NewString(), CompanyID: p.CompanyID, EventType: eventType,
		ResourceType: resourceType, ResourceID: resourceID, ActorUserID: p.UserID, OccurredAt: d.clock(),
		Reason: reason, Details: raw,
	})
}

// tradingCompany checks that a company exists, sells vehicles and has active
// platform access. Unknown companies surface as field errors, not 404s.
func (d Deps) tradingCompany(ctx context.Context, id, field string) (*Company, error) {
	c, err := d.directory.Company(ctx, id)
	if errors.Is(err, apperr.ErrNotFound) {
		return nil, apperr.FieldError(field, "company not found")
	}
	if err != nil {
		return nil, err
	}
	if c.Kind != "seller" || !c.Active {
		return nil, apperr.New(apperr.ErrConflict, "company_not_trading", c.Name+" cannot trade: it must be an active seller company")
	}
	return c, nil
}
