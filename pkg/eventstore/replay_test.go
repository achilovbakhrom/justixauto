package eventstore_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
)

func TestReplayRejectsHistoryBeforeApplying(t *testing.T) {
	_, schema := referenceSchemas(t)
	id, company, ref := uuid.NewString(), uuid.NewString(), uuid.NewString()
	stream := eventstore.Stream{Owner: events.OwnerInventory, AggregateType: "fixture", AggregateID: id}
	makeEvent := func(rev, seq int64) eventstore.Event {
		return referenceEvent(t, replayInput(id, company, rev, seq), schema, currentReference{SecureRecordRef: ref})
	}
	one, two := makeEvent(1, 1), makeEvent(2, 2)
	dupInput := replayInput(id, company, 2, 2)
	firstEnvelope, _ := one.IntegrationEnvelope()
	dupInput.EventID = firstEnvelope.EventID()
	duplicateID := referenceEvent(t, dupInput, schema, currentReference{SecureRecordRef: ref})
	cases := map[string][]eventstore.Event{
		"missing first": {two}, "gap": {one, makeEvent(3, 2)}, "reordered": {two, one},
		"duplicate revision": {one, one}, "duplicate identity": {one, duplicateID},
		"integration gap": {one, makeEvent(2, 3)}, "initial integration gap": {makeEvent(1, 2)},
		"zero event": {{}}, "foreign stream": {referenceEvent(t, replayInput(uuid.NewString(), company, 1, 1), schema, currentReference{SecureRecordRef: ref})},
	}
	for name, history := range cases {
		t.Run(name, func(t *testing.T) {
			calls := 0
			result, err := eventstore.Replay(context.Background(), stream, history, []eventstore.Decoder[currentReference]{eventstore.DecodeWith(schema)}, func() int { calls++; return 0 }, func(s int, _ eventstore.ReplayMetadata, _ currentReference) (int, error) { calls++; return s + 1, nil })
			if !errors.Is(err, eventstore.ErrCorruptHistory) || calls != 0 || result.State != 0 || !result.Revision.IsZero() {
				t.Fatalf("%+v %v calls=%d", result, err, calls)
			}
		})
	}
	for name, decoders := range map[string][]eventstore.Decoder[currentReference]{"unknown": nil, "zero decoder": {{}}, "duplicate registration": {eventstore.DecodeWith(schema), eventstore.DecodeWith(schema)}} {
		t.Run(name, func(t *testing.T) {
			_, err := eventstore.Replay(context.Background(), stream, []eventstore.Event{one, two}, decoders, func() int { t.Fatal("constructed state before validating schemas"); return 0 }, func(s int, _ eventstore.ReplayMetadata, _ currentReference) (int, error) { return s, nil })
			want := eventstore.ErrUnknownSchema
			if name == "duplicate registration" {
				want = eventstore.ErrAmbiguousSchema
			}
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
		})
	}
}

func TestReplayOwnerTransitionsAndIsolation(t *testing.T) {
	_, schema := referenceSchemas(t)
	id, company, other := uuid.NewString(), uuid.NewString(), uuid.NewString()
	stream := eventstore.Stream{Owner: events.OwnerInventory, AggregateType: "fixture", AggregateID: id}
	history := []eventstore.Event{
		referenceEvent(t, replayInput(id, company, 1, 0), schema, currentReference{SecureRecordRef: uuid.NewString()}),
		referenceEvent(t, replayInput(id, other, 2, 1), schema, currentReference{SecureRecordRef: uuid.NewString()}),
	}
	decoders := []eventstore.Decoder[currentReference]{eventstore.DecodeWith(schema)}
	apply := func(s []string, m eventstore.ReplayMetadata, _ currentReference) ([]string, error) {
		return append(s, *m.CompanyID), nil
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			r, err := eventstore.Replay(context.Background(), stream, history, decoders, func() []string { return nil }, apply)
			if err != nil || len(r.State) != 2 || r.State[0] != company || r.State[1] != other || r.Revision != revision(2) || r.IntegrationSequence != revision(1) {
				t.Errorf("replay: %+v %v", r, err)
			}
		})
	}
	wg.Wait()
	denied := errors.New("owner transition denied")
	r, err := eventstore.Replay(context.Background(), stream, history, decoders, func() int { return 0 }, func(s int, m eventstore.ReplayMetadata, _ currentReference) (int, error) {
		if m.Revision == revision(2) {
			return 0, denied
		}
		return s + 1, nil
	})
	if !errors.Is(err, denied) || r.State != 0 || !r.Revision.IsZero() {
		t.Fatal("partial result escaped", r, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = eventstore.Replay(ctx, stream, history, decoders, func() int { t.Fatal("cancelled replay constructed state"); return 0 }, func(s int, _ eventstore.ReplayMetadata, _ currentReference) (int, error) { return s, nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	_, err = eventstore.Replay(context.Background(), stream, nil, decoders, func() int { return 0 }, func(s int, _ eventstore.ReplayMetadata, _ currentReference) (int, error) { return s, nil })
	if !errors.Is(err, eventstore.ErrAggregateNotFound) {
		t.Fatal(err)
	}
	stream.Owner = events.OwnerCommerce
	_, err = eventstore.Replay(context.Background(), stream, history, decoders, func() int { return 0 }, func(s int, _ eventstore.ReplayMetadata, _ currentReference) (int, error) { return s, nil })
	if !errors.Is(err, eventstore.ErrCorruptHistory) {
		t.Fatal("accepted wrong owner", err)
	}
}

func TestReplayLateSchemaFailuresNeverApplyPrefix(t *testing.T) {
	_, schema := referenceSchemas(t)
	id, company := uuid.NewString(), uuid.NewString()
	stream := eventstore.Stream{Owner: events.OwnerInventory, AggregateType: "fixture", AggregateID: id}
	one := referenceEvent(t, replayInput(id, company, 1, 1), schema, currentReference{SecureRecordRef: uuid.NewString()})
	// Simulate schema-valid historical bytes written by a weaker old validator.
	weak, err := events.NewEventSchema("inventory.fixture.reference.v2", 2, "fixture", events.CompanyScopeTenantRequired, func(currentReference) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	badPayload := referenceEvent(t, replayInput(id, company, 2, 2), weak, currentReference{})
	future, err := events.NewEventSchema("inventory.fixture.reference.v99", 99, "fixture", events.CompanyScopeTenantRequired, func(currentReference) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	futureEvent := referenceEvent(t, replayInput(id, company, 2, 2), future, currentReference{SecureRecordRef: uuid.NewString()})
	for _, tc := range []struct {
		event eventstore.Event
		want  error
	}{{badPayload, events.ErrInvalidPayload}, {futureEvent, eventstore.ErrUnknownSchema}} {
		_, err := eventstore.Replay(context.Background(), stream, []eventstore.Event{one, tc.event}, []eventstore.Decoder[currentReference]{eventstore.DecodeWith(schema)}, func() int { t.Fatal("created state before validating whole history"); return 0 }, func(s int, _ eventstore.ReplayMetadata, _ currentReference) (int, error) {
			t.Fatal("applied valid prefix of unsupported history")
			return s, nil
		})
		if !errors.Is(err, tc.want) {
			t.Fatal(err)
		}
	}
	if _, err := eventstore.NewLoader(nil, events.OwnerInventory); !errors.Is(err, eventstore.ErrInvalidReplay) {
		t.Fatal(err)
	}
	var loader *eventstore.Loader
	if _, err := loader.Load(context.Background(), stream); !errors.Is(err, eventstore.ErrInvalidReplay) {
		t.Fatal(err)
	}
}

func TestReplayPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_EVENTSTORE_REPLAY") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_EVENTSTORE_REPLAY=1 for isolated Docker/PostgreSQL replay tests")
	}
	db := appendDatabase(t)
	loader, err := eventstore.NewLoader(db, events.OwnerInventory)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	id, company := uuid.NewString(), uuid.NewString()
	stream := eventstore.Stream{Owner: events.OwnerInventory, AggregateType: "fixture", AggregateID: id}
	if _, err := loader.Load(ctx, stream); !errors.Is(err, eventstore.ErrAggregateNotFound) {
		t.Fatal(err)
	}
	old, current := referenceSchemas(t)
	ref := uuid.NewString()
	history := []eventstore.Event{
		referenceEvent(t, replayInput(id, company, 1, 1), old, oldReference{Ref: ref}),
		referenceEvent(t, replayInput(id, company, 2, 0), current, currentReference{SecureRecordRef: ref, Removed: true}),
	}
	if err := runner(t, db).Run(ctx, func(u fixtureUOW) error { return u.Append(ctx, eventstore.Batch{Events: history}) }); err != nil {
		t.Fatal(err)
	}
	loaded, err := loader.Load(ctx, stream)
	if err != nil {
		t.Fatal(err)
	}
	decoders := []eventstore.Decoder[currentReference]{eventstore.DecodeWith(current), eventstore.UpcastWith(old, current, func(p oldReference) (currentReference, error) { return currentReference{SecureRecordRef: p.Ref}, nil })}
	got, err := eventstore.Replay(ctx, stream, loaded, decoders, func() currentReference { return currentReference{} }, func(_ currentReference, _ eventstore.ReplayMetadata, p currentReference) (currentReference, error) {
		return p, nil
	})
	if err != nil || !got.State.Removed || got.State.SecureRecordRef != ref || got.Revision != revision(2) {
		t.Fatal(got, err)
	}
	if _, ok := loaded[1].IntegrationEnvelope(); ok {
		t.Fatal("loaded internal event became publishable")
	}
	// A concurrent two-event append is observed as one committed prefix. Load
	// never assembles pages from separate snapshots or exposes half a batch.
	appendRunner := runner(t, db)
	next := []eventstore.Event{
		referenceEvent(t, replayInput(id, company, 3, 2), current, currentReference{SecureRecordRef: ref, Removed: true}),
		referenceEvent(t, replayInput(id, company, 4, 0), current, currentReference{SecureRecordRef: ref, Removed: true}),
	}
	start := make(chan struct{})
	var readers sync.WaitGroup
	for range 8 {
		readers.Go(func() {
			<-start
			for range 5 {
				seen, err := loader.Load(ctx, stream)
				if err != nil || (len(seen) != 2 && len(seen) != 4) {
					t.Errorf("non-atomic read: %d events, %v", len(seen), err)
				}
			}
		})
	}
	close(start)
	if err := appendRunner.Run(ctx, func(u fixtureUOW) error {
		return u.Append(ctx, eventstore.Batch{Expected: revision(2), Events: next})
	}); err != nil {
		t.Error(err)
	}
	readers.Wait()
	if latest, err := loader.Load(ctx, stream); err != nil || len(latest) != 4 {
		t.Fatal("did not observe committed append", err)
	}
	if _, err := loader.Load(ctx, eventstore.Stream{Owner: events.OwnerCommerce, AggregateType: "fixture", AggregateID: id}); !errors.Is(err, eventstore.ErrInvalidReplay) {
		t.Fatal(err)
	}
	bad := uuid.NewString()
	if err := db.Exec(eventSQL("inventory", uuid.NewString(), bad, "'"+company+"'", 2)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load(ctx, eventstore.Stream{Owner: events.OwnerInventory, AggregateType: "fixture", AggregateID: bad}); !errors.Is(err, eventstore.ErrCorruptHistory) {
		t.Fatal("adopted corrupt history", err)
	}
	// The SQL shape accepts a future schema; replay must not silently skip it.
	future := uuid.NewString()
	statement := strings.Replace(eventSQL("inventory", uuid.NewString(), future, "'"+company+"'", 1), "inventory.fixture.recorded.v1',1", "inventory.fixture.recorded.v99',99", 1)
	if !strings.Contains(statement, "recorded.v99',99") {
		t.Fatal("future-schema fixture was not upgraded")
	}
	if err := db.Exec(statement).Error; err != nil {
		t.Fatal(err)
	}
	unknownStream := eventstore.Stream{Owner: events.OwnerInventory, AggregateType: "fixture", AggregateID: future}
	unknown, err := loader.Load(ctx, unknownStream)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eventstore.Replay(ctx, unknownStream, unknown, decoders, func() int { return 0 }, func(s int, _ eventstore.ReplayMetadata, _ currentReference) (int, error) { return s, nil }); !errors.Is(err, eventstore.ErrUnknownSchema) {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := loader.Load(cancelled, stream); !errors.Is(err, context.Canceled) || errors.Is(err, eventstore.ErrAggregateNotFound) {
		t.Fatal("masked cancellation", err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load(ctx, stream); err == nil || errors.Is(err, eventstore.ErrAggregateNotFound) {
		t.Fatal("masked storage failure", err)
	}
}
