# ADR-13 dependency pin amendment — T-035

2026-09-15. Coordinator approves exact development-only `msw` 2.15.0 for
T-035's schema-checked fixture harness, completing the package family already
selected by architecture §9 and the approved task. This is a pin completion
under ADR-13; service ownership, production transports and reliability
guarantees are unchanged.

Public [npm registry metadata](https://registry.npmjs.org/msw/2.15.0) was read
using `npm view msw version dist.integrity engines peerDependencies --json`.
It reports Node `>=18`, TypeScript `>=4.8.x`, and integrity
`sha512-2wQAmKkQKxRuXvYJxVhPGG0wZNBQyD06oJvxqw90XqLvptdqxdlHrFUfEteKkpaNORX3Xzc+HtEl/q0nfmN2wQ==`.
These ranges accept the approved Node 24.21.0 and TypeScript 6.0.3.

The coordinator owns the serialized root lock update on T-035. Independent
QA must verify the resulting exact task commit, fixture behavior and locked
installation before integration. Registry metadata is not execution evidence
or a vulnerability audit. MSW is not production authentication or an app backend.

The pre-amendment canonical lock document has a SHA-256 manifest and recoverable
gzip snapshot in `../backups/2026-09-15-msw-pin/`.
