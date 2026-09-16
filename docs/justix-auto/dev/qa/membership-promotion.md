# Durable membership canonical promotion QA — GREEN

2026-09-16. Exact reviewed promotion:
`b50490b48c5f31a542b0305827c1e345c2214cf6`.
Exact parent: `9df6d1fb7aaef10368d8db8177351dc000e404eb`.
Detached checkout: `.worktrees/membership-promotion-qa`.

**GREEN for canonical technical adoption and the bounded task mapping.** No
blocking documentation, graph, ownership or evidence-preservation finding.
This review does not approve configuration mutations, assign implementation,
integrate T-928, authorize retained-history adoption or establish runtime
readiness. Existing per-task dependencies and explicit approval gates remain.

## Exact contract and approval limits

The canonical contract exactly equals the architect draft at reviewed proposal
`bb908b4425003cbf41375b46b07fb88d2a3be817`, SHA-256
`c468216706f0b40e01dc5c5e2bf07060cac60cecd429eac54ec5616bc9886330`.
The technical approval pins those bytes and links the actual r3 proposal GREEN.
Historical PROPOSED wording is deliberately preserved; the separate approval
provides the technical adoption decision without rewriting reviewed evidence.

The approval accurately calls the76 PostgreSQL observations reduced model
evidence. It preserves the rejected configuration-test boundary, unexecuted
fresh runtime-login verification, pending explicit authorization and T-928
independent verification/integration. External exact-runtime LOGIN/database
origin provisioning, quiescence, actual defaults/parameter-authority and runtime
handle checks remain required. No installer-session or opaque evidence reference
is promoted into startup proof. No configuration mutation or activation is
authorized by this canonical change.

## Independently checked task and ownership promotion

| Alias | Task | Maximum hours | Direct dependencies |
|---|---|---:|---|
| MS-CATALOG | T-945 | 4 | T-928 |
| MS-SELECTION | T-946 | 4 | T-945 |
| MS-IDENTITY | T-947 | 4 | T-746, T-007 |
| MS-STATE | T-948 | 4 | T-946, T-947, T-014 |

All four tasks are todo/unassigned at this exact promotion. Their index records,
task files, board rows, exact eight leaves and approval mapping agree. New leaves
are mutually disjoint and do not overlap existing ownership. Existing T-934
remains the serial checker successor of T-930. Catalog/selection SQL, codec and
fenced-state ownership remain distinct; the two declared custom old-table
trigger attachments belong to the new selection SQL leaf, not rewritten old
migration files.

Compared with the exact parent, all944 predecessor task records are identical
except these six appended dependencies:

- T-920→T-948, T-921→T-948, T-016→T-948.
- T-022→T-946 and T-022→T-948.
- T-934→T-946.

The actual948-node graph is acyclic with no missing dependency; summed effort is
3154hours and44 records are integrated. All948 board rows match task status,
parent, title, kind, effort, lane, dependencies and branch. The previous baseline
longest-chain/frontier/width figures remain explicitly scoped to the916-task
baseline; this promotion makes no recomputed width or elapsed-schedule claim.

T-591 and each of T-580…T-586 transitively include all four additions. Only the
B-01.AC2 contribution row changes, adding exactly T-945…T-948 while preserving
T-591 as the combined closer. The linked T-920/921/016/022/934/591 handoffs retain
full finite authority/backlog, historical reconciliation, guard/lock sequencing,
and actual startup verification. T-920 stays blocked with its3h maximum and an
explicit reassessment/reslice requirement before resuming. Pure identity work
is explicitly unaffected by the restricted configuration tests, but still
requires the coordinator's normal assignment and independent implementation QA.

No source authority, partial activation, current-state recovery, unknown historic
catalog adoption or ACK permission is inferred from the technical contract.
The incidental dev-state correction describing T-936 as integrated is supported
by its already-integrated record in the promotion parent.

## Preimages, links and preserved evidence

- Exactly31 documentation paths change:12 existing canonical documents, four
  task documents, approval/contract,12 gzip snapshots and their manifest.
  No application source, dependency lock, original SQL, deletion or rename.
- Every existing changed canonical document has a unique recoverable snapshot.
  All12 gzip payloads equal the exact parent Git bytes and match the recorded
  SHA-256 and byte length. No existing canonical edit lacks a preimage.
- All1024 relative local link targets in changed Markdown files resolve within
  the reviewed Git tree, including the moved exact contract and downstream
  task/approval links. This count includes repeated references and board links.
- All40 earlier membership QA artifacts are byte-identical to the exact parent:
  original13, r2's12 and r3's15. Earlier BOUNCE reports, safe probe evidence and
  the exact automatic-review rejection record remain preserved.

The later main T-931 status transition is outside this review. All baseline and
preservation comparisons use the stated parent and promotion SHAs, never the
moving main worktree.

## Reproducible verification

Executed in the detached exact promotion checkout:

```sh
python3 docs/justix-auto/dev/qa/membership-promotion/audit.py
git diff --check
git rev-parse HEAD
git rev-parse HEAD^
git status --short
```

[audit.py](membership-promotion/audit.py) exited0 on its first execution.
[audit-output.json](membership-promotion/audit-output.json) records12,996
assertions, including repeated graph visits and board comparisons; this is not
a count of independent runtime scenarios. Its complete path/hash/link/closure
records permit inspection without relying on the coordinator's summary.
[audit-stderr.txt](membership-promotion/audit-stderr.txt) is empty.
`git diff --check` passes; HEAD remains the reviewed SHA. Evidence hashes are
recorded in the adjacent manifest.

No Docker, runtime, configuration or application test was run for this
documentation-only promotion, as assigned. Only this new report/evidence
directory was written; nothing was committed or canonically edited by QA.
