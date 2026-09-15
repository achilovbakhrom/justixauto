from pathlib import Path
import hashlib,json,re,subprocess

root=Path(__file__).resolve().parents[5]
sha='d87174069acd57c2ced3689ead129749ff86dec6'
base='d59faa8dc5b26e13e19c47686dd47744b6d73a11'
def git(*args): return subprocess.check_output(['git',*args],cwd=root)
assert git('rev-parse','HEAD').decode().strip()==sha
changed=git('diff','--name-only',base,sha).decode().splitlines()
assert sorted(changed)==sorted(['pkg/eventstore/migrations/000006_projection_generation.up.sql','pkg/eventstore/projection_generation_migration_test.go','docs/justix-auto/dev/results/T-928.md'])
prior=['pkg/eventstore/schema.sql',*['pkg/eventstore/migrations/'+name for name in ['000002_messaging_delivery.up.sql','000003_messaging_route_compatibility.up.sql','000004_projection_checkpoint.up.sql','000005_quarantine_evidence.up.sql']]]
hashes={}
for name in prior+changed:
 data=(root/name).read_bytes()
 assert data==git('show',sha+':'+name),name
 if name in prior: assert data==git('show',base+':'+name),name
 hashes[name]=hashlib.sha256(data).hexdigest()
logs={'author':'/private/tmp/justixauto-t928-final-race.log','qa_full':'/private/tmp/justixauto-t928-qa-full-race.log','qa_independent':'/private/tmp/justixauto-t928-qa-independent-attempt1.log'}
result={'exact_commit':sha,'base':base,'source_hashes':hashes,'runs':{}}
for label,path in logs.items():
 log=Path(path).read_text()
 directories=list(dict.fromkeys(re.findall(r'Recoverable synthetic migration evidence: (\S+)',log)))
 records=[]
 for directory in directories:
  for sidecar in sorted(Path(directory).glob('*.sha256')):
   target=Path(str(sidecar)[:-7]);digest=hashlib.sha256(target.read_bytes()).hexdigest()
   assert sidecar.read_text().strip()==digest,target
   records.append({'path':str(target),'sha256':digest})
 fixtures=re.findall(r'validated task fixture removed and verified absent: ([0-9a-f]{64}) (justixauto-t927-[0-9a-f-]{36})',log)
 assert len(fixtures)==(1 if label=='qa_independent' else 3),(label,fixtures)
 for identity,name in fixtures:
  for value in (identity,name):
   p=subprocess.run(['docker','inspect','--format','{{.Id}}',value],capture_output=True,text=True)
   assert p.returncode!=0 and 'no such object' in (p.stdout+p.stderr).lower(),value
 if label!='qa_independent':
  assert len(records)==42,(label,len(records))
  assert not re.search(r'^FAIL',log,re.M),label
  assert re.search(r'^ok\s+justixauto/pkg/eventstore\s+',log,re.M),label
 else:
  assert len(re.findall(r'^    --- FAIL:',log,re.M))==4
  assert len(re.findall(r'^    --- PASS:',log,re.M))==3
 result['runs'][label]={'log':path,'sha256':hashlib.sha256(Path(path).read_bytes()).hexdigest(),'preimage_count':len(records),'preimages':records,'fixtures_absent':fixtures,'package_results':re.findall(r'^ok\s+justixauto/[^\n]+',log,re.M)}
assert Path('/private/tmp/justixauto-t928-qa-vet.log').read_bytes()==b''
print(json.dumps(result,indent=2))
