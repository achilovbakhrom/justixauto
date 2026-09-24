package service

import (
	"context"

	"github.com/google/uuid"

	"justixauto/internal/modules/identity/model"
	"justixauto/internal/pkg/apperr"
)

// NewAudit builds the audit log service.
func NewAudit(d Deps) *Audit { return &Audit{d} }

// Audit reads the identity audit log.
type Audit struct{ Deps }

func (s *Audit) List(ctx context.Context, f model.AuditFilter) ([]model.AuditEvent, error) {
	var v apperr.Validation
	for field, id := range map[string]string{"resourceId": f.ResourceID, "actorId": f.ActorID} {
		if id != "" && uuid.Validate(id) != nil {
			v.Add(field, "must be a valid ID")
		}
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	return s.store.Audit().List(ctx, f)
}
