package persistence_test

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"justixauto/pkg/persistence"
)

const pgImage = "docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"
const fixtureLabel = "justixauto.t930.fixture-owner"

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
	nameUUID, err := uuid.Parse(strings.TrimPrefix(name, "justixauto-t930-"))
	if err != nil || nameUUID == uuid.Nil || name != "justixauto-t930-"+nameUUID.String() {
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
	runtime                    *gorm.DB
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
	f := fixture{t: t, name: "justixauto-t930-" + uuid.NewString(), password: "synthetic-" + uuid.NewString(), root: root}
	f.evidence, err = os.MkdirTemp("", "justixauto-t930-evidence-")
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
	f.must("CREATE TABLE public.schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL); INSERT INTO public.schema_migrations VALUES(1,true)")
	f.must(f.read("services/identity/migrations/0001_mechanics.up.sql"))
	f.must("UPDATE public.schema_migrations SET dirty=false")
	f.spec = spec()
	f.spec.Artifacts[0].Identity.SHA256 = hash(f.read("services/identity/migrations/0001_mechanics.up.sql"))
	f.spec.Artifacts[1].Prerequisites[0] = f.spec.Artifacts[0].Identity
	f.spec.HistorySHA256 = hash(f.read("pkg/eventstore/owner_history.sql"))
	f.spec.Baseline.RequestID = uuid.New()
	f.spec.Shared.Artifacts[0].SHA256 = hash(f.read("pkg/eventstore/schema.sql"))
	f.install("pkg/eventstore/owner_history.sql", "template_sha256="+hx(f.spec.HistorySHA256), "base_schema_sha256="+hx(f.spec.Shared.Artifacts[0].SHA256), "base_artifact_sha256="+hx(f.spec.Artifacts[0].Identity.SHA256), "base_manifest_sha256="+hx(f.spec.Artifacts[0].ManifestSHA256), "profile_sha256="+hx(hash("historical-installing-bundle")), "baseline_kind="+f.spec.Baseline.Kind, "evidence_ref="+f.spec.Baseline.EvidenceRef, "approval_ref="+f.spec.Baseline.ApprovalRef, "backup_ref="+f.spec.Baseline.BackupRef, "stopped_runtimes_ref="+f.spec.Baseline.StoppedRuntimesRef, "request_id="+f.spec.Baseline.RequestID.String())
	db, err := gorm.Open(postgres.Open(fmt.Sprintf("host=127.0.0.1 port=%s user=justix_identity_runtime password=%s dbname=justix_identity sslmode=disable", f.port, f.password)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	f.runtime = db
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	baseSpec := f.spec
	baseSpec.Head = 1
	baseSpec.Artifacts = baseSpec.Artifacts[:1]
	baseSpec.Features = nil
	if err := persistence.Check(context.Background(), db, profile(t, baseSpec)); err != nil {
		t.Fatalf("explicit history + clean base profile: %v", err)
	}
	f.installFeature(f.spec.Artifacts[1], f.spec.Features[0])
	f.must(fmt.Sprintf(`INSERT INTO eventstore.events(event_id,event_type,schema_version,aggregate_type,aggregate_id,aggregate_version,company_id,occurred_at,actor_kind,actor_id,correlation_id,causation_id,operation_id,data)
 VALUES('%s','identity.fixture.recorded.v1',1,'fixture','%s',1,NULL,'2026-09-15T00:00:00Z','system','%s','%s','%s','%s','{"retained":true}');
 INSERT INTO eventstore.command_receipts(receipt_id,actor_id,company_id,command_name,idempotency_key,request_hash,http_status,receipt)
 VALUES('%s','%s',NULL,'synthetic.fixture','%s',decode('%s','hex'),200,'{"retained":true}');`, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), hx(hash("synthetic-nonsecret-request"))))
	return f
}

// Synthetic approved fixture schema/protocol only. Actual golang-migrate
// execution, Run ordering and commit-reply faults belong to T-932/T-933.
func (f fixture) installFeature(a persistence.Artifact, feature persistence.Feature) {
	g := feature.Tables[0]
	prior := a.Prerequisites[len(a.Prerequisites)-1]
	f.must(fmt.Sprintf(`BEGIN;UPDATE public.schema_migrations SET version=%d,dirty=true;COMMIT;
 BEGIN;CREATE SCHEMA IF NOT EXISTS %s; REVOKE ALL ON SCHEMA %s FROM PUBLIC; GRANT USAGE ON SCHEMA %s TO justix_identity_runtime;
 CREATE TABLE %s.%s(id uuid PRIMARY KEY,body text NOT NULL,immutable text NOT NULL DEFAULT 'retained');
 GRANT SELECT ON %s.%s TO justix_identity_runtime;GRANT INSERT(id,body),UPDATE(body) ON %s.%s TO justix_identity_runtime;
 INSERT INTO %s.%s(id,body) VALUES('%s','retained body');
 INSERT INTO owner_migrations.feature_contracts VALUES('%s',%d,%d,decode('%s','hex'));COMMIT;
 BEGIN;INSERT INTO owner_migrations.artifacts(version,filename,sql_sha256,predecessor_version,predecessor_sha256,feature_contract_id,feature_contract_revision,feature_contract_sha256,installation_request_id,artifact_manifest_sha256,installing_profile_sha256,provenance_kind,provenance_ref)
 VALUES(%d,'%s',decode('%s','hex'),%d,decode('%s','hex'),'%s',%d,decode('%s','hex'),'%s',decode('%s','hex'),decode('%s','hex'),'verified-installation','synthetic-feature-execution');
 UPDATE public.schema_migrations SET dirty=false;COMMIT;`, a.Identity.Version, g.Schema, g.Schema, g.Schema, g.Schema, g.Table, g.Schema, g.Table, g.Schema, g.Table, g.Schema, g.Table, uuid.NewString(), feature.Identity.ID, feature.Identity.Revision, a.Identity.Version, hx(feature.Identity.SHA256), a.Identity.Version, a.Identity.Filename, hx(a.Identity.SHA256), prior.Version, hx(prior.SHA256), feature.Identity.ID, feature.Identity.Revision, hx(feature.Identity.SHA256), uuid.NewString(), hx(a.ManifestSHA256), hx(hash("different-historical-profile"))))
}

func profile(t *testing.T, s persistence.Specification) persistence.Profile {
	t.Helper()
	p, err := persistence.NewProfile(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func (f fixture) snapshot() string {
	// Independent fixture evidence stays readable even while a test deliberately
	// changes a feature object's owner. Runtime checks still use only its role.
	return f.must(`SELECT jsonb_build_object('ledger',(SELECT jsonb_agg(to_jsonb(x) ORDER BY version) FROM public.schema_migrations x),'compatibility',(SELECT jsonb_agg(to_jsonb(x)) FROM owner_migrations.compatibility x),'history',(SELECT jsonb_agg(to_jsonb(x) ORDER BY version) FROM owner_migrations.artifacts x),'features',(SELECT jsonb_agg(to_jsonb(x) ORDER BY migration_version) FROM owner_migrations.feature_contracts x),'rows',(SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM synthetic_feature.items x),'events',(SELECT jsonb_agg(to_jsonb(x) ORDER BY event_id) FROM eventstore.events x),'receipts',(SELECT jsonb_agg(to_jsonb(x) ORDER BY receipt_id) FROM eventstore.command_receipts x),'catalog',(SELECT jsonb_agg(jsonb_build_object('oid',c.oid,'schema',n.nspname,'name',c.relname,'owner',c.relowner,'acl',c.relacl,'columns',(SELECT jsonb_agg(jsonb_build_object('number',a.attnum,'name',a.attname,'type',a.atttypid,'notnull',a.attnotnull,'acl',a.attacl) ORDER BY a.attnum) FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped),'constraints',(SELECT jsonb_agg(to_jsonb(k) ORDER BY k.oid) FROM pg_constraint k WHERE k.conrelid=c.oid),'triggers',(SELECT jsonb_agg(to_jsonb(g) ORDER BY g.oid) FROM pg_trigger g WHERE g.tgrelid=c.oid)) ORDER BY c.oid) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind='r' AND n.nspname IN ('public','eventstore','identity_mechanics','owner_migrations','synthetic_feature')))`, "postgres")
}
func (f fixture) retain(name, data string) {
	f.t.Helper()
	b := []byte(data + "\n")
	path := filepath.Join(f.evidence, name)
	if err := os.WriteFile(path, b, 0600); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path+".sha256", []byte(hx(hash(string(b)))+"\n"), 0600); err != nil {
		f.t.Fatal(err)
	}
}
func (f fixture) check(p persistence.Profile, want bool) {
	f.t.Helper()
	before := f.snapshot()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx := f.runtime.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: true})
	if tx.Error != nil {
		f.t.Fatal(tx.Error)
	}
	err := persistence.Check(ctx, tx, p)
	rollback := tx.Rollback().Error
	if rollback != nil {
		f.t.Fatal(rollback)
	}
	if (err == nil) != want {
		f.t.Fatalf("Check success=%v wanted %v: %v", err == nil, want, err)
	}
	if after := f.snapshot(); after != before {
		f.t.Fatal("read-only check mutated retained evidence/rows")
	}
}

func TestCheckIsInertOnMissingHandlesAndUnsupportedProfiles(t *testing.T) {
	p := profile(t, spec())
	for _, db := range []*gorm.DB{nil, {}, {Config: &gorm.Config{}}} {
		if persistence.Check(context.Background(), db, p) == nil {
			t.Fatal("unusable handle accepted")
		}
	}
	if persistence.Check(context.Background(), nil, persistence.Profile{}) == nil {
		t.Fatal("zero profile accepted")
	}
}

func TestFixtureRefusesUnownedIdentityOrMounts(t *testing.T) {
	name, token := "justixauto-t930-"+uuid.NewString(), uuid.NewString()
	root := "/explicit/task/root"
	base := containerIdentity{ID: strings.Repeat("a", 64), Name: "/" + name, Image: pgImage, Labels: map[string]string{fixtureLabel: token}, Tmpfs: map[string]string{"/var/lib/postgresql": "rw"}}
	base.Mounts = append(base.Mounts, struct {
		Type        string
		Source      string
		Destination string
		RW          bool
	}{"bind", filepath.Join(root, "infra/local/postgres/init-owners.sql"), "/docker-entrypoint-initdb.d/10-owners.sql", false})
	if err := base.validate(name, token, root); err != nil {
		t.Fatal(err)
	}
	for label, change := range map[string]func(*containerIdentity){"wrong label": func(v *containerIdentity) { v.Labels = map[string]string{fixtureLabel: uuid.NewString()} }, "wrong image": func(v *containerIdentity) { v.Image = "other" }, "wrong name": func(v *containerIdentity) { v.Name = "/other" }, "bad id": func(v *containerIdentity) { v.ID = "mutable-name" }, "no tmpfs": func(v *containerIdentity) { v.Tmpfs = nil }, "unexpected volume": func(v *containerIdentity) { v.Mounts[0].Type = "volume" }, "writable bind": func(v *containerIdentity) { v.Mounts[0].RW = true }, "different bind": func(v *containerIdentity) { v.Mounts[0].Source = "/some/other/path" }} {
		t.Run(label, func(t *testing.T) {
			v := base
			v.Mounts = append(v.Mounts[:0:0], base.Mounts...)
			change(&v)
			if v.validate(name, token, root) == nil {
				t.Fatal("unowned identity/mount authorized removal")
			}
		})
	}
}

func TestFixtureAllocationFailures(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_PERSISTENCE_PROFILE") != "1" {
		t.Skip("owned PostgreSQL opt-in required")
	}
	if os.Getenv("JUSTIXAUTO_T930_FIXTURE_CHILD") != "" {
		startFixture(t)
		t.Fatal("expected allocation failure")
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	for _, mode := range []string{"lost-reply", "no-allocation"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			namefile, tokenfile, idfile := filepath.Join(dir, "name"), filepath.Join(dir, "token"), filepath.Join(dir, "id")
			t.Cleanup(func() {
				nb, ne := os.ReadFile(namefile)
				tb, te := os.ReadFile(tokenfile)
				if ne == nil && te == nil {
					cleanupFixture(t, docker, strings.TrimSpace(string(nb)), strings.TrimSpace(string(tb)), root)
				}
			})
			wrapper := "#!/bin/sh\nset -eu\nif [ \"$1\" = create ]; then\n previous=\n for argument in \"$@\"; do\n if [ \"$previous\" = --name ]; then printf '%s\\n' \"$argument\" > " + quote(namefile) + "; fi\n if [ \"$previous\" = --label ]; then case \"$argument\" in " + fixtureLabel + "=*) printf '%s\\n' \"${argument#*=}\" > " + quote(tokenfile) + ";; esac; fi\n previous=$argument\n done\n"
			if mode == "lost-reply" {
				wrapper += " " + quote(docker) + " \"$@\" > " + quote(idfile) + "\n"
			} else {
				wrapper += " shift\n status=0\n " + quote(docker) + " create --t930-invalid-option \"$@\" || status=$?\n if [ \"$status\" = 0 ]; then exit 1; fi\n echo \"T930 actual allocation rejected: $status\" >&2\n"
			}
			wrapper += " echo 'T930 synthetic " + mode + " failure' >&2\n exit 42\nfi\nexec " + quote(docker) + " \"$@\"\n"
			if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(wrapper), 0700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFixtureAllocationFailures$", "-test.v")
			child.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "JUSTIXAUTO_T930_FIXTURE_CHILD="+mode)
			out, err := child.CombinedOutput()
			t.Logf("expected child failure: %v\n%s", err, out)
			if err == nil || !strings.Contains(string(out), "T930 synthetic "+mode+" failure") {
				t.Fatal("allocation failure not exercised")
			}
			nb, err := os.ReadFile(namefile)
			if err != nil {
				t.Fatal(err)
			}
			name := strings.TrimSpace(string(nb))
			if _, exists, err := inspectFixture(ctx, docker, name); err != nil || exists {
				t.Fatalf("child left allocated fixture: %s %v", name, err)
			}
			if mode == "lost-reply" {
				ib, err := os.ReadFile(idfile)
				if err != nil {
					t.Fatal(err)
				}
				id := strings.TrimSpace(string(ib))
				decoded, err := hex.DecodeString(id)
				if err != nil || len(decoded) != 32 {
					t.Fatal("actual allocation receipt absent")
				}
				if _, exists, err := inspectFixture(ctx, docker, id); err != nil || exists {
					t.Fatalf("actual allocation remains: %s %v", id, err)
				}
				if !strings.Contains(string(out), "validated fixture removed and absent: "+id+" "+name) {
					t.Fatal("child did not independently clean its allocation")
				}
			} else {
				if !strings.Contains(string(out), "unknown flag") || !strings.Contains(string(out), "T930 actual allocation rejected:") || !strings.Contains(string(out), "fixture absent after failed allocation: "+name) {
					t.Fatal("actual pre-allocation rejection and absence not proven")
				}
				if _, err := os.Stat(idfile); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("unexpected allocation receipt")
				}
			}
		})
	}
}

func TestPersistenceProfilePostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_PERSISTENCE_PROFILE") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_PERSISTENCE_PROFILE=1 for owned pinned PostgreSQL")
	}
	f := startFixture(t)
	p := profile(t, f.spec)
	t.Run("valid exact history is SELECT-only and retained", func(t *testing.T) {
		f.t = t
		f.retain("initial.json", f.snapshot())
		f.check(p, true)
		f.check(p, true)
		f.retain("after-readonly.json", f.snapshot())
		t.Logf("retained preimage SHA256 %s", hx(hash(f.snapshot())))
	})
	t.Run("wrong profiles and mode fail without writes", func(t *testing.T) {
		f.t = t
		for _, change := range []func(*persistence.Specification){func(s *persistence.Specification) { s.Baseline.BackupRef = "other" }, func(s *persistence.Specification) { s.HistorySHA256 = hash("wrong") }, func(s *persistence.Specification) { s.Artifacts[1].ManifestSHA256 = hash("wrong") }, func(s *persistence.Specification) { s.Artifacts[1].Identity.SHA256 = hash("wrong") }, func(s *persistence.Specification) { s.Shared.Artifacts[0].SHA256 = hash("wrong") }, func(s *persistence.Specification) { s.Shared.Mode = "custody" }, func(s *persistence.Specification) {
			s.Owner = "inventory"
			s.Database = "justix_inventory"
			s.RuntimeRole = "justix_inventory_runtime"
		}} {
			s := p.Specification()
			change(&s)
			f.check(profile(t, s), false)
		}
	})
	t.Run("required and excess permissions", func(t *testing.T) {
		f.t = t
		cases := []struct{ name, change, undo string }{
			{"metadata SELECT lost", "REVOKE SELECT ON owner_migrations.artifacts FROM justix_identity_runtime", "GRANT SELECT ON owner_migrations.artifacts TO justix_identity_runtime"},
			{"feature column required", "REVOKE UPDATE(body) ON synthetic_feature.items FROM justix_identity_runtime", "GRANT UPDATE(body) ON synthetic_feature.items TO justix_identity_runtime"},
			{"feature table SELECT required", "REVOKE SELECT ON synthetic_feature.items FROM justix_identity_runtime", "GRANT SELECT ON synthetic_feature.items TO justix_identity_runtime"},
			{"metadata INSERT column", "GRANT INSERT(version) ON owner_migrations.artifacts TO justix_identity_runtime", "REVOKE INSERT(version) ON owner_migrations.artifacts FROM justix_identity_runtime"},
			{"metadata MAINTAIN", "GRANT MAINTAIN ON owner_migrations.artifacts TO justix_identity_runtime", "REVOKE MAINTAIN ON owner_migrations.artifacts FROM justix_identity_runtime"},
			{"metadata REFERENCES", "GRANT REFERENCES ON owner_migrations.compatibility TO justix_identity_runtime", "REVOKE REFERENCES ON owner_migrations.compatibility FROM justix_identity_runtime"},
			{"immutable feature column", "GRANT UPDATE(immutable) ON synthetic_feature.items TO justix_identity_runtime", "REVOKE UPDATE(immutable) ON synthetic_feature.items FROM justix_identity_runtime"},
			{"broad feature INSERT", "GRANT INSERT ON synthetic_feature.items TO justix_identity_runtime", "REVOKE INSERT ON synthetic_feature.items FROM justix_identity_runtime;GRANT INSERT(id,body) ON synthetic_feature.items TO justix_identity_runtime"},
			{"adjacent feature SELECT", "CREATE TABLE synthetic_feature.secret(value text);GRANT SELECT(value) ON synthetic_feature.secret TO justix_identity_runtime", "DROP TABLE synthetic_feature.secret"},
			{"PUBLIC table SELECT", "GRANT SELECT ON synthetic_feature.items TO PUBLIC", "REVOKE SELECT ON synthetic_feature.items FROM PUBLIC"},
			{"PUBLIC column SELECT", "GRANT SELECT(body) ON synthetic_feature.items TO PUBLIC", "REVOKE SELECT(body) ON synthetic_feature.items FROM PUBLIC"},
			{"delegated required SELECT", "GRANT SELECT ON synthetic_feature.items TO justix_identity_runtime WITH GRANT OPTION", "REVOKE GRANT OPTION FOR SELECT ON synthetic_feature.items FROM justix_identity_runtime"},
			{"delegated required column", "GRANT UPDATE(body) ON synthetic_feature.items TO justix_identity_runtime WITH GRANT OPTION", "REVOKE GRANT OPTION FOR UPDATE(body) ON synthetic_feature.items FROM justix_identity_runtime"},
			{"delegated schema USAGE", "GRANT USAGE ON SCHEMA synthetic_feature TO justix_identity_runtime WITH GRANT OPTION", "REVOKE GRANT OPTION FOR USAGE ON SCHEMA synthetic_feature FROM justix_identity_runtime"},
			{"default table privileges", "ALTER DEFAULT PRIVILEGES FOR ROLE justix_identity IN SCHEMA synthetic_feature GRANT INSERT ON TABLES TO justix_identity_runtime", "ALTER DEFAULT PRIVILEGES FOR ROLE justix_identity IN SCHEMA synthetic_feature REVOKE INSERT ON TABLES FROM justix_identity_runtime"},
			{"global PUBLIC default", "ALTER DEFAULT PRIVILEGES FOR ROLE justix_identity GRANT SELECT ON TABLES TO PUBLIC", "ALTER DEFAULT PRIVILEGES FOR ROLE justix_identity REVOKE SELECT ON TABLES FROM PUBLIC"},
			{"schema CREATE", "GRANT CREATE ON SCHEMA synthetic_feature TO justix_identity_runtime", "REVOKE CREATE ON SCHEMA synthetic_feature FROM justix_identity_runtime"},
			{"database TEMP", "GRANT TEMP ON DATABASE justix_identity TO justix_identity_runtime", "REVOKE TEMP ON DATABASE justix_identity FROM justix_identity_runtime"},
			{"PUBLIC database CONNECT", "GRANT CONNECT ON DATABASE justix_identity TO PUBLIC", "REVOKE CONNECT ON DATABASE justix_identity FROM PUBLIC"},
			{"database CREATE", "GRANT CREATE ON DATABASE justix_identity TO justix_identity_runtime", "REVOKE CREATE ON DATABASE justix_identity FROM justix_identity_runtime"},
			{"function EXECUTE", "GRANT EXECUTE ON FUNCTION owner_migrations.immutable() TO justix_identity_runtime", "REVOKE EXECUTE ON FUNCTION owner_migrations.immutable() FROM justix_identity_runtime"},
			{"foreign schema reach", "CREATE SCHEMA unrelated;GRANT USAGE ON SCHEMA unrelated TO justix_identity_runtime", "DROP SCHEMA unrelated"},
			{"missing named object", "ALTER TABLE synthetic_feature.items RENAME TO temporarily_missing", "ALTER TABLE synthetic_feature.temporarily_missing RENAME TO items"},
			{"disabled history guard", "ALTER TABLE owner_migrations.artifacts DISABLE TRIGGER artifacts_immutable", "ALTER TABLE owner_migrations.artifacts ENABLE TRIGGER artifacts_immutable"},
			{"history RLS cannot hide complete rows", "ALTER TABLE owner_migrations.artifacts ENABLE ROW LEVEL SECURITY;CREATE POLICY synthetic_visible ON owner_migrations.artifacts USING(true)", "DROP POLICY synthetic_visible ON owner_migrations.artifacts;ALTER TABLE owner_migrations.artifacts DISABLE ROW LEVEL SECURITY"},
			{"wrong feature object owner", "ALTER TABLE synthetic_feature.items OWNER TO postgres", "ALTER TABLE synthetic_feature.items OWNER TO justix_identity"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				f.t = t
				role := "justix_identity"
				if tc.name == "wrong feature object owner" {
					role = "postgres"
				}
				f.must(tc.change, role)
				defer f.must(tc.undo, role)
				if tc.name == "missing named object" {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					if persistence.Check(ctx, f.runtime, p) == nil {
						t.Fatal("missing required object accepted")
					}
				} else {
					f.check(p, false)
				}
			})
			f.t = t
			f.check(p, true)
		}
	})
	t.Run("actual incomplete and divergent immutable evidence", func(t *testing.T) {
		f.t = t
		cases := []struct{ name, table, change string }{
			{"missing historical receipt", "artifacts", "DELETE FROM owner_migrations.artifacts WHERE version=12"},
			{"missing feature marker", "feature_contracts", "DELETE FROM owner_migrations.feature_contracts"},
			{"wrong feature marker hash", "feature_contracts", "UPDATE owner_migrations.feature_contracts SET contract_sha256=decode('" + hx(hash("wrong-marker")) + "','hex')"},
			{"broken predecessor hash", "artifacts", "UPDATE owner_migrations.artifacts SET predecessor_sha256=decode('" + hx(hash("wrong-predecessor")) + "','hex') WHERE version=12"},
			{"wrong SQL hash", "artifacts", "UPDATE owner_migrations.artifacts SET sql_sha256=decode('" + hx(hash("wrong-sql")) + "','hex') WHERE version=12"},
			{"wrong baseline reference", "compatibility", "UPDATE owner_migrations.compatibility SET evidence_ref='different-baseline'"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				f.t = t
				f.must("CREATE TABLE public.t930_saved AS TABLE owner_migrations."+tc.table+";ALTER TABLE owner_migrations."+tc.table+" DISABLE TRIGGER ALL;"+tc.change+";ALTER TABLE owner_migrations."+tc.table+" ENABLE TRIGGER ALL", "postgres")
				defer f.must("ALTER TABLE owner_migrations."+tc.table+" DISABLE TRIGGER ALL;DELETE FROM owner_migrations."+tc.table+";INSERT INTO owner_migrations."+tc.table+" SELECT * FROM public.t930_saved;ALTER TABLE owner_migrations."+tc.table+" ENABLE TRIGGER ALL;DROP TABLE public.t930_saved", "postgres")
				f.check(p, false)
			})
			f.t = t
			f.check(p, true)
		}
	})
	t.Run("transitive NOINHERIT and privileged identities", func(t *testing.T) {
		f.t = t
		f.must("CREATE ROLE t930_outer NOINHERIT;CREATE ROLE t930_inner NOINHERIT;GRANT t930_inner TO t930_outer;GRANT t930_outer TO justix_identity_runtime;GRANT UPDATE(immutable) ON synthetic_feature.items TO t930_inner", "postgres")
		f.check(p, false)
		f.must("REVOKE UPDATE(immutable) ON synthetic_feature.items FROM t930_inner;REVOKE t930_outer FROM justix_identity_runtime;DROP ROLE t930_outer;DROP ROLE t930_inner", "postgres")
		f.check(p, true)
		for _, role := range []string{"justix_identity", "pg_read_all_data"} {
			f.must("GRANT "+role+" TO justix_identity_runtime", "postgres")
			f.check(p, false)
			f.must("REVOKE "+role+" FROM justix_identity_runtime", "postgres")
			f.check(p, true)
		}
		f.must("GRANT CONNECT ON DATABASE justix_inventory TO justix_identity_runtime", "postgres")
		f.check(p, false)
		f.must("REVOKE CONNECT ON DATABASE justix_inventory FROM justix_identity_runtime", "postgres")
		f.check(p, true)
	})
	t.Run("ledger and retained history mismatch", func(t *testing.T) {
		f.t = t
		cases := []struct{ name, change, undo string }{
			{"dirty", "UPDATE public.schema_migrations SET dirty=true", "UPDATE public.schema_migrations SET dirty=false"},
			{"newer", "UPDATE public.schema_migrations SET version=17", "UPDATE public.schema_migrations SET version=12"},
			{"extra row", "INSERT INTO public.schema_migrations VALUES(99,false)", "DELETE FROM public.schema_migrations WHERE version=99"},
			{"extra column", "ALTER TABLE public.schema_migrations ADD COLUMN bypass boolean", "ALTER TABLE public.schema_migrations DROP COLUMN bypass"},
			{"old hash drift", "ALTER TABLE owner_migrations.artifacts DISABLE TRIGGER artifacts_immutable;UPDATE owner_migrations.artifacts SET artifact_manifest_sha256=decode('" + hx(hash("different")) + "','hex') WHERE version=1;ALTER TABLE owner_migrations.artifacts ENABLE TRIGGER artifacts_immutable", "ALTER TABLE owner_migrations.artifacts DISABLE TRIGGER artifacts_immutable;UPDATE owner_migrations.artifacts SET artifact_manifest_sha256=decode('" + hx(f.spec.Artifacts[0].ManifestSHA256) + "','hex') WHERE version=1;ALTER TABLE owner_migrations.artifacts ENABLE TRIGGER artifacts_immutable"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) { f.t = t; f.must(tc.change); defer f.must(tc.undo); f.check(p, false) })
			f.t = t
			f.check(p, true)
		}
	})
	t.Run("approved feature columns remain independently mutable", func(t *testing.T) {
		f.t = t
		if _, err := f.command("BEGIN;UPDATE synthetic_feature.items SET body='allowed';ROLLBACK", "justix_identity_runtime"); err != nil {
			t.Fatal(err)
		}
		for _, stmt := range []string{"UPDATE synthetic_feature.items SET immutable='forbidden'", "INSERT INTO synthetic_feature.items(id,body,immutable) VALUES(gen_random_uuid(),'x','forbidden')", "DELETE FROM synthetic_feature.items", "UPDATE owner_migrations.artifacts SET filename=filename"} {
			if _, err := f.command(stmt, "justix_identity_runtime"); err == nil {
				t.Fatal("forbidden mutation succeeded")
			}
		}
		f.check(p, true)
	})
	t.Run("feature column SELECT is independently exact", func(t *testing.T) {
		f.t = t
		f.must("REVOKE SELECT ON synthetic_feature.items FROM justix_identity_runtime;GRANT SELECT(id,body) ON synthetic_feature.items TO justix_identity_runtime")
		defer f.must("REVOKE SELECT(id,body) ON synthetic_feature.items FROM justix_identity_runtime;GRANT SELECT ON synthetic_feature.items TO justix_identity_runtime")
		s := p.Specification()
		s.Features[0].Tables[0].Select = false
		s.Features[0].Tables[0].SelectColumns = []string{"id", "body"}
		cp := profile(t, s)
		f.check(cp, true)
		f.check(p, false)
		if _, err := f.command("SELECT immutable FROM synthetic_feature.items", "justix_identity_runtime"); err == nil {
			t.Fatal("undeclared SELECT column allowed")
		}
	})
	t.Run("cancellation is fail closed", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := persistence.Check(ctx, f.runtime, p); err == nil {
			t.Fatal("cancelled check succeeded")
		}
	})
	t.Run("complete upgrade preserves prior installing profile identities", func(t *testing.T) {
		f.t = t
		before := f.snapshot()
		fi := persistence.FeatureIdentity{ID: "synthetic.second", Revision: 1, SHA256: hash("contract17")}
		a := persistence.Artifact{Identity: persistence.ArtifactIdentity{Version: 17, Filename: "migrations/0017_synthetic.up.sql", SHA256: hash("feature17")}, ManifestSHA256: hash("manifest17"), Prerequisites: []persistence.ArtifactIdentity{f.spec.Artifacts[1].Identity}, Feature: &fi}
		feature := persistence.Feature{Identity: fi, Tables: []persistence.TableGrant{{Schema: "synthetic_feature", Table: "second", Select: true, InsertColumns: []string{"id", "body"}, UpdateColumns: []string{"body"}}}}
		oldHistory := f.must("SELECT jsonb_agg(to_jsonb(x) ORDER BY version) FROM owner_migrations.artifacts x")
		f.installFeature(a, feature)
		f.check(p, false)
		s := p.Specification()
		s.Artifacts = append(s.Artifacts, a)
		s.Head = 17
		s.Features = append(s.Features, feature)
		p = profile(t, s)
		f.spec = s
		f.check(p, true)
		f.retain("upgraded.json", f.snapshot())
		if f.must("SELECT jsonb_agg(to_jsonb(x) ORDER BY version) FROM owner_migrations.artifacts x WHERE version<=12") != oldHistory {
			t.Fatal("upgrade rewrote prior receipts")
		}
		if before == f.snapshot() {
			t.Fatal("upgrade fixture did not actually change state")
		}
		s = p.Specification()
		s.Artifacts = append(s.Artifacts[:1], s.Artifacts[2:]...)
		s.Artifacts[1].Prerequisites = []persistence.ArtifactIdentity{s.Artifacts[0].Identity}
		s.Features = s.Features[1:]
		f.check(profile(t, s), false)
	})
	t.Run("declared shared lineage remains independent", func(t *testing.T) {
		f.t = t
		paths := []string{"pkg/eventstore/migrations/000002_messaging_delivery.up.sql", "pkg/eventstore/migrations/000003_messaging_route_compatibility.up.sql", "pkg/eventstore/migrations/000004_projection_checkpoint.up.sql"}
		f.install(paths[0])
		f.check(p, false)
		s := p.Specification()
		s.Shared.Artifacts = append(s.Shared.Artifacts, persistence.SharedArtifact{Revision: 2, Filename: paths[0], SHA256: hash(f.read(paths[0]))})
		p = profile(t, s)
		f.check(p, true)
		f.install(paths[1], "prior_migration_sha256="+hx(s.Shared.Artifacts[1].SHA256), "correction_migration_sha256="+hx(hash(f.read(paths[1]))), "backup_ref=synthetic-backup", "stopped_runtimes_ref=synthetic-stopped", "compatibility_ref=synthetic-compatible")
		f.check(p, false)
		s.Shared.Artifacts = append(s.Shared.Artifacts, persistence.SharedArtifact{Revision: 3, Filename: paths[1], SHA256: hash(f.read(paths[1]))})
		p = profile(t, s)
		f.check(p, true)
		f.install(paths[2], "base_schema_sha256="+hx(s.Shared.Artifacts[0].SHA256), "prior_migration_sha256="+hx(s.Shared.Artifacts[1].SHA256), "correction_migration_sha256="+hx(s.Shared.Artifacts[2].SHA256), "checkpoint_migration_sha256="+hx(hash(f.read(paths[2]))), "backup_ref=synthetic-backup", "stopped_runtimes_ref=synthetic-stopped", "compatibility_ref=synthetic-compatible")
		f.check(p, false)
		s.Shared.Artifacts = append(s.Shared.Artifacts, persistence.SharedArtifact{Revision: 4, Filename: paths[2], SHA256: hash(f.read(paths[2]))})
		p = profile(t, s)
		f.check(p, true)
		f.must("ALTER TABLE eventstore.projection_checkpoint_compatibility DISABLE TRIGGER immutable_content;UPDATE eventstore.projection_checkpoint_compatibility SET checkpoint_migration_sha256=decode('" + hx(hash("divergent-shared-artifact")) + "','hex');ALTER TABLE eventstore.projection_checkpoint_compatibility ENABLE TRIGGER immutable_content")
		f.check(p, false)
		f.must("ALTER TABLE eventstore.projection_checkpoint_compatibility DISABLE TRIGGER immutable_content;UPDATE eventstore.projection_checkpoint_compatibility SET checkpoint_migration_sha256=decode('" + hx(s.Shared.Artifacts[3].SHA256) + "','hex');ALTER TABLE eventstore.projection_checkpoint_compatibility ENABLE TRIGGER immutable_content")
		f.check(p, true)
		f.must("REVOKE UPDATE(position) ON eventstore.consumer_checkpoints FROM justix_identity_runtime")
		f.check(p, false)
		f.must("GRANT UPDATE(position) ON eventstore.consumer_checkpoints TO justix_identity_runtime")
		f.check(p, true)
		f.must("ALTER TABLE eventstore.consumer_gaps RENAME TO missing_gap_object")
		f.check(p, false)
		f.must("ALTER TABLE eventstore.missing_gap_object RENAME TO consumer_gaps")
		f.check(p, true)
		s = p.Specification()
		s.Shared.Mode = "custody"
		if err := persistence.Check(context.Background(), f.runtime, profile(t, s)); !errors.Is(err, persistence.ErrUnsupported) {
			t.Fatalf("custody not explicitly unsupported: %v", err)
		}
		s = p.Specification()
		s.Shared.Artifacts = append(s.Shared.Artifacts, persistence.SharedArtifact{Revision: 5, Filename: "pkg/eventstore/migrations/000005_quarantine_evidence.up.sql", SHA256: hash("later")})
		if err := persistence.Check(context.Background(), f.runtime, profile(t, s)); !errors.Is(err, persistence.ErrUnsupported) {
			t.Fatalf("later lineage not explicitly unsupported: %v", err)
		}
	})
	if b, err := json.MarshalIndent(p.Specification(), "", "  "); err != nil {
		t.Fatal(err)
	} else {
		t.Logf("final exact fixture profile digest %s; specification bytes %d", hx(p.Digest()), len(b))
		f.t = t
		f.retain("final-profile.json", string(b))
		f.retain("final-with-shared.json", f.snapshot())
	}
}

func TestPersistenceEffectiveDefaultsAndGuardsPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_PERSISTENCE_PROFILE") != "1" {
		t.Skip("owned PostgreSQL opt-in required")
	}
	f := startFixture(t)
	p := profile(t, f.spec)
	f.check(p, true)
	t.Run("implicit function default survives absent ACL rows and schema revoke", func(t *testing.T) {
		f.t = t
		f.must("ALTER DEFAULT PRIVILEGES GRANT EXECUTE ON FUNCTIONS TO PUBLIC;ALTER DEFAULT PRIVILEGES IN SCHEMA synthetic_feature REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC")
		defer f.must("ALTER DEFAULT PRIVILEGES REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC")
		if rows := f.must("SELECT count(*) FROM pg_default_acl WHERE defaclrole=current_user::regrole AND defaclobjtype='f'"); rows != "0" {
			t.Fatalf("implicit default probe retained %s rows", rows)
		}
		f.check(p, false)
		f.must("CREATE FUNCTION synthetic_feature.t930_implicit() RETURNS integer LANGUAGE sql AS 'SELECT 1'")
		defer f.must("DROP FUNCTION synthetic_feature.t930_implicit()")
		if value := f.must("SELECT synthetic_feature.t930_implicit()", "justix_identity_runtime"); value != "1" {
			t.Fatal("actual inherited function execution not demonstrated")
		}
	})
	f.t = t
	f.check(p, true)
	t.Run("private effective default denies newly created function", func(t *testing.T) {
		f.t = t
		f.must("CREATE FUNCTION synthetic_feature.t930_private() RETURNS integer LANGUAGE sql AS 'SELECT 1'")
		defer f.must("DROP FUNCTION synthetic_feature.t930_private()")
		f.check(p, true)
		if _, err := f.command("SELECT synthetic_feature.t930_private()", "justix_identity_runtime"); err == nil {
			t.Fatal("runtime executed new function despite denied effective defaults")
		}
	})
	t.Run("implicit type default", func(t *testing.T) {
		f.t = t
		f.must("ALTER DEFAULT PRIVILEGES GRANT USAGE ON TYPES TO PUBLIC")
		defer f.must("ALTER DEFAULT PRIVILEGES REVOKE USAGE ON TYPES FROM PUBLIC")
		if rows := f.must("SELECT count(*) FROM pg_default_acl WHERE defaclrole=current_user::regrole AND defaclobjtype='T'"); rows != "0" {
			t.Fatalf("implicit type default retained %s rows", rows)
		}
		f.check(p, false)
		f.must("CREATE DOMAIN synthetic_feature.t930_type AS integer")
		defer f.must("DROP DOMAIN synthetic_feature.t930_type")
		if got := f.must("SELECT has_type_privilege('justix_identity_runtime','synthetic_feature.t930_type','USAGE')"); got != "t" {
			t.Fatal("actual type default not demonstrated")
		}
	})
	f.t = t
	f.check(p, true)
	for _, family := range []struct{ object, privilege string }{{"TABLES", "SELECT"}, {"SEQUENCES", "USAGE"}, {"FUNCTIONS", "EXECUTE"}, {"TYPES", "USAGE"}, {"SCHEMAS", "USAGE"}, {"LARGE OBJECTS", "SELECT"}} {
		for _, scope := range []string{"", " IN SCHEMA synthetic_feature"} {
			if scope != "" && (family.object == "SCHEMAS" || family.object == "LARGE OBJECTS") {
				continue
			}
			t.Run("effective "+family.object+scope, func(t *testing.T) {
				f.t = t
				f.must("ALTER DEFAULT PRIVILEGES" + scope + " GRANT " + family.privilege + " ON " + family.object + " TO justix_identity_runtime")
				defer f.must("ALTER DEFAULT PRIVILEGES" + scope + " REVOKE " + family.privilege + " ON " + family.object + " FROM justix_identity_runtime")
				f.check(p, false)
			})
			f.t = t
			f.check(p, true)
		}
	}
	t.Run("schema additions cannot be hidden by private global defaults", func(t *testing.T) {
		f.t = t
		f.must("ALTER DEFAULT PRIVILEGES IN SCHEMA synthetic_feature GRANT EXECUTE ON FUNCTIONS TO PUBLIC")
		defer f.must("ALTER DEFAULT PRIVILEGES IN SCHEMA synthetic_feature REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC")
		f.check(p, false)
		f.must("CREATE FUNCTION synthetic_feature.t930_schema_default() RETURNS integer LANGUAGE sql AS 'SELECT 2'")
		defer f.must("DROP FUNCTION synthetic_feature.t930_schema_default()")
		if got := f.must("SELECT synthetic_feature.t930_schema_default()", "justix_identity_runtime"); got != "2" {
			t.Fatal("schema default addition not demonstrated")
		}
	})
	f.t = t
	f.check(p, true)
	for _, guard := range []struct{ name, definition, bypass string }{
		{"WHEN false", "BEFORE UPDATE OR DELETE OR TRUNCATE ON owner_migrations.artifacts FOR EACH STATEMENT WHEN(false) EXECUTE FUNCTION owner_migrations.immutable()", "UPDATE owner_migrations.artifacts SET provenance_ref=provenance_ref"},
		{"WHEN true", "BEFORE UPDATE OR DELETE OR TRUNCATE ON owner_migrations.artifacts FOR EACH STATEMENT WHEN(true) EXECUTE FUNCTION owner_migrations.immutable()", ""},
		{"column filter", "BEFORE UPDATE OF provenance_ref OR DELETE OR TRUNCATE ON owner_migrations.artifacts FOR EACH STATEMENT EXECUTE FUNCTION owner_migrations.immutable()", "UPDATE owner_migrations.artifacts SET artifact_manifest_sha256=artifact_manifest_sha256"},
		{"unexpected trigger argument", "BEFORE UPDATE OR DELETE OR TRUNCATE ON owner_migrations.artifacts FOR EACH STATEMENT EXECUTE FUNCTION owner_migrations.immutable('unexpected')", ""},
	} {
		t.Run("immutable execution metadata "+guard.name, func(t *testing.T) {
			f.t = t
			f.must("DROP TRIGGER artifacts_immutable ON owner_migrations.artifacts;CREATE TRIGGER artifacts_immutable " + guard.definition)
			defer f.must("DROP TRIGGER artifacts_immutable ON owner_migrations.artifacts;CREATE TRIGGER artifacts_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON owner_migrations.artifacts FOR EACH STATEMENT EXECUTE FUNCTION owner_migrations.immutable()")
			before := f.snapshot()
			f.check(p, false)
			if guard.bypass != "" {
				if _, err := f.command("BEGIN;"+guard.bypass+";ROLLBACK", "justix_identity"); err != nil {
					t.Fatalf("ordinary owner bypass not demonstrated: %v", err)
				}
				if before != f.snapshot() {
					t.Fatal("bypass rollback changed evidence")
				}
			}
		})
		f.t = t
		f.check(p, true)
	}
	t.Run("guard function execution path", func(t *testing.T) {
		f.t = t
		f.must("ALTER FUNCTION owner_migrations.immutable() SET search_path TO public,pg_catalog")
		defer f.must("ALTER FUNCTION owner_migrations.immutable() SET search_path TO pg_catalog")
		f.check(p, false)
	})
	f.t = t
	f.check(p, true)
	t.Run("runtime cannot acquire a trigger disabling mode", func(t *testing.T) {
		f.t = t
		f.must("GRANT SET ON PARAMETER session_replication_role TO justix_identity_runtime", "postgres")
		defer f.must("REVOKE SET ON PARAMETER session_replication_role FROM justix_identity_runtime", "postgres")
		f.check(p, false)
		if got := f.must("BEGIN;SET LOCAL session_replication_role=replica;SELECT current_setting('session_replication_role');ROLLBACK", "justix_identity_runtime"); got != "replica" {
			t.Fatalf("actual trigger mode change not demonstrated: %s", got)
		}
	})
	f.t = t
	f.check(p, true)
	t.Run("reachable NOINHERIT trigger mode privilege", func(t *testing.T) {
		f.t = t
		f.must("CREATE ROLE t930_mode_outer NOINHERIT;CREATE ROLE t930_mode_inner NOINHERIT;GRANT t930_mode_inner TO t930_mode_outer;GRANT t930_mode_outer TO justix_identity_runtime;GRANT SET ON PARAMETER session_replication_role TO t930_mode_inner", "postgres")
		defer f.must("REVOKE SET ON PARAMETER session_replication_role FROM t930_mode_inner;REVOKE t930_mode_outer FROM justix_identity_runtime;DROP ROLE t930_mode_outer;DROP ROLE t930_mode_inner", "postgres")
		f.check(p, false)
	})
	f.t = t
	f.check(p, true)
	t.Run("restored unconditional guard rejects ordinary owner mutation", func(t *testing.T) {
		f.t = t
		if _, err := f.command("BEGIN;UPDATE owner_migrations.artifacts SET provenance_ref=provenance_ref;ROLLBACK", "justix_identity"); err == nil {
			t.Fatal("restored guard failed to reject owner update")
		}
		f.must("ALTER TABLE owner_migrations.artifacts ENABLE ALWAYS TRIGGER artifacts_immutable")
		defer f.must("ALTER TABLE owner_migrations.artifacts ENABLE TRIGGER artifacts_immutable")
		f.check(p, true)
	})
	f.t = t
	f.retain("effective-defaults-and-guards-final.json", f.snapshot())
}
