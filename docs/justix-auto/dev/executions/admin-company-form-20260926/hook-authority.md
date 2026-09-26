# Infrastructure authority receipt

Actor/run: `/root/company_form_hook_authority`, 2026-09-26.
Response preserved verbatim below. Primary concurs with this bounded correction.

Approved the exact one-line tooling correction in `lefthook.yml`:

```yaml
run: npx eslint --config web/eslint.config.js --max-warnings 0 --no-warn-ignored {staged_files}
```

Read the required authority and tooling rules. Verified locally:

- Installed ESLint supports the flag.
- Original mixed-file command fails on three CSS/JSON ignored-file warnings.
- Corrected JS/TS + CSS/JSON and CSS/JSON-only commands exit 0 without warnings.
- Actual TypeScript lint errors and warnings still exit 1; `--max-warnings 0` remains effective.

Proceed through the proposed independent review, worker edit and normal commit hooks. No new ADR or user reconfirmation is needed for this bounded correction. No files or Git state changed by this decision run; primary should persist this receipt.
