import base64, gzip, hashlib, json, pathlib, re, subprocess, sys

root = pathlib.Path.cwd()
qa = root / 'docs/justix-auto/dev/qa/T-932'
target = '555480d7b1c794fd5e9016545d459c975edd756f'
prepared = '45df272b986c0c6be2998bcc018c2eec277f91b0'
def git(*args):
    return subprocess.check_output(['git', *args], cwd=root)
def sha(b): return hashlib.sha256(b).hexdigest()
assert git('rev-parse', 'HEAD').decode().strip() == target
entries = []
seen = set()
def retain(path, kind):
    p = pathlib.Path(path).resolve()
    if str(p) in seen: return
    seen.add(str(p))
    b = p.read_bytes()
    entries.append(dict(path=str(p), kind=kind, sha256=sha(b), bytes=len(b), base64=base64.b64encode(b).decode()))

source = []
for name in ['tools/owner-migrate/driver.go', 'tools/owner-migrate/driver_test.go', 'docs/justix-auto/dev/results/T-932.md', 'go.mod', 'go.sum', 'docs/justix-auto/dev/results/T-932-dependency-preparation.md']:
    p=root/name; b=p.read_bytes()
    assert b == git('show', target+':'+name), name
    if name in ['go.mod','go.sum','docs/justix-auto/dev/results/T-932-dependency-preparation.md']:
        assert b == git('show', prepared+':'+name), name
    source.append(dict(path=name,sha256=sha(b)))
    retain(p,'exact-reviewed-source')
changed=git('diff','--name-only',prepared,target).decode().splitlines()
assert set(changed)=={'tools/owner-migrate/driver.go','tools/owner-migrate/driver_test.go','docs/justix-auto/dev/results/T-932.md'},changed
for name in ['pkg/eventstore/schema.sql','pkg/eventstore/owner_history.sql','services/identity/migrations/0001_mechanics.up.sql','pkg/persistence/profile.go','pkg/persistence/check.go']:
    p=root/name
    if not p.exists(): continue
    b=p.read_bytes();assert b==git('show',prepared+':'+name)==git('show',target+':'+name),name
    source.append(dict(path=name,sha256=sha(b)));retain(p,'unchanged-contract-source')

auditpath=pathlib.Path('/private/tmp/justixauto-t932-reviewed-evidence-audit.json')
author=json.loads(auditpath.read_text());retain(auditpath,'author-inventory')
sidecars=[];logs=[]
for group in author:
    logs.append(pathlib.Path(group['log']))
    for item in group['verified_sidecars']:
        p=pathlib.Path(item['path']);b=p.read_bytes();s=pathlib.Path(str(p)+'.sha256')
        expected=s.read_text().split()[0]
        assert sha(b)==expected==item['sha256'],p
        sidecars.append(dict(path=str(p.resolve()),sha256=sha(b)))
        retain(p,'author-retained-data');retain(s,'author-sidecar')
assert len(sidecars)==51,len(sidecars)
logs.extend(pathlib.Path('/private/tmp')/name for name in ['justixauto-t932-final-scoped-race.log','justixauto-t932-reviewed-vet.log','justixauto-t932-preflight.log'])
missing=[]
for p in logs:
    if p.exists():retain(p,'author-log')
    else:missing.append(str(p))
refs=set()
for p in logs:
    if not p.exists():continue
    for a,b in re.findall(r'validated[^\n]*?absent: ([0-9a-f]{64}) (justixauto-[\w-]+)',p.read_text()):refs.update([a,b])
archive=json.dumps(entries,separators=(',',':')).encode()
compressed=gzip.compress(archive,mtime=0)
(qa/'author-archive.json.gz.b64').write_text(base64.b64encode(compressed).decode()+'\n')
inventory=[{k:v for k,v in x.items() if k!='base64'} for x in entries]
result=dict(target=target,prepared_base=prepared,changed=changed,source=source,author_sidecars=sidecars,author_sidecars_count=len(sidecars),archive_entries=len(entries),archive_compressed_sha256=sha(compressed),archive_json_sha256=sha(archive),inventory=inventory,missing_optional_logs=missing,author_fixture_references=sorted(refs))
(qa/'source-author-audit.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps({k:v for k,v in result.items() if k not in ['inventory','author_sidecars','author_fixture_references']},indent=2))
