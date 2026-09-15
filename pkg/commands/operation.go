package commands

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"justixauto/pkg/events"
)

var (
	ErrOperationNotFound = errors.New("operation not found in authorized scope")
	ErrInvalidOperation  = errors.New("invalid operation or owner policy")
	ErrOperationConflict = errors.New("operation revision, decision or step conflict")
	ErrLeaseLost         = errors.New("operation scheduling lease lost")
)

type Decision string

const (
	Undecided Decision = "undecided"
	Commit    Decision = "commit"
	Abort     Decision = "abort"
)

// Operation is an authoritative owner snapshot, not a public response DTO.
// Result/error must be owner-declared safe DTOs. Transport adapters expose only
// the approved status fields after Status has reauthorized the reader.
type Operation[R, E any] struct {
	ID, Kind, AggregateID, ActorID, CompanyID, AdmissionRef string
	IntentRevision, Revision                                events.Revision
	Phase                                                   string
	Decision                                                Decision
	AttentionRequired                                       bool
	Result                                                  *R
	Error                                                   *E
	Attempts                                                int64
	NextAttemptAt, CreatedAt, UpdatedAt                     time.Time
}

// OperationPolicy contains no default business phases or outcome decisions.
// Phases maps each approved phase to whether it is terminal. Transition must
// validate the exact owner protocol, including result/decision combinations.
// Initial must validate the admitted initial phase and intent. These functions
// must be pure; live guards belong in the same transaction's owner ports.
type OperationPolicy[I, R, E any] struct {
	Schema        Schema[I, R]
	Phases        map[string]bool
	Initial       func(Operation[R, E]) error
	Transition    func(Operation[R, E], Operation[R, E]) error
	ValidateError func(E) error
}

// Operations is an adapter bound inside T-009's typed UnitOfWork factory to the
// owner's authoritative database. Do not let it escape the callback. Every
// method error MUST abort that transaction; candidates are not commit receipts.
// Owner commands/subscribers enforce live guards and bind events/inbox/outbox
// to this same transaction. This package exposes no database getter.
type Operations[I, R, E any] struct {
	tx     *gorm.DB
	policy OperationPolicy[I, R, E]
}

func NewOperations[I, R, E any](tx *gorm.DB, p OperationPolicy[I, R, E]) (*Operations[I, R, E], error) {
	if err := transactionRequired(tx); err != nil {
		return nil, err
	}
	if !p.Schema.valid() || len(p.Schema.hmacKey) > 0 || len(p.Phases) == 0 || p.Initial == nil || p.Transition == nil || p.ValidateError == nil {
		return nil, ErrInvalidOperation
	}
	phases := make(map[string]bool, len(p.Phases))
	for phase, terminal := range p.Phases {
		if strings.TrimSpace(phase) == "" || phase != strings.TrimSpace(phase) {
			return nil, ErrInvalidOperation
		}
		phases[phase] = terminal
	}
	p.Phases = phases
	return &Operations[I, R, E]{tx, p}, nil
}

type OperationIntent struct {
	ID, AggregateID, ActorID, CompanyID, AdmissionRef string
	IntentRevision                                    events.Revision
	Phase                                             string
}

// Create and the initiating event/guard/receipt/outbox must use the same UnitOfWork.
// A duplicate ID is an error: initial command retry belongs to T-017's ledger.
func (s *Operations[I, R, E]) Create(ctx context.Context, intent OperationIntent, input I) error {
	if s == nil {
		return ErrInvalidOperation
	}
	if !validID(intent.ID) || !validID(intent.AggregateID) || !validID(intent.ActorID) || !validID(intent.AdmissionRef) || !(validID(intent.CompanyID) || (intent.CompanyID == "" && s.policy.Schema.global)) {
		return ErrInvalidOperation
	}
	o := Operation[R, E]{ID: intent.ID, Kind: s.policy.Schema.command, AggregateID: intent.AggregateID, ActorID: intent.ActorID, CompanyID: intent.CompanyID, AdmissionRef: intent.AdmissionRef, IntentRevision: intent.IntentRevision, Phase: intent.Phase, Decision: Undecided}
	terminal, ok := s.policy.Phases[o.Phase]
	if !ok || terminal || s.policy.Initial(o) != nil {
		return ErrInvalidOperation
	}
	hash, err := s.policy.Schema.digest(input)
	if err != nil {
		return err
	}
	return s.tx.WithContext(ctx).Exec(`INSERT INTO eventstore.operations(operation_id,kind,aggregate_id,actor_id,company_id,admission_ref,intent_revision,request_hash,phase,revision) VALUES (?,?,?,?,?::uuid,?,?,?,?,1)`, o.ID, o.Kind, o.AggregateID, o.ActorID, nullable(o.CompanyID), o.AdmissionRef, o.IntentRevision.Int64(), hash[:], o.Phase).Error
}

type operationRow struct {
	OperationID, Kind, AggregateID, ActorID, CompanyID, AdmissionRef string
	IntentRevision, Revision                                         int64
	Phase                                                            string
	Decision                                                         Decision
	AttentionRequired                                                bool
	Result, Error                                                    []byte
	Attempts                                                         int64
	NextAttemptAt, CreatedAt, UpdatedAt                              time.Time
}

func (s *Operations[I, R, E]) decode(row operationRow) (Operation[R, E], error) {
	var o Operation[R, E]
	revision, e1 := events.NewRevision(row.Revision)
	intent, e2 := events.NewRevision(row.IntentRevision)
	_, phaseOK := s.policy.Phases[row.Phase]
	if e1 != nil || e2 != nil || row.Revision < 1 || !phaseOK || row.Kind != s.policy.Schema.command || !validID(row.OperationID) || !validID(row.ActorID) || !validID(row.AggregateID) || !validID(row.AdmissionRef) || !(validID(row.CompanyID) || (row.CompanyID == "" && s.policy.Schema.global)) || !validDecision(row.Decision) || row.Attempts < 0 {
		return o, ErrInvalidOperation
	}
	o = Operation[R, E]{ID: row.OperationID, Kind: row.Kind, AggregateID: row.AggregateID, ActorID: row.ActorID, CompanyID: row.CompanyID, AdmissionRef: row.AdmissionRef, IntentRevision: intent, Revision: revision, Phase: row.Phase, Decision: row.Decision, AttentionRequired: row.AttentionRequired, Attempts: row.Attempts, NextAttemptAt: row.NextAttemptAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if len(row.Result) > 0 {
		var r R
		if !safeObject(row.Result) || strictDecode(row.Result, &r) != nil || s.policy.Schema.validate(r) != nil {
			return Operation[R, E]{}, ErrInvalidReceipt
		}
		o.Result = &r
	}
	if len(row.Error) > 0 {
		var e E
		if !safeObject(row.Error) || strictDecode(row.Error, &e) != nil || s.policy.ValidateError(e) != nil {
			return Operation[R, E]{}, ErrInvalidReceipt
		}
		o.Error = &e
	}
	return o, nil
}
func validDecision(d Decision) bool { return d == Undecided || d == Commit || d == Abort }

func (s *Operations[I, R, E]) load(ctx context.Context, id string, lock bool) (Operation[R, E], error) {
	if s == nil || !validID(id) {
		return Operation[R, E]{}, ErrInvalidOperation
	}
	query := `SELECT * FROM eventstore.operations WHERE operation_id=? AND kind=?`
	if lock {
		query += " FOR UPDATE"
	}
	var row operationRow
	r := s.tx.WithContext(ctx).Raw(query, id, s.policy.Schema.command).Scan(&row)
	if r.Error != nil {
		return Operation[R, E]{}, r.Error
	}
	if r.RowsAffected != 1 {
		return Operation[R, E]{}, ErrOperationNotFound
	}
	return s.decode(row)
}

// OperationAccess is freshly authenticated scope. Authorize is mandatory and
// must check current actor/action/branch/party and manager rights, not body flags.
// Company equality is enforced in SQL before exposing any operation to it.
type OperationAccess struct{ ActorID, CompanyID string }
type OperationAction string

const (
	InspectOperation OperationAction = "inspect"
	ResumeOperation  OperationAction = "resume"
)

func (s *Operations[I, R, E]) scoped(ctx context.Context, id string, a OperationAccess, action OperationAction, authorize func(context.Context, OperationAccess, OperationAction, Operation[R, E]) error, lock bool) (Operation[R, E], error) {
	if s == nil || !validID(id) || !validID(a.ActorID) || !(validID(a.CompanyID) || (a.CompanyID == "" && s.policy.Schema.global)) || authorize == nil {
		return Operation[R, E]{}, ErrOperationNotFound
	}
	query := `SELECT * FROM eventstore.operations WHERE operation_id=? AND kind=? AND company_id IS NOT DISTINCT FROM ?::uuid`
	if lock {
		query += " FOR UPDATE"
	}
	var row operationRow
	r := s.tx.WithContext(ctx).Raw(query, id, s.policy.Schema.command, nullable(a.CompanyID)).Scan(&row)
	if r.Error != nil {
		return Operation[R, E]{}, r.Error
	}
	if r.RowsAffected != 1 {
		return Operation[R, E]{}, ErrOperationNotFound
	}
	o, err := s.decode(row)
	if err != nil {
		return Operation[R, E]{}, err
	}
	if err = authorize(ctx, a, action, o); err != nil {
		return Operation[R, E]{}, err
	}
	return o, nil
}

// Status reads the authoritative owner database. Missing does not prove an
// in-flight creation rolled back. This transaction must not use a read replica.
func (s *Operations[I, R, E]) Status(ctx context.Context, id string, a OperationAccess, authorize func(context.Context, OperationAccess, OperationAction, Operation[R, E]) error) (Operation[R, E], error) {
	return s.scoped(ctx, id, a, InspectOperation, authorize, false)
}

// Resume only makes the existing nonterminal operation due. It neither revokes
// an active lease nor alters phase, decision, attempts, receipts or attention.
func (s *Operations[I, R, E]) Resume(ctx context.Context, id string, a OperationAccess, authorize func(context.Context, OperationAccess, OperationAction, Operation[R, E]) error) error {
	o, err := s.scoped(ctx, id, a, ResumeOperation, authorize, true)
	if err != nil {
		return err
	}
	if s.policy.Phases[o.Phase] {
		return ErrOperationConflict
	}
	return s.tx.WithContext(ctx).Exec(`UPDATE eventstore.operations SET next_attempt_at=clock_timestamp(),revision=revision+1,updated_at=clock_timestamp() WHERE operation_id=?`, id).Error
}

type OperationChange[R, E any] struct {
	Phase    string
	Decision Decision
	Result   *R
	Error    *E
}

// Advance is a trusted owner command/subscriber port, NOT an administrative
// status setter. Expected revision, immutable decisions and mandatory owner
// transition policy fence it. Local effects must share this transaction.
func (s *Operations[I, R, E]) Advance(ctx context.Context, id string, expected events.Revision, c OperationChange[R, E]) error {
	old, err := s.load(ctx, id, true)
	if err != nil {
		return err
	}
	if old.Revision != expected || s.policy.Phases[old.Phase] || !validDecision(c.Decision) || (old.Decision != Undecided && old.Decision != c.Decision) {
		return ErrOperationConflict
	}
	if _, ok := s.policy.Phases[c.Phase]; !ok {
		return ErrInvalidOperation
	}
	rb, err := safeOptional(c.Result, s.policy.Schema.validate)
	if err != nil {
		return err
	}
	eb, err := safeOptional(c.Error, s.policy.ValidateError)
	if err != nil {
		return err
	}
	next := old
	next.Phase = c.Phase
	next.Decision = c.Decision
	next.Result = c.Result
	next.Error = c.Error
	if s.policy.Transition(old, next) != nil {
		return ErrOperationConflict
	}
	return s.tx.WithContext(ctx).Exec(`UPDATE eventstore.operations SET phase=?,decision=?,result=?::jsonb,error=?::jsonb,revision=revision+1,updated_at=clock_timestamp() WHERE operation_id=?`, c.Phase, c.Decision, rb, eb, id).Error
}
func safeOptional[T any](v *T, validate func(T) error) (any, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil || !safeObject(b) || validate(*v) != nil {
		return nil, ErrInvalidReceipt
	}
	return string(b), nil
}

// Steps binds a separate owner-declared nonsecret request/result schema to the
// same transaction. The kind binds it to the matching operation protocol.
type Steps[I, R any] struct {
	tx         *gorm.DB
	schema     Schema[I, R]
	kind, step string
}

func NewSteps[I, R any](tx *gorm.DB, s Schema[I, R], kind, step string) (*Steps[I, R], error) {
	if err := transactionRequired(tx); err != nil {
		return nil, err
	}
	if !s.valid() || len(s.hmacKey) > 0 || !strings.HasPrefix(kind, string(s.owner)+".") || strings.TrimSpace(step) == "" || step != strings.TrimSpace(step) {
		return nil, ErrInvalidOperation
	}
	return &Steps[I, R]{tx, s, kind, step}, nil
}

// Execute records only a completed safe step result. A transport timeout is an
// error, never an invented business outcome. All errors abort the outer UOW;
// work is local only. Remote requests use this SAME operation ID and step.
func (s *Steps[I, R]) Execute(ctx context.Context, id string, input I, work func(context.Context) (R, error)) (R, bool, error) {
	var zero R
	if s == nil || !validID(id) || work == nil {
		return zero, false, ErrInvalidOperation
	}
	hash, err := s.schema.digest(input)
	if err != nil {
		return zero, false, err
	}
	var locked string
	q := s.tx.WithContext(ctx).Raw(`SELECT operation_id FROM eventstore.operations WHERE operation_id=? AND kind=? FOR UPDATE`, id, s.kind).Scan(&locked)
	if q.Error != nil {
		return zero, false, q.Error
	}
	if q.RowsAffected != 1 {
		return zero, false, ErrOperationNotFound
	}
	var row struct{ RequestHash, Result []byte }
	q = s.tx.WithContext(ctx).Raw(`SELECT request_hash,result FROM eventstore.operation_steps WHERE operation_id=? AND step=?`, id, s.step).Scan(&row)
	if q.Error != nil {
		return zero, false, q.Error
	}
	if q.RowsAffected == 1 {
		if !hmac.Equal(hash[:], row.RequestHash) {
			return zero, false, ErrConflict
		}
		if !safeObject(row.Result) || strictDecode(row.Result, &zero) != nil || s.schema.validate(zero) != nil {
			return zero, false, ErrCorruptReceipt
		}
		return zero, true, nil
	}
	result, err := work(ctx)
	if err != nil {
		return zero, false, err
	}
	b, err := safeOptional(&result, s.schema.validate)
	if err != nil {
		return zero, false, err
	}
	err = s.tx.WithContext(ctx).Exec(`INSERT INTO eventstore.operation_steps(operation_id,step,request_hash,result) VALUES (?,?,?,?::jsonb)`, id, s.step, hash[:], b).Error
	if err != nil {
		return zero, false, err
	}
	return result, false, nil
}
