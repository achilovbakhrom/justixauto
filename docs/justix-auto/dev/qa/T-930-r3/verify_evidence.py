import hashlib,json,pathlib,re,subprocess
qa=pathlib.Path(__file__).resolve().parent
root=qa.parents[4]
sha='9802e7a0800a8e54043f59d0583d0ff7eb074744'
prior='979972f766503dd31fcd53ca088275ae7679afda'
def git(*args):return subprocess.check_output(['git',*args],cwd=root)
assert git('rev-parse','HEAD').decode().strip()==sha
leaves=['pkg/persistence/profile.go','pkg/persistence/profile_test.go','pkg/persistence/check.go','pkg/persistence/check_test.go']
assert set(git('diff','--name-only',prior+'..'+sha).decode().splitlines())==set(['pkg/persistence/profile.go','pkg/persistence/profile_test.go','docs/justix-auto/dev/results/T-930.md'])
source={}
for name in leaves+['pkg/eventstore/owner_history.sql','pkg/eventstore/schema.sql','services/identity/migrations/0001_mechanics.up.sql']:
 b=(root/name).read_bytes();assert b==git('show',sha+':'+name)
 if name not in ['pkg/persistence/profile.go','pkg/persistence/profile_test.go']:assert b==git('show',prior+':'+name)
 source[name]=hashlib.sha256(b).hexdigest()
old=[root/'docs/justix-auto/dev/qa/T-930.md',*sorted((root/'docs/justix-auto/dev/qa/T-930').iterdir())]
old += [root/'docs/justix-auto/dev/qa/T-930-r2.md',*sorted((root/'docs/justix-auto/dev/qa/T-930-r2').iterdir())]
assert len(old)==21
original={}
for p in old:
 name=str(p.relative_to(root));b=p.read_bytes();assert b==git('show','b18bfd1:'+name)
 original[name]=hashlib.sha256(b).hexdigest()
snapshots={};ids=set();directories=set()
for name in ['full-race.log','original.log','round2.log']:
 text=(qa/name).read_text()
 directories.update(re.findall(r'retained synthetic evidence directory (\S+)',text))
 ids.update(re.findall(r'validated fixture removed and absent: ([a-f0-9]{64}) (justixauto-t930-[a-f0-9-]+)',text))
for d in sorted(directories):
 path=pathlib.Path(d);hashes={}
 for side in path.glob('*.sha256'):
  target=side.with_suffix('');h=hashlib.sha256(target.read_bytes()).hexdigest();assert side.read_text().strip()==h;hashes[target.name]=h
 if (path/'initial.json').exists():
  a=json.loads((path/'initial.json').read_text());b=json.loads((path/'after-readonly.json').read_text());assert a==b
  c=json.loads((path/'final-with-shared.json').read_text())
  for key in ['compatibility','rows','events','receipts']:assert a[key]==c[key],key
  for key in ['history','features']:assert c[key][:len(a[key])]==a[key],key
 if (path/'r2-initial.json').exists():assert (path/'r2-initial.json').read_bytes()==(path/'r2-final.json').read_bytes()
 snapshots[d]=hashes
absence=[]
for ident,name in sorted(ids):
 for key in [ident,name]:
  p=subprocess.run(['docker','inspect','--format','{{.Id}}',key],capture_output=True,text=True)
  assert p.returncode!=0 and 'no such object' in (p.stdout+p.stderr).lower()
 absence.append({'id':ident,'name':name,'absent':True})
assert len(absence)==5
print(json.dumps({'sha':sha,'changed_source_exact':source,'original_21_artifacts_unchanged':original,'verified_snapshots':snapshots,'fixture_absence':absence},indent=2))
