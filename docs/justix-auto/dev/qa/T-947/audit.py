"""Read-only exact-commit scope, input and recoverable evidence verification."""
import base64, gzip, hashlib, json, pathlib, re, subprocess

root = pathlib.Path(__file__).resolve().parents[5]
def git(*args):
    return subprocess.check_output(['git', '-C', str(root), *args])
def sha(b):
    return hashlib.sha256(b).hexdigest()
target = '51266880b925714483dd47cf50d59e57ec41e698'
base = 'cdc3d7a238fd1e33bc8222f06e0d65c5cf92cf14'
assert git('rev-parse', 'HEAD').decode().strip() == target
changed = git('diff', '--name-only', base, target).decode().splitlines()
assert set(changed) == {'pkg/inbox/membership_catalog.go', 'pkg/inbox/membership_catalog_test.go', 'docs/justix-auto/dev/results/T-947.md'}
text = (root/'docs/justix-auto/dev/results/T-947.md').read_text()
archive = base64.b64decode(re.search(r'```base64\n(.*?)\n```',text,re.S)[1])
assert sha(archive) == '824f195f4484cd48063fa5efbbf740fe950917ffffb0a84cc0c5e03299612561'
entries = json.loads(gzip.decompress(archive))
assert len(entries) == 7
assert len({e['name'] for e in entries}) == 7
inventory = []
for e in entries:
    body = base64.b64decode(e['base64'])
    assert sha(body) == e['sha256'], e['name']
    inventory.append({'name':e['name'], 'sha256':sha(body), 'bytes':len(body)})
    if e['name'] == 'justixauto-t947-source-evidence.json':
        sources = json.loads(body)
assert sources['base'] == base
assert len(sources['files']) == 11
unchanged = 0
for f in sources['files']:
    content = git('show', target+':'+f['path'])
    assert (root/f['path']).read_bytes() == content
    assert sha(content) == f['sha256'] and len(content) == f['bytes']
    if f.get('unchanged_from_base'):
        assert content == git('show', base+':'+f['path'])
        unchanged += 1
assert unchanged == 9
for p in ('pkg/inbox/membership_catalog.go','pkg/inbox/membership_catalog_test.go'):
    assert not git('ls-tree', base, '--', p)
assert sha((root/'docs/justix-auto/state/drafts/contracts/membership-version-storage.md').read_bytes()) == 'c468216706f0b40e01dc5c5e2bf07060cac60cecd429eac54ec5616bc9886330'
assert b'justixauto/pkg/projection' not in (root/'pkg/inbox/membership_catalog.go').read_bytes()
assert not git('diff', target, '--', 'pkg', 'services', 'go.mod','go.sum','docs/justix-auto/state')
subprocess.run(['git','-C',str(root),'diff','--check'],check=True)
print(json.dumps({'result':'PASS','commit':target,'base':base,'changed_paths':changed,'archive_sha256':sha(archive),'verified_entries':inventory,'verified_input_files':sources['files'],'unchanged_inputs':unchanged,'new_source_leaves':2,'canonical_contract_unchanged':True,'tracked_source_changes_after_commit':False}, indent=2))
