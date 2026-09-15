package independentauth

import (
 "context"
 "reflect"
 "testing"
)

// Independent representative interfaces from the revised proposal. Exchange
// request/response declarations are separately extracted from the exact source.
type Outcome uint8
const (Valid Outcome = iota+1; Mismatch; FatalPolicy; FatalConfiguration; FatalBinding)
type Session struct{Revision, ContextRevision string}
type ErrorResponse struct{Code string}
type Semantics interface {
 CheckReady() Outcome
 ValidateSession(Session) Outcome
 ValidateError(string,int,ErrorResponse) Outcome
}
type binding struct{ ready, named, errorHook func()Outcome }
func(b *binding)CheckReady()Outcome{return b.ready()}
func(b *binding)ValidateSession(Session)Outcome{return b.named()}
func(b *binding)ValidateError(string,int,ErrorResponse)Outcome{return b.errorHook()}
var _ Semantics=(*binding)(nil)
type nilMap map[string]string
func(nilMap)CheckReady()Outcome{return Valid}
func(nilMap)ValidateSession(Session)Outcome{return Valid}
func(nilMap)ValidateError(string,int,ErrorResponse)Outcome{return Valid}
func nilValue(v any)bool{
 if v==nil{return true};r:=reflect.ValueOf(v)
 switch r.Kind(){case reflect.Chan,reflect.Func,reflect.Interface,reflect.Map,reflect.Pointer,reflect.Slice:return r.IsNil()};return false
}
func guard(fn func()Outcome)(out Outcome){
 out=FatalBinding
 defer func(){if recover()!=nil{out=FatalBinding}}()
 r:=fn();switch r{case Valid,Mismatch,FatalPolicy,FatalConfiguration,FatalBinding:return r};return FatalBinding
}
func ready(s Semantics)Outcome{r:=guard(s.CheckReady);if r==Mismatch{return FatalBinding};return r}
func invoke(s Semantics,fn func()Outcome)Outcome{if r:=ready(s);r!=Valid{return r};return guard(fn)}
type basicExchange struct{}
func(*basicExchange)Exchange(context.Context,ContractRequest)(ContractResponse,error){return ContractResponse{Status:200},nil}
type v1Exchange struct{basicExchange}
func(*v1Exchange)ResponseContractVersion()int{return 1}
func construct(exchange ContractExchange,s Semantics)Outcome{
 if nilValue(exchange)||nilValue(s){return FatalBinding}
 version,ok:=exchange.(interface{ResponseContractVersion()int});if !ok||version.ResponseContractVersion()!=1{return FatalBinding}
 return ready(s)
}
func fixed(r Outcome)func()Outcome{return func()Outcome{return r}}
// Structural alternatives are selected completely, independently of semantic
// hooks. Only explicit Mismatch is ignored; every other fault aborts selection.
func selectPlan(alternatives []func()Outcome)([]int,Outcome){
 plan:=[]int{}
 for i,fn:=range alternatives{r:=guard(fn);switch r{case Valid:plan=append(plan,i);case Mismatch:default:return nil,r}}
 if len(plan)!=1{return nil,Mismatch};return plan,Valid
}
func validate(s Semantics,alternatives []func()Outcome,semanticCalls *int)Outcome{
 if r:=ready(s);r!=Valid{return r}
 if _,r:=selectPlan(alternatives);r!=Valid{return r}
 *semanticCalls++
 if r:=invoke(s,func()Outcome{return s.ValidateSession(Session{"2","1"})});r!=Valid{return r}
 return invoke(s,func()Outcome{return s.ValidateError("ReadSession",401,ErrorResponse{"SESSION_REQUIRED"})})
}
func TestEveryHookExactOutcome(t *testing.T){
 for _,category:=range []string{"ready","named","error"}{
  for _,value:=range []Outcome{0,Valid,Mismatch,FatalPolicy,FatalConfiguration,FatalBinding,6,255}{
   t.Run(category+"/"+string(rune('A'+value)),func(t *testing.T){
    s:=&binding{fixed(Valid),fixed(Valid),fixed(Valid)}
    switch category{case "ready":s.ready=fixed(value);case "named":s.named=fixed(value);case "error":s.errorHook=fixed(value)}
    calls:=0;got:=validate(s,[]func()Outcome{fixed(Valid),fixed(Mismatch)},&calls)
    want:=value;if value==0||value>FatalBinding||category=="ready"&&value==Mismatch{want=FatalBinding}
    if got!=want{t.Fatalf("got %d want %d",got,want)}
    if category=="ready"&&value!=Valid&&calls!=0{t.Fatal("hook ran despite readiness failure")}
   })
  }
 }
}
func TestPanicAndNilConstruction(t *testing.T){
 for _,category:=range []string{"ready","named","error"}{t.Run(category,func(t *testing.T){
  boom:=func()Outcome{panic("synthetic secret")};s:=&binding{fixed(Valid),fixed(Valid),fixed(Valid)}
  switch category{case "ready":s.ready=boom;case "named":s.named=boom;case "error":s.errorHook=boom}
  calls:=0;if validate(s,[]func()Outcome{fixed(Valid)},&calls)!=FatalBinding{t.Fatal("panic accepted")}
 })}
 good:=&binding{fixed(Valid),fixed(Valid),fixed(Valid)};var absent *binding;var absentMap nilMap;var absentExchange *v1Exchange
 for _,s:=range []Semantics{nil,absent,absentMap}{if construct(&v1Exchange{},s)!=FatalBinding{t.Fatal("typed nil accepted")}}
 if construct(absentExchange,good)!=FatalBinding||construct(&basicExchange{},good)!=FatalBinding{t.Fatal("invalid exchange accepted")}
 if construct(&v1Exchange{},good)!=Valid{t.Fatal("valid interfaces rejected")}
}
func TestStructuralAndSemanticSeparation(t *testing.T){
 cases:=[]struct{name string; shapes []func()Outcome; hook Outcome;want Outcome;hooks int}{
  {"ambiguous cannot use mismatch",[]func()Outcome{fixed(Valid),fixed(Valid)},Mismatch,Mismatch,0},
  {"ambiguous cannot use fault",[]func()Outcome{fixed(Valid),fixed(Valid)},FatalPolicy,Mismatch,0},
  {"structural fault with valid alternative",[]func()Outcome{fixed(FatalConfiguration),fixed(Valid)},Valid,FatalConfiguration,0},
  {"structural panic with valid alternative",[]func()Outcome{func()Outcome{panic("synthetic")},fixed(Valid)},Valid,FatalBinding,0},
  {"selected mismatch no fallback",[]func()Outcome{fixed(Valid),fixed(Mismatch)},Mismatch,Mismatch,1},
  {"selected fault no fallback",[]func()Outcome{fixed(Mismatch),fixed(Valid)},FatalPolicy,FatalPolicy,1},
  {"unique valid",[]func()Outcome{fixed(Mismatch),fixed(Valid)},Valid,Valid,1},
 }
 for _,c:=range cases{t.Run(c.name,func(t *testing.T){s:=&binding{fixed(Valid),fixed(c.hook),fixed(Valid)};calls:=0
  if got:=validate(s,c.shapes,&calls);got!=c.want||calls!=c.hooks{t.Fatalf("outcome %d calls %d",got,calls)}
 })}
}
