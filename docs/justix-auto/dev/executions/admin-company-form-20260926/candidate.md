# Frozen candidate

Primary reserved read-only source to independent reviewer `/root/company_form_code_review`
and QA `/root/company_form_qa`; QA alone holds the browser/local frontend fixture.
No implementation writer, branch switch, backend mutation or deployment.

Version-control actor `/root/company_form_git` returned the following receipt
verbatim on 2026-09-26:

Candidate complete.

- Final SHA: `db055e46d3d2d022f6eddb1a72d563106a5de384`
- Branch: `fix/country-region-combobox`
- Cumulative range: `eca22f60f9082f7fe59004c81be63170ebfe1483..db055e46d3d2d022f6eddb1a72d563106a5de384`
- Prior commit: `fce20e65dd7f1b45a2a0bac8d86c67ea5d70cf10`

H1 commit paths:

- `lefthook.yml`
- `package-lock.json`
- `web/packages/kit/package.json`
- `web/packages/kit/src/design.css`

Hooks passed: Prettier, ESLint with corrected ignored-file filtering, and conventional-commit.

The cumulative candidate contains the expected 13 application/tooling paths. The index is clean; only intended docs/task records and backups remain unstaged/untracked. No push or branch switch occurred.

## Checkpoint reconciliation

The original planned single commit became two normal-hook commits: nine TS/TSX
paths first, then CSS/package metadata and reviewed H1. Both are one coherent
task on one branch; the complete range above is the review/QA identity. The
initial split alone did not resolve the nested hook glob issue. H1 was approved
by the infrastructure hub and independent plan reviewer, implemented by the
worker, tested with positive/negative controls, then committed with all hooks.
No bypass, relaxed lint threshold, main/dev integration or hook conflict resolution.

Canonical records and recoverable backups intentionally remain outside the
application candidate while the exact-SHA review and QA run. Seven protected
precursor files retain their entry snapshot hashes. Usage data unavailable from
delegation tools is unknown, not zero.
