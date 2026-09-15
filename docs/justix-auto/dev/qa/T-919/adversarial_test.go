package outbox_test

import (
 "context"
 "crypto/sha256"
 "errors"
 "fmt"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/google/uuid"
 "justixauto/pkg/events"
 "justixauto/pkg/eventstore"
 "justixauto/pkg/outbox"
)

// QA-only overlay: reuses the reviewed disposable fixture and typed value
// constructors, while every scenario/assertion below is independently authored.
func TestQAT919Adversarial(t *testing.T) {
 var admin func(string,...string)(string,error)
 db:=database(t,func(run func(string,...string)(string,error)) {
  admin=run
  for _,file:=range []string{"000002_messaging_delivery.up.sql","000003_messaging_route_compatibility.up.sql"} {
   b,err:=os.ReadFile("../eventstore/migrations/"+file); if err!=nil {t.Fatal(err)}
   args:=[]string{"-v","owner_service=inventory","-v","runtime_role=justix_inventory_runtime"}
   if strings.HasPrefix(file,"000003") {args=append(args,"-v","prior_migration_sha256=1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2","-v",fmt.Sprintf("correction_migration_sha256=%x",sha256.Sum256(b)),"-v","backup_ref=qa-empty","-v","stopped_runtimes_ref=qa-no-runtimes","-v","compatibility_ref=qa-no-broker")}
   if out,err:=run(string(b),args...);err!=nil {t.Fatal(err,out)}
  }
 })
 if out,err:=admin(`SELECT eventstore.activate_messaging_custody('`+uuid.NewString()+`',sha256('qa-new'::bytea),sha256('qa-old'::bytea),'qa-empty','qa-no-runtimes','qa-no-broker','qa-compatible')`);err!=nil {t.Fatal(err,out)}
 ctx:=context.Background(); r:=custodyRunner(t,db)
 count:=func(table,id string) int64 {t.Helper();var n int64;if err:=db.Raw("SELECT count(*) FROM eventstore."+table+" WHERE aggregate_id=?::uuid",id).Scan(&n).Error;err!=nil {t.Fatal(err)};return n}
 empty:=func(s outbox.SourceStream){t.Helper();for _,table:=range []string{"events","outbox_messages","fixture_guard","messaging_admissions"} {if n:=count(table,s.AggregateID());n!=0 {t.Fatalf("%s retained %d for %s",table,n,s.AggregateID())}}}
 write:=func(p custodyPorts,s outbox.SourceStream,e eventstore.Event,allowEmpty bool) error {if err:=p.append.Append(ctx,eventstore.Batch{Events:[]eventstore.Event{e}});err!=nil{return err};return p.insert.InsertMessages(ctx,custodyMessage(t,e,custodyRule(t,s,allowEmpty)))}
 t.Run("unfenced append cannot emit even with explicit empty authority",func(t *testing.T){
  s:=custodyStream(t,uuid.NewString());e:=pending(t,s.AggregateID(),1,1)
  err:=r.Run(ctx,func(p custodyPorts)error{return write(p,s,e,true)})
  if !errors.Is(err,outbox.ErrUnfencedStream){t.Fatal(err)};empty(s)
 })
 t.Run("later stream missing plan rolls back earlier complete plan and guard",func(t *testing.T){
  a,b:=custodyStream(t,uuid.NewString()),custodyStream(t,uuid.NewString())
  err:=r.Run(ctx,func(p custodyPorts)error{
   if err:=p.admissions.Fence(ctx,b,a);err!=nil{return err};custodyAdmit(t,p,a,events.OwnerRetail,0)
   if err:=p.tx.Exec("INSERT INTO eventstore.fixture_guard VALUES(?::uuid)",a.AggregateID()).Error;err!=nil{return err}
   if err:=write(p,a,pending(t,a.AggregateID(),1,1),false);err!=nil{return err}
   return write(p,b,pending(t,b.AggregateID(),1,1),false)
  });if !errors.Is(err,outbox.ErrMissingRecipientPlan){t.Fatal(err)};empty(a);empty(b)
 })
 t.Run("rule allowlist rejection rolls back admitted stream",func(t *testing.T){
  s:=custodyStream(t,uuid.NewString());e:=pending(t,s.AggregateID(),1,1);env,_:=e.IntegrationEnvelope()
  rule,err:=outbox.NewSourcePlanRule(s,"qa-authority",[]string{"inventory.fixture.other.v1"},"qa-empty");if err!=nil {t.Fatal(err)}
  m,err:=outbox.NewMessage(env,schema(t,events.OwnerInventory),rule);if err!=nil {t.Fatal(err)}
  err=r.Run(ctx,func(p custodyPorts)error{if err:=p.admissions.Fence(ctx,s);err!=nil{return err};custodyAdmit(t,p,s,events.OwnerRetail,0);if err:=p.append.Append(ctx,eventstore.Batch{Events:[]eventstore.Event{e}});err!=nil{return err};return p.insert.InsertMessages(ctx,m)})
  if !errors.Is(err,outbox.ErrAdmissionSchema){t.Fatal(err)};empty(s)
 })
 t.Run("foreign owner empty call cannot use inventory store",func(t *testing.T){
  err:=r.Run(ctx,func(p custodyPorts)error{a,err:=outbox.NewSourceAdmissions(p.tx,events.OwnerCommerce);if err!=nil{return err};i,err:=outbox.NewCustodyInserter(p.tx,events.OwnerCommerce,a);if err!=nil{return err};return i.InsertMessages(ctx)})
  if !errors.Is(err,outbox.ErrMessagingMode){t.Fatal(err)}
 })
 t.Run("lost marker select privilege fails empty call and restoration works",func(t *testing.T){
  if out,err:=admin("REVOKE SELECT ON eventstore.messaging_route_compatibility FROM justix_inventory_runtime");err!=nil {t.Fatal(err,out)}
  defer func(){if out,err:=admin("GRANT SELECT ON eventstore.messaging_route_compatibility TO justix_inventory_runtime");err!=nil {t.Error(err,out)}}()
  if err:=r.Run(ctx,func(p custodyPorts)error{return p.insert.InsertMessages(ctx)});err==nil {t.Fatal("missing privilege accepted")}
 })
 t.Run("runtime cannot rewrite source authority or compatibility evidence",func(t *testing.T){
  for _,q:=range []string{"UPDATE eventstore.messaging_admissions SET authority_ref='qa-forged'","DELETE FROM eventstore.messaging_admissions","UPDATE eventstore.messaging_route_compatibility SET backup_ref='qa-forged'","TRUNCATE eventstore.outbox_messages CASCADE"} {if err:=db.Exec(q).Error;err==nil {t.Fatal("privilege bypass",q)}}
  if err:=r.Run(ctx,func(p custodyPorts)error{return p.insert.InsertMessages(ctx)});err!=nil {t.Fatal("privilege not restored",err)}
 })
 t.Run("concurrent close retains committed child but blocks next emission",func(t *testing.T){
  s:=custodyStream(t,uuid.NewString());e:=pending(t,s.AggregateID(),1,1);ready,release:=make(chan struct{}),make(chan struct{});done:=make(chan error,1);var admission string
  go func(){done<-r.Run(ctx,func(p custodyPorts)error{if err:=p.admissions.Fence(ctx,s);err!=nil{return err};admission=custodyAdmit(t,p,s,events.OwnerRetail,0);if err:=write(p,s,e,false);err!=nil{return err};close(ready);<-release;return nil})}()
  select {case <-ready:case err:=<-done:t.Fatal(err);case <-time.After(5*time.Second):t.Fatal("writer not ready")}
  closed:=make(chan error,1)
  go func(){closed<-r.Run(ctx,func(p custodyPorts)error{if err:=p.admissions.Fence(ctx,s);err!=nil{return err};return p.admissions.Change(ctx,s,outbox.SourceAdmissionChange{ID:uuid.NewString(),PriorID:admission,Action:outbox.AdmissionClose,Checkpoint:revision(1),AuthorityRef:"qa-close",EvidenceRef:"qa-drain"})})}()
  select {case err:=<-closed:close(release);t.Fatal("close bypassed fence",err);case <-time.After(100*time.Millisecond):}
  close(release);if err:=<-done;err!=nil {t.Fatal(err)};if err:=<-closed;err!=nil {t.Fatal(err)}
  err:=r.Run(ctx,func(p custodyPorts)error{if err:=p.admissions.Fence(ctx,s);err!=nil{return err};e2:=pending(t,s.AggregateID(),2,2);if err:=p.append.Append(ctx,eventstore.Batch{Expected:revision(1),Events:[]eventstore.Event{e2}});err!=nil{return err};return p.insert.InsertMessages(ctx,custodyMessage(t,e2,custodyRule(t,s,false)))})
  if !errors.Is(err,outbox.ErrMissingRecipientPlan)||count("events",s.AggregateID())!=1||count("outbox_messages",s.AggregateID())!=1 {t.Fatal("closed range leaked emission",err)}
  var n int64;env,_:=e.IntegrationEnvelope();if err:=db.Raw("SELECT count(*) FROM eventstore.outbox_deliveries WHERE event_id=?::uuid AND sent_at IS NULL",env.EventID()).Scan(&n).Error;err!=nil||n!=1 {t.Fatal("drain lost prior obligation",err,n)}
 })
 t.Run("extra child attempt aborts full command after valid insertion",func(t *testing.T){
  s:=custodyStream(t,uuid.NewString());e:=pending(t,s.AggregateID(),1,1);env,_:=e.IntegrationEnvelope()
  err:=r.Run(ctx,func(p custodyPorts)error{if err:=p.admissions.Fence(ctx,s);err!=nil{return err};custodyAdmit(t,p,s,events.OwnerRetail,0);if err:=write(p,s,e,false);err!=nil{return err};return p.tx.Exec(`INSERT INTO eventstore.outbox_deliveries(event_id,destination,admission_id,exchange,routing_key) SELECT event_id,'commerce',admission_id,exchange,'inventory.commerce.inventory.fixture.changed.v1' FROM eventstore.outbox_deliveries WHERE event_id=?::uuid`,env.EventID()).Error})
  if err==nil {t.Fatal("extra child committed")};empty(s)
 })
}
