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
