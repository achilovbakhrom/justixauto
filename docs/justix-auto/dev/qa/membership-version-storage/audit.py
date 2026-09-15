import hashlib
import json
import pathlib
import re
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[5]
OUT = pathlib.Path(__file__).resolve().parent
SHA = '5996c77e3df371a3992154b639ce9ba91ffc3e75'
BASE = 'd59faa8dc5b26e13e19c47686dd47744b6d73a11'
DRAFT = 'docs/justix-auto/state/drafts/architect/membership-version-storage.md'
RESULT = 'docs/justix-auto/dev/results/membership-version-storage.md'
def git(*args):
    return subprocess.check_output(['git','-C',str(ROOT),*args])
assert git('rev-parse','HEAD').decode().strip() == SHA
changed = git('diff','--name-only',BASE,SHA).decode().splitlines()
assert sorted(changed) == sorted([DRAFT, RESULT])
draft = git('show',SHA+':'+DRAFT).decode()
result = git('show',SHA+':'+RESULT).decode()
for path in changed:
    assert (ROOT/path).read_bytes() == git('show',SHA+':'+path)

aliases = {
 'MS-CATALOG': (['T-928'], ['pkg/eventstore/migrations/000007_membership_catalog.up.sql','pkg/eventstore/membership_catalog_migration_test.go']),
 'MS-SELECTION': (['MS-CATALOG'], ['pkg/eventstore/migrations/000008_membership_selection.up.sql','pkg/eventstore/membership_selection_migration_test.go']),
 'MS-IDENTITY': (['T-746','T-007'], ['pkg/inbox/membership_catalog.go','pkg/inbox/membership_catalog_test.go']),
 'MS-STATE': (['MS-SELECTION','MS-IDENTITY','T-014'], ['pkg/inbox/membership_state.go','pkg/inbox/membership_state_test.go']),
}
leaves = [p for _, paths in aliases.values() for p in paths]
assert len(leaves) == len(set(leaves)) == 8
assert 'at most4h' in draft
for alias,(deps,paths) in aliases.items():
    row = next(x for x in draft.splitlines() if x.startswith('| '+alias+' |'))
    assert all(x in row for x in deps+paths)
edges = {'T-920':['MS-STATE'],'T-921':['MS-STATE'],'T-016':['MS-STATE'],
         'T-022':['MS-SELECTION','MS-STATE'],'T-934':['MS-SELECTION']}

def check_graph(sha):
    index=json.loads(git('show',sha+':docs/justix-auto/dev/task-index.json'))
    tasks={t['id']:t for t in index['tasks']}
    for t in tasks.values():
        assert not set(leaves).intersection(t['files_owned']), t['id']
    graph={k:list(v['depends_on']) for k,v in tasks.items()}
    for k,(deps,_) in aliases.items(): graph[k]=list(deps)
    for k,deps in edges.items(): graph[k] += deps
    done=set()
    def visit(k,active):
        assert k in graph and k not in active, (k,active)
        if k in done:return
        for dep in graph[k]:visit(dep,active|{k})
        done.add(k)
    for k in graph:visit(k,set())
    def closure(k):
        out=set()
        for dep in graph[k]:out.add(dep);out.update(closure(dep))
        return out
    # Memoize closure to avoid repeated traversal of the large shared DAG.
    import functools
    closure=functools.lru_cache(None)(closure)
    assert set(aliases).issubset(closure('T-591'))
    for n in range(580,587):assert set(aliases).issubset(closure('T-'+str(n)))
    assert 'T-930' in closure('T-934')
    assert set(tasks['T-928']['files_owned']) == {'pkg/eventstore/migrations/000006_projection_generation.up.sql','pkg/eventstore/projection_generation_migration_test.go'}
    return {'sha':sha,'existing_tasks':len(tasks),'prospective_nodes':len(graph),'new_aliases':4,'new_leaves':8,'new_explicit_downstream_edges':sum(map(len,edges.values())), 'acyclic':True,'T591_and_all_owner_roots_cover_all_aliases':True}

main = git('rev-parse','refs/heads/main').decode().strip()
graphs=[check_graph(SHA),check_graph(main)]
blocks = re.findall(r'^```(?:go|python|text|json)\n(.*?)^```', result, re.M|re.S)
claimed = re.findall(r'\| (Temporary Go probe|Final Python SQL/fixture probe|Attempt1 log|Attempt2 log|Final log) \| `([0-9a-f]{64})`', result)
assert len(blocks)==len(claimed)==5
embedded=[]
for (name,want),block in zip(claimed,blocks):
    got=hashlib.sha256(block.encode()).hexdigest()
    assert got==want,(name,got,want)
    embedded.append({'name':name,'sha256':got,'recoverable_hash_match':True,'execution_not_inferred':True})
report={'reviewed_sha':SHA,'base':BASE,'changed':changed,'document_hashes':{p:hashlib.sha256(git('show',SHA+':'+p)).hexdigest() for p in changed},'graphs':graphs,'embedded_artifacts':embedded,
 'immutable_base_files':{p:hashlib.sha256(git('show',SHA+':'+p)).hexdigest() for p in git('ls-tree','-r','--name-only',SHA,'pkg/eventstore/migrations','pkg/projection/checkpoint.go','pkg/projection/sequence.go','pkg/eventstore/receiver_fence.go').decode().splitlines()}}
print(json.dumps(report,indent=2))
