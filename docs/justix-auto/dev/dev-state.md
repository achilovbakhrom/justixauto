# Development state

- Date: 2026-09-14
- Project: justix-auto / JustixAuto
- Target: `/Users/bakhromachilov/startups/justixauto`
- Phase: development authorized; dependency lock integrated; Go/frontend foundation execution next
- Default branch: `main`; initial commit `e93c6d6260f7567da631b8a0217169c215a5b459` pushed to origin
- Remote: `git@github.com:achilovbakhrom/justixauto.git`
- Parent repository: `/Users/bakhromachilov/startups`; must NOT be used for tasks
- Git readiness: PASS; own root and initial commit verified; `npm run doctor` passes.
  Run `bash tools/check-git.sh` before assigning application tasks.
- Confirmed: Go microservices, CQRS + Event Sourcing, Gaze reference approach;
  React; one project folder; minified four-app HTML documentation references
- Reference: `/Users/bakhromachilov/golang/gaze-executor`, HEAD
  `913c018fb2182ec943ef308da63baa85b5628bec`
- Additional reference: `/Users/bakhromachilov/gaze-executor-cc`, HEAD
  `f81b62ce346320ec229817dd73cf9ec42032e1d9`; audited 2026-09-14 in
  `../reference/gaze-executor-cc-reference.md`. User reconfirmed microservices.
  Existing seven-owner boundaries and approved reliability decisions remain.
- Frontend build tool/version: React/TypeScript/Vite/shared npm workspaces approved; exact supported version lock remains a prerequisite
- Backend service ownership, contracts and reliability ADRs: see `../architecture.md`;
  approved on 2026-09-13. Exact version lock is a first gated task; do not import
  trading dependencies or Gaze credentials
- Package manager: npm for documentation tooling; lockfile included
- Validation: 106 mock domain tests pass against minified JS; four entries load;
  hashes, syntax and static local dependencies pass
- Execution authorization: user said "ok lets start" on 2026-09-14; proceed through bounded implementation and QA gates.
- Active task: T-007 bounced (`.worktrees/T-007`, `task/T-007-envelope`).
- Integrated tasks: 6/916; independent exact-commit QA required before each integration.
- Reviewed PO backlog: 52 items — 42 Must, 7 Should, 3 deferred; scope confirmed by user continuation
- Formal PM task files: 916; see task-board.md, task-index.json and backlog-coverage.md
- Coverage: 167 acceptance clauses across 49 active backlog items; one closing verification per item
- Execution limit: three eligible workers, one browser-heavy QA; one task/branch/worktree and independent exact-SHA QA

## Laptop preflight observed

Node `v22.23.0`; npm `10.9.8`; Go `go1.24.2 darwin/arm64`; Docker Compose
`v2.40.3-desktop.1`; Docker daemon `29.0.1`; Codex CLI `0.154.0`.
Target-folder default Go resolves to `/usr/local/go/bin/go` 1.22.2; the
project-local `bash tools/go.sh` chooses installed `/opt/homebrew/bin/go` 1.24.2
to meet the reference minimum. Use the wrapper, not an unqualified Go command.
Gaze declares Go `1.23.0`; target compiler/module/container pins need a recorded
architecture decision. No compiler upgrade, service images or app dependencies
were installed. Root node dependencies are minification tools only.

## Commands available now

```sh
npm ci --ignore-scripts
npm run mocks:verify
npm run mocks:test
npm run mocks:serve
npm run doctor
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

No Go test/build/migration targets or React dev/build targets exist. Infrastructure
Compose and CI pipelines are not implemented. Do not substitute mock tests for
application tests or describe the repository as a running Go/React app.

## Next coordinator action

Architecture confirmed by user "yes" on 2026-09-13; release scope confirmed by
"conmtinue" in `../state/backlog-approval.md`. Planning is complete; do not repeat
these checkpoints or regenerate the plan. `task-board.md` is the formal plan;
`task-preview.md` remains historical examples only. Coordinator review and actual
validation results are in `../state/planning-review.md`.

Next: review T-001's exact dependency lock, independently QA its commit, then
start T-003/T-004 in parallel worktrees. Follow the board's first-wave sequence; resolve scoped choices only before
their affected tasks. No policy invention. The user explicitly authorized Git
initialization and push on 2026-09-14 and removed that prohibition from AGENTS.md.

Git readiness blocks application code, NOT documentation architecture/planning.
Policy decisions block only affected operations. Release approval and PM task
generation are complete; exact dependency pins still precede scaffold work.
Keep current mocks immutable. Use the approved contract, not raw slice alternatives.

### Historical Git diagnostic — 2026-09-13

```text
GIT CHECK FAILED: project needs its own Git repository: /Users/bakhromachilov/startups/justixauto
Detected Git root: /Users/bakhromachilov/startups. Do not use the parent repository.
User action: cd '/Users/bakhromachilov/startups/justixauto' && git init -b main && git add . && git commit -m 'Prepare development workspace'
```

This is tool output, not a command executed by the coordinator. User should review
what will be staged before creating the initial commit; parent repo is untouched.
