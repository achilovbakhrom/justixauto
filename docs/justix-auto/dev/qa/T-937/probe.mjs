import assert from 'node:assert/strict';
import * as fs from 'node:fs';
import path from 'node:path';
import { tmpdir } from 'node:os';
import { pathToFileURL } from 'node:url';
import { execFileSync, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
const root=process.cwd(), sha='a4de45c811c8eaa7b8d3f7559df0db4c9454a3e3', base='23d8cede88d228ef57fa73205c159b07e57a49b2';
assert.equal(execFileSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).trim(),sha);
const directory=fs.realpathSync(fs.mkdtempSync(path.join(tmpdir(),'t937-qa-')));
const source=fs.readFileSync('tools/generate-contracts.mjs','utf8');
const predecessor=execFileSync('git',['show',base+':tools/generate-contracts.mjs'],{encoding:'utf8'});
const go=execFileSync('bash',['tools/go.sh','env','GOROOT'],{encoding:'utf8'}).trim();
async function privateModule(text,name){
 const barrier='\ntry { main(); } catch (error)';assert.equal(text.split(barrier).length,2);
 const file=path.join(directory,name+'.mjs');
 fs.writeFileSync(file,text.slice(0,text.indexOf(barrier))+`\ncachedGoRoot=${JSON.stringify(go)};export {compile,renderGo,renderTS};\n`);
 return import(pathToFileURL(file));
}
const actual=await privateModule(source,'current'), prior=await privateModule(predecessor,'predecessor');
const entry={input:'auth.json',goOutput:'auth.go',tsOutput:'auth.ts',goPackage:'qa',owner:'identity',namespace:'Audit'};
const closed={type:'object',properties:{},required:[],additionalProperties:false};
const csrf={type:'string',pattern:'^[A-Za-z0-9_-]{1,4096}$'};
const cache={type:'string',const:'no-store'};
const retry={type:'integer',minimum:0,maximum:2147483647};
const header=s=>({required:true,schema:structuredClone(s)});
const table=[['get','session',200,1],['post','session/login',200,1],['post','session/mfa/challenges',200,0],['post','session/mfa/verify',200,1],['post','session/logout',204,0],['post','session/revoke-all',200,1],['post','session/mfa/enrollment',200,0],['post','session/mfa/enrollment/{id}/confirm',200,1],['post','session/recovery/request',202,0],['post','session/recovery/complete',204,0]];
const statuses=[400,401,403,404,409,412,422,428,429,503];
function fixture(row=table[1]){
 const [method,suffix,success,rotation]=row;
 const operation={operationId:'ExecuteFixture','x-justix-auth-transport':1,parameters:[],responses:{},security:[{}, {Cookie:[]}]};
 if(method!=='get')operation.parameters.push({name:'X-CSRF-Token',in:'header',required:true,schema:structuredClone(csrf)});
 if(suffix.includes('{id}'))operation.parameters.push({name:'id',in:'path',required:true,schema:{type:'string'}});
 for(const status of [success,...statuses]) {
  const headers={'Cache-Control':header(cache)};
  if((status===success&&rotation)||(status===401&&method==='get'))headers['X-CSRF-Token']=header(csrf);
  if(status===429)headers['Retry-After']=header(retry);
  operation.responses[status]={description:'QA synthetic',headers,...(status===204?{}:{content:{'application/json':{schema:{$ref:'#/components/schemas/Datum'}}}})};
 }
 return {openapi:'3.1.1',info:{title:'Independent inert parser test',version:'1'},'x-justix-auth-semantics':1,components:{schemas:{Datum:structuredClone(closed)},securitySchemes:{Cookie:{type:'apiKey',in:'cookie',name:'__Host-justix_session'},TLS:{type:'mutualTLS'}}},paths:{['/api/v1/identity/'+suffix]:{[method]:operation}}};
}
const operation=d=>Object.values(Object.values(d.paths)[0])[0];
let positive=0, negative=0, publicCases=0;const rows=[];
function reject(label,change,row){const d=fixture(row);change(d);assert.throws(()=>actual.compile(entry,d),label);negative++;rows.push(label);}
for(const row of table){
 const d=fixture(row), plan=actual.compile(entry,d), op=plan.operations[0];
 assert.deepEqual(plan.authProfile,{version:1,semantics:1});
 assert(Object.isFrozen(plan.authProfile));
 assert.deepEqual(op.statuses,[row[2]]);
 assert.equal(op.authTransport.requestCSRF===null,row[0]==='get');
 assert(!op.parameters.some(p=>p.name==='X-CSRF-Token'));
 assert(!Object.hasOwn(plan.schemas[op.paramName].properties,'X-CSRF-Token'));
 assert(!plan.schemas[op.paramName].required.includes('X-CSRF-Token'));
 for(const status of [row[2],...statuses]){
  const m=op.authTransport.metadata[status];assert.equal(m.csrf,(status===row[2]&&row[3])||(status===401&&row[0]==='get')?'required':'forbidden');
  assert.equal(m.retryAfter,status===429?'required':'forbidden');assert.equal(m.cacheControl,'no-store');
  if(status===429)assert.deepEqual(m.retryAfterWire,{pattern:'^(0|[1-9][0-9]{0,9})$',maximum:2147483647});
  assert(Object.isFrozen(m.headers['Cache-Control'].schema));positive++;
  for(const name of ['Cache-Control','X-CSRF-Token','Retry-After'])reject(`${row[1]} ${status}: toggle ${name}`,doc=>{const hs=operation(doc).responses[status].headers;if(Object.hasOwn(hs,name))delete hs[name];else hs[name]=header(name==='Retry-After'?retry:csrf);},row);
 }
 for(const render of [actual.renderTS,actual.renderGo])assert.throws(()=>render(plan),/not activated/);
 operation(d).responses[row[2]].headers['Cache-Control'].schema.const='public';d.components.securitySchemes.Cookie.name='changed';
 assert.equal(op.authTransport.metadata[row[2]].headers['Cache-Control'].schema.const,'no-store');assert.equal(op.securityRequirements[1].Cookie.name,'__Host-justix_session');positive++;
}
for(const field of ['x-justix-auth-semantics','x-justix-auth-transport'])for(const value of [undefined,null,false,0,2,'1',{},[]])reject(`marker ${field} ${JSON.stringify(value)}`,d=>{const target=field.endsWith('semantics')?d:operation(d);if(value===undefined)delete target[field];else target[field]=value;});
for(const [name,status] of [['Cache-Control',200],['X-CSRF-Token',200],['Retry-After',429]]) {
 for(const [key,value] of [['nullable',false],['description','equivalent?'],['default',0],['enum',[0]],['$ref','#/components/schemas/Datum'],['format','int32']])reject(`${name} closed schema ${key}`,d=>operation(d).responses[status].headers[name].schema[key]=value);
 for(const [key,value] of [['required',false],['description',7],['explode',false],['style','simple'],['$ref','#/components/headers/Any']])reject(`${name} closed Header ${key}`,d=>operation(d).responses[status].headers[name][key]=value);
 reject(`${name} casing`,d=>{const hs=operation(d).responses[status].headers;hs[name.toLowerCase()]=hs[name];delete hs[name];});
}
for(const status of ['0200','2e2','200.0','default','500','206'])reject('invalid status '+status,d=>operation(d).responses[status]=structuredClone(operation(d).responses[200]));
for(const owner of ['retail','commerce','insurance']){assert.throws(()=>actual.compile({...entry,owner},fixture()));negative++;}
for(const name of ['If-Match','Idempotency-Key','x-csrf-token','X_CSRF_Token'])reject('forbidden request '+name,d=>operation(d).parameters.push({name,in:'header',required:true,schema:{type:'string'}}));
reject('request CSRF absent',d=>operation(d).parameters=[]);
reject('request CSRF duplicate',d=>operation(d).parameters.push(structuredClone(operation(d).parameters[0])));
reject('request CSRF optional',d=>operation(d).parameters[0].required=false);
reject('request CSRF nullable',d=>operation(d).parameters[0].schema.type=['string','null']);
reject('request CSRF extra keyword',d=>operation(d).parameters[0].schema.maxLength=4096);
for(const security of [[{Unknown:[]}],[{Cookie:['scope']}],[{TLS:[]}],{}])reject('invalid security '+JSON.stringify(security),d=>operation(d).security=security);
const findings=[];
for(const inherited of [false,true]){
 const d=fixture();operation(d).security=null;if(inherited)d.security=[{Cookie:[]}];
 const p=actual.compile(entry,d);assert.deepEqual(p.operations[0].securityRequirements,inherited?[{Cookie:d.components.securitySchemes.Cookie}]:[]);
 findings.push({field:'operation.security',input:null,inherited,acceptedSecurity:p.operations[0].securityRequirements});
}
{
 const d=fixture(table[0]);operation(d).parameters=null;
 const p=actual.compile(entry,d);assert.deepEqual(p.operations[0].parameters,[]);
 findings.push({field:'GET operation.parameters',input:null,acceptedParameters:p.operations[0].parameters});
}
reject('internal auth',d=>{d.paths={'/internal/v1/identity/session/login':{post:operation(d)}};operation(d).security=[{TLS:[]}];});
reject('unsupported auth route',d=>d.paths={'/api/v1/identity/session/other':{post:operation(d)}});
reject('unsupported auth method',d=>d.paths={'/api/v1/identity/session/login':{delete:operation(d)}});
reject('GET CSRF request',d=>operation(d).parameters=[{name:'X-CSRF-Token',in:'header',required:true,schema:csrf}],table[0]);
reject('GET missing anonymous 401',d=>delete operation(d).responses[401],table[0]);
reject('orphan semantics',d=>{delete operation(d)['x-justix-auth-transport'];d.paths={'/api/v1/identity/ordinary':{post:operation(d)}};});

function legacy(internal=false){
 const d={openapi:'3.1.0',info:{title:'Independent legacy fixture',version:'1'},components:{schemas:{Datum:{type:'object',additionalProperties:false,properties:{value:{type:'number'},label:{type:['string','null']}},required:['value','label']}},securitySchemes:{TLS:{type:'mutualTLS'}}},paths:{}};
 const r={description:'Data',content:{'application/json':{schema:{$ref:'#/components/schemas/Datum'}}}};
 d.paths[(internal?'/internal':'/api')+'/v1/identity/ordinary']={get:{operationId:'ReadOrdinary',...(internal?{security:[{TLS:[]}]}:{}),responses:{200:r}}};
 return d;
}
// Profile 1 supports integer, not arbitrary number schemas; use legacy finite numeric decoder beneath integer schema.
const legacyHashes=[];
for(const internal of [false,true]){
 const d=legacy(internal);d.components.schemas.Datum.properties.value={type:'integer',minimum:0,maximum:100};
 const a=actual.compile(entry,d),b=prior.compile(entry,d);assert(!('authProfile' in a));assert(!('authTransport' in a.operations[0]));
 for(const render of ['renderGo','renderTS']){const one=actual[render](a),two=prior[render](b);assert.equal(one,two);legacyHashes.push({internal,render,sha256:createHash('sha256').update(one).digest('hex')});}
}
const generic=legacy();generic.components.schemas.Datum.properties.value={type:'integer',minimum:0,maximum:100};
const write=(name,value)=>fs.writeFileSync(path.join(directory,name),typeof value==='string'||Buffer.isBuffer(value)?value:JSON.stringify(value));
write('legacy.json',generic);write('auth.json',fixture());
const ordinary={...entry,input:'legacy.json',goOutput:'ordinary.go',tsOutput:'ordinary.ts',namespace:'Ordinary'};
const config={version:1,contracts:[ordinary]};write('config.json',config);
function cli(extra=[],env={}){return spawnSync(process.execPath,[path.join(root,'tools/generate-contracts.mjs'),'--config',path.join(directory,'config.json'),...extra],{encoding:'utf8',env:{...process.env,...env}});}
assert.equal(cli().status,0);write('auth.go','// Code generated by justix-contracts/1; DO NOT EDIT.\nQA sentinel');write('auth.ts','// Code generated by justix-contracts/1; DO NOT EDIT.\nQA sentinel');
const names=['ordinary.go','ordinary.ts','auth.go','auth.ts'];const before=names.map(n=>fs.readFileSync(path.join(directory,n)));
generic.info.title='Drift before forbidden auth';write('legacy.json',generic);config.contracts.push(entry);write('config.json',config);
for(const [extra,env] of [[[],{}],[['--check'],{}],[['--enable-auth'],{}],[['--auth'],{}],[[],{JUSTIX_AUTH_GENERATION:'1',JUSTIX_CONTRACTS_AUTH:'1',JUSTIX_AUTH_PROFILE:'1'}]]){
 const r=cli(extra,env);assert.notEqual(r.status,0);if(!extra.length||extra[0]==='--check')assert.match(r.stderr,/not activated/);
 names.forEach((n,i)=>assert.deepEqual(fs.readFileSync(path.join(directory,n)),before[i]));publicCases++;
}
for(const value of [{...config,auth:true},{...config,version:2},{...config,contracts:[{...ordinary,auth:true},entry]}]){write('config.json',value);assert.notEqual(cli().status,0);names.forEach((n,i)=>assert.deepEqual(fs.readFileSync(path.join(directory,n)),before[i]));publicCases++;}
write('config.json',{version:1,contracts:[{...ordinary,goOutput:'absent/one.go',tsOutput:'absent/one.ts'},{...entry,goOutput:'absent/two.go',tsOutput:'absent/two.ts'}]});assert.notEqual(cli().status,0);assert(!fs.existsSync(path.join(directory,'absent')));publicCases++;
// Legacy guards through the real public CLI, each preserving existing outputs.
write('config.json',{version:1,contracts:[ordinary]});
for(const raw of ['{"openapi":"3.1.0","openapi":"3.1.0"}',Buffer.from([0x7b,0x22,0xff,0x22,0x3a,0x31,0x7d])]){write('legacy.json',raw);assert.notEqual(cli().status,0);names.forEach((n,i)=>assert.deepEqual(fs.readFileSync(path.join(directory,n)),before[i]));publicCases++;}
write('legacy.json',generic);fs.symlinkSync(path.join(directory,'ordinary.go'),path.join(directory,'link.go'));write('config.json',{version:1,contracts:[{...ordinary,goOutput:'link.go'}]});assert.notEqual(cli().status,0);assert.deepEqual(fs.readFileSync(path.join(directory,'ordinary.go')),before[0]);publicCases++;
console.log(JSON.stringify({verdict:'BOUNCE',sha,directory,positiveMatrixAndSnapshotChecks:positive,negativePrivateCases:negative,publicCLIRejectionCases:publicCases,legacyHashes,findings,rows},null,2));
