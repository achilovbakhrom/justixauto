# T-004 — proposed Vitest pin correction

Date: 2026-09-14. Status: proposed for coordinator approval and T-004 exact-commit QA.
Scope: replace only the Vitest/coverage pair in the approved dependency family. This draft does not reopen integrated T-001, edit canonical documents, install packages or change application code.

## Decision proposed

Change `vitest` and `@vitest/coverage-v8` from **5.0.0 to 4.1.11** together. Retain Node 24.21.0, npm 11.19.0, Vite 8.3.0, TypeScript 6.0.3 and all other approved pins. The independently retrieved exact registry integrity values match the frontend worker's reported values.

| Package | Exact version | SHA-512 integrity | Official metadata |
|---|---|---|---|
| `vitest` | `4.1.11` | `sha512-fhACrNXUidIbGSBr5FlbuBkO7VWC1ZyLl0DO4CU2DrQoAPxX84Ysxs+HeGQpii5lZWV1Q4gBZTTu49mF+A6Edw==` | [npm version record](https://registry.npmjs.org/vitest/4.1.11) |
| `@vitest/coverage-v8` | `4.1.11` | `sha512-8MVGEFnJIcdGjcbfKmeq8z0pZHH0JlVtoVZH9Q/qwUp6wyFnEJUBMrw9DCaj+ra3vShGmhavjalMIhPNxZAUcw==` | [npm version record](https://registry.npmjs.org/@vitest/coverage-v8/4.1.11) |

The [Vitest registry](https://registry.npmjs.org/vitest) returned `latest: 5.0.0` and `V4: 4.1.11`; no stable 5.0.1 or later patch was published in the retrieved metadata. Therefore a nonexistent 5.x patch is not a candidate. The [official 4.1.11 release](https://github.com/vitest-dev/vitest/releases/tag/v4.1.11), dated 2026-08-18, contains v4 backports including filesystem allowlist enforcement for redirected mocks. The registry marks neither selected package deprecated. This is an available prior-major patch with recent maintenance evidence, not a promised LTS or support end date; the [v4 documentation](https://v4.vitest.dev/guide/projects) labels that series an old version.

## Confirmed metadata compatibility

- Vitest 4.1.11 declares Node `^20.0.0 || ^22.0.0 || >=24.0.0`; Node 24.21.0 satisfies it.
- Its Vite dependency and peer range is `^6.0.0 || ^7.0.0 || ^8.0.0`; Vite 8.3.0 satisfies it.
- Its optional Node declarations peer accepts `^20.0.0 || ^22.0.0 || >=24.0.0`; the existing @types/node 24.13.4 satisfies it.
- Coverage 4.1.11 requires exactly `vitest: 4.1.11`; its `@vitest/browser: 4.1.11` peer is optional. No new browser adapter is required for the current non-browser coverage configuration.
- Vitest 4.1.11 explicitly depends on `@vitest/expect: 4.1.11`, alongside matching internal runner/spy/utils packages. Vitest 5.0.0's published dependency metadata omits that package. The [5.0.0 metadata](https://registry.npmjs.org/vitest/5.0.0) was inspected independently; this supports the reported missing-package symptom but does not independently reproduce all declaration failures.
- Neither candidate package declares a TypeScript peer requirement. Consequently metadata cannot certify strict TypeScript 6.0.3 declaration compatibility; the frontend's compiler probe and exact-commit QA supply that evidence.

## Observed failure and reported runtime evidence

The coordinator reports the frontend worker found strict TypeScript failures in the 5.0.0 declarations: `config.d.ts` references missing `@vitest/expect`, and plugin declarations import `MarkOptions` absent from `vitest/browser`. The earlier T-001 metadata review checked engines and peer ranges but **missed these declaration-package failures**. Its package compatibility statement must not substitute for compiler/runtime verification.

The coordinator subsequently reports that the frontend worker's isolated 4.1.11 probe passes strict configuration and test-source compilation, `test.projects` execution, coverage and an audit with zero findings. Those are attributed frontend results, not tests performed by this architect. Preserve the worker's exact commands and outputs in the T-004 result. Independent QA must check the final T-004 commit after installation and lock regeneration.

## Configuration implications

Keep the assigned `web/vitest.workspace.ts` file as an explicitly selected config: `vitest run --config web/vitest.workspace.ts`, using `defineConfig` and `test.projects`. The [version-specific projects documentation](https://v4.vitest.dev/guide/projects) supports that shape and deprecates the old workspace option. No `defineWorkspace` fallback is needed.

If project configuration relies on parent settings, use explicit inheritance and explicit mock-reset settings where required. The [5.0 migration guide](https://vitest.dev/guide/migration/) identifies changes from v4: inline project inheritance and `clearMocks` default differently, and nested project containers are new in v5. Do not assume v5 defaults under the corrected v4 pin. No missing declaration shim, direct @vitest/expect workaround, broad package expansion or disabled strict library checking is proposed.

## Security lookup and limits

On 2026-09-14 an independent request to the official npm bulk-advisory endpoint `https://registry.npmjs.org/-/npm/v1/security/advisories/bulk`, containing only `{"vitest":["4.1.11"],"@vitest/coverage-v8":["4.1.11"]}`, returned `{}`. This direct-version result is not a full transitive audit or a guarantee of no vulnerabilities. T-004's regenerated lock and QA must retain the ordinary full dependency audit and strict compiler/runtime checks. Metadata SRI values were read; no tarballs were downloaded or installed by this follow-up.

Coordinator approval within ADR-13 is sufficient for this tooling correction; no additional user architecture checkpoint is needed. The coordinator owns canonical lock/result updates and promotion, and the frontend owns its task manifest/lock changes. Service ownership, event delivery, local transactions and financial-policy decisions are unaffected. No unfinished policy or runtime guarantee is approved by this proposal.
