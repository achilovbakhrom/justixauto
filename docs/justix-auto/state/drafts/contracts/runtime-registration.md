# T-746 — Incremental runtime registration contract proposal

Status: **PROPOSED; coordinator approval and independent exact-commit QA pending.**
Base: `537b662af02db56611bf6377fc42b16086a037dd`. Date: 2026-09-15.
This is a documentation contract, not implemented runtime behavior or a new ADR.
The coordinator will promote this architect draft to the task-owned
`state/drafts/contracts/runtime-registration.md` before QA. Relative links below
work from either directory.

## Authority and reference evidence

Confirmed constraints come from [architecture §§2–5,7–9](../../../architecture.md),
[HTTP contracts](../../../contracts/http-domain.md),
[cross-owner protocols](../../../contracts/cross-owner.md), and
[T-746](../../../dev/tasks/T-746.md). Seven private owners, database-free edge,
explicit dependency injection, local transactions, outbox/inbox, full replay,
four independent React apps and fail-closed policy gates remain unchanged.
[T-001 approval](../../dependency-lock-approval.md) controls dependencies; this
proposal adds no library, transport, mutable business store or dependency pin.

Observed reference: [Gaze audit §2](../../../reference/gaze-reference.md)
shows explicit `cmd` construction and projection subscriptions. The
[CC audit §§2–3](../../../reference/gaze-executor-cc-reference.md) shows small
ports and event-to-command adapters, but also app-to-adapter coupling, pre-commit
publication and incomplete confirmation handling. Those defects are not this
contract's guarantees. Registration cannot supply delivery reliability by itself.

Observed prototype evidence is limited to the four app entries and contextual
controls in [mock-map](../../../mock-map.md). No browser or mock runtime was
executed for this proposal. The typed discovery convention below is a proposal,
not a pattern claimed to exist in the prototype or either Gaze revision.

## 1. Ownership and construction boundary

Each owner has three adapter-local registries: `adapter/http/routes.go`,
`adapter/projection/registry.go`, and `adapter/amqp/registry.go`, initially owned
by T-580…T-586. Composition roots remain `cmd/{api,projection,worker}/main.go`.
Leaf tasks such as [T-747](../../../dev/tasks/T-747.md) add only their assigned
registration/projector/subscriber files in those same packages. Existing Go
package compilation discovers those files; no blank imports, generated list of
future features, reflection plugins, shared business registry or root edit is
needed for each completed leaf.

Registry definitions and construction factories are outer adapter code.
`domain`, `app`, `port`, public contracts and shared value packages must not
import or consult a registry. Keep the existing T-003 import checks: no new
`services/<owner>/runtime` layer, adapter import from app, shared package import
of owner implementation, or owner import of another owner's private package.

Each root constructs its own configured technical resources and explicitly
passes a typed dependencies value into factory binding. An adapter dependency
struct may contain the owner's concrete database handle and approved client
adapters, but application constructors receive narrow owner ports only.
Existing owner `port/dependencies.go` remains infrastructure-free. A leaf can
construct its typed repositories and handlers from the supplied resources;
this pure outer assembly is delegated composition, not domain construction of
adapters. No `Get(string) any`, global dependency map, type assertion lookup,
ambient request scope, hidden environment lookup or application access to the
registry is permitted. Do not predeclare fields/imports for absent business
modules. New technical capabilities require separately assigned dependency
contract ownership; a missing dependency is not permission to inject a fake.

Factories must not open connections, migrate, query a store, subscribe, publish,
start goroutines, seed records or call business commands. Root-controlled startup
performs connection/compatibility checks and starts validated runners explicitly.
Feature-local handlers retain all owner authorization, party/state/revision and
policy checks. Registry membership grants no business permission.

## 2. Inert typed catalog and two-phase activation

The following Go signatures are proposed contracts, not existing package APIs.
They may be repeated as owner-local types in the assigned registry files rather
than adding an unowned shared runtime package. `D` and `F` are concrete typed
adapter dependencies and prepared outputs; neither is a string-keyed container.

```go
type FeatureID string // owner/category/feature; stable, nonempty
type Descriptor struct {
    ID       FeatureID
    Requires []FeatureID // required installed capabilities, not task IDs
    Consumers []ConsumerClaim // inert claims, empty for HTTP-only features
}
type ConsumerKind string // exactly "projection" or "process"
type SchemaClaim struct {
    EventType string
    Version   uint32
}
type SubscriptionClaim struct {
    ContractID   string // stable approved subscription contract/version ID
    SourceOwner  string
    StreamID     string // approved stream selection, not a feature-invented filter
    Exchange     string
    Queue        string
    RoutingKeys  []string
    Schemas      []SchemaClaim
}
type ConsumerClaim struct {
    Name         string
    Kind         ConsumerKind
    Subscription SubscriptionClaim
}
type Factory[D, F any] struct {
    Descriptor Descriptor
    Bind       func(D) (F, error) // pure constructor, never starts work
}
type Catalog[D, F any] interface {
    Add(Factory[D, F])             // inert; retains validation errors
    Freeze() ([]Factory[D, F], error) // copies, validates, sorts; seals catalog
}
```

`Add` is the sole permitted feature `init()` action: append a literal descriptor
and constructor function reference to its same-package catalog. It must not call
the constructor, evaluate runtime configuration or panic on duplicate registration.
Invalid additions accumulate an error so startup reports a deterministic failure.
Late additions after freeze are rejected; no hot registration. Catalog copies
descriptor slices on entry and return, owns its storage and is safe against
concurrent misuse. Independent tests use fresh catalog instances, not resets of
production package globals. Order never depends on source-file or init ordering.

Consumer claims are literal nonsecret metadata from the approved subscription
contract. They exist before Bind and include every declared consumer, including
consumers belonging to other process roles. Deep-copy nested routing/schema
slices and validate required fields, supported kind/owner/schema keys and the
approved contract ID. A deployment-specific name must use one declared symbolic
name resolved by the same root configuration for every role before comparison;
factories cannot invent a different queue/exchange at bind time. This convention
does not approve new broker topology or subscription scope. Claims outside the
approved topology/feature subscription contract fail validation.

Each root unions these frozen claims across the owner's projection and process
catalogs without calling either role's Bind. Feature IDs P and Q are insufficient:
if P and Q both claim consumer name X, startup fails even when only one role is
selected. Compare full subscription claims and reject conflicting ownership of
binding/checkpoint identities. Any intentional physical queue sharing must already
be defined by the approved ingress/dispatch contract; separate process roles must
not silently become competing consumers for independent required effects.

Conceptual leaf: `init() { httpFactories.Add(credentialsFactory) }`, where
`credentialsFactory.Bind` constructs the completed credentials HTTP adapter with
explicit dependencies. There is no placeholder consumer for credentials if its
approved contract emits/subscribes to no integration fact.

Startup is all-or-nothing for its selected installed capabilities:

1. Freeze catalogs, sort stable IDs and validate unique IDs, nonnil constructors,
   owner/category, dependency existence and acyclic dependency graph. Selection
   uses explicit local configuration/assembly; an unknown requested ID is fatal.
2. Validate approved deployment capabilities and required mechanics. Baseline
   development may select an empty business set; a release profile must require
   its declared delivered capabilities. Never infer release completeness from
   an empty set or enumerate all future backlog features as runtime requirements.
3. Validate configuration, create root resources and check compatible owner
   migrations. Resources opened before a failure are closed by the root. Never
   perform destructive automatic migrations or fallback to another owner's DSN.
4. Bind the selected factories into an isolated prepared plan. Require declared
   dependencies even if the missing feature was simply not selected. Reject nil
   required ports, typed-nil implementations, nil handlers, constructor errors,
   duplicate routes/consumers and invalid references before mounting anything.
   Match each selected factory's bound consumer set exactly to its own frozen
   claims: name, kind, contract/source/stream, resolved exchange/queue, normalized
   routing-key set and schema-key set. No extra, missing or changed consumer is
   permitted. Reordering may normalize deterministically; duplicates are errors,
   not silently deduplicated. A selected factory cannot emit another feature's
   claim. The root creates actual subscriptions from the matched immutable plan;
   factory outputs cannot append opaque extra bindings or subscribe directly.
5. Freeze the prepared plan, mount validated handlers and start approved runners
   under a root context. Startup failure cancels and joins started runners and
   closes owned resources in reverse order; readiness stays false. Routine stop
   drains using the approved mechanics; registration adds no new retry policy.

Factories cannot mutate the live router/broker directly. A constructor never
registers another factory. Failures cannot leave a partial business API serving
while readiness reports success. A liveness endpoint may exist during startup;
readiness covers required technical dependencies and selected capabilities only.
Configuration names, feature IDs and missing capability diagnostics are operator
data; HTTP errors must not expose DSNs, credentials or private scope details.

Each owner root inspects immutable metadata from all three owner-local catalogs
to catch namespace collisions even across separately deployed binaries; only its
role's selected factories are bound and started. Catalog inspection is pure and
does not create a projector in the API process. `Requires` denotes a construction
prerequisite within the selected process role, not proof another binary is alive.
Cross-process/owner dependency readiness uses the already approved transport and
mechanics checks. The registry cannot infer remote service health from a local
descriptor. A duplicate installed descriptor is invalid even if a profile would
otherwise deselect one copy.

## 3. Prepared HTTP and consumer plans

Example typed HTTP output at the adapter boundary:

```go
type RouteSpec struct {
    Method string
    Path   string
    Handle echo.HandlerFunc
}
type HTTPFeature struct {
    Routes []RouteSpec
}
// Owner HTTP Dependencies is a concrete adapter-local struct.
// Factory[Dependencies, HTTPFeature] binds it without mounting routes.
```

Before `Echo.Add`, validate the entire plan against the root's reserved health,
readiness and transport routes. Require explicit HTTP methods, approved public
`/api/v1/<owner>/...` or internal `/internal/v1/<owner>/...` prefixes and the exact
named feature API contract. Auth challenge exceptions keep their existing
protocol. Reject query/fragment, encoded separators, dot segments, repeated
separators and unexpected trailing slash; no permissive path cleanup. Canonical
collision keys use upper-case method and positional parameter shape: `/users/:id`
and `/users/:userId` collide. Reject duplicate keys even for identical handlers.
Static and parameter alternatives may coexist only with documented deterministic
router precedence and tests; feature wildcards/catch-all routes are forbidden.
Root-controlled HEAD/OPTIONS behavior must be included in collision validation,
so automatic methods cannot silently overwrite explicit routes. Case and slash
redirect behavior must match the root's fixed routing policy.

HTTP descriptors never install their own public authentication bypass. Root
transport guards run before handlers; the owner performs live action/object
authorization. Internal handlers retain mTLS caller/action/purpose admission.
Edge dispatches only approved owner prefixes and keeps no business registry/DB.
Absent or unknown API routes return the common 404 error, never demo success,
another owner handler, HTML fallback or a synthesized command receipt. A known
policy-gated action returns the existing `POLICY_UNRESOLVED` 403 contract.

Consumer and projector output must carry a stable owner-local `consumerName`,
kind (projection or process), approved source/stream subscription contract,
schema descriptors and a typed handler accepted by the existing inbox machinery.
Use a concrete envelope/transaction context from the approved mechanics; do not
introduce `any` payload dispatch or pass AMQP acknowledgement capability to a
domain handler. Projection and process outputs share one consumer-name namespace
per owner database. Reject a duplicate name across either catalog, conflicting
bindings/checkpoints, nil handlers and an undeclared/unsupported schema before
starting consumers. Multiple consumers may observe one event only when they
have distinct approved identities and independently justified effects.

One consumer that handles several types registers one dispatcher with the full
authorized integration sequence. Irrelevant types deliberately no-op and still
checkpoint; leaf registration must not filter away required sequence positions.
Projectors use historical payloads only and cannot execute business commands.
Process subscribers call the application through its approved admitted durable
operation ports. The inbox + effect + checkpoint transaction commits before ACK;
quarantine must persist before malformed originals are ACKed. Outbox, lease,
retry, lost-reply and redrive behavior remain ADR-03/05 mechanics, not new factory
promises. Reusing a consumer name with changed meaning requires a separately
reviewed migration/checkpoint/rebuild plan, not registration-order replacement.

## 4. App-local discovery and contextual features

T-587…T-590 own each app's `src/routes/registry.ts` and `src/app/App.tsx`.
Completed binding leaves export `featureRoutes` from their assigned
`src/features/<feature>/route.tsx`; that file exists only when its own imports
and binding are implemented. The app registry can discover them as follows:

```ts
import type { ComponentType } from 'react';

type AppID = 'realization' | 'financing' | 'insurance' | 'admin';
type FeatureRoutes = Readonly<{
  id: string;
  app: AppID;
  requires: readonly string[];
  pages: readonly Readonly<{
    id: string;
    path: string; // app-relative; basename belongs to the app root
    Component: ComponentType;
  }>[];
  slots: readonly Readonly<{
    parentRouteId: string;
    name: string;
    Component: ComponentType;
  }>[];
}>;

const modules = import.meta.glob<{ featureRoutes: FeatureRoutes }>(
  '../features/*/route.tsx', { eager: true },
);
// Registry validates modules in sorted path order, then constructs routes.
// This snippet describes the convention; validation is not implemented here.
```

Exports are inert component references and metadata: no fetch, session mutation,
browser persistence, subscription or business action at module evaluation.
Component dependencies can be lazy imports only of existing completed files;
lazy loading does not permit missing source imports or silently skip build errors.
The glob is app-local and literal; never scan sibling apps or shared packages
for app routes. Type checking plus runtime structural checks validate exports;
generic glob typing alone does not prove shape, uniqueness or dependency safety.

Validate nonempty unique feature and route IDs, `app` equals the current app,
local relative paths, dependencies present/acyclic and unique normalized route
patterns including parameter-name equivalence. Feature routes cannot claim the
root's fallback/auth boundary or another app prefix. A root index page is explicit
and unique. Parent route IDs and slot names must be defined in that page's approved
contract; reject missing parents and duplicate `(parentRouteId, name)` slots.
Forms and financing/documents/payment controls bind to approved contextual slots,
not new sidebar entries or duplicate copies of their parent page. A feature leaf
may export only slots with an empty page list. Slots get the parent's scoped
context through the existing component/provider contract, not a global locator.

Navigation is built only from validated, present pages with approved M-ID/menu
metadata; export presence alone never creates a new menu item. Permission/session
visibility derives from the existing session contract and never substitutes for
server authorization. Missing optional leaves need no stub, active menu, empty
success screen or static import. A valid zero-feature development shell renders
its scoped unavailable/not-found state; a declared required but absent feature
is a build/boot validation failure. Malformed present exports are fatal rather
than silently ignored. They do not disable unrelated applications' builds.

Root providers remain QueryClient → session boundary → app shell → route boundary,
with overlays/recovery inside authentication. Prefixes stay `/`, `/finance/`,
`/insurance/`, `/admin/`; the owner/API spelling remains `financing`. Deep links,
back/forward and prefix-only HTML fallback must work with one completed feature.
API errors/assets never receive HTML fallback. Registry configuration cannot
enable a policy decision, confirm missing visual states, create demo identities,
share query caches across apps or bypass context epoch invalidation.

## 5. Required implementation validation inventory

These are acceptance obligations for runtime/leaf tasks, **not tests executed by
this documentation task**. Use only synthetic values and isolated infrastructure.

| Case | Required result |
|---|---|
| Empty baseline owner and app; add one real leaf | Technical readiness and scoped fallback work first; completed slice runs with no absent imports or shared-root edit |
| Import/freeze/bind with I/O tripwires | No connection, migration, command, publish, goroutine or subscription before explicit root startup |
| Two fresh catalogs and repeated snapshot reads | No shared test state, mutation leaks or init-order-dependent activation |
| Duplicate ID, HTTP parameter alias, automatic method collision, consumer name across kinds | Deterministic error before route exposure or consumer start |
| Projection P and process Q have different IDs but both claim consumer X; only one role selected | Inert union rejects X before any Bind executes |
| Bound consumer renamed, omitted or added; queue/exchange/routing/schema changed from claim | Selected factory rejected before subscription, including cross-feature claim substitution |
| Missing/nil/typed-nil port, factory error, missing required feature, dependency cycle | Startup fails and resources close; no ready partial API or fake handler |
| Invalid prefix, wildcard catch-all, foreign owner/app import | Contract/import checks reject; no request forwarding or cross-app fallback |
| One permitted request plus unauthenticated/foreign company/branch/party and stale revision fixtures | Actual feature admission enforces its approved matrix; registration cannot make forbidden commands succeed |
| Same-key retry, changed payload, lost response before/after commit | Real handler/receipt machinery preserves the approved outcome; no registry-level retry or invented receipt |
| Duplicate event, gap, irrelevant sequence, crash before/after inbox commit/ACK, failed quarantine | Approved inbox/checkpoint/recovery mechanics hold; no duplicate effect, lost poison event or skipped sequence |
| Historical projection replay versus process handler | Replay changes only read generation; business commands occur only through approved process admission |
| App zero/one export, malformed export, wrong app, duplicate path/ID/slot, missing parent | Valid subset builds; invalid present graph fails; no phantom navigation |
| Four-prefix deep link, refresh, back/forward, API404 and missing asset | Correct app boundaries; no HTML on API errors or missing assets |
| Context switch while a page/slot request is pending | Existing epoch/session guards discard stale responses and clear scoped overlays/cache |
| Absent policy-gated slice and present unrelated CRM/provider slice | Unrelated slice remains usable; missing slice stays unavailable with no production rule invented |

Run T-003 import checks with compiled minimal owner examples, adapter tests, real
owner/broker fixtures where effects exist, strict app typecheck/build/unit checks
and browser checks for app runtime tasks. Concrete feature tasks own their exact
actor/state/revision matrices and event fixtures; registration-only work does not
claim their exhaustive coverage. No-event features explicitly assert no consumer.

## 6. Scoped unresolved decisions and approval handoff

No new domain owner, cross-owner transaction, event delivery guarantee or financial
policy is proposed. Existing ownership and ADR-03/05 settle these mechanics.
[Open decisions](../../../open-decisions.md) still gate affected business actions,
including suspension policy, fulfillment evidence, registration, own installments,
real product/PII configuration and unconfirmed UI states. Do not treat successful
factory binding as clearance of those gates.

Coordinator review must approve this registration convention before dependent
runtime tasks. Root tasks supply their concrete typed resource structs from
approved foundation ports and bind them in assigned files. A future capability
requiring a new port/resource, consumer contract, topology, schema migration or
release-required capability list needs its own explicit ownership and review.
If the existing ports cannot support a factory without `any`, service lookup or
outer imports, stop that factory and propose the missing narrow port; do not
broaden the convention silently. This draft does not approve itself, clear task
gates, complete B-01 or certify application/runtime/release readiness.

## Coordinator review — 2026-09-15

Promoted from the preserved architect draft for independent QA. The convention
fits the approved owner boundaries and introduces no dependency or business
policy. Coordinator technical review is complete; final approval remains
conditional on independent proposal QA. No runtime behavior is implemented.

Navigation clarification: each app root owns the fixed approved HTML navigation
catalog, keyed by stable page ID with its M-ID, label and placement. It contains
metadata only, never imports absent feature modules. Discovery intersects that
catalog with validated present page exports and current session visibility. A
leaf export cannot create navigation by itself or change catalog placement. New
navigation requires a separately approved catalog change; contextual slot-only
features need no catalog entry. This makes the example FeatureRoutes shape
sufficient without an implicit untyped menu-property extension.

## Approved messaging transport amendment — 2026-09-15

[Coordinator approval](../../approvals/messaging-delivery.md) adopts the
[reviewed delivery contract](messaging-delivery.md), supplementing this original
T-746 proposal. In custody mode only owner intake competes on the AMQP queue;
projection/process factories supply logical SQL consumer jobs. Complete inert
claims still cover both roles. Broker ACK follows durable custody plus all
admitted jobs; each consumer separately commits inbox/effect/checkpoint/job
completion. Registration never grants stream access, alters enrollment history
or authorizes snapshot/process catch-up. Existing direct-ACK mode remains only
for explicitly single-consumer compositions. T-917–T-924 and updated owner
wiring dependencies must integrate before this mode is activated.
