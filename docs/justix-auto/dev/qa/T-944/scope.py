import base64,gzip,hashlib,json,pathlib,re,subprocess
wt=pathlib.Path.cwd();sha='26e2ef72ce270d0af6954caf24e2a49e18162532'
def git(*args):return subprocess.check_output(['git',*args])
assert git('rev-parse','HEAD').decode().strip()==sha
assert not git('diff','--name-only')
base=git('rev-parse','b2dd2bf').decode().strip()
files=['web/packages/api/src/authEpoch.ts','web/packages/api/src/authEpoch.test.ts','web/packages/api/package.json','docs/justix-auto/dev/results/T-944.md']
assert set(git('diff','--name-only',base,sha).decode().splitlines())==set(files)
before=json.loads(git('show',base+':web/packages/api/package.json'));after=json.loads((wt/'web/packages/api/package.json').read_text())
before['exports']['./authEpoch']='./src/authEpoch.ts';assert before==after
result=(wt/files[-1]).read_text();sourceHashes=[]
for expected,name in re.findall(r'^([a-f0-9]{64})  (web/[^\n]+)$',result,re.M):
 actual=hashlib.sha256((wt/name).read_bytes()).hexdigest();assert actual==expected
 sourceHashes.append({'path':name,'sha256':actual})
assert len(sourceHashes)==3
block=re.findall(r'```text\n(.*?)\n```',result,re.S)[-1]
archive=gzip.decompress(base64.b64decode(block));archiveHash=hashlib.sha256(archive).hexdigest()
assert archiveHash=='0337ac2e7739137c57c13b926f6efb2863f330b8f0e51ebd2a1071ec559789a3'
logs=json.loads(archive);assert len(logs)==19
recovered=[]
for name,value in logs.items():
 data=base64.b64decode(value['bytes_base64']);assert hashlib.sha256(data).hexdigest()==value['sha256']
 recovered.append({'name':name,'bytes':len(data),'sha256':value['sha256']})
main=wt.parent.parent;task=(main/'docs/justix-auto/dev/tasks/T-944.md').read_text()
assert sha in task and 'ready-for-qa' in task
print(json.dumps({'sha':sha,'base':base,'changedLeaves':files,'onlyAuthEpochExportAdded':True,'sourceHashes':sourceHashes,'archiveJSONSHA256':archiveHash,'recoveredDeveloperLogs':recovered},indent=2))
