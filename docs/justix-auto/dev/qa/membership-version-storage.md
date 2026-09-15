# Membership-version storage proposal QA — BOUNCE

2026-09-15. Exact reviewed commit:
`5996c77e3df371a3992154b639ce9ba91ffc3e75`.
Base: `d59faa8dc5b26e13e19c47686dd47744b6d73a11`.
Detached checkout: `.worktrees/membership-version-storage-qa`.
Proposal SHA-256: `74ec431e86b33346c82f28c8181fd671720f7456ba675b87ea8a6a4475de68ce`.

**Do not promote this exact proposal.** One storage-protocol ambiguity must be
resolved before MS-SELECTION/MS-STATE and T-920/T-921 implementation. This is a
documentation-only architecture review, not an application-release assessment.
No source, canonical document, task assignment or dependency was changed by QA.

## P1 — distinguish frozen transition children from future enrollment evidence

The proposal's [§4, lines 185–193](../../state/drafts/architect/membership-version-storage.md)
groups `dispatch_membership_evidence` with transition/stream/consumer/head rows
under deferred completeness and post-commit child-addition checks, then requires
the transition row to originate in the current transaction. Lines 201–209 require
an evidence link for every new initial or late enrollment, including future
ordinary intake. §5 also requires ordinary intake to use the already committed
current membership head. These requirements need an explicit distinction.

Counterexample: select V1 for source-proved empty S and commit R1 with no messages.
The first later valid message needs its initial enrollment and a link to R1/S.
R1 necessarily belongs to an older transaction. Applying the stated common
transition provenance guard to the evidence link rejects this otherwise valid
intake and rolls back the enrollment. Creating another membership transition for
each message is not the approved protocol and would not be a valid workaround.

Independent pinned PostgreSQL18.6 reproduction in [probe.py](membership-version-storage/probe.py)
uses a reduced schema and the literal deferred provenance rule. Its
`future ordinary intake fails under literal transition guard` case rejected at
commit with `transition already committed`; the next query confirmed zero
enrollments and the unchanged epoch1 head. The probe then changed only its
synthetic link guard to provenance of the new enrollment: later intake committed
against an older transition while late inserts into frozen transition streams
still rejected. This contrast establishes feasibility of separating the two
lifecycles; it is not an implementation or authorization of revised SQL.

Required clarification:

- Freeze the declared transition/stream/consumer snapshot within its creation
  transaction; retain the no-orphan/no-late-child/CAS guarantees.
- Define per-enrollment evidence as separately append-only. Its new enrollment,
  exact jobs/set and evidence must be one transaction, but its referenced
  membership transition may already be committed. Specify current-head checks
  for ordinary intake versus the sealed prospective-selection path during
  activation and historical exact-duplicate reconciliation.
- State which completeness checks are SQL-enforced and which are mandatory
  typed-adapter checks, including a missing link (where a trigger only on link
  insertion cannot fire). Preserve the declared migration ownership and existing
  artifact/trigger boundaries; do not silently add unowned old-table rewrites.
- Add positive future-intake-after-empty-selection and negative late mutation,
  mismatched/omitted evidence and stale-head cases to the assigned acceptance
  floor. Fix the architecture text before delegating implementations.

The finding is a conflicting/ambiguous contract, not a claim that unimplemented
production migrations currently fail. Interpreting the creation clause as only
applying to snapshot children could avoid the failure, but that exception and
the replacement evidence rule are absent from this exact reviewed text.

## Independent checks and accepted design boundaries

Read AGENTS, MAIN assignment/readiness, the exact proposal/result, task board and
T-920/T-928/T-934, approved messaging/storage/registration/owner-migration and
label contracts, actual 000002–000005, PreparedFences and T-014 APIs. Product/open
rules and both Gaze reference audits remain authority limits, not fixture policy.

- The change contains exactly two assigned documentation leaves. All application
  source, prior SQL, canonical task state and T-928 ownership are unchanged.
  Hashes of the seven inspected SQL/API leaves are retained in the audit output.
- The five distinct identities cover immutable consumer meaning, complete inert
  owner catalog, owner selection, per-stream selection and exact transition.
  Version-only and zero-consumer-delta transitions get durable receipts even
  with no custody rows; owner-local epochs do not claim a cross-service sequence.
- Complete local retained streams plus finite source-authorized discovery,
  explicit zero applicability, late-stream admission, under-fence revalidation,
  omission/raced-boundary abort and no inferred h=0 are stated. Local fences do
  not purport to make remote source revocation atomic. Retained installations
  without historic catalog proof remain inert pending separately approved
  adoption rather than inventing a prior version.
- The described same-transaction bootstrap/admissions/snapshots/finite original
  backlog/enrollment/jobs and final head CAS preserve frozen old sets. Recovery
  preparation occurs outside SQL; process catch-up needs distinct authority.
  Unknown commit uses fresh authoritative exact-request history after bounded
  conflicting-fence resolution, separating historic success from current
  readiness. Absence while an old transaction might still commit is not rollback.
- A reduced independent SQL model checks changed same-version/digest rejection,
  omitted-stream rollback, one winner between competing successors, zero-delta
  receipts, historical exact-request lookup after a newer head and changed
  request conflict. Narrow runtime permission checks reject receipt UPDATE and
  head DELETE; a separate role cannot read the model owner's schema. Duplicate
  enrollment evidence is rejected. These are model properties, not the full
  proposed migration grants, source discovery or actual dispatcher behavior.
- Independent Go compilation uses the actual T-014 and PreparedFences APIs.
  An outer adapter exposes only a typed `Install(context.Context)` action while
  capturing the actual transaction/fences. Valid contract construction accepts
  exact ordinary/space/tab/padded generation labels. Nil and invalid handles
  reject before binding/revalidation. Zero high-water requires explicit proof
  and a nonnil verifier; denied authority propagates. This establishes the
  interface composition, not live multi-effect atomicity or a reverse inbox
  dependency on projection. No existing package import was altered.
- Four aliases each have the explicit at-most4h bound, eight mutually disjoint
  leaves, no root manifest ownership and no collision with existing tasks.
  Independent graph overlays are acyclic: reviewed 934+4=938 nodes; separately
  sampled main `6b3685bebefb556eab2a30afb26cbae909752d97` has 944+4=948 nodes.
  These are checks, not task allocation. Six explicit downstream additions
  connect T-920/921/016/022/934; T-591 and all seven owner roots transitively
  include every alias. T-934 remains the serial successor of T-930's checker.
  MS-CATALOG waits for unchanged T-928; MS-SELECTION supplies prospective rev8
  after7; T-022/T-933 bundle requirements enumerate both. T-920's remaining
  orchestration must be resliced if it exceeds its retained3h bound.
- Required full lineage, markers, owner/runtime identities, immutable guards,
  narrow direct/column/default/PUBLIC/reachable-role privileges, unselected
  storage compatibility versus false membership readiness and full rollback
  are explicitly assigned to implementation QA. Proposal approval would not
  establish any of those unimplemented runtime properties.

## Commands, evidence and limitations

Executed from the exact detached checkout:

```sh
python3 docs/justix-auto/dev/qa/membership-version-storage/probe.py
GOMODCACHE=/private/tmp/justixauto-t003-modcache GOCACHE=/private/tmp/justixauto-integration-gocache GOPROXY=off bash tools/go.sh test -race -count=1 -mod=readonly -v ./docs/justix-auto/dev/qa/membership-version-storage/interfaces_test.go
GOMODCACHE=/private/tmp/justixauto-t003-modcache GOCACHE=/private/tmp/justixauto-integration-gocache GOPROXY=off bash tools/go.sh vet -mod=readonly ./docs/justix-auto/dev/qa/membership-version-storage/interfaces_test.go
python3 docs/justix-auto/dev/qa/membership-version-storage/audit.py
git diff --check
git rev-parse HEAD
```

- [Final SQL output](membership-version-storage/probe-attempt2-output.json):
  exit0, 20 recorded observations including engine identity and cleanup; one
  confirms the blocking contract counterexample, so these are not20 green
  implementation scenarios. PostgreSQL server180006, pinned image digest
  `sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2`.
- [Go race output](membership-version-storage/go-race.txt): PASS1.459s, two test
  functions with four label subtests and eight invalid-handle combinations.
  [Vet output](membership-version-storage/go-vet.txt) empty, exit0.
- [Audit output](membership-version-storage/audit-output.json): exact source
  scope/hashes, two dependency overlays and five recovered author artifact
  hashes pass. Matching embedded source/log hashes do not substitute for
  independent execution; the author's fixture script was not rerun.
- `git diff --check` passed; final HEAD remains the reviewed SHA. Only this new
  report and its evidence directory are written. No QA artifact is committed.

The first QA probe attempt stopped before Docker creation because its preflight
recognized capitalized missing-object text while this CLI returned lowercase.
[Original stderr](membership-version-storage/probe-stderr.txt) is retained, and
only that case-insensitive diagnostic was fixed in the QA harness. Attempt2
used UUID container `justix-membership-qa-4cfaebe3-249a-4875-9d53-07440f84a6bf`,
ID `6cc3667b14256a0ac98686835c54f682fbb46b59f36c57e1e615fab31586d12e`.
Cleanup was armed before create, including lookup after a lost create reply;
exact64-hex ID, name, independent UUID label, pinned image, tmpfs and no
volume/bind mounts were verified before removing that ID. ID and name absence
were checked afterward. It had no host ports and network none. No existing
container, reference infrastructure or unknown volume was changed.

No full prospective migration, old enrollment preservation, server crash/restart,
actual lost-COMMIT reply, remote discovery, crypto/policy enforcement, bound
executor or browser behavior was exercised by this architecture QA. Those are
explicit implementation acceptance cases. No application code changed, so the
full application test suite and UI visual comparison were not applicable to
this documentation proposal.
