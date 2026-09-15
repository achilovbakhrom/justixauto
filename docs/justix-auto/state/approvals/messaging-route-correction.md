# ADR-03/05 route compatibility correction approval

2026-09-15. Under the authorized development workflow, the coordinator approves
the bounded forward correction at exact proposal `631388169f3f24c97b9237bcf933de79315b3651` after
[independent proposal QA GREEN](../../dev/qa/messaging-route-correction.md)
and coordinator source/retention review. The [canonical contract](../drafts/contracts/messaging-route-correction.md)
is an exact copy, SHA-256 `7130ed6d2cf47c609223be2ea6d9a7d6500e515a028c918896224b1b90d7766a`. Its PROPOSED header remains as
review evidence; this decision supplies approval without rewriting reviewed text.

Keep the established `source.target.fullOwnerQualifiedEventType` format and
correct the SQL allowlist extraction in a new explicit forward migration.
Preserve installed 000002, mode transitions, routes, bytes, IDs, holds and all
historical evidence. An immutable correction marker supplements base version 2;
it does not grant business access or by itself establish custody readiness.
Incompatible retained records fail without automatic normalization or repair.

T-925 owns two new leaves and at most four task-effort hours. Its eleven direct
dependent tasks wait for reviewed implementation; the original 916-task scope
is retained with nine bounded messaging follow-ups, not regenerated. Total925
tasks/3066 effort-hours and coverage mappings are updated; original graph-width
and schedule metrics remain explicitly historical. The earlier eight amendment
tasks' B-01.AC2 mappings are also made explicit in backlog-coverage.md.

No migration, PostgreSQL cutover, broker runtime, production change or concrete
data admission is approved as completed. Genuine producer/cutover and custody
regressions plus independent exact-commit QA remain required. Canonical preimages
have SHA-256/recoverable snapshots under `../backups/2026-09-15-route-promotion/`.
