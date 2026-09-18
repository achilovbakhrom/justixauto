package inbox_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
	"justixauto/pkg/inbox"
)

type qa015CommitFaultPool struct {
	*sql.DB
	count       atomic.Int32
	faultAt     atomic.Int32
	commitFirst atomic.Bool
}

func (p *qa015CommitFaultPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	if at := p.faultAt.Load(); at > 0 && p.count.Add(1) == at {
		return &commitFaultTx{Tx: tx, commitFirst: p.commitFirst.Load()}, nil
	}
	return tx, nil
}

func (p *qa015CommitFaultPool) arm(at int32, commitFirst bool) {
	p.count.Store(0)
	p.commitFirst.Store(commitFirst)
	p.faultAt.Store(at)
}

// TestQA015Fix1RestartRequiresDurableRequestReplay checks both halves of the
// instance seal: an old in-memory ticket cannot cross a reconstructed adapter,
// while replaying the exact durable request returns a locally bound ticket whose
// result is itself replayable without invoking the ordinary path twice.
func TestQA015Fix1RestartRequiresDurableRequestReplay(t *testing.T) {
	db, _ := installQuarantineAdapterSchema(t)
	ctx := context.Background()
	security := newFixtureSecurity()
	authorizer := &fixtureAuthorizer{}
	q, err := inbox.NewQuarantine(db, events.OwnerInventory, security)
	if err != nil {
		t.Fatal(err)
	}
	capture := captureInput(inbox.QuarantineDirectHandler)
	capture.ConsumerName = "qa-restart-replay"
	if _, err := q.Capture(ctx, capture, []byte("synthetic-restart-body")); err != nil {
		t.Fatal(err)
	}
	request := redriveRequest(capture)
	beforeRestart, err := inbox.NewRedriver(db, events.OwnerInventory, security, authorizer)
	if err != nil {
		t.Fatal(err)
	}
	oldTicket, err := beforeRestart.Request(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	afterRestart, err := inbox.NewRedriver(db, events.OwnerInventory, security, authorizer)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	pathErr := errors.New("synthetic ordinary failure")
	path := inbox.OrdinaryRedriveFunc(func(context.Context, inbox.RedrivePayload) (inbox.OrdinaryReceipt, error) {
		calls.Add(1)
		return inbox.OrdinaryReceipt{}, pathErr
	})
	if _, err := afterRestart.Execute(ctx, oldTicket, resultInput(), path); !errors.Is(err, inbox.ErrRedriveConflict) || calls.Load() != 0 {
		t.Fatalf("stale ticket crossed restart boundary: %v calls=%d", err, calls.Load())
	}
	newTicket, err := afterRestart.Request(ctx, request)
	if err != nil {
		t.Fatal("exact durable request replay", err)
	}
	result := resultInput()
	out, err := afterRestart.Execute(ctx, newTicket, result, path)
	if out.Code != "failed" || !errors.Is(err, inbox.ErrRedriveFailed) || !errors.Is(err, pathErr) || calls.Load() != 1 {
		t.Fatalf("replayed ticket did not use ordinary path exactly once: %+v %v calls=%d", out, err, calls.Load())
	}
	out, err = afterRestart.Execute(ctx, newTicket, result, path)
	if err != nil || out.Code != "failed" || !out.Duplicate || calls.Load() != 1 {
		t.Fatalf("retained failure result was not replayed: %+v %v calls=%d", out, err, calls.Load())
	}
}

// TestQA015Fix1ResultUnknownCommitReconciles ensures the new validation
// transaction does not weaken unknown-outcome recovery at the later result
// journal commit. It faults the second transaction begun by Execute: validation
// completes, then the redrive-result commit reply is lost after a real server
// commit or rollback.
func TestQA015Fix1ResultUnknownCommitReconciles(t *testing.T) {
	db, _ := installQuarantineAdapterSchema(t)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, commitFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit"}[commitFirst], func(t *testing.T) {
			faults := &qa015CommitFaultPool{DB: pool}
			faultDB, err := gorm.Open(postgres.New(postgres.Config{Conn: faults}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			security := newFixtureSecurity()
			authorizer := &fixtureAuthorizer{}
			q, err := inbox.NewQuarantine(db, events.OwnerInventory, security)
			if err != nil {
				t.Fatal(err)
			}
			capture := captureInput(inbox.QuarantineDirectHandler)
			capture.ConsumerName = "qa-result-unknown-" + map[bool]string{false: "rollback", true: "commit"}[commitFirst]
			if _, err := q.Capture(ctx, capture, []byte("synthetic-result-unknown")); err != nil {
				t.Fatal(err)
			}
			r, err := inbox.NewRedriver(faultDB, events.OwnerInventory, security, authorizer)
			if err != nil {
				t.Fatal(err)
			}
			ticket, err := r.Request(ctx, redriveRequest(capture))
			if err != nil {
				t.Fatal(err)
			}
			faults.arm(2, commitFirst)
			var calls atomic.Int32
			pathErr := errors.New("synthetic ordinary failure")
			path := inbox.OrdinaryRedriveFunc(func(context.Context, inbox.RedrivePayload) (inbox.OrdinaryReceipt, error) {
				calls.Add(1)
				return inbox.OrdinaryReceipt{}, pathErr
			})
			result := resultInput()
			out, err := r.Execute(ctx, ticket, result, path)
			if out.Code != "failed" || !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) || !errors.Is(err, io.ErrUnexpectedEOF) || !errors.Is(err, pathErr) || calls.Load() != 1 {
				t.Fatalf("unknown result commit was not surfaced: %+v %v calls=%d", out, err, calls.Load())
			}
			faults.arm(0, false)
			out, err = r.Execute(ctx, ticket, result, path)
			if commitFirst {
				if err != nil || out.Code != "failed" || !out.Duplicate || calls.Load() != 1 {
					t.Fatalf("committed result did not reconcile: %+v %v calls=%d", out, err, calls.Load())
				}
				return
			}
			if out.Code != "failed" || !errors.Is(err, inbox.ErrRedriveFailed) || !errors.Is(err, pathErr) || out.Duplicate || calls.Load() != 2 {
				t.Fatalf("rolled-back result did not retry: %+v %v calls=%d", out, err, calls.Load())
			}
			out, err = r.Execute(ctx, ticket, result, path)
			if err != nil || out.Code != "failed" || !out.Duplicate || calls.Load() != 2 {
				t.Fatalf("retried result was not retained: %+v %v calls=%d", out, err, calls.Load())
			}
		})
	}
}
