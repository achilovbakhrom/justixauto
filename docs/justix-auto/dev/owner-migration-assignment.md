# Owner migration compatibility proposal assignment

2026-09-15. Status: ready-for-qa architecture handoff; no implementation approval.
Agent: qa_owner_migration. Branch: `task/owner-migration-compatibility`.
Worktree: `.worktrees/owner-migration-compatibility`; base `f0a72a9`.

Resolve the concrete [readiness finding](owner-migration-readiness.md) with a
bounded technical proposal. Inputs: T-024/T-025/T-026 owner UOW/migration results,
T-022 runner ownership, T-059/060/061 feature ownership, and the approved shared
messaging/checkpoint/quarantine/generation compatibility contracts. Preserve
legacy-only defaults and all existing source/SQL artifacts during proposal work.

Own only `state/drafts/architect/owner-migration-compatibility.md` and ancillary
`dev/results/owner-migration-compatibility.md` in the assigned worktree. Specify
trusted exact expected owner ledger/artifact identity, immutable feature evidence,
read-only pre-bind/Run checks, private owner and shared marker distinction, narrow
privileges, incremental ordering and fail-closed dirty/incompatible behavior.
Account explicitly for custody-mode composition readiness without silently
weakening existing legacy owner checks. No arbitrary newer-version acceptance,
automatic repair or unowned feature-table authorization design.

Provide small implementation tasks or explicit existing-task ownership extensions,
exact file leaves, dependency edges and verification cases. Keep work bounded to
this compatibility gap; generic auth response transport is a separate handoff.
No security policy, feature permissions, service roots or migration execution.
Coordinator reviews and obtains independent proposal QA before promotion.

Review exact commit `c3734a5c3f2863be8fe752338c11740b9bdf31a4`; the six task
aliases remain unassigned until technical adoption. QA owns only
`dev/qa/owner-migration-compatibility.md` and its same-named evidence directory
in the task worktree. Check actual pinned migrate/pgx interface feasibility,
receipt-plus-clean atomicity and precise ownership/dependency amendments.
