import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,mkdtempSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {tmpdir} from 'node:os';
import path from 'node:path';
const original=readFileSync('docs/justix-auto/dev/qa/T-944/probe.mjs','utf8');
let source=original.replaceAll('26e2ef72ce270d0af6954caf24e2a49e18162532','a3c291d124bae46e381faa8307d02551c26d0bc3');
const marker='console.log(JSON.stringify({sha,compiledOutput:out';assert.equal(source.split(marker).length,2);
const extra=`
for(const f of findings){assert.deepEqual(f.result,{kind:'binding-invalid'});assert.equal(f.stateAfterRejectedBinding.phase,'empty');assert.equal(f.stateAfterRejectedBinding.epoch,1);assert.equal(f.protectedContextStillVisible,false);assert.equal(f.subsequentUnsafeFetch,false);assert.equal(f.reusedOldToken,false);}
let correctionCases=0;
for(const state of ['anonymous','restricted','context-unresolved'])for(const kind of ['unchanged','invalidate-binding']){
 const s=setup();
 if(state==='anonymous'){s.queue.push(()=>response(401,receipt('SESSION_REQUIRED')));await s.c.execute(op('session',{kind:'anonymous'}));}
 else{s.queue.push(()=>response());await s.c.execute(op('session',{kind:'restricted',restricted:{identity:'PRIVATE_RESTRICTED'}}));}
 if(state==='context-unresolved'){s.queue.push(()=>response());await s.c.execute(op('enrollment-confirm',{kind:'enrollment-confirmed',view:{codes:['PRIVATE_CODE']}}));}
 assert.equal(s.c.state().phase,state);s.queue.push(()=>response(403,receipt('CSRF_REJECTED'),''));
 assert.deepEqual(await s.c.execute(op('enrollment-start',{kind},200,false)),{kind:'binding-invalid'});
 assert.deepEqual(s.c.state(),{epoch:1,phase:'empty',busy:false,hasView:false});
 assert.equal(s.c.withSession(()=>assert.fail()),false);assert.equal(s.c.withRestricted(()=>assert.fail()),false);assert.equal(s.c.withView(()=>assert.fail()),false);
 const count=s.calls.length;assert.deepEqual(await s.c.execute(op('login',{kind:'unchanged'})),{kind:'invalid-request'});assert.equal(s.calls.length,count);
 s.queue.push(()=>response(401,receipt('SESSION_REQUIRED'),'fresh_csrf'));assert.equal((await s.c.execute(op('session',{kind:'anonymous'}))).kind,'accepted');assert.equal(s.c.state().phase,'anonymous');correctionCases++;
}
// Precise status/code controls: only the approved binding error pairs invalidate.
for(const [status,code,kind,want] of [[403,'CSRF_REJECTED','invalidate-binding','binding-invalid'],[401,'SESSION_EXPIRED','invalidate-binding','binding-invalid'],[401,'SESSION_REQUIRED','invalidate-binding','binding-invalid'],[403,'FORBIDDEN','unchanged','accepted'],[401,'AUTHENTICATION_FAILED','unchanged','accepted'],[503,'AUTHORITY_UNAVAILABLE','unchanged','accepted'],[503,'AUTH_OUTCOME_UNKNOWN','uncertain','uncertain'],[503,'AUTH_OUTCOME_UNKNOWN','unchanged','uncertain'],[403,'SESSION_EXPIRED','unchanged','accepted'],[401,'CSRF_REJECTED','unchanged','accepted'],[403,'FORBIDDEN','invalidate-binding','uncertain']]){
 const s=setup();await s.acquire();s.queue.push(()=>response(status,receipt(code),''));const r=await s.c.execute(op('challenge-create',{kind},200,false));assert.equal(r.kind,want);
 if(want==='accepted')assert.equal(s.c.state().phase,'authenticated');else assert.equal(s.c.withSession(()=>assert.fail()),false);correctionCases++;
}
{
 const s=setup();await s.acquire();s.queue.push(()=>{const r=response(429,receipt('RATE_LIMITED'),'');r.headers.set('Retry-After','3');return r;});assert.equal((await s.c.execute(op('challenge-create',{kind:'unchanged'},200,false))).kind,'accepted');assert.equal(s.c.state().phase,'authenticated');correctionCases++;
}
for(const reason of [null,'navigation','uncertainty'])for(const exhausted of [false,true]){
 let hold=false,calls=0;const gate=deferred(),observed=deferred();
 const actual=createApiClient({origin:'https://qa.invalid',fetch:async()=>{calls++;return calls===1?response():response(403,receipt('CSRF_REJECTED'),'');}});
 const c=createAuthEpoch({responseContractVersion:1,async request(request){const r=await actual.request(request);if(hold){observed.resolve();await gate.promise;}return r;}},{initialEpoch:exhausted?Number.MAX_SAFE_INTEGER-1:0});
 await c.execute(op('session',{kind:'session',session:{identity:'PRIVATE'}}));hold=true;
 const work=c.execute(op('challenge-create',{kind:'invalidate-binding'},200,false));await observed.promise;
 assert.equal(c.state().busy,true);assert.equal(c.withSession(()=>assert.fail()),false);assert.equal(c.state().phase,exhausted?'exhausted':'empty');
 assert.equal((await c.execute(op('session',{kind:'anonymous'}))).kind,exhausted?'exhausted':'busy');assert.equal(calls,2);
 if(reason)c.invalidate(reason);gate.resolve();const r=await work;
 assert.equal(r.kind,exhausted?'exhausted':reason===null?'binding-invalid':reason==='uncertainty'?'uncertain':'stale');assert.equal(c.state().busy,false);assert.equal(calls,2);correctionCases++;
}
for(const reason of ['navigation','uncertainty']){
 const s=setup();await s.acquire();s.queue.push(()=>response(403,receipt('CSRF_REJECTED'),''));const operation=op('challenge-create',{kind:'invalidate-binding'},200,false);operation.classify=()=>{s.c.invalidate(reason);return{kind:'invalidate-binding'};};
 assert.deepEqual(await s.c.execute(operation),{kind:'uncertain'});assert.equal(s.c.state().phase,'uncertain');assert.equal(s.c.withSession(()=>assert.fail()),false);correctionCases++;
}
`;
source=source.replace(marker,extra+marker).replace('unhandledRejections:unhandled.length,findings','unhandledRejections:unhandled.length,findings,correctionCases');
const dir=mkdtempSync(path.join(tmpdir(),'t944-r2-probe-')),file=path.join(dir,'probe.mjs');writeFileSync(file,source);
const output=JSON.parse(execFileSync(process.execPath,[file],{encoding:'utf8'}));
output.originalProbeSHA256=createHash('sha256').update(original).digest('hex');output.adaptedProbeSHA256=createHash('sha256').update(source).digest('hex');
output.adaptation='Original probe retained verbatim except exact SHA; added assertions on three observations and separate correction cases. No previous scenario removed.';
console.log(JSON.stringify(output,null,2));
