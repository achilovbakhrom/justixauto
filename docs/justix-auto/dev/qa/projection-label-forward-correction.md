# Projection label forward correction — independent QA

**GREEN — bounded technical proposal only.** Reviewed commit
`f3fc029de3da3dbadb0cab980ef6bd33c86723b3`, detached checkout
`.worktrees/projection-label-correction-qa`. Proposal SHA-256:
`704d2c65eedbe1e98f36bc0270d6ce366ad7cd38cccc2fe94b25e2a3ffa9f196`.
Reviewed on 2026-09-15 by `qa_projection_label`.

## Scope and source findings

The commit adds only the proposed architect document. Independent source checks
confirm 000002 admits exact nonempty generation text while 000004 applies
`length(btrim(generation))>0`. The approved generation-label clarification
requires exact retained labels, including process labels without a projection
UUID requirement. The coordinator's historical overstatement of all-space test
coverage remains explicitly recorded in `dev/projection-label-readiness.md`;
neither this review nor the proposal relabels historical implementation QA.

The correction fits T-928's existing two exclusive leaves. Its prerequisite is
T-927, and T-016/T-934 already depend on T-928. All three remain todo at the
reviewed commit; no new tasks, dependency edges or source ownership are needed.
The owner-migration P5 OM-CUSTODY checker ownership already covers the proposed
revision-6 object predicate check. Canonical adoption must append the exact
correction requirements to T-928, T-016 and T-934 before assigning affected work.

The proposal's revision-6 marker inherits the approved storage handoff's
immutable runtime-SELECT-only singleton, exact supplied artifact hash, complete
prior lineage, owner/runtime checks and installation evidence. SQL cannot hash
its own input or attest stopped processes. The runner verifies actual artifacts.
Revision 4 or mere base mode 2 cannot establish the correction; downstream
revision-6 readiness must check the corrected object predicate as well as the
exact marker/hash. This is compatible with P5's read-only custody checker and
does not establish adapter assembly, admissions or business effects.

000002/000003/000004 hashes remain respectively `1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2`,
`c198af498e39056a7a251b5906d6db050ad994e04588bf6cecf8316221ea85ec`, and
`41536429fb95d4b60a11866843a2ccbd6a4cf723cd919884933b6bc1d8202c4c`.
The actual admission equality trigger compares kind/generation without trimming.
Other reference fields' btrim checks remain unchanged. Read-only inspection of
T-014 at `3a166bc39ed93eed13662a951da1626fab01fcb0` confirms its generation
constructor uses exact nonempty valid UTF-8/NUL-free text and its result records
the existing storage rejection. This is source confirmation, not a renewed
implementation QA verdict for that separate task.

## Executed checks and evidence

From the detached checkout:

```sh
python3 docs/justix-auto/dev/qa/projection-label-forward-correction/check.py
python3 docs/justix-auto/dev/qa/projection-label-forward-correction/probe.py
git diff --exit-code
```

All final commands exited 0. The static checker verifies exact HEAD/proposal
scope, immutable artifact hashes, task ownership/dependencies/status, and the
retained historical finding. The live probe passed 37 assertions plus owned
container removal (38 assertions total), using PostgreSQL server `180006` from
the approved digest `sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2`.

The probe executes the actual unmodified 000004 `consumer_bootstraps` CREATE
TABLE statement with a minimal synthetic referenced admission table and separate
migration/runtime roles. It intentionally does not install the actual admission
trigger, complete migration lineage, or a real revision-6 migration. Findings:

- The installed CHECK is `consumer_bootstraps_generation_check`, column vector
  `{7}`, validated/enforced/local with no inheritance. `pg_get_expr` under
  `search_path=pg_catalog` yields `(length(btrim(generation)) > 0)`.
- Exact catalog identification rejects missing, renamed, duplicate, altered and
  unvalidated variants. Every trial rollback restores the original catalog.
  The pinned server rejects disabling this CHECK using ALTER CONSTRAINT; the
  probe records that denial and retains validation/enforcement checks.
- Default btrim removes U+0020 spaces, but leaves tab and NBSP. The old real
  table CHECK rejects one/multiple spaces and empty text, while admitting a tab.
- A precise same-name CHECK replacement preserves retained row values, other
  constraints/foreign keys and ACLs. It admits exact spaces, tabs, padded and
  ordinary non-UUID values and still rejects empty text. Runtime cannot perform
  that DDL. No normalization or row rewrite occurs.
- A forced later failure in the same explicit transaction rolls back both the
  CHECK replacement and a synthetic revision marker, retaining the old rows.

Evidence is in [the assigned directory](projection-label-forward-correction/):
`check.py`, `check-output.txt`, `probe.py`, `probe-output.txt`, and
`probe-attempt-1.txt`. The first probe attempt assumed ALTER CHECK NOT ENFORCED
was supported; PostgreSQL rejected that setup. The attempt and fixture cleanup
are preserved; the final probe tests the denial. An initial manual hash command
used the wrong 000003 filename; the checked-in static checker uses the actual
`000003_messaging_route_compatibility.up.sql` and verifies its exact hash.

## Limits and handoff

The fixture used a unique network-isolated container, tmpfs PostgreSQL storage,
no published port and no attached Docker volumes. Both attempt containers were
removed and absence checked. No existing container, DSN, service, reference
server, volume or production state was changed.

GREEN establishes that the proposal is precise, feasible and within approved
ownership. Full T-928 implementation QA must still prove real owner/runtime/prior
hash rejection, complete marker immutability, ordered five-second lock timeout,
both projection/process admission-bootstrap-checkpoint flows, exact mismatches,
all retained function/OID/ACL preimages, duplicate install behavior, and failure
atomicity across the complete revision-6 script. The bounded probe does not
prove those full installation/runtime guarantees. No production migration,
privacy policy, process replay, checkpoint reset or admission is authorized here.
