import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,mkdtempSync} from 'node:fs';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
const original=readFileSync('docs/justix-auto/dev/qa/T-937/probe.mjs','utf8');
let source=original;
function replace(before,after){assert.equal(source.split(before).length,2,before);source=source.replace(before,after);}
replace("sha='a4de45c811c8eaa7b8d3f7559df0db4c9454a3e3'","sha='1636aeb291007163f80e41855694a2390a7694f5'");
replace("const p=actual.compile(entry,d);assert.deepEqual(p.operations[0].securityRequirements,inherited?[{Cookie:d.components.securitySchemes.Cookie}]:[]);", "assert.throws(()=>actual.compile(entry,d),/auth security declaration must be an array/);");
replace("acceptedSecurity:p.operations[0].securityRequirements", "rejected:true");
replace("const p=actual.compile(entry,d);assert.deepEqual(p.operations[0].parameters,[]);", "assert.throws(()=>actual.compile(entry,d),/auth parameters declaration must be an array/);");
replace("acceptedParameters:p.operations[0].parameters", "rejected:true");
replace("verdict:'BOUNCE'", "verdict:'GREEN-FOCUSED-PROBE'");
// Additional independent checks are appended before the original public CLI work.
replace("const generic=legacy();", `
let additionalNegatives=0, additionalControls=0;
for(const row of [table[0],table[1]]) for(const field of ['security','parameters']) for(const value of [null,false,0,'',{},'[]']){
 const d=fixture(row);operation(d)[field]=value;
 assert.throws(()=>actual.compile(entry,d),new RegExp('auth '+field+' declaration must be an array'));additionalNegatives++;
}
for(const value of [null,false,0,'',{},'[]']){
 const d=fixture();d.security=value;delete operation(d).security;
 assert.throws(()=>actual.compile(entry,d),/security requirements must be an array/);additionalNegatives++;
}
for(const row of [table[0],table[1]])for(const requirements of [undefined,[],[{}],[{}, {Cookie:[]}]]){
 const d=fixture(row);delete operation(d).security;if(requirements!==undefined)d.security=requirements;
 if(row[0]==='get')delete operation(d).parameters;
 const p=actual.compile(entry,d),op=p.operations[0];
 assert.deepEqual(op.securityRequirements,(requirements??[]).map(r=>Object.fromEntries(Object.keys(r).map(k=>[k,d.components.securitySchemes[k]]))));
 assert.equal(op.authTransport.requestCSRF===null,row[0]==='get');additionalControls++;
 operation(d).security=[];d.security=[{Cookie:[]}];if(row[0]==='get')operation(d).parameters=[];
 const overridden=actual.compile(entry,d).operations[0];assert.deepEqual(overridden.securityRequirements,[]);additionalControls++;
 if(row[0]!=='get'){operation(d).parameters=[];assert.throws(()=>actual.compile(entry,d),/unsafe auth requires injected CSRF/);additionalNegatives++;}
}
for(const inherited of [false,true]){
 const d=legacy();d.components.schemas.Datum.properties.value={type:'integer',minimum:0,maximum:100};
 d.components.securitySchemes.Cookie={type:'apiKey',in:'cookie',name:'__Host-justix_session'};
 const o=Object.values(Object.values(d.paths)[0])[0];o.security=null;o.parameters=null;if(inherited)d.security=[{Cookie:[]}];
 const a=actual.compile(entry,d),b=prior.compile(entry,d);
 for(const name of ['renderGo','renderTS'])assert.equal(actual[name](a),prior[name](b));additionalControls++;
}
const generic=legacy();`);
replace("legacyHashes,findings,rows", "legacyHashes,findings,rows,additionalNegatives,additionalControls");
const directory=mkdtempSync(path.join(tmpdir(),'t937-r2-adapt-'));
const script=path.join(directory,'probe.mjs');writeFileSync(script,source);
const output=JSON.parse(execFileSync(process.execPath,[script],{encoding:'utf8'}));
output.originalProbeSHA256=createHash('sha256').update(original).digest('hex');
output.adaptedProbeSHA256=createHash('sha256').update(source).digest('hex');
output.adaptation='Exact correction SHA and the three accepted-null observations changed to assert.throws/rejected:true; verdict changed. Other original logic retained. Added non-array declarations and valid/legacy controls.';
console.log(JSON.stringify(output,null,2));
