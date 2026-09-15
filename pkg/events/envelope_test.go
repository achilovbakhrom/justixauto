package events

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	eventID       = "10000000-0000-4000-8000-000000000001"
	aggregateID   = "10000000-0000-4000-8000-000000000002"
	companyID     = "10000000-0000-4000-8000-000000000003"
	actorID       = "10000000-0000-4000-8000-000000000004"
	correlationID = "10000000-0000-4000-8000-000000000005"
	causationID   = "10000000-0000-4000-8000-000000000006"
	operationID   = "10000000-0000-4000-8000-000000000007"
)

type reservationGrantedV1 struct {
	Holder struct {
		Service string `json:"service"`
		ID      string `json:"id"`
	} `json:"holder"`
	EvidenceRef string `json:"evidenceRef,omitempty"`
}

func reservationGrantedSchema(t *testing.T) EventSchema[reservationGrantedV1] {
	t.Helper()
	schema, err := NewEventSchema("inventory.reservation.granted.v1", 1, "reservation-set", CompanyScopeTenantRequired, func(payload reservationGrantedV1) error {
		if payload.Holder.Service == "" {
			return errors.New("holder.service is required")
		}
		if !isCanonicalUUID(payload.Holder.ID) {
			return errors.New("holder.id must be a canonical UUID")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NewEventSchema: %v", err)
	}
	return schema
}

func mapSchema(t *testing.T, eventType string, version uint32, aggregateType string, scope CompanyScope) EventSchema[map[string]any] {
	t.Helper()
	schema, err := NewEventSchema(eventType, version, aggregateType, scope, func(map[string]any) error { return nil })
	if err != nil {
		t.Fatalf("NewEventSchema: %v", err)
	}
	return schema
}

func validEnvelopeInput() EnvelopeInput {
	aggregateVersion, _ := ParseRevision("5")
	integrationSequence, _ := ParseRevision("2")
	company := companyID
	return EnvelopeInput{
		EventID: eventID, AggregateID: aggregateID,
		AggregateVersion: aggregateVersion, IntegrationSequence: integrationSequence,
		CompanyID: &company, OccurredAt: time.Date(2026, 9, 13, 10, 0, 0, 123, time.UTC),
		Actor: Actor{Kind: ActorUser, ID: actorID}, CorrelationID: correlationID,
		CausationID: causationID, OperationID: operationID,
	}
}

func validPayload() reservationGrantedV1 {
	var payload reservationGrantedV1
	payload.Holder.Service = "retail"
	payload.Holder.ID = "10000000-0000-4000-8000-000000000008"
	payload.EvidenceRef = "10000000-0000-4000-8000-000000000009"
	return payload
}

func TestEnvelopeRoundTripPreservesContractFields(t *testing.T) {
	envelope, err := NewEnvelope(validEnvelopeInput(), reservationGrantedSchema(t), validPayload())
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	if envelope.Owner() != OwnerInventory {
		t.Fatalf("owner = %q", envelope.Owner())
	}
	if envelope.AggregateVersion().String() != "5" || envelope.IntegrationSequence().String() != "2" {
		t.Fatalf("aggregate revision and integration sequence were conflated: %s/%s", envelope.AggregateVersion(), envelope.IntegrationSequence())
	}

	wire, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, fragment := range []string{`"aggregateVersion":"5"`, `"integrationSequence":"2"`, `"schemaVersion":1`, `"occurredAt":"2026-09-13T10:00:00.000000123Z"`} {
		if !strings.Contains(string(wire), fragment) {
			t.Errorf("wire payload missing %s: %s", fragment, wire)
		}
	}

	var decoded Envelope
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	payload, err := DecodeData(decoded, reservationGrantedSchema(t))
	if err != nil {
		t.Fatalf("DecodeData: %v", err)
	}
	if payload.Holder.Service != "retail" || payload.EvidenceRef == "" {
		t.Fatalf("decoded payload = %+v", payload)
	}
}

func TestEnvelopeCopiesPayloadAndCompanyID(t *testing.T) {
	input := validEnvelopeInput()
	raw := json.RawMessage(`{"holder":{"service":"retail","id":"10000000-0000-4000-8000-000000000008"}}`)
	envelope, err := NewEnvelopeJSON(input, reservationGrantedSchema(t), raw)
	if err != nil {
		t.Fatalf("NewEnvelopeJSON: %v", err)
	}
	raw[2] = 'X'
	*input.CompanyID = "20000000-0000-4000-8000-000000000003"
	copyOut := envelope.Data()
	copyOut[2] = 'X'

	second := envelope.Data()
	if !json.Valid(second) || strings.Contains(string(second), "X") {
		t.Fatalf("stored payload was mutated: %s", second)
	}
	if got, ok := envelope.CompanyID(); !ok || got != companyID {
		t.Fatalf("companyId = %q, %v", got, ok)
	}
}

func TestEnvelopeMetadataValidation(t *testing.T) {
	tests := map[string]func(*EnvelopeInput){
		"noncanonical event UUID": func(in *EnvelopeInput) { in.EventID = "AAAAAAAA-0000-4000-8000-000000000001" },
		"zero aggregate revision": func(in *EnvelopeInput) { in.AggregateVersion = Revision{} },
		"zero integration sequence": func(in *EnvelopeInput) {
			in.IntegrationSequence = Revision{}
		},
		"missing company":    func(in *EnvelopeInput) { in.CompanyID = nil },
		"unknown actor kind": func(in *EnvelopeInput) { in.Actor.Kind = "partner" },
		"non-UTC time": func(in *EnvelopeInput) {
			in.OccurredAt = in.OccurredAt.In(time.FixedZone("UTC+5", 5*60*60))
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := validEnvelopeInput()
			mutate(&input)
			if _, err := NewEnvelope(input, reservationGrantedSchema(t), validPayload()); !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("error = %v, want ErrInvalidEnvelope", err)
			}
		})
	}
}

func TestEventSchemaValidation(t *testing.T) {
	validator := func(map[string]any) error { return nil }
	for name, create := range map[string]func() error{
		"unknown owner": func() error {
			_, err := NewEventSchema("billing.invoice.created.v1", 1, "invoice", CompanyScopeTenantRequired, validator)
			return err
		},
		"schema mismatch": func() error {
			_, err := NewEventSchema("inventory.sample.recorded.v2", 1, "sample", CompanyScopeTenantRequired, validator)
			return err
		},
		"invalid aggregate type": func() error {
			_, err := NewEventSchema("inventory.sample.recorded.v1", 1, "Sample", CompanyScopeTenantRequired, validator)
			return err
		},
		"global non-identity": func() error {
			_, err := NewEventSchema("inventory.sample.recorded.v1", 1, "sample", CompanyScopeGlobalAllowed, validator)
			return err
		},
		"missing validator": func() error {
			_, err := NewEventSchema[map[string]any]("inventory.sample.recorded.v1", 1, "sample", CompanyScopeTenantRequired, nil)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := create(); !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("error = %v, want ErrInvalidEnvelope", err)
			}
		})
	}
}

func TestGlobalIdentityEventMayHaveNullCompany(t *testing.T) {
	input := validEnvelopeInput()
	input.CompanyID = nil
	schema := mapSchema(t, "identity.platform-access-guard.bootstrap-recorded.v1", 1, "platform-access-guard", CompanyScopeGlobalAllowed)
	envelope, err := NewEnvelope(input, schema, map[string]any{"bootstrapRef": "10000000-0000-4000-8000-000000000010"})
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	if _, ok := envelope.CompanyID(); ok {
		t.Fatal("global identity event unexpectedly has companyId")
	}
}

func TestTenantIdentityEventRequiresCompany(t *testing.T) {
	input := validEnvelopeInput()
	input.CompanyID = nil
	schema := mapSchema(t, "identity.branch.created.v1", 1, "branch", CompanyScopeTenantRequired)
	if _, err := NewEnvelope(input, schema, map[string]any{"branchId": aggregateID}); !errors.Is(err, ErrPayloadSchemaMismatch) {
		t.Fatalf("tenant identity event error = %v, want ErrPayloadSchemaMismatch", err)
	}
}

func TestEnvelopeRejectsSecretAndRawPIIPayloadsRecursively(t *testing.T) {
	for _, raw := range []string{
		`{"password":"unsafe"}`,
		`{"nested":{"accessToken":"unsafe"}}`,
		`{"people":[{"email":"person@example.test"}]}`,
		`{"note":"raw private note"}`,
		`{"sessionHandle":"unsafe"}`,
		`{"api_key":"unsafe"}`,
		`{"nationalId":"AA1234567"}`,
		`{"nested":[{"BirthDate":"2000-01-01"}]}`,
		`{"nested":[{"surname":"Person"}]}`,
		`{"nested":[{"mobile":"+998000000000"}]}`,
	} {
		if _, err := NewEnvelopeJSON(validEnvelopeInput(), mapSchema(t, "inventory.sample.recorded.v1", 1, "sample", CompanyScopeTenantRequired), json.RawMessage(raw)); !errors.Is(err, ErrSensitivePayload) {
			t.Errorf("payload %s error = %v, want ErrSensitivePayload", raw, err)
		}
	}

	for _, raw := range []string{
		`{"piiRef":"10000000-0000-4000-8000-000000000010"}`,
		`{"noteRef":"10000000-0000-4000-8000-000000000011"}`,
		`{"credentialId":"10000000-0000-4000-8000-000000000012"}`,
	} {
		if _, err := NewEnvelopeJSON(validEnvelopeInput(), mapSchema(t, "inventory.sample.recorded.v1", 1, "sample", CompanyScopeTenantRequired), json.RawMessage(raw)); err != nil {
			t.Errorf("safe reference payload %s rejected: %v", raw, err)
		}
	}
}

func TestPayloadDecoderBindsEventTypeAndVersion(t *testing.T) {
	v2Schema, err := NewEventSchema("inventory.reservation.granted.v2", 2, "reservation-set", CompanyScopeTenantRequired, func(reservationGrantedV1) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := NewEnvelope(validEnvelopeInput(), v2Schema, validPayload())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeData(envelope, reservationGrantedSchema(t)); !errors.Is(err, ErrPayloadSchemaMismatch) {
		t.Fatalf("DecodeData mismatch error = %v, want ErrPayloadSchemaMismatch", err)
	}
}

func TestPayloadDecoderRejectsMissingRequiredFields(t *testing.T) {
	if _, err := NewEnvelopeJSON(validEnvelopeInput(), reservationGrantedSchema(t), json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("NewEnvelopeJSON missing required field error = %v, want ErrInvalidPayload", err)
	}
}

func TestSchemaRejectsAggregateAndScopeSpoofFromRawWire(t *testing.T) {
	input := validEnvelopeInput()
	input.CompanyID = nil
	approvedGlobal := mapSchema(t, "identity.platform-access-guard.bootstrap-recorded.v1", 1, "platform-access-guard", CompanyScopeGlobalAllowed)
	globalEnvelope, err := NewEnvelope(input, approvedGlobal, map[string]any{"bootstrapRef": aggregateID})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(globalEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	wire = []byte(strings.ReplaceAll(string(wire),
		`"eventType":"identity.platform-access-guard.bootstrap-recorded.v1"`,
		`"eventType":"identity.branch.created.v1"`))
	wire = []byte(strings.ReplaceAll(string(wire),
		`"aggregateType":"platform-access-guard"`,
		`"aggregateType":"user"`))
	var untrusted Envelope
	if err := json.Unmarshal(wire, &untrusted); err != nil {
		t.Fatalf("structural wire parse: %v", err)
	}
	branchSchema := mapSchema(t, "identity.branch.created.v1", 1, "branch", CompanyScopeTenantRequired)
	if _, err := DecodeData(untrusted, branchSchema); !errors.Is(err, ErrPayloadSchemaMismatch) {
		t.Fatalf("spoofed aggregate/scope error = %v, want ErrPayloadSchemaMismatch", err)
	}
}

func TestUnapprovedGlobalClaimsHaveNoMatchingSchema(t *testing.T) {
	input := validEnvelopeInput()
	input.CompanyID = nil
	approvedGlobal := mapSchema(t, "identity.platform-access-guard.bootstrap-recorded.v1", 1, "platform-access-guard", CompanyScopeGlobalAllowed)
	globalEnvelope, err := NewEnvelope(input, approvedGlobal, map[string]any{"bootstrapRef": aggregateID})
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := json.Marshal(globalEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	for _, aggregateType := range []string{"permission", "session", "credential", "mfa-factor", "recovery-token"} {
		t.Run(aggregateType, func(t *testing.T) {
			wire := strings.ReplaceAll(string(baseline),
				`"eventType":"identity.platform-access-guard.bootstrap-recorded.v1"`,
				`"eventType":"identity.`+aggregateType+`.recorded.v1"`)
			wire = strings.ReplaceAll(wire,
				`"aggregateType":"platform-access-guard"`,
				`"aggregateType":"`+aggregateType+`"`)
			var structural Envelope
			if err := json.Unmarshal([]byte(wire), &structural); err != nil {
				t.Fatalf("structural parse: %v", err)
			}
			if _, err := DecodeData(structural, approvedGlobal); !errors.Is(err, ErrPayloadSchemaMismatch) {
				t.Fatalf("unapproved global claim error = %v, want ErrPayloadSchemaMismatch", err)
			}
		})
	}
}

func TestZeroEnvelopeCannotBeSerialized(t *testing.T) {
	if _, err := json.Marshal(Envelope{}); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("Marshal zero Envelope error = %v, want ErrInvalidEnvelope", err)
	}
}

func TestEnvelopeRejectsMalformedPayloadAndUnknownEnvelopeOrDataFields(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{`, `{} {}`} {
		if _, err := NewEnvelopeJSON(validEnvelopeInput(), reservationGrantedSchema(t), json.RawMessage(raw)); !errors.Is(err, ErrInvalidEnvelope) {
			t.Errorf("payload %s error = %v", raw, err)
		}
	}

	envelope, err := NewEnvelope(validEnvelopeInput(), reservationGrantedSchema(t), validPayload())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewEnvelopeJSON(validEnvelopeInput(), reservationGrantedSchema(t), json.RawMessage(`{"holder":{"service":"retail","id":"10000000-0000-4000-8000-000000000008"},"future":true}`)); err == nil {
		t.Fatal("NewEnvelopeJSON accepted a field outside the selected schema")
	}

	wire, _ := json.Marshal(envelope)
	wire = append(wire[:len(wire)-1], []byte(`,"transport":"amqp"}`)...)
	var decoded Envelope
	if err := json.Unmarshal(wire, &decoded); err == nil {
		t.Fatal("Unmarshal accepted an unknown envelope field")
	}
}

func TestEnvelopeRejectsOffsetTimeOnWire(t *testing.T) {
	envelope, err := NewEnvelope(validEnvelopeInput(), reservationGrantedSchema(t), validPayload())
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := json.Marshal(envelope)
	wire = []byte(strings.Replace(string(wire), "2026-09-13T10:00:00.000000123Z", "2026-09-13T15:00:00.000000123+05:00", 1))
	var decoded Envelope
	if err := json.Unmarshal(wire, &decoded); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("error = %v, want ErrInvalidEnvelope", err)
	}
}

func TestEnvelopeConcurrentDecodeIsReadOnly(t *testing.T) {
	envelope, err := NewEnvelope(validEnvelopeInput(), reservationGrantedSchema(t), validPayload())
	if err != nil {
		t.Fatal(err)
	}
	schema := reservationGrantedSchema(t)
	var wait sync.WaitGroup
	for i := 0; i < 32; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for j := 0; j < 100; j++ {
				payload, decodeErr := DecodeData(envelope, schema)
				if decodeErr != nil || payload.Holder.Service != "retail" {
					t.Errorf("DecodeData = (%+v, %v)", payload, decodeErr)
					return
				}
			}
		}()
	}
	wait.Wait()
}
