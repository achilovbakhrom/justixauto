import {readFileSync,writeFileSync} from 'node:fs';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
import assert from 'node:assert/strict';
const qa=path.dirname(fileURLToPath(import.meta.url)),root=path.resolve(qa,'../../../../..');
const output=JSON.parse(readFileSync(path.join(qa,'independent-output.json'),'utf8'));
const dir=output.parityDirectory;
assert.ok(dir.startsWith('/private/tmp/justix-t935-independent-parity-'));
const schema={openapi:'3.1.0',info:{title:'Independent QA corpus',version:'1'},components:{schemas:{Count:{type:'integer',minimum:-9007199254740991,maximum:9007199254740991}}},paths:{'/api/v1/identity/synthetic':{get:{operationId:'ReadProbe',responses:{200:{description:'Synthetic',content:{'application/json':{schema:{$ref:'#/components/schemas/Count'}}}}}}}}};
writeFileSync(path.join(dir,'input.json'),JSON.stringify(schema)+'\n');
writeFileSync(path.join(dir,'config.json'),JSON.stringify({version:1,contracts:[{input:'input.json',goOutput:'audit.gen.go',tsOutput:'audit.ts',goPackage:'audit',owner:'identity',namespace:'Audit'}]})+'\n');
const generated=spawnSync(process.execPath,[path.join(root,'tools/generate-contracts.mjs'),'--config',path.join(dir,'config.json')],{encoding:'utf8',env:{...process.env,GOPROXY:'off',GOTOOLCHAIN:'local',GOCACHE:'/private/tmp/justixauto-integration-gocache',GOMODCACHE:'/private/tmp/justixauto-t003-modcache'}});
assert.equal(generated.status,0,generated.stderr);
writeFileSync(path.join(dir,'go.mod'),'module independent-decoder-audit\n\ngo 1.27.1\n');
writeFileSync(path.join(dir,'parity_test.go'),`package audit
import("encoding/json";"os";"testing")
func TestActualGeneratedDecoderAgainstIndependentCorpus(t *testing.T){
 data,err:=os.ReadFile("vectors.json");if err!=nil{t.Fatal(err)}
 var cases []struct{Source string;Valid bool;Normalized string};if err=json.Unmarshal(data,&cases);err!=nil{t.Fatal(err)}
 accepted,rejected:=0,0
 for i,c:=range cases{v,err:=auditContractRead([]byte(c.Source));if (err==nil)!=c.Valid{t.Fatalf("case %d acceptance error %v",i,err)};if !c.Valid{rejected++;continue};accepted++
  b,err:=json.Marshal(v);if err!=nil{t.Fatal(err)};if string(b)!=c.Normalized{t.Fatalf("case %d: %s want %s",i,b,c.Normalized)}
 }
 t.Logf("%d independent cases: %d accepted, %d rejected",len(cases),accepted,rejected)
}
`);
console.log(JSON.stringify({result:'PASS',directory:dir,cases:output.parityCases,mode:'actual CLI generated Go source, independent rational corpus; no developer vectors reused'}));
