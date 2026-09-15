package eventstore

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"
	"justixauto/pkg/events"
)

var (
	ErrInvalidAppend   = errors.New("invalid event append")
	ErrVersionConflict = errors.New("aggregate expected revision conflict")
	ErrCorruptHistory  = errors.New("aggregate history is not contiguous")
)

// Event can only be built through an explicit owner-declared payload schema.
// Internal events retain no integration position in SQL. They still use the
// same validated metadata/payload shape; the private placeholder required by
// the integration envelope validator is never persisted or exposed as a wire
// integration event. Only NewEvent produces integration events.
type Event struct {
	envelope events.Envelope
	internal bool
}

// IntegrationEnvelope returns the immutable envelope for transactional outbox
// serialization. Internal-only events cannot be published through this method.
func (e Event) IntegrationEnvelope() (events.Envelope, bool) {
	if e.internal || e.envelope.Validate() != nil {
		return events.Envelope{}, false
	}
	return e.envelope, true
}

func NewEvent[T any](input events.EnvelopeInput, schema events.EventSchema[T], data T) (Event, error) {
	e, err := events.NewEnvelope(input, schema, data)
	return Event{envelope: e}, err
}

// NewInternalEvent requires an unset integration sequence. Internal-only
// revisions do not advance the aggregate's published integration sequence.
func NewInternalEvent[T any](input events.EnvelopeInput, schema events.EventSchema[T], data T) (Event, error) {
	if !input.IntegrationSequence.IsZero() {
		return Event{}, fmt.Errorf("%w: internal event has integration sequence", ErrInvalidAppend)
	}
	input.IntegrationSequence, _ = events.NewRevision(1)
	e, err := events.NewEnvelope(input, schema, data)
	return Event{envelope: e, internal: true}, err
}

// Batch contains one stream's expected current revision and contiguous pending
// events. Stream identity comes from the validated events. Actor/company and
// business transition authorization must be checked by owner command ports;
// this technical adapter does not infer an immutable tenant for all streams.
type Batch struct {
	Expected events.Revision
	Events   []Event
}

// Appender is bound to the SQL transaction supplied by an outer adapter's
// UnitOfWork factory. It does not expose a database handle or commit method.
type Appender struct {
	tx    *gorm.DB
	owner events.Owner
}

func NewAppender(tx *gorm.DB, owner events.Owner) (*Appender, error) {
	if err := requireTransaction(tx); err != nil {
		return nil, err
	}
	if !owner.Valid() {
		return nil, fmt.Errorf("%w: invalid owner", ErrInvalidAppend)
	}
	return &Appender{tx: tx, owner: owner}, nil
}

// Append locks all named streams in sorted order (including absent streams),
// checks expected revisions, then inserts. Call once with every affected stream
// where possible. PostgreSQL unique constraints remain a second concurrency
// fence. The caller must return any error to Run; it must not swallow errors or
// commit partial work. Same-transaction guard/outbox/receipt ports run in U.
func (a *Appender) Append(ctx context.Context, batches ...Batch) error {
	if a == nil {
		return ErrTransactionRequired
	}
	if err := requireTransaction(a.tx); err != nil {
		return err
	}
	if len(batches) == 0 {
		return fmt.Errorf("%w: empty batch set", ErrInvalidAppend)
	}
	ordered := append([]Batch(nil), batches...)
	keys := make(map[string]bool, len(batches))
	for _, batch := range ordered {
		if err := a.validateBatch(batch); err != nil {
			return err
		}
		key := streamKey(batch)
		if keys[key] {
			return fmt.Errorf("%w: repeated stream", ErrInvalidAppend)
		}
		keys[key] = true
	}
	sort.Slice(ordered, func(i, j int) bool { return streamKey(ordered[i]) < streamKey(ordered[j]) })
	tx := a.tx.WithContext(ctx)
	for _, batch := range ordered {
		// Hash collisions only serialize unrelated streams. The namespace and
		// owner prevent accidental lock sharing with other technical uses.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "justixauto:eventstore:"+string(a.owner)+":"+streamKey(batch)).Error; err != nil {
			return err
		}
	}
	for _, batch := range ordered {
		e := batch.Events[0].envelope
		var history struct {
			Count, Revision, Sequence, IntegrationCount int64
		}
		err := tx.Raw(`SELECT count(*) AS count, coalesce(max(aggregate_version),0) AS revision,
		 coalesce(max(integration_sequence),0) AS sequence, count(integration_sequence) AS integration_count
		 FROM eventstore.events WHERE aggregate_type=? AND aggregate_id=?::uuid`, e.AggregateType(), e.AggregateID()).Scan(&history).Error
		if err != nil {
			return err
		}
		if history.Count != history.Revision || history.Sequence != history.IntegrationCount {
			return ErrCorruptHistory
		}
		if history.Revision != batch.Expected.Int64() {
			return ErrVersionConflict
		}
		sequence := history.Sequence
		for _, event := range batch.Events {
			if event.internal {
				continue
			}
			next, err := mustRevision(sequence).Next()
			if err != nil {
				return err
			}
			if event.envelope.IntegrationSequence() != next {
				return fmt.Errorf("%w: noncontiguous integration sequence", ErrInvalidAppend)
			}
			sequence = next.Int64()
		}
	}
	for _, batch := range ordered {
		for _, event := range batch.Events {
			e := event.envelope
			var company, sequence any
			if id, ok := e.CompanyID(); ok {
				company = id
			}
			if !event.internal {
				sequence = e.IntegrationSequence().Int64()
			}
			if err := tx.Exec(`INSERT INTO eventstore.events
			 (event_id,event_type,owner_service,schema_version,aggregate_type,aggregate_id,aggregate_version,integration_sequence,company_id,occurred_at,actor_kind,actor_id,correlation_id,causation_id,operation_id,data)
			 VALUES (?::uuid,?,?,?,?,?::uuid,?,?,?::uuid,?,?,?::uuid,?::uuid,?::uuid,?::uuid,?::jsonb)`,
				e.EventID(), e.EventType(), string(e.Owner()), int64(e.SchemaVersion()), e.AggregateType(), e.AggregateID(), e.AggregateVersion().Int64(), sequence, company, e.OccurredAt(), string(e.Actor().Kind), e.Actor().ID, e.CorrelationID(), e.CausationID(), e.OperationID(), string(e.Data())).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *Appender) validateBatch(batch Batch) error {
	if len(batch.Events) == 0 {
		return fmt.Errorf("%w: empty stream", ErrInvalidAppend)
	}
	first := batch.Events[0].envelope
	revision := batch.Expected
	for _, event := range batch.Events {
		e := event.envelope
		if err := e.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidAppend, err)
		}
		if e.Owner() != a.owner || e.AggregateType() != first.AggregateType() || e.AggregateID() != first.AggregateID() {
			return fmt.Errorf("%w: mixed owner or stream", ErrInvalidAppend)
		}
		var err error
		revision, err = revision.Next()
		if err != nil {
			return err
		}
		if e.AggregateVersion() != revision {
			return fmt.Errorf("%w: noncontiguous aggregate revision", ErrInvalidAppend)
		}
	}
	return nil
}

func streamKey(batch Batch) string {
	e := batch.Events[0].envelope
	return e.AggregateType() + ":" + e.AggregateID()
}
func mustRevision(value int64) events.Revision { r, _ := events.NewRevision(value); return r }
