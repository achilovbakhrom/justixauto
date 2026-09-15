#!/usr/bin/env python3
"""Independent exact-object proposal, ownership/DAG and upstream API audit."""
import base64
import hashlib
import json
from pathlib import Path
import re
import subprocess
import zipfile

ROOT = Path(__file__).resolve().parents[5]
SHA = 'c3734a5c3f2863be8fe752338c11740b9bdf31a4'
BASE = 'f0a72a94b9badabc32f9ad028f913a98960e6b4a'
DRAFT = 'docs/justix-auto/state/drafts/architect/owner-migration-compatibility.md'
RESULT = 'docs/justix-auto/dev/results/owner-migration-compatibility.md'
AUDIT = Path('/private/tmp/justixauto-owner-migration-audit')
MOD = AUDIT / 'github.com/golang-migrate/migrate/v4@v4.20.1'
PGX = Path('/private/tmp/justixauto-t003-modcache/github.com/jackc/pgx/v5@v5.11.0')
checks = []

def git(*args):
    return subprocess.check_output(['git', '-C', str(ROOT), *args])

def obj(path):
    return git('show', SHA + ':' + path)

def check(name, condition):
    assert condition, name
    checks.append(name)

check('exact reviewed HEAD', git('rev-parse', 'HEAD').decode().strip() == SHA)
changed = git('diff', '--name-only', BASE, SHA).decode().splitlines()
check('only assigned proposal/result changed', set(changed) == {DRAFT, RESULT})
git('diff', '--check', BASE, SHA)
check('proposal bytes equal exact Git object', (ROOT / DRAFT).read_bytes() == obj(DRAFT))
check('result bytes equal exact Git object', (ROOT / RESULT).read_bytes() == obj(RESULT))
draft = obj(DRAFT).decode()
for path in (DRAFT, RESULT):
    for link in re.findall(r'\[[^]]*\]\(([^)]+)\)', obj(path).decode()):
        if '://' in link or link.startswith('#'):
            continue
        resolved = (ROOT / path).parent.joinpath(link.split('#')[0]).resolve().relative_to(ROOT)
        git('cat-file', '-e', SHA + ':' + str(resolved))
        check('link: ' + link, True)

tasks = json.loads(obj('docs/justix-auto/dev/task-index.json'))['tasks']
byid = {t['id']: t for t in tasks}
check('baseline has 928 distinct tasks', len(byid) == len(tasks) == 928)
rows = [line.split('|')[1:-1] for line in draft.splitlines() if line.startswith('| OM-')]
new = {}
for alias, deps, content in rows:
    new[alias.strip()] = {'deps': [x.strip() for x in deps.split(',')],
                         'files': re.findall(r'`([^`]+)`', content)}
check('six aliases, no numeric reservations', len(new) == 6 and all(a.startswith('OM-') for a in new))
graph = {k: set(t['depends_on']) for k, t in byid.items()}
graph.update({k: set(t['deps']) for k, t in new.items()})
amendments = {
    'T-059': {'OM-HISTORY', 'OM-IDENTITY'},
    'T-060': {'OM-IDENTITY'},
    'T-022': {'OM-RUNNER', 'OM-CUSTODY'},
    'T-580': {'OM-IDENTITY', 'OM-CUSTODY', 'OM-RUNNER'},
}
for key, edges in amendments.items():
    graph[key].update(edges)
visiting, visited = set(), set()
def visit(k):
    assert k in graph, 'unknown prerequisite: ' + k
    assert k not in visiting, 'cycle: ' + k
    if k in visited:
        return
    visiting.add(k)
    for dep in graph[k]:
        visit(dep)
    visiting.remove(k)
    visited.add(k)
for key in graph:
    visit(key)
check('hypothetical 934-node graph is acyclic', len(visited) == 934)
def ancestors(k):
    found = set()
    def add(n):
        for d in graph[n]:
            if d not in found:
                found.add(d)
                add(d)
    add(k)
    return found
check('all new aliases remain under B-01 closer T-591', set(new) <= ancestors('T-591'))
owners = {}
for t in tasks:
    for path in t['files_owned']:
        owners.setdefault(path, []).append(t['id'])
fresh = set()
for alias, t in new.items():
    for path in t['files']:
        for owner in owners.get(path, []):
            check('explicit serial ownership: '+alias+' after '+owner+' '+path, owner in ancestors(alias))
        if path not in owners:
            fresh.add(path)
        owners.setdefault(path, []).append(alias)
manifest = 'services/identity/migrations/0012_auth.manifest.json'
check('auth manifest unowned before explicit T-059 extension', manifest not in owners)
fresh.add(manifest)
check('13 distinct new leaves including T-059 manifest', len(fresh) == 13)
check('Identity UOW ownership explicit successor to integrated T-024',
      all(owners[p] == ['T-024', 'OM-IDENTITY'] for p in new['OM-IDENTITY']['files']))
check('custody checker serialization preserves profile ownership',
      all(owners[p] == ['OM-PROFILE', 'OM-CUSTODY'] for p in ['pkg/persistence/check.go', 'pkg/persistence/check_test.go']))
check('OM-IDENTITY and OM-CUSTODY disjoint', not (set(new['OM-IDENTITY']['files']) & set(new['OM-CUSTODY']['files'])))
check('T-061 inherits compatible Identity dependency', 'OM-IDENTITY' in ancestors('T-061'))

for owner in ['identity', 'inventory', 'commerce']:
    sql = obj('services/'+owner+'/migrations/0001_mechanics.up.sql').decode()
    uow = obj('services/'+owner+'/adapter/postgres/unit_of_work.go').decode()
    check(owner+' unchanged explicit transaction and dirty v1 guard',
          '\nBEGIN;' in sql and sql.endswith('COMMIT;\n') and 'version=1 AND dirty' in sql)
    check(owner+' legacy exact clean v1, check before bind',
          'bool_and(version=1 AND NOT dirty)' in uow and uow.index('if err := check(tx)') < uow.index('return bind(tx)'))
    check(owner+' existing SQL lacks fabricated historical artifact evidence',
          'owner_migrations' not in sql and 'sha256' not in sql.lower())
check('migrate insertion is still pending, pgx pin unchanged',
      b'github.com/golang-migrate/migrate/v4' not in obj('go.mod') and b'github.com/jackc/pgx/v5 v5.11.0' in obj('go.mod'))

def h1(entries):
    lines = ''.join(hashlib.sha256(data).hexdigest()+'  '+name+'\n' for name,data in sorted(entries))
    return 'h1:'+base64.b64encode(hashlib.sha256(lines.encode()).digest()).decode()
cache = AUDIT / 'cache/download/github.com/golang-migrate/migrate/v4/@v'
with zipfile.ZipFile(cache / 'v4.20.1.zip') as z:
    entries = [(n, z.read(n)) for n in z.namelist() if not n.endswith('/')]
    check('downloaded source equals approved archive bytes', all(
        (AUDIT/n).read_bytes() == data for n,data in entries))
    check('independently recomputed migrate module h1', h1(entries) == 'h1:2N/ToVTKrKl58ynBpgeVJ4In7VcLCjWTZtm4eP1LxhU=')
check('independently recomputed migrate go.mod h1', h1([('go.mod', (MOD/'go.mod').read_bytes())]) == 'h1:DDPgKVb4ovSWc4FwSPfV2Uz1160f4XBiTHTrAJtljmM=')
engine = (MOD/'migrate.go').read_text()
driver = (MOD/'database/driver.go').read_text()
upstream = (MOD/'database/pgx/v5/pgx.go').read_text()
pgconn = (PGX/'pgconn/pgconn.go').read_text()
check('NewWithInstance accepts public Driver directly', 'databaseInstance database.Driver' in engine)
check('Driver has Run and SetVersion hooks', 'Run(migration io.Reader) error' in driver and 'SetVersion(version int, dirty bool) error' in driver)
check('upstream connection is private and finalization owns separate tx',
      'conn     *sql.Conn' in upstream and 'p.conn.BeginTx(context.Background(), &sql.TxOptions{})' in upstream)
check('pgx supports complete simple-query batch and drained results',
      'func (pgConn *PgConn) Exec(ctx context.Context, sql string) *MultiResultReader' in pgconn and
      'func (mrr *MultiResultReader) ReadAll() ([]*Result, error)' in pgconn and
      'func (pgConn *PgConn) TxStatus() byte' in pgconn)
check('pgx Tx retains its owned connection', 'func (c *Conn) BeginTx(ctx context.Context, txOptions TxOptions) (Tx, error)' in (PGX/'tx.go').read_text()
      and 'conn         *Conn' in (PGX/'tx.go').read_text())
print(json.dumps({'reviewed_sha': SHA, 'base_sha': BASE,
    'proposal_sha256': hashlib.sha256(obj(DRAFT)).hexdigest(),
    'hypothetical_nodes': len(graph), 'new_aliases': new,
    'existing_amendments': {k:sorted(v) for k,v in amendments.items()},
    'new_leaves': sorted(fresh), 'checks_passed': len(checks), 'checks': checks,
    'api_source_sha256': {str(p):hashlib.sha256(p.read_bytes()).hexdigest() for p in
        [MOD/'migrate.go', MOD/'database/driver.go', MOD/'database/pgx/v5/pgx.go', PGX/'pgconn/pgconn.go', PGX/'tx.go']},
    'limitations': 'Proposal/static/API feasibility only. Engine probe uses an observed fake Driver; no live PostgreSQL or implemented driver guarantee.'}, indent=2))
