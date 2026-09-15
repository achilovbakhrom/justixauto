package outbox

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
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

const sourceSchema = "inventory.fixture.changed.v1"

func sourceRevision(n int64) events.Revision {
	r, err := events.NewRevision(n)
	if err != nil {
		panic(err)
	}
	return r
}
func sourceStream(t *testing.T) SourceStream {
	t.Helper()
	s, err := NewSourceStream(events.OwnerInventory, "fixture", uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func sourceInput(s SourceStream, d events.Owner, h int64) SourceAdmissionInput {
	return SourceAdmissionInput{ID: uuid.NewString(), Stream: s, Destination: d, Contract: AdmissionContract{"synthetic-contract", 1, sha256.Sum256([]byte("fixture"))}, Schemas: []string{sourceSchema},
		AuthorityRef: "synthetic-authority", ScopeRef: "synthetic-concrete-scope", Purpose: "synthetic-checkpoint",
		Bootstrap: BootstrapEvidence{sourceRevision(h), "synthetic-bootstrap", "synthetic-new-empty-stream"}}
}
func sourceAdmit(t *testing.T, in SourceAdmissionInput) SourceAdmission {
	t.Helper()
	a, err := NewSourceAdmission(in)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func sourceRule(t *testing.T, s SourceStream, empty bool) SourcePlanRule {
	t.Helper()
	ref := ""
	if empty {
		ref = "explicit-synthetic-empty-rule"
	}
	r, err := NewSourcePlanRule(s, "synthetic-stream-rule", []string{sourceSchema}, ref)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func sourceEvent(t *testing.T, s SourceStream, rev, seq int64) eventstore.Event {
	t.Helper()
	type data struct {
		Ref string `json:"ref"`
	}
	schema, err := events.NewEventSchema(sourceSchema, 1, "fixture", events.CompanyScopeTenantRequired, func(d data) error {
		if d.Ref == "" {
			return errors.New("empty")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	company := uuid.NewString()
	in := events.EnvelopeInput{EventID: uuid.NewString(), AggregateID: s.aggregateID, AggregateVersion: sourceRevision(rev), IntegrationSequence: sourceRevision(seq), CompanyID: &company, OccurredAt: time.Now().UTC(), Actor: events.Actor{Kind: events.ActorUser, ID: uuid.NewString()}, CorrelationID: uuid.NewString(), CausationID: uuid.NewString(), OperationID: uuid.NewString()}
	var e eventstore.Event
	if seq == 0 {
		e, err = eventstore.NewInternalEvent(in, schema, data{"synthetic"})
	} else {
		e, err = eventstore.NewEvent(in, schema, data{"synthetic"})
	}
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func sourceEnvelope(e eventstore.Event) events.Envelope { v, _ := e.IntegrationEnvelope(); return v }

func TestSourceAdmissionValues(t *testing.T) {
	s := sourceStream(t)
	in := sourceInput(s, events.OwnerRetail, 0)
	for _, mutate := range []func(*SourceAdmissionInput){
		func(i *SourceAdmissionInput) { i.ID = uuid.Nil.String() }, func(i *SourceAdmissionInput) { i.Stream = SourceStream{} },
		func(i *SourceAdmissionInput) { i.Destination = "*" }, func(i *SourceAdmissionInput) { i.Contract.Digest = [32]byte{} },
		func(i *SourceAdmissionInput) { i.Contract.Version = 0 }, func(i *SourceAdmissionInput) { i.AuthorityRef = " " },
		func(i *SourceAdmissionInput) { i.ScopeRef = "" }, func(i *SourceAdmissionInput) { i.Purpose = "" },
		func(i *SourceAdmissionInput) { i.Bootstrap.Ref = "" }, func(i *SourceAdmissionInput) { i.Bootstrap.EmptyStreamRef = "" },
		func(i *SourceAdmissionInput) { i.Schemas = nil }, func(i *SourceAdmissionInput) { i.Schemas = []string{sourceSchema, sourceSchema} },
		func(i *SourceAdmissionInput) { i.Schemas = []string{"retail.fixture.changed.v1"} }, func(i *SourceAdmissionInput) { i.Schemas = []string{"inventory.fixture.v4294967296"} },
	} {
		candidate := in
		mutate(&candidate)
		if _, err := NewSourceAdmission(candidate); !errors.Is(err, ErrInvalidAdmission) {
			t.Fatal(err)
		}
	}
	a := sourceAdmit(t, in)
	in.Schemas[0] = "inventory.changed.v2"
	if a.input.Schemas[0] != sourceSchema {
		t.Fatal("mutable admission schemas")
	}
	if _, err := NewSourcePlanRule(s, "", []string{sourceSchema}, ""); err == nil {
		t.Fatal("missing emission rule")
	}
	if _, err := NewSourceStream(events.OwnerInventory, "fixture", strings.ToUpper(s.aggregateID)); err == nil {
		t.Fatal("noncanonical lock key")
	}
	if _, err := NewSourceAdmissions(nil, events.OwnerInventory); !errors.Is(err, eventstore.ErrTransactionRequired) {
		t.Fatal(err)
	}
	var repo *SourceAdmissions
	if err := repo.Fence(context.Background(), s); !errors.Is(err, eventstore.ErrTransactionRequired) {
		t.Fatal(err)
	}
	if _, err := repo.Resolve(context.Background(), SourcePlanRule{}, events.Envelope{}); !errors.Is(err, eventstore.ErrTransactionRequired) {
		t.Fatal(err)
	}
}

func sourceRow(s SourceStream) sourceAdmissionRow {
	in := sourceInput(s, events.OwnerRetail, 1)
	schemas, _ := json.Marshal(in.Schemas)
	return sourceAdmissionRow{AdmissionID: in.ID, Subject: string(in.Destination), Action: "admit", ContractID: in.Contract.ID, ContractVersion: 1, ContractDigest: in.Contract.Digest[:], SchemaAllowlist: schemas, AuthorityRef: in.AuthorityRef, ScopeRef: in.ScopeRef, Purpose: in.Purpose, BootstrapRef: in.Bootstrap.Ref, StartAfter: 1}
}
func nextSourceRow(prior sourceAdmissionRow, action string) sourceAdmissionRow {
	n := prior
	n.AdmissionID = uuid.NewString()
	n.PriorAdmissionID = sql.NullString{String: prior.AdmissionID, Valid: true}
	n.Action = action
	return n
}
func TestSourceAdmissionReplay(t *testing.T) {
	s := sourceStream(t)
	root := sourceRow(s)
	closeRow := nextSourceRow(root, "close")
	closeRow.EndInclusive = sql.NullInt64{Int64: 3, Valid: true}
	hold := nextSourceRow(closeRow, "hold")
	resume := nextSourceRow(hold, "resume")
	rows := []sourceAdmissionRow{resume, root, hold, closeRow}
	states, err := resolveAdmissionRows(s, rows)
	if err != nil || len(states) != 1 || states[0].end.Int64 != 3 || states[0].held != "" {
		t.Fatal(states, err)
	}
	for _, tc := range []struct {
		name string
		rows []sourceAdmissionRow
	}{
		{"orphan", []sourceAdmissionRow{hold}},
		{"branch", []sourceAdmissionRow{root, closeRow, nextSourceRow(root, "hold")}},
		{"resume without hold", []sourceAdmissionRow{root, nextSourceRow(root, "resume")}},
		{"double hold", []sourceAdmissionRow{root, nextSourceRow(root, "hold"), nextSourceRow(nextSourceRow(root, "hold"), "hold")}},
		{"overlap", []sourceAdmissionRow{root, sourceRow(s)}},
		{"unknown action", []sourceAdmissionRow{root, nextSourceRow(root, "delete")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := resolveAdmissionRows(s, tc.rows); !errors.Is(err, ErrAdmissionConflict) {
				t.Fatal(err)
			}
		})
	}
	for _, mutate := range []func(*sourceAdmissionRow){func(r *sourceAdmissionRow) { r.ContractID = "other" }, func(r *sourceAdmissionRow) { r.ScopeRef = "other" }, func(r *sourceAdmissionRow) { r.Purpose = "other" }, func(r *sourceAdmissionRow) { r.StartAfter = 2 }, func(r *sourceAdmissionRow) { r.SchemaAllowlist = []byte(`["inventory.other.v1"]`) }} {
		changed := closeRow
		mutate(&changed)
		if _, err := resolveAdmissionRows(s, []sourceAdmissionRow{root, changed}); !errors.Is(err, ErrAdmissionConflict) {
			t.Fatal("metadata drift", err)
		}
	}
	newRoot := sourceRow(s)
	newRoot.StartAfter = 3
	if _, err := resolveAdmissionRows(s, []sourceAdmissionRow{root, closeRow, newRoot}); err != nil {
		t.Fatal("adjacent admitted ranges overlap", err)
	}
	newRoot.StartAfter = 2
	if _, err := resolveAdmissionRows(s, []sourceAdmissionRow{root, closeRow, newRoot}); !errors.Is(err, ErrAdmissionConflict) {
		t.Fatal(err)
	}
}

type sourcePorts struct {
	admissions *SourceAdmissions
	append     *eventstore.Appender
	tx         *gorm.DB
}

func sourceRunner(t *testing.T, db *gorm.DB) *eventstore.Transactions[sourcePorts] {
	t.Helper()
	r, err := eventstore.NewTransactions(db, func(tx *gorm.DB) (sourcePorts, error) {
		a, err := NewSourceAdmissions(tx, events.OwnerInventory)
		if err != nil {
			return sourcePorts{}, err
		}
		append, err := eventstore.NewAppender(tx, events.OwnerInventory)
		return sourcePorts{a, append, tx}, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSourceAdmissionPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_SOURCE_ADMISSION") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_SOURCE_ADMISSION=1 for pinned isolated PostgreSQL")
	}
	db := sourceDatabase(t)
	runner := sourceRunner(t, db)
	ctx := context.Background()
	run := func(s SourceStream, fn func(sourcePorts) error) error {
		return runner.Run(ctx, func(p sourcePorts) error {
			if err := p.admissions.Fence(ctx, s); err != nil {
				return err
			}
			return fn(p)
		})
	}
	count := func(table string, s SourceStream) int64 {
		t.Helper()
		var n int64
		if err := db.Raw("SELECT count(*) FROM eventstore."+table+" WHERE aggregate_id=?::uuid", s.aggregateID).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	appendOne := func(p sourcePorts, e eventstore.Event, expected int64) error {
		return p.append.Append(ctx, eventstore.Batch{Expected: sourceRevision(expected), Events: []eventstore.Event{e}})
	}
	t.Run("complete fence and local event identity are mandatory", func(t *testing.T) {
		s := sourceStream(t)
		other := sourceStream(t)
		if err := runner.Run(ctx, func(p sourcePorts) error {
			return p.admissions.Admit(ctx, sourceAdmit(t, sourceInput(s, events.OwnerRetail, 0)))
		}); !errors.Is(err, ErrUnfencedStream) {
			t.Fatal(err)
		}
		if err := run(s, func(p sourcePorts) error { return p.admissions.Fence(ctx, other) }); !errors.Is(err, ErrUnfencedStream) {
			t.Fatal(err)
		}
		if err := run(s, func(p sourcePorts) error {
			_, err := p.admissions.Resolve(ctx, sourceRule(t, s, true), sourceEnvelope(sourceEvent(t, s, 1, 1)))
			return err
		}); !errors.Is(err, ErrAdmissionCheckpoint) {
			t.Fatal("unappended envelope", err)
		}
		if err := run(s, func(p sourcePorts) error {
			e := sourceEvent(t, s, 1, 1)
			if err := appendOne(p, e, 0); err != nil {
				return err
			}
			_, err := p.admissions.Resolve(ctx, sourceRule(t, other, true), sourceEnvelope(e))
			return err
		}); !errors.Is(err, ErrUnfencedStream) {
			t.Fatal(err)
		}
		if count("events", s) != 0 {
			t.Fatal("unfenced command persisted")
		}
		if err := run(s, func(p sourcePorts) error {
			if err := p.tx.Exec("SAVEPOINT synthetic_append").Error; err != nil {
				return err
			}
			e := sourceEvent(t, s, 1, 1)
			if err := appendOne(p, e, 0); err != nil {
				return err
			}
			plan, err := p.admissions.Resolve(ctx, sourceRule(t, s, true), sourceEnvelope(e))
			if err != nil {
				return err
			}
			if err := p.tx.Exec("ROLLBACK TO SAVEPOINT synthetic_append").Error; err != nil {
				return err
			}
			return p.admissions.ValidatePlan(ctx, plan)
		}); !errors.Is(err, ErrAdmissionCheckpoint) {
			t.Fatal("plan survived event rollback", err)
		}
	})
	t.Run("missing versus explicit empty and transaction ownership", func(t *testing.T) {
		s := sourceStream(t)
		e := sourceEvent(t, s, 1, 1)
		var candidate DeliveryPlan
		err := run(s, func(p sourcePorts) error {
			if err := appendOne(p, e, 0); err != nil {
				return err
			}
			_, err := p.admissions.Resolve(ctx, sourceRule(t, s, false), sourceEnvelope(e))
			return err
		})
		if !errors.Is(err, ErrMissingRecipientPlan) || count("events", s) != 0 {
			t.Fatal(err)
		}
		if err := run(s, func(p sourcePorts) error {
			if err := appendOne(p, e, 0); err != nil {
				return err
			}
			var err error
			candidate, err = p.admissions.Resolve(ctx, sourceRule(t, s, true), sourceEnvelope(e))
			if err != nil {
				return err
			}
			if len(candidate.Recipients()) != 0 || candidate.AuthorityRef() != "explicit-synthetic-empty-rule" {
				t.Fatal("invalid empty plan")
			}
			return p.admissions.ValidatePlan(ctx, candidate)
		}); err != nil {
			t.Fatal(err)
		}
		if err := run(s, func(p sourcePorts) error { return p.admissions.ValidatePlan(ctx, candidate) }); !errors.Is(err, ErrInvalidAdmission) {
			t.Fatal("escaped transaction", err)
		}
		if err := run(s, func(p sourcePorts) error {
			_, err := p.admissions.Resolve(ctx, sourceRule(t, s, true), sourceEnvelope(e))
			return err
		}); !errors.Is(err, ErrAdmissionCheckpoint) {
			t.Fatal("replanned committed history", err)
		}
	})
	t.Run("complete distinct copied plan and rollback", func(t *testing.T) {
		s := sourceStream(t)
		retail := sourceAdmit(t, sourceInput(s, events.OwnerRetail, 0))
		finance := sourceAdmit(t, sourceInput(s, events.OwnerFinancing, 0))
		e1 := sourceEvent(t, s, 1, 1)
		e2 := sourceEvent(t, s, 2, 2)
		write := func(p sourcePorts) error {
			if err := p.admissions.Admit(ctx, retail); err != nil {
				return err
			}
			if err := p.admissions.Admit(ctx, finance); err != nil {
				return err
			}
			if err := p.append.Append(ctx, eventstore.Batch{Events: []eventstore.Event{e1, e2}}); err != nil {
				return err
			}
			for _, e := range []eventstore.Event{e1, e2} {
				plan, err := p.admissions.Resolve(ctx, sourceRule(t, s, false), sourceEnvelope(e))
				if err != nil {
					return err
				}
				got := plan.Recipients()
				if len(got) != 2 || got[0].Destination != events.OwnerFinancing || got[1].Destination != events.OwnerRetail {
					t.Fatal(got)
				}
				got[0].AdmissionID = "changed"
				if plan.Recipients()[0].AdmissionID == "changed" {
					t.Fatal("mutable plan")
				}
				if err := p.admissions.ValidatePlan(ctx, plan); err != nil {
					return err
				}
			}
			return nil
		}
		abort := errors.New("abort fixture")
		if err := run(s, func(p sourcePorts) error {
			if err := write(p); err != nil {
				return err
			}
			return abort
		}); !errors.Is(err, abort) {
			t.Fatal(err)
		}
		if count("events", s) != 0 || count("messaging_admissions", s) != 0 {
			t.Fatal("partial rollback")
		}
		if err := run(s, write); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("new recipient requires source high-water and bootstrap", func(t *testing.T) {
		s := sourceStream(t)
		if err := run(s, func(p sourcePorts) error { return appendOne(p, sourceEvent(t, s, 1, 1), 0) }); err != nil {
			t.Fatal(err)
		}
		if err := run(s, func(p sourcePorts) error {
			return p.admissions.Admit(ctx, sourceAdmit(t, sourceInput(s, events.OwnerRetail, 0)))
		}); !errors.Is(err, ErrAdmissionCheckpoint) {
			t.Fatal(err)
		}
		if err := run(s, func(p sourcePorts) error {
			return p.admissions.Admit(ctx, sourceAdmit(t, sourceInput(s, events.OwnerRetail, 2)))
		}); !errors.Is(err, ErrAdmissionCheckpoint) {
			t.Fatal(err)
		}
		if err := run(s, func(p sourcePorts) error {
			if err := p.admissions.Admit(ctx, sourceAdmit(t, sourceInput(s, events.OwnerRetail, 1))); err != nil {
				return err
			}
			e := sourceEvent(t, s, 2, 2)
			if err := appendOne(p, e, 1); err != nil {
				return err
			}
			plan, err := p.admissions.Resolve(ctx, sourceRule(t, s, false), sourceEnvelope(e))
			if err == nil && (len(plan.Recipients()) != 1 || plan.Sequence().Int64() != 2) {
				t.Fatal(plan)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		internal := sourceStream(t)
		if err := run(internal, func(p sourcePorts) error { return appendOne(p, sourceEvent(t, internal, 1, 0), 0) }); err != nil {
			t.Fatal(err)
		}
		if err := run(internal, func(p sourcePorts) error {
			return p.admissions.Admit(ctx, sourceAdmit(t, sourceInput(internal, events.OwnerRetail, 0)))
		}); !errors.Is(err, ErrAdmissionCheckpoint) {
			t.Fatal("internal history is not a new empty stream", err)
		}
	})
	t.Run("schema mismatch rolls back every event", func(t *testing.T) {
		s := sourceStream(t)
		in := sourceInput(s, events.OwnerRetail, 0)
		in.Schemas = []string{"inventory.other.v1"}
		if err := run(s, func(p sourcePorts) error {
			if err := p.admissions.Admit(ctx, sourceAdmit(t, in)); err != nil {
				return err
			}
			e := sourceEvent(t, s, 1, 1)
			if err := appendOne(p, e, 0); err != nil {
				return err
			}
			_, err := p.admissions.Resolve(ctx, sourceRule(t, s, true), sourceEnvelope(e))
			return err
		}); !errors.Is(err, ErrAdmissionSchema) {
			t.Fatal(err)
		}
		if count("events", s) != 0 || count("messaging_admissions", s) != 0 {
			t.Fatal("schema failure persisted command")
		}
	})
	t.Run("concurrent admissions cannot overlap", func(t *testing.T) {
		s := sourceStream(t)
		inputs := []SourceAdmission{sourceAdmit(t, sourceInput(s, events.OwnerRetail, 0)), sourceAdmit(t, sourceInput(s, events.OwnerRetail, 0))}
		start := make(chan struct{})
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for _, in := range inputs {
			wg.Go(func() { <-start; errs <- run(s, func(p sourcePorts) error { return p.admissions.Admit(ctx, in) }) })
		}
		close(start)
		wg.Wait()
		close(errs)
		wins, conflicts := 0, 0
		for err := range errs {
			if err == nil {
				wins++
			} else if errors.Is(err, ErrAdmissionConflict) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
		if wins != 1 || conflicts != 1 || count("messaging_admissions", s) != 1 {
			t.Fatal(wins, conflicts)
		}
	})
	t.Run("append fence blocks stale zero admission", func(t *testing.T) {
		s := sourceStream(t)
		locked := make(chan struct{})
		release := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- runner.Run(ctx, func(p sourcePorts) error {
				if err := appendOne(p, sourceEvent(t, s, 1, 1), 0); err != nil {
					return err
				}
				close(locked)
				<-release
				return nil
			})
		}()
		<-locked
		started := make(chan struct{})
		admitted := make(chan error, 1)
		go func() {
			close(started)
			admitted <- run(s, func(p sourcePorts) error {
				return p.admissions.Admit(ctx, sourceAdmit(t, sourceInput(s, events.OwnerRetail, 0)))
			})
		}()
		<-started
		select {
		case err := <-admitted:
			close(release)
			t.Fatal("different fence", err)
		case <-time.After(150 * time.Millisecond):
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if err := <-admitted; !errors.Is(err, ErrAdmissionCheckpoint) {
			t.Fatal(err)
		}
	})
	t.Run("multi-stream fence uses append order", func(t *testing.T) {
		a, b := sourceStream(t), sourceStream(t)
		streams := []SourceStream{a, b}
		slices.SortFunc(streams, func(x, y SourceStream) int { return strings.Compare(x.key(), y.key()) })
		start := make(chan struct{})
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for _, order := range [][]SourceStream{streams, {streams[1], streams[0]}} {
			wg.Go(func() {
				<-start
				limited, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				errs <- runner.Run(limited, func(p sourcePorts) error {
					if err := p.admissions.Fence(limited, order...); err != nil {
						return err
					}
					return p.append.Append(limited, eventstore.Batch{Events: []eventstore.Event{sourceEvent(t, a, 1, 1)}}, eventstore.Batch{Events: []eventstore.Event{sourceEvent(t, b, 1, 1)}})
				})
			})
		}
		close(start)
		wg.Wait()
		close(errs)
		wins := 0
		for err := range errs {
			if err == nil {
				wins++
			} else if !errors.Is(err, eventstore.ErrVersionConflict) {
				t.Fatal(err)
			}
		}
		if wins != 1 {
			t.Fatal(wins)
		}
	})
	t.Run("drain hold resume and fresh re-admission", func(t *testing.T) {
		s := sourceStream(t)
		in := sourceInput(s, events.OwnerRetail, 0)
		e := sourceEvent(t, s, 1, 1)
		if err := run(s, func(p sourcePorts) error {
			if err := p.admissions.Admit(ctx, sourceAdmit(t, in)); err != nil {
				return err
			}
			if err := appendOne(p, e, 0); err != nil {
				return err
			}
			plan, err := p.admissions.Resolve(ctx, sourceRule(t, s, false), sourceEnvelope(e))
			if err != nil {
				return err
			}
			return sourceStageFixture(p.tx, plan, sourceEnvelope(e))
		}); err != nil {
			t.Fatal(err)
		}
		hold := SourceAdmissionChange{ID: uuid.NewString(), PriorID: in.ID, Action: AdmissionHold, Checkpoint: sourceRevision(1), AuthorityRef: "synthetic-hold-authority", EvidenceRef: "synthetic-inflight-disposition"}
		if err := run(s, func(p sourcePorts) error { return p.admissions.Change(ctx, s, hold) }); err != nil {
			t.Fatal(err)
		}
		var held string
		if err := db.Raw("SELECT hold_ref FROM eventstore.outbox_deliveries WHERE event_id=?::uuid", sourceEnvelope(e).EventID()).Scan(&held).Error; err != nil || held != "source-admission:"+hold.ID {
			t.Fatal(held, err)
		}
		if err := run(s, func(p sourcePorts) error {
			next := sourceEvent(t, s, 2, 2)
			if err := appendOne(p, next, 1); err != nil {
				return err
			}
			_, err := p.admissions.Resolve(ctx, sourceRule(t, s, true), sourceEnvelope(next))
			return err
		}); !errors.Is(err, ErrAdmissionHeld) {
			t.Fatal(err)
		}
		resume := SourceAdmissionChange{ID: uuid.NewString(), PriorID: hold.ID, Action: AdmissionResume, Checkpoint: sourceRevision(1), AuthorityRef: "synthetic-resume", EvidenceRef: "synthetic-reviewed-hold"}
		if err := run(s, func(p sourcePorts) error { return p.admissions.Change(ctx, s, resume) }); !errors.Is(err, ErrInvalidAdmission) {
			t.Fatal("implicit private history revival", err)
		}
		resume.RetainedWorkAuthorityRef = "synthetic-release-retained-work"
		if err := run(s, func(p sourcePorts) error { return p.admissions.Change(ctx, s, resume) }); err != nil {
			t.Fatal(err)
		}
		var unheld bool
		if err := db.Raw("SELECT hold_ref IS NULL FROM eventstore.outbox_deliveries WHERE event_id=?::uuid", sourceEnvelope(e).EventID()).Scan(&unheld).Error; err != nil || !unheld {
			t.Fatal(err)
		}
		closeChange := SourceAdmissionChange{ID: uuid.NewString(), PriorID: resume.ID, Action: AdmissionClose, Checkpoint: sourceRevision(1), AuthorityRef: "synthetic-drain", EvidenceRef: "synthetic-highwater"}
		if err := run(s, func(p sourcePorts) error { return p.admissions.Change(ctx, s, closeChange) }); err != nil {
			t.Fatal(err)
		}
		if err := run(s, func(p sourcePorts) error { return p.admissions.Change(ctx, s, hold) }); !errors.Is(err, ErrAdmissionConflict) {
			t.Fatal("branched stale evidence", err)
		}
		if err := run(s, func(p sourcePorts) error {
			next := sourceEvent(t, s, 2, 2)
			if err := appendOne(p, next, 1); err != nil {
				return err
			}
			_, err := p.admissions.Resolve(ctx, sourceRule(t, s, false), sourceEnvelope(next))
			return err
		}); !errors.Is(err, ErrMissingRecipientPlan) {
			t.Fatal("closed range emitted", err)
		}
		fresh := sourceInput(s, events.OwnerRetail, 1)
		if err := run(s, func(p sourcePorts) error {
			if err := p.admissions.Admit(ctx, sourceAdmit(t, fresh)); err != nil {
				return err
			}
			next := sourceEvent(t, s, 2, 2)
			if err := appendOne(p, next, 1); err != nil {
				return err
			}
			plan, err := p.admissions.Resolve(ctx, sourceRule(t, s, false), sourceEnvelope(next))
			if err == nil && plan.Recipients()[0].AdmissionID != fresh.ID {
				t.Fatal("old admission revived")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if count("outbox_messages", s) != 1 {
			t.Fatal("drain removed old obligations")
		}
	})
	t.Run("same transaction evidence changes invalidate candidate", func(t *testing.T) {
		s := sourceStream(t)
		in := sourceInput(s, events.OwnerRetail, 0)
		if err := run(s, func(p sourcePorts) error {
			if err := p.admissions.Admit(ctx, sourceAdmit(t, in)); err != nil {
				return err
			}
			e := sourceEvent(t, s, 1, 1)
			if err := appendOne(p, e, 0); err != nil {
				return err
			}
			plan, err := p.admissions.Resolve(ctx, sourceRule(t, s, false), sourceEnvelope(e))
			if err != nil {
				return err
			}
			if err := p.admissions.Change(ctx, s, SourceAdmissionChange{ID: uuid.NewString(), PriorID: in.ID, Action: AdmissionClose, Checkpoint: sourceRevision(1), AuthorityRef: "fixture", EvidenceRef: "fixture"}); err != nil {
				return err
			}
			return p.admissions.ValidatePlan(ctx, plan)
		}); !errors.Is(err, ErrAdmissionConflict) {
			t.Fatal(err)
		}
		if count("events", s) != 0 || count("messaging_admissions", s) != 0 {
			t.Fatal("invalidated candidate committed")
		}
	})
	t.Run("database conflict evidence fails closed", func(t *testing.T) {
		s := sourceStream(t)
		in := sourceInput(s, events.OwnerRetail, 0)
		if err := run(s, func(p sourcePorts) error { return p.admissions.Admit(ctx, sourceAdmit(t, in)) }); err != nil {
			t.Fatal(err)
		}
		// A faulty older runtime can INSERT structurally valid evidence directly.
		// Semantic replay must reject the complete state, not silently union it.
		if err := db.Exec(`INSERT INTO eventstore.messaging_admissions(admission_id,namespace,source_owner,aggregate_type,aggregate_id,subject,action,contract_id,contract_version,contract_digest,schema_allowlist,authority_ref,scope_ref,purpose,bootstrap_ref,start_after)
 SELECT ?::uuid,namespace,source_owner,aggregate_type,aggregate_id,subject,action,contract_id,contract_version,contract_digest,schema_allowlist,authority_ref,scope_ref,purpose,bootstrap_ref,start_after FROM eventstore.messaging_admissions WHERE admission_id=?::uuid`, uuid.NewString(), in.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := run(s, func(p sourcePorts) error {
			e := sourceEvent(t, s, 1, 1)
			if err := appendOne(p, e, 0); err != nil {
				return err
			}
			_, err := p.admissions.Resolve(ctx, sourceRule(t, s, true), sourceEnvelope(e))
			return err
		}); !errors.Is(err, ErrAdmissionConflict) {
			t.Fatal(err)
		}
		if count("events", s) != 0 {
			t.Fatal("conflicting state allowed emission")
		}
	})
	t.Run("panic and cancellation retain no partial admission", func(t *testing.T) {
		s := sourceStream(t)
		func() {
			defer func() {
				if recover() != "synthetic panic" {
					t.Error("panic not preserved")
				}
			}()
			_ = run(s, func(p sourcePorts) error {
				if err := p.admissions.Admit(ctx, sourceAdmit(t, sourceInput(s, events.OwnerRetail, 0))); err != nil {
					return err
				}
				panic("synthetic panic")
			})
		}()
		if count("messaging_admissions", s) != 0 {
			t.Fatal("panic committed admission")
		}
		cancelled, cancel := context.WithCancel(ctx)
		err := runner.Run(cancelled, func(p sourcePorts) error {
			if err := p.admissions.Fence(cancelled, s); err != nil {
				return err
			}
			if err := p.admissions.Admit(cancelled, sourceAdmit(t, sourceInput(s, events.OwnerRetail, 0))); err != nil {
				return err
			}
			cancel()
			return nil
		})
		if !errors.Is(err, context.Canceled) || count("messaging_admissions", s) != 0 {
			t.Fatal(err)
		}
	})
	t.Run("lost commit reply never retries or authorizes a plan receipt", func(t *testing.T) {
		pool, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		for _, commitFirst := range []bool{false, true} {
			s := sourceStream(t)
			faultDB, err := gorm.Open(postgres.New(postgres.Config{Conn: sourceFaultPool{pool, commitFirst}}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			err = sourceRunner(t, faultDB).Run(ctx, func(p sourcePorts) error {
				calls++
				if err := p.admissions.Fence(ctx, s); err != nil {
					return err
				}
				if err := p.admissions.Admit(ctx, sourceAdmit(t, sourceInput(s, events.OwnerRetail, 0))); err != nil {
					return err
				}
				e := sourceEvent(t, s, 1, 1)
				if err := appendOne(p, e, 0); err != nil {
					return err
				}
				plan, err := p.admissions.Resolve(ctx, sourceRule(t, s, false), sourceEnvelope(e))
				if err != nil {
					return err
				}
				return sourceStageFixture(p.tx, plan, sourceEnvelope(e))
			})
			if !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) || !errors.Is(err, io.ErrUnexpectedEOF) || calls != 1 {
				t.Fatal(commitFirst, calls, err)
			}
			want := int64(0)
			if commitFirst {
				want = 1
			}
			for _, table := range []string{"events", "messaging_admissions", "outbox_messages"} {
				if count(table, s) != want {
					t.Fatal("partial or incorrectly reported commit", table, commitFirst)
				}
			}
		}
	})
}

type sourceFaultPool struct {
	*sql.DB
	commitFirst bool
}

func (p sourceFaultPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &sourceFaultTx{tx, p.commitFirst}, nil
}

type sourceFaultTx struct {
	*sql.Tx
	commitFirst bool
}

func (t sourceFaultTx) Commit() error {
	var err error
	if t.commitFirst {
		err = t.Tx.Commit()
	} else {
		err = t.Tx.Rollback()
	}
	if err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}

// T-919 owns production parent/child writing. This helper only creates a
// synthetic retained obligation to exercise hold/drain behavior under real SQL.
func sourceStageFixture(tx *gorm.DB, p DeliveryPlan, e events.Envelope) error {
	body, err := e.MarshalJSON()
	if err != nil {
		return err
	}
	hash := sha256.Sum256(body)
	type child struct {
		Destination events.Owner `json:"destination"`
		AdmissionID string       `json:"admission_id"`
	}
	children := make([]child, 0, len(p.recipients))
	for _, r := range p.recipients {
		children = append(children, child{r.Destination, r.AdmissionID})
	}
	encoded, _ := json.Marshal(children)
	if err := tx.Exec(`INSERT INTO eventstore.outbox_messages(event_id,source_owner,aggregate_type,aggregate_id,integration_sequence,envelope,envelope_hash,recipients,plan_authority_ref)
 VALUES(?::uuid,?,?,?::uuid,?,?,?,?::jsonb,?)`, e.EventID(), string(e.Owner()), e.AggregateType(), e.AggregateID(), e.IntegrationSequence().Int64(), body, hash[:], string(encoded), p.authority).Error; err != nil {
		return err
	}
	for _, r := range p.recipients {
		// T-917 currently reconstructs the schema from this shorter fixture
		// route. This is NOT the established T-011 route contract; its schema
		// correction is coordinator-owned and remains an integration gate.
		fixtureRoute := string(e.Owner()) + "." + string(r.Destination) + strings.TrimPrefix(e.EventType(), string(e.Owner()))
		if err := tx.Exec(`INSERT INTO eventstore.outbox_deliveries(event_id,destination,admission_id,exchange,routing_key) VALUES(?::uuid,?,?::uuid,?,?)`, e.EventID(), string(r.Destination), r.AdmissionID, IntegrationExchange, fixtureRoute).Error; err != nil {
			return err
		}
	}
	return nil
}

func sourceDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	name := "justixauto-t918-" + uuid.NewString()
	password := "synthetic-" + uuid.NewString()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	const image = "docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"
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
		t.Fatal(got, err)
	}
	if out, err := sql("CREATE ROLE justix_inventory_runtime LOGIN PASSWORD '" + password + "'"); err != nil {
		t.Fatal(out, err)
	}
	for _, file := range []string{"../eventstore/schema.sql", "../eventstore/migrations/000002_messaging_delivery.up.sql"} {
		script, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if out, err := sql(string(script), "-v", "owner_service=inventory", "-v", "runtime_role=justix_inventory_runtime"); err != nil {
			t.Fatalf("install %s: %v %s", file, err, out)
		}
	}
	// Explicit synthetic empty-database activation; never auto-activates an
	// existing owner or infers approval from migration installation.
	if out, err := sql(`SELECT eventstore.activate_messaging_custody('` + uuid.NewString() + `',sha256('new-fixture'::bytea),sha256('old-fixture'::bytea),'synthetic-empty-backup','synthetic-no-runtimes','synthetic-no-broker-or-checkpoints','synthetic-compatible-fixture')`); err != nil {
		t.Fatal(out, err)
	}
	out, err := exec.CommandContext(ctx, "docker", "port", name, "5432/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	address := strings.TrimSpace(string(out))
	if !strings.HasPrefix(address, "127.0.0.1:") || strings.Contains(address, "\n") {
		t.Fatal("unexpected binding", address)
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
