# T-DEVTOOL — P1 result: devtool skeleton, `check-git`, `doctor`, `env`

Worker: Go, fresh context. Checked `bash tools/check-git.sh` before editing →
`GIT CHECK OK: /Users/bakhromachilov/startups/justixauto`, exit 0.

## Files created (all new; nothing else touched)

- `tools/devtool/main.go` — package doc, `app`, `exitError`, `sigCause`,
  `commands` table, `randomHex`, `printUsage`, `lookupCommand`, `main`, `realMain`.
- `tools/devtool/runner.go` — `command`, `runner`, `execRunner`.
- `tools/devtool/root.go` — `moduleName`, `isProjectRoot`, `findRoot`.
- `tools/devtool/checkgit.go` — `detectGitRoot`, `checkGit`, `runCheckGit`.
- `tools/devtool/doctor.go` — `runDoctor`.
- `tools/devtool/env.go` — `parsePort`, `transformEnv`, `runEnv`.
- `tools/devtool/main_test.go`, `runner_test.go`, `root_test.go`,
  `checkgit_test.go`, `doctor_test.go`, `env_test.go`.

## §5 interface as implemented

```go
// runner.go
type command struct {
	name   string
	args   []string
	dir    string
	env    []string
	stdout io.Writer
	stderr io.Writer
}
type runner interface {
	run(ctx context.Context, c command) (output string, exitCode int, err error)
}
type execRunner struct{}
func (execRunner) run(ctx context.Context, c command) (string, int, error)

// main.go
type app struct {
	root     string
	run      runner
	stdout   io.Writer
	stderr   io.Writer
	getenv   func(string) string
	lookPath func(string) (string, error)
}
type exitError struct{ code int }
type sigCause struct{ sig os.Signal } // context.Cause carrying the signal
var commands = []struct {
	name, summary string
	run           func(ctx context.Context, a *app, args []string) error
}{ {"check-git", ...}, {"doctor", ...}, {"env", ...} }
func randomHex(nBytes int) (string, error)
func printUsage(w io.Writer)
func lookupCommand(name string) func(ctx context.Context, a *app, args []string) error
func main()
func realMain(args []string, stdout, stderr io.Writer) int

// root.go
func findRoot(cwd string) (string, error)

// checkgit.go
func checkGit(ctx context.Context, a *app) int
func runCheckGit(ctx context.Context, a *app, args []string) error

// doctor.go
func runDoctor(ctx context.Context, a *app, args []string) error

// env.go
func parsePort(getenv func(string) string, key, def string) (int, error)
func transformEnv(template, password string, pgPort, apiPort int) string
func runEnv(ctx context.Context, a *app, args []string) error
```

No new Go module dependency; `os.OpenRoot`/`os.Root` (stdlib, Go 1.27) used for
`.env`/`.env.example` access scoped to the project root (D-7 preference).

`main`/`realMain`: `os.Exit(realMain(...))`, no defers before exit.
`context.WithCancelCause`; first SIGINT/SIGTERM cancels with `sigCause{sig}`,
then `signal.Stop`; a command returning after such cancellation maps to exit
130 (SIGINT) or 143 (SIGTERM) via `errors.As(context.Cause(ctx), &sc)`.
`exitError{n}` → exit n, nothing else printed. Usage error / unknown command →
usage on stderr, exit 2. `help`/`-h`/`--help` → usage on stdout, exit 0.

## Exact stdout messages and exit codes per subcommand

### `check-git` (byte-exact vs `tools/check-git.sh`, DT-02)

- OK: `GIT CHECK OK: <root>\n` → exit 0.
- Foreign root:
  ```
  GIT CHECK FAILED: project needs its own Git repository: <root>
  Detected Git root: <detected or none>. Do not use the parent repository.
  User action: cd '<root>' && git init -b main && git add . && git commit -m 'Prepare development workspace'
  ```
  → exit 4.
- No HEAD: `GIT CHECK FAILED: create the first commit before development.\n` → exit 5.
- Git stderr discarded (command.stderr left nil).
- `checkGit(ctx, a) int` is exposed and called in-process by `doctor`.

### `doctor`

- `FOUND: <p>` / `MISSING: <p>` for `git`, `node`, `npm`, `docker` via `a.lookPath`.
- `bash tools/go.sh version` streamed (argv `["bash", "tools/go.sh", "version"]`, `dir=root`).
- `node --version`, `npm --version`, `docker compose version`,
  `docker info --format "Docker daemon: {{.ServerVersion}}"` streamed; each
  skipped when its program was reported MISSING.
- check-git in-process; its own lines print, plus `FAILED: check-git` on failure.
- A failed exec step prints `FAILED: <argv joined by spaces>`.
- Last line `DOCTOR OK` (exit 0, `nil` error) or `DOCTOR FAILED: <n> check(s)`
  (`&exitError{1}`). All checks run even after an earlier failure. No `codex`,
  no `go` `LookPath`, no "not scaffolded" line (D-11).

### `env`

- Ports: `getenv("POSTGRES_PORT")` default `55432`, `getenv("API_PORT")`
  default `8080`; invalid → plain error `invalid POSTGRES_PORT "<v>"` (or
  `API_PORT`), which `realMain` wraps as `devtool env: invalid POSTGRES_PORT "<v>"`
  on stderr, exit 1, no file written.
- Missing `<root>/.env.example` → wrapped error `devtool env: read .env.example: <os error>`, exit 1.
- Password: `randomHex(16)` → 32 lowercase hex chars.
- `POSTGRES_PASSWORD=`, `DATABASE_URL=`, `HTTP_ADDR=`, `ALLOWED_ORIGINS=` lines
  replaced wholesale exactly as specified; all other bytes unchanged; result
  always ends with one trailing newline; existing `POSTGRES_PORT=` line
  replaced, else `POSTGRES_PORT=<pg>\n` appended.
- `.env` created via `os.OpenRoot(root)` + `root.OpenFile(".env", O_WRONLY|O_CREATE|O_EXCL, 0600)`.
  `ErrExist` → stdout `.env exists — edit it or delete it first\n`, exit 0, file untouched.
  Write error → partial file removed via `root.Remove`, wrapped error, exit 1.
- Success: stdout `created .env (Postgres on <pg>, API on <api>)\n`, exit 0.

## Check commands run (exit codes)

```
bash tools/check-git.sh                                                                → GIT CHECK OK, 0 (before editing)
bash tools/go.sh vet ./tools/devtool/...                                               → 0, no output
bash tools/go.sh test -race -count=1 ./tools/devtool/...                               → 0, "ok"
bash tools/go.sh tool -modfile=tools/lint/go.mod golangci-lint fmt ./tools/devtool/...  → 0 (used once to fix a gci import-group finding)
bash tools/go.sh tool -modfile=tools/lint/go.mod golangci-lint run ./tools/devtool/...  → 0, "0 issues."
make deadcode                                                                            → 0, no output
git diff --stat -- go.mod go.sum                                                         → 0, empty
bash tools/go.sh build -buildvcs=false -o var/bin/devtool ./tools/devtool && var/bin/devtool check-git
    → "GIT CHECK OK: /Users/bakhromachilov/startups/justixauto", 0
var/bin/devtool bogus; echo $?                                                          → usage on stderr, 2
var/bin/devtool --help; echo $?                                                         → usage on stdout, 0
```

Manual smoke test (not required, informational): `var/bin/devtool doctor` in
the checkout ended with `DOCTOR OK`, exit 0, no `codex` in the output.

## gosec `//nolint` lines and explanations

- `runner.go`, `execRunner.run`:
  `//nolint:gosec // argv from the fixed command table; no shell` on the
  `exec.CommandContext(ctx, c.name, c.args...)` call (G204). This is the only
  G204 nolint, exactly matching the packet's requirement; no blanket nolint.
- `root.go`, `isProjectRoot`:
  `//nolint:gosec // dir walks only real filesystem ancestors of the process cwd; not user input`
  on `os.ReadFile(filepath.Join(dir, "go.mod"))` (G304). Golangci-lint (with
  `gosec` enabled per `.golangci.yml`) flagged this file-inclusion pattern even
  though `dir` only ever comes from walking up `os.Getwd()`'s ancestors inside
  `findRoot`; the nolint is specific to this line and explained, matching the
  `nolintlint` `require-explanation`/`require-specific` settings.

## Required-test coverage (mapped to the packet's list)

- Root discovery: `TestFindRootNested` (nested dir), `TestFindRootSkipsForeignGoMod`
  (foreign `go.mod`, e.g. `tools/lint/go.mod`-style path, is skipped), `TestFindRootNotFound`.
- check-git: `TestCheckGitOK` (0), `TestCheckGitForeignRoot` (4, exact 3-line
  message), `TestCheckGitFailurePrintsNone` (git failure → `Detected Git root: none.`, 4),
  `TestCheckGitNoHead` (5), `TestRunCheckGitExitError` (wraps as `*exitError`).
- doctor: `TestDoctorAllPass`, `TestDoctorOneMissingSkipsVersionChecks` (MISSING
  node → its version check skipped, remaining checks still run and pass, but the
  MISSING itself still fails the run → `DOCTOR FAILED: 1 check(s)`, matching
  DT-04 "exit 1 if any failed"), `TestDoctorDaemonFailure`, `TestDoctorCheckGitFailure`;
  all assert no call/argv named `codex`.
- env: `TestEnvDefaultPorts`, `TestEnvCustomPorts`, `TestEnvInvalidPort`,
  `TestEnvOutOfRangePort`, `TestEnvExistingUntouched` (byte-for-byte), `TestEnvMissingTemplate`,
  `TestEnvFileMode` (0600), `TestEnvPasswordDiffersAcrossRuns` (32 hex, differs),
  `TestTransformEnvContent` (untouched comment/unrelated lines, trailing newline),
  `TestTransformEnvReplacesExistingPortLine`, `TestTransformEnvNoTrailingNewlineInTemplate`.
- dispatch: `TestRealMainUnknownCommand`/`TestRealMainNoArgs` (2), `TestRealMainHelp` (0
  for `help`/`-h`/`--help`), `TestLookupCommandKnowsP1Commands`.
- runner: `TestExecRunnerRunsAndReportsExit`, `TestExecRunnerNoSuchProgram`,
  `TestExecRunnerCancelledContext` (basic hermetic smoke coverage of `execRunner`,
  using only `true`/`false`/`sleep`, no network, no Docker).

All tests are hermetic: fake `runner`/`lookPath`/`getenv`, `t.TempDir()` roots;
`.env` is never read from the real checkout.

## Deviations from the spec

- None identified. One clarification: DT-04's "exit 1 if any failed" is read to
  include a MISSING program itself as a failed check (not only failed exec
  steps), consistent with the base `tools/doctor.sh`'s `FAILED=1` on `MISSING`.
  This is exercised directly by `TestDoctorOneMissingSkipsVersionChecks`.
