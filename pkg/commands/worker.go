package commands

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
)

// OperationClaim is a scheduling fence. A worker may continue a remote request
// after its lease expires; the remote owner MUST deduplicate operation ID+step.
// Losing a lease never cancels or reverses a durable business decision.
type OperationClaim[R, E any] struct {
	Operation    Operation[R, E]
	LeaseID      string
	LeaseUntil   time.Time
	AttentionNew bool
}

// MarkAttention is the bounded recovery scanner for old nonterminal operations,
// including those with live leases or future retry times. The composition root
// must run this periodically alongside Claim; a lease cannot suppress alerts.
// The required callback persists a local alert/outbox intent in this SAME UOW.
// All rows and intents roll back together on error; transport is downstream.
func (s *Operations[I, R, E]) MarkAttention(ctx context.Context, limit int, alert func(context.Context, Operation[R, E]) error) (int, error) {
	if s == nil || limit < 1 || alert == nil {
		return 0, ErrInvalidOperation
	}
	phases := []string{}
	for phase, terminal := range s.policy.Phases {
		if !terminal {
			phases = append(phases, phase)
		}
	}
	sort.Strings(phases)
	if len(phases) == 0 {
		return 0, nil
	}
	var rows []operationRow
	err := s.tx.WithContext(ctx).Raw(`SELECT * FROM eventstore.operations WHERE kind=? AND phase IN ? AND attention_required=false AND created_at<clock_timestamp()-interval '5 minutes' ORDER BY created_at,operation_id FOR UPDATE SKIP LOCKED LIMIT ?`, s.policy.Schema.command, phases, limit).Scan(&rows).Error
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		if _, err = s.decode(row); err != nil {
			return 0, err
		}
		err = s.tx.WithContext(ctx).Exec(`UPDATE eventstore.operations SET attention_required=true,revision=revision+1,updated_at=clock_timestamp() WHERE operation_id=?`, row.OperationID).Error
		if err != nil {
			return 0, err
		}
		o, err := s.load(ctx, row.OperationID, false)
		if err != nil {
			return 0, err
		}
		if err = alert(ctx, o); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

// Claim scans only this registered kind's owner-declared nonterminal phases.
// SKIP LOCKED permits independent workers. Each attempt uses a new fence token,
// including reuse by the same worker process. Time comes from PostgreSQL.
// The returned claim is unusable until the caller's transaction commits. On
// unknown commit, do not dispatch it; reconcile the same operation after lease
// expiry. alert is a REQUIRED same-transaction local outbox/alert intent port;
// it runs once when crossing five minutes. No network publication in this port.
func (s *Operations[I, R, E]) Claim(ctx context.Context, lease time.Duration, alert func(context.Context, Operation[R, E]) error) (OperationClaim[R, E], bool, error) {
	var zero OperationClaim[R, E]
	if s == nil || lease < time.Millisecond || alert == nil {
		return zero, false, ErrInvalidOperation
	}
	phases := []string{}
	for phase, terminal := range s.policy.Phases {
		if !terminal {
			phases = append(phases, phase)
		}
	}
	sort.Strings(phases)
	if len(phases) == 0 {
		return zero, false, nil
	}
	var row operationRow
	q := s.tx.WithContext(ctx).Raw(`SELECT * FROM eventstore.operations WHERE kind=? AND phase IN ? AND next_attempt_at<=clock_timestamp() AND (lease_until IS NULL OR lease_until<=clock_timestamp()) ORDER BY next_attempt_at,operation_id FOR UPDATE SKIP LOCKED LIMIT 1`, s.policy.Schema.command, phases).Scan(&row)
	if q.Error != nil {
		return zero, false, q.Error
	}
	if q.RowsAffected == 0 {
		return zero, false, nil
	}
	before, err := s.decode(row)
	if err != nil {
		return zero, false, err
	}
	id := uuid.NewString()
	var timing struct {
		LeaseUntil        time.Time
		AttentionRequired bool
	}
	q = s.tx.WithContext(ctx).Raw(`UPDATE eventstore.operations SET lease_owner=?,lease_until=clock_timestamp()+(? * interval '1 microsecond'),attempts=attempts+1,attention_required=attention_required OR created_at<clock_timestamp()-interval '5 minutes',revision=revision+1,updated_at=clock_timestamp() WHERE operation_id=? RETURNING lease_until,attention_required`, id, lease.Microseconds(), before.ID).Scan(&timing)
	if q.Error != nil {
		return zero, false, q.Error
	}
	after, err := s.load(ctx, before.ID, false)
	if err != nil {
		return zero, false, err
	}
	fresh := !before.AttentionRequired && timing.AttentionRequired
	if fresh {
		if err = alert(ctx, after); err != nil {
			return zero, false, err
		}
	}
	return OperationClaim[R, E]{after, id, timing.LeaseUntil, fresh}, true, nil
}

// WithClaim locks and checks the current fence before any local effect. work
// must use only same-transaction owner ports, and errors abort the complete UOW.
// The caller supplies typed ports via T-009's factory closure; no SQL handle is
// given to application work. Long remote calls belong BEFORE this local callback.
// A worker result becomes authoritative only after the enclosing commit succeeds.
func (s *Operations[I, R, E]) WithClaim(ctx context.Context, claim OperationClaim[R, E], work func(context.Context, Operation[R, E]) error) error {
	if work == nil {
		return ErrInvalidOperation
	}
	o, err := s.lockClaim(ctx, claim)
	if err != nil {
		return err
	}
	if err = work(ctx, o); err != nil {
		return err
	}
	// Local callback owns the phase/decision through Advance. Release only the
	// same fence. A nonterminal operation remains due for its next stable step.
	return s.tx.WithContext(ctx).Exec(`UPDATE eventstore.operations SET lease_owner=NULL,lease_until=NULL,next_attempt_at=clock_timestamp(),revision=revision+1,updated_at=clock_timestamp() WHERE operation_id=? AND lease_owner=?`, o.ID, claim.LeaseID).Error
}

func (s *Operations[I, R, E]) lockClaim(ctx context.Context, c OperationClaim[R, E]) (Operation[R, E], error) {
	if s == nil || !validID(c.LeaseID) || !validID(c.Operation.ID) {
		return Operation[R, E]{}, ErrLeaseLost
	}
	// Lock first, then inspect time in a fresh statement: waiting for a concurrent
	// transaction must not permit a token whose lease expired while waiting.
	o, err := s.load(ctx, c.Operation.ID, true)
	if err != nil {
		return Operation[R, E]{}, err
	}
	var live bool
	// Released leases have NULL owner/until. Classify that normal recovery state
	// as a lost fence, while preserving actual query/scan errors below.
	err = s.tx.WithContext(ctx).Raw(`SELECT COALESCE(lease_owner=?::uuid AND lease_until>clock_timestamp(),false) FROM eventstore.operations WHERE operation_id=?`, c.LeaseID, o.ID).Scan(&live).Error
	if err != nil {
		return Operation[R, E]{}, err
	}
	if !live || s.policy.Phases[o.Phase] {
		return Operation[R, E]{}, ErrLeaseLost
	}
	return o, nil
}

// RetryDelay is capped exponential backoff with bounded jitter. sample is a
// caller-supplied random value in [0,1]; inject deterministic values in tests.
// First retry is one second; every delay remains in [1s,60s], even at bigint max.
func RetryDelay(attempt int64, sample float64) (time.Duration, error) {
	if attempt < 1 || math.IsNaN(sample) || math.IsInf(sample, 0) || sample < 0 || sample > 1 {
		return 0, ErrInvalidOperation
	}
	capSeconds := int64(60)
	if attempt <= 6 {
		capSeconds = 1 << uint(attempt-1)
	}
	low := float64(capSeconds) / 2
	if low < 1 {
		low = 1
	}
	return time.Duration((low + (float64(capSeconds)-low)*sample) * float64(time.Second)), nil
}

// Retry retains phase/decision/result/error and marks only scheduling metadata.
// Never persist arbitrary transport errors (which can contain secrets/PII) as
// business failure. An unavailable owner remains pending, with attention.
func (s *Operations[I, R, E]) Retry(ctx context.Context, c OperationClaim[R, E], sample float64, alert func(context.Context, Operation[R, E]) error) error {
	if alert == nil {
		return ErrInvalidOperation
	}
	o, err := s.lockClaim(ctx, c)
	if err != nil {
		return err
	}
	delay, err := RetryDelay(o.Attempts, sample)
	if err != nil {
		return err
	}
	err = s.tx.WithContext(ctx).Exec(`UPDATE eventstore.operations SET next_attempt_at=clock_timestamp()+(? * interval '1 microsecond'),lease_owner=NULL,lease_until=NULL,attention_required=attention_required OR created_at<clock_timestamp()-interval '5 minutes',revision=revision+1,updated_at=clock_timestamp() WHERE operation_id=?`, delay.Microseconds(), o.ID).Error
	if err != nil {
		return err
	}
	after, err := s.load(ctx, o.ID, false)
	if err != nil {
		return err
	}
	if !o.AttentionRequired && after.AttentionRequired {
		return alert(ctx, after)
	}
	return nil
}
