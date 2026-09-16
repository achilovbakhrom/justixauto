package postgres_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"justixauto/pkg/commands"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
	"justixauto/pkg/persistence"
	identity "justixauto/services/identity/adapter/postgres"
	"justixauto/services/identity/port"
)

func TestConstructionIsInertAndRejectsMissingDependencies(t *testing.T) {
	if _, err := identity.NewUnitOfWork[struct{}](nil, func(*gorm.DB) (struct{}, error) { return struct{}{}, nil }); err == nil {
		t.Fatal("nil database accepted")
	}
	db := &gorm.DB{Config: &gorm.Config{}}
	if _, err := identity.NewUnitOfWork[struct{}](db, nil); err == nil {
		t.Fatal("nil factory accepted")
	}
	called := false
	if _, err := identity.NewUnitOfWork(db, func(*gorm.DB) (struct{}, error) { called = true; return struct{}{}, nil }); err != nil || called {
		t.Fatalf("construction was not inert: %v", err)
	}
	var empty *identity.UnitOfWork[struct{}]
	if empty.Check(context.Background()) == nil || empty.Run(context.Background(), nil) == nil {
		t.Fatal("zero runner accepted")
	}
}

type fixture struct {
	t                          *testing.T
	name, password, root, port string
}

const image = "docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"

const fixtureLabel = "justixauto.t931.fixture-owner"

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
	nameUUID, err := uuid.Parse(strings.TrimPrefix(name, "justixauto-t931-"))
	if err != nil || nameUUID == uuid.Nil || name != "justixauto-t931-"+nameUUID.String() {
		return errors.New("invalid generated fixture name")
	}
	tokenUUID, err := uuid.Parse(token)
	if err != nil || tokenUUID == uuid.Nil || token != tokenUUID.String() {
		return errors.New("invalid fixture ownership token")
	}
	id, err := hex.DecodeString(v.ID)
	if err != nil || len(id) != 32 || hex.EncodeToString(id) != v.ID || v.Name != "/"+name || v.Image != image || v.Labels[fixtureLabel] != token {
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

func start(t *testing.T) fixture {
	t.Helper()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{t: t, name: "justixauto-t931-" + uuid.NewString(), password: "synthetic-" + uuid.NewString(), root: root}
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
	args = append(args, image)
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
	t.Logf("created owned fixture %s id %s image %s; captured attached volume IDs []", f.name, v.ID, image)
	if out, err := exec.CommandContext(ctx, "docker", "start", v.ID).CombinedOutput(); err != nil {
		t.Fatalf("start owned fixture: %v %s", err, out)
	}
	for {
		if _, err := f.sql("justix_documents", "justix_documents", "SELECT 1"); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("fixture bootstrap timeout")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if f.must("justix_identity", "postgres", "SHOW server_version_num") != "180006" {
		t.Fatal("wrong server version")
	}
	out, err = exec.CommandContext(ctx, "docker", "port", f.name, "5432/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	f.port = strings.TrimPrefix(strings.TrimSpace(string(out)), "127.0.0.1:")
	return f
}
func (f fixture) sql(database, role, statement string, options ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	args := []string{"exec", "-i", "-e", "PGPASSWORD=" + f.password, f.name, "psql", "-X", "-qAt", "-h", "127.0.0.1", "-U", role, "-d", database, "-v", "ON_ERROR_STOP=1"}
	args = append(args, options...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = strings.NewReader(statement)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
func (f fixture) must(db, role, sql string) string {
	f.t.Helper()
	out, err := f.sql(db, role, sql)
	if err != nil {
		f.t.Fatalf("fixture SQL: %v %s", err, out)
	}
	return out
}
func (f fixture) open(role string) *gorm.DB {
	f.t.Helper()
	dsn := fmt.Sprintf("host=127.0.0.1 port=%s user=%s password=%s dbname=justix_identity sslmode=disable", f.port, role, f.password)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		f.t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { pool.Close() })
	return db
}
func (f fixture) file(path string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.root, path))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

// The fixture performs the documented runner protocol with SQL. It does not
// claim that a golang-migrate binary/driver or the future tools/migrate.sh ran.
func TestIdentityPostgresMechanics(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_IDENTITY_MECHANICS") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_IDENTITY_MECHANICS=1 for disposable PostgreSQL")
	}
	f := start(t)
	ctx := context.Background()
	f.must("justix_identity", "postgres", "CREATE ROLE justix_identity_runtime LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_identity TO justix_identity_runtime;")
	db := f.open("justix_identity_runtime")
	runner, err := identity.NewUnitOfWork(db, func(tx *gorm.DB) (fixturePorts, error) { return bindFixture(tx) })
	if err != nil {
		t.Fatal(err)
	}
	var dependencies = port.Dependencies[fixturePorts]{Transactions: runner, Persistence: runner}
	reject := func() {
		t.Helper()
		if !errors.Is(dependencies.Persistence.Check(ctx), identity.ErrIncompatiblePersistence) {
			t.Fatal("incompatible state ready")
		}
		called := false
		err := dependencies.Transactions.Run(ctx, func(fixturePorts) error { called = true; return nil })
		if !errors.Is(err, identity.ErrIncompatiblePersistence) || called {
			t.Fatalf("unready callback executed: %v %v", called, err)
		}
	}
	reject()
	migration := f.file("services/identity/migrations/0001_mechanics.up.sql")
	f.must("justix_identity", "justix_identity", "CREATE TABLE public.schema_migrations(version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL); INSERT INTO public.schema_migrations VALUES (1,true);")
	if _, err := f.sql("justix_identity", "justix_identity", migration); err == nil {
		t.Fatal("migration accepted absent shared schema")
	}
	if out, err := f.sql("justix_identity", "justix_identity", f.file("pkg/eventstore/schema.sql"), "-v", "owner_service=identity", "-v", "runtime_role=justix_identity_runtime"); err != nil {
		t.Fatalf("shared install: %v %s", err, out)
	}
	reject()
	if _, err := f.sql("justix_inventory", "justix_inventory", migration); err == nil {
		t.Fatal("foreign migration accepted")
	}
	if out, err := f.sql("justix_identity", "justix_identity", migration); err != nil {
		t.Fatalf("owner migration: %v %s", err, out)
	}
	reject() // installation is not ready while the runner's ledger remains dirty
	f.must("justix_identity", "justix_identity", "UPDATE public.schema_migrations SET dirty=false;")
	if err := runner.Check(ctx); err != nil {
		t.Fatalf("clean installation: %v", err)
	}
	if _, err := f.sql("justix_identity", "justix_identity", migration); err == nil {
		t.Fatal("silently reapplied migration")
	}
	for _, change := range []struct{ set, reset string }{
		{"UPDATE public.schema_migrations SET dirty=true", "UPDATE public.schema_migrations SET dirty=false"},
		{"UPDATE public.schema_migrations SET version=2", "UPDATE public.schema_migrations SET version=1"},
		{"GRANT UPDATE ON eventstore.events TO justix_identity_runtime", "REVOKE UPDATE ON eventstore.events FROM justix_identity_runtime"},
		{"GRANT UPDATE (data) ON eventstore.events TO justix_identity_runtime", "REVOKE UPDATE (data) ON eventstore.events FROM justix_identity_runtime"},
		{"GRANT UPDATE ON public.schema_migrations TO justix_identity_runtime", "REVOKE UPDATE ON public.schema_migrations FROM justix_identity_runtime"},
		{"REVOKE UPDATE (phase) ON eventstore.operations FROM justix_identity_runtime", "GRANT UPDATE (phase) ON eventstore.operations TO justix_identity_runtime"},
		{"ALTER TABLE eventstore.events ALTER COLUMN owner_service SET DEFAULT 'inventory'", "ALTER TABLE eventstore.events ALTER COLUMN owner_service SET DEFAULT 'identity'"},
	} {
		f.must("justix_identity", "justix_identity", change.set)
		reject()
		f.must("justix_identity", "justix_identity", change.reset)
	}
	f.must("justix_identity", "postgres", "GRANT pg_write_all_data TO justix_identity_runtime WITH INHERIT FALSE;")
	reject()
	f.must("justix_identity", "postgres", "REVOKE pg_write_all_data FROM justix_identity_runtime;")
	t.Run("excess-direct-and-reachable-role-grants", func(t *testing.T) {
		// NOINHERIT membership is still reachable through SET ROLE. Every
		// check must reject before both feature construction and execution.
		f.must("justix_identity", "postgres", "CREATE ROLE identity_fixture_grants NOLOGIN; GRANT identity_fixture_grants TO justix_identity_runtime WITH INHERIT FALSE;")
		defer f.must("justix_identity", "postgres", "REVOKE identity_fixture_grants FROM justix_identity_runtime;")
		cases := []struct{ name, privilege, object string }{
			{"database_create", "CREATE", "DATABASE justix_identity"},
			{"database_temporary", "TEMPORARY", "DATABASE justix_identity"},
			{"events_maintain", "MAINTAIN", "eventstore.events"},
			{"events_references", "REFERENCES", "eventstore.events"},
			{"events_column_references", "REFERENCES(event_id)", "eventstore.events"},
			{"marker_maintain", "MAINTAIN", "identity_mechanics.compatibility"},
			{"ledger_maintain", "MAINTAIN", "public.schema_migrations"},
			{"marker_references", "REFERENCES", "identity_mechanics.compatibility"},
			{"ledger_references", "REFERENCES", "public.schema_migrations"},
			{"marker_column_insert", "INSERT(singleton,owner_service,mechanics_version)", "identity_mechanics.compatibility"},
			{"ledger_column_insert", "INSERT(version,dirty)", "public.schema_migrations"},
			{"marker_column_references", "REFERENCES(singleton)", "identity_mechanics.compatibility"},
			{"ledger_column_references", "REFERENCES(version)", "public.schema_migrations"},
		}
		for _, role := range []string{"justix_identity_runtime", "identity_fixture_grants"} {
			for _, tc := range cases {
				t.Run(role+"/"+tc.name, func(t *testing.T) {
					f.must("justix_identity", "justix_identity", "GRANT "+tc.privilege+" ON "+tc.object+" TO "+role)
					defer f.must("justix_identity", "justix_identity", "REVOKE "+tc.privilege+" ON "+tc.object+" FROM "+role)
					binds := 0
					r, err := identity.NewUnitOfWork(db, func(tx *gorm.DB) (fixturePorts, error) { binds++; return bindFixture(tx) })
					if err != nil {
						t.Fatal(err)
					}
					if !errors.Is(r.Check(ctx), identity.ErrIncompatiblePersistence) {
						t.Fatal("excess grants accepted by Check")
					}
					called := false
					err = r.Run(ctx, func(fixturePorts) error { called = true; return nil })
					if !errors.Is(err, identity.ErrIncompatiblePersistence) || binds != 0 || called {
						t.Fatalf("excess grants reached feature: %v binds=%d called=%v", err, binds, called)
					}
				})
			}
		}
		if err := runner.Check(ctx); err != nil {
			t.Fatalf("narrow privileges no longer accepted: %v", err)
		}
	})
	ownerRunner, _ := identity.NewUnitOfWork(f.open("justix_identity"), bindFixture)
	if ownerRunner.Check(ctx) == nil {
		t.Fatal("migration owner accepted as runtime")
	}
	for _, owner := range []string{"inventory", "commerce", "retail", "financing", "insurance", "documents"} {
		if _, err := f.sql("justix_"+owner, "justix_identity_runtime", "SELECT 1"); err == nil {
			t.Fatalf("runtime connected to %s", owner)
		}
	}
	for _, statement := range []string{"UPDATE identity_mechanics.compatibility SET mechanics_version=1", "UPDATE public.schema_migrations SET dirty=false", "DELETE FROM eventstore.events", "CREATE TABLE eventstore.foreign_table(id int)"} {
		if _, err := f.sql("justix_identity", "justix_identity_runtime", statement); err == nil {
			t.Fatalf("runtime privilege accepted: %s", statement)
		}
	}

	t.Run("atomic-rollback-replay-and-idempotency", func(t *testing.T) {
		scope := commands.Scope{ActorID: uuid.NewString(), Key: uuid.NewString()}
		input := fixtureValue{ReferenceID: uuid.NewString()}
		abort := errors.New("synthetic failure after all effects")
		if err := runner.Run(ctx, func(u fixturePorts) error {
			if _, err := u.Record(ctx, scope, input); err != nil {
				return err
			}
			return abort
		}); !errors.Is(err, abort) {
			t.Fatalf("rollback: %v", err)
		}
		for _, table := range []string{"events", "command_receipts", "operations"} {
			if n := f.must("justix_identity", "justix_identity_runtime", "SELECT count(*) FROM eventstore."+table); n != "0" {
				t.Fatalf("%s leaked %s rows", table, n)
			}
		}
		start := make(chan struct{})
		errs := make(chan error, 6)
		var wg sync.WaitGroup
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs <- runner.Run(ctx, func(u fixturePorts) error { _, err := u.Record(ctx, scope, input); return err })
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, table := range []string{"events", "command_receipts", "operations"} {
			if n := f.must("justix_identity", "justix_identity_runtime", "SELECT count(*) FROM eventstore."+table); n != "1" {
				t.Fatalf("%s effects=%s", table, n)
			}
		}
		if err := runner.Run(ctx, func(u fixturePorts) error {
			_, err := u.Record(ctx, scope, fixtureValue{ReferenceID: uuid.NewString()})
			return err
		}); !errors.Is(err, commands.ErrConflict) {
			t.Fatalf("changed retry: %v", err)
		}
		if err := runner.Run(ctx, func(u fixturePorts) error {
			state, err := u.Load(ctx, input.ReferenceID)
			if err == nil && state != input {
				return errors.New("wrong replay")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("uncommitted-history-invisible-and-panic-cleanup", func(t *testing.T) {
		input := fixtureValue{ReferenceID: uuid.NewString()}
		scope := commands.Scope{ActorID: uuid.NewString(), Key: uuid.NewString()}
		entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		go func() {
			done <- runner.Run(ctx, func(u fixturePorts) error {
				if _, err := u.Record(ctx, scope, input); err != nil {
					return err
				}
				close(entered)
				<-release
				return errors.New("rollback")
			})
		}()
		select {
		case <-entered:
		case err := <-done:
			t.Fatalf("writer failed: %v", err)
		case <-time.After(10 * time.Second):
			t.Fatal("writer timeout")
		}
		err := runner.Run(ctx, func(u fixturePorts) error { _, err := u.Load(ctx, input.ReferenceID); return err })
		close(release)
		<-done
		if !errors.Is(err, eventstore.ErrAggregateNotFound) {
			t.Fatalf("uncommitted history visible: %v", err)
		}
		func() {
			defer func() {
				if recover() == nil {
					t.Error("panic was swallowed")
				}
			}()
			_ = runner.Run(ctx, func(u fixturePorts) error {
				if _, err := u.Record(ctx, scope, input); err != nil {
					t.Fatal(err)
				}
				panic("synthetic")
			})
		}()
		if err := runner.Run(ctx, func(u fixturePorts) error { _, err := u.Record(ctx, scope, input); return err }); err != nil {
			t.Fatalf("panic held lock or leaked effect: %v", err)
		}
	})
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := runner.Run(cancelled, func(fixturePorts) error { t.Fatal("cancelled callback"); return nil }); err == nil {
		t.Fatal("cancelled transaction succeeded")
	}
}

// Synthetic feature ports demonstrate explicit DI and typed shared mechanics;
// these are test-only references, not invented Identity business schemas.
type fixtureValue struct {
	ReferenceID string `json:"referenceId"`
}
type fixturePorts interface {
	Record(context.Context, commands.Scope, fixtureValue) (commands.Receipt[fixtureValue], error)
	Load(context.Context, string) (fixtureValue, error)
}
type fixtureAdapter struct {
	append     *eventstore.Appender
	load       *eventstore.Loader
	ledger     *commands.Ledger[fixtureValue, fixtureValue]
	operations *commands.Operations[fixtureValue, fixtureValue, fixtureValue]
	schema     events.EventSchema[fixtureValue]
}

func validate(v fixtureValue) error {
	if _, err := uuid.Parse(v.ReferenceID); err != nil {
		return err
	}
	return nil
}

func TestFixtureRefusesUnownedIdentityOrMounts(t *testing.T) {
	name, token := "justixauto-t931-"+uuid.NewString(), uuid.NewString()
	root := "/explicit/task/root"
	base := containerIdentity{ID: strings.Repeat("a", 64), Name: "/" + name, Image: image, Labels: map[string]string{fixtureLabel: token}, Tmpfs: map[string]string{"/var/lib/postgresql": "rw"}}
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
	if os.Getenv("JUSTIXAUTO_TEST_IDENTITY_MECHANICS") != "1" {
		t.Skip("owned PostgreSQL opt-in required")
	}
	if os.Getenv("JUSTIXAUTO_T931_FIXTURE_CHILD") != "" {
		start(t)
		t.Fatal("expected allocation failure")
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../../../..")
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
				wrapper += " shift\n status=0\n " + quote(docker) + " create --t931-invalid-option \"$@\" || status=$?\n if [ \"$status\" = 0 ]; then exit 1; fi\n echo \"T931 actual allocation rejected: $status\" >&2\n"
			}
			wrapper += " echo 'T931 synthetic " + mode + " failure' >&2\n exit 42\nfi\nexec " + quote(docker) + " \"$@\"\n"
			if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(wrapper), 0700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFixtureAllocationFailures$", "-test.v")
			child.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "JUSTIXAUTO_T931_FIXTURE_CHILD="+mode)
			out, err := child.CombinedOutput()
			t.Logf("expected child failure: %v\n%s", err, out)
			if err == nil || !strings.Contains(string(out), "T931 synthetic "+mode+" failure") {
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
				if !strings.Contains(string(out), "unknown flag") || !strings.Contains(string(out), "T931 actual allocation rejected:") || !strings.Contains(string(out), "fixture absent after failed allocation: "+name) {
					t.Fatal("actual pre-allocation rejection and absence not proven")
				}
				if _, err := os.Stat(idfile); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("unexpected allocation receipt")
				}
			}
		})
	}
}

func digest(s string) persistence.Digest    { return persistence.Digest(sha256.Sum256([]byte(s))) }
func digestHex(d persistence.Digest) string { return hex.EncodeToString(d[:]) }

func compatibleSpec() persistence.Specification {
	base := persistence.ArtifactIdentity{Version: 1, Filename: "migrations/0001_mechanics.up.sql", SHA256: digest("synthetic base")}
	feature := persistence.FeatureIdentity{ID: "identity.synthetic.fixture", Revision: 1, SHA256: digest("synthetic exact items(id,body,immutable); SELECT; INSERT(id,body); UPDATE(body)")}
	return persistence.Specification{Owner: "identity", Database: "justix_identity", RuntimeRole: "justix_identity_runtime", FormatRevision: 1, Head: 12, HistoryRevision: 1, HistorySHA256: digest("synthetic history"),
		Baseline:  persistence.Baseline{Kind: "verified-installation", EvidenceRef: "synthetic-execution", ApprovalRef: "synthetic-test-only", BackupRef: "synthetic-empty-fixture", StoppedRuntimesRef: "synthetic-no-runtimes", RequestID: uuid.New()},
		Artifacts: []persistence.Artifact{{Identity: base, ManifestSHA256: digest("synthetic manifest1")}, {Identity: persistence.ArtifactIdentity{Version: 12, Filename: "migrations/0012_synthetic.up.sql", SHA256: digest("synthetic feature")}, ManifestSHA256: digest("synthetic manifest12"), Prerequisites: []persistence.ArtifactIdentity{base}, Feature: &feature}},
		Features:  []persistence.Feature{{Identity: feature, Tables: []persistence.TableGrant{{Schema: "synthetic_feature", Table: "items", Select: true, InsertColumns: []string{"id", "body"}, UpdateColumns: []string{"body"}}}}},
		Shared:    persistence.Shared{Mode: "legacy", Artifacts: []persistence.SharedArtifact{{Revision: 1, Filename: "pkg/eventstore/schema.sql", SHA256: digest("synthetic shared")}}}}
}

func mustProfile(t *testing.T, s persistence.Specification) persistence.Profile {
	t.Helper()
	p, err := persistence.NewProfile(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCompatibleConstructionIsInertAndOwnerBound(t *testing.T) {
	db := &gorm.DB{Config: &gorm.Config{}}
	called := false
	bind := func(*gorm.DB) (struct{}, error) { called = true; return struct{}{}, nil }
	p := mustProfile(t, compatibleSpec())
	if _, err := identity.NewCompatibleUnitOfWork(db, p, bind); err != nil || called {
		t.Fatalf("not inert: %v", err)
	}
	for _, tc := range []struct {
		db   *gorm.DB
		p    persistence.Profile
		bind func(*gorm.DB) (struct{}, error)
	}{
		{nil, p, bind}, {&gorm.DB{Error: errors.New("failed handle")}, p, bind}, {db, p, nil}, {db, persistence.Profile{}, bind},
	} {
		if _, err := identity.NewCompatibleUnitOfWork(tc.db, tc.p, tc.bind); !errors.Is(err, identity.ErrIncompatiblePersistence) {
			t.Fatalf("partial constructor accepted: %v", err)
		}
	}
	for _, owner := range []string{"inventory", "commerce", "retail", "financing", "insurance", "documents"} {
		s := compatibleSpec()
		s.Owner = owner
		s.Database = "justix_" + owner
		s.RuntimeRole = s.Database + "_runtime"
		if _, err := identity.NewCompatibleUnitOfWork(db, mustProfile(t, s), bind); !errors.Is(err, identity.ErrIncompatiblePersistence) {
			t.Fatalf("foreign %s profile accepted: %v", owner, err)
		}
	}
	var zero identity.UnitOfWork[struct{}]
	if zero.Check(context.Background()) == nil || zero.Run(context.Background(), func(struct{}) error { return nil }) == nil {
		t.Fatal("zero value accepted")
	}
}

// Only this synthetic schema has a version-12 feature contract. It is not auth
// policy, a production release manifest or proof that the future driver ran.
func (f fixture) compatibleInstallation() (persistence.Profile, string) {
	t := f.t
	s := compatibleSpec()
	f.must("justix_identity", "postgres", "CREATE ROLE justix_identity_runtime LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_identity TO justix_identity_runtime; REVOKE TEMP ON DATABASE justix_identity FROM PUBLIC")
	install := func(path string, params ...string) {
		t.Helper()
		args := []string{"-v", "owner_service=identity", "-v", "runtime_role=justix_identity_runtime"}
		for _, p := range params {
			args = append(args, "-v", p)
		}
		if out, err := f.sql("justix_identity", "justix_identity", f.file(path), args...); err != nil {
			t.Fatalf("install %s: %v %s", path, err, out)
		}
	}
	install("pkg/eventstore/schema.sql")
	f.must("justix_identity", "justix_identity", "ALTER DEFAULT PRIVILEGES REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC; ALTER DEFAULT PRIVILEGES REVOKE USAGE ON TYPES FROM PUBLIC; CREATE TABLE public.schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL); INSERT INTO public.schema_migrations VALUES(1,true)")
	baseSQL := f.file("services/identity/migrations/0001_mechanics.up.sql")
	f.must("justix_identity", "justix_identity", baseSQL)
	f.must("justix_identity", "justix_identity", "UPDATE public.schema_migrations SET dirty=false")
	s.Artifacts[0].Identity.SHA256 = digest(baseSQL)
	s.Artifacts[1].Prerequisites[0] = s.Artifacts[0].Identity
	s.HistorySHA256 = digest(f.file("pkg/eventstore/owner_history.sql"))
	s.Shared.Artifacts[0].SHA256 = digest(f.file("pkg/eventstore/schema.sql"))
	featureSQL := fmt.Sprintf(`BEGIN;
CREATE SCHEMA synthetic_feature; REVOKE ALL ON SCHEMA synthetic_feature FROM PUBLIC;
GRANT USAGE ON SCHEMA synthetic_feature TO justix_identity_runtime;
CREATE TABLE synthetic_feature.items(id uuid PRIMARY KEY,body text NOT NULL,immutable text NOT NULL DEFAULT 'retained');
GRANT SELECT ON synthetic_feature.items TO justix_identity_runtime;
GRANT INSERT(id,body),UPDATE(body) ON synthetic_feature.items TO justix_identity_runtime;
INSERT INTO owner_migrations.feature_contracts VALUES('%s',1,12,decode('%s','hex'));
COMMIT;`, s.Features[0].Identity.ID, digestHex(s.Features[0].Identity.SHA256))
	s.Artifacts[1].Identity.SHA256 = digest(featureSQL)
	bundle, err := os.MkdirTemp("", "justixauto-t931-evidence-")
	if err != nil {
		t.Fatal(err)
	}
	bundle, err = filepath.EvalSymlinks(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(bundle, "migrations"), 0700); err != nil {
		t.Fatal(err)
	}
	for i, sqlBytes := range []string{baseSQL, featureSQL} {
		a := s.Artifacts[i]
		manifest, err := json.Marshal(persistence.ArtifactManifest{FormatRevision: 1, Owner: s.Owner, Identity: a.Identity, Prerequisites: a.Prerequisites, Feature: a.Feature})
		if err != nil {
			t.Fatal(err)
		}
		s.Artifacts[i].ManifestSHA256 = digest(string(manifest))
		path := filepath.Join(bundle, a.Identity.Filename)
		if err := os.WriteFile(path, []byte(sqlBytes), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(strings.TrimSuffix(path, ".up.sql")+".manifest.json", manifest, 0600); err != nil {
			t.Fatal(err)
		}
	}
	p := mustProfile(t, s)
	profileBytes, err := json.MarshalIndent(p.Specification(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	retainEvidence(t, bundle, "profile.json", string(profileBytes))
	t.Logf("retained synthetic evidence directory %s", bundle)
	if err := p.VerifyFiles(bundle); err != nil {
		t.Fatal(err)
	}
	install("pkg/eventstore/owner_history.sql", "template_sha256="+digestHex(s.HistorySHA256), "base_schema_sha256="+digestHex(s.Shared.Artifacts[0].SHA256), "base_artifact_sha256="+digestHex(s.Artifacts[0].Identity.SHA256), "base_manifest_sha256="+digestHex(s.Artifacts[0].ManifestSHA256), "profile_sha256="+digestHex(p.Digest()), "baseline_kind="+s.Baseline.Kind, "evidence_ref="+s.Baseline.EvidenceRef, "approval_ref="+s.Baseline.ApprovalRef, "backup_ref="+s.Baseline.BackupRef, "stopped_runtimes_ref="+s.Baseline.StoppedRuntimesRef, "request_id="+s.Baseline.RequestID.String())
	baseSpec := p.Specification()
	baseSpec.Head = 1
	baseSpec.Artifacts = baseSpec.Artifacts[:1]
	baseSpec.Features = nil
	base, err := identity.NewCompatibleUnitOfWork(f.open("justix_identity_runtime"), mustProfile(t, baseSpec), bindFixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := base.Check(context.Background()); err != nil {
		t.Fatalf("history plus base: %v", err)
	}
	f.must("justix_identity", "justix_identity", "UPDATE public.schema_migrations SET version=12,dirty=true")
	f.must("justix_identity", "justix_identity", featureSQL)
	a := s.Artifacts[1]
	f.must("justix_identity", "justix_identity", fmt.Sprintf(`BEGIN;
INSERT INTO owner_migrations.artifacts(version,filename,sql_sha256,predecessor_version,predecessor_sha256,feature_contract_id,feature_contract_revision,feature_contract_sha256,installation_request_id,artifact_manifest_sha256,installing_profile_sha256,provenance_kind,provenance_ref)
VALUES(12,'%s',decode('%s','hex'),1,decode('%s','hex'),'%s',1,decode('%s','hex'),'%s',decode('%s','hex'),decode('%s','hex'),'verified-installation','synthetic-feature-execution');
UPDATE public.schema_migrations SET dirty=false;COMMIT;`, a.Identity.Filename, digestHex(a.Identity.SHA256), digestHex(s.Artifacts[0].Identity.SHA256), a.Feature.ID, digestHex(a.Feature.SHA256), uuid.NewString(), digestHex(a.ManifestSHA256), digestHex(p.Digest())))
	if err := base.Check(context.Background()); !errors.Is(err, identity.ErrIncompatiblePersistence) {
		t.Fatalf("old explicit head accepted upgrade: %v", err)
	}
	// Keep original byte identities in test output; no credentials or row payloads.
	for _, a := range s.Artifacts {
		t.Logf("synthetic artifact %s sql=%s manifest=%s", a.Identity.Filename, digestHex(a.Identity.SHA256), digestHex(a.ManifestSHA256))
	}
	return p, bundle
}

func retainEvidence(t *testing.T, root, name, content string) {
	t.Helper()
	b := []byte(content + "\n")
	if err := os.WriteFile(filepath.Join(root, name), b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name+".sha256"), []byte(digestHex(digest(string(b)))+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) compatibilitySnapshot() string {
	return f.must("justix_identity", "postgres", `SELECT jsonb_build_object('ledger',(SELECT jsonb_agg(to_jsonb(x) ORDER BY version) FROM public.schema_migrations x),'baseline',(SELECT jsonb_agg(to_jsonb(x)) FROM owner_migrations.compatibility x),'history',(SELECT jsonb_agg(to_jsonb(x) ORDER BY version) FROM owner_migrations.artifacts x),'markers',(SELECT jsonb_agg(to_jsonb(x) ORDER BY migration_version) FROM owner_migrations.feature_contracts x),'items',(SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM synthetic_feature.items x),'events',(SELECT jsonb_agg(to_jsonb(x) ORDER BY event_id) FROM eventstore.events x),'receipts',(SELECT jsonb_agg(to_jsonb(x) ORDER BY receipt_id) FROM eventstore.command_receipts x))`)
}

func TestIdentityCompatiblePostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_IDENTITY_MECHANICS") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_IDENTITY_MECHANICS=1 for disposable PostgreSQL")
	}
	f := start(t)
	p, evidence := f.compatibleInstallation()
	db := f.open("justix_identity_runtime")
	ctx := context.Background()
	binds, calls := 0, 0
	r, err := identity.NewCompatibleUnitOfWork(db, p, func(tx *gorm.DB) (fixturePorts, error) {
		binds++
		var isolation string
		if err := tx.Raw("SHOW transaction_isolation").Scan(&isolation).Error; err != nil {
			return nil, err
		}
		if isolation != "read committed" {
			return nil, fmt.Errorf("wrong isolation %s", isolation)
		}
		if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
			return nil, errors.New("not transaction bound")
		}
		return bindFixture(tx)
	})
	if err != nil {
		t.Fatal(err)
	}
	accept := func() {
		t.Helper()
		before := binds
		if err := r.Check(ctx); err != nil {
			t.Fatal(err)
		}
		if binds != before {
			t.Fatal("startup bound factory")
		}
		if err := r.Run(ctx, func(fixturePorts) error { calls++; return nil }); err != nil {
			t.Fatal(err)
		}
		if binds != before+1 {
			t.Fatal("factory not bound once")
		}
	}
	reject := func() {
		t.Helper()
		b, c := binds, calls
		for _, err := range []error{r.Check(ctx), r.Run(ctx, func(fixturePorts) error { calls++; return nil })} {
			if !errors.Is(err, identity.ErrIncompatiblePersistence) {
				t.Fatalf("incompatible accepted: %v", err)
			}
		}
		if binds != b || calls != c {
			t.Fatal("incompatible state reached factory/callback")
		}
	}
	accept()
	// Both caller-owned input and returned snapshots are mutable copies only.
	copySpec := p.Specification()
	copySpec.Artifacts[1].Feature.ID = "changed"
	copySpec.Features[0].Tables[0].UpdateColumns[0] = "immutable"
	copySpec.Shared.Mode = "custody"
	accept()
	legacy, err := identity.NewUnitOfWork(db, func(tx *gorm.DB) (fixturePorts, error) {
		t.Fatal("legacy factory reached version12")
		return bindFixture(tx)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(legacy.Check(ctx), identity.ErrIncompatiblePersistence) || !errors.Is(legacy.Run(ctx, func(fixturePorts) error { t.Fatal("legacy callback"); return nil }), identity.ErrIncompatiblePersistence) {
		t.Fatal("legacy accepted 12")
	}
	// Preserve populated business rows as well as installation evidence across
	// all readiness successes and deliberate drift failures below.
	retained := fixtureValue{ReferenceID: uuid.NewString()}
	if err := r.Run(ctx, func(u fixturePorts) error {
		_, err := u.Record(ctx, commands.Scope{ActorID: uuid.NewString(), Key: uuid.NewString()}, retained)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	itemID := uuid.NewString()
	if err := db.Exec("INSERT INTO synthetic_feature.items(id,body) VALUES(?::uuid,?)", itemID, "retained body").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE synthetic_feature.items SET body=? WHERE id=?::uuid", "approved mutable column", itemID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE synthetic_feature.items SET immutable='forbidden' WHERE id=?::uuid", itemID).Error; err == nil {
		t.Fatal("adjacent immutable feature column writable")
	}
	before := f.compatibilitySnapshot()
	retainEvidence(t, evidence, "before-readiness.json", before)
	for _, tc := range []struct{ name, set, undo string }{
		{"dirty", "UPDATE public.schema_migrations SET dirty=true", "UPDATE public.schema_migrations SET dirty=false"},
		{"wrong-head", "UPDATE public.schema_migrations SET version=17", "UPDATE public.schema_migrations SET version=12"},
		{"extra-ledger", "INSERT INTO public.schema_migrations VALUES(17,false)", "DELETE FROM public.schema_migrations WHERE version=17"},
		{"missing-feature-select", "REVOKE SELECT ON synthetic_feature.items FROM justix_identity_runtime", "GRANT SELECT ON synthetic_feature.items TO justix_identity_runtime"},
		{"missing-feature-update", "REVOKE UPDATE(body) ON synthetic_feature.items FROM justix_identity_runtime", "GRANT UPDATE(body) ON synthetic_feature.items TO justix_identity_runtime"},
		{"broad-feature-update", "GRANT UPDATE ON synthetic_feature.items TO justix_identity_runtime", "REVOKE UPDATE ON synthetic_feature.items FROM justix_identity_runtime;GRANT UPDATE(body) ON synthetic_feature.items TO justix_identity_runtime"},
		{"public-immutable-column", "GRANT UPDATE(immutable) ON synthetic_feature.items TO PUBLIC", "REVOKE UPDATE(immutable) ON synthetic_feature.items FROM PUBLIC"},
		{"history-select", "REVOKE SELECT ON owner_migrations.artifacts FROM justix_identity_runtime", "GRANT SELECT ON owner_migrations.artifacts TO justix_identity_runtime"},
		{"history-write", "GRANT INSERT(version) ON owner_migrations.artifacts TO justix_identity_runtime", "REVOKE INSERT(version) ON owner_migrations.artifacts FROM justix_identity_runtime"},
		{"function-default", "ALTER DEFAULT PRIVILEGES GRANT EXECUTE ON FUNCTIONS TO PUBLIC", "ALTER DEFAULT PRIVILEGES REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC"},
		{"schema-create", "GRANT CREATE ON SCHEMA synthetic_feature TO justix_identity_runtime", "REVOKE CREATE ON SCHEMA synthetic_feature FROM justix_identity_runtime"},
		{"mechanics-owner-default", "ALTER TABLE eventstore.events ALTER COLUMN owner_service SET DEFAULT 'inventory'", "ALTER TABLE eventstore.events ALTER COLUMN owner_service SET DEFAULT 'identity'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.must("justix_identity", "justix_identity", tc.set)
			reject()
			f.must("justix_identity", "justix_identity", tc.undo)
			accept()
		})
	}
	for _, tc := range []struct {
		name   string
		mutate func(*persistence.Specification)
	}{
		{"wrong-baseline", func(s *persistence.Specification) { s.Baseline.EvidenceRef = "other" }},
		{"wrong-sql", func(s *persistence.Specification) { s.Artifacts[1].Identity.SHA256 = digest("other") }},
		{"wrong-manifest", func(s *persistence.Specification) { s.Artifacts[1].ManifestSHA256 = digest("other") }},
		{"wrong-feature", func(s *persistence.Specification) {
			s.Artifacts[1].Feature.SHA256 = digest("other")
			s.Features[0].Identity.SHA256 = digest("other")
		}},
		{"wrong-predecessor", func(s *persistence.Specification) {
			s.Artifacts[0].Identity.SHA256 = digest("other")
			s.Artifacts[1].Prerequisites[0] = s.Artifacts[0].Identity
		}},
		{"custody-unsupported", func(s *persistence.Specification) { s.Shared.Mode = "custody" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := p.Specification()
			tc.mutate(&s)
			bad, err := identity.NewCompatibleUnitOfWork(db, mustProfile(t, s), func(*gorm.DB) (struct{}, error) { t.Fatal("bad profile bound"); return struct{}{}, nil })
			if err != nil {
				t.Fatal(err)
			}
			if !errors.Is(bad.Check(ctx), identity.ErrIncompatiblePersistence) || !errors.Is(bad.Run(ctx, func(struct{}) error { t.Fatal("bad profile callback"); return nil }), identity.ErrIncompatiblePersistence) {
				t.Fatal("bad profile accepted")
			}
		})
	}
	t.Run("actual-history-drift", func(t *testing.T) {
		for _, tc := range []struct{ name, table, change string }{
			{"missing-receipt", "artifacts", "DELETE FROM owner_migrations.artifacts WHERE version=12"},
			{"missing-marker", "feature_contracts", "DELETE FROM owner_migrations.feature_contracts"},
			{"marker-hash", "feature_contracts", "UPDATE owner_migrations.feature_contracts SET contract_sha256=decode(repeat('ab',32),'hex')"},
			{"broken-predecessor", "artifacts", "UPDATE owner_migrations.artifacts SET predecessor_sha256=decode(repeat('ab',32),'hex') WHERE version=12"},
			{"history-hash", "artifacts", "UPDATE owner_migrations.artifacts SET sql_sha256=decode(repeat('ab',32),'hex') WHERE version=12"},
			{"baseline", "compatibility", "UPDATE owner_migrations.compatibility SET evidence_ref='changed-fixture'"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				local := f
				local.t = t
				// Deliberate fixture corruption is performed only as this disposable
				// cluster's administrator. All guards are enabled during readiness.
				local.must("justix_identity", "postgres", "CREATE TABLE public.t931_saved AS TABLE owner_migrations."+tc.table+"; ALTER TABLE owner_migrations."+tc.table+" DISABLE TRIGGER ALL; "+tc.change+"; ALTER TABLE owner_migrations."+tc.table+" ENABLE TRIGGER ALL")
				defer local.must("justix_identity", "postgres", "ALTER TABLE owner_migrations."+tc.table+" DISABLE TRIGGER ALL; DELETE FROM owner_migrations."+tc.table+"; INSERT INTO owner_migrations."+tc.table+" SELECT * FROM public.t931_saved; ALTER TABLE owner_migrations."+tc.table+" ENABLE TRIGGER ALL; DROP TABLE public.t931_saved")
				reject()
			})
			accept()
		}
	})
	t.Run("transitive-role-and-foreign-database", func(t *testing.T) {
		f.must("justix_identity", "postgres", "CREATE ROLE t931_outer NOINHERIT; CREATE ROLE t931_inner NOINHERIT; GRANT t931_inner TO t931_outer; GRANT t931_outer TO justix_identity_runtime; GRANT UPDATE(immutable) ON synthetic_feature.items TO t931_inner")
		reject()
		f.must("justix_identity", "postgres", "REVOKE UPDATE(immutable) ON synthetic_feature.items FROM t931_inner; REVOKE t931_outer FROM justix_identity_runtime; DROP ROLE t931_outer; DROP ROLE t931_inner")
		accept()
		f.must("justix_identity", "postgres", "GRANT CONNECT ON DATABASE justix_inventory TO justix_identity_runtime")
		reject()
		f.must("justix_identity", "postgres", "REVOKE CONNECT ON DATABASE justix_inventory FROM justix_identity_runtime")
		accept()
	})
	after := f.compatibilitySnapshot()
	retainEvidence(t, evidence, "after-readiness.json", after)
	if after != before {
		t.Fatal("readiness mutated retained state")
	}
	t.Logf("read-only success/failure retained snapshot sha256=%s", digestHex(digest(before)))
	t.Run("read-only-startup", func(t *testing.T) {
		tx := db.Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: true})
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		defer tx.Rollback()
		readonly, err := identity.NewCompatibleUnitOfWork(tx, p, bindFixture)
		if err != nil {
			t.Fatal(err)
		}
		if err := readonly.Check(ctx); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("migration-owner-rejected", func(t *testing.T) {
		bad, err := identity.NewCompatibleUnitOfWork(f.open("justix_identity"), p, func(*gorm.DB) (struct{}, error) { t.Fatal("owner bound"); return struct{}{}, nil })
		if err != nil {
			t.Fatal(err)
		}
		if !errors.Is(bad.Check(ctx), identity.ErrIncompatiblePersistence) || !errors.Is(bad.Run(ctx, func(struct{}) error { return nil }), identity.ErrIncompatiblePersistence) {
			t.Fatal("migration role accepted")
		}
	})
	t.Run("cancel-before-bind", func(t *testing.T) {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		b := binds
		if r.Run(cancelled, func(fixturePorts) error { t.Fatal("cancelled callback"); return nil }) == nil || binds != b {
			t.Fatal("cancelled run bound")
		}
		if r.Check(cancelled) == nil {
			t.Fatal("cancelled startup accepted")
		}
	})
	t.Run("atomic-replay-concurrent-idempotency", func(t *testing.T) {
		parallel, err := identity.NewCompatibleUnitOfWork(db, p, bindFixture)
		if err != nil {
			t.Fatal(err)
		}
		scope := commands.Scope{ActorID: uuid.NewString(), Key: uuid.NewString()}
		value := fixtureValue{ReferenceID: uuid.NewString()}
		abort := errors.New("rollback")
		work := func(u fixturePorts) error { _, err := u.Record(ctx, scope, value); return err }
		if err := parallel.Run(ctx, func(u fixturePorts) error {
			if err := work(u); err != nil {
				return err
			}
			return abort
		}); !errors.Is(err, abort) {
			t.Fatal(err)
		}
		if err := parallel.Run(ctx, func(u fixturePorts) error { _, err := u.Load(ctx, value.ReferenceID); return err }); !errors.Is(err, eventstore.ErrAggregateNotFound) {
			t.Fatalf("rollback leaked: %v", err)
		}
		start := make(chan struct{})
		errs := make(chan error, 6)
		var wg sync.WaitGroup
		for range 6 {
			wg.Go(func() { <-start; errs <- parallel.Run(ctx, work) })
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if got := f.must("justix_identity", "justix_identity", "SELECT count(*) FROM eventstore.events WHERE aggregate_id='"+value.ReferenceID+"'"); got != "1" {
			t.Fatalf("duplicate effects %s", got)
		}
		if err := parallel.Run(ctx, func(u fixturePorts) error {
			v, err := u.Load(ctx, value.ReferenceID)
			if err == nil && v != value {
				return errors.New("wrong replay")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("checks-use-actual-transaction", func(t *testing.T) {
		pool, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		probe := &observedPool{DB: pool}
		observed, err := gorm.Open(postgres.New(postgres.Config{Conn: probe}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		checked, err := identity.NewCompatibleUnitOfWork(observed, p, func(tx *gorm.DB) (fixturePorts, error) {
			if probe.txQueries == 0 || probe.poolQueries != 0 {
				t.Fatalf("checks used wrong handle: tx=%d pool=%d", probe.txQueries, probe.poolQueries)
			}
			return bindFixture(tx)
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := checked.Check(ctx); err != nil {
			t.Fatal(err)
		}
		if probe.poolQueries == 0 || probe.txQueries != 0 {
			t.Fatal("startup did not use supplied pool")
		}
		probe.poolQueries = 0
		for range 2 {
			previous := probe.txQueries
			if err := checked.Run(ctx, func(fixturePorts) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if probe.txQueries <= previous {
				t.Fatal("transaction reused cached check")
			}
		}
	})
	t.Run("commit-outcome-remains-unknown", func(t *testing.T) {
		pool, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		for _, commitFirst := range []bool{false, true} {
			fault, err := gorm.Open(postgres.New(postgres.Config{Conn: commitFaultPool{DB: pool, commitFirst: commitFirst}}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			runner, err := identity.NewCompatibleUnitOfWork(fault, p, bindFixture)
			if err != nil {
				t.Fatal(err)
			}
			scope := commands.Scope{ActorID: uuid.NewString(), Key: uuid.NewString()}
			value := fixtureValue{ReferenceID: uuid.NewString()}
			calls := 0
			err = runner.Run(ctx, func(u fixturePorts) error { calls++; _, err := u.Record(ctx, scope, value); return err })
			if !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) || !errors.Is(err, io.ErrUnexpectedEOF) || calls != 1 {
				t.Fatalf("unknown commit: %v calls=%d", err, calls)
			}
			want := "0"
			if commitFirst {
				want = "1"
			}
			if got := f.must("justix_identity", "justix_identity", "SELECT count(*) FROM eventstore.command_receipts WHERE idempotency_key='"+scope.Key+"'"); got != want {
				t.Fatalf("authoritative receipt %s want %s", got, want)
			}
		}
	})
	t.Run("actual-mode-cannot-be-replaced-by-grants", func(t *testing.T) {
		path := "pkg/eventstore/migrations/000002_messaging_delivery.up.sql"
		if out, err := f.sql("justix_identity", "justix_identity", f.file(path), "-v", "owner_service=identity", "-v", "runtime_role=justix_identity_runtime"); err != nil {
			t.Fatalf("messaging install: %v %s", err, out)
		}
		reject() // The old explicit profile did not declare this installed lineage.
		s := p.Specification()
		s.Shared.Artifacts = append(s.Shared.Artifacts, persistence.SharedArtifact{Revision: 2, Filename: path, SHA256: digest(f.file(path))})
		binds, calls := 0, 0
		checked, err := identity.NewCompatibleUnitOfWork(db, mustProfile(t, s), func(tx *gorm.DB) (fixturePorts, error) { binds++; return bindFixture(tx) })
		if err != nil {
			t.Fatal(err)
		}
		if err := checked.Check(ctx); err != nil {
			t.Fatal(err)
		}
		if err := checked.Run(ctx, func(fixturePorts) error { calls++; return nil }); err != nil {
			t.Fatal(err)
		}
		f.must("justix_identity", "justix_identity", "SELECT eventstore.activate_messaging_custody('"+uuid.NewString()+"',decode(repeat('ab',32),'hex'),decode(repeat('cd',32),'hex'),'synthetic-fixture-backup','no-runtimes-started','no-broker-fixture','explicit-profile-incompatible')")
		f.must("justix_identity", "justix_identity", "GRANT INSERT ON eventstore.outbox TO justix_identity_runtime; GRANT UPDATE(attempts,next_attempt_at,lease_owner,lease_until,sent_at) ON eventstore.outbox TO justix_identity_runtime")
		if mode := f.must("justix_identity", "justix_identity_runtime", "SELECT mode FROM eventstore.messaging_mode"); mode != "custody" {
			t.Fatalf("mode %s", mode)
		}
		if !errors.Is(checked.Check(ctx), identity.ErrIncompatiblePersistence) || !errors.Is(checked.Run(ctx, func(fixturePorts) error { calls++; return nil }), identity.ErrIncompatiblePersistence) || binds != 1 || calls != 1 {
			t.Fatalf("custody reached factory/callback: binds=%d calls=%d", binds, calls)
		}
	})
}

type observedPool struct {
	*sql.DB
	poolQueries, txQueries int
}

func (p *observedPool) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	p.poolQueries++
	return p.DB.QueryContext(ctx, q, args...)
}
func (p *observedPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	if opts == nil || opts.Isolation != sql.LevelReadCommitted {
		return nil, errors.New("not ReadCommitted")
	}
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &observedTx{Tx: tx, p: p}, nil
}

type observedTx struct {
	*sql.Tx
	p *observedPool
}

func (tx *observedTx) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	tx.p.txQueries++
	return tx.Tx.QueryContext(ctx, q, args...)
}

type commitFaultPool struct {
	*sql.DB
	commitFirst bool
}

func (p commitFaultPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &commitFaultTx{Tx: tx, commitFirst: p.commitFirst}, nil
}

type commitFaultTx struct {
	*sql.Tx
	commitFirst bool
}

func (tx commitFaultTx) Commit() error {
	var err error
	if tx.commitFirst {
		err = tx.Tx.Commit()
	} else {
		err = tx.Tx.Rollback()
	}
	if err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}
func bindFixture(tx *gorm.DB) (fixturePorts, error) {
	a := &fixtureAdapter{}
	var err error
	a.append, err = eventstore.NewAppender(tx, events.OwnerIdentity)
	if err != nil {
		return nil, err
	}
	a.load, err = eventstore.NewLoader(tx, events.OwnerIdentity)
	if err != nil {
		return nil, err
	}
	a.schema, err = events.NewEventSchema("identity.fixture.recorded.v1", 1, "fixture", events.CompanyScopeGlobalAllowed, validate)
	if err != nil {
		return nil, err
	}
	schema, err := commands.NewSchema(events.OwnerIdentity, "identity.fixture.record", true, func(v fixtureValue) (fixtureValue, error) { return v, validate(v) }, validate)
	if err != nil {
		return nil, err
	}
	a.ledger, err = commands.NewLedger(tx, schema)
	if err != nil {
		return nil, err
	}
	a.operations, err = commands.NewOperations(tx, commands.OperationPolicy[fixtureValue, fixtureValue, fixtureValue]{Schema: schema, Phases: map[string]bool{"pending": false}, Initial: func(commands.Operation[fixtureValue, fixtureValue]) error { return nil }, Transition: func(commands.Operation[fixtureValue, fixtureValue], commands.Operation[fixtureValue, fixtureValue]) error {
		return nil
	}, ValidateError: validate})
	return a, err
}
func (a *fixtureAdapter) Record(ctx context.Context, scope commands.Scope, input fixtureValue) (commands.Receipt[fixtureValue], error) {
	receipt, _, err := a.ledger.Execute(ctx, scope, input, nil, func(ctx context.Context) (commands.Receipt[fixtureValue], error) {
		revision, _ := events.NewRevision(1)
		operationID := uuid.NewString()
		event, err := eventstore.NewInternalEvent(events.EnvelopeInput{EventID: uuid.NewString(), AggregateID: input.ReferenceID, AggregateVersion: revision, OccurredAt: time.Now().UTC(), Actor: events.Actor{Kind: events.ActorUser, ID: scope.ActorID}, CorrelationID: uuid.NewString(), CausationID: uuid.NewString(), OperationID: operationID}, a.schema, input)
		if err != nil {
			return commands.Receipt[fixtureValue]{}, err
		}
		if err := a.append.Append(ctx, eventstore.Batch{Events: []eventstore.Event{event}}); err != nil {
			return commands.Receipt[fixtureValue]{}, err
		}
		if err := a.operations.Create(ctx, commands.OperationIntent{ID: operationID, AggregateID: input.ReferenceID, ActorID: scope.ActorID, AdmissionRef: uuid.NewString(), Phase: "pending"}, input); err != nil {
			return commands.Receipt[fixtureValue]{}, err
		}
		return commands.NewReceipt(201, input, revision, operationID)
	})
	return receipt, err
}
func (a *fixtureAdapter) Load(ctx context.Context, id string) (fixtureValue, error) {
	stream := eventstore.Stream{Owner: events.OwnerIdentity, AggregateType: "fixture", AggregateID: id}
	history, err := a.load.Load(ctx, stream)
	if err != nil {
		return fixtureValue{}, err
	}
	result, err := eventstore.Replay(ctx, stream, history, []eventstore.Decoder[fixtureValue]{eventstore.DecodeWith(a.schema)}, func() fixtureValue { return fixtureValue{} }, func(_ fixtureValue, _ eventstore.ReplayMetadata, v fixtureValue) (fixtureValue, error) { return v, nil })
	return result.State, err
}
