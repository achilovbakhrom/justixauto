import hashlib
import json
import pathlib
import subprocess

root = pathlib.Path(__file__).resolve().parents[5]
def git(*args):
    return subprocess.check_output(['git', *args], cwd=root, text=True).strip()

assert git('rev-parse', 'HEAD') == 'ead003147ebbde7316d903923baf83cc5f0bcecd'
changed = set(git('diff', '--name-only', '8e714dc98659d2ef7c901362501d60dd7e7335db', 'HEAD').splitlines())
assert changed == {'pkg/eventstore/owner_history.sql', 'pkg/eventstore/owner_history_test.go', 'docs/justix-auto/dev/results/T-929.md'}
for name, expected in {
    'pkg/eventstore/owner_history.sql': '1e5138d344f07c3b3f78695f7ca7167f14b6582dbbbbd95654566eb8ddb58b70',
    'pkg/eventstore/owner_history_test.go': '66b69e4349f69ca966fb231bbc7ece5367092a56407a6149e358692d57f49578',
    'pkg/eventstore/schema.sql': '86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53',
}.items():
    assert hashlib.sha256((root/name).read_bytes()).hexdigest() == expected
print('PASS exact commit, three assigned additions, SQL/test/base digests; every pre-existing tracked source unchanged')

for run in ['2350788359', '3455070264']:
    directory = pathlib.Path('/private/var/folders/yq/j2ffv8bj2f35mcvbk7v7rk3m0000gn/T') / ('justixauto-t929-evidence-' + run)
    files = sorted(directory.glob('*.json'))
    assert len(files) == 4
    for path in files:
        data = path.read_bytes()
        digest = hashlib.sha256(data).hexdigest()
        assert digest == pathlib.Path(str(path)+'.sha256').read_text().strip()
        doc = json.loads(data)
        assert doc['ledger'] == [{'version': 1, 'dirty': False}]
        assert len(doc['events']) >= 1 and doc['catalog'] and doc['functions'] and len(doc['marker']) == 1
        if path.name.startswith('identity'):
            for key in ['receipts', 'outbox', 'inbox', 'operations', 'steps']:
                assert doc[key], (path, key)
        print('PASS retained full rows/catalog/functions', path.name, digest)
    assert (directory/'identity-before-rejections.json').read_bytes() == (directory/'identity-before-install.json').read_bytes() == (directory/'identity-after-install.json').read_bytes()
    print('PASS identity exact before-rejection/before-install/after-install byte equality for run', run)
