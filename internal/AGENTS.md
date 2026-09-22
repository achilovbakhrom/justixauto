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
- Tests: service rules with in-memory fakes, HTTP mapping with httptest, and
  repository tests against a disposable database via `TEST_DATABASE_URL`.
