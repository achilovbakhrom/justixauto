# Authentication contract adoption

2026-09-15. Coordinator technical adoption under the approved architecture and
authorized development workflow; no production security acceptance.

Adopt the exact [T-058 auth supplement](../drafts/contracts/auth.md) at task commit
`fe3ffbfbe37d05261296a0ef5ba91cbec8edcead`, SHA-256
`935aa3bf7e2c060c38696d96576c9f7a8a7c39beef1eaa6e27b9fefabeee947e`.
Its original proposal status records provenance; this approval establishes the
technical contract. [Independent QA](../../dev/qa/T-058.md) is GREEN for proposal
readiness, with 121 contract checks and the documented transport limitation probe.

Explicitly adopt A1 through A5: anonymous/session CSRF header acquisition;
restricted deployment-bootstrap enrollment login result; session-bound step-up
challenge initiation; disjoint TOTP/recovery-code proof discrimination; and
typed one-time enrollment/provisioning and recovery-code results. Adopt the closed
safe `identity.auth.security-changed.v1` metadata schema and fixture catalog.
The proposed endpoint/state/error matrix is the bounded supplement to the
approved HTTP/domain baseline. Ordinary CRM gains no blanket MFA requirement.

T-059 may encode this contract and its private owner schema. It does not gain
ownership of the existing UOW or shared browser transport. Before dependent
runtime/browser wiring, the coordinator must concretely assign and review the
[owner migration compatibility](../../dev/owner-migration-readiness.md) and
[auth response transport](../../dev/auth-client-readiness.md) handoffs. Default
readiness remains closed; missing response headers cannot be silently ignored.
General active-user factor enrollment/replacement remains outside this contract.

External event activation still requires explicit approved recipients, source
admissions and authorized bootstrap/high-water evidence. Missing configuration
means no activation, never broadcast or an implicitly authorized empty plan.
Owner-local safe security history is not an external delivery guarantee.

T-002/OD-12 remains unapproved. No numerical session/credential/proof/rate policy,
production key, recovery/invitation proof or delivery adapter is accepted here.
Synthetic policies are explicitly fixture-only. All-factor-loss and general
pending-user activation remain gated. The four-app auth interaction supplement
and real security/runtime/acceptance tests remain their assigned tasks.

No dependency, service/data ownership, secret-publication exception or operational
guarantee changes beyond the stated technical auth supplement. This approval does
not complete B-02 or certify a running authentication service.

The [reviewed transport refinement](auth-transport-compatibility.md) explicitly
adopts P4 header/cache/media grammar and P6 synchronous semantic binding, without
rewriting the exact T-058 wire document. Implementation and real cookie/owner
race tests remain required; no release security settings are accepted.
