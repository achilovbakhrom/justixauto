package contracts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Explicit synthetic contracts only. This test does not activate an owner route
// or assert that a pending feature's OpenAPI/security policy has been approved.
const generationFixture = `{
 "openapi":"3.1.0","info":{"title":"Synthetic generation fixture","version":"1"},
 "components":{"securitySchemes":{"OwnerTLS":{"type":"mutualTLS"},"SessionCookie":{"type":"apiKey","in":"cookie","name":"__Host-justix_session"}},"schemas":{
  "Payload":{"type":"object","additionalProperties":false,"required":["amountMinor","nullable","revision","tags","metadata","count"],"properties":{
   "amountMinor":{"type":"string","pattern":"^(0|[1-9][0-9]*)(?![\\s\\S])","minLength":1,"maxLength":38},
   "revision":{"type":"string","pattern":"^(0|[1-9][0-9]*)(?![\\s\\S])","maxLength":19},
   "nullable":{"type":["string","null"]},"note":{"type":["string","null"]},
   "count":{"type":"integer","minimum":0,"maximum":100},
   "tags":{"type":"array","items":{"type":"string"},"uniqueItems":true},
   "metadata":{"type":"object","additionalProperties":{"type":"string"}}
  }},
  "Ready":{"type":"object","additionalProperties":false,"required":["kind","data"],"properties":{"kind":{"type":"string","const":"ready"},"data":{"$ref":"#/components/schemas/Payload"}}},
  "Pending":{"type":"object","additionalProperties":false,"required":["kind","operationId"],"properties":{"kind":{"type":"string","const":"pending"},"operationId":{"type":"string","format":"uuid"}}},
  "Choice":{"oneOf":[{"$ref":"#/components/schemas/Ready"},{"$ref":"#/components/schemas/Pending"}]},
  "Maybe":{"type":["string","null"]},
  "LooseEnd":{"type":"string","pattern":"^x$"},
  "Dates":{"type":"object","additionalProperties":false,"required":["date","instant"],"properties":{"date":{"type":"string","format":"date"},"instant":{"type":"string","format":"date-time"}}},
  "Problem":{"type":"object","additionalProperties":false,"required":["error"],"properties":{"error":{"type":"object","additionalProperties":false,"required":["code","message","fields","traceId"],"properties":{"code":{"type":"string","minLength":1},"message":{"type":"string"},"fields":{"type":"object","additionalProperties":{"type":"string"}},"traceId":{"type":"string"}}}}}
 }},
 "paths":{
  "/internal/v1/identity/synthetic/{id}":{"get":{"operationId":"ReadInternal","security":[{"OwnerTLS":[]}],"parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"Internal synthetic receipt","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Payload"}}}}}}},
  "/api/v1/identity/synthetic/{id}":{
   "post":{"operationId":"CreateSynthetic","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string","minLength":1}},{"name":"filter","in":"query","schema":{"type":"string"}},{"name":"If-Match","in":"header","required":true,"schema":{"type":"string"}}],"requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Payload"}}}},"responses":{"200":{"description":"Synthetic receipt","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Payload"}}}},"201":{"description":"Same synthetic receipt","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Payload"}}}},"422":{"description":"Synthetic validation","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Problem"}}}}}},
   "get":{"operationId":"ReadSynthetic","security":[{"SessionCookie":[]}],"parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"Synthetic union","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Choice"}}}}}},
   "delete":{"operationId":"DeleteSynthetic","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"204":{"description":"Synthetic empty"}}}
  }
 }
}`

type generationHarness struct {
	t               *testing.T
	root, dir, node string
	main            string
}

func newGenerationHarness(t *testing.T) generationHarness {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "../.."))
	common, err := exec.Command("git", "-C", root, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		t.Fatal(err)
	}
	main := filepath.Dir(strings.TrimSpace(string(common)))
	node := filepath.Join(main, "docs/justix-auto/dev/local/toolchains/node-24.21.0/bin/node")
	if _, err := os.Stat(node); err != nil {
		node, err = exec.LookPath("node")
		if err != nil {
			t.Fatal("install the approved Node 24.21.0 toolchain before contract checks")
		}
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := generationHarness{t: t, root: root, main: main, dir: dir, node: node}
	if version := h.run(true, node, "--version"); strings.TrimSpace(version) != "v24.21.0" {
		t.Fatalf("approved Node version required, got %s", version)
	}
	h.write("input.json", generationFixture)
	h.config(false)
	return h
}

func (h generationHarness) write(path, content string) {
	h.t.Helper()
	full := filepath.Join(h.dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0600); err != nil {
		h.t.Fatal(err)
	}
}

func (h generationHarness) config(two bool) {
	h.t.Helper()
	entries := []map[string]string{{"input": "input.json", "goOutput": "fixture.gen.go", "tsOutput": "fixture.ts", "goPackage": "synthetic", "owner": "identity", "namespace": "Fixture"}}
	if two {
		entries = append(entries, map[string]string{"input": "input.json", "goOutput": "second.gen.go", "tsOutput": "second.ts", "goPackage": "synthetic", "owner": "identity", "namespace": "Second"})
	}
	data, err := json.Marshal(map[string]any{"version": 1, "contracts": entries})
	if err != nil {
		h.t.Fatal(err)
	}
	h.write("config.json", string(data))
}

func (h generationHarness) run(pass bool, program string, args ...string) string {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, program, args...)
	command.Dir = h.dir
	command.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOPROXY=off", "GOFLAGS=-mod=readonly")
	output, err := command.CombinedOutput()
	if (err == nil) != pass {
		h.t.Fatalf("%s %v: %v\n%s", program, args, err, output)
	}
	return string(output)
}

func (h generationHarness) generate(pass bool, extra ...string) string {
	h.t.Helper()
	args := []string{filepath.Join(h.root, "tools/generate-contracts.mjs"), "--config", filepath.Join(h.dir, "config.json")}
	return h.run(pass, h.node, append(args, extra...)...)
}

func (h generationHarness) read(path string) []byte {
	h.t.Helper()
	value, err := os.ReadFile(filepath.Join(h.dir, path))
	if err != nil {
		h.t.Fatal(err)
	}
	return value
}

// Exercise the actual private compiler in a temporary module, with only its
// unconditional CLI entry point removed. The shipping tool exposes no parser
// export, activation flag or alternate auth command. Fixtures remain synthetic.
func TestContractGenerationInertAuthProfile(t *testing.T) {
	h := newGenerationHarness(t)
	source, err := os.ReadFile(filepath.Join(h.root, "tools/generate-contracts.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	entry := "\ntry { main(); } catch (error)"
	if strings.Count(string(source), entry) != 1 {
		t.Fatal("generator CLI entry point changed; review inert probe boundary")
	}
	rootJSON, _ := json.Marshal(runtime.GOROOT())
	h.write("inert-profile.mjs", string(source[:strings.LastIndex(string(source), entry)])+"\ncachedGoRoot="+string(rootJSON)+";\n"+inertAuthProfileChecks)
	output := h.run(true, h.node, filepath.Join(h.dir, "inert-profile.mjs"), filepath.Join(h.dir, "input.json"))
	t.Log(strings.TrimSpace(output))
	// The inert probe writes the full valid matrix only into its owned temp dir.
	// Verify the unchanged public CLI rejects this complete model before output,
	// including when a previous non-auth entry would otherwise be regenerated.
	h.config(true)
	h.generate(true)
	before := map[string]string{}
	for _, name := range []string{"fixture.gen.go", "fixture.ts", "second.gen.go", "second.ts"} {
		before[name] = string(h.read(name))
	}
	config := `{"version":1,"contracts":[{"input":"input.json","goOutput":"fixture.gen.go","tsOutput":"fixture.ts","goPackage":"synthetic","owner":"identity","namespace":"Fixture"},{"input":"auth.json","goOutput":"second.gen.go","tsOutput":"second.ts","goPackage":"synthetic","owner":"identity","namespace":"Second"}]}`
	h.write("config.json", config)
	h.write("input.json", strings.Replace(generationFixture, "Synthetic generation fixture", "Earlier valid entry would drift", 1))
	for _, extra := range [][]string{nil, {"--check"}, {"--auth"}, {"--enable-auth"}} {
		output := h.generate(false, extra...)
		if len(extra) == 0 || extra[0] == "--check" {
			if !strings.Contains(output, "auth generation is not activated") {
				t.Fatalf("complete auth profile did not reach fixed activation barrier: %s", output)
			}
		}
		for name, content := range before {
			if string(h.read(name)) != content {
				t.Fatalf("auth rejection modified earlier/existing output %s", name)
			}
		}
	}
	// Config/environment candidates cannot bypass the unconditional barrier.
	t.Setenv("JUSTIX_AUTH_GENERATION", "1")
	t.Setenv("JUSTIX_CONTRACTS_AUTH", "1")
	if output := h.generate(false); !strings.Contains(output, "auth generation is not activated") {
		t.Fatal(output)
	}
	h.write("config.json", strings.Replace(config, `"version":1`, `"version":1,"auth":true`, 1))
	h.generate(false)
	for name, content := range before {
		if string(h.read(name)) != content {
			t.Fatalf("activation candidate modified %s", name)
		}
	}
	// Absent output paths are not created either. A non-auth first plan cannot
	// leave a partial bundle when a later auth profile is rejected.
	h.write("config.json", strings.ReplaceAll(config, "fixture.", "absent-first."))
	h.generate(false)
	for _, name := range []string{"absent-first.gen.go", "absent-first.ts"} {
		if _, err := os.Stat(filepath.Join(h.dir, name)); !os.IsNotExist(err) {
			t.Fatalf("auth rejection created %s", name)
		}
	}
}

// Compile actual inert semantic fragments in isolated modules. No shipping
// export, CLI switch, schema registration or transport activation is added.
func TestContractGenerationAuthSemantics(t *testing.T) {
	h := newGenerationHarness(t)
	source, err := os.ReadFile(filepath.Join(h.root, "tools/generate-contracts.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	entry := "\ntry { main(); } catch (error)"
	if strings.Count(string(source), entry) != 1 {
		t.Fatal("generator entry changed")
	}
	rootJSON, _ := json.Marshal(runtime.GOROOT())
	fixtureEnd := strings.Index(inertAuthProfileChecks, "\nlet count=0;")
	if fixtureEnd < 0 {
		t.Fatal("inert fixture boundary changed")
	}
	h.write("semantic-emitter.mjs", string(source[:strings.LastIndex(string(source), entry)])+"\ncachedGoRoot="+string(rootJSON)+";\n"+inertAuthProfileChecks[:fixtureEnd]+authSemanticEmitterChecks)
	h.write("semantic.test.ts", authSemanticTSChecks)
	h.write("semantic_test.go", authSemanticGoChecks)
	h.write("go.mod", "module synthetic\n\ngo 1.27.1\n")
	t.Log(strings.TrimSpace(h.run(true, h.node, filepath.Join(h.dir, "semantic-emitter.mjs"), filepath.Join(h.dir, "input.json"))))
	h.write("tsconfig.json", `{"compilerOptions":{"strict":true,"exactOptionalPropertyTypes":true,"noUncheckedIndexedAccess":true,"target":"ES2022","module":"CommonJS","moduleResolution":"node","ignoreDeprecations":"6.0","skipLibCheck":true,"outDir":"dist","types":[]},"include":["semantic.ts","bindings.ts","semantic.test.ts","negative.ts"]}`)
	t.Log(strings.TrimSpace(h.run(true, h.node, filepath.Join(h.main, "node_modules/typescript/bin/tsc"), "--project", "tsconfig.json")))
	t.Log(strings.TrimSpace(h.run(true, h.node, "dist/semantic.test.js")))
	t.Log(strings.TrimSpace(h.run(true, filepath.Join(runtime.GOROOT(), "bin/go"), "test", "-race", "-count=1", "-v", "./...")))
	t.Log(strings.TrimSpace(h.run(true, filepath.Join(runtime.GOROOT(), "bin/go"), "vet", "./...")))
}

const authSemanticEmitterChecks = `
const semanticDocument=()=>JSON.parse(JSON.stringify(authDocument()).replaceAll('"Body"','"Envelope"').replaceAll('/Body"','/Envelope"'));
const d=semanticDocument();
const ref=(name)=>({$ref:'#/components/schemas/'+name});
const closed=(properties)=>({type:'object',additionalProperties:false,required:Object.keys(properties),properties});
Object.assign(d.components.schemas,{
 Leaf:{type:'string',minLength:1}, Other:{type:'integer',minimum:0,maximum:10},
 Left:closed({kind:{type:'string',const:'left'},value:ref('Leaf')}),
 Right:closed({kind:{type:'string',const:'right'},value:ref('Other')}),
 Choice:{oneOf:[ref('Left'),ref('Right')]}, Alias:ref('Choice'),
 Twin:{type:'string',minLength:1}, Ambiguous:{oneOf:[ref('Leaf'),ref('Twin')]},
 Maybe:{type:['string','null']},
 Envelope:closed({a:ref('Alias'),items:{type:'array',items:ref('Leaf')},map:{type:'object',additionalProperties:ref('Leaf')},z:{type:'boolean'}}),
 Problem:closed({detail:ref('Leaf')}), OtherProblem:closed({code:ref('Other')})
});
for(const item of Object.values(d.paths))for(const op of Object.values(item))op.responses[429].content['application/json'].schema=ref('OtherProblem');
const c=compile(configEntry,d),names=authSemanticModel(c).names;
const ts=renderAuthValidatorsTS(c),go=renderAuthValidatorsGo(c);
const validEnvelope={a:{kind:'left',value:'x'},items:['i','j'],map:{b:'b',a:'a'},z:true};
const parity=[{name:'valid',schema:'Envelope',value:validEnvelope},{name:'bad parent',schema:'Envelope',value:{...validEnvelope,z:'wrong'}},{name:'ambiguity',schema:'Ambiguous',value:'x'},
 {name:'nullable',schema:'Maybe',value:null},{name:'second branch',schema:'Choice',value:{kind:'right',value:2}},
 {name:'error',schema:'Problem',value:{detail:'x'},error:{operation:'AuthOperation0',status:401}},
 {name:'second error',schema:'OtherProblem',value:{code:2},error:{operation:'AuthOperation0',status:429}},
 {name:'wrong status schema',schema:'Problem',value:{code:2},error:{operation:'AuthOperation0',status:401}},
 {name:'undeclared status',schema:'Problem',value:{detail:'x'},error:{operation:'AuthOperation0',status:200}}];
for(const hook of ['ready','Leaf','Choice','Alias','Envelope','error'])for(const outcome of ['mismatch','fatal:policy','fatal:configuration','fatal:binding'])parity.push({name:hook+':'+outcome,schema:'Envelope',value:hook==='error'?{detail:'x'}:validEnvelope,hook,outcome,...(hook==='error'?{error:{operation:'AuthOperation0',status:401}}:{})});
writeFileSync(resolve('corpus.json'),JSON.stringify(parity));
assert.throws(()=>renderTS(c),/not activated/);assert.throws(()=>renderGo(c),/not activated/);
assert(!ts.includes('fetch(')&&!go.includes('ContractExchange'),'fragment must not contain transport');
assert.equal(renderAuthValidatorsTS(compile(configEntry,JSON.parse(stable(d)))),ts);
assert.equal(renderAuthValidatorsGo(compile(configEntry,JSON.parse(stable(d)))),go);
writeFileSync(resolve('semantic.ts'),ts+'\nexport function breakConfiguration(){delete contractSchemas.Right;}\n');
writeFileSync(resolve('semantic.gen.go'),go);
writeFileSync(resolve('bindings.ts'),'import type {AuthSemantics, SemanticOutcome} from "./semantic";\nexport const names='+JSON.stringify(names)+' as const;\nexport function bindings(hook:(name:string,value?:unknown)=>SemanticOutcome):AuthSemantics{return {checkReady:()=>hook("ready"),validateError:(op,status,value)=>hook("error:"+op+":"+status,value),'+names.map((name)=>'validate'+name+':value=>hook('+JSON.stringify(name)+',value)').join(',')+'};}\n');
let bindingGo='package synthetic\n\ntype probeBinding struct {call func(string,any)FixtureSemanticOutcome}\nfunc(b *probeBinding)CheckReady()FixtureSemanticOutcome{return b.call("ready",nil)}\nfunc(b *probeBinding)ValidateError(op string,status int,value FixtureAuthErrorResponse)FixtureSemanticOutcome{return b.call("error",value)}\n';
for(const name of names)bindingGo+='func(b *probeBinding)Validate'+name+'(value Fixture'+name+')FixtureSemanticOutcome{return b.call('+JSON.stringify(name)+',value)}\n';
// A nil slice/map/function can implement an interface without a nil pointer.
for(const [kind,type]of [['Slice','[]int'],['Map','map[string]int'],['Function','func()']]){
 bindingGo+='type nil'+kind+' '+type+'\nfunc(nil'+kind+')CheckReady()FixtureSemanticOutcome{panic("nil receiver must not be invoked")}\nfunc(nil'+kind+')ValidateError(string,int,FixtureAuthErrorResponse)FixtureSemanticOutcome{panic("nil receiver must not be invoked")}\n';
 for(const name of names)bindingGo+='func(nil'+kind+')Validate'+name+'(Fixture'+name+')FixtureSemanticOutcome{panic("nil receiver must not be invoked")}\n';
}
writeFileSync(resolve('bindings_test.go'),bindingGo);
let negative='import type {AuthSemantics,SemanticOutcome} from "./semantic";\n';
for(const method of ['checkReady','validateEnvelope','validateError'])for(const value of ['async()=>"valid"','()=>Promise.resolve("valid")','()=>({then(){}})','()=>true','()=>{}','()=>({kind:"valid"})'])negative+='// @ts-expect-error only closed synchronous primitives are accepted\nconst invalid'+negative.length+':AuthSemantics['+JSON.stringify(method)+']='+value+';\n';
negative+='// @ts-expect-error arbitrary strings are not outcomes\nconst invalidOutcome:SemanticOutcome="anything";\n';
writeFileSync(resolve('negative.ts'),negative);
for(const collision of ['AuthSemantics','SemanticOutcome','ValidateError','Variant1','ParseLeaf','Outcome','LeafStructuralSchema']){
 const bad=structuredClone(d);bad.components.schemas[collision]={type:'string'};
 assert.throws(()=>renderAuthValidatorsTS(compile(configEntry,bad)),/collision/);
 assert.throws(()=>renderAuthValidatorsGo(compile(configEntry,bad)),/collision/);
}
// A single-error primitive alias also compiles in Go.
for(const directory of ['single']){
 const alternative=semanticDocument();alternative.components.schemas.Problem={type:'string'};
 const compiled=compile(configEntry,alternative);mkdirSync(resolve(directory),{recursive:true});
 writeFileSync(resolve(directory,'semantic.gen.go'),renderAuthValidatorsGo(compiled));
}
console.log('actual Go/TS semantic fragments, deterministic emission, closed types, collision and activation guards emitted');
`

const authSemanticTSChecks = `
import * as s from './semantic';
import {bindings,names} from './bindings';
declare function require(name:string):any;
declare const process:{on(name:string,fn:(value:unknown)=>void):void};
const assert=require('node:assert/strict');
const vm=require('node:vm');
const tick=()=>new Promise<void>(resolve=>require('node:timers').setImmediate(resolve));
const unhandled:unknown[]=[];process.on('unhandledRejection',value=>unhandled.push(value));
let log:string[]=[],outcome:s.SemanticOutcome='valid',stop='',ready=0,failReady=0;
const factory=()=>bindings((name)=>{log.push(name);if(name==='ready'){ready++;if(ready===failReady)return 'fatal:policy';}return name===stop?outcome:'valid';});
const valid={a:{kind:'left',value:'x'},items:['i','j'],map:{b:'b',a:'a'},z:true};
const hooks=['Leaf','Left','Choice','Alias','Leaf','Leaf','Leaf','Leaf','Envelope'];
function reset(){log=[];ready=0;failReady=0;stop='';outcome='valid';}
function expect(run:()=>unknown,want:s.SemanticOutcome){assert.throws(run,(e:unknown)=>e instanceof s.AuthValidationError&&e.outcome===want&&e.message==='Invalid auth validation');}
function noHooks(){assert.deepEqual(log.filter(name=>name!=='ready'),[]);}
async function main(){
 reset();const table=factory(),v=s.createAuthValidators(table);assert.equal(ready,1);assert(Object.isFrozen(v));
 reset();assert.equal(v.EnvelopeSchema.parse(valid),valid);assert.deepEqual(log.filter(x=>x!=='ready'),hooks);assert.equal(ready,1+hooks.length);
 reset();s.EnvelopeStructuralSchema.parse(valid);noHooks();assert.equal(ready,0);
 for(const input of [{...valid,z:'wrong'},{...valid,items:['ok',4]},{...valid,a:{kind:'unknown',value:'x'}},{...valid,extra:1}]){reset();expect(()=>v.EnvelopeSchema.parse(input),'mismatch');noHooks();assert.equal(ready,1);}
 reset();expect(()=>v.AmbiguousSchema.parse('yes'),'mismatch');noHooks();
 reset();assert.equal(v.MaybeSchema.parse(null),null);assert.deepEqual(log,['ready','ready','Maybe']);
 // Structural data fields named then/constructor are ordinary JSON keys.
 reset();v.EnvelopeSchema.parse({...valid,map:{then:'data',constructor:'data'}});
 for(const result of ['mismatch','fatal:policy','fatal:configuration','fatal:binding'] as const){reset();stop='Leaf';outcome=result;expect(()=>v.EnvelopeSchema.parse(valid),result);assert.deepEqual(log.filter(x=>x!=='ready'),['Leaf']);}
 reset();failReady=1;expect(()=>s.createAuthValidators(factory()),'fatal:policy');noHooks();
 reset();failReady=2;expect(()=>v.EnvelopeSchema.parse(valid),'fatal:policy');noHooks();
 reset();failReady=3;expect(()=>v.EnvelopeSchema.parse(valid),'fatal:policy');assert.deepEqual(log.filter(x=>x!=='ready'),['Leaf']);
 reset();stop='ready';outcome='mismatch';expect(()=>v.EnvelopeSchema.parse(valid),'fatal:binding');noHooks();
 reset();v.validateError('AuthOperation0',401,{detail:'x'});assert.deepEqual(log.filter(x=>x!=='ready'),['Leaf','Problem','error:AuthOperation0:401']);
 reset();v.validateError('AuthOperation0',429,{code:2});assert.deepEqual(log.filter(x=>x!=='ready'),['Other','OtherProblem','error:AuthOperation0:429']);
 reset();expect(()=>v.validateError('AuthOperation0',401,{code:2}),'mismatch');noHooks();
 reset();expect(()=>v.validateError('AuthOperation0',200 as s.AuthErrorStatus,{detail:'x'}),'fatal:configuration');noHooks();
 reset();expect(()=>v.validateError('Unknown' as s.AuthOperationID,401,{detail:'x'}),'fatal:configuration');noHooks();
 reset();table.validateEnvelope=()=>{throw Error('later mutation');};v.EnvelopeSchema.parse(valid);
 // Own data functions only; accessors are never read, no missing/default hook.
 for(const name of ['checkReady','validateEnvelope','validateError']){
  reset();let getters=0;const accessor=factory();Object.defineProperty(accessor,name,{get(){getters++;return()=> 'valid';}});expect(()=>s.createAuthValidators(accessor),'fatal:binding');assert.equal(getters,0);noHooks();
  const missing=factory() as unknown as Record<string,unknown>;delete missing[name];expect(()=>s.createAuthValidators(missing as unknown as s.AuthSemantics),'fatal:binding');
 }
 reset();expect(()=>s.createAuthValidators(Object.create(factory()) as s.AuthSemantics),'fatal:binding');noHooks();
 let arbitraryReads=0;const arbitrary={get then(){arbitraryReads++;throw Error('then getter');},get kind(){arbitraryReads++;throw Error('kind getter');}};
 const callable=Object.assign(()=> 'valid',{then(){arbitraryReads++;}});
 const badValues:unknown[]=[undefined,null,true,false,0,1,'other',{},arbitrary,{then(){arbitraryReads++;}},callable,new String('valid')];
 for(const field of ['checkReady','validateLeaf','validateError'])for(const bad of badValues){
  reset();const b=factory();(b as unknown as Record<string,unknown>)[field]=()=>bad;
  if(field==='checkReady')expect(()=>s.createAuthValidators(b),'fatal:binding');else{const validators=s.createAuthValidators(b);expect(()=>field==='validateError'?validators.validateError('AuthOperation0',401,{detail:'x'}):validators.EnvelopeSchema.parse(valid),'fatal:binding');}
 }
 assert.equal(arbitraryReads,0);
 for(const field of ['checkReady','validateLeaf','validateError'])for(const thrown of [false,true])for(const cross of [false,true])for(const rejected of [false,true]){
  reset();const b=factory();(b as unknown as Record<string,unknown>)[field]=()=>{const p=cross?vm.runInNewContext(rejected?'Promise.reject("secret")':'Promise.resolve("valid")'):rejected?Promise.reject('secret'):Promise.resolve('valid');if(thrown)throw p;return p;};
  if(field==='checkReady')expect(()=>s.createAuthValidators(b),'fatal:binding');else{const validators=s.createAuthValidators(b);expect(()=>field==='validateError'?validators.validateError('AuthOperation0',401,{detail:'x'}):validators.EnvelopeSchema.parse(valid),'fatal:binding');}
 }
 for(const field of ['checkReady','validateLeaf','validateError'])for(const rejected of [false,true]){reset();const b=factory();(b as unknown as Record<string,unknown>)[field]=async()=>{if(rejected)throw Error('async secret');return 'valid';};if(field==='checkReady')expect(()=>s.createAuthValidators(b),'fatal:binding');else{const validators=s.createAuthValidators(b);expect(()=>field==='validateError'?validators.validateError('AuthOperation0',401,{detail:'x'}):validators.EnvelopeSchema.parse(valid),'fatal:binding');}}
 // A rejected native Promise mistakenly supplied as a data field is observed.
 for(const field of ['checkReady','validateEnvelope','validateError']){reset();const b=factory();(b as unknown as Record<string,unknown>)[field]=vm.runInNewContext('Promise.reject("field secret")');expect(()=>s.createAuthValidators(b),'fatal:binding');noHooks();}
 reset();const badProxy=new Proxy(valid,{getPrototypeOf(){throw Promise.reject('structure secret');}});expect(()=>v.EnvelopeSchema.parse(badProxy),'fatal:configuration');noHooks();
 reset();let branches=0;const branchFault=new Proxy({kind:'left',value:'x'},{getPrototypeOf(){if(++branches===2)throw Promise.reject('unselected branch secret');return Object.prototype;}});expect(()=>v.ChoiceSchema.parse(branchFault),'fatal:configuration');noHooks();assert.equal(branches,2);
 const corpus=JSON.parse(require('node:fs').readFileSync('corpus.json','utf8')) as {name:string;schema:string;value:unknown;hook?:string;outcome?:s.SemanticOutcome;error?:{operation:s.AuthOperationID;status:s.AuthErrorStatus}}[];
 const parity=corpus.map(row=>{const calls:string[]=[];let active=false;const validators=s.createAuthValidators(bindings(name=>{if(active)calls.push(name.startsWith('error:')?'error':name);return active&&(name===row.hook||name.startsWith('error:')&&row.hook==='error')?row.outcome!:'valid';}));active=true;let result:s.SemanticOutcome='valid';try{if(row.error)validators.validateError(row.error.operation,row.error.status,row.value);else (validators as unknown as Record<string,{parse(value:unknown):unknown}>)[row.schema+'Schema']!.parse(row.value);}catch(error){assert(error instanceof s.AuthValidationError);result=(error as s.AuthValidationError).outcome;}return {name:row.name,outcome:result,calls};});
 require('node:fs').writeFileSync('ts-parity.json',JSON.stringify(parity));assert.equal(parity.length,33);
 reset();s.breakConfiguration();expect(()=>v.ChoiceSchema.parse({kind:'left',value:'x'}),'fatal:configuration');noHooks();
 await tick();await tick();assert.deepEqual(unhandled,[]);assert.equal(arbitraryReads,0);assert(names.length>10);
 console.log('TS semantic validation: whole-tree/order/readiness/outcomes/capture/error bindings/native Promise disposal PASS');
}
void main().catch((error)=>{require('node:process').exitCode=1;console.error(error);});
`

const authSemanticGoChecks = `package synthetic

import("encoding/json";"os";"reflect";"testing")

const validEnvelope = "{\"a\":{\"kind\":\"left\",\"value\":\"x\"},\"items\":[\"i\",\"j\"],\"map\":{\"b\":\"b\",\"a\":\"a\"},\"z\":true}"
func assertOutcome(t *testing.T,err error,want FixtureSemanticOutcome){t.Helper();got,ok:=err.(FixtureAuthValidationError);if !ok||got.Outcome!=want||err.Error()!="invalid auth validation"{t.Fatalf("outcome %v, want %v",err,want)}}
func TestAuthSemantics(t *testing.T){
 var log []string;ready,failReady:=0,0;stop:="";outcome:=FixtureSemanticValid
 reset:=func(){log=nil;ready=0;failReady=0;stop="";outcome=FixtureSemanticValid}
 named:=func()[]string{result:=[]string{};for _,name:=range log{if name!="ready"{result=append(result,name)}};return result}
 equal:=func(want []string){t.Helper();if !reflect.DeepEqual(named(),want){t.Fatalf("hooks %v, want %v",named(),want)}}
 b:=&probeBinding{call:func(name string,value any)FixtureSemanticOutcome{log=append(log,name);if name=="ready"{ready++;if ready==failReady{return FixtureSemanticFatalPolicy}};if name==stop{return outcome};return FixtureSemanticValid}}
 v,err:=FixtureNewAuthValidators(b);if err!=nil||ready!=1{t.Fatal(err,ready)}
 reset();if _,err=v.ParseEnvelope([]byte(validEnvelope));err!=nil{t.Fatal(err)};equal([]string{"Leaf","Left","Choice","Alias","Leaf","Leaf","Leaf","Leaf","Envelope"});if ready!=10{t.Fatal(ready)}
 reset();var structural FixtureEnvelope;if err=json.Unmarshal([]byte(validEnvelope),&structural);err!=nil{t.Fatal(err)};equal([]string{});if ready!=0{t.Fatal(ready)}
 for _,bad:=range []string{"{\"a\":{\"kind\":\"left\",\"value\":\"x\"},\"items\":[\"i\",4],\"map\":{},\"z\":true}","{\"a\":{\"kind\":\"left\",\"value\":\"x\"},\"items\":[],\"map\":{},\"z\":\"wrong\"}","{\"a\":{\"kind\":\"unknown\",\"value\":\"x\"},\"items\":[],\"map\":{},\"z\":true}"}{reset();_,err=v.ParseEnvelope([]byte(bad));assertOutcome(t,err,FixtureSemanticMismatch);equal([]string{});if ready!=1{t.Fatal(ready)}}
 reset();_,err=v.ParseAmbiguous([]byte("\"yes\""));assertOutcome(t,err,FixtureSemanticMismatch);equal([]string{})
 reset();if _,err=v.ParseMaybe([]byte("null"));err!=nil{t.Fatal(err)};equal([]string{"Maybe"})
 for _,result:=range []FixtureSemanticOutcome{FixtureSemanticMismatch,FixtureSemanticFatalPolicy,FixtureSemanticFatalConfiguration,FixtureSemanticFatalBinding,0,6,255}{reset();stop="Leaf";outcome=result;_,err=v.ParseEnvelope([]byte(validEnvelope));want:=result;if result<1||result>5{want=FixtureSemanticFatalBinding};assertOutcome(t,err,want);equal([]string{"Leaf"})}
 reset();failReady=1;_,err=FixtureNewAuthValidators(b);assertOutcome(t,err,FixtureSemanticFatalPolicy);equal([]string{})
 reset();failReady=2;_,err=v.ParseEnvelope([]byte(validEnvelope));assertOutcome(t,err,FixtureSemanticFatalPolicy);equal([]string{})
 reset();failReady=3;_,err=v.ParseEnvelope([]byte(validEnvelope));assertOutcome(t,err,FixtureSemanticFatalPolicy);equal([]string{"Leaf"})
 reset();stop="ready";outcome=FixtureSemanticMismatch;_,err=v.ParseEnvelope([]byte(validEnvelope));assertOutcome(t,err,FixtureSemanticFatalBinding);equal([]string{})
 reset();receipt,err:=v.ParseError("AuthOperation0",401,[]byte("{\"detail\":\"x\"}"));if err!=nil||receipt.Variant1!=nil||receipt.Variant2==nil{t.Fatal(receipt,err)};equal([]string{"Leaf","Problem","error"})
 reset();if err=v.ValidateError("AuthOperation0",401,receipt);err!=nil{t.Fatal(err)};equal([]string{"Leaf","Problem","error"})
 for _,value:=range []FixtureAuthErrorResponse{{},{Variant1:&FixtureOtherProblem{}},{Variant1:&FixtureOtherProblem{},Variant2:receipt.Variant2}}{reset();assertOutcome(t,v.ValidateError("AuthOperation0",401,value),FixtureSemanticMismatch);equal([]string{})}
 reset();_,err=v.ParseError("AuthOperation0",401,[]byte("{\"code\":2}"));assertOutcome(t,err,FixtureSemanticMismatch);equal([]string{})
 reset();_,err=v.ParseError("AuthOperation0",200,[]byte("{}"));assertOutcome(t,err,FixtureSemanticFatalConfiguration);equal([]string{})
 reset();_,err=v.ParseError("Unknown",401,[]byte("{}"));assertOutcome(t,err,FixtureSemanticFatalConfiguration);equal([]string{})
 for _,binding:=range []FixtureAuthSemantics{nil,(*probeBinding)(nil),nilSlice(nil),nilMap(nil),nilFunction(nil)}{_,err=FixtureNewAuthValidators(binding);assertOutcome(t,err,FixtureSemanticFatalBinding)}
 for _,field:=range []string{"ready","Leaf","error"}{for _,unknown:=range []FixtureSemanticOutcome{0,6,255}{reset();stop=field;outcome=unknown;if field=="ready"{_,err=FixtureNewAuthValidators(b)}else{_,err=v.ParseError("AuthOperation0",401,[]byte("{\"detail\":\"x\"}"))};assertOutcome(t,err,FixtureSemanticFatalBinding)}}
 for _,field:=range []string{"ready","Leaf","error"}{reset();b.call=func(name string,_ any)FixtureSemanticOutcome{if name==field{panic("secret")};return FixtureSemanticValid};if field=="ready"{_,err=FixtureNewAuthValidators(b)}else{_,err=v.ParseError("AuthOperation0",401,[]byte("{\"detail\":\"x\"}"))};assertOutcome(t,err,FixtureSemanticFatalBinding)}
 var absent *FixtureAuthValidators;_,err=absent.ParseEnvelope([]byte(validEnvelope));assertOutcome(t,err,FixtureSemanticFatalBinding)
 b.call=func(name string,_ any)FixtureSemanticOutcome{log=append(log,name);return FixtureSemanticValid};reset()
 original:=fixtureContractSchemas["Right"];delete(fixtureContractSchemas,"Right");_,err=v.ParseChoice([]byte("{\"kind\":\"left\",\"value\":\"x\"}"));assertOutcome(t,err,FixtureSemanticFatalConfiguration);equal([]string{});fixtureContractSchemas["Right"]=original
 // Malformed unselected structure must remain fatal even when the first
 // branch matches. Update the test-only fingerprint to reach the walker.
 fingerprint:=fixtureContractAuthExpectedSchemas;fixtureContractSchemas["Right"]=map[string]any{"type":"object","additionalProperties":false,"properties":true};fixtureContractAuthExpectedSchemas,_=json.Marshal(fixtureContractSchemas)
 reset();_,err=v.ParseChoice([]byte("{\"kind\":\"left\",\"value\":\"x\"}"));assertOutcome(t,err,FixtureSemanticFatalConfiguration);equal([]string{});fixtureContractSchemas["Right"]=original;fixtureContractAuthExpectedSchemas=fingerprint
 // All typed conversion/builders are prepared before any named callback.
 builder:=v.builders["Envelope"];v.builders["Envelope"]=func(any)(func()FixtureSemanticOutcome,error){panic("conversion secret")};reset();_,err=v.ParseEnvelope([]byte(validEnvelope));assertOutcome(t,err,FixtureSemanticFatalConfiguration);equal([]string{});v.builders["Envelope"]=builder
}

func TestAuthSemanticCrossLanguageParity(t *testing.T){
 var corpus []struct{Name,Schema,Hook,Outcome string;Value json.RawMessage;Error *struct{Operation string;Status int}}
 data,err:=os.ReadFile("corpus.json");if err!=nil{t.Fatal(err)};if err=json.Unmarshal(data,&corpus);err!=nil{t.Fatal(err)}
 outcomes:=map[string]FixtureSemanticOutcome{"valid":FixtureSemanticValid,"mismatch":FixtureSemanticMismatch,"fatal:policy":FixtureSemanticFatalPolicy,"fatal:configuration":FixtureSemanticFatalConfiguration,"fatal:binding":FixtureSemanticFatalBinding}
 var actual []map[string]any
 for _,row:=range corpus{
  calls:=[]string{};active:=false;binding:=&probeBinding{call:func(name string,_ any)FixtureSemanticOutcome{if active{calls=append(calls,name);if name==row.Hook{return outcomes[row.Outcome]}};return FixtureSemanticValid}}
  v,err:=FixtureNewAuthValidators(binding);if err!=nil{t.Fatal(err)};active=true
  if row.Error!=nil{_,err=v.ParseError(row.Error.Operation,row.Error.Status,row.Value)}else{switch row.Schema{case "Envelope":_,err=v.ParseEnvelope(row.Value);case "Ambiguous":_,err=v.ParseAmbiguous(row.Value);case "Maybe":_,err=v.ParseMaybe(row.Value);case "Choice":_,err=v.ParseChoice(row.Value);default:t.Fatal(row.Schema)}}
  outcome:="valid";if err!=nil{typed,ok:=err.(FixtureAuthValidationError);if !ok{t.Fatal(err)};for name,value:=range outcomes{if typed.Outcome==value{outcome=name}};if outcome=="valid"{t.Fatal("unknown outcome",err)}}
  actual=append(actual,map[string]any{"name":row.Name,"outcome":outcome,"calls":calls})
 }
 encoded,_:=json.Marshal(actual);var actualJSON,expectedJSON any;if err=json.Unmarshal(encoded,&actualJSON);err!=nil{t.Fatal(err)}
 expected,err:=os.ReadFile("ts-parity.json");if err!=nil{t.Fatal(err)};if err=json.Unmarshal(expected,&expectedJSON);err!=nil{t.Fatal(err)}
 if !reflect.DeepEqual(actualJSON,expectedJSON){t.Fatalf("Go/TS parity mismatch\nGo: %s\nTS: %s",encoded,expected)};t.Logf("%d identical outcome and readiness/hook traces",len(corpus))
}
`

const inertAuthProfileChecks = `
const assert=(await import('node:assert/strict')).default;
const fixture=readJSON(process.argv[2]);
const configEntry={input:'auth.json',goOutput:'auth.gen.go',tsOutput:'auth.ts',goPackage:'synthetic',owner:'identity',namespace:'Fixture'};
const tokenSchema={type:'string',pattern:'^[A-Za-z0-9_-]{1,4096}$'};
const header=(schema)=>({required:true,description:'Synthetic transport declaration',schema:structuredClone(schema)});
const csrf=()=>header(tokenSchema);
const cache=()=>header({type:'string',const:'no-store'});
const retry=()=>header({type:'integer',minimum:0,maximum:2147483647});
const requestHeader=()=>({name:'X-CSRF-Token',in:'header',...csrf()});
const routes=[
 ['get','/session',200,true],['post','/session/login',200,true],
 ['post','/session/mfa/challenges',200,false],['post','/session/mfa/verify',200,true],
 ['post','/session/logout',204,false],['post','/session/revoke-all',200,true],
 ['post','/session/mfa/enrollment',200,false],['post','/session/mfa/enrollment/{id}/confirm',200,true],
 ['post','/session/recovery/request',202,false],['post','/session/recovery/complete',204,false],
];
const response=(status,rotation=false)=>({description:'Synthetic status',headers:{'Cache-Control':cache(),...(rotation?{'X-CSRF-Token':csrf()}:{}),...(status===429?{'Retry-After':retry()}:{})},
 ...(status===204?{}:{content:{'application/json':{schema:{$ref:'#/components/schemas/'+(status<400?'Body':'Problem')}}}})});
function authDocument(){
 const d={openapi:'3.1.0',info:{title:'Synthetic auth declarations',version:'1'},'x-justix-auth-semantics':1,
 components:{schemas:{Body:{type:'object',properties:{},required:[],additionalProperties:false},Problem:{type:'object',properties:{},required:[],additionalProperties:false}},
 securitySchemes:{SessionCookie:{type:'apiKey',in:'cookie',name:'__Host-justix_session'},ChallengeCookie:{type:'apiKey',in:'cookie',name:'__Host-justix_challenge'},CSRFCookie:{type:'apiKey',in:'cookie',name:'__Host-justix_csrf'},OwnerTLS:{type:'mutualTLS'}}},paths:{}};
 routes.forEach(([method,path,status,rotation],index)=>{
  const parameters=path.includes('{id}')?[{name:'id',in:'path',required:true,schema:{type:'string',format:'uuid'}}]:[];
  if(method!=='get')parameters.push(requestHeader());
  d.paths['/api/v1/identity'+path]={[method]:{operationId:'AuthOperation'+index,'x-justix-auth-transport':1,parameters,
   security:method==='get'?[{}, {SessionCookie:[]},{ChallengeCookie:[]}]:[{CSRFCookie:[]}],
   ...(method==='get'?{}:{requestBody:{required:true,content:{'application/json':{schema:{$ref:'#/components/schemas/Body'}}}}}),
   responses:{[status]:response(status,rotation),401:response(401,method==='get'),403:response(403),429:response(429)}}};
 });
 return d;
}
let count=0;
const rows=[];
function pass(name,run){run();count++;rows.push(name);}
function reject(name,change,entry=configEntry){pass(name,()=>{const d=authDocument();change(d);assert.throws(()=>compile(entry,d));});}
const login=(d)=>d.paths['/api/v1/identity/session/login'].post;
const read=(d)=>d.paths['/api/v1/identity/session'].get;
const d=authDocument(),plan=compile(configEntry,d);
pass('closed version and immutable auth profile',()=>{assert.deepEqual(plan.authProfile,{version:1,semantics:1});assert(Object.isFrozen(plan.authProfile));assert.equal(plan.operations.length,10);});
for(const [method,path,status,rotation] of routes)pass('matrix '+method+' '+path,()=>{
 const op=plan.operations.find((op)=>op.path==='/api/v1/identity'+path&&op.method===method.toUpperCase());
 assert.deepEqual(op.statuses,[status]);assert.equal(op.authTransport.version,1);assert.equal(op.authTransport.semantics,1);
 assert.equal(op.authTransport.metadata[status].csrf,rotation?'required':'forbidden');
 assert.equal(op.authTransport.metadata[401].csrf,method==='get'?'required':'forbidden');
 assert.equal(op.authTransport.metadata[403].csrf,'forbidden');
 assert.equal(op.authTransport.metadata[429].retryAfter,'required');
 assert.deepEqual(op.authTransport.metadata[429].retryAfterWire,{pattern:'^(0|[1-9][0-9]{0,9})$',maximum:2147483647});
 assert.equal(op.authTransport.metadata[429].csrf,'forbidden');
 assert.equal(op.authTransport.requestCSRF===null,method==='get');
 if(method!=='get')assert.deepEqual(op.authTransport.requestCSRF,requestHeader());
 assert(!op.parameters.some((p)=>p.name==='X-CSRF-Token'));
 assert(!Object.hasOwn(plan.schemas[op.paramName].properties,'X-CSRF-Token'));
 assert(!plan.schemas[op.paramName].required.includes('X-CSRF-Token'));
 assert.deepEqual(op.securityRequirements,method==='get'?[{}, {SessionCookie:d.components.securitySchemes.SessionCookie},{ChallengeCookie:d.components.securitySchemes.ChallengeCookie}]:[{CSRFCookie:d.components.securitySchemes.CSRFCookie}]);
 assert(Object.isFrozen(op.authTransport));assert(Object.isFrozen(op.authTransport.metadata));
 assert(Object.isFrozen(op.authTransport.metadata[status].headers['Cache-Control'].schema));
});
pass('ordinary path parameters remain in parameter DTO',()=>{const op=plan.operations.find((op)=>op.path.includes('{id}'));assert.deepEqual(op.parameters.map((p)=>p.name),['id']);assert.deepEqual(plan.schemas[op.paramName].required,['id']);});
pass('captured header declarations cannot be changed by source mutation',()=>{login(d).responses[200].headers['X-CSRF-Token'].schema.pattern='anything';const op=plan.operations.find((op)=>op.path.endsWith('/login'));assert.equal(op.authTransport.metadata[200].headers['X-CSRF-Token'].schema.pattern,tokenSchema.pattern);assert.throws(()=>{op.authTransport.requestCSRF.schema.pattern='anything';});});
pass('auth security declarations are captured and deeply immutable',()=>{d.components.securitySchemes.CSRFCookie.name='mutated';const op=plan.operations.find((op)=>op.path.endsWith('/login'));assert.equal(op.securityRequirements[0].CSRFCookie.name,'__Host-justix_csrf');assert(Object.isFrozen(op.securityRequirements));assert(Object.isFrozen(op.securityRequirements[0].CSRFCookie));assert.throws(()=>{op.securityRequirements[0].CSRFCookie.name='mutated';});});
pass('both renderer entry points remain blocked',()=>{assert.throws(()=>renderTS(plan),/not activated/);assert.throws(()=>renderGo(plan),/not activated/);});
pass('declaration order does not change the internal model',()=>{const original=authDocument();const reordered=JSON.parse(stable(original));assert.equal(stable(compile(configEntry,original)),stable(compile(configEntry,reordered)));});
for(const value of [0,2,'1',true,null,{},[]]){
 reject('unsupported semantic version '+JSON.stringify(value),(d)=>{d['x-justix-auth-semantics']=value;});
 reject('unsupported operation version '+JSON.stringify(value),(d)=>{login(d)['x-justix-auth-transport']=value;});
}
reject('missing semantic marker',(d)=>{delete d['x-justix-auth-semantics'];});
reject('missing operation marker',(d)=>{delete login(d)['x-justix-auth-transport'];});
reject('semantics without an auth operation',(d)=>{d.paths=structuredClone(fixture.paths);d.components=structuredClone(fixture.components);});
reject('non Identity owner',(d)=>{}, {...configEntry,owner:'retail'});
reject('auth marker on an internal route',(d)=>{d.paths['/internal/v1/identity/session/login']={post:login(d)};delete d.paths['/api/v1/identity/session/login'];});
reject('unknown auth route',(d)=>{d.paths['/api/v1/identity/session/new-action']={post:login(d)};delete d.paths['/api/v1/identity/session/login'];});
reject('wrong auth method',(d)=>{d.paths['/api/v1/identity/session/login']={put:login(d)};});
reject('missing anonymous 401',(d)=>{delete read(d).responses[401];});
reject('extra success status',(d)=>{login(d).responses[201]=response(201);});
reject('wrong success status',(d)=>{login(d).responses[201]=response(201);delete login(d).responses[200];});
reject('unsupported error status',(d)=>{login(d).responses[500]=response(500);});
reject('noncanonical status key',(d)=>{login(d).responses['0429']=response(429);});
reject('default response',(d)=>{login(d).responses.default=response(503);});
reject('204 content',(d)=>{d.paths['/api/v1/identity/session/logout'].post.responses[204].content=response(200).content;});
reject('missing response schema',(d)=>{delete login(d).responses[200].content;});
reject('unknown response schema',(d)=>{login(d).responses[200].content['application/json'].schema.$ref='#/components/schemas/Unknown';});
reject('missing request CSRF',(d)=>{login(d).parameters=[];});
reject('duplicate request CSRF',(d)=>{login(d).parameters.push(requestHeader());});
reject('GET request CSRF',(d)=>{read(d).parameters.push(requestHeader());});
reject('request CSRF casing',(d)=>{login(d).parameters[0].name='x-csrf-token';});
reject('request CSRF underscore alias',(d)=>{login(d).parameters[0].name='X_CSRF_Token';});
reject('request CSRF query value',(d)=>{login(d).parameters[0].in='query';});
reject('request optional CSRF',(d)=>{login(d).parameters[0].required=false;});
reject('request CSRF indirection',(d)=>{login(d).parameters[0].schema={$ref:'#/components/schemas/Body'};});
reject('request CSRF alternative bounds',(d)=>{login(d).parameters[0].schema.maxLength=4096;});
reject('request CSRF description type',(d)=>{login(d).parameters[0].description=1;});
reject('request CSRF unknown extension',(d)=>{login(d).parameters[0]['x-extra']=1;});
for(const name of ['If-Match','Idempotency-Key'])reject('auth forbids business header '+name,(d)=>{login(d).parameters.push({name,in:'header',required:true,schema:{type:'string'}});});
reject('unknown cookie scheme',(d)=>{login(d).security=[{Unknown:[]}];});
reject('cookie scopes',(d)=>{login(d).security=[{CSRFCookie:['write']}];});
reject('public mutual TLS',(d)=>{login(d).security=[{OwnerTLS:[]}];});
reject('cookie scheme wrong name',(d)=>{d.components.securitySchemes.CSRFCookie.name='csrf';});
reject('cookie scheme bearer',(d)=>{d.components.securitySchemes.CSRFCookie={type:'http',scheme:'bearer'};});
reject('security object instead of alternatives',(d)=>{login(d).security={CSRFCookie:[]};});
reject('null auth security without document requirements',(d)=>{login(d).security=null;});
reject('null auth security with inherited document cookies',(d)=>{d.security=[{CSRFCookie:[]}];login(d).security=null;});
reject('null auth security with empty document requirements',(d)=>{d.security=[];read(d).security=null;});
reject('null auth document security',(d)=>{d.security=null;delete read(d).security;});
reject('null GET auth parameters',(d)=>{read(d).parameters=null;});
reject('null GET auth parameters with inherited security',(d)=>{d.security=[{CSRFCookie:[]}];delete read(d).security;read(d).parameters=null;});
reject('null unsafe auth parameters',(d)=>{login(d).parameters=null;});
for(const requirements of [undefined,[],[{}],[{CSRFCookie:[]}]])pass('omitted auth security inherits valid document '+JSON.stringify(requirements),()=>{
 const d=authDocument();if(requirements!==undefined)d.security=requirements;
 delete read(d).security;delete read(d).parameters;delete login(d).security;
 const p=compile(configEntry,d);for(const op of p.operations.filter((op)=>op.path==='/api/v1/identity/session'||op.path.endsWith('/login'))){
  assert.deepEqual(op.securityRequirements,(requirements??[]).map((alternative)=>Object.fromEntries(Object.keys(alternative).map((name)=>[name,d.components.securitySchemes[name]]))));
  assert.equal(op.authTransport.requestCSRF===null,op.method==='GET');
 }
 assert.deepEqual(p.operations.find((op)=>op.method==='GET').parameters,[]);
});
for(const requirements of [[],[{}],[{}, {SessionCookie:[]}],[{CSRFCookie:[]}]])pass('explicit auth security overrides document '+JSON.stringify(requirements),()=>{
 const d=authDocument();d.security=[{ChallengeCookie:[]}];read(d).security=requirements;read(d).parameters=[];login(d).security=requirements;
 const p=compile(configEntry,d);for(const op of p.operations.filter((op)=>op.path==='/api/v1/identity/session'||op.path.endsWith('/login'))){
  assert.deepEqual(op.securityRequirements,requirements.map((alternative)=>Object.fromEntries(Object.keys(alternative).map((name)=>[name,d.components.securitySchemes[name]]))));
  assert.equal(op.authTransport.requestCSRF===null,op.method==='GET');
 }
});
pass('legacy null operation fields retain historical fallback behavior',()=>{
 const d=structuredClone(fixture);const read=d.paths['/api/v1/identity/synthetic/{id}'].get;
 read.security=null;const empty=d.paths['/api/v1/identity/synthetic/{id}'].delete;
 const withoutId=structuredClone(empty);withoutId.parameters=null;withoutId.security=null;withoutId.operationId='DeleteLegacyNull';
 d.paths['/api/v1/identity/legacy-null']={delete:withoutId};
 const p=compile(configEntry,d);assert.equal(p.authProfile,undefined);
 assert.deepEqual(p.operations.find((op)=>op.id==='ReadSynthetic').securityRequirements,[]);
 const op=p.operations.find((op)=>op.id==='DeleteLegacyNull');assert.deepEqual(op.parameters,[]);assert.deepEqual(op.securityRequirements,[]);
});
for(const [method,path,status,rotation] of routes){
 for(const code of [status,401,403,429]){
  reject('missing cache '+path+' '+code,(d)=>{delete d.paths['/api/v1/identity'+path][method].responses[code].headers['Cache-Control'];});
  reject('wrong CSRF matrix '+path+' '+code,(d)=>{const headers=d.paths['/api/v1/identity'+path][method].responses[code].headers;if(Object.hasOwn(headers,'X-CSRF-Token'))delete headers['X-CSRF-Token'];else headers['X-CSRF-Token']=csrf();});
  reject('wrong retry matrix '+path+' '+code,(d)=>{const headers=d.paths['/api/v1/identity'+path][method].responses[code].headers;if(Object.hasOwn(headers,'Retry-After'))delete headers['Retry-After'];else headers['Retry-After']=retry();});
 }
}
for(const [name,status] of [['X-CSRF-Token',200],['Retry-After',429],['Cache-Control',200]]){
 for(const [label,mutate] of [
  ['optional',(h)=>{h.required=false;}],['missing required',(h)=>{delete h.required;}],
  ['string required',(h)=>{h.required='true';}],['schema ref',(h)=>{h.schema={$ref:'#/components/schemas/Body'};}],
  ['header ref',(h)=>{h.$ref='#/components/headers/Shared';}],['extra field',(h)=>{h.explode=false;}],
  ['content encoding',(h)=>{h.content={'application/json':{schema:h.schema}};}],
  ['extra schema',(h)=>{h.schema.description='not an accepted alternative form';}],
  ['bad description',(h)=>{h.description={};}],['nullable',(h)=>{h.schema.type=[h.schema.type,'null'];}],
 ])reject(name+' '+label,(d)=>mutate(login(d).responses[status].headers[name]));
 reject(name+' lower-case field',(d)=>{const headers=login(d).responses[status].headers;headers[name.toLowerCase()]=headers[name];delete headers[name];});
 reject(name+' duplicate case alias',(d)=>{const headers=login(d).responses[status].headers;headers[name.toLowerCase()]=headers[name];});
}
reject('unknown response header',(d)=>{login(d).responses[200].headers['Set-Cookie']=cache();});
reject('token empty grammar',(d)=>{login(d).responses[200].headers['X-CSRF-Token'].schema.pattern='^[A-Za-z0-9_-]{0,4096}$';});
reject('token alternate grammar',(d)=>{login(d).responses[200].headers['X-CSRF-Token'].schema.pattern='^[A-Za-z0-9_-]+$';});
reject('retry string encoding',(d)=>{login(d).responses[429].headers['Retry-After'].schema={type:'string',pattern:'^(0|[1-9][0-9]{0,9})$'};});
reject('retry bound overflow',(d)=>{login(d).responses[429].headers['Retry-After'].schema.maximum=2147483648;});
reject('retry negative bound',(d)=>{login(d).responses[429].headers['Retry-After'].schema.minimum=-1;});
reject('cache alternative const',(d)=>{login(d).responses[200].headers['Cache-Control'].schema.const='private, no-store';});
reject('cache equivalent enum',(d)=>{login(d).responses[200].headers['Cache-Control'].schema={type:'string',enum:['no-store']};});
pass('ordinary and internal security behavior stays separate',()=>{
 const mixed=authDocument();Object.assign(mixed.paths,structuredClone(fixture.paths));Object.assign(mixed.components.schemas,structuredClone(fixture.components.schemas));
 const mixedPlan=compile(configEntry,mixed);const internal=mixedPlan.operations.find((op)=>op.internal);
 assert(internal);assert.equal(internal.authTransport,undefined);assert.equal(internal.securityRequirements[0].OwnerTLS.type,'mutualTLS');
 const ordinary=mixedPlan.operations.find((op)=>op.id==='CreateSynthetic');assert.equal(ordinary.authTransport,undefined);assert(ordinary.parameters.some((p)=>p.name==='If-Match'));
});
pass('inherited cookie security resolves named metadata',()=>{const inherited=authDocument();inherited.security=[{CSRFCookie:[]}];delete login(inherited).security;const op=compile(configEntry,inherited).operations.find((op)=>op.path.endsWith('/login'));assert.equal(op.securityRequirements[0].CSRFCookie.name,'__Host-justix_csrf');});
pass('header descriptions are optional but the shape is closed',()=>{const minimal=authDocument();for(const methods of Object.values(minimal.paths))for(const op of Object.values(methods)){for(const parameter of op.parameters)delete parameter.description;for(const r of Object.values(op.responses))for(const header of Object.values(r.headers))delete header.description;}assert(compile(configEntry,minimal).authProfile);});
const legacy=compile({...configEntry,input:'input.json'},fixture);
pass('legacy has no auth model or advertised capability',()=>{assert.equal(legacy.authProfile,undefined);assert(legacy.operations.every((op)=>op.authTransport===undefined));for(const text of [renderTS(legacy),renderGo(legacy)]){assert(!text.includes('responseContractVersion'));assert(!text.includes('AuthSemantics'));}});
const legacyHashes={go:createHash('sha256').update(renderGo(legacy)).digest('hex'),ts:createHash('sha256').update(renderTS(legacy)).digest('hex')};
// Captured by running the exact 23d8cede predecessor generator on generationFixture
// with this entry/namespace and the pinned Go formatter, independently of this probe.
pass('legacy emitted bytes match the exact pre-auth-profile baseline',()=>assert.deepEqual(legacyHashes,{go:'8ee16a5bda4e5e1866c955edb00d85b44c1500fbe0f743c0dbe075514a02aa70',ts:'0a8d2463781ffc99f9ef287aecee73e9b149751833758ce3cd5efbce10c4d866'}));
writeFileSync(resolve(dirname(process.argv[2]),'auth.json'),JSON.stringify(authDocument()));
console.log(JSON.stringify({passingCases:count,legacyHashes,rows}));
`

func TestContractGenerationReproducibility(t *testing.T) {
	h := newGenerationHarness(t)
	h.config(true)
	h.generate(false, "--check")
	if _, err := os.Stat(filepath.Join(h.dir, "fixture.ts")); !os.IsNotExist(err) {
		t.Fatal("check mode wrote a missing output")
	}
	h.generate(true)
	goFirst, tsFirst := h.read("fixture.gen.go"), h.read("fixture.ts")
	if bytes.Contains(tsFirst, []byte("/internal/v1/")) || bytes.Contains(tsFirst, []byte("function ReadInternal(")) {
		t.Fatal("internal owner route became browser callable")
	}
	h.generate(true, "--check")
	// Different formatting/property order cannot change generated bytes or hash.
	var document any
	if err := json.Unmarshal([]byte(generationFixture), &document); err != nil {
		t.Fatal(err)
	}
	compact, _ := json.Marshal(document)
	h.write("input.json", string(compact))
	h.generate(true)
	if !bytes.Equal(goFirst, h.read("fixture.gen.go")) || !bytes.Equal(tsFirst, h.read("fixture.ts")) {
		t.Fatal("canonical input ordering was not reproducible")
	}
	h.write("fixture.ts", string(tsFirst)+"\n// explicit drift\n")
	h.generate(false, "--check")
	if !bytes.Contains(h.read("fixture.ts"), []byte("explicit drift")) {
		t.Fatal("check mode modified drift")
	}
	h.generate(true)
	h.generate(true, "--check")
	h.write("go.mod", "module synthetic\n\ngo 1.27.1\n")
	h.write("fixture_test.go", generatedGoChecks)
	h.run(true, filepath.Join(runtime.GOROOT(), "bin/go"), "test", "-race", "-count=1", "./...")
	h.run(true, filepath.Join(runtime.GOROOT(), "bin/go"), "vet", "./...")
	api, err := os.ReadFile(filepath.Join(h.root, "web/packages/api/src/client.ts"))
	if err != nil {
		t.Fatal(err)
	}
	h.write("client.ts", string(api))
	h.write("check.ts", generatedTSChecks)
	decoder, err := os.ReadFile(filepath.Join(h.root, "web/packages/api/src/responseJSON.ts"))
	if err != nil {
		t.Fatal(err)
	}
	h.write("responseJSON.ts", string(decoder))
	h.write("package.json", `{"type":"commonjs"}`)
	h.write("tsconfig.json", `{"compilerOptions":{"target":"ES2022","lib":["ES2022","DOM","DOM.Iterable"],"module":"Node16","moduleResolution":"Node16","strict":true,"noUncheckedIndexedAccess":true,"exactOptionalPropertyTypes":true,"noUnusedLocals":true,"noUnusedParameters":true,"skipLibCheck":false,"types":[],"paths":{"@justixauto/api":["./client.ts"]},"outDir":"out"},"include":["*.ts"]}`)
	h.run(true, h.node, filepath.Join(h.main, "node_modules/typescript/bin/tsc"), "-p", filepath.Join(h.dir, "tsconfig.json"))
	h.run(true, h.node, filepath.Join(h.dir, "out/check.js"))
}

func TestContractGenerationRejectsUnsupportedBeforeWrites(t *testing.T) {
	cases := map[string]func(string) string{
		"general YAML": func(string) string { return "openapi: 3.1.0\n" },
		"Go result symbol collision": func(s string) string {
			return strings.Replace(s, `"Maybe":`, `"CreateSyntheticResult":{"type":"string"},"Maybe":`, 1)
		},
		"TypeScript validator collision": func(s string) string {
			return strings.Replace(s, `"operationId":"ReadSynthetic"`, `"operationId":"PayloadSchema"`, 1)
		},
		"duplicate JSON keys": func(s string) string {
			return strings.Replace(s, `"openapi":"3.1.0"`, `"openapi":"3.1.0","openapi":"3.1.0"`, 1)
		},
		"old nullable": func(s string) string {
			return strings.Replace(s, `"type":["string","null"]`, `"type":"string","nullable":true`, 1)
		},
		"floating number": func(s string) string { return strings.Replace(s, `"type":"integer"`, `"type":"number"`, 1) },
		"unsafe integer":  func(s string) string { return strings.Replace(s, `"maximum":100`, `"maximum":9007199254740992`, 1) },
		"open object": func(s string) string {
			return strings.Replace(s, `"additionalProperties":false`, `"additionalProperties":true`, 1)
		},
		"inherited required": func(s string) string {
			return strings.Replace(s, `"required":["amountMinor"`, `"required":["toString","amountMinor"`, 1)
		},
		"unknown constraint": func(s string) string { return strings.Replace(s, `"maxLength":38`, `"maxLength":38,"default":"1"`, 1) },
		"external ref": func(s string) string {
			return strings.ReplaceAll(s, "#/components/schemas/Payload", "https://invalid.example/schema")
		},
		"missing ref": func(s string) string {
			return strings.ReplaceAll(s, "#/components/schemas/Payload", "#/components/schemas/Missing")
		},
		"recursive ref": func(s string) string {
			return strings.Replace(s, `"note":{"type":["string","null"]}`, `"note":{"$ref":"#/components/schemas/Payload"}`, 1)
		},
		"429 handoff": func(s string) string { return strings.Replace(s, `"422":`, `"429":`, 1) },
		"response header handoff": func(s string) string {
			return strings.Replace(s, `"description":"Synthetic empty"`, `"description":"Synthetic empty","headers":{"X-CSRF-Token":{}}`, 1)
		},
		"discriminator": func(s string) string {
			return strings.Replace(s, `"Choice":{"oneOf"`, `"Choice":{"discriminator":{"propertyName":"kind"},"oneOf"`, 1)
		},
		"unbounded integer":        func(s string) string { return strings.Replace(s, `,"maximum":100`, "", 1) },
		"path mismatch":            func(s string) string { return strings.Replace(s, `/synthetic/{id}`, `/synthetic/{foreign}`, 1) },
		"foreign owner":            func(s string) string { return strings.Replace(s, `/api/v1/identity/`, `/api/v1/commerce/`, 1) },
		"cookie parameter":         func(s string) string { return strings.Replace(s, `"in":"query"`, `"in":"cookie"`, 1) },
		"pattern dialect":          func(s string) string { return strings.Replace(s, `^x$`, `^(?=x)x$`, 1) },
		"RE2 incompatible pattern": func(s string) string { return strings.Replace(s, `^x$`, `[]`, 1) },
		"missing internal mTLS":    func(s string) string { return strings.Replace(s, `"security":[{"OwnerTLS":[]}]`, `"security":[]`, 1) },
		"unknown security": func(s string) string {
			return strings.Replace(s, `"type":"mutualTLS"`, `"type":"http","scheme":"bearer"`, 1)
		},
		"allOf": func(s string) string {
			return strings.Replace(s, `"Maybe":{"type":["string","null"]}`, `"Maybe":{"allOf":[{"type":"string"}]}`, 1)
		},
		"mixed map": func(s string) string {
			return strings.Replace(s, `"metadata":{"type":"object","additionalProperties"`, `"metadata":{"type":"object","properties":{},"additionalProperties"`, 1)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			h := newGenerationHarness(t)
			h.generate(true)
			before := h.read("fixture.gen.go")
			h.write("input.json", mutate(generationFixture))
			h.generate(false)
			if !bytes.Equal(before, h.read("fixture.gen.go")) {
				t.Fatal("invalid input partially wrote outputs")
			}
		})
	}
}

func TestContractGenerationInternalOnly(t *testing.T) {
	h := newGenerationHarness(t)
	var document map[string]any
	if err := json.Unmarshal([]byte(generationFixture), &document); err != nil {
		t.Fatal(err)
	}
	paths := document["paths"].(map[string]any)
	delete(paths, "/api/v1/identity/synthetic/{id}")
	encoded, _ := json.Marshal(document)
	h.write("input.json", string(encoded))
	h.generate(true)
	h.write("go.mod", "module synthetic\n\ngo 1.27.1\n")
	h.run(true, filepath.Join(runtime.GOROOT(), "bin/go"), "test", "-race", "./...")
	api, err := os.ReadFile(filepath.Join(h.root, "web/packages/api/src/client.ts"))
	if err != nil {
		t.Fatal(err)
	}
	h.write("client.ts", string(api))
	decoder, err := os.ReadFile(filepath.Join(h.root, "web/packages/api/src/responseJSON.ts"))
	if err != nil {
		t.Fatal(err)
	}
	h.write("responseJSON.ts", string(decoder))
	h.write("tsconfig.json", `{"compilerOptions":{"target":"ES2022","lib":["ES2022","DOM","DOM.Iterable"],"module":"Node16","moduleResolution":"Node16","strict":true,"noUnusedLocals":true,"noUnusedParameters":true,"types":[],"paths":{"@justixauto/api":["./client.ts"]},"noEmit":true},"include":["*.ts"]}`)
	h.run(true, h.node, filepath.Join(h.main, "node_modules/typescript/bin/tsc"), "-p", filepath.Join(h.dir, "tsconfig.json"))
}

func TestContractGenerationSymbolCollisionsBeforeWrites(t *testing.T) {
	h := newGenerationHarness(t)
	h.generate(true)
	goBefore, tsBefore := h.read("fixture.gen.go"), h.read("fixture.ts")
	// Imports, emitted helpers/constructor, generated declarations, and every
	// referenced capitalized TS global must not be shadowed by accepted DTOs.
	for _, name := range []string{"ApiRequest", "ApiResult", "NewContractClient", "ContractTransport", "ContractOptional", "ContractClient", "ContractExchange", "MarshalJSON", "UnmarshalJSON", "IsZero", "Marshal", "Unmarshal", "Context", "PayloadSchema", "ReadSyntheticSecurity", "ReadSyntheticParameters", "CreateSyntheticResult", "Record", "Readonly", "ReadonlyArray", "Object", "Array", "JSON", "Error", "RegExp", "Number", "Date", "Reflect", "Set", "Promise", "AbortSignal", "URLSearchParams"} {
		t.Run("schema_"+name, func(t *testing.T) {
			h.write("input.json", strings.Replace(generationFixture, `"Maybe":`, `"`+name+`":{"type":"string"},"Maybe":`, 1))
			h.generate(false)
			if !bytes.Equal(goBefore, h.read("fixture.gen.go")) || !bytes.Equal(tsBefore, h.read("fixture.ts")) {
				t.Fatal("symbol rejection changed an output")
			}
		})
	}
	for _, name := range []string{"ApiRequest", "ApiResult", "ContractTransport", "MarshalJSON", "UnmarshalJSON", "IsZero", "Marshal", "Unmarshal", "Context", "Record", "Readonly", "ReadonlyArray", "Object", "Array", "JSON", "Error", "RegExp", "Number", "Date", "Reflect", "Set", "Promise", "AbortSignal", "URLSearchParams", "PayloadSchema", "CreateSyntheticSecurity"} {
		t.Run("operation_"+name, func(t *testing.T) {
			h.write("input.json", strings.Replace(generationFixture, `"operationId":"ReadSynthetic"`, `"operationId":"`+name+`"`, 1))
			h.generate(false)
			if !bytes.Equal(goBefore, h.read("fixture.gen.go")) || !bytes.Equal(tsBefore, h.read("fixture.ts")) {
				t.Fatal("symbol rejection changed an output")
			}
		})
	}
	h.write("input.json", strings.Replace(generationFixture, `"note":{"type":["string","null"]}`, `"note":{"type":["string","null"]},"payload":{"type":"string"},"fixturePayload":{"type":"string"}`, 1))
	h.generate(false)
	if !bytes.Equal(goBefore, h.read("fixture.gen.go")) || !bytes.Equal(tsBefore, h.read("fixture.ts")) {
		t.Fatal("namespaced field collision changed an output")
	}
	// Distinct namespace strings can still produce identical final Go symbols.
	h.config(true)
	h.write("config.json", strings.Replace(string(h.read("config.json")), `"Second"`, `"FixtureExtra"`, 1))
	h.write("input.json", strings.Replace(generationFixture, `"Maybe":`, `"ExtraPayload":{"type":"string"},"Maybe":`, 1))
	h.generate(false)
	if !bytes.Equal(goBefore, h.read("fixture.gen.go")) || !bytes.Equal(tsBefore, h.read("fixture.ts")) {
		t.Fatal("cross-file symbol rejection changed an output")
	}
	for _, path := range []string{"second.gen.go", "second.ts"} {
		if _, err := os.Lstat(filepath.Join(h.dir, path)); !os.IsNotExist(err) {
			t.Fatal("cross-file symbol rejection created an output")
		}
	}
}

func TestContractGenerationGoMethodContractsBeforeWrites(t *testing.T) {
	h := newGenerationHarness(t)
	h.generate(true)
	goBefore, tsBefore := h.read("fixture.gen.go"), h.read("fixture.ts")
	// Go-only operation Error bypassed the public TS reserved-name check and
	// removed ContractFailure's error interface. Cover every runtime/DTO hook
	// and the external receiver methods used by the decoder, numbers, patterns
	// and query builder; those selectors must survive namespacing as well.
	names := []string{"Error", "MarshalJSON", "UnmarshalJSON", "IsZero", "Exchange", "Token", "UseNumber", "More", "SetString", "IsInt", "Num", "IsInt64", "Int64", "String", "MatchString", "Set", "Encode",
		// Stable fields in the runtime transport/security/failure/optional types
		// and result wrappers must not be renamed consistently into a broken API.
		"Owner", "OperationID", "Method", "Path", "Headers", "Body", "Security", "Status", "ContentType", "Kind", "UnknownOutcome", "Type", "In", "Name", "Value", "Present", "Status200", "Status201", "Status422"}
	for _, kind := range []string{"schema", "public-operation", "internal-operation"} {
		for _, name := range names {
			t.Run(kind+"_"+name, func(t *testing.T) {
				h := h
				h.t = t
				input := generationFixture
				if kind == "schema" {
					input = strings.Replace(input, `"Maybe":`, `"`+name+`":{"type":"string"},"Maybe":`, 1)
				} else {
					original := "ReadSynthetic"
					if kind == "internal-operation" {
						original = "ReadInternal"
					}
					input = strings.Replace(input, `"operationId":"`+original+`"`, `"operationId":"`+name+`"`, 1)
				}
				h.write("input.json", input)
				h.generate(false)
				if !bytes.Equal(goBefore, h.read("fixture.gen.go")) || !bytes.Equal(tsBefore, h.read("fixture.ts")) {
					t.Fatal("Go method collision changed an output")
				}
			})
		}
	}
}

func TestContractGenerationSelectorLookingWireText(t *testing.T) {
	h := newGenerationHarness(t)
	input := strings.Replace(generationFixture, `"operationId":"ReadInternal"`, `"operationId":"ReadReceipt"`, 1)
	// Neither ordinary receiver-looking text nor imported-package-looking text
	// in schema literals is Go code. Both must survive generation and decoding.
	input = strings.Replace(input, `"Maybe":`, `"WireText":{"type":"string","enum":["x.ReadReceipt(","strings.ReadReceipt(","/* x.ReadReceipt( */","'x.ReadReceipt('","`+"`x.ReadReceipt(`"+`"]},"Maybe":`, 1)
	h.write("input.json", input)
	h.generate(true)
	if bytes.Contains(h.read("fixture.ts"), []byte("/internal/v1/")) || bytes.Contains(h.read("fixture.ts"), []byte("function ReadReceipt(")) {
		t.Fatal("internal method exposed to browser")
	}
	h.write("go.mod", "module synthetic\n\ngo 1.27.1\n")
	h.write("wire_test.go", `package synthetic
import("context";"encoding/json";"testing")
type wireExchange struct{}
func(wireExchange) Exchange(_ context.Context,r FixtureContractRequest)(FixtureContractResponse,error){
 _=r.Owner;_=r.OperationID;_=r.Method;_=r.Path;_=r.Headers;_=r.Body;_=r.Security
 return FixtureContractResponse{Status:200,ContentType:"application/json",Body:nil},FixtureContractFailure{Kind:"synthetic failure",UnknownOutcome:true}
}
var _ FixtureContractExchange=wireExchange{}
var _ error=FixtureContractFailure{}
func TestWireText(t *testing.T){
 if _,err:=FixtureNewContractClient(wireExchange{});err!=nil{t.Fatal(err)}
 for _,value:=range []string{"x.ReadReceipt(","strings.ReadReceipt(","/* x.ReadReceipt( */","'x.ReadReceipt('","`+"`x.ReadReceipt(`"+`"}{
  raw,_:=json.Marshal(value);var decoded FixtureWireText
  if err:=json.Unmarshal(raw,&decoded);err!=nil{t.Fatal(err)}
  after,err:=json.Marshal(decoded);if err!=nil||string(after)!=string(raw){t.Fatal("wire text changed",err)}
 }
}
`)
	h.run(true, filepath.Join(runtime.GOROOT(), "bin/go"), "test", "-race", "./...")
	h.run(true, filepath.Join(runtime.GOROOT(), "bin/go"), "vet", "./...")
	api, err := os.ReadFile(filepath.Join(h.root, "web/packages/api/src/client.ts"))
	if err != nil {
		t.Fatal(err)
	}
	h.write("client.ts", string(api))
	decoder, err := os.ReadFile(filepath.Join(h.root, "web/packages/api/src/responseJSON.ts"))
	if err != nil {
		t.Fatal(err)
	}
	h.write("responseJSON.ts", string(decoder))
	h.write("tsconfig.json", `{"compilerOptions":{"target":"ES2022","lib":["ES2022","DOM","DOM.Iterable"],"module":"Node16","moduleResolution":"Node16","strict":true,"types":[],"paths":{"@justixauto/api":["./client.ts"]},"outDir":"out"},"include":["*.ts"]}`)
	h.write("wire.ts", "import { WireTextSchema } from './fixture.js';\nfor (const value of ['x.ReadReceipt(', 'strings.ReadReceipt(', '/* x.ReadReceipt( */', \"'x.ReadReceipt('\", '`x.ReadReceipt(`']) { if (WireTextSchema.parse(value) !== value || WireTextSchema.parse(JSON.parse(JSON.stringify(value))) !== value) throw new Error('wire text changed'); }\n")
	h.run(true, h.node, filepath.Join(h.main, "node_modules/typescript/bin/tsc"), "-p", filepath.Join(h.dir, "tsconfig.json"))
	h.run(true, h.node, filepath.Join(h.dir, "out/wire.js"))
}

func TestContractGenerationDanglingLinksBeforeWrites(t *testing.T) {
	for _, path := range []string{"second.ts", "second.gen.go", "missing-parent"} {
		t.Run(path, func(t *testing.T) {
			h := newGenerationHarness(t)
			h.config(true)
			h.generate(true)
			// Force an earlier output to need rewriting; a later invalid output must
			// abort the entire plan before any write, in check and generation modes.
			h.write("fixture.gen.go", string(h.read("fixture.gen.go"))+"\n// preserved drift\n")
			before := map[string][]byte{}
			for _, file := range []string{"fixture.gen.go", "fixture.ts", "second.gen.go", "second.ts"} {
				before[file] = h.read(file)
			}
			if path == "missing-parent" {
				h.write("config.json", strings.Replace(string(h.read("config.json")), `"second.ts"`, `"missing-parent/second.ts"`, 1))
			} else if err := os.Remove(filepath.Join(h.dir, path)); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(filepath.Dir(h.dir), filepath.Base(h.dir)+"-absent-target")
			link := filepath.Join(h.dir, path)
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			for _, check := range []bool{true, false} {
				if check {
					h.generate(false, "--check")
				} else {
					h.generate(false)
				}
				if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("link changed: %v", err)
				}
				if got, err := os.Readlink(link); err != nil || got != target {
					t.Fatalf("link target changed: %s %v", got, err)
				}
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatalf("dangling target created: %v", err)
				}
				for file, content := range before {
					if file != path && !bytes.Equal(content, h.read(file)) {
						t.Fatalf("other output %s changed", file)
					}
				}
			}
		})
	}
}

func TestContractGenerationOutputGuards(t *testing.T) {
	for _, which := range []string{"empty", "namespace", "duplicate output", "input overwrite", "user file", "symlink", "malformed UTF8"} {
		t.Run(which, func(t *testing.T) {
			h := newGenerationHarness(t)
			switch which {
			case "empty":
				h.write("config.json", `{"version":1,"contracts":[]}`)
			case "malformed UTF8":
				h.write("input.json", strings.Replace(generationFixture, "Synthetic generation fixture", string([]byte{0xff}), 1))
			case "namespace":
				h.config(true)
				h.write("config.json", strings.Replace(string(h.read("config.json")), `"Second"`, `"Fixture"`, 1))
			case "duplicate output":
				h.config(true)
				h.write("config.json", strings.Replace(string(h.read("config.json")), `"second.ts"`, `"fixture.ts"`, 1))
			case "input overwrite":
				h.write("input.ts", generationFixture)
				h.write("config.json", strings.ReplaceAll(strings.ReplaceAll(string(h.read("config.json")), "input.json", "input.ts"), "fixture.ts", "input.ts"))
			case "user file":
				h.write("fixture.gen.go", "package synthetic // user owned\n")
			case "symlink":
				h.write("target.ts", "// user target\n")
				if err := os.Symlink(filepath.Join(h.dir, "target.ts"), filepath.Join(h.dir, "fixture.ts")); err != nil {
					t.Fatal(err)
				}
			}
			h.generate(false)
			if _, err := os.Stat(filepath.Join(h.dir, "second.gen.go")); !os.IsNotExist(err) {
				t.Fatal("failed plan wrote an output")
			}
		})
	}
}

const generatedGoChecks = `package synthetic
import("context";"encoding/json";"errors";"strings";"testing")
const validPayload = "{\"amountMinor\":\"99999999999999999999999999999999999999\",\"revision\":\"9223372036854775807\",\"nullable\":null,\"tags\":[],\"metadata\":{},\"count\":1}"
func TestMapKeyUnicode(t *testing.T){for _,tt:=range []struct{key string;valid bool}{{"\\ud800",false},{"\\udfff",false},{"prefix\\ud800suffix",false},{"\\ud83d\\ude00",true},{"😀",true},{"\\ufffd",true}}{raw:=strings.Replace(validPayload,"\"metadata\":{}","\"metadata\":{\""+tt.key+"\":\"x\"}",1);var value FixturePayload;err:=json.Unmarshal([]byte(raw),&value);if (err==nil)!=tt.valid{t.Fatalf("key %q accepted=%v",tt.key,err==nil)};if tt.valid{var key string;if err:=json.Unmarshal([]byte("\""+tt.key+"\""),&key);err!=nil{t.Fatal(err)};if len(value.Metadata)!=1||value.Metadata[key]!="x"{t.Fatal("map key normalized/dropped")};encoded,err:=json.Marshal(value);if err!=nil{t.Fatal(err)};var again FixturePayload;if err=json.Unmarshal(encoded,&again);err!=nil||again.Metadata[key]!="x"{t.Fatal("map key round trip changed")}}}}
func TestRoundTrip(t *testing.T){var value FixturePayload;if err:=json.Unmarshal([]byte(validPayload),&value);err!=nil{t.Fatal(err)};if value.Note.Present||value.Nullable!=nil||value.AmountMinor!="99999999999999999999999999999999999999"{t.Fatal("precision/presence")};raw,err:=json.Marshal(value);if err!=nil||strings.Contains(string(raw),"note"){t.Fatalf("%s %v",raw,err)};value.Note=FixtureContractOptional[*string]{Present:true};raw,err=json.Marshal(value);if err!=nil||!strings.Contains(string(raw),"\"note\":null"){t.Fatalf("%s %v",raw,err)};var again FixturePayload;if err=json.Unmarshal(raw,&again);err!=nil||!again.Note.Present||again.Note.Value!=nil{t.Fatal(err)};value.Count=101;if _,err=json.Marshal(value);err==nil{t.Fatal("invalid constructed DTO serialized")}
 for _,bad:=range []string{strings.Replace(validPayload,"\"nullable\":null,","",1),strings.Replace(validPayload,"\"count\":1","\"count\":1,\"unknown\":true",1),strings.Replace(validPayload,"\"count\":1","\"count\":null",1),strings.Replace(validPayload,"\"count\":1","\"count\":1,\"count\":2",1),strings.Replace(validPayload,"\"tags\":[]","\"tags\":[\"x\",\"x\"]",1),strings.Replace(validPayload,"\"metadata\":{}","\"metadata\":{\"x\":1}",1),strings.Replace(validPayload,"\"nullable\":null","\"nullable\":\"\\ud800\"",1),validPayload+" {}"}{var next FixturePayload;if json.Unmarshal([]byte(bad),&next)==nil{t.Fatalf("accepted %s",bad)}}
 for _,number:=range []string{"1.0","1e0"}{var next FixturePayload;if err:=json.Unmarshal([]byte(strings.Replace(validPayload,"\"count\":1","\"count\":"+number,1)),&next);err!=nil||next.Count!=1{t.Fatal(err)}}
 var maybe FixtureMaybe;if err:=json.Unmarshal([]byte("null"),&maybe);err!=nil||maybe.Value!=nil{t.Fatal(err)}
 for _,suffix:=range []string{"\n","\r\n","\u2028","\u2029"}{var value FixturePayload;_ = json.Unmarshal([]byte(validPayload),&value);value.AmountMinor="1"+suffix;if _,err:=json.Marshal(value);err==nil{t.Fatal("noncanonical amount")};value.AmountMinor="1";value.Revision="1"+suffix;if _,err:=json.Marshal(value);err==nil{t.Fatal("noncanonical revision")};raw,_:=json.Marshal("x"+suffix);var loose FixtureLooseEnd;if json.Unmarshal(raw,&loose)==nil{t.Fatal("ordinary dollar accepted final line terminator")}}
}
func TestUnionsAndDates(t *testing.T){var v FixtureChoice;pending:=[]byte("{\"kind\":\"pending\",\"operationId\":\"11111111-1111-4111-8111-111111111111\"}");if err:=json.Unmarshal(pending,&v);err!=nil||v.Variant2==nil||v.Variant1!=nil{t.Fatal(err)};if _,err:=json.Marshal(v);err!=nil{t.Fatal(err)};v.Variant1=&FixtureReady{};if _,err:=json.Marshal(v);err==nil{t.Fatal("two alternatives accepted")};for _,raw:=range []string{"{\"kind\":\"unknown\"}","null"}{if json.Unmarshal([]byte(raw),&v)==nil{t.Fatal("union accepted invalid")}}
 for _,date:=range []string{"2026-02-30","2026-13-01"}{var d FixtureDates;if json.Unmarshal([]byte("{\"date\":\""+date+"\",\"instant\":\"2026-09-15T12:00:00Z\"}"),&d)==nil{t.Fatal("bad date")}}
}
type exchangeFunc func(context.Context,FixtureContractRequest)(FixtureContractResponse,error)
func(f exchangeFunc)Exchange(ctx context.Context,r FixtureContractRequest)(FixtureContractResponse,error){return f(ctx,r)}
func TestInjectedClient(t *testing.T){calls:=0;var body FixturePayload;if err:=json.Unmarshal([]byte(validPayload),&body);err!=nil{t.Fatal(err)};client,err:=FixtureNewContractClient(exchangeFunc(func(_ context.Context,r FixtureContractRequest)(FixtureContractResponse,error){calls++;if r.Path!="/api/v1/identity/synthetic/a?filter=a%26b"||r.Headers["If-Match"]!="\"9223372036854775807\""||r.Method!="POST"{t.Fatalf("bad request %#v",r)};return FixtureContractResponse{Status:201,ContentType:"application/json",Body:[]byte(validPayload)},nil}));if err!=nil{t.Fatal(err)}
 params:=FixtureCreateSyntheticParameters{Id:"a",IfMatch:"9223372036854775807",Filter:FixtureContractOptional[string]{Value:"a&b",Present:true}};result,err:=client.FixtureCreateSynthetic(context.Background(),params,body);if err!=nil||result.Status201==nil||calls!=1{t.Fatalf("%#v %v",result,err)}
 params.Id="..";if _,err:=client.FixtureCreateSynthetic(context.Background(),params,body);err==nil||calls!=1{t.Fatal("unsafe route reached exchange")};params.Id="a";params.IfMatch="9223372036854775808";if _,err:=client.FixtureCreateSynthetic(context.Background(),params,body);err==nil||calls!=1{t.Fatal("unsafe revision")}
 failed,_:=FixtureNewContractClient(exchangeFunc(func(context.Context,FixtureContractRequest)(FixtureContractResponse,error){calls++;return FixtureContractResponse{},errors.New("synthetic lost reply secret excluded")}));params.IfMatch="1";_,err=failed.FixtureCreateSynthetic(context.Background(),params,body);var failure FixtureContractFailure;if !errors.As(err,&failure)||!failure.UnknownOutcome||strings.Contains(err.Error(),"secret")||calls!=2{t.Fatal("lost reply/retry semantics")}
 internal,_:=FixtureNewContractClient(exchangeFunc(func(_ context.Context,r FixtureContractRequest)(FixtureContractResponse,error){if r.Owner!="identity"||r.OperationID!="ReadInternal"||r.Path!="/internal/v1/identity/synthetic/a"||len(r.Security)!=1||r.Security[0]["OwnerTLS"].Type!="mutualTLS"{t.Fatalf("internal declaration lost: %#v",r)};return FixtureContractResponse{Status:200,ContentType:"application/json",Body:[]byte(validPayload)},nil}));if result,err:=internal.FixtureReadInternal(context.Background(),FixtureReadInternalParameters{Id:"a"});err!=nil||result.Status200==nil{t.Fatal(err)}
}
`

const generatedTSChecks = `import {PayloadSchema,ChoiceSchema,DatesSchema,MaybeSchema,LooseEndSchema,CreateSynthetic,ReadSynthetic,ReadSyntheticSecurity,DeleteSynthetic} from './fixture.js';
import type {Payload} from './fixture.js';
import {createApiClient} from './client.js';
function assert(value: unknown, message: string): asserts value { if (!value) throw new Error(message); }
function rejects(run: () => unknown): void { let rejected = false; try { run(); } catch { rejected = true; } assert(rejected,'invalid fixture accepted'); }
const payload: Payload = {amountMinor:'99999999999999999999999999999999999999',revision:'9223372036854775807',nullable:null,count:1,tags:[],metadata:{}};
assert(PayloadSchema.parse(payload).amountMinor===payload.amountMinor,'precision changed');
assert(!Object.hasOwn(PayloadSchema.parse(payload),'note'),'omitted changed');
assert(PayloadSchema.parse({...payload,note:null}).note===null,'explicit null changed');
for (const bad of [{...payload,unknown:true},{...payload,note:undefined},{...payload,nullable:undefined},{...payload,count:null},{...payload,count:1.2},{...payload,amountMinor:1},{...payload,tags:['x','x']},{...payload,metadata:{x:1}},{...payload,note:'\ud800'}]) rejects(()=>PayloadSchema.parse(bad));
for (const key of ['\ud800','\udfff','prefix\ud800suffix']) rejects(()=>PayloadSchema.parse({...payload,metadata:{[key]:'x'}}));
for (const key of ['\ud83d\ude00','😀','\ufffd']) { const value=PayloadSchema.parse({...payload,metadata:{[key]:'x'}}); assert(Object.keys(value.metadata).length===1&&value.metadata[key]==='x','map key normalized/dropped'); assert(PayloadSchema.parse(JSON.parse(JSON.stringify(value))).metadata[key]==='x','map key round trip changed'); }
const {nullable: removed,...missing} = payload; void removed; rejects(()=>PayloadSchema.parse(missing));
const inherited = Object.create({nullable:null}) as Record<string,unknown>; Object.assign(inherited,payload); rejects(()=>PayloadSchema.parse(inherited));
const sparse: unknown[] = []; sparse.length=1; rejects(()=>PayloadSchema.parse({...payload,tags:sparse}));
assert(MaybeSchema.parse(null)===null,'nullable root');
assert(ReadSyntheticSecurity[0].SessionCookie.name==='__Host-justix_session','public cookie declaration lost');
for(const suffix of ['\n','\r\n','\u2028','\u2029']) { rejects(()=>PayloadSchema.parse({...payload,amountMinor:'1'+suffix})); rejects(()=>PayloadSchema.parse({...payload,revision:'1'+suffix})); rejects(()=>LooseEndSchema.parse('x'+suffix)); }
assert(ChoiceSchema.parse({kind:'pending',operationId:'11111111-1111-4111-8111-111111111111'}).kind==='pending','union');
rejects(()=>ChoiceSchema.parse({kind:'unknown'}));
for(const date of ['2026-02-30','2026-13-01']) rejects(()=>DatesSchema.parse({date,instant:'2026-09-15T12:00:00Z'}));
// @ts-expect-error explicit undefined is not an optional nullable string
const wrong: Payload = {...payload,note:undefined}; void wrong;
async function main(): Promise<void> {
 let calls=0; let response: Response = new Response(JSON.stringify(payload),{status:201,headers:{'Content-Type':'application/json'}});
 const client=createApiClient({origin:'https://synthetic.invalid',fetch:async(input,init)=>{calls++;assert(String(input)==='https://synthetic.invalid/api/v1/identity/synthetic/a?filter=a%26b','route encoding');assert(new Headers(init?.headers).get('If-Match')==='"9223372036854775807"','revision precision');assert(init?.credentials==='same-origin'&&init.redirect==='error'&&init.cache==='no-store','shared transport');return response;}});
 const params={id:'a',filter:'a&b','If-Match':'9223372036854775807'};
 const ok=await CreateSynthetic(client,params,payload);assert(ok.kind==='success'&&ok.status===201&&calls===1,'typed success');
 const invalid=await CreateSynthetic(client,{...params,id:'..'},payload);assert(invalid.kind==='invalid-request'&&calls===1,'unsafe path performed IO');
 for(const revision of ['1\n','1\r\n','1\u2028','9223372036854775808']) { const badHeader=await CreateSynthetic(client,{...params,'If-Match':revision},payload);assert(badHeader.kind==='invalid-request'&&calls===1,'bad revision header performed IO'); }
 response=new Response(JSON.stringify({error:{code:'INVALID',message:'Synthetic',fields:{x:123},traceId:'synthetic'}}),{status:422,headers:{'Content-Type':'application/json'}});
 const bad=await CreateSynthetic(client,params,payload);assert(bad.kind==='invalid-response','feature error validator');
 const failed=createApiClient({origin:'https://synthetic.invalid',fetch:async()=>{throw new Error('synthetic lost reply');}});
 const lost=await CreateSynthetic(failed,params,payload);assert(lost.kind==='transport-error'&&lost.outcome==='unknown','unknown write');
 const unionClient=createApiClient({origin:'https://synthetic.invalid',fetch:async()=>new Response(JSON.stringify({kind:'pending',operationId:'11111111-1111-4111-8111-111111111111'}),{headers:{'Content-Type':'application/json'}})});
 const union=await ReadSynthetic(unionClient,{id:'a'});assert(union.kind==='success'&&union.data.kind==='pending','union response');
 const emptyClient=createApiClient({origin:'https://synthetic.invalid',fetch:async()=>new Response(null,{status:204})});
 const empty=await DeleteSynthetic(emptyClient,{id:'a'});assert(empty.kind==='success'&&empty.data===undefined,'204');
}
void main();
`
