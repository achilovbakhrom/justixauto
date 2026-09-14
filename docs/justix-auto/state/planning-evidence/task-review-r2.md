# PM revision recheck — R1–R7 only

2026-09-13. Bounded review of the revised 914-task proposal; no architecture or scope reopening. Index inspected:

`../pm/dev/task-index.json`

SHA-256: `61de50fe567286d048c5db69bb4c870ec1529db3cef72b66decd216bd2b98bb5`.
Size: 1,403,581 bytes; fingerprint unchanged at review completion.

Evidence: compact index/scope/file inventory, transitive ancestor checks, targeted materialized files T-694/T-729/T-627 and original finding references. All original T-001–T-639 IDs remain. Graph-validator success does not by itself establish semantic runtime dependency closure.

## R1 — PARTLY RESOLVED; remaining HIGH dependency-closure blocker

The blanket gates are removed: T-580–T-586 now build minimal feature-independent runtimes; T-587–T-590 load only existing route exports; T-746 defines inert registration. T-747–T-914 assign incremental feature adapters/registration. This fixes the former requirement to finish every policy/feature first.

However the independent cross-owner acceptance tasks omit required **counterpart registrations**, not merely optional features:

- **T-627 (retail reservation acceptance)** transitively includes caller client registration T-902 and local T-834/T-835/T-836, but **does not include T-810/T-811** (inventory reservation acquire/cancel registrations). It also omits T-895 (retail durable lookup registration) and T-751 (live-authorize route registration). Module T-173 is not registered runtime T-810. Thus the declared cross-boundary sale/reservation and recoverability checks can become ready without the authoritative destination endpoints. Its task file promises combined AC closure, not just an `httptest` port test.
- **T-601 (provider journey acceptance)** does not transitively include T-747/T-748 (credential/MFA registration), T-774 (activation registration), or T-589/T-590 (independent financing/insurance app runtime). The approved B-11 journey includes explicit activation and admitted empty workspace entry; local provider handlers plus Admin wiring alone do not close it.

**Correction:** add the precise runtime producer dependencies to these feature acceptance closures (and apply the same counterpart/lookup/auth closure check to the other existing cross-owner acceptance tasks). Do not restore all-owner or all-feature barriers. If a check intentionally uses an isolated composition instead, explicitly assign that composition and avoid calling it combined runtime acceptance.

## R2 — PARTLY RESOLVED; remaining HIGH generated-client readiness gap

Resolved producers: T-640 owns deterministic generation/config; T-641–T-687 own exact generated Go/web client/validator leaves; T-742–T-745 cover additional private schemas. T-540 now depends on concrete T-242 and generated T-681; T-362 depends on T-059/T-641. The former missing-generation and prose-only-fixture problem is corrected.

A few new remote adapter consumers still depend only on the generator executable and schema, not the exact generated producer:

- **T-729** owns only `retail_reservations_client.go` and its test, and depends on T-031/T-640/T-172/T-214, but **neither T-666 nor T-675** is an ancestor. At least T-666 supplies its inventory reservation DTO/client artifacts.
- **T-727** consumes identity branch attachment responses but **T-655** is not an ancestor.
- **T-733** consumes financing/insurance decision DTOs from T-242/T-263 but **T-681/T-685** are not ancestors.

**Correction:** add the relevant generated output dependencies to these typed adapters (and verify the same rule for the bounded T-726–T-741 adapter set). T-640 availability is not execution of each feature's generation task; adapters cannot create generated leaves they do not own or silently handwrite parallel contracts.

## R3 — RESOLVED

T-694 now explicitly owns retail private acquire/consume/release/status/snapshot OpenAPI, fixtures, event schema and `0061_submission_intents.up.sql`, including guard/tombstone/admission/minimum-snapshot records. T-337/T-338, T-358 and T-360 transitively depend on it; T-742 owns its generated outputs. Financing/insurance snapshots remain in their owners, with no new finance uniqueness policy.

## R4 — RESOLVED as deliverable ownership; runtime closure remains under R1

T-712 supplies storage/scanner ports; T-713 private immutable filesystem; T-714 staging stream; T-715 version-pinned object adapter; T-716 ClamAV I/O; T-717 durable scan jobs. T-085/T-086/T-088 consume appropriate concrete adapters. T-726–T-741 assign remote-owner and document-authority adapters. T-747–T-914 explicitly own projector/subscriber/registration leaves with payload-only, dedup/sequence tests and a no-invented-event exception for command/client-only slices. T-599 includes concrete storage/scanner and document registration tasks. These are no longer unowned work hidden inside terminal registration.

## R5 — RESOLVED as owner lookup implementation and B-07 coverage

T-718 provides shared owner-local query mechanics; T-719–T-724 each own public/private lookup schema, HTTP/store adapter and scoped/recovery tests for the six non-identity owners. Existing identity T-080 plus T-759 remains. T-893–T-898 register the new endpoints; all seven registration paths are ancestors of B-07 acceptance T-597. No centralized identity/edge business ledger is introduced. Feature-specific lookup closure is the R1 follow-through above.

## R6 — RESOLVED

T-520 now owns cash-only binding/QA and depends on T-220/T-221 plus their T-837/T-838 registrations. Its transitive ancestors exclude T-222, T-048, T-049 and T-711. T-711 separately owns gated own-contribution child/slot binding, and B-38 combined acceptance T-628 retains both portions. Cash independence is real in the graph, not just explanatory prose.

## R7 — RESOLVED for the identified scopes

T-250 is now only submitted→review; request/response/decline are T-688/T-689/T-690. T-269 is only insurer take-review; T-692/T-693 own request/response. T-362/T-363 explicitly name SignIn/MfaChallenge; T-699/T-700 separately name enrollment/recovery-invitation with confirmed-supplement gates and explicit exclusion of an Admin bootstrap screen. T-580–T-583 are minimal runtimes, with later per-feature registration/adapter units instead of dozens of modules at once. These changes resolve the original concrete size/scope ambiguity; estimates remain planning estimates, not measured implementation durations.

## Disposition

**Targeted BOUNCE remains for R1/R2 only. R3–R7 resolved within this recheck.** The remaining changes are dependency closure, not additional product requirements or another architecture decision. No task/canonical artifacts were edited by this reviewer.
