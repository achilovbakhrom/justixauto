# Auth transport compatibility — independent QA

2026-09-15. **BOUNCE** for exact proposal commit
`9ad5a64422b04dbbc8bee969a47acdd3e9278d81`, detached checkout
`.worktrees/auth-transport-compatibility-qa`.
Proposal SHA-256: `539aada61e3751130e880fe059c13e92663d716425b60ea981a5a4161dc4670d`.
This is proposal-readiness review, not a finding that unimplemented auth runtime
is deployed. No application source, canonical document, dependency or Git history
was changed. Only this report and its assigned evidence directory were written.

## Required correction: enforce the synchronous semantic boundary

P6 specifies `validate<Name>(value: Name): void`, forbids async validators, and
requires construction to reject missing/nonfunction methods. Those concrete
checks do not enforce the stated synchronous boundary. Strict TypeScript accepts
an `async` function **and an ordinary function returning a Promise** for that
void method. The independent [interface probe](auth-transport-compatibility/interfaces.ts)
compiles against the actual T-032 `Schema`, `ApiRequest` and `ApiResult` types.
Its structurally valid SessionRead has mismatched outer/context revisions.
The described function-presence check accepts its binding, `Schema.parse`
returns the invalid session, and a simulated metadata sink runs before the
Promise rejects with `semantic mismatch`.

This is the same class of pre-validation acceptance the proposal intends to
remove. The statement “no async validator” needs an enforceable return contract
and runtime behavior, not a convention. Define a synchronous return such as
`undefined` or an explicit synchronous validation outcome, reject all other
runtime returns before use, and cover normal Promise-returning functions and
thenables as well as `async` functions. Checking `AsyncFunction` identity alone
is insufficient. Apply this to every named/ref semantic hook and operation/status
error hook. Missing/invalid policy, binding defects and asynchronous returns
must produce conservative invalid-request before dispatch or invalid-response /
unknown write outcome after dispatch, with zero sink calls and no retry. Define
safe handling of a rejected asynchronous return without exposing diagnostics or
secrets through an unhandled rejection. No awaiting a semantic callback between
epoch verification and memory update should be introduced.

The return-type behavior is documented by the official
[TypeScript handbook](https://www.typescriptlang.org/docs/handbook/2/functions.html#return-type-void).
The required-marker negative type assertion also passes: the proposal correctly
identified that optional request additions alone permit the legacy transport.

## Required correction: distinguish union mismatch from binding failure

P6 says to invoke semantic bindings per named/ref occurrence and then count union
alternatives including semantics. The actual T-640 TS runtime counts alternatives
with an unconditional `catch`; its Go runtime similarly counts any nil error as
matching and does not classify failures. Reusing those paths for new semantic
hooks can turn missing-policy/configuration faults or implementation exceptions
into a harmless nonmatching branch and accept another branch. The proposal
defines no semantic mismatch versus fatal binding/configuration outcome.

The independent [union composition probe](auth-transport-compatibility/union_probe.mjs)
reproduces that mechanism and shows a feasible discriminated failure contract.
Specify in both languages which exact outcome may count as a nonmatch, which
failures abort the entire validation, and how asynchronous/invalid returns are
handled before union counting. Ordinary structural `oneOf` cardinality must
remain mandatory; a broken binding must not resolve structural ambiguity.
Require tests with another otherwise valid alternative present, since testing
only a single invalid variant misses the acceptance path. This is an extension
design gap, not a claim that a concrete T-058 union currently has overlapping
structural branches or that existing structural-only T-640 code is defective.

## Checks that passed

- Exact HEAD, proposal hash and two-leaf source scope relative to base
  `476777139ffa6a470c7b3f6e79086c64f2dc7db5`; actual T-032/T-640/auth source hashes
  match the proposal result. No reliance on the author's scanner as evidence.
- Five aliases, 13 distinct owned leaves, five explicitly serial prior-owned
  leaves and all proposed dependencies: the 934-task graph plus five aliases is
  acyclic (939 nodes). T-641→AT-GEN, T-055→AT-EPOCH/AT-AUTH and T-060→AT-AUTH
  preserve existing edges. T-059 has no artificial frontend dependency.
  Root config/lock/export ownership is explicit; root must serialize its API
  package export edits with AT-EPOCH's manifest ownership in actual assignments.
- Current T-032 API regression suite: **93/93 PASS**. Independent actual-client
  probes confirm duplicate-name loss, invalid UTF-8 replacement, decimal rounding,
  underflow, absent feature error validation before generic401, missing metadata
  and unsupported429. Existing finite fraction behavior, reserved-header and
  foreign-path rejection, same-origin/error-redirect/no-store options and a single
  failed unsafe fetch with unknown outcome remain intact.
- Actual generated Go decoder text extracted from this exact generator:
  **26/26 cases PASS**, `go test -race -count=1 -v ./...`, 1.641s.
  Includes integral exponent forms, safe integer endpoints, fraction/underflow/
  overflow rejection, extreme-exponent zero, nested escaped duplicate names,
  lone surrogate keys/values, valid pairs, prototype names and trailing values.
  This proves current decoder compatibility examples, not the proposed bounded
  browser decoder, Go resource guards or a complete Go auth adapter.
- Strict TS interface compile exit0. Independent Node probe exit0 deliberately
  asserts that the blocking premature-sink behavior is reproduced; exit0 is
  **not** a GREEN proposal verdict. Seven union composition cases (nine assertions) also pass.
- Header syntax boundary probes cover merged duplicates, trimmed whitespace,
  token size/alphabet and canonical Retry-After limits. Synthetic Node204 rejects
  a body, while Node synthetic Set-Cookie remains observable, confirming why
  that fixture cannot prove browser filtering. No browser access was needed.
- `bash tools/check-git.sh` and final `git diff --check` pass. Tracked files are
  unchanged; exact HEAD remains the assigned SHA.

P4 correctly labels token alphabet/4096 ceiling, delay-seconds ceiling, canonical
no-store/media forms and 8 MiB/128-level defaults as **new proposed technical
refinements**. They require coordinator adoption and owner/schema handoffs, and
do not accept T-002 entropy, lifetimes, rate schedule or proof policy. Bound IO
must count decoded bytes and depth, and Go must avoid constructing huge rational
powers from untrusted exponents; the old decoder probe is not that resource test.

P5 correctly separates local ticket/epoch acceptance from server cookie authority:
abort/discard cannot undo browser Set-Cookie, stale cookies must be rejected by
owner revocation checks, local busy does not mean successful logout, and explicit
session reacquisition cannot recover lost one-time material or prove a write
rolled back. Mandatory current-ticket checks must also govern failure/finally
paths so a stale rejection cannot clear a newer session or release its slot.
No automatic retry/reacquisition, cross-tab token transfer or new logout scope is
approved. These browser limits agree with the official
[Fetch header/response model](https://fetch.spec.whatwg.org/#concept-filtered-response-basic),
[Encoding decoder rules](https://encoding.spec.whatwg.org/#interface-textdecoder)
and the explicitly narrowed [Retry-After grammar](https://www.rfc-editor.org/rfc/rfc9110.html#section-10.2.3).

## Scope and remaining gates

AT-JSON, AT-HTTP, AT-EPOCH and the separate AT-AUTH binder have coherent file and
dependency boundaries. AT-GEN is the highest scope risk: two-language headers,
auth ports, semantic dispatch/failure taxonomy, raw numeric/resource parity and
output-guard regressions are substantial. Its max4h condition is an assignment
gate, not demonstrated by these probes; estimate and reslice before assignment
if necessary. Do not shift generator implementation into T-641's generated leaves.
The two required corrections can remain in the proposed generator/binder scope
if that estimate holds; this QA invents no new task IDs or ownership changes.

After a revised exact proposal passes independent review, canonical promotion
must preserve current task states, add the reviewed aliases/edges and concrete
scope handoffs, and record preimages. T-059 must declare the adopted wire/header
and mandatory semantic binding profile; T-641 supplies generated interfaces and
synthetic conformance, AT-AUTH concrete semantics, and T-055 current-epoch React
integration. T-060/server/edge tasks and T-592 retain raw wire/cookie race/owner
transaction verification; uncovered files must be assigned explicitly. A missing
release policy still prevents production activation. Neither this review nor
parser probes establish authentication, private persistence, broker delivery,
frontend UI parity or recovery delivery. No production auth, secret, container,
reference infrastructure, npm install/audit or external message was used.

Commands, source identities and harness-only failed attempts are retained in
[commands.txt](auth-transport-compatibility/commands.txt). Reproducers are the
[independent runner](auth-transport-compatibility/independent_probe.mjs),
[interface probe](auth-transport-compatibility/interfaces.ts) and
[union probe](auth-transport-compatibility/union_probe.mjs).
