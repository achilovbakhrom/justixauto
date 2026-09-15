// Package outbox implements owner-local transactional integration persistence.
// Service domain/application packages consume their own typed ports; only outer
// adapters bind these repositories to the eventstore UnitOfWork transaction.
package outbox

import (
	"context"
	"crypto/sha256"
	"encoding/json"
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
	ErrMessagingMode = errors.New("outbox: incompatible messaging mode or route installation")
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
// This is the explicit legacy single-target API. Custody writers use NewMessage
// and InsertMessages, never repeated legacy records with invented event IDs.
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
	tx         *gorm.DB
	owner      events.Owner
	admissions *SourceAdmissions
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

// Message seals one schema-validated serialization and its owner-approved
// stream rule. It contains no caller-selected recipient list or cached plan.
type Message struct {
	envelope events.Envelope
	body     []byte
	hash     [sha256.Size]byte
	rule     SourcePlanRule
}

func NewMessage[T any](e events.Envelope, schema events.EventSchema[T], rule SourcePlanRule) (Message, error) {
	if _, err := events.DecodeData(e, schema); err != nil {
		return Message{}, fmt.Errorf("%w: %w", ErrInvalidRecord, err)
	}
	if !rule.stream.valid() || !evidence(rule.authority) || len(rule.schemas) == 0 ||
		e.Owner() != rule.stream.owner || e.AggregateType() != rule.stream.aggregateType || e.AggregateID() != rule.stream.aggregateID {
		return Message{}, ErrInvalidRecord
	}
	body, err := e.MarshalJSON()
	if err != nil {
		return Message{}, fmt.Errorf("%w: %w", ErrInvalidRecord, err)
	}
	return Message{e, body, sha256.Sum256(body), rule}, nil
}

// NewCustodyInserter binds to the same SQL transaction as admissions. Call
// admissions.Fence with ALL command streams before append or any stream locks.
// Append all events before InsertMessages; no adapter may escape the callback.
// Construction is inert. Every write checks the installed mode and correction.
func NewCustodyInserter(tx *gorm.DB, owner events.Owner, admissions *SourceAdmissions) (*Inserter, error) {
	i, err := NewInserter(tx, owner)
	if err != nil {
		return nil, err
	}
	if admissions == nil || admissions.owner != owner || requireTransaction(admissions.tx) != nil ||
		admissions.tx.Statement.ConnPool != tx.Statement.ConnPool {
		return nil, ErrInvalidAdmission
	}
	i.admissions = admissions
	return i, nil
}

// InsertMessages resolves the entire admitted set under the source fence and
// validates its evidence immediately before persisting one immutable parent and
// every child. Empty sets require an explicit empty authority. Success is only
// a transaction candidate: the outer Run must also commit guards and command
// receipt. Propagate ALL errors; unknown commit outcomes require authoritative
// receipt reconciliation. There is no internal retry, commit, publish or ACK.
func (i *Inserter) InsertMessages(ctx context.Context, messages ...Message) error {
	if i == nil {
		return eventstore.ErrTransactionRequired
	}
	if i.admissions == nil {
		return ErrMessagingMode
	}
	if err := i.checkMode(ctx, "custody"); err != nil {
		return err
	}
	for _, m := range messages {
		if len(m.body) == 0 || m.envelope.Owner() != i.owner || sha256.Sum256(m.body) != m.hash {
			return ErrInvalidRecord
		}
	}
	for _, m := range messages {
		e := m.envelope
		plan, err := i.admissions.Resolve(ctx, m.rule, e)
		if err != nil {
			return err
		}
		type child struct {
			Destination events.Owner `json:"destination"`
			AdmissionID string       `json:"admission_id"`
		}
		children := make([]child, 0, len(plan.recipients))
		for _, r := range plan.recipients {
			children = append(children, child{r.Destination, r.AdmissionID})
		}
		encoded, err := json.Marshal(children)
		if err != nil {
			return err
		}
		if err := i.admissions.ValidatePlan(ctx, plan); err != nil {
			return err
		}
		var company any
		if id, ok := e.CompanyID(); ok {
			company = id
		}
		result := i.tx.WithContext(ctx).Exec(`INSERT INTO eventstore.outbox_messages
 (event_id,source_owner,aggregate_type,aggregate_id,integration_sequence,envelope,envelope_hash,recipients,plan_authority_ref)
 SELECT event_id,owner_service,aggregate_type,aggregate_id,integration_sequence,?,?,?::jsonb,?
 FROM eventstore.events WHERE event_id=?::uuid AND owner_service=?
 AND aggregate_type=? AND aggregate_id=?::uuid AND aggregate_version=? AND integration_sequence=?
 AND event_type=? AND schema_version=? AND company_id IS NOT DISTINCT FROM ?::uuid
 AND occurred_at=? AND actor_kind=? AND actor_id=?::uuid
 AND correlation_id=?::uuid AND causation_id=?::uuid AND operation_id=?::uuid
 AND xmin=pg_current_xact_id()::xid`, m.body, m.hash[:], string(encoded), plan.authority,
			e.EventID(), string(i.owner), e.AggregateType(), e.AggregateID(), e.AggregateVersion().Int64(), e.IntegrationSequence().Int64(),
			e.EventType(), int64(e.SchemaVersion()), company, e.OccurredAt(), string(e.Actor().Kind), e.Actor().ID, e.CorrelationID(), e.CausationID(), e.OperationID())
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrEventNotFound
		}
		for _, r := range plan.recipients {
			child := i.tx.WithContext(ctx).Exec(`INSERT INTO eventstore.outbox_deliveries
 (event_id,destination,admission_id,exchange,routing_key) VALUES (?::uuid,?,?::uuid,?,?)`, e.EventID(), string(r.Destination), r.AdmissionID, IntegrationExchange, route(e, r.Destination))
			if child.Error != nil {
				return child.Error
			}
			if child.RowsAffected != 1 {
				return ErrInvalidRecord
			}
		}
	}
	return nil
}

func (i *Inserter) checkMode(ctx context.Context, mode string) error {
	if err := requireTransaction(i.tx); err != nil {
		return err
	}
	tx := i.tx.WithContext(ctx)
	var isolation string
	if err := tx.Raw("SHOW transaction_isolation").Scan(&isolation).Error; err != nil {
		return err
	}
	if isolation != "read committed" {
		return ErrMessagingMode
	}
	// Lock before reading mode so cutover cannot straddle even empty writes.
	// This is not a global lock-order promise: migrations stop runtimes, and
	// deadlocks/lock timeouts must abort the outer transaction without retry.
	table := "eventstore.outbox"
	if mode == "custody" {
		table = "eventstore.outbox_messages"
	}
	if err := tx.Exec("LOCK TABLE " + table + " IN ROW EXCLUSIVE MODE").Error; err != nil {
		return err
	}
	var state struct {
		Installed bool
		Marker    string
	}
	if err := tx.Raw(`SELECT to_regclass('eventstore.messaging_mode') IS NOT NULL AS installed,
 coalesce(current_setting('justix.messaging_mode',true),'') AS marker`).Scan(&state).Error; err != nil {
		return err
	}
	if state.Marker != "" && state.Marker != mode {
		return ErrMessagingMode
	}
	if !state.Installed {
		if mode == "legacy" {
			return nil
		} // Explicit original T-008 compatibility.
		return ErrMessagingMode
	}
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM eventstore.messaging_mode WHERE singleton AND mode=?
 AND schema_version=2 AND owner_service=? AND runtime_role::text=current_user`, mode, string(i.owner)).Scan(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrMessagingMode
	}
	if mode == "legacy" {
		return nil
	}
	if err := tx.Raw(`SELECT count(*) FROM eventstore.messaging_route_compatibility WHERE singleton
 AND owner_service=? AND runtime_role::text=current_user AND base_schema_version=2
 AND migration_revision=3 AND route_format_version=1
 AND prior_migration_sha256=decode('1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2','hex')
 AND correction_migration_sha256=decode('c198af498e39056a7a251b5906d6db050ad994e04588bf6cecf8316221ea85ec','hex')
 AND length(btrim(backup_ref))>0 AND length(btrim(stopped_runtimes_ref))>0
 AND length(btrim(compatibility_ref))>0 AND isfinite(installed_at)`, string(i.owner)).Scan(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrMessagingMode
	}
	return tx.Exec("SET LOCAL justix.messaging_mode='custody'").Error
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
	if i.admissions != nil {
		return ErrMessagingMode
	}
	if err := i.checkMode(ctx, "legacy"); err != nil {
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
