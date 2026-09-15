package outbox

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "testing"
 "time"

 "justixauto/pkg/events"
)

// Independent behavioral oracles reuse only the exact-commit disposable fixture
// bootstrap, actual producer and AMQP administration helpers. Loaded by overlay;
// no application/test source is edited.
func TestIndependentRelayDurability(t *testing.T) {
 ctx := context.Background()
 f := newRelayPG(t, RelayCustody, true)
 b := newRelayBroker(t)
 s := relayStore(t, f, 15*time.Second)
 publisher := b.publisher(t)
 isolate := func() { relayMustSQL(t, f, "UPDATE eventstore.outbox_deliveries SET next_attempt_at=clock_timestamp()+interval '1 day' WHERE sent_at IS NULL") }
 assertUnsent := func(id string) {
  t.Helper()
  var n int64
  if err := f.db.Raw("SELECT count(*) FROM eventstore.outbox_deliveries WHERE event_id=?::uuid AND sent_at IS NOT NULL", id).Scan(&n).Error; err != nil || n != 0 { t.Fatalf("sent unexpectedly: %d %v",n,err) }
 }
 t.Run("real receipts bind destination and copied accessors preserve broker bytes",func(t *testing.T) {
  isolate()
  env := f.seed(t,events.OwnerDocuments,events.OwnerRetail)
  original,err := env.MarshalJSON(); if err != nil { t.Fatal(err) }
  docs := relayClaim(t,s); retail := relayClaim(t,s)
  if docs.destination != "documents" || retail.destination != "retail" { t.Fatal("fixture ordering") }
  exposed := docs.Publication().Bytes(); exposed[0] ^= 0xff
  docReceipt,err := publisher.Publish(ctx,docs.Publication()); if err != nil {t.Fatal(err)}
  retailReceipt,err := publisher.Publish(ctx,retail.Publication()); if err != nil {t.Fatal(err)}
  if err := s.MarkSent(ctx,retail,docReceipt); !errors.Is(err,ErrRelayLease) {t.Fatal("cross-destination receipt accepted",err)}
  if err := s.MarkSent(ctx,docs,retailReceipt); !errors.Is(err,ErrRelayLease) {t.Fatal("cross-destination receipt accepted",err)}
  assertUnsent(env.EventID())
  for _,target := range []string{"documents","retail"} {
   d := b.get(t,target)
   if d.MessageId != env.EventID() || !bytes.Equal(d.Body,original) || d.RoutingKey != "inventory."+target+".inventory.fixture.changed.v1" {t.Fatal("broker identity changed")}
  }
  if err := s.MarkSent(ctx,docs,docReceipt); err != nil {t.Fatal(err)}
  if err := s.MarkSent(ctx,retail,retailReceipt); err != nil {t.Fatal(err)}
  if err := s.MarkSent(ctx,docs,docReceipt); !errors.Is(err,ErrRelayLease) {t.Fatal("duplicate completion changed sent history",err)}
 })
 t.Run("committed hold while completion waits retains broker accepted bytes unsent",func(t *testing.T) {
  isolate(); env := f.seed(t,events.OwnerRetail); l := relayClaim(t,s)
  receipt,err := publisher.Publish(ctx,l.Publication()); if err != nil {t.Fatal(err)}
  tx := f.db.Begin(); if tx.Error != nil {t.Fatal(tx.Error)}; defer tx.Rollback()
  if err := tx.Exec("UPDATE eventstore.outbox_deliveries SET hold_ref='independent-committed-hold' WHERE event_id=?::uuid",env.EventID()).Error; err != nil {t.Fatal(err)}
  done := make(chan error,1); go func(){ done <- s.MarkSent(ctx,l,receipt) }()
  select {case err := <-done: t.Fatal("completion did not wait for held row",err); case <-time.After(150*time.Millisecond):}
  if err := tx.Commit().Error; err != nil {t.Fatal(err)}
  if err := <-done; !errors.Is(err,ErrRelayLease) {t.Fatal("post-lock hold accepted",err)}
  assertUnsent(env.EventID()); relayState(t,s,l,DeliveryHeld)
  if err := s.Retry(ctx,l); !errors.Is(err,ErrRelayLease) {t.Fatal("retry cleared hold",err)}
  d := b.get(t,"retail"); if d.MessageId != env.EventID() {t.Fatal("accepted bytes missing")}
 })
 t.Run("unknown retry reconciles actual commit versus rollback without sent state",func(t *testing.T) {
  for _,committed := range []bool{false,true} {
   isolate(); env := f.seed(t,events.OwnerRetail)
   local := relayStore(t,f,15*time.Second); l := relayClaim(t,local); normal := local.db
   local.db = relayFaultDB(t,normal,committed)
   err := local.Retry(ctx,l); local.db = normal
   if !errors.Is(err,ErrRelayUnknown) {t.Fatal("retry uncertainty lost",committed,err)}
   want := DeliveryLeased; if committed {want=DeliveryPending}
   relayState(t,local,l,want); assertUnsent(env.EventID())
  }
 })
 t.Run("canceled completion cannot consume a real receipt",func(t *testing.T) {
  isolate(); env := f.seed(t,events.OwnerRetail); l := relayClaim(t,s)
  receipt,err := publisher.Publish(ctx,l.Publication()); if err != nil {t.Fatal(err)}
  canceled,cancel := context.WithCancel(ctx); cancel()
  if err := s.MarkSent(canceled,l,receipt); err == nil {t.Fatal("canceled completion succeeded")}
  assertUnsent(env.EventID()); relayState(t,s,l,DeliveryLeased)
  if err := s.MarkSent(ctx,l,receipt); err != nil {t.Fatal(err)}
  d := b.get(t,"retail"); if d.MessageId != env.EventID() {t.Fatal("broker identity")}
 })
 t.Run("additional privilege grants reject before attempts or publication",func(t *testing.T) {
  isolate(); env := f.seed(t,events.OwnerRetail)
  cases := []struct{grant,revoke string}{
   {"GRANT REFERENCES(event_id) ON eventstore.outbox_deliveries TO PUBLIC","REVOKE REFERENCES(event_id) ON eventstore.outbox_deliveries FROM PUBLIC"},
   {"GRANT MAINTAIN ON eventstore.outbox_messages TO justix_inventory_runtime","REVOKE MAINTAIN ON eventstore.outbox_messages FROM justix_inventory_runtime"},
   {"GRANT CREATE ON SCHEMA eventstore TO justix_inventory_runtime","REVOKE CREATE ON SCHEMA eventstore FROM justix_inventory_runtime"},
   {"REVOKE SELECT ON eventstore.outbox_messages FROM justix_inventory_runtime","GRANT SELECT ON eventstore.outbox_messages TO justix_inventory_runtime"},
  }
  for _,c := range cases {
   relayMustSQL(t,f,c.grant)
   called := false
   r,_ := NewRelay(s,relayPublisherFunc(func(context.Context,Publication)(RoutedConfirmation,error){called=true;return RoutedConfirmation{},nil}))
   l,err := r.Once(ctx); relayMustSQL(t,f,c.revoke)
   if err == nil || called || !l.IsEmpty() {t.Fatal("privilege violation performed work",c.grant,err,called)}
   var attempts int64
   if err := f.db.Raw("SELECT attempts FROM eventstore.outbox_deliveries WHERE event_id=?::uuid",env.EventID()).Scan(&attempts).Error; err != nil || attempts != 0 {t.Fatal("failed readiness mutated attempts",attempts,err)}
  }
  if err := s.Check(ctx); err != nil {t.Fatal("restored grants incompatible",err)}
 })
 t.Run("unknown claim remains unpublishable after authoritative reconciliation",func(t *testing.T) {
  isolate(); env := f.seed(t,events.OwnerRetail)
  local := relayStore(t,f,15*time.Second); normal := local.db
  local.db=relayFaultDB(t,normal,true); l,err := local.Claim(ctx); local.db=normal
  if !errors.Is(err,ErrRelayUnknown) {t.Fatal(err)}
  relayState(t,local,l,DeliveryLeased)
  receipt,err := publisher.Publish(ctx,l.Publication())
  if !errors.Is(err,ErrInvalidRecord) || receipt.routed {t.Fatal("unknown claim published",err)}
  assertUnsent(env.EventID())
  conn,ch:=b.adminChannel(t); defer conn.CloseDeadline(time.Now())
  if _,ok,err := ch.Get("justix.retail.inbox.v1",true); err != nil || ok {t.Fatal("unknown attempt reached broker",ok,err)}
 })
 t.Run("unchanged recipients remain durable through independent retry attempt",func(t *testing.T) {
  isolate(); env := f.seed(t,events.OwnerDocuments,events.OwnerRetail)
  before,err := f.admin(fmt.Sprintf("SELECT md5(string_agg(destination||routing_key||admission_id::text,',' ORDER BY destination)) FROM eventstore.outbox_deliveries WHERE event_id='%s'",env.EventID())); if err != nil {t.Fatal(before,err)}
  l := relayClaim(t,s)
  if err := s.Retry(ctx,l); err != nil {t.Fatal(err)}
  relayMustSQL(t,f,fmt.Sprintf("UPDATE eventstore.outbox_deliveries SET next_attempt_at=clock_timestamp() WHERE event_id='%s' AND destination='%s'",env.EventID(),l.destination))
  retry := relayClaim(t,s)
  if retry.publication.eventID != l.publication.eventID || !bytes.Equal(retry.Publication().Bytes(),l.Publication().Bytes()) || retry.token==l.token {t.Fatal("retained retry changed")}
  after,err := f.admin(fmt.Sprintf("SELECT md5(string_agg(destination||routing_key||admission_id::text,',' ORDER BY destination)) FROM eventstore.outbox_deliveries WHERE event_id='%s'",env.EventID())); if err != nil || before!=after {t.Fatal("recipient plan changed",before,after,err)}
 })
}
