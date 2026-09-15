"""Independent T-008 SQL probes; run from the exact task worktree root."""
import concurrent.futures
import pathlib
import subprocess
import time
import uuid

ROOT = pathlib.Path.cwd()
EXPECTED = "19ee699a79d9d6da3d022c4a71c8b734c8e5ca1d"
assert subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip() == EXPECTED
IMAGE = "docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"
NAME = "justixauto-qa-t008-" + str(uuid.uuid4())
PASSWORD = "synthetic-" + str(uuid.uuid4())
OWNERS = "identity inventory commerce retail financing insurance documents".split()
SCHEMA = (ROOT / "pkg/eventstore/schema.sql").read_text()

def run(args, source=None):
    return subprocess.run(args, input=source, text=True, capture_output=True, timeout=45)

def sql(statement, role="justix_identity", check=True, options=()):
    result = run(["docker", "exec", "-i", "-e", "PGPASSWORD=" + PASSWORD, NAME,
                  "psql", "-XqAt", "-h", "127.0.0.1", "-U", role,
                  "-d", "justix_identity", "-v", "ON_ERROR_STOP=1", *options], statement)
    if check:
        assert result.returncode == 0, result.stderr
    return result

def install():
    return sql(SCHEMA, check=False, options=("-v", "owner_service=identity", "-v", "runtime_role=qa_runtime"))

def rejected_install(label, expected):
    result = install()
    assert result.returncode != 0 and expected in result.stderr, result.stderr
    assert sql("SELECT count(*) FROM pg_namespace WHERE nspname='eventstore'").stdout.strip() == "0"
    print("PASS", label, "and atomic rollback", flush=True)

def deny(statement, expected="permission denied"):
    result = sql(statement, role="qa_runtime", check=False)
    assert result.returncode != 0 and expected in result.stderr, (statement, result.stdout, result.stderr)

def fresh():
    return str(uuid.uuid4())

started = False
try:
    args = ["docker", "run", "-d", "--name", NAME, "-e", "POSTGRES_PASSWORD=" + PASSWORD,
            "-v", str(ROOT / "infra/local/postgres/init-owners.sql") + ":/docker-entrypoint-initdb.d/10-owners.sql:ro"]
    for owner in OWNERS:
        args += ["-e", "JUSTIXAUTO_" + owner.upper() + "_DB_PASSWORD=" + PASSWORD]
    result = run(args + [IMAGE])
    assert result.returncode == 0, result.stderr
    started = True
    for attempt in range(100):
        if sql("SELECT 1", check=False).returncode == 0:
            break
        time.sleep(0.1)
    else:
        raise AssertionError("fixture not ready")
    assert sql("SHOW server_version_num").stdout.strip() == "180006"
    sql("CREATE ROLE qa_runtime LOGIN PASSWORD '" + PASSWORD + "'; CREATE ROLE qa_group; GRANT CONNECT ON DATABASE justix_identity TO qa_runtime", "postgres")
    sql("GRANT justix_identity TO qa_group WITH INHERIT FALSE; GRANT qa_group TO qa_runtime WITH INHERIT FALSE", "postgres")
    rejected_install("transitive NOINHERIT owner access rejected", "separate from migration/database owner")
    sql("REVOKE justix_identity FROM qa_group", "postgres")
    sql("GRANT pg_write_all_data TO qa_group WITH INHERIT FALSE", "postgres")
    rejected_install("transitive NOINHERIT predefined role rejected", "privileged role membership")
    sql("REVOKE pg_write_all_data FROM qa_group", "postgres")
    sql("ALTER DEFAULT PRIVILEGES GRANT UPDATE ON TABLES TO qa_group")
    rejected_install("SET ROLE ordinary group default UPDATE rejected", "unexpected table privileges")
    sql("ALTER DEFAULT PRIVILEGES REVOKE UPDATE ON TABLES FROM qa_group; ALTER DEFAULT PRIVILEGES GRANT CREATE ON SCHEMAS TO qa_group")
    rejected_install("SET ROLE ordinary group default schema CREATE rejected", "create schema objects")
    sql("ALTER DEFAULT PRIVILEGES REVOKE CREATE ON SCHEMAS FROM qa_group")
    result = install()
    assert result.returncode == 0, result.stderr
    # Effective privileges under both direct runtime and SET ROLE identities.
    for statement in ["UPDATE eventstore.events SET data='{}'", "DELETE FROM eventstore.events", "TRUNCATE eventstore.events", "CREATE TABLE eventstore.injected(x int)"]:
        deny(statement)
        deny("SET ROLE qa_group; " + statement)
    deny("SET ROLE justix_identity", "permission denied to set role")
    print("PASS direct and reachable ordinary-role effective ACLs", flush=True)
    event, agg, actor, op, key = [fresh() for _ in range(5)]
    event_sql = f"""INSERT INTO eventstore.events(event_id,event_type,schema_version,aggregate_type,aggregate_id,aggregate_version,company_id,occurred_at,actor_kind,actor_id,correlation_id,causation_id,operation_id,data)
      VALUES('{event}','identity.fixture.recorded.v4294967295',4294967295,'fixture','{agg}',9223372036854775807,NULL,'2026-09-15T00:00:00Z','system','{actor}','{fresh()}','{fresh()}','{op}','{{}}');"""
    operation_sql = f"""INSERT INTO eventstore.operations(operation_id,kind,aggregate_id,actor_id,company_id,admission_ref,intent_revision,request_hash,phase,revision)
      VALUES('{op}','fixture','{agg}','{actor}',NULL,'{fresh()}',0,sha256('safe'::bytea),'prepare',1);"""
    receipt_sql = f"""INSERT INTO eventstore.command_receipts(receipt_id,actor_id,company_id,command_name,target_id,idempotency_key,request_hash,http_status,receipt)
      VALUES('{fresh()}','{actor}',NULL,'fixture',NULL,'{key}',sha256('safe'::bytea),202,'{{"operationId":"{op}","status":"pending"}}');"""
    deny("BEGIN;" + event_sql + operation_sql + receipt_sql + "SELECT 1/0; COMMIT;", "division by zero")
    for table in ["events", "operations", "command_receipts"]:
        assert sql("SELECT count(*) FROM eventstore." + table, "qa_runtime").stdout.strip() == "0"
    sql("BEGIN;" + event_sql + operation_sql + receipt_sql + "COMMIT;", "qa_runtime")
    print("PASS SQL-error rollback and T-007 maximum revision/schema compatibility", flush=True)
    # Competing decisions serialize and only one can establish the final value.
    def decide(value):
        return sql("UPDATE eventstore.operations SET decision='" + value + "' WHERE operation_id='" + op + "'", "qa_runtime", check=False)
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        decisions = list(pool.map(decide, ["commit", "abort"]))
    assert sorted(r.returncode == 0 for r in decisions) == [False, True]
    assert "operation decision is immutable" in next(r.stderr for r in decisions if r.returncode != 0)
    before = sql("SELECT decision FROM eventstore.operations", "qa_runtime").stdout.strip()
    assert before in ["commit", "abort"]
    sql("UPDATE eventstore.operations SET decision='" + before + "'", "qa_runtime")
    deny("UPDATE eventstore.operations SET decision='undecided'", "operation decision is immutable")
    print("PASS concurrent decision race, idempotent same-decision write, reversal rejection", flush=True)
    # Kill PostgreSQL rather than relying on a graceful restart; inspect durable receipt afterward.
    assert run(["docker", "kill", NAME]).returncode == 0
    assert run(["docker", "start", NAME]).returncode == 0
    for attempt in range(100):
        if sql("SELECT 1", check=False).returncode == 0:
            break
        time.sleep(0.1)
    else:
        raise AssertionError("fixture did not recover")
    assert sql("SELECT decision FROM eventstore.operations", "qa_runtime").stdout.strip() == before
    assert sql("SELECT receipt->>'operationId' FROM eventstore.command_receipts WHERE idempotency_key='" + key + "'", "qa_runtime").stdout.strip() == op
    assert sql("SELECT aggregate_version::text||':'||schema_version::text FROM eventstore.events", "qa_runtime").stdout.strip() == "9223372036854775807:4294967295"
    deny(receipt_sql, "duplicate key")
    print("PASS hard-stop/restart preserves authoritative decision, receipt and immutable history", flush=True)
    print("GREEN independent probes at " + EXPECTED, flush=True)
finally:
    if started:
        result = run(["docker", "rm", "-f", NAME])
        assert result.returncode == 0, result.stderr
        print("Fixture removed: " + NAME, flush=True)
