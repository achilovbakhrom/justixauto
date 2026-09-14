# Codex development workflow

Status: prepared for this repository; application execution not started.

## 1. Environment and inputs

Run `npm run doctor` from the project root. The own-Git-root check must pass;
never accept the parent repository as a substitute. Go/Node/Docker/Codex are
installed on the laptop, but service and React dependencies are not installed.
The root package.json belongs to mock documentation tooling only.
Use `bash tools/go.sh <arguments>` for Go commands: it rejects the laptop's
older default and chooses an installed compiler >= 1.23 without PATH mutation.

Read dev-state.md, current business-logic.md, open-decisions.md and mock-map.md.
Use the Gaze source audit as technology evidence, not as product requirements.
Do not reconstruct requirements by loading every minified file into context.

## 2. Architecture checkpoint

The coordinator assigns bounded slices (identity/tenant, inventory/commercial,
retail/finance/insurance, infrastructure/events, React UI) to architect workers
in batches of at most three. They write drafts only. Resolve shared ownership,
API/event schema, concurrency, dispatch reliability, projection recovery,
authorization, storage and money rules. Technical proposals are distinguished
from business questions. Present an integrated architecture for user approval.
There is intentionally no approved architecture.md or service contract yet.

## 3. Backlog checkpoint

Derive value slices from approved rules/contracts and request scope approval.
Each task must identify: acceptance criteria; owned files; dependencies; exact
mock URL/navigation and states for FE; contracts; commands; test expectations;
result and QA paths. A blocked financial-policy slice must not block unrelated
identity or UI foundation work. No task may invent a missing partner policy.

## 4. Isolated execution

Coordinator validates Git, clean integration branch and dependency readiness;
creates one branch/worktree per task, then gives a bounded prompt to backend or
frontend. Agents do not create isolation automatically. Avoid concurrent edits
to the same shared contracts/package manifests. Workers may commit their own
task changes but must not merge or alter shared project state.

Branch convention: `task/T-NNN-short-name`; worktree under `.worktrees/T-NNN`.
Use the actual selected base branch from dev-state, never hardcode a merge into
a parent's main. If Git is dirty or ownership overlaps, preserve and report it.

## 5. Independent QA and integration

QA runs on the exact task SHA. Required checks depend on risk: replay and
aggregate invariants, DB version conflicts, authorization/tenant isolation,
idempotent command/consumer behavior, projection consistency, financial
rounding, API contracts and accessible React interactions. Browser comparisons
use the current HTML mocks; Figma is not required. Only one browser QA at once.

QA writes GREEN/BOUNCE plus commands/results and evidence. Two unsuccessful
fix cycles trigger coordinator review of scope/contract/root cause, not blind
retries. Coordinator merges only after GREEN and integration checks. Changes
after a QA SHA invalidate that result. Do not delete task branches/worktrees
until successful integration and recoverability are verified.

## 6. Durable state and restart

Coordinator updates task-board and dev-state after every status transition;
workers write only assigned result/draft files. Before updating existing
canonical documents, save their SHA-256 and recoverable copy under
`state/backups/<run-id>/`. Resume by reconciling board, task branch/SHA and QA;
never start over merely because a conversation was compacted.

Suggested task states: proposed → approved → ready → implementing → ready-for-qa
→ qa-green → integrated, with explicit blocked/bounced states and owners.
Reports include what was not tested; local demo tests do not certify production.

## Configuration provenance

Project roles are in `.codex/agents/*.toml`, cap in `.codex/config.toml`; all
inherit the user's model. No sandbox weakening, API secrets, global agent
configuration or automatic background execution. The app must trust the project
before loading project-scoped configuration. Role execution is verified only
when a development session actually spawns them; TOML parsing alone is not that
verification.

Official references inspected 2026-09-11:
[Custom agents](https://learn.chatgpt.com/docs/agent-configuration/subagents),
[Git worktrees](https://learn.chatgpt.com/docs/environments/git-worktrees).
