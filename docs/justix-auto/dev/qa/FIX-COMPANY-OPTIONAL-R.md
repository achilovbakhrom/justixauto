# FIX-COMPANY-OPTIONAL — independent reviews

Decision: docs/justix-auto/business-logic.md §9 (user, 2026-09-26).

## R1 — eba3a6b (parent b7b861d): BOUNCE

- Critical: `Company.Update` applied the PATCH body with an unconditional
  `c.RegistrationNumber = …`, so a PATCH omitting `registration` erased a stored
  number; only the two frontends guarded it; no test for the omit case.
- Warning: swagger `CompanyInput.required` listed optional fields.
- Verified OK: migration 000020 up/down, `EmailOrLoginTaken`, edit payloads,
  displayName default. Checks: vet, identity tests, typecheck, 460/460 unit.

## R2 — 7a33ae5 (repair, parent eba3a6b): GREEN, no findings

- `apply(…, keepRegistration)`: create passes false, update passes true; empty or
  omitted keeps the stored value, non-empty replaces and stays duplicate-checked.
- Only two callers of `CompanyInput.apply` (grep). Tests: 4 service unit tests +
  e2e omit / "" / "REG-2" through handler→DB.
- swagger `CompanyInput.required` = ["name"]; `make openapi-check` exit 0.
- Cumulative rescan b7b861d..7a33ae5: no remaining Critical.
- Checks: `go.sh vet ./...` 0; `devtool test ./internal/modules/identity/...` ok;
  `make openapi-check` 0; `npm run test:unit` 460/460.
