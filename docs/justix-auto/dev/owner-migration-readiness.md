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

The bounded [architecture assignment](owner-migration-assignment.md) is active
in `.worktrees/owner-migration-compatibility`. Its proposal must pass independent
review before the coordinator promotes any implementation ownership or edges.


## Reviewed technical adoption

Exact proposal `c3734a5c3f2863be8fe752338c11740b9bdf31a4` passed [independent proposal QA](qa/owner-migration-compatibility.md).
The [technical approval](../state/approvals/owner-migration-compatibility.md)
maps six aliases to T-929–T-934, eight existing edges and the auth manifest leaf.
Canonical promotion QA precedes assignment; implementation and actual PostgreSQL
driver/runtime evidence remain unfinished. Earlier findings remain provenance.

## Exact feature-grant profile boundary — 2026-09-15

T-930's implementation candidate supports nonempty exact table/column grants
with disjoint table ownership across feature identities. It rejects marker-only
feature contracts and overlaps rather than guessing grant supersession or union.
Complete historical feature markers/artifacts remain required independently.
Its first-auth/synthetic profile support still requires exact-commit QA; the
actual T-059 schema is absent, so no production compatibility is established.

Before assigning any schema/profile that revises an existing feature table,
overlaps another feature's grant contract, or needs marker-only feature grants,
approve an explicit bounded current-grant/supersession refinement and file
ownership. Do not discard prior markers, union permissions, or broaden this
candidate's API silently. This affects only those profiles, not independent
owner history, disjoint feature schemas or other ready tasks.

## Effective default ACLs and baseline manifests — 2026-09-15

T-930 independent QA demonstrated that an absent function `pg_default_acl` row
can mean PostgreSQL's built-in PUBLIC EXECUTE default, not denied access. Its
fix cycle evaluates built-in/global/schema defaults; new T-928 storage aligns
with the approved default-privilege audit. Explicit owner function/type default
denials are installation prerequisites. Existing original SQL and init templates
do not establish those defaults. Read-only compatibility checks must reject an
unsafe installation and must never alter its ACLs automatically.

Before concrete T-932/T-933 bundle installation is advertised, assign and review
the exact outer prerequisite provisioning and verify it under the actual owner
roles. Synthetic fixtures may set these defaults explicitly and must label that
setup. Preserve old SQL hashes; no silent revision of installed artifacts.

T-930 also verifies every private SQL artifact's adjacent manifest, including
version 1 and already installed lower versions. Concrete baseline companion
manifests have not yet been assigned or supplied. Before a real owner bundle is
registered, the coordinator must assign their exact leaves and trusted profile
assembly, with independently checked SQL/manifest byte hashes. Driver/runner
tests may use explicit synthetic manifests; that does not prove real bundle
readiness or grant authority to expand those tasks' source ownership.

## Explicit legacy messaging mode — 2026-09-15

[Retail independent QA](qa/T-027.md) reproduced that old narrow outbox grants
restored after actual custody activation can satisfy a scaffold's grant-based
legacy check. Retail's bounded fix must explicitly reject a present nonlegacy
mode before factories. This is configuration drift; the runtime cannot grant
itself privileges or bypass the database's mode guard through that restoration.

Earlier scaffolds use the same grant-based convention but remain unchanged;
this Retail reproduction does not replace their historical QA. Before composing
an owner root, its existing OWNER-COMPATIBILITY handoff must verify explicit
mode/profile compatibility rather than treating old grants as mode evidence.
T-930's exact profile path already checks declared legacy mode. No working
owner runtime or production custody compatibility is inferred here.
