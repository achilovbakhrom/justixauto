package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "sync"
 "testing"

 "github.com/golang-migrate/migrate/v4"
 "github.com/jackc/pgx/v5"
 "gorm.io/driver/postgres"
 "gorm.io/gorm"
 "gorm.io/gorm/logger"
 "justixauto/pkg/persistence"
)

// QA owns this overlay only; the reviewed implementation stays byte-exact.
func TestQA932ConcurrentInstallers(t *testing.T) {
 f := startFixture(t)
 f.bootstrap()
 ds := []*Driver{f.driver(f.config(17,false),nil),f.driver(f.config(17,false),nil)}
 engines := []*migrate.Migrate{f.engine(ds[0]),f.engine(ds[1])}
 gate:=make(chan struct{}); results:=make(chan error,2)
 var wg sync.WaitGroup
 for _,m:=range engines { wg.Add(1); go func(m *migrate.Migrate){defer wg.Done(); <-gate; results<-m.Up()}(m) }
 close(gate); wg.Wait(); close(results)
 successes,noops:=0,0
 for err:=range results { if err==nil {successes++} else if errors.Is(err,migrate.ErrNoChange) {noops++} else {t.Fatalf("installer: %v",err)} }
 if successes!=1||noops!=1 {t.Fatalf("successes=%d noops=%d",successes,noops)}
 if got:=f.must(`SELECT count(*)||':'||count(DISTINCT installation_request_id) FROM owner_migrations.artifacts`);got!="3:3" {t.Fatal(got)}
 if got:=f.must(`SELECT version||':'||dirty FROM public.schema_migrations`);got!="17:false" {t.Fatal(got)}
 f.retain("qa-concurrent-final.json",f.snapshot())
}

func TestQA932DriftAfterAcquiredLock(t *testing.T) {
 f:=startFixture(t);f.bootstrap()
 for _,mode:=range []string{"installed-file","required-grant"} {
  t.Run(mode,func(t *testing.T){
   f.t=t; d:=f.driver(f.config(12,false),nil)
   if err:=d.Lock();err!=nil{t.Fatal(err)}
   before:=f.snapshot()
   if mode=="installed-file" {
    path:=filepath.Join(f.ownerRoot,f.spec.Artifacts[0].Identity.Filename)
    original,err:=os.ReadFile(path);if err!=nil{t.Fatal(err)}
    defer func(){if err:=os.WriteFile(path,original,0600);err!=nil{t.Fatal(err)}}()
    if err:=os.WriteFile(path,append(original,' '),0600);err!=nil{t.Fatal(err)}
   } else {
    f.must(`REVOKE SELECT ON owner_migrations.artifacts FROM justix_identity_runtime`)
    defer f.must(`GRANT SELECT ON owner_migrations.artifacts TO justix_identity_runtime`)
   }
   if err:=d.SetVersion(12,true);err==nil{t.Fatal("drift accepted before dirty write")}
   if f.snapshot()!=before{t.Fatal("denied dirty call changed evidence")}
   f.retain("qa-after-lock-"+mode+".json",f.snapshot())
  })
  f.t=t
 }
 f.up(f.config(12,false))
}

func TestQA932OwnerParityAdditionalPrivileges(t *testing.T) {
 f:=startFixture(t);f.bootstrap();f.up(f.config(12,false))
 cfg:=f.config(12,false)
 db,err:=gorm.Open(postgres.Open(fmt.Sprintf("host=127.0.0.1 port=%s user=justix_identity_runtime password=%s dbname=justix_identity sslmode=disable",f.port,f.password)),&gorm.Config{Logger:logger.Default.LogMode(logger.Silent)})
 if err!=nil{t.Fatal(err)}
 pool,err:=db.DB();if err!=nil{t.Fatal(err)};defer pool.Close()
 check:=func(want bool){
  t.Helper(); runtimeErr:=persistence.Check(context.Background(),db,cfg.Profile)
  d:=f.driver(cfg,nil);ownerErr:=d.Lock();_ = d.Close()
  if (runtimeErr==nil)!=want||(ownerErr==nil)!=want {t.Fatalf("want %v runtime=%v owner=%v",want,runtimeErr,ownerErr)}
 }
 check(true); before:=f.snapshot()
 cases:=[]struct{name,change,restore string}{
  {"maintain","GRANT MAINTAIN ON synthetic_feature12.items TO justix_identity_runtime","REVOKE MAINTAIN ON synthetic_feature12.items FROM justix_identity_runtime"},
  {"references-column","GRANT REFERENCES(immutable) ON synthetic_feature12.items TO justix_identity_runtime","REVOKE REFERENCES(immutable) ON synthetic_feature12.items FROM justix_identity_runtime"},
  {"metadata-write","GRANT UPDATE ON owner_migrations.artifacts TO justix_identity_runtime","REVOKE UPDATE ON owner_migrations.artifacts FROM justix_identity_runtime"},
  {"schema-create","GRANT CREATE ON SCHEMA synthetic_feature12 TO justix_identity_runtime","REVOKE CREATE ON SCHEMA synthetic_feature12 FROM justix_identity_runtime"},
  {"database-create","GRANT CREATE ON DATABASE justix_identity TO justix_identity_runtime","REVOKE CREATE ON DATABASE justix_identity FROM justix_identity_runtime"},
  {"unrelated-feature","CREATE SCHEMA qa932_unrelated; CREATE TABLE qa932_unrelated.extra(id int); GRANT USAGE ON SCHEMA qa932_unrelated TO justix_identity_runtime; GRANT SELECT ON qa932_unrelated.extra TO justix_identity_runtime","REVOKE SELECT ON qa932_unrelated.extra FROM justix_identity_runtime; REVOKE USAGE ON SCHEMA qa932_unrelated FROM justix_identity_runtime; DROP TABLE qa932_unrelated.extra; DROP SCHEMA qa932_unrelated"},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){f.t=t;f.must(tc.change);defer f.must(tc.restore);check(false)})
  f.t=t;check(true)
 }
 if f.snapshot()!=before{t.Fatal("checks changed retained evidence")}
 f.retain("qa-privilege-parity.json",f.snapshot())
}

func TestQA932RuntimeCannotInstall(t *testing.T) {
 f:=startFixture(t);f.bootstrap();before:=f.snapshot()
 cfg,err:=pgx.ParseConfig(fmt.Sprintf("host=127.0.0.1 port=%s user=justix_identity_runtime password=%s dbname=justix_identity sslmode=disable",f.port,f.password));if err!=nil{t.Fatal(err)}
 c,err:=pgx.ConnectConfig(context.Background(),cfg);if err!=nil{t.Fatal(err)}
 defer c.Close(context.Background())
 d,err:=NewDriver(context.Background(),c,f.config(12,false));if err!=nil{t.Fatal(err)}
 defer d.Close()
 if f.snapshot()!=before{t.Fatal("construction wrote database")}
 if err:=d.Lock();err==nil{t.Fatal("runtime installed as owner")}
 if f.snapshot()!=before{t.Fatal("runtime refusal changed database")}
 f.retain("qa-runtime-refusal.json",f.snapshot())
}

func TestQA932SharedLineageAndCustodyBoundary(t *testing.T) {
 f:=startFixture(t);f.bootstrap();f.up(f.config(12,false))
 before:=f.snapshot()
 paths:=[]string{"pkg/eventstore/migrations/000002_messaging_delivery.up.sql","pkg/eventstore/migrations/000003_messaging_route_compatibility.up.sql","pkg/eventstore/migrations/000004_projection_checkpoint.up.sql"}
 for i,path:=range paths {
  old:=f.config(12,false)
  params:=[]string{}
  if i>=1 {params=append(params,"prior_migration_sha256="+hx(f.spec.Shared.Artifacts[1].SHA256),"backup_ref=qa-synthetic-backup","stopped_runtimes_ref=qa-synthetic-stopped","compatibility_ref=qa-synthetic-compatible")}
  if i==1 {params=append(params,"correction_migration_sha256="+hx(testHash(f.read(path))))}
  if i==2 {params=append(params,"base_schema_sha256="+hx(f.spec.Shared.Artifacts[0].SHA256),"correction_migration_sha256="+hx(f.spec.Shared.Artifacts[2].SHA256),"checkpoint_migration_sha256="+hx(testHash(f.read(path))))}
  f.install(path,params...)
  d:=f.driver(old,nil)
  if err:=d.Lock();err==nil{t.Fatal("undeclared shared lineage accepted")};_ = d.Close()
  f.spec.Shared.Artifacts=append(f.spec.Shared.Artifacts,persistence.SharedArtifact{Revision:uint32(i+2),Filename:path,SHA256:testHash(f.read(path))})
  d=f.driver(f.config(12,false),nil)
  if err:=d.Lock();err!=nil{t.Fatalf("revision %d: %v",i+2,err)};_ = d.Close()
 }
 f.must(`REVOKE UPDATE(position) ON eventstore.consumer_checkpoints FROM justix_identity_runtime`)
 d:=f.driver(f.config(12,false),nil)
 if err:=d.Lock();err==nil{t.Fatal("missing shared grant accepted")};_ = d.Close()
 f.must(`GRANT UPDATE(position) ON eventstore.consumer_checkpoints TO justix_identity_runtime`)
 cfg:=f.config(12,false);s:=cfg.Profile.Specification();s.Shared.Mode="custody"
 var err error;cfg.Profile,err=persistence.NewProfile(s);if err!=nil{t.Fatal(err)}
 c:=f.connect(nil);if _,err:=NewDriver(context.Background(),c,cfg);!errors.Is(err,ErrUnsupported){t.Fatalf("custody: %v",err)}
 if f.snapshot()!=before{t.Fatal("shared checks changed private history/business rows")}
 f.retain("qa-shared-lineage.json",f.snapshot())
}
