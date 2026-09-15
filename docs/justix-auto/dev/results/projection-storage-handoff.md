# Projection storage handoff — architect result

Date: 2026-09-15. Proposal complete; coordinator approval and independent
exact-commit proposal QA pending. No implementation, migration or runtime
readiness is claimed.

- Worktree: `.worktrees/projection-storage-handoff`.
- Branch: `task/projection-storage-handoff`.
- Base: `2c1ff1de53f4dc7b0588d63d6b34de1d0983f0b3`.
- Deliverable: [projection-storage-handoff.md](../../state/drafts/architect/projection-storage-handoff.md).
- Only that new proposal and this assigned result changed. The commit containing
  this result is the review target; exact SHA is returned to the coordinator.

## Recommendation and bounded ownership

Use owner-local shared mechanical SQL behind owner-declared typed ports. The
proposal adds three separately reviewed forward migration tasks, provisionally
T-926/927/928, and eight exact new leaves; the shared fence helper stays in the
T-926 assignment below both inbox and projection adapters. T-014/015/016/920 keep
their existing Go leaves. Fifteen explicit existing-task dependency additions
connect durable adapters, bootstrap, intake, rebuild and runtime readiness.
No task graph or canonical file was changed. IDs remain coordinator reservations.

The proposal covers independent checkpoints, atomically authorized bootstrap,
separate durable gap recording after effect rollback, protected malformed-byte
evidence, immutable redrive actions, historical generation manifests, pointer
CAS and shared/exclusive generation/guard fences. Retained inbox duplicates
cannot complete sequence-dependent jobs with absent/inconsistent checkpoint
evidence. Generic T-922 candidate semantics and previous migrations stay intact.

Production quarantine has no plaintext fallback: an approved owner security port
must seal exact bytes under an accepted policy/key configuration. Fixture-only
sealing permits synthetic adapter tests without claiming production crypto.
Feature admission/snapshot authority, OD-10 retention/limits, process catch-up,
immediate-revocation disposition and concrete owner guard-writer participation
remain scoped activation gates. No finance or privacy policy is invented.

## Inputs inspected and verification

Read assigned AGENTS/architect role, dev-state, coordinator readiness question,
product/open-decision references, both Gaze audits, architecture §5, approved
messaging §§3/6/9, task board/index and T-014/015/016/920 leaves. Inspected current
T-008/T-917 schema and transaction/inbox source, T-922 implementation/result at
`e15f5e7a677084ad84c6952d53c53eddfb6d0b10`, and T-925 migration/result at
`4dbe2593bce71c045f785fc07aed9da5b4113a73` in their existing worktrees. No reference
environment files, keys, infrastructure or real business data were accessed.

Executed read-only Python checks against the assigned base index:

```text
PASS: proposed DAG 928 tasks
PASS: 15 additional existing-task dependency edges
PASS: eight proposed application leaves are absent and not already task-owned
PASS: four proposal relative links resolve
PASS: git diff --check
```

The check adds proposed T-926→T-925/T-008/T-009, T-927→T-926/T-013 and
T-928→T-927 in memory, adds the fifteen edges listed in proposal §7, then performs
a DFS cycle/missing-dependency check over all 928 nodes. It compares the eight
proposed leaves with all existing `files_owned` entries and the worktree. It
does not rewrite task-index.json or assume original effort metrics were updated.

Preserved prior artifact hashes:

```text
pkg/eventstore/schema.sql
86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53
pkg/eventstore/migrations/000002_messaging_delivery.up.sql
1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2
```

All PostgreSQL failure cases in proposal §8 are future implementation acceptance,
not tests run by this documentation slice. Independent proposal QA should review
the exact final commit, source/ownership/DAG assertions, package dependency
direction, lock/candidate boundaries and preserved policy gates before promotion.
