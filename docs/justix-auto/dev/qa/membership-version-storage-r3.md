# Membership-version storage proposal r3 QA — GREEN

2026-09-16. Exact reviewed commit:
`bb908b4425003cbf41375b46b07fb88d2a3be817`.
Detached checkout: `.worktrees/membership-version-storage-r3-qa`.
Proposal SHA-256:
`c468216706f0b40e01dc5c5e2bf07060cac60cecd429eac54ec5616bc9886330`.

**GREEN for the bounded architecture proposal and its technical handoff.** Both
prior findings are resolved by an explicit feasible mechanism. No blocking
proposal finding remains. This is not implementation, migration, configuration,
source-authority or production-readiness approval. Canonical promotion QA and
the explicit implementation/dependency gates still precede dependent work.

## Prior findings and final mechanism

Read AGENTS, the updated MAIN assignment/readiness, exact proposal/result and
both earlier QA reports, relevant approved contracts and actual Go interfaces.

- Original P1: frozen transition snapshots and future enrollment evidence remain
  separate. Ordinary future intake references a committed current transition;
  prospective evidence belongs to a still-pending transition, with jobs/link
  preceding its final CAS. Historical exact redelivery reads the old retained
  lifecycle without requiring its transition to equal today's head or reader
  transaction. Missing historic evidence is held, not backfilled.
- R2 P2: immediate ordinary head/link guards now cover both temporal orders.
  A head mutation rechecks all this transaction's new evidence/enrollments;
  a new link validates the current or prospective selection and exact present
  job set. Deferred constraints are explicitly not commit hooks. Early ALL,
  paired named and individually named checks cannot disable these guards.
- The link seals its job set. A new immediate job INSERT trigger forbids adding
  a job afterward, including after early validation. Snapshot child insertion
  rejects after selection; immutable snapshot rows cannot be changed later.
  Missing/orphan records still schedule their own completeness checks.
- Current links retain a head `FOR SHARE` lock; prospective links retain the
  expected head `FOR UPDATE` lock. The adapter obtains these before job/effect
  work and the SQL guard independently revalidates them. Separate concurrent
  head mutation cannot invalidate an early-checked current link before commit.
  A later transaction can advance normally, preserving historical evidence.
- Server BEFORE guards overwrite creation xid8 and head-selection bookkeeping;
  runtime grants exclude those columns. The head checks OLD bookkeeping before
  every selection, so a second retained selection fails after constraint
  flushing or an attempted supplied marker. Savepoint rollback restores both
  pointer and marker; rolling back a failed second attempt retains the first.
  Enrollment writes in unsupported subtransactions fail top-level provenance.

These rules are explicit SQL obligations plus the existing typed authority/fence
checks. They do not downgrade the final-head guarantee to cooperative-only Go
behavior, infer remote source freshness, or provide an ACK capability.

## Executed independent PostgreSQL checks

[safe_probe.py](membership-version-storage-r3/safe_probe.py) is a standalone
reduced model built from the exact recovered lifecycle/guard source plus
independent cases. It was statically checked to exclude security-setting
mutations and dynamic execution. First approved execution exited0 on pinned
PostgreSQL18.6/server180006; [output](membership-version-storage-r3/safe-output.json)
records76 observations, including engine identity, exact SQL traces and cleanup.
These are58 retained lifecycle/timing observations and18 additional QA
observations, not76 independent business acceptance scenarios.

- Empty R1 followed by ordinary initial enrollment succeeds; late prospective
  B-only delta and R2 select atomically; original A enrollment remains unchanged.
  Missing/mismatched links, missing/extra jobs, old-enrollment repair,
  stale-current and wrong prospective/current lifecycle reject with rollback.
  Inert trigger installation preserves retained unknown history without adoption.
- The exact r2 shape—valid current link, early constraint flush, sole next-head
  CAS—rejects immediately. Head-first then stale link also rejects. Both ALL and
  paired named forms pass the negative regressions, as does the no-toggle
  control. Independent variants toggle each named constraint alone in both
  orders and attempt to append a job after sealing; all reject as required.
- CAS before pending enrollment gains a link, prospective link after selection,
  and snapshot additions after selection/flush reject. Second head selection
  rejects under both timing forms. A single legitimate selection and normal
  current intake remain valid.
- Rolling back a complete first selection to a savepoint permits one retained
  replacement. Catching/rolling back a failed second selection preserves the
  first head/marker. Independent enrollment creation within a subtransaction
  rejects before link creation, leaving no message/enrollment. This verifies
  the stated unsupported-savepoint-write boundary rather than inventing support.
- Privileged synthetic transition/head and independently supplied link stamps
  are overwritten by the server. Runtime bookkeeping-column UPDATE rejects.
- Forward concurrency: a current link flushes ALL checks while its transaction
  stays open; another head UPDATE hits its150ms lock timeout and leaves no
  transition. After the link commits, a later head change succeeds. An additional
  independent run repeats this with paired named checks and reaches the same
  result, preserving the committed historical link.
- Reverse concurrency: the head writer updates first and stays open. Stale
  current intake waits1.938s, then rejects after the writer commits because its
  lock-taking SELECT observes the new head. No stale message/enrollment survives.
- Read-only configuration observations show current origin mode alone does not
  establish the exact LOGIN/database setting prerequisite and that this fixture
  runtime has no effective parameter-setting authority. No configuration setting
  was changed by the approved probe; the limitation below remains explicit.

The recovered base has synthetic tables, trusted fixture seeds and reduced
constraints. Successful execution proves the tested guard/locking mechanism,
not full prospective migrations, every real FK/preimage, complete finite source
discovery, authority, sealed Go selection capability or actual owner assembly.

## Storage scope, configuration and dependency audit

The final contract defines exactly two new custom trigger attachments on old
tables: the enrollment deferred INSERT completeness trigger and job ordinary
BEFORE INSERT sealing guard. Other guards belong to new membership tables.
Declared new-FK attachments are additional known catalog deltas. No old SQL,
column, grant or existing trigger/function is rewritten. The model verifies
the job trigger's ordinary enabled BEFORE INSERT shape and retains the old
enrollment trigger definition; full artifact preimages are implementation QA.
T-934 owns exact guard/column/marker/privilege inspection, including immediate
versus deferred shape, and storage compatibility does not require an active head.

Configuration is explicit and fail closed: exact runtime LOGIN plus its exact
role-OID/database-OID `session_replication_role=origin` setting, applicable
default and reachable SET/ALTER SYSTEM authority audits, and actual origin
mode on the supplied runtime handle. Installer/session-only or SET ROLE proof
is insufficient. External approved provisioning supplies the setting; migrations
inspect it read-only. T-022/T-933 must verify a fresh exact-runtime login under
the actual configuration, and T-934 rechecks the supplied handle before factory
binding. Unknown effective configuration stays unready. The dependency on
T-928's final independently approved/integrated contract is preserved, and no
T-928 artifact hash or guarantee is inferred from its pending worktree.

The complete owner catalog, immutable consumer meaning, explicit owner/stream
selection, finite complete authorized universe, late discovery hold and exact
request receipt still distinguish empty-stream and zero-delta transitions.
Unknown commits reconcile exact historical requests under fresh authoritative
fences even after later heads, without confusing old success and current
readiness. Missing source proof/bytes or retained-adoption authority remains
held. No current-state recovery, partial activation, old obligation deletion,
cross-owner mutable table or reverse inbox→projection dependency is introduced.

[audit.py](membership-version-storage-r3/audit.py) independently verifies:

- Exactly the original two documentation leaves changed; the previous result
  is an unchanged prefix after removing the explicit final-submission notice.
  All nine embedded source/log hashes match. Both earlier60-observation author
  logs reconstruct to their stated hashes; reconstruction is not execution.
- All25 earlier QA artifacts remain byte-identical on sampled main
  `23d8cede88d228ef57fa73205c159b07e57a49b2`, compared with original import
  `b18bfd1f1df07c77e8afdfe0c1bae7909268aab5` and r2 import
  `97246d5c0b803786b78b7a32ce650137e8aa97dc`.
- Four aliases remain each at most4h with eight disjoint leaves and six explicit
  downstream additions. Both overlays are acyclic: reviewed934+4=938 and sampled
  main944+4=948. All aliases reach T-591 and all seven owner roots. T-928's two
  leaves are unchanged; MS-CATALOG waits for T-928, MS-SELECTION supplies8 after7,
  T-934 remains the serial checker successor, and T-022 retains both storage
  and state dependencies. IDs935–944 are not reused or reserved by this proposal.
  Expanded guards are explicitly assigned to SQL008/test and state leaves;
  source ownership is not hidden in old migration files. The stated4h/3h bounds
  require reslicing before assignment if the implementation cannot fit.

## Go checks, commands and fixture ownership

Independent actual-API tests were rerun at this exact commit:

```sh
python3 docs/justix-auto/dev/qa/membership-version-storage-r3/safe_probe.py
GOMODCACHE=/private/tmp/justixauto-t003-modcache GOCACHE=/private/tmp/justixauto-integration-gocache GOPROXY=off bash tools/go.sh test -race -count=1 -mod=readonly -v ./docs/justix-auto/dev/qa/membership-version-storage-r3/interfaces_test.go
GOMODCACHE=/private/tmp/justixauto-t003-modcache GOCACHE=/private/tmp/justixauto-integration-gocache GOPROXY=off bash tools/go.sh vet -mod=readonly ./docs/justix-auto/dev/qa/membership-version-storage-r3/interfaces_test.go
python3 docs/justix-auto/dev/qa/membership-version-storage-r3/audit.py
git diff --check
git rev-parse HEAD
```

[Go race](membership-version-storage-r3/go-race.txt) PASS1.733s, two tests/four
label subtests/eight invalid-handle combinations. [Vet](membership-version-storage-r3/go-vet.txt)
exit0. Actual T-014/PreparedFences outer composition rejects invalid handles
before binding, preserves ordinary/space/tab/padded labels and requires explicit
h=0 proof plus an authority verifier. No new Go API or package dependency is
claimed implemented. Audit exit0 and diff check pass; HEAD is unchanged.

The approved probe created only
`justix-membership-r3-safe-fad054db-0fef-4b78-8554-fac11b82e8fd`, ID
`38a2943656e2de3dbbfcc14c89f5e0a8e38bb60afee345bd768bf534fb2219d8`.
Image:
`postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2`.
Cleanup was armed before create; exact UUID name/independent label,64-hex ID,
image, network none, absent host ports and tmpfs-only/no-volume/no-bind mounts
were validated before removing the exact ID. ID and name absence were checked.
No existing container/reference infrastructure/unknown volume was changed.

## Verification limits and approval-review boundary

The initial broader probe was rejected by automatic approval review before any
process or fixture creation because it included persistent role/database
replication-setting changes and parameter grants. The rejected plan is retained
as unexecuted evidence. It was not rerun indirectly. The approved standalone
safer probe removes every such mutation and only performs read-only parameter
and setting observations; [approval-limit.md](membership-version-storage-r3/approval-limit.md)
records the rejection and exact safer scope.

Consequently, fresh direct-runtime-login setup/verification and the full
dangerous-default/reachable-parameter negative matrix were **not executed** by
this QA. No server-wide configuration change was attempted. Those are explicit
implementation/startup prerequisites in the reviewed contract, not established
facts. Complete prospective migrations, real COMMIT-loss/server-restart tests,
finite authority/backlog and actual owner adapters remain implementation QA.
No application/UI source changed, so full application-suite/browser checks were
not applicable. This bounded architecture GREEN does not waive any such gate.
