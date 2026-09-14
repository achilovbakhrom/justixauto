# Gaze Executor CC — architecture pattern audit

Date: 2026-09-14. User direction: keep the microservice approach and also inspect
`gaze-executor-cc` for patterns. This supplements [the original Gaze audit](gaze-reference.md)
and informs the already approved [JustixAuto architecture](../architecture.md).

Reference: `/Users/bakhromachilov/gaze-executor-cc`, HEAD
`f81b62ce346320ec229817dd73cf9ec42032e1d9`.
The 24 inspected source/document files were compared with HEAD and are unchanged.
The working tree has 11 tracked deletions: two Compose files, eight planning
documents and `executor-bin`. Deleted artifacts were not treated as runnable
infrastructure or restored. No reference files were changed; no environment
files, private keys or credentials were read. No install, build, migration,
external integration or runtime test was performed.

Source anchors below are relative to this reference root and pinned revision.

## 1. What this adds to the existing reference

| Concern | Original `gaze-executor` audit | Observed `gaze-executor-cc` source |
|---|---|---|
| Service topology | Several service directories and separate projection entrypoint | One executor composition root wires HTTP, gRPC, projections and command subscribers in one process; `executor/cmd/executor/main.go:44` |
| Module and layering | One module; legacy and v2 implementations coexist | One module `github.com/fewcatltd/gaze-executor`, Go directive `1.23.0`; visible executor uses `internal/domain`, `app`, `port`, `adapter` and service `pkg` |
| Event publication | Intended publication after SQL commit, without durable dispatch | `AggregateStore.Save` publishes inside the SQL transaction callback before commit; `pkg/eventsourcing/aggregate_store.go:120` |
| Snapshots | Persistence methods are stubs in the inspected original revision | Active SELECT/upsert methods; `pkg/eventsourcing/snapshot_store.go:46,74`; frequency is five versions in `snapshot.go:7` |
| Publisher confirms | Default publisher does not enable confirms | Constructor invokes `Channel().Confirm(false)` but ignores its error; publication path does not consume per-message confirmation results; `pkg/eventbus/mqpublisher.go:16` |
| Process coordination | Commands/use cases and event handlers | Explicit event-to-application-command chain through subscriber adapters and `ApplicationService`; no durable process ledger was found |

These are two distinct reference revisions, not proof that one supersedes the
other. In particular, the original audit's snapshot-stub conclusion must not be
applied to CC. Active snapshot code still does not prove correct snapshot replay
or recovery under rollback.

## 2. Patterns to use within each JustixAuto microservice

1. **Explicit composition roots.** Construct ports, repositories, event stores,
   publishers and inbound adapters in `cmd`; preserve separate deployability of
   JustixAuto owners. CC's single executor process is an example of wiring one
   owner, not a template to combine all seven owners.
2. **Commands and replayed aggregates.** Application commands load an aggregate,
   invoke a domain method and save events. Typed events are deserialized and
   applied by `When`; domain transitions are explicit. Evidence:
   `executor/internal/app/command/position_accept_market_order.go:42`,
   `executor/internal/domain/position/commands.go:32`, and
   `executor/internal/domain/position/aggregate.go:94`.
3. **Separate read projections and process subscribers.** Projection handlers
   update query models; command subscribers ask the application layer to perform
   the next operation. Evidence: `executor/internal/adapter/eventbus/projection/on_position_event.go:12`,
   `executor/internal/adapter/eventbus/executor/on_position_market_order_requested.go:12`
   and the adjacent `subscribe.go`. JustixAuto keeps projection rebuild free of
   business side effects; subscriber effects use approved inbox/process mechanics.
4. **Small interfaces and constructor injection.** Reuse the port concept and
   explicit registration. JustixAuto's application layer consumes ports, with
   adapters constructed only at the outside boundary.
5. **Shared technical packages.** Event envelope/replay, database adapters,
   logging/tracing and shutdown organization are useful precedents. Shared
   packages cannot own business state or import another service's domain.

## 3. Limits that must not become JustixAuto guarantees

### Transaction and broker consistency

`aggregate_store.go:126` opens a database transaction and `:162` publishes from
inside its callback. Inference from this control flow: a consumer can receive an
event before the corresponding database commit; a later publish/commit failure
can leave already published events for a rolled-back transaction. A SQL
transaction does not roll RabbitMQ back. Keep ADR-03's transactional outbox.

CC enables confirmation mode but does not demonstrate processing confirmation
outcomes. Its `pkg/brokerx/amqp.go:153` publish struct omits persistent delivery
mode, and the default call does not request mandatory routing. Search of `pkg`
and `executor` Go/SQL found no `outbox`, `inbox`, `NotifyPublish`, `DeliveryMode`
or `SKIP LOCKED` implementation. Publisher confirms and consumer acknowledgements
cover different legs of delivery; neither establishes the SQL transaction's
outcome. See [RabbitMQ's confirmation documentation](https://www.rabbitmq.com/docs/confirms).

`pkg/eventbus/mqsubscriber.go:349` ACKs deserialization failures; `:393` ACKs even
after sending to the failed queue returns an error. JustixAuto must durably
quarantine before ACK and commit deduplication with the handler effect.

### Replay and command boundaries

`pkg/eventsourcing/aggregate.go:160` rejects non-increasing versions but permits
gaps; JustixAuto requires contiguous aggregate history. The migration at
`executor/db/migrations/20250727151147_create_eventsourcing_tables.sql:16`
has unique `(aggregate_id,version)` but no unique `event_id` or append-only role
enforcement. Retain the stronger approved event schema and expected-revision contract.

The accept command treats any load error as permission to construct a new
aggregate (`position_accept_market_order.go:55`) and directly appends an order
before emitting an event; `PositionAggregate.When` appends it again. This is a
source-level replay-equivalence risk, not a tested runtime result. JustixAuto
must distinguish missing aggregates from storage failures and make replayed
events the authoritative domain-state transition path.

CC's comments describe commands as single-aggregate mutations. This does not
override approved JustixAuto owner-local atomic operations, such as provisioning
a provider company/user/membership together. Multi-owner workflows still use
the approved durable protocol, never cross-service SQL transactions.

### Dependency and projection boundaries

`executor/internal/app/application_service.go:6` imports adapter packages;
`position_accept_market_order.go:59` constructs a query adapter through
`AggregateStore.GetDB`. The query joins user, signal, pool, token and wallet
tables (`executor/internal/adapter/query/position_creation_query.go:18`). These
are not precedents for accessing another JustixAuto service's database. Retain
the port-only application rule and T-003 import-boundary checks.

`executor/internal/adapter/projection/position.go:109` defers commit and ignores
its error; `:359` looks up the previous projection version without a demonstrated
inbox or gap recovery mechanism. Keep JustixAuto's transaction-result handling,
deduplication, ordered checkpoints and generation-based projection rebuild.

### Completeness and production reuse

The HTTP example returns placeholder IDs and continues to the success return
after a command error (`executor/internal/adapter/http/positions.go:47`).
`ApplicationService` leaves `userPresetRepo` unassigned in its constructor and
has a no-op notification method. These are wiring/completeness cautions, not
reusable response or completion semantics. No `*_test.go` files were found.
No runtime correctness or independent microservice deployment was verified.

Do not inherit trading entities, wallet handling, external trading/Telegram
gateways, credentials, package patch pins or infrastructure versions. Exact
supported versions remain T-001; an observed Go directive is not a support claim.

## 4. Effect on the approved plan

No new service, transport or dependency family is required by this inspection.
The seven owners remain identity, inventory, commerce, retail, financing,
insurance and documents, with a database-free edge API and four React apps.
One Git repository and Go module do not combine their deployment or data ownership.

| Existing decision/tasks | Reference lesson and required implementation evidence |
|---|---|
| ADR-01, T-003/T-005 and owner scaffolds | Independent owner runtimes and database credentials; import-boundary and cross-owner access checks |
| ADR-02, T-010 | Keep full replay initially; CC's working SQL snapshot methods justify correcting reference attribution, not enabling snapshots without an approved measured need |
| ADR-03, T-009…T-016/T-021 | Test SQL rollback after a publish attempt, crash after commit, lost confirms, duplicate delivery, sequence gaps, commit failure and durable quarantine |
| ADR-05, T-018 | Retain persistent operation/decision records; event-to-command adapters alone do not establish crash recovery or duplicate-safe effects |
| ADR-06, T-017 | Return actual authoritative command receipts and errors; no placeholder IDs or success fall-through |
| ADR-12/13, T-001 | Treat CC libraries as reference evidence; retain approved package families and independently verify supported pins |

This review clarifies reference usage under existing ADRs. It does not change
approved service ownership, operational guarantees, feature scope or task status.
Any later proposal to change those contracts needs its own concrete review.
