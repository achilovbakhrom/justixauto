package postgres_test

import (
 "context"
 "crypto/sha256"
 "database/sql"
 "errors"
 "fmt"
 "io"
 "os"
 "testing"

 "github.com/google/uuid"
 "gorm.io/gorm"
 "justixauto/pkg/commands"
 "justixauto/pkg/eventstore"
 identity "justixauto/services/identity/adapter/postgres"
)

func qaInstalled(t *testing.T) (fixture, *gorm.DB) {
 t.Helper()
 f:=start(t)
 f.must("justix_identity","postgres","CREATE ROLE justix_identity_runtime LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_identity TO justix_identity_runtime;")
 if out,err:=f.sql("justix_identity","justix_identity",f.file("pkg/eventstore/schema.sql"),"-v","owner_service=identity","-v","runtime_role=justix_identity_runtime"); err!=nil { t.Fatalf("shared install: %v %s",err,out) }
 f.must("justix_identity","justix_identity","CREATE TABLE public.schema_migrations(version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL); INSERT INTO public.schema_migrations VALUES (1,true);")
 f.must("justix_identity","justix_identity",f.file("services/identity/migrations/0001_mechanics.up.sql"))
 f.must("justix_identity","justix_identity","UPDATE public.schema_migrations SET dirty=false")
 return f,f.open("justix_identity_runtime")
}

func TestQAIdentityAdversarial(t *testing.T) {
 f,db:=qaInstalled(t)
 ctx:=context.Background()
 binds:=0
 r,err:=identity.NewUnitOfWork(db,func(tx *gorm.DB)(fixturePorts,error){ binds++; return bindFixture(tx) })
 if err!=nil {t.Fatal(err)}
 if err:=r.Check(ctx);err!=nil {t.Fatal(err)}
 for _,tc:=range []struct{name,set,reset string}{
 {"table_maintain","GRANT MAINTAIN ON eventstore.events TO justix_identity_runtime","REVOKE MAINTAIN ON eventstore.events FROM justix_identity_runtime"},
 {"marker_column_insert","GRANT INSERT(singleton,owner_service,mechanics_version) ON identity_mechanics.compatibility TO justix_identity_runtime","REVOKE INSERT(singleton,owner_service,mechanics_version) ON identity_mechanics.compatibility FROM justix_identity_runtime"},
 {"ledger_column_insert","GRANT INSERT(version,dirty) ON public.schema_migrations TO justix_identity_runtime","REVOKE INSERT(version,dirty) ON public.schema_migrations FROM justix_identity_runtime"},
 {"marker_column_references","GRANT REFERENCES(singleton) ON identity_mechanics.compatibility TO justix_identity_runtime","REVOKE REFERENCES(singleton) ON identity_mechanics.compatibility FROM justix_identity_runtime"},
 {"ledger_column_references","GRANT REFERENCES(version) ON public.schema_migrations TO justix_identity_runtime","REVOKE REFERENCES(version) ON public.schema_migrations FROM justix_identity_runtime"},
 {"table_references","GRANT REFERENCES ON eventstore.events TO justix_identity_runtime","REVOKE REFERENCES ON eventstore.events FROM justix_identity_runtime"},
 {"column_references","GRANT REFERENCES(event_id) ON eventstore.events TO justix_identity_runtime","REVOKE REFERENCES(event_id) ON eventstore.events FROM justix_identity_runtime"},
 {"missing_schema_usage","REVOKE USAGE ON SCHEMA eventstore FROM justix_identity_runtime","GRANT USAGE ON SCHEMA eventstore TO justix_identity_runtime"},
 {"database_create","GRANT CREATE ON DATABASE justix_identity TO justix_identity_runtime","REVOKE CREATE ON DATABASE justix_identity FROM justix_identity_runtime"},
 {"missing_insert","REVOKE INSERT ON eventstore.inbox FROM justix_identity_runtime","GRANT INSERT ON eventstore.inbox TO justix_identity_runtime"},
 {"column_marker_update","GRANT UPDATE(mechanics_version) ON identity_mechanics.compatibility TO justix_identity_runtime","REVOKE UPDATE(mechanics_version) ON identity_mechanics.compatibility FROM justix_identity_runtime"},
 } {
  t.Run(tc.name,func(t *testing.T){
   f.must("justix_identity","justix_identity",tc.set)
   defer f.must("justix_identity","justix_identity",tc.reset)
   checkErr:=r.Check(ctx)
   before:=binds; called:=false
   runErr:=r.Run(ctx,func(fixturePorts)error{called=true;return nil})
   if !errors.Is(checkErr,identity.ErrIncompatiblePersistence)||!errors.Is(runErr,identity.ErrIncompatiblePersistence)||called||binds!=before{
    t.Errorf("incompatible grants accepted: Check=%v Run=%v callback=%v factory calls=%d",checkErr,runErr,called,binds-before)
   }
  })
 }
 t.Run("empty_ledger",func(t *testing.T){
  f.must("justix_identity","justix_identity","DELETE FROM public.schema_migrations")
  defer f.must("justix_identity","justix_identity","INSERT INTO public.schema_migrations VALUES(1,false)")
  if r.Check(ctx)==nil {t.Fatal("empty ledger accepted")}
 })
 for _,committed:=range []bool{false,true}{
  name:="rollback_lost_reply"; if committed{name="commit_lost_reply"}
  t.Run(name,func(t *testing.T){
   pool,err:=db.DB();if err!=nil{t.Fatal(err)}
   fault:=db.Session(&gorm.Session{NewDB:true})
   fault.Statement=&gorm.Statement{DB:fault,ConnPool:qaFaultPool{DB:pool,committed:committed}}
   fr,err:=identity.NewUnitOfWork(fault,bindFixture);if err!=nil{t.Fatal(err)}
   input:=fixtureValue{ReferenceID:uuid.NewString()}
   scope:=commands.Scope{ActorID:uuid.NewString(),Key:uuid.NewString()}
   calls:=0
   err=fr.Run(ctx,func(u fixturePorts)error{calls++;_,err:=u.Record(ctx,scope,input);return err})
   if !errors.Is(err,eventstore.ErrCommitOutcomeUnknown)||!errors.Is(err,io.ErrUnexpectedEOF)||calls!=1{t.Fatalf("lost reply: %v calls=%d",err,calls)}
   expected:="0";if committed{expected="1"}
   if n:=f.must("justix_identity","justix_identity_runtime","SELECT count(*) FROM eventstore.command_receipts WHERE idempotency_key='"+scope.Key+"'");n!=expected{t.Fatalf("receipt=%s expected=%s",n,expected)}
   if err:=r.Run(ctx,func(u fixturePorts)error{_,err:=u.Record(ctx,scope,input);return err});err!=nil{t.Fatal(err)}
   for _,table:=range []string{"events","operations"}{
    if n:=f.must("justix_identity","justix_identity_runtime","SELECT count(*) FROM eventstore."+table+" WHERE aggregate_id='"+input.ReferenceID+"'");n!="1"{t.Fatalf("%s effects=%s",table,n)}
   }
  })
 }
}
func TestQAIdentityMessagingCompatibility(t *testing.T) {
 f,db:=qaInstalled(t)
 ctx:=context.Background()
 r,err:=identity.NewUnitOfWork(db,bindFixture);if err!=nil{t.Fatal(err)}
 if err:=r.Check(ctx);err!=nil{t.Fatal(err)}
 body,err:=os.ReadFile(os.Getenv("JUSTIXAUTO_QA_T917_SCHEMA"));if err!=nil{t.Fatal(err)}
 if hash:=fmt.Sprintf("%x",sha256.Sum256(body));hash!="1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2"{t.Fatalf("unexpected T917 schema %s",hash)}
 if out,err:=f.sql("justix_identity","justix_identity",string(body),"-v","owner_service=identity","-v","runtime_role=justix_identity_runtime");err!=nil{t.Fatalf("T917 install: %v %s",err,out)}
 if err:=r.Check(ctx);err!=nil{t.Fatalf("additive legacy readiness: %v",err)}
 if err:=r.Run(ctx,func(fixturePorts)error{return nil});err!=nil{t.Fatal(err)}
 t.Log("Exact T917 additive schema in legacy mode: Check and Run succeed")
 f.must("justix_identity","justix_identity","SELECT eventstore.activate_messaging_custody('"+uuid.NewString()+"',decode(repeat('ab',32),'hex'),decode(repeat('cd',32),'hex'),'synthetic-empty-fixture-backup','no-runtimes-started','no-broker-fixture','qa-incompatible-legacy-adapter')")
 if mode:=f.must("justix_identity","justix_identity_runtime","SELECT mode FROM eventstore.messaging_mode");mode!="custody"{t.Fatalf("mode=%s",mode)}
 if err:=r.Check(ctx);!errors.Is(err,identity.ErrIncompatiblePersistence){t.Fatalf("custody accepted: %v",err)}
 called:=false
 if err:=r.Run(ctx,func(fixturePorts)error{called=true;return nil});!errors.Is(err,identity.ErrIncompatiblePersistence)||called{t.Fatalf("custody callback=%v err=%v",called,err)}
 t.Log("Exact T917 empty-owner custody activation: Check and Run reject, no callback")
}
type qaFaultPool struct{*sql.DB;committed bool}
func(p qaFaultPool) BeginTx(ctx context.Context,opts *sql.TxOptions)(gorm.ConnPool,error){tx,err:=p.DB.BeginTx(ctx,opts);if err!=nil{return nil,err};return &qaFaultTx{tx,p.committed},nil}
type qaFaultTx struct{*sql.Tx;committed bool}
func(t *qaFaultTx) Commit()error{var err error;if t.committed{err=t.Tx.Commit()}else{err=t.Tx.Rollback()};if err!=nil{return err};return io.ErrUnexpectedEOF}
