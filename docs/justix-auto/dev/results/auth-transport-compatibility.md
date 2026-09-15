# Auth transport compatibility — architect result

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
