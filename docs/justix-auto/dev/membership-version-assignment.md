# Membership version storage architecture assignment

2026-09-15. Status: independent r2 BOUNCE; final allowed architect fix cycle 2 assigned to architect_membership_storage; T-920 remains blocked.
Agent: `architect_membership_storage`.
T-920 result-only commit: `0cbc724f3171786f78aa922f179d8cfe0b75fead`.
Worktree: `.worktrees/membership-version-storage`.
Branch: `task/membership-version-storage`.
Base: `d59faa8dc5b26e13e19c47686dd47744b6d73a11`.

Resolve the [observed missing durable version](membership-version-readiness.md)
against approved messaging/storage/runtime-registration contracts and actual
000002–000005, T-014 and T-930 interfaces. T-928's revision-6 generation storage
and exact label correction are approved but unimplemented. Existing original
artifacts, retained rows and consumer meanings are immutable.

Architect owns only `state/drafts/architect/membership-version-storage.md`
and `dev/results/membership-version-storage.md` in the assigned worktree.
No application source, canonical document, task ID, migration or policy edits.
Give concrete bounded task aliases, exact leaves and serial successor ownership,
full lineage/privilege requirements and affected T-920/921/016/022/934 handoffs.
No reverse inbox-to-projection import or new global mutable business table.

Define complete inert catalog identity distinct from role selection, durable
admitted version and exact cutover request, current selection and zero-consumer
delta, with recoverable authoritative evidence on an empty stream. Preserve
source-issued checkpoints, explicit authorized empty proof, finite complete
backlog, snapshot/bootstraps/admission/enrollment/jobs atomicity and exact old
sets. Readiness and current-version comparisons must use retained proof under
catalog/receiver fences, not latest message or caller-chosen version strings.
Schema alone does not authenticate the release manifest or authorize data reuse.

Inspect actual PostgreSQL/public Go APIs and demonstrate the bounded storage
and interface feasibility. Cover concurrent distinct versions, exact retry versus
changed request, unknown commit, restart, zero-new-consumer transition, omitted
required claims, late stream discovery and preserved existing enrollments.
Propose explicit forward migration ownership/marker and downstream compatibility
checks; never rewrite previous SQL or overload an arbitrary reference field.
Use only unique owned pinned disposable fixtures with failure-safe cleanup if
needed. Independent exact-proposal QA and canonical promotion precede code.

## Independent proposal review

Exact proposal commit: `5996c77e3df371a3992154b639ce9ba91ffc3e75`.
Detached review checkout: `.worktrees/membership-version-storage-qa`.
Agent: `qa_membership_storage`. Own only new
`dev/qa/membership-version-storage.md` and its evidence directory.
Review complete owner catalog versus stream selection, empty-stream/zero-delta
receipts, historical reconciliation after a later head, finite complete universe,
concurrency/fences, immutable child evidence and ordinary future enrollment.
Verify four bounded aliases and exact serial/disjoint leaves; T-928 is unchanged.
The architect's reduced SQL and public Go probes are feasibility evidence, not
full migration, server restart, actual commit-loss or authority verification.
T-935–T-944 are already allocated to auth; this proposal reserves no task IDs.

## Fix cycle 1

[Independent BOUNCE](qa/membership-version-storage.md) at
`5996c77e3df371a3992154b639ce9ba91ffc3e75` reproduced a lifecycle conflict:
the proposed same-transaction transition guard rejects future ordinary intake
that links a new enrollment to an already committed selection. Separate frozen
transition/stream/consumer children from append-only dispatch evidence. Specify
the enrollment's same-transaction provenance, exact complete link/job set,
current or prospective selection binding, historical redelivery and the precise
SQL versus adapter missing-link enforcement boundary. Preserve old migrations
and ordinary future intake; no new policy or source ownership is authorized.
Architect revises only the original draft/result leaves, preserves original
evidence, and supplies concrete reduced regressions before independent r2 QA.

Revised proposal commit: `c3c0da45965c980fee22a32f43e90d7a674a55c4`.
Coordinator imports its exact draft/result after preserving both preimages.
The revision explicitly proposes a new deferred INSERT constraint trigger on
existing enrollments within SQL008's owned leaf; old trigger/function definitions
and migrations remain unchanged. Independent r2 must audit that declared catalog
delta, missing-link enforcement, ordinary future intake and inert retained state.
Original BOUNCE evidence remains unchanged; no canonical contract adoption yet.

Independent r2 assigned to `qa_membership_storage` in detached
`.worktrees/membership-version-storage-r2-qa` at the exact revised commit above.
Own only NEW `dev/qa/membership-version-storage-r2.md` and its evidence directory.
Recheck the original counterexample, every lifecycle/provenance/missing-link
boundary, declared additive trigger and exact alias/dependency/ownership delta.
Preserve all original artifacts; independent GREEN and canonical promotion QA
must precede dependent implementation.

## Fix cycle 2 queued

[Independent r2 BOUNCE](qa/membership-version-storage-r2.md) at
`c3c0da45965c980fee22a32f43e90d7a674a55c4` closes the original lifecycle
finding but reproduces a timing gap: after a valid current enrollment/link,
flushing ALL or only named constraints and then performing the sole valid head
CAS commits a link to the old head. The no-toggle control rejects.

The final allowed architect revision must define timing-independent enforcement
on every relevant mutation, preserving valid current/prospective paths and exact
final selection, missing-link checks, immutable historical reads and savepoint
rollback. Deferred checks may run early and cannot alone establish commit-final
state. Keep the SQL guarantee; no silent downgrade to cooperative-only behavior.
Own only the original draft/result leaves, preserve both QA rounds and all old
artifacts, and demonstrate ALL/named toggles in both temporal mutation orders.
Do not start a third automatic fix cycle if the next review fails; require a
bounded scope review. No new task IDs, policy or canonical contract is adopted.

Fix cycle 2 now assigned in the original `.worktrees/membership-version-storage`.
The architect owns the same original draft/result leaves; both independent QA
rounds are imported unchanged. Current task graph is 944 nodes; no alias IDs
are reserved. Future installation checks must also deny reachable parameter
authority to bypass triggers and account for explicit constraint timing.

## Final proposal review — 2026-09-16

Fix2 proposal `bb908b4425003cbf41375b46b07fb88d2a3be817` is imported exactly
after preserving both canonical preimages. It specifies immediate bidirectional
head/link guards, job-set sealing, snapshot closure, server xid8 bookkeeping and
head locks through commit. The exact runtime LOGIN/database origin prerequisite
is explicit and remains dependent on T-928's final independent approval.
The reduced probe records 63 observations, including actual lock contention.
This is proposal evidence, not an implemented membership subsystem.

Independent r3 is assigned to qa_membership_storage in detached worktree
`.worktrees/membership-version-storage-r3-qa` at the exact proposal above.
Only new `qa/membership-version-storage-r3.md` and its evidence directory are
owned. Preserve all 25 earlier review artifacts; review the complete final
contract and its declared catalog/dependency deltas. A further BOUNCE ends
automatic fixes and requires bounded scope/root-cause review. No task IDs or
canonical implementation contract are approved by this assignment.

## Technical adoption — 2026-09-16

Exact proposal `bb908b4425003cbf41375b46b07fb88d2a3be817` passed independent r3 proposal-feasibility review.
[Approval](../state/approvals/membership-version-storage.md) maps four aliases to
T-945–T-948 and adds six reviewed downstream edges. Canonical promotion QA is
pending before assignment. T-928 configuration-test permission/independent
integration and actual runtime startup are still gated. All earlier evidence is
retained; proposal GREEN does not authorize restricted configuration mutations.

Canonical promotion reviewer qa_membership_storage is assigned the exact
`b50490b48c5f31a542b0305827c1e345c2214cf6` commit in detached
`.worktrees/membership-promotion-qa`. New outputs are only
`dev/qa/membership-promotion.md` and its evidence directory. This is a document,
identity, graph, ownership and recoverable-preimage audit; no runtime or Docker
configuration test is assigned. T-945–T-948 cannot start before this gate is GREEN.
