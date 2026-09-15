package postgres_test

import (
	"context"
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
	"justixauto/pkg/commands"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
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

func start(t *testing.T) fixture {
	t.Helper()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{t: t, name: "justixauto-t024-" + uuid.NewString(), password: "synthetic-" + uuid.NewString(), root: root}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	args := []string{"run", "-d", "--name", f.name, "-p", "127.0.0.1::5432", "-e", "POSTGRES_PASSWORD=" + f.password, "-v", filepath.Join(root, "infra/local/postgres/init-owners.sql") + ":/docker-entrypoint-initdb.d/10-owners.sql:ro"}
	for _, owner := range []string{"identity", "inventory", "commerce", "retail", "financing", "insurance", "documents"} {
		args = append(args, "-e", "JUSTIXAUTO_"+strings.ToUpper(owner)+"_DB_PASSWORD="+f.password)
	}
	args = append(args, image)
	if out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture start: %v %s", err, out)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "docker", "rm", "-f", f.name).CombinedOutput(); err != nil {
			t.Errorf("owned fixture cleanup: %v %s", err, out)
		}
	})
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
	out, err := exec.CommandContext(ctx, "docker", "port", f.name, "5432/tcp").Output()
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
