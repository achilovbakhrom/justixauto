# T-933 independent QA observations

Reviewed commit: `d4512f1c8d7aebe71a39725b536e06a73b5e6b74`.

## Adversarial multi-artifact recovery probe

Command:

```sh
GOMODCACHE=/private/tmp/justixauto-t003-modcache \
GOCACHE=/private/tmp/justixauto-t933-qa-gocache GOPROXY=off \
bash tools/go.sh test -race -mod=readonly -count=1 -v \
  -overlay docs/justix-auto/dev/qa/T-933/overlay.json \
  -run '^TestQA933IntermediateCompletionIsNotOverallSuccess$' \
  ./tools/owner-migrate
```

Observed exit: `1`, as required for a regression that exposes the candidate bug.
The runner returned a nil error and:

```text
result.Status=recorded-completion
result.Version=12
ledger=12:false
receipts=1,12
feature17_missing=t
```

The requested profile head was 17. The injected failure happened after artifact
12 committed and before artifact 17 could make its dirty transition. The second
connection restored the original verified artifact bytes and performed only the
candidate's normal read-only reconciliation. That reconciliation proved artifact
12 completed, but `runPrivate` converted that intermediate result into overall
success even though 17 was absent.

The disposable PostgreSQL fixture was
`justixauto-t932-eebf1008-c66c-41c2-a628-0d9738168315`, immutable container ID
`a004134dbe828d26fe3e5f2ce602420dc3e4ede7d4459e726b62c3c7e4b17c1f`.
The fixture helper validated that exact ID and name absent after cleanup. Its
retained synthetic evidence directory is
`/var/folders/yq/j2ffv8bj2f35mcvbk7v7rk3m0000gn/T/justixauto-t932-evidence-2012531703`.

## Resolved-report shape probe

Command:

```sh
GOMODCACHE=/private/tmp/justixauto-t003-modcache \
GOCACHE=/private/tmp/justixauto-t933-qa-gocache GOPROXY=off \
bash tools/go.sh test -race -mod=readonly -count=1 -v \
  -overlay docs/justix-auto/dev/qa/T-933/overlay.json \
  -run '^TestQA933MalformedResolvedReportCannotReleasePath$' \
  ./tools/owner-migrate
```

Observed exit: `1`. All five malformed resolved reports released the evidence
path because `requireResolvedReport` accepted their common owner/database/profile
fields and status without validating status-specific evidence:

- recorded completion with version `999`;
- recorded completion without a request ID;
- recorded completion with the wrong SQL digest;
- recorded completion without provenance;
- verified no-op for version 16 while the exact profile head was 17.

## Passing exact-commit checks

- Git gate: PASS; root was the assigned T-933 worktree.
- Exact source SHA-256:
  - `main.go`: `9d4240fd1d8665b819a4f79763ca93ab81a42f508d65a4a925084337480ce6b5`
  - `main_test.go`: `0fc3dadba7124c5c386b0ce3fc402f3a6bbba98d006d3d723780b651717bfd77`
  - developer result: `dde46e3ceab87c407a7f4c1dc7df14cc15f29b2a07c6074747f410adb5919e06`
- Existing source/report unit focus: PASS, `0.572s`.
- Existing real runner phase/concurrency, failed-DDL and lost-final-commit focus:
  PASS, package `15.341s`; all three exact fixture IDs/names were validated absent.
- Complete `./tools/owner-migrate` race suite: PASS, `86.356s`.
- Required scoped race suite: PASS. Notable durations were contracts `128.546s`,
  integration `4.649s`, owner-migrate `87.503s`; all service/shared packages passed.
- Scoped vet with the independent overlay: PASS, no diagnostics.
- `git diff --check`: PASS.

The first sandboxed invocation of the live focus stopped before fixture creation
because child Docker access was denied. It was rerun with approved Docker access;
only the rerun is counted as application-test evidence.

The two previously restricted parameter-authority mutation probes remained
excluded from the scoped run under the existing user-authorization gate. No UI
surface changed, so browser/mock comparison was not applicable.
