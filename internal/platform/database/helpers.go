package database

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"justixauto/internal/platform/apperr"
)

// Translate maps GORM errors to apperr kinds. Services add specific codes.
func Translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return apperr.ErrNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return apperr.New(apperr.ErrConflict, "duplicate", "a record with the same unique values already exists")
	case errors.Is(err, gorm.ErrForeignKeyViolated):
		return apperr.New(apperr.ErrConflict, "reference_missing", "a referenced record does not exist")
	}
	return err
}

// UpdateVersioned applies fields only if the row still has the expected
// version, and bumps the version. It reports ErrNotFound or ErrStale otherwise.
func UpdateVersioned(db *gorm.DB, model any, id string, expected int64, fields map[string]any) error {
	fields["version"] = expected + 1
	res := db.Model(model).Where("id = ? AND version = ?", id, expected).Updates(fields)
	if res.Error != nil {
		return Translate(res.Error)
	}
	if res.RowsAffected == 1 {
		return nil
	}
	var n int64
	if err := db.Model(model).Where("id = ?", id).Count(&n).Error; err != nil {
		return Translate(err)
	}
	if n == 0 {
		return apperr.ErrNotFound
	}
	return apperr.ErrStale
}

// Page applies list defaults: 50 items, at most 100.
func Page(limit, offset int) (int, int) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

type txKey struct{}

// WithTx carries an open transaction in ctx so that another module called
// through a port joins it (as a savepoint) instead of committing separately.
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// Conn returns the transaction carried by ctx, or db if there is none.
func Conn(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return tx
	}
	return db
}
