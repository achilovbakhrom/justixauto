package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
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

// validID turns malformed IDs into ErrNotFound instead of database errors.
func validID(ids ...string) error {
	for _, id := range ids {
		if uuid.Validate(id) != nil {
			return apperr.ErrNotFound
		}
	}
	return nil
}

func text(v *apperr.Validation, field, value string, min, max int) string {
	value = strings.TrimSpace(value)
	if n := utf8.RuneCountInString(value); n < min || n > max {
		if min > 0 {
			v.Add(field, "required, at most "+strconv.Itoa(max)+" characters")
		} else {
			v.Add(field, "at most "+strconv.Itoa(max)+" characters")
		}
	}
	return value
}

func email(v *apperr.Validation, field, value string) string {
	value = strings.TrimSpace(value)
	addr, err := mail.ParseAddress(value)
	if err != nil || addr.Address != value || len(value) > 254 {
		v.Add(field, "must be a valid email address")
	}
	return value
}

func reason(v *apperr.Validation, value string) string {
	return text(v, "reason", value, 1, 500)
}

// uniqueIDs validates and de-duplicates a list of UUIDs.
func uniqueIDs(v *apperr.Validation, field string, ids []string) []string {
	out := []string{}
	for _, id := range ids {
		if uuid.Validate(id) != nil {
			v.Add(field, "must contain valid IDs")
			return out
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}
