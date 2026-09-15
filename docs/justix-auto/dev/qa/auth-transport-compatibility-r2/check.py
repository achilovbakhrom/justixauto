#!/usr/bin/env python3
"""Independent exact-proposal/ownership check; writes only a caller-owned temp Go module."""
import hashlib,json,pathlib,subprocess,tempfile
root=pathlib.Path(__file__).resolve().parents[5]
qa=pathlib.Path(__file__).parent
sha='914fcd78cc0789e41e9091efdc99fb5afc6c5c50'
old='9ad5a64422b04dbbc8bee969a47acdd3e9278d81'
base='476777139ffa6a470c7b3f6e79086c64f2dc7db5'
def git(*a): return subprocess.check_output(['git',*a],cwd=root).decode().strip()
def digest(p): return hashlib.sha256(p.read_bytes()).hexdigest()
assert git('rev-parse','HEAD')==sha
draft='docs/justix-auto/state/drafts/architect/auth-transport-compatibility.md'
result='docs/justix-auto/dev/results/auth-transport-compatibility.md'
assert set(git('diff','--name-only',base,sha).splitlines())=={draft,result}
assert digest(root/draft)=='e1643c8cf36ba5733f7bfa0d32e2285604b345fe72ed506e17248fbae9af3e0c'
assert git('show',old+':'+result).split('\n\n',1)[1] in (root/result).read_text()
source_hashes={p:digest(root/p) for p in ['web/packages/api/src/client.ts','tools/generate-contracts.mjs','docs/justix-auto/state/drafts/contracts/auth.md']}
for p,h in source_hashes.items(): assert hashlib.sha256(subprocess.check_output(['git','show',base+':'+p],cwd=root)).hexdigest()==h
doc=(root/draft).read_text()
tasks=json.loads((root/'docs/justix-auto/dev/task-index.json').read_text())['tasks']
graph={t['id']:list(t['depends_on']) for t in tasks}
aliases=[list(map(str.strip,l.split('|')[1:-1])) for l in doc.splitlines() if l.startswith('| AT-')]
assert len(aliases)==10
hours=0; owners={}; predecessor={}; estimated={}
import re
for name,deps,scope in aliases:
    graph[name]=deps.split(', ')
    estimate=int(re.match(r'(\d+)h;',scope)[1]);assert 0<estimate<=4
    estimated[name]=estimate;hours+=estimate
    leaves=re.findall(r'`([^`]+\.(?:ts|go|mjs|json))`',scope)
    assert leaves
    for leaf in leaves:
        if leaf in owners:
            prior=owners[leaf][-1]
            assert prior in graph[name] and 'serial successor of '+prior in scope
        else:
            existing=[t['id'] for t in tasks if leaf in t['files_owned']]
            if existing:
                assert 'serial successor' in scope
                predecessor[leaf]=existing
            else: assert not git('ls-files','--',leaf),leaf
        owners.setdefault(leaf,[]).append(name)
assert hours==36 and len(owners)==13
chain=['AT-GEN-PROFILE','AT-GEN-SEM','AT-GEN-TS','AT-GEN-GO','AT-GEN-RAW','AT-GEN-VERIFY']
assert owners['tools/generate-contracts.mjs']==chain
assert owners['tests/contracts/generation_reproducibility_test.go']==chain
for task,deps in [('T-641',['AT-GEN-VERIFY']),('T-055',['AT-EPOCH','AT-AUTH']),('T-060',['AT-AUTH'])]:graph[task]+=deps
visited=set();active=set()
def walk(t):
    assert t in graph and t not in active,t
    if t in visited:return
    active.add(t)
    for d in graph[t]:walk(d)
    active.remove(t);visited.add(t)
for t in graph:walk(t)
assert len(visited)==944
# Original independent BOUNCE/evidence are preserved in two separate checkouts.
main=pathlib.Path('/Users/bakhromachilov/startups/justixauto')
oldqa=main/'.worktrees/auth-transport-compatibility-qa'
oldreport='docs/justix-auto/dev/qa/auth-transport-compatibility.md'
originals=[oldreport]+[str(p.relative_to(oldqa)) for p in (oldqa/'docs/justix-auto/dev/qa/auth-transport-compatibility').iterdir() if p.is_file()]
assert len(originals)==5
preserved={p:digest(oldqa/p) for p in originals}
for p,h in preserved.items():assert digest(main/p)==h,p
print(json.dumps({'commit':sha,'proposal_sha256':digest(root/draft),'tasks':len(visited),'estimated_hours':hours,'estimates':estimated,'unique_leaves':len(owners),'serial_generator_chain':chain,'previous_owners':predecessor,'source_hashes':source_hashes,'preserved_original_artifacts':preserved,'result':'PASS'},indent=2))
