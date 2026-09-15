package eventstore_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"justixauto/pkg/eventstore"
)

const quarantineCheckpointHash = "41536429fb95d4b60a11866843a2ccbd6a4cf723cd919884933b6bc1d8202c4c"

// Fixture storage is synthetic and nonsecret. No production key, crypto,
// retention policy, existing DSN, broker or reference infrastructure is used.
// tmpfs covers PostgreSQL's image volume destination; cleanup removes only this
// UUID-named container and verifies its absence. There is no anonymous volume.
func newQuarantineFixture(t *testing.T) *routeFixture {
	t.Helper()
	f := &routeFixture{t: t, container: "justixauto-t927-" + uuid.NewString(), password: "synthetic-" + uuid.NewString()}
	var err error
	f.evidence, err = os.MkdirTemp("", "justixauto-t927-evidence-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Recoverable synthetic migration evidence: %s", f.evidence)
	for path, dst := range map[string]*string{"schema.sql": &f.base, "migrations/000002_messaging_delivery.up.sql": &f.prior, "migrations/000003_messaging_route_compatibility.up.sql": &f.correction} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		*dst = string(b)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	image := "docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"
	if out, err := exec.CommandContext(ctx, "docker", "run", "-d", "--name", f.container, "--tmpfs", "/var/lib/postgresql:rw", "-e", "POSTGRES_PASSWORD="+f.password, "-p", "127.0.0.1::5432", image).CombinedOutput(); err != nil {
		t.Fatalf("start fixture: %v %s", err, out)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "docker", "rm", "-f", "-v", f.container).CombinedOutput(); err != nil {
			t.Errorf("remove owned fixture: %v %s", err, out)
		}
		if out, err := exec.CommandContext(ctx, "docker", "inspect", f.container).CombinedOutput(); err == nil || !strings.Contains(strings.ToLower(string(out)), "no such object") {
			t.Errorf("owned fixture not proven absent: %v %s", err, out)
		}
	})
	if out, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{json .Mounts}} {{json .HostConfig.Tmpfs}}", f.container).CombinedOutput(); err != nil || strings.Contains(string(out), `"Type":"volume"`) || !strings.Contains(string(out), `"/var/lib/postgresql":"rw"`) {
		t.Fatalf("unexpected fixture mounts: %v %s", err, out)
	}
	for {
		if out, err := f.sql("postgres", "postgres", "SHOW server_version_num"); err == nil {
			if out != "180006" {
				t.Fatal("unexpected PostgreSQL", out)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("fixture startup timeout")
		case <-time.After(100 * time.Millisecond):
		}
	}
	out, err := exec.CommandContext(ctx, "docker", "port", f.container, "5432/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	address := strings.TrimSpace(string(out))
	if !strings.HasPrefix(address, "127.0.0.1:") || strings.Contains(address, "\n") {
		t.Fatal("unexpected binding", address)
	}
	f.port = strings.TrimPrefix(address, "127.0.0.1:")
	return f
}

func quarantineStore(t *testing.T, f *routeFixture) *routeStore {
	t.Helper()
	s := checkpointStore(t, f)
	mustCheckpoint(t, s)
	return s
}
func quarantineScript(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("migrations/000005_quarantine_evidence.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func installQuarantine(t *testing.T, s *routeStore, overrides ...string) (string, error) {
	t.Helper()
	script := quarantineScript(t)
	args := []string{"-v", "base_schema_sha256=" + checkpointBaseHash, "-v", "checkpoint_migration_sha256=" + quarantineCheckpointHash, "-v", fmt.Sprintf("quarantine_migration_sha256=%x", sha256.Sum256([]byte(script)))}
	return s.install(script, append(args, overrides...)...)
}
func mustQuarantine(t *testing.T, s *routeStore) {
	t.Helper()
	if out, err := installQuarantine(t, s); err != nil {
		t.Fatalf("000005: %v %s", err, out)
	}
}

func quarantineCutover(t *testing.T, s *routeStore, r routeRecord) {
	t.Helper()
	a := uuid.NewString()
	s.must(t, s.runtime, routeAdmission(a, r.Stream, "source-recipient", "inventory", "inventory.fixture.changed.v1"))
	s.must(t, s.migration, fmt.Sprintf("INSERT INTO eventstore.messaging_legacy_authorizations VALUES('%s','inventory','%s',NULL,'synthetic-reviewed-retained-route')", r.ID, a))
	s.cutover(t)
}

type sealedFixture struct {
	id, request, rawHash, cipherHash, context string
	rawLen                                    int
	cipher                                    []byte
}

// This intentionally fake sealing output is test-only opaque content. It is
// neither encryption nor an implementation of the future security port.
func fixtureSeal(raw []byte, trustedContext string) sealedFixture {
	cipher := []byte("synthetic-opaque-seal:" + uuid.NewString())
	return sealedFixture{uuid.NewString(), uuid.NewString(), fmt.Sprintf("%x", sha256.Sum256(raw)), fmt.Sprintf("%x", sha256.Sum256(cipher)), fmt.Sprintf("%x", sha256.Sum256([]byte(trustedContext))), len(raw), cipher}
}
func evidenceSQL(e sealedFixture) string {
	return fmt.Sprintf(`INSERT INTO eventstore.quarantine_evidence(evidence_id,capture_request_id,stage,owner_service,queue_ref,
 raw_sha256,raw_byte_length,sealed_ciphertext,ciphertext_sha256,seal_format_ref,seal_key_ref,capture_policy_ref,reason_code,capture_context_digest)
 VALUES('%s','%s','intake','inventory','synthetic-owner-queue',decode('%s','hex'),%d,decode('%s','hex'),decode('%s','hex'),
 'synthetic-seal-format','synthetic-key-ref','synthetic-policy-ref','invalid-envelope',decode('%s','hex'));`, e.id, e.request, e.rawHash, e.rawLen, hex.EncodeToString(e.cipher), e.cipherHash, e.context)
}
func custodyEvidenceSQL(e sealedFixture, r routeRecord, consumer, observation string) string {
	q := evidenceSQL(e)
	q = strings.Replace(q, "stage,owner_service,queue_ref,", "stage,owner_service,queue_ref,consumer_name,source_owner,aggregate_type,aggregate_id,job_event_id,job_observation,", 1)
	return strings.Replace(q, "'intake','inventory','synthetic-owner-queue',", fmt.Sprintf("'custody-handler','inventory',NULL,'%s','inventory','fixture','%s','%s','%s',", consumer, r.Stream, r.ID, observation), 1)
}
func actionSQL(evidence, id, prior, request, action, path, consumer, outcome string) string {
	p, c := "NULL", "NULL"
	if prior != "" {
		p = routeQuote(prior)
	}
	if consumer != "" {
		c = routeQuote(consumer)
	}
	return fmt.Sprintf(`INSERT INTO eventstore.quarantine_actions(action_id,evidence_id,request_id,prior_action_id,action,evidence_format_version,
 actor_ref,authority_ref,scope_ref,purpose_ref,repair_ref,intended_path,intended_consumer,manifest_ref,manifest_sha256,outcome_code)
 VALUES('%s','%s','%s',%s,'%s',1,'synthetic-actor','synthetic-authority','synthetic-scope','synthetic-purpose','synthetic-no-correction',
 '%s',%s,'synthetic-canonical-manifest',sha256('synthetic-manifest'::bytea),'%s');`, id, evidence, request, p, action, path, c, outcome)
}
func acceptedActionSQL(evidence, prior, path, consumer, kind, event, hash string) string {
	q := actionSQL(evidence, uuid.NewString(), prior, uuid.NewString(), "redrive-result", path, consumer, "accepted")
	c := "NULL"
	if kind != "custody" {
		c = routeQuote(consumer)
	}
	q = strings.Replace(q, "manifest_sha256,outcome_code)", "manifest_sha256,outcome_code,accepted_event_id,accepted_event_hash,receipt_kind,receipt_consumer_name,receipt_event_id)", 1)
	return strings.Replace(q, "'accepted');", fmt.Sprintf("'accepted','%s',decode('%s','hex'),'%s',%s,'%s');", event, hash, kind, c, event), 1)
}

func quarantinePreimage(t *testing.T, s *routeStore) string {
	t.Helper()
	var rows []string
	for _, tab := range []string{"messaging_route_compatibility", "projection_checkpoint_compatibility", "consumer_bootstraps", "consumer_checkpoints", "consumer_gaps", "consumer_gap_attempts"} {
		rows = append(rows, s.must(t, s.migration, "SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]') FROM eventstore."+tab+" t"))
	}
	return s.snapshot(t) + "\n" + strings.Join(rows, "\n")
}
func quarantineSave(t *testing.T, s *routeStore, label string) {
	t.Helper()
	s.saveEvidence(t, label)
	for name, body := range map[string]string{"preimage": quarantinePreimage(t, s), "catalog": s.catalog(t), "functions": s.functions(t, true), "function-acls": s.functions(t, false)} {
		p := filepath.Join(s.f.evidence, s.database+"-"+label+"-"+name+".txt")
		b := []byte(body + "\n")
		if err := os.WriteFile(p, b, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p+".sha256", []byte(fmt.Sprintf("%x\n", sha256.Sum256(b))), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func quarantineRejectInstall(t *testing.T, s *routeStore, overrides ...string) {
	t.Helper()
	before, catalog, functions := quarantinePreimage(t, s), s.catalog(t), s.functions(t, true)
	if out, err := installQuarantine(t, s, overrides...); err == nil {
		t.Fatal("unexpected install success", out)
	}
	if quarantinePreimage(t, s) != before || s.catalog(t) != catalog || s.functions(t, true) != functions {
		t.Fatal("failed migration changed retained content/catalog")
	}
	if got := s.must(t, s.migration, "SELECT count(*) FROM pg_class WHERE relnamespace='eventstore'::regnamespace AND relname LIKE 'quarantine_%'"); got != "0" {
		t.Fatal("partial quarantine DDL", got)
	}
}

func TestQuarantineMigrationPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_QUARANTINE") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_QUARANTINE=1 for owned PostgreSQL fixture")
	}
	f := newQuarantineFixture(t)
	for p, want := range map[string]string{"schema.sql": checkpointBaseHash, "migrations/000002_messaging_delivery.up.sql": routePriorHash, "migrations/000003_messaging_route_compatibility.up.sql": checkpointRouteHash, "migrations/000004_projection_checkpoint.up.sql": quarantineCheckpointHash} {
		b, err := os.ReadFile(p)
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(b)) != want {
			t.Fatalf("prior artifact changed: %s %v", p, err)
		}
	}
	t.Logf("000005 SHA-256 %x", sha256.Sum256([]byte(quarantineScript(t))))
	t.Run("populated legacy and custody preserve every prior row catalog and function", func(t *testing.T) {
		for _, custody := range []bool{false, true} {
			t.Run(fmt.Sprint(custody), func(t *testing.T) {
				s := quarantineStore(t, f)
				r := s.produce(t, "inventory")[0]
				if custody {
					quarantineCutover(t, s, r)
					a, c := uuid.NewString(), "synthetic-consumer"
					s.must(t, s.runtime, "BEGIN;"+routeAdmission(a, r.Stream, "local-consumer", c, "inventory.fixture.changed.v1")+routeCustody(r, a, c, "generation-1")+"COMMIT;")
				}
				b := checkpointID(3)
				if !custody {
					s.must(t, s.runtime, "BEGIN;"+bootstrapSQL(b, "", "projection")+checkpointSQL(b)+"COMMIT;")
				}
				before, catalog, functions, acls := quarantinePreimage(t, s), s.catalog(t), s.functions(t, true), s.functions(t, false)
				quarantineSave(t, s, "before-quarantine")
				mustQuarantine(t, s)
				if quarantinePreimage(t, s) != before {
					t.Fatal("prior rows changed")
				}
				// No FK points into an old table: context/receipt checks are invoker
				// reads, preserving every old trigger attachment as well as its ACL.
				for _, pair := range [][2]string{{catalog, s.catalog(t)}, {functions, s.functions(t, true)}, {acls, s.functions(t, false)}} {
					var old, new map[string]json.RawMessage
					if err := json.Unmarshal([]byte(pair[0]), &old); err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal([]byte(pair[1]), &new); err != nil {
						t.Fatal(err)
					}
					for k, v := range old {
						if string(v) != string(new[k]) {
							t.Fatal("old catalog/function changed", k)
						}
					}
				}
				if got := s.must(t, s.runtime, "SELECT migration_revision||':'||feature_format_version||':'||(SELECT schema_version FROM eventstore.messaging_mode) FROM eventstore.quarantine_compatibility"); got != "5:1:2" {
					t.Fatal(got)
				}
				if got := s.must(t, s.runtime, "SELECT (SELECT count(*) FROM eventstore.quarantine_evidence)+(SELECT count(*) FROM eventstore.quarantine_actions)"); got != "0" {
					t.Fatal("inferred evidence", got)
				}
				if out, err := installQuarantine(t, s); err == nil {
					t.Fatal("reinstall succeeded", out)
				}
				quarantineSave(t, s, "after-quarantine")
			})
		}
	})
	t.Run("malformed arbitrary bytes exact requests immutable protected storage", func(t *testing.T) {
		s := quarantineStore(t, f)
		mustQuarantine(t, s)
		for _, raw := range [][]byte{nil, []byte("{broken-secret-token"), {0, 255, 128, 1}, []byte(`{"id":"not-a-uuid","schemaVersion":999}`)} {
			e := fixtureSeal(raw, "trusted-queue-A")
			s.must(t, s.runtime, evidenceSQL(e))
			if got := s.must(t, s.runtime, fmt.Sprintf("SELECT encode(raw_sha256,'hex')||':'||raw_byte_length||':'||encode(capture_context_digest,'hex') FROM eventstore.quarantine_evidence WHERE capture_request_id='%s'", e.request)); got != fmt.Sprintf("%s:%d:%s", e.rawHash, e.rawLen, e.context) {
				t.Fatal("request receipt mismatch", got)
			}
			copy := e
			copy.id = uuid.NewString()
			s.reject(t, s.runtime, evidenceSQL(copy), "duplicate key")
			copy.rawHash = strings.Repeat("00", 32)
			s.reject(t, s.runtime, evidenceSQL(copy), "duplicate key")
			other := fixtureSeal(raw, "trusted-queue-B")
			s.must(t, s.runtime, evidenceSQL(other))
		}
		for name, mutate := range map[string]func(string) string{
			"tampered ciphertext": func(q string) string {
				return strings.Replace(q, "'synthetic-seal-format'", "'synthetic-seal-format'", 1)
			},
			"missing policy": func(q string) string { return strings.Replace(q, "'synthetic-policy-ref'", "NULL", 1) },
			"missing key":    func(q string) string { return strings.Replace(q, "'synthetic-key-ref'", "''", 1) },
			"unbounded reason": func(q string) string {
				return strings.Replace(q, "'invalid-envelope'", "'"+strings.Repeat("x", 65)+"'", 1)
			},
			"unsafe error string": func(q string) string { return strings.Replace(q, "'invalid-envelope'", "E'bad\\nsecret'", 1) },
			"infinite timestamp": func(q string) string {
				return strings.Replace(strings.Replace(q, "capture_context_digest)", "capture_context_digest,created_at)", 1), "'hex'));", "'hex'),'infinity');", 1)
			},
		} {
			t.Run(name, func(t *testing.T) {
				e := fixtureSeal([]byte("sensitive-input"), "trusted")
				if name == "tampered ciphertext" {
					e.cipher = []byte("tampered")
				}
				if out, err := f.sql(s.database, s.runtime, mutate(evidenceSQL(e))); err == nil {
					t.Fatal("invalid evidence accepted", out)
				}
			})
		}
		for _, table := range []string{"quarantine_evidence", "quarantine_actions", "quarantine_compatibility"} {
			for _, q := range []string{"DELETE FROM eventstore." + table, "TRUNCATE eventstore." + table} {
				s.reject(t, s.runtime, q, "permission denied")
				if strings.HasPrefix(q, "TRUNCATE") {
					s.reject(t, s.migration, q+" CASCADE", "immutable")
				} else if table != "quarantine_actions" {
					s.reject(t, s.migration, q, "immutable")
				}
			}
		}
		s.reject(t, s.runtime, "UPDATE eventstore.quarantine_evidence SET reason_code='other'", "permission denied")
		s.reject(t, s.migration, "UPDATE eventstore.quarantine_evidence SET reason_code='other'", "immutable")
		visible := s.must(t, s.runtime, "SELECT jsonb_agg(to_jsonb(t)-'sealed_ciphertext') FROM eventstore.quarantine_evidence t")
		if strings.Contains(visible, "broken-secret-token") || strings.Contains(visible, "not-a-uuid") {
			t.Fatal("plaintext leaked to metadata")
		}
		if got := s.must(t, s.runtime, "SELECT count(*) FROM information_schema.columns WHERE table_schema='eventstore' AND table_name='quarantine_evidence' AND column_name IN ('raw_body','error','claimed_event_id','event_id')"); got != "0" {
			t.Fatal("unsafe metadata columns", got)
		}
	})
	t.Run("append only action chains reject roots forks cycles context and unbacked results", func(t *testing.T) {
		s := quarantineStore(t, f)
		mustQuarantine(t, s)
		e := fixtureSeal([]byte("invalid"), "trusted")
		other := fixtureSeal([]byte("invalid"), "other")
		s.must(t, s.runtime, evidenceSQL(e)+evidenceSQL(other))
		root, request := uuid.NewString(), uuid.NewString()
		s.must(t, s.runtime, actionSQL(e.id, root, "", request, "hold", "intake", "", "held"))
		s.reject(t, s.runtime, actionSQL(e.id, uuid.NewString(), "", uuid.NewString(), "hold", "intake", "", "held"), "duplicate key")
		s.reject(t, s.runtime, actionSQL(other.id, uuid.NewString(), root, uuid.NewString(), "hold", "intake", "", "held"), "parent mismatch")
		s.reject(t, s.runtime, actionSQL(e.id, uuid.NewString(), uuid.NewString(), uuid.NewString(), "hold", "intake", "", "held"), "no rows")
		self := uuid.NewString()
		s.reject(t, s.runtime, actionSQL(e.id, self, self, uuid.NewString(), "hold", "intake", "", "held"), "no rows")
		s.reject(t, s.runtime, actionSQL(e.id, uuid.NewString(), root, uuid.NewString(), "redrive-result", "intake", "", "failed"), "exact prior request")
		var wg sync.WaitGroup
		wins := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				wins <- s.db.Exec(actionSQL(e.id, uuid.NewString(), root, uuid.NewString(), "redrive-request", "direct-handler", "synthetic-consumer", "requested")).Error
			}()
		}
		wg.Wait()
		close(wins)
		n := 0
		for err := range wins {
			if err == nil {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("fork winners %d", n)
		}
		tip := s.must(t, s.runtime, "SELECT action_id FROM eventstore.quarantine_actions WHERE prior_action_id='"+root+"'")
		event, hash := uuid.NewString(), strings.Repeat("11", 32)
		accepted := acceptedActionSQL(e.id, tip, "direct-handler", "synthetic-consumer", "inbox", event, hash)
		s.reject(t, s.runtime, accepted, "missing matching authoritative")
		s.must(t, s.runtime, fmt.Sprintf("INSERT INTO eventstore.inbox(consumer_name,event_id,envelope_hash) VALUES('synthetic-consumer','%s',decode('%s','hex'))", event, hash))
		s.reject(t, s.runtime, strings.Replace(accepted, "'synthetic-authority'", "'foreign-authority'", 1), "exact prior request")
		s.reject(t, s.runtime, strings.Replace(accepted, hash, strings.Repeat("22", 32), 1), "missing matching authoritative")
		s.must(t, s.runtime, accepted)
		s.reject(t, s.runtime, accepted, "duplicate key")
		s.reject(t, s.migration, "UPDATE eventstore.quarantine_actions SET outcome_code='failed'", "immutable")
		if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.consumer_checkpoints"); got != "0" {
			t.Fatal("action invented progress", got)
		}
	})
	t.Run("failed capture cancellation and lost commit replies reconcile exact request", func(t *testing.T) {
		s := quarantineStore(t, f)
		mustQuarantine(t, s)
		runner, err := eventstore.NewTransactions(s.db, func(tx *gorm.DB) (*gorm.DB, error) { return tx, nil })
		if err != nil {
			t.Fatal(err)
		}
		assertCount := func(e sealedFixture, want string) {
			t.Helper()
			if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.quarantine_evidence WHERE capture_request_id='"+e.request+"' AND raw_sha256=decode('"+e.rawHash+"','hex') AND capture_context_digest=decode('"+e.context+"','hex') AND ciphertext_sha256=decode('"+e.cipherHash+"','hex') AND raw_byte_length="+fmt.Sprint(e.rawLen)); got != want {
				t.Fatalf("request reconciliation %s != %s", got, want)
			}
		}
		e := fixtureSeal([]byte("invalid"), "trusted")
		err = runner.Run(context.Background(), func(tx *gorm.DB) error {
			if err := tx.Exec(evidenceSQL(e)).Error; err != nil {
				return err
			}
			return errors.New("synthetic failure")
		})
		if err == nil {
			t.Fatal("failed capture succeeded")
		}
		assertCount(e, "0")
		e = fixtureSeal(nil, "trusted")
		func() {
			defer func() {
				if recover() != "synthetic panic" {
					t.Error("panic missing")
				}
			}()
			_ = runner.Run(context.Background(), func(tx *gorm.DB) error {
				if err := tx.Exec(evidenceSQL(e)).Error; err != nil {
					return err
				}
				panic("synthetic panic")
			})
		}()
		assertCount(e, "0")
		e = fixtureSeal(nil, "trusted")
		ctx, cancel := context.WithCancel(context.Background())
		err = runner.Run(ctx, func(tx *gorm.DB) error { err := tx.Exec(evidenceSQL(e)).Error; cancel(); return err })
		if err == nil {
			t.Fatal("cancelled capture succeeded")
		}
		assertCount(e, "0")
		for _, commit := range []bool{false, true} {
			e := fixtureSeal([]byte("bad-bytes"), "trusted")
			pool, err := s.db.DB()
			if err != nil {
				t.Fatal(err)
			}
			fault, err := gorm.Open(postgres.New(postgres.Config{Conn: commitFaultPool{DB: pool, commitFirst: commit}}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			r, err := eventstore.NewTransactions(fault, func(tx *gorm.DB) (*gorm.DB, error) { return tx, nil })
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			err = r.Run(context.Background(), func(tx *gorm.DB) error { calls++; return tx.Exec(evidenceSQL(e)).Error })
			if !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) || calls != 1 {
				t.Fatalf("unknown outcome/retry %v %d", err, calls)
			}
			want := "0"
			if commit {
				want = "1"
			}
			assertCount(e, want)
			wrong := e
			wrong.context = strings.Repeat("00", 32)
			assertCount(wrong, "0")
		}
	})
	t.Run("custody rollback quarantine fencing and completed observation preserve obligations", func(t *testing.T) {
		s := quarantineStore(t, f)
		r := s.produce(t, "inventory")[0]
		quarantineCutover(t, s, r)
		mustQuarantine(t, s)
		a, c := uuid.NewString(), "consumer-one"
		s.must(t, s.runtime, "BEGIN;"+routeAdmission(a, r.Stream, "local-consumer", c, "inventory.fixture.changed.v1")+routeCustody(r, a, c, "generation-1")+"COMMIT;")
		e := fixtureSeal(r.Body, "trusted-job")
		ref := "quarantine:" + e.id
		jobWhere := fmt.Sprintf("consumer_name='%s' AND event_id='%s'", c, r.ID)
		lease := uuid.NewString()
		s.must(t, s.runtime, "UPDATE eventstore.dispatch_jobs SET lease_owner='"+lease+"',lease_until=clock_timestamp()+interval '1 minute',hold_ref='foreign-hold' WHERE "+jobWhere)
		capture := func(tx *gorm.DB, e sealedFixture) error {
			stream, err := eventstore.ReceiverFence("inventory", "inventory", "fixture", r.Stream)
			if err != nil {
				return err
			}
			if _, err := eventstore.PrepareFences(context.Background(), tx, "inventory", eventstore.SharedFence, stream); err != nil {
				return err
			}
			var count int64
			if err := tx.Raw("SELECT count(*) FROM (SELECT 1 FROM eventstore.dispatch_jobs WHERE " + jobWhere + " AND completed_at IS NULL AND lease_owner='" + lease + "' AND lease_until>clock_timestamp() FOR UPDATE) locked").Scan(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return errors.New("synthetic job fence lost")
			}
			if err := tx.Exec(custodyEvidenceSQL(e, r, c, "unfinished") + actionSQL(e.id, uuid.NewString(), "", uuid.NewString(), "hold", "custody-handler", c, "held")).Error; err != nil {
				return err
			}
			result := tx.Exec("UPDATE eventstore.dispatch_jobs SET quarantine_ref=? WHERE "+jobWhere+" AND completed_at IS NULL AND lease_owner='"+lease+"' AND lease_until>clock_timestamp()", "quarantine:"+e.id)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("synthetic final lease lost")
			}
			return nil
		}
		runner, err := eventstore.NewTransactions(s.db, func(tx *gorm.DB) (*gorm.DB, error) { return tx, nil })
		if err != nil {
			t.Fatal(err)
		}
		// Effect transaction fails first. No inbox, completion or checkpoint is
		// retained; evidence uses a new separately fenced transaction.
		err = runner.Run(context.Background(), func(tx *gorm.DB) error {
			if err := tx.Exec("SET LOCAL justix.messaging_mode='custody'; INSERT INTO eventstore.inbox(consumer_name,event_id,envelope_hash) VALUES('" + c + "','" + r.ID + "',decode('" + hex.EncodeToString(r.Hash) + "','hex'))").Error; err != nil {
				return err
			}
			return errors.New("synthetic effect failed")
		})
		if err == nil {
			t.Fatal("effect succeeded")
		}
		err = runner.Run(context.Background(), func(tx *gorm.DB) error {
			if err := capture(tx, e); err != nil {
				return err
			}
			return errors.New("synthetic capture failure")
		})
		if err == nil {
			t.Fatal("failed capture succeeded")
		}
		if got := s.must(t, s.runtime, "SELECT (SELECT count(*)::text FROM eventstore.quarantine_evidence)||(SELECT count(quarantine_ref) FROM eventstore.dispatch_jobs)||(SELECT count(*) FROM eventstore.inbox)"); got != "000" {
			t.Fatal("partial capture", got)
		}
		if err := runner.Run(context.Background(), func(tx *gorm.DB) error { return capture(tx, e) }); err != nil {
			t.Fatal(err)
		}
		if got := s.must(t, s.runtime, "SELECT quarantine_ref||':'||hold_ref||':'||(completed_at IS NULL) FROM eventstore.dispatch_jobs WHERE "+jobWhere); got != ref+":foreign-hold:true" {
			t.Fatal(got)
		}
		wrong := fixtureSeal(r.Body, "wrong-consumer")
		if out, err := f.sql(s.database, s.runtime, custodyEvidenceSQL(wrong, r, "other-consumer", "unfinished")); err == nil {
			t.Fatal("wrong job accepted", out)
		}
		wrong = fixtureSeal([]byte("changed-body"), "trusted-job")
		s.reject(t, s.runtime, custodyEvidenceSQL(wrong, r, c, "unfinished"), "job context mismatch")
		root := s.must(t, s.runtime, "SELECT action_id FROM eventstore.quarantine_actions WHERE evidence_id='"+e.id+"'")
		request := uuid.NewString()
		s.reject(t, s.runtime, actionSQL(e.id, request, root, uuid.NewString(), "redrive-request", "custody-handler", "other-consumer", "requested"), "another custody obligation")
		s.must(t, s.runtime, actionSQL(e.id, request, root, uuid.NewString(), "redrive-request", "custody-handler", c, "requested"))
		// Fixture-authorized release has a result action and only this reference
		// update in one transaction. Foreign hold stays intact.
		s.must(t, s.runtime, "BEGIN;"+actionSQL(e.id, uuid.NewString(), request, uuid.NewString(), "redrive-result", "custody-handler", c, "released")+"UPDATE eventstore.dispatch_jobs SET quarantine_ref=NULL WHERE "+jobWhere+" AND quarantine_ref='"+ref+"' AND completed_at IS NULL;COMMIT;")
		if got := s.must(t, s.runtime, "SELECT hold_ref FROM eventstore.dispatch_jobs WHERE "+jobWhere); got != "foreign-hold" {
			t.Fatal("foreign hold cleared")
		}
		s.reject(t, s.runtime, "BEGIN;"+routeComplete(r, c, r.Hash)+"COMMIT;", "held")
		s.must(t, s.runtime, "UPDATE eventstore.dispatch_jobs SET hold_ref=NULL WHERE "+jobWhere)
		observed := fixtureSeal(r.Body, "trusted-completed-job")
		completion := s.db.Begin()
		defer completion.Rollback()
		stream, err := eventstore.ReceiverFence("inventory", "inventory", "fixture", r.Stream)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := eventstore.PrepareFences(context.Background(), completion, "inventory", eventstore.SharedFence, stream); err != nil {
			t.Fatal(err)
		}
		if err := completion.Exec(routeComplete(r, c, r.Hash)).Error; err != nil {
			t.Fatal(err)
		}
		// A real competing capture waits on the receiver fence. After completion
		// commits its fresh statement must observe the completed job and refuse.
		result := make(chan error, 1)
		go func() {
			result <- runner.Run(context.Background(), func(tx *gorm.DB) error {
				if err := tx.Exec("SET LOCAL application_name='t927-completed-race'").Error; err != nil {
					return err
				}
				return capture(tx, observed)
			})
		}()
		deadline := time.Now().Add(3 * time.Second)
		for {
			var waiting int64
			if err := s.db.Raw("SELECT count(*) FROM pg_stat_activity WHERE application_name='t927-completed-race' AND wait_event_type='Lock'").Scan(&waiting).Error; err != nil {
				t.Fatal(err)
			}
			if waiting == 1 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("competing quarantine did not block on fence")
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err := completion.Commit().Error; err != nil {
			t.Fatal(err)
		}
		if err := <-result; err == nil || !strings.Contains(err.Error(), "job fence lost") {
			t.Fatalf("completed job capture result: %v", err)
		}
		if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.quarantine_evidence WHERE capture_request_id='"+observed.request+"'"); got != "0" {
			t.Fatal("race persisted false unfinished observation")
		}
		s.reject(t, s.runtime, custodyEvidenceSQL(observed, r, c, "unfinished"), "job context mismatch")
		s.must(t, s.runtime, custodyEvidenceSQL(observed, r, c, "completed"))
		s.reject(t, s.runtime, "UPDATE eventstore.dispatch_jobs SET completed_at=NULL,inbox_consumer_name=NULL,inbox_event_id=NULL WHERE "+jobWhere, "completion is immutable")
		if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.dispatch_jobs WHERE "+jobWhere+" AND completed_at IS NOT NULL AND quarantine_ref IS NULL"); got != "1" {
			t.Fatal("completed job changed", got)
		}
		jobRequest := uuid.NewString()
		s.must(t, s.runtime, actionSQL(observed.id, jobRequest, "", uuid.NewString(), "redrive-request", "custody-handler", c, "requested"))
		s.must(t, s.runtime, acceptedActionSQL(observed.id, jobRequest, "custody-handler", c, "job", r.ID, hex.EncodeToString(r.Hash)))
		intake := fixtureSeal(r.Body, "synthetic-intake-context")
		intakeRequest := uuid.NewString()
		s.must(t, s.runtime, evidenceSQL(intake)+actionSQL(intake.id, intakeRequest, "", uuid.NewString(), "redrive-request", "intake", "", "requested"))
		s.reject(t, s.runtime, acceptedActionSQL(intake.id, intakeRequest, "intake", "", "custody", uuid.NewString(), hex.EncodeToString(r.Hash)), "missing matching authoritative")
		s.must(t, s.runtime, acceptedActionSQL(intake.id, intakeRequest, "intake", "", "custody", r.ID, hex.EncodeToString(r.Hash)))
	})
	t.Run("installation rejects lineage wrong owner and privilege escalation atomically", func(t *testing.T) {
		for name, override := range map[string][]string{"base": {"-v", "base_schema_sha256=" + strings.Repeat("0", 64)}, "checkpoint": {"-v", "checkpoint_migration_sha256=" + strings.Repeat("0", 64)}, "quarantine": {"-v", "quarantine_migration_sha256=invalid"}, "owner": {"-v", "owner_service=retail"}, "runtime": {"-v", "runtime_role=postgres"}, "backup": {"-v", "backup_ref="}} {
			t.Run(name, func(t *testing.T) { s := quarantineStore(t, f); quarantineRejectInstall(t, s, override...) })
		}
		for name, change := range map[string]func(*routeStore) string{
			"table update": func(s *routeStore) string { return "GRANT UPDATE ON eventstore.consumer_bootstraps TO " + s.runtime },
			"column update": func(s *routeStore) string {
				return "GRANT UPDATE(start_after) ON eventstore.consumer_bootstraps TO " + s.runtime
			},
			"column references": func(s *routeStore) string {
				return "GRANT REFERENCES(position) ON eventstore.consumer_checkpoints TO " + s.runtime
			},
			"marker insert": func(s *routeStore) string {
				return "GRANT INSERT(singleton) ON eventstore.projection_checkpoint_compatibility TO " + s.runtime
			},
			"PUBLIC select": func(s *routeStore) string { return "GRANT SELECT ON eventstore.consumer_bootstraps TO PUBLIC" },
			"PUBLIC column": func(s *routeStore) string {
				return "GRANT SELECT(position) ON eventstore.consumer_checkpoints TO PUBLIC"
			},
			"delegable select": func(s *routeStore) string {
				return "GRANT SELECT ON eventstore.consumer_bootstraps TO " + s.runtime + " WITH GRANT OPTION"
			},
			"default update": func(s *routeStore) string {
				return "ALTER DEFAULT PRIVILEGES IN SCHEMA eventstore GRANT UPDATE ON TABLES TO " + s.runtime
			},
			"default PUBLIC": func(s *routeStore) string {
				return "ALTER DEFAULT PRIVILEGES IN SCHEMA eventstore GRANT SELECT ON TABLES TO PUBLIC"
			},
			"schema create": func(s *routeStore) string { return "GRANT CREATE ON SCHEMA eventstore TO " + s.runtime },
			"database temp": func(s *routeStore) string { return "GRANT TEMP ON DATABASE " + s.database + " TO " + s.runtime },
			"migration execute": func(s *routeStore) string {
				return "GRANT EXECUTE ON FUNCTION eventstore.consumer_checkpoint_guard() TO " + s.runtime
			},
			"missing select": func(s *routeStore) string {
				return "REVOKE SELECT ON eventstore.consumer_checkpoints FROM " + s.runtime
			},
			"missing insert":       func(s *routeStore) string { return "REVOKE INSERT ON eventstore.consumer_gaps FROM " + s.runtime },
			"missing schema usage": func(s *routeStore) string { return "REVOKE USAGE ON SCHEMA eventstore FROM " + s.runtime },
		} {
			t.Run(name, func(t *testing.T) {
				s := quarantineStore(t, f)
				s.must(t, s.migration, change(s))
				quarantineRejectInstall(t, s)
			})
		}
		s := quarantineStore(t, f)
		a, b := "hop_"+strings.ReplaceAll(uuid.NewString(), "-", "")[:10], "hop_"+strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
		if out, err := f.sql(s.database, "postgres", "CREATE ROLE "+a+" NOINHERIT;CREATE ROLE "+b+" NOINHERIT;GRANT "+b+" TO "+a+";GRANT "+a+" TO "+s.runtime+" WITH INHERIT FALSE;GRANT UPDATE ON eventstore.consumer_bootstraps TO "+b); err != nil {
			t.Fatal(err, out)
		}
		quarantineRejectInstall(t, s)
		t.Run("installed checkpoint digest mismatch", func(t *testing.T) {
			s := quarantineStore(t, f)
			s.must(t, s.migration, "ALTER TABLE eventstore.projection_checkpoint_compatibility DISABLE TRIGGER immutable_content; UPDATE eventstore.projection_checkpoint_compatibility SET checkpoint_migration_sha256=decode(repeat('00',32),'hex'); ALTER TABLE eventstore.projection_checkpoint_compatibility ENABLE TRIGGER immutable_content")
			quarantineRejectInstall(t, s)
		})
		t.Run("runtime cannot install and foreign owner cannot read evidence", func(t *testing.T) {
			s := quarantineStore(t, f)
			before := quarantinePreimage(t, s)
			if out, err := s.installAs(s.runtime, quarantineScript(t), "-v", "base_schema_sha256="+checkpointBaseHash, "-v", "checkpoint_migration_sha256="+quarantineCheckpointHash, "-v", fmt.Sprintf("quarantine_migration_sha256=%x", sha256.Sum256([]byte(quarantineScript(t))))); err == nil {
				t.Fatal("runtime installed migration", out)
			}
			if quarantinePreimage(t, s) != before {
				t.Fatal("runtime attempt changed old rows")
			}
			mustQuarantine(t, s)
			foreign := f.store(t, "retail", true)
			if out, err := foreign.install(f.correction); err != nil {
				t.Fatal(err, out)
			}
			foreign.must(t, foreign.migration, "REVOKE TEMP ON DATABASE "+foreign.database+" FROM PUBLIC")
			mustCheckpoint(t, foreign)
			mustQuarantine(t, foreign)
			for _, pair := range [][2]*routeStore{{s, foreign}, {foreign, s}} {
				if out, err := f.sql(pair[1].database, pair[0].runtime, "SELECT count(*) FROM eventstore.quarantine_evidence"); err == nil || !strings.Contains(out, "permission denied") {
					t.Fatalf("foreign owner evidence access: %v %s", err, out)
				}
			}
		})
	})
	t.Run("concurrent installers and five second active writer rollback", func(t *testing.T) {
		s := quarantineStore(t, f)
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := installQuarantine(t, s); results <- err }()
		}
		wg.Wait()
		close(results)
		wins := 0
		for err := range results {
			if err == nil {
				wins++
			}
		}
		if wins != 1 {
			t.Fatal("installer winners", wins)
		}
		s = quarantineStore(t, f)
		tx := s.db.Begin()
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		defer tx.Rollback()
		if err := tx.Exec("LOCK TABLE eventstore.outbox IN ROW EXCLUSIVE MODE").Error; err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		quarantineRejectInstall(t, s)
		if elapsed := time.Since(start); elapsed < 4*time.Second || elapsed > 9*time.Second {
			t.Fatal("unbounded or missing lock wait", elapsed)
		}
		tx.Rollback()
		mustQuarantine(t, s)
	})
}
