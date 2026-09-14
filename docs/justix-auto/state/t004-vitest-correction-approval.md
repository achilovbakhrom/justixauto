# T-004 Vitest correction approval

2026-09-14. The coordinator approves replacing only `vitest` and
`@vitest/coverage-v8` 5.0.0 with the matched **4.1.11** pair under ADR-13.
User authorization is the active development workflow; no service architecture,
business rule or production permission changes.

Strict TypeScript compilation of the actual installed Vitest 5.0.0 package
failed on a missing `@vitest/expect` declaration dependency and a `MarkOptions`
export mismatch. Metadata-only T-001 QA did not establish executable compatibility.
The frontend developer reproduced the failures and verified the 4.1.11 candidate
with strict config/source/test checks, test execution, coverage and builds.
The architect independently verified exact metadata, SRI hashes and direct
advisory evidence in `state/drafts/architect/t004-vitest-pin-correction.md`
(relative to the project docs root, recorded separately on main).

Selected hashes:

- `vitest@4.1.11`: `sha512-fhACrNXUidIbGSBr5FlbuBkO7VWC1ZyLl0DO4CU2DrQoAPxX84Ysxs+HeGQpii5lZWV1Q4gBZTTu49mF+A6Edw==`
- `@vitest/coverage-v8@4.1.11`: `sha512-8MVGEFnJIcdGjcbfKmeq8z0pZHH0JlVtoVZH9Q/qwUp6wyFnEJUBMrw9DCaj+ra3vShGmhavjalMIhPNxZAUcw==`

Official [Vitest metadata](https://registry.npmjs.org/vitest/4.1.11) and
[coverage metadata](https://registry.npmjs.org/@vitest/coverage-v8/4.1.11)
match those hashes. Node 24.21.0, Vite 8.3.0 and the matching coverage peer are
compatible by their declared ranges. The [4.1.11 release](https://github.com/vitest-dev/vitest/releases/tag/v4.1.11)
provides recent maintenance evidence; this is not an LTS/support-duration claim.
No stable 5.x patch beyond 5.0.0 existed in the inspected registry metadata.

No library-check suppression or node_modules patch is authorized. Preserve strict
checking of the shared tooling config to catch future declaration regressions.
Use explicit `--config web/vitest.workspace.ts`, `defineConfig` and `test.projects`
according to the [v4 projects documentation](https://v4.vitest.dev/guide/projects).

The coordinator owns this approval and the amended dependency-lock document,
with a recoverable pre-edit snapshot. T-004's developer owns its package/config
changes. Independent QA must review the final combined T-004 commit before merge;
this approval does not substitute for QA or imply a completed application.
