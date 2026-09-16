package contracts_test

import (
 "encoding/json"
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
)

// Compare hook values and outcomes for Unicode map keys beyond ASCII traces.
func TestQA938UnicodeSemanticTraversalParity(t *testing.T){
 h:=newGenerationHarness(t)
 source,err:=os.ReadFile(filepath.Join(h.root,"tools/generate-contracts.mjs"));if err!=nil{t.Fatal(err)}
 entry:="\ntry { main(); } catch (error)";if strings.Count(string(source),entry)!=1{t.Fatal("entry changed")}
 end:=strings.Index(inertAuthProfileChecks,"\nlet count=0;");if end<0{t.Fatal("fixture changed")}
 goroot,_:=json.Marshal(runtime.GOROOT())
 h.write("qa-emitter.mjs",string(source[:strings.LastIndex(string(source),entry)])+"\ncachedGoRoot="+string(goroot)+";\n"+inertAuthProfileChecks[:end]+authSemanticEmitterChecks)
 h.write("qa.ts",qa938TS);h.write("qa_test.go",qa938Go)
 h.write("go.mod","module synthetic\n\ngo 1.27.1\n")
 t.Log(h.run(true,h.node,filepath.Join(h.dir,"qa-emitter.mjs"),filepath.Join(h.dir,"input.json")))
 h.write("tsconfig.json",`{"compilerOptions":{"strict":true,"exactOptionalPropertyTypes":true,"noUncheckedIndexedAccess":true,"target":"ES2022","module":"CommonJS","moduleResolution":"node","ignoreDeprecations":"6.0","skipLibCheck":true,"outDir":"dist","types":[]},"include":["semantic.ts","bindings.ts","qa.ts"]}`)
 t.Log(h.run(true,h.node,filepath.Join(h.main,"node_modules/typescript/bin/tsc"),"--project","tsconfig.json"))
 t.Log(h.run(true,h.node,"dist/qa.js"))
 evidence:=filepath.Join(h.root,"docs/justix-auto/dev/qa/T-938")
 for _,name:=range []string{"semantic.ts","semantic.gen.go","bindings.ts","bindings_test.go","qa.ts","qa_test.go","input.json","qa-ts.json"}{
  b:=h.read(name);if len(b)>0&&b[len(b)-1]!='\n'{b=append(b,'\n')}
  if err:=os.WriteFile(filepath.Join(evidence,name+".txt"),b,0600);err!=nil{t.Fatal(err)}
 }
 t.Log(h.run(true,filepath.Join(runtime.GOROOT(),"bin/go"),"test","-race","-count=1","-v","./..."))
}

const qa938TS=`import * as s from './semantic';
import {bindings} from './bindings';
declare function require(name:string):any;
const fs=require('node:fs');
const input={a:{kind:'left',value:'ordinary'},items:[],map:{'\uE000':'bmp','\u{10000}':'supplementary'},z:true};
const rows=[];
for(const reject of [false,true]){
 const calls:string[]=[];
 const validators=s.createAuthValidators(bindings((name,value)=>{
  if(name==='Leaf'){calls.push(String(value));if(reject&&value==='bmp')return 'mismatch';if(reject&&value==='supplementary')return 'fatal:policy';}
  return 'valid';
 }));
 let outcome:s.SemanticOutcome='valid';
 try{validators.EnvelopeSchema.parse(input);}catch(e){if(!(e instanceof s.AuthValidationError))throw e;outcome=e.outcome;}
 rows.push({reject,outcome,calls});
}
fs.writeFileSync('qa-ts.json',JSON.stringify(rows)+'\n');
console.log('Actual TypeScript semantic traversal:',JSON.stringify(rows));
`

const qa938Go=`package synthetic
import("encoding/json";"os";"reflect";"testing")
func TestQAUnicodeTraversal(t *testing.T){
 input:=[]byte("{\"a\":{\"kind\":\"left\",\"value\":\"ordinary\"},\"items\":[],\"map\":{\"\uE000\":\"bmp\",\"\U00010000\":\"supplementary\"},\"z\":true}")
 type row struct{Reject bool;Outcome string;Calls []string}
 var rows []row
 for _,reject:=range []bool{false,true}{
  calls:=[]string{}
  validators,err:=FixtureNewAuthValidators(&probeBinding{call:func(name string,value any)FixtureSemanticOutcome{
   if name=="Leaf"{text:=string(value.(FixtureLeaf));calls=append(calls,text);if reject&&text=="bmp"{return FixtureSemanticMismatch};if reject&&text=="supplementary"{return FixtureSemanticFatalPolicy}}
   return FixtureSemanticValid
  }});if err!=nil{t.Fatal(err)}
  _,err=validators.ParseEnvelope(input);outcome:="valid"
  if err!=nil{failure,ok:=err.(FixtureAuthValidationError);if !ok{t.Fatal(err)};switch failure.Outcome{case FixtureSemanticMismatch:outcome="mismatch";case FixtureSemanticFatalPolicy:outcome="fatal:policy";default:t.Fatal(failure)}}
  rows=append(rows,row{reject,outcome,calls})
 }
 raw,err:=os.ReadFile("qa-ts.json");if err!=nil{t.Fatal(err)};var expected []row;if err=json.Unmarshal(raw,&expected);err!=nil{t.Fatal(err)}
 encoded,_:=json.Marshal(rows);t.Logf("Actual Go semantic traversal: %s",encoded)
 if !reflect.DeepEqual(rows,expected){t.Fatalf("semantic Go/TS Unicode parity diverges\nGo: %s\nTS: %s",encoded,raw)}
}
`
