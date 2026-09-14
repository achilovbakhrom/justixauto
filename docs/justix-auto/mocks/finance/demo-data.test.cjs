const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
function setup(){
  let raw=null,full=false,queue=Promise.resolve();
  const context=vm.createContext({localStorage:{getItem:()=>raw,setItem:(k,v)=>{if(full)throw Error('quota');raw=v;}},navigator:{locks:{request:(k,fn)=>{const task=queue.then(fn);queue=task.catch(()=>{});return task;}}},window:{dispatchEvent(){}},Event:class{}});
  for(const file of ['../partner-finance-domain.js','../finance-exchange.js','../finance-documents-domain.js','demo-file.js','demo-data.js'])vm.runInContext(fs.readFileSync(path.join(__dirname,file),'utf8'),context);
  return {...vm.runInContext('({E:FinanceExchange,D:FinanceDemoData})',context),full:()=>{full=true;}};
}
test('both organizations receive all six application statuses plus a document example',()=>{
  const {E,D}=setup(),db=E.initial();assert.equal(D.add(db),14);
  for(const p of db.providers){
    const sales=db.sales.filter(s=>s.partnerFinance.providerId===p.id);
    assert.deepEqual(Array.from(sales,s=>s.partnerFinance.status),['submitted','review','needs-info','terms','agreed','declined','agreed']);
    for(const s of sales){
      const f=s.partnerFinance,c=f.offer?.calculation||f.calculation;
      assert.equal(c.schedule.reduce((n,r)=>n+r.paymentCents,0)+c.downCents,c.totalCents);
      assert.equal(s.label,E.labels[f.status]);assert.ok(s.activities.length>=2);
      assert.equal(s.vehicleSnapshot.id,s.vehicleId);assert.equal(s.customerSnapshot.id,s.customerId);
      assert.match(s.vin,/^[A-HJ-NPR-Z0-9]{17}$/);assert.equal(s.completed,false);assert.equal(s.paid,'0 USD');
      if(f.status==='needs-info')assert.ok(f.requestNote);
      if(f.status==='declined')assert.ok(f.declineReason);
      if(['terms','agreed'].includes(f.status))assert.ok(f.offer);
    }
  }
  assert.equal(new Set(db.sales.map(s=>s.vin)).size,14);
  assert.equal(new Set(db.sales.map(s=>s.customerId)).size,14);
});
test('examples remain interactive through normal domain actions',()=>{
  const {E,D}=setup(),db=E.initial();D.add(db);
  const s=db.sales[0];E.decide(db,s.partnerFinance.providerId,s.id,s.revision,'take');assert.equal(s.partnerFinance.status,'review');
  const info=db.sales[2];E.respond(db,info.companyId,info.id,info.revision,'reply',{note:'Взнос из накоплений'});assert.equal(info.partnerFinance.status,'review');
  const terms=db.sales[3];E.respond(db,terms.companyId,terms.id,terms.revision,'agree',{confirm:true});assert.equal(terms.partnerFinance.status,'agreed');
});
test('additive one-time migration preserves existing sales, programs and edited examples',()=>{
  const {E,D}=setup(),db=E.initial();
  db.sales.push({id:'USER-SALE',revision:77,notes:'Keep'});
  db.providers[0].programs[0].name='User-edited program';db.providers[0].programs[0].status='draft';
  const providers=JSON.stringify(db.providers),original=JSON.stringify(db.sales[0]);
  D.add(db);assert.equal(JSON.stringify(db.providers),providers);assert.equal(JSON.stringify(db.sales[0]),original);
  db.sales[1].client='Edited by user';const before=JSON.stringify(db);
  assert.equal(D.add(db),0);assert.equal(JSON.stringify(db),before);
});
test('ID collisions are preserved without duplicating or overwriting the existing record',()=>{
  const {E,D}=setup(),db=E.initial();db.sales.push({id:'FIN-DEMO-B01',revision:90});
  assert.equal(D.add(db),13);assert.equal(db.sales[0].revision,90);assert.equal(db.sales.length,14);
});
test('concurrent startup is idempotent; later startup performs no storage write',async()=>{
  const {E,D}=setup();await Promise.all([D.ensure(),D.ensure()]);
  assert.equal(E.read().sales.length,14);const before=JSON.stringify(E.read());
  await D.ensure();assert.equal(JSON.stringify(E.read()),before);
});
test('storage failure never resets persisted data or marks the migration complete',async()=>{
  const {E,D,full}=setup();await E.transact(db=>{db.sales.push({id:'KEEP'});});
  const before=JSON.stringify(E.read());full();await assert.rejects(D.ensure());
  assert.equal(JSON.stringify(E.read()),before);assert.equal(E.read().demoSamplesVersion,undefined);
});
test('v1 migration adds only new document examples, preserving edited and removed old examples',()=>{
  const {E,D}=setup(),db=E.initial();D.add(db);
  db.sales=db.sales.filter(s=>!s.id.endsWith('07')&&s.id!=='FIN-DEMO-B01');
  db.demoSamplesVersion=1;db.sales[0].client='User changed this';
  const old=JSON.stringify(db.sales);
  assert.equal(D.add(db),2);
  assert.equal(JSON.stringify(db.sales.slice(0,-2)),old);
  assert.equal(db.sales.some(s=>s.id==='FIN-DEMO-B01'),false);
  assert.equal(D.add(db),0);
});
test('document examples cover every status, with usable JPEG fixture and immutable version history',()=>{
  const {E,D}=setup(),db=E.initial();D.add(db);
  for(const s of db.sales.filter(s=>s.financeDocuments)){
    assert.deepEqual(Array.from(s.financeDocuments,d=>d.status),['requested','review','changes','accepted','cancelled']);
    const accepted=s.financeDocuments.find(d=>d.status==='accepted');
    assert.equal(accepted.versions.length,2);
    assert.equal(accepted.versions[0].review.result,'return');
    assert.equal(accepted.versions[1].review.result,'accept');
    const bytes=Buffer.from(accepted.versions[0].data.split(',')[1],'base64');
    assert.equal(bytes.subarray(0,3).toString('hex'),'ffd8ff');
    assert.ok(accepted.versions[0].data.startsWith('data:image/jpeg;base64,'));
    assert.equal(bytes.length,accepted.versions[0].size);
    assert.ok(bytes.length<1048576);
    assert.ok(s.financeDocuments.every(d=>d.history.length>0));
    assert.equal(s.partnerFinance.status,'agreed');assert.equal(s.paid,'0 USD');assert.equal(s.completed,false);
  }
});
