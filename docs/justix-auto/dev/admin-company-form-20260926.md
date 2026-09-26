# Admin company onboarding form — user-approved correction

Date: 2026-09-26. Coordinator: primary application hub.
Status: qa-green at source `f9c4f3564afb195b48cba2e66c9be3e4d00d2948`;
human dev integration remains pending.
Canonical association: T-426 / B-11 admin provisioning presentation; existing
implementation is the ADR-14 monolith, so historical T-426 paths/dependencies do
not describe this correction. This does not close T-426 or B-11.

## Latest user decision

The screenshot shows an unclear combined company/first-user form. The user
requested one company name instead of separate display/legal names, removal of
the confusing duplicate email entry, sections for information, address, contacts
and login, and clearer wording. This explicitly authorizes the UI correction.

## Bounded acceptance

- One visible company-name field and one visible email field during creation.
- Four clearly labelled groups: company information, address, contacts and sign-in.
- Administrator name remains explicit; explain that the contact email is also
  used for the first administrator during creation. Map the one email to both
  existing API fields without changing backend identity policy.
- Clear Russian field/dialog/action labels; retain password requirements and
  confirmation. Preserve draft/activation behavior and all company kinds.
- Preserve current country/region combobox work, arbitrary-country support and
  dependency reset. Keep errors next to the appropriate visible fields and retain
  entered values after failure.
- Existing company edits must not silently erase a previously stored legal name.
  No backend/schema change, account merging, deployment or dev/main integration.
- Grouped forms remain usable on desktop/mobile; ungrouped shared forms retain
  their existing behavior.

## Git and execution provenance

Own-root guard passed. HEAD: eca22f6, branch fix/country-region-combobox,
tracking origin/dev. Entry tree contains uncommitted country/region changes in
admin, realization and shared kit, including manifest/lock/test changes. Preserve
all of them. Version control must reconcile a safe checkpoint/branch strategy
before mutation; never discard, silently stage, or switch away from dirty work.
Coordinator reconciliation: continue the existing branch as one cumulative
company-form correction, incorporating its already-present geographic input
precursor. No branch switch, stash, worktree or dev write. Version control will
commit the cumulative precursor and correction together; review and final QA cover the complete
eca22f60f9082f7fe59004c81be63170ebfe1483..candidate range. Implementation ownership
is narrower than cumulative review ownership. Six protected precursor leaves
remain byte-for-byte preserved; four country-label test selectors were later
explicitly adopted and reviewed. This avoids concealing pre-existing changes
from QA. See the subsequent cross-session provenance reconciliation below.
Entry source and canonical snapshots plus SHA-256 inventory are preserved under
state/backups/admin-company-form-20260926-entry/ (manifest SHA-256
697477666c6089f28ea29191d28d8d86d00ceb33db55186369d29c5319730fdf).

Source contracts: business-logic.md §§3,9,11 and current identity service company
input/provisioning behavior. OD-01/06/12 are unaffected by this presentation fix.

Execution artifacts: dev/executions/admin-company-form-20260926/.
Only the coordinator changes this canonical record and task-board/dev-state.

## Execution checkpoint

Independent plan review GREEN; accepted manifest digest:
`cfc12959fd8d67038b9b3d4f18b9b8d16197d47797805dc416450749bb4473a3`.
Source changes are committed on the existing fix branch. Complete cumulative
review covers `eca22f60f9082f7fe59004c81be63170ebfe1483..f9c4f3564afb195b48cba2e66c9be3e4d00d2948`.
Independent [R2 review](executions/admin-company-form-20260926/code-review-r2.md)
and [final CSS review](executions/admin-company-form-20260926/code-review-r3.md)
are GREEN. A first review caught duplicate backend-email error routing; its
repair is covered by rendered tests and final browser checks. A desktop visual
check caught mismatched password-row alignment; the final three-line CSS repair
was independently reviewed and verified on desktop and mobile.

[Q1 is GREEN](executions/admin-company-form-20260926/qa-result.md). Ten selected
test files / 28 tests, all-workspace typecheck, scoped lint and hook controls
passed at `e8709bb`. All non-CSS source is byte-identical at `f9c4f356`; fresh
final-SHA browser integration and four-app builds passed after the CSS repair.
Browser checks cover one-name/one-email payloads, geo dependencies, pending-submit
protection, validation/error retention, legacy legal-name preservation and exact
If-Match edits, separate activation, mobile layout and footer reachability.
All four company kinds have exact payload evidence. Browser traffic used
synthetic API interception; this does not certify unchanged backend persistence,
authorization or transactionality. Evidence, screenshots and the reproducible
harness are in the execution directory. Browser/Vite leases are released.

The staged-lint H1 correction was approved by DevOps and independent review;
normal commit hooks passed, with positive/negative controls proving actual
TypeScript errors still fail. No hook bypass was used. The completed React
Doctor run scored 65/100 with 30 warnings; its bounded assessment records no
blocking scoped finding and does not replace behavioral QA.

The separate session's `dev/results/FIX-GEO-1.md` reports authorship of the
geographic precursor and the late selector edit, then cessation after detecting
the shared-checkout collision. That report is retained unmodified and is not
owned or closed by this task. [Reconciliation](executions/admin-company-form-20260926/test-drift-reconciliation.md)
records the initial coordination gap, explicit precursor adoption, six preserved
entry hashes plus adopted combobox hash, and complete cumulative review/QA.
No application drift occurred during final verification.

The tested dev base remains `eca22f60` (remote read verified before publication).
Documentation-only publication must preserve the entire application/tooling
tree from `f9c4f356`. The bounded correction is ready for task-branch handoff;
no dev/main integration, deployment, or broad T-426/B-11 completion is claimed.
Canonical pre-update snapshots and hashes are under
`state/backups/admin-company-form-20260926-final/`.
