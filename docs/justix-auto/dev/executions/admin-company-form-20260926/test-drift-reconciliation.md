# Preserved test selector edit

During the CF-07 repair an external/unattributed edit appeared in the already
inventoried precursor path `web/packages/kit/src/combobox.test.ts`. Initial repair
and prior QA source were clean; final repair status showed this file modified.
QA and repair actors deny editing it; provenance remains unknown. Never attribute
this change to either actor or claim its worktree bytes still match entry.

Primary inspected the complete delta: four country-label queries add
`{ exact: false }`, tolerating required-marker suffixes. No assertion, tested
behavior, field mutation or application code changes. Preserve these edits and
include them as an explicitly adopted test-only delta in the cumulative review
and final candidate. No production behavior or task requirement changes. Source
must remain frozen and all final test cases must still execute; this is not a
waiver of country/region coverage or a claim that its author was identified.

Entry SHA-256: `f2ea9d30ab48c20b0d34342df2b2f850c96f45c26594565fa053c3f236b38749`.
Observed/preserved SHA-256: `adb57e7e6e28ed799c8092ebaeb5f6e678ddc167b9e88a37b6c18239c36195c4`.
Recoverable copy and exact delta: `state/backups/admin-company-form-20260926-test-drift/`.
The other six protected precursor leaves remain bound to the original entry
snapshot. The final reviewer must inspect this delta and QA must execute all
five combobox tests at the final committed SHA. Any further unexpected drift
must be reconciled before claiming final evidence.

## Subsequent provenance evidence

At final handoff the primary read the separate session's preserved
`dev/results/FIX-GEO-1.md`. That actor reports implementing the geographic
precursor and correcting the four country-label selectors before detecting the
shared-checkout collision and ceasing edits. This is consistent with the
observed delta and supplies reported authorship; the earlier statement above
records what was known when the edit was adopted. The report is preserved
without rewriting or claiming that this task owns or closes FIX-GEO-1.

The separate report speculates that broad staging absorbed its changes. This
task instead recorded and adopted the ten existing precursor paths at entry,
used explicit-path version-control commits, and requested cumulative review
from `eca22f60`, not merely review of the onboarding additions. Coordination
across the two sessions was still incomplete; this reconciliation does not
claim that the other actor had released its writer before the initial edits.
No further application drift was observed during final review and QA. Full
cumulative review, all five combobox tests, source hashes, and final browser
checks now cover the combined candidate at `f9c4f356`. Future work must obtain
one shared-checkout writer lease before editing.
