package events

import (
	"errors"
	"testing"
	"time"
)

const testAggregateID = "123e4567-e89b-12d3-a456-426614174000"

func testEvolve(state int, change Change[int]) (int, error) {
	return state + change.Value(), nil
}

func TestNewAggregateValidation(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		typeName string
		evolve   Evolve[int, int]
	}{
		{name: "invalid UUID", id: "not-a-uuid", typeName: "order", evolve: testEvolve},
		{name: "non canonical UUID", id: "123E4567-E89B-12D3-A456-426614174000", typeName: "order", evolve: testEvolve},
		{name: "invalid aggregate type", id: testAggregateID, typeName: "Order", evolve: testEvolve},
		{name: "nil evolve", id: testAggregateID, typeName: "order", evolve: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAggregate(tt.id, tt.typeName, 0, tt.evolve)
			if !errors.Is(err, ErrInvalidAggregate) {
				t.Fatalf("error = %v, want ErrInvalidAggregate", err)
			}
		})
	}
}

func TestNewChangeValidationAndNormalization(t *testing.T) {
	positive, err := NewRevision(1)
	if err != nil {
		t.Fatal(err)
	}
	namedZero := time.FixedZone("named-zero", 0)
	at := time.Date(2026, time.September, 20, 10, 11, 12, 123, namedZero)
	change, err := NewChange(positive, at, "event")
	if err != nil {
		t.Fatal(err)
	}
	if change.OccurredAt().Location() != time.UTC {
		t.Fatalf("location = %v, want time.UTC", change.OccurredAt().Location())
	}
	if !change.OccurredAt().Equal(at) {
		t.Fatalf("occurredAt = %v, want %v", change.OccurredAt(), at)
	}

	zero, err := NewRevision(0)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		revision Revision
		at       time.Time
	}{
		{name: "zero revision", revision: zero, at: time.Now().UTC()},
		{name: "zero time", revision: positive, at: time.Time{}},
		{name: "non zero UTC offset", revision: positive, at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("offset", 3600))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewChange(tt.revision, tt.at, "event")
			if !errors.Is(err, ErrInvalidChange) {
				t.Fatalf("error = %v, want ErrInvalidChange", err)
			}
		})
	}
}

func TestAggregateRevisionsAndPendingOrder(t *testing.T) {
	aggregate, err := NewAggregate(testAggregateID, "order", 10, testEvolve)
	if err != nil {
		t.Fatal(err)
	}
	if !aggregate.Revision().IsZero() || !aggregate.ExpectedRevision().IsZero() {
		t.Fatalf("new revisions = %s/%s, want 0/0", aggregate.Revision(), aggregate.ExpectedRevision())
	}
	firstAt := time.Date(2026, 9, 20, 1, 2, 3, 0, time.FixedZone("zero", 0))
	secondAt := firstAt.Add(time.Minute)
	if err := aggregate.Raise(firstAt, 2); err != nil {
		t.Fatal(err)
	}
	if err := aggregate.Raise(secondAt, 3); err != nil {
		t.Fatal(err)
	}
	if aggregate.State() != 15 || aggregate.Revision().Int64() != 2 || !aggregate.ExpectedRevision().IsZero() {
		t.Fatalf("state/revisions = %d/%s/%s, want 15/2/0", aggregate.State(), aggregate.Revision(), aggregate.ExpectedRevision())
	}
	pending := aggregate.Pending()
	if len(pending) != 2 || pending[0].Revision().Int64() != 1 || pending[1].Revision().Int64() != 2 {
		t.Fatalf("pending revisions = %#v, want [1 2]", pending)
	}
	if pending[0].OccurredAt().Location() != time.UTC || pending[1].OccurredAt().Location() != time.UTC {
		t.Fatal("pending times were not normalized to UTC")
	}

	restoredRevision, err := NewRevision(8)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreAggregate(testAggregateID, "order", 4, restoredRevision, testEvolve)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Revision().Int64() != 8 || restored.ExpectedRevision().Int64() != 8 {
		t.Fatalf("restored revisions = %s/%s, want 8/8", restored.Revision(), restored.ExpectedRevision())
	}
	if err := restored.Raise(firstAt, 1); err != nil {
		t.Fatal(err)
	}
	if restored.Revision().Int64() != 9 || restored.ExpectedRevision().Int64() != 8 || restored.Pending()[0].Revision().Int64() != 9 {
		t.Fatalf("restored raise revisions = %s/%s/%s, want 9/8/9", restored.Revision(), restored.ExpectedRevision(), restored.Pending()[0].Revision())
	}
}

func TestAggregateFailedTransitionIsAtomicAndPreservesError(t *testing.T) {
	transitionErr := errors.New("transition rejected")
	aggregate, err := NewAggregate(testAggregateID, "order", 10, func(state int, _ Change[int]) (int, error) {
		return state + 1, transitionErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := aggregate.Raise(time.Now().UTC(), 1); err != transitionErr {
		t.Fatalf("error = %v, want exact transition error identity", err)
	}
	if aggregate.State() != 10 || !aggregate.Revision().IsZero() || aggregate.ExpectedRevision().Int64() != 0 || aggregate.Pending() != nil {
		t.Fatalf("aggregate changed after failed transition: state=%d revision=%s expected=%s pending=%v", aggregate.State(), aggregate.Revision(), aggregate.ExpectedRevision(), aggregate.Pending())
	}
}

func TestAggregateRaiseRejectsInvalidChangeBeforeTransition(t *testing.T) {
	called := false
	aggregate, err := NewAggregate(testAggregateID, "order", 10, func(state int, _ Change[int]) (int, error) {
		called = true
		return state + 1, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := aggregate.Raise(time.Time{}, 1); !errors.Is(err, ErrInvalidChange) {
		t.Fatalf("error = %v, want ErrInvalidChange", err)
	}
	if called || aggregate.State() != 10 || !aggregate.Revision().IsZero() || aggregate.Pending() != nil {
		t.Fatalf("aggregate changed or transition invoked after invalid change")
	}
}

func TestAggregateRevisionOverflowIsAtomic(t *testing.T) {
	maximum, err := NewRevision(int64(^uint64(0) >> 1))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	aggregate, err := RestoreAggregate(testAggregateID, "order", 10, maximum, func(state int, _ Change[int]) (int, error) {
		called = true
		return state + 1, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := aggregate.Raise(time.Now().UTC(), 1); !errors.Is(err, ErrRevisionOverflow) {
		t.Fatalf("error = %v, want ErrRevisionOverflow", err)
	}
	if called || aggregate.State() != 10 || aggregate.Revision() != maximum || aggregate.ExpectedRevision() != maximum || aggregate.Pending() != nil {
		t.Fatalf("aggregate changed or transition invoked after overflow")
	}
}

func TestAggregatePendingIsDefensiveCopy(t *testing.T) {
	aggregate, err := NewAggregate(testAggregateID, "order", 0, testEvolve)
	if err != nil {
		t.Fatal(err)
	}
	if err := aggregate.Raise(time.Now().UTC(), 1); err != nil {
		t.Fatal(err)
	}
	pending := aggregate.Pending()
	pending[0] = Change[int]{}
	again := aggregate.Pending()
	if len(again) != 1 || again[0].Revision().Int64() != 1 || again[0].Value() != 1 {
		t.Fatalf("pending collection changed through returned slice: %#v", again)
	}
}

func TestNilAggregateBehavior(t *testing.T) {
	var aggregate *Aggregate[int, int]
	if aggregate.ID() != "" || aggregate.Type() != "" || aggregate.State() != 0 || !aggregate.Revision().IsZero() || !aggregate.ExpectedRevision().IsZero() {
		t.Fatal("nil receiver accessor did not return zero value")
	}
	if aggregate.Pending() != nil {
		t.Fatal("nil receiver Pending must return nil")
	}
	if err := aggregate.Raise(time.Now().UTC(), 1); !errors.Is(err, ErrInvalidAggregate) {
		t.Fatalf("error = %v, want ErrInvalidAggregate", err)
	}
}
