# Auth transport canonical promotion — independent QA

2026-09-15. **GREEN**, exact promotion commit
`56e904a1a2748033be1f4d3565b0fc8eca8bdbe8`, parent
`7dabbfd8c9e783652a48ecace39660eb66a7e009`, detached checkout
`.worktrees/auth-transport-promotion-qa`.

The canonical promotion faithfully records the independently reviewed proposal
`914fcd78cc0789e41e9091efdc99fb5afc6c5c50`, SHA-256
`e1643c8cf36ba5733f7bfa0d32e2285604b345fe72ed506e17248fbae9af3e0c`.
This closes the independent canonical-promotion review gate. Coordinator status
recording and each task's dependency/Git/exact-commit implementation QA gates
remain; no task was implemented or marked complete by this review.

## Verified against immutable Git objects

- All **13 recoverable preimages** decompress to the exact parent Git bytes;
  every stored SHA-256 and byte length agrees. Every existing modified canonical
  file has exactly one recorded preimage. The complete40-path change consists
  only of the declared documentation and snapshots; there are no application,
  dependency, source contract rewrite or hidden ownership changes.
- Canonical and architect contract bytes equal the exact r2-reviewed proposal.
  All **934 pre-existing task records** are identical to the parent except the
  four approved dependency additions: T-641→T-942, T-055→T-944/T-943 and
  T-060→T-943. Existing statuses, worktrees, reviewed/integration evidence,
  ownership, scope, coverage and other dependencies are preserved.
- The graph contains **944 unique tasks /3,138 total effort hours**, is acyclic,
  and adds ten unassigned `todo` tasks totaling36 estimated hours. Every alias,
  estimate≤4h, direct dependency and exact file list matches the reviewed P7.
  All944 task/board status, dependency, parent, kind and lane fields agree;
  board titles/branches/effort and task ownership lists also agree with the index.
- T-937→T-938→T-939→T-940→T-941→T-942 is the exact six-stage generator chain,
  serially succeeding T-640 on the same two leaves. The new tasks span13 unique
  implementation leaves. Independently enumerated **45 distinct
  (file, owner-A, owner-B) tuples** involving new tasks are ordered by dependency
  ancestry:42 generator tuples,2 client tuples and1 API-manifest tuple. This is
  a computed ownership count, not the coordinator's earlier printed25 label or
  a count of business tests.
- Coverage changes add contributions only: T-935/T-936→B-02.AC1;
  T-937–T-942→B-01.AC4; T-943→B-02.AC1–4; T-944→B-04.AC2. Existing AC text,
  contributors and dedicated closers are unchanged. Each new contributor is a
  transitive prerequisite of its dedicated T-591, T-592 or T-594 closer, and
  every new task document repeats the correct contribution and closer.
- **1,014 local Markdown file/directory links** in changed documents resolve
  within this exact Git tree. This checks link targets, not remote sites or
  section-anchor rendering. All **13 prior QA artifacts**—original BOUNCE plus
  four files and r2 GREEN plus seven files—match the parent and their respective
  independent review checkouts byte-for-byte. Original verdicts remain intact.

The independent [checker](auth-transport-promotion/check.py) passed **19,822
assertions**, including repeated dependency visits and document field checks;
that number is not19,822 independent scenarios. Recoverable preimage identities,
all45 ownership tuples and all13 prior QA hashes are retained in the exact
[JSON output](auth-transport-promotion/check-output.json). The first checker run
passed19,792 assertions; the final30 added assertions verify each new task
document's contribution, closer and parent. Neither run found a discrepancy.

## Scope and gate review

The approval, task-index metadata and exact task documents retain root ownership
of concrete generation config, generated/authValidation exports and dependency
locks. Root manifest changes must be serialized around T-944's active ownership
and included before dependent exact-commit QA. No worker receives an unowned
helper, root manifest or alternate generated transport.

T-059 has the adopted schema/header/semantic declaration handoff without an
invented frontend completion dependency. T-641 waits for terminal T-942 and root
registration, and cannot implement generator gaps inside generated leaves.
T-055 composes T-944/T-943 with generated clients and retains its session/cache
ownership. T-060 and T-592 explicitly retain owner transaction, raw header/cookie,
revocation-race and unknown-outcome verification; uncovered server/edge test
leaves require explicit assignment. These handoffs add no secret-storage,
authorization or callback-policy fallback.

The generator stages remain inert for public auth through T-941, rejecting
unsupported auth declarations before output. Only terminal T-942 activates the
complete, independently checked profile. It explicitly tightens the8MiB and128
container limits for **all newly regenerated Go outputs**, including non-auth
and internal methods. Intermediate legacy outputs are preserved. The approval
correctly distinguishes generated validation of an already allocated Body slice
from the exchange's responsibility to bound network reading. This canonical
audit does not substitute for terminal parser/parity/resource tests.

The exact reviewed synchronous outcomes, whole-tree structural-first selection,
fatal binding propagation, safe Promise-disposal limits and current-ticket
failure/finally rules remain in the byte-identical canonical contract. No runtime
models were rerun because that reviewed content and source were unchanged.
T-058 ownership and wire authority are retained; T-002/OD-12 security settings,
release field policy and recovery/delivery remain unaccepted. No browser cookie,
working service, production auth or release-acceptance claim was introduced.

Only this new QA report and its three evidence files were written. No canonical
or application file, Git history, prior QA artifact or external system changed.
Commands and limits are in [commands.txt](auth-transport-promotion/commands.txt).
