# Go backend (modular monolith)

Decision: [ADR-14](../docs/justix-auto/adr-14-classic-modular-monolith.md).
One binary (`cmd/api`), one PostgreSQL, Echo + GORM, SQL migrations in
`migrations/`. Use `bash tools/go.sh` (pinned Go).

- Each module lives in `internal/modules/<module>/` as one package per layer:
  `model/` (GORM models, enums, filters, value types), `repository/` (GORM
  code, `store.go` + one file per aggregate), `service/` (business rules, one
  file per service, `ports.go` with the repository interfaces and the ports
  to other modules, `deps.go`), `handler/` (Echo handlers, DTOs, `Routes`),
  and `module.go` at the root (`New`, `Register` under `/api/v1/<module>`,
  the cross-module methods, `Permissions`, and type aliases for everything
  other code uses). Imports run one way: `handler → service → model` and
  `repository → model`; the service declares the repository interfaces and
  the repository satisfies them structurally. Only the root package is
  imported from outside the module. Use `service.Warehouse`, not
  `service.WarehouseService`. Split files per aggregate inside each layer.
- Handlers: bind/parse HTTP, call one service method, write JSON. No rules.
- Services: all business rules and validation; return `apperr` errors; never
  import Echo or GORM. Depend on the interfaces in `service/ports.go`.
- Repositories: GORM only; resolve the ambient transaction with
  `database.Conn(ctx, db)`; translate `gorm.ErrRecordNotFound`/duplicate keys
  into `apperr`. Mutations use optimistic locking (`version` column + If-Match).
- Modules own their tables in their own PostgreSQL schema (`identity.*`, …).
  Never query another module's tables; call its exported service instead.
  Cross-module transactions only through an explicit service method that takes
  a `*gorm.DB` transaction, never by sharing repositories.
- Schema changes are new numbered files in `migrations/` (up + down); never edit
  an applied migration. No GORM AutoMigrate.
- Money: integer minor units + currency code; never float.
- HTTP contract: `docs/justix-auto/contracts/http-domain.md`. Use `httpx.Data`
  (`{data,revision}` + ETag), `httpx.List` (`{items,nextCursor,asOf}`) and
  return `apperr` errors (rendered as `{error:{code,message,fields,traceId}}`).
  Models never go to JSON directly; map them to DTOs.
- Auth: the identity module authenticates every `/api/v1` request. Read the
  caller with `auth.Get(c)`; protect routes with `auth.Require(perm...)` and
  company-scoped routes with `auth.RequireCompany()`. In services use
  `actor.Allow(perm)` (returns the precise 403 reason) rather than `Can`.
  Add new permission keys to the identity catalog
  (`<module>.<resource>.<action>`) and mark `RequiresMFA` for sensitive ones
  (decisions, terms, payments, fulfillment, sensitive downloads).
- Idempotency lives in the data, never in request keys: creates carry a unique
  business key (409 on repeat), actions check the state they transition from,
  existing-resource writes use If-Match (412). Background jobs run on one
  replica through `database.RunOnce` (advisory lock); migrations run once in
  the deploy Job.
- `internal/app` is the composition root (used by `cmd/api` and tests): it
  registers module permissions (`Permissions []auth.PermissionInfo`) with
  identity and mounts every module. Modules never import each other: a module
  needing another's data declares a small port interface (e.g. commerce
  `Directory`) and `internal/app` adapts the other module's exported service.
  Shared helpers are in `internal/pkg` (`database`, `validate`, `jsonx`,
  `httpx`, `auth`, `apperr`, `envx`). Modules never import each other, and `internal/pkg`
  never imports a module or `internal/app`.
- Tests: pure logic as in-package unit tests; behaviour end to end through
  HTTP in the external `<module>_test` package with `internal/e2e`
  (full app, active seller companies, MFA admin, CSRF handled).
  `bash tools/test-go.sh` starts a throwaway database.
