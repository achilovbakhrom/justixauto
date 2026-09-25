# T-DEVTOOL — Q1 QA report (after P1, offline)

- Tested SHA: `ad4bb27b904d16fc71d45c91519727891605cf6c` (branch `chore/go-devtool`,
  checkout `/Users/bakhromachilov/startups/justixauto`).
- Pre-check: `git rev-parse HEAD` == `ad4bb27b904d16fc71d45c91519727891605cf6c`; `git status --short` empty
  (clean tree, confirmed before and after all checks).
- No source edits, no branch switch, no commit, no Docker used, `.env` contents never read.
- Disposable directory created with `mktemp -d`; verified NOT inside any Git repo before use;
  removed afterward with non-recursive `rm` per file + `rmdir` per directory (the guard hook
  blocked `rm -rf` as expected).
- Compared against `docs/justix-auto/dev/results/T-DEVTOOL-P1.md`: no mismatch found (see per-step
  notes below).

## 1. Common checks

| Command | Exit | Notes |
|---|---|---|
| `bash tools/go.sh vet ./tools/devtool/...` | 0 | no output |
| `bash tools/go.sh test -race -count=1 -v ./tools/devtool/...` | 0 | 31 tests, all `--- PASS`, 0 `--- FAIL`; `ok justixauto/tools/devtool 1.502s` |
| `bash tools/go.sh tool -modfile=tools/lint/go.mod golangci-lint run ./tools/devtool/...` | 0 | `0 issues.` |
| `make deadcode` | 0 | no output |
| `git diff --stat -- go.mod go.sum` | 0 | empty (as expected) |

Full test-name list (all PASS): TestCheckGitOK, TestCheckGitForeignRoot,
TestCheckGitFailurePrintsNone, TestCheckGitNoHead, TestRunCheckGitExitError,
TestDoctorAllPass, TestDoctorOneMissingSkipsVersionChecks, TestDoctorDaemonFailure,
TestDoctorCheckGitFailure, TestEnvDefaultPorts, TestEnvCustomPorts, TestEnvInvalidPort,
TestEnvOutOfRangePort, TestEnvExistingUntouched, TestEnvMissingTemplate, TestEnvFileMode,
TestEnvPasswordDiffersAcrossRuns, TestTransformEnvContent,
TestTransformEnvReplacesExistingPortLine, TestTransformEnvNoTrailingNewlineInTemplate,
TestRealMainUnknownCommand, TestRealMainNoArgs, TestRealMainHelp, TestRandomHex,
TestLookupCommandKnowsP1Commands, TestFindRootNested, TestFindRootSkipsForeignGoMod,
TestFindRootNotFound, TestExecRunnerRunsAndReportsExit, TestExecRunnerNoSuchProgram,
TestExecRunnerCancelledContext.

This matches the P1 result's "Required-test coverage" list one-for-one (31 named tests,
all present and passing); no test named in the result is missing from the live run and
no extra/unexpected failing test appeared.

## 2. Build

```
bash tools/go.sh build -buildvcs=false -o var/bin/devtool ./tools/devtool
```
Exit 0. `var/bin/devtool` produced, executable, 3193154 bytes. `var/` is confirmed
git-ignored (`git check-ignore -v var/bin/devtool` → `.gitignore:12:var/`), so building
it does not affect tree cleanliness.

## 3. `check-git` in the checkout

```
var/bin/devtool check-git
```
Output: `GIT CHECK OK: /Users/bakhromachilov/startups/justixauto`
Exit: 0. Matches spec exactly.

## 4. Disposable directory sequence

`d=$(mktemp -d)` → e.g. `/var/folders/.../tmp.050j61Ptgs`.

- Verified not inside any Git repo: `git -C "$d" rev-parse --show-toplevel` →
  `fatal: not a git repository (or any of the parent directories): .git`, exit 128.
- Wrote `go.mod` containing `module justixauto`.
- `(cd "$d" && /Users/bakhromachilov/startups/justixauto/var/bin/devtool check-git); echo $?`
  →
  ```
  GIT CHECK FAILED: project needs its own Git repository: /private/var/folders/.../tmp.050j61Ptgs
  Detected Git root: none. Do not use the parent repository.
  User action: cd '/private/var/folders/.../tmp.050j61Ptgs' && git init -b main && git add . && git commit -m 'Prepare development workspace'
  ```
  Exit: 4. (Note: path is printed symlink-resolved under `/private/var/folders/...`,
  consistent with D-5 `filepath.EvalSymlinks` root discovery on macOS where `/var` is a
  symlink to `/private/var`; this is expected and not a deviation.)
- `git -C "$d" init -b main` → `Initialized empty Git repository in .../.git/`.
- Rerun check-git →
  ```
  GIT CHECK FAILED: create the first commit before development.
  ```
  Exit: 5.
- `git -C "$d" -c user.name=qa -c user.email=qa@example.invalid commit --allow-empty -m init`
  → succeeded (`[main (root-commit) 2adedf4] init`).
- Rerun check-git → `GIT CHECK OK: /private/var/folders/.../tmp.050j61Ptgs`, exit 0.
- Cleanup: `find "$d" -type f -print0 | xargs -0 rm -f` then
  `find "$d" -depth -type d -print0 | xargs -0 rmdir` (recursive `rm -rf` was blocked by
  the guard hook, as the spec anticipated). Directory confirmed gone afterward.

All four outcomes byte-match the P1 result's documented messages and DT-02's
0/4/4/5 exit-code sequence (§9 coverage: DT-02 "Q1 (live 4/5/0)").

## 5. `doctor`

```
var/bin/devtool doctor
```
Output:
```
FOUND: git
FOUND: node
FOUND: npm
FOUND: docker
go version go1.27.1 darwin/arm64
v22.23.0
10.9.8
Docker Compose version v2.40.3-desktop.1
Docker daemon: 29.0.1
GIT CHECK OK: /Users/bakhromachilov/startups/justixauto
DOCTOR OK
```
Exit: 0. No real environment gaps found (all four programs present, pinned Go
1.27.1, Docker daemon reachable). Grep for `codex` (case-insensitive) over the full
output: no match.

## 6. Unknown command / help

- `var/bin/devtool nope`: stdout empty, stderr = usage block (`usage: bash tools/go.sh
  run ./tools/devtool <command> [args...]` + the three P1 command lines), exit 2.
- `var/bin/devtool --help`: same usage block on stdout, exit 0.

Both match §5's dispatch rule (usage error → stderr exit 2; `--help` → stdout exit 0).

## 7. `env` inside the checkout

```
var/bin/devtool env
```
Output: `.env exists — edit it or delete it first` (em dash present), exit 0.
`git status --short` immediately after: empty (clean). `.env` contents were not read
at any point.

## 8. Byte-exact parity, check-git messages (P1 source vs `tools/check-git.sh`)

Compared `tools/check-git.sh` (shell, lines 6–8, 12, 15) against the three message
templates in `tools/devtool/checkgit.go` (`checkGit`):

| Message | Shell (`tools/check-git.sh`) | Go (`checkgit.go`, `%s`→value) | Match |
|---|---|---|---|
| Foreign-root line 1 | `GIT CHECK FAILED: project needs its own Git repository: $PROJECT_ROOT` | `GIT CHECK FAILED: project needs its own Git repository: %s` (a.root) | identical |
| Foreign-root line 2 | `Detected Git root: ${GIT_ROOT:-none}. Do not use the parent repository.` | `Detected Git root: %s. Do not use the parent repository.` (displayDetected, defaults to `"none"`) | identical |
| Foreign-root line 3 | `User action: cd '$PROJECT_ROOT' && git init -b main && git add . && git commit -m 'Prepare development workspace'` | `User action: cd '%s' && git init -b main && git add . && git commit -m 'Prepare development workspace'` (a.root) | identical |
| No-HEAD line | `GIT CHECK FAILED: create the first commit before development.` | same, via `fmt.Fprintln` | identical |
| OK line | `GIT CHECK OK: $PROJECT_ROOT` | `GIT CHECK OK: %s` (a.root) | identical |
| Exit codes | 4 / 5 / 0 | 4 / 5 / 0 | identical |

No byte differences found in any of the three message blocks. This is consistent
with the empirical output captured live in step 4 above (note: P1 is not required to
make `tools/check-git.sh` itself a shim — that rewrite is P4/DT-03 scope; Q1 only
checks the devtool `check-git` subcommand's own message/exit parity against the base
script, which holds).

## Findings vs `docs/justix-auto/dev/results/T-DEVTOOL-P1.md`

No mismatch. Every command, message, exit code and test-name claimed in the P1
result was independently reproduced with identical output at the exact tested SHA:

- Common checks: all 0 exit / expected empty or "0 issues"/"ok" output — matches.
- Build + in-checkout `check-git` OK line — matches.
- The four-stage disposable-dir sequence (none/4, git-init/5, commit/0) — matches,
  including the exact three-line foreign-root message.
- `doctor` ends `DOCTOR OK`, no `codex` — matches (P1 result's "manual smoke test"
  note confirmed independently, and extended with `nope`/`--help`/`env` checks not
  explicitly logged in the P1 result but specified by the Q1 packet).
- `env` "exists" message and exit 0 — matches; tree stayed clean.

## Verdict: GREEN

All Q1 acceptance items (DT-01, DT-02 live 0/4/4/5, DT-04 doctor, common checks,
build+check-git) reproduced independently with no discrepancy against the P1
result or the task spec. Tree confirmed clean and at SHA `ad4bb27b904d16fc71d45c91519727891605cf6c`
before and after this QA run.
