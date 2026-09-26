# T-DEVTOOL P3 — `openapi-staged` subcommand

Status: DONE

## Pre-check

- `bash tools/check-git.sh` → `GIT CHECK OK: /Users/bakhromachilov/startups/justixauto`, exit 0.
- Input hash check: `sha256sum tools/openapi-staged.sh` matched the base value recorded
  in T-DEVTOOL.md §2 (`652009df...c47973b`); no stale-input condition.

## Changed files

- `tools/devtool/main.go` — appended one command-table entry:
  `{"openapi-staged", "regenerate and verify the OpenAPI spec against staged Go changes", runOpenapiStaged}`.
  No other line changed.
- `tools/devtool/openapistaged.go` (new) — `runOpenapiStaged` and helpers
  (`stagedNames`, `anyStagedGoPath`, `diffQuiet`, `runMakeOpenapi`).
- `tools/devtool/openapistaged_test.go` (new) — unit tests with a fake `runner`.

## Behaviour ported (DT-07)

Byte-exact port of `tools/openapi-staged.sh`:

1. `git diff --cached --name-only --diff-filter=ACMRD`; if no staged path matches
   `^(cmd|internal)/.*\.go$` → return with no output, exit 0 (only this one call runs).
2. `git diff --quiet -- cmd/*.go internal/*.go` (two literal argv pathspecs, no glob
   expansion). Exit 1 → stderr
   `pre-commit: stash or stage your unstaged Go changes so the OpenAPI spec matches the commit.`,
   `*exitError{1}`, `make` not run. Exit code other than 0/1, or a start/run error →
   explicit `"check unstaged Go changes: ..."` error (mapped to exit 1 by `realMain`).
3. `make --no-print-directory openapi`; stdout discarded (not assigned to `a.stdout`,
   captured into an internal buffer and dropped), stderr streamed to `a.stderr`.
   Failure → explicit `"run make openapi: ..."` error (exit 1).
4. `git diff --quiet -- internal/pkg/apidocs/swagger.json`, then — only if that was
   clean, matching the base script's `||` short-circuit — `git diff --quiet -- cmd/*.go
   internal/*.go` again. Either dirty → stderr
   ```
   pre-commit: OpenAPI annotations changed the spec (or swag fmt reformatted them).
   Review with 'git diff', then: git add internal/pkg/apidocs/swagger.json <changed files> && git commit
   ```
   `*exitError{1}`. Both clean → `nil` (exit 0).

Git stderr is streamed to `a.stderr` on every git call (`stderr: a.stderr` on each
`command`), matching the base script's unredirected git invocations.

## Commands run (from the project root)

| Command | Exit code | Notes |
|---|---|---|
| `bash tools/check-git.sh` | 0 | `GIT CHECK OK: /Users/bakhromachilov/startups/justixauto` |
| `bash tools/go.sh vet ./tools/devtool/...` | 0 | no output |
| `bash tools/go.sh test -race -count=1 ./tools/devtool/...` | 0 | `ok justixauto/tools/devtool` |
| `bash tools/go.sh tool -modfile=tools/lint/go.mod golangci-lint run ./tools/devtool/...` | 0 | `0 issues.` (one `gci`-formatting finding was fixed via `bash tools/go.sh fmt ./tools/devtool/...` before the final run) |
| `make deadcode` | 0 | no output |
| `git diff --stat -- go.mod go.sum` | — | empty (unchanged, D-13) |
| `git status --porcelain` | — | only the three files listed above changed |

All commands were re-run a second time after a mid-task interruption to confirm the
on-disk state still passes; results were identical (vet 0, test 0 `ok`, lint `0
issues.`, deadcode 0, go.mod/go.sum diff empty).

## Tests added (openapistaged_test.go)

- `TestOpenapiStagedNoGoFiles` — no `cmd/**`/`internal/**` Go path staged (incl.
  `tools/devtool/main.go`, `web/x.ts`, `internal/a.go.txt`) → exactly one call, no output.
- `TestOpenapiStagedDeletedFileCounts` — a staged deleted `cmd/x.go` still triggers the
  full check sequence.
- `TestOpenapiStagedUnstagedChangesBlocks` — unstaged Go changes → exact stderr message,
  exit 1, `make` not called; asserts the literal two-pathspec argv.
- `TestOpenapiStagedHappyPath` — exact 5-call sequence and argv strings, no stderr.
- `TestOpenapiStagedSpecChanged` — spec diff dirty → exact two-line message, exit 1, and
  the second `cmd/*.go internal/*.go` diff is *not* called (short-circuit, 4 calls total).
- `TestOpenapiStagedGoReformatted` — spec clean but Go reformatted by `swag fmt` → same
  two-line message, exit 1, 5 calls.
- `TestOpenapiStagedMakeFails` — `make openapi` non-zero exit → explicit error naming the
  step, spec diff not run.
- `TestOpenapiStagedGitDiffCachedFails` — first `git diff --cached` returns exit 128 with
  a start error → explicit `"list staged files: ..."` error.
- `TestOpenapiStagedUnstagedDiffOtherExitCode` — `git diff --quiet` step 2 returns exit
  128 → a plain (non-`exitError`) error, so `realMain` maps it to exit 1 rather than the
  refusal message.
- `TestLookupCommandKnowsOpenapiStaged` — dispatch table exposes the new subcommand.

## Deviations

None. Implementation follows §5/§6 P3 exactly: only the listed files were created/edited,
no new dependency, no change to `go.mod`/`go.sum`, messages and exit codes byte-exact to
`tools/openapi-staged.sh`, git/make argv passed as literal argv elements (no `sh -c`).

## Stop conditions checked

- STALE_INPUT: not triggered (hash matched).
- No change needed outside owned paths.
- No new module dependency (stdlib only: `context`, `fmt`, `regexp`, `strings`).
- No guard/permission denial encountered.
- No check failed for a reason outside the packet.
