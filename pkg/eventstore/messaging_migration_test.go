package eventstore_test

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// The fixture is deliberately populated before applying the additive migration.
// No existing database, external credentials, or broker is used by this leaf.
func TestMessagingMigrationPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_MESSAGING_MIGRATION") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_MESSAGING_MIGRATION=1 for isolated pinned PostgreSQL")
	}
	f := startFixture(t)
	const owner = "inventory"
	const migration = "justix_inventory"
	const runtime = "justix_inventory_runtime"
	f.mustSQL(owner, "postgres", "CREATE ROLE "+runtime+" LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_inventory TO "+runtime)
	if out, err := f.install(owner, runtime); err != nil {
		t.Fatalf("T-008 fixture installation: %v %s", err, out)
	}
	script, err := os.ReadFile("migrations/000002_messaging_delivery.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	install := func(asOwner string) (string, error) {
		return f.sql(owner, migration, string(script), "-v", "owner_service="+asOwner, "-v", "runtime_role="+runtime)
	}
	failInstall := func(want string) {
		t.Helper()
		out, err := install(owner)
		if err == nil || !strings.Contains(out, want) {
			t.Fatalf("migration expected %q: %v %s", want, err, out)
		}
		if got := f.mustSQL(owner, migration, "SELECT to_regclass('eventstore.messaging_mode') IS NULL"); got != "t" {
			t.Fatal("failed migration left partial schema")
		}
	}
	// Reachable elevated/default privileges must reject the entire migration.
	f.mustSQL(owner, "postgres", "GRANT pg_write_all_data TO "+runtime+" WITH INHERIT FALSE")
	failInstall("privileged ownership or membership")
	f.mustSQL(owner, "postgres", "REVOKE pg_write_all_data FROM "+runtime)
	f.mustSQL(owner, migration, "ALTER DEFAULT PRIVILEGES GRANT UPDATE ON TABLES TO "+runtime)
	failInstall("unexpected messaging table privileges")
	f.mustSQL(owner, migration, "ALTER DEFAULT PRIVILEGES REVOKE UPDATE ON TABLES FROM "+runtime)
	f.mustSQL(owner, migration, "ALTER DEFAULT PRIVILEGES GRANT INSERT ON TABLES TO "+runtime)
	failInstall("unexpected migration evidence INSERT")
	f.mustSQL(owner, migration, "ALTER DEFAULT PRIVILEGES REVOKE INSERT ON TABLES FROM "+runtime)
	f.mustSQL(owner, migration, "ALTER DEFAULT PRIVILEGES GRANT EXECUTE ON FUNCTIONS TO "+runtime)
	failInstall("runtime may activate migration")
	f.mustSQL(owner, migration, "ALTER DEFAULT PRIVILEGES REVOKE EXECUTE ON FUNCTIONS FROM "+runtime)
	if out, err := install("commerce"); err == nil || !strings.Contains(out, "owner default mismatch") {
		t.Fatalf("accepted wrong owner on empty legacy schema: %v %s", err, out)
	}

	stream := uuid.NewString()
	sent, unsent, held := uuid.NewString(), uuid.NewString(), uuid.NewString()
	legacy := func(id string, sequence int, destination string, confirmed bool) string {
		sentSQL := "NULL"
		if confirmed {
			sentSQL = "'2026-09-14T12:00:00Z'"
		}
		return fmt.Sprintf(`INSERT INTO eventstore.outbox(event_id,aggregate_type,aggregate_id,integration_sequence,
 envelope,envelope_hash,exchange,routing_key,attempts,next_attempt_at,lease_owner,lease_until,sent_at)
 VALUES('%s','fixture','%s',%d,'original-%s',sha256('original-%s'::bytea),'justix.integration.v1',
 'inventory.%s.fixture.changed.v1',4,'2026-09-14T10:00:00Z','%s','2026-09-15T12:00:00Z',%s);`,
			id, stream, sequence, id, id, destination, uuid.NewString(), sentSQL)
	}
	f.mustSQL(owner, runtime, legacy(sent, 1, "retail", true)+legacy(unsent, 2, "retail", false)+legacy(held, 3, "financing", false))
	legacySnapshot := f.mustSQL(owner, migration, "SELECT jsonb_agg(to_jsonb(o) ORDER BY event_id) FROM eventstore.outbox o")
	legacyInboxEvent := uuid.NewString()
	f.mustSQL(owner, runtime, fmt.Sprintf("INSERT INTO eventstore.inbox VALUES('legacy.consumer','%s',sha256('legacy'::bytea),clock_timestamp())", legacyInboxEvent))
	if out, err := install(owner); err != nil {
		t.Fatalf("additive migration: %v %s", err, out)
	}
	if out, err := install(owner); err == nil || !strings.Contains(out, "already exists") {
		t.Fatalf("silently reapplied forward migration: %v %s", err, out)
	}
	if got := f.mustSQL(owner, runtime, "SELECT mode FROM eventstore.messaging_mode"); got != "legacy" {
		t.Fatal("installation activated custody")
	}
	f.denied(owner, "UPDATE eventstore.messaging_mode SET mode='custody'", "permission denied")
	f.denied(owner, messagingCutoverSQL(), "permission denied")
	f.denied(owner, "INSERT INTO eventstore.messaging_legacy_authorizations(event_id,destination,hold_ref,authority_ref) VALUES('"+held+"','financing','review','authority')", "permission denied")
	f.denied(owner, messagingParent(uuid.NewString(), stream, 4, "[]"), "mode mismatch")
	f.mustSQL(owner, runtime, "UPDATE eventstore.outbox SET attempts=5 WHERE event_id='"+unsent+"'")
	legacySnapshot = f.mustSQL(owner, migration, "SELECT jsonb_agg(to_jsonb(o) ORDER BY event_id) FROM eventstore.outbox o")

	admit := uuid.NewString()
	f.mustSQL(owner, runtime, messagingAdmission(admit, stream, "source-recipient", "retail"))
	f.denied(owner, strings.Replace(messagingAdmission(uuid.NewString(), stream, "source-recipient", "retail"),
		`["inventory.fixture.changed.v1"]`, `[null]`, 1), "invalid admission schema")
	cutoverFails := func(want string) {
		t.Helper()
		out, err := f.sql(owner, migration, messagingCutoverSQL())
		if err == nil || !strings.Contains(out, want) {
			t.Fatalf("expected cutover %q: %v %s", want, err, out)
		}
		if got := f.mustSQL(owner, runtime, "SELECT mode||':'||(SELECT count(*) FROM eventstore.outbox_messages) FROM eventstore.messaging_mode"); got != "legacy:0" {
			t.Fatalf("failed cutover leaked state: %s", got)
		}
	}
	cutoverFails("unrecognized or unauthorized legacy route")
	f.mustSQL(owner, migration, fmt.Sprintf(`INSERT INTO eventstore.messaging_legacy_authorizations VALUES
 ('%s','retail','%s',NULL,'reviewed-sent-route'),('%s','retail','%s',NULL,'reviewed-unsent-route'),
 ('%s','financing',NULL,'historical-authority-unavailable','reviewed-hold');`, sent, admit, unsent, admit, held))
	// Unknown route is rejected even when an authority record names a destination.
	bad := uuid.NewString()
	f.mustSQL(owner, runtime, legacy(bad, 4, "unknown", false))
	f.mustSQL(owner, migration, fmt.Sprintf("INSERT INTO eventstore.messaging_legacy_authorizations VALUES('%s','retail','%s',NULL,'fixture-review')", bad, admit))
	cutoverFails("unrecognized or unauthorized legacy route")
	// Test-only repair of the original fixture row; never a migration feature.
	f.mustSQL(owner, migration, "UPDATE eventstore.outbox SET routing_key='inventory.retail.fixture.changed.v1' WHERE event_id='"+bad+"'")
	legacySnapshot = f.mustSQL(owner, migration, "SELECT jsonb_agg(to_jsonb(o) ORDER BY event_id) FROM eventstore.outbox o")
	// A reachable ordinary role can retain legacy INSERT despite direct revocation.
	f.mustSQL(owner, "postgres", "CREATE ROLE messaging_legacy_writer; GRANT messaging_legacy_writer TO "+runtime+" WITH INHERIT FALSE")
	f.mustSQL(owner, migration, "GRANT INSERT ON eventstore.outbox TO messaging_legacy_writer")
	cutoverFails("legacy runtime grants remain writable")
	f.mustSQL(owner, migration, "REVOKE INSERT ON eventstore.outbox FROM messaging_legacy_writer")
	f.mustSQL(owner, migration, messagingCutoverSQL())
	if got := f.mustSQL(owner, runtime, "SELECT mode FROM eventstore.messaging_mode"); got != "custody" {
		t.Fatalf("cutover mode: %s", got)
	}
	if got := f.mustSQL(owner, runtime, "SELECT jsonb_agg(to_jsonb(o) ORDER BY event_id) FROM eventstore.outbox o"); got != legacySnapshot {
		t.Fatal("legacy bytes, routing or scheduling evidence changed")
	}
	if got := f.mustSQL(owner, runtime, "SELECT jsonb_agg(original_row ORDER BY event_id) FROM eventstore.messaging_legacy_evidence"); got != legacySnapshot {
		t.Fatal("migration evidence differs from original populated rows")
	}
	if got := f.mustSQL(owner, runtime, `SELECT count(*) FROM eventstore.outbox_messages m JOIN eventstore.outbox o USING(event_id)
 WHERE m.envelope=o.envelope AND m.envelope_hash=o.envelope_hash AND m.created_at=o.created_at`); got != "4" {
		t.Fatal("copy did not preserve all identities and bytes")
	}
	if got := f.mustSQL(owner, runtime, `SELECT count(*) FROM eventstore.outbox_deliveries d JOIN eventstore.outbox o USING(event_id)
 WHERE d.sent_at IS NOT DISTINCT FROM o.sent_at AND d.lease_owner IS NULL AND d.lease_until IS NULL AND d.attempts=o.attempts`); got != "4" {
		t.Fatal("copy changed per-child confirmation evidence or retained active leases")
	}
	if got := f.mustSQL(owner, runtime, "SELECT legacy_rows||':'||legacy_inbox_rows FROM eventstore.messaging_cutovers"); got != "4:1" {
		t.Fatalf("cutover counts: %s", got)
	}
	if got := f.mustSQL(owner, runtime, "SELECT count(*) FROM eventstore.inbox WHERE consumer_name='legacy.consumer'"); got != "1" {
		t.Fatal("cutover discarded direct consumer inbox")
	}
	for _, sql := range []string{
		legacy(uuid.NewString(), 10, "retail", false),
		"UPDATE eventstore.outbox SET attempts=0", "DELETE FROM eventstore.outbox", "TRUNCATE eventstore.outbox",
		"UPDATE eventstore.outbox_messages SET envelope='changed'", "UPDATE eventstore.outbox_deliveries SET routing_key='changed'",
		"UPDATE eventstore.messaging_admissions SET purpose='changed'", "DELETE FROM eventstore.dispatch_jobs",
		"CREATE TABLE eventstore.injected(id integer)",
	} {
		f.denied(owner, sql, "permission denied")
	}
	if out, err := f.sql(owner, migration, legacy(uuid.NewString(), 10, "retail", false)); err == nil || !strings.Contains(out, "mode mismatch") {
		t.Fatalf("old writer bypassed schema fence as migration owner: %v %s", err, out)
	}
	f.denied(owner, "UPDATE eventstore.outbox_deliveries SET hold_ref=NULL WHERE event_id='"+held+"'", "retain its evidenced hold")
	f.denied(owner, "UPDATE eventstore.outbox_deliveries SET sent_at=clock_timestamp() WHERE event_id='"+held+"'", "held delivery")
	f.denied(owner, "UPDATE eventstore.outbox_deliveries SET sent_at=NULL WHERE event_id='"+sent+"'", "confirmed status is immutable")
	f.mustSQL(owner, runtime, "UPDATE eventstore.outbox_deliveries SET attempts=attempts+1,lease_owner='"+uuid.NewString()+"',lease_until=clock_timestamp()+interval '1 minute' WHERE event_id='"+unsent+"'")
	f.denied(owner, "INSERT INTO eventstore.inbox VALUES('old.binary','"+uuid.NewString()+"',sha256('old'::bytea),clock_timestamp())", "explicit transaction adapter")
	if out, err := f.sql(owner, migration, "UPDATE eventstore.messaging_mode SET mode='legacy'"); err == nil || !strings.Contains(out, "forward custody transition") {
		t.Fatalf("allowed destructive mode rollback: %v %s", err, out)
	}

	t.Run("complete source plan", func(t *testing.T) {
		f := f
		f.t = t
		id := uuid.NewString()
		plan := fmt.Sprintf(`[{"destination":"retail","admission_id":"%s"}]`, admit)
		parent := messagingParent(id, stream, 10, plan)
		child := fmt.Sprintf(`INSERT INTO eventstore.outbox_deliveries(event_id,destination,admission_id,exchange,routing_key)
 VALUES('%s','retail','%s','justix.integration.v1','inventory.retail.fixture.changed.v1');`, id, admit)
		f.denied(owner, "BEGIN;"+parent+"COMMIT;", "incomplete or conflicting")
		if got := f.mustSQL(owner, runtime, "SELECT count(*) FROM eventstore.outbox_messages WHERE event_id='"+id+"'"); got != "0" {
			t.Fatal("incomplete parent persisted")
		}
		f.mustSQL(owner, runtime, "BEGIN;"+parent+child+"ROLLBACK;")
		f.denied(owner, "BEGIN;"+parent+strings.Replace(child, "fixture.changed.v1", "fixture.changed.v2", 1)+"COMMIT;", "schema is not admitted")
		f.denied(owner, "BEGIN;"+messagingParent(id, stream, 10, "[]")+child+"COMMIT;", "incomplete or conflicting")
		f.mustSQL(owner, runtime, "BEGIN;"+parent+child+"COMMIT;")
		f.denied(owner, "BEGIN;"+parent+child+"COMMIT;", "duplicate key")
		f.denied(owner, strings.Replace(messagingParent(uuid.NewString(), stream, 11, "[]"), "sha256('message'::bytea)", "sha256('wrong'::bytea)", 1), "check constraint")
		f.mustSQL(owner, runtime, messagingParent(uuid.NewString(), stream, 11, "[]"))
		// An empty set is syntactically supported, but still requires an explicit
		// plan authority reference. Owner adapters decide if that set is permitted.
		f.denied(owner, strings.Replace(messagingParent(uuid.NewString(), stream, 12, "[]"), "'fixture-plan-authority'", "''", 1), "check constraint")
		dupID := uuid.NewString()
		dupPlan := strings.TrimSuffix(plan, "]") + "," + strings.TrimPrefix(plan, "[")
		f.denied(owner, "BEGIN;"+messagingParent(dupID, stream, 12, dupPlan)+strings.ReplaceAll(child, id, dupID)+"COMMIT;", "incomplete or conflicting")
		f.denied(owner, "BEGIN;"+messagingParent(uuid.NewString(), stream, 13, "[null]")+"COMMIT;", "incomplete or conflicting")
		// Competing connections cannot reserve the same stream position twice.
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := f.sql(owner, runtime, messagingParent(uuid.NewString(), stream, 20, "[]"))
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		success := 0
		for err := range errs {
			if err == nil {
				success++
			}
		}
		if success != 1 {
			t.Fatalf("competing stream positions: %d successes", success)
		}
	})
	t.Run("custody enrollment and inbox completion", func(t *testing.T) { f := f; f.t = t; checkMessagingDispatch(f) })
	t.Run("conflicting historical authority", func(t *testing.T) {
		f := f
		f.t = t
		const owner, migration, runtime = "commerce", "justix_commerce", "justix_commerce_runtime"
		f.mustSQL(owner, "postgres", "CREATE ROLE "+runtime+" LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_commerce TO "+runtime)
		if out, err := f.install(owner, runtime); err != nil {
			t.Fatalf("base fixture: %v %s", err, out)
		}
		if out, err := f.sql(owner, migration, string(script), "-v", "owner_service="+owner, "-v", "runtime_role="+runtime); err != nil {
			t.Fatalf("migration: %v %s", err, out)
		}
		id, admit := uuid.NewString(), uuid.NewString()
		f.mustSQL(owner, runtime, strings.ReplaceAll(legacy(id, 1, "financing", false), "inventory", "commerce"))
		f.mustSQL(owner, runtime, strings.ReplaceAll(messagingAdmission(admit, stream, "source-recipient", "retail"), "inventory", "commerce"))
		f.mustSQL(owner, migration, fmt.Sprintf("INSERT INTO eventstore.messaging_legacy_authorizations VALUES('%s','financing','%s',NULL,'conflicting-fixture-review')", id, admit))
		if out, err := f.sql(owner, migration, messagingCutoverSQL()); err == nil || !strings.Contains(out, "delivery admission mismatch") {
			t.Fatalf("accepted conflicting historical admission: %v %s", err, out)
		}
		if got := f.mustSQL(owner, runtime, `SELECT mode||':'||(SELECT count(*) FROM eventstore.outbox)||':'||
 (SELECT count(*) FROM eventstore.outbox_messages)||':'||(SELECT count(*) FROM eventstore.messaging_legacy_evidence) FROM eventstore.messaging_mode`); got != "legacy:1:0:0" {
			t.Fatalf("failed populated copy leaked state: %s", got)
		}
		f.mustSQL(owner, runtime, "UPDATE eventstore.outbox SET attempts=6")
	})
	t.Run("explicit empty legacy assertion", func(t *testing.T) {
		f := f
		f.t = t
		const owner, migration, runtime = "insurance", "justix_insurance", "justix_insurance_runtime"
		f.mustSQL(owner, "postgres", "CREATE ROLE "+runtime+" LOGIN PASSWORD '"+f.password+"'; GRANT CONNECT ON DATABASE justix_insurance TO "+runtime)
		if out, err := f.install(owner, runtime); err != nil {
			t.Fatalf("base fixture: %v %s", err, out)
		}
		if out, err := f.sql(owner, migration, string(script), "-v", "owner_service="+owner, "-v", "runtime_role="+runtime); err != nil {
			t.Fatalf("migration: %v %s", err, out)
		}
		f.mustSQL(owner, migration, messagingCutoverSQL())
		if got := f.mustSQL(owner, runtime, "SELECT legacy_rows||':'||legacy_inbox_rows FROM eventstore.messaging_cutovers"); got != "0:0" {
			t.Fatalf("empty fixture not explicitly established: %s", got)
		}
	})
}

func messagingAdmission(id, stream, namespace, subject string) string {
	kind, generation := "NULL", "NULL"
	if namespace == "local-consumer" {
		kind, generation = "'projection'", "'generation-1'"
	}
	return fmt.Sprintf(`INSERT INTO eventstore.messaging_admissions(admission_id,namespace,source_owner,
 aggregate_type,aggregate_id,subject,action,contract_id,contract_version,contract_digest,schema_allowlist,
 authority_ref,scope_ref,purpose,bootstrap_ref,start_after,consumer_kind,generation)
 VALUES('%s','%s','inventory','fixture','%s','%s','admit','fixture.contract',1,sha256('contract'::bytea),
 '["inventory.fixture.changed.v1"]','fixture-authority','fixture-scope','fixture-purpose','explicit-empty-stream',0,%s,%s);`, id, namespace, stream, subject, kind, generation)
}

func messagingCutoverSQL() string {
	return fmt.Sprintf(`SELECT eventstore.activate_messaging_custody('%s',sha256('new-schema'::bytea),
 sha256('prior-schema'::bytea),'synthetic-backup-reference','synthetic-stopped-runtime-reference',
 'synthetic-broker-checkpoint-reference','synthetic-compatibility-reference');`, uuid.NewString())
}

func messagingParent(id, stream string, sequence int, recipients string) string {
	return fmt.Sprintf(`INSERT INTO eventstore.outbox_messages(event_id,source_owner,aggregate_type,aggregate_id,
 integration_sequence,envelope,envelope_hash,recipients,plan_authority_ref)
 VALUES('%s','inventory','fixture','%s',%d,'message',sha256('message'::bytea),'%s','fixture-plan-authority');`, id, stream, sequence, recipients)
}

func checkMessagingDispatch(f schemaFixture) {
	t := f.t
	const owner, runtime = "inventory", "justix_inventory_runtime"
	stream, id, enrollment, admission := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	f.mustSQL(owner, runtime, messagingAdmission(admission, stream, "local-consumer", "fixture.consumer"))
	message := fmt.Sprintf(`INSERT INTO eventstore.dispatch_messages(event_id,source_owner,aggregate_type,aggregate_id,
 integration_sequence,envelope,envelope_hash,exchange,routing_key,membership_version,admission_ref,bootstrap_ref,initial_enrollment_id)
 VALUES('%s','inventory','fixture','%s',1,'custody',sha256('custody'::bytea),'justix.integration.v1',
 'inventory.inventory.fixture.changed.v1','membership-1','receiver-admission','source-checkpoint','%s');`, id, stream, enrollment)
	enroll := fmt.Sprintf(`INSERT INTO eventstore.dispatch_enrollments(event_id,enrollment_id,membership_version,cutover_ref,consumers)
 VALUES('%s','%s','membership-1','admitted-cutover','[{"consumer_name":"fixture.consumer","admission_id":"%s"}]');`, id, enrollment, admission)
	job := fmt.Sprintf(`INSERT INTO eventstore.dispatch_jobs(consumer_name,event_id,enrollment_id,admission_id,consumer_kind,generation)
 VALUES('fixture.consumer','%s','%s','%s','projection','generation-1');`, id, enrollment, admission)
	f.denied(owner, "BEGIN;"+message+"COMMIT;", "dispatch_initial_enrollment")
	f.denied(owner, "BEGIN;"+message+enroll+"COMMIT;", "incomplete or conflicting")
	f.denied(owner, "BEGIN;"+message+strings.Replace(enroll, "'membership-1'", "'membership-2'", 1)+job+"COMMIT;", "initial membership mismatch")
	f.denied(owner, "BEGIN;"+message+enroll+strings.Replace(job, "'projection'", "'process'", 1)+"COMMIT;", "job admission mismatch")
	f.mustSQL(owner, runtime, "BEGIN;"+message+enroll+job+"COMMIT;")
	f.mustSQL(owner, runtime, "UPDATE eventstore.dispatch_jobs SET attempts=attempts+1,hold_ref='review-required' WHERE event_id='"+id+"'")
	complete := fmt.Sprintf(`UPDATE eventstore.dispatch_jobs SET completed_at='2026-09-15T00:00:00Z',
 inbox_consumer_name='fixture.consumer',inbox_event_id='%s' WHERE event_id='%s' AND consumer_name='fixture.consumer';`, id, id)
	f.denied(owner, complete, "matching inbox identity and bytes")
	inbox := fmt.Sprintf("INSERT INTO eventstore.inbox VALUES('fixture.consumer','%s',sha256('custody'::bytea),clock_timestamp());", id)
	const begin = "BEGIN; SET LOCAL justix.messaging_mode='custody';"
	f.denied(owner, begin+strings.Replace(inbox, "sha256('custody'::bytea)", "sha256('conflict'::bytea)", 1)+complete+"COMMIT;", "matching inbox identity and bytes")
	f.denied(owner, begin+inbox+complete+"COMMIT;", "held or quarantined job")
	f.mustSQL(owner, runtime, "UPDATE eventstore.dispatch_jobs SET hold_ref=NULL WHERE event_id='"+id+"'")
	f.mustSQL(owner, runtime, begin+inbox+complete+"ROLLBACK;")
	if got := f.mustSQL(owner, runtime, "SELECT count(*) FROM eventstore.inbox WHERE event_id='"+id+"'"); got != "0" {
		t.Fatal("rolled back inbox persisted")
	}
	f.mustSQL(owner, runtime, begin+inbox+complete+"COMMIT;")
	f.denied(owner, "UPDATE eventstore.dispatch_jobs SET completed_at=NULL,inbox_consumer_name=NULL,inbox_event_id=NULL WHERE event_id='"+id+"'", "completion is immutable")
	f.denied(owner, "UPDATE eventstore.dispatch_jobs SET admission_id='"+uuid.NewString()+"'", "permission denied")
	f.denied(owner, "UPDATE eventstore.dispatch_enrollments SET consumers='[]'", "permission denied")
	// A late consumer receives a new immutable enrollment. Original membership,
	// original jobs and completed inbox remain unchanged.
	lateAdmission, lateEnrollment := uuid.NewString(), uuid.NewString()
	f.mustSQL(owner, runtime, messagingAdmission(lateAdmission, stream, "local-consumer", "late.consumer"))
	lateEnroll := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(enroll, enrollment, lateEnrollment), admission, lateAdmission), "fixture.consumer", "late.consumer")
	lateJob := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(job, enrollment, lateEnrollment), admission, lateAdmission), "fixture.consumer", "late.consumer")
	f.denied(owner, "BEGIN;"+lateEnroll+"COMMIT;", "incomplete or conflicting")
	f.mustSQL(owner, runtime, "BEGIN;"+lateEnroll+lateJob+"COMMIT;")
	if got := f.mustSQL(owner, runtime, "SELECT count(*)||':'||count(completed_at) FROM eventstore.dispatch_jobs WHERE event_id='"+id+"'"); got != "2:1" {
		t.Fatalf("independent jobs: %s", got)
	}
	f.denied(owner, strings.ReplaceAll(complete, "fixture.consumer", "late.consumer"), "matching inbox identity and bytes")
	f.denied(owner, "BEGIN;"+strings.ReplaceAll(lateEnroll, lateEnrollment, uuid.NewString())+"COMMIT;", "incomplete or conflicting")
}
