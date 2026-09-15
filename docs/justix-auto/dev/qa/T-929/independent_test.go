package eventstore_test

import (
 "crypto/sha256"
 "encoding/binary"
 "fmt"
 "strings"
 "sync"
 "testing"
 "time"
)

// Overlay-only independent QA; production and developer test bytes stay exact.
func TestQA929Independent(t *testing.T) {
 f := historyFixture(t)
 s := newHistoryStore(t, f, "identity")
 t.Run("required_grants_and_ledger_shape", func(t *testing.T) {
  for _, c := range []struct{ change, restore string }{
   {"REVOKE INSERT ON eventstore.inbox FROM justix_identity_runtime", "GRANT INSERT ON eventstore.inbox TO justix_identity_runtime"},
   {"REVOKE USAGE ON SCHEMA identity_mechanics FROM justix_identity_runtime", "GRANT USAGE ON SCHEMA identity_mechanics TO justix_identity_runtime"},
   {"ALTER TABLE public.schema_migrations ADD COLUMN unexpected text", "ALTER TABLE public.schema_migrations DROP COLUMN unexpected"},
   {"ALTER TABLE public.schema_migrations ALTER dirty DROP NOT NULL", "ALTER TABLE public.schema_migrations ALTER dirty SET NOT NULL"},
   {"GRANT UPDATE(lease_until) ON eventstore.outbox TO justix_identity_runtime WITH GRANT OPTION", "REVOKE GRANT OPTION FOR UPDATE(lease_until) ON eventstore.outbox FROM justix_identity_runtime"},
   {"GRANT INSERT(event_id) ON eventstore.events TO justix_identity_runtime WITH GRANT OPTION", "REVOKE GRANT OPTION FOR INSERT(event_id) ON eventstore.events FROM justix_identity_runtime"},
  } { s.must(t,c.change); s.reject(t); s.must(t,c.restore) }
 })
 t.Run("interrupted_install_rolls_back_every_new_object", func(t *testing.T) {
  before := s.snapshot(t)
  slow := s
  slow.script = strings.TrimSuffix(strings.TrimSpace(s.script), "COMMIT;") + "SELECT pg_sleep(7); COMMIT;\n"
  start := time.Now()
  out, err := slow.install()
  if err == nil { t.Fatalf("slow install unexpectedly committed: %s",out) }
  if elapsed:=time.Since(start); elapsed<4*time.Second || elapsed>6500*time.Millisecond {t.Fatalf("transaction bound %s",elapsed)}
  if before != s.snapshot(t) {t.Fatal("retained preimage changed")}
  if got:=s.must(t,"SELECT count(*) FROM pg_namespace WHERE nspname='owner_migrations'"); got!="0" {t.Fatal("partial schema",got)}
  if out,err:=s.install(); err!=nil {t.Fatalf("clean retry after KNOWN failed transaction: %v %s",err,out)}
 })
 t.Run("runtime_effective_privileges_and_invoker_guards",func(t *testing.T){
  if got:=s.must(t,`SELECT count(*) FROM pg_proc WHERE pronamespace='owner_migrations'::regnamespace AND (prosecdef OR has_function_privilege('justix_identity_runtime',oid,'EXECUTE'))`);got!="0" {t.Fatal("definer or executable function",got)}
  if got:=s.must(t,`SELECT count(*) FROM pg_class WHERE relnamespace='owner_migrations'::regnamespace AND relkind='r' AND (NOT has_table_privilege('justix_identity_runtime',oid,'SELECT') OR has_table_privilege('justix_identity_runtime',oid,'INSERT,UPDATE,DELETE,TRUNCATE,TRIGGER,REFERENCES,MAINTAIN'))`);got!="0" {t.Fatal("runtime table privilege",got)}
  if got:=s.must(t,`SELECT count(*) FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid WHERE c.relnamespace='owner_migrations'::regnamespace AND c.relkind='r' AND a.attnum>0 AND NOT a.attisdropped AND (has_column_privilege('justix_identity_runtime',c.oid,a.attnum,'INSERT,UPDATE,REFERENCES') OR EXISTS(SELECT FROM aclexplode(a.attacl) x WHERE x.is_grantable))`);got!="0" {t.Fatal("runtime column privilege",got)}
  for _,sql:=range []string{"UPDATE owner_migrations.artifacts SET version=version WHERE false","DELETE FROM owner_migrations.feature_contracts WHERE false","TRUNCATE owner_migrations.feature_contracts"} {if out,err:=f.sql("identity","justix_identity",sql);err==nil {t.Fatal("statement guard bypass",out)}}
 })
 t.Run("complete_dirty_rowset_and_concurrent_receipts",func(t *testing.T){
  s.must(t,"UPDATE public.schema_migrations SET version=12,dirty=true; INSERT INTO public.schema_migrations VALUES(17,true)")
  if out,err:=f.sql("identity","justix_identity",historyMarker(12));err==nil {t.Fatal("multiple rows accepted",out)}
  s.must(t,"DELETE FROM public.schema_migrations WHERE version=17")
  s.must(t,historyMarker(12))
  basehash:=fmt.Sprintf("%x",sha256.Sum256([]byte(s.base)))
  var wg sync.WaitGroup
  answers:=make(chan error,2)
  for i:=0;i<2;i++ {wg.Go(func(){_,err:=f.sql("identity","justix_identity",historyReceipt(12,1,basehash));answers<-err})}
  wg.Wait();close(answers); successes:=0
  for err:=range answers {if err==nil {successes++}}
  if successes!=1 {t.Fatalf("concurrent receipts: %d successes",successes)}
  if got:=s.must(t,"SELECT count(*) FROM owner_migrations.artifacts WHERE version=12");got!="1" {t.Fatal(got)}
  // This template deliberately does not promise driver finalization: owner SQL
  // can retain a receipt while dirty. Future driver owns atomic receipt+clean.
  if got:=s.must(t,"SELECT version||':'||dirty FROM public.schema_migrations");got!="12:true" {t.Fatal("unexpected ledger mutation",got)}
 })
 t.Run("all_owner_signed_common_keys",func(t *testing.T){
  negative:=0
  for _,o:=range owners {h:=sha256.Sum256([]byte("justixauto:owner-install:v1:justix_"+o));key:=int64(binary.BigEndian.Uint64(h[:8]));if key<0 {negative++}
   got:=f.mustSQL(o,"justix_"+o,`SELECT ('x'||substr(encode(sha256(convert_to('justixauto:owner-install:v1:'||current_database(),'UTF8')),'hex'),1,16))::bit(64)::bigint`)
   if got!=fmt.Sprint(key) {t.Fatalf("%s got %s want %d",o,got,key)}
  }
  if negative==0 {t.Fatal("negative signed path untested")}
 })
}
