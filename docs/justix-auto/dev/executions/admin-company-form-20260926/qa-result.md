# Q1 result — GREEN

- QA actor: `/root/company_form_qa`
- Tested final SHA: `f9c4f3564afb195b48cba2e66c9be3e4d00d2948`
- Tested base: `eca22f60f9082f7fe59004c81be63170ebfe1483`
- Branch supplied by coordinator: `fix/country-region-combobox`
- Source state before/after final checks: application/tooling worktree and index matched `HEAD`; only coordinator records, backups and assigned QA evidence were dirty.

## Acceptance coverage

| Requirement | Result | Evidence |
| --- | --- | --- |
| CF-01 | GREEN | Final Chrome: one company name, one email, separate admin name; no legal/admin-email controls. Admin rendered tests passed. |
| CF-02 | GREEN | Final Chrome: four ordered native fieldsets, labels and DOM order; desktop/mobile screenshots. |
| CF-03 | GREEN | Final Chrome seller body mapped one email to both API objects; explanation visible. Same-source all-kind bodies passed. |
| CF-04 | GREEN | Seller/provider entry wording inspected; seller final submission and same-source all-kind endpoint/kind submissions passed; draft flow and separate activation verified. |
| CF-05 | GREEN | Final Chrome legacy and empty legal-name edits preserved the fetched value, company ID and exact `If-Match`; stale input retained; no user/admin mutation. |
| CF-06 | GREEN | Five combobox and five geo tests passed; grouped browser dependency/canonicalization passed. Six protected precursor leaves match entry; adopted combobox test hash is `adb57e7e6e28ed799c8092ebaeb5f6e678ddc167b9e88a37b6c18239c36195c4`. |
| CF-07 | GREEN | Six form-dialog tests include repaired duplicate routing and synchronous replay guard. Final Chrome covered distinct/duplicate email errors, hidden/unmapped notice, pending replay, cancel, network/conflict/stale retention and successful retry. |
| CF-08 | GREEN | Final CSS repair built in all apps. Desktop two-column and mobile one-column geometry, footer reachability and overflow assertions passed. Ungrouped shared form unit coverage and Admin activation browser flow passed. |
| H1 | GREEN | Both positive `--no-warn-ignored` controls exited 0; malformed TypeScript negative control exited 1 with the expected parser error. |

## Executable evidence

- Pinned Node `24.21.0` and npm `11.19.0`.
- Selected Admin/Kit/Realization/Financing/Insurance unit suites: 10 files, 28 tests, all passed. New/critical discovery: `company-form.test.tsx` 3, `form-dialog.test.ts` 6, `combobox.test.ts` 5, `geo.test.ts` 5.
- Typecheck passed for config and every workspace.
- Scoped ESLint and hook controls passed. `FieldInput` stayed below the established complexity limit according to the successful scoped lint; the completed React Doctor run is assessed separately and is not treated as behavioral acceptance.
- Build passed at `e8709bb` and again at final SHA `f9c4f356`; all four applications produced Vite bundles after the CSS fix.
- Final Google Chrome browser run exited 0 with synthetic API interception at 1440×900 and 390×844.

## Limits and exclusions

- API interception verifies rendered UI, payloads, headers and lifecycle behavior. It does not verify backend transactionality, authorization, persistence, email ownership policy or database concurrency; those services were unchanged and no live backend was authorized.
- Realization was not opened in a second browser fixture. Its country-field consumer is covered by byte-identical source provenance, selected units, all-workspace typecheck/build and shared ungrouped-form tests, per the coordinator's bounded final-QA direction.
- The independent QA React Doctor package-resolution attempt at the earlier candidate failed due DNS. The later authorized repair run completed (65/100, 30 warnings); the scoped assessment records no blocking finding. No package was installed into the project.
- No production, deployment, external message, real credentials or shared fixture was used.

Evidence files: `qa-commands.log`, `qa-browser.md`, `qa-browser.cjs`, `qa-desktop.png`, `qa-mobile.png`, `qa-errors.png`.
