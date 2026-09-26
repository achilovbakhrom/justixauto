# Independent acceptance aggregation

Actor: `/root/company_form_aggregate`; role: `qa_aggregator`,
`gpt-5.6-sol` / high. Response below persisted verbatim by the primary.

GREEN — READY_FOR_HUMAN_APPROVAL

Candidate: `f9c4f3564afb195b48cba2e66c9be3e4d00d2948`  
Tested base: `eca22f60f9082f7fe59004c81be63170ebfe1483`  
Branch: `fix/country-region-combobox`

Ancestry is linear:

`eca22f60 → fce20e65 → db055e46 → 450fb48e → e8709bb → f9c4f356`

The final leaf from `e8709bb` contains only three CSS lines in `design.css`. Its SHA-256 is `84bdba343c035c438329d4d526980a9fb665de6948846f1e2190bbbd831c1670`. The cumulative base-to-final diff SHA-256 is `87f67935ab34a5f3122f020f095aeffdb10bdc626a256b68a2ab0d1e3c3859bb`, matching R3. Current `HEAD` is the exact candidate, and all 13 application/tooling paths match `HEAD` in both worktree and index.

Coverage:

| Requirement | Audited evidence |
| --- | --- |
| CF-01 | Final-SHA Chrome confirms one company name, one email, separate administrator name, and no visible legal-name/admin-email controls. |
| CF-02 | Final-SHA Chrome confirms four correctly ordered native fieldsets, associated labels, required markers and DOM order at desktop and mobile widths. |
| CF-03 | Final-SHA seller submission maps the one email to both API objects. Exact bank/MFO/insurance payload evidence comes from `e8709bb`; all relevant TypeScript, tests and contracts are byte-identical at final SHA. |
| CF-04 | Final Chrome covers clear seller/provider wording, draft refresh behavior, unchanged active-company context and separate activation. Same-source evidence covers all provider endpoints and kinds. |
| CF-05 | Final Chrome covers legacy and empty `legalName`, exact company ID and `If-Match`, retained stale edits, and absence of administrator/user mutation. |
| CF-06 | Final Chrome covers grouped dependency reset/disable and canonicalization. Five combobox and five geo tests passed on identical TypeScript source. Six protected precursor leaves retain their entry hashes; the adopted selector delta is independently reviewed at hash `adb57e7e…`. |
| CF-07 | Final Chrome covers distinct and duplicate email errors, hidden/unmapped errors, pending replay prevention, cancel, network/conflict/stale retention and successful retry. Six same-source FormDialog tests cover the repaired routing and replay guard. |
| CF-08 | Final-SHA four-app build and Chrome cover the three-line CSS repair, desktop/mobile geometry, overflow and footer reachability. Final Chrome also covers the ungrouped Admin activation form; same-source units cover shared ungrouped serialization. |
| H1 | Positive mixed/ignored-only controls and the malformed-TypeScript negative control passed on `e8709bb`; `lefthook.yml` is byte-identical at final SHA. |

The final browser integration run executed at `f9c4f356` in Chrome 154 with isolated synthetic API interception. The final screenshot receipt records:

- Desktop: `4d745701e89be108b25aae21036f48503bb50aa0ff297774d8714d3713faa17a`
- Mobile: `494dea23f3ef52ac13de5c376157ab0de666ae2ea6c8640662431b6e1940d91a`
- Errors: `070b73b667b886dabe5421b7bd353f155997ab093e5e6b2e75fc3a22693e8b5d`

Limitations are explicit and non-blocking:

- Unit, typecheck, lint, hook controls and all-kind payload checks executed at `e8709bb`, not at `f9c4f356`. They are accepted only through the recorded exact equivalence proof; they must not be described as final-SHA executions.
- Realization was not opened in a second browser fixture. Its ungrouped country-field consumer is unchanged, the final CSS selector applies only within `.form-fieldset`, and final four-app build plus same-source shared behavior tests cover the interface. No extra browser packet is warranted.
- Synthetic interception does not prove backend transactionality, authorization, persistence, ownership policy or database concurrency. Backend code did not change.
- The geographic precursor’s initial cross-session writer coordination was incomplete. Its four selector changes were subsequently adopted, attributed through the preserved external report, independently reviewed and executed; six other protected hashes remained unchanged. This leaves no unresolved source finding.

No additional QA packet is necessary.

For a later docs-only publication commit `P`, avoid repeating runtime QA if all of these are recorded:

1. `f9c4f356` is an ancestor of `P`.
2. `f9c4f356..P` contains only documentation, receipts, screenshots and the preserved synthetic harness.
3. All 13 application/tooling paths are byte-identical between the two SHAs.
4. The source-only cumulative diff from `eca22f60` retains hash `87f67935…`.
5. The human handoff identifies `P` as the publication branch head and `f9c4f356` as its tested implementation parent.

Any application/tooling change after `f9c4f356` invalidates that equivalence and requires affected review/QA.
