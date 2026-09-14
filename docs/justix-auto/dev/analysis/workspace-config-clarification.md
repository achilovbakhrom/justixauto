# Workspace configuration ownership clarification

2026-09-14. Coordinator found a concrete planning omission during T-004:
T-032…T-039 own package manifests and tests but did not own TypeScript or Vitest
configuration. T-004's shared configuration expects child projects, so leaving
the omission would prevent their tests/typechecks from being discovered.

Each of those eight tasks now owns its unique `tsconfig.json` and
`vitest.config.ts` beside its package manifest. These are task-local leaves;
service ownership, dependencies, acceptance scope and task count are unchanged.

Adding workspace package manifests also needs the single npm root lock updated.
Their `coordinator_files` metadata and workflow now explicitly authorize a
serialized coordinator-only `package-lock.json` refresh on each task branch,
before independent QA. The exact final task SHA includes that lock delta.
Workers retain disjoint ownership and may not edit the root lock concurrently.
This clarification does not authorize arbitrary manifest/dependency changes or
post-QA lock regeneration.

Recoverable pre-edit documents and SHA-256 values are recorded in
`state/backups/2026-09-14-workspace-plan-repair/` under the project docs root.
