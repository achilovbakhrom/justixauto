package outbox_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
	"justixauto/pkg/outbox"
)

type payload struct {
	Ref string `json:"ref"`
}

func schema(t *testing.T, owner events.Owner) events.EventSchema[payload] {
	t.Helper()
	s, err := events.NewEventSchema(string(owner)+".fixture.changed.v1", 1, "fixture", events.CompanyScopeTenantRequired, func(p payload) error {
		if p.Ref == "" {
			return errors.New("missing reference")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func revision(n int64) events.Revision {
	r, err := events.NewRevision(n)
	if err != nil {
		panic(err)
	}
	return r
}
func pending(t *testing.T, id string, rev, seq int64) eventstore.Event {
	t.Helper()
	company := uuid.NewString()
	in := events.EnvelopeInput{EventID: uuid.NewString(), AggregateID: id, AggregateVersion: revision(rev), IntegrationSequence: revision(seq), CompanyID: &company, OccurredAt: time.Now().UTC(), Actor: events.Actor{Kind: events.ActorUser, ID: uuid.NewString()}, CorrelationID: uuid.NewString(), CausationID: uuid.NewString(), OperationID: uuid.NewString()}
	var e eventstore.Event
	var err error
	if seq == 0 {
		e, err = eventstore.NewInternalEvent(in, schema(t, events.OwnerInventory), payload{Ref: "synthetic"})
	} else {
		e, err = eventstore.NewEvent(in, schema(t, events.OwnerInventory), payload{Ref: "synthetic"})
	}
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func record(t *testing.T, e eventstore.Event) outbox.Record {
	t.Helper()
	envelope, ok := e.IntegrationEnvelope()
	if !ok {
		t.Fatal("missing integration envelope")
	}
	r, err := outbox.NewRecord(envelope, schema(t, events.OwnerInventory), events.OwnerRetail)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func changed(t *testing.T, e eventstore.Event, field string, value any) events.Envelope {
	t.Helper()
	envelope, _ := e.IntegrationEnvelope()
	b, err := envelope.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatal(err)
	}
	wire[field] = value
	b, err = json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	var result events.Envelope
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRecordRequiresTypedSchemaAndRoute(t *testing.T) {
	e := pending(t, uuid.NewString(), 1, 1)
	envelope, _ := e.IntegrationEnvelope()
	for _, tc := range []struct {
		name     string
		envelope events.Envelope
		schema   events.EventSchema[payload]
		target   events.Owner
	}{
		{"zero envelope", events.Envelope{}, schema(t, events.OwnerInventory), events.OwnerRetail},
		{"unbound schema", envelope, events.EventSchema[payload]{}, events.OwnerRetail},
		{"foreign schema", envelope, schema(t, events.OwnerIdentity), events.OwnerRetail},
		{"invalid target", envelope, schema(t, events.OwnerInventory), events.Owner("*")},
		{"raw unvalidated payload", changed(t, e, "data", map[string]string{"ref": ""}), schema(t, events.OwnerInventory), events.OwnerRetail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := outbox.NewRecord(tc.envelope, tc.schema, tc.target); !errors.Is(err, outbox.ErrInvalidRecord) {
				t.Fatal(err)
			}
		})
	}
	if _, err := outbox.NewInserter(nil, events.OwnerInventory); !errors.Is(err, eventstore.ErrTransactionRequired) {
		t.Fatal(err)
	}
	var i *outbox.Inserter
	if err := i.Insert(context.Background()); !errors.Is(err, eventstore.ErrTransactionRequired) {
		t.Fatal(err)
	}
	longSchema, err := events.NewEventSchema("inventory."+strings.Repeat("a", 240)+".v1", 1, "fixture", events.CompanyScopeTenantRequired, func(payload) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	long := changed(t, e, "eventType", "inventory."+strings.Repeat("a", 240)+".v1")
	if _, err := outbox.NewRecord(long, longSchema, events.OwnerRetail); !errors.Is(err, outbox.ErrInvalidRecord) {
		t.Fatal("accepted unroutable long key", err)
	}
}

type ports struct {
	append *eventstore.Appender
	insert *outbox.Inserter
	tx     *gorm.DB
}

func runner(t *testing.T, db *gorm.DB) *eventstore.Transactions[ports] {
	t.Helper()
	r, err := eventstore.NewTransactions(db, func(tx *gorm.DB) (ports, error) {
		a, err := eventstore.NewAppender(tx, events.OwnerInventory)
		if err != nil {
			return ports{}, err
		}
		i, err := outbox.NewInserter(tx, events.OwnerInventory)
		return ports{a, i, tx}, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestInsertPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_OUTBOX_INSERT") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_OUTBOX_INSERT=1 for isolated PostgreSQL outbox checks")
	}
	db := database(t)
	r := runner(t, db)
	ctx := context.Background()
	if err := r.Run(ctx, func(p ports) error {
		if _, err := outbox.NewInserter(p.tx, events.Owner("foreign")); !errors.Is(err, outbox.ErrInvalidRecord) {
			t.Fatal("accepted invalid owner", err)
		}
		return p.insert.Insert(ctx)
	}); err != nil {
		t.Fatal("internal-only empty insert", err)
	}
	if _, err := outbox.NewInserter(db, events.OwnerInventory); !errors.Is(err, eventstore.ErrTransactionRequired) {
		t.Fatal("accepted pool", err)
	}
	count := func(table, id string) int64 {
		t.Helper()
		var n int64
		if err := db.Raw("SELECT count(*) FROM eventstore."+table+" WHERE aggregate_id=?::uuid", id).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	write := func(p ports, id string, es ...eventstore.Event) error {
		if err := p.append.Append(ctx, eventstore.Batch{Expected: revision(0), Events: es}); err != nil {
			return err
		}
		if err := p.tx.Exec("INSERT INTO eventstore.fixture_guard (aggregate_id) VALUES (?::uuid)", id).Error; err != nil {
			return err
		}
		var records []outbox.Record
		for _, e := range es {
			if _, ok := e.IntegrationEnvelope(); ok {
				records = append(records, record(t, e))
			}
		}
		return p.insert.Insert(ctx, records...)
	}
	assertCount := func(id string, eventCount, outboxCount, guardCount int64) {
		t.Helper()
		for table, want := range map[string]int64{"events": eventCount, "outbox": outboxCount, "fixture_guard": guardCount} {
			if got := count(table, id); got != want {
				t.Fatalf("%s: got %d want %d", table, got, want)
			}
		}
	}
	t.Run("atomic commit preserves bytes and independent positions", func(t *testing.T) {
		id := uuid.NewString()
		es := []eventstore.Event{pending(t, id, 1, 1), pending(t, id, 2, 0), pending(t, id, 3, 2)}
		if err := r.Run(ctx, func(p ports) error { return write(p, id, es...) }); err != nil {
			t.Fatal(err)
		}
		assertCount(id, 3, 2, 1)
		for _, e := range []eventstore.Event{es[0], es[2]} {
			env, _ := e.IntegrationEnvelope()
			want, _ := env.MarshalJSON()
			hash := sha256.Sum256(want)
			var row struct {
				Envelope, EnvelopeHash        []byte
				Exchange, RoutingKey          string
				IntegrationSequence, Attempts int64
				SentAt, LeaseUntil            *time.Time
				LeaseOwner                    *string
			}
			if err := db.Raw("SELECT * FROM eventstore.outbox WHERE event_id=?::uuid", env.EventID()).Scan(&row).Error; err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(row.Envelope, want) || !bytes.Equal(row.EnvelopeHash, hash[:]) || row.Exchange != outbox.IntegrationExchange || row.RoutingKey != "inventory.retail."+env.EventType() || row.IntegrationSequence != env.IntegrationSequence().Int64() || row.Attempts != 0 || row.SentAt != nil || row.LeaseUntil != nil || row.LeaseOwner != nil {
				t.Fatalf("persisted record mismatch: %+v", row)
			}
		}
	})
	t.Run("rollback and cancellation remove every local effect", func(t *testing.T) {
		stop := errors.New("synthetic rollback")
		for _, cancelled := range []bool{false, true} {
			id := uuid.NewString()
			e := pending(t, id, 1, 1)
			runctx, cancel := context.WithCancel(ctx)
			err := r.Run(runctx, func(p ports) error {
				if err := write(p, id, e); err != nil {
					return err
				}
				if cancelled {
					cancel()
					return nil
				}
				return stop
			})
			cancel()
			if cancelled {
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			} else if !errors.Is(err, stop) {
				t.Fatal(err)
			}
			assertCount(id, 0, 0, 0)
		}
	})
	t.Run("uncommitted outbox is invisible", func(t *testing.T) {
		id := uuid.NewString()
		e := pending(t, id, 1, 1)
		ready, release := make(chan struct{}), make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- r.Run(ctx, func(p ports) error {
				if err := write(p, id, e); err != nil {
					return err
				}
				close(ready)
				<-release
				return nil
			})
		}()
		select {
		case <-ready:
		case err := <-done:
			t.Fatal(err)
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("write timeout")
		}
		assertCount(id, 0, 0, 0)
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		assertCount(id, 1, 1, 1)
	})
	t.Run("missing internal or mismatched local event fails closed", func(t *testing.T) {
		for _, field := range []string{"absent", "internal", "aggregateVersion", "integrationSequence", "companyId", "actor", "operationId"} {
			t.Run(field, func(t *testing.T) {
				id := uuid.NewString()
				e := pending(t, id, 1, 1)
				env, _ := e.IntegrationEnvelope()
				if field == "aggregateVersion" || field == "integrationSequence" {
					env = changed(t, e, field, "2")
				}
				if field == "companyId" || field == "operationId" {
					env = changed(t, e, field, uuid.NewString())
				}
				if field == "actor" {
					env = changed(t, e, field, map[string]string{"kind": "user", "id": uuid.NewString()})
				}
				rec, err := outbox.NewRecord(env, schema(t, events.OwnerInventory), events.OwnerRetail)
				if err != nil {
					t.Fatal(err)
				}
				err = r.Run(ctx, func(p ports) error {
					if field != "absent" {
						stored := e
						if field == "internal" {
							company, _ := env.CompanyID()
							stored, err = eventstore.NewInternalEvent(events.EnvelopeInput{
								EventID: env.EventID(), AggregateID: env.AggregateID(), AggregateVersion: env.AggregateVersion(),
								CompanyID: &company, OccurredAt: env.OccurredAt(), Actor: env.Actor(),
								CorrelationID: env.CorrelationID(), CausationID: env.CausationID(), OperationID: env.OperationID(),
							}, schema(t, events.OwnerInventory), payload{Ref: "synthetic"})
							if err != nil {
								return err
							}
						}
						if err := p.append.Append(ctx, eventstore.Batch{Events: []eventstore.Event{stored}}); err != nil {
							return err
						}
					}
					return p.insert.Insert(ctx, rec)
				})
				if !errors.Is(err, outbox.ErrEventNotFound) {
					t.Fatal(err)
				}
				assertCount(id, 0, 0, 0)
			})
		}
	})
	t.Run("foreign owner record and late missing event roll back", func(t *testing.T) {
		for _, foreign := range []bool{true, false} {
			id := uuid.NewString()
			e := pending(t, id, 1, 1)
			missing := pending(t, uuid.NewString(), 1, 1)
			rec := record(t, missing)
			want := outbox.ErrEventNotFound
			if foreign {
				env := changed(t, missing, "eventType", "identity.fixture.changed.v1")
				var err error
				rec, err = outbox.NewRecord(env, schema(t, events.OwnerIdentity), events.OwnerRetail)
				if err != nil {
					t.Fatal(err)
				}
				want = outbox.ErrInvalidRecord
			}
			err := r.Run(ctx, func(p ports) error {
				if err := write(p, id, e); err != nil {
					return err
				}
				return p.insert.Insert(ctx, rec)
			})
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
			assertCount(id, 0, 0, 0)
		}
	})
	t.Run("duplicate insert aborts the complete command", func(t *testing.T) {
		id := uuid.NewString()
		e := pending(t, id, 1, 1)
		err := r.Run(ctx, func(p ports) error {
			if err := write(p, id, e); err != nil {
				return err
			}
			return p.insert.Insert(ctx, record(t, e))
		})
		if err == nil {
			t.Fatal("duplicate accepted")
		}
		assertCount(id, 0, 0, 0)
	})
	t.Run("invalid later record cannot leave partial command", func(t *testing.T) {
		id := uuid.NewString()
		e := pending(t, id, 1, 1)
		err := r.Run(ctx, func(p ports) error {
			if err := p.append.Append(ctx, eventstore.Batch{Events: []eventstore.Event{e}}); err != nil {
				return err
			}
			return p.insert.Insert(ctx, record(t, e), outbox.Record{})
		})
		if !errors.Is(err, outbox.ErrInvalidRecord) {
			t.Fatal(err)
		}
		assertCount(id, 0, 0, 0)
	})
	t.Run("concurrent expected revision gives one outbound winner", func(t *testing.T) {
		id := uuid.NewString()
		es := []eventstore.Event{pending(t, id, 1, 1), pending(t, id, 1, 1)}
		start := make(chan struct{})
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for _, e := range es {
			wg.Go(func() { <-start; errs <- r.Run(ctx, func(p ports) error { return write(p, id, e) }) })
		}
		close(start)
		wg.Wait()
		close(errs)
		wins, conflicts := 0, 0
		for err := range errs {
			if err == nil {
				wins++
			} else if errors.Is(err, eventstore.ErrVersionConflict) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
		if wins != 1 || conflicts != 1 {
			t.Fatalf("winners=%d conflicts=%d", wins, conflicts)
		}
		assertCount(id, 1, 1, 1)
	})
	t.Run("committed record cannot change recipient or bytes", func(t *testing.T) {
		id := uuid.NewString()
		e := pending(t, id, 1, 1)
		if err := r.Run(ctx, func(p ports) error { return write(p, id, e) }); err != nil {
			t.Fatal(err)
		}
		env, _ := e.IntegrationEnvelope()
		rec, err := outbox.NewRecord(env, schema(t, events.OwnerInventory), events.OwnerCommerce)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Run(ctx, func(p ports) error { return p.insert.Insert(ctx, rec) }); err == nil {
			t.Fatal("retargeted event")
		}
		for _, statement := range []string{"UPDATE eventstore.outbox SET routing_key='inventory.commerce.fixture' WHERE aggregate_id=?::uuid", "UPDATE eventstore.outbox SET envelope='{}'::bytea,envelope_hash=sha256('{}'::bytea) WHERE aggregate_id=?::uuid", "DELETE FROM eventstore.outbox WHERE aggregate_id=?::uuid"} {
			if err := db.Exec(statement, id).Error; err == nil {
				t.Fatal("runtime changed immutable record")
			}
		}
		assertCount(id, 1, 1, 1)
	})
}

func TestMessageRequiresTypedSchemaAndConcreteRule(t *testing.T) {
	e := pending(t, uuid.NewString(), 1, 1)
	env, _ := e.IntegrationEnvelope()
	s := custodyStream(t, env.AggregateID())
	rule := custodyRule(t, s, true)
	for _, tc := range []struct {
		name   string
		env    events.Envelope
		schema events.EventSchema[payload]
		rule   outbox.SourcePlanRule
	}{
		{"zero envelope", events.Envelope{}, schema(t, events.OwnerInventory), rule},
		{"zero schema", env, events.EventSchema[payload]{}, rule},
		{"foreign schema", env, schema(t, events.OwnerIdentity), rule},
		{"invalid data", changed(t, e, "data", map[string]string{"ref": ""}), schema(t, events.OwnerInventory), rule},
		{"missing rule", env, schema(t, events.OwnerInventory), outbox.SourcePlanRule{}},
		{"other stream", env, schema(t, events.OwnerInventory), custodyRule(t, custodyStream(t, uuid.NewString()), true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := outbox.NewMessage(tc.env, tc.schema, tc.rule); !errors.Is(err, outbox.ErrInvalidRecord) {
				t.Fatal(err)
			}
		})
	}
	if _, err := outbox.NewCustodyInserter(nil, events.OwnerInventory, nil); !errors.Is(err, eventstore.ErrTransactionRequired) {
		t.Fatal(err)
	}
	var i *outbox.Inserter
	if err := i.InsertMessages(context.Background()); !errors.Is(err, eventstore.ErrTransactionRequired) {
		t.Fatal(err)
	}
}

type custodyPorts struct {
	tx         *gorm.DB
	admissions *outbox.SourceAdmissions
	append     *eventstore.Appender
	insert     *outbox.Inserter
}

func custodyRunner(t *testing.T, db *gorm.DB) *eventstore.Transactions[custodyPorts] {
	t.Helper()
	r, err := eventstore.NewTransactions(db, func(tx *gorm.DB) (custodyPorts, error) {
		a, err := outbox.NewSourceAdmissions(tx, events.OwnerInventory)
		if err != nil {
			return custodyPorts{}, err
		}
		app, err := eventstore.NewAppender(tx, events.OwnerInventory)
		if err != nil {
			return custodyPorts{}, err
		}
		i, err := outbox.NewCustodyInserter(tx, events.OwnerInventory, a)
		return custodyPorts{tx, a, app, i}, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func custodyStream(t *testing.T, id string) outbox.SourceStream {
	t.Helper()
	s, err := outbox.NewSourceStream(events.OwnerInventory, "fixture", id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func custodyRule(t *testing.T, s outbox.SourceStream, empty bool) outbox.SourcePlanRule {
	t.Helper()
	ref := ""
	if empty {
		ref = "synthetic-explicit-empty"
	}
	r, err := outbox.NewSourcePlanRule(s, "synthetic-owner-rule", []string{"inventory.fixture.changed.v1"}, ref)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func custodyMessage(t *testing.T, e eventstore.Event, rule outbox.SourcePlanRule) outbox.Message {
	t.Helper()
	env, _ := e.IntegrationEnvelope()
	m, err := outbox.NewMessage(env, schema(t, events.OwnerInventory), rule)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func custodyAdmit(t *testing.T, p custodyPorts, s outbox.SourceStream, d events.Owner, h int64) string {
	t.Helper()
	id := uuid.NewString()
	empty := ""
	if h == 0 {
		empty = "synthetic-new-stream-proof"
	}
	a, err := outbox.NewSourceAdmission(outbox.SourceAdmissionInput{ID: id, Stream: s, Destination: d,
		Contract: outbox.AdmissionContract{ID: "synthetic-v1", Version: 1, Digest: sha256.Sum256([]byte("synthetic-approved-contract"))},
		Schemas:  []string{"inventory.fixture.changed.v1"}, AuthorityRef: "synthetic-authority", ScopeRef: "synthetic-scope", Purpose: "synthetic-test",
		Bootstrap: outbox.BootstrapEvidence{Checkpoint: revision(h), Ref: "synthetic-bootstrap", EmptyStreamRef: empty}})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.admissions.Admit(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCustodyInsertPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_OUTBOX_INSERT") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_OUTBOX_INSERT=1 for isolated PostgreSQL custody insertion")
	}
	var admin func(string, ...string) (string, error)
	db := database(t, func(run func(string, ...string) (string, error)) {
		admin = run
		for _, file := range []string{"000002_messaging_delivery.up.sql", "000003_messaging_route_compatibility.up.sql"} {
			b, err := os.ReadFile("../eventstore/migrations/" + file)
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"-v", "owner_service=inventory", "-v", "runtime_role=justix_inventory_runtime"}
			if strings.HasPrefix(file, "000003") {
				args = append(args, "-v", "prior_migration_sha256=1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2", "-v", fmt.Sprintf("correction_migration_sha256=%x", sha256.Sum256(b)), "-v", "backup_ref=synthetic-empty-backup", "-v", "stopped_runtimes_ref=synthetic-no-runtimes", "-v", "compatibility_ref=synthetic-no-broker")
			}
			if out, err := run(string(b), args...); err != nil {
				t.Fatalf("%s: %v %s", file, err, out)
			}
		}
	})
	ctx := context.Background()
	r := custodyRunner(t, db)
	if err := r.Run(ctx, func(p custodyPorts) error { return p.insert.InsertMessages(ctx) }); !errors.Is(err, outbox.ErrMessagingMode) {
		t.Fatal("custody accepted legacy mode", err)
	}
	if err := runner(t, db).Run(ctx, func(p ports) error { return p.insert.Insert(ctx) }); err != nil {
		t.Fatal("additive legacy mode", err)
	}
	if out, err := admin(`SELECT eventstore.activate_messaging_custody('` + uuid.NewString() + `',sha256('synthetic-new'::bytea),sha256('synthetic-old'::bytea),'synthetic-empty-backup','synthetic-no-runtimes','synthetic-no-broker-or-checkpoints','synthetic-compatible')`); err != nil {
		t.Fatal(err, out)
	}
	count := func(table, id string) int64 {
		t.Helper()
		var n int64
		if err := db.Raw("SELECT count(*) FROM eventstore."+table+" WHERE aggregate_id=?::uuid", id).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	assertCounts := func(id string, e, m int64) {
		t.Helper()
		if count("events", id) != e || count("outbox_messages", id) != m {
			t.Fatalf("partial command %s: events=%d messages=%d", id, count("events", id), count("outbox_messages", id))
		}
	}
	write := func(p custodyPorts, s outbox.SourceStream, expected int64, empty bool, es ...eventstore.Event) error {
		if err := p.append.Append(ctx, eventstore.Batch{Expected: revision(expected), Events: es}); err != nil {
			return err
		}
		ms := make([]outbox.Message, 0, len(es))
		for _, e := range es {
			if _, ok := e.IntegrationEnvelope(); ok {
				ms = append(ms, custodyMessage(t, e, custodyRule(t, s, empty)))
			}
		}
		return p.insert.InsertMessages(ctx, ms...)
	}
	t.Run("mode guards include empty and mixed calls", func(t *testing.T) {
		if err := runner(t, db).Run(ctx, func(p ports) error { return p.insert.Insert(ctx) }); err == nil {
			t.Fatal("legacy empty accepted custody")
		}
		if err := r.Run(ctx, func(p custodyPorts) error { return p.insert.InsertMessages(ctx) }); err != nil {
			t.Fatal(err)
		}
		if err := r.Run(ctx, func(p custodyPorts) error { return p.insert.Insert(ctx) }); !errors.Is(err, outbox.ErrMessagingMode) {
			t.Fatal(err)
		}
		if err := r.Run(ctx, func(p custodyPorts) error {
			if err := p.tx.Exec("SET LOCAL justix.messaging_mode='legacy'").Error; err != nil {
				return err
			}
			return p.insert.InsertMessages(ctx)
		}); !errors.Is(err, outbox.ErrMessagingMode) {
			t.Fatal(err)
		}
		if err := r.Run(ctx, func(p custodyPorts) error {
			if err := p.tx.Exec("SET TRANSACTION ISOLATION LEVEL REPEATABLE READ").Error; err != nil {
				return err
			}
			return p.insert.InsertMessages(ctx)
		}); !errors.Is(err, outbox.ErrMessagingMode) {
			t.Fatal(err)
		}
	})
	t.Run("one parent complete recipients full routes and internal positions", func(t *testing.T) {
		s := custodyStream(t, uuid.NewString())
		es := []eventstore.Event{pending(t, s.AggregateID(), 1, 1), pending(t, s.AggregateID(), 2, 0), pending(t, s.AggregateID(), 3, 2)}
		if err := r.Run(ctx, func(p custodyPorts) error {
			if err := p.admissions.Fence(ctx, s); err != nil {
				return err
			}
			custodyAdmit(t, p, s, events.OwnerRetail, 0)
			custodyAdmit(t, p, s, events.OwnerInventory, 0)
			return write(p, s, 0, false, es...)
		}); err != nil {
			t.Fatal(err)
		}
		assertCounts(s.AggregateID(), 3, 2)
		for _, e := range []eventstore.Event{es[0], es[2]} {
			env, _ := e.IntegrationEnvelope()
			body, _ := env.MarshalJSON()
			hash := sha256.Sum256(body)
			var rows []struct {
				Envelope, EnvelopeHash, Recipients   []byte
				Destination, RoutingKey, AdmissionID string
				Attempts                             int64
				SentAt                               *time.Time
			}
			if err := db.Raw(`SELECT m.envelope,m.envelope_hash,m.recipients,d.destination,d.routing_key,d.admission_id,d.attempts,d.sent_at FROM eventstore.outbox_messages m JOIN eventstore.outbox_deliveries d USING(event_id) WHERE event_id=?::uuid ORDER BY destination`, env.EventID()).Scan(&rows).Error; err != nil {
				t.Fatal(err)
			}
			if len(rows) != 2 {
				t.Fatal("incomplete fanout", len(rows))
			}
			for _, row := range rows {
				if !bytes.Equal(row.Envelope, body) || !bytes.Equal(row.EnvelopeHash, hash[:]) || row.RoutingKey != "inventory."+row.Destination+"."+env.EventType() || row.Attempts != 0 || row.SentAt != nil {
					t.Fatalf("wrong immutable message/child %+v", row)
				}
				var children []map[string]string
				if json.Unmarshal(row.Recipients, &children) != nil || len(children) != 2 {
					t.Fatal("bad parent set")
				}
			}
		}
	})
	t.Run("explicit empty authority absent rule and no historical replanning", func(t *testing.T) {
		s := custodyStream(t, uuid.NewString())
		e := pending(t, s.AggregateID(), 1, 1)
		if err := r.Run(ctx, func(p custodyPorts) error {
			if err := p.admissions.Fence(ctx, s); err != nil {
				return err
			}
			return write(p, s, 0, false, e)
		}); !errors.Is(err, outbox.ErrMissingRecipientPlan) {
			t.Fatal(err)
		}
		assertCounts(s.AggregateID(), 0, 0)
		if err := r.Run(ctx, func(p custodyPorts) error {
			if err := p.admissions.Fence(ctx, s); err != nil {
				return err
			}
			return write(p, s, 0, true, e)
		}); err != nil {
			t.Fatal(err)
		}
		var authority string
		if err := db.Raw("SELECT plan_authority_ref FROM eventstore.outbox_messages WHERE aggregate_id=?::uuid", s.AggregateID()).Scan(&authority).Error; err != nil || authority != "synthetic-explicit-empty" {
			t.Fatal(authority, err)
		}
		if err := r.Run(ctx, func(p custodyPorts) error {
			if err := p.admissions.Fence(ctx, s); err != nil {
				return err
			}
			return p.insert.InsertMessages(ctx, custodyMessage(t, e, custodyRule(t, s, true)))
		}); !errors.Is(err, outbox.ErrAdmissionCheckpoint) {
			t.Fatal("replanned history", err)
		}
		assertCounts(s.AggregateID(), 1, 1)
	})
	t.Run("all actual metadata must match and late failure is atomic", func(t *testing.T) {
		for _, field := range []string{"eventId", "aggregateVersion", "integrationSequence", "companyId", "actor", "occurredAt", "correlationId", "causationId", "operationId"} {
			t.Run(field, func(t *testing.T) {
				s := custodyStream(t, uuid.NewString())
				e := pending(t, s.AggregateID(), 1, 1)
				var value any = uuid.NewString()
				if field == "aggregateVersion" || field == "integrationSequence" {
					value = "2"
				}
				if field == "actor" {
					value = map[string]string{"kind": "service", "id": uuid.NewString()}
				}
				if field == "occurredAt" {
					value = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
				}
				m, err := outbox.NewMessage(changed(t, e, field, value), schema(t, events.OwnerInventory), custodyRule(t, s, true))
				if err != nil {
					t.Fatal(err)
				}
				err = r.Run(ctx, func(p custodyPorts) error {
					if err := p.admissions.Fence(ctx, s); err != nil {
						return err
					}
					if err := p.append.Append(ctx, eventstore.Batch{Events: []eventstore.Event{e}}); err != nil {
						return err
					}
					return p.insert.InsertMessages(ctx, m)
				})
				if err == nil {
					t.Fatal("mismatch accepted")
				}
				assertCounts(s.AggregateID(), 0, 0)
			})
		}
		for _, kind := range []string{"duplicate", "zero", "late-missing", "callback-error", "panic", "cancel"} {
			t.Run(kind, func(t *testing.T) {
				s := custodyStream(t, uuid.NewString())
				e := pending(t, s.AggregateID(), 1, 1)
				stop := errors.New("synthetic-stop")
				func() {
					defer func() {
						if v := recover(); v != nil && kind != "panic" {
							panic(v)
						}
					}()
					err := r.Run(ctx, func(p custodyPorts) error {
						if err := p.admissions.Fence(ctx, s); err != nil {
							return err
						}
						custodyAdmit(t, p, s, events.OwnerRetail, 0)
						if err := write(p, s, 0, false, e); err != nil {
							return err
						}
						switch kind {
						case "duplicate":
							return p.insert.InsertMessages(ctx, custodyMessage(t, e, custodyRule(t, s, false)))
						case "zero":
							return p.insert.InsertMessages(ctx, outbox.Message{})
						case "late-missing":
							return p.insert.InsertMessages(ctx, custodyMessage(t, pending(t, s.AggregateID(), 2, 2), custodyRule(t, s, false)))
						case "panic":
							panic(stop)
						case "cancel":
							c, cancel := context.WithCancel(ctx)
							cancel()
							return p.insert.InsertMessages(c)
						}
						return stop
					})
					if err == nil {
						t.Fatal("partial command succeeded")
					}
				}()
				assertCounts(s.AggregateID(), 0, 0)
			})
		}
	})
	t.Run("nonzero bootstrap drain hold resume and fresh resolution", func(t *testing.T) {
		s := custodyStream(t, uuid.NewString())
		e1 := pending(t, s.AggregateID(), 1, 1)
		if err := r.Run(ctx, func(p custodyPorts) error {
			if err := p.admissions.Fence(ctx, s); err != nil {
				return err
			}
			return write(p, s, 0, true, e1)
		}); err != nil {
			t.Fatal(err)
		}
		var aID, bID string
		if err := r.Run(ctx, func(p custodyPorts) error {
			if err := p.admissions.Fence(ctx, s); err != nil {
				return err
			}
			aID = custodyAdmit(t, p, s, events.OwnerRetail, 1)
			bID = custodyAdmit(t, p, s, events.OwnerCommerce, 1)
			return write(p, s, 1, false, pending(t, s.AggregateID(), 2, 2))
		}); err != nil {
			t.Fatal(err)
		}
		holdID := uuid.NewString()
		change := func(p custodyPorts, id, prior string, action outbox.AdmissionAction, h int64) error {
			return p.admissions.Change(ctx, s, outbox.SourceAdmissionChange{ID: id, PriorID: prior, Action: action, Checkpoint: revision(h), AuthorityRef: "synthetic-change", EvidenceRef: "synthetic-evidence", RetainedWorkAuthorityRef: "synthetic-release"})
		}
		if err := r.Run(ctx, func(p custodyPorts) error {
			if err := p.admissions.Fence(ctx, s); err != nil {
				return err
			}
			return change(p, holdID, aID, outbox.AdmissionHold, 2)
		}); err != nil {
			t.Fatal(err)
		}
		if err := r.Run(ctx, func(p custodyPorts) error {
			if err := p.admissions.Fence(ctx, s); err != nil {
				return err
			}
			return write(p, s, 2, false, pending(t, s.AggregateID(), 3, 3))
		}); !errors.Is(err, outbox.ErrAdmissionHeld) {
			t.Fatal(err)
		}
		if err := r.Run(ctx, func(p custodyPorts) error {
			if err := p.admissions.Fence(ctx, s); err != nil {
				return err
			}
			resume := uuid.NewString()
			if err := change(p, resume, holdID, outbox.AdmissionResume, 2); err != nil {
				return err
			}
			if err := change(p, uuid.NewString(), bID, outbox.AdmissionClose, 2); err != nil {
				return err
			}
			return write(p, s, 2, false, pending(t, s.AggregateID(), 3, 3))
		}); err != nil {
			t.Fatal(err)
		}
		var rows []struct {
			IntegrationSequence int64
			Destination         string
			HoldRef             *string
		}
		if err := db.Raw(`SELECT m.integration_sequence,d.destination,d.hold_ref FROM eventstore.outbox_messages m JOIN eventstore.outbox_deliveries d USING(event_id) WHERE aggregate_id=?::uuid ORDER BY integration_sequence,destination`, s.AggregateID()).Scan(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if len(rows) != 3 || rows[0].IntegrationSequence != 2 || rows[1].IntegrationSequence != 2 || rows[2].IntegrationSequence != 3 || rows[2].Destination != "retail" {
			t.Fatalf("range drain/plan error %+v", rows)
		}
		for _, row := range rows {
			if row.HoldRef != nil {
				t.Fatal("held after explicit resume")
			}
		}
	})
	t.Run("concurrent admission fences append before resolving complete set", func(t *testing.T) {
		s := custodyStream(t, uuid.NewString())
		ready := make(chan struct{})
		release := make(chan struct{})
		first := make(chan error, 1)
		second := make(chan error, 1)
		go func() {
			first <- r.Run(ctx, func(p custodyPorts) error {
				if err := p.admissions.Fence(ctx, s); err != nil {
					return err
				}
				custodyAdmit(t, p, s, events.OwnerRetail, 0)
				close(ready)
				<-release
				return nil
			})
		}()
		select {
		case <-ready:
		case err := <-first:
			t.Fatal(err)
		case <-time.After(5 * time.Second):
			t.Fatal("admission start timeout")
		}
		go func() {
			second <- r.Run(ctx, func(p custodyPorts) error {
				if err := p.admissions.Fence(ctx, s); err != nil {
					return err
				}
				return write(p, s, 0, false, pending(t, s.AggregateID(), 1, 1))
			})
		}()
		select {
		case err := <-second:
			close(release)
			t.Fatal("writer bypassed admission fence", err)
		case <-time.After(100 * time.Millisecond):
		}
		close(release)
		if err := <-first; err != nil {
			t.Fatal(err)
		}
		if err := <-second; err != nil {
			t.Fatal(err)
		}
		assertCounts(s.AggregateID(), 1, 1)
	})
	t.Run("cached plan is invalidated and writer resolves the complete new plan", func(t *testing.T) {
		s := custodyStream(t, uuid.NewString())
		e := pending(t, s.AggregateID(), 1, 1)
		env, _ := e.IntegrationEnvelope()
		if err := r.Run(ctx, func(p custodyPorts) error {
			if err := p.admissions.Fence(ctx, s); err != nil {
				return err
			}
			custodyAdmit(t, p, s, events.OwnerRetail, 0)
			if err := p.append.Append(ctx, eventstore.Batch{Events: []eventstore.Event{e}}); err != nil {
				return err
			}
			plan, err := p.admissions.Resolve(ctx, custodyRule(t, s, false), env)
			if err != nil {
				return err
			}
			// A hold added after resolution invalidates even an otherwise sealed
			// candidate. The writer cannot accept that stale object as an input.
			prior := plan.Recipients()[0].AdmissionID
			if err := p.admissions.Change(ctx, s, outbox.SourceAdmissionChange{ID: uuid.NewString(), PriorID: prior, Action: outbox.AdmissionHold, Checkpoint: revision(1), AuthorityRef: "synthetic-change", EvidenceRef: "synthetic-hold"}); err != nil {
				return err
			}
			if err := p.admissions.ValidatePlan(ctx, plan); !errors.Is(err, outbox.ErrAdmissionConflict) {
				t.Fatal("stale plan not rejected", err)
			}
			return p.insert.InsertMessages(ctx, custodyMessage(t, e, custodyRule(t, s, false)))
		}); !errors.Is(err, outbox.ErrAdmissionHeld) {
			t.Fatal("writer used stale plan", err)
		}
		assertCounts(s.AggregateID(), 0, 0)
	})
	t.Run("missing second child rolls parent first child event and guard back", func(t *testing.T) {
		if out, err := admin(`CREATE FUNCTION eventstore.fixture_skip_child() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.destination='retail' THEN RETURN NULL; END IF; RETURN NEW; END $$; CREATE TRIGGER fixture_skip_child BEFORE INSERT ON eventstore.outbox_deliveries FOR EACH ROW EXECUTE FUNCTION eventstore.fixture_skip_child();`); err != nil {
			t.Fatal(err, out)
		}
		defer func() {
			if out, err := admin(`DROP TRIGGER fixture_skip_child ON eventstore.outbox_deliveries; DROP FUNCTION eventstore.fixture_skip_child();`); err != nil {
				t.Error(err, out)
			}
		}()
		s := custodyStream(t, uuid.NewString())
		e := pending(t, s.AggregateID(), 1, 1)
		env, _ := e.IntegrationEnvelope()
		if err := r.Run(ctx, func(p custodyPorts) error {
			if err := p.admissions.Fence(ctx, s); err != nil {
				return err
			}
			custodyAdmit(t, p, s, events.OwnerInventory, 0)
			custodyAdmit(t, p, s, events.OwnerRetail, 0)
			if err := p.tx.Exec("INSERT INTO eventstore.fixture_guard(aggregate_id) VALUES(?::uuid)", s.AggregateID()).Error; err != nil {
				return err
			}
			return write(p, s, 0, false, e)
		}); !errors.Is(err, outbox.ErrInvalidRecord) {
			t.Fatal("missing child not detected", err)
		}
		assertCounts(s.AggregateID(), 0, 0)
		var n int64
		if err := db.Raw("SELECT count(*) FROM eventstore.outbox_deliveries WHERE event_id=?::uuid", env.EventID()).Scan(&n).Error; err != nil || n != 0 || count("fixture_guard", s.AggregateID()) != 0 {
			t.Fatal("partial child/guard", n, err)
		}
	})
	t.Run("separate SQL transactions cannot share an admission adapter", func(t *testing.T) {
		a, b := db.Begin(), db.Begin()
		defer a.Rollback()
		defer b.Rollback()
		ad, err := outbox.NewSourceAdmissions(a, events.OwnerInventory)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := outbox.NewCustodyInserter(b, events.OwnerInventory, ad); !errors.Is(err, outbox.ErrInvalidAdmission) {
			t.Fatal("cross transaction binding", err)
		}
	})
	t.Run("commit and rollback lost replies keep authoritative durability", func(t *testing.T) {
		pool, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		for _, commit := range []bool{false, true} {
			s := custodyStream(t, uuid.NewString())
			e := pending(t, s.AggregateID(), 1, 1)
			fault, err := gorm.Open(postgres.New(postgres.Config{Conn: insertFaultPool{pool, commit}}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			err = custodyRunner(t, fault).Run(ctx, func(p custodyPorts) error {
				calls++
				if err := p.admissions.Fence(ctx, s); err != nil {
					return err
				}
				custodyAdmit(t, p, s, events.OwnerRetail, 0)
				if err := p.tx.Exec("INSERT INTO eventstore.fixture_guard(aggregate_id) VALUES(?::uuid)", s.AggregateID()).Error; err != nil {
					return err
				}
				return write(p, s, 0, false, e)
			})
			if !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) || calls != 1 {
				t.Fatal("unknown commit retried/reported", err, calls)
			}
			want := int64(0)
			if commit {
				want = 1
			}
			assertCounts(s.AggregateID(), want, want)
			if count("fixture_guard", s.AggregateID()) != want {
				t.Fatal("guard split from message")
			}
		}
	})
	t.Run("runtime cannot change immutable plan hash routes or add children", func(t *testing.T) {
		s := custodyStream(t, uuid.NewString())
		e := pending(t, s.AggregateID(), 1, 1)
		env, _ := e.IntegrationEnvelope()
		if err := r.Run(ctx, func(p custodyPorts) error {
			if err := p.admissions.Fence(ctx, s); err != nil {
				return err
			}
			custodyAdmit(t, p, s, events.OwnerRetail, 0)
			return write(p, s, 0, false, e)
		}); err != nil {
			t.Fatal(err)
		}
		for _, q := range []string{"UPDATE eventstore.outbox_messages SET recipients='[]'::jsonb WHERE event_id=?::uuid", "UPDATE eventstore.outbox_messages SET envelope='{}'::bytea,envelope_hash=sha256('{}'::bytea) WHERE event_id=?::uuid", "UPDATE eventstore.outbox_deliveries SET routing_key='inventory.retail.fixture.changed.v1' WHERE event_id=?::uuid", "DELETE FROM eventstore.outbox_deliveries WHERE event_id=?::uuid"} {
			if err := db.Exec(q, env.EventID()).Error; err == nil {
				t.Fatal("immutable mutation", q)
			}
		}
		if err := db.Exec(`INSERT INTO eventstore.outbox_deliveries(event_id,destination,admission_id,exchange,routing_key) SELECT event_id,'commerce',admission_id,exchange,'inventory.commerce.inventory.fixture.changed.v1' FROM eventstore.outbox_deliveries WHERE event_id=?::uuid`, env.EventID()).Error; err == nil {
			t.Fatal("extra recipient accepted")
		}
		var n int64
		if err := db.Raw("SELECT count(*) FROM eventstore.outbox_deliveries WHERE event_id=?::uuid", env.EventID()).Scan(&n).Error; err != nil || n != 1 {
			t.Fatal("changed complete recipient set", n, err)
		}
	})
	t.Run("missing or corrupt route marker rejects even no-op writes", func(t *testing.T) {
		// Migration-owner fault injection affects only this disposable database.
		// Restore the exact preimage after each committed corruption probe.
		for _, mutation := range []string{"ALTER TABLE eventstore.messaging_route_compatibility DISABLE TRIGGER USER; DELETE FROM eventstore.messaging_route_compatibility", "ALTER TABLE eventstore.messaging_route_compatibility DISABLE TRIGGER USER; UPDATE eventstore.messaging_route_compatibility SET correction_migration_sha256=sha256('wrong'::bytea)"} {
			// A separate committed disposable corruption is restored from the
			// exact preimage by migration authority immediately after the probe.
			out, err := admin(`BEGIN; CREATE TEMP TABLE saved_marker AS TABLE eventstore.messaging_route_compatibility; ` + mutation + `; ALTER TABLE eventstore.messaging_route_compatibility ENABLE TRIGGER USER; SELECT encode(convert_to(row_to_json(s)::text,'UTF8'),'hex') FROM saved_marker s; COMMIT;`)
			if err != nil {
				t.Fatal(err, out)
			}
			if err := r.Run(ctx, func(p custodyPorts) error { return p.insert.InsertMessages(ctx) }); !errors.Is(err, outbox.ErrMessagingMode) {
				t.Fatal("bad marker accepted", err)
			}
			if out, err := admin(`ALTER TABLE eventstore.messaging_route_compatibility DISABLE TRIGGER USER; DELETE FROM eventstore.messaging_route_compatibility; INSERT INTO eventstore.messaging_route_compatibility SELECT * FROM json_populate_record(NULL::eventstore.messaging_route_compatibility,convert_from(decode('` + out + `','hex'),'UTF8')::json); ALTER TABLE eventstore.messaging_route_compatibility ENABLE TRIGGER USER;`); err != nil {
				t.Fatal(err, out)
			}
		}
	})
}

type insertFaultPool struct {
	*sql.DB
	commit bool
}

func (p insertFaultPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &insertFaultTx{tx, p.commit}, nil
}

type insertFaultTx struct {
	*sql.Tx
	commit bool
}

func (t insertFaultTx) Commit() error {
	var err error
	if t.commit {
		err = t.Tx.Commit()
	} else {
		err = t.Tx.Rollback()
	}
	if err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}

// Only this randomly named disposable container is touched. The fixture cannot
// accept an existing DSN, uses a random loopback port and narrow runtime login.
func database(t *testing.T, setups ...func(func(string, ...string) (string, error))) *gorm.DB {
	t.Helper()
	name := "justixauto-t011-" + uuid.NewString()
	password := "synthetic-" + uuid.NewString()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	image := "docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"
	if out, err := exec.CommandContext(ctx, "docker", "run", "-d", "--name", name, "-e", "POSTGRES_PASSWORD="+password, "-e", "POSTGRES_DB=justix_inventory", "-p", "127.0.0.1::5432", image).CombinedOutput(); err != nil {
		t.Fatalf("fixture start: %v %s", err, out)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "docker", "rm", "-f", name).CombinedOutput(); err != nil {
			t.Errorf("fixture cleanup: %v %s", err, out)
		}
	})
	sql := func(query string, args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", append([]string{"exec", "-i", name, "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "justix_inventory"}, args...)...)
		cmd.Stdin = strings.NewReader(query)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	for {
		if _, err := sql("SELECT 1"); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("fixture startup timeout")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if got, err := sql("SHOW server_version_num"); err != nil || got != "180006" {
		t.Fatalf("version %s: %v", got, err)
	}
	if out, err := sql("CREATE ROLE justix_inventory_runtime LOGIN PASSWORD '" + password + "'"); err != nil {
		t.Fatalf("runtime role: %v %s", err, out)
	}
	schema, err := os.ReadFile("../eventstore/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := sql(string(schema), "-v", "owner_service=inventory", "-v", "runtime_role=justix_inventory_runtime"); err != nil {
		t.Fatalf("schema: %v %s", err, out)
	}
	if out, err := sql("CREATE TABLE eventstore.fixture_guard (aggregate_id uuid PRIMARY KEY); GRANT SELECT,INSERT ON eventstore.fixture_guard TO justix_inventory_runtime"); err != nil {
		t.Fatalf("guard: %v %s", err, out)
	}
	for _, setup := range setups {
		setup(sql)
	}
	out, err := exec.CommandContext(ctx, "docker", "port", name, "5432/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	address := strings.TrimSpace(string(out))
	if !strings.HasPrefix(address, "127.0.0.1:") || strings.Contains(address, "\n") {
		t.Fatalf("unexpected binding %q", address)
	}
	dsn := fmt.Sprintf("host=127.0.0.1 port=%s user=justix_inventory_runtime password=%s dbname=justix_inventory sslmode=disable", strings.TrimPrefix(address, "127.0.0.1:"), password)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(8)
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}
