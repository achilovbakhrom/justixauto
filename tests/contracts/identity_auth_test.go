package contracts_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"justixauto/pkg/events"
)

const (
	authOpenAPIPath  = "services/identity/contracts/openapi/auth.yaml"
	authFixturesPath = "services/identity/contracts/openapi/auth.fixtures.json"
	authEventPath    = "services/identity/contracts/events/auth.schema.json"

	testUserID = "10000000-0000-4000-8000-000000000001"
)

type authFixtureFile struct {
	Version                int               `json:"version"`
	Profile                string            `json:"profile"`
	Description            string            `json:"description"`
	SchemaPositive         []authFixtureCase `json:"schemaPositive"`
	StructuralNegative     []authFixtureCase `json:"structuralNegative"`
	SemanticNegative       []authFixtureCase `json:"semanticNegative"`
	StateTransportNegative []authFixtureCase `json:"stateTransportNegative"`
}

type authFixtureCase struct {
	ID       string         `json:"id"`
	Schema   string         `json:"schema"`
	Input    any            `json:"input"`
	Bindings map[string]any `json:"bindings"`
	Expected map[string]any `json:"expected"`
}

type authSecurityChanged struct {
	UserID           string `json:"userId"`
	SecurityRevision string `json:"securityRevision"`
	Change           string `json:"change"`
}

type replayedAuthSecurity struct {
	UserID             string
	SecurityRevision   events.Revision
	AggregateRevision  events.Revision
	LastChange         string
	TransitionCount    int
	CanAuthenticate    bool
	CredentialMaterial string
}

func TestIdentityAuthOpenAPIProfileAndGeneratorBarrier(t *testing.T) {
	root := authRepositoryRoot(t)
	document := readJSONObject(t, filepath.Join(root, authOpenAPIPath))

	if document["openapi"] != "3.1.0" || integer(document["x-justix-auth-semantics"]) != 1 {
		t.Fatalf("auth document lacks the adopted OpenAPI/semantic profile: %#v", document)
	}
	components := objectAt(t, document, "components")
	schemas := objectAt(t, components, "schemas")
	paths := objectAt(t, document, "paths")
	if len(paths) != 10 {
		t.Fatalf("auth path count = %d, want 10", len(paths))
	}

	csrfSchema := map[string]any{"type": "string", "pattern": "^[A-Za-z0-9_-]{1,4096}$"}
	cacheSchema := map[string]any{"type": "string", "const": "no-store"}
	retrySchema := map[string]any{"type": "integer", "minimum": json.Number("0"), "maximum": json.Number("2147483647")}
	rotation := map[string]bool{
		"GET /api/v1/identity/session":                              true,
		"POST /api/v1/identity/session/login":                       true,
		"POST /api/v1/identity/session/mfa/verify":                  true,
		"POST /api/v1/identity/session/revoke-all":                  true,
		"POST /api/v1/identity/session/mfa/enrollment/{id}/confirm": true,
	}
	operationIDs := map[string]struct{}{}
	for path, rawPath := range paths {
		pathItem := asObject(t, rawPath, path)
		if len(pathItem) != 1 {
			t.Fatalf("%s has %d methods", path, len(pathItem))
		}
		for method, rawOperation := range pathItem {
			operation := asObject(t, rawOperation, method+" "+path)
			if integer(operation["x-justix-auth-transport"]) != 1 {
				t.Fatalf("%s %s lacks transport marker", method, path)
			}
			operationID, ok := operation["operationId"].(string)
			if !ok || operationID == "" {
				t.Fatalf("%s %s lacks operationId", method, path)
			}
			if _, duplicate := operationIDs[operationID]; duplicate {
				t.Fatalf("duplicate operationId %s", operationID)
			}
			operationIDs[operationID] = struct{}{}

			parameters := arrayAt(t, operation, "parameters")
			csrfParameters := 0
			for _, rawParameter := range parameters {
				parameter := asObject(t, rawParameter, "parameter")
				name, _ := parameter["name"].(string)
				if name == "If-Match" || name == "Idempotency-Key" {
					t.Fatalf("auth handshake %s %s accepts %s", method, path, name)
				}
				if name == "X-CSRF-Token" {
					csrfParameters++
					if method == "get" || parameter["in"] != "header" || parameter["required"] != true || !reflect.DeepEqual(parameter["schema"], csrfSchema) {
						t.Fatalf("invalid request CSRF declaration on %s %s: %#v", method, path, parameter)
					}
				}
			}
			if method == "get" && csrfParameters != 0 || method != "get" && csrfParameters != 1 {
				t.Fatalf("%s %s request CSRF declarations = %d", method, path, csrfParameters)
			}

			responses := objectAt(t, operation, "responses")
			key := strings.ToUpper(method) + " " + path
			for status, rawResponse := range responses {
				response := asObject(t, rawResponse, key+" "+status)
				headers := objectAt(t, response, "headers")
				cache := objectAt(t, asObject(t, headers["Cache-Control"], "Cache-Control"), "schema")
				if !reflect.DeepEqual(cache, cacheSchema) {
					t.Fatalf("%s %s has invalid Cache-Control schema: %#v", key, status, cache)
				}
				_, hasCSRF := headers["X-CSRF-Token"]
				wantCSRF := status == successStatus(path, method) && rotation[key] || method == "get" && path == "/api/v1/identity/session" && status == "401"
				if hasCSRF != wantCSRF {
					t.Fatalf("%s %s CSRF response header = %v, want %v", key, status, hasCSRF, wantCSRF)
				}
				if hasCSRF {
					actual := objectAt(t, asObject(t, headers["X-CSRF-Token"], "X-CSRF-Token"), "schema")
					if !reflect.DeepEqual(actual, csrfSchema) {
						t.Fatalf("%s %s has invalid CSRF response schema", key, status)
					}
				}
				_, hasRetry := headers["Retry-After"]
				if hasRetry != (status == "429") {
					t.Fatalf("%s %s Retry-After = %v", key, status, hasRetry)
				}
				if hasRetry {
					actual := objectAt(t, asObject(t, headers["Retry-After"], "Retry-After"), "schema")
					if !reflect.DeepEqual(actual, retrySchema) {
						t.Fatalf("%s has invalid Retry-After schema: %#v", key, actual)
					}
				}
				if status == "204" {
					if _, hasContent := response["content"]; hasContent {
						t.Fatalf("%s 204 declares a body", key)
					}
				} else {
					media := objectAt(t, response, "content")
					if len(media) != 1 || media["application/json"] == nil {
						t.Fatalf("%s %s has noncanonical JSON media", key, status)
					}
				}
			}
		}
	}

	for _, name := range []string{"SessionRead", "SessionResult", "LoginRequest", "VerifyRequest", "EnrollmentSecret", "EnrollmentConfirmed", "RecoveryAccepted", "RecoveryCompleteRequest", "RateLimitedError"} {
		if schemas[name] == nil {
			t.Fatalf("missing required auth schema %s", name)
		}
	}
	assertClosedSchemas(t, schemas)

	// The real generator must completely parse the concrete document, then stop
	// at its approved inert activation barrier. Any earlier failure means this
	// schema drifted outside the reviewed generation profile.
	h := newGenerationHarness(t)
	authBytes, err := os.ReadFile(filepath.Join(root, authOpenAPIPath))
	if err != nil {
		t.Fatal(err)
	}
	h.write("input.json", string(authBytes))
	output := h.generate(false)
	if !strings.Contains(output, "auth generation is not activated") {
		t.Fatalf("auth document did not reach inert generator barrier:\n%s", output)
	}
	for _, path := range []string{"fixture.gen.go", "fixture.ts"} {
		if _, err := os.Stat(filepath.Join(h.dir, path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("inert auth generation wrote %s: %v", path, err)
		}
	}
}

func TestIdentityAuthFixturesStructuralSemanticAndStateSeparation(t *testing.T) {
	root := authRepositoryRoot(t)
	document := readJSONObject(t, filepath.Join(root, authOpenAPIPath))
	eventDocument := readJSONObject(t, filepath.Join(root, authEventPath))
	fixtures := readAuthFixtures(t, filepath.Join(root, authFixturesPath))
	if fixtures.Version != 1 || fixtures.Profile != "fixture-only" {
		t.Fatalf("fixture profile = %d/%q", fixtures.Version, fixtures.Profile)
	}
	if len(fixtures.SchemaPositive) < 10 || len(fixtures.StructuralNegative) < 15 || len(fixtures.SemanticNegative) < 8 || len(fixtures.StateTransportNegative) < 30 {
		t.Fatalf("fixture classes unexpectedly thin: positive=%d structural=%d semantic=%d state=%d", len(fixtures.SchemaPositive), len(fixtures.StructuralNegative), len(fixtures.SemanticNegative), len(fixtures.StateTransportNegative))
	}

	validator := newAuthSchemaValidator(t, document, eventDocument)
	seen := map[string]string{}
	validateIDs := func(class string, cases []authFixtureCase, structuralValid bool) {
		t.Helper()
		for _, fixture := range cases {
			if fixture.ID == "" || fixture.Schema == "" || fixture.Expected == nil {
				t.Fatalf("%s contains incomplete fixture: %#v", class, fixture)
			}
			if previous, duplicate := seen[fixture.ID]; duplicate {
				t.Fatalf("fixture %s occurs in %s and %s", fixture.ID, previous, class)
			}
			seen[fixture.ID] = class
			err := validator.validateNamed(fixture.Schema, fixture.Input)
			if structuralValid && err != nil {
				t.Fatalf("%s %s should be structurally valid: %v", class, fixture.ID, err)
			}
			if !structuralValid && err == nil {
				t.Fatalf("%s %s should be structurally invalid", class, fixture.ID)
			}
		}
	}
	validateIDs("schemaPositive", fixtures.SchemaPositive, true)
	validateIDs("structuralNegative", fixtures.StructuralNegative, false)
	validateIDs("semanticNegative", fixtures.SemanticNegative, true)
	validateIDs("stateTransportNegative", fixtures.StateTransportNegative, true)

	for _, fixture := range fixtures.SchemaPositive {
		if semantic, ok := fixture.Expected["semanticValid"].(bool); ok && semantic {
			if err := validateFixtureSemantics(fixture); err != nil {
				t.Fatalf("positive semantic fixture %s: %v", fixture.ID, err)
			}
		}
	}
	for _, fixture := range fixtures.SemanticNegative {
		if err := validateFixtureSemantics(fixture); err == nil {
			t.Fatalf("semantic-negative fixture %s passed", fixture.ID)
		}
	}

	requiredIDs := []string{
		"auth.login.valid", "auth.login.challenge", "auth.bootstrap.pending", "auth.login.unknown-field",
		"auth.login.null", "auth.login.missing", "auth.login.empty", "auth.login.numeric", "auth.login.no-origin",
		"auth.login.csrf", "auth.verify.valid", "auth.verify.concurrent", "auth.verify.recovery-code",
		"auth.verify.foreign", "auth.verify.replay", "auth.verify.body-actor", "auth.logout.repeat",
		"auth.revoke.reason", "auth.revoke.mfa", "auth.recovery.disabled", "auth.recovery.confirmation",
		"auth.event.valid", "auth.event.secret", "auth.event.numeric-revision", "auth.event.revision-overflow",
		"auth.bootstrap.concurrent", "auth.outcome.login-commit-lost-reply", "auth.outcome.verify-commit-lost-reply",
		"auth.permission.crm-no-blanket-mfa", "auth.reliability.publication-failure", "auth.transport.rate-limited",
	}
	for _, id := range requiredIDs {
		if _, ok := seen[id]; !ok {
			t.Errorf("required fixture %s is missing", id)
		}
	}

	assertExpected(t, seen, fixtures.StateTransportNegative, "auth.verify.concurrent", "successfulConsumptions", json.Number("1"))
	assertExpected(t, seen, fixtures.StateTransportNegative, "auth.bootstrap.concurrent", "pendingBootstrapSubjects", json.Number("1"))
	assertExpected(t, seen, fixtures.StateTransportNegative, "auth.recovery.disabled", "accountLookup", false)
	assertExpected(t, seen, fixtures.StateTransportNegative, "auth.permission.crm-no-blanket-mfa", "inventedMfaRequirement", false)
	assertExpected(t, seen, fixtures.StateTransportNegative, "auth.reliability.publication-failure", "broadcast", false)

	// The validator and loaded documents are immutable after construction. Run
	// the whole fixture corpus concurrently so -race exercises schema reads and
	// catches accidental mutable caches in this contract test boundary.
	all := append([]authFixtureCase{}, fixtures.SchemaPositive...)
	all = append(all, fixtures.SemanticNegative...)
	all = append(all, fixtures.StateTransportNegative...)
	var wg sync.WaitGroup
	errorsFound := make(chan error, len(all)*4)
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, fixture := range all {
				if err := validator.validateNamed(fixture.Schema, fixture.Input); err != nil {
					errorsFound <- fmt.Errorf("%s: %w", fixture.ID, err)
				}
			}
		}()
	}
	wg.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
}

func TestIdentityAuthSafeEventSchemaReplayAndSecretExclusion(t *testing.T) {
	root := authRepositoryRoot(t)
	document := readJSONObject(t, filepath.Join(root, authEventPath))
	if document["title"] != "identity.auth.security-changed.v1" || document["additionalProperties"] != false {
		t.Fatalf("unexpected event schema identity/closure: %#v", document)
	}
	properties := objectAt(t, document, "properties")
	if properties["owner"] != nil {
		t.Fatal("auth event added forbidden owner wire property")
	}
	for key, want := range map[string]any{"eventType": "identity.auth.security-changed.v1", "aggregateType": "auth-security"} {
		property := asObject(t, properties[key], key)
		if property["const"] != want {
			t.Fatalf("%s const = %#v", key, property["const"])
		}
	}
	company := asObject(t, properties["companyId"], "companyId")
	if company["type"] != "null" || company["const"] != nil {
		t.Fatalf("companyId is not fixed global null: %#v", company)
	}

	schema, err := events.NewEventSchema("identity.auth.security-changed.v1", 1, "auth-security", events.CompanyScopeGlobalAllowed, validateAuthSecurityChanged)
	if err != nil {
		t.Fatal(err)
	}
	makeEnvelope := func(aggregateVersion, integrationSequence, securityRevision int64, change string) events.Envelope {
		t.Helper()
		aggregate, _ := events.NewRevision(aggregateVersion)
		sequence, _ := events.NewRevision(integrationSequence)
		envelope, err := events.NewEnvelope(events.EnvelopeInput{
			EventID:             fmt.Sprintf("60000000-0000-4000-8000-%012d", aggregateVersion),
			AggregateID:         testUserID,
			AggregateVersion:    aggregate,
			IntegrationSequence: sequence,
			CompanyID:           nil,
			OccurredAt:          time.Date(2026, 9, 15, 0, 0, int(aggregateVersion), 0, time.UTC),
			Actor:               events.Actor{Kind: events.ActorSystem, ID: "70000000-0000-4000-8000-000000000001"},
			CorrelationID:       "80000000-0000-4000-8000-000000000001",
			CausationID:         fmt.Sprintf("90000000-0000-4000-8000-%012d", aggregateVersion),
			OperationID:         fmt.Sprintf("a0000000-0000-4000-8000-%012d", aggregateVersion),
		}, schema, authSecurityChanged{UserID: testUserID, SecurityRevision: strconv.FormatInt(securityRevision, 10), Change: change})
		if err != nil {
			t.Fatal(err)
		}
		return envelope
	}

	stream := []events.Envelope{
		makeEnvelope(1, 21, 1, "credential-created"),
		makeEnvelope(2, 22, 2, "mfa-enrolled"),
		makeEnvelope(3, 23, 3, "sessions-revoked"),
	}
	state, err := replayAuthSecurity(stream, schema)
	if err != nil {
		t.Fatal(err)
	}
	if state.UserID != testUserID || state.SecurityRevision.String() != "3" || state.AggregateRevision.String() != "3" || state.LastChange != "sessions-revoked" || state.TransitionCount != 3 {
		t.Fatalf("unexpected replay state: %#v", state)
	}
	if state.CanAuthenticate || state.CredentialMaterial != "" {
		t.Fatalf("safe event replay reconstructed authority or credentials: %#v", state)
	}
	if _, err := replayAuthSecurity([]events.Envelope{stream[1], stream[0]}, schema); err == nil {
		t.Fatal("out-of-order aggregate replay succeeded")
	}

	validator := newAuthSchemaValidator(t, readJSONObject(t, filepath.Join(root, authOpenAPIPath)), document)
	for _, envelope := range stream {
		raw, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		decodeJSON(t, raw, &value)
		if err := validator.validateEvent(value); err != nil {
			t.Fatalf("event schema rejected envelope: %v\n%s", err, raw)
		}
		lower := strings.ToLower(string(raw))
		for _, forbidden := range []string{"password", "provisioninguri", "recoverycode", "sessionhandle", "tokendigest", "challengehandle", "csrf", "email", "login"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("event contains forbidden auth material name %q: %s", forbidden, raw)
			}
		}
		var decoded events.Envelope
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		data, err := events.DecodeData(decoded, schema)
		if err != nil || data.UserID != testUserID {
			t.Fatalf("round-trip event decode: %#v, %v", data, err)
		}
	}

	invalidData := map[string]any{"userId": testUserID, "securityRevision": "4", "change": "mfa-enrolled", "recoveryCode": "fixture-proof"}
	if _, err := events.NewEnvelope(events.EnvelopeInput{
		EventID: "60000000-0000-4000-8000-000000000004", AggregateID: testUserID,
		AggregateVersion: mustRevision(t, 4), IntegrationSequence: mustRevision(t, 24),
		OccurredAt:    time.Date(2026, 9, 15, 0, 0, 4, 0, time.UTC),
		Actor:         events.Actor{Kind: events.ActorSystem, ID: "70000000-0000-4000-8000-000000000001"},
		CorrelationID: "80000000-0000-4000-8000-000000000001", CausationID: "90000000-0000-4000-8000-000000000004", OperationID: "a0000000-0000-4000-8000-000000000004",
	}, mustMapEventSchema(t), invalidData); !errors.Is(err, events.ErrSensitivePayload) {
		t.Fatalf("secret-shaped event field error = %v, want ErrSensitivePayload", err)
	}
}

func validateAuthSecurityChanged(value authSecurityChanged) error {
	if !canonicalNonzeroUUID(value.UserID) {
		return errors.New("userId is not a canonical nonzero UUID")
	}
	revision, err := events.ParseRevision(value.SecurityRevision)
	if err != nil || revision.IsZero() {
		return errors.New("securityRevision must be positive signed-64 revision")
	}
	switch value.Change {
	case "credential-created", "credential-replaced", "mfa-enrolled", "sessions-revoked", "bootstrap-enrollment-started":
		return nil
	default:
		return errors.New("unknown auth security change")
	}
}

func replayAuthSecurity(stream []events.Envelope, schema events.EventSchema[authSecurityChanged]) (replayedAuthSecurity, error) {
	var state replayedAuthSecurity
	for _, envelope := range stream {
		data, err := events.DecodeData(envelope, schema)
		if err != nil {
			return state, err
		}
		if state.TransitionCount == 0 {
			if envelope.AggregateVersion().String() != "1" {
				return state, errors.New("replay must start at aggregate revision 1")
			}
			state.UserID = data.UserID
		} else {
			next, err := state.AggregateRevision.Next()
			if err != nil || envelope.AggregateVersion() != next || data.UserID != state.UserID {
				return state, errors.New("aggregate replay gap or subject mismatch")
			}
		}
		securityRevision, err := events.ParseRevision(data.SecurityRevision)
		if err != nil || securityRevision.IsZero() {
			return state, errors.New("invalid security revision")
		}
		if state.TransitionCount > 0 {
			next, err := state.SecurityRevision.Next()
			if err != nil || securityRevision != next {
				return state, errors.New("security revision gap")
			}
		}
		state.SecurityRevision = securityRevision
		state.AggregateRevision = envelope.AggregateVersion()
		state.LastChange = data.Change
		state.TransitionCount++
	}
	return state, nil
}

func validateFixtureSemantics(fixture authFixtureCase) error {
	value, ok := fixture.Input.(map[string]any)
	if !ok {
		return errors.New("fixture input is not an object")
	}
	switch fixture.Schema {
	case "SessionRead":
		data, _ := value["data"].(map[string]any)
		context, _ := data["context"].(map[string]any)
		if value["revision"] != context["revision"] {
			return errors.New("outer/context revision mismatch")
		}
		scope, _ := context["branchScope"].(map[string]any)
		if context["companyId"] == nil && scope["mode"] != "ALL" {
			return errors.New("null company has selected scope")
		}
		mfa, _ := data["mfa"].(map[string]any)
		if mfa["enrolled"] == false && mfa["authenticatedAt"] != nil {
			return errors.New("unenrolled session has MFA time")
		}
		if duplicateObjectKey(data["roles"], "id") || duplicateObjectKey(data["accessibleCompanies"], "id") || duplicateStrings(data["permissions"]) {
			return errors.New("duplicate session identity")
		}
		for _, permission := range stringsFrom(data["permissions"]) {
			if strings.HasPrefix(permission, "fixture.unknown.") {
				return errors.New("unknown permission")
			}
		}
	case "RevokeAllRequest":
		if strings.TrimSpace(stringValue(value["reason"])) == "" {
			return errors.New("blank reason")
		}
	case "RecoveryCompleteRequest":
		if value["newPassword"] != value["confirmation"] {
			return errors.New("confirmation mismatch")
		}
	case "SecurityChangedData":
		data := authSecurityChanged{UserID: stringValue(value["userId"]), SecurityRevision: stringValue(value["securityRevision"]), Change: stringValue(value["change"])}
		if err := validateAuthSecurityChanged(data); err != nil {
			return err
		}
		if aggregate, exists := fixture.Bindings["aggregateId"]; exists && aggregate != data.UserID {
			return errors.New("event user/aggregate mismatch")
		}
	case "AccessError":
		if fixture.ID == "auth.error.wrong-status-code" {
			return errors.New("operation/status code mismatch")
		}
	}
	return nil
}

type authSchemaValidator struct {
	openAPISchemas map[string]any
	eventRoot      map[string]any
	eventDefs      map[string]any
}

func newAuthSchemaValidator(t *testing.T, openAPI, event map[string]any) *authSchemaValidator {
	t.Helper()
	components := objectAt(t, openAPI, "components")
	return &authSchemaValidator{
		openAPISchemas: objectAt(t, components, "schemas"),
		eventRoot:      event,
		eventDefs:      objectAt(t, event, "$defs"),
	}
}

func (v *authSchemaValidator) validateNamed(name string, value any) error {
	if name == "SecurityChangedData" {
		return v.validate(v.eventDefs["securityChangedData"], value, "event:$defs/securityChangedData")
	}
	schema, ok := v.openAPISchemas[name]
	if !ok {
		return fmt.Errorf("unknown schema %s", name)
	}
	return v.validate(schema, value, "#/components/schemas/"+name)
}

func (v *authSchemaValidator) validateEvent(value any) error {
	return v.validate(v.eventRoot, value, "event")
}

func (v *authSchemaValidator) validate(rawSchema, value any, path string) error {
	schema, ok := rawSchema.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: schema is not an object", path)
	}
	if ref, ok := schema["$ref"].(string); ok {
		const openPrefix = "#/components/schemas/"
		const eventPrefix = "#/$defs/"
		switch {
		case strings.HasPrefix(ref, openPrefix):
			return v.validate(v.openAPISchemas[strings.TrimPrefix(ref, openPrefix)], value, path+"->"+ref)
		case strings.HasPrefix(ref, eventPrefix):
			return v.validate(v.eventDefs[strings.TrimPrefix(ref, eventPrefix)], value, path+"->"+ref)
		default:
			return fmt.Errorf("%s: unsupported ref %s", path, ref)
		}
	}
	if variants, ok := schema["oneOf"].([]any); ok {
		matches := 0
		for index, variant := range variants {
			if v.validate(variant, value, fmt.Sprintf("%s.oneOf[%d]", path, index)) == nil {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("%s: oneOf matched %d alternatives", path, matches)
		}
		return nil
	}
	if rejected, ok := schema["not"].(map[string]any); ok && v.validate(rejected, value, path+".not") == nil {
		return fmt.Errorf("%s: value matches forbidden schema", path)
	}
	if expected, exists := schema["const"]; exists && !jsonEqual(expected, value) {
		return fmt.Errorf("%s: const mismatch", path)
	}
	if values, ok := schema["enum"].([]any); ok {
		matched := false
		for _, candidate := range values {
			matched = matched || jsonEqual(candidate, value)
		}
		if !matched {
			return fmt.Errorf("%s: enum mismatch", path)
		}
	}
	if schema["type"] == nil {
		return nil
	}

	types := []string{}
	switch rawType := schema["type"].(type) {
	case string:
		types = append(types, rawType)
	case []any:
		for _, item := range rawType {
			types = append(types, stringValue(item))
		}
	case nil:
		return fmt.Errorf("%s: missing type", path)
	default:
		return fmt.Errorf("%s: malformed type", path)
	}
	if value == nil {
		if containsString(types, "null") {
			return nil
		}
		return fmt.Errorf("%s: null is not allowed", path)
	}
	typeName := ""
	for _, candidate := range types {
		if candidate != "null" {
			typeName = candidate
			break
		}
	}
	switch typeName {
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s: expected string", path)
		}
		length := len([]rune(text))
		if minimum, ok := optionalInteger(schema["minLength"]); ok && length < minimum {
			return fmt.Errorf("%s: string shorter than %d", path, minimum)
		}
		if maximum, ok := optionalInteger(schema["maxLength"]); ok && length > maximum {
			return fmt.Errorf("%s: string longer than %d", path, maximum)
		}
		if pattern, ok := schema["pattern"].(string); ok {
			matched, err := regexp.MatchString(pattern, text)
			if err != nil || !matched {
				return fmt.Errorf("%s: pattern mismatch", path)
			}
		}
		if format, _ := schema["format"].(string); format == "uuid" && !canonicalUUID(text) {
			return fmt.Errorf("%s: invalid UUID", path)
		} else if format == "date-time" {
			parsed, err := time.Parse(time.RFC3339Nano, text)
			if err != nil || !strings.HasSuffix(text, "Z") || parsed.Location() != time.UTC {
				return fmt.Errorf("%s: invalid UTC instant", path)
			}
		}
	case "integer":
		actual, ok := exactInteger(value)
		if !ok {
			return fmt.Errorf("%s: expected integer", path)
		}
		if minimum, ok := exactInteger(schema["minimum"]); ok && actual < minimum {
			return fmt.Errorf("%s: integer below minimum", path)
		}
		if maximum, ok := exactInteger(schema["maximum"]); ok && actual > maximum {
			return fmt.Errorf("%s: integer above maximum", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s: expected boolean", path)
		}
	case "null":
		return fmt.Errorf("%s: expected null", path)
	case "array":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s: expected array", path)
		}
		if minimum, ok := optionalInteger(schema["minItems"]); ok && len(items) < minimum {
			return fmt.Errorf("%s: too few items", path)
		}
		if maximum, ok := optionalInteger(schema["maxItems"]); ok && len(items) > maximum {
			return fmt.Errorf("%s: too many items", path)
		}
		seen := map[string]struct{}{}
		for index, item := range items {
			if err := v.validate(schema["items"], item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
			if schema["uniqueItems"] == true {
				key := stableJSON(item)
				if _, duplicate := seen[key]; duplicate {
					return fmt.Errorf("%s: duplicate array item", path)
				}
				seen[key] = struct{}{}
			}
		}
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: expected object", path)
		}
		properties, closed := schema["properties"].(map[string]any)
		if closed {
			for _, required := range stringsFrom(schema["required"]) {
				if _, ok := object[required]; !ok {
					return fmt.Errorf("%s: missing %s", path, required)
				}
			}
			for key, item := range object {
				child, known := properties[key]
				if !known {
					if schema["additionalProperties"] == false {
						return fmt.Errorf("%s: unknown property %s", path, key)
					}
					continue
				}
				if err := v.validate(child, item, path+"."+key); err != nil {
					return err
				}
			}
		} else if additional, ok := schema["additionalProperties"].(map[string]any); ok {
			for key, item := range object {
				if err := v.validate(additional, item, path+"."+key); err != nil {
					return err
				}
			}
		} else {
			return fmt.Errorf("%s: object is not explicitly closed or typed", path)
		}
	default:
		return fmt.Errorf("%s: unsupported type %s", path, typeName)
	}
	return nil
}

func assertClosedSchemas(t *testing.T, schemas map[string]any) {
	t.Helper()
	var walk func(any, string)
	walk = func(raw any, path string) {
		schema, ok := raw.(map[string]any)
		if !ok {
			return
		}
		if schema["type"] == "object" {
			_, fixed := schema["properties"]
			_, typed := schema["additionalProperties"].(map[string]any)
			if fixed && schema["additionalProperties"] != false || !fixed && !typed {
				t.Errorf("%s is not a closed object or explicit typed map", path)
			}
		}
		for key, child := range schema {
			switch key {
			case "properties":
				if properties, ok := child.(map[string]any); ok {
					for name, property := range properties {
						walk(property, path+"."+name)
					}
				}
			case "items", "additionalProperties":
				walk(child, path+"."+key)
			case "oneOf":
				if variants, ok := child.([]any); ok {
					for index, variant := range variants {
						walk(variant, fmt.Sprintf("%s.oneOf[%d]", path, index))
					}
				}
			}
		}
	}
	for name, schema := range schemas {
		walk(schema, name)
	}
}

func successStatus(path, method string) string {
	switch {
	case path == "/api/v1/identity/session/recovery/request":
		return "202"
	case path == "/api/v1/identity/session/logout", path == "/api/v1/identity/session/recovery/complete":
		return "204"
	default:
		return "200"
	}
}

func authRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func readJSONObject(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	decodeJSON(t, raw, &value)
	return value
}

func readAuthFixtures(t *testing.T, path string) authFixtureFile {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixtures authFixtureFile
	decodeJSON(t, raw, &fixtures)
	return fixtures
}

func decodeJSON(t *testing.T, raw []byte, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
	if decoder.More() {
		t.Fatal("trailing JSON")
	}
}

func objectAt(t *testing.T, object map[string]any, key string) map[string]any {
	t.Helper()
	return asObject(t, object[key], key)
}

func asObject(t *testing.T, value any, name string) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object: %#v", name, value)
	}
	return object
}

func arrayAt(t *testing.T, object map[string]any, key string) []any {
	t.Helper()
	array, ok := object[key].([]any)
	if !ok {
		t.Fatalf("%s is not an array: %#v", key, object[key])
	}
	return array
}

func integer(value any) int64 {
	result, _ := exactInteger(value)
	return result
}

func exactInteger(value any) (int64, bool) {
	switch value := value.(type) {
	case json.Number:
		parsed, err := value.Int64()
		return parsed, err == nil
	case float64:
		return int64(value), value == float64(int64(value))
	case int:
		return int64(value), true
	case int64:
		return value, true
	default:
		return 0, false
	}
}

func optionalInteger(value any) (int, bool) {
	if value == nil {
		return 0, false
	}
	integer, ok := exactInteger(value)
	return int(integer), ok
}

func canonicalUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}

func canonicalNonzeroUUID(value string) bool {
	return canonicalUUID(value) && value != uuid.Nil.String()
}

func jsonEqual(left, right any) bool { return stableJSON(left) == stableJSON(right) }

func stableJSON(value any) string {
	return string(mustJSON(canonicalJSON(value)))
}

func canonicalJSON(value any) any {
	switch value := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		ordered := make([]any, 0, len(keys)*2)
		for _, key := range keys {
			ordered = append(ordered, key, canonicalJSON(value[key]))
		}
		return ordered
	case []any:
		result := make([]any, len(value))
		for index := range value {
			result[index] = canonicalJSON(value[index])
		}
		return result
	default:
		return value
	}
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func stringsFrom(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func duplicateStrings(value any) bool {
	seen := map[string]struct{}{}
	for _, item := range stringsFrom(value) {
		if _, exists := seen[item]; exists {
			return true
		}
		seen[item] = struct{}{}
	}
	return false
}

func duplicateObjectKey(value any, key string) bool {
	items, _ := value.([]any)
	seen := map[string]struct{}{}
	for _, item := range items {
		object, _ := item.(map[string]any)
		field := stringValue(object[key])
		if _, exists := seen[field]; exists {
			return true
		}
		seen[field] = struct{}{}
	}
	return false
}

func assertExpected(t *testing.T, classes map[string]string, fixtures []authFixtureCase, id, field string, expected any) {
	t.Helper()
	if classes[id] != "stateTransportNegative" {
		t.Fatalf("%s is not a state/transport fixture", id)
	}
	for _, fixture := range fixtures {
		if fixture.ID == id {
			if !reflect.DeepEqual(fixture.Expected[field], expected) {
				t.Fatalf("%s expected.%s = %#v, want %#v", id, field, fixture.Expected[field], expected)
			}
			return
		}
	}
	t.Fatalf("missing fixture %s", id)
}

func mustRevision(t *testing.T, value int64) events.Revision {
	t.Helper()
	revision, err := events.NewRevision(value)
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func mustMapEventSchema(t *testing.T) events.EventSchema[map[string]any] {
	t.Helper()
	schema, err := events.NewEventSchema("identity.auth.security-changed.v1", 1, "auth-security", events.CompanyScopeGlobalAllowed, func(map[string]any) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return schema
}
