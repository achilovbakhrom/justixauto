# ADR-03/05 projection storage handoff approval

2026-09-15. Under the authorized development workflow, the coordinator approves
exact proposal `90dfe726ab53c4fdd54c3cac09e706797a32ea6d` following [independent proposal QA GREEN](../../dev/qa/projection-storage-handoff.md)
and coordinator transaction/concurrency review. The [canonical contract](../drafts/contracts/projection-storage-handoff.md)
preserves the reviewed bytes, SHA-256 `c15418fe9d63215b8301cc0349c7a4bdbb17ba611e25d58fc97b7345d3ff191a`; its proposed header remains
as historical evidence and this decision supplies approval.

Adopt reusable owner-local checkpoint/gap, protected quarantine and generation
mechanics behind owner-declared typed ports. T-926–T-928 own eight new leaves,
at most four task-effort hours each. Fifteen explicit dependency additions gate
affected adapters/runtime roots; the original 916-task release is retained.
The current graph has928 tasks/3078 task-effort hours; original width/schedule
metrics remain historical. B-01.AC2 closes only through its acceptance task.

Preserve installed SQL/route identities and mode history. Bootstrap, inbox,
effect, checkpoint and job completion obey their exact atomic boundaries;
unknown outcomes reconcile authoritative evidence. No inherited checkpoint from
a duplicate, plaintext quarantine fallback, automatic history repair or hidden
owner guard-writer guarantee. Snapshot/process admission, retention/keys and
real guard participation remain feature activation gates.

Approval covers implementation only. New migrations/adapters require live
failure/concurrency checks and independent exact-commit QA before integration.
No production migration, external data access, broker readiness or completed
runtime is claimed. Canonical SHA-256/recoverable preimages are in
`../backups/2026-09-15-projection-storage-promotion/`.
