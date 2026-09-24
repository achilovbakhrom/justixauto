package identity

import (
	"context"

	"github.com/google/uuid"

	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
)

type BranchInput struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	// Warehouse: only {"mode":"none"} until the inventory module exists.
	Warehouse *struct {
		Mode string `json:"mode"`
	} `json:"warehouse"`
}

type BranchService struct{ deps }

// companyFor checks the caller may act in the company: members (plus the
// permission, if any), or directory readers for read access.
func (s *BranchService) companyFor(ctx context.Context, actor *auth.Principal, companyID, permission string) error {
	if err := validID(companyID); err != nil {
		return err
	}
	member, err := s.isMember(ctx, s.store, actor.UserID, companyID)
	if err != nil {
		return err
	}
	if !member {
		if permission == "" && actor.Can(PermPlatformDirectoryRead) {
			_, err := s.store.Companies().Get(ctx, companyID)
			return err
		}
		return apperr.ErrNotFound
	}
	if permission != "" {
		return actor.Allow(permission)
	}
	return nil
}

func (in BranchInput) apply(v *apperr.Validation, b *Branch) {
	b.Name = text(v, "name", in.Name, 1, 200)
	b.Address = text(v, "address", in.Address, 0, 500)
}

func duplicateBranch(err error) error {
	if isConflict(err) {
		return apperr.New(apperr.ErrConflict, "branch_duplicate", "a branch with this name already exists in the company")
	}
	return err
}

func (s *BranchService) List(ctx context.Context, actor *auth.Principal, companyID string) ([]Branch, error) {
	if err := s.companyFor(ctx, actor, companyID, ""); err != nil {
		return nil, err
	}
	return s.store.Branches().List(ctx, companyID)
}

func (s *BranchService) Create(ctx context.Context, actor *auth.Principal, companyID string, in BranchInput) (*Branch, error) {
	if err := s.companyFor(ctx, actor, companyID, PermBranchesCreate); err != nil {
		return nil, err
	}
	var v apperr.Validation
	now := s.clock()
	b := &Branch{ID: uuid.NewString(), CompanyID: companyID, Version: 1, CreatedAt: now, UpdatedAt: now}
	in.apply(&v, b)
	if in.Warehouse != nil && in.Warehouse.Mode != "none" {
		v.Add("warehouse.mode", "only \"none\" is supported until warehouses exist")
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	err := s.store.InTx(ctx, func(st Store) error {
		if err := st.Branches().Create(ctx, b); err != nil {
			return duplicateBranch(err)
		}
		return s.audit(ctx, st, actor, "branch.created", "branch", b.ID, &companyID, "", map[string]any{"name": b.Name})
	})
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (s *BranchService) Update(ctx context.Context, actor *auth.Principal, companyID, id string, expected int64, in BranchInput) (*Branch, error) {
	if err := s.companyFor(ctx, actor, companyID, PermBranchesEdit); err != nil {
		return nil, err
	}
	if err := validID(id); err != nil {
		return nil, err
	}
	var result *Branch
	err := s.store.InTx(ctx, func(st Store) error {
		b, err := st.Branches().Get(ctx, companyID, id)
		if err != nil {
			return err
		}
		if b.Version != expected {
			return apperr.ErrStale
		}
		var v apperr.Validation
		in.apply(&v, b)
		if err := v.Err(); err != nil {
			return err
		}
		b.UpdatedAt = s.clock()
		if err := st.Branches().Update(ctx, b, expected); err != nil {
			return duplicateBranch(err)
		}
		result = b
		return s.audit(ctx, st, actor, "branch.updated", "branch", b.ID, &companyID, "", map[string]any{"name": b.Name})
	})
	return result, err
}

type AuditService struct{ deps }

func (s *AuditService) List(ctx context.Context, f AuditFilter) ([]AuditEvent, error) {
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
