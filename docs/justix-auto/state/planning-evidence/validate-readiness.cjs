// Read-only regression checks for the reviewed dependency traps.
const fs = require('node:fs');
const {tasks} = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const byId = new Map(tasks.map(t => [t.id,t]));
const byKey = new Map(tasks.map(t => [t.key,t]));
const memo = new Map();
function upstream(id, visiting = new Set()) {
  if(memo.has(id)) return memo.get(id);
  if(visiting.has(id)) throw new Error('Cycle at '+id);
  visiting.add(id);const all=new Set();
  for(const d of byId.get(id).depends_on){all.add(d);for(const a of upstream(d,visiting))all.add(a);}
  visiting.delete(id);memo.set(id,all);return all;
}
const traps = [
  ['credentials',['security_release']],
  ['inventory_finalize',['wholesale_policy','coverage_policy']],
  ['customer_create',['coverage_policy','own_money_policy','servicing_policy','program_policy']],
  ['lead_contacts',['coverage_policy','own_money_policy','servicing_policy','program_policy']],
  ['sale_pending',['coverage_policy','own_money_policy','servicing_policy']],
  ['cash_contract',['coverage_policy','own_money_policy','insurance_decision']],
  ['retail_payment',['coverage_policy','own_money_policy','insurance_decision']],
  ['delivery_finalize',['coverage_policy','own_money_policy','wholesale_policy']],
  ['insurance_information',['coverage_policy','own_money_policy']],
  ['insurance_decision',['coverage_policy','own_money_policy']],
  ['finance_information',['coverage_policy','program_policy','live_data_policy']],
  ['document_request',['coverage_policy','program_policy','live_data_policy']],
  ['identity_wire',['security_release','access_policy']],
  ['inventory_wire',['receipt_policy','route_policy','wholesale_policy']],
  ['retail_wire',['coverage_policy','own_money_policy','servicing_policy','registration_policy']],
  ['realization_routes',['coverage_policy','own_money_policy','servicing_policy','registration_policy']],
];
const errors=[],checks=[];
for(const [key,forbidden] of traps){
  const task=byKey.get(key);if(!task){errors.push('Review selector no longer exists: '+key);continue;}
  const closure=upstream(task.id);
  for(const blockedKey of forbidden){
    const blocked=byKey.get(blockedKey);if(!blocked){errors.push('Gate selector no longer exists: '+blockedKey);continue;}
    const pass=!closure.has(blocked.id);checks.push({task:task.id,key,unrelatedGate:blocked.id,pass});
    if(!pass)errors.push(`${task.id} ${key} still waits on ${blocked.id} ${blockedKey}`);
  }
}
const requiredEdges={
 acceptance_11:['credentials_registration','mfa_registration','activation_registration','financing_routes','insurance_routes','ui_2_financing_bind','ui_2_insurance_bind'],
 acceptance_37:['reserve_set_registration','reserve_cancel_registration','retail_lookup_registration','live_authorization_registration'],
 inventory_identity_client:['clients_17'],commerce_reservations_client:['clients_28'],
 retail_reservations_client:['clients_28','clients_37'],
 retail_application_decisions_client:['clients_43','clients_47'],
 documents_authorization_client:['clients_9'],
 inventory_fulfillment_intents_client:['fulfillment_intent_schema','fulfillment_intent_clients']
};
for(const [key,producers] of Object.entries(requiredEdges))for(const producer of producers){
 const task=byKey.get(key),dep=byKey.get(producer);
 const pass=!!task&&!!dep&&upstream(task.id).has(dep.id);
 checks.push({key,requiredProducer:producer,pass});
 if(!pass)errors.push(`${key} lacks required producer ${producer}`);
}
for(const task of tasks.filter(t=>t.key.startsWith('acceptance_'))){
 const closure=upstream(task.id);
 for(const id of closure){const producer=byId.get(id),registration=byKey.get(producer.key+'_registration');
  if(!registration)continue;
  const pass=closure.has(registration.id);checks.push({task:task.id,requiredRegistration:registration.id,pass});
  if(!pass)errors.push(`${task.id} lacks registration ${registration.id}`);
 }
}
console.log(JSON.stringify({checks:checks.length,passed:checks.filter(x=>x.pass).length,errors,note:'Focused graph regressions only; scoped live-policy activation and missing UI still require their own approvals.'},null,2));
if(errors.length)process.exit(1);
