# Retained generation label storage mismatch

2026-09-15. Coordinator finding during T-014 implementation review. This records
an incomplete compatibility guarantee; it does not change installed SQL.

T-917 `000002_messaging_delivery.up.sql` permits exact nonempty text generation
labels with `length(generation)>0`. The approved generation-label clarification
forbids trimming or rewriting retained labels. T-926's immutable
`000004_projection_checkpoint.up.sql` instead uses
`length(btrim(generation))>0` on `consumer_bootstraps.generation` (line 124).
It preserves padded nonblank labels, but rejects an all-space retained label
that the earlier admission accepts. Prior T-926 tests cover ordinary non-UUID
projection/process labels, empty labels and mismatches; they do not establish
all-space compatibility. The coordinator's working summary overstated that
coverage; the actual SQL and retained test evidence take precedence.

T-014 must preserve exact generation input without trim rejection and document
the live storage rejection for this case. Other bootstrap, sequence, gap and
checkpoint mechanics can proceed, but full retained-label compatibility remains
unfinished. Do not fabricate successful bootstrap evidence or edit 000004.

Proposed bounded correction: the still-unimplemented T-928 forward generation
migration owns a precise constraint replacement on this existing column, after
verifying the exact prior constraint/artifact/owner state under its installation
locks. Replace only the stricter generation check with exact nonempty text,
preserving all rows, keys, admission equality, ACLs and old migration bytes.
Its new immutable compatibility marker must identify this behavior. Test both
projection/process all-space labels, padded labels, empty rejection and exact
mismatch rejection, plus retained-data and failure atomicity. T-016 already
depends on T-928; owner readiness must not claim this guarantee without the
correction marker. Other reference/purpose fields keep their approved checks.

This is a proposed technical scope amendment, requiring independent review and
canonical task handoff before T-928 assignment. It grants no reset, new consumer
admission, production migration or reinterpretation of a process label as a
projection UUID. Existing artifact SHA-256 remains
`41536429fb95d4b60a11866843a2ccbd6a4cf723cd919884933b6bc1d8202c4c`.

Inputs: `state/approvals/projection-generation-label-compatibility.md`, T-917 and
T-926 SQL/test leaves, T-014 author finding, and T-928 ownership/dependencies.
