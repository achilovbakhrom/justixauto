package postgres_test

import (
 "context"
 "errors"
 "fmt"
 "os"
 "testing"
 "github.com/google/uuid"
 "gorm.io/gorm"
 retail "justixauto/services/retail/adapter/postgres"
)

func TestQA027Independent(t *testing.T) {
 if os.Getenv("JUSTIXAUTO_TEST_RETAIL_MECHANICS") != "1" { t.Skip("explicit fixture opt-in required") }
 f:=start(t)
 f.must("justix_retail","postgres","CREATE ROLE justix_retail_runtime LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_retail TO justix_retail_runtime")
 if out,err:=f.sql("justix_retail","justix_retail",f.file("pkg/eventstore/schema.sql"),"-v","owner_service=retail","-v","runtime_role=justix_retail_runtime");err!=nil { t.Fatalf("shared install: %v %s",err,out) }
 f.must("justix_retail","justix_retail","CREATE TABLE public.schema_migrations(version bigint NOT NULL PRIMARY KEY,dirty boolean NOT NULL); INSERT INTO public.schema_migrations VALUES(1,true)")
 migration:=f.file("services/retail/migrations/0001_mechanics.up.sql")
 for _,owner:=range []string{"identity","inventory","commerce","financing","insurance","documents"} {
  t.Run("foreign-migration-"+owner,func(t *testing.T){
   if _,err:=f.sql("justix_"+owner,"justix_"+owner,migration);err==nil {t.Fatal("foreign migration succeeded")}
   if got:=f.must("justix_"+owner,"justix_"+owner,"SELECT count(*) FROM pg_namespace WHERE nspname='retail_mechanics'");got!="0" {t.Fatal("foreign database mutated")}
  })
 }
 f.must("justix_retail","justix_retail",migration)
 f.must("justix_retail","justix_retail","UPDATE public.schema_migrations SET dirty=false")
 db:=f.open("justix_retail_runtime")
 ctx:=context.Background()
 binds,calls:=0,0
 r,err:=retail.NewUnitOfWork(db,func(tx *gorm.DB)(*gorm.DB,error){binds++;return tx,nil})
 if err!=nil {t.Fatal(err)}
 healthy:=func(t *testing.T) {t.Helper();if err:=r.Check(ctx);err!=nil{t.Fatalf("clean fixture rejected: %v",err)}}
 reject:=func(t *testing.T){t.Helper();binds,calls=0,0; ce:=r.Check(ctx); re:=r.Run(ctx,func(*gorm.DB)error{calls++;return nil});if !errors.Is(ce,retail.ErrIncompatiblePersistence)||!errors.Is(re,retail.ErrIncompatiblePersistence)||binds!=0||calls!=0 {t.Errorf("incompatible installation reached feature: Check=%v Run=%v binds=%d calls=%d",ce,re,binds,calls)}}
 healthy(t)
 for _,tc:=range []struct{name,set,reset string}{
  {"empty-ledger","DELETE FROM public.schema_migrations","INSERT INTO public.schema_migrations VALUES(1,false)"},
  {"empty-marker","DELETE FROM retail_mechanics.compatibility","INSERT INTO retail_mechanics.compatibility VALUES(true,'retail',1)"},
  {"no-owner-default","ALTER TABLE eventstore.events ALTER owner_service DROP DEFAULT","ALTER TABLE eventstore.events ALTER owner_service SET DEFAULT 'retail'"},
  {"public-schema-create","GRANT CREATE ON SCHEMA public TO PUBLIC","REVOKE CREATE ON SCHEMA public FROM PUBLIC"},
  {"public-event-column-update","GRANT UPDATE(data) ON eventstore.events TO PUBLIC","REVOKE UPDATE(data) ON eventstore.events FROM PUBLIC"},
  {"public-marker-column-update","GRANT UPDATE(mechanics_version) ON retail_mechanics.compatibility TO PUBLIC","REVOKE UPDATE(mechanics_version) ON retail_mechanics.compatibility FROM PUBLIC"},
  {"public-ledger-column-insert","GRANT INSERT(version) ON public.schema_migrations TO PUBLIC","REVOKE INSERT(version) ON public.schema_migrations FROM PUBLIC"},
  {"operation-step-delete","GRANT DELETE ON eventstore.operation_steps TO justix_retail_runtime","REVOKE DELETE ON eventstore.operation_steps FROM justix_retail_runtime"},
  {"outbox-truncate","GRANT TRUNCATE ON eventstore.outbox TO justix_retail_runtime","REVOKE TRUNCATE ON eventstore.outbox FROM justix_retail_runtime"},
  {"receipt-update","GRANT UPDATE(receipt) ON eventstore.command_receipts TO justix_retail_runtime","REVOKE UPDATE(receipt) ON eventstore.command_receipts FROM justix_retail_runtime"},
 } {t.Run(tc.name,func(t *testing.T){f.must("justix_retail","justix_retail",tc.set);defer func(){f.must("justix_retail","justix_retail",tc.reset);healthy(t)}();reject(t)})}
 t.Run("actual-readcommitted-and-single-transaction",func(t *testing.T){
  var boundID string
  rr,err:=retail.NewUnitOfWork(db,func(tx *gorm.DB)(*gorm.DB,error){
   if err:=tx.Raw("SELECT pg_current_xact_id()::text").Scan(&boundID).Error;err!=nil{return nil,err};return tx,nil})
  if err!=nil{t.Fatal(err)}
  if err:=rr.Run(ctx,func(tx *gorm.DB)error{var iso,id string;if err:=tx.Raw("SHOW transaction_isolation").Scan(&iso).Error;err!=nil{return err};if err:=tx.Raw("SELECT pg_current_xact_id()::text").Scan(&id).Error;err!=nil{return err};if iso!="read committed"||id!=boundID||id==""{return fmt.Errorf("isolation=%s bound=%s callback=%s",iso,boundID,id)};return nil});err!=nil{t.Fatal(err)}
 })
 t.Run("same-runner-rechecks-between-transactions",func(t *testing.T){
  if err:=r.Run(ctx,func(*gorm.DB)error{return nil});err!=nil{t.Fatal(err)}
  f.must("justix_retail","justix_retail","UPDATE public.schema_migrations SET dirty=true")
  reject(t)
  f.must("justix_retail","justix_retail","UPDATE public.schema_migrations SET dirty=false")
  healthy(t)
 })
 t.Run("custody-with-restored-legacy-grants",func(t *testing.T){
  if out,err:=f.sql("justix_retail","justix_retail",f.file("pkg/eventstore/migrations/000002_messaging_delivery.up.sql"),"-v","owner_service=retail","-v","runtime_role=justix_retail_runtime");err!=nil{t.Fatalf("additive install: %v %s",err,out)}
  healthy(t)
  f.must("justix_retail","justix_retail","SELECT eventstore.activate_messaging_custody('"+uuid.NewString()+"',decode(repeat('ab',32),'hex'),decode(repeat('cd',32),'hex'),'qa-fixture','no-runtime','no-broker','qa-legacy-mode-probe')")
  reject(t)
  f.must("justix_retail","justix_retail","GRANT INSERT ON eventstore.outbox TO justix_retail_runtime; GRANT UPDATE(attempts,next_attempt_at,lease_owner,lease_until,sent_at) ON eventstore.outbox TO justix_retail_runtime")
  if mode:=f.must("justix_retail","justix_retail_runtime","SELECT mode FROM eventstore.messaging_mode");mode!="custody"{t.Fatal("not custody")}
  reject(t)
 })
}
