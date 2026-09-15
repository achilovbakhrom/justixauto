package eventstore_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
)

func TestFenceRequestsAreSealedAndCanonical(t *testing.T) {
	id := uuid.NewString()
	for _, bad := range []string{"", uuid.Nil.String(), strings.ToUpper(id), "{" + id + "}", " " + id} {
		if _, err := eventstore.ReceiverFence(events.OwnerInventory, events.OwnerRetail, "vehicle", bad); err == nil {
			t.Fatalf("accepted ID %q", bad)
		}
	}
	for _, bad := range []string{"", "vehicle:other", "Vehicle", "vehicle.name"} {
		if _, err := eventstore.SourceFence(events.OwnerInventory, bad, id); err == nil {
			t.Fatal("accepted aggregate", bad)
		}
	}
	if _, err := eventstore.GenerationFence(events.OwnerInventory, "view", id, 0); err == nil {
		t.Fatal("zero mode")
	}
	if _, err := eventstore.GuardFence(events.OwnerInventory, "guard", "", eventstore.SharedFence); err == nil {
		t.Fatal("empty scope")
	}
	if _, err := eventstore.GuardFence(events.OwnerInventory, "guard", "a\x00b", eventstore.SharedFence); err == nil {
		t.Fatal("NUL scope")
	}
	if _, err := eventstore.GuardFence(events.OwnerInventory, "guard", string([]byte{255}), eventstore.SharedFence); err == nil {
		t.Fatal("invalid UTF8")
	}
	if _, err := eventstore.ReceiverFence("outside", events.OwnerRetail, "vehicle", id); err == nil {
		t.Fatal("unknown owner")
	}
	if _, err := eventstore.PrepareFences(context.Background(), nil, events.OwnerInventory, eventstore.SharedFence); !errors.Is(err, eventstore.ErrTransactionRequired) {
		t.Fatal(err)
	}
	var empty eventstore.PreparedFences
	if err := empty.Require(context.Background(), nil, events.OwnerInventory, eventstore.SharedFence); !errors.Is(err, eventstore.ErrFencePreparation) {
		t.Fatal(err)
	}
}

func fenceBegin(t *testing.T, db *gorm.DB) *gorm.DB {
	t.Helper()
	tx := db.Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	return tx
}
func fenceRequest(t *testing.T, r eventstore.FenceRequest, err error) eventstore.FenceRequest {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func fenceTry(t *testing.T, tx *gorm.DB, key string, shared bool) bool {
	t.Helper()
	query := "SELECT pg_try_advisory_xact_lock(hashtextextended(?,0))"
	if shared {
		query = "SELECT pg_try_advisory_xact_lock_shared(hashtextextended(?,0))"
	}
	var acquired bool
	if err := tx.Raw(query, key).Scan(&acquired).Error; err != nil {
		t.Fatal(err)
	}
	return acquired
}
func fenceEncoded(namespace string, parts ...string) string {
	for _, p := range parts {
		namespace += fmt.Sprintf(":%d:%s", len(p), p)
	}
	return namespace
}

// Deliberately not a transaction: proves implementing GORM's committer
// interface on a pool cannot manufacture transaction-scoped lock custody.
type falseFenceTransaction struct{ *sql.DB }

func (falseFenceTransaction) Commit() error   { return nil }
func (falseFenceTransaction) Rollback() error { return nil }

func TestPreparedReceiverFencesPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_RECEIVER_FENCES") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_RECEIVER_FENCES=1 for owned PostgreSQL fixture")
	}
	f := newRouteFixture(t)
	s := checkpointStore(t, f)
	ctx := context.Background()
	owner := events.OwnerInventory
	t.Run("complete coverage cloned sessions foreign transaction ended isolation pool and no second preparation", func(t *testing.T) {
		tx := fenceBegin(t, s.db)
		stream := uuid.NewString()
		r, e := eventstore.ReceiverFence(owner, events.OwnerRetail, "vehicle", stream)
		receiver := fenceRequest(t, r, e)
		r, e = eventstore.SourceFence(owner, "vehicle", stream)
		source := fenceRequest(t, r, e)
		r, e = eventstore.GenerationFence(owner, "view", uuid.NewString(), eventstore.SharedFence)
		generation := fenceRequest(t, r, e)
		r, e = eventstore.GuardFence(owner, "fixture-vin-guard", "company:fixture", eventstore.SharedFence)
		guard := fenceRequest(t, r, e)
		p, err := eventstore.PrepareFences(ctx, tx, owner, eventstore.SharedFence, guard, receiver, source, generation)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Require(ctx, tx.Session(&gorm.Session{NewDB: true}), owner, eventstore.SharedFence, source, receiver, guard, generation); err != nil {
			t.Fatal(err)
		}
		r, e = eventstore.ReceiverFence(owner, events.OwnerRetail, "vehicle", uuid.NewString())
		missing := fenceRequest(t, r, e)
		if err := p.Require(ctx, tx, owner, eventstore.SharedFence, missing); !errors.Is(err, eventstore.ErrFencePreparation) {
			t.Fatal("missing key", err)
		}
		r, e = eventstore.GuardFence(owner, "fixture-vin-guard", "company:fixture", eventstore.ExclusiveFence)
		stronger := fenceRequest(t, r, e)
		if err := p.Require(ctx, tx, owner, eventstore.SharedFence, stronger); !errors.Is(err, eventstore.ErrFencePreparation) {
			t.Fatal("missing exclusive mode", err)
		}
		if err := p.Require(ctx, tx, owner, eventstore.ExclusiveFence); !errors.Is(err, eventstore.ErrFencePreparation) {
			t.Fatal("catalog upgrade", err)
		}
		if _, err := eventstore.PrepareFences(ctx, tx, owner, eventstore.ExclusiveFence, missing); !errors.Is(err, eventstore.ErrFencePreparation) {
			t.Fatal("second preparation", err)
		}
		other := fenceBegin(t, s.db)
		if err := p.Require(ctx, other, owner, eventstore.SharedFence, receiver); !errors.Is(err, eventstore.ErrFencePreparation) {
			t.Fatal("foreign transaction", err)
		}
		if _, err := eventstore.PrepareFences(ctx, s.db, owner, eventstore.SharedFence, receiver); !errors.Is(err, eventstore.ErrTransactionRequired) {
			t.Fatal("pool", err)
		}
		pool, err := s.db.DB()
		if err != nil {
			t.Fatal(err)
		}
		fake, err := gorm.Open(postgres.New(postgres.Config{Conn: falseFenceTransaction{pool}}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := eventstore.PrepareFences(ctx, fake, owner, eventstore.SharedFence); !errors.Is(err, eventstore.ErrFencePreparation) {
			t.Fatal("committer-shaped pool accepted", err)
		}
		if err := p.Require(ctx, s.db, owner, eventstore.SharedFence); !errors.Is(err, eventstore.ErrTransactionRequired) {
			t.Fatal("pool capability", err)
		}
		tx.Rollback()
		if err := p.Require(ctx, tx, owner, eventstore.SharedFence); err == nil {
			t.Fatal("ended transaction accepted")
		}
		serial := s.db.Begin(&sql.TxOptions{Isolation: sql.LevelSerializable})
		defer serial.Rollback()
		if _, err := eventstore.PrepareFences(ctx, serial, owner, eventstore.SharedFence); !errors.Is(err, eventstore.ErrFencePreparation) {
			t.Fatal("wrong isolation", err)
		}
		zero := fenceBegin(t, s.db)
		if _, err := eventstore.PrepareFences(ctx, zero, owner, eventstore.SharedFence, eventstore.FenceRequest{}); !errors.Is(err, eventstore.ErrFencePreparation) {
			t.Fatal("unsealed request", err)
		}
	})
	t.Run("exact source receiver keys and length prefixed non alias guard keys", func(t *testing.T) {
		tx := fenceBegin(t, s.db)
		id := uuid.NewString()
		r, e := eventstore.SourceFence(owner, "vehicle", id)
		source := fenceRequest(t, r, e)
		r, e = eventstore.ReceiverFence(owner, events.OwnerRetail, "vehicle", id)
		receiver := fenceRequest(t, r, e)
		r, e = eventstore.GuardFence(owner, "guard:a", "b", eventstore.SharedFence)
		first := fenceRequest(t, r, e)
		r, e = eventstore.GuardFence(owner, "guard", "a:b", eventstore.ExclusiveFence)
		second := fenceRequest(t, r, e)
		gen := uuid.NewString()
		r, e = eventstore.GenerationFence(owner, "вид:cars", gen, eventstore.SharedFence)
		generation := fenceRequest(t, r, e)
		if _, err := eventstore.PrepareFences(ctx, tx, owner, eventstore.SharedFence, source, receiver, first, generation); err != nil {
			t.Fatal(err)
		}
		probe := fenceBegin(t, s.db)
		for _, key := range []string{"justixauto:eventstore:inventory:vehicle:" + id, "justixauto:receiver:inventory:retail:vehicle:" + id, fenceEncoded("justixauto:command-guard", "inventory", "guard:a", "b"), fenceEncoded("justixauto:projection-generation", "inventory", "вид:cars", gen)} {
			if fenceTry(t, probe, key, false) {
				t.Fatal("expected held exact key", key)
			}
		}
		if !fenceTry(t, probe, fenceEncoded("justixauto:command-guard", "inventory", "guard", "a:b"), false) {
			t.Fatal("delimiter alias")
		}
		probe.Rollback()
		independent := fenceBegin(t, s.db)
		if _, err := eventstore.PrepareFences(ctx, independent, owner, eventstore.SharedFence, second); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("shared writers coexist exclusive rebuild blocks then succeeds strongest duplicate", func(t *testing.T) {
		id := uuid.NewString()
		r, e := eventstore.GenerationFence(owner, "projection", id, eventstore.SharedFence)
		shared := fenceRequest(t, r, e)
		r, e = eventstore.GenerationFence(owner, "projection", id, eventstore.ExclusiveFence)
		exclusive := fenceRequest(t, r, e)
		a, b := fenceBegin(t, s.db), fenceBegin(t, s.db)
		if _, err := eventstore.PrepareFences(ctx, a, owner, eventstore.SharedFence, shared); err != nil {
			t.Fatal(err)
		}
		if _, err := eventstore.PrepareFences(ctx, b, owner, eventstore.SharedFence, shared); err != nil {
			t.Fatal(err)
		}
		blocked := fenceBegin(t, s.db)
		blocked.Exec("SET LOCAL lock_timeout='150ms'")
		if _, err := eventstore.PrepareFences(ctx, blocked, owner, eventstore.SharedFence, exclusive); err == nil || !strings.Contains(err.Error(), "55P03") {
			t.Fatal("exclusive did not time out", err)
		}
		blocked.Rollback()
		a.Rollback()
		b.Rollback()
		x := fenceBegin(t, s.db)
		p, err := eventstore.PrepareFences(ctx, x, owner, eventstore.SharedFence, shared, exclusive, shared)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Require(ctx, x, owner, eventstore.SharedFence, exclusive); err != nil {
			t.Fatal(err)
		}
		probe := fenceBegin(t, s.db)
		if fenceTry(t, probe, fenceEncoded("justixauto:projection-generation", "inventory", "projection", id), true) {
			t.Fatal("strongest duplicate did not take exclusive lock")
		}
	})
	t.Run("catalog exclusive prevents phantom membership and source precedes catalog", func(t *testing.T) {
		catalog := fenceBegin(t, s.db)
		if _, err := eventstore.PrepareFences(ctx, catalog, owner, eventstore.ExclusiveFence); err != nil {
			t.Fatal(err)
		}
		id := uuid.NewString()
		r, e := eventstore.SourceFence(owner, "vehicle", id)
		source := fenceRequest(t, r, e)
		waiting := fenceBegin(t, s.db)
		waiting.Exec("SET LOCAL lock_timeout='2s'")
		result := make(chan error, 1)
		go func() {
			_, err := eventstore.PrepareFences(ctx, waiting, owner, eventstore.SharedFence, source)
			result <- err
		}()
		// Observe the precise source lock while its holder waits for catalog.
		var held bool
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			probe := s.db.Begin()
			acquired := fenceTry(t, probe, "justixauto:eventstore:inventory:vehicle:"+id, false)
			probe.Rollback()
			if !acquired {
				held = true
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !held {
			t.Fatal("source prerequisite not acquired before catalog")
		}
		select {
		case err := <-result:
			t.Fatalf("catalog did not block %v", err)
		default:
		}
		catalog.Rollback()
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	})
	t.Run("missing prerequisite suppresses effect and cancellation releases prepared set", func(t *testing.T) {
		id := uuid.NewString()
		r, e := eventstore.GuardFence(owner, "fixture-guard", id, eventstore.SharedFence)
		guard := fenceRequest(t, r, e)
		runner, err := eventstore.NewTransactions(s.db, func(tx *gorm.DB) (*gorm.DB, error) { return tx, nil })
		if err != nil {
			t.Fatal(err)
		}
		called := false
		err = runner.Run(ctx, func(tx *gorm.DB) error {
			p, err := eventstore.PrepareFences(ctx, tx, owner, eventstore.SharedFence)
			if err != nil {
				return err
			}
			if err := p.Require(ctx, tx, owner, eventstore.SharedFence, guard); err != nil {
				return err
			}
			called = true
			return nil
		})
		if !errors.Is(err, eventstore.ErrFencePreparation) || called {
			t.Fatal("missing key reached effect", err)
		}
		cancelCtx, cancel := context.WithCancel(ctx)
		err = runner.Run(cancelCtx, func(tx *gorm.DB) error {
			_, err := eventstore.PrepareFences(cancelCtx, tx, owner, eventstore.SharedFence, guard)
			cancel()
			return err
		})
		if err == nil {
			t.Fatal("cancelled transaction committed")
		}
		probe := fenceBegin(t, s.db)
		if !fenceTry(t, probe, fenceEncoded("justixauto:command-guard", "inventory", "fixture-guard", id), false) {
			t.Fatal("cancel retained lock")
		}
	})
	t.Run("reversed receiver and generation guard requests acquire sorted keys", func(t *testing.T) {
		for _, phase := range []string{"receiver", "generation-guard"} {
			aID, bID := uuid.NewString(), uuid.NewString()
			if aID > bID {
				aID, bID = bID, aID
			}
			var first, second eventstore.FenceRequest
			var firstKey, secondKey string
			var err error
			if phase == "receiver" {
				first, err = eventstore.ReceiverFence(owner, events.OwnerRetail, "vehicle", aID)
				if err != nil {
					t.Fatal(err)
				}
				second, err = eventstore.ReceiverFence(owner, events.OwnerRetail, "vehicle", bID)
				if err != nil {
					t.Fatal(err)
				}
				firstKey = "justixauto:receiver:inventory:retail:vehicle:" + aID
				secondKey = "justixauto:receiver:inventory:retail:vehicle:" + bID
			} else {
				first, err = eventstore.GuardFence(owner, "fixture-guard", aID, eventstore.ExclusiveFence)
				if err != nil {
					t.Fatal(err)
				}
				second, err = eventstore.GenerationFence(owner, "view", bID, eventstore.ExclusiveFence)
				if err != nil {
					t.Fatal(err)
				}
				firstKey = fenceEncoded("justixauto:command-guard", "inventory", "fixture-guard", aID)
				secondKey = fenceEncoded("justixauto:projection-generation", "inventory", "view", bID)
			}
			blocker := fenceBegin(t, s.db)
			if !fenceTry(t, blocker, firstKey, false) {
				t.Fatal("fixture lock unavailable")
			}
			waiting := fenceBegin(t, s.db)
			waiting.Exec("SET LOCAL lock_timeout='2s'")
			var pid int
			if err := waiting.Raw("SELECT pg_backend_pid()").Scan(&pid).Error; err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				_, err := eventstore.PrepareFences(ctx, waiting, owner, eventstore.SharedFence, second, first)
				result <- err
			}()
			observed := false
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				if err := s.db.Raw("SELECT coalesce(wait_event='advisory',false) FROM pg_stat_activity WHERE pid=?", pid).Scan(&observed).Error; err != nil {
					t.Fatal(err)
				}
				if observed {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !observed {
				t.Fatal("preparation did not wait on first sorted key", phase)
			}
			probe := fenceBegin(t, s.db)
			if !fenceTry(t, probe, secondKey, false) {
				t.Fatal("later key acquired before earlier key", phase)
			}
			probe.Rollback()
			blocker.Rollback()
			if err := <-result; err != nil {
				t.Fatal(err)
			}
			waiting.Rollback()
		}
	})
}
