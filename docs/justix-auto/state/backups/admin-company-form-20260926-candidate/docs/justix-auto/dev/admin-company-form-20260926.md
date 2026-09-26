# Admin company onboarding form — user-approved correction

Date: 2026-09-26. Coordinator: primary application hub.
Status: approved for bounded implementation; plan/review/QA pending.
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
checkpoint the exact precursor separately; review and final QA cover the complete
eca22f60f9082f7fe59004c81be63170ebfe1483..candidate range. Implementation ownership
is narrower than cumulative review ownership; existing unrelated leaves remain
byte-for-byte preserved. This avoids concealing pre-existing changes from QA.
Entry source and canonical snapshots plus SHA-256 inventory are preserved under
state/backups/admin-company-form-20260926-entry/ (manifest SHA-256
697477666c6089f28ea29191d28d8d86d00ceb33db55186369d29c5319730fdf).

Source contracts: business-logic.md §§3,9,11 and current identity service company
input/provisioning behavior. OD-01/06/12 are unaffected by this presentation fix.

Execution artifacts: dev/executions/admin-company-form-20260926/.
Only the coordinator changes this canonical record and task-board/dev-state.
