package inbox_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
	"justixauto/pkg/inbox"
)

type payload struct {
	Ref string `json:"ref"`
}

func schema(t *testing.T) events.EventSchema[payload] {
	t.Helper()
	s, err := events.NewEventSchema("inventory.fixture.changed.v1", 1, "fixture", events.CompanyScopeTenantRequired, func(p payload) error {
		if p.Ref == "" {
			return errors.New("missing safe reference")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func message(t *testing.T, id, stream, company string, sequence int64) []byte {
	t.Helper()
	rev, err := events.NewRevision(sequence)
	if err != nil {
		t.Fatal(err)
	}
	e, err := events.NewEnvelope(events.EnvelopeInput{
		EventID: id, AggregateID: stream, CompanyID: &company,
		AggregateVersion: rev, IntegrationSequence: rev, OccurredAt: time.Now().UTC(),
		Actor:         events.Actor{Kind: events.ActorService, ID: uuid.NewString()},
		CorrelationID: uuid.NewString(), CausationID: uuid.NewString(), OperationID: uuid.NewString(),
	}, schema(t), payload{Ref: "synthetic-reference"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func validator(t *testing.T, company string) func(events.Envelope) error {
	t.Helper()
	s := schema(t)
	return func(e events.Envelope) error {
		if _, err := events.DecodeData(e, s); err != nil {
			return err
		}
		if id, ok := e.CompanyID(); !ok || id != company {
			return errors.New("fixture scope denied")
		}
		return nil
	}
}

func TestConsumerValidationBeforePersistence(t *testing.T) {
	bind := func(*gorm.DB) (struct{}, error) { t.Fatal("bound database for invalid input"); return struct{}{}, nil }
	validate := validator(t, uuid.NewString())
	for _, name := range []string{"", " \t"} {
		if _, err := inbox.NewConsumer(&gorm.DB{}, name, bind, validate); err == nil {
			t.Fatal("empty consumer")
		}
	}
	if _, err := inbox.NewConsumer[struct{}](&gorm.DB{}, "fixture", nil, validate); err == nil {
		t.Fatal("nil factory")
	}
	if _, err := inbox.NewConsumer(&gorm.DB{}, "fixture", bind, nil); err == nil {
		t.Fatal("nil validator")
	}
	if _, err := inbox.NewConsumer(nil, "fixture", bind, validate); err == nil {
		t.Fatal("nil database")
	}
	c, err := inbox.NewConsumer(&gorm.DB{}, "fixture", bind, validate)
	if err != nil {
		t.Fatal(err)
	}
	apply := func(struct{}, events.Envelope) error { t.Fatal("invalid input applied"); return nil }
	ack := func() error { t.Fatal("invalid input acknowledged"); return nil }
	var zero inbox.Consumer[struct{}]
	if _, err := zero.Consume(context.Background(), nil, apply, ack); !errors.Is(err, inbox.ErrInvalidConsumer) {
		t.Fatal(err)
	}
	if _, err := c.Consume(context.Background(), nil, nil, ack); !errors.Is(err, inbox.ErrInvalidConsumer) {
		t.Fatal(err)
	}
	if _, err := c.Consume(context.Background(), nil, apply, nil); !errors.Is(err, inbox.ErrInvalidConsumer) {
		t.Fatal(err)
	}
	validButUnauthorized := message(t, uuid.NewString(), uuid.NewString(), uuid.NewString(), 1)
	for _, body := range [][]byte{nil, []byte("null"), []byte("{}"), []byte("[]"), []byte(`{"eventId":"not-a-uuid"}`), validButUnauthorized} {
		if outcome, err := c.Consume(context.Background(), body, apply, ack); err == nil || outcome != inbox.Unconfirmed {
			t.Fatalf("accepted input: %v %v", outcome, err)
		}
	}
	denied := errors.New("unsupported schema")
	c, _ = inbox.NewConsumer(&gorm.DB{}, "fixture", bind, func(events.Envelope) error { return denied })
	if _, err := c.Consume(context.Background(), validButUnauthorized, apply, ack); !errors.Is(err, denied) {
		t.Fatal(err)
	}
}

// Only this typed port reaches the effect callback. The synthetic owner tables
// model checkpoint and projection writes; production owner schema is not added.
type effects interface {
	Apply(context.Context, events.Envelope) error
}
type effectAdapter struct {
	tx       *gorm.DB
	consumer string
}

var errGap = errors.New("fixture sequence gap")

func (p effectAdapter) Apply(ctx context.Context, e events.Envelope) error {
	tx := p.tx.WithContext(ctx)
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", p.consumer+":"+e.AggregateID()).Error; err != nil {
		return err
	}
	var row struct{ Position int64 }
	if err := tx.Raw("SELECT position FROM fixture.checkpoints WHERE consumer_name=? AND aggregate_id=?::uuid", p.consumer, e.AggregateID()).Scan(&row).Error; err != nil {
		return err
	}
	sequence := e.IntegrationSequence().Int64()
	if sequence != row.Position+1 {
		return errGap
	}
	if err := tx.Exec("INSERT INTO fixture.effects (consumer_name,event_id) VALUES (?,?::uuid)", p.consumer, e.EventID()).Error; err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO fixture.checkpoints (consumer_name,aggregate_id,position) VALUES (?,?::uuid,?)
		ON CONFLICT (consumer_name,aggregate_id) DO UPDATE SET position=excluded.position`, p.consumer, e.AggregateID(), sequence).Error
}

func consumer(t *testing.T, db *gorm.DB, name, company string) *inbox.Consumer[effects] {
	t.Helper()
	c, err := inbox.NewConsumer(db, name, func(tx *gorm.DB) (effects, error) { return effectAdapter{tx: tx, consumer: name}, nil }, validator(t, company))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestInboxPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_INBOX") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_INBOX=1 for disposable PostgreSQL inbox tests")
	}
	db := database(t)
	ctx := context.Background()
	company := uuid.NewString()
	apply := func(u effects, e events.Envelope) error { return u.Apply(ctx, e) }
	assertState := func(t *testing.T, name, id, stream string, count, position int64) {
		t.Helper()
		for _, table := range []string{"eventstore.inbox", "fixture.effects"} {
			var n int64
			if err := db.Raw("SELECT count(*) FROM "+table+" WHERE consumer_name=? AND event_id=?::uuid", name, id).Scan(&n).Error; err != nil {
				t.Fatal(err)
			}
			if n != count {
				t.Fatalf("%s=%d want %d", table, n, count)
			}
		}
		var got int64
		if err := db.Raw("SELECT coalesce(max(position),0) FROM fixture.checkpoints WHERE consumer_name=? AND aggregate_id=?::uuid", name, stream).Scan(&got).Error; err != nil {
			t.Fatal(err)
		}
		if got != position {
			t.Fatalf("checkpoint=%d want %d", got, position)
		}
	}
	noAck := func() error { t.Error("acknowledged unsuccessful transaction"); return nil }

	t.Run("commit before acknowledgment and exact duplicate", func(t *testing.T) {
		name, id, stream := "commit", uuid.NewString(), uuid.NewString()
		body := message(t, id, stream, company, 1)
		c := consumer(t, db, name, company)
		ackCount := 0
		ack := func() error { ackCount++; assertState(t, name, id, stream, 1, 1); return nil }
		if got, err := c.Consume(ctx, body, apply, ack); got != inbox.Applied || err != nil {
			t.Fatal(got, err)
		}
		if got, err := c.Consume(ctx, body, func(effects, events.Envelope) error { t.Fatal("duplicate effect"); return nil }, ack); got != inbox.Duplicate || err != nil {
			t.Fatal(got, err)
		}
		if ackCount != 2 {
			t.Fatal(ackCount)
		}
		// Another generation/consumer processes the same delivery independently.
		other := consumer(t, db, "commit-generation-2", company)
		if got, err := other.Consume(ctx, body, apply, func() error { return nil }); got != inbox.Applied || err != nil {
			t.Fatal(got, err)
		}
		assertState(t, "commit-generation-2", id, stream, 1, 1)
	})

	t.Run("concurrent duplicates", func(t *testing.T) {
		name, id, stream := "concurrent", uuid.NewString(), uuid.NewString()
		body := message(t, id, stream, company, 1)
		c := consumer(t, db, name, company)
		var calls, acks atomic.Int32
		start := make(chan struct{})
		type result struct {
			outcome inbox.Outcome
			err     error
		}
		results := make(chan result, 8)
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				<-start
				got, err := c.Consume(ctx, body, func(u effects, e events.Envelope) error { calls.Add(1); return u.Apply(ctx, e) }, func() error { acks.Add(1); return nil })
				results <- result{got, err}
			})
		}
		close(start)
		wg.Wait()
		close(results)
		fresh, duplicates := 0, 0
		for r := range results {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.outcome == inbox.Applied {
				fresh++
			} else if r.outcome == inbox.Duplicate {
				duplicates++
			}
		}
		if fresh != 1 || duplicates != 7 || calls.Load() != 1 || acks.Load() != 8 {
			t.Fatalf("fresh=%d duplicate=%d calls=%d ack=%d", fresh, duplicates, calls.Load(), acks.Load())
		}
		assertState(t, name, id, stream, 1, 1)
	})

	t.Run("failed checkpoint and effect roll back then retry", func(t *testing.T) {
		name, id, stream := "rollback", uuid.NewString(), uuid.NewString()
		body := message(t, id, stream, company, 1)
		c := consumer(t, db, name, company)
		failed := errors.New("effect rejected after checkpoint write")
		got, err := c.Consume(ctx, body, func(u effects, e events.Envelope) error {
			if err := u.Apply(ctx, e); err != nil {
				return err
			}
			return failed
		}, noAck)
		if got != inbox.Unconfirmed || !errors.Is(err, failed) {
			t.Fatal(got, err)
		}
		assertState(t, name, id, stream, 0, 0)
		if got, err := c.Consume(ctx, body, apply, func() error { return nil }); got != inbox.Applied || err != nil {
			t.Fatal(got, err)
		}
		assertState(t, name, id, stream, 1, 1)
	})

	t.Run("concurrent duplicate waits through rollback", func(t *testing.T) {
		name, id, stream := "rollback-waiter", uuid.NewString(), uuid.NewString()
		body := message(t, id, stream, company, 1)
		c := consumer(t, db, name, company)
		entered, release := make(chan struct{}), make(chan struct{})
		first, second := make(chan error, 1), make(chan error, 1)
		failed := errors.New("abort winner")
		go func() {
			_, err := c.Consume(ctx, body, func(u effects, e events.Envelope) error {
				if err := u.Apply(ctx, e); err != nil {
					return err
				}
				close(entered)
				<-release
				return failed
			}, noAck)
			first <- err
		}()
		<-entered
		go func() {
			got, err := c.Consume(ctx, body, apply, func() error { return nil })
			if err == nil && got != inbox.Applied {
				err = fmt.Errorf("waiter outcome %v", got)
			}
			second <- err
		}()
		select {
		case err := <-second:
			t.Fatalf("waiter escaped held transaction: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
		assertState(t, name, id, stream, 0, 0)
		close(release)
		if err := <-first; !errors.Is(err, failed) {
			t.Fatal(err)
		}
		if err := <-second; err != nil {
			t.Fatal(err)
		}
		assertState(t, name, id, stream, 1, 1)
	})

	t.Run("gap never consumes future event", func(t *testing.T) {
		name, stream := "gap", uuid.NewString()
		first, second := uuid.NewString(), uuid.NewString()
		c := consumer(t, db, name, company)
		future := message(t, second, stream, company, 2)
		if got, err := c.Consume(ctx, future, apply, noAck); got != inbox.Unconfirmed || !errors.Is(err, errGap) {
			t.Fatal(got, err)
		}
		assertState(t, name, second, stream, 0, 0)
		if _, err := c.Consume(ctx, message(t, first, stream, company, 1), apply, func() error { return nil }); err != nil {
			t.Fatal(err)
		}
		if got, err := c.Consume(ctx, future, apply, func() error { return nil }); got != inbox.Applied || err != nil {
			t.Fatal(got, err)
		}
		assertState(t, name, second, stream, 1, 2)
	})

	t.Run("changed envelope is not a duplicate", func(t *testing.T) {
		name, id, stream := "hash", uuid.NewString(), uuid.NewString()
		body := message(t, id, stream, company, 1)
		c := consumer(t, db, name, company)
		if _, err := c.Consume(ctx, body, apply, func() error { return nil }); err != nil {
			t.Fatal(err)
		}
		for _, changed := range [][]byte{bytes.Replace(body, []byte("synthetic-reference"), []byte("different-safe-reference"), 1), append(bytes.Clone(body), ' ')} {
			if got, err := c.Consume(ctx, changed, apply, noAck); got != inbox.Unconfirmed || !errors.Is(err, inbox.ErrEnvelopeConflict) {
				t.Fatal(got, err)
			}
		}
		assertState(t, name, id, stream, 1, 1)
	})

	t.Run("validation precedes duplicate lookup and retains received bytes", func(t *testing.T) {
		name, id, stream := "validation", uuid.NewString(), uuid.NewString()
		body := message(t, id, stream, company, 1)
		original := bytes.Clone(body)
		check := validator(t, company)
		c, err := inbox.NewConsumer(db, name, func(tx *gorm.DB) (effects, error) {
			return effectAdapter{tx: tx, consumer: name}, nil
		}, func(e events.Envelope) error {
			// Mutation after entry must not change the committed envelope hash.
			body[0] = ' '
			return check(e)
		})
		if err != nil {
			t.Fatal(err)
		}
		if got, err := c.Consume(ctx, body, apply, func() error { return nil }); got != inbox.Applied || err != nil {
			t.Fatal(got, err)
		}
		if got, err := consumer(t, db, name, company).Consume(ctx, original, apply, func() error { return nil }); got != inbox.Duplicate || err != nil {
			t.Fatal(got, err)
		}
		denied := errors.New("subscription no longer permitted")
		c, err = inbox.NewConsumer(db, name, func(*gorm.DB) (effects, error) {
			t.Error("database bound before validation")
			return nil, denied
		}, func(events.Envelope) error { return denied })
		if err != nil {
			t.Fatal(err)
		}
		if got, err := c.Consume(ctx, original, apply, noAck); got != inbox.Unconfirmed || !errors.Is(err, denied) {
			t.Fatal(got, err)
		}
		assertState(t, name, id, stream, 1, 1)
	})

	t.Run("acknowledgment loss redelivers without effect", func(t *testing.T) {
		name, id, stream := "ack-loss", uuid.NewString(), uuid.NewString()
		body := message(t, id, stream, company, 1)
		c := consumer(t, db, name, company)
		got, err := c.Consume(ctx, body, apply, func() error { assertState(t, name, id, stream, 1, 1); return io.ErrUnexpectedEOF })
		if got != inbox.Applied || !errors.Is(err, inbox.ErrAcknowledgment) || !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatal(got, err)
		}
		if got, err := c.Consume(ctx, body, func(effects, events.Envelope) error { t.Error("repeated effect"); return nil }, func() error { return nil }); got != inbox.Duplicate || err != nil {
			t.Fatal(got, err)
		}
	})

	t.Run("lost commit reply before and after server commit", func(t *testing.T) {
		for _, commitFirst := range []bool{false, true} {
			name, id, stream := fmt.Sprintf("commit-loss-%v", commitFirst), uuid.NewString(), uuid.NewString()
			body := message(t, id, stream, company, 1)
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			faultDB, err := gorm.Open(postgres.New(postgres.Config{Conn: commitFaultPool{DB: sqlDB, commitFirst: commitFirst}}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			got, err := consumer(t, faultDB, name, company).Consume(ctx, body, func(u effects, e events.Envelope) error { calls++; return u.Apply(ctx, e) }, noAck)
			if got != inbox.Unconfirmed || !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) || !errors.Is(err, io.ErrUnexpectedEOF) || calls != 1 {
				t.Fatal(got, err, calls)
			}
			want := int64(0)
			expected := inbox.Applied
			if commitFirst {
				want = 1
				expected = inbox.Duplicate
			}
			assertState(t, name, id, stream, want, want)
			if got, err := consumer(t, db, name, company).Consume(ctx, body, apply, func() error { return nil }); got != expected || err != nil {
				t.Fatal(got, err)
			}
			assertState(t, name, id, stream, 1, 1)
		}
	})

	t.Run("panic and cancellation leave no receipt", func(t *testing.T) {
		for _, panicFirst := range []bool{false, true} {
			name, id, stream := fmt.Sprintf("interrupted-%v", panicFirst), uuid.NewString(), uuid.NewString()
			body := message(t, id, stream, company, 1)
			c := consumer(t, db, name, company)
			cancelCtx, cancel := context.WithCancel(ctx)
			func() {
				defer func() {
					if panicFirst && recover() != "fixture panic" {
						t.Error("panic not propagated")
					}
				}()
				got, err := c.Consume(cancelCtx, body, func(u effects, e events.Envelope) error {
					if err := u.Apply(cancelCtx, e); err != nil {
						return err
					}
					if panicFirst {
						panic("fixture panic")
					}
					cancel()
					return nil
				}, noAck)
				if got != inbox.Unconfirmed || !errors.Is(err, context.Canceled) {
					t.Error(got, err)
				}
			}()
			cancel()
			assertState(t, name, id, stream, 0, 0)
			if got, err := c.Consume(ctx, body, apply, func() error { return nil }); got != inbox.Applied || err != nil {
				t.Fatal(got, err)
			}
		}
	})
}

type commitFaultPool struct {
	*sql.DB
	commitFirst bool
}

func (p commitFaultPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &commitFaultTx{Tx: tx, commitFirst: p.commitFirst}, nil
}

type commitFaultTx struct {
	*sql.Tx
	commitFirst bool
}

func (tx commitFaultTx) Commit() error {
	var err error
	if tx.commitFirst {
		err = tx.Tx.Commit()
	} else {
		err = tx.Tx.Rollback()
	}
	if err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}

func database(t *testing.T) *gorm.DB {
	t.Helper()
	const image = "docker.io/library/postgres@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"
	name, password := "justixauto-t013-"+uuid.NewString(), "synthetic-"+uuid.NewString()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "docker", "run", "-d", "--name", name, "-e", "POSTGRES_PASSWORD="+password, "-e", "POSTGRES_DB=justix_inventory", "-p", "127.0.0.1::5432", image).CombinedOutput(); err != nil {
		t.Fatalf("start owned fixture: %v %s", err, out)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "docker", "rm", "-f", name).CombinedOutput(); err != nil {
			t.Errorf("remove owned fixture: %v %s", err, out)
		}
	})
	query := func(statement string, args ...string) (string, error) {
		argv := []string{"exec", "-i", name, "psql", "-XAt", "-U", "postgres", "-d", "justix_inventory", "-v", "ON_ERROR_STOP=1"}
		argv = append(argv, args...)
		cmd := exec.CommandContext(ctx, "docker", argv...)
		cmd.Stdin = strings.NewReader(statement)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	for {
		if _, err := query("SELECT 1"); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("fixture startup timeout")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if out, err := query("SHOW server_version_num"); err != nil || out != "180006" {
		t.Fatal(out, err)
	}
	if out, err := query("CREATE ROLE justix_inventory_runtime LOGIN PASSWORD '" + password + "'"); err != nil {
		t.Fatal(out, err)
	}
	schema, err := os.ReadFile("../eventstore/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := query(string(schema), "-v", "owner_service=inventory", "-v", "runtime_role=justix_inventory_runtime"); err != nil {
		t.Fatal(out, err)
	}
	if out, err := query(`CREATE SCHEMA fixture;
		CREATE TABLE fixture.effects (consumer_name text NOT NULL,event_id uuid NOT NULL,PRIMARY KEY(consumer_name,event_id));
		CREATE TABLE fixture.checkpoints (consumer_name text NOT NULL,aggregate_id uuid NOT NULL,position bigint NOT NULL,PRIMARY KEY(consumer_name,aggregate_id));
		GRANT USAGE ON SCHEMA fixture TO justix_inventory_runtime;
		GRANT SELECT,INSERT ON fixture.effects TO justix_inventory_runtime;
		GRANT SELECT,INSERT,UPDATE ON fixture.checkpoints TO justix_inventory_runtime;`); err != nil {
		t.Fatal(out, err)
	}
	out, err := exec.CommandContext(ctx, "docker", "port", name, "5432/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	address := strings.TrimSpace(string(out))
	if !strings.HasPrefix(address, "127.0.0.1:") || strings.Contains(address, "\n") {
		t.Fatalf("unexpected fixture binding %q", address)
	}
	dsn := fmt.Sprintf("host=127.0.0.1 port=%s user=justix_inventory_runtime password=%s dbname=justix_inventory sslmode=disable", strings.TrimPrefix(address, "127.0.0.1:"), password)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(12)
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}
