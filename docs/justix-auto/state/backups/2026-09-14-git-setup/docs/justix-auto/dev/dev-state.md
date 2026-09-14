# Development state

- Date: 2026-09-13
- Project: justix-auto / JustixAuto
- Target: `/Users/bakhromachilov/startups/justixauto`
- Phase: architecture and release scope approved; PM planning complete; implementation not started
- Default branch: planned `main`; no project Git initialized, no initial commit
- Parent repository: `/Users/bakhromachilov/startups`; must NOT be used for tasks
- Git readiness: BLOCKED until user creates this project's own Git root + initial commit
- Confirmed: Go microservices, CQRS + Event Sourcing, Gaze reference approach;
  React; one project folder; minified four-app HTML documentation references
- Reference: `/Users/bakhromachilov/golang/gaze-executor`, HEAD
  `913c018fb2182ec943ef308da63baa85b5628bec`
- Frontend build tool/version: React/TypeScript/Vite/shared npm workspaces approved; exact supported version lock remains a prerequisite
- Backend service ownership, contracts and reliability ADRs: see `../architecture.md`;
  approved on 2026-09-13. Exact version lock is a first gated task; do not import
  trading dependencies or Gaze credentials
- Package manager: npm for documentation tooling; lockfile included
- Validation: 106 mock domain tests pass against minified JS; four entries load;
  hashes, syntax and static local dependencies pass
- Implementation tasks: all todo; no execution authorized or in progress
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

`doctor` intentionally fails the independent Git gate until setup is complete.
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

Next: user establishes the independent Git repository and initial commit, then
requests `/dev justix-auto` from the target project. Start with T-001 dependency
lock and the board's first-wave sequence; resolve scoped choices only before
their affected tasks. No automatic Git initialization or policy invention.

Git readiness blocks application code, NOT documentation architecture/planning.
Policy decisions block only affected operations. Release approval and PM task
generation are complete, but no application task is ready while ENV-GIT fails.
Keep current mocks immutable. Use the approved contract, not raw slice alternatives.

### Latest Git diagnostic — 2026-09-13

```text
GIT CHECK FAILED: project needs its own Git repository: /Users/bakhromachilov/startups/justixauto
Detected Git root: /Users/bakhromachilov/startups. Do not use the parent repository.
User action: cd '/Users/bakhromachilov/startups/justixauto' && git init -b main && git add . && git commit -m 'Prepare development workspace'
```

This is tool output, not a command executed by the coordinator. User should review
what will be staged before creating the initial commit; parent repo is untouched.
