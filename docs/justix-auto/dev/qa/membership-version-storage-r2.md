# Membership-version storage proposal r2 QA — BOUNCE

2026-09-15. Reviewed exact commit
`c3c0da45965c980fee22a32f43e90d7a674a55c4`, detached checkout
`.worktrees/membership-version-storage-r2-qa`.
Proposal SHA-256:
`ea11e1a807292c2a4027c18f77f5d30e47fc1bfeddbcbb8a317ce3f464881cd9`.

The original lifecycle conflict is fixed, including explicit missing-link SQL
enforcement and historical read semantics. **One P2 enforcement-mechanism gap
remains before canonical promotion.** No application/canonical changes or task
allocation were made, and no QA artifact was committed.

## P2 — early constraint execution can bypass the claimed final-head check

Revised proposal §4, lines246–257, explicitly assigns to SQL the guarantee that
every new link references the final selected head and that a current-mode
enrollment followed by a different selection in the same transaction fails.
The specified enrollment/link checks are deferred constraint triggers. Deferred
triggers may be executed early by `SET CONSTRAINTS`; changing the head afterward
does not itself enqueue another check on the old enrollment/link rows. This
case is absent from the proposal's enforcement mechanism and acceptance floor.

Independent reproduction against the exact recovered lifecycle model:

1. R2 is already committed. In a new ordinary runtime transaction, insert a valid
   current-mode R2 message, initial enrollment, complete A/B jobs and R2 link.
2. `SET CONSTRAINTS ALL IMMEDIATE` executes the pending checks while R2 is still
   the head. Then `SET CONSTRAINTS ALL DEFERRED` restores deferred timing.
3. Create the sole new R3 transition/stream snapshot, perform its valid R2→R3
   head CAS, and commit.
4. The transaction **commits**: its new current-mode link points at R2 while the
   final head is R3. The same transaction without the timing toggle rejects
   with `stale or absent head`, rolling back both the enrollment and R3.

The independent probe also reproduces the bypass using only named constraints
`probe.membership_enrollment_complete, probe.link_complete`; `ALL` is not
required. Both complete SQL transactions and resulting state assertions are
retained in [timing-output.json](membership-version-storage-r2/timing-output.json).

To isolate this from the separate two-head timing pitfall, QA added immediate
synthetic head guards enforcing exact prior/epoch CAS, current transaction
origin and only one new transition per transaction, plus a deferred orphan
transition check. Both bypasses still commit with **one** head change. A
two-transition control rejects even across an immediate/deferred toggle and
leaves no new transitions. The finding does not depend on allowing two head
changes or on the original reduced model's missing CAS guard.

Required architecture clarification: assign a timing-independent enforcement
path for relevant head mutations, or an equivalent invariant, so prior new
enrollment checks cannot be made stale after early execution. Preserve both
valid current intake and valid prospective activation. Include `ALL` and
named-constraint immediate/deferred regressions and reject/fail closed before
an invalid commit. If the intended guarantee is only cooperative adapter
behavior, changing that explicit SQL guarantee requires a reviewed contract
decision rather than silently inheriting a weaker mechanism.

This is a gap in the proposed enforcement handoff, demonstrated on its reduced
SQL model. It is not a claim that every possible implementation of the required
invariant must fail, nor that unimplemented production migrations currently
exist. A correct implementation can add appropriate guards in the already
assigned new selection-storage leaf; no new business policy is implied.

## Original BOUNCE and revised boundaries checked

Read the updated MAIN assignment/readiness, AGENTS, exact changed proposal/result,
original QA, relevant approved contracts and actual bootstrap/fence interfaces.

- Frozen transition/stream/consumer snapshots and separately append-only
  enrollment evidence now have explicit distinct lifecycles. Future ordinary
  intake against committed R1 succeeds with no new transition. Prospective
  enrollment requires a new transition and final CAS; current/prospective
  substitution and old-enrollment repair reject. Runtime cannot supply the new
  full transaction marker. The original P1 counterexample is closed.
- `membership_enrollment_complete` is explicitly an additive deferred INSERT
  constraint trigger on the existing enrollment table, owned by MS-SELECTION's
  SQL008 leaf. It catches a missing link even when no evidence INSERT occurred,
  rolling back message/enrollment/jobs. Existing functions/triggers and SQL
  artifacts remain unchanged; the new attachment and FK attachments are
  enumerated catalog deltas. T-934 must inspect the new trigger's exact shape.
- Inert installation does not require an active head and does not modify old
  enrollment rows or fire the new INSERT trigger retrospectively. The reduced
  model preserves its old function/trigger definitions and retained row. Its
  seeded R1 is fixture setup, not an approved adoption of unknown history.
- Exact set/job controls reject mismatched link, missing job and extra consumer.
  Late B-only enrollment plus R2 commits while old A enrollment bytes stay
  unchanged; no duplicate A job is created. A later R2 allows read-only exact
  historical R1 evidence, but new stale R1 intake rejects. Unknown historic
  evidence remains held and is not silently repaired or backfilled.
- Immediate validation of an enrollment missing its link still fails closed.
  Forcing prospective checks before final CAS also rejects safely. Ordinary
  current R4 intake works after the independent timing probes; historical R1
  request/catalog and original enrollment bytes remain unchanged.
- The proposal retains a complete finite authorized universe, source-proof and
  local completeness revalidation, explicit late discovery/hold, no inferred
  h=0 or remote atomic revocation guarantee, and same-transaction finite
  bootstrap/admission/backlog/enrollment/job effects. Unknown commits still use
  a fresh authoritative exact-request read after bounded fence resolution;
  historical success after a later head is distinct from current readiness.
  These are reviewed specifications, not newly exercised full adapter behavior.
- Four aliases remain each at most4h, with eight mutually disjoint leaves and
  six explicit downstream edge additions. Read-only graph overlays are acyclic:
  this checkout934+4=938; sampled main
  `c48dc79994af0b36a85d75fad93bc473d711a0df` has944+4=948. T-591 and every owner
  root transitively include all aliases. T-928 ownership stays unchanged;
  prospective7/8 lineage passes through MS-CATALOG/MS-SELECTION to T-934/T-022.
  T-934 remains the serial checker successor. No IDs935–944 were reused.
  Bounds require reslicing before assignment if full implementation cannot fit.
- Existing Go interfaces still support outer-only bootstrap composition without
  an inbox→projection import. Independent race tests reject invalid handles
  before factories, preserve exact generation labels and require explicit
  source empty proof plus a verifier; denied verification propagates.

## Commands, results and evidence limits

Executed from the exact detached checkout:

```sh
python3 docs/justix-auto/dev/qa/membership-version-storage-r2/timing_probe.py
GOMODCACHE=/private/tmp/justixauto-t003-modcache GOCACHE=/private/tmp/justixauto-integration-gocache GOPROXY=off bash tools/go.sh test -race -count=1 -mod=readonly -v ./docs/justix-auto/dev/qa/membership-version-storage-r2/interfaces_test.go
GOMODCACHE=/private/tmp/justixauto-t003-modcache GOCACHE=/private/tmp/justixauto-integration-gocache GOPROXY=off bash tools/go.sh vet -mod=readonly ./docs/justix-auto/dev/qa/membership-version-storage-r2/interfaces_test.go
python3 docs/justix-auto/dev/qa/membership-version-storage-r2/audit.py
git diff --check
git rev-parse HEAD
```

- [timing_probe.py](membership-version-storage-r2/timing_probe.py) independently
  adds timing controls and stronger CAS guards to the hash-verified
  [recovered lifecycle model](membership-version-storage-r2/recovered_lifecycle.py).
  First execution exit0,39 observations: original27 plus12 additional controls,
  counterexample state assertions and exact-SQL records. Two assertions confirm
  an unwanted commit, so39 does not mean39 green implementation scenarios.
  [stderr](membership-version-storage-r2/timing-stderr.txt) is empty.
- [Go race](membership-version-storage-r2/go-race.txt): PASS1.705s, two test
  functions with four label subtests/eight invalid-handle combinations.
  [Vet](membership-version-storage-r2/go-vet.txt): exit0, empty output.
- [Audit](membership-version-storage-r2/audit-output.json): exact two changed
  documentation leaves; original result retained as an unchanged prefix after
  the explicit new submission notice; all seven embedded source/log blocks
  verified. All13 original QA artifacts from
  `b18bfd1f1df07c77e8afdfe0c1bae7909268aab5` remain byte-identical in sampled main.
  Both graph overlays pass. `git diff --check` passes; HEAD stays the reviewed
  SHA. SHA-256 evidence manifest is adjacent.

The single new fixture was
`justix-membership-r2-qa-0b03075e-e47d-4866-a502-cacd7737b20b`, exact container ID
`9f1f24c349524147cc1aa5c1fd5254f4dd7434021e4a47ce56e3f86c7011ca1d`.
PostgreSQL server180006/image
`postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2`.
Cleanup was registered before create, including exact-name recovery after a
lost create response. Name, independent UUID label,64-hex ID, image, network
none, absent host ports and tmpfs-only/no-volume/no-bind mounts were checked
before deleting that ID. Both ID and name absence were verified afterward.
No existing container, reference service or unknown volume was touched.

The independently executed model omits complete prospective migrations,
full actual table/FK/ACL lineage, actual Go selection capabilities, source
enumeration, complete bootstrap/recovery authority and real COMMIT-loss or
server-restart injection. No new proof of those behaviors is claimed. Their
implementation acceptance remains required. This is a documentation proposal;
no application or UI changed, so full application-suite/browser QA was not
applicable. Original BOUNCE evidence remains preserved.
