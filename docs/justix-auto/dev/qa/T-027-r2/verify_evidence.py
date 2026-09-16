import hashlib, json, pathlib, re, subprocess
root = pathlib.Path(__file__).resolve().parents[5]
target = '0dab8c7ae691d7c34cd681d4f5894c6a4a188ef4'
original = 'db5c28c6ea7fe709d874f4e2a083d891a965877b'
def git(*args): return subprocess.check_output(['git','-C',str(root),*args])
def sha(data): return hashlib.sha256(data).hexdigest()
assert git('rev-parse','HEAD').decode().strip() == target
directory=root/'docs/justix-auto/dev/qa/T-027'
manifest=json.loads((directory/'sha256.json').read_text())
for path,expected in manifest.items(): assert sha((root/path).read_bytes())==expected,path
artifacts=[root/path for path in manifest]+[directory/'sha256.json']
assert len(artifacts)==14
original_hashes={}
for path in artifacts:
    rel=str(path.relative_to(root))
    assert path.read_bytes()==git('show','main:'+rel),rel
    original_hashes[rel]=sha(path.read_bytes())
changed=set(git('diff','--name-only',original,target).decode().splitlines())
assert changed=={'services/retail/adapter/postgres/unit_of_work.go','services/retail/adapter/postgres/unit_of_work_test.go','docs/justix-auto/dev/results/T-027.md'},changed
sources={}
for path in sorted((root/'services/retail').rglob('*')):
    if path.is_file():
        rel=str(path.relative_to(root))
        assert path.read_bytes()==git('show',target+':'+rel),rel
        sources[rel]=sha(path.read_bytes())
assert len(sources)==5
result=(root/'docs/justix-auto/dev/results/T-027.md').read_text()
assert result.startswith(git('show',original+':docs/justix-auto/dev/results/T-027.md').decode())
logs=re.findall(r'(justixauto-t027-fix1-\S+) SHA-256 `([0-9a-f]{64})`\n\n```json\n(.*?)\n```',result,re.S)
assert len(logs)==3
decoded_logs={}
for name,expected,encoded in logs:
    raw=json.loads(encoded).encode()
    assert sha(raw)==expected,name
    decoded_logs[name]=expected
for path in ['services/retail/adapter/postgres/unit_of_work.go','services/retail/adapter/postgres/unit_of_work_test.go']:
    expected=re.search(r'- `'+re.escape(path)+r'`: `([0-9a-f]{64})`',result).group(1)
    assert sources[path]==expected,path
for path in ['services/retail/migrations/0001_mechanics.up.sql','services/retail/port/unit_of_work.go','services/retail/port/dependencies.go','pkg/eventstore/schema.sql','pkg/eventstore/migrations/000002_messaging_delivery.up.sql','pkg/eventstore/transaction.go']:
    assert git('show',original+':'+path)==git('show',target+':'+path),(path,'unexpected mutation')
print(json.dumps({'result':'PASS','reviewed_commit':target,'changed_files':sorted(changed),'sources':sources,'original_qa_artifacts':original_hashes,'decoded_developer_logs':decoded_logs},indent=2))
