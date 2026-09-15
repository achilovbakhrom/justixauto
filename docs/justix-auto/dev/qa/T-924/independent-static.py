import hashlib
import json
import pathlib
import subprocess

root = pathlib.Path(__file__).resolve().parents[5]
reviewed = 'd5dd07e4ca1327f5a9983be24797e11800e42914'
base = '288b084a988f62d09d1f08c664ffbdf1a3ba3498'

def git(*args):
    return subprocess.check_output(['git', '-C', str(root), *args])

assert git('rev-parse', 'HEAD').decode().strip() == reviewed
name = 'infra/local/rabbitmq/definitions.json'
old = json.loads(git('show', f'{base}:{name}'))
new = json.loads((root / name).read_bytes())
for key in sorted(old.keys() | new.keys()):
    if key not in ('users', 'permissions', 'topic_permissions'):
        assert old.get(key) == new.get(key), key
        print(key, 'UNCHANGED')
for name in ('infra/local/rabbitmq/definitions.json',
             'infra/local/rabbitmq/route-contracts.json',
             'tests/integration/broker_topology_test.go'):
    raw = (root / name).read_bytes()
    assert raw == git('show', f'{reviewed}:{name}'), name
    print(name, hashlib.sha256(raw).hexdigest(), 'EXACT COMMIT')
print('PASS: exact reviewed commit and all retained topology fields')
