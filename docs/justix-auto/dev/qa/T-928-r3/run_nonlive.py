import json, os, pathlib, subprocess
root=pathlib.Path(__file__).resolve().parents[5]
out=pathlib.Path(__file__).resolve().parent
env=dict(os.environ)
for key in list(env):
    if key.startswith('JUSTIXAUTO_TEST_') or key.startswith('JUSTIXAUTO_T927_'):
        del env[key]
env.update(GOMODCACHE='/private/tmp/justixauto-t003-modcache',GOCACHE='/private/tmp/justixauto-integration-gocache',GOPROXY='off')
commands=[('nonlive-race.log',['bash','tools/go.sh','test','-race','-count=1','-mod=readonly','-v','-overlay=docs/justix-auto/dev/qa/T-928/overlay.json','./pkg/eventstore']),('vet.log',['bash','tools/go.sh','vet','./pkg/eventstore'])]
results=[]
for filename,command in commands:
    with (out/filename).open('wb') as log:
        p=subprocess.run(command,cwd=root,env=env,stdout=log,stderr=subprocess.STDOUT)
    results.append({'command':command,'log':filename,'exit_code':p.returncode})
print(json.dumps({'live_opt_ins':'all JUSTIXAUTO_TEST_* removed; no live PostgreSQL cases authorized or executed','results':results},indent=2))
raise SystemExit(0 if all(x['exit_code']==0 for x in results) else 1)
