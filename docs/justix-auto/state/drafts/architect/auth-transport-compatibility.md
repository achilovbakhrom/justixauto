# Auth transport compatibility — bounded architecture proposal

Date: 2026-09-15. Status: proposed, not implementation or security acceptance.
Assignment/base: `task/auth-transport-compatibility` at
`476777139ffa6a470c7b3f6e79086c64f2dc7db5`. Independent exact-commit QA and
coordinator adoption/promotion are required before these aliases become tasks.

## P1. Authority and observed mismatch

The [approved T-058 contract](../../approvals/auth.md) adopts the exact
[auth wire](../contracts/auth.md), including A1–A5. This proposal preserves its
routes, DTOs, anonymous401, challenge/full distinction, one-time secret handling,
cookie ownership, Origin/CSRF checks, errors and recovery gates. It refines only
the missing technical transport representation and its implementation ownership.
T-002/OD-12 is unaccepted: no lifetime, entropy, proof syntax, password bound,
rate threshold, recovery delivery or factor-loss permission is selected here.
Money and revisions remain approved strings. Identity owns authentication and
its private transaction; edge forwards responses without inventing authority.
Neither transport validation nor a session read grants business permissions.

Observed at this base:

- T-032 `web/packages/api/src/client.ts` has one stateless fetch path, same-origin
  credentials/mode, disabled redirects, no-store request cache, path and reserved
  header controls, no retry. `ApiResult` has no response metadata; `errorStatuses`
  omits429. `response.json()` loses duplicate keys, invalid UTF-8 and lexemes.
  A generic `parseError` runs before the endpoint-specific generator validator.
- Reviewed T-640 `c69d52221287abecc4f1803ec7ed3532dce070f9` emits a structural
  `ContractTransport.request(ApiRequest)` and validates errors after the call.
  It explicitly rejects response headers/429 and only injects the three business
  revision/idempotency request headers. Generated Go `ContractResponse` currently
  carries Status, ContentType and Body, without response headers. Internal routes
  are Go-only with explicit mutualTLS metadata and an injected exchange.
- Go `contractRead` preserves `json.Number`, checks exact rational integral safe
  values before conversion, rejects duplicate names/invalid Unicode/trailing JSON.
  TS parsed-value validators cannot recreate raw information. This is a transport
  gap, separate from T-640's reviewed malformed-key/output-guard fixes.
- T-059 owns schema/migration leaves, T-641 only four generated/test leaves, and
  T-055 session package/provider/context-epoch leaves. None owns this whole gap.

The [Gaze audit](../../../reference/gaze-reference.md) supplies ports/adapters
and explicit composition patterns, not browser auth guarantees. The distinct
[CC audit](../../../reference/gaze-executor-cc-reference.md) does not supply them
either. Mock localStorage/demo sessions are interaction observations only; this
proposal does not reuse them. No Gaze credentials or application code were read.

## P2. One raw decoder and explicit numeric profiles

Add `decodeResponseJSON(bytes: Uint8Array, numeric: 'finite-json' |
'safe-integers'): unknown` in the shared API package. Both profiles share the
same grammar walk and object construction. No independent generated browser
decoder or second fetch path. The decoder consumes bytes from the shared fetch
response before any feature schema or metadata callback sees a value.

1. Decode UTF-8 fatally, preserving a leading BOM for rejection
   (`TextDecoder('utf-8', {fatal:true, ignoreBOM:true})` plus JSON grammar).
   Reject malformed, overlong/truncated encodings and surrogate UTF-8. Empty
   bytes are not JSON. Only JSON's four whitespace characters are permitted.
2. Tokenize nested objects/arrays/strings/literals/numbers through exactly one
   final value; reject trailing values, commas, leading-zero/plus/non-JSON numbers,
   controls and malformed escapes. Compare object names **after escape decoding**
   in each object's Set, including nested maps and `x` versus `\u0078`.
3. Reject lone UTF-16 surrogates in every decoded key and string; allow proper
   pairs and valid Unicode scalars, including literal U+FFFD. Do not normalize
   Unicode, fold case, or treat canonically equivalent distinct names as equal.
   Create own data properties safely for `__proto__`, `constructor`, etc.; no
   prototype assignment, accessors or inherited-field acceptance.
4. Keep each numeric lexeme until its selected rule finishes. `finite-json`
   preserves legacy generic callers' finite JSON-number behavior (including
   fractions), rejecting nonfinite overflow. It does **not** promise exact decimal
   precision. New generated operations always request `safe-integers`: inspect
   decimal digits/exponent exactly, require a mathematical integer in
   [-9007199254740991,9007199254740991], then convert to Number. `1.0`, `1e0`,
   `10e-1` and negative zero are permitted integer representations; `1.0000000000000001`,
   nonzero underflow and unsafe integers reject before IEEE rounding. Zero with
   an extreme exponent is still zero. Determine exponent/digit length before any
   large power/allocation; do not allocate 10^an-untrusted-exponent.
5. A numeric value never satisfies a schema string, including money/revision.
   All endpoint field bounds and cross-field rules still run after decoding.
   No coercion of digit strings, booleans, null, dates or ID casing.

This deliberately tightens malformed-input acceptance for old generic callers
(duplicates/Unicode/trailing/overflow), while preserving their valid finite-number
and `ApiResult` behavior. Existing callers need no request changes. Generated
safe-number selection aligns with actual Go mechanics rather than tightening its
currently accepted `1.0` to a new canonical-integer-only rule.

For bounded IO, implementation must expose immutable client construction limits
`maxResponseBytes` and `maxJSONDepth`, validated positive safe integers. Preserve
existing constructor callers through documented technical defaults of 8 MiB
decoded body and 128 container levels; generated Go checks use the same defaults.
These are **proposed parser resource bounds**, not accepted T-002 security/rate
policy or an endpoint payload entitlement. Check accumulated stream bytes while
reading, cancel on overflow, never trust Content-Length or allocate from it.
Auth deployment can impose smaller owner-approved schema/request limits; callers
cannot increase bounds in response to an untrusted header. If a real existing
fixture exceeds these bounds, reslice that compatibility change before adoption.

## P3. Stable shared transport contract and secret delivery boundary

Keep `request<T>(ApiRequest<T>): Promise<ApiResult<T>>`, all existing union
discriminants and receipt fields. Add429 to `ErrorStatus` and its `HttpFailure`
mapping; this is a reviewed additive status expansion requiring exhaustive-switch
checks. Do not put token/header/raw-body fields into `ApiResult`, ErrorReceipt,
generic errors, logs, query keys, caches, serialized receipts or storage.

The additive request interface is:

```ts
type HeaderRule = {
  readonly csrf: 'required' | 'forbidden';
  readonly retryAfter: 'required' | 'forbidden';
};
interface ResponseContract {
  readonly numeric: 'safe-integers';
  readonly errors: Readonly<Partial<Record<ErrorStatus, Schema<ErrorReceipt>>>>;
  readonly metadata: Readonly<Partial<Record<SuccessStatus | ErrorStatus, HeaderRule>>>;
  readonly auth: boolean;
}
type ValidatedResult<T> = Extract<ApiResult<T>, {kind:'success' | 'http-error'}>;
interface AuthExchange<T> {
  // Undefined cancels before fetch; empty object is a permitted safe GET.
  prepare(): {readonly csrf?: string} | undefined;
  // Synchronous, nonthrowing, one-shot; validates captured epoch before effects.
  accept(result: ValidatedResult<T>, metadata: {
    readonly csrf?: string; readonly retryAfterSeconds?: number;
  }): boolean;
}
// Optional fields on existing ApiRequest<T>:
// responseContract?: ResponseContract; authExchange?: AuthExchange<T>;
// Upgraded client exposes readonly responseContractVersion: 1.
```

`ResponseContract` is trusted generated configuration, not response-controlled
policy. It must have an exact rule for every declared status and no unrecognized
status/header directive. `errors` is the exact endpoint status-specific schema
map, not a permissive copy of generic ErrorReceipt. Schema parsers must include
the approved T-058 semantic predicates (e.g. code/status relation and session
revision/context relation) before accept. Generic parseError may additionally
enforce common structure; it cannot substitute for these predicates.

Single shared request pipeline:

1. Validate request/path/method/status contract and headers/body; preserve existing
   reserved headers. With auth=true require the capability/version and an
   AuthExchange. Immediately before fetch call prepare once; absence yields
   `invalid-request` and no fetch. Unsafe auth requires its validated CSRF value;
   GET must not send it. Forbid a competing supplied X-CSRF-Token header (including
   underscore/case aliases) when AuthExchange controls it. The token is inserted
   only into this request's Headers, never its body/DTO/parameters/result.
2. Fetch exactly once with existing same-origin/redirect/no-store controls.
   Retain redirected/final-URL rejection. Classify declared status; an undeclared
   status returns unexpected-status without metadata acceptance.
3. For non204 require the accepted JSON media type, consume bounded bytes, strict
   decode, then exact success schema or exact endpoint error schema. Retain
   legacy generic error fallback only when responseContract is absent. For204
   require no **observable** body bytes and pass undefined to the empty schema.
4. Validate all selected metadata and auth Cache-Control requirements. Only
   **after body and metadata both pass** construct ephemeral metadata and call
   accept exactly once, synchronously. No await or background callback between
   its epoch check and atomic memory update. The sink must validate everything
   before mutation and return true, or mutate nothing and return false.
5. False/throw returns `invalid-response` and suppresses body/metadata delivery.
   A throwing third-party sink cannot be rolled back by the transport; only the
   reviewed session sink is supported for auth. True permits returning the
   existing typed result; metadata is never attached to it. The caller must not
   independently render a result without the same epoch/ownership check.

Do not accept CSRF from a generic valid401 whose endpoint validator later rejects
the code/body, or from successful data before cross-field checks. Generated
wrappers may retain redundant validation for old non-auth callers, but it is not
the security boundary. The explicit `responseContractVersion === 1` runtime
check and matching required TypeScript property are mandatory for regenerated
contract-profile calls. TypeScript structurally accepts an old generic transport
with additional optional request fields; a type-only claim cannot prevent that
old client ignoring them. This is a cooperative interface marker, not an
unforgeable authorization capability; injected malicious fetch/schema code is
outside this client boundary.

No automatic retry for any status, invalid response, timeout or abort. Preserve
existing generic result shapes: caller method + result determines uncertainty.
For any dispatched unsafe request, transport-error, invalid-response or
unexpected-status means **unknown outcome**, including body-read failure and
invalid/missing metadata after a200/204. A validated503 AUTH_OUTCOME_UNKNOWN is
explicit uncertainty too. A valid error such as429 is a typed rejection only
under the endpoint contract, never an instruction to replay credentials. An
invalid-request produced before fetch is local non-dispatch. Session operations
record this distinction privately; never infer server rollback from validation.

## P4. Narrow header wire refinement and browser limits

These are proposed explicit T-058 technical refinements for coordinator adoption,
not claims they were already specified there:

- `X-CSRF-Token` is a nonempty opaque unpadded base64url-alphabet string matching
  `^[A-Za-z0-9_-]{1,4096}$`. This chooses transport syntax and a parsing ceiling,
  **not entropy, decoded size or lifetime**; those still require accepted policy.
  Do not decode or log it. Owner emits exactly one field, no padding/whitespace.
- `Retry-After` on429 is required canonical decimal delay-seconds
  `^(0|[1-9][0-9]{0,9})$`, <=2147483647. No HTTP-date, signs, decimals,
  comma-list, leading zeros or overflow. This finite wire ceiling is not a rate
  schedule, maximum practical UI delay or automatic timer/retry permission.
  Owner-approved policy must produce an encodable delay; invalid/missing is an
  invalid response. Keep it ephemeral in the auth sink, not error receipt fields.
- Auth responses must expose the singleton canonical `Cache-Control: no-store`
  (case-insensitive value after HTTP normalization); no conflicting directives.
  All non204 responses use `application/json`, optionally `charset=utf-8`
  (case-insensitive), with no other/duplicate media parameters. These narrow
  auth forms are represented in the schema/profile, not imposed on generic
  legacy callers' otherwise accepted JSON parameter forms.

Exact auth metadata matrix (all unspecified listed headers forbidden):

| Route/status | CSRF | Retry-After | Session handling |
|---|---|---|---|
| GET /session 200,401 SESSION_REQUIRED | required | forbidden | accept full/restricted/anonymous only from exact body |
| login200, mfa/verify200, revoke-all200 | required | forbidden | atomic new body and token acceptance |
| enrollment/{id}/confirm200 | required | forbidden | one-time recovery codes; no SessionRead yet; explicit subsequent GET /session |
| logout204, recovery/complete204 | forbidden | forbidden | clear memory; no inferred authentication |
| mfa/challenges200, mfa/enrollment200, recovery/request202 | forbidden | forbidden | no cookie rotation newly introduced here |
| any declared429 RATE_LIMITED | forbidden | required | retain no newly supplied token; no retry |
| all other declared errors | forbidden | forbidden | no token adoption; CSRF/session errors invalidate usable binding |

GET401 SESSION_EXPIRED is not silently redefined as SESSION_REQUIRED: if the
concrete auth schema needs that additional branch/header rule, obtain an explicit
contract refinement first. Existing approved GET anonymous401 is SESSION_REQUIRED.
No header issued by a non-rotation error silently rotates client authority.
The success enrollment-confirm header follows its approved fresh full-cookie
rotation even though its response revision is a security revision.

Fetch Headers merges ordinary repeated field values using comma separation and
normalizes whitespace; response bodies expose decoded body bytes, not HTTP
framing or compression originals. Browser responses filter Set-Cookie, and204
has a null body. Thus the client can reject comma-combined CSRF/Retry-After or
malformed observable media/cache values, but cannot prove raw singleton field
count, reject whitespace already removed, inspect cookie attributes, detect
illegal bytes the HTTP stack discarded, or undo browser cookie updates. Owner
and edge wire tests must verify singleton raw headers, exact cookie flags and
clearing, cache directives, no body/framing on204 and no secret logging. Node's
synthetic Response does not prove browser forbidden-header filtering. These
limits follow the [Fetch Standard](https://fetch.spec.whatwg.org/#headers-class)
and [body model](https://fetch.spec.whatwg.org/#concept-body); fatal decoding uses
the [Encoding Standard](https://encoding.spec.whatwg.org/#interface-textdecoder).
The delay-seconds-only subset deliberately narrows the alternatives in
[RFC9110 §10.2.3](https://www.rfc-editor.org/rfc/rfc9110.html#section-10.2.3).

## P5. Memory-only session acceptance and concurrency

Introduce a non-React controller in the API package (AT-EPOCH below), depending
only on the new shared interfaces and injected typed classification callbacks.
It does not import generated identity types or the not-yet-created session
package. T-055 later supplies adapters using T-641/T-643 validators and connects
this controller to React/query clearing. No controller↔generator import cycle.

Use local monotonically increasing safe epochs plus unique per-request tickets;
epoch exhaustion fails closed. Epoch is **not** a server revision or cross-tab
ordering proof. Capture the epoch, ticket, operation purpose and current binding
before dispatch. A ticket is one-shot. AT-EPOCH serializes operations capable of
issuing/rotating/clearing cookies **within this controller**, including session
acquisition reads and handshake writes; concurrent attempts return local busy
without fetching. It never queues/retries a secret. Any explicit logout, new
login intent, context switch/revocation notification or uncertainty invalidates
the old epoch, aborts reads, hides protected content/one-time secrets, clears token
and drops pending result acceptance before new work. The current logout command
may use a captured old token in its private one-use request closure; it cannot
expose it back to the cleared state. Cancellation does not prove non-dispatch.
Allocate that logout ticket in the new epoch after invalidating older tickets.
Keep the local in-flight exclusion until the prior fetch settles, even after
abort/epoch invalidation; never treat abort as completed server execution. If
logout cannot dispatch because an older auth exchange is still in flight, clear
visible memory immediately but report local busy/uncertain binding; do not queue
the secret or claim logout success. A later explicit action must reacquire/check
the binding as necessary. Clear the in-flight slot by its exact ticket only, so
an old finally callback cannot release a newer exchange's slot.

Accept checks the ticket/epoch/operation and expected state before changing
anything. Anonymous SESSION_REQUIRED401 installs only anonymous CSRF, never a
user. Restricted ChallengeResult/EnrollmentRequired has no protected admission.
Full SessionRead may install validated session context + token together.
EnrollmentConfirmed retains recovery codes only in the current one-time view
closure, sets a context-unresolved state and permits an explicit GET /session;
it does not manufacture context from its security revision. EnrollmentSecret is
similarly confined to its current one-time view. No React Query auth mutation
cache, persisted atom, telemetry/error breadcrumb or generic command receipt may
retain those bodies or tokens; ordinary ApiResult is not a safe cache directive.
Clear all such closures on navigation/unmount/logout/epoch invalidation; no claim
of guaranteed physical memory zeroization in JavaScript.

On lost/invalid/stale dispatched auth-write response, invalidate usable binding
and enter uncertain-session state. Do not replay login, proof, enrollment, recovery
or logout automatically. A **user-initiated** serialized GET /session can acquire
the server's currently admissible cookie binding for further action. This read
does not recover a lost enrollment secret/recovery codes, prove a failed logout
did not occur, or certify another operation's commit. A successful logout204
clears memory and never reacquires a session automatically. A subsequent explicit
sign-in may begin anonymous acquisition; the server determines what is live.

Epoch discard prevents stale application-state/secret adoption only. An older
response may already have written HttpOnly cookies despite abort or local discard;
another tab or independent client can race too. Server rotation must atomically
invalidate old bindings, verify challenge/security state inside the owner
transaction, and reject stale/revoked cookies; logout/recovery/revoke-all must
retain their approved revocation scope. These are owner task obligations, not
guarantees supplied by serializing fetch. Server/edge concurrency tests must show
a stale Set-Cookie cannot revive a revoked binding; stale cookie overwrite may
still log a user out or require reacquisition. Do not invent global cross-tab
mutual exclusion or claim immediate logout of an independently concurrent new
login beyond T-058's presented-session revocation scope. Unresolved stronger
logout/rotation semantics require a separate owner contract, not a client fix.

T-055 retains its approved context revision CAS and ALL reset contract. It hides
old PII/dialogs and clears/cancels shared caches synchronously on context changes;
late old responses cannot render. Cross-tab messages contain revision only as
already approved, never token, session DTO, identity or one-time codes. Treat a
message as an invalidation hint, not new authority; even equal/lower revision
notifications may invalidate local binding, while outgoing current context comes
only from live validated reads. Logout/invalidation notification encoding and
revision availability must follow T-070; until specified, no fabricated revision
for an anonymous session and no claim of complete cross-tab logout propagation.
No new cross-tab schema is hidden in AT-EPOCH. Server revocation still applies.

## P6. Generator and owner wire handoff

Extend the deterministic generator profile explicitly; retain fail-closed
unsupported-feature handling and all reviewed output guards. No general-purpose
OpenAPI header/security runtime is implied.

- Recognize only inline response Header Objects for the three fixed names above,
  canonical case, `required:true`, description and exact schema forms. Optional
  security-bearing response headers, unknown header names, `$ref` header indirection,
  alternate encodings and undeclared response statuses fail generation. Absence
  from a declared auth status means forbidden. Infer no policy from a header name
  alone: auth-cookie behavior is explicit operation extension
  `x-justix-auth-transport: 1`, forbidden on internal routes and unsupported owners.
  Require Cache-Control on each auth response; Retry-After exactly on429 and
  X-CSRF-Token according to the adopted concrete operation matrix.
- Permit the exact X-CSRF-Token request Header Object on unsafe auth operations
  with required:true and the same token syntax. Preserve its schema declaration
  and generate request-security metadata, but deliberately obtain its value from
  AuthExchange.prepare, **not a generated parameter DTO or browser cookie read**.
  Reject it on safe GET/internal routes. This transport-injected-header exception
  is an explicit generator-profile refinement, not removal of required CSRF.
- Generated browser wrappers pass exact error schema maps and status header rules
  into shared request before a sink can run; all regenerated wrappers select
  safe-integers and require responseContractVersion1. Keep existing method names,
  paths and ordinary parameter/body/signal positions. Add only an optional last
  AuthExchange argument after signal for auth operations; it is required at
  runtime when that operation carries auth transport metadata. No caller token
  option, hidden global state or secondary fetch. A missing capability/port
  returns invalid-request before dispatch.
- Generated Go keeps namespace isolation and injected ContractExchange. Add an
  optional `Headers map[string][]string` to ContractResponse for reviewed auth
  response metadata and an explicit request-scoped auth port to auth methods
  (a variadic final port permits old non-auth calls to remain unchanged). The
  auth port supplies request CSRF and synchronously accepts validated result plus
  typed ephemeral metadata; never put CSRF into generated DTO/result structs.
  Reject ambiguous case-insensitive duplicate keys/multiple values. Go can
  observe multiplicity preserved by its adapter; adapters must not collapse it
  before validation. Existing ContentType stays authoritative for legacy calls;
  auth requires agreement with singleton Headers Content-Type for non204 and
  rejects conflicting values. A new interface-version marker on the exchange
  is required for auth; no default assertion that an old exchange handles cookies.
  Internal mutualTLS methods remain Go-only and do not require an auth port,
  CSRF, browser cookies or response cache metadata. Actual cookie jars/TLS/IO
  remain explicit composition responsibilities, not generated authentication.
  The concrete added auth port is generated per operation:
  `Prepare(context.Context) (csrf string, ready bool, err error)` and
  `Accept(OperationResult, AuthMetadata) bool`; the last auth method argument is
  `ports ...OperationAuthPort`, with exactly one required for auth operations.
  Empty CSRF is valid only for GET. `AuthMetadata` has optional CSRF and optional
  RetryAfterSeconds with explicit presence; operation result has neither.
  Auth exchanges must additionally implement `ResponseContractVersion() int`
  returning1, checked before Exchange. Keep all these identifiers namespaced.
- Generated Go and TS enforce the same adopted byte/depth/header/numeric limits
  and fixture semantics. Typed429 remains the endpoint error shape in both;
  Retry-After is only separate ephemeral metadata. All status/body/header
  validation finishes before either auth port. Do not generate a raw-header dump.

Structural JSON Schema validation alone cannot enforce T-058's session relations.
Make that missing ownership concrete through AT-AUTH below, **after T-641**.
Use mandatory typed feature bindings, not an assertion language in OpenAPI:
T-059 declares `x-justix-auth-semantics: 1` on the auth document, restricted to
Identity and paired with the transport operation marker. The generator rejects
missing/unsupported declaration on auth-cookie operations. This declaration
requires a binding; it does not claim a structural schema proves the rules.

AT-GEN emits a namespaced `AuthSemantics` interface with one synchronous typed
`validate<Name>(value: Name): void` method for each concrete named schema in the
auth document (Go `ValidateName(value Name) error`). All methods are required;
no optional callback, default true, fallback, async validator or endpoint-only
postprocessing. Generate `createAuthValidators(bindings)` in TS and a Go
constructor accepting the required semantics binding for an auth client. Runtime
construction rejects missing/nonfunction methods (Go nil bindings, including typed
nil, reject); construction of legacy non-auth clients stays unchanged. Bindings
have no HTTP, storage or token interface. Generated validators structurally walk
first, call the typed binding on every named schema/ref occurrence and then check
union cardinality including semantics. Full request and response validation uses
this bound factory, not an unbound structural parser. Standalone generated auth
structural parsers are explicitly named `NameStructuralSchema` in TS and
structural Go DTO marshal/unmarshal remains documented as structural only; they
cannot be passed off as complete auth validators. The generated typed auth client
requires bound validators before dispatch or response metadata delivery.

Keep the existing direct operation entry point positions for parameters/body/signal;
auth-specific generated operations live on the bound client returned by a factory.
Calling an unbound auth entry point must fail before fetch rather than silently
using structural validation. This is an explicit auth-only generated interface
refinement; no concrete auth client exists yet to preserve. Public non-auth and
internal mTLS signatures remain unchanged. For Go, auth construction is
`NewAuthContractClient(exchange, semantics)` and auth methods retain the typed
request-scoped auth port described above. Missing semantics is an error before
Exchange; ordinary `NewContractClient(exchange)` must reject auth operations.
The Go generated helpers must expose a bound validator for owner HTTP inputs as
well; raw DTO unmarshal alone is not admissibility validation.

AT-AUTH owns the concrete stateless semantic binder in both languages. It imports
T-641 generated DTO/interface types, never the other way around. Its exact rules
are T-058's canonical lower-case nonzero IDs; signed-bigint string revision bounds
and positive revision; exact code/status/error field-path relations with operationId
absent; equal SessionRead/context revisions; null company implies ALL/[]; ALL has
no branches, SELECTED nonempty unique IDs; unique role/company IDs and permission
keys; no authenticatedAt when enrolled=false; non-whitespace reason; and the
closed challenge/enrollment/recovery variant constraints. Structural checks remain
mandatory too. Endpoint error schemas and the generated operation context must
supply the concrete approved status/code and allowed field-path sets; binder
selection is based on trusted operation metadata, never a response field selecting
its own validator. Add required `validateError(operationId, status, value)` and
Go equivalent to AuthSemantics specifically for that operation-dependent check,
called before the sink even after a named error schema passes.

Policy-dependent constraints use a validated immutable explicit policy binding
(`fixture-only` for synthetic tests), whose constructor rejects absent inputs.
No browser-provided policy authorizes the owner; browser validation is advisory
and must not evaluate server freshness/membership/admission from displayed data.
No policy serialization/new API is added here. Until an accepted way of providing
release client field constraints exists, build/test only with synthetic policy;
do not claim browser production activation. The owner independently uses its
accepted AuthDeploymentPolicy. Missing release policy blocks that capability,
not structural generation. No new field bounds, recovery-code syntax or permission
catalog entries are invented by this binder.

This is a **fifth bounded alias**: inspection revealed that fitting dual-language
feature semantics into AT-GEN would hide separate ownership. T-641 can generate
and test mandatory inert bindings with explicit synthetic validators before the
concrete binder exists; construction/use without those validators must fail closed.
AT-AUTH follows T-641; actual owner and session consumers follow AT-AUTH. There is
no generator-to-feature import and no graph cycle.

T-059 can author the JSON-form OpenAPI3.1 auth schema and migration after its
existing prerequisites; it need not wait for transport code. It **must consume
this adopted wire refinement** before freezing fixtures/header declarations,
including status-specific semantic checks not expressible by T-640's structural
profile alone. Keep unsupported schema constraints visible; reslice generator
support or explicit validator hooks rather than weakening T-058 to fit it.
T-641 remains blocked until the new profile and concrete config entry exist.
Root alone adds the auth entry `{input, goOutput, tsOutput, goPackage, owner,
namespace}` to `tools/contracts-generator.config.json`; registration is a
reviewed integration change before the exact T-641 QA commit, not worker scope.

## P7. Exact bounded implementation aliases and dependency delta

Aliases below are proposed new tasks, each max4h with independent exact-commit QA;
reslice before assignment if an implementation estimate exceeds that bound.
No implementation IDs are reserved by this draft. No source changes here.

| Alias | Direct prerequisites | Exclusive leaves / serial successor |
|---|---|---|
| AT-JSON | T-032 | new `web/packages/api/src/responseJSON.ts`, `web/packages/api/src/responseJSON.test.ts`; scanner/profiles/limits only, no client edits |
| AT-HTTP | AT-JSON, T-058, T-032 | serial successor `web/packages/api/src/client.ts`, `web/packages/api/src/client.test.ts`; one fetch path, exported interface types,429, metadata validation/sink and limits |
| AT-GEN | AT-HTTP, T-640 | serial successor `tools/generate-contracts.mjs`, `tests/contracts/generation_reproducibility_test.go`; exact supported profile, mandatory typed semantic-binding interfaces, Go/TS transport parity fixtures inside test leaf |
| AT-AUTH | T-641, T-058 | new `web/packages/api/src/authValidation.ts`, `web/packages/api/src/authValidation.test.ts`, `services/identity/contracts/openapi/auth_semantics.go`, `services/identity/contracts/openapi/auth_semantics_test.go`; concrete typed feature binders and synthetic parity tests |
| AT-EPOCH | AT-HTTP, T-058 | new `web/packages/api/src/authEpoch.ts`, `web/packages/api/src/authEpoch.test.ts`; serial successor `web/packages/api/package.json` **only** to add explicit `./authEpoch` export; non-React controller accepts injected classifiers, no generated imports |

Root controls any lockfile/root workspace changes; the export-only API manifest
change introduces no dependencies. New decoder is internal; client.ts exports
its public types as necessary without a decoder subpath or extra index file.
Root alone registers `./authValidation` and the concrete generated auth export
in `web/packages/api/package.json` for AT-AUTH/T-641 integration, preserving
AT-EPOCH's explicit subpath and all existing entries. This does not authorize
either worker to edit that shared manifest; root stages that concrete change
before the affected exact-commit QA. No additional npm dependency is proposed.
AT-EPOCH is located in the already integrated API workspace specifically because
T-055 owns the new session package manifest and must not be preempted. The two
new source pairs can remain independent of T-059/T-641 fixtures.

Proposed existing dependency additions: T-641 + AT-GEN; T-055 + AT-EPOCH and
AT-AUTH; T-060 + AT-AUTH. Retain T-641's T-640/T-059 and T-055's
T-032/T-070/T-643; T-641 is a transitive T-055 prerequisite through AT-AUTH.
Do not replace
them or create a cycle. T-059's T-058/T-024/T-929/T-931 stay unchanged; its schema
must cite the adopted handoff before assignment, not an irrelevant frontend
completion barrier. T-055 is serial owner of its existing SessionProvider,
contextEpoch tests and session package.json; it imports `@justixauto/api/authEpoch`
and generated auth/context clients plus the concrete auth binder, registers the package through root's normal
manifest/lock gate, and integrates cache clearing/current result acceptance.

T-059 and T-641 retain their existing exact leaves; no handwritten alternate
auth transport or generated DTO patch. T-060/owner HTTP/auth verification and
T-592 release closure must cite P4–P5 raw wire/cookie concurrency requirements
in their existing relevant scopes. Coordinator must inspect/reslice their exact
leaves before assigning an uncovered server/edge check; this proposal does not
quietly assign a new server file or add all owner tasks as transport dependencies.
Browser UI parity/notice supplements remain T-054/T-361 and relevant frontend
tasks. T-002 and recovery capability gates still block release behaviors only.

## P8. Verification floor and executed feasibility

[Result/reproducer](../../../dev/results/auth-transport-compatibility.md) records
40 Node checks, the 19-case actual generated Go decoder parity probe and strict
TypeScript interface compilation. These are isolated exploratory probes against
the actual base APIs, **not implementation tests or browser/session guarantees**.
The proposal scanner is not installed application code. No dependency install,
network request to an auth service, cookie secret or production action occurred.

Every alias must add meaningful negative tests in its owned test leaf:

1. Byte splits inside multibyte Unicode, malformed UTF-8/BOM/lone surrogate keys
   and values, escape-equivalent duplicates at depth, prototype names, trailing
   values, exact-integer exponent/fraction/underflow/overflow, byte/depth limits,
   absent/false/null and string money/revisions; legacy finite fractions preserved.
2. Anonymous401 body code wrong but generic receipt valid + valid token: zero
   sink calls. Valid endpoint body + missing/comma/invalid token also zero.
   Cover every matrix cell, stale ticket, sink rejection,429 missing/malformed
   delay/cache headers, malformed observable204 body, body-stream failure,
   invalid URL/redirect/foreign origin/reserved header, abort before/after dispatch.
   Assert one fetch, no retries/raw diagnostics/token-bearing generic results.
3. Generated output strict TS compile plus execution through the **actual upgraded
   shared client**; actual generated Go tests through its injected adapter, same
   synthetic raw/metadata corpus, old non-auth callers, internal-only mTLS methods,
   wrong capability/port, forbidden auth header combinations, exact code/status
   schema failures, deterministic regeneration/check drift and existing guard suite.
4. Deferred response orderings: read→logout, anonymous read→login, verify→logout,
   context switch→late response, old epoch/new epoch with same server revision,
   two attempted rotations, accepted/unknown enrollment-confirm, recovery204,
   cross-tab revision hint. No stale token/PII/one-time secret is accepted; no
   unconditional reset from a late callback and no automatic reacquisition.
5. Later real browser/owner QA must separately test HttpOnly filtering, cookie
   flags/deletion and stale-cookie races across tabs; script-visible mocks cannot
   establish those guarantees. Server failure injection must retain T-058 atomic
   Identity transaction and unknown-commit handling, with no broker call inside.

Open boundaries are explicit: accepted token entropy/lifetimes/rate policy;
recovery proof/delivery; stronger cross-tab logout semantics if desired; T-070's
revision-only invalidation details; exact concrete auth schema semantic predicates;
and owner/edge wire activation. None may be inferred from passing parser probes.
