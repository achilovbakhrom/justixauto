# Durable membership version gap

2026-09-15. T-920 implementation preflight found an uncovered storage boundary.
The approved messaging contract requires an immutable complete membership
version, exact installed claims at readiness, and authoritative cutover recovery.

Actual 000002 stores `membership_version` on dispatch_messages/enrollments.
Local messaging_admissions retains consumer/stream/contract/range evidence, but
neither that table nor 000004 bootstraps stores the complete approved catalog
version or its cutover request. T-746 supplies inert catalog interfaces as a
contract; it is not an implemented durable catalog registry.

Counterexample: on a source-proved empty stream, A/B admissions and zero
checkpoints can commit while no dispatch row exists. Restarted approved catalog
versions V1 and V2 with identical consumer contracts but distinct catalog/binding
identity observe identical SQL. A version-only transition adding no consumers
also has no durable receipt. Advisory locks serialize a transaction but cannot
recover its version after release. Existing custody rows cannot prove a current
version on a previously empty stream or a complete cutover request.

Do not encode a new protocol inside authority/purpose refs, fabricate a business
event or dummy custody message, infer zero or derive an undocumented version
hash. Preserve installed SQL, frozen enrollments and existing consumer meaning.
T-920 cannot complete its full acceptance until a separately reviewed bounded
storage/identity handoff resolves this case. Other independent tasks continue.

The architect must specify immutable complete catalog/claim identity and version
evidence, atomic current selection/cutover, exact request reconciliation after
unknown commit, zero-new-consumer upgrades, and readiness/composition ownership.
Finite source-authorized bootstrap/backlog transitions must remain atomic and
fenced. Missing bytes or policy remains a hold, never partial activation.
Any new migration, marker, predicate/ACL or task dependency needs explicit
canonical approval; no production migration or new business admission follows
from this finding. See the [assignment](membership-version-assignment.md).
