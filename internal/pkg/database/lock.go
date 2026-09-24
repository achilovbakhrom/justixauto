package database

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ErrLocked reports that another replica is running the same job.
var ErrLocked = errors.New("database: job is running on another replica")

// RunOnce runs fn on at most one replica at a time. It holds the PostgreSQL
// advisory lock named by name for the duration of fn (a transaction-level
// lock, so it is released when fn returns and when the connection or the pod
// dies). While another replica holds the lock RunOnce returns ErrLocked at
// once instead of waiting: the caller skips this run and tries again on its
// next tick. Use it for scheduled and background jobs.
func RunOnce(ctx context.Context, db *gorm.DB, name string, fn func(ctx context.Context) error) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var acquired bool
		if err := tx.Raw("SELECT pg_try_advisory_xact_lock(hashtext(?))", name).Scan(&acquired).Error; err != nil {
			return err
		}
		if !acquired {
			return ErrLocked
		}
		return fn(ctx)
	})
}
