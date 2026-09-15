#!/usr/bin/env python3
"""Read-only, exact-commit proposal checks; no SQL/runtime certification."""
import hashlib
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[5]
MAIN = Path('/Users/bakhromachilov/startups/justixauto')
SHA = '90dfe726ab53c4fdd54c3cac09e706797a32ea6d'
BASE = '2c1ff1de53f4dc7b0588d63d6b34de1d0983f0b3'
PROPOSAL = 'docs/justix-auto/state/drafts/architect/projection-storage-handoff.md'
RESULT = 'docs/justix-auto/dev/results/projection-storage-handoff.md'

def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT)

def check(ok, label):
    if not ok:
        raise AssertionError(label)
    print('PASS:', label)

check(git('rev-parse', 'HEAD').decode().strip() == SHA, 'exact proposal HEAD ' + SHA)
changed = git('diff', '--name-status', BASE, SHA).decode().splitlines()
check(set(changed) == {'A\t' + PROPOSAL, 'A\t' + RESULT}, 'only two assigned new documentation leaves')
git('diff', '--check', BASE, SHA)
print('PASS: git diff --check BASE SHA')
for path in (PROPOSAL, RESULT):
    check((ROOT / path).read_bytes() == git('show', SHA + ':' + path), 'reviewed working bytes ' + path)
    print('SHA256:', path, hashlib.sha256((ROOT / path).read_bytes()).hexdigest())

proposal = (ROOT / PROPOSAL).read_text()
link_count = 0
for path in (PROPOSAL, RESULT):
    for link in re.findall(r'\[[^\]]+\]\(([^)]+)\)', (ROOT / path).read_text()):
        if '://' in link:
            continue
        check(((ROOT / path).parent / link.split('#')[0]).exists(), 'relative link ' + link)
        link_count += 1
print('PASS: relative links', link_count)

new = {
    'T-926': ['T-925', 'T-008', 'T-009'],
    'T-927': ['T-926', 'T-013'],
    'T-928': ['T-927'],
}
extra = {
    'T-014': ['T-926'], 'T-015': ['T-927', 'T-922'],
    'T-920': ['T-014'], 'T-921': ['T-015'],
    'T-016': ['T-928', 'T-920'], 'T-022': ['T-928'],
    **{f'T-{i:03d}': ['T-016'] for i in range(580, 587)},
}
leaves = re.findall(r'`(pkg/[^`]+)`', proposal.split('| Proposed task |', 1)[1].split('Ancillary results/', 1)[0])
check(len(leaves) == len(set(leaves)) == 8, 'exact eight unique proposed application leaves')
for tree, label in ((ROOT, 'assigned base'), (MAIN, 'current main')):
    index = json.loads((tree / 'docs/justix-auto/dev/task-index.json').read_text())
    tasks = {t['id']: t for t in index['tasks']}
    check(len(tasks) == 925 and not set(new).intersection(tasks), label + ': 925 baseline tasks; proposed IDs free')
    owned = {p for t in tasks.values() for p in t['files_owned']}
    for path in leaves:
        check(path not in owned and not (tree / path).exists(), label + ': absent/unowned ' + path)
    graph = {k: set(t['depends_on']) for k, t in tasks.items()}
    graph.update({k: set(v) for k, v in new.items()})
    added = 0
    for task, deps in extra.items():
        check(not graph[task].intersection(deps), label + ': genuinely new edges for ' + task)
        added += len(deps)
        graph[task].update(deps)
    check(added == 15, label + ': exactly 15 existing-task edge additions')
    visited, active = set(), set()
    def visit(node):
        if node in active:
            raise AssertionError('cycle at ' + node)
        if node in visited:
            return
        check(node in graph, label + ': dependency exists ' + node) if node not in graph else None
        active.add(node)
        for dep in graph[node]:
            visit(dep)
        active.remove(node)
        visited.add(node)
    for node in graph:
        visit(node)
    check(len(visited) == 928, label + ': all 928 nodes form an acyclic graph')
    check('T-920' not in graph['T-014'] and 'T-016' not in graph['T-920'], label + ': bootstrap/generation directions preserved')
    check(graph['T-919'] == set(tasks['T-919']['depends_on']), label + ': T-919 dependencies unchanged')

for path, digest in {
    'pkg/eventstore/schema.sql': '86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53',
    'pkg/eventstore/migrations/000002_messaging_delivery.up.sql': '1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2',
}.items():
    for tree in (ROOT, MAIN):
        check(hashlib.sha256((tree / path).read_bytes()).hexdigest() == digest, str(tree.name) + ': retained digest ' + path)

for sha, path in (
    ('e15f5e7a677084ad84c6952d53c53eddfb6d0b10', 'pkg/inbox/consume.go'),
    ('4dbe2593bce71c045f785fc07aed9da5b4113a73', 'pkg/eventstore/migrations/000003_messaging_route_compatibility.up.sql'),
):
    check(sha in proposal, 'exact implementation reference ' + sha)
    check((MAIN / path).read_bytes() == git('show', sha + ':' + path), 'current main matches referenced implementation ' + path)
print('Current main:', subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=MAIN).decode().strip())
print('GREEN: structural proposal checks only; lock/SQL/crypto behavior is not executed by this script.')
