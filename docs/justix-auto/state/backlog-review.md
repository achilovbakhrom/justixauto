# Release backlog review — 2026-09-13

Status: coordinator-reviewed PO proposal, not user release approval or a PM task board.
Canonical candidate: `../dev/backlog.md`. Architecture approval is recorded in
`architecture-approval.md`; business and visual gaps remain scoped gates.

## Review and corrections

The product-owner produced a 50-item proposal. An independent coverage reviewer
checked it against business logic, approved architecture/contracts, mock mappings
and the active Realization dashboard override. Coordinator reviewed the outputs
and promoted a 52-item proposal with these bounded corrections:

- B-06 adds the actual ready-dashboard summaries, shortcuts and recent wholesale
  rows. It does not restore the overridden legacy task dashboard or invent the
  public API for a presentation-only wholesale-sale shortcut.
- B-32 adds wholesale physical handover, durable inventory finalization and
  commerce completion. Route/payment/evidence and missing public command/UI
  contracts remain explicit gates; physical completion is not legal-title transfer.
- B-28 and B-30 consume the common accepted order from B-26 OR B-27 and test both
  origins, not an all-of blocker on completing both entire ordering flows.
- B-47 depends on B-40 only if the approved submission snapshot needs a persisted
  installment calculation. B-38 keeps insurer/calculation dependencies specific
  to own installments, not cash sales.
- Approval status now points to the recorded user decision. RU/UZ remains a
  proposed backlog choice, not an inferred previously confirmed requirement.

## Validation performed

- 52 unique sequential IDs; 42 Must, 7 Should, 3 Won't. B-01…B-49 are the proposed
  release envelope; B-50…B-52 are Icebox.
- All items carry value, acceptance, dependency, exclusion and design fields.
- All release FE items carry M-IDs and HTML navigation/source anchors; every
  referenced M-ID exists in architecture and referenced JS/CSS source paths exist.
  B-01 and B-03 are nonvisual service/operability items.
- No unknown dependency IDs. The conservative graph including both alternatives
  and conditional references is acyclic. This does not replace PM's task-level
  graph, size, ownership or readiness checks.
- Missing auth/MFA/recovery and operational microstates, unified RFQ/factory/
  logistics views and unconfirmed policy-specific states remain explicit design
  gates. Source anchors alone are not screenshot proof. No mock was changed.
- The independent-root project Git guard still fails; the source repository's
  older checker incorrectly accepts the inherited parent. No Git was initialized,
  staged or committed; application code and infrastructure remain untouched.

## Draft evidence and ID mapping

Original worker evidence is preserved in `backlog-review/coverage-checklist.md`
and `backlog-review/original-review.md`. Their B-IDs refer to the original 50-item
draft, before final renumbering:

| Original draft IDs | Published proposal IDs |
|---|---|
| B-01…B-05 | unchanged |
| B-06…B-30 | add 1: B-07…B-31 |
| B-31…B-50 | add 2: B-33…B-52 |
| new dashboard | B-06 |
| new wholesale handover | B-32 |

No user-approved IDs or task files were renumbered. Source coordinator drafts
remain at `spec-team/docs/justix-auto/state/drafts/plan-po-r1/`.
Existing canonical docs were fingerprinted and snapshotted before status edits
under `backups/2026-09-13-plan-po-r1/`; no prior artifact was deleted.

## Next checkpoint

Present release scope/priorities, missing UI states and exclusions to the user.
Only after approval, append that decision and run PM decomposition into bounded
task files. Do not treat this review or architecture approval as backlog approval.
