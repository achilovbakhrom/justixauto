import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,mkdtempSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
import {performance} from 'node:perf_hooks';
import {decodeResponseJSON as decode,responseJSONLimits as capture,defaultResponseJSONLimits as defaults} from '../../../../../web/packages/api/src/responseJSON.ts';
const qa=path.dirname(fileURLToPath(import.meta.url)),root=path.resolve(qa,'../../../../..');
const sha='a71115801f57b579398f39f47b1ee4437ea2b537',base='596627e1d17ead2a6e8a818e2db63c046823b0cd';
assert.equal(execFileSync('git',['rev-parse','HEAD'],{cwd:root,encoding:'utf8'}).trim(),sha);
assert.deepEqual(execFileSync('git',['diff','--name-only',base,sha],{cwd:root,encoding:'utf8'}).trim().split('\n').sort(),['docs/justix-auto/dev/results/T-935.md','web/packages/api/src/responseJSON.test.ts','web/packages/api/src/responseJSON.ts'].sort());
const groups={numeric:0,grammar:0,utf8:0,ownership:0,limits:0};
const encoder=new TextEncoder(),bytes=s=>encoder.encode(s);
const call=(s,p='safe-integers',limits)=>decode(bytes(s),p,limits);
const failure=(fn,label='invalid fixture')=>{let error;try{fn()}catch(e){error=e}assert.ok(error instanceof SyntaxError,'Expected SyntaxError for '+JSON.stringify(label));assert.equal(String(error),'SyntaxError: Invalid response JSON');assert.ok(!Object.values(Object.getOwnPropertyDescriptors(error)).some(d=>typeof d.value==='string'&&d.value.includes('SYNTHETIC_PRIVATE_VALUE')))};
let seed=0x3a294051;
function rnd(n){seed=(Math.imul(seed,1664525)+1013904223)>>>0;return seed%n}
// Independent rational oracle. Inputs are bounded here so arbitrary precision
// arithmetic is safe in QA and structurally unlike the saturating product code.
const parity=[];
for(let i=0;i<18000;i++){
 const sign=rnd(2)?'-':'';const len=1+rnd(40);let digits='';for(let k=0;k<len;k++)digits+=String(rnd(10));
 const frac=rnd(len+1);const whole=digits.slice(0,len-frac).replace(/^0+/,'')||'0';
 const mantissa=whole+(frac?'.'+digits.slice(len-frac):'');const exp=rnd(151)-75;
 const source=sign+mantissa+(rnd(2)?'e':'E')+(exp>=0&&rnd(2)?'+':'')+String(exp);
 const coefficient=BigInt(sign+(digits||'0'));const scale=exp-frac;
 const numerator=coefficient*(scale>=0?10n**BigInt(scale):1n),denominator=scale<0?10n**BigInt(-scale):1n;
 const expected=numerator%denominator===0n&&numerator/denominator>=-9007199254740991n&&numerator/denominator<=9007199254740991n;
 let value;try{value=call(source);assert.ok(expected,source);assert.ok(Object.is(value,coefficient===0n&&sign==='-'?-0:Number(numerator/denominator)),source)}catch(e){if(expected)throw e;assert.ok(e instanceof SyntaxError)}
 const finite=JSON.parse(source);if(Number.isFinite(finite))assert.ok(Object.is(call(source,'finite-json'),finite),source);else failure(()=>call(source,'finite-json'));
 groups.numeric++;
 if(i%9===0)parity.push({source,valid:expected,...(expected?{normalized:JSON.stringify(value)}:{})});
}
for(const prefix of ['','-'])for(const near of [9007199254740988n,9007199254740989n,9007199254740990n,9007199254740991n,9007199254740992n,9007199254740993n]){
 for(let trailing=0;trailing<40;trailing++){
  const source=prefix+near+'0'.repeat(trailing)+'e-'+trailing;
  if(near<=9007199254740991n)assert.equal(call(source),Number(prefix+near));else failure(()=>call(source));groups.numeric++;
 }
}
// JSON.parse is the independent grammar oracle for generated single-property
// objects; one-character mutations cannot create a second full object member.
function value(depth=0){
 const which=depth>4?rnd(5):rnd(8);
 if(which===0)return null;if(which===1)return !!rnd(2);if(which===2)return rnd(200)-100;
 if(which===3)return (rnd(400)-200)/8;
 if(which===4)return ['','quoted"slash\\','😀','\u0000\n','é','e\u0301','[[{}]]','\ufeff','a/b'][rnd(9)];
 if(which===5)return [value(depth+1),value(depth+1)];
 if(which===6)return {['key'+depth]:value(depth+1)};
 return [];
}
function admissible(v){if(typeof v==='string')return v.isWellFormed();if(typeof v==='number')return Number.isFinite(v);if(v&&typeof v==='object')return Object.keys(v).every(k=>k.isWellFormed())&&Object.values(v).every(admissible);return true}
const normalize=v=>JSON.stringify(v);
const alphabet='{}[],:"\\ \t\n01.eE+-ntfax';
for(let i=0;i<3500;i++){
 const positive=JSON.stringify(value());const at=rnd(positive.length+1),char=alphabet[rnd(alphabet.length)];
 for(const source of [positive,positive.slice(0,at)+char+positive.slice(at),positive.slice(0,at)+positive.slice(at+1),positive.slice(0,at)+char+positive.slice(at+1)]){
  // Mutation may split a JS surrogate pair. TextEncoder replaces those units;
  // the wire oracle must inspect the actual encoded bytes, not the pre-wire JS.
  let expected,valid=true;try{expected=JSON.parse(new TextDecoder('utf-8',{fatal:true,ignoreBOM:true}).decode(bytes(source)));valid=admissible(expected)}catch{valid=false}
  if(valid)assert.equal(normalize(call(source,'finite-json')),normalize(expected),source);else failure(()=>call(source,'finite-json'),source);
  groups.grammar++;
 }
}
// Exhaustive short grammar with no object member names/Unicode ambiguity.
const shortAlphabet=['[',']','0',',',' ','n','-','.','e'];
function enumerate(prefix,left){if(left===0){let valid=true,expected;try{expected=JSON.parse(prefix);valid=admissible(expected)}catch{valid=false};if(valid)assert.equal(normalize(call(prefix,'finite-json')),normalize(expected),prefix);else failure(()=>call(prefix,'finite-json'));groups.grammar++;return};for(const c of shortAlphabet)enumerate(prefix+c,left-1)}
for(let length=0;length<=4;length++)enumerate('',length);
// Exhaust all two-byte payloads inside a JSON string, independent fatal-decoder
// and JSON.parse oracle, including controls, quotes, escapes and invalid UTF-8.
for(let a=0;a<256;a++)for(let b=0;b<256;b++){
 const input=Uint8Array.of(34,a,b,34);let valid=true,expected;
 try{expected=JSON.parse(new TextDecoder('utf-8',{fatal:true,ignoreBOM:true}).decode(input));valid=admissible(expected)}catch{valid=false}
 if(valid)assert.equal(decode(input,'safe-integers'),expected);else failure(()=>decode(input,'safe-integers'));groups.utf8++;
}
// Decoded-key equality is per object and exact code point sequence.
for(const key of ['x','__proto__','constructor','toString','\u0000','😀','é','e\u0301']){
 const spelling=JSON.stringify(key),escaped='"'+[...key].map(c=>{let out='';for(let i=0;i<c.length;i++)out+='\\u'+c.charCodeAt(i).toString(16).padStart(4,'0');return out}).join('')+'"';
 failure(()=>call('{'+spelling+':1,'+escaped+':2}'));
 const v=call('[{'+spelling+':1},{'+escaped+':2}]');assert.equal(v[0][key],1);assert.equal(v[1][key],2);
 assert.equal(Object.getPrototypeOf(v[0]),null);assert.equal(Object.getOwnPropertyDescriptor(v[0],key).get,undefined);groups.ownership++;
 parity.push({source:'{'+spelling+':1,'+escaped+':2}',valid:false});
}
for(const escaped of ['"\\ud800"','"\\udfff"','"\\ud800x"','{"\\ud800":1}','["\\udc00"]']){failure(()=>call(escaped));parity.push({source:escaped,valid:false})}
assert.equal(call('"\\ud800\\udc00"'),'𐀀');assert.equal(Object.getPrototypeOf(call('{"__proto__":{"polluted":1},"constructor":{"prototype":{"polluted":1}}}')),null);assert.equal({}.polluted,undefined);
const storage=new Uint8Array([255,...bytes('{"a":1}'),255]);assert.equal(call('1'),1);assert.equal(decode(storage.subarray(1,-1),'safe-integers').a,1);
const captured=capture({maxResponseBytes:20,maxJSONDepth:2});assert.ok(Object.isFrozen(captured));assert.notEqual(captured,defaults);
const original={maxResponseBytes:20,maxJSONDepth:2},snapshot=capture(original);original.maxResponseBytes=1;original.maxJSONDepth=1;assert.equal(decode(bytes('[[]]'),'safe-integers',snapshot)[0].length,0);
for(const field of ['maxResponseBytes','maxJSONDepth'])for(const v of [0,-0,-1,.1,NaN,Infinity,-Infinity,Number.MAX_SAFE_INTEGER+1,1n,null,'128',{},true]){
 assert.throws(()=>capture({[field]:v}),TypeError);groups.limits++;
}
for(const depth of [1,2,127,128,129,50000]){
 const source='{"x":'.repeat(depth)+'0'+'}'.repeat(depth);
 if(depth<=128){let v=call(source);for(let i=0;i<depth;i++){assert.equal(Object.getPrototypeOf(v),null);v=v.x}assert.equal(v,0)}else failure(()=>call(source));
 let v=call(source,'safe-integers',{maxJSONDepth:depth});for(let i=0;i<depth;i++)v=v.x;assert.equal(v,0);
 if(depth>1)failure(()=>call(source,'safe-integers',{maxJSONDepth:depth-1}));groups.limits++;
}
const max=8*1024*1024;
assert.equal(call('"'+'x'.repeat(max-2)+'"').length,max-2);
failure(()=>call('"'+'x'.repeat(max-1)+'"'));
assert.equal(decode(bytes('"😀"'),'safe-integers',{maxResponseBytes:6}),'😀');failure(()=>decode(bytes('"😀"'),'safe-integers',{maxResponseBytes:5}));
const Native=globalThis.TextDecoder;let constructed=0;
try{globalThis.TextDecoder=class{constructor(){constructed++;throw Error('unexpected construction')}};failure(()=>decode(Uint8Array.of(34,34,0),'safe-integers',{maxResponseBytes:2}));assert.equal(constructed,0)}finally{globalThis.TextDecoder=Native}
failure(()=>call('{"SYNTHETIC_PRIVATE_VALUE":true,}'));
for(const invalid of [undefined,null,{},'SYNTHETIC_PRIVATE_VALUE'])assert.throws(()=>decode(bytes('0'),invalid),{name:'TypeError',message:'Invalid response JSON numeric profile'});
// Large-token timings are observations on this host, not service SLOs. They
// expose exponent-magnitude work and stack dependence without huge powers.
const observations=[];
for(const n of [200000,1000000]){
 const cases=[['positive exponent','1e'+'9'.repeat(n),false],['negative exponent','1e-'+'9'.repeat(n),false],['zero exponent','-0e'+'9'.repeat(n),true,-0],['integer cancellation','7'+'0'.repeat(n)+'e-'+n,true,7],['fraction cancellation','0.'+'0'.repeat(n)+'7e'+(n+1),true,7]];
 for(const [name,source,valid,want] of cases){const start=performance.now();if(valid)assert.ok(Object.is(call(source),want));else failure(()=>call(source));observations.push({name,bytes:source.length,milliseconds:Number((performance.now()-start).toFixed(2))})}
}
const hashes=Object.fromEntries(['web/packages/api/src/responseJSON.ts','web/packages/api/src/responseJSON.test.ts','web/packages/api/src/client.ts','web/packages/api/package.json','tools/generate-contracts.mjs'].map(p=>[p,createHash('sha256').update(readFileSync(path.join(root,p))).digest('hex')]));
// Only a finite sample of independent oracle cases is used for Go parity;
// Go's old resource behavior and normalized -0 remain separate documented limits.
const temp=mkdtempSync('/private/tmp/justix-t935-independent-parity-');
writeFileSync(path.join(temp,'vectors.json'),JSON.stringify(parity)+'\n');
console.log(JSON.stringify({result:'PASS',commit:sha,node:process.version,seed:'0x3a294051',groups,largeTokenObservations:observations,parityDirectory:temp,parityCases:parity.length,hashes,limits:'No transport integration; Go parity excludes proposed future resource tightening and negative-zero sign'},null,2));
