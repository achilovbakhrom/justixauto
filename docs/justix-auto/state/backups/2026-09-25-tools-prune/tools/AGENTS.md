# Tooling and version control

Use explicit argv arrays for subprocesses, no shell interpolation of packets or
Git refs. Local orchestration state is not proof of human identity. Tools must
reject protected-ref writes and stale SHAs before effects. Test in disposable
repositories/bare remotes; tests must not push the project remote. Unknown dirty
paths, conflicts, locks and stale evidence fail closed and preserve work.
Use `node --test tools/agent-*.test.mjs` for the hub-tool tests.
Only version_control performs project Git mutations after its hub supplies the
exact operation/paths/expected SHA. Only the human promotes production.
