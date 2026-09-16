import collections,copy,functools,gzip,hashlib,json,pathlib,posixpath,re,subprocess,urllib.parse

ROOT=pathlib.Path(__file__).resolve().parents[5]
SHA='b50490b48c5f31a542b0305827c1e345c2214cf6'
PARENT='9df6d1fb7aaef10368d8db8177351dc000e404eb'
PROPOSAL='bb908b4425003cbf41375b46b07fb88d2a3be817'
HASH='c468216706f0b40e01dc5c5e2bf07060cac60cecd429eac54ec5616bc9886330'
PREFIX='docs/justix-auto/'
BACKUP=PREFIX+'state/backups/2026-09-16-membership-promotion/'
CONTRACT=PREFIX+'state/drafts/contracts/membership-version-storage.md'
APPROVAL=PREFIX+'state/approvals/membership-version-storage.md'
INDEX=PREFIX+'dev/task-index.json'
checks=0
def check(value,message):
 global checks
 checks+=1
 if not value:raise AssertionError(message)
def git(*args):return subprocess.check_output(['git','-C',str(ROOT),*args])
def file(sha,path):return git('show',sha+':'+path)
def sha(b):return hashlib.sha256(b).hexdigest()
check(git('rev-parse','HEAD').decode().strip()==SHA,'exact reviewed HEAD')
check(git('rev-parse','HEAD^').decode().strip()==PARENT,'exact promotion parent')
names=git('diff','--name-status',PARENT,SHA).decode().splitlines()
changed={line.split('\t')[1]:line.split('\t')[0] for line in names}
check(len(changed)==31,'31 promotion paths')
check(all(p.startswith(PREFIX) for p in changed),'documentation-only change')
check(all(status in ('M','A') for status in changed.values()),'no deletes/renames')
contract=file(SHA,CONTRACT)
check(contract==file(PROPOSAL,PREFIX+'state/drafts/architect/membership-version-storage.md'),'canonical exact approved proposal bytes')
check(sha(contract)==HASH,'canonical approved SHA256')
approval=file(SHA,APPROVAL).decode()
check(HASH in approval and PROPOSAL in approval,'approval pins exact bytes/SHA')
for phrase in ['reduced observations','not full application','does not authorize','T-928 remains verification-blocked','No configuration mutation/activation','Reassess each4h','remaining3h','canonical promotion QA','T-920 remains blocked']:
 check(phrase.lower() in approval.lower(),'approval retains '+phrase)

before=json.loads(file(PARENT,INDEX)); after=json.loads(file(SHA,INDEX))
old={t['id']:t for t in before['tasks']};tasks={t['id']:t for t in after['tasks']}
check(len(old)==len(before['tasks'])==944,'944 unique predecessor records')
check(len(tasks)==len(after['tasks'])==948,'948 unique promoted records')
newids=['T-945','T-946','T-947','T-948']
check(set(tasks)-set(old)==set(newids),'exact four added task IDs')
aliases={'MS-CATALOG':'T-945','MS-SELECTION':'T-946','MS-IDENTITY':'T-947','MS-STATE':'T-948'}
edges={'T-920':['T-948'],'T-921':['T-948'],'T-016':['T-948'],'T-022':['T-946','T-948'],'T-934':['T-946']}
for id,t in old.items():
 expected=copy.deepcopy(t);expected['depends_on']+=edges.get(id,[])
 check(tasks[id]==expected,'all old fields preserved except approved additions '+id)
check(sum(map(len,edges.values()))==6,'six dependency additions')
check(after['metadata']['task_count']==948 and after['metadata']['effort_hours']==3154,'metadata counts')
check(sum(t['effort_hours'] for t in tasks.values())==3154,'recomputed effort hours')
check(sum(t['status']=='integrated' for t in tasks.values())==44,'44 integrated task records')
allowed_meta={'date','task_count','effort_hours','graph_metrics_scope','membership_version_storage'}
meta_changes={k for k in set(before['metadata'])|set(after['metadata']) if before['metadata'].get(k)!=after['metadata'].get(k)}
check(meta_changes==allowed_meta,'only approved metadata changes')
check('916-task baseline' in after['metadata']['graph_metrics_scope'] and 'do not claim recomputed width' in after['metadata']['graph_metrics_scope'],'no fabricated width/schedule')
amend=after['metadata']['membership_version_storage']
check(amend['aliases']==aliases and amend['added_existing_edges']==edges,'metadata alias/edge mapping')
check(amend['proposal_sha']==PROPOSAL and amend['proposal_sha256']==HASH and amend['canonical_promotion_qa']=='pending','metadata exact gate')

expected_new={
 'T-945':(['T-928'],['pkg/eventstore/migrations/000007_membership_catalog.up.sql','pkg/eventstore/membership_catalog_migration_test.go']),
 'T-946':(['T-945'],['pkg/eventstore/migrations/000008_membership_selection.up.sql','pkg/eventstore/membership_selection_migration_test.go']),
 'T-947':(['T-746','T-007'],['pkg/inbox/membership_catalog.go','pkg/inbox/membership_catalog_test.go']),
 'T-948':(['T-946','T-947','T-014'],['pkg/inbox/membership_state.go','pkg/inbox/membership_state_test.go'])}
allnew=[]
board=file(SHA,PREFIX+'dev/task-board.md').decode()
rows={}
for line in board.splitlines():
 if re.match(r'^\| \[T-\d{3}\]',line):
  cells=[x.strip() for x in line.split('|')[1:-1]]
  id=re.search(r'T-\d{3}',cells[0]).group()
  check(id not in rows,'one board row '+id);rows[id]=cells
check(set(rows)==set(tasks),'board contains exactly all 948 tasks')
for id,t in tasks.items():
 row=rows[id]
 check(row[1]==t['parent'] and row[2]==t['title'],'board title/parent '+id)
 check(row[3]==f"{t['kind']} / {t['effort_hours']}" and row[4]==t['lane'] and row[5]==t['status'],'board effort/kind/lane/status '+id)
 deps=[] if row[6] in ('—','-','None','none','') else [x.strip() for x in row[6].split(',')]
 check(deps==t['depends_on'] and row[7].strip('`')==t['branch'],'board dependencies/branch '+id)
for id,(deps,paths) in expected_new.items():
 t=tasks[id];doc=file(SHA,PREFIX+'dev/tasks/'+id+'.md').decode()
 check(t['depends_on']==deps and t['files_owned']==paths,'new exact dependencies/ownership '+id)
 check(t['effort_hours']==4 and t['status']=='todo' and t['worktree'] is None and t['base_sha'] is None,'new task not assigned/integrated '+id)
 check(t['parent']=='B-01' and t['coverage']==['B-01.AC2'],'new task coverage '+id)
 alias=next(k for k,v in aliases.items() if v==id)
 for phrase in ['- Proposal alias: '+alias,'- Depends on: '+', '.join(deps),'- Status: todo','4 hours maximum','canonical promotion QA before assignment','Pure identity work is unaffected','never bypass that gate','inert retained storage is not adoption/readiness']:
  check(phrase.lower() in doc.lower(),'task handoff '+id+' '+phrase)
 owned=re.search(r'## Exact file ownership\n(.*?)(?=\n## )',doc,re.S).group(1)
 check(re.findall(r'^- `([^`]+)`$',owned,re.M)==paths,'task doc owned leaves '+id)
 check(t['scope'] in doc,'full scope mirrored '+id)
 allnew+=paths
check(len(set(allnew))==len(allnew)==8,'eight disjoint new leaves')
for id,t in old.items():check(not set(allnew).intersection(t['files_owned']),'no collision with old task '+id)

graph={id:t['depends_on'] for id,t in tasks.items()};done=set()
def visit(id,active):
 check(id in graph and id not in active,'DAG/missing dependency '+id)
 if id in done:return
 for dep in graph[id]:visit(dep,active|{id})
 done.add(id)
for id in graph:visit(id,set())
@functools.lru_cache(None)
def closure(id):
 out=set()
 for dep in graph[id]:out.add(dep);out.update(closure(dep))
 return out
closure_evidence={}
for id in ['T-591']+['T-'+str(n) for n in range(580,587)]:
 check(set(newids)<=closure(id),'combined acceptance/owner closure '+id)
 closure_evidence[id]=sorted(set(newids)&closure(id))
check('T-930' in closure('T-934'),'serial checker successor')
check(tasks['T-920']['status']=='blocked' and tasks['T-920']['effort_hours']==3,'T920 retained blocked/3h')
for id in ['T-920','T-921','T-016','T-022','T-934','T-591']:
 doc=file(SHA,PREFIX+'dev/tasks/'+id+'.md').decode()
 check('## Approved durable membership handoff' in doc,'owned downstream handoff '+id)
 check('../../state/drafts/contracts/membership-version-storage.md' in doc and '../../state/approvals/membership-version-storage.md' in doc,'exact linked handoff '+id)
 check('configuration-test authorization' in doc and 'actual runtime startup' in doc,'retained runtime gates '+id)
 if id in edges:check('- Depends on: '+', '.join(tasks[id]['depends_on']) in doc,'downstream doc dependencies '+id)
check('Reassess remaining 3h bound' in file(SHA,PREFIX+'dev/tasks/T-920.md').decode(),'T920 reassessment')

cov_before=file(PARENT,PREFIX+'dev/backlog-coverage.md').decode().splitlines()
cov_after=file(SHA,PREFIX+'dev/backlog-coverage.md').decode().splitlines()
check(len(cov_before)==len(cov_after),'coverage rows preserved')
changed_rows=[(a,b) for a,b in zip(cov_before,cov_after) if a!=b]
check(len(changed_rows)==1,'one coverage row changed')
a,b=changed_rows[0]
check(a.startswith('| B-01.AC2 |') and b==a.replace(' | T-591 |',', '+', '.join(newids)+' | T-591 |'),'exact B01AC2 contribution addition only')
state=file(SHA,PREFIX+'dev/dev-state.md').decode()
for text in ['44/948','Formal PM task files: 948','canonical promotion QA pending before assignment','T-928 live verification authorization remains pending']:
 check(text in state,'state consistency '+text)
check('metrics below describe the original reviewed 916-task baseline; no new' in board,'board historical graph qualification')

manifest=json.loads(file(SHA,BACKUP+'manifest.json'))
check(len(manifest)==12 and len({v['path'] for v in manifest})==12,'12 unique preimages')
check({v['path'] for v in manifest}=={p for p,status in changed.items() if status=='M'},'every changed existing canonical doc covered')
preimages=[]
for item in manifest:
 raw=gzip.decompress(file(SHA,BACKUP+item['snapshot']));prior=file(PARENT,item['path'])
 check(raw==prior and sha(raw)==item['sha256'] and len(raw)==item['bytes'],'recoverable exact parent snapshot '+item['path'])
 preimages.append({'path':item['path'],'bytes':len(raw),'sha256':sha(raw)})

tracked=set(git('ls-tree','-r','--name-only',SHA).decode().splitlines())
links=[]
for path in sorted(p for p in changed if p.endswith('.md')):
 text=file(SHA,path).decode();text=re.sub(r'```.*?```','',text,flags=re.S)
 for target in re.findall(r'(?<!!)\[[^\]]+\]\(([^)]+)\)',text):
  target=target.strip().split(' "',1)[0].strip('<>')
  if re.match(r'^[a-zA-Z][a-zA-Z0-9+.-]*:',target) or target.startswith('#'):continue
  raw=urllib.parse.unquote(target.split('#',1)[0].split('?',1)[0])
  resolved=posixpath.normpath(posixpath.join(posixpath.dirname(path),raw))
  check(resolved in tracked or any(x.startswith(resolved.rstrip('/')+'/') for x in tracked),'local link '+path+' -> '+target)
  links.append({'from':path,'target':target,'resolved':resolved})

preserved={}
for name,count in [('membership-version-storage',13),('membership-version-storage-r2',12),('membership-version-storage-r3',15)]:
 prefix=PREFIX+'dev/qa/'+name
 paths=git('ls-tree','-r','--name-only',PARENT,prefix+'.md',prefix).decode().splitlines()
 check(len(paths)==count,'earlier QA artifact count '+name)
 for p in paths:
  raw=file(PARENT,p);check(file(SHA,p)==raw,'old QA artifact preserved '+p);preserved[p]=sha(raw)
for p in changed:check((ROOT/p).read_bytes()==file(SHA,p),'review checkout unchanged '+p)
print(json.dumps({'outcome':'GREEN','reviewed_sha':SHA,'parent_sha':PARENT,'proposal_sha':PROPOSAL,'canonical_sha256':HASH,'checks':checks,'promotion_paths':changed,'task_count':len(tasks),'effort_hours':sum(t['effort_hours'] for t in tasks.values()),'integrated':sum(t['status']=='integrated' for t in tasks.values()),'old_records_preserved_except_edges':len(old),'aliases':aliases,'new_leaves':allnew,'added_existing_edges':edges,'closure':closure_evidence,'metadata_changes':sorted(meta_changes),'preimages':preimages,'local_link_count':len(links),'links':links,'preserved_40_qa_artifacts':preserved,'runtime_tests_executed':False,'configuration_changes_authorized_by_audit':False},indent=2))
