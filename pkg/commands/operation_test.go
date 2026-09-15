package commands

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"justixauto/pkg/eventstore"
)

type processError struct {
	Code string `json:"code"`
}
type processPorts struct {
	operations *Operations[input, outcome, processError]
	steps      *Steps[input, outcome]
	tx         *gorm.DB
}

func processPolicy(t *testing.T) OperationPolicy[input, outcome, processError] {
	s := schema(t)
	s.command = "inventory.fixture.process." + uuid.NewString()
	return OperationPolicy[input, outcome, processError]{Schema: s, Phases: map[string]bool{"pending": false, "finishing": false, "succeeded": true, "aborted": true}, Initial: func(o Operation[outcome, processError]) error {
		if o.Phase != "pending" {
			return ErrInvalidOperation
		}
		return nil
	}, Transition: func(old, next Operation[outcome, processError]) error {
		if old.Phase == "pending" && next.Phase == "finishing" && next.Decision != Undecided && next.Result == nil {
			return nil
		}
		if old.Phase == "finishing" && ((next.Phase == "succeeded" && next.Decision == Commit) || (next.Phase == "aborted" && next.Decision == Abort)) && next.Result != nil {
			return nil
		}
		return ErrOperationConflict
	}, ValidateError: func(e processError) error {
		if e.Code != "OWNER_REJECTED" {
			return ErrInvalidReceipt
		}
		return nil
	}}
}
func processRun(db *gorm.DB, p OperationPolicy[input, outcome, processError]) *eventstore.Transactions[processPorts] {
	r, _ := eventstore.NewTransactions(db, func(tx *gorm.DB) (processPorts, error) {
		o, err := NewOperations(tx, p)
		if err != nil {
			return processPorts{}, err
		}
		s, err := NewSteps(tx, p.Schema, p.Schema.command, "finish")
		return processPorts{o, s, tx}, err
	})
	return r
}
func processIntent() OperationIntent {
	return OperationIntent{ID: uuid.NewString(), AggregateID: uuid.NewString(), ActorID: uuid.NewString(), CompanyID: uuid.NewString(), AdmissionRef: uuid.NewString(), IntentRevision: rev(9), Phase: "pending"}
}
func processAuth(_ context.Context, a OperationAccess, _ OperationAction, o Operation[outcome, processError]) error {
	if a.ActorID != o.ActorID {
		return ErrOperationNotFound
	}
	return nil
}
func processStatus(ctx context.Context, p processPorts, i OperationIntent) (Operation[outcome, processError], error) {
	return p.operations.Status(ctx, i.ID, OperationAccess{i.ActorID, i.CompanyID}, processAuth)
}
func processCreate(t *testing.T, db *gorm.DB, p OperationPolicy[input, outcome, processError]) OperationIntent {
	t.Helper()
	i := processIntent()
	if err := processRun(db, p).Run(context.Background(), func(u processPorts) error { return u.operations.Create(context.Background(), i, request) }); err != nil {
		t.Fatal(err)
	}
	return i
}

func TestOperationConfiguration(t *testing.T) {
	if _, err := NewOperations[input, outcome, processError](nil, OperationPolicy[input, outcome, processError]{}); !errors.Is(err, eventstore.ErrTransactionRequired) {
		t.Fatal(err)
	}
	if _, err := NewSteps[input, outcome](nil, schema(t), "inventory.fixture", "finish"); !errors.Is(err, eventstore.ErrTransactionRequired) {
		t.Fatal(err)
	}
	var s *Operations[input, outcome, processError]
	if err := s.Create(context.Background(), processIntent(), request); !errors.Is(err, ErrInvalidOperation) {
		t.Fatal(err)
	}
	var st *Steps[input, outcome]
	if _, _, err := st.Execute(context.Background(), uuid.NewString(), request, func(context.Context) (outcome, error) { return outcome{}, nil }); !errors.Is(err, ErrInvalidOperation) {
		t.Fatal(err)
	}
}

func TestOperationsPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_OPERATIONS") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_OPERATIONS=1 for disposable PostgreSQL fixture")
	}
	db := fixtureDB(t)
	ctx := context.Background()
	t.Run("cancel and panic leave no step or effect", func(t *testing.T) {
		for _, panicCase := range []bool{false, true} {
			p := processPolicy(t)
			i := processCreate(t, db, p)
			id := uuid.NewString()
			cancelCtx, cancel := context.WithCancel(ctx)
			execute := func() (err error) {
				defer func() {
					if r := recover(); r != nil {
						if !panicCase {
							panic(r)
						}
						err = errors.New("observed synthetic panic")
					}
				}()
				return processRun(db, p).Run(cancelCtx, func(u processPorts) error {
					_, _, err := u.steps.Execute(cancelCtx, i.ID, request, func(context.Context) (outcome, error) {
						if err := u.tx.Exec(`INSERT INTO eventstore.fixture_effects(id) VALUES (?)`, id).Error; err != nil {
							return outcome{}, err
						}
						if panicCase {
							panic("synthetic crash")
						}
						cancel()
						return outcome{ID: id, State: "created"}, nil
					})
					return err
				})
			}
			if err := execute(); err == nil {
				t.Fatal("failure reported success")
			}
			cancel()
			var n int64
			if err := db.Raw(`SELECT count(*) FROM eventstore.operation_steps WHERE operation_id=?`, i.ID).Scan(&n).Error; err != nil || n != 0 {
				t.Fatal("step survived", n, err)
			}
			if err := db.Raw(`SELECT count(*) FROM eventstore.fixture_effects WHERE id=?`, id).Scan(&n).Error; err != nil || n != 0 {
				t.Fatal("effect survived", n, err)
			}
		}
	})
	t.Run("creation rollback and authoritative scope", func(t *testing.T) {
		p := processPolicy(t)
		i := processIntent()
		injected := errors.New("synthetic rollback")
		err := processRun(db, p).Run(ctx, func(u processPorts) error {
			if err := u.operations.Create(ctx, i, request); err != nil {
				return err
			}
			return injected
		})
		if !errors.Is(err, injected) {
			t.Fatal(err)
		}
		err = processRun(db, p).Run(ctx, func(u processPorts) error { _, err := processStatus(ctx, u, i); return err })
		if !errors.Is(err, ErrOperationNotFound) {
			t.Fatal(err)
		}
		i = processCreate(t, db, p)
		for _, access := range []OperationAccess{{uuid.NewString(), i.CompanyID}, {i.ActorID, uuid.NewString()}, {i.ActorID, ""}, {"", i.CompanyID}} {
			err = processRun(db, p).Run(ctx, func(u processPorts) error { _, err := u.operations.Status(ctx, i.ID, access, processAuth); return err })
			if !errors.Is(err, ErrOperationNotFound) {
				t.Fatal("scope escaped", err)
			}
		}
		err = processRun(db, p).Run(ctx, func(u processPorts) error {
			o, err := processStatus(ctx, u, i)
			if err == nil && (o.Revision != rev(1) || o.IntentRevision != rev(9) || o.Decision != Undecided) {
				t.Fatal(o)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		denied := false
		err = processRun(db, p).Run(ctx, func(u processPorts) error {
			return u.operations.Resume(ctx, i.ID, OperationAccess{i.ActorID, uuid.NewString()}, func(context.Context, OperationAccess, OperationAction, Operation[outcome, processError]) error {
				denied = true
				return nil
			})
		})
		if !errors.Is(err, ErrOperationNotFound) || denied {
			t.Fatal("foreign row reached authorization", err)
		}
	})
	t.Run("mutually exclusive concurrent decisions and immutable terminal", func(t *testing.T) {
		p := processPolicy(t)
		i := processCreate(t, db, p)
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for _, decision := range []Decision{Commit, Abort} {
			wg.Add(1)
			go func(d Decision) {
				defer wg.Done()
				results <- processRun(db, p).Run(ctx, func(u processPorts) error {
					return u.operations.Advance(ctx, i.ID, rev(1), OperationChange[outcome, processError]{Phase: "finishing", Decision: d})
				})
			}(decision)
		}
		wg.Wait()
		close(results)
		wins, conflicts := 0, 0
		for err := range results {
			if err == nil {
				wins++
			} else if errors.Is(err, ErrOperationConflict) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
		if wins != 1 || conflicts != 1 {
			t.Fatal(wins, conflicts)
		}
		var chosen Decision
		err := processRun(db, p).Run(ctx, func(u processPorts) error {
			o, err := processStatus(ctx, u, i)
			if err != nil {
				return err
			}
			chosen = o.Decision
			opposite := Commit
			if chosen == Commit {
				opposite = Abort
			}
			return u.operations.Advance(ctx, i.ID, o.Revision, OperationChange[outcome, processError]{Phase: "finishing", Decision: opposite})
		})
		if !errors.Is(err, ErrOperationConflict) {
			t.Fatal(err)
		}
		result := outcome{ID: uuid.NewString(), State: "created"}
		err = processRun(db, p).Run(ctx, func(u processPorts) error {
			o, err := processStatus(ctx, u, i)
			if err != nil {
				return err
			}
			phase := "succeeded"
			if chosen == Abort {
				phase = "aborted"
			}
			return u.operations.Advance(ctx, i.ID, o.Revision, OperationChange[outcome, processError]{Phase: phase, Decision: chosen, Result: &result})
		})
		if err != nil {
			t.Fatal(err)
		}
		err = processRun(db, p).Run(ctx, func(u processPorts) error {
			return u.operations.Resume(ctx, i.ID, OperationAccess{i.ActorID, i.CompanyID}, processAuth)
		})
		if !errors.Is(err, ErrOperationConflict) {
			t.Fatal(err)
		}
		if err := db.Exec(`UPDATE eventstore.operations SET decision='undecided' WHERE operation_id=?`, i.ID).Error; err == nil {
			t.Fatal("SQL reversed decision")
		}
	})
	t.Run("same step identity concurrent replay changed input and atomic rollback", func(t *testing.T) {
		p := processPolicy(t)
		i := processCreate(t, db, p)
		r := outcome{ID: uuid.NewString(), State: "created"}
		var calls atomic.Int64
		var wg sync.WaitGroup
		results := make(chan error, 8)
		execute := func(u processPorts, in input) error {
			_, _, err := u.steps.Execute(ctx, i.ID, in, func(context.Context) (outcome, error) {
				calls.Add(1)
				return r, u.tx.Exec(`INSERT INTO eventstore.fixture_effects(id) VALUES (?)`, r.ID).Error
			})
			return err
		}
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- processRun(db, p).Run(ctx, func(u processPorts) error { return execute(u, request) })
			}()
		}
		wg.Wait()
		close(results)
		for err := range results {
			if err != nil {
				t.Fatal(err)
			}
		}
		if calls.Load() != 1 {
			t.Fatal(calls.Load())
		}
		changed := request
		changed.Amount = "2"
		err := processRun(db, p).Run(ctx, func(u processPorts) error { return execute(u, changed) })
		if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
		i = processCreate(t, db, p)
		r.ID = uuid.NewString()
		injected := errors.New("rollback")
		err = processRun(db, p).Run(ctx, func(u processPorts) error {
			if err := execute(u, request); err != nil {
				return err
			}
			return injected
		})
		if !errors.Is(err, injected) {
			t.Fatal(err)
		}
		var n int64
		db.Raw(`SELECT count(*) FROM eventstore.operation_steps WHERE operation_id=?`, i.ID).Scan(&n)
		if n != 0 {
			t.Fatal("step survived rollback")
		}
		err = processRun(db, p).Run(ctx, func(u processPorts) error { return execute(u, request) })
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`UPDATE eventstore.operation_steps SET result='{}' WHERE operation_id=?`, i.ID).Error; err == nil {
			t.Fatal("receipt mutable")
		}
	})
	t.Run("unknown step commit converges through retained identity", func(t *testing.T) {
		for _, committed := range []bool{false, true} {
			p := processPolicy(t)
			i := processCreate(t, db, p)
			r := outcome{ID: uuid.NewString(), State: "created"}
			calls := 0
			sqlDB, _ := db.DB()
			fault := db.Session(&gorm.Session{NewDB: true})
			fault.Statement = &gorm.Statement{DB: fault, ConnPool: faultPool{DB: sqlDB, committed: committed}}
			execute := func(u processPorts) error {
				_, _, err := u.steps.Execute(ctx, i.ID, request, func(context.Context) (outcome, error) {
					calls++
					return r, u.tx.Exec(`INSERT INTO eventstore.fixture_effects(id) VALUES (?)`, r.ID).Error
				})
				return err
			}
			err := processRun(fault, p).Run(ctx, execute)
			if !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) || calls != 1 {
				t.Fatal(err, calls)
			}
			err = processRun(db, p).Run(ctx, execute)
			if err != nil {
				t.Fatal(err)
			}
			want := 2
			if committed {
				want = 1
			}
			if calls != want {
				t.Fatal("lost reply repeated committed effect", calls, want)
			}
		}
	})
	t.Run("invalid stored and newly produced outcomes fail closed", func(t *testing.T) {
		p := processPolicy(t)
		i := processCreate(t, db, p)
		err := processRun(db, p).Run(ctx, func(u processPorts) error {
			_, _, err := u.steps.Execute(ctx, i.ID, request, func(context.Context) (outcome, error) { return outcome{ID: uuid.NewString(), State: "invented"}, nil })
			return err
		})
		if !errors.Is(err, ErrInvalidReceipt) {
			t.Fatal(err)
		}
		if err = db.Exec(`INSERT INTO eventstore.operation_steps(operation_id,step,request_hash,result) VALUES (?,'finish',?,'{"password":"forbidden"}')`, i.ID, make([]byte, 32)).Error; err != nil {
			t.Fatal(err)
		}
		// Correct digest with malformed safe DTO, simulating migration/corruption.
		digest, _ := p.Schema.digest(request)
		j := processCreate(t, db, p)
		if err = db.Exec(`INSERT INTO eventstore.operation_steps(operation_id,step,request_hash,result) VALUES (?,'finish',?,'{}')`, j.ID, digest[:]).Error; err != nil {
			t.Fatal(err)
		}
		err = processRun(db, p).Run(ctx, func(u processPorts) error {
			_, _, err := u.steps.Execute(ctx, j.ID, request, func(context.Context) (outcome, error) { t.Error("corrupt receipt ran effect"); return outcome{}, nil })
			return err
		})
		if !errors.Is(err, ErrCorruptReceipt) {
			t.Fatal(err)
		}
	})
}
