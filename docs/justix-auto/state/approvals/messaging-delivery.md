# ADR-03/05 messaging delivery amendment approval

2026-09-15. Under the authorized development workflow, the coordinator approves
the storage and handoff model at reviewed proposal commit
`ba1723fdcbbfd9a0bbccdf6736e9346052d80fd7`, after independent
[QA GREEN](../../dev/qa/messaging-delivery.md) and coordinator scenario review.
The [canonical contract](../drafts/contracts/messaging-delivery.md) is an exact
byte copy of that reviewed proposal. Its original PROPOSED header is retained
as evidence; this decision supplies approval without rewriting its reviewed text.

Adopt immutable messages with complete per-recipient delivery plans and durable
owner intake with independent logical consumer jobs. Custody-mode AMQP ACK means
durable acceptance of the full admitted job set, not business-effect completion.
Direct-ACK mode stays compatible only for explicitly single-consumer contracts.
Preserve event identity/bytes, every admitted sequence position, authorized
checkpoint/bootstrap cutovers, role-specific broker grants and retained additive
legacy migration. No mixed writer/intake mode or automatic destructive rollback.

T-917–T-924 supply bounded implementation and independent exact-commit QA.
T-922 is a definite prerequisite for T-923: current T-013 owns its transaction
and skips effect callbacks on duplicates; nesting it does not provide atomic
job completion or legacy-inbox recovery. Updated relay, integration proof,
Compose and owner-wiring dependencies prevent premature activation.

The eight tasks contribute to existing B-01 scope; they do not regenerate or
replace the approved 916-task backlog. Original graph width/schedule metrics
remain explicitly historical. Canonical changes have SHA-256 manifests and
recoverable snapshots under `../backups/2026-09-15-messaging-promotion/`.

No runtime/migration test is claimed by this design approval. Concrete data
grants, personal snapshot access, process catch-up and immediate-revocation
disposition remain feature-specific approvals. No new dependency pin, business
owner, financial rule, production deployment or exactly-once broker promise.
