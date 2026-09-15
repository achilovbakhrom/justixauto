package eventstore_test

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// This fixture uses only an owned random container and its internal loopback;
// no DSN input, host port, external database or real baseline attestation.
func historyFixture(t *testing.T) schemaFixture {
	t.Helper()
	base, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := filepath.Abs("../../infra/local/postgres/init-owners.sql")
	if err != nil {
		t.Fatal(err)
	}
	f := schemaFixture{t: t, container: "justixauto-t929-" + uuid.NewString(), password: "synthetic-" + uuid.NewString(), schema: base}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	args := []string{"run", "-d", "--name", f.container, "--tmpfs", "/var/lib/postgresql:rw", "-e", "POSTGRES_PASSWORD=" + f.password, "-v", bootstrap + ":/docker-entrypoint-initdb.d/10-owners.sql:ro"}
	for _, owner := range owners {
		args = append(args, "-e", "JUSTIXAUTO_"+strings.ToUpper(owner)+"_DB_PASSWORD="+f.password)
	}
	args = append(args, postgresImage)
	if out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture start: %v %s", err, out)
	}
	t.Logf("owned fixture %s; image %s", f.container, postgresImage)
	var ownedVolumes []string
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "docker", "rm", "-fv", f.container).CombinedOutput(); err != nil {
			t.Errorf("owned fixture cleanup: %v %s", err, out)
		}
		if out, err := exec.CommandContext(ctx, "docker", "container", "inspect", f.container).CombinedOutput(); err == nil || !strings.Contains(string(out), "No such container") {
			t.Errorf("owned container absence not verified: %v %s", err, out)
		}
		for _, volume := range ownedVolumes {
			if out, err := exec.CommandContext(ctx, "docker", "volume", "inspect", volume).CombinedOutput(); err == nil || !strings.Contains(string(out), "no such volume") {
				t.Errorf("owned volume absence not verified: %v %s", err, out)
			}
		}
		t.Logf("cleanup verified for owned container and %d attached volumes", len(ownedVolumes))
	})
	if out, err := exec.CommandContext(ctx, "docker", "inspect", "--format", `{{range .Mounts}}{{if eq .Type "volume"}}{{println .Name}}{{end}}{{end}}`, f.container).CombinedOutput(); err != nil {
		t.Fatalf("inspect owned mounts: %v %s", err, out)
	} else {
		ownedVolumes = strings.Fields(string(out))
		t.Logf("captured owned attached volume IDs: %v", ownedVolumes)
	}
	for {
		if _, err := f.sql("documents", "justix_documents", "SELECT 1"); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("fixture bootstrap timeout")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if got := f.mustSQL("identity", "postgres", "SHOW server_version_num"); got != "180006" {
		t.Fatal(got)
	}
	return f
}

type historyStore struct {
	f                            schemaFixture
	owner, script, base, request string
}

func newHistoryStore(t *testing.T, f schemaFixture, owner string) historyStore {
	t.Helper()
	f.t = t
	script, err := os.ReadFile("owner_history.sql")
	if err != nil {
		t.Fatal(err)
	}
	base, err := os.ReadFile("../../services/" + owner + "/migrations/0001_mechanics.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	s := historyStore{f, owner, string(script), string(base), uuid.NewString()}
	f.mustSQL(owner, "postgres", "CREATE ROLE justix_"+owner+"_runtime LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_"+owner+" TO justix_"+owner+"_runtime; REVOKE TEMP ON DATABASE justix_"+owner+" FROM PUBLIC;")
	if out, err := f.install(owner, "justix_"+owner+"_runtime"); err != nil {
		t.Fatalf("shared base: %v %s", err, out)
	}
	// Explicitly labelled SQL protocol fixture; the actual migrate driver/runner
	// is T932/933. This does not assert that the real engine executed version 1.
	s.must(t, "CREATE TABLE public.schema_migrations(version bigint PRIMARY KEY, dirty boolean NOT NULL); INSERT INTO public.schema_migrations VALUES(1,true);")
	s.must(t, s.base)
	s.must(t, "UPDATE public.schema_migrations SET dirty=false")
	s.must(t, eventSQL(owner, uuid.NewString(), uuid.NewString(), "'"+uuid.NewString()+"'", 1))
	if owner == "identity" {
		f.checkAtomicRecords(owner, "'"+uuid.NewString()+"'")
	}
	return s
}
func (s historyStore) must(t *testing.T, sql string) string {
	t.Helper()
	f := s.f
	f.t = t
	return f.mustSQL(s.owner, "justix_"+s.owner, sql)
}
func (s historyStore) install(extra ...string) (string, error) {
	params := []string{"owner_service=" + s.owner, "runtime_role=justix_" + s.owner + "_runtime", fmt.Sprintf("template_sha256=%x", sha256.Sum256([]byte(s.script))),
		"base_schema_sha256=" + checkpointBaseHash, fmt.Sprintf("base_artifact_sha256=%x", sha256.Sum256([]byte(s.base))), "base_manifest_sha256=" + strings.Repeat("12", 32),
		"profile_sha256=" + strings.Repeat("34", 32), "baseline_kind=verified-installation", "evidence_ref=synthetic-protocol-fixture-execution", "approval_ref=synthetic-fixture-approval",
		"backup_ref=synthetic-fixture-backup", "stopped_runtimes_ref=synthetic-no-running-processes", "request_id=" + s.request}
	params = append(params, extra...)
	args := make([]string, 0, len(params)*2)
	for _, p := range params {
		args = append(args, "-v", p)
	}
	return s.f.sql(s.owner, "justix_"+s.owner, s.script, args...)
}
func (s historyStore) snapshot(t *testing.T) string {
	t.Helper()
	// Original table definitions/owners/ACLs/columns/constraints/triggers and all
	// original rows are retained, not merely a row count. New history is separate.
	return s.must(t, `SELECT jsonb_build_object('ledger',(SELECT jsonb_agg(to_jsonb(x) ORDER BY version) FROM public.schema_migrations x),
 'marker',(SELECT jsonb_agg(to_jsonb(x)) FROM `+s.owner+`_mechanics.compatibility x),
 'events',(SELECT jsonb_agg(to_jsonb(x) ORDER BY event_id) FROM eventstore.events x),
 'receipts',(SELECT jsonb_agg(to_jsonb(x) ORDER BY receipt_id) FROM eventstore.command_receipts x),
 'outbox',(SELECT jsonb_agg(to_jsonb(x) ORDER BY event_id) FROM eventstore.outbox x),
 'inbox',(SELECT jsonb_agg(to_jsonb(x) ORDER BY consumer_name,event_id) FROM eventstore.inbox x),
 'operations',(SELECT jsonb_agg(to_jsonb(x) ORDER BY operation_id) FROM eventstore.operations x),
 'steps',(SELECT jsonb_agg(to_jsonb(x) ORDER BY operation_id,step) FROM eventstore.operation_steps x),
 'catalog',(SELECT jsonb_agg(jsonb_build_object('oid',c.oid,'name',c.relname,'owner',c.relowner,'acl',c.relacl,
 'attrs',(SELECT jsonb_agg(to_jsonb(a) ORDER BY attnum) FROM pg_attribute a WHERE a.attrelid=c.oid),
 'constraints',(SELECT jsonb_agg(to_jsonb(k) ORDER BY k.oid) FROM pg_constraint k WHERE k.conrelid=c.oid),
 'triggers',(SELECT jsonb_agg(to_jsonb(g) ORDER BY g.oid) FROM pg_trigger g WHERE g.tgrelid=c.oid)) ORDER BY c.oid)
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind='r' AND n.nspname IN ('eventstore','public','`+s.owner+`_mechanics')),
 'functions',(SELECT jsonb_agg(to_jsonb(p) ORDER BY p.oid) FROM pg_proc p WHERE p.pronamespace='eventstore'::regnamespace));`)
}
func (s historyStore) evidence(t *testing.T, dir, label string) string {
	t.Helper()
	data := s.snapshot(t)
	path := filepath.Join(dir, s.owner+"-"+label+".json")
	if err := os.WriteFile(path, []byte(data+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(data+"\n")))
	if err := os.WriteFile(path+".sha256", []byte(sum+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("retained preimage %s SHA-256 %s", path, sum)
	return data
}
func (s historyStore) reject(t *testing.T, params ...string) {
	t.Helper()
	before := s.snapshot(t)
	if out, err := s.install(params...); err == nil {
		t.Fatalf("unexpected install success %s", out)
	}
	if after := s.snapshot(t); after != before {
		t.Fatal("rejected installation changed existing data/catalog")
	}
	if got := s.must(t, "SELECT count(*) FROM pg_namespace WHERE nspname='owner_migrations'"); got != "0" {
		t.Fatal("partial history schema: " + got)
	}
}
func historyMarker(version int) string {
	return fmt.Sprintf("INSERT INTO owner_migrations.feature_contracts VALUES('synthetic.fixture',%d,%d,decode(repeat('56',32),'hex'));", version, version)
}
func historyReceipt(version, prior int, priorHash string) string {
	return fmt.Sprintf(`INSERT INTO owner_migrations.artifacts(version,filename,sql_sha256,predecessor_version,predecessor_sha256,
 feature_contract_id,feature_contract_revision,feature_contract_sha256,installation_request_id,artifact_manifest_sha256,installing_profile_sha256,provenance_kind,provenance_ref)
 VALUES(%d,'migrations/%04d_fixture.up.sql',decode(repeat('78',32),'hex'),%d,decode('%s','hex'),'synthetic.fixture',%d,decode(repeat('56',32),'hex'),'%s',decode(repeat('9a',32),'hex'),decode(repeat('bc',32),'hex'),'verified-installation','synthetic-feature-execution');`, version, version, prior, priorHash, version, uuid.NewString())
}
func TestOwnerHistoryPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_OWNER_HISTORY") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_OWNER_HISTORY=1 for an owned PostgreSQL fixture")
	}
	f := historyFixture(t)
	evidence, err := os.MkdirTemp("", "justixauto-t929-evidence-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("retained evidence directory %s", evidence)
	t.Run("baseline failures are atomic and narrow grants are required", func(t *testing.T) {
		s := newHistoryStore(t, f, "identity")
		s.evidence(t, evidence, "before-rejections")
		for _, params := range [][]string{{"owner_service=inventory"}, {"runtime_role=justix_identity"}, {"runtime_role=missing"}, {"base_schema_sha256=" + strings.Repeat("ff", 32)},
			{"template_sha256="}, {"template_sha256=" + strings.Repeat("00", 32)}, {"base_artifact_sha256=invalid"}, {"base_manifest_sha256=aa"}, {"profile_sha256=" + strings.Repeat("00", 32)},
			{"baseline_kind=inferred"}, {"evidence_ref="}, {"approval_ref= "}, {"backup_ref="}, {"stopped_runtimes_ref=bad\nref"}, {"request_id=00000000-0000-0000-0000-000000000000"}} {
			s.reject(t, params...)
		}
		for _, change := range []struct{ setup, restore string }{
			{"UPDATE public.schema_migrations SET dirty=true", "UPDATE public.schema_migrations SET dirty=false"},
			{"UPDATE public.schema_migrations SET version=12", "UPDATE public.schema_migrations SET version=1"},
			{"INSERT INTO public.schema_migrations VALUES(2,false)", "DELETE FROM public.schema_migrations WHERE version=2"},
			{"DELETE FROM public.schema_migrations", "INSERT INTO public.schema_migrations VALUES(1,false)"},
			{"ALTER TABLE eventstore.events ALTER owner_service SET DEFAULT 'inventory'", "ALTER TABLE eventstore.events ALTER owner_service SET DEFAULT 'identity'"},
			{"REVOKE SELECT ON public.schema_migrations FROM justix_identity_runtime", "GRANT SELECT ON public.schema_migrations TO justix_identity_runtime"},
			{"REVOKE UPDATE(phase) ON eventstore.operations FROM justix_identity_runtime", "GRANT UPDATE(phase) ON eventstore.operations TO justix_identity_runtime"},
			{"GRANT INSERT(version) ON public.schema_migrations TO justix_identity_runtime", "REVOKE INSERT(version) ON public.schema_migrations FROM justix_identity_runtime"},
			{"GRANT REFERENCES(owner_service) ON identity_mechanics.compatibility TO justix_identity_runtime", "REVOKE REFERENCES(owner_service) ON identity_mechanics.compatibility FROM justix_identity_runtime"},
			{"GRANT MAINTAIN ON eventstore.events TO justix_identity_runtime", "REVOKE MAINTAIN ON eventstore.events FROM justix_identity_runtime"},
			{"GRANT SELECT ON public.schema_migrations TO justix_identity_runtime WITH GRANT OPTION", "REVOKE GRANT OPTION FOR SELECT ON public.schema_migrations FROM justix_identity_runtime"},
			{"GRANT SELECT ON public.schema_migrations TO PUBLIC", "REVOKE SELECT ON public.schema_migrations FROM PUBLIC"},
			{"GRANT TEMP ON DATABASE justix_identity TO PUBLIC", "REVOKE TEMP ON DATABASE justix_identity FROM PUBLIC"},
			{"GRANT CREATE ON SCHEMA public TO justix_identity_runtime", "REVOKE CREATE ON SCHEMA public FROM justix_identity_runtime"},
			{"ALTER DEFAULT PRIVILEGES GRANT SELECT ON TABLES TO PUBLIC", "ALTER DEFAULT PRIVILEGES REVOKE SELECT ON TABLES FROM PUBLIC"},
			{"ALTER DEFAULT PRIVILEGES GRANT UPDATE ON TABLES TO justix_identity_runtime", "ALTER DEFAULT PRIVILEGES REVOKE UPDATE ON TABLES FROM justix_identity_runtime"},
		} {
			s.must(t, change.setup)
			s.reject(t)
			s.must(t, change.restore)
		}
		f.mustSQL("identity", "postgres", "CREATE ROLE history_bridge NOINHERIT; CREATE ROLE history_delegate NOINHERIT; GRANT history_delegate TO history_bridge WITH INHERIT FALSE; GRANT history_bridge TO justix_identity_runtime WITH INHERIT FALSE;")
		s.must(t, "GRANT INSERT(version) ON public.schema_migrations TO history_delegate")
		s.reject(t)
		s.must(t, "REVOKE INSERT(version) ON public.schema_migrations FROM history_delegate")
		s.must(t, "ALTER DEFAULT PRIVILEGES GRANT SELECT ON TABLES TO justix_inventory")
		s.reject(t)
		s.must(t, "ALTER DEFAULT PRIVILEGES REVOKE SELECT ON TABLES FROM justix_inventory")
		f.mustSQL("identity", "postgres", "GRANT justix_identity TO history_delegate WITH INHERIT FALSE")
		s.reject(t)
		f.mustSQL("identity", "postgres", "REVOKE justix_identity FROM history_delegate")
		before := s.evidence(t, evidence, "before-install")
		if out, err := s.install(); err != nil {
			t.Fatalf("history install: %v %s", err, out)
		}
		if s.evidence(t, evidence, "after-install") != before {
			t.Fatal("successful installation changed preimage")
		}
		if out, err := s.install(); err == nil {
			t.Fatalf("reinstallation succeeded: %s", out)
		}
		if got := s.must(t, "SELECT (SELECT count(*) FROM owner_migrations.artifacts)||':'||(SELECT count(*) FROM owner_migrations.feature_contracts)"); got != "1:0" {
			t.Fatal(got)
		}
		if got := s.must(t, "SELECT encode(template_sha256,'hex')||':'||encode(base_artifact_sha256,'hex') FROM owner_migrations.compatibility"); got != fmt.Sprintf("%x:%x", sha256.Sum256([]byte(s.script)), sha256.Sum256([]byte(s.base))) {
			t.Fatal("hash binding " + got)
		}
		t.Logf("template SHA-256 %x; identity v1 SHA-256 %x", sha256.Sum256([]byte(s.script)), sha256.Sum256([]byte(s.base)))
		for _, role := range []string{"justix_identity", "justix_identity_runtime"} {
			for _, table := range []string{"compatibility", "artifacts", "feature_contracts"} {
				for _, sql := range []string{"DELETE FROM owner_migrations." + table, "TRUNCATE owner_migrations." + table, "UPDATE owner_migrations." + table + " SET " + map[string]string{"compatibility": "singleton=singleton", "artifacts": "version=version", "feature_contracts": "contract_id=contract_id"}[table]} {
					if out, err := f.sql("identity", role, sql); err == nil {
						t.Fatalf("mutable %s: %s", role, out)
					}
				}
			}
		}
		for _, sql := range []string{"INSERT INTO owner_migrations.artifacts(version) VALUES(9)", "INSERT INTO owner_migrations.feature_contracts VALUES('x',1,9,decode(repeat('11',32),'hex'))", "CREATE TABLE owner_migrations.bad(id int)", "ALTER TABLE owner_migrations.artifacts DISABLE TRIGGER ALL"} {
			if _, err := f.sql("identity", "justix_identity_runtime", sql); err == nil {
				t.Fatal("runtime privilege accepted: " + sql)
			}
		}
		// PostgreSQL GRANT without grant option may succeed with a warning and
		// grant nothing. Verify authoritative ACL effect, not command exit alone.
		_, _ = f.sql("identity", "justix_identity_runtime", "GRANT SELECT ON owner_migrations.artifacts TO history_delegate")
		if got := s.must(t, "SELECT has_table_privilege('history_delegate','owner_migrations.artifacts','SELECT')"); got != "f" {
			t.Fatal("runtime delegated history SELECT")
		}
		if out, err := f.sql("identity", "justix_identity_runtime", "BEGIN READ ONLY; SELECT count(*) FROM owner_migrations.compatibility; SELECT count(*) FROM owner_migrations.artifacts; SELECT count(*) FROM owner_migrations.feature_contracts; COMMIT;"); err != nil {
			t.Fatalf("read only metadata: %v %s", err, out)
		}
		for _, foreign := range owners {
			if foreign != "identity" {
				if _, err := f.sql(foreign, "justix_identity_runtime", "SELECT 1"); err == nil {
					t.Fatal("foreign database connected: " + foreign)
				}
			}
		}
		t.Run("feature dirty marker and immutable receipt binding", func(t *testing.T) {
			baseline := s.must(t, "SELECT row_to_json(x) FROM owner_migrations.artifacts x WHERE version=1")
			reject := func(sql string) {
				t.Helper()
				if out, err := f.sql("identity", "justix_identity", sql); err == nil {
					t.Fatalf("unexpected feature mutation: %s", out)
				}
			}
			reject(historyMarker(12))
			s.must(t, "UPDATE public.schema_migrations SET version=12,dirty=true")
			reject(historyMarker(17))
			reject(historyReceipt(12, 1, fmt.Sprintf("%x", sha256.Sum256([]byte(s.base)))))
			s.must(t, historyMarker(12))
			reject(historyMarker(12))
			reject(historyReceipt(12, 1, strings.Repeat("ff", 32)))
			reject(strings.Replace(historyReceipt(12, 1, fmt.Sprintf("%x", sha256.Sum256([]byte(s.base)))), "repeat('56',32)", "repeat('ff',32)", 1))
			reject(strings.Replace(historyReceipt(12, 1, fmt.Sprintf("%x", sha256.Sum256([]byte(s.base)))), "'verified-installation'", "'attested-existing-v1'", 1))
			infinite := historyReceipt(12, 1, fmt.Sprintf("%x", sha256.Sum256([]byte(s.base))))
			infinite = strings.Replace(infinite, "provenance_kind,provenance_ref)", "provenance_kind,provenance_ref,completed_at)", 1)
			infinite = strings.Replace(infinite, "'synthetic-feature-execution');", "'synthetic-feature-execution','infinity');", 1)
			reject(infinite)
			reject("BEGIN; " + historyReceipt(12, 1, fmt.Sprintf("%x", sha256.Sum256([]byte(s.base)))) + " SELECT 1/0; COMMIT;")
			if got := s.must(t, "SELECT count(*) FROM owner_migrations.artifacts"); got != "1" {
				t.Fatal("failed finalization retained receipt")
			}
			s.must(t, "BEGIN; "+historyReceipt(12, 1, fmt.Sprintf("%x", sha256.Sum256([]byte(s.base))))+" UPDATE public.schema_migrations SET dirty=false; COMMIT;")
			previous := s.must(t, "SELECT row_to_json(x) FROM owner_migrations.artifacts x WHERE version=12")
			s.must(t, "UPDATE public.schema_migrations SET version=17,dirty=true")
			s.must(t, historyMarker(17))
			reject(historyReceipt(17, 1, fmt.Sprintf("%x", sha256.Sum256([]byte(s.base)))))
			reject(`INSERT INTO owner_migrations.artifacts(version,filename,sql_sha256,predecessor_version,predecessor_sha256,
 feature_contract_id,feature_contract_revision,feature_contract_sha256,installation_request_id,artifact_manifest_sha256,installing_profile_sha256,provenance_kind,provenance_ref)
 SELECT 17,'migrations/0017_fixture.up.sql',sql_sha256,12,sql_sha256,feature_contract_id,17,feature_contract_sha256,
 installation_request_id,artifact_manifest_sha256,installing_profile_sha256,provenance_kind,provenance_ref FROM owner_migrations.artifacts WHERE version=12`)
			s.must(t, "BEGIN; "+historyReceipt(17, 12, strings.Repeat("78", 32))+" UPDATE public.schema_migrations SET dirty=false; COMMIT;")
			if baseline != s.must(t, "SELECT row_to_json(x) FROM owner_migrations.artifacts x WHERE version=1") || previous != s.must(t, "SELECT row_to_json(x) FROM owner_migrations.artifacts x WHERE version=12") {
				t.Fatal("upgrade rewrote prior receipt/profile audit")
			}
			reject(historyMarker(12))
			reject("INSERT INTO owner_migrations.artifacts SELECT * FROM owner_migrations.artifacts WHERE version=1")
		})
	})
	t.Run("bounded same key and concurrent installation", func(t *testing.T) {
		s := newHistoryStore(t, f, "inventory")
		hash := sha256.Sum256([]byte("justixauto:owner-install:v1:justix_inventory"))
		key := int64(binary.BigEndian.Uint64(hash[:8]))
		if got := s.must(t, "SELECT ('x'||substr(encode(sha256(convert_to('justixauto:owner-install:v1:'||current_database(),'UTF8')),'hex'),1,16))::bit(64)::bigint"); got != fmt.Sprint(key) {
			t.Fatal("key mismatch")
		}
		done := make(chan error, 1)
		go func() {
			_, err := f.sql("inventory", "justix_inventory", fmt.Sprintf("SELECT pg_advisory_lock(%d); SELECT pg_sleep(7);", key))
			done <- err
		}()
		deadline := time.Now().Add(3 * time.Second)
		for {
			if got := s.must(t, "SELECT count(*) FROM pg_locks WHERE locktype='advisory' AND database=(SELECT oid FROM pg_database WHERE datname=current_database()) AND granted"); got == "1" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("lock holder not ready")
			}
			time.Sleep(20 * time.Millisecond)
		}
		start := time.Now()
		s.reject(t)
		elapsed := time.Since(start)
		if elapsed < 4*time.Second || elapsed > 6500*time.Millisecond {
			t.Fatalf("lock bound %s", elapsed)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		type attempt struct {
			request string
			err     error
		}
		results := make(chan attempt, 2)
		for i := 0; i < 2; i++ {
			wg.Go(func() {
				request := uuid.NewString()
				_, err := s.install("request_id="+request, "baseline_kind=attested-existing-v1", "evidence_ref=synthetic-explicit-existing-v1-attestation")
				results <- attempt{request, err}
			})
		}
		wg.Wait()
		close(results)
		successes := 0
		winner := ""
		for result := range results {
			if result.err == nil {
				successes++
				winner = result.request
			}
		}
		if successes != 1 {
			t.Fatalf("concurrent successes %d", successes)
		}
		if got := s.must(t, "SELECT installation_request_id::text FROM owner_migrations.compatibility"); got != winner {
			t.Fatal("retained installation request differs from winner")
		}
		if got := s.must(t, "SELECT baseline_kind FROM owner_migrations.compatibility"); got != "attested-existing-v1" {
			t.Fatal(got)
		}
		s.evidence(t, evidence, "after-concurrent-install")
	})
	t.Run("explicit legacy additive messaging and custody refusal", func(t *testing.T) {
		s := newHistoryStore(t, f, "commerce")
		prior, err := os.ReadFile("migrations/000002_messaging_delivery.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if out, err := f.sql("commerce", "justix_commerce", string(prior), "-v", "owner_service=commerce", "-v", "runtime_role=justix_commerce_runtime", "-v", "backup_ref=synthetic-backup", "-v", "stopped_runtimes_ref=synthetic-stopped", "-v", "compatibility_ref=synthetic-compatibility"); err != nil {
			t.Fatalf("additive messaging: %v %s", err, out)
		}
		// Roll back the install after successful full validation to preserve the
		// fixture for the real one-way cutover rejection, without dropping history.
		probe := s
		probe.script = strings.TrimSuffix(strings.TrimSpace(s.script), "COMMIT;") + "ROLLBACK;\n"
		if out, err := probe.install(); err != nil {
			t.Fatalf("legacy additive: %v %s", err, out)
		}
		s.must(t, routeCutover())
		s.reject(t)
	})
}
