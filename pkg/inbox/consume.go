// Package inbox provides owner-local subscriber persistence adapters. Service
// domain, app and port packages must not import this infrastructure package.
package inbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
)

var (
	ErrInvalidConsumer  = errors.New("inbox: consumer, schema validator, typed factory and acknowledgment are required")
	ErrEnvelopeConflict = errors.New("inbox: event ID was previously committed with different envelope bytes")
	ErrAcknowledgment   = errors.New("inbox: effect committed but acknowledgment failed")
	ErrMessagingMode    = errors.New("inbox: incompatible messaging mode")
	ErrIsolation        = errors.New("inbox: read committed transaction required")
)

// Outcome is authoritative only for the local database transaction. Neither
// Applied nor Duplicate promises broker delivery or acknowledgment durability.
type Outcome uint8

const (
	Unconfirmed Outcome = iota // No confirmed commit; includes unknown outcomes.
	Applied
	Duplicate
)

// Candidate describes work in the caller's still-open transaction. It is NOT
// an Outcome or a committed receipt. Discard it on rollback or unknown commit;
// reconcile durable inbox/job state before reporting completion.
type Candidate uint8

const (
	NoCandidate Candidate = iota
	AppliedCandidate
	DuplicateCandidate
)

// TransactionalConsumer is the custody executor's transaction-bound adapter.
// It has no transaction runner, commit method, or broker acknowledgment hook.
// Its typed ports and this adapter must not escape the caller's transaction.
type TransactionalConsumer[U any] struct {
	bound[U]
	name     string
	validate func(events.Envelope) error
}

// NewTransactionalConsumer binds an owner's typed effect/checkpoint ports to
// the supplied transaction. The outer adapter also binds the job repository to
// that same transaction. bind must not capture another database or perform I/O
// outside local SQL. The caller must propagate factory and Apply errors to Run.
// A plain connection pool is rejected; Apply additionally checks isolation and
// the installed custody mode. Neither this constructor nor Apply grants stream,
// consumer, scope or job-lease authority; the dispatcher must verify those.
func NewTransactionalConsumer[U any](tx *gorm.DB, name string, bind func(*gorm.DB) (U, error), validate func(events.Envelope) error) (*TransactionalConsumer[U], error) {
	if strings.TrimSpace(name) == "" || bind == nil || validate == nil {
		return nil, ErrInvalidConsumer
	}
	if err := requireTransaction(tx); err != nil {
		return nil, err
	}
	ports, err := bind(tx)
	if err != nil {
		return nil, err
	}
	return &TransactionalConsumer[U]{bound: bound[U]{tx: tx, ports: ports}, name: name, validate: validate}, nil
}

// Apply validates before every inbox lookup, including exact duplicates. Fresh
// bytes record inbox + the typed local effect/checkpoint; identical prior bytes
// return DuplicateCandidate without reapplying. The caller must complete its
// own pending job for EITHER candidate in this same transaction, including a
// migrated job whose inbox already committed in legacy mode. A failed final
// lease-conditioned completion must roll back the entire outer transaction.
// Never ACK, perform external effects, nest Run, or commit inside apply.
func (c *TransactionalConsumer[U]) Apply(ctx context.Context, body []byte, apply func(U, events.Envelope) error) (Candidate, error) {
	if c == nil || c.validate == nil || apply == nil {
		return NoCandidate, ErrInvalidConsumer
	}
	e, hash, err := prepare(body, c.validate)
	if err != nil {
		return NoCandidate, err
	}
	return applyTransaction(ctx, c.bound, c.name, e, hash, "custody", apply)
}

type bound[U any] struct {
	tx    *gorm.DB
	ports U
}

// Consumer atomically records deduplication with the owner's typed local effect
// and checkpoint in legacy, explicitly single-consumer direct-ACK mode. Calling
// Consume asserts that composition; it is rejected after custody cutover. A
// generation-specific consumer name provides a distinct inbox
// namespace. It is not an authorization or ordering mechanism.
type Consumer[U any] struct {
	name     string
	runner   *eventstore.Transactions[bound[U]]
	validate func(events.Envelope) error
}

// NewConsumer binds only transaction-local adapters. bind must not perform
// network effects, capture another database, or allow its ports to escape the
// callback. validate must enforce the subscribed allowlisted payload schemas
// (events.DecodeData), source and permitted scope before any inbox lookup,
// including redelivery. JSON structural validation alone is insufficient.
func NewConsumer[U any](db *gorm.DB, name string, bind func(*gorm.DB) (U, error), validate func(events.Envelope) error) (*Consumer[U], error) {
	if strings.TrimSpace(name) == "" || bind == nil || validate == nil {
		return nil, ErrInvalidConsumer
	}
	runner, err := eventstore.NewTransactions(db, func(tx *gorm.DB) (bound[U], error) {
		ports, err := bind(tx)
		return bound[U]{tx: tx, ports: ports}, err
	})
	if err != nil {
		return nil, err
	}
	return &Consumer[U]{name: name, runner: runner, validate: validate}, nil
}

// Consume hashes the exact received envelope bytes, validates their metadata
// and schema, and commits inbox + effect + checkpoint in one transaction. apply
// must enforce sequence/scope invariants using the supplied typed local ports;
// a gap or failed effect must return an error, so the inbox insert rolls back.
// An irrelevant but authorized event may no-op its effect and still checkpoint.
// Do not call a broker or any external service inside apply.
//
// ack must acknowledge only this delivery (AMQP Ack(false)), never all earlier
// deliveries. It runs only after a confirmed commit, also for exact duplicates.
// Any database/validation/conflicting-envelope error leaves it unacknowledged.
// No retry, NACK, gap recovery or quarantine policy is invented here. The caller
// must durably quarantine invalid/unsupported events before acknowledging them.
// A lost commit reply returns Unconfirmed and preserves ErrCommitOutcomeUnknown;
// redelivery resolves it using the durable inbox without repeating the effect.
func (c *Consumer[U]) Consume(ctx context.Context, body []byte, apply func(U, events.Envelope) error, ack func() error) (Outcome, error) {
	if c == nil || c.runner == nil || c.validate == nil || apply == nil || ack == nil {
		return Unconfirmed, ErrInvalidConsumer
	}
	envelope, hash, err := prepare(body, c.validate)
	if err != nil {
		return Unconfirmed, err
	}
	var candidate Candidate
	err = c.runner.Run(ctx, func(u bound[U]) error {
		var err error
		candidate, err = applyTransaction(ctx, u, c.name, envelope, hash, "legacy", apply)
		return err
	})
	if err != nil {
		return Unconfirmed, err
	}
	outcome := Applied
	if candidate == DuplicateCandidate {
		outcome = Duplicate
	}
	if err := ack(); err != nil {
		return outcome, errors.Join(ErrAcknowledgment, err)
	}
	return outcome, nil
}

func prepare(body []byte, validate func(events.Envelope) error) (events.Envelope, [32]byte, error) {
	// Retain no caller-owned bytes across validation or transaction callbacks.
	body = bytes.Clone(body)
	var envelope events.Envelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return envelope, [32]byte{}, fmt.Errorf("inbox: decode envelope: %w", err)
	}
	if err := validate(envelope); err != nil {
		return envelope, [32]byte{}, err
	}
	return envelope, sha256.Sum256(body), nil
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

func checkMode(ctx context.Context, tx *gorm.DB, mode string) error {
	if err := requireTransaction(tx); err != nil {
		return err
	}
	tx = tx.WithContext(ctx)
	var isolation string
	if err := tx.Raw("SHOW transaction_isolation").Scan(&isolation).Error; err != nil {
		return err
	}
	if isolation != "read committed" {
		return ErrIsolation
	}
	// Take the inbox table lock BEFORE reading mode. Cutover cannot straddle
	// even a duplicate fast path. This is not a global lock-order guarantee:
	// cutover locks outbox first, and owner effects/dispatch may hold other locks.
	// Stop old runtimes before migration; propagate deadlocks/timeouts to Run.
	// ROW EXCLUSIVE is compatible between consumers and needs only INSERT rights;
	// the runtime has SELECT only on messaging_mode, so cannot SELECT FOR SHARE.
	if err := tx.Exec("LOCK TABLE eventstore.inbox IN ROW EXCLUSIVE MODE").Error; err != nil {
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
		if mode != "legacy" {
			return ErrMessagingMode
		}
		return nil // Explicit compatibility with the original T-008 installation.
	}
	var active struct {
		Mode          string
		SchemaVersion int
	}
	read := tx.Raw("SELECT mode,schema_version FROM eventstore.messaging_mode WHERE singleton").Scan(&active)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 || active.Mode != mode || active.SchemaVersion != 2 {
		return ErrMessagingMode
	}
	if mode == "custody" {
		// Compatibility marker only, never an authorization credential. LOCAL
		// ensures connection pooling cannot leak this mode into the next transaction.
		return tx.Exec("SET LOCAL justix.messaging_mode='custody'").Error
	}
	return nil
}

func applyTransaction[U any](ctx context.Context, u bound[U], name string, envelope events.Envelope, hash [32]byte, mode string, apply func(U, events.Envelope) error) (Candidate, error) {
	if err := checkMode(ctx, u.tx, mode); err != nil {
		return NoCandidate, err
	}
	insert := u.tx.WithContext(ctx).Exec(`INSERT INTO eventstore.inbox (consumer_name,event_id,envelope_hash)
		VALUES (?,?::uuid,?) ON CONFLICT (consumer_name,event_id) DO NOTHING`, name, envelope.EventID(), hash[:])
	if insert.Error != nil {
		return NoCandidate, insert.Error
	}
	if insert.RowsAffected == 0 {
		// ReadCommitted gives this next statement a fresh snapshot after the
		// unique-key insert waited for any concurrent winner to commit.
		var stored struct{ EnvelopeHash []byte }
		read := u.tx.WithContext(ctx).Raw(`SELECT envelope_hash FROM eventstore.inbox
			WHERE consumer_name=? AND event_id=?::uuid`, name, envelope.EventID()).Scan(&stored)
		if read.Error != nil {
			return NoCandidate, read.Error
		}
		if read.RowsAffected != 1 || !bytes.Equal(stored.EnvelopeHash, hash[:]) {
			return NoCandidate, ErrEnvelopeConflict
		}
		return DuplicateCandidate, nil
	}
	if err := apply(u.ports, envelope); err != nil {
		return NoCandidate, err
	}
	return AppliedCandidate, nil
}
