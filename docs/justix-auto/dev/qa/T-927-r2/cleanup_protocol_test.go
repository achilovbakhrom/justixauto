package eventstore_test

import (
 "context"
 "encoding/json"
 "os"
 "os/exec"
 "path/filepath"
 "reflect"
 "strings"
 "testing"
 "time"

 "github.com/google/uuid"
)

// No Docker daemon calls: a task-local CLI double records every operation and
// emulates only inspect/remove replies. Live lost-reply paths run separately
// through the original unchanged QA overlay and exact committed fixture tests.
func TestQA927R2CleanupProtocol(t *testing.T) {
 if os.Getenv("QA927_R2_CLEANUP_CHILD")=="1" {
  quarantineCleanup(t,os.Getenv("QA927_R2_CLI"),os.Getenv("QA927_R2_NAME"),os.Getenv("QA927_R2_TOKEN"))
  return
 }
 name,token,id:="justixauto-t927-"+uuid.NewString(),uuid.NewString(),strings.Repeat("a",64)
 cases:=[]string{"valid","absent","name","label","image","id","volume","bind","extra-tmpfs","missing-tmpfs","inspect-error","invalid-json","still-present"}
 for _,mode:=range cases {t.Run(mode,func(t *testing.T){
  dir:=t.TempDir();cli:=filepath.Join(dir,"fixture-cli");log:=filepath.Join(dir,"calls.jsonl")
  data:=map[string]any{"id":id,"name":"/"+name,"image":quarantineImage,"labels":map[string]string{quarantineFixtureLabel:token},"mounts":[]any{},"tmpfs":map[string]string{"/var/lib/postgresql":"rw"}}
  switch mode {
  case "name":data["name"]="/unrelated-name"
  case "label":data["labels"]=map[string]string{quarantineFixtureLabel:uuid.NewString()}
  case "image":data["image"]="postgres:latest"
  case "id":data["id"]="invalid-ID"
  case "volume","bind":data["mounts"]=[]any{map[string]string{"Type":mode,"Destination":"/var/lib/postgresql"}}
  case "extra-tmpfs":data["tmpfs"]=map[string]string{"/var/lib/postgresql":"rw","/other":"rw"}
  case "missing-tmpfs":data["tmpfs"]=map[string]string{}
  }
  body,err:=json.Marshal(data);if err!=nil{t.Fatal(err)}
  if err:=os.WriteFile(filepath.Join(dir,"inspect.json"),body,0600);err!=nil{t.Fatal(err)}
  code:=`#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
root=Path(__file__).parent
args=sys.argv[1:]
with (root/'calls.jsonl').open('a') as f: f.write(json.dumps(args)+'\n')
mode=os.environ['QA927_R2_MODE']
if args[0]=='rm':
    (root/'removed').write_text('yes')
    sys.exit(0)
if args[0]!='inspect': sys.exit(91)
if mode=='inspect-error':
    print('synthetic daemon connection refused',file=sys.stderr)
    sys.exit(1)
if mode=='absent' or ((root/'removed').exists() and mode!='still-present'):
    print('Error: No such object: '+args[-1],file=sys.stderr)
    sys.exit(1)
if mode=='invalid-json': print('{invalid')
else: print((root/'inspect.json').read_text())
`
  if err:=os.WriteFile(cli,[]byte(code),0700);err!=nil{t.Fatal(err)}
  ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
  child:=exec.CommandContext(ctx,os.Args[0],"-test.run=^TestQA927R2CleanupProtocol$","-test.v")
  child.Env=append(os.Environ(),"QA927_R2_CLEANUP_CHILD=1","QA927_R2_CLI="+cli,"QA927_R2_NAME="+name,"QA927_R2_TOKEN="+token,"QA927_R2_MODE="+mode)
  out,err:=child.CombinedOutput();t.Logf("expected child outcome for %s: %v\n%s",mode,err,out)
  wantSuccess:=mode=="valid"||mode=="absent"
  if (err==nil)!=wantSuccess {t.Fatalf("cleanup outcome %s: %v %s",mode,err,out)}
  recorded,err:=os.ReadFile(log);if err!=nil{t.Fatal(err)}
  var calls [][]string
  for _,line:=range strings.Split(strings.TrimSpace(string(recorded)),"\n") {var args []string;if err:=json.Unmarshal([]byte(line),&args);err!=nil{t.Fatal(err)};calls=append(calls,args)}
  var removes [][]string;var inspected []string
  for _,call:=range calls {if call[0]=="rm"{removes=append(removes,call)};if call[0]=="inspect"{inspected=append(inspected,call[len(call)-1])}}
  if mode=="valid"||mode=="still-present" {
   if !reflect.DeepEqual(removes,[][]string{{"rm","-f",id}}){t.Fatalf("removal did not use exact immutable ID: %#v",removes)}
   want:=[]string{name,id,name};if mode=="still-present"{want=[]string{name,id}}
   if !reflect.DeepEqual(inspected,want){t.Fatalf("missing post-removal identity checks: %#v",inspected)}
  }else if len(removes)!=0 {t.Fatalf("unsafe removal in %s: %#v",mode,removes)}
  t.Logf("CLI protocol verified: %s",recorded)
 })}
}
