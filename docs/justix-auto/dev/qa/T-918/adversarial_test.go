package outbox

import (
 "context"
 "crypto/sha256"
 "encoding/json"
 "errors"
 "strings"
 "testing"

 "github.com/google/uuid"
 "justixauto/pkg/events"
 "justixauto/pkg/eventstore"
)

// Loaded into the exact package using go -overlay. Reuses only the disposable
// pinned PostgreSQL installer and value fixtures; assertions are independent QA.
func TestQA918Adversarial(t *testing.T) {
 db := sourceDatabase(t)
 runner := sourceRunner(t, db)
 ctx := context.Background()
 run := func(s SourceStream, fn func(sourcePorts) error) error {
  return runner.Run(ctx, func(p sourcePorts) error {
   if err := p.admissions.Fence(ctx, s); err != nil { return err }
   return fn(p)
  })
 }
 must := func(err error) { t.Helper(); if err != nil { t.Fatal(err) } }
 t.Run("foreign owner cannot fence inventory database", func(t *testing.T) {
  s, err := NewSourceStream(events.OwnerRetail, "fixture", uuid.NewString()); must(err)
  err = runner.Run(ctx, func(p sourcePorts) error {
   foreign, err := NewSourceAdmissions(p.tx, events.OwnerRetail); if err != nil { return err }
   return foreign.Fence(ctx, s)
  })
  if !errors.Is(err, ErrInvalidAdmission) { t.Fatalf("foreign owner accepted: %v", err) }
 })
 t.Run("runtime cannot rewrite admission or mode evidence", func(t *testing.T) {
  s := sourceStream(t); in := sourceInput(s, events.OwnerRetail, 0)
  must(run(s, func(p sourcePorts) error { return p.admissions.Admit(ctx, sourceAdmit(t, in)) }))
  for _, query := range []string{
   "UPDATE eventstore.messaging_admissions SET authority_ref='qa-tampered' WHERE admission_id='"+in.ID+"'",
   "DELETE FROM eventstore.messaging_admissions WHERE admission_id='"+in.ID+"'",
   "UPDATE eventstore.messaging_mode SET mode='legacy' WHERE singleton",
  } { if err := db.Exec(query).Error; err == nil { t.Fatalf("runtime mutation succeeded: %s", query) } }
 })
 t.Run("internal revisions do not skip integration positions", func(t *testing.T) {
  s := sourceStream(t); in := sourceInput(s, events.OwnerRetail, 0)
  must(run(s, func(p sourcePorts) error {
   if err := p.admissions.Admit(ctx, sourceAdmit(t, in)); err != nil { return err }
   batch := []eventstore.Event{sourceEvent(t,s,1,0),sourceEvent(t,s,2,1),sourceEvent(t,s,3,0),sourceEvent(t,s,4,2)}
   if err := p.append.Append(ctx,eventstore.Batch{Events:batch}); err != nil { return err }
   for _, index := range []int{1,3} {
    plan, err := p.admissions.Resolve(ctx,sourceRule(t,s,false),sourceEnvelope(batch[index])); if err != nil { return err }
    if len(plan.Recipients()) != 1 || plan.Recipients()[0].AdmissionID != in.ID || plan.Sequence().Int64() != int64((index+1)/2) { t.Fatal("position or recipient lost") }
    if err := p.admissions.ValidatePlan(ctx,plan); err != nil { return err }
   }
   return nil
  }))
 })
 t.Run("recipient copy cannot change candidate or admission digest", func(t *testing.T) {
  s := sourceStream(t); in := sourceInput(s, events.OwnerRetail, 0)
  must(run(s,func(p sourcePorts) error {
   if err := p.admissions.Admit(ctx,sourceAdmit(t,in)); err != nil { return err }
   e := sourceEvent(t,s,1,1)
   if err := p.append.Append(ctx,eventstore.Batch{Events:[]eventstore.Event{e}}); err != nil { return err }
   plan, err := p.admissions.Resolve(ctx,sourceRule(t,s,false),sourceEnvelope(e)); if err != nil { return err }
   copy := plan.Recipients(); copy[0].Destination=events.OwnerDocuments; copy[0].AdmissionID=uuid.NewString(); copy[0].Contract.Digest[0]++
   got := plan.Recipients()[0]
   if got.Destination != in.Destination || got.AdmissionID != in.ID || got.Contract != in.Contract { t.Fatal("candidate aliased") }
   return p.admissions.ValidatePlan(ctx,plan)
  }))
 })
 t.Run("foreign security hold and sent status survive source hold resume", func(t *testing.T) {
  s := sourceStream(t); in := sourceInput(s,events.OwnerRetail,0)
  e1,e2 := sourceEvent(t,s,1,1),sourceEvent(t,s,2,2)
  must(run(s,func(p sourcePorts) error {
   if err := p.admissions.Admit(ctx,sourceAdmit(t,in)); err != nil { return err }
   if err := p.append.Append(ctx,eventstore.Batch{Events:[]eventstore.Event{e1,e2}}); err != nil { return err }
   for _,e := range []eventstore.Event{e1,e2} {
    plan,err := p.admissions.Resolve(ctx,sourceRule(t,s,false),sourceEnvelope(e)); if err != nil { return err }
    if err := sourceStageFixture(p.tx,plan,sourceEnvelope(e)); err != nil { return err }
   }; return nil
  }))
  must(db.Exec("UPDATE eventstore.outbox_deliveries SET hold_ref='qa-independent-security-hold' WHERE event_id=?::uuid",sourceEnvelope(e1).EventID()).Error)
  must(db.Exec("UPDATE eventstore.outbox_deliveries SET sent_at=clock_timestamp() WHERE event_id=?::uuid",sourceEnvelope(e2).EventID()).Error)
  hold := SourceAdmissionChange{ID:uuid.NewString(),PriorID:in.ID,Action:AdmissionHold,Checkpoint:sourceRevision(2),AuthorityRef:"qa-hold",EvidenceRef:"qa-evidence"}
  must(run(s,func(p sourcePorts) error { return p.admissions.Change(ctx,s,hold) }))
  resume := SourceAdmissionChange{ID:uuid.NewString(),PriorID:hold.ID,Action:AdmissionResume,Checkpoint:sourceRevision(2),AuthorityRef:"qa-resume",EvidenceRef:"qa-evidence",RetainedWorkAuthorityRef:"qa-retained-work"}
  must(run(s,func(p sourcePorts) error { return p.admissions.Change(ctx,s,resume) }))
  var n int64
  must(db.Raw("SELECT count(*) FROM eventstore.outbox_deliveries WHERE (event_id=?::uuid AND hold_ref='qa-independent-security-hold' AND sent_at IS NULL) OR (event_id=?::uuid AND sent_at IS NOT NULL AND hold_ref IS NULL)",sourceEnvelope(e1).EventID(),sourceEnvelope(e2).EventID()).Scan(&n).Error)
  if n != 2 { t.Fatal("foreign hold or sent status changed") }
 })
 t.Run("closed held range resumes retained work without reopening", func(t *testing.T) {
  s := sourceStream(t); in:=sourceInput(s,events.OwnerRetail,0)
  must(run(s,func(p sourcePorts) error { return p.admissions.Admit(ctx,sourceAdmit(t,in)) }))
  prior:=in.ID
  for _, action := range []AdmissionAction{AdmissionHold,AdmissionClose,AdmissionResume} {
   change:=SourceAdmissionChange{ID:uuid.NewString(),PriorID:prior,Action:action,AuthorityRef:"qa-change",EvidenceRef:"qa-checkpoint-zero",RetainedWorkAuthorityRef:"qa-retained-work"}
   must(run(s,func(p sourcePorts) error { return p.admissions.Change(ctx,s,change) })); prior=change.ID
  }
  err:=run(s,func(p sourcePorts) error {
   e:=sourceEvent(t,s,1,1)
   if err:=p.append.Append(ctx,eventstore.Batch{Events:[]eventstore.Event{e}});err!=nil{return err}
   _,err:=p.admissions.Resolve(ctx,sourceRule(t,s,false),sourceEnvelope(e));return err
  })
  if !errors.Is(err,ErrMissingRecipientPlan){t.Fatalf("closed admission reopened: %v",err)}
 })
 t.Run("second adapter in same SQL transaction cannot reuse issuer candidate",func(t *testing.T){
  s:=sourceStream(t)
  err:=run(s,func(p sourcePorts)error{
   e:=sourceEvent(t,s,1,1)
   if err:=p.append.Append(ctx,eventstore.Batch{Events:[]eventstore.Event{e}});err!=nil{return err}
   plan,err:=p.admissions.Resolve(ctx,sourceRule(t,s,true),sourceEnvelope(e));if err!=nil{return err}
   other,err:=NewSourceAdmissions(p.tx,events.OwnerInventory);if err!=nil{return err}
   if err:=other.Fence(ctx,s);err!=nil{return err}
   return other.ValidatePlan(ctx,plan)
  })
  if !errors.Is(err,ErrInvalidAdmission){t.Fatalf("candidate escaped issuer: %v",err)}
 })
 t.Run("direct SQL branch fails complete replay",func(t *testing.T){
  s:=sourceStream(t);in:=sourceInput(s,events.OwnerRetail,0)
  must(run(s,func(p sourcePorts)error{return p.admissions.Admit(ctx,sourceAdmit(t,in))}))
  hold:=SourceAdmissionChange{ID:uuid.NewString(),PriorID:in.ID,Action:AdmissionHold,AuthorityRef:"qa-hold",EvidenceRef:"qa-evidence"}
  must(run(s,func(p sourcePorts)error{return p.admissions.Change(ctx,s,hold)}))
  must(db.Exec(`INSERT INTO eventstore.messaging_admissions(admission_id,namespace,source_owner,aggregate_type,aggregate_id,subject,action,prior_admission_id,contract_id,contract_version,contract_digest,schema_allowlist,authority_ref,scope_ref,purpose,bootstrap_ref,start_after,end_inclusive)
   SELECT ?::uuid,namespace,source_owner,aggregate_type,aggregate_id,subject,action,prior_admission_id,contract_id,contract_version,contract_digest,schema_allowlist,authority_ref,scope_ref,purpose,bootstrap_ref,start_after,end_inclusive FROM eventstore.messaging_admissions WHERE admission_id=?::uuid`,uuid.NewString(),hold.ID).Error)
  err:=run(s,func(p sourcePorts)error{return p.admissions.Admit(ctx,sourceAdmit(t,sourceInput(s,events.OwnerDocuments,0)))})
  if !errors.Is(err,ErrAdmissionConflict){t.Fatalf("branch accepted: %v",err)}
 })
 t.Run("tracked T917 canonical route rejection remains explicit",func(t *testing.T){
  s:=sourceStream(t);in:=sourceInput(s,events.OwnerRetail,0)
  err:=run(s,func(p sourcePorts)error{
   if err:=p.admissions.Admit(ctx,sourceAdmit(t,in));err!=nil{return err}
   event:=sourceEvent(t,s,1,1);e:=sourceEnvelope(event)
   if err:=p.append.Append(ctx,eventstore.Batch{Events:[]eventstore.Event{event}});err!=nil{return err}
   plan,err:=p.admissions.Resolve(ctx,sourceRule(t,s,false),e);if err!=nil{return err}
   if err:=p.admissions.ValidatePlan(ctx,plan);err!=nil{return err}
   body,err:=e.MarshalJSON();if err!=nil{return err};hash:=sha256.Sum256(body)
   recipients,_:=json.Marshal([]map[string]string{{"destination":"retail","admission_id":in.ID}})
   if err:=p.tx.Exec(`INSERT INTO eventstore.outbox_messages(event_id,source_owner,aggregate_type,aggregate_id,integration_sequence,envelope,envelope_hash,recipients,plan_authority_ref) VALUES(?::uuid,?,?,?::uuid,?,?,?,?::jsonb,?)`,e.EventID(),string(e.Owner()),e.AggregateType(),e.AggregateID(),e.IntegrationSequence().Int64(),body,hash[:],string(recipients),plan.AuthorityRef()).Error;err!=nil{return err}
   canonical:=route(e,events.OwnerRetail)
   if canonical!="inventory.retail.inventory.fixture.changed.v1"{t.Fatalf("route silently normalized: %s",canonical)}
   return p.tx.Exec(`INSERT INTO eventstore.outbox_deliveries(event_id,destination,admission_id,exchange,routing_key) VALUES(?::uuid,'retail',?::uuid,?,?)`,e.EventID(),in.ID,IntegrationExchange,canonical).Error
  })
  if err==nil||!strings.Contains(err.Error(),"delivery schema is not admitted") {t.Fatalf("unexpected route result: %v",err)}
  t.Log("Known separate T917 gate reproduced: canonical T011 route rejected; production admission resolver preserved route contract")
  var n int64
  must(db.Raw("SELECT count(*) FROM eventstore.outbox_messages WHERE aggregate_id=?::uuid",s.aggregateID).Scan(&n).Error)
  if n!=0{t.Fatal("route failure committed partial parent")}
 })
}
