# PKT-INTEGRATION
Kind integration; depends on accepted PKT-ENGINE; exclusive resource postgres; evidence subdirectory integration/.
Read common.md. Acceptance includes every requirement in plan.json.
Require same frozen actual HEAD as regression and engine, same original probe, same approved contracts and source hashes. Audit coverage and exclusions from both prerequisite reports; do not merely concatenate earlier GREENs.
Run all following commands with common sanitized environment, pinned Node PATH and packet TMPDIR:
```
GOCACHE=/private/tmp/justixauto-go-cache.Ew93Ez bash tools/go.sh test -race -mod=readonly -count=1 -v -timeout 15m ./services/... ./pkg/... ./tests/...
GOCACHE=/private/tmp/justixauto-go-cache.Ew93Ez bash tools/go.sh test -race -mod=readonly -count=1 -v -timeout 10m -overlay docs/justix-auto/dev/local/execution/T-933-20260919/overlay.json -run '^(TestVerifiedSourceImmutableAndUpwardOnly|TestBundleAndAttemptReportsAreExact|TestMalformedResolvedReportCannotReleasePath|TestQA933MalformedResolvedReportCannotReleasePath|TestRunnerActualEnginePhasesUpgradeNoOpAndConcurrency|TestRunnerFailedDDLRetainsDirtyEvidenceWithoutReplay|TestRunnerUnknownFinalCommitReconcilesAuthoritatively|TestRunnerIntermediateCompletionIsNotOverallSuccess|TestQA933IntermediateCompletionIsNotOverallSuccess)$' ./tools/owner-migrate
GOCACHE=/private/tmp/justixauto-go-cache.Ew93Ez bash tools/go.sh vet -mod=readonly ./services/... ./pkg/... ./tests/... ./tools/owner-migrate
GOCACHE=/private/tmp/justixauto-go-cache.Ew93Ez bash tools/go.sh vet -mod=readonly -overlay docs/justix-auto/dev/local/execution/T-933-20260919/overlay.json ./tools/owner-migrate
```
Never use root ./... (ignored toolchains are inside the checkout). Broad-scope opt-in live tests stay disabled, including T-928; report those exclusions. Owner-migrate selected tests execute actual live fixtures and both original independent probes; all nine must run.
Do not invoke unselected TestDriver tests; this re-QA validates T-933 composition against the integrated driver without claiming a fresh exhaustive T-932 acceptance.
Rehash all locked inputs after checks and record same exact actual HEAD, raw results, test/pass/skip counts and complete fixture cleanup proof. Any tracked/code change invalidates this packet; no integration GREEN across differing SHAs.
Write integration/report.md mapping every requirement to observed final-SHA evidence and limitations. Report GREEN only after all checks pass and each required behavior is demonstrated.
No B-01 closure, T-928 completion, deployment or dev integration is authorized by this report. Coordinator/aggregator and human integration gate remain separate.
