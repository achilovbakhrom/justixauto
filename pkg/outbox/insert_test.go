package outbox_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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

// Only this randomly named disposable container is touched. The fixture cannot
// accept an existing DSN, uses a random loopback port and narrow runtime login.
func database(t *testing.T) *gorm.DB {
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
