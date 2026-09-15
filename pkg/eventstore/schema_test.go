package eventstore_test

import (
	"context"
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

const postgresImage = "docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"

var owners = []string{"identity", "inventory", "commerce", "retail", "financing", "insurance", "documents"}

type schemaFixture struct {
	t         *testing.T
	container string
	schema    []byte
	password  string
}

// This test owns a fresh, randomly named container with no host ports or named
// volume. It never accepts a DSN or connects to reference/existing databases.
func TestOwnerSchemaPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_EVENTSTORE_SCHEMA") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_EVENTSTORE_SCHEMA=1 for isolated Docker/PostgreSQL tests")
	}
	f := startFixture(t)
	for _, owner := range owners {
		t.Run(owner, func(t *testing.T) {
			f := f
			f.t = t
			f.mustSQL(owner, "postgres", "CREATE ROLE justix_"+owner+"_runtime LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_"+owner+" TO justix_"+owner+"_runtime;")
			// A database owner can bypass append-only ACLs, so installation must
			// reject that role before creating any schema objects.
			if _, err := f.install(owner, "justix_"+owner); err == nil {
				t.Fatal("accepted database owner as runtime")
			}
			if got := f.mustSQL(owner, "postgres", "SELECT count(*) FROM pg_namespace WHERE nspname='eventstore'"); got != "0" {
				t.Fatalf("failed install left schema: %s", got)
			}
			if owner == "identity" {
				for _, privileged := range []string{"pg_write_all_data", "pg_read_all_data", "pg_execute_server_program"} {
					f.mustSQL(owner, "postgres", "GRANT "+privileged+" TO justix_identity_runtime WITH INHERIT FALSE;")
					if out, err := f.install(owner, "justix_identity_runtime"); err == nil || !strings.Contains(out, "privileged role membership") {
						t.Fatalf("accepted privileged SET ROLE membership %s: %v %s", privileged, err, out)
					}
					f.mustSQL(owner, "postgres", "REVOKE "+privileged+" FROM justix_identity_runtime;")
				}
				f.mustSQL(owner, "justix_identity", "ALTER DEFAULT PRIVILEGES GRANT UPDATE ON TABLES TO justix_identity_runtime;")
				if out, err := f.install(owner, "justix_identity_runtime"); err == nil || !strings.Contains(out, "unexpected table privileges") {
					t.Fatalf("accepted broad default privileges: %v %s", err, out)
				}
				f.mustSQL(owner, "justix_identity", "ALTER DEFAULT PRIVILEGES REVOKE UPDATE ON TABLES FROM justix_identity_runtime;")
			}
			if out, err := f.install(owner, "justix_"+owner+"_runtime"); err != nil {
				t.Fatalf("install: %v\n%s", err, out)
			}
			if _, err := f.install(owner, "justix_"+owner+"_runtime"); err == nil {
				t.Fatal("silently reinstalled existing schema")
			}
			f.checkOwner(owner)
		})
	}
	t.Run("invalid-owner", func(t *testing.T) {
		if _, err := f.sql("identity", "justix_identity", string(f.schema), "-v", "owner_service=foreign", "-v", "runtime_role=justix_identity_runtime"); err == nil {
			t.Fatal("accepted unknown owner")
		}
	})
}

func startFixture(t *testing.T) schemaFixture {
	t.Helper()
	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := filepath.Abs("../../infra/local/postgres/init-owners.sql")
	if err != nil {
		t.Fatal(err)
	}
	f := schemaFixture{t: t, container: "justixauto-t008-" + uuid.NewString(), schema: schema, password: "synthetic-" + uuid.NewString()}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "docker", "rm", "-f", f.container).CombinedOutput(); err != nil {
			t.Errorf("remove owned fixture: %v: %s", err, out)
		}
	})
	args := []string{"run", "-d", "--name", f.container, "-e", "POSTGRES_PASSWORD=" + f.password, "-v", bootstrap + ":/docker-entrypoint-initdb.d/10-owners.sql:ro"}
	for _, owner := range owners {
		args = append(args, "-e", "JUSTIXAUTO_"+strings.ToUpper(owner)+"_DB_PASSWORD="+f.password)
	}
	args = append(args, postgresImage)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
		t.Fatalf("start owned fixture: %v: %s", err, out)
	}
	for {
		// TCP becomes available only after the temporary init server exits.
		if _, err := f.sql("documents", "justix_documents", "SELECT 1"); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("fixture did not finish bootstrap")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if got := f.mustSQL("identity", "postgres", "SHOW server_version_num"); got != "180006" {
		t.Fatalf("unexpected PostgreSQL version: %s", got)
	}
	return f
}

func (f schemaFixture) sql(owner, role, statement string, options ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	args := []string{"exec", "-i", "-e", "PGPASSWORD=" + f.password, f.container, "psql", "-X", "-qAt", "-h", "127.0.0.1", "-U", role, "-d", "justix_" + owner, "-v", "ON_ERROR_STOP=1"}
	args = append(args, options...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = strings.NewReader(statement)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (f schemaFixture) mustSQL(owner, role, statement string) string {
	f.t.Helper()
	out, err := f.sql(owner, role, statement)
	if err != nil {
		f.t.Fatalf("SQL failed: %v\n%s\n%s", err, statement, out)
	}
	return out
}

func (f schemaFixture) denied(owner, statement, expected string) {
	f.t.Helper()
	out, err := f.sql(owner, "justix_"+owner+"_runtime", statement)
	if err == nil || !strings.Contains(out, expected) {
		f.t.Fatalf("expected %q rejection; err=%v output=%s SQL=%s", expected, err, out, statement)
	}
}

func (f schemaFixture) install(owner, runtime string) (string, error) {
	return f.sql(owner, "justix_"+owner, string(f.schema), "-v", "owner_service="+owner, "-v", "runtime_role="+runtime)
}

func eventSQL(owner, eventID, aggregateID, company string, revision int) string {
	return fmt.Sprintf(`INSERT INTO eventstore.events
 (event_id,event_type,schema_version,aggregate_type,aggregate_id,aggregate_version,company_id,occurred_at,actor_kind,actor_id,correlation_id,causation_id,operation_id,data)
 VALUES ('%s','%s.fixture.recorded.v1',1,'fixture','%s',%d,%s,'2026-09-15T00:00:00Z','user','%s','%s','%s','%s','{"referenceId":"%s"}');`,
		eventID, owner, aggregateID, revision, company, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString())
}

func (f schemaFixture) checkOwner(owner string) {
	t := f.t
	runtime := "justix_" + owner + "_runtime"
	company := "'" + uuid.NewString() + "'"
	eventID, aggregateID := uuid.NewString(), uuid.NewString()
	first := eventSQL(owner, eventID, aggregateID, company, 1)
	f.mustSQL(owner, runtime, first)
	f.denied(owner, first, "duplicate key")
	f.denied(owner, eventSQL(owner, uuid.NewString(), aggregateID, company, 1), "duplicate key")
	f.denied(owner, eventSQL(owner, uuid.NewString(), aggregateID, company, 0), "check constraint")
	f.denied(owner, strings.Replace(eventSQL(owner, uuid.NewString(), uuid.NewString(), company, 1), "'user'", "'branch'", 1), "check constraint")
	f.denied(owner, strings.Replace(eventSQL(owner, uuid.NewString(), uuid.NewString(), company, 1), ".recorded.v1", ".recorded.v2", 1), "check constraint")
	f.denied(owner, strings.Replace(eventSQL(owner, uuid.NewString(), uuid.NewString(), company, 1), "'"+owner+".fixture", "'foreign.fixture", 1), "check constraint")
	global := eventSQL(owner, uuid.NewString(), uuid.NewString(), "NULL", 1)
	if owner == "identity" {
		f.mustSQL(owner, runtime, global)
	} else {
		f.denied(owner, global, "check constraint")
	}
	for _, statement := range []string{
		"UPDATE eventstore.events SET data='{}'", "DELETE FROM eventstore.events", "TRUNCATE eventstore.events",
		"CREATE TABLE eventstore.injected(id integer)",
	} {
		f.denied(owner, statement, "permission denied")
	}
	for _, statement := range []string{"ALTER TABLE eventstore.events DISABLE TRIGGER ALL", "DROP TABLE eventstore.events"} {
		f.denied(owner, statement, "must be owner")
	}
	// Runtime cannot connect to any other owner's database.
	for _, foreign := range owners {
		if foreign == owner {
			continue
		}
		if out, err := f.sql(foreign, runtime, "SELECT * FROM eventstore.events"); err == nil || !strings.Contains(out, "permission denied for database") {
			t.Fatalf("cross-owner connection %s -> %s: %v %s", owner, foreign, err, out)
		}
	}
	// Two independent connections compete for precisely the same stream version.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.sql(owner, runtime, eventSQL(owner, uuid.NewString(), aggregateID, company, 2))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("competing writers succeeded %d times", success)
	}
	if got := f.mustSQL(owner, runtime, "SELECT string_agg(aggregate_version::text,',' ORDER BY aggregate_version) FROM eventstore.events WHERE aggregate_id='"+aggregateID+"'"); got != "1,2" {
		t.Fatalf("ordered history: %s", got)
	}
	f.checkAtomicRecords(owner, company)
}

func (f schemaFixture) checkAtomicRecords(owner, company string) {
	t := f.t
	runtime := "justix_" + owner + "_runtime"
	eventID, aggregateID, operationID, actorID, key := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	event := eventSQL(owner, eventID, aggregateID, company, 1)
	operation := fmt.Sprintf(`INSERT INTO eventstore.operations(operation_id,kind,aggregate_id,actor_id,company_id,admission_ref,intent_revision,request_hash,phase,revision) VALUES('%s','fixture','%s','%s',%s,'%s',0,sha256('safe'::bytea),'prepare',1);`, operationID, aggregateID, actorID, company, uuid.NewString())
	outbox := fmt.Sprintf(`INSERT INTO eventstore.outbox(event_id,aggregate_type,aggregate_id,integration_sequence,envelope,envelope_hash,exchange,routing_key) VALUES('%s','fixture','%s',1,'safe'::bytea,sha256('safe'::bytea),'justix.events','%s.fixture');`, eventID, aggregateID, owner)
	receipt := fmt.Sprintf(`INSERT INTO eventstore.command_receipts(receipt_id,actor_id,company_id,command_name,target_id,idempotency_key,request_hash,http_status,receipt) VALUES('%s','%s',%s,'fixture',NULL,'%s',sha256('safe'::bytea),202,'{"status":"pending"}');`, uuid.NewString(), actorID, company, key)
	inbox := fmt.Sprintf(`INSERT INTO eventstore.inbox(consumer_name,event_id,envelope_hash) VALUES('fixture','%s',sha256('safe'::bytea));`, eventID)
	step := fmt.Sprintf(`INSERT INTO eventstore.operation_steps(operation_id,step,request_hash,result) VALUES('%s','prepare',sha256('safe'::bytea),'{}');`, operationID)
	all := event + operation + outbox + receipt + inbox + step
	f.mustSQL(owner, runtime, "BEGIN;"+all+"ROLLBACK;")
	for _, table := range []string{"outbox", "command_receipts", "inbox", "operations", "operation_steps"} {
		if got := f.mustSQL(owner, runtime, "SELECT count(*) FROM eventstore."+table); got != "0" {
			t.Fatalf("rollback leaked %s: %s", table, got)
		}
	}
	if got := f.mustSQL(owner, runtime, "SELECT count(*) FROM eventstore.events WHERE event_id='"+eventID+"'"); got != "0" {
		t.Fatal("rollback leaked event")
	}
	f.mustSQL(owner, runtime, "BEGIN;"+all+"COMMIT;")
	// Replay evidence survives disconnect; same keys (even with changed hash)
	// cannot add another receipt/step/effect. T-017 supplies receipt lookup/409.
	f.denied(owner, newReceiptID(receipt), "duplicate key")
	f.denied(owner, newReceiptID(strings.Replace(receipt, "sha256('safe'::bytea)", "sha256('changed'::bytea)", 1)), "duplicate key")
	f.denied(owner, inbox, "duplicate key")
	f.denied(owner, step, "duplicate key")
	f.denied(owner, outbox, "duplicate key")
	f.denied(owner, strings.Replace(outbox, eventID, uuid.NewString(), 1), "duplicate key")
	invalidHash := strings.Replace(outbox, eventID, uuid.NewString(), 1)
	invalidHash = strings.Replace(invalidHash, aggregateID, uuid.NewString(), 1)
	f.denied(owner, strings.Replace(invalidHash, "sha256('safe'::bytea)", "sha256('wrong'::bytea)", 1), "check constraint")
	for _, table := range []string{"command_receipts", "inbox", "operation_steps"} {
		f.denied(owner, "DELETE FROM eventstore."+table, "permission denied")
		f.denied(owner, "TRUNCATE eventstore."+table, "permission denied")
	}
	f.denied(owner, "UPDATE eventstore.outbox SET envelope='changed'::bytea", "permission denied")
	f.denied(owner, "UPDATE eventstore.operations SET request_hash=sha256('changed'::bytea)", "permission denied")
	f.denied(owner, "UPDATE eventstore.operations SET company_id='"+uuid.NewString()+"'", "permission denied")
	f.mustSQL(owner, runtime, "UPDATE eventstore.outbox SET attempts=1,lease_owner='"+uuid.NewString()+"',lease_until=clock_timestamp()+interval '1 minute';")
	f.mustSQL(owner, runtime, "UPDATE eventstore.operations SET decision='commit',revision=2 WHERE operation_id='"+operationID+"';")
	f.denied(owner, "UPDATE eventstore.operations SET decision='abort'", "operation decision is immutable")
	f.denied(owner, "UPDATE eventstore.operations SET decision='undecided'", "operation decision is immutable")
	// Expired leases do not delete work or establish a terminal business result.
	f.mustSQL(owner, runtime, "UPDATE eventstore.operations SET lease_owner='"+uuid.NewString()+"',lease_until=clock_timestamp()-interval '1 minute',attention_required=true;")
	if got := f.mustSQL(owner, runtime, "SELECT phase||':'||decision FROM eventstore.operations WHERE operation_id='"+operationID+"'"); got != "prepare:commit" {
		t.Fatalf("lease changed business state: %s", got)
	}
	if owner == "identity" {
		global := strings.Replace(receipt, company, "NULL", 1)
		// Change receipt identity, retain actor/command/target/key so NULL company
		// scope is independently inserted once and deduplicated thereafter.
		global = newReceiptID(global)
		f.mustSQL(owner, runtime, global)
		f.denied(owner, newReceiptID(global), "duplicate key")
	}
}

func newReceiptID(statement string) string {
	start := strings.Index(statement, "VALUES('") + len("VALUES('")
	return statement[:start] + uuid.NewString() + statement[start+36:]
}
