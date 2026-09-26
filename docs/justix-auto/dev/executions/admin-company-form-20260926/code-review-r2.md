# Independent code review — R2

Actor/run `/root/company_form_review_r2`, 2026-09-26. Response preserved verbatim:

GREEN

- Identity: `/root/company_form_review_r2`
- Mode: `CODE`
- Reviewed range: `eca22f60f9082f7fe59004c81be63170ebfe1483..e8709bbffa125f94b7667e52e230938b656838b4`
- Head SHA: `e8709bbffa125f94b7667e52e230938b656838b4`
- Cumulative diff SHA-256: `8320b9c6a4334d9811a23bd1694effa9bc4973abf1ba6d4c5cfae0fa577ec221`
- Requirements reviewed: `CF-01`–`CF-08`, `H1`
- Findings: None.

Evidence:

- `CF-01`–`CF-04`: creation exposes one company-name and one email field, four ordered groups, maps the email to both API objects, and retains seller/provider endpoints, dynamic provider kind, credentials, draft wording and separate activation actions. [pages.tsx:96](/Users/bakhromachilov/startups/justixauto/web/apps/admin/src/pages.tsx:96), [pages.tsx:177](/Users/bakhromachilov/startups/justixauto/web/apps/admin/src/pages.tsx:177), [pages.tsx:212](/Users/bakhromachilov/startups/justixauto/web/apps/admin/src/pages.tsx:212)
- `CF-05`: edit supplies only company requisites while preserving fetched `legalName`, company ID and revision/`If-Match`. This is necessary because the unchanged service replaces all requisites during update. [pages.tsx:340](/Users/bakhromachilov/startups/justixauto/web/apps/admin/src/pages.tsx:340), [company.go:52](/Users/bakhromachilov/startups/justixauto/internal/modules/identity/service/company.go:52), [company.go:239](/Users/bakhromachilov/startups/justixauto/internal/modules/identity/service/company.go:239)
- `CF-06`: combobox inputs remain free-form, canonicalize known countries, disable region only for an empty country, and clear dependent values only when the country changes. [ui.tsx:755](/Users/bakhromachilov/startups/justixauto/web/packages/kit/src/ui.tsx:755), [ui.tsx:888](/Users/bakhromachilov/startups/justixauto/web/packages/kit/src/ui.tsx:888), [geo.ts:37](/Users/bakhromachilov/startups/justixauto/web/packages/kit/src/geo.ts:37)
- `CF-07`: the R1 defect is repaired. A matched nested key is always consumed by its visible field; identical messages are deduplicated without falling into the general notice. The strengthened regression rejects both a second occurrence and a raw `firstAdmin.email` notice. The synchronous ref guards click/Enter replay before React state updates. [ui.tsx:766](/Users/bakhromachilov/startups/justixauto/web/packages/kit/src/ui.tsx:766), [ui.tsx:795](/Users/bakhromachilov/startups/justixauto/web/packages/kit/src/ui.tsx:795), [form-dialog.test.ts:59](/Users/bakhromachilov/startups/justixauto/web/packages/kit/src/form-dialog.test.ts:59)
- `CF-08`: grouping preserves the flat value/submission model and leaves ungrouped forms on their prior rendering path. Fieldsets span the outer grid and collapse internally to one column below 600px. [ui.tsx:838](/Users/bakhromachilov/startups/justixauto/web/packages/kit/src/ui.tsx:838), [design.css:2375](/Users/bakhromachilov/startups/justixauto/web/packages/kit/src/design.css:2375), [design.css:4443](/Users/bakhromachilov/startups/justixauto/web/packages/kit/src/design.css:4443)
- The unchanged API contracts confirm atomic company/admin/membership provisioning and optimistic-locking company updates. [admin.go:79](/Users/bakhromachilov/startups/justixauto/internal/modules/identity/handler/admin.go:79), [company.go:119](/Users/bakhromachilov/startups/justixauto/internal/modules/identity/service/company.go:119), [company.go:167](/Users/bakhromachilov/startups/justixauto/internal/modules/identity/service/company.go:167)
- `H1`: the sole hook change adds `--no-warn-ignored` while retaining `--max-warnings 0`. [lefthook.yml:28](/Users/bakhromachilov/startups/justixauto/lefthook.yml:28)
- The adopted test drift is exactly four `{ exact: false }` country-label selector changes; no assertion or test case was removed. Its SHA-256 is the recorded `adb57e7e6e28ed799c8092ebaeb5f6e678ddc167b9e88a37b6c18239c36195c4`. The other six protected precursor leaves match their entry hashes.
- The cumulative patch contains exactly the authorized 13 paths and passes static `git diff --check`. All committed application/tooling paths match `HEAD`; only canonical records and backups are dirty.

Unverifiable areas:

- Per reviewer constraints, I ran no unit, type, build, lint, hook, browser or accessibility tests.
- Final test execution, all four provider paths, responsive layout, datalist browser behavior, hook controls and request interception remain owned by Q1 at this same SHA.
