import difflib,hashlib,json,pathlib,re,subprocess
wt=pathlib.Path.cwd();main=wt.parent.parent
sha='1636aeb291007163f80e41855694a2390a7694f5';old='a4de45c811c8eaa7b8d3f7559df0db4c9454a3e3'
def git(*args,cwd=wt):return subprocess.check_output(['git',*args],cwd=cwd)
assert git('rev-parse','HEAD').decode().strip()==sha
assert not git('diff','--name-only')
owned=['tools/generate-contracts.mjs','tests/contracts/generation_reproducibility_test.go','docs/justix-auto/dev/results/T-937.md']
assert set(git('diff','--name-only',old,sha).decode().splitlines())==set(owned)
inserts={}
for file in owned[:2]:
 before=git('show',old+':'+file).decode();after=(wt/file).read_text()
 ops=difflib.SequenceMatcher(a=before.splitlines(),b=after.splitlines(),autojunk=False).get_opcodes()
 total=0
 for tag,a,b,c,d in ops:
  assert tag in ['equal','insert'];total+=d-c if tag=='insert' else 0
 inserts[file]=total
assert list(inserts.values())==[5,32]
before=git('show',old+':'+owned[2]);after=(wt/owned[2]).read_bytes();assert after.startswith(before)
artifacts=[wt/'docs/justix-auto/dev/qa/T-937.md',*sorted((wt/'docs/justix-auto/dev/qa/T-937').iterdir())]
assert len(artifacts)==11
hashes=[]
for file in artifacts:
 rel=str(file.relative_to(wt));data=file.read_bytes()
 assert data==(main/rel).read_bytes()==git('show','3725e0b:'+rel,cwd=main)
 hashes.append({'path':rel,'sha256':hashlib.sha256(data).hexdigest()})
resultHashes=[]
for name,expected in re.findall(r'\| `([^`]+)` \| `([a-f0-9]{64})` \|',after[len(before):].decode()):
 p=pathlib.Path(name) if name.startswith('/') else wt/name
 actual=hashlib.sha256(p.read_bytes()).hexdigest();assert actual==expected
 resultHashes.append({'path':name,'sha256':actual})
assert len(resultHashes)==7
task=(main/'docs/justix-auto/dev/tasks/T-937.md').read_text();assert '## Approved header schema representation' in task
print(json.dumps({'sha':sha,'previous':old,'insertedLines':inserts,'resultPrefixBytesPreserved':len(before),'originalArtifacts':hashes,'correctionHashes':resultHashes,'mainSHA':git('rev-parse','HEAD',cwd=main).decode().strip()},indent=2))
