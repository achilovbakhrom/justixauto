# T-DEVTOOL — One Go dev tool instead of bash scripts; remove the local Kubernetes stack

- Status: proposed (sliced 2026-09-25; awaiting independent plan review and hub acceptance)
- Kind: repository tooling change (like the HUB setup). It is **not** one of the 949
  product backlog tasks and adds no backlog ID; the primary records it on the board.
- Authority: primary (application/delivery hub) owns scheduling and acceptance.
  P5 deletes `infra/kind/` and edits a `deploy` comment: infrastructure scope, so
  the devops_orchestrator acknowledgement is recorded at plan acceptance (cross-boundary
  rule). The user's decision below is the authority for the removal itself.
- Depends on: nothing. Branch `chore/go-devtool`, main checkout
  `/Users/bakhromachilov/startups/justixauto`, base SHA
  `5f37324920fccc5a2755026ade19941916f96b83` (from `dev`). No worktrees.
- Owner/agent: unassigned (packets below). Planner: task_slicer.
- Approved scope: user decisions 2026-09-25 (UD-1..UD-3 below). No product rules,
  no business-logic or open-decision IDs are involved.
- Mock entry URL: not applicable (no UI).
- Files owned: see each packet; union = `tools/devtool/**`, `tools/check-git.sh`,
  `tools/go.sh`, deletions of `tools/test-go.sh`, `tools/doctor.sh`,
  `tools/openapi-staged.sh`, `Makefile`, `lefthook.yml`, `package.json` (`scripts.doctor`
  only), `internal/e2e/e2e.go` (one skip string), `internal/modules/documents/repository/s3_test.go`
  (one skip string), deletion of `infra/kind/**`, `README.md`, `infra/README.md`,
  `.vscode/launch.json` (new), `deploy/values-dev.yaml` (header comment only).
- NOT owned by any packet (primary follow-up, section 10): ADR-15, ADR-16, `AGENTS.md`,
  `internal/AGENTS.md`, `tools/AGENTS.md`, `docs/justix-auto/dev/*` canonical records,
  `docs/justix-auto/architecture.md`, `.claude/**`, `.codex/**`, `.github/workflows/ci.yml`
  (no change needed, see D-12), `deploy/values-prod.yaml`, chart templates.

## 1. Binding user decisions (2026-09-25)

- UD-1 Tooling logic is Go. Only `tools/go.sh` stays shell (it selects the pinned Go
  1.27.1 compiler); trim it if possible, keeping: `GOTOOLCHAIN=local`, `JUSTIX_GO_BIN`
  override, candidate paths, exact-version check, clear error.
- UD-2 No local Kubernetes: delete `infra/kind/` entirely, the `k8s-up`/`k8s-down`
  Makefile targets and the README / infra README kind sections. Keep `make image`.
  Laptop workflow: `make env`, `make db-up`, `make migrate`, `make api`,
  `make web APP=…`, plus a minimal, documented `.vscode/launch.json`.
- UD-3 Kubernetes stays for servers: `deploy/helm/justixauto/` and `values-prod.yaml`
  are untouched. `values-dev.yaml`: slicer recommendation adopted in D-8.
  (UD-5 moves the chart; its content stays untouched.)

Added by the user 2026-09-25 after slicing (primary amendment, packet P6):

- UD-4 Remove the legacy hub helpers `tools/agent-flow*` and `tools/agent-git*`
  (scripts, tests, README folders). The packet ledger is unused since 2026-09-20 and
  the only allowed agent-git operations were thin Git wrappers. The local ledger in
  `.git/justix-agent-state` is not tracked and stays as history.
- UD-5 `deploy/` holds only this project's chart: move `deploy/helm/justixauto/*`
  to `deploy/` (chart root `deploy/Chart.yaml`), no content change.
- UD-6 `deploy/` is the only infrastructure folder: keep `infra/local` as
  `deploy/local/compose.yaml` (third-party services; backend and frontend run on the
  laptop), move `infra/AGENTS.md`/`CLAUDE.md` to `deploy/`, delete the rest of `infra/`
  (incl. `infra/kind`, so P5's deletion and `infra/README.md` edit are done by the
  primary here; `deploy/README.md` replaces it), drop `k8s-up`/`k8s-down`, add
  `deploy/.helmignore`. P5 keeps: README laptop workflow, `.vscode/launch.json`,
  `values-dev.yaml` header comment.
- UD-7 (2026-09-25) Server environments are dev and prod; the laptop uses
  `deploy/local`. `values-dev.yaml` is rewritten by the primary as a dev-server
  template (registry image with required tag/digest, HTTPS ingress, S3 via IRSA,
  REPLACE placeholders), superseding D-8 and every P5 item/check on `values-dev.yaml`.

## 2. Source hashes (SHA-256 at base 5f37324)

| File | SHA-256 |
|---|---|
| AGENTS.md | 3aec643378885a8090dbac003ff2754db30c8d3183e1132f0c65ee34d290f792 |
| docs/justix-auto/dev/agent-workflow.md | ba4d7ef077c0ad57f18739955bbd7cda35e7600b75a23c2fbc561001fcddf892 |
| docs/justix-auto/dev/task-template.md | 29ee162b1182d63cb99a0ace5a385e0ffbafa5521488d5ea28e4e0c266afbe06 |
| tools/AGENTS.md | 6ccebe156aff7d7912e5c427c0e8f550185651ea0995b6f4e4955ac839cc080c |
| tools/go.sh | 21d8dcf9580088529a7a34a9ba4fc0bd6a56646a5c5e7fe7f79b66303d0d92c3 |
| tools/check-git.sh | 6c6375ec50d6d580ea7a411b7985f13926582b92465b38d4f8df5dfb721ac8a4 |
| tools/test-go.sh | bae447e7134e0c6d4e13221b3a8551bc1e1ea570a1c8d8588df81183238e934b |
| tools/doctor.sh | b90934da907f8f9701b4e27a5dac824475e1809f696eaf49520f6e92a1f3e2d2 |
| tools/openapi-staged.sh | 652009df10682106e18f3e96b247228bef8ad60981fe4b6bd77a1c23fc47973b |
| Makefile | 6047f6e4fd85b2c94a88ea286466a37f2c8913b15d808d3fb882ad71bb83ad7c |
| lefthook.yml | 7691810ddd69f6cd3672e5198beb29b66f188e2b336f604740991ab47f45d97b |
| package.json | c63db7ea8ecaa4f70169870f0da84ae262a6691a23c3187f9014e5d1aa6dd9e2 |
| .github/workflows/ci.yml | 233f650b156eb8aaa9443b40dcc00fcea1d26acd373c7a76873fabf44b170b5f |
| README.md | 73acc0f20df5120af2706d44a0ece8c05a6b921a684e857fed6a1aa8bec68d12 |
| infra/README.md | 2bc2d87019cd2d2a658417d659f93104e43fe57cbfa98a761f57a5cdc7a1e568 |
| infra/AGENTS.md | 28d03b9e6d5971860a9e4376d2b9c18d086c06a212697fbb7031244b8171f23a |
| infra/local/compose.yaml | a5be8a6d5ceafa9780cd4e4dd78c2f08efcef093825f22df102d0b4029c452c7 |
| .env.example | cb00fe986300f9d9baa6aef22a86497d822813b060fa64a8e742ce10311d19c1 |
| deploy/helm/justixauto/values-dev.yaml | 5080fe69f76213dfa51b54858ad5425b5be2c8e4510821aa803bdee464a1ff9e |
| .golangci.yml | 81145b7de9d8623ae6ca3b66cc38ce95f43da77a341fa1bf5e879839e139dab9 |
| docs/justix-auto/adr-15-kubernetes-multi-replica-observability.md | d5647ec623de3c83e8c5e513e8ecb078e87e2405644ded8c7d5b045f94792040 |
| docs/justix-auto/adr-16-github-actions-ci.md | be4f153978ab2d01c95629bcf481c2ec0667949590f55d763c36e9f3b2e9022d |
| internal/AGENTS.md | 345caa73ab2e4a1089bc6a9b6a59494328115ac544946bec6cc400c4e838b096 |
| internal/e2e/e2e.go | dd25a958113eea3e03b407831cca2a1c0a982ff19a9d603f2eafabbd6b6991cf |
| internal/modules/documents/repository/s3_test.go | d8290869cdc11a944caf7e770c4283790128564423e44ed43d15cdc817d878f8 |
| go.mod | 053c6c6a13987e5bc48d287005e9d2525b58a136f2c9bbe2b5dfe7741049dd89 |
| .gitignore | 4f0fb822fe91fab9b22c3c4af12ff6289c84b603448e83da1d60ed8d79ccb66c |

A packet whose input file no longer matches (other than changes made by an earlier
accepted packet of this task) stops with `STALE_INPUT`.

## 3. Slicer decisions (settled; reviewer may challenge)

- **D-1 Keep `tools/check-git.sh` as a compatibility shim.** 786 canonical task files
  under `docs/justix-auto/dev/tasks/`, `AGENTS.md:54`, `architecture.md:37`,
  `dev-state.md:17`, `workflow.md:13`, `.claude/agents/worker.md:14` and
  `.codex/agents/worker.toml:10` run `bash tools/check-git.sh`. Rewriting them is
  canonical-record churn outside this task. The shim holds no logic.
- **D-2 The shim builds and execs; it does not `go run`.** Verified in the pinned
  toolchain source (`src/cmd/go/internal/run/run.go`: "The exit status of Run is not
  the exit status of the compiled binary"; `base.Errorf` sets exit status 1): `go run`
  turns every non-zero exit into 1 and adds an `exit status N` line on stderr. Exit
  codes 4/5 therefore require `go build -o var/bin/devtool` + `exec`. It uses
  `-buildvcs=false` because VCS stamping fails precisely in the exit-4 (no own repo)
  and exit-5 (no commit) situations. `var/` is git-ignored and docker-ignored.
- **D-3 All other entry points use `bash tools/go.sh run ./tools/devtool <cmd>`**
  (Makefile, lefthook, `package.json`). They only need zero/non-zero.
- **D-4 devtool runs Go through `bash <root>/tools/go.sh …` (argv)**, never a `go`
  from PATH, so the pinned-compiler guarantees hold wherever it is started.
- **D-5 Project root** = nearest ancestor of the working directory (inclusive) that
  contains a `go.mod` whose `module` directive is exactly `justixauto`, with symlinks
  resolved (`filepath.EvalSymlinks`). `go.mod` files of other modules (e.g.
  `tools/lint/go.mod`) are skipped. All callers start at the root.
- **D-6 Readiness timeouts fail explicitly** (the bash loops silently continued to
  `go test`). Explicit-error constraint.
- **D-7 `.env` is created with `O_CREATE|O_EXCL`, mode 0600** (bash used the umask,
  usually 0644). Never overwrites, race-free, gosec G306-clean.
- **D-8 `values-dev.yaml` is kept** as the non-production server values; only its
  2-line header comment changes. Values stay byte-identical.
- **D-9 `tools/go.sh` gets a small, exact trim** (P4). The `JUSTIX_` variable prefixes
  stay: a shell variable inherited from the environment is exported to the exec'd
  compiler, so short names like `root` could clobber the caller's environment.
- **D-10 Makefile has one owner (P4)**, including removal of `k8s-up`/`k8s-down`.
- **D-11 `doctor` drops `codex` and does not `LookPath("go")`**: under `go run` PATH
  starts with `$GOROOT/bin`, so that check is meaningless; the pinned compiler is
  checked with `bash tools/go.sh version`.
- **D-12 CI needs no edit.** `make test-go` stays the CI entry; setup-go puts 1.27.1
  on PATH, which `tools/go.sh` accepts; openssl/curl are no longer needed.
- **D-13 No new Go module dependencies.** Standard library only (`flag`, `os/exec`,
  `os/signal`, `net/http`, `crypto/rand`, `encoding/hex`, `regexp`, `bufio`, …).
  `go.mod`/`go.sum` must not change; a packet that believes it needs one stops.
- **D-14 No new Makefile targets** (`doctor`, `check-git`) — not requested.
- **D-15 Skip messages** in `internal/e2e/e2e.go:48` and
  `internal/modules/documents/repository/s3_test.go:16` name `bash tools/test-go.sh`;
  P4 changes them to `make test-go` so no code points at a deleted script.

## 4. Requirements

| ID | Requirement |
|---|---|
| DT-01 | One package `tools/devtool` (`package main`, root module `justixauto`), stdlib only, subcommand dispatch, usage (exit 2), root discovery (D-5), argv-only subprocesses (never `sh -c`/`bash -c` with composed strings), explicit errors `devtool <cmd>: <reason>` on stderr, exit 1. |
| DT-02 | `check-git`: identical stdout lines and exit codes 0/4/5 to `tools/check-git.sh` at base. |
| DT-03 | `tools/check-git.sh` becomes a logic-free shim preserving the exact output and exit codes 0/4/5 (D-1, D-2). |
| DT-04 | `doctor`: git/node/npm/docker presence, pinned Go, node/npm versions, `docker compose version`, daemon reachable, check-git; no `codex`, no scaffold line; runs all checks, exit 1 if any failed; `npm run doctor` runs it. |
| DT-05 | `env`: create `.env` from `.env.example` with generated password, DATABASE_URL, HTTP_ADDR, ALLOWED_ORIGINS, POSTGRES_PORT; never overwrite; same messages; `make env` is a thin call. |
| DT-06 | `test`: digest-pinned PostgreSQL 18.6 + MinIO containers, readiness waits, exported TEST_DATABASE_URL/TEST_S3_ENDPOINT/AWS_*, `go test -race -count=1 -p 1 <args or ./...>` through `tools/go.sh`, `NO_S3=1`, cleanup on success/failure/signal, exit code propagation, password never printed; `make test-go` calls it. |
| DT-07 | `openapi-staged`: identical semantics and messages to `tools/openapi-staged.sh`; lefthook calls it. |
| DT-08 | `tools/test-go.sh`, `tools/doctor.sh`, `tools/openapi-staged.sh` deleted; every live reference outside section 10 updated. |
| DT-09 | `tools/go.sh` trimmed with all five guarantees and its exact messages preserved. |
| DT-10 | `infra/kind/` deleted (10 files, list in P5); `k8s-up`/`k8s-down` removed; `image` kept. |
| DT-11 | README and infra README describe the laptop workflow (make env / db-up / migrate / api / web), the dev tool, and the debug setup; no local-kind text remains. |
| DT-12 | `.vscode/launch.json`: launch `cmd/api` with `envFile` `.env`, a remote-attach config, one optional Chrome/Vite config; strict JSON. |
| DT-13 | `values-dev.yaml` header comment corrected; values, `values-prod.yaml`, chart unchanged. |
| DT-14 | Quality: golangci-lint (`.golangci.yml`) clean, `make deadcode` clean, every `//nolint` specific with explanation, unit tests hermetic (no Docker, no network beyond `httptest` loopback), `-race` clean, `go.mod` unchanged, `make lint` green at final SHA. |
| DT-15 | Documentation follow-up list for the primary (section 10) is complete; packets do not edit those files. |
| DT-16 | `tools/agent-flow.mjs`, `tools/agent-flow.test.mjs`, `tools/agent-flow/README.md`, `tools/agent-git.mjs`, `tools/agent-git.test.mjs`, `tools/agent-git/README.md` deleted; no live reference outside history (UD-4). |
| DT-17 | Chart files live directly under `deploy/` with byte-identical content; live path references updated (UD-5). |

## 5. Shared internal interface (defined in P1, consumed by P2/P3)

Workers keep names unless lint forces a change; later packets use but do not modify P1 files,
except appending one entry to the command table in `main.go`.

```go
// runner.go
type command struct {
	name   string    // program looked up on PATH by os/exec
	args   []string
	dir    string    // always the project root
	env    []string  // nil = inherit os.Environ(); otherwise the full environment
	stdout io.Writer // nil = capture and return as output
	stderr io.Writer // nil = discard
}
type runner interface {
	// exitCode is the child's status when it started; err != nil only when it
	// could not start or ctx was cancelled (exitCode -1 then).
	run(ctx context.Context, c command) (output string, exitCode int, err error)
}
type execRunner struct{} // exec.CommandContext; Cancel sends os.Interrupt, WaitDelay 10s

// main.go
type app struct {
	root     string
	run      runner
	stdout   io.Writer
	stderr   io.Writer
	getenv   func(string) string
	lookPath func(string) (string, error)
}
type exitError struct{ code int } // main exits with code, prints nothing extra
var commands = []struct {
	name, summary string
	run           func(ctx context.Context, a *app, args []string) error
}{ /* check-git, doctor, env (P1); test (P2); openapi-staged (P3) */ }
func randomHex(nBytes int) (string, error) // crypto/rand + hex, used by env and test
```

`main`: `os.Exit(realMain(...))` (no defers before exit). Context via
`context.WithCancelCause`; the first SIGINT/SIGTERM cancels with a cause carrying the
signal, then `signal.Stop` (a second signal uses the default action). If a command
returns after such a cancellation, exit code is 130 (SIGINT) or 143 (SIGTERM).
`exitError{n}` → exit n. Usage error/unknown command → usage on stderr, exit 2;
`help`, `-h`, `--help` → usage on stdout, exit 0. Package doc comment states the
invocation `bash tools/go.sh run ./tools/devtool <command>` and that `go run` reports
any failure as exit 1 (D-2).

## 6. Implementation packets

All packets: Go worker (gpt-5.6-terra/medium, Claude `sonnet` medium), fresh
context, context budget ≤ 60k tokens of input; read only the listed inputs.
Scoped rules: `AGENTS.md`, `tools/AGENTS.md` (argv subprocesses, fail closed), plus
the ones named per packet. Before editing run `bash tools/check-git.sh` → expect
`GIT CHECK OK: /Users/bakhromachilov/startups/justixauto`, exit 0. No Git writes, no
branch switch, no commits. Write the result to
`docs/justix-auto/dev/results/T-DEVTOOL-Pn.md` (changed files, commands run with exit
codes, deviations). Common stop conditions: `STALE_INPUT`; a needed change outside
owned paths; a new module dependency seems required; a guard/permission denial;
a check fails for a reason outside the packet. Never read `.env` (the guard blocks it;
tests use `t.TempDir()`).

Common checks (expected: exit 0, no output from lint/deadcode):

```sh
bash tools/go.sh vet ./tools/devtool/...
bash tools/go.sh test -race -count=1 ./tools/devtool/...
bash tools/go.sh tool -modfile=tools/lint/go.mod golangci-lint run ./tools/devtool/...
make deadcode
git diff --stat -- go.mod go.sum        # expected: empty
```

### P1 — devtool skeleton, `check-git`, `doctor`, `env` (DT-01, 02, 04 logic, 05 logic, 14)

- Depends on: none. Resources: main-checkout writer.
- Read-only inputs: `tools/check-git.sh`, `tools/doctor.sh`, `Makefile` lines 15–41,
  `.env.example`, `.golangci.yml`, `go.mod` (module name/version), this section and §5.
- Owned (new): `tools/devtool/main.go`, `runner.go`, `root.go`, `checkgit.go`,
  `doctor.go`, `env.go`, and their `_test.go` files. Nothing else.
- Behaviour:
  - Skeleton per §5. Root discovery per D-5; failure message
    `devtool: go.mod of module justixauto not found in <cwd> or its parents`, exit 1.
  - `check-git` (stdout, byte-exact; `<root>` = resolved project root):
    - git root (`git -C <root> rev-parse --show-toplevel`, stdout trimmed; failure or git
      missing → empty, printed as `none`), compared after `EvalSymlinks`, differs →
      ```
      GIT CHECK FAILED: project needs its own Git repository: <root>
      Detected Git root: <detected or none>. Do not use the parent repository.
      User action: cd '<root>' && git init -b main && git add . && git commit -m 'Prepare development workspace'
      ```
      exit 4.
    - `git -C <root> rev-parse --verify HEAD` non-zero → `GIT CHECK FAILED: create the first commit before development.` exit 5.
    - else `GIT CHECK OK: <root>` exit 0. Git stderr discarded.
    - Expose the check as a function returning the code so `doctor` can call it in-process.
  - `doctor` (stdout; all checks run even after a failure):
    1. `FOUND: <p>` / `MISSING: <p>` for `git`, `node`, `npm`, `docker` (via `lookPath`).
    2. `bash tools/go.sh version`, streamed.
    3. `node --version`, `npm --version`, `docker compose version`,
       `docker info --format "Docker daemon: {{.ServerVersion}}"` (one argv element),
       streamed; skipped when the program was reported MISSING.
    4. check-git in-process (its lines printed).
    - A failed step prints `FAILED: <argv joined by spaces>` (or `FAILED: check-git`).
    - Last line `DOCTOR OK` (exit 0) or `DOCTOR FAILED: <n> check(s)` (exit 1).
    - No `codex`, no "not scaffolded" line, no `go` LookPath (D-11).
  - `env` (stdout messages byte-exact, em dash U+2014):
    - Ports from `getenv("POSTGRES_PORT")` (default `55432`) and `getenv("API_PORT")`
      (default `8080`); each must be decimal 1–65535, else error
      `devtool env: invalid POSTGRES_PORT "<v>"` (or API_PORT), exit 1, no file.
    - Read `<root>/.env.example` (missing → explicit error, exit 1).
    - Password: `randomHex(16)` (32 lowercase hex chars).
    - Each line starting with `POSTGRES_PASSWORD=`, `DATABASE_URL=`, `HTTP_ADDR=`,
      `ALLOWED_ORIGINS=` is replaced by, respectively, `POSTGRES_PASSWORD=<pw>`,
      `DATABASE_URL=postgres://justixauto:<pw>@127.0.0.1:<pg>/justixauto?sslmode=disable`,
      `HTTP_ADDR=127.0.0.1:<api>`,
      `ALLOWED_ORIGINS=http://127.0.0.1:5173,http://127.0.0.1:5174,http://127.0.0.1:5175,http://127.0.0.1:5176`.
      All other bytes unchanged. Ensure a final newline, then: replace a
      `POSTGRES_PORT=` line if present, else append `POSTGRES_PORT=<pg>\n`.
    - Create `<root>/.env` with `O_WRONLY|O_CREATE|O_EXCL`, 0600 (prefer `os.OpenRoot`
      scoped to root). `ErrExist` → print `.env exists — edit it or delete it first`,
      exit 0, file untouched. Write error → remove the partial file, exit 1.
    - Success: `created .env (Postgres on <pg>, API on <api>)`, exit 0.
- Required tests (hermetic, fake runner/lookPath/getenv, `t.TempDir()` roots):
  root discovery (nested dir, skip foreign `go.mod`, not found); check-git OK / foreign
  root / git failure prints `none` / no HEAD — exact stdout and codes 0/4/4/5;
  doctor all-pass, one MISSING (remaining checks still run, versions of the missing
  program skipped), daemon failure, check-git failure — exact lines, exit codes, and
  assert no call names `codex`; env default ports, custom ports, invalid port (no
  file), existing `.env` unchanged byte-for-byte, content transformation on a
  fixture template incl. untouched comment lines and trailing-newline handling,
  existing `POSTGRES_PORT=` line replaced, password 32 hex and different across two
  runs, mode 0600, missing template; dispatch: unknown command → 2, help → 0.
- gosec: `G204` on `execRunner` only, with `//nolint:gosec // argv from the fixed command table; no shell` style explanation; no blanket nolint.
- Acceptance: common checks green; `bash tools/go.sh build -buildvcs=false -o var/bin/devtool ./tools/devtool && var/bin/devtool check-git`
  prints `GIT CHECK OK: /Users/bakhromachilov/startups/justixauto`, exit 0.

### P2 — `test` subcommand (DT-06, 14)

- Depends on: P1 accepted. Resources: main-checkout writer. No Docker needed.
- Read-only inputs: `tools/test-go.sh`, P1 files (`main.go`, `runner.go`), §5.
- Owned: new `tools/devtool/gotest.go`, `gotest_test.go`; in `main.go` only the
  appended `test` command-table entry.
- Behaviour (in this order; `<name>` = `justixauto-test-<pid>`; images byte-exact):
  - `pg` = `docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2`
  - `minio` = `quay.io/minio/minio:RELEASE.2025-09-07T16-13-09Z@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e`
  1. `password = randomHex(16)`; `s3 := getenv("NO_S3") != "1"`.
  2. Cleanup registered before the first container starts, runs on every return path
     with a fresh context (`context.WithoutCancel` + 30 s):
     `docker rm -f <name>-pg <name>-s3`, output discarded, error ignored.
  3. `docker run -d --rm --name <name>-pg -p 127.0.0.1::5432 -e POSTGRES_PASSWORD=<pw> -e POSTGRES_DB=justixauto_test <pg>`
  4. if s3: `docker run -d --rm --name <name>-s3 -p 127.0.0.1::9000 -e MINIO_ROOT_USER=justixtest -e MINIO_ROOT_PASSWORD=<pw> <minio> server /data`,
     then `docker port <name>-s3 9000/tcp` → endpoint `http://127.0.0.1:<port>`.
  5. Up to 60 attempts, 1 s apart: `docker exec <name>-pg pg_isready -U postgres -d justixauto_test -h 127.0.0.1`
     exit 0 → ready; else error `PostgreSQL test container not ready after 60s`.
  6. if s3: up to 60 attempts, 1 s apart: HTTP GET `<endpoint>/minio/health/ready`
     (2 s timeout, `NewRequestWithContext`, body closed) status 200 → ready; else
     error `MinIO test container not ready after 60s`.
  7. `docker port <name>-pg 5432/tcp` → port. Port parsing: first non-empty line, text
     after the last `:`, decimal 1–65535, else error naming the container role.
  8. Env = `os.Environ()` + `TEST_DATABASE_URL=postgres://postgres:<pw>@127.0.0.1:<port>/justixauto_test?sslmode=disable`
     and, if s3, `TEST_S3_ENDPOINT=<endpoint>`, `AWS_ACCESS_KEY_ID=justixtest`,
     `AWS_SECRET_ACCESS_KEY=<pw>`, `AWS_REGION=us-east-1` (later entries win). With
     `NO_S3=1` nothing S3-related is added or removed.
  9. `bash <root>/tools/go.sh test -race -count=1 -p 1 <args…>`; `<args…>` = the
     subcommand's arguments verbatim, or `./...` when none. stdout/stderr streamed.
     Exit n≠0 → `exitError{n}`. Keep the comment `-p 1: packages share one database`.
  - Errors name the step (`start PostgreSQL container: exit status 125`), never the
    argv or environment: the password must not appear in any devtool output.
  - Cancellation: when ctx is cancelled (signal), the runner interrupts the child,
    cleanup runs, and the ctx cause is returned (main maps 130/143).
  - Inject for tests: runner, pid, password source, sleep/attempt delay, HTTP probe.
- Required tests: exact argv sequence and env (S3 and NO_S3); args forwarded verbatim
  vs default `./...`; go test exit 3 → exit code 3 with cleanup; each failure point
  (pg run fails, s3 run fails, pg never ready, MinIO never ready, bad port output)
  → explicit error, `go test` not run, cleanup still called exactly once; cancellation
  while `go test` runs → cleanup called, cause returned; password absent from captured
  stdout/stderr and error strings; port parser table (`127.0.0.1:49153`,
  `[::]:49153`, two lines, empty, `abc`, `0`, `70000`); real HTTP probe against
  `httptest` (200 ready, 503 not ready). No Docker, no real sleeps > 10 ms.
- Acceptance: common checks green; `gocognit` ≤ 30 per function.

### P3 — `openapi-staged` subcommand (DT-07, 14)

- Depends on: P2 accepted. Resources: main-checkout writer.
- Read-only inputs: `tools/openapi-staged.sh`, `Makefile` lines 110–119, P1 `main.go`/`runner.go`, §5.
- Owned: new `tools/devtool/openapistaged.go`, `openapistaged_test.go`; in `main.go`
  only the appended `openapi-staged` entry.
- Behaviour (git/make run in root; spec = `internal/pkg/apidocs/swagger.json`):
  1. `git diff --cached --name-only --diff-filter=ACMRD`; if no line matches
     `^(cmd|internal)/.*\.go$` → exit 0, nothing else runs, no output.
  2. `git diff --quiet -- cmd/*.go internal/*.go` (pathspecs passed literally as two
     argv elements). Exit 1 → stderr
     `pre-commit: stash or stage your unstaged Go changes so the OpenAPI spec matches the commit.`,
     exit 1, make not run. Exit other than 0/1 → explicit error, exit 1.
  3. `make --no-print-directory openapi`, stdout discarded, stderr streamed; failure → exit 1.
  4. `git diff --quiet -- internal/pkg/apidocs/swagger.json`, then (only if clean)
     `git diff --quiet -- cmd/*.go internal/*.go`; either exit 1 → stderr, exactly:
     ```
     pre-commit: OpenAPI annotations changed the spec (or swag fmt reformatted them).
     Review with 'git diff', then: git add internal/pkg/apidocs/swagger.json <changed files> && git commit
     ```
     exit 1. Both clean → exit 0.
  - Git stderr streamed to stderr.
- Required tests (fake runner): no staged Go (incl. `tools/devtool/main.go`,
  `web/x.ts`, `internal/a.go.txt`) → only one call; staged deleted `cmd/x.go` counts;
  unstaged changes → message, no make; happy path → exact call sequence; spec
  changed; Go reformatted; make fails; git diff exit 128 → error.
- Acceptance: common checks green.

### P4 — Rewire entry points, shim, deletions, go.sh trim (DT-03, 04, 05, 06, 07, 08, 09, 10 Makefile part)

- Depends on: P3 accepted. Resources: main-checkout writer.
- Scoped rules additionally: `internal/AGENTS.md` (only two string edits there).
- Read-only inputs: `Makefile`, `lefthook.yml`, `package.json`, `tools/go.sh`,
  `tools/check-git.sh`, the two internal files at the lines named below.
- Owned: `Makefile`, `lefthook.yml`, `package.json` (only `scripts.doctor`),
  `tools/check-git.sh`, `tools/go.sh`, delete `tools/test-go.sh`, `tools/doctor.sh`,
  `tools/openapi-staged.sh` (plain `rm <file>`, non-recursive),
  `internal/e2e/e2e.go` line 48 string, `internal/modules/documents/repository/s3_test.go` line 16 string.
- Changes:
  - Makefile: `env` recipe → `@$(GO) run ./tools/devtool env` (help text unchanged);
    `test-go` recipe → `$(GO) run ./tools/devtool test`; delete `k8s-up` and `k8s-down`
    (targets, help lines, `.PHONY` entries); section header
    `# ---- containers / Kubernetes ----` → `# ---- container image ----`; keep `image`;
    `fmt` recipe first line → `$(LINT) fmt ./cmd/... ./internal/... ./migrations/... ./tools/devtool/...`.
    No other Makefile change.
  - lefthook.yml line 15 → `run: bash tools/go.sh run ./tools/devtool openapi-staged`.
  - package.json → `"doctor": "bash tools/go.sh run ./tools/devtool doctor"`; no other key.
  - `tools/check-git.sh`, exact content:
    ```bash
    #!/usr/bin/env bash
    # Kept for task records and agent rules; the check itself is `devtool check-git`.
    # Built instead of `go run` so the exit codes 4 and 5 reach the caller.
    # Rebuilt only when a source file is newer than the binary (R0 review, 2026-09-25).
    set -eu
    cd "$(dirname "$0")/.."
    bin=var/bin/devtool
    if [ ! -x "$bin" ] || [ -n "$(find tools/devtool -name '*.go' -newer "$bin" -print -quit)" ]; then
      bash tools/go.sh build -buildvcs=false -o "$bin" ./tools/devtool
    fi
    exec "$bin" check-git
    ```
  - `tools/go.sh`, exact content (guarantees and messages unchanged, D-9):
    ```bash
    #!/usr/bin/env bash
    # Runs the ADR-13 pinned compiler without downloads or global PATH edits.
    set -eu
    export GOTOOLCHAIN=local
    JUSTIX_GO_VERSION=go1.27.1
    JUSTIX_PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
    compatible() { [ -x "$1" ] && [ "$("$1" env GOVERSION 2>/dev/null)" = "$JUSTIX_GO_VERSION" ]; }
    if [ -n "${JUSTIX_GO_BIN:-}" ]; then
      compatible "$JUSTIX_GO_BIN" || { echo "JUSTIX_GO_BIN must point to the installed $JUSTIX_GO_VERSION compiler." >&2; exit 1; }
      exec "$JUSTIX_GO_BIN" "$@"
    fi
    # Linked worktrees share the main checkout's ignored toolchain installation.
    JUSTIX_COMMON_DIR="$(git -C "$JUSTIX_PROJECT_ROOT" rev-parse --path-format=absolute --git-common-dir 2>/dev/null || true)"
    for CANDIDATE in \
      "$JUSTIX_PROJECT_ROOT/docs/justix-auto/dev/local/toolchains/go-1.27.1/bin/go" \
      "${JUSTIX_COMMON_DIR%/.git}/docs/justix-auto/dev/local/toolchains/go-1.27.1/bin/go" \
      "$(command -v go || true)" /opt/homebrew/bin/go; do
      if compatible "$CANDIDATE"; then exec "$CANDIDATE" "$@"; fi
    done
    echo "Go 1.27.1 is required. Install the approved toolchain locally or set JUSTIX_GO_BIN to its executable path. Automatic toolchain downloads are disabled." >&2
    exit 1
    ```
    Keep the file mode of the base file (not executable at base; callers use `bash`).
  - `internal/e2e/e2e.go:48` → `"set TEST_DATABASE_URL to a disposable PostgreSQL database (make test-go)"`;
    `s3_test.go:16` → `"set TEST_S3_ENDPOINT (make test-go starts MinIO)"`.
- Checks (expected results):
  - `bash tools/check-git.sh; echo $?` → `GIT CHECK OK: /Users/bakhromachilov/startups/justixauto`, `0`.
  - `make help` lists no `k8s-*`; lists `image`, `env`, `test-go`.
  - `make env` → `.env exists — edit it or delete it first` (the checkout has a `.env`), exit 0.
  - `npm run doctor` runs the Go doctor and ends with `DOCTOR OK` or a `DOCTOR FAILED` line naming only environment issues.
  - `bash tools/go.sh env GOVERSION GOTOOLCHAIN` → `go1.27.1` / `local`;
    `JUSTIX_GO_BIN=/usr/bin/true bash tools/go.sh version` → exact JUSTIX_GO_BIN message, exit 1.
  - `git grep -n -E 'test-go\.sh|doctor\.sh|openapi-staged\.sh' -- Makefile lefthook.yml package.json tools internal cmd .github`
    → no matches.
  - `bash tools/go.sh vet ./internal/e2e/... ./internal/modules/documents/...`, `make lint-go`, `make deadcode` → exit 0.
- Stop: any other Makefile/lefthook change seems necessary.

### P5 — Remove the local kind stack; docs; VS Code debug; values-dev comment (DT-10, 11, 12, 13)

- Depends on: P4 accepted. Resources: main-checkout writer. Infra scope: `infra/AGENTS.md`
  applies; offline only, no cluster, no helm install/upgrade, no `kind`.
- Read-only inputs: `README.md`, `infra/README.md`, `values-dev.yaml` lines 1–2,
  `Makefile` (post-P4), `.env.example` lines 1–12, `web/apps/*/package.json` `dev` script.
- Owned: delete exactly these tracked files (plain `rm <file>`; then `rmdir infra/kind/observability infra/kind`):
  `infra/kind/deps.yaml`, `infra/kind/down.sh`, `infra/kind/kind-config.yaml`, `infra/kind/up.sh`,
  `infra/kind/observability/{dashboard,grafana,loki,otel-collector,prometheus,tempo}.yaml`;
  edit `README.md`, `infra/README.md`, `deploy/values-dev.yaml` (lines 1–2 only);
  create `.vscode/launch.json`.
- Changes:
  - `values-dev.yaml` lines 1–2 → exactly:
    ```yaml
    # Non-production server environment (dev/staging cluster): plain HTTP on a NodePort,
    # S3 via MinIO and telemetry to an OpenTelemetry Collector at the addresses below.
    ```
    All other bytes unchanged.
  - `.vscode/launch.json` (strict JSON, no comments):
    ```json
    {
      "version": "0.2.0",
      "configurations": [
        {
          "name": "API: launch cmd/api",
          "type": "go",
          "request": "launch",
          "mode": "debug",
          "program": "${workspaceFolder}/cmd/api",
          "cwd": "${workspaceFolder}",
          "envFile": "${workspaceFolder}/.env",
          "env": { "GOTOOLCHAIN": "local" }
        },
        {
          "name": "API: attach to dlv on 127.0.0.1:2345",
          "type": "go",
          "request": "attach",
          "mode": "remote",
          "host": "127.0.0.1",
          "port": 2345
        },
        {
          "name": "Web: realization in Chrome (make web)",
          "type": "chrome",
          "request": "launch",
          "url": "http://127.0.0.1:5173/",
          "webRoot": "${workspaceFolder}/web/apps/realization"
        }
      ]
    }
    ```
  - `README.md`: line 43 drop "and the local Kubernetes stack (`make k8s-up`)";
    Quick start becomes the laptop workflow (`make env`, `make db-up`, `make migrate`,
    `make api`, `make web APP=…`, `make dev`); Tests block uses `make test-go`,
    `bash tools/go.sh run ./tools/devtool test ./internal/...` (extra args),
    `NO_S3=1 make test-go`, `bash tools/go.sh test ./...` (no Docker); new short
    section "Debugging (VS Code)": needs the Go extension and Delve built for Go
    1.27.1, point the extension at the pinned compiler (`go.alternateTools` →
    `tools/go.sh` or `go.goroot`), run `make db-up migrate` first; launch config loads
    `.env`; attach config expects
    `dlv debug ./cmd/api --headless --listen=127.0.0.1:2345 --api-version=2` started
    with `.env` loaded (`set -a && . ./.env && set +a`); Chrome config after
    `make web APP=realization`. New short "Developer tool" note: subcommands
    `check-git`, `doctor`, `env`, `test`, `openapi-staged` via
    `bash tools/go.sh run ./tools/devtool <command>`; `npm run doctor`. Layout: `tools/`
    line mentions `devtool`; add `deploy/` line ("server Helm chart"). A stale
    test container, if a run was killed: `docker ps --filter name=justixauto-test-`.
    No other sections change.
  - `infra/README.md`: remove the "Local Kubernetes (kind)" section (lines 13–32);
    keep the compose section; replace the production paragraph with a "Servers
    (Kubernetes)" paragraph: chart `deploy`, `values-dev.yaml`
    non-production server values, `values-prod.yaml` template; rollout to any cluster
    is human-only (`AGENTS.md`); link ADR-15. No kind/kubectl commands.
- Checks:
  - `git ls-files infra/kind` → empty; `test ! -e infra/kind`.
  - `git grep -n -I -E 'infra/kind|k8s-up|k8s-down|kind cluster|kind-justixauto' -- ':!docs' ':!.claude' ':!.codex'` → no matches.
  - `node -e "JSON.parse(require('fs').readFileSync('.vscode/launch.json','utf8'))"` → exit 0.
  - `git diff -U0 -- deploy/values-dev.yaml` → only lines 1–2, both comments.
  - `git diff --stat -- deploy/values-prod.yaml deploy/templates deploy/values.yaml deploy/Chart.yaml` → empty.
  - If `helm` is installed: `helm template justixauto deploy -f deploy/values-dev.yaml`
    output identical at base and after (compare via files under `$TMPDIR`); else record "helm not installed".
  - Every `make <target>` named in README exists in `make help`.

### P6 — Remove legacy hub helpers; flatten the chart into `deploy/` (DT-16, DT-17)

- Runs after Q2 and before P3 (user request 2026-09-25; file-disjoint from P3/P4).
  P5 text above already uses the new chart paths. Resources: main-checkout writer.
  Infra scope for the chart move: offline only, no helm install/upgrade, no cluster.
- Read-only inputs: `infra/README.md` line 16, `infra/kind/up.sh` lines 1–10 and 60–75.
- Owned: delete (plain `rm`, then `rmdir tools/agent-flow tools/agent-git`)
  `tools/agent-flow.mjs`, `tools/agent-flow.test.mjs`, `tools/agent-flow/README.md`,
  `tools/agent-git.mjs`, `tools/agent-git.test.mjs`, `tools/agent-git/README.md`;
  move (plain `mv`, no Git commands) every file under `deploy/helm/justixauto/`
  to the same relative path under `deploy/`, then `rmdir` the empty
  `deploy/helm/justixauto` and `deploy/helm`; edit the chart path only in
  `infra/README.md` (line 16) and `infra/kind/up.sh` (line 4 comment and the
  `helm upgrade --install` line; the file is deleted by P5 anyway).
- Canonical docs (`AGENTS.md`, `tools/AGENTS.md`, `agent-workflow.md`, version_control
  role files, ADR-15) are edited by the primary, not by P6.
- Checks:
  - `git ls-files tools | grep agent-` after staging → empty; `test ! -e tools/agent-flow -a ! -e tools/agent-git`.
  - `test ! -e deploy/helm`; `ls deploy` → `Chart.yaml templates values-dev.yaml values-prod.yaml values.yaml`.
  - Content identical: for each file `f` in `git ls-tree -r --name-only HEAD deploy/helm/justixauto`,
    `git show HEAD:$f | cmp - deploy/${f#deploy/helm/justixauto/}` → exit 0.
  - `git grep -n -I -e 'deploy/helm' -e 'agent-flow' -e 'agent-git' -- ':!docs/justix-auto/state' ':!docs/justix-auto/dev/tasks' ':!docs/justix-auto/dev/qa' ':!docs/justix-auto/dev/results' ':!docs/justix-auto/dev/agent-setup-plan.md' ':!docs/justix-auto/dev/agent-setup-results.md'`
    → only primary-owned files (AGENTS.md, tools/AGENTS.md, agent-workflow.md, ADR-15,
    `.claude/**`, `.codex/**`), none in P6-owned files.
  - If `helm` is installed: `helm lint deploy -f deploy/values-dev.yaml` → exit 0;
    else record "helm not installed".
- Result: `docs/justix-auto/dev/results/T-DEVTOOL-P6.md`. Commit (version_control):
  `chore(tooling): remove agent-flow and agent-git; move chart to deploy/`, explicit
  paths (deleted + old and new chart paths + the two infra files).

## 7. Review packets (reviewer, gpt-5.6-sol/high, read-only, fresh context)

| ID | After | Scope | Must check |
|---|---|---|---|
| R0 | this plan | T-DEVTOOL.md vs §1 decisions and sources | coverage of every UD/DT, D-1..D-15 sound, packets fit budget, no ADR/AGENTS edits in packets |
| R1 | P1 commit | exact diff base→P1 | §5 interface, byte-exact check-git lines/codes, doctor order/no codex, env transform/0600/O_EXCL, no `sh -c`, nolints specific+explained, tests hermetic |
| R2 | P2 commit | P1→P2 diff | image strings byte-exact vs base `tools/test-go.sh`, argv order, cleanup on all paths incl. cancel, fresh cleanup context, password never logged, exit propagation |
| R3 | P3 commit | P2→P3 diff | literal pathspecs, message bytes vs base script, exit-code handling of git diff |
| R4 | P4 commit | P3→P4 diff | Makefile only the listed hunks, shim/go.sh byte-exact to §6, go.sh guarantees, deleted files, two string edits only |
| R6 | P6 commit | Q2 SHA→P6 diff (`-M`) | only the 6 deletions, 100% renames of all chart files, two path edits, primary doc edits consistent with UD-4/UD-5, no other change |
| R5 | P5 commit | P4→P5 diff | 10 files deleted and nothing else in infra/, values-dev comment-only, launch.json content, README accuracy vs Makefile, no helm/prod change |

Verdict GREEN/BOUNCE with file:line findings to `docs/justix-auto/dev/qa/T-DEVTOOL-Rn.md`
(persisted by the hub). BOUNCE returns to a fresh worker with the findings.

## 8. QA packets (qa, gpt-5.6-sol/high; evidence to `docs/justix-auto/dev/qa/T-DEVTOOL-*.md`)

Each QA runs at the exact accepted commit, clean tree, no source edits. Record every
command, exit code and relevant output lines. Use `mktemp -d` under `$TMPDIR` for
disposable directories; never touch `.env` contents.

- **Q1 (after P1, offline)**: common checks; build `var/bin/devtool`
  (`bash tools/go.sh build -buildvcs=false -o var/bin/devtool ./tools/devtool`);
  `var/bin/devtool check-git` in the checkout → OK line, 0; disposable dir with a
  `go.mod` containing `module justixauto` and no Git (not inside a repo) → the three
  exit-4 lines with `Detected Git root: none.`, exit 4; after `git init -b main` there
  → exit-5 line, 5; after a commit (`git -c user.name=qa -c user.email=qa@example.invalid commit --allow-empty -m init`) → OK, 0.
  `var/bin/devtool doctor` → ends with `DOCTOR OK` (or FAILED naming only real env
  gaps), no `codex` in output. `var/bin/devtool nope; echo $?` → usage on stderr, `2`.
  `var/bin/devtool env` inside the checkout → exists message, exit 0.
- **Q2 (after P2, offline)**: `bash tools/go.sh test -race -count=1 -v ./tools/devtool/...`
  and full common checks; confirm by reading tests that each P2 required case exists.
- **Q3 (after P3, offline)**: common checks; in the checkout with nothing staged
  `bash tools/go.sh run ./tools/devtool openapi-staged; echo $?` → no output, `0`.
  Disposable clone (`git clone --no-hardlinks /Users/bakhromachilov/startups/justixauto "$d/c"`,
  never pushed; run everything there with
  `JUSTIX_GO_BIN=/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/go-1.27.1/bin/go`
  because the clone has no ignored toolchain copy): stage a comment-only change in an `internal/**/*.go` file plus an
  unstaged Go edit → refusal message, 1; with only the staged change → run completes
  (exit 0 when the spec is unchanged; otherwise the two-line message, 1) — record which.
- **Q4 (after P4, offline)**: all P4 checks; in a disposable copy of the checkout without `.env`, `make env POSTGRES_PORT=55499 API_PORT=8099` must produce `DATABASE_URL` with `:55499/`, `HTTP_ADDR=127.0.0.1:8099` and `POSTGRES_PORT=55499` (R0 suggestion); the shim must not rebuild on a second consecutive run (no `go build` output, same exit code); `git show --stat HEAD` lists exactly
  the P4 owned paths; `git ls-files tools | grep -E '\.sh$'` → only `tools/check-git.sh`,
  `tools/go.sh`; go.sh no-candidate path: copy `tools/go.sh` into `$d/tools/`, run
  `env -u JUSTIX_GO_BIN PATH=/usr/bin:/bin bash $d/tools/go.sh version` → exact "Go 1.27.1 is required…"
  message and exit 1, unless `/opt/homebrew/bin/go` is 1.27.1 (then record that and
  skip); `npx lefthook run pre-commit` with nothing staged → exit 0.
- **Q5 (after P5, offline)**: all P5 checks; README commands exist; `.vscode/launch.json`
  has `envFile` `${workspaceFolder}/.env`, `cwd` workspace root, attach `remote`
  127.0.0.1:2345. VS Code interactive debugging is not verifiable headlessly: marked
  "manual — user" in evidence, not GREEN-claimed.
- **Q-LIVE (after P4; Docker; resource lock `postgres`)**: Docker daemon local, no
  cluster. (a) `make test-go` → exit 0, `ok` lines incl. `internal/modules/documents/repository`
  (S3 tests not skipped: run `bash tools/go.sh run ./tools/devtool test -run S3 -v ./internal/modules/documents/repository/`
  and confirm no `SKIP`); (b) `NO_S3=1 bash tools/go.sh run ./tools/devtool test -run S3 -v ./internal/modules/documents/repository/`
  → exit 0, S3 tests report `SKIP`, no `-s3` container was started (`docker ps -a --filter name=justixauto-test-` during the run);
  (c) failing args `bash tools/go.sh run ./tools/devtool test ./does-not-exist/...` → non-zero;
  after each: `docker ps -a --filter name=justixauto-test- --format '{{.Names}}'` → empty;
  (d) signal: rebuild `var/bin/devtool` at this commit
  (`bash tools/go.sh build -buildvcs=false -o var/bin/devtool ./tools/devtool`), start
  `var/bin/devtool test ./...` in the background, wait until its
  containers appear, `kill -INT <pid>`, then exit code 130 and no `justixauto-test-`
  containers within 15 s. If the harness blocks `kill`, record (d) as BLOCKED, not
  GREEN. (e) devtool's own captured output of (a)–(d) contains no 32-character hex
  string (the password); `go test` output is excluded from this check.
- **QA-FINAL (qa_aggregator, final SHA of P5; lock `postgres`)**: at the final commit,
  not a list of earlier reports: `make lint` (includes openapi-check, golangci-lint,
  deadcode, ESLint, Prettier, knip), `make typecheck`, `make test-go`,
  `bash tools/check-git.sh` (0), `npm run doctor`, `make help`,
  `git diff --exit-code` afterwards (tree unchanged), `git diff --stat 5f37324 -- go.mod go.sum .github`
  → empty; chart moved unchanged (UD-5): for each of `Chart.yaml`, `values.yaml`,
  `values-prod.yaml`, `templates` the object ID of `5f37324:deploy/helm/justixauto/<x>`
  equals `HEAD:deploy/<x>` (`git rev-parse`); reference sweep
  `git grep -n -I -E 'test-go\.sh|doctor\.sh|openapi-staged\.sh|infra/kind|k8s-up|k8s-down' -- ':!docs/justix-auto/state' ':!docs/justix-auto/dev/tasks' ':!docs/justix-auto/dev/qa' ':!docs/justix-auto/dev/results'`
  → only the section-10 files (ADR-15/16, `internal/AGENTS.md`, dev docs, `.claude/hooks`);
  audit the coverage matrix below against the evidence.

- **Q6 (after P6, offline)**: all P6 checks at the P6 commit; `git show -M --stat HEAD`
  lists only renames `deploy/helm/justixauto/* → deploy/*` (100%), the six deletions,
  the two infra files and the primary doc edits; `make help` and `npm run doctor`
  still work (neither referenced the removed tools).

Dependency graph: R0 → P1 → R1 → Q1 → P2 → R2 → Q2 → P6 → R6 → Q6 → P3 → R3 → Q3 → P4 → R4 → Q4 →
Q-LIVE → P5 → R5 → Q5 → QA-FINAL → primary follow-up (§9) → human approval for dev.
Commits between packets: version_control, explicit paths, one commit per packet
(`feat(tools): …` / `chore(infra): …`, no AI attribution per user rule).

## 9. Coverage matrix

| Req | Packets | Tests / checks | QA |
|---|---|---|---|
| DT-01 | P1 (P2, P3 entries) | dispatch/root tests, lint | Q1, QA-FINAL |
| DT-02 | P1 | check-git unit tests (0/4/4/5) | Q1 (live 4/5/0) |
| DT-03 | P4 | shim byte-exact, `bash tools/check-git.sh` | Q4, QA-FINAL |
| DT-04 | P1, P4 | doctor unit tests; `npm run doctor` | Q1, Q4, QA-FINAL |
| DT-05 | P1, P4 | env unit tests; `make env` exists path | Q1, Q4 |
| DT-06 | P2, P4 | orchestration/port/probe/cancel tests | Q2, Q-LIVE, QA-FINAL |
| DT-07 | P3, P4 | openapi-staged unit tests | Q3, Q4 |
| DT-08 | P4 (P5 README) | reference grep | Q4, QA-FINAL |
| DT-09 | P4 | go.sh checks | Q4 |
| DT-10 | P4 (targets), P5 (files) | ls-files, make help | Q4, Q5, QA-FINAL |
| DT-11 | P5 | README/Makefile cross-check | Q5 |
| DT-12 | P5 | JSON parse, fields | Q5 (+ manual by user) |
| DT-13 | P5 | diff -U0, helm template equality | Q5, QA-FINAL |
| DT-14 | P1–P4 | common checks, `make lint` | every Q, QA-FINAL |
| DT-15 | primary | §10 list | QA-FINAL sweep |
| DT-16 | P6 (+ primary docs) | ls-files, reference grep | Q6, QA-FINAL |
| DT-17 | P6 | cmp per file, `helm lint` if available | Q6, QA-FINAL (object IDs) |

## 10. Primary follow-up (not packet work; after QA-FINAL, with SHA-256 + snapshot)

- ADR-15 (`adr-15-kubernetes-multi-replica-observability.md`): line 5 (status quote
  says "local kind cluster": add that the local stack was removed 2026-09-25 by user
  decision); line 13 (`values-dev.yaml` (kind) → non-production server values);
  line 20 (Local stack row → laptop uses Docker Compose PostgreSQL + `make api`/`make web`
  + VS Code debug; no local Kubernetes); lines 44–46 (Tempo "fine for the local
  stack"); lines 47–48 (MinIO "loads … into kind").
- ADR-16 (`adr-16-github-actions-ci.md`): line 20 (runner needs Docker and gcc;
  openssl/curl no longer used); line 36 (`make test-go` = `tools/test-go.sh` →
  `bash tools/go.sh run ./tools/devtool test`); lines 74–100 prerequisite section
  (image pin now lives in `tools/devtool`; lines 95–100 "one-line change to
  `tools/test-go.sh`" and the kind follow-up are obsolete); line 104 (kind in out-of-scope list, optional).
- `AGENTS.md`: line 54 unchanged (shim). Recommended one line recording UD-1:
  "Repository tooling is Go (`tools/devtool`); only `tools/go.sh` and the
  `tools/check-git.sh` shim are shell."
- `internal/AGENTS.md:59`: `bash tools/test-go.sh` → `make test-go`.
- `tools/AGENTS.md`: add the devtool test command `bash tools/go.sh test ./tools/devtool/...`.
- `docs/justix-auto/dev/workflow.md:21–22` (historical doctor scaffold message is gone);
  `docs/justix-auto/dev/dev-state.md:86–87` (doctor/check-git now devtool; check-git needs the pinned Go);
  `docs/justix-auto/architecture.md:89` (preserve `go.sh`, `check-git.sh` shim, `tools/devtool`).
- `.claude/hooks/autonomous-guard.py:69` (`kind get|version`), `:221–222` (kubectl `kind-`
  context exception), `:250` (`k8s-down` ask rule) are stale; configuration changes need
  the user (agents may not edit hook config). Mirror in `.codex` if it has an equivalent.
- `.claude/agents/worker.md:14`, `.codex/agents/worker.toml:10`, `.github/workflows/ci.yml`: no change.
- UD-4/UD-5 (done by the primary with P6, snapshot `docs/justix-auto/state/backups/2026-09-25-tools-prune/`):
  `AGENTS.md`, `tools/AGENTS.md`, `agent-workflow.md`, both version_control role files,
  ADR-15 chart path. `.claude/hooks/autonomous-guard.py:200–202` (agent-git rule) is
  now dead but harmless; removing it is a user hook-config change.

## 11. Risks and gaps

- check-git now needs the pinned Go toolchain and a (cached) build; without it the
  shim exits 1 with the go.sh message instead of 4/5. Earlier it needed only git.
- `go run` entry points report failures as exit 1 plus an `exit status N` stderr line.
- SIGTERM sent only to `make`/`go run` (not the process group) does not reach devtool;
  containers may remain (same as the bash script under make). Ctrl-C reaches the
  whole group and triggers cleanup. Containers are `--rm` and named `justixauto-test-*`.
  A second signal during cleanup uses the default action.
- Concurrent shim runs may race on `var/bin/devtool` (rare; one writer rule).
- `values-dev.yaml` still names `minio.justixauto-deps` and
  `otel-collector.observability`, which only `infra/kind` provisioned. A
  non-production server environment needs a DevOps decision for those services
  before any deploy (not blocking this task).
- VS Code debugging depends on the user's Go extension/Delve for Go 1.27.1;
  verified only structurally.
- The guard hook blocks reading `.env` paths, so generated `.env` content is
  verified by unit tests only.
- ADR-15 records the kind stack as decided until the primary edits it (§10).

## Result and QA

- Developer results: `docs/justix-auto/dev/results/T-DEVTOOL-P1..P5.md`
- QA/review reports: `docs/justix-auto/dev/qa/T-DEVTOOL-*.md`
- Reviewed SHA: unassigned
- GREEN / BOUNCE / blocked reason: not run
- Integration SHA and checks (coordinator only): unassigned
