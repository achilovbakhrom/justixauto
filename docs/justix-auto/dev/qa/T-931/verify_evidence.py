import base64,gzip,hashlib,json,pathlib,re,subprocess
root=pathlib.Path(__file__).resolve().parents[5]
target='aee07f4047ac293cc2c35349c5711a519e9475fc'
base='7cfd21465c80390d4d33ab40091a5360d12d88af'
def git(*args):return subprocess.check_output(['git','-C',str(root),*args])
def sha(b):return hashlib.sha256(b).hexdigest()
assert git('rev-parse','HEAD').decode().strip()==target
result_path='docs/justix-auto/dev/results/T-931.md'
result=(root/result_path).read_bytes();assert result==git('show',target+':'+result_path)
compressed=base64.b64decode(re.search(rb'```base64\n(.*?)\n```',result,re.S).group(1))
assert sha(compressed)=='97b2fa77690a77cf5618ccbba55f4b5e46f3e9ca033c05b321fa28c09402140a'
entries=json.loads(gzip.decompress(compressed));assert len(entries)==44
archive={}
for entry in entries:
    name=entry['name'];assert name not in archive and '..' not in pathlib.PurePosixPath(name).parts and not name.startswith('/')
    raw=base64.b64decode(entry['base64']);assert sha(raw)==entry['sha256'],name
    archive[name]=raw
source=json.loads(archive['verification/justixauto-t931-source-evidence.json'])
assert source['base']==base and len(source['source'])==12
for item in source['source']:
    path=item['path'];raw=(root/path).read_bytes();old=git('show',base+':'+path)
    assert raw==git('show',target+':'+path)
    assert sha(raw)==item['current_sha256'] and sha(old)==item['base_sha256'],path
    assert (raw==old)==item['unchanged'],path
expected={result_path,'services/identity/adapter/postgres/unit_of_work.go','services/identity/adapter/postgres/unit_of_work_test.go'}
assert set(git('diff','--name-only',base,target).decode().splitlines())==expected
for leaf in ['unit_of_work.go','unit_of_work_test.go']:
    assert archive['preimages/'+leaf]==git('show',base+':services/identity/adapter/postgres/'+leaf)
old=git('show',base+':services/identity/adapter/postgres/unit_of_work.go')
new=(root/'services/identity/adapter/postgres/unit_of_work.go').read_bytes()
assert old[old.index(b'func check('):]==new[new.index(b'func check('):]
assert b'session_replication_role' not in (root/'services/identity/adapter/postgres/unit_of_work_test.go').read_bytes()
archived_sidecars=0
for name,raw in archive.items():
    if name.endswith('.sha256'):
        assert sha(archive[name[:-7]])==raw.decode().strip(),name
        archived_sidecars+=1
assert archived_sidecars==9
def bundle(files):
    profile=json.loads(files['profile.json'])
    assert profile['Owner']=='identity' and profile['Head']==12
    for artifact in profile['Artifacts']:
        filename=artifact['Identity']['Filename'];manifest_name=filename.replace('.up.sql','.manifest.json')
        assert list(hashlib.sha256(files[filename]).digest())==artifact['Identity']['SHA256']
        assert list(hashlib.sha256(files[manifest_name]).digest())==artifact['ManifestSHA256']
        manifest=json.loads(files[manifest_name]);assert manifest['artifact']==artifact['Identity']
        if manifest['prerequisites'] is None:assert artifact['Identity']['Version']==1
        assert (manifest['prerequisites'] or [])==artifact['Prerequisites'] and manifest['feature_contract']==artifact['Feature']
    counts=None
    if 'before-readiness.json' in files:
        assert files['before-readiness.json']==files['after-readiness.json']
        before=json.loads(files['before-readiness.json'])
        counts={key:len(before[key] or []) for key in ['items','events','receipts','history']}
    return counts
archive_roots=sorted({name.split('/')[1] for name in archive if name.startswith('retained/')})
archived_bundles={}
for name in archive_roots:
    prefix='retained/'+name+'/'
    archived_bundles[name]=bundle({path[len(prefix):]:raw for path,raw in archive.items() if path.startswith(prefix)})
assert archived_bundles['justixauto-t931-evidence-2206865864']=={'items':1,'events':1,'receipts':1,'history':2}
fresh={};fixture_refs=set();author_refs=set()
for name,raw in archive.items():
    if name.startswith('logs/'):
        body=raw.decode()
        author_refs.update(re.findall(r'justixauto-t931-[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}',body))
        author_refs.update(re.findall(r'validated fixture removed and absent: ([0-9a-f]{64})',body))
assert len(author_refs)==32,len(author_refs)
for name in ['full-race.log','independent.log']:
    body=(root/'docs/justix-auto/dev/qa/T-931'/name).read_text()
    assert re.search(r'^ok\s+justixauto/services/identity/adapter/postgres',body,re.M)
    assert not re.search(r'^FAIL',body,re.M)
    fixture_refs.update(re.findall(r'justixauto-t931-[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}',body))
    fixture_refs.update(re.findall(r'validated fixture removed and absent: ([0-9a-f]{64})',body))
    for path in re.findall(r'retained synthetic evidence directory (\S+)',body):
        directory=pathlib.Path(path);files={str(p.relative_to(directory)):p.read_bytes() for p in directory.rglob('*') if p.is_file()}
        for leaf,raw in files.items():
            if leaf.endswith('.sha256'):assert sha(files[leaf[:-7]])==raw.decode().strip(),leaf
        fresh[path]={'counts':bundle(files),'files':{leaf:sha(raw) for leaf,raw in files.items()}}
assert len(fresh)==2
assert len(fixture_refs)==9,len(fixture_refs)
print(json.dumps({'result':'PASS','target':target,'source_checks':source,'archive_entries':{name:sha(raw) for name,raw in archive.items()},'archived_sidecars':archived_sidecars,'archived_bundle_counts':archived_bundles,'fresh_bundles':fresh,'author_fixture_refs':sorted(author_refs),'qa_fixture_refs':sorted(fixture_refs),'legacy_checker_byte_exact':True},indent=2))
