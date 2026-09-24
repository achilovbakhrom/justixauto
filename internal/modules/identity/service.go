package identity

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/validate"
)

// deps is shared by all identity services.
type deps struct {
	store Store
	now   func() time.Time
}

func (d deps) clock() time.Time { return d.now().UTC() }

// audit appends an audit event inside the caller's transaction.
func (d deps) audit(ctx context.Context, st Store, actor *auth.Principal, action, resourceType, resourceID string, companyID *string, reason string, details map[string]any) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	e := &AuditEvent{
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
func (d deps) isMember(ctx context.Context, st Store, userID, companyID string) (bool, error) {
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
