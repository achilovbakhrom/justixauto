# Owner-local durable membership versions and cutover evidence

2026-09-15. **PROPOSED; independent exact-commit proposal QA and coordinator
canonical promotion required before implementation.** Base
`d59faa8dc5b26e13e19c47686dd47744b6d73a11`. No task numbers reserved.

## 1. Confirmed contract, observation and proposal boundary

[Messaging §6](../contracts/messaging-delivery.md) already requires a complete,
role-independent approved membership version, source-authorized bootstrap and
atomic backlog enrollment. [Registration](../contracts/runtime-registration.md)
is approved despite its historical proposal header; its owner-local catalog
interfaces are not implemented. [Storage §§3,6–8](../contracts/projection-storage-handoff.md)
requires the actual ReadCommitted transaction, complete predeclared fences and
immutable retained consumer identities. [Owner migration P1–P6](../contracts/owner-migration-compatibility.md)
requires exact trusted artifacts, independent private history and narrow grants.

The result-only T-920 commit `0cbc724f3171786f78aa922f179d8cfe0b75fead`
documents a real schema counterexample: approved V1 and V2 can produce identical
admissions/bootstrap/checkpoints on a source-proved empty stream, with no
dispatch row to retain either version or the complete transition request.
000002's `messaging_cutovers` belongs to legacy-to-custody migration. Its
dispatch `membership_version` strings cannot prove current owner membership.
000004–000005 do not fill that gap. 000006 is approved generation storage and
the exact retained-label correction, not a membership registry.

This proposal adds owner-local infrastructure evidence only. Each of the seven
owners has its own catalog head in its own PostgreSQL database; there is no
cross-owner mutable table, distributed transaction or global sequence. Gaze's
explicit Go/CQRS composition is the reference shape, not evidence that it has
this protocol or its delivery guarantees. No HTML fixture supplies source
authority, retention policy, financial policy or process catch-up approval.

## 2. Identities and deterministic manifests

Distinguish five values; never overload an existing opaque reference:

1. **Logical meaning**: immutable `(local owner, consumer name)` identifies one
   owning feature, projection/process kind, generation label, subscription
   contract ID/version/digest, source/selector contract and schema semantics.
   Names cannot be reused with changed meaning. Queue/deployment placement is
   separately approved binding metadata; moving it is not permission to change
   effects or reset a checkpoint. A changed meaning requires a new name or a
   separately reviewed migration, which this proposal does not implement.
2. **Catalog**: an approved nonempty exact version label plus a complete canonical
   manifest of both inert projection/process catalogs, independent of which
   executor is currently running. Include owning feature IDs, dependency IDs,
   every consumer meaning, resolved exchange/queue, exact routing/schema sets,
   subscription selector ID/version/digest and approved binding identity. The
   manifest also identifies the exact required installed feature descriptors.
   Root namespace checks still inspect all three T-746 registries. No HTTP
   handlers, function pointers, credentials, source payloads or executable liveness
   go into the catalog. Same version with different bytes is a conflict forever.
3. **Owner selection**: the approved complete active/draining/held consumer set
   for that catalog, with explicit per-consumer disposition references. It is
   independent of the selected process role. A process binds only its selected
   role, but intake must possess all required inert claims. An explicit empty
   set needs its approved release/selection authority, never a default.
4. **Stream selection**: for one exact S=(source owner, aggregate type, UUID),
   enumerate the full applicable consumer set, effective local admission roots
   and tips, bootstrap IDs, exact ranges, kinds and generation labels. Include
   explicit exclusions/zero-applicable selection authority. The complete owner
   catalog is not a grant for every stream. A stream's source/scope/purpose and
   selector coverage evidence are distinct from its high-water/snapshot proof.
5. **Transition**: one stable UUID request, exact canonical request bytes/digest,
   expected owner-head epoch/request/catalog, resulting catalog and owner
   selection, complete stream universe and full per-stream selections/effects.
   A version-only or zero-new-consumer transition still creates a receipt and
   increments the owner epoch. Catalog versions need not sort or increase;
   epochs are positive signed-bigint CAS counters, never source positions.

The new manifest format is explicitly `membership-v1`. Implement one codec in
MS-IDENTITY: UTF-8 JSON with fixed struct field order, no insignificant whitespace,
no optional omission (explicit null for absence), decimal integer tokens and
standard Go `encoding/json` string escaping with HTML escaping disabled. Set-like
arrays sort by the bytewise UTF-8 tuple of their declared identity components;
schema keys sort by event type then numeric version. Reject duplicate set keys,
unknown fields, invalid UTF-8/NUL/lone surrogates, duplicate decoded JSON keys,
trailing input, noncanonical UUIDs, out-of-range positions and empty required
references. Reject noncanonical bytes on decode by exact re-encode comparison;
do not normalize submitted bytes into a different identity. SHA-256 covers the
complete canonical bytes; SQL also checks `digest=sha256(bytes)`.

Version and retained generation labels are exact nonempty text, including space
and tab labels where PostgreSQL permits them; no trimming, case folding or UUID
cast. New generation UUIDs remain a separate T-928 identity. All numeric fields
are bounded integers; no float, money calculation or arbitrary user JSON exists.
Reject count/byte-length overflow before allocation or SQL; operational maximum
manifest sizes are an explicit deployment resource limit, not an inferred
business policy. Oversized transitions hold and require an approved staged
optimization; do not split an atomic activation invisibly.

T-746's current `ConsumerClaim` lacks generation and numeric contract digest
fields. The root must supply an approved typed membership supplement keyed by
the exact owner/feature/consumer identity and compare every overlapping field
to the frozen claim. No invented field derived from a queue/name, environment
version string, factory output or latest message. Missing/extra supplements
fail before Bind. MS-IDENTITY defines that adapter-neutral DTO; owner registry
implementation maps it without importing an owner's private packages.

## 3. Complete universe and permitted transitions

The universe is a **finite explicit list of currently retained/admitted receiver
streams plus newly authorized streams** for this owner, not all theoretically
possible aggregate UUIDs. The request includes an immutable universe manifest,
digest, authority/selector revision and current source verification evidence.
It must account for every retained stream known through prior membership history,
local admissions, bootstrap/checkpoints and custody rows. Closed/drained streams
remain in the history and universe with their disposition; omission is not
deletion. Source enumeration is verified outside SQL; current local completeness
and current authority are revalidated under the exclusive owner catalog fence.

A subscription selector that cannot supply an approved complete finite discovery
boundary cannot activate this initial protocol. Source discovery is a separate
authenticated owner port, not a SQL wildcard or a claim that a local scan proves
remote completeness. A source may authorize future new streams via its selector
contract, but each late stream still needs an explicit `admit-stream` request,
complete new universe and source-issued bootstrap before intake accepts it.
Intake never creates that request or assumes h=0. A newly discovered local stream
outside the prepared universe invalidates activation: abort, release all locks,
obtain complete evidence and construct a new request. Missing bytes/proof stays
disabled/recovery-held, without ACK or partial membership activation.

"Current authority" means the already approved source/admission proof and its
owner-defined local validity/revocation checks, not an invented atomic read of
another service. A local PostgreSQL fence cannot freeze remote source writes or
prove immediate cross-owner revocation. If an applicable source contract cannot
make its finite boundary/proof valid for this activation, hold that transition;
this proposal adds no distributed freshness guarantee.

Supported request actions are `initial`, `select`, `admit-stream`, `drain` and
`hold/resume`, all carrying a full resulting snapshot. For an initial request,
the owner head is absent and the affected receiver tables must have no retained
local admissions/bootstrap/checkpoints/custody/enrollments/jobs. Source-side
outbox/admissions need not be empty. Source-authorized empty receiver streams
are valid: bootstrap h=0 still requires the explicit new-empty proof.

Retained installations lacking membership history are **not eligible for initial**.
Installing the additive tables leaves them unselected and unready. An initial
retained adoption would require a separately approved manifest identifying every
historical set/meaning/authority/disposition and exact retained preimages; it
cannot infer the missing full catalog version from existing strings or rewrite
old rows. No general adoption executor or invented baseline authority is included
here. Ordinary removal drains obligations; an immediate hold never completes,
deletes or redacts a job. Business-specific disposition remains unresolved until
its owner contract explicitly permits it.

## 4. Distinct forward storage surfaces

Use separate proposed revisions 7 and 8 after exact revision 6, with no change to
000002–000006 or T-928 ownership. Numbers here identify proposed shared artifacts,
not reserved task IDs. Every new table/function is in the private `eventstore`
schema and owned by the existing migration owner.

**000007_membership_catalog.up.sql — MS-CATALOG:**

| Table | Immutable shape and constraints |
|---|---|
| `membership_consumer_meanings` | consumer name PK; canonical meaning bytes/hash; owner/feature/kind/generation and exact contract identities; no mutable columns |
| `membership_catalogs` | version label PK; format=1; complete canonical catalog bytes/hash; approval/binding manifest reference and digest; declared claim count; finite creation time |
| `membership_catalog_claims` | (catalog version, consumer name) PK; FKs to exact catalog and meaning; complete canonical claim bytes/hash and exact owning feature; deferred count/set consistency |
| `membership_catalog_compatibility` | runtime SELECT-only singleton, owner/runtime/base mode schema2, revision7/format1; exact ordered artifact hashes through7 and external backup/stopped/compatibility evidence |

Catalog creation checks full manifest-to-relational claim equality, not just
count: decoded names/meanings/claims are compared exactly. SQL verifies hashes,
FKs, nonempty identity, duplicates and deferred row counts; the typed codec and
adapter verify canonical semantic equality on write **and every retained read**.
An arbitrary runtime INSERT can therefore produce incompatible evidence but
cannot make the typed adapter consider mismatched bytes valid. This is trusted
owner runtime cooperation, not authorization against a malicious database owner.
Catalogs may be installed inertly without selection; their presence activates
nothing. Once committed, no claim may be appended to an existing catalog.

**000008_membership_selection.up.sql — MS-SELECTION:**

| Table | Immutable evidence / allowed work |
|---|---|
| `membership_transitions` | request UUID PK; resulting epoch UNIQUE; previous request UNIQUE FK (null only first epoch); exact expected/new catalog and selection identities; complete request bytes/hash, universe bytes/hash/count, action and approval references; finite creation time; unique(request,epoch) |
| `membership_stream_selections` | (request,S) PK; transition FK; complete canonical per-stream selection/effect manifest bytes/hash; source/universe evidence, finite bootstrap/backlog vector, expected old selection identity; explicit zero set |
| `membership_stream_consumers` | (request,S,consumer name) PK; exact catalog-claim FK, admission root/tip and bootstrap FK, range/disposition; typed adapter checks same S/kind/generation/contract and complete equality to retained stream manifest |
| `membership_head` | singleton PK; current request/epoch/catalog via exact composite transition FK; INSERT first epoch, then only matching previous request and epoch+1 CAS; no delete/truncate |
| `dispatch_membership_evidence` | (event ID,enrollment ID) PK/FK to existing enrollment; exact transition/stream FK, phase initial/late, canonical exact consumer-set digest; immutable proof linking the new enrollment to the admitted selection without changing old rows |
| `membership_selection_compatibility` | SELECT-only singleton: same exact owner/runtime, revision8/format1, ordered hashes through8 and external installation evidence |

The head references immutable evidence, not caller-chosen current strings.
Require deferred constraint triggers on new transition/stream/consumer/head and
dispatch-evidence rows so a transaction cannot commit an incomplete declared
set, a dangling receipt, an orphan successor not selected at commit or late
addition to an already committed transition. For creation require the transition
row to originate in this transaction using the same full-xid provenance approach
as the existing message-plan checks; no durable transaction ID is an authority
token. Check all row identities and full manifest equality again on reads.
Do not rely only on a head trigger that permits later child-row inserts.

One successful activation per owner transaction; no transient sequence of two
heads in one commit. CAS is performed after all effects are complete. Unique
epoch/prior successor plus catalog fencing ensures distinct competing versions
cannot both become the same successor. The immutable historical receipt remains
queryable after the head advances; it does not have to equal today's head.

Evidence links are written with every new initial or late enrollment and all
jobs in the same transaction. For initial, membership version equals selected
catalog and set equals all effective authorized consumers for this position.
For late, set equals only newly admitted applicable consumers, never existing
jobs; changed consumer meaning is not a new job under the old name. Zero jobs
does not require a fake enrollment/message: the transition/stream evidence is
already durable. For a valid future intake with explicitly authorized zero
applicable consumers, an actual message may have the existing approved empty
initial set; catalog absence cannot stand in for empty-set authority.

Keep original SQL artifacts, OIDs/ACLs, rows, frozen enrollment sets, legacy
cutovers and markers unchanged. New FKs necessarily add only their declared
referential triggers to existing referenced tables; record those catalog deltas
explicitly in preimage tests. Neither migration inserts a catalog/head, adopts
retained history, updates base messaging mode2 or changes private owner history.

## 5. Atomic adapter and recovery protocol

Prepare the sealed catalog, approved complete selection, universe proof, request,
all source-issued high-water/snapshot/no-snapshot/new-empty evidence and finite
original-byte recovery manifests outside SQL. The request records stable IDs for
every admission/bootstrap/enrollment, expected prior heads/tips and all effect
digests. A changed finite boundary requires a new request; never change the
payload under the same request ID after an uncertain outcome.

Within the actual caller ReadCommitted transaction:

1. Prepare the **complete** sorted T-009 source fences if any, exclusive owner
   catalog fence, sorted receiver S fences, then generation/guard fences. Use
   existing `PreparedFences.Require` before reads/effects. No late lock upgrade.
   A T-016 projection head lock, where applicable, follows these advisory locks;
   membership head CAS/row lock precedes job/inbox/checkpoint/effect row work.
2. Verify exact shared artifacts/mode/privileges and current authority. Read the
   request history first: same ID plus byte-identical request is an existing
   committed result only when found by a fresh authoritative read; within a
   transaction it is still a candidate. Changed bytes conflict. Otherwise check
   expected head, immutable catalog/meaning and complete universe/current tips.
3. Re-scan all authorized retained custody above each consumer's issued h through
   the finite selected source boundary. Prove exact contiguous original positions
   and schema/authority, accounting for concurrent arrivals now fenced out. A
   newest observed message is never that source boundary. Any retained position
   beyond the prepared boundary, changed authority or missing required bytes
   invalidates preparation and aborts. Obtain recovery outside locks; no network
   or automatic callback retry inside this transaction.
4. Write proposed immutable transition and complete stream rows; append admissions
   and install typed bootstrap/snapshot/checkpoints through the **same tx/fences**.
   Enroll all required newly admitted consumers in each retained message and
   insert all jobs/evidence links. Existing sets and obligations stay unchanged.
   Pre-recovered missing original bytes, if permitted, enter through the same
   ordinary custody validator with an explicit sealed prospective-selection
   capability bound to this transaction/request. It cannot be used by ordinary
   intake, skip original routing/schema/authority validation, ACK, or survive
   rollback. This narrow lower-level writer must not call current-head readiness
   recursively. Process catch-up requires its own explicit proof; projection
   snapshot data cannot authorize process commands.
5. Check every declared result, finite manifest/count/set, bootstrap and exact
   new jobs; CAS the owner head once. Commit remains the outer UOW's job. Any
   failure rolls back head, receipts, admissions, snapshot, checkpoints, links
   and jobs together. Only known commit permits activation; this operation has
   no AMQP ACK capability.

Ordinary intake takes shared catalog and receiver fences and checks the installed
sealed catalog/selection against the current head **for each new custody write**.
Startup success is not a cache permitting stale writes. An exact redelivery uses
its original retained bytes/set/evidence rather than recomputing today's set;
this narrow reconciliation may confirm existing custody without authorizing new
selection or execution. Stale executors stop new claims; approved draining
executors must be explicitly present as draining claims in the current catalog,
with unchanged meaning, and still satisfy current authority and job fences.

Unknown commit: do not retry callbacks, overwrite the request or infer success
from a bootstrap. Close/discard the uncertain connection and read the exact
request on a fresh authoritative connection under the owner catalog fence.
Matching complete historical receipt proves that request committed even if a
later head is current. Return both immutable committed result and separately
read current status; do not restart a stale runtime. Absent receipt while the
old transaction may still hold locks is **not proof of rollback**: wait boundedly
for its conflicting fence/transaction to resolve, then read again. A definite
rollback leaves no receipt/effect. A new execution after confirmed rollback is
an explicit caller action with fresh validation, not an automatic retry.

## 6. Go ownership, readiness and compatibility

MS-IDENTITY supplies immutable `Catalog`, `Selection`, `StreamUniverse` and
`TransitionRequest` values with defensive copies, exact decode/re-encode/digests
and zero-value rejection. It does not validate external authority by itself.
MS-STATE supplies a transaction-bound store and immutable read results, equivalent
to `NewMembershipStore(ctx, tx, PreparedFences, owner)`, `ReadCurrent`,
`ReadTransition`, `RecordCandidate` and `SelectCandidate`; exact names may vary.
No method opens a transaction, commits, ACKs, runs network I/O or takes arbitrary
SQL from a caller. Receipt/candidate types are distinct; stored proof is not a
permission capability. Read helpers accept the actual supplied GORM handle;
the outer owner supplies fresh authoritative transactions for reconciliation.

T-920 composes the protocol through narrow typed callbacks bound by the outer
adapter. An outer closure can call actual
`projection.NewCheckpoints(ctx, tx, fences, contract, bind)` and
`InstallBootstrap(ctx, verified, revalidate, install)` without an inbox import
of `pkg/projection`. Its owner-facing port exposes typed actions only; the raw
GORM tx/fences stay in outer adapter construction. No callback receives a pool,
fresh factory transaction, commit function or arbitrary consumer-controlled SQL.
The temporary compile probe verified this existing public signature composition,
not the proposed store or transaction orchestration.

MS-STATE owns the membership evidence writer needed by both T-920 and T-921,
including the prospective-selection capability boundary. Keep shared concrete
types below their callers in `pkg/inbox`; T-920/T-921 must not import each other's
future leaves or depend on a future owner root. The existing T-920 acceptance
must be explicitly revised to compose this writer and not invent its own second
catalog implementation. If that remaining orchestration exceeds its 3h bound,
stop and reslice before assignment rather than silently expand it.

T-930's inspected in-progress API already has `Shared{Mode,Artifacts}` with exact
revision/path/hash identities and defensively copied Profile. `Check(ctx, db,p)`
is SELECT-only, has no callbacks/transaction and rejects custody/beyond supported
legacy artifacts. No change to its public profile shape is needed for7/8.
T-934 must consume both new approved artifacts after T-928, check their exact
markers, named objects/constraints/trigger ownership and privileges, and reject
missing/extra lineage. A valid empty unselected store is **storage compatible**
but membership readiness is false; the checker must not require an active head
to run the transaction that first installs one.

Installer metadata is runtime SELECT-only. New immutable evidence tables grant
only SELECT/INSERT, guarded against post-commit child addition; head grants
SELECT/INSERT and UPDATE only its exact pointer columns. No table-wide UPDATE,
DELETE/TRUNCATE/TRIGGER/REFERENCES/MAINTAIN, function EXECUTE, grant option,
schema CREATE, DB CREATE/TEMP or unexpected default grants. Check PUBLIC and
every transitively reachable role including NOINHERIT/SET ROLE paths and column
grants. Trigger functions are non-security-definer with fixed pg_catalog search
path and explicit qualified tables; invocation is via table trigger only.
Migration owner retains trusted DDL power; runtime SQL grants are not business
authority. Evidence stores metadata/secure manifest references, never raw private
source snapshots or credentials. Missing approved sensitive-evidence handling
blocks affected manifests; storage grants do not supply retention permission.

Both forward installers use explicit psql transaction, bounded 5s lock wait,
the established ordered offline locks, exact prior artifact/owner/runtime checks,
captured prior OIDs/ACLs/rows and external backup/stopped/channel evidence.
Record their new marker only with all objects/grants in that transaction; later
failure rolls it all back. No destructive down, reapply, inferred newer profile
or automatic migration. T-933/T-022 trusted bundle assembly must enumerate both
artifacts explicitly, following owner P1–P6; private head/history is independent.

## 7. Bounded aliases and dependency delta

Each row is one branch/worktree and **at most4h**, with independent exact-SHA QA.
These are proposed aliases only; coordinator allocates IDs after both proposal
and canonical promotion QA. Tests embed fixtures in their assigned test leaf.

| Alias | Dependencies | Exact application leaves / responsibility |
|---|---|---|
| MS-CATALOG | T-928 | `pkg/eventstore/migrations/000007_membership_catalog.up.sql`; `pkg/eventstore/membership_catalog_migration_test.go`: immutable complete catalogs/meanings and rev7 lineage/ACLs only |
| MS-SELECTION | MS-CATALOG | `pkg/eventstore/migrations/000008_membership_selection.up.sql`; `pkg/eventstore/membership_selection_migration_test.go`: transition/stream/head/dispatch-link SQL constraints and rev8 only |
| MS-IDENTITY | T-746, T-007 | `pkg/inbox/membership_catalog.go`; `pkg/inbox/membership_catalog_test.go`: canonical value codec, claim supplement/full-versus-stream identities, immutable request/selection constructors, no SQL |
| MS-STATE | MS-SELECTION, MS-IDENTITY, T-014 | `pkg/inbox/membership_state.go`; `pkg/inbox/membership_state_test.go`: fenced storage/read/reconcile/candidate/selection and evidence-link primitives, no owner snapshot or source network orchestration |

Explicit additions: T-920 depends on MS-STATE; T-921 on MS-STATE (and existing
T-920); T-016 on MS-STATE (and existing T-920); T-022 on MS-SELECTION and
MS-STATE (retains T-934/T-933); T-934 on MS-SELECTION. T-591 remains the combined
B-01 acceptance closer and must include these additions in its closure.
T-934 remains sole serial successor of T-930's `check.go/check_test.go`; no new
alias edits them. T-920 owns existing membership.go/test; T-921 owns intake.go/test;
T-016 owns its existing generation adapter leaves. MS aliases do not claim those
files, root manifests, go.mod, dependency locks or T-928's migration/test.
Root T-580…586 assembly additionally consumes current membership readiness and
complete inert catalogs; their existing membership/owner compatibility gates
cannot be bypassed. No new root edit belongs to these four aliases.

## 8. Acceptance floor and remaining decisions

Independent implementation QA must verify actual complete forward artifacts,
not only the reduced proposal probe:

- Empty receiver/source-proved h=0 installs V1 with no dummy custody; identical
  consumer meanings but changed catalog/binding V2 remain distinguishable.
- Canonical order is stable; duplicates, omitted required claims/supplements,
  mutated caller slices, changed same-version catalog and changed same-request
  bytes fail; version-only and zero-new-consumer transitions retain receipts.
- Competing initial/upgrade requests yield one exact successor; wrong expected
  head and post-commit child inserts fail; source/later stream discovery outside
  the complete universe aborts with no partial snapshot/admission/enrollment.
- Exact request after later head advance reconciles committed historical result
  while stale readiness remains false. Real commit-lost-reply injection, eventual
  rollback versus commit and bounded fence waiting prevent false absent=rollback.
- Full authorized finite backlog including original recovered bytes is atomic;
  missing/denied bytes, raced boundary, bootstrap/snapshot/job or final CAS failure
  roll back everything. Old rows/sets/requests/jobs remain byte-identical.
- No old-consumer duplicate jobs; drain/hold preserves unfinished jobs; no process
  replay under projection authority; no callbacks on stale/invalid transaction,
  omitted fence, incompatible lineage or mismatched retained manifest.
- Ordinary intake rechecks current head inside fences; duplicate old custody
  validates original evidence; complete claim set works while an executor is down;
  required missing claim prevents readiness before Bind/consume.
- Exact old SQL/preimages preserved except declared new-FK catalog attachments;
  required and forbidden direct/column/default/PUBLIC/reachable-role grants,
  wrong owner/runtime/hash/marker/constraint and lock/later transaction failure.
- Forward installation on retained ambiguous state succeeds only as inert storage;
  initial selection/adoption remains denied without separately approved history.

Open authority is scoped: real source enumeration/snapshot/catch-up contracts,
sensitive manifest policy, retained-installation adoption and exceptional
revocation disposition still need their owning approvals. No financial rule or
new event delivery promise is invented. Custody ACK still means durable original
bytes and all admitted jobs, not completed effects. Safe finite activation is
promised, not bounded catch-up under an indefinitely changing source.
