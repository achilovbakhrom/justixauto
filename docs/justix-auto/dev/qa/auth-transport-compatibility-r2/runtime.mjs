// Independent executable model of the revised proposal, not application code.
import assert from 'node:assert/strict';
import vm from 'node:vm';
let assertions=0, scenarios=0;
const eq=(a,b)=>{assert.deepEqual(a,b);assertions++};
const then=Promise.prototype.then;
const outcomes=['valid','mismatch','fatal:policy','fatal:configuration','fatal:binding'];
function discard(value){
  // Intrinsic brand check, without Promise.resolve or arbitrary then access.
  try{Reflect.apply(then,value,[()=>undefined,()=>undefined])}catch{}
}
function call(fn,args=[]){
  let value;
  try{value=Reflect.apply(fn,undefined,args)}catch(thrown){discard(thrown);return 'fatal:binding'}
  if(outcomes.some(o=>value===o))return value;
  discard(value);return 'fatal:binding';
}
function bind(table){
  if(table===null||typeof table!=='object'||Object.getPrototypeOf(table)!==Object.prototype)throw new Error('fatal:binding');
  const functions=Object.create(null);
  for(const name of ['checkReady','A','B','Inner','Root','validateError']){
    const descriptor=Object.getOwnPropertyDescriptor(table,name);
    if(!descriptor||!Object.hasOwn(descriptor,'value')||typeof descriptor.value!=='function')throw new Error('fatal:binding');
    functions[name]=descriptor.value;
  }
  Object.freeze(functions);
  if(ready(functions)!=='valid')throw new Error('not ready');
  return functions;
}
function ready(functions){const r=call(functions.checkReady);return r==='mismatch'?'fatal:binding':r}
function structure(node){
  if(node.type==='leaf'){
    const r=call(node.check);
    return r==='valid'?{outcome:'valid',plan:[node.name]}:{outcome:r,plan:[]};
  }
  if(node.type==='union'){
    const matches=[];
    for(const child of node.children){
      const result=structure(child);
      if(result.outcome==='valid')matches.push(result.plan);
      else if(result.outcome!=='mismatch')return result;
    }
    return matches.length===1?{outcome:'valid',plan:[...matches[0],node.name]}:{outcome:'mismatch',plan:[]};
  }
  if(node.type==='object'){
    const plan=[];
    for(const child of node.children){const result=structure(child);if(result.outcome!=='valid')return result;plan.push(...result.plan)}
    return {outcome:'valid',plan:[...plan,node.name]};
  }
  return {outcome:'fatal:configuration',plan:[]};
}
function validate(functions,tree,error=false){
  let r=ready(functions);if(r!=='valid')return r;
  const parsed=structure(tree);if(parsed.outcome!=='valid')return parsed.outcome;
  for(const name of [...parsed.plan,...(error?['validateError']:[])]){
    r=ready(functions);if(r!=='valid')return r;
    r=call(functions[name],name==='validateError'?['ReadSession',401,{error:{code:'SESSION_REQUIRED'}}]:[{revision:'2',data:{context:{revision:'1'}}}]);
    if(r!=='valid')return r;
  }
  return 'valid';
}
const leaf=(name,outcome='valid')=>({type:'leaf',name,check:()=>outcome});
const unique={type:'object',name:'Root',children:[{type:'union',name:'Inner',children:[leaf('A'),leaf('B','mismatch')]}]};
function fixture(){
  const calls=[];let activeFault=null;
  const table=Object.fromEntries(['checkReady','A','B','Inner','Root','validateError'].map(name=>[name,()=>{calls.push(name);return activeFault?.name===name?activeFault.fn():'valid'}]));
  const bound=bind(table);calls.length=0;
  return {table,bound,calls,fault(name,fn){activeFault={name,fn}}};
}
const unhandled=[];const listener=e=>unhandled.push(e);process.on('unhandledRejection',listener);
let getters=0,thenCalls=0;
const invalids=[
  ['undefined',()=>undefined],['null',()=>null],['boolean',()=>true],['number',()=>1],['symbol',()=>Symbol('valid')],['boxed',()=>new String('valid')],['unknown',()=> 'VALID'],
  ['plain object',()=>({kind:'valid'})],
  ['object getter',()=>({get kind(){getters++;return 'valid'}})],
  ['then getter',()=>({get then(){getters++;throw Error('must not read')}})],
  ['then callable',()=>({then(){thenCalls++;}})],
  ['function then getter',()=>Object.defineProperty(()=>{},'then',{get(){getters++;return ()=>thenCalls++}})],
  ['resolved native',()=>Promise.resolve('valid')],
  ['rejected native',()=>Promise.reject(Error('synthetic secret'))],
  ['async resolved',async()=> 'valid'],
  ['async rejected',async()=>{throw Error('synthetic secret')}],
  ['cross realm rejection',()=>vm.runInNewContext('Promise.reject(Error("synthetic secret"))')],
  ['thrown promise',()=>{throw Promise.reject(Error('synthetic secret'))}],
  ['thrown exception',()=>{throw Error('synthetic secret')}],
];
for(const category of ['checkReady','A','validateError']){
  for(const [name,fn] of invalids){
    const f=fixture();f.fault(category,fn);eq(validate(f.bound,unique,true),'fatal:binding');
    if(category==='checkReady')eq(f.calls,['checkReady']);
    else if(category==='A')eq(f.calls.includes('Root'),false);
    scenarios++;
  }
}
eq(getters,0);eq(thenCalls,0);
for(const outcome of outcomes){
  for(const category of ['checkReady','A','validateError']){
    const f=fixture();f.fault(category,()=>outcome);
    eq(validate(f.bound,unique,true),category==='checkReady'&&outcome==='mismatch'?'fatal:binding':outcome);scenarios++;
  }
}
// Missing policy for the unselected branch is global readiness failure.
{
  const f=fixture();f.fault('checkReady',()=> 'fatal:policy');eq(validate(f.bound,unique),'fatal:policy');eq(f.calls,['checkReady']);scenarios++;
}
// Each selected occurrence including ref, containing union and operation hook
// gets fresh readiness. A later readiness fault must stop before that hook.
for(const stopAt of [2,3,4,5]){
  const f=fixture();let n=0;f.fault('checkReady',()=>++n===stopAt?'fatal:policy':'valid');
  eq(validate(f.bound,unique,true),'fatal:policy');eq(f.calls.filter(n=>n!=='checkReady').length,stopAt-2);scenarios++;
}
// Whole-tree structural ambiguity/configuration faults happen before ANY hook.
for(const broken of [
  {type:'union',name:'Inner',children:[leaf('A'),leaf('B')]},
  {type:'union',name:'Inner',children:[leaf('A','fatal:configuration'),leaf('B')]},
  {type:'union',name:'Inner',children:[{type:'leaf',name:'A',check(){throw Error('broken schema')}},leaf('B')]},
  {type:'union',name:'Inner',children:[leaf('A','mismatch'),leaf('B','mismatch')]},
]){
  const f=fixture();
  const tree={type:'object',name:'Root',children:[leaf('A'),broken]};
  eq(validate(f.bound,tree)!=='valid',true);eq(f.calls,['checkReady']);scenarios++;
}
// No semantic fallback: unselected B must never execute; parent/error after a
// selected mismatch/fault must not execute either.
for(const outcome of ['mismatch','fatal:policy','fatal:configuration','fatal:binding']){
  const f=fixture();f.fault('A',()=>outcome);eq(validate(f.bound,unique,true),outcome);eq(f.calls,['checkReady','checkReady','A']);scenarios++;
}
// Named method references are captured. Mutating the input table cannot remove
// the structural/semantic check or change an accepted factory's dispatch.
{
  const f=fixture();f.table.A=()=>{throw Error('mutable table')};eq(validate(f.bound,unique,true),'valid');
  eq(f.calls,['checkReady','checkReady','A','checkReady','Inner','checkReady','Root','checkReady','validateError']);scenarios++;
}
for(const defect of ['missing','nonfunction','accessor','prototype']){
  const f=fixture();let table=f.table;
  if(defect==='missing')delete table.B;
  if(defect==='nonfunction')table.B=null;
  if(defect==='accessor')Object.defineProperty(table,'B',{get(){getters++;throw Error('must not read')}});
  if(defect==='prototype'){table=Object.create(f.table)}
  assert.throws(()=>bind(table));assertions++;scenarios++;
}
eq(getters,0);
// Safe outward classification is phase-dependent; never return semantic fault
// diagnostics, never retry, and only the current ticket can clear/release state.
{
  const classify=(phase,unsafe)=>phase==='before'?{kind:'invalid-request'}:{kind:'invalid-response',unknown:unsafe};
  eq(classify('before',true),{kind:'invalid-request'});eq(classify('after',true),{kind:'invalid-response',unknown:true});
  eq(classify('after',false),{kind:'invalid-response',unknown:false});
  let epoch=1,slot={epoch:1,id:1},state='old';const old=slot;
  function clear(ticket){if(ticket===slot&&ticket.epoch===epoch)state='uncertain'}
  function finish(ticket){if(ticket===slot)slot=null}
  // Prior fetch settles; new request legitimately obtains the new slot.
  epoch++;finish(old);slot={epoch,id:2};state='current';const current=slot;
  clear(old);finish(old);eq(state,'current');eq(slot,current);
  clear(current);eq(state,'uncertain');finish(current);eq(slot,null);scenarios++;
}
await new Promise(resolve=>setImmediate(resolve));await new Promise(resolve=>setImmediate(resolve));
eq(unhandled.length,0);process.off('unhandledRejection',listener);
console.log(JSON.stringify({result:'PASS',scenarios,assertions,invalidReturnKinds:invalids.length,hookCategories:3,unhandledRejections:unhandled.length,getterCalls:getters,thenCalls,limits:'independent model of exact revised interfaces; no generated auth implementation, server, real cookie or arbitrary plugin sandbox claim'},null,2));
