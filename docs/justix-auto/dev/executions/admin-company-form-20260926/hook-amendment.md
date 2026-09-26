# Company form hook amendment H1

Status: proposed; independent plan reviewer and DevOps acceptance required before editing.
Authority: primary application hub plus DevOps for this hook-only correction.
Parent packet/acceptance remain frozen; frontend requirements and QA coverage are unchanged.
Base: `eca22f60f9082f7fe59004c81be63170ebfe1483`; coordinator reports nine TS files committed at `fce20e65dd7f1b45a2a0bac8d86c67ea5d70cf10`.
Remaining staged precursor paths: `package-lock.json`, `web/packages/kit/package.json`, `web/packages/kit/src/design.css`; preserve them.
Reason: the outer web hook passes CSS/JSON through staged_files to nested ESLint despite its narrower glob. Both mixed and CSS/JSON-only normal commit attempts failed on ignored-file warnings with `--max-warnings 0`.
Additional worker ownership: ONLY `lefthook.yml`, one existing ESLint command line; release writer after edit/checks. No Git writes by worker, dependencies, runtime policy, hook bypass or frontend edits.
Read root AGENTS.md and dev/agent-workflow.md; run own-root guard before mutation. One writer; no branch switch. Context limit: 3,000 input tokens.

Exact diff:
```diff
-            run: npx eslint --config web/eslint.config.js --max-warnings 0 {staged_files}
+            run: npx eslint --config web/eslint.config.js --max-warnings 0 --no-warn-ignored {staged_files}
```

Local evidence: pinned-toolchain `npm exec -- eslint --help` succeeded during slicing and explicitly lists `--no-warn-ignored`: “Suppress warnings when the file list includes ignored files”. This retains zero tolerated actual lint warnings/errors.
Independent verification after approval (pinned Node/npm, repository root):
```sh
export PATH="$PWD/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin:$PATH"
npm exec -- eslint --config web/eslint.config.js --max-warnings 0 --no-warn-ignored web/apps/admin/src/pages.tsx web/packages/kit/src/ui.tsx web/packages/kit/src/design.css web/packages/kit/package.json package-lock.json
npm exec -- eslint --config web/eslint.config.js --max-warnings 0 --no-warn-ignored web/packages/kit/src/design.css web/packages/kit/package.json package-lock.json
```
Both commands must exit 0. Negative control: create only `/private/tmp/justixauto-hook-invalid.ts` containing `const broken: = ;`; pipe it to the same ESLint flags with `--stdin --stdin-filename web/apps/admin/src/hook-negative.ts`. Require nonzero exit and an actual TypeScript parsing diagnostic, not ignored-file/configuration failure. This applies the real project config without creating a source fixture. Persist command outputs through the hub's evidence channel.
Version_control then commits the remaining exact precursor files plus lefthook.yml through normal hooks, verifies formatting/staged changes and reports the immutable final SHA. No `--no-verify`, hook-disable flags or reduced lint thresholds.
Independent exact-diff review and Q1 still cover cumulative base..final candidate, all ten precursor files and the form correction, now including lefthook.yml. Run unchanged frontend acceptance against that final SHA; a list of earlier passes is insufficient.
Stop on rejected/missing reviewer or DevOps acceptance, unsupported flag, failed positive checks, negative control accepted/ignored, unrelated hook changes or inability to create a normal-hook candidate. Report gap to hub; no broad lint/config rewrite. No production or dev/main integration is authorized.
