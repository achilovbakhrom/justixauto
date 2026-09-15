# Projection and quarantine storage handoff

Date: 2026-09-15. Coordinator finding; bounded architecture review pending.
This records an implementation ownership gap, not a new business policy.

## Observed gap

T-014 owns `pkg/projection/sequence.go`, `sequence_test.go` and `checkpoint.go`.
T-015 owns the four named quarantine/redrive Go leaves. T-016 owns the two
rebuild Go leaves. None owns a migration. The integrated T-008 schema has
events, command receipts, legacy outbox/inbox and operations/steps. T-917 adds
messaging admissions, custody, enrollment and job storage, including a nullable
quarantine reference; it supplies no durable checkpoint, quarantine evidence or
projection generation/pointer repository. Identity/Inventory scaffolds add only
their compatibility markers. T-925 corrects routes and adds its immutable
compatibility marker; it does not fill this gap.

The approved messaging contract §9 explicitly requires separate assignment for
additional migration leaves. Its §§3/6 require atomic bootstrap/checkpoint
installation, durable incomplete gaps, independent consumer checkpoints,
quarantine before acknowledgment and evidence-preserving redrive. T-922's
synthetic checkpoint/effect/job tables prove transaction composition only.

## Bounded architecture question

Determine the smallest storage and typed-port handoff needed by T-014, T-015,
T-016 and T-920 under the existing accepted guarantees. Compare owner-provided
storage ports with shared owner-local mechanical storage. Identify precisely
which behavior can proceed under existing contracts and which requires an
approved forward migration or new dependency. Do not implement hidden DDL in
Go adapters, edit installed migrations or silently assign broad owner paths.

The proposal must cover keys/ownership, immutable evidence versus mutable work,
runtime grants, transaction and lock boundaries, direct/custody mode interaction,
bootstrap authority and gap reconciliation, malformed raw bytes without event
IDs, redrive audit, generation switch/fencing and unknown commit recovery.
It must distinguish generic mechanics from feature-specific authorization,
personal-data retention and process catch-up. Keep unknown policy disabled.

Return exact proposed leaves and dependency edges, bounded implementation tasks,
compatibility/installation checks and failure tests. Preserve all earlier
migration identities and the established full event routing format. Any proposed
additional migration must be independently reviewed before dependent adapters
are assigned. This document does not approve a schema or mark a task blocked.

## Next action

Allocate an isolated architect worktree and assign the bounded review when a
worker slot is free. T-919's already-approved writer work and app shells can
continue independently. No complete delivery-runtime claim follows from their
individual integration.
