// Independent expectation-only adaptation. The original BOUNCE probe stays byte-exact.
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, mkdtempSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import path from 'node:path';
const original=readFileSync('docs/justix-auto/dev/qa/T-936/probe.mjs','utf8');
let source=original;
function replace(before,after){assert.equal(source.split(before).length,2,`Unique replacement: ${before}`);source=source.replace(before,after);}
source=source.replaceAll('0adcda6d5011c6e602963d0392b1d8241fb4a5a4','bf64253647a425f2d818ce30348bc75d91e9f2c8');
replace("assert.equal(unhandled.length-before,1,'Exact reviewed source must reproduce the missing rejection handler');", "assert.equal(unhandled.length-before,0,'Fixed source must observe returned native rejections');assert.equal(s.counts.prepare,0);assert.equal(s.counts.accept,0);");
replace("verdict:'BOUNCE'", "verdict:'GREEN-FOCUSED-PROBE'");
// Extend the independent cases with inert thenables and fulfilled native promises.
replace("process.off('unhandledRejection',handler);", `
let thenAccesses=0, thenCalls=0, extraGetterCases=0;
for(const hook of ['success parse','error parse','prepare','accept']) for(const factory of [
 ()=>Object.defineProperty({},'then',{get(){thenAccesses++;throw Error('SYNTHETIC_PRIVATE');}}),
 ()=>({then(){thenCalls++;}}), ()=>Promise.resolve(()=>true),
 ()=>vm.runInNewContext('Promise.resolve(()=>true)'),
]) {
 const s=setup();let getterCalls=0;
 s.request.responseContract.errors={403:{parse:v=>v}};s.request.responseContract.metadata[403]={...forbidden};
 const target=hook==='success parse'?s.request.schema:hook==='error parse'?s.request.responseContract.errors[403]:s.request.authExchange;
 Object.defineProperty(target,hook.endsWith('parse')?'parse':hook,{get(){getterCalls++;return factory();}});
 assert.deepEqual(await s.client.request(s.request),{kind:'invalid-request'});
 assert.deepEqual(s.counts,{fetch:0,parse:0,prepare:0,accept:0});assert.equal(getterCalls,1);extraGetterCases++;
}
await new Promise(r=>setImmediate(r));await new Promise(r=>setImmediate(r));
assert.equal(unhandled.length,0);assert.equal(thenAccesses,0);assert.equal(thenCalls,0);
rows.push('16 extra getter cases: local/foreign fulfilled native Promises, inert callable/accessor thenables; zero side effects');
process.off('unhandledRejection',handler);`);
const dir=mkdtempSync(path.join(tmpdir(),'t936-r2-qa-'));
const file=path.join(dir,'probe.mjs');writeFileSync(file,source);
const result=JSON.parse(execFileSync(process.execPath,[file],{encoding:'utf8'}));
result.originalProbeSHA256=createHash('sha256').update(original).digest('hex');
result.adaptedProbeSHA256=createHash('sha256').update(source).digest('hex');
result.extraGetterCases=16;
result.expectationChanges='Exact SHA; expected unhandled count 1 -> 0; zero prepare/accept checks; verdict label. Added 16 cases without altering original scenarios.';
console.log(JSON.stringify(result,null,2));
