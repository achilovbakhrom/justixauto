// Package service holds identity's business rules. It imports model and
// internal/pkg only; it must never import echo, gorm, repository or handler.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/modules/identity/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/validate"
)

// Deps is shared by all identity services.
type Deps struct {
	store Store
	now   func() time.Time
}

// NewDeps builds the shared dependencies every identity service embeds.
func NewDeps(store Store, now func() time.Time) Deps { return Deps{store: store, now: now} }

func (d Deps) clock() time.Time { return d.now().UTC() }

// audit appends an audit event inside the caller's transaction.
func (d Deps) audit(ctx context.Context, st Store, actor *auth.Principal, action, resourceType, resourceID string, companyID *string, reason string, details map[string]any) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	e := &model.AuditEvent{
		ID: uuid.NewString(), OccurredAt: d.clock(), Action: action,
		ResourceType: resourceType, ResourceID: resourceID, CompanyID: companyID,
		Reason: reason, Details: raw,
	}
	if actor != nil {
		e.ActorUserID = &actor.UserID
	}
	return st.Audit().Append(ctx, e)
}

// isMember reports whether the user has an active membership in the company.
func (d Deps) isMember(ctx context.Context, st Store, userID, companyID string) (bool, error) {
	_, err := st.Memberships().Active(ctx, userID, companyID)
	if errors.Is(err, apperr.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Shared input checks (see internal/pkg/validate).
var (
	validID   = validate.IDs
	text      = validate.Text
	email     = validate.Email
	reason    = validate.Reason
	uniqueIDs = validate.UniqueIDs
)

// revision formats an optimistic-locking version for an ETag/If-Match value.
func revision(v int64) string { return strconv.FormatInt(v, 10) }
