# T-933-REQA PKT-INTEGRATION — GREEN

Actor: `qa.t933.integration`  
Run: `t933-integration-20260919`  
Packet: `PKT-INTEGRATION`  
Accepted checks: `2026-09-19T13:37:33+0500` through postflight at
`2026-09-19T13:42:28+0500`

## Identity and prerequisites

- Checkout: `/Users/bakhromachilov/startups/justixauto`; branch:
  `task/T-933-main-checkout-reqa`.
- Frozen HEAD before and after:
  `c75dfcd683f612493598c8c40973dd125cdcb090`. The base
  `3f3fe0339cdd9171e5ca3bdaf739c748c3e20ce9` and fixed task
  `fc6f6cd1e2219cb300eef6ab75c0f4937b56a471` are ancestors.
- Tracked status was empty before and after. No source, canonical document,
  dependency, Git, controller, branch or worktree mutation occurred.
- Independent code/plan review, PKT-REGRESSION and PKT-ENGINE were GREEN at the
  same frozen HEAD. Their coverage and exclusions were audited before execution.
- All 18 `source-lock.json` entries exactly matched before and after, and the two
  inventories are byte-identical: `source-hashes-before.txt` and
  `source-hashes-after.txt`. The original probe remained a regular non-symlink
  file at SHA-256
  `6e2af6254cd2053affc9ce5836bd865bacd0d4444c2ae885353f8e776279ed9c`;
  the overlay still maps only `tools/owner-migrate/qa_t933_test.go` to that file.
- Pinned probes were Go `1.27.1 darwin/arm64` and Node `v24.21.0`.

## Accepted commands and raw results

Every accepted Go child used `env -i` with only nonsecret settings:
`HOME=/Users/bakhromachilov`, the required Node 24.21.0 directory prepended to
the system `PATH`, packet-local absolute `TMPDIR`,
`GOCACHE=/private/tmp/justixauto-go-cache.Ew93Ez`, `GOPROXY=off`, and
`GOTOOLCHAIN=local`. Thus no inherited `JUSTIXAUTO_TEST_*`,
`JUSTIXAUTO_T927_FIXTURE_CHILD`, `JUSTIXAUTO_OWNER_MIGRATION_DSN`, `PG*`, or
`GOMODCACHE` setting reached a test.

| Check | Exact Go argv after the environment | Exit | Observed result | Raw evidence |
| --- | --- | ---: | --- | --- |
| Scoped race | `bash tools/go.sh test -race -mod=readonly -count=1 -v -timeout 15m ./services/... ./pkg/... ./tests/...` | 0 | 18 packages passed, 7 had no test files; 120 top-level PASS, 58 top-level SKIP, 0 FAIL | `scoped-race.log`, SHA-256 `8f1f9aaaa5d0ea027545acf2ce28bd905d92b332ce7927c1c48ce520235eadb9` |
| Nine owner tests | `bash tools/go.sh test -race -mod=readonly -count=1 -v -timeout 10m -overlay docs/justix-auto/dev/local/execution/T-933-20260919/overlay.json -run '^(TestVerifiedSourceImmutableAndUpwardOnly|TestBundleAndAttemptReportsAreExact|TestMalformedResolvedReportCannotReleasePath|TestQA933MalformedResolvedReportCannotReleasePath|TestRunnerActualEnginePhasesUpgradeNoOpAndConcurrency|TestRunnerFailedDDLRetainsDirtyEvidenceWithoutReplay|TestRunnerUnknownFinalCommitReconcilesAuthoritatively|TestRunnerIntermediateCompletionIsNotOverallSuccess|TestQA933IntermediateCompletionIsNotOverallSuccess)$' ./tools/owner-migrate` | 0 | all 9 required top-level tests ran and passed; 0 SKIP/FAIL; package `19.022s` | `owner-race.log`, SHA-256 `0738684e09bc65462fe087951317588c0db6bbda0df4168f02d9b3b628eb5bcf` |
| Scoped vet | `bash tools/go.sh vet -mod=readonly ./services/... ./pkg/... ./tests/... ./tools/owner-migrate` | 0 | no diagnostics | `scoped-vet.log`, empty SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| Overlay vet | `bash tools/go.sh vet -mod=readonly -overlay docs/justix-auto/dev/local/execution/T-933-20260919/overlay.json ./tools/owner-migrate` | 0 | no diagnostics | `overlay-vet.log`, empty SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |

The 58 broad-suite skips were inherited live PostgreSQL/broker opt-ins, including
the expected service-owner, projection, persistence, database-isolation and
broker-topology fixtures. They were intentionally not enabled. No T-928 probe,
parameter-grant probe, `session_replication_role`, or `ALTER SYSTEM` path ran.
No required owner test skipped.

## Fixture custody

The accepted owner command used the locked `startFixture`/`cleanupFixture`
lifecycle and the pinned image
`docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2`.
Each creation line was emitted after validation of generated UUID name/token,
exact immutable ID, image/label, loopback exposure, the one PostgreSQL tmpfs and
exact read-only bootstrap bind. Cleanup revalidated identity, removed only the
exact ID, and logged both exact ID/name absent with attached volume IDs `[]`.

| Accepted behavior | Exact ID | Exact generated name | Cleanup |
| --- | --- | --- | --- |
| phases/concurrency | `04477baef1ab3ccea584353fd413ae339586c1ed0ed0dcf92c2c7b6887ce7991` | `justixauto-t932-e17fe377-75df-4e43-8265-2f5aecd9abee` | validated removed; ID/name absent |
| failed DDL | `3bc8c2b0a5fd8531076a514144b01018214a8b3c324ba4d7496047c15ae72932` | `justixauto-t932-f026cec2-486c-4596-b06c-723f030c9554` | validated removed; ID/name absent |
| lost final reply | `a832580c72713f93458b153ec6c010d59898add31acb33fa23b50b9518d62aa7` | `justixauto-t932-9426acb7-20fa-4ad0-b1b0-ce74946a1352` | validated removed; ID/name absent |
| owned intermediate-head | `568bd74a298ab3cf5d1c67c485cb21c515ec900feda4e6f9e496b93ea2843da2` | `justixauto-t932-1547c15d-5d95-4327-89c8-02b38044c3e1` | validated removed; ID/name absent |
| original intermediate-head | `36c2451b87751206e04474d2df30d88433f43702c877bc25e5d1785c9d089e13` | `justixauto-t932-5390c4a1-b99e-48b1-a5c9-de365a9ea672` | validated removed; ID/name absent |

The test retained five accepted empty synthetic evidence directories. Together
with the five empty directories from the non-accepted attempt below, the
10-directory inventory is `retained-evidence-dirs.txt` (SHA-256
`d2b139c68ab0e25e7f1f2a7f8e568ef080ed79ebdbb4d8c6ef5946f1e3a9e2cf`);
`retained-evidence-files.txt` has zero entries (empty SHA-256
`e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`).

## Requirement mapping

- `REQ-IDENTITY`: GREEN — one clean frozen HEAD, exact overlay/original probe,
  and all 18 locked hashes remained unchanged across all accepted checks.
- `REQ-REPORT-SHAPE`: GREEN — bundle/report exactness and both owned/original
  five-case malformed-report probes passed.
- `REQ-REQUESTED-HEAD`: GREEN — both owned and unchanged original live probes
  rejected intermediate 12-of-17 completion as overall success while retaining
  its authoritative version-12 evidence.
- `REQ-ENGINE-PHASES`: GREEN — actual bootstrap, upgrade, no-op and immutable
  upward-only source behavior were re-observed on the final SHA.
- `REQ-FAILURE-EVIDENCE`: GREEN — failed DDL retained dirty evidence without
  replay; lost final COMMIT reply reconciled authoritatively on a fresh connection.
- `REQ-CONCURRENCY`: GREEN — bounded shared-template contention and concurrent
  installers produced the asserted single installation/receipt behavior.
- `REQ-FIXTURE-CUSTODY`: GREEN — five accepted allocations had five validated
  exact-ID removals and exact ID/name absence proofs; the five non-accepted-run
  allocations below were equally accounted for.
- `REQ-INTEGRATION`: GREEN — the final frozen SHA passed the scoped race suite,
  all nine owner tests with both original probes, and both scoped vet commands.

## Non-accepted invocation and exclusions

Before the accepted owner command, the same nine-test selection exited 0 but its
sanitized `PATH` misspelled the pinned directory as `docs/just-auto/...`. It is
not accepted evidence. The mistake was detected before reporting; the raw log is
preserved as `owner-race-invalid-node-path.log` (SHA-256
`0ec042206e8fadb59ec11149ebdfa4ed5f778a5178a5ad9930f4e7bdd2ae7385`).
Its five fixtures were all validated removed with exact ID/name absence:

| Exact ID | Exact generated name |
| --- | --- |
| `426e936c34d764b9a042354950618744b8d6a328ec802e425ffd7f5df124ad67` | `justixauto-t932-b925ffa7-6340-4caf-9daa-ed839740d9ef` |
| `f942d343d11c6cf01670f6b7b62f1cd6dde48143f0b09ae5074ed3387ceb30d4` | `justixauto-t932-3183c348-fdd3-4589-b2e9-dce970d0ee52` |
| `9408e146af6cf53a6e803ae655d6cb569270e3b97f2916636877e3b373f179c0` | `justixauto-t932-22253c61-be5c-46b4-8cb9-7f8643c529e6` |
| `ef1a72b3eefb149ae219eb7b458581b1ce2c0376495e40f840a64ab6ae76c239` | `justixauto-t932-8ecdbb0d-1690-45b0-a491-b7b23b9d241f` |
| `7677b324847691f22cf0764eead190434c848117c8a65eb31806e5089515235a` | `justixauto-t932-630f2547-716a-4a55-b3cf-489a9af91f5f` |

No existing database/DSN/cluster, broker fixture, alternate fixture, T-928 probe,
new SQL grant, parameter grant, production credential/action, install, download,
dependency change or unrelated test was used. Concrete release profiles,
baseline attestations/default provisioning, T-934 custody, production transport,
policy/runtime composition, B-01 closure, dev integration and deployment remain
outside this report and require their separate gates.

## Verdict

**GREEN** for `T-933-REQA / PKT-INTEGRATION` at frozen HEAD
`c75dfcd683f612493598c8c40973dd125cdcb090`. This report authorizes no merge,
push, deployment, B-01 closure or production promotion.
