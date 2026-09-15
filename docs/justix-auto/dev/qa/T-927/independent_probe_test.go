package eventstore_test

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "strings"
 "testing"

 "github.com/google/uuid"
 "gorm.io/driver/postgres"
 "gorm.io/gorm"
 "gorm.io/gorm/logger"
 "justixauto/pkg/eventstore"
)

// QA overlay: independent assertions against exact committed SQL; existing
// helpers supply only synthetic values and isolated database installation.
func TestQA927Adversarial(t *testing.T) {
 if os.Getenv("JUSTIXAUTO_TEST_QUARANTINE")!="1" { t.Skip("explicit fixture opt-in required") }
 var container string
 // Registered first, so this independent absence check runs after fixture cleanup.
 t.Cleanup(func(){ if container!="" { out,err:=exec.Command("docker","inspect",container).CombinedOutput(); if err==nil || !strings.Contains(strings.ToLower(string(out)),"no such object") { t.Errorf("QA fixture absence not verified: %v %s",err,out) } else { t.Logf("QA fixture verified absent: %s",container) } } })
 f:=newQuarantineFixture(t); container=f.container
 out,err:=exec.Command("docker","inspect","--format","{{.Id}} {{.Config.Image}} {{json .Mounts}} {{json .HostConfig.Tmpfs}}",container).CombinedOutput(); if err!=nil {t.Fatal(err)}; t.Logf("QA fixture identity: %s",out)
 reject:=func(t *testing.T,s *routeStore,q string){t.Helper(); before:=s.must(t,s.runtime,"SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY action_id),'[]') FROM eventstore.quarantine_actions t"); out,err:=f.sql(s.database,s.runtime,q); if err==nil {t.Fatalf("adversarial SQL unexpectedly accepted: %s; %s",q,out)}; after:=s.must(t,s.runtime,"SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY action_id),'[]') FROM eventstore.quarantine_actions t"); if before!=after {t.Fatal("rejected action changed chain")}; t.Logf("rejection retained chain: %s",strings.TrimSpace(out))}

 t.Run("nullable shapes exact request context and immutable content",func(t *testing.T){
  s:=quarantineStore(t,f);mustQuarantine(t,s)
  raw:=[]byte{0xff,0,0xc0,0xaf,'{','"'};e:=fixtureSeal(raw,"QA-exact-context");s.must(t,s.runtime,evidenceSQL(e))
  before:=s.must(t,s.runtime,"SELECT to_jsonb(t) FROM eventstore.quarantine_evidence t")
  root:=uuid.NewString();s.must(t,s.runtime,actionSQL(e.id,root,"",uuid.NewString(),"redrive-request","direct-handler","qa-consumer","requested"))
  ev:=uuid.NewString();hash:=fmt.Sprintf("%x",sha256.Sum256([]byte("QA-authorized-correction-fixture")))
  s.must(t,s.runtime,fmt.Sprintf("INSERT INTO eventstore.inbox(consumer_name,event_id,envelope_hash) VALUES('qa-consumer','%s',decode('%s','hex'))",ev,hash))
  valid:=acceptedActionSQL(e.id,root,"direct-handler","qa-consumer","inbox",ev,hash)
  for _,field:=range []string{"synthetic-actor","synthetic-authority","synthetic-scope","synthetic-purpose","synthetic-no-correction"} {t.Run(field,func(t *testing.T){reject(t,s,strings.Replace(valid,"'"+field+"'","'foreign-context'",1))})}
  for name,q:=range map[string]string{
   "null accepted event":strings.Replace(valid,"'accepted','"+ev+"'","'accepted',NULL",1),
   "null hash":strings.Replace(valid,"decode('"+hash+"','hex')","NULL",1),
   "null receipt kind":strings.Replace(valid,"'inbox'","NULL",1),
   "null receipt consumer":strings.Replace(valid,"'inbox','qa-consumer'","'inbox',NULL",1),
   "foreign receipt consumer":strings.Replace(valid,"'inbox','qa-consumer'","'inbox','other-consumer'",1),
   "receipt event differs":strings.Replace(valid,"'inbox','qa-consumer','"+ev+"'","'inbox','qa-consumer','"+uuid.NewString()+"'",1),
   "released direct":actionSQL(e.id,uuid.NewString(),root,uuid.NewString(),"redrive-result","direct-handler","qa-consumer","released"),
  } {t.Run(name,func(t *testing.T){reject(t,s,q)})}
  s.must(t,s.runtime,valid)
  reject(t,s,valid)
  for _,role:=range []string{s.runtime,s.migration} {for _,q:=range []string{"UPDATE eventstore.quarantine_actions SET request_id='"+uuid.NewString()+"'","DELETE FROM eventstore.quarantine_actions","UPDATE eventstore.quarantine_evidence SET sealed_ciphertext=sealed_ciphertext","TRUNCATE eventstore.quarantine_actions"} {if out,err:=f.sql(s.database,role,q);err==nil {t.Fatal("immutable DML accepted",q,out)}}}
  if after:=s.must(t,s.runtime,"SELECT to_jsonb(t) FROM eventstore.quarantine_evidence t");before!=after {t.Fatal("protected evidence changed")}
  if n:=s.must(t,s.runtime,"SELECT count(*) FROM eventstore.consumer_checkpoints");n!="0" {t.Fatal("audit invented checkpoint")}
 })

 t.Run("custody receipt cannot substitute another job or early inbox",func(t *testing.T){
  s:=quarantineStore(t,f);r:=s.produce(t,"inventory")[0];quarantineCutover(t,s,r);mustQuarantine(t,s)
  for _,c:=range []string{"qa-first","qa-second"} {s.must(t,s.runtime,"BEGIN;"+routeAdmission(uuid.NewString(),r.Stream,"local-consumer",c,"inventory.fixture.changed.v1")+"COMMIT;")}
  admission:=s.must(t,s.runtime,"SELECT admission_id FROM eventstore.messaging_admissions WHERE namespace='local-consumer' AND subject='qa-first'")
  // This fixture enrolls one exact job. The other admitted consumer is not a
  // receipt for that obligation; full production membership remains T-921.
  s.must(t,s.runtime,"BEGIN;"+routeCustody(r,admission,"qa-first","generation-1")+"COMMIT;")
  e:=fixtureSeal(r.Body,"QA-custody");s.must(t,s.runtime,custodyEvidenceSQL(e,r,"qa-first","unfinished"))
  req:=uuid.NewString();s.must(t,s.runtime,actionSQL(e.id,req,"",uuid.NewString(),"redrive-request","custody-handler","qa-first","requested"))
  s.must(t,s.runtime,fmt.Sprintf("BEGIN;SET LOCAL justix.messaging_mode='custody';INSERT INTO eventstore.inbox(consumer_name,event_id,envelope_hash) VALUES('qa-first','%s',decode('%s','hex'));COMMIT;",r.ID,hex.EncodeToString(r.Hash)))
  reject(t,s,acceptedActionSQL(e.id,req,"custody-handler","qa-first","job",r.ID,hex.EncodeToString(r.Hash)))
  reject(t,s,acceptedActionSQL(e.id,req,"custody-handler","qa-first","inbox",r.ID,hex.EncodeToString(r.Hash)))
  reject(t,s,acceptedActionSQL(e.id,req,"custody-handler","qa-first","custody",r.ID,hex.EncodeToString(r.Hash)))
  reject(t,s,actionSQL(e.id,uuid.NewString(),req,uuid.NewString(),"redrive-result","custody-handler","qa-second","released"))
  if got:=s.must(t,s.runtime,"SELECT count(*) FROM eventstore.dispatch_jobs WHERE completed_at IS NOT NULL OR quarantine_ref IS NOT NULL");got!="0" {t.Fatal("audit forged job progress",got)}
 })

 t.Run("action commit lost reply exact receipt and rollback",func(t *testing.T){
  s:=quarantineStore(t,f);mustQuarantine(t,s)
  for _,committed:=range []bool{false,true} {
   e:=fixtureSeal(nil,"QA-action-unknown");s.must(t,s.runtime,evidenceSQL(e));id,request:=uuid.NewString(),uuid.NewString()
   q:=actionSQL(e.id,id,"",request,"redrive-request","intake","","requested")
   pool,err:=s.db.DB();if err!=nil {t.Fatal(err)}
   fault,err:=gorm.Open(postgres.New(postgres.Config{Conn:commitFaultPool{DB:pool,commitFirst:committed}}),&gorm.Config{Logger:logger.Default.LogMode(logger.Silent)});if err!=nil{t.Fatal(err)}
   runner,err:=eventstore.NewTransactions(fault,func(tx *gorm.DB)(*gorm.DB,error){return tx,nil});if err!=nil{t.Fatal(err)}
   calls:=0;err=runner.Run(context.Background(),func(tx *gorm.DB)error{calls++;return tx.Exec(q).Error})
   if !errors.Is(err,eventstore.ErrCommitOutcomeUnknown)||calls!=1 {t.Fatalf("unknown outcome auto-retry: %v %d",err,calls)}
   want:="0";if committed{want="1"}
   where:=fmt.Sprintf("request_id='%s' AND action_id='%s' AND evidence_id='%s' AND authority_ref='synthetic-authority' AND intended_path='intake' AND outcome_code='requested'",request,id,e.id)
   if got:=s.must(t,s.runtime,"SELECT count(*) FROM eventstore.quarantine_actions WHERE "+where);got!=want {t.Fatalf("actual commit %v receipt %s",committed,got)}
   if got:=s.must(t,s.runtime,"SELECT count(*) FROM eventstore.quarantine_actions WHERE "+where+" AND scope_ref='foreign'");got!="0"{t.Fatal("wrong context reconciled")}
   if committed {reject(t,s,q)} else {s.must(t,s.runtime,q)} // explicit fresh reconcile, never an automatic retry
   if got:=s.must(t,s.runtime,"SELECT count(*) FROM eventstore.quarantine_actions WHERE request_id='"+request+"'");got!="1"{t.Fatal("duplicate action",got)}
  }
 })

 t.Run("late installation rollback and independent escalation probes",func(t *testing.T){
  s:=quarantineStore(t,f);before,cat,fn:=quarantinePreimage(t,s),s.catalog(t),s.functions(t,true)
  script:=strings.Replace(quarantineScript(t),"COMMIT;","SELECT 1/0;\nCOMMIT;",1)
  out,err:=s.install(script,"-v","base_schema_sha256="+checkpointBaseHash,"-v","checkpoint_migration_sha256="+quarantineCheckpointHash,"-v",fmt.Sprintf("quarantine_migration_sha256=%x",sha256.Sum256([]byte(script))))
  if err==nil||!strings.Contains(out,"division by zero"){t.Fatalf("late failure not exercised: %v %s",err,out)}
  if quarantinePreimage(t,s)!=before||s.catalog(t)!=cat||s.functions(t,true)!=fn{t.Fatal("late failure changed retained catalog or rows")}
  if got:=s.must(t,s.migration,"SELECT (SELECT count(*) FROM pg_class WHERE relnamespace='eventstore'::regnamespace AND relname LIKE 'quarantine_%')+(SELECT count(*) FROM pg_proc WHERE pronamespace='eventstore'::regnamespace AND proname LIKE 'quarantine_%')");got!="0"{t.Fatal("partial late DDL",got)}
  for name,change:=range map[string]func(*routeStore)string{
   "MAINTAIN":func(s *routeStore)string{return "GRANT MAINTAIN ON eventstore.inbox TO "+s.runtime},
   "database CREATE":func(s *routeStore)string{return "GRANT CREATE ON DATABASE "+s.database+" TO "+s.runtime},
   "column grant option":func(s *routeStore)string{return "GRANT SELECT(position) ON eventstore.consumer_checkpoints TO "+s.runtime+" WITH GRANT OPTION"},
   "default EXECUTE":func(s *routeStore)string{return "ALTER DEFAULT PRIVILEGES IN SCHEMA eventstore GRANT EXECUTE ON FUNCTIONS TO "+s.runtime},
   "trigger definer":func(s *routeStore)string{return "ALTER FUNCTION eventstore.consumer_checkpoint_guard() SECURITY DEFINER"},
  }{t.Run(name,func(t *testing.T){s:=quarantineStore(t,f);s.must(t,s.migration,change(s));quarantineRejectInstall(t,s)})}
 })
}
