# Auth transport compatibility proposal assignment

2026-09-15. Status: independent exact-proposal QA assigned; no implementation approval.
Assigned worktree: `.worktrees/auth-transport-compatibility`.
Branch: `task/auth-transport-compatibility`.
Base: `476777139ffa6a470c7b3f6e79086c64f2dc7db5`.
Agent: `architect_auth_transport`.

Resolve [the observed transport gap](auth-client-readiness.md) against the
approved T-058 contract and actual T-032/T-640 APIs. T-640 passed independent
QA at `c69d52221287abecc4f1803ec7ed3532dce070f9`; its reviewed source is in
this base, with coordinator integration checks in progress. This proposal owns
only `state/drafts/architect/auth-transport-compatibility.md` and ancillary
`dev/results/auth-transport-compatibility.md` in an explicitly assigned worktree.
No application, schema, policy or canonical document edits.

Specify one shared strict raw JSON decoder, an explicit allowlisted validated
response-metadata interface, typed 429 handling, and the matching generator
profile extension. Cover anonymous 401 CSRF acquisition, restricted/full session
reads, login/MFA/session rotations, logout, missing/malformed headers and current
session epoch acceptance. Keep ephemeral CSRF out of generic receipts, logs,
query caches and persistent storage. Define conservative outcomes for a write
whose response/body/metadata cannot be validated; never infer rollback or retry.

Use the actual browser Fetch/Headers behavior: distinguish properties observable
from decoded headers or bytes from guarantees that require server validation.
Reject malformed UTF-8, duplicate decoded member names, lone surrogates and
trailing values; specify numeric handling without weakening the schema's string
money/revision constraints or claiming access to information already discarded.
Retain same-origin credentials, path/header restrictions, disabled redirects,
no automatic retries and existing safe typed errors. Internal mTLS metadata
must stay separate from browser cookie/session behavior.

Give concrete bounded task aliases, exact file leaves and serial successor
ownership for shared transport, decoder, generator and session integration.
Identify exact T-059/T-641/T-055 prerequisites or handoffs without inventing
irrelevant barriers. Coordinator alone registers concrete generation config
entries and changes root manifests/locks. Provide executed interface feasibility
probes and adversarial acceptance cases, including old callers and generated
Go/TS parity. No browser UI or production activation is implied by these probes.

T-002 security settings and OD-02 remain unaccepted. Token lifetime, rate policy,
recovery delivery or factor-loss decisions cannot be supplied by this technical
handoff. Preserve approved T-058 route and response semantics; any necessary
technical refinement must be explicit and reviewed before dependent code.
Independent exact-proposal QA and canonical promotion checks precede assignment.

## Independent review assignment

Proposal commit: `9ad5a64422b04dbbc8bee969a47acdd3e9278d81`.
QA checkout: `.worktrees/auth-transport-compatibility-qa` (detached).
Agent: `qa_auth_transport`. Own only `dev/qa/auth-transport-compatibility.md`
and its same-named evidence directory. Review five bounded aliases, exact
Go/TypeScript interfaces, pre-metadata semantic validation, raw-byte/header
limits, T-058 preservation and the proposed dependency/ownership delta.
Executed feasibility is not implementation QA or policy acceptance. Preserve
source and canonical files; coordinator promotion follows independent GREEN.
