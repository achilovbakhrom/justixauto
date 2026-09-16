package postgres_test

import (
	"context"
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
	insurance "justixauto/services/insurance/adapter/postgres"
	"justixauto/services/insurance/port"
)

func TestConstructionIsInertAndRejectsMissingDependencies(t *testing.T) {
	if _, err := insurance.NewUnitOfWork[struct{}](nil, func(*gorm.DB) (struct{}, error) { return struct{}{}, nil }); err == nil {
		t.Fatal("nil database accepted")
	}
	db := &gorm.DB{Config: &gorm.Config{}}
	if _, err := insurance.NewUnitOfWork[struct{}](db, nil); err == nil {
		t.Fatal("nil factory accepted")
	}
	called := false
	if _, err := insurance.NewUnitOfWork(db, func(*gorm.DB) (struct{}, error) { called = true; return struct{}{}, nil }); err != nil || called {
		t.Fatalf("construction was not inert: %v", err)
	}
	var empty *insurance.UnitOfWork[struct{}]
	if empty.Check(context.Background()) == nil || empty.Run(context.Background(), nil) == nil {
		t.Fatal("zero runner accepted")
	}
}

type fixture struct {
	t                          *testing.T
	name, password, root, port string
}

const pgImage = "docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"
const fixtureLabel = "justixauto.t029.fixture-owner"

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
	nameUUID, err := uuid.Parse(strings.TrimPrefix(name, "justixauto-t029-"))
	if err != nil || nameUUID == uuid.Nil || name != "justixauto-t029-"+nameUUID.String() {
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

func start(t *testing.T) fixture {
	t.Helper()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{t: t, name: "justixauto-t029-" + uuid.NewString(), password: "synthetic-" + uuid.NewString(), root: root}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	token := uuid.NewString()
	if _, exists, err := inspectFixture(ctx, "docker", f.name); err != nil || exists {
		t.Fatalf("fresh fixture name not proven absent: %v", err)
	}
	// Registered before create: even a failed CLI reply may have allocated it.
	t.Cleanup(func() { cleanupFixture(t, "docker", f.name, token, root) })
	args := []string{"create", "--name", f.name, "--label", fixtureLabel + "=" + token, "--tmpfs", "/var/lib/postgresql:rw", "-p", "127.0.0.1::5432", "-e", "POSTGRES_PASSWORD=" + f.password, "-v", filepath.Join(root, "infra/local/postgres/init-owners.sql") + ":/docker-entrypoint-initdb.d/10-owners.sql:ro"}
	for _, owner := range []string{"identity", "inventory", "commerce", "retail", "financing", "insurance", "documents"} {
		args = append(args, "-e", "JUSTIXAUTO_"+strings.ToUpper(owner)+"_DB_PASSWORD="+f.password)
	}
	args = append(args, pgImage)
	if out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
		t.Fatalf("create owned fixture: %v %s", err, out)
	}
	v, exists, err := inspectFixture(ctx, "docker", f.name)
	if err != nil || !exists {
		t.Fatalf("created fixture unavailable: %v", err)
	}
	if err := v.validate(f.name, token, root); err != nil {
		t.Fatal(err)
	}
	t.Logf("created owned fixture %s id %s image %s; attached volume IDs []", f.name, v.ID, pgImage)
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
	if f.must("justix_insurance", "postgres", "SHOW server_version_num") != "180006" {
		t.Fatal("wrong PostgreSQL version")
	}
	out, err := exec.CommandContext(ctx, "docker", "port", v.ID, "5432/tcp").Output()
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
	dsn := fmt.Sprintf("host=127.0.0.1 port=%s user=%s password=%s dbname=justix_insurance sslmode=disable", f.port, role, f.password)
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
func TestInsurancePostgresMechanics(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_INSURANCE_MECHANICS") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_INSURANCE_MECHANICS=1 for disposable PostgreSQL")
	}
	f := start(t)
	ctx := context.Background()
	f.must("justix_insurance", "postgres", "CREATE ROLE justix_insurance_runtime LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_insurance TO justix_insurance_runtime;")
	db := f.open("justix_insurance_runtime")
	runner, err := insurance.NewUnitOfWork(db, func(tx *gorm.DB) (fixturePorts, error) { return bindFixture(tx) })
	if err != nil {
		t.Fatal(err)
	}
	var dependencies = port.Dependencies[fixturePorts]{Transactions: runner, Persistence: runner}
	reject := func() {
		t.Helper()
		if !errors.Is(dependencies.Persistence.Check(ctx), insurance.ErrIncompatiblePersistence) {
			t.Fatal("incompatible state ready")
		}
		called := false
		err := dependencies.Transactions.Run(ctx, func(fixturePorts) error { called = true; return nil })
		if !errors.Is(err, insurance.ErrIncompatiblePersistence) || called {
			t.Fatalf("unready callback executed: %v %v", called, err)
		}
	}
	reject()
	migration := f.file("services/insurance/migrations/0001_mechanics.up.sql")
	f.must("justix_insurance", "justix_insurance", "CREATE TABLE public.schema_migrations(version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL); INSERT INTO public.schema_migrations VALUES (1,true);")
	if _, err := f.sql("justix_insurance", "justix_insurance", migration); err == nil {
		t.Fatal("migration accepted absent shared schema")
	}
	if out, err := f.sql("justix_insurance", "justix_insurance", f.file("pkg/eventstore/schema.sql"), "-v", "owner_service=insurance", "-v", "runtime_role=justix_insurance_runtime"); err != nil {
		t.Fatalf("shared install: %v %s", err, out)
	}
	reject()
	if _, err := f.sql("justix_identity", "justix_identity", migration); err == nil {
		t.Fatal("foreign migration accepted")
	}
	if out, err := f.sql("justix_insurance", "justix_insurance", migration); err != nil {
		t.Fatalf("owner migration: %v %s", err, out)
	}
	reject() // installation is not ready while the runner's ledger remains dirty
	f.must("justix_insurance", "justix_insurance", "UPDATE public.schema_migrations SET dirty=false;")
	if err := runner.Check(ctx); err != nil {
		t.Fatalf("clean installation: %v", err)
	}
	if _, err := f.sql("justix_insurance", "justix_insurance", migration); err == nil {
		t.Fatal("silently reapplied migration")
	}
	for _, change := range []struct{ set, reset string }{
		{"UPDATE public.schema_migrations SET dirty=true", "UPDATE public.schema_migrations SET dirty=false"},
		{"UPDATE public.schema_migrations SET version=2", "UPDATE public.schema_migrations SET version=1"},
		{"GRANT UPDATE ON eventstore.events TO justix_insurance_runtime", "REVOKE UPDATE ON eventstore.events FROM justix_insurance_runtime"},
		{"GRANT UPDATE (data) ON eventstore.events TO justix_insurance_runtime", "REVOKE UPDATE (data) ON eventstore.events FROM justix_insurance_runtime"},
		{"GRANT UPDATE ON public.schema_migrations TO justix_insurance_runtime", "REVOKE UPDATE ON public.schema_migrations FROM justix_insurance_runtime"},
		{"REVOKE UPDATE (phase) ON eventstore.operations FROM justix_insurance_runtime", "GRANT UPDATE (phase) ON eventstore.operations TO justix_insurance_runtime"},
		{"ALTER TABLE eventstore.events ALTER COLUMN owner_service SET DEFAULT 'identity'", "ALTER TABLE eventstore.events ALTER COLUMN owner_service SET DEFAULT 'insurance'"},
	} {
		f.must("justix_insurance", "justix_insurance", change.set)
		reject()
		f.must("justix_insurance", "justix_insurance", change.reset)
	}
	f.must("justix_insurance", "postgres", "GRANT pg_write_all_data TO justix_insurance_runtime WITH INHERIT FALSE;")
	reject()
	f.must("justix_insurance", "postgres", "REVOKE pg_write_all_data FROM justix_insurance_runtime;")
	t.Run("excess-direct-and-reachable-role-grants", func(t *testing.T) {
		// NOINHERIT membership is still reachable through SET ROLE. Every
		// check must reject before both feature construction and execution.
		f.must("justix_insurance", "postgres", "CREATE ROLE insurance_fixture_grants NOLOGIN; GRANT insurance_fixture_grants TO justix_insurance_runtime WITH INHERIT FALSE;")
		defer f.must("justix_insurance", "postgres", "REVOKE insurance_fixture_grants FROM justix_insurance_runtime;")
		cases := []struct{ name, privilege, object string }{
			{"database_create", "CREATE", "DATABASE justix_insurance"},
			{"database_temporary", "TEMPORARY", "DATABASE justix_insurance"},
			{"events_maintain", "MAINTAIN", "eventstore.events"},
			{"events_references", "REFERENCES", "eventstore.events"},
			{"events_column_references", "REFERENCES(event_id)", "eventstore.events"},
			{"marker_maintain", "MAINTAIN", "insurance_mechanics.compatibility"},
			{"ledger_maintain", "MAINTAIN", "public.schema_migrations"},
			{"marker_references", "REFERENCES", "insurance_mechanics.compatibility"},
			{"ledger_references", "REFERENCES", "public.schema_migrations"},
			{"marker_column_insert", "INSERT(singleton,owner_service,mechanics_version)", "insurance_mechanics.compatibility"},
			{"ledger_column_insert", "INSERT(version,dirty)", "public.schema_migrations"},
			{"marker_column_references", "REFERENCES(singleton)", "insurance_mechanics.compatibility"},
			{"ledger_column_references", "REFERENCES(version)", "public.schema_migrations"},
		}
		for _, role := range []string{"justix_insurance_runtime", "insurance_fixture_grants"} {
			for _, tc := range cases {
				t.Run(role+"/"+tc.name, func(t *testing.T) {
					f.must("justix_insurance", "justix_insurance", "GRANT "+tc.privilege+" ON "+tc.object+" TO "+role)
					defer f.must("justix_insurance", "justix_insurance", "REVOKE "+tc.privilege+" ON "+tc.object+" FROM "+role)
					binds := 0
					r, err := insurance.NewUnitOfWork(db, func(tx *gorm.DB) (fixturePorts, error) { binds++; return bindFixture(tx) })
					if err != nil {
						t.Fatal(err)
					}
					if !errors.Is(r.Check(ctx), insurance.ErrIncompatiblePersistence) {
						t.Fatal("excess grants accepted by Check")
					}
					called := false
					err = r.Run(ctx, func(fixturePorts) error { called = true; return nil })
					if !errors.Is(err, insurance.ErrIncompatiblePersistence) || binds != 0 || called {
						t.Fatalf("excess grants reached feature: %v binds=%d called=%v", err, binds, called)
					}
				})
			}
		}
		if err := runner.Check(ctx); err != nil {
			t.Fatalf("narrow privileges no longer accepted: %v", err)
		}
	})
	t.Run("required-grants-and-transitive-reachability", func(t *testing.T) {
		binds := 0
		r, err := insurance.NewUnitOfWork(db, func(tx *gorm.DB) (fixturePorts, error) { binds++; return bindFixture(tx) })
		if err != nil {
			t.Fatal(err)
		}
		rejectBeforeBind := func() {
			t.Helper()
			binds = 0
			called := false
			if !errors.Is(r.Check(ctx), insurance.ErrIncompatiblePersistence) {
				t.Fatal("incompatible privileges passed Check")
			}
			if err := r.Run(ctx, func(fixturePorts) error { called = true; return nil }); !errors.Is(err, insurance.ErrIncompatiblePersistence) || called || binds != 0 {
				t.Fatalf("incompatible privileges reached feature: %v binds=%d called=%v", err, binds, called)
			}
		}
		for _, grant := range []struct{ privilege, object string }{
			{"INSERT", "eventstore.events"},
			{"SELECT", "eventstore.inbox"},
			{"UPDATE(lease_until)", "eventstore.outbox"},
			{"UPDATE(revision)", "eventstore.operations"},
			{"USAGE", "SCHEMA eventstore"},
			{"SELECT", "insurance_mechanics.compatibility"},
			{"SELECT", "public.schema_migrations"},
		} {
			f.must("justix_insurance", "justix_insurance", "REVOKE "+grant.privilege+" ON "+grant.object+" FROM justix_insurance_runtime")
			rejectBeforeBind()
			f.must("justix_insurance", "justix_insurance", "GRANT "+grant.privilege+" ON "+grant.object+" TO justix_insurance_runtime")
			if err := r.Check(ctx); err != nil {
				t.Fatalf("restored required privilege rejected: %v", err)
			}
		}
		f.must("justix_insurance", "postgres", "CREATE ROLE insurance_fixture_bridge NOLOGIN; CREATE ROLE insurance_fixture_deep NOLOGIN; GRANT insurance_fixture_bridge TO justix_insurance_runtime WITH INHERIT FALSE; GRANT insurance_fixture_deep TO insurance_fixture_bridge WITH INHERIT FALSE")
		f.must("justix_insurance", "justix_insurance", "GRANT UPDATE(data) ON eventstore.events TO insurance_fixture_deep")
		rejectBeforeBind()
		f.must("justix_insurance", "justix_insurance", "REVOKE UPDATE(data) ON eventstore.events FROM insurance_fixture_deep")
		f.must("justix_insurance", "justix_insurance", "GRANT MAINTAIN ON eventstore.events TO PUBLIC")
		rejectBeforeBind()
		f.must("justix_insurance", "justix_insurance", "REVOKE MAINTAIN ON eventstore.events FROM PUBLIC")
		f.must("justix_insurance", "justix_insurance", "INSERT INTO public.schema_migrations VALUES(2,false)")
		rejectBeforeBind()
		f.must("justix_insurance", "justix_insurance", "DELETE FROM public.schema_migrations WHERE version=2")
		if err := r.Check(ctx); err != nil {
			t.Fatalf("restored installation rejected: %v", err)
		}
	})
	ownerRunner, _ := insurance.NewUnitOfWork(f.open("justix_insurance"), bindFixture)
	if ownerRunner.Check(ctx) == nil {
		t.Fatal("migration owner accepted as runtime")
	}
	t.Run("privileged-session-cannot-masquerade-as-runtime", func(t *testing.T) {
		masquerade := f.open("postgres")
		pool, err := masquerade.DB()
		if err != nil {
			t.Fatal(err)
		}
		pool.SetMaxOpenConns(1)
		pool.SetMaxIdleConns(1)
		if err := masquerade.Exec("SET ROLE justix_insurance_runtime").Error; err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := masquerade.Exec("RESET ROLE").Error; err != nil {
				t.Error(err)
			}
		}()
		binds, calls := 0, 0
		r, err := insurance.NewUnitOfWork(masquerade, func(tx *gorm.DB) (fixturePorts, error) {
			binds++
			return bindFixture(tx)
		})
		if err != nil {
			t.Fatal(err)
		}
		if !errors.Is(r.Check(ctx), insurance.ErrIncompatiblePersistence) {
			t.Fatal("privileged login passed readiness after SET ROLE")
		}
		if err := r.Run(ctx, func(fixturePorts) error { calls++; return nil }); !errors.Is(err, insurance.ErrIncompatiblePersistence) || binds != 0 || calls != 0 {
			t.Fatalf("privileged session reached business boundary: err=%v binds=%d calls=%d", err, binds, calls)
		}
	})
	for _, owner := range []string{"identity", "inventory", "commerce", "retail", "financing", "documents"} {
		if _, err := f.sql("justix_"+owner, "justix_insurance_runtime", "SELECT 1"); err == nil {
			t.Fatalf("runtime connected to %s", owner)
		}
	}
	for _, statement := range []string{"UPDATE insurance_mechanics.compatibility SET mechanics_version=1", "UPDATE public.schema_migrations SET dirty=false", "DELETE FROM eventstore.events", "CREATE TABLE eventstore.foreign_table(id int)"} {
		if _, err := f.sql("justix_insurance", "justix_insurance_runtime", statement); err == nil {
			t.Fatalf("runtime privilege accepted: %s", statement)
		}
	}

	t.Run("factory-failure-rolls-back-before-callback", func(t *testing.T) {
		abort := errors.New("synthetic binding failure")
		r, err := insurance.NewUnitOfWork(db, func(tx *gorm.DB) (fixturePorts, error) {
			u, err := bindFixture(tx)
			if err != nil {
				return nil, err
			}
			if _, err := u.Record(ctx, commands.Scope{ActorID: uuid.NewString(), CompanyID: uuid.NewString(), Key: uuid.NewString()}, fixtureValue{ReferenceID: uuid.NewString()}); err != nil {
				return nil, err
			}
			return nil, abort
		})
		if err != nil {
			t.Fatal(err)
		}
		called := false
		if err := r.Run(ctx, func(fixturePorts) error { called = true; return nil }); !errors.Is(err, abort) || called {
			t.Fatalf("failed binding reached callback: %v called=%v", err, called)
		}
		for _, table := range []string{"events", "command_receipts", "operations"} {
			if n := f.must("justix_insurance", "justix_insurance_runtime", "SELECT count(*) FROM eventstore."+table); n != "0" {
				t.Fatalf("factory leaked %s=%s", table, n)
			}
		}
	})
	t.Run("atomic-rollback-replay-and-idempotency", func(t *testing.T) {
		scope := commands.Scope{ActorID: uuid.NewString(), CompanyID: uuid.NewString(), Key: uuid.NewString()}
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
			if n := f.must("justix_insurance", "justix_insurance_runtime", "SELECT count(*) FROM eventstore."+table); n != "0" {
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
			if n := f.must("justix_insurance", "justix_insurance_runtime", "SELECT count(*) FROM eventstore."+table); n != "1" {
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
		scope := commands.Scope{ActorID: uuid.NewString(), CompanyID: uuid.NewString(), Key: uuid.NewString()}
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
// these are test-only references, not invented Insurance business schemas.
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
func bindFixture(tx *gorm.DB) (fixturePorts, error) {
	a := &fixtureAdapter{}
	var err error
	a.append, err = eventstore.NewAppender(tx, events.OwnerInsurance)
	if err != nil {
		return nil, err
	}
	a.load, err = eventstore.NewLoader(tx, events.OwnerInsurance)
	if err != nil {
		return nil, err
	}
	a.schema, err = events.NewEventSchema("insurance.fixture.recorded.v1", 1, "fixture", events.CompanyScopeTenantRequired, validate)
	if err != nil {
		return nil, err
	}
	schema, err := commands.NewSchema(events.OwnerInsurance, "insurance.fixture.record", false, func(v fixtureValue) (fixtureValue, error) { return v, validate(v) }, validate)
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
		event, err := eventstore.NewInternalEvent(events.EnvelopeInput{EventID: uuid.NewString(), AggregateID: input.ReferenceID, AggregateVersion: revision, CompanyID: &scope.CompanyID, OccurredAt: time.Now().UTC(), Actor: events.Actor{Kind: events.ActorUser, ID: scope.ActorID}, CorrelationID: uuid.NewString(), CausationID: uuid.NewString(), OperationID: operationID}, a.schema, input)
		if err != nil {
			return commands.Receipt[fixtureValue]{}, err
		}
		if err := a.append.Append(ctx, eventstore.Batch{Events: []eventstore.Event{event}}); err != nil {
			return commands.Receipt[fixtureValue]{}, err
		}
		if err := a.operations.Create(ctx, commands.OperationIntent{ID: operationID, AggregateID: input.ReferenceID, ActorID: scope.ActorID, CompanyID: scope.CompanyID, AdmissionRef: uuid.NewString(), Phase: "pending"}, input); err != nil {
			return commands.Receipt[fixtureValue]{}, err
		}
		return commands.NewReceipt(201, input, revision, operationID)
	})
	return receipt, err
}
func (a *fixtureAdapter) Load(ctx context.Context, id string) (fixtureValue, error) {
	stream := eventstore.Stream{Owner: events.OwnerInsurance, AggregateType: "fixture", AggregateID: id}
	history, err := a.load.Load(ctx, stream)
	if err != nil {
		return fixtureValue{}, err
	}
	result, err := eventstore.Replay(ctx, stream, history, []eventstore.Decoder[fixtureValue]{eventstore.DecodeWith(a.schema)}, func() fixtureValue { return fixtureValue{} }, func(_ fixtureValue, _ eventstore.ReplayMetadata, v fixtureValue) (fixtureValue, error) { return v, nil })
	return result.State, err
}

func TestInsuranceScopeRecoveryAndMessaging(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_INSURANCE_MECHANICS") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_INSURANCE_MECHANICS=1 for disposable PostgreSQL")
	}
	f := start(t)
	f.must("justix_insurance", "postgres", "CREATE ROLE justix_insurance_runtime LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_insurance TO justix_insurance_runtime;")
	if out, err := f.sql("justix_insurance", "justix_insurance", f.file("pkg/eventstore/schema.sql"), "-v", "owner_service=insurance", "-v", "runtime_role=justix_insurance_runtime"); err != nil {
		t.Fatalf("shared install: %v %s", err, out)
	}
	f.must("justix_insurance", "justix_insurance", "CREATE TABLE public.schema_migrations(version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL); INSERT INTO public.schema_migrations VALUES(1,true);")
	f.must("justix_insurance", "justix_insurance", f.file("services/insurance/migrations/0001_mechanics.up.sql"))
	f.must("justix_insurance", "justix_insurance", "UPDATE public.schema_migrations SET dirty=false")
	db := f.open("justix_insurance_runtime")
	ctx := context.Background()
	runner, err := insurance.NewUnitOfWork(db, bindFixture)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := commands.NewSchema(events.OwnerInsurance, "insurance.fixture.record", false, func(v fixtureValue) (fixtureValue, error) { return v, validate(v) }, validate)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("company-required-and-receipt-scope", func(t *testing.T) {
		input := fixtureValue{ReferenceID: uuid.NewString()}
		scope := commands.Scope{ActorID: uuid.NewString(), CompanyID: uuid.NewString(), Key: uuid.NewString()}
		for _, badCompany := range []string{"", "not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
			bad := scope
			bad.CompanyID = badCompany
			if err := runner.Run(ctx, func(u fixturePorts) error { _, err := u.Record(ctx, bad, input); return err }); !errors.Is(err, commands.ErrInvalidCommand) {
				t.Fatalf("invalid company admitted: %q %v", badCompany, err)
			}
		}
		if err := runner.Run(ctx, func(u fixturePorts) error { _, err := u.Record(ctx, scope, input); return err }); err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"events", "command_receipts", "operations"} {
			if n := f.must("justix_insurance", "justix_insurance_runtime", "SELECT count(*) FROM eventstore."+table+" WHERE company_id='"+scope.CompanyID+"'"); n != "1" {
				t.Fatalf("company not retained in %s: %s", table, n)
			}
		}
		for _, change := range []string{"actor", "company", "target", "key"} {
			foreign := scope
			switch change {
			case "actor":
				foreign.ActorID = uuid.NewString()
			case "company":
				foreign.CompanyID = uuid.NewString()
			case "target":
				foreign.TargetID = uuid.NewString()
			case "key":
				foreign.Key = uuid.NewString()
			}
			if _, found, err := commands.Recover(ctx, db, schema, foreign); err != nil || found {
				t.Fatalf("receipt escaped %s scope: found=%v err=%v", change, found, err)
			}
		}
		if _, found, err := commands.Recover(ctx, db, schema, scope); err != nil || !found {
			t.Fatalf("own receipt missing: %v %v", found, err)
		}
		// Distinct admitted companies have distinct command keys. This tests
		// ledger scoping, not permission to act for either synthetic company.
		other := scope
		other.CompanyID = uuid.NewString()
		if err := runner.Run(ctx, func(u fixturePorts) error {
			_, err := u.Record(ctx, other, fixtureValue{ReferenceID: uuid.NewString()})
			return err
		}); err != nil {
			t.Fatalf("other company collided with scoped key: %v", err)
		}
		eventSchema, err := events.NewEventSchema("insurance.fixture.recorded.v1", 1, "fixture", events.CompanyScopeTenantRequired, validate)
		if err != nil {
			t.Fatal(err)
		}
		revision, _ := events.NewRevision(1)
		if _, err := eventstore.NewInternalEvent(events.EnvelopeInput{EventID: uuid.NewString(), AggregateID: uuid.NewString(), AggregateVersion: revision, OccurredAt: time.Now().UTC(), Actor: events.Actor{Kind: events.ActorUser, ID: scope.ActorID}, CorrelationID: uuid.NewString(), CausationID: uuid.NewString()}, eventSchema, input); err == nil {
			t.Fatal("insurance event without company accepted")
		}
	})
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprintf("lost-reply-actual-commit-%v", committed), func(t *testing.T) {
			pool, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			fault := db.Session(&gorm.Session{NewDB: true})
			fault.Statement = &gorm.Statement{DB: fault, ConnPool: faultPool{DB: pool, committed: committed}}
			faultRunner, err := insurance.NewUnitOfWork(fault, bindFixture)
			if err != nil {
				t.Fatal(err)
			}
			input := fixtureValue{ReferenceID: uuid.NewString()}
			scope := commands.Scope{ActorID: uuid.NewString(), CompanyID: uuid.NewString(), Key: uuid.NewString()}
			calls := 0
			err = faultRunner.Run(ctx, func(u fixturePorts) error { calls++; _, err := u.Record(ctx, scope, input); return err })
			if !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) || !errors.Is(err, io.ErrUnexpectedEOF) || calls != 1 {
				t.Fatalf("lost reply classification: %v calls=%d", err, calls)
			}
			if _, found, err := commands.Recover(ctx, db, schema, scope); err != nil || found != committed {
				t.Fatalf("authoritative recovery: found=%v committed=%v err=%v", found, committed, err)
			}
			if err := runner.Run(ctx, func(u fixturePorts) error { _, err := u.Record(ctx, scope, input); return err }); err != nil {
				t.Fatal(err)
			}
			for _, table := range []string{"events", "operations"} {
				if n := f.must("justix_insurance", "justix_insurance_runtime", "SELECT count(*) FROM eventstore."+table+" WHERE aggregate_id='"+input.ReferenceID+"'"); n != "1" {
					t.Fatalf("%s retry effects=%s", table, n)
				}
			}
		})
	}
	t.Run("additive-legacy-ready-custody-rejected", func(t *testing.T) {
		if out, err := f.sql("justix_insurance", "justix_insurance", f.file("pkg/eventstore/migrations/000002_messaging_delivery.up.sql"), "-v", "owner_service=insurance", "-v", "runtime_role=justix_insurance_runtime"); err != nil {
			t.Fatalf("additive messaging install: %v %s", err, out)
		}
		if err := runner.Check(ctx); err != nil {
			t.Fatalf("additive legacy not ready: %v", err)
		}
		if err := runner.Run(ctx, func(fixturePorts) error { return nil }); err != nil {
			t.Fatal(err)
		}
		for _, change := range []struct{ name, set, reset string }{
			{"metadata column write", "GRANT UPDATE(mode) ON eventstore.messaging_mode TO justix_insurance_runtime", "REVOKE UPDATE(mode) ON eventstore.messaging_mode FROM justix_insurance_runtime"},
			{"PUBLIC metadata write", "GRANT INSERT(mode) ON eventstore.messaging_mode TO PUBLIC", "REVOKE INSERT(mode) ON eventstore.messaging_mode FROM PUBLIC"},
			{"RLS metadata", "ALTER TABLE eventstore.messaging_mode ENABLE ROW LEVEL SECURITY", "ALTER TABLE eventstore.messaging_mode DISABLE ROW LEVEL SECURITY"},
			{"missing installed marker", "ALTER TABLE eventstore.messaging_mode RENAME TO hidden_mode", "ALTER TABLE eventstore.hidden_mode RENAME TO messaging_mode"},
			{"view marker substitution", "ALTER TABLE eventstore.messaging_mode RENAME TO hidden_mode; CREATE VIEW eventstore.messaging_mode AS SELECT * FROM eventstore.hidden_mode; GRANT SELECT ON eventstore.messaging_mode TO justix_insurance_runtime", "DROP VIEW eventstore.messaging_mode; ALTER TABLE eventstore.hidden_mode RENAME TO messaging_mode"},
		} {
			t.Run(change.name, func(t *testing.T) {
				local := f
				local.t = t
				local.must("justix_insurance", "justix_insurance", change.set)
				binds, calls := 0, 0
				checked, err := insurance.NewUnitOfWork(db, func(tx *gorm.DB) (fixturePorts, error) { binds++; return bindFixture(tx) })
				if err != nil {
					t.Fatal(err)
				}
				if !errors.Is(checked.Check(ctx), insurance.ErrIncompatiblePersistence) {
					t.Fatal("invalid mode metadata ready")
				}
				if err := checked.Run(ctx, func(fixturePorts) error { calls++; return nil }); !errors.Is(err, insurance.ErrIncompatiblePersistence) || binds != 0 || calls != 0 {
					t.Fatalf("invalid mode metadata reached feature: %v binds=%d calls=%d", err, binds, calls)
				}
				local.must("justix_insurance", "justix_insurance", change.reset)
				if err := runner.Check(ctx); err != nil {
					t.Fatalf("restored legacy metadata: %v", err)
				}
			})
		}
		// No legacy outbox entries and no runtimes or broker were started.
		f.must("justix_insurance", "justix_insurance", "SELECT eventstore.activate_messaging_custody('"+uuid.NewString()+"',decode(repeat('ab',32),'hex'),decode(repeat('cd',32),'hex'),'synthetic-fixture-backup','no-runtimes-started','no-broker-fixture','legacy-scaffold-incompatible')")
		if mode := f.must("justix_insurance", "justix_insurance_runtime", "SELECT mode FROM eventstore.messaging_mode"); mode != "custody" {
			t.Fatalf("mode=%s", mode)
		}
		binds := 0
		r, err := insurance.NewUnitOfWork(db, func(tx *gorm.DB) (fixturePorts, error) { binds++; return bindFixture(tx) })
		if err != nil {
			t.Fatal(err)
		}
		if !errors.Is(r.Check(ctx), insurance.ErrIncompatiblePersistence) {
			t.Fatal("custody installation accepted")
		}
		called := false
		if err := r.Run(ctx, func(fixturePorts) error { called = true; return nil }); !errors.Is(err, insurance.ErrIncompatiblePersistence) || called || binds != 0 {
			t.Fatalf("custody reached legacy feature: %v called=%v binds=%d", err, called, binds)
		}
		// Configuration drift must not turn custody into a supported legacy mode.
		f.must("justix_insurance", "justix_insurance", "GRANT INSERT ON eventstore.outbox TO justix_insurance_runtime; GRANT UPDATE(attempts,next_attempt_at,lease_owner,lease_until,sent_at) ON eventstore.outbox TO justix_insurance_runtime")
		if mode := f.must("justix_insurance", "justix_insurance_runtime", "SELECT mode FROM eventstore.messaging_mode"); mode != "custody" {
			t.Fatalf("mode changed: %s", mode)
		}
		if !errors.Is(r.Check(ctx), insurance.ErrIncompatiblePersistence) {
			t.Fatal("custody with restored legacy grants ready")
		}
		if err := r.Run(ctx, func(fixturePorts) error { called = true; return nil }); !errors.Is(err, insurance.ErrIncompatiblePersistence) || called || binds != 0 {
			t.Fatalf("restored grants reached legacy feature in custody: %v called=%v binds=%d", err, called, binds)
		}
	})
}

// Inject a lost reply around a real PostgreSQL commit/rollback, preserving all
// other operations and letting authoritative receipt recovery resolve it.
type faultPool struct {
	*sql.DB
	committed bool
}

func (p faultPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &faultTx{Tx: tx, committed: p.committed}, nil
}

type faultTx struct {
	*sql.Tx
	committed bool
}

func (t *faultTx) Commit() error {
	var err error
	if t.committed {
		err = t.Tx.Commit()
	} else {
		err = t.Tx.Rollback()
	}
	if err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}

func TestFixtureRefusesUnownedIdentityOrMounts(t *testing.T) {
	name, token := "justixauto-t029-"+uuid.NewString(), uuid.NewString()
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
	if os.Getenv("JUSTIXAUTO_TEST_INSURANCE_MECHANICS") != "1" {
		t.Skip("owned PostgreSQL opt-in required")
	}
	if os.Getenv("JUSTIXAUTO_T029_FIXTURE_CHILD") != "" {
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
				wrapper += " shift\n status=0\n " + quote(docker) + " create --t029-invalid-option \"$@\" || status=$?\n if [ \"$status\" = 0 ]; then exit 1; fi\n echo \"T029 actual allocation rejected: $status\" >&2\n"
			}
			wrapper += " echo 'T029 synthetic " + mode + " failure' >&2\n exit 42\nfi\nexec " + quote(docker) + " \"$@\"\n"
			if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(wrapper), 0700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFixtureAllocationFailures$", "-test.v")
			child.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "JUSTIXAUTO_T029_FIXTURE_CHILD="+mode)
			out, err := child.CombinedOutput()
			t.Logf("expected child failure: %v\n%s", err, out)
			if err == nil || !strings.Contains(string(out), "T029 synthetic "+mode+" failure") {
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
				if !strings.Contains(string(out), "unknown flag") || !strings.Contains(string(out), "T029 actual allocation rejected:") || !strings.Contains(string(out), "fixture absent after failed allocation: "+name) {
					t.Fatal("actual pre-allocation rejection and absence not proven")
				}
				if _, err := os.Stat(idfile); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("unexpected allocation receipt")
				}
			}
		})
	}
}
