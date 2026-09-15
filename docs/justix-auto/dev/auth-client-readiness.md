# Auth response transport readiness handoff

Date: 2026-09-15. Coordinator finding; no shared transport change approved here.

The approved T-058 contract requires a CSRF response header on anonymous 401, restricted/full session
reads and authentication rotations, plus typed 429 rate-limit responses. The
integrated `web/packages/api/src/client.ts` returns parsed data/error receipts
without response headers; its known error statuses omit 429. Its existing
transport therefore cannot supply the approved auth response contract unchanged.

Before assigning dependent browser authentication wiring, assign and independently
review the exact shared-client and test leaves needed to expose only the approved
response metadata and typed rate-limit outcome. T-641 owns generated auth leaves,
and T-055 owns session/context state; neither currently owns the shared transport.
The generator must not quietly patch it or duplicate a weaker fetch transport.
T-064 is the authorization contract, not the session client.

The handoff must preserve same-origin credentials, URL/header restrictions,
strict response validation, no redirects, no automatic mutation retry and safe
unknown outcomes. CSRF values belong only in the live session's memory; exclude
them from persistent storage, logs, generic receipts and query caches. Accept
rotation metadata only from the correct validated response and current session
epoch. Define rejection behavior for missing/malformed required headers, stale
responses and rate-limit metadata, including anonymous 401 acquisition. Do not
expose bearer cookies or arbitrary response headers through generic application
state. Exact implementation ownership and dependency edges remain to be assigned.

This finding does not change T-058's adopted wire contract or accept T-002 security
settings. Independent generic generation may proceed;
browser auth readiness requires the concrete handoff and its implementation.

Observed inputs: T-032 shared client; T-058 proposal
`fe3ffbfbe37d05261296a0ef5ba91cbec8edcead`; T-055/T-640/T-641 task ownership.

## Raw response decoding and generator handoff

T-640 independent QA at `9ed232ebb5ffa110454e16f8c92d3cd8bec8162b`
also confirms a separate transport limitation: `response.json()` discards duplicate
member and raw numeric-lexeme information and does not provide fatal UTF-8
validation. T-058 requires malformed UTF-8, duplicate members and trailing JSON
rejection before use. A generated validator over parsed values cannot restore
that lost information. Assign the raw shared decoder and its negative tests;
define number handling without relaxing string-only revisions or money. This is
separate from T-640's reproduced malformed map-key bug, which is fixable in the
existing parsed-value validator and remains in its current fix cycle.

The bounded architecture follow-up must inspect the actual T-032 and reviewed
T-640 APIs, specify strict decoding plus allowlisted response metadata and 429,
and assign serial successor ownership and explicit dependency edges. Preserve
existing generic consumers and typed errors; do not expose arbitrary Headers,
store CSRF in generic result/cache objects, or create a second fetch path.
Generator profile changes follow that exact transport interface. Session epoch
acceptance belongs to the explicitly assigned session layer, including anonymous
acquisition, authenticated rotation, logout and stale concurrent responses.

Technical choices for header syntax/bounds and invalid or unknown outcomes must
be reviewable and must not invent the unapproved T-002 lifetime/rate policies.
T-059 may preserve the approved schema declarations while the transport work
proceeds; T-641 and dependent browser authentication must wait for the complete
owned handoff. Approval: `../state/approvals/auth.md`.

## Reviewed technical adoption

Revised proposal `914fcd78cc0789e41e9091efdc99fb5afc6c5c50` passed independent r2 review. The
[technical approval](../state/approvals/auth-transport-compatibility.md) maps ten
bounded tasks with explicit generator serial ownership and producer/consumer
handoffs. Canonical promotion QA precedes assignment. Earlier BOUNCE evidence
and observed transport gaps remain; implementation is not complete.
