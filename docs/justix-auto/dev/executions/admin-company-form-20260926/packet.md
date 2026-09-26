# Admin company form correction — P1

Planning checkpoint; not dispatchable until independent review and hub acceptance.

- Task: `admin-company-form-20260926`, associated with T-426 / B-11 without closing either.
- Authority: primary application hub; approval: `docs/justix-auto/dev/admin-company-form-20260926.md`.
- Coordinator-confirmed base: `eca22f60f9082f7fe59004c81be63170ebfe1483` (origin/dev); continue the same `fix/country-region-combobox` branch. No branch switch/worktree. One cumulative final commit; entry snapshots pin precursor provenance. Execution candidate remains pending.
- Owned plan artifacts: this file and `acceptance.md` in this directory only.
- Read-only contracts: `docs/justix-auto/business-logic.md` §§3, 9, 11; `internal/modules/identity/service/company.go` input/apply/provision/update.

## Intermediate source map

`web/apps/admin/src/pages.tsx` owns companyFields/companyInput/adminFields/firstAdmin, both create entry points and CompanyDialog edit. Creation currently exposes two company-name and two email inputs. `web/packages/kit/src/ui.tsx` owns FieldSpec, FormDialog and server-error suffix matching. ActionButton actually lives in `web/packages/kit/src/shell.tsx`, not actions.tsx. Grouping must preserve the flat collection/submission/dependency behavior. Service apply replaces legalName on edit; omitting it would erase a legacy value. Both company.email and firstAdmin.email can already match the one visible email suffix. Current country/region implementation and its tests/manifests are dirty precursor work and must be preserved.

The mapping above was persisted before additional targeted reads. No application edits or Git operations performed by slicer.

## Dispatch gates and dependencies

P1 has no other implementation packet dependency. It depends on the user-approved correction and preservation of the country/region precursor. Independent plan review, then the primary hub's acceptance of the two-file manifest digest, precede worker dispatch. The coordinator explicitly selected the existing branch above because this is the same company-form correction extended by the latest request. The precursor checkpoint failed pre-commit lint (FieldInput complexity 27 exceeds 20), without source mutation; HEAD remains the base. Version_control unstages its ten files preserving bytes. Worker starts from that documented dirty source, with no clean-tree prerequisite. Entry snapshots/hashes pin provenance; version_control creates one cumulative final commit after checks pass. Final review/QA cover the full base..candidate range on clean frozen application source. Do not stash/discard precursor work, switch branches or require unrelated process shutdown. Serialize checkout writes and freeze source for review/QA. Source drift after the hashes below requires targeted reconciliation before dispatch.

Exact ten-file precursor inventory (recoverable snapshot and SHA-256 values: `docs/justix-auto/state/backups/admin-company-form-20260926-entry/sha256.json`): `package-lock.json`, `web/apps/admin/src/pages.tsx`, `web/apps/realization/src/data.ts`, `web/packages/kit/package.json`, `web/packages/kit/src/design.css`, `web/packages/kit/src/index.ts`, `web/packages/kit/src/ui.tsx`, `web/packages/kit/src/combobox.test.ts`, `web/packages/kit/src/geo.test.ts`, `web/packages/kit/src/geo.ts`. All ten belong in cumulative review/QA scope. Preserve the snapshot bytes for the seven paths outside overlapping P1 production ownership; preserve precursor behavior/hunks in pages.tsx, ui.tsx and design.css. The six exact P1 paths below plus this inventory are the complete allowed cumulative application/test diff (13 distinct paths); canonical/planning artifacts are attributed separately by the hub. Original broad T-426 stays open; this correction makes no production-readiness guarantee.

Sequence: one worker → version_control frozen candidate → one independent reviewer for the exact complete correction range → one independent executable QA packet Q1. The reviewer capability also supplies pre-dispatch plan review. The same unchanged candidate and tested base may serve Q1 and final shared-interface coverage; no redundant integration packet is needed. A later candidate/base change invalidates affected evidence. Hub/coverage audit records exclusions and acceptance, not a new canonical backlog item.

## P1 — present one understandable company onboarding form

Behavior: Admin creates any company kind through one company-name and one email input, with accessible sections, correct API mapping and safe existing-company edits.

Worker: `gpt-5.6-terra / medium`, fresh context. Context budget: 12,000 input tokens for this packet, acceptance manifest, scoped rules and targeted source slices; do not load whole board/history/mocks. Read root AGENTS.md, web/AGENTS.md, dev/agent-workflow.md, approval record, business-logic §§3/9/11 only, then the mapped symbols. The Go file is a read-only contract reference. OD-01/06/12 are unaffected; no decision is made about financing, photo rules or production identity.

Exact allowed application/test paths (no directory wildcards):

1. `web/apps/admin/src/pages.tsx` — company fields, mapping, creation buttons and CompanyDialog editing only.
2. `web/packages/kit/src/ui.tsx` — additive grouping support in FieldSpec/FormDialog and locally scoped styles if appropriate; focused extraction of combobox rendering/helper to satisfy existing FieldInput complexity limit, preserving geography behavior and state dependencies.
3. `web/packages/kit/src/shell.tsx` — ActionButton forwards optional grouping metadata only if the chosen API needs it.
4. `web/packages/kit/src/design.css` — optional minimal scoped section/mobile styles; reuse existing tokens/classes, avoid changing ungrouped layouts.
5. `web/apps/admin/src/company-form.test.tsx` — new behavior tests against actual UI submissions and edit retention.
6. `web/packages/kit/src/form-dialog.test.ts` — new grouping/error/submission and ungrouped regression tests, with jsdom annotation and existing renderer tooling.

Worker result: `docs/justix-auto/dev/executions/admin-company-form-20260926/worker-result.md`. Existing precursor tests, geography source, manifests/lock and realization files are read-only. No backend, schema, canonical record, fixture policy or dependency changes. No deployment or dev/main integration. Before code changes run `bash tools/check-git.sh` and require this exact project root. Use apply_patch and preserve user changes. Root/web scoped rules govern modal patterns, keyboard access, all UI states and locked tooling. After React changes use the available `/Users/bakhromachilov/.agents/skills/react-doctor/SKILL.md`; record findings/execution limitations without adding dependencies or broad unrelated repairs.

Implementation acceptance (local requirement IDs, not new backlog tasks):

| ID | Required behavior |
| --- | --- |
| CF-01 | Creation has one visible `Название компании` and one visible `Электронная почта`; no legalName/adminEmail inputs. Administrator name stays separate. Preserve required markers and field types. |
| CF-02 | Four semantic `fieldset`/`legend` sections on creation, ordered: `Информация о компании` (name, registration), `Адрес` (country, region, address), `Контакты` (email, phone), `Данные для входа` (administrator name, login, password, confirmation). Use native group semantics, labelled controls, predictable keyboard order and no nested forms. Grouping is optional metadata on the existing flat fields/submission model. |
| CF-03 | One entered email feeds both `company.email` and `firstAdmin.email` on both create endpoints; remove adminEmail mapping. A short creation-only explanation states that the contact email is also used by the first administrator. No account linking or changed identity policy. New-company legalName remains optional/empty, as previously when left blank; do not silently copy a company display name into legalName. |
| CF-04 | All seller/bank/mfo/insurance paths preserve endpoint, provider kind, required administrator name/login/password/confirmation, current minimum 12-character guidance (may move into hint), draft response handling, query refresh and separate activation with reason. Use clear dialog/action wording (e.g. `Новая компания и администратор`, `Создать компанию`), with correct provider context. Creation does not switch active company. |
| CF-05 | Edit has one name field; serialize the original `c.legalName` from the fetched-company closure, even if visible name changes. Preserve empty legalName too. Submit company requisites only, same ID and revision/If-Match; changing company email must not update administrator/user email. Edit uses the three applicable company sections; do not add credential inputs or an empty sign-in fieldset. |
| CF-06 | Keep country/region combobox options, free-text countries, canonicalization and dependent region clearing/disabling intact, including unknown country values. Group wrappers cannot reset values or break dependencies. |
| CF-07 | Backend field errors for `company.email` and `firstAdmin.email` appear by the same visible email input; both remain represented if returned together. Other nested field errors route to matching visible controls. Unmapped/hidden-field errors remain in the form notice. Validation/conflict/network/stale failures preserve entered values and keep the dialog open. Success closes/refreshes; cancel does not write; pending submit prevents repeated command dispatch. |
| CF-08 | Sections, help, errors and actions remain usable at desktop and narrow mobile widths; avoid overflow, clipped inputs or inaccessible footer. Existing ungrouped forms keep layout/order, values, error mapping, serialization and confirmation-only behavior. |

Keep error changes minimal. Existing suffix matching already recognizes both email paths once only email is visible; verify before adding an alias API. If two API keys map to one control, retain their messages rather than overwriting one. Hidden legalName errors remain visible in the general notice; do not expose a new legal-name editor. Reuse shared current UI patterns and tokens. The latest screenshot/user correction supersedes the historical flat mock layout; fixtures do not define policy.

Required worker checks and independent coverage are defined in acceptance.md. Tests must exercise rendered controls and outgoing requests, not snapshots that merely assert Russian strings or reimplement payload helpers. Use existing tooling; no dependency installation/change is part of P1.

Resource locks: primary reserves the sole application writer for P1; no concurrent checkout mutation/branch switch. Worker unit/type/build runs need no browser or live database. Reviewer/Q1 require immutable candidate source with no writer. Q1 exclusively holds the browser and its named local test fixture; share no live fixture with another job. Version_control alone performs Git operations after leases are released.

Stop conditions: missing base/snapshot provenance/review/hub acceptance; unexpected precursor/source drift; required change outside owned paths; undecided product rule; backend/API/schema/policy change required; inability to demonstrate identity/data retention; required QA unavailable. Report the specific gap to the hub. Do not manufacture GREEN or infer policy from demos. If the coherent packet cannot fit the context budget, checkpoint findings and request a narrower slice; use `RESLICE_REQUIRED` if canonical task scope would change. After two unsuccessful repairs, stop for diagnosis/reslicing under hub authority.

## Source fingerprints

SHA-256 of working-tree bytes at slicing time, including dirty precursor content; these do not certify a commit. Business-logic hash covers the file for drift detection, while authorized content reads are scoped to §§3/9/11. Reconcile relevant drift, do not overwrite it.

```text
a80c252b967009767c248c59b2671362ce241f1715be4857d070fc0f1577b3ee  AGENTS.md
b8dee411a0a1043a038d72ccde611f5d12e70a6065a3d60eb64fde72600bdd02  web/AGENTS.md
b2d5153012dfec17e20a5e55a65af858d693cae27adb66ef6a5c440599149b27  docs/justix-auto/dev/AGENTS.md
dcc8c4f9f33490b83a65b9b0a7d2ed51a7f381ae7305743b5620735d53bdc3aa  docs/justix-auto/dev/agent-workflow.md
c229bb0be4dd8356329e7067828f9bc42ff8f74b0a0516f728ac129b21f7a325  .codex/agents/task_slicer.toml
b1b5f302a1ea5938a716ee1b0e2753d25acfe1542b06ab0c782b4d644c1d109c  docs/justix-auto/dev/admin-company-form-20260926.md
e0c3f33db3246c63dc205bf7036038bf0771e78245241d9c7a9754d4f7e13a66  docs/justix-auto/business-logic.md
467469a3a569a3bc009da3584a4dc63fe4072ebc44d0ccc2b90581e8958ea631  docs/justix-auto/open-decisions.md
4b3e32af2e2a232fdef5f206db199304e818742eac1dee3f23019f54fd868ff5  web/apps/admin/src/pages.tsx
10186b2aa3ea88537ee16937dd3fec95d3b8234b8098d058d666a59f16d4b603  web/packages/kit/src/ui.tsx
aa077655a438a044bf382e50fc6815a16b16da62b0193548cd3191cff8127f9e  web/packages/kit/src/shell.tsx
49ee83df1a053573fbea8d205ab60ae8ae6833a26021fbccef0b6e1508019f4c  web/packages/kit/src/design.css
f2ea9d30ab48c20b0d34342df2b2f850c96f45c26594565fa053c3f236b38749  web/packages/kit/src/combobox.test.ts
2b7cfc9bf1a10de10bee6077a4773eab3323f3970d9a8f765b273ecb1c4b4062  web/packages/kit/src/geo.ts
1f76e1ef90b762f761090985647c03edeab6dc7e423f7cb7dd7e94915b3d3281  internal/modules/identity/service/company.go
0dd8bd11a3694a579fefc9425fca6c7a40abd02ff50d124a7ad34c8a17addc00  package.json
d82fffa16072ebc06b37c8f5fee740c3185e234b5b4ccadd2e3e1dd95d383abf  package-lock.json
44d5556f1b42e88bb9aa075b478564a26be1b92fff34197dbceacac74861b7fe  web/apps/admin/package.json
5cadd4b05265b5e76067b88eeb5f5368fa02ae5e4d624c54af890ae98589331a  web/packages/kit/package.json
cfd1d7d11366878f721fe86d6393a44bc8c7656a760aa3142d2bd633cdcb6d59  web/vitest.workspace.ts
fa8b0b3b14192a3e80a1b279edccd6ba51881c7ee3681db368510c58933a3abb  web/apps/admin/vitest.config.ts
86383a384da55cc7673002878b8496725d6ab57f6af3aac4eafa40f76a6d9ff2  web/packages/kit/vitest.config.ts
```
