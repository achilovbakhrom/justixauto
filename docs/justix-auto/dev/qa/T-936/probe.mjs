import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { createRequire } from 'node:module';
import vm from 'node:vm';
const root = process.cwd();
assert.equal(execFileSync('git', ['rev-parse','HEAD'], {encoding:'utf8'}).trim(), '0adcda6d5011c6e602963d0392b1d8241fb4a5a4');
const out = mkdtempSync(path.join(tmpdir(), 't936-independent-'));
execFileSync(process.execPath, [path.join(root,'node_modules/typescript/bin/tsc'), '--ignoreConfig','--strict','--target','es2022','--module','commonjs','--moduleResolution','node','--ignoreDeprecations','6.0','--skipLibCheck','--types','node','--outDir',out,'web/packages/api/src/client.ts'], {stdio:'pipe'});
const {createApiClient} = createRequire(import.meta.url)(path.join(out,'client.js'));
const origin='https://qa.invalid';
const encoder=new TextEncoder();
const rule={csrf:'required',retryAfter:'forbidden'};
const forbidden={csrf:'forbidden',retryAfter:'forbidden'};
const receipt={error:{code:'DENIED',message:'synthetic',fields:{},traceId:'qa'}};
const response=(body='{}',status=200,headers={})=>new Response(status===204?null:body,{status,headers:{'Content-Type':'application/json','Cache-Control':'no-store','X-CSRF-Token':'qa_Token-1',...headers}});
let count=0;
const rows=[];
async function run(name,fn){await fn(); count++; rows.push(name);}
function setup(make=()=>response()) {
 const counts={fetch:0,parse:0,prepare:0,accept:0};
 const request={path:'/api/v1/identity/session',method:'POST',successStatuses:[200],schema:{parse(v){counts.parse++;return v;}},responseContract:{numeric:'safe-integers',auth:true,errors:{},metadata:{200:{...rule}}},authExchange:{prepare(){counts.prepare++;return {csrf:'qa_request'};},accept(r,m){counts.accept++;assert(Object.isFrozen(m));assert.deepEqual(Object.keys(r).sort(),['data','kind','status']);return true;}}};
 const client=createApiClient({origin,fetch:async(url,init)=>{counts.fetch++;assert.equal(init.credentials,'same-origin');assert.equal(init.redirect,'error');assert.equal(init.cache,'no-store');return make(url,init);}});
 return {counts,request,client};
}
await run('capture success/error class getters and receivers before IO',async()=>{
 let accesses=0;
 class Parser { code='DENIED'; get parse(){accesses++;return function(v){assert.equal(v.error.code,this.code);return v;};}}
 class Exchange { marker='qa'; get prepare(){accesses++;return function(){assert.equal(this.marker,'qa');return {csrf:'qa_request'};};} get accept(){accesses++;return function(r,m){assert.equal(this.marker,'qa');assert.equal(r.status,403);assert.deepEqual(m,{});return true;};}}
 const errorParser=new Parser(); const exchange=new Exchange();
 const s=setup();
 s.request.responseContract.errors={403:errorParser};s.request.responseContract.metadata[403]={...forbidden};s.request.authExchange=exchange;
 // Headers.delete supplies an absent forbidden header, not an empty field.
 const fetcher=async()=>{Object.defineProperty(errorParser,'parse',{value:()=>{throw Error('late');}});Object.defineProperty(exchange,'accept',{value:()=>false});s.request.responseContract.metadata[403].csrf='required';const r=response(JSON.stringify(receipt),403);r.headers.delete('X-CSRF-Token');return r;};
 assert.equal((await createApiClient({origin,fetch:fetcher}).request(s.request)).kind,'http-error');assert.equal(accesses,3);
});
for(const text of ['{"x":"😀","y":1e0}','{"x":1,"\\u0078":2}','"\\ud800"','1.0000000000000001']) {
 const bytes=encoder.encode(text);
 for(let cut=0;cut<=bytes.length;cut++) await run(`partition ${JSON.stringify(text)}@${cut}`,async()=>{
  const foreign=vm.runInNewContext('new Uint8Array('+JSON.stringify([...bytes.subarray(cut)])+')');
  const r=response();Object.defineProperty(r,'body',{value:new ReadableStream({start(c){c.enqueue(bytes.subarray(0,cut));c.enqueue(foreign);c.close();}})});
  const s=setup(()=>r);assert.equal((await s.client.request(s.request)).kind,text.startsWith('{"x":"')?'success':'invalid-response');assert.equal(s.counts.accept,text.startsWith('{"x":"')?1:0);
 });
}
for(const method of ['POST','PUT','PATCH','DELETE']) for(const mode of ['fetch-error','bad-body','bad-metadata','undeclared']) await run(`${method} ${mode} no retry`,async()=>{
 const s=setup(()=>{if(mode==='fetch-error')throw Error('SYNTHETIC_PRIVATE');return mode==='bad-body'?response('{'):mode==='bad-metadata'?response('{}',200,{'Cache-Control':'public'}):response('{}',201);});s.request.method=method;
 const r=await s.client.request(s.request);assert.equal(r.kind,mode==='fetch-error'?'transport-error':mode==='undeclared'?'unexpected-status':'invalid-response');if(r.kind==='transport-error')assert.equal(r.outcome,'unknown');assert.equal(s.counts.fetch,1);assert.equal(s.counts.accept,0);assert(!JSON.stringify(r).includes('SYNTHETIC_PRIVATE'));
});
await run('overflow cancellation rejection is observed; reader released',async()=>{
 let cancels=0;const body=new ReadableStream({start(c){c.enqueue(encoder.encode('{}x'));},cancel(){cancels++;return Promise.reject(Error('SYNTHETIC_PRIVATE'));}});
 const s=setup();const client=createApiClient({origin,maxResponseBytes:2,fetch:async()=>new Response(body,{headers:{'Content-Type':'application/json','Cache-Control':'no-store','X-CSRF-Token':'qa'}})});
 assert.equal((await client.request(s.request)).kind,'invalid-response');assert.equal(cancels,1);assert.equal(body.locked,false);assert.equal(s.counts.accept,0);
});
const unhandled=[];const handler=(reason)=>unhandled.push(reason);process.on('unhandledRejection',handler);
const getterFindings=[];
for(const hook of ['success parse','error parse','prepare','accept']) for(const foreign of [false,true]) {
 const before=unhandled.length;const s=setup(()=>response(JSON.stringify(receipt),403));
 s.request.responseContract.errors={403:{parse:v=>v}};s.request.responseContract.metadata[403]={...forbidden};
 const invalidGetter=()=>foreign?vm.runInNewContext('Promise.reject(new Error("SYNTHETIC_PRIVATE"))'):Promise.reject(Error('SYNTHETIC_PRIVATE'));
 if(hook==='success parse')Object.defineProperty(s.request.schema,'parse',{get:invalidGetter});
 if(hook==='error parse')Object.defineProperty(s.request.responseContract.errors[403],'parse',{get:invalidGetter});
 if(hook==='prepare'||hook==='accept')Object.defineProperty(s.request.authExchange,hook,{get:invalidGetter});
 const result=await s.client.request(s.request);assert.deepEqual(result,{kind:'invalid-request'});assert.equal(s.counts.fetch,0);
 await new Promise(r=>setImmediate(r));await new Promise(r=>setImmediate(r));
 assert.equal(unhandled.length-before,1,'Exact reviewed source must reproduce the missing rejection handler');
 getterFindings.push({hook,foreign,result,fetch:s.counts.fetch,unhandled:unhandled.length-before,privateDiagnosticEscaped:unhandled.at(-1)?.message==='SYNTHETIC_PRIVATE'});
}
process.off('unhandledRejection',handler);
console.log(JSON.stringify({verdict:'BOUNCE',sha:'0adcda6d5011c6e602963d0392b1d8241fb4a5a4',compiledOutput:out,passingFocusedCases:count,rows,getterFindings,unhandledCount:unhandled.length},null,2));
