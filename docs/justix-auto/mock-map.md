# Runnable mock map

All paths below are relative to `mocks/`. Run `npm run mocks:serve` at the project
root. Do not implement product policy by copying demo constants. Read the
business handoff, then the exact screen/state. No Figma or retired role apps.

| App / feature | Entry and navigation | Relevant shared/runtime files |
|---|---|---|
| Realization shell, Dashboard, Vehicles, Warehouses | `/` → sidebar | `app.js`, `realization.js`, `company-directory-bridge.js`, `common.css`, `styles.css`, `realization.css` |
| Vehicle specifications and photos | Vehicles or warehouse inventory → row → details; receive/add modal | `vehicle-fields.js/css`, `vehicle-photos.js/css`, `app.js`; exterior/interior separate |
| Partners, procurement, offers, CRM and retail | `/` → respective sidebar entries; row opens detail | `app.js`, `realization.js`, `realization-domain.js`; wholesale parity has documented gaps |
| Installment estimate and seller→financier request | Sales → financing action → organization → program | `partner-finance.js`, `partner-finance-domain.js`, `finance-bridge.js`, `finance-exchange.js` |
| Financier programs and application review | `/finance/` → Overview / Applications / Programs | `finance/workspace.js/css`, `finance/demo-data.js`, shared finance modules; seven demo examples per seeded financier |
| Post-agreement documents and interaction history | Financing → agreed application → documents | `finance-documents.js`, `finance-documents-domain.js`, `finance/history-stepper.js` |
| Seller insurance requests | `/` → Страхование | `insurance-ui.js`, `insurance-bridge.js`, `insurance-exchange.js`, `insurance.css` |
| Insurer review | `/insurance/` → Overview / Applications | `insurance/workspace.js`, shared insurance modules |
| Admin companies, provider integration, users and roles | `/admin/` → Companies / Integrations / Users / Roles / Audit | `admin/admin.js/css`, `admin/management.js`, `platform-directory.js`, `integration-registry.js` |
| Provider account creation and demo login | Admin → MFO/Bank/Insurer → Add company; activate → linked app | `demo-accounts.js`, `integration-access.js`; not production auth |
| Country/region autocomplete and company settings | Add/edit company in Admin or Realization Settings | `company-fields.js/css`, `company-directory-bridge.js`; dependent demo dictionary |

## How agents should inspect this bundle

1. Read the product rule and open-decision IDs for the requested feature.
2. Open the app and follow the explicit navigation above; inspect the starting
   state, modal, validation and result without destructive demo operations.
3. Read only relevant functions/files if interaction evidence is insufficient.
   Global function/class names are retained, but local variable names are minified.
4. Use the adjacent `*.test.cjs` as examples of existing demo invariants, not a
   production coverage claim. `npm run mocks:test` runs all 106 baseline tests.
5. Never load the whole bundle merely to find a label. Never copy an old source
   override into the production React app. Build components against contracts.

## Bundle boundaries

Four entries, one shared dependency graph, three vehicle illustrations, demo
financial attachment fixture and existing regression tests. No archived Figma
pages, screenshots, retired dealer/distributor directories, node_modules,
credentials, state snapshots or duplicated original source are packaged here.
The filenames are intentionally unchanged so relative links and classic-script
order remain valid. Conservative minification does not remove legacy helper
functions that current scripts might still override or reference.
