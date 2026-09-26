# CF-08 desktop visual correction result

Status: DONE

Changed `web/packages/kit/src/design.css`: grouped `.field` grid content now aligns at the start, so a field with a hint does not stretch a sibling password input in the same fieldset.

Evidence: `git diff --check` passes; the diff changes only the assigned CSS file and this result artifact.

Excluded: browser validation, full build, TypeScript, contracts, tests, APIs, and ungrouped field styling; Q1 owns final new-SHA browser/build checks.
