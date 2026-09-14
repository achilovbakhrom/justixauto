# Development planning review

2026-09-13. Documentation planning only; no application implementation or QA execution.

## Outcome

Release scope confirmed in `backlog-approval.md`: 42 Must + 7 Should, with three
Icebox items excluded. The formal plan contains 916 tasks mapped to 167 acceptance
clauses across 49 active backlog items. All tasks are todo; none assigned, built,
reviewed as application code, or merged. The older 24-unit preview is not the board.

Each task has bounded ownership, dependencies, branch name, acceptance contribution,
verification commands and developer/independent QA output paths. Estimates are
at most four effort-hours assuming predecessors exist; developers must reslice
if actual scope exceeds that. These are estimates, not measured durations.

## Review corrections

Independent PM review identified blanket runtime/policy dependencies, missing
generated-client and storage/projector work, private admission ownership gaps,
incomplete owner-local operation lookup, cash-flow coupling and oversized slices.
The PM revision resolved these by splitting concrete deliverables and minimal
runtime assembly. Coordinator follow-through added precise counterpart runtime
dependencies to integrating acceptance (not client unit tests), core provider
account entry, generated-output edges, and T-915/T-916 for the private fulfillment
intent schema/client. No new product behavior or policy values were invented.

Runtime acceptance closes over registrations of its required backend ancestors;
it never waits for every feature of an owner. Schema-valid isolated tests remain
distinct from combined cross-owner runtime acceptance. Reviews are bounded
planning reviews, not a guarantee that implementation will need no reslicing.

## Actual checks

- Structural validator: 916 task files, 49 parents, 167 mapped acceptance clauses;
  acyclic graph; no unordered file-ownership collisions; no errors or warnings.
- Readiness regression validator: 1,016 checks passed, covering unrelated policy
  barriers, required generated/runtime producers and acceptance registration closure.
- Reference integrity: 57 mock hashes, JavaScript syntax and local dependencies pass.
- Existing mock regression suite: 106 passed, zero failed. These are not Go/React tests.
- Git guard: fails because target inherits `/Users/bakhromachilov/startups`.
  No Git initialization, staging, commits, infrastructure startup or deployment performed.

Graph metrics: 45 dependency levels; maximum earliest-rank width 150;
effort-weighted critical chain 152 task-effort hours. These ignore gates and worker
contention and are not delivery estimates. Actual execution cap is three workers,
one browser-heavy QA. The board and index contain the exact critical path.

## Preservation and handoff

Before canonical updates, SHA-256/size manifests and recoverable originals were
verified under `state/backups/2026-09-13-plan-pm-r1/`. Promotion updates only the
reviewed documentation; no deletion or replacement of business logic/mocks.
Audit scripts/results and historical review reports are retained in
`state/planning-evidence/`; older review findings are superseded only by the final
report and these explicit corrections.

Next: establish the project's own Git root and initial commit, then request
`/dev justix-auto`. Start with T-001 dependency lock and the board's first-wave
guide. Scoped policy, security and missing-UI decisions still require their named
approvals before the affected implementation; they do not block unrelated tasks.
