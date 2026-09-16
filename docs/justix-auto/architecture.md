# JustixAuto — implementation architecture

Date: 2026-09-13. Status: **ARCHITECTURE APPROVED; release scope confirmed for task planning**.
Target: `/Users/bakhromachilov/startups/justixauto`. This is documentation, not implemented software.
The user-confirmed direction is Go microservices + CQRS/Event Sourcing in Gaze's hexagonal shape, React and four existing HTML-reference apps.
Technical decisions below and the linked appendices were approved by the user's
explicit "yes" to the architecture checkpoint on 2026-09-13. This does not approve
unknown business rules, unresolved security-release settings or release scope.
Planning is complete; see `dev/dev-state.md` for current readiness. No application implementation is authorized here.

## 1. Authority, scope and readiness

Product authority: current `business-logic.md`, scoped latest user decisions and `open-decisions.md`; visual authority: `mock-map.md` and immutable `mocks/` files.
Architecture contract authority: this document and its two subordinate appendices, [HTTP/domain contracts](contracts/http-domain.md) and [cross-owner protocols](contracts/cross-owner.md).
The appendices are part of this approved baseline, not competing specifications; this document wins if an inconsistency is found and the coordinator must correct the appendix before dependent readiness.
`dev/analysis/{identity,inventory-commerce,retail-finance-insurance,frontend-infra}.md` record alternatives/evidence only; their prefixes, schemas and task suggestions are superseded here.
`reference/gaze-reference.md` records observed reference behavior, not inherited guarantees. Historical `screens.md` is not the current React screen contract.
The supplementary [Gaze Executor CC audit](reference/gaze-executor-cc-reference.md)
records the user's 2026-09-14 request to retain microservices and also examine
`/Users/bakhromachilov/gaze-executor-cc` at `f81b62ce346320ec229817dd73cf9ec42032e1d9`.
Its command/aggregate/projection/subscriber structure informs implementation
within each owner. Its pre-commit broker publication, adapter imports in the app
layer and combined executor process do not replace the approved JustixAuto
reliability, dependency or independently deployable service boundaries.
Do not restore dealer/distributor apps, a fifth client app, public marketplace, dispute workflow, standalone billing/audit/workflow service or Figma prerequisite.
No bank disbursement, actual insurance policy, legal title transfer, production registration tariff or monetary servicing rule is inferred from demo state changes.

Independent Git setup was authorized and completed on 2026-09-14. The project
has its own `main` and initial commit, pushed to the user-provided GitHub remote:

```text
GIT CHECK OK: /Users/bakhromachilov/startups/justixauto
```

Do not use the parent repository. Before application edits run the target `bash tools/check-git.sh` and require its own root + initial commit.
Existing mock tests (106 at handoff), toolchain observations and source inspection are not Go/React QA. No application, dependency install, deployment or browser parity check is claimed here.

## 2. System and ownership

Exactly seven independently deployable domain owners; edge is routing/authentication transport, with no business database.
One PostgreSQL server may host seven databases locally; each has a distinct owner/user and no cross-database grants.
Each owner has its own domain event streams, command guards, projections, idempotency ledger, outbox/inbox and durable process records.
Separate API, projection and worker binaries may share the same owner's database; physical read/write database separation is not required.

| Owner | Authoritative aggregates/facts | Cross-owner limit |
|---|---|---|
| identity | Organizations/profiles, branches, users/global roles, permissions, memberships, sessions and credential records | No stock, document approval, provider decision or implicit compliance |
| inventory | VehicleModel, canonical VIN/VehicleUnit, warehouse/capacity/placement, receipt batches, eligibility facts, all reservations | No commercial price/contract/payment/CRM ownership |
| commerce | Partnership, offer versions, RFQ, quotation versions, orders/addenda, shipments, B2B invoice/evidence | Requests inventory allocation/receipt; never duplicates a VIN |
| retail | Natural-person customers, leads/tasks/contact history, listings, retail deals, retail invoice/evidence, own-installment records | Requests inventory reservation; no financier/insurer impersonation |
| financing | Provider programs/versions, shared applications, condition versions, information/document-request decisions | No bank transfer, vehicle acquisition or handover authority |
| insurance | Own-installment underwriting application, versioned additions and decision | No policy, premium, coverage, claims or money movement |
| documents | Uploads, immutable byte versions, scan facts, explicit attachment bindings | Owner domain decides purpose, sharing and business acceptance |

Synchronous authenticated internal HTTP handles owner queries and idempotent commands; RabbitMQ propagates minimal integration facts asynchronously.
Use one contract transport (HTTP/JSON) initially; do not add gRPC, Redis or WebSockets merely because the reference contains them.
Caller-owned durable processes bridge databases. No cross-service SQL transaction, shared mutable business table or distributed lock masquerading as ACID.
The same vehicle may appear in several read views, but ownership/custody/placement/customs/reservation remain separate inventory facts on one identity.
Billing is a permission-filtered composition of commerce and retail invoice views; payment commands route to their originating owner.
Audit is an allowlisted projection of facts, not an alternative mutable truth or permission to operate another party's account.

## 3. Definitive target repository layout

All application paths below are relative to the target root, never this Spec Team repository. This is the approved layout.

```text
go.mod                         # module justixauto; one module, no implied remote
go.sum
services/
  <identity|inventory|commerce|retail|financing|insurance|documents>/
    domain/                    # aggregates, commands/events, replay; no adapters
    app/                       # handlers, local unit-of-work, durable processes
    port/                      # repositories, transaction, owner/service ports
    adapter/{http,postgres,amqp,projection}/
    cmd/{api,projection,worker}/main.go
    contracts/{openapi,events}/ # versioned owner API/schema source + fixtures
    migrations/                # owner-only domain SQL
edge/{cmd/edge,app,adapter}/
pkg/{events,eventstore,outbox,inbox,projection,commands,money,auth,telemetry}/
  # shared mechanics only; reusable SQL templates installed per owner database
web/
  apps/{realization,financing,insurance,admin}/src/{app,routes,features}/
  packages/{ui,tokens,api,session,operations,test-kit}/
infra/local/{postgres,rabbitmq,telemetry,documents}/
infra/compose.yaml
tests/{integration,contracts,e2e}/
tools/                         # preserve go.sh/check-git.sh/documentation tooling
package.json                   # preserve mock tooling; add npm workspaces
package-lock.json              # one coordinator-serialized root lock
docs/justix-auto/               # canonical specs/state/mocks preserved
```

`services/<owner>/domain` imports domain value objects/shared primitives only. `app` imports domain/ports; adapters implement ports; composition roots wire dependencies.
Shared packages cannot import an owner's domain. Apps cannot import another app; shared web packages cannot import apps.
Use Go import-boundary tests/static checks: the chosen short layout has no Go `internal/` enforcement, so this check is a required scaffold acceptance criterion.
Domain events are concrete Go structs; app transactions expose a typed UnitOfWork port, not `*gorm.DB` in domain signatures.
GORM implements PostgreSQL repositories; raw parameterized SQL is allowed inside adapters for conditional inserts/locks/guard updates.

## 4. Technical decision record

| ID | Approved architectural decision | Rationale / superseded alternative |
|---|---|---|
| ADR-01 | Seven owners, one Go module, private DBs, HTTP internal contracts | Align with Gaze's shape without its shared business-DSN assumptions |
| ADR-02 | Full ordered aggregate replay; no event-store snapshots initially | Original Gaze revision has snapshot stubs; CC has active SQL methods but unverified recovery. Optimize only with measured need |
| ADR-03 | Transactional outbox + confirmed persistent AMQP + inbox + sequence-aware projections | Explicit hardening of observed publication/poison-loss gaps |
| ADR-04 | Inventory-only atomic reservation sets; no expiry/partial allocation | Closes retail/wholesale race and prevents timeout-driven overselling |
| ADR-05 | Caller-owned persistent processes with exact cancel/commit decisions | Resolves reservation, branch and submission races without distributed SQL |
| ADR-06 | `/api/v1/{owner}`, string revisions, If-Match, common receipt/error | Replaces incompatible slice routes/body versions/number receipts |
| ADR-07 | Minor-unit integer money strings + versioned currency/policy | Replaces decimal-major DTO; no copied float/demo financial authority |
| ADR-08 | Identity-local credentials/session records; same-origin opaque cookie | Enables atomic new-provider provisioning and live permission checks |
| ADR-09 | Argon2id; TOTP + recovery codes; MFA on catalogued sensitive actions | Preserves sensitive-action requirement without adding MFA to ordinary CRM edits |
| ADR-10 | Private filesystem locally; version-pinned S3-compatible object API for production | No vendor commitment; byte identity cannot change after clean scan |
| ADR-11 | React/strict TS/Vite, Router, TanStack Query, RHF/Zod, CSS Modules, Radix Dialog | Four client workspaces need no React server or new design system |
| ADR-12 | Echo/GORM, amqp091-go, Zap/OTel, golang-migrate | Preserve useful Gaze technologies; maintained AMQP replacement is explicit |
| ADR-13 | Serialized supported-version/security lock before scaffolding | Reference pins are evidence, not current supported production selections |

ADR-13 fixes the package families and selection process here; exact supported patch pins/container digests are a bounded dependency-lock deliverable before code readiness, not unchecked `latest` values.
Use `bash tools/go.sh`; Go >=1.23 is the reference floor, not a production-version recommendation. Wrapper must enforce the subsequently approved compiler pin.
Do not inherit observed RabbitMQ3.13.6, gRPC-dev, unpinned Jaeger or obsolete base-image pins as supported. Node22.23/npm10.9.8/Go1.24.2 were observations only.
Outbox semantics follow the distinction between [publisher confirms and consumer acknowledgements](https://www.rabbitmq.com/docs/confirms); recovery can duplicate deliveries, as [RabbitMQ reliability guidance](https://www.rabbitmq.com/docs/reliability) explains.
The React choice follows [React's client-app guidance](https://react.dev/learn/build-a-react-app-from-scratch), with the explicit routing/data responsibilities selected above.

## 5. Shared values, persistence and envelopes

`ID` = UUID string; `Revision`/sequence/int64 quantities = canonical base-10 strings on JSON, nonnegative signed-bigint range in SQL/Go unless field explicitly allows signed delta.
Money = `{amountMinor:"1840000",currency:"USD"}`; SQL `numeric(38,0)`, Go arbitrary-precision integer; currency dictionary has versioned exponent. No float64/JS Number money or silent FX.
Rates are separately validated decimal strings, not Money; dates are `YYYY-MM-DD`, instants UTC RFC3339, and calculations declare their business timezone.
Every aggregate has `{id,revision,createdAt,updatedAt}`. Tenant-owned records have immutable owner company reference; foreign owner IDs are logical refs, not cross-DB FKs.
Domain events contain secure-record references for PII/notes; encrypted restricted side records hold content. Replay tolerates tombstoned refs; no live PII release before OD-10 retention/consent acceptance.

```json
{
  "eventId":"uuid","eventType":"inventory.reservation.granted.v1","schemaVersion":1,
  "aggregateType":"reservation-set","aggregateId":"uuid","aggregateVersion":"2",
  "integrationSequence":"1","companyId":"uuid","occurredAt":"2026-09-13T10:00:00Z",
  "actor":{"kind":"user","id":"uuid"},"correlationId":"uuid","causationId":"uuid",
  "operationId":"uuid","data":{"holder":{"service":"retail","id":"uuid"}}
}
```

`companyId` may be null only for global identity events. Counterparty IDs are explicit typed data, never universal read grants.
Internal domain event revision is contiguous per `(owner,aggregateType,aggregateId)`. Integration sequence increments only for that aggregate's emitted integration events; internal-only revisions do not create false loss gaps.
Each subscribed integration stream delivers all sequence positions to that authorized service; event handlers may no-op irrelevant types but must checkpoint them. Newly authorized consumers start from an owner-issued checkpoint + permitted snapshot, not presumed sequence zero.
Do not broker-broadcast personal snapshots; authenticated owner ports supply minimum purpose-bound data. Integration envelope/data schemas are allowlisted and versioned.
`events` has unique eventId and `(aggregateType,aggregateId,aggregateVersion)`; event-writing DB roles cannot UPDATE/DELETE history. Upcasters handle old schema, never retroactive edits.
One local transaction checks expected revisions and idempotency, appends all affected streams, updates synchronous guards/private side records/process state, writes integration outbox and commits receipt.
Guards (VIN, capacity, reservation, last-admin, uniqueness) are rebuildable synchronous command indexes, never subscriber-fed authority. Rebuild under fenced writes and compare replayed state before reopening.
Outbox uses one immutable envelope/recipient plan and per-recipient routing, attempts, nextAttemptAt, lease token/until and sentAt under the [approved messaging amendment](state/approvals/messaging-delivery.md). Workers claim recipient rows with bounded leases; lost lease never changes event identity. Implementation readiness is tracked by T-917–T-924; the legacy single-target primitive remains mode-gated. The [approved forward route correction](state/approvals/messaging-route-correction.md) adds T-925: preserve `source.target.fullOwnerQualifiedEventType`, installed migrations and retained identities, with the explicit correction marker required before custody composition.
Relay uses durable queues/exchanges, persistent messages, mandatory routing and publisher confirms; mark sent only after routed confirmation. Reconnect/lost confirm retries identical eventId.
For an explicitly single-consumer direct-ACK composition, inbox unique `(consumerName,eventId)` + effect + checkpoint commit before AMQP ACK. For approved multi-consumer custody mode, intake commits immutable bytes and every admitted consumer job before AMQP ACK; that ACK means durable custody, not completed business effects. Each logical consumer then commits inbox + effect + checkpoint + job completion atomically. Never mix the modes on one queue. Duplicates no-op; gaps reconcile from the authorized owner, never adopt arbitrary future state. The [reviewed amendment](state/drafts/contracts/messaging-delivery.md) defines complete recipient/membership plans, admission cutovers, role separation and retained forward migration.
Malformed/unsupported events go to durable quarantine before original ACK; failed quarantine leaves original unacknowledged. Redrive uses the same inbox/dedup path and keeps repair evidence.
Retry delay: exponential 1s→60s with jitter; alert after five minutes unresolved. Reaching an alert/retry threshold is not business failure, successful cancellation or stock release.
Projection rebuild uses recorded payloads, no latest aggregate lookups or command side effects; build separate generation, compare invariants, atomically switch read pointer.
No exactly-once transport claim, no global ordering across aggregates, no reference-derived snapshot performance claim.

## 6. Consolidated domain data model

Child lists below are typed immutable versions/local child records. Foreign-object references must resolve through an authorized owner port when admitting a command.

| Owner / entity | Required model, state and main constraints |
|---|---|
| identity.Organization | kind seller/bank/mfo/insurance; profileRevisionId; normalized countryKey+registrationKey UNIQUE; access draft/active/suspended; capabilities set; separate complianceRef/integrationRef |
| OrganizationProfileRevision | name/legalName/country `{key,label}`/optional region `{key?,label}`/registration/email/address?/phone?; encrypted restricted record; country change clears region |
| identity.Branch | companyId/name/address?/status active/inactive; setupState none/pending/failed/complete; linkOperationId?; lifecycleFence?; primary warehouse is inventory-owned |
| identity.User/Role | user profileRevisionId, normalized login/email UNIQUE, pending/active/suspended, global roleIds, securityVersion; role name/system/permissionKeys; system role protected |
| identity.Membership | userId+companyId UNIQUE; active/revoked; branchAccess ALL_BRANCHES or SELECTED_BRANCHES+branchIds; **no role field** |
| identity.Auth records | Credential algorithm/hash/parameters; Session tokenDigest/expiry/MFA/context; MfaFactor secret/counter; RecoveryToken digest/use; PlatformAccessGuard singleton; none is a public event payload |
| inventory.VehicleModel | make/model/variant/specificationVersion/specificationRef; exact nine-field specification in HTTP appendix; matching uses IDs/immutable versions, not labels |
| inventory.VehicleUnit | immutable normalizedVin UNIQUE; modelId/specificationVersion; exterior/interior photo-version refs; ownerCompanyId?/custodianCompanyId?; receipt/customs/ownership/custody fact refs; damageBlockerIds; availabilityFence |
| inventory.Warehouse | companyId, branchId?, name, country, region?, city, address, capacity>0, active; UNIQUE nonnull `(companyId,branchId)`; capacity>=occupied |
| inventory.Placement | vehicleId PK, warehouseId, placedAt, receiptBatchId?, revision; one current placement across all companies |
| inventory.ReceiptBatch | companyId/warehouseId/modelId/specificationVersion; confirmedQuantity/identifiedCount/unidentifiedCount/correctedQuantityDelta; receivedAt/evidenceRefs/sourceOrderId?/sourceShipmentId? |
| inventory.ReservationSet | operationId UNIQUE; holder `{service:commerce\|retail,id}`; companyId/items `{vehicleId,modelId}`; held/released/finalized; one held slot per vehicleId; no expiry |
| inventory.Fact | factType/vehicleId/source owner+aggregate+revision/policyVersion/evidenceRefs/occurredAt/recordedAt/actor/reasonRef?; unique source tuple; no generic status edit |
| commerce.Partnership | sorted company pair UNIQUE; requestedByCompanyId; requested/active/declined/withdrawn/ended; reasonRef?/respondedAt?; requester withdrawal is distinct from recipient decline or active ending |
| commerce.SupplierOffer | supplierCompanyId; draft/published/withdrawn; immutable OfferVersion modelLines/route/price/payment/warranty/service/audience; publish acquires no stock |
| commerce.RFQ/QuotationVersion | buyer/supplier/offerVersion?; RFQ draft/sent/negotiating/accepted/closed; immutable numbered quote lines/route/delivery/payment/warranty/service/hash; accepted version immutable |
| commerce.PurchaseOrder/Addendum | source quotation/direct-offer; parties/immutable accepted snapshot/model+quantity lines; awaiting-supplier/accepted/fulfilling/completed/cancelled; allocation unallocated/pending/reserved/failed; mutual versioned addenda |
| commerce.Shipment | orderId/parties/known vehicleIds/route; typed milestones with occurrence/location/responsible/evidence; damage refs; no unknown VIN allocation |
| commerce.Invoice/Evidence | order/issuer/payer/payeeSnapshot/amount/dueDate/schedule; evidence invoiceId/claimedAmount/paidOn/externalReference/bindings/submitted-accepted-rejected/reviewer/reasonRef |
| retail.Customer/Lead | company-local natural person piiRef/profileVersion; lead customer/branch/assignee/source/stage/lossReasonRef/dealId; no cross-company customer dedup |
| retail.Task/Contact | customer/lead?/deal?/owner/dueAt/title/state open-completed; contact channel/noteRef/actor/time, preserving history |
| retail.Listing | company/vehicleId/text/askingPrice; draft/published/withdrawn; never rewrites vehicle facts |
| retail.Deal | company/branch/customer/lead?/vehicleId/reservationSetId?; cash/own-installment/partner-finance; reservation-pending/reserved/reservation-failed/delivery-pending/delivered; condition/doc/invoice/registration/application refs |
| retail.Invoice/Evidence | dealId/purpose vehicle-registration-own-installment-payment/immutable recipient+amount+issue version; same evidence state as commerce, owned separately |
| retail.OwnInstallment | dealId/policy+calculation+terms snapshot/schedule/allocations/servicing state; activation/balances/default/payoff remain OD-09-gated |
| financing.Program | provider/name/draft-published-withdrawn/currentVersion/audience; immutable version currency/terms/eligibility/calculationPolicy refs |
| financing.Application | seller/provider/deal/vehicle/reservation/programVersion; draft/submitted/review/needs-info/terms/agreed/declined; immutable submittedSnapshotId and term version refs |
| financing.Terms/Info | immutable `(applicationId,number)` conditions/calculation/noteRef; information request and additive response versions with exact bindings |
| financing.DocumentRequest | application/title/requirementsRef; requested/review/changes/accepted/cancelled; current submission version; immutable numbered bindings/review history |
| insurance.Application | seller/insurer/deal/snapshot; draft/submitted/review/needs-info/approved/declined; note/response versions/decision author+time; UNIQUE `(sellerCompanyId,retailDealId)` across all states |
| documents.Document/Version | ownerDomain/object/company/purpose; immutable numbered byte identity, digest/length/MIME, uploader/name, scan state/verdict/version/time; exact-byte binding required |

All list indexes begin with authorized company/party scope and the stable sort fields+ID; e.g. leads `(companyId,branchId,stage,assignee,id)` and provider inbox `(providerCompanyId,state,submittedAt,id)` excluding drafts.
Commerce enforces unique accepted quotationVersion→order in the acceptance transaction. Retail has local active-vehicle and converted-lead claims; inventory remains the global race arbiter.
Do **not** impose a one-nonterminal-finance-application-per-deal unique constraint: alternate-provider/repeat rules are an explicit user gate. Idempotency prevents command duplicates only.
Inventory occupied = placed units + active unidentified quantities; identify decreases unidentified and adds placement with zero occupied delta. No fake VIN stock.
Manual receipt accepts known VINs atomically or unidentified quantity; existing canonical units use authorized move/receipt flows, never duplicate VIN creation. See the typed receipt union in the HTTP appendix.
Warehouse moves lock source/destination guards in sorted-ID order; receipt/identification/move update events, capacity and unique placement in the same transaction.
Receipt correction is reasoned delta, not editing confirmed quantity; cannot remove identified units or exceed capacity. Operation evidence/adjustment authorization waits applicable OD-06 approval.

## 7. HTTP, authorization and business workflows

Exact action fields, query shapes and transitions are in [HTTP/domain contracts](contracts/http-domain.md); process commands/state and immutable scan identity are in [cross-owner protocols](contracts/cross-owner.md).
All browser routes `/api/v1/{owner}/...`; internal routes `/internal/v1/{owner}/...`. Only static pages use `/finance/`; backend owner name is always `financing`.
Creates/actions carry Idempotency-Key UUID; existing aggregate writes carry `If-Match: "<revision>"`; missing is428, stale412, invariant/idempotency conflict409, field validation422.
Login/MFA/recovery handshakes use their single-use challenge/session protocol rather than the business idempotency ledger; provider provisioning uses the secret-safe business command contract.
`X-Context-Revision` accompanies authenticated company requests. Body companyId is never authority; context change uses session revision as If-Match and atomically resets scope to ALL.
Synchronous success `{data,revision,operationId}` is the actual commit receipt, never a lagging projection. Multi-aggregate data includes `related:[{type,id,revision}]`.
Async success202 `{operationId,status:"pending"}`; GET `/api/v1/operations/{owner}/{id}` routes to the named owner without an edge registry/database.
GET `/api/v1/{owner}/command-receipts/{key}?command=<name>&target=<id>` recovers lost receipts; actor/company/action scope rechecked. Unknown404 is not proof an in-flight transaction failed; replay identical command/key or reconcile.
Operation view `{operationId,kind,status:pending|succeeded|failed,phase,attentionRequired,result?,error?}`; unknown network outcome remains pending/recoverable, not terminal failed.
Error `{error:{code,message,fields:{},traceId}}`; optional operationId if known. Protected foreign IDs return404, accessible forbidden action403, session401, auth dependency outage503; POLICY_UNRESOLVED403 with action unavailable.
Lists `{items,nextCursor:null|string,asOf}`; default50/max100; projection rows have string revision. Scope before counts/pagination; ID tiebreaker. Multi-source views return source revisions, not a fabricated global revision.
Idempotency key scope actor+company+command+target; same canonical payload hash returns receipt, changed hash409. No raw bodies in ledger; credential routes use keyed secret-safe verification described in the appendix.
Idempotency/tombstone retention cannot be shorter than supported delayed delivery/recovery; do not purge terminal process evidence automatically in this initial implementation.

Identity evaluates live session/user/global permission/membership/branch/capability/access policy; owner separately checks actual party, object state and invariants.
No positive cached authorization admits a new command. Admission before a revocation may complete; new admissions after revocation are denied. This is an explicit in-flight boundary, not cross-service atomic revocation.
Service requests use mutually authenticated TLS identities, allowlisted caller→action grants and purpose-bound accepted operation refs; edge strips supplied actor/permission/internal headers.
Only admitted durable processes may finish/compensate after session expiry; they cannot create a new business intent using expired authority. Human retry of a new action reauthorizes.
Company-wide objects with branchId=null require company permission and remain company-scoped in SELECTED mode; branch-bound objects are filtered to effective authorized branches. ALL never means all platform branches.
Organization operational activation is independent of capability/compliance/legal/API facts; OD-05/11 disputed state combinations fail closed with explicit unavailable action rather than invented policy.
New provider provisioning commits draft company+new user+credential+global company-admin role+membership+safe events atomically; duplicate existing login/email rejects all, no silent linking/reset.
Bootstrap uses deployment-only stdin/secret injection and single-use guard; creates pending MFA enrollment, never a public default Admin account.
Opaque `__Host-justix_session` cookie Secure/HttpOnly/SameSite=Lax/Path=/; Origin+CSRF checks; session rotation at login/MFA/recovery; no browser credential/localStorage tokens.
Proposed limits: idle30m/absolute12h/MFA freshness5m. Argon2id parameters are versioned and benchmarked in security lock; TOTP verifies counter once, recovery codes single-use; no unrestricted MFA recovery bypass.
MFA required for catalogued sensitive operations: platform access/role/credential changes, finance/insurance decisions and terms agreement, payment acceptance, fulfillment and sensitive document downloads. Ordinary CRM contacts/model edits do not require MFA unless explicitly reclassified; unknown permission keys deny by default. Recovery proof/delivery requires security acceptance before release.
Last-admin guard locks one identity row before computing effective active MFA-enabled platform admins; covers roles/user status/MFA removal. Self-removal/blocking is separately rejected.
Changing company clears scope and query epoch; create-company leaves working context unchanged. Membership revocation invalidates only affected context, preserving other memberships.

Confirmed commerce: active partnership for new offers/RFQ/orders; profile-only beforehand; prior contractual records persist after ending. Requested-side decline and requester-side withdraw are separate commands.
RFQ accepted quote+order creation is local-atomic; direct-offer order starts awaiting supplier and grants no early VIN execution. Shipping and payment are separate objects, not a single editable deal status.
Retail sale starts reservation-pending; reservation grant commits sale reserved+lead conversion locally; rejection leaves no active sale. Delivery completion alone wins lead/withdraws listing after inventory completion.
Cash and own-installment progression preserve current UX intent, but unknown registration/insurance/money policies disable affected production transitions. Partner-finance ends at agreement/document exchange.
Finance: draft→submitted→review; review→needs-info/terms/declined; seller response→review; terms→agreed or counter→review. Only addressed provider decides; drafts invisible to provider.
Finance documents: requested/changes→review on a new exact clean version; review→accepted with confirmation or changes with note; requested/changes→cancelled with reason. Review-cancel requires explicit user decision.
Insurance: draft→submitted→review; review→needs-info/approved/declined; seller response→review. Note required on request/response/decision; own-installment, own-company, undelivered sale only.
`agreed`, file `accepted` and insurer `approved` produce no payment/title/policy/handover effects. Separate document scan status never makes a business decision.
Money preview is frontend-fast; authoritative commands recompute versioned approved inputs/policy and verify digest, currency/schedule sum/final balance/date invariants. No demo algorithm fallback in production.

## 8. Frontend architecture and current reference map

Four independent static bundles served at `/`, `/finance/`, `/insurance/`, `/admin/`; matching Vite base/Router basename and prefix-only history fallbacks.
API errors and missing assets never fall back to HTML or another app. Same-origin edge also proxies development Vite prefixes; no cross-app copied runtime state.
Component tree: `AppProviders(QueryClient,SessionContext) → SessionBoundary → AppShell → RouteBoundary → FeaturePage`, with `OverlayHost` and `OperationReceipt` inside the authenticated boundary.
Shared UI: Button/Field/CountryRegionFields, Radix-based Dialog/ConfirmDialog, Table/Pagination, StatusBadge/Tabs/Toast, MoneyText, VehicleIdentity/PhotoGroups, CompanyBranchPicker, DocumentVersions/HistoryStepper.
Domain forms and action placement remain owner-local. Insurance retains its distinct shell layout; sharing primitives does not redesign it.
CSS Modules + token custom properties extracted from current common/scoped CSS; font `Inter,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif`, no assumed bundled font asset.
Initial tokens: canvas#f6f7f9, surface#fff, border#d9dee7, text#18212f, primary#1769e0, success#147a55, warning#a85c00, danger#c23b43, radius6px, sidebar232px, topbar64px.
Responsive sidebar76px applies only at the reference media condition; token extraction task verifies it. No invented dark theme; existing vehicle SVGs remain assets, no raster regeneration.
`M-*` below are new planning references, not approved legacy S-IDs. States must be captured at implementation from the exact entry/file hashes, viewport and fixture.

| M-ID | Proposed route(s) | Current HTML navigation / component responsibility |
|---|---|---|
| M-R01 | `/` | Realization Dashboard/setup → DashboardPage; company/branch prerequisite, OD-03 scoped |
| M-R02 | `/partners/:id?` | Partners → PartnershipPage; incoming accept/decline, outgoing withdraw, active end; OD-04 extras excluded |
| M-R03 | `/purchases`, `/purchases/:id` | Purchases list/detail → PurchasePage; contextual evidence/docs; missing RFQ states OD-13 |
| M-R04 | `/purchases/catalog`, `/purchases/offers/:id`, `/purchases/compare` | Purchase catalog/offer/compare → partner-only offer views |
| M-R05 | `/warehouses`, `/warehouses/:id` | Warehouses/inventory/receipt → WarehousePage; occupancy, add/receive dialogs |
| M-R06 | `/vehicles`, `/vehicles/:id` | Vehicles/details → VehiclePage; shared specs/exterior/interior; default coverage OD-02 |
| M-R07 | `/offers`, `/offers/:id/edit` | Offers retail/wholesale tabs → separate ListingForm/OfferForm |
| M-R08 | `/sales?tab=retail\|wholesale`, `/sales/:id` | Sales/details → SalePage; pending shared reservation, context-specific operations |
| M-R09 | `/sales?tab=installments` | Sales/Installments → ServicingPage; production money actions OD-09 |
| M-R10 | `/crm?tab=leads\|customers\|tasks`, `/crm/leads/:id` | CRM → LeadPage/CustomerForm/TaskForm; qualified sale delegates M-R08 |
| M-R11 | `/billing?tab=suppliers\|customers`, `/billing/invoices/:id` | Bills/payments → InvoicePage; no removed «От партнёров» tab |
| M-R12 | `/settings?tab=companies` | Settings/Companies & branches → CompanyForm/BranchForm; separate context picker |
| M-R13 | `/sales/:id?panel=financing` | Sales financing action → ProviderProgramPicker/Preview/Submit/Terms; no funding |
| M-R14 | `/insurance-requests` | Realization Insurance → SellerInsurancePage; draft and explicit send |
| M-F01 | `/finance/` | Financier Overview → ProviderDashboard; no demo impersonation |
| M-F02 | `/finance/applications`, `/finance/applications/:id` | Applications → FinanceApplicationPage; addressed-side states |
| M-F03 | `/finance/programs` | Programs → ProgramList/Form; version/publish/withdraw |
| M-F04 | `/finance/applications/:id?tab=documents` | Agreed application documents → DocumentRequestPanel/versions/history |
| M-F05 | `/finance/partners`, `/finance/settings` | Existing partners/settings views only, no inferred management/security UI |
| M-I01 | `/insurance/`, `/insurance/applications` | Insurer Overview/Applications → scoped queue |
| M-I02 | `/insurance/applications/:id` | Insurer review detail → UnderwritingPanel; note/decision/response |
| M-I03 | `/insurance/settings` | Existing insurer account summary only |
| M-A01 | `/admin/`, `/admin/companies`, `/admin/companies/:id` | Admin Overview/Companies → CompanyRegistry/Form |
| M-A02 | `/admin/integrations/:type`, `/admin/integrations/:type/:id` | MFO/Bank/Insurer → ProviderForm/AccessActions; credentials first admin |
| M-A03 | `/admin/users`, `/admin/users/:id` | Users → UserForm/global-role/membership rows |
| M-A04 | `/admin/roles` | Roles → RoleEditor; protected system roles and allowlisted permissions |
| M-A05 | `/admin/audit` | Audit → read-only scoped AuditTable |

Exact source-file anchors remain `mock-map.md`: app/realization, vehicle-fields/photos, company-fields/bridge, partner-finance/finance-exchange/documents, insurance-exchange/UI, admin/management and shared directory modules.
Query key `[app,userId,contextRevision,companyId,sortedBranchIds,owner,resource,id,filters,cursor]`; no persistent PII query cache. Runtime-validate generated OpenAPI responses.
Context switch increments local epoch immediately, cancels queries/hides dialogs, performs server CAS and rebuilds scope; discard late old-epoch responses. Cross-tab message carries revision only then reloads session.
Query staleTime15s; focus/reconnect refresh; at most two retries for network/502/503/504 only. Mutations automatic retry off; preserve dirty forms against background data.
Command reducer: edit→validate→confirm→submitting→pending/receipt; validation keeps non-secret inputs, stale412 requires explicit refresh/review,409 preserves draft, unknown network outcome retains same key.
Polling starts2s, backs off10s; reconnect/focus resumes. Persist only operationId/key/command in per-session recovery metadata; no bodies, documents or secrets. Owner receipt lookup recovers after reload.
Closing modal/browser request is not durable cancellation; a supported cancel command is explicit. Radix owns focus trapping/restoration; clicks inside never dismiss, Escape behavior respects processing/dirty state.
Reads retain committed receipt until projection revision catches up; no optimistic authoritative stock decrement. Sign-out/context change clears local data, not the server's durable operations.
Base states loading/empty/filter-empty/read-error/session-expired/forbidden/not-found; mutation states invalid/confirm/submitting/pending/success/error/stale/unknown-outcome. Partial state only for an explicitly approved partial endpoint; current batch endpoints are atomic.
Missing production auth/MFA, pending branch, scan interruption, stale/permission, projection-lag, RFQ/factory/logistics states need captured evidence or user-confirmed supplements before their FE parity tasks. No Figma gate is introduced.

## 9. Infrastructure, verification and build order

Local Compose: seven private PostgreSQL databases/users, RabbitMQ restricted vhost/service credentials, private documents filesystem, optional development OTel collector/Jaeger profile. Redis is not required initially.
Loopback proposals: edge8080; Vite5173–5176; PostgreSQL55432; AMQP5672/management15672; telemetry4317/4318/16686. Service APIs container-private8080. Report port collisions, never kill unrelated processes.
Production packaging: separate non-root owner/edge containers, external PostgreSQL/RabbitMQ/S3-compatible endpoints and injected secrets; TLS edge + mTLS internal. No selected cloud, deployment authority or production topology claim.
Migrations use golang-migrate with owner ledgers; shared templates install into each owner's DB. Startup verifies compatibility/readiness; no automatic destructive production migration/rollback.
Structured Zap logs/OTel traces contain correlation/operation/event IDs, not bodies/secrets/PII; alert oldest outbox age, sequence gaps, quarantine, stuck processes and scanner failures.
Proposed commands after implementation: `bash tools/go.sh test ./...`, `test -race ./...`, `vet ./...`; npm typecheck/lint/test:unit/build:apps/test:e2e; Make infra-up/migrate/test-integration/verify. Existing mock tooling stays intact.
CI scripts are provider-neutral; a hosted CI adapter follows repo hosting approval. Exact dependencies/images/actions lock with dated support/security/compatibility evidence; no automatic deployments.
Unit tests cover aggregate replay, invalid transition, privacy serialization, decimal parsing, immutable snapshots, role/membership/branch matrices and every handler's negative inputs.
Real PostgreSQL tests cover concurrent VIN/reservation/capacity, opposite moves, duplicate accepted quotations, provider rollback, last-admin race, optimistic revisions and insurance uniqueness.
Broker/process tests inject crash before/after local commit, publish confirm, consumer ACK, branch commit and submission consume; duplicate/out-of-order/lost replies must converge without unsafe release.
Document tests include upload overwrite after clean scan, exact generation reads, spoofed metadata, malicious file, scanner outage, foreign scope, draft invisibility and no clean→business-accepted implication.
Frontend uses Vitest/RTL + schema-validated MSW, Playwright four-prefix/party journeys and context-race tests; React Doctor after actual React changes. Visual QA compares captured mock and React state/viewport, one browser-heavy worker at once.
End-to-end negative assertion: finance agreed/document accepted/insurance approved never creates a money movement, title transfer, coverage or vehicle handover.
Build order: version/contract lock → service/storage/event/auth mechanics → first Admin provider journey → branch/warehouse/stock → shared reservation → commerce and retail → finance/insurance/doc exchange → policy-approved fulfillment/servicing.
Parallelize only disjoint owner paths after their contracts freeze; shared lockfiles/schema generators/migration templates have one designated owner. Three workers maximum; one task/branch/worktree, exact-commit QA then coordinator merge.

## 10. Twenty-four first-wave candidate units — preview only

`dev/task-preview.md` is the candidate detail/acceptance reference, **not an exhaustive PO backlog or PM task board**. Estimates2–4h assume listed foundations and require reslicing if larger.
C-01 dependency lock; C-02 one identity Go scaffold; C-03 local PostgreSQL; C-04 local RabbitMQ; C-05 event envelope; C-06 event migration; C-07 conditional append; C-08 ordered replay.
C-09 transactional outbox insert; C-10 confirmed relay adapter; C-11 inbox transaction; C-12 sequence-aware projector; C-13 quarantine; C-14 idempotency ledger; C-15 Money; C-16 messaging crash harness.
C-17 provider organization aggregate; C-18 global roles/role-free membership contracts; C-19 credential adapter; C-20 atomic provider-create command; C-21 **one Admin** React scaffold; C-22 Dialog primitive; C-23 country/region fields; C-24 provider form presentation with fixtures.
The provider journey is not finished at C-24: session/MFA/CSRF/bootstrap, activation/API binding, app admission and independent end-to-end QA remain separate follow-on units.
All candidates are proposed; architecture approval→PO release-backlog approval→PM decomposition precede readiness. Git/security/OD/mock-state gates apply to their exact affected units.

## 11. Open for user — scoped business/security/release decisions

| Gate | Exact unresolved scope; independent work unaffected |
|---|---|
| OD-01 | Partner finance funding/down payment/acquisition/signature/delivery/servicing; application+agreed document exchange can proceed |
| OD-02 / OD-13 | Vehicles default coverage and missing unified RFQ/factory/logistics/microstates; canonical backend/contract fixtures can proceed |
| OD-03 | Universal B2B partnership setup gate for retail-only companies; enforce partnership on B2B, do not gate all retail |
| OD-04 | Partner warehouse/resale, paid cancellation/transfer/addendum policy; basic partnership/offers/RFQ/reservation mechanics can proceed |
| OD-05 / OD-11 | Suspended-company operations and capability/compliance significance; active normal paths/records preserved, disputed combinations unavailable |
| OD-06 | Route/photo/evidence/correction rules; pure VIN/capacity/placement primitives can proceed |
| OD-07 | Registration recipient/tariff/currencies/checklist and delivery prerequisites; no demo GAI invoice authority |
| OD-08 | Insurer decision→coverage/policy/own-installment handover; underwriting ends at decision |
| OD-09 / OD-10 | Approved financial algorithms/real products/consent/PII/retention/storage limits/external APIs; synthetic state machines and secure ports can proceed |
| OD-12 | Security mechanism baseline approved; recovery identity proof/delivery and release configuration still require security acceptance; no silent recovery bypass |
| OD-14 | First-release scope; fifth app/expanded oversight/exceptional override remain excluded unless explicitly selected |
| OD-15 | ADR-01…13 approved; supported exact dependency/image versions remain the serialized pre-scaffold lock deliverable |
| Scoped follow-up | Repeat/alternate-provider finance applications; finance document cancellation during review; CRM reopen/backward transition policy. Gate only those commands, not the entire owners |

The PO release scope checkpoint passed on 2026-09-13; see `state/backlog-approval.md`.
PM creates bounded dependency-linked task files; unresolved rules remain scoped gates.

## User corrections (confirmed)

2026-09-13: user answered "yes" to "Can I treat the proposed architecture as
approved and finish the task breakdown?" No architectural corrections requested.
Approval covers the seven-owner model, four React apps, ADR-01…13 and the two
linked contract appendices as the implementation baseline. It does not approve
unanswered OD business rules, a not-yet-presented release backlog, missing visual
states, production deployment, application implementation or Git initialization.
The exact pre-approval files and hashes are preserved under
`state/backups/2026-09-13-plan-po-r1/`.


## Approved projection durability implementation handoff — 2026-09-15

The [ADR-03/05 storage handoff](state/approvals/projection-storage-handoff.md)
assigns owner-local checkpoint/gap, protected quarantine and generation evidence
storage to T-926–T-928. It preserves private service databases, existing event
and mode history, typed outer transaction composition and separately authorized
bootstrap/retention/process catch-up. Its exact contract and runtime gates
supplement §5; no running application or production policy is implied.


## Approved owner migration implementation handoff — 2026-09-15

The [ADR-03/05/13 compatibility handoff](state/approvals/owner-migration-compatibility.md)
adds exact private artifact history and explicit read-only owner profiles, separate
from shared messaging/storage markers. The approved migrate engine uses a bounded
project pgx database.Driver and immutable verified source, refining the original
upstream-adapter/file-discovery guidance. Existing SQL and legacy constructors
are preserved; actual DDL/finalization faults and later-owner follow-ups remain
required. No schema health check supplies authorization or delivery guarantees.


## Approved auth transport implementation handoff — 2026-09-15

The [ADR-08 transport refinement](state/approvals/auth-transport-compatibility.md)
adds strict shared response decoding, validated ephemeral header delivery,
synchronous structural/semantic boundaries and current session tickets. Ten
bounded tasks include six serial generator stages; partial auth profiles stay
disabled. Terminal generation deliberately bounds newly regenerated Go bodies,
including non-auth/internal outputs; actual network IO is separately bounded.
T-058 security meaning and T-002 acceptance gates remain unchanged. Client
epochs do not undo browser cookies or replace owner revocation/authority.

## Approved durable membership storage handoff — 2026-09-16

The [technical adoption](state/approvals/membership-version-storage.md) adds four
bounded catalog/selection/value/state tasks. Immutable owner catalogs differ
from selected membership and per-stream evidence. Immediate SQL checks and head
locks cover both mutation orders and early constraints; deferred checks alone
are not commit hooks. Original SQL stays unchanged. No remote authority,
retained-history adoption or readiness is inferred from identities. Actual
configuration authorization, T-928 integration and full implementation QA remain.
