# T-DEVTOOL P2 — `test` subcommand — result

- Branch: `chore/go-devtool` (main checkout, no worktree). `bash tools/check-git.sh`
  → `GIT CHECK OK: /Users/bakhromachilov/startups/justixauto`, exit 0, before editing.
- Status: **BLOCKED** — implementation complete, all required P2 tests green, but
  one common-check (`golangci-lint run`) fails for a reason rooted in a P1-owned
  line (`tools/devtool/main.go:55`), outside this packet's owned paths. This is
  one of the packet's own listed stop conditions ("a check fails for a reason
  outside the packet"), so I stopped rather than editing `main.go` beyond the
  one permitted command-table line.

## Files created/changed

- New: `tools/devtool/gotest.go` — `test` command implementation (containers,
  readiness, env, `go test` invocation, cleanup, cancellation, port parsing).
- New: `tools/devtool/gotest_test.go` — required unit tests (hermetic, fake
  runner, no Docker, no real sleeps > 10ms).
- Changed: `tools/devtool/main.go` — exactly one appended line in the
  `commands` table:
  ```go
  {"test", "run go test with throwaway PostgreSQL/MinIO containers", runTest},
  ```
  No other line in `main.go` touched (verified with `git diff`).

## Behaviour implemented (§ P2 steps 1–9)

1. `password, _ := cfg.password()` (production: `randomHex(16)`); `s3 := getenv("NO_S3") != "1"`.
2. `defer cleanupContainers(...)` registered immediately after computing
   `containerNames`, before the first `docker run`; uses
   `context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)`, runs
   `docker rm -f <name>-pg <name>-s3`, output/error discarded.
3. `startPostgres`: `docker run -d --rm --name <name>-pg -p 127.0.0.1::5432 -e POSTGRES_PASSWORD=<pw> -e POSTGRES_DB=justixauto_test <pg image>`.
4. `startMinio` (only if `s3`): `docker run -d --rm --name <name>-s3 -p 127.0.0.1::9000 -e MINIO_ROOT_USER=justixtest -e MINIO_ROOT_PASSWORD=<pw> <minio image> server /data`, then `docker port <name>-s3 9000/tcp` → `http://127.0.0.1:<port>`.
5. `waitPostgresReady`: up to 60× 1s `docker exec <name>-pg pg_isready -U postgres -d justixauto_test -h 127.0.0.1`; explicit error `PostgreSQL test container not ready after 60s` on exhaustion.
6. `waitMinioReady` (only if `s3`): up to 60× 1s `probeHTTP` (`GET <endpoint>/minio/health/ready`, `context.WithTimeout(ctx, 2*time.Second)`, `NewRequestWithContext`, body closed); explicit error `MinIO test container not ready after 60s`.
7. `postgresPort`: `docker port <name>-pg 5432/tcp` → `parseContainerPort` (first non-empty line, text after last `:`, decimal 1–65535).
8. `buildTestEnv`: `os.Environ()` + `TEST_DATABASE_URL=postgres://postgres:<pw>@127.0.0.1:<port>/justixauto_test?sslmode=disable`, and if `s3`: `TEST_S3_ENDPOINT`, `AWS_ACCESS_KEY_ID=justixtest`, `AWS_SECRET_ACCESS_KEY=<pw>`, `AWS_REGION=us-east-1` (appended after `os.Environ()`, later entries win); with `NO_S3=1` nothing S3-related is added.
9. `runGoTest`: `bash <root>/tools/go.sh test -race -count=1 -p 1 <args or ./...>`, stdout/stderr streamed (`a.stdout`/`a.stderr`), non-zero exit → `&exitError{code}`. Comment `-p 1: packages share one database.` kept verbatim.

## Exact argv sequence

S3 path (`NO_S3` unset or not `"1"`), no extra args:
```
docker run -d --rm --name justixauto-test-<pid>-pg -p 127.0.0.1::5432 -e POSTGRES_PASSWORD=<pw> -e POSTGRES_DB=justixauto_test docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2
docker run -d --rm --name justixauto-test-<pid>-s3 -p 127.0.0.1::9000 -e MINIO_ROOT_USER=justixtest -e MINIO_ROOT_PASSWORD=<pw> quay.io/minio/minio:RELEASE.2025-09-07T16-13-09Z@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e server /data
docker port justixauto-test-<pid>-s3 9000/tcp
docker exec justixauto-test-<pid>-pg pg_isready -U postgres -d justixauto_test -h 127.0.0.1   (looped up to 60x)
<probe http://127.0.0.1:<s3port>/minio/health/ready>                                          (looped up to 60x)
docker port justixauto-test-<pid>-pg 5432/tcp
bash <root>/tools/go.sh test -race -count=1 -p 1 <args or ./...>
docker rm -f justixauto-test-<pid>-pg justixauto-test-<pid>-s3   (deferred cleanup, always last)
```

`NO_S3=1` path: identical minus the two `-s3` `docker run`/`docker port` calls
and the MinIO probe loop; `docker rm -f` cleanup still names both `-pg` and
`-s3` (matches `tools/test-go.sh`'s unconditional `docker rm -f "$name-pg" "$name-s3"`,
harmless when `-s3` was never created).

## Parity table vs `tools/test-go.sh`

| `tools/test-go.sh` line | Go step |
|---|---|
| `pg_image=...` / `minio_image=...` | `testPGImage` / `testMinioImage` consts (byte-identical strings) |
| `name="justixauto-test-$$"` | `newContainerNames(cfg.pid)`, `cfg.pid = os.Getpid()` in production |
| `password="$(openssl rand -hex 16)"` | `cfg.password()` → `randomHex(16)` (crypto/rand, not openssl, per D-13 stdlib-only) |
| `cleanup() {...}; trap cleanup EXIT` | `defer cleanupContainers(...)`, fresh `context.WithoutCancel`+30s |
| `docker run ... $name-pg ...` | `startPostgres` |
| `if [ "${NO_S3:-}" != 1 ]; then docker run ... $name-s3 ...` | `startMinio` (returns `""` when `!s3`) |
| `export TEST_S3_ENDPOINT=...` (via `docker port ... | awk`) | `startMinio` → `parseContainerPort` |
| `export AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=... AWS_REGION=...` | `buildTestEnv` (s3 branch) |
| `for _ in $(seq 60); do docker exec ... pg_isready ...; sleep 1; done` | `waitPostgresReady` |
| `for _ in $(seq 60); do curl ... minio/health/ready; sleep 1; done` | `waitMinioReady` / `probeHTTP` |
| `port="$(docker port "$name-pg" 5432/tcp | ...)"` | `postgresPort` → `parseContainerPort` |
| `export TEST_DATABASE_URL=...` | `buildTestEnv` |
| `# -p 1: packages share one database.` `bash tools/go.sh test -race -count=1 -p 1 "${@:-./...}"` | `runGoTest` (comment kept verbatim) |

## Allowed deviations (D-6)

1. **Readiness timeout is now an explicit error** (D-6): the bash loops
   silently fell through to `go test` after 60 failed attempts; Go returns
   `"PostgreSQL test container not ready after 60s"` / `"MinIO test container
   not ready after 60s"` and never runs `go test`.
2. **Password generator**: `crypto/rand`+`encoding/hex` (`randomHex`, shared
   with `env`, per §5) instead of shelling out to `openssl rand -hex 16` — D-13
   (stdlib only, no new dependency, no subprocess for this step).

No other behavioural deviation from `tools/test-go.sh` was introduced.

## Cleanup and cancellation guarantee

- `cleanupContainers` is registered via `defer` in `runTestWithConfig`
  immediately after `containerNames` are computed — before the first
  `docker run` — so it fires on every return path: normal completion,
  any explicit error return, and `panic`/cancellation unwinding.
- It builds its own context (`context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)`)
  so a caller cancellation (SIGINT/SIGTERM) that cancelled `ctx` does not also
  cancel the cleanup call itself.
- Cancellation while `go test` runs: `execRunner.run` (P1, unmodified) sets
  `cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }`, so a
  cancelled `ctx` interrupts the `bash tools/go.sh test ...` child; `runGoTest`
  receives `err = context.Cause(ctx)` from the runner and returns it unchanged
  (not wrapped), so `realMain`'s `errors.As(context.Cause(ctx), &sc)` still
  matches and maps to exit 130/143. The same unwrap-on-cancellation path is
  used by `stepErr` for every earlier step (container start/port/pg_isready),
  so a signal during any step also propagates the raw cause instead of a
  wrapped string. Verified by `TestRunTestCancellationDuringGoTest`
  (`tools/devtool/gotest_test.go`).

## Password never in devtool output

- Every error message is built by `stepErr`, which only ever names the step
  (`"start PostgreSQL container: exit status 125"`) and wraps the runner's
  `error` value (a Go `error` from `os/exec`, e.g. `exit status N` — never
  argv/env) — the password is never interpolated into any string devtool
  prints.
- `docker run ...` calls that carry `-e POSTGRES_PASSWORD=<pw>` /
  `-e MINIO_ROOT_PASSWORD=<pw>` leave `command.stdout`/`command.stderr` unset
  (`nil`), so `execRunner` captures their output into a discarded buffer and
  discards stderr — never written to `a.stdout`/`a.stderr`.
- Only the final `go test` call streams to `a.stdout`/`a.stderr`, and its argv
  never contains the password (env vars are passed via `command.env`, not
  printed).
- `TestRunTestPasswordNeverLogged` (`gotest_test.go`) asserts the captured
  `a.stdout`/`a.stderr` strings never contain the password, and that a
  `stepErr` failure message never contains it either.

## Injection points used by tests

- `runner` — existing P1 `fakeRunner` (argv/env assertions, scripted exit
  codes/errors).
- `pid` — `testConfig.pid` (no `os.Getpid()` in tests).
- Password source — `testConfig.password func() (string, error)`.
- Sleep/attempt delay — `testConfig.attemptDelay` (tests use `time.Millisecond`,
  `maxAttempts` small, so the readiness-loop tests never sleep more than a few
  milliseconds total) plus `sleepCtx` selecting on `ctx.Done()` so a
  cancelled context aborts the wait immediately.
- HTTP probe — `testConfig.probe httpProber`; `TestProbeHTTPReadyAndNotReady`
  exercises the real `probeHTTP` against `httptest.NewServer` (200 → ready,
  503 → not ready); all orchestration tests use fake probes (`alwaysReady`/`neverReady`).

## Required tests present (`tools/devtool/gotest_test.go`)

`TestRunTestArgvSequenceWithS3` (exact argv+env, S3), `TestRunTestNoS3` (exact
argv+env, NO_S3, cleanup still names both containers), `TestRunTestForwardsArgsVerbatim`,
`TestRunTestExitCodePropagated` (go test exit 3 → `*exitError{3}`, cleanup
called once), `TestRunTestPGRunFails`, `TestRunTestS3RunFails`,
`TestRunTestPostgresNeverReady`, `TestRunTestMinioNeverReady`,
`TestRunTestBadPortOutput` (each: explicit error, `go test` never called,
cleanup called exactly once), `TestRunTestCancellationDuringGoTest` (cleanup
called, cancellation cause returned), `TestRunTestPasswordNeverLogged`,
`TestParseContainerPort` (table: `127.0.0.1:49153`, `[::]:49153`, two lines,
empty, `abc`/no colon, `0`, `70000`), `TestProbeHTTPReadyAndNotReady` (real
`httptest`, 200/503), `TestLookupCommandKnowsTest`.

## Checks run

```
bash tools/check-git.sh                                                          → GIT CHECK OK, exit 0
bash tools/go.sh vet ./tools/devtool/...                                         → exit 0, no output
bash tools/go.sh test -race -count=1 -v ./tools/devtool/...                      → PASS, ok, exit 0 (all P1+P2 tests)
bash tools/go.sh tool -modfile=tools/lint/go.mod golangci-lint fmt ./tools/devtool/... → exit 0 (no diff produced by fmt itself)
bash tools/go.sh tool -modfile=tools/lint/go.mod golangci-lint run ./tools/devtool/...  → exit 1, SEE FINDING BELOW
make deadcode                                                                     → exit 0, no output
git diff --stat -- go.mod go.sum                                                 → empty (no dependency added)
bash tools/go.sh build -buildvcs=false -o var/bin/devtool ./tools/devtool        → exit 0
```

## Blocking finding: `golangci-lint run` — `unparam` at `tools/devtool/main.go:55`

```
tools/devtool/main.go:55:16: randomHex - nBytes always receives 16 (unparam)
func randomHex(nBytes int) (string, error) {
               ^
1 issues:
* unparam: 1
```

- `randomHex(nBytes int)` is a P1 function in `main.go`, documented in §5 as
  "used by env and test". `env.go` already calls `randomHex(16)`; P2's
  `gotest.go` necessarily adds a second call `randomHex(16)` (packet body,
  step 1: `password = randomHex(16)`) to match `tools/test-go.sh`'s
  `openssl rand -hex 16`.
- Confirmed by isolating `gotest.go`/`gotest_test.go` out of the package and
  re-running the same lint command: **0 issues** with only P1's single call
  site. The finding appears only once a second call site with the same
  literal value exists — an emergent interaction between P1's shared
  `randomHex` and P2's required second caller, not a defect in `gotest.go`
  itself. I also confirmed constant-folding defeats a cosmetic workaround (a
  local `nBytes := 16` variable in `gotest.go` still triggers the same
  finding, since `unparam` reasons about the propagated constant value, not
  the call-site syntax) — I reverted that experiment; `gotest.go` calls
  `randomHex(16)` directly, matching the spec text.
- The finding's location (`main.go:55`, the `randomHex` function signature)
  is a P1-owned line. This packet's owned paths are only `gotest.go`,
  `gotest_test.go`, and the single appended `test` line in the `commands`
  table in `main.go` — "nothing else in P1 files." A `//nolint:unparam`
  comment (or any other fix) belongs on that P1 line, which is out of scope
  for this packet.
- This matches the packet's own stop condition: "a check fails for a reason
  outside the packet." I did not edit `main.go` beyond the one permitted line.

**Everything else required by P2 is done and green**: behaviour, all required
tests, `vet`, hermetic race tests, `deadcode`, unchanged `go.mod`/`go.sum`,
successful build, and — read from the full `golangci-lint run` output above —
**no `gocognit` finding was reported for any function** (the run evaluates all
enabled linters together; only the one `unparam` issue was reported), so the
`gocognit ≤ 30` acceptance criterion is satisfied.

## gosec

No new `//nolint` added by this packet. `gotest.go`'s subprocess calls all go
through the existing P1 `execRunner`/`a.run.run(ctx, command{...})` path,
which already carries the sole permitted `//nolint:gosec // argv from the
fixed command table; no shell` in `runner.go` (P1, unmodified). No blanket
`//nolint` anywhere in P2 code.

## Recommendation for the hub

One of:
1. Reslice a one-line P2 exception to add `//nolint:unparam // randomHex is
   shared by env (16-byte password) and test (16-byte password, D-13
   stdlib-only, matches tools/test-go.sh's openssl rand -hex 16); dropping the
   parameter would break env's call.` at `tools/devtool/main.go:55`, reviewed
   under R2 instead of R1; or
2. Accept this as a known P1/P2 boundary artifact and authorize the one-line
   `main.go` edit within P2 directly (widen owned paths for this specific
   line); or
3. Tune `.golangci.yml`'s `unparam` settings (also outside P2's owned paths).

Awaiting a decision before this packet can be marked DONE.
