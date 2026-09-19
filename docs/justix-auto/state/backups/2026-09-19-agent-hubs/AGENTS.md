# JustixAuto development workspace

## Read first

1. `docs/justix-auto/dev/dev-state.md` — current phase, target root, blockers.
2. `docs/justix-auto/business-logic.md` — reconciled product rules.
3. `docs/justix-auto/open-decisions.md` — unresolved rules; never invent answers.
4. `docs/justix-auto/reference/gaze-reference.md` and
   `docs/justix-auto/reference/gaze-executor-cc-reference.md` — observed Go
   reference patterns and their distinct reliability limits.
5. `docs/justix-auto/mock-map.md` — exact UI surface and relevant source files.
6. `docs/justix-auto/dev/task-board.md`, then only the assigned task/contract.

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

When the user starts the development workflow, the primary agent coordinates
the project-scoped `architect`, `backend`, `frontend` and `qa` agents. Use at most
three concurrent workers and only independent tasks with disjoint file ownership.
Delegate a concrete bounded task while doing useful coordinator work. If agent
tools are unavailable, execute sequentially with the same gates.

- One implementation task = one branch = one Git worktree.
- Before any application code change run `bash tools/check-git.sh`; the Git root
  MUST equal this project, not the enclosing `/Users/bakhromachilov/startups` repo.
  Never commit to the parent repository.
- Task prompts contain IDs, paths and decisions, not full upstream documents.
- Workers write proposals/results to assigned files. Only the coordinator
  updates canonical docs and the board. Record SHA-256 + a recoverable snapshot
  before changing an existing canonical document. Never delete+add to replace it.
- Agent spawning alone does NOT create worktrees. The coordinator creates and
  assigns each worktree explicitly after the Git gate passes.
- QA checks the exact task commit; one browser-heavy QA at a time. Any changes
  after QA require a new check. Only the coordinator merges after QA GREEN and
  integration checks. No blind conflict resolution; preserve work on failure.
- No model override is required; inherit the user's current Codex selection.
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

This initial delivery is a documentation/tooling handoff, NOT a working Go/React
application. Follow `docs/justix-auto/dev/workflow.md`: environment preflight →
architecture approval → backlog approval → bounded implementation → independent
QA → coordinator integration. Product ambiguities block only affected tasks.
