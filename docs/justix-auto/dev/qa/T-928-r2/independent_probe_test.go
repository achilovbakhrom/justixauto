package eventstore_test

import (
 "fmt"
 "os"
 "strings"
 "testing"
 "time"
 "github.com/google/uuid"
)

// New r2 probes. Old independent probes remain unchanged in their own overlay.
// Each fixture is freshly allocated through the independently reviewed lifecycle.
func TestQAT928R2ServerDefaultPostgres(t *testing.T) {
 if os.Getenv("JUSTIXAUTO_TEST_PROJECTION_GENERATION")!="1" {t.Skip("owned fixture opt-in")}
 f:=newQuarantineFixture(t);s:=generationStore(t,f)
 // Change only this disposable server. The migration login has an explicit safe
 // override, whereas the runtime inherits the unsafe server-wide default.
 s.must(t,"postgres","ALTER ROLE "+s.migration+" SET session_replication_role=origin")
 s.must(t,"postgres","ALTER SYSTEM SET session_replication_role=replica")
 s.must(t,"postgres","SELECT pg_reload_conf()")
 t.Cleanup(func(){s.must(t,"postgres","ALTER SYSTEM RESET session_replication_role");s.must(t,"postgres","SELECT pg_reload_conf()")})
 deadline:=time.Now().Add(5*time.Second)
 for s.must(t,s.runtime,"SHOW session_replication_role")!="replica" {if time.Now().After(deadline){t.Fatal("server reload did not take effect")};time.Sleep(20*time.Millisecond)}
 t.Log("migration login mode:",s.must(t,s.migration,"SHOW session_replication_role"))
 t.Log("runtime login mode:",s.must(t,s.runtime,"SHOW session_replication_role"))
 t.Log("runtime SET authority:",s.must(t,"postgres","SELECT has_parameter_privilege('"+s.runtime+"','session_replication_role','SET')"))
 out,err:=installGeneration(t,s)
 if err!=nil {t.Log("unsafe server default rejected:",out);return}
 g:=generationID()
 s.must(t,s.runtime,fmt.Sprintf("INSERT INTO eventstore.projection_heads(projection_name,epoch,active_generation_id) VALUES('%s',73,'%s')",g.projection,g.generation))
 t.Log("committed missing-generation head:",s.must(t,s.runtime,"SELECT epoch||'|'||active_generation_id FROM eventstore.projection_heads WHERE projection_name='"+g.projection+"'"))
 t.Error("installer origin override hid server-wide replica default inherited by runtime without SET permission")
}

func TestQAT928R2ProvenancePostgres(t *testing.T) {
 if os.Getenv("JUSTIXAUTO_TEST_PROJECTION_GENERATION")!="1" {t.Skip("owned fixture opt-in")}
 f:=newQuarantineFixture(t);s:=generationStore(t,f);mustGeneration(t,s)
 t.Run("extreme supplied xid ignored and exact types readonly",func(t *testing.T){
  g:=generationID(); b:=uuid.NewString()
  q:=generationEventSQL(g,b,"","build-progress",0,"")
  q=strings.Replace(q,"hold_ref)","hold_ref,created_xid)",1)
  q=strings.TrimSuffix(q,"NULL);")+"NULL,'18446744073709551615'::xid8);"
  head:=fmt.Sprintf("INSERT INTO eventstore.projection_heads(projection_name,epoch,last_mutation_xid) VALUES('%s',1,pg_current_xact_id());",g.projection)
  got:=s.must(t,s.runtime,"BEGIN;"+generationSQL(g)+q+head+"SELECT e.created_xid=pg_current_xact_id() AND h.last_mutation_xid IS NULL FROM eventstore.projection_generation_events e JOIN eventstore.projection_heads h USING(projection_name) WHERE e.event_id='"+b+"';COMMIT;")
  if got!="t" {t.Fatal("spoofed xid retained",got)}
  got=s.must(t,s.runtime,"SELECT string_agg(attname||':'||atttypid::regtype||':'||attnotnull,',' ORDER BY attname) FROM pg_attribute WHERE (attrelid='eventstore.projection_generation_events'::regclass AND attname='created_xid') OR (attrelid='eventstore.projection_heads'::regclass AND attname='last_mutation_xid')")
  if got!="created_xid:xid8:true,last_mutation_xid:xid8:false" {t.Fatal("provenance types",got)}
  s.reject(t,s.runtime,"UPDATE eventstore.projection_heads SET last_mutation_xid=NULL WHERE projection_name='"+g.projection+"'","permission denied")
  s.reject(t,s.runtime,"UPDATE eventstore.projection_generation_events SET created_xid=pg_current_xact_id() WHERE event_id='"+b+"'","permission denied")
  s.reject(t,s.migration,"UPDATE eventstore.projection_generation_events SET created_xid=pg_current_xact_id() WHERE event_id='"+b+"'","immutable")
 })
 t.Run("two projections in one transaction each activate exactly once",func(t *testing.T){
  first,second:=generationID(),generationID();p1,p2:=generationPrepare(t,s,first),generationPrepare(t,s,second)
  e1,e2:=uuid.NewString(),uuid.NewString()
  q:="BEGIN;"+generationHeadSQL(first)+generationHeadSQL(second)+generationSwitchSQL(first,e1,p1,1,"")+"SET CONSTRAINTS ALL IMMEDIATE;SET CONSTRAINTS ALL DEFERRED;"+generationSwitchSQL(second,e2,p2,1,"")+"COMMIT;"
  s.must(t,s.runtime,q)
  if got:=s.must(t,s.runtime,"SELECT count(*)||'|'||count(DISTINCT last_mutation_xid) FROM eventstore.projection_heads WHERE projection_name IN ('"+first.projection+"','"+second.projection+"') AND epoch=2");got!="2|1" {t.Fatal("per-projection transaction boundary",got)}
 })
 t.Run("caught failed second mutation rolls back only child and keeps first",func(t *testing.T){
  g:=generationID();v:=generationPrepare(t,s,g);s.must(t,s.runtime,generationHeadSQL(g));sw:=uuid.NewString()
  s.must(t,s.runtime,"BEGIN;"+generationSwitchSQL(g,sw,v,1,"")+"COMMIT;")
  one,two:=uuid.NewString(),uuid.NewString()
  step:=func(id,prev string,epoch int64)string{return generationEventSQL(g,id,prev,"hold",epoch,g.generation)+fmt.Sprintf("UPDATE eventstore.projection_heads SET epoch=%d,hold_ref='synthetic-hold',last_event_id='%s' WHERE projection_name='%s';",epoch+1,id,g.projection)}
  q:="BEGIN;"+step(one,sw,2)+"SET CONSTRAINTS eventstore.projection_pointer_receipt_guard IMMEDIATE;SET CONSTRAINTS eventstore.projection_pointer_receipt_guard DEFERRED;DO $qa$ BEGIN BEGIN "+step(two,one,3)+"RAISE EXCEPTION 'second mutation unexpectedly succeeded'; EXCEPTION WHEN raise_exception THEN IF SQLERRM NOT LIKE '%one mutation per transaction%' THEN RAISE; END IF; END; END $qa$;COMMIT;"
  s.must(t,s.runtime,q)
  if got:=s.must(t,s.runtime,"SELECT epoch||'|'||last_event_id FROM eventstore.projection_heads WHERE projection_name='"+g.projection+"'");got!="3|"+one {t.Fatal("child rollback damaged parent",got)}
  if got:=s.must(t,s.runtime,"SELECT count(*) FROM eventstore.projection_generation_events WHERE event_id='"+two+"'");got!="0" {t.Fatal("child evidence survived",got)}
  s.must(t,s.runtime,"BEGIN;"+step(two,one,3)+"COMMIT;")
 })
 t.Run("nested savepoint restoration after flushed activation",func(t *testing.T){
  g:=generationID();v:=generationPrepare(t,s,g);s.must(t,s.runtime,generationHeadSQL(g));lost,win:=uuid.NewString(),uuid.NewString()
  q:="BEGIN;SAVEPOINT outer_qa;SAVEPOINT inner_qa;"+generationSwitchSQL(g,lost,v,1,"")+"SET CONSTRAINTS ALL IMMEDIATE;RELEASE inner_qa;ROLLBACK TO outer_qa;SET CONSTRAINTS ALL DEFERRED;"+generationSwitchSQL(g,win,v,1,"")+"COMMIT;"
  s.must(t,s.runtime,q)
  if got:=s.must(t,s.runtime,"SELECT last_event_id FROM eventstore.projection_heads WHERE projection_name='"+g.projection+"'");got!=win {t.Fatal("restored transaction mutation",got)}
  if got:=s.must(t,s.runtime,"SELECT count(*) FROM eventstore.projection_generation_events WHERE event_id='"+lost+"'");got!="0" {t.Fatal("rolled back flushed receipt survived",got)}
 })
 t.Run("repeated same request cannot overwrite provenance",func(t *testing.T){
  g:=generationID();v:=generationPrepare(t,s,g);s.must(t,s.runtime,generationHeadSQL(g));sw:=uuid.NewString();q:=generationSwitchSQL(g,sw,v,1,"")
  s.must(t,s.runtime,"BEGIN;"+q+"COMMIT;")
  before:=s.must(t,s.runtime,"SELECT row_to_json(h) FROM eventstore.projection_heads h WHERE projection_name='"+g.projection+"'")
  s.reject(t,s.runtime,"BEGIN;"+q+"COMMIT;","duplicate key")
  if got:=s.must(t,s.runtime,"SELECT row_to_json(h) FROM eventstore.projection_heads h WHERE projection_name='"+g.projection+"'");got!=before {t.Fatal("duplicate changed head/provenance")}
 })
}
