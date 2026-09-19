# T-933-REQA PKT-ENGINE — GREEN

Actor: `qa.t933.engine`  
Run: `t933-engine-20260919`  
Packet: `PKT-ENGINE`  
Accepted execution observed from fixture allocation at `2026-09-19T08:32:06Z`
through completion before the `2026-09-19T08:32:53Z` postflight.

## Identity

- Checkout: `/Users/bakhromachilov/startups/justixauto`
- Branch: `task/T-933-main-checkout-reqa`
- Frozen HEAD before and after: `c75dfcd683f612493598c8c40973dd125cdcb090`
- Base `3f3fe0339cdd9171e5ca3bdaf739c748c3e20ce9` and fixed task
  `fc6f6cd1e2219cb300eef6ab75c0f4937b56a471` are ancestors of the frozen HEAD.
- Tracked status was empty before and after. Only this ignored packet directory
  and the approved external GOCACHE were used for artifacts.
- Approved controller/plan digest:
  `af43e0cf7c700b139c260fc400aa5d9a00bdc485c2e16eceb197ac8b0ec9e890`.
  Independent code/plan review and prerequisite regression report were GREEN.
- The overlay remained exactly the locked replacement from
  `tools/owner-migrate/qa_t933_test.go` to the original independent probe. The
  probe was a regular non-symlink file before and after with SHA-256
  `6e2af6254cd2053affc9ce5836bd865bacd0d4444c2ae885353f8e776279ed9c`.

All 18 source-lock entries matched before and after:

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

## Command and environment

The child process was created with `env -i`; therefore no inherited
`JUSTIXAUTO_TEST_*`, `JUSTIXAUTO_T927_FIXTURE_CHILD`,
`JUSTIXAUTO_OWNER_MIGRATION_DSN`, `PG*`, or `GOMODCACHE` value was present.
Its complete nonsecret settings were:

- `HOME=/Users/bakhromachilov`
- `PATH=/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin`
- `TMPDIR=/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/execution/T-933-20260919/engine/tmp`
- `GOCACHE=/private/tmp/justixauto-go-cache.Ew93Ez`
- `GOPROXY=off`
- `GOTOOLCHAIN=local`

Pinned probes returned `go version go1.27.1 darwin/arm64` and `v24.21.0`.
Exact accepted argv:

```text
env -i HOME=/Users/bakhromachilov PATH=/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin TMPDIR=/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/execution/T-933-20260919/engine/tmp GOCACHE=/private/tmp/justixauto-go-cache.Ew93Ez GOPROXY=off GOTOOLCHAIN=local bash tools/go.sh test -race -mod=readonly -count=1 -v -timeout 10m -overlay docs/justix-auto/dev/local/execution/T-933-20260919/overlay.json -run '^(TestRunnerActualEnginePhasesUpgradeNoOpAndConcurrency|TestRunnerFailedDDLRetainsDirtyEvidenceWithoutReplay|TestRunnerUnknownFinalCommitReconcilesAuthoritatively|TestRunnerIntermediateCompletionIsNotOverallSuccess|TestQA933IntermediateCompletionIsNotOverallSuccess)$' ./tools/owner-migrate
```

Exit: `0`. Package time: `19.259s`. The successful raw combined output is
`successful-test.log` (SHA-256
`d51e0ec2a722ab0944676d3ef94bad114b728d7c05b05d968a0ad2f06d10576c`).
It contains exactly five top-level RUN lines, five PASS lines, five validated
cleanup/absence lines, and no SKIP or FAIL line. All five required tests ran:

- `TestRunnerActualEnginePhasesUpgradeNoOpAndConcurrency` — PASS (`8.11s`)
- `TestRunnerFailedDDLRetainsDirtyEvidenceWithoutReplay` — PASS (`2.29s`)
- `TestRunnerUnknownFinalCommitReconcilesAuthoritatively` — PASS (`2.24s`)
- `TestRunnerIntermediateCompletionIsNotOverallSuccess` — PASS (`2.46s`)
- `TestQA933IntermediateCompletionIsNotOverallSuccess` — PASS (`2.43s`)

The first sandboxed invocation could not access Docker and stopped every selected
test at its initial read-only absence inspection (`fixture identity inspection:
exit status 1`), before allocation. It is retained verbatim in `test.log`
(SHA-256 `7c3034f9057b47afaa5e315692c33c79f44baf3bd8a4ec1cebe5463580a1b2f9`).
Per the packet, the exact command above was then rerun once under narrowly
approved normal Docker permissions; no alternate operation or workaround ran.

## Engine and failure evidence

- `REQ-ENGINE-PHASES` and `REQ-CONCURRENCY`: the actual runner exercised bounded
  shared-template contention, fresh v1 bootstrap, history install, concurrent
  head-12 installers with exactly one applied and one verified no-op, exactly one
  version-12 receipt, a later verified no-op, and retained 12-to-17 upgrade. Its
  asserted final history was exactly `1,12,17`.
- `REQ-FAILURE-EVIDENCE`: the failed transactional DDL returned retained
  incomplete, kept the ledger `12:true`, kept only the version-1 artifact,
  omitted the failed schema, and stored a retained-incomplete report. The lost
  final COMMIT reply was hit, opened one fresh reconciliation connection,
  returned/stored recorded completion, and observed authoritative `12:false`.
- `REQ-REQUESTED-HEAD`: the owned probe required
  `errRequestedHeadNotReached`, retained exact recorded completion at 12,
  observed ledger `12:false` and receipts `1,12`, and proved feature 17 absent.
  The unchanged independent overlay probe separately required a non-nil overall
  error and the same exact ledger and receipts.
- These selected tests expose their SQL/receipt/report evidence through direct
  assertions and in-memory report stores. They do not call `fixture.retain`; the
  five accepted-run evidence directories were therefore intentionally empty.
  No SQL bodies, DSNs, passwords, or synthetic credentials were logged.

## Fixture custody and cleanup

Every accepted test used the existing `startFixture`/`cleanupFixture` lifecycle.
Before each creation, absence of the generated UUID name was proved. The creation
line is emitted only after validation of UUID name and ownership token, exact
64-hex container ID, exact pinned image, ownership label, the single
`/var/lib/postgresql:rw` tmpfs, the exact read-only bootstrap bind, and no other
mount. The fixture exposed PostgreSQL only on loopback and asserted server version
`180006`. Cleanup re-inspected and revalidated the same identity, removed only
the exact immutable ID, inspected both exact ID and exact name for absence, and
logged attached volume IDs `[]`.

| Test | Exact container ID | Exact generated name | Result |
| --- | --- | --- | --- |
| phases/concurrency | `150a7354b1895b437c6e4f8cef27ba92aea081a4ce6ee236a1627d7efbc59bab` | `justixauto-t932-1c763ee7-62b4-4abf-9127-780bc3eb4fbf` | validated removed; ID and name absent |
| failed DDL | `314001c7e7c279eed9d490b393ab63e9f6a54c9dcfb31ab95fb22bbd65abcc8b` | `justixauto-t932-e32b9eb0-8e3e-47e3-912a-527b8d4712fc` | validated removed; ID and name absent |
| unknown commit | `29a3755ef720e3ee5f04c0a2ca9eeded7619d1ddcc24c25084077e863a9c56c0` | `justixauto-t932-300bd0de-d12e-4cbb-ae1b-cef38dbafa69` | validated removed; ID and name absent |
| owned intermediate head | `ec69c1928dfd74826a5a88f00a4b7f7f910936ec403a390a55dab53e892950bd` | `justixauto-t932-fc064cf7-0ed7-4398-9773-49bc5a52fef3` | validated removed; ID and name absent |
| independent intermediate head | `4b42a3ca300f9541561c485fda32a7eb421cb555335045ba56962b111f8e4a07` | `justixauto-t932-42e7cb8d-e5a3-4db3-beb3-22af63616993` | validated removed; ID and name absent |

The sandboxed pre-attempt allocated no fixture, so its five generated names were
already absent and had no IDs to clean.

## Requirements and exclusions

- `REQ-IDENTITY`: GREEN — same clean frozen HEAD, all 18 hashes, overlay and
  original probe before/after.
- `REQ-ENGINE-PHASES`: GREEN — observed actual bootstrap, upgrade, no-op and
  immutable-profile engine behavior.
- `REQ-FAILURE-EVIDENCE`: GREEN — failed DDL and unknown final reply preserved or
  reconciled exact evidence without replay or dirty repair.
- `REQ-CONCURRENCY`: GREEN — bounded template lock plus concurrent installers
  yielded one receipt and no interleaved cleanup.
- `REQ-REQUESTED-HEAD`: GREEN — both owned and unchanged independent probes reject
  intermediate completion as overall success while retaining version-12 evidence.
- `REQ-FIXTURE-CUSTODY`: GREEN — five owned allocations, five validated exact-ID
  removals and five exact ID/name absence proofs; no uncertain cleanup.

No T-928 opt-in or parameter/server-default probe, parameter `SET` grant,
`session_replication_role` configuration, `ALTER SYSTEM`, newly introduced SQL
grant beyond the reviewed fixture lifecycle, existing database/DSN, broker,
cluster, install, download, dependency/source/canonical/Git mutation, controller
write, deployment, production action or unrelated test ran.
Release profiles, baseline attestations, actual provisioning, T-934 custody,
production transport/policy and runtime composition remain outside this packet.

## Verdict

**GREEN** for `T-933-REQA / PKT-ENGINE` at frozen HEAD
`c75dfcd683f612493598c8c40973dd125cdcb090`.
