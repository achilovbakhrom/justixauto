# PM proposal review — material readiness findings

2026-09-13; reviewed the 639-task index, board and targeted task files against `readiness-checklist.md`. This is a bounded qualitative review, not a claim to have inspected every task. Existing approved architecture/release scope stands. The coordinator independently validates graph, ownership and coverage. Credentials/security-release and shared-finalize/wholesale-policy issues already being corrected are not duplicated below.

## R1 — HIGH: terminal all-feature wiring recreates blanket readiness gates

**T-580–T-590; concrete affected acceptance tasks T-601, T-624, T-627.** T-583 requires every retail feature, including T-231/T-235/T-236 (own calculation/servicing) and live-data T-278. T-588 requires every realization binding, including T-532 (servicing), T-520 (installment linkage) and all missing-state supplements. Consequently ordinary lead/reserved-sale acceptance T-624/T-627 requires unrelated money policies through T-583/T-588. T-580 includes gated suspension/recovery, and T-587 requires all Admin features, similarly delaying the first provider journey T-601. The board's “typed ports allow independent work” only enables isolated modules; it does not provide independently runnable/QA-able features.

**Required correction:** bounded base composition plus ordered feature registrations (or explicitly assigned per-feature runtime test compositions), with each feature's integration/acceptance depending only on its actual owner routes/consumers and scoped policy. Preserve one writer per shared registry; do not clear this by dropping runtime checks or by marking unavailable features implemented.

## R2 — HIGH: generated API clients and schema-checked presentation have no producing task

**T-032, T-059, T-242, T-540, T-542; analogous schema/presentation tasks.** T-032 supplies only generic transport/query keys and says feature schema adapters are injected. T-059/T-242 own OpenAPI/event JSON/fixtures/SQL, not generated Go/web clients or generation tooling. No index task owns generator/config/generated-client files. T-542 nevertheless requires the feature's generated owner API. T-540 promises schema-checked fixtures but depends on prose-contract T-241, not concrete-schema T-242; the same pattern exists in T-362 versus T-058/T-059. Worker ownership prevents silently creating these missing outputs elsewhere.

**Required correction:** assign deterministic generator/config ownership and bounded per-feature generated DTO/client/runtime-validator outputs; wire concrete schemas into fixture validation and consumer dependencies. Documentation-only contract approval is not a generated artifact.

## R3 — HIGH: retail submission storage lacks a retail-owned schema producer

**T-337/T-338, T-357/T-358, T-359/T-360, T-242.** T-337 starts from financing schema T-242; T-338 must consume an existing schema and may not alter migrations. T-242 only owns `services/financing/.../finance_submission.yaml` and financing SQL. Retail submission acquire/consume/release/snapshot guards and admission records have no named retail migration/private API producer in the index. T-214 is scoped to retail reservation, not this later submission protocol; T-027 explicitly excludes business tables. Consume/release storage inherits the same gap.

**Required correction:** assign the retail-owned private submission-intent contract, fixtures and local schema/guards, then make acquire/consume/release adapters depend on it. Keep application-owner committed snapshot/decision tables separate. This is the same ownership discipline already applied to inventory branch schema T-287.

## R4 — HIGH: injected ports and registries cannot close the missing runtime adapters

**T-085–T-088, T-030, T-277, T-586; T-580–T-586.** Document tasks own domain/app/HTTP/PostgreSQL files only. T-030 defines ports/mechanics, T-277 validates live configuration, and T-586 is explicitly registration-only. No task owns private immutable filesystem/object-version storage, staging upload streaming or the ClamAV client/scan-job I/O implementation. Thus the exact-byte safety workflow has no actual external I/O producer. Across all owners, the only owned `adapter/projection` and `adapter/amqp` files are terminal `registry.go` files, while wiring tasks forbid new behavior. Shared broker/checkpoint mechanics are not owner-specific projection handlers. The task index does not explicitly allocate those implementations to other leaves either.

**Required correction:** assign bounded storage/scanner and owner projection/subscriber/remote-owner adapter outputs with exact dependencies and attack/crash tests. If a current task is intended to provide one, name its concrete adapter responsibility and owned leaf explicitly instead of leaving it for registration or acceptance to invent.

## R5 — HIGH: recoverable operation/receipt endpoint is assigned only to identity

**T-078–T-080, T-579, T-581–T-586, T-597.** T-080's only domain/app/HTTP/PostgreSQL implementation is `services/identity/.../operation_lookup.go`; T-079's schema is identity-only. Yet the approved contract routes operation and command-receipt lookups to the actual deciding owner, with no edge/identity registry. T-579 routes but owns no owner-ledger behavior. Other owner wiring cannot implement new endpoints under its explicit registration-only scope. Reservation/submission step tasks do not explicitly own the public receipt lookup across their owner ledgers.

**Required correction:** assign shared lookup mechanics plus per-owner schema/HTTP bindings and scoped absence/recovery tests for all seven ledgers; connect B-07 acceptance T-597 to those producers. Do not centralize their business ledgers in identity.

## R6 — HIGH: cash FE binding directly waits on installment/insurance policy

**T-520, T-220/T-221 versus T-222/T-048.** Even independently of R1, T-520 is the only retail contract/payment binding and has all-of dependencies on cash T-220/T-221 plus installment T-222 and coverage decision T-048. Its all-actions scope prevents finishing the approved cash journey while insurer→contribution linkage remains unanswered. B-38 and board conditional semantics explicitly exclude this cash dependency.

**Required correction:** split cash binding/feature QA from own-installment linkage binding/QA (or explicitly bounded separately completable outcomes with matching dependency semantics), preserving gated release coverage of both.

## R7 — MEDIUM: several four-hour labels are not bounded executable slices

**T-250, T-269; T-362/T-363; T-580–T-583.** T-250 combines take-review, information request, seller response with document bindings and decline across domain replay, handler, HTTP, PostgreSQL and integration tests in four hours. T-269 similarly combines three commands across those layers. Neither identifies a narrower state/adapter slice comparable to the already split saga tasks. T-362/T-363 divide by inherited AC numbers rather than explicit screens: AC3 includes deployment-only bootstrap, and AC4 includes gated recovery/invitations; those are not a concrete Admin “Presentation B” screen inventory. T-580–T-583 then introduce three binaries, image, routes and consumer/projection registrations plus runtime tests for dozens of modules in a single four-hour task.

**Required correction:** explicitly identify command/screen and layer boundaries and reslice these items before declaring the size rule satisfied. Keep synthetic fixture work independent, but “reslice if exceeded” is not evidence the current scope fits four hours. Do not invent a bootstrap Admin screen from a backend AC.

## Disposition

**BOUNCE for targeted revision** on R1–R6; resolve R7's concrete scope/size ambiguity in the same revision. No new product-policy answer is requested by these findings. Receipt known/unidentified modes and separately owned branch commit/abort/detach tasks are present; no reason to reopen those approved protocols.
