package identity

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/google/uuid"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
)

const (
	mfaFreshness      = 5 * time.Minute // sensitive actions need a second factor this recent
	mfaChallengeTTL   = 5 * time.Minute
	mfaEnrollmentTTL  = 10 * time.Minute
	mfaChallengeTries = 5
	recoveryCodeCount = 10
)

var errInvalidCode = apperr.New(apperr.ErrUnauthenticated, "invalid_mfa_code", "the two-factor code is incorrect or expired")

// mfaRequired lists the catalog permissions that need a fresh second factor.
func mfaRequired() map[string]bool {
	out := map[string]bool{}
	for _, p := range Catalog {
		if p.RequiresMFA {
			out[p.Key] = true
		}
	}
	return out
}

func recoveryHash(code string) []byte {
	h := sha256.Sum256([]byte(normalizeRecoveryCode(code)))
	return h[:]
}

// secondFactor checks a TOTP code (each step usable once) or an unused
// recovery code for the user.
func (s *AuthService) secondFactor(ctx context.Context, st Store, u *User, code string) (bool, error) {
	if u.MFAEnabledAt == nil {
		return false, nil
	}
	now := s.clock()
	if len(code) == totpDigits {
		secret, err := s.box.open(u.MFASecretEnc)
		if err != nil {
			return false, err
		}
		counter, ok := verifyTOTP(secret, code, now, u.MFALastCounter)
		if !ok {
			return false, nil
		}
		return st.MFA().UseCounter(ctx, u.ID, counter)
	}
	return st.MFA().UseRecoveryCode(ctx, u.ID, recoveryHash(code), now)
}

// Enrollment is returned once; the secret is never shown again.
type Enrollment struct {
	ID         string `json:"enrollmentId"`
	Secret     string `json:"secret"`
	OTPAuthURI string `json:"otpauthUri"`
}

// StartEnrollment creates a pending TOTP secret for the signed-in user.
func (s *AuthService) StartEnrollment(ctx context.Context, p *auth.Principal) (*Enrollment, error) {
	u, err := s.store.Users().Get(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	if u.MFAEnabledAt != nil {
		return nil, apperr.New(apperr.ErrConflict, "mfa_already_enrolled", "two-factor authentication is already set up")
	}
	secret, err := newTOTPSecret()
	if err != nil {
		return nil, err
	}
	sealed, err := s.box.seal(secret)
	if err != nil {
		return nil, err
	}
	now := s.clock()
	e := &mfaEnrollment{ID: uuid.NewString(), UserID: u.ID, SecretEnc: sealed, CreatedAt: now, ExpiresAt: now.Add(mfaEnrollmentTTL)}
	if err := s.store.MFA().CreateEnrollment(ctx, e); err != nil {
		return nil, err
	}
	account := u.Email
	if u.Login != nil {
		account = *u.Login
	}
	return &Enrollment{ID: e.ID, Secret: b32.EncodeToString(secret), OTPAuthURI: otpauthURI(secret, account)}, nil
}

// ConfirmEnrollment turns MFA on once the user proves the authenticator works.
// It returns single-use recovery codes (shown only now) and counts as a fresh
// second factor for the current session.
func (s *AuthService) ConfirmEnrollment(ctx context.Context, p *auth.Principal, enrollmentID, code string) ([]string, error) {
	if err := validID(enrollmentID); err != nil {
		return nil, err
	}
	codes, err := newRecoveryCodes(recoveryCodeCount)
	if err != nil {
		return nil, err
	}
	now := s.clock()
	err = s.store.InTx(ctx, func(st Store) error {
		e, err := st.MFA().Enrollment(ctx, enrollmentID, p.UserID)
		if err != nil {
			return err
		}
		if now.After(e.ExpiresAt) {
			return apperr.New(apperr.ErrConflict, "enrollment_expired", "start the setup again")
		}
		secret, err := s.box.open(e.SecretEnc)
		if err != nil {
			return err
		}
		counter, ok := verifyTOTP(secret, code, now, 0)
		if !ok {
			return errInvalidCode
		}
		if err := st.MFA().Enable(ctx, e, now, counter); err != nil {
			return err
		}
		hashes := make([][]byte, len(codes))
		for i, c := range codes {
			hashes[i] = recoveryHash(c)
		}
		if err := st.MFA().ReplaceRecoveryCodes(ctx, p.UserID, hashes); err != nil {
			return err
		}
		if err := st.Sessions().SetMFA(ctx, p.SessionID, now); err != nil {
			return err
		}
		return s.audit(ctx, st, p, "user.mfa_enrolled", "user", p.UserID, nil, "", nil)
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// StepUp re-confirms the second factor in the current session. Failures count
// toward the same lockout as wrong passwords.
func (s *AuthService) StepUp(ctx context.Context, p *auth.Principal, code string) error {
	u, err := s.store.Users().Get(ctx, p.UserID)
	if err != nil {
		return err
	}
	if u.MFAEnabledAt == nil {
		return apperr.New(apperr.ErrConflict, "mfa_not_enrolled", "set up two-factor authentication first")
	}
	now := s.clock()
	if u.LockedUntil != nil && u.LockedUntil.After(now) {
		return &apperr.RateLimitedError{RetryAfter: u.LockedUntil.Sub(now)}
	}
	ok, err := s.secondFactor(ctx, s.store, u, code)
	if err != nil {
		return err
	}
	if !ok {
		if err := s.recordFailure(ctx, u); err != nil {
			return err
		}
		return errInvalidCode
	}
	if err := s.store.Users().SetLoginState(ctx, u.ID, 0, nil); err != nil {
		return err
	}
	return s.store.Sessions().SetMFA(ctx, p.SessionID, now)
}

// VerifyChallenge completes a login that needs a second factor. The challenge
// token comes from the browser's challenge cookie, binding it to that browser.
func (s *AuthService) VerifyChallenge(ctx context.Context, challengeID, challengeToken, code, previousToken string) (string, *Session, error) {
	if uuid.Validate(challengeID) != nil || challengeToken == "" {
		return "", nil, errInvalidCode
	}
	ch, err := s.store.MFA().Challenge(ctx, challengeID)
	if errors.Is(err, apperr.ErrNotFound) {
		return "", nil, errInvalidCode
	}
	if err != nil {
		return "", nil, err
	}
	now := s.clock()
	if ch.ConsumedAt != nil || now.After(ch.ExpiresAt) || ch.Attempts >= mfaChallengeTries ||
		subtle.ConstantTimeCompare(ch.TokenHash, tokenHash(challengeToken)) != 1 {
		return "", nil, errInvalidCode
	}
	u, err := s.store.Users().Get(ctx, ch.UserID)
	if err != nil {
		return "", nil, err
	}
	if u.Status != UserActive {
		return "", nil, errInvalidCode
	}
	if u.LockedUntil != nil && u.LockedUntil.After(now) {
		return "", nil, &apperr.RateLimitedError{RetryAfter: u.LockedUntil.Sub(now)}
	}
	ok, err := s.secondFactor(ctx, s.store, u, code)
	if err != nil {
		return "", nil, err
	}
	if !ok {
		if err := s.store.MFA().FailChallenge(ctx, ch.ID); err != nil {
			return "", nil, err
		}
		if err := s.recordFailure(ctx, u); err != nil {
			return "", nil, err
		}
		return "", nil, errInvalidCode
	}
	if err := s.store.Users().SetLoginState(ctx, u.ID, 0, nil); err != nil {
		return "", nil, err
	}
	consumed, err := s.store.MFA().ConsumeChallenge(ctx, ch.ID, now)
	if err != nil {
		return "", nil, err
	}
	if !consumed {
		return "", nil, errInvalidCode
	}
	return s.startSession(ctx, u, previousToken, &now)
}
