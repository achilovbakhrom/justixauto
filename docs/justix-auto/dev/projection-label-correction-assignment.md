# Retained generation label correction review

2026-09-15. Status: proposal QA GREEN; canonical scope correction adopted.
Exact proposal commit: `f3fc029de3da3dbadb0cab980ef6bd33c86723b3`.
Worktree: `.worktrees/projection-label-correction-qa` (detached).
Agent: `qa_projection_label`.

Review `state/drafts/architect/projection-label-forward-correction.md` against
the installed 000002/000004 predicates, approved label clarification and T-928,
T-016 and T-934 ownership/lineage. Proposed change belongs only to the future
revision-6 forward migration and tests; old SQL and retained labels stay intact.
This task runs no production migration and approves no new admission or reset.

QA owns only `dev/qa/projection-label-forward-correction.md` and its same-named
evidence directory in the assigned checkout. Verify the real PostgreSQL catalog
feasibility with a bounded synthetic probe if needed; actual full migration and
runtime guarantees remain implementation work. Coordinator adoption and exact
canonical scope amendments follow GREEN. No source or canonical edits by QA.

## Review and adoption outcome

GREEN: [independent report](qa/projection-label-forward-correction.md), exact
proposal SHA-256 `704d2c65eedbe1e98f36bc0270d6ce366ad7cd38cccc2fe94b25e2a3ffa9f196`.
Coordinator [adoption](../state/approvals/projection-label-forward-correction.md)
adds only exact existing-task scope handoffs; T-928 implementation remains
dependent on integrated T-927 and separate independent exact-commit QA.
