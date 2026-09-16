import os,pathlib,subprocess,sys
root=pathlib.Path.cwd();qa=root/'docs/justix-auto/dev/qa/T-029'
env={k:v for k,v in os.environ.items() if not k.startswith('JUSTIXAUTO_TEST_')}
env.update(GOMODCACHE='/private/tmp/justixauto-t003-modcache',GOCACHE='/private/tmp/justixauto-integration-gocache',GOPROXY='off',JUSTIXAUTO_TEST_INSURANCE_MECHANICS='1')
env['PATH']='/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin:'+env.get('PATH','')
mode=sys.argv[1]
args=['bash','tools/go.sh']
if mode=='scope':args+=['test','-race','-mod=readonly','-count=1','-v','./services/...','./pkg/...','./tests/...']
elif mode=='vet':args+=['vet','-mod=readonly','./services/...','./pkg/...','./tests/...']
elif mode=='independent':args+=['test','-race','-mod=readonly','-count=1','-v','-overlay',str(qa/'overlay.json'),'-run','^TestQA029','./services/insurance/adapter/postgres']
elif mode=='migration':args+=['test','-race','-mod=readonly','-count=1','-v','-overlay',str(qa/'overlay.json'),'-run','^TestQA029MigrationGuardsBeforeMarker$','./services/insurance/adapter/postgres']
else:raise ValueError(mode)
with (qa/(mode+'.log')).open('wb') as out:
    code=subprocess.run(args,env=env,stdout=out,stderr=subprocess.STDOUT).returncode
(qa/(mode+'-exit.txt')).write_text(str(code)+'\n')
sys.exit(code)
