#!/usr/bin/env python3
"""Independent exact-commit generator probes; synthetic temporary files only."""
import hashlib, json, os, pathlib, re, subprocess, tempfile

ROOT = pathlib.Path(__file__).resolve().parents[5]
MAIN = pathlib.Path('/Users/bakhromachilov/startups/justixauto')
NODE = MAIN / 'docs/justix-auto/dev/local/toolchains/node-24.21.0/bin/node'
GEN = ROOT / 'tools/generate-contracts.mjs'
ENV = dict(os.environ, GOMODCACHE='/private/tmp/justixauto-t003-modcache', GOCACHE='/private/tmp/justixauto-integration-gocache', GOPROXY='off', GOTOOLCHAIN='local')
assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip() == '9ed232ebb5ffa110454e16f8c92d3cd8bec8162b'
source = (ROOT/'tests/contracts/generation_reproducibility_test.go').read_text()
fixture = json.loads(re.search(r'const generationFixture = `([\s\S]*?)`',source)[1])
run = pathlib.Path(tempfile.mkdtemp(prefix='justix-t640-independent-',dir='/private/tmp'))
results = []

def execute(args, cwd):
    p = subprocess.run([str(x) for x in args],cwd=cwd,env=ENV,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=120)
    return {'exit':p.returncode,'output':p.stdout}

def setup(name, document=None):
    d=run/name; d.mkdir()
    (d/'input.json').write_text(json.dumps(document or fixture))
    (d/'config.json').write_text(json.dumps({'version':1,'contracts':[{'input':'input.json','goOutput':'fixture.gen.go','tsOutput':'fixture.ts','goPackage':'synthetic','owner':'identity','namespace':'Fixture'}]}))
    return d

def generate(d,*args):
    return execute([NODE,GEN,'--config',d/'config.json',*args],d)

def compile_ts(d):
    (d/'client.ts').write_text((ROOT/'web/packages/api/src/client.ts').read_text())
    (d/'tsconfig.json').write_text(json.dumps({'compilerOptions':{'target':'ES2022','lib':['ES2022','DOM','DOM.Iterable'],'module':'Node16','moduleResolution':'Node16','strict':True,'types':[],'paths':{'@justixauto/api':['./client.ts']},'outDir':'out'},'include':['*.ts']}))
    return execute([NODE,MAIN/'node_modules/typescript/bin/tsc','-p',d/'tsconfig.json'],d)

# A dangling output symlink must be rejected, even though its target is absent.
d=setup('dangling-output')
target=run/'outside-config-created.ts'
(d/'fixture.ts').symlink_to(target)
r=generate(d)
results.append({'case':'dangling output symlink rejected without following','pass':r['exit']!=0 and not target.exists(),'target_created':target.exists(),'target':str(target),'result':r})

# Supported schema names must either generate compilable DTOs or fail before writes.
for name, language in [('ApiRequest','ts'),('ApiResult','ts'),('NewContractClient','go')]:
    doc=json.loads(json.dumps(fixture)); doc['components']['schemas'][name]={'type':'string'}
    d=setup('symbol-'+name,doc); gen=generate(d)
    if language=='ts': compiled=compile_ts(d) if gen['exit']==0 else None
    else:
        (d/'go.mod').write_text('module synthetic\n\ngo 1.27.1\n')
        compiled=execute(['bash',ROOT/'tools/go.sh','test','./...'],d) if gen['exit']==0 else None
    results.append({'case':'emitted symbol collision '+name,'pass':gen['exit']!=0 or compiled['exit']==0,'generation':gen,'compile':compiled})

# Runtime value adapter versus Go raw decoder, including map-key Unicode.
d=setup('runtime-values'); gen=generate(d); assert gen['exit']==0,gen
(d/'check.ts').write_text("""import {PayloadSchema,ChoiceSchema,DatesSchema} from './fixture.js';
const p={amountMinor:'1',revision:'1',nullable:null,count:1,tags:[],metadata:{}};
const cases:[string,boolean,unknown][]=[
 ['valid unicode map key',true,{...p,metadata:{'😀':'x'}}],
 ['unpaired surrogate map key',false,{...p,metadata:{'\\ud800':'x'}}],
 ['unknown nonenumerable property',false,Object.defineProperty({...p},'extra',{value:1})],
 ['unknown symbol property',false,{...p,[Symbol('extra')]:1}],
 ['fractional number',false,{...p,count:1.0001}],
 ['prototype map',false,{...p,metadata:Object.create({x:'x'})}],
 ['missing required',false,{amountMinor:'1',revision:'1',nullable:null,count:1,tags:[]}]
];
const results=cases.map(([name,expected,value])=>{let accepted=true;try{PayloadSchema.parse(value)}catch{accepted=false}return {name,expected,accepted,pass:expected===accepted}});
for(const [name,instant,expected] of [['leap date','2024-02-29T23:59:59.123456789Z',true],['nonleap date','2025-02-29T23:59:59Z',false],['hour overflow','2024-02-29T24:00:00Z',false]] as const){let accepted=true;try{DatesSchema.parse({date:'2024-02-29',instant})}catch{accepted=false}results.push({name,expected,accepted,pass:expected===accepted})}
console.log(JSON.stringify(results));
""")
compiled=compile_ts(d); assert compiled['exit']==0,compiled
runtime=execute([NODE,d/'out/check.js'],d)
results.append({'case':'TS value probes','checks':json.loads(runtime['output']),'result':runtime})
(d/'go.mod').write_text('module synthetic\n\ngo 1.27.1\n')
(d/'independent_test.go').write_text('''package synthetic
import("encoding/json";"testing")
func TestIndependentMapKeyUnicode(t *testing.T){for _,tt:=range []struct{key string;valid bool}{{`\\ud800`,false},{`\\ud83d\\ude00`,true}}{raw:=`{"amountMinor":"1","revision":"1","nullable":null,"count":1,"tags":[],"metadata":{"`+tt.key+`":"x"}}`;var v FixturePayload;err:=json.Unmarshal([]byte(raw),&v);if (err==nil)!=tt.valid{t.Fatalf("key %q accepted=%v",tt.key,err==nil)}}}
''')
results.append({'case':'Go map-key Unicode parity counterpart','result':execute(['bash',ROOT/'tools/go.sh','test','-race','-v','./...'],d)})

print(json.dumps({'sha':'9ed232ebb5ffa110454e16f8c92d3cd8bec8162b','temporary_root':str(run),'results':results},indent=2))
