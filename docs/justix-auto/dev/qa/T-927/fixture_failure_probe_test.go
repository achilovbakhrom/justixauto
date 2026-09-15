package eventstore_test

import (
 "encoding/json"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "strings"
 "testing"
)

// The wrapper executes only this test's new UUID fixture, captures its original
// name argument and actual returned ID, then loses the successful start reply.
// Parent cleanup is registered before starting the child and validates both
// exact ID and UUID name before removing any object.
func TestQA927FixtureLostStartReply(t *testing.T) {
 if os.Getenv("JUSTIXAUTO_TEST_QUARANTINE")!="1" {t.Skip("explicit fixture opt-in required")}
 if os.Getenv("QA927_START_CHILD")=="1" {newQuarantineFixture(t);t.Fatal("expected synthetic start failure")}
 docker,err:=exec.LookPath("docker");if err!=nil {t.Fatal(err)}
 dir:=t.TempDir();capture:=filepath.Join(dir,"owned-id");namefile:=filepath.Join(dir,"owned-name")
 quote:=func(s string)string{return "'"+strings.ReplaceAll(s,"'","'\\''")+"'"}
 wrapper:="#!/bin/sh\nif [ \"$1\" = run ]; then\n  previous=\n  for argument in \"$@\"; do\n    if [ \"$previous\" = --name ]; then printf '%s\\n' \"$argument\" > "+quote(namefile)+"; fi\n    previous=$argument\n  done\n  "+quote(docker)+" \"$@\" > "+quote(capture)+"\n  status=$?\n  cat "+quote(capture)+"\n  if [ \"$status\" = 0 ]; then echo 'QA synthetic successful-start reply lost' >&2; exit 42; fi\n  exit \"$status\"\nfi\nexec "+quote(docker)+" \"$@\"\n"
 if err:=os.WriteFile(filepath.Join(dir,"docker"),[]byte(wrapper),0700);err!=nil{t.Fatal(err)}
 var ownedID,ownedName string
 t.Cleanup(func(){
  if ownedID=="" {return}
  // Names are derived from the intercepted command's --name, never stderr.
  out,err:=exec.Command(docker,"inspect",ownedID).CombinedOutput();if err!=nil {t.Errorf("owned fixture unexpectedly unavailable before parent cleanup: %v %s",err,out);return}
  var values []struct{ID string `json:"Id"`;Name string;Mounts []struct{Type string};Config struct{Image string}}
  if err:=json.Unmarshal(out,&values);err!=nil||len(values)!=1 {t.Errorf("cannot validate owned fixture: %v",err);return}
  v:=values[0]
  if v.ID!=ownedID||v.Name!="/"+ownedName||v.Config.Image!="docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2" {t.Error("owned fixture identity mismatch; no removal attempted");return}
  for _,m:=range v.Mounts {if m.Type=="volume"{t.Error("unexpected volume; no removal attempted");return}}
  if out,err:=exec.Command(docker,"rm","-f","-v",ownedID).CombinedOutput();err!=nil{t.Errorf("parent cleanup: %v %s",err,out);return}
  if out,err:=exec.Command(docker,"inspect",ownedID).CombinedOutput();err==nil||!strings.Contains(strings.ToLower(string(out)),"no such object"){t.Errorf("parent cleanup absence not proven: %v %s",err,out)}else{t.Logf("QA parent removed and verified absent: %s %s",ownedID,ownedName)}
 })
 child:=exec.Command(os.Args[0],"-test.run=^TestQA927FixtureLostStartReply$","-test.v")
 child.Env=append(os.Environ(),"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),"QA927_START_CHILD=1")
 out,childErr:=child.CombinedOutput();t.Logf("child result: %v\n%s",childErr,out)
 idBytes,idErr:=os.ReadFile(capture);nameBytes,nameErr:=os.ReadFile(namefile)
 if idErr!=nil||nameErr!=nil {t.Fatalf("fixture capture unavailable: %v %v",idErr,nameErr)}
 id,name:=strings.TrimSpace(string(idBytes)),strings.TrimSpace(string(nameBytes))
 if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(id)||!regexp.MustCompile(`^justixauto-t927-[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`).MatchString(name){t.Fatal("invalid captured owned identity")}
 ownedID,ownedName=id,name
 if childErr==nil||!strings.Contains(string(out),"QA synthetic successful-start reply lost"){t.Fatal("synthetic failed-start path not exercised")}
 inspect,inspectErr:=exec.Command(docker,"inspect","--format","{{.Id}} {{.Name}} {{.State.Running}} {{json .Mounts}} {{json .HostConfig.Tmpfs}}",id).CombinedOutput()
 if inspectErr==nil {t.Errorf("BOUNCE: fixture child returned after failed start but left its container allocated: %s",inspect)}else{t.Logf("fixture child cleaned up after failed start: %v %s",inspectErr,inspect);ownedID=""}
}
