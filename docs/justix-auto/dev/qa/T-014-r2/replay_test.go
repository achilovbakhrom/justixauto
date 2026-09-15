package projection_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"justixauto/pkg/eventstore"
	"justixauto/pkg/projection"
)

// Independent QA tests use only public adapter operations and the committed
// synthetic fixture. No immutable SQL guard is disabled to forge bad state.
func TestIndependentT014R2Receipts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	changes := []struct {
		name string
		mutate func(*projection.ContractInput)
	}{
		{"kind", func(v *projection.ContractInput) { v.Kind = "process" }},
		{"generation", func(v *projection.ContractInput) { v.Generation += " " }},
		{"contract-id", func(v *projection.ContractInput) { v.ContractID += "-new" }},
		{"contract-version", func(v *projection.ContractInput) { v.ContractVersion += 2 }},
		{"contract-digest", func(v *projection.ContractInput) { v.ContractDigest[31] ^= 128 }},
	}
	for _, change := range changes {
		t.Run(change.name, func(t *testing.T) {
			c := contract(t)
			install(t, f, c, bootstrap(t, c, 5, false))
			blocked := message(t, c, 8)
			_, err := consume(t, f.db, c, blocked, false, func() error { t.Error("blocked event ACK"); return nil })
			var gap *projection.Gap
			if !errors.As(err, &gap) { t.Fatalf("expected gap: %v", err) }
			ev := gapEvidence()
			record := func() error { return run(t, f.db, c, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
				_, err := a.RecordGap(ctx, gap, ev, authorizeGap); return err
			}) }
			if err := record(); err != nil { t.Fatal(err) }
			if _, err := projection.ReadRecovery(ctx, f.db, c.contract, ev.GapID, ev.AttemptID, ev.AttemptRequestID, allowRecovery); err != nil { t.Fatalf("nonzero bootstrap recovery: %v", err) }
			wrong := c
			change.mutate(&wrong.input)
			wrong.contract, err = projection.NewContract(wrong.input)
			if err != nil { t.Fatal(err) }
			if _, err := projection.ReadRecovery(ctx, f.db, wrong.contract, ev.GapID, ev.AttemptID, ev.AttemptRequestID, allowRecovery); !errors.Is(err, projection.ErrReconciliationHold) { t.Fatalf("wrong recovery identity: %v", err) }
			in := projection.AttemptInput{GapID: ev.GapID, AttemptID: uuid.NewString(), RequestID: uuid.NewString(), PriorAttemptID: ev.AttemptID, Action: "held", AuthorityRef: "qa-current-authority", HoldReasonCode: "source-unavailable"}
			appendWith := func(db *gorm.DB, consumer fixtureContract, input projection.AttemptInput) (projection.GapRecord, error) {
				var result projection.GapRecord
				err := run(t, db, consumer, eventstore.SharedFence, func(a *projection.Checkpoints[port], _ *gorm.DB) error {
					var err error; result, err = a.AppendAttempt(ctx, input, authorizeAttempt); return err
				})
				return result, err
			}
			// Compare every retained field, including timestamps and historical
			// resulting_position, rather than accepting only an unchanged count.
			history := func() string {
				var value string
				if err := f.db.Raw("SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY attempt_id)::text,'[]') FROM eventstore.consumer_gap_attempts a WHERE gap_id=?", ev.GapID).Scan(&value).Error; err != nil { t.Fatal(err) }
				return value
			}
			assertWrong := func(phase string) {
				t.Helper(); before := history()
				if _, err := appendWith(f.db, wrong, in); !errors.Is(err, projection.ErrReconciliationHold) { t.Fatalf("%s wrong identity: %v", phase, err) }
				if after := history(); before != after { t.Fatalf("%s changed retained evidence", phase) }
				t.Logf("%s: wrong %s rejected; every retained attempt field unchanged", phase, change.name)
			}
			assertWrong("new-request")
			pool, err := f.db.DB(); if err != nil { t.Fatal(err) }
			fault := f.db.Session(&gorm.Session{NewDB:true})
			fault.Statement = &gorm.Statement{DB:fault, ConnPool:commitFaultPool{pool,true}}
			if _, err := appendWith(fault, c, in); !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) { t.Fatalf("actual commit with lost reply: %v", err) }
			retained := history()
			assertReplay := func(phase string) {
				t.Helper()
				got, err := appendWith(f.db, c, in)
				if err != nil || got.GapID != in.GapID || got.AttemptID != in.AttemptID || got.Resolved { t.Fatalf("%s exact receipt: %+v %v", phase, got, err) }
				if history() != retained { t.Fatalf("%s rewrote historical receipt", phase) }
			}
			assertReplay("lost-commit-reconciliation")
			assertWrong("exact-replay-before-progress")
			for seq := int64(6); seq <= 8; seq++ {
				raw := blocked; if seq != 8 { raw = message(t, c, seq) }
				if _, err := consume(t, f.db, c, raw, false, func() error { return nil }); err != nil { t.Fatalf("ordinary progress %d: %v", seq, err) }
				assertWrong(fmt.Sprintf("exact-replay-at-%d", seq))
				assertReplay(fmt.Sprintf("exact-replay-at-%d", seq))
			}
			if progress(t, f, c) != 8 { t.Fatal("progress changed by recovery receipt") }
			if err := record(); err != nil { t.Fatalf("original gap receipt after progress: %v", err) }
			if history() != retained { t.Fatal("gap receipt replay changed retained history") }
			changed := in; changed.AuthorityRef += "-changed"
			if _, err := appendWith(f.db, c, changed); !errors.Is(err, projection.ErrConflict) { t.Fatalf("changed receipt payload: %v", err) }
			if history() != retained { t.Fatal("conflicting receipt changed history") }
			if _, err := projection.ReadRecovery(ctx, f.db, c.contract, ev.GapID, ev.AttemptID, ev.AttemptRequestID, allowRecovery); !errors.Is(err, projection.ErrReconciliationHold) { t.Fatalf("obsolete requested tip accepted: %v", err) }
			t.Log("nonzero bootstrap 5, lost-commit receipt, unchanged replay through position 8, changed payload conflict and obsolete recovery hold verified")
		})
	}
}
