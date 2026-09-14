# Small-task preview: first development wave

Status: **proposal only; not the approved backlog or execution board**.
Date: 2026-09-13. This previews decomposition quality and first dependencies.
The architecture checkpoint, then the PO release-backlog checkpoint, precede
formal `T-NNN` files/branches. Do not start any candidate from this document.
Application code also requires the project's own Git root and initial commit.

Every estimate is 2–4 developer hours after listed prerequisites, not a delivery
promise. Split again if actual implementation exceeds that scope. Each candidate
includes focused tests; later integration testing does not replace them. Paths
are proposed ownership zones, reconciled with the architecture before PM assigns
exact files. `C-*` IDs are preview identifiers, never implementation task IDs.

## Foundation candidates

| ID | Bounded result and acceptance | h | Prerequisite | Exclusive path scope |
|---|---|---:|---|---|
| C-01 | Dependency decision manifest: supported compiler/container/library versions, reference deltas, compatibility/security evidence; no trading packages copied | 3 | Architecture approval | `docs/justix-auto/dev/dependency-lock.md` |
| C-02 | Single Go module and ONE example service compile skeleton; ports/domain have no adapter imports; compiler check succeeds | 3 | C-01 + Git | `go.mod`, `go.sum`, `services/identity/{domain,app,port,adapter,cmd}` scaffolding only |
| C-03 | Local PostgreSQL Compose profile, service-private DB users, disposable integration DB; cannot read another service's tables | 3 | C-01 + Git | `infra/local/postgres/`, Compose database fragment |
| C-04 | Local RabbitMQ Compose profile with explicit image pin/health check; restart preserves a synthetic persistent message | 3 | C-01 + Git | `infra/local/rabbitmq/`, Compose broker fragment |
| C-05 | Event envelope/schema fixtures; UUID/revision/integration-sequence parsing and privacy allowlist tests reject malformed payload | 3 | C-02 | `pkg/events/envelope*` |
| C-06 | SQL event-stream migration; duplicate event ID or aggregate version rejected and indexes verified | 3 | C-03,05 | `pkg/eventstore/migrations/` |
| C-07 | Expected-version append adapter for ONE stream; concurrent writers produce one winner; injected failure rolls back | 4 | C-06 | `pkg/eventstore/append*` |
| C-08 | Ordered replay adapter; missing version fails explicitly, no latest-state lookups or side effects during replay | 3 | C-07 | `pkg/eventstore/replay*` |
| C-09 | Outbox insert in same append transaction; rollback leaves neither event nor publishable row | 3 | C-07 | `pkg/outbox/store*`, outbox migration |
| C-10 | One relay publisher adapter: persistent message + confirms + unroutable handling; lost confirmation retries same event ID | 4 | C-04,09 | `pkg/outbox/publisher*` |
| C-11 | Inbox migration/transaction adapter; duplicate event effects commit once and ACK follows commit | 4 | C-03,05 | `pkg/inbox/store*`, inbox migration |
| C-12 | Version-aware projection consumer for synthetic ONE aggregate; integration sequence gap waits/reconciles, private domain revisions do not create false gaps | 4 | C-08,11 | `pkg/projection/consumer*` |
| C-13 | Poison-event quarantine adapter; failed quarantine preserves original message, redrive remains duplicate-safe | 3 | C-04,11 | `pkg/inbox/quarantine*` |
| C-14 | Generic idempotency ledger with secret-safe digest hook; identical retry returns receipt, changed payload conflicts, receipt + mutation rollback together | 4 | C-07 | `pkg/commands/idempotency*` |
| C-15 | Money transport parser/value object; string minor units/currency precision and overflow tests; no pricing formula | 3 | C-02 | `pkg/money/` |
| C-16 | Integration test failure harness for outbox/inbox crash points; demonstrates committed-but-unconfirmed recovery without duplicate effect | 4 | C-10,11,13,14 | `tests/integration/messaging/` |

These are infrastructure examples, not a complete reliability subsystem. Relay
claim leases, reconnects, retry scheduling, schema upcasters and projection
rebuild operations need their own formal tasks if included in approved scope.
All shared dependency/lockfile edits remain coordinator-serialized, even when
implementation directories are disjoint.

## First user-visible journey candidates

Recommended first journey: Admin creates a provider account → activates it →
its administrator signs in to the correct separate app. This tests the shared
tenant boundary before money, stock or external integration behavior. Activation
must not grant legal/compliance status or publish financial programs.

| ID | Bounded result and acceptance | h | Prerequisite | Exclusive path scope |
|---|---|---:|---|---|
| C-17 | Provider organization aggregate create; registration/country duplicate and draft-default tests, zero application/program creation | 3 | C-05,07 | `services/identity/domain/organization*` |
| C-18 | User/global-role and role-free membership DTO/invariant contract tests; role alone grants no tenant access | 3 | C-05,07 | `services/identity/domain/access*` |
| C-19 | Credential hasher/private repository adapter; no plaintext/hash/secret in events, audit, errors or idempotency body | 4 | C-02 + approved security ADR | `services/identity/adapter/credentials/` |
| C-20 | Atomic provider-create application command; failure at each insert leaves no partial company/user/membership/credential | 4 | C-14,17,18,19 | `services/identity/app/provision_provider*` |
| C-21 | ONE empty Admin React entry + shared component package wiring; build passes, no copied global mock runtime; other entries are separate follow-on units | 4 | C-01 + Git | root npm lock/config, `web/apps/admin` scaffolding, `web/packages/ui` scaffold |
| C-22 | Shared modal primitive using agreed library and existing visual tokens; inside click never closes, Escape/cancel closes safely, focus restored | 3 | C-21 | `web/packages/ui/dialog/` |
| C-23 | Country/region autocomplete fields; free country entry, dependent region reset and keyboard selection component tests | 4 | C-21 | `web/packages/ui/company-fields/` |
| C-24 | Provider form PRESENTATION with contract fixtures; account login/password/confirmation, validation, processing and error preserve only non-secret input | 4 | C-22,23 + reviewed HTML states | `web/apps/admin/src/integrations/provider-form/` |

Mock references for these FE examples:

- C-22: `/admin/` → Integrations → MFO → Add company; inside/outside dialog
  click, close, cancellation, keyboard focus. Current screenshots/behavior must
  be inspected at implementation, not asserted approved by this preview.
- C-23: same dialog → country/region; current `mocks/company-fields.js/css`.
- C-24: same dialog → company + first administrator fields; current
  `mocks/admin/management.js`, `mocks/demo-accounts.js`. Demo credential hashing
  or browser storage must not be copied into React or Go production logic.
- C-21 has no bespoke page design: empty bootstrapping and shared tokens only.

The journey is **not complete after C-24**. Production session/MFA/CSRF/bootstrap,
permissions, activation endpoint, form API binding, separate-app admission and
end-to-end QA must be sliced from the approved identity analysis. Missing auth/
MFA/pending/error visual states require scoped UX acceptance, not improvised UI.

## Preview parallel lanes (not a ready queue)

After approvals and C-01: backend scaffold, local PostgreSQL and local RabbitMQ
can proceed independently. After C-21, modal and autocomplete have separate
owners. Shared root lockfiles/config, aggregate transaction changes and final
integration are serialized. Maximum three workers; only one browser QA worker.

Each formal task will carry exact contract versions, acceptance criteria,
negative tests, OD gates, branch/worktree and QA commit. This document deliberately
does not turn unapproved estimates into an executable task board.

## Following domain waves

The reviewed analysis files contain additional bounded command/adapter examples:

1. Company/branch/warehouse setup and capacity guards.
2. Quantity-before-VIN receipt, VIN identification and shared reservation.
3. Partnership invitation/accept/decline/withdraw; offer/quotation/order journey.
4. CRM and retail sale referencing the same reservation owner.
5. Financier program/application/terms/document exchange.
6. Insurance submission/review/decision without automatic policy or handover.
7. Financial/route-policy-dependent commands once their exact OD gates resolve.

Coverage is not claimed exhaustive until PO acceptance criteria are approved and
PM maps each approved item to task files, dependencies and tests.
