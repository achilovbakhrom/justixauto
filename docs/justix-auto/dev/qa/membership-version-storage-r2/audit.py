import functools,hashlib,json,pathlib,re,subprocess
ROOT=pathlib.Path(__file__).resolve().parents[5]
SHA='c3c0da45965c980fee22a32f43e90d7a674a55c4'
OLD='5996c77e3df371a3992154b639ce9ba91ffc3e75'
DRAFT='docs/justix-auto/state/drafts/architect/membership-version-storage.md'
RESULT='docs/justix-auto/dev/results/membership-version-storage.md'
def git(*args):return subprocess.check_output(['git','-C',str(ROOT),*args])
def file(sha,path):return git('show',sha+':'+path)
def digest(b):return hashlib.sha256(b).hexdigest()
assert git('rev-parse','HEAD').decode().strip()==SHA
changed=git('diff','--name-only',OLD,SHA).decode().splitlines()
assert sorted(changed)==sorted([DRAFT,RESULT])
for p in changed:assert (ROOT/p).read_bytes()==file(SHA,p)
draft=file(SHA,DRAFT).decode(); result=file(SHA,RESULT).decode()
old_result=file(OLD,RESULT).decode()
notice='Current submission: **fix cycle1; independent r2 QA required**. The original\nproposal at `5996c77e3df371a3992154b639ce9ba91ffc3e75` received BOUNCE.\nThe original result/evidence below is preserved; the appended fix-cycle section\nrecords the revised lifecycle contract and new reduced PostgreSQL verification.\n\n'
assert result.count(notice)==1 and result.replace(notice,'',1).startswith(old_result)
blocks=re.findall(r'^```(?:go|python|text|json)\n(.*?)^```',result,re.M|re.S)
old_blocks=re.findall(r'^```(?:go|python|text|json)\n(.*?)^```',old_result,re.M|re.S)
assert len(blocks)==7 and len(old_blocks)==5 and blocks[:5]==old_blocks
assert digest(blocks[5].encode())=='222d07307c97614f03a95dc9cebc8a2fac8a9ded3ddea6d767e72015f39ee634'
assert digest(blocks[6].encode())=='98b1816f823dd4fd0836bb942a24cdd23d46fb853805b07bccb63af2d53c3e07'
aliases={
 'MS-CATALOG':(['T-928'],['pkg/eventstore/migrations/000007_membership_catalog.up.sql','pkg/eventstore/membership_catalog_migration_test.go']),
 'MS-SELECTION':(['MS-CATALOG'],['pkg/eventstore/migrations/000008_membership_selection.up.sql','pkg/eventstore/membership_selection_migration_test.go']),
 'MS-IDENTITY':(['T-746','T-007'],['pkg/inbox/membership_catalog.go','pkg/inbox/membership_catalog_test.go']),
 'MS-STATE':(['MS-SELECTION','MS-IDENTITY','T-014'],['pkg/inbox/membership_state.go','pkg/inbox/membership_state_test.go'])}
leaves=[p for deps,paths in aliases.values() for p in paths]
assert len(leaves)==len(set(leaves))==8 and 'at most4h' in draft
for alias,(deps,paths) in aliases.items():
 row=next(x for x in draft.splitlines() if x.startswith('| '+alias+' |'))
 assert all(x in row for x in deps+paths)
edges={'T-920':['MS-STATE'],'T-921':['MS-STATE'],'T-016':['MS-STATE'],'T-022':['MS-SELECTION','MS-STATE'],'T-934':['MS-SELECTION']}
def graph(sha):
 tasks={t['id']:t for t in json.loads(file(sha,'docs/justix-auto/dev/task-index.json'))['tasks']}
 for t in tasks.values():assert not set(leaves).intersection(t['files_owned'])
 g={k:list(t['depends_on']) for k,t in tasks.items()}
 for k,(deps,_) in aliases.items():g[k]=list(deps)
 for k,deps in edges.items():g[k]+=deps
 done=set()
 def walk(k,active):
  assert k in g and k not in active
  if k in done:return
  for dep in g[k]:walk(dep,active|{k})
  done.add(k)
 for k in g:walk(k,set())
 @functools.lru_cache(None)
 def closure(k):
  v=set()
  for dep in g[k]:v.add(dep);v.update(closure(dep))
  return v
 for k in ['T-591']+['T-'+str(n) for n in range(580,587)]:assert set(aliases)<=closure(k)
 assert 'T-930' in closure('T-934')
 assert set(tasks['T-928']['files_owned'])=={'pkg/eventstore/migrations/000006_projection_generation.up.sql','pkg/eventstore/projection_generation_migration_test.go'}
 return {'sha':sha,'existing':len(tasks),'prospective':len(g),'acyclic':True,'aliases':4,'leaves':8,'downstream_edges':6,'all_roots_and_T591_covered':True}
main=git('rev-parse','refs/heads/main').decode().strip()
imported=git('rev-parse','b18bfd1').decode().strip()
qa_paths=git('ls-tree','-r','--name-only',imported,'docs/justix-auto/dev/qa/membership-version-storage.md','docs/justix-auto/dev/qa/membership-version-storage').decode().splitlines()
assert len(qa_paths)==13
original_artifacts={}
for p in qa_paths:
 b=file(imported,p);assert file(main,p)==b
 original_artifacts[p]=digest(b)
print(json.dumps({'reviewed_sha':SHA,'prior_reviewed_sha':OLD,'changed':changed,'hashes':{p:digest(file(SHA,p)) for p in changed},'old_result_preserved':True,'seven_embedded_blocks_verified':True,'graphs':[graph(SHA),graph(main)],'original_qa_import':imported,'original_13_qa_artifacts_unchanged_at_main':original_artifacts},indent=2))
