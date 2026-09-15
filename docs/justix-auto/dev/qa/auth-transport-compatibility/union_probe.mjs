// Independent composition probe. It deliberately retains T640's catch-all union
// pattern to show why semantic/configuration errors need a distinct outcome.
import assert from 'node:assert/strict';
class Mismatch extends Error {}
class BindingDefect extends Error {}
const oldUnion = variants => {
  let matches=0;
  for(const validate of variants) {try {validate();matches++;} catch {/* existing shape */}}
  if(matches!==1)throw new Mismatch();
};
const revisedUnion = variants => {
  let matches=0;
  for(const validate of variants) {
    try {if(validate()!==undefined)throw new BindingDefect('non-synchronous result');matches++;}
    catch(error){if(!(error instanceof Mismatch))throw error;}
  }
  if(matches!==1)throw new Mismatch();
};
const pass=()=>undefined;
// A schema mismatch is legitimately a nonmatching branch.
assert.doesNotThrow(()=>revisedUnion([()=>{throw new Mismatch()},pass]));
// A broken binding is not evidence that this branch does not match.
for(const failure of [new BindingDefect('missing policy'),new TypeError('binding defect')]) {
  assert.doesNotThrow(()=>oldUnion([()=>{throw failure},pass]));
  assert.throws(()=>revisedUnion([()=>{throw failure},pass]),error=>error===failure);
}
// Even a regular (non-async) function can return a thenable/Promise. It must not
// count as a valid branch or be converted into a harmless alternative mismatch.
for(const asynchronous of [()=>Promise.resolve(),()=>({then(){}})]) {
  assert.throws(()=>revisedUnion([asynchronous,pass]),BindingDefect);
}
assert.throws(()=>revisedUnion([pass,pass]),Mismatch);
assert.throws(()=>revisedUnion([()=>{throw new Mismatch()}]),Mismatch);
console.log(JSON.stringify({result:'PASS',cases:7,assertions:9,observation:'catch-all branch handling hides binding defects; revised contract needs explicit mismatch versus fatal binding failure',limits:'isolated feasibility model, not an installed generator or an auth schema fixture'}));
