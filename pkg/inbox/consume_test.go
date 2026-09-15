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
	t.Run("custody cannot use a legacy-only installation", func(t *testing.T) {
		err := dispatchRunner(t, db, "missing-mode", validator(t, company)).Run(ctx, func(u dispatchUnit) error {
			got, err := u.Apply(ctx, message(t, uuid.NewString(), uuid.NewString(), company, 1), func(effects, events.Envelope) error { t.Error("missing custody schema applied"); return nil })
			if got != inbox.NoCandidate {
				t.Error(got)
			}
			return err
		})
		if !errors.Is(err, inbox.ErrMessagingMode) {
			t.Fatal(err)
		}
	})

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

func TestTransactionalConsumerRequiresTransaction(t *testing.T) {
	bind := func(*gorm.DB) (effects, error) { t.Fatal("invalid transaction bound"); return nil, nil }
	validate := validator(t, uuid.NewString())
	for _, db := range []*gorm.DB{nil, {}, {Error: errors.New("invalid database")}} {
		if _, err := inbox.NewTransactionalConsumer(db, "fixture", bind, validate); !errors.Is(err, eventstore.ErrTransactionRequired) {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"", " \t"} {
		if _, err := inbox.NewTransactionalConsumer(&gorm.DB{}, name, bind, validate); !errors.Is(err, inbox.ErrInvalidConsumer) {
			t.Fatal(err)
		}
	}
	if _, err := inbox.NewTransactionalConsumer[effects](&gorm.DB{}, "fixture", nil, validate); !errors.Is(err, inbox.ErrInvalidConsumer) {
		t.Fatal(err)
	}
	if _, err := inbox.NewTransactionalConsumer(&gorm.DB{}, "fixture", bind, nil); !errors.Is(err, inbox.ErrInvalidConsumer) {
		t.Fatal(err)
	}
	var zero inbox.TransactionalConsumer[effects]
	if got, err := zero.Apply(context.Background(), nil, func(effects, events.Envelope) error { return nil }); got != inbox.NoCandidate || !errors.Is(err, inbox.ErrInvalidConsumer) {
		t.Fatal(got, err)
	}
}

// dispatchUnit is a synthetic owner-local port, not the T-923 production job
// scheduler. It demonstrates that completion uses the SAME transaction as
// inbox, effect and checkpoint while neither callback exposes a SQL handle.
type dispatchUnit interface {
	Apply(context.Context, []byte, func(effects, events.Envelope) error) (inbox.Candidate, error)
	Complete(context.Context, string) error
}

type dispatchAdapter struct {
	*inbox.TransactionalConsumer[effects]
	tx   *gorm.DB
	name string
}

var errCompletion = errors.New("fixture final completion fence lost")

func (u dispatchAdapter) Complete(ctx context.Context, id string) error {
	write := u.tx.WithContext(ctx).Exec(`UPDATE fixture.jobs SET completed=true WHERE consumer_name=? AND event_id=?::uuid`, u.name, id)
	if write.Error != nil {
		return write.Error
	}
	if write.RowsAffected != 1 {
		return errCompletion
	}
	return nil
}

func dispatchRunner(t *testing.T, db *gorm.DB, name string, validate func(events.Envelope) error) *eventstore.Transactions[dispatchUnit] {
	t.Helper()
	runner, err := eventstore.NewTransactions(db, func(tx *gorm.DB) (dispatchUnit, error) {
		c, err := inbox.NewTransactionalConsumer(tx, name, func(tx *gorm.DB) (effects, error) {
			return effectAdapter{tx: tx, consumer: name}, nil
		}, validate)
		if err != nil {
			return nil, err
		}
		return dispatchAdapter{TransactionalConsumer: c, tx: tx, name: name}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func TestTransactionalInboxPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_INBOX") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_INBOX=1 for disposable PostgreSQL inbox tests")
	}
	db, sqlOwner := databaseWithMessaging(t, true)
	ctx, company := context.Background(), uuid.NewString()
	apply := func(u effects, e events.Envelope) error { return u.Apply(ctx, e) }
	noApply := func(effects, events.Envelope) error { t.Error("unexpected effect"); return nil }
	noAck := func() error { t.Error("mixed-mode acknowledgment"); return nil }
	mustOwner := func(sql string) {
		t.Helper()
		if out, err := sqlOwner(sql); err != nil {
			t.Fatal(out, err)
		}
	}
	seedJob := func(name, id string) {
		t.Helper()
		if err := db.Exec("INSERT INTO fixture.jobs(consumer_name,event_id) VALUES (?,?::uuid)", name, id).Error; err != nil {
			t.Fatal(err)
		}
	}
	assertState := func(name, id, stream string, count, position int64, completed bool) {
		t.Helper()
		for _, table := range []string{"eventstore.inbox", "fixture.effects"} {
			var n int64
			if err := db.Raw("SELECT count(*) FROM "+table+" WHERE consumer_name=? AND event_id=?::uuid", name, id).Scan(&n).Error; err != nil || n != count {
				t.Fatalf("%s count %d want %d: %v", table, n, count, err)
			}
		}
		var got int64
		if err := db.Raw("SELECT coalesce(max(position),0) FROM fixture.checkpoints WHERE consumer_name=? AND aggregate_id=?::uuid", name, stream).Scan(&got).Error; err != nil || got != position {
			t.Fatalf("checkpoint %d want %d: %v", got, position, err)
		}
		var done bool
		if err := db.Raw("SELECT coalesce(bool_or(completed),false) FROM fixture.jobs WHERE consumer_name=? AND event_id=?::uuid", name, id).Scan(&done).Error; err != nil || done != completed {
			t.Fatalf("completion %v want %v: %v", done, completed, err)
		}
	}
	run := func(db *gorm.DB, name string, body []byte, effect func(effects, events.Envelope) error, after func(dispatchUnit) error) (inbox.Candidate, error) {
		var candidate inbox.Candidate
		err := dispatchRunner(t, db, name, validator(t, company)).Run(ctx, func(u dispatchUnit) error {
			var err error
			candidate, err = u.Apply(ctx, body, effect)
			if err != nil {
				return err
			}
			return after(u)
		})
		return candidate, err
	}
	name, id, stream := "migrated", uuid.NewString(), uuid.NewString()
	body := message(t, id, stream, company, 1)
	legacy := consumer(t, db, name, company)
	if got, err := legacy.Consume(ctx, body, apply, func() error { return nil }); got != inbox.Applied || err != nil {
		t.Fatal(got, err)
	}
	if got, err := run(db, name, body, noApply, func(dispatchUnit) error { t.Error("legacy accepted custody"); return nil }); got != inbox.NoCandidate || !errors.Is(err, inbox.ErrMessagingMode) {
		t.Fatal(got, err)
	}
	// The fixture has no legacy outbox. This explicit migration-owner transition
	// supplies synthetic evidence only; it does not assert real runtime shutdown.
	mustOwner(fmt.Sprintf(`SELECT eventstore.activate_messaging_custody('%s',sha256('new'::bytea),sha256('old'::bytea),'synthetic-backup','synthetic-stopped','synthetic-broker-checkpoints','synthetic-compatibility')`, uuid.NewString()))

	t.Run("migrated duplicate completes its pending job without effects", func(t *testing.T) {
		seedJob(name, id)
		for _, b := range [][]byte{body, message(t, uuid.NewString(), uuid.NewString(), company, 1)} {
			if got, err := legacy.Consume(ctx, b, noApply, noAck); got != inbox.Unconfirmed || !errors.Is(err, inbox.ErrMessagingMode) {
				t.Fatal(got, err)
			}
		}
		rollback := errors.New("synthetic completion rollback")
		got, err := run(db, name, body, noApply, func(u dispatchUnit) error {
			if err := u.Complete(ctx, id); err != nil {
				return err
			}
			assertState(name, id, stream, 1, 1, false)
			return rollback
		})
		if got != inbox.DuplicateCandidate || !errors.Is(err, rollback) {
			t.Fatal(got, err)
		}
		assertState(name, id, stream, 1, 1, false)
		got, err = run(db, name, body, noApply, func(u dispatchUnit) error { return u.Complete(ctx, id) })
		if got != inbox.DuplicateCandidate || err != nil {
			t.Fatal(got, err)
		}
		assertState(name, id, stream, 1, 1, true)
		var marker string
		if err := db.Raw("SELECT coalesce(current_setting('justix.messaging_mode',true),'')").Scan(&marker).Error; err != nil || marker != "" {
			t.Fatal(marker, err)
		}
	})

	t.Run("fresh candidate rollback includes checkpoint and job completion", func(t *testing.T) {
		name, id, stream := "rollback-candidate", uuid.NewString(), uuid.NewString()
		body := message(t, id, stream, company, 1)
		seedJob(name, id)
		rollback := errors.New("outer caller rolls back")
		got, err := run(db, name, body, apply, func(u dispatchUnit) error {
			if err := u.Complete(ctx, id); err != nil {
				return err
			}
			assertState(name, id, stream, 0, 0, false)
			return rollback
		})
		if got != inbox.AppliedCandidate || !errors.Is(err, rollback) {
			t.Fatal(got, err)
		}
		assertState(name, id, stream, 0, 0, false)
		got, err = run(db, name, body, apply, func(u dispatchUnit) error { return u.Complete(ctx, uuid.NewString()) })
		if got != inbox.AppliedCandidate || !errors.Is(err, errCompletion) {
			t.Fatal(got, err)
		}
		assertState(name, id, stream, 0, 0, false)
		got, err = run(db, name, body, apply, func(u dispatchUnit) error { return u.Complete(ctx, id) })
		if got != inbox.AppliedCandidate || err != nil {
			t.Fatal(got, err)
		}
		assertState(name, id, stream, 1, 1, true)
	})

	t.Run("duplicates revalidate and compare exact bytes", func(t *testing.T) {
		denied := errors.New("current scope denied")
		for _, b := range [][]byte{body, []byte("{}"), bytes.Replace(body, []byte("synthetic-reference"), []byte("changed-reference"), 1), append(bytes.Clone(body), ' ')} {
			got, err := run(db, name, b, noApply, func(dispatchUnit) error {
				if !bytes.Equal(b, body) {
					t.Error("invalid duplicate accepted")
				}
				return nil
			})
			if bytes.Equal(b, body) {
				// The exact authorized duplicate is valid; validate denial below.
				if got != inbox.DuplicateCandidate || err != nil {
					t.Fatal(got, err)
				}
			} else if got != inbox.NoCandidate || err == nil {
				t.Fatal(got, err)
			}
		}
		err := dispatchRunner(t, db, name, func(events.Envelope) error { return denied }).Run(ctx, func(u dispatchUnit) error {
			got, err := u.Apply(ctx, body, noApply)
			if got != inbox.NoCandidate {
				t.Error(got)
			}
			return err
		})
		if !errors.Is(err, denied) {
			t.Fatal(err)
		}
		assertState(name, id, stream, 1, 1, true)
	})

	t.Run("concurrent identical candidates apply once", func(t *testing.T) {
		name, id, stream := "custody-concurrent", uuid.NewString(), uuid.NewString()
		body := message(t, id, stream, company, 1)
		seedJob(name, id)
		var calls, applied, duplicates atomic.Int32
		start, errs := make(chan struct{}), make(chan error, 8)
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				<-start
				got, err := run(db, name, body, func(u effects, e events.Envelope) error { calls.Add(1); return u.Apply(ctx, e) }, func(u dispatchUnit) error { return u.Complete(ctx, id) })
				if got == inbox.AppliedCandidate {
					applied.Add(1)
				}
				if got == inbox.DuplicateCandidate {
					duplicates.Add(1)
				}
				errs <- err
			})
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if calls.Load() != 1 || applied.Load() != 1 || duplicates.Load() != 7 {
			t.Fatal(calls.Load(), applied.Load(), duplicates.Load())
		}
		assertState(name, id, stream, 1, 1, true)
	})

	t.Run("conflicting concurrent bytes cannot complete a second effect", func(t *testing.T) {
		name, id, stream := "custody-conflict", uuid.NewString(), uuid.NewString()
		body := message(t, id, stream, company, 1)
		seedJob(name, id)
		start, errs := make(chan struct{}), make(chan error, 2)
		var wg sync.WaitGroup
		for _, b := range [][]byte{body, append(bytes.Clone(body), ' ')} {
			wg.Go(func() {
				<-start
				_, err := run(db, name, b, apply, func(u dispatchUnit) error { return u.Complete(ctx, id) })
				errs <- err
			})
		}
		close(start)
		wg.Wait()
		close(errs)
		var successes, conflicts int
		for err := range errs {
			if err == nil {
				successes++
			} else if errors.Is(err, inbox.ErrEnvelopeConflict) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatal(successes, conflicts)
		}
		assertState(name, id, stream, 1, 1, true)
	})

	t.Run("caller factory failure panic and cancellation roll back", func(t *testing.T) {
		for _, failure := range []string{"factory", "panic", "cancel"} {
			name, id, stream := "custody-"+failure, uuid.NewString(), uuid.NewString()
			body := message(t, id, stream, company, 1)
			seedJob(name, id)
			cancelCtx, cancel := context.WithCancel(ctx)
			func() {
				defer func() {
					if failure == "panic" && recover() != "fixture panic" {
						t.Error("panic not propagated")
					}
				}()
				if failure == "factory" {
					factoryError := errors.New("fixture factory failed")
					runner, err := eventstore.NewTransactions(db, func(tx *gorm.DB) (dispatchUnit, error) {
						_, err := inbox.NewTransactionalConsumer(tx, name, func(tx *gorm.DB) (effects, error) {
							if err := tx.Exec("UPDATE fixture.jobs SET completed=true WHERE consumer_name=? AND event_id=?::uuid", name, id).Error; err != nil {
								return nil, err
							}
							return nil, factoryError
						}, validator(t, company))
						return nil, err
					})
					if err != nil {
						t.Fatal(err)
					}
					if err := runner.Run(ctx, func(dispatchUnit) error { t.Error("failed factory escaped"); return nil }); !errors.Is(err, factoryError) {
						t.Fatal(err)
					}
					return
				}
				err := dispatchRunner(t, db, name, validator(t, company)).Run(cancelCtx, func(u dispatchUnit) error {
					if _, err := u.Apply(cancelCtx, body, apply); err != nil {
						return err
					}
					if err := u.Complete(cancelCtx, id); err != nil {
						return err
					}
					if failure == "panic" {
						panic("fixture panic")
					}
					cancel()
					return nil
				})
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			}()
			cancel()
			assertState(name, id, stream, 0, 0, false)
			got, err := run(db, name, body, apply, func(u dispatchUnit) error { return u.Complete(ctx, id) })
			if got != inbox.AppliedCandidate || err != nil {
				t.Fatal(got, err)
			}
			assertState(name, id, stream, 1, 1, true)
		}
	})

	t.Run("lost replies after actual commit and rollback remain unconfirmed", func(t *testing.T) {
		for _, commitFirst := range []bool{false, true} {
			name, id, stream := fmt.Sprintf("custody-unknown-%v", commitFirst), uuid.NewString(), uuid.NewString()
			body := message(t, id, stream, company, 1)
			seedJob(name, id)
			pool, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			fault, err := gorm.Open(postgres.New(postgres.Config{Conn: commitFaultPool{DB: pool, commitFirst: commitFirst}}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			got, err := run(fault, name, body, apply, func(u dispatchUnit) error { return u.Complete(ctx, id) })
			if got != inbox.AppliedCandidate || !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) || !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatal(got, err)
			}
			count, expected := int64(0), inbox.AppliedCandidate
			if commitFirst {
				count, expected = 1, inbox.DuplicateCandidate
			}
			assertState(name, id, stream, count, count, commitFirst)
			got, err = run(db, name, body, apply, func(u dispatchUnit) error { return u.Complete(ctx, id) })
			if got != expected || err != nil {
				t.Fatal(got, err)
			}
			assertState(name, id, stream, 1, 1, true)
		}
	})

	t.Run("duplicate completion with lost reply reconciles the pending job", func(t *testing.T) {
		for _, commitFirst := range []bool{false, true} {
			name, id, stream := fmt.Sprintf("custody-duplicate-unknown-%v", commitFirst), uuid.NewString(), uuid.NewString()
			body := message(t, id, stream, company, 1)
			seedJob(name, id)
			// A prior committed inbox/effect models retained pre-migration evidence.
			if got, err := run(db, name, body, apply, func(dispatchUnit) error { return nil }); got != inbox.AppliedCandidate || err != nil {
				t.Fatal(got, err)
			}
			pool, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			fault, err := gorm.Open(postgres.New(postgres.Config{Conn: commitFaultPool{DB: pool, commitFirst: commitFirst}}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			got, err := run(fault, name, body, noApply, func(u dispatchUnit) error { return u.Complete(ctx, id) })
			if got != inbox.DuplicateCandidate || !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) || !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatal(got, err)
			}
			assertState(name, id, stream, 1, 1, commitFirst)
			got, err = run(db, name, body, noApply, func(u dispatchUnit) error { return u.Complete(ctx, id) })
			if got != inbox.DuplicateCandidate || err != nil {
				t.Fatal(got, err)
			}
			assertState(name, id, stream, 1, 1, true)
		}
	})

	t.Run("plain pool ended transactions isolation and mixed markers fail closed", func(t *testing.T) {
		bind := func(tx *gorm.DB) (effects, error) { return effectAdapter{tx: tx, consumer: name}, nil }
		if _, err := inbox.NewTransactionalConsumer(db, name, bind, validator(t, company)); !errors.Is(err, eventstore.ErrTransactionRequired) {
			t.Fatal(err)
		}
		for _, isolation := range []sql.IsolationLevel{sql.LevelRepeatableRead, sql.LevelSerializable} {
			tx := db.Begin(&sql.TxOptions{Isolation: isolation})
			c, err := inbox.NewTransactionalConsumer(tx, name, bind, validator(t, company))
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.Apply(ctx, body, noApply)
			if got != inbox.NoCandidate || !errors.Is(err, inbox.ErrIsolation) {
				t.Fatal(got, err)
			}
			tx.Rollback()
		}
		tx := db.Begin()
		c, err := inbox.NewTransactionalConsumer(tx, name, bind, validator(t, company))
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec("SET LOCAL justix.messaging_mode='legacy'").Error; err != nil {
			t.Fatal(err)
		}
		if got, err := c.Apply(ctx, body, noApply); got != inbox.NoCandidate || !errors.Is(err, inbox.ErrMessagingMode) {
			t.Fatal(got, err)
		}
		tx.Rollback()
		if got, err := c.Apply(ctx, body, noApply); got != inbox.NoCandidate || err == nil {
			t.Fatal(got, err)
		}
	})
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

func TestInboxCutoverLockFailurePostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_INBOX") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_INBOX=1 for disposable PostgreSQL inbox tests")
	}
	db, ownerSQL := databaseWithMessaging(t, true)
	ctx, company := context.Background(), uuid.NewString()
	name, id, stream := "cutover-contention", uuid.NewString(), uuid.NewString()
	body := message(t, id, stream, company, 1)
	started, contend := make(chan struct{}), make(chan struct{})
	c, err := inbox.NewConsumer(db, name, func(tx *gorm.DB) (effects, error) {
		if err := tx.Exec("SET LOCAL lock_timeout='300ms'").Error; err != nil {
			return nil, err
		}
		return effectAdapter{tx: tx, consumer: name}, nil
	}, validator(t, company))
	if err != nil {
		t.Fatal(err)
	}
	type answer struct {
		outcome inbox.Outcome
		err     error
	}
	consumed := make(chan answer, 1)
	go func() {
		got, err := c.Consume(ctx, body, func(u effects, e events.Envelope) error {
			if err := u.Apply(ctx, e); err != nil {
				return err
			}
			close(started)
			<-contend
			// Test-only access: reproduce an owner effect trying to write legacy
			// outbox after inbox, while migration holds outbox and waits on inbox.
			return u.(effectAdapter).tx.Exec("LOCK TABLE eventstore.outbox IN ROW EXCLUSIVE MODE").Error
		}, func() error { t.Error("lock failure acknowledged"); return nil })
		consumed <- answer{got, err}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		close(contend)
		t.Fatal("consumer did not reach effect")
	}
	migrated := make(chan error, 1)
	go func() {
		out, err := ownerSQL(fmt.Sprintf(`BEGIN; LOCK TABLE eventstore.outbox IN ACCESS EXCLUSIVE MODE;
		SELECT eventstore.activate_messaging_custody('%s',sha256('new'::bytea),sha256('old'::bytea),'synthetic-backup','fault-injected-running-consumer','synthetic-broker-checkpoints','synthetic-compatibility'); COMMIT;`, uuid.NewString()))
		if err != nil {
			err = fmt.Errorf("%w: %s", err, out)
		}
		migrated <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var locked bool
		err := db.Raw(`SELECT EXISTS(SELECT FROM pg_locks WHERE relation='eventstore.outbox'::regclass AND mode='AccessExclusiveLock' AND granted)`).Scan(&locked).Error
		if err != nil {
			close(contend)
			t.Fatal(err)
		}
		if locked {
			break
		}
		if time.Now().After(deadline) {
			close(contend)
			t.Fatal("migration did not lock outbox")
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(contend)
	result := <-consumed
	var state interface{ SQLState() string }
	if result.outcome != inbox.Unconfirmed || !errors.As(result.err, &state) || state.SQLState() != "55P03" {
		t.Fatalf("expected unconfirmed lock timeout: %v %v", result.outcome, result.err)
	}
	if err := <-migrated; err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"eventstore.inbox", "fixture.effects", "fixture.checkpoints"} {
		var count int64
		if err := db.Raw("SELECT count(*) FROM "+table+" WHERE consumer_name=?", name).Scan(&count).Error; err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	if got, err := c.Consume(ctx, body, func(effects, events.Envelope) error { t.Error("post-cutover direct effect"); return nil }, func() error { t.Error("post-cutover ACK"); return nil }); got != inbox.Unconfirmed || !errors.Is(err, inbox.ErrMessagingMode) {
		t.Fatal(got, err)
	}
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
	db, _ := databaseWithMessaging(t, false)
	return db
}

func databaseWithMessaging(t *testing.T, messaging bool) (*gorm.DB, func(string, ...string) (string, error)) {
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
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
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
	if messaging {
		migration, err := os.ReadFile("../eventstore/migrations/000002_messaging_delivery.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if out, err := query(string(migration), "-v", "owner_service=inventory", "-v", "runtime_role=justix_inventory_runtime"); err != nil {
			t.Fatal(out, err)
		}
	}
	if out, err := query(`CREATE SCHEMA fixture;
		CREATE TABLE fixture.effects (consumer_name text NOT NULL,event_id uuid NOT NULL,PRIMARY KEY(consumer_name,event_id));
		CREATE TABLE fixture.checkpoints (consumer_name text NOT NULL,aggregate_id uuid NOT NULL,position bigint NOT NULL,PRIMARY KEY(consumer_name,aggregate_id));
		CREATE TABLE fixture.jobs (consumer_name text NOT NULL,event_id uuid NOT NULL,completed boolean NOT NULL DEFAULT false,PRIMARY KEY(consumer_name,event_id));
		GRANT USAGE ON SCHEMA fixture TO justix_inventory_runtime;
		GRANT SELECT,INSERT ON fixture.effects TO justix_inventory_runtime;
		GRANT SELECT,INSERT,UPDATE ON fixture.jobs TO justix_inventory_runtime;
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
	return db, query
}
