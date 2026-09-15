# Auth response transport readiness handoff

Date: 2026-09-15. Coordinator finding; no shared transport change approved here.

T-058 proposes a CSRF response header on anonymous 401, restricted/full session
reads and authentication rotations, plus typed 429 rate-limit responses. The
integrated `web/packages/api/src/client.ts` returns parsed data/error receipts
without response headers; its known error statuses omit 429. Its existing
transport therefore cannot supply the proposed auth response contract unchanged.

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

This finding does not change T-058's wire proposal or accept T-002 security
settings. Contract proposal review and independent generic generation may proceed;
browser auth readiness requires the concrete handoff and its implementation.

Observed inputs: T-032 shared client; T-058 proposal
`fe3ffbfbe37d05261296a0ef5ba91cbec8edcead`; T-055/T-640/T-641 task ownership.
