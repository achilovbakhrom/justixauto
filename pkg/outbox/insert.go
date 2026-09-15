// Package outbox implements owner-local transactional integration persistence.
// Service domain/application packages consume their own typed ports; only outer
// adapters bind these repositories to the eventstore UnitOfWork transaction.
package outbox

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
)

const IntegrationExchange = "justix.integration.v1"

var (
	ErrInvalidRecord = errors.New("invalid integration outbox record")
	ErrEventNotFound = errors.New("matching local integration event not found")
)

// Record seals an explicitly schema-validated integration envelope and its
// route. No byte slices or routing fields are exposed for later mutation.
// A target is technical routing, not authorization: the owner must authorize
// that subscriber and payload before constructing a record.
type Record struct {
	envelope events.Envelope
	body     []byte
	hash     [sha256.Size]byte
	target   events.Owner
}

// NewRecord validates even envelopes decoded from untrusted JSON against the
// supplied owner schema. Serialization happens once; relay retries must use the
// stored bytes, never reconstruct an envelope from current aggregate state.
// The approved schema supports one target per event ID. Multi-target fan-out
// cannot be implemented by reinserting this event or inventing new event IDs.
func NewRecord[T any](envelope events.Envelope, schema events.EventSchema[T], target events.Owner) (Record, error) {
	if !target.Valid() {
		return Record{}, fmt.Errorf("%w: invalid target owner", ErrInvalidRecord)
	}
	if _, err := events.DecodeData(envelope, schema); err != nil {
		return Record{}, fmt.Errorf("%w: %w", ErrInvalidRecord, err)
	}
	body, err := envelope.MarshalJSON()
	if err != nil {
		return Record{}, fmt.Errorf("%w: %w", ErrInvalidRecord, err)
	}
	// AMQP short strings have a byte limit; the event schema validates the
	// component alphabet, so wildcards and empty route components cannot enter.
	if len(route(envelope, target)) > 255 {
		return Record{}, fmt.Errorf("%w: routing key exceeds 255 bytes", ErrInvalidRecord)
	}
	return Record{envelope: envelope, body: body, hash: sha256.Sum256(body), target: target}, nil
}

func route(e events.Envelope, target events.Owner) string {
	return string(e.Owner()) + "." + string(target) + "." + e.EventType()
}

// Inserter neither commits nor publishes. Construct it from the exact tx given
// to the T-009 UnitOfWork factory, alongside append, guards and receipt ports.
type Inserter struct {
	tx    *gorm.DB
	owner events.Owner
}

func NewInserter(tx *gorm.DB, owner events.Owner) (*Inserter, error) {
	if err := requireTransaction(tx); err != nil {
		return nil, err
	}
	if !owner.Valid() {
		return nil, fmt.Errorf("%w: invalid source owner", ErrInvalidRecord)
	}
	return &Inserter{tx: tx, owner: owner}, nil
}

// Insert runs after local append. Every record must match a persisted local
// event's identity, owner, stream/revisions and envelope metadata (including
// company, actor and correlation references). Internal events have NULL sequence
// and cannot match. Integration payload serialization
// is separately schema-approved and need not equal stored domain JSON.
// Empty input is a no-op for an internal-only append. Duplicate identities or
// positions are errors, not successful replays; command receipts own retry
// reconciliation. Propagate every error to Run so all local effects roll back.
func (i *Inserter) Insert(ctx context.Context, records ...Record) error {
	if i == nil {
		return eventstore.ErrTransactionRequired
	}
	if err := requireTransaction(i.tx); err != nil {
		return err
	}
	for _, r := range records {
		if len(r.body) == 0 || !r.target.Valid() || r.envelope.Owner() != i.owner {
			return ErrInvalidRecord
		}
	}
	for _, r := range records {
		e := r.envelope
		var company any
		if id, ok := e.CompanyID(); ok {
			company = id
		}
		result := i.tx.WithContext(ctx).Exec(`INSERT INTO eventstore.outbox
		(event_id,aggregate_type,aggregate_id,integration_sequence,envelope,envelope_hash,exchange,routing_key)
		SELECT event_id,aggregate_type,aggregate_id,integration_sequence,?,?,?,?
		FROM eventstore.events WHERE event_id=?::uuid AND owner_service=?
		AND aggregate_type=? AND aggregate_id=?::uuid AND aggregate_version=? AND integration_sequence=?
		AND event_type=? AND schema_version=? AND company_id IS NOT DISTINCT FROM ?::uuid
		AND occurred_at=? AND actor_kind=? AND actor_id=?::uuid
		AND correlation_id=?::uuid AND causation_id=?::uuid AND operation_id=?::uuid`,
			r.body, r.hash[:], IntegrationExchange, route(e, r.target),
			e.EventID(), string(i.owner), e.AggregateType(), e.AggregateID(), e.AggregateVersion().Int64(), e.IntegrationSequence().Int64(),
			e.EventType(), int64(e.SchemaVersion()), company, e.OccurredAt(), string(e.Actor().Kind), e.Actor().ID,
			e.CorrelationID(), e.CausationID(), e.OperationID())
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrEventNotFound
		}
	}
	return nil
}

func requireTransaction(tx *gorm.DB) error {
	if tx == nil || tx.Error != nil || tx.Statement == nil {
		return eventstore.ErrTransactionRequired
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return eventstore.ErrTransactionRequired
	}
	return nil
}
