# Coordinator review findings for synthesis

Resolve these explicitly; slice documents are alternatives, not four equal APIs.

- Public prefixes conflict (`/api/inventory/v1`, `/v1/retail`, etc). Adopt
  `/api/v1/{service}/...` everywhere. Shared transport/error/concurrency contract
  follows `contract-conventions.md`, not inconsistent slice 409/expectedVersion
  body choices. Header If-Match is primary aggregate revision; body versions are
  only for additional explicitly versioned references in a multi-object action.
- Serialize revisions/int64 IDs-as-numbers and money minor units as decimal
  strings; UUIDs remain strings. Finance decimal-major Money and inventory
  minor-unit Money conflict: choose minor-unit string + currency exponent;
  non-integer rate parameters use separately validated decimal strings. Explain
  calculators still need user-approved policy, no silent demo activation.
- Internal event revisions and filtered integration events need different
  contiguous checkpoints. Use per-owner aggregate integrationSequence incremented
  only for published integration events, distinct from domain aggregateVersion.
  No consumer should wait forever for a deliberately private event.
- Customer is a natural person in current business-logic §4. The coordinator's
  earlier concern was incorrect; keep the slice's scope. Do not add corporate
  clients or a new client-kind selector as a purported correction.
- Do not impose the financial slice's proposed one nonterminal finance app per
  deal as confirmed policy. Command idempotency is confirmed; alternate-provider/
  repeat-application business rules need a scoped question before any uniqueness
  restriction other than confirmed insurance company+sale constraint.
- Warehouse model must retain name/country/region/city/address/capacity/branch;
  do not copy an abbreviated slice schema that drops mock-supported fields.
- Partnership needs requested-side decline and requester-side withdraw/cancel.
  Explicit user asked outgoing cancel. Include command/state and mock reference;
  ending active relationship is a different operation, never a substitute.
- Finance document cancel in review conflicts with broad "unfinished" wording.
  Preserve current requested/changes cancel and label review-cancel user decision;
  do not silently authorize it. Approval does not auto-create a contract/policy.
- New branch + existing warehouse attachment need explicit operation lifecycle.
  Do not promise a cross-service SQL transaction. Branch suspension/deletion
  races need attachment intents/guard protocol or scope them out; no phantom
  linked branch or half-created branch reported as full success.
- Application submission-intent eligibility: specify acquire/consume/release,
  retry/status/reconciliation, immutable snapshot and conflict with delivery.
  Do not add a permanent sale lock with no recovery or limit deadlock to timeout.
- Task candidates are preview only. Formal PM files/board wait architecture AND
  backlog checkpoints. Deliver a clearly titled task-preview document now if
  useful, but no implementation-ready claims or skipped approvals.
