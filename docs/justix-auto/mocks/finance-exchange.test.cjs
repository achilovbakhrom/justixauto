const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
function setup(){
  let raw=null,full=false,queue=Promise.resolve();
  const context=vm.createContext({localStorage:{getItem:()=>raw,setItem:(k,v)=>{if(full)throw Error('quota');raw=v;}},navigator:{locks:{request:(k,fn)=>{const next=queue.then(fn);queue=next.catch(()=>{});return next;}}},window:{dispatchEvent(){}},Event:class{}});
  const P=vm.runInContext(fs.readFileSync(path.join(__dirname,'partner-finance-domain.js'),'utf8')+'\nPartnerFinance',context);
  const E=vm.runInContext(fs.readFileSync(path.join(__dirname,'finance-exchange.js'),'utf8')+'\nFinanceExchange',context);
  const db=E.initial();
  db.sales.push({id:'FIN1',companyId:'COM-D-01',owner:'Seller',revision:1,activities:[],documents:[],partnerFinance:{providerId:'demo-bank',programId:'bank-standard',programVersion:1,status:'draft',calculation:P.calculate({price:25400,downPayment:5080,markupBps:1800,months:24,firstDue:'2026-10-31',minDownPercent:20})}});
  return {P,E,db,s:db.sales[0],full:()=>{full=true;}};
}
test('draft invisible to financier; company/provider/revision guards',()=>{
  const {E,db}=setup();
  assert.throws(()=>E.decide(db,'demo-bank','FIN1',1,'take'));
  assert.throws(()=>E.submit(db,'FIN1','COM-D-02'));
  E.submit(db,'FIN1','COM-D-01');
  assert.throws(()=>E.decide(db,'demo-mfo','FIN1',2,'take'));
  assert.throws(()=>E.decide(db,'demo-bank','FIN1',1,'take'));
  assert.throws(()=>E.submit(db,'FIN1','COM-D-01'));
});
test('round trip: submit, review, request, reply/file, terms, counter, revised terms, agree',()=>{
  const {E,db,s}=setup();
  E.submit(db,s.id,s.companyId);E.decide(db,'demo-bank',s.id,s.revision,'take');
  assert.equal(s.partnerFinance.status,'review');
  E.decide(db,'demo-bank',s.id,s.revision,'info',{note:'Уточните взнос'});
  assert.throws(()=>E.respond(db,'COM-D-02',s.id,s.revision,'reply',{note:'test'}));
  E.respond(db,s.companyId,s.id,s.revision,'reply',{note:'Взнос подтверждён',file:{name:'demo.pdf',data:'data:application/pdf;base64,JVBERg=='}});
  assert.equal(s.documents.length,1);
  const offer={note:'Условия согласованы организацией',downPayment:6000,months:24,markupBps:1700,firstDue:'2026-10-31'};
  E.decide(db,'demo-bank',s.id,s.revision,'terms',offer);
  assert.equal(s.partnerFinance.calculation.markupBps,1800);
  assert.equal(s.partnerFinance.offer.calculation.markupBps,1700);
  assert.throws(()=>E.respond(db,s.companyId,s.id,s.revision,'agree',{confirm:false}));
  E.respond(db,s.companyId,s.id,s.revision,'counter',{note:'Просим 12 месяцев'});
  E.decide(db,'demo-bank',s.id,s.revision,'terms',{...offer,months:12,markupBps:1000});
  assert.equal(s.partnerFinance.offer.version,2);
  E.respond(db,s.companyId,s.id,s.revision,'agree',{confirm:true});
  assert.equal(s.partnerFinance.status,'agreed');assert.equal(s.activities.length,8);
  assert.throws(()=>E.decide(db,'demo-bank',s.id,s.revision,'decline',{note:'late'}));
});
test('decline requires reason and is terminal',()=>{
  const {E,db,s}=setup();E.submit(db,s.id,s.companyId);E.decide(db,'demo-bank',s.id,s.revision,'take');
  assert.throws(()=>E.decide(db,'demo-bank',s.id,s.revision,'decline',{note:' '}));
  E.decide(db,'demo-bank',s.id,s.revision,'decline',{note:'Демо-причина'});
  assert.equal(s.partnerFinance.status,'declined');
  assert.throws(()=>E.decide(db,'demo-bank',s.id,s.revision,'take'));
});
test('publication, drafts and versions do not rewrite submitted snapshots',()=>{
  const {E,db,s}=setup();
  const x=E.programSave(db,'demo-bank',{name:'Новая программа',minDownPercent:25,terms:[{months:12,markupBps:900}]});
  assert.equal(x.status,'draft');E.publish(db,'demo-bank',x.id,1);assert.equal(x.status,'published');
  assert.throws(()=>E.programSave(db,'demo-bank',{name:'Oops',minDownPercent:20,terms:[]},x.id));
  assert.throws(()=>E.publish(db,'demo-bank',x.id,1));
  E.submit(db,s.id,s.companyId);const before=JSON.stringify(s.partnerFinance.calculation);
  E.publish(db,'demo-bank','bank-standard',1);
  E.programSave(db,'demo-bank',{name:'Changed',minDownPercent:30,terms:[{months:12,markupBps:1500}],version:2},'bank-standard');
  assert.equal(JSON.stringify(s.partnerFinance.calculation),before);
  assert.equal(s.partnerFinance.programVersion,1);
});
test('withdrawn/stale program or partner access blocks draft submission',()=>{
  for(const mutate of [(E,db)=>E.publish(db,'demo-bank','bank-standard',1),(E,db)=>{db.providers[0].partnerIds=[];},(E,db)=>{db.providers[0].programs[0].version++;}]){
    const {E,db,s}=setup();mutate(E,db);assert.throws(()=>E.submit(db,s.id,s.companyId));assert.equal(s.partnerFinance.status,'draft');
  }
});
test('program input validation rejects empty, invalid rates and duplicate terms',()=>{
  const {E,db}=setup();const good={name:'Program',minDownPercent:20,terms:[{months:12,markupBps:900}]};
  for(const bad of [{name:''},{minDownPercent:''},{minDownPercent:100},{terms:[]},{terms:[{months:5,markupBps:1}]},{terms:[{months:12,markupBps:-1}]},{terms:[{months:12,markupBps:10001}]},{terms:[{months:12,markupBps:1},{months:12,markupBps:1}]}])assert.throws(()=>E.programSave(db,'demo-bank',{...good,...bad}));
});
test('invalid attachment rejected without changing state',()=>{
  const {E,db,s}=setup();E.submit(db,s.id,s.companyId);E.decide(db,'demo-bank',s.id,s.revision,'take');E.decide(db,'demo-bank',s.id,s.revision,'info',{note:'file'});
  assert.throws(()=>E.respond(db,s.companyId,s.id,s.revision,'reply',{note:'test',file:{data:'data:text/html;base64,AAAA'}}));
  assert.equal(s.partnerFinance.status,'needs-info');assert.equal(s.documents.length,0);
});
test('shared transactions are serial and failed changes/storage leave persisted data intact',async()=>{
  const {E,db,full}=setup();await E.transact(d=>Object.assign(d,db));
  const results=await Promise.allSettled([E.transact(d=>E.submit(d,'FIN1','COM-D-01')),E.transact(d=>E.submit(d,'FIN1','COM-D-01'))]);
  assert.equal(results.filter(x=>x.status==='fulfilled').length,1);
  const before=JSON.stringify(E.read());
  await assert.rejects(E.transact(d=>{d.sales=[];throw Error('fail');}));assert.equal(JSON.stringify(E.read()),before);
  full();await assert.rejects(E.transact(d=>{d.sales=[];}));assert.equal(JSON.stringify(E.read()),before);
});
