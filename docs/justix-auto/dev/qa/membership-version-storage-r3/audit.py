import ast,functools,hashlib,json,pathlib,re,subprocess
ROOT=pathlib.Path(__file__).resolve().parents[5]
SHA='bb908b4425003cbf41375b46b07fb88d2a3be817'
OLD='c3c0da45965c980fee22a32f43e90d7a674a55c4'
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
notice='Latest submission: **final fix cycle2, 2026-09-16; independent r3 QA required**.\nThis supersedes the fix-cycle1 submission status below. Both BOUNCE rounds and\nall earlier result/evidence content remain preserved. A further BOUNCE requires\nbounded scope review rather than a third automatic fix cycle.\n\n'
assert result.count(notice)==1 and result.replace(notice,'',1).startswith(old_result)
blocks=re.findall(r'^```(?:go|python|text|json)\n(.*?)^```',result,re.M|re.S)
old_blocks=re.findall(r'^```(?:go|python|text|json)\n(.*?)^```',old_result,re.M|re.S)
assert len(blocks)==9 and len(old_blocks)==7 and blocks[:7]==old_blocks
assert digest(blocks[7].encode())=='fafd2ff96ac1b50a053adc411d6f4fa43a8178daf04610ef024509c33825205a'
assert digest(blocks[8].encode())=='83341b44093fbf7d0bb5d2880ce325a4136d38795444071678c81e7ff49237e2'
oldest=file('5996c77e3df371a3992154b639ce9ba91ffc3e75',RESULT).decode()
expected_first_five=re.findall(r'\| (?:Temporary Go probe|Final Python SQL/fixture probe|Attempt1 log|Attempt2 log|Final log) \| `([0-9a-f]{64})`',oldest)
assert len(expected_first_five)==5
assert [digest(b.encode()) for b in blocks[:5]]==expected_first_five
assert digest(blocks[5].encode())=='222d07307c97614f03a95dc9cebc8a2fac8a9ded3ddea6d767e72015f39ee634'
assert digest(blocks[6].encode())=='98b1816f823dd4fd0836bb942a24cdd23d46fb853805b07bccb63af2d53c3e07'
reconstructions=[]
omit={'concurrent head cannot change after current-link constraint flush','current link committed while concurrent head attempt rolled back','head changes normally after current-link transaction ends'}
for name,fixture,cid,want in [
 ('first60','justix-membership-fix2-895878db-b8b4-489d-98ac-a497613c8a0b','28c0dba729e65b23af608f55c525bc9275337d7ff229edc3ab3c393a58b246bc','e8e2115837ea863511f2b8dfae9ca9fd8faa89e9ba25a108bc862f3b54f84f93'),
 ('retry60','justix-membership-fix2-36ea4022-dd47-43da-9c2c-17f2b46295df','ddbaa1570b5c3c238e9ae0f4717f61ef8fac5beb8456bc905be642f3da83f764','c55d60a8d37f2e24edbd8e6284261c03fb102ecc3af747875825b53b6c0e7788')]:
 data=json.loads(blocks[8]);data['observations']=[o for o in data['observations'] if o['case'] not in omit];data['count']=60;data['fixture']=fixture;data['observations'][-1]['id']=cid
 assert len(data['observations'])==60 and digest((json.dumps(data,indent=2)+'\n').encode())==want
 reconstructions.append({'output':name,'sha256':want,'reconstructed':True,'not_reexecuted':True})
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
preserved={}
for imported,name,count in [('b18bfd1f1df07c77e8afdfe0c1bae7909268aab5','membership-version-storage',13),('97246d5c0b803786b78b7a32ce650137e8aa97dc','membership-version-storage-r2',12)]:
 prefix='docs/justix-auto/dev/qa/'+name
 paths=git('ls-tree','-r','--name-only',imported,prefix+'.md',prefix).decode().splitlines()
 assert len(paths)==count
 for p in paths:
  b=file(imported,p);assert file(main,p)==b;preserved[p]=digest(b)
safe=pathlib.Path(__file__).with_name('safe_probe.py').read_text()
for token in ['ALTER ROLE','ALTER DATABASE','ALTER SYSTEM ','GRANT SET ON PARAMETER','REVOKE SET ON PARAMETER','SET session_replication_role','pg_reload_conf','parameter_holder']:assert token not in safe
assert not any(isinstance(n,ast.Call) and isinstance(n.func,ast.Name) and n.func.id in ('exec','eval') for n in ast.walk(ast.parse(safe)))
print(json.dumps({'reviewed_sha':SHA,'prior_reviewed_sha':OLD,'changed':changed,'hashes':{p:digest(file(SHA,p)) for p in changed},'old_result_preserved':True,'nine_embedded_blocks_verified':[digest(b.encode()) for b in blocks],'reconstructed_outputs':reconstructions,'graphs':[graph(SHA),graph(main)],'original_25_qa_artifacts_unchanged':preserved,'safe_probe_has_no_security_setting_mutations_or_dynamic_execution':True,'safe_probe_sha256':digest(safe.encode())},indent=2))
