# JustixAuto development workspace

## Read first

1. `docs/justix-auto/dev/agent-workflow.md` — current authority, dispatch and Git rules.
   `docs/justix-auto/dev/dev-state.md` — current phase, target root, blockers.
2. `docs/justix-auto/business-logic.md` — reconciled product rules.
3. `docs/justix-auto/open-decisions.md` — unresolved rules; never invent answers.
4. `docs/justix-auto/reference/gaze-reference.md` and
   `docs/justix-auto/reference/gaze-executor-cc-reference.md` — observed Go
   reference patterns and their distinct reliability limits.
5. `docs/justix-auto/mock-map.md` — exact UI surface and relevant source files.
6. Coordinator: `docs/justix-auto/dev/task-board.md`, then the selected task.
   Delegates: only their assigned packet, referenced contract sections and scoped
   rules; the packet supplies the relevant product/open-decision IDs. Do not load
   the full board or full conversation into every worker.

Newest explicit user decisions override historical documentation. Mock fixtures
are interaction examples, not authorization, financial policy or legal contracts.
No Figma prerequisite: the user selected HTML mocks as the design reference.

## Architecture constraints

- Backend: Go microservices with Gaze-style hexagonal architecture, CQRS,
  Event Sourcing, aggregate replay, projections and event-driven subscribers.
  Use `bash tools/go.sh` to select an installed compiler meeting Go >= 1.23.
- Frontend: React; four apps: Realization, Financing, Insurance and Admin.
- Replicate the reference's approach, not its trading features or credentials.
  Record changes to dependencies and operational guarantees in approved ADRs.
- Service/data ownership, event contracts and reliability semantics must be
  approved before dependent implementation tasks can become ready.
- Never share mutable business tables between services by accident. Money uses
  approved fixed-precision rules, not copied browser floating-point demo math.

## Coordinator and agents

The primary session is the application/delivery orchestrator. The
`devops_orchestrator` is the infrastructure authority, including Kubernetes and
CI/CD. Both use `task_slicer`, `worker`, `reviewer`, `qa`, `qa_aggregator` and
`version_control` capabilities. Technology rules live in scoped AGENTS.md files.
The primary owns scheduling and canonical project records; DevOps owns recorded
infrastructure decisions. Cross-boundary contracts require both authorities.
Use at most three concurrent delegates TOTAL across both hubs, including the
DevOps hub itself. Nested spawning must reserve capacity with the primary first;
where unavailable, the primary dispatches on DevOps' behalf. Leaf agents never
spawn children. Only one writer per worktree and one holder per exclusive
resource (browser, PostgreSQL fixture). If tools cannot enforce isolation, run
sequentially with the same review/QA gates.

- One implementation task = one branch = one Git worktree.
- Before any application code change run `bash tools/check-git.sh`; the Git root
  MUST equal this project, not the enclosing `/Users/bakhromachilov/startups` repo.
  Never commit to the parent repository.
- Task prompts contain IDs, paths and decisions, not full upstream documents.
  Start each packet/review/QA with a fresh context. Explicitly select the role's
  model and reasoning; never silently inherit an expensive coordinator model.
  Use file artifacts and compact checkpoints after interruptions/compaction.
- Workers write proposals/results to assigned files. Only the coordinator
  updates canonical docs and the board. Record SHA-256 + a recoverable snapshot
  before changing an existing canonical document. Never delete+add to replace it.
- Agent spawning alone does NOT create worktrees. `version_control` creates
  and assigns each worktree after the Git gate and orchestrator instruction.
- Slicer writes bounded packets and acceptance coverage. Independent plan review
  precedes dispatch. Worker writes code; version_control commits; reviewer checks
  the exact diff; QA verifies small behavior packets. Aggregation schedules
  cross-packet checks at the final SHA; a list of earlier GREEN reports is not
  cumulative verification. Reports cannot approve their own implementation.
- Version control may commit/push task, feature, fix and infra branches. Every
  merge/push into `dev` requires human approval bound to the source SHA and tested
  integration base. `main` is production: promotion is HUMAN ONLY. The latest
  user policy overrides older tasks/skills that auto-merge to main. No force
  pushes, automatic conflict resolution or production deployment by agents.
- Normal branches start from `dev`. Initial HUB bootstrap is explicitly based
  on main SHA 682696f2fea8e874cc9572749f9faadb2b532b4e because dev does not yet
  exist. Its creation/push waits for human approval; this is not a general fallback.
- Keep status in `docs/justix-auto/dev/`; recover from files after context changes.
  Never mark unfinished architecture, tasks or implementation complete.

## Safety and scope

Use apply_patch for edits; preserve user changes. No production actions, secrets,
external messages, deployments or destructive cleanup without explicit authority.
Do not read `.env`/keys from Gaze. Do not copy trading code into this product.
Do not overwrite or delete the original spec-team documentation/mocks.
Generated minified files under `docs/justix-auto/mocks/` are reference artifacts,
not editable application source. Read targeted symbols or run them in a browser.
Passwords/local demo accounts in mocks must never become production auth.

## Phase gates

The application remains incomplete. Follow `docs/justix-auto/dev/workflow.md`:
approved canonical task → slicer → independent plan review → packet execution,
code review and QA → integration QA → human approval → dev integration.
Infrastructure rollout and GitOps selection require an infrastructure decision
and environment scope; Kubernetes intent alone does not authorize a deployment.
Product ambiguities block only affected tasks.
