# Identity, tenant, Admin and Settings — SLICE proposal

Date: 2026-09-13. Status: **proposed; not architecture/release approval**.
Worker role: `codex/agents/tech-architect.md`, SLICE. No application code,
target writes, Git initialization or browser QA performed.
Aligned with `../../state/architecture-inputs/coordinator-decisions.md`: seven domain owners, one Go module,
service-private SQL, no separate audit/workflow service; task list is a preview
before the architecture checkpoint and formal PO/PM planning.

## 1. Evidence and scope

Current source root: `/Users/bakhromachilov/startups/justixauto/docs/justix-auto`.
Read `business-logic.md` (especially §§3, 9, 11), `open-decisions.md`,
`dev/dev-state.md`, `reference/gaze-reference.md`, and `mock-map.md`.
Additional operation evidence: `mocks/platform-directory.test.cjs` and
`mocks/demo-accounts.test.cjs`. Tests are inspected demo evidence, not production
authorization or verified browser parity. Legacy aggregate screen IDs are not
current UI truth and are deliberately not imported.

Confirmed rules:

- One User, global Role assignments, independent company Memberships without
  roles; a role is not tenant access. Capability, user permission, membership,
  branch scope, object side and object state are separate gates.
- Organization access `draft|active|suspended`, compliance and technical
  integration are different facts. Activation grants none of the latter.
- Provider creation atomically creates draft company + new credentialed user +
  membership. A duplicate login or existing user's email rejects the whole
  operation; it never overwrites credentials or silently links that identity.
- Admin cannot make financier/insurer decisions or confirm a partner payment.
- Revocation and suspension preserve records/history. Role edits reject unknown
  permissions; system roles are protected; stale mutations fail atomically.
- A company switch resets branch selection to ALL in the same server operation.
  Creating a company never switches context. Warehouse remains inventory-owned.

Technical details below are explicit architecture proposals resolving the
mechanisms portion of OD-12, not claims that these mechanisms exist in Gaze.
OD-03/05/10/11/14 remain narrowly scoped product/release questions.

## 2. Ownership and concrete records

Types: `ID=UUID`, `Version=uint64`, `Time=UTC RFC3339`, non-empty string unless
marked `?`; nullable references are explicit. All aggregates have `id,version,
createdAt,updatedAt`. No entity is identified by email, country or registration.
Identity owns its SQL schema/event streams; other services never write them.

| Aggregate / local record | Fields and transitions | Constraints / ownership |
| --- | --- | --- |
| Organization | `kind:seller\|bank\|mfo\|insurance`, `profileRevisionId:ID`, `countryKey:string`, `registrationKey:string`, `access:draft\|active\|suspended`, `capabilities:set<string>`, `complianceRef:ID?` | Stable ID; unique `(countryKey,registrationKey)`; create as draft; activate draft→active, suspend active→suspended, restore suspended→active; all access transitions require reason; no hard deletion |
| OrganizationProfileRevision | `id,organizationId,name,legalName,country:{key,label},region:{key?,label}?,registration,email,address?,phone?,createdBy,createdAt` | Restricted versioned record; editing keeps prior snapshots and related IDs; no copying programs/applications/inventory; country supports normalized free text, not five-country enum; changing country clears region, and region cannot exist without country |
| ComplianceAssessment | `id,organizationId,status:VERIFIED\|LIMITED\|SUSPENDED\|REJECTED,sourceRef?,assessedAt,assessedBy,reason,version` | Independent record; initial absent assessment means unknown, not verified. Assessment command/policy is blocked by OD-11/14; never derive it from operational activation |
| IntegrationConnection | `id,organizationId,providerKind,externalSystemRef?,configurationRef?,technicalState:not-configured\|configured\|connected\|disabled,version` | Proposed technical ownership record, not permission/licence. Provider APIs and real states require OD-10; four-app account access requires no external API connection |
| Branch | `organizationId,name,address?,status:active\|inactive,version` | Immutable organization FK; branch lifecycle belongs to identity; warehouse link is NOT duplicated here as a write field |
| User | `profileRevisionId,normalizedEmail,normalizedLogin?,status:pending\|active\|suspended,roleIds:set<ID>,securityVersion:uint64` | Unique normalized email; unique non-null login. Email/login changes are not in proposed R1; separate future verification flow. Global roles, no company-specific role assignment |
| UserProfileRevision | `id,userId,displayName,email,createdAt,createdBy` | Restricted mutable-retention side store/version references; no raw contact details on public integration events; long-term retention decision remains open under OD-10 |
| Role | `name,system:bool,permissionKeys:set<string>` | System roles immutable through public commands; custom roles use allowlisted assignable permissions; `platform.manage` cannot be smuggled into custom roles; no role deletion required in this slice |
| PermissionCatalog | `key,scope:platform\|company,requiresMfa:bool,assignable:bool` | Versioned deployment-owned allowlist, not arbitrary strings accepted from clients; domain declares per-command mapping; UI uses same names for preview only |
| Membership | `userId,organizationId,status:active\|revoked,branchAccess:{mode:ALL_BRANCHES\|SELECTED_BRANCHES,branchIds:ID[]}` | Unique `(userId,organizationId)` including revoked history; no role field; regrant is explicit versioned transition, not duplicate identity; all branch IDs must belong to organization |
| Invitation | `organizationId,normalizedEmail,expiresAt,invitedBy,status:pending\|accepted\|revoked\|expired,acceptedUserId?` | Single-use secret stored only as a token digest in private auth storage; no role assignment implicitly inherited from membership |
| Session (private mutable auth record, not ES aggregate) | `id,userId,tokenDigest,createdAt,lastSeenAt,expiresAt,revokedAt?,mfaAuthenticatedAt?,securityVersion,contextVersion,activeOrganizationId?,branchScope:{mode:ALL\|SELECTED,branchIds:ID[]}` | Browser gets opaque cookie, not role/membership claims; context updated with CAS; ALL is effective authorized branches, not all platform branches |
| Credential/MfaFactor/RecoveryToken (private records) | `userId,algorithm,hashAndParameters,changedAt`; `factorId,userId,type,encryptedSecretOrPublicKey,confirmedAt?,lastAcceptedCounter?`; `tokenDigest,userId,purpose,expiresAt,consumedAt?` | Never serialize into events, audit, command log/idempotency payload, error, trace or normal DTO. Salt/hash/digest are sensitive credential material too |
| PlatformAccessGuard (singleton invariant row) | `id='platform-access',version` | Lock in the same identity transaction as changes that could remove last active MFA-enabled platform admin. Count effective admins under this lock, not a stale projection |

Profile revisions are encrypted/restricted supporting records referenced by
events; their contents are not a broadly published stream. This preserves
event-replayed identity state while avoiding irreversible plaintext contact data
in event history. Retention/erasure policy is not invented here (OD-10); a purge
must leave a tombstoned revision reference so replay does not fail.

Indexes beyond listed unique keys: Membership `(organizationId,status,userId)`;
Branch `(organizationId,status,id)`; User `status`; Invitation
`(organizationId,normalizedEmail,status)`; Session `(userId,revokedAt,expiresAt)`;
private token digest unique; organization lookup normalized keys. Event store
must enforce `(aggregateType,aggregateId,version)` uniqueness and expected
version writes; this is a proposed Justix guarantee, not inherited assurance.

## 3. Commands and edge DTOs

Proposed module-local prefix `/api/v1/identity`; final edge routing is synthesis.
Writes carry `Idempotency-Key: UUID`; existing aggregates carry
`If-Match: "<version>"`. Create uses a reserved idempotency outcome and allocated
IDs in the same transaction. Body never supplies trusted actor/permission/MFA.
Private credentials are accepted only on TLS request paths whose bodies are
redacted before tracing/error middleware. Do not persist secret-bearing bodies
or unkeyed body fingerprints in the idempotency ledger.

`WriteResult={operationId:ID,resource:{type,id,version},related?:[{type,id,version}],
projectionStatus:pending|current}`; creates return 201, ordinary committed
commands 200, cross-service process acceptance 202. Command result confirms
commit, not projection catch-up. Query envelope `={data,projectionVersion}`;
paginated lists `={items,nextCursor?}`. Errors
`={code,messageKey,fieldErrors?:{field:code},currentVersion?,operationId?}`:
400 invalid syntax, 401 authentication, 403 denied/MFA_REQUIRED/POLICY_UNRESOLVED,
404 hidden or missing resource, 409 VERSION_CONFLICT/ALREADY_EXISTS/
IDEMPOTENCY_CONFLICT, 422 validation, 503 authoritative policy unavailable.
No error leaks whether a foreign tenant's object exists.

`CompanyInput={name,legalName,country:{key?,label},region?:{key?,label},
registration,email,address?,phone?}`. Server returns canonical normalized keys;
region from the old country is rejected/cleared instead of silently retained.
String bounds, normalization implementation and permission catalog receive
contract tests; no national registration checksum is assumed without a policy.

| Method/path | Input → result / command |
| --- | --- |
| `GET /session` | → `{user:{id,displayName,status},roles:[{id,name}],permissions:string[],mfa:{enrolled,authenticatedAt?},context:{version,organizationId?,branchScope},accessibleOrganizations:[{id,name,kind,access}],setup:{next:company\|branch\|none,partnershipRequiredForB2B:bool}}`; never secrets |
| `POST /session/login` | `{login,password}` → authenticated session cookie or `{challengeId,required:mfa}`; no company access inferred from successful credential verification |
| `POST /session/mfa/verify` | `{challengeId,code}` → rotated authenticated cookie; challenge is short-lived, single-use, bound to initial login/session |
| `POST /session/logout` | no body → 204, revoke server record and expire cookie; replay idempotent |
| `POST /session/revoke-all` | `{reason}` + reauthentication → revoke caller's other sessions; current cookie rotated; administrative revoke has separate user endpoint |
| `PUT /session/context` | `{organizationId,expectedContextVersion}` → `{context:{version,organizationId,branchScope:{mode:ALL,branchIds:[]}}}`; ALL reset is atomic |
| `PUT /session/branch-scope` | `{mode:ALL\|SELECTED,branchIds:ID[],expectedContextVersion}` → full context; ALL requires empty IDs, SELECTED nonempty valid authorized branches |
| `POST /admin/provider-companies` | `{kind:bank\|mfo\|insurance,company:CompanyInput,firstAdmin:{displayName,login,email,password,passwordConfirmation}}` → Organization + User + Membership references; local SQL transaction includes credential insert, global company-admin role assignment and safe event/outbox writes; audit derives from those events |
| `POST /companies` | `{company:CompanyInput}` → new seller Organization + actor Membership; global `company.create` privilege required, no automatic role escalation or context switch |
| `GET /companies/:id`; `GET /admin/companies?kind=&access=&cursor=` | → `{id,version,kind,profile:CompanyInput,access,capabilities,compliance:{status}\|null,integration:{technicalState}\|null}`; read permissions and field filtering apply independently |
| `PATCH /companies/:id` | `CompanyInput` → preserve ID, snapshots and dependents; same application command for Settings and permitted Admin route |
| `POST /admin/companies/:id/activate\|suspend\|restore` | `{reason}` → versioned access transition, no compliance or API side effects; suspension policy integration depends on OD-05 |
| `POST /companies/:id/branches` | `{name,address?,warehouse:{mode:none}\|{mode:link,warehouseId,expectedWarehouseVersion}\|{mode:create,name,capacity:int>0}}` → 201 branch-only or 202 durable branch-setup operation; inventory owns warehouse/link/capacity |
| `PATCH /companies/:id/branches/:branchId` | `{name,address?}` → branch details only; status change with attached warehouse requires explicit coordinated operation |
| `GET /admin/users?cursor=`; `GET /admin/users/:id` | → `{id,version,displayName,email,status,roleIds,mfaEnrolled,memberships:[{id,organizationId,status,branchAccess,version}]}`; no credential info |
| `POST /admin/users` | `{displayName,email,roleIds}` → User in pending state, not silently authenticated; credential enrollment/ invitation is explicit |
| `PATCH /admin/users/:id` | `{displayName,roleIds}` → global role assignment update; unknown roles rejected; last-admin and self-access guards |
| `POST /admin/users/:id/suspend\|restore` | `{reason}` → status; suspend increments securityVersion/revokes sessions without deleting Memberships |
| `POST /admin/users/:id/revoke-sessions` | `{reason}` → all user's sessions invalidated; no password disclosure/reset implied |
| `POST /admin/users/:id/memberships` | `{organizationId,branchAccess}` → Membership active; duplicate active membership rejected, existing revoked record explicitly reactivated |
| `PATCH /admin/memberships/:id/branch-access` | `{branchAccess}` → update allowed branches, invalidate incompatible session context |
| `POST /admin/memberships/:id/revoke` | `{reason}` → revoked; other companies remain accessible; own-access protection |
| `GET /admin/roles`; `GET /admin/permissions` | → roles + canonical allowlisted permission descriptions, scopes and MFA requirements |
| `POST /admin/roles`; `PATCH /admin/roles/:id` | `{name,permissionKeys:string[]}` → create/update custom role; system/unknown/unassignable permissions rejected |
| `POST /admin/companies/:id/invitations` | `{email}` → `{id,version,status,expiresAt}`, not raw token; delivery adapter receives token transiently via approved secret handling |
| `POST /invitations/accept` | `{token}` + authenticated verified matching identity → Membership, no duplicate User; new identity completes credential enrollment first, never overwrites an existing user from a bearer token alone |
| `POST /admin/invitations/:id/revoke` | `{reason}` → revoke pending invitation; no effect on an already accepted Membership |
| `GET /admin/audit?resourceType=&resourceId=&actorId=&cursor=` | → allowlisted `{id,actorId,action,resource,at,reason?,before,after,correlationId}`; read-only, authorization-filtered |
| `GET /operations/:id` | → `{id,status:pending\|succeeded\|failed\|needs-attention,result?,error?}`; only originating actor or authorized same-company manager/platform operation reader |

Auth bootstrap/MFA enrollment/recovery are security infrastructure, not a fifth
product app. Proposed explicit endpoints after mechanism approval:
`POST /session/mfa/enrollment` → `{enrollmentId,secretOrChallenge}` once over TLS;
`POST /session/mfa/enrollment/:id/confirm {code}`;
`POST /session/recovery/request {login}` → uniform 202;
`POST /session/recovery/complete {token,newPassword,confirmation}` → 204 plus
session revocation, never automatically changes membership/roles/MFA.
Delivery provider, recovery identity proof and production password/MFA policy
must be finalized in OD-12/security acceptance; these endpoints are not ready
tasks while those choices are unresolved. Encrypted transient challenge storage
is not an event or normal application read projection.

Idempotency for credentialed creation: preserve a keyed HMAC of normalized
non-secret fields plus the resulting IDs/outcome; do not store password hashes
as request fingerprints. On replay, require the same non-secret fields and
verify supplied password against the created private credential, or return a
generic mismatch; never alter the prior account. Bounded retention and
rate-limiting apply. Failed validation creates neither rows nor reusable outcome.

## 4. Events and inter-service policy contract

Common proposed envelope:
`{eventId,aggregateType,aggregateId,aggregateVersion,schemaVersion:1,
occurredAt,actorId,organizationId?:ID,operationId,correlationId,causationId,data}`.
Organization is nullable for global User/Role/security events; it is not filled
with the actor's arbitrary active company. Outbox/confirm-aware relay/inbox are
proposed hardening ADRs; Gaze's post-commit goroutine is not durable publication.
Consumers upsert by event ID/version and cannot treat a delayed role projection
as an authorization grant.

| Event | Exact `data` contract / recipient |
| --- | --- |
| `identity.organization.created.v1` | `{organizationId,kind,profileRevisionId,access:draft,capabilities:string[]}`; other domains create directory views only, no programs/stock |
| `identity.organization.profile-updated.v1` | `{organizationId,profileRevisionId,previousProfileRevisionId}`; consumers may resolve an authorized current profile, never rewrite business snapshots |
| `identity.organization.access-changed.v1` | `{organizationId,from,to,reason,securityPolicyVersion}`; directory projections/cache invalidation; policy effect for suspended company is OD-05 |
| `identity.branch.created.v1` | `{branchId,organizationId,name,address?,status:active,operationId}`; branch projection; attaching warehouse is separate inventory acknowledgement |
| `identity.branch.details-updated.v1` | `{branchId,organizationId,name,address?}`; no warehouse mutation |
| `identity.user.created.v1` | `{userId,profileRevisionId,status,roleIds:ID[]}`; restricted identity stream only |
| `identity.user.profile-updated.v1` | `{userId,profileRevisionId}`; restricted identity stream only |
| `identity.user.roles-changed.v1` | `{userId,roleIds:ID[],securityVersion}`; invalidation, not new company access |
| `identity.user.status-changed.v1` | `{userId,from,to,reason,securityVersion}`; revoke/invalidate sessions authoritatively in originating transaction |
| `identity.role.created.v1` / `identity.role.permissions-changed.v1` | `{roleId,name,system,permissionKeys:string[],policyVersion}`; no per-company assignments |
| `identity.membership.granted.v1` / `identity.membership.branch-access-changed.v1` | `{membershipId,userId,organizationId,branchAccess,securityPolicyVersion}` |
| `identity.membership.revoked.v1` | `{membershipId,userId,organizationId,reason,securityPolicyVersion}`; retain historical records and other memberships |
| `identity.invitation.created.v1` / `accepted.v1` / `revoked.v1` | `{invitationId,organizationId,status,acceptedUserId?:ID,reason?:string}`; no email/token in published event |
| `identity.security-state-changed.v1` | `{userId,change:credential-changed\|mfa-enrolled\|sessions-revoked,securityVersion}`; restricted stream; no secret, hash, salt, backup code or session token |

Any sensitive before/after field needs explicit audit allowlisting; reason text
has length limits and secret-detection warnings, and must not be used as an
uncontrolled error/credential dump. Profile/body logging disabled for auth routes.

Internal authenticated service port, transport chosen in synthesis:

```text
Authorize(AuthorizationRequest) -> AuthorizationDecision
AuthorizationRequest = {
  sessionHandle: opaque, operationId: ID, action: canonicalPermissionKey,
  organizationId: ID?, branchId: ID?, resource: {type: string,id: ID}?,
  contextVersion: uint64?, requestId: ID
}
AuthorizationDecision = {
  decisionId: ID, allowed: bool, denyCode?: string,
  actorId: ID, organizationId: ID?, effectiveBranchIds: ID[],
  policyVersion: uint64, sessionVersion: uint64,
  mfaAuthenticatedAt?: Time, decidedAt: Time
}
GetOrganization(id, callerService, purpose) -> {
  id,version,kind,access,capabilities,compliance:{status}|null,
  profileRevisionId
}
```

Internal clients are authenticated/allowlisted by service identity; public edge
must strip spoofed actor/company/permission headers. Domain loads the object
first under safe tenant filters and passes its actual owner/branch, not client
claims. Identity verifies active user/session, permission, membership, branch,
capability and configured company-access policy against authoritative state.
Domain separately verifies partnership, deal side, object state and business
invariants. Finance/insurance documents require their owner domain's per-object
authorization before documents service grants a file URL.

No cached positive decision authorizes a later command. Revoke completion denies
new authorization admissions; an already authorized in-flight domain transaction
can complete. This explicit admission-time boundary avoids falsely claiming
cross-service atomic revocation. Higher-risk operations may reauthorize just
before local commit, but that is still not a distributed lock. A stricter
no-in-flight-execution policy would require a separately approved coordinated
protocol. Timeout/identity outage fails closed for commands and protected reads.

## 5. Per-operation authorization and policy gates

Every company row below means `authenticated active user + live session +
global permission + active Membership + authorized branch where applicable +
configured Organization policy + owning-domain object-side/state checks`.
Platform operations require platform-scoped grants but not an invented platform
company Membership. All mutation grants default to MFA-required in this proposal;
the confirmed minimum is sensitive operations, and the exact catalog can narrow
that conservative default during architecture approval. MFA enrollment state
alone is not recent successful challenge verification.

| Operation | Proposed permission / additional checks | Dependency scope |
| --- | --- | --- |
| Directory list/detail/audit | `platform.directory.read` / `platform.audit.read`; field-scoped view; no partner operational override | Expanded oversight/references/settings excluded by OD-14 |
| Provider create | `platform.companies.create`, recent MFA; uniqueness + atomic company/user/credential/membership writes; system company-admin role only | OD-12 mechanism approval, not OD-10 bank API |
| Activate draft / restore suspended | `platform.companies.access`, recent MFA + reason + expected version | Must not imply compliance. Restore/suspended effects depend OD-05; capability/compliance matrix OD-11 |
| Suspend active company | `platform.companies.access`, recent MFA + reason; preserve obligations and data; self-access and last-admin checks if applicable | Propagation/access effect is OD-05-blocked, while transition model tests may be drafted |
| Seller create / edit own details | `company.create` / `company.edit`; editing bound to target membership, only seller type; no context switch | Creation/default capabilities need synthesis allowlist; no B2B partnership gate |
| Branch create/edit | `branches.create` / `branches.edit`; target company/branch checks, reject foreign warehouse; linked setup uses operation ID | Inventory port required; no business-policy block for basic branch record |
| Context / branch selection | no new business grant; validate current membership/allowed branches, CAS context version | OD-05 for suspended company workspace access; ALL never bypasses branch permissions |
| User create/edit/status/session revoke | `platform.users.manage`, recent MFA; known global roles; immutable identity keys; self-access/last-admin lock | OD-12 enrollment/recovery policy only where used |
| Membership grant/change/revoke | `platform.memberships.manage`, recent MFA; same-company branches, no roles; self-revocation protection; no effect on other companies | Safe foundation; suspended tenant grant usability depends OD-05 |
| Custom role create/edit | `platform.roles.manage`, recent MFA; allowlisted assignable keys; system role immutable; global impact preview; last-admin lock | No per-company customization invented |
| Invitation create/revoke/accept | `platform.memberships.manage` + MFA for issuer; single-use token; acceptance requires verified matching identity; roles unchanged | OD-12 proof/delivery mechanism; no invitation required for new credentialed provider admin |
| Financier/insurer operation | domain permission + target organization membership/capability + addressed-to side + correct state; platform grant insufficient | Finance/insurance owner performs side/state checks; OD-05/11 only for affected access policies |
| B2B new transaction | company gate above + commerce active partnership | OD-03 does not block retail setup; OD-04 applies resale/warehouse access, not generic identity |
| Company capability/compliance edit / external integration connect | distinct explicit grants and transitions; no generic `PATCH status` | OD-10/11/14: do not create ready activation-as-licence or API-connect tasks |

Common-company objects with `branchId=null` use company membership/permission;
branch SELECTED filters only branch-bound objects. This is a proposed explicit
scope rule for synthesis to validate against Settings/inventory query contracts,
not a grant to all warehouses. Branch-linked setup has one user operation but
not a distributed SQL transaction: reserve operation ID, create Branch, request
inventory link/create, expose pending/failed state, retry idempotently. On
inventory failure keep the created branch visible with setup failure; never
delete its canonical history. Any UX not present in current mocks is a parity
gap, not evidence this pending state is already approved.

## 6. Bootstrap, sessions and protected-admin invariants

Recommended OD-12 mechanism: identity-owned credential/session records behind
ports, kept in the same database transaction as provider provisioning. Alternative
managed IAM simplifies authentication maintenance but cannot natively satisfy
the confirmed same-transaction identity/company create semantics; adopting it
requires an explicit provisioning process/failure contract, not a hidden change.

Proposed security ADR details:

- First platform admin is created through a deployment-only CLI reading secrets
  from stdin/secret injection, never shell arguments or a public bootstrap HTTP
  route. A unique bootstrap sentinel and PlatformAccessGuard lock make the
  operation single-use under concurrency. No hardcoded/default/demo accounts.
  Bootstrap creates only a pending enrollment identity; activation of platform
  authority requires verified MFA in an atomic guard transaction. No non-MFA
  user can operate Admin during the enrollment window.
- Use a maintained Argon2id implementation with per-password random salt and
  explicit parameter version, configurable baseline benchmarked before release;
  no custom crypto or copied demo PBKDF2 constants. Hash material stays private.
  This proposal follows [OWASP Password Storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html),
  consulted 2026-09-13; implementation/package pins remain synthesis work.
- Four apps share a same-origin edge in the proposal; use an opaque server-side
  session cookie `__Host-justix_session; Secure; HttpOnly; SameSite=Lax; Path=/`,
  no Domain attribute. Enforce Origin and per-session CSRF tokens on cookie-
  authenticated mutations, including logout. Never browser localStorage tokens.
  Rotate session identifier at login, MFA elevation and recovery. These measures
  follow [OWASP Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)
  and [CSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html).
- Propose configurable initial session limits: 30-minute idle, 12-hour absolute,
  5-minute MFA freshness for sensitive commands; these are proposed security
  defaults, not derived product facts. Login/recovery have per-account and
  per-source throttles, bounded challenge attempts and generic failure messages.
- Propose TOTP plus single-use recovery codes as first MFA adapter; private
  encrypted secrets, replay-counter check, single-use recovery digest, enrollment
  confirmation required. Exact recovery proof/delivery needs security acceptance;
  do not add an unrestricted admin bypass for a lost second factor.
- A draft-only provider user may establish an enrollment/recovery session but
  cannot enter the provider workspace. An active company/membership grants app
  access subject to permissions; a suspended company is OD-05-blocked. A user
  may still use another accessible company without a duplicated identity.
- User suspension, global grant reduction, MFA change or credential recovery
  increments authoritative security version/revokes affected sessions in the
  identity transaction. Membership change invalidates only affected company
  context; if active company is revoked, context becomes unset and company
  picker can expose remaining memberships. Never silently choose another tenant.
- Last-admin guard applies to User suspension, role changes, MFA removal and
  any operation changing effective platform authority. Serialize these commands
  against PlatformAccessGuard before counting eligible admins. Reject self-
  suspension, self-revocation and self-removal of administrative access even if
  another admin exists. Concurrent attempts to demote separate final admins must
  leave at least one active MFA-enabled admin. Do not count pending/draft
  enrollment or merely enrolled-but-disabled factor state as a usable admin.

## 7. Query/UI mapping and testability

Current navigation anchors, not invented screen IDs:

- `/admin/` → Companies and Integrations → MFO/Bank/Insurer: shared directory
  detail/form and access actions; provider first-admin credentials are write-only.
- `/admin/` → Users, Roles, Audit: global role selection, independent membership
  rows, protected system roles, immutable audit. Custom role and membership
  mutations lack complete prior browser QA; do not label parity QA green.
- `/` → Settings → Companies and branches: common command/query ownership with
  Admin; context picker is separate; create result retains current workspace.
- Country/region inputs: suggestions plus free country input, region disabled
  without country, old region cleared when country changes.
- Session/permission shell is shared by realization/finance/insurance/Admin;
  no dealer/distributor app restoration. Auth errors retain non-secret form
  fields; clear password fields after success/failure as appropriate; never
  cache credentials with draft form state.

Each FE mutation must document entry→validation/cancel→pending→success/reopen,
403/MFA, stale 409 and retry. Pending cross-service branch setup and production
MFA/recovery surfaces need explicit UX acceptance; their absence blocks those
FE parity tasks, not the base aggregates.

Unit tests: role-without-membership denial; global role union across memberships;
unknown/system/unassignable role edits; country change clears region; access vs
compliance independence; ALL vs SELECTED validation; context creation/switch
semantics; event replay references/tombstones; privacy serialization allowlists.

Real PostgreSQL integration: provider creation failure at each insert rolls back
all domain/credential/event/outbox/idempotency writes; case-normalized duplicate
email/login/company keys under races; expected-version conflict; simultaneous
last-admin removals; unique bootstrap attempts; invitation consume race; MFA
counter/recovery code replay; atomic context CAS; expired/revoked session denial.

Cross-service contract tests: forged tenant/actor headers rejected; identity
timeout denies; stale projection never permits; own and foreign branch queries;
financier/insurer side filtering despite platform role; new post-revoke commands
denied; explicitly test admitted-before-revoke transaction boundary; duplicated
branch-setup messages do not create two warehouses. Reliability tests require
the proposed outbox/inbox ADR, not claims of inherited Gaze delivery guarantees.

Fixtures: two seller companies, bank, MFO, insurer; one multi-member user with
global viewer role; sales role; two MFA-enabled platform admins plus a pending
one; two branches plus company-wide warehouse reference; draft/active/suspended
companies with independent compliance states. All data synthetic, no demo
plaintext credentials copied to seed files; test credentials generated in memory.

## 8. Small task candidates

All items remain `proposed`; estimated **2–4 engineering hours each** after
common scaffold/persistence/contract fixtures exist. A candidate is one adapter,
command, projection or focused test unit, not a whole IAM feature. Safe means
safe to plan now, not approved for implementation. PM should split any candidate
whose repository-specific estimate exceeds four hours.
This is an architecture task-candidate preview, not the formal PO/PM backlog,
task board or authorization to skip either user checkpoint.

| ID | One coherent unit / done evidence | Dependencies / gate |
| --- | --- | --- |
| ID-01 | Define identity DTO/error/event envelope fixtures with secret-exclusion tests | Shared contract convention; safe foundation |
| ID-02 | Organization create/edit aggregate + normalized registration race-test contract | ID-01; safe foundation |
| ID-03 | Country/region normalization port + table-driven dependency tests | ID-01; safe foundation, no fixed national list |
| ID-04 | User global role assignment aggregate tests, unknown-role rejection | ID-01 + permission catalog |
| ID-05 | Role custom-edit command + system/unassignable protection tests | ID-04 |
| ID-06 | Membership grant/revoke command + cross-company independence tests | ID-04 |
| ID-07 | Membership branch-access command and branch-ownership tests | ID-06 + Branch contract |
| ID-08 | PlatformAccessGuard transaction adapter + concurrent last-admin tests | SQL fixture + ID-04/05; OD-12 ADR approval |
| ID-09 | Credential store/hasher port adapter + private serialization tests | OD-12 algorithm/parameters acceptance |
| ID-10 | Provider provisioning application command + injected-failure rollback tests | ID-02/04/06/09; identity transaction infrastructure |
| ID-11 | Provider-create HTTP adapter and DTO/duplicate/error tests | ID-10 + auth/CSRF middleware |
| ID-12 | Session persistence/expiry/revoke adapter tests | OD-12 lifetime/cookie ADR |
| ID-13 | Login challenge command + enumeration/throttling tests | ID-09/12; OD-12 acceptance |
| ID-14 | MFA challenge verify adapter + single-use/counter replay tests | ID-12; MFA adapter/security review |
| ID-15 | Session cookie/CSRF transport middleware tests | ID-12/14; same-origin edge decision |
| ID-16 | Authorize service port + exhaustive capability/member/permission denial table | ID-04/06/07/12; OD-05/11 unresolved states return explicit deny |
| ID-17 | Context-switch CAS command + ALL-reset/concurrency tests | ID-12/16; safe for active-company scenarios |
| ID-18 | Branch-scope CAS command + cross-company/unauthorized branch tests | ID-17/07 |
| ID-19 | Single-use bootstrap CLI adapter + concurrent sentinel tests | ID-08/09/14; OD-12 approval |
| ID-20 | Invitation consume command + identity-match/duplicate/race tests | ID-06; OD-12 proof/delivery contract |
| ID-21 | User suspension/revoke-session command + preservation/invalidation tests | ID-08/12/16 |
| ID-22 | Directory projection for company detail/list + pagination tests | ID-02; projection infrastructure |
| ID-23 | Audit allowlist projection + credential contamination tests | ID-01; sensitive field rules |
| ID-24 | Seller self-service company-create command + context-preservation test | ID-02/06/16 |
| ID-25 | Branch-create aggregate + company ownership tests | ID-02/16 |
| ID-26 | Branch-setup durable process transition unit + fake inventory acknowledgements | ID-25 + inventory ownership + reliability ADR approval |
| ID-27 | Branch-setup inventory request adapter + duplicate/failure contract tests | ID-26 + inventory idempotent create/link port |
| ID-28 | Company access transition aggregate tests; no compliance side effects | ID-02; safe model foundation only |
| ID-29 | Company suspension command integration/authorization policy tests | ID-28/16; **OD-05 policy-blocked** |
| ID-30 | Compliance/capability gate mapping tests for one approved operation | **OD-11 policy-blocked**; not generic activation alias |
| ID-31 | Password recovery request/consume adapter tests | ID-09/12; **OD-12 proof/delivery approval required** |
| ID-32 | Shared company form country/region fields + component tests | ID-03/22 + React scaffold + exact current mock parity |
| ID-33 | Shared company edit mutation adapter + 409/403/retry component tests | ID-32 + company HTTP adapter; no entire screen claim |
| ID-34 | Provider credentials form mutation wiring + secret-reset/error tests | ID-11 + React auth shell + approved mock states |
| ID-35 | Context picker query/mutation wiring + ALL-reset race test | ID-17/18 + shell; no nav redesign |
| ID-36 | One membership row revoke interaction + preserved other-company test | ID-06 + HTTP adapter + missing microstate review |
| ID-37 | One custom-role edit interaction + protected-system/unknown/stale tests | ID-05 + HTTP adapter + missing microstate review |

Separate follow-on task units still needed when in approved release: individual
User/Membership/Role HTTP adapters, per-route auth guard tests, MFA enrollment
component, invitation delivery and acceptance UI, remaining directory tables,
and audit read filter adapter. Do not hide these in ID-01 or label the 37 units
a completed exhaustive backlog; PM owns coverage and exact task IDs.

## 9. Decisions for synthesis / user

Synthesis should explicitly adopt or replace: identity-local atomic credential
provisioning; profile-revision privacy stores; same-origin opaque session model;
MFA adapter/security defaults; serialized last-admin guard; authoritative
admission-time authorization; branch-null query rule; branch-setup process
ownership and failure UX; endpoint/error/permission naming; reliability ADR.

User/product decisions remain: OD-05 suspended-company operation matrix;
OD-11 capability/compliance significance per operation; OD-10 real integration,
PII/consent/retention configuration; OD-14 expanded Admin release scope. OD-03
blocks only a universal partnership setup gate: current proposal leaves retail
usable and asks commerce to enforce active partnership on B2B commands. OD-12
mechanisms can be resolved architecturally, but must receive architecture and
security acceptance before their proposed implementation tasks become ready.
