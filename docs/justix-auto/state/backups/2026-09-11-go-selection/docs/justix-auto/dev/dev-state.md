# Development state

- Date: 2026-09-11
- Project: justix-auto / JustixAuto
- Target: `/Users/bakhromachilov/startups/justixauto`
- Phase: documentation/tooling handoff prepared; architecture and implementation not started
- Default branch: planned `main`; no project Git initialized, no initial commit
- Parent repository: `/Users/bakhromachilov/startups`; must NOT be used for tasks
- Git readiness: BLOCKED until user creates this project's own Git root + initial commit
- Confirmed: Go microservices, CQRS + Event Sourcing, Gaze reference approach;
  React; one project folder; minified four-app HTML documentation references
- Reference: `/Users/bakhromachilov/golang/gaze-executor`, HEAD
  `913c018fb2182ec943ef308da63baa85b5628bec`
- Frontend build tool/version: pending architecture; React confirmed, TypeScript/Vite proposed only
- Backend module path, pins, deployment boundaries, DB ownership, reliability ADRs:
  pending architecture; do not import all trading dependencies or Gaze credentials
- Package manager: npm for documentation tooling; lockfile included
- Validation: 106 mock domain tests pass against minified JS; four entries load;
  hashes, syntax and static local dependencies pass
- Implementation tasks: none approved or in progress

## Laptop preflight observed

Node `v22.23.0`; npm `10.9.8`; Go `go1.24.2 darwin/arm64`; Docker Compose
`v2.40.3-desktop.1`; Docker daemon `29.0.1`; Codex CLI `0.154.0`.
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

## Commands not available yet

No Go test/build/migration targets or React dev/build targets exist. Infrastructure
Compose and CI pipelines are not implemented. Do not substitute mock tests for
application tests or describe the repository as a running Go/React app.

## Next coordinator action

After Git readiness, open this folder in Codex and begin architecture slices,
then request user approval. Read business/open-decision docs first. Scope blockers
to affected slices (e.g. payments/policy issuance), and leave unrelated foundations
available for planning. Workflow and task template are adjacent to this file.
