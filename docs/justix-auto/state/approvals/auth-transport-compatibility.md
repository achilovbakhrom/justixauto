# ADR-08 auth transport compatibility adoption

2026-09-15. Coordinator adopts P1-P8 of exact revised proposal `914fcd78cc0789e41e9091efdc99fb5afc6c5c50`
after [independent r2 GREEN](../../dev/qa/auth-transport-compatibility-r2.md).
The [canonical contract](../drafts/contracts/auth-transport-compatibility.md)
equals the architect copy, SHA-256 `e1643c8cf36ba5733f7bfa0d32e2285604b345fe72ed506e17248fbae9af3e0c`. Its proposed header is historical;
this approval supplies technical adoption under the authorized workflow.
Original proposal BOUNCE and all five artifacts remain unchanged.

Explicit new technical refinements: strict raw decoding; immutable parser limits
8MiB/128 levels; CSRF unpadded base64url-alphabet length1–4096; canonical bounded
Retry-After delay seconds; narrow no-store/media forms; required versioned ports;
synchronous primitive semantic outcomes and structural-first validation. These
are parsing/transport constraints, not token entropy/lifetimes/rate/proof policy.
T-002 remains unaccepted. Owner authorization and cookie revocation are separate.

Six generator stages are serial on the same two leaves. Public auth generation
rejects before output through T-941; only T-942 activates complete behavior and
deliberately tightens byte/depth acceptance of all newly regenerated Go outputs,
including legacy non-auth/internal contracts. Intermediate output is preserved.
Generated Body validation cannot bound bytes already read by an exchange; actual
bounded IO is an explicit adapter duty. Never weaken a constraint or fabricate
missing bindings to complete generation.

| Alias | Task | Estimated hours |
|---|---|---|
| AT-JSON | T-935 | 4 |
| AT-HTTP | T-936 | 4 |
| AT-GEN-PROFILE | T-937 | 2 |
| AT-GEN-SEM | T-938 | 4 |
| AT-GEN-TS | T-939 | 4 |
| AT-GEN-GO | T-940 | 4 |
| AT-GEN-RAW | T-941 | 3 |
| AT-GEN-VERIFY | T-942 | 3 |
| AT-AUTH | T-943 | 4 |
| AT-EPOCH | T-944 | 4 |

The graph adds ten tasks/36 hours and four existing edges: T-641→T-942,
T-055→T-944/T-943, T-060→T-943. Existing producer and owner/edge verification
handoffs are appended without extra leaves. Root alone controls concrete config,
generated/authValidation exports and locks; serialize API manifest edits after
its active T-944 owner and include them before dependent exact-commit QA.
No new dependency family or UI design is introduced. Parent acceptance remains
with T-591/T-592/T-594, not these mechanical fragments.

Canonical promotion QA must verify all prior task states/edges/ownership,
coverage/closure, exact copies and recoverable preimages before assignment.
Snapshots: `../backups/2026-09-15-auth-transport-promotion/`. Implementation, browser/owner cookie tests,
production configuration, recovery delivery and running services remain pending.

Independent [canonical promotion QA is GREEN](../../dev/qa/auth-transport-promotion.md)
at `56e904a1a2748033be1f4d3565b0fc8eca8bdbe8`. Coordinator accepts that
verification and clears the pre-assignment promotion gate. Task dependencies,
exact file ownership and independent implementation QA remain mandatory.

## Isolated generator fixture dependency correction — 2026-09-15

Coordinator corrects T-936's slice after its worker identified that the actual
generator test copies only `client.ts` into an isolated module. The adopted
decoder import needs `responseJSON.ts` copied alongside it. T-936 additionally
owns `tests/contracts/generation_reproducibility_test.go` solely for that fixture
copy, with explicit T-640 predecessor and existing T-937 successor. No generator
behavior, contract, dependency pin, policy, task count or effort changes.
The exact original proposal/canonical copy and promotion QA remain historical
evidence; this records the subsequent bounded implementation-scope correction.
Independent T-936 QA must verify that restriction and actual fixture regression.

## Closed header schema representation — 2026-09-16

T-937 makes P4/P6's exact wire values concrete in JSON-form OpenAPI. The accepted
inline Header Object contains only `required: true`, `schema` and an optional
string `description`. Its exact schema is one of:

- X-CSRF-Token: `{"type":"string","pattern":"^[A-Za-z0-9_-]{1,4096}$"}`.
- Retry-After: `{"type":"integer","minimum":0,"maximum":2147483647}`.
- Cache-Control: `{"type":"string","const":"no-store"}`.

Retry-After's integer schema describes its value; the trusted parsed metadata
also fixes P4's canonical decimal wire grammar `^(0|[1-9][0-9]{0,9})$`.
Alternate encodings, dates, refs, optional security headers, unknown schema keys
and extra Header Object fields remain rejected. Unsafe request CSRF uses the
same exact string schema with name/in/required as its Parameter Object; the
transport injects it, so it must not become an ordinary generated DTO field.
This representation introduces no new entropy, lifetime or rate policy.
The public auth generator remains disabled through T-941; only T-942 activates
the complete reviewed runtime. Earlier proposal bytes remain historical.

## Typed error binding representation — 2026-09-16

T-937 permits different named error schemas per declared operation/status. For
T-938's P6 semantic hook, the coordinator approves `AuthErrorResponse` as the
TypeScript union of those declared DTO types. Go uses a generated typed wrapper
with exactly one per-schema pointer field, selected by trusted compiled
operation/status metadata; a single-schema profile may instead use a type alias
in either language. No arbitrary payload or untyped fallback is introduced.

The generated boundary rejects empty, multiple or wrong alternatives, nil and
typed-nil values, and operation/status mismatches before the hook. Wrapper/type
and field names participate in existing generated symbol collision checks.
Whole-tree structural selection still precedes semantic validation; the hook
receives the chosen typed DTO under the existing synchronous outcome/readiness
contract. This resolves a generated binding representation only. Wire schemas,
endpoint matrices, policy, HTTP exchange interfaces and activation gates do not
change. Public auth output remains disabled through T-941.
