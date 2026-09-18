package postgres_test

import (
 "context"
 "database/sql"
 "errors"
 "os"
 "path/filepath"
 "testing"

 "github.com/google/uuid"
 "gorm.io/gorm"
 "justixauto/pkg/commands"
 documents "justixauto/services/documents/adapter/postgres"
)

func qa030Setup(t *testing.T)(fixture,*gorm.DB){
 f:=start(t)
 f.must("justix_documents","postgres","CREATE ROLE justix_documents_runtime LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_documents TO justix_documents_runtime")
 if out,err:=f.sql("justix_documents","justix_documents",f.file("pkg/eventstore/schema.sql"),"-v","owner_service=documents","-v","runtime_role=justix_documents_runtime");err!=nil{t.Fatalf("shared: %v %s",err,out)}
 f.must("justix_documents","justix_documents","CREATE TABLE public.schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL);INSERT INTO public.schema_migrations VALUES(1,true)")
 f.must("justix_documents","justix_documents",f.file("services/documents/migrations/0001_mechanics.up.sql"))
 f.must("justix_documents","justix_documents","UPDATE public.schema_migrations SET dirty=false")
 return f,f.open("justix_documents_runtime")
}
func qa030Snapshot(f fixture)string{
 return f.must("justix_documents","justix_documents",`SELECT jsonb_build_object('ledger',(SELECT jsonb_agg(to_jsonb(x)) FROM public.schema_migrations x),'marker',(SELECT jsonb_agg(to_jsonb(x)) FROM documents_mechanics.compatibility x),'events',(SELECT jsonb_agg(to_jsonb(x) ORDER BY event_id) FROM eventstore.events x),'receipts',(SELECT jsonb_agg(to_jsonb(x) ORDER BY receipt_id) FROM eventstore.command_receipts x),'operations',(SELECT jsonb_agg(to_jsonb(x) ORDER BY operation_id) FROM eventstore.operations x))`)
}
func qa030Retain(t *testing.T,f fixture,name,data string){
 t.Helper();if err:=os.WriteFile(filepath.Join(f.root,"docs/justix-auto/dev/qa/T-030",name),[]byte(data+"\n"),0600);err!=nil{t.Fatal(err)}
}
type qa030BeginHook struct{*sql.DB;after func();begins *int}
func(p qa030BeginHook)BeginTx(ctx context.Context,opts *sql.TxOptions)(gorm.ConnPool,error){
 if opts==nil||opts.Isolation!=sql.LevelReadCommitted{return nil,errors.New("actual transaction not ReadCommitted")}
 tx,err:=p.DB.BeginTx(ctx,opts);if err!=nil{return nil,err};*p.begins++;p.after();return tx,nil
}

func TestQA030ChecksActualTransactionAndRollsBack(t *testing.T){
 f,db:=qa030Setup(t);ctx:=context.Background();binds,calls,begins:=0,0,0
 pool,err:=db.DB();if err!=nil{t.Fatal(err)}
 wrapped:=db.Session(&gorm.Session{NewDB:true})
 wrapped.Statement=&gorm.Statement{DB:wrapped,ConnPool:qa030BeginHook{DB:pool,begins:&begins,after:func(){f.must("justix_documents","justix_documents","UPDATE public.schema_migrations SET dirty=true")}}}
 r,err:=documents.NewUnitOfWork(wrapped,func(tx *gorm.DB)(fixturePorts,error){binds++;return bindFixture(tx)});if err!=nil{t.Fatal(err)}
 if err:=r.Check(ctx);err!=nil{t.Fatal(err)}
 if err:=r.Run(ctx,func(fixturePorts)error{calls++;return nil});!errors.Is(err,documents.ErrIncompatiblePersistence)||begins!=1||binds!=0||calls!=0{t.Fatalf("post-BEGIN drift: %v begins=%d binds=%d calls=%d",err,begins,binds,calls)}
 f.must("justix_documents","justix_documents","UPDATE public.schema_migrations SET dirty=false")
 clean,err:=documents.NewUnitOfWork(db,func(tx *gorm.DB)(fixturePorts,error){
  var isolation string;if err:=tx.Raw("SHOW transaction_isolation").Scan(&isolation).Error;err!=nil{return nil,err};if isolation!="read committed"{return nil,errors.New(isolation)}
  return bindFixture(tx)
 });if err!=nil{t.Fatal(err)}
 before:=qa030Snapshot(f)
 scope:=commands.Scope{ActorID:uuid.NewString(),CompanyID:uuid.NewString(),Key:uuid.NewString()};input:=fixtureValue{ReferenceID:uuid.NewString()}
 canceled,cancel:=context.WithCancel(ctx)
 if err:=clean.Run(canceled,func(u fixturePorts)error{if _,err:=u.Record(canceled,scope,input);err!=nil{return err};cancel();return nil});!errors.Is(err,context.Canceled){t.Fatalf("cancel after effects: %v",err)}
 if qa030Snapshot(f)!=before{t.Fatal("cancel retained uncommitted effects")}
 outer:=db.Begin();if outer.Error!=nil{t.Fatal(outer.Error)}
 nested,err:=documents.NewUnitOfWork(outer,func(tx *gorm.DB)(fixturePorts,error){binds++;return bindFixture(tx)});if err!=nil{t.Fatal(err)}
 if err:=nested.Run(ctx,func(fixturePorts)error{calls++;return nil});err==nil||binds!=0||calls!=0{t.Fatalf("nested handle accepted: %v",err)}
 _=outer.Rollback()
 if err:=clean.Run(ctx,func(u fixturePorts)error{_,err:=u.Record(ctx,scope,input);return err});err!=nil{t.Fatal(err)}
 retained:=qa030Snapshot(f)
 if err:=clean.Check(ctx);err!=nil{t.Fatal(err)}
 if err:=clean.Run(ctx,func(u fixturePorts)error{v,err:=u.Load(ctx,input.ReferenceID);if err==nil&&v!=input{return errors.New("wrong replay")};return err});err!=nil{t.Fatal(err)}
 if qa030Snapshot(f)!=retained{t.Fatal("readiness/replay rewrote populated history")}
 qa030Retain(t,f,"qa-populated-retained.json",retained)
}

func TestQA030ModeMetadataExactness(t *testing.T){
 f,db:=qa030Setup(t);ctx:=context.Background()
 if out,err:=f.sql("justix_documents","justix_documents",f.file("pkg/eventstore/migrations/000002_messaging_delivery.up.sql"),"-v","owner_service=documents","-v","runtime_role=justix_documents_runtime");err!=nil{t.Fatalf("additive: %v %s",err,out)}
 // A deliberately replaceable synthetic catalog fixture isolates each scalar
 // check from original owner constraints; never edits the real migration SQL.
 f.must("justix_documents","justix_documents",`ALTER TABLE eventstore.messaging_mode RENAME TO qa_original_mode;CREATE TABLE eventstore.messaging_mode(singleton boolean,owner_service text,runtime_role name,mode text,schema_version integer);INSERT INTO eventstore.messaging_mode VALUES(true,'documents','justix_documents_runtime','legacy',2);GRANT SELECT ON eventstore.messaging_mode TO justix_documents_runtime`)
 binds,calls:=0,0
 r,err:=documents.NewUnitOfWork(db,func(tx *gorm.DB)(fixturePorts,error){binds++;return bindFixture(tx)});if err!=nil{t.Fatal(err)}
 if err:=r.Check(ctx);err!=nil{t.Fatal(err)}
 before:=qa030Snapshot(f)
 cases:=[]struct{name,set,restore string}{
  {"wrong-owner","UPDATE eventstore.messaging_mode SET owner_service='retail'","UPDATE eventstore.messaging_mode SET owner_service='documents'"},
  {"wrong-runtime","UPDATE eventstore.messaging_mode SET runtime_role='justix_documents'","UPDATE eventstore.messaging_mode SET runtime_role='justix_documents_runtime'"},
  {"wrong-revision","UPDATE eventstore.messaging_mode SET schema_version=3","UPDATE eventstore.messaging_mode SET schema_version=2"},
  {"not-singleton","UPDATE eventstore.messaging_mode SET singleton=false","UPDATE eventstore.messaging_mode SET singleton=true"},
  {"empty","DELETE FROM eventstore.messaging_mode","INSERT INTO eventstore.messaging_mode VALUES(true,'documents','justix_documents_runtime','legacy',2)"},
  {"duplicate","INSERT INTO eventstore.messaging_mode SELECT * FROM eventstore.messaging_mode","DELETE FROM eventstore.messaging_mode WHERE ctid IN (SELECT ctid FROM eventstore.messaging_mode LIMIT 1)"},
  {"wrong-table-owner","ALTER TABLE eventstore.messaging_mode OWNER TO postgres","ALTER TABLE eventstore.messaging_mode OWNER TO justix_documents"},
  {"forced-rls","ALTER TABLE eventstore.messaging_mode FORCE ROW LEVEL SECURITY","ALTER TABLE eventstore.messaging_mode NO FORCE ROW LEVEL SECURITY"},
  {"column-references","GRANT REFERENCES(mode) ON eventstore.messaging_mode TO justix_documents_runtime","REVOKE REFERENCES(mode) ON eventstore.messaging_mode FROM justix_documents_runtime"},
  {"custody-restored-legacy-grants","UPDATE eventstore.messaging_mode SET mode='custody'","UPDATE eventstore.messaging_mode SET mode='legacy'"},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   f.t=t;role:="justix_documents";if tc.name=="wrong-table-owner"{role="postgres"}
   f.must("justix_documents",role,tc.set);defer f.must("justix_documents",role,tc.restore)
   binds,calls=0,0
   if !errors.Is(r.Check(ctx),documents.ErrIncompatiblePersistence){t.Fatal("invalid metadata passed Check")}
   if err:=r.Run(ctx,func(fixturePorts)error{calls++;return nil});!errors.Is(err,documents.ErrIncompatiblePersistence)||binds!=0||calls!=0{t.Fatalf("invalid metadata reached feature: %v binds=%d calls=%d",err,binds,calls)}
  });f.t=t
  if err:=r.Check(ctx);err!=nil{t.Fatalf("restored metadata: %v",err)}
 }
 if qa030Snapshot(f)!=before{t.Fatal("metadata probes changed private evidence")}
 qa030Retain(t,f,"qa-mode-evidence.json",before)
}

func TestQA030MigrationGuardsBeforeMarker(t *testing.T){
 f:=start(t)
 f.must("justix_documents","postgres","CREATE ROLE justix_documents_runtime LOGIN PASSWORD '"+f.password+"';GRANT CONNECT ON DATABASE justix_documents TO justix_documents_runtime")
 if out,err:=f.sql("justix_documents","justix_documents",f.file("pkg/eventstore/schema.sql"),"-v","owner_service=documents","-v","runtime_role=justix_documents_runtime");err!=nil{t.Fatalf("shared: %v %s",err,out)}
 f.must("justix_documents","justix_documents","CREATE TABLE public.schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL)")
 body:=f.file("services/documents/migrations/0001_mechanics.up.sql")
 for _,tc:=range []struct{name,ledger string}{{"clean-v1","(1,false)"},{"wrong-version","(2,true)"},{"multiple-rows","(1,true),(2,false)"},{"empty",""}}{
  t.Run(tc.name,func(t *testing.T){
   f.t=t
   f.must("justix_documents","justix_documents","DELETE FROM public.schema_migrations")
   if tc.ledger!=""{f.must("justix_documents","justix_documents","INSERT INTO public.schema_migrations VALUES"+tc.ledger)}
   if _,err:=f.sql("justix_documents","justix_documents",body);err==nil{t.Fatal("invalid runner ledger accepted")}
   if f.must("justix_documents","justix_documents","SELECT to_regnamespace('documents_mechanics') IS NULL")!="t"{t.Fatal("rejected migration left schema")}
  });f.t=t
 }
 f.must("justix_documents","justix_documents","INSERT INTO public.schema_migrations VALUES(1,true);ALTER TABLE eventstore.events ALTER COLUMN owner_service SET DEFAULT 'retail'")
 if _,err:=f.sql("justix_documents","justix_documents",body);err==nil{t.Fatal("foreign shared owner accepted")}
 if f.must("justix_documents","justix_documents","SELECT to_regnamespace('documents_mechanics') IS NULL")!="t"{t.Fatal("foreign rejection left schema")}
 f.must("justix_documents","justix_documents","ALTER TABLE eventstore.events ALTER COLUMN owner_service SET DEFAULT 'documents'")
 f.must("justix_documents","justix_documents",body)
 if f.must("justix_documents","justix_documents","SELECT version||':'||dirty FROM public.schema_migrations")!="1:true"{t.Fatal("SQL migration cleaned runner ledger")}
 qa030Retain(t,f,"qa-migration-marker.json",qa030Snapshot(f))
}
