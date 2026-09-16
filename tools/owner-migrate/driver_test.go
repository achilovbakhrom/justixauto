package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/jackc/pgx/v5"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"justixauto/pkg/persistence"
)

const pgImage = "docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"
const fixtureLabel = "justixauto.t932.fixture-owner"

type containerIdentity struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	Labels map[string]string `json:"labels"`
	Tmpfs  map[string]string `json:"tmpfs"`
	Mounts []struct {
		Type        string
		Source      string
		Destination string
		RW          bool
	} `json:"mounts"`
}

// Deliberately never inspect/log Config.Env, credentials or unrelated objects.
func inspectFixture(ctx context.Context, docker, name string) (containerIdentity, bool, error) {
	var result containerIdentity
	format := `{"id":{{json .Id}},"name":{{json .Name}},"image":{{json .Config.Image}},"labels":{{json .Config.Labels}},"tmpfs":{{json .HostConfig.Tmpfs}},"mounts":{{json .Mounts}}}`
	out, err := exec.CommandContext(ctx, docker, "inspect", "--format", format, name).CombinedOutput()
	if err != nil {
		if strings.Contains(strings.ToLower(string(out)), "no such object") {
			return result, false, nil
		}
		return result, false, fmt.Errorf("fixture identity inspection: %w", err)
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return result, false, err
	}
	return result, true, nil
}
func (v containerIdentity) validate(name, token, root string) error {
	nameUUID, err := uuid.Parse(strings.TrimPrefix(name, "justixauto-t932-"))
	if err != nil || nameUUID == uuid.Nil || name != "justixauto-t932-"+nameUUID.String() {
		return errors.New("invalid generated fixture name")
	}
	tokenUUID, err := uuid.Parse(token)
	if err != nil || tokenUUID == uuid.Nil || token != tokenUUID.String() {
		return errors.New("invalid fixture ownership token")
	}
	id, err := hex.DecodeString(v.ID)
	if err != nil || len(id) != 32 || hex.EncodeToString(id) != v.ID || v.Name != "/"+name || v.Image != pgImage || v.Labels[fixtureLabel] != token {
		return errors.New("fixture identity mismatch; no removal authorized")
	}
	if len(v.Tmpfs) != 1 || v.Tmpfs["/var/lib/postgresql"] != "rw" {
		return errors.New("fixture tmpfs mismatch; no removal authorized")
	}
	binds := 0
	for _, m := range v.Mounts {
		if m.Type == "tmpfs" && m.Destination == "/var/lib/postgresql" {
			continue
		}
		if m.Type == "bind" && m.Source == filepath.Join(root, "infra/local/postgres/init-owners.sql") && m.Destination == "/docker-entrypoint-initdb.d/10-owners.sql" && !m.RW {
			binds++
			continue
		}
		return errors.New("unexpected fixture mount; no removal authorized")
	}
	if binds != 1 {
		return errors.New("exact readonly fixture bootstrap bind required")
	}
	return nil
}
func cleanupFixture(t *testing.T, docker, name, token, root string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	v, exists, err := inspectFixture(ctx, docker, name)
	if err != nil {
		t.Error(err)
		return
	}
	if !exists {
		t.Logf("fixture absent after failed allocation: %s", name)
		return
	}
	if err := v.validate(name, token, root); err != nil {
		t.Error(err)
		return
	}
	// Remove only the validated immutable ID. Validation admits no volumes and
	// only this task's read-only bootstrap bind; never remove mounts or roots.
	if _, err := exec.CommandContext(ctx, docker, "rm", "-f", v.ID).CombinedOutput(); err != nil {
		t.Errorf("remove validated fixture: %v", err)
		return
	}
	for _, id := range []string{v.ID, name} {
		if _, exists, err := inspectFixture(ctx, docker, id); err != nil || exists {
			t.Errorf("fixture absence unverified: %s %v", id, err)
			return
		}
	}
	t.Logf("validated fixture removed and absent: %s %s; attached volume IDs []", v.ID, name)
}

type fixture struct {
	t                          *testing.T
	name, password, root, port string
	spec                       persistence.Specification
	ownerRoot                  string
	evidence                   string
}

func (f fixture) command(sqlText, role string, params ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"exec", "-i", "-e", "PGPASSWORD=" + f.password, f.name, "psql", "-X", "-qAt", "-h", "127.0.0.1", "-U", role, "-d", "justix_identity", "-v", "ON_ERROR_STOP=1"}
	args = append(args, params...)
	c := exec.CommandContext(ctx, "docker", args...)
	c.Stdin = strings.NewReader(sqlText)
	b, err := c.CombinedOutput()
	return strings.TrimSpace(string(b)), err
}
func (f fixture) must(statement string, role ...string) string {
	f.t.Helper()
	r := "justix_identity"
	if len(role) > 0 {
		r = role[0]
	}
	out, err := f.command(statement, r)
	if err != nil {
		f.t.Fatalf("fixture SQL: %v %s", err, out)
	}
	return out
}
func (f fixture) read(name string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.root, name))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}
func (f fixture) install(name string, extra ...string) {
	f.t.Helper()
	args := []string{"-v", "owner_service=identity", "-v", "runtime_role=justix_identity_runtime"}
	for _, v := range extra {
		args = append(args, "-v", v)
	}
	out, err := f.command(f.read(name), "justix_identity", args...)
	if err != nil {
		f.t.Fatalf("install %s: %v %s", name, err, out)
	}
}
func hx(d persistence.Digest) string { return hex.EncodeToString(d[:]) }

func startFixture(t *testing.T) fixture {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{t: t, name: "justixauto-t932-" + uuid.NewString(), password: "synthetic-" + uuid.NewString(), root: root}
	f.evidence, err = os.MkdirTemp("", "justixauto-t932-evidence-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("retained synthetic evidence directory %s", f.evidence)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	token := uuid.NewString()
	if _, exists, err := inspectFixture(ctx, "docker", f.name); err != nil || exists {
		t.Fatalf("fresh fixture name not proven absent: %v", err)
	}
	// Register BEFORE allocation: a failed CLI reply does not prove no object.
	t.Cleanup(func() { cleanupFixture(t, "docker", f.name, token, root) })
	args := []string{"create", "--name", f.name, "--label", fixtureLabel + "=" + token, "--tmpfs", "/var/lib/postgresql:rw", "-p", "127.0.0.1::5432", "-e", "POSTGRES_PASSWORD=" + f.password, "-v", filepath.Join(root, "infra/local/postgres/init-owners.sql") + ":/docker-entrypoint-initdb.d/10-owners.sql:ro"}
	for _, o := range []string{"identity", "inventory", "commerce", "retail", "financing", "insurance", "documents"} {
		args = append(args, "-e", "JUSTIXAUTO_"+strings.ToUpper(o)+"_DB_PASSWORD="+f.password)
	}
	args = append(args, pgImage)
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("create owned fixture: %v %s", err, out)
	}
	v, exists, err := inspectFixture(ctx, "docker", f.name)
	if err != nil || !exists {
		t.Fatalf("created fixture identity unavailable: %v", err)
	}
	if err := v.validate(f.name, token, root); err != nil {
		t.Fatal(err)
	}
	t.Logf("created owned fixture %s id %s image %s; captured attached volume IDs []", f.name, v.ID, pgImage)
	if out, err := exec.CommandContext(ctx, "docker", "start", f.name).CombinedOutput(); err != nil {
		t.Fatalf("start owned fixture: %v %s", err, out)
	}
	for {
		if _, err := f.command("SELECT 1", "justix_identity"); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("fixture start timeout")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if f.must("SHOW server_version_num", "postgres") != "180006" {
		t.Fatal("wrong PostgreSQL version")
	}
	out, err = exec.CommandContext(ctx, "docker", "port", f.name, "5432/tcp").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	f.port = strings.TrimPrefix(strings.TrimSpace(string(out)), "127.0.0.1:")
	f.must("CREATE ROLE justix_identity_runtime LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_identity TO justix_identity_runtime; REVOKE TEMP ON DATABASE justix_identity FROM PUBLIC;", "postgres")
	f.install("pkg/eventstore/schema.sql")
	// Existing function REVOKEs do not alter built-in creation defaults. The
	// compatible path requires explicit private defaults for future objects.
	f.must("ALTER DEFAULT PRIVILEGES REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC;ALTER DEFAULT PRIVILEGES REVOKE USAGE ON TYPES FROM PUBLIC")
	f.ownerRoot, err = filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f.buildBundle()
	return f
}

func testHash(s string) persistence.Digest { return persistence.Digest(sha256.Sum256([]byte(s))) }
func (f *fixture) buildBundle() {
	f.t.Helper()
	base := persistence.Artifact{Identity: persistence.ArtifactIdentity{Version: 1, Filename: "migrations/0001_mechanics.up.sql", SHA256: testHash(f.read("services/identity/migrations/0001_mechanics.up.sql"))}}
	f.spec = persistence.Specification{Owner: "identity", Database: "justix_identity", RuntimeRole: "justix_identity_runtime", FormatRevision: 1, Head: 17, HistoryRevision: 1, HistorySHA256: testHash(f.read("pkg/eventstore/owner_history.sql")), Baseline: persistence.Baseline{Kind: "verified-installation", EvidenceRef: "synthetic-exact-base-execution", ApprovalRef: "synthetic-test-approval", BackupRef: "disposable-empty-fixture", StoppedRuntimesRef: "no-runtime-processes", RequestID: uuid.New()}, Artifacts: []persistence.Artifact{base}, Shared: persistence.Shared{Mode: "legacy", Artifacts: []persistence.SharedArtifact{{Revision: 1, Filename: "pkg/eventstore/schema.sql", SHA256: testHash(f.read("pkg/eventstore/schema.sql"))}}}}
	if err := os.MkdirAll(filepath.Join(f.ownerRoot, "migrations"), 0700); err != nil {
		f.t.Fatal(err)
	}
	f.writeArtifact(0, f.read("services/identity/migrations/0001_mechanics.up.sql"))
	for _, v := range []int64{12, 17} {
		feature := persistence.FeatureIdentity{ID: fmt.Sprintf("synthetic.feature%d", v), Revision: 1, SHA256: testHash(fmt.Sprintf("synthetic-contract-%d", v))}
		a := persistence.Artifact{Identity: persistence.ArtifactIdentity{Version: v, Filename: fmt.Sprintf("migrations/%04d_synthetic.up.sql", v)}, Prerequisites: []persistence.ArtifactIdentity{f.spec.Artifacts[len(f.spec.Artifacts)-1].Identity}, Feature: &feature}
		f.spec.Artifacts = append(f.spec.Artifacts, a)
		schema := fmt.Sprintf("synthetic_feature%d", v)
		f.spec.Features = append(f.spec.Features, persistence.Feature{Identity: feature, Tables: []persistence.TableGrant{{Schema: schema, Table: "items", Select: true, InsertColumns: []string{"id", "body"}, UpdateColumns: []string{"body"}}}})
		sql := fmt.Sprintf(`BEGIN;
CREATE SCHEMA %s; REVOKE ALL ON SCHEMA %s FROM PUBLIC; GRANT USAGE ON SCHEMA %s TO justix_identity_runtime;
CREATE TABLE %s.items(id uuid PRIMARY KEY,body text NOT NULL,immutable text NOT NULL DEFAULT 'retained;literal');
GRANT SELECT ON %s.items TO justix_identity_runtime;
GRANT INSERT(id,body),UPDATE(body) ON %s.items TO justix_identity_runtime;
INSERT INTO %s.items(id,body) VALUES('%s','retained; body');
DO $body$ BEGIN PERFORM 'semicolon;inside'; END $body$;
INSERT INTO owner_migrations.feature_contracts VALUES('%s',1,%d,decode('%s','hex'));
COMMIT;`, schema, schema, schema, schema, schema, schema, schema, uuid.NewString(), feature.ID, v, hx(feature.SHA256))
		f.writeArtifact(len(f.spec.Artifacts)-1, sql)
	}
}
func (f *fixture) writeArtifact(index int, sql string) {
	f.t.Helper()
	a := &f.spec.Artifacts[index]
	a.Identity.SHA256 = testHash(sql)
	manifest, err := json.Marshal(persistence.ArtifactManifest{FormatRevision: 1, Owner: "identity", Identity: a.Identity, Prerequisites: a.Prerequisites, Feature: a.Feature})
	if err != nil {
		f.t.Fatal(err)
	}
	a.ManifestSHA256 = persistence.Digest(sha256.Sum256(manifest))
	for name, b := range map[string][]byte{a.Identity.Filename: []byte(sql), strings.TrimSuffix(a.Identity.Filename, ".up.sql") + ".manifest.json": manifest} {
		if err := os.WriteFile(filepath.Join(f.ownerRoot, name), b, 0600); err != nil {
			f.t.Fatal(err)
		}
	}
}
func (f fixture) config(head int64, fresh bool) Config {
	s := f.spec
	n := 0
	for _, a := range s.Artifacts {
		if a.Identity.Version <= head {
			n++
		}
	}
	s.Head = head
	s.Artifacts = s.Artifacts[:n]
	s.Features = append([]persistence.Feature(nil), s.Features[:n-1]...)
	p, err := persistence.NewProfile(s)
	if err != nil {
		f.t.Fatal(err)
	}
	return Config{Profile: p, RepoRoot: f.root, OwnerRoot: f.ownerRoot, ProvenanceRef: "synthetic-owned-driver-execution", FreshBootstrap: fresh}
}
func (f fixture) connect(fault *wireFault) *pgx.Conn {
	f.t.Helper()
	cfg, err := pgx.ParseConfig(fmt.Sprintf("host=127.0.0.1 port=%s user=justix_identity password=%s dbname=justix_identity sslmode=disable", f.port, f.password))
	if err != nil {
		f.t.Fatal(err)
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	if fault != nil {
		cfg.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
			c, err := (&net.Dialer{}).DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			// pgx also invokes DialFunc for its separate CancelRequest socket.
			// Never replace the primary wrapper's transport or return that same
			// wrapper for a second connection while cleanup is using the first.
			fault.mu.Lock()
			defer fault.mu.Unlock()
			if fault.Conn == nil {
				fault.Conn = c
				return fault, nil
			}
			return c, nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}
func (f fixture) driver(cfg Config, fault *wireFault) *Driver {
	f.t.Helper()
	d, err := NewDriver(context.Background(), f.connect(fault), cfg)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { _ = d.Close() })
	return d
}

// Test-only immutable in-memory source exercises the actual approved engine.
// The production verified source and command composition remain T-933.
type memorySource struct {
	spec   persistence.Specification
	bodies map[uint]string
}

func (*memorySource) Open(string) (source.Driver, error) { return nil, ErrUnsupported }
func (*memorySource) Close() error                       { return nil }
func (s *memorySource) First() (uint, error) {
	if len(s.spec.Artifacts) == 0 {
		return 0, os.ErrNotExist
	}
	return uint(s.spec.Artifacts[0].Identity.Version), nil
}
func (s *memorySource) Next(v uint) (uint, error) {
	for _, a := range s.spec.Artifacts {
		if uint(a.Identity.Version) > v {
			return uint(a.Identity.Version), nil
		}
	}
	return 0, os.ErrNotExist
}
func (*memorySource) Prev(uint) (uint, error) { return 0, os.ErrNotExist }
func (s *memorySource) ReadUp(v uint) (io.ReadCloser, string, error) {
	b, ok := s.bodies[v]
	if !ok {
		return nil, "", os.ErrNotExist
	}
	return io.NopCloser(strings.NewReader(b)), fmt.Sprint(v), nil
}
func (*memorySource) ReadDown(uint) (io.ReadCloser, string, error) { return nil, "", os.ErrNotExist }
func (f fixture) engine(d *Driver) *migrate.Migrate {
	f.t.Helper()
	src := &memorySource{spec: d.spec, bodies: map[uint]string{}}
	for _, a := range d.spec.Artifacts {
		b, err := os.ReadFile(filepath.Join(f.ownerRoot, a.Identity.Filename))
		if err != nil {
			f.t.Fatal(err)
		}
		src.bodies[uint(a.Identity.Version)] = string(b)
	}
	m, err := migrate.NewWithInstance("verified-fixture", src, "owner-pgx", d)
	if err != nil {
		f.t.Fatal(err)
	}
	if m.LockTimeout <= LockBound {
		f.t.Fatal("engine timeout must exceed entire driver lock bound")
	}
	return m
}
func (f fixture) up(cfg Config) *Driver {
	f.t.Helper()
	d := f.driver(cfg, nil)
	if err := f.engine(d).Up(); err != nil {
		f.t.Fatal(err)
	}
	_ = d.Close()
	return d
}
func (f fixture) bootstrap() {
	f.t.Helper()
	f.up(f.config(1, true))
	f.install("pkg/eventstore/owner_history.sql", "template_sha256="+hx(f.spec.HistorySHA256), "base_schema_sha256="+hx(f.spec.Shared.Artifacts[0].SHA256), "base_artifact_sha256="+hx(f.spec.Artifacts[0].Identity.SHA256), "base_manifest_sha256="+hx(f.spec.Artifacts[0].ManifestSHA256), "profile_sha256="+hx(f.config(1, true).Profile.Digest()), "baseline_kind="+f.spec.Baseline.Kind, "evidence_ref="+f.spec.Baseline.EvidenceRef, "approval_ref="+f.spec.Baseline.ApprovalRef, "backup_ref="+f.spec.Baseline.BackupRef, "stopped_runtimes_ref="+f.spec.Baseline.StoppedRuntimesRef, "request_id="+f.spec.Baseline.RequestID.String())
}
func (f fixture) snapshot() string {
	return f.must(`SELECT jsonb_build_object('ledger',(SELECT jsonb_agg(to_jsonb(x) ORDER BY version) FROM public.schema_migrations x),'baseline',(SELECT jsonb_agg(to_jsonb(x)) FROM owner_migrations.compatibility x),'history',(SELECT jsonb_agg(to_jsonb(x) ORDER BY version) FROM owner_migrations.artifacts x),'features',(SELECT jsonb_agg(to_jsonb(x) ORDER BY migration_version) FROM owner_migrations.feature_contracts x),'events',(SELECT jsonb_agg(to_jsonb(x) ORDER BY event_id) FROM eventstore.events x),'command_receipts',(SELECT jsonb_agg(to_jsonb(x) ORDER BY receipt_id) FROM eventstore.command_receipts x))`)
}
func (f fixture) retain(name, data string) {
	f.t.Helper()
	b := []byte(data)
	path := filepath.Join(f.evidence, name)
	if err := os.WriteFile(path, b, 0600); err != nil {
		f.t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	if err := os.WriteFile(path+".sha256", []byte(fmt.Sprintf("%x  %s\n", sum, name)), 0600); err != nil {
		f.t.Fatal(err)
	}
}

// Discard an ACTUAL server CommandComplete reply only after PostgreSQL has
// sent COMMIT/ROLLBACK. This is not an injected error before tx.Commit. Native
// pgx still executes all SQL, then sees an EOF from the owned connection.
type wireFault struct {
	net.Conn
	mu        sync.Mutex
	command   string
	remaining int
	hit       bool
	incoming  []byte
}

func (f *wireFault) arm(command string, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.command = command
	f.remaining = n
	f.hit = false
	f.incoming = nil
}
func (f *wireFault) Read(b []byte) (int, error) {
	n, err := f.Conn.Read(b)
	if n == 0 {
		return n, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.incoming = append(f.incoming, b[:n]...)
	for len(f.incoming) >= 5 {
		size := int(binary.BigEndian.Uint32(f.incoming[1:5])) + 1
		if size < 5 || size > 1<<24 {
			return n, errors.New("invalid server fixture frame")
		}
		if len(f.incoming) < size {
			break
		}
		frame := f.incoming[:size]
		if frame[0] == 'C' && string(frame[5:len(frame)-1]) == f.command && f.remaining > 0 {
			f.remaining--
			if f.remaining == 0 {
				f.hit = true
				_ = f.Conn.Close()
				return 0, io.ErrUnexpectedEOF
			}
		}
		f.incoming = f.incoming[size:]
	}
	return n, err
}
func (f *wireFault) wasHit() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.hit }

func TestDriverActualEngineUpgradeAndRetainedHistory(t *testing.T) {
	f := startFixture(t)
	f.bootstrap()
	f.must(fmt.Sprintf(`INSERT INTO eventstore.events(event_id,event_type,schema_version,aggregate_type,aggregate_id,aggregate_version,company_id,occurred_at,actor_kind,actor_id,correlation_id,causation_id,operation_id,data)
 VALUES('%s','identity.fixture.recorded.v1',1,'fixture','%s',1,NULL,'2026-09-16T00:00:00Z','system','%s','%s','%s','%s','{"retained":true}');
 INSERT INTO eventstore.command_receipts(receipt_id,actor_id,company_id,command_name,idempotency_key,request_hash,http_status,receipt)
 VALUES('%s','%s',NULL,'synthetic.fixture','%s',decode('%s','hex'),200,'{"retained":true}');`, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), hx(testHash("synthetic-request"))))
	baseRows := f.must(`SELECT jsonb_build_object('events',(SELECT jsonb_agg(to_jsonb(x) ORDER BY event_id) FROM eventstore.events x),'receipts',(SELECT jsonb_agg(to_jsonb(x) ORDER BY receipt_id) FROM eventstore.command_receipts x))`)
	f.retain("baseline-rows.json", baseRows)
	f.retain("base.json", f.snapshot())
	twelve := f.up(f.config(12, false))
	prior := f.must(`SELECT row_to_json(a) FROM owner_migrations.artifacts a WHERE version=12`)
	rows := f.must(`SELECT jsonb_agg(to_jsonb(i)) FROM synthetic_feature12.items i`)
	f.retain("feature12-before.json", rows)
	f.retain("at12.json", f.snapshot())
	f.up(f.config(17, false))
	if _, err := Reconcile(context.Background(), f.connect(nil), f.config(12, false), twelve.LastAttempt()); err == nil {
		t.Fatal("newer head accepted as exact older attempt")
	}
	if got := f.must(`SELECT row_to_json(a) FROM owner_migrations.artifacts a WHERE version=12`); got != prior {
		t.Fatal("old installing request/profile/time rewritten")
	}
	if got := f.must(`SELECT jsonb_agg(to_jsonb(i)) FROM synthetic_feature12.items i`); got != rows {
		t.Fatal("retained row changed")
	}
	f.retain("feature12-after.json", f.must(`SELECT jsonb_agg(to_jsonb(i)) FROM synthetic_feature12.items i`))
	if got := f.must(`SELECT jsonb_build_object('events',(SELECT jsonb_agg(to_jsonb(x) ORDER BY event_id) FROM eventstore.events x),'receipts',(SELECT jsonb_agg(to_jsonb(x) ORDER BY receipt_id) FROM eventstore.command_receipts x))`); got != baseRows {
		t.Fatal("populated version 1 mechanics changed during upgrade")
	}
	before := f.snapshot()
	d := f.driver(f.config(17, false), nil)
	if err := f.engine(d).Up(); !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("no-op: %v", err)
	}
	if f.snapshot() != before {
		t.Fatal("no-op wrote history")
	}
	f.retain("at17.json", before)
	if _, err := d.Open("postgres://ignored"); !errors.Is(err, ErrUnsupported) {
		t.Fatal("Open supported")
	}
	if !errors.Is(d.Drop(), ErrUnsupported) {
		t.Fatal("Drop supported")
	}
	if err := f.engine(d).Force(1); err == nil {
		t.Fatal("Force accepted")
	}
	if f.snapshot() != before {
		t.Fatal("Force changed history")
	}
}

func TestDriverDirtyOriginalBytesAndAtomicFinalization(t *testing.T) {
	for _, mode := range []string{"sql-error", "unclosed-transaction", "wrong-source", "empty-source", "missing-run", "missing-marker", "missing-required-grant", "released-owner-lock", "receipt-failure", "ignored-receipt-insert", "clean-failure", "ignored-clean-update", "lost-dirty-commit", "lost-ddl-commit", "lost-final-commit", "lost-rollback"} {
		t.Run(mode, func(t *testing.T) {
			f := startFixture(t)
			f.bootstrap()
			cfg := f.config(12, false)
			if mode == "sql-error" || mode == "lost-rollback" {
				f.writeArtifact(1, "BEGIN; SELECT 1/0; COMMIT;")
				cfg = f.config(12, false)
			}
			if mode == "unclosed-transaction" {
				f.writeArtifact(1, "BEGIN; SELECT 1;")
				cfg = f.config(12, false)
			}
			if mode == "missing-marker" {
				f.writeArtifact(1, "BEGIN; SELECT 1; COMMIT;")
				cfg = f.config(12, false)
			}
			if mode == "missing-required-grant" {
				path := filepath.Join(f.ownerRoot, f.spec.Artifacts[1].Identity.Filename)
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				f.writeArtifact(1, strings.Replace(string(b), "GRANT INSERT(id,body),UPDATE(body)", "GRANT INSERT(id,body)", 1))
				cfg = f.config(12, false)
			}
			if mode == "released-owner-lock" {
				path := filepath.Join(f.ownerRoot, f.spec.Artifacts[1].Identity.Filename)
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				f.writeArtifact(1, string(b)+fmt.Sprintf("\nSELECT pg_advisory_unlock(%d);", OwnerLockKey("justix_identity")))
				cfg = f.config(12, false)
			}
			fault := &wireFault{}
			d := f.driver(cfg, fault)
			if mode == "receipt-failure" {
				f.must(`ALTER TABLE owner_migrations.artifacts ADD CONSTRAINT synthetic_reject_receipt CHECK(version<>12)`)
			}
			if mode == "ignored-receipt-insert" {
				f.must(`CREATE FUNCTION public.synthetic_ignore_receipt() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RETURN NULL; END$$; CREATE TRIGGER synthetic_ignore_receipt BEFORE INSERT ON owner_migrations.artifacts FOR EACH ROW EXECUTE FUNCTION public.synthetic_ignore_receipt()`)
			}
			if mode == "clean-failure" {
				f.must(`CREATE FUNCTION public.synthetic_clean_failure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.version=12 AND NOT NEW.dirty THEN RAISE EXCEPTION 'synthetic finalization failure'; END IF; RETURN NEW; END$$; CREATE TRIGGER synthetic_clean_failure BEFORE UPDATE ON public.schema_migrations FOR EACH ROW EXECUTE FUNCTION public.synthetic_clean_failure()`)
			}
			if mode == "ignored-clean-update" {
				f.must(`CREATE FUNCTION public.synthetic_clean_failure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.version=12 AND NOT NEW.dirty THEN RETURN NULL; END IF; RETURN NEW; END$$; CREATE TRIGGER synthetic_clean_failure BEFORE UPDATE ON public.schema_migrations FOR EACH ROW EXECUTE FUNCTION public.synthetic_clean_failure()`)
			}
			f.retain("before.json", f.snapshot())
			if mode == "lost-dirty-commit" {
				fault.arm("COMMIT", 1)
			}
			if mode == "lost-final-commit" {
				fault.arm("COMMIT", 3)
			}
			if mode == "lost-ddl-commit" {
				fault.arm("COMMIT", 2)
			}
			if mode == "lost-rollback" {
				fault.arm("ROLLBACK", 1)
			}
			var err error
			if mode == "wrong-source" || mode == "empty-source" || mode == "missing-run" {
				if err = d.Lock(); err != nil {
					t.Fatal(err)
				}
				if err = d.SetVersion(12, true); err != nil {
					t.Fatal(err)
				}
				if mode == "wrong-source" {
					err = d.Run(strings.NewReader("SELECT 1"))
				} else if mode == "empty-source" {
					err = d.Run(strings.NewReader(""))
				} else {
					err = d.SetVersion(12, false)
				}
			} else {
				err = f.engine(d).Up()
			}
			if err == nil {
				t.Fatal("failure accepted")
			}
			a := d.LastAttempt()
			_ = d.Close()
			if strings.HasPrefix(mode, "lost-") && !fault.wasHit() {
				t.Fatal("actual server reply fault was not reached")
			}
			f.retain("after.json", f.snapshot())
			ledger := f.must(`SELECT version||':'||dirty FROM public.schema_migrations`)
			count := f.must(`SELECT count(*) FROM owner_migrations.artifacts WHERE version=12`)
			if mode == "lost-final-commit" {
				if ledger != "12:false" || count != "1" {
					t.Fatalf("atomic completion absent: %s/%s", ledger, count)
				}
			} else {
				if ledger != "12:true" || count != "0" {
					t.Fatalf("dirty/receipt pair: %s/%s", ledger, count)
				}
			}
			if mode == "clean-failure" || mode == "ignored-clean-update" {
				f.must(`DROP TRIGGER synthetic_clean_failure ON public.schema_migrations; DROP FUNCTION public.synthetic_clean_failure()`)
			}
			out, reconcileErr := Reconcile(context.Background(), f.connect(nil), cfg, a)
			if mode == "missing-required-grant" {
				if reconcileErr == nil {
					t.Fatal("invalid grant reconciliation accepted")
				}
				return
			}
			if reconcileErr != nil {
				t.Fatal(reconcileErr)
			}
			want := RetainedIncomplete
			if mode == "lost-final-commit" {
				want = RecordedCompletion
			}
			if out != want {
				t.Fatalf("reconcile: %s", out)
			}
			if mode == "lost-final-commit" {
				wrong := a
				wrong.request = uuid.New()
				if _, err := Reconcile(context.Background(), f.connect(nil), cfg, wrong); err == nil {
					t.Fatal("different request accepted as completion")
				}
				wrong = a
				wrong.sql = testHash("different SQL")
				if _, err := Reconcile(context.Background(), f.connect(nil), cfg, wrong); err == nil {
					t.Fatal("different SQL accepted as completion")
				}
			}
			if err := d.SetVersion(12, false); err == nil {
				t.Fatal("closed failed driver reused")
			}
		})
	}
}

func TestDriverCompleteBundlePreflightAndFreshBoundary(t *testing.T) {
	f := startFixture(t)
	// Construction is inert: no ledger or metadata is created until Lock.
	d := f.driver(f.config(1, true), nil)
	if f.must(`SELECT to_regclass('public.schema_migrations') IS NULL`) != "t" {
		t.Fatal("constructor mutated DB")
	}
	_ = d.Close()
	f.up(f.config(1, true))
	before := f.must(`SELECT jsonb_agg(to_jsonb(x)) FROM public.schema_migrations x`)
	for _, cfg := range []Config{f.config(1, true), f.config(12, false)} {
		d := f.driver(cfg, nil)
		if err := f.engine(d).Up(); err == nil {
			t.Fatal("fresh/history boundary bypassed")
		}
	}
	if f.must(`SELECT jsonb_agg(to_jsonb(x)) FROM public.schema_migrations x`) != before {
		t.Fatal("existing base changed")
	}
	f.install("pkg/eventstore/owner_history.sql", "template_sha256="+hx(f.spec.HistorySHA256), "base_schema_sha256="+hx(f.spec.Shared.Artifacts[0].SHA256), "base_artifact_sha256="+hx(f.spec.Artifacts[0].Identity.SHA256), "base_manifest_sha256="+hx(f.spec.Artifacts[0].ManifestSHA256), "profile_sha256="+hx(f.config(1, true).Profile.Digest()), "baseline_kind="+f.spec.Baseline.Kind, "evidence_ref="+f.spec.Baseline.EvidenceRef, "approval_ref="+f.spec.Baseline.ApprovalRef, "backup_ref="+f.spec.Baseline.BackupRef, "stopped_runtimes_ref="+f.spec.Baseline.StoppedRuntimesRef, "request_id="+f.spec.Baseline.RequestID.String())
	f.up(f.config(12, false))
	before = f.snapshot()
	f.retain("before-denials.json", before)
	for _, name := range []string{"changed-installed-sql", "changed-installed-manifest", "missing-installed-file", "installed-symlink", "multiple-ledger-rows", "downgrade", "missing-prior-profile"} {
		t.Run(name, func(t *testing.T) {
			f.t = t
			cfg := f.config(17, false)
			var engine *migrate.Migrate
			restore := func() {}
			a := f.spec.Artifacts[0].Identity
			path := filepath.Join(f.ownerRoot, a.Filename)
			switch name {
			case "changed-installed-sql", "changed-installed-manifest", "missing-installed-file", "installed-symlink":
				engine = f.engine(f.driver(cfg, nil))
				if name == "changed-installed-manifest" {
					path = filepath.Join(f.ownerRoot, strings.TrimSuffix(a.Filename, ".up.sql")+".manifest.json")
				}
				original, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				restore = func() {
					_ = os.Remove(path)
					if err := os.WriteFile(path, original, 0600); err != nil {
						t.Fatal(err)
					}
				}
				defer restore()
				if name == "missing-installed-file" || name == "installed-symlink" {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					if name == "installed-symlink" {
						target := path + ".target"
						if err := os.WriteFile(target, original, 0600); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(target, path); err != nil {
							t.Fatal(err)
						}
					}
				} else {
					if err := os.WriteFile(path, append(original, ' '), 0600); err != nil {
						t.Fatal(err)
					}
				}
			case "multiple-ledger-rows":
				f.must(`INSERT INTO public.schema_migrations VALUES(99,false)`)
				defer f.must(`DELETE FROM public.schema_migrations WHERE version=99`)
			case "downgrade":
				cfg = f.config(1, false)
			case "missing-prior-profile":
				s := f.spec
				s.Artifacts = []persistence.Artifact{s.Artifacts[0], s.Artifacts[2]}
				s.Artifacts[1].Prerequisites = []persistence.ArtifactIdentity{s.Artifacts[0].Identity}
				s.Features = s.Features[1:]
				var err error
				cfg.Profile, err = persistence.NewProfile(s)
				if err != nil {
					t.Fatal(err)
				}
			}
			pre := f.snapshot()
			if engine == nil {
				engine = f.engine(f.driver(cfg, nil))
			}
			if err := engine.Up(); err == nil {
				t.Fatal("incomplete bundle accepted")
			}
			if f.snapshot() != pre {
				t.Fatal("preflight denial wrote database")
			}
		})
	}
	f.t = t
	if f.snapshot() != before {
		t.Fatal("denial did not preserve installation")
	}
	f.retain("after-denials.json", f.snapshot())
}

func TestDriverLowerVersionInsertionRejected(t *testing.T) {
	f := startFixture(t)
	full := f.spec
	seventeen := f.spec.Artifacts[2]
	seventeen.Prerequisites = []persistence.ArtifactIdentity{f.spec.Artifacts[0].Identity}
	f.spec.Artifacts = []persistence.Artifact{f.spec.Artifacts[0], seventeen}
	f.spec.Features = f.spec.Features[1:]
	sql, err := os.ReadFile(filepath.Join(f.ownerRoot, seventeen.Identity.Filename))
	if err != nil {
		t.Fatal(err)
	}
	f.writeArtifact(1, string(sql))
	f.bootstrap()
	f.up(f.config(17, false))
	before := f.snapshot()
	f.retain("sequence-1-17.json", before)
	f.spec = full
	f.writeArtifact(2, string(sql))
	d := f.driver(f.config(17, false), nil)
	if err := f.engine(d).Up(); err == nil || errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("new lower item silently skipped: %v", err)
	}
	if f.snapshot() != before {
		t.Fatal("lower version rejection changed retained history")
	}
	f.retain("lower-insertion-denied.json", f.snapshot())
}

func TestDriverBoundedLockAndPreflight(t *testing.T) {
	f := startFixture(t)
	f.bootstrap()
	cfg := f.config(12, false)
	holder := f.connect(nil)
	if _, err := holder.Exec(context.Background(), `SELECT pg_advisory_lock($1)`, OwnerLockKey("justix_identity")); err != nil {
		t.Fatal(err)
	}
	d := f.driver(cfg, nil)
	start := time.Now()
	err := f.engine(d).Up()
	elapsed := time.Since(start)
	if err == nil || elapsed < 4*time.Second || elapsed > 7*time.Second {
		t.Fatalf("lock bound: %v %s", err, elapsed)
	}
	// An expired query may have closed pgx asynchronously; IsClosed alone is
	// not server release evidence. Keep the blocker held while observing bounded
	// backend disappearance, so a pending server request cannot win a race with
	// our subsequent unlock. A live, idle connection took the known-no-lock poll
	// timeout path and must have no acquisition work or held lock left instead.
	if d.conn.IsClosed() {
		observeBackendExit(t, holder, d.conn.PgConn().PID())
	} else {
		var idleWithoutLock bool
		if err := holder.QueryRow(context.Background(), `SELECT EXISTS(SELECT FROM pg_stat_activity WHERE pid=$1 AND state='idle') AND NOT EXISTS(SELECT FROM pg_locks WHERE pid=$1 AND locktype='advisory' AND granted)`, d.conn.PgConn().PID()).Scan(&idleWithoutLock); err != nil || !idleWithoutLock || d.conn.PgConn().IsBusy() || d.locked {
			t.Fatalf("known-no-lock timeout still active: %v idle=%v", err, idleWithoutLock)
		}
	}
	if _, err := holder.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, OwnerLockKey("justix_identity")); err != nil {
		t.Fatal(err)
	}
	// An expired acquisition must never acquire the lock later in a goroutine.
	var acquired bool
	if err := holder.QueryRow(context.Background(), `SELECT pg_try_advisory_lock($1)`, OwnerLockKey("justix_identity")).Scan(&acquired); err != nil {
		t.Fatalf("observer reacquisition query failed: %v", err)
	}
	if !acquired {
		t.Fatal("late acquisition retained lock after backend/idle confirmation")
	}
	_, _ = holder.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, OwnerLockKey("justix_identity"))
	// Lock's bound also covers blocked compatibility queries after acquisition.
	tx, err := holder.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(context.Background(), `LOCK TABLE owner_migrations.compatibility IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	d = f.driver(cfg, nil)
	start = time.Now()
	err = f.engine(d).Up()
	elapsed = time.Since(start)
	if err == nil || elapsed > 7*time.Second {
		t.Fatalf("preflight not bounded: %v %s", err, elapsed)
	}
	_ = tx.Rollback(context.Background())
	if !d.conn.IsClosed() {
		t.Fatal("uncertain lock session remained open")
	}
	observeBackendExit(t, holder, d.conn.PgConn().PID())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d, err = NewDriver(ctx, f.connect(nil), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Lock(); err == nil {
		t.Fatal("canceled parent accepted")
	}
	_ = d.Close()
}

func observeBackendExit(t *testing.T, observer *pgx.Conn, pid uint32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for {
		var gone bool
		if err := observer.QueryRow(ctx, `SELECT NOT EXISTS(SELECT FROM pg_stat_activity WHERE pid=$1) AND NOT EXISTS(SELECT FROM pg_locks WHERE pid=$1 AND locktype='advisory')`, pid).Scan(&gone); err != nil {
			t.Fatalf("bounded server release observation for pid %d: %v", pid, err)
		}
		if gone {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("backend %d survived bounded transport cleanup", pid)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestDriverOwnerValidatorRuntimeCheckerParity(t *testing.T) {
	f := startFixture(t)
	f.bootstrap()
	f.up(f.config(12, false))
	cfg := f.config(12, false)
	runtime, err := gorm.Open(postgres.Open(fmt.Sprintf("host=127.0.0.1 port=%s user=justix_identity_runtime password=%s dbname=justix_identity sslmode=disable", f.port, f.password)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := runtime.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	check := func(want bool) {
		t.Helper()
		runtimeErr := persistence.Check(context.Background(), runtime, cfg.Profile)
		d := f.driver(cfg, nil)
		driverErr := d.Lock()
		_ = d.Close()
		if (runtimeErr == nil) != want || (driverErr == nil) != want {
			t.Fatalf("checker parity want %v: runtime %v owner %v", want, runtimeErr, driverErr)
		}
	}
	check(true)
	before := f.snapshot()
	f.retain("before-grant-drift.json", before)
	cases := []struct{ name, change, restore string }{
		{"missing-required-column", "REVOKE UPDATE(body) ON synthetic_feature12.items FROM justix_identity_runtime", "GRANT UPDATE(body) ON synthetic_feature12.items TO justix_identity_runtime"},
		{"broad-update", "GRANT UPDATE ON synthetic_feature12.items TO justix_identity_runtime", "REVOKE UPDATE ON synthetic_feature12.items FROM justix_identity_runtime; GRANT UPDATE(body) ON synthetic_feature12.items TO justix_identity_runtime"},
		{"public-select", "GRANT SELECT ON synthetic_feature12.items TO PUBLIC", "REVOKE SELECT ON synthetic_feature12.items FROM PUBLIC"},
		{"delegable-column", "GRANT INSERT(id) ON synthetic_feature12.items TO justix_identity_runtime WITH GRANT OPTION", "REVOKE GRANT OPTION FOR INSERT(id) ON synthetic_feature12.items FROM justix_identity_runtime"},
		{"unsafe-default-function", "ALTER DEFAULT PRIVILEGES GRANT EXECUTE ON FUNCTIONS TO PUBLIC", "ALTER DEFAULT PRIVILEGES REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC"},
		{"unsafe-default-type", "ALTER DEFAULT PRIVILEGES GRANT USAGE ON TYPES TO PUBLIC", "ALTER DEFAULT PRIVILEGES REVOKE USAGE ON TYPES FROM PUBLIC"},
		{"disabled-history-guard", "ALTER TABLE owner_migrations.artifacts DISABLE TRIGGER artifacts_immutable", "ALTER TABLE owner_migrations.artifacts ENABLE TRIGGER artifacts_immutable"},
	}
	// No parameter privileges or session_replication_role settings are changed.
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { f.t = t; f.must(tc.change); defer f.must(tc.restore); check(false) })
		f.t = t
		check(true)
	}
	t.Run("multihop-noinherit", func(t *testing.T) {
		f.t = t
		f.must(`CREATE ROLE t932_outer NOINHERIT; CREATE ROLE t932_inner NOINHERIT; GRANT t932_inner TO t932_outer; GRANT t932_outer TO justix_identity_runtime`, "postgres")
		f.must(`GRANT UPDATE(immutable) ON synthetic_feature12.items TO t932_inner`)
		defer f.must(`REVOKE t932_outer FROM justix_identity_runtime; DROP ROLE t932_outer; DROP ROLE t932_inner`, "postgres")
		defer f.must(`REVOKE UPDATE(immutable) ON synthetic_feature12.items FROM t932_inner`)
		check(false)
	})
	f.t = t
	check(true)
	if f.snapshot() != before {
		t.Fatal("read-only compatibility mutated evidence")
	}
	f.retain("after-grant-drift.json", f.snapshot())
}

func TestDriverPendingSequenceAndCanceledEnginePipe(t *testing.T) {
	f := startFixture(t)
	f.bootstrap()
	cfg := f.config(17, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d, err := NewDriver(ctx, f.connect(nil), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.Lock(); err != nil {
		t.Fatal(err)
	}
	before := f.snapshot()
	if err := d.SetVersion(17, true); err == nil {
		t.Fatal("skipped next artifact")
	}
	if err := d.SetVersion(12, false); err == nil {
		t.Fatal("cleaned without dirty/Run")
	}
	if f.snapshot() != before {
		t.Fatal("invalid sequence changed ledger")
	}
	if err := d.SetVersion(12, true); err != nil {
		t.Fatal(err)
	}
	if err := d.SetVersion(17, true); err == nil {
		t.Fatal("overlapping pending attempt")
	}
	reader, writer := io.Pipe()
	defer writer.Close()
	finished := make(chan error, 1)
	go func() { finished <- d.Run(reader) }()
	cancel()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("canceled source pipe succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("engine source pipe cancellation unbounded")
	}
	if !d.conn.IsClosed() {
		t.Fatal("canceled pending connection not closed")
	}
	if got := f.must(`SELECT version||':'||dirty FROM public.schema_migrations`); got != "12:true" {
		t.Fatal("cancel lost dirty state")
	}
	f.retain("canceled-source.json", f.snapshot())
}

// A lost actual acquisition reply makes pgx start asynchronous cleanup. Hold
// only its separate cancel dial, never the owned PostgreSQL socket: this proves
// that the adapter itself closes its transport even when native IsClosed is
// already true and native Close consequently returns without doing so.
type cleanupProbeConn struct {
	net.Conn
	mu             sync.Mutex
	armed, dropped bool
	closed         chan struct{}
	once           sync.Once
}

func (c *cleanupProbeConn) Write(b []byte) (int, error) {
	if len(b) > 5 && b[0] == 'Q' && strings.Contains(string(b[5:]), "SELECT pg_try_advisory_lock") {
		c.mu.Lock()
		c.armed = true
		c.mu.Unlock()
	}
	return c.Conn.Write(b)
}
func (c *cleanupProbeConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.mu.Lock()
	defer c.mu.Unlock()
	if n > 0 && c.armed && !c.dropped {
		c.dropped = true
		return 0, io.ErrUnexpectedEOF
	}
	return n, err
}
func (c *cleanupProbeConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

func TestDriverPoisonClosesTransportDuringNativeAsyncCleanup(t *testing.T) {
	f := startFixture(t)
	f.bootstrap()
	cfg, err := pgx.ParseConfig(fmt.Sprintf("host=127.0.0.1 port=%s user=justix_identity password=%s dbname=justix_identity sslmode=disable", f.port, f.password))
	if err != nil {
		t.Fatal(err)
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	releaseCancel := make(chan struct{})
	defer close(releaseCancel)
	cancelStarted := make(chan struct{})
	var cancelOnce sync.Once
	var wire *cleanupProbeConn
	cfg.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		if wire != nil {
			cancelOnce.Do(func() { close(cancelStarted) })
			select {
			case <-releaseCancel:
				return nil, errors.New("synthetic cancel dial released")
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		raw, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		wire = &cleanupProbeConn{Conn: raw, closed: make(chan struct{})}
		return wire, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewDriver(context.Background(), conn, f.config(12, false))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	pid := conn.PgConn().PID()
	err = d.Lock()
	if err == nil || !errors.Is(err, ErrPoisoned) {
		t.Fatalf("lost acquisition must poison: %v", err)
	}
	select {
	case <-cancelStarted:
	case <-time.After(time.Second):
		t.Fatal("native asynchronous cleanup was not reached")
	}
	state := f.must(fmt.Sprintf(`SELECT jsonb_build_object('backend',EXISTS(SELECT FROM pg_stat_activity WHERE pid=%d),'advisory_locks',(SELECT count(*) FROM pg_locks WHERE pid=%d AND locktype='advisory' AND granted))`, pid, pid))
	f.retain("native-async-close-observation.json", state)
	select {
	case <-wire.closed:
	case <-time.After(10 * time.Millisecond):
		t.Fatalf("poison returned before owned transport closure; native IsClosed=%v; server=%s", conn.IsClosed(), state)
	}
	// No retry of Lock/SQL is needed. Verify the server has converged while the
	// separate native cancel dial is STILL blocked, then acquire with another
	// connection immediately. This establishes transport closure independently
	// of native asynchronous cleanup completion and excludes future acquisition.
	observer := f.connect(nil)
	observeBackendExit(t, observer, pid)
	var acquired bool
	if err := observer.QueryRow(context.Background(), `SELECT pg_try_advisory_lock($1)`, OwnerLockKey("justix_identity")).Scan(&acquired); err != nil || !acquired {
		t.Fatalf("closed poisoned backend retained owner lock: %v acquired=%v", err, acquired)
	}
	_, _ = observer.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, OwnerLockKey("justix_identity"))
	if d.LastAttempt().RequestID() != uuid.Nil {
		t.Fatal("failed acquisition reached dirty write")
	}
}
