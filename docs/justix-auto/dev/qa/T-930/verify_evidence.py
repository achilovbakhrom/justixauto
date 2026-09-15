import hashlib,json,pathlib,re,subprocess
root=pathlib.Path(__file__).resolve().parents[5]
expected='2ca96dfd636fdc349777f72f1a23e5cd9f82bb98'
assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip()==expected
source={}
for name in ['pkg/persistence/profile.go','pkg/persistence/profile_test.go','pkg/persistence/check.go','pkg/persistence/check_test.go','pkg/eventstore/owner_history.sql','pkg/eventstore/schema.sql','services/identity/migrations/0001_mechanics.up.sql']:
 b=(root/name).read_bytes(); assert b==subprocess.check_output(['git','show',expected+':'+name],cwd=root)
 source[name]=hashlib.sha256(b).hexdigest()
snapshots={}
for d in ['/private/var/folders/yq/j2ffv8bj2f35mcvbk7v7rk3m0000gn/T/justixauto-t930-evidence-120050282','/var/folders/yq/j2ffv8bj2f35mcvbk7v7rk3m0000gn/T/justixauto-t930-evidence-2265368711','/var/folders/yq/j2ffv8bj2f35mcvbk7v7rk3m0000gn/T/justixauto-t930-evidence-1623284051']:
 directory=pathlib.Path(d); hashes={}
 for side in directory.glob('*.sha256'):
  target=side.with_suffix(''); digest=hashlib.sha256(target.read_bytes()).hexdigest(); assert side.read_text().strip()==digest
  hashes[target.name]=digest
 if (directory/'initial.json').exists():
  a=json.loads((directory/'initial.json').read_text()); b=json.loads((directory/'after-readonly.json').read_text()); assert a==b
  final=json.loads((directory/'final-with-shared.json').read_text())
  for key in ['compatibility','rows','events','receipts']: assert a[key]==final[key],key
  assert final['history'][:len(a['history'])]==a['history']
  assert final['features'][:len(a['features'])]==a['features']
 snapshots[str(directory)]={'verified_hashes':hashes,'retained_preimages_preserved':True}
qa=pathlib.Path(__file__).resolve().parent
ids=set()
for filename in ['full-race.log','independent-probes-attempt1.log','independent-probes-attempt2.log']:
 text=(qa/filename).read_text()
 ids.update(re.findall(r'validated fixture removed and absent: ([a-f0-9]{64}) (justixauto-t930-[a-f0-9-]+)',text))
absent=[]
for pair in sorted(ids):
 for identifier in pair:
  p=subprocess.run(['docker','inspect','--format','{{.Id}}',identifier],capture_output=True,text=True)
  assert p.returncode!=0 and 'no such object' in (p.stdout+p.stderr).lower(),identifier
 absent.append({'id':pair[0],'name':pair[1],'independently_absent':True})
print(json.dumps({'reviewed_sha':expected,'source_hashes':source,'snapshots':snapshots,'fixture_absence':absent},indent=2))
