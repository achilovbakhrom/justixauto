# Architecture proposal review — 2026-09-13

Scope: documentation only. No application implementation, runtime integration
test, new browser QA or legal/security certification. User approval remains open.

## Inputs and provenance

| Current source | SHA-256 read for this proposal |
|---|---|
| `business-logic.md` | `20041e7f71f2c220b6e35400dd6a046b38d6dbb7be8695456724c93c2e69f911` |
| `mock-map.md` | `6c2bb091a3ff8f6eeca1aa62eccbc05886cccd5244453c643d4a0ebf719b82ba` |
| `reference/gaze-reference.md` | `365987ba30ce0864fabb9117a1c3e6a1cfe638d57d6c455d3e88907e8a28a729` |
| `mocks/manifest.json` | `bd57ee305e6e3cb074db51da9919484b71d7de7c78094f51fd69a3e0983a8c43` |

Original `open-decisions.md` and prior state documents have hashes/sizes and
recoverable copies under `backups/2026-09-13-architecture/`. The OD update records
that Gaze has already been inspected and identifies two narrow future questions;
it does not resolve unknown financial, insurance or suspension policy.

## Review work

- Four slice analyses: identity; inventory/commerce; retail/finance/insurance/
  documents; frontend/infrastructure. Kept under `../dev/analysis/` as evidence,
  not additional canonical APIs.
- Coordinator reconciliation: common route prefix, revision/error/Money format,
  domain versus integration sequence, shared VIN ownership, outgoing partnership
  cancellation, branch coordination, immutable commercial and financial snapshots.
- Independent intake review identified a scan/upload time-of-check/time-of-use
  risk: final bytes must be pinned to an immutable storage generation. See
  `architecture-inputs/analysis-review.md` and the synthesized storage contract.
- An initial coordinator objection to natural-person Customer scope was withdrawn
  after checking business-logic §4. No corporate-customer scope was added.
- Independent focused synthesis review: no new P1 in the main architecture and
  reservation/branch/submission/storage protocols; scanned-byte finding addressed
  at contract level. See `architecture-inputs/synthesis-review.md` for its scope;
  the final HTTP appendix requires coordinator review separately.
- Coordinator reviewed the final HTTP appendix and added two clarifications:
  known-VIN receipt is one atomic command alongside quantity-before-VIN, and
  authentication handshakes never store secret responses in the generic business
  idempotency ledger. Sensitive-only MFA replaces blanket step-up for every edit.

## Completion boundary

Documentation checks on this turn: 24 candidate IDs unique; dependency graph
acyclic; declared estimates 2–4h; pre-promotion canonical fingerprints unchanged.
Existing reference bundle rechecked: `npm run mocks:verify` passed 57 hashes,
syntax/local references and four entries; `npm run mocks:test` passed all 106
tests. These checks certify neither a new Go/React application nor visual parity.

`../architecture.md` is the consolidated **proposal** and owns conventions;
raw slice recommendations never override it. `../dev/task-preview.md` has 24
candidate units with focused acceptance/owner paths. It is not a completed
release backlog, an exhaustive task set or an approved development queue.

Current checkpoint: architecture approval. Next: PO backlog → user scope approval
→ PM formal task files/dependency validation. Independent project Git is required
before application code, not before those documentation phases.
