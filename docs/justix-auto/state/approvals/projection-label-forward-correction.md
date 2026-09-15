# Retained generation label forward correction approval

2026-09-15. Coordinator adopts exact proposal commit
`f3fc029de3da3dbadb0cab980ef6bd33c86723b3` after
[independent proposal QA GREEN](../../dev/qa/projection-label-forward-correction.md).
The [canonical contract](../drafts/contracts/projection-label-forward-correction.md)
preserves proposal bytes, SHA-256
`704d2c65eedbe1e98f36bc0270d6ce366ad7cd38cccc2fe94b25e2a3ffa9f196`.
Its proposed header is historical; this decision supplies technical approval
under the existing storage and generation-label approvals.

T-928 replaces only the exact verified bootstrap generation CHECK in its existing
revision-6 forward transaction. Prior SQL/markers, labels, rows and grants remain
unchanged. T-016 and T-934 require the exact revision-6 artifact and corrected
object predicate before claiming complete retained-label compatibility. No new
task, edge, file ownership, effort or implementation status is added by adoption.

The pinned PostgreSQL proposal probe establishes catalog feasibility, retained
values/ACLs and transaction rollback, not full migration/runtime correctness.
Its server rejected disabling this CHECK; the recorded failed setup remains.
T-928 must independently prove the complete migration, lineage, roles, locks,
both consumer kinds and failure atomicity. T-014's bounded implementation and
historical QA remain intact with their explicit older-storage limitation.

This is implementation approval only: no applied migration, data repair,
admission, checkpoint reset, process replay or production authority. Recoverable
canonical preimages are in `../backups/2026-09-15-projection-label-promotion/`.
