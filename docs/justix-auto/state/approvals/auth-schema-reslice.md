# Auth schema and storage task reslice

2026-09-16. Coordinator execution decision under the already authorized dev
workflow. Before any T-059 application edit, the assigned developer assessed
the original six-leaf deliverable as exceeding its three-hour bound: ten-route
wire/event/fixture encoding and seven private state families with live migration
and privilege checks require separate bounded work. Existing acceptance remains.

- T-059 retains three wire/fixture leaves and
  `tests/contracts/identity_auth_test.go`, at three hours. It owns no SQL.
- T-949 owns `services/identity/migrations/0012_auth.up.sql`, the adjacent
  `0012_auth.manifest.json`, and the serial successor of that test leaf, at four
  hours. It depends on T-059, T-929 and T-931 and preserves all schema tests.
- T-060 gains T-949 before credential runtime work. MFA/bootstrap/recovery
  already depend transitively on T-060. T-592 gains T-949 explicitly for B-02
  closure. T-641 and presentation tasks consume wire schemas without a new
  database barrier. Minimal Identity composition retains its existing scope.

The [adopted auth contract](auth.md), [transport refinement](auth-transport-compatibility.md)
and [owner migration contract](owner-migration-compatibility.md) remain unchanged.
This moves ownership and testing only. It does not accept T-002, recovery proof,
delivery, numerical security policy, production baseline or deployment. Feature
storage design must implement the existing ownership and secret exclusions;
any new contract or reliability guarantee still needs a separate approved handoff.

There are now 949 tasks and 3,158 estimated task-effort hours. Historical graph
width/schedule metrics are not recomputed forecasts. Both tasks require their
own branch/worktree and independent exact-commit QA. Reassess the four-hour
storage bound before implementation; do not weaken negative checks to fit it.

Seven canonical preimages are recoverable in
`../backups/2026-09-16-auth-schema-reslice/`, with original SHA-256 and byte counts.
