from pathlib import Path
import hashlib
import re
import subprocess

root = Path.cwd()
expected = '3a166bc39ed93eed13662a951da1626fab01fcb0'
prior = '8692b939de06b696269c7cce9b31c247b5762f25'
base = '0df6cdcd7ee77f7d07a1a2c151695274af8728ba'
def git(*args):
    return subprocess.check_output(['git', *args])
assert git('rev-parse', 'HEAD').decode().strip() == expected
assert git('diff', '--name-only') == b''
assert git('diff', '--cached', '--name-only') == b''
assert set(git('diff', '--name-only', prior, expected).decode().splitlines()) == {
    'pkg/projection/checkpoint.go', 'pkg/projection/sequence_test.go',
    'docs/justix-auto/dev/results/T-014.md'}
print('Exact SHA and bounded three-file fix diff PASS; tracked tree unchanged')
artifacts = [
    'pkg/eventstore/schema.sql',
    'pkg/eventstore/migrations/000002_messaging_delivery.up.sql',
    'pkg/eventstore/migrations/000003_messaging_route_compatibility.up.sql',
    'pkg/eventstore/migrations/000004_projection_checkpoint.up.sql',
]
for path in artifacts:
    content = (root / path).read_bytes()
    assert content == git('show', base + ':' + path)
    assert content == git('show', prior + ':' + path)
    print('Unchanged prerequisite', hashlib.sha256(content).hexdigest(), path)
old = root / 'docs/justix-auto/dev/qa/T-014/independent_test.go'
assert hashlib.sha256(old.read_bytes()).hexdigest() == '335ac254135e5f68310386e58791babd30c25ae71655bf2da9056c4f7c882ec6'
main = root.parents[1]
for path in sorted((root / 'docs/justix-auto/dev/qa/T-014').iterdir()):
    relative = path.relative_to(root)
    assert path.read_bytes() == (main / relative).read_bytes(), str(relative)
assert (root / 'docs/justix-auto/dev/qa/T-014.md').read_bytes() == (main / 'docs/justix-auto/dev/qa/T-014.md').read_bytes()
print('Original BOUNCE report and all five original evidence files match preserved main copies')
log = ''.join(Path(p).read_text() for p in [
    '/private/tmp/justixauto-t014-r2-qa-race.log',
    '/private/tmp/justixauto-t014-r2-qa-independent.log',
])
containers = set(re.findall(r'owned fixture container: (justixauto-t014-[0-9a-f-]{36})', log))
containers.update(re.findall(r'failed setup cleanup verified: container=(justixauto-t014-[0-9a-f-]{36})', log))
volumes = set(re.findall(r'owned fixture anonymous volume: ([0-9a-f]{64})', log))
volumes.update(re.findall(r'anonymous-volume=([0-9a-f]{64})', log))
assert len(containers) == len(volumes) == 4, (containers, volumes)
for kind, identifiers in [('container', containers), ('volume', volumes)]:
    for identifier in sorted(identifiers):
        args = ['docker', 'inspect', identifier] if kind == 'container' else ['docker', 'volume', 'inspect', identifier]
        result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        marker = 'no such object:' if kind == 'container' else 'no such volume'
        assert result.returncode != 0 and marker in result.stdout.lower(), (args, result.stdout)
        print('Absent', kind, identifier)
print('PASS: four exact owned containers and four captured anonymous volumes absent; no cleanup performed by this checker')
