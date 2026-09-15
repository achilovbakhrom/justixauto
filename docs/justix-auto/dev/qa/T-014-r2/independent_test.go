package projection_test

import (
 "context"
 "errors"
 "os/exec"
 "strings"
 "testing"

 "github.com/google/uuid"
 "gorm.io/gorm"
 "justixauto/pkg/eventstore"
 "justixauto/pkg/projection"
)

// QA overlay only: production files and committed tests remain untouched.
func TestIndependentT014(t *testing.T) {
 var volumes []string
 // Registered before fixture cleanup so only its exact inspected volumes are
 // removed after that fixture's container is gone. No global prune occurs.
 t.Cleanup(func() {
  for _, v := range volumes {
   if out, err := exec.Command("docker", "volume", "inspect", v).CombinedOutput(); err == nil || !strings.Contains(string(out), "no such volume") { t.Errorf("own volume absence not verified: %v %s", err, out) }
  }
 })
 f := newFixture(t)
 out, err := exec.Command("docker", "inspect", "--format", "{{range .Mounts}}{{if eq .Type \"volume\"}}{{println .Name}}{{end}}{{end}}", f.container).Output()
 if err != nil { t.Fatal(err) }
 volumes = strings.Fields(string(out))
 t.Logf("owned fixture anonymous volumes: %v", volumes)
 ctx := context.Background()
 makeGap := func(t *testing.T) (fixtureContract, projection.GapEvidence, []byte) {
  c := contract(t)
  install(t, f, c, bootstrap(t, c, 0, false))
  blocked := message(t, c, 3)
  _, err := consume(t, f.db, c, blocked, false, func() error { t.Error("gap ACK"); return nil })
  var gap *projection.Gap
  if !errors.As(err, &gap) { t.Fatal(err) }
  ev := gapEvidence()
  if err := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
   _, e := a.RecordGap(ctx, gap, ev, authorizeGap); return e
  }); err != nil { t.Fatal(err) }
  return c, ev, blocked
 }
 mutations := []struct{name string; change func(*projection.ContractInput)}{
  {"kind", func(c *projection.ContractInput){c.Kind="process"}},
  {"generation",func(c *projection.ContractInput){c.Generation="different-generation"}},
  {"contract-id",func(c *projection.ContractInput){c.ContractID="different-contract"}},
  {"contract-version",func(c *projection.ContractInput){c.ContractVersion++}},
  {"contract-digest",func(c *projection.ContractInput){c.ContractDigest[0]^=1}},
 }
 for _, mutation := range mutations {
  t.Run("changed-identity-"+mutation.name, func(t *testing.T){
   c, ev, blocked := makeGap(t)
   wrong := c
   mutation.change(&wrong.input)
   wrong.contract, err = projection.NewContract(wrong.input)
   if err != nil {t.Fatal(err)}
   // Existing read path provides a control: it rejects the wrong identity.
   err = run(t, f.db, wrong, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
    _, e := a.FindGap(ctx, position(t, wrong, blocked, false), authorizePosition); return e
   })
   if !errors.Is(err, projection.ErrReconciliationHold) {t.Fatalf("FindGap control: %v",err)}
   in := projection.AttemptInput{GapID:ev.GapID, AttemptID:uuid.NewString(), RequestID:uuid.NewString(),PriorAttemptID:ev.AttemptID,Action:"held",AuthorityRef:"synthetic-current-authority",HoldReasonCode:"source-unavailable"}
   err = run(t, f.db, wrong, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
    _, e := a.AppendAttempt(ctx,in,authorizeAttempt); return e
   })
   var retained int64
   if e := f.db.Table("eventstore.consumer_gap_attempts").Where("attempt_id=?",in.AttemptID).Count(&retained).Error; e != nil {t.Fatal(e)}
   if err == nil || retained != 0 {t.Errorf("changed %s accepted: error=%v persisted_attempts=%d",mutation.name,err,retained)}
  })
 }
 t.Run("current-authority-and-scope-controls",func(t *testing.T){
  c,ev,_ := makeGap(t)
  denied := errors.New("synthetic-authority-denied")
  in := projection.AttemptInput{GapID:ev.GapID,AttemptID:uuid.NewString(),RequestID:uuid.NewString(),PriorAttemptID:ev.AttemptID,Action:"held",AuthorityRef:"synthetic",HoldReasonCode:"source-unavailable"}
  err := run(t,f.db,c,eventstore.SharedFence,func(a *projection.Checkpoints[port],_ *gorm.DB)error{
   _,e:=a.AppendAttempt(ctx,in,func(context.Context,port,projection.AttemptInput)error{return denied});return e
  })
  if !errors.Is(err,denied){t.Fatal(err)}
  foreign := contract(t)
  install(t,f,foreign,bootstrap(t,foreign,0,false))
  if err:=run(t,f.db,foreign,eventstore.SharedFence,func(a *projection.Checkpoints[port],_ *gorm.DB)error{_,e:=a.AppendAttempt(ctx,in,authorizeAttempt);return e});err==nil{t.Fatal("foreign K accepted")}
  if _,err:=projection.ReadRecovery(ctx,f.db,c.contract,ev.GapID,ev.AttemptID,ev.AttemptRequestID,func(context.Context,projection.RecoveryRequest)error{return denied});!errors.Is(err,denied){t.Fatal(err)}
  var n int64
  if err:=f.db.Table("eventstore.consumer_gap_attempts").Where("gap_id=?",ev.GapID).Count(&n).Error;err!=nil||n!=1{t.Fatal(n,err)}
 })
 t.Run("runtime-history-mutation-denied",func(t *testing.T){
  c,ev,_:=makeGap(t)
  for _,q:=range []string{
   "UPDATE eventstore.consumer_gap_attempts SET authority_ref='changed' WHERE gap_id='"+ev.GapID+"'",
   "DELETE FROM eventstore.consumer_gaps WHERE gap_id='"+ev.GapID+"'",
   "UPDATE eventstore.consumer_bootstraps SET generation='changed' WHERE consumer_name='"+c.input.Consumer+"'",
   "UPDATE eventstore.consumer_checkpoints SET position=position+2,revision=revision+2 WHERE consumer_name='"+c.input.Consumer+"'",
  }{if err:=f.db.Exec(q).Error;err==nil{t.Errorf("forbidden SQL accepted: %s",q)}}
  if progress(t,f,c)!=0{t.Fatal("forbidden progress persisted")}
 })
}
