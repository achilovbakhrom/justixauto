package postgres_test

import (
 "context"
 "errors"
 "os"
 "testing"

 "gorm.io/gorm"
 retail "justixauto/services/retail/adapter/postgres"
)

func TestQA027R2Metadata(t *testing.T) {
 if os.Getenv("JUSTIXAUTO_TEST_RETAIL_MECHANICS")!="1" {t.Skip("explicit live fixture opt-in required")}
 f:=start(t)
 f.must("justix_retail","postgres","CREATE ROLE justix_retail_runtime LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_retail TO justix_retail_runtime")
 if out,err:=f.sql("justix_retail","justix_retail",f.file("pkg/eventstore/schema.sql"),"-v","owner_service=retail","-v","runtime_role=justix_retail_runtime");err!=nil{t.Fatalf("shared: %v %s",err,out)}
 f.must("justix_retail","justix_retail","CREATE TABLE public.schema_migrations(version bigint NOT NULL PRIMARY KEY,dirty boolean NOT NULL); INSERT INTO public.schema_migrations VALUES(1,true)")
 f.must("justix_retail","justix_retail",f.file("services/retail/migrations/0001_mechanics.up.sql"))
 f.must("justix_retail","justix_retail","UPDATE public.schema_migrations SET dirty=false")
 db:=f.open("justix_retail_runtime")
 binds,calls:=0,0
 r,err:=retail.NewUnitOfWork(db,func(tx *gorm.DB)(*gorm.DB,error){binds++;return tx,nil})
 if err!=nil{t.Fatal(err)}
 ctx:=context.Background()
 healthy:=func(t *testing.T){t.Helper();if err:=r.Check(ctx);err!=nil{t.Fatalf("expected supported legacy: %v",err)};if err:=r.Run(ctx,func(*gorm.DB)error{return nil});err!=nil{t.Fatal(err)}}
 snapshot:=func()string{return f.must("justix_retail","postgres","SELECT coalesce(jsonb_agg(to_jsonb(m) ORDER BY to_jsonb(m)::text),'[]'::jsonb)::text FROM eventstore.messaging_mode m")}
 reject:=func(t *testing.T){t.Helper();before:=snapshot();binds,calls=0,0;ce:=r.Check(ctx);re:=r.Run(ctx,func(*gorm.DB)error{calls++;return nil});if !errors.Is(ce,retail.ErrIncompatiblePersistence)||!errors.Is(re,retail.ErrIncompatiblePersistence)||binds!=0||calls!=0{t.Errorf("Check=%v Run=%v binds=%d calls=%d",ce,re,binds,calls)};if after:=snapshot();before!=after{t.Errorf("readiness mutated metadata: before=%s after=%s",before,after)}}
 t.Run("pre-additive-accepted",healthy)
 if out,err:=f.sql("justix_retail","justix_retail",f.file("pkg/eventstore/migrations/000002_messaging_delivery.up.sql"),"-v","owner_service=retail","-v","runtime_role=justix_retail_runtime");err!=nil{t.Fatalf("additive: %v %s",err,out)}
 t.Run("additive-legacy-accepted",healthy)
 f.must("justix_retail","postgres","CREATE ROLE qa027_bridge NOLOGIN; CREATE ROLE qa027_deep NOLOGIN; GRANT qa027_bridge TO justix_retail_runtime WITH INHERIT FALSE; GRANT qa027_deep TO qa027_bridge WITH INHERIT FALSE")
 for _,tc:=range []struct{name,set,reset string}{
  {"wrong-table-owner","ALTER TABLE eventstore.messaging_mode OWNER TO postgres","ALTER TABLE eventstore.messaging_mode OWNER TO justix_retail"},
  {"forced-RLS-even-disabled","ALTER TABLE eventstore.messaging_mode FORCE ROW LEVEL SECURITY","ALTER TABLE eventstore.messaging_mode NO FORCE ROW LEVEL SECURITY"},
  {"visible-RLS","CREATE POLICY qa027_all ON eventstore.messaging_mode FOR SELECT TO justix_retail_runtime USING(true); ALTER TABLE eventstore.messaging_mode ENABLE ROW LEVEL SECURITY","ALTER TABLE eventstore.messaging_mode DISABLE ROW LEVEL SECURITY; DROP POLICY qa027_all ON eventstore.messaging_mode"},
  {"missing-metadata-read","REVOKE SELECT ON eventstore.messaging_mode FROM justix_retail_runtime","GRANT SELECT ON eventstore.messaging_mode TO justix_retail_runtime"},
  {"PUBLIC-update-column","GRANT UPDATE(mode) ON eventstore.messaging_mode TO PUBLIC","REVOKE UPDATE(mode) ON eventstore.messaging_mode FROM PUBLIC"},
  {"PUBLIC-maintain","GRANT MAINTAIN ON eventstore.messaging_mode TO PUBLIC","REVOKE MAINTAIN ON eventstore.messaging_mode FROM PUBLIC"},
  {"runtime-delete","GRANT DELETE ON eventstore.messaging_mode TO justix_retail_runtime","REVOKE DELETE ON eventstore.messaging_mode FROM justix_retail_runtime"},
  {"runtime-references-column","GRANT REFERENCES(owner_service) ON eventstore.messaging_mode TO justix_retail_runtime","REVOKE REFERENCES(owner_service) ON eventstore.messaging_mode FROM justix_retail_runtime"},
  {"transitive-NOINHERIT-update","GRANT UPDATE(mode) ON eventstore.messaging_mode TO qa027_deep","REVOKE UPDATE(mode) ON eventstore.messaging_mode FROM qa027_deep"},
  {"transitive-NOINHERIT-trigger","GRANT TRIGGER ON eventstore.messaging_mode TO qa027_deep","REVOKE TRIGGER ON eventstore.messaging_mode FROM qa027_deep"},
 } {t.Run(tc.name,func(t *testing.T){local:=f;local.t=t;local.must("justix_retail","postgres",tc.set);defer func(){local.must("justix_retail","postgres",tc.reset);healthy(t)}();reject(t)})}
 // Substitution is confined to this disposable fixture. Preserve the real table
 // and its dependencies; unconstrained copies exercise corrupt retained rows.
 for _,tc:=range []struct{name,mutation string}{
  {"empty-mode","DELETE FROM eventstore.messaging_mode"},
  {"duplicate-singleton","INSERT INTO eventstore.messaging_mode SELECT * FROM eventstore.messaging_mode"},
  {"false-singleton","UPDATE eventstore.messaging_mode SET singleton=false"},
  {"null-singleton","UPDATE eventstore.messaging_mode SET singleton=NULL"},
  {"wrong-owner-value","UPDATE eventstore.messaging_mode SET owner_service='identity'"},
  {"wrong-runtime-value","UPDATE eventstore.messaging_mode SET runtime_role='justix_identity_runtime'"},
  {"wrong-schema-version","UPDATE eventstore.messaging_mode SET schema_version=3"},
  {"unknown-mode","UPDATE eventstore.messaging_mode SET mode='unknown'"},
  {"null-mode","UPDATE eventstore.messaging_mode SET mode=NULL"},
 } {t.Run(tc.name,func(t *testing.T){local:=f;local.t=t;local.must("justix_retail","justix_retail","ALTER TABLE eventstore.messaging_mode RENAME TO qa027_saved_mode; CREATE TABLE eventstore.messaging_mode AS SELECT * FROM eventstore.qa027_saved_mode; GRANT SELECT ON eventstore.messaging_mode TO justix_retail_runtime");defer func(){local.must("justix_retail","justix_retail","DROP TABLE eventstore.messaging_mode; ALTER TABLE eventstore.qa027_saved_mode RENAME TO messaging_mode");healthy(t)}();local.must("justix_retail","justix_retail",tc.mutation);reject(t)})}
}
