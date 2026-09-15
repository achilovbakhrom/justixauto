package eventstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"justixauto/pkg/events"
)

var (
	ErrAggregateNotFound = errors.New("eventstore: aggregate not found")
	ErrInvalidReplay     = errors.New("eventstore: invalid replay request")
	streamTypePattern    = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// Stream is the approved owner-local stream identity. Company authorization and
// business transitions belong to the owner's command/domain layers, not this
// technical loader; company metadata may differ between historical events.
type Stream struct {
	Owner         events.Owner
	AggregateType string
	AggregateID   string
}

func (s Stream) valid() bool {
	id, err := uuid.Parse(s.AggregateID)
	return s.Owner.Valid() && streamTypePattern.MatchString(s.AggregateType) && err == nil && id.String() == s.AggregateID
}

// Loader is an infrastructure adapter. Owners expose a typed aggregate-loading
// port to application code, never this database handle or shared package.
type Loader struct {
	db    *gorm.DB
	owner events.Owner
}

func NewLoader(db *gorm.DB, owner events.Owner) (*Loader, error) {
	if db == nil || db.Error != nil || db.Statement == nil || !owner.Valid() {
		return nil, ErrInvalidReplay
	}
	return &Loader{db: db, owner: owner}, nil
}

// Load reads the complete stream in one PostgreSQL statement/snapshot. It has
// no pagination, snapshot shortcut or SELECT of private side records. Concurrent
// appends may appear on a subsequent load; commands still condition their write
// on the replayed revision. Storage failures must never be treated as absence.
// Returned records are structurally valid; Replay must bind owner payload schemas.
func (l *Loader) Load(ctx context.Context, stream Stream) ([]Event, error) {
	if l == nil || l.db == nil || !stream.valid() || stream.Owner != l.owner {
		return nil, ErrInvalidReplay
	}
	var rows []storedEvent
	err := l.db.WithContext(ctx).Raw(`SELECT event_id,event_type,owner_service,schema_version,
	 aggregate_type,aggregate_id,aggregate_version,integration_sequence,company_id,
	 occurred_at,actor_kind,actor_id,correlation_id,causation_id,operation_id,data
	 FROM eventstore.events WHERE aggregate_type=? AND aggregate_id=?::uuid
	 ORDER BY aggregate_version ASC`, stream.AggregateType, stream.AggregateID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	history := make([]Event, 0, len(rows))
	for _, row := range rows {
		e, err := row.event()
		if err != nil {
			return nil, err
		}
		history = append(history, e)
	}
	if err := validateHistory(stream, history); err != nil {
		return nil, err
	}
	return history, nil
}

type storedEvent struct {
	EventID, EventType, OwnerService                            string
	SchemaVersion                                               int64
	AggregateType, AggregateID                                  string
	AggregateVersion                                            int64
	IntegrationSequence                                         sql.NullInt64
	CompanyID                                                   sql.NullString
	OccurredAt                                                  time.Time
	ActorKind, ActorID, CorrelationID, CausationID, OperationID string
	Data                                                        []byte
}

func (r storedEvent) event() (Event, error) {
	sequence := int64(1) // private validator placeholder; SQL NULL stays internal
	if r.IntegrationSequence.Valid {
		sequence = r.IntegrationSequence.Int64
	}
	var company any
	if r.CompanyID.Valid {
		company = r.CompanyID.String
	}
	wire, err := json.Marshal(map[string]any{
		"eventId": r.EventID, "eventType": r.EventType, "schemaVersion": r.SchemaVersion,
		"aggregateType": r.AggregateType, "aggregateId": r.AggregateID,
		"aggregateVersion": fmt.Sprint(r.AggregateVersion), "integrationSequence": fmt.Sprint(sequence),
		"companyId": company, "occurredAt": r.OccurredAt.UTC().Format(time.RFC3339Nano),
		"actor":         events.Actor{Kind: events.ActorKind(r.ActorKind), ID: r.ActorID},
		"correlationId": r.CorrelationID, "causationId": r.CausationID,
		"operationId": r.OperationID, "data": json.RawMessage(r.Data),
	})
	if err != nil {
		return Event{}, ErrCorruptHistory
	}
	var envelope events.Envelope
	if err := json.Unmarshal(wire, &envelope); err != nil || envelope.Owner() != events.Owner(r.OwnerService) {
		return Event{}, ErrCorruptHistory
	}
	return Event{envelope: envelope, internal: !r.IntegrationSequence.Valid}, nil
}

// ReplayMetadata describes the original stored fact, even when its payload is
// upcast for a current handler. Internal events expose sequence zero, never the
// envelope validator's private placeholder. CompanyID is copied per callback.
type ReplayMetadata struct {
	Stream                                  Stream
	EventID, EventType                      string
	SchemaVersion                           uint32
	Revision, IntegrationSequence           events.Revision
	CompanyID                               *string
	OccurredAt                              time.Time
	Actor                                   events.Actor
	CorrelationID, CausationID, OperationID string
}

type ReplayResult[S any] struct {
	State                         S
	Revision, IntegrationSequence events.Revision
}

// Replay validates the complete history and all supported payloads before
// invoking any domain transition. fresh must return a new isolated aggregate;
// apply must mutate only that aggregate and must have no external effects.
// On failure no partial result is returned. Replay does not emit new events or
// consult snapshots. Empty history is explicitly not-found, not a new aggregate.
func Replay[S, T any](ctx context.Context, stream Stream, history []Event, decoders []Decoder[T], fresh func() S, apply func(S, ReplayMetadata, T) (S, error)) (ReplayResult[S], error) {
	var zero ReplayResult[S]
	if fresh == nil || apply == nil {
		return zero, ErrInvalidReplay
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if err := validateHistory(stream, history); err != nil {
		return zero, err
	}
	data := make([]T, len(history))
	for i, e := range history {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		value, err := decodeReplay(e.envelope, decoders)
		if err != nil {
			return zero, err
		}
		data[i] = value
	}
	result := ReplayResult[S]{State: fresh()}
	for i, e := range history {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		m := metadata(e)
		state, err := apply(result.State, m, data[i])
		if err != nil {
			return zero, err
		}
		result.State, result.Revision = state, m.Revision
		if !e.internal {
			result.IntegrationSequence = m.IntegrationSequence
		}
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return result, nil
}

func validateHistory(stream Stream, history []Event) error {
	if !stream.valid() {
		return ErrInvalidReplay
	}
	if len(history) == 0 {
		return ErrAggregateNotFound
	}
	var revision, sequence events.Revision
	ids := make(map[string]bool, len(history))
	for _, event := range history {
		e := event.envelope
		if e.Validate() != nil || e.Owner() != stream.Owner || e.AggregateType() != stream.AggregateType || e.AggregateID() != stream.AggregateID || ids[e.EventID()] {
			return ErrCorruptHistory
		}
		ids[e.EventID()] = true
		next, err := revision.Next()
		if err != nil || e.AggregateVersion() != next {
			return ErrCorruptHistory
		}
		revision = next
		if !event.internal {
			next, err = sequence.Next()
			if err != nil || e.IntegrationSequence() != next {
				return ErrCorruptHistory
			}
			sequence = next
		}
	}
	return nil
}

func metadata(event Event) ReplayMetadata {
	e := event.envelope
	m := ReplayMetadata{Stream: Stream{Owner: e.Owner(), AggregateType: e.AggregateType(), AggregateID: e.AggregateID()}, EventID: e.EventID(), EventType: e.EventType(), SchemaVersion: e.SchemaVersion(), Revision: e.AggregateVersion(), OccurredAt: e.OccurredAt(), Actor: e.Actor(), CorrelationID: e.CorrelationID(), CausationID: e.CausationID(), OperationID: e.OperationID()}
	if company, ok := e.CompanyID(); ok {
		m.CompanyID = &company
	}
	if !event.internal {
		m.IntegrationSequence = e.IntegrationSequence()
	}
	return m
}
