package commands

import (
	"context"
	"errors"
	"math"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"justixauto/pkg/eventstore"
)

func TestOperationRetryBounds(t *testing.T) {
	for _, attempt := range []int64{1, 2, 3, 6, 7, 8, 100, math.MaxInt64} {
		for _, jitter := range []float64{0, 0.2, 0.5, 1} {
			d, err := RetryDelay(attempt, jitter)
			if err != nil || d < time.Second || d > time.Minute {
				t.Fatal(attempt, jitter, d, err)
			}
		}
	}
	for _, sample := range []float64{-1, 2, math.NaN(), math.Inf(1)} {
		if _, err := RetryDelay(1, sample); err == nil {
			t.Fatal(sample)
		}
	}
	if _, err := RetryDelay(0, 0); err == nil {
		t.Fatal("zero attempt")
	}
	d, _ := RetryDelay(1, 1)
	if d != time.Second {
		t.Fatal(d)
	}
	d, _ = RetryDelay(7, 1)
	if d != time.Minute {
		t.Fatal(d)
	}
}

func noProcessAlert(context.Context, Operation[outcome, processError]) error { return nil }

func TestOperationWorkerPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_OPERATIONS") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_OPERATIONS=1 for disposable PostgreSQL fixture")
	}
	db := fixtureDB(t)
	ctx := context.Background()
	t.Run("attention scanner includes active leases and future scheduling", func(t *testing.T) {
		p := processPolicy(t)
		i := processIntent()
		hash, _ := p.Schema.digest(request)
		leaseID := uuid.NewString()
		err := db.Exec(`INSERT INTO eventstore.operations(operation_id,kind,aggregate_id,actor_id,company_id,admission_ref,intent_revision,request_hash,phase,revision,created_at,next_attempt_at,lease_owner,lease_until) VALUES (?,?,?,?,?,?,0,?,'pending',1,clock_timestamp()-interval '6 minutes',clock_timestamp()+interval '1 hour',?,clock_timestamp()+interval '1 hour')`, i.ID, p.Schema.command, i.AggregateID, i.ActorID, i.CompanyID, i.AdmissionRef, hash[:], leaseID).Error
		if err != nil {
			t.Fatal(err)
		}
		alerts := 0
		for _, want := range []int{1, 0} {
			err = processRun(db, p).Run(ctx, func(u processPorts) error {
				n, err := u.operations.MarkAttention(ctx, 10, func(_ context.Context, o Operation[outcome, processError]) error {
					alerts++
					if o.Decision != Undecided || o.Phase != "pending" || !o.AttentionRequired {
						t.Error(o)
					}
					return nil
				})
				if n != want {
					t.Error(n, want)
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		if alerts != 1 {
			t.Fatal(alerts)
		}
		var retained string
		if err = db.Raw(`SELECT lease_owner FROM eventstore.operations WHERE operation_id=?`, i.ID).Scan(&retained).Error; err != nil || retained != leaseID {
			t.Fatal("attention changed lease", err)
		}
	})
	claimOne := func(t *testing.T, p OperationPolicy[input, outcome, processError]) OperationClaim[outcome, processError] {
		t.Helper()
		var c OperationClaim[outcome, processError]
		err := processRun(db, p).Run(ctx, func(u processPorts) error {
			var ok bool
			var err error
			c, ok, err = u.operations.Claim(ctx, time.Minute, noProcessAlert)
			if err == nil && !ok {
				t.Error("no due work")
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	expire := func(t *testing.T, id string) {
		t.Helper()
		if err := db.Exec(`UPDATE eventstore.operations SET lease_until=clock_timestamp()-interval '1 second' WHERE operation_id=?`, id).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Run("exclusive claims stale fences resume retains decision", func(t *testing.T) {
		p := processPolicy(t)
		i := processCreate(t, db, p)
		var wg sync.WaitGroup
		claims := make(chan OperationClaim[outcome, processError], 6)
		errs := make(chan error, 6)
		for range 6 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var c OperationClaim[outcome, processError]
				var ok bool
				err := processRun(db, p).Run(ctx, func(u processPorts) error {
					var err error
					c, ok, err = u.operations.Claim(ctx, time.Minute, noProcessAlert)
					return err
				})
				errs <- err
				if err == nil && ok {
					claims <- c
				}
			}()
		}
		wg.Wait()
		close(claims)
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if len(claims) != 1 {
			t.Fatal("multiple leases", len(claims))
		}
		old := <-claims
		if err := processRun(db, p).Run(ctx, func(u processPorts) error {
			return u.operations.Resume(ctx, i.ID, OperationAccess{i.ActorID, i.CompanyID}, processAuth)
		}); err != nil {
			t.Fatal(err)
		}
		if err := processRun(db, p).Run(ctx, func(u processPorts) error {
			_, ok, err := u.operations.Claim(ctx, time.Minute, noProcessAlert)
			if ok {
				t.Error("resume revoked active lease")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		expire(t, i.ID)
		fresh := claimOne(t, p)
		if fresh.LeaseID == old.LeaseID || fresh.Operation.ID != old.Operation.ID || fresh.Operation.Decision != Undecided {
			t.Fatal("new identity or decision", fresh)
		}
		called := false
		err := processRun(db, p).Run(ctx, func(u processPorts) error {
			return u.operations.WithClaim(ctx, old, func(context.Context, Operation[outcome, processError]) error { called = true; return nil })
		})
		if !errors.Is(err, ErrLeaseLost) || called {
			t.Fatal("stale worker wrote", err)
		}
		err = processRun(db, p).Run(ctx, func(u processPorts) error {
			return u.operations.WithClaim(ctx, fresh, func(ctx context.Context, o Operation[outcome, processError]) error {
				return u.operations.Advance(ctx, o.ID, o.Revision, OperationChange[outcome, processError]{Phase: "finishing", Decision: Commit})
			})
		})
		if err != nil {
			t.Fatal(err)
		}
		next := claimOne(t, p)
		if next.Operation.Decision != Commit || next.Operation.Phase != "finishing" || next.Operation.ID != i.ID {
			t.Fatal(next)
		}
		err = processRun(db, p).Run(ctx, func(u processPorts) error { return u.operations.Retry(ctx, next, 1, noProcessAlert) })
		if err != nil {
			t.Fatal(err)
		}
		err = processRun(db, p).Run(ctx, func(u processPorts) error {
			o, err := processStatus(ctx, u, i)
			if err == nil && (o.Decision != Commit || o.Phase != "finishing" || o.Attempts != 3) {
				t.Error(o)
			}
			_, due, e := u.operations.Claim(ctx, time.Minute, noProcessAlert)
			if due {
				t.Error("retry ignored due timestamp")
			}
			if err != nil {
				return err
			}
			return e
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("attention and alert intent commit atomically with no terminal timeout", func(t *testing.T) {
		p := processPolicy(t)
		i := processIntent()
		hash, _ := p.Schema.digest(request)
		err := db.Exec(`INSERT INTO eventstore.operations(operation_id,kind,aggregate_id,actor_id,company_id,admission_ref,intent_revision,request_hash,phase,decision,revision,created_at) VALUES (?,?,?,?,?,?,0,?,'finishing','commit',1,clock_timestamp()-interval '10 days')`, i.ID, p.Schema.command, i.AggregateID, i.ActorID, i.CompanyID, i.AdmissionRef, hash[:]).Error
		if err != nil {
			t.Fatal(err)
		}
		injected := errors.New("alert intent unavailable")
		err = processRun(db, p).Run(ctx, func(u processPorts) error {
			_, _, err := u.operations.Claim(ctx, time.Minute, func(context.Context, Operation[outcome, processError]) error { return injected })
			return err
		})
		if !errors.Is(err, injected) {
			t.Fatal(err)
		}
		err = processRun(db, p).Run(ctx, func(u processPorts) error {
			o, err := processStatus(ctx, u, i)
			if o.AttentionRequired || o.Attempts != 0 {
				t.Error("partial alert/lease commit")
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		alertID := uuid.NewString()
		var c OperationClaim[outcome, processError]
		alerts := 0
		err = processRun(db, p).Run(ctx, func(u processPorts) error {
			var err error
			c, _, err = u.operations.Claim(ctx, time.Minute, func(context.Context, Operation[outcome, processError]) error {
				alerts++
				return u.tx.Exec(`INSERT INTO eventstore.fixture_effects(id) VALUES (?)`, alertID).Error
			})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if !c.AttentionNew || !c.Operation.AttentionRequired || c.Operation.Decision != Commit || c.Operation.Phase != "finishing" || alerts != 1 {
			t.Fatal(c, alerts)
		}
		expire(t, i.ID)
		again := claimOne(t, p)
		if again.AttentionNew || again.Operation.Decision != Commit || again.Operation.ID != i.ID {
			t.Fatal(again)
		}
	})
	t.Run("local completion rollback and terminal exclusion", func(t *testing.T) {
		p := processPolicy(t)
		i := processCreate(t, db, p)
		c := claimOne(t, p)
		injected := errors.New("local crash")
		err := processRun(db, p).Run(ctx, func(u processPorts) error {
			return u.operations.WithClaim(ctx, c, func(ctx context.Context, o Operation[outcome, processError]) error {
				if err := u.operations.Advance(ctx, o.ID, o.Revision, OperationChange[outcome, processError]{Phase: "finishing", Decision: Commit}); err != nil {
					return err
				}
				return injected
			})
		})
		if !errors.Is(err, injected) {
			t.Fatal(err)
		}
		err = processRun(db, p).Run(ctx, func(u processPorts) error {
			return u.operations.WithClaim(ctx, c, func(ctx context.Context, o Operation[outcome, processError]) error {
				if o.Decision != Undecided {
					t.Error("decision survived rollback")
				}
				return u.operations.Advance(ctx, o.ID, o.Revision, OperationChange[outcome, processError]{Phase: "finishing", Decision: Commit})
			})
		})
		if err != nil {
			t.Fatal(err)
		}
		c = claimOne(t, p)
		result := outcome{ID: uuid.NewString(), State: "created"}
		err = processRun(db, p).Run(ctx, func(u processPorts) error {
			return u.operations.WithClaim(ctx, c, func(ctx context.Context, o Operation[outcome, processError]) error {
				return u.operations.Advance(ctx, o.ID, o.Revision, OperationChange[outcome, processError]{Phase: "succeeded", Decision: Commit, Result: &result})
			})
		})
		if err != nil {
			t.Fatal(err)
		}
		err = processRun(db, p).Run(ctx, func(u processPorts) error {
			_, ok, err := u.operations.Claim(ctx, time.Minute, noProcessAlert)
			if ok {
				t.Error("terminal operation scheduled")
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = i
	})
	t.Run("unknown lease commit never dispatches and recovers same operation", func(t *testing.T) {
		for _, committed := range []bool{false, true} {
			p := processPolicy(t)
			i := processCreate(t, db, p)
			sqlDB, _ := db.DB()
			fault := db.Session(&gorm.Session{NewDB: true})
			fault.Statement = &gorm.Statement{DB: fault, ConnPool: faultPool{DB: sqlDB, committed: committed}}
			var candidate OperationClaim[outcome, processError]
			err := processRun(fault, p).Run(ctx, func(u processPorts) error {
				var err error
				candidate, _, err = u.operations.Claim(ctx, time.Minute, noProcessAlert)
				return err
			})
			if !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) {
				t.Fatal(err)
			}
			if committed {
				expire(t, i.ID)
			}
			recovered := claimOne(t, p)
			if recovered.Operation.ID != i.ID || candidate.LeaseID == recovered.LeaseID || recovered.Operation.Decision != Undecided {
				t.Fatal(recovered)
			}
		}
	})
}
