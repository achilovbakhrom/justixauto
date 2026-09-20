# Gaze Event Sourcing/CQRS core refresh

Date: 2026-09-20. Reference checkout:
`/Users/bakhromachilov/golang/gaze-executor`, clean HEAD
`6a277d916b417754fe5d5e5b11cebe77ea3d8c42`.

This refresh is deliberately narrower than the historical full audit in
`gaze-reference.md`. No reference files were changed, and no `.env`, key,
credential, trading gateway or production integration was read or copied.

## Adopted patterns

- A pure aggregate lifecycle: one transition function, replayed state, current
  revision and command-local pending changes.
- Typed owner command/query handlers with constructor injection.
- Separate projection handlers and event-to-command subscriber adapters.
- Explicit composition roots that construct ports and adapters.

## Justix adaptations

The reference is not copied literally. Justix keeps its immutable validated
`pkg/events` envelopes, contiguous `pkg/eventstore` replay, expected-revision
transactional append, typed owner UOW, transactional outbox, durable inbox,
sequence-aware projections and quarantine mechanics.

The pure aggregate lifecycle belongs in `pkg/events`; persistence remains in
`pkg/eventstore`. Commands and queries remain owner-local under
`services/<owner>/app`; there is no global dispatcher or service locator.

## Explicitly rejected reference behavior

- mutable event setters and aggregate setters;
- replay that accepts revision gaps;
- snapshot stubs or snapshot activation;
- `*gorm.DB`, `Tx() any` or adapter imports in domain/application APIs;
- asynchronous post-commit publishing with ignored errors;
- broker publication from aggregate persistence;
- trading entities, wallets, gateways, credentials or amount utilities.

Current Gaze still does not supply the reliability guarantees already present
in Justix. It is a structural reference for aggregate/CQRS composition, not a
replacement event platform.
