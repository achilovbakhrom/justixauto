# Membership version storage architecture assignment

2026-09-15. Status: independent proposal QA BOUNCE; architect fix cycle 1 assigned; T-920 remains blocked.
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
