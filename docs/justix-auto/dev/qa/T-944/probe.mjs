import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {mkdtempSync} from 'node:fs';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {createRequire} from 'node:module';
import vm from 'node:vm';
const sha='26e2ef72ce270d0af6954caf24e2a49e18162532';
assert.equal(execFileSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).trim(),sha);
const out=mkdtempSync(path.join(tmpdir(),'t944-qa-'));
execFileSync(process.execPath,['node_modules/typescript/bin/tsc','--ignoreConfig','--strict','--target','es2022','--module','commonjs','--moduleResolution','node','--ignoreDeprecations','6.0','--skipLibCheck','--types','node','--outDir',out,'web/packages/api/src/authEpoch.ts'],{stdio:'pipe'});
const req=createRequire(import.meta.url),{createAuthEpoch}=req(path.join(out,'authEpoch.js')),{createApiClient}=req(path.join(out,'client.js'));
const states=['empty','anonymous','restricted','authenticated','context-unresolved','uncertain'];
const paths={session:'',login:'/login',logout:'/logout','enrollment-start':'/mfa/enrollment','enrollment-confirm':'/mfa/enrollment/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/confirm','challenge-create':'/mfa/challenges','challenge-verify':'/mfa/verify','recovery-request':'/recovery/request','recovery-complete':'/recovery/complete','revoke-all':'/revoke-all'};
const receipt=code=>({error:{code,message:'Fixture',fields:{},traceId:'qa'}});
const schema={parse(v){return v;}};
function op(purpose,decision,status=200,rotate=true){
 const errors={401:schema,403:schema,429:schema,503:schema};
 const metadata={};for(const code of [status,...Object.keys(errors).map(Number)])metadata[code]={csrf:(code===status&&rotate)||(code===401&&purpose==='session')?'required':'forbidden',retryAfter:code===429?'required':'forbidden'};
 return {purpose,expected:states,request:{path:'/api/v1/identity/session'+paths[purpose],method:purpose==='session'?'GET':'POST',schema,successStatuses:[status],responseContract:{auth:true,numeric:'safe-integers',errors,metadata}},classify:()=>decision};
}
const response=(status=200,data={value:'fixture'},token='qa_token')=>new Response(status===204?null:JSON.stringify(data),{status,headers:{'Content-Type':'application/json','Cache-Control':'no-store',...(token?{'X-CSRF-Token':token}:{})}});
function setup(){const queue=[],calls=[];const c=createAuthEpoch(createApiClient({origin:'https://qa.invalid',fetch:(url,init)=>{calls.push({url:String(url),init});assert(queue.length);return Promise.resolve(queue.shift()());}}));return {c,queue,calls,async acquire(){queue.push(()=>response());assert.equal((await c.execute(op('session',{kind:'session',session:{identity:'PRIVATE_CONTEXT'}}))).kind,'accepted');}};}
function deferred(){let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b;});return{promise,resolve,reject};}
let passes=0;const rows=[];async function test(name,fn){await fn();passes++;rows.push(name);}
for(const purpose of ['session','login','challenge-verify','enrollment-confirm'])for(const reason of ['logout','context-change','revocation','unmount'])await test(`${purpose} pending -> ${reason}`,async()=>{
 const s=setup();await s.acquire();const gate=deferred();s.queue.push(()=>gate.promise);
 const work=s.c.execute(op(purpose,purpose==='enrollment-confirm'?{kind:'enrollment-confirmed',view:{codes:['PRIVATE_CODE']}}:{kind:'session',session:{identity:'LATE'}}));
 if(reason==='logout'){assert.deepEqual(await s.c.execute(op('logout',{kind:'clear'},204,false)),{kind:'busy'});}else s.c.invalidate(reason);
 assert.equal(s.c.state().busy,true);assert.equal(s.c.withSession(()=>assert.fail()),false);assert.equal(s.c.withView(()=>assert.fail()),false);
 assert.deepEqual(await s.c.execute(op('session',{kind:'session',session:{}})),{kind:'busy'});assert.equal(s.calls.length,2);
 gate.resolve(response());const result=await work;assert(['stale','uncertain'].includes(result.kind));assert.equal(s.c.state().busy,false);assert.equal(s.c.withSession(()=>assert.fail()),false);assert.equal(s.c.withView(()=>assert.fail()),false);assert.equal(s.calls.length,2);
 s.queue.push(()=>response(401,receipt('SESSION_REQUIRED'),'fresh_anon'));assert.equal((await s.c.execute(op('session',{kind:'anonymous'}))).kind,'accepted');assert.equal(s.c.state().phase,'anonymous');
});
await test('real logout sends private old token after clearing visible state',async()=>{
 const s=setup();await s.acquire();const gate=deferred();s.queue.push(()=>gate.promise);
 const pending=s.c.execute(op('logout',{kind:'clear'},204,false));assert.equal(s.c.state().phase,'uncertain');assert.equal(s.c.withSession(()=>assert.fail()),false);assert.equal(new Headers(s.calls[1].init.headers).get('X-CSRF-Token'),'qa_token');
 gate.resolve(response(204,null,''));const r=await pending;assert.deepEqual(Object.keys(r).sort(),['epoch','kind','status']);assert.equal(s.c.state().phase,'empty');assert.equal(s.calls.length,2);
});
const unhandled=[];const onUnhandled=x=>unhandled.push(x);process.on('unhandledRejection',onUnhandled);
for(const maker of [()=>Promise.reject(Error('PRIVATE_DIAGNOSTIC')),()=>vm.runInNewContext('Promise.reject(Error("PRIVATE_DIAGNOSTIC"))'),()=>Object.defineProperty({},'then',{get(){assert.fail('then accessor invoked');}})])await test('invalid classifier suppresses state and global diagnostics',async()=>{
 const s=setup();s.queue.push(()=>response());const operation=op('session',{kind:'session',session:{}});operation.classify=maker;
 assert.deepEqual(await s.c.execute(operation),{kind:'uncertain'});assert.equal(s.c.withSession(()=>assert.fail()),false);
});
await new Promise(r=>setImmediate(r));await new Promise(r=>setImmediate(r));assert.equal(unhandled.length,0);process.off('unhandledRejection',onUnhandled);
await test('reentrant nested execute stays busy; epoch invalidation wins',async()=>{
 const s=setup();s.queue.push(()=>response());let nested;const operation=op('session',{kind:'session',session:{}});operation.classify=()=>{nested=s.c.execute(op('session',{kind:'anonymous'}));s.c.invalidate('revocation');return{kind:'session',session:{identity:'LATE'}};};
 assert.deepEqual(await s.c.execute(operation),{kind:'stale'});assert.deepEqual(await nested,{kind:'busy'});assert.equal(s.c.withSession(()=>assert.fail()),false);
});
await test('secret view clears on invalidation and receipt contains no payload',async()=>{
 const s=setup();await s.acquire();s.queue.push(()=>response(200,{},''));const r=await s.c.execute(op('enrollment-start',{kind:'enrollment-secret',view:{secret:'PRIVATE_SEED'}},200,false));
 assert.equal(r.kind,'accepted');assert(!JSON.stringify(r).includes('PRIVATE'));assert.equal(s.c.withView(v=>{assert(Object.isFrozen(v));assert.equal(v.secret,'PRIVATE_SEED');}),true);s.c.invalidate('navigation');assert.equal(s.c.withView(()=>assert.fail()),false);
});
const findings=[];
for(const [status,code] of [[403,'CSRF_REJECTED'],[401,'SESSION_EXPIRED'],[401,'SESSION_REQUIRED']]){
 const s=setup();await s.acquire();s.queue.push(()=>response(status,receipt(code),''));
 const rejected=op('challenge-create',{kind:'unchanged'},200,false);rejected.expected=['authenticated'];
 rejected.request.responseContract.errors[status]={parse(v){assert.equal(v.error.code,code);return v;}};
 const result=await s.c.execute(rejected);const after=s.c.state();let visible=false;s.c.withSession(()=>{visible=true;});
 s.queue.push(()=>response(429,receipt('RATE_LIMITED'),'')); // Invalid missing Retry-After safely ends the diagnostic follow-up.
 await s.c.execute(op('challenge-create',{kind:'unchanged'},200,false));
 findings.push({purpose:'challenge-create',status,code,result,stateAfterRejectedBinding:after,protectedContextStillVisible:visible,subsequentUnsafeFetch:s.calls.length===3,reusedOldToken:new Headers(s.calls[2]?.init.headers).get('X-CSRF-Token')==='qa_token'});
}
console.log(JSON.stringify({sha,compiledOutput:out,passingIndependentCases:passes,rows,unhandledRejections:unhandled.length,findings},null,2));
