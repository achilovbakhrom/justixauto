# Messaging delivery architecture result

Date: 2026-09-15. Branch: `task/messaging-delivery-contract`; base `6524338`.
Status: proposal ready for coordinator review and independent exact-commit QA.

Draft: `docs/justix-auto/state/drafts/architect/messaging-delivery.md`.

Diagnosed the single-target outbox and competing owner-queue consumer gaps from
integrated T-006/T-008/T-009/T-746 evidence. Recommends immutable message plus
per-recipient delivery records and owner durable intake/per-consumer work.
The proposed ADR-03/05 amendment explicitly distinguishes broker custody ACK
from each consumer's atomic inbox/effect/checkpoint completion, retaining the
T-013 direct-ACK primitive for approved single-consumer compositions.

Includes typed boundaries, admission/checkpoint and removal rules, lease fencing,
least-privilege broker roles, additive legacy migration/evidence, bounded exact
file assignments and a ten-part crash/authorization test inventory. Concrete
business data grants, process bootstrap and immediate revocation disposition
are not invented or approved by this mechanism.

Read AGENTS, current state/readiness, architecture, product/open decisions,
mock map, both pinned Gaze audits and assigned integrated task contracts/results.
Consulted RabbitMQ's official 4.3 confirms and access-control documentation for
the proposed broker contract. No reference secrets or live infrastructure read.

Validation: documentation-only diff/ownership and relative-link checks; no
runtime/migration/browser tests executed or claimed. Only the two assigned
Markdown leaves changed; canonical docs, application code, dependencies and
broker topology are untouched. Exact task SHA is returned after commit creation.
