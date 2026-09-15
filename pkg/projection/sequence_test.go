package projection_test

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"justixauto/pkg/projection"
)

type payload struct {
	Ref string `json:"ref"`
}

func eventSchema(t *testing.T) events.EventSchema[payload] {
	t.Helper()
	s, e := events.NewEventSchema("inventory.fixture.changed.v1", 1, "fixture", events.CompanyScopeTenantRequired, func(p payload) error {
		if p.Ref == "" {
			return errors.New("missing fixture ref")
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return s
}

type fixtureContract struct {
	contract projection.Contract
	input    projection.ContractInput
	company  string
}

func contract(t *testing.T) fixtureContract {
	t.Helper()
	in := projection.ContractInput{LocalOwner: events.OwnerInventory, SourceOwner: events.OwnerInventory, Consumer: "fixture-" + uuid.NewString(), AggregateType: "fixture", AggregateID: uuid.NewString(), Kind: "projection", Generation: " generation-1 ", ContractID: "fixture-contract", ContractVersion: 1, ContractDigest: sha256.Sum256([]byte("synthetic-contract"))}
	c, e := projection.NewContract(in)
	if e != nil {
		t.Fatal(e)
	}
	return fixtureContract{c, in, uuid.NewString()}
}
func message(t *testing.T, c fixtureContract, sequence int64) []byte {
	t.Helper()
	rev, e := events.NewRevision(sequence)
	if e != nil {
		t.Fatal(e)
	}
	v, e := events.NewEnvelope(events.EnvelopeInput{EventID: uuid.NewString(), AggregateID: c.input.AggregateID, AggregateVersion: rev, IntegrationSequence: rev, CompanyID: &c.company, OccurredAt: time.Now().UTC(), Actor: events.Actor{Kind: events.ActorService, ID: uuid.NewString()}, CorrelationID: uuid.NewString(), CausationID: uuid.NewString(), OperationID: uuid.NewString()}, eventSchema(t), payload{"synthetic-reference"})
	if e != nil {
		t.Fatal(e)
	}
	raw, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
func validate(t *testing.T, c fixtureContract) func(events.Envelope) error {
	t.Helper()
	s := eventSchema(t)
	return func(e events.Envelope) error {
		if _, err := events.DecodeData(e, s); err != nil {
			return err
		}
		company, ok := e.CompanyID()
		if !ok || company != c.company {
			return errors.New("fixture scope denied")
		}
		return nil
	}
}
func position(t *testing.T, c fixtureContract, b []byte, irrelevant bool) projection.ValidatedPosition {
	t.Helper()
	p, e := projection.NewPosition(c.contract, b, eventSchema(t), irrelevant, validate(t, c))
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func bootstrap(t *testing.T, c fixtureContract, start int64, admission bool) projection.VerifiedBootstrap {
	t.Helper()
	b := projection.BootstrapInput{BootstrapID: uuid.NewString(), RequestID: uuid.NewString(), AuthorityRef: "synthetic-authority", ScopeRef: "synthetic-scope", PurposeRef: "synthetic-read-purpose", SourceCheckpointRef: "synthetic-source-highwater", StartAfter: start, NoSnapshotContractRef: "synthetic-no-snapshot"}
	if start == 0 {
		b.NewEmptyProofRef = "synthetic-authenticated-new-empty"
	}
	if admission {
		b.AdmissionID = uuid.NewString()
	}
	v, e := projection.VerifyBootstrap(context.Background(), c.contract, b, func(context.Context, projection.BootstrapInput) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	return v
}

func TestSealedInputValidation(t *testing.T) {
	c := contract(t)
	raw := message(t, c, 1)
	for _, label := range []string{" ", "\t", " padded-label "} {
		in := c.input
		in.Generation = label
		if _, e := projection.NewContract(in); e != nil {
			t.Fatal("exact nonempty retained label rejected", e)
		}
	}
	for _, change := range []func(*projection.ContractInput){func(v *projection.ContractInput) { v.Generation = "" }, func(v *projection.ContractInput) { v.Kind = "unknown" }, func(v *projection.ContractInput) { v.AggregateID = uuid.Nil.String() }, func(v *projection.ContractInput) { v.ContractVersion = 0 }, func(v *projection.ContractInput) { v.SourceOwner = "unknown" }} {
		in := c.input
		change(&in)
		if _, e := projection.NewContract(in); e == nil {
			t.Fatal("invalid contract accepted")
		}
	}
	if _, e := projection.NewPosition(c.contract, raw, eventSchema(t), false, nil); e == nil {
		t.Fatal("nil scope verifier")
	}
	wrong := c
	wrong.company = uuid.NewString()
	if _, e := projection.NewPosition(c.contract, raw, eventSchema(t), true, validate(t, wrong)); e == nil {
		t.Fatal("irrelevant scope bypass")
	}
	other := contract(t)
	if _, e := projection.NewPosition(other.contract, raw, eventSchema(t), false, validate(t, c)); e == nil {
		t.Fatal("wrong stream")
	}
	for _, body := range [][]byte{nil, []byte("null"), []byte("{}"), append(bytes.Clone(raw), []byte("{}")...)} {
		if _, e := projection.NewPosition(c.contract, body, eventSchema(t), false, validate(t, c)); e == nil {
			t.Fatal("malformed accepted")
		}
	}
	p := position(t, c, raw, false)
	hash := p.Hash()
	raw[0] = 'x'
	if p.Hash() != hash || p.Envelope().EventID() == "" {
		t.Fatal("caller mutated sealed position")
	}
	b := bootstrap(t, c, 0, false).Input()
	for _, change := range []func(*projection.BootstrapInput){func(v *projection.BootstrapInput) { v.NewEmptyProofRef = "" }, func(v *projection.BootstrapInput) { v.StartAfter = 1 }, func(v *projection.BootstrapInput) { v.BootstrapID = uuid.Nil.String() }, func(v *projection.BootstrapInput) { v.SnapshotManifestRef = "unsafe-ambiguous" }, func(v *projection.BootstrapInput) { v.AuthorityRef = "" }} {
		in := b
		change(&in)
		if _, e := projection.VerifyBootstrap(context.Background(), c.contract, in, func(context.Context, projection.BootstrapInput) error { return nil }); e == nil {
			t.Fatal("invalid bootstrap")
		}
	}
	denied := errors.New("source denied")
	if _, e := projection.VerifyBootstrap(context.Background(), c.contract, b, func(context.Context, projection.BootstrapInput) error { return denied }); !errors.Is(e, denied) {
		t.Fatal(e)
	}
	if _, e := projection.NewCheckpoints(context.Background(), &gorm.DB{}, eventstore.PreparedFences{}, c.contract, func(*gorm.DB) (port, error) { t.Fatal("factory before fence"); return nil, nil }); e == nil {
		t.Fatal("unprepared adapter")
	}
	if e := projection.SubmitRecovery(context.Background(), projection.VerifiedRecovery{}, projection.DirectWithoutBrokerDelivery, func(context.Context, projection.RecoveryPath, []byte) error {
		t.Fatal("zero recovery submitted")
		return nil
	}); e == nil {
		t.Fatal("zero recovery")
	}
}

// These typed owner ports and tables are deliberately synthetic. They prove
// same-transaction composition, not a real owner's guard, admission or policy.
type port interface {
	Effect(context.Context, projection.ValidatedPosition) error
	Snapshot(context.Context, string) error
	Coverage(context.Context, projection.DuplicateCoverage) error
}
type adapter struct {
	tx       *gorm.DB
	consumer string
}

func (a adapter) Effect(ctx context.Context, p projection.ValidatedPosition) error {
	h := p.Hash()
	e := p.Envelope()
	return a.tx.WithContext(ctx).Exec("INSERT INTO fixture.effects(consumer,event_id,stream,position,hash) VALUES(?,?,?,?,?)", a.consumer, e.EventID(), e.AggregateID(), e.IntegrationSequence().Int64(), h[:]).Error
}
func (a adapter) Snapshot(ctx context.Context, id string) error {
	return a.tx.WithContext(ctx).Exec("INSERT INTO fixture.snapshots(id) VALUES(?)", id).Error
}
func (a adapter) Coverage(ctx context.Context, c projection.DuplicateCoverage) error {
	p := c.Position
	h := p.Hash()
	var count int64
	err := a.tx.WithContext(ctx).Table("fixture.effects").Where("consumer=? AND event_id=? AND stream=? AND position=? AND hash=?", a.consumer, p.Envelope().EventID(), p.Envelope().AggregateID(), p.Envelope().IntegrationSequence().Int64(), h[:]).Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return projection.ErrReconciliationHold
	}
	return nil
}
func authorizePosition(_ context.Context, _ port, _ projection.ValidatedPosition) error  { return nil }
func authorizeBootstrap(_ context.Context, _ port, _ projection.VerifiedBootstrap) error { return nil }
func authorizeGap(_ context.Context, _ port, _ projection.GapEvidence) error             { return nil }
func authorizeAttempt(_ context.Context, _ port, _ projection.AttemptInput) error        { return nil }
func noSnapshot(context.Context, port) error                                             { return nil }
func apply(ctx context.Context, p port, v projection.ValidatedPosition) error {
	return p.Effect(ctx, v)
}
func coverage(ctx context.Context, p port, v projection.DuplicateCoverage) error {
	return p.Coverage(ctx, v)
}
func bind(c fixtureContract) func(*gorm.DB) (port, error) {
	return func(tx *gorm.DB) (port, error) { return adapter{tx, c.input.Consumer}, nil }
}
func prepared(ctx context.Context, tx *gorm.DB, c fixtureContract, mode eventstore.FenceMode) (eventstore.PreparedFences, error) {
	r, e := eventstore.ReceiverFence(c.input.LocalOwner, c.input.SourceOwner, c.input.AggregateType, c.input.AggregateID)
	if e != nil {
		return eventstore.PreparedFences{}, e
	}
	return eventstore.PrepareFences(ctx, tx, c.input.LocalOwner, mode, append([]eventstore.FenceRequest{r}, c.input.RequiredFences...)...)
}
func bound(ctx context.Context, tx *gorm.DB, c fixtureContract, mode eventstore.FenceMode) (*projection.Checkpoints[port], error) {
	f, e := prepared(ctx, tx, c, mode)
	if e != nil {
		return nil, e
	}
	return projection.NewCheckpoints(ctx, tx, f, c.contract, bind(c))
}
func run(t *testing.T, db *gorm.DB, c fixtureContract, mode eventstore.FenceMode, fn func(*projection.Checkpoints[port], *gorm.DB) error) error {
	t.Helper()
	ctx := context.Background()
	r, e := eventstore.NewTransactions(db, func(tx *gorm.DB) (*gorm.DB, error) { return tx, nil })
	if e != nil {
		t.Fatal(e)
	}
	return r.Run(ctx, func(tx *gorm.DB) error {
		a, e := bound(ctx, tx, c, mode)
		if e != nil {
			return e
		}
		return fn(a, tx)
	})
}
func install(t *testing.T, f *fixture, c fixtureContract, b projection.VerifiedBootstrap) {
	t.Helper()
	e := run(t, f.db, c, eventstore.ExclusiveFence, func(a *projection.Checkpoints[port], tx *gorm.DB) error {
		if b.Input().AdmissionID != "" {
			if e := admit(tx, c, b.Input()); e != nil {
				return e
			}
		}
		_, e := a.InstallBootstrap(context.Background(), b, authorizeBootstrap, noSnapshot)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
}
func admit(tx *gorm.DB, c fixtureContract, b projection.BootstrapInput) error {
	return tx.Exec(`INSERT INTO eventstore.messaging_admissions(admission_id,namespace,source_owner,aggregate_type,aggregate_id,subject,action,contract_id,contract_version,contract_digest,schema_allowlist,authority_ref,scope_ref,purpose,bootstrap_ref,start_after,consumer_kind,generation)
 VALUES(?,'local-consumer',?,?,?,?, 'admit',?,?,?, '["inventory.fixture.changed.v1"]',?,?,?,?,?,?,?)`, b.AdmissionID, c.input.SourceOwner, c.input.AggregateType, c.input.AggregateID, c.input.Consumer, c.input.ContractID, c.input.ContractVersion, c.input.ContractDigest[:], b.AuthorityRef, b.ScopeRef, b.PurposeRef, b.SourceCheckpointRef, b.StartAfter, c.input.Kind, c.input.Generation).Error
}
func progress(t *testing.T, f *fixture, c fixtureContract) int64 {
	t.Helper()
	var p int64
	if e := f.db.Raw("SELECT position FROM eventstore.consumer_checkpoints WHERE consumer_name=? AND aggregate_id=?", c.input.Consumer, c.input.AggregateID).Scan(&p).Error; e != nil {
		t.Fatal(e)
	}
	return p
}
func count(t *testing.T, f *fixture, table string, c fixtureContract) int64 {
	t.Helper()
	var n int64
	column := "consumer_name"
	if table == "fixture.effects" {
		column = "consumer"
	}
	if e := f.db.Table(table).Where(column+"=?", c.input.Consumer).Count(&n).Error; e != nil {
		t.Fatal(e)
	}
	return n
}

// Direct composition prepares fences in T-013's factory, before its inbox row.
// Duplicate completion checks are exercised through caller-bound custody below.
func consume(t *testing.T, db *gorm.DB, c fixtureContract, raw []byte, irrelevant bool, ack func() error) (inbox.Outcome, error) {
	t.Helper()
	p := position(t, c, raw, irrelevant)
	ctx := context.Background()
	consumer, e := inbox.NewConsumer(db, c.input.Consumer, func(tx *gorm.DB) (*projection.Checkpoints[port], error) {
		return bound(ctx, tx, c, eventstore.SharedFence)
	}, validate(t, c))
	if e != nil {
		t.Fatal(e)
	}
	return consumer.Consume(ctx, raw, func(a *projection.Checkpoints[port], _ events.Envelope) error {
		n, e := a.CheckNext(ctx, p, authorizePosition)
		if e != nil {
			return e
		}
		return a.Advance(ctx, n, apply)
	}, ack)
}

func TestCheckpointPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_PROJECTION_SEQUENCE") != "1" {
		t.Skip("explicit isolated PostgreSQL fixture opt-in")
	}
	f := newFixture(t)
	t.Run("bootstrap-atomic-replay-conflict-and-labels", func(t *testing.T) {
		for _, kind := range []string{"projection", "process"} {
			c := contract(t)
			c.input.Kind = kind
			c.contract, _ = projection.NewContract(c.input)
			b := bootstrap(t, c, 9, true)
			calls := 0
			operation := func(fail bool) error {
				return run(t, f.db, c, eventstore.ExclusiveFence, func(a *projection.Checkpoints[port], tx *gorm.DB) error {
					if e := admit(tx, c, b.Input()); e != nil {
						return e
					}
					_, e := a.InstallBootstrap(context.Background(), b, authorizeBootstrap, func(ctx context.Context, p port) error {
						calls++
						if e := p.Snapshot(ctx, b.Input().BootstrapID); e != nil {
							return e
						}
						if fail {
							return errors.New("fixture snapshot incomplete")
						}
						return nil
					})
					return e
				})
			}
			if e := operation(true); e == nil {
				t.Fatal("failed snapshot committed")
			}
			if n := count(t, f, "eventstore.consumer_bootstraps", c); n != 0 {
				t.Fatal("partial bootstrap", n)
			}
			if e := operation(false); e != nil {
				t.Fatal(e)
			}
			if progress(t, f, c) != 9 {
				t.Fatal("wrong highwater")
			}
			e := run(t, f.db, c, eventstore.ExclusiveFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
				fresh, e := a.InstallBootstrap(context.Background(), b, authorizeBootstrap, func(context.Context, port) error { t.Fatal("snapshot repeated"); return nil })
				if fresh {
					t.Fatal("request replay fresh")
				}
				return e
			})
			if e != nil {
				t.Fatal(e)
			}
			other := bootstrap(t, c, 0, false)
			if e := run(t, f.db, c, eventstore.ExclusiveFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
				_, e := a.InstallBootstrap(context.Background(), other, authorizeBootstrap, noSnapshot)
				return e
			}); !errors.Is(e, projection.ErrConflict) {
				t.Fatal("reset accepted", e)
			}
			denied := errors.New("current admission denied")
			if e := run(t, f.db, c, eventstore.ExclusiveFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
				_, e := a.InstallBootstrap(context.Background(), b, func(context.Context, port, projection.VerifiedBootstrap) error { return denied }, noSnapshot)
				return e
			}); !errors.Is(e, denied) {
				t.Fatal(e)
			}
			if calls != 2 {
				t.Fatal(calls)
			}
		}
	})
	t.Run("exact-sequence-noop-duplicates-and-gaps", func(t *testing.T) {
		c := contract(t)
		b := bootstrap(t, c, 0, false)
		install(t, f, c, b)
		acked := 0
		ack := func() error { acked++; return nil }
		one := message(t, c, 1)
		two := message(t, c, 2)
		four := message(t, c, 4)
		if out, e := consume(t, f.db, c, one, false, ack); e != nil || out != inbox.Applied {
			t.Fatal(out, e)
		}
		if out, e := consume(t, f.db, c, one, false, ack); e != nil || out != inbox.Duplicate {
			t.Fatal(out, e)
		}
		if _, e := consume(t, f.db, c, two, true, ack); e != nil {
			t.Fatal(e)
		}
		if progress(t, f, c) != 2 || count(t, f, "fixture.effects", c) != 1 || acked != 3 {
			t.Fatal("wrong noop/duplicate")
		}
		out, e := consume(t, f.db, c, four, false, ack)
		var gap *projection.Gap
		if !errors.As(e, &gap) || out != inbox.Unconfirmed || acked != 3 {
			t.Fatal(out, e, acked)
		}
		if count(t, f, "eventstore.inbox", c) != 2 || count(t, f, "eventstore.consumer_gaps", c) != 0 || progress(t, f, c) != 2 {
			t.Fatal("gap effect survived rollback")
		}
		from, through := gap.MissingInterval()
		if from != 3 || through != 3 {
			t.Fatal(from, through)
		}
		ev := gapEvidence()
		var record projection.GapRecord
		e = run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			var err error
			record, err = a.RecordGap(context.Background(), gap, ev, authorizeGap)
			return err
		})
		if e != nil {
			t.Fatal(e)
		}
		if record.Resolved || count(t, f, "eventstore.consumer_gaps", c) != 1 {
			t.Fatal(record)
		}
		if e = run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.RecordGap(context.Background(), gap, ev, authorizeGap)
			return e
		}); e != nil {
			t.Fatal("record replay", e)
		}
		req, e := projection.ReadRecovery(context.Background(), f.db, c.contract, ev.GapID, ev.AttemptID, ev.AttemptRequestID, allowRecovery)
		if e != nil {
			t.Fatal(e)
		}
		three := message(t, c, 3)
		source := &historySource{bodies: [][]byte{three}}
		recovered, e := projection.FetchRecovery(context.Background(), req, source, func(raw []byte) (projection.ValidatedPosition, error) {
			return projection.NewPosition(c.contract, raw, eventSchema(t), false, validate(t, c))
		})
		if e != nil {
			t.Fatal(e)
		}
		attempt := projection.AttemptInput{GapID: ev.GapID, AttemptID: uuid.NewString(), RequestID: uuid.NewString(), PriorAttemptID: ev.AttemptID, Action: "recovered", AuthorityRef: "synthetic-history-authority", Recovery: recovered}
		if e = run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.AppendAttempt(context.Background(), attempt, authorizeAttempt)
			return e
		}); e != nil {
			t.Fatal(e)
		}
		if _, e = projection.ReadRecovery(context.Background(), f.db, c.contract, ev.GapID, ev.AttemptID, ev.AttemptRequestID, allowRecovery); !errors.Is(e, projection.ErrReconciliationHold) {
			t.Fatal("stale attempt fetched", e)
		}
		e = projection.SubmitRecovery(context.Background(), recovered, projection.DirectWithoutBrokerDelivery, func(ctx context.Context, path projection.RecoveryPath, raw []byte) error {
			if path != projection.DirectWithoutBrokerDelivery || !bytes.Equal(raw, three) {
				t.Fatal("wrong path/bytes")
			}
			_, e := consume(t, f.db, c, raw, false, func() error { return nil })
			return e
		})
		if e != nil {
			t.Fatal(e)
		}
		if acked != 3 || progress(t, f, c) != 3 {
			t.Fatal("recovery ACKed blocked original")
		}
		resolved := projection.AttemptInput{GapID: ev.GapID, AttemptID: uuid.NewString(), RequestID: uuid.NewString(), PriorAttemptID: attempt.AttemptID, Action: "resolved", AuthorityRef: "synthetic-history-authority"}
		if e = run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			r, e := a.AppendAttempt(context.Background(), resolved, authorizeAttempt)
			if !r.Resolved {
				t.Fatal("not resolved")
			}
			return e
		}); e != nil {
			t.Fatal(e)
		}
		if _, e = consume(t, f.db, c, four, false, ack); e != nil {
			t.Fatal(e)
		}
		if progress(t, f, c) != 4 || acked != 4 {
			t.Fatal("not converged")
		}
	})
	t.Run("concurrent-delivery-and-independent-consumers", func(t *testing.T) {
		c := contract(t)
		install(t, f, c, bootstrap(t, c, 0, false))
		body := message(t, c, 1)
		var wg sync.WaitGroup
		errs := make(chan error, 6)
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, e := consume(t, f.db, c, body, false, func() error { return nil })
				errs <- e
			}()
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		if count(t, f, "fixture.effects", c) != 1 || progress(t, f, c) != 1 {
			t.Fatal("duplicate effect")
		}
		d := c
		d.input.Consumer = "other-" + uuid.NewString()
		d.input.Generation = "other-generation"
		d.contract, _ = projection.NewContract(d.input)
		install(t, f, d, bootstrap(t, d, 0, false))
		if _, e := consume(t, f.db, d, body, false, func() error { return nil }); e != nil {
			t.Fatal(e)
		}
		if count(t, f, "fixture.effects", d) != 1 {
			t.Fatal("consumer suppressed")
		}
	})
	t.Run("same-transaction-gap-and-candidate-custody", func(t *testing.T) {
		c := contract(t)
		install(t, f, c, bootstrap(t, c, 0, false))
		p := position(t, c, message(t, c, 2), false)
		e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, err := a.CheckNext(context.Background(), p, authorizePosition)
			var g *projection.Gap
			if !errors.As(err, &g) {
				return errors.New("missing gap")
			}
			if _, e := a.RecordGap(context.Background(), g, gapEvidence(), authorizeGap); !errors.Is(e, projection.ErrSeparateTransaction) {
				t.Fatal(e)
			}
			return err
		})
		if !errors.Is(e, projection.ErrGap) {
			t.Fatal(e)
		}
		if count(t, f, "eventstore.consumer_gaps", c) != 0 {
			t.Fatal("gap persisted in failed effect tx")
		}
	})
	t.Run("bootstrap-label-meaning-and-prepared-prerequisites", func(t *testing.T) {
		c := contract(t)
		b := bootstrap(t, c, 0, false)
		install(t, f, c, b)
		d := c
		d.input.AggregateID = uuid.NewString()
		d.input.Generation = "changed-label"
		d.contract, _ = projection.NewContract(d.input)
		other := bootstrap(t, d, 0, false)
		if e := run(t, f.db, d, eventstore.ExclusiveFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.InstallBootstrap(context.Background(), other, authorizeBootstrap, noSnapshot)
			return e
		}); !errors.Is(e, projection.ErrConflict) {
			t.Fatal("consumer meaning changed", e)
		}
		if e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.InstallBootstrap(context.Background(), b, authorizeBootstrap, noSnapshot)
			return e
		}); !errors.Is(e, eventstore.ErrFencePreparation) {
			t.Fatal("shared membership fence", e)
		}
		tx := f.db.Begin()
		defer tx.Rollback()
		fences, e := prepared(context.Background(), tx, c, eventstore.SharedFence)
		if e != nil {
			t.Fatal(e)
		}
		g, e := eventstore.GenerationFence(events.OwnerInventory, "fixture-projection", uuid.NewString(), eventstore.SharedFence)
		if e != nil {
			t.Fatal(e)
		}
		d = c
		d.input.RequiredFences = []eventstore.FenceRequest{g}
		d.contract, _ = projection.NewContract(d.input)
		if _, e := projection.NewCheckpoints(context.Background(), tx, fences, d.contract, func(*gorm.DB) (port, error) { t.Fatal("missing prerequisite reached factory"); return nil, nil }); !errors.Is(e, eventstore.ErrFencePreparation) {
			t.Fatal(e)
		}
		otherTX := f.db.Begin()
		defer otherTX.Rollback()
		if _, e := projection.NewCheckpoints(context.Background(), otherTX, fences, c.contract, bind(c)); !errors.Is(e, eventstore.ErrFencePreparation) {
			t.Fatal("foreign transaction", e)
		}
	})
	t.Run("retained-whitespace-labels-and-004-space-only-storage-limit", func(t *testing.T) {
		for _, kind := range []string{"projection", "process"} {
			c := contract(t)
			c.input.Kind = kind
			c.input.Generation = "\t"
			c.contract, _ = projection.NewContract(c.input)
			b := bootstrap(t, c, 0, true)
			install(t, f, c, b)
			var label string
			if e := f.db.Raw("SELECT generation FROM eventstore.consumer_bootstraps WHERE bootstrap_id=?", b.Input().BootstrapID).Scan(&label).Error; e != nil {
				t.Fatal(e)
			}
			if label != "\t" {
				t.Fatal("retained whitespace label normalized")
			}
			d := contract(t)
			d.input.Kind = kind
			d.input.Generation = " "
			var e error
			d.contract, e = projection.NewContract(d.input)
			if e != nil {
				t.Fatal("nonempty label rejected before storage", e)
			}
			space := bootstrap(t, d, 0, true)
			err := run(t, f.db, d, eventstore.ExclusiveFence, func(a *projection.Checkpoints[port], tx *gorm.DB) error {
				if e := admit(tx, d, space.Input()); e != nil {
					return e
				}
				_, e := a.InstallBootstrap(context.Background(), space, authorizeBootstrap, noSnapshot)
				return e
			})
			if err == nil || !strings.Contains(err.Error(), "consumer_bootstraps_generation_check") {
				t.Fatal("expected documented immutable 000004 btrim compatibility limit", err)
			}
			if count(t, f, "eventstore.consumer_bootstraps", d) != 0 || count(t, f, "eventstore.consumer_checkpoints", d) != 0 {
				t.Fatal("unsupported label partially installed")
			}
		}
	})
	t.Run("candidate-hash-position-and-authority-cannot-be-substituted", func(t *testing.T) {
		c := contract(t)
		install(t, f, c, bootstrap(t, c, 0, false))
		raw := message(t, c, 1)
		p := position(t, c, raw, false)
		var escaped projection.NextCandidate
		denied := errors.New("current fixture scope revoked")
		if e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.CheckNext(context.Background(), p, func(context.Context, port, projection.ValidatedPosition) error { return denied })
			return e
		}); !errors.Is(e, denied) {
			t.Fatal(e)
		}
		for _, mode := range []string{"absent-inbox", "wrong-hash", "wrong-event"} {
			e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], tx *gorm.DB) error {
				if mode != "absent-inbox" {
					h := p.Hash()
					id := p.Envelope().EventID()
					if mode == "wrong-hash" {
						h[0] ^= 1
					} else {
						id = uuid.NewString()
					}
					if e := tx.Exec("INSERT INTO eventstore.inbox(consumer_name,event_id,envelope_hash) VALUES(?,?,?)", c.input.Consumer, id, h[:]).Error; e != nil {
						return e
					}
				}
				n, e := a.CheckNext(context.Background(), p, authorizePosition)
				if e != nil {
					return e
				}
				escaped = n
				return a.Advance(context.Background(), n, func(context.Context, port, projection.ValidatedPosition) error {
					t.Fatal("mismatched inbox reached effect")
					return nil
				})
			})
			if !errors.Is(e, projection.ErrReconciliationHold) {
				t.Fatal(mode, e)
			}
		}
		if e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			return a.Advance(context.Background(), escaped, apply)
		}); !errors.Is(e, projection.ErrInvalid) {
			t.Fatal("escaped candidate", e)
		}
		if e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], tx *gorm.DB) error {
			h := p.Hash()
			if e := tx.Exec("INSERT INTO eventstore.inbox(consumer_name,event_id,envelope_hash) VALUES(?,?,?)", c.input.Consumer, p.Envelope().EventID(), h[:]).Error; e != nil {
				return e
			}
			n, e := a.CheckNext(context.Background(), p, authorizePosition)
			if e != nil {
				return e
			}
			if e = a.Advance(context.Background(), n, apply); e != nil {
				return e
			}
			if e = a.Advance(context.Background(), n, func(context.Context, port, projection.ValidatedPosition) error {
				t.Fatal("candidate reused")
				return nil
			}); !errors.Is(e, projection.ErrConflict) {
				t.Fatal(e)
			}
			return e
		}); !errors.Is(e, projection.ErrConflict) {
			t.Fatal(e)
		}
		if progress(t, f, c) != 0 || count(t, f, "fixture.effects", c) != 0 || count(t, f, "eventstore.inbox", c) != 0 {
			t.Fatal("candidate failure partially committed")
		}
	})
	t.Run("bootstrap-and-gap-unknown-commit-reconcile-exact-request", func(t *testing.T) {
		for _, committed := range []bool{false, true} {
			c := contract(t)
			b := bootstrap(t, c, 0, false)
			pool, e := f.db.DB()
			if e != nil {
				t.Fatal(e)
			}
			fault := f.db.Session(&gorm.Session{NewDB: true})
			fault.Statement = &gorm.Statement{DB: fault, ConnPool: commitFaultPool{pool, committed}}
			calls := 0
			err := run(t, fault, c, eventstore.ExclusiveFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
				_, e := a.InstallBootstrap(context.Background(), b, authorizeBootstrap, func(ctx context.Context, p port) error { calls++; return p.Snapshot(ctx, b.Input().BootstrapID) })
				return e
			})
			if !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) || calls != 1 {
				t.Fatal(err, calls)
			}
			err = run(t, f.db, c, eventstore.ExclusiveFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
				fresh, e := a.InstallBootstrap(context.Background(), b, authorizeBootstrap, func(ctx context.Context, p port) error { calls++; return p.Snapshot(ctx, b.Input().BootstrapID) })
				if fresh == committed {
					t.Fatal("bootstrap receipt assumed outcome")
				}
				return e
			})
			if err != nil {
				t.Fatal(err)
			}
			if (committed && calls != 1) || (!committed && calls != 2) {
				t.Fatal(calls)
			}
			_, err = consume(t, f.db, c, message(t, c, 2), false, func() error { t.Fatal("gap ACK"); return nil })
			var gap *projection.Gap
			if !errors.As(err, &gap) {
				t.Fatal(err)
			}
			ev := gapEvidence()
			err = run(t, fault, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
				_, e := a.RecordGap(context.Background(), gap, ev, authorizeGap)
				return e
			})
			if !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) {
				t.Fatal(err)
			}
			_, err = projection.ReadRecovery(context.Background(), f.db, c.contract, ev.GapID, ev.AttemptID, ev.AttemptRequestID, allowRecovery)
			if committed && err != nil {
				t.Fatal("committed request missing", err)
			}
			if !committed && !errors.Is(err, projection.ErrReconciliationHold) {
				t.Fatal("rolled-back request fetched", err)
			}
			if err = run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
				_, e := a.RecordGap(context.Background(), gap, ev, authorizeGap)
				return e
			}); err != nil {
				t.Fatal(err)
			}
			if count(t, f, "eventstore.consumer_gaps", c) != 1 {
				t.Fatal("gap request duplicated")
			}
		}
	})
	t.Run("source-prerequisite-order-and-cancelled-contention", func(t *testing.T) {
		c := contract(t)
		install(t, f, c, bootstrap(t, c, 0, false))
		source, e := eventstore.SourceFence(events.OwnerInventory, "fixture", uuid.NewString())
		if e != nil {
			t.Fatal(e)
		}
		c.input.RequiredFences = []eventstore.FenceRequest{source}
		c.contract, _ = projection.NewContract(c.input)
		holder := f.db.Begin()
		defer holder.Rollback()
		if _, e := prepared(context.Background(), holder, c, eventstore.SharedFence); e != nil {
			t.Fatal(e)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
		defer cancel()
		tx := f.db.WithContext(ctx).Begin()
		defer tx.Rollback()
		if _, e := bound(ctx, tx, c, eventstore.SharedFence); e == nil {
			t.Fatal("contended receiver did not cancel")
		}
		if e := holder.Rollback().Error; e != nil {
			t.Fatal(e)
		}
		if e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], tx *gorm.DB) error {
			if _, e := eventstore.PrepareFences(context.Background(), tx, events.OwnerInventory, eventstore.SharedFence, source); !errors.Is(e, eventstore.ErrFencePreparation) {
				t.Fatal("late preparation", e)
			}
			return nil
		}); e != nil {
			t.Fatal("locks not released", e)
		}
	})
	t.Run("rollback-panic-cancel-and-lost-commit", func(t *testing.T) {
		for _, commit := range []bool{false, true} {
			c := contract(t)
			install(t, f, c, bootstrap(t, c, 0, false))
			body := message(t, c, 1)
			pool, e := f.db.DB()
			if e != nil {
				t.Fatal(e)
			}
			fault := f.db.Session(&gorm.Session{NewDB: true})
			fault.Statement = &gorm.Statement{DB: fault, ConnPool: commitFaultPool{pool, commit}}
			acks := 0
			out, e := consume(t, fault, c, body, false, func() error { acks++; return nil })
			if !errors.Is(e, eventstore.ErrCommitOutcomeUnknown) || out != inbox.Unconfirmed || acks != 0 {
				t.Fatal(out, e, acks)
			}
			want := int64(0)
			if commit {
				want = 1
			}
			if progress(t, f, c) != want || count(t, f, "fixture.effects", c) != want {
				t.Fatal("unknown outcome assumed")
			}
			if _, e = consume(t, f.db, c, body, false, func() error { acks++; return nil }); e != nil {
				t.Fatal(e)
			}
			if progress(t, f, c) != 1 || count(t, f, "fixture.effects", c) != 1 || acks != 1 {
				t.Fatal("receipt reconciliation")
			}
		}
		c := contract(t)
		install(t, f, c, bootstrap(t, c, 0, false))
		raw := message(t, c, 1)
		p := position(t, c, raw, false)
		for _, behavior := range []string{"error", "panic", "cancel"} {
			func() {
				defer func() {
					if v := recover(); v != nil && behavior != "panic" {
						t.Fatal(v)
					}
				}()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				r, _ := eventstore.NewTransactions(f.db, func(tx *gorm.DB) (*gorm.DB, error) { return tx, nil })
				e := r.Run(ctx, func(tx *gorm.DB) error {
					a, e := bound(ctx, tx, c, eventstore.SharedFence)
					if e != nil {
						return e
					}
					h := p.Hash()
					if e = tx.Exec("INSERT INTO eventstore.inbox(consumer_name,event_id,envelope_hash) VALUES(?,?,?)", c.input.Consumer, p.Envelope().EventID(), h[:]).Error; e != nil {
						return e
					}
					n, e := a.CheckNext(ctx, p, authorizePosition)
					if e != nil {
						return e
					}
					return a.Advance(ctx, n, func(ctx context.Context, u port, p projection.ValidatedPosition) error {
						if e := u.Effect(ctx, p); e != nil {
							return e
						}
						switch behavior {
						case "panic":
							panic("synthetic panic")
						case "cancel":
							cancel()
							return ctx.Err()
						default:
							return errors.New("synthetic effect failure")
						}
					})
				})
				if e == nil {
					t.Fatal("failed effect committed")
				}
			}()
			if progress(t, f, c) != 0 || count(t, f, "fixture.effects", c) != 0 || count(t, f, "eventstore.inbox", c) != 0 {
				t.Fatal("partial failed effect", behavior)
			}
		}
	})
	t.Run("recovery-invalid-interval-scope-hash-and-denial", func(t *testing.T) {
		c := contract(t)
		install(t, f, c, bootstrap(t, c, 0, false))
		_, err := consume(t, f.db, c, message(t, c, 3), false, func() error { t.Fatal("gap ACK"); return nil })
		var gap *projection.Gap
		if !errors.As(err, &gap) {
			t.Fatal(err)
		}
		ev := gapEvidence()
		if e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.RecordGap(context.Background(), gap, ev, authorizeGap)
			return e
		}); e != nil {
			t.Fatal(e)
		}
		req, e := projection.ReadRecovery(context.Background(), f.db, c.contract, ev.GapID, ev.AttemptID, ev.AttemptRequestID, allowRecovery)
		if e != nil {
			t.Fatal(e)
		}
		one, two := message(t, c, 1), message(t, c, 2)
		for _, src := range []*historySource{{bodies: [][]byte{one}}, {bodies: [][]byte{two, one}}, {bodies: [][]byte{one, one}}, {bodies: [][]byte{one, two}, badHash: true}, {bodies: [][]byte{one, two}, denied: true}, {bodies: [][]byte{one, message(t, contract(t), 2)}}} {
			if _, e := projection.FetchRecovery(context.Background(), req, src, func(raw []byte) (projection.ValidatedPosition, error) {
				return projection.NewPosition(c.contract, raw, eventSchema(t), false, validate(t, c))
			}); e == nil {
				t.Fatal("invalid history accepted")
			}
		}
		if progress(t, f, c) != 0 {
			t.Fatal("invalid recovery advanced")
		}
		hold := projection.AttemptInput{GapID: ev.GapID, AttemptID: uuid.NewString(), RequestID: uuid.NewString(), PriorAttemptID: ev.AttemptID, Action: "held", AuthorityRef: "synthetic-denial", HoldReasonCode: "source-denied"}
		if e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.AppendAttempt(context.Background(), hold, authorizeAttempt)
			return e
		}); e != nil {
			t.Fatal(e)
		}
		if _, e := projection.ReadRecovery(context.Background(), f.db, c.contract, ev.GapID, ev.AttemptID, ev.AttemptRequestID, allowRecovery); !errors.Is(e, projection.ErrReconciliationHold) {
			t.Fatal(e)
		}
		if e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.AppendAttempt(context.Background(), hold, authorizeAttempt)
			return e
		}); e != nil {
			t.Fatal("hold replay", e)
		}
		changed := hold
		changed.HoldReasonCode = "different-denial"
		if e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.AppendAttempt(context.Background(), changed, authorizeAttempt)
			return e
		}); !errors.Is(e, projection.ErrConflict) {
			t.Fatal(e)
		}
	})
	t.Run("stale-gap-records-resolution-and-concurrent-attempts", func(t *testing.T) {
		c := contract(t)
		install(t, f, c, bootstrap(t, c, 0, false))
		_, err := consume(t, f.db, c, message(t, c, 2), false, func() error { return nil })
		var gap *projection.Gap
		if !errors.As(err, &gap) {
			t.Fatal(err)
		}
		if _, e := consume(t, f.db, c, message(t, c, 1), false, func() error { return nil }); e != nil {
			t.Fatal(e)
		}
		ev := gapEvidence()
		if e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			r, e := a.RecordGap(context.Background(), gap, ev, authorizeGap)
			if !r.Resolved {
				t.Fatal("stale gap became hold")
			}
			return e
		}); e != nil {
			t.Fatal(e)
		}
		d := contract(t)
		install(t, f, d, bootstrap(t, d, 0, false))
		_, err = consume(t, f.db, d, message(t, d, 2), false, func() error { return nil })
		if !errors.As(err, &gap) {
			t.Fatal(err)
		}
		ev = gapEvidence()
		if e := run(t, f.db, d, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.RecordGap(context.Background(), gap, ev, authorizeGap)
			return e
		}); e != nil {
			t.Fatal(e)
		}
		var successes atomic.Int32
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			go func() {
				in := projection.AttemptInput{GapID: ev.GapID, AttemptID: uuid.NewString(), RequestID: uuid.NewString(), PriorAttemptID: ev.AttemptID, Action: "held", AuthorityRef: "synthetic", HoldReasonCode: "source-unavailable"}
				e := run(t, f.db, d, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
					_, e := a.AppendAttempt(context.Background(), in, authorizeAttempt)
					return e
				})
				if e == nil {
					successes.Add(1)
				}
				errs <- e
			}()
		}
		for i := 0; i < 2; i++ {
			e := <-errs
			if e != nil && !errors.Is(e, projection.ErrConflict) {
				t.Fatal(e)
			}
		}
		if successes.Load() != 1 {
			t.Fatal("branching attempts", successes.Load())
		}
	})
	t.Run("attempt-retained-identity-before-new-and-replayed-receipts", func(t *testing.T) {
		for _, mutation := range []struct {
			name   string
			change func(*projection.ContractInput)
		}{
			{"kind", func(c *projection.ContractInput) { c.Kind = "process" }},
			{"generation", func(c *projection.ContractInput) { c.Generation = "other-generation" }},
			{"contract-id", func(c *projection.ContractInput) { c.ContractID = "other-contract" }},
			{"contract-version", func(c *projection.ContractInput) { c.ContractVersion++ }},
			{"contract-digest", func(c *projection.ContractInput) { c.ContractDigest[0] ^= 1 }},
		} {
			t.Run(mutation.name, func(t *testing.T) {
				c := contract(t)
				install(t, f, c, bootstrap(t, c, 0, false))
				blocked := message(t, c, 3)
				_, err := consume(t, f.db, c, blocked, false, func() error { t.Error("gap ACK"); return nil })
				var gap *projection.Gap
				if !errors.As(err, &gap) {
					t.Fatal(err)
				}
				ev := gapEvidence()
				if err := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
					_, e := a.RecordGap(context.Background(), gap, ev, authorizeGap)
					return e
				}); err != nil {
					t.Fatal(err)
				}
				wrong := c
				mutation.change(&wrong.input)
				wrong.contract, err = projection.NewContract(wrong.input)
				if err != nil {
					t.Fatal(err)
				}
				in := projection.AttemptInput{GapID: ev.GapID, AttemptID: uuid.NewString(), RequestID: uuid.NewString(), PriorAttemptID: ev.AttemptID, Action: "held", AuthorityRef: "synthetic", HoldReasonCode: "source-unavailable"}
				appendAttempt := func(db *gorm.DB, consumer fixtureContract) error {
					return run(t, db, consumer, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
						_, e := a.AppendAttempt(context.Background(), in, authorizeAttempt)
						return e
					})
				}
				for _, phase := range []string{"new", "replay-after-progress"} {
					if err := appendAttempt(f.db, wrong); !errors.Is(err, projection.ErrReconciliationHold) {
						t.Fatalf("%s: %v", phase, err)
					}
					if _, err := projection.ReadRecovery(context.Background(), f.db, wrong.contract, ev.GapID, ev.AttemptID, ev.AttemptRequestID, allowRecovery); !errors.Is(err, projection.ErrReconciliationHold) {
						t.Fatal(err)
					}
					var attempts int64
					if err := f.db.Table("eventstore.consumer_gap_attempts").Where("gap_id=?", ev.GapID).Count(&attempts).Error; err != nil {
						t.Fatal(err)
					}
					want := int64(1)
					if phase != "new" {
						want = 2
					}
					if attempts != want {
						t.Fatalf("%s changed chain: %d", phase, attempts)
					}
					if phase == "new" {
						pool, err := f.db.DB()
						if err != nil {
							t.Fatal(err)
						}
						fault := f.db.Session(&gorm.Session{NewDB: true})
						fault.Statement = &gorm.Statement{DB: fault, ConnPool: commitFaultPool{pool, true}}
						if err := appendAttempt(fault, c); !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) {
							t.Fatal("lost reply outcome", err)
						}
						if err := appendAttempt(f.db, c); err != nil {
							t.Fatal("exact receipt reconciliation", err)
						}
						if _, err := consume(t, f.db, c, message(t, c, 1), false, func() error { return nil }); err != nil {
							t.Fatal(err)
						}
					} else if err := appendAttempt(f.db, c); err != nil {
						t.Fatal("exact replay after progress", err)
					}
				}
			})
		}
	})
	t.Run("restart-discovers-gap-and-fetches-only-current-missing-interval", func(t *testing.T) {
		c := contract(t)
		install(t, f, c, bootstrap(t, c, 0, false))
		blocked := message(t, c, 4)
		_, err := consume(t, f.db, c, blocked, false, func() error { t.Fatal("gap ACK"); return nil })
		var gap *projection.Gap
		if !errors.As(err, &gap) {
			t.Fatal(err)
		}
		ev := gapEvidence()
		if err = run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.RecordGap(context.Background(), gap, ev, authorizeGap)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		if _, err = consume(t, f.db, c, message(t, c, 1), false, func() error { return nil }); err != nil {
			t.Fatal(err)
		}
		if _, err = projection.ReadRecovery(context.Background(), f.db, c.contract, ev.GapID, ev.AttemptID, ev.AttemptRequestID, allowRecovery); !errors.Is(err, projection.ErrReconciliationHold) {
			t.Fatal("stale interval requested", err)
		}
		var status projection.GapStatus
		if err = run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			var e error
			status, e = a.FindGap(context.Background(), position(t, c, blocked, false), authorizePosition)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		if status.GapID != ev.GapID || status.AttemptID != ev.AttemptID || status.Position != 1 || status.Expected != 1 || status.Observed != 4 {
			t.Fatal(status)
		}
		next := projection.AttemptInput{GapID: status.GapID, AttemptID: uuid.NewString(), RequestID: uuid.NewString(), PriorAttemptID: status.AttemptID, Action: "requested", AuthorityRef: "synthetic-current-history"}
		if err = run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.AppendAttempt(context.Background(), next, authorizeAttempt)
			return e
		}); err != nil {
			t.Fatal(err)
		}
		req, e := projection.ReadRecovery(context.Background(), f.db, c.contract, next.GapID, next.AttemptID, next.RequestID, allowRecovery)
		if e != nil {
			t.Fatal(e)
		}
		from, through := req.Interval()
		if from != 2 || through != 3 {
			t.Fatal("fetched covered history", from, through)
		}
		tx := f.db.Begin()
		if _, e := projection.ReadRecovery(context.Background(), tx, c.contract, next.GapID, next.AttemptID, next.RequestID, allowRecovery); !errors.Is(e, projection.ErrSeparateTransaction) {
			t.Fatal("recovery read inside effect tx", e)
		}
		tx.Rollback()
		wrong := append([]byte(" "), blocked...)
		if e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.FindGap(context.Background(), position(t, c, wrong, false), authorizePosition)
			return e
		}); !errors.Is(e, projection.ErrConflict) {
			t.Fatal("changed blocked bytes found gap", e)
		}
		bodies := [][]byte{message(t, c, 2), message(t, c, 3)}
		rec, e := projection.FetchRecovery(context.Background(), req, &historySource{bodies: bodies}, func(raw []byte) (projection.ValidatedPosition, error) {
			return projection.NewPosition(c.contract, raw, eventSchema(t), false, validate(t, c))
		})
		if e != nil {
			t.Fatal(e)
		}
		recovered := projection.AttemptInput{GapID: next.GapID, AttemptID: uuid.NewString(), RequestID: uuid.NewString(), PriorAttemptID: next.AttemptID, Action: "recovered", AuthorityRef: "synthetic-current-history", Recovery: rec}
		if e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.AppendAttempt(context.Background(), recovered, authorizeAttempt)
			return e
		}); e != nil {
			t.Fatal("narrowed recovered manifest", e)
		}
		for _, raw := range bodies {
			if _, e := consume(t, f.db, c, raw, false, func() error { return nil }); e != nil {
				t.Fatal(e)
			}
		}
		if e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.AppendAttempt(context.Background(), recovered, authorizeAttempt)
			return e
		}); e != nil {
			t.Fatal("immutable recovery receipt after progress", e)
		}
		hold := projection.AttemptInput{GapID: next.GapID, AttemptID: uuid.NewString(), RequestID: uuid.NewString(), PriorAttemptID: recovered.AttemptID, Action: "held", AuthorityRef: "synthetic-current-history", HoldReasonCode: "source-unavailable"}
		if e := run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
			_, e := a.AppendAttempt(context.Background(), hold, authorizeAttempt)
			return e
		}); !errors.Is(e, projection.ErrReconciliationHold) {
			t.Fatal("stale hold installed", e)
		}
	})
	// Switch this owned, empty-outbox fixture only after direct-mode checks.
	f.cutover(t)
	t.Run("custody-exact-duplicate-reconciliation-and-final-job-rollback", func(t *testing.T) { custodyChecks(t, f) })
}

func gapEvidence() projection.GapEvidence {
	return projection.GapEvidence{GapID: uuid.NewString(), RequestID: uuid.NewString(), AttemptID: uuid.NewString(), AttemptRequestID: uuid.NewString(), AdmissionRef: "synthetic-admission", AuthorityRef: "synthetic-recovery-authority"}
}
func allowRecovery(context.Context, projection.RecoveryRequest) error { return nil }

type historySource struct {
	bodies          [][]byte
	badHash, denied bool
}

func (s *historySource) Fetch(context.Context, projection.RecoveryRequest) (projection.HistoryBatch, error) {
	h := projection.HistoryDigest(s.bodies)
	if s.badHash {
		h[0] ^= 1
	}
	return projection.HistoryBatch{Bodies: s.bodies, ManifestRef: "synthetic-authenticated-original-history", ManifestHash: h}, nil
}
func (s *historySource) Verify(context.Context, projection.RecoveryRequest, projection.HistoryBatch) error {
	if s.denied {
		return errors.New("synthetic source denial")
	}
	return nil
}

func custodyChecks(t *testing.T, f *fixture) {
	c := contract(t)
	install(t, f, c, bootstrap(t, c, 0, true))
	one := message(t, c, 1)
	two := message(t, c, 2)
	applyCustody := func(body []byte, finalFail bool, verify func(context.Context, port, projection.DuplicateCoverage) error) error {
		p := position(t, c, body, false)
		return run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], tx *gorm.DB) error {
			i, e := inbox.NewTransactionalConsumer(tx, c.input.Consumer, func(*gorm.DB) (*projection.Checkpoints[port], error) { return a, nil }, validate(t, c))
			if e != nil {
				return e
			}
			candidate, e := i.Apply(context.Background(), body, func(a *projection.Checkpoints[port], _ events.Envelope) error {
				n, e := a.CheckNext(context.Background(), p, authorizePosition)
				if e != nil {
					return e
				}
				return a.Advance(context.Background(), n, apply)
			})
			if e != nil {
				return e
			}
			if candidate == inbox.DuplicateCandidate {
				if e := a.VerifyDuplicate(context.Background(), p, authorizePosition, verify); e != nil {
					return e
				}
			}
			// Synthetic final lease/completion failure, not T923 implementation.
			if finalFail {
				return errors.New("synthetic final job lease lost")
			}
			return nil
		})
	}
	if e := applyCustody(one, true, coverage); e == nil {
		t.Fatal("lost lease accepted")
	}
	if progress(t, f, c) != 0 || count(t, f, "eventstore.inbox", c) != 0 {
		t.Fatal("lease failure partial commit")
	}
	if e := applyCustody(one, false, nil); e != nil {
		t.Fatal(e)
	}
	if e := applyCustody(one, false, nil); e != nil {
		t.Fatal("exact duplicate rejected", e)
	}
	if e := applyCustody(two, false, nil); e != nil {
		t.Fatal(e)
	}
	if e := applyCustody(one, false, nil); !errors.Is(e, projection.ErrReconciliationHold) {
		t.Fatal("older duplicate without retained coverage", e)
	}
	if e := applyCustody(one, false, func(context.Context, port, projection.DuplicateCoverage) error {
		return projection.ErrReconciliationHold
	}); !errors.Is(e, projection.ErrReconciliationHold) {
		t.Fatal(e)
	}
	if e := applyCustody(one, false, coverage); e != nil {
		t.Fatal("retained coverage", e)
	}
	// A legacy inbox alone cannot establish a missing/behind checkpoint.
	d := contract(t)
	body := message(t, d, 1)
	p := position(t, d, body, false)
	h := p.Hash()
	if e := f.db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SET LOCAL justix.messaging_mode='custody'").Error; e != nil {
			return e
		}
		return tx.Exec("INSERT INTO eventstore.inbox(consumer_name,event_id,envelope_hash) VALUES(?,?,?)", d.input.Consumer, p.Envelope().EventID(), h[:]).Error
	}); e != nil {
		t.Fatal(e)
	}
	if e := run(t, f.db, d, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
		return a.VerifyDuplicate(context.Background(), p, authorizePosition, coverage)
	}); !errors.Is(e, projection.ErrReconciliationHold) {
		t.Fatal("missing checkpoint accepted", e)
	}
	install(t, f, d, bootstrap(t, d, 0, true))
	if e := run(t, f.db, d, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
		return a.VerifyDuplicate(context.Background(), p, authorizePosition, coverage)
	}); !errors.Is(e, projection.ErrReconciliationHold) {
		t.Fatal("behind checkpoint accepted", e)
	}
	// Byte-conflicting duplicate must fail even with the same decoded envelope.
	changed := append([]byte(" "), one...)
	if e := applyCustody(changed, false, coverage); !errors.Is(e, inbox.ErrEnvelopeConflict) {
		t.Fatal("hash conflict", e)
	}
}

type commitFaultPool struct {
	*sql.DB
	commit bool
}

func (p commitFaultPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, e := p.DB.BeginTx(ctx, opts)
	if e != nil {
		return nil, e
	}
	return &commitFaultTx{tx, p.commit}, nil
}

type commitFaultTx struct {
	*sql.Tx
	commit bool
}

func (t commitFaultTx) Commit() error {
	var e error
	if t.commit {
		e = t.Tx.Commit()
	} else {
		e = t.Tx.Rollback()
	}
	if e != nil {
		return e
	}
	return io.ErrUnexpectedEOF
}

func TestFixtureCleanupAfterSetupFailure(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_PROJECTION_SEQUENCE") != "1" {
		t.Skip("opt-in own PostgreSQL fixture")
	}
	if os.Getenv("JUSTIXAUTO_T014_FIXTURE_FAILURE_CHILD") == "1" {
		newFixture(t)
		t.Fatal("failure injection did not fire")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFixtureCleanupAfterSetupFailure$", "-test.v")
	cmd.Env = append(os.Environ(), "JUSTIXAUTO_T014_FIXTURE_FAILURE_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "synthetic setup failure after owned volume capture") {
		t.Fatalf("expected setup failure: %v %s", err, out)
	}
	if strings.Contains(string(out), "absence not verified") || strings.Contains(string(out), "remove own fixture:") {
		t.Fatalf("child cleanup failed: %s", out)
	}
	var container, volume string
	for line := range strings.SplitSeq(string(out), "\n") {
		if _, value, ok := strings.Cut(line, "owned fixture container: "); ok {
			container = strings.TrimSpace(value)
		}
		if _, value, ok := strings.Cut(line, "owned fixture anonymous volume: "); ok {
			volume = strings.TrimSpace(value)
		}
	}
	if container == "" || volume == "" {
		t.Fatalf("missing owned IDs: %s", out)
	}
	if out, err := exec.CommandContext(ctx, "docker", "inspect", container).CombinedOutput(); err == nil || !strings.Contains(strings.ToLower(string(out)), "no such object:") {
		t.Fatalf("failed setup container retained: %v %s", err, out)
	}
	if out, err := exec.CommandContext(ctx, "docker", "volume", "inspect", volume).CombinedOutput(); err == nil || !strings.Contains(string(out), "no such volume") {
		t.Fatalf("failed setup volume retained: %v %s", err, out)
	}
	t.Logf("failed setup cleanup verified: container=%s anonymous-volume=%s", container, volume)
}

// All fixture resources are new, synthetic, private and owned by this test.
// No existing DSN, host port, volume, reference credential or broker is used.
type fixture struct {
	db                        *gorm.DB
	container, password, port string
	query                     func(string, ...string) (string, error)
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{container: "justixauto-t014-" + uuid.NewString(), password: "synthetic-" + uuid.NewString()}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	image := "docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"
	// Register cleanup before creation/startup so failed setup follows the same
	// ownership-limited removal path. No named or preexisting volume is mounted.
	var volumes []string
	inspectVolumes := func(ctx context.Context) ([]string, error) {
		out, err := exec.CommandContext(ctx, "docker", "inspect", "--format", `{{range .Mounts}}{{if eq .Type "volume"}}{{println .Name}}{{end}}{{end}}`, f.container).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("inspect own mounts: %w %s", err, out)
		}
		return strings.Fields(string(out)), nil
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if volumes == nil {
			var err error
			volumes, err = inspectVolumes(ctx)
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "no such object:") {
					return
				}
				t.Error(err)
			}
		}
		if out, e := exec.CommandContext(ctx, "docker", "rm", "-fv", f.container).CombinedOutput(); e != nil {
			t.Errorf("remove own fixture: %v %s", e, out)
		}
		if out, err := exec.CommandContext(ctx, "docker", "inspect", f.container).CombinedOutput(); err == nil || !strings.Contains(strings.ToLower(string(out)), "no such object:") {
			t.Errorf("own container absence not verified: %v %s", err, out)
		}
		for _, volume := range volumes {
			if out, err := exec.CommandContext(ctx, "docker", "volume", "inspect", volume).CombinedOutput(); err == nil || !strings.Contains(string(out), "no such volume") {
				t.Errorf("own anonymous volume absence not verified: %s %v %s", volume, err, out)
			} else {
				t.Logf("removed own anonymous volume: %s", volume)
			}
		}
	})
	if out, e := exec.CommandContext(ctx, "docker", "create", "--name", f.container, "-e", "POSTGRES_PASSWORD="+f.password, "-e", "POSTGRES_DB=justix_inventory", "-p", "127.0.0.1::5432", image).CombinedOutput(); e != nil {
		t.Fatalf("create own fixture: %v %s", e, out)
	}
	var err error
	volumes, err = inspectVolumes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(volumes) != 1 {
		t.Fatalf("expected pinned image's one anonymous volume, got %v", volumes)
	}
	t.Logf("owned fixture container: %s", f.container)
	for _, volume := range volumes {
		t.Logf("owned fixture anonymous volume: %s", volume)
	}
	if os.Getenv("JUSTIXAUTO_T014_FIXTURE_FAILURE_CHILD") == "1" {
		t.Fatal("synthetic setup failure after owned volume capture")
	}
	if out, e := exec.CommandContext(ctx, "docker", "start", f.container).CombinedOutput(); e != nil {
		t.Fatalf("start own fixture: %v %s", e, out)
	}
	f.query = func(statement string, args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", append([]string{"exec", "-i", f.container, "psql", "-XqAt", "-U", "postgres", "-d", "justix_inventory", "-v", "ON_ERROR_STOP=1"}, args...)...)
		cmd.Stdin = strings.NewReader(statement)
		out, e := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), e
	}
	for {
		if out, e := f.query("SHOW server_version_num"); e == nil {
			if out != "180006" {
				t.Fatal(out)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("fixture startup timeout")
		case <-time.After(100 * time.Millisecond):
		}
	}
	f.must(t, "CREATE ROLE justix_inventory_runtime LOGIN PASSWORD '"+f.password+"'; REVOKE TEMP ON DATABASE justix_inventory FROM PUBLIC;")
	files := []string{"../eventstore/schema.sql", "../eventstore/migrations/000002_messaging_delivery.up.sql", "../eventstore/migrations/000003_messaging_route_compatibility.up.sql", "../eventstore/migrations/000004_projection_checkpoint.up.sql"}
	hashes := []string{"86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53", "1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2", "c198af498e39056a7a251b5906d6db050ad994e04588bf6cecf8316221ea85ec", "41536429fb95d4b60a11866843a2ccbd6a4cf723cd919884933b6bc1d8202c4c"}
	args := []string{"-v", "owner_service=inventory", "-v", "runtime_role=justix_inventory_runtime", "-v", "base_schema_sha256=" + hashes[0], "-v", "prior_migration_sha256=" + hashes[1], "-v", "correction_migration_sha256=" + hashes[2], "-v", "checkpoint_migration_sha256=" + hashes[3], "-v", "backup_ref=synthetic-new-empty-database", "-v", "stopped_runtimes_ref=synthetic-no-runtimes", "-v", "compatibility_ref=synthetic-T014-checks"}
	for i, path := range files {
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != hashes[i] {
			t.Fatal("migration artifact changed", path)
		}
		if out, e := f.query(string(b), args...); e != nil {
			t.Fatalf("install %s: %v %s", path, e, out)
		}
	}
	f.must(t, `CREATE SCHEMA fixture;
 CREATE TABLE fixture.effects(consumer text NOT NULL,event_id uuid NOT NULL,stream uuid NOT NULL,position bigint NOT NULL,hash bytea NOT NULL,PRIMARY KEY(consumer,event_id));
 CREATE TABLE fixture.snapshots(id uuid PRIMARY KEY);
 GRANT USAGE ON SCHEMA fixture TO justix_inventory_runtime;
 GRANT SELECT,INSERT ON fixture.effects,fixture.snapshots TO justix_inventory_runtime;`)
	out, e := exec.CommandContext(ctx, "docker", "port", f.container, "5432/tcp").Output()
	if e != nil {
		t.Fatal(e)
	}
	address := strings.TrimSpace(string(out))
	if !strings.HasPrefix(address, "127.0.0.1:") || strings.Contains(address, "\n") {
		t.Fatal(address)
	}
	f.port = strings.TrimPrefix(address, "127.0.0.1:")
	dsn := fmt.Sprintf("host=127.0.0.1 port=%s user=justix_inventory_runtime password=%s dbname=justix_inventory sslmode=disable", f.port, f.password)
	f.db, e = gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	pool, e := f.db.DB()
	if e != nil {
		t.Fatal(e)
	}
	pool.SetMaxOpenConns(12)
	t.Cleanup(func() {
		if e := pool.Close(); e != nil {
			t.Error(e)
		}
	})
	t.Log("owned PG 180006; exact T008/000002/000003/000004 artifacts; narrow runtime; random loopback")
	return f
}
func (f *fixture) must(t *testing.T, q string) {
	t.Helper()
	if out, e := f.query(q); e != nil {
		t.Fatalf("fixture SQL: %v %s", e, out)
	}
}
func (f *fixture) cutover(t *testing.T) {
	t.Helper()
	f.must(t, fmt.Sprintf("SELECT eventstore.activate_messaging_custody('%s',sha256('synthetic-marker'::bytea),decode('1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2','hex'),'synthetic-empty-outbox','synthetic-no-runtimes','synthetic-existing-checkpoints','synthetic-compatibility')", uuid.NewString()))
}
