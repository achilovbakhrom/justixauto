# T-058 — Authentication contract v1 (proposal)

Date: 2026-09-15. Status: **proposed bounded contract; independent QA and coordinator
promotion pending**. This document does not approve production security settings,
enable recovery/invitation delivery, or complete B-02.

## 1. Authority, scope and explicit refinements

Confirmed sources: [architecture §§6–8](../../../architecture.md),
[HTTP/domain §2](../../../contracts/http-domain.md),
[cross-owner §1](../../../contracts/cross-owner.md),
[B-02](../../../dev/backlog.md), [business rules §3](../../../business-logic.md),
[OD-12](../../../open-decisions.md), and the approved dependency lock's Argon2
benchmark plan. [Gaze](../../../reference/gaze-reference.md) supplies the
hexagonal/CQRS structure; [Gaze CC](../../../reference/gaze-executor-cc-reference.md)
does not supply reliable auth, atomic publication, snapshots or replay guards.
The [HTML map](../../../mock-map.md) supplies four app entry anchors, not a
production sign-in/enrollment/recovery design or authentication policy.

Identity alone owns credentials, sessions, factors, challenges, recovery material
and bootstrap guard. Edge transports cookies and strips forged internal identity
headers; it has no business database. Other owners call live authorization and
enforce their own object/party/state rules. No shared mutable auth tables.

This supplement covers `/api/v1/identity/session` and its auth children plus the
deployment-only bootstrap input. Session context/branch writes remain the B-03/B-04
contract; their response view is specified here for reuse. Provider provisioning,
users/roles/memberships and invitations remain their assigned contracts. Their
secret exclusions and delivery gates apply here; this is not a full Identity rewrite.

The following **proposed technical refinements need explicit coordinator adoption
with this supplement** before T-059 encodes them. They resolve gaps in the approved
mechanisms, not the outstanding recovery proof or security policy:

| Ref | Refinement |
|---|---|
| A1 | `GET /session` can return anonymous `401` while issuing an anonymous anti-CSRF cookie and `X-CSRF-Token` response header; the header is also available on authenticated/restricted session reads. No new CSRF endpoint. |
| A2 | Login adds the restricted `required:"mfa-enrollment"` response for the deployment-bootstrap subject already marked pending enrollment. This cannot activate an arbitrary pending user. |
| A3 | Add `POST /session/mfa/challenges {}` to start a session-bound step-up without resending the password. It requires an authenticated active subject with an enrolled factor; it grants nothing until verification. |
| A4 | Preserve verify's `{challengeId,code}` shape. The private challenge captures allowed proof kinds; the configured TOTP and recovery-code syntaxes must be disjoint. A one-use recovery code is an already enrolled factor alternative, never an all-factor-loss bypass. |
| A5 | Type the baseline `secretOrChallenge` as a TOTP provisioning object and define the one-time recovery-code response at enrollment confirmation. Neither response is replayable from a receipt or generic log. |

If one refinement is not adopted, the affected wire path remains blocked; a
generator must not silently substitute an endpoint or schema. Ordinary session,
credential and factor storage can still follow the approved mechanisms. T-002
proposal `a262f075bd5338c03c7049141c6c9f7e674110e5` is **unapproved**; none of its
values, proof protocol or delivery adapter is incorporated by reference.

## 2. Encoding and common schemas

All schemas below are closed objects (`additionalProperties:false` recursively),
including nested responses and event payloads. Properties are required unless
marked `?`; omitted optional properties are absent, never `null`. Only
`SessionContext.companyId` explicitly permits null. An absent body differs from
`{}`; empty-body requests shown as `{}` require that JSON object. Duplicate JSON
members, malformed UTF-8/JSON, trailing JSON values and wrong media type reject
before use. No coercion of numbers, booleans or arrays into strings.

```text
ID = canonical lower-case nonzero UUID string
Revision = /^(0|[1-9][0-9]*)$/, <= 9223372036854775807, encoded as JSON string
PositiveRevision = Revision excluding "0"
Instant = RFC3339 UTC string with Z, finite timestamp; never browser-local time
Text = nonempty string, with input bounds from the versioned field policy
Secret = nonempty string, transient secret input; no trim/case-fold/normalization
ProofCode = nonempty string matching exactly one allowed configured proof syntax
PermissionKey = nonempty exact catalog key; no wildcard expansion
Scope = {mode:"ALL",branchIds:[]} |
        {mode:"SELECTED",branchIds:ID[]} // nonempty, unique, authorized branches
ErrorResponse = {error:{code:ErrorCode,message:Text,
                 fields:record<string,Text>,traceId:ID},operationId?:ID}
```

`record` is an explicit map exception, not permission for unknown fields in an
object. Error `fields` keys are known request field paths and values are safe
validation messages, never submitted values. `message` is display text, not a
machine discriminator. `operationId` is omitted from auth errors; no receipt
operation is created. An opaque `traceId` is diagnostic only and cannot recover
credentials or a session. Reject unknown query parameters on auth routes.

Revisions `"01"`, `"+1"`, `"-0"`, `"1.0"`, `"1e3"`, number `1` and values above
signed bigint are invalid. Auth has **no Money, price, currency, rate or decimal
amount fields**; adding them is an unknown-field error. Epochs, durations and TOTP
counters do not appear as browser authority. Timestamps are server observations.

Login lookup uses the same versioned login canonicalizer as credential creation;
it cannot choose different Unicode/case/whitespace rules at lookup. Its policy
identifier and bounds are mandatory deployment inputs. Passwords and proofs are
compared as supplied; normalization of the login never changes the password.
Empty secret fields are invalid; spaces in a password are not silently trimmed.
`reason` must contain non-whitespace text and satisfy its configured bound.

## 3. Session view and auth result variants

```text
SessionContext = {revision:Revision,companyId:ID|null,branchScope:Scope}
SessionView = {
  user:{id:ID,displayName:Text,status:"active"},
  roles:[{id:ID,name:Text}], permissions:PermissionKey[],
  mfa:{enrolled:boolean,authenticatedAt?:Instant},
  context:SessionContext,
  accessibleCompanies:[{id:ID,name:Text,kind:"seller"|"bank"|"mfo"|"insurance",
                        access:"draft"|"active"|"suspended"}],
  setup:{next:"company"|"branch"|"none",partnershipRequiredForB2B:true}
}
SessionRead = {data:SessionView,revision:Revision,asOf:Instant}
ChallengeResult = {data:{challengeId:ID,required:"mfa"},
                   revision:"0",operationId:ID}
EnrollmentRequired = {data:{challengeId:ID,required:"mfa-enrollment"},
                      revision:"0",operationId:ID}
EnrollmentSecret = {enrollmentId:ID,secretOrChallenge:{kind:"totp",
                    secret:Secret,provisioningUri:Secret}}
EnrollmentConfirmed = {data:{enrolled:true,recoveryCodes:Secret[]},
                       revision:PositiveRevision,operationId:ID}
RecoveryAccepted = {data:{status:"pending"},revision:"0",operationId:ID}
```

Arrays of roles, permissions and accessible companies are unique by key/ID;
they may be empty. `branchIds` are unique. Session outer revision equals
`context.revision`, including login and successful verification responses.
When companyId is null, scope is exactly ALL/[]; SELECTED cannot exist without a
company. An authenticated session does not promise a company has any capability.
EnrollmentConfirmed recoveryCodes is nonempty and unique; exact count and syntax
come from the validated fixture or approved deployment policy, never this example.
An empty accessibleCompanies list is valid; it creates no demo data or privileges.
`setup.next` derives from approved setup rules; no mandatory partnership for all
retail users. Suspended-company obligations and disputed capability/compliance
combinations remain OD-05/11. A displayed entry is not admission to that workspace.

`authenticatedAt` is absent if no accepted MFA proof remains on this session;
it must not be present with `enrolled:false`. Freshness is evaluated server-side
using the approved deployment policy, not a client comparison or an auth event.
Pending/suspended users never receive a full SessionView. Restricted bootstrap
enrollment is represented by EnrollmentRequired and cannot expose roles,
permissions, companies or protected pages before completion. General pending-user
activation is a separate unresolved delivery/proof gate.

These auth handshakes have **no allowedActions field** and are explicit exceptions
to business resource receipts. The endpoint/state matrix is the closed action
catalog for the handshake. Session permissions are advisory UI inputs; every
business resource's allowedActions and actual command require current authority.
`operationId` in challenge/enrollment/recovery responses preserves the baseline
envelope convention but is only a random correlation ID. It is never a generic
operation-ledger row or a credential retrieval/status endpoint.

## 4. HTTP transport and endpoint matrix

All responses, including errors and one-time secrets: `Cache-Control: no-store`.
No bearer token/session handle in JSON, URL, redirects or browser persistent
storage. Successful full authentication uses only
`__Host-justix_session=<opaque>; Secure; HttpOnly; SameSite=Lax; Path=/` with no
Domain attribute. Restricted challenge sessions use a distinct
`__Host-justix_challenge` cookie with the same attributes and narrower server-side
route permissions; an anonymous CSRF binding uses `__Host-justix_csrf` with the
same attributes. Cookie clearing uses the same attributes/path and Max-Age=0.
Opaque cookie entropy/lifetime are deployment policy, not fixture constants.

A1 proposes a synchronizer token returned as `X-CSRF-Token`, held only in memory,
and bound server-side to the anonymous/restricted/full cookie as applicable.
`GET /session` issues it on anonymous 401, restricted challenge 200 or full 200;
these responses disclose no credential. Anonymous issuance never grants a
session. On rotation issue a new CSRF token; stale tokens cannot mutate. No
cross-origin CORS exposure of this header. All unsafe browser requests, including
login, verification, enrollment, recovery and logout, require exact allowlisted
Origin **and** matching `X-CSRF-Token`. Missing/null/mismatched Origin or CSRF
returns 403 before credential work. SameSite alone is insufficient. Nonbrowser
bootstrap does not call these public routes.

No business `Idempotency-Key` or `If-Match` is accepted on the auth handshake
routes below (unexpected values reject 400); CSRF/challenge/replay guards apply
instead. Context/branch-scope writes retain their existing session If-Match and
`X-Context-Revision` contract; this exception does not weaken business commands.

| Method / suffix after `/api/v1/identity` | Exact body; admissible authority | Success and state change |
|---|---|---|
| GET `/session` | No body; anonymous, restricted or full cookie | Anonymous401 ErrorResponse `SESSION_REQUIRED` plus A1 header; restricted200 ChallengeResult/EnrollmentRequired; full200 SessionRead. No idle/absolute expiry bypass. |
| POST `/session/login` | `{login:Text,password:Secret}`; anonymous CSRF binding | Active eligible user without enrolled MFA:200 SessionRead + fresh full cookie. Enrolled user:200 ChallengeResult + restricted cookie, no full admission. Bootstrap pending-enrollment subject:200 EnrollmentRequired. Wrong/unknown/suspended or ineligible pending identity:401 `AUTHENTICATION_FAILED`, no enumeration. Never trust body actor/company/role. |
| POST `/session/mfa/challenges` (A3) | `{}`; live full session, enrolled factor | 200 ChallengeResult, one-use challenge bound to that user/session/security version and purpose `step-up`. Existing full session remains at its previous assurance; response alone raises none. |
| POST `/session/mfa/verify` | `{challengeId:ID,code:ProofCode}`; matching restricted or full cookie | 200 SessionRead; atomically consume challenge and TOTP counter or recovery code, rotate cookie/security session binding, record MFA time. Old cookie rejected. No target-user input or changed role/membership. |
| POST `/session/logout` | `{}`; matching CSRF binding even for a recently revoked session | 204 no body; revoke presented session/challenge, clear corresponding cookies. Repeating revocation has no extra effect; missing/invalid CSRF does not become success. Does not cancel durable admitted business operations. |
| POST `/session/revoke-all` | `{reason:Text}`; live full subject, recent primary reauthentication and fresh MFA | 200 SessionRead; revoke other sessions, rotate current cookie and CSRF atomically. No targetUserId; admin revoke-user is a separate contract. Freshness thresholds are mandatory policy inputs. |
| POST `/session/mfa/enrollment` | `{}`; restricted authenticated bootstrap enrollment intent | 200 EnrollmentSecret once; create one pending factor bound to subject/challenge and security version. Repeated/lost-response attempt cannot retrieve prior secret; new enrollment explicitly replaces the unconfirmed attempt and invalidates its confirmation capability. |
| POST `/session/mfa/enrollment/{id}/confirm` | `{code:ProofCode}` where only configured TOTP syntax allowed; matching enrollment cookie/intent | 200 EnrollmentConfirmed once, fresh full cookie only if the original bootstrap activation conditions still hold. Atomically verify unused counter, enroll factor, consume intent, create private recovery-code digests, update security version and release pending bootstrap enrollment. Response revision is security revision, not context revision; client GET /session obtains context. |
| POST `/session/recovery/request` | `{login:Text}`; anonymous CSRF; accepted recovery deployment capability | Uniform202 RecoveryAccepted for known/unknown/ineligible login; never an existence hint or public delivery token. No state transition granting access. |
| POST `/session/recovery/complete` | `{token:Secret,newPassword:Secret,confirmation:Secret}`; anonymous CSRF and separately approved proof binding | 204 no body; atomically consume proof/token, replace credential, increment security version and revoke sessions. Password equals confirmation exactly. No login, role, MFA bypass, membership or company change. Invalid/reused/expired proof receives generic401 `AUTHENTICATION_FAILED`. |

Revoke-all primary reauthentication uses the existing login flow for the same
subject; switching identity starts a separate login and cannot satisfy the old
subject's reauthentication. Private server timestamps, never a request boolean,
prove primary authentication. An enrolled user completes the new login's MFA
challenge. A3 is sufficient for ordinary sensitive step-up where primary
reauthentication is not separately required by the approved action catalog.

General factor enrollment/replacement/removal, recovery-code regeneration and
pending-user activation are **not newly exposed APIs** in this slice. The narrow
bootstrap enrollment above implements the approved bootstrap mechanism. Other
enrollment eligibility requires a separately adopted security/credential-action
contract; no active user gains a general enrollment bypass from A2/A5.

No arbitrary redirect/return URL input is accepted. App routing is the registered
four-app route contract and live workspace admission. T-054 boundary states and
T-361 auth design remain independent visual gates; no Figma prerequisite.

### Exact errors and outcome uncertainty

| Status | ErrorCode | Meaning |
|---|---|---|
| 400 | `INVALID_REQUEST` | Malformed JSON, unknown field/query/header, wrong type or representation; no submitted secret in fields. |
| 401 | `SESSION_REQUIRED`, `SESSION_EXPIRED`, `AUTHENTICATION_FAILED` | Missing/expired full session or generic rejected credential/proof. No unknown-user/suspended-user distinction on login/recovery. |
| 403 | `ORIGIN_REJECTED`, `CSRF_REJECTED`, `FORBIDDEN`, `MFA_REQUIRED`, `POLICY_UNRESOLVED` | Transport binding, known accessible action denial, insufficient assurance or unavailable approved policy. |
| 404 | `NOT_FOUND` | Foreign/nonexistent enrollment/challenge reference where resource hiding applies; no subject disclosure. Wrong submitted login/proof stays generic401. |
| 409 | `AUTH_STATE_CONFLICT` | Accessible conflicting/consumed enrollment state; no old one-time secret or success receipt replay. |
| 412 / 428 | `STALE_REVISION` / `PRECONDITION_REQUIRED` | Retained context-write contract only; handshake does not accept client revisions. |
| 422 | `VALIDATION_FAILED` | Known structurally valid field fails configured bound, syntax or confirmation rule. |
| 429 | `RATE_LIMITED` | Policy-controlled rate/concurrency limit; bounded Retry-After, never automatic auth client retry. |
| 503 | `AUTHORITY_UNAVAILABLE`, `AUTH_OUTCOME_UNKNOWN` | Identity/private storage/required policy dependency unavailable, or commit outcome cannot be established. Never guessed success or automatic replay of a secret. |

Challenge validity, allowed proof kind, session/security binding and replay state
are checked even for repeated verification. A rejected TOTP can never be tried as
an arbitrary recovery code. TOTP consumption is unique per factor+counter across
all challenges; recovery code consumption is unique per stored digest. Expired
proof/counter state must not be erased merely to make a replay succeed.

Persist private auth changes, relevant nonsecret security event and synchronous
revocation/last-admin guards in one Identity transaction. No broker/network call
inside it. A challenge that loses a successful response cannot be used to recover
the rotated bearer secret. Re-read /session if the browser received the cookie;
otherwise restart authentication. Lost enrollment-secret response needs a new
unconfirmed attempt. Lost confirmation response may lose the one-time recovery
codes; an enrolled user can sign in with TOTP, but this contract grants no secret
retrieval/regeneration route. Lost recovery-completion reply permits a fresh login
with the submitted new password, not generic ledger lookup or token reuse. All
these clients preserve uncertainty until an authoritative auth state establishes
it; they never copy the business receipt retry loop onto auth.

## 5. Private state and safe events

T-059 owns auth-only schema/migration generation. The private model must retain:
Credential(subjectId, encoded Argon2id record, parameterPolicyId, securityVersion);
Session(tokenDigest, subjectId, state, context revision/scope, issued/idle/absolute
expiry, primaryAuthenticatedAt, mfaAuthenticatedAt, security binding);
Challenge(token/session digest binding, subject, purpose, allowed proof kinds,
security version, expiry, consumed state);
MfaFactor(encrypted secret, encryption key version, accepted counter guard,
pending/active state); RecoveryCode(keyed digest, subject/factor binding, usedAt);
RecoveryProof(typed accepted-proof reference, digest, purpose, expiry, consumed
state); BootstrapGuard(singleton consumption and pending enrollment subject).
These descriptions are ownership constraints, not permission to share schemas or
freeze key sizes, lifetimes, retention or accepted recovery proofs.

Private auth records are the approved exception to publishing secret state as
domain history. Replay of safe events reconstructs only security metadata; it
cannot reconstruct credentials, factors, bearer material or session authority.
No event consumer or projection can mint/restore credentials or authorize a new
command. Local locks/checks enforce guards, never eventual projections. The
last-active-MFA-admin singleton is locked before effective-admin computation;
self blocking/removal is separately denied. General factor-removal policy remains
outside this slice, but no private adapter may bypass that guard.

Proposed single closed auth metadata event schema:

```text
EventType = "identity.auth.security-changed.v1"
schemaVersion = 1
aggregateType = "auth-security"
aggregateId = subject user ID
companyId = null // global identity security, not a company event
data = {userId:ID,securityRevision:PositiveRevision,
        change:"credential-created"|"credential-replaced"|"mfa-enrolled"|
               "sessions-revoked"|"bootstrap-enrollment-started"}
```

Use the existing `pkg/events` envelope unchanged: eventId,
eventType/schemaVersion, aggregateType/id/version, integrationSequence,
companyId, occurredAt, actor, correlationId, causationId, operationId and data.
Owner identity is derived from eventType by the codec; **no extra wire `owner`
property** is added.
All IDs are nonzero canonical UUIDs; revisions use their existing string codecs.
`data.userId` equals aggregateId. `securityRevision` equals the committed private
subject security version and advances once per security transition; it need not
equal the independently numbered auth-security aggregateVersion. No event for an
uncommitted/rejected proof. Several safe transitions in one owner transaction
must retain normal aggregate revision and integration sequence rules.

This event is **a proposed allowlisted integration fact, not automatic broadcast**.
Registration requires an explicit approved recipient plan/custody contract and
authorized high-water/bootstrap policy. An explicitly authorized empty plan can
mean no external recipients; absent configuration cannot mean broadcast or an
implicit empty plan. No target service or personal bootstrap is invented here.
Until such admissions exist, security changes may persist owner-local safe events
without claiming an integrated external auth subscriber. Login, CSRF rotation,
context changes, challenge creation/failed attempts and TOTP/recovery-code use
need private security/audit mechanics, not new public broker payloads.

Never put passwords, password confirmations, credential hashes/salts/parameters,
TOTP secrets/codes/counters, provisioning URIs, recovery codes/tokens/digests,
session/challenge handles/digests, CSRF values, delivery bodies, email/login,
arbitrary reason text, IP/user-agent or raw request data in these events. No
generic receipt stores them or their raw/unkeyed/keyed secret-body hash. Safe
audit uses actor/subject IDs, action/outcome category and server time under its
own read authorization; diagnostic errors/logs exclude credentials and request
bodies. Sensitive auth responses must also be excluded from tracing/HTTP capture.

Provider provisioning's separate business command may store keyed HMAC of only
normalized **nonsecret** input and safe outcome IDs. Same key/nonsecret input
plus matching supplied password verifies against its created private credential;
mismatch is generic409 and never password reset. Authorized safe receipt lookup
can recover IDs after credential change; it cannot recover credentials. No
password/MFA-containing body is sent through a generic idempotency hash helper.

## 6. Deployment policy inputs and scoped unresolved gates

Deployment-only bootstrap command input is proposed as the closed object
`{login:Text,email:Text,displayName:Text,password:Secret,confirmation:Secret}`
read from protected stdin or injected secret input, never command-line arguments,
public HTTP or checked-in configuration. Email syntax/canonicalization uses the
same accepted Identity profile policy, and password equals confirmation exactly.
No input roleIds, actorId, companyId or `mfaVerified` flag. A typed deployment
authorization capability is separately injected; the object cannot grant itself
deployment authority. Under the singleton guard, atomically create the unique
pending bootstrap subject/credential, the explicitly configured protected
platform-admin role assignment and pending MFA enrollment intent. Do not use an
arbitrary caller-provided role or a public default account. Result is the safe
closed object `{userId:ID,status:"pending-mfa-enrollment"}` with no token, password
or provisioning secret. Login follows A2. Repeat bootstrap rejects already-used
guard and cannot reset a password; a lost result needs a deployment-authorized
safe guard read, not rerunning bootstrap as an update. No public guard-read route
is added. Expired/abandoned bootstrap recovery remains a separately authorized
operator/security procedure, not an automatic guard reset.

Mechanics accept a validated, immutable, versioned `AuthDeploymentPolicy`, with
typed components for session idle/absolute lifetimes; primary/MFA freshness;
challenge/anonymous-CSRF/enrollment lifetimes; random material sizes; Argon2id
parameter version and accepted verification bounds; TOTP algorithm/digits/period/
window; disjoint recovery-code syntax/entropy/count; login/secret field bounds
and canonicalization; rate-limit/concurrency/backoff controls; secret protection
key references; exact allowed browser origins; credential/digest rotation and
retention policy. Durations/counts are typed positive bounded values, never
unvalidated strings or body input. No runtime zero/unset fallback.

Readiness rejects an enabled capability missing its required validated policy or
key material. Synthetic tests supply an explicitly named `fixture-only` policy;
the fixture policy is not a deployment default. T-002/security acceptance must
select release values and validate actual-target Argon2 resource/latency limits.
This supplement fixes neither idle30m/absolute12h/MFA5m nor any Argon2 cost,
TOTP window, recovery-code count, password rule or delivery transport.

Recovery and invitation adapters accept a typed deployment capability:
`Disabled` or `Accepted {policyVersion,acceptanceRef,proofAdapter,deliveryAdapter}`.
These are server construction inputs, not public JSON and not self-asserted
request flags. Accepted references must resolve to separately recorded security
acceptance with configured proof, sender/origin, key material, lifetime and
delivery failure rules. A proposal path is not an accepted reference. Disabled
recovery endpoints return uniform403 POLICY_UNRESOLVED for all logins and perform
no account lookup/delivery. When enabled, request remains uniform202; adapter
failure must not reveal account existence or imply completed delivery.

Invitation wire bodies already approved remain `{email}`, `{token}` and
`{reason}` on their existing routes; T-058 adds none. Acceptance requires verified
matching identity, grants membership only, never overwrites/links an unverified
existing user, and never returns bearer tokens in admin receipts. Exact accepted
proof/delivery semantics, all-factor-loss handling and pending-user activation
remain unresolved OD-12/T-002/T-063 gates. They block those enabled flows, not
T-060/T-061/T-062 synthetic mechanics. Company suspension/capability policy
remains OD-05/11; no authority is inferred from organization kind/activation.

No financial policy is selected: fresh MFA does not approve a payment, insurance
decision, legal title, fulfillment or lender terms. The owner permission, actual
party, state and all relevant OD/financial policy gates still apply. Ordinary
CRM contact and model/profile edits acquire no blanket MFA requirement.

## 7. Typed fixture catalog for T-059/T-641

The following JSON is a **contract fixture source**, not seed data or runtime
config. `input` is public JSON. `bindings` describe synthetic server-only state;
they are never merged into a request. `expected` names an exact schema/status or
invariant. Secret markers such as `fixture-password` are intentionally nonworking
test strings, never defaults. Crypto/proof tests must generate ephemeral material
under a separately labelled fixture-only policy. IDs here are synthetic.

```json
[
  {"id":"auth.login.valid","schema":"LoginRequest","input":{"login":"fixture-user","password":"fixture-password"},"bindings":{"subject":"active-unenrolled","policy":"fixture-only","csrf":"valid"},"expected":{"status":200,"schema":"SessionRead"}},
  {"id":"auth.login.challenge","schema":"LoginRequest","input":{"login":"fixture-user","password":"fixture-password"},"bindings":{"subject":"active-enrolled","csrf":"valid"},"expected":{"status":200,"schema":"ChallengeResult","fullSessionIssued":false}},
  {"id":"auth.bootstrap.pending","schema":"LoginRequest","input":{"login":"fixture-bootstrap","password":"fixture-password"},"bindings":{"subject":"bootstrap-pending-enrollment","csrf":"valid"},"expected":{"status":200,"schema":"EnrollmentRequired","protectedAdmission":false}},
  {"id":"auth.login.unknown-field","schema":"LoginRequest","input":{"login":"fixture-user","password":"fixture-password","companyId":"20000000-0000-4000-8000-000000000001"},"expected":{"status":400,"code":"INVALID_REQUEST"}},
  {"id":"auth.login.null","schema":"LoginRequest","input":{"login":"fixture-user","password":null},"expected":{"status":400,"code":"INVALID_REQUEST"}},
  {"id":"auth.login.missing","schema":"LoginRequest","input":{"login":"fixture-user"},"expected":{"status":400,"code":"INVALID_REQUEST"}},
  {"id":"auth.login.empty","schema":"LoginRequest","input":{"login":"fixture-user","password":""},"expected":{"status":422,"code":"VALIDATION_FAILED"}},
  {"id":"auth.login.numeric","schema":"LoginRequest","input":{"login":123,"password":"fixture-password"},"expected":{"status":400,"code":"INVALID_REQUEST"}},
  {"id":"auth.login.no-origin","schema":"LoginRequest","input":{"login":"fixture-user","password":"fixture-password"},"bindings":{"origin":"missing","csrf":"valid"},"expected":{"status":403,"code":"ORIGIN_REJECTED","credentialWork":false}},
  {"id":"auth.login.csrf","schema":"LoginRequest","input":{"login":"fixture-user","password":"fixture-password"},"bindings":{"origin":"allowed","csrf":"stale"},"expected":{"status":403,"code":"CSRF_REJECTED","credentialWork":false}},
  {"id":"auth.verify.valid","schema":"VerifyRequest","input":{"challengeId":"30000000-0000-4000-8000-000000000001","code":"fixture-proof"},"bindings":{"proof":"generated-totp-for-fixture-policy","cookie":"bound","counter":"unused"},"expected":{"status":200,"schema":"SessionRead","oldCookieRejected":true}},
  {"id":"auth.verify.concurrent","schema":"VerifyRequest","input":{"challengeId":"30000000-0000-4000-8000-000000000001","code":"fixture-proof"},"bindings":{"concurrentRequests":2,"counter":"same-valid-unused"},"expected":{"successfulConsumptions":1}},
  {"id":"auth.verify.recovery-code","schema":"VerifyRequest","input":{"challengeId":"30000000-0000-4000-8000-000000000001","code":"fixture-recovery-proof"},"bindings":{"proof":"generated-unused-recovery-code","primaryAuthentication":"satisfied","allowedKind":"recovery-code"},"expected":{"status":200,"codeConsumptions":1,"roleChange":false}},
  {"id":"auth.verify.foreign","schema":"VerifyRequest","input":{"challengeId":"30000000-0000-4000-8000-000000000002","code":"fixture-proof"},"bindings":{"cookie":"other-subject"},"expected":{"status":404,"code":"NOT_FOUND","consumptions":0}},
  {"id":"auth.verify.replay","schema":"VerifyRequest","input":{"challengeId":"30000000-0000-4000-8000-000000000001","code":"fixture-proof"},"bindings":{"counter":"already-consumed"},"expected":{"status":401,"code":"AUTHENTICATION_FAILED","newSessions":0}},
  {"id":"auth.verify.body-actor","schema":"VerifyRequest","input":{"challengeId":"30000000-0000-4000-8000-000000000001","code":"fixture-proof","actorId":"10000000-0000-4000-8000-000000000002"},"expected":{"status":400,"code":"INVALID_REQUEST"}},
  {"id":"auth.logout.repeat","schema":"EmptyRequest","input":{},"bindings":{"session":"already-revoked","csrf":"matching-revocation-binding"},"expected":{"status":204,"bodyAbsent":true}},
  {"id":"auth.revoke.reason","schema":"RevokeAllRequest","input":{"reason":"  "},"expected":{"status":422,"code":"VALIDATION_FAILED"}},
  {"id":"auth.revoke.mfa","schema":"RevokeAllRequest","input":{"reason":"Synthetic revoke"},"bindings":{"primaryAuthentication":"fresh","mfa":"stale"},"expected":{"status":403,"code":"MFA_REQUIRED"}},
  {"id":"auth.recovery.disabled","schema":"RecoveryRequest","input":{"login":"fixture-user"},"bindings":{"capability":"Disabled"},"expected":{"status":403,"code":"POLICY_UNRESOLVED","accountLookup":false,"delivery":false}},
  {"id":"auth.recovery.confirmation","schema":"RecoveryCompleteRequest","input":{"token":"fixture-token","newPassword":"fixture-password","confirmation":"different-fixture"},"bindings":{"capability":"fixture-only-accepted"},"expected":{"status":422,"code":"VALIDATION_FAILED","credentialChanges":0}},
  {"id":"auth.event.valid","schema":"SecurityChangedData","input":{"userId":"10000000-0000-4000-8000-000000000001","securityRevision":"2","change":"mfa-enrolled"},"expected":{"valid":true}},
  {"id":"auth.event.secret","schema":"SecurityChangedData","input":{"userId":"10000000-0000-4000-8000-000000000001","securityRevision":"2","change":"mfa-enrolled","code":"fixture-proof"},"expected":{"valid":false}},
  {"id":"auth.event.numeric-revision","schema":"SecurityChangedData","input":{"userId":"10000000-0000-4000-8000-000000000001","securityRevision":2,"change":"mfa-enrolled"},"expected":{"valid":false}},
  {"id":"auth.event.revision-overflow","schema":"SecurityChangedData","input":{"userId":"10000000-0000-4000-8000-000000000001","securityRevision":"9223372036854775808","change":"mfa-enrolled"},"expected":{"valid":false}}
]
```

Public request schema aliases above are exactly the corresponding matrix bodies:
LoginRequest, VerifyRequest, EmptyRequest, RevokeAllRequest, RecoveryRequest and
RecoveryCompleteRequest. Secret/proof markers are generator substitutions, not
an assertion those literals pass a real TOTP/code format. Fixture-only accepted
recovery bindings test protocol mechanics without representing security acceptance.

Reusable positive full-session JSON (schema SessionRead):

```json
{"data":{"user":{"id":"10000000-0000-4000-8000-000000000001","displayName":"Synthetic User","status":"active"},"roles":[],"permissions":[],"mfa":{"enrolled":false},"context":{"revision":"0","companyId":null,"branchScope":{"mode":"ALL","branchIds":[]}},"accessibleCompanies":[],"setup":{"next":"company","partnershipRequiredForB2B":true}},"revision":"0","asOf":"2026-09-15T00:00:00Z"}
```

T-059 must expand this source into its assigned auth.fixtures.json with distinct
schema-positive, schema-negative and state/transport-negative cases. Additional
required cases are explicit, not silently inferred from frontend hiding:

| Fixture family | Required assertion |
|---|---|
| Response closure | Extra token/password/role fields; unknown status/change; nested unknown keys; duplicate/null IDs; mismatched outer/context revision; enrolled=false with authenticatedAt all reject. |
| Session scope | Null company + SELECTED rejects; ALL with IDs rejects; SELECTED empty/duplicate/foreign branch rejects; role in membership cannot be submitted; same user switching company cannot retain prior branch authority. Context stale412/missing precondition428, foreign company404. |
| Permission/party | Platform administrator without company-financial permission cannot perform partner decision; valid session/MFA alone grants no owner action; ordinary permitted CRM edit without MFA remains eligible; unknown catalog key denied. |
| Enumeration | Unknown login, wrong password, suspended and ineligible pending subject have same status/schema/safe message; enabled recovery request known/unknown/ineligible has same202 shape; delivery errors do not expose existence. |
| State/revision | Expired challenge, changed security version, consumed recovery code, concurrent TOTP on two challenges, revoked session, suspended subject and cross-subject enrollment cannot produce a new full session. Check binding again inside transaction. |
| Enrollment/bootstrap | Two deployments race one guard: one pending bootstrap; no HTTP bootstrap; no default credentials/argv secrets; arbitrary pending user cannot use bootstrap enrollment; confirmation repeat never reissues codes. Last-admin/self-protection remains serialized. |
| Unknown outcome | Inject rollback and actual-commit lost replies for login/MFA/enrollment/recovery; no false success, secret receipt, replayed factor consumption or automatic secret retry. Session read/login can establish only its documented authoritative result. |
| Secret safety | Walk errors, events, outbox, generic receipts, logs/traces and operation metadata; reject every forbidden auth field and secret value, including nested/alternate encoding. Provider command hashes only approved nonsecret canonical values. |
| Gated adapters | Missing policy/key/readiness input fails construction; Disabled recovery makes no account/delivery call; unapproved T-002 reference cannot construct Accepted; invalid proof cannot change roles/memberships/MFA or issue session. |
| Reliability | Event/private security state rollback together; failed external publication never repairs by duplicating security transition; external consumer admission absent does not broadcast user data; no auth projection admits a new command. |

These are downstream acceptance fixtures, **not executed backend tests**. T-058
verification parses the examples, checks local links and schema/authority
consistency. T-059/T-641 supply executable schemas/validators; T-060–T-063 supply
private transactional/cryptographic tests; T-592 closes combined B-02 acceptance.

## 8. Coordinator handoff

Adopt/reject A1–A5 and the auth metadata schema as this bounded technical proposal;
preserve T-002/OD-12 release gates. No broad architecture reapproval is requested.
T-059 must not expose an unapproved extension or invent recovery proof. Its
auth migration owns only Identity private state; owner migration readiness/version
coordination remains the coordinator's assigned integration handoff. Specifically,
T-059 owns `services/identity/migrations/0012_auth.up.sql`, while T-024's existing
UOW.Check accepts clean owner ledger version 1. Installing version 12 does not
authorize a feature adapter to bypass readiness or edit the unowned UOW. A separate
coordinator-owned ledger/version compatibility handoff is required before
dependent runtime implementation; this proposal does not select a migration
skip/order rule or claim that all intermediate owner migrations exist. Auth-security
event activation additionally needs explicit subscriber/admission ownership; this
document assigns no other service a recipient grant. T-063/invitation delivery and
production activation wait separate security acceptance. Missing UI remains its
assigned design supplement and uses HTML, with no Figma dependency.
