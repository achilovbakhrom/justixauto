# Coordinator synthesis constraints

Status: architectural proposals for user approval, not confirmed product policy.

1. Exactly seven domain owners: identity, inventory, commerce, retail,
   financing, insurance, documents. Edge has no business database. Do not create
   separate billing, notifications, audit or workflow services in the first cut.
2. One Go module with separate service binaries and private SQL ownership.
   Gaze hexagonal layers and event sourcing are preserved. A service's event
   append, uniqueness/capacity guards and outbox share one local transaction.
3. Event replay is canonical for domain decisions. Projection lag must never
   authorize overselling, oversubscribing a warehouse, or bypassing permissions.
   Guard tables are rebuildable but changes are synchronous, not subscriber-fed.
4. Reliability ADR: transactional outbox, confirm-aware persistent publication,
   inbox deduplication and version-aware projections. At-least-once, never claim
   exactly-once delivery. Quarantine malformed/gapped events with durable evidence.
   Retry and recovery tests are first-class tasks. No snapshot optimization yet.
5. Inventory grants all reservations. A caller-owned persistent process manager
   coordinates retail/B2B intent; operation identifiers, pending-result polling,
   exact-holder release and rejection of delayed cancelled acquisition are part
   of the contract. Timeouts are not successful cancellation or stock release.
6. Credentials and sessions are mutable restricted auth tables; never event
   payloads. Shared documents service owns bytes/scan/version metadata, while
   the originating business aggregate alone decides review/acceptance/access.
   Audit is a projection, not a second mutable truth.
7. Confirmed business state machines remain intact. No fake production money
   formulas, legal permissions, actual bank disbursement, policy issuance or
   ownership transfer follows from demo status updates. Policy gaps gate only
   the affected commands. Safe foundation and confirmed exchange stay buildable.
8. Four React apps, shared components/tokens, domain API clients; use current
   minified HTML as immutable visual/interaction reference. No Figma, fifth app,
   public marketplace or obsolete dealer/distributor directories.
9. Package/image lock task must select supported patch versions and verify
   compatibility/security before application scaffolding. Gaze's observed pins
   are not silently advertised as production supported versions.
10. Architecture checkpoint precedes formal PO/PM pipeline. Analysts' small task
    candidates are a preview, not an approved backlog/ready task board. Preserve
    this distinction in state and final handoff; do not bypass user checkpoints.

## External verification used in this proposal

- [RabbitMQ acknowledgements and confirms](https://www.rabbitmq.com/docs/confirms):
  publisher and consumer acknowledgements cover different delivery boundaries.
- [RabbitMQ reliability](https://www.rabbitmq.com/docs/reliability): confirm
  recovery can redeliver; consumers must tolerate duplicates.
- [React build-from-scratch guidance](https://react.dev/learn/build-a-react-app-from-scratch):
  Vite is an available build tool; routing/data layers must still be selected.

Read on 2026-09-13. These references justify infrastructure choices, not product
rules or unsupported version pins.
