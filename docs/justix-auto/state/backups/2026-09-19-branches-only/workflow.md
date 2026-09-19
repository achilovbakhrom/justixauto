# Codex development workflow

Status: architecture and release planning scope approved; development authorized on 2026-09-14. See dev-state.md and task-board.md for live task status.

The user subsequently approved the two-hub packet workflow in
[agent-workflow.md](agent-workflow.md). It is authoritative for agent roles,
model routing, context limits, infrastructure authority and Git approvals.
Historical automatic main integration is retired. Human approval gates dev;
the human alone promotes dev to main/production.

## 1. Environment and inputs

Run `bash tools/check-git.sh` from the project root before application changes;
never accept the parent repository as a substitute. Go/Node/Docker/Codex are
installed. The Go module and root frontend workspace dependencies are locked
and installed; owner services and the four React apps follow their task gates.
The root package.json now contains shared frontend and mock tooling scripts.
Use `bash tools/go.sh <arguments>` for the locked Go 1.27.1 compiler and the
project-local Node 24.21.0/npm 11.19.0 via command-local PATH. See dev-state.md
for toolchain locations and the supported project-package test command.
The existing `doctor` script still prints a historical scaffold message and
uses the caller's Node/npm PATH; its output alone is not development readiness.

Read dev-state.md, current business-logic.md, open-decisions.md and mock-map.md.
Use the Gaze source audit as technology evidence, not as product requirements.
Do not reconstruct requirements by loading every minified file into context.

## 2. Architecture checkpoint

The owning application or DevOps hub assigns bounded architecture proposals
through the task_slicer and independent plan reviewer. They write drafts only.
The global delegate budget is three, including the DevOps hub. Resolve shared ownership,
API/event schema, concurrency, dispatch reliability, projection recovery,
authorization, storage and money rules. Technical proposals are distinguished
from business questions. Present an integrated architecture for user approval.
This checkpoint is complete: `architecture.md` and its two contract appendices
were approved on 2026-09-13. Read the accepted baseline and approval record;
do not re-run completed architecture slices without an explicit change request.

## 3. Backlog checkpoint

Derive value slices from approved rules/contracts and request scope approval.
This scope checkpoint passed on 2026-09-13; `dev/backlog.md` and
`state/backlog-approval.md` record the user's direction. Formal task slicing
uses that scope; it does not answer remaining financial/security/visual gates.
Each task must identify: acceptance criteria; owned files; dependencies; exact
mock URL/navigation and states for FE; contracts; commands; test expectations;
result and QA paths. A blocked financial-policy slice must not block unrelated
identity or UI foundation work. No task may invent a missing partner policy.

## 4. Isolated execution

Coordinator validates Git, integration base and dependency readiness. Slicer
writes small implementation/QA packets with full requirement coverage; independent
plan review and owning-hub acceptance precede dispatch. Version control creates
one branch/worktree per canonical task from dev and commits packet results.
Generic workers receive relevant scoped Go/React/infra rules; they do not perform
Git writes or alter shared project state. Avoid concurrent writes in one worktree
and overlapping shared contracts/package manifests. Spawning does not create isolation.

Workspace scaffold tasks T-032…T-039 explicitly own their local tsconfig and
Vitest config. Root npm lock updates for those workspace manifests are serialized
serialized handoffs before QA: inspect the manifest delta, regenerate
`package-lock.json` with approved npm/pins in that task worktree, and commit it
through version_control on the task branch before assigning its exact SHA to QA. Preserve mock lock
entries and reject unrelated dependency churn. Workers never concurrently edit
the root lock. Any lock change after QA requires renewed QA; integration does
not silently regenerate it.

Branch convention: `task/T-NNN-short-name`; worktree under `.worktrees/T-NNN`.
Normal base is dev; never merge into the parent repository. The initial HUB
bootstrap records its explicit main SHA because dev is not yet created. No
silent main fallback. If Git is dirty or ownership overlaps, preserve and report it.

## 5. Independent QA and integration

QA runs small verification packets on exact commits. Required checks depend on risk: replay and
aggregate invariants, DB version conflicts, authorization/tenant isolation,
idempotent command/consumer behavior, projection consistency, financial
rounding, API contracts and accessible React interactions. Browser comparisons
use the current HTML mocks; Figma is not required. Only one browser QA at once.

Reviewer separately checks each implementation diff. QA writes GREEN/BOUNCE
with commands/results and evidence. Two unsuccessful fix cycles trigger hub
diagnosis/reslicing or explicit model escalation. Aggregator checks acceptance
coverage and schedules small cross-packet tests at the final candidate SHA.
Historical packet results cannot replace these integration checks. Changed code
needs renewed affected checks. Source/base changes invalidate promotion evidence.
After GREEN, version_control may push its task branch and prepare dev integration.
Human approval is required before EVERY merge/push into dev. Until protected
GitHub approval enforcement is verified, the human performs integration. Main
promotion is human-only. Keep existing task worktrees and branches recoverable.

## 6. Durable state and restart

The controller stores packet/job state in the Git common directory, shared by
worktrees; only its transaction interface updates that state. Hubs recover from
compact checkpoints, immutable report files and Git. Coordinator reconciles
canonical task-index/task-board/dev-state only at canonical task transitions;
packets do not multiply permanent backlog tasks. Workers write only assigned
result/draft files. Before updating existing
canonical documents, save their SHA-256 and recoverable copy under
`state/backups/<run-id>/`. Resume by reconciling board, task branch/SHA and QA;
never start over merely because a conversation was compacted.

Suggested task states: proposed → approved → ready → implementing → ready-for-qa
→ qa-green → integrated, with explicit blocked/bounced states and owners.
Reports include what was not tested; local demo tests do not certify production.

## Configuration provenance

Project roles are in `.codex/agents/*.toml`, cap in `.codex/config.toml`; models
and reasoning are explicit per role. No sandbox weakening, API secrets, global agent
configuration or automatic background execution. The app must trust the project
before loading project-scoped configuration. Role execution is verified only
when a development session actually spawns them; TOML parsing alone is not that
verification.

Official references inspected 2026-09-11:
[Custom agents](https://learn.chatgpt.com/docs/agent-configuration/subagents),
[Git worktrees](https://learn.chatgpt.com/docs/environments/git-worktrees).
