# Projection storage handoff — independent proposal QA

**GREEN — proposal readiness only.** Reviewed commit
`90dfe726ab53c4fdd54c3cac09e706797a32ea6d` in
`.worktrees/projection-storage-handoff`, against base
`2c1ff1de53f4dc7b0588d63d6b34de1d0983f0b3`.
Date: 2026-09-15. Reviewer: `qa_projection_storage`.

No blocking contract defect found. This verdict permits coordinator review and
canonical promotion of the bounded handoff. It certifies no migration, adapter,
production cryptography, service activation or combined B-01 acceptance.
No application file, proposal, canonical document, commit or infrastructure was
changed by QA. No browser or production data was used.

## Exact evidence and executed checks

- [Reviewed proposal](../../state/drafts/architect/projection-storage-handoff.md),
  SHA-256 `c15418fe9d63215b8301cc0349c7a4bdbb17ba611e25d58fc97b7345d3ff191a`.
- [Architect result](../results/projection-storage-handoff.md), SHA-256
  `153864022366fc0ee6ce49cb26e13911f87094a73735d107da927969960fccd1`.
- [Independent structural checker](projection-storage-handoff/check.py) and
  [actual command output](projection-storage-handoff/check-output.txt).

Executed from the assigned worktree:

```text
git rev-parse HEAD
git status --short
git diff --name-status 2c1ff1de53f4dc7b0588d63d6b34de1d0983f0b3 90dfe726ab53c4fdd54c3cac09e706797a32ea6d
git diff --check 2c1ff1de53f4dc7b0588d63d6b34de1d0983f0b3 90dfe726ab53c4fdd54c3cac09e706797a32ea6d
python3 docs/justix-auto/dev/qa/projection-storage-handoff/check.py
```

All passed. The exact commit adds only the assigned proposal and result. The
checker verified the reviewed working bytes, five relative links across both
documents, eight absent/unowned application leaves, free T-926/927/928 IDs,
exactly fifteen additional existing-task edges and an acyclic 928-node graph.
These ownership/graph checks passed independently against both the assigned
base index and current main at `7fbfe5f6b33929117ab01a143110c6e8d9abed46`.
Existing dependencies remain, and T-919 remains independent of this amendment.

The checker verified retained T-008/000002 hashes against both trees:

```text
pkg/eventstore/schema.sql
86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53
pkg/eventstore/migrations/000002_messaging_delivery.up.sql
1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2
```

Read actual source with `git show` at T-922
`e15f5e7a677084ad84c6952d53c53eddfb6d0b10` and T-925
`4dbe2593bce71c045f785fc07aed9da5b4113a73`; their `consume.go` and
`000003_messaging_route_compatibility.up.sql` respectively match current main
byte-for-byte. The proposal worktree predates these integrations; its older
`consume.go` was not mistaken for the referenced T-922 API. Also inspected
T-009 transaction/append, T-918 source fences, T-014/015/016/920 and affected
runtime task ownership, architecture §5, approved messaging §§3–9, the main
coordinator readiness question, product/open-decision boundaries and both Gaze
audits. Developer summaries were used as navigation, not proof of correctness.

## Contract review

| Scenario / boundary | Review result |
| --- | --- |
| Smallest durable ownership handoff | Three independently reviewable migrations separate checkpoint/gap, protected evidence and generation failure surfaces. Their eight exact leaves include the shared fence helper below both adapters. Existing adapter leaves and per-task bounds remain; overruns require explicit reslicing. No seven-owner business schema or unassigned generic store is introduced. |
| Isolation and immutable evidence | Each private owner database installs its own mechanics. Keys include consumer and source stream; generation names remain immutable/distinct. No cross-owner FK/database access or company-null wildcard is granted. Bootstrap/action/manifest references are evidence shape, not business authority. |
| Runtime privileges and migration failure | Named mutable work columns only; immutable rows deny broad mutation. Direct, column, PUBLIC, grant-option, default and reachable NOINHERIT privileges must be audited. Scripts preserve prior artifacts, modes, ACLs and route format, require exact lineage markers and fail atomically on incompatible installation/contention. SQL cannot prove external backup/stopped-runtime assertions. |
| Caller transaction and missing-checkpoint duplicate | T-922 really returns uncommitted AppliedCandidate/DuplicateCandidate and bypasses the effect callback for duplicates. The handoff puts the additional sequence-dependent duplicate proof in T-923: absent, behind or inconsistent checkpoint evidence holds the job; an ahead integer alone is insufficient. It cannot manufacture an Advance or committed receipt. Final live lease failure rolls back all staged effect/checkpoint/job writes. |
| Bootstrap with concurrent higher backlog | Authenticated source proof fixes h; zero requires new-empty proof. Snapshot rows, checkpoint, local admission and complete authorized higher backlog activate atomically under the receiver fence. Recovery/network preparation happens outside SQL; the finite boundary is re-read under activation locks and invalidated completeness aborts/reprepares. Inactive staging cannot authorize partial membership. No bounded catch-up time is promised under continual change. |
| Gap then rollback/crash | A detected gap rolls back effect/inbox/checkpoint/job before a separate transaction records gap evidence. That second transaction rechecks current progress so a concurrent repair produces resolution rather than a stale hold. Direct input remains unacknowledged; custody retains its incomplete job. Recovery uses authorized original positions through ordinary intake/dedup; no synthesized event or future-state adoption. |
| Receiver/catalog/generation lock order | Shared catalog locks cover ordinary receiver mutation; exclusive membership/switch locks prevent newly admitted stream phantoms. Complete receiver and generation/guard sets are sorted, acquired once, duplicate modes merge to exclusive and upgrades are forbidden. Source append prerequisites precede this set; head/job/inbox/checkpoint/effect follow it. Missing earlier prerequisites abort and reprepare outside the transaction. Offline migration contention retains its separate stopped-runtime and rollback requirement. |
| Package and capability boundaries | The helper is in eventstore, already below inbox/projection. Inbox does not import projection: bootstrap uses an injected typed installer, and checkpoint effects use the owner's UnitOfWork. Projection rebuild may compose inbox membership; T-920 does not depend on T-016. PreparedFences must bind actual transaction and required key/mode before adapter effects, rather than exposing generic SQL to application callbacks. |
| Malformed arbitrary bytes | Invalid JSON/claimed event IDs are not integration identity. Stable capture request plus exact digest/context resolves unknown commits; conflicting request reuse fails. Only injected approved sealing can create protected SQL custody; unavailable policy/key means no ACK. Synthetic fixture sealing has no production fallback or crypto/retention approval. No plaintext body/error logging or silent export/purge is authorized. |
| Quarantine race and redrive | Fresh evidence/job-hold transaction follows failed effect rollback; concurrent completed jobs are not reopened. Redrive preserves original evidence, authorization, other holds and ordinary dedup/checkpoint paths. A later result must cite authoritative receipts; crash after application before journaling reconciles those receipts. Neither audit presence nor missing commit reply means successful effect or rollback. |
| Historical generation switch | Exact authorized per-stream historical vector is distinct from live checkpoint zero. Same-vector invariant comparison and full universe validation occur under both generation fences; the new vector must dominate active progress. Failed comparison/stale epoch leaves the old pointer. Queued higher work and old obligations remain durable. Readers pin a generation/epoch and pagination must preserve or reject incompatible reuse. |
| Synchronous guards | Synthetic fence participation is testable within the generic task. Every real owner writer must take its matching shared fence before guard access and check epoch/hold; absent participation keeps that real guard rebuild disabled pending an assigned owner handoff. Async projection state does not become command authority. |

## Remaining gates

Proposal §8 contains **planned** SQL and adapter failure tests, not executed test
results. Later exact-commit QA must demonstrate real lock contention/rollback,
bootstrap/backlog atomicity, generation writer participation, grant isolation,
duplicate/conflict handling, quarantine/redrive races and actual lost commit
replies. String/graph checks in this review do not establish those guarantees.

Canonical promotion must carry these semantics into affected tasks, retain all
existing edges and record its own snapshots/approval. T-022 and owner runtime
wiring must check applicable exact migration markers; current Identity/Inventory
legacy-only readiness cannot silently activate these adapters. Actual source
admission/snapshot purpose, production security/retention, process historical
catch-up, revocation disposition and concrete command-guard participation remain
scoped activation gates. No broader architecture or business-policy reapproval
is required by this technical proposal review.
