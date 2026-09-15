# Owner migration canonical promotion — GREEN

Reviewed exact commit `1a6d78b87eaa354561e36755877f8ef0a5327af3`, parent
`d667b9e3a40309df8a1fd16ac077d5d869487870`, on 2026-09-15 in detached
`.worktrees/owner-migration-promotion-qa`. No blocking promotion defect found.
This approves the canonical planning handoff for bounded implementation;
it does not approve an implemented migration driver, an existing-baseline
attestation, runtime activation or production deployment.

Read the approved proposal P1–P6, its technical adoption, prior independent
proposal QA, assignment/readiness finding, changed architecture/dependency
guidance, task files, board/index and coverage. Applied project AGENTS rules.
The independent checker was written for this review; the coordinator's checker
was not used as evidence. No database, broker, browser, dependency download or
application test was needed for this documentation-only promotion. Prior real
engine feasibility evidence is retained, not represented as new execution here.

## Reproduction and outcome

From the assigned exact checkout:

```sh
python3 docs/justix-auto/dev/qa/owner-migration-promotion/check.py
git diff --check d667b9e3a40309df8a1fd16ac077d5d869487870 \
  1a6d78b87eaa354561e36755877f8ef0a5327af3
git diff --exit-code
git diff --cached --exit-code
git rev-parse HEAD
```

PASS: [independent checker](owner-migration-promotion/check.py) and
[result](owner-migration-promotion/check-output.json). It executes 15,537
assertions, including repeated graph visits; this number is not a count of
distinct business scenarios. All 1,052 relative Markdown links encountered in
changed documents resolve. No tracked source/canonical changes were made by QA.

The first checker run compared ownership lists in order and stopped at unchanged
T-032: its task and index contain the same six leaves in a different order.
Ownership is a set, so the checker now verifies equal counts and sets while
retaining ordered dependency comparisons. This was a checker assumption, not a
promotion defect. [Original observation](owner-migration-promotion/harness-observations.txt)
is preserved; no task was edited to make the check pass.

## Verified promotion

- All 19 modified canonical files have a unique gzip snapshot whose decompressed
  bytes match the exact parent Git object, declared byte count and SHA-256.
  Changes consist only of documentation additions/modifications, with no deletion,
  SQL/application change, manifest dependency change or original mock edit.
- Both canonical proposal copies match exact reviewed proposal
  `c3734a5c3f2863be8fe752338c11740b9bdf31a4`, SHA-256
  `87ca5a5b7b5398726a53a9f4ec52b4f63c665e08ebaf69f57003401fa9ccbd76`.
  The imported result also matches; prior independent proposal QA is unchanged.
  The historical proposed header is explicitly reconciled by technical adoption.
- All 928 prior index records are preserved in full, except the eight approved
  added edges, T-059's adjacent `0012_auth.manifest.json`, and the six explicit
  OWNER-COMPATIBILITY gates on T-581–T-586. Existing statuses, branches, worktrees,
  reviewed commits, other file ownership and policy gates remain unchanged.
- OM-HISTORY/PROFILE/IDENTITY/DRIVER/RUNNER/CUSTODY map exactly to T-929–T-934.
  Each is a four-hour todo task with unassigned worktree/base, approved exact
  leaves and dependencies, B-01.AC1 contribution and required independent QA.
  Total is 934 tasks and 3,102 task-effort hours; historical baseline scheduling
  metrics remain labelled as such. The integrated count stays 35.
- The entire graph is acyclic with valid unique dependency references. Every new
  task is in T-591's dependency ancestry, and coverage retains T-591 as closer.
  All 934 board rows and task files agree with the index for status, dependencies,
  branch, ownership, and applicable gates; board identity/effort also agrees.
- Thirteen previously unassigned leaves include the T-059 manifest. T-931 is a
  serial successor to T-024's two Identity UOW leaves; T-934 follows T-930 before
  extending only its two common checker leaves. Identity and custody work are
  disjoint. Remaining-owner UOW changes are recorded follow-ups, not concurrently
  granted ownership or completed seven-owner rollout.

## Contract and activation boundaries

Technical adoption correctly records the actual migrate engine with a project
pgx database.Driver and verified immutable source as an ADR-13 refinement.
Existing migrate/pgx pins remain; no lib/pq or alternate engine is authorized.
The coordinator retains module/transitive insertion before exact implementation
QA. T-932/T-933 explicitly require `./tools/owner-migrate` checks and real engine
and PostgreSQL fault tests; ordinary services/pkg/tests patterns do not cover it.

The handoff preserves exact private history versus independent shared marker
lineage, legacy constructor behavior, read-only checks before binding and Run,
narrow required/forbidden grants, and a separate custody checker. Complete Lock,
including preflight, is bounded. Fresh-v1 finalization checks original mechanics
without requiring nonexistent history and cannot authorize an existing/feature
upgrade. Subsequent artifact receipt and clean head commit atomically; missing
Run, Force, dirty/lower-version gaps and ambiguous replies cannot silently pass.
Actual baseline attestations, complete release profiles and migration execution
remain separately required. A healthy database does not establish source admission,
consumer effects or runtime assembly.

T-059/060/022/580 gained exactly their approved prerequisites; T-061 inherits
Identity compatibility. T-581–T-586 retain explicit OWNER-COMPATIBILITY gates,
and the board requires concrete manifest/UOW follow-ups before later assignments.
Security/recovery/auth transport, business permissions, privacy, financial/legal
policies and production settings remain separate. Later commits outside this
exact promotion target were not included or represented as tested here.

Only this assigned QA report and its evidence directory were written. No commit,
merge, task transition, fixture activation or canonical fix was performed by QA.
