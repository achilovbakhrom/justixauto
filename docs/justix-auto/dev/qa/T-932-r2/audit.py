import base64, concurrent.futures, gzip, hashlib, json, pathlib, re, subprocess

ROOT = pathlib.Path.cwd()
QA = ROOT / 'docs/justix-auto/dev/qa/T-932-r2'
TARGET = '95952e896e0ea7e47d1ac93b4f8c594594f11268'
BEFORE = '555480d7b1c794fd5e9016545d459c975edd756f'
MAIN = pathlib.Path('/Users/bakhromachilov/startups/justixauto')
def git(*args): return subprocess.check_output(['git', *args], cwd=ROOT)
def sha(b): return hashlib.sha256(b).hexdigest()
def write(name, obj): (QA/name).write_text(json.dumps(obj, indent=2)+'\n')
assert git('rev-parse', 'HEAD').decode().strip() == TARGET
assert git('diff', '--name-only').decode() == ''
changed = git('diff', '--name-only', BEFORE, TARGET).decode().splitlines()
assert set(changed) == {'tools/owner-migrate/driver.go', 'tools/owner-migrate/driver_test.go', 'docs/justix-auto/dev/results/T-932.md'}
source = []
for name in changed + ['go.mod', 'go.sum', 'pkg/eventstore/schema.sql', 'pkg/eventstore/owner_history.sql', 'pkg/persistence/profile.go', 'pkg/persistence/check.go', 'services/identity/migrations/0001_mechanics.up.sql']:
    b=(ROOT/name).read_bytes()
    assert b == git('show', TARGET+':'+name), name
    if name not in changed: assert b == git('show', BEFORE+':'+name), name
    source.append(dict(path=name, bytes=len(b), sha256=sha(b)))
prefix=git('show', BEFORE+':docs/justix-auto/dev/results/T-932.md')
assert (ROOT/'docs/justix-auto/dev/results/T-932.md').read_bytes().startswith(prefix)
assert (QA/'original-driver.go').read_bytes() == git('show', BEFORE+':tools/owner-migrate/driver.go')
originals=[]
for p in [ROOT/'docs/justix-auto/dev/qa/T-932.md', *sorted((ROOT/'docs/justix-auto/dev/qa/T-932').rglob('*'))]:
    if not p.is_file(): continue
    rel=p.relative_to(ROOT); b=p.read_bytes()
    assert b == (MAIN/rel).read_bytes(), rel
    assert b == git('show', '31119ab:'+str(rel)), rel
    originals.append(dict(path=str(rel), sha256=sha(b), bytes=len(b)))
assert len(originals)==23, len(originals)

author_inventory=pathlib.Path('/private/tmp/justixauto-t932-fix1-evidence-audit.json')
author=json.loads(author_inventory.read_text())
author_entries={};fresh_entries={}; author_refs=set();fresh_refs=set()
def retain(entries,p,kind):
    p=pathlib.Path(p).resolve();b=p.read_bytes()
    entries[str(p)]=dict(path=str(p),kind=kind,sha256=sha(b),bytes=len(b),base64=base64.b64encode(b).decode())
def fixtures(log):
    made=set(re.findall(r'created owned fixture (justixauto-[\w-]+) id ([a-f0-9]{64})',log))
    removed={(name,ident) for ident,name in re.findall(r'validated fixture removed and absent: ([a-f0-9]{64}) (justixauto-[\w-]+)',log)}
    assert made==removed,(made-removed,removed-made)
    return made
retain(author_entries,author_inventory,'author-inventory')
author_sidecars=[]
for group in author:
    log=pathlib.Path(group['log']);b=log.read_bytes()
    assert sha(b)==group['log_sha256'],log
    retain(author_entries,log,'author-log')
    refs=fixtures(b.decode())
    assert refs=={(x['name'],x['id']) for x in group['fixtures']}
    for name,ident in refs: author_refs.update([name,ident])
    for item in group['verified_sidecars']:
        p=pathlib.Path(item['path']);side=pathlib.Path(str(p)+'.sha256')
        assert sha(p.read_bytes())==item['sha256']==side.read_text().split()[0],p
        retain(author_entries,p,'author-retained-data');retain(author_entries,side,'author-sidecar')
        author_sidecars.append(dict(path=str(p),sha256=item['sha256']))
for name in ['justixauto-t932-fix1-vet-r2.log','justixauto-t932-main-race.log']:
    retain(author_entries,pathlib.Path('/private/tmp')/name,'author-or-main-log')
assert sha(pathlib.Path('/private/tmp/justixauto-t932-main-race.log').read_bytes())=='a409332e4bc7eef907b30d1b42c8f4ce0a5e8bae8ce67ba960e1dba86159b585'
fresh_logs=[]; fresh_sidecars=[]
for name in ['scoped-race.log','scoped-vet.log','independent-race.log','independent-vet.log','before-regression.log','repeated-lock-race.log']:
    p=QA/name;b=p.read_bytes();log=b.decode()
    if name=='before-regression.log':
        assert log.count('physical transport Close has not returned')==2
        assert 'FAIL\tjustixauto/tools/owner-migrate' in log
    elif name.endswith('race.log'):
        assert '\nFAIL' not in log and 'WARNING: DATA RACE' not in log
        assert 'ok  \tjustixauto/tools/owner-migrate' in log
    else: assert b==b''
    fresh_logs.append(dict(path=str(p.relative_to(ROOT)),sha256=sha(b),bytes=len(b)))
    for fixture_name,ident in fixtures(log):fresh_refs.update([fixture_name,ident])
    for directory in sorted(set(re.findall(r'retained synthetic evidence directory (\S+)',log))):
        for side in pathlib.Path(directory).glob('*.sha256'):
            p=pathlib.Path(str(side)[:-7]);b=p.read_bytes()
            assert sha(b)==side.read_text().split()[0],p
            retain(fresh_entries,p,'fresh-retained-data');retain(fresh_entries,side,'fresh-sidecar')
            fresh_sidecars.append(dict(path=str(p.resolve()),sha256=sha(b)))
def absent(ref):
    r=subprocess.run(['docker','inspect','--format','{{.Id}}',ref],capture_output=True,text=True,timeout=10)
    assert r.returncode!=0 and 'no such object' in (r.stdout+r.stderr).lower(),(ref,r.returncode,r.stdout,r.stderr)
    return dict(reference=ref,absent=True)
with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
    absence=list(pool.map(absent,sorted(author_refs|fresh_refs)))
archives=[]
for name,entries in [('author-fix-evidence.json.gz.b64',author_entries),('fresh-evidence.json.gz.b64',fresh_entries)]:
    raw=json.dumps(list(entries.values()),separators=(',',':')).encode()
    compressed=gzip.compress(raw,mtime=0)
    (QA/name).write_text(base64.b64encode(compressed).decode()+'\n')
    decoded=json.loads(gzip.decompress(base64.b64decode((QA/name).read_bytes())))
    for entry in decoded:
        b=base64.b64decode(entry['base64'])
        assert sha(b)==entry['sha256'] and len(b)==entry['bytes']
    archives.append(dict(path=name,entries=len(entries),compressed_sha256=sha(compressed),json_sha256=sha(raw)))
result=dict(target=TARGET,before=BEFORE,changed=changed,source=source,result_prefix_bytes=len(prefix),result_prefix_sha256=sha(prefix),original_qa=originals,original_qa_count=len(originals),author_sidecars=author_sidecars,author_sidecars_count=len(author_sidecars),fresh_sidecars=fresh_sidecars,fresh_sidecars_count=len(fresh_sidecars),author_fixture_references=sorted(author_refs),fresh_fixture_references=sorted(fresh_refs),absence=absence,logs=fresh_logs,archives=archives)
write('audit.json',result)
print(json.dumps({k:result[k] for k in ['target','result_prefix_bytes','original_qa_count','author_sidecars_count','fresh_sidecars_count','archives']}))
print('Author absence lookups:',len(author_refs),'Fresh absence lookups:',len(fresh_refs))
