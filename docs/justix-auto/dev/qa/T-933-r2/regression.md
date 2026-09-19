# PKT-REGRESSION
Kind qa; no dependencies or exclusive resource; evidence subdirectory regression/.
Read common.md. Acceptance: REQ-IDENTITY, REQ-REPORT-SHAPE, REQ-ENGINE-PHASES.
Inspect fixed runPrivate requested-head handling and status-specific report validation against original BOUNCE and P3/P6.
Confirm immutable upward-only source behavior, exact bundle/report fields, no-op shape, preserved original probe bytes and driver API compatibility; report concrete omissions without editing source.
After identity checks, run the following exact non-live selection with the common environment:
```
GOCACHE=/private/tmp/justixauto-go-cache.Ew93Ez bash tools/go.sh test -race -mod=readonly -count=1 -v -timeout 5m -overlay docs/justix-auto/dev/local/execution/T-933-20260919/overlay.json -run '^(TestVerifiedSourceImmutableAndUpwardOnly|TestBundleAndAttemptReportsAreExact|TestMalformedResolvedReportCannotReleasePath|TestQA933MalformedResolvedReportCannotReleasePath)$' ./tools/owner-migrate
```
No live tests are selected; do not run the complete owner-migrate package in this packet.
All four named tests must execute and pass, including unchanged independent malformed-report probe.
Write regression/report.md with actual HEAD, commands/exits, coverage, exclusions, findings and GREEN/BOUNCE/BLOCKED. Source hashes must match before/after.
