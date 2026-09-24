// Package service holds financing's business rules. It imports model and
// internal/pkg only; it must never import echo, gorm, repository or handler.
package service

import (
	"context"
	"errors"
	"time"

	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/validate"
)

// Service connects sellers with banks and MFOs: providers publish programs;
// sellers send a partner-finance sale with a calculation; the provider
// reviews, proposes terms and the seller agrees. Agreement is not a
// contract, signature, funding or permission to deliver.
type Service struct {
	repo      Repository
	sales     Sales
	directory Directory
	files     Files
	now       func() time.Time
}

// New builds the financing service.
func New(repo Repository, sales Sales, directory Directory, files Files, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, sales: sales, directory: directory, files: files, now: now}
}

func (s *Service) clock() time.Time { return s.now().UTC() }

func (s *Service) provider(ctx context.Context, id, field string) error {
	if validate.IDs(id) != nil {
		return apperr.FieldError(field, "must be a valid ID")
	}
	c, err := s.directory.Company(ctx, id)
	if errors.Is(err, apperr.ErrNotFound) || (err == nil && ((c.Kind != "bank" && c.Kind != "mfo") || !c.Active)) {
		return apperr.FieldError(field, "not an active bank or MFO")
	}
	return err
}
