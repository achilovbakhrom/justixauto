# Hub acceptance and dispatch

Primary application hub, 2026-09-26: accepts independently GREEN manifest digest
`cfc12959fd8d67038b9b3d4f18b9b8d16197d47797805dc416450749bb4473a3`.
See plan-review.md for the independent response and provenance reconciliation.

Reserve sole application writer to `/root/company_form_worker` for P1. Branch
`fix/country-region-combobox`, base `eca22f60f9082f7fe59004c81be63170ebfe1483`.
No branch switch. Entry snapshot hashes pin all existing geo changes. The attempted
precursor checkpoint failed only the existing FieldInput complexity gate (27 > 20),
with no source edits; version_control restored its own index staging to empty.
One cumulative candidate commit will contain the reviewed exact precursor plus P1
paths. Reviewer/QA cover base..candidate after the writer releases the checkout.

The prior canonical record's planned separate checkpoint is superseded by this
actual hook result and frozen packet. No hook bypass, dev/main write or deployment.
Unchanged source outside P1 retains its original snapshot bytes. Only focused
combobox extraction is authorized to repair the existing complexity failure.

Browser is unassigned until candidate QA. No other application writer is dispatched.
This authorizes implementation, not QA GREEN or closure of T-426/B-11.
