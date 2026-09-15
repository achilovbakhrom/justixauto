# Auth transport compatibility — architect result

Current status: architect fix cycle1 complete; revised proposal awaits independent
exact-commit QA. Original candidate `9ad5a64422b04dbbc8bee969a47acdd3e9278d81`
BOUNCED for the synchronous void-hook and fatal union-composition gaps. The
following original result/probes are preserved as historical evidence, not
relabeled as sufficient validation. Revised evidence follows at the end.

## Historical original candidate result

2026-09-15. Proposal only; independent exact-commit QA not yet run.
Draft: [auth-transport-compatibility.md](../../state/drafts/architect/auth-transport-compatibility.md).
Base: `476777139ffa6a470c7b3f6e79086c64f2dc7db5`.

Resolved against actual T-032 client and reviewed T-640 source: strict raw JSON
before lossy parse; status-specific body/semantic and allowlisted metadata checks
before token acceptance; typed429; explicit transport capability; memory epoch
and request-ticket rules; browser cookie/header observability limits; exact
Go/TS interface and five bounded alias handoffs. Concrete feature semantics are
mandatory typed bindings after T-641, not a new schema assertion language.
No application, canonical, mock, dependency or policy files changed.

## Executed checks and honest limits

- Pinned Node24.21.0: 40 assertions PASS. Reproduced current client duplicate-key,
  malformed UTF-8, rounding and429 behavior; observed normalized Headers; tested
  19 strict integer/Unicode/duplicate/trailing cases, pre-sink body+header checks,
  stale epochs and required semantic binding. Synthetic Responses only.
- Pinned TypeScript strict compile: exit0 with actual T-032 types. Demonstrated
  structural legacy transport assignability despite optional new fields, then
  compile-time rejection with required version marker. This is interface
  feasibility, not an implemented upgraded transport.
- Pinned Go1.27.1 `go test -race -count=1 ./...` in the isolated probe module:
  `ok transportprobe 1.186s`. Actual T-640 generated decoder function text ran
  the same19 cases. Separate typed semantic-port probe rejected absent and typed
  nil bindings, semantic mismatch and missing metadata before any sink call;
  one valid candidate was accepted. Proposed Go auth adapter is not implemented.
- Initial Go probe fixture source failed with `illegal byte order mark` because
  JavaScript JSON.stringify emitted a literal BOM inside a Go source string.
  Corrected the harness to escape that source character as `\uFEFF`, preserving
  the intended runtime input; the final reproducible harness below includes the
  correction. This was not an application decoder defect.

No browser UI/session/HttpOnly cookie test, server transaction, production login,
real token or release acceptance is claimed. No npm install/audit or dependency
update. Read-only official Fetch/Encoding/RFC9110 references are linked inline in
the draft; no third-party package facts were inferred. Gaze references provide
architectural context only, with their documented limits retained.

## Source identities
- `web/packages/api/src/client.ts` SHA-256 `eb420ca1ca94224b069a41d99864e4c5984c66f27fe9e7c7d96284c1d127457c`.
- `tools/generate-contracts.mjs` SHA-256 `38919750f561b5c4feadbcd80eaff0b34b7711d8e283786e12e532c4548ce868`.
- `docs/justix-auto/state/drafts/contracts/auth.md` SHA-256 `935aa3bf7e2c060c38696d96576c9f7a8a7c39beef1eaa6e27b9fefabeee947e`.
- Draft SHA-256 `539aada61e3751130e880fe059c13e92663d716425b60ea981a5a4161dc4670d`.

## Reproduce isolated probes

The following are recoverable exact probe sources, not application deliverables.
Save the first two blocks at their named `/private/tmp` paths. The Node probe
imports the actual assigned-worktree client and extracts the actual Go runtime
from the generator template without invoking its CLI. It creates only the
isolated probe module and decoder test in `/private/tmp`. Save the third block
as the additional Go interface test. These paths use the assigned worktree at
this base; change only that root if QA creates an equivalent detached checkout.

```sh
/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin/node /private/tmp/justix-auth-transport-probe.mjs
/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin/node /Users/bakhromachilov/startups/justixauto/node_modules/typescript/bin/tsc --noEmit --strict --target es2023 --module nodenext --moduleResolution nodenext --skipLibCheck /private/tmp/justix-auth-transport-types.ts
cd /private/tmp/justix-auth-transport-go-probe
/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/go-1.27.1/bin/go test -race -count=1 ./...
```

The direct Go compiler is the same installed pinned compiler selected by the
project wrapper; this isolated module is outside the application package set.
The initial Node probe reports before writing the extracted Go fixture; require
both its exit0 and the separate Go test exit0, not only the printed PASS line.

### `/private/tmp/justix-auth-transport-probe.mjs`

SHA-256 `28766be1c07776c777e69bdaf6d26d8477f640a8af2909f5bd2f32e6090831f9`.

```js
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs';
import {createApiClient} from '/Users/bakhromachilov/startups/justixauto/.worktrees/auth-transport-compatibility/web/packages/api/src/client.ts';
let checks=0;
const check=(v)=>{assert.ok(v);checks++};
const error={error:{code:'SESSION_REQUIRED',message:'',fields:{},traceId:''}};
const call=async(body,status=200,extra={})=>createApiClient({origin:'https://fixture.invalid',fetch:async()=>new Response(body,{status,headers:{'content-type':'application/json',...extra}})}).request({path:'/api/v1/identity/session',schema:{parse:v=>v},successStatuses:[200]});
check((await call('{"x":1,"x":2}')).data.x===2);
check((await call(Uint8Array.from([123,34,120,34,58,34,255,34,125]))).data.x==='�');
check((await call('{"x":1.0000000000000001}')).data.x===1);
check((await call(JSON.stringify(error),401,{'x-csrf-token':'synthetic'})).kind==='http-error');
check((await call(JSON.stringify(error),429)).kind==='unexpected-status');
const h=new Headers([['X-CSRF-Token','a'],['x-csrf-token','b']]);check(h.get('x-csrf-token')==='a, b');
check(new Headers({'X-CSRF-Token':'  a\t'}).get('x-csrf-token')==='a');
check(new Headers([['Retry-After','1'],['Retry-After','2']]).get('retry-after')==='1, 2');
assert.throws(()=>new TextDecoder('utf-8',{fatal:true}).decode(Uint8Array.of(255)));checks++;
check(new TextDecoder('utf-8',{fatal:true,ignoreBOM:true}).decode(Uint8Array.of(239,187,191,123,125)).charCodeAt(0)===0xfeff);
// Feasibility scanner, not an installed decoder: preserve lexemes until exact checks.
function strict(raw){
 const t=new TextDecoder('utf-8',{fatal:true,ignoreBOM:true}).decode(raw);let i=0;
 const ws=()=>{while(/[ \t\r\n]/.test(t[i]??'!'))i++};
 const str=()=>{const start=i++;while(i<t.length){if(t[i]==='\\'){i+=2;continue}if(t[i++]==='"'){const s=JSON.parse(t.slice(start,i));if(!s.isWellFormed())throw Error('unicode');return s}}throw Error('string')};
 const num=(s)=>{const m=/^(-?)(\d+)(?:\.(\d+))?(?:[eE]([+-]?\d+))?$/.exec(s);let digits=(m[2]+(m[3]??'')).replace(/^0+/,'');if(!digits)return 0;const scale=BigInt(m[4]??0)-BigInt((m[3]??'').length);if(scale>16n||scale<BigInt(-digits.length))throw Error('range');if(scale<0n){const cut=Number(-scale);if(!/^0*$/.test(digits.slice(-cut)))throw Error('fraction');digits=digits.slice(0,-cut)}else digits+='0'.repeat(Number(scale));const n=BigInt((m[1]??'')+(digits||'0'));if(n>9007199254740991n||n< -9007199254740991n)throw Error('range');return Number(n)};
 const val=()=>{ws();if(t[i]==='"')return str();if(t[i]==='{'||t[i]==='['){const obj=t[i++]==='{',end=obj?'}':']',v=obj?{}:[],seen=new Set();ws();if(t[i]===end){i++;return v}for(;;){let k;if(obj){ws();if(t[i]!=='"')throw Error('key');k=str();if(seen.has(k))throw Error('duplicate');seen.add(k);ws();if(t[i++]!==':')throw Error(':')}const x=val();if(obj)Object.defineProperty(v,k,{value:x,enumerable:true,writable:true,configurable:true});else v.push(x);ws();if(t[i]===end){i++;return v}if(t[i++]!==',')throw Error(',')}}const m=/^(?:true|false|null|-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)/.exec(t.slice(i));if(!m)throw Error('token');i+=m[0].length;return /^[tfn]/.test(m[0])?JSON.parse(m[0]):num(m[0])};
 const v=val();ws();if(i!==t.length)throw Error('trailing');return v;
}
const enc=s=>new TextEncoder().encode(s);
const cases=[['1.0',true],['1e0',true],['10e-1',true],['-0',true],['0e999999',true],['9007199254740991',true],['1.0000000000000001',false],['1e-9999',false],['9007199254740992',false],['1e9999',false],['{"x":1,"\\u0078":2}',false],['"\\ud800"',false],['{"\\udc00":1}',false],['"\\ud83d\\ude00"',true],['{} {}',false],['{"__proto__":{"x":1}}',true],['[1,]',false],['01',false],['\ufeff{}',false]];
for(const [s,valid]of cases){let actual=true;try{strict(enc(s))}catch{actual=false}assert.equal(actual,valid,s);checks++}
check(Object.getPrototypeOf(strict(enc('{"__proto__":{"x":1}}')))===Object.prototype);
// Contract composition: typed body and metadata finish before a synchronous epoch sink.
let epoch=0,token=null,calls=0;
function acceptResponse(captured,body,header){if(body.error.code!=='SESSION_REQUIRED')throw Error('endpoint schema');if(!/^[A-Za-z0-9_-]{1,4096}$/.test(header))throw Error('metadata');if(captured!==epoch)return false;token=header;calls++;return true}
assert.throws(()=>acceptResponse(0,{error:{code:'BOGUS'}},'new'));checks++;check(token===null&&calls===0);
assert.throws(()=>acceptResponse(0,error,'a, b'));checks++;check(token===null&&calls===0);
check(acceptResponse(0,error,'first'));epoch++;token=null;check(!acceptResponse(0,error,'stale')&&token===null);
check(acceptResponse(1,error,'current')&&token==='current');
function bind(semantics){if(typeof semantics?.validateSession!=='function')throw Error('missing semantics');return value=>{semantics.validateSession(value);return value}}
assert.throws(()=>bind(undefined));checks++;
const validate=bind({validateSession:v=>{if(v.revision!==v.data.context.revision)throw Error('revision relation')}});
assert.throws(()=>validate({revision:'2',data:{context:{revision:'1'}}}));checks++;
check(validate({revision:'2',data:{context:{revision:'2'}}}).revision==='2');
console.log(JSON.stringify({node:process.version,checks,cases:cases.length,result:'PASS',limits:'Node Response/Headers and isolated interface probes; no browser cookie, server or application implementation proof'}));
const src=readFileSync('/Users/bakhromachilov/startups/justixauto/.worktrees/auth-transport-compatibility/tools/generate-contracts.mjs','utf8');
const runtime=src.slice(src.indexOf('const goRuntime = `')+19,src.indexOf('\nfunction renderGo'));
// Use the actual template literal evaluation, without running the generator CLI.
const expression=src.slice(src.indexOf('const goRuntime = ')+18,src.indexOf('\nfunction renderGo')).trim().replace(/;$/,'');
const goRuntime=Function('return '+expression)();
const begin=goRuntime.indexOf('func contractRead('),end=goRuntime.indexOf('func contractDecode(');
mkdirSync('/private/tmp/justix-auth-transport-go-probe',{recursive:true});
writeFileSync('/private/tmp/justix-auth-transport-go-probe/go.mod','module transportprobe\n\ngo 1.27.1\n');
writeFileSync('/private/tmp/justix-auth-transport-go-probe/probe_test.go','package transportprobe\nimport("bytes";"encoding/json";"errors";"io";"math";"math/big";"strconv";"unicode/utf8";"testing")\nvar contractInvalid=errors.New("invalid")\n'+goRuntime.slice(begin,end)+'\nfunc TestActualGeneratedRead(t *testing.T){for _,c:=range []struct{s string;valid bool}{'+cases.map(([s,v])=>'{'+JSON.stringify(s).replaceAll('\ufeff','\\uFEFF')+','+v+'}').join(',')+'}{_,err:=contractRead([]byte(c.s));if (err==nil)!=c.valid{t.Errorf("%q valid=%v err=%v",c.s,c.valid,err)}}}\n');
```

### `/private/tmp/justix-auth-transport-types.ts`

SHA-256 `a8385397f1bcfb69b1dfb85392ccfea9fc09fa325e9589f2b896e544d2f67c55`.

```ts
import type {ApiRequest,ApiResult,Schema,ErrorReceipt,ErrorStatus} from '/Users/bakhromachilov/startups/justixauto/.worktrees/auth-transport-compatibility/web/packages/api/src/client.ts';
type Status = ErrorStatus | 429;
type Valid<T> = Extract<ApiResult<T>,{kind:'success'|'http-error'}> | {kind:'http-error';status:429;receipt:ErrorReceipt};
type Policy = {csrf:'required'|'forbidden';retryAfter:'required'|'forbidden'};
type Contract = {readonly numeric:'safe-integers';readonly errors:Partial<Record<Status,Schema<ErrorReceipt>>>;readonly metadata:Partial<Record<number,Policy>>};
type Port<T> = {prepare():{csrf?:string}|undefined;accept(result:Valid<T>,metadata:{csrf?:string;retryAfterSeconds?:number}):boolean};
type NextRequest<T> = ApiRequest<T> & {responseContract?:Contract;authExchange?:Port<T>};
declare const original:ApiRequest<string>;
const remainsCompatible:NextRequest<string> = original;
declare const legacyTransport:{request<T>(request:ApiRequest<T>):Promise<ApiResult<T>>};
const structural:{request<T>(request:NextRequest<T>):Promise<ApiResult<T>>}=legacyTransport;
// The structural assignment compiles: generated secure calls MUST also require a runtime capability marker.
type SecureTransport={readonly responseContractVersion:1;request<T>(request:NextRequest<T>):Promise<ApiResult<T>>};
// @ts-expect-error A legacy client cannot prove it enforces metadata and pre-sink endpoint schemas.
const insecure:SecureTransport=legacyTransport;
void remainsCompatible;void structural;void insecure;
```

### `/private/tmp/justix-auth-transport-go-probe/interface_test.go`

SHA-256 `0ae4a50718bc32e9e65b7532333e31aa151b5804cbae659694393cb6ee6b0aca`.

```go
package transportprobe

import (
 "errors"
 "reflect"
 "testing"
)

type session struct { Revision, ContextRevision string }
type semantics interface { ValidateSession(session) error }
type concrete struct{}
func (*concrete) ValidateSession(v session) error {if v.Revision!=v.ContextRevision{return errors.New("invalid")};return nil}
type metadata struct { CSRF *string }
type port interface { Accept(session,metadata) bool }
type sink struct { calls int; token string }
func(s *sink) Accept(_ session,m metadata)bool{s.calls++;s.token=*m.CSRF;return true}
func exchange(s semantics,p port,v session,token string)error{
 if s==nil || reflect.ValueOf(s).Kind()==reflect.Ptr&&reflect.ValueOf(s).IsNil(){return errors.New("missing semantics")}
 if err:=s.ValidateSession(v);err!=nil{return err}
 if token==""{return errors.New("missing metadata")}
 if !p.Accept(v,metadata{CSRF:&token}){return errors.New("stale")};return nil
}
func TestTypedSemanticPort(t *testing.T){
 p:=&sink{};var absent *concrete
 for _,c:=range []struct{s semantics;v session;token string}{
  {nil,session{"1","1"},"ok"},{absent,session{"1","1"},"ok"},
  {&concrete{},session{"2","1"},"ok"},{&concrete{},session{"1","1"},""},
 }{if exchange(c.s,p,c.v,c.token)==nil{t.Fatal("invalid candidate accepted")}}
 if p.calls!=0{t.Fatal("premature sink")}
 if err:=exchange(&concrete{},p,session{"1","1"},"accepted");err!=nil||p.calls!=1||p.token!="accepted"{t.Fatal("valid candidate rejected")}
}
```

## Fix cycle1 — explicit outcomes, structural-first validation and bounded scope

Independent BOUNCE report and all four evidence artifacts remain unchanged in
main `docs/justix-auto/dev/qa/auth-transport-compatibility.md` and its directory.
Reviewed original SHA: `9ad5a64422b04dbbc8bee969a47acdd3e9278d81`.
The original draft hash above identifies that historical candidate, not this revision.

Corrections in the revised proposal:

- Replace void/error hooks with a closed synchronous primitive outcome in TS and
  a nonzero enum in Go; reject every other runtime return, catch implementation
  exceptions/panics as safe fatal faults, and check readiness before every hook.
- Supported native Promise returns reject immediately and receive constant
  rejection handlers without awaiting or emitting diagnostics. Plain thenables
  reject without invoking their then method/getter. Hostile Promise species/
  proxies and unreturned async work are explicitly outside the trusted binding
  contract; this does not claim arbitrary executable-code isolation.
- Validate the full structural tree and oneOf cardinality before semantic hooks;
  only the unique selected branch runs. Mismatch rejects the operation without
  fallback; policy/configuration/binding faults cannot be swallowed as nonmatches.
  Preserve current-ticket checks in failure/finally paths and unknown dispatched
  unsafe outcomes; zero sink calls and no retry on invalid validation.
- Estimate the original AT-GEN at20h and replace it with six explicitly serial
  tasks (2/4/4/4/3/3h), sharing only the exact two former generator leaves.
  There are ten proposed aliases /36h total. Intermediate public generation
  remains closed to auth; only the terminal complete-profile verification task
  enables it and precedes T-641. No implementation is shifted into generated leaves.

Executed new checks (isolated feasibility, not installed application behavior):

1. Pinned TypeScript strict compile exit0 with six used @ts-expect-error negative
   assertions covering async/ordinary Promise/thenable/void callbacks plus readiness
   and operation-error hooks. Initial invocation omitted `--types node` and failed
   to discover existing Node declarations; adding that invocation option fixed the
   harness. No package/dependency was installed or changed.
2. Pinned Node24.21.0 executed the bypass/runtime probe:60 assertions PASS.
   Illegal returns were injected into named, readiness and operation-error hooks;
   nested selected-ref failure stops later hooks. Native rejected async/ordinary/
   cross-realm Promises produce zero unhandledRejection events across two event-loop
   turns; arbitrary thenable getters/methods are invoked zero times. Structural
   ambiguity cannot be repaired by semantic mismatch/fatal; selected faults and
   global policy faults abort before the sink. One uniquely valid case reaches it.
3. Pinned Go1.27.1 through the project wrapper, `go test -race -count=1 -v ./...`
   in the isolated semanticprobe module:15 subtests PASS, `ok semanticprobe 1.594s`.
   Actual typed interface/enum calls cover readiness, named/error hooks, unknown/
   zero values, selected policy faults, panics, typed-nil binding, no fallback,
   independent structural cardinality and exact sink counts.
4. Full existing934-task dependency graph plus ten proposed aliases:944 nodes,
   acyclic; estimated36h. Repeated generator leaves have cumulative serial
   prerequisites. Local prose links and revised draft digest pass; the original
   result body and its three embedded probe sources remain byte-identical to
   the original candidate. Only the two assigned documentation leaves differ.

The final runtime activation explicitly tightens resource acceptance for all
newly regenerated Go outputs, including non-auth/internal routes; intermediate
RAW work leaves the old emitted path unchanged. Final tests must cover formerly
accepted oversized/deep payloads now rejecting. Generated byte-slice checks do
not prove bounded adapter network IO, including internal mTLS exchanges. Their
composition adapters must bound reads before creating the Body byte slice;
generated validation neither implements TLS nor constrains an already completed
network allocation. This clarification preserves the distinct
transport resource responsibilities and does not accept a release policy.

No application source, canonical document, dependency, mock, cookie, policy or
runtime activation changed. The original40-check/19-case result remains valid
only for what it measured and does not override QA's BOUNCE. New probes verify
an executable model of the revised interface contract, not emitted auth clients.

Revised draft SHA-256: `e1643c8cf36ba5733f7bfa0d32e2285604b345fe72ed506e17248fbae9af3e0c`.

### Reproduce fix-cycle probes

Save the following exact sources at the named temporary paths. They contain
synthetic values only. The compiler/module paths are existing pinned installations.

```sh
/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin/node /Users/bakhromachilov/startups/justixauto/node_modules/typescript/bin/tsc --noEmit --strict --target es2023 --module esnext --moduleResolution bundler --skipLibCheck --types node --typeRoots /Users/bakhromachilov/startups/justixauto/node_modules/@types /private/tmp/justix-auth-transport-r2.ts
/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin/node /private/tmp/justix-auth-transport-r2.ts
cd /private/tmp/justix-auth-transport-r2-go
GOMODCACHE=/private/tmp/justixauto-t003-modcache GOCACHE=/private/tmp/justixauto-integration-gocache GOPROXY=off bash /Users/bakhromachilov/startups/justixauto/.worktrees/auth-transport-compatibility/tools/go.sh test -race -count=1 -v ./...
```

### `/private/tmp/justix-auth-transport-r2.ts`

SHA-256 `c308bd5434e2f138f8ece35c901c78d0a24c57c1da1430702c79694b309d5416`.

```ts
// Isolated proposal probe. No application implementation.
import assert from 'node:assert/strict';
import vm from 'node:vm';
export type SemanticOutcome = 'valid' | 'mismatch' | 'fatal:policy' | 'fatal:configuration' | 'fatal:binding';
type Session = {revision:string;data:{context:{revision:string}}};
interface Semantics {checkReady():SemanticOutcome;validateSession(value:Session):SemanticOutcome;validateError(operation:string,status:number,value:unknown):SemanticOutcome}
// @ts-expect-error Async methods cannot satisfy the exact primitive return union.
const asyncBad:Semantics['validateSession']=async ()=> 'valid' as const;
// @ts-expect-error Ordinary Promise-returning functions are equally excluded.
const promiseBad:Semantics['validateSession']=()=>Promise.resolve('valid' as const);
// @ts-expect-error Thenable objects are not a SemanticOutcome.
const thenableBad:Semantics['validateSession']=()=>({then(){}});
// @ts-expect-error Void/implicit undefined is not a validation success.
const voidBad:Semantics['validateSession']=()=>{};
// @ts-expect-error Readiness uses the identical synchronous boundary.
const readyBad:Semantics['checkReady']=async ()=> 'valid' as const;
// @ts-expect-error Operation/status error validation cannot return Promise either.
const errorBad:Semantics['validateError']=()=>Promise.resolve('valid' as const);
void asyncBad;void promiseBad;void thenableBad;void voidBad;void readyBad;void errorBad;
let checks=0;const eq=(a:unknown,b:unknown)=>{assert.deepEqual(a,b);checks++};
const intrinsicThen=Promise.prototype.then;
// Return disposal is not validation: native Promise rejections get constant handlers.
// Non-Promise thenables are never invoked/read; intrinsic brand check rejects them.
function discardNativePromise(value:unknown){
 if((typeof value!=='object'||value===null)&&typeof value!=='function')return;
 try{Reflect.apply(intrinsicThen,value,[()=>undefined,()=>undefined])}catch{/* no diagnostics */}
}
function guardedCall(hook:unknown,args:unknown[]=[]):SemanticOutcome{
 if(typeof hook!=='function')return 'fatal:binding';
 let out:unknown;try{out=Reflect.apply(hook,undefined,args)}catch(error){discardNativePromise(error);return 'fatal:binding'}
 if(out==='valid'||out==='mismatch'||out==='fatal:policy'||out==='fatal:configuration'||out==='fatal:binding')return out;
 discardNativePromise(out);return 'fatal:binding';
}
function readyHook(ready:unknown,hook:unknown,args:unknown[]=[]):SemanticOutcome{
 const r=guardedCall(ready);if(r!=='valid')return r==='mismatch'?'fatal:binding':r;
 return guardedCall(hook,args);
}
type Structural = 'match'|'mismatch'|'fatal:configuration'|'fatal:binding';
function select(structural: (()=>Structural)[]):number|Structural{
 const matches:number[]=[];
 for(let i=0;i<structural.length;i++){
  let r:unknown;try{r=structural[i]!()}catch{return 'fatal:binding'}
  if(r==='match')matches.push(i);else if(r==='mismatch')continue;
  else return r==='fatal:configuration'?'fatal:configuration':'fatal:binding';
 }
 return matches.length===1?matches[0]!:'mismatch';
}
let sinks=0,hookCalls=0;
function pipeline(structural:(()=>Structural)[],hooks:unknown[],ready:unknown=()=> 'valid',errorHook:unknown=()=> 'valid'):SemanticOutcome{
 const r=guardedCall(ready);if(r!=='valid')return r==='mismatch'?'fatal:binding':r;
 const chosen=select(structural);if(typeof chosen!=='number')return chosen==='match'?'fatal:binding':chosen;
 hookCalls++;const s=readyHook(ready,hooks[chosen]);if(s!=='valid')return s;
 const e=readyHook(ready,errorHook,['sessionRead',401,{}]);if(e!=='valid')return e;
 sinks++;return 'valid';
}
const match=()=> 'match' as const, miss=()=> 'mismatch' as const, pass=()=> 'valid' as const;
const unhandled:unknown[]=[];const listener=(e:unknown)=>unhandled.push(e);process.on('unhandledRejection',listener);
let getterCalls=0,thenCalls=0;
const invalidReturns:unknown[]=[undefined,null,true,{},'VALID',Promise.resolve('valid'),Promise.reject(new Error('synthetic-secret')),vm.runInNewContext('Promise.reject(new Error("cross-realm-synthetic-secret"))'),{get then(){getterCalls++;throw new Error('must not inspect')}},{then(){thenCalls++;throw new Error('must not invoke')}}];
for(const v of invalidReturns){
 eq(pipeline([match],[()=>v]),'fatal:binding');
 eq(pipeline([match],[pass],()=>v),'fatal:binding');
 eq(pipeline([match],[pass],pass,()=>v),'fatal:binding');
}
eq(pipeline([match],[async()=>{throw new Error('async-synthetic-secret')}]),'fatal:binding');
eq(pipeline([match],[()=>Promise.reject(new Error('ordinary-synthetic-secret'))]),'fatal:binding');
for(const where of ['ready','error']as const){
 eq(where==='ready'?pipeline([match],[pass],async()=> 'valid'):pipeline([match],[pass],pass,async()=> 'valid'),'fatal:binding');
}
const throwsPromise=()=>{throw Promise.reject(Error('thrown-promise-synthetic-secret'))};
eq(pipeline([match],[throwsPromise]),'fatal:binding');
eq(pipeline([match],[pass],throwsPromise),'fatal:binding');
eq(pipeline([match],[pass],pass,throwsPromise),'fatal:binding');
eq(getterCalls,0);eq(thenCalls,0);eq(sinks,0);
for(const fault of ['fatal:policy','fatal:configuration','fatal:binding'] as const){
 eq(pipeline([miss,match],[pass,()=>fault]),fault);
 eq(pipeline([match,match],[()=>fault,pass]),'mismatch');
 eq(pipeline([()=>fault==='fatal:policy'?'fatal:configuration':fault,match],[pass,pass]),fault==='fatal:policy'?'fatal:configuration':fault);
}
const before=hookCalls;eq(pipeline([match,match],[()=> 'mismatch',pass]),'mismatch');eq(hookCalls,before);
eq(pipeline([match,miss],[()=> 'mismatch',pass]),'mismatch');
eq(pipeline([miss,match],[pass,()=>{throw new Error('hidden policy')}]),'fatal:binding');
eq(pipeline([miss,match],[pass,pass],()=> 'fatal:policy'),'fatal:policy');
eq(pipeline([miss,match],[pass,pass],()=> 'mismatch'),'fatal:binding');
// A selected root/ref/error sequence stops at the first fatal outcome.
let later=0;
for(const hook of [pass,()=>Promise.reject(Error('nested-ref-synthetic-secret')),()=>{later++;return 'valid'}]){
 if(readyHook(pass,hook)!=='valid')break;
}
eq(later,0);
eq(sinks,0);
eq(pipeline([miss,match],[()=>{throw Error('unselected must not run')},pass]),'valid');eq(sinks,1);
await new Promise<void>(resolve=>setImmediate(resolve));
await new Promise<void>(resolve=>setImmediate(resolve));
eq(unhandled.length,0);process.off('unhandledRejection',listener);
console.log(JSON.stringify({checks,result:'PASS',negativeTypeAssertions:6,limits:'isolated exact-outcome/structural-first model; not generated application code; no hostile Promise species or Proxy execution isolation claim'}));
```

### `/private/tmp/justix-auth-transport-r2-go/go.mod`

SHA-256 `db9ae1ee238fa6ade699661f85bcd63679451505cf878f4b1093a4d6a86bb3d2`.

```text
module semanticprobe

go 1.27.1
```

### `/private/tmp/justix-auth-transport-r2-go/semantics_test.go`

SHA-256 `cfac8efec057a751e8cf924d0968072c2f0567ba71ce6714fbab7fd30b98c75e`.

```go
package semanticprobe

import "testing"

type Outcome uint8
const (Valid Outcome=iota+1; Mismatch; FatalPolicy; FatalConfiguration; FatalBinding)
type Semantics interface { CheckReady() Outcome; ValidateName(string) Outcome; ValidateError(string,int,any) Outcome }
type fixture struct {ready,name,err func()Outcome}
func(f *fixture)CheckReady()Outcome{return f.ready()}
func(f *fixture)ValidateName(string)Outcome{return f.name()}
func(f *fixture)ValidateError(string,int,any)Outcome{return f.err()}
func guard(fn func()Outcome)(out Outcome){
 out=FatalBinding;defer func(){if recover()!=nil{out=FatalBinding}}()
 got:=fn();switch got{case Valid,Mismatch,FatalPolicy,FatalConfiguration,FatalBinding:return got};return FatalBinding
}
func ready(s Semantics)Outcome{r:=guard(s.CheckReady);if r==Mismatch{return FatalBinding};return r}
func invoke(s Semantics,fn func()Outcome)Outcome{if r:=ready(s);r!=Valid{return r};return guard(fn)}
func selectBranch(shapes []func()Outcome)(int,Outcome){
 chosen:=-1;matches:=0
 for i,fn:=range shapes{r:=guard(fn);switch r{case Valid:chosen=i;matches++;case Mismatch:default:return -1,r}}
 if matches!=1{return -1,Mismatch};return chosen,Valid
}
func run(s Semantics,shapes []func()Outcome,hooks []func()Outcome,sinks *int)Outcome{
 if r:=ready(s);r!=Valid{return r};chosen,r:=selectBranch(shapes);if r!=Valid{return r}
 if r=invoke(s,hooks[chosen]);r!=Valid{return r}
 if r=invoke(s,func()Outcome{return s.ValidateError("sessionRead",401,nil)});r!=Valid{return r}
 *sinks++;return Valid
}
func fixed(v Outcome)func()Outcome{return func()Outcome{return v}}
func TestExactOutcomesAndStructuralFirst(t *testing.T){
 ok,miss:=fixed(Valid),fixed(Mismatch);boom:=func()Outcome{panic("synthetic secret")}
 cases:=[]struct{name string;shapes,hooks []func()Outcome;ready,err func()Outcome;want Outcome}{
  {"valid unique",[]func()Outcome{miss,ok},[]func()Outcome{boom,ok},ok,ok,Valid},
  {"ambiguous cannot be resolved by mismatch",[]func()Outcome{ok,ok},[]func()Outcome{miss,ok},ok,ok,Mismatch},
  {"ambiguous cannot be resolved by fatal",[]func()Outcome{ok,ok},[]func()Outcome{fixed(FatalPolicy),ok},ok,ok,Mismatch},
  {"semantic mismatch no fallback",[]func()Outcome{ok,miss},[]func()Outcome{miss,ok},ok,ok,Mismatch},
  {"structural fault plus passing alternative",[]func()Outcome{fixed(FatalConfiguration),ok},[]func()Outcome{ok,ok},ok,ok,FatalConfiguration},
  {"policy fault selected",[]func()Outcome{miss,ok},[]func()Outcome{ok,fixed(FatalPolicy)},ok,ok,FatalPolicy},
  {"binding panic selected",[]func()Outcome{miss,ok},[]func()Outcome{ok,boom},ok,ok,FatalBinding},
  {"unknown outcome selected",[]func()Outcome{ok},[]func()Outcome{fixed(99)},ok,ok,FatalBinding},
  {"zero outcome selected",[]func()Outcome{ok},[]func()Outcome{fixed(0)},ok,ok,FatalBinding},
  {"policy readiness",[]func()Outcome{ok},[]func()Outcome{ok},fixed(FatalPolicy),ok,FatalPolicy},
  {"mismatch readiness is fault",[]func()Outcome{ok},[]func()Outcome{ok},miss,ok,FatalBinding},
  {"error hook fault",[]func()Outcome{ok},[]func()Outcome{ok},ok,fixed(FatalConfiguration),FatalConfiguration},
  {"error hook panic",[]func()Outcome{ok},[]func()Outcome{ok},ok,boom,FatalBinding},
  {"error hook zero",[]func()Outcome{ok},[]func()Outcome{ok},ok,fixed(0),FatalBinding},
 }
 for _,c:=range cases{t.Run(c.name,func(t *testing.T){sinks:=0;s:=&fixture{c.ready,ok,c.err};got:=run(s,c.shapes,c.hooks,&sinks);if got!=c.want{t.Fatalf("got %d want %d",got,c.want)};wantSink:=0;if c.want==Valid{wantSink=1};if sinks!=wantSink{t.Fatalf("sinks %d",sinks)}})}
 t.Run("typed nil is fatal",func(t *testing.T){var f *fixture;var s Semantics=f;if ready(s)!=FatalBinding{t.Fatal("nil accepted")}})
}
```
