package eventstore

import (
	"errors"
	"fmt"

	"justixauto/pkg/events"
)

var (
	ErrUnknownSchema   = errors.New("eventstore: no replay decoder accepts stored schema")
	ErrAmbiguousSchema = errors.New("eventstore: multiple replay decoders accept stored schema")
	ErrInvalidUpcast   = errors.New("eventstore: invalid schema upcast")
)

// Decoder is a schema-bound, typed owner adapter. Decoders, validators and
// transforms must be pure and deterministic: no clock, random values, network
// calls or sensitive side-record lookups. A secure-record reference remains a
// reference even after its side record is tombstoned. Shared replay defines no
// business event registry or private-record lifecycle policy.
type Decoder[T any] struct {
	decode func(events.Envelope) (T, error)
}

// DecodeWith accepts exactly the event name, version, aggregate type and scope
// declared by schema, including its required-field validator.
func DecodeWith[T any](schema events.EventSchema[T]) Decoder[T] {
	return Decoder[T]{decode: func(e events.Envelope) (T, error) {
		return events.DecodeData(e, schema)
	}}
}

// UpcastWith maps an explicitly supported historical schema directly to the
// owner's current schema. Both payloads are validated. Stored event bytes and
// metadata are never rewritten. Owners needing several historical versions
// register one deterministic mapping per version; unknown versions fail closed.
func UpcastWith[Old, Current any](source events.EventSchema[Old], target events.EventSchema[Current], transform func(Old) (Current, error)) Decoder[Current] {
	return Decoder[Current]{decode: func(e events.Envelope) (Current, error) {
		var zero Current
		old, err := events.DecodeData(e, source)
		if err != nil {
			return zero, err
		}
		if transform == nil {
			return zero, ErrInvalidUpcast
		}
		current, err := transform(old)
		if err != nil {
			return zero, fmt.Errorf("%w: transform failed", ErrInvalidUpcast)
		}
		converted, err := events.NewEnvelope(envelopeInput(e), target, current)
		if err != nil {
			return zero, fmt.Errorf("%w: target schema rejected transformed data", ErrInvalidUpcast)
		}
		if converted.Owner() != e.Owner() || converted.AggregateType() != e.AggregateType() || converted.SchemaVersion() <= e.SchemaVersion() {
			return zero, ErrInvalidUpcast
		}
		return events.DecodeData(converted, target)
	}}
}

func decodeReplay[T any](e events.Envelope, decoders []Decoder[T]) (T, error) {
	var result T
	matched := false
	for _, decoder := range decoders {
		if decoder.decode == nil {
			return result, ErrUnknownSchema
		}
		value, err := decoder.decode(e)
		if errors.Is(err, events.ErrPayloadSchemaMismatch) {
			continue
		}
		if err != nil {
			// Do not expose payloads or errors supplied by owner validators.
			return result, fmt.Errorf("eventstore: stored payload rejected: %w", safeDecodeError(err))
		}
		if matched {
			return result, ErrAmbiguousSchema
		}
		matched, result = true, value
	}
	if !matched {
		return result, ErrUnknownSchema
	}
	return result, nil
}

func safeDecodeError(err error) error {
	if errors.Is(err, ErrInvalidUpcast) {
		return ErrInvalidUpcast
	}
	return events.ErrInvalidPayload
}

func envelopeInput(e events.Envelope) events.EnvelopeInput {
	input := events.EnvelopeInput{
		EventID: e.EventID(), AggregateID: e.AggregateID(), AggregateVersion: e.AggregateVersion(),
		IntegrationSequence: e.IntegrationSequence(), OccurredAt: e.OccurredAt(), Actor: e.Actor(),
		CorrelationID: e.CorrelationID(), CausationID: e.CausationID(), OperationID: e.OperationID(),
	}
	if company, ok := e.CompanyID(); ok {
		input.CompanyID = &company
	}
	return input
}
