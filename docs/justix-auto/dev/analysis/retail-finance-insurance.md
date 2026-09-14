# Retail, financing, insurance and document storage — SLICE proposal

Date: 2026-09-13. Status: proposed architecture/task inputs, not approved release scope. No application changes. Task candidates are preview units only; formal PO/PM starts after the architecture checkpoint. Read `../../state/architecture-inputs/coordinator-decisions.md`: seven domain owners, one Go module, separate binaries/private SQL ownership, no event-snapshot optimization; framework/pins and common envelope remain synthesis decisions.

## Evidence and limits

Current source root: `/Users/bakhromachilov/startups/justixauto/docs/justix-auto/`. Read `business-logic.md` §§2–4,6–11, `open-decisions.md`, `mock-map.md`, `dev/dev-state.md`, `reference/gaze-reference.md`; inspected current `mocks/{finance-exchange,finance-documents-domain,insurance-exchange}.test.cjs`. Historical scoped support: spec-team `state/drafts/ux-planner/dealer-crm-sales-r1.md` and `state/drafts/finance-post-agreement-r2.md`. Current business handoff overrides older combined retail/bank flow. No browser QA or external regulatory research performed; this document defines software boundaries, not legal/financial guidance.

No current authoritative S-ID contract exists for these React tasks. Map UI to the four current HTML entries and navigation below; do not reuse legacy `screens.md` IDs. Mock tests establish demonstration transitions, not production limits or backend safety. All task candidates stay `proposed`; own Git root is mandatory for implementation but not for this document.

## Ownership and representation

Recommend the intake's boundaries: retail owns CRM, listings, retail sales and its external payment evidence; financing owns programs/applications/conditions/document-request decisions; insurance owns underwriting applications; documents owns private bytes, immutable file versions and scan lifecycle. Do not create a money-moving billing service. B2B invoices/evidence remain commerce-owned. A cross-domain billing view is a read composition, never a new payment aggregate owner.

IDs are opaque UUIDs; versions are unsigned integers serialized as JSON decimal strings (avoid JS precision loss). Timestamps are RFC3339 UTC; due/payment dates are ISO civil dates, never inferred from UTC midnight. Every owned aggregate has `id`, `version`, `createdAt`, `updatedAt`; tenant-owned records include immutable `sellerCompanyId` or `ownerCompanyId`, optional `branchId` consistent with the owning company. Cross-service foreign IDs are references, not cross-database SQL foreign keys. Local children have database FKs and unique constraints. Event payloads below inherit the eventual common envelope, including event/schema/aggregate version, operation/correlation/causation IDs and actor context.

### Retail model

| Aggregate / records | Concrete owned data and invariants |
|---|---|
| `Customer` | `companyId`, `piiRef`, `profileVersion`; natural persons only. Contact names/phones are encrypted PII records, not broker event payloads. No global cross-company customer dedup or contact sharing |
| `Lead` | `companyId`, `branchId?`, `customerId`, `assignedUserId?`, `source: website\|telegram\|phone\|manual`, `stage: new\|contacted\|qualified\|test-drive\|negotiation\|won\|lost`, `lossReason?`, `dealId?`; loss requires reason, conversion keeps original history; sources are labels, not integration adapters |
| `ContactActivity`, `CRMTask` | activity: `leadId?/customerId/dealId?`, `channel`, encrypted `noteRef`, actor/time; task: `customerId`, optional `leadId/dealId`, `ownerUserId`, `dueAt`, `title`, `state: open\|completed`, `completedAt?`; task completion is idempotent and auditable |
| `RetailListing` | `vehicleUnitId`, `companyId`, `text`, `askingPrice: Money`, `version`, `publicationState: draft\|published\|withdrawn`; never owns VIN/location/customs. A withdrawn listing and sold vehicle are distinct facts |
| `RetailDeal` | `companyId`, `branchId`, `customerId`, `leadId?`, `vehicleUnitId`, `reservationId?`, `paymentScheme: cash\|own-installment\|partner-finance`, `fulfillmentState: reservation-pending\|reserved\|reservation-failed\|delivery-pending\|delivered`, immutable price/condition snapshots, contract/document references, registration facts, linked external application IDs. Scheme names are transport distinctions, not legal product classifications |
| `RetailInvoice` | `dealId`, `purpose: vehicle\|registration\|own-installment-payment`, `amount: Money`, recipient party snapshot/reference, `dueDate?`, `issuedAt`, `revision`, state `draft\|issued`; invoice revision never changes accepted evidence. Registration issuance is policy-blocked, not auto-created from a UZS demo constant |
| `PaymentEvidence` | `invoiceId`, `claimedAmount: Money`, `paidOn`, `externalReference`, `attachmentVersionIds[]`, `state: submitted\|accepted\|rejected`, reviewer/time/reason, originating company/deal; acceptance records an external-payment assertion, not a transfer. No bank/MFO customer payment counted as seller revenue |
| `OwnInstallmentContract` | `dealId`, immutable terms/schedule/calculation-policy versions; schedule row number, due date, principal/charge/total Money, allocations; servicing state is independent of delivery. Calculation, activation, allocation/payoff/default commands remain blocked by OD-09 and relevant OD-08/07 gates |

Indexes: retail customer `(companyId,id)`; leads `(companyId,branchId,stage,assignedUserId,id)`; tasks `(companyId,ownerUserId,state,dueAt,id)`; deals `(companyId,branchId,fulfillmentState,id)`; unique active local deal claim `(companyId,vehicleUnitId)` and unique non-null converted `leadId` claim prevent local duplicates but do NOT replace inventory's global VIN reservation invariant. Evidence unique command ID; external bank references are searchable but not assumed globally unique. Money assertions/evidence cannot be edited in place after acceptance; correction needs a separate future policy-approved reversal/correction command, not deletion.

Retail UI business stages are derived from independent facts, not a generic `set-status` command. Cash sequence contract→car-payment→registration-payment→registration→delivery and own-installment sequence insurer-decision→down-payment→contract→registration→delivery are retained as workflow intentions. Unknown registration/coverage/arithmetic rules prevent production progression, not CRM/reserve/data foundations. `partner-finance` ends at agreed conditions/document exchange in this proposal; no retail delivery eligibility follows automatically.

### Financing model

| Aggregate / records | Concrete data |
|---|---|
| `FinanceProgram` | `providerCompanyId`, `name`, `state: draft\|published\|withdrawn`, `currentVersion`, allowed seller audience references, `termsSchemaVersion`; program versions immutable: currency, permitted term choices, rate/markup decimal data, down-payment constraints, `calculationPolicyId/version`, effective dates only if approved. Demo rates/limits are not defaults |
| `FinanceApplication` | `sellerCompanyId`, `providerCompanyId`, `retailDealId`, `vehicleUnitId`, `reservationId`, `programId/programVersion`, `state: draft\|submitted\|review\|needs-info\|terms\|agreed\|declined`, immutable `submittedSnapshotId?`, `latestTermsVersion?`, `agreedTermsVersion?` |
| `ApplicationSnapshot` | seller/customer/vehicle reference versions, encrypted authorized customer/seller facts, price/down/schedule, currency, policy version/hash, reservation proof/version, program version, snapshot digest, submission time. Editing source profile or program never rewrites snapshot |
| `TermsVersion` | `(applicationId,number)` unique; immutable recalculated terms, calculation inputs/result/policy version, noteRef, author/time; only currently offered version may be agreed |
| `InformationRequest`, `ResponseVersion` | applicationId, ordinal, request noteRef, author/time; each response has noteRef and clean attachment version references. App event history is single-source with role-specific labels |
| `FinanceDocumentRequest` | applicationId, title, requirements noteRef, `state: requested\|review\|changes\|accepted\|cancelled`, `currentSubmissionVersion?`; child submissions `(requestId,number)` unique, attachment version ID, uploader/time/noteRef, immutable decision history |

Index financier inbox `(providerCompanyId,state,submittedAt,id)` excluding drafts; seller index `(sellerCompanyId,retailDealId,id)`; child version uniqueness. Recommend one nonterminal financing application claim per retail deal to prevent duplicate accidental submission; terminal retry/new-provider policy is not established and must not be invented. Draft provider change clears program/calculation atomically. Submitted provider and snapshot cannot change. Program withdrawal/version change blocks NEW submissions against stale draft; existing submitted applications/history remain.

### Insurance model

`InsuranceApplication`: `id`, `sellerCompanyId`, `insurerCompanyId`, `retailDealId`, immutable submitted `snapshotId`, `state: draft|submitted|review|needs-info|approved|declined`, note/response versions and decision author/time. Unique `(sellerCompanyId,retailDealId)` across ALL states, matching confirmed one company+sale application. Only own-installment, own-company, not-delivered sale qualifies at create and submit. Draft insurer change is allowed; submission freezes insurer/sale snapshot. Updates after submission are additive response versions. Terminal outcomes cannot be overwritten. No policy number, premium, coverage, claim, payment or handover grant entity is introduced. Index `(insurerCompanyId,state,submittedAt,id)` excludes draft; seller index scoped likewise.

## Exact module-local command contracts

All POST/PATCH mutations require authenticated server context (user/company/branch), `Idempotency-Key` and `{expectedVersion:"n",operationId:"uuid",...}`; create uses expectedVersion `"0"`. Company from request body never overrides context. A command response is `{operationId,aggregateId,aggregateVersion,state,projectionPending:true|false}`; asynchronous reserve/delivery starts return 202, persisted domain changes 200/201. Invalid input 422 `{code,fieldErrors}`; state/version/duplicate conflict 409 `{code,currentVersion}`; unauthorized 403 or existence-hiding 404; unapproved operation 409 `POLICY_UNRESOLVED` with OD IDs in internal/admin diagnostics, not an enabled UI action. Retry same key/body returns original receipt; reused key/different body conflicts. List reads use cursor/limit and return item versions; authorization occurs on detail, list, history and download independently.

Recommended contracts below are architecture proposals; names are not existing endpoints. Base prefixes `/v1/retail`, `/v1/financing`, `/v1/insurance`, `/v1/documents`.

| Method/path under base | Body beyond common envelope | Guard → event/result |
|---|---|---|
| retail `POST /customers` | `{profile:{displayName,phone?}}` | validated company-local PII → `CustomerCreated{id,profileRef,profileVersion}` |
| retail `POST /leads` | `{customerId,branchId,source,assignedUserId?}` | accessible customer/branch/assignee → `LeadCreated` |
| retail `POST /leads/{id}/assign` | `{assignedUserId}` | authorized active assignee → `LeadAssigned` |
| retail `POST /leads/{id}/contacts` | `{channel,note}` | writable lead → `LeadContactRecorded{activityId,noteRef}` |
| retail `POST /leads/{id}/stage` | `{stage,reason?}` | allowed transition, loss reason; won derives delivery, not arbitrary UI → `LeadStageChanged` |
| retail `POST /tasks`; `POST /tasks/{id}/complete` | create `{customerId,leadId?,dealId?,ownerUserId,dueAt,title}`; complete `{}` | local links valid / open → `TaskCreated` / `TaskCompleted` |
| retail `POST /deals` | `{customerId,leadId?,vehicleUnitId,branchId,paymentScheme,price}` | eligible lead/customer + current inventory eligibility → reserve process below |
| retail `POST /listings`; `PATCH /listings/{id}` | `{vehicleUnitId,text,askingPrice}` / changed text/price | inventory reference validated; no VIN mutation → `RetailListingCreated/Edited` |
| retail `POST /listings/{id}/publish`; `/withdraw` | `{}` | publish availability facts/current access → `RetailListingPublished/Withdrawn` |
| retail `POST /deals/{id}/contract-records` | `{documentVersionIds,signedOn,reference}` | records external contract facts only, no e-sign integration → `RetailContractRecorded`; partner-finance path disabled OD-01 |
| retail `POST /deals/{id}/invoices` | `{purpose,amount,recipientSnapshot,dueDate?}` | domain policy chooses permitted purpose → `RetailInvoiceIssued`; registration OD-07 |
| retail `POST /invoices/{id}/evidence` | `{claimedAmount,paidOn,externalReference,attachmentVersionIds}` | authorized party, same invoice currency, clean files → `PaymentEvidenceSubmitted` |
| retail `POST /evidence/{id}/accept`; `/reject` | `{confirmation:true}` / `{reason}` | financial permission, submitted, valid invoice snapshot → `PaymentEvidenceAccepted/Rejected`; installment allocation is NOT an implied side effect |
| financing `POST /programs`; `POST /programs/{id}/versions` | `{name,currency,terms,eligibility,calculationPolicyId,calculationPolicyVersion}` | provider-owned draft/version validation → `FinanceProgramDrafted/VersionCreated` |
| financing `POST /programs/{id}/publish`; `/withdraw` | `{programVersion}` | own provider, publishable configuration → `FinanceProgramPublished/Withdrawn` |
| financing `POST /applications`; `PATCH /applications/{id}` | `{retailDealId,providerCompanyId,programId?,programVersion?,calculationInputs?}` | seller owns reserved deal; only draft editable → `FinanceApplicationDrafted/DraftEdited` |
| financing `POST /applications/{id}/submit` | `{confirmation:true,calculationDigest,consentRecordRef?}` | current eligible published program, reservation, validated policy/PII scope → `FinanceApplicationSubmitted{snapshotId,digest}` |
| financing `POST /applications/{id}/take` | `{}` | addressed financier, submitted → review / `FinanceReviewStarted` |
| financing `POST /applications/{id}/information-requests` | `{note}` | financier, review → needs-info / `FinanceInformationRequested` |
| financing `POST /applications/{id}/responses` | `{requestId,note,attachmentVersionIds:[]}` | seller, needs-info → review / `FinanceInformationResponded` |
| financing `POST /applications/{id}/terms` | `{note,calculationInputs,calculationPolicyVersion}` | financier, review; authoritative recompute → terms / `FinanceTermsOffered{termsVersion}` |
| financing `POST /applications/{id}/counter` | `{termsVersion,note}` | seller, current terms → review / `FinanceTermsRevisionRequested` |
| financing `POST /applications/{id}/agree` | `{termsVersion,confirmation:true}` | seller, current terms → agreed / `FinanceTermsAgreed`; does not claim authenticated buyer signature |
| financing `POST /applications/{id}/decline` | `{reason}` | financier, review → declined / `FinanceApplicationDeclined` |
| financing `POST /applications/{id}/document-requests` | `{title,requirements}` | financier, agreed → `FinanceDocumentRequested` |
| financing `POST /document-requests/{id}/submissions` | `{attachmentVersionId,note?}` | seller, requested/changes, file clean and bound → review / `FinanceDocumentSubmitted{submissionVersion}` |
| financing `POST /document-requests/{id}/accept`; `/return`; `/cancel` | `{submissionVersion,confirmation:true}` / `{submissionVersion,note}` / `{reason}` | financier; accept/return in review; cancel requested/changes → accepted/changes/cancelled and corresponding event |
| insurance `POST /applications`; `PATCH /applications/{id}` | `{retailDealId,insurerCompanyId,note?}` / `{insurerCompanyId,note?}` | own eligible sale; edit draft only → `InsuranceApplicationDrafted/DraftEdited` |
| insurance `POST /applications/{id}/submit` | `{confirmation:true,consentRecordRef?}` | draft + revalidated sale/provider/share policy → submitted / `InsuranceApplicationSubmitted{snapshotId,digest}` |
| insurance `POST /applications/{id}/take` | `{}` | addressed insurer, submitted → review / `InsuranceReviewStarted` |
| insurance `POST /applications/{id}/information-requests`; `/responses` | `{note}` / `{requestId,note,attachmentVersionIds:[]}` | insurer review / seller needs-info → needs-info / review and respective event |
| insurance `POST /applications/{id}/approve`; `/decline` | `{note}` | insurer, review → terminal decision; `InsuranceApplicationApproved/Declined`; no policy/payment/retail-stage event |

GET queries: retail `/customers`, `/customers/{id}`, `/leads`, `/leads/{id}`, `/tasks`, `/deals`, `/deals/{id}`, `/listings`, `/invoices`, `/invoices/{id}`, `/deals/{id}/history`; financing `/programs`, `/programs/{id}`, `/applications`, `/applications/{id}`, `/applications/{id}/history`, `/applications/{id}/document-requests`; insurance `/applications`, `/applications/{id}`, `/applications/{id}/history`. History excludes raw PII/file content and shares only facts authorized for the recipient. Seller/partner labels differ without separate application copies.

Two parity cautions: current mock document cancel rejects review; the broad business description says unfinished requests. Recommend preserve requested/changes cancel now and explicitly decide review-cancel before adding it. Lead stages are confirmed labels but exhaustive backward/reopen transitions are not specified: implement a documented transition map for forward stages/lost, do not invent reopen or successful conversion before delivery.

## Inventory and identity boundaries / process managers

Expected inventory port, names to reconcile with inventory slice: `ReserveVehicle{operationId,dealRef:{domain:"retail",id,companyId},vehicleUnitId,expectedVehicleVersion}` → durable `ReservationGranted{operationId,reservationId,vehicleUnitId,inventoryVersion}` or `ReservationRejected{operationId,code}`. Inventory alone enforces mutually exclusive wholesale/retail reservation and eligibility. No caller-generated VIN. Release/finalize commands require original reservation ID, deal ref and operation ID; inventory must reject another deal's release/finalize and make duplicates harmless. Reservation TTL/expiry must not be invented; if introduced, inventory owns policy and expiry event.

`CreateRetailDeal` transaction claims lead/local deal uniqueness and saves pending deal + durable process state + requested reserve event. The same operation returns pending until grant/reject; no active-sale success or lead conversion event before grant. A grant appends `RetailDealReserved` and `LeadConverted` atomically inside retail; reject releases only local pending claims and marks visible failure. A late grant for a cancelled/failed operation triggers a persisted release compensation, never silently leaks the reservation. Timeout is unknown outcome, not automatic reject; reconciliation queries inventory by operation ID. Cancellation/expiry product rules are unresolved; transport compensation is not authority to cancel a committed sale.

Delivery has a separate durable process: first verify retail policy-approved contract/payment/registration prerequisites; inventory must atomically validate its current reservation/placement/customs/damage facts and finalize. Only the matching inventory completion event records `RetailDealDelivered`, withdraws own listing and marks linked lead won in retail's local transaction. No financier/insurer approval triggers this command. An async fact projection alone is insufficient to grant delivery. OD-01/07/08/09/06 disable their affected delivery paths until resolved.

Finance/insurance submit must not trust a stale projected deal snapshot. Recommended source authorization port: retail `AuthorizeApplicationSubmission{operationId,applicationId,kind,targetCompanyId,expectedDealVersion}` returns an immutable versioned application intent/snapshot ref, including correct scheme/company/reservation and eligibility. Persist the intent; serialize competing sale mutations against active intent, and consume/finalize it idempotently. Provider/program publication eligibility is checked within financing's submit transaction. Share the minimum authorized snapshot via authenticated service calls, not broad raw-PII broker broadcasts. Synthesis must settle the intent lifecycle with inventory/retail so a retry, cancellation or delivered sale cannot admit a stale submission. Financing does not independently reserve or release VIN on declined/agreed without retail-owned policy.

Identity port must authorize actor permission + active membership + branch + capability + correct party + object state at every command/read, not just edge routing. Roles are global; do not introduce per-membership roles. Provider type/access activation is not license/compliance clearance. Preserve suspended-company records; OD-05/11 block disputed suspended-organization access modes only. Synthesis must define precise MFA-sensitive commands (payment acceptance, provider decisions, export/download of sensitive documents are candidate high-risk actions), using identity's common step-up mechanism, not a second login model.

## Money and immutable calculation snapshots

Recommend transport `Money={currency:"USD",amount:"25400.00"}`, decimal string with canonical scale supplied by a versioned currency configuration; Go arbitrary-precision decimal/integer arithmetic only, no float64 or JS Number as authoritative money. Database amounts `numeric(38,9)` with explicit supported-currency/scale checks and overflow rejection; currency codes are not silently converted. API rejects exponent notation, commas, NaN, negative invoice amounts and extra unsupported scale. Precision/storage scale is a technical upper bound, NOT approval of a currency or financial formula. Rates also decimal strings; do not lock real products to demo BPS if partner contract later needs different precision.

`CalculationSnapshot={id,kind:"own-installment"|"partner-program",policyId,policyVersion,programVersion?,currency,input:{price,downPayment,termMonths,firstDueDate,rateParameters},output:{total,financedAmount,schedule:[{number,dueDate,principal?,charge?,total,balance}]},inputDigest,resultDigest,calculatedAt}`. Exact per-policy fields are schema-validated and immutable. Decimal money and event-sourcing performance snapshots are unrelated; Gaze's snapshot store stubs do not implement either application snapshot persistence or production event snapshots.

Frontend computes instant preview using the same versioned algorithm fixture contract, but submit/terms backend recomputes from approved inputs and policy. Reject stale policy/program and mismatched result digest; return authoritative validation details so UI retains input. Enforce no mixed currency, schedule sum + down = approved total, last balance zero, monotonically increasing due dates, and policy-defined nonnegative components. Do not impose partner fixed-markup economics on own-installment annuity or vice versa. Demo calculators may be isolated nonproduction fixtures with explicit `demo-*` IDs; there is no production fallback to demo algorithms. OD-09 blocks own-installment contractual calculation/allocation/activation/payoff/default amounts. OD-10 blocks real partner policy/configuration, PII consent and live submission release, not the generic state-machine implementation with synthetic fixtures.

Retail payment reconciliation compares same-currency accepted evidence to the exact invoice version; partial/overpayment policy and installment allocation are not inferred from sum alone. Recording/reviewing evidence can be built safely; commands deriving installment balances or clearing a schedule row wait OD-09. No fee, penalty, exchange rate, debit, refund or bank-disbursement command exists here.

## Shared documents: storage contract and privacy

`Document`: `id`, `ownerDomain`, `ownerObjectId`, `ownerCompanyId`, `purpose`, `version`; not a business approval. `DocumentVersion`: immutable `id/documentId/number`, private `objectKey`, uploader, sanitized original name, declared/detected MIME, actual byte count, SHA-256, createdAt, encryption key ref, `scanState: awaiting-upload|quarantined|scanning|clean|rejected|scan-failed`, scanner/signature version and result time. Separate `AttachmentBinding` binds exactly one version to domain object/request/submission under explicit allowed party scope; unique binding `(ownerDomain,ownerObjectId,purpose,versionId)`. Index documents by owner context; never expose object keys through ordinary list DTOs. Upload/scan success never produces `FinanceDocumentAccepted` or `PaymentEvidenceAccepted`.

Endpoints:

- `POST /v1/documents/upload-intents {ownerDomain,ownerObjectId,purpose,fileName,declaredMime,declaredBytes,sha256}` → `{uploadId,documentId,documentVersionId,uploadUrl,expiresAt,requiredHeaders}`. Domain authorization determines context before issuing a short-lived, object-specific, size-constrained upload capability; client cannot select another tenant or shared-read scope.
- `POST /v1/documents/upload-intents/{id}/complete {expectedVersion,operationId}` → pending scan receipt. Adapter checks actual existence/size/checksum/content type and then quarantines/scans. Forged metadata or oversized bytes rejected; completion retry is idempotent.
- Internal scanner command `RecordScanResult{versionId,expectedVersion,scanJobId,result,engineVersion,signatureVersion}` is service-authenticated. Only verified clean may be downloaded or attached as a submitted business document. Failure stays unavailable and retryable; infected/rejected does not become clean by user override.
- Internal domain command `BindAttachment{operationId,versionId,ownerDomain,ownerObjectId,purpose,partyScopeRef}` validates domain ownership and clean status and returns binding ID. Business submission references bound exact version, never a mutable latest file URL. Reconciliation of clean→later-revoked verdict must block new downloads and alert owning domain; it does not erase past decisions/history.
- `GET /v1/documents/versions/{id}` returns permitted metadata/scan state. `POST /v1/documents/versions/{id}/download-grants` rechecks domain access on every request and returns short-lived URL (or authenticated stream where revocation requirements demand it). `Cache-Control: no-store`, attachment disposition and safe MIME response; audit grant/download without signed URL/token contents.

Draft files belong only to seller context; financier/insurer sees them only after domain-authorized sharing on submission/response, and only the addressed provider. A financier's global role alone never grants other providers' files. Audit readers/Admin do not inherit document content access. Keep file bytes, customer names/phones, free-text notes, credentials and signed URLs out of immutable event payloads, logs and broker routing keys. Events carry opaque secure-record references + digests/version/audit actor metadata. Sensitive encrypted stores and object keys need lifecycle/deletion/retention design with source snapshots, backups and read projections; OD-10 must establish permissible PII/consent/retention before live data. No claim that encryption alone solves deletion or regulatory compliance.

Allowlist MIME/bytes/storage region/retention/scanner/deployment credentials are explicit configuration with no demo 1 MiB default carried to production. Technical scanner adapter can be implemented/tested before these release settings are approved. Orphan cleanup targets only expired unbound upload intents after a retention window chosen by deployment policy, never canonical business documents; no delete API or implicit replacement of existing accepted versions proposed.

## Reliability and tests

This slice requires durable operation records, optimistic stream versions, command idempotency and duplicate-safe consumers. Coordinator selects transactional outbox + confirm-aware persistent relay + consumer inbox as the PROPOSED hardening ADR, not existing Gaze behavior or user-approved scope. At-least-once delivery only; malformed/gapped events retain durable quarantine evidence. Append, synchronous rebuildable uniqueness guards, operation state and outbox share one local transaction; projection lag cannot authorize decisions. A broker-only reserve workflow cannot be marked ready until that ADR and its crash/replay contracts are approved. Separate service DBs means no distributed SQL transaction; source events plus authenticated snapshot ports carry cross-owner contracts.

Unit: all finance/insurance legal and illegal transitions; provider/seller direction; required reasons/confirmation; immutable snapshot/program/terms/file versions; decimal parsing and policy arithmetic fixtures; CRM conversion/stages; accepted evidence cannot mutate. Property tests: no mixed currency/rounding drift, generated schedules close exactly per approved policy, replay returns same state.

Integration on real PostgreSQL: concurrent sale creates/reserve replies, duplicate command IDs with changed payload, unique company+insurance-sale, stale terms agreement, rollback with event/outbox/idempotency record, resubmission of same file, version-conditioned decisions. Process tests: crash before/after reserve grant and retail commit; duplicate/out-of-order grant/release; lost ACK; query by operation on unknown outcome; delivery finalization replay cannot win lead twice. Cross-domain negative assertions: document clean/accepted, insurer approved and finance agreed emit no money/ownership/handover effects.

Security/attachment tests: wrong company/provider/branch, draft invisibility across list/detail/history/download, guessed object IDs, membership revocation after UI load, spoofed MIME/byte count/checksum, malicious file quarantine, scanner outage, signed-URL leakage, PII absent from events/logs. Use synthetic customers/files; no real partner credentials or financial data. FE contract tests retain input on 422/409, pending queries, no duplicate submit, centered modal cancel makes no mutation; visual tasks require actual current mock state inspection and recorded screenshots, not this source-only analysis.

## Small task candidates (all proposed; 2–4h estimates)

Each row is one coherent implementation unit AFTER shared service skeleton/auth/event-store interfaces exist. A command unit includes its local handler/guards/unit tests, not an entire module HTTP+DB+UI vertical slice. Split any estimate exceeding four hours before readiness. Schema/projector/HTTP/FE adapters are separate tasks using these stable contracts; PM should instantiate individual task IDs, not combine plural variants in one card.

| Candidate | Hours | Immediate prerequisites / operation blockers |
|---|---:|---|
| Decimal Money parser + canonical serialization/overflow tests | 3 | common type convention; no financial formula |
| Calculation snapshot schema + immutable digest tests | 3 | Money; no production policy claimed |
| Customer create command + PII repository port fake tests | 3 | auth/secure-record ports; live PII OD-10 |
| Lead create command | 2 | customer reference port |
| Lead contact command | 2 | secure note port |
| Lead assignment command | 2 | identity assignee authorization |
| Lead stage command | 3 | explicit stage transition map |
| CRM task create command | 3 | customer/lead refs |
| CRM task complete command | 2 | task aggregate |
| Retail listing create command | 3 | inventory read eligibility port |
| Retail listing edit command | 2 | listing aggregate/Money |
| Retail listing publish command | 3 | inventory eligibility; live publication checks |
| Retail deal reserve-intent command | 4 | inventory agreed contract, durable operation store |
| Reserve grant/reject consumer and local-claim transaction | 4 | prior command, inbox/outbox ADR; scope only this callback |
| Late reserve grant compensation handler | 3 | inventory release idempotency contract |
| Reserve unknown-outcome reconciliation adapter | 3 | inventory operation lookup; durable PM |
| External contract-fact record command | 3 | bound clean docs; partner path OD-01 |
| Retail vehicle invoice issue command | 3 | approved invoice/evidence policy fields; registration separate OD-07 |
| Payment evidence submit command | 3 | invoice and document binding ports |
| Payment evidence accept command | 3 | financial permission/step-up; no allocation |
| Payment evidence reject command | 2 | submitted evidence/reason |
| Finance program draft command | 3 | typed versioned policy/config schema; live rates OD-10 |
| Finance program version command | 3 | previous program, immutable version tests |
| Finance program publish command | 3 | audience/policy validation; OD-10 live configuration |
| Finance program withdraw command | 2 | preserve existing submitted applications |
| Finance application draft/create/edit command, each separate card | 3 each | retail reference; provider-change reset on edit |
| Application submission authorization intent in retail | 4 | reconciled cross-service intent lifecycle; not entire submit flow |
| Finance submit command with snapshot port fake | 4 | authorization intent/calculator/program; live submit OD-10 |
| Finance take / information request / response / counter / agree / decline command, each separate card | 2–3 each | finance state fixture; response additionally document bindings |
| Finance terms-offer command | 4 | authoritative calculation port/version; live product OD-10 |
| Finance document request command | 3 | agreed app; OD-01 does not block |
| Finance document submission command | 3 | exact bound clean file version |
| Finance document accept / return / cancel command, each separate card | 2–3 each | required confirmation/reason and current submission version |
| Insurance draft create command | 3 | sale eligibility port; unique company+sale |
| Insurance draft edit command | 2 | draft-only insurer switch |
| Insurance submit command with snapshot port fake | 4 | intent/snapshot; live PII/consent OD-10 |
| Insurance take / info request / response / approve / decline command, each separate card | 2–3 each | addressed party/state guards; OD-08 does not block underwriting decision |
| Upload-intent command + storage signing port fake | 4 | domain authorization contract/config |
| Upload completion verification adapter | 4 | object-store adapter and scan job port |
| Scan-result command / scanner adapter / download authorization adapter, each separate card | 3–4 each | storage fixture; exact adapters/config selected |
| Attachment-binding command + tenant/draft isolation tests | 4 | domain authorization/clean status |
| One aggregate's SQL projection + replay/duplicate tests, per aggregate separate card | 3–4 each | selected events/inbox standard |
| One bounded route adapter + contract tests, per command separate card | 2–3 each | handler/auth/errors; do not bundle full service API |

Oversized/nonready cards to split, NOT 2–4h claims: “CRM”, “retail sale end-to-end”, “financing workflow”, “document subsystem”, “all finance HTTP routes”, “reserve saga”, “installment servicing”, “financial calculator production rules”, “replicate complete mock app”. The complete reserve process needs command, callbacks, compensation, reconciliation and crash integration cards. Complete file exchange needs schema/storage/scanner/ACL/domain binding/FE cards. Frontend each registry, dossier section or command dialog must be scoped to one inspected mock state and shared React primitives; missing transition screenshots block that parity card only.

## Scoped policy blockers and synthesis decisions

| Open item | Block only these operations | Safe to plan independently |
|---|---|---|
| OD-01 | partner-finance down-payment recipient/amount, funding, acquisition/signing ownership, delivery authorization, servicing | programs, application round trip, agreed document requests/versions/review |
| OD-07 | production registration invoice recipient/tariff/currency/checklist, registration completion and cash delivery prerequisites | CRM, reservations, evidence storage/review framework, vehicle invoice contract |
| OD-08 | insurer-approved→coverage/policy/payment/own-installment handover progression | underwriting applications/decision and shared history |
| OD-09 | own-installment authoritative arithmetic/schedule activation/allocation/payoff/default balance effects | Money, preview fixture harness, immutable snapshots, external evidence capture |
| OD-10 | real partner product configuration, consent/PII payload, external integrations/live submission/storage-policy release | generic commands/state machines/private storage contracts and synthetic tests |
| OD-05/11 | disputed suspended/access-vs-compliance reads/commands and permission matrix | active-company normal path, preserving records; no silent choice of suspension policy |
| OD-06 | photo/damage/route-specific fulfillment eligibility at delivery | document/storage foundations and no generic status edit |
| OD-02/13 | missing vehicle/procurement UI projections/parity states used by sale selection | canonical inventory refs and backend sale command tests |
| OD-14 | fifth app/public client, exceptional Admin overrides | four-app current scope; CRM customer entity |

Synthesis must decide: common money/IDs/envelope; per-service DB owners and piiRef/secure store lifecycle; exact inventory reserve/finalize/operation lookup and retail submission-intent protocol; document binding authorization and delayed-scan behavior; reliability hardening ADR; exact MFA permission matrix; financing nonterminal duplicate claim vs future new-application policy; document cancel-review discrepancy. Recommended choices above allow precise parallel module contracts without pretending product-policy gaps are architecture-approved. Unknown real financial/legal choices remain user decisions, operation-gated rather than fabricated.
