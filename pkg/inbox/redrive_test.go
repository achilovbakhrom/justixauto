package inbox_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"justixauto/pkg/events"
	"justixauto/pkg/inbox"
)

type fixtureAuthorizer struct {
	err   atomic.Pointer[error]
	calls atomic.Int32
}

func (a *fixtureAuthorizer) Authorize(_ context.Context, in inbox.RedriveAuthorization) error {
	a.calls.Add(1)
	if in.EvidenceID == "" || in.OwnerService != "inventory" || in.AuthorityRef == "" || in.PurposeRef == "" {
		return errors.New("invalid fixture authorization context")
	}
	if p := a.err.Load(); p != nil {
		return *p
	}
	return nil
}

func (a *fixtureAuthorizer) deny(err error) { a.err.Store(&err) }

func redriveRequest(capture inbox.CaptureInput) inbox.RedriveRequestInput {
	consumer := capture.ConsumerName
	return inbox.RedriveRequestInput{
		EvidenceID: capture.EvidenceID, ActionID: uuid.NewString(), RequestID: uuid.NewString(), PriorActionID: capture.HoldActionID,
		ActorRef: "synthetic-repair-actor", AuthorityRef: "synthetic-current-authority", ScopeRef: "synthetic-repair-scope",
		PurposeRef: "synthetic-redrive-purpose", RepairRef: "synthetic-approved-repair", ManifestRef: "synthetic-redrive-manifest",
		ManifestHash: sha256.Sum256([]byte("synthetic-redrive-manifest")), IntendedPath: capture.Stage, IntendedConsumer: consumer,
	}
}

func resultInput() inbox.RedriveResultInput {
	return inbox.RedriveResultInput{ActionID: uuid.NewString(), RequestID: uuid.NewString()}
}

func TestRedrivePostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_QUARANTINE_ADAPTER") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_QUARANTINE_ADAPTER=1 for disposable PostgreSQL redrive tests")
	}
	db, ownerSQL := installQuarantineAdapterSchema(t)
	ctx := context.Background()
	security := newFixtureSecurity()
	q, err := inbox.NewQuarantine(db, events.OwnerInventory, security)
	if err != nil {
		t.Fatal(err)
	}
	authorizer := &fixtureAuthorizer{}
	r, err := inbox.NewRedriver(db, events.OwnerInventory, security, authorizer)
	if err != nil {
		t.Fatal(err)
	}
	company := uuid.NewString()
	apply := func(u effects, e events.Envelope) error { return u.Apply(ctx, e) }

	t.Run("request replay ordinary dedup and retained accepted result", func(t *testing.T) {
		name, id, stream := "redrive-direct", uuid.NewString(), uuid.NewString()
		body := message(t, id, stream, company, 1)
		capture := captureInput(inbox.QuarantineDirectHandler)
		capture.ConsumerName = name
		if _, err := q.Capture(ctx, capture, body); err != nil {
			t.Fatal(err)
		}
		request := redriveRequest(capture)
		ticket, err := r.Request(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Request(ctx, request); err != nil {
			t.Fatal("exact request replay", err)
		}
		changed := request
		changed.RepairRef = "synthetic-different-repair"
		if _, err := r.Request(ctx, changed); !errors.Is(err, inbox.ErrRedriveConflict) {
			t.Fatal("changed request", err)
		}

		consumer := consumer(t, db, name, company)
		var calls atomic.Int32
		path := inbox.OrdinaryRedriveFunc(func(ctx context.Context, p inbox.RedrivePayload) (inbox.OrdinaryReceipt, error) {
			calls.Add(1)
			outcome, err := consumer.Consume(ctx, p.OriginalBytes, apply, func() error { return nil })
			if err != nil || (outcome != inbox.Applied && outcome != inbox.Duplicate) {
				return inbox.OrdinaryReceipt{}, fmt.Errorf("ordinary inbox: %v %w", outcome, err)
			}
			return inbox.OrdinaryReceipt{Kind: inbox.ReceiptInbox, ConsumerName: name, EventID: id, EnvelopeHash: sha256.Sum256(p.OriginalBytes)}, nil
		})
		result := resultInput()
		out, err := r.Execute(ctx, ticket, result, path)
		if err != nil || out.Code != "accepted" || out.Duplicate || calls.Load() != 1 {
			t.Fatal(out, err, calls.Load())
		}
		out, err = r.Execute(ctx, ticket, result, path)
		if err != nil || !out.Duplicate || calls.Load() != 1 {
			t.Fatal("retained result reapplied ordinary path", out, err, calls.Load())
		}
		var effects int64
		if err := db.Raw("SELECT count(*) FROM fixture.effects WHERE consumer_name=? AND event_id=?::uuid", name, id).Scan(&effects).Error; err != nil || effects != 1 {
			t.Fatal(effects, err)
		}
	})

	t.Run("crash after ordinary apply reconciles through inbox receipt", func(t *testing.T) {
		name, id, stream := "redrive-crash", uuid.NewString(), uuid.NewString()
		body := message(t, id, stream, company, 1)
		capture := captureInput(inbox.QuarantineDirectHandler)
		capture.ConsumerName = name
		if _, err := q.Capture(ctx, capture, body); err != nil {
			t.Fatal(err)
		}
		ticket, err := r.Request(ctx, redriveRequest(capture))
		if err != nil {
			t.Fatal(err)
		}
		consumer := consumer(t, db, name, company)
		var calls atomic.Int32
		crash := true
		result := resultInput()
		path := inbox.OrdinaryRedriveFunc(func(ctx context.Context, p inbox.RedrivePayload) (inbox.OrdinaryReceipt, error) {
			calls.Add(1)
			if _, err := consumer.Consume(ctx, p.OriginalBytes, apply, func() error { return nil }); err != nil {
				return inbox.OrdinaryReceipt{}, err
			}
			if crash {
				panic("synthetic crash after ordinary commit")
			}
			return inbox.OrdinaryReceipt{Kind: inbox.ReceiptInbox, ConsumerName: name, EventID: id, EnvelopeHash: sha256.Sum256(p.OriginalBytes)}, nil
		})
		func() {
			defer func() {
				if recover() != "synthetic crash after ordinary commit" {
					t.Error("crash did not propagate")
				}
			}()
			_, _ = r.Execute(ctx, ticket, result, path)
		}()
		crash = false
		out, err := r.Execute(ctx, ticket, result, path)
		if err != nil || out.Code != "accepted" || calls.Load() != 2 {
			t.Fatal(out, err, calls.Load())
		}
		var effects int64
		if err := db.Raw("SELECT count(*) FROM fixture.effects WHERE consumer_name=? AND event_id=?::uuid", name, id).Scan(&effects).Error; err != nil || effects != 1 {
			t.Fatal(effects, err)
		}
	})

	t.Run("revoked current authority and restore failure never enter ordinary path", func(t *testing.T) {
		for _, failure := range []string{"denied", "restore"} {
			localSecurity := newFixtureSecurity()
			localQ, _ := inbox.NewQuarantine(db, events.OwnerInventory, localSecurity)
			localAuth := &fixtureAuthorizer{}
			localR, _ := inbox.NewRedriver(db, events.OwnerInventory, localSecurity, localAuth)
			capture := captureInput(inbox.QuarantineDirectHandler)
			if _, err := localQ.Capture(ctx, capture, []byte("synthetic-invalid")); err != nil {
				t.Fatal(err)
			}
			ticket, err := localR.Request(ctx, redriveRequest(capture))
			if err != nil {
				t.Fatal(err)
			}
			if failure == "denied" {
				localAuth.deny(errors.New("synthetic authority revoked"))
			} else {
				localSecurity.openErr = errors.New("synthetic key unavailable")
			}
			called := false
			out, err := localR.Execute(ctx, ticket, resultInput(), inbox.OrdinaryRedriveFunc(func(context.Context, inbox.RedrivePayload) (inbox.OrdinaryReceipt, error) {
				called = true
				return inbox.OrdinaryReceipt{}, nil
			}))
			if err == nil || called || out.Code != map[string]string{"denied": "denied", "restore": "failed"}[failure] {
				t.Fatal(failure, out, err, called)
			}
		}
	})

	t.Run("custody release keeps job incomplete and preserves other hold", func(t *testing.T) {
		if out, err := ownerSQL(fmt.Sprintf(`SELECT eventstore.activate_messaging_custody('%s',sha256('new'::bytea),sha256('old'::bytea),'synthetic-backup','synthetic-stopped','synthetic-broker-checkpoints','synthetic-compatibility')`, uuid.NewString())); err != nil {
			t.Fatal(out, err)
		}
		name, id, stream, admission, enrollment := "redrive-custody", uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
		lease := uuid.NewString()
		body := message(t, id, stream, company, 1)
		hash := sha256.Sum256(body)
		seed := fmt.Sprintf(`BEGIN;
		INSERT INTO eventstore.messaging_admissions(admission_id,namespace,source_owner,aggregate_type,aggregate_id,subject,action,contract_id,contract_version,contract_digest,schema_allowlist,authority_ref,scope_ref,purpose,bootstrap_ref,start_after,consumer_kind,generation)
		VALUES('%s','local-consumer','inventory','fixture','%s','%s','admit','synthetic-contract',1,sha256('contract'::bytea),'["inventory.fixture.changed.v1"]','synthetic-authority','synthetic-scope','synthetic-purpose','synthetic-bootstrap',0,'projection','generation-1');
		INSERT INTO eventstore.dispatch_messages(event_id,source_owner,aggregate_type,aggregate_id,integration_sequence,envelope,envelope_hash,exchange,routing_key,membership_version,admission_ref,bootstrap_ref,initial_enrollment_id)
		VALUES('%s','inventory','fixture','%s',1,decode('%s','hex'),decode('%s','hex'),'justix.integration.v1','inventory.inventory.inventory.fixture.changed.v1','membership-1','synthetic-admission','synthetic-bootstrap','%s');
		INSERT INTO eventstore.dispatch_enrollments(event_id,enrollment_id,membership_version,cutover_ref,consumers) VALUES('%s','%s','membership-1','synthetic-cutover','[{"consumer_name":"%s","admission_id":"%s"}]');
		INSERT INTO eventstore.dispatch_jobs(consumer_name,event_id,enrollment_id,admission_id,consumer_kind,generation,lease_owner,lease_until,hold_ref) VALUES('%s','%s','%s','%s','projection','generation-1','%s',clock_timestamp()+interval '5 minutes','synthetic-independent-hold'); COMMIT;`,
			admission, stream, name, id, stream, hex.EncodeToString(body), hex.EncodeToString(hash[:]), enrollment, id, enrollment, name, admission, name, id, enrollment, admission, lease)
		if out, err := ownerSQL(seed); err != nil {
			t.Fatal(out, err)
		}
		capture := captureInput(inbox.QuarantineCustodyHandler)
		capture.QueueRef = ""
		capture.ConsumerName, capture.JobEventID, capture.LeaseOwner = name, id, lease
		capture.Stream = &inbox.QuarantineStream{SourceOwner: events.OwnerInventory, AggregateType: "fixture", AggregateID: stream}
		if receipt, err := q.Capture(ctx, capture, body); err != nil || receipt.Observation != "unfinished" {
			t.Fatal(receipt, err)
		}
		request := redriveRequest(capture)
		ticket, err := r.Request(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		foreignRedriver, err := inbox.NewRedriver(db, events.OwnerInventory, security, authorizer)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := foreignRedriver.ReleaseCustody(ctx, ticket, resultInput()); !errors.Is(err, inbox.ErrRedriveConflict) {
			t.Fatal("custody ticket crossed composition", err)
		}
		out, err := r.ReleaseCustody(ctx, ticket, resultInput())
		if err != nil || out.Code != "released" {
			t.Fatal(out, err)
		}
		var state struct {
			Completed              bool
			HoldRef, QuarantineRef *string
		}
		if err := db.Raw(`SELECT completed_at IS NOT NULL AS completed,hold_ref,quarantine_ref FROM eventstore.dispatch_jobs
			WHERE consumer_name=? AND event_id=?::uuid`, name, id).Scan(&state).Error; err != nil {
			t.Fatal(err)
		}
		if state.Completed || state.HoldRef == nil || *state.HoldRef != "synthetic-independent-hold" || state.QuarantineRef != nil {
			t.Fatal(state)
		}
		// A later authorized owner action removes its independent hold and the
		// ordinary job path completes. Retrying the old capture reconciles its
		// retained unfinished observation without requiring the expired lease;
		// a distinct later capture records the actual completed observation and
		// cannot reopen or quarantine the completed job.
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		if err := tx.Exec("SET LOCAL justix.messaging_mode='custody'").Error; err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Exec(`INSERT INTO eventstore.inbox(consumer_name,event_id,envelope_hash) VALUES(?,?::uuid,?)`, name, id, hash[:]).Error; err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Exec(`UPDATE eventstore.dispatch_jobs SET completed_at=clock_timestamp(),
			inbox_consumer_name=consumer_name,inbox_event_id=event_id,hold_ref=NULL
			WHERE consumer_name=? AND event_id=?::uuid`, name, id).Error; err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit().Error; err != nil {
			t.Fatal(err)
		}
		if receipt, err := q.Capture(ctx, capture, body); err != nil || !receipt.Duplicate || receipt.Observation != "unfinished" {
			t.Fatal("old request reconciliation", receipt, err)
		}
		completedCapture := captureInput(inbox.QuarantineCustodyHandler)
		completedCapture.QueueRef = ""
		completedCapture.ConsumerName, completedCapture.JobEventID, completedCapture.LeaseOwner = name, id, uuid.NewString()
		completedCapture.Stream = capture.Stream
		if receipt, err := q.Capture(ctx, completedCapture, body); err != nil || receipt.Duplicate || receipt.Observation != "completed" {
			t.Fatal("completed observation", receipt, err)
		}
		if err := db.Raw(`SELECT completed_at IS NOT NULL AS completed,quarantine_ref FROM eventstore.dispatch_jobs
			WHERE consumer_name=? AND event_id=?::uuid`, name, id).Scan(&state).Error; err != nil || !state.Completed || state.QuarantineRef != nil {
			t.Fatal("completed job changed by observation", state, err)
		}
	})
}

func TestRedriveTicketCannotCrossOwnerStore(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_QUARANTINE_ADAPTER") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_QUARANTINE_ADAPTER=1 for disposable PostgreSQL redrive isolation tests")
	}
	ctx := context.Background()
	dbA, _ := installQuarantineAdapterSchema(t)
	dbB, _ := installQuarantineAdapterSchema(t)
	security := newFixtureSecurity()
	capture := captureInput(inbox.QuarantineDirectHandler)
	capture.ConsumerName = "redrive-store-isolation"
	qA, err := inbox.NewQuarantine(dbA, events.OwnerInventory, security)
	if err != nil {
		t.Fatal(err)
	}
	qB, err := inbox.NewQuarantine(dbB, events.OwnerInventory, security)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := qA.Capture(ctx, capture, []byte("store-a-protected-input")); err != nil {
		t.Fatal(err)
	}
	if _, err := qB.Capture(ctx, capture, []byte("store-b-different-input")); err != nil {
		t.Fatal(err)
	}
	authorizer := &fixtureAuthorizer{}
	rA, err := inbox.NewRedriver(dbA, events.OwnerInventory, security, authorizer)
	if err != nil {
		t.Fatal(err)
	}
	rB, err := inbox.NewRedriver(dbB, events.OwnerInventory, security, authorizer)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := rA.Request(ctx, redriveRequest(capture))
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	_, err = rB.Execute(ctx, ticket, resultInput(), inbox.OrdinaryRedriveFunc(func(context.Context, inbox.RedrivePayload) (inbox.OrdinaryReceipt, error) {
		calls.Add(1)
		return inbox.OrdinaryReceipt{}, errors.New("foreign ordinary path must not run")
	}))
	if !errors.Is(err, inbox.ErrRedriveConflict) || calls.Load() != 0 {
		t.Fatal("foreign-store ticket crossed pre-effect boundary", err, calls.Load())
	}
}
