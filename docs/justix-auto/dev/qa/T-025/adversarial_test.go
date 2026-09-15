package postgres_test

import (
 "context"
 "errors"
 "os"
 "sync"
 "testing"

 "github.com/google/uuid"
 "gorm.io/gorm"
 "justixauto/pkg/commands"
 inventory "justixauto/services/inventory/adapter/postgres"
)

// Reuse disposable fixture/bootstrap helpers, but supply independent assertions.
// This file is overlaid as an additional test; production files are untouched.
func TestQAInventoryAdversarial(t *testing.T) {
 if os.Getenv("JUSTIXAUTO_TEST_INVENTORY_MECHANICS") != "1" { t.Skip("isolated PostgreSQL opt-in required") }
 f := start(t)
 f.must("justix_inventory", "postgres", "CREATE ROLE justix_inventory_runtime LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_inventory TO justix_inventory_runtime;")
 if out, err := f.sql("justix_inventory", "justix_inventory", f.file("pkg/eventstore/schema.sql"), "-v", "owner_service=inventory", "-v", "runtime_role=justix_inventory_runtime"); err != nil { t.Fatalf("shared install: %v %s", err, out) }
 f.must("justix_inventory", "justix_inventory", "CREATE TABLE public.schema_migrations(version bigint PRIMARY KEY, dirty boolean NOT NULL); INSERT INTO public.schema_migrations VALUES(1,true)")
 f.must("justix_inventory", "justix_inventory", f.file("services/inventory/migrations/0001_mechanics.up.sql"))
 f.must("justix_inventory", "justix_inventory", "UPDATE public.schema_migrations SET dirty=false")
 db := f.open("justix_inventory_runtime")
 ctx := context.Background()
 binds := 0
 ready, err := inventory.NewUnitOfWork(db, func(tx *gorm.DB) (fixturePorts,error) { binds++; return bindFixture(tx) })
 if err != nil { t.Fatal(err) }
 reject := func(t *testing.T) {
  t.Helper(); before := binds; called := false
  if !errors.Is(ready.Check(ctx), inventory.ErrIncompatiblePersistence) { t.Fatal("Check admitted incompatible grants") }
  err := ready.Run(ctx, func(fixturePorts) error { called=true; return nil })
  if !errors.Is(err,inventory.ErrIncompatiblePersistence) || called || binds!=before { t.Fatalf("Run crossed readiness fence: %v called=%v binds=%d",err,called,binds-before) }
 }
 for _, tc := range []struct{name, setup, restore string}{
  {"public-maintain", "GRANT MAINTAIN ON eventstore.operations TO PUBLIC", "REVOKE MAINTAIN ON eventstore.operations FROM PUBLIC"},
  {"two-hop-noinherit", "CREATE ROLE qa_inventory_a NOLOGIN; CREATE ROLE qa_inventory_b NOLOGIN; GRANT qa_inventory_a TO justix_inventory_runtime WITH INHERIT FALSE; GRANT qa_inventory_b TO qa_inventory_a WITH INHERIT FALSE; GRANT UPDATE(data) ON eventstore.events TO qa_inventory_b", "REVOKE qa_inventory_a FROM justix_inventory_runtime"},
  {"missing-events-insert", "REVOKE INSERT ON eventstore.events FROM justix_inventory_runtime", "GRANT INSERT ON eventstore.events TO justix_inventory_runtime"},
  {"missing-inbox-select", "REVOKE SELECT ON eventstore.inbox FROM justix_inventory_runtime", "GRANT SELECT ON eventstore.inbox TO justix_inventory_runtime"},
  {"missing-outbox-lease-update", "REVOKE UPDATE(lease_until) ON eventstore.outbox FROM justix_inventory_runtime", "GRANT UPDATE(lease_until) ON eventstore.outbox TO justix_inventory_runtime"},
  {"missing-operation-revision-update", "REVOKE UPDATE(revision) ON eventstore.operations FROM justix_inventory_runtime", "GRANT UPDATE(revision) ON eventstore.operations TO justix_inventory_runtime"},
  {"missing-eventstore-usage", "REVOKE USAGE ON SCHEMA eventstore FROM justix_inventory_runtime", "GRANT USAGE ON SCHEMA eventstore TO justix_inventory_runtime"},
  {"extra-ledger-row", "INSERT INTO public.schema_migrations VALUES(2,false)", "DELETE FROM public.schema_migrations WHERE version=2"},
 } {
  t.Run(tc.name,func(t *testing.T) {
   f.must("justix_inventory","postgres",tc.setup)
   defer func(){ f.must("justix_inventory","postgres",tc.restore); if err:=ready.Check(ctx);err!=nil { t.Fatalf("restored narrow state rejected: %v",err) } }()
   reject(t)
  })
 }
 runner,err:=inventory.NewUnitOfWork(db,bindFixture); if err!=nil {t.Fatal(err)}
 t.Run("factory-error-rolls-back-all-three-records",func(t *testing.T){
  scope:=commands.Scope{ActorID:uuid.NewString(),CompanyID:uuid.NewString(),Key:uuid.NewString()}
  input:=fixtureValue{ReferenceID:uuid.NewString()}; fault:=errors.New("QA binding dependency failed"); called:=false
  r,err:=inventory.NewUnitOfWork(db,func(tx *gorm.DB)(fixturePorts,error){
   u,err:=bindFixture(tx);if err!=nil{return nil,err};if _,err=u.Record(ctx,scope,input);err!=nil{return nil,err};return nil,fault
  });if err!=nil{t.Fatal(err)}
  err=r.Run(ctx,func(fixturePorts)error{called=true;return nil})
  if !errors.Is(err,fault)||called{t.Fatalf("factory failure lost: %v called=%v",err,called)}
  for _,table:=range []string{"events","command_receipts","operations"}{if got:=f.must("justix_inventory","justix_inventory_runtime","SELECT count(*) FROM eventstore."+table+" WHERE company_id='"+scope.CompanyID+"'");got!="0"{t.Fatalf("factory leaked %s=%s",table,got)}}
 })
 t.Run("competing-changed-input-one-receipt",func(t *testing.T){
  scope:=commands.Scope{ActorID:uuid.NewString(),CompanyID:uuid.NewString(),Key:uuid.NewString()}
  start:=make(chan struct{});out:=make(chan error,2);var wg sync.WaitGroup
  for range 2 {wg.Add(1);go func(){defer wg.Done();input:=fixtureValue{ReferenceID:uuid.NewString()};<-start;out<-runner.Run(ctx,func(u fixturePorts)error{_,err:=u.Record(ctx,scope,input);return err})}()}
  close(start);wg.Wait();close(out);ok,conflict:=0,0
  for err:=range out{if err==nil{ok++}else if errors.Is(err,commands.ErrConflict){conflict++}else{t.Fatal(err)}}
  if ok!=1||conflict!=1{t.Fatalf("outcomes success=%d conflict=%d",ok,conflict)}
  for _,table:=range []string{"events","command_receipts","operations"}{if got:=f.must("justix_inventory","justix_inventory_runtime","SELECT count(*) FROM eventstore."+table+" WHERE company_id='"+scope.CompanyID+"'");got!="1"{t.Fatalf("conflict partial effects %s=%s",table,got)}}
 })
 t.Run("operation-company-fence-before-authorization",func(t *testing.T){
  scope:=commands.Scope{ActorID:uuid.NewString(),CompanyID:uuid.NewString(),Key:uuid.NewString()};input:=fixtureValue{ReferenceID:uuid.NewString()}
  if err:=runner.Run(ctx,func(u fixturePorts)error{_,err:=u.Record(ctx,scope,input);return err});err!=nil{t.Fatal(err)}
  id:=f.must("justix_inventory","justix_inventory_runtime","SELECT operation_id FROM eventstore.operations WHERE company_id='"+scope.CompanyID+"'")
  for _,company:=range []string{"",uuid.NewString()}{
   called:=false;err:=runner.Run(ctx,func(u fixturePorts)error{
    _,err:=u.(*fixtureAdapter).operations.Status(ctx,id,commands.OperationAccess{ActorID:scope.ActorID,CompanyID:company},func(context.Context,commands.OperationAccess,commands.OperationAction,commands.Operation[fixtureValue,fixtureValue])error{called=true;return nil});return err
   });if !errors.Is(err,commands.ErrOperationNotFound)||called{t.Fatalf("foreign company reached authorization: %v called=%v",err,called)}
  }
  denied:=errors.New("QA actor not permitted");called:=0
  err:=runner.Run(ctx,func(u fixturePorts)error{_,err:=u.(*fixtureAdapter).operations.Status(ctx,id,commands.OperationAccess{ActorID:uuid.NewString(),CompanyID:scope.CompanyID},func(context.Context,commands.OperationAccess,commands.OperationAction,commands.Operation[fixtureValue,fixtureValue])error{called++;return denied});return err})
  if !errors.Is(err,denied)||called!=1{t.Fatalf("fresh actor denial ignored: %v calls=%d",err,called)}
 })
}
