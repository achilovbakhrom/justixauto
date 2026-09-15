# Messaging route correction — independent design QA

**GREEN — proposal readiness only.** Reviewed exact commit
`631388169f3f24c97b9237bcf933de79315b3651` on 2026-09-15 in
`.worktrees/messaging-route-correction`. No blocking design finding.
This does not certify T-925 implementation or a working messaging runtime.

## Evidence and commands

Independent source inspection covered the proposal/result, approved messaging
contract and approval, T-007 envelope grammar/validation, T-011 producer and
tests, complete installed 000002 trigger/cutover/ACL definitions and relevant
fixtures, T-006 topology, and canonical dependency/ownership index. The proposal
commit changes only its two assigned documentation leaves.

- `python3 docs/justix-auto/dev/qa/messaging-route-correction/check.py`: PASS.
  [Reproducible diagnostic](messaging-route-correction/check.py) checks the exact
  commit and all three pinned source hashes, 49 source/destination combinations,
  13 malformed/unknown cases, a valid nested repeated-owner event name, and the
  proposed 925-node dependency graph. Both new source leaves are unowned; all
  11 direct and five checked transitive targets are gated without cycles.
  T-918/T-920/T-922 retain independence. This is an in-memory proposed graph;
  the canonical graph has not been changed by QA.
- `GOMODCACHE=/private/tmp/justixauto-t003-modcache
  GOCACHE=/private/tmp/justixauto-integration-gocache GOPROXY=off
  bash tools/go.sh test -race -mod=readonly ./pkg/events ./pkg/outbox
  -run 'TestRecordRequiresTypedSchemaAndRoute|TestEnvelope|TestSchema'
  -count=1 -v`: PASS, eight event tests and one record test, including their
  subtests; package times 1.548s and 1.677s. These run existing typed-envelope
  and producer-record checks, not the PostgreSQL insertion/cutover path.
- `git diff --check HEAD^ HEAD` and `git diff --check`: PASS.
- Local link resolution for the proposal/result: PASS, three links.

## Assessment

The mismatch is confirmed independently. `insert.go:59` builds
`inventory.retail.inventory.fixture.changed.v1`. Installed delivery/job guards
at SQL lines 343/383 prepend the source to a suffix beginning with a separator,
producing `inventory.inventory.fixture.changed.v1` for schema lookup. The
existing populated and custody fixtures use shortened keys, so their success
does not establish composition with T-011. The diagnostic ties its arithmetic
to the exact source expressions; it is not a PostgreSQL reproduction.

Keeping the existing producer format is consistent with T-007 and the current
broker grants/bindings. The corrected one-based SQL offset is
`length(source)+length(target)+3`, with no prepended owner. Validation must use
the complete owner-qualified suffix and exact admitted schema; legitimate
nested event names may repeat an owner word. Typed adapters still verify
envelope source/type/version and payload against their schema.

The three proposed function replacements cover the affected paths. The delivery
guard performs route validation before its admitted-versus-held branch; cutover
inserts all copied children through that trigger. The job guard checks the
exact suffix; dispatch-route validation covers custody messages including an
empty consumer set. Preserving the cutover function is therefore sufficient
for this correction, provided the specified trigger attachments and ACLs stay
intact. Its broader initial route regex does not bypass the stricter child
guard. No event, route, byte sequence, hold or completion rewrite is required.

The forward migration plan preserves 000002, one-way legacy/custody mode and
historical evidence. Ordered exclusive locks and a bounded timeout fence the
retained-row preflight; all schema replacement and marker creation share one
transaction. Preflight includes retained legacy, delivery and custody routes
and corrected delivery/job allowlists. Shortened historical records cause an
atomic failure and require separate disposition; they cannot be normalized or
reinterpreted as a new admission. The old consumer/channel shutdown remains an
operational prerequisite, not something SQL can prove.

The immutable compatibility marker resolves readiness without changing the
existing version-2 mode constraint. Owner/runtime identities, base and correction
versions, route format, artifact identities and evidence references are explicit.
Missing/wrong prerequisites, identity, marker, privileges or retained contents
must fail closed. Preserved function ACLs plus SELECT-only marker access and
direct/column/default/SET ROLE-reachable grant checks maintain the migration
authority boundary. Supplied hashes and evidence references remain claims the
runner/operator must verify; they are not independent proof of installed file
contents, external backups, closed channels or business authority.

T-925's two leaves and external test package support a genuine public
T-007/T-009/T-011 producer regression without changing active admission code or
historical fixtures. Its four-hour bound explicitly requires reslicing if
exceeded. T-919/T-921/T-924, migration readiness and every owner composition are
gated; relay, dispatch, recovery and B-01 acceptance follow transitively.

## Required implementation proof remains outstanding

The proposal's acceptance set covers actual T-011-produced populated legacy
cutover failure on unchanged 000002, exact rollback evidence, correction and
successful cutover retaining sent/unsent/held/inbox history, distinct-owner and
self-target custody/jobs, own-inbox hash and completion, malformed/admission
failures, incomplete children, immutable-content denial, concurrency, repeated
installation, grant escalation and incompatible existing custody records.
These are appropriate T-925 regressions; QA has not executed or passed them here.
T-925 must supply its pinned PostgreSQL version, migration hashes, exact commit,
runtime evidence and independent review. Later adapters/broker tasks still own
ACK, effects/checkpoints, route grants and delivery recovery verification.

No PostgreSQL container, broker, production state, business grant, payload policy,
canonical document or application source was changed by this QA. Only this
report and its diagnostic were written; no commit or merge was performed.
