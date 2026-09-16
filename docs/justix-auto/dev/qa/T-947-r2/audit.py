"""Independent exact fix commit and retained evidence audit; no mutations."""
import base64,gzip,hashlib,json,pathlib,re,subprocess
root=pathlib.Path(__file__).resolve().parents[5]
def git(*args):return subprocess.check_output(['git','-C',str(root),*args])
def sha(b):return hashlib.sha256(b).hexdigest()
target='be944b633be73ba8d69942d561a379e33f01afb9'
prior='51266880b925714483dd47cf50d59e57ec41e698'
assert git('rev-parse','HEAD').decode().strip()==target
changed=git('diff','--name-only',prior,target).decode().splitlines()
assert set(changed)=={'pkg/inbox/membership_catalog.go','pkg/inbox/membership_catalog_test.go','docs/justix-auto/dev/results/T-947.md'}
result_path='docs/justix-auto/dev/results/T-947.md'
result=(root/result_path).read_bytes();old=git('show',prior+':'+result_path)
assert len(old)==50753 and result[:50753]==old
assert sha(old)=='493543ccf7a51def70831ed829306533e5703a43d6380d966b075c8668143170'
blocks=re.findall(rb'```base64\n(.*?)\n```',result,re.S)
assert len(blocks)==2
hashes=['824f195f4484cd48063fa5efbbf740fe950917ffffb0a84cc0c5e03299612561','7967c4dce8b86c8a523bf50084ecf964e2433d7934553e217ce3d8e6d5387c9f']
archives=[];decoded={}
for i,block in enumerate(blocks):
    packed=base64.b64decode(block);assert sha(packed)==hashes[i]
    entries=json.loads(gzip.decompress(packed));assert len(entries)==[7,6][i]
    inventory=[]
    for e in entries:
        b=base64.b64decode(e['base64']);assert sha(b)==e['sha256']
        assert e['name'] not in decoded;decoded[e['name']]=b
        inventory.append({'name':e['name'],'sha256':sha(b),'bytes':len(b)})
    archives.append({'sha256':sha(packed),'entries':inventory})
identities=json.loads(decoded['justixauto-t947-fix1-identities.json'])
oldqa=[]
for p,expected in identities['original_qa_and_result_prefix'].items():
    b=(root/p).read_bytes()
    if p==result_path:b=b[:50753]
    else:
        assert b==git('show','5a6fbe5:'+p)
        oldqa.append(p)
    assert len(b)==expected['size'] and sha(b)==expected['sha256']
assert len(oldqa)==8
for p,expected in identities['current_sources'].items():
    b=(root/p).read_bytes();assert b==git('show',target+':'+p) and sha(b)==expected
sources=json.loads(decoded['justixauto-t947-source-evidence.json'])
unchanged=[]
for f in sources['files']:
    if not f.get('unchanged_from_base'):continue
    b=(root/f['path']).read_bytes();assert sha(b)==f['sha256']
    assert b==git('show',sources['base']+':'+f['path'])
    unchanged.append(f['path'])
assert len(unchanged)==9
original=root/'docs/justix-auto/dev/qa/T-947/independent_test.go'
copy=root/'docs/justix-auto/dev/qa/T-947-r2/original_test.go'
assert original.read_bytes()==copy.read_bytes()
assert sha(copy.read_bytes())=='357017dcfbdbc736396138b69078f5e64b6ea83fa64f9f2fb027bf2e16ea3c63'
assert not git('diff',target,'--','pkg','services','go.mod','go.sum','docs/justix-auto/state','docs/justix-auto/dev/results')
subprocess.run(['git','-C',str(root),'diff','--check'],check=True)
print(json.dumps({'result':'PASS','target':target,'prior':prior,'changed_paths':changed,'original_result_prefix_bytes':len(old),'original_result_prefix_sha256':sha(old),'preserved_qa_artifacts':oldqa,'unchanged_approved_inputs':unchanged,'current_sources':identities['current_sources'],'original_regression_copy_byte_identical':True,'archives':archives,'source_diff_after_commit':False},indent=2))
