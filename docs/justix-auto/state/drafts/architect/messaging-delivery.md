# Messaging delivery amendment proposal

Status: **PROPOSED — coordinator approval and independent exact-commit QA required.**
Date: 2026-09-15. Base: `6524338`.
Scope: close the two composition gaps in [messaging readiness](../../../dev/messaging-readiness.md).
No implementation, migration execution, topology mutation or production activation
is performed by this draft. This proposes an explicit ADR-03/ADR-05 and T-746
amendment; it does not silently reinterpret their current ACK contract.

## 1. Authority and observed limits

Confirmed [architecture §§2,5,9](../../../architecture.md): seven private owners;
one local transaction for domain events, guards, outbox and command receipt;
immutable event identity/bytes; independent contiguous integration sequence;
every authorized stream subscriber receives every position; late subscribers
need an owner-issued checkpoint and permitted snapshot; no personal snapshot
broadcast; inbox, effect and checkpoint are atomic; projections never issue
business commands; process subscribers use admitted durable operations.

Observed integrated evidence:

- [T-006](../../../dev/results/T-006.md), `infra/local/rabbitmq/definitions.json`:
  `justix.integration.v1`, targeted `<source>.<target>.<event>` routes and one
  durable quorum inbox per owner. Resource grants prevent foreign queue reads
  and configuration. Current topic grants allow a source to address any of the
  seven owners; they are a technical ceiling, not an approved recipient matrix.
- [T-008](../../../dev/results/T-008.md), `pkg/eventstore/schema.sql`:
  `outbox.event_id` is the primary key, with one immutable route and one mutable
  sent state. `inbox` deduplicates `(consumer_name,event_id)`. These are sound
  single-target/single-consumer primitives, not a multi-recipient dispatch plan.
- [T-009](../../../dev/results/T-009.md): typed owner transaction ports, separate
  aggregate/integration positions, rollback and unknown commit handling. It
  does not select recipients or authorize a company/custody transition.
- [T-746](../contracts/runtime-registration.md): inert claims across process
  and projection catalogs, distinct logical consumer names, separate role
  binding, and explicit prohibition on inferring queue-sharing guarantees.

Original [Gaze](../../../reference/gaze-reference.md) has post-commit in-memory
publication gaps; [CC](../../../reference/gaze-executor-cc-reference.md) publishes
inside a SQL transaction without waiting for individual confirms. Neither audit
establishes outbox/inbox durability. Their explicit composition and typed
subscriber-to-application pattern remain useful. HTML fixtures demonstrate UI
interaction only; no prototype messaging behavior is evidence here.

RabbitMQ publisher confirms and consumer ACKs cover separate legs. A mandatory
unroutable publication can be returned and still positively confirmed; return
must therefore prevent marking it sent. These facts are supported by the
[RabbitMQ 4.3 confirmation guide](https://www.rabbitmq.com/docs/confirms) and
T-006's pinned live evidence. The implementation must also test its actual
client correlation and connection-loss behavior, rather than infer it from
confirmation mode alone.

## 2. Alternatives and recommendation

| Option | Consequences | Decision |
|---|---|---|
| One broker fanout/topic publication with subscriber queues | Broker distributes copies, but one confirm does not prove the intended recipient set was bound; per-event authorization and subscription cutovers still need durable state; feature queues multiply topology and permissions | Not the minimum compatible change |
| Duplicate complete outbox row per recipient using composite key | Correct with per-recipient status, but duplicates bytes and requires enforcing cross-row identity/byte equality | Viable, less explicit invariant |
| Immutable envelope parent plus per-recipient delivery children; owner durable intake plus per-consumer work | Retains owner-targeted routes, proves complete destination plan locally, preserves separate projection/worker roles and independent recovery | **Recommend** |
| Same parent/children, one owner dispatcher invokes all consumers before broker ACK | Fewer tables; valid only if all required roles are bound in that dispatcher or have durable remote invocation. Couples availability, deployment and repeated gap handling | Valid alternative, not selected |

Do not rotate destinations across sequence positions, mint recipient-specific
event IDs, filter types before a required checkpoint, or run independent required
effects as competing consumers of the same AMQP queue. Replicas of the *same*
intake role may compete because each stages the complete required local work set.

The selected change introduces one durable handoff inside the destination owner.
**AMQP ACK means that owner has durably accepted custody, not that every business
consumer has completed.** Logical completion still requires inbox + effect +
checkpoint + work completion in one transaction. This distinction must appear
in the approved ADR text, runtime status and failure tests before activation.
Existing T-013 direct-ACK mode remains valid for an explicitly single-consumer
fixture/contract; it cannot share an active queue with this intake mode.

## 3. Source admission and complete delivery plan

An approved subscription contract grants a destination service access to a
specified source stream and its entire integration sequence over an explicit
admitted interval. It enumerates permitted schema versions and purpose/scope.
Deployment catalogs, queue existence, company IDs, caller-supplied destination
lists and broker regexes confer no such grant. Each concrete owner feature must
reference an approved admission rule; unresolved OD-10 data release remains gated.

Use the existing stream key `(sourceOwner,aggregateType,aggregateId)`. Source
admission records identify `admissionId`, contract version/digest, destination,
stream, an owner-authorized starting checkpoint and optional closed upper
sequence. Scope/purpose and the authority evidence reference are immutable.
An admission range `(startAfter,endInclusive]` is a sequence range, not a time
expiry or new business permission. An open upper bound means an admitted active
subscription. Closing a range creates append-only evidence; it does not rewrite
already committed event or delivery records.

Source commands select the exact distinct authorized destination set from this
owner-local state **inside the same transaction as append**, under the same
stream fence used for admission changes. Lock ordering must compose with T-009's
sorted stream locks. Recipient choice is never an arbitrary callback run later
by the relay. A single source event is serialized once using its T-007 approved
schema; all destination deliveries reference those identical bytes/hash and
event ID. The plan freezes its contract/admission references and destination
set. A destination appearing under two overlapping admissions is a contract
error, not two copies or a silent union of permissions.

Every integration event records a plan, including an explicitly validated empty
set where the owner contract permits no active recipient. Missing admission
configuration is not an empty set. All required children must be inserted with
the parent and local command effects, or the entire transaction fails. An
application cannot omit a required destination and retain a successful receipt.
Internal-only domain events have no integration envelope or delivery plan.

Any subscribed destination must be permitted to receive every envelope position
in the admitted interval. A schema addition that would expose unapproved fields
blocks that stream's integration emission until its contract is reconciled;
silently dropping that type would create a gap. An irrelevant *known, permitted*
type is checkpointed as a no-op. Different recipient payloads or redacted
sequence markers require a separately approved event/stream contract; they
cannot be introduced by this transport amendment.

New recipient: the source fences the stream at high-water `h`, records approved
admission beginning after `h`, and supplies a purpose-authorized snapshot and
checkpoint for `h` through its authenticated owner port. The receiver persists
the bootstrap evidence and installs it atomically with its checkpoint before
applying `h+1`; intervening live positions may already be durably staged. Snapshot
authorization is separate from event transport authorization. Zero is accepted
only when the source explicitly establishes a new empty stream, never inferred.
If the subscriber needs historical *process* effects, a read snapshot is not
sufficient: its feature must approve an explicit historical catch-up protocol.

Removal is also fenced at a recorded high-water. An approved drain closes future
admission while retaining all obligations through that boundary. Missing a
deployment, disabling a handler or changing a config file never cancels jobs.
If a policy requires immediate access revocation instead of draining, affected
undelivered work is placed on an explicit hold; no further publish, fake sent
mark or deletion is allowed. Scope of in-flight/already accepted bytes and a
permanent disposition requires that feature's security/business decision; this
draft does not invent recall of delivered bytes. Re-admission needs fresh
evidence and a checkpoint; it cannot revive old private history implicitly.

## 4. Typed boundaries

These are proposed signatures/semantics, not existing API names. All value
constructors validate and defensively copy. `Owner`, `ID`, `Revision` and
`IntegrationEnvelope` use existing approved value/envelope constraints; the
envelope is schema-bound, not an unvalidated `map[string]any`.

```go
type Stream struct { Source Owner; AggregateType string; AggregateID ID }
type ContractRef struct { ID string; Version uint32; Digest [32]byte }
type Recipient struct { Owner Owner; AdmissionID ID; Contract ContractRef }
type DeliveryPlan struct { /* sealed stream, sequence, sorted recipients */ }
type ImmutableMessage struct { /* sealed event ID, stream, bytes, SHA-256 */ }
type DeliveryKey struct { EventID ID; Destination Owner }
type ConsumerKey struct { Name string; EventID ID }
type Lease struct { Token ID; Until time.Time }

// Owner port. Adapter binds admitted source state and all writes to T-009 tx.
type IntegrationWriter interface {
    AppendIntegration(context.Context, IntegrationEnvelope) error
}
// Adapter implementation resolves/seals DeliveryPlan inside that same tx;
// callers cannot submit raw destinations, SQL handles or an arbitrary plan.

// Transport adapter only: has no business handler or AMQP ACK capability.
type Intake interface {
    Accept(context.Context, RoutedMessage) (CustodyReceipt, error)
}
// RoutedMessage owns copied raw bytes plus exchange/routing provenance.
// CustodyReceipt is returned only after known committed durable acceptance.

type Consumer[T any, U any] interface {
    Handle(context.Context, U, TypedEvent[T]) error
}
// T is the declared payload, U the concrete owner transaction port set.
// Schema-key dispatch is assembled from sealed typed handlers at the adapter.
// No domain/application registry lookup, any-payload callback or broker ACK.
```

`IntegrationWriter` is an owner application port, implemented outside the domain;
the owner adapter binds admission reader, serializer and storage to the same
transaction. The generic dispatch adapter selects a registered typed decoder
before invoking the owner consumer. No shared package imports owner application
code. Process consumers stage admitted local operations, never call external
services inside the intake/effect transaction. T-018 performs subsequent durable
steps through approved ports. Projection handlers remain historical and read-only
with respect to business command state.

## 5. Durable records and privilege contract

All following records are owner-local technical tables in `eventstore`, not
shared business tables or cross-owner foreign keys. Exact migrations need the
separate assignments in §9; these definitions specify required keys/invariants.

| Proposed table | Immutable identity/content | Mutable scheduling/completion |
|---|---|---|
| `outbox_messages` | event ID PK; stream/sequence unique; original bytes with checked SHA-256; frozen plan including admission/contract refs and exact recipient set; creation time | None |
| `outbox_deliveries` | `(event_id,destination)` PK and local parent FK; exact exchange/route; admission ref matching frozen plan | attempts, next attempt, lease token/until, confirmed sent time |
| `dispatch_messages` | event ID PK; source/stream/sequence unique; copied bytes/hash, admitted source route, initial membership version and bootstrap/admission refs | None |
| `dispatch_enrollments` | `(event_id,enrollment_id)` PK and message FK; immutable admitted membership/cutover reference and exact consumer set; exactly one initial enrollment per message | None |
| `dispatch_jobs` | `(consumer_name,event_id)` PK and local enrollment FK; consumer kind/contract/generation admission ref | attempts, next attempt, lease token/until, completed time, explicit hold/quarantine reference |
| `messaging_admissions` | append-only source/recipient or local-consumer admission records: stable ID, stream, contract digest, authority/bootstrap evidence, start checkpoint and admission action | None; closures/holds/resumptions are additional evidenced records |

Separate namespaces distinguish source-recipient admissions from local-consumer
admissions. Their effective ranges are resolved under stream fencing; malformed,
overlapping or conflicting evidence fails closed. Scope selectors must resolve
to concrete admitted streams before work is staged. No arbitrary SQL predicate
or wildcard in a handler descriptor constitutes admission.

Enforce parent/child completeness at transaction commit (a deferred constraint
trigger compares the immutable canonical recipient set, or each enrollment's
consumer set, with its child keys, including exact admission references). Each
message requires its initial enrollment in that same transaction. Missing/extra children, an
orphan, duplicate owner/consumer, or changed route/hash is rejected. Uniqueness
must not collapse different bytes under the same event ID; retry is accepted
only after comparing all immutable identity/content. A conflicting incoming
message is preserved as quarantine evidence without overwriting the first copy.
Hash comparison is an integrity check, not proof that a publisher is authorized.

Runtime roles receive SELECT/INSERT only on immutable content, and enumerated
UPDATE columns on work rows. Migration owners alone own tables/triggers; audit
reachable/default grants as T-008 does. No runtime DELETE/TRUNCATE/DDL or content
UPDATE. Dispatch job completion is monotonic; no completion without matching
consumer inbox/effect/checkpoint transaction. Completion includes a nullable
composite reference to `inbox(consumer_name,event_id)`, constrained to the job's
own identity and present if and only if completion time is present. Its FK
prevents completion without durable inbox evidence; the typed transaction adapter
must additionally enforce effect/checkpoint atomicity. A freely callable setter
for `completed_at` without those checks is not sufficient. Unit/live tests must
distinguish SQL-enforced constraints from application-enforced invariants.

For a local source-to-itself route the same mechanics apply. The envelope still
has one identity; no extra domain event or global database is introduced.

## 6. Owner intake, logical consumers and sequence

The owner worker root runs AMQP intake; projection and process roles run their
own SQL dispatch executors. Intake replicas alone compete on the owner's inbox
queue. Projection roots do not `basic.consume` that same queue. Process consumers
in a worker read their named jobs rather than competing directly with intake.
Both role catalogs are inspected inertly under T-746 to construct the complete
required membership, including consumers whose executable is temporarily down.

Deployment profiles carry an approved immutable membership version; the database
records its admitted stream ranges. Startup compares installed claims, selected
role and the admitted version before consuming. Adding a binary or factory does
not amend admission. A deployment missing a required claim fails readiness; an
intake with all claims but a temporarily unavailable executor can safely stage
that executor's jobs. Compatible replicas use the same membership version;
conflicting versions cannot race to determine the child set.

Intake validates source/target routing, envelope metadata, schema allowlist and
receiver admission. Under the receiver stream/membership fence it persists one
immutable message and **all** required consumer jobs atomically. Duplicate
re-delivery compares bytes/hash and reuses that original frozen set, even after
a catalog upgrade. It does not recompute old membership from today's registry.
Only a known committed custody receipt permits AMQP ACK on the delivery channel.
A commit error is unknown: keep unacknowledged/close channel and recover by an
authoritative read or duplicate intake. No in-memory staging before ACK.

Each executor claims only its approved consumer identity. In a local transaction
it fences the job lease, calls T-013 dedup plus its typed effect and T-014
checkpoint, and marks the job completed. Concurrent duplicate jobs commit at
most one effect. A successful no-op advances exactly the next permitted
integration position. Every `(consumer,source,stream)` checkpoint is independent;
consumer A's progress cannot complete B's job or advance B's position.

Out-of-order jobs remain durable and incomplete. T-014 reconciles a missing
range through the authenticated source owner port using the applicable admission
and original envelope identity; no latest-state lookup or arbitrary checkpoint
jump. Existing custody is sufficient for broker ACK even while a logical gap
waits, because the job remains recoverable. Recovery may restore only authorized
positions; denied or missing recovery evidence raises an actionable hold.

Local consumer addition follows its own approved checkpoint/bootstrap contract.
Fence membership at `h`; preserve every old consumer's obligations unchanged,
install the new consumer's authorized bootstrap for `h`, and stage all required
positions above `h`, including messages already accepted while bootstrap runs.
Cutover must select `h` consistently with the source's issued checkpoint and
receiver accepted backlog; a receiver's newest observed message alone is not a
contiguous high-water. Admission installation scans retained, authorized custody
above `h` in the same fenced transition and appends a new enrollment containing
only the newly admitted consumer jobs. It never edits a message's initial
membership/enrollment or adds unaccounted children to it. The enrollment's exact
set and jobs commit together; existing consumer jobs are not enrolled twice.
Subsequent intake uses the new membership version for its initial enrollment.
This does not give access to positions at/below `h`.
If required bytes are absent, the admitted owner recovery port must supply them
before activation is complete. No presumption that retained SQL bytes authorize
reuse. Process subscriptions without an approved bootstrap/catch-up stay disabled.

Removal closes a consumer range at an approved boundary and drains its existing
jobs; no handler absence, rename or generation switch deletes an obligation.
Rebuild generations are separate admitted projection consumers under T-016;
historical read rebuilding never replays a process subscriber. Immediate local
revocation uses an evidenced hold and separately approved disposition, as in §3.
An old consumer name may not be reused with changed meaning.

Malformed/unsupported intake stores durable quarantine evidence before original
ACK; failed quarantine leaves it unacknowledged. A valid custody message whose
particular handler later fails preserves its job and per-consumer quarantine
reference; it cannot mark that consumer completed or skip its checkpoint. Other
consumers can progress independently within their own sequence constraints.
Repair/redrive goes through the same admission, inbox and checkpoint checks and
retains original evidence. An invalid message without a usable event ID gets a
separate evidence ID; never fabricate an integration identity for it.

## 7. Relay, leases and broker access

Relay claims individual `(eventId,destination)` rows. A short claim transaction
uses locked due rows with bounded leases and unique fencing tokens, then commits
before publishing. Each AMQP attempt uses the parent's original bytes and event
ID with persistent mode, `mandatory=true` and confirms. Route is the immutable
approved `<source>.<target>.<event>` route. Retry neither reserializes payload nor
re-resolves the recipient plan. One destination's failure never marks another
unsent destination delivered, nor undoes a destination already confirmed.

Maintain correlation between publication sequence, return and confirm; a return,
nack, timeout or lost channel is not success. Only a routed positive confirm may
conditionally set that child `sent_at` for the current unexpired lease token.
Late stale completion affects zero rows; the new owner retries the same bytes.
Parent delivery is derived as all child rows confirmed, not an independently
mutable global sent flag. An empty validated plan has no broker delivery claim.

Dispatch uses the same claim/fence pattern. The effect transaction locks the job,
verifies token and lease, and requires a final valid lease-conditioned completion
write; losing the fence rolls the entire effect/inbox/checkpoint back. No external
side effect is protected by a SQL lease. Unknown commits are resolved from durable
inbox/job state, never blind callback repetition. Retry/backoff affects scheduling
only; expiry cannot cancel delivery, advance checkpoints, release reservations or
choose a business terminal outcome. Per-stream gaps remain possible despite
publisher ordering and are handled by §6.

Retain `/justixauto`, current durable quorum owner inbox/quarantine queues and
the two current topic exchanges. Provisioning remains a separate migration
authority; services have no configure rights, administrator tags, foreign queue
reads or default-exchange publish grants. Restrict integration topic write regexes
to the explicit approved source/target/schema routes compiled from activated
contracts (anchored, regex-escaped, no `.*` granting unknown owners/types).
No approved route means a deny-all grant, not the current seven-target ceiling.
Existing owner queue bindings can remain target-wide because publisher grants
and application admission both constrain traffic; an infrastructure audit must
detect unexpected bindings and cross-owner copies before activation.

Broker ACLs cannot express company/party/stream-object authorization. Source
admission checks and receiver validation are mandatory. Split credentials by
runtime capability: `relay_<owner>` writes only its approved integration routes
and cannot read queues; `intake_<owner>` reads only its own inbox and writes only
its approved quarantine route if that adapter uses the broker. Projectors need
no broker credential for SQL dispatch; redrive gets only its reviewed owner-local
capability. The old broad `svc_<owner>` credential is not used for new activated
compositions. Credentials remain injected, never stored in definitions.
[RabbitMQ access-control documentation](https://www.rabbitmq.com/docs/access-control)
distinguishes resource permissions from topic routing restrictions; neither is
a replacement for object authorization in this proposal.

## 8. Migration and retained evidence

Use an explicit forward migration, not editing/re-running the T-008 installer
against an existing database. Preserve `eventstore.outbox` as the legacy table;
create the new parent/child and dispatch tables additively. Before cutover record
schema/version hashes, row counts, per-event bytes/hash/route/status and recoverable
backup under owner migration authority. This is migration evidence, not a new
retention policy or permission to export private bytes to logs.

Stop/fence old append/relay/intake runtimes, drain/close their AMQP channels,
record remaining delivery/confirm ambiguity and allow no mixed writer mode.
Runtime schema compatibility rejects an old binary after the migration switches
the active mode. In a fenced transaction copy each legacy event to one parent
and its observed destination child, preserving bytes, identity and original
evidence. Resolve/validate its recorded route; unrecognized or unauthorized
legacy routes block migration for investigation. Never infer missing recipients
from the latest catalog. Where historical recipient authority is unavailable,
hold that copied obligation for review instead of publishing it.

Preserve confirmed legacy sent status only as that exact child's prior status;
it does not prove any other recipient was served. Unsent or ambiguous children
remain retryable with the same identity after admission validation. Preserve old
attempt/lease/confirm evidence in the immutable migration record; fence/reset
active scheduling leases for the new executor rather than reuse old ownership.
Grant legacy tables read-only runtime access after cutover; no content deletion
or destructive down migration. New multi-recipient events cannot be losslessly
collapsed back to one legacy row; operational rollback is a forward compatible
binary/schema recovery, with new dispatch paused and durable evidence retained.

For a pre-existing direct-ACK consumer, do not attach new intake concurrently.
Freeze publication/intake, record its durable inbox/checkpoint and broker backlog,
then install custody mode and explicit consumer membership. Redelivered events
reuse existing consumer inbox evidence without re-running effects. A legacy ACK
proves only its original consumer completed, never all newly required consumers.
Historical catch-up for new consumers requires their approved admission/bootstrap;
messages already removed from RabbitMQ cannot be recreated by changing a binding.
Resume publishers only after schema, ACL, catalog and bootstrap compatibility
checks pass. Fixtures may establish that no legacy rows/consumers exist, but must
assert that fact instead of claiming migration coverage from an empty install.

## 9. Bounded follow-up ownership and gates

Coordinator assigns IDs/branches after approving this amendment. The MD labels
below are proposal slices, not newly ready task-board IDs. Existing integrated
results remain valid for their stated primitive scope; preserve their exact QA
SHAs and add amendment evidence instead of rewriting historical PASS claims.

| Slice/task | Exact files and required work |
|---|---|
| MD-1 schema/admission migration | New `pkg/eventstore/migrations/000002_messaging_delivery.up.sql`, `pkg/eventstore/messaging_migration_test.go`: additive records, completeness/immutability/privilege checks, legacy audit/copy/fences. No destructive down migration. New `pkg/outbox/admission.go`, `pkg/outbox/admission_test.go`: typed source stream/range admission resolver using the supplied transaction |
| MD-2 source insertion amendment, after MD-1 | T-011-owned `pkg/outbox/insert.go`, `pkg/outbox/insert_test.go`: serialize once, resolve complete plan, atomic parent/children; legacy single-target path explicitly mode-gated. Do not amend T-011 while its worker owns those files |
| T-012 relay, after MD-2 and MD-5 | Existing `pkg/outbox/relay.go`, `relay_test.go`, `amqp.go`, `amqp_test.go`: recipient-keyed claims, correlated returns/confirms, lease fencing and partial-destination recovery |
| MD-3 durable intake, after MD-1/T-013 | New `pkg/inbox/intake.go`, `intake_test.go`, `membership.go`, `membership_test.go`: frozen full membership, atomic custody/jobs, duplicate identity check and safe ACK adapter; source/local admission cutover and backlog enrollment |
| MD-4 logical dispatch, after MD-3/T-014/T-015 | New `pkg/inbox/dispatch.go`, `dispatch_test.go`: role-specific claims, T-013 effect transaction, checkpoint plus job completion, independent failure recovery. T-013 `consume.go`/`consume_test.go` stay direct-ACK compatible; if their port cannot compose, coordinator separately assigns the narrow interface amendment before MD-4 |
| MD-5 least-privilege topology, before activated delivery | Existing `infra/local/rabbitmq/definitions.json`, `tests/integration/broker_topology_test.go`; new `infra/local/rabbitmq/route-contracts.json`: explicit reviewed route metadata, split role principals, no configure/foreign reads; pinned live permission and route tests. Route metadata is not business admission |
| T-014/T-015/T-016 | Existing assigned leaves only: sequence/checkpoint and bootstrap behavior; quarantine/redrive durable work references; generation admission and historical read-only rebuilding. Any additional migration leaf must be separately assigned to MD-1, not silently edited |
| T-021 integration proof | Existing `tests/integration/messaging_crash_test.go`, `recovery_fixture_test.go`: required cross-boundary failure matrix below |
| T-580…T-586 owner wiring; T-022 infra | Existing owner `cmd/worker/main.go`, `cmd/projection/main.go`, `adapter/amqp/registry.go`, `adapter/projection/registry.go`, owner wiring tests; worker intake and role-specific SQL executors. T-022's `infra/compose.yaml`, `tools/migrate.sh`, `tests/integration/readiness_test.go` enforce mode/schema/ACL readiness, no automatic mutation |

MD-1 may be split further to keep one bounded task under the board's effort cap;
no worker owns overlapping leaves concurrently. Source schema/admission work and
topology can proceed independently after approval. Owner feature subscription
leaves (T-747 onward) activate only after the relevant concrete stream admission,
schemas and complete membership are approved; no mass invention of consumers.
Coordinator alone promotes the ADR/architecture/T-746 corrections and updates
dependencies, board and readiness with required backups.

## 10. Mandatory failure evidence before composition is ready

All fixtures use pinned isolated PostgreSQL/RabbitMQ and synthetic data. These
are planned acceptance tests, **not checks executed by this proposal**.

1. One event, two admitted owners, identical bytes/hash/ID at both; recipient A
   confirms while B is unroutable/offline. Only A's child is sent; B recovers.
   Missing/extra/duplicate plan children and changed retry bytes fail atomically.
2. Consecutive stream types relevant to different handlers reach *both* admitted
   owners in full; each irrelevant type advances its own checkpoint. Internal
   domain-only revisions create no integration gaps.
3. Crash before/after source commit, claim, publish, return, confirm and sent
   update; broker restart/lost confirm/stale lease. No rolled-back event is
   published; ambiguous outcomes duplicate identical messages without lost work.
4. Two required consumers (projection/process), separate executables: only intake
   reads AMQP. Crash before/after custody commit and ACK. One role offline,
   restarted or failing cannot lose the other's required job or receive credit
   for its completion; custody ACK is not reported as business completion.
5. Crash before/after effect/inbox/checkpoint/job completion and commit reply;
   competing/stale executors. One committed authorized effect per consumer,
   no completion on rollback, no cross-consumer checkpoint advancement.
6. Gap, duplicate, unknown schema, hash conflict and quarantine outage. Originals
   or custody/jobs remain durable; no arbitrary sequence adoption. Redrive
   preserves first bytes and repair evidence and uses ordinary dedup.
7. Add recipient/consumer at nonzero checkpoint with concurrent append/intake,
   out-of-order backlog and unavailable bootstrap. All retained subscriptions
   remain contiguous; new one starts only at its admitted boundary; no historical
   private data released by catalog registration or restart.
8. Remove/drain, immediate hold, binary rollback and re-admission. Existing
   obligations/evidence remain; absent handlers are not silent success. Process
   bootstrap without explicit catch-up approval and name reuse fail closed.
9. Service role cannot spoof source, publish unknown route, read a foreign queue,
   configure objects, use default exchange or mutate event bytes/recipient set.
   Runtime role cannot bypass checks through inherited/default grants. A valid
   broker route with unauthorized company/stream is rejected at admission.
10. Populated legacy migration: sent/unsent/ambiguous rows and prior inbox entries;
    immutable byte comparison, recognized admissions, duplicate redelivery and
    old-binary rejection. No assertion of missing historical recipients having
    completed. Full repair/rollback evidence remains recoverable.

Expose separate oldest-unsent-delivery, oldest-unfinished-job, gap and quarantine
signals with owner/consumer/event IDs, not payloads/secrets. Alert thresholds and
capacity tuning are operational configuration; this proposal sets no delivery
deadline, purge interval, financial policy or production availability guarantee.

## Approval boundary

Approve or amend the selected storage/handoff model, role-specific broker
permissions, explicit custody ACK meaning, admission cutover and forward migration
before dependent implementation. Concrete source/recipient data permissions,
process bootstrap and immediate-revocation disposition remain feature-specific
gates. No distributed transaction, exactly-once broker claim, cross-owner SQL,
new business owner, snapshot publication, cancellation policy or financial rule
is authorized by this draft.
