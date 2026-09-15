# Projection storage generation-label compatibility clarification

2026-09-15. Coordinator clarification under the existing ADR-03/05
[storage approval](projection-storage-handoff.md). This resolves a type ambiguity
without changing either reviewed proposal copy or an installed SQL artifact.

The storage handoff's canonical UUID rule applies to its new mechanical UUID
identities, including `bootstrap_id`, request/evidence IDs and the new
`projection_generations.generation_id`. It does not convert preexisting logical
names, contract IDs or admission generation labels into UUIDs.

T-917's installed `messaging_admissions.generation` and `dispatch_jobs.generation`
are nonempty text labels for both projection and process consumers. Their
immutable equality is already enforced; the integrated migration tests retain
the non-UUID label `generation-1`. Therefore T-926
`consumer_bootstraps.generation` remains nonempty text and must compare exactly
with its referenced local admission's generation and consumer kind. No UUID cast,
trimming, case conversion, renaming or historical label rewrite is permitted.
An empty or mismatched label fails. A process consumer does not acquire a
projection-generation record or projection UUID requirement.

T-928's newly introduced projection-generation primary key remains a canonical
nonzero UUID. T-016 must explicitly bind that record, its immutable consumer name
and the local admission label; a new label may use the UUID's canonical text.
That choice cannot reinterpret an old admission or reuse a consumer with changed
meaning. Bootstrap/checkpoint identity and progress remain immutable/contiguous
under the reviewed handoff; this clarification grants no reset or new admission.

T-926 must test non-UUID projection and process admission labels, exact matching,
empty/wrong labels, and conflicting second-bootstrap rejection. Its independent
implementation QA checks this compatibility alongside the full storage contract.
No new task, dependency, privilege, business policy or runtime guarantee is added.

Observed evidence: `pkg/eventstore/migrations/000002_messaging_delivery.up.sql`
lines 75–83 and its job guard; `pkg/eventstore/messaging_migration_test.go`
generation fixtures. Installed artifact SHA-256 remains
`1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2`.
The existing task preimage is retained under
`../backups/2026-09-15-generation-label-clarification/`.
