# Proposed retained generation label forward correction

2026-09-15. Technical scope correction only; independent review and coordinator
adoption are pending. See `../../../dev/projection-label-readiness.md`.

## Observed mismatch and retained meaning

T-917's local admission and dispatch job generation columns accept exact
nonempty text. T-926's `consumer_bootstraps.generation` instead checks
`length(btrim(generation))>0`, rejecting text consisting only of U+0020 spaces.
PostgreSQL's default btrim behavior is narrower than general Unicode whitespace;
do not conflate tab-only labels with this failure. The earlier approved label
clarification requires preservation of these opaque labels and exact equality
to the admission. The correction changes neither that meaning nor consumer
authority. Empty labels remain invalid.

## Bounded T-928 scope amendment

T-928 already owns the not-yet-created
`pkg/eventstore/migrations/000006_projection_generation.up.sql` and
`pkg/eventstore/projection_generation_migration_test.go`. Add this correction to
that forward transaction and its negative/retained-data tests. Do not edit the
original schema or migrations 000002–000005, add parallel ownership, or create
a separate retroactive marker. The new revision-6 generation compatibility
marker and exact artifact hash identify the correction together with T-928's
existing generation storage. Keep base messaging schema/mode revision 2 intact.

Within the same explicit bounded installation transaction and existing ordered
installation locks, verify the prior revision-4 marker, its exact artifact hash,
owner/runtime identity and the target table's actual expected CHECK constraint.
Require a single validated CHECK on precisely the generation column with the
installed `length(btrim(generation))>0` expression; reject missing, renamed,
duplicated, disabled/unvalidated or altered target constraints and unexpected
object state. Use PostgreSQL catalog identity/expression inspection tested on
the pinned server, not a broad matching DROP or a caller-supplied object name.

Replace only that CHECK with `length(generation)>0`, preserving its stable
constraint name and validating the replacement in the transaction. Retain the
column NOT NULL, bootstrap/checkpoint keys and foreign keys, admission equality
guard, immutable generation/consumer contracts and all ACLs/default grants.
No row update, label normalization, checkpoint reset or job/admission rewrite.
Existing valid rows satisfy the weaker CHECK, but verify actual data and reject
unexpected tampering; do not turn a failed validation into automatic repair.
The correction and revision-6 marker commit or roll back with the full T-928
installation. Retain the existing bounded lock timeout and no automatic retry.

Original 000004 artifact SHA-256 remains
`41536429fb95d4b60a11866843a2ccbd6a4cf723cd919884933b6bc1d8202c4c`.
The new template's normal exact-artifact marker is sufficient provenance for
this correction; prior markers and installing evidence remain unchanged.

## Required evidence and downstream limits

Use owned pinned PostgreSQL fixtures with real migration/runtime roles. Before
the forward install, demonstrate that a local admission accepts a space-only
generation while bootstrap rejects it atomically. After install, demonstrate
exact matching bootstrap/checkpoint operation for projection and process labels:
one/multiple spaces, tab-only, padded nonblank and ordinary non-UUID values.
Empty and mismatched labels still reject. Process consumers acquire no new
projection-generation UUID requirement.

Capture old SQL/marker hashes and retained rows, keys, function definitions and
ACLs. After the install, verify those preimages except the one declared CHECK;
verify its name, expression and validation state and the new marker. Include
wrong owner/runtime/prior artifact, missing/altered/renamed CHECK, lock timeout
and a forced later failure in the same installation transaction. Each failure
must leave the old constraint/data/markers unchanged. Reapplication follows
T-928's existing explicit installation contract, without silent partial repair.

T-014's current adapter does not need a source rewrite: its constructor keeps
exact nonempty labels and its result records the older storage rejection.
Its original QA evidence remains historically correct. T-016 already depends on
T-928, and T-934 already requires T-928 for custody readiness. Their exact
revision-6 artifact/object checks must include this corrected CHECK before
claiming complete retained-label compatibility. Readiness for an earlier marker
must not assert the correction. Add this requirement to their canonical handoffs
on adoption; no new dependency edge or file ownership is necessary.

This proposal authorizes no live migration, production admission, database
attestation, business effect, privacy policy or process replay. It closes one
storage predicate discrepancy under the existing generation-label decision.
