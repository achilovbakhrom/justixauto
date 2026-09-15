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
)

// Outcome is authoritative only for the local database transaction. Neither
// Applied nor Duplicate promises broker delivery or acknowledgment durability.
type Outcome uint8

const (
	Unconfirmed Outcome = iota // No confirmed commit; includes unknown outcomes.
	Applied
	Duplicate
)

type bound[U any] struct {
	tx    *gorm.DB
	ports U
}

// Consumer atomically records deduplication with the owner's typed local effect
// and checkpoint. A generation-specific consumer name provides a distinct inbox
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
	// Retain no caller-owned bytes across validation or the transaction callback.
	body = bytes.Clone(body)
	var envelope events.Envelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Unconfirmed, fmt.Errorf("inbox: decode envelope: %w", err)
	}
	if err := c.validate(envelope); err != nil {
		return Unconfirmed, err
	}
	hash := sha256.Sum256(body)
	outcome := Applied
	err := c.runner.Run(ctx, func(u bound[U]) error {
		insert := u.tx.WithContext(ctx).Exec(`INSERT INTO eventstore.inbox (consumer_name,event_id,envelope_hash)
			VALUES (?,?::uuid,?) ON CONFLICT (consumer_name,event_id) DO NOTHING`, c.name, envelope.EventID(), hash[:])
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 {
			// ReadCommitted gives this next statement a fresh snapshot after the
			// unique-key insert waited for any concurrent winner to commit.
			var stored struct{ EnvelopeHash []byte }
			read := u.tx.WithContext(ctx).Raw(`SELECT envelope_hash FROM eventstore.inbox
				WHERE consumer_name=? AND event_id=?::uuid`, c.name, envelope.EventID()).Scan(&stored)
			if read.Error != nil {
				return read.Error
			}
			if read.RowsAffected != 1 || !bytes.Equal(stored.EnvelopeHash, hash[:]) {
				return ErrEnvelopeConflict
			}
			outcome = Duplicate
			return nil
		}
		return apply(u.ports, envelope)
	})
	if err != nil {
		return Unconfirmed, err
	}
	if err := ack(); err != nil {
		return outcome, errors.Join(ErrAcknowledgment, err)
	}
	return outcome, nil
}
