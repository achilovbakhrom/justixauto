package eventstore_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
)

type oldReference struct {
	Ref string `json:"ref"`
}
type currentReference struct {
	SecureRecordRef string `json:"secureRecordRef"`
	Removed         bool   `json:"removed"`
}

func referenceSchemas(t *testing.T) (events.EventSchema[oldReference], events.EventSchema[currentReference]) {
	t.Helper()
	old, err := events.NewEventSchema("inventory.fixture.reference.v1", 1, "fixture", events.CompanyScopeTenantRequired, func(p oldReference) error {
		if _, err := uuid.Parse(p.Ref); err != nil {
			return errors.New("invalid reference")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := events.NewEventSchema("inventory.fixture.reference.v2", 2, "fixture", events.CompanyScopeTenantRequired, func(p currentReference) error {
		if _, err := uuid.Parse(p.SecureRecordRef); err != nil {
			return errors.New("invalid reference")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return old, current
}

func replayInput(id, company string, rev, seq int64) events.EnvelopeInput {
	return events.EnvelopeInput{EventID: uuid.NewString(), AggregateID: id, AggregateVersion: revision(rev), IntegrationSequence: revision(seq), CompanyID: &company, OccurredAt: time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC), Actor: events.Actor{Kind: events.ActorUser, ID: uuid.NewString()}, CorrelationID: uuid.NewString(), CausationID: uuid.NewString(), OperationID: uuid.NewString()}
}

func referenceEvent[T any](t *testing.T, in events.EnvelopeInput, schema events.EventSchema[T], payload T) eventstore.Event {
	t.Helper()
	var event eventstore.Event
	var err error
	if in.IntegrationSequence.IsZero() {
		event, err = eventstore.NewInternalEvent(in, schema, payload)
	} else {
		event, err = eventstore.NewEvent(in, schema, payload)
	}
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func TestTypedUpcastTombstonedReferenceReplay(t *testing.T) {
	old, current := referenceSchemas(t)
	id, company, ref := uuid.NewString(), uuid.NewString(), uuid.NewString()
	stream := eventstore.Stream{Owner: events.OwnerInventory, AggregateType: "fixture", AggregateID: id}
	// No sensitive side-record table/client exists. The historical opaque ID is
	// preserved after the owner records a tombstone in a later event.
	history := []eventstore.Event{
		referenceEvent(t, replayInput(id, company, 1, 1), old, oldReference{Ref: ref}),
		referenceEvent(t, replayInput(id, company, 2, 0), current, currentReference{SecureRecordRef: ref, Removed: true}),
	}
	before, _ := history[0].IntegrationEnvelope()
	beforeBytes, _ := before.MarshalJSON()
	decoders := []eventstore.Decoder[currentReference]{
		eventstore.DecodeWith(current),
		eventstore.UpcastWith(old, current, func(p oldReference) (currentReference, error) { return currentReference{SecureRecordRef: p.Ref}, nil }),
	}
	var metadata []eventstore.ReplayMetadata
	apply := func(_ currentReference, m eventstore.ReplayMetadata, p currentReference) (currentReference, error) {
		metadata = append(metadata, m)
		return p, nil
	}
	first, err := eventstore.Replay(context.Background(), stream, history, decoders, func() currentReference { return currentReference{} }, apply)
	if err != nil {
		t.Fatal(err)
	}
	if first.State != (currentReference{SecureRecordRef: ref, Removed: true}) || first.Revision != revision(2) || first.IntegrationSequence != revision(1) {
		t.Fatalf("unexpected replay: %+v", first)
	}
	if metadata[0].SchemaVersion != 1 || metadata[0].EventType != "inventory.fixture.reference.v1" || !metadata[1].IntegrationSequence.IsZero() {
		t.Fatal("rewrote stored metadata or exposed internal sequence")
	}
	if metadata[0].EventID != before.EventID() || metadata[0].Actor != before.Actor() || metadata[0].CorrelationID != before.CorrelationID() || metadata[0].CausationID != before.CausationID() || metadata[0].OperationID != before.OperationID() || metadata[0].OccurredAt != before.OccurredAt() {
		t.Fatal("lost metadata")
	}
	*metadata[0].CompanyID = uuid.NewString()
	second, err := eventstore.Replay(context.Background(), stream, history, decoders, func() currentReference { return currentReference{} }, apply)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("nondeterministic reconstruction", err)
	}
	after, _ := history[0].IntegrationEnvelope()
	afterBytes, _ := after.MarshalJSON()
	if string(beforeBytes) != string(afterBytes) {
		t.Fatal("replay mutated persisted bytes")
	}
}

func TestUpcastRejectsInvalidMappings(t *testing.T) {
	old, current := referenceSchemas(t)
	id, company := uuid.NewString(), uuid.NewString()
	stream := eventstore.Stream{Owner: events.OwnerInventory, AggregateType: "fixture", AggregateID: id}
	history := []eventstore.Event{referenceEvent(t, replayInput(id, company, 1, 1), old, oldReference{Ref: uuid.NewString()})}
	foreign, err := events.NewEventSchema("commerce.fixture.reference.v2", 2, "fixture", events.CompanyScopeTenantRequired, func(currentReference) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	wrongAggregate, err := events.NewEventSchema("inventory.fixture.reference.v2", 2, "other", events.CompanyScopeTenantRequired, func(currentReference) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	mapping := func(p oldReference) (currentReference, error) { return currentReference{SecureRecordRef: p.Ref}, nil }
	cases := []eventstore.Decoder[currentReference]{
		eventstore.UpcastWith(old, current, nil),
		eventstore.UpcastWith(old, current, func(oldReference) (currentReference, error) { return currentReference{}, nil }),
		eventstore.UpcastWith(old, current, func(oldReference) (currentReference, error) {
			return currentReference{}, errors.New("synthetic-private-value")
		}),
		eventstore.UpcastWith(old, foreign, mapping),
		eventstore.UpcastWith(old, wrongAggregate, mapping),
	}
	for i, decoder := range cases {
		calls := 0
		_, err := eventstore.Replay(context.Background(), stream, history, []eventstore.Decoder[currentReference]{decoder}, func() int { calls++; return 0 }, func(s int, _ eventstore.ReplayMetadata, _ currentReference) (int, error) { calls++; return s, nil })
		if !errors.Is(err, eventstore.ErrInvalidUpcast) || calls != 0 || strings.Contains(err.Error(), "synthetic-private-value") {
			t.Fatalf("case %d: %v calls=%d", i, err, calls)
		}
	}
	same := eventstore.UpcastWith(old, old, func(p oldReference) (oldReference, error) { return p, nil })
	_, err = eventstore.Replay(context.Background(), stream, history, []eventstore.Decoder[oldReference]{same}, func() int { return 0 }, func(s int, _ eventstore.ReplayMetadata, _ oldReference) (int, error) { return s, nil })
	if !errors.Is(err, eventstore.ErrInvalidUpcast) {
		t.Fatal("accepted same-version mapping", err)
	}
}
