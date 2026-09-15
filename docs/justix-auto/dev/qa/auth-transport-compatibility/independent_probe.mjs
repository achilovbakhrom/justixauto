import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,mkdtempSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../../../..');
const expected = '9ad5a64422b04dbbc8bee969a47acdd3e9278d81';
const run = (args) => execFileSync('git',args,{cwd:root,encoding:'utf8'}).trim();
assert.equal(run(['rev-parse','HEAD']),expected);
const groups=[];
const read=p=>readFileSync(path.join(root,p),'utf8');
const hash=p=>createHash('sha256').update(readFileSync(path.join(root,p))).digest('hex');
const draftPath='docs/justix-auto/state/drafts/architect/auth-transport-compatibility.md';
assert.equal(hash(draftPath),'539aada61e3751130e880fe059c13e92663d716425b60ea981a5a4161dc4670d');
assert.deepEqual(run(['diff','--name-only','476777139ffa6a470c7b3f6e79086c64f2dc7db5',expected]).split('\n').sort(),['docs/justix-auto/dev/results/auth-transport-compatibility.md',draftPath].sort());
groups.push('exact proposal SHA, two-leaf scope and digest');
const tasks=JSON.parse(read('docs/justix-auto/dev/task-index.json')).tasks;
const graph=new Map(tasks.map(t=>[t.id,[...t.depends_on]]));
const draft=read(draftPath);
const aliases=draft.split('\n').filter(l=>l.startsWith('| AT-')).map(l=>l.split('|').slice(1,-1).map(v=>v.trim()));
assert.equal(aliases.length,5);
const leaves=new Map();
const serial=[];
for(const [id,deps,scope] of aliases){
  graph.set(id,deps.split(',').map(v=>v.trim()));
  for(const [,leaf] of scope.matchAll(/`([^`]+\.(?:ts|go|mjs|json))`/g)){
    assert.ok(!leaves.has(leaf),`alias overlap ${leaf}`);leaves.set(leaf,id);
    const old=tasks.filter(t=>t.files_owned.includes(leaf));
    if(old.length){assert.ok(scope.includes('serial successor'),leaf);serial.push({leaf,previous:old.map(t=>t.id),next:id});}
    else assert.ok(!run(['ls-files','--',leaf]),`new leaf exists ${leaf}`);
  }
}
for(const [id,added] of [['T-641',['AT-GEN']],['T-055',['AT-EPOCH','AT-AUTH']],['T-060',['AT-AUTH']]]) graph.get(id).push(...added);
const visited=new Set(),active=new Set();
function visit(id){assert.ok(graph.has(id),`missing ${id}`);if(visited.has(id))return;assert.ok(!active.has(id),`cycle ${id}`);active.add(id);for(const dep of graph.get(id))visit(dep);active.delete(id);visited.add(id);}
for(const id of graph.keys())visit(id);
assert.equal(visited.size,tasks.length+5);
assert.equal(leaves.size,13);
assert.ok(draft.includes('Root alone registers `./authValidation`'));
assert.ok(draft.includes('Root alone adds the auth entry'));
groups.push('five aliases, 13 unique leaves, 939-node proposed DAG, root manifest/config boundaries');

const {createApiClient}=await import(path.join(root,'web/packages/api/src/client.ts'));
const error={error:{code:'BOGUS_BUT_GENERICALLY_VALID',message:'safe',fields:{},traceId:''}};
let fetches=0,schemas=0;
async function current(raw,status=200,extras={},requestExtras={}){
  let options;
  const client=createApiClient({origin:'https://fixture.invalid',fetch:async(_,init)=>{fetches++;options=init;return new Response(raw,{status,headers:{'content-type':'application/json',...extras}})}});
  const result=await client.request({path:'/api/v1/identity/session',schema:{parse(v){schemas++;return v}},successStatuses:[200],...requestExtras});
  return {result,options};
}
for(const [raw,value] of [['{"x":1,"\\u0078":2}',2],['{"x":1.0000000000000001}',1],['{"x":1e-400}',0],['{"x":1.25}',1.25]]) assert.equal((await current(raw)).result.data.x,value);
assert.equal((await current(Uint8Array.from([123,34,120,34,58,34,255,34,125]))).result.data.x,'\ufffd');
const before=schemas;
const known=await current(JSON.stringify(error),401,{'x-csrf-token':'synthetic'});
assert.equal(known.result.kind,'http-error');assert.equal(schemas,before);assert.ok(!JSON.stringify(known.result).includes('synthetic'));
assert.equal((await current(JSON.stringify(error),429)).result.kind,'unexpected-status');
assert.equal((await current('{} {}')).result.kind,'invalid-response');
const start=fetches;
assert.equal((await current('{}',200,{}, {headers:{X_Actor_Id:'x'}})).result.kind,'invalid-request');
assert.equal((await current('{}',200,{}, {path:'https://foreign.invalid/'})).result.kind,'invalid-request');
assert.equal(fetches,start);
assert.deepEqual([known.options.credentials,known.options.mode,known.options.redirect,known.options.cache],['same-origin','same-origin','error','no-store']);
let attempts=0;
const failed=createApiClient({origin:'https://fixture.invalid',fetch:async()=>{attempts++;throw Error('synthetic secret omitted')}});
assert.deepEqual(await failed.request({path:'/api/v1/identity/session/logout',method:'POST',body:{},successStatuses:[204],schema:{parse:v=>v}}),{kind:'transport-error',outcome:'unknown',aborted:false});
assert.equal(attempts,1);
groups.push('actual T032 raw-loss/header-loss/429 mismatch, legacy finite fractions, reserved header and origin isolation, single-attempt unknown write');

assert.equal(new Headers([['X-CSRF-Token','abc'],['x-csrf-token','abc']]).get('x-csrf-token'),'abc, abc');
assert.equal(new Headers({'X-CSRF-Token':' \tabc\t '}).get('x-csrf-token'),'abc');
assert.throws(()=>new Response('{}',{status:204}));
assert.equal(new Response(null,{status:204}).body,null);
assert.equal(new Response('{}',{headers:{'Set-Cookie':'fixture=synthetic'}}).headers.get('set-cookie'),'fixture=synthetic');
const csrf=/^[A-Za-z0-9_-]{1,4096}$/;
for(const v of ['', 'a,b','a, a','abc=','a\nb','a '.repeat(2),'x'.repeat(4097),'abc\n'])assert.equal(csrf.test(v),false,v);
for(const v of ['a','a-B_9','x'.repeat(4096)])assert.ok(csrf.test(v));
const retry=s=>/^(0|[1-9][0-9]{0,9})$/.test(s)&&Number(s)<=2147483647;
for(const v of ['00','01','-1','1.0','1, 2','2147483648','Wed, 21 Oct 2015 07:28:00 GMT','1\n'])assert.equal(retry(v),false,v);
for(const v of ['0','1','2147483647'])assert.ok(retry(v));
for(const bytes of [[0xff],[0xc0,0xaf],[0xe2,0x82],[0xed,0xa0,0x80]])assert.throws(()=>new TextDecoder('utf-8',{fatal:true,ignoreBOM:true}).decode(Uint8Array.from(bytes)));
assert.equal(new TextDecoder('utf-8',{fatal:true,ignoreBOM:true}).decode(Uint8Array.of(239,187,191,123,125)).charCodeAt(0),0xfeff);
groups.push('observable header normalization and syntax boundaries; Node Set-Cookie exposure explicitly cannot prove browser filtering');

let asyncError;
try{await (await import('./interfaces.ts')).exposePrematureSink()}catch(e){asyncError=e}
assert.equal(asyncError?.message,'semantic mismatch');
groups.push('REPRODUCED: void semantic interface permits ordinary Promise-returning validator and premature sink');

const source=read('tools/generate-contracts.mjs');
const template=source.slice(source.indexOf('const goRuntime = ')+18,source.indexOf('\nfunction renderGo')).trim().replace(/;$/,'');
const runtime=Function('return '+template)();
const decoder=runtime.slice(runtime.indexOf('func contractRead('),runtime.indexOf('func contractDecode('));
const goDir=mkdtempSync('/private/tmp/justix-auth-transport-independent-go-');
writeFileSync(path.join(goDir,'go.mod'),'module independentauthprobe\n\ngo 1.27.1\n');
const cases=[['1.0',true],['25e-1',false],['250e-1',true],['1.0000000000000001',false],['9007199254740991',true],['-9007199254740991',true],['9007199254740992',false],['-9007199254740992',false],['0e999999',true],['-0e-999999',true],['1e-400',false],['1e400',false],['1000e-3',true],['1001e-3',false],['{"a":{"x":1,"\\u0078":2}}',false],['{"\\ud800":0}',false],['"\\udc00"',false],['"\\ud83d\\ude00"',true],['{"__proto__":1}',true],['{} []',false],['[1,]',false],['01',false],['true',true],['null',true],['"01"',true],['"\uFFFD"',true]];
writeFileSync(path.join(goDir,'decoder_test.go'),'package independentauthprobe\nimport("bytes";"encoding/json";"errors";"io";"math";"math/big";"strconv";"unicode/utf8";"testing")\nvar contractInvalid=errors.New("invalid")\n'+decoder+'\nfunc TestActualDecoder(t *testing.T){for _,c:=range []struct{raw string;valid bool}{'+cases.map(([s,v])=>'{'+JSON.stringify(s)+','+v+'}').join(',')+'}{t.Run(c.raw,func(t *testing.T){_,err:=contractRead([]byte(c.raw));if (err==nil)!=c.valid{t.Fatalf("valid=%v error=%v",c.valid,err)}})}}\n');
const output={commit:expected,node:process.version,groups,serial,goDir,goCases:cases.length,sourceHashes:{draft:hash(draftPath),client:hash('web/packages/api/src/client.ts'),generator:hash('tools/generate-contracts.mjs'),auth:hash('docs/justix-auto/state/drafts/contracts/auth.md')},verdict:'BOUNCE: synchronous semantic binding is not enforced by proposed void signature and stated runtime checks'};
console.log(JSON.stringify(output,null,2));
