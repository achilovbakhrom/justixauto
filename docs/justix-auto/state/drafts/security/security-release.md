# T-002 — security settings proposal

Date: 2026-09-15. Status: PROPOSED, not security acceptance. ADR-08/09 and
the cookie/identity contract remain approved; the values and delivery choices
below need explicit acceptance. This document does not enable recovery or a
production identity deployment.

## Proposed implementation configuration

| Setting | Proposed value and behavior |
|---|---|
| Session | Idle 30 minutes, absolute 12 hours; enforced by identity using server time. Sensitive-action MFA freshness 5 minutes. These retain the architecture's proposed values. |
| Session secret | 32 random bytes from crypto/rand; only digest stored. Cookie remains `__Host-justix_session`, Secure, HttpOnly, SameSite=Lax, Path=/, no Domain. Rotate on login/MFA/recovery and privilege changes. |
| CSRF | Session-bound 32-byte random token plus exact configured Origin checks on state-changing browser requests; no suffix or wildcard Origin match. Tokens are transient, excluded from events/logs/receipts. |
| Password input | 15–128 Unicode code points after consistent NFC normalization, maximum 1024 UTF-8 bytes before hashing. Permit spaces/paste/password managers; no composition rules or periodic forced changes. Reject known compromised/common passwords using a locally provisioned versioned blocklist. No password or hash sent to an external checking service. |
| Password hash candidate | Argon2id v19, 64 MiB, 3 iterations, 4 lanes, fresh 16-byte random salt and 32-byte output; store parameters with the hash. Validate record bounds before allocation, constant-time comparison, no automatic cost reduction under load. Final release suitability depends on target-resource benchmarks below. |
| Hash concurrency | Initial bounded capacity 4 concurrent hashes per identity instance; queue at most 16 for at most 2 seconds, then generic retry response. Configuration cannot exceed the measured deployment memory/CPU budget. This is an operational candidate, not a scaling guarantee. |
| Login throttling | Shared identity-owned counters: maximum 5 failed attempts per normalized login per rolling 15 minutes, and 30 per source IP per minute. Short cooldown, no indefinite account lock. Uniform unknown-login/wrong-password behavior; trusted proxy configuration controls source IP. |
| TOTP | 30-second steps, six digits, at most previous/current/next step; atomically consume the accepted counter once per factor. Challenge lifetime 5 minutes, maximum 5 attempts; fresh challenge does not reset account limits. Secrets encrypted with externally supplied key material; browser disclosure only during enrollment, transient server decryption for verification, no logging. |
| Recovery codes | Ten independently random 128-bit codes, shown once; store digests, atomic one-use consumption. Replace the whole previous set when regenerated after fresh MFA. Codes authorize only the approved restricted recovery flow, never a new role or membership. |
| Recovery request | Uniform 202 regardless of account/delivery state; at most 3 deliveries per account per hour and 10 requests per IP per hour. No invalidation of existing credentials on request. |
| Password reset | 32-byte random one-use token, digest only, 15-minute lifetime, purpose/user/security-version bound. Send only to the account's previously verified email. Completion requires existing TOTP or a one-use recovery code when MFA is enrolled, then revokes all sessions and outstanding recovery challenges. Email possession alone cannot reset MFA. |
| Lost all factors | No support/operator bypass in this proposal. Recovery stays unavailable until a separate identity-proof procedure is approved. Password reset must not silently remove MFA. |
| Invitations | Only an already credentialed identity with a previously verified matching email may accept the one-use 32-byte token, valid 24 hours. Pending-user credential setup/initial email verification remains separately gated. No silent linking/password reset; new credentialed provider-company provisioning retains its approved invitation exception. |
| Delivery | Injectable mail adapter, synthetic sink for development. A bounded encrypted identity-private delivery record permits retry of the same token; see the proposed handoff below. Production sender/domain, provider credentials and monitoring remain unconfigured. Delivery failure remains uniform to callers. |

Numeric request budgets are proposed application settings. They are not values
mandated by the references and must be load-tested with shared counters across
instances. Throttle responses for known and unknown accounts must be equivalent;
no login/email/token in URLs, metrics labels, audit payloads or error receipts.

## Proposed restricted recovery contract delta — unapproved

The existing `recovery/complete {token,newPassword,confirmation}` route does not
carry factor proof. The following explicit additions are proposed for T-058
contract review and user acceptance; they are not silently part of the approved
HTTP baseline. Until accepted, recovery remains disabled even if session limits
are approved. All paths below use `/api/v1/identity`.

1. `GET /session/recovery/challenge` issues a fresh, opaque restricted
   `__Host-justix_recovery` cookie (Secure/HttpOnly/SameSite=Lax/Path=/) and
   `{data:{csrfToken}}`. It discloses no account information and grants no login
   session. The 15-minute server record stores cookie/token digests and purpose
   `password-reset`. Bound creation throttles prevent unbounded allocation.
2. `POST /session/recovery/request {login}` requires that cookie, exact Origin
   and `X-CSRF-Token`. It returns uniform 202. For an eligible account, identity
   binds a fresh reset-token digest to this browser challenge, user and security
   version. Unknown/ineligible accounts use an indistinguishable public flow.
   Repeating a delivery attempt reuses its token; a new accepted user request
   revokes older reset authorities for that account. Concurrent requests serialize.
3. `POST /session/recovery/proof` accepts exactly
   `{token,proof:{kind:"totp"|"recovery-code",code}}`, or
   `{token,proof:{kind:"none"}}` for an authoritatively non-enrolled account.
   Cookie, Origin and CSRF are required. Valid proof returns 204 and records an
   identity-private restricted grant bound to challenge, token digest, user,
   purpose, current security version and the exact factor counter/code digest.
   It expires at the earlier of token expiry and five minutes after proof.
   It authorizes only password-reset completion; it is never an ordinary session.
   Proof verifies a currently acceptable unused TOTP counter/recovery code but
   final consumption occurs in the completion transaction. No codes enter the
   grant record, generic command receipts, events or logs.
4. `POST /session/recovery/complete {token,newPassword,confirmation:true}` keeps
   the approved field set and requires the same restricted cookie/Origin/CSRF
   and grant. Validate password/confirmation before consuming anything. In one
   identity transaction lock/recheck challenge, token, grant, credential and
   factor security version; require unused proof and current validity; consume
   the exact TOTP counter/recovery code and reset token; replace the credential;
   increment security version; revoke all sessions and recovery grants. Return
   204 and clear the restricted cookie. No auto-login or role/membership change.

Wrong/reused/expired proof or changed security version returns a generic 403
recovery-invalid receipt; malformed input is 400 and password validation is
422. No account-existence or factor-specific diagnostic is returned. Concurrent
completion has one winner; a TOTP/code consumed by another operation invalidates
the pending grant rather than allowing replay. A lost completion response stays
unknown to the browser: it offers login with the new password or a fresh recovery
request; it must not infer rollback from a replay rejection. No reusable business
idempotency receipt is added to authentication flows.

A recovery code in this proposal permits password reset only. It does not remove
MFA or authorize re-enrollment of a lost factor. That separate proof/re-enrollment
flow remains gated, as does recovery after losing all factors. User acceptance
must explicitly cover this limited recovery scope.

## Proposed secret delivery and invitation handoff

Email carries a manual copy/paste code (base64url encoding of the 32-byte token)
and a fixed public page URL containing no identity or token. The user completes
recovery in the initiating browser. No token appears in a query string, fragment,
analytics or referrer. Cross-device restart means a fresh user-initiated request.

The authorization record remains digest-only. A separate identity-private,
short-lived delivery job holds authenticated ciphertext of recipient and token,
encrypted with an externally supplied versioned key. Associated data binds the
job ID, purpose, user, challenge/invitation ID, security version and expiry.
Creation of token authority and delivery job is atomic. This is an explicit
proposal to retain recoverable encrypted token material for delivery; it is not
a claim that a digest can reconstruct a token. Generic event/outbox records
carry no token, recipient or ciphertext. The private worker rechecks current
authority/expiry before sending, and retries the same job and same token after
lost acknowledgements. Duplicate emails therefore confer the same one-use
authority. Confirmed delivery destroys ciphertext; expiry/revocation cancels the
job and destroys ciphertext. A crash after send may produce a duplicate before
acknowledgement; no exactly-once email guarantee is claimed. No automatic
replacement token is minted on a retry. Encrypted storage/keys and provider
configuration are required before enabling real delivery.

Invitations bind token digest to invitation ID, company, target verified email,
purpose and expiry; the private delivery job follows the same retry rules.
`POST /invitations/accept {token}` requires the authenticated credentialed user
whose independently verified email matches the target, plus normal Origin/CSRF.
Identity atomically rechecks pending status, expiry/revocation, user/email and
invitation/company binding, consumes the invitation and creates its authorized
membership. Concurrent acceptance yields one effect; no actor/body company
substitution or account linking. Existing command retry semantics stay as defined
by the feature contract and never store the bearer token in a generic receipt.
Pending users without credentials or independently verified email cannot accept
under this proposal; their initial activation protocol must be separately approved.

## Initial local benchmark evidence

Synthetic `argon2.IDKey` calls used the already locked x/crypto v0.57.0 and
Go 1.27.1 on darwin/arm64 (10 logical CPUs), with 64 MiB and three iterations.
There were 12 samples per condition, four calls per batch in concurrent runs.
Only the benchmark used a constant synthetic password/salt; production salts
must use crypto/rand. These measurements are a small local screening experiment.

| Lanes | Concurrent calls | Median | Maximum | Throughput |
|---|---:|---:|---:|---:|
| 4 | 1 | 36.794 ms | 44.967 ms | 25.69/s |
| 4 | 4 | 93.514 ms | 120.797 ms | 37.95/s |
| 1 | 1 | 136.899 ms | 138.440 ms | 7.31/s |
| 1 | 4 | 148.655 ms | 151.399 ms | 26.92/s |

These results do not establish p95/p99, peak RSS, constant-time verification,
malformed-record rejection, unknown-account equivalence or Linux behavior.
The dependency lock requires hash/verify p50/p95/p99, allocations/RSS and CPU
measurements under actual linux/amd64 and linux/arm64 limits. Those remain a
release gate. No production hardware/resource budget has been supplied.

## Acceptance record and remaining gates

- User acceptance of proposed implementation values: pending.
- Independent review of proposal against ADR-08/09 and HTTP contract: pending.
- Delivery provider, verified sender and target resource limits: not supplied.
- Full target benchmark/abuse tests: not run; remain required before release.
- Recovery after loss of all factors: no approved identity-proof procedure.
- No approval is inferred from the user's instruction to continue development.

Approve implementation settings separately from enabling production delivery.
Explicit acceptance can unblock deterministic implementation using synthetic
delivery while deployment-dependent checks and unapproved recovery stay gated.
T-002 must not be labelled fully complete while its required acceptance is absent.

## Primary reference checks

Checked 2026-09-15. Recommendations above are this project's proposal; these
sources support the mechanisms, not the product-specific timeouts or budgets.

- [OWASP Password Storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html): Argon2id and parameter guidance.
- [NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html): password length, blocklists and rate limits.
- [OWASP Forgot Password](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html): uniform recovery responses and expiring single-use tokens.
- [OWASP Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html): server-side session lifetime enforcement.
- [RFC 9106](https://www.rfc-editor.org/rfc/rfc9106.html): memory-constrained Argon2id candidate already selected for measurement in the approved dependency lock.
