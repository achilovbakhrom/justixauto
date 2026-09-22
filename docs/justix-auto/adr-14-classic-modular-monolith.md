# ADR-14 — Classic modular monolith replaces microservices + CQRS/ES

- Date: 2026-09-22
- Status: **approved by the user** (explicit instruction in this session)
- Supersedes: ADR-01, ADR-02, ADR-03, ADR-05 and every event-sourcing,
  outbox/inbox, projection and per-owner-database rule in `architecture.md`.

## Decision

The Go backend is rebuilt from zero as a **modular monolith** with the classic
layering **handler → service → repository**:

| Concern | Choice |
|---|---|
| Deployment | One binary `cmd/api`; migrations via `cmd/migrate` |
| Database | One PostgreSQL; one schema per module (`identity`, `inventory`, `commerce`, `retail`, `financing`, `insurance`, `documents`) |
| HTTP | Echo, routes `/api/v1/<module>/…` (matches `web/packages/api`) |
| ORM | GORM; `TranslateError` on, explicit repositories, no AutoMigrate |
| Migrations | Versioned SQL files in `migrations/`, golang-migrate, embedded |
| Concurrency | Optimistic locking: `version` column, `ETag` / `If-Match`, 412 on stale |
| Errors | Services return `internal/platform/apperr` kinds; one Echo error handler maps them to 404/409/412/422 |

Layout and rules: [`internal/AGENTS.md`](../../internal/AGENTS.md).

## Why

After weeks of work the CQRS/ES implementation contained ~34k lines of
infrastructure (event store, outbox/inbox, projections, gap recovery, custody
messaging, seven isolated databases) and no business feature. For one team at
this stage the cost outweighed the benefits. The audit needs of financing and
insurance are met with version columns, history/audit tables and immutable
snapshots, which the business rules already require.

## Kept

- `web/` (React apps and API client), `docs/` (business rules, open decisions,
  HTML mocks), the agent setup and Git policy.
- Business invariants from `business-logic.md`: tenant/company scoping,
  immutable submitted snapshots, stale-version rejection, retry without
  duplicates, audit of sensitive actions, integer money.

## Removed

`pkg/`, `services/`, Go contract/integration tests, `tools/owner-migrate`, the
contract generator and RabbitMQ / per-owner PostgreSQL local infrastructure.
The previous backend is preserved at Git tag `archive/cqrs-es-backend`.

## Consequences

- Backend tasks on `dev/task-board.md` that assume event sourcing, outbox/inbox,
  projections or per-owner databases are obsolete and must be re-planned against
  this ADR before being assigned. Frontend and documentation tasks are unaffected.
- If a module later needs asynchronous integration, add a transactional outbox
  table in that module's schema; do not reintroduce a broker by default.
- A module can still be extracted into a service later because modules only
  talk through service methods and own their schema.
