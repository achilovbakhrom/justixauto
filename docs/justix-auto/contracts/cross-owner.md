# Cross-owner contracts v1 — subordinate architecture proposal

Status: ARCHITECTURE BASELINE APPROVED 2026-09-13; part of `../architecture.md`, not implemented. OD/security/visual-state gates remain. All fields use its ID/string Revision/Money conventions and all mutations are idempotent.
Private endpoints below use authenticated service identity, allowlisted service/action/purpose and a durable admitted operation reference. Never trust a public holder/company header.

## 1. Persistent operation mechanics

Owner-local `Operation={id,kind,aggregateId,actorId,companyId,admissionRef,intentRevision,requestHash,phase,decision:undecided|commit|abort,attentionRequired,result?,error?,attempts,nextAttemptAt,leaseUntil?,revision}`.
Creation, initiating domain intent, idempotency receipt and outbound work commit together. Consumers commit inbox+phase/domain events+outbound next step together.
One step has stable `(operationId,step)` request identity/hash. Repeat returns stored result; different payload409. Worker leases are scheduling only, never business expiry.
Public view remains status pending during prepare/compensation/reconciliation. Succeeded requires all authoritative commit receipts; failed requires proven reject/abort/compensation, never timeout alone.
Public `/api/v1/operations/{owner}/{id}` delegates to `/internal/v1/{owner}/operations/{id}` with current actor scope. Owner must match its operation ledger; edge stores nothing.
Authenticated same-company operation managers may inspect/resume the same deterministic step; no arbitrary terminal-status edit, force-release, invented override or deletion API.
Retries1s→60s jitter; >5m attentionRequired+alert. Recovery scans durable nonterminal phases; never substitutes a fresh operation ID for an unknown result.
Admission authorizes the exact immutable intent; completion/compensation after session expiration is allowlisted. Changing input is a new user command with fresh authorization and revision.
Decision commit/abort is durable and mutually exclusive. A caller that lost a response queries the deciding owner, not a projection. No automatic decision TTL.
Internal response `{operationId,phase,revision,result?,error?}`; domain-specific states below are exact, not aliases of HTTP failure.

## 2. Inventory reservation set

Only commerce/retail can own reservation requests. `Holder={service:"commerce"|"retail",id:UUID}`; requested company and holder must match the admitted order/deal.

```text
POST /internal/v1/inventory/reservation-operations/{op}/acquire
{holder,companyId,intentRevision,admissionRef,items:[{vehicleId,modelId}]}
-> {operationId,state:"acquired"|"rejected"|"cancelled",reservationSetId?,revision,errorCode?}

POST /internal/v1/inventory/reservation-operations/{op}/cancel
{holder,reservationSetId?:UUID,reason}
-> {operationId,state:"cancelled"|"finalized",reservationSetId?,revision}

GET /internal/v1/inventory/reservation-operations/{op}
-> {operationId,holder,state:"acquired"|"rejected"|"cancelled"|"finalized",reservationSetId?,revision,errorCode?}

POST /internal/v1/inventory/reservations/{setId}/finalize
If-Match: "<set revision>"
{operationId,acquireOperationId,holder,fulfillmentIntentId,policyId,policyVersion,
 disposition:"physical-handover",occurredAt,evidenceBindingIds:UUID[]}
-> {operationId,state:"finalized",reservationSetId,revision,inventoryFactIds:UUID[]}
```

Acquire is all-or-none; nonempty items unique, concrete identified VINs, model/ownership/availability eligible. Atomic sorted vehicle-slot locks + operation guard + set events + outbox.
Grant data `{operationId,holder,reservationSetId,vehicleIds,revision}`; rejection `{operationId,holder,errorCode}`. Event names `inventory.reservation.granted.v1`, `.rejected.v1`, `.cancelled.v1`, `.finalized.v1`; common envelope includes integrationSequence.
Same acquire op/body always yields same recorded result. Cancel-before-acquire persists holder-scoped cancelled tombstone; delayed acquire returns cancelled without acquiring.
Cancel with no setId applies only to that original acquire operation/holder; when setId is supplied it must match. Wrong holder/set409, foreign scope404. It never releases a later set for the same VIN.
Cancellation of held set atomically releases its slots and records cancelled result. Cancel finalized returns finalized, not cancelled; caller cannot reverse actual handover by removing events.
Retail creates pending deal/local lead+vehicle claims; commerce creates pending allocation on an accepted order. Both persist an acquire process, never mark reserved on202 alone.
Matching grant commits owner reserved state; retail also links lead conversion in its same transaction. Reject releases only pending local claims and ends failed.
An abort decision before grant acceptance causes cancel and waits for inventory cancelled result. A late grant while aborting triggers the same cancel, never reactivates the deal.
Public operation cancellation is only `POST /api/v1/{retail|commerce}/operations/{op}/cancel {reason}` for undecided reservation preparation; after reserved, sale/order cancellation is a separate policy-gated business command.
The process's terminal abort requires cancelled/tombstone acknowledgment; a delayed acquire cannot recreate stock hold. Do not purge tombstone while messages/retries remain possible.
Fulfillment intent in caller blocks contradictory cancellation/scheme/financial edits and, in retail, requires no active application-submission fence. Its ID is validated through authenticated caller-owner port.
Inventory finalize locks set/slots/facts; checks exact holder/revision, approved fulfillment policy, receipt/customs/damage, evidence and one-shot fulfillment intent. It commits physical facts+availability fence before caller marks delivered/completed.
Physical departure removes placement/decrements occupancy only when the approved fact says departure occurred. Legal owner is unchanged without a separate approved ownership fact.
Finalized availability fence prevents immediate resale/re-reservation; only a separately admitted eligible receipt/acquisition fact opens it. No grant/finalize event alone transfers title.
Caller completion atomically records retail delivered+listing withdrawn+lead won, or commerce fulfillment fact. A lost completion message replays this one effect.
No automatic reserve TTL; stuck abandoned holds have durable inspection/reconciliation, not guessed expiration. Financier decline/agreement and insurer decisions never independently release inventory.

## 3. Branch plus warehouse setup/attachment

Identity coordinates new branch+existing warehouse, new branch+new warehouse, and existing active branch+warehouse attachment. Only inventory writes warehouse.branchId; no duplicated authoritative link in identity.
A branch-only create (`warehouse.mode=none`) is local201. Linked setup is202; a pending new branch is not selectable for operations or reported as a successfully active branch.
Identity creates Branch with setupState pending and lifecycleFence=op plus an operation; existing active branch receives the same fence. Every attach/detach and branch lifecycle command serializes on that fence.
Identity setup payload freezes company/branch IDs, branch revision, warehouse create/link body and admission; concurrent competing primary attachments409. Branch details can be edited only after fence release in this initial protocol.

```text
POST /internal/v1/inventory/branch-attachments/{op}/prepare
{companyId,branchId,branchIntentRevision,identityIntentId,
 warehouse:{mode:"link",warehouseId,warehouseRevision} |
           {mode:"create",warehouseId,name,country,region?,city,address,capacity}}
-> {operationId,state:"prepared"|"rejected"|"aborted"|"committed",warehouseId?,revision,errorCode?}

POST /internal/v1/inventory/branch-attachments/{op}/commit
{identityDecisionId,companyId,branchId}
-> {operationId,state:"committed",warehouseId,branchId,revision}

POST /internal/v1/inventory/branch-attachments/{op}/abort
{identityDecisionId,reason}
-> {operationId,state:"aborted"|"committed",warehouseId?,revision}

GET /internal/v1/inventory/branch-attachments/{op}
-> same prepare response, plus identityDecisionId?
GET /internal/v1/identity/branch-attachments/{op}/decision
-> {operationId,decision:"undecided"|"commit"|"abort",decisionId?,companyId,branchId,intentRevision}
```

Inventory prepare checks identity's fenced intent, own-company warehouse/branch, supplied warehouse revision and no existing primary link. It atomically reserves warehouse and `(companyId,branchId)` attachment guards.
For create, prepared warehouse exists only as pending setup, no capacity intake/normal query presence. For link, existing warehouse remains usable for stock operations but attachment/capacity/profile edits are fenced; occupancy is never reset.
Prepared link is invisible as a committed primary warehouse. Inventory stores prepare event/result in its transaction; a failure preserves the existing warehouse and stock.
After prepared receipt identity locks operation+branch fence, rechecks its local valid intent and records immutable commit decision. If precommit cancellation/failure won instead, records abort. Once commit chosen it cannot switch to abort.
Inventory commit authenticates/fetches matching identity decision (or validated signed receipt), applies branch link locally and emits `inventory.branch-attachment.committed.v1 {operationId,branchId,warehouseId,revision}`.
Identity consumes matching commit and atomically marks new branch active/setup complete, operation succeeded, releases lifecycle fence. Existing branch stays active throughout, but operation success waits receipt.
Between remote link commit and identity acknowledgment a new branch remains pending: joins/views label setup pending or hide it from usable choices, never show a fully ready branch or orphan successful link.
Abort before prepare creates tombstone. Abort after prepare releases only that op's guards/pending setup; new branch remains durable setup failed, not deleted. Retry of failed creation requires explicit new operation against retained branch, not duplicate branch creation.
Abort after commit decision is disallowed; recovery finishes commit. If response is lost, query inventory/identity authoritative status and retry the decided step. Timeout cannot release fence or create a second warehouse.
Public precommit cancellation: `POST /api/v1/identity/operations/{op}/cancel {reason}`; if decision commit exists returns409 COMMIT_IN_PROGRESS and current operation link, not false cancellation.
Successful detach uses same identity fence/decision protocol with intent `detach` and exact linked warehouse revision; inventory clears branchId only, preserving capacity/stock/history. New attachment waits its completion.
Branch status-changing commands reject while any attachment fence exists. In this first release, inactivation of an attached branch requires completed detach first; no hard branch deletion API is exposed. This prevents suspension/deletion races, not a newly invented cascading delete policy.
Company suspension semantics remain OD-05; no production suspension path is implemented until its matrix includes pending attachment operations. Model preservation/normal active-path tests are independent.

## 4. Finance/insurance submission admission

Application owner (financing or insurance) coordinates; retail owns authoritative source-sale eligibility and a short-lived **durable**, not timed, preparation fence. No other owner reserves VIN anew.
Submitted snapshot freezes seller/customer/vehicle/price/calculation/program and reservation references; PII resides in encrypted application-owner side records, not broker events.
Retail reservation release/finalize/scheme/vehicle/customer/material-price edits must lock the same deal eligibility guard; no such change can bypass an acquired submission intent.
Finance duplicate/repeat provider rules are not settled: do not add a permanent per-deal nonterminal unique constraint. This guard serializes in-flight preparations only. Insurance retains confirmed unique company+sale across states.

```text
POST /internal/v1/retail/submission-intents/{op}/acquire
{applicationOwner:"financing"|"insurance",applicationId,dealId,dealRevision,
 targetCompanyId,kind:"partner-finance"|"own-installment-insurance",admissionRef}
-> {operationId,state:"acquired"|"rejected"|"released"|"consumed",intentId?,
     snapshotRef?,snapshotDigest?,reservationSetId?,reservationRevision?,errorCode?}

POST /internal/v1/retail/submission-intents/{op}/consume
{intentId,applicationDecisionId,snapshotDigest}
-> {operationId,state:"consumed",admissionId,acceptedAt,sourceDealRevision,snapshotDigest}

POST /internal/v1/retail/submission-intents/{op}/release
{intentId?:UUID,applicationAbortDecisionId,reason}
-> {operationId,state:"released"|"consumed",admissionId?}

GET /internal/v1/retail/submission-intents/{op}
-> acquire result plus admissionId?,acceptedAt?
GET /internal/v1/{financing|insurance}/submission-operations/{op}/decision
-> {operationId,decision:"undecided"|"commit"|"abort",decisionId?,applicationId,intentId?,snapshotDigest?}
GET /internal/v1/retail/submission-intents/{op}/snapshot
-> {snapshotRef,snapshotDigest,sourceVersions,authorizedFacts}
```

1. Public submit checks app revision and explicit confirmation. Owner saves submit preparing process and freezes draft edits; draft remains provider-invisible. Respond202.
2. Retail acquire locks deal eligibility, confirms seller/scheme/not-delivered, no delivery/cancellation pending, exact current held reservation and no conflicting prepare fence. It saves immutable minimum snapshot+fence+result together.
3. Retail checks inventory's authoritative exact-holder reservation receipt, not a projection. Because only retail's own processes can release/finalize its set and all lock this eligibility guard, the fence preserves that eligibility until admission. Reject uncertain owner facts503/pending, not guessed eligibility.
4. Application owner fetches purpose-bound snapshot; in one local transaction verifies current provider/program publication/version, approved policy/consent/inputs/recomputed digest and matching intent. Saves immutable encrypted submitted snapshot plus irreversible **commit decision**, application technical phase accept-pending. No submitted broker event/provider visibility yet.
5. Retail consume validates exact caller/application/intent/digest against owner commit decision and local acquired fence. It atomically records admission+snapshot digest and releases prepare fence. `acceptedAt` is the submission eligibility linearization point.
6. Owner receives/reconciles consumed receipt, records business state submitted + admissionId/acceptedAt + integration outbox, and operation succeeds. Providers now see one submitted application, never a duplicate shadow record.
7. Delivery arriving before acquire wins: acquire rejects. Delivery arriving while acquired waits/conflicts. Delivery after consumed is a later action requiring its own approved policy and cannot invalidate the immutable accepted snapshot; it does not mean the provider authorized delivery.
8. Abort is allowed only before owner commit decision. Owner records abort, calls retail release, and waits for release/tombstone before reopening editable draft. Release-before-acquire tombstone rejects delayed acquire; consumed returns consumed and cannot roll admission back.
9. After commit, only consume+complete recovery is legal; no TTL release or draft reset. Lost response/crash queries both decision and intent states with same op; worker finishes the fixed decision. Another app cannot acquire conflicting preparation until prior fence terminal.
10. If undecided owner is reachable, reconciliation may ask it to choose abort on explicit user cancel; unreachable owner leaves pending+attention. This is recoverable blocking, not a hidden perpetual lock: durable decision/status/retry/resume endpoints exist, and no unsafe timeout resolves a truly unknown remote decision.

Public cancel-before-decision: `POST /api/v1/{financing|insurance}/operations/{op}/cancel {reason}`. This cancels submission preparation, not a submitted application or insurance decision; latter policy is not invented.
Provider/program withdrawal after owner's commit decision is a later program action; it does not revoke an already accepted decision. A stale/unpublished program before decision aborts and releases retail fence.
Integration submission event `{applicationId,sellerCompanyId,providerCompanyId,dealId,snapshotRef,snapshotDigest,admissionId,acceptedAt}`; no raw PII. Seller/provider projections both derive from this owner.
Every acquired/consumed/released transition is idempotent, local-atomic with outbox; replay never re-fetches mutable source values into an old snapshot.

## 5. Document byte identity and binding

Local storage port publishes a private immutable finalized file with exclusive create, no symlink traversal, no writable browser path and a distinct staging upload path. Production adapter requires S3-compatible versioned-object reads and exact returned object VersionId, never `latest`.
`ByteIdentity={storageNamespace,objectKey,storageGeneration,sha256,byteLength}` is fixed at upload finalization; object keys/generation are private. Scanner verdict, binding and download must reference this exact identity.
Upload capability can only write its staging key until expiry; finalization either copies into write-once final storage or pins an immutable object generation. Reusing upload after finalization cannot alter the selected bytes or metadata.
`DocumentVersion={id,documentId,number,byteIdentity?,uploaderId,fileName,declaredMime,detectedMime?,createdAt,scanState,scanJobId?,scannerVersion?,signatureVersion?,scannedAt?}`.
Scan state awaiting-upload→quarantined→scanning→clean|rejected|scan-failed. Retry failed scan scans **same byte identity**; a replacement upload creates a new version. An immutable byte version may have a later revoked safety verdict without deleting history.
Scanner reads version-pinned bytes, hashes/counts what it actually scans and returns `{scanJobId,versionId,byteIdentity,result,engineVersion,signatureVersion}`. Digest/generation mismatch rejects result; client-supplied hashes are never scan authority.
Bindings `{id,operationId,versionId,byteIdentityDigest,ownerDomain,ownerObjectId,purpose,partyScopeRef,revision}` unique on exact owner/object/purpose/version; business owner alone admits sharing/review.
Business acceptance validates clean exact binding at admission; scan completion only changes scan state. Later safety revocation blocks new download/binding and alerts owner without silently rewriting a past decision.
Documents asks owner `POST /internal/v1/{owner}/document-authorizations {actorContextRef,action:upload|bind|download,ownerObjectId,purpose,versionId?}` → `{allowed,authorizationId,companyId,partyScopeRef,ownerRevision}`.
Owner validates actor+object+draft/addressed-party scope; documents validates ownership/scan identity and never treats Admin audit or globally held provider role as file access.
`POST /internal/v1/documents/bindings {operationId,versionId,ownerDomain,ownerObjectId,purpose,authorizationId}` → `{bindingId,versionId,byteIdentityDigest,revision}`; response replayable. Business owner persists exact binding ref, never latest-file URL.
Bindings can precede a domain commit; unreferenced bindings remain private/inaccessible because owner authorization is checked per download. Owner projection lag does not authorize newly shared bytes.
Download grant is short-lived, actor/session-bound and single-purpose; `GET /api/v1/documents/downloads/{grantId}` authenticated proxy reauthorizes owner and streams pinned generation. No raw object-key exposure, no long-lived signed clean URL, Cache-Control:no-store, attachment disposition/safe MIME.
In-flight admitted download may finish after revocation; new grants/streams are denied. Report this boundary, not instantaneous recall of already transmitted bytes.
Default local scanner adapter: ClamAV through a private scanner port, exact supported image/engine pin in dependency lock. Scanner outage fails closed; no development no-op adapter in production configuration.
MIME/size/region/retention policy is server-configured and release-approved under OD-10, not demo1MiB. No delete endpoint or replacement of accepted versions. Cleanup of unbound staging uploads requires an approved retention policy and exact targets.
Required attack test: finish upload A→scan clean→replay/overwrite upload with B→download/bind still pins A or rejects; B never inherits A's verdict. Also test mutable-object `latest` access is impossible in storage adapter contract.
