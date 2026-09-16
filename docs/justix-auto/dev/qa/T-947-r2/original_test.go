package membershipqa_test

import (
 "bytes"
 "crypto/sha256"
 "fmt"
 "strings"
 "testing"

 "justixauto/pkg/events"
 "justixauto/pkg/inbox"
)

var limits = inbox.MembershipLimits{MaxBytes: 1<<20, MaxValues: 100000, MaxDepth: 64}
func digest(s string) inbox.MembershipDigest { return inbox.MembershipDigest(sha256.Sum256([]byte(s))) }
func proof(s string) inbox.MembershipEvidence { return inbox.MembershipEvidence{Reference:s,Digest:digest(s)} }
func id(n int) string { return fmt.Sprintf("10000000-0000-4000-8000-%012x", n) }

// An independently assembled full finite universe with no selected consumers.
// One real message still has its single explicit empty initial enrollment.
func emptyRequest(t *testing.T) inbox.TransitionRequestSpec {
 t.Helper()
 c,e := inbox.NewCatalog(inbox.CatalogSpec{Format:inbox.MembershipFormat,Owner:events.OwnerIdentity,Version:" \t",Approval:proof("catalog"),BindingManifest:proof("bindings")}, limits)
 if e != nil { t.Fatal(e) }
 s,e := inbox.NewSelection(inbox.SelectionSpec{Format:inbox.MembershipFormat,Owner:events.OwnerIdentity,CatalogVersion:" \t",CatalogDigest:c.Digest(),Approval:proof("empty owner selection")},c,limits)
 if e != nil { t.Fatal(e) }
 stream := inbox.MembershipStream{SourceOwner:events.OwnerInventory,AggregateType:"vehicle",AggregateID:id(1)}
 u,e := inbox.NewStreamUniverse(inbox.StreamUniverseSpec{Format:inbox.MembershipFormat,Owner:events.OwnerIdentity,Authority:proof("finite universe"),Streams:[]inbox.UniverseMember{{Stream:stream,Selectors:[]inbox.MembershipContract{{ID:"source selector",Version:1,Digest:digest("selector")}},SourceVerification:proof("source"),Disposition:proof("retained")}}},limits)
 if e != nil { t.Fatal(e) }
 zero := proof("explicit no applicable consumers")
 return inbox.TransitionRequestSpec{Format:inbox.MembershipFormat,Owner:events.OwnerIdentity,RequestID:id(2),Action:"initial",ResultEpoch:1,Approval:proof("request"),Catalog:c.Specification(),Selection:s.Specification(),Universe:u.Specification(),EffectsDigest:digest("all effects"),Streams:[]inbox.StreamSelectionSpec{{Format:inbox.MembershipFormat,Owner:events.OwnerIdentity,Stream:stream,CatalogVersion:c.Specification().Version,CatalogDigest:c.Digest(),SelectionDigest:s.Digest(),UniverseDigest:u.Digest(),Authority:proof("stream"),SourceProof:proof("source"),Boundary:1,ZeroApplicable:&zero,Backlog:[]inbox.EnrollmentEffect{{EventID:id(3),EnrollmentID:id(4),Position:1,EnvelopeDigest:digest("envelope"),Phase:"initial",EffectDigest:digest("enrollment")}}}}}
}

func TestQAEmptyEnrollmentAndCanonicalInputs(t *testing.T) {
 r,e := inbox.NewTransitionRequest(emptyRequest(t),limits)
 if e != nil { t.Fatal(e) }
 if r.Digest()!=digest(string(r.Bytes())) { t.Fatal("digest differs from exact bytes") }
 if _,e = inbox.DecodeTransitionRequest(r.Bytes(),limits); e != nil { t.Fatal(e) }
 for name,raw := range map[string][]byte{
  "duplicate decoded key":bytes.Replace(r.Bytes(),[]byte(`"format":"membership-v1"`),[]byte(`"format":"membership-v1","\u0066ormat":"membership-v1"`),1),
  "lone surrogate":bytes.Replace(r.Bytes(),[]byte(`"version":" \t"`),[]byte(`"version":"\udfff"`),1),
  "case alias":bytes.Replace(r.Bytes(),[]byte(`"format"`),[]byte(`"FORMAT"`),1),
  "trailing object":append(r.Bytes(),[]byte(`{}`)...),
  "noncanonical whitespace":append([]byte(" "),r.Bytes()...),
  "explicit array null":bytes.Replace(r.Bytes(),[]byte(`"features":[]`),[]byte(`"features":null`),1),
 } {
  t.Run(name,func(t *testing.T){v,e := inbox.DecodeTransitionRequest(raw,limits);if e==nil || v.Valid(){t.Fatal("accepted malformed identity")}})
 }
 for name,mutate := range map[string]func(*inbox.TransitionRequestSpec){
  "missing stream":func(s *inbox.TransitionRequestSpec){s.Streams=nil},
  "missing zero authority":func(s *inbox.TransitionRequestSpec){s.Streams[0].ZeroApplicable=nil},
  "duplicate enrollment key":func(s *inbox.TransitionRequestSpec){s.Streams[0].Backlog=append(s.Streams[0].Backlog,s.Streams[0].Backlog[0])},
  "empty late delta":func(s *inbox.TransitionRequestSpec){s.Streams[0].Backlog[0].Phase="late"},
  "changed position for same event":func(s *inbox.TransitionRequestSpec){x:=s.Streams[0].Backlog[0];x.EnrollmentID=id(5);x.Position=2;s.Streams[0].Boundary=2;s.Streams[0].Backlog=append(s.Streams[0].Backlog,x)},
 } { t.Run(name,func(t *testing.T){s:=r.Specification();mutate(&s);if v,e:=inbox.NewTransitionRequest(s,limits);e==nil || v.Valid(){t.Fatal("accepted contradictory identity")}}) }
 original:=r.Bytes(); returned:=r.Specification();returned.Streams[0].ZeroApplicable.Reference="mutated";returned.Streams[0].Backlog[0].Position=9
 copied:=r.Bytes();copied[0]='!';if !bytes.Equal(original,r.Bytes()){t.Fatal("escaped mutable storage")}
 if !strings.Contains(string(original),`"version":" \t"`){t.Fatal("opaque version label changed")}
}

func TestQARejectTwoEmptyInitialEnrollmentsForOneEvent(t *testing.T) {
 s := emptyRequest(t)
 extra:=s.Streams[0].Backlog[0]
 extra.EnrollmentID=id(5)
 extra.EffectDigest=digest("second empty enrollment")
 s.Streams[0].Backlog=append(s.Streams[0].Backlog,extra)
 r,e:=inbox.NewTransitionRequest(s,limits)
 if e==nil {
  decoded,de:=inbox.DecodeTransitionRequest(r.Bytes(),limits)
  t.Fatalf("accepted two distinct initial enrollments for event %s: constructor Valid=%v, canonical decoder Valid=%v err=%v, backlog=%d",extra.EventID,r.Valid(),decoded.Valid(),de,len(decoded.Specification().Streams[0].Backlog))
 }
 if r.Valid(){t.Fatal("error returned a valid value")}
}
