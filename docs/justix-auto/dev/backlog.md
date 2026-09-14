# JustixAuto — proposed release backlog

Date: 2026-09-13. Status: **SCOPE CONFIRMED FOR PM PLANNING — implementation readiness remains task-gated.**

Target application repository: `/Users/bakhromachilov/startups/justixauto`.
Authority: its `docs/justix-auto/business-logic.md`, approved `architecture.md` and contract appendices, `open-decisions.md`, and `mock-map.md`. User architecture approval is recorded in `state/architecture-approval.md`. It does not answer open business policies or approve this release scope.

This is a value backlog, not the 24-unit technical preview and not a task board. PM decomposition follows the PO checkpoint. Dependency IDs describe value dependencies, not an instruction to complete every policy-gated portion of an upstream item before unrelated work. PM must carry forward the exact dependent contract/state and gate each task separately.

## Release boundary and shared acceptance

- Four applications only: Realization, Financing (bank/MFO), Insurance and Admin. One seller app, one canonical VIN/stock; no retired dealer/distributor entries or Figma dependency.
- Priorities propose a complete release envelope, not unconditional production enablement: **Must** is required within its stated boundary, **Should** is preferred, **Won't (this release)** is Icebox. Policy-gated required capabilities remain in release scope pending decisions; they are not silently deferred or falsely marked ready. Any decision to ship without them is an explicit release amendment.
- Every item requires its own allowed/forbidden-party, membership/branch, state/revision and retry tests. UI hiding is not authorization. Mutations preserve durable history and never impersonate the counterparty; no demo accounts, float money, browser state or copied fixture limits become production truth.
- Commands use the approved receipts, idempotency and revision contracts. Unknown network outcomes remain recoverable/pending; no timeout releases VIN, deletes evidence or marks a remote operation failed. Current bulk receipt/identification endpoints are atomic, not invented partial-success workflows.
- Every FE item includes an exact architecture M-ID, HTML entry/navigation and relevant source filenames below. All filenames in `Design` are relative to target `docs/justix-auto/mocks/`. M-IDs are planning identifiers, not legacy approved screen IDs. Actual viewport/state/file hashes must be captured during implementation; textual mappings are not screenshot QA.
- Existing HTML is the visual authority. Missing auth/MFA, pending branch/reservation/submission, stale/permission/projection-lag/scan-interruption and unified RFQ/factory/logistics states require captured evidence or a user-confirmed HTML/interaction supplement before the affected FE parity task. A nearby M-ID is an integration anchor, **not evidence that the missing state exists**. Backend mechanics and existing-state presentation can proceed independently.
- Existing Russian content is retained. RU/UZ support is **proposed PO release scope**, not a confirmed requirement inferred from the current four-app HTML (UZS is a currency, not a locale). Current sources do not establish approved Uzbek copy, locale entry placement or formatting decisions; B-08 keeps these acceptance decisions explicit. No translation is represented as user-approved.
- Git preflight currently fails because the target inherits `/Users/bakhromachilov/startups`; documentation can proceed, application implementation cannot. The user must establish the target's own Git root and initial commit; no auto-init or parent-repository commits. Version/security/contract locks precede dependent code. No deployment, hosted CI selection or real external integration is authorized here.

## A. Safe, usable and operable workspaces

### B-01 — Recover users' committed work after interruption

**Must.** Value: operators can run and recover the product without losing or duplicating business actions.
AC: (1) Local supported-version-locked environment starts the seven privately owned stores/services, broker and private documents adapter with health/readiness and compatible owner migrations; no cross-owner mutable-table access or destructive automatic migration. (2) Crash, duplicate, lost-confirm and out-of-order tests converge to one authorized effect using replay, transactional outbox/inbox, sequence checkpoints and durable receipts; quarantine/redrive and projection rebuild preserve evidence. (3) Operators can identify stuck operations, outbox age, sequence gaps and scan failure with correlation IDs and documented recovery checks without logging secrets/PII. (4) Provider-neutral verification and reproducible synthetic seed/reset procedures preserve existing mock tooling; new production companies receive no demo vehicles. Go architectural import boundaries and four-app build verification are enforced.
Deps: none. Excludes: cloud deployment, customer business overrides, arbitrary terminal-state editing, Redis/gRPC/WebSockets and production seed credentials. Design: none; CLI/operability enabler, not a new Admin operations screen. Gates: supported dependency/security lock and target Git gate before code.

### B-02 — Sign in, step up and recover access securely

**Must.** Value: each person enters only their permitted workspace using a real identity.
AC: (1) Login/logout, session rotation/expiry and CSRF/Origin defenses follow the cookie contract; no browser credential/token storage. (2) TOTP replay protection, one-use recovery codes and catalogued sensitive-action MFA are tested; ordinary CRM edits do not acquire an invented MFA requirement. (3) Deployment-only single-use bootstrap creates pending MFA enrollment, not a public default admin. (4) Recovery/invitation proof and delivery are enabled only after security acceptance; auth secrets never enter generic command receipts.
Deps: B-01. Excludes: social login, production demo credentials, unrestricted recovery bypass. Design: **M-R01/M-F01/M-I01/M-A01 integration anchors only**; entries `/`, `/finance/`, `/insurance/`, `/admin/`; shells `app.js`, `finance/workspace.js`, `insurance/workspace.js`, `admin/admin.js`; demo login reference `demo-accounts.js`, `integration-access.js`. **Production login/MFA/recovery/enrollment UI is missing and design-gated**; OD-12 proof/delivery/security release gate remains.

### B-03 — Keep each company's data and actions private

**Must.** Value: users and partners can collaborate without exposing another company's records or authority.
AC: (1) Live session/global role/permission, membership, branch, capability and owner party/state checks all apply before commands and before list counts/pagination. (2) Foreign objects/drafts return scoped absence, accessible forbidden actions return denial, authority outages fail closed; positive cached grants admit no new command after revocation. (3) Admitted durable completion may finish under its exact original intent after expiry; fresh intent requires reauthorization. (4) Edge rejects spoofed internal identity headers and service calls require explicit purpose/caller grants.
Deps: B-01, B-02. Excludes: per-membership roles, universal Admin partner powers. Design: none; server behavior consumed by B-05/B-07. Gates: OD-05/11 disputed access/capability combinations only; normal active admitted paths remain buildable.

### B-04 — Switch company and branch without stale context

**Must.** Value: multi-company staff work in the intended company without leaking the previous context.
AC: (1) Server company change atomically resets branch scope to ALL; SELECTED accepts only accessible branches and ALL does not mean platform-wide access. (2) Old requests/dialogs/cached PII are cleared or hidden immediately and late old-epoch responses cannot reappear, including cross-tab changes. (3) Creating a company does not switch context; revoking one membership preserves the others. (4) Company-wide records remain correctly company-scoped alongside selected branch records.
Deps: B-03. Excludes: different roles per company. Design: **M-R01/M-R12**; `/` → company/branch picker; Settings → Companies and branches; `app.js`, `realization.js`, `company-directory-bridge.js`, `company-fields.js/css`. Gates: new stale/revoked-context states need confirmed supplement under B-07.

### B-05 — Navigate four independent, accessible applications

**Must.** Value: each business party reaches its own tools through the existing recognizable workspace.
AC: (1) `/`, `/finance/`, `/insurance/`, `/admin/` load and deep-link independently with prefix-only fallbacks; missing assets/API errors never return another app's HTML. (2) Existing sidebar placement, tables/dialogs and distinct insurer shell match captured HTML viewports; keyboard focus is trapped/restored and inside-dialog clicks do not dismiss. (3) Loading, empty, filtered-empty, read-error, forbidden, missing and expired-session boundaries are implemented only against verified states/supplements. (4) User/company PII cache is not persisted.
Deps: B-02, B-03. Excludes: redesign, dark theme, retired apps or extra navigation. Design: **M-R01/M-F01/M-I01/M-A01**; four entries → existing shell/Overview; `app.js`, `realization.js`, `common.css`, `styles.css`, `realization.css`, `finance/workspace.js/css`, `insurance/workspace.js`, `insurance.css`, `admin/admin.js/css`. Gates: missing production boundary states require explicit design evidence.

### B-06 — Use the seller dashboard to reach current work

**Must.** Value: seller staff can see the current company's workload and enter an existing permitted operation directly.
AC: (1) Ready dashboard shows scoped procurement, active retail/wholesale sales and partner summaries from their owners, not hardcoded demo counts; recent wholesale rows open their exact records. (2) Existing shortcuts reach supplier offers, warehouses and new retail sale with the current authorized context; navigation alone makes no mutation. The wholesale-sale shortcut requires a supported order-entry contract, not an invented seller-originated order API. (3) Empty/loading/error and projection-age handling never expose another company or invent zero during failure. (4) Incomplete setup retains B-16 prerequisite handling, rather than restoring the overridden legacy task dashboard.
Deps: B-04, B-05; dashboard data/actions consume the relevant projections/contracts from B-18, B-23, B-24, B-26 or B-27, and B-37. These are per-widget integration dependencies; shell work can use contract fixtures without waiting for all downstream policy-gated transitions. Excludes: new analytics, new notification center or legacy dashboard restoration. Design: **M-R01**; `/` → ready Dashboard; `realization.js::dashboardPage` override (summary strip, quick actions, recent wholesale table), `app.js` setup/shared helpers only, `common.css`, `realization.css`. Gates: missing server/pending/projection states need B-07 supplement; wholesale entry requires contract/interaction confirmation where current UI exceeds accepted ordering contracts.

### B-07 — Understand and recover an in-progress action

**Must.** Value: users can safely finish work after validation errors, conflicts or a lost response.
AC: (1) Existing mutations have visible entry, validation, confirmation where required, cancel-before-write, disabled duplicate submit, receipt/result reopening and non-secret draft preservation. (2) 412 requires explicit refresh/review, 409 preserves input, unknown outcomes retain the same key and recover via receipt/operation lookup. (3) Pending polling resumes after reload/reconnect; closing the browser is not cancellation; explicit cancellation obeys the durable decision. (4) Success waits authoritative completion and projection lag never triggers optimistic stock changes; no partial-result view for atomic endpoints.
Deps: B-01, B-05. Excludes: generic cancel of any business object or generic status override. Design: **M-R05/M-R08/M-R12/M-R13/M-F02/M-F04/M-I02/M-A02**; corresponding receive/sale/branch/submit/review/provider forms; `app.js`, `company-directory-bridge.js`, `partner-finance.js`, `finance-exchange.js`, `finance-documents.js`, `finance/workspace.js`, `insurance/workspace.js`, `insurance-exchange.js`, `admin/management.js`. **Pending/stale/unknown-outcome/permission/projection-lag gaps are design-gated per operation, not claimed as covered by those forms.**

### B-08 — Work in Russian and Uzbek consistently

**Must.** Value: RU- and UZ-speaking staff can understand the same business states and amounts.
AC: (1) User-facing labels, validation, notifications and state names use locale keys across all four apps; existing Russian terminology and party-specific finance labels remain intact. (2) Approved RU/UZ dictionaries, selection/persistence behavior and number/date/currency examples are required before locale release acceptance; absent copy is flagged, not silently machine-approved. (3) Locale changes never change currency, calculation inputs, stored IDs, decisions or snapshots; long labels and dialogs receive screenshot/keyboard checks.
Deps: B-05. Excludes: legal-document translation, automatic FX and invented Uzbek translations. Design: **M-R01/M-F01/M-I01/M-A01**, all four entry shells and downstream feature surfaces; `app.js`, `realization.js`, `finance/workspace.js`, `insurance/workspace.js`, `admin/admin.js`. **Locale control placement/UZ reference states are not established by current HTML and need a confirmed supplement.**

### B-09 — Attach and retrieve the exact safe document version

**Must.** Value: counterparties can trust which private file was submitted, scanned and reviewed.
AC: (1) Upload finalization fixes immutable generation/hash/length; scanner/binding/download all address those exact bytes. Overwriting staging never lets new bytes inherit an old clean verdict. (2) Unclean/rejected/scan-failed files cannot bind/download as accepted evidence; scanner outage fails closed and retry scans the same bytes; interrupted upload/expired capability has an explicit safe retry outcome. (3) Owner/party/draft permissions and sensitive-download MFA are rechecked; URLs expose no storage keys and grants are session/purpose-bound. (4) Replacement creates a new version, preserving previous authors/history; clean does not mean business accepted.
Deps: B-01, B-03, B-07. Excludes: signing, public file hosting, deletion of accepted versions. Design: **M-R03/M-R06/M-R08/M-R11/M-R14/M-F04/M-I02**; contextual attachments in purchase/vehicle/sale/invoice/insurance details and agreed finance → Documents; `app.js`, `vehicle-photos.js/css`, `finance-documents.js`, `finance-documents-domain.js`, `finance/history-stepper.js`, `insurance-ui.js`, `insurance/workspace.js`. Gates: scan interruption/rejection/version safety supplements; OD-10 real limits, consent, retention/storage configuration, not demo 1 MiB.

## B. Administration and company structure

### B-10 — Maintain seller company records without breaking history

**Must.** Value: Admin can create/find/update sellers while existing work keeps the same identity.
AC: (1) Registry/detail/create/edit retain immutable IDs and prior snapshots, memberships and transactions. (2) Country/registration duplicates reject; country supports free input and changing it clears region, which is disabled without country. (3) Seller creation grants only the explicit creator membership, no inferred capabilities or context switch.
Deps: B-03, B-05, B-07. Excludes: legal verification, inferred license/capability and provider first-admin credentials. Design: **M-A01/M-R12**; `/admin/` → Companies → add/row/edit; `/` → Settings → Companies and branches; `admin/admin.js/css`, `admin/management.js`, `platform-directory.js`, `company-fields.js/css`, `company-directory-bridge.js`.

### B-11 — Provision a provider and its first administrator atomically

**Must.** Value: a bank, MFO or insurer receives its own account without manual identity linking.
AC: (1) Add provider commits draft organization, new user, private credentials, global company-admin role and membership together, or none. (2) Existing login/email, invalid confirmation and duplicate registration reject without linking/resetting another identity. (3) Retrying secret-bearing provisioning returns the same outcome with no password/token in audit/events/receipt; first-admin invitation is not additionally required. (4) Explicit activation permits admitted entry to the correct independent empty workspace, not automatic programs, partnerships or product powers.
Deps: B-02, B-03, B-05, B-07, B-10. Excludes: production default password, actual bank/insurer API binding. Design: **M-A02/M-F01/M-I01**; `/admin/` → Integrations → MFO / Banks / Insurers → Add company → activation → linked `/finance/` or `/insurance/`; `admin/management.js`, `admin/admin.js/css`, `integration-registry.js`, `demo-accounts.js`, `integration-access.js`. Gates: production auth B-02; active workspace entry independent of OD-10 product policy.

### B-12 — Manage users and independent company memberships

**Must.** Value: Admin can grant or revoke a person's access without destroying unrelated company access.
AC: (1) User create/edit preserves global roles and membership rows separately; membership has branch access, never a role field. (2) Duplicate membership rejects; revoke/regrant are explicit revisioned actions; revocation invalidates only affected context. (3) User suspend/restore/session revoke require reason/permission/MFA; own-removal and last active MFA-enabled platform-admin protections hold under concurrent requests.
Deps: B-03, B-07, B-10. Excludes: unverified existing-user linking; invitation delivery implementation is B-02's gated subflow. Design: **M-A03**; `/admin/` → Users → add/detail → global roles/memberships/access actions; `admin/admin.js/css`, `admin/management.js`, `platform-directory.js`.

### B-13 — Maintain safe global roles and permissions

**Must.** Value: Admin can tailor global duties without accidentally creating unrestricted powers.
AC: (1) Custom role create/edit uses the versioned allowlist; unknown/unassignable keys reject and system roles remain protected. (2) Roles apply consistently across memberships, with company/branch/capability/party checks still independent. (3) Stale edits fail without overwrite; changes that remove the last active MFA-enabled Admin or own protected access are rejected atomically and audited.
Deps: B-03, B-07, B-12. Excludes: per-company role variants and financial impersonation. Design: **M-A04**; `/admin/` → Roles → role editor; `admin/admin.js/css`, `admin/management.js`, `platform-directory.js`.

### B-14 — Control operational access without claiming compliance

**Must.** Value: Admin sees whether an organization is operationally admitted without misleading licensing claims.
AC: (1) Reasoned draft→active preserves identity, history and prior records; activation does not verify compliance, enable API, publish programs or assign capabilities. (2) Normal active paths and historical record preservation are tested independently. (3) Suspend/restore exposure and disputed capability/compliance combinations stay unavailable until an approved operation matrix covers existing obligations and pending processes; then exact matrix cases are tested.
Deps: B-03, B-07, B-10, B-11. Excludes: automatic legal verification and clearing historical obligations. Design: **M-A01/M-A02**; `/admin/` → Companies or Integrations → company detail → access actions; `admin/management.js`, `platform-directory.js`, `integration-registry.js`, `integration-access.js`. Gates: **OD-05/11 affect disputed suspension/capability operations, not all provider or active-company implementation.**

### B-15 — Inspect a trustworthy, read-only audit trail

**Must.** Value: authorized administrators can explain who changed a record and why.
AC: (1) Scoped/filterable audit records actor/action/time/reason and safe before/after with stable pagination. (2) Passwords, tokens, MFA secrets, raw private documents and unauthorized PII never appear. (3) Audit access offers neither mutation nor partner decision/download authority; existing events remain after access changes.
Deps: B-01, B-03, B-05. Excludes: exceptional override console and full operational surveillance. Design: **M-A05**; `/admin/` → Audit → filtered table; `admin/admin.js/css`, `admin/management.js`, `platform-directory.js`.

### B-16 — Maintain company and branch details in the seller workspace

**Must.** Value: sellers organize their business without confusing record management and active context.
AC: (1) Settings edits preserve company/branch IDs; company fields obey country/region dependencies and duplicate rules. (2) Branch-only creation returns a committed branch; branch belongs to one company and ordinary detail edits reject active lifecycle fences. (3) Setup exposes only first missing company/branch prerequisite; B2B partnership requirement does not block all retail without OD-03 approval.
Deps: B-04, B-10. Excludes: hard branch deletion and automatic partnership setup for retail-only companies. Design: **M-R01/M-R12**; `/` → Dashboard setup / Settings → Companies and branches → add/edit branch; `app.js`, `company-directory-bridge.js`, `company-fields.js/css`, `realization.js`. Gates: OD-03 only the disputed universal partnership gate.

### B-17 — Link a branch warehouse with recoverable completion

**Must.** Value: sellers can create/link/detach their branch warehouse without losing stock or seeing false success.
AC: (1) A new or existing active branch can create/link a warehouse through one durable operation; maximum one primary per active branch is enforced. (2) Pending new branch/warehouse is not selectable as ready; success waits both owners' authoritative commit. (3) Precommit cancel/failed retry preserves retained branch and existing warehouse stock; after commit decision recovery completes, never falsely aborts. (4) Detach clears only the link, preserving warehouse/capacity/stock; attached branch inactivation requires completed detach.
Deps: B-07, B-16, B-18. Excludes: cascading deletion and timeout-based fence release. Design: **M-R12/M-R05**; `/` → Settings → Companies and branches → branch warehouse create/link; Warehouses → linked warehouse; `company-directory-bridge.js`, `company-fields.js/css`, `app.js`, `realization.js`. **Pending/failure/cancel/commit-in-progress presentation missing: design-gated supplement.** OD-05 disputed suspension during setup remains gated.

## C. One stock and vehicle truth

### B-18 — Manage warehouse capacity and inspect occupancy

**Must.** Value: stock staff can see available space and avoid impossible intake.
AC: (1) Warehouse create/profile edit preserves separate warehouse identity and optional branch link; capacity is a positive integer and cannot fall below occupied. (2) Occupied = placed identified units + active unidentified quantity; free = capacity minus occupied. (3) Scoped inventory lists separate unidentified batches from VehicleUnit rows and show no fabricated VIN; capacity changes require reason and reject attachment fences where applicable.
Deps: B-03, B-05, B-07. Excludes: zones/rows/bins, deleting occupied warehouses and legal ownership changes. Design: **M-R05**; `/` → Warehouses → add/detail/inventory/capacity controls; `app.js`, `realization.js`, `common.css`, `styles.css`, `realization.css`. Gates: any absent capacity-change confirmation needs captured/confirmed interaction before parity.

### B-19 — View consistent vehicle specifications and photo groups

**Must.** Value: sales and stock staff recognize the same vehicle from every relevant view.
AC: (1) Model/version uses nine distinct fields, dependent make→model→variant autocomplete/free entry, and independent exterior/interior colors; stale dependent references reject. (2) VIN is normalized, validated and globally unique without leaking another owner's details; specification editing does not reassign an existing VIN. (3) Warehouse/detail references address one VehicleUnit with separate owner/custodian/placement/customs/reservation facts and ordered exterior/interior clean photo versions. (4) Registry default coverage is implemented only after OD-02 resolution; supported owner query filters remain independent.
Deps: B-03, B-07, B-09. Excludes: used-car scope, national check-digit assumptions, mandatory milestone evidence inferred from gallery photos. Design: **M-R06/M-R05**; `/` → Vehicles or Warehouses → inventory row → details; receive/add modal; `app.js`, `realization.js`, `vehicle-fields.js/css`, `vehicle-photos.js/css`. Gate: OD-02 only default Vehicles projection, not canonical model/detail work.

### B-20 — Receive known VINs or a quantity awaiting VIN identification

**Must.** Value: stock staff record physical receipt once, even when VIN entry follows later.
AC: (1) Typed known-VIN receipt creates all units, placements, batch and occupied delta atomically; duplicate/invalid VIN or insufficient capacity rolls everything back. (2) Quantity-only receipt creates no fake units and contributes its full unidentified quantity to occupancy. (3) Concurrent capacity checks cannot overfill; an existing VIN uses authorized move/shipment receipt rather than recreation.
Deps: B-18, B-19. Excludes: shipment acceptance policy, arbitrary editing of confirmed quantity and partial endpoint success. Design: **M-R05/M-R06**; `/` → Warehouses → inventory → receive/add modal; `app.js`, `realization.js`, `vehicle-fields.js/css`, `vehicle-photos.js/css`. Gates: OD-06 applicable receipt evidence/photo requirements only; intake mechanics and synthetic tests independent.

### B-21 — Identify batches and correct receipt quantities with evidence

**Must.** Value: staff convert unidentified stock into usable VIN records without inflating inventory.
AC: (1) Each identified VIN reduces unidentified quantity by one and adds its one placement with zero occupied delta; batch and VIN updates are atomic. (2) Unknown VIN stock cannot be reserved, assigned, shipped or sold. (3) Reasoned correction is a separate audited delta; it cannot erase identified units or breach capacity. (4) Duplicate/model mismatch/too-many VINs reject the entire submitted identification command.
Deps: B-20. Excludes: arbitrary overwrite of confirmed quantity and fabricated factory VINs. Design: **M-R05/M-R06**; `/` → Warehouses → receipt batch/inventory → VIN entry; `app.js`, `realization.js`, `vehicle-fields.js/css`. Gates: OD-06 correction evidence/authority; missing correction/microstate UI requires supplement, not inferred from add-vehicle modal.

### B-22 — Move a vehicle between own warehouses safely

**Should.** Value: stock staff relocate a car without duplicating it or losing track of space.
AC: (1) Same-company move checks exact source placement and destination capacity in one inventory transaction with deterministic lock ordering. (2) Opposite concurrent moves avoid deadlock/overfill and one VIN retains one placement; identity/title remain unchanged. (3) Receipt/move history retains occurrence/evidence and retry returns the same result.
Deps: B-18, B-19, B-20. Excludes: intercompany sale/title transfer, route milestones. Design: **M-R05/M-R06**; `/` → Warehouses inventory or Vehicles → vehicle actions; `app.js`, `realization.js`. Gate: exact current move entry/states must be captured or supplemented; OD-06 applicable evidence policy.

## D. Partnership and wholesale procurement

### B-23 — Establish and end mutual partnerships

**Must.** Value: companies choose who may see commercial terms and initiate new trade.
AC: (1) Self/duplicate pair requests reject; only recipient accepts/declines and requester withdraws; decline/withdraw/end record reasons. (2) Active partnership permits authorized offers/new orders; before it only profile is visible. (3) Ending blocks new trade while preserving previous contractual records/documents.
Deps: B-03, B-07, B-10. Excludes: warehouse access/resale-before-buy policy. Design: **M-R02**; `/` → Partners → incoming/outgoing/active row actions; `app.js`, `realization.js`, `realization-domain.js`. Gate: OD-04 advanced partner access only.

### B-24 — Publish versioned supplier offers to selected partners

**Must.** Value: suppliers expose precise commercial terms to their intended buyers.
AC: (1) Draft/version/publish/withdraw retains immutable model/quantity/route/price/payment/warranty/service terms. (2) Audience is all-active or selected active partners; unauthorized counterparties cannot read prices/stock. (3) Publication reserves no VIN and changes do not rewrite captured order/quotation terms.
Deps: B-23, B-19. Excludes: retail listing mutation, implicit stock reservation. Design: **M-R07**; `/` → Offers → wholesale tab → add/edit/publish; `app.js`, `realization.js`, `realization-domain.js`. Gates: approved real money/configuration where applicable; no demo constants become commercial policy.

### B-25 — Browse and compare eligible procurement offers

**Should.** Value: buyers can compare partner products before deciding how to order.
AC: (1) Catalog, offer detail and compare use only audience/partnership-authorized published versions. (2) Warranty/service/route/prices shown correspond to the displayed version; filters/counts remain tenant-scoped. (3) Empty/unavailable/withdrawn offer states prevent new ordering but do not erase existing records.
Deps: B-24, B-05. Excludes: public marketplace, unconfirmed partner warehouse access. Design: **M-R04**; `/` → Purchases → Catalog → offer detail / compare; `app.js`, `realization.js`, `realization-domain.js`.

### B-26 — Agree a quotation and create one immutable purchase order

**Must.** Value: both trade parties can rely on the exact commercial terms they accepted.
AC: (1) Active partners create/send RFQ, exchange immutable numbered quotation versions and buyer accepts the exact current version/digest. (2) Quote acceptance and one corresponding order commit atomically; stale/concurrent acceptance cannot create duplicate orders. (3) Factory/foreign-direct/in-transit/local route terms remain distinct; local stock requires concrete verified vehicle facts and factory orders do not invent VINs.
Deps: B-23, B-24, B-07. Excludes: editing accepted terms in place, logistics policy or restoration of retired apps. Design: **M-R03/M-R04**; `/` → Purchases → current unified detail/catalog; `app.js`, `realization.js`, `realization-domain.js`. **OD-13: complete RFQ/quotation/factory interaction surfaces are missing; these are anchors only. Backend contracts/fixtures can proceed, FE requires user-confirmed unified supplement.**

### B-27 — Order directly from a published offer with supplier confirmation

**Must.** Value: buyers can use the existing simpler ordering journey without bypassing seller acceptance.
AC: (1) Direct order captures an eligible published offer version and starts awaiting-supplier. (2) Only supplier confirmation creates accepted status; creating the order grants no VIN allocation/execution. (3) Order detail shows separate terms/allocation/shipment/payment sources and contextual history/documents, not a generic editable status.
Deps: B-24, B-07. Excludes: replacing RFQ or treating demo full-payment-only rules as universal. Design: **M-R03/M-R04/M-R08**; `/` → Purchases → Catalog → order → purchase detail; supplier Sales → wholesale → order; `app.js`, `realization.js`, `realization-domain.js`.

### B-28 — Allocate identified vehicles without retail/wholesale double booking

**Must.** Value: an accepted wholesale order gets its exact stock or an explicit rejection.
AC: (1) Inventory reserves the complete concrete VIN/model set or none, against the accepted order's authorized holder. (2) Retail/wholesale races yield only one hold per VIN; unavailable/unidentified/mismatched units reject safely. (3) Commerce shows pending until authoritative grant; precommit cancel-before-acquire tombstones defeat delayed grants and no timeout expires holds.
Deps: B-19, B-20, B-07 and the common accepted-order contract from B-26 OR B-27 (not an all-of barrier). Acceptance/contract fixtures cover both order origins. Excludes: shipment/title transfer, partial allocation and automatic TTL. Design: **M-R03/M-R08/M-R06**; `/` → Purchases or Sales wholesale → accepted order → VIN allocation/detail; `app.js`, `realization.js`, `realization-domain.js`. Gates: OD-13 missing factory/assignment states and B-07 pending/cancel supplement; backend shared reservation can proceed.

### B-29 — Record shipment, customs and receipt facts by route

**Must.** Value: buyers and sellers know what physically happened before stock becomes eligible for delivery.
AC: (1) Shipments reference assigned concrete VINs and record typed milestone time/location/responsible/evidence under a named route policy; damage remains a separate blocker. (2) Local customs “not required” needs already confirmed facts; no standalone Import sidebar or arbitrary status dropdown. (3) Rejected receipt requires reason and creates no deliverable stock; accepted receipt waits atomic inventory placement/capacity confirmation before commerce completes.
Deps: B-28, B-20, B-09. Excludes: dispute module, universal photo rule and automatic title transfer. Design: **M-R03/M-R05/M-R06**; `/` → purchase detail → shipment/customs/receipt, warehouse/vehicle contextual actions; `app.js`, `realization.js`, `vehicle-photos.js/css`. Gates: **OD-06 route/evidence matrix; OD-13 missing full logistics and factory UI**. Typed fact/recovery tests are independent of unresolved transition activation.

### B-30 — Reconcile B2B invoices and partial external payments

**Must.** Value: buyers can submit payment proof and sellers can verify it without pretending the platform moves money.
AC: (1) Invoice schedule/amount/currency/recipient and partial claimed payments remain distinct immutable financial facts; no cross-currency summing. (2) Buyer submits exact clean evidence, seller financial permission/MFA accepts with confirmation or rejects with reason, preserving history and revision/idempotency guards. (3) Procurement/sales wholesale detail and supplier billing reflect originating commerce records; removed “From partners” tab does not return.
Deps: B-09, B-03 and the common accepted-order contract from B-26 OR B-27 (not an all-of barrier). Acceptance/contract fixtures cover both order origins. Excludes: funds transfer, invented invoice-creation endpoint, overpayment allocation and demo payment-gated handover policy. Design: **M-R03/M-R08/M-R11**; `/` → purchase or wholesale order → invoices/payment evidence; Bills and payments → Suppliers; `app.js`, `realization.js`, `realization-domain.js`. Gate: invoice issue/financial configuration not specified by current API must be contract-approved before dependent creation work; OD-04 affects fulfillment/cancellation, not proof review itself.

### B-31 — Amend or cancel orders without rewriting commitments

**Should.** Value: parties can correct an agreed order with mutual, auditable consent.
AC: (1) Proposed addendum is immutable/versioned with reason; other party accepts exact revision and original snapshot remains. (2) Inventory-impacting amendments and cancellation evaluate approved payment/reservation policy. (3) Cancellation becomes terminal only after exact-holder release; finalized physical handover cannot be undone by deleting events.
Deps: B-26, B-27, B-28, B-30. Excludes: unilateral commercial rewrites and inferred universal unpaid-only cancellation. Design: **M-R03/M-R08**; `/` → Purchases or Sales wholesale → order actions; `app.js`, `realization.js`, `realization-domain.js`. Gates: OD-04 amendment/paid-cancellation policy; absent addendum/conflict UI requires OD-13 supplement. Version storage/proposal mechanics independent.

### B-32 — Complete an authorized wholesale vehicle handover

**Must.** Value: the supplier can record physical delivery and both parties can see a trustworthy completed fulfillment.
AC: (1) The accepted order's exact VIN set/holder and route, payment and evidence prerequisites are checked against the approved fulfillment policy; no universal full-payment-only rule is copied from the demo. (2) Authorized supplier confirmation/MFA records a durable commerce fulfillment intent; inventory finalization commits physical facts/availability before commerce marks fulfillment completed. (3) Lost responses, reload and repeated delivery events recover the same result: no duplicate stock decrement, extra vehicle or premature success. Stale intent and cancellation after physical finalization cannot silently undo handover. (4) History retains actor/time/evidence and the order origin, covering quotation-based and direct orders; physical completion does not itself assert legal-title transfer.
Deps: B-28, B-30, B-09 and the approved route/physical-fact contract consumed by B-29; only the named prerequisites of the chosen policy apply. Excludes: legal ownership, real bank disbursement, insurance coverage, arbitrary completed-status editing and reversal by deleting history. Design: **M-R08/M-R03**; `/` → Sales → wholesale → order → handover/detail; `realization.js::wholesaleDetail`, `realization-domain.js`, shared `app.js`. Gates: OD-04/06 fulfillment/payment/evidence policy, OD-13 counterparty/route and pending states; the public commerce handover command is not yet specified and needs a narrow approved contract supplement before consumers. Existing private inventory finalization is not a substitute public API.

## E. CRM, retail and own installments

### B-33 — Maintain company-local customer records

**Must.** Value: sellers can reuse the correct customer in leads and sales without leaking contacts across companies.
AC: (1) Natural-person customer create/list/detail uses authorized company-local PII records. (2) Profile/version references are stable and scoped; another company's customer cannot be attached to a lead/deal. (3) Real PII release waits approved collection/consent/retention; synthetic data tests remain available.
Deps: B-03, B-05, B-07. Excludes: public customer account, cross-company deduplication and unspecified edit APIs. Design: **M-R10**; `/` → CRM → Customers → add/detail; `app.js`, `realization.js`, `realization-domain.js`. Gate: OD-10 live PII use, not synthetic CRM implementation.

### B-34 — Follow a lead and retain its contact history

**Must.** Value: sales staff know the next step and why an opportunity advanced or was lost.
AC: (1) New→contacted→qualified→test-drive→negotiation forward path preserves assignee/source/contact author/time/history. (2) Any nonterminal loss requires reason; won comes only from completed delivery, never a dropdown. (3) Website/Telegram/phone/manual remain source labels, not claimed integrations; backward/reopen/skip behavior stays gated.
Deps: B-33. Excludes: automatic channel ingestion and unapproved stage jumps/reopening. Design: **M-R10**; `/` → CRM → Leads → lead detail → contact/assign/stage; `app.js`, `realization.js`, `realization-domain.js`. Gate: scoped CRM backward/reopen decision only.

### B-35 — Assign and complete customer follow-up tasks

**Should.** Value: sales staff do not lose promised customer follow-ups.
AC: (1) Create task with customer, optional lead/deal, responsible user and due time after scope validation. (2) Open→completed is explicit/idempotent and retained in the relevant history. (3) List/detail filters and empty/error states preserve access and input.
Deps: B-33, B-34. Excludes: calendar synchronization, automated debt collection or uncontracted reminders. Design: **M-R10**; `/` → CRM → Tasks; lead/customer detail → next task/complete; `app.js`, `realization.js`, `realization-domain.js`.

### B-36 — Publish a retail listing for an existing vehicle

**Must.** Value: sellers present accurate sale offers without creating a second vehicle identity.
AC: (1) Listing references existing authorized VIN and own text/asking price; edits never alter vehicle identity/specifications/customs. (2) Publish/withdraw uses authoritative eligibility; permitted in-transit interest is not delivery authorization. (3) Listing remains consistent with completed sale withdrawal, not mere finance/insurance decisions.
Deps: B-19, B-03, B-07. Excludes: public marketplace/fifth app and a listing-based reservation. Design: **M-R07/M-R06**; `/` → Offers → retail tab → create/edit/publish, linked vehicle detail; `app.js`, `realization.js`, `realization-domain.js`.

### B-37 — Start one retail sale from a client or qualified lead

**Must.** Value: sales staff reserve the intended available VIN without double-selling it.
AC: (1) Manual/eligible-lead creation requires existing same-company client, accessible branch and concrete eligible VIN with payment scheme. (2) Pending local lead/vehicle claims and inventory reservation converge atomically to reserved+lead conversion, or release pending claims with no active sale on rejection. (3) Concurrent wholesale allocation cannot win the same VIN; duplicate/retried creation does not create a second sale; no lead is won yet.
Deps: B-33, B-34, B-19, B-07. Excludes: automatic delivery, financial approval and unapproved post-reservation cancellation. Design: **M-R08/M-R10**; `/` → Sales → retail → create/detail; CRM → qualified lead → create sale; `app.js`, `realization.js`, `realization-domain.js`. Gate: reservation-pending/reject/unknown/cancel UI supplement; shared reservation backend contract already specified.

### B-38 — Record retail contracts and verified external contributions

**Must.** Value: sellers can demonstrate the agreement and external vehicle-payment or own-installment first-contribution facts.
AC: (1) Reserved cash deal records external contract reference/date/exact clean files and an authorized vehicle-purpose invoice. (2) External proof is submitted then accepted/confirmed or rejected with reason by permitted financial staff; it is not a money transfer. (3) Own-installment progression retains insurer decision → first contribution → external contract as distinct steps, enabled only under approved own-installment/coverage policy; approval alone neither records a contribution nor creates a contract. (4) Contract, file scan, signature assertion and payment review remain different facts; billing “From clients” does not include client's bank/MFO repayment.
Deps: B-37, B-09; own-installment-specific portion additionally B-40, B-48 (not cash). Excludes: electronic signature service, partner-finance contracting, registration tariffs and installment allocation policy. Design: **M-R08/M-R11**; `/` → Sales → retail deal → contract/payment/first contribution; Bills and payments → From clients; `app.js`, `realization.js`, `realization-domain.js`. Gates: named real invoice/money configuration and required document/PII policy; OD-08/09 first-contribution progression only; do not enable OD-01 partner-finance contract flow.

### B-39 — Complete registration and physical vehicle delivery

**Must.** Value: sellers hand over only an eligible, properly completed sale and update stock once.
AC: (1) Approved checklist controls separate registration invoice recipient/currency/amount, proof and registration facts; demo GAI amounts are never authoritative. (2) Cash/own-installment prerequisites, customs/receipt/damage/evidence and exact reservation holder are verified under approved policy. (3) Durable finalize updates physical availability/placement once before retail marks delivered, withdraws listing and wins lead; legal owner does not change without a separately approved fact. (4) Insurance approval or finance agreement alone never permits handover.
Deps: B-37, B-38, B-20, B-09. Excludes: assumed tariffs, automatic legal title transfer and partner-finance funding/fulfillment. Design: **M-R08/M-R06/M-R11**; `/` → Sales → deal → registration invoice/proof/facts → delivery; linked vehicle and customer billing; `app.js`, `realization.js`, `realization-domain.js`. Gates: **OD-06/07; OD-08 only own-installment coverage linkage; OD-01 partner-finance fulfillment excluded until separately scoped**. Pending finalize/failure UI supplement and registration-fact API contract completion required.

### B-40 — Preview own-installment terms with a trustworthy schedule

**Must.** Value: seller and customer can inspect the whole proposed payment schedule before committing.
AC: (1) Fast preview and full monthly schedule use a distinct own-installment policy, not the partner-program calculator. (2) Authoritative accepted inputs/snapshot use versioned fixed-precision currency, date/rounding rules and exact schedule/final-balance invariants; backend recomputes and checks digest. (3) Without approved policy production command returns POLICY_UNRESOLVED; fixture annuity/whole-USD math is clearly nonproduction, not a fallback.
Deps: B-37, B-07. Excludes: bank product/legal classification, fees/penalties and automatic FX. Design: **M-R08/M-R09**; `/` → Sales → own-installment deal/calculator → schedule; `app.js` (`calculateInstallment`), `realization.js`. Gate: **OD-09** production calculation policy and exact persisted-calculation API; existing preview presentation can proceed using synthetic fixtures.

### B-41 — Service the seller's own installment after delivery

**Should.** Value: a delivered car's remaining seller receivable is not mistaken for a closed obligation.
AC: (1) Delivered own-installment contract remains in Sales → Installments with its immutable approved terms/schedule. (2) External payments, overdue contacts, payoff and explicit collection transfer preserve history and activate only under approved allocation/date/default/payoff policy. (3) No invented sanctions, calculation rounding or automatic collection; partner-finance customer payments never become seller receipts.
Deps: B-39, B-40, B-35. Excludes: bank/MFO debt servicing, automatic debits and legal collection execution. Design: **M-R09/M-R11**; `/` → Sales → Installments → contract/payment/contact/payoff/collection controls; Bills and payments → From clients; `app.js`, `realization.js`, `realization-domain.js`. Gates: **OD-09**, missing servicing endpoint/state contracts and any uncaptured microstates. This is policy-gated release scope, not silently enabled demo behavior.

## F. Financing — application, terms and document exchange

### B-42 — Maintain and publish a financier's program versions

**Must.** Value: each bank/MFO offers only its own controlled financing programs.
AC: (1) Authorized provider creates/edits draft versions, publishes or withdraws exact versions; creating its organization does not create a program/partnership. (2) Seller sees only published programs available to its company; withdrawal prevents new selection without rewriting historical submissions/terms. (3) Named currency/eligibility/limit/calculation schemas are validated; unapproved real-product configuration cannot masquerade as a production program.
Deps: B-11, B-03, B-07. Excludes: lending execution and demo examples as live offers. Design: **M-F03/M-R13**; `/finance/` → Programs → create/edit/publish/withdraw; seller Sales → financing action → provider → program; `finance/workspace.js/css`, `finance/demo-data.js`, `partner-finance.js`, `partner-finance-domain.js`, `finance-exchange.js`. Gate: OD-10 real products/configuration; drafts/version/state implementation and synthetic fixtures independent.

### B-43 — Prepare and explicitly send a financing application

**Must.** Value: seller sends the intended provider one reviewed snapshot, not an accidental draft.
AC: (1) Select bank/MFO→organization→available program; provider change clears old program/calculation. Draft remains editable and invisible to provider on list/detail/count/history/files. (2) Preview shows full schedule; authoritative versioned recomputation validates digest/currency/date/balance before acceptance. (3) Explicit confirmed send durably fences current reserved sale, freezes snapshot and appears submitted to provider only after exact retail admission receipt. (4) Cancel before commit, stale program, crash and lost reply recover without releasing a consumed admission or duplicating the application.
Deps: B-37, B-42, B-07. Excludes: first-payment routing, loan disbursement and a newly invented one-active-finance-app-per-deal constraint. Design: **M-R13/M-F02**; `/` → Sales → financing action → organization/program → preview → save/send; `/finance/` → Applications; `partner-finance.js`, `partner-finance-domain.js`, `finance-bridge.js`, `finance-exchange.js`, `finance/workspace.js`. Gates: OD-10 approved calculation/product/consent/PII; pending-submission supplement. Repeat/alternate-provider business flow is separately gated, not a reason to block initial submission or impose uniqueness.

### B-44 — Review financing information and agree exact terms

**Must.** Value: seller and addressed financier can negotiate one shared application transparently.
AC: (1) Addressed provider takes submitted→review, requests information with note or declines with reason; seller response with note/exact attachments returns needs-info→review. (2) Provider sends immutable calculated terms; seller sees “Terms received” and provider “Terms sent”; seller agrees exact current version with confirmation or counters back to review. (3) MFA/party/state/revision checks prevent impersonation or stale acceptance; program edits never rewrite snapshots and decline is terminal in this slice. (4) Agreed emits no payment, ownership, handover or debt-servicing action.
Deps: B-43, B-09. Excludes: customer digital signature and post-agreement money flow. Design: **M-F02/M-R13**; `/finance/` → Applications → detail → take/request/terms/decline; seller Sales → financing application → response/counter/agree; `finance/workspace.js/css`, `finance-exchange.js`, `partner-finance.js`, `partner-finance-domain.js`. Gates: OD-10 approved terms calculation/real data; **OD-01 does not block this state machine or document exchange**.

### B-45 — Request, revise and accept post-agreement documents

**Must.** Value: both parties can complete the documented exchange with exact version accountability.
AC: (1) Only agreed application permits provider document request; seller submits exact clean binding requested/changes→review. (2) Provider accepts exact submission version with confirmation or returns with mandatory note; resubmission retains all prior versions/authors/history. (3) Reasoned cancel is limited to requested/changes; review-cancellation awaits its explicit decision. (4) Accepted document does not imply contract signature, vehicle purchase, funding or delivery; foreign/draft downloads remain denied.
Deps: B-44, B-09. Excludes: automatic electronic signature, legal template generation and cancel-during-review without approval. Design: **M-F04/M-R13**; Financing → agreed application → Documents on financier/seller side; `finance-documents.js`, `finance-documents-domain.js`, `finance/history-stepper.js`, `finance/workspace.js`, `partner-finance.js`. Gates: scan/version microstate supplement; OD-10 approved document policy; no broad OD-01 blocker.

### B-46 — Inspect financier workload and existing account views

**Should.** Value: financier staff can find their addressed work and understand their account context.
AC: (1) Overview/application counts and navigation derive from the same scoped financing owner and exclude seller drafts/other providers. (2) Existing Partners and Settings presentation shows authorized current records; empty state is genuine for newly provisioned companies. (3) No demo provider impersonation or fabricated management/security control is carried into production.
Deps: B-05, B-11, B-42, B-43. Excludes: implied partner-management/security settings CRUD or disbursement dashboard. Design: **M-F01/M-F02/M-F05**; `/finance/` → Overview / Applications / Partners / Settings; `finance/workspace.js/css`, `finance/demo-data.js`, shared `finance-exchange.js`. Gates: uncaptured states only; admitted empty workspace is not gated on financial policy.

## G. Own-installment insurance — through underwriting decision

### B-47 — Draft and send an own-installment insurance request

**Must.** Value: a seller asks the selected insurer to review the correct undelivered installment sale.
AC: (1) Only own-company, own-installment, undelivered sale is eligible; one company+sale application across all states is enforced. (2) Save draft and explicit confirmed send are separate; insurer cannot see drafts anywhere. (3) Submission uses exact reserved-sale snapshot/admission protocol; later information is additive/versioned, not snapshot mutation. (4) Pending/cancel/crash recovery preserves the source-sale fence and never invents coverage.
Deps: B-37, B-11, B-07; B-40 only if the approved submission snapshot requires a persisted installment calculation, not as a blanket dependency for draft/review mechanics. Excludes: cash insurance, general auto insurance and bank/MFO insurance requirements. Design: **M-R14/M-I01**; `/` → Insurance → create/save/send → request detail; `/insurance/` → Applications; `insurance-ui.js`, `insurance-bridge.js`, `insurance-exchange.js`, `insurance.css`, `insurance/workspace.js`. Gates: OD-10 approved live data/consent and any referenced production installment calculation; pending submission supplement. **OD-08 does not block draft/review mechanics.**

### B-48 — Exchange underwriting information and issue a decision

**Must.** Value: seller and assigned insurer share a clear, attributable underwriting result.
AC: (1) Only addressed insurer takes submitted→review; review may request information or approve/decline. (2) Request, seller response and insurer decision require notes; response returns to review with preserved versioned evidence/history. (3) Decision requires permission/MFA and exact current revision; seller cannot decide for insurer. (4) Approved/declined are terminal here and produce no policy, premium, payment, coverage or handover effects.
Deps: B-47, B-09. Excludes: insurer policy issuance, claims and automatic advancement of seller delivery. Design: **M-I02/M-R14**; `/insurance/` → Applications → detail → take/request/approve/decline; seller `/` → Insurance → detail → response/history; `insurance/workspace.js`, `insurance-ui.js`, `insurance-exchange.js`. Gates: OD-10 live document/PII policy; **OD-08 only downstream coverage/fulfillment linkage, not underwriting decision implementation**.

### B-49 — Inspect insurer queue and account summary

**Should.** Value: insurer staff can prioritize their own requests in the existing dedicated workspace.
AC: (1) Overview and application list reflect addressed submitted/review/result states and never drafts/other companies. (2) Filters/counts, empty/loading/error handling and detail navigation use owner data and existing distinct insurer layout. (3) Settings remains the existing authorized account summary, with no inferred product/policy-management controls.
Deps: B-05, B-11, B-47, B-48. Excludes: policy administration, account security redesign and insurance pricing. Design: **M-I01/M-I03**; `/insurance/` → Overview / Applications / Settings; `insurance/workspace.js`, `insurance.css`, `insurance-exchange.js`. Gates: missing production boundary states only; empty account summary can precede underwriting policy decisions.

## Icebox — deliberately outside this release

### B-50 — Public customer discovery and self-service

**Won't (this release).** Value: customers could discover stock and manage their own requests independently.
Future AC: approved customer identity/audience, navigation, listing visibility and consent contract; authorized customer actions operate on existing seller entities without a second stock truth.
Deps: B-33, B-36. Excludes now: fifth app/public marketplace/customer cabinet; existing CRM Customer stays in scope. Design: **none — no current M-ID or HTML entry; new app/interaction design required**, not M-R10. Gate: OD-14 explicit release expansion.

### B-51 — Execute partner funding and insurance coverage lifecycles

**Won't (this release).** Value: counterparties could progress beyond information exchange into agreed regulated execution.
Future AC: separately approved acquisition/down-payment/disbursement/signature/delivery responsibilities and insurance policy/premium/coverage/claims contracts; negative tests prevent mere agreement/approval from authorizing them.
Deps: B-44, B-45, B-48. Excludes now: actual bank money movement/debt servicing, insurance issuance/claims and real external API integration. Design: **none for these downstream operations — M-R13/M-F02/M-I02 stop at exchange/decision; no valid current HTML entry**. Gates: OD-01/08/10, explicit scope and new interaction contract. B-39's own-installment handover policy remains a separate gated release item, not automatically deferred by this Icebox entry.

### B-52 — Expand platform oversight and exceptional operations

**Won't (this release).** Value: future platform operators could manage explicitly approved references and exceptional cases.
Future AC: approved references/settings/oversight catalog, exact exceptional-action authority, mandatory reason/MFA/audit and counterparty safeguards; no unrestricted Admin substitute for partner decisions.
Deps: B-13, B-15. Excludes now: broad references/settings console, exceptional overrides, disputes and forced stock release. Design: **none for proposed new operations — M-A05 is read-only audit, not an override reference; new navigation/states needed**. Gate: OD-14 and explicit authority contract.

## PO checkpoint / handoff to PM

Proposed scope: 49 release items across seven value areas (42 Must, 7 Should); three Won't items in Icebox. All dependencies are B-IDs and no task files/branches/worktrees are created here.

Before task readiness, PM must distinguish: (a) buildable approved mechanics/current HTML states using synthetic fixtures, (b) affected FE states awaiting captured/confirmed supplement, (c) production policy/security/live-data gates, and (d) explicitly deferred Icebox. It must not turn all finance/insurance, all stock, or all active-company work into one blocked item.

Decision register carried forward: OD-02 registry default; OD-03 universal setup partnership; OD-04 advanced partnership/resale/amend/cancel; OD-05/11 suspended/capability matrix; OD-06 route/evidence/correction; OD-07 registration/fulfillment; OD-08 own-installment coverage linkage; OD-09 own-money/servicing; OD-10 real product/currency/configuration/PII/consent/document/storage/API; OD-12 security recovery/invitations; OD-13 unified RFQ/factory/logistics and state coverage; OD-14 expanded release scope. OD-15 architectural choices are approved; exact versions remain a pre-scaffold lock, not evidence these business gates were answered. Scoped extra decisions: finance repeat/alternate-provider behavior, cancellation of finance documents during review, CRM backward/reopen/skip transitions, approved RU/UZ copy and locale interactions.

Contract gaps to close narrowly before consumers: B2B invoice issuing, wholesale handover and seller-originated wholesale entry, retail registration-fact recording, persisted own-installment calculation/servicing, and any currently presentation-only settings/capacity/move/correction controls lacking an observed entry. Do not fabricate endpoints or reuse a nearby HTML modal as proof.

Proposed approval question: approve/amend these release priorities and exclusions so PM can decompose them, preserving the listed policy/design/security/Git gates. Backlog approval is not approval of unresolved policy values.

## User decisions (confirmed)

Architecture approval is recorded separately in `../state/architecture-approval.md`.
2026-09-13: after the 42 Must / 7 Should / 3 deferred scope and exclusions were
presented, the user asked "anything else?". The coordinator explained that scope
approval precedes task generation, Git setup and scoped follow-up decisions. The
user then directed **"conmtinue"**. Coordinator treats that direction as approval
to proceed with the presented scope and priorities into PM planning; no scope
corrections were requested.

RU/UZ is included in the planning envelope; translations and locale UI still
need their stated acceptance. All unresolved monetary, insurance, evidence,
security and missing-UI questions retain their gates. No production rules,
application changes, Git initialization or deployment were approved by this step.
See `../state/backlog-approval.md` for the exact baseline fingerprint. Review
evidence and original draft-ID mapping remain in `../state/backlog-review.md`.
