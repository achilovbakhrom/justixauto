# Preparation board

This is a readiness board, not an approved implementation backlog.

| ID | Item | Status | Gate / owner |
|---|---|---|---|
| PREP-01 | Current business handoff + reference inventory | prepared | Coordinator review; open decisions retained |
| PREP-02 | Four compact runnable HTML mocks + checks | prepared | Build manifest + automated/browser evidence |
| PREP-03 | Project Codex roles, coordination and mock tooling | prepared | No development agents executed in target yet |
| ENV-01 | Independent Git root and first commit | needs-user-action | Do not initialize or use parent Git automatically |
| ARCH-01 | Confirm service boundaries, reliability and React tooling | approved | User "yes", 2026-09-13; `../architecture.md` |
| PLAN-01 | Approve first release backlog and task contracts | awaiting-backlog-confirmation | Reviewed `backlog.md`: 42 Must + 7 Should, 3 deferred; PM tasks after scope approval |
| DEV-01 | Implement the first approved vertical slice | waiting | ENV-01 + PLAN-01 + independent QA |

Updated 2026-09-13. `task-preview.md` contains 24 small examples with dependencies,
acceptance and owner paths. These are NOT approved implementation tasks and do
not replace PO release-scope or PM task-file checkpoints. No `T-NNN` task is ready.
The independent Git issue blocks DEV-01 only, not ARCH-01 or PLAN-01 documentation.
