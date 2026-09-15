# Development state

- Date: 2026-09-15
- Project: justix-auto / JustixAuto
- Target: `/Users/bakhromachilov/startups/justixauto`
- Phase: development active; eight QA-gated foundation tasks integrated; event-store and shared frontend slices next
- Default branch: `main`; integrated foundation state `5a74db0` pushed to origin
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
- Active task: T-002 ready-for-qa (`.worktrees/T-002`, `task/T-002-security-release`); T-009 implementing (`.worktrees/T-009`, `task/T-009-append`); T-032 bounced (`.worktrees/T-032`, `task/T-032-api-client`); T-033 ready-for-qa (`.worktrees/T-033`, `task/T-033-tokens`); T-746 implementing (`.worktrees/T-746`, `task/T-746-runtime-registration-contract`).
- Integrated tasks: 9/916; independent exact-commit QA required before each integration.
- Reviewed PO backlog: 52 items — 42 Must, 7 Should, 3 deferred; scope confirmed by user continuation
- Formal PM task files: 916; see task-board.md, task-index.json and backlog-coverage.md
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
bash tools/go.sh test -race -mod=readonly ./pkg/... ./tests/...
bash tools/go.sh vet ./pkg/... ./tests/...
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

No owner service composition roots, shared local Compose runner, migrations,
CI pipeline, or React application workspaces exist yet. Root React test/build
commands intentionally fail until child app/package workspaces are added.
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

Next eligible foundation work starts with T-008 owner-installable event/command
SQL templates, followed by conditional append/replay/outbox/inbox mechanics.
Independent frontend work may proceed through T-032 schema-validating API client
and T-033 mock-token extraction. Follow task dependencies and resolve scoped
choices only before affected tasks. No policy invention. The user explicitly
authorized Git initialization and push on 2026-09-14 and removed that
prohibition from AGENTS.md.

Git readiness blocks application code, NOT documentation architecture/planning.
Policy decisions block only affected operations. Release approval, PM task
generation and the initial dependency/foundation gates are complete.
Keep current mocks immutable. Use the approved contract, not raw slice alternatives.

### Historical Git diagnostic — 2026-09-13

```text
GIT CHECK FAILED: project needs its own Git repository: /Users/bakhromachilov/startups/justixauto
Detected Git root: /Users/bakhromachilov/startups. Do not use the parent repository.
User action: cd '/Users/bakhromachilov/startups/justixauto' && git init -b main && git add . && git commit -m 'Prepare development workspace'
```

This is tool output, not a command executed by the coordinator. User should review
what will be staged before creating the initial commit; parent repo is untouched.
