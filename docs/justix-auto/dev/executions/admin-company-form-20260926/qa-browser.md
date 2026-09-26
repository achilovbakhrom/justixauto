# Q1 browser evidence

- Tested SHA: `f9c4f3564afb195b48cba2e66c9be3e4d00d2948`
- Base: `eca22f60f9082f7fe59004c81be63170ebfe1483`
- Browser: Google Chrome `154.0.8037.57`, launched by Playwright with `channel: 'chrome'` and an isolated temporary profile.
- URL: `http://127.0.0.1:5176/admin/`
- Viewports: desktop `1440x900`; mobile `390x844`.
- Fixture authority: Playwright intercepted `/api/v1/**` with a synthetic authorized session and deterministic in-memory responses. No backend, shared database, production environment, real account or external request was used.

## Reproduce the browser run

`qa-browser.cjs` is the exact harness preserved with `apply_patch` from the successful-run temporary path `/private/tmp/justixauto-admin-company-form-qa/browser-qa.cjs`. It contains only synthetic fixture identities, values and CSRF text; Playwright creates an isolated temporary Chrome profile and does not load a user's browser profile or real secrets.

Prerequisites are the frozen SHA above, the repository's existing locked `node_modules`, pinned Node 24.21.0, installed Google Chrome at `/Applications/Google Chrome.app`, and exclusive access to loopback port 5176. From the repository root:

```sh
export PATH="$PWD/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin:$PATH"
node docs/justix-auto/dev/local/toolchains/node-24.21.0/lib/node_modules/npm/bin/npm-cli.js run dev --workspace @justixauto/admin -- --port 5176 --strictPort
```

With that server running, invoke the harness in a second shell:

```sh
export PATH="$PWD/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin:$PATH"
node docs/justix-auto/dev/executions/admin-company-form-20260926/qa-browser.cjs
```

The harness intercepts every `/api/v1/**` request and writes the three named screenshots in this execution directory. Stop the owned Vite process after completion. The reported final run used the temporary path above; preservation did not rerun it.

## Rendered behavior

- Creation rendered exactly four native fieldsets in order: `Информация о компании`, `Адрес`, `Контакты`, `Данные для входа`. It exposed one company-name input, one email input and one administrator-name input, with no visible `legalName` or administrator-email input.
- The contact-email explanation and 12-character password hint were visible. Labels, required markers, native control association and DOM keyboard order were inspected.
- Country/region behavior passed in the grouped form: region began disabled, enabled after country entry, cleared when country changed and disabled again when country was cleared. Known-country spelling canonicalized. The earlier same-source all-kind run also serialized the free-text `Германия` / `Берлин` pair.
- A fresh seller creation on the final SHA posted to `/identity/admin/seller-companies`; the one visible email populated both `company.email` and `firstAdmin.email`, `legalName` stayed empty, and no `adminEmail` or grouping metadata was serialized. Success closed the dialog and refreshed the list without changing active-company context.
- Bank mobile and MFO/insurance dialog entry points rendered provider-specific titles/groups. Exact bank/MFO/insurance payload assertions had already passed at `e8709bb`; `git diff --quiet ... -- ':!web/packages/kit/src/design.css'` proves their TypeScript, tests and contracts are byte-identical at the final SHA. Only the scoped CSS alignment rule changed.
- Cancel emitted no mutation. While a creation request was held pending, click plus Enter produced one request and the submit control was disabled; resolving success closed and refreshed.
- Network and `user_exists` conflict responses kept the dialog open and retained entered values.
- Validation was injected separately for `company.email` and `firstAdmin.email`, then together with distinct messages. Both distinct messages appeared beside the single email input. Identical messages deduplicated to one adjacent message and did not expose `firstAdmin.email` in the general notice. Login, password, confirmation, company name, country and region mapped beside their controls; hidden `company.legalName` stayed in the general notice. Corrected retry succeeded.
- Existing-company edit rendered three applicable fieldsets and no credentials. A legacy legal name different from the visible name was preserved in the PATCH; URL, company ID and `If-Match: "company-r7"` were exact, and no administrator/user object was sent. A company with empty `legalName` retained the empty value with `If-Match: "company-r3"`. A stale response kept the unsaved visible name and dialog open.
- Existing ungrouped activation rendered without grouped fieldsets. Opening and canceling wrote nothing; a separate reason submission posted only `{reason}` to the activation endpoint. No automatic activation occurred.
- Desktop had no horizontal overflow. Password and confirmation inputs shared their top position and height within the two-column group after the CSS repair (browser assertion tolerance: one CSS pixel).
- Mobile collapsed to one column, with password followed by confirmation and equal input heights. The modal scrolled to a visible, usable footer with both cancel and create actions and no horizontal overflow.

## Screenshot evidence

| File | SHA-256 | Bytes | Inspection |
| --- | --- | ---: | --- |
| `qa-desktop.png` | `4d745701e89be108b25aae21036f48503bb50aa0ff297774d8714d3713faa17a` | 53,691 | Four grouped sections, aligned password row and complete footer; presentation capture used 1440×1000 after the 1440×900 geometry/overflow assertions; inspected at original resolution. |
| `qa-mobile.png` | `494dea23f3ef52ac13de5c376157ab0de666ae2ea6c8640662431b6e1940d91a` | 31,666 | One-column fields, equal-height password inputs and reachable footer; inspected at original resolution. |
| `qa-errors.png` | `070b73b667b886dabe5421b7bd353f155997ab093e5e6b2e75fc3a22693e8b5d` | 68,417 | Distinct email messages adjacent to one input plus hidden-field notice and other routed field messages; inspected at original resolution. |

The original mock server on port 4180 was not changed or stopped. The latest user screenshot was visually inspected as the historical flat reference; the approved grouped design and explicit correction requirements governed acceptance.
