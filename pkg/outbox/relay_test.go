package outbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
)

type relayPublisherFunc func(context.Context, Publication) (RoutedConfirmation, error)

func (f relayPublisherFunc) Publish(ctx context.Context, p Publication) (RoutedConfirmation, error) {
	return f(ctx, p)
}

func TestRelayConstructorClosed(t *testing.T) {
	if _, err := NewRelayStore(nil, events.OwnerInventory, RelayLegacy, time.Second); err == nil {
		t.Fatal("nil database")
	}
	if _, err := NewRelay(nil, nil); err == nil {
		t.Fatal("nil relay")
	}
	var s *RelayStore
	if _, err := s.Claim(context.Background()); err == nil {
		t.Fatal("nil store")
	}
	if err := s.MarkSent(context.Background(), RelayLease{}, RoutedConfirmation{}); err == nil {
		t.Fatal("unsealed completion")
	}
	p := Publication{eventID: uuid.NewString(), exchange: IntegrationExchange, routingKey: "inventory.retail.inventory.fixture.changed.v1", attempt: uuid.NewString(), body: []byte(`{"ref":"synthetic"}`)}
	p.hash = sha256.Sum256(p.body)
	p.committed = true
	copyBytes := p.Bytes()
	copyBytes[0] = '!'
	if bytes.Equal(copyBytes, p.Bytes()) {
		t.Fatal("publication bytes escaped")
	}
	c := RoutedConfirmation{p, true}
	other := p
	other.attempt = uuid.NewString()
	if c.matches(other) || (RoutedConfirmation{}).matches(p) {
		t.Fatal("foreign attempt receipt")
	}
}

type relayPG struct {
	db    *gorm.DB
	admin func(string, ...string) (string, error)
	mode  RelayMode
}

func TestRelayBackoffBounds(t *testing.T) {
	for _, attempt := range []int64{1, 2, 3, 6, 7, 63, math.MaxInt64} {
		low, err := relayRetryDelay(attempt, 0)
		if err != nil {
			t.Fatal(err)
		}
		high, err := relayRetryDelay(attempt, 1)
		if err != nil {
			t.Fatal(err)
		}
		mid, err := relayRetryDelay(attempt, 0.5)
		if err != nil || low < time.Second || high > time.Minute || mid < low || mid > high {
			t.Fatal("backoff range", attempt, low, mid, high, err)
		}
		if attempt == 1 && (low != time.Second || high != time.Second) {
			t.Fatal("first retry")
		}
		if attempt >= 7 && (low != 30*time.Second || high != time.Minute) {
			t.Fatal("capped jitter")
		}
	}
	for _, sample := range []float64{-1, 1.1, math.NaN(), math.Inf(1)} {
		if _, err := relayRetryDelay(1, sample); err == nil {
			t.Fatal("invalid jitter accepted")
		}
	}
	if _, err := relayRetryDelay(0, 0.5); err == nil {
		t.Fatal("missing attempt accepted")
	}
}

func relayDocker(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("owned fixture command %s: %v %s", args[0], err, out)
	}
	return strings.TrimSpace(string(out))
}

// No external DSN is accepted. Only a random, loopback, disposable PostgreSQL
// fixture is installed; immutable event history is retained until its teardown.
func newRelayPG(t *testing.T, mode RelayMode, additive bool) *relayPG {
	t.Helper()
	name := "justixauto-t012-pg-" + uuid.NewString()
	password := "synthetic-" + uuid.NewString()
	relayDocker(t, "run", "-d", "--name", name, "-e", "POSTGRES_PASSWORD="+password, "-e", "POSTGRES_DB=justix_inventory", "-p", "127.0.0.1::5432", "docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2")
	t.Cleanup(func() { relayDocker(t, "rm", "-f", "-v", name) })
	admin := func(query string, args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", append([]string{"exec", "-i", name, "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "justix_inventory"}, args...)...)
		cmd.Stdin = strings.NewReader(query)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	deadline := time.Now().Add(45 * time.Second)
	for {
		if _, err := admin("SELECT 1"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("PostgreSQL startup")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if v, err := admin("SHOW server_version_num"); err != nil || v != "180006" {
		t.Fatal("PostgreSQL version", v, err)
	}
	if out, err := admin("CREATE ROLE justix_inventory_runtime LOGIN PASSWORD '" + password + "'"); err != nil {
		t.Fatal(out, err)
	}
	if out, err := admin("REVOKE CREATE,TEMPORARY ON DATABASE justix_inventory FROM PUBLIC; REVOKE CREATE ON SCHEMA public FROM PUBLIC"); err != nil {
		t.Fatal(out, err)
	}
	files := []string{"../eventstore/schema.sql"}
	if additive || mode == RelayCustody {
		files = append(files, "../eventstore/migrations/000002_messaging_delivery.up.sql", "../eventstore/migrations/000003_messaging_route_compatibility.up.sql")
	}
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		args := []string{"-v", "owner_service=inventory", "-v", "runtime_role=justix_inventory_runtime"}
		if strings.Contains(file, "000003") {
			args = append(args, "-v", "prior_migration_sha256=1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2", "-v", fmt.Sprintf("correction_migration_sha256=%x", sha256.Sum256(b)), "-v", "backup_ref=synthetic-empty", "-v", "stopped_runtimes_ref=synthetic-stopped", "-v", "compatibility_ref=synthetic-test")
		}
		if out, err := admin(string(b), args...); err != nil {
			t.Fatal(file, out, err)
		}
	}
	if mode == RelayCustody {
		if out, err := admin(`SELECT eventstore.activate_messaging_custody('` + uuid.NewString() + `',sha256('new'::bytea),sha256('old'::bytea),'synthetic-empty','synthetic-stopped','synthetic-no-broker','synthetic-compatible')`); err != nil {
			t.Fatal(out, err)
		}
	}
	address := relayDocker(t, "port", name, "5432/tcp")
	if !strings.HasPrefix(address, "127.0.0.1:") || strings.Contains(address, "\n") {
		t.Fatal("non-loopback database")
	}
	dsn := fmt.Sprintf("host=127.0.0.1 port=%s user=justix_inventory_runtime password=%s dbname=justix_inventory sslmode=disable", strings.TrimPrefix(address, "127.0.0.1:"), password)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("fixture connect failed")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(12)
	t.Cleanup(func() { _ = pool.Close() })
	return &relayPG{db, admin, mode}
}

type relayData struct {
	Ref string `json:"ref"`
}

func relaySchema(t *testing.T) events.EventSchema[relayData] {
	t.Helper()
	s, err := events.NewEventSchema("inventory.fixture.changed.v1", 1, "fixture", events.CompanyScopeTenantRequired, func(p relayData) error {
		if p.Ref != "synthetic" {
			return errors.New("invalid fixture")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (f *relayPG) seed(t *testing.T, destinations ...events.Owner) events.Envelope {
	return f.seedOutcome(t, false, destinations...)
}

func (f *relayPG) seedOutcome(t *testing.T, rollback bool, destinations ...events.Owner) events.Envelope {
	t.Helper()
	ctx := context.Background()
	schema := relaySchema(t)
	one, _ := events.NewRevision(1)
	company := uuid.NewString()
	e, err := eventstore.NewEvent(events.EnvelopeInput{EventID: uuid.NewString(), AggregateID: uuid.NewString(), AggregateVersion: one, IntegrationSequence: one, CompanyID: &company, OccurredAt: time.Now().UTC(), Actor: events.Actor{Kind: events.ActorSystem, ID: uuid.NewString()}, CorrelationID: uuid.NewString(), CausationID: uuid.NewString(), OperationID: uuid.NewString()}, schema, relayData{"synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	env, _ := e.IntegrationEnvelope()
	r, err := eventstore.NewTransactions(f.db, func(tx *gorm.DB) (*gorm.DB, error) { return tx, nil })
	if err != nil {
		t.Fatal(err)
	}
	rolledBack := errors.New("synthetic command rollback")
	err = r.Run(ctx, func(tx *gorm.DB) (result error) {
		defer func() {
			if rollback && result == nil {
				result = rolledBack
			}
		}()
		a, err := eventstore.NewAppender(tx, events.OwnerInventory)
		if err != nil {
			return err
		}
		if f.mode == RelayLegacy {
			if len(destinations) != 1 {
				return errors.New("legacy fixture needs one target")
			}
			if err := a.Append(ctx, eventstore.Batch{Events: []eventstore.Event{e}}); err != nil {
				return err
			}
			i, err := NewInserter(tx, events.OwnerInventory)
			if err != nil {
				return err
			}
			record, err := NewRecord(env, schema, destinations[0])
			if err != nil {
				return err
			}
			return i.Insert(ctx, record)
		}
		ad, err := NewSourceAdmissions(tx, events.OwnerInventory)
		if err != nil {
			return err
		}
		stream, err := NewSourceStream(events.OwnerInventory, env.AggregateType(), env.AggregateID())
		if err != nil {
			return err
		}
		if err := ad.Fence(ctx, stream); err != nil {
			return err
		}
		for _, destination := range destinations {
			admission, err := NewSourceAdmission(SourceAdmissionInput{ID: uuid.NewString(), Stream: stream, Destination: destination, Contract: AdmissionContract{ID: "synthetic", Version: 1, Digest: sha256.Sum256([]byte("synthetic"))}, Schemas: []string{env.EventType()}, AuthorityRef: "synthetic", ScopeRef: "synthetic", Purpose: "synthetic", Bootstrap: BootstrapEvidence{Ref: "synthetic", EmptyStreamRef: "synthetic-new-empty"}})
			if err != nil {
				return err
			}
			if err := ad.Admit(ctx, admission); err != nil {
				return err
			}
		}
		if err := a.Append(ctx, eventstore.Batch{Events: []eventstore.Event{e}}); err != nil {
			return err
		}
		rule, err := NewSourcePlanRule(stream, "synthetic", []string{env.EventType()}, "synthetic-empty-authority")
		if err != nil {
			return err
		}
		message, err := NewMessage(env, schema, rule)
		if err != nil {
			return err
		}
		i, err := NewCustodyInserter(tx, events.OwnerInventory, ad)
		if err != nil {
			return err
		}
		return i.InsertMessages(ctx, message)
	})
	if err != nil && !(rollback && errors.Is(err, rolledBack)) {
		t.Fatal("seed through actual append/writer", err)
	}
	return env
}

func relayStore(t *testing.T, f *relayPG, lease time.Duration) *RelayStore {
	t.Helper()
	s, err := NewRelayStore(f.db, events.OwnerInventory, f.mode, lease)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func relayMustSQL(t *testing.T, f *relayPG, q string) {
	t.Helper()
	if out, err := f.admin(q); err != nil {
		t.Fatal(out, err)
	}
}
func relayClaim(t *testing.T, s *RelayStore) RelayLease {
	t.Helper()
	l, err := s.Claim(context.Background())
	if err != nil || l.IsEmpty() {
		t.Fatal("claim", err)
	}
	return l
}
func relayState(t *testing.T, s *RelayStore, l RelayLease, want DeliveryState) {
	t.Helper()
	got, err := s.Reconcile(context.Background(), l)
	if err != nil || got != want {
		t.Fatalf("state %s want %s: %v", got, want, err)
	}
}

func TestRelayPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_OUTBOX_RELAY") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_OUTBOX_RELAY=1 for owned pinned PostgreSQL fixtures")
	}
	for _, variant := range []struct {
		name     string
		mode     RelayMode
		additive bool
	}{{"original legacy", RelayLegacy, false}, {"additive legacy", RelayLegacy, true}, {"custody", RelayCustody, true}} {
		t.Run(variant.name, func(t *testing.T) {
			f := newRelayPG(t, variant.mode, variant.additive)
			s := relayStore(t, f, 3*time.Second)
			ctx := context.Background()
			if err := s.Check(ctx); err != nil {
				t.Fatal("readiness", err)
			}
			isolate := func() {
				relayMustSQL(t, f, "UPDATE "+s.table()+" SET next_attempt_at=clock_timestamp()+interval '1 day' WHERE sent_at IS NULL")
			}
			t.Run("rolled back domain commit has no outbound publication", func(t *testing.T) {
				isolate()
				env := f.seedOutcome(t, true, events.OwnerRetail)
				var eventsCount int64
				if err := f.db.Raw("SELECT count(*) FROM eventstore.events WHERE event_id=?::uuid", env.EventID()).Scan(&eventsCount).Error; err != nil || eventsCount != 0 {
					t.Fatal("rolled back event retained", err)
				}
				called := false
				r, _ := NewRelay(s, relayPublisherFunc(func(context.Context, Publication) (RoutedConfirmation, error) {
					called = true
					return RoutedConfirmation{}, nil
				}))
				l, err := r.Once(ctx)
				if err != nil || !l.IsEmpty() || called {
					t.Fatal("rolled back event published", err)
				}
			})
			t.Run("committed claim immutable bytes and partial recipient completion", func(t *testing.T) {
				isolate()
				targets := []events.Owner{events.OwnerRetail}
				if variant.mode == RelayCustody {
					targets = append(targets, events.OwnerDocuments)
				}
				env := f.seed(t, targets...)
				body, _ := env.MarshalJSON()
				for range targets {
					l := relayClaim(t, s)
					if l.publication.eventID != env.EventID() || !bytes.Equal(l.publication.body, body) {
						t.Fatal("retained publication changed")
					}
					relayState(t, s, l, DeliveryLeased)
					if err := s.MarkSent(ctx, l, RoutedConfirmation{}); !errors.Is(err, ErrRelayLease) {
						t.Fatal("empty receipt", err)
					}
					if err := s.MarkSent(ctx, l, RoutedConfirmation{l.publication, true}); err != nil {
						t.Fatal(err)
					}
					relayState(t, s, l, DeliverySent)
					if err := s.Retry(ctx, l); !errors.Is(err, ErrRelayLease) {
						t.Fatal("retry changed sent", err)
					}
				}
				idle, err := s.Claim(ctx)
				if err != nil || !idle.IsEmpty() {
					t.Fatal("already sent claimed", err)
				}
			})
			t.Run("concurrent workers claim each row once", func(t *testing.T) {
				isolate()
				for range 8 {
					f.seed(t, events.OwnerRetail)
				}
				var wg sync.WaitGroup
				results := make(chan RelayLease, 12)
				errs := make(chan error, 12)
				for range 12 {
					wg.Go(func() { l, err := s.Claim(ctx); results <- l; errs <- err })
				}
				wg.Wait()
				close(results)
				close(errs)
				for err := range errs {
					if err != nil {
						t.Fatal(err)
					}
				}
				seen := map[string]bool{}
				for l := range results {
					if l.IsEmpty() {
						continue
					}
					key := l.publication.eventID + "/" + l.destination
					if seen[key] {
						t.Fatal("double lease", key)
					}
					seen[key] = true
				}
				if len(seen) != 8 {
					t.Fatal("claim coverage", len(seen))
				}
			})
			t.Run("expired fence rejects late success and retries identical bytes", func(t *testing.T) {
				isolate()
				f.seed(t, events.OwnerRetail)
				old := relayClaim(t, s)
				relayMustSQL(t, f, "UPDATE "+s.table()+" SET lease_until=clock_timestamp()-interval '1 second' WHERE event_id='"+old.publication.eventID+"'")
				current := relayClaim(t, s)
				if current.token == old.token || !bytes.Equal(current.publication.body, old.publication.body) {
					t.Fatal("retry identity")
				}
				if err := s.MarkSent(ctx, old, RoutedConfirmation{old.publication, true}); !errors.Is(err, ErrRelayLease) {
					t.Fatal("stale completed", err)
				}
				if err := s.MarkSent(ctx, current, RoutedConfirmation{old.publication, true}); !errors.Is(err, ErrRelayLease) {
					t.Fatal("old attempt receipt reused", err)
				}
				if err := s.MarkSent(ctx, current, RoutedConfirmation{current.publication, true}); err != nil {
					t.Fatal(err)
				}
			})
			t.Run("completion waits then checks current database time", func(t *testing.T) {
				isolate()
				f.seed(t, events.OwnerRetail)
				short := relayStore(t, f, 150*time.Millisecond)
				l := relayClaim(t, short)
				tx := f.db.Begin()
				if tx.Error != nil {
					t.Fatal(tx.Error)
				}
				if err := tx.Exec("SELECT 1 FROM "+s.table()+" WHERE event_id=?::uuid FOR UPDATE", l.publication.eventID).Error; err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { done <- short.MarkSent(ctx, l, RoutedConfirmation{l.publication, true}) }()
				time.Sleep(200 * time.Millisecond)
				_ = tx.Rollback().Error
				if err := <-done; !errors.Is(err, ErrRelayLease) {
					t.Fatal("wait used stale clock", err)
				}
				relayState(t, short, l, DeliveryPending)
			})
			t.Run("NULL or foreign lease has no completion authority", func(t *testing.T) {
				isolate()
				f.seed(t, events.OwnerRetail)
				l := relayClaim(t, s)
				other := relayStore(t, f, time.Second)
				if err := other.MarkSent(ctx, l, RoutedConfirmation{l.publication, true}); !errors.Is(err, ErrRelayLease) {
					t.Fatal("foreign issuing store", err)
				}
				relayMustSQL(t, f, "UPDATE "+s.table()+" SET lease_owner=NULL,lease_until=NULL WHERE event_id='"+l.publication.eventID+"'")
				if err := s.MarkSent(ctx, l, RoutedConfirmation{l.publication, true}); !errors.Is(err, ErrRelayLease) {
					t.Fatal("NULL lease completed", err)
				}
				relayState(t, s, l, DeliveryPending)
			})
			t.Run("unknown claim and completion reconcile commit and rollback", func(t *testing.T) {
				for _, committed := range []bool{true, false} {
					isolate()
					f.seed(t, events.OwnerRetail)
					fault := relayStore(t, f, 3*time.Second)
					normalDB := fault.db
					fault.db = relayFaultDB(t, normalDB, committed)
					l, err := fault.Claim(ctx)
					if !errors.Is(err, ErrRelayUnknown) || l.IsEmpty() {
						t.Fatal("unknown claim", err)
					}
					fault.db = normalDB
					want := DeliveryPending
					if committed {
						want = DeliveryLeased
					}
					relayState(t, fault, l, want)
					called := false
					publisher, _ := NewAMQPPublisher(func(context.Context) (*amqp.Connection, error) {
						called = true
						return nil, errors.New("unexpected dial")
					}, time.Second)
					if _, err := publisher.Publish(ctx, l.Publication()); !errors.Is(err, ErrInvalidRecord) || called {
						t.Fatal("unknown claim was publishable", err, called)
					}
					if committed {
						relayMustSQL(t, f, "UPDATE "+s.table()+" SET lease_until=clock_timestamp()-interval '1 second' WHERE event_id='"+l.publication.eventID+"'")
					}
					l = relayClaim(t, fault)
					fault.db = relayFaultDB(t, normalDB, committed)
					err = fault.MarkSent(ctx, l, RoutedConfirmation{l.publication, true})
					if !errors.Is(err, ErrRelayUnknown) {
						t.Fatal("unknown sent", err)
					}
					fault.db = normalDB
					want = DeliveryLeased
					if committed {
						want = DeliverySent
					}
					relayState(t, fault, l, want)
				}
			})
			t.Run("network outside SQL and lost confirm cannot mark sent", func(t *testing.T) {
				isolate()
				f.seed(t, events.OwnerRetail)
				var attempts int
				pub := relayPublisherFunc(func(ctx context.Context, p Publication) (RoutedConfirmation, error) {
					attempts++
					var leased int64
					if err := f.db.Raw("SELECT count(*) FROM "+s.table()+" WHERE event_id=?::uuid AND lease_owner IS NOT NULL", p.eventID).Scan(&leased).Error; err != nil || leased != 1 {
						t.Fatal("claim not committed before network", err)
					}
					tx := f.db.WithContext(ctx).Begin()
					if err := tx.Exec("SELECT 1 FROM "+s.table()+" WHERE event_id=?::uuid FOR UPDATE NOWAIT", p.eventID).Error; err != nil {
						t.Fatal("network has row lock", err)
					}
					_ = tx.Rollback().Error
					return RoutedConfirmation{}, ErrPublishUnavailable
				})
				r, _ := NewRelay(s, pub)
				l, err := r.Once(ctx)
				if !errors.Is(err, ErrPublishUnavailable) || attempts != 1 {
					t.Fatal("lost confirm", err, attempts)
				}
				relayState(t, s, l, DeliveryPending)
			})
			t.Run("persisted attempts drive bounded exponential retry scheduling", func(t *testing.T) {
				for _, prior := range []int64{0, 6, 1000} {
					isolate()
					env := f.seed(t, events.OwnerRetail)
					relayMustSQL(t, f, fmt.Sprintf("UPDATE %s SET attempts=%d WHERE event_id='%s'", s.table(), prior, env.EventID()))
					l := relayClaim(t, s)
					var before, after, next time.Time
					if err := f.db.Raw("SELECT clock_timestamp()").Scan(&before).Error; err != nil {
						t.Fatal(err)
					}
					if err := s.Retry(ctx, l); err != nil {
						t.Fatal(err)
					}
					if err := f.db.Raw("SELECT clock_timestamp()").Scan(&after).Error; err != nil {
						t.Fatal(err)
					}
					if err := f.db.Raw("SELECT next_attempt_at FROM "+s.table()+" WHERE event_id=?::uuid", env.EventID()).Scan(&next).Error; err != nil {
						t.Fatal(err)
					}
					low, _ := relayRetryDelay(prior+1, 0)
					high, _ := relayRetryDelay(prior+1, 1)
					if next.Before(before.Add(low)) || next.After(after.Add(high)) {
						t.Fatal("persisted retry outside approved delay", prior, next, before, after)
					}
					idle, err := s.Claim(ctx)
					if err != nil || !idle.IsEmpty() {
						t.Fatal("retry due immediately", err)
					}
				}
			})
			t.Run("owner and privilege failures before work", func(t *testing.T) {
				isolate()
				f.seed(t, events.OwnerRetail)
				foreign, _ := NewRelayStore(f.db, events.OwnerCommerce, variant.mode, time.Second)
				if err := foreign.Check(ctx); !errors.Is(err, ErrRelayReadiness) {
					t.Fatal("foreign owner", err)
				}
				for _, grant := range []struct{ grant, revoke string }{{"GRANT UPDATE(envelope) ON eventstore.outbox TO justix_inventory_runtime", "REVOKE UPDATE(envelope) ON eventstore.outbox FROM justix_inventory_runtime"}, {"GRANT DELETE ON " + s.table() + " TO PUBLIC", "REVOKE DELETE ON " + s.table() + " FROM PUBLIC"}} {
					if variant.mode == RelayCustody && strings.Contains(grant.grant, "UPDATE(envelope)") {
						grant.grant = "GRANT UPDATE(envelope) ON eventstore.outbox_messages TO justix_inventory_runtime"
						grant.revoke = "REVOKE UPDATE(envelope) ON eventstore.outbox_messages FROM justix_inventory_runtime"
					}
					relayMustSQL(t, f, grant.grant)
					_, err := s.Claim(ctx)
					relayMustSQL(t, f, grant.revoke)
					if !errors.Is(err, ErrRelayReadiness) {
						t.Fatal("excess privilege accepted", err)
					}
				}
				relayMustSQL(t, f, "CREATE ROLE t012_reachable NOINHERIT; CREATE ROLE t012_hop NOINHERIT; GRANT t012_reachable TO t012_hop; GRANT t012_hop TO justix_inventory_runtime; GRANT UPDATE(envelope) ON eventstore.outbox TO t012_reachable")
				if variant.mode == RelayCustody {
					relayMustSQL(t, f, "GRANT UPDATE(envelope) ON eventstore.outbox_messages TO t012_reachable")
				}
				_, err := s.Claim(ctx)
				relayMustSQL(t, f, "REVOKE t012_hop FROM justix_inventory_runtime")
				if !errors.Is(err, ErrRelayReadiness) {
					t.Fatal("reachable role accepted", err)
				}
				relayMustSQL(t, f, "REVOKE UPDATE(sent_at) ON "+s.table()+" FROM justix_inventory_runtime")
				_, err = s.Claim(ctx)
				relayMustSQL(t, f, "GRANT UPDATE(sent_at) ON "+s.table()+" TO justix_inventory_runtime")
				if !errors.Is(err, ErrRelayReadiness) {
					t.Fatal("missing completion privilege", err)
				}
			})
			t.Run("DDL metadata insertion and delegation fail closed", func(t *testing.T) {
				isolate()
				f.seed(t, events.OwnerRetail)
				cases := []struct{ grant, revoke string }{
					{"GRANT TEMPORARY ON DATABASE justix_inventory TO PUBLIC", "REVOKE TEMPORARY ON DATABASE justix_inventory FROM PUBLIC"},
					{"GRANT CREATE ON DATABASE justix_inventory TO justix_inventory_runtime", "REVOKE CREATE ON DATABASE justix_inventory FROM justix_inventory_runtime"},
					{"GRANT CREATE ON SCHEMA public TO justix_inventory_runtime", "REVOKE CREATE ON SCHEMA public FROM justix_inventory_runtime"},
					{"GRANT SELECT ON " + s.table() + " TO justix_inventory_runtime WITH GRANT OPTION", "REVOKE GRANT OPTION FOR SELECT ON " + s.table() + " FROM justix_inventory_runtime"},
					{"GRANT UPDATE(sent_at) ON " + s.table() + " TO justix_inventory_runtime WITH GRANT OPTION", "REVOKE GRANT OPTION FOR UPDATE(sent_at) ON " + s.table() + " FROM justix_inventory_runtime"},
				}
				if variant.additive {
					for _, table := range []string{"eventstore.messaging_mode", "eventstore.messaging_route_compatibility"} {
						cases = append(cases, struct{ grant, revoke string }{"GRANT INSERT ON " + table + " TO justix_inventory_runtime", "REVOKE INSERT ON " + table + " FROM justix_inventory_runtime"}, struct{ grant, revoke string }{"GRANT INSERT(singleton) ON " + table + " TO justix_inventory_runtime", "REVOKE INSERT(singleton) ON " + table + " FROM justix_inventory_runtime"}, struct{ grant, revoke string }{"GRANT SELECT ON " + table + " TO justix_inventory_runtime WITH GRANT OPTION", "REVOKE GRANT OPTION FOR SELECT ON " + table + " FROM justix_inventory_runtime"})
					}
				}
				for _, c := range cases {
					relayMustSQL(t, f, c.grant)
					_, err := s.Claim(ctx)
					relayMustSQL(t, f, c.revoke)
					if !errors.Is(err, ErrRelayReadiness) {
						t.Fatal("incompatible privilege accepted", c.grant, err)
					}
				}
				if variant.additive {
					relayMustSQL(t, f, "GRANT t012_hop TO justix_inventory_runtime; GRANT INSERT(singleton) ON eventstore.messaging_mode TO t012_reachable")
					_, err := s.Claim(ctx)
					relayMustSQL(t, f, "REVOKE t012_hop FROM justix_inventory_runtime")
					if !errors.Is(err, ErrRelayReadiness) {
						t.Fatal("reachable marker INSERT accepted", err)
					}
				}
			})
			if variant.mode == RelayCustody {
				t.Run("holds exclude claims and fence already claimed work", func(t *testing.T) {
					isolate()
					f.seed(t, events.OwnerRetail)
					l := relayClaim(t, s)
					relayMustSQL(t, f, "UPDATE eventstore.outbox_deliveries SET hold_ref='synthetic-hold' WHERE event_id='"+l.publication.eventID+"'")
					if err := s.MarkSent(ctx, l, RoutedConfirmation{l.publication, true}); !errors.Is(err, ErrRelayLease) {
						t.Fatal("held completed", err)
					}
					relayState(t, s, l, DeliveryHeld)
					idle, err := s.Claim(ctx)
					if err != nil || !idle.IsEmpty() {
						t.Fatal("held claimed", err)
					}
				})
				t.Run("empty validated parent plan creates no publish claim", func(t *testing.T) {
					isolate()
					f.seed(t)
					l, err := s.Claim(ctx)
					if err != nil || !l.IsEmpty() {
						t.Fatal("empty plan publication", err)
					}
				})
				t.Run("legacy mode cannot execute custody rows", func(t *testing.T) {
					legacy, _ := NewRelayStore(f.db, events.OwnerInventory, RelayLegacy, time.Second)
					if err := legacy.Check(ctx); err == nil {
						t.Fatal("custody accepted legacy")
					}
				})
			} else {
				t.Run("custody requires explicit installed activation", func(t *testing.T) {
					custody, _ := NewRelayStore(f.db, events.OwnerInventory, RelayCustody, time.Second)
					if err := custody.Check(ctx); err == nil {
						t.Fatal("implicit custody")
					}
				})
			}
		})
	}
}

type relayFaultPool struct {
	*sql.DB
	committed bool
}

func (p relayFaultPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &relayFaultTx{tx, p.committed}, nil
}

type relayFaultTx struct {
	*sql.Tx
	committed bool
}

func (t relayFaultTx) Commit() error {
	var err error
	if t.committed {
		err = t.Tx.Commit()
	} else {
		err = t.Tx.Rollback()
	}
	if err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}
func relayFaultDB(t *testing.T, db *gorm.DB, commit bool) *gorm.DB {
	t.Helper()
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	isolated := db.Session(&gorm.Session{NewDB: true, Initialized: true})
	isolated.Statement.ConnPool = relayFaultPool{pool, commit}
	return isolated
}
