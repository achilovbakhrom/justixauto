package eventstore_test

import (
 "context"
 "database/sql"
 "fmt"
 "strings"
 "testing"

 "github.com/google/uuid"
 "justixauto/pkg/events"
 "justixauto/pkg/eventstore"
)

// Independent QA overlay, synthetic fixture only. No application source edits.
func TestQA926Adversarial(t *testing.T) {
 f := newRouteFixture(t)
 t.Run("extra reachable and default privilege rejection remains atomic", func(t *testing.T) {
  for _, grant := range []string{
   "GRANT REFERENCES(consumer_name) ON eventstore.inbox TO %s",
   "GRANT INSERT(singleton) ON eventstore.messaging_mode TO %s",
   "GRANT USAGE ON SCHEMA eventstore TO %s WITH GRANT OPTION",
   "ALTER DEFAULT PRIVILEGES IN SCHEMA eventstore GRANT SELECT ON TABLES TO %s WITH GRANT OPTION",
   "ALTER DEFAULT PRIVILEGES IN SCHEMA eventstore GRANT EXECUTE ON FUNCTIONS TO %s",
  } {
   s := checkpointStore(t, f)
   role, bridge := "qa_leaf_"+strings.ReplaceAll(uuid.NewString(),"-",""), "qa_bridge_"+strings.ReplaceAll(uuid.NewString(),"-","")
   s.must(t,"postgres","CREATE ROLE "+role+" NOINHERIT; CREATE ROLE "+bridge+" NOINHERIT; ALTER ROLE "+s.runtime+" NOINHERIT; GRANT "+role+" TO "+bridge+"; GRANT "+bridge+" TO "+s.runtime)
   s.must(t,s.migration,fmt.Sprintf(grant,role))
   checkpointRejectInstall(t,s)
  }
 })
 t.Run("all admission identity evidence exact and retained generation whitespace",func(t *testing.T){
  s := checkpointStore(t,f); mustCheckpoint(t,s); s.cutover(t)
  for _, kind := range []string{"projection","process"} {
   b:=checkpointID(19); b.generation=" generation-1 "
   admission:=uuid.NewString(); s.must(t,s.runtime,checkpointAdmission(b,admission,kind))
   for _, change:=range [][2]string{{"'fixture-scope'","'other-scope'"},{"'fixture-purpose'","'other-purpose'"},{"'fixture-authority'","'other-authority'"},{"'fixture-contract'","'other-contract'"},{"repeat('11',32)","repeat('12',32)"},{"' generation-1 '","'generation-1'"}} {
    s.reject(t,s.runtime,strings.Replace(bootstrapSQL(b,admission,kind),change[0],change[1],1),"identity mismatch")
   }
   s.must(t,s.runtime,"BEGIN;"+bootstrapSQL(b,admission,kind)+checkpointSQL(b)+"COMMIT;")
   if got:=s.must(t,s.runtime,"SELECT octet_length(generation) FROM eventstore.consumer_bootstraps WHERE bootstrap_id='"+b.bootstrap+"'");got!="14" {t.Fatalf("label modified: %s",got)}
  }
 })
 t.Run("checkpoint bigint overflow and nonfinite evidence rollback inbox",func(t *testing.T){
  s:=checkpointStore(t,f); mustCheckpoint(t,s)
  b:=checkpointID(9223372036854775807); s.must(t,s.runtime,"BEGIN;"+bootstrapSQL(b,"","projection")+checkpointSQL(b)+"COMMIT;")
  e:=uuid.NewString()
  s.reject(t,s.runtime,"BEGIN;"+checkpointAdvance(b,e,0)+"COMMIT;","contiguous increment")
  if got:=s.must(t,s.runtime,"SELECT count(*) FROM eventstore.inbox WHERE event_id='"+e+"'");got!="0"{t.Fatal("failed overflow retained inbox")}
  for _, table:=range []string{"consumer_bootstraps","projection_checkpoint_compatibility"}{
   col:="created_at"; if table=="projection_checkpoint_compatibility"{col="installed_at"}
   s.reject(t,s.migration,"UPDATE eventstore."+table+" SET "+col+"='infinity'","immutable")
  }
 })
 t.Run("cross bootstrap gaps and duplicate requests preserve evidence",func(t *testing.T){
  s:=checkpointStore(t,f);mustCheckpoint(t,s)
  a,b:=checkpointID(3),checkpointID(3)
  s.must(t,s.runtime,"BEGIN;"+bootstrapSQL(a,"","projection")+checkpointSQL(a)+bootstrapSQL(b,"","projection")+checkpointSQL(b)+"COMMIT;")
  gap:=uuid.NewString(); q:=gapSQL(a,gap,uuid.NewString(),4,7)
  s.reject(t,s.runtime,strings.Replace(q,a.bootstrap,b.bootstrap,1),"checkpoint bootstrap")
  s.must(t,s.runtime,q);s.reject(t,s.runtime,q,"duplicate key")
  root:=uuid.NewString();request:=attemptSQL(gap,root,"","requested",4,6,3)
  s.must(t,s.runtime,request);s.reject(t,s.runtime,request,"duplicate key")
  s.reject(t,s.runtime,attemptSQL(gap,uuid.NewString(),root,"recovered",3,6,3),"interval")
  s.reject(t,s.runtime,attemptSQL(gap,uuid.NewString(),root,"resolved",4,6,3),"checkpoint")
  if got:=s.must(t,s.runtime,"SELECT count(*) FROM eventstore.consumer_gap_attempts");got!="1"{t.Fatal(got)}
 })
 t.Run("savepoint rollback invalidates capability and releases locks",func(t *testing.T){
  s:=checkpointStore(t,f);tx:=fenceBegin(t,s.db);ctx:=context.Background()
  if err:=tx.Exec("SAVEPOINT qa_before_fences").Error;err!=nil{t.Fatal(err)}
  id:=uuid.NewString();req,err:=eventstore.ReceiverFence(events.OwnerInventory,events.OwnerRetail,"vehicle",id);if err!=nil{t.Fatal(err)}
  p,err:=eventstore.PrepareFences(ctx,tx,events.OwnerInventory,eventstore.SharedFence,req);if err!=nil{t.Fatal(err)}
  if err:=tx.Exec("ROLLBACK TO SAVEPOINT qa_before_fences").Error;err!=nil{t.Fatal(err)}
  if err:=p.Require(ctx,tx,events.OwnerInventory,eventstore.SharedFence,req);err==nil{t.Fatal("rolled-back capability accepted")}
  other:=fenceBegin(t,s.db)
  if !fenceTry(t,other,"justixauto:receiver:inventory:retail:vehicle:"+id,false){t.Fatal("rolled-back fence retained")}
  other.Rollback()
  fresh,err:=eventstore.PrepareFences(ctx,tx,events.OwnerInventory,eventstore.SharedFence,req);if err!=nil{t.Fatal(err)}
  if err:=fresh.Require(ctx,tx,events.OwnerInventory,eventstore.SharedFence,req);err!=nil{t.Fatal(err)}
  if err:=p.Require(ctx,tx,events.OwnerInventory,eventstore.SharedFence,req);err==nil{t.Fatal("old nonce accepted after reprepare")}
 })
 t.Run("readonly transactions preserve write prohibition and foreign owner coverage stays closed",func(t *testing.T){
  s:=checkpointStore(t,f);ctx:=context.Background()
  ro:=s.db.Begin(&sql.TxOptions{Isolation:sql.LevelReadCommitted,ReadOnly:true});defer ro.Rollback()
  var readonly string
  if err:=ro.Raw("SHOW transaction_read_only").Scan(&readonly).Error;err!=nil || readonly!="on"{t.Fatalf("read-only fixture: %s %v",readonly,err)}
  readcap,err:=eventstore.PrepareFences(ctx,ro,events.OwnerInventory,eventstore.SharedFence);if err!=nil{t.Fatal(err)}
  if err:=readcap.Require(ctx,ro,events.OwnerInventory,eventstore.SharedFence);err!=nil{t.Fatal(err)}
  if err:=ro.Exec("INSERT INTO eventstore.inbox(consumer_name,event_id,envelope_hash) VALUES('qa-readonly',?,decode(repeat('22',32),'hex'))",uuid.NewString()).Error;err==nil || !strings.Contains(err.Error(),"25006"){t.Fatal("read-only write prohibition lost",err)}
  ro.Rollback()
  tx:=fenceBegin(t,s.db)
  p,err:=eventstore.PrepareFences(ctx,tx,events.OwnerInventory,eventstore.SharedFence);if err!=nil{t.Fatal(err)}
  if err:=p.Require(ctx,tx,events.OwnerRetail,eventstore.SharedFence);err==nil{t.Fatal("foreign owner accepted")}
  if _,err:=eventstore.PrepareFences(ctx,tx,events.OwnerRetail,eventstore.SharedFence);err==nil{t.Fatal("second owner preparation accepted")}
 })
}
