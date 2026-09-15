#!/usr/bin/env python3
"""Exact canonical promotion verification. No application/runtime tests."""
import copy
import gzip
import hashlib
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[5]
SHA = '940094adae97ef9582ffebadb4f69ce6fba2f88f'
PROPOSAL_SHA = '90dfe726ab53c4fdd54c3cac09e706797a32ea6d'
DIGEST = 'c15418fe9d63215b8301cc0349c7a4bdbb17ba611e25d58fc97b7345d3ff191a'
BASE = 'docs/justix-auto/'
BACKUPS = BASE + 'state/backups/2026-09-15-projection-storage-promotion'
EXTRA = {
    'T-014': ['T-926'], 'T-015': ['T-927', 'T-922'],
    'T-920': ['T-014'], 'T-921': ['T-015'],
    'T-016': ['T-928', 'T-920'], 'T-022': ['T-928'],
    **{f'T-{i:03d}': ['T-016'] for i in range(580, 587)},
}
NEW = {'T-926': ['T-925', 'T-008', 'T-009'], 'T-927': ['T-926', 'T-013'], 'T-928': ['T-927']}

def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT)

def check(ok, label):
    if not ok:
        raise AssertionError(label)
    print('PASS:', label)

def read(path):
    return (ROOT / path).read_text()

check(git('rev-parse', 'HEAD').decode().strip() == SHA, 'exact canonical promotion ' + SHA)
print('Parent:', git('rev-parse', SHA + '^').decode().strip())
git('diff', '--check', SHA + '^', SHA)
changes = [line.split('\t') for line in git('diff', '--name-status', SHA + '^', SHA).decode().splitlines()]
check(all(status in ('A', 'M') and path.startswith(BASE) for status, path in changes), 'documentation-only additions/modifications; no deletion or application change')
modified = {path for status, path in changes if status == 'M'}
manifest = json.loads(read(BACKUPS + '/manifest.json'))
check(len(manifest) == len({m['path'] for m in manifest}) == 20, '20 unique canonical preimages')
check({m['path'] for m in manifest} == modified, 'preimages cover every and only modified canonical document')
for item in manifest:
    before = git('show', SHA + '^:' + item['path'])
    recovered = gzip.decompress((ROOT / BACKUPS / item['snapshot']).read_bytes())
    check(recovered == before and len(recovered) == item['bytes'] and hashlib.sha256(recovered).hexdigest() == item['sha256'], 'recoverable exact parent SHA-256 preimage ' + item['path'])

architect = BASE + 'state/drafts/architect/projection-storage-handoff.md'
contract = BASE + 'state/drafts/contracts/projection-storage-handoff.md'
original = git('show', PROPOSAL_SHA + ':' + architect)
check(hashlib.sha256(original).hexdigest() == DIGEST, 'reviewed proposal digest')
for path in (architect, contract):
    check((ROOT / path).read_bytes() == original, 'exact reviewed proposal copy ' + path)
result = BASE + 'dev/results/projection-storage-handoff.md'
check((ROOT / result).read_bytes() == git('show', PROPOSAL_SHA + ':' + result), 'architect result provenance preserved byte-for-byte')
approval = read(BASE + 'state/approvals/projection-storage-handoff.md')
check(PROPOSAL_SHA in approval and DIGEST in approval and 'implementation only' in approval, 'approval pins proposal and bounded implementation authority')

link_paths = {path for _, path in changes if path.endswith('.md')}
link_paths.add(BASE + 'dev/qa/projection-storage-handoff.md')
count = 0
for path in sorted(link_paths):
    for link in re.findall(r'\[[^\]]+\]\(([^)]+)\)', read(path)):
        if '://' in link or link.startswith('#'):
            continue
        target = (ROOT / path).parent / link.split('#')[0]
        if not target.exists():
            raise AssertionError('broken link ' + path + ' -> ' + link)
        count += 1
print('PASS: changed/provenance/QA relative links resolve:', count)

index_path = BASE + 'dev/task-index.json'
before = json.loads(git('show', SHA + '^:' + index_path))
after = json.loads(read(index_path))
old = {t['id']: t for t in before['tasks']}
tasks = {t['id']: t for t in after['tasks']}
check(len(old) == 925 and len(tasks) == 928 and set(tasks) - set(old) == set(NEW), '925 baseline tasks retained plus exactly T-926/927/928')
check([t['id'] for t in after['tasks'][:925]] == [t['id'] for t in before['tasks']], 'original 925 task ordering retained')
check(sum(t['effort_hours'] for t in tasks.values()) == after['metadata']['effort_hours'] == 3078, '928 tasks total 3078 task-effort hours')
check(after['metadata']['task_count'] == 928, 'metadata task count 928')
check(sum(map(len, EXTRA.values())) == 15, 'exactly fifteen specified existing-task edge additions')
for id_, task in old.items():
    expected = copy.deepcopy(task)
    expected['depends_on'] += EXTRA.get(id_, [])
    if tasks[id_] != expected:
        raise AssertionError('unexpected prior task change ' + id_)
print('PASS: all 925 prior task records otherwise unchanged, including statuses, ownership, scope, effort and evidence')
allowed_meta = {'task_count', 'effort_hours', 'projection_storage_handoff', 'graph_metrics_scope'}
check({k for k in before['metadata'].keys() | after['metadata'].keys() if before['metadata'].get(k) != after['metadata'].get(k)} == allowed_meta, 'only four intended metadata fields changed; historical graph measurements retained')
check(after['metadata']['projection_storage_handoff']['added_existing_edges'] == EXTRA, 'edge metadata matches approved amendment')

old_owned = {p for task in old.values() for p in task['files_owned']}
new_leaves = [p for id_ in NEW for p in tasks[id_]['files_owned']]
check(len(new_leaves) == len(set(new_leaves)) == 8, 'eight distinct new leaves')
check(not old_owned.intersection(new_leaves) and not any((ROOT / p).exists() for p in new_leaves), 'new leaves are absent and nonoverlapping with all prior ownership')
proposal_leaves = re.findall(r'`(pkg/[^`]+)`', original.decode().split('| Proposed task |', 1)[1].split('Ancillary results/', 1)[0])
check(set(new_leaves) == set(proposal_leaves), 'new ownership equals exact approved eight leaves')
for id_, deps in NEW.items():
    t = tasks[id_]
    check(t['depends_on'] == deps and t['effort_hours'] == 4 and t['status'] == 'todo' and t['worktree'] is None and t['base_sha'] is None and t['coverage'] == ['B-01.AC2'], id_ + ' bounded dependency/status/coverage contract')

visited, active = set(), set()
def visit(id_):
    if id_ in active or id_ not in tasks:
        raise AssertionError('cycle or missing dependency: ' + id_)
    if id_ in visited:
        return
    active.add(id_)
    for dep in tasks[id_]['depends_on']:
        visit(dep)
    active.remove(id_)
    visited.add(id_)
for id_ in tasks:
    visit(id_)
check(len(visited) == 928, 'acyclic complete 928-node DAG')

board = read(BASE + 'dev/task-board.md')
rows = {}
for line in board.splitlines():
    m = re.match(r'\| \[(T-\d+)\]\(tasks/T-\d+\.md\) \|', line)
    if m:
        if m[1] in rows:
            raise AssertionError('duplicate board row ' + m[1])
        rows[m[1]] = [s.strip() for s in line.split('|')[1:-1]]
check(set(rows) == set(tasks), 'board contains each of 928 tasks exactly once')
for id_, task in tasks.items():
    doc = read(BASE + 'dev/tasks/' + id_ + '.md')
    dep_line = re.search(r'^- Depends on: (.*)$', doc, re.M)[1]
    doc_deps = re.findall(r'T-\d+', dep_line)
    board_deps = re.findall(r'T-\d+', rows[id_][6])
    status = re.search(r'^- Status: (.*)$', doc, re.M)[1]
    if doc_deps != task['depends_on'] or board_deps != task['depends_on'] or status != task['status'] or rows[id_][5] != task['status']:
        raise AssertionError('board/index/task dependency or status mismatch ' + id_)
print('PASS: all 928 task/index/board dependencies and statuses agree')
state = read(BASE + 'dev/dev-state.md')
integrated = sum(t['status'] == 'integrated' for t in tasks.values())
check(f'Integrated tasks: {integrated}/928' in state and 'Formal PM task files: 928' in state and '3,078 total task-effort hours' in board, 'state/board count and effort consistency')
for id_ in set(EXTRA) | {'T-923'}:
    doc = read(BASE + 'dev/tasks/' + id_ + '.md')
    check('../../state/drafts/contracts/projection-storage-handoff.md' in doc and 'No callback may acquire an earlier-order lock' in doc and 'Missing/inconsistent checkpoint evidence' in doc, id_ + ' carries approved lock/duplicate completion handoff')
check(tasks['T-923'] == old['T-923'], 'T-923 unchanged graph/ownership; receives required linked behavioral handoff')
coverage = read(BASE + 'dev/backlog-coverage.md')
ac2 = next(line for line in coverage.splitlines() if line.startswith('| B-01.AC2 |'))
check(all(id_ in ac2 for id_ in NEW) and ac2.endswith('| T-591 |'), 'AC2 adds three contributors and retains T-591 sole closer')
print('GREEN: canonical promotion structural checks; no SQL or production runtime certification.')
