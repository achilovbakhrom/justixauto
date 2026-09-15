package eventstore_test

import (
 "fmt"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/google/uuid"
)

// QA-only overlay. Reuses the pinned disposable container helper, not the
// developer test scenarios. No files under application source are modified.
func TestQA917Independent(t *testing.T) {
 f := startFixture(t)
 const owner, migration, runtime = "inventory", "justix_inventory", "justix_inventory_runtime"
 f.mustSQL(owner,"postgres","CREATE ROLE "+runtime+" LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_inventory TO "+runtime)
 if out,err:=f.install(owner,runtime); err!=nil {t.Fatalf("base: %v %s",err,out)}
 script,err:=os.ReadFile("migrations/000002_messaging_delivery.up.sql"); if err!=nil {t.Fatal(err)}
 install:=func()(string,error){return f.sql(owner,migration,string(script),"-v","owner_service="+owner,"-v","runtime_role="+runtime)}
 t.Run("two-hop non-inherited default privilege rejects atomically",func(t *testing.T){
  f.mustSQL(owner,"postgres","CREATE ROLE qa917_middle; CREATE ROLE qa917_writer; GRANT qa917_middle TO "+runtime+" WITH INHERIT FALSE; GRANT qa917_writer TO qa917_middle WITH INHERIT FALSE")
  f.mustSQL(owner,migration,"ALTER DEFAULT PRIVILEGES GRANT TRUNCATE ON TABLES TO qa917_writer")
  out,err:=install(); if err==nil || !strings.Contains(out,"unexpected messaging table privileges") {t.Fatalf("accepted chain: %v %s",err,out)}
  if got:=f.mustSQL(owner,migration,"SELECT to_regclass('eventstore.messaging_mode') IS NULL");got!="t" {t.Fatal("partial schema")}
  f.mustSQL(owner,migration,"ALTER DEFAULT PRIVILEGES REVOKE TRUNCATE ON TABLES FROM qa917_writer")
 })
 if out,err:=install();err!=nil {t.Fatalf("migration: %v %s",err,out)}
 t.Run("cutover missing evidence rolls back mode and grants",func(t *testing.T){
  sql:=strings.Replace(messagingCutoverSQL(),"'synthetic-backup-reference'","''",1)
  out,err:=f.sql(owner,migration,sql); if err==nil || !strings.Contains(out,"check constraint") {t.Fatalf("missing evidence: %v %s",err,out)}
  if got:=f.mustSQL(owner,runtime,"SELECT mode||':'||(SELECT count(*) FROM eventstore.messaging_cutovers)||':'||has_table_privilege(current_user,'eventstore.outbox','INSERT') FROM eventstore.messaging_mode");got!="legacy:0:true" {t.Fatal(got)}
 })
 f.mustSQL(owner,migration,messagingCutoverSQL())
 sub:=func(name string,run func(schemaFixture)){t.Run(name,func(t *testing.T){local:=f;local.t=t;run(local)})}
 sub("complete two-destination plan and independent confirmation",func(f schemaFixture){
  stream,id,a,b:=uuid.NewString(),uuid.NewString(),uuid.NewString(),uuid.NewString()
  f.mustSQL(owner,runtime,messagingAdmission(a,stream,"source-recipient","retail")+messagingAdmission(b,stream,"source-recipient","financing"))
  plan:=fmt.Sprintf(`[{"destination":"financing","admission_id":"%s"},{"destination":"retail","admission_id":"%s"}]`,b,a)
  parent:=messagingParent(id,stream,1,plan)
  child:=func(dest,admit string)string{return fmt.Sprintf("INSERT INTO eventstore.outbox_deliveries(event_id,destination,admission_id,exchange,routing_key) VALUES('%s','%s','%s','justix.integration.v1','inventory.%s.fixture.changed.v1');",id,dest,admit,dest)}
  f.denied(owner,"BEGIN;"+parent+child("retail",a)+"SET CONSTRAINTS ALL IMMEDIATE; COMMIT;","incomplete or conflicting")
  f.mustSQL(owner,runtime,"BEGIN;"+parent+child("retail",a)+child("financing",b)+"SET CONSTRAINTS ALL IMMEDIATE; COMMIT;")
  f.mustSQL(owner,runtime,"UPDATE eventstore.outbox_deliveries SET sent_at=clock_timestamp() WHERE event_id='"+id+"' AND destination='retail'")
  if got:=f.mustSQL(owner,runtime,"SELECT count(*)||':'||count(sent_at) FROM eventstore.outbox_deliveries WHERE event_id='"+id+"'");got!="2:1" {f.t.Fatal(got)}
  f.denied(owner,"BEGIN;"+strings.Replace(parent,"'message'","'conflicting'",1)+"COMMIT;","check constraint")
  if got:=f.mustSQL(owner,runtime,"SELECT convert_from(envelope,'UTF8') FROM eventstore.outbox_messages WHERE event_id='"+id+"'");got!="message" {f.t.Fatal(got)}
 })
 sub("admission interval excludes checkpoint and includes upper boundary",func(f schemaFixture){
  stream,a:=uuid.NewString(),uuid.NewString()
  ad:=strings.Replace(messagingAdmission(a,stream,"source-recipient","retail"),"bootstrap_ref,start_after,consumer_kind","bootstrap_ref,start_after,end_inclusive,consumer_kind",1)
  ad=strings.Replace(ad,"'explicit-empty-stream',0,","'source-issued-checkpoint-5',5,7,",1)
  f.mustSQL(owner,runtime,ad)
  for _,seq:=range []int{5,6,7,8} {
   id:=uuid.NewString(); plan:=fmt.Sprintf(`[{"destination":"retail","admission_id":"%s"}]`,a)
   sql:="BEGIN;"+messagingParent(id,stream,seq,plan)+fmt.Sprintf("INSERT INTO eventstore.outbox_deliveries(event_id,destination,admission_id,exchange,routing_key) VALUES('%s','retail','%s','justix.integration.v1','inventory.retail.fixture.changed.v1'); COMMIT;",id,a)
   if seq==5||seq==8 {f.denied(owner,sql,"delivery admission mismatch")} else {f.mustSQL(owner,runtime,sql)}
  }
  if got:=f.mustSQL(owner,runtime,"SELECT string_agg(integration_sequence::text,',' ORDER BY integration_sequence) FROM eventstore.outbox_messages WHERE aggregate_id='"+stream+"'");got!="6,7" {f.t.Fatal(got)}
 })
 sub("append-only admission chain cannot cross stream or subject",func(f schemaFixture){
  stream,a:=uuid.NewString(),uuid.NewString(); f.mustSQL(owner,runtime,messagingAdmission(a,stream,"source-recipient","retail"))
  close:=fmt.Sprintf("INSERT INTO eventstore.messaging_admissions SELECT '%s',namespace,source_owner,aggregate_type,aggregate_id,subject,'close',admission_id,contract_id,contract_version,contract_digest,schema_allowlist,authority_ref,scope_ref,purpose,bootstrap_ref,start_after,7,consumer_kind,generation,created_at FROM eventstore.messaging_admissions WHERE admission_id='%s';",uuid.NewString(),a)
  f.denied(owner,strings.Replace(close,"aggregate_id,subject,","aggregate_id,'financing',",1),"different stream or subject")
  f.mustSQL(owner,runtime,close)
  f.denied(owner,"DELETE FROM eventstore.messaging_admissions WHERE admission_id='"+a+"'","permission denied")
 })
 sub("two local consumers preserve independent completion and rollback",func(f schemaFixture){
  stream,id,enrollment,a,b:=uuid.NewString(),uuid.NewString(),uuid.NewString(),uuid.NewString(),uuid.NewString()
  f.mustSQL(owner,runtime,messagingAdmission(a,stream,"local-consumer","qa.projection")+strings.Replace(messagingAdmission(b,stream,"local-consumer","qa.process"),"'projection'","'process'",1))
  message:=fmt.Sprintf("INSERT INTO eventstore.dispatch_messages(event_id,source_owner,aggregate_type,aggregate_id,integration_sequence,envelope,envelope_hash,exchange,routing_key,membership_version,admission_ref,bootstrap_ref,initial_enrollment_id) VALUES('%s','inventory','fixture','%s',1,'qa-custody',sha256('qa-custody'::bytea),'justix.integration.v1','inventory.inventory.fixture.changed.v1','qa-v1','qa-admission','qa-bootstrap','%s');",id,stream,enrollment)
  enroll:=fmt.Sprintf(`INSERT INTO eventstore.dispatch_enrollments VALUES('%s','%s','qa-v1','qa-cutover','[{"consumer_name":"qa.process","admission_id":"%s"},{"consumer_name":"qa.projection","admission_id":"%s"}]',clock_timestamp());`,id,enrollment,b,a)
  job:=func(name,admit,kind string)string{return fmt.Sprintf("INSERT INTO eventstore.dispatch_jobs(consumer_name,event_id,enrollment_id,admission_id,consumer_kind,generation) VALUES('%s','%s','%s','%s','%s','generation-1');",name,id,enrollment,admit,kind)}
  jobs:=job("qa.projection",a,"projection")+job("qa.process",b,"process")
  f.denied(owner,"BEGIN;"+strings.Replace(message,"inventory.inventory.fixture","inventory.retail.fixture",1)+enroll+jobs+"COMMIT;","invalid custody destination route")
  f.denied(owner,"BEGIN;"+message+enroll+strings.Replace(jobs,"generation-1","generation-2",1)+"COMMIT;","job admission mismatch")
  f.mustSQL(owner,runtime,"BEGIN;"+message+enroll+jobs+"COMMIT;")
  complete:=func(name string)string{return fmt.Sprintf("UPDATE eventstore.dispatch_jobs SET completed_at=clock_timestamp(),inbox_consumer_name='%s',inbox_event_id='%s' WHERE consumer_name='%s' AND event_id='%s';",name,id,name,id)}
  inbox:=fmt.Sprintf("INSERT INTO eventstore.inbox VALUES('qa.projection','%s',sha256('qa-custody'::bytea),clock_timestamp());",id)
  tx:="BEGIN; SET LOCAL justix.messaging_mode='custody';"+inbox+complete("qa.projection")
  f.mustSQL(owner,runtime,tx+"ROLLBACK;")
  if got:=f.mustSQL(owner,runtime,"SELECT count(*) FROM eventstore.inbox WHERE event_id='"+id+"'");got!="0" {f.t.Fatal(got)}
  f.mustSQL(owner,runtime,tx+"COMMIT;")
  f.denied(owner,complete("qa.process"),"matching inbox identity and bytes")
  if got:=f.mustSQL(owner,runtime,"SELECT count(*)||':'||count(completed_at) FROM eventstore.dispatch_jobs WHERE event_id='"+id+"'");got!="2:1" {f.t.Fatal(got)}
  f.denied(owner,"INSERT INTO eventstore.inbox VALUES('old-runtime','"+uuid.NewString()+"',sha256('old'::bytea),clock_timestamp())","explicit transaction adapter")
 })
 sub("runtime cannot disable triggers or alter immutable records",func(f schemaFixture){
  for _,sql:=range []string{"ALTER TABLE eventstore.outbox_messages DISABLE TRIGGER ALL","ALTER TABLE eventstore.dispatch_jobs DISABLE TRIGGER ALL","TRUNCATE eventstore.messaging_legacy_evidence","DELETE FROM eventstore.dispatch_enrollments","UPDATE eventstore.messaging_cutovers SET backup_ref='rewritten'"} {
   if out,err:=f.sql(owner,runtime,sql);err==nil {f.t.Fatalf("unexpected privilege: %s %s",sql,out)}
  }
  if out,err:=f.sql(owner,runtime,"SET ROLE qa917_writer; TRUNCATE eventstore.outbox_messages");err==nil {f.t.Fatalf("reachable role bypassed: %s",out)}
 })
 sub("cutover waits for legacy transaction and retains newly committed obligation",func(f schemaFixture){
  const owner,migration,runtime="commerce","justix_commerce","justix_commerce_runtime"
  f.mustSQL(owner,"postgres","CREATE ROLE "+runtime+" LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_commerce TO "+runtime)
  if out,err:=f.install(owner,runtime);err!=nil {f.t.Fatalf("base: %v %s",err,out)}
  if out,err:=f.sql(owner,migration,string(script),"-v","owner_service="+owner,"-v","runtime_role="+runtime);err!=nil {f.t.Fatalf("migration: %v %s",err,out)}
  id,stream:=uuid.NewString(),uuid.NewString()
  sql:=fmt.Sprintf("SET application_name='qa917-active-legacy'; BEGIN; INSERT INTO eventstore.outbox(event_id,aggregate_type,aggregate_id,integration_sequence,envelope,envelope_hash,exchange,routing_key) VALUES('%s','fixture','%s',1,'concurrent-legacy',sha256('concurrent-legacy'::bytea),'justix.integration.v1','commerce.retail.fixture.changed.v1'); SELECT pg_sleep(2); COMMIT;",id,stream)
  done:=make(chan error,1);go func(){out,err:=f.sql(owner,runtime,sql);if err!=nil {err=fmt.Errorf("%w: %s",err,out)};done<-err}()
  deadline:=time.Now().Add(5*time.Second)
  for {
   if f.mustSQL(owner,"postgres","SELECT count(*) FROM pg_stat_activity WHERE application_name='qa917-active-legacy' AND wait_event='PgSleep'")=="1" {break}
   if time.Now().After(deadline) {f.t.Fatal("legacy transaction did not reach synchronization point")};time.Sleep(20*time.Millisecond)
  }
  out,err:=f.sql(owner,migration,messagingCutoverSQL())
  if err==nil || !strings.Contains(out,"unrecognized or unauthorized legacy route") {f.t.Fatalf("cutover missed concurrent obligation: %v %s",err,out)}
  if err:=<-done;err!=nil {f.t.Fatal(err)}
  if got:=f.mustSQL(owner,runtime,"SELECT mode||':'||(SELECT count(*) FROM eventstore.outbox)||':'||(SELECT count(*) FROM eventstore.messaging_legacy_evidence) FROM eventstore.messaging_mode");got!="legacy:1:0" {f.t.Fatal(got)}
  f.mustSQL(owner,migration,fmt.Sprintf("INSERT INTO eventstore.messaging_legacy_authorizations(event_id,destination,hold_ref,authority_ref) VALUES('%s','retail','qa-historical-authority-unavailable','qa-reviewed-route');",id)+messagingCutoverSQL())
  if got:=f.mustSQL(owner,runtime,"SELECT convert_from(m.envelope,'UTF8')||':'||d.hold_ref FROM eventstore.outbox_messages m JOIN eventstore.outbox_deliveries d USING(event_id)");got!="concurrent-legacy:qa-historical-authority-unavailable" {f.t.Fatal(got)}
 })
}
