# ADR-16 — GitHub Actions CI runs `make check` for dev and main

- Date: 2026-09-24
- Status: **Proposed** by the infrastructure authority (devops_orchestrator).
  Becomes **Accepted** only when the user approves it. Scope already fixed by
  the user this session: the checks (the full `make check`), the triggers (pull
  requests into and pushes to `dev`/`main`, nothing else), and "lefthook covers
  local work; CI is the enforcing gate for dev".
- Builds on: ADR-13 (exact pins, no unchecked `latest`), [ADR-15](adr-15-kubernetes-multi-replica-observability.md)
  (delivery stays outside CI), [agent-workflow.md](dev/agent-workflow.md) "Git and human gate"
- Workflow: [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)

## Decision

| Concern | Choice |
|---|---|
| Tool | GitHub Actions, one workflow `CI`, standard GitHub-hosted runners |
| Triggers | `pull_request` into `dev`/`main`; `push` to `dev`/`main`. No `pull_request_target`, `workflow_dispatch`, schedule, tag or path filters |
| What runs | `make check`, split into two parallel jobs (below). The Makefile is the single definition: CI calls make targets and never copies their commands |
| Runner | `ubuntu-24.04`: a pinned label, not the moving `ubuntu-latest`. Docker, openssl, curl, gcc (for `-race`) are preinstalled |
| Toolchains | Go `1.27.1` via setup-go (`check-latest: false`). `tools/go.sh` still enforces the exact version with `GOTOOLCHAIN=local`. Node from `package.json` `engines.node` (24.21.0, which bundles npm 11.19.0, checked in the nodejs.org release index) |
| Tools | golangci-lint, deadcode and swag run as `go tool` from the pinned modules. Nothing is installed globally |
| npm install | `make web-install` = `npm ci --ignore-scripts`. This skips `prepare` (lefthook hooks have no use in CI) and third-party install scripts. The lockfile needs none: lefthook, msw (no `workerDirectory`), fsevents (macOS only) |
| Actions | Pinned to full commit SHAs, with the tag in a comment: checkout v7.0.1, setup-go v7.0.0, setup-node v7.0.0, cache v6.1.0. Upgrades are deliberate, reviewed edits |
| Permissions | `contents: read` for the whole workflow; checkout with `persist-credentials: false`. No secrets, environments or cloud credentials |
| Concurrency | One group per workflow and ref. A new PR commit cancels the PR's running check. A running push check on `dev`/`main` is never cancelled because it is integration evidence; only a queued older one is replaced |
| Caching | setup-go (modules + build cache). The static job keys on `go.sum` + `tools/lint/go.sum`; the Go test job keys on `go.sum`, so the two jobs do not overwrite each other. golangci-lint cache: `~/.cache/golangci-lint`, keyed on both `go.sum` files + `.golangci.yml`, with a prefix fallback. npm cache: keyed on `package-lock.json`. Pushes to `dev`/`main` seed caches that PRs into them restore. PR caches (forks included) stay in their PR's scope |
| Timeouts | 20 min static, 25 min Go tests (instead of the 6 h default) |
| CI-only guard | After the checks, `git diff --exit-code`: the checks must not change tracked files. This catches `swag fmt` rewriting annotations of a commit made with `--no-verify`, which `openapi-check` (spec only) misses |

## Jobs and triggers

| Job (= check name) | Runs | Needs |
|---|---|---|
| `Lint, typecheck and web tests` | `make web-install`; `make lint` (OpenAPI freshness, golangci-lint, deadcode, ESLint, Prettier check, knip); `make typecheck`; `make test-web`; tree-unchanged guard | Go, Node |
| `Go tests with PostgreSQL and MinIO` | `make test-go` = `tools/test-go.sh`: `go test -race -count=1 -p 1 ./...` against throwaway PostgreSQL 18.6 (digest-pinned) and MinIO containers | Go, Docker |

Typecheck and web tests still run when lint fails, as long as `npm ci`
succeeded, so one run reports every static failure.

| Event | Target | Jobs | Superseded run |
|---|---|---|---|
| `pull_request` (opened, synchronize, reopened) | base `dev` or `main` | both, on GitHub's merge commit (source + current target) | cancelled |
| `push` | `dev`, `main` | both | running: kept; queued: replaced |
| anything else | — | none | — |

A PR run tests the merge commit, which is the "tested integration base" that human
approval binds to. If the target branch moves, the result is stale until
re-run (see "require branches to be up to date" below).

## Runtime and runner cost

The repository is public (checked 2026-09-24). Standard GitHub-hosted Linux
runners for public repositories have 4 vCPU and 16 GB and are free: they
consume no plan minutes. The following are estimates, not yet measured in CI;
replace them with the first real runs.

| Job | Cold caches | Warm caches |
|---|---|---|
| Lint, typecheck and web tests | 6–9 min | 4–6 min |
| Go tests (≈3 min locally) | 6–9 min | 4–7 min |

Wall clock per run ≈ the slower job, about 5–9 min; about 10–18 runner-minutes
per run.

If the repository becomes private, runners drop to 2 vCPU (slower; then watch
golangci-lint's 5 min `run.timeout` on a cold cache). Every minute would then
count against the plan's included minutes (at the time of writing: GitHub Free
2,000/month, Pro and Team 3,000; verify the current plan) and is billed after
that. For example, 100 runs a month use about 1,500 minutes. Cache storage is
limited to 10 GB per repository. Keys change only with `go.sum`, the lockfile
or the lint config, so few entries accumulate.

## Prerequisite: the MinIO test image (blocks the Go test job)

`tools/test-go.sh` starts `docker.io/minio/minio:latest`. That repository no
longer exists on Docker Hub (checked 2026-09-24: Hub API 404, anonymous registry
pull 401). Local runs work only because the image is cached on the developer
machine (built 2025-09-07). On a clean runner the Go test job fails at
`docker run`. An unpinned `latest` also contradicts ADR-13.

Decision: the test S3 fixture uses the same image, pinned by digest, from the
registry MinIO still serves:

```bash
minio_image="quay.io/minio/minio:RELEASE.2025-09-07T16-13-09Z@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e"
```

This digest is the same manifest as the locally cached `minio/minio:latest`, so
local behaviour does not change. Anonymous pulls from quay.io work. It is a
frozen upstream release used only as a test fixture; production uses AWS S3
(ADR-15). If quay.io stops serving it, replacing MinIO with another
S3-compatible test server is a separate dependency decision.

The primary schedules this one-line change to `tools/test-go.sh`, which is
outside this ADR's files. Until it lands, the Go test job fails on a clean
runner, and CI should not be made a required check before then. The local kind
stack has the same source problem (`infra/kind/deps.yaml` and
`infra/kind/up.sh` use `minio/minio:latest` from the local Docker cache). That
is a follow-up, not part of CI.

## Out of scope

- Delivery of any kind: image build or push, registry, Helm, `kubectl`, kind,
  Argo CD (still only a candidate that needs its own ADR). CI touches no cluster.
- Promotion. Merging into `dev` needs human approval of the exact SHA;
  `dev` → `main` (production) is human-only. CI produces evidence; it never
  merges, tags or releases.
- Secrets, environments, OIDC or cloud credentials, `pull_request_target`,
  self-hosted or larger runners, merge queue, scheduled runs, path filters.
- Automatic updates of pinned actions (Dependabot or Renovate): not configured,
  and would need a separate decision.
- GitHub repository settings. This ADR enables nothing on GitHub.

## Making CI the enforcing gate (human steps after approval)

The workflow reports results but blocks nothing until the user configures
branch protection or rulesets on `dev` and `main`:

- require a pull request with a human review, dismiss stale approvals, and allow
  no bypass for agent identities;
- required status checks `Lint, typecheck and web tests` and
  `Go tests with PostgreSQL and MinIO`, with "require branches to be up to date",
  so the tested base is the merge base;
- no force pushes or deletions;
- keep "require approval for fork pull request workflows" (the repository is
  public). Fork runs get a read-only token and no secrets.

## Consequences

- `make check` remains the single definition of the gate. Changing its targets
  changes CI; CI adds only the tree-unchanged guard.
- Version bumps: Go in `tools/go.sh`, `go.mod`, `tools/lint/go.mod` and
  `ci.yml` (a mismatch fails loudly in `tools/go.sh`); Node only in
  `package.json` `engines`.
- The `ubuntu-24.04` label and the action SHAs change only through a reviewed
  edit. Renaming a job requires updating the required-check names.
- `go test -p 1` runs packages one at a time against one database, so Go test
  time grows with the suite. Parallel databases would be a test-harness
  decision; they are not needed yet.
