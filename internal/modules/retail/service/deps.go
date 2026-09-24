// Package service holds retail's business rules. It imports model and
// internal/pkg only; it must never import echo, gorm, repository or handler.
package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/modules/retail/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
)

// Deps are the dependencies every retail service embeds.
type Deps struct {
	store   Store
	company Company
	stock   Stock
	files   Files
	now     func() time.Time
}

// NewDeps builds the shared dependencies every retail service embeds.
func NewDeps(store Store, company Company, stock Stock, files Files, now func() time.Time) Deps {
	return Deps{store: store, company: company, stock: stock, files: files, now: now}
}

// NewCRM builds the customer, lead and task service.
func NewCRM(d Deps) *CRM { return &CRM{d} }

// NewListing builds the marketplace listing service.
func NewListing(d Deps) *Listing { return &Listing{d} }

// NewDeal builds the retail sale service.
func NewDeal(d Deps, insurance Insurance) *Deal { return &Deal{Deps: d, insurance: insurance} }

func (d Deps) clock() time.Time { return d.now().UTC() }

func (d Deps) event(ctx context.Context, st Store, p *auth.Principal, eventType, resourceType, id, reason string, details map[string]any) error {
	raw := []byte("{}")
	if details != nil {
		raw, _ = json.Marshal(details)
	}
	return st.Events().Append(ctx, &model.Event{
		ID: uuid.NewString(), CompanyID: p.CompanyID, EventType: eventType,
		ResourceType: resourceType, ResourceID: id, ActorUserID: p.UserID, OccurredAt: d.clock(), Reason: reason, Details: raw,
	})
}

// branch checks that a branch belongs to the active company and is inside the
// session's branch scope.
func (d Deps) branch(ctx context.Context, p *auth.Principal, field, branchID string) error {
	ok, err := d.company.BranchOf(ctx, p.CompanyID, branchID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.FieldError(field, "not a branch of your company")
	}
	if !inScope(p, branchID) {
		return apperr.FieldError(field, "outside your selected branches")
	}
	return nil
}

// inScope applies the session's branch scope (ALL or SELECTED).
func inScope(p *auth.Principal, branchID string) bool {
	if p.BranchScope.Mode != "SELECTED" {
		return true
	}
	for _, b := range p.BranchScope.BranchIDs {
		if b == branchID {
			return true
		}
	}
	return false
}

// scopeBranches returns the branch filter for lists (nil = all).
func scopeBranches(p *auth.Principal) []string {
	if p.BranchScope.Mode == "SELECTED" {
		return p.BranchScope.BranchIDs
	}
	return nil
}

func (d Deps) member(ctx context.Context, p *auth.Principal, field, userID string) error {
	ok, err := d.company.IsMember(ctx, userID, p.CompanyID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.FieldError(field, "not a member of your company")
	}
	return nil
}
