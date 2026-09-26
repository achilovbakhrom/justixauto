# CF-08 desktop visual correction

Primary inspected actual Chrome screenshot qa-desktop.png from candidate e8709bb
at 1440×900; preserved as qa-desktop-r1.png. Four sections/name/email are correct.
The password hint adds a third `.field` grid child, while confirmation has two;
stretched grid content makes the confirmation input lower and taller than password.

The original approved CF-08 scoped-group styling already owns design.css. Correct
only grouped field alignment, e.g. `.form-fieldset .field { align-content: start; }`
or equivalent section-grid item alignment. No ungrouped layout, labels, types,
API, state, tests, dependency or backend edits. This is a visual correction within
P1, not new feature scope. Source must remain frozen until Q1 releases its lease.

Assign one worker to only `web/packages/kit/src/design.css` and
`visual-repair-result.md`; version control commits that exact leaf with normal
hooks after worker release. Independent review checks the CSS diff and verifies
all TS/contract/test source trees are unchanged. Q1 refreshes desktop/mobile,
checks equal password/confirmation input geometry, footer and representative
submission/error interaction at the new final SHA. Existing full behavioral
evidence remains attributed to its executed SHA; final acceptance records both
unchanged-code equivalence and new-SHA integration checks. Do not rerun unrelated
backend work or expand this alignment correction into a redesign.
