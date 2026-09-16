import hashlib, json, pathlib, re, subprocess
root=pathlib.Path(__file__).resolve().parents[5]
main=root.parent.parent
target='c877b75c4a07969775c2a3aebce99e390de5c38d'
prior='38d67c2cae8065ecccdd70ff3597d83294fd4eeb'
base='d59faa8dc5b26e13e19c47686dd47744b6d73a11'
def git(*args):return subprocess.check_output(['git','-C',str(root),*args])
def sha(data):return hashlib.sha256(data).hexdigest()
assert git('rev-parse','HEAD').decode().strip()==target
owned=['pkg/eventstore/migrations/000006_projection_generation.up.sql','pkg/eventstore/projection_generation_migration_test.go','docs/justix-auto/dev/results/T-928.md']
assert set(git('diff','--name-only',base,target).decode().splitlines())==set(owned)
hashes={}
for path in owned:
    raw=(root/path).read_bytes()
    assert raw==git('show',target+':'+path),path
    hashes[path]=sha(raw)
assert hashes[owned[0]]=='13706a57d6b8fccc57956043a816836d2fc621ad0c823024e347a205524b4afe'
protected=['pkg/eventstore/schema.sql','pkg/eventstore/migrations/000002_messaging_delivery.up.sql','pkg/eventstore/migrations/000003_messaging_route_compatibility.up.sql','pkg/eventstore/migrations/000004_projection_checkpoint.up.sql','pkg/eventstore/migrations/000005_quarantine_evidence.up.sql']
for path in protected:
    raw=(root/path).read_bytes()
    assert raw==git('show',base+':'+path)==git('show',target+':'+path),path
    hashes[path]=sha(raw)
current_sql=(root/owned[0]).read_text()
block=re.search(r"  -- The installer's effective setting.*?(?=  -- Trigger/FK integrity)",current_sql,re.S).group()
assert current_sql.replace(block,'',1)==git('show',prior+':'+owned[0]).decode()
assert 'IF NOT r.rolcanlogin' in block and "setting='session_replication_role=origin')<>1" in block
assert 's.setrole=r.oid AND s.setdatabase=(SELECT oid FROM pg_database WHERE datname=current_database())' in block
assert not re.search(r'\b(?:ALTER|GRANT|REVOKE|UPDATE|INSERT|DELETE)\b',block)
def funcs(text):
    matches=list(re.finditer(r'^func (\w+)\([^\n]*',text,re.M))
    return {m.group(1):text[m.start():matches[i+1].start() if i+1<len(matches) else len(text)].rstrip() for i,m in enumerate(matches)}
old_tests=funcs(git('show',prior+':'+owned[1]).decode())
new_tests=funcs((root/owned[1]).read_text())
retained=[]
for name,body in old_tests.items():
    if name!='generationStore':
        assert new_tests[name]==body,name
        retained.append(name)
original={}
for suite,commit in [('T-928','c48dc79994af0b36a85d75fad93bc473d711a0df'),('T-928-r2','c40846186a04a62ed55bedc152387beb9479513a')]:
    paths=[main/f'docs/justix-auto/dev/qa/{suite}.md',*sorted((main/f'docs/justix-auto/dev/qa/{suite}').glob('*'))]
    assert len(paths)==9,(suite,len(paths))
    for path in paths:
        rel=str(path.relative_to(main));expected=git('show',commit+':'+rel)
        assert path.read_bytes()==expected,rel
        local=root/rel
        if local.exists():assert local.read_bytes()==expected,rel
        original[rel]=sha(expected)
assert len(original)==18
probe=(root/'docs/justix-auto/dev/qa/T-928/independent_probe_test.go').read_text()
groups=re.findall(r't.Run\("([^"\n]+)"',probe)
assert len(groups)==7,groups
runs={}
for label,path,want_dirs in [('developer_final',pathlib.Path('/private/tmp/justixauto-t928-fix2-full-race.log'),6),('prior_interrupted_r2',main/'docs/justix-auto/dev/qa/T-928-r2/full-race.log',5)]:
    raw=path.read_bytes();body=raw.decode()
    directories=list(dict.fromkeys(re.findall(r'Recoverable synthetic migration evidence: (\S+)',body)))
    assert len(directories)==want_dirs,(label,directories)
    records=[]
    for directory in directories:
        assert pathlib.Path(directory).is_dir(),directory
        for sidecar in sorted(pathlib.Path(directory).glob('*.sha256')):
            content=pathlib.Path(str(sidecar)[:-7]);digest=sha(content.read_bytes())
            assert sidecar.read_text().strip()==digest,content
            records.append({'path':str(content),'sha256':digest})
    assert len(records)==42,(label,len(records))
    fixtures=re.findall(r'validated task fixture removed and verified absent: ([0-9a-f]{64}) (justixauto-t927-[0-9a-f-]{36})',body)
    assert len(fixtures)==want_dirs,(label,fixtures)
    runs[label]={'log':str(path),'sha256':sha(raw),'preimages':records,'fixture_ids_reported_in_log':fixtures,'process_exit':'not observed by this r3 reviewer','package_result_lines':re.findall(r'^ok\s+justixauto/[^\n]+',body,re.M)}
print(json.dumps({'result':'PASS for read-only integrity checks only','target':target,'source_hashes':hashes,'final_sql_addition':block,'retained_test_functions':retained,'original_seven_groups':groups,'preserved_qa_hashes':original,'retained_runs':runs,'live_validation':'NOT RUN; provisioning authorization unresolved'},indent=2))
