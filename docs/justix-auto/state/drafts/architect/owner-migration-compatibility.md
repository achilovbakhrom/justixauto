# Owner migration compatibility — bounded technical proposal

2026-09-15. **PROPOSED; no implementation or deployment approval.**
Assignment: `dev/owner-migration-assignment.md` in the coordinator checkout.
Inspected base: `f0a72a94b9badabc32f9ad028f913a98960e6b4a`.
Task aliases below are local proposal names; the coordinator assigns numeric IDs.

## Evidence and scope

Confirmed project rules: seven private PostgreSQL owners; Go ports/adapters,
CQRS/event sourcing; explicit migrations with golang-migrate; read-only startup
readiness; no destructive automatic repair. See [architecture](../../../architecture.md),
[readiness finding](../../../dev/owner-migration-readiness.md),
[messaging contract](../contracts/messaging-delivery.md) and
[storage handoff](../contracts/projection-storage-handoff.md).

Observed source at the inspected base:

- `services/{identity,inventory,commerce}/adapter/postgres/unit_of_work.go`
  requires exactly one clean `public.schema_migrations` row at version 1,
  the exact owner mechanics marker/default, private database/runtime identity,
  and narrow mechanics privileges. `Check` is read-only; `Run` rechecks on its
  actual transaction before the repository factory and callback. The three
  implementations differ only in owner identity/comments/imports.
- Their unchanged `migrations/0001_mechanics.up.sql` requires the runner's
  dirty version 1, then explicitly begins/commits its DDL. It records mechanics
  version/owner, **not an SQL artifact digest or complete installation history**.
- T-024 r2/T-025/T-026 tests exercise a SQL approximation of the owner ledger
  protocol. No actual golang-migrate runner/driver integration exists. An added
  legacy messaging schema remains compatible; custody cutover revokes legacy
  outbox writes and correctly makes these adapters fail before binding.
- T-059 owns `0012_auth.up.sql`, but no existing UOW leaf. T-060/061 cannot
  make that installation usable by bypassing the exact version-1 predicate.
- Shared `schema.sql` and messaging/storage `000002`–`000006` install explicitly
  into each owner's database with their own markers. Their numbers are not
  private owner ledger versions. Version 12 does not imply those are installed.

The [Gaze audit](../../../reference/gaze-reference.md) supplies the explicit
composition/transaction pattern, not migration history, fail-closed grants or
delivery guarantees. HTML mocks establish no database compatibility behavior.
Everything below is a new technical proposal; it changes no financial policy,
authorization, security settings, auth response transport or business contract.

## P1. Exact trusted installation profile

Add a sealed, defensively copied `persistence.Profile` constructed only by the
outer adapter/composition from reviewed release artifacts. It contains:

1. Exact owner, database and runtime role; manifest format/revision; one expected
   clean private ledger head; ordered **complete** private artifact list.
2. For every artifact: numeric version, owner-relative filename, exact byte
   SHA-256, prerequisite artifact identities, and feature contract ID/revision/
   digest when applicable. Version 1 explicitly identifies the unchanged owner
   mechanics file. The expected head equals the final artifact's version.
3. Exact history-template revision/digest and baseline provenance identity;
   feature marker set and the typed runtime privilege requirements supplied by
   their approved schema contracts.
4. A separate shared mechanics/messaging/storage profile: exact applicable
   artifact hashes and marker revisions, explicit `legacy` or `custody` mode,
   required narrow privileges, and the owner-specific composition requirements.

Reject zero/negative/duplicate/out-of-order private versions, duplicate paths,
unknown owners/modes, empty hashes, path escape/symlinks, contradictory grants,
missing prerequisites and any private/shared namespace collision. A manifest
cannot declare a missing lower-numbered private artifact optional after the
head has passed it. Filenames are identifiers, never SQL interpolation.

The runtime never chooses its expected version by querying the database, scanning
whatever files happen to exist, or accepting request/tenant/environment-provided
version ranges. A deployment may select an already approved named profile;
arbitrary `minimumVersion`, `allowNewer`, permissive fallback and unvalidated
runtime JSON are forbidden. A future release may approve a different complete
set, including additional schema for a disabled feature; exact compatibility
still requires that whole set. Feature business activation is independent.

Each schema-producing task owns one adjacent manifest, e.g.
`services/identity/migrations/0012_auth.manifest.json`. It records the SQL hash;
the SQL does not embed its own hash, and the manifest does not hash itself.
Profile identity is SHA-256 of a canonical encoding of the ordered manifest
identities plus shared profile/privilege identities. No new dependency family
is needed. Profile immutability prevents mutation of caller-owned slices/maps.

## P2. Owner-local immutable installation evidence

Add an explicit psql template `pkg/eventstore/owner_history.sql`. It installs
`owner_migrations` in **one existing private owner database**, independently of
the private numeric ledger and all shared storage markers. It is not discovered
as a numbered owner migration. Do not edit any existing numbered SQL or markers.

The template requires the exact migration owner/database/runtime, clean private
version 1, existing owner mechanics/default, verified template/base hashes and
explicit baseline evidence. It must fail atomically if the schema already
exists, identities/privileges differ, the ledger is dirty or above 1, or locks
cannot be acquired within five seconds. A later populated installation without
history needs a separate reviewed adoption migration; this template must not
pretend it can infer the missing history. Stop affected processes first.

| Table | Immutable contents and constraints |
| --- | --- |
| `owner_migrations.compatibility` | One singleton: exact owner/database/runtime, format revision 1, template SHA-256, base version 1 and exact base artifact SHA-256, baseline kind `verified-installation` or `attested-existing-v1`, evidence/approval/backup/stopped-runtime refs, unique installation request UUID, finite installed time. Nonempty refs are required; they contain no credentials. |
| `owner_migrations.artifacts` | One row per private version: version PK, exact owner-relative filename unique, SQL SHA-256, predecessor version/hash, feature contract identity when applicable, unique install request UUID, installing manifest/profile digest, finite completion time, provenance kind/reference. Seed only version 1 from the explicitly accepted baseline. No synthetic history for skipped versions. |
| `owner_migrations.feature_contracts` | Feature contract ID/revision PK, unique private migration version, immutable contract digest. Each feature SQL writes its literal approved marker in the same DDL transaction; no success marker before successful schema creation/constraints/grants. |

All rows are append-only, including for the ordinary migration login: invoker
guards reject UPDATE/DELETE/TRUNCATE; runtime has only schema USAGE and SELECT.
New artifacts require the exact predecessor already recorded and an existing
matching feature marker (except the version-1 baseline). New feature markers
require the exact runner dirty version and owner identity. Installing a feature
does not modify prior markers or the compatibility singleton. Runtime cannot
INSERT even individual columns, delegate writes, own objects or alter guards.
Migration owners remain trusted administrators who could deliberately change
DDL; this is auditability and runtime isolation, not tamper-proof attestation
against the database owner. No SECURITY DEFINER bypass is needed.

For a fresh fixture, provenance can identify the verified execution of the exact
base artifact. Existing v1 requires an explicit operator attestation plus object,
owner/ACL and retained-data inspection with backup reference. A current file hash
and the old v1 marker alone cannot establish what historically ran. This proposal
approves neither an actual baseline attestation nor repair of an existing store.

## P3. Actual runner and failure protocol

Keep golang-migrate as the private version/dirty-ledger mechanism. The bounded
runner task must use the approved pinned library/driver and test its real
execution; it must not present another hand-written test ledger as integration.
Use a narrow wrapping database driver/hook so successful migration execution is
followed by **one transaction appending the exact artifact receipt and setting
that same private version clean**. If the pinned driver cannot provide this
boundary, stop and reslice the adapter; do not separately clean then append.

The wrapper delegates actual schema locking/version/SQL execution to the pinned
driver. It holds one owner installation lock for the entire operation, including
validation and finalization, with bounded acquisition; competing runner fails or
waits within that bound. Shared-template installation and private runner use the
same explicit owner installation lock. Do not assume unrelated golang-migrate
and psql advisory locks happen to be identical. No application runs migrations.

Initial bootstrap is explicit: install the unchanged shared base template, run
the unchanged private version 1 with the real runner, and retain verified base
execution evidence before installing the history template. This deliberately
permits the original clean-v1/no-history intermediate state; only the original
legacy constructor supports that state. The new compatible constructor requires
history. No feature upgrade starts before history exists. Receipt+clean atomicity
below applies to every subsequent private migration, not retroactively to the
unchanged version-1 file. A failed history installation preserves clean v1 and
leaves the new compatibility path unavailable; it does not reset/rewrite v1.

Before any dirty write, verify the full requested artifact bundle against its
trusted manifest, private history/ledger agreement, exact next version and every
prerequisite. Verify all paths/hashes, including already installed files, not
only the next file. An intentional numeric gap such as 1→12 is permitted only
when that is the complete reviewed sequence; it does not imply versions 2–11 ran.
Private version numbers are not dense counters or shared migration revisions.

For T-059 specifically: history exists at clean 1; set `(12,true)` through the
real runner; execute the complete verified `0012_auth.up.sql` including its
BEGIN/COMMIT. Its guard requires exact Identity owner and dirty 12 plus the
approved predecessor/history; it installs auth schema and its feature marker
atomically. On successful return, verify that marker/required object and grant
contract, then append artifact 12 and mark `(12,false)` atomically. Immutable
artifact 1 remains. The clean head and complete history must agree afterward.

If execution rolls back, times out, crashes after DDL commit, or finalization
fails, preserve the dirty ledger and all committed evidence. Readiness is false;
do not re-execute SQL, force clean, downgrade or reconstruct a missing receipt
automatically. A lost finalization reply may leave clean+receipt or dirty+no
receipt: reconcile by exact request/version/hash on a new owner connection;
do not report completion from the client error alone. Inconsistent combinations
are blocked. Recovery of a retained dirty installation requires its own explicit
reviewed disposition and is not delivered by this task.

Reject a newly added artifact below or equal to an already installed head when
its exact receipt is absent or different, even if stock migration discovery
would skip it. Example: installed `[1,17]`, later bundle `[1,12,17]` fails before
changing the ledger. Never relabel 12, rewrite 17, reset the ledger to 1 or silently
skip 12. A separate reviewed forward migration above 17 or explicit audited
adoption plan must establish compatibility. Retained data/history must survive.
Likewise, a new manifest missing any installed artifact, changed historical
bytes, extra ledger rows, dirty state, absent markers or unknown history fails.

## P4. Read-only adapter boundary and custody distinction

Preserve existing `NewUnitOfWork(db, bind)` and its mechanics-v1 legacy behavior.
Add explicit `NewCompatibleUnitOfWork(db, profile, bind)` to Identity first. The
owner adapter binds fixed Identity database/role constants and rejects a profile
for any other owner. It is inert. Both explicit startup `Check(ctx)` and **every
Run on its actual ReadCommitted transaction before factory binding** perform
the complete compatibility check. Nil/partial profiles do not select defaults.
No caller-supplied arbitrary SQL or permissive readiness callback is accepted.

For the explicit path require exact clean private head; exact complete immutable
history including baseline provenance and predecessor chain; exact feature marker
set; unchanged owner mechanics/default; exact shared profile/markers; and the
approved privileges. Extra or missing recorded artifacts/markers fail even when
the head matches. Metadata is installer evidence; the check also validates the
named required objects and effective permissions. It is not a full schema dump
equivalence proof or business authorization. Feature schema tests own exact
column/constraint semantics, and feature schema contracts own object access.

Maintain the existing denials for privileged/migration/database-owner identities,
all transitively reachable roles including NOINHERIT/SET ROLE paths, PUBLIC,
database CREATE/TEMPORARY, schema CREATE, historical and metadata mutation,
TRUNCATE/TRIGGER/REFERENCES/MAINTAIN, table/column grants and delegation. Verify
required grants as well as forbidden grants; inspect applicable default ACLs so
new objects cannot silently inherit broader permissions. Metadata remains
runtime SELECT-only. Feature contracts enumerate their own tables and exact
mutable columns; this proposal grants no generic feature-wide CRUD, SELECT on
other owners, sensitive-table permission or unapproved financial write.

Split profiles strictly:

- `legacy`: preserve legacy outbox/inbox work grants and reject custody mode.
  The explicit profile checks whichever additive shared artifacts it declares;
  the original constructor retains its historically tested v1 behavior.
- `custody`: require exact messaging schema 2 and actual mode custody, correction
  revision 3/route format 1, and the applicable revision 4/5/6 storage lineage
  with exact approved hashes/owner/runtime. Require the custody ACLs and revoked
  legacy outbox writes; **never restore old INSERT merely to pass a legacy
  check**, and never require both incompatible privilege profiles at once.
  Require the actual T-919 writer, T-920/921/923 intake/dispatch, T-922 transaction
  inbox and storage/fence composition. Shared checker success alone cannot
  prove those adapters were assembled or that any consumer is admitted.

Custody predicates must be implemented/tested as a separate bounded checker
task against the approved SQL, before an owner opts into that profile. Root
T-580 must explicitly assemble and verify its named components/registrations,
admissions, contract schemas and receiver/generation participation. Readiness
does not mutate mode, admit streams, recover gaps, run business commands or ACK.
Do not confuse private owner head 12 with shared schema 2/revisions 3–6. A healthy
database is not a delivery guarantee; custody ACK continues to mean durable
bytes/all jobs, not consumer effects.

Each check is read-only and fails closed on catalog/permission/query errors.
Within Run it checks the actual transaction, not a cached startup result or a
different pool. The offline migration protocol requires stopped processes and
closed old channels; these checks do not claim to fence concurrent malicious
administrative DDL. Existing source/receiver lock ordering and T-009 unknown
commit semantics remain unchanged; no nested UOW, automatic retry or broker call.

## P5. Bounded ownership and dependency proposal

Every alias is at most four effort-hours; reslice before implementation if that
bound cannot cover its named behavior. Paths below are exact repository leaves.
Tests embed synthetic fixtures in their assigned test leaf; no shared test-file
ownership is implied. New tasks contribute to B-01.AC1; T-591 remains its closer.

| Alias | Direct prerequisites | Exclusive new/change leaves and deliverable |
| --- | --- | --- |
| OM-HISTORY | T-008, T-024 | `pkg/eventstore/owner_history.sql`; `pkg/eventstore/owner_history_test.go`. Owner-local immutable metadata template and real PG isolation/installation tests, no runner. |
| OM-PROFILE | OM-HISTORY, T-009, T-024 | `pkg/persistence/profile.go`; `pkg/persistence/profile_test.go`; `pkg/persistence/check.go`; `pkg/persistence/check_test.go`. Sealed exact manifests and read-only metadata/legacy privilege verification; custody selection explicitly unsupported. |
| OM-IDENTITY | OM-PROFILE | Existing `services/identity/adapter/postgres/unit_of_work.go`; existing `services/identity/adapter/postgres/unit_of_work_test.go`. Explicit compatible constructor, unchanged legacy default, actual pre-bind/Run verification. Synthetic feature schema only until T-059. |
| OM-RUNNER | OM-HISTORY, OM-PROFILE, T-001, T-005 | `tools/owner-migrate/main.go`; `tools/owner-migrate/main_test.go`; `tools/owner-migrate/driver.go`; `tools/owner-migrate/driver_test.go`. Actual pinned golang-migrate driver wrapper, verified artifact bundle, locked dirty/DDL/receipt+clean protocol and retained upgrade/fault tests. No dependency/lock edit is implied; coordinator owns any needed already-approved package pin insertion. |
| OM-CUSTODY | OM-PROFILE, T-919, T-922, T-924, T-925, T-928 | `pkg/persistence/custody.go`; `pkg/persistence/custody_test.go`; existing `pkg/persistence/check.go`; existing `pkg/persistence/check_test.go`. Exact shared lineage/mode/ACL checker and explicit dispatch from the common Check, replacing only the unsupported-custody branch after OM-PROFILE integration; no service wiring or broker activation. |

OM-IDENTITY and OM-CUSTODY can run independently after OM-PROFILE: only the
former owns Identity UOW leaves and only the latter extends the shared checker.
OM-CUSTODY must retain all OM-PROFILE legacy checks. OM-RUNNER's targeted Go
verification includes `./tools/owner-migrate`; the usual project command covering
only services/pkg/tests does not discover that package automatically.

Existing-task amendments, applied only by the coordinator after proposal approval:

1. T-059 gains dependencies OM-HISTORY and OM-IDENTITY, plus one new leaf
   `services/identity/migrations/0012_auth.manifest.json`. Its existing SQL/test
   leaves implement marker/dirty-version/prerequisite/grant rules and verify the
   real compatible Identity adapter against the feature schema. Its HTTP/event
   contract work does not wait for a running T-022 stack. Schema QA may use an
   explicitly labelled protocol fixture until OM-RUNNER; actual runner proof
   remains mandatory before local runtime activation.
2. T-060 gains OM-IDENTITY explicitly; T-061 inherits it through T-060. They use
   the provided explicit profile/UOW and may not edit the UOW or migration SQL.
   Security production-policy gates remain separate and unchanged.
3. T-022 gains OM-RUNNER and OM-CUSTODY; retains its exact existing ownership.
   `tools/migrate.sh` invokes the new runner and orchestrates explicit shared
   templates under the agreed installation lock. It cannot use generic `up`
   directory discovery to bypass bundle checks. `readiness_test.go` verifies
   distinct owner/shared profiles; Compose does not auto-run repair/cutover.
4. T-580 gains OM-IDENTITY, OM-CUSTODY and OM-RUNNER and consumes trusted explicit release
   profiles in its already owned composition leaves. A feature profile requires
   its actual schema/manifest task before activation; an empty skeleton must
   not manufacture an auth manifest to satisfy this requirement.
5. Inventory/Commerce and the four remaining owners **do not gain compatibility
   silently**. Before assigning affected feature persistence/root activation,
   create one owner adapter follow-up apiece (same two UOW leaves as OM-IDENTITY,
   substituted exact owner path) depending on OM-PROFILE and its scaffold; add it
   to that owner's first affected schema task/root. T-581/T-582 currently remain
   legacy-only and blocked from custody activation until those explicit leaves
   and OM-CUSTODY are assigned/integrated. This proposal reserves no such leaves
   concurrently and claims no completed seven-owner rollout.

The next two follow-ups' exact leaves, to be separately assigned before work,
are `services/inventory/adapter/postgres/unit_of_work.go` and
`services/inventory/adapter/postgres/unit_of_work_test.go` after T-025/OM-PROFILE;
and `services/commerce/adapter/postgres/unit_of_work.go` and
`services/commerce/adapter/postgres/unit_of_work_test.go` after T-026/OM-PROFILE.
Their corresponding roots must also gain OM-CUSTODY/OM-RUNNER before compatible
runtime activation. This is a recorded remaining handoff, not permission to edit
those leaves or declare those root tasks ready now.

For later schema tasks, coordinator assigns each adjacent manifest and the
feature-marker requirements before readiness. No concurrent schema task edits a
shared release manifest or another feature's manifest. A root's sole owner
assembles a reviewed complete release profile from those immutable artifacts.
Independent fixture-backed domain work remains possible without claiming a
running service. This handoff changes no other existing task ownership/status.

## P6. Meaningful acceptance probes

All database probes use owned disposable pinned PostgreSQL fixtures and actual
runtime/migration roles. No existing store, Gaze data, real credentials or broker
is needed for this contract. Independent QA must review the exact task commit.

1. Preserve original v1 constructor behavior, all current excess/required grant
   regressions and foreign-owner denial. Explicit profile: valid `[1,12]` passes
   Check/Run, default constructor rejects 12, all failures precede factory and
   callback. Mutating a supplied manifest after construction changes nothing.
2. Reject wrong clean head, multiple/dirty ledger rows, missing/extra history,
   changed SQL hash, missing/wrong feature marker, reordered/broken predecessor,
   wrong profile/baseline identity, and cross-owner database/runtime/markers.
   Demonstrate an empty SELECT-only database transaction can run Check; capture
   unchanged metadata/business rows before/after both successful and failed checks.
3. History template on clean existing v1 requires explicit provenance; later head,
   missing attestation, invalid ref/hash, duplicate install, broad defaults and
   lock contention fail atomically. Baseline seed does not fabricate earlier
   feature receipts. Immutable-row modification fails for runtime and ordinary
   migration DML. Required marker/history SELECT loss also rejects readiness.
4. Real runner fresh history+1→12, then populated 12→17 preserves event/receipt/
   feature rows and prior hashes. Same bundle is a verified no-op; changed bytes,
   dropped prior file, `[1,17]`→`[1,12,17]`, duplicate versions and path escape fail
   **before** dirty writes. Run two concurrent installers; assert only one
   installation/receipt, bounded loser, no interleaved ledger cleanup.
5. Inject failure before DDL, during DDL, after explicit SQL COMMIT and before
   receipt+clean; restart the real runner. Dirty states remain blocked without
   reapplying or forcing clean. Fault the final commit reply after actual commit
   and actual rollback; exact receipt/head reconciliation distinguishes them.
   No post-baseline clean head may be observable without its matching artifact
   receipt; test the separate explicit version-1 bootstrap exception above.
6. Test direct, PUBLIC, column, default, delegated and multi-hop NOINHERIT excess
   grants; metadata writes/REFERENCES/MAINTAIN, schema/database CREATE, owner
   membership, missing required feature grant and foreign-schema reachability.
   A synthetically approved mutable feature column works while adjacent immutable
   and unrelated feature columns remain unwritable. The fixture is not auth policy.
7. Both independent version axes: owner12 with missing route/storage marker is
   unready for custody; full shared lineage with owner1 cannot satisfy an auth12
   profile. Legacy mode never selects custody implicitly; custody rejects revived
   legacy writes, wrong marker hashes and unavailable required custody grants.
   Verify valid custody no longer demands legacy outbox INSERT. No test claims
   business delivery/ACK from metadata checks; owner root QA proves composition.

## Decisions and limits for coordinator review

P1–P6 require technical adoption and exact-commit proposal QA. In particular,
approve the separate owner-local history template, explicit existing-v1 baseline
attestation boundary, actual driver finalization hook and bounded task ownership.
No installed artifact hash in this draft is a placeholder approval to execute SQL.
The actual new template/manifests get their reviewed hashes after implementation.

Unresolved: evidence for any real existing baseline; dirty/higher-version history
adoption/disposition; later-owner follow-up assignments and release-specific
feature/privilege profiles. Until separately supplied, only affected compatibility
or activation is unavailable. Source admission, receiver bootstrap, sealed-data
policy, process catch-up and real guard-writer participation keep their existing
owners/gates. T-002, business authorization, financial arithmetic/payment rules,
legal obligations and all-factor-loss recovery are outside this proposal.
