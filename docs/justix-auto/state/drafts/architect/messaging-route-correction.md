# Messaging route compatibility correction — proposal

2026-09-15. **PROPOSED; implementation and runtime validation have not run.**
Inspected base: `f0d0454e0794dbdc4bddd3cb3b3bb4e331e9a51a`.
This is a bounded compatibility correction to ADR-03/05, not a new transport,
business admission, event schema, financial rule or service boundary.

## 1. Confirmed contract and observed defect

The approved [messaging amendment](../contracts/messaging-delivery.md), with
[approval](../../approvals/messaging-delivery.md), preserves immutable event
identity, bytes and exact recorded destination routes across forward cutover.
T-007 requires an owner-qualified event type. T-011 `pkg/outbox/insert.go:59`
constructs `sourceOwner + "." + targetOwner + "." + envelope.EventType()`.
Consequently the established route for `inventory.fixture.changed.v1` sent to
Retail is **`inventory.retail.inventory.fixture.changed.v1`**. The repeated
owner is intentional: transport source and full event type are distinct fields.
T-006's `*.retail.#` binding and source/target topic permissions accept it.

T-917 `000002_messaging_delivery.up.sql:346,381` extracts a suffix starting at
the separator after target, then prepends source. On this established key the
allowlist lookup becomes `inventory.inventory.fixture.changed.v1`; a correct
allowlist containing `inventory.fixture.changed.v1` fails. Both delivery and
logical-job guards have the defect. The dispatch and cutover regexes are broad
enough to accept both route spellings, so they do not catch the inconsistency.
T-917's populated fixture at `messaging_migration_test.go:71` instead builds
`inventory.retail.fixture.changed.v1`; its custody fixture at line 294 similarly
omits the full event type's owner. Those tests exercise the installed SQL in
isolation and do not prove composition with T-011.

This diagnosis is source inspection, not a newly executed PostgreSQL failure.
No HTML behavior supplies a transport contract. Neither Gaze audit supplies
durable outbox/inbox guarantees; retain JustixAuto's explicitly approved
hardening and the distinct reference limits in both reference documents.

## 2. One canonical route and a minimal forward correction

Retain the exchange `justix.integration.v1`. The canonical route is exactly:

```text
<sourceOwner>.<targetOwner>.<fullOwnerQualifiedEventType>
inventory.retail.inventory.fixture.changed.v1
```

After validating exact source/target components, extract everything **after the
second dot**, with no source prepended and no normalization. Require that suffix
to have T-007's source-qualified, versioned event-type grammar, including at
least one event-name component after its owner, and enforce T-011's 255-byte
route limit. Check that exact suffix against the relevant admission's schema
allowlist. Unknown owner, wrong embedded owner, missing component, wildcard,
shortened route, malformed version, extra prefix or changed destination fails.
Do not blindly reject a legitimate nested event name merely because it repeats
an owner word: exact full type and schema admission decide that case.
For validated component lengths the SQL suffix starts at
`length(sourceOwner) + length(targetOwner) + 3` (PostgreSQL's one-based indexing).
Its grammar is `^<sourceOwner>(\.[a-z][a-z0-9-]*)+\.v[1-9][0-9]*$`;
the complete key is the exact source/target prefix plus that suffix.

Typed producer/intake adapters must additionally compare the suffix with the
validated envelope's actual `EventType()`, source and schema version. The SQL
correction continues treating envelope bytes as opaque; a matching route or
schema allowlist alone is not payload validation, object authorization or
evidence that the recipient may receive the company's data.

Add `pkg/eventstore/migrations/000003_messaging_route_compatibility.up.sql`.
Preserve `000002` and T-008 byte-for-byte. In one explicit psql transaction,
requiring the existing eventstore migration owner and the exact installed owner
and runtime role, replace only these trigger functions with `CREATE OR REPLACE`:

- `messaging_delivery_guard`: canonical route validation and exact suffix lookup;
  preserve all immutable-content, admitted range, held legacy and sent rules.
- `messaging_job_guard`: exact canonical suffix lookup; preserve namespace,
  stream/range, generation, inbox hash, completion and hold/quarantine rules.
- `messaging_dispatch_route`: require canonical source/target/full-type grammar.

Keep existing trigger attachments, SECURITY INVOKER properties, fixed search
paths and function ACLs. No change to `activate_messaging_custody` is necessary:
its copied children pass through the corrected delivery guard, including held
children. Its existing evidence, locks, copy, mode transition and grant revocation
remain. A malformed legacy route still aborts the entire cutover.

Before replacing functions, take exclusive locks in this order: legacy outbox,
legacy inbox, messaging_mode, outbox_messages, outbox_deliveries,
dispatch_messages, dispatch_enrollments, dispatch_jobs; then lock the singleton
mode row. Use a bounded lock timeout. This preserves cutover's legacy-table-before-
mode ordering; a competing transaction must finish or the correction fails
without partial work. Stop affected writers/consumers
and close old AMQP channels first under the amendment's operational procedure.
Inspect retained outbox routes and every existing delivery/custody route using
the new grammar; inspect existing delivery/job allowlists using the corrected
suffix. Reject any incompatibility atomically. Do not UPDATE rows to trigger
validation, disable immutable triggers, infer replacement admissions or rewrite
bytes, routes, IDs, holds, sent/completed state or prior evidence. An installed
v2 custody database is eligible only if its retained records already pass.
Shortened v2 fixture/operational records require investigation and a separately
reviewed forward disposition; this correction supplies no automatic repair.

## 3. Version evidence, privileges and readiness

Keep `messaging_mode.schema_version=2`, its check constraint and one-way mode
transition unchanged. Add one immutable singleton table
`eventstore.messaging_route_compatibility` with this explicit compatibility tuple:

| Field | Required value/meaning |
|---|---|
| `singleton` | Boolean primary key constrained true |
| `owner_service`, `runtime_role` | Exact existing `messaging_mode` identities |
| `base_schema_version` | 2 |
| `migration_revision` | 3 |
| `route_format_version` | 1, the full-event-type format specified above |
| `prior_migration_sha256`, `correction_migration_sha256` | 32-byte digests of installed 000002 and the exact 000003 artifact |
| `backup_ref`, `stopped_runtimes_ref`, `compatibility_ref` | Nonempty references to recoverable preimage, stopped channels/processes and reviewed installation evidence |
| `installed_at` | Finite timestamp |

The runner computes/verifies artifact hashes before supplying them to the psql
script. SQL validates the expected prior digest, correction digest length and
required evidence fields; it cannot prove a supplied hash matches its own file,
an external backup exists, or a broker channel is closed. Record old/new function
definitions and ACLs, table counts/content digests and these references in the
recoverable operator/test evidence. Preserve any existing cutover's schema hashes
and rows; later activation evidence identifies the cumulative corrected schema
and marker, rather than overwriting earlier evidence.

Create the marker and functions atomically. Reinstallation, wrong prerequisite,
wrong identity, incompatible retained rows or unexpected grants aborts with no
marker and unchanged definitions. Runtime receives SELECT on the marker only;
PUBLIC receives no grants. Reuse the immutable trigger for UPDATE/DELETE defense,
and audit direct, inherited/SET ROLE-reachable, column and default privileges.
Do not grant runtime DDL, marker INSERT/UPDATE/DELETE/TRUNCATE, migration-function
EXECUTE, or wider work-table permissions. No destructive down migration.

Custody readiness requires **both** base schema version 2 and exactly one matching
marker with revision 3 / route format 1 and the expected artifact identities,
plus existing owner, mode, ACL, membership/admission and adapter checks. Missing,
wrong or partially applied marker is incompatible; `schema_version=2` alone is
insufficient. A marker proves this SQL correction installed, not operational
readiness or business delivery. T-022's explicit migration runner and readiness
checks consume the tuple; T-580–T-586 must enforce it before starting custody
producer/intake/dispatch compositions. Their normal owner migration ledger is
separate and must stay clean/compatible. T-024–T-030 legacy-only scaffold checks
do not acquire custody guarantees through this proposal and need no edits here.

## 4. Bounded task and dependency changes proposed for coordinator

Add **T-925 — Correct retained messaging route compatibility**, B-01 foundation,
implementation maximum 4 hours, dependencies **T-917, T-011, T-007, T-006**,
branch `task/T-925-messaging-route-compatibility`. Reslice before exceeding the
bound. Only these application leaves plus assigned result/QA evidence are owned:

- `pkg/eventstore/migrations/000003_messaging_route_compatibility.up.sql`
- `pkg/eventstore/messaging_route_migration_test.go`

The new external-package test can import public T-011 APIs without an import
cycle. Keep any additional disposable fixture helpers in that test leaf; do not
edit the old fixture helpers, 000002, T-011 or T-918's active leaves. No new
dependency or shared manifest is needed. Developer result is `dev/results/T-925.md`;
independent exact-commit QA is `dev/qa/T-925.md` and `dev/qa/T-925/`.

Add T-925 as a direct prerequisite of **T-919, T-921, T-924, T-022 and
T-580–T-586**. Existing edges then block relay T-012, dispatch T-923, recovery
T-021 and closing acceptance T-591 transitively. These tasks retain their
existing owned leaves; coordinator adds the readiness/route acceptance to their
task inputs. T-919 builds the same established key, T-921 compares it to the
validated envelope and stages jobs against corrected SQL, and T-924 compiles
exact full-type topic grants and proves those keys against the pinned broker.
T-925 does not own broker topology or promise its live verification.

T-918 and T-920 admission-only development can continue against installed 000002.
Any shortened fixture route in T-918 is explicitly a **historical 000002-only SQL
fixture**, never a producer integration or approved route example. Preserve that
test's historical version scope; separately assigned T-925 supplies the genuine
T-011 integration regression and T-919 supplies source-admission plus corrected
insertion integration. No concurrent edit or new T-918 implementation dependency
is required. T-922's transaction-bound inbox work is independently eligible.

## 5. Required regression evidence before integration

Use fresh pinned PostgreSQL 18.6 fixtures with synthetic credentials, isolated
owner databases and narrow runtime roles; no existing DSN or production action.
An opt-in `JUSTIXAUTO_TEST_MESSAGING_ROUTE_MIGRATION=1` test must prove:

1. Construct a T-007 schema/envelope, append with T-009 and call actual public
   T-011 `NewRecord`/`Inserter.Insert` in the same transaction. Read the resulting
   SQL row; assert canonical route and byte/hash identity. Do not manually insert
   that legacy row or reconstruct its route in the fixture.
2. Install unchanged 000002, stage a correct synthetic source admission and
   legacy authorization, and show the populated cutover fails specifically at
   the old schema lookup. Assert mode, cutover/evidence rows and new message/work
   rows all rolled back, with legacy content intact.
3. Apply 000003, then perform the same populated cutover successfully. Include
   genuine producer-created sent, unsent and explicitly held obligations plus
   prior inbox evidence. Prove full original rows/evidence remain, routes/bytes/
   hashes/IDs/sequences and prior sent status are retained, active leases reset
   only as already specified, and held obligations stay held.
4. Take the exact resulting canonical route and envelope to a distinct target
   owner's corrected store. Stage explicit synthetic local membership and
   complete custody/enrollment/jobs in one transaction. Validate successful
   scheduling and own-inbox completion without changing content. Also exercise
   a canonical source-to-self route. This is SQL custody/job compatibility,
   not an implemented T-921 ACK or T-923 business-effect proof.
5. Wrong embedded source, target, full event type, version, shortened key,
   wildcard, overlength key and missing schema admission fail; incorrect job
   generation, mismatched inbox hash, incomplete children, changed immutable
   bytes/route and held completion still fail. Retain rollback/concurrent-writer
   probes and test that a failed correction leaves the old functions and no marker.
6. Test an empty installation without claiming populated coverage; missing/wrong
   prerequisite/marker, repeated install, attempted reverse mode, runtime DDL or
   marker mutation, inherited/default grant escalation, and an incompatible
   preexisting v2 custody fixture. Verify failure preserves every historical row
   and correction does not make a previously forbidden grant or operation legal.

Record exact reviewed commit, 000002/000003 hashes, fixture server version,
commands and PASS/failure evidence. Run project-package Go race/vet checks and
independent QA. T-919/T-921/T-924 later prove their complete adapter/broker legs;
route-correction SQL tests alone do not establish end-to-end delivery.

## 6. Evidence boundaries and unresolved gates

Ownership remains seven private stores; all copy and validation is owner-local.
No mutable business table, cross-owner transaction, new schema admission or
financial policy is authorized. Concrete authority, scope, bootstrap, immediate
revocation disposition and unusual retained-data recovery remain separately
approved owner decisions. Unknown historical routes stop the affected migration;
they are never silently normalized into an apparently valid grant.

Pinned source hashes at the inspected base:

```text
000002: 1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2
T-011 insert.go: e9e6d7e37f7adc0e6f7e15539a06f8215f15cb4da720703ab0ead14996c98a03
approved amendment: 97e70c29183feded9e58f702ba2c995dc67b8ad4e6896c5f3e877b49db94c7d7
```
