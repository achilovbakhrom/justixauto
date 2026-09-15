package contracts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Explicit synthetic contracts only. This test does not activate an owner route
// or assert that a pending feature's OpenAPI/security policy has been approved.
const generationFixture = `{
 "openapi":"3.1.0","info":{"title":"Synthetic generation fixture","version":"1"},
 "components":{"securitySchemes":{"OwnerTLS":{"type":"mutualTLS"},"SessionCookie":{"type":"apiKey","in":"cookie","name":"__Host-justix_session"}},"schemas":{
  "Payload":{"type":"object","additionalProperties":false,"required":["amountMinor","nullable","revision","tags","metadata","count"],"properties":{
   "amountMinor":{"type":"string","pattern":"^(0|[1-9][0-9]*)(?![\\s\\S])","minLength":1,"maxLength":38},
   "revision":{"type":"string","pattern":"^(0|[1-9][0-9]*)(?![\\s\\S])","maxLength":19},
   "nullable":{"type":["string","null"]},"note":{"type":["string","null"]},
   "count":{"type":"integer","minimum":0,"maximum":100},
   "tags":{"type":"array","items":{"type":"string"},"uniqueItems":true},
   "metadata":{"type":"object","additionalProperties":{"type":"string"}}
  }},
  "Ready":{"type":"object","additionalProperties":false,"required":["kind","data"],"properties":{"kind":{"type":"string","const":"ready"},"data":{"$ref":"#/components/schemas/Payload"}}},
  "Pending":{"type":"object","additionalProperties":false,"required":["kind","operationId"],"properties":{"kind":{"type":"string","const":"pending"},"operationId":{"type":"string","format":"uuid"}}},
  "Choice":{"oneOf":[{"$ref":"#/components/schemas/Ready"},{"$ref":"#/components/schemas/Pending"}]},
  "Maybe":{"type":["string","null"]},
  "LooseEnd":{"type":"string","pattern":"^x$"},
  "Dates":{"type":"object","additionalProperties":false,"required":["date","instant"],"properties":{"date":{"type":"string","format":"date"},"instant":{"type":"string","format":"date-time"}}},
  "Problem":{"type":"object","additionalProperties":false,"required":["error"],"properties":{"error":{"type":"object","additionalProperties":false,"required":["code","message","fields","traceId"],"properties":{"code":{"type":"string","minLength":1},"message":{"type":"string"},"fields":{"type":"object","additionalProperties":{"type":"string"}},"traceId":{"type":"string"}}}}}
 }},
 "paths":{
  "/internal/v1/identity/synthetic/{id}":{"get":{"operationId":"ReadInternal","security":[{"OwnerTLS":[]}],"parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"Internal synthetic receipt","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Payload"}}}}}}},
  "/api/v1/identity/synthetic/{id}":{
   "post":{"operationId":"CreateSynthetic","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string","minLength":1}},{"name":"filter","in":"query","schema":{"type":"string"}},{"name":"If-Match","in":"header","required":true,"schema":{"type":"string"}}],"requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Payload"}}}},"responses":{"200":{"description":"Synthetic receipt","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Payload"}}}},"201":{"description":"Same synthetic receipt","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Payload"}}}},"422":{"description":"Synthetic validation","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Problem"}}}}}},
   "get":{"operationId":"ReadSynthetic","security":[{"SessionCookie":[]}],"parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"Synthetic union","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Choice"}}}}}},
   "delete":{"operationId":"DeleteSynthetic","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"responses":{"204":{"description":"Synthetic empty"}}}
  }
 }
}`

type generationHarness struct {
	t               *testing.T
	root, dir, node string
	main            string
}

func newGenerationHarness(t *testing.T) generationHarness {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "../.."))
	common, err := exec.Command("git", "-C", root, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		t.Fatal(err)
	}
	main := filepath.Dir(strings.TrimSpace(string(common)))
	node := filepath.Join(main, "docs/justix-auto/dev/local/toolchains/node-24.21.0/bin/node")
	if _, err := os.Stat(node); err != nil {
		node, err = exec.LookPath("node")
		if err != nil {
			t.Fatal("install the approved Node 24.21.0 toolchain before contract checks")
		}
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := generationHarness{t: t, root: root, main: main, dir: dir, node: node}
	if version := h.run(true, node, "--version"); strings.TrimSpace(version) != "v24.21.0" {
		t.Fatalf("approved Node version required, got %s", version)
	}
	h.write("input.json", generationFixture)
	h.config(false)
	return h
}

func (h generationHarness) write(path, content string) {
	h.t.Helper()
	full := filepath.Join(h.dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0600); err != nil {
		h.t.Fatal(err)
	}
}

func (h generationHarness) config(two bool) {
	h.t.Helper()
	entries := []map[string]string{{"input": "input.json", "goOutput": "fixture.gen.go", "tsOutput": "fixture.ts", "goPackage": "synthetic", "owner": "identity", "namespace": "Fixture"}}
	if two {
		entries = append(entries, map[string]string{"input": "input.json", "goOutput": "second.gen.go", "tsOutput": "second.ts", "goPackage": "synthetic", "owner": "identity", "namespace": "Second"})
	}
	data, err := json.Marshal(map[string]any{"version": 1, "contracts": entries})
	if err != nil {
		h.t.Fatal(err)
	}
	h.write("config.json", string(data))
}

func (h generationHarness) run(pass bool, program string, args ...string) string {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, program, args...)
	command.Dir = h.dir
	command.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOPROXY=off", "GOFLAGS=-mod=readonly")
	output, err := command.CombinedOutput()
	if (err == nil) != pass {
		h.t.Fatalf("%s %v: %v\n%s", program, args, err, output)
	}
	return string(output)
}

func (h generationHarness) generate(pass bool, extra ...string) string {
	h.t.Helper()
	args := []string{filepath.Join(h.root, "tools/generate-contracts.mjs"), "--config", filepath.Join(h.dir, "config.json")}
	return h.run(pass, h.node, append(args, extra...)...)
}

func (h generationHarness) read(path string) []byte {
	h.t.Helper()
	value, err := os.ReadFile(filepath.Join(h.dir, path))
	if err != nil {
		h.t.Fatal(err)
	}
	return value
}

func TestContractGenerationReproducibility(t *testing.T) {
	h := newGenerationHarness(t)
	h.config(true)
	h.generate(false, "--check")
	if _, err := os.Stat(filepath.Join(h.dir, "fixture.ts")); !os.IsNotExist(err) {
		t.Fatal("check mode wrote a missing output")
	}
	h.generate(true)
	goFirst, tsFirst := h.read("fixture.gen.go"), h.read("fixture.ts")
	if bytes.Contains(tsFirst, []byte("/internal/v1/")) || bytes.Contains(tsFirst, []byte("function ReadInternal(")) {
		t.Fatal("internal owner route became browser callable")
	}
	h.generate(true, "--check")
	// Different formatting/property order cannot change generated bytes or hash.
	var document any
	if err := json.Unmarshal([]byte(generationFixture), &document); err != nil {
		t.Fatal(err)
	}
	compact, _ := json.Marshal(document)
	h.write("input.json", string(compact))
	h.generate(true)
	if !bytes.Equal(goFirst, h.read("fixture.gen.go")) || !bytes.Equal(tsFirst, h.read("fixture.ts")) {
		t.Fatal("canonical input ordering was not reproducible")
	}
	h.write("fixture.ts", string(tsFirst)+"\n// explicit drift\n")
	h.generate(false, "--check")
	if !bytes.Contains(h.read("fixture.ts"), []byte("explicit drift")) {
		t.Fatal("check mode modified drift")
	}
	h.generate(true)
	h.generate(true, "--check")
	h.write("go.mod", "module synthetic\n\ngo 1.27.1\n")
	h.write("fixture_test.go", generatedGoChecks)
	h.run(true, filepath.Join(runtime.GOROOT(), "bin/go"), "test", "-race", "-count=1", "./...")
	h.run(true, filepath.Join(runtime.GOROOT(), "bin/go"), "vet", "./...")
	api, err := os.ReadFile(filepath.Join(h.root, "web/packages/api/src/client.ts"))
	if err != nil {
		t.Fatal(err)
	}
	h.write("client.ts", string(api))
	h.write("check.ts", generatedTSChecks)
	h.write("package.json", `{"type":"commonjs"}`)
	h.write("tsconfig.json", `{"compilerOptions":{"target":"ES2022","lib":["ES2022","DOM","DOM.Iterable"],"module":"Node16","moduleResolution":"Node16","strict":true,"noUncheckedIndexedAccess":true,"exactOptionalPropertyTypes":true,"noUnusedLocals":true,"noUnusedParameters":true,"skipLibCheck":false,"types":[],"paths":{"@justixauto/api":["./client.ts"]},"outDir":"out"},"include":["*.ts"]}`)
	h.run(true, h.node, filepath.Join(h.main, "node_modules/typescript/bin/tsc"), "-p", filepath.Join(h.dir, "tsconfig.json"))
	h.run(true, h.node, filepath.Join(h.dir, "out/check.js"))
}

func TestContractGenerationRejectsUnsupportedBeforeWrites(t *testing.T) {
	cases := map[string]func(string) string{
		"general YAML": func(string) string { return "openapi: 3.1.0\n" },
		"Go result symbol collision": func(s string) string {
			return strings.Replace(s, `"Maybe":`, `"CreateSyntheticResult":{"type":"string"},"Maybe":`, 1)
		},
		"TypeScript validator collision": func(s string) string {
			return strings.Replace(s, `"operationId":"ReadSynthetic"`, `"operationId":"PayloadSchema"`, 1)
		},
		"duplicate JSON keys": func(s string) string {
			return strings.Replace(s, `"openapi":"3.1.0"`, `"openapi":"3.1.0","openapi":"3.1.0"`, 1)
		},
		"old nullable": func(s string) string {
			return strings.Replace(s, `"type":["string","null"]`, `"type":"string","nullable":true`, 1)
		},
		"floating number": func(s string) string { return strings.Replace(s, `"type":"integer"`, `"type":"number"`, 1) },
		"unsafe integer":  func(s string) string { return strings.Replace(s, `"maximum":100`, `"maximum":9007199254740992`, 1) },
		"open object": func(s string) string {
			return strings.Replace(s, `"additionalProperties":false`, `"additionalProperties":true`, 1)
		},
		"inherited required": func(s string) string {
			return strings.Replace(s, `"required":["amountMinor"`, `"required":["toString","amountMinor"`, 1)
		},
		"unknown constraint": func(s string) string { return strings.Replace(s, `"maxLength":38`, `"maxLength":38,"default":"1"`, 1) },
		"external ref": func(s string) string {
			return strings.ReplaceAll(s, "#/components/schemas/Payload", "https://invalid.example/schema")
		},
		"missing ref": func(s string) string {
			return strings.ReplaceAll(s, "#/components/schemas/Payload", "#/components/schemas/Missing")
		},
		"recursive ref": func(s string) string {
			return strings.Replace(s, `"note":{"type":["string","null"]}`, `"note":{"$ref":"#/components/schemas/Payload"}`, 1)
		},
		"429 handoff": func(s string) string { return strings.Replace(s, `"422":`, `"429":`, 1) },
		"response header handoff": func(s string) string {
			return strings.Replace(s, `"description":"Synthetic empty"`, `"description":"Synthetic empty","headers":{"X-CSRF-Token":{}}`, 1)
		},
		"discriminator": func(s string) string {
			return strings.Replace(s, `"Choice":{"oneOf"`, `"Choice":{"discriminator":{"propertyName":"kind"},"oneOf"`, 1)
		},
		"unbounded integer":        func(s string) string { return strings.Replace(s, `,"maximum":100`, "", 1) },
		"path mismatch":            func(s string) string { return strings.Replace(s, `/synthetic/{id}`, `/synthetic/{foreign}`, 1) },
		"foreign owner":            func(s string) string { return strings.Replace(s, `/api/v1/identity/`, `/api/v1/commerce/`, 1) },
		"cookie parameter":         func(s string) string { return strings.Replace(s, `"in":"query"`, `"in":"cookie"`, 1) },
		"pattern dialect":          func(s string) string { return strings.Replace(s, `^x$`, `^(?=x)x$`, 1) },
		"RE2 incompatible pattern": func(s string) string { return strings.Replace(s, `^x$`, `[]`, 1) },
		"missing internal mTLS":    func(s string) string { return strings.Replace(s, `"security":[{"OwnerTLS":[]}]`, `"security":[]`, 1) },
		"unknown security": func(s string) string {
			return strings.Replace(s, `"type":"mutualTLS"`, `"type":"http","scheme":"bearer"`, 1)
		},
		"allOf": func(s string) string {
			return strings.Replace(s, `"Maybe":{"type":["string","null"]}`, `"Maybe":{"allOf":[{"type":"string"}]}`, 1)
		},
		"mixed map": func(s string) string {
			return strings.Replace(s, `"metadata":{"type":"object","additionalProperties"`, `"metadata":{"type":"object","properties":{},"additionalProperties"`, 1)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			h := newGenerationHarness(t)
			h.generate(true)
			before := h.read("fixture.gen.go")
			h.write("input.json", mutate(generationFixture))
			h.generate(false)
			if !bytes.Equal(before, h.read("fixture.gen.go")) {
				t.Fatal("invalid input partially wrote outputs")
			}
		})
	}
}

func TestContractGenerationInternalOnly(t *testing.T) {
	h := newGenerationHarness(t)
	var document map[string]any
	if err := json.Unmarshal([]byte(generationFixture), &document); err != nil {
		t.Fatal(err)
	}
	paths := document["paths"].(map[string]any)
	delete(paths, "/api/v1/identity/synthetic/{id}")
	encoded, _ := json.Marshal(document)
	h.write("input.json", string(encoded))
	h.generate(true)
	h.write("go.mod", "module synthetic\n\ngo 1.27.1\n")
	h.run(true, filepath.Join(runtime.GOROOT(), "bin/go"), "test", "-race", "./...")
	api, err := os.ReadFile(filepath.Join(h.root, "web/packages/api/src/client.ts"))
	if err != nil {
		t.Fatal(err)
	}
	h.write("client.ts", string(api))
	h.write("tsconfig.json", `{"compilerOptions":{"target":"ES2022","lib":["ES2022","DOM","DOM.Iterable"],"module":"Node16","moduleResolution":"Node16","strict":true,"noUnusedLocals":true,"noUnusedParameters":true,"types":[],"paths":{"@justixauto/api":["./client.ts"]},"noEmit":true},"include":["*.ts"]}`)
	h.run(true, h.node, filepath.Join(h.main, "node_modules/typescript/bin/tsc"), "-p", filepath.Join(h.dir, "tsconfig.json"))
}

func TestContractGenerationSymbolCollisionsBeforeWrites(t *testing.T) {
	h := newGenerationHarness(t)
	h.generate(true)
	goBefore, tsBefore := h.read("fixture.gen.go"), h.read("fixture.ts")
	// Imports, emitted helpers/constructor, generated declarations, and every
	// referenced capitalized TS global must not be shadowed by accepted DTOs.
	for _, name := range []string{"ApiRequest", "ApiResult", "NewContractClient", "ContractTransport", "ContractOptional", "ContractClient", "ContractExchange", "MarshalJSON", "UnmarshalJSON", "IsZero", "Marshal", "Unmarshal", "Context", "PayloadSchema", "ReadSyntheticSecurity", "ReadSyntheticParameters", "CreateSyntheticResult", "Record", "Readonly", "ReadonlyArray", "Object", "Array", "JSON", "Error", "RegExp", "Number", "Date", "Reflect", "Set", "Promise", "AbortSignal", "URLSearchParams"} {
		t.Run("schema_"+name, func(t *testing.T) {
			h.write("input.json", strings.Replace(generationFixture, `"Maybe":`, `"`+name+`":{"type":"string"},"Maybe":`, 1))
			h.generate(false)
			if !bytes.Equal(goBefore, h.read("fixture.gen.go")) || !bytes.Equal(tsBefore, h.read("fixture.ts")) {
				t.Fatal("symbol rejection changed an output")
			}
		})
	}
	for _, name := range []string{"ApiRequest", "ApiResult", "ContractTransport", "MarshalJSON", "UnmarshalJSON", "IsZero", "Marshal", "Unmarshal", "Context", "Record", "Readonly", "ReadonlyArray", "Object", "Array", "JSON", "Error", "RegExp", "Number", "Date", "Reflect", "Set", "Promise", "AbortSignal", "URLSearchParams", "PayloadSchema", "CreateSyntheticSecurity"} {
		t.Run("operation_"+name, func(t *testing.T) {
			h.write("input.json", strings.Replace(generationFixture, `"operationId":"ReadSynthetic"`, `"operationId":"`+name+`"`, 1))
			h.generate(false)
			if !bytes.Equal(goBefore, h.read("fixture.gen.go")) || !bytes.Equal(tsBefore, h.read("fixture.ts")) {
				t.Fatal("symbol rejection changed an output")
			}
		})
	}
	h.write("input.json", strings.Replace(generationFixture, `"note":{"type":["string","null"]}`, `"note":{"type":["string","null"]},"payload":{"type":"string"},"fixturePayload":{"type":"string"}`, 1))
	h.generate(false)
	if !bytes.Equal(goBefore, h.read("fixture.gen.go")) || !bytes.Equal(tsBefore, h.read("fixture.ts")) {
		t.Fatal("namespaced field collision changed an output")
	}
	// Distinct namespace strings can still produce identical final Go symbols.
	h.config(true)
	h.write("config.json", strings.Replace(string(h.read("config.json")), `"Second"`, `"FixtureExtra"`, 1))
	h.write("input.json", strings.Replace(generationFixture, `"Maybe":`, `"ExtraPayload":{"type":"string"},"Maybe":`, 1))
	h.generate(false)
	if !bytes.Equal(goBefore, h.read("fixture.gen.go")) || !bytes.Equal(tsBefore, h.read("fixture.ts")) {
		t.Fatal("cross-file symbol rejection changed an output")
	}
	for _, path := range []string{"second.gen.go", "second.ts"} {
		if _, err := os.Lstat(filepath.Join(h.dir, path)); !os.IsNotExist(err) {
			t.Fatal("cross-file symbol rejection created an output")
		}
	}
}

func TestContractGenerationGoMethodContractsBeforeWrites(t *testing.T) {
	h := newGenerationHarness(t)
	h.generate(true)
	goBefore, tsBefore := h.read("fixture.gen.go"), h.read("fixture.ts")
	// Go-only operation Error bypassed the public TS reserved-name check and
	// removed ContractFailure's error interface. Cover every runtime/DTO hook
	// and the external receiver methods used by the decoder, numbers, patterns
	// and query builder; those selectors must survive namespacing as well.
	names := []string{"Error", "MarshalJSON", "UnmarshalJSON", "IsZero", "Exchange", "Token", "UseNumber", "More", "SetString", "IsInt", "Num", "IsInt64", "Int64", "String", "MatchString", "Set", "Encode",
		// Stable fields in the runtime transport/security/failure/optional types
		// and result wrappers must not be renamed consistently into a broken API.
		"Owner", "OperationID", "Method", "Path", "Headers", "Body", "Security", "Status", "ContentType", "Kind", "UnknownOutcome", "Type", "In", "Name", "Value", "Present", "Status200", "Status201", "Status422"}
	for _, kind := range []string{"schema", "public-operation", "internal-operation"} {
		for _, name := range names {
			t.Run(kind+"_"+name, func(t *testing.T) {
				h := h
				h.t = t
				input := generationFixture
				if kind == "schema" {
					input = strings.Replace(input, `"Maybe":`, `"`+name+`":{"type":"string"},"Maybe":`, 1)
				} else {
					original := "ReadSynthetic"
					if kind == "internal-operation" {
						original = "ReadInternal"
					}
					input = strings.Replace(input, `"operationId":"`+original+`"`, `"operationId":"`+name+`"`, 1)
				}
				h.write("input.json", input)
				h.generate(false)
				if !bytes.Equal(goBefore, h.read("fixture.gen.go")) || !bytes.Equal(tsBefore, h.read("fixture.ts")) {
					t.Fatal("Go method collision changed an output")
				}
			})
		}
	}
}

func TestContractGenerationSelectorLookingWireText(t *testing.T) {
	h := newGenerationHarness(t)
	input := strings.Replace(generationFixture, `"operationId":"ReadInternal"`, `"operationId":"ReadReceipt"`, 1)
	// Neither ordinary receiver-looking text nor imported-package-looking text
	// in schema literals is Go code. Both must survive generation and decoding.
	input = strings.Replace(input, `"Maybe":`, `"WireText":{"type":"string","enum":["x.ReadReceipt(","strings.ReadReceipt(","/* x.ReadReceipt( */","'x.ReadReceipt('","`+"`x.ReadReceipt(`"+`"]},"Maybe":`, 1)
	h.write("input.json", input)
	h.generate(true)
	if bytes.Contains(h.read("fixture.ts"), []byte("/internal/v1/")) || bytes.Contains(h.read("fixture.ts"), []byte("function ReadReceipt(")) {
		t.Fatal("internal method exposed to browser")
	}
	h.write("go.mod", "module synthetic\n\ngo 1.27.1\n")
	h.write("wire_test.go", `package synthetic
import("context";"encoding/json";"testing")
type wireExchange struct{}
func(wireExchange) Exchange(_ context.Context,r FixtureContractRequest)(FixtureContractResponse,error){
 _=r.Owner;_=r.OperationID;_=r.Method;_=r.Path;_=r.Headers;_=r.Body;_=r.Security
 return FixtureContractResponse{Status:200,ContentType:"application/json",Body:nil},FixtureContractFailure{Kind:"synthetic failure",UnknownOutcome:true}
}
var _ FixtureContractExchange=wireExchange{}
var _ error=FixtureContractFailure{}
func TestWireText(t *testing.T){
 if _,err:=FixtureNewContractClient(wireExchange{});err!=nil{t.Fatal(err)}
 for _,value:=range []string{"x.ReadReceipt(","strings.ReadReceipt(","/* x.ReadReceipt( */","'x.ReadReceipt('","`+"`x.ReadReceipt(`"+`"}{
  raw,_:=json.Marshal(value);var decoded FixtureWireText
  if err:=json.Unmarshal(raw,&decoded);err!=nil{t.Fatal(err)}
  after,err:=json.Marshal(decoded);if err!=nil||string(after)!=string(raw){t.Fatal("wire text changed",err)}
 }
}
`)
	h.run(true, filepath.Join(runtime.GOROOT(), "bin/go"), "test", "-race", "./...")
	h.run(true, filepath.Join(runtime.GOROOT(), "bin/go"), "vet", "./...")
	api, err := os.ReadFile(filepath.Join(h.root, "web/packages/api/src/client.ts"))
	if err != nil {
		t.Fatal(err)
	}
	h.write("client.ts", string(api))
	h.write("tsconfig.json", `{"compilerOptions":{"target":"ES2022","lib":["ES2022","DOM","DOM.Iterable"],"module":"Node16","moduleResolution":"Node16","strict":true,"types":[],"paths":{"@justixauto/api":["./client.ts"]},"outDir":"out"},"include":["*.ts"]}`)
	h.write("wire.ts", "import { WireTextSchema } from './fixture.js';\nfor (const value of ['x.ReadReceipt(', 'strings.ReadReceipt(', '/* x.ReadReceipt( */', \"'x.ReadReceipt('\", '`x.ReadReceipt(`']) { if (WireTextSchema.parse(value) !== value || WireTextSchema.parse(JSON.parse(JSON.stringify(value))) !== value) throw new Error('wire text changed'); }\n")
	h.run(true, h.node, filepath.Join(h.main, "node_modules/typescript/bin/tsc"), "-p", filepath.Join(h.dir, "tsconfig.json"))
	h.run(true, h.node, filepath.Join(h.dir, "out/wire.js"))
}

func TestContractGenerationDanglingLinksBeforeWrites(t *testing.T) {
	for _, path := range []string{"second.ts", "second.gen.go", "missing-parent"} {
		t.Run(path, func(t *testing.T) {
			h := newGenerationHarness(t)
			h.config(true)
			h.generate(true)
			// Force an earlier output to need rewriting; a later invalid output must
			// abort the entire plan before any write, in check and generation modes.
			h.write("fixture.gen.go", string(h.read("fixture.gen.go"))+"\n// preserved drift\n")
			before := map[string][]byte{}
			for _, file := range []string{"fixture.gen.go", "fixture.ts", "second.gen.go", "second.ts"} {
				before[file] = h.read(file)
			}
			if path == "missing-parent" {
				h.write("config.json", strings.Replace(string(h.read("config.json")), `"second.ts"`, `"missing-parent/second.ts"`, 1))
			} else if err := os.Remove(filepath.Join(h.dir, path)); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(filepath.Dir(h.dir), filepath.Base(h.dir)+"-absent-target")
			link := filepath.Join(h.dir, path)
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			for _, check := range []bool{true, false} {
				if check {
					h.generate(false, "--check")
				} else {
					h.generate(false)
				}
				if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("link changed: %v", err)
				}
				if got, err := os.Readlink(link); err != nil || got != target {
					t.Fatalf("link target changed: %s %v", got, err)
				}
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatalf("dangling target created: %v", err)
				}
				for file, content := range before {
					if file != path && !bytes.Equal(content, h.read(file)) {
						t.Fatalf("other output %s changed", file)
					}
				}
			}
		})
	}
}

func TestContractGenerationOutputGuards(t *testing.T) {
	for _, which := range []string{"empty", "namespace", "duplicate output", "input overwrite", "user file", "symlink", "malformed UTF8"} {
		t.Run(which, func(t *testing.T) {
			h := newGenerationHarness(t)
			switch which {
			case "empty":
				h.write("config.json", `{"version":1,"contracts":[]}`)
			case "malformed UTF8":
				h.write("input.json", strings.Replace(generationFixture, "Synthetic generation fixture", string([]byte{0xff}), 1))
			case "namespace":
				h.config(true)
				h.write("config.json", strings.Replace(string(h.read("config.json")), `"Second"`, `"Fixture"`, 1))
			case "duplicate output":
				h.config(true)
				h.write("config.json", strings.Replace(string(h.read("config.json")), `"second.ts"`, `"fixture.ts"`, 1))
			case "input overwrite":
				h.write("input.ts", generationFixture)
				h.write("config.json", strings.ReplaceAll(strings.ReplaceAll(string(h.read("config.json")), "input.json", "input.ts"), "fixture.ts", "input.ts"))
			case "user file":
				h.write("fixture.gen.go", "package synthetic // user owned\n")
			case "symlink":
				h.write("target.ts", "// user target\n")
				if err := os.Symlink(filepath.Join(h.dir, "target.ts"), filepath.Join(h.dir, "fixture.ts")); err != nil {
					t.Fatal(err)
				}
			}
			h.generate(false)
			if _, err := os.Stat(filepath.Join(h.dir, "second.gen.go")); !os.IsNotExist(err) {
				t.Fatal("failed plan wrote an output")
			}
		})
	}
}

const generatedGoChecks = `package synthetic
import("context";"encoding/json";"errors";"strings";"testing")
const validPayload = "{\"amountMinor\":\"99999999999999999999999999999999999999\",\"revision\":\"9223372036854775807\",\"nullable\":null,\"tags\":[],\"metadata\":{},\"count\":1}"
func TestMapKeyUnicode(t *testing.T){for _,tt:=range []struct{key string;valid bool}{{"\\ud800",false},{"\\udfff",false},{"prefix\\ud800suffix",false},{"\\ud83d\\ude00",true},{"😀",true},{"\\ufffd",true}}{raw:=strings.Replace(validPayload,"\"metadata\":{}","\"metadata\":{\""+tt.key+"\":\"x\"}",1);var value FixturePayload;err:=json.Unmarshal([]byte(raw),&value);if (err==nil)!=tt.valid{t.Fatalf("key %q accepted=%v",tt.key,err==nil)};if tt.valid{var key string;if err:=json.Unmarshal([]byte("\""+tt.key+"\""),&key);err!=nil{t.Fatal(err)};if len(value.Metadata)!=1||value.Metadata[key]!="x"{t.Fatal("map key normalized/dropped")};encoded,err:=json.Marshal(value);if err!=nil{t.Fatal(err)};var again FixturePayload;if err=json.Unmarshal(encoded,&again);err!=nil||again.Metadata[key]!="x"{t.Fatal("map key round trip changed")}}}}
func TestRoundTrip(t *testing.T){var value FixturePayload;if err:=json.Unmarshal([]byte(validPayload),&value);err!=nil{t.Fatal(err)};if value.Note.Present||value.Nullable!=nil||value.AmountMinor!="99999999999999999999999999999999999999"{t.Fatal("precision/presence")};raw,err:=json.Marshal(value);if err!=nil||strings.Contains(string(raw),"note"){t.Fatalf("%s %v",raw,err)};value.Note=FixtureContractOptional[*string]{Present:true};raw,err=json.Marshal(value);if err!=nil||!strings.Contains(string(raw),"\"note\":null"){t.Fatalf("%s %v",raw,err)};var again FixturePayload;if err=json.Unmarshal(raw,&again);err!=nil||!again.Note.Present||again.Note.Value!=nil{t.Fatal(err)};value.Count=101;if _,err=json.Marshal(value);err==nil{t.Fatal("invalid constructed DTO serialized")}
 for _,bad:=range []string{strings.Replace(validPayload,"\"nullable\":null,","",1),strings.Replace(validPayload,"\"count\":1","\"count\":1,\"unknown\":true",1),strings.Replace(validPayload,"\"count\":1","\"count\":null",1),strings.Replace(validPayload,"\"count\":1","\"count\":1,\"count\":2",1),strings.Replace(validPayload,"\"tags\":[]","\"tags\":[\"x\",\"x\"]",1),strings.Replace(validPayload,"\"metadata\":{}","\"metadata\":{\"x\":1}",1),strings.Replace(validPayload,"\"nullable\":null","\"nullable\":\"\\ud800\"",1),validPayload+" {}"}{var next FixturePayload;if json.Unmarshal([]byte(bad),&next)==nil{t.Fatalf("accepted %s",bad)}}
 for _,number:=range []string{"1.0","1e0"}{var next FixturePayload;if err:=json.Unmarshal([]byte(strings.Replace(validPayload,"\"count\":1","\"count\":"+number,1)),&next);err!=nil||next.Count!=1{t.Fatal(err)}}
 var maybe FixtureMaybe;if err:=json.Unmarshal([]byte("null"),&maybe);err!=nil||maybe.Value!=nil{t.Fatal(err)}
 for _,suffix:=range []string{"\n","\r\n","\u2028","\u2029"}{var value FixturePayload;_ = json.Unmarshal([]byte(validPayload),&value);value.AmountMinor="1"+suffix;if _,err:=json.Marshal(value);err==nil{t.Fatal("noncanonical amount")};value.AmountMinor="1";value.Revision="1"+suffix;if _,err:=json.Marshal(value);err==nil{t.Fatal("noncanonical revision")};raw,_:=json.Marshal("x"+suffix);var loose FixtureLooseEnd;if json.Unmarshal(raw,&loose)==nil{t.Fatal("ordinary dollar accepted final line terminator")}}
}
func TestUnionsAndDates(t *testing.T){var v FixtureChoice;pending:=[]byte("{\"kind\":\"pending\",\"operationId\":\"11111111-1111-4111-8111-111111111111\"}");if err:=json.Unmarshal(pending,&v);err!=nil||v.Variant2==nil||v.Variant1!=nil{t.Fatal(err)};if _,err:=json.Marshal(v);err!=nil{t.Fatal(err)};v.Variant1=&FixtureReady{};if _,err:=json.Marshal(v);err==nil{t.Fatal("two alternatives accepted")};for _,raw:=range []string{"{\"kind\":\"unknown\"}","null"}{if json.Unmarshal([]byte(raw),&v)==nil{t.Fatal("union accepted invalid")}}
 for _,date:=range []string{"2026-02-30","2026-13-01"}{var d FixtureDates;if json.Unmarshal([]byte("{\"date\":\""+date+"\",\"instant\":\"2026-09-15T12:00:00Z\"}"),&d)==nil{t.Fatal("bad date")}}
}
type exchangeFunc func(context.Context,FixtureContractRequest)(FixtureContractResponse,error)
func(f exchangeFunc)Exchange(ctx context.Context,r FixtureContractRequest)(FixtureContractResponse,error){return f(ctx,r)}
func TestInjectedClient(t *testing.T){calls:=0;var body FixturePayload;if err:=json.Unmarshal([]byte(validPayload),&body);err!=nil{t.Fatal(err)};client,err:=FixtureNewContractClient(exchangeFunc(func(_ context.Context,r FixtureContractRequest)(FixtureContractResponse,error){calls++;if r.Path!="/api/v1/identity/synthetic/a?filter=a%26b"||r.Headers["If-Match"]!="\"9223372036854775807\""||r.Method!="POST"{t.Fatalf("bad request %#v",r)};return FixtureContractResponse{Status:201,ContentType:"application/json",Body:[]byte(validPayload)},nil}));if err!=nil{t.Fatal(err)}
 params:=FixtureCreateSyntheticParameters{Id:"a",IfMatch:"9223372036854775807",Filter:FixtureContractOptional[string]{Value:"a&b",Present:true}};result,err:=client.FixtureCreateSynthetic(context.Background(),params,body);if err!=nil||result.Status201==nil||calls!=1{t.Fatalf("%#v %v",result,err)}
 params.Id="..";if _,err:=client.FixtureCreateSynthetic(context.Background(),params,body);err==nil||calls!=1{t.Fatal("unsafe route reached exchange")};params.Id="a";params.IfMatch="9223372036854775808";if _,err:=client.FixtureCreateSynthetic(context.Background(),params,body);err==nil||calls!=1{t.Fatal("unsafe revision")}
 failed,_:=FixtureNewContractClient(exchangeFunc(func(context.Context,FixtureContractRequest)(FixtureContractResponse,error){calls++;return FixtureContractResponse{},errors.New("synthetic lost reply secret excluded")}));params.IfMatch="1";_,err=failed.FixtureCreateSynthetic(context.Background(),params,body);var failure FixtureContractFailure;if !errors.As(err,&failure)||!failure.UnknownOutcome||strings.Contains(err.Error(),"secret")||calls!=2{t.Fatal("lost reply/retry semantics")}
 internal,_:=FixtureNewContractClient(exchangeFunc(func(_ context.Context,r FixtureContractRequest)(FixtureContractResponse,error){if r.Owner!="identity"||r.OperationID!="ReadInternal"||r.Path!="/internal/v1/identity/synthetic/a"||len(r.Security)!=1||r.Security[0]["OwnerTLS"].Type!="mutualTLS"{t.Fatalf("internal declaration lost: %#v",r)};return FixtureContractResponse{Status:200,ContentType:"application/json",Body:[]byte(validPayload)},nil}));if result,err:=internal.FixtureReadInternal(context.Background(),FixtureReadInternalParameters{Id:"a"});err!=nil||result.Status200==nil{t.Fatal(err)}
}
`

const generatedTSChecks = `import {PayloadSchema,ChoiceSchema,DatesSchema,MaybeSchema,LooseEndSchema,CreateSynthetic,ReadSynthetic,ReadSyntheticSecurity,DeleteSynthetic} from './fixture.js';
import type {Payload} from './fixture.js';
import {createApiClient} from './client.js';
function assert(value: unknown, message: string): asserts value { if (!value) throw new Error(message); }
function rejects(run: () => unknown): void { let rejected = false; try { run(); } catch { rejected = true; } assert(rejected,'invalid fixture accepted'); }
const payload: Payload = {amountMinor:'99999999999999999999999999999999999999',revision:'9223372036854775807',nullable:null,count:1,tags:[],metadata:{}};
assert(PayloadSchema.parse(payload).amountMinor===payload.amountMinor,'precision changed');
assert(!Object.hasOwn(PayloadSchema.parse(payload),'note'),'omitted changed');
assert(PayloadSchema.parse({...payload,note:null}).note===null,'explicit null changed');
for (const bad of [{...payload,unknown:true},{...payload,note:undefined},{...payload,nullable:undefined},{...payload,count:null},{...payload,count:1.2},{...payload,amountMinor:1},{...payload,tags:['x','x']},{...payload,metadata:{x:1}},{...payload,note:'\ud800'}]) rejects(()=>PayloadSchema.parse(bad));
for (const key of ['\ud800','\udfff','prefix\ud800suffix']) rejects(()=>PayloadSchema.parse({...payload,metadata:{[key]:'x'}}));
for (const key of ['\ud83d\ude00','😀','\ufffd']) { const value=PayloadSchema.parse({...payload,metadata:{[key]:'x'}}); assert(Object.keys(value.metadata).length===1&&value.metadata[key]==='x','map key normalized/dropped'); assert(PayloadSchema.parse(JSON.parse(JSON.stringify(value))).metadata[key]==='x','map key round trip changed'); }
const {nullable: removed,...missing} = payload; void removed; rejects(()=>PayloadSchema.parse(missing));
const inherited = Object.create({nullable:null}) as Record<string,unknown>; Object.assign(inherited,payload); rejects(()=>PayloadSchema.parse(inherited));
const sparse: unknown[] = []; sparse.length=1; rejects(()=>PayloadSchema.parse({...payload,tags:sparse}));
assert(MaybeSchema.parse(null)===null,'nullable root');
assert(ReadSyntheticSecurity[0].SessionCookie.name==='__Host-justix_session','public cookie declaration lost');
for(const suffix of ['\n','\r\n','\u2028','\u2029']) { rejects(()=>PayloadSchema.parse({...payload,amountMinor:'1'+suffix})); rejects(()=>PayloadSchema.parse({...payload,revision:'1'+suffix})); rejects(()=>LooseEndSchema.parse('x'+suffix)); }
assert(ChoiceSchema.parse({kind:'pending',operationId:'11111111-1111-4111-8111-111111111111'}).kind==='pending','union');
rejects(()=>ChoiceSchema.parse({kind:'unknown'}));
for(const date of ['2026-02-30','2026-13-01']) rejects(()=>DatesSchema.parse({date,instant:'2026-09-15T12:00:00Z'}));
// @ts-expect-error explicit undefined is not an optional nullable string
const wrong: Payload = {...payload,note:undefined}; void wrong;
async function main(): Promise<void> {
 let calls=0; let response: Response = new Response(JSON.stringify(payload),{status:201,headers:{'Content-Type':'application/json'}});
 const client=createApiClient({origin:'https://synthetic.invalid',fetch:async(input,init)=>{calls++;assert(String(input)==='https://synthetic.invalid/api/v1/identity/synthetic/a?filter=a%26b','route encoding');assert(new Headers(init?.headers).get('If-Match')==='"9223372036854775807"','revision precision');assert(init?.credentials==='same-origin'&&init.redirect==='error'&&init.cache==='no-store','shared transport');return response;}});
 const params={id:'a',filter:'a&b','If-Match':'9223372036854775807'};
 const ok=await CreateSynthetic(client,params,payload);assert(ok.kind==='success'&&ok.status===201&&calls===1,'typed success');
 const invalid=await CreateSynthetic(client,{...params,id:'..'},payload);assert(invalid.kind==='invalid-request'&&calls===1,'unsafe path performed IO');
 for(const revision of ['1\n','1\r\n','1\u2028','9223372036854775808']) { const badHeader=await CreateSynthetic(client,{...params,'If-Match':revision},payload);assert(badHeader.kind==='invalid-request'&&calls===1,'bad revision header performed IO'); }
 response=new Response(JSON.stringify({error:{code:'INVALID',message:'Synthetic',fields:{x:123},traceId:'synthetic'}}),{status:422,headers:{'Content-Type':'application/json'}});
 const bad=await CreateSynthetic(client,params,payload);assert(bad.kind==='invalid-response','feature error validator');
 const failed=createApiClient({origin:'https://synthetic.invalid',fetch:async()=>{throw new Error('synthetic lost reply');}});
 const lost=await CreateSynthetic(failed,params,payload);assert(lost.kind==='transport-error'&&lost.outcome==='unknown','unknown write');
 const unionClient=createApiClient({origin:'https://synthetic.invalid',fetch:async()=>new Response(JSON.stringify({kind:'pending',operationId:'11111111-1111-4111-8111-111111111111'}),{headers:{'Content-Type':'application/json'}})});
 const union=await ReadSynthetic(unionClient,{id:'a'});assert(union.kind==='success'&&union.data.kind==='pending','union response');
 const emptyClient=createApiClient({origin:'https://synthetic.invalid',fetch:async()=>new Response(null,{status:204})});
 const empty=await DeleteSynthetic(emptyClient,{id:'a'});assert(empty.kind==='success'&&empty.data===undefined,'204');
}
void main();
`
