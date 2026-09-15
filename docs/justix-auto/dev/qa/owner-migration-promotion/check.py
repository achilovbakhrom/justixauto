#!/usr/bin/env python3
"""Independent read-only exact-commit canonical promotion audit."""
import copy
import gzip
import hashlib
import json
import pathlib
import re
import subprocess
from collections import defaultdict

ROOT = pathlib.Path(__file__).resolve().parents[5]
SHA = '1a6d78b87eaa354561e36755877f8ef0a5327af3'
PROPOSAL = 'c3734a5c3f2863be8fe752338c11740b9bdf31a4'
DIGEST = '87ca5a5b7b5398726a53a9f4ec52b4f63c665e08ebaf69f57003401fa9ccbd76'
DOC = 'docs/justix-auto/'
BACKUP = DOC + 'state/backups/2026-09-15-owner-migration-promotion/'
checks = []

def git(*args):
    return subprocess.check_output(['git', '-C', str(ROOT), *args])

def check(value, label):
    if not value:
        raise AssertionError(label)
    checks.append(label)

def blob(commit, path):
    return git('show', commit + ':' + path)

def text(path):
    return (ROOT / path).read_text()

check(git('rev-parse', 'HEAD').decode().strip() == SHA, 'exact HEAD')
parent = git('rev-parse', SHA + '^').decode().strip()
changes = [line.split('\t') for line in git('diff', '--name-status', parent, SHA).decode().splitlines()]
check(all(kind in ('A', 'M') and path.startswith(DOC) for kind, path in changes), 'documentation-only additions/modifications; no deletions or application/dependency mutations')
for _, path in changes:
    check((ROOT / path).read_bytes() == blob(SHA, path), 'exact target bytes ' + path)
manifest = json.loads(text(BACKUP + 'manifest.json'))
check(len(manifest) == 19, '19 snapshot entries')
check({r['path'] for r in manifest} == {path for kind, path in changes if kind == 'M'}, 'every modified canonical file has one preimage')
for r in manifest:
    raw = gzip.decompress((ROOT / BACKUP / r['snapshot']).read_bytes())
    check(raw == blob(parent, r['path']) and len(raw) == r['bytes'] and hashlib.sha256(raw).hexdigest() == r['sha256'], 'recoverable exact-parent SHA256 preimage ' + r['path'])

proposal_path = DOC + 'state/drafts/architect/owner-migration-compatibility.md'
proposal = blob(PROPOSAL, proposal_path)
check(hashlib.sha256(proposal).hexdigest() == DIGEST, 'exact independently reviewed proposal digest')
for target in ('architect', 'contracts'):
    check((ROOT / (DOC + 'state/drafts/' + target + '/owner-migration-compatibility.md')).read_bytes() == proposal, 'canonical proposal copy ' + target)
check((ROOT / (DOC + 'dev/results/owner-migration-compatibility.md')).read_bytes() == blob(PROPOSAL, DOC + 'dev/results/owner-migration-compatibility.md'), 'exact proposal result retained')
prior_qa = DOC + 'dev/qa/owner-migration-compatibility.md'
check(blob(parent, prior_qa) == (ROOT / prior_qa).read_bytes(), 'prior independent proposal QA unchanged')

new = json.loads(text(DOC + 'dev/task-index.json'))
old = json.loads(blob(parent, DOC + 'dev/task-index.json'))
N = {t['id']: t for t in new['tasks']}
O = {t['id']: t for t in old['tasks']}
ids = ['T-' + str(n) for n in range(929, 935)]
check(len(N) == len(new['tasks']) == 934 and len(O) == 928 and set(N) - set(O) == set(ids), '934 unique tasks; exactly six additions')
check(sum(t['effort_hours'] for t in N.values()) == new['metadata']['effort_hours'] == 3102, '3102 task-effort hours')
check(new['metadata']['task_count'] == 934, 'metadata task count')
edges = {'T-059':['T-929','T-931'], 'T-060':['T-931'], 'T-022':['T-933','T-934'], 'T-580':['T-931','T-934','T-933']}
gated = ['T-' + str(n) for n in range(581, 587)]
leaf = 'services/identity/migrations/0012_auth.manifest.json'
for id, before in O.items():
    expected = copy.deepcopy(before)
    expected['depends_on'] += edges.get(id, [])
    if id == 'T-059': expected['files_owned'].append(leaf)
    if id in gated: expected['gates'].append('OWNER-COMPATIBILITY')
    check(expected == N[id], 'prior full task record preserved except approved amendment ' + id)

spec = {
 'T-929': ('OM-HISTORY',['T-008','T-024'],['pkg/eventstore/owner_history.sql','pkg/eventstore/owner_history_test.go']),
 'T-930': ('OM-PROFILE',['T-929','T-009','T-024'],['pkg/persistence/profile.go','pkg/persistence/profile_test.go','pkg/persistence/check.go','pkg/persistence/check_test.go']),
 'T-931': ('OM-IDENTITY',['T-930'],['services/identity/adapter/postgres/unit_of_work.go','services/identity/adapter/postgres/unit_of_work_test.go']),
 'T-932': ('OM-DRIVER',['T-929','T-930','T-001','T-005'],['tools/owner-migrate/driver.go','tools/owner-migrate/driver_test.go']),
 'T-933': ('OM-RUNNER',['T-932'],['tools/owner-migrate/main.go','tools/owner-migrate/main_test.go']),
 'T-934': ('OM-CUSTODY',['T-930','T-919','T-922','T-924','T-925','T-928'],['pkg/persistence/custody.go','pkg/persistence/custody_test.go','pkg/persistence/check.go','pkg/persistence/check_test.go'])}
for id, (alias, deps, files) in spec.items():
    t = N[id]
    check(t['depends_on'] == deps and t['files_owned'] == files, 'approved dependency/ownership ' + id)
    check(t['status'] == 'todo' and t['effort_hours'] == 4 and t['parent'] == 'B-01' and t['coverage'] == ['B-01.AC1'] and t['worktree'] is None and t['base_sha'] is None, 'bounded unassigned implementation contribution ' + id)
    check('- Proposal alias: ' + alias in text(DOC + 'dev/tasks/' + id + '.md'), 'exact alias ' + id)
meta = new['metadata']['owner_migration_compatibility']
check(meta['aliases'] == {v[0]: k for k,v in spec.items()} and meta['added_existing_edges'] == edges and meta['additional_schema_leaf'] == leaf and meta['remaining_owner_gate'] == {'tasks':gated,'gate':'OWNER-COMPATIBILITY'}, 'all promotion metadata maps exactly')
check(meta['proposal_sha'] == PROPOSAL and (ROOT / meta['approval']).is_file(), 'metadata approval provenance')
for k, v in old['metadata'].items():
    if k not in ('task_count','effort_hours','graph_metrics_scope','scope'):
        check(new['metadata'][k] == v, 'prior metadata retained ' + k)
check('916-task baseline' in new['metadata']['graph_metrics_scope'], 'schedule metrics explicitly historical')

visiting = set()
ancestors = {}
def visit(id):
    check(id not in visiting, 'acyclic visit ' + id)
    if id in ancestors: return ancestors[id]
    visiting.add(id)
    found = set()
    deps = N[id]['depends_on']
    check(len(deps) == len(set(deps)) and all(d in N for d in deps), 'unique existing dependencies ' + id)
    for d in deps:
        found.add(d)
        found.update(visit(d))
    visiting.remove(id)
    ancestors[id] = found
    return found
for id in N: visit(id)
check(set(ids) <= ancestors['T-591'], 'T591 remains closing ancestor for all additions')
owners = defaultdict(list)
for id,t in O.items():
    for file in t['files_owned']: owners[file].append(id)
new_leaves = {leaf}
for id in ids:
    for file in N[id]['files_owned']:
        if file in owners:
            check(id == 'T-931' and owners[file] == ['T-024'] and 'T-024' in ancestors[id], 'explicit ordered Identity successor ' + file)
        else: new_leaves.add(file)
check(len(new_leaves) == 13, '13 previously unassigned leaves including adjacent feature manifest')
check(set(N['T-934']['files_owned']) & set(N['T-930']['files_owned']) == {'pkg/persistence/check.go','pkg/persistence/check_test.go'} and 'T-930' in ancestors['T-934'], 'ordered shared checker successor')
check(not (set(N['T-931']['files_owned']) & set(N['T-934']['files_owned'])), 'Identity and custody ownership disjoint')

board = text(DOC + 'dev/task-board.md')
rows = {}
for line in board.splitlines():
    match = re.match(r'\| \[(T-\d+)\]\(tasks/\1\.md\)',line)
    if match:
        check(match[1] not in rows, 'unique board row ' + match[1])
        rows[match[1]] = [cell.strip() for cell in line.split('|')[1:-1]]
check(set(rows) == set(N), 'board contains all and only indexed tasks')
for id,t in N.items():
    row = rows[id]
    check(row[1:6] == [t['parent'],t['title'],t['kind']+' / '+str(t['effort_hours']),t['lane'],t['status']], 'board identity/status/effort ' + id)
    check(re.findall(r'T-\d+',row[6]) == t['depends_on'] and row[7].strip('`') == t['branch'], 'board dependency/branch ' + id)
    if t['gates']: check(row[8] == ', '.join(t['gates']), 'board gates ' + id)
    task = text(DOC + 'dev/tasks/' + id + '.md')
    field = lambda name: re.search(r'^- ' + re.escape(name) + r': (.*)$',task,re.M)[1]
    check(field('Status') == t['status'] and re.findall(r'T-\d+',field('Depends on')) == t['depends_on'] and field('Branch').strip('`') == t['branch'], 'task file status/dependency/branch ' + id)
    for gate in t['gates']: check(gate in field('Gates'), 'task file gate ' + id + ' ' + gate)
    section = task.split('## Exact file ownership',1)[1].split('\n## ',1)[0]
    owned = re.findall(r'^- `([^`]+)`',section,re.M)
    check(len(owned) == len(t['files_owned']) and set(owned) == set(t['files_owned']), 'task file exact ownership ' + id)

coverage = text(DOC + 'dev/backlog-coverage.md')
row = next(line for line in coverage.splitlines() if line.startswith('| B-01.AC1 |'))
check(all(id in row for id in ids) and 'T-591' in row, 'coverage additions and original closer')
link_count = 0
for _,path in changes:
    if not path.endswith('.md'): continue
    for target in re.findall(r'\[[^\]]*\]\(([^)]+)\)',text(path)):
        if '://' in target or target.startswith('#'): continue
        target = target.split('#')[0]
        check((ROOT / path).parent.joinpath(target).resolve().exists(), 'relative link ' + path + ' -> ' + target)
        link_count += 1
subprocess.run(['git','-C',str(ROOT),'diff','--check',parent,SHA],check=True)
result = {'outcome':'GREEN','commit':SHA,'parent':parent,'proposal':PROPOSAL,'checks':len(checks),'relative_links':link_count,'snapshot_count':19,'prior_records_retained':928,'task_count':934,'effort_hours':3102,'new_leaves':sorted(new_leaves),'verification_groups':['exact commit and changed-file bytes','documentation-only scope','all modified canonical preimages recoverable against exact parent','exact proposal and result copies','prior proposal QA retained','all prior task records preserved with only approved deltas','six aliases, ownership, dependencies and bounded unassigned status','historical metrics and preserved metadata','DAG and closer ancestry','serial successor ownership and 13 previously unassigned leaves','934 board/task/index records agree','coverage and 1052 relative Markdown links','git diff --check']}
print(json.dumps(result,indent=2))
