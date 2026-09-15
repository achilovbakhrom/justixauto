#!/usr/bin/env python3
"""Independent final-fix probes: emitted ports, lexical data and prewrite gates."""
import copy, hashlib, json, os, pathlib, re, subprocess, tempfile
ROOT=pathlib.Path(__file__).resolve().parents[5]
MAIN=pathlib.Path('/Users/bakhromachilov/startups/justixauto')
SHA='c69d52221287abecc4f1803ec7ed3532dce070f9'
NODE=MAIN/'docs/justix-auto/dev/local/toolchains/node-24.21.0/bin/node'
ENV=dict(os.environ,GOMODCACHE='/private/tmp/justixauto-t003-modcache',GOCACHE='/private/tmp/justixauto-integration-gocache',GOPROXY='off',GOTOOLCHAIN='local')
assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()==SHA
fixture=json.loads(re.search(r'const generationFixture = `([\s\S]*?)`',(ROOT/'tests/contracts/generation_reproducibility_test.go').read_text())[1])
run=pathlib.Path(tempfile.mkdtemp(prefix='justix-t640-r3-independent-',dir='/private/tmp'))
results=[]
def execute(args,cwd):
    p=subprocess.run([str(x) for x in args],cwd=cwd,env=ENV,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=180)
    return {'exit':p.returncode,'output':p.stdout}
def setup(name):
    d=run/name; d.mkdir()
    (d/'input.json').write_text(json.dumps(fixture))
    (d/'config.json').write_text(json.dumps({'version':1,'contracts':[{'input':'input.json','goOutput':'fixture.gen.go','tsOutput':'fixture.ts','goPackage':'synthetic','owner':'identity','namespace':'Fixture'}]}))
    return d
def generate(d,*args): return execute([NODE,ROOT/'tools/generate-contracts.mjs','--config',d/'config.json',*args],d)
def outputs(d): return {p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in d.glob('*.gen.go')}|{p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in d.glob('*.ts')}
def compile_ts(d):
    (d/'client.ts').write_text((ROOT/'web/packages/api/src/client.ts').read_text())
    (d/'tsconfig.json').write_text(json.dumps({'compilerOptions':{'target':'ES2022','lib':['ES2022','DOM','DOM.Iterable'],'module':'Node16','moduleResolution':'Node16','strict':True,'types':[],'paths':{'@justixauto/api':['./client.ts']},'outDir':'out'},'include':['*.ts']}))
    return execute([NODE,MAIN/'node_modules/typescript/bin/tsc','-p',d/'tsconfig.json'],d)

# Rejected later contract cannot repair earlier drift or create its own outputs.
d=setup('later-contract')
assert generate(d)['exit']==0
(d/'fixture.gen.go').write_text((d/'fixture.gen.go').read_text()+'\n// preserve this earlier drift\n')
config=json.loads((d/'config.json').read_text())
config['contracts'].append(dict(config['contracts'][0],input='second.json',goOutput='second.gen.go',tsOutput='second.ts',namespace='Second'))
(d/'config.json').write_text(json.dumps(config)); before=outputs(d)
names=['Error','Exchange','Owner','OperationID','Method','Path','Headers','Body','Security','Status','ContentType','Kind','UnknownOutcome','Type','In','Name','Value','Present','Status200','Status422','Token','UseNumber','More','SetString','IsInt','Num','IsInt64','Int64','String','MatchString','Set','Encode','ParseMediaType','PathEscape','NewDecoder','RuneCountInString']
for name in names:
    doc=copy.deepcopy(fixture)
    doc['paths']['/internal/v1/identity/synthetic/{id}']['get']['operationId']=name
    (d/'second.json').write_text(json.dumps(doc))
    for mode in [(),('--check',)]:
        actual=generate(d,*mode)
        results.append({'case':'later internal '+name+' '+str(mode),'pass':actual['exit']!=0 and outputs(d)==before and not (d/'second.gen.go').exists() and not (d/'second.ts').exists(),'result':actual})

# A schema literal is data even when it resembles selectors, comments or Go quotes.
d=setup('adapter-and-wire')
doc=copy.deepcopy(fixture)
doc['paths']['/internal/v1/identity/synthetic/{id}']['get']['operationId']='ReadReceipt'
values=['x.ReadReceipt(', 'strings.ReadReceipt(', 'ContractSecret', '// x.ReadReceipt(\nstrings.ReadReceipt(', '/* x.ReadReceipt( */', "'x.ReadReceipt('", '`x.ReadReceipt(`', '"x.ReadReceipt(\\"', 'x.ReadReceipt(\u2028end', '\\x.ReadReceipt(', '/* unterminated x.ReadReceipt(']
doc['components']['schemas']['WireText']={'type':'string','enum':values}
doc['components']['schemas']['WireObject']={'type':'object','additionalProperties':False,'required':['readReceipt','ContractLabel'],'properties':{'readReceipt':{'type':'string'},'ContractLabel':{'type':'string'}}}
(d/'input.json').write_text(json.dumps(doc))
gen=generate(d); results.append({'case':'selector-looking literals accepted','pass':gen['exit']==0,'result':gen})
if gen['exit']==0:
    before=outputs(d); check=generate(d,'--check')
    results.append({'case':'accepted adapter regeneration stable','pass':check['exit']==0 and outputs(d)==before,'result':check})
    ts_text=(d/'fixture.ts').read_text()
    results.append({'case':'internal method absent from browser output','pass':'/internal/v1/' not in ts_text and 'function ReadReceipt(' not in ts_text})
    (d/'go.mod').write_text('module synthetic\n\ngo 1.27.1\n')
    (d/'values.json').write_text(json.dumps(values))
    (d/'independent_test.go').write_text(r'''package synthetic
import("context";"encoding/json";"errors";"os";"reflect";"testing")
type independentExchange struct{ calls int; fail bool; t *testing.T }
var _ FixtureContractExchange=(*independentExchange)(nil)
var _ error=FixtureContractFailure{}
func(e *independentExchange) Exchange(_ context.Context,r FixtureContractRequest)(FixtureContractResponse,error){
 e.calls++
 if r.Owner!="identity"||r.Path!="/internal/v1/identity/synthetic/receipt"||r.OperationID!="ReadReceipt"||r.Method!="GET"||r.Headers["Accept"]!="application/json"||len(r.Body)!=0{e.t.Fatalf("adapter request fields changed: %+v",r)}
 if len(r.Security)!=1{e.t.Fatal("security alternatives lost")};s:=r.Security[0]["OwnerTLS"]
 if s.Type!="mutualTLS"||s.In!=""||s.Name!=""{e.t.Fatalf("security fields changed: %+v",s)}
 if e.fail{return FixtureContractResponse{},errors.New("private transport detail")}
 return FixtureContractResponse{Status:200,ContentType:"application/json",Body:[]byte(`{"amountMinor":"12345678901234567890123456789012345678","revision":"1","nullable":null,"count":1,"tags":[],"metadata":{}}`)},nil
}
func TestIndependentAdapter(t *testing.T){
 e:=&independentExchange{t:t};client,err:=FixtureNewContractClient(e);if err!=nil{t.Fatal(err)}
 result,err:=client.FixtureReadReceipt(context.Background(),FixtureReadReceiptParameters{Id:"receipt"});if err!=nil||result.Status!=200||result.Status200==nil||result.Status200.AmountMinor!="12345678901234567890123456789012345678"||e.calls!=1{t.Fatalf("adapter result: %+v %v",result,err)}
 e.fail=true;_,err=client.FixtureReadReceipt(context.Background(),FixtureReadReceiptParameters{Id:"receipt"});f,ok:=err.(FixtureContractFailure);if !ok||f.Kind!="transport error"||f.UnknownOutcome||f.Error()!="transport error"||e.calls!=2{t.Fatalf("error contract or retry changed: %v",err)}
 optional:=FixtureContractOptional[*string]{Value:nil,Present:true};raw,err:=json.Marshal(optional);if err!=nil||string(raw)!="null"||optional.IsZero(){t.Fatal("optional fields changed")}
 var decoded FixtureContractOptional[*string];if err=json.Unmarshal(raw,&decoded);err!=nil||!decoded.Present||decoded.Value!=nil{t.Fatal("optional decode changed")}
}
func TestIndependentWireData(t *testing.T){
 raw,err:=os.ReadFile("values.json");if err!=nil{t.Fatal(err)};var values []string;if err=json.Unmarshal(raw,&values);err!=nil{t.Fatal(err)}
 for _,value:=range values{wire,_:=json.Marshal(value);var dto FixtureWireText;if err=json.Unmarshal(wire,&dto);err!=nil{t.Fatal(value,err)};again,err:=json.Marshal(dto);if err!=nil||string(again)!=string(wire){t.Fatal("literal rewritten",value,err)}}
 input:=[]byte(`{"readReceipt":"wire-key","ContractLabel":"kept"}`);var dto FixtureWireObject;if err=json.Unmarshal(input,&dto);err!=nil{t.Fatal(err)};output,err:=json.Marshal(dto);if err!=nil{t.Fatal(err)};var a,b map[string]string;_ = json.Unmarshal(input,&a);_ = json.Unmarshal(output,&b);if !reflect.DeepEqual(a,b){t.Fatalf("wire tags changed: %s",output)}
}
''')
    go=execute(['bash',ROOT/'tools/go.sh','test','-race','-count=1','-v','./...'],d)
    vet=execute(['bash',ROOT/'tools/go.sh','vet','./...'],d)
    results.append({'case':'independent generated Go adapter and wire execution','pass':go['exit']==0 and vet['exit']==0,'go':go,'vet':vet})
    (d/'independent.ts').write_text("import {WireTextSchema,WireObjectSchema} from './fixture.js';\nconst values="+json.dumps(values)+";\nfor(const value of values) {if(WireTextSchema.parse(value)!==value||WireTextSchema.parse(JSON.parse(JSON.stringify(value)))!==value) throw new Error('wire literal changed');}\nconst raw={readReceipt:'wire-key',ContractLabel:'kept'};const value=WireObjectSchema.parse(raw);if(JSON.stringify(value)!==JSON.stringify(raw)) throw new Error('wire keys changed');\nconsole.log(JSON.stringify({wire_values:values.length,wire_keys:Object.keys(value),pass:true}));\n")
    ts=compile_ts(d); runtime=execute([NODE,d/'out/independent.js'],d) if ts['exit']==0 else None
    results.append({'case':'independent generated TS wire execution','pass':ts['exit']==0 and runtime['exit']==0,'compile':ts,'runtime':runtime})

print(json.dumps({'sha':SHA,'temporary_root':str(run),'results':results,'passed':sum(r['pass'] for r in results),'total':len(results)},indent=2))
raise SystemExit(0 if all(r['pass'] for r in results) else 1)
