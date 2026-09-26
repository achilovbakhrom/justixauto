package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"justixauto/internal/modules/identity/model"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
)

// NewMembership builds the membership service.
func NewMembership(d Deps) *Membership { return &Membership{d} }

type BranchAccessInput struct {
	Mode      string   `json:"mode"` // ALL_BRANCHES | SELECTED_BRANCHES
	BranchIDs []string `json:"branchIds"`
}

type GrantMembershipInput struct {
	CompanyID    string            `json:"companyId"`
	BranchAccess BranchAccessInput `json:"branchAccess"`
}

// Membership manages company membership grants and branch access.
type Membership struct{ Deps }

func isConflict(err error) bool { return errors.Is(err, apperr.ErrConflict) }

// branchAccess validates the access mode and that all branches belong to the company.
func (s *Membership) branchAccess(ctx context.Context, st Store, companyID string, in BranchAccessInput) (string, []string, error) {
	var v apperr.Validation
	ids := uniqueIDs(&v, "branchAccess.branchIds", in.BranchIDs)
	switch in.Mode {
	case model.AllBranches:
		if len(ids) != 0 {
			v.Add("branchAccess.branchIds", "must be empty for ALL_BRANCHES")
		}
	case model.SelectedBranches:
		if len(ids) == 0 {
			v.Add("branchAccess.branchIds", "select at least one branch")
		}
	default:
		v.Add("branchAccess.mode", "must be ALL_BRANCHES or SELECTED_BRANCHES")
	}
	if err := v.Err(); err != nil {
		return "", nil, err
	}
	n, err := st.Branches().CountInCompany(ctx, companyID, ids)
	if err != nil {
		return "", nil, err
	}
	if int(n) != len(ids) {
		return "", nil, apperr.FieldError("branchAccess.branchIds", "contains branches of another company")
	}
	return in.Mode, ids, nil
}

func (s *Membership) ListByUser(ctx context.Context, userID string) ([]model.Membership, error) {
	if err := validID(userID); err != nil {
		return nil, err
	}
	if _, err := s.store.Users().Get(ctx, userID); err != nil {
		return nil, err
	}
	return s.store.Memberships().ListByUser(ctx, userID)
}

// Grant adds an active membership. A duplicate active membership is a conflict;
// revoked ones stay as history and a new one can be granted.
func (s *Membership) Grant(ctx context.Context, actor *auth.Principal, userID string, in GrantMembershipInput) (*model.Membership, error) {
	if err := validID(userID); err != nil {
		return nil, err
	}
	if uuid.Validate(in.CompanyID) != nil {
		return nil, apperr.FieldError("companyId", "must be a valid ID")
	}
	var result *model.Membership
	err := s.store.InTx(ctx, func(st Store) error {
		if _, err := st.Users().Get(ctx, userID); err != nil {
			return err
		}
		if err := oneCompany(ctx, st, userID, in.CompanyID); err != nil {
			return err
		}
		if _, err := st.Companies().Get(ctx, in.CompanyID); errors.Is(err, apperr.ErrNotFound) {
			return apperr.FieldError("companyId", "company does not exist")
		} else if err != nil {
			return err
		}
		mode, ids, err := s.branchAccess(ctx, st, in.CompanyID, in.BranchAccess)
		if err != nil {
			return err
		}
		now := s.clock()
		m := &model.Membership{
			ID: uuid.NewString(), UserID: userID, CompanyID: in.CompanyID, Status: model.MembershipActive,
			BranchAccess: mode, BranchIDs: ids, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := st.Memberships().Create(ctx, m); err != nil {
			if isConflict(err) {
				return apperr.New(apperr.ErrConflict, "membership_exists", "the user is already a member of this company")
			}
			return err
		}
		result = m
		return s.audit(ctx, st, actor, "membership.granted", "membership", m.ID, &m.CompanyID, "",
			map[string]any{"userId": userID, "branchAccess": mode, "branchIds": ids})
	})
	return result, err
}

func (s *Membership) load(ctx context.Context, st Store, id string, expected int64) (*model.Membership, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	m, err := st.Memberships().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if m.Version != expected {
		return nil, apperr.ErrStale
	}
	if m.Status != model.MembershipActive {
		return nil, apperr.New(apperr.ErrConflict, "membership_revoked", "the membership is revoked")
	}
	return m, nil
}

// UpdateBranchAccess changes branch access; the user's sessions in that
// company fall back to "no company" so they re-select with fresh access.
func (s *Membership) UpdateBranchAccess(ctx context.Context, actor *auth.Principal, id string, expected int64, in BranchAccessInput) (*model.Membership, error) {
	var result *model.Membership
	err := s.store.InTx(ctx, func(st Store) error {
		m, err := s.load(ctx, st, id, expected)
		if err != nil {
			return err
		}
		mode, ids, err := s.branchAccess(ctx, st, m.CompanyID, in)
		if err != nil {
			return err
		}
		before := map[string]any{"mode": m.BranchAccess, "branchIds": m.BranchIDs}
		m.BranchAccess, m.BranchIDs, m.UpdatedAt = mode, ids, s.clock()
		if err := st.Memberships().Update(ctx, m, expected); err != nil {
			return err
		}
		if err := st.Sessions().ResetCompanyContext(ctx, m.UserID, m.CompanyID); err != nil {
			return err
		}
		result = m
		return s.audit(ctx, st, actor, "membership.branch_access_changed", "membership", m.ID, &m.CompanyID, "",
			map[string]any{"before": before, "after": map[string]any{"mode": mode, "branchIds": ids}})
	})
	return result, err
}

// Revoke ends one membership; the user's other memberships are untouched.
func (s *Membership) Revoke(ctx context.Context, actor *auth.Principal, id string, expected int64, why string) (*model.Membership, error) {
	var v apperr.Validation
	why = reason(&v, why)
	if err := v.Err(); err != nil {
		return nil, err
	}
	var result *model.Membership
	err := s.store.InTx(ctx, func(st Store) error {
		m, err := s.load(ctx, st, id, expected)
		if err != nil {
			return err
		}
		m.Status, m.StatusReason, m.UpdatedAt = model.MembershipRevoked, why, s.clock()
		if err := st.Memberships().Update(ctx, m, expected); err != nil {
			return err
		}
		if err := st.Sessions().ResetCompanyContext(ctx, m.UserID, m.CompanyID); err != nil {
			return err
		}
		result = m
		return s.audit(ctx, st, actor, "membership.revoked", "membership", m.ID, &m.CompanyID, why, map[string]any{"userId": m.UserID})
	})
	return result, err
}

// oneCompany enforces "one user = one company" (user decision 2026-09-26):
// a user with an active membership elsewhere cannot join another company.
func oneCompany(ctx context.Context, st Store, userID, companyID string) error {
	ms, err := st.Memberships().ListByUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if m.Status == model.MembershipActive && m.CompanyID != companyID {
			return apperr.New(apperr.ErrConflict, "user_has_company", "the user already belongs to another company")
		}
	}
	return nil
}
