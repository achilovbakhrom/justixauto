# Owner migration compatibility — independent proposal QA

**GREEN — proposal readiness only.** Reviewed exact commit
`c3734a5c3f2863be8fe752338c11740b9bdf31a4`, base
`f0a72a94b9badabc32f9ad028f913a98960e6b4a`, on 2026-09-15 in
`.worktrees/owner-migration-compatibility`. No blocking proposal defect found.
Draft SHA-256: `87ca5a5b7b5398726a53a9f4ec52b4f63c665e08ebaf69f57003401fa9ccbd76`.
This is not technical adoption, implementation QA, migration execution approval,
an existing-baseline attestation, or service/runtime readiness.

Read AGENTS, the main assignment/readiness finding, the exact proposed draft and
result, product/open-decision/reference/mock boundaries, the task board/index,
approved messaging/storage handoffs, T-001 dependency lock and the affected owner
SQL/UOW and task ownership. Inspected actual cached migrate v4.20.1 and native
pgx v5.11.0 source; did not rely on the author's report for API feasibility.

## Commands and evidence

From the task worktree:

```sh
git rev-parse HEAD
git diff --check f0a72a94b9badabc32f9ad028f913a98960e6b4a c3734a5c3f2863be8fe752338c11740b9bdf31a4
python3 docs/justix-auto/dev/qa/owner-migration-compatibility/check.py
```

PASS: [checker](owner-migration-compatibility/check.py) and
[42 checks with graph, ownership, artifact and API evidence](owner-migration-compatibility/check-output.json).
The checker reads exact Git objects, verifies both assigned files against HEAD,
resolves all six relative Markdown links, and verifies that only proposal/result
changed. It independently recomputes both approved migrate module sums from the
archive/go.mod and compares the unpacked source to every archive file.

From `docs/justix-auto/dev/qa/owner-migration-compatibility`:

```sh
GOPROXY=off GOSUMDB=off GOCACHE=/private/tmp/justixauto-owner-migration-qa-gocache /Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/go-1.27.1/bin/go test -race -count=1 -mod=readonly -v .
```

PASS, 1.400s: [real-engine probe](owner-migration-compatibility/engine_probe_test.go),
[output](owner-migration-compatibility/engine-output.txt),
[isolated QA module](owner-migration-compatibility/go.mod).
The module replaces only migrate with its verified local v4.20.1 source and uses
no network. It does not import an upstream PostgreSQL driver or lib/pq. The eight
probe scenarios exercise the real engine with a deliberately observed fake
database/source boundary; they do not simulate PostgreSQL atomicity as proof.

## Findings

| Area | Independent assessment |
| --- | --- |
| Actual engine hook | `NewWithInstance` accepts a public `database.Driver`. The real engine calls dirty, Run, then clean on the same supplied object. A project driver can own its pgx connection and make receipt insertion plus clean update one transaction. Upstream pgx's private `*sql.Conn` and independent SetVersion transaction cannot supply that hook through the previously imagined wrapper. |
| Exact SQL execution | The unchanged three owner SQL files contain their own BEGIN/COMMIT and dirty-v1 guards. Native `PgConn.Exec` uses simple-query protocol and accepts transaction-control/multiple statements; `ReadAll` drains results; `TxStatus` exposes idle/transaction/error state. Native `BeginTx` retains its owned connection. The proposal's complete original-byte execution without an outer transaction is feasible. Real DDL rollback, explicit COMMIT, cancellation and lost-reply tests remain required. |
| Engine failure behavior | Actual tests observe no Run after dirty-write failure, no clean after Run failure, no next work after finalization failure, and no Run for a retained dirty ledger. Missing source bodies can cause the engine to call clean without Run; the proposed pending-success guard is necessary and the probe demonstrates its rejection. Direct Force calls clean without Run and is likewise rejectable. |
| Bounded installation lock | Upstream Lock uses an uncancelled wait; engine timeout runs independently. The custom driver's captured parent context and finite bound, longer engine timeout and owned-session cleanup address that gap. Actual probe confirms a bounded driver error is returned before the longer engine timeout. Implementation must bound the entire Lock method, including any preflight placed inside it, and must not close a connection concurrently with a still-running driver operation. No live PostgreSQL lock claim is made here. |
| Baseline and immutable history | Existing v1 SQL proves neither historical file bytes nor complete installation history. The proposal distinguishes verified fresh execution from explicit attested-existing-v1 provenance, requires stopped-runtime/backup evidence, refuses later/dirty adoption, and does not infer history from a current file hash. Runtime sees SELECT-only immutable metadata; stable artifact identity is distinct from the old installing-bundle audit digest. |
| Bootstrap exception | P3 explicitly permits clean-v1/no-history before installing the history template, and says receipt-plus-clean atomicity applies to subsequent private migrations, not retroactively to v1. P6 requires testing this exception. Read the generic SetVersion table with that explicit exception: the fresh-v1 finalization must verify its mechanics marker without requiring nonexistent feature/history tables, retain verified execution evidence, and be inaccessible to existing/feature upgrades. Creating the initial empty ledger is an explicit locked fresh-bootstrap action; missing ledger elsewhere remains an error. |
| Feature 0012 and recovery | With clean 1 plus history, dirty 12 precedes verified SQL, the feature marker commits with DDL, and the immutable artifact receipt and clean-12 head commit together afterward. DDL-committed/dirty-without-receipt remains blocked. Lost finalization replies reconcile exact request/hash/profile/marker/history/head under the same installation lock on a fresh connection after closing the failed session. No automatic reapply, Force, reconstructed receipt or inference from client error is allowed. |
| Retained history and duplicates | Duplicate installation requests/version/path constraints, exact predecessor/history checks, verified no-op and concurrency probes are assigned. `[1,17]` to `[1,12,17]`, changed historical bytes, dropped old artifacts, dirty/multiple ledger rows and wrong receipts all fail before a dirty write. Real duplicate commands/events remain governed by existing owner event/receipt mechanics; the proposal preserves them and requires retained-data upgrade tests. |
| Readiness and isolation | Compatible profiles are sealed and trusted release inputs, never database-selected minima or request-supplied SQL/version ranges. Identity owner constants remain fixed. Explicit Check and each actual ReadCommitted Run verify before binding/callback; metadata and required object/grant checks are read-only. Existing legacy constructor behavior stays intact. PUBLIC/default/column/delegated/multi-hop NOINHERIT, owner membership, missing grants and metadata mutation are explicit negative cases. Feature contracts retain their own mutable-column and business authorization decisions. |
| Private versus shared axes | Private owner head 12 cannot substitute for shared messaging/storage revisions; complete shared markers cannot substitute for auth history. Custody has separate ACLs, revoked legacy outbox writes, exact shared lineage and actual component participation requirements. A database check does not prove assembly, admission, business effects or delivery. Custody ACK still means durable bytes/all jobs. |
| Ownership and graph | Six aliases produce an acyclic hypothetical 934-node graph after eight added edges to existing T-059/060/022/580. All aliases remain ancestors of B-01 closer T-591. Thirteen new leaves include the T-059 adjacent auth manifest. Identity UOW is an explicit serial successor to T-024; OM-CUSTODY extends the two common checker leaves only after OM-PROFILE. Identity and custody adapter work is disjoint. T-061 inherits Identity compatibility through T-060. No numeric task IDs are reserved by this QA. |
| Remaining owners and gates | Inventory/Commerce exact UOW follow-up leaves and prerequisites are recorded but unassigned; later owners require their own follow-ups. Their roots cannot claim custody activation yet. T-059 fixture protocol is explicitly distinguishable from mandatory actual-runner proof before local activation. T-002, recovery/security/auth transport, owner authorization, admission, sealed-data policy, process catch-up, guard participation and financial/legal decisions stay separate. |

## Adoption and implementation limits

The proposed project-owned driver and verified in-memory source deliberately
refine T-001's earlier upstream pgx/file-source guidance. Coordinator technical
adoption must record that bounded change; this QA does not itself approve it.
The exact migrate pin is approved but is absent from current project go.mod.
The proposal correctly leaves insertion and transitive resolution with the
coordinator, retaining pgx v5.11.0. Actual dependency changes need their own
exact-commit checks; optional drivers/cloud modules are not automatically accepted.

The six four-hour aliases are planning bounds, not measured completion estimates;
reslicing is required if the actual driver/fault protocol exceeds its bound.
The runner package lies under tools and needs the explicitly assigned
`./tools/owner-migrate` checks in addition to usual services/pkg/tests commands.

No live database, broker, application, browser, existing installation or external
service was touched. There is no UI change to compare against runnable mocks.
No source, original SQL, dependency lock, canonical board or task was edited;
only this assigned QA report and its evidence directory were written. No merge,
commit, baseline acceptance or migration execution was performed.
