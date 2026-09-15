#!/usr/bin/env python3
"""Independent immutable Git-object audit of canonical auth promotion."""
import gzip,hashlib,itertools,json,pathlib,re,subprocess,urllib.parse
root=pathlib.Path(__file__).resolve().parents[5]
head='56e904a1a2748033be1f4d3565b0fc8eca8bdbe8'
parent='7dabbfd8c9e783652a48ecace39660eb66a7e009'
reviewed='914fcd78cc0789e41e9091efdc99fb5afc6c5c50'
proposal_hash='e1643c8cf36ba5733f7bfa0d32e2285604b345fe72ed506e17248fbae9af3e0c'
count=0
def require(ok,label):
 global count
 assert ok,label
 count+=1
def git(*args):return subprocess.check_output(['git',*args],cwd=root)
def obj(path,sha=head):return git('show',sha+':'+path)
def text(path,sha=head):return obj(path,sha).decode()
def digest(data):return hashlib.sha256(data).hexdigest()
require(git('rev-parse','HEAD').decode().strip()==head,'exact HEAD')
require(git('rev-parse',head+'^').decode().strip()==parent,'exact parent')
changed={line.split('\t',1)[1]:line.split('\t',1)[0] for line in git('diff','--name-status',parent,head).decode().splitlines()}
backup='docs/justix-auto/state/backups/2026-09-15-auth-transport-promotion/'
manifest=json.loads(obj(backup+'manifest.json'))
require(len(manifest)==13,'13 preimages')
require(len({r['path'] for r in manifest})==13,'distinct preimages')
preimages=[]
for row in manifest:
 original=obj(row['path'],parent)
 recovered=gzip.decompress(obj(backup+row['snapshot']))
 require(original==recovered,row['path']+' parent/snapshot bytes')
 require(row['bytes']==len(original),row['path']+' byte length')
 require(row['sha256']==digest(original),row['path']+' SHA256')
 require(changed.get(row['path'])=='M',row['path']+' retained canonical mutation')
 preimages.append({'path':row['path'],'bytes':len(original),'sha256':digest(original)})
require({p for p,s in changed.items() if s=='M'}=={r['path'] for r in manifest},'every existing mutation has preimage')
new_ids=['T-'+str(i) for i in range(935,945)]
expected_paths={r['path'] for r in manifest}|{backup+r['snapshot'] for r in manifest}|{backup+'manifest.json'}|{'docs/justix-auto/dev/tasks/'+t+'.md' for t in new_ids}|{'docs/justix-auto/state/approvals/auth-transport-compatibility.md','docs/justix-auto/state/drafts/contracts/auth-transport-compatibility.md','docs/justix-auto/dev/qa/auth-transport-canonical-check.md'}
require(set(changed)==expected_paths,'exact 40-document/snapshot scope, no hidden changes')
draft='docs/justix-auto/state/drafts/architect/auth-transport-compatibility.md'
canonical='docs/justix-auto/state/drafts/contracts/auth-transport-compatibility.md'
require(obj(draft,reviewed)==obj(draft)==obj(canonical),'exact reviewed/canonical/architect bytes')
require(digest(obj(canonical))==proposal_hash,'reviewed SHA256')

indexpath='docs/justix-auto/dev/task-index.json'
before=json.loads(obj(indexpath,parent));after=json.loads(obj(indexpath))
old={t['id']:t for t in before['tasks']};new={t['id']:t for t in after['tasks']}
require(len(old)==934 and len(new)==len(after['tasks'])==944,'934 preserved plus10 unique records')
require(set(new)-set(old)==set(new_ids),'exact new task IDs')
additions={'T-055':['T-944','T-943'],'T-060':['T-943'],'T-641':['T-942']}
for id,t in old.items():
 expected={**t,'depends_on':t['depends_on']+additions.get(id,[])}
 require(new[id]==expected,id+' full record preserved apart from declared dependencies')
require(sum(len(v) for v in additions.values())==4,'four added edges')
require(sum(t['effort_hours'] for t in new.values())==3138,'3138 total hours')
require(after['metadata']['task_count']==944 and after['metadata']['effort_hours']==3138,'metadata counts')
meta_changes={k for k in set(before['metadata'])|set(after['metadata']) if before['metadata'].get(k)!=after['metadata'].get(k)}
require(meta_changes=={'task_count','effort_hours','graph_metrics_scope','auth_transport_compatibility'},'bounded metadata change')
require('916-task baseline' in after['metadata']['graph_metrics_scope'],'baseline metrics remain explicitly historical')
meta=after['metadata']['auth_transport_compatibility']
require(meta['proposal_sha']==reviewed and meta['proposal_sha256']==proposal_hash,'metadata exact authority')
require(meta['added_existing_edges']=={'T-641':['T-942'],'T-055':['T-944','T-943'],'T-060':['T-943']},'metadata edge mapping')
rows=[list(map(str.strip,l.split('|')[1:-1])) for l in text(draft).splitlines() if l.startswith('| AT-')]
mapping={row[0]:id for row,id in zip(rows,new_ids)}
require(len(rows)==10 and mapping==meta['aliases'],'ten reviewed aliases in order')
proposal_leaves={};estimates={}
for alias,deps,scope in rows:
 id=mapping[alias];t=new[id]
 expected_deps=[mapping.get(v,v) for v in deps.split(', ')]
 leaves=re.findall(r'`([^`]+\.(?:ts|go|mjs|json))`',scope)
 estimate=int(re.match(r'(\d+)h;',scope)[1]);estimates[id]=estimate
 require(t['depends_on']==expected_deps,id+' proposal dependencies')
 require(t['files_owned']==leaves,id+' exact proposal leaves/order')
 require(t['effort_hours']==estimate and 0<estimate<=4,id+' effort bound')
 require(t['status']=='todo' and t['worktree'] is None and t['base_sha'] is None,id+' unassigned todo')
 require(t.get('reviewed_sha') is None and t.get('integration_sha') is None,id+' no unearned QA/integration')
 proposal_leaves[id]=leaves
require(sum(estimates.values())==36,'new36h')
require(len({p for paths in proposal_leaves.values() for p in paths})==13,'13 unique new-task leaves')

active=set();ancestors={}
def visit(id):
 require(id in new and id not in active,id+' graph entry/cycle')
 if id in ancestors:return ancestors[id]
 active.add(id);seen=set()
 for dep in new[id]['depends_on']:seen.add(dep);seen|=visit(dep)
 active.remove(id);ancestors[id]=seen;return seen
for id in new:visit(id)
owners={}
for id,t in new.items():
 for path in t['files_owned']:owners.setdefault(path,[]).append(id)
shared=[]
for path,ids in owners.items():
 for a,b in itertools.combinations(ids,2):
  if a not in new_ids and b not in new_ids:continue
  require(a in ancestors[b] or b in ancestors[a],path+' serial '+a+'/'+b)
  shared.append((path,*sorted([a,b])))
require(len(shared)==len(set(shared)),'unique shared-leaf/task-pair tuples')
chain=['T-'+str(i) for i in range(937,943)]
for path in ['tools/generate-contracts.mjs','tests/contracts/generation_reproducibility_test.go']:
 require([id for id in owners[path] if id in new_ids]==chain,path+' exact six-task chain')
for earlier,later in zip(chain,chain[1:]):require(new[later]['depends_on']==[earlier],later+' immediate serial predecessor')
require(meta['generator_serial_tasks']==chain,'metadata generator chain')

boardpath='docs/justix-auto/dev/task-board.md'
board={}
for line in text(boardpath).splitlines():
 match=re.match(r'^\| \[(T-\d+)\]',line)
 if match:
  require(match[1] not in board,match[1]+' board unique')
  board[match[1]]=[v.strip() for v in line.split('|')[1:-1]]
require(set(board)==set(new),'all944 board rows')
for id,t in new.items():
 row=board[id];taskpath='docs/justix-auto/dev/tasks/'+id+'.md';doc=text(taskpath)
 require(row[1]==t['parent'] and row[2]==t['title'],id+' board title/parent')
 require(row[3]==t['kind']+' / '+str(t['effort_hours']),id+' board effort/kind')
 require(row[4]==t['lane'] and row[5]==t['status'],id+' board lane/status')
 require(re.findall(r'T-\d+',row[6])==t['depends_on'],id+' board dependencies')
 require(row[7].strip('`')==t['branch'],id+' board branch')
 for label,key in [('Status','status'),('Parent','parent'),('Kind','kind'),('Lane','lane')]:
  require(re.search(r'^- '+label+r': (.*)$',doc,re.M)[1]==t[key],id+' task '+key)
 require(re.findall(r'T-\d+',re.search(r'^- Depends on: (.*)$',doc,re.M)[1])==t['depends_on'],id+' task dependencies')
 section=doc.split('## Exact file ownership',1)[1].split('\n## ',1)[0]
 files=re.findall(r'^- `([^`]+)`',section,re.M)
 require(set(files)==set(t['files_owned']) and len(files)==len(t['files_owned']),id+' task exact ownership')
 if id in new_ids:
  require('- Proposal alias: '+next(a for a,v in mapping.items() if v==id) in doc,id+' task alias')
  require('canonical promotion QA before assignment' in doc,id+' promotion gate')
  require('coordinator' in doc.lower() and 'before exact QA' in doc,id+' root boundary')

coveragepath='docs/justix-auto/dev/backlog-coverage.md'
def coverage(sha):
 out={}
 for line in text(coveragepath,sha).splitlines():
  if re.match(r'^\| B-\d+\.AC\d+',line):
   row=[s.strip() for s in line.split('|')[1:-1]];out[row[0]]=row
 return out
cold,cnew=coverage(parent),coverage(head)
expected_contributions={id:({'B-02.AC1'} if id in ['T-935','T-936'] else {'B-01.AC4'} if id in chain else {'B-02.AC1','B-02.AC2','B-02.AC3','B-02.AC4'} if id=='T-943' else {'B-04.AC2'}) for id in new_ids}
closers={'B-01.AC4':'T-591',**{'B-02.AC'+str(i):'T-592' for i in range(1,5)},'B-04.AC2':'T-594'}
require(set(cold)==set(cnew),'coverage AC catalog unchanged')
for ac,row in cold.items():
 add=[id for id in new_ids if ac in expected_contributions[id]]
 expected=[*row];expected[2]=row[2]+(''+', '+', '.join(add) if add else '')
 require(cnew[ac]==expected,ac+' exact contribution-only delta')
for id,acs in expected_contributions.items():
 require(set(new[id]['coverage'])==acs,id+' exact coverage field')
 doc=text('docs/justix-auto/dev/tasks/'+id+'.md')
 contribution=re.search(r'^- Coverage contribution: (.*)$',doc,re.M)[1]
 require(set(re.findall(r'B-\d+\.AC\d+',contribution))==acs,id+' task-file contribution')
 require(re.findall(r'T-\d+',contribution)==[closers[next(iter(acs))]],id+' task-file dedicated closer')
 require(new[id]['parent']==next(iter(acs)).split('.')[0],id+' contribution parent')
 for ac in acs:
  require(id in re.findall(r'T-\d+',cnew[ac][2]),id+' contributor '+ac)
  require(cnew[ac][3]==closers[ac],id+' dedicated closer '+ac)
  require(id in ancestors[closers[ac]],id+' transitive closure '+closers[ac])

# Check every real Markdown link in changed Markdown documents against this
# exact tree. Source-code/ancillary future leaves in backticks are not links.
tracked=set(git('ls-tree','-r','--name-only',head).decode().splitlines());links=[]
for p in changed:
 if not p.endswith('.md'):continue
 doc=text(p)
 for match in re.finditer(r'\[[^\]\n]*\]\(([^)\n]+)\)',doc):
  target=match[1].strip().strip('<>');target=target.split('#',1)[0]
  if not target or re.match(r'[A-Za-z][A-Za-z0-9+.-]*:',target):continue
  target=urllib.parse.unquote(target)
  resolved=(pathlib.Path(p).parent/target)
  normalized=str(pathlib.Path(__import__('os').path.normpath(resolved)))
  require(normalized in tracked or any(q.startswith(normalized.rstrip('/')+'/') for q in tracked),p+' link '+target)
  links.append({'from':p,'to':normalized})

originals=[]
main=pathlib.Path('/Users/bakhromachilov/startups/justixauto')
for suffix,expected_n in [('',5),('-r2',8)]:
 stem='auth-transport-compatibility'+suffix
 checkout=main/'.worktrees'/('auth-transport-compatibility'+suffix+'-qa')
 prefix='docs/justix-auto/dev/qa/'+stem
 files=[prefix+'.md']+[str(p.relative_to(checkout)) for p in (checkout/prefix).iterdir() if p.is_file()]
 require(len(files)==expected_n,stem+' artifact count')
 for p in files:
  require(obj(p)==obj(p,parent)==(checkout/p).read_bytes(),p+' immutable original QA bytes')
  originals.append({'path':p,'sha256':digest(obj(p))})
approval=text('docs/justix-auto/state/approvals/auth-transport-compatibility.md')
require('Canonical promotion QA must verify' in approval,'approval gate retained')
require('T-002 remains unaccepted' in approval,'no security acceptance')
require('including legacy non-auth/internal' in approval,'terminal non-auth/internal resource scope')
require('Body validation cannot bound bytes already read' in approval,'IO allocation distinction')
require('serialize API manifest edits' in approval,'root manifest serialization')
for id in ['T-059','T-641','T-055','T-060','T-592']:
 doc=text('docs/justix-auto/dev/tasks/'+id+'.md')
 require('## Approved auth transport handoff' in doc and 'T-002 is still unaccepted' in doc,id+' canonical handoff')
 require('P4 adopts' in doc and 'P6 requires' in doc and 'P5 preserves' in doc,id+' exact sections')
print(json.dumps({'result':'PASS','commit':head,'parent':parent,'assertions':count,'preimages':preimages,'preserved_existing_task_records':len(old),'task_count':len(new),'total_effort_hours':3138,'new_effort_hours':sum(estimates.values()),'aliases':mapping,'serial_generator_chain':chain,'unique_shared_leaf_owner_pairs':len(shared),'shared_pairs':shared,'modified_markdown_links_checked':len(links),'preserved_qa_artifacts':originals,'limits':'canonical promotion audit only; runtime models were not rerun and implementation/production gates remain'},indent=2))
