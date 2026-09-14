# Dependency lock approval — T-001

2026-09-14. User authorized the development workflow with "ok lets start".
The coordinator approves the exact pins in task commit
`2908e9ee82fdce240d0161ab33cae858ea2007ca`, following independent
[QA GREEN](../dev/qa/T-001.md), under existing ADR-11/12/13.

Approved baseline: [dependency-lock.md](../dev/dependency-lock.md).
This approval record supplies the decision required by T-001; the reviewed
proposal's original pending-status wording is retained to preserve its exact SHA.

The supported compiler and Node distributions may be installed into ignored
project-local toolchain storage with the approved SHA-256 verification. Global
toolchain installations and PATH settings remain unchanged. T-003 and T-004 may
proceed after integration, in separate worktrees, with independent exact-SHA QA.

T-003 is the initial Go manifest owner: declare required approved backend runtime
packages and resolve their real module graph/sums there, without dummy source
imports. Keep optional verification executables versioned separately. Later
dependency additions or changes require explicitly assigned serialized manifest
ownership; feature workers must not quietly alter shared manifests.

The ClamAV engine image requires AMD64 emulation on this Apple Silicon laptop;
scanner readiness, signatures and behavior must be checked in its owning task.
Optional telemetry collector infrastructure remains disabled until separately
pinned. Full transitive vulnerability checks, executable compatibility and
Argon2 benchmarks remain implementation evidence, not outcomes of this lock.

No service ownership, transport, business policy, security-release settings or
production permissions change. T-001 contributes to B-01; it does not complete
that backlog item's acceptance.
