package commands

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
)

// Scope comes from freshly authorized owner context, never untrusted body
// actor/company fields. Empty CompanyID is global; empty TargetID is a create.
type Scope struct{ ActorID, CompanyID, TargetID, Key string }

// Schema binds one admitted business command to typed normalization and a safe
// outcome validator. Normalization must retain all effect-affecting nonsecret
// input (including preconditions), normalize unordered sets/labels as the owner
// contract requires, and omit credentials. It is not a raw HTTP body decoder.
// Authentication handshakes must never register a business command schema.
type Schema[I, O any] struct {
	owner     events.Owner
	command   string
	global    bool
	normalize func(I) (I, error)
	validate  func(O) error
	hmacKey   []byte
}

func NewSchema[I, O any](owner events.Owner, command string, global bool, normalize func(I) (I, error), validate func(O) error) (Schema[I, O], error) {
	s := Schema[I, O]{owner: owner, command: command, global: global, normalize: normalize, validate: validate}
	if !s.valid() {
		return Schema[I, O]{}, ErrInvalidCommand
	}
	return s, nil
}

// NewSecretSafeSchema is for identity's authenticated provisioning commands.
// Only a keyed digest of normalized NONSECRET input is stored. Password checks
// happen through Execute's required private verifier and never enter this type.
// Key retention/rotation belongs to the owner configuration adapter; this
// package cannot infer a key version from the approved receipt table.
func NewSecretSafeSchema[I, O any](command string, global bool, key []byte, normalize func(I) (I, error), validate func(O) error) (Schema[I, O], error) {
	s, err := NewSchema(events.OwnerIdentity, command, global, normalize, validate)
	if err != nil || len(key) < sha256.Size {
		return Schema[I, O]{}, ErrInvalidCommand
	}
	s.hmacKey = append([]byte(nil), key...)
	return s, nil
}

func (s Schema[I, O]) valid() bool {
	return s.owner.Valid() && strings.HasPrefix(s.command, string(s.owner)+".") && len(s.command) > len(s.owner)+1 && strings.TrimSpace(s.command) == s.command && (!s.global || s.owner == events.OwnerIdentity) && s.normalize != nil && s.validate != nil
}

func (s Schema[I, O]) scopeOK(scope Scope) bool {
	return s.valid() && validID(scope.ActorID) && validID(scope.Key) && (validID(scope.CompanyID) || (scope.CompanyID == "" && s.global)) && (scope.TargetID == "" || validID(scope.TargetID))
}

// Ledger is transaction-bound. Construct it inside an adapter UnitOfWork
// factory, alongside the same transaction's event, guard and outbox ports.
type Ledger[I, O any] struct {
	tx     *gorm.DB
	schema Schema[I, O]
}

func NewLedger[I, O any](tx *gorm.DB, schema Schema[I, O]) (*Ledger[I, O], error) {
	if err := transactionRequired(tx); err != nil {
		return nil, err
	}
	if !schema.valid() {
		return nil, ErrInvalidCommand
	}
	return &Ledger[I, O]{tx, schema}, nil
}

func transactionRequired(tx *gorm.DB) error {
	if tx == nil || tx.Error != nil || tx.Statement == nil {
		return eventstore.ErrTransactionRequired
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return eventstore.ErrTransactionRequired
	}
	return nil
}

// Execute serializes the complete scoped key, checks its authoritative receipt,
// then runs the local effect at most once for concurrent successful commits.
// All errors MUST abort the outer transaction. Do not escape repositories or
// publish/call another service in work. Reauthorize human requests before entry.
// verify is required for secret-safe schemas: it checks the supplied password
// against the receipt's created private credential, without resetting it. Its
// mismatch is intentionally generic 409. Recovery below needs no old password.
// A returned receipt must not be sent until the outer Run successfully commits.
func (l *Ledger[I, O]) Execute(ctx context.Context, scope Scope, input I, verify func(context.Context, Receipt[O]) (bool, error), work func(context.Context) (Receipt[O], error)) (Receipt[O], bool, error) {
	var zero Receipt[O]
	if l == nil {
		return zero, false, eventstore.ErrTransactionRequired
	}
	if err := transactionRequired(l.tx); err != nil {
		return zero, false, err
	}
	if !l.schema.scopeOK(scope) || work == nil || (len(l.schema.hmacKey) > 0 && verify == nil) {
		return zero, false, ErrInvalidCommand
	}
	digest, err := l.schema.digest(input)
	if err != nil {
		return zero, false, err
	}
	key, _ := json.Marshal([]string{string(l.schema.owner), scope.ActorID, scope.CompanyID, l.schema.command, scope.TargetID, scope.Key})
	lock := sha256.Sum256(key)
	if err := l.tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(?)", int64(binary.BigEndian.Uint64(lock[:8]))).Error; err != nil {
		return zero, false, err
	}
	row, found, err := lookup(ctx, l.tx, l.schema.command, scope)
	if err != nil {
		return zero, false, err
	}
	if found {
		if !hmac.Equal(row.RequestHash, digest[:]) {
			return zero, false, ErrConflict
		}
		r := Receipt[O]{row.HTTPStatus, append([]byte(nil), row.Receipt...)}
		if _, err := decodeReceipt(r, l.schema.validate); err != nil {
			return zero, false, ErrCorruptReceipt
		}
		if len(l.schema.hmacKey) > 0 {
			matches, err := verify(ctx, r)
			if err != nil {
				return zero, false, err
			}
			if !matches {
				return zero, false, ErrConflict
			}
		}
		return r, true, nil
	}
	r, err := work(ctx)
	if err != nil {
		return zero, false, err
	}
	if _, err := decodeReceipt(r, l.schema.validate); err != nil {
		return zero, false, ErrInvalidReceipt
	}
	err = l.tx.WithContext(ctx).Exec(`INSERT INTO eventstore.command_receipts
	(receipt_id,actor_id,company_id,command_name,target_id,idempotency_key,request_hash,http_status,receipt)
	VALUES (?, ?, ?::uuid, ?, ?::uuid, ?, ?, ?, ?::jsonb)`, uuid.NewString(), scope.ActorID, nullable(scope.CompanyID), l.schema.command, nullable(scope.TargetID), scope.Key, digest[:], r.status, string(r.body)).Error
	if err != nil {
		return zero, false, err
	}
	return r, false, nil
}

// Recover reads the owner's authoritative database after live actor/company/
// action authorization. found=false is NOT proof that an in-flight command
// failed; replay the same command/key or reconcile. Never use a projection or
// read replica for this API. It deliberately does not compare an old password.
func Recover[I, O any](ctx context.Context, db *gorm.DB, schema Schema[I, O], scope Scope) (Receipt[O], bool, error) {
	var zero Receipt[O]
	if db == nil || db.Error != nil || !schema.scopeOK(scope) {
		return zero, false, ErrInvalidCommand
	}
	row, found, err := lookup(ctx, db, schema.command, scope)
	if err != nil || !found {
		return zero, found, err
	}
	r := Receipt[O]{row.HTTPStatus, append([]byte(nil), row.Receipt...)}
	if _, err := decodeReceipt(r, schema.validate); err != nil {
		return zero, false, ErrCorruptReceipt
	}
	return r, true, nil
}

type receiptRow struct {
	RequestHash []byte
	HTTPStatus  int
	Receipt     []byte
}

func lookup(ctx context.Context, db *gorm.DB, command string, s Scope) (receiptRow, bool, error) {
	var row receiptRow
	result := db.WithContext(ctx).Raw(`SELECT request_hash,http_status,receipt FROM eventstore.command_receipts
	WHERE actor_id=? AND company_id IS NOT DISTINCT FROM ?::uuid AND command_name=?
	AND target_id IS NOT DISTINCT FROM ?::uuid AND idempotency_key=?`, s.ActorID, nullable(s.CompanyID), command, nullable(s.TargetID), s.Key).Scan(&row)
	return row, result.RowsAffected == 1, result.Error
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s Schema[I, O]) digest(input I) ([32]byte, error) {
	var zero [32]byte
	value, err := s.normalize(input)
	if err != nil {
		return zero, ErrInvalidCommand
	}
	b, err := json.Marshal(value)
	if err != nil || !safeObject(b) {
		return zero, ErrInvalidCommand
	}
	// Normalize object ordering through the JSON value model without float64
	// conversion. Owner typed normalization defines numeric/string/array meaning:
	// null and omitted are distinct, array order is retained, decimal strings are
	// exact. json.Number representations require owner normalization. This is
	// deterministic encoding of the typed projection, not RFC 8785. Changing the
	// projection/normalization is a versioned command compatibility decision.
	var object map[string]any
	if strictDecode(b, &object) != nil {
		return zero, ErrInvalidCommand
	}
	b, err = json.Marshal(object)
	if err != nil {
		return zero, ErrInvalidCommand
	}
	if len(s.hmacKey) == 0 {
		return sha256.Sum256(b), nil
	}
	h := hmac.New(sha256.New, s.hmacKey)
	_, _ = h.Write(b)
	copy(zero[:], h.Sum(nil))
	return zero, nil
}
