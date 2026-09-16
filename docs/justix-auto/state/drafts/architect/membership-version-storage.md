# Owner-local durable membership versions and cutover evidence

2026-09-15. **PROPOSED; independent exact-commit proposal QA and coordinator
canonical promotion required before implementation.** Base
`d59faa8dc5b26e13e19c47686dd47744b6d73a11`. No task numbers reserved.
Final fix cycle2 (2026-09-16) preserves the lifecycle correction from
`c3c0da45965c980fee22a32f43e90d7a674a55c4` and addresses its r2 constraint-timing
BOUNCE. Both that review and the original BOUNCE at
`5996c77e3df371a3992154b639ce9ba91ffc3e75` remain evidence, not approval.
A further BOUNCE requires bounded scope review, not a third automatic fix.

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
| `membership_head` | singleton PK; current request/epoch/catalog via exact composite transition FK; INSERT first epoch, then only matching previous request and epoch+1 CAS; server-overwritten `last_selected_xid xid8` bookkeeping on every selection; no delete/truncate |
| `dispatch_membership_evidence` | (event ID,enrollment ID) PK/FK to existing enrollment; exact transition/epoch/catalog/stream FK, phase initial/late, selection mode current/prospective, canonical exact consumer-set bytes/hash, server-created full transaction ID; separately append-only enrollment proof, not a frozen transition child |
| `membership_selection_compatibility` | SELECT-only singleton: same exact owner/runtime, revision8/format1, ordered hashes through8 and external installation evidence |

The head references immutable evidence, not caller-chosen current strings.
There are **two different creation lifecycles**:

- Transition, stream-selection and stream-consumer rows form one frozen snapshot.
  Immediate origin guards require the transition's server-recorded full `xid8`
  to equal `pg_current_xact_id()` when adding its snapshot children, and reject
  additions once that transition has been selected, even in its creation
  transaction. Deferred completeness checks require full declared set equality
  and selection of the exact successor; they may fail early if forced before
  the snapshot/CAS is complete. Reject orphan successors and incomplete snapshots.
  Server BEFORE INSERT guards **overwrite** transition/evidence creation IDs
  with `pg_current_xact_id()`; they are not caller-preserved defaults. Runtime
  grants exclude supplying/updating those columns. These IDs are transactional
  bookkeeping, never authorization or evidence of source approval.
- Dispatch evidence is independently append-only. A later new enrollment may
  link to an older already committed selection. **It does not require that the
  referenced transition was created in this transaction.** It requires that
  the enrollment, its exact jobs and its one evidence row are all created in
  this transaction. Transition-snapshot immutability does not freeze the future
  set of enrollment links that refer to it.

MS-SELECTION explicitly owns a new additive, deferred `AFTER INSERT` constraint
trigger named `membership_enrollment_complete` on the existing
`eventstore.dispatch_enrollments` table, plus guards on the new evidence table.
It does not replace, disable or change any old trigger/function. A trigger only
on evidence insertion cannot detect omitted evidence; this enrollment trigger
must fire even when **no link was inserted**. Its function performs qualified
SELECTs only, remains non-security-definer with fixed pg_catalog search path,
and is not granted runtime EXECUTE. T-934 checks its exact validated installation,
enabled constraint-trigger shape, timing and function identity as a declared
revision8 object. The new trigger does not scan or fire for retained enrollment
rows, read-only redelivery, or inert installation with no active head.

At the deferred enrollment and evidence checks require exactly one link for the
new `(event,enrollment)` and the link's server `created_xid=pg_current_xact_id()`.
The evidence-insert guard also verifies that the referenced immutable enrollment
tuple's `xmin=pg_current_xact_id()::xid`, the same immediate provenance predicate
observed in T-919; an existing committed enrollment cannot receive a new link as
a repair. Record full xid8 only on the new evidence/transition tables; add no
column to old tables and do not describe old xmin as a stored full xid8.
Runtime INSERT grants exclude these server provenance columns; server guards
overwrite supplied values even in privileged synthetic SQL. Owner DDL remains
trusted. The prepared Go capability additionally
binds actual full transaction ID/backend/fence nonce and the insertion candidate.
No SQL transaction ID or user-supplied `selection_mode` supplies authority.
The writer creates no nested transaction or savepoint. A caller subtransaction
that cannot meet the existing top-level origin predicate fails closed; this
handoff does not add savepoint-write support to T-919-style provenance checks.

SQL enforces link/enrollment/message S identity, exact referenced transition,
epoch/catalog, initial-versus-late identity against the message's initial
enrollment ID, matching enrollment membership version, canonical set hash,
duplicate-free relational set equality and actual jobs. Compare complete
`(consumer_name,admission_id)` sets, not counts or hashes alone; existing
`messaging_complete` and `messaging_job_guard` remain active. Additional link
checks match kind/generation/contract and ranges through admission/bootstrap
and stream-consumer rows. The link's finite set is exactly the effective stream
selection for the event position in initial phase, or exactly the newly admitted
applicable delta declared in this transition's finite backlog manifest for late
phase. Initial enrollment names can never be used for a late append. Future
ordinary intake uses initial/current; a late enrollment requires prospective
selection and an explicitly declared activation backlog entry.

**Timing-independent guard coverage.** PostgreSQL deferred checks can run early
through `SET CONSTRAINTS ALL` or named constraints. They are not commit hooks.
MS-SELECTION must install the immediate guards below as ordinary enabled triggers,
whose timing cannot be changed by `SET CONSTRAINTS`; deferred checks remain an
additional completeness layer. This is an explicit SQL guarantee, not merely
a cooperative Go ordering rule.

| Relevant mutation | Immediate SQL obligation |
|---|---|
| Transition INSERT | Overwrite server creation xid8; validate immutable request/predecessor identity. Defer only pending selection/completeness, not provenance. |
| Snapshot stream/consumer INSERT | Require current-transaction transition and not-yet-selected snapshot; validate declared membership/identity. No post-selection or post-commit children. UPDATE/DELETE/TRUNCATE remain denied. |
| Evidence-link INSERT | Require the new enrollment and all exact jobs already present in this transaction; validate full immutable content/set. Current mode requires the exact already-committed current head, locked FOR SHARE until transaction end. Prospective mode requires the new transition's expected prior head, locked FOR UPDATE (or absent head for initial), with that transition not yet selected. It cannot link after selection. |
| Membership head INSERT/UPDATE | Enforce exact first/CAS identity and complete proposed snapshot/effects; inspect **every new link in this transaction** and reject any request/epoch/catalog differing from NEW. Inspect newly inserted enrollments for missing/different links. Reject a second head selection in this transaction using OLD server bookkeeping, then overwrite NEW bookkeeping. |
| Dispatch job INSERT | Require a newly inserted enrollment in this transaction and **no evidence link yet**. The evidence link seals its complete job set. Reject subsequent job INSERT, including after constraint flushing; old job work-column UPDATE retains existing guards and is not a membership mutation. |

The immediate link guard validates content after the row is available (an AFTER
INSERT ordinary trigger is suitable); its creation stamp runs before validation.
It does **not** demand the prospective target already be the head. The sequence
is message/enrollment, every exact job, complete evidence link, then prospective
head CAS last. A link before all its jobs fails immediately. A head before its
declared enrollment/link effects fails immediately. With a legitimate zero-delta
selection and no new enrollment, head CAS is valid, but subsequent new current
links to the former head or prospective links to the now-selected target fail.
An unrelated enrollment inserted after selection still cannot commit without
a valid link; its INSERT completeness check runs even if earlier checks were
flushed. There is no alternate writer that inserts jobs after sealing.

The new explicit attachment `membership_job_set_guard` is a BEFORE INSERT
ordinary trigger on existing `eventstore.dispatch_jobs`, owned solely by
MS-SELECTION's SQL008/test leaves. It neither replaces `messaging_job_guard` nor
runs on existing job updates/reads. The previously declared enrollment INSERT
constraint trigger remains. These are the **two** new custom trigger attachments
on old tables, in addition to new-FK referential attachments; all other immediate
guards are on new membership tables. Exact preimages must distinguish these
additions from unchanged old trigger/function definitions and privileges.

`membership_head.last_selected_xid` is written on every successful first/CAS
selection from the server's full xid8, never from a caller value, session GUC,
constraint setting or cached count. An UPDATE first checks OLD's value: if it
equals the current full xid, reject even when NEW supplies another value or
the preceding constraints were flushed. Runtime UPDATE grants exclude it.
A rolled-back savepoint restores both pointer and marker; replacing a selection
that was fully rolled back is one retained change. Rolling back only a failed
second attempt cannot erase the first selection's marker. New evidence's
server-created xid selects exactly this transaction's links for the reverse
head check; historical committed links are intentionally excluded.

This closes both temporal orders: link then head is checked again by the head
mutation even after early link checks; head then stale link is rejected by the
link mutation. Link sealing plus immediate job-insert rejection prevents a later
job from invalidating a previously checked set. Snapshot child guards likewise
prevent later changes after selection. Missing/orphan records still enqueue
their own deferred checks on creation; flushing an incomplete state fails
closed, and a new insert schedules a new check. `SET CONSTRAINTS` timing cannot
disable any of the immediate checks or authorize a second selection.

Retain the head row locks through transaction end: merely reading the current
head at immediate-check time is insufficient when another transaction can
advance it after an early constraint flush. Current link insertion takes
`FOR SHARE`, which conflicts with a concurrent pointer UPDATE; prospective
insertion takes `FOR UPDATE` on its prior head, and revalidates the exact expected
request/epoch/catalog after acquiring it. For first selection with no head row,
the immutable first-transition identity, unique singleton head INSERT and
deferred orphan checks reject a competing initial commit. The trusted adapter
acquires the same head lock when minting CurrentSelection/ProspectiveSelection,
**before** job/effect row work in the approved fence order; the SQL link guard
independently reacquires/verifies it so correctness does not rely on callers
avoiding `SET CONSTRAINTS`. Uncoordinated SQL can deadlock or time out and abort;
there is no global deadlock-free or unbounded-wait guarantee. A later separate
transaction may advance the head after the current-link transaction commits,
leaving that valid historical link unchanged.

At commit the new link must reference the selected head's exact request/epoch/
catalog. In current mode the transition must predate this transaction; in
prospective mode it must originate here and this transaction must select it by
its required CAS. A current-mode insert followed by a different selection in
the same transaction fails; choose the prospective path before writing anything.
Foreign stream, stale head, absent head, mismatched/extra/omitted link, set or job,
and prospective link without final selection all reject the complete transaction.
Hash/JSON shape, relational content and local selected-head checks belong to SQL;
canonical codec semantic equality, approved full claims, source authority,
complete universe/backlog, original envelope validation and correctly prepared
fences also remain mandatory typed-adapter checks. SQL shape success alone
cannot authorize a consumer. Validate all these again on retained reads.

One successful activation per owner transaction; no transient sequence of two
heads in one commit, enforced by the immediate OLD-bookkeeping check as well as
immutable successor/CAS constraints. CAS is performed after all effects are complete. Unique
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
cutovers and markers unchanged. The only permitted additions on existing table
catalogs are the declared new-FK referential attachments and the new enrollment
INSERT constraint trigger plus the new job INSERT ordinary trigger above;
snapshot their exact function/trigger definitions
and demonstrate every old function/trigger is byte-identical. No old column,
constraint or grant changes. Record these explicit deltas in preimage tests.
Neither migration inserts a catalog/head, adopts
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
   membership head FOR UPDATE lock precedes job/inbox/checkpoint/effect row work.
   Ordinary current intake uses the same order with its head FOR SHARE lock.
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
   insert every enrollment's exact jobs before its sealing evidence link.
   Existing sets and obligations stay unchanged.
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
It obtains an unexported-state **CurrentSelection** capability bound to the
actual transaction/backend, prepared catalog/receiver fences, current request/
epoch/catalog, exact S, validated event position/hash and complete selected set.
The store's evidence writer revalidates that capability and the newly inserted
enrollment/jobs before returning a custody candidate. T-921 cannot substitute a
caller version string, a raw link row or an unchecked INSERT result.
**ProspectiveSelection** is a distinct sealed capability minted by MS-STATE only
after validating a recorded transition candidate, expected prior head, exclusive
catalog/stream fences and its exact finite bootstrap/backlog effects. It binds
the same transaction plus candidate identity and permits only those declared
initial-recovery or late-delta enrollments. T-920 supplies it to the lower-level
writer; ordinary intake cannot construct/use it or recurse through current-head
readiness. Final SQL selection/completeness checks must still pass. Reuse on a
different transaction, ended/rolled-back savepoint, changed event/set or stale
fences fails before write/callback; there is no permissive common interface that
accepts arbitrary caller-supplied capability implementations.

Startup success is not a cache permitting stale writes. An exact redelivery uses
its original retained bytes/set/evidence rather than recomputing today's set.
It performs no new enrollment, job or link INSERT and does not call the
new-insert current-head/provenance predicate on historical evidence. Validate
the original immutable transition/stream/claim reference and exact original
link/enrollment/jobs; a later admitted head is allowed on this read path.
Check the retained lifecycle relation itself:
current links have a creation xid different from their referenced transition's,
whereas prospective links have the same recorded creation xid. Neither must
equal the fresh reader's xid, and neither must point at the current head. Missing
or conflicting old evidence is a hold, never backfilled, repaired, re-enrolled
or ACKed as complete. This narrow reconciliation may confirm existing custody
without authorizing new selection or execution. Stale executors stop new claims;
approved draining
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

MS-STATE owns the membership evidence writer and typed `VerifyEnrollment`/read
checks needed by both T-920 and T-921, including both distinct sealed selection
capabilities. The actual T-921 writer must call them around its message/enrollment/
job writes before returning its candidate, and known outer commit remains the
only ACK boundary. Even if a caller omits VerifyEnrollment, SQL's new enrollment
trigger rejects a missing link at commit; this does not replace the adapter's
authority/fence checks. T-920 must verify every prospective backlog result before
head selection. Keep shared concrete
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

T-934 additionally verifies every required immediate guard is an ordinary enabled
trigger, every required deferred guard has its declared constraint timing, the
server bookkeeping columns/functions are exact, and no privilege/default can
disable them. A marker or a passing prior T-928 check does not substitute for
the actual revision7/8 guard objects. This design's reduced tests are independent
of T-928's implementation/review and make no claim about that task's QA.

Installer metadata is runtime SELECT-only. New immutable evidence tables grant
only SELECT/INSERT on the exact content columns (not server provenance columns).
Frozen snapshot-child guards and separately append-only enrollment-link guards
enforce their respective lifecycles; head grants
SELECT/INSERT and UPDATE only its exact pointer columns. No table-wide UPDATE,
DELETE/TRUNCATE/TRIGGER/REFERENCES/MAINTAIN, function EXECUTE, grant option,
schema CREATE, DB CREATE/TEMP or unexpected default grants. Check PUBLIC and
every transitively reachable role including NOINHERIT/SET ROLE paths and column
grants. Trigger functions are non-security-definer with fixed pg_catalog search
path and explicit qualified tables; invocation is via table trigger only.
Installation and runtime compatibility checks require
`current_setting('session_replication_role')='origin'`; deny effective SET or
ALTER SYSTEM authority on that parameter for the runtime, PUBLIC and every
transitively reachable role, including NOINHERIT roles reachable through SET
ROLE. Inspect parameter ACLs/effective privileges and all applicable database,
role and database-role defaults for non-origin replication mode. A role able
to switch to replica could bypass ordinary triggers/FKs and is incompatible
even when its current session is origin. Existing owner/superuser/DDL/TRIGGER
denials also remain. Ordinary `SET CONSTRAINTS` is deliberately permitted: the
SQL invariants must survive its ALL and named forms, not rely on denying it.

An installer's origin session and role/database-default audit do **not** prove
the runtime's effective login mode: an installer override can hide a server-wide
or command-line default. Effective startup verification must open a fresh
connection as the exact runtime login into the exact owner database using the
approved connection configuration, without a session-level replication-mode
override, and verify actual session/current identity, origin mode, parameter
authority and applicable defaults there. T-022/T-933's outer installation/startup
handoff owns that verification; SQL cannot certify it from an opaque evidence
reference or the installer's own `SHOW`. Missing access to that exact effective
configuration is an unverifiable compatibility result: do not authorize runtime
activation or label the installation startup-compatible. Inert DDL/marker presence
alone remains no such claim. T-934 also checks the actual supplied runtime
handle's mode before factory binding on every compatible UOW Run, so a new
connection with unsafe defaults fails closed. No secret or server configuration
file is read by this proposal, and no fallback to an installer/admin connection
may stand in for the runtime check.

Revision7/8 preflight also requires the forthcoming T-928 prerequisite: the
exact runtime role is a LOGIN role, and `pg_db_role_setting` contains exactly
the explicit `session_replication_role=origin` setting for **that runtime role
OID and that owner database OID**. Unrelated, inherited, installer-only,
role-global or database-global settings are not equivalent. Reject absence,
conflicting entries or unverifiable identity. Approved external provisioning
establishes this role-in-database setting before the stopped/quiesced install;
these migrations and the runtime check inspect it read-only and never repair
it or grant the runtime parameter-setting authority. Fresh direct-runtime-login
sessions must then pass the actual-handle verification above; a pooled installer
session or SET ROLE substitution is not that proof. T-934 repeats this exact
setting/identity prerequisite alongside the actual runtime session check.

This aligns the dependent proposal with T-928's final proposed configuration
contract, **pending T-928 independent approval**. MS-CATALOG remains gated on
the actual approved/integrated T-928 artifact and contract; neither its hash nor
its runtime guarantees may be inferred from an in-progress worktree.
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
| MS-SELECTION | MS-CATALOG | `pkg/eventstore/migrations/000008_membership_selection.up.sql`; `pkg/eventstore/membership_selection_migration_test.go`: transition/stream/head/dispatch-link constraints, immediate reverse/head-lock/job-set guards, the two explicitly added old-table INSERT triggers and rev8 only |
| MS-IDENTITY | T-746, T-007 | `pkg/inbox/membership_catalog.go`; `pkg/inbox/membership_catalog_test.go`: canonical value codec, claim supplement/full-versus-stream identities, immutable request/selection constructors, no SQL |
| MS-STATE | MS-SELECTION, MS-IDENTITY, T-014 | `pkg/inbox/membership_state.go`; `pkg/inbox/membership_state_test.go`: fenced storage/read/reconcile/candidate, sealed current/prospective selection, complete enrollment verification and evidence-link primitives, no owner snapshot or source network orchestration |

Explicit additions: T-920 depends on MS-STATE; T-921 on MS-STATE (and existing
T-920); T-016 on MS-STATE (and existing T-920); T-022 on MS-SELECTION and
MS-STATE (retains T-934/T-933); T-934 on MS-SELECTION. T-591 remains the combined
B-01 acceptance closer and must include these additions in its closure.
T-934 remains sole serial successor of T-930's `check.go/check_test.go`; no new
alias edits them. T-920 owns existing membership.go/test; T-921 owns intake.go/test;
T-016 owns its existing generation adapter leaves. MS aliases do not claim those
files, root manifests, go.mod, dependency locks or T-928's migration/test.
Final fix cycle2 retains four aliases/eight disjoint leaves and the same dependency
delta. Immediate reverse checks, head bookkeeping, head locks and the added job
INSERT trigger complete MS-SELECTION's existing SQL invariant responsibility;
the two capability lifecycles and ordered head locking belong to MS-STATE.
Parameter/default audits belong to these installers and T-934's serial checker.
No fifth task or earlier-migration source ownership is hidden here. The reduced
lifecycle/timing probes support this boundary, not a time
guarantee; if either full migration/adapter cannot fit its4h bound, coordinator
must reslice before dependent assignment. T-920 retains its separate3h limit.
Root T-580…586 assembly additionally consumes current membership readiness and
complete inert catalogs; their existing membership/owner compatibility gates
cannot be bypassed. No new root edit belongs to these four aliases.

## 8. Acceptance floor and remaining decisions

Independent implementation QA must verify actual complete forward artifacts,
not only the reduced proposal probe:

- Empty receiver/source-proved h=0 installs V1 with no dummy custody; identical
  consumer meanings but changed catalog/binding V2 remain distinguishable.
- After empty R1 commits, future ordinary intake creates a new initial enrollment,
  exact jobs and one link to **committed R1**, with no new transition. A later
  R2 permits read-only exact R1 redelivery, while a new R1 intake is stale and
  rolls back. No active head is required merely to install the new trigger.
- Late transition/stream/consumer mutation, link insertion for an old enrollment,
  duplicate/omitted/mismatched link, wrong/extra/missing consumer/job, wrong phase,
  prospective capability reused as current or without head CAS, stale/foreign
  transaction capability and fabricated provenance all reject. Missing-link
  failure is demonstrated by the **enrollment** trigger when no link trigger ran.
- Flush ALL and named enrollment/link constraints immediate then deferred in
  both mutation orders: current link then sole valid next-head CAS, and head CAS
  then stale link. Both must reject regardless of timing. Prospective pending
  creation remains valid in its declared order; forcing incomplete prospective
  checks early fails closed. Append a job or snapshot child after checks/selection
  and reject immediately. One valid head change works; a second fails even with
  supplied bookkeeping and constraint toggles. Savepoint rollback of the whole
  first selection permits one replacement; rollback of a failed second attempt
  cannot clear the retained first marker. Historical later transactions work.
- After a current link's checks are flushed, a concurrent head UPDATE must still
  wait or time out on its head row lock. On current-link commit, a later separate
  transaction can advance normally; failed lock/commit outcomes never fabricate
  a selected version. Exercise the exact adapter lock order and SQL guard locks.
- Deny direct/PUBLIC/reachable parameter SET/ALTER SYSTEM authority and unsafe
  role/database defaults capable of disabling guards, including NOINHERIT/SET
  ROLE paths. Runtime bookkeeping-column writes fail; privileged synthetic input
  is overwritten by server guards, not preserved as a trusted receipt.
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
- Exact old SQL/preimages preserved except declared new-FK catalog attachments
  and the new enrollment/job INSERT triggers; old definitions unchanged;
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
