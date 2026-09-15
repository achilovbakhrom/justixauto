package persistence_test

import (
 "bytes"
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "justixauto/pkg/persistence"
)

func TestIndependentR3CanonicalAndMalformedBoundaries(t *testing.T) {
 cases:=[]struct{name string; valid bool;rewrite func([]byte)[]byte}{
  {"canonical maximum identities",true,func(b []byte)[]byte{return b}},
  {"CRLF whitespace",true,func(b []byte)[]byte{var out bytes.Buffer;if err:=json.Indent(&out,b,"","\t");err!=nil{panic(err)};return []byte("\r\n"+strings.ReplaceAll(out.String(),"\n","\r\n")+" \t\r\n")}},
  {"escaped exact field names and filename",true,func(b []byte)[]byte{return []byte(strings.ReplaceAll(strings.ReplaceAll(string(b),`"SHA256":`,`"\u0053HA256":`),`migrations/`,`migrations\/`))}},
  {"UTF8 BOM",false,func(b []byte)[]byte{return append([]byte{0xef,0xbb,0xbf},b...)}},
  {"NUL trailer",false,func(b []byte)[]byte{return append(b,0)}},
  {"comment trailer",false,func(b []byte)[]byte{return append(b,[]byte(" /* ignored? */")...)}},
  {"leading comment",false,func(b []byte)[]byte{return append([]byte("// manifest\n"),b...)}},
  {"max version overflow",false,func(b []byte)[]byte{return bytes.ReplaceAll(b,[]byte(`"Version":9223372036854775807`),[]byte(`"Version":9223372036854775808`))}},
  {"max revision overflow",false,func(b []byte)[]byte{return bytes.ReplaceAll(b,[]byte(`"Revision":4294967295`),[]byte(`"Revision":4294967296`))}},
  {"same version exponent",false,func(b []byte)[]byte{return bytes.ReplaceAll(b,[]byte(`"Version":9223372036854775807`),[]byte(`"Version":9223372036854775807e0`))}},
  {"same revision fraction",false,func(b []byte)[]byte{return bytes.ReplaceAll(b,[]byte(`"Revision":4294967295`),[]byte(`"Revision":4294967295.0`))}},
  {"quoted maximum version",false,func(b []byte)[]byte{return bytes.ReplaceAll(b,[]byte(`"Version":9223372036854775807`),[]byte(`"Version":"9223372036854775807"`))}},
  {"leading zero byte",false,func(b []byte)[]byte{return bytes.ReplaceAll(b,[]byte(`"SHA256":[0,255`),[]byte(`"SHA256":[00,255`))}},
  {"positive-sign byte",false,func(b []byte)[]byte{return bytes.ReplaceAll(b,[]byte(`"SHA256":[0,255`),[]byte(`"SHA256":[+0,255`))}},
  {"nonfinite byte",false,func(b []byte)[]byte{return bytes.ReplaceAll(b,[]byte(`"SHA256":[0,255`),[]byte(`"SHA256":[NaN,255`))}},
  {"hex byte",false,func(b []byte)[]byte{return bytes.ReplaceAll(b,[]byte(`"SHA256":[0,255`),[]byte(`"SHA256":[0x0,255`))}},
  {"byte overflow at second position",false,func(b []byte)[]byte{return bytes.ReplaceAll(b,[]byte(`"SHA256":[0,255`),[]byte(`"SHA256":[0,256`))}},
  {"fractional byte at second position",false,func(b []byte)[]byte{return bytes.ReplaceAll(b,[]byte(`"SHA256":[0,255`),[]byte(`"SHA256":[0,255.0`))}},
  {"duplicate byte field escaped",false,func(b []byte)[]byte{return bytes.ReplaceAll(b,[]byte(`"feature_contract":{`),[]byte(`"feature_contract":{"\u0053HA256":[],`))}},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  root,err:=filepath.EvalSymlinks(t.TempDir());if err!=nil{t.Fatal(err)};if err=os.Mkdir(filepath.Join(root,"migrations"),0700);err!=nil{t.Fatal(err)}
  s:=spec();s.Head=9223372036854775807;s.Artifacts[1].Identity.Version=s.Head;s.Artifacts[1].Identity.Filename=fmt.Sprintf("migrations/%d_boundary.up.sql",s.Head)
  s.Artifacts[1].Feature.Revision=4294967295
  for j:=range s.Artifacts[1].Feature.SHA256{s.Artifacts[1].Feature.SHA256[j]=byte((j%2)*255)}
  s.Features[0].Identity=*s.Artifacts[1].Feature
  for i,a:=range s.Artifacts{
   body:="base";if i==1{body="feature12"};if hash(body)!=a.Identity.SHA256{t.Fatal("incorrect SQL digest")}
   if err=os.WriteFile(filepath.Join(root,a.Identity.Filename),[]byte(body),0600);err!=nil{t.Fatal(err)}
   b,err:=json.Marshal(persistence.ArtifactManifest{FormatRevision:1,Owner:s.Owner,Identity:a.Identity,Prerequisites:a.Prerequisites,Feature:a.Feature});if err!=nil{t.Fatal(err)}
   if i==1{original:=append([]byte{},b...);b=tc.rewrite(b);if tc.name!="canonical maximum identities"&&bytes.Equal(original,b){t.Fatal("rewrite did not exercise intended boundary")}}
   s.Artifacts[i].ManifestSHA256=hash(string(b));if err=os.WriteFile(filepath.Join(root,strings.TrimSuffix(a.Identity.Filename,".up.sql")+".manifest.json"),b,0600);err!=nil{t.Fatal(err)}
  }
  p:=profile(t,s);before:=p.Digest();err=p.VerifyFiles(root)
  if (err==nil)!=tc.valid{t.Fatalf("wanted valid=%v, got %v",tc.valid,err)}
  if p.Digest()!=before{t.Fatal("file check mutated sealed profile")}
  if tc.valid{
   // Canonical set order and copies cannot change the trusted identity; a
   // byte-different manifest does change it, even if decoded fields agree.
   c:=p.Specification();c.Features[0].Tables[0].InsertColumns=[]string{"body","id"}
   q:=profile(t,c);if q.Digest()!=p.Digest(){t.Fatal("grant-set order changed canonical profile identity")}
   c.Artifacts[0].ManifestSHA256=hash("different original manifest bytes")
   if profile(t,c).Digest()==p.Digest(){t.Fatal("original manifest byte identity was discarded")}
  }
 })}
}
