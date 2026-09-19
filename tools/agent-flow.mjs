#!/usr/bin/env node
/** Durable local packet controller. Actor IDs/evidence are coordinator
 * attestations, not signatures or proof of a human/model identity. */
import fs from 'node:fs';

import path from 'node:path';

import { createHash } from 'node:crypto';

import { execFileSync } from 'node:child_process';

import { fileURLToPath } from 'node:url';


const SHA=/^[a-f0-9]{40}$/, SHA256=/^[a-f0-9]{64}$/, ID=/^[A-Z][A-Z0-9]*(?:-[A-Z0-9]+)*$/;

const ACTOR=/^[A-Za-z0-9][A-Za-z0-9._:@/-]{1,119}$/, KINDS=new Set(['implementation','qa','integration']), RESOURCES=new Set(['browser','postgres']), MAX_PLAN=64*1024;

const fail=(m)=>{throw new Error(m)}, now=()=>new Date().toISOString(), object=(v)=>v!==null&&typeof v==='object'&&!Array.isArray(v);

const stable=(v)=>JSON.stringify(v,(_,x)=>x&&typeof x==='object'&&!Array.isArray(x)?Object.fromEntries(Object.keys(x).sort().map(k=>[k,x[k]])):x);

const hash=(v)=>createHash('sha256').update(Buffer.isBuffer(v)||typeof v==='string'?v:stable(v)).digest('hex');


export function parseJSON(text,label='JSON') { let i=0;
   const ws=()=>{while(/\s/.test(text[i]??''))i++};
   const str=()=>{const s=i++;
    while(i<text.length){if(text[i]==='\\')i+=2;
      else if(text[i++]==='"')return JSON.parse(text.slice(s,i))}fail(`${label}: unterminated string`)};
   const val=()=>{ws();
    if(text[i]==='"')return str();
    if(text[i]==='{'){i++;
      const seen=new Set();
      ws();
      if(text[i]==='}'){i++;
        return}while(true){ws();
        if(text[i]!=='"')fail(`${label}: object key expected`);
        const key=str();
        if(seen.has(key))fail(`${label}: duplicate key ${key}`);
        seen.add(key);
        ws();
        if(text[i++]!==':')fail(`${label}: colon expected`);
        val();
        ws();
        if(text[i]==='}'){i++;
          return}if(text[i++]!==',')fail(`${label}: comma expected`)}}if(text[i]==='['){i++;
      ws();
      if(text[i]===']'){i++;
        return}while(true){val();
        ws();
        if(text[i]===']'){i++;
          return}if(text[i++]!==',')fail(`${label}: comma expected`)}}const t=text.slice(i).match(/^(?:true|false|null|-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)/)?.[0];
    if(!t)fail(`${label}: value expected`);
    i+=t.length};
  val();
  ws();
  if(i!==text.length)fail(`${label}: trailing data`);
  try{return JSON.parse(text)}catch{fail(`${label}: invalid JSON`)}}

function exact(v,keys,where){if(!object(v))fail(`${where}: object required`);
  for(const k of Object.keys(v))if(!keys.includes(k))fail(`${where}: unsupported field ${k}`)}

function id(v,w){if(typeof v!=='string'||!ID.test(v))fail(`${w}: canonical uppercase hyphenated ID required`)}

function sha(v,w){if(typeof v!=='string'||!SHA.test(v))fail(`${w}: lowercase 40-character SHA required`)}

function actor(v,w='actor'){if(typeof v!=='string'||!ACTOR.test(v))fail(`${w}: trusted stable actor ID required`)}

function text(v,w='evidence'){if(typeof v!=='string'||!v||v.length>1024)fail(`${w}: 1..1024 characters required`)}

function array(v,w){if(!Array.isArray(v))fail(`${w}: array required`);
  return v}

function rel(v,w){if(typeof v!=='string'||!v||v.includes('\\')||path.posix.isAbsolute(v)||v.split('/').some(x=>!x||x==='.'||x==='..'||x==='.git')||/[*?\[\]{}]/.test(v))fail(`${w}: safe relative non-glob, non-.git path required`);
  return v}

function contract(v,w){exact(v,['path','sha256'],w);
  rel(v.path,`${w}.path`);
  if(typeof v.sha256!=='string'||!SHA256.test(v.sha256))fail(`${w}.sha256: SHA-256 required`)}

function safeFile(root,relative){const realRoot=fs.realpathSync(root);
  let current=realRoot;
  for(const part of relative.split('/')){const next=path.join(current,part),stat=fs.lstatSync(next);
    if(stat.isSymbolicLink())fail(`contract path uses symlink: ${relative}`);
    current=next}if(!fs.lstatSync(current).isFile())fail(`contract is not a file: ${relative}`);
  return current}

function verifyContracts(m,root){for(const item of [...m.contracts,...m.packets.map(p=>p.brief)])if(hash(fs.readFileSync(safeFile(root,item.path)))!==item.sha256)fail(`contract hash mismatch: ${item.path}`)}

export function validateManifest(m,{root=process.cwd(),verify=true}={}){exact(m,['schema','task_id','planner_actor','hub_actor','hub','base_sha','contracts','requirements','packets'],'manifest');
  if(m.schema!==2)fail('manifest.schema must be 2');
  id(m.task_id,'manifest.task_id');
  actor(m.planner_actor,'manifest.planner_actor');
  actor(m.hub_actor,'manifest.hub_actor');
  if(!['application','infrastructure'].includes(m.hub))fail('manifest.hub must be application or infrastructure');
  sha(m.base_sha,'manifest.base_sha');
  const contracts=array(m.contracts,'manifest.contracts');
  if(!contracts.length)fail('manifest.contracts required');
  const cpaths=new Set();
  for(const c of contracts){contract(c,'manifest.contract');
    if(cpaths.has(c.path))fail(`duplicate contract ${c.path}`);
    cpaths.add(c.path)}const requirements=array(m.requirements,'manifest.requirements');
  if(!requirements.length)fail('manifest.requirements required');
  const req=new Set();
  for(const r of requirements){id(r,'requirement');
    if(req.has(r))fail(`duplicate requirement ${r}`);
    req.add(r)}const packets=array(m.packets,'manifest.packets');
  if(!packets.length)fail('manifest.packets required');
  const ids=new Set(),graph=new Map(),coverage=new Set();
  for(const p of packets){exact(p,['id','kind','depends_on','owned_paths','brief','acceptance','resources'],'packet');
    id(p.id,'packet.id');
    if(ids.has(p.id))fail(`duplicate packet ${p.id}`);
    ids.add(p.id);
    if(!KINDS.has(p.kind))fail(`packet ${p.id}: unsupported kind`);
    const deps=array(p.depends_on,`packet ${p.id}.depends_on`);
    if(new Set(deps).size!==deps.length)fail(`packet ${p.id}: duplicate dependency`);
    const owned=array(p.owned_paths,`packet ${p.id}.owned_paths`);
    if(p.kind==='implementation'&&!owned.length)fail(`packet ${p.id}: implementation ownership required`);
    if(p.kind!=='implementation'&&owned.length)fail(`packet ${p.id}: verification packets cannot own code paths`);
    for(const x of owned)rel(x,`packet ${p.id}.owned_paths`);
    contract(p.brief,`packet ${p.id}.brief`);
    const accepts=array(p.acceptance,`packet ${p.id}.acceptance`);
    if(!accepts.length)fail(`packet ${p.id}: acceptance required`);
    for(const x of accepts){id(x,`packet ${p.id}.acceptance`);
      if(!req.has(x))fail(`packet ${p.id}: unknown requirement ${x}`);
      coverage.add(x)}const resources=array(p.resources,`packet ${p.id}.resources`);
    if(new Set(resources).size!==resources.length||resources.some(x=>!RESOURCES.has(x)))fail(`packet ${p.id}: only browser/postgres resources allowed`);
    graph.set(p.id,deps)}for(const [p,deps] of graph)for(const d of deps)if(!ids.has(d))fail(`packet ${p}: unknown dependency ${d}`);
  const visiting=new Set(),visited=new Set(),walk=(p)=>{if(visiting.has(p))fail(`packet dependency cycle at ${p}`);
    if(visited.has(p))return;
    visiting.add(p);
    for(const d of graph.get(p))walk(d);
    visiting.delete(p);
    visited.add(p)};
  for(const p of ids)walk(p);
  for(const r of req)if(!coverage.has(r))fail(`missing acceptance coverage for ${r}`);
  if(verify)verifyContracts(m,root);
  return structuredClone(m)}

export function gitInfo(worktree){const real=fs.realpathSync(worktree),top=execFileSync('git',['-C',real,'rev-parse','--show-toplevel'],{encoding:'utf8'}).trim();
  if(fs.realpathSync(top)!==real)fail('worktree must name its Git top-level directory');
  const common=fs.realpathSync(path.resolve(real,execFileSync('git',['-C',real,'rev-parse','--git-common-dir'],{encoding:'utf8'}).trim())),head=execFileSync('git',['-C',real,'rev-parse','HEAD'],{encoding:'utf8'}).trim(),dirty=execFileSync('git',['-C',real,'status','--porcelain'],{encoding:'utf8'}).trim();
  if(dirty)fail('worktree must be clean');
  return{worktree:real,common,head}}

export function gitStateDir(cwd=process.cwd()){return path.join(gitInfo(cwd).common,'justix-agent-state')}

function seal(s){const c={...s};
  delete c.integrity;
  return hash(c)}

function readState(dir){try{const s=parseJSON(fs.readFileSync(path.join(dir,'state.json'),'utf8'),'state');
    if(s.schema!==2||!SHA256.test(s.integrity??'')||s.integrity!==seal(s))fail('state integrity mismatch (tampered or incomplete state)');
    return s}catch(e){if(e.code==='ENOENT')fail('agent-flow not initialized');
    throw e}}

function writeState(dir,s){s.integrity=seal(s);
  const tmp=path.join(dir,`.state-${process.pid}-${Date.now()}-${Math.random().toString(16).slice(2)}`);
  fs.writeFileSync(tmp,`${stable(s)}\n`,{mode:0o600});
  fs.renameSync(tmp,path.join(dir,'state.json'))}

function transact(dir,fn){fs.mkdirSync(dir,{recursive:true});
  const lock=path.join(dir,'writer.lock');
  try{fs.mkdirSync(lock,{mode:0o700})}catch(e){if(e.code==='EEXIST')fail('another agent-flow writer is active; retry safely');
    throw e}try{const s=readState(dir),result=fn(s);
    writeState(dir,s);
    return result}finally{fs.rmdirSync(lock)}}

function stateDir(o={}){return o.stateDir??gitStateDir(o.cwd)}

function task(s,tid){id(tid,'task');
  if(!s.tasks[tid])fail(`unknown task ${tid}`);
  return s.tasks[tid]}

function definition(l,pid){const p=l.manifest.packets.find(x=>x.id===pid);
  if(!p)fail(`unknown packet ${pid}`);
  return p}

function entry(l,pid){return l.packets[pid]}

function active(i){return['claimed','submitted','verifying'].includes(i.status)}

function allActive(s){return Object.entries(s.tasks).flatMap(([tid,l])=>Object.entries(l.packets).filter(([,x])=>active(x)).map(([pid,x])=>({tid,pid,item:x})))}

function ready(l,p){return p.depends_on.every(d=>entry(l,d).status==='accepted')}

function requireApproved(l){if(l.plan_review?.verdict!=='GREEN'||l.plan_review.digest!==l.digest)fail('GREEN plan review for exact manifest digest required');
  if(!l.hub_acceptance)fail('separate hub acceptance required')}

function ensureRegistered(s,candidate,expected){const info=gitInfo(candidate);
  if(info.common!==s.common_dir)fail('worktree Git common-dir differs from shared state');
  if(expected&&info.worktree!==expected.worktree)fail('claim worktree mismatch');
  return info}

function headAt(i,expected){if(i.head!==expected)fail('worktree HEAD is stale versus pinned SHA')}

function ancestors(worktree,a,b){try{execFileSync('git',['-C',worktree,'merge-base','--is-ancestor',a,b])}catch{fail(`${a} is not an ancestor of ${b}`)}}

function ownership(p,paths){for(const x of paths){rel(x,'changed path');
    if(!p.owned_paths.some(o=>x===o||x.startsWith(`${o}/`)))fail(`changed path outside packet ownership: ${x}`)}}

function evidence(d){actor(d.actor);
  if(!['GREEN','BOUNCE'].includes(d.verdict))fail('verdict must be GREEN or BOUNCE');
  text(d.evidence)}

export function init(planFile,o={}){if(fs.statSync(planFile).size>MAX_PLAN)fail('plan exceeds 64 KiB');
  const root=path.dirname(fs.realpathSync(planFile)),manifest=validateManifest(parseJSON(fs.readFileSync(planFile,'utf8'),'plan'),{root}),dir=stateDir(o);
  fs.mkdirSync(dir,{recursive:true});
  const lock=path.join(dir,'writer.lock');
  try{fs.mkdirSync(lock)}catch{fail('another agent-flow writer is active; retry safely')}try{let s;
    try{s=readState(dir)}catch(e){if(!/not initialized/.test(e.message))throw e;
      const common=o.commonDir??gitInfo(o.cwd??process.cwd()).common;
      s={schema:2,common_dir:common,tasks:{},worktrees:{},initialized_at:now()}}if(s.tasks[manifest.task_id])fail(`task ${manifest.task_id} already initialized`);
    s.tasks[manifest.task_id]={manifest,root,digest:hash(manifest),plan_review:null,hub_acceptance:null,packets:Object.fromEntries(manifest.packets.map(p=>[p.id,{status:'pending',history:[]}]))};
    writeState(dir,s);
    return{task:manifest.task_id,digest:s.tasks[manifest.task_id].digest,stateDir:dir}}finally{fs.rmdirSync(lock)}}

export function registerWorktree(worktree,o={}){return transact(stateDir(o),s=>{const i=ensureRegistered(s,worktree);s.worktrees[i.worktree]={common:i.common,registered_head:i.head,registered_at:now()};return s.worktrees[i.worktree]})}

export function reviewPlan(tid,d,o={}){evidence(d);
  return transact(stateDir(o),s=>{const l=task(s,tid);if(d.actor===l.manifest.planner_actor)fail('planner cannot independently review plan');if(d.digest!==l.digest)fail('plan review digest does not match manifest');l.plan_review={...d,at:now()};return l.plan_review})}

export function acceptHub(tid,d,o={}){actor(d.actor);
  text(d.evidence);
  return transact(stateDir(o),s=>{const l=task(s,tid);if(d.actor!==l.manifest.hub_actor)fail('only manifest hub_actor may record hub acceptance');if(l.plan_review?.verdict!=='GREEN'||l.plan_review.digest!==l.digest)fail('GREEN plan review required before hub acceptance');l.hub_acceptance={...d,at:now()};return l.hub_acceptance})}

export function claim(tid,kind,d,o={}){
  actor(d.actor);
  if(typeof d.run_id!=='string'||!/^[A-Za-z0-9][A-Za-z0-9._-]{2,119}$/.test(d.run_id)) fail('run ID required');
  return transact(stateDir(o),s=>{
    const l=task(s,tid);
    requireApproved(l);
    const i=ensureRegistered(s,d.worktree);
    const registered=s.worktrees[i.worktree];
    if(!registered) fail('worktree must be registered before claim');
    if(registered.common!==i.common) fail('registered worktree common-dir mismatch');
    const claims=allActive(s);
    if(claims.length>=3) fail('global active packet limit (3) reached');
    if(claims.some(c=>c.item.claim.worktree===i.worktree)) fail('worktree already reserved by active packet');
    const locked=new Set(claims.flatMap(c=>c.item.claim.resources));
    const p=l.manifest.packets.find(x=>x.kind===kind&&entry(l,x.id).status==='pending'&&ready(l,x)&&x.resources.every(r=>!locked.has(r)));
    if(!p) fail('no ready unblocked packet');
    verifyContracts(l.manifest,l.root);
    const item=entry(l,p.id);
    item.status='claimed';
    item.review_base=item.review_base??i.head;
    item.claim={run_id:d.run_id,actor:d.actor,worktree:i.worktree,common:i.common,head:i.head,review_base:item.review_base,resources:p.resources,at:now()};
    item.history.push({event:'claimed',at:now(),run_id:d.run_id,head:i.head});
    return {task:tid,packet:p.id,pinned_sha:i.head,resources:p.resources};
  });
}

export function checkpoint(tid,pid,d,o={}){text(d.note,'note');
  if(!['claimed','submitted','verifying'].includes(d.status))fail('checkpoint status must be claimed, submitted, or verifying');
  return transact(stateDir(o),s=>{const item=entry(task(s,tid),pid);if(!active(item)||item.claim.run_id!==d.run_id)fail('checkpoint does not own active claim');item.status=d.status;item.checkpoint={status:d.status,note:d.note,at:now()};item.history.push({event:'checkpoint',...item.checkpoint});return item.checkpoint})}

export function submit(tid,pid,d,o={}){
  actor(d.actor);
  return transact(stateDir(o),s=>{
    const l=task(s,tid),p=definition(l,pid),item=entry(l,pid);
    if(p.kind!=='implementation') fail('only implementation packets submit code');
    if(item.status!=='claimed'||item.claim.run_id!==d.run_id||item.claim.actor!==d.actor) fail('submission does not own claimed packet');
    const i=ensureRegistered(s,d.worktree,item.claim),head=i.head;
    if(head===item.claim.head) fail('implementation submission requires a new HEAD');
    ancestors(i.worktree,item.claim.review_base,head);
    const paths=execFileSync('git',['-C',i.worktree,'diff','--name-only',`${item.claim.review_base}..${head}`],{encoding:'utf8'}).trim().split('\n').filter(Boolean);
    if(!paths.length) fail('submission requires an actual path diff');
    ownership(p,paths);
    item.submission={before:item.claim.review_base,head,paths,actor:d.actor,at:now()};
    item.status='submitted';
    item.history.push({event:'submitted',head,at:now()});
    return item.submission;
  });
}

function verifier(s,l,p,item,d){const i=ensureRegistered(s,d.worktree,item.claim),pinned=p.kind==='implementation'?item.submission?.head:item.claim.head;
  if(!pinned)fail('implementation requires submitted code before verification');
  headAt(i,pinned);
  const seen=new Set(),walk=(pid)=>{if(seen.has(pid))return;
    seen.add(pid);
    const source=entry(l,pid);
    if(source.submission?.actor===d.actor)fail('verification actor must be independent of upstream implementer');
    for(const dep of definition(l,pid).depends_on)walk(dep)};
  for(const dep of p.depends_on)walk(dep);
  return pinned}

export function report(tid,pid,phase,d,o={}){if(!['review','qa','verify'].includes(phase))fail('report phase must be review, qa, or verify');
  evidence(d);
  return transact(stateDir(o),s=>{const l=task(s,tid),p=definition(l,pid),item=entry(l,pid);if(!active(item))fail('report requires active claim');const pinned=verifier(s,l,p,item,d);if(p.kind==='implementation'&&phase==='verify')fail('implementation uses review and qa reports');if(p.kind!=='implementation'&&phase!=='verify')fail('verification packet uses verify report');if(p.kind==='implementation'&&item.submission.actor===d.actor)fail('submitter cannot independently verify own packet');const key=phase==='verify'?'verification':phase;if(item[key])fail(`${phase} report already recorded`);if(phase==='qa'&&item.review?.actor===d.actor||phase==='review'&&item.qa?.actor===d.actor)fail('review and QA actors must be distinct');item[key]={...d,sha:pinned,at:now()};item.status=d.verdict==='GREEN'?'verifying':'failed';if(d.verdict!=='GREEN'){item.reason=`${phase} ${d.verdict}: ${d.evidence}`;item.claim=null}item.history.push({event:phase,verdict:d.verdict,sha:pinned,at:now()});return{status:item.status,sha:pinned}})}

export function accept(tid,pid,d,o={}){actor(d.actor);
  text(d.evidence);
  return transact(stateDir(o),s=>{const l=task(s,tid),p=definition(l,pid),item=entry(l,pid);requireApproved(l);if(d.actor!==l.manifest.hub_actor)fail('only manifest hub_actor may accept a packet');if(!active(item))fail('accept requires active verified claim');const pinned=verifier(s,l,p,item,d);if(p.kind==='implementation'){if(item.review?.verdict!=='GREEN'||item.qa?.verdict!=='GREEN'||item.review.sha!==pinned||item.qa.sha!==pinned)fail('GREEN review and QA for exact submitted SHA required');if(new Set([item.submission.actor,item.review.actor,item.qa.actor,d.actor]).size!==4)fail('submitter, reviewer, QA, and accept actor must be distinct')}else{if(item.verification?.verdict!=='GREEN'||item.verification.sha!==pinned)fail('GREEN verify report for exact pinned SHA required');if(item.verification.actor===d.actor)fail('verification and accept actor must be distinct')}item.status='accepted';item.acceptance={actor:d.actor,evidence:d.evidence,sha:pinned,at:now()};item.claim=null;item.history.push({event:'accepted',sha:pinned,at:now()});return item.acceptance})}

export function recover(tid,pid,d,o={}){
  text(d.reason,'reason');
  return transact(stateDir(o),s=>{
    const l=task(s,tid),packet=definition(l,pid),item=entry(l,pid);
    const reopenVerification=item.status==='accepted'&&packet.kind!=='implementation';
    if(!active(item)&&!['failed','blocked'].includes(item.status)&&!reopenVerification) fail('only active, failed, blocked, or accepted verification packet can recover');
    item.attempts=[...(item.attempts??[]),{claim:item.claim,submission:item.submission,review:item.review,qa:item.qa,verification:item.verification,reason:d.reason,at:now()}];
    delete item.claim;
    delete item.submission;
    delete item.review;
    delete item.qa;
    delete item.verification;
    delete item.acceptance;
    item.status='pending';
    item.reason=`recovered: ${d.reason}`;
    item.history.push({event:reopenVerification?'reopened':'recovered',reason:d.reason,at:now()});
    const invalidate=(source)=>{
      for(const candidate of l.manifest.packets.filter(x=>x.depends_on.includes(source))){
        const dependent=entry(l,candidate.id);
        if(dependent.status==='accepted'){
          dependent.attempts=[...(dependent.attempts??[]),{acceptance:dependent.acceptance,verification:dependent.verification,reason:`upstream ${source} reopened`,at:now()}];
          delete dependent.acceptance;
          delete dependent.verification;
          dependent.status='pending';
          dependent.reason=`invalidated by upstream ${source} reopening`;
          dependent.history.push({event:'invalidated',source,at:now()});
          invalidate(candidate.id);
        }
      }
    };
    if(reopenVerification) invalidate(pid);
    delete l.aggregate;
    return {status:item.status};
  });
}

export function status(tid,o={}){const s=readState(stateDir(o)),compact=(id,l)=>({task:id,digest:l.digest,plan_review:l.plan_review?.verdict??'PENDING',hub_accepted:Boolean(l.hub_acceptance),packets:Object.fromEntries(Object.entries(l.packets).map(([p,x])=>[p,x.status]))});
  return tid?compact(tid,task(s,tid)):{tasks:Object.fromEntries(Object.entries(s.tasks).map(([id,l])=>[id,compact(id,l)])),active_claims:allActive(s).length,registered_worktrees:Object.keys(s.worktrees).length}}

export function audit(tid,o={}){const s=readState(stateDir(o));
  return tid?task(s,tid):s}

export function aggregate(tid,d,o={}){actor(d.actor);
  text(d.evidence);
  return transact(stateDir(o),s=>{const l=task(s,tid);requireApproved(l);const integrations=l.manifest.packets.filter(p=>p.kind==='integration'),implementations=l.manifest.packets.filter(p=>p.kind==='implementation');if(!integrations.length)fail('integration packet required');for(const p of l.manifest.packets)if(entry(l,p.id).status!=='accepted')fail(`packet ${p.id} is not accepted`);for(const p of integrations){const x=entry(l,p.id);if(x.acceptance.actor===d.actor||x.verification.actor===d.actor)fail('aggregate actor must be separate from integration verification/accept')}const final=ensureRegistered(s,d.worktree);ancestors(final.worktree,l.manifest.base_sha,final.head);for(const p of implementations){const sub=entry(l,p.id).submission??entry(l,p.id).attempts?.at(-1)?.submission;if(!sub)fail(`implementation ${p.id} has no submission receipt`);ancestors(final.worktree,sub.head,final.head);ancestors(final.worktree,l.manifest.base_sha,sub.head)}for(const p of integrations)if(entry(l,p.id).acceptance.sha!==final.head)fail(`integration ${p.id} was not verified at current final HEAD`);const covered=new Set(integrations.flatMap(p=>p.acceptance));for(const r of l.manifest.requirements)if(!covered.has(r))fail(`final integration QA lacks requirement coverage for ${r}`);l.aggregate={final_sha:final.head,actor:d.actor,evidence:d.evidence,at:now()};return l.aggregate})}

function flags(values,allowed,required=[]){const out={_:[]};
  for(let i=0;i<values.length;i++){const v=values[i];
    if(!v.startsWith('--')){out._.push(v);
      continue}const k=v.slice(2);
    if(!allowed.includes(k))fail(`unknown flag --${k}`);
    if(Object.hasOwn(out,k))fail(`duplicate flag --${k}`);
    if(++i>=values.length||values[i].startsWith('--'))fail(`missing value for --${k}`);
    out[k]=values[i]}for(const k of required)if(!Object.hasOwn(out,k))fail(`missing required flag --${k}`);
  return out}

const usage=`Usage: node tools/agent-flow.mjs <command> [arguments]\n\n  init PLAN.json\n  register-worktree --worktree PATH\n  review-plan --task TASK --digest SHA256 --actor ID --verdict GREEN|BOUNCE --evidence TEXT\n  accept-hub --task TASK --actor ID --evidence TEXT\n  claim implementation|qa|integration --task TASK --run ID --actor ID --worktree PATH\n  checkpoint PACKET --task TASK --run ID --status claimed|submitted|verifying --note TEXT\n  submit PACKET --task TASK --run ID --actor ID --worktree PATH\n  review|qa|verify PACKET --task TASK --actor ID --verdict GREEN|BOUNCE --evidence TEXT --worktree PATH\n  accept PACKET --task TASK --actor ID --evidence TEXT --worktree PATH\n  recover PACKET --task TASK --reason TEXT\n  aggregate --task TASK --actor ID --evidence TEXT --worktree PATH\n  status [--task TASK] | audit [--task TASK]\n\nAll source-changing and verification actions use a registered, clean Git worktree; reports never auto-accept.`;

export function main(argv=process.argv.slice(2)){const [command,...rest]=argv,print=v=>process.stdout.write(`${JSON.stringify(v,null,2)}\n`);
  if(!command||['help','--help','-h'].includes(command))return process.stdout.write(`${usage}\n`);
  let a;
  const taskFlags=['task'];
  if(command==='init'){a=flags(rest,[]);
    if(a._.length!==1)fail('init requires exactly one plan path');
    return print(init(a._[0]))}if(command==='register-worktree'){a=flags(rest,['worktree'],['worktree']);
    if(a._.length)fail('register-worktree takes no positional arguments');
    return print(registerWorktree(a.worktree))}if(command==='review-plan'){a=flags(rest,[...taskFlags,'digest','actor','verdict','evidence'],['task','digest','actor','verdict','evidence']);
    return print(reviewPlan(a.task,{digest:a.digest,actor:a.actor,verdict:a.verdict,evidence:a.evidence}))}if(command==='accept-hub'){a=flags(rest,[...taskFlags,'actor','evidence'],['task','actor','evidence']);
    return print(acceptHub(a.task,{actor:a.actor,evidence:a.evidence}))}if(command==='claim'){a=flags(rest,[...taskFlags,'run','actor','worktree'],['task','run','actor','worktree']);
    if(a._.length!==1)fail('claim requires one packet kind');
    return print(claim(a.task,a._[0],{run_id:a.run,actor:a.actor,worktree:a.worktree}))}if(command==='checkpoint'){a=flags(rest,[...taskFlags,'run','status','note'],['task','run','status','note']);
    if(a._.length!==1)fail('checkpoint requires packet');
    return print(checkpoint(a.task,a._[0],{run_id:a.run,status:a.status,note:a.note}))}if(command==='submit'){a=flags(rest,[...taskFlags,'run','actor','worktree'],['task','run','actor','worktree']);
    if(a._.length!==1)fail('submit requires packet');
    return print(submit(a.task,a._[0],{run_id:a.run,actor:a.actor,worktree:a.worktree}))}if(['review','qa','verify'].includes(command)){a=flags(rest,[...taskFlags,'actor','verdict','evidence','worktree'],['task','actor','verdict','evidence','worktree']);
    if(a._.length!==1)fail(`${command} requires packet`);
    return print(report(a.task,a._[0],command,{actor:a.actor,verdict:a.verdict,evidence:a.evidence,worktree:a.worktree}))}if(command==='accept'){a=flags(rest,[...taskFlags,'actor','evidence','worktree'],['task','actor','evidence','worktree']);
    if(a._.length!==1)fail('accept requires packet');
    return print(accept(a.task,a._[0],{actor:a.actor,evidence:a.evidence,worktree:a.worktree}))}if(command==='recover'){a=flags(rest,[...taskFlags,'reason'],['task','reason']);
    if(a._.length!==1)fail('recover requires packet');
    return print(recover(a.task,a._[0],{reason:a.reason}))}if(command==='aggregate'){a=flags(rest,[...taskFlags,'actor','evidence','worktree'],['task','actor','evidence','worktree']);
    return print(aggregate(a.task,{actor:a.actor,evidence:a.evidence,worktree:a.worktree}))}if(command==='status'||command==='audit'){a=flags(rest,['task']);
    if(a._.length)fail(`${command} takes no positional arguments`);
    return print((command==='status'?status:audit)(a.task))}fail(`unknown command ${command}`)}
if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)){try{main()}catch(e){process.stderr.write(`agent-flow: ${e.message}\n`);
    process.exitCode=1}}
