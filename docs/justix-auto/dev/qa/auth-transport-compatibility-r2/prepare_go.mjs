import {readFileSync,writeFileSync,mkdtempSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
const dir=path.dirname(fileURLToPath(import.meta.url)),root=path.resolve(dir,'../../../../..');
const source=readFileSync(path.join(root,'tools/generate-contracts.mjs'),'utf8');
const expression=source.slice(source.indexOf('const goRuntime = ')+18,source.indexOf('\nfunction renderGo')).trim().replace(/;$/,'');
const runtime=Function('return '+expression)();
const decoder=runtime.slice(runtime.indexOf('func contractRead('),runtime.indexOf('func contractDecode('));
const exchange=runtime.slice(runtime.indexOf('type ContractExchange interface'),runtime.indexOf('func contractJSONMedia('));
const temp=mkdtempSync('/private/tmp/justix-auth-r2-independent-go-');
writeFileSync(path.join(temp,'go.mod'),'module independentauth\n\ngo 1.27.1\n');
writeFileSync(path.join(temp,'semantics_test.go'),readFileSync(path.join(dir,'semantics_test.go')));
writeFileSync(path.join(temp,'exchange.go'),'package independentauth\nimport "context"\n'+exchange);
writeFileSync(path.join(temp,'decoder_test.go'),`package independentauth
import("bytes";"encoding/json";"errors";"io";"math";"math/big";"strconv";"strings";"unicode/utf8";"testing")
var contractInvalid=errors.New("invalid")
${decoder}
func TestLegacyResourceAcceptance(t *testing.T){
 t.Run("over8MiB",func(t *testing.T){raw:=[]byte("\\\""+strings.Repeat("x",8*1024*1024)+"\\\"");if len(raw)<=8*1024*1024{t.Fatal("fixture bound")};if _,err:=contractRead(raw);err!=nil{t.Fatal(err)}})
 t.Run("129containers",func(t *testing.T){raw:=[]byte(strings.Repeat("[",129)+"0"+strings.Repeat("]",129));if _,err:=contractRead(raw);err!=nil{t.Fatal(err)}})
}
func TestLegacyNumbers(t *testing.T){for _,c:=range []struct{raw string;valid bool}{{"1.0",true},{"1000e-3",true},{"1001e-3",false},{"1.0000000000000001",false},{"0e999999",true},{"1e-400",false},{"9007199254740991",true},{"9007199254740992",false}}{if _,err:=contractRead([]byte(c.raw));(err==nil)!=c.valid{t.Errorf("%s: %v",c.raw,err)}}}
`);
console.log(temp);
