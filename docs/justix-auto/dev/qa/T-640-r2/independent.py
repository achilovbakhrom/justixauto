#!/usr/bin/env python3
"""Independent fix-cycle probes; all generated files live in a synthetic temp root."""
import copy, hashlib, json, os, pathlib, re, subprocess, tempfile

ROOT = pathlib.Path(__file__).resolve().parents[5]
MAIN = pathlib.Path('/Users/bakhromachilov/startups/justixauto')
NODE = MAIN/'docs/justix-auto/dev/local/toolchains/node-24.21.0/bin/node'
SHA = '14eb0c60895c9e47e829338c5bf78b1e873c9fe1'
ENV = dict(os.environ, GOMODCACHE='/private/tmp/justixauto-t003-modcache', GOCACHE='/private/tmp/justixauto-integration-gocache', GOPROXY='off', GOTOOLCHAIN='local')
assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip() == SHA
fixture=json.loads(re.search(r'const generationFixture = `([\s\S]*?)`',(ROOT/'tests/contracts/generation_reproducibility_test.go').read_text())[1])
run=pathlib.Path(tempfile.mkdtemp(prefix='justix-t640-r2-independent-',dir='/private/tmp'))
results=[]
def execute(args,cwd):
    p=subprocess.run([str(x) for x in args],cwd=cwd,env=ENV,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=120)
    return {'exit':p.returncode,'output':p.stdout}
def setup(name):
    d=run/name; d.mkdir()
    (d/'input.json').write_text(json.dumps(fixture))
    (d/'config.json').write_text(json.dumps({'version':1,'contracts':[{'input':'input.json','goOutput':'fixture.gen.go','tsOutput':'fixture.ts','goPackage':'synthetic','owner':'identity','namespace':'Fixture'}]}))
    return d
def generate(d,*args): return execute([NODE,ROOT/'tools/generate-contracts.mjs','--config',d/'config.json',*args],d)
def compile_go(d):
    (d/'go.mod').write_text('module synthetic\n\ngo 1.27.1\n')
    return execute(['bash',ROOT/'tools/go.sh','test','-race','-v','./...'],d)
def compile_ts(d):
    (d/'client.ts').write_text((ROOT/'web/packages/api/src/client.ts').read_text())
    (d/'tsconfig.json').write_text(json.dumps({'compilerOptions':{'target':'ES2022','lib':['ES2022','DOM','DOM.Iterable'],'module':'Node16','moduleResolution':'Node16','strict':True,'types':[],'paths':{'@justixauto/api':['./client.ts']},'outDir':'out'},'include':['*.ts']}))
    return execute([NODE,MAIN/'node_modules/typescript/bin/tsc','-p',d/'tsconfig.json'],d)
def outputs(d): return {p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in d.glob('fixture.*') if not p.is_symlink()}

for leaf in ['fixture.ts','fixture.gen.go','missing-parent']:
    d=setup('dangling-'+leaf); assert generate(d)['exit']==0
    (d/'fixture.gen.go').write_text((d/'fixture.gen.go').read_text()+'\n// preserved drift\n')
    target=run/('absent-target-'+leaf)
    if leaf=='missing-parent':
        config=json.loads((d/'config.json').read_text()); config['contracts'][0]['tsOutput']='missing-parent/result.ts'; (d/'config.json').write_text(json.dumps(config))
    else: (d/leaf).unlink()
    (d/leaf).symlink_to(target); before=outputs(d)
    for mode in [('--check',),()]:
        actual=generate(d,*mode)
        results.append({'case':'dangling '+leaf+' '+str(mode),'pass':actual['exit']!=0 and not target.exists() and (d/leaf).is_symlink() and os.readlink(d/leaf)==str(target) and outputs(d)==before,'result':actual})

for kind,names in [('schema',['ApiRequest','ApiResult','NewContractClient','Context','Marshal','Unmarshal','MarshalJSON','UnmarshalJSON','IsZero','ContractFailure','PayloadSchema']),('public-operation',['ApiRequest','ApiResult','Context','MarshalJSON','UnmarshalJSON','IsZero','Error']),('internal-operation',['Error','MarshalJSON','UnmarshalJSON','IsZero','Context','Exchange','ReadReceipt'])]:
    for name in names:
        d=setup(kind+'-'+name); assert generate(d)['exit']==0; before=outputs(d); doc=copy.deepcopy(fixture)
        if kind=='schema': doc['components']['schemas'][name]={'type':'string'}
        elif kind=='public-operation': doc['paths']['/api/v1/identity/synthetic/{id}']['get']['operationId']=name
        else: doc['paths']['/internal/v1/identity/synthetic/{id}']['get']['operationId']=name
        (d/'input.json').write_text(json.dumps(doc)); gen=generate(d)
        go=compile_go(d) if gen['exit']==0 else None; ts=compile_ts(d) if gen['exit']==0 else None
        passed=(outputs(d)==before) if gen['exit']!=0 else (go['exit']==0 and ts['exit']==0)
        results.append({'case':kind+' '+name,'pass':passed,'generation':gen,'go':go,'ts':ts,'outputs_unchanged':outputs(d)==before})

d=setup('unicode'); assert generate(d)['exit']==0
(d/'check.ts').write_text(r'''import {PayloadSchema} from './fixture.js';
const p={amountMinor:'1',revision:'1',nullable:null,count:1,tags:[],metadata:{}};
const cases:[string,boolean][]=[['\ud800',false],['\udfff',false],['prefix\ud800suffix',false],['\ud800\ud800',false],['\udfff\ud800',false],['\ud83d\ude00',true],['😀',true],['\ufffd',true],['',true],['__proto__',true]];
const results=cases.map(([key,expected])=>{let accepted=true;let exact=false;try{const value=PayloadSchema.parse({...p,metadata:{[key]:'x'}});const again=PayloadSchema.parse(JSON.parse(JSON.stringify(value)));exact=Object.keys(value.metadata).length===1&&Object.hasOwn(value.metadata,key)&&value.metadata[key]==='x'&&Object.hasOwn(again.metadata,key)&&again.metadata[key]==='x'}catch{accepted=false}return {key,expected,accepted,exact,pass:accepted===expected&&(!expected||exact)}});
console.log(JSON.stringify(results));if(results.some(r=>!r.pass))process.exitCode=1;
'''.replace('if(results.some(r=>!r.pass))process.exitCode=1;','if(results.some(r=>!r.pass))throw new Error("parity failure");'))
ts=compile_ts(d); runtime=execute([NODE,d/'out/check.js'],d) if ts['exit']==0 else None
results.append({'case':'TS typed map Unicode exact round trips','pass':ts['exit']==0 and runtime['exit']==0,'compile':ts,'runtime':runtime})
(d/'independent_test.go').write_text(r'''package synthetic
import("encoding/json";"testing")
func TestIndependentUnicodeKeys(t *testing.T){for _,tt:=range []struct{key string;valid bool}{{`\ud800`,false},{`\udfff`,false},{`prefix\ud800suffix`,false},{`\ud800\ud800`,false},{`\udfff\ud800`,false},{`\ud83d\ude00`,true},{`😀`,true},{`\ufffd`,true},{``,true},{`__proto__`,true}}{raw:=`{"amountMinor":"1","revision":"1","nullable":null,"count":1,"tags":[],"metadata":{"`+tt.key+`":"x"}}`;var v FixturePayload;err:=json.Unmarshal([]byte(raw),&v);if (err==nil)!=tt.valid{t.Fatalf("key %q accepted=%v",tt.key,err==nil)};if tt.valid{var key string;if err:=json.Unmarshal([]byte(`"`+tt.key+`"`),&key);err!=nil{t.Fatal(err)};if len(v.Metadata)!=1||v.Metadata[key]!="x"{t.Fatal("key changed")};encoded,err:=json.Marshal(v);if err!=nil{t.Fatal(err)};var again FixturePayload;if err=json.Unmarshal(encoded,&again);err!=nil||again.Metadata[key]!="x"{t.Fatal("round trip changed")}}}}
''')
go=compile_go(d); vet=execute(['bash',ROOT/'tools/go.sh','vet','./...'],d)
results.append({'case':'Go raw map Unicode exact round trips','pass':go['exit']==0 and vet['exit']==0,'go':go,'vet':vet})
print(json.dumps({'sha':SHA,'temporary_root':str(run),'results':results,'passed':sum(r['pass'] for r in results),'total':len(results)},indent=2))
