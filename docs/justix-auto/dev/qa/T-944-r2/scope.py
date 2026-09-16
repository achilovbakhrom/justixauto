import base64,gzip,hashlib,json,pathlib,re,subprocess
wt=pathlib.Path.cwd();main=wt.parent.parent;sha='a3c291d124bae46e381faa8307d02551c26d0bc3';old='26e2ef72ce270d0af6954caf24e2a49e18162532'
def git(*args,cwd=wt):return subprocess.check_output(['git',*args],cwd=cwd)
assert git('rev-parse','HEAD').decode().strip()==sha
assert not git('diff','--name-only')
files=['web/packages/api/src/authEpoch.ts','web/packages/api/src/authEpoch.test.ts','docs/justix-auto/dev/results/T-944.md']
assert set(git('diff','--name-only',old,sha).decode().splitlines())==set(files)
before=git('show',old+':'+files[-1]);after=(wt/files[-1]).read_bytes();assert len(before)==16901 and after.startswith(before)
originals=[wt/'docs/justix-auto/dev/qa/T-944.md',*sorted((wt/'docs/justix-auto/dev/qa/T-944').iterdir())];assert len(originals)==10
preserved=[]
for f in originals:
 rel=str(f.relative_to(wt));data=f.read_bytes();assert data==(main/rel).read_bytes()==git('show','809d539:'+rel,cwd=main)
 preserved.append({'path':rel,'sha256':hashlib.sha256(data).hexdigest()})
sourceHashes=[]
for expected,name in re.findall(r'^([a-f0-9]{64})  (web/[^\n]+)$',after[len(before):].decode(),re.M):
 actual=hashlib.sha256((wt/name).read_bytes()).hexdigest();assert actual==expected
 sourceHashes.append({'path':name,'sha256':actual})
assert len(sourceHashes)==3
archives=[]
for data,count,checksum in [(before,19,'0337ac2e7739137c57c13b926f6efb2863f330b8f0e51ebd2a1071ec559789a3'),(after[len(before):],9,'667727d7fcc99883f40d0fca038f2a44f95f28da31f0117f4df1ccf6bfb06978')]:
 block=re.findall(r'```text\n(.*?)\n```',data.decode(),re.S)[-1];raw=gzip.decompress(base64.b64decode(block));assert hashlib.sha256(raw).hexdigest()==checksum
 logs=json.loads(raw);assert len(logs)==count
 for v in logs.values():assert hashlib.sha256(base64.b64decode(v['bytes_base64'])).hexdigest()==v['sha256']
 archives.append({'count':count,'sha256':checksum,'logNames':list(logs)})
print(json.dumps({'sha':sha,'previous':old,'changedLeaves':files,'originalResultPrefixBytes':len(before),'originalArtifacts':preserved,'sourceHashes':sourceHashes,'archives':archives},indent=2))
