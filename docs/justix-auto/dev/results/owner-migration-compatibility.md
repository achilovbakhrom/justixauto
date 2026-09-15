# Owner migration compatibility proposal result

2026-09-15. Proposal complete; independent exact-commit QA and coordinator adoption
pending. No implementation, migration execution or runtime readiness is claimed.
Base: `f0a72a94b9badabc32f9ad028f913a98960e6b4a`.
Branch: `task/owner-migration-compatibility`.

Draft: [owner-migration-compatibility.md](../../state/drafts/architect/owner-migration-compatibility.md).

Inspected actual Identity/Inventory/Commerce UOW and owner SQL, their reviewed
results/tests, T-022/T-059/060/061 ownership, task index and approved messaging/
storage/source-reference contracts. The v1 marker supplies no artifact history;
existing tests simulate the ledger and do not test an actual migration runner.

The proposal specifies exact trusted profiles, immutable owner-local artifact
and feature evidence, explicit baseline provenance, read-only checks before bind
and each Run, separate legacy/custody privileges and marker axes, actual
golang-migrate receipt+clean finalization, failure reconciliation and retained
lower-number migration rejection. Six bounded aliases and precise existing-task
extensions avoid changing reviewed SQL or silently broadening feature ownership.

Only this result and the assigned draft were added. No canonical/source files,
task states, dependency pins, original SQL, mocks or external applications changed.
Validation: local Markdown links and exact assigned path ownership checked;
13 new proposal leaves are unowned in the inspected task index; the two Identity
UOW leaves are explicitly identified as successors to T-024. The hypothetical
934-node DAG (928 existing plus six aliases) is acyclic after the named new
edges. Whitespace checked.
These are document/ownership checks, not executable proof of the proposed design.

Coordinator follow-up: retrieved the exact approved migrate v4.20.1 module into
`/private/tmp/justixauto-owner-migration-audit`; both module sums match T-001.
Inspected database.Driver, NewWithInstance, runMigrations and pgx/v5 driver source.
The upstream pgx driver's dedicated connection is private and its SetVersion
commits independently, so the initial wrapper assumption was removed. The final
proposal explicitly assigns a pgx-v5-backed project database.Driver and a separate
CLI task, anchored to the actual engine call sequence. Existing pgx v5.11.0 APIs
support the specified complete SQL batch and transaction-status inspection.
No dependency files changed and no database/migration execution occurred.

Open items are explicit: technical adoption; actual new artifact hashes after
implementation; real baseline attestations; dirty/higher-version dispositions;
later-owner assignments and exact feature privilege profiles. Security/auth
transport, admission/delivery activation and financial-policy gates are unchanged.
