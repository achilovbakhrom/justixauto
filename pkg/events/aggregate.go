package events

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidAggregate = errors.New("invalid aggregate")
	ErrInvalidChange    = errors.New("invalid aggregate change")
)

// Change is an immutable-by-contract aggregate change. Owners must provide an
// immutable event value: Go cannot deep-copy arbitrary generic maps, slices or
// pointers. The package guarantees atomic publication only under that
// contract.
type Change[E any] struct {
	revision   Revision
	occurredAt time.Time
	value      E
}

// NewChange constructs a validated aggregate change. The event value must be
// immutable by contract; this package cannot deep-copy arbitrary generic maps,
// slices or pointers.
func NewChange[E any](revision Revision, occurredAt time.Time, value E) (Change[E], error) {
	if revision.IsZero() {
		return Change[E]{}, fmt.Errorf("%w: revision must be positive", ErrInvalidChange)
	}
	if occurredAt.IsZero() {
		return Change[E]{}, fmt.Errorf("%w: occurredAt is required", ErrInvalidChange)
	}
	_, offset := occurredAt.Zone()
	if offset != 0 {
		return Change[E]{}, fmt.Errorf("%w: occurredAt must have a zero UTC offset", ErrInvalidChange)
	}
	return Change[E]{revision: revision, occurredAt: occurredAt.UTC(), value: value}, nil
}

func (c Change[E]) Revision() Revision { return c.revision }

func (c Change[E]) OccurredAt() time.Time { return c.occurredAt }

// Value returns the event value. The value is immutable by contract; callers
// must use immutable owner values because arbitrary generic maps, slices and
// pointers cannot be deep-copied by this package.
func (c Change[E]) Value() E { return c.value }

// Evolve is a pure aggregate transition. It must not mutate its input state or
// event value, and S and E must be immutable owner values by contract. Go
// cannot deep-copy arbitrary generic maps, slices or pointers; Aggregate only
// provides atomic publication under this explicit contract.
type Evolve[S, E any] func(S, Change[E]) (S, error)

// Aggregate is a command-scoped, pure aggregate lifecycle. S and E must be
// immutable owner values and Evolve must be pure: Go cannot deep-copy
// arbitrary generic maps, slices or pointers. Atomic publication is provided
// only under that explicit owner contract.
type Aggregate[S, E any] struct {
	id               string
	aggregateType    string
	state            S
	revision         Revision
	expectedRevision Revision
	evolve           Evolve[S, E]
	pending          []Change[E]
}

// NewAggregate constructs a new aggregate at revision zero.
func NewAggregate[S, E any](id, aggregateType string, initial S, evolve Evolve[S, E]) (*Aggregate[S, E], error) {
	if !isCanonicalUUID(id) {
		return nil, fmt.Errorf("%w: id must be a canonical UUID", ErrInvalidAggregate)
	}
	if !aggregateTypePattern.MatchString(aggregateType) {
		return nil, fmt.Errorf("%w: aggregate type must be a lower-case slug", ErrInvalidAggregate)
	}
	if evolve == nil {
		return nil, fmt.Errorf("%w: evolve function is required", ErrInvalidAggregate)
	}
	return &Aggregate[S, E]{
		id: id, aggregateType: aggregateType, state: initial, evolve: evolve,
	}, nil
}

// RestoreAggregate constructs an aggregate at a validated persisted revision.
// The restored revision is both the current and expected revision until a
// command raises a new change.
func RestoreAggregate[S, E any](id, aggregateType string, state S, revision Revision, evolve Evolve[S, E]) (*Aggregate[S, E], error) {
	if !isCanonicalUUID(id) {
		return nil, fmt.Errorf("%w: id must be a canonical UUID", ErrInvalidAggregate)
	}
	if !aggregateTypePattern.MatchString(aggregateType) {
		return nil, fmt.Errorf("%w: aggregate type must be a lower-case slug", ErrInvalidAggregate)
	}
	if evolve == nil {
		return nil, fmt.Errorf("%w: evolve function is required", ErrInvalidAggregate)
	}
	return &Aggregate[S, E]{
		id: id, aggregateType: aggregateType, state: state,
		revision: revision, expectedRevision: revision, evolve: evolve,
	}, nil
}

func (a *Aggregate[S, E]) ID() string {
	if a == nil {
		return ""
	}
	return a.id
}

func (a *Aggregate[S, E]) Type() string {
	if a == nil {
		return ""
	}
	return a.aggregateType
}

// State returns the current state. State values must be immutable by
// contract; arbitrary generic maps, slices and pointers cannot be deep-copied.
func (a *Aggregate[S, E]) State() (zero S) {
	if a == nil {
		return zero
	}
	return a.state
}

func (a *Aggregate[S, E]) Revision() (zero Revision) {
	if a == nil {
		return zero
	}
	return a.revision
}

func (a *Aggregate[S, E]) ExpectedRevision() (zero Revision) {
	if a == nil {
		return zero
	}
	return a.expectedRevision
}

// Pending returns a defensive copy of command-local changes in raise order.
// Change values and their event values are immutable by contract; this package
// cannot deep-copy arbitrary generic maps, slices or pointers.
func (a *Aggregate[S, E]) Pending() []Change[E] {
	if a == nil {
		return nil
	}
	return append([]Change[E](nil), a.pending...)
}

// Raise computes the next revision, applies the pure transition, and publishes
// the resulting state and change only after the transition succeeds.
func (a *Aggregate[S, E]) Raise(occurredAt time.Time, event E) error {
	if a == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidAggregate)
	}
	nextRevision, err := a.revision.Next()
	if err != nil {
		return err
	}
	change, err := NewChange(nextRevision, occurredAt, event)
	if err != nil {
		return err
	}
	nextState, err := a.evolve(a.state, change)
	if err != nil {
		return err
	}
	a.state = nextState
	a.revision = nextRevision
	a.pending = append(a.pending, change)
	return nil
}
