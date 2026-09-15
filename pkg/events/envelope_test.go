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

func validEnvelopeInput() EnvelopeInput {
	aggregateVersion, _ := ParseRevision("5")
	integrationSequence, _ := ParseRevision("2")
	company := companyID
	return EnvelopeInput{
		EventID: eventID, EventType: "inventory.reservation.granted.v1", SchemaVersion: 1,
		AggregateType: "reservation-set", AggregateID: aggregateID,
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
	envelope, err := NewEnvelope(validEnvelopeInput(), validPayload())
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
	payload, err := DecodeData[reservationGrantedV1](decoded)
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
	envelope, err := NewEnvelopeJSON(input, raw)
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
		"unknown owner":           func(in *EnvelopeInput) { in.EventType = "billing.invoice.created.v1" },
		"schema mismatch":         func(in *EnvelopeInput) { in.SchemaVersion = 2 },
		"invalid aggregate type":  func(in *EnvelopeInput) { in.AggregateType = "ReservationSet" },
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
			if _, err := NewEnvelope(input, validPayload()); !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("error = %v, want ErrInvalidEnvelope", err)
			}
		})
	}
}

func TestGlobalIdentityEventMayHaveNullCompany(t *testing.T) {
	input := validEnvelopeInput()
	input.EventType = "identity.platform.bootstrap-recorded.v1"
	input.CompanyID = nil
	envelope, err := NewEnvelope(input, struct {
		BootstrapRef string `json:"bootstrapRef"`
	}{BootstrapRef: "10000000-0000-4000-8000-000000000010"})
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	if _, ok := envelope.CompanyID(); ok {
		t.Fatal("global identity event unexpectedly has companyId")
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
	} {
		if _, err := NewEnvelopeJSON(validEnvelopeInput(), json.RawMessage(raw)); !errors.Is(err, ErrSensitivePayload) {
			t.Errorf("payload %s error = %v, want ErrSensitivePayload", raw, err)
		}
	}

	for _, raw := range []string{
		`{"piiRef":"10000000-0000-4000-8000-000000000010"}`,
		`{"noteRef":"10000000-0000-4000-8000-000000000011"}`,
		`{"credentialId":"10000000-0000-4000-8000-000000000012"}`,
	} {
		if _, err := NewEnvelopeJSON(validEnvelopeInput(), json.RawMessage(raw)); err != nil {
			t.Errorf("safe reference payload %s rejected: %v", raw, err)
		}
	}
}

func TestZeroEnvelopeCannotBeSerialized(t *testing.T) {
	if _, err := json.Marshal(Envelope{}); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("Marshal zero Envelope error = %v, want ErrInvalidEnvelope", err)
	}
}

func TestEnvelopeRejectsMalformedPayloadAndUnknownEnvelopeOrDataFields(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{`, `{} {}`} {
		if _, err := NewEnvelopeJSON(validEnvelopeInput(), json.RawMessage(raw)); !errors.Is(err, ErrInvalidEnvelope) {
			t.Errorf("payload %s error = %v", raw, err)
		}
	}

	envelope, err := NewEnvelope(validEnvelopeInput(), validPayload())
	if err != nil {
		t.Fatal(err)
	}
	withUnknownData, err := NewEnvelopeJSON(validEnvelopeInput(), json.RawMessage(`{"holder":{"service":"retail","id":"10000000-0000-4000-8000-000000000008"},"future":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeData[reservationGrantedV1](withUnknownData); err == nil {
		t.Fatal("DecodeData accepted a field outside the selected schema")
	}

	wire, _ := json.Marshal(envelope)
	wire = append(wire[:len(wire)-1], []byte(`,"transport":"amqp"}`)...)
	var decoded Envelope
	if err := json.Unmarshal(wire, &decoded); err == nil {
		t.Fatal("Unmarshal accepted an unknown envelope field")
	}
}

func TestEnvelopeRejectsOffsetTimeOnWire(t *testing.T) {
	envelope, err := NewEnvelope(validEnvelopeInput(), validPayload())
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
	envelope, err := NewEnvelope(validEnvelopeInput(), validPayload())
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for i := 0; i < 32; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for j := 0; j < 100; j++ {
				payload, decodeErr := DecodeData[reservationGrantedV1](envelope)
				if decodeErr != nil || payload.Holder.Service != "retail" {
					t.Errorf("DecodeData = (%+v, %v)", payload, decodeErr)
					return
				}
			}
		}()
	}
	wait.Wait()
}
