# Development state

- Date: 2026-09-15
- Project: justix-auto / JustixAuto
- Target: `/Users/bakhromachilov/startups/justixauto`
- Phase: development active; Go event mechanics and shared React components in progress
- Default branch: `main`; independently reviewed tasks are integrated and pushed incrementally
- Remote: `git@github.com:achilovbakhrom/justixauto.git`
- Parent repository: `/Users/bakhromachilov/startups`; must NOT be used for tasks
- Git readiness: PASS; own root, clean main and origin synchronization verified.
  Run `bash tools/check-git.sh` before assigning application tasks.
- Confirmed: Go microservices, CQRS + Event Sourcing, Gaze reference approach;
  React; one project folder; minified four-app HTML documentation references
- Reference: `/Users/bakhromachilov/golang/gaze-executor`, HEAD
  `913c018fb2182ec943ef308da63baa85b5628bec`
- Additional reference: `/Users/bakhromachilov/gaze-executor-cc`, HEAD
  `f81b62ce346320ec229817dd73cf9ec42032e1d9`; audited 2026-09-14 in
  `../reference/gaze-executor-cc-reference.md`. User reconfirmed microservices.
  Existing seven-owner boundaries and approved reliability decisions remain.
- Frontend build tool/version: React/TypeScript/Vite/shared npm workspaces approved and locked; workspace config discovery is QA-gated
- Backend service ownership, contracts and reliability ADRs: see `../architecture.md`;
  approved on 2026-09-13. Go module/boundaries, seven PostgreSQL owners,
  restricted RabbitMQ topology, event envelopes, money values and internal
  caller authentication are integrated. Do not import trading dependencies or
  Gaze credentials.
- Package manager: npm 11.19.0 with Node 24.21.0; exact workspace lock included
- Validation: 106 mock domain tests pass against minified JS; four entries load;
  hashes, syntax and static local dependencies pass
- Execution authorization: user said "ok lets start" on 2026-09-14; proceed through bounded implementation and QA gates.
- Active task: T-002 blocked (`.worktrees/T-002`, `task/T-002-security-release`); T-037 in-progress (`.worktrees/T-037`, `task/T-037-realization-shell`); T-058 ready (`.worktrees/T-058`, `task/T-058-contract-2`); T-918 in-progress (`.worktrees/T-918`, `task/T-918-messaging-source-admission`).
- Integrated tasks: 23/924; independent exact-commit QA required before each integration.
- Reviewed PO backlog: 52 items — 42 Must, 7 Should, 3 deferred; scope confirmed by user continuation
- Formal PM task files: 924 (916 reviewed baseline + eight approved messaging amendments); see task-board.md, task-index.json and backlog-coverage.md
- Coverage: 167 acceptance clauses across 49 active backlog items; one closing verification per item
- Execution limit: three eligible workers, one browser-heavy QA; one task/branch/worktree and independent exact-SHA QA

## Laptop preflight observed

Project-local Go `1.27.1` and Node `24.21.0`/npm `11.19.0` are installed under
the ignored `docs/justix-auto/dev/local/toolchains/` directory. The pinned
wrapper enforces Go 1.27.1; use `bash tools/go.sh`, not an unqualified Go
command. Docker daemon `29.0.1` and Compose `v2.40.3-desktop.1` are available.
Pinned PostgreSQL 18.6 and RabbitMQ 4.3.5 images were verified through
disposable loopback-only QA containers; no project infrastructure remains
running after the checks.

## Commands available now

```sh
npm ci --ignore-scripts
npm run typecheck:config
npm run mocks:verify
npm run mocks:test
npm run mocks:serve
bash tools/go.sh test -race -mod=readonly ./services/... ./pkg/... ./tests/...
bash tools/go.sh vet ./services/... ./pkg/... ./tests/...
```

Historical preflight below predates the authorized Git setup on 2026-09-14.
`doctor` checks the independent Git root and initial commit.
`tools/check-git.sh` reports the detected root and exact user commands. The old
spec-team checker returned this misleading inherited-parent success:

```text
GIT CHECK OK: /Users/bakhromachilov/startups/justixauto (branch: main, dirty tree, 4 commits)
```

Read-only `git rev-parse --show-toplevel` instead returned
`/Users/bakhromachilov/startups`. The new project guard explicitly rejects that.
No changes were staged/committed in either repository and no remote was added.

Final project guard result:

```text
GIT CHECK FAILED: project needs its own Git repository: /Users/bakhromachilov/startups/justixauto
Detected Git root: /Users/bakhromachilov/startups. Do not use the parent repository.
```

Docker socket access is denied in the restricted shell; the approved read-only
doctor check outside the sandbox confirms daemon 29.0.1. This is a permission
boundary, not a missing Docker installation.

## Commands not available yet

No owner service composition roots, shared local Compose runner, owner migration
runner or CI pipeline exist yet. The Admin React workspace and shared API,
tokens, UI and test-kit packages are integrated. `npm run build:apps` builds
Admin; its default session boundary renders no protected children until session
integration. Explicit synthetic QA fixtures display the empty shell. This is
not a working business application; the other three app shells and feature
routes remain in their assigned tasks. Explicit SQL installation templates and
shared Go durability primitives are integrated. Identity now has typed owner
transaction ports, an explicit migration and fail-closed persistence readiness.
Its composition roots, auth features and custody-mode adapter remain separate
tasks; the scaffold does not start an Identity API or worker.
Broad `go test ./...` from the main checkout also discovers ignored Go sources
inside the local compiler and `node_modules`; use the project-package commands
above until tooling relocates or isolates those development artifacts. Do not
describe the repository as a running Go/React application yet.

## Next coordinator action

Architecture confirmed by user "yes" on 2026-09-13; release scope confirmed by
"conmtinue" in `../state/backlog-approval.md`. Planning is complete; do not repeat
these checkpoints or regenerate the plan. `task-board.md` is the formal plan;
`task-preview.md` remains historical examples only. Coordinator review and actual
validation results are in `../state/planning-review.md`.

Event schema/append, shared API/tokens and runtime registration convention are
integrated. The Active task field and task-index.json carry current assignments;
continue their exact-commit QA and dependency transitions. Follow the live task
dependencies for replay/receipts/outbox/inbox and the app harness, and resolve scoped
choices only before affected tasks. No policy invention. The user explicitly
authorized Git initialization and push on 2026-09-14 and removed that
prohibition from AGENTS.md.

Git readiness blocks application code, NOT documentation architecture/planning.
Policy decisions block only affected operations. Release approval, PM task
generation and the initial dependency/foundation gates are complete.
Keep current mocks immutable. Use the approved contract, not raw slice alternatives.

### Pending user decisions — asked 2026-09-15

- OD-02: Cars registry default should include all scoped company vehicles,
  including warehouse stock, or only in-work vehicles outside warehouses.
  The all-vehicles option was recommended but no response has been received.
  T-042 remains unapproved; supported owner query filters are unaffected.
- T-002 / OD-12: implementation security profile and restricted recovery contract
  proposal at task commit `a262f075bd5338c03c7049141c6c9f7e674110e5` passed
  proposal-readiness QA (`qa/T-002-r2.md`). Explicit acceptance was requested;
  no answer has been received. The proposal stays in `.worktrees/T-002` and is
  not an approved production configuration. Delivery/target benchmarks and
  all-factor-loss/pending-user activation remain separately gated.

### Historical Git diagnostic — 2026-09-13

```text
GIT CHECK FAILED: project needs its own Git repository: /Users/bakhromachilov/startups/justixauto
Detected Git root: /Users/bakhromachilov/startups. Do not use the parent repository.
User action: cd '/Users/bakhromachilov/startups/justixauto' && git init -b main && git add . && git commit -m 'Prepare development workspace'
```

This is tool output, not a command executed by the coordinator. User should review
what will be staged before creating the initial commit; parent repo is untouched.

### Approved messaging correction — 2026-09-15

The reviewed [amendment](../state/drafts/contracts/messaging-delivery.md) is
approved by [coordinator decision](../state/approvals/messaging-delivery.md).
T-917–T-924 implement recipient plans, durable intake and logical dispatch;
T-922 is the definite transaction-composable inbox prerequisite identified by QA.
No affected composition is ready until its added graph dependencies integrate.
