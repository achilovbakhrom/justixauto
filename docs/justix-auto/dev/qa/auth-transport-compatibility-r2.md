# Auth transport compatibility — independent r2 QA

2026-09-15. **GREEN for proposal readiness**, exact revised commit
`914fcd78cc0789e41e9091efdc99fb5afc6c5c50` in detached checkout
`.worktrees/auth-transport-compatibility-r2-qa`.
Proposal SHA-256: `e1643c8cf36ba5733f7bfa0d32e2285604b345fe72ed506e17248fbae9af3e0c`.

Both findings from the [original BOUNCE](auth-transport-compatibility.md) are
resolved in the revised specification. This approves the bounded technical
handoff for canonical promotion and subsequent task assignment. It does not
claim that an upgraded transport, generator, concrete auth binder, session
controller or owner HTTP implementation exists. Each task still requires its
own exact-commit implementation QA.

## Independent verification

- [Strict TypeScript probe](auth-transport-compatibility-r2/interfaces.mts):
  compile exit0 using pinned TypeScript6.0.3 and actual T-032 exported types.
  Eleven expected negative assignments verify async, ordinary Promise and
  thenable returns are rejected for readiness, named and operation/status hooks;
  old void hooks and missing transport version are rejected too. The old void
  callback still accepts async, preserving the original failure's distinction.
- [Independent runtime model](auth-transport-compatibility-r2/runtime.mjs):
  **91 scenarios /153 assertions PASS** on Node24.21.0. Nineteen illegal return
  categories run through all three hook categories, including native resolved /
  rejected /async /cross-realm /thrown Promises, objects with getters, callable
  thenables, boxed strings, undefined and unknown strings. Supported Promise
  rejection cases produce **zero unhandledRejection events** across two event-loop
  turns; arbitrary then/getter invocation counts are **zero**. Runtime rejection
  is immediate and never awaits validation or treats Promise cleanup as success.
- The same model checks global policy readiness, repeated readiness before each
  selected occurrence and error hook, captured immutable function dispatch,
  missing/nonfunction/accessor/inherited binding rejection, complete nested
  structural validation before any semantic hook, selected child→union→parent
  hook ordering, ambiguity rejection and fatal propagation when another
  structural alternative could otherwise match. A selected semantic mismatch
  or fault never falls through to another branch. Phase-sensitive failure
  classification preserves unknown dispatched writes; stale failure/finally
  tickets cannot clear a newer state or release its slot.
- [Independent Go interface probe](auth-transport-compatibility-r2/semantics_test.go),
  plus actual generator `ContractExchange` declarations and actual decoder text
  extracted by [prepare_go.mjs](auth-transport-compatibility-r2/prepare_go.mjs):
  `go test -race -count=1 -v ./...` **PASS,2.039s**; `go vet ./...` exit0.
  This compiles the proposed enum and method shapes with the existing injected
  exchange, rejects legacy/missing capability, nil and typed-nil pointer/map
  bindings, and checks all five exact outcomes plus zero/unknown values across
  all three hooks. Narrow panic conversion and structural/semantic separation
  pass. There are24 outcome,3 panic,7 union and2 resource subtests, plus the
  constructor checks and8 numeric corpus cases. These are independently written
  interface models, not the author's probe or generated auth clients.
- [Actual CLI probe](auth-transport-compatibility-r2/cli_probe.mjs): valid
  non-auth generation succeeds first. Ten generate/check cases then place an
  unsupported auth semantic marker, transport marker, response header,429 or
  request-CSRF declaration in a later contract. All reject before changing any
  of four retained output sentinels, including earlier-contract drift. This
  verifies the existing prewrite baseline the intermediate tasks must retain;
  it does not implement or certify those future intermediate gates.
- [Exact-scope checker](auth-transport-compatibility-r2/check.py): exact HEAD,
  proposal digest, two-leaf proposal/result scope and unchanged actual
  T-032/T-640/T-058 source identities pass. The original result/probe text remains
  embedded unchanged. All **five original independent QA artifacts** are
  byte-identical between the old review checkout and main. They retain BOUNCE.
  Git gate passes; final tracked diff is empty and only assigned new QA paths
  are present. No application code or canonical document was modified.

## Why the original findings are closed

P6 now requires five exact primitive string outcomes in TypeScript and matching
nonzero enum values in Go. Runtime validates every return independently of its
type, including readiness and operation/error hooks. Construction captures own
plain function descriptors without invoking getters; missing bindings cannot
hide behind an unselected union branch. Readiness mismatch is a binding fault,
not an ordinary data mismatch. Go nil, unknown/zero outcome and panic paths
fail closed at the narrow boundary without formatting raw diagnostics.

Native Promise rejection disposal is separate from validation and cannot allow
acceptance. The captured intrinsic `Promise.prototype.then` handles supported
ordinary and cross-realm native Promises while its brand check rejects plain
thenables without invoking their accessors. The proposal explicitly excludes
hostile Promise species/constructor overrides, proxies and independently spawned
unreturned work from its guarantees; it is not a sandbox for arbitrary callbacks.
That limit matters because the intrinsic uses Promise species construction after
checking its receiver. The mechanism agrees with the official
[ECMAScript algorithm](https://tc39.es/ecma262/multipage/control-abstraction-objects.html#sec-promise.prototype.then)
and the executed pinned-runtime cases.

The structural phase now traverses the entire tree and resolves every nested
oneOf cardinality before semantic hooks. Only ordinary structural mismatch may
remove an alternative. A missing schema, malformed structural result or fatal
configuration fault aborts the whole validation. Semantic hooks run only on the
unique selected tree, and their mismatch/fatal results reject that tree without
trying another alternative. Thus neither an async binding nor a fatal binding
fault can silently select a different branch or reach metadata acceptance.

## Scope, compatibility and promotion handoff

The proposed graph has **944 nodes**:934 existing tasks plus ten aliases,
**36 estimated hours**, with every estimate≤4h. Six generator aliases replace
the former AT-GEN: PROFILE→SEM→TS→GO→RAW→VERIFY. Each owns exactly the same two
generator/test leaves as a serial successor of its direct predecessor; later
work must start from that integrated predecessor in a new worktree. Across all
aliases there are13 unique leaves. Existing task dependencies remain intact;
the only proposed existing-task additions are T-641→AT-GEN-VERIFY,
T-055→AT-EPOCH/AT-AUTH and T-060→AT-AUTH. No graph cycle or new unowned helper
file was found. Root retains config/lock and concrete auth export registration;
it must serialize manifest edits with AT-EPOCH's explicit API subpath export.

The six-part generator split provides concrete intermediate outcomes with
bounded ownership. All intermediates retain public rejection of auth declarations
before output, have no bypass switch or concrete auth registration, and preserve
legacy emitted non-auth behavior. AT-GEN-VERIFY alone enables the completed
profile after two-language/actual-client/guard parity passes. Its task label
does not waive independent QA before integration. If concrete work exceeds4h
or terminal tests expose missing implementation, reslice the responsible task;
T-641 generated leaves must not absorb hidden generator work.

The newly explicit Go resource change is material and ready for coordinator
technical adoption: at terminal activation, **all newly regenerated Go outputs**,
including non-auth and internal methods, acquire8MiB/128-container limits.
The independent actual old decoder probe accepted both an8MiB-plus-two-byte
JSON string and129 nested arrays. Therefore identical acceptance cannot be
claimed; terminal QA must demonstrate the intended rejection and below-limit
compatibility. Old generated files are not automatically rewritten, and the
inert RAW phase must not change their emitted runtime early. Eight numeric cases
confirm existing exact safe-integer representations remain distinct from this
new resource bound. The Go byte-slice check happens after allocation by the
exchange: bounded network reading is still an adapter/composition obligation.

The retained P3/P4/P5 contract still requires status-specific structural and
semantic body validation plus exact allowlisted metadata before token handling,
one request without automatic retry, memory-only token/one-time secret handling
and current-ticket result acceptance. Browser header normalization and cookie
filtering do not expose raw singleton fields or let JavaScript undo Set-Cookie.
Owner/edge raw wire, cookie race, revocation, transaction and unknown-commit tests
remain downstream obligations; no browser or owner-runtime guarantee comes
from these models. T-070's revision-only cross-tab handoff remains unresolved
where already documented. No new logout scope or fabricated revision is added.

Canonical promotion must adopt the exact revised proposal/digest and preserve
original BOUNCE evidence and current task states using the required preimages.
Record all ten aliases/estimates/serial leaves, four new edges across three
existing tasks and T-059/T-641/T-055/T-060/T-592 scope handoffs. T-059 can then encode
the adopted header/resource/semantic declaration refinements on its existing
schema leaves without waiting for frontend implementation. T-641 requires
terminal generator integration and root registration; AT-AUTH follows its
generated types. Security-policy, entropy, token lifetimes, rate schedule,
release client constraints, recovery proof/delivery and T-002/OD-12 remain
unaccepted. No deployment, working auth feature, UI parity or release acceptance
is implied by this GREEN.

Commands and harness observations are in
[commands.txt](auth-transport-compatibility-r2/commands.txt). No npm install/audit,
Docker, Gaze secret, real credential, external message or production action was
used. Only the assigned new report and seven supporting evidence files remain.
