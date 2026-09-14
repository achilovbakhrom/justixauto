# Justix Auto — Gaze architecture-reference audit (proposed handoff)

Date: 2026-09-11. Scope: bounded read-only SLICE audit of `/Users/bakhromachilov/golang/gaze-executor`; only this draft is written. Reference HEAD: `913c018fb2182ec943ef308da63baa85b5628bec`; `git status --short` returned no changes. No `.env`, credentials, private config overrides, or running infrastructure inspected. No build, dependency installation, migrations, or tests executed. This is source inspection, not a runtime certification or completed Justix implementation architecture.

Binding project direction supplied by coordinator: **new** target `/Users/bakhromachilov/startups/justixauto`; **Go microservices + CQRS + event sourcing using Gaze's approach; React frontend**. Preserve these choices. `business-logic.md` remains authoritative for Justix entities; Gaze's trading entities are not a product template. No `dev/dev-state.md` was present when this audit began.

All source paths below are relative to the reference repository unless explicitly identified as Justix. Line numbers are inspection anchors, not promises of a stable future revision.

## 1. What the reference actually uses

| Concern | Evidence-backed reference choice | Source |
| --- | --- | --- |
| Go/module | Single root Go module `github.com/gaze-ai-bot/gaze-executor`, `go 1.23.0`; several service/binary directories, not a separate Go module per microservice | `go.mod:1`; `executor/cmd/`, `price-oracle/cmd/`, `trigger-engine/cmd/`, `notification-ultra/cmd/` |
| PostgreSQL | Executable wiring calls `database.NewPGDatabase`; shared GORM database handle is passed to the event store and read repositories in executor; trigger engine has its own `TriggerDSN`; notification reads `ExecutorDSN` | `pkg/database/database.go:23`; `executor/cmd/projection/main.go:47`; `executor/cmd/executor_v2/main.go:80`; `trigger-engine/cmd/trigger-engine/main.go:48`; `notification-ultra/cmd/notification/main.go:55` |
| SQL access | GORM `v1.30.1`, PostgreSQL driver `v1.6.0`; MySQL driver also declared and generic connector supports MySQL, but inspected service wiring uses PostgreSQL. An indirect MongoDB dependency is not evidence of a MongoDB persistence architecture | `go.mod`; `pkg/database/database.go` |
| Broker | RabbitMQ/AMQP via `github.com/streadway/amqp v1.1.0`, wrapped in `pkg/brokerx` and `pkg/eventbus`; executor-specific publisher sets its exchange | `go.mod`; `pkg/brokerx/amqp.go`; `executor/pkg/eventbus/publisher.go` |
| Cache/stream-adjacent data | Redis client `github.com/redis/go-redis/v9 v9.7.0`; executor initializes Redis and price-oracle instantiates a Redis price cache. Redis is not the event store | `go.mod`; `executor/cmd/executor_v2/main.go:127`; `price-oracle/cmd/price-oracle/main.go:56` |
| Inbound/outbound interfaces | Echo `v4.13.4` HTTP adapters; gRPC `v1.69.0-dev` and protobuf `v1.36.6`; Gorilla WebSocket `v1.5.3`. These are reference pins, not proposed current Justix pins | `go.mod`; `executor/internal/v2/adapter/http/http_server.go`; `executor/internal/adapter/grpc/grpc_server.go`; `price-oracle/pkg/client/websocket_client.go` |
| Observability/config | Zap `v1.27.0`, OpenTelemetry `v1.29.0` core and Jaeger exporter `v1.10.0`; `pkg/ika` typed environment reader, graceful shutdown helpers | `go.mod`; `executor/cmd/projection/main.go`; `pkg/trace/`; `pkg/ika/`; `pkg/shutdown/` |
| Build/runtime | Service Dockerfiles build with `golang:1.23-alpine3.20`, run on `alpine:3.19`. Executor Dockerfile targets `executor/cmd/executor_v2`. Older generic build path uses `golang:1.23.0` → `alpine:3.12` and Makefiles | `executor/Dockerfile`; `price-oracle/Dockerfile`; `notification-ultra/Dockerfile`; `tools/docker/Dockerfile`; `Makefile` |
| Development infrastructure | Compose declares `postgres:15-bullseye`, `rabbitmq:3.13.6-management`, `jaegertracing/all-in-one:latest`. It does not prove the deployed production topology or provision every dependency used by all service entrypoints | `docker-compose.yml:49`, `:66`, `:76` |

Reference port defaults: executor HTTP `8080` (`HTTP_SERVER_PORT`), executor gRPC `50051` (`GRPC_SERVER_PORT`), price-oracle HTTP `8080` (`HTTP_PORT`); notification's HTTP struct also specifies `8080` but uses an `envDefault` tag spelling unlike executor's `env-default`, so do not assume its default is effective without checking the config reader. Redis fallback is `6379` in price-oracle config. Compose exposes PostgreSQL container port `5432` without a fixed host-port mapping, RabbitMQ `5672`/management `15672`, and Jaeger `6831/udp`, `16686`, `14268`. These are reference observations, not a collision-free local Justix port allocation. Sources: HTTP/gRPC server files above; `price-oracle/internal/config/config.go:38`; `notification-ultra/internal/adapter/http/server.go:15`; `docker-compose.yml`.

## 2. Reusable architecture shape

The reference concretely implements a ports-and-adapters arrangement:

- `internal/domain/<aggregate>/`: domain commands/events, state and transitions; shared `AggregateBase` supplies versions and pending events.
- `internal/app/command/` and `internal/app/usecases/`: command execution and orchestration; commands create/mutate aggregates; use cases can coordinate persistence of several aggregates.
- `internal/port/`: repository and external integration contracts.
- `internal/adapter/http`, `grpc`, `repository`, `eventbus`, `projection`, gateways: delivery, persistence, integration and read-side adapters.
- `cmd/<binary>/main.go`: explicit dependency construction and runtime startup; shared reusable implementation under root `pkg/`; public service DTO/client/event packages under service `pkg/`.

Evidence: `docs/architecture.md` describes this layout; `executor/cmd/projection/main.go:81` constructs repositories/store/serializers/projections and subscribes them; `executor/internal/v2/app/command/position/create_position.go` prepares and presents an aggregate; `executor/internal/v2/app/usecases/create_position.go:191` opens a transaction and persists it with linked orders. Query HTTP adapters receive repository ports, e.g. `executor/internal/v2/adapter/http/http_server.go:30`.

Do not turn documentation slogans into strict assumptions: `docs/architecture.md` says commands return only errors, but the inspected v2 command uses a typed `Presenter[CreatePositionOut]` carrying an aggregate. The reference contains old and `internal/v2` implementations at once. The new project should select one clean convention rather than copying both generations. Go `AggregateStore` exposes `*gorm.DB`/`any` transactions, so this is pragmatic layering, not a fully storage-agnostic domain/application boundary.

CQRS means distinct command/event and read-model paths here, **not demonstrated physical separation of write/read databases**. The separate projection executable connects through the same executor DSN and constructs both read repositories and event store with that handle. Separate binaries are useful precedents; independent database ownership per future Justix service still needs an explicit contract.

## 3. Event store and replay: observed guarantees and limitations

`pkg/eventsourcing/event_store.go:27` persists an event envelope containing `event_id`, `event_type`, `aggregate_id`, `aggregate_type`, `version` (`uint64` in Go), `data`, `metadata`, and `timestamp`. The SQL migration uses `varchar`, `bigint`, `bytea`, and `timestamp`. `GetEvents` reads one aggregate ordered by ascending version; `GetEventsByVersion` reads versions greater than the supplied version. `SaveEvents` bulk-inserts with GORM.

`executor/db/migrations/20250727151147_create_eventsourcing_tables.sql` defines:

- `events` partitioned by hash of `aggregate_id` into three partitions; unique `(aggregate_id, version)`; ascending/descending aggregate/version indexes.
- `snapshots` unique on `aggregate_id`; state bytes and version.
- Version-increment triggers on updates to events, snapshots, positions and position_orders.

This gives a database collision constraint for competing writes at the same aggregate version **if this migration is installed**. It is not an explicit expected-version API, automatic retry, global event ordering, or proof of append-only permissions. The migration does not make `event_id` unique and its update trigger does not prohibit all historical modification/deletion. Schema also references trading read tables, so it is not directly reusable as a standalone generic Justix migration.

`AggregateBase.Apply` invokes the transition handler, increments the version, and records uncommitted events. Replay via `RaiseEvent` rejects a version not greater than the current one, then adopts the supplied version; it does not itself require each replayed version to be exactly the previous version plus one. Source: `pkg/eventsourcing/aggregate.go:150` onward.

**Snapshots are not functioning persistence in the inspected implementation.** `SnapshotFrequency` is `5` (`snapshot.go`), and aggregate-store branches invoke snapshot methods, but `snapshot_store.go:53` returns `nil, nil` from `GetSnapshot` and `:77` returns `nil` from `SaveSnapshot`; actual SQL bodies are commented out. Aggregate loading therefore replays full history. Do not promise production snapshotting or snapshot-derived performance from interface/schema presence.

## 4. Transactions and publication: do not inherit invented reliability

`pkg/eventsourcing/aggregate_store.go:122` has two Save paths. Outside a detected transaction it wraps event insertion and optional snapshot work in `db.Transaction`, then starts a goroutine publishing saved events after success. With a detected transaction it inserts and appends events to in-memory `pendingEvents`; `Commit` commits the DB then publishes pending events asynchronously. `Rollback` clears pending events. This establishes **intended post-commit publication**, not a durable atomic database-to-broker handoff.

Important limits visible in source:

- Publisher return errors are ignored by aggregate-store goroutines. A process crash after SQL commit or failed broker publish can leave a persisted event undispatched; no outbox/durable dispatcher is present in this path.
- `pendingEvents` is an in-memory slice; `WithTx` copies its slice header, not a durable shared transaction unit. Transaction detection compares GORM connection-pool fields. These implementation details need tests rather than an assumed nested-transaction guarantee.
- The sample multi-aggregate use case shows transaction intent but its deferred rollback checks an outer `err`, while save/commit failure branches use shadowing `if err := ...`; that is a rollback-cleanup risk on those paths. Source: `executor/internal/v2/app/usecases/create_position.go:191`.
- `pkg/eventbus/mqpublisher.go:24` defaults `noWait=true` and only calls channel `Confirm` when `!noWait`; executor publisher does not override it. `pkg/brokerx/amqp.go:153` sets message body/content-type/headers but no persistent `DeliveryMode`. Durable exchanges/queues alone are not proof of durable publication.
- Repository search of `*.go` and `*.sql` found no `outbox`, `LISTEN events`, `WaitForNotification`, `pg_notify`, `NotifyPublish`, or `DeliveryMode` implementation. `roadmap/cdc-implementation.md` and `roadmap/phase1-notify.md` describe proposed CDC work; they are not executable evidence of a delivered bridge.

Accordingly, this audit cannot assert guaranteed eventual delivery, exactly-once effects, atomic SQL+RabbitMQ commit, or service-wide transaction atomicity. Correctly scoped database transactions can protect writes on one connection; the publisher boundary is separate.

## 5. Projectors, retries, and read consistency

Shared projection contract is `When(ctx context.Context, event Event) error` (`pkg/eventsourcing/projection.go`). The independent `executor/cmd/projection/main.go` connects to PostgreSQL/RabbitMQ, configures channel prefetch `1`, constructs read repositories and projections, and subscribes them. This is an asynchronous read-model path, not synchronous read-your-writes.

The v2 position projection dispatches typed events, creates SQL read rows and for updates asks `PositionRepository.GetByVersion(id, incomingVersion-1)` before saving the new version. Its update handler also reloads the **latest** aggregate, so rebuilding a historically exact point-in-time view cannot be presumed. Sources: `executor/internal/v2/adapter/projection/position/projection.go:41`, `:61`, `:127`; `executor/internal/adapter/repository/position.go:89`.

Broker subscriber defaults to retry enabled, delay `500ms`, attempts `3`; it declares durable primary/idle/retry/failed queues and routes failures through dead lettering. `Subscribe` starts goroutines per delivery. Handler success is ACKed; deserialization failure is also ACKed. Once retries are exhausted it attempts to send the body to the failed queue and then ACKs even if that send failed. Sources: `pkg/eventbus/mqsubscriber.go:40`, `:108`, `:283`, `:408`.

These are retry mechanisms, **not** proof of deduplication, permanent message retention, globally ordered processing, a replay cursor, safe projection rebuild/swap, or duplicate-safe external side effects. The version-conditioned projection can fail on out-of-order/duplicate delivery; no generic inbox/dedup ledger is established by the inspected example. Prefetch `1` in one entrypoint does not confer total ordering across all consumers/event-type queues.

## 6. Reuse versus trading-specific implementation

Reuse as architectural patterns: root shared Go packages, per-service ports/adapters, explicit composition roots, typed commands/events, event-replayed aggregates, GORM-backed event persistence, SQL query projections, asynchronous domain-event subscriptions, correlation-aware logging/tracing, config and graceful-shutdown organization, service Dockerfile pattern.

Do not copy as Justix domain code: positions/orders and crypto amount conversion, market/limit execution, OKX/Jupiter/0x sagas, wallet signatures, Ethereum/Solana clients, Telegram trading signals/presets, token price-oracle/trigger rules, trading table names/migrations, copied vendor environment or credential configuration. These dependencies are not required merely because Justix uses Go/CQRS/event sourcing.

Justix already confirms separate backend `RFQ`, `QuotationVersion`, `PurchaseOrder` despite the unified Deals navigation; canonical `VehicleUnit` with warehouse-placement-driven views; and separate linked `Shipment`. Those ownership/invariant decisions come from `docs/justix-auto/business-logic.md`, not from Gaze service names or UI menu items. This audit does not invent their service allocation.

## 7. Decisions required before implementation handoff

1. **Service/data boundaries:** map confirmed Justix aggregates to bounded services; decide per-service database/schema ownership and cross-service workflows. Do not infer one service per UI navigation section or import Gaze's shared-DSN access as a compulsory microservice rule.
2. **Reliability hardening:** explicitly decide whether to preserve only Gaze's architecture shape while correcting dispatch gaps. Recommended architecture decision: durable transactional outbox + confirm-aware relay + idempotent consumers/inbox where needed; choose contracts, ownership and tests before implementation. This is a proposed improvement, not a reference feature or already-approved scope.
3. **Concurrency/idempotency:** expected-version command contract, conflict response, command idempotency keys, retry ownership, duplicate side-effect control, aggregate ordering strategy, SQL constraints.
4. **Event schema/lifecycle:** event envelope IDs, tenant/company/actor/correlation/causation metadata, schema versions/upcasting, sensitive-data retention/deletion strategy and authorization boundaries. Gaze's byte fields alone do not decide these.
5. **Read-side contract:** projection lag/pending UI behavior, command result versus query result, rebuilding/checkpoints, poison-message operations, catch-up/reconciliation, snapshot policy. Snapshot support requires real implementation if selected.
6. **Justix integrations and auth:** tenant/company/branch/partner authorization, external document storage, payment facts/providers and imported logistics facts remain Justix decisions; trading gateways cannot answer them.
7. **Versions and delivery tooling:** choose supported Go/dependency/container pins for the new project, migration tool/version, CI/lint/test/build/run commands, service ports and deployment target. Reference versions above are pinned observations, not current-version recommendations; do not silently inherit the reference gRPC development version or unpinned Jaeger image.
8. **React setup:** React is user-selected, but framework/build tool, TypeScript policy, router, query/cache layer, forms, validation and package manager are not established by this Go reference. Select these in Justix architecture rather than attributing them to Gaze.

Verification floor for reused infrastructure: aggregate replay and invalid-version unit tests; real PostgreSQL competing-writer/rollback tests; snapshot round trip if enabled; crash-after-commit/broker-outage and relay-restart tests; duplicate/out-of-order projector tests; failed-queue replay and tenant-isolation tests. Source inventory found **zero `*_test.go` files** in the reference; this is not proof of no external testing, but no inherited Go test safety net was demonstrated here.

## Handoff status

Reference approach is clear enough to inform architecture: Go multi-service repository, CQRS with PostgreSQL event history and SQL read models, ports/adapters and RabbitMQ propagation; Redis and observability where applicable. It is **not** a drop-in reliable event platform or a React architecture. Preserve the user's chosen approach, document the above decisions, then design the actual Justix implementation. No target project was initialized or modified by this audit.
