# Messaging composition gaps — coordinator tracking

2026-09-15. These gaps do not invalidate the verified single-target outbox or
single-consumer inbox primitives. They block dependent multi-recipient and
multi-subscriber compositions until a bounded architecture amendment and its
implementation pass independent QA.

1. Architecture §5 requires every authorized subscriber of an integration stream
   to receive all sequence positions. The current T-008 outbox has one row per
   event ID, one routing key and one sent state; T-006 routing keys select one
   destination owner. Alternating targets for consecutive events would create
   false gaps, while inserting the same event twice violates its primary key.
   New per-recipient event identities or weakening contiguous checkpoints are
   not approved solutions.
2. T-006 currently provisions one integration queue per owner. Independently
   required projectors and process subscribers cannot each consume that queue
   competitively and assume all received the event. T-746 requires distinct
   consumer identities and justified effects but explicitly does not approve
   new queue-sharing or topology semantics.

The coordinator assigned `/root/architect_messaging_delivery` a bounded proposal
in `.worktrees/messaging-delivery-contract`, branch
`task/messaging-delivery-contract`, base `6524338`. Its only proposal leaf is
`../state/drafts/architect/messaging-delivery.md`, with ancillary result
`results/messaging-delivery.md`. It requires coordinator review and independent
exact-commit QA before approval; implementation scope is not yet selected.

The proposal covers durable authorized
recipient delivery and owner-local subscriber dispatch, preserving immutable
event bytes/identity, sequence guarantees, least-privilege broker access and
confirmed delivery/ACK semantics. No runtime policy is selected by this note.
Approved single-consumer fixtures may proceed; no affected owner subscription
is ready for activation on the basis of current primitive tests alone.
