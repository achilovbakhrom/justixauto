package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
)

type SessionConfig struct {
	IdleTimeout      time.Duration
	AbsoluteTimeout  time.Duration
	LockoutThreshold int
	LockoutDuration  time.Duration
}

var DefaultSessionConfig = SessionConfig{
	IdleTimeout:      30 * time.Minute,
	AbsoluteTimeout:  12 * time.Hour,
	LockoutThreshold: 5,
	LockoutDuration:  15 * time.Minute,
}

type AuthService struct {
	deps
	cfg SessionConfig
	box *secretBox // encrypts MFA secrets
	// mfaOff: two-factor authentication is switched off (Config.MFADisabled).
	mfaOff bool
}

var errInvalidCredentials = apperr.New(apperr.ErrUnauthenticated, "invalid_credentials", "login or password is incorrect")

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func tokenHash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// LoginResult is either a session (Token) or, for users with MFA, a pending
// challenge (ChallengeID + ChallengeToken for the challenge cookie).
type LoginResult struct {
	Token          string
	ChallengeID    string
	ChallengeToken string
}

// Login verifies credentials. Without MFA it starts a session; with MFA it
// creates a short-lived challenge that VerifyChallenge completes. A previous
// session of the same browser (previousToken) is revoked, so the cookie
// always rotates.
func (s *AuthService) Login(ctx context.Context, login, password, previousToken string) (*LoginResult, error) {
	now := s.clock()
	u, err := s.store.Users().FindByLogin(ctx, strings.TrimSpace(login))
	if errors.Is(err, apperr.ErrNotFound) || (err == nil && u.PasswordHash == nil) {
		_, _ = verifyPassword(password, dummyHash) // equalize timing
		return nil, errInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if u.LockedUntil != nil && u.LockedUntil.After(now) {
		return nil, &apperr.RateLimitedError{RetryAfter: u.LockedUntil.Sub(now)}
	}
	ok, err := verifyPassword(password, *u.PasswordHash)
	if err != nil {
		return nil, err
	}
	if !ok {
		if err := s.recordFailure(ctx, u); err != nil {
			return nil, err
		}
		return nil, errInvalidCredentials
	}
	if u.Status != UserActive {
		return nil, apperr.New(apperr.ErrForbidden, "account_suspended", "this account is suspended")
	}
	if u.MFAEnabledAt != nil && !s.mfaOff {
		// Failed-attempt counter resets only after the second factor, so
		// repeated logins cannot bypass the lockout for code guessing.
		token, err := randomToken()
		if err != nil {
			return nil, err
		}
		ch := &mfaChallenge{ID: uuid.NewString(), UserID: u.ID, TokenHash: tokenHash(token),
			CreatedAt: now, ExpiresAt: now.Add(mfaChallengeTTL)}
		if err := s.store.MFA().CreateChallenge(ctx, ch); err != nil {
			return nil, err
		}
		return &LoginResult{ChallengeID: ch.ID, ChallengeToken: token}, nil
	}
	if err := s.store.Users().SetLoginState(ctx, u.ID, 0, nil); err != nil {
		return nil, err
	}
	token, _, err := s.startSession(ctx, u, previousToken, nil)
	if err != nil {
		return nil, err
	}
	return &LoginResult{Token: token}, nil
}

// recordFailure counts a failed password or second factor and locks the
// account for a while after too many in a row.
func (s *AuthService) recordFailure(ctx context.Context, u *User) error {
	failed, lockedUntil := u.FailedLogins+1, (*time.Time)(nil)
	if failed >= s.cfg.LockoutThreshold {
		until := s.clock().Add(s.cfg.LockoutDuration)
		failed, lockedUntil = 0, &until
	}
	return s.store.Users().SetLoginState(ctx, u.ID, failed, lockedUntil)
}

// startSession creates a session and returns its raw cookie token, which is
// never stored. mfaAt marks a session that passed the second factor.
func (s *AuthService) startSession(ctx context.Context, u *User, previousToken string, mfaAt *time.Time) (string, *Session, error) {
	now := s.clock()
	token, err := randomToken()
	if err != nil {
		return "", nil, err
	}
	csrf, err := randomToken()
	if err != nil {
		return "", nil, err
	}
	sess := &Session{ID: uuid.NewString(), TokenHash: tokenHash(token), CSRFToken: csrf, UserID: u.ID,
		BranchScopeMode: ScopeAll, BranchIDs: []string{}, ContextRevision: 1, MFAAuthenticatedAt: mfaAt,
		CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(s.cfg.AbsoluteTimeout)}
	err = s.store.InTx(ctx, func(st Store) error {
		if previousToken != "" {
			if prev, err := st.Sessions().FindLive(ctx, tokenHash(previousToken)); err == nil {
				if err := st.Sessions().Revoke(ctx, prev.ID, now); err != nil {
					return err
				}
			}
		}
		return st.Sessions().Create(ctx, sess)
	})
	if err != nil {
		return "", nil, err
	}
	return token, sess, nil
}

// Authenticate resolves a cookie token into the caller. Expired, revoked or
// unknown sessions and inactive users return ErrUnauthenticated. A company
// context whose membership was revoked is ignored.
func (s *AuthService) Authenticate(ctx context.Context, token string) (*auth.Principal, *Session, error) {
	if token == "" {
		return nil, nil, apperr.ErrUnauthenticated
	}
	sess, err := s.store.Sessions().FindLive(ctx, tokenHash(token))
	if errors.Is(err, apperr.ErrNotFound) {
		return nil, nil, apperr.ErrUnauthenticated
	}
	if err != nil {
		return nil, nil, err
	}
	now := s.clock()
	if now.After(sess.ExpiresAt) || now.Sub(sess.LastSeenAt) > s.cfg.IdleTimeout {
		_ = s.store.Sessions().Revoke(ctx, sess.ID, now)
		return nil, nil, apperr.ErrUnauthenticated
	}
	u, err := s.store.Users().Get(ctx, sess.UserID)
	if err != nil {
		return nil, nil, err
	}
	if u.Status != UserActive {
		return nil, nil, apperr.ErrUnauthenticated
	}
	roles, err := s.store.Roles().UserRoles(ctx, u.ID)
	if err != nil {
		return nil, nil, err
	}
	p := &auth.Principal{UserID: u.ID, SessionID: sess.ID, Permissions: map[string]bool{},
		ContextRevision: sess.ContextRevision, BranchScope: auth.BranchScope{Mode: ScopeAll, BranchIDs: []string{}},
		MFAEnrolled: u.MFAEnabledAt != nil, MFARequired: s.mfaRequired(),
		MFAFresh:               sess.MFAAuthenticatedAt != nil && now.Sub(*sess.MFAAuthenticatedAt) <= mfaFreshness,
		PasswordChangeRequired: u.PasswordChangeRequired}
	for _, r := range roles {
		for _, perm := range effectivePermissions(r) {
			p.Permissions[perm] = true
		}
	}
	if sess.ActiveCompanyID != nil {
		member, err := s.isMember(ctx, s.store, u.ID, *sess.ActiveCompanyID)
		if err != nil {
			return nil, nil, err
		}
		if member {
			p.CompanyID = *sess.ActiveCompanyID
			p.BranchScope = auth.BranchScope{Mode: sess.BranchScopeMode, BranchIDs: sess.BranchIDs}
		}
	}
	if now.Sub(sess.LastSeenAt) > time.Minute {
		if err := s.store.Sessions().Touch(ctx, sess.ID, now); err != nil {
			return nil, nil, err
		}
	}
	return p, sess, nil
}

func (s *AuthService) Logout(ctx context.Context, sessionID string) error {
	return s.store.Sessions().Revoke(ctx, sessionID, s.clock())
}

// SessionView is the contract's GET /session data.
type SessionView struct {
	User                SessionUser         `json:"user"`
	Roles               []RoleRef           `json:"roles"`
	Permissions         []string            `json:"permissions"`
	MFA                 MFAView             `json:"mfa"`
	Context             ContextView         `json:"context"`
	AccessibleCompanies []AccessibleCompany `json:"accessibleCompanies"`
	Setup               SetupView           `json:"setup"`
}

type SessionUser struct {
	ID                     string     `json:"id"`
	DisplayName            string     `json:"displayName"`
	Status                 UserStatus `json:"status"`
	PasswordChangeRequired bool       `json:"passwordChangeRequired"`
}

type RoleRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type MFAView struct {
	Enrolled        bool       `json:"enrolled"`
	Disabled        bool       `json:"disabled"` // two-factor authentication is switched off on this server
	AuthenticatedAt *time.Time `json:"authenticatedAt,omitempty"`
}

type ContextView struct {
	Revision    string           `json:"revision"`
	CompanyID   *string          `json:"companyId"`
	BranchScope auth.BranchScope `json:"branchScope"`
}

type AccessibleCompany struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Kind   CompanyKind   `json:"kind"`
	Access CompanyAccess `json:"access"`
}

type SetupView struct {
	Next                      string `json:"next"` // company | branch | none
	PartnershipRequiredForB2B bool   `json:"partnershipRequiredForB2B"`
}

func (s *AuthService) View(ctx context.Context, p *auth.Principal, sess *Session) (*SessionView, error) {
	u, err := s.store.Users().Get(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	roles, err := s.store.Roles().UserRoles(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	memberships, err := s.store.Memberships().ListByUser(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	var companyIDs []string
	for _, m := range memberships {
		if m.Status == MembershipActive {
			companyIDs = append(companyIDs, m.CompanyID)
		}
	}
	companies, err := s.store.Companies().GetMany(ctx, companyIDs)
	if err != nil {
		return nil, err
	}
	view := &SessionView{
		User:                SessionUser{ID: u.ID, DisplayName: u.DisplayName, Status: u.Status, PasswordChangeRequired: u.PasswordChangeRequired},
		Roles:               make([]RoleRef, len(roles)),
		Permissions:         p.PermissionList(),
		MFA:                 MFAView{Enrolled: u.MFAEnabledAt != nil, Disabled: s.mfaOff, AuthenticatedAt: sess.MFAAuthenticatedAt},
		Context:             ContextView{Revision: revision(p.ContextRevision), BranchScope: p.BranchScope},
		AccessibleCompanies: make([]AccessibleCompany, len(companies)),
		Setup:               SetupView{Next: "none", PartnershipRequiredForB2B: true},
	}
	for i, r := range roles {
		view.Roles[i] = RoleRef{ID: r.ID, Name: r.Name}
	}
	for i, c := range companies {
		view.AccessibleCompanies[i] = AccessibleCompany{ID: c.ID, Name: c.Name, Kind: c.Kind, Access: c.Status}
	}
	if p.CompanyID != "" {
		id := p.CompanyID
		view.Context.CompanyID = &id
	}
	// Setup shows only the first missing prerequisite: company, then branch.
	switch {
	case len(companies) == 0:
		view.Setup.Next = "company"
	case p.CompanyID != "":
		branches, err := s.store.Branches().List(ctx, p.CompanyID)
		if err != nil {
			return nil, err
		}
		if len(branches) == 0 {
			view.Setup.Next = "branch"
		}
	}
	return view, nil
}

// SetCompany switches the working company (nil clears it) and resets the
// branch scope to ALL. The user must be an active member of the company.
func (s *AuthService) SetCompany(ctx context.Context, p *auth.Principal, expected int64, companyID *string) error {
	if companyID != nil {
		if err := validID(*companyID); err != nil {
			return apperr.FieldError("companyId", "must be a valid ID")
		}
		member, err := s.isMember(ctx, s.store, p.UserID, *companyID)
		if err != nil {
			return err
		}
		if !member {
			return apperr.New(apperr.ErrForbidden, "not_a_member", "you are not a member of this company")
		}
	}
	sess := &Session{ID: p.SessionID, ActiveCompanyID: companyID, BranchScopeMode: ScopeAll, BranchIDs: []string{}}
	return s.store.Sessions().UpdateContext(ctx, sess, expected)
}

// SetBranchScope narrows the working branches inside the active company. ALL
// requires an empty list; SELECTED requires branches the membership allows.
func (s *AuthService) SetBranchScope(ctx context.Context, p *auth.Principal, expected int64, scope auth.BranchScope) (auth.BranchScope, error) {
	if p.CompanyID == "" {
		return auth.BranchScope{}, apperr.New(apperr.ErrConflict, "no_active_company", "select a company first")
	}
	var v apperr.Validation
	ids := uniqueIDs(&v, "branchIds", scope.BranchIDs)
	switch scope.Mode {
	case ScopeAll:
		if len(ids) != 0 {
			v.Add("branchIds", "must be empty for ALL")
		}
	case ScopeSelected:
		if len(ids) == 0 {
			v.Add("branchIds", "select at least one branch")
		}
	default:
		v.Add("mode", "must be ALL or SELECTED")
	}
	if err := v.Err(); err != nil {
		return auth.BranchScope{}, err
	}
	if len(ids) > 0 {
		n, err := s.store.Branches().CountInCompany(ctx, p.CompanyID, ids)
		if err != nil {
			return auth.BranchScope{}, err
		}
		m, err := s.store.Memberships().Active(ctx, p.UserID, p.CompanyID)
		if err != nil {
			return auth.BranchScope{}, err
		}
		allowed := int(n) == len(ids)
		if m.BranchAccess == SelectedBranches {
			for _, id := range ids {
				allowed = allowed && slices.Contains(m.BranchIDs, id)
			}
		}
		if !allowed {
			return auth.BranchScope{}, apperr.FieldError("branchIds", "contains branches you cannot access")
		}
	}
	sess := &Session{ID: p.SessionID, ActiveCompanyID: &p.CompanyID, BranchScopeMode: scope.Mode, BranchIDs: ids}
	return auth.BranchScope{Mode: scope.Mode, BranchIDs: ids}, s.store.Sessions().UpdateContext(ctx, sess, expected)
}

func revision(v int64) string { return strconv.FormatInt(v, 10) }

// ChangePassword replaces the signed-in user's password after checking the
// current one. Other sessions end; this one stays signed in.
func (s *AuthService) ChangePassword(ctx context.Context, p *auth.Principal, current, next, confirmation string) error {
	u, err := s.store.Users().Get(ctx, p.UserID)
	if err != nil {
		return err
	}
	now := s.clock()
	if u.LockedUntil != nil && u.LockedUntil.After(now) {
		return &apperr.RateLimitedError{RetryAfter: u.LockedUntil.Sub(now)}
	}
	ok, err := verifyPassword(current, *u.PasswordHash)
	if err != nil {
		return err
	}
	if !ok {
		if err := s.recordFailure(ctx, u); err != nil {
			return err
		}
		return apperr.FieldError("currentPassword", "is incorrect")
	}
	var v apperr.Validation
	validatePassword(&v, "newPassword", next, confirmation)
	if next == current {
		v.Add("newPassword", "must differ from the current password")
	}
	if err := v.Err(); err != nil {
		return err
	}
	hash, err := hashPassword(next)
	if err != nil {
		return err
	}
	return s.store.InTx(ctx, func(st Store) error {
		u.PasswordHash, u.PasswordChangeRequired, u.UpdatedAt = &hash, false, now
		if err := st.Users().Update(ctx, u, u.Version); err != nil {
			return err
		}
		if err := st.Users().SetLoginState(ctx, u.ID, 0, nil); err != nil {
			return err
		}
		if err := st.Sessions().RevokeUser(ctx, u.ID, p.SessionID, now); err != nil {
			return err
		}
		return s.audit(ctx, st, p, "user.password_changed", "user", u.ID, nil, "", nil)
	})
}
