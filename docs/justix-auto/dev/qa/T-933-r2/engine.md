# PKT-ENGINE
Kind qa; depends on accepted PKT-REGRESSION; exclusive resource postgres; evidence subdirectory engine/.
Read common.md. Acceptance: REQ-IDENTITY, REQ-REQUESTED-HEAD, REQ-ENGINE-PHASES, REQ-FAILURE-EVIDENCE, REQ-CONCURRENCY, REQ-FIXTURE-CUSTODY.
Require unchanged frozen claim HEAD and source lock. Inspect selected TestRunner and original independent intermediate-head probe before execution; every fixture must use existing verified startFixture lifecycle.
Run with the common sanitized environment and packet TMPDIR:
```
GOCACHE=/private/tmp/justixauto-go-cache.Ew93Ez bash tools/go.sh test -race -mod=readonly -count=1 -v -timeout 10m -overlay docs/justix-auto/dev/local/execution/T-933-20260919/overlay.json -run '^(TestRunnerActualEnginePhasesUpgradeNoOpAndConcurrency|TestRunnerFailedDDLRetainsDirtyEvidenceWithoutReplay|TestRunnerUnknownFinalCommitReconcilesAuthoritatively|TestRunnerIntermediateCompletionIsNotOverallSuccess|TestQA933IntermediateCompletionIsNotOverallSuccess)$' ./tools/owner-migrate
```
Observe all five tests PASS: actual engine phases and retained 1->12->17/no-op; bounded shared-template lock contention/concurrent installers; failed DDL stays dirty without replay; lost final reply reconciles authoritative receipt/head; exact intermediate receipt 12 remains recorded while target17 returns failure.
The original independent probe must run unchanged via new overlay. Collect synthetic retained SQL/report evidence and database/receipt assertions exposed by those tests without reading unrelated data.
Check creation and validated-cleanup lines for every exact fixture; unmatched allocation or missing absence proof blocks GREEN.
Do not broaden to unselected TestDriver probes, T-928 suites or inherited live opt-ins.
Write engine/report.md with actual HEAD, five test outcomes, custody/cleanup evidence, source hashes, requirement mapping, exclusions and verdict.
