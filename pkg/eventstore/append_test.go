package eventstore_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
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
)

type testPayload struct {
	Ref string `json:"ref"`
}

func revision(v int64) events.Revision {
	r, err := events.NewRevision(v)
	if err != nil {
		panic(err)
	}
	return r
}

func pending(t *testing.T, aggregate, company string, rev, sequence int64) eventstore.Event {
	t.Helper()
	schema, err := events.NewEventSchema("inventory.fixture.changed.v1", 1, "fixture", events.CompanyScopeTenantRequired, func(p testPayload) error {
		if p.Ref == "" {
			return errors.New("missing reference")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	in := events.EnvelopeInput{EventID: uuid.NewString(), AggregateID: aggregate, CompanyID: &company, AggregateVersion: revision(rev), IntegrationSequence: revision(sequence), OccurredAt: time.Now().UTC(), Actor: events.Actor{Kind: events.ActorUser, ID: uuid.NewString()}, CorrelationID: uuid.NewString(), CausationID: uuid.NewString(), OperationID: uuid.NewString()}
	var e eventstore.Event
	if sequence == 0 {
		e, err = eventstore.NewInternalEvent(in, schema, testPayload{Ref: "synthetic"})
	} else {
		e, err = eventstore.NewEvent(in, schema, testPayload{Ref: "synthetic"})
	}
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestAppendRequiresTransactionAndSchema(t *testing.T) {
	if _, err := eventstore.NewAppender(nil, events.OwnerInventory); !errors.Is(err, eventstore.ErrTransactionRequired) {
		t.Fatal(err)
	}
	if _, err := eventstore.NewTransactions[struct{}](nil, nil); err == nil {
		t.Fatal("accepted nil database")
	}
	if _, err := eventstore.NewEvent(events.EnvelopeInput{}, events.EventSchema[testPayload]{}, testPayload{}); err == nil {
		t.Fatal("accepted unbound schema")
	}
	if _, err := eventstore.NewInternalEvent(events.EnvelopeInput{IntegrationSequence: revision(1)}, events.EventSchema[testPayload]{}, testPayload{}); !errors.Is(err, eventstore.ErrInvalidAppend) {
		t.Fatal(err)
	}
	company, id := uuid.NewString(), uuid.NewString()
	if _, ok := pending(t, id, company, 1, 0).IntegrationEnvelope(); ok {
		t.Fatal("internal event exposed an integration envelope")
	}
	if envelope, ok := pending(t, id, company, 1, 1).IntegrationEnvelope(); !ok || envelope.IntegrationSequence() != revision(1) {
		t.Fatal("valid integration envelope unavailable")
	}
	if _, ok := (eventstore.Event{}).IntegrationEnvelope(); ok {
		t.Fatal("zero event exposed an integration envelope")
	}
}

// The application sees only this port shape, never GORM or an untyped service
// locator. Concrete owner guard/outbox/receipt implementations are downstream;
// these typed fixture ports prove they share the append transaction boundary.
type fixtureUOW interface {
	Append(context.Context, ...eventstore.Batch) error
	Effects(context.Context, string) error
}
type fixturePorts struct {
	*eventstore.Appender
	tx *gorm.DB
}

func (u fixturePorts) Effects(ctx context.Context, id string) error {
	tx := u.tx.WithContext(ctx)
	if err := tx.Exec("INSERT INTO eventstore.fixture_guard (id) VALUES (?::uuid)", id).Error; err != nil {
		return err
	}
	if err := tx.Exec(`INSERT INTO eventstore.command_receipts
	(receipt_id,actor_id,company_id,command_name,target_id,idempotency_key,request_hash,http_status,receipt)
	VALUES (?::uuid,?::uuid,?::uuid,'fixture',?::uuid,?::uuid,?,200,'{"result":"recorded"}')`, id, id, id, id, id, make([]byte, 32)).Error; err != nil {
		return err
	}
	body := []byte(`{"fixture":true}`)
	digest := sha256.Sum256(body)
	return tx.Exec(`INSERT INTO eventstore.outbox (event_id,aggregate_type,aggregate_id,integration_sequence,envelope,envelope_hash,exchange,routing_key)
	VALUES (?::uuid,'fixture',?::uuid,1,?,?,'fixture','fixture')`, id, id, body, digest[:]).Error
}
func runner(t *testing.T, db *gorm.DB) *eventstore.Transactions[fixtureUOW] {
	t.Helper()
	r, err := eventstore.NewTransactions(db, func(tx *gorm.DB) (fixtureUOW, error) {
		a, err := eventstore.NewAppender(tx, events.OwnerInventory)
		return fixturePorts{Appender: a, tx: tx}, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestConditionalAppendPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_EVENTSTORE_APPEND") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_EVENTSTORE_APPEND=1 for isolated Docker/PostgreSQL append tests")
	}
	db := appendDatabase(t)
	r := runner(t, db)
	ctx := context.Background()
	company := uuid.NewString()
	if _, err := eventstore.NewAppender(db, events.OwnerInventory); !errors.Is(err, eventstore.ErrTransactionRequired) {
		t.Fatal("accepted nontransaction handle", err)
	}
	count := func(table, id string) int64 {
		t.Helper()
		var n int64
		column := "id"
		if table == "events" {
			column = "aggregate_id"
		}
		if table == "command_receipts" {
			column = "receipt_id"
		}
		if table == "outbox" {
			column = "event_id"
		}
		if err := db.Raw("SELECT count(*) FROM eventstore."+table+" WHERE "+column+"=?::uuid", id).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	assertEffects := func(id string, want int64) {
		t.Helper()
		for _, table := range []string{"events", "fixture_guard", "command_receipts", "outbox"} {
			if got := count(table, id); got != want {
				t.Fatalf("%s: got %d want %d", table, got, want)
			}
		}
	}

	t.Run("competing creates and updates", func(t *testing.T) {
		id := uuid.NewString()
		for expected := int64(0); expected < 2; expected++ {
			start := make(chan struct{})
			results := make(chan error, 2)
			var wg sync.WaitGroup
			for range 2 {
				e := pending(t, id, company, expected+1, expected+1)
				wg.Go(func() {
					<-start
					results <- r.Run(ctx, func(u fixtureUOW) error {
						return u.Append(ctx, eventstore.Batch{Expected: revision(expected), Events: []eventstore.Event{e}})
					})
				})
			}
			close(start)
			wg.Wait()
			close(results)
			wins, conflicts := 0, 0
			for err := range results {
				if err == nil {
					wins++
				} else if errors.Is(err, eventstore.ErrVersionConflict) {
					conflicts++
				} else {
					t.Fatal(err)
				}
			}
			if wins != 1 || conflicts != 1 {
				t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
			}
		}
		if count("events", id) != 2 {
			t.Fatal("competing append corrupted history")
		}
	})

	t.Run("internal revisions and integration sequences", func(t *testing.T) {
		id := uuid.NewString()
		es := []eventstore.Event{pending(t, id, company, 1, 1), pending(t, id, company, 2, 0), pending(t, id, company, 3, 2)}
		if err := r.Run(ctx, func(u fixtureUOW) error { return u.Append(ctx, eventstore.Batch{Events: es}) }); err != nil {
			t.Fatal(err)
		}
		var got string
		if err := db.Raw("SELECT string_agg(coalesce(integration_sequence::text,'null'),',' ORDER BY aggregate_version) FROM eventstore.events WHERE aggregate_id=?::uuid", id).Scan(&got).Error; err != nil {
			t.Fatal(err)
		}
		if got != "1,null,2" {
			t.Fatal(got)
		}
		bad := pending(t, id, company, 4, 4)
		if err := r.Run(ctx, func(u fixtureUOW) error {
			return u.Append(ctx, eventstore.Batch{Expected: revision(3), Events: []eventstore.Event{bad}})
		}); !errors.Is(err, eventstore.ErrInvalidAppend) {
			t.Fatal(err)
		}
		if count("events", id) != 3 {
			t.Fatal("sequence gap persisted")
		}
	})

	t.Run("reversed multistream races acquire sorted locks", func(t *testing.T) {
		ids := []string{uuid.NewString(), uuid.NewString()}
		start, results := make(chan struct{}), make(chan error, 2)
		for i := range 2 {
			batches := []eventstore.Batch{
				{Events: []eventstore.Event{pending(t, ids[i], company, 1, 1)}},
				{Events: []eventstore.Event{pending(t, ids[1-i], company, 1, 1)}},
			}
			go func() {
				<-start
				raceCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				results <- r.Run(raceCtx, func(u fixtureUOW) error { return u.Append(raceCtx, batches...) })
			}()
		}
		close(start)
		wins, conflicts := 0, 0
		for range 2 {
			err := <-results
			if err == nil {
				wins++
			} else if errors.Is(err, eventstore.ErrVersionConflict) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
		if wins != 1 || conflicts != 1 || count("events", ids[0]) != 1 || count("events", ids[1]) != 1 {
			t.Fatalf("atomic race: wins=%d conflicts=%d", wins, conflicts)
		}
	})

	t.Run("corrupt history fails closed", func(t *testing.T) {
		id := uuid.NewString()
		// Runtime INSERT grants alone do not prove contiguous history. Simulate
		// a faulty old adapter that bypassed conditional append.
		if err := db.Exec(eventSQL("inventory", uuid.NewString(), id, "'"+company+"'", 2)).Error; err != nil {
			t.Fatal(err)
		}
		err := r.Run(ctx, func(u fixtureUOW) error {
			return u.Append(ctx, eventstore.Batch{Expected: revision(2), Events: []eventstore.Event{pending(t, id, company, 3, 1)}})
		})
		if !errors.Is(err, eventstore.ErrCorruptHistory) || count("events", id) != 1 {
			t.Fatal("adopted gapped history", err)
		}
	})

	t.Run("invalid batch and event company metadata", func(t *testing.T) {
		id := uuid.NewString()
		other := uuid.NewString()
		cases := [][]eventstore.Batch{
			nil, {{}}, {{Events: []eventstore.Event{{}}}},
			{{Events: []eventstore.Event{pending(t, id, company, 2, 1)}}},
			{{Events: []eventstore.Event{pending(t, id, company, 1, 1), pending(t, other, company, 2, 2)}}},
			{{Events: []eventstore.Event{pending(t, id, company, 1, 1)}}, {Events: []eventstore.Event{pending(t, id, company, 1, 1)}}},
		}
		for i, batches := range cases {
			if err := r.Run(ctx, func(u fixtureUOW) error { return u.Append(ctx, batches...) }); !errors.Is(err, eventstore.ErrInvalidAppend) {
				t.Fatalf("case %d: %v", i, err)
			}
		}
		if count("events", id) != 0 {
			t.Fatal("invalid batch persisted")
		}
		foreign, err := eventstore.NewTransactions(db, func(tx *gorm.DB) (fixtureUOW, error) {
			a, err := eventstore.NewAppender(tx, events.OwnerCommerce)
			return fixturePorts{Appender: a, tx: tx}, err
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := foreign.Run(ctx, func(u fixtureUOW) error {
			return u.Append(ctx, eventstore.Batch{Events: []eventstore.Event{pending(t, id, company, 1, 1)}})
		}); !errors.Is(err, eventstore.ErrInvalidAppend) {
			t.Fatal("accepted foreign owner", err)
		}
		if err := r.Run(ctx, func(u fixtureUOW) error {
			return u.Append(ctx, eventstore.Batch{Events: []eventstore.Event{pending(t, id, company, 1, 1)}})
		}); err != nil {
			t.Fatal(err)
		}
		if err := r.Run(ctx, func(u fixtureUOW) error {
			return u.Append(ctx, eventstore.Batch{Expected: revision(1), Events: []eventstore.Event{pending(t, id, other, 2, 2)}})
		}); err != nil {
			t.Fatal("technical adapter invented immutable company rule", err)
		}
		var got string
		if err := db.Raw("SELECT company_id::text FROM eventstore.events WHERE aggregate_id=?::uuid AND aggregate_version=2", id).Scan(&got).Error; err != nil {
			t.Fatal(err)
		}
		if got != other {
			t.Fatal("event company metadata was rewritten")
		}
	})

	t.Run("all effects rollback and commit", func(t *testing.T) {
		id := uuid.NewString()
		failed := errors.New("guard rejected")
		work := func(u fixtureUOW) error {
			if err := u.Append(ctx, eventstore.Batch{Events: []eventstore.Event{pending(t, id, company, 1, 1)}}); err != nil {
				return err
			}
			return u.Effects(ctx, id)
		}
		if err := r.Run(ctx, func(u fixtureUOW) error {
			if err := work(u); err != nil {
				return err
			}
			return failed
		}); !errors.Is(err, failed) {
			t.Fatal(err)
		}
		assertEffects(id, 0)
		if err := r.Run(ctx, work); err != nil {
			t.Fatal(err)
		}
		assertEffects(id, 1)
	})

	t.Run("multiple streams unique failure rolls back earlier inserts", func(t *testing.T) {
		id, other := uuid.NewString(), uuid.NewString()
		e := pending(t, id, company, 1, 1)
		if err := r.Run(ctx, func(u fixtureUOW) error { return u.Append(ctx, eventstore.Batch{Events: []eventstore.Event{e}}) }); err != nil {
			t.Fatal(err)
		}
		// A duplicate event ID on a different stream is constructed explicitly
		// through a valid schema, keeping payload validation in the path.
		var eventID string
		if err := db.Raw("SELECT event_id::text FROM eventstore.events WHERE aggregate_id=?::uuid", id).Scan(&eventID).Error; err != nil {
			t.Fatal(err)
		}
		schema, _ := events.NewEventSchema("inventory.fixture.changed.v1", 1, "fixture", events.CompanyScopeTenantRequired, func(p testPayload) error { return nil })
		duplicate, err := eventstore.NewEvent(events.EnvelopeInput{EventID: eventID, AggregateID: other, CompanyID: &company, AggregateVersion: revision(1), IntegrationSequence: revision(1), OccurredAt: time.Now().UTC(), Actor: events.Actor{Kind: events.ActorUser, ID: uuid.NewString()}, CorrelationID: uuid.NewString(), CausationID: uuid.NewString(), OperationID: uuid.NewString()}, schema, testPayload{Ref: "synthetic"})
		if err != nil {
			t.Fatal(err)
		}
		fresh := uuid.NewString()
		if err := r.Run(ctx, func(u fixtureUOW) error {
			if err := u.Effects(ctx, fresh); err != nil {
				return err
			}
			return u.Append(ctx, eventstore.Batch{Events: []eventstore.Event{pending(t, fresh, company, 1, 1)}}, eventstore.Batch{Events: []eventstore.Event{duplicate}})
		}); err == nil {
			t.Fatal("accepted duplicate event ID")
		}
		assertEffects(fresh, 0)
		if count("events", other) != 0 {
			t.Fatal("duplicate persisted")
		}
	})

	t.Run("lost commit replies require authoritative receipt", func(t *testing.T) {
		for _, commitFirst := range []bool{false, true} {
			id := uuid.NewString()
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			faultDB, err := gorm.Open(postgres.New(postgres.Config{Conn: commitFaultPool{DB: sqlDB, commitFirst: commitFirst}}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			err = runner(t, faultDB).Run(ctx, func(u fixtureUOW) error {
				calls++
				if err := u.Append(ctx, eventstore.Batch{Events: []eventstore.Event{pending(t, id, company, 1, 1)}}); err != nil {
					return err
				}
				return u.Effects(ctx, id)
			})
			if !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) || !errors.Is(err, io.ErrUnexpectedEOF) || calls != 1 {
				t.Fatalf("commitFirst=%v calls=%d err=%v", commitFirst, calls, err)
			}
			want := int64(0)
			if commitFirst {
				want = 1
			}
			assertEffects(id, want)
		}
	})

	t.Run("panic and cancellation roll back", func(t *testing.T) {
		id := uuid.NewString()
		func() {
			defer func() {
				if recover() != "fixture panic" {
					t.Error("panic not preserved")
				}
			}()
			_ = r.Run(ctx, func(u fixtureUOW) error {
				if err := u.Effects(ctx, id); err != nil {
					t.Fatal(err)
				}
				panic("fixture panic")
			})
		}()
		assertEffects(id, 0)
		cancelCtx, cancel := context.WithCancel(ctx)
		err := r.Run(cancelCtx, func(u fixtureUOW) error {
			if err := u.Effects(cancelCtx, id); err != nil {
				return err
			}
			cancel()
			return nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		assertEffects(id, 0)
	})
}

// This fault is injected at the actual SQL transaction's commit boundary. Both
// variants lose the reply; one really commits and the other really rolls back.
// The runner must not claim to distinguish them from that reply alone.
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

func appendDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	f := schemaFixture{t: t, container: "justixauto-t009-" + uuid.NewString(), schema: schema, password: "synthetic-" + uuid.NewString()}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "run", "-d", "--name", f.container, "-e", "POSTGRES_PASSWORD="+f.password, "-e", "POSTGRES_DB=justix_inventory", "-p", "127.0.0.1::5432", postgresImage)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("start owned fixture: %v %s", err, out)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "docker", "rm", "-f", f.container).CombinedOutput(); err != nil {
			t.Errorf("remove owned fixture: %v %s", err, out)
		}
	})
	for {
		if _, err := f.sql("inventory", "postgres", "SELECT 1"); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("fixture startup timeout")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if got := f.mustSQL("inventory", "postgres", "SHOW server_version_num"); got != "180006" {
		t.Fatal(got)
	}
	f.mustSQL("inventory", "postgres", "CREATE ROLE justix_inventory_runtime LOGIN PASSWORD '"+f.password+"';")
	if out, err := f.sql("inventory", "postgres", string(schema), "-v", "owner_service=inventory", "-v", "runtime_role=justix_inventory_runtime"); err != nil {
		t.Fatalf("install schema: %v %s", err, out)
	}
	f.mustSQL("inventory", "postgres", "CREATE TABLE eventstore.fixture_guard (id uuid PRIMARY KEY); GRANT SELECT,INSERT ON eventstore.fixture_guard TO justix_inventory_runtime;")
	out, err := exec.CommandContext(ctx, "docker", "port", f.container, "5432/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	address := strings.TrimSpace(string(out))
	if !strings.HasPrefix(address, "127.0.0.1:") || strings.Contains(address, "\n") {
		t.Fatalf("unexpected fixture binding %q", address)
	}
	dsn := fmt.Sprintf("host=127.0.0.1 port=%s user=justix_inventory_runtime password=%s dbname=justix_inventory sslmode=disable", strings.TrimPrefix(address, "127.0.0.1:"), f.password)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(8)
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}
