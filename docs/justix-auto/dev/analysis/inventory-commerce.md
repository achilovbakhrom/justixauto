# Inventory / commerce architecture slice — proposed, awaiting approval

Date: 2026-09-13. Mode: tech-architect SLICE. This is a coordinator-review draft, not approved architecture, release scope, implementation, or a ready task board. Only this draft is written. No target-repository mutation, application code, Git initialization, browser audit, or subagents.

## Evidence and limits

Source root `/Users/bakhromachilov/startups/justixauto/docs/justix-auto`: `business-logic.md` §§3–6,11; `open-decisions.md`; `dev/dev-state.md`; `reference/gaze-reference.md`; `mock-map.md`; `mocks/realization-domain.test.cjs`; `mocks/vehicle-photos.test.cjs`. Also reviewed this run's `coordinator-decisions.md`; the slice fits its seven-owner/local-transaction constraints. Historical `screens.md` is not the current screen contract. The baseline tests demonstrate demo invariants, not server concurrency or production contract policy. No live UI inspection was performed; FE mapping below is navigation-level only.

Binding direction: Go microservices, CQRS/Event Sourcing in Gaze's ports/adapters shape, React, four apps; no retired dealer/distributor apps, fifth client app, Figma dependency, or money-moving billing service. Root layout, libraries/versions, transport/auth mechanics and infrastructure ADRs belong to synthesis. Local names/DTOs below are recommended contracts requiring adoption there.

## 1. Ownership and transaction boundaries

| Owner | Authoritative state | Explicitly not owned |
| --- | --- | --- |
| inventory | VehicleModel, global VIN→VehicleUnit identity, ownership/custody facts, warehouse placement, receipt batches, capacity, damage/receipt/customs eligibility facts, exclusive reservations | Prices, contract acceptance, invoice payment decision, CRM, retail listing text |
| commerce | Partnership, SupplierOffer versions, RFQ, QuotationVersion, PurchaseOrder/addenda, Shipment and route evidence, B2B invoices/payment-evidence decisions, reservation process manager for wholesale | New copies of VehicleUnit, stock locks outside inventory, retail payment/registration facts |
| retail boundary | RetailDeal/listing and retail reservation process manager; requests inventory operations | Direct placement/reservation table writes; finance/insurance decisions |
| identity boundary | Company/branch identity, permissions, membership, branch scope | Capacity/stock; actor or tenant IDs supplied as unverified user body fields |
| documents boundary | Versioned blobs and scan/access facts | Business approval of receipt, customs release or payment |

One inventory database owns both event streams and strongly consistent command-side guard tables. A command can atomically append several inventory aggregates' events and update the guards in one local transaction. A separate commerce database cannot join that transaction. Read projections are NEVER consulted to guarantee VIN uniqueness, capacity or reservation exclusivity. Proposed synchronous guard indexes are derived from committed events in the same transaction, not a second independently writable business truth; rebuilding them requires a fenced write-maintenance procedure.

Canonical VIN identity survives transfers and changes of tenant visibility. A company buying an existing unit references its ID, never inserts its own unit. Global uniqueness errors must not reveal another company's owner, price, location or documents. Unknown-VIN quantity is not a VehicleUnit with a fake VIN, a sellable line item, or a reservation target. Factory order demand can exist by model+quantity before VIN; that is distinct from allocating physical unidentified receipt stock.

Ownership, custody, placement, reservation, customs release and physical handover are independent facts. Inventory records a validated ownership fact only through an explicitly approved evidence/policy command; no reservation finalization, payment receipt, financing `agreed`, or insurer approval silently transfers legal title.

## 2. Concrete inventory model

Conventions: `ID=UUID`, `Version=uint64` constrained to positive SQL bigint range, `Instant=UTC RFC3339`, quantities SQL bigint/Go int64 with domain `>=0`, required positive quantities `>0`. Every aggregate has `id`, `version`, `createdAt`; every event has actor/correlation metadata per synthesis. References to other services are IDs, not cross-database foreign keys. Child records in the same service have local FKs/indexes.

| Aggregate / command guard | Fields and invariants |
| --- | --- |
| VehicleModel | `id`, `make`, `model`, `variant`, `specificationVersion`, `specification: object`; concrete spec schema follows existing `vehicle-fields.js` contract audit, not arbitrary model matching by label |
| VehicleUnit | `id`, immutable `normalizedVin`, `modelId`, `modelSpecificationVersion`, `ownerCompanyId?`, `custodianCompanyId?`, `ownershipEvidenceRef?`, `custodyEvidenceRef?`, `receiptFactId?`, `customsFactId?`, `damageBlockerIds[]`; unit is never deleted/recreated on transfer |
| VIN registry guard | `normalizedVin PK`, `vehicleId UNIQUE`; global across companies. VIN parser accepts only the approved normalized syntax; recommended MVP 17 uppercase ASCII VIN characters excluding I/O/Q, trim whitespace, reject invalid symbols; regional check-digit/WMI rules require explicit policy, never guessed |
| Warehouse | `id`, `companyId`, `branchId?`, `name`, `capacity>0`, `active:bool`; no zones/rows/bins. Capacity decrease below occupied rejected. `branchId` means the optional primary attachment; partial unique `(companyId,branchId) WHERE branchId IS NOT NULL` ensures at most one |
| Warehouse command guard | `warehouseId PK`, `capacity`, `occupied`, `version`; CHECK `0<=occupied AND occupied<=capacity`. Updated atomically with receipt/move/departure events; verified against placement+batch facts |
| Placement | `vehicleId PK`, `warehouseId`, `placedAt`, `receiptBatchId?`, `placementVersion`; at most one current placement globally. Historical movement is event history; no separate dealer/distributor placement |
| WarehouseReceiptBatch | `id`, `companyId`, `warehouseId`, `modelId`, `modelSpecificationVersion`, `confirmedQuantity>0`, `identifiedCount>=0`, `unidentifiedCount>=0`, `correctedQuantityDelta:int64`, `state:active\|exhausted`, `receivedAt`, `receiptEvidenceRefs[]`, `sourceOrderId?`, `sourceShipmentId?`; `identifiedCount+unidentifiedCount=confirmedQuantity+correctedQuantityDelta`; identified count denotes units ever identified from this batch, not units still present |
| ReservationSet | `id`, `operationId UNIQUE`, `holderService:commerce\|retail`, `holderId`, `companyId`, `items:[{vehicleId,expectedModelId}]`, `state:held\|released\|finalized`, `reason?`, `finalizationRef?`, `version`; one or many concrete units; no unidentified units; no automatic expiry in initial proposal |
| Reservation slot guard | `vehicleId PK`, `reservationSetId`, `holderService`, `holderId`, `availabilityFence`; held reservations occupy the slot; finalization closes it together with an inventory availability block, never makes a sold/handed-over unit automatically reservable |
| Reservation operation guard | `operationId PK`, `holderService`, `holderId`, `requestHash?`, `state:acquired\|cancelled\|rejected\|finalized`, `reservationSetId?`, `result`; cancellation before acquire creates tombstone. Tombstones retained for supported message/idempotency lifetime; never purged while delayed commands remain possible |
| Inventory fact | typed receipt/customs/damage facts retain `id`, `vehicleId`, `sourceService`, `sourceAggregateId`, `sourceVersion`, `evidenceDocumentVersionIds[]`, `occurredAt`, `recordedAt`, `actorId`, `reason?`, `policyVersion`; no generic writable status enum. Imported facts dedup by `(sourceService,sourceAggregateId,sourceVersion,factType)` |

Receipt capacity arithmetic: occupied = currently placed units + active batches' unidentified counts. Receiving `q` unidentified units increments occupancy by `q`. Identifying one VIN adds one placement and decrements unidentified by one, so occupancy delta is zero. Moving an identified unit decrements source and increments destination once. A rejected receipt logs its reason/evidence but does not create accepted usable stock; physical quarantine/storage of rejected goods is not defined here and cannot be invented as saleable placement.

Receive/move algorithm: validate actor/object/company scope; lock affected warehouse guard rows in sorted-ID order, then unit/batch guards in a consistent order; recheck capacity/version/model; append events and update occupancy/placement/VIN index in one transaction. Identify creates VIN index+unit+placement and updates batch counters atomically; VIN collision or wrong model rolls back all changes. Multiple warehouse locks avoid opposing-transfer deadlocks; serialization/deadlock retries stay bounded within the idempotent command, never return false success. Quantity corrections are separate reasoned events; positive delta checks capacity, negative delta cannot remove already identified units. Approval of adjustment permission/evidence is a narrowly scoped gate, not permission to mutate confirmed quantity freely.

## 3. Inventory command and query contracts

Proposed edge prefix `/api/inventory/v1`; internal operations under `/internal/inventory/v1`. All mutations require `Idempotency-Key: <UUID>`, object `expectedVersion` (create `0`), authenticated server context, and where applicable `operationId`. Same authorized scope+key+request hash returns the stored result; changed payload returns `409 IDEMPOTENCY_CONFLICT`. Responses: synchronous durable command `200/201 {aggregateId,version,operationId,projectionPending:true}`; cross-service work `202 {operationId,state:"pending"}`. Errors `{code,message,fieldErrors?,currentVersion?,operationId?,retryable}`; auth failures cannot disclose foreign object details; `409 VERSION_CONFLICT|VIN_UNAVAILABLE|CAPACITY_EXCEEDED`, `422 INVALID_TRANSITION|INVALID_VIN|MODEL_MISMATCH|POLICY_NOT_CONFIGURED`, `503 DEPENDENCY_UNAVAILABLE`. JSON version/int64 values should use the synthesis-safe encoding; do not lose precision in React.

| Method/path | Command body → committed domain event |
| --- | --- |
| POST `/warehouses` | `{name,capacity,branchId?,expectedVersion:0}` → `WarehouseCreated` |
| POST `/warehouses/{id}/capacity-changes` | `{expectedVersion,capacity,reason}` → `WarehouseCapacityChanged {before,after,reason}` |
| POST `/warehouses/{id}/receipt-batches` | `{expectedWarehouseVersion,modelId,modelSpecificationVersion,quantity,receivedAt,evidenceDocumentVersionIds,sourceOrderId?,sourceShipmentId?}` → `ReceiptBatchAccepted {batchId,warehouseId,quantity,...}` + occupancy update; evidence gate OD-06 |
| POST `/receipt-batches/{id}/identifications` | `{expectedVersion,items:[{vin,modelId}],atomic:true}` → `BatchUnitIdentified {batchId,vehicleId,normalizedVin,warehouseId,modelId}` per unit; full rollback on any item error. Repeating smaller batches supports partial identification over time without partial commit ambiguity |
| POST `/receipt-batches/{id}/quantity-adjustments` | `{expectedVersion,delta,reason,evidenceDocumentVersionIds}` → `ReceiptQuantityCorrected`; permission/evidence rule must be approved |
| POST `/vehicle-units/{id}/warehouse-moves` | `{expectedVersion,fromWarehouseId,toWarehouseId,occurredAt,evidenceDocumentVersionIds}` → `VehicleWarehouseMoved`; same-company physical relocation only; cross-company/title transfer is separate and policy-gated |
| GET `/warehouses?cursor&limit` | `{items:[{id,name,branchId,capacity,occupied,free,projectionVersion}],nextCursor,asOf}` company/scope filtered |
| GET `/warehouses/{id}/inventory?cursor&limit` | `{units:[{vehicleId,vin,modelId,placement,availability,sourceVersion}],unidentifiedBatches:[{batchId,modelId,quantity}],nextCursor,asOf}` no fabricated VINs |
| GET `/vehicle-units/{id}` | `{id,vin,model,ownershipFact?,custodyFact?,placement?,receiptFact?,customsFact?,damageBlockers,availability,sourceVersion,asOf}` field-level evidence access enforced |
| GET `/vehicle-units?placement=warehouse\|outside\|any&warehouseId&cursor&limit` | typed placement predicate; OD-02 blocks selecting the app's default registry predicate, not this query model |

Internal reservation contract (service authentication plus authorized company/deal intent; no public arbitrary holder/body authority):

```text
POST /internal/inventory/v1/reservation-operations/{operationId}/acquire
{holder:{service:"commerce"|"retail",id:UUID}, companyId:UUID,
 items:[{vehicleId:UUID,expectedModelId:UUID}], intentVersion:uint64,
 authorizationContextRef:ID}
→ {operationId,state:"acquired"|"rejected"|"cancelled",reservationSetId?,version?,errorCode?}

POST /internal/inventory/v1/reservation-operations/{operationId}/cancel
{holder:{service,id}, reservationSetId?:UUID, reason:string}
→ {operationId,state:"cancelled"|"finalized",reservationSetId?}

POST /internal/inventory/v1/reservations/{setId}/finalize
{operationId:UUID,holder:{service,id},expectedVersion:uint64,
 fulfillmentAuthorizationRef:ID, disposition:"physical-handover",
 occurredAt:Instant,evidenceDocumentVersionIds:[UUID]}
→ {operationId,reservationSetId,state:"finalized",version,inventoryEventIds:[UUID]}

GET /internal/inventory/v1/reservation-operations/{operationId}
→ {operationId,state:"acquired"|"rejected"|"cancelled"|"finalized",reservationSetId?,version?}
```

All-or-none acquire sorts/locks units, rejects duplicates/wrong model/unknown VIN/unavailable unit, and stores deterministic result. No partial reservation set. No new reservation attempt with a new ID until the old operation has a terminal result. Cancel is idempotent and verifies exact holder/set; it cannot free a newer reservation for another deal. Finalize must remain unavailable until fulfillment authority/policy is approved for that specific flow (OD-04 wholesale, retail gates on the other slice). A finalized conflict on cancel is not `cancelled`; caller must reconcile completion, never compensate physical handover with an inverse reservation. Finalization removes placement/decrements occupancy only if an approved physical-departure fact actually occurs; ownership stays unchanged without a separately authorized fact. The resulting availability fence prevents re-reservation until an explicit eligible acquisition/receipt transition. A delayed operation cannot clear the fence.

## 4. Commerce entities and lifecycles

| Aggregate/entity | Concrete fields and lifecycle |
| --- | --- |
| Partnership | `id`, `companyLowId`, `companyHighId`, `requestedByCompanyId`, `status:requested\|active\|declined\|ended`, `respondedAt?`, `reason?`, `version`; UNIQUE sorted company pair; activate only other party's acceptance; no self partnership |
| SupplierOffer | `id`, `supplierCompanyId`, `currentDraftVersion`, `publishedVersion?`, `status:draft\|published\|withdrawn`; `OfferVersion {id,offerId,number,modelLines[],route,priceTerms,warrantyTerms,serviceTerms,audience:{mode:all-active\|selected,partnerCompanyIds:[]},createdAt}` immutable after publish; publishing does not acquire stock |
| RFQ | `id`, `buyerCompanyId`, `supplierCompanyId`, `offerVersionId?`, `requestedLines:[{lineId,modelId,quantity>0}]`, `status:draft\|sent\|negotiating\|accepted\|closed`, `acceptedQuotationVersionId?`, `version`; quotation submission establishes negotiating; later terms do not overwrite submitted versions |
| QuotationVersion | `id`, `rfqId`, `number`, `supplierCompanyId`, `lines:[{lineId,modelId,quantity,unitPrice}]`, `currency`, `route`, `deliveryTerms`, `paymentSchedule:[{id,amount,dueDate}]`, `warrantyTerms`, `serviceTerms`, `createdAt`, `createdBy`, `contentHash`; UNIQUE `(rfqId,number)`; immutable proposal payload; acceptance event references exact ID/hash |
| PurchaseOrder | `id`, `buyerCompanyId`, `supplierCompanyId`, `rfqId?`, `quotationVersionId?`, `offerVersionId?`, `source:quotation\|direct-offer`, `acceptedTermsSnapshot`, `lines:[{id,modelId,quantity,assignedVehicleIds:[]}]`, `route:factory\|foreign-direct\|in-transit\|local`, `status:awaiting-supplier\|accepted\|fulfilling\|completed\|cancelled`, `reservationOperationId?`, `reservationSetId?`, `allocationState:unallocated\|pending\|reserved\|failed`, `version`; accepted terms immutable; an order can exist pre-VIN without allocation |
| OrderAddendum | `id`, `orderId`, `baseOrderVersion`, `proposedTermsSnapshot`, `proposerCompanyId`, `status:proposed\|accepted\|rejected`, `acceptorCompanyId?`, `reason?`; mutual acceptance does not rewrite original terms; inventory-impacting addenda need their own allocation saga and are not silently applied |
| Shipment | `id`, `orderId`, `supplierCompanyId`, `buyerCompanyId`, `vehicleIds:[]`, `route`, `milestones:[{id,type,occurredAt,location,responsibleActorId,evidenceDocumentVersionIds}]`, `damageBlockerIds`, `version`; concrete VINs only; no general `PATCH status`; route transition/evidence matrix blocked OD-06/13 |
| B2BInvoice | `id`, `orderId`, `issuerCompanyId`, `payerCompanyId`, `payeeSnapshot`, `currency`, `amount`, `dueDate`, `schedule:[{id,amount,dueDate}]`, `version`; records obligations, does not move money |
| PaymentEvidence | `id`, `invoiceId`, `submittedByCompanyId`, `amount`, `currency`, `externalReference`, `paidAt`, `documentVersionIds[]`, `state:submitted\|accepted\|rejected`, `reviewerId?`, `reviewNote?`, `version`; buyer submits, seller financial permission accepts/rejects; rejection requires reason, acceptance audited; accepted factual sums derived, not client-writable `paid` field |

Money shape proposed `{amountMinor:string,currency:string}` backed by integer arithmetic and approved currency precision metadata. Never aggregate mixed currencies or apply FX implicitly. Price/schedule validation can be implemented as structural same-currency/sum checks after precision contract approval; payment allocation, overpayment/refund and handover eligibility are separate unresolved policy, not copied cents/USD/UZS demo assumptions. Cross-currency proof or ambiguous external reference routes to validation/review, not auto-credit.

Partnership gate applies to new RFQs/offers visibility/orders and must be checked at their command/visibility boundary, not only login. Existing order participants keep access to their contractual records after partnership ends; company suspension access remains OD-05, do not override it with this partnership rule. Stock/price exposure is restricted to active partnership plus offer audience. This does not grant free partner-warehouse browsing or resale rights (OD-04).

## 5. Commerce contracts and domain events

Use shared command headers/error/receipt conventions above, prefix `/api/commerce/v1`. Company IDs for counterparty selection are verified against server actor context. Commands touching multiple commerce aggregates use one local transaction with expected versions. Economic changes never piggyback on unrelated inventory events.

| Method/path | Body → event / explicit guard |
| --- | --- |
| POST `/partnerships` | `{counterpartyCompanyId,expectedVersion:0}` → `PartnershipRequested` |
| POST `/partnerships/{id}/accept` | `{expectedVersion}` → `PartnershipActivated`; only requested party |
| POST `/partnerships/{id}/decline` or `/end` | `{expectedVersion,reason}` → `PartnershipDeclined` / `PartnershipEnded`; preserve existing records |
| POST `/offers` | `{expectedVersion:0,modelLines,route,priceTerms,warrantyTerms,serviceTerms,audience}` → `OfferDraftCreated` |
| POST `/offers/{id}/versions` | `{expectedVersion,...versionFields}` → `OfferVersionDrafted`; no overwrite published version |
| POST `/offers/{id}/publish` | `{expectedVersion,draftVersionId}` → `OfferPublished {offerId,versionId,audience}`; no reserve side effect |
| POST `/offers/{id}/withdraw` | `{expectedVersion,reason}` → `OfferWithdrawn`; accepted order snapshot unchanged |
| POST `/rfqs` | `{expectedVersion:0,supplierCompanyId,offerVersionId?,requestedLines}` → `RFQDraftCreated` |
| POST `/rfqs/{id}/send` | `{expectedVersion}` → `RFQSent`; active partnership |
| POST `/rfqs/{id}/quotation-versions` | `{expectedVersion,lines,currency,route,deliveryTerms,paymentSchedule,warrantyTerms,serviceTerms}` → `QuotationVersionSubmitted {rfqId,quotationVersionId,number,contentHash}`; supplier only |
| POST `/rfqs/{id}/accept-quotation` | `{expectedVersion,quotationVersionId,contentHash}` → `QuotationAccepted` + `PurchaseOrderCreated`; buyer only, exact current acceptable version; allocation separately pending when concrete VINs selected |
| POST `/orders` | `{expectedVersion:0,offerVersionId,lines}` → `DirectOrderRequested`; awaiting supplier, no early VIN-execution access |
| POST `/orders/{id}/supplier-confirmations` | `{expectedVersion}` → `OrderSupplierConfirmed`; supplier only; later allocation command may acquire units |
| POST `/orders/{id}/allocations` | `{expectedVersion,operationId,items:[{orderLineId,vehicleId}]}` → `OrderAllocationRequested`; accepted order, counts/models checked; `202` until inventory result, no synthetic units |
| POST `/orders/{id}/addenda` | `{expectedVersion,proposedTermsSnapshot,reason}` → `OrderAddendumProposed` |
| POST `/orders/{id}/addenda/{addendumId}/accept` | `{expectedVersion,expectedAddendumVersion}` → `OrderAddendumAccepted`; counterparty and current base version; changed VIN/quantity uses separate gated saga |
| POST `/orders/{id}/cancellations` | `{expectedVersion,reason}` → `OrderCancellationRequested`; policy gate OD-04, final cancelled only after reservation cancel confirmation |
| POST `/orders/{id}/shipments` | `{expectedVersion,vehicleIds,route}` → `ShipmentCreated`; assigned/reserved known units only, no duplicate active shipment allocation; OD-13 UX gate, OD-06 transition gates |
| POST `/shipments/{id}/milestones` | `{expectedVersion,milestoneType,occurredAt,location,responsibleActorId,evidenceDocumentVersionIds,policyVersion}` → `ShipmentMilestoneRecorded`; only allowed route-specific types; policy absent means no production transition |
| POST `/shipments/{id}/receipt-decisions` | `{expectedVersion,decision:accept\|reject,vehicleIds,warehouseId?,reason?,evidenceDocumentVersionIds}` → `ShipmentReceiptRequested` or `ShipmentReceiptRejected`; accepted physical placement completes through inventory command, not merely a commerce flag; reject reason required |
| POST `/invoices/{id}/payment-evidence` | `{expectedVersion,amount,currency,paidAt,externalReference,documentVersionIds}` → `PaymentEvidenceSubmitted` |
| POST `/payment-evidence/{id}/accept` or `/reject` | `{expectedVersion,note}` → `PaymentEvidenceAccepted\|Rejected`; seller financial permission, evidence not arbitrary seller payment on buyer's behalf |
| GET `/orders/{id}` | `{order,termsSnapshot,allocation:{state,operationId,reservationSetId?},shipmentSummaries,paymentSummary,allowedActions,sourceVersion,asOf}` existing participant-only records |
| GET `/offers`, `/rfqs`, `/orders`, `/shipments/{id}` | cursor-filtered typed views with party/company visibility and `sourceVersion,asOf`; no undeclared universal “deal” state |

Order acceptance atomicity recommendation: quotation acceptance and order creation live in commerce and commit together, unique `(quotationVersionId)` prevents duplicate orders from double-submit. Direct-offer acceptance is distinct; choosing it does not remove RFQ support. Factory demand lines are valid before VIN, but cannot generate shipment or reserve unidentified batch quantity. Structural shipment creation can be planned separately from policy-blocked physical transitions.

## 6. Cross-service process managers, idempotency and compensation

Process state in the requesting service: `operationId PK`, `kind`, `aggregateId`, `intentVersion`, `requestHash`, `step`, `inventoryReservationSetId?`, `lastAttemptAt`, `nextAttemptAt`, `attemptCount`, `lastErrorCode?`, `cancelRequestedAt?`, `version`. It is durable business coordination, not a goroutine/localStorage lock. Creation and intent event commit in the service's own transaction. A new public sale/order is not reported “reserved/ready” while this operation is unresolved.

1. Commerce/retail validates its own command, locks the relevant deal intent, persists pending state+operation and reliable outbound request (if reliability ADR approved). Inventory rechecks authoritative unit ownership/availability/model plus valid actor/service authorization, then grants/rejects all units atomically.
2. Inventory stores operation result and `ReservationGranted {operationId,reservationSetId,holder,companyId,vehicleIds,reservationVersion}` or `ReservationRejected {operationId,holder,code}`. Consumer dedups event IDs and correlates holder+operation; matching pending deal becomes reserved. Timeout alone is not rejection; query authoritative operation or retry identical request.
3. If deal validation has changed or user cancellation is accepted before result, saga moves to `compensating`, issues cancel for exact operation/holder, and waits for `ReservationReleased` / cancelled tombstone acknowledgement before final failed/cancelled state. An acquire arriving after cancellation is rejected by tombstone; a grant arriving late triggers/retries cancel, not activation.
4. Finalization starts from a persisted fulfillment intent that freezes mutually exclusive cancellation/financial edits in the originating aggregate. Inventory requires approved disposition eligibility, correct reservation version/holder and clear damage/receipt/customs blockers. `ReservationFinalized {operationId,reservationSetId,holder,vehicleIds,disposition,inventoryFactIds}` drives completion; originating service cannot mark final completion first.
5. Crash after inventory commit but before response: retry/query yields same result. Crash after remote success but before local state: replay resumes the pending saga; never reacquire with a new operationId. Conflict after actual physical completion enters `reconciliation-required`, preserving facts. Do not invent admin override or reverse a physical handover by deleting events.
6. Timeout/retry budget exhaustion leaves a visible pending/reconciliation state and operational alert; not automatic release of a potentially completed sale. Retry cadence and alert SLO belong to synthesis. No time-based reserve expiry until duration/renewal/race policy is approved.

Receipt cross-service variant: commerce records `ShipmentReceiptRequested {operationId,shipmentId,vehicleIds,warehouseId,evidenceRefs,policyVersion}`; inventory atomically confirms capacity/placement and emits `InventoryReceiptAccepted` or rejection result; commerce marks receipt only on authoritative success. No capacity is consumed by merely queuing the request. Shipment departure/location events do not blindly set placement or legal owner. The trusted commerce→inventory evidence adapter is allowlisted by fact type and policy version; replay does not rerun external evidence approvals.

Reliability is conditional on the proposed hardening ADR: atomic event+outbox+operation write, publisher confirms/persistent delivery, recoverable relay, consumer inbox dedup transactionally with state, explicit dead-letter recovery. Gaze inspected publication has post-commit goroutines, ignored errors, no demonstrated durable outbox/inbox, and unsafe poison-message retention assumptions. Without the ADR or an equivalently approved durable mechanism, these workflows cannot claim eventual delivery/exactly-once effects and stay not ready. No distributed SQL/2PC is proposed. At-least-once delivery with idempotent effects, not exactly-once transport.

Public integration event envelope recommendation: `{eventId,eventType,schemaVersion,aggregateId,aggregateVersion,companyIds,actorId,occurredAt,correlationId,causationId,operationId?,payload}`. `companyIds` aids authorized routing, not authorization by itself. Integration events expose only required facts, no full commercial/PERSONAL snapshots; event type payloads above must be versioned in module-local `pkg/contracts`. Contracts requiring synthesis coordination: auth reference verification, serialization/version format, document evidence acceptance, transport, reliable relay and replay cursor.

## 7. Independent projections and UI scope

Inventory maintains `warehouse_summary` (capacity/occupied/free), `warehouse_inventory` (placed units+unidentified batches), `vehicle_registry` (canonical units with placement), and availability/detail views. Commerce maintains partner/offer/RFQ/order/shipment views; retail listing is an independently updated projection referencing canonical vehicle ID. The same VIN may appear in warehouse and registry views, never twice as inventory. Each row carries source aggregate version; cross-aggregate view reports per-source versions or `asOf`, not a fabricated global event order.

Projectors process recorded event payloads without loading the latest aggregate to reconstruct historical state. Inbox and projection change/checkpoint commit together; duplicates no-op, version gaps trigger ordered catch-up before advancing. Cross-aggregate views combine independent source versions. Read-your-writes: command receipt includes affected IDs+versions; React preserves pending result and polls/refetches until required source versions appear; it must not decrement local stock optimistically as authoritative confirmation. Rebuild into a new projection generation, compare counts/invariants, then swap reads; never replay business commands/side effects during query rebuild.

Navigation mapping from current mocks: Realization sidebar Warehouses→inventory/receive; Vehicles→unit details/photos; Partners; Procurement/orders; Offers; contextual shipment, invoice and documents. No stable current S-ID mapping was established; assign scoped IDs during OD-13 parity inventory instead of citing legacy S-IDs. Photo gallery exterior/interior is separate from mandatory operation evidence, as `vehicle-photos.test.cjs` demonstrates. Missing loading/error/conflict/permission/cancel/pending/projection-lag states must be documented in the current four-app contract before FE parity tasks are ready; existing demo globals are not the production data API.

## 8. Scoped decisions / risks for synthesis

| Decision | Recommendation and trade-off | Exact blocked scope |
| --- | --- | --- |
| Inventory ownership | One service for canonical unit, stock and reservation; local multi-aggregate transactions/guards. More concentrated contention, but closes retail/wholesale race without distributed locks | Requires architecture approval; all tasks remain proposed |
| Reservation partial behavior | Atomic sets only; partial identification via several explicit atomic calls. Simpler visible result and compensation; larger imports need bounded batch size selected by synthesis | No mixed per-item partial success API unless separately specified |
| Reservation expiry | No automatic expiry first; durable reconciliation for abandoned operations. Avoids expiring a real sale while consumer lags; operations require monitoring | Expiration/renewal feature only |
| OD-02 | Keep separate query models and generic placement filter; user chooses Vehicles default scope/navigation semantics | Vehicles registry FE parity/default query binding only |
| OD-04 | No partner warehouse visibility, speculative resale, universal full-payment transfer or cancel-unpaid rule. Need handover/cancel/addendum/resale policy matrix | Those operation handlers/UI; safe partnership, offer, quote snapshots, reserve primitives unaffected |
| OD-06 | Explicit route/evidence version; damage blocker independent. No generic next-state dropdown or gallery photo substitute | Route milestone/receipt/handover production gates and evidence UI; pure capacity/VIN primitives unaffected |
| OD-13 | Build a delta inventory of missing RFQ/factory/VIN/logistics states in unified Realization; do not restore retired apps | Corresponding FE parity approval; pure backend RFQ/quote/order entities remain plan-able |
| Branch+warehouse one operation | Identity creates pending branch intent; inventory atomically attaches warehouse; identity exposes success when saga completes. If UX requires cross-service ACID, reconsider branch ownership rather than claiming distributed atomicity | Synthesis must settle identity boundary and operation visibility; no visible half-created “success” |
| Ownership/receipt facts | Evidence-backed fact commands separate from commercial state; no automatic title transition | OD-04 / financing ownership and route evidence where applicable |
| B2B money policy | Same-currency exact amounts and audit, no money movement; approve precision/allocation/overpayment before monetary production handler | Monetary policy-sensitive commands, not document metadata or evidence state machine skeleton |

## 9. Testability and genuinely small proposed tasks

Every candidate is a preview only, proposed and estimated 2–4 engineering hours after shared event store/auth/testing harness exists. The formal PO/PM pipeline starts only after the architecture checkpoint; this table is not the formal backlog or task board. One command, adapter, projection, or focused test fixture per task; not “implement inventory/commerce”. Dependencies here use local candidate labels; PM should assign final IDs. Architecture/release approval and `/dev` independent Git preflight remain global execution gates. Each task must include its own focused acceptance test, not defer correctness to a later integration task. “Safe” means not additionally policy-blocked, not authorized to start.

| ID | One deliverable + acceptance | Hours | Dependency / extra gate |
| --- | --- | --- | --- |
| IC-01 | VIN parser/value object; normalization and invalid-character table tests, no regional check-digit claim | 2 | Approved syntax decision |
| IC-02 | VIN registry+placement guard migration; unique VIN/one placement DB tests | 3 | Shared migration/event store |
| IC-03 | Warehouse capacity domain command; shrink-below-occupied rejected | 2 | Shared aggregate harness |
| IC-04 | Warehouse guard SQL adapter; two simultaneous last-space claims yield one winner | 3 | IC-03, DB harness |
| IC-05 | Receipt-batch quantity domain aggregate; unidentified arithmetic and negative corrections tested | 3 | Shared aggregate harness |
| IC-06 | Accept-batch local transaction adapter; capacity conflict rolls back batch/events | 4 | IC-04,05; evidence policy before exposed receipt operation |
| IC-07 | Identify-one batch command; one VIN consumes unidentified slot with zero occupied delta | 3 | IC-01,02,05 |
| IC-08 | Atomic identify-list adapter; duplicate/wrong-model item rolls back whole request | 3 | IC-07, DB harness |
| IC-09 | Same-company identified-unit move adapter; sorted locks/no capacity leak in opposing moves | 4 | IC-02,04; evidence policy before exposed move operation |
| IC-10 | ReservationSet domain acquire/cancel/finalize guards; terminal transitions/table tests | 3 | Shared aggregate harness; finalize exposure policy-gated |
| IC-11 | Atomic reservation-slot SQL acquire adapter; wholesale versus retail race gives one set | 4 | IC-02,10 |
| IC-12 | Operation tombstone/cancel adapter; cancel-before-acquire and stale holder cannot release new set | 3 | IC-11 |
| IC-13 | Acquire internal HTTP adapter; authenticate service/context, stable idempotent result | 3 | IC-11, shared auth/idempotency |
| IC-14 | Cancel/status internal HTTP adapters; exact holder enforcement and cancelled tombstone response | 3 | IC-12, shared auth |
| IC-15 | Warehouse summary projector; duplicate receipt event no double occupancy and version gap handled | 3 | Shared inbox/checkpoint, IC-04 |
| IC-16 | Warehouse inventory query adapter; unidentified lines separate from VIN units, tenant isolation test | 3 | IC-15, auth scope |
| IC-17 | Vehicle registry projector; move changes placement without adding a duplicate unit | 3 | Shared projector, IC-02 |
| IC-18 | Partnership request aggregate/adapter; sorted-pair uniqueness and self-partner rejection | 3 | Shared event store/auth |
| IC-19 | Partnership acceptance command; requester's self-accept denied | 2 | IC-18 |
| IC-20 | Partnership ending command; new-action gate closes, existing order references retained | 3 | IC-19 |
| IC-21 | Offer version domain serializer; published warranty/service/terms immutable | 3 | Shared aggregate, money DTO decision |
| IC-22 | Offer publish command; active audience checked and zero reservation writes | 3 | IC-19,21 |
| IC-23 | RFQ draft/send domain command; active partner/counterparty guards | 3 | IC-19 |
| IC-24 | Quotation-version append command; monotonically numbered immutable snapshot | 3 | IC-23, money DTO decision |
| IC-25 | Accept-quotation local transaction; exact hash and duplicate acceptance creates one order | 4 | IC-24 |
| IC-26 | Direct-order request command; published snapshot captured in awaiting-supplier state | 3 | IC-22 |
| IC-27 | Supplier confirmation command; requester cannot impersonate supplier, no early fulfillment | 2 | IC-26 |
| IC-28 | Order allocation request command; quantity/model checks and pending durable intent | 3 | IC-25 or27, shared process/outbox ADR |
| IC-29 | Wholesale acquire dispatcher adapter; retries identical operation after timeout | 3 | IC-13,28, durable relay |
| IC-30 | Wholesale grant-result consumer; duplicate grant no-op and late grant routes compensation | 3 | IC-29, inbox |
| IC-31 | Wholesale compensation worker; wait for exact cancel/tombstone result before terminal failure | 3 | IC-14,30 |
| IC-32 | Crash/timeout saga integration fixture; fail after remote commit then recover without duplicate reserve | 4 | IC-31, real DB/broker harness |
| IC-33 | Order addendum proposal aggregate; original order snapshot untouched | 3 | IC-25; acceptance stock changes separately gated OD-04 |
| IC-34 | Shipment structural aggregate; rejects unknown/unassigned VINs and preserves order link | 3 | IC-28; route transitions not included |
| IC-35 | B2B evidence submission state command; buyer ownership and versioned document references | 3 | Shared docs port; amount DTO approved |
| IC-36 | B2B evidence review state command; seller financial permission and rejection reason | 3 | IC-35; allocation/overpayment separate policy |
| IC-37 | Order detail projection; independent accepted-terms/allocation/shipment source versions | 3 | Shared projector, IC-25,30,34 |
| IC-38 | Inventory availability event→retail adapter contract test; same vehicleId, stale events do not reopen listing | 2 | Retail slice agreement, inventory event contracts |
| IC-39 | Unified-app RFQ microstate/parity inventory doc (one journey) | 3 | OD-13 UX confirmation afterward; no code |
| IC-40 | Unified-app factory identification microstate/parity inventory doc (one operation) | 3 | OD-13; evidence operations OD-06 |
| IC-41 | Unified-app logistics/receipt microstate inventory doc (one route selected by user) | 3 | OD-06/13 scope choice |
| IC-42 | Vehicles registry query binding + states (one approved view) | 4 | OD-02 decision + FE/shared query harness |

Intentionally not bundled into these candidates: universal payment handover policy, cancellation after partial payment, ownership transfer, route/evidence matrix, reserve expiry, bulk partial-result semantics, photo upload UI, all RFQ screens, all logistics, factory lifecycle or full money backend. Each needs its own scoped decision and subsequent 2–4h unit tasks. No production task is `ready` merely because a row is labeled safe.

Cross-slice integration acceptance: same VIN requested concurrently by retail+commerce → exactly one held set; unidentified stock never appears as reservable; identify preserves occupied; opposite warehouse transfers cannot overfill; late reserve after cancel is rejected; lost grant response reconciles to same set; finalization prevents cancel/free/new-reserve races; projection replay causes no business side effects; partnership ending hides new offer/stock access without deleting old contract records; document scan success alone never approves payment/customs/receipt; policy-unconfigured transitions return explicit unavailable action rather than selecting demo rules.
