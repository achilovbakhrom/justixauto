# Independent code review — R1

Actor/run `/root/company_form_code_review`, 2026-09-26. Response preserved verbatim:

BOUNCE

- Identity: `/root/company_form_code_review`
- Mode: `CODE`
- Reviewed range: `eca22f60f9082f7fe59004c81be63170ebfe1483..db055e46d3d2d022f6eddb1a72d563106a5de384`
- Head SHA: `db055e46d3d2d022f6eddb1a72d563106a5de384`
- Cumulative diff SHA-256: `7fac3b2980c7146932636a434be1285b5bb82d652dff5e54108494eead360235`
- Requirements reviewed: `CF-01`–`CF-08`, `H1`

Blocking finding:

1. `CF-07` — identical errors for the two email API paths are still duplicated and one is incorrectly classified as an unmapped error.

   In [ui.tsx](/Users/bakhromachilov/startups/justixauto/web/packages/kit/src/ui.tsx:798), the first matching email error is added to `byField.email`. When the second API key has the same message, the duplicate check fails the whole `if`, so line 801 adds the matched key to `rest`. Line 804 then renders it in the general notice. For:

   ```text
   company.email: Некорректный адрес
   firstAdmin.email: Некорректный адрес
   ```

   the user receives one adjacent field error plus `firstAdmin.email: Некорректный адрес` in the notice. This defeats the intended deduplication and violates the CF-07 boundary that mapped email errors belong by the one visible email control while the notice is for unmapped or hidden fields.

   The regression test at [form-dialog.test.ts](/Users/bakhromachilov/startups/justixauto/web/packages/kit/src/form-dialog.test.ts:59) checks only the number of `.field-error` elements, so it misses the duplicate notice. The mapped-key and duplicate-message branches need to be separated, and the test should also reject a notice containing `firstAdmin.email` or a second occurrence of the message.

Other reviewed evidence:

- `CF-01`–`CF-05`: the create form has one company-name/email control, four ordered native fieldsets, dual email serialization, preserved endpoints/kinds/password fields, and edit retains fetched `legalName` plus the current `If-Match` revision. Evidence: `pages.tsx:96-179, 212-249, 343-372`; backend replacement and provisioning contracts: `company.go:52-68, 119-193, 239-280`.
- `CF-06`: country remains free-form with canonical catalogue spelling; region depends on and clears with country changes. The seven non-overlapping precursor paths match their recorded entry SHA-256 values byte-for-byte.
- `CF-07`: the synchronous `submitting` ref prevents click/Enter duplicate dispatch; values remain stateful across failures. Distinct nested email messages accumulate correctly. Only the identical-message branch above fails.
- `CF-08`: grouped forms retain flat serialization; fieldsets span the outer grid and collapse to one column below 600px. Existing ungrouped fields remain on the prior rendering path.
- `H1`: `lefthook.yml:28` contains the authorized one-line `--no-warn-ignored` addition while preserving `--max-warnings 0`; no other hook behavior changed.
- The added kit manifest entries use dependency versions already resolved elsewhere in the existing lockfile; no new resolved library versions were introduced.
- The cumulative diff contains exactly the stated 13 paths and passes static `git diff --check`.

Unverifiable areas:

- Per reviewer constraints, I ran no unit, type, build, lint, hook, browser, or accessibility execution.
- Responsive layout, browser datalist behavior, request interception, and actual hook positive/negative controls remain QA-owned.
- Intentional dirty canonical docs and backup artifacts were excluded from the reviewed commit range; application/tooling source at `HEAD` matched the target SHA.
