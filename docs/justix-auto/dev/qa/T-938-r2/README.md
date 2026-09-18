# T-938 r2 QA evidence

This directory verifies exact commit
`931ce802d736950e1d89f4b0cf231bebe6f6dc2f` after the Unicode traversal
correction. `independent_test.go` repeats the original BOUNCE logic; only its
evidence destination and overlay filename differ so the original evidence stays
immutable.

- `independent.log`: actual emitted TypeScript and Go Unicode order/outcome.
- `scope.log`: full scoped race suite; `scope-exit.txt` is `0`.
- `vet.log`: scoped vet output (intentionally empty); `vet-exit.txt` is `0`.
- `semantic.ts.txt`, `semantic.gen.go.txt`, binding/probe/input/result copies:
  artifacts emitted by the independent probe.
- `audit.json`: exact commit/diff/source/manifest and original-evidence audit.
- `SHA256SUMS`: r2 report and evidence inventory.

The prior `T-938.md` BOUNCE and `T-938/` evidence are unchanged and verified by
their original checksum inventory.
