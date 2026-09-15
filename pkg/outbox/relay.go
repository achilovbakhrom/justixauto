package outbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"justixauto/pkg/events"
)

var (
	ErrRelayConfiguration = errors.New("outbox relay configuration is invalid")
	ErrRelayReadiness     = errors.New("outbox relay storage is incompatible")
	ErrRelayLease         = errors.New("outbox relay lease is absent, held or stale")
	ErrRelayUnknown       = errors.New("outbox relay commit outcome unknown; reconcile retained delivery")
)

type RelayMode string

const (
	RelayLegacy  RelayMode = "legacy"
	RelayCustody RelayMode = "custody"
)

// RelayStore is an outer persistence adapter, not a business authorization port.
// It never resolves admissions or modifies a recipient plan. The standard owner
// database/runtime identities are required. Construction is inert; Check and all
// operations verify the explicit mode under the same migration table fence.
type RelayStore struct {
	db    *gorm.DB
	owner events.Owner
	mode  RelayMode
	lease time.Duration
}

func NewRelayStore(db *gorm.DB, owner events.Owner, mode RelayMode, lease time.Duration) (*RelayStore, error) {
	if db == nil || db.Error != nil || db.Statement == nil || !owner.Valid() ||
		(mode != RelayLegacy && mode != RelayCustody) || lease < time.Millisecond || lease > 5*time.Minute {
		return nil, ErrRelayConfiguration
	}
	if _, transactional := db.Statement.ConnPool.(gorm.TxCommitter); transactional {
		return nil, ErrRelayConfiguration // network work must never inherit a caller transaction
	}
	return &RelayStore{db, owner, mode, lease}, nil
}

// Publication carries retained bytes and a sealed, known-committed attempt.
// Accessors return copies; changing current aggregate state, schema registration
// or admissions cannot rewrite it. The local deadline bounds network waiting;
// PostgreSQL remains the authority for the final lease/hold check.
type Publication struct {
	eventID, exchange, routingKey, attempt string
	body                                   []byte
	hash                                   [sha256.Size]byte
	committed                              bool
	until                                  time.Time
}

func (p Publication) EventID() string         { return p.eventID }
func (p Publication) Exchange() string        { return p.exchange }
func (p Publication) RoutingKey() string      { return p.routingKey }
func (p Publication) Bytes() []byte           { return bytes.Clone(p.body) }
func (p Publication) Hash() [sha256.Size]byte { return p.hash }

// RelayLease is sealed to its issuing store and exact immutable delivery. An
// unknown Claim commit returns this handle for Reconcile, never permission to
// publish. The normal Relay path stops on that error and lets the lease expire.
type RelayLease struct {
	store              *RelayStore
	publication        Publication
	destination, token string
	until              time.Time
}

// Publication from an unknown claim retains identity for inspection, but has no
// publish authority. Reconcile does not promote it; acquire a fresh claim after
// expiry. For normal execution prefer Relay.Once's pre-publication recheck.
func (l RelayLease) Publication() Publication { return l.publication }
func (l RelayLease) Until() time.Time         { return l.until }

type DeliveryState string

const (
	DeliveryMissing DeliveryState = "missing"
	DeliveryPending DeliveryState = "pending"
	DeliveryLeased  DeliveryState = "leased"
	DeliveryHeld    DeliveryState = "held"
	DeliverySent    DeliveryState = "sent"
)

func (s *RelayStore) table() string {
	if s.mode == RelayCustody {
		return "eventstore.outbox_deliveries"
	}
	return "eventstore.outbox"
}

// tx never runs a network callback or retries. Every commit error is unknown,
// even when a driver reports cancellation. Rollback cleanup cannot establish
// whether a preceding COMMIT reached PostgreSQL.
func (s *RelayStore) tx(ctx context.Context, work func(*gorm.DB) error) error {
	if s == nil || s.db == nil {
		return ErrRelayConfiguration
	}
	tx := s.db.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()
	if err := s.check(tx); err != nil {
		return err
	}
	if err := work(tx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return errors.Join(ErrRelayUnknown, err)
	}
	return nil
}

func (s *RelayStore) Check(ctx context.Context) error {
	return s.tx(ctx, func(*gorm.DB) error { return nil })
}

func (s *RelayStore) check(tx *gorm.DB) error {
	var valid bool
	// Reachable NOINHERIT roles matter: SET ROLE must not grant mutation of
	// retained bytes or confirmed history. PUBLIC privileges participate in the
	// has_* checks. INSERT is allowed for the existing shared owner writer role.
	err := tx.Raw(`SELECT current_database()=? AND current_user=?
 AND (SELECT pg_get_expr(d.adbin,d.adrelid)=quote_literal(?)||'::text'
      FROM pg_attrdef d JOIN pg_attribute a ON a.attrelid=d.adrelid AND a.attnum=d.adnum
      WHERE d.adrelid='eventstore.events'::regclass AND a.attname='owner_service')
 AND NOT EXISTS (SELECT FROM pg_roles r WHERE pg_has_role(current_user,r.oid,'MEMBER')
   AND (r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolreplication OR r.rolbypassrls
     OR r.rolname LIKE 'pg\_%' ESCAPE '\'
     OR r.oid=(SELECT datdba FROM pg_database WHERE datname=current_database())
     OR has_database_privilege(r.oid,current_database(),'CREATE,TEMPORARY')
     OR has_schema_privilege(r.oid,'public','CREATE')
     OR has_schema_privilege(r.oid,'eventstore','CREATE')))`,
		"justix_"+string(s.owner), "justix_"+string(s.owner)+"_runtime", string(s.owner)).Scan(&valid).Error
	if err != nil {
		return err
	}
	if !valid {
		return ErrRelayReadiness
	}
	i := &Inserter{tx: tx, owner: s.owner}
	if err := i.checkMode(tx.Statement.Context, string(s.mode)); err != nil {
		return err
	}
	tables := []string{s.table()}
	if s.mode == RelayCustody {
		tables = append(tables, "eventstore.outbox_messages")
	}
	for _, marker := range []string{"eventstore.messaging_mode", "eventstore.messaging_route_compatibility"} {
		var installed bool
		if err := tx.Raw("SELECT to_regclass(?) IS NOT NULL", marker).Scan(&installed).Error; err != nil {
			return err
		}
		if installed {
			tables = append(tables, marker)
		}
	}
	for _, table := range tables {
		err = tx.Raw(`SELECT has_table_privilege(current_user,?,'SELECT')
 AND NOT EXISTS (SELECT FROM pg_roles r WHERE pg_has_role(current_user,r.oid,'MEMBER')
   AND (has_table_privilege(r.oid,?,'UPDATE,DELETE,TRUNCATE,TRIGGER,REFERENCES,MAINTAIN')
     OR EXISTS (SELECT FROM pg_attribute a WHERE a.attrelid=?::regclass AND a.attnum>0 AND NOT a.attisdropped
       AND (has_column_privilege(r.oid,a.attrelid,a.attnum,'REFERENCES')
         OR (has_column_privilege(r.oid,a.attrelid,a.attnum,'UPDATE')
           AND NOT (? AND a.attname IN ('attempts','next_attempt_at','lease_owner','lease_until','sent_at','hold_ref')))))))`,
			table, table, table, table == s.table()).Scan(&valid).Error
		if err != nil {
			return err
		}
		if !valid {
			return ErrRelayReadiness
		}
		immutableMarker := table == "eventstore.messaging_mode" || table == "eventstore.messaging_route_compatibility"
		if err := tx.Raw(`SELECT NOT EXISTS (SELECT FROM pg_class c
 CROSS JOIN LATERAL (SELECT x.* FROM aclexplode(c.relacl) x
 UNION ALL SELECT x.* FROM pg_attribute a CROSS JOIN LATERAL aclexplode(a.attacl) x
 WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped) acl
 WHERE c.oid=?::regclass AND acl.is_grantable
 AND (acl.grantee=0 OR pg_has_role(current_user,acl.grantee,'MEMBER')))
 AND NOT EXISTS (SELECT FROM pg_roles r WHERE ? AND pg_has_role(current_user,r.oid,'MEMBER')
 AND (has_table_privilege(r.oid,?,'INSERT') OR EXISTS(SELECT FROM pg_attribute a
 WHERE a.attrelid=?::regclass AND a.attnum>0 AND NOT a.attisdropped
 AND has_column_privilege(r.oid,a.attrelid,a.attnum,'INSERT'))))`, table, immutableMarker, table, table).Scan(&valid).Error; err != nil {
			return err
		}
		if !valid {
			return ErrRelayReadiness
		}
	}
	for _, col := range []string{"attempts", "next_attempt_at", "lease_owner", "lease_until", "sent_at"} {
		if err := tx.Raw(`SELECT has_column_privilege(current_user,?::regclass,?,'UPDATE')`, s.table(), col).Scan(&valid).Error; err != nil {
			return err
		}
		if !valid {
			return ErrRelayReadiness
		}
	}
	return nil
}

type relayRow struct {
	EventID, Destination, Exchange, RoutingKey, AggregateType, AggregateID string
	IntegrationSequence                                                    int64
	Envelope, EnvelopeHash                                                 []byte
	LeaseUntil                                                             time.Time
}

func (s *RelayStore) Claim(ctx context.Context) (RelayLease, error) {
	var lease RelayLease
	err := s.tx(ctx, func(tx *gorm.DB) error {
		var rows []relayRow
		query := `SELECT d.event_id::text,d.exchange,d.routing_key,d.aggregate_type,d.aggregate_id::text,
 d.integration_sequence,d.envelope,d.envelope_hash FROM eventstore.outbox d
 WHERE d.sent_at IS NULL AND d.next_attempt_at<=clock_timestamp()
 AND (d.lease_until IS NULL OR d.lease_until<=clock_timestamp())
 ORDER BY d.next_attempt_at,d.event_id LIMIT 1 FOR UPDATE OF d SKIP LOCKED`
		if s.mode == RelayCustody {
			query = `SELECT d.event_id::text,d.destination,d.exchange,d.routing_key,m.aggregate_type,m.aggregate_id::text,
 m.integration_sequence,m.envelope,m.envelope_hash FROM eventstore.outbox_deliveries d
 JOIN eventstore.outbox_messages m USING(event_id)
 WHERE d.sent_at IS NULL AND d.hold_ref IS NULL AND d.next_attempt_at<=clock_timestamp()
 AND (d.lease_until IS NULL OR d.lease_until<=clock_timestamp())
 ORDER BY d.next_attempt_at,d.event_id,d.destination LIMIT 1 FOR UPDATE OF d SKIP LOCKED`
		}
		if err := tx.Raw(query).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		r := rows[0]
		var e events.Envelope
		if json.Unmarshal(r.Envelope, &e) != nil || e.Owner() != s.owner || e.EventID() != r.EventID ||
			e.AggregateType() != r.AggregateType || e.AggregateID() != r.AggregateID || e.IntegrationSequence().Int64() != r.IntegrationSequence {
			return ErrInvalidRecord
		}
		if s.mode == RelayLegacy {
			parts := strings.SplitN(r.RoutingKey, ".", 3)
			if len(parts) != 3 {
				return ErrInvalidRecord
			}
			r.Destination = parts[1]
		}
		h := sha256.Sum256(r.Envelope)
		if !events.Owner(r.Destination).Valid() || r.Exchange != IntegrationExchange ||
			r.RoutingKey != route(e, events.Owner(r.Destination)) || len(r.RoutingKey) > 255 || !bytes.Equal(h[:], r.EnvelopeHash) {
			return ErrInvalidRecord
		}
		token := uuid.NewString()
		lease = RelayLease{s, Publication{eventID: r.EventID, exchange: r.Exchange, routingKey: r.RoutingKey, attempt: token, body: bytes.Clone(r.Envelope), hash: h}, r.Destination, token, time.Time{}}
		args := []any{lease.token, s.lease.Microseconds(), r.EventID}
		where := "event_id=?::uuid"
		if s.mode == RelayCustody {
			where += " AND destination=?"
			args = append(args, r.Destination)
		}
		var updated []relayRow
		if err := tx.Raw("UPDATE "+s.table()+` SET attempts=attempts+1,lease_owner=?::uuid,
 lease_until=clock_timestamp()+(? * interval '1 microsecond') WHERE `+where+" RETURNING lease_until", args...).Scan(&updated).Error; err != nil {
			return err
		}
		if len(updated) != 1 {
			return ErrRelayLease
		}
		lease.until = updated[0].LeaseUntil
		lease.publication.until = lease.until
		return nil
	})
	if err != nil && !errors.Is(err, ErrRelayUnknown) {
		return RelayLease{}, err
	}
	if err == nil && !lease.IsEmpty() {
		lease.publication.committed = true
	}
	return lease, err
}

// IsEmpty distinguishes an idle successful claim from a sealed lease. Always
// inspect Claim's error first; a nonempty unknown candidate is not publishable.
func (l RelayLease) IsEmpty() bool { return l.store == nil }

func (s *RelayStore) validLease(l RelayLease) bool {
	return s != nil && l.store == s && admissionUUID(l.token) && admissionUUID(l.publication.eventID) && !l.until.IsZero()
}

// MarkSent requires the routed-confirm receipt for these exact retained bytes.
// A positive broker receipt is not a consumer or business-effect receipt.
func (s *RelayStore) MarkSent(ctx context.Context, l RelayLease, c RoutedConfirmation) error {
	if !s.validLease(l) || !l.publication.committed || !c.matches(l.publication) {
		return ErrRelayLease
	}
	return s.finish(ctx, l, true)
}

// Retry releases only this still-current unexpired lease and changes scheduling,
// never business state. On unknown completion, Reconcile first; do not call Retry
// as an assertion of rollback. Expired/stale/held rows remain untouched.
func (s *RelayStore) Retry(ctx context.Context, l RelayLease) error {
	if !s.validLease(l) {
		return ErrRelayLease
	}
	return s.finish(ctx, l, false)
}

func (s *RelayStore) finish(ctx context.Context, l RelayLease, sent bool) error {
	return s.tx(ctx, func(tx *gorm.DB) error {
		where, args := s.leaseWhere(l)
		// Acquire the row before the final clock predicate: waiting behind a hold
		// or another writer must not reuse a pre-wait lease-time observation.
		var rows []struct{ Attempts int64 }
		if err := tx.Raw("SELECT attempts FROM "+s.table()+" WHERE "+where+" FOR UPDATE", args...).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) != 1 {
			return ErrRelayLease
		}
		set := "sent_at=clock_timestamp(),lease_owner=NULL,lease_until=NULL"
		if !sent {
			delay, err := relayRetryDelay(rows[0].Attempts, rand.Float64())
			if err != nil {
				return err
			}
			set = "next_attempt_at=clock_timestamp()+(? * interval '1 microsecond'),lease_owner=NULL,lease_until=NULL"
			args = append([]any{delay.Microseconds()}, args...)
		}
		guard := " AND sent_at IS NULL AND lease_until>clock_timestamp()"
		if s.mode == RelayCustody {
			guard += " AND hold_ref IS NULL"
		}
		r := tx.Exec("UPDATE "+s.table()+" SET "+set+" WHERE "+where+guard, args...)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrRelayLease
		}
		return nil
	})
}

// Architecture §5: exponential 1s→60s bounded jitter. Attempts are persisted;
// restarting a worker does not reset the schedule. First retry is exactly 1s;
// subsequent samples span half the exponential cap through the cap, never <1s.
// This affects scheduling only. Outbox age/alert instrumentation belongs to T-020.
func relayRetryDelay(attempt int64, sample float64) (time.Duration, error) {
	if attempt < 1 || math.IsNaN(sample) || math.IsInf(sample, 0) || sample < 0 || sample > 1 {
		return 0, ErrRelayConfiguration
	}
	capSeconds := int64(60)
	if attempt <= 6 {
		capSeconds = 1 << uint(attempt-1)
	}
	low := math.Max(1, float64(capSeconds)/2)
	return time.Duration((low + (float64(capSeconds)-low)*sample) * float64(time.Second)), nil
}

func (s *RelayStore) leaseWhere(l RelayLease) (string, []any) {
	w := "event_id=?::uuid AND lease_owner=?::uuid"
	a := []any{l.publication.eventID, l.token}
	if s.mode == RelayCustody {
		w += " AND destination=?"
		a = append(a, l.destination)
	}
	return w, a
}

// Reconcile reads authoritative immutable identity and durable sent state after
// an unknown claim/completion. It never publishes, retries or alters scheduling.
// Leased is only a point-in-time observation, not a renewable lease authority.
func (s *RelayStore) Reconcile(ctx context.Context, l RelayLease) (DeliveryState, error) {
	if !s.validLease(l) {
		return "", ErrRelayLease
	}
	state := DeliveryMissing
	err := s.tx(ctx, func(tx *gorm.DB) error {
		var rows []struct {
			EnvelopeHash                []byte
			Exchange, RoutingKey, State string
		}
		q := `SELECT d.envelope_hash,d.exchange,d.routing_key, CASE WHEN d.sent_at IS NOT NULL THEN 'sent'
 WHEN d.lease_owner=?::uuid AND d.lease_until>clock_timestamp() THEN 'leased' ELSE 'pending' END AS state
 FROM eventstore.outbox d WHERE d.event_id=?::uuid`
		args := []any{l.token, l.publication.eventID}
		if s.mode == RelayCustody {
			q = `SELECT m.envelope_hash,d.exchange,d.routing_key,CASE WHEN d.sent_at IS NOT NULL THEN 'sent'
 WHEN d.hold_ref IS NOT NULL THEN 'held' WHEN d.lease_owner=?::uuid AND d.lease_until>clock_timestamp() THEN 'leased'
 ELSE 'pending' END AS state FROM eventstore.outbox_deliveries d JOIN eventstore.outbox_messages m USING(event_id)
 WHERE d.event_id=?::uuid AND d.destination=?`
			args = append(args, l.destination)
		}
		if err := tx.Raw(q, args...).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		r := rows[0]
		if !bytes.Equal(r.EnvelopeHash, l.publication.hash[:]) || r.Exchange != l.publication.exchange || r.RoutingKey != l.publication.routingKey {
			return ErrInvalidRecord
		}
		state = DeliveryState(r.State)
		return nil
	})
	if err != nil {
		return "", err
	}
	return state, nil
}

// ConfirmedPublisher is the outbound AMQP port. A successful implementation
// returns a sealed routed confirmation; arbitrary nil/error results cannot mark
// a delivery sent. Production wiring uses AMQPPublisher.
type ConfirmedPublisher interface {
	Publish(context.Context, Publication) (RoutedConfirmation, error)
}

type Relay struct {
	store     *RelayStore
	publisher ConfirmedPublisher
}

func NewRelay(store *RelayStore, publisher ConfirmedPublisher) (*Relay, error) {
	if store == nil || publisher == nil {
		return nil, ErrRelayConfiguration
	}
	return &Relay{store, publisher}, nil
}

// Once does bounded work for one recipient. It commits the claim before calling
// the publisher and opens a new completion transaction only after a routed
// confirm. Callers schedule subsequent attempts; there is no hidden retry loop.
// On any error the returned sealed lease supports authoritative reconciliation.
func (r *Relay) Once(ctx context.Context) (RelayLease, error) {
	if r == nil || r.store == nil || r.publisher == nil {
		return RelayLease{}, ErrRelayConfiguration
	}
	l, err := r.store.Claim(ctx)
	if err != nil || l.IsEmpty() {
		return l, err
	}
	state, err := r.store.Reconcile(ctx, l)
	if err != nil {
		return l, err
	}
	if state != DeliveryLeased {
		return l, ErrRelayLease
	}
	// Database clocks remain authoritative for sent state. This local deadline
	// only bounds the network attempt; clock skew cannot grant a SQL lease.
	publishCtx, cancel := context.WithDeadline(ctx, l.until)
	defer cancel()
	c, err := r.publisher.Publish(publishCtx, l.publication)
	if err != nil {
		return l, errors.Join(err, r.store.Retry(ctx, l))
	}
	return l, r.store.MarkSent(ctx, l, c)
}
