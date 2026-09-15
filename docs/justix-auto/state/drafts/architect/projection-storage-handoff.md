# Projection, gap and quarantine storage handoff

Date: 2026-09-15. **Proposal for coordinator review, not an approved schema or
runtime guarantee.** Assigned documentation worktree base
`2c1ff1de53f4dc7b0588d63d6b34de1d0983f0b3`. No application changes, migrations,
policy activation or canonical task transitions are performed by this proposal.

## 1. Authority and observed gap

Confirmed requirements are [architecture §5](../../../architecture.md),
[messaging delivery §§3/6/9](../contracts/messaging-delivery.md), and existing
T-014/015/016/920 ownership. They require independent contiguous checkpoints,
authorized bootstrap, durable gaps, quarantine before acknowledgment,
evidence-preserving redrive and historical generation rebuild/switch. These
strengthen the reference: [Gaze](../../../reference/gaze-reference.md) shows
no durable deduplication/replay cursor or safe generation swap; the
[CC audit](../../../reference/gaze-executor-cc-reference.md) records distinct
snapshot code but no proof of these guarantees. HTML mocks supply no durable
storage, privacy authority, process catch-up or production financial rule.

Observed repository facts at the assigned base:

- T-008 owns immutable events/inbox/receipts and operation mechanics. T-917's
  `000002` adds admissions, custody, enrollment and jobs; `quarantine_ref` is a
  nullable text work reference. Neither supplies checkpoint, gap, quarantine or
  generation storage. T-014/015/016 own only their named Go leaves.
- T-922 exact `e15f5e7a677084ad84c6952d53c53eddfb6d0b10` supplies caller-bound
  custody `Apply`, candidate outcomes and ReadCommitted/mode guards. Its
  synthetic checkpoint/job fixtures are transaction evidence, not these stores.
- T-925 exact `4dbe2593bce71c045f785fc07aed9da5b4113a73` supplies forward route correction and an immutable
  revision-3 marker. It does not supply projection storage. Its unchanged full
  route is `source.target.fullOwnerQualifiedEventType`.

The missing migration ownership is a concrete implementation gap. It does not
reopen the seven owners, ADR-03, existing release approval or the 925-task plan.

## 2. Decision proposed: shared mechanics, privately installed per owner

Use reusable SQL and outer Go adapters in `pkg/eventstore`, `pkg/projection` and
`pkg/inbox`, installed independently in each owner's existing private database.
The schema `eventstore` remains owned by that database's migration role; runtime
credentials never access another owner database. No mutable business table is
shared between services. Owner-specific read rows, synchronous guards, secure
records, authorization and invariant comparisons remain owner-owned.

Owner-provided abstract storage alone would permit T-014/015/016 unit tests, but
would leave seven unassigned durable implementations and no common migration
evidence. A global projection service/shared database would violate ownership.
The recommended middle ground implements mechanical durability once and maps it
to **owner-declared typed ports** in each outer adapter. Service domain/app/port
packages must not import GORM, `pkg/projection` or `pkg/inbox`. No generic SQL,
`any` repository or fresh pool may escape into an application callback.

The following names/columns are a proposed storage contract. All IDs are
canonical nonzero UUIDs; positions/epochs are checked signed-bigint nonnegative
values (positive where specified), never floating point. Hashes are exactly
32 bytes; timestamps are finite. Database ownership is implicit in every key;
`source_owner` is still explicit and restricted to the seven approved names.
An opaque evidence reference is never itself an authorization grant.

## 3. Checkpoints, bootstrap and durable gaps — migration 000004

Define stream `S = (source_owner, aggregate_type, aggregate_id)` and consumer
stream `K = (consumer_name, S)`. A generation has a distinct immutable consumer
name; changing its meaning or reusing an old name is forbidden. Company/branch
scope is preserved in validated authority/contract evidence; no nullable company
column becomes a wildcard or an invented universal stream invariant.

| Owner-local table | Key and immutable evidence | Allowed mutable work |
| --- | --- | --- |
| `consumer_bootstraps` | `bootstrap_id` PK; K, consumer kind, generation, root local admission ID when custody, contract ID/version/digest, authority/scope/purpose refs, source checkpoint ref, `start_after`, secure snapshot manifest/hash or explicit no-snapshot contract ref, new-empty proof if start zero, installation request ID unique, created time | None |
| `consumer_checkpoints` | K PK; bootstrap ID FK with exact K/kind/generation match | `position`, last event ID/hash (both absent only at bootstrap), revision, updated time |
| `consumer_gaps` | `gap_id` PK; K/bootstrap FK, blocked event ID/hash, expected position, observed higher position, admission/authority refs, request ID unique, created time; unique K/blocked-event identity | None |
| `consumer_gap_attempts` | `attempt_id` PK; gap FK, prior attempt ID (unique successor, same gap), request ID unique, action `requested/recovered/held/resumed/resolved`, requested inclusive interval, checked recovery manifest ref/hash or safe hold reason/authority ref, resulting contiguous position, created time | None |

Checkpoint creation requires the exact bootstrap row and position `start_after`;
it never defaults to zero. Database guards enforce bootstrap identity and
nonempty required evidence shape. Subsequent changes are **exactly position+1,
revision+1**, with an existing same-transaction-or-committed inbox row matching
consumer/event/hash. The adapter additionally matches the validated envelope's
S and integration position and enforces current admission/schema/scope. SQL
shape checks are not that authorization and do not parse opaque payloads.
Inbox identity is not enough to advance an arbitrary different stream. Deny
UPDATE of all bootstrap/key fields and checkpoint DELETE/TRUNCATE.

T-014's typed transaction-bound checkpoint adapter must expose operations
equivalent to `InstallBootstrap(VerifiedBootstrap, InstallLocalSnapshot)`,
`CheckNext(ValidatedPosition)` and `Advance(NextCandidate, ApplyLocalEffect)`.
Exact Go names may be selected within its leaves; the contractual semantics are
mandatory. Constructors seal copied values; candidates bind the transaction,
bootstrap, consumer, stream, expected position and envelope hash. They are not
committed receipts and cannot be manufactured by passing an integer. The owner
maps these operations into its own UnitOfWork interface. No network request,
commit, ACK or automatic callback retry occurs inside these operations.

Bootstrap authority is obtained outside the SQL transaction through the
authenticated source port, including a purpose-permitted snapshot for exact
high-water h. Installation acquires the receiver stream fence, revalidates
the proof/current admission and atomically inserts bootstrap, installs local
snapshot-derived read rows, initializes checkpoint h, and performs T-920's
admission/enrollment/backlog transition. Any part failing rolls all back. A
source proof for a newly empty stream is required for h=0; newest observed
delivery is never a high-water. A read bootstrap never authorizes historical
process commands. Large snapshot/backlog work may be staged in an inactive
generation, but final activation must prove completeness under the same fence;
do not silently replace atomic activation with partially ready membership.
Prepare bounded source recovery/history manifests outside locks, then re-read
retained backlog and the selected finite boundary under the activation fence.
If new observations invalidate completeness, abort activation and prepare again;
never call the source while holding that transaction open. A staged generation
is neither an active consumer admission nor permission to skip a missing range.
This initial protocol promises safe activation, not bounded catch-up time under
an indefinitely changing source; a needed staged activation optimization must
be explicitly resliced with equivalent completeness evidence.

On a gap, T-014 returns a typed gap candidate and fails the effect transaction;
the attempted inbox insert/effect/checkpoint/job completion must roll back.
Then the orchestrator uses a **separate local transaction** to record the gap
and a recovery attempt, re-reading checkpoint and identity under the same fence.
If another worker already filled it, record that resolution rather than a stale
hold. This separate commit is essential: returning an error after writing gap
evidence in the effect transaction would erase that evidence. For direct mode,
leave the original delivery unacknowledged; crash before evidence commit remains
recoverable from it. Custody jobs remain incomplete and their bytes retained;
the prior custody ACK remains valid. No need to copy raw bodies into gap tables.

Recovery fetches only the missing authorized interval from the source port
**outside** any database transaction. Returned original envelopes must have
contiguous requested positions, matching S/event IDs/schema/hash and purpose
authority. Feed them through the ordinary admission and dedup/checkpoint path:
T-921 custody/enrollment in custody mode, or T-013 direct composition in legacy
mode. The direct recovery callback has no broker delivery to ACK and must be
explicitly configured as such; it never ACKs the still-blocked original. Do not
resolve a gap until authoritative checkpoint covers its required interval.
Denied/unavailable/history-conflicting recovery records a safe evidenced hold;
no current-state adoption, skipped position or synthesized integration event.
Attempts form a retained chain and drive existing bounded retry/alert policy;
request timing/attempt selection derives from that chain, not a second mutable
queue or a terminal business-failure state. Duplicate/unknown attempt commits
reconcile by request ID and exact evidence before another request.

## 4. Quarantine and redrive — migration 000005

Quarantine must preserve arbitrary input even when JSON, route, event ID or
schema is invalid. An event ID parsed from rejected bytes is untrusted metadata,
not a PK/FK or a newly accepted integration identity. Preserve a separate
`evidence_id` and a checked content digest; never fabricate a valid event ID.

| Owner-local table | Immutable contents and key | Mutable work |
| --- | --- | --- |
| `quarantine_evidence` | evidence UUID PK; capture request UUID unique; stage `intake/direct-handler/custody-handler`; trusted owner/queue or K/job context when known; optional untrusted claimed ID stored only as protected content; exact raw SHA-256 and byte length; sealed ciphertext, ciphertext SHA-256, approved format/key/policy refs; safe bounded reason code, trusted capture context digest, created time | None |
| `quarantine_actions` | action UUID PK; evidence FK; request UUID unique; prior action FK and unique successor for this evidence; `hold/redrive-request/redrive-result` action, authorized actor/scope/purpose and repair refs, intended path/consumer, optional accepted event/hash and resulting authoritative inbox/job/custody receipt refs, safe outcome, created time | None |

The capture request UUID is created before starting persistence and reused for
unknown-commit reconciliation. Redelivery without a stable request ID may create
another capture, which is safe evidence duplication; equal arbitrary bodies in
different trusted contexts must not be silently merged. A reused request ID with
different bytes/context is a conflict. Store safe reason **codes**, not error
strings containing body fragments, tokens, stack dumps or attacker headers.

**Raw input has no implied plaintext retention authorization.** Production
capture requires an injected owner security port that returns a sealed value
binding exact raw hash/length to ciphertext, format/key reference and approved
capture/retention policy. Encrypt outside the effect transaction; persist the
sealed bytes in the owner's SQL evidence row inside its transaction. Keep no
raw body column, body logging or unreviewed external blob export. No encryption
algorithm, production key, permitted byte limit or retention duration is chosen
here. If policy/key/sealing is unavailable, fail closed before ACK; this is an
activation gate, not permission to discard or store plaintext instead.

T-015 can implement and live-test its generic adapter using explicitly synthetic
nonsecret fixture input and a fixture-only sealing port. Such a port is injected
only by tests and must not be a default/fallback production constructor. Tests
prove ciphertext/reference storage and transaction behavior, **not production
cryptography or authorization**. Real security adapter/key/storage acceptance
and OD-10 data/retention choices remain feature activation gates. No automatic
purge, expiry, broker-body export or cryptographic erasure policy is added.

For invalid intake or a direct poison message, only a known committed evidence
receipt permits this delivery's ACK(false). Failed/unknown evidence commit
leaves it unacknowledged; reconcile request ID plus exact hash/context before
ACK. A receipt is database custody of protected evidence, not accepted business
effect or validation success. In custody-handler failure, first roll back the
effect transaction. A fresh transaction locks/fences the same unfinished job,
persists evidence/action, and sets `quarantine_ref = 'quarantine:<evidence UUID>'`
atomically. It must not set completed/inbox fields or advance checkpoint. If the
job completed concurrently, retain a correctly described observation without
reopening or quarantining the completed job. T-015/T-923 validate the reference
belongs to that K/event; do not widen T-917 grants or retroactively reinterpret
old arbitrary text references. Existing unknown references stay held pending
separate evidence disposition, not silently backfilled or cleared.

Redrive authorizes and appends a request action before restoring plaintext via
the security port outside the SQL transaction. Verify restored exact raw bytes
against their original digest/length; unknown schema can be retried after an
approved decoder exists. Correcting bytes cannot overwrite evidence, reuse an
already conflicting accepted event ID or invent a missing integration position.
An actual source correction needs its own approved event/repair contract;
otherwise remain held. Recovered valid bytes re-enter ordinary T-921 admission
and complete-membership intake, or T-013 direct dedup. For a retained custody
job, authorized release of its own quarantine reference and the audit action
commit together, preserving other holds; T-923 executes the original job later.
No nested effect transaction, bypass of schema/current scope, clearing another
hold or fabricated successful repair result. A later result action cites actual
authoritative receipt(s). Crash after application but before result journaling
is recovered by ordinary dedup and receipt reads, not a second effect.

## 5. Projection generations and switching — migration 000006

| Owner-local table | Key and immutable content | Allowed mutable work |
| --- | --- | --- |
| `projection_generations` | `(projection_name,generation_id)` PK; globally unique immutable consumer name, projection-only kind, handler/contract digest, approved authority/history/bootstrap manifest refs and hashes, creation request UUID unique, created time | None |
| `projection_generation_events` | event UUID PK; generation FK; request UUID unique; previous event FK with unique successor; action `build-progress/validated/hold/abandon/switch`; exact permitted source-stream vector and historical cursor manifest/hash, invariant comparator/version/result ref/hash, expected pointer epoch/generation for switch, created time | None |
| `projection_heads` | projection name PK | active generation FK (nullable before first activation), positive epoch, fence/hold reference, last switch event FK, updated time; all changes CAS revision+1 with corresponding immutable event evidence |

All generation read rows are in their owner's feature tables keyed by generation;
these migrations create no business read model, synchronous VIN/capacity guard,
financial table or snapshot payload store. T-016 binds owner-supplied historical
reader, generation row writer and invariant comparator ports. They receive
recorded payloads/immutable secure references only: no current aggregate/query
lookup, clock-based business decision, command bus, outbox or process subscriber.
Missing history/upcaster or unresolved tombstoned data semantics blocks that
generation, preserving the active pointer.

Historical rebuild and live admission are distinct phases. An owner-authorized
finite **vector** of stream bounds is not a global event order. Replay the exact
permitted recorded history into inactive rows, persisting build cursor manifest
and read rows atomically per bounded batch. Never infer history authorization
from stored custody. Internal aggregate revisions use T-010's full ordered
replay when the owner projection contract needs them; integration positions use
T-014. A historical cursor beginning at the start of approved history is not a
live checkpoint of zero. After the historical prefix through h is built and
verified, install that generation's live checkpoint at authenticated h through
T-920 and T-014, including retained authorized positions above h. No source
zero bootstrap is manufactured to replay an existing stream. Already processed
old-generation inbox entries do not suppress the new consumer's live work.

T-920 must not depend on T-016: it accepts a verified opaque projection
generation identity/contract, with no generic SQL dependency on generation
tables. T-016 composes T-920 after its generation exists. Process membership
cannot use the projection-generation API; process catch-up remains its own
unapproved feature contract.

Switch uses one local transaction. Take the exclusive receiver catalog fence,
revalidate the complete stream universe, fence receiver membership for that
stream set, then fence both generations against local apply/build writers and
lock the projection head. Revalidate full source-stream manifest, current
admissions/holds, no unresolved gaps, new-generation contiguous checkpoints and
invariant comparison at the **same** vector. The vector must dominate the
current active generation's checked progress; if it does not, leave the pointer
unchanged and catch up. Equality of aggregate counts alone is insufficient.
Future queued jobs above that vector stay durable for the new generation;
never require unrelated owners to stop globally. Persist comparison/switch
evidence and CAS the expected head epoch/generation together. No historical row
deletion or implicit old-consumer cancellation follows from switching. Old
obligations are drained/held using their already-approved admission policy.

Readers resolve one `(generation,epoch)` per query and use it consistently for
all rows/counts/pages in that query; no cross-generation joins after rereading
the pointer mid-request. Retaining the previous generation makes already-started
reads safe. A pointer switch does not promise a transaction across separate HTTP
requests; pagination tokens must carry generation or reject incompatible reuse.

Synchronous command-guard rebuilding is stricter. T-016 can provide a typed
exclusive fence protocol and synthetic fixture proof, but cannot certify a real
VIN/capacity/last-admin guard whose command paths are outside its ownership.
Every command writer must obtain the matching shared fence **before** reading
or modifying that guard and check active epoch/hold. Rebuild holds the exclusive
fence while replay comparison and guard pointer/data switch occur. If a concrete
owner command cannot participate, its guard rebuild remains disabled until a
separately assigned exact owner-adapter handoff; do not invent a generic table
lock covering unseen code or treat an asynchronous projector as guard authority.

## 6. Transaction, lock, grant and installation contract

All mechanical adapters bind the actual outer ReadCommitted SQL transaction,
reject pools/ended handles and never create nested Run/commit/ACK boundaries.
T-922 `AppliedCandidate` and `DuplicateCandidate` remain uncommitted; T-923 must
complete its exact job for either candidate and recheck the final live lease
condition after effect/checkpoint writes. An expired/lost fence rolls them all
back. A retained legacy inbox duplicate can finish only its matching pending
job, never synthesize a missing checkpoint or advance another consumer.
For a sequence-dependent pending job, T-923 must independently verify its
retained duplicate against the matching checkpoint/bootstrap and stream evidence
before completion. If that checkpoint is absent, behind the event position, or
inconsistent with the retained inbox/event identity, return a typed reconciliation
hold and leave the job incomplete. A checkpoint ahead is acceptable only with
matching bootstrap/stream and retained evidence covering that event, not merely
a larger integer. The duplicate path never calls `Advance` to repair a missing
position. A migrated inbox alone proves deduplication, not the missing historical
effect/checkpoint transaction; a separately authorized import/recovery must
establish that evidence before release. T-923 owns this completion precondition;
T-922's generic duplicate candidate API remains unchanged.

For the new receiver adapters first acquire the owner catalog advisory key
`justixauto:receiver-catalog:<local-owner>`: shared for ordinary intake/apply,
exclusive for membership changes and generation switches. This prevents a new
stream admission from becoming a phantom outside a switch's checked vector.
Then use one deterministic transaction advisory fence per S:
`justixauto:receiver:<local-owner>:<source-owner>:<aggregate-type>:<uuid>`,
hashed with `hashtextextended(key,0)`; acquire the sorted complete set once before
receiver data locks. T-926 owns the small shared helper in
`pkg/eventstore/receiver_fence.go`, consumed by T-014/T-920/T-921/T-923; its values
are opaque transaction-bound handles. The helper validates actual transaction,
canonical owner/stream keys, one complete sorted acquisition and handle custody;
it does not install schema or grant admission. Keeping it below both adapters
avoids a `projection`/`inbox` import cycle. `pkg/inbox` must not import
`pkg/projection`: T-920 accepts a typed transaction-bound bootstrap installer
callback that the outer composition maps to T-014, and T-923 receives checkpoint
effects through its generic owner UnitOfWork. T-016 may compose membership through
`pkg/inbox`; no reverse package import is required.
That same T-926 helper leaf owns typed preparation of generation/guard advisory
fences, so neither a new unassigned utility file nor an inbox-to-projection import
is needed. Generation key namespace is `justixauto:projection-generation`, with
length-prefixed UTF-8 components `(local-owner, projection-name, generation UUID)`;
guard namespace is `justixauto:command-guard` with components `(local-owner,
approved guard-contract ID, canonical owner-defined scope key)`. Length prefixing
prevents delimiter aliases. A typed request includes shared/exclusive mode;
merge duplicate keys to the strongest required mode and sort encoded keys before
acquisition, with no later shared-to-exclusive upgrade. Ordinary generation
apply/build writers hold shared generation fences; invariant comparison/switch
holds exclusive fences for every compared/switched generation. Guard command
writers hold the declared shared guard fences; guard rebuild holds exclusive.
The owner must supply the exact guard scope/key mapping to every affected writer;
this helper cannot discover that business write set.

Before opening or entering the effect phase, T-923's outer typed preparation maps
the validated consumer/generation claim and proposed owner effect to the complete
source/receiver/generation/guard fence requests. Acquire them in the order below,
then revalidate current admissions and the exact job under those locks. Pass a
sealed transaction-bound `PreparedFences` capability into bound adapters; each
adapter checks that its required key/mode is covered. It must reject a missing
key before reading/writing its effect, never acquire an earlier-order fence from
inside the effect callback. Preparation can abort and be repeated outside the
transaction when the current state invalidates its proposed write set. T-014,
T-920 and T-923 consume the shared preparation API; T-016 supplies its exclusive
generation/guard requests and owner compositions map concrete command ports.
SQL advisory locks remain cooperation within a trusted owner runtime, not proof
that arbitrary owner code obeys the protocol or an authorization grant.
Receiver fences serialize short local mutations, not remote fetching or a
consumer's entire backlog. Distinct checkpoints/jobs keep consumers independent.

Composition order is: any required sorted T-009 source append fences, receiver
catalog fence, sorted receiver stream fences, sorted generation/guard advisory fences when applicable,
projection head if needed, exact job row/lease, T-922 inbox/mode fence and dedup,
checkpoint row, owner effect rows, final checkpoint and job-completion writes.
Owner effect adapters must declare extra locks and acquire prerequisite source
fences before this sequence; discovering a missing prerequisite requires leaving
the transaction and replanning. Do not obtain a receiver/generation fence after
holding a job, and do not acquire new earlier-order locks from an effect callback.
This is a **new adapter composition rule**, not a claim that existing arbitrary
owner adapters or T-922 alone implement a global deadlock-free order. The actual
composition must test lock contention and rollback; deadlock/timeout yields no
ACK, completion or automatic callback retry. T-922's demonstrated migration
contention limit remains. Migration locks have separate offline ordering and
require stopped affected processes/channels, not online lock-order optimism.

Runtime gets SELECT+INSERT on the append-only tables and SELECT+INSERT plus only
the named UPDATE columns on checkpoint/head tables. Foreign-key checks and
invoker triggers enforce key/evidence linkage, append-only content, contiguous
checkpoint increments and pointer epoch CAS shape. No UPDATE/DELETE/TRUNCATE,
REFERENCES/MAINTAIN privilege on immutable tables, broad table UPDATE, PUBLIC
or grant-option access, ownership, DDL, privileged migration execution or
default/reachable-role escalation. Audit direct and column privileges, inherited
and SET ROLE-reachable NOINHERIT chains and default privileges, as prior QA did.
The owner runtime remains a trusted service boundary, not per-user SQL auth;
SQL cannot attest real business authorization, cryptography, callback effects
or that a comparison algorithm is correct. Those are typed-port and exact
composition QA responsibilities. No SECURITY DEFINER blanket mutation API.
Every evidence chain has exactly one root, at most one successor per prior ID,
same-parent validation and no cycles; enforce the root with a partial unique
index rather than nullable uniqueness alone. Appenders serialize under their
parent fence and match expected current tip. They reject a branch/conflict rather
than selecting whichever action timestamp sorts latest. Safe action payloads
are typed/versioned with checked canonical manifest digests; arbitrary JSON or
free-form SQL is not an evidence port.

Each forward script is explicit psql BEGIN/COMMIT under the existing migration
owner with five-second lock waits, owner/runtime/prior-artifact checks and a
new immutable, runtime-SELECT-only compatibility singleton. Use
`projection_checkpoint_compatibility` (revision 4),
`quarantine_compatibility` (revision 5), and
`projection_generation_compatibility` (revision 6). Each marker contains owner,
runtime, feature format 1, preceding migration identities, its exact supplied
artifact SHA-256, backup/stopped-runtime/compatibility refs and finite installed
time. The runner verifies actual files; SQL cannot hash its own input, validate
a backup or prove external processes stopped. Preserve `messaging_mode` base
version 2, mode transition, existing marker/OIDs/ACLs, all previous content and
the established routes. No new mode, broad readiness bypass, DDL in Go, automatic
backfill, down migration or installed-file rewrite.

000004 requires T-008+000002+corrected 000003; 000005 requires 000004; 000006
requires 000005. Ordered installation keeps one artifact lineage. Lock existing
tables in the established migration order (outbox, inbox, mode, messaging
parents/children), then new prerequisite tables in declared stable name order;
fail atomically for wrong identities, mismatched prior digests, existing markers,
incompatible shape/privileges or lock contention. New storage starts empty;
retained legacy checkpoint/quarantine state, if any exists outside these tables,
requires an explicit separately reviewed import before activation. Absence is
never a checkpoint-zero migration strategy.

T-022 checks the applicable exact markers/artifact hashes and grants in addition
to existing owner/mode/admission readiness. Owner scaffolds T-024/025 remain
explicitly legacy-only; these migrations do not silently advance their
hardcoded owner ledger or make them custody-compatible. Their later composition
handoff must own any necessary narrow readiness changes before enabling these
adapters. Unknown commit always discards local candidates. Reconcile bootstrap
request, gap attempt, quarantine request/action, checkpoint+inbox+job or
generation switch event/head from a fresh authoritative read using the exact
request identity/hash/expected epoch. No missing reply becomes success or proof
of rollback; no external source/security/broker effect is repeated inside Run.

## 7. Smallest bounded ownership amendment proposed

Reserve IDs only after coordinator confirms they remain free. Three migration
tasks isolate independently reviewable durability/privilege failure surfaces;
one omnibus migration would hide all three behind the existing three-hour Go
tasks. No new generic storage package/task is necessary: adapters remain in
their already assigned leaves. Each new migration task is at most four hours;
reslice before exceeding it, preserving its independently testable schema unit.

| Proposed task | Exact application leaves | Direct dependencies | Contribution |
| --- | --- | --- | --- |
| T-926 checkpoint/gap storage and receiver fence | `pkg/eventstore/migrations/000004_projection_checkpoint.up.sql`; `pkg/eventstore/projection_checkpoint_migration_test.go`; `pkg/eventstore/receiver_fence.go`; `pkg/eventstore/receiver_fence_test.go` | T-925, T-008, T-009 | B-01.AC2; live constraints/rollback/grants and transaction-bound fence only |
| T-927 protected quarantine storage | `pkg/eventstore/migrations/000005_quarantine_evidence.up.sql`; `pkg/eventstore/quarantine_migration_test.go` | T-926, T-013 | B-01.AC2; sealed synthetic evidence/immutable audit only |
| T-928 generation/fence storage | `pkg/eventstore/migrations/000006_projection_generation.up.sql`; `pkg/eventstore/projection_generation_migration_test.go` | T-927 | B-01.AC2; generation/pointer evidence only |

Ancillary results/independent QA use the ordinary exact T-ID paths. No shared
manifest or existing migration ownership changes. The coordinator's canonical
promotion, if approved, adds **only** these direct dependency edges:

- T-014 → T-926 (checkpoint/gap adapter now has reviewed storage).
- T-015 → T-927, T-922 (both direct evidence and caller-bound custody composition).
- T-920 → T-014 (atomic verified bootstrap and shared receiver fence).
- T-921 → T-015 (durable protected invalid-intake evidence before ACK).
- T-016 → T-928, T-920 (generation storage and real admission handoff).
- T-022 → T-928 (full local migration lineage/readiness; not owner feature wiring).
- T-580, T-581, T-582, T-583, T-584, T-585, T-586 → T-016
  (projector roots must consume actual generation/fence readiness).

Keep all existing dependencies. T-923 already depends on T-014/015 and receives
the receiver-first lock/candidate handoff within its assigned leaves; T-921
already depends on T-920. T-014 must not depend on T-920/921/923: recovered bytes
are returned through a typed injected transport orchestration port. T-015 must
not depend on T-921/923: its redrive result describes the required ordinary path
to its outer orchestrator, not an import/call into future intake/dispatch. These
directions avoid cycles. T-016 depends on T-920, never vice versa. T-919 and app
shell tasks remain independent. B-01 acceptance still closes only at T-591.

T-014/015/016 may first implement narrow typed contracts and synthetic fixtures
in their owned Go leaves, but cannot claim durable adapter completion or become
integration-ready before their migration dependencies are independently GREEN
and integrated. Three-hour adapter bounds still apply; a demonstrated larger
implementation needs an explicit coordinator reslice, not hidden additional
files. This proposal does not itself alter statuses or grant production policy.

## 8. Mandatory evidence for approval and implementation

Proposal QA should check exact source/task references, absence of application
changes, dependency acyclicity, nonoverlapping leaves, every mechanical key and
mutable grant, and the retained policy/transaction boundaries above. SQL and
adapter failure tests remain **planned**, not executed by this draft:

1. Pinned isolated PostgreSQL, narrow runtime and separately provisioned owner;
   each migration installs atomically, old files/data/ACLs/markers are unchanged,
   reinstall/wrong artifact/owner/default or reachable excessive grants fail,
   concurrent installer/active transaction contention gives one winner or
   bounded complete rollback. No foreign owner data access.
2. Independent consumers, same stream and same event with different generations;
   h>0 bootstrap/read rows/admission/backlog commit together; source-proved empty
   zero only; concurrent append/intake cannot strand jobs. Rollback/factory panic,
   cancellation and actual commit/rollback lost replies leave no partial state.
3. Known irrelevant type advances one position; duplicate never repeats effect;
   hash/type/source/sequence conflict fails. Out-of-order effect rollback followed
   by durable gap commit; crash in between, two recovery workers, newly resolved
   stale gap, denied/missing recovery and invalid history do not skip positions.
   Direct ACK and custody incomplete-job outcomes are asserted separately.
4. Arbitrary invalid JSON/no event ID, malformed claimed IDs, unsupported schema,
   different bytes under one request ID, ciphertext tampering and unavailable
   sealing/policy fail correctly. No raw body/error/secret appears in SQL-visible
   metadata or logs. Failed/unknown quarantine commit never ACKs; request receipt
   recovery does. Custody evidence/ref update rolls back together, wrong consumer
   or completed-job race cannot mutate another obligation. Fixture sealing is
   explicitly not a production security claim.
5. Redrive crash before/after applying and before result action, repeated request,
   current authority denied, foreign hold and repaired bytes under conflicting
   event ID preserve original evidence and one ordinary authorized effect. A
   quarantine/result record alone never supplies missing inbox/checkpoint/job
   success or marks a source position complete.
6. Historical recorded payload replay with forbidden latest-state/command ports,
   missing history/upcaster, secure tombstone and generation-name conflicts;
   crash/resume batches, differing stream vectors, failed comparison, concurrent
   live apply/switch and stale pointer epoch. Readers see one retained generation;
   failed switch leaves old pointer unchanged. Synthetic guard writer honoring
   the shared fence cannot race an exclusive rebuild; missing real owner-writer
   participation remains disabled, with no invented full-product guarantee.

Unresolved scoped decisions remain: actual feature admission/snapshot purpose,
personal-data retention/limits/production key adapter, process historical
catch-up, immediate-revocation disposition and each real guard writer's fence
integration. They block only that activation; no financial policy, privacy
retention schedule or undocumented Gaze reliability guarantee is inferred.
