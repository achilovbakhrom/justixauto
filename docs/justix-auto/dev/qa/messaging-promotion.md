# Messaging canonical promotion — independent QA

**GREEN — canonical documentation and task graph only.**

Reviewed commit: `3168a653bde21a4432f1fcf62c01c207c138bbe7`.
Date: 2026-09-15. Reviewer: `/root/qa_messaging_delivery`.
Worktree: `.worktrees/messaging-promotion-qa`.
Exact HEAD was unchanged before and after review. Only this new report and its
assigned QA script were written. No canonical, application, lock or migration
file was changed; no merge or runtime test was performed.

## Checks executed

- `git rev-parse HEAD`, `git status --short`, `git show --stat --oneline HEAD`:
  exact requested commit, initially clean; commit is documentation-only.
- `git diff HEAD^ HEAD --check` and `git diff --check`: exit 0.
- `python3 docs/justix-auto/dev/qa/messaging-promotion/check.py`: PASS.
  [Repeatable independent checker](messaging-promotion/check.py) verifies exact
  contract bytes, all modified preimages, snapshots, every graph node/edge,
  original task preservation, new leaf ownership and dependency reachability.
- Pinned Node 24.21.0 command:
  `node docs/justix-auto/state/planning-evidence/validate-readiness.cjs docs/justix-auto/dev/task-index.json`:
  **1,016/1,016 focused readiness checks pass, zero errors**.
- Independent relative-link scan of new approval, contract and eight task files:
  **26 links resolve**. Manual diff of intermediate board/state snapshots shows
  only the separately recorded T-036 allocation before messaging promotion.

The actual Node executable used was
`/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin/node`.
No dependency install was needed. These results do not claim application,
PostgreSQL, RabbitMQ, migration or browser behavior.

## Contract and canonical adoption

The promoted `state/drafts/contracts/messaging-delivery.md` is byte-identical
to the architect draft at previously reviewed
`ba1723fdcbbfd9a0bbccdf6736e9346052d80fd7`. Both SHA-256 values are
`97e70c29183feded9e58f702ba2c995dc67b8ad4e6896c5f3e877b49db94c7d7`.
Its retained PROPOSED header is explicitly explained by the separate approval,
which names the exact reviewed SHA and earlier independent QA.

Architecture §5 accurately adopts immutable parent/per-recipient delivery and
distinguishes direct single-consumer ACK from durable custody ACK. It preserves
atomic logical inbox/effect/checkpoint/job completion, all admitted positions,
authorized gap recovery and the prohibition on mixed modes. The dated T-746
addendum makes intake the only AMQP competitor while logical projection/process
consumers use SQL jobs and complete inert claims. It does not turn registration
into authorization or rewrite enrollment history. Existing direct-ACK text is
scoped by the explicit addendum, rather than silently treated as custody mode.

Approval, readiness and development state retain runtime implementation gates,
historical primitive evidence and unapproved concrete data grants, private
snapshots, process catch-up and immediate-revocation disposition. No new business
policy, dependency pin or runtime availability claim was introduced.

## Graph, scope and ownership

Independently traversed **all 924 unique IDs and keys**, rejecting unknown,
duplicate or cyclic dependencies. The original 916 task objects are unchanged
except the ten expected dependency additions and T-036 allocation metadata.
All eight new tasks remain `todo` with no worktree/base assigned; task text keeps
Git, approved-contract and exact-commit QA gates. Existing scoped policy gates
are unchanged. Total effort sums to 3,062 task-hours; historical chain/frontier/
width metrics explicitly describe the original 916-task baseline. Current board
and state report 924 tasks and 19 integrated tasks.

| Task | Verified responsibility and dependency boundary |
|---|---|
| T-917 | Additive migration and migration test only; complete immutable technical records, mode fences, populated legacy preservation, no destructive down migration. |
| T-918 | Source admission pair after schema and T-009; same-transaction full recipient plan and authorized cutovers. |
| T-919 | Existing insertion pair after T-918 and integrated T-011; complete parent/children and explicit legacy mode. |
| T-920 | Local membership pair after schema/T-009/T-746; complete role membership and fenced backlog enrollments. |
| T-921 | Intake pair after membership/T-013; durable custody before ACK, original duplicate membership and unknown-commit withholding. |
| T-922 | Existing consume pair after T-013; definite composable transaction interface, direct-ACK compatibility and prior-inbox recovery without reapplying effects. |
| T-923 | Dispatch pair after T-921, **T-922**, T-014 and T-015; atomic job completion, independent progress and lease/gap/quarantine recovery. |
| T-924 | Broker definitions, route metadata and topology test only; role separation, explicit deny-all empty routes, synthetic grants do not activate business access. |

Leaf lists match between task files and index. Every overlap with any existing
task is ordered by a dependency path; no new concurrently eligible task pair
shares application leaves. T-917/T-923 retain explicit reslicing obligations if
their four-hour bounds are exceeded.

Relay T-012 waits for recipient insertion and role topology. T-021 waits for
logical dispatch in addition to its original recovery producers. T-022 waits for
schema and topology; it does not claim business consumer completion from local
infrastructure readiness. All seven owner wiring tasks include the insertion,
dispatch and broker-role dependencies, and **all eight new tasks are transitively
required by every owner wiring task and final B-01 acceptance T-591**. The prior
QA finding about T-013 is therefore a definite graph prerequisite, not a
conditional prose warning. The broader 1,016-check helper confirms unrelated
product decisions have not become new global development blockers.

## Recoverable canonical history

Decompressed and verified all **20 snapshot entries** in the messaging-promotion
and T-036-start manifests: each stored byte count and SHA-256 matches. All **17
modified canonical files** have recoverable snapshots matching their exact
`HEAD^` bytes. The three additional intermediate snapshots represent only the
T-036 allocation, confirmed by object comparison and board/state diffs. No
canonical delete/add replacement or application change appears in the commit.

This verifies recoverability and content; file evidence alone does not independently
establish the wall-clock order in which the coordinator made each snapshot/edit.
No blocking finding remains for this bounded canonical promotion. Implementation
still requires allocated tasks, exact-commit QA and the contract's live failure
evidence; concrete feature policy gates remain intact.
