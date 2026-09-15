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
| TOTP | 30-second steps, six digits, at most previous/current/next step; atomically consume the accepted counter once per factor. Challenge lifetime 5 minutes, maximum 5 attempts; fresh challenge does not reset account limits. Secrets encrypted with externally supplied key material; plaintext only during enrollment. |
| Recovery codes | Ten independently random 128-bit codes, shown once; store digests, atomic one-use consumption. Replace the whole previous set when regenerated after fresh MFA. Codes authorize only the approved restricted recovery flow, never a new role or membership. |
| Recovery request | Uniform 202 regardless of account/delivery state; at most 3 deliveries per account per hour and 10 requests per IP per hour. No invalidation of existing credentials on request. |
| Password reset | 32-byte random one-use token, digest only, 15-minute lifetime, purpose/user/security-version bound. Send only to the account's previously verified email. Completion requires existing TOTP or a one-use recovery code when MFA is enrolled, then revokes all sessions and outstanding recovery challenges. Email possession alone cannot reset MFA. |
| Lost all factors | No support/operator bypass in this proposal. Recovery stays unavailable until a separate identity-proof procedure is approved. Password reset must not silently remove MFA. |
| Invitations | Verified target email plus a separate one-use, purpose-bound 32-byte token, 24-hour lifetime; accept only as the verified matching identity. Existing users authenticate, never silently link or reset their password. New credentialed provider-company provisioning keeps its already approved invitation exception. |
| Delivery | Injectable mail adapter, synthetic sink for development. Production delivery requires a separately configured verified sender/domain, provider credentials and delivery monitoring. None is selected or configured by this proposal. Delivery failure remains uniform to callers and retryable internally without exposing tokens. |

Numeric request budgets are proposed application settings. They are not values
mandated by the references and must be load-tested with shared counters across
instances. Throttle responses for known and unknown accounts must be equivalent;
no login/email/token in URLs, metrics labels, audit payloads or error receipts.

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
