# Owner feature migration readiness handoff

Date: 2026-09-15. Coordinator finding; no schema or API change approved here.

The integrated Identity/Inventory/Commerce persistence scaffolds intentionally
require a clean exact owner ledger version 1 and their immutable mechanics
markers. Their results explicitly defer later feature/custody compatibility.
T-059 introduces Identity `0012_auth.up.sql`, but owns no change to Identity's
existing `adapter/postgres/unit_of_work.go` or its readiness tests. T-060/061
must not work around the version-1 check after installing that feature schema.

Before assigning affected feature persistence/runtime composition, the
coordinator must approve and assign a narrow version/marker readiness handoff.
It must preserve the scaffold's fail-closed default, trusted expected migration
identity, narrow role checks and read-only validation before factory/callback.
Accepting arbitrary newer versions, forcing the ledger back to 1, disabling
checks or silently editing another task's UOW leaves is not a solution.

The actual migration runner remains T-022. Its explicit artifact/order checks
must distinguish private owner ledger versions from shared messaging/storage
markers. Existing installations and dirty/incompatible ledgers are retained;
no automatic repair, destructive reset or applied migration rewrite is implied.
Later additions with lower migration numbers than installed history require an
explicit compatibility/disposition check, not silent skipping. Synthetic fresh
fixtures do not establish incremental upgrade behavior.

This gap does not block T-058's bounded auth wire contract, generic contract
generation, current broker/checkpoint work or independent owner scaffolds.
T-059 can define its schema contract, but a proposed versioned installation must
have the concrete UOW/runtime compatibility assignment before dependent runtime
behavior is marked ready. A technical handoff cannot approve T-002 settings,
recovery/delivery policy or new business permissions.

Inputs: T-024 r2/T-025/T-026 results and exact owner UOW source; T-059/T-060/
T-061 ownership; approved architecture and storage handoffs. Reuse their reviewed
mechanics and record any additional owned leaves/dependency edges explicitly.
