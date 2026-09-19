# T-933-REQA PKT-REGRESSION — GREEN

Actor: `qa.t933.regression`  
Run: `t933-regression-20260919`  
Packet: `PKT-REGRESSION`  
Observed interval: `2026-09-19T08:25:53Z` through post-run verification at `2026-09-19T08:26:36Z`

## Identity

- Checkout: `/Users/bakhromachilov/startups/justixauto`
- Branch: `task/T-933-main-checkout-reqa`
- Frozen HEAD before and after: `c75dfcd683f612493598c8c40973dd125cdcb090`
- Base `3f3fe0339cdd9171e5ca3bdaf739c748c3e20ce9` and fixed task `fc6f6cd1e2219cb300eef6ab75c0f4937b56a471` are ancestors of the frozen HEAD.
- Tracked status was clean before and after. Only this ignored packet directory was used for artifacts.
- Approved plan digest: `af43e0cf7c700b139c260fc400aa5d9a00bdc485c2e16eceb197ac8b0ec9e890`; the persisted independent code/plan review was GREEN.
- Overlay replacement remained exactly
  `/Users/bakhromachilov/startups/justixauto/tools/owner-migrate/qa_t933_test.go` →
  `/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/qa/T-933/independent_test.go`.
- The original independent probe remained a regular, non-symlink file with SHA-256
  `6e2af6254cd2053affc9ce5836bd865bacd0d4444c2ae885353f8e776279ed9c`.

The complete 18-entry source lock matched both before and after the test:

| Locked path | SHA-256 before and after |
| --- | --- |
| `AGENTS.md` | `9370257d8fe0f5a598b35ca1168fa039b072e6df62ddd801403a5a014cfc07e4` |
| `tools/AGENTS.md` | `6ccebe156aff7d7912e5c427c0e8f550185651ea0995b6f4e4955ac839cc080c` |
| `tools/owner-migrate/AGENTS.md` | `93f53b7b081cf3588a16cf92a09c1725a746d4de8757bb9c8fb738ab562bb70f` |
| `docs/justix-auto/dev/agent-workflow.md` | `ba4d7ef077c0ad57f18739955bbd7cda35e7600b75a23c2fbc561001fcddf892` |
| `docs/justix-auto/state/drafts/contracts/owner-migration-compatibility.md` | `87ca5a5b7b5398726a53a9f4ec52b4f63c665e08ebaf69f57003401fa9ccbd76` |
| `docs/justix-auto/state/approvals/owner-migration-compatibility.md` | `32936629f1ada848a159938e4457fcc13a78c3990e952c9baafafcb4a6c2aea6` |
| `docs/justix-auto/dev/owner-migration-readiness.md` | `5ce4603553a33c9b5ae0564275219e66931dda4e6584c48b06cfd57c74202236` |
| `docs/justix-auto/dev/results/T-933.md` | `8c06f256ad6c228c1e2c0f557aed868382041992d445835c30e2ce48e46d39f1` |
| `docs/justix-auto/dev/qa/T-933.md` | `7766dca8846b3d3172283fee0131c62c31fb128afe240b132cd6f95a8a351ce6` |
| `docs/justix-auto/dev/qa/T-933/independent_test.go` | `6e2af6254cd2053affc9ce5836bd865bacd0d4444c2ae885353f8e776279ed9c` |
| `tools/owner-migrate/main.go` | `69ead41b503f0ec8d4aabb41dd21931547afa39372ccec2cbbb953491c2a6fb1` |
| `tools/owner-migrate/main_test.go` | `3882af445038eda70eeab9fff8c55dd79120fc8985b70a9c0aa2ee498881ccdb` |
| `tools/owner-migrate/driver.go` | `0daeda74249eeacf8cfa7397265483ce73d4fea8bb50725a17d81d0507af1eb1` |
| `tools/owner-migrate/driver_test.go` | `7b2216e3b451a09b2842db623c7eb18f29f813b56b1f868f2d8e64269f3afa5e` |
| `infra/local/postgres/init-owners.sql` | `79eaf102013af5845520deb73907aea6b2a6cc220683631d1b933fbccc20479d` |
| `tools/go.sh` | `21d8dcf9580088529a7a34a9ba4fc0bd6a56646a5c5e7fe7f79b66303d0d92c3` |
| `go.mod` | `f56fa77420290dcdcb59c51279bf2f3cc9797541df2226a64d2c19fb93c07a8a` |
| `go.sum` | `0965a4a214289f9fddf3696b5c979ede73072c1afd2d3410ef69b791cc796dce` |

## Environment and exact command

The child process was created with `env -i`, so no inherited
`JUSTIXAUTO_TEST_*`, `JUSTIXAUTO_T927_FIXTURE_CHILD`,
`JUSTIXAUTO_OWNER_MIGRATION_DSN`, `PG*`, or `GOMODCACHE` value was present.
The complete nonsecret child settings were:

- `HOME=/Users/bakhromachilov`
- `PATH=/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin`
- `TMPDIR=/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/execution/T-933-20260919/regression/tmp`
- `GOCACHE=/private/tmp/justixauto-go-cache.Ew93Ez`
- `GOPROXY=off`
- `GOTOOLCHAIN=local`

Pinned tool probes exited 0: `go version go1.27.1 darwin/arm64` through
`bash tools/go.sh version`, and the prepended Node binary reported `v24.21.0`.

Exact test argv:

```text
env -i HOME=/Users/bakhromachilov PATH=/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin TMPDIR=/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/execution/T-933-20260919/regression/tmp GOCACHE=/private/tmp/justixauto-go-cache.Ew93Ez GOPROXY=off GOTOOLCHAIN=local bash tools/go.sh test -race -mod=readonly -count=1 -v -timeout 5m -overlay docs/justix-auto/dev/local/execution/T-933-20260919/overlay.json -run '^(TestVerifiedSourceImmutableAndUpwardOnly|TestBundleAndAttemptReportsAreExact|TestMalformedResolvedReportCannotReleasePath|TestQA933MalformedResolvedReportCannotReleasePath)$' ./tools/owner-migrate
```

Exit: `0`. Wall time reported by the command runner: `13.256624375s`.

Raw combined stdout/stderr:

```text
=== RUN   TestVerifiedSourceImmutableAndUpwardOnly
--- PASS: TestVerifiedSourceImmutableAndUpwardOnly (0.01s)
=== RUN   TestBundleAndAttemptReportsAreExact
--- PASS: TestBundleAndAttemptReportsAreExact (0.07s)
=== RUN   TestMalformedResolvedReportCannotReleasePath
=== RUN   TestMalformedResolvedReportCannotReleasePath/recorded_missing_provenance
=== RUN   TestMalformedResolvedReportCannotReleasePath/no-op_wrong_head
=== RUN   TestMalformedResolvedReportCannotReleasePath/recorded_wrong_version
=== RUN   TestMalformedResolvedReportCannotReleasePath/recorded_missing_request
=== RUN   TestMalformedResolvedReportCannotReleasePath/recorded_wrong_SQL
--- PASS: TestMalformedResolvedReportCannotReleasePath (0.01s)
    --- PASS: TestMalformedResolvedReportCannotReleasePath/recorded_missing_provenance (0.00s)
    --- PASS: TestMalformedResolvedReportCannotReleasePath/no-op_wrong_head (0.00s)
    --- PASS: TestMalformedResolvedReportCannotReleasePath/recorded_wrong_version (0.00s)
    --- PASS: TestMalformedResolvedReportCannotReleasePath/recorded_missing_request (0.00s)
    --- PASS: TestMalformedResolvedReportCannotReleasePath/recorded_wrong_SQL (0.00s)
=== RUN   TestQA933MalformedResolvedReportCannotReleasePath
=== RUN   TestQA933MalformedResolvedReportCannotReleasePath/recorded_wrong_SQL
=== RUN   TestQA933MalformedResolvedReportCannotReleasePath/recorded_missing_provenance
=== RUN   TestQA933MalformedResolvedReportCannotReleasePath/no-op_wrong_head
=== RUN   TestQA933MalformedResolvedReportCannotReleasePath/recorded_wrong_version
=== RUN   TestQA933MalformedResolvedReportCannotReleasePath/recorded_missing_request
--- PASS: TestQA933MalformedResolvedReportCannotReleasePath (0.01s)
    --- PASS: TestQA933MalformedResolvedReportCannotReleasePath/recorded_wrong_SQL (0.00s)
    --- PASS: TestQA933MalformedResolvedReportCannotReleasePath/recorded_missing_provenance (0.00s)
    --- PASS: TestQA933MalformedResolvedReportCannotReleasePath/no-op_wrong_head (0.00s)
    --- PASS: TestQA933MalformedResolvedReportCannotReleasePath/recorded_wrong_version (0.00s)
    --- PASS: TestQA933MalformedResolvedReportCannotReleasePath/recorded_missing_request (0.00s)
PASS
ok  	justixauto/tools/owner-migrate	1.731s
```

All four required top-level tests actually ran and passed. No required test was
skipped. Both malformed-report implementations ran all five adversarial cases.

## Coverage and inspection

- `REQ-IDENTITY`: GREEN. Frozen HEAD, clean tracked state, overlay target, the
  unchanged independent probe, and every one of the 18 locked files were verified
  before and after execution.
- `REQ-REPORT-SHAPE`: GREEN. The exact bundle/report test passed. Both the owned
  test and unchanged independent probe rejected a nonexistent recorded version,
  missing request, wrong SQL digest, missing provenance, and a no-op below the
  profile head. Inspection confirms the report fields are fixed to format,
  status, owner, database, version, request, profile digest, SQL digest and
  provenance (`main.go:223-233`); attempt-bearing statuses bind a nonzero UUID,
  exact sealed artifact/digests and configured provenance (`main.go:431-469`);
  and verified no-op requires the exact head with empty request/SQL/provenance
  (`main.go:472-479`). `requireResolvedReport` dispatches through those
  status-specific validators (`main.go:494-522`).
- `REQ-ENGINE-PHASES` within this non-live packet: GREEN for its assigned
  source/report surface. The immutable source verifies the complete profile,
  copies original artifact bytes, exposes only increasing `Next` values, denies
  `Prev`/`ReadDown`, and returns copied buffers (`main.go:117-210`). Its required
  mutation/down/close regression passed. The runner composes that source and the
  project driver through actual `migrate.NewWithInstance` (`main.go:343-370`),
  and the compile-time interface assertions preserve both source-driver and
  database-driver API compatibility (`main.go:127`, `main.go:313`,
  `driver.go:77`). Live engine phases remain assigned to later packets.
- Original BOUNCE handling remains corrected by inspection: after authoritative
  reconciliation the exact intermediate result/report is preserved, while an
  intermediate version unequal to the sealed requested head returns
  `errRequestedHeadNotReached` (`main.go:394-414`). This packet intentionally did
  not execute the live intermediate-head probe.

No blocking or non-blocking omission was found in the assigned surface.

## Exclusions

No Docker command, PostgreSQL fixture, live DSN, existing database, broker,
cluster, install, download, dependency change, source edit, Git mutation,
controller write, canonical-doc edit, deployment or production action occurred.
Live bootstrap/upgrade/no-op/failure/concurrency/reconciliation behavior belongs
to `PKT-ENGINE` and `PKT-INTEGRATION`. T-928 parameter/server-default probes,
release profiles, baseline attestations, actual provisioning, T-934 custody,
production transport/policy and runtime composition remain out of scope.

An initial preflight helper invocation exited before hashing because the local
zsh variable name `path` reset zsh's reserved command-search array. It executed
no test and changed no source or Git state. The complete corrected preflight was
then rerun from the start and all 18 entries matched, as did the mandatory
post-run recheck above.

## Verdict

**GREEN** for `T-933-REQA / PKT-REGRESSION` at frozen HEAD
`c75dfcd683f612493598c8c40973dd125cdcb090`.
