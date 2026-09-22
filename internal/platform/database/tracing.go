package database

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

const spanKey = "otel:span"

// useTracing adds a span per SQL statement under the request's trace. With no
// tracer provider installed the spans are no-ops.
func useTracing(db *gorm.DB) error {
	tracer := otel.Tracer("justixauto/database")
	before := func(op string) func(*gorm.DB) {
		return func(tx *gorm.DB) {
			ctx, span := tracer.Start(tx.Statement.Context, "db."+op, trace.WithSpanKind(trace.SpanKindClient),
				trace.WithAttributes(attribute.String("db.system", "postgresql"), attribute.String("db.sql.table", tx.Statement.Table)))
			tx.Statement.Context = ctx
			tx.InstanceSet(spanKey, span)
		}
	}
	after := func(tx *gorm.DB) {
		v, ok := tx.InstanceGet(spanKey)
		if !ok {
			return
		}
		span := v.(trace.Span)
		span.SetAttributes(attribute.String("db.statement", tx.Statement.SQL.String()), attribute.Int64("db.rows_affected", tx.Statement.RowsAffected))
		if tx.Error != nil && tx.Error != gorm.ErrRecordNotFound {
			span.RecordError(tx.Error)
			span.SetStatus(codes.Error, tx.Error.Error())
		}
		span.End()
	}
	cb := db.Callback()
	for _, r := range []struct {
		op     string
		before func(string, func(*gorm.DB)) error
		after  func(string, func(*gorm.DB)) error
	}{
		{"create", cb.Create().Before("gorm:create").Register, cb.Create().After("gorm:create").Register},
		{"query", cb.Query().Before("gorm:query").Register, cb.Query().After("gorm:query").Register},
		{"update", cb.Update().Before("gorm:update").Register, cb.Update().After("gorm:update").Register},
		{"delete", cb.Delete().Before("gorm:delete").Register, cb.Delete().After("gorm:delete").Register},
		{"row", cb.Row().Before("gorm:row").Register, cb.Row().After("gorm:row").Register},
		{"raw", cb.Raw().Before("gorm:raw").Register, cb.Raw().After("gorm:raw").Register},
	} {
		if err := r.before("otel:before_"+r.op, before(r.op)); err != nil {
			return err
		}
		if err := r.after("otel:after_"+r.op, after); err != nil {
			return err
		}
	}
	return nil
}
