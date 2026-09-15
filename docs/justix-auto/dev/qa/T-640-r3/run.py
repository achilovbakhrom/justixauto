#!/usr/bin/env python3
"""Exact-commit reruns; old evidence is read, never rewritten."""
import concurrent.futures, contextlib, hashlib, io, json, os, pathlib, subprocess

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[4]
MAIN = pathlib.Path('/Users/bakhromachilov/startups/justixauto')
SHA = 'c69d52221287abecc4f1803ec7ed3532dce070f9'
NODE = MAIN/'docs/justix-auto/dev/local/toolchains/node-24.21.0/bin/node'
ENV = dict(os.environ, GOMODCACHE='/private/tmp/justixauto-t003-modcache', GOCACHE='/private/tmp/justixauto-integration-gocache', GOPROXY='off', GOTOOLCHAIN='local')
ENV['PATH'] = str(NODE.parent)+os.pathsep+ENV['PATH']
assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip() == SHA
old = sorted(p for name in ['T-640','T-640-r2'] for p in [ROOT/f'docs/justix-auto/dev/qa/{name}.md', *(ROOT/f'docs/justix-auto/dev/qa/{name}').rglob('*')] if p.is_file())
before = {str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for p in old}
(HERE/'preimages.json').write_text(json.dumps(before,indent=2)+'\n')

def command(name, args):
    with (HERE/(name+'.txt')).open('w') as output:
        p = subprocess.run([str(x) for x in args],cwd=ROOT,env=ENV,stdout=output,stderr=subprocess.STDOUT,timeout=900)
    return {'name':name,'args':[str(x) for x in args],'exit':p.returncode}

commands = [('baseline',['bash','tools/go.sh','test','-race','-count=1','-v','./tests/contracts']),('vet',['bash','tools/go.sh','vet','./tests/contracts']),('syntax',[NODE,'--check','tools/generate-contracts.mjs'])]
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
    checks=list(pool.map(lambda item:command(*item),commands))
print(json.dumps(checks),flush=True)
(HERE/'commands.json').write_text(json.dumps(checks,indent=2)+'\n')

for name, previous in [('T-640','9ed232ebb5ffa110454e16f8c92d3cd8bec8162b'),('T-640-r2','14eb0c60895c9e47e829338c5bf78b1e873c9fe1')]:
    source=ROOT/f'docs/justix-auto/dev/qa/{name}/independent.py'
    code=source.read_text()
    assert previous in code
    code=code.replace(previous,SHA)
    output=io.StringIO()
    with contextlib.redirect_stdout(output):
        exec(compile(code,str(source),'exec'),{'__file__':str(source),'__name__':'__main__'})
    result=json.loads(output.getvalue())
    groups=[]
    for entry in result['results']:
        if 'pass' in entry: passed=entry['pass']
        elif 'checks' in entry: passed=entry['result']['exit']==0 and all(c['pass'] for c in entry['checks'])
        else: passed=entry['result']['exit']==0
        groups.append({'case':entry['case'],'pass':passed})
    result['exact_reviewed_sha']=SHA
    result['expectation_summary']={'passed':sum(g['pass'] for g in groups),'total':len(groups),'groups':groups}
    (HERE/(name+'-rerun.json')).write_text(json.dumps(result,indent=2)+'\n')
    print(name,result['expectation_summary'],flush=True)
    checks.append({'name':name+' expectations','exit':0 if all(g['pass'] for g in groups) else 1})

after={str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for p in old}
integrity={'sha':subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),'old_artifacts_unchanged':before==after,'old_artifacts':after,'tracked_diff':subprocess.run(['git','diff','--exit-code'],cwd=ROOT,capture_output=True,text=True).returncode,'whitespace':subprocess.run(['git','diff','--check'],cwd=ROOT,capture_output=True,text=True).returncode}
(HERE/'integrity.json').write_text(json.dumps(integrity,indent=2)+'\n')
assert integrity['sha']==SHA and integrity['old_artifacts_unchanged'] and integrity['tracked_diff']==0 and integrity['whitespace']==0
assert all(c['exit']==0 for c in checks), checks
