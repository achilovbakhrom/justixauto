import os,pathlib,subprocess,sys
r=pathlib.Path.cwd();q=r/'docs/justix-auto/dev/qa/T-938'
env={k:v for k,v in os.environ.items() if not k.startswith('JUSTIXAUTO_TEST_')}
env.update(GOMODCACHE='/private/tmp/justixauto-t003-modcache',GOCACHE='/private/tmp/justixauto-integration-gocache',GOPROXY='off')
env['PATH']='/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/local/toolchains/node-24.21.0/bin:'+env.get('PATH','')
mode=sys.argv[1];cmd=['bash','tools/go.sh']
if mode=='scope':cmd+=['test','-race','-mod=readonly','-count=1','-v','./services/...','./pkg/...','./tests/...']
elif mode=='vet':cmd+=['vet','-mod=readonly','./services/...','./pkg/...','./tests/...']
elif mode=='independent':cmd+=['test','-race','-mod=readonly','-count=1','-v','-overlay',str(q/'overlay.json'),'-run','^TestQA938','./tests/contracts']
else:raise ValueError(mode)
with (q/(mode+'.log')).open('wb') as out:code=subprocess.run(cmd,env=env,stdout=out,stderr=subprocess.STDOUT).returncode
(q/(mode+'-exit.txt')).write_text(str(code)+'\n');sys.exit(code)
