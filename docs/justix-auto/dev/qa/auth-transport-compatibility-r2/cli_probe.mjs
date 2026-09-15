// Exercise the actual unchanged generator's prewrite guard with a later invalid
// contract; the proposed serial implementation must preserve this until VERIFY.
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,mkdtempSync} from 'node:fs';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../../../../..');
const temp=mkdtempSync('/private/tmp/justix-auth-r2-cli-');
const fixture={openapi:'3.1.0',info:{title:'Synthetic QA',version:'1'},components:{schemas:{Payload:{type:'string'}}},paths:{'/api/v1/identity/synthetic':{get:{operationId:'ReadSynthetic',responses:{'200':{description:'Synthetic',content:{'application/json':{schema:{$ref:'#/components/schemas/Payload'}}}}}}}}};
const entries=['First','Second'].map((namespace,i)=>({input:`${i}.json`,goOutput:`${i}.go`,tsOutput:`${i}.ts`,goPackage:'synthetic',owner:'identity',namespace}));
for(let i=0;i<2;i++)writeFileSync(path.join(temp,`${i}.json`),JSON.stringify(fixture));
writeFileSync(path.join(temp,'config.json'),JSON.stringify({version:1,contracts:entries}));
const run=extra=>spawnSync(process.execPath,[path.join(root,'tools/generate-contracts.mjs'),'--config',path.join(temp,'config.json'),...extra],{encoding:'utf8',env:{...process.env,GOPROXY:'off',GOTOOLCHAIN:'local',GOCACHE:'/private/tmp/justixauto-integration-gocache',GOMODCACHE:'/private/tmp/justixauto-t003-modcache'}});
let result=run([]);assert.equal(result.status,0,result.stderr);
const files=entries.flatMap(e=>[e.goOutput,e.tsOutput]);
const mutations=[
 ['semantic declaration',d=>d['x-justix-auth-semantics']=1],
 ['transport declaration',d=>d.paths['/api/v1/identity/synthetic'].get['x-justix-auth-transport']=1],
 ['response header',d=>d.paths['/api/v1/identity/synthetic'].get.responses['200'].headers={'X-CSRF-Token':{required:true,schema:{type:'string'}}}],
 ['429',d=>d.paths['/api/v1/identity/synthetic'].get.responses['429']=structuredClone(d.paths['/api/v1/identity/synthetic'].get.responses['200'])],
 ['request CSRF',d=>d.paths['/api/v1/identity/synthetic'].get.parameters=[{in:'header',name:'X-CSRF-Token',required:true,schema:{type:'string'}}]],
];
const results=[];
for(const [name,mutate] of mutations){
 const altered=structuredClone(fixture);mutate(altered);writeFileSync(path.join(temp,'1.json'),JSON.stringify(altered));
 for(const extra of [[],['--check']]){
  for(const file of files)writeFileSync(path.join(temp,file),'synthetic retained sentinel\n');
  result=run(extra);assert.notEqual(result.status,0,name);
  for(const file of files)assert.equal(readFileSync(path.join(temp,file),'utf8'),'synthetic retained sentinel\n');
  results.push({case:name,mode:extra.length?'check':'generate',exit:result.status,diagnostic:result.stderr.trim()});
 }
}
console.log(JSON.stringify({result:'PASS',cases:results.length,results,temp,limits:'current CLI baseline; future intermediate rejection and terminal activation still require their exact implementation QA'},null,2));
