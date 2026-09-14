# Shared contract conventions — synthesis input

Proposed, not approved. These decisions must be incorporated into the canonical
architecture or explicitly superseded there; this file is not a competing API.

## Transport and concurrency

- Browser traffic: versioned `/api/v1` JSON over HTTPS, opaque session cookie.
- An existing aggregate mutation supplies `If-Match: "<revision>"`; missing
  precondition returns 428, stale revision returns 412. A cross-resource invariant
  conflict returns 409. Validation returns 422 with safe field errors. Unauthenticated
  returns 401; an object outside the caller's tenant/counterparty scope returns
  404 (no existence oracle), an accessible object's forbidden action returns 403.
- All creates/action commands carry `Idempotency-Key` (UUID), including actions
  with files or process-manager operations. Scope: actor + company + command;
  canonical request hash excludes trace IDs, includes business payload and target.
  Same key/same hash returns the stored result; same key/different hash returns
  409. Current access is checked again even for a replayed response.
- Authoritative company, branch scope and actor come from authenticated context.
  A body companyId is never trusted as authority. Switching context has a revision
  and invalidates the browser query cache; old-context responses must not render.
- Synchronous success returns `{data, revision, operationId}` with the actual
  aggregate receipt, not an eventually consistent query pretending to be fresh.
  Async cross-service command returns HTTP202 `{operationId,status:"pending"}`;
  scoped `GET /api/v1/operations/{id}` gives pending/succeeded/failed and a safe
  result link. Unknown outcome is not a failure and must not encourage new keys.
- Lists use opaque cursor + limit; default50/max100 are technical proposal values,
  not business limits. Stable sort has ID tiebreaker. Scope applies before counts
  and pagination. Projection lag includes `asOf`; write UI retains confirmed
  receipt until projection catches up. Never authorize writes from these views.

Error shape:

```json
{"error":{"code":"REVISION_CONFLICT","message":"Refresh this record.","fields":{},"traceId":"opaque"}}
```

## Event envelope and durability

```json
{
  "eventId":"uuid",
  "eventType":"inventory.vehicle.vin-assigned.v1",
  "schemaVersion":1,
  "aggregateId":"uuid",
  "aggregateType":"vehicle",
  "aggregateVersion":2,
  "companyId":"uuid",
  "occurredAt":"2026-09-13T10:00:00Z",
  "actor":{"kind":"user","id":"uuid"},
  "correlationId":"uuid",
  "causationId":"uuid",
  "data":{}
}
```

- Store authoritative internal events locally; publish a versioned integration
  event only to authorized consumers. Audit/trace contains IDs, not credentials,
  document bodies or gratuitous customer PII. Schema changes are additive or
  explicitly versioned with replay upcasters, never retroactive record editing.
- Unique eventId plus unique aggregateType/aggregateId/aggregateVersion. Guard
  expected revision, state change, idempotency receipt, event append and outbox
  append are one SQL transaction. Counterparty IDs remain explicit in event
  payloads; they are not an implicit authorization grant to every subscriber.
- Relay waits for broker confirms and mandatory-route success before marking
  sent. Durable exchange/queues + persistent messages. Lost confirmation causes
  retransmission of the SAME eventId. No goroutine fire-and-forget guarantee.
- Subscriber inbox and projection/process-manager effect commit in one local
  transaction before ACK. Dedup key consumerName+eventId. Version gaps wait/retry
  or reconcile from owner; do not apply arbitrary future aggregate state.
- Poison event quarantine must itself be durable before removing the original
  delivery; failed quarantine persistence keeps original unacknowledged. Alert,
  inspect redacted metadata, repair/upcast and replay through the same inbox path.
- A public event's aggregateVersion can skip internal-only revisions. Synthesis
  must choose a contiguous integration-stream sequence or explicit checkpoint
  protocol; consumers MUST NOT confuse internal revision gaps with message loss.

## Money and time

- Transport amounts as decimal strings with currency, e.g.
  `{ "amountMinor":"1840000", "currency":"USD" }`. Currency dictionary owns
  exponent; no implicit two decimals, floating-point JSON money, or silent FX.
- Calculation input/output snapshots include `policyId`, `policyVersion`,
  `programVersion` where relevant, rounding policy and schedule dates.
- `YYYY-MM-DD` for local business dates; UTC RFC3339 for instants. Country does
  not infer currency/timezone. Date calculations must explicitly select timezone.
- Financial schedule implementation is policy-gated; mock arithmetic may supply
  fixture examples but cannot become an approved payable schedule accidentally.

## Task decomposition quality gate

Formal tasks are created after architecture/backlog approval. For candidate units:

1. One bounded outcome, ideally2–4h, one owner/branch/worktree and focused files.
2. Explicit contract predecessor; backend can use agreed ports and frontend can
   use contract fixtures before full integration, not invented endpoints.
3. Acceptance includes success + invalid input + forbidden scope + stale/retry
   as applicable; crash/redelivery on persistence tasks; screenshots/modal states
   on UI tasks. A generic "write tests" item is not substitute for task tests.
4. Shared paths have a prerequisite owner; lanes never modify the same file
   concurrently. Runtime-wide migration/lockfile generation is serialized.
5. Each blocked unit names OD and exact missing decision, not whole-project halt.
   Each FE unit points to mock entry/navigation/state and names missing states.
6. User approvals and independent Git readiness are gates, not dev task claims.
