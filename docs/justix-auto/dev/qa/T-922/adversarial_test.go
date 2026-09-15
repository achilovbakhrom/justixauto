package inbox_test

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "os"
 "testing"
 "time"

 "github.com/google/uuid"
 "gorm.io/gorm"
 "justixauto/pkg/events"
 "justixauto/pkg/inbox"
)

// Independently authored overlay tests; reuse only the pinned disposable fixture
// and typed synthetic owner adapters from the exact reviewed commit.
func TestQA922AdversarialPostgres(t *testing.T) {
 if os.Getenv("JUSTIXAUTO_TEST_INBOX") != "1" { t.Skip("live fixture opt-in") }
 db, owner := databaseWithMessaging(t, true)
 ctx, company := context.Background(), uuid.NewString()
 mustOwner := func(s string) { t.Helper(); if out, err := owner(s); err != nil { t.Fatal(out,err) } }
 mustOwner(fmt.Sprintf(`SELECT eventstore.activate_messaging_custody('%s',sha256('qa-new'::bytea),sha256('qa-old'::bytea),'qa-backup','qa-stopped','qa-broker','qa-compatibility')`,uuid.NewString()))
 apply := func(u effects,e events.Envelope) error {return u.Apply(ctx,e)}
 never := func(effects,events.Envelope) error {t.Error("effect must not run");return nil}
 bound := func(tx *gorm.DB,name string) *inbox.TransactionalConsumer[effects] {
  t.Helper(); c,err:=inbox.NewTransactionalConsumer(tx,name,func(tx *gorm.DB)(effects,error){return effectAdapter{tx:tx,consumer:name},nil},validator(t,company)); if err!=nil {t.Fatal(err)};return c
 }
 count := func(table,name string) int64 {t.Helper();var n int64;if err:=db.Raw("SELECT count(*) FROM "+table+" WHERE consumer_name=?",name).Scan(&n).Error;err!=nil {t.Fatal(err)};return n}
 seed := func(name string)(string,[]byte) { t.Helper();id:=uuid.NewString();body:=message(t,id,uuid.NewString(),company,1);tx:=db.Begin();defer tx.Rollback();if got,err:=bound(tx,name).Apply(ctx,body,apply);got!=inbox.AppliedCandidate||err!=nil {t.Fatal(got,err)};if err:=tx.Commit().Error;err!=nil {t.Fatal(err)};return id,body }

 t.Run("consumer namespaces do not reuse another inbox or complete its job",func(t *testing.T){
  id,body:=seed("qa-A")
  if err:=db.Exec("INSERT INTO fixture.jobs(consumer_name,event_id) VALUES ('qa-A',?::uuid),('qa-B',?::uuid)",id,id).Error;err!=nil {t.Fatal(err)}
  err:=dispatchRunner(t,db,"qa-B",validator(t,company)).Run(ctx,func(u dispatchUnit)error{got,err:=u.Apply(ctx,body,apply);if got!=inbox.AppliedCandidate||err!=nil {t.Fatalf("B reused A: %v %v",got,err)};return u.Complete(ctx,id)})
  if err!=nil {t.Fatal(err)}
  for _,name:=range []string{"qa-A","qa-B"} {if count("eventstore.inbox",name)!=1||count("fixture.effects",name)!=1 {t.Fatal("independent effects missing")}}
  var a,b bool
  if err:=db.Raw("SELECT completed FROM fixture.jobs WHERE consumer_name='qa-A'").Scan(&a).Error;err!=nil {t.Fatal(err)}
  if err:=db.Raw("SELECT completed FROM fixture.jobs WHERE consumer_name='qa-B'").Scan(&b).Error;err!=nil||a||!b {t.Fatal(a,b,err)}
 })
 t.Run("read only caller cannot return even a duplicate candidate",func(t *testing.T){
  _,body:=seed("qa-readonly");tx:=db.Begin(&sql.TxOptions{Isolation:sql.LevelReadCommitted,ReadOnly:true});defer tx.Rollback()
  got,err:=bound(tx,"qa-readonly").Apply(ctx,body,never);var state interface{SQLState()string}
  if got!=inbox.NoCandidate||!errors.As(err,&state)||state.SQLState()!="25006" {t.Fatal(got,err)}
 })
 t.Run("duplicate does not bypass missing runtime insert privilege",func(t *testing.T){
  _,body:=seed("qa-privilege")
  mustOwner("REVOKE INSERT ON eventstore.inbox FROM justix_inventory_runtime")
  defer mustOwner("GRANT INSERT ON eventstore.inbox TO justix_inventory_runtime")
  tx:=db.Begin();defer tx.Rollback();got,err:=bound(tx,"qa-privilege").Apply(ctx,body,never);var state interface{SQLState()string}
  if got!=inbox.NoCandidate||!errors.As(err,&state)||state.SQLState()!="42501" {t.Fatal(got,err)}
 })
 t.Run("savepoint rollback invalidates candidate and restores fresh processing",func(t *testing.T){
  name:="qa-savepoint";body:=message(t,uuid.NewString(),uuid.NewString(),company,1);tx:=db.Begin();defer tx.Rollback();c:=bound(tx,name)
  if err:=tx.Exec("SAVEPOINT qa_before").Error;err!=nil {t.Fatal(err)}
  if got,err:=c.Apply(ctx,body,apply);got!=inbox.AppliedCandidate||err!=nil {t.Fatal(got,err)}
  if err:=tx.Exec("ROLLBACK TO SAVEPOINT qa_before").Error;err!=nil {t.Fatal(err)}
  if got,err:=c.Apply(ctx,body,apply);got!=inbox.AppliedCandidate||err!=nil {t.Fatal(got,err)}
  if err:=tx.Commit().Error;err!=nil {t.Fatal(err)}
  if count("eventstore.inbox",name)!=1||count("fixture.effects",name)!=1 {t.Fatal("savepoint retry did not converge")}
  if got,err:=c.Apply(ctx,body,never);got!=inbox.NoCandidate||err==nil {t.Fatal("committed handle accepted",got,err)}
 })
 t.Run("duplicate retains table fence until caller rollback",func(t *testing.T){
  _,body:=seed("qa-fence");tx:=db.Begin();defer tx.Rollback()
  if got,err:=bound(tx,"qa-fence").Apply(ctx,body,never);got!=inbox.DuplicateCandidate||err!=nil {t.Fatal(got,err)}
  out,err:=owner("BEGIN; SET LOCAL lock_timeout='100ms'; LOCK TABLE eventstore.inbox IN ACCESS EXCLUSIVE MODE; COMMIT;")
  if err==nil {t.Fatal("duplicate released its fence before outer transaction ended",out)}
  if err:=tx.Rollback().Error;err!=nil {t.Fatal(err)}
  mustOwner("BEGIN; SET LOCAL lock_timeout='1s'; LOCK TABLE eventstore.inbox IN ACCESS EXCLUSIVE MODE; COMMIT;")
 })
 t.Run("cancel while waiting for conflicting insert leaves no partial duplicate",func(t *testing.T){
  name:="qa-wait-cancel";body:=message(t,uuid.NewString(),uuid.NewString(),company,1);first:=db.Begin();defer first.Rollback()
  if got,err:=bound(first,name).Apply(ctx,body,apply);got!=inbox.AppliedCandidate||err!=nil {t.Fatal(got,err)}
  second:=db.Begin();defer second.Rollback();deadline,cancel:=context.WithTimeout(ctx,100*time.Millisecond);defer cancel()
  if got,err:=bound(second,name).Apply(deadline,body,never);got!=inbox.NoCandidate||err==nil {t.Fatal(got,err)}
  second.Rollback();first.Rollback()
  if count("eventstore.inbox",name)!=0||count("fixture.effects",name)!=0 {t.Fatal("rollback leaked writes")}
  third:=db.Begin();defer third.Rollback()
  if got,err:=bound(third,name).Apply(ctx,body,apply);got!=inbox.AppliedCandidate||err!=nil {t.Fatal(got,err)}
  if err:=third.Commit().Error;err!=nil {t.Fatal(err)}
 })
 t.Run("runtime cannot rewrite durable inbox evidence or active mode",func(t *testing.T){
  seed("qa-immutability")
  for _,statement:=range []string{
   "UPDATE eventstore.inbox SET envelope_hash=sha256('replacement'::bytea) WHERE consumer_name='qa-immutability'",
   "DELETE FROM eventstore.inbox WHERE consumer_name='qa-immutability'",
   "UPDATE eventstore.messaging_mode SET mode='legacy' WHERE singleton",
  } {if err:=db.Exec(statement).Error;err==nil {t.Fatal("runtime mutation accepted",statement)}}
  if count("eventstore.inbox","qa-immutability")!=1 {t.Fatal("evidence missing")}
 })
}
