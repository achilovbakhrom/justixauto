# H1 hook worker result

Status: DONE. Edited only `lefthook.yml`: added `--no-warn-ignored` to the
existing ESLint command while retaining `--max-warnings 0`.

Pinned Node/npm command evidence:

```text
MIXED_EXIT=0
IGNORED_EXIT=0

/Users/bakhromachilov/startups/justixauto/web/apps/admin/src/hook-negative.ts
  1:14  error  Parsing error: Type expected

✖ 1 problem (1 error, 0 warnings)

NEGATIVE_EXIT=1
```

The mixed command included TS plus ignored CSS/JSON paths; the ignored-only
command included CSS/JSON paths. The negative stdin control used the project
config and produced a TypeScript parsing error with a nonzero status. No Git or
index operations occurred; staged precursor files and frozen frontend were not
altered. Source writer lease released.
