package membershipqa_test

import (
 "bytes"
 "encoding/json"
 "errors"
 "fmt"
 "testing"
 "justixauto/pkg/inbox"
)

func raw(t *testing.T,v any) []byte {
 t.Helper();var b bytes.Buffer;e:=json.NewEncoder(&b);e.SetEscapeHTML(false)
 if err:=e.Encode(v);err!=nil{t.Fatal(err)}
 return bytes.TrimSuffix(b.Bytes(),[]byte("\n"))
}
func canonical(t *testing.T,s inbox.TransitionRequestSpec) inbox.TransitionRequestSpec {
 t.Helper();v,e:=inbox.NewTransitionRequest(s,limits);if e!=nil{t.Fatal(e)};return v.Specification()
}
func checkAll(t *testing.T,s inbox.TransitionRequestSpec,reject bool) {
 t.Helper()
 c,e:=inbox.NewCatalog(s.Catalog,limits);if e!=nil{t.Fatal(e)}
 o,e:=inbox.NewSelection(s.Selection,c,limits);if e!=nil{t.Fatal(e)}
 u,e:=inbox.NewStreamUniverse(s.Universe,limits);if e!=nil{t.Fatal(e)}
 check:=func(name string,valid bool,e error){t.Helper();if reject {if !errors.Is(e,inbox.ErrMembershipIdentity)||valid{t.Fatalf("%s accepted contradictory identity: valid=%v err=%v",name,valid,e)}} else if e!=nil||!valid{t.Fatalf("%s rejected valid shape: %v",name,e)}}
 r,e:=inbox.NewTransitionRequest(s,limits);check("request constructor",r.Valid(),e)
 rd,e:=inbox.DecodeTransitionRequest(raw(t,s),limits);check("request decoder",rd.Valid(),e)
 v,e:=inbox.NewStreamSelection(s.Streams[0],c,o,u,limits);check("stream constructor",v.Valid(),e)
 vd,e:=inbox.DecodeStreamSelection(raw(t,s.Streams[0]),c,o,u,limits);check("stream decoder",vd.Valid(),e)
 if !reject && (!bytes.Equal(r.Bytes(),rd.Bytes())||r.Digest()!=rd.Digest()||!bytes.Equal(v.Bytes(),vd.Bytes())||v.Digest()!=vd.Digest()){t.Fatal("round-trip identity drift")}
}

func TestQAFixFourEntryPoints(t *testing.T) {
 base:=canonical(t,emptyRequest(t));checkAll(t,base,false)
 for _,count:=range []int{2,3} {for _,changedEffect:=range []bool{false,true}{
  t.Run(fmt.Sprintf("empty-initial-count-%d/changed-effect-%v",count,changedEffect),func(t *testing.T){
   s:=canonical(t,base)
   for n:=1;n<count;n++{extra:=s.Streams[0].Backlog[0];extra.EnrollmentID=id(4+n);if changedEffect{extra.EffectDigest=digest(fmt.Sprint(n))};s.Streams[0].Backlog=append(s.Streams[0].Backlog,extra)}
   checkAll(t,s,true)
  })
 }}
 t.Run("distinct-empty-events",func(t *testing.T){
  s:=canonical(t,base);s.Streams[0].Boundary=2;x:=s.Streams[0].Backlog[0];x.EventID=id(30);x.EnrollmentID=id(40);x.Position=2;x.EnvelopeDigest=digest("second event");s.Streams[0].Backlog=append(s.Streams[0].Backlog,x)
  checkAll(t,s,false)
 })
 t.Run("zero-messages",func(t *testing.T){s:=canonical(t,base);s.Streams[0].Backlog=[]inbox.EnrollmentEffect{};checkAll(t,s,false)})
}

// Create two approved-shaped consumers independently of the implementation's fixture.
func lateRequest(t *testing.T) inbox.TransitionRequestSpec {
 t.Helper();s:=emptyRequest(t);contract:=func(n string)inbox.MembershipContract{return inbox.MembershipContract{ID:n,Version:1,Digest:digest(n)}}
 feature:="identity/projection/qa";s.Catalog.Features=[]inbox.MembershipFeature{{ID:feature,Contract:contract(feature)}}
 for _,name:=range []string{"qa-a","qa-b"}{
  s.Catalog.Claims=append(s.Catalog.Claims,inbox.CatalogClaim{Format:inbox.MembershipFormat,Meaning:inbox.ConsumerMeaning{Format:inbox.MembershipFormat,Owner:s.Owner,FeatureID:feature,Name:name,Kind:"projection",Generation:" \t",Subscription:contract("subscription"),SourceOwner:s.Universe.Streams[0].Stream.SourceOwner,StreamID:"source selector",Selector:s.Universe.Streams[0].Selectors[0],SchemaSemantics:proof("schema"),RoutingKeys:[]string{"inventory.qa.v1"},Schemas:[]inbox.MembershipSchema{{EventType:"inventory.qa.changed.v1",Version:1}}},Binding:inbox.ConsumerBinding{Exchange:"owner.inventory",Queue:"identity.qa",Approval:proof("bindings")}})
 }
 c,e:=inbox.NewCatalog(s.Catalog,limits);if e!=nil{t.Fatal(e)};s.Catalog=c.Specification();s.Selection.CatalogDigest=c.Digest()
 stream:=&s.Streams[0];stream.CatalogDigest=c.Digest();stream.ZeroApplicable=nil;stream.Backlog=nil
 noSnapshot,newEmpty:="no-snapshot","new-empty"
 for n,claim:=range s.Catalog.Claims{
  name:=claim.Meaning.Name;_,h,e:=c.ClaimBytes(name);if e!=nil{t.Fatal(e)};admission:=id(100+n)
  s.Selection.Consumers=append(s.Selection.Consumers,inbox.SelectedConsumer{Name:name,ClaimDigest:h,State:"active",Disposition:proof("selected")})
  stream.Consumers=append(stream.Consumers,inbox.StreamConsumer{Name:name,ClaimDigest:h,Kind:"projection",Generation:" \t",AdmissionRootID:admission,AdmissionTipID:admission,Bootstrap:inbox.BootstrapIdentity{ID:id(200+n),RequestID:s.RequestID,AdmissionID:admission,AuthorityRef:"authority",ScopeRef:"scope",PurposeRef:"purpose",SourceCheckpointRef:"checkpoint",NoSnapshotContractRef:&noSnapshot,NewEmptyProofRef:&newEmpty},Disposition:proof("included"),EffectDigest:digest(name)})
  stream.Backlog=append(stream.Backlog,inbox.EnrollmentEffect{EventID:id(3),EnrollmentID:id(4+n),Position:1,EnvelopeDigest:digest("envelope"),Phase:"late",Consumers:[]inbox.EnrollmentConsumer{{Name:name,AdmissionID:admission}},EffectDigest:digest(name)})
 }
 o,e:=inbox.NewSelection(s.Selection,c,limits);if e!=nil{t.Fatal(e)};s.Selection=o.Specification();stream.SelectionDigest=o.Digest()
 return canonical(t,s)
}
func TestQAFixLateDeltaIsolation(t *testing.T){
 s:=lateRequest(t);checkAll(t,s,false)
 t.Run("duplicate-consumer-across-late-enrollments",func(t *testing.T){x:=canonical(t,s);x.Streams[0].Backlog[1].Consumers=x.Streams[0].Backlog[0].Consumers;checkAll(t,x,true)})
 t.Run("two-initial-with-consumers",func(t *testing.T){x:=canonical(t,s);a:=x.Streams[0].Backlog[0];a.Phase="initial";a.Consumers=append(a.Consumers,x.Streams[0].Backlog[1].Consumers...);b:=a;b.EnrollmentID=id(5);x.Streams[0].Backlog=[]inbox.EnrollmentEffect{a,b};checkAll(t,x,true)})
}
