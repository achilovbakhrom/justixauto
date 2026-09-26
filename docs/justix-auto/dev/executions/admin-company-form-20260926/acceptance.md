# Q1 — Admin company form correction acceptance

One independently executed QA packet for P1 in packet.md. Status: planned, not run. Authority: primary application hub. No canonical task closure claimed. Role: `gpt-5.6-sol / high`, fresh context; 10,000 input-token budget, reading this manifest, packet scoped rules, worker result and exact changed symbols only. No code/Git/canonical writes. Independent reviewer checks the entire correction range and preserves source-contract boundaries before acceptance.

Dispatch supplies full candidate SHA from version_control and entry snapshot provenance. The coordinator selected one cumulative final commit on the same `fix/country-region-combobox` branch and base `eca22f60f9082f7fe59004c81be63170ebfe1483`; review and QA cover the complete base..candidate diff, including all ten precursor files inventoried in packet.md. The failed precursor checkpoint did not alter source; worker starts from documented dirty bytes, then QA uses clean frozen candidate application source. No candidate SHA is invented here. Reserve immutable checkout and exclusive browser; use isolated API interception for deterministic frontend browser evidence, or an explicitly named authorized local fixture. Do not start/deploy a backend or mutate shared/production data implicitly. API interception demonstrates UI behavior and payloads, not backend transaction/auth correctness; backend behavior is unchanged and independently reviewed against the source contract.

Evidence ownership within `docs/justix-auto/dev/executions/admin-company-form-20260926/`: `qa-result.md`, `qa-commands.log`, `qa-browser.md`, `qa-desktop.png`, `qa-mobile.png`, `qa-errors.png`. QA writes only those evidence files, not this plan; temporary harness, if needed, lives under `/private/tmp/justixauto-admin-company-form-qa/`. Raw logs must redact credentials/cookies and use synthetic test inputs. The hub persists reviewer/coverage conclusions separately under its authority.

## Executable checks

From the repository root, use pinned Node/npm. These commands run on the frozen candidate; no installs or manifest changes:

```sh
export PATH="$PWD/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin:$PATH"
node --version
npm --version
npm run test:unit -- --project @justixauto/admin --project @justixauto/kit --project @justixauto/realization --project @justixauto/financing --project @justixauto/insurance
npm run typecheck
npm run build
npm exec -- eslint --config web/eslint.config.js web/apps/admin/src/pages.tsx web/packages/kit/src/ui.tsx web/packages/kit/src/shell.tsx web/apps/admin/src/company-form.test.tsx web/packages/kit/src/form-dialog.test.ts --max-warnings 0
```

Require Node 24.21.0/npm 11.19.0. Record test files/counts, including the new admin/company-form.test.tsx and kit/form-dialog.test.ts plus existing kit/combobox.test.ts and geo tests; zero discovery is a failure, not a pass. Kit config includes `.test.ts`, admin includes `.test.tsx`; use those exact suffixes. If test discovery/tooling fails because of the precursor, report it to the hub without modifying configuration. `npm run build` and typecheck cover all shared consumers in all four apps. Scoped lint must pass the existing complexity gate (the precursor failed at FieldInput 27 > 20); focused combobox extraction stays inside ui.tsx and requires the same unchanged behavior tests. Run available React Doctor after React changes per web/AGENTS.md and the skill path in packet.md; record findings or a precise tooling limitation and let the hub assess scope, without installing a new project dependency. Do not confuse a score with behavioral QA.

For browser QA, start only the local Admin frontend using the pinned toolchain:

```sh
npm run dev --workspace @justixauto/admin -- --port 5176 --strictPort
```

If occupied, coordinate the browser/server lease; do not kill an unrelated process. Use the configured local Admin route entry, current route definitions in `web/apps/admin/src/main.tsx`, a synthetic authorized-session response and isolated API responses through the existing Playwright/browser capability. Test all four company-kind entries. Record browser version, URL, viewport and whether requests were intercepted or backed by a named live fixture. Stop the process started for this packet on completion. A browser/environment blocker is an evidence gap, never substituted with screenshots of static markup.

## Coverage matrix and exact scenarios

| Check | Requirements | Execution/evidence |
| --- | --- | --- |
| Q1-A create payloads | CF-01/02/03/04 | Render actual CompaniesPage with required providers/router. Parameterize seller, bank, mfo, insurance. Fill one company name, one email, administrator name/login/password/confirmation, country/registration; submit and inspect exact endpoint/body. Both email fields equal the one input; provider kind correct; no adminEmail or section metadata serialized; legalName is empty/omitted for new companies. Success closes and refreshes draft list. Assert same active-company context. |
| Q1-B retained edit data | CF-05 | Open existing company then edit via actual CompanyDialog. Start with legacy legalName different from name. Change visible name and company email, submit; assert original legalName, same company ID, revision/If-Match and only company requisites in PATCH. No user/firstAdmin mutation. Reopen fetched updated details. Repeat with empty legalName. Stale response keeps edits and exposes failure; no silent retry or overwrite. |
| Q1-C routed errors | CF-03/07 | Submit and inject ApiError/HTTP validation separately for company.email and firstAdmin.email, then together with distinct messages: all applicable messages by one email control, no duplicate email input. Exercise firstAdmin.login, password/confirmation, company.name, company.country/region and unknown/hidden company.legalName keys. Matching fields get adjacent messages; unmapped keys remain in notice. Values persist, including credentials in the still-open form (never log them). Retry after corrected input succeeds once. |
| Q1-D failure and mutation lifecycle | CF-04/07 | Network error, user_exists conflict and stale revision remain visible without clearing the form. Cancel emits no command. Hold submission pending and attempt click/Enter again: one request, clear processing state. Resolve success: close and refresh. Activation is still a separate reason form; selecting it alone writes nothing. No automatic activation. |
| Q1-E country/region preservation | CF-06 | Run unchanged precursor geo/combobox tests; compare seven non-overlap precursor files, including realization/data.ts, byte-for-byte against the entry snapshot hashes; inspect overlapping pages/ui/CSS diff to ensure precursor hunks remain. In grouped rendered form choose/type a known country, enter region, change country and verify reset. Clear country: region empty/disabled. Enter an arbitrary country outside catalog and arbitrary region, blur/submit: values accepted and serialized. Country spelling canonicalization still works; fieldset rerender does not lose other inputs. |
| Q1-F shared forms | CF-08/07 | In kit test an ungrouped form with text, nested validation key, money and checkbox/multiselect: existing flat field order, initial/edited values, money minor-unit output and selections persist; field error, failure retention, cancel and successful submit unchanged. Exercise ActionButton with no fields as confirmation-only. In browser run existing ungrouped Admin activation-reason form and one company/requisites form in Realization against isolated responses; include no-write cancel and field interaction. All four app typechecks/builds and selected unit suites pass. |
| Q1-G browser layout/accessibility | CF-01/02/04/08 | At 1440×900 and 390×844, inspect creation for seller and one provider; open remaining provider kinds to confirm wording/groups. Exactly four named native fieldsets, one company-name and email input, separate administrator-name, helpful email explanation and password guidance. Keyboard follows visual order; controls have labels; no clipped content/horizontal overflow; scroll reaches actions; errors are visible beside fields. Capture desktop/mobile and error screenshots. Edit exposes three applicable company sections and no credential entry. Check loading/open, cancel, pending, failure, success/reopen states. |

Source product coverage: business-logic §3 context/membership boundary → Q1-A/B/D; §9 creation/requisites/draft/activation/country rules → Q1-A/B/D/E; §11 mutation lifecycle and retained input → Q1-B/C/D/F/G. Every bounded approval bullet is covered: single fields → A/G; four groups → A/G; admin/email mapping → A/C/G; wording/password/all kinds/draft → A/D/G; precursor/errors → C/E; legacy legal-name preservation/no backend policy → B plus diff review; responsive/ungrouped → F/G.

## Final candidate and stop rules

Q1 is both the bounded behavioral packet and final-SHA shared-interface check when its frozen SHA/base remain the final candidate. Final audit maps CF-01..08 to the actual raw evidence and reviewer result, records any skipped cases, verifies precursor provenance, and confirms no writes occurred underneath checks. Earlier GREEN reports cannot replace this evidence. If source/base changes, rerun affected behavioral and shared-consumer checks against the new candidate and record new SHA/base. Do not expand this small correction into account merging, backend policy or a new canonical task.

Stop and report FAIL/BLOCKED for missing candidate/provenance/leases, lost data, wrong endpoint/email mapping, missing browser coverage, shared-consumer regression, new product ambiguity or context overflow. A static label snapshot does not satisfy submission/error/retention coverage. The worker's checks are supporting evidence, not independent QA. Human approval remains necessary for any later dev integration; no such integration is part of Q1.
