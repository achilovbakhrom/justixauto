const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm'),path=require('node:path');
const D=vm.runInNewContext(fs.readFileSync(path.join(__dirname,'finance-documents-domain.js'),'utf8')+'\nFinanceDocumentFlow');
const provider={role:'provider',id:'B'},seller={role:'seller',id:'S'};
const file={name:'demo.png',data:'data:image/png;base64,aGVsbG8='};
function fixture(){const s={id:'F',companyId:'S',sellerName:'Seller',owner:'Manager',revision:1,paid:'0 USD',completed:false,activities:[],partnerFinance:{providerId:'B',status:'agreed',offer:{version:1}}};return {sales:[s],providers:[{id:'B',name:'Bank',employee:'Reviewer'}]};}
function run(db,actor,action,input){return D.change(db,actor,'F',db.sales[0].revision,action,input);}
test('request/upload/return/correct/accept preserves previous versions and money state',()=>{
  const db=fixture(),s=db.sales[0];const d=run(db,provider,'request',{title:'Сведения об авто',note:'Нужен VIN'});
  assert.equal(d.status,'requested');assert.equal(s.revision,2);
  run(db,seller,'upload',{documentId:d.id,file});
  assert.equal(d.status,'review');
  run(db,provider,'return',{documentId:d.id,note:'VIN не читается'});
  assert.equal(d.status,'changes');const original=JSON.stringify(d.versions[0]);
  run(db,seller,'upload',{documentId:d.id,file:{...file,name:'corrected.png'},note:'Исправлено'});
  run(db,provider,'accept',{documentId:d.id,confirm:true});
  assert.equal(d.status,'accepted');assert.equal(d.versions.length,2);
  assert.equal(JSON.stringify(d.versions[0]),original);assert.equal(d.history.length,5);
  assert.equal(s.paid,'0 USD');assert.equal(s.completed,false);assert.equal(s.partnerFinance.status,'agreed');assert.equal(s.partnerFinance.offer.version,1);
});
test('wrong organization, role, phase and stale revision rejected',()=>{
  const db=fixture();const input={title:'Title',note:'Details'};
  for(const actor of [{role:'provider',id:'OTHER'},{role:'seller',id:'OTHER'},{role:'admin',id:'B'},seller])assert.throws(()=>run(db,actor,'request',input));
  db.sales[0].partnerFinance.status='terms';assert.throws(()=>run(db,provider,'request',input));
  db.sales[0].partnerFinance.status='agreed';assert.throws(()=>D.change(db,provider,'F',0,'request',input));
  assert.equal(db.sales[0].revision,1);assert.equal(db.sales[0].financeDocuments,undefined);
});
test('validation is atomic; duplicate active requests and repeated uploads rejected',()=>{
  const db=fixture();assert.throws(()=>run(db,provider,'request',{title:'',note:'x'}));
  assert.throws(()=>run(db,provider,'request',{title:'A',note:' '}));
  const d=run(db,provider,'request',{title:'A',note:'Details'});
  assert.throws(()=>run(db,provider,'request',{title:'a',note:'Details'}));
  assert.throws(()=>run(db,provider,'upload',{documentId:d.id,file}));
  assert.throws(()=>run(db,seller,'upload',{documentId:d.id,file:{name:'bad.html',data:'data:text/html;base64,YWJj'}}));
  assert.equal(d.versions.length,0);assert.equal(d.status,'requested');
  run(db,seller,'upload',{documentId:d.id,file});
  assert.throws(()=>run(db,seller,'upload',{documentId:d.id,file}));assert.equal(d.versions.length,1);
});
test('accept requires confirmation, return and cancel require reasons',()=>{
  const db=fixture(),d=run(db,provider,'request',{title:'A',note:'Details'});
  assert.throws(()=>run(db,provider,'cancel',{documentId:d.id}));
  run(db,seller,'upload',{documentId:d.id,file});
  assert.throws(()=>run(db,provider,'accept',{documentId:d.id}));
  assert.throws(()=>run(db,provider,'return',{documentId:d.id,note:' '}));
  assert.throws(()=>run(db,seller,'accept',{documentId:d.id,confirm:true}));
  assert.throws(()=>run(db,provider,'cancel',{documentId:d.id,note:'Cancel'}));
  run(db,provider,'return',{documentId:d.id,note:'Please fix'});
  run(db,provider,'cancel',{documentId:d.id,note:'No longer needed'});
  assert.equal(d.status,'cancelled');assert.equal(d.versions.length,1);
  assert.throws(()=>run(db,seller,'upload',{documentId:d.id,file}));
});
test('file validation enforces actual decoded size and known MIME',()=>{
  assert.equal(D.validFile(file).size,5);
  for(const f of [null,{...file,name:''},{...file,data:'javascript:alert(1)'},{...file,data:'data:image/png;base64,A'},{...file,data:'data:image/png;base64,'+'AAAA'.repeat(350000)}])assert.throws(()=>D.validFile(f));
});
test('shared panel renders different actions for each separate app and escapes user text',()=>{
  const uiContext={FinanceExchange:{},FinanceDocumentFlow:D,document:{createElement:()=>({}),body:{append(){}},addEventListener(){}},window:{addEventListener(){}}};
  const UI=vm.runInNewContext(fs.readFileSync(path.join(__dirname,'finance-documents.js'),'utf8')+'\nFinanceDocuments',uiContext);
  const db=fixture(),s=db.sales[0],d=run(db,provider,'request',{title:'<script>test</script>',note:'Details'});
  let bank=UI.panel(s,'provider','B'),sales=UI.panel(s,'seller','S');
  assert.match(bank,/Запросить документ/);assert.doesNotMatch(sales,/data-fin-doc="request"/);
  assert.match(sales,/Загрузить файл/);assert.doesNotMatch(bank,/data-fin-doc="upload"/);
  assert.match(bank,/&lt;script&gt;/);assert.doesNotMatch(bank,/<script>/);
  run(db,seller,'upload',{documentId:d.id,file});
  bank=UI.panel(s,'provider','B');sales=UI.panel(s,'seller','S');
  assert.match(bank,/Проверить/);assert.doesNotMatch(sales,/data-fin-doc="review"/);
  run(db,provider,'return',{documentId:d.id,note:'Исправьте VIN'});
  assert.match(UI.panel(s,'seller','S'),/Загрузить исправление/);
  assert.match(UI.panel(s,'seller','S'),/Исправьте VIN/);
  s.partnerFinance.status='review';assert.equal(UI.panel(s,'seller','S'),'');
});
