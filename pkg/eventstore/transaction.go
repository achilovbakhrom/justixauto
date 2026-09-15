// Package eventstore implements owner-local PostgreSQL persistence adapters.
// It belongs outside service domain, application and port packages.
package eventstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var (
	ErrTransactionRequired  = errors.New("eventstore requires an active SQL transaction")
	ErrCommitOutcomeUnknown = errors.New("commit outcome unknown; reconcile authoritative command receipt")
)

// Transactions binds an owner's typed UnitOfWork to one SQL transaction. U is
// normally an interface declared in that owner's port package. Only the outer
// adapter/composition factory sees GORM; application callbacks receive U.
// Factories must construct transaction-bound repositories, never capture a
// separate database handle. Neither U nor its repositories may escape Run.
type Transactions[U any] struct {
	db   *gorm.DB
	bind func(*gorm.DB) (U, error)
}

func NewTransactions[U any](db *gorm.DB, bind func(*gorm.DB) (U, error)) (*Transactions[U], error) {
	if db == nil || db.Error != nil || bind == nil {
		return nil, errors.New("eventstore: database and typed unit-of-work factory are required")
	}
	return &Transactions[U]{db: db, bind: bind}, nil
}

// Run commits only after the complete typed callback succeeds. It does not
// retry callbacks: their command idempotency and receipt recovery are owner
// responsibilities. Any commit error is conservatively unknown, including a
// reply lost after the server committed. It is never reported as success or as
// proof of rollback. Network/broker effects must not execute in this callback.
func (r *Transactions[U]) Run(ctx context.Context, work func(U) error) (err error) {
	if r == nil || r.db == nil || r.bind == nil || work == nil {
		return errors.New("eventstore: uninitialized transaction runner or callback")
	}
	tx := r.db.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if tx.Error != nil {
		return tx.Error
	}
	committing := false
	defer func() {
		// Rollback also releases locks after a panic. After a commit attempt it
		// is cleanup only and cannot resolve the commit's authoritative outcome.
		rollbackErr := tx.Rollback().Error
		if !committing && rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback: %w", rollbackErr))
		}
	}()
	uow, err := r.bind(tx)
	if err != nil {
		return err
	}
	if err = work(uow); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	committing = true
	if err = tx.Commit().Error; err != nil {
		return errors.Join(ErrCommitOutcomeUnknown, err)
	}
	return nil
}

func requireTransaction(tx *gorm.DB) error {
	if tx == nil || tx.Error != nil || tx.Statement == nil {
		return ErrTransactionRequired
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return ErrTransactionRequired
	}
	return nil
}
