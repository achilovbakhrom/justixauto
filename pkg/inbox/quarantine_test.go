package inbox_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
	"justixauto/pkg/inbox"
)

type fixtureSecurity struct {
	mu      sync.Mutex
	values  map[string][]byte
	sealErr error
	openErr error
}

func newFixtureSecurity() *fixtureSecurity { return &fixtureSecurity{values: make(map[string][]byte)} }

func (s *fixtureSecurity) Seal(_ context.Context, in inbox.SealRequest) (inbox.SealedEvidence, error) {
	if s.sealErr != nil {
		return inbox.SealedEvidence{}, s.sealErr
	}
	if sha256.Sum256(in.Raw) != in.RawHash || int64(len(in.Raw)) != in.RawByteLength || in.ContextDigest == ([32]byte{}) {
		return inbox.SealedEvidence{}, errors.New("fixture seal binding mismatch")
	}
	cipher := []byte("synthetic-sealed-reference:" + uuid.NewString())
	s.mu.Lock()
	s.values[string(cipher)] = bytes.Clone(in.Raw)
	s.mu.Unlock()
	return inbox.SealedEvidence{Ciphertext: cipher, FormatRef: "synthetic-format-v1", KeyRef: "synthetic-key-ref", CapturePolicyRef: "synthetic-policy-ref"}, nil
}

func (s *fixtureSecurity) Restore(_ context.Context, sealed inbox.SealedEvidence) ([]byte, error) {
	if s.openErr != nil {
		return nil, s.openErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.values[string(sealed.Ciphertext)]
	if !ok {
		return nil, errors.New("fixture seal not found")
	}
	return bytes.Clone(v), nil
}

func captureInput(stage inbox.QuarantineStage) inbox.CaptureInput {
	return inbox.CaptureInput{
		EvidenceID: uuid.NewString(), RequestID: uuid.NewString(), HoldActionID: uuid.NewString(), HoldRequestID: uuid.NewString(),
		Stage: stage, QueueRef: "synthetic-owner-queue", ConsumerName: "synthetic-consumer", ReasonCode: "invalid-envelope",
		ActorRef: "synthetic-system-actor", AuthorityRef: "synthetic-quarantine-authority", ScopeRef: "synthetic-owner-scope",
		PurposeRef: "synthetic-quarantine-purpose", RepairRef: "synthetic-unrepaired", ManifestRef: "synthetic-capture-manifest",
		ManifestHash: sha256.Sum256([]byte("synthetic-capture-manifest")),
	}
}

func installQuarantineAdapterSchema(t *testing.T) (*gorm.DB, func(string, ...string) (string, error)) {
	t.Helper()
	db, ownerSQL := databaseWithMessaging(t, true)
	if out, err := ownerSQL("REVOKE CREATE,TEMP ON DATABASE justix_inventory FROM PUBLIC"); err != nil {
		t.Fatalf("seal fixture database privileges: %v %s", err, out)
	}
	type migration struct {
		path string
		args []string
	}
	const base = "86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53"
	const prior = "1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2"
	const correction = "c198af498e39056a7a251b5906d6db050ad994e04588bf6cecf8316221ea85ec"
	const checkpoint = "41536429fb95d4b60a11866843a2ccbd6a4cf723cd919884933b6bc1d8202c4c"
	const quarantine = "1c1e7d17efa8ff84f2469d6d55b3b6c810143e5260aabf5f82d748797f2296a3"
	common := []string{"-v", "owner_service=inventory", "-v", "runtime_role=justix_inventory_runtime", "-v", "backup_ref=synthetic-backup", "-v", "stopped_runtimes_ref=synthetic-stopped-runtimes", "-v", "compatibility_ref=synthetic-compatibility"}
	migrations := []migration{
		{"../eventstore/migrations/000003_messaging_route_compatibility.up.sql", append(append([]string{}, common...), "-v", "prior_migration_sha256="+prior, "-v", "correction_migration_sha256="+correction)},
		{"../eventstore/migrations/000004_projection_checkpoint.up.sql", append(append([]string{}, common...), "-v", "base_schema_sha256="+base, "-v", "prior_migration_sha256="+prior, "-v", "correction_migration_sha256="+correction, "-v", "checkpoint_migration_sha256="+checkpoint)},
		{"../eventstore/migrations/000005_quarantine_evidence.up.sql", append(append([]string{}, common...), "-v", "base_schema_sha256="+base, "-v", "prior_migration_sha256="+prior, "-v", "correction_migration_sha256="+correction, "-v", "checkpoint_migration_sha256="+checkpoint, "-v", "quarantine_migration_sha256="+quarantine)},
	}
	for _, m := range migrations {
		body, err := os.ReadFile(m.path)
		if err != nil {
			t.Fatal(err)
		}
		if out, err := ownerSQL(string(body), m.args...); err != nil {
			t.Fatalf("install %s: %v %s", m.path, err, out)
		}
	}
	return db, ownerSQL
}

func TestQuarantineValidationAndSealFailure(t *testing.T) {
	security := newFixtureSecurity()
	if _, err := inbox.NewQuarantine(nil, events.OwnerInventory, security); err == nil {
		t.Fatal("nil database")
	}
	if _, err := inbox.NewQuarantine(&gorm.DB{}, "foreign", security); !errors.Is(err, inbox.ErrInvalidQuarantine) {
		t.Fatal(err)
	}
	q, err := inbox.NewQuarantine(&gorm.DB{}, events.OwnerInventory, security)
	if err != nil {
		t.Fatal(err)
	}
	in := captureInput(inbox.QuarantineDirectHandler)
	in.ReasonCode = "unsafe body"
	if _, err := q.Capture(context.Background(), in, []byte("secret")); !errors.Is(err, inbox.ErrInvalidQuarantine) {
		t.Fatal(err)
	}
	security.sealErr = errors.New("synthetic key unavailable")
	in = captureInput(inbox.QuarantineDirectHandler)
	acked := false
	if _, err := q.CaptureAndAcknowledge(context.Background(), in, []byte("secret"), func() error { acked = true; return nil }); !errors.Is(err, security.sealErr) || acked {
		t.Fatal(err, acked)
	}
	in.Stage = inbox.QuarantineCustodyHandler
	if _, err := q.CaptureAndAcknowledge(context.Background(), in, nil, func() error { return nil }); !errors.Is(err, inbox.ErrInvalidQuarantine) {
		t.Fatal(err)
	}
}

func TestQuarantinePostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_QUARANTINE_ADAPTER") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_QUARANTINE_ADAPTER=1 for disposable PostgreSQL quarantine adapter tests")
	}
	db, _ := installQuarantineAdapterSchema(t)
	ctx := context.Background()
	security := newFixtureSecurity()
	q, err := inbox.NewQuarantine(db, events.OwnerInventory, security)
	if err != nil {
		t.Fatal(err)
	}
	count := func(table, where string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := db.Raw("SELECT count(*) FROM "+table+" WHERE "+where, args...).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("arbitrary bytes commit before one-delivery acknowledgment", func(t *testing.T) {
		in := captureInput(inbox.QuarantineDirectHandler)
		raw := []byte("not-json synthetic-private-body")
		acks := 0
		receipt, err := q.CaptureAndAcknowledge(ctx, in, raw, func() error {
			acks++
			if count("eventstore.quarantine_evidence", "capture_request_id=?::uuid", in.RequestID) != 1 || count("eventstore.quarantine_actions", "request_id=?::uuid", in.HoldRequestID) != 1 {
				t.Fatal("ack before durable evidence")
			}
			return nil
		})
		if err != nil || receipt.Duplicate || receipt.EvidenceID != in.EvidenceID || acks != 1 {
			t.Fatal(receipt, err, acks)
		}
		var leaked bool
		if err := db.Raw(`SELECT EXISTS(SELECT FROM eventstore.quarantine_evidence
			WHERE concat_ws('|',queue_ref,consumer_name,reason_code,seal_format_ref,seal_key_ref,capture_policy_ref) LIKE ?)`, "%"+string(raw)+"%").Scan(&leaked).Error; err != nil || leaked {
			t.Fatal("raw body leaked to visible metadata", err)
		}
		if _, err := q.CaptureAndAcknowledge(ctx, in, raw, func() error { acks++; return nil }); err != nil || acks != 2 {
			t.Fatal(err, acks)
		}
		changed := append(bytes.Clone(raw), '!')
		if _, err := q.CaptureAndAcknowledge(ctx, in, changed, func() error { acks++; return nil }); !errors.Is(err, inbox.ErrQuarantineConflict) || acks != 2 {
			t.Fatal(err, acks)
		}
	})

	t.Run("ack failure retains committed evidence", func(t *testing.T) {
		in := captureInput(inbox.QuarantineIntake)
		ackErr := errors.New("synthetic channel lost")
		r, err := q.CaptureAndAcknowledge(ctx, in, nil, func() error { return ackErr })
		if r.EvidenceID != in.EvidenceID || !errors.Is(err, inbox.ErrQuarantineAcknowledgment) || !errors.Is(err, ackErr) {
			t.Fatal(r, err)
		}
		if count("eventstore.quarantine_evidence", "evidence_id=?::uuid", in.EvidenceID) != 1 {
			t.Fatal("committed evidence missing after ack failure")
		}
	})

	t.Run("concurrent exact request converges", func(t *testing.T) {
		in := captureInput(inbox.QuarantineDirectHandler)
		raw := []byte{0, 1, 2, 255}
		start := make(chan struct{})
		errs := make(chan error, 8)
		var acks atomic.Int32
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				<-start
				_, err := q.CaptureAndAcknowledge(ctx, in, raw, func() error { acks.Add(1); return nil })
				errs <- err
			})
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if acks.Load() != 8 || count("eventstore.quarantine_evidence", "capture_request_id=?::uuid", in.RequestID) != 1 || count("eventstore.quarantine_actions", "evidence_id=?::uuid", in.EvidenceID) != 1 {
			t.Fatal(acks.Load())
		}
	})

	t.Run("lost commit reply never acknowledges and exact retry reconciles", func(t *testing.T) {
		for _, commitFirst := range []bool{false, true} {
			in := captureInput(inbox.QuarantineDirectHandler)
			raw := []byte(fmt.Sprintf("unknown-%v", commitFirst))
			pool, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			fault, err := gorm.Open(postgres.New(postgres.Config{Conn: commitFaultPool{DB: pool, commitFirst: commitFirst}}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			fq, err := inbox.NewQuarantine(fault, events.OwnerInventory, security)
			if err != nil {
				t.Fatal(err)
			}
			acks := 0
			if _, err := fq.CaptureAndAcknowledge(ctx, in, raw, func() error { acks++; return nil }); !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) || !errors.Is(err, io.ErrUnexpectedEOF) || acks != 0 {
				t.Fatal(err, acks)
			}
			r, err := q.CaptureAndAcknowledge(ctx, in, raw, func() error { acks++; return nil })
			if err != nil || acks != 1 || r.Duplicate != commitFirst {
				t.Fatal(r, err, acks)
			}
		}
	})
}
