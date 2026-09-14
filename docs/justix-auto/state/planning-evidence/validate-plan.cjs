// Read-only coordinator audit. Run: node validate-plan.cjs <draft dev dir> <canonical docs root>
const fs = require('node:fs');
const path = require('node:path');
const [devDir, docsRoot] = process.argv.slice(2);
if (!devDir || !docsRoot) throw new Error('Expected draft dev directory and canonical docs root');
const read = p => fs.readFileSync(p, 'utf8');
const data = JSON.parse(read(path.join(devDir, 'task-index.json')));
const tasks = data.tasks;
if (!Array.isArray(tasks) || !tasks.length) throw new Error('No task array');
const backlog = read(path.join(docsRoot, 'dev/backlog.md'));
const architecture = read(path.join(docsRoot, 'architecture.md'));
const board = read(path.join(devDir, 'task-board.md'));
const expected = new Map();
for (const chunk of backlog.split(/^### /m).slice(1, 50)) {
  const id = chunk.match(/^B-\d+/)[0];
  const ac = chunk.match(/^AC: (.+)$/m)?.[1];
  if (!ac) throw new Error('No parent acceptance: ' + id);
  expected.set(id, [...ac.matchAll(/\((\d+)\)/g)].map(m => `${id}.AC${m[1]}`));
}
const errors = [], warnings = [], ids = new Map(), branches = new Set(), covered = new Set();
const docs = new Map();
for (const t of tasks) {
  if (!/^T-\d{3}$/.test(t.id || '') || ids.has(t.id)) errors.push('Invalid/duplicate ID: ' + t.id);
  ids.set(t.id, t);
  if (!expected.has(t.parent)) errors.push(`${t.id}: invalid/inactive parent ${t.parent}`);
  if (!(t.effort_hours > 0 && t.effort_hours <= 4)) errors.push(`${t.id}: effort must be >0 and <=4`);
  if (t.status !== 'todo') errors.push(`${t.id}: not todo`);
  if (!t.branch?.startsWith(`task/${t.id}-`) || branches.has(t.branch)) errors.push(`${t.id}: invalid/duplicate branch`);
  branches.add(t.branch);
  for (const field of ['depends_on', 'files_owned', 'gates', 'design_refs', 'coverage'])
    if (!Array.isArray(t[field])) errors.push(`${t.id}: ${field} must be an array`);
  if (!t.files_owned?.length) errors.push(`${t.id}: no owned files`);
  if (!t.coverage?.length) errors.push(`${t.id}: no acceptance coverage`);
  for (const c of t.coverage || []) {
    if (!expected.get(t.parent)?.includes(c)) errors.push(`${t.id}: unknown parent acceptance ${c}`);
    covered.add(c);
  }
  const p = path.join(devDir, 'tasks', `${t.id}.md`);
  if (!fs.existsSync(p)) { errors.push(`${t.id}: missing task file`); continue; }
  const md = read(p); docs.set(t.id, md);
  if (!md.includes(t.id) || !md.includes(t.parent) || !md.includes(t.branch)) errors.push(`${t.id}: file metadata mismatch`);
  if (!/acceptance/i.test(md) || !/test|verification/i.test(md)) errors.push(`${t.id}: missing acceptance/test expectations`);
  if (!md.includes(`results/${t.id}.md`) || !md.includes(`qa/${t.id}.md`)) errors.push(`${t.id}: missing result/QA paths`);
  if (!board.includes(t.id) || !board.includes(t.branch)) errors.push(`${t.id}: missing board row/branch`);
  for (const d of t.depends_on || []) if (!md.includes(d)) errors.push(`${t.id}: dependency ${d} omitted from task`);
  for (const ref of t.design_refs || []) {
    if (!architecture.includes(ref)) errors.push(`${t.id}: unknown design anchor ${ref}`);
    if (!md.includes(ref)) errors.push(`${t.id}: design anchor not in task ${ref}`);
  }
  const visual = /frontend|^fe$|^ui$/i.test(t.kind || '') || (t.files_owned || []).some(f => /^web\//.test(f) && /(?<!\.test)\.tsx$|\.css$/.test(f));
  if (visual && !t.design_refs?.length) errors.push(`${t.id}: FE lacks design refs`);
  for (const f of t.files_owned || []) {
    if (f.startsWith('/') || f.includes('..') || !/^(services|edge|pkg|web|infra|tests|tools|docs)\/|^(go\.(mod|sum)|package(-lock)?\.json|Makefile|\.gitignore)$/.test(f))
      errors.push(`${t.id}: invalid/out-of-layout ownership ${f}`);
    if (!md.includes(f)) errors.push(`${t.id}: owned file omitted from task ${f}`);
  }
}
for (const [p, acs] of expected) for (const ac of acs) if (!covered.has(ac)) errors.push('Uncovered: ' + ac);
for (const t of tasks) for (const d of t.depends_on || []) if (!ids.has(d) || d === t.id) errors.push(`${t.id}: invalid dependency ${d}`);
if (errors.some(e => /Invalid\/duplicate ID|must be an array|invalid dependency/.test(e))) { console.log(JSON.stringify({errors}, null, 2)); process.exit(1); }
const active = new Set(), ancestors = new Map(), order = [];
function visit(id) {
  if (active.has(id)) throw new Error('Dependency cycle: ' + [...active, id].join(' -> '));
  if (ancestors.has(id)) return ancestors.get(id);
  active.add(id); const all = new Set();
  for (const d of ids.get(id).depends_on) { all.add(d); for (const a of visit(d)) all.add(a); }
  active.delete(id); ancestors.set(id, all); order.push(id); return all;
}
tasks.forEach(t => visit(t.id));
const pattern = value => new RegExp('^' + value.split('**').map(s => s.split('*').map(p => p.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('[^/]*')).join('.*') + '$');
const overlap = (a,b) => a === b || pattern(a).test(b) || pattern(b).test(a) || (a.includes('*') && b.includes('*') && a.split('*')[0] === b.split('*')[0]);
for (let i=0;i<tasks.length;i++) for(let j=i+1;j<tasks.length;j++) {
  const a=tasks[i],b=tasks[j]; if(ancestors.get(a.id).has(b.id)||ancestors.get(b.id).has(a.id))continue;
  for(const x of a.files_owned)for(const y of b.files_owned)if(overlap(x,y))errors.push(`Unordered ownership: ${a.id}/${b.id}: ${x} / ${y}`);
}
const level=new Map(),duration=new Map(),previous=new Map();
for(const id of order){const t=ids.get(id);level.set(id,t.depends_on.length?Math.max(...t.depends_on.map(d=>level.get(d)))+1:0);let best=null;for(const d of t.depends_on)if(best===null||duration.get(d)>duration.get(best))best=d;previous.set(id,best);duration.set(id,t.effort_hours+(best?duration.get(best):0));}
const buckets={};for(const [id,l] of level)(buckets[l]??=[]).push(id);
const end=order.reduce((a,b)=>duration.get(a)>duration.get(b)?a:b);const critical=[];for(let x=end;x;x=previous.get(x))critical.unshift(x);
const files=fs.readdirSync(path.join(devDir,'tasks')).filter(x=>/^T-\d+\.md$/.test(x));
if(files.length!==tasks.length)errors.push('Task file count differs from index');
const report={tasks:tasks.length,parents:expected.size,coveredAC:covered.size,effortHours:tasks.reduce((a,t)=>a+t.effort_hours,0),maxEffort:Math.max(...tasks.map(t=>t.effort_hours)),taskFiles:files.length,dependencyGraph:'acyclic',dependencyLevels:Object.keys(buckets).length,maxLevelWidth:Math.max(...Object.values(buckets).map(x=>x.length)),criticalPath:critical,criticalPathHours:duration.get(end),runtimeWorkers:3,metricsNote:'Levels/longest weighted chain ignore unresolved gates and worker contention; not an elapsed delivery promise or exact maximum antichain.',errors,warnings};
console.log(JSON.stringify(report,null,2));if(errors.length)process.exit(1);
