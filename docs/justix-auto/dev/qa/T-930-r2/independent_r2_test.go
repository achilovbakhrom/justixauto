package persistence_test

import (
 "context"
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "justixauto/pkg/persistence"
)

func TestIndependentR2RuntimeAuthority(t *testing.T) {
 if os.Getenv("JUSTIXAUTO_TEST_PERSISTENCE_PROFILE")!="1" {t.Skip("owned PostgreSQL fixture")}
 f:=startFixture(t);p:=profile(t,f.spec)
 f.retain("r2-initial.json",f.snapshot())
 for _, privilege:=range []string{"SET","ALTER SYSTEM"} {
  for _, target:=range []string{"justix_identity_runtime","PUBLIC","qa_parameter_inner"} {t.Run(privilege+"/"+target,func(t *testing.T){
   f.t=t
   if target=="qa_parameter_inner" {f.must("CREATE ROLE qa_parameter_outer NOINHERIT;CREATE ROLE qa_parameter_inner NOINHERIT;GRANT qa_parameter_inner TO qa_parameter_outer;GRANT qa_parameter_outer TO justix_identity_runtime","postgres");defer f.must("REVOKE qa_parameter_outer FROM justix_identity_runtime;DROP ROLE qa_parameter_outer;DROP ROLE qa_parameter_inner","postgres")}
   f.must("GRANT "+privilege+" ON PARAMETER session_replication_role TO "+target,"postgres")
   defer f.must("REVOKE "+privilege+" ON PARAMETER session_replication_role FROM "+target,"postgres")
   f.check(p,false)
   if target=="qa_parameter_inner" {got:=f.must("BEGIN;SET LOCAL ROLE qa_parameter_inner;SELECT has_parameter_privilege(current_user,'session_replication_role','"+privilege+"');ROLLBACK","justix_identity_runtime");if got!="t"{t.Fatal("SET ROLE reachability not established",got)}}
  });f.t=t;f.check(p,true)}
 }
 for _, family:=range []struct{object,priv string}{{"TABLES","INSERT"},{"SEQUENCES","UPDATE"},{"FUNCTIONS","EXECUTE"},{"TYPES","USAGE"},{"SCHEMAS","CREATE"},{"LARGE OBJECTS","UPDATE"}} {t.Run("unrelated grantee defaults/"+family.object,func(t *testing.T){
  f.t=t;f.must("CREATE ROLE qa_unrelated_default","postgres");defer f.must("DROP ROLE qa_unrelated_default","postgres")
  f.must("ALTER DEFAULT PRIVILEGES GRANT "+family.priv+" ON "+family.object+" TO qa_unrelated_default")
  defer f.must("ALTER DEFAULT PRIVILEGES REVOKE "+family.priv+" ON "+family.object+" FROM qa_unrelated_default")
  f.check(p,false)
 });f.t=t;f.check(p,true)}
 t.Run("source session replication mode cannot be inherited at login",func(t *testing.T){
  f.t=t;f.must("ALTER ROLE justix_identity_runtime IN DATABASE justix_identity SET session_replication_role=replica","postgres")
  defer f.must("ALTER ROLE justix_identity_runtime IN DATABASE justix_identity RESET session_replication_role","postgres")
  // Existing checked sessions are origin. New sessions inherit replica even
  // without parameter SET privilege; close the pool so Check must reconnect.
  pool,err:=f.runtime.DB();if err!=nil{t.Fatal(err)};pool.SetMaxIdleConns(0)
  if mode:=f.must("SELECT current_setting('session_replication_role')","justix_identity_runtime");mode!="replica"{t.Fatal("probe did not select replica")}
  if persistence.Check(context.Background(),f.runtime,p)==nil{t.Fatal("inherited replica mode accepted")}
 })
 f.t=t;f.check(p,true);f.retain("r2-final.json",f.snapshot())
}

func TestIndependentR2ManifestDigestShape(t *testing.T){
 for _, location:=range []string{"artifact","prerequisite","feature"}{for _, shape:=range []string{"valid","extra byte","extra null","extra object","null byte","missing byte"}{t.Run(location+"/"+shape,func(t *testing.T){
  root,err:=filepath.EvalSymlinks(t.TempDir());if err!=nil{t.Fatal(err)};if err=os.Mkdir(filepath.Join(root,"migrations"),0700);err!=nil{t.Fatal(err)}
  s:=spec()
  // A nonzero digest ending in zero makes missing/null byte coercion visible.
  // Keep SQL identity hashes real; the feature digest has no SQL file of its own.
  if location=="feature"{s.Artifacts[1].Feature.SHA256[31]=0;s.Artifacts[1].Feature.SHA256[0]=0;s.Features[0].Identity=*s.Artifacts[1].Feature}
  for i,a:=range s.Artifacts{
   body:="base";if i==1{body="feature12"};if err=os.WriteFile(filepath.Join(root,a.Identity.Filename),[]byte(body),0600);err!=nil{t.Fatal(err)}
   b,err:=json.Marshal(persistence.ArtifactManifest{FormatRevision:1,Owner:s.Owner,Identity:a.Identity,Prerequisites:a.Prerequisites,Feature:a.Feature});if err!=nil{t.Fatal(err)}
   if i==1{
    var m map[string]any;if err=json.Unmarshal(b,&m);err!=nil{t.Fatal(err)}
    var item map[string]any
    switch location{case "artifact":item=m["artifact"].(map[string]any);case "prerequisite":item=m["prerequisites"].([]any)[0].(map[string]any);case "feature":item=m["feature_contract"].(map[string]any)}
    a:=item["SHA256"].([]any)
    switch shape{case "extra byte":a=append(a,float64(255));case "extra null":a=append(a,nil);case "extra object":a=append(a,map[string]any{"ignored":true});case "null byte":a[0]=nil;case "missing byte":a=a[:31]}
    item["SHA256"]=a;b,err=json.Marshal(m);if err!=nil{t.Fatal(err)}
   }
   s.Artifacts[i].ManifestSHA256=hash(string(b));if err=os.WriteFile(filepath.Join(root,strings.TrimSuffix(a.Identity.Filename,".up.sql")+".manifest.json"),b,0600);err!=nil{t.Fatal(err)}
  }
  err=profile(t,s).VerifyFiles(root)
  if shape=="valid" {if err!=nil{t.Fatal(err)}} else if err==nil{t.Fatal("digest must contain exactly 32 integer bytes; malformed typed value was silently normalized")}
 })}}
}
