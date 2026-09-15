// Package postgres binds Identity ports to its privately owned PostgreSQL store.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"justixauto/pkg/eventstore"
	"justixauto/services/identity/port"
)

var ErrIncompatiblePersistence = errors.New("identity persistence is not compatible or runtime is not narrowly privileged")

// UnitOfWork constructs feature-local typed repositories through bind. Only
// adapter code sees the transaction. Bind must use this handle exclusively;
// it must not capture another database or perform network effects. Shared
// appender/replay/ledger/operation adapters are assembled here by each feature,
// with events.OwnerIdentity and explicit owner schemas/policies, never global lookup.
type UnitOfWork[U any] struct {
	db           *gorm.DB
	transactions *eventstore.Transactions[U]
}

// NewUnitOfWork is inert: it neither connects nor queries nor migrates. Startup
// must call Check explicitly. Run also checks within its transaction so a dirty
// or mismatched installation cannot reach a business callback.
func NewUnitOfWork[U any](db *gorm.DB, bind func(*gorm.DB) (U, error)) (*UnitOfWork[U], error) {
	if db == nil || db.Error != nil || bind == nil {
		return nil, ErrIncompatiblePersistence
	}
	r := &UnitOfWork[U]{db: db}
	tx, err := eventstore.NewTransactions(db, func(tx *gorm.DB) (U, error) {
		var zero U
		if err := check(tx); err != nil {
			return zero, err
		}
		return bind(tx)
	})
	if err != nil {
		return nil, err
	}
	r.transactions = tx
	return r, nil
}

func (r *UnitOfWork[U]) Check(ctx context.Context) error {
	if r == nil || r.db == nil {
		return ErrIncompatiblePersistence
	}
	return check(r.db.WithContext(ctx))
}

func (r *UnitOfWork[U]) Run(ctx context.Context, work func(U) error) error {
	if r == nil || r.transactions == nil {
		return ErrIncompatiblePersistence
	}
	return r.transactions.Run(ctx, work)
}

var _ port.UnitOfWork[struct{}] = (*UnitOfWork[struct{}])(nil)
var _ port.Readiness = (*UnitOfWork[struct{}])(nil)

// This compatibility check covers mechanics v1 only. It is not custody-mode
// readiness, an authorization check, or proof of delivery guarantees. Later
// additive migrations/composition must explicitly revise the supported version.
func check(db *gorm.DB) error {
	var valid bool
	err := db.Raw(`SELECT current_database()='justix_identity'
	 AND current_user='justix_identity_runtime'
	 AND (SELECT count(*)=1 AND bool_and(version=1 AND NOT dirty) FROM public.schema_migrations)
	 AND (SELECT count(*)=1 AND bool_and(owner_service='identity' AND mechanics_version=1)
	      FROM identity_mechanics.compatibility)
	 AND NOT EXISTS (SELECT FROM pg_roles r WHERE pg_has_role(current_user,r.oid,'MEMBER')
	      AND (r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolreplication OR r.rolbypassrls
	        OR r.rolname LIKE 'pg\_%' ESCAPE '\'
	        OR r.oid=(SELECT datdba FROM pg_database WHERE datname=current_database())))
	 AND NOT EXISTS (SELECT FROM pg_roles r WHERE pg_has_role(current_user,r.oid,'MEMBER')
	      AND (has_schema_privilege(r.oid,'eventstore','CREATE')
	        OR has_schema_privilege(r.oid,'public','CREATE')
	        OR has_schema_privilege(r.oid,'identity_mechanics','CREATE')
	        OR has_table_privilege(r.oid,'identity_mechanics.compatibility','INSERT,UPDATE,DELETE,TRUNCATE,TRIGGER')
	        OR has_table_privilege(r.oid,'public.schema_migrations','INSERT,UPDATE,DELETE,TRUNCATE,TRIGGER')))
	 AND NOT EXISTS (SELECT FROM pg_roles r CROSS JOIN
	      (VALUES ('events'),('command_receipts'),('outbox'),('inbox'),('operations'),('operation_steps')) t(name)
	      WHERE pg_has_role(current_user,r.oid,'MEMBER') AND
	      (NOT has_table_privilege(current_user,'eventstore.'||t.name,'SELECT')
	       OR NOT has_table_privilege(current_user,'eventstore.'||t.name,'INSERT')
	       OR has_table_privilege(r.oid,'eventstore.'||t.name,'UPDATE,DELETE,TRUNCATE,TRIGGER')))
	 AND NOT EXISTS (SELECT FROM pg_roles r CROSS JOIN pg_attribute a
	      JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace
	      WHERE pg_has_role(current_user,r.oid,'MEMBER') AND a.attnum>0 AND NOT a.attisdropped
	        AND (n.nspname='identity_mechanics' OR (n.nspname='public' AND c.relname='schema_migrations')
	          OR (n.nspname='eventstore' AND c.relname IN ('events','command_receipts','outbox','inbox','operations','operation_steps')))
	        AND has_column_privilege(r.oid,c.oid,a.attnum,'UPDATE')
	        AND NOT (n.nspname='eventstore' AND
	          ((c.relname='outbox' AND a.attname IN ('attempts','next_attempt_at','lease_owner','lease_until','sent_at'))
	           OR (c.relname='operations' AND a.attname IN ('phase','decision','attention_required','result','error','attempts','next_attempt_at','lease_owner','lease_until','revision','updated_at')))))
	 AND NOT EXISTS (SELECT FROM (VALUES
	      ('outbox','attempts'),('outbox','next_attempt_at'),('outbox','lease_owner'),('outbox','lease_until'),('outbox','sent_at'),
	      ('operations','phase'),('operations','decision'),('operations','attention_required'),('operations','result'),('operations','error'),
	      ('operations','attempts'),('operations','next_attempt_at'),('operations','lease_owner'),('operations','lease_until'),('operations','revision'),('operations','updated_at')) required(table_name,column_name)
	      WHERE NOT has_column_privilege(current_user,'eventstore.'||table_name,column_name,'UPDATE'))
	 AND EXISTS (SELECT FROM pg_attrdef d JOIN pg_attribute a ON a.attrelid=d.adrelid AND a.attnum=d.adnum
	      WHERE d.adrelid='eventstore.events'::regclass AND a.attname='owner_service'
	        AND pg_get_expr(d.adbin,d.adrelid)='''identity''::text')`).Scan(&valid).Error
	if err != nil {
		return fmt.Errorf("%w: %w", ErrIncompatiblePersistence, err)
	}
	if !valid {
		return ErrIncompatiblePersistence
	}
	return nil
}
