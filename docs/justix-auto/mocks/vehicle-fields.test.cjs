const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const fields = vm.runInNewContext(fs.readFileSync(path.join(__dirname,'vehicle-fields.js'),'utf8')+'\nVehicleFields;', {document:{addEventListener(){}}});
test('nine specification fields use five autocompletes and four selects',()=>{
  const html=fields.fields();
  assert.equal((html.match(/<datalist /g)||[]).length,5);
  assert.equal((html.match(/<select /g)||[]).length,4);
  assert.equal((html.match(/<label for=/g)||[]).length,9);
  assert.equal((html.match(/name="/g)||[]).length,9);
});
test('model and trim dictionaries depend on the selected brand and model',()=>{
  assert.deepEqual(Array.from(fields.models('Li Auto')),['L7 Pro','L9']);
  assert.deepEqual(Array.from(fields.trims('Zeekr','001')),['YOU · 100 kWh','WE']);
  assert.deepEqual(Array.from(fields.trims('BYD','001')),[]);
});
test('lookup tolerates casing and whitespace, unknown values remain custom input',()=>{
  assert.deepEqual(Array.from(fields.models(' zeekr ')),['001','X']);
  assert.deepEqual(Array.from(fields.models('Custom brand')),[]);
});
test('multiple renders do not reuse input and datalist IDs',()=>{
  const first=[...fields.fields().matchAll(/ id="([^"]+)"/g)].map(x=>x[1]);
  const second=[...fields.fields().matchAll(/ id="([^"]+)"/g)].map(x=>x[1]);
  assert.equal(new Set(first).size,first.length);
  assert.equal(first.some(id=>second.includes(id)),false);
});
