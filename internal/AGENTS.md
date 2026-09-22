# Go backend (modular monolith)

Decision: [ADR-14](../docs/justix-auto/adr-14-classic-modular-monolith.md).
One binary (`cmd/api`), one PostgreSQL, Echo + GORM, SQL migrations in
`migrations/`. Use `bash tools/go.sh` (pinned Go).

- Each module lives in `internal/modules/<module>/` with the fixed layering
  `handler.go → service.go → repository.go`, plus `model.go` and `module.go`
  (wiring + routes under `/api/v1/<module>`). Split files per entity when a
  module grows (`company_service.go`, `branch_service.go`, …).
- Handlers: bind/parse HTTP, call one service method, write JSON. No rules.
- Services: all business rules and validation; return `apperr` errors; never
  import Echo or GORM. Depend on repository interfaces declared in the module.
- Repositories: GORM only; translate `gorm.ErrRecordNotFound`/duplicate keys
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
- Retries: authenticated POSTs require `Idempotency-Key`; the platform
  middleware replays the first 2xx response. Existing-resource writes use
  If-Match instead.
- Module permissions live in the module (`Permissions []auth.PermissionInfo`)
  and are registered with `identity.RegisterPermissions` in `cmd/api`.
  Modules never import each other; shared helpers are in `internal/platform`
  (`database`, `validate`, `jsonx`, `httpx`, `auth`, `apperr`).
- Tests: pure logic as unit tests; behaviour end to end through HTTP against a
  real database with `internal/testkit` (signed-in company users, MFA admin,
  CSRF/Idempotency handled). `bash tools/test-go.sh` starts a throwaway DB.
