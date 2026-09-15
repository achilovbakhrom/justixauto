package eventstore_test

import (
 "fmt"
 "os"
 "strings"
 "testing"
 "github.com/google/uuid"
)

// Independent QA probes reuse only the reviewed disposable fixture and SQL
// constructors. Each installation executes every real prior artifact unchanged.
// References are synthetic storage-shape evidence, not source/comparator authority.
func TestQAT928IndependentPostgres(t *testing.T) {
 if os.Getenv("JUSTIXAUTO_TEST_PROJECTION_GENERATION")!="1" { t.Skip("owned fixture opt-in") }
 f:=newQuarantineFixture(t)
 t.Run("parameter trigger bypass permission",func(t *testing.T){
  s:=generationStore(t,f)
  s.must(t,"postgres","GRANT SET ON PARAMETER session_replication_role TO "+s.runtime)
  t.Cleanup(func(){s.must(t,"postgres","REVOKE SET ON PARAMETER session_replication_role FROM "+s.runtime)})
  t.Log("effective SET privilege:",s.must(t,"postgres","SELECT has_parameter_privilege('"+s.runtime+"','session_replication_role','SET')"))
  out,err:=installGeneration(t,s)
  if err!=nil { t.Log("unsafe installer refused:",out); return }
  bad:=generationID()
  q:="BEGIN; SET LOCAL session_replication_role=replica; INSERT INTO eventstore.projection_heads(projection_name,epoch,active_generation_id) VALUES('"+bad.projection+"',77,'"+bad.generation+"'); COMMIT;"
  s.must(t,s.runtime,q)
  t.Log("committed head without generation FK/evidence:",s.must(t,s.runtime,"SELECT epoch||'|'||active_generation_id FROM eventstore.projection_heads WHERE projection_name='"+bad.projection+"'"))
  t.Error("installation accepted runtime trigger/FK bypass capability")
 })
 t.Run("required checkpoint update missing",func(t *testing.T){
  s:=generationStore(t,f)
  s.must(t,s.migration,"REVOKE UPDATE(position) ON eventstore.consumer_checkpoints FROM "+s.runtime)
  t.Log("effective UPDATE position:",s.must(t,s.migration,"SELECT has_column_privilege('"+s.runtime+"','eventstore.consumer_checkpoints','position','UPDATE')"))
  out,err:=installGeneration(t,s)
  if err==nil {t.Log(out);t.Error("installation accepted missing required checkpoint position UPDATE")}
 })
 t.Run("deferred flush allows multiple pointer changes",func(t *testing.T){
  s:=generationStore(t,f); mustGeneration(t,s)
  g:=generationID(); v:=generationPrepare(t,s,g)
  s.must(t,s.runtime,generationHeadSQL(g))
  sw:=uuid.NewString(); s.must(t,s.runtime,"BEGIN;"+generationSwitchSQL(g,sw,v,1,"")+"COMMIT;")
  one,two:=uuid.NewString(),uuid.NewString()
  step:=func(id,previous string,epoch int64)string{return generationEventSQL(g,id,previous,"hold",epoch,g.generation)+fmt.Sprintf("UPDATE eventstore.projection_heads SET epoch=%d,hold_ref='synthetic-hold',last_event_id='%s' WHERE projection_name='%s';",epoch+1,id,g.projection)}
  q:="BEGIN;"+step(one,sw,2)+"SET CONSTRAINTS ALL IMMEDIATE; SET CONSTRAINTS ALL DEFERRED;"+step(two,one,3)+"COMMIT;"
  out,err:=s.f.sql(s.database,s.runtime,q)
  if err==nil {t.Log(out);t.Log("final epoch/event:",s.must(t,s.runtime,"SELECT epoch||'|'||last_event_id FROM eventstore.projection_heads WHERE projection_name='"+g.projection+"'"));t.Error("two pointer changes committed in one transaction by flushing deferred evidence early")}
 })
 t.Run("early inactive hold becomes active without head hold",func(t *testing.T){
  s:=generationStore(t,f); mustGeneration(t,s)
  g:=generationID();s.must(t,s.runtime,generationSQL(g)+generationHeadSQL(g))
  hold,build,valid,sw:=uuid.NewString(),uuid.NewString(),uuid.NewString(),uuid.NewString()
  q:="BEGIN;"+generationEventSQL(g,hold,"","hold",0,"")+"SET CONSTRAINTS ALL IMMEDIATE; SET CONSTRAINTS ALL DEFERRED;"+generationEventSQL(g,build,hold,"build-progress",0,"")+generationEventSQL(g,valid,build,"validated",0,"")+generationSwitchSQL(g,sw,valid,1,"")+"COMMIT;"
  out,err:=s.f.sql(s.database,s.runtime,q)
  if err==nil {t.Log(out);t.Error("inactive hold final-state check was flushed before generation activated in the same transaction")}
 })
 t.Run("comparator and cursor mismatch all fields",func(t *testing.T){
  s:=generationStore(t,f);mustGeneration(t,s)
  cases:=[][2]string{{"'synthetic-comparator'","'other-comparator'"},{"'synthetic-comparator',1","'synthetic-comparator',2"},{"'synthetic-comparison'","'other-comparison'"},{"repeat('77',32)","repeat('78',32)"},{"'synthetic-history-cursor'","'other-cursor'"},{"repeat('66',32)","repeat('67',32)"},{"repeat('55',32)","repeat('56',32)"}}
  for _,c:=range cases {g:=generationID();v:=generationPrepare(t,s,g);s.must(t,s.runtime,generationHeadSQL(g));sw:=uuid.NewString();q:=strings.Replace(generationSwitchSQL(g,sw,v,1,""),c[0],c[1],1);s.reject(t,s.runtime,"BEGIN;"+q+"COMMIT;","");if got:=s.must(t,s.runtime,"SELECT epoch FROM eventstore.projection_heads WHERE projection_name='"+g.projection+"'");got!="1" {t.Fatal("mismatch changed pointer",got)}; if got:=s.must(t,s.runtime,"SELECT count(*) FROM eventstore.projection_generation_events WHERE event_id='"+sw+"'");got!="0" {t.Fatal("mismatch retained false receipt",got)}}
 })
 t.Run("duplicate request and one root retain original",func(t *testing.T){
  s:=generationStore(t,f);mustGeneration(t,s);g:=generationID();v:=generationPrepare(t,s,g)
  before:=s.must(t,s.runtime,"SELECT jsonb_agg(to_jsonb(e) ORDER BY event_id) FROM eventstore.projection_generation_events e")
  other:=generationID();other.request=g.request;s.reject(t,s.runtime,generationSQL(other),"duplicate key")
  s.reject(t,s.runtime,generationEventSQL(g,uuid.NewString(),"","build-progress",0,""),"duplicate key")
  next:=uuid.NewString();q:=strings.Replace(generationEventSQL(g,next,v,"hold",0,""),"'"+next+"',NULL", "'"+v+"',NULL",1)
  // Use a fixed reused request while preserving a new event ID.
  q=strings.Replace(generationEventSQL(g,next,v,"hold",0,""),"'"+g.generation+"','"+next+"'","'"+g.generation+"','"+v+"'",1)
  s.reject(t,s.runtime,q,"duplicate key")
  if after:=s.must(t,s.runtime,"SELECT jsonb_agg(to_jsonb(e) ORDER BY event_id) FROM eventstore.projection_generation_events e");after!=before {t.Fatal("duplicate changed immutable evidence")}
 })
 t.Run("all effective default families and schema additive",func(t *testing.T){
  for _,clause:=range []string{"USAGE ON TYPES","CREATE ON SCHEMAS","SELECT ON LARGE OBJECTS","IN SCHEMA eventstore GRANT SELECT ON TABLES"} {
   s:=generationStore(t,f);q:="ALTER DEFAULT PRIVILEGES GRANT "+clause+" TO "+s.runtime
   if strings.HasPrefix(clause,"IN SCHEMA") {q="ALTER DEFAULT PRIVILEGES "+clause+" TO "+s.runtime}
   // PostgreSQL 18 supports the object families used by the exact checker.
   s.must(t,s.migration,q);generationReject(t,s,generationScript(t))
  }
 })
}
