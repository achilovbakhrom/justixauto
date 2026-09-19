# T-933 re-QA common boundaries

Canonical task: T-933; execution ID: T-933-REQA; application hub: hub.application.
Base: 3f3fe0339cdd9171e5ca3bdaf739c748c3e20ce9. Fixed historical task: fc6f6cd1e2219cb300eef6ab75c0f4937b56a471.
Use only /Users/bakhromachilov/startups/justixauto on task/T-933-main-checkout-reqa.
No worktrees, branch switches, source changes, tracked QA edits, dependency installs, downloads, compiler changes, production actions or Git mutations.
The coordinator owns scheduling docs. Preserve historical BOUNCE and the original independent probe bytes.
Evidence, temporary fixture evidence and reports belong only under this ignored execution directory, within the packet's named evidence subdirectory; GOCACHE is the explicitly approved external cache exception.

Read this common brief, assigned packet, source-lock.json, root/tools/owner-migrate scoped AGENTS, owner migration contract P1-P6 and technical approval.
Use targeted original result/QA sections as needed; do not load the board or complete conversation.
Context ceiling: 16k tokens per packet; if the coherent behavior cannot fit, return RESLICE_REQUIRED.
Independent plan review and hub acceptance precede claims. Separate exact fix-diff review is required before acceptance; this manifest does not replace it.

The coordinator reserves one frozen checkout across all packets. Current scheduling checkpoint is c75dfcd683f612493598c8c40973dd125cdcb090; base remains 3f3fe0339cdd9171e5ca3bdaf739c748c3e20ce9. Actual claim HEAD may include prior scheduling-only commits over base; record it, require clean tracked files, preserve it throughout all three packets.
Before and after every packet, independently SHA-256 every source-lock.files entry, compare exact hashes, and verify the overlay still targets the original probe in this checkout.
The controller resolves manifest contracts and briefs relative to this plan directory and rehashes source-lock.json, common.md, overlay.json and each packet brief at claim. The hub and QA independently resolve source-lock.files paths relative to its checkout field and rehash every canonical/source/probe entry before and after verification; the controller does not recursively enforce those entries. Do not edit plans/briefs after their digest is reviewed.
Any source/contract drift, checkout mismatch, unexpected dirty tracked state, missing independent code review, prerequisite failure or fixture custody uncertainty stops acceptance.
No previous developer pass or merged ancestry replaces observed tests at claim HEAD.

Every Go invocation must run from the checkout using bash tools/go.sh, with GOCACHE=/private/tmp/justixauto-go-cache.Ew93Ez, GOPROXY=off and GOTOOLCHAIN=local. Use the default installed module cache that passed preflight; omit inherited GOMODCACHE overrides. GODEBUG=goindex=0 is an optional diagnostic only, not required for these checks.
Prepend /Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin to PATH for Go tests that launch Node.
Use the existing Go 1.27.1 installation; the prior failure was a stale global index. Do not reinstall/download/change versions or reuse the stale cache.
Before spawning tests, construct a child environment omitting every JUSTIXAUTO_TEST_* key, JUSTIXAUTO_T927_FIXTURE_CHILD, JUSTIXAUTO_OWNER_MIGRATION_DSN, and inherited PG* connection settings.
Set TMPDIR to an existing absolute packet evidence/tmp directory, using permitted artifact writes; startFixture then retains evidence inside this assignment. Synthetic t.TempDir files may be cleaned by Go.
Capture argv, sanitized nonsecret settings, actual HEAD, timestamps, exit status, raw stdout/stderr, passed/skipped names and before/after hash results. Never log the full environment or synthetic credentials.
Do not edit tests to bypass a failure or skip. GREEN requires observed exit 0 plus the required named tests executing; skipped required tests are BLOCKED.

Only PKT-ENGINE and PKT-INTEGRATION may hold postgres. They may execute existing startFixture/cleanupFixture from the locked driver_test.go, with pinned PostgreSQL 18.6 digest, generated UUID names/ownership token, loopback-only port, tmpfs and exact read-only bootstrap bind.
No existing databases/DSNs, broker, cluster, role session_replication_role configuration, parameter SET grants, ALTER SYSTEM, T-928 generation probes, alternative server-default mutation or external credentials.
Ordinary reviewed startFixture role/database grants and default ACL setup remain within its existing disposable lifecycle. No new SQL grants or fixture implementation.
Cleanup must validate immutable fixture ID/name/image/label/mounts before deletion and prove exact ID and name absent. Capture retained evidence hashes and cleanup log lines; uncertainty is BLOCKED, never broad cleanup.
No permission workaround if execution is rejected; retain evidence and return the exact blocked action/reason.

Requirements: REQ-IDENTITY = exact source/probe/contract and claim SHA; REQ-REPORT-SHAPE = malformed reports cannot release evidence;
REQ-REQUESTED-HEAD = intermediate completion cannot imply full requested-head success; REQ-ENGINE-PHASES = actual engine bootstrap/upgrade/no-op/immutable source;
REQ-FAILURE-EVIDENCE = failed DDL and lost commit preserve/reconcile evidence without replay or dirty repair;
REQ-CONCURRENCY = bounded lock and concurrent installer behavior; REQ-FIXTURE-CUSTODY = owned fixture lifecycle and exclusions;
REQ-INTEGRATION = all required behaviors rechecked on one final frozen SHA with scoped race/vet.
These contribute only to B-01.AC1; T-591 closes combined acceptance. Release profiles/baseline attestations, default provisioning for actual installations, custody T-934, T-928 authorization, production transport, policy and runtime composition remain separate.
