package eventstore_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
)

const checkpointBaseHash = "86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53"
const checkpointRouteHash = "c198af498e39056a7a251b5906d6db050ad994e04588bf6cecf8316221ea85ec"

// Reuses the integrated T925 disposable fixture only: random loopback container,
// PG 180006, isolated migration/runtime roles, no existing DSN. Every admission,
// manifest, source proof and snapshot in this leaf is synthetic test evidence.
func checkpointStore(t *testing.T, f *routeFixture) *routeStore {
	t.Helper()
	s := f.store(t, "inventory", true)
	if out, err := s.install(f.correction); err != nil {
		t.Fatalf("000003: %v %s", err, out)
	}
	s.must(t, s.migration, "REVOKE TEMP ON DATABASE "+s.database+" FROM PUBLIC")
	return s
}

func checkpointScript(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("migrations/000004_projection_checkpoint.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func installCheckpoint(t *testing.T, s *routeStore, overrides ...string) (string, error) {
	t.Helper()
	script := checkpointScript(t)
	args := []string{"-v", "base_schema_sha256=" + checkpointBaseHash, "-v", fmt.Sprintf("checkpoint_migration_sha256=%x", sha256.Sum256([]byte(script)))}
	return s.install(script, append(args, overrides...)...)
}
func mustCheckpoint(t *testing.T, s *routeStore) {
	t.Helper()
	if out, err := installCheckpoint(t, s); err != nil {
		t.Fatalf("000004: %v %s", err, out)
	}
}

type checkpointIdentity struct {
	bootstrap, consumer, stream, generation, request string
	start                                            int64
}

func checkpointID(start int64) checkpointIdentity {
	return checkpointIdentity{uuid.NewString(), "fixture-" + uuid.NewString(), uuid.NewString(), "generation-1", uuid.NewString(), start}
}
func bootstrapSQL(b checkpointIdentity, admission string, kind string) string {
	a := "NULL"
	if admission != "" {
		a = routeQuote(admission)
	}
	empty := "NULL"
	if b.start == 0 {
		empty = "'synthetic-source-empty-proof'"
	}
	return fmt.Sprintf(`INSERT INTO eventstore.consumer_bootstraps(bootstrap_id,consumer_name,source_owner,aggregate_type,aggregate_id,
 consumer_kind,generation,admission_id,contract_id,contract_version,contract_digest,authority_ref,scope_ref,purpose_ref,
 source_checkpoint_ref,start_after,no_snapshot_contract_ref,new_empty_proof_ref,installation_request_id)
 VALUES('%s','%s','inventory','fixture','%s','%s','%s',%s,'fixture-contract',1,decode(repeat('11',32),'hex'),
 'fixture-authority','fixture-scope','fixture-purpose','synthetic-source-checkpoint',%d,'synthetic-no-snapshot-contract',%s,'%s');`, b.bootstrap, b.consumer, b.stream, kind, b.generation, a, b.start, empty, b.request)
}
func checkpointSQL(b checkpointIdentity) string {
	return fmt.Sprintf(`INSERT INTO eventstore.consumer_checkpoints(consumer_name,source_owner,aggregate_type,aggregate_id,bootstrap_id,consumer_kind,generation,position,revision)
 SELECT consumer_name,source_owner,aggregate_type,aggregate_id,bootstrap_id,consumer_kind,generation,start_after,0 FROM eventstore.consumer_bootstraps WHERE bootstrap_id='%s';`, b.bootstrap)
}
func checkpointWhere(b checkpointIdentity) string {
	return fmt.Sprintf("consumer_name='%s' AND source_owner='inventory' AND aggregate_type='fixture' AND aggregate_id='%s'", b.consumer, b.stream)
}
func checkpointAdvance(b checkpointIdentity, event string, position int64) string {
	return fmt.Sprintf(`INSERT INTO eventstore.inbox(consumer_name,event_id,envelope_hash) VALUES('%s','%s',decode(repeat('22',32),'hex'));
 UPDATE eventstore.consumer_checkpoints SET position=%d,revision=revision+1,last_event_id='%s',last_event_hash=decode(repeat('22',32),'hex'),updated_at=clock_timestamp() WHERE %s;`, b.consumer, event, position, event, checkpointWhere(b))
}
func checkpointAdmission(b checkpointIdentity, id, kind string) string {
	return fmt.Sprintf(`INSERT INTO eventstore.messaging_admissions(admission_id,namespace,source_owner,aggregate_type,aggregate_id,subject,action,
 contract_id,contract_version,contract_digest,schema_allowlist,authority_ref,scope_ref,purpose,bootstrap_ref,start_after,consumer_kind,generation)
 VALUES('%s','local-consumer','inventory','fixture','%s','%s','admit','fixture-contract',1,decode(repeat('11',32),'hex'),
 '["inventory.fixture.changed.v1"]','fixture-authority','fixture-scope','fixture-purpose','synthetic-bootstrap',%d,'%s','%s');`, id, b.stream, b.consumer, b.start, kind, b.generation)
}
func gapSQL(b checkpointIdentity, gap, event string, expected, observed int64) string {
	return fmt.Sprintf(`INSERT INTO eventstore.consumer_gaps(gap_id,consumer_name,source_owner,aggregate_type,aggregate_id,bootstrap_id,blocked_event_id,blocked_event_hash,
 expected_position,observed_position,admission_ref,authority_ref,request_id) VALUES('%s','%s','inventory','fixture','%s','%s','%s',decode(repeat('33',32),'hex'),%d,%d,'synthetic-admission','synthetic-authority','%s');`, gap, b.consumer, b.stream, b.bootstrap, event, expected, observed, uuid.NewString())
}
func attemptSQL(gap, id, prior, action string, from, through, position int64) string {
	p := "NULL"
	if prior != "" {
		p = routeQuote(prior)
	}
	manifest, hash, reason := "NULL", "NULL", "NULL"
	if action == "recovered" {
		manifest = "'synthetic-checked-history'"
		hash = "decode(repeat('44',32),'hex')"
	}
	if action == "held" {
		reason = "'source-unavailable'"
	}
	return fmt.Sprintf(`INSERT INTO eventstore.consumer_gap_attempts(attempt_id,gap_id,prior_attempt_id,request_id,action,evidence_format_version,
 interval_from,interval_through,recovery_manifest_ref,recovery_manifest_hash,hold_reason_code,authority_ref,resulting_position)
 VALUES('%s','%s',%s,'%s','%s',1,%d,%d,%s,%s,%s,'synthetic-authority',%d);`, id, gap, p, uuid.NewString(), action, from, through, manifest, hash, reason, position)
}

func checkpointPreimage(t *testing.T, s *routeStore) string {
	t.Helper()
	return s.snapshot(t) + "\n" + s.must(t, s.migration, "SELECT row_to_json(t) FROM eventstore.messaging_route_compatibility t") + "\n" + s.functions(t, true) + "\n" + s.functions(t, false)
}

func checkpointSaveEvidence(t *testing.T, s *routeStore, label string) {
	t.Helper()
	s.saveEvidence(t, label)
	content := []byte(checkpointPreimage(t, s) + "\n")
	path := filepath.Join(s.f.evidence, s.database+"-"+label+"-complete-preimage.txt")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".sha256", []byte(fmt.Sprintf("%x\n", sha256.Sum256(content))), 0600); err != nil {
		t.Fatal(err)
	}
}
func checkpointRejectInstall(t *testing.T, s *routeStore, overrides ...string) {
	t.Helper()
	before := checkpointPreimage(t, s)
	catalog := s.catalog(t)
	if out, err := installCheckpoint(t, s, overrides...); err == nil {
		t.Fatalf("unexpected install success: %s", out)
	}
	if checkpointPreimage(t, s) != before || s.catalog(t) != catalog {
		t.Fatal("failed install changed retained preimage")
	}
	if got := s.must(t, s.migration, "SELECT count(*) FROM pg_class WHERE relnamespace='eventstore'::regnamespace AND relname LIKE 'consumer_%'"); got != "0" {
		t.Fatalf("partial DDL: %s", got)
	}
}

func TestProjectionCheckpointMigrationPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_PROJECTION_CHECKPOINT") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_PROJECTION_CHECKPOINT=1 for owned PostgreSQL fixture")
	}
	f := newRouteFixture(t)
	if fmt.Sprintf("%x", sha256.Sum256([]byte(f.base))) != checkpointBaseHash || fmt.Sprintf("%x", sha256.Sum256([]byte(f.correction))) != checkpointRouteHash {
		t.Fatal("prior artifact changed")
	}
	t.Logf("000004 SHA-256 %x", sha256.Sum256([]byte(checkpointScript(t))))
	t.Run("preserves populated prior data functions grants and mode", func(t *testing.T) {
		s := checkpointStore(t, f)
		s.produce(t, events.OwnerRetail)
		checkpointSaveEvidence(t, s, "checkpoint-before-legacy")
		before := checkpointPreimage(t, s)
		oldCatalog := s.catalog(t)
		mustCheckpoint(t, s)
		// New functions are additional; all existing definitions/ACLs remain exact.
		parts := strings.Split(before, "\n")
		afterParts := strings.Split(checkpointPreimage(t, s), "\n")
		if parts[0] != afterParts[0] || parts[1] != afterParts[1] {
			t.Fatal("retained data/marker changed")
		}
		for i := 2; i < 4; i++ {
			var old, new map[string]json.RawMessage
			if err := json.Unmarshal([]byte(parts[i]), &old); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(afterParts[i]), &new); err != nil {
				t.Fatal(err)
			}
			for k, v := range old {
				if string(v) != string(new[k]) {
					t.Fatalf("old function changed %s", k)
				}
			}
		}
		var old, new map[string]map[string]json.RawMessage
		_ = json.Unmarshal([]byte(oldCatalog), &old)
		_ = json.Unmarshal([]byte(s.catalog(t)), &new)
		for k, v := range old {
			for field, value := range v {
				if field != "triggers" && string(value) != string(new[k][field]) {
					t.Fatalf("old table catalog changed %s.%s", k, field)
				}
			}
			var oldTriggers, newTriggers []json.RawMessage
			_ = json.Unmarshal(v["triggers"], &oldTriggers)
			_ = json.Unmarshal(new[k]["triggers"], &newTriggers)
			for _, trigger := range oldTriggers {
				found := false
				for _, candidate := range newTriggers {
					if string(trigger) == string(candidate) {
						found = true
					}
				}
				if !found {
					t.Fatalf("prior trigger changed on %s", k)
				}
			}
			if k != "messaging_admissions" && len(oldTriggers) != len(newTriggers) {
				t.Fatalf("unexpected added prior-table trigger on %s", k)
			}
		}
		// The new bootstrap FK necessarily adds two PostgreSQL internal RI
		// triggers to its old admission parent. Existing trigger OIDs stay intact.
		if got := s.must(t, s.migration, "SELECT count(*) FROM pg_trigger WHERE tgrelid='eventstore.messaging_admissions'::regclass AND tgconstrrelid='eventstore.consumer_bootstraps'::regclass AND tgisinternal"); got != "2" {
			t.Fatal("new FK trigger shape", got)
		}
		if got := s.must(t, s.runtime, "SELECT base_schema_version||':'||migration_revision||':'||feature_format_version FROM eventstore.projection_checkpoint_compatibility"); got != "2:4:1" {
			t.Fatal(got)
		}
		for _, q := range []string{"UPDATE eventstore.projection_checkpoint_compatibility SET feature_format_version=1", "INSERT INTO eventstore.projection_checkpoint_compatibility SELECT * FROM eventstore.projection_checkpoint_compatibility", "UPDATE eventstore.consumer_bootstraps SET start_after=0", "UPDATE eventstore.consumer_gaps SET expected_position=1", "UPDATE eventstore.consumer_gap_attempts SET action='requested'", "CREATE TABLE eventstore.forbidden(id int)", "SELECT eventstore.consumer_checkpoint_guard()"} {
			s.reject(t, s.runtime, q, "permission denied")
		}
		if out, err := installCheckpoint(t, s); err == nil || !strings.Contains(out, "already exists") {
			t.Fatalf("reinstall %v %s", err, out)
		}
		checkpointSaveEvidence(t, s, "checkpoint-after-legacy")
		foreign := f.store(t, "retail", true)
		if out, err := foreign.install(f.correction); err != nil {
			t.Fatalf("foreign owner fixture prerequisite: %v %s", err, out)
		}
		foreign.must(t, foreign.migration, "REVOKE TEMP ON DATABASE "+foreign.database+" FROM PUBLIC")
		mustCheckpoint(t, foreign)
		for _, pair := range [][2]*routeStore{{s, foreign}, {foreign, s}} {
			out, err := f.sql(pair[1].database, pair[0].runtime, "SELECT count(*) FROM eventstore.consumer_checkpoints")
			if err == nil || !strings.Contains(out, "permission denied for schema eventstore") {
				t.Fatalf("foreign owner data access: %v %s", err, out)
			}
		}
	})
	t.Run("bootstrap exact evidence no reset and contiguous inbox identity", func(t *testing.T) {
		s := checkpointStore(t, f)
		mustCheckpoint(t, s)
		b := checkpointID(7)
		s.must(t, s.runtime, "BEGIN;"+bootstrapSQL(b, "", "projection")+checkpointSQL(b)+"COMMIT;")
		if got := s.must(t, s.runtime, "SELECT position FROM eventstore.consumer_checkpoints WHERE "+checkpointWhere(b)); got != "7" {
			t.Fatal(got)
		}
		// Request replay can be reconciled by exact retained identity; plain repeat
		// INSERT conflicts, and a conflicting request never mutates the old row.
		s.reject(t, s.runtime, bootstrapSQL(b, "", "projection"), "duplicate key")
		second := b
		second.bootstrap = uuid.NewString()
		second.request = uuid.NewString()
		second.start = 0
		s.reject(t, s.runtime, bootstrapSQL(second, "", "projection"), "duplicate key")
		s.reject(t, s.runtime, "UPDATE eventstore.consumer_checkpoints SET bootstrap_id='"+uuid.NewString()+"' WHERE "+checkpointWhere(b), "permission denied")
		for _, q := range []string{
			"UPDATE eventstore.consumer_checkpoints SET position=9,revision=1,last_event_id='" + uuid.NewString() + "',last_event_hash=decode(repeat('22',32),'hex') WHERE " + checkpointWhere(b),
			"UPDATE eventstore.consumer_checkpoints SET position=8,revision=1,last_event_id='" + uuid.NewString() + "',last_event_hash=decode(repeat('22',32),'hex') WHERE " + checkpointWhere(b),
			"UPDATE eventstore.consumer_checkpoints SET position=0,revision=0 WHERE " + checkpointWhere(b),
		} {
			s.reject(t, s.runtime, q, "checkpoint")
		}
		event := uuid.NewString()
		s.must(t, s.runtime, "BEGIN;"+checkpointAdvance(b, event, 8)+"COMMIT;")
		s.reject(t, s.runtime, "UPDATE eventstore.consumer_checkpoints SET position=9,revision=2 WHERE "+checkpointWhere(b), "checkpoint")
		wrong := uuid.NewString()
		s.must(t, s.runtime, fmt.Sprintf("INSERT INTO eventstore.inbox VALUES('other-consumer','%s',decode(repeat('22',32),'hex'),now())", wrong))
		s.reject(t, s.runtime, fmt.Sprintf("UPDATE eventstore.consumer_checkpoints SET position=9,revision=2,last_event_id='%s' WHERE %s", wrong, checkpointWhere(b)), "matching inbox")
		wrongHash := uuid.NewString()
		s.must(t, s.runtime, fmt.Sprintf("INSERT INTO eventstore.inbox VALUES('%s','%s',decode(repeat('77',32),'hex'),now())", b.consumer, wrongHash))
		s.reject(t, s.runtime, fmt.Sprintf("UPDATE eventstore.consumer_checkpoints SET position=9,revision=2,last_event_id='%s' WHERE %s", wrongHash, checkpointWhere(b)), "matching inbox")
		zero := checkpointID(0)
		s.reject(t, s.runtime, strings.Replace(bootstrapSQL(zero, "", "projection"), "'synthetic-source-empty-proof'", "NULL", 1), "check constraint")
		s.must(t, s.runtime, "BEGIN;"+bootstrapSQL(zero, "", "projection")+checkpointSQL(zero)+"COMMIT;")
		for _, change := range []struct{ old, new string }{{"'fixture-authority'", "' '"}, {"'generation-1'", "''"}, {"'synthetic-no-snapshot-contract'", "NULL"}, {"repeat('11',32)", "repeat('11',31)"}} {
			bad := checkpointID(3)
			s.reject(t, s.runtime, strings.Replace(bootstrapSQL(bad, "", "projection"), change.old, change.new, 1), "check constraint")
		}
		bad := checkpointID(4)
		s.must(t, s.runtime, bootstrapSQL(bad, "", "projection"))
		s.reject(t, s.runtime, strings.Replace(checkpointSQL(bad), "start_after,0", "0,0", 1), "exact bootstrap")
		// Two consumers/generations may consume one event independently.
		other := checkpointID(7)
		other.stream = b.stream
		other.generation = "generation-two"
		s.must(t, s.runtime, "BEGIN;"+bootstrapSQL(other, "", "projection")+checkpointSQL(other)+checkpointAdvance(other, event, 8)+"COMMIT;")
	})
	t.Run("custody requires exact non UUID projection and process admission labels", func(t *testing.T) {
		s := checkpointStore(t, f)
		mustCheckpoint(t, s)
		s.cutover(t)
		for _, kind := range []string{"projection", "process"} {
			b := checkpointID(12)
			b.generation = "retained-" + kind + "-label"
			admission := uuid.NewString()
			s.must(t, s.runtime, checkpointAdmission(b, admission, kind))
			s.reject(t, s.runtime, bootstrapSQL(b, "", kind), "requires root local admission")
			wrong := b
			wrong.generation += "-wrong"
			s.reject(t, s.runtime, bootstrapSQL(wrong, admission, kind), "identity mismatch")
			wrongKind := "projection"
			if kind == wrongKind {
				wrongKind = "process"
			}
			s.reject(t, s.runtime, bootstrapSQL(b, admission, wrongKind), "identity mismatch")
			wrong = b
			wrong.generation = ""
			s.reject(t, s.runtime, bootstrapSQL(wrong, admission, kind), "identity mismatch")
			s.must(t, s.runtime, "BEGIN;"+bootstrapSQL(b, admission, kind)+checkpointSQL(b)+"COMMIT;")
			if got := s.must(t, s.runtime, "SELECT generation FROM eventstore.consumer_bootstraps WHERE bootstrap_id='"+b.bootstrap+"'"); got != b.generation {
				t.Fatal(got)
			}
		}
	})
	t.Run("effect rollback followed by durable append only gap chain", func(t *testing.T) {
		s := checkpointStore(t, f)
		mustCheckpoint(t, s)
		b := checkpointID(7)
		s.must(t, s.runtime, "BEGIN;"+bootstrapSQL(b, "", "projection")+checkpointSQL(b)+"COMMIT;")
		event := uuid.NewString()
		s.reject(t, s.runtime, "BEGIN;"+checkpointAdvance(b, event, 10)+"COMMIT;", "contiguous increment")
		if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.inbox WHERE event_id='"+event+"'"); got != "0" {
			t.Fatal("failed effect inbox retained")
		}
		gap, root := uuid.NewString(), uuid.NewString()
		s.must(t, s.runtime, "BEGIN;"+gapSQL(b, gap, event, 8, 10)+attemptSQL(gap, root, "", "requested", 8, 9, 7)+"COMMIT;")
		s.reject(t, s.runtime, attemptSQL(gap, uuid.NewString(), "", "requested", 8, 9, 7), "duplicate key")
		s.reject(t, s.runtime, attemptSQL(gap, uuid.NewString(), root, "resolved", 8, 9, 7), "authoritative checkpoint")
		held := uuid.NewString()
		s.must(t, s.runtime, attemptSQL(gap, held, root, "held", 8, 9, 7))
		s.reject(t, s.runtime, attemptSQL(gap, uuid.NewString(), root, "requested", 8, 9, 7), "duplicate key")
		s.must(t, s.runtime, "BEGIN;"+checkpointAdvance(b, uuid.NewString(), 8)+checkpointAdvance(b, uuid.NewString(), 9)+"COMMIT;")
		s.reject(t, s.runtime, attemptSQL(gap, uuid.NewString(), held, "held", 8, 9, 9), "authoritative checkpoint")
		resolved := uuid.NewString()
		s.must(t, s.runtime, attemptSQL(gap, resolved, held, "resolved", 8, 9, 9))
		s.reject(t, s.runtime, attemptSQL(gap, uuid.NewString(), resolved, "requested", 8, 9, 9), "parent or progress")
		otherGap := uuid.NewString()
		s.must(t, s.runtime, gapSQL(b, otherGap, uuid.NewString(), 10, 12))
		s.reject(t, s.runtime, attemptSQL(otherGap, uuid.NewString(), held, "requested", 10, 11, 9), "parent or progress")
		for _, table := range []string{"consumer_bootstraps", "consumer_gaps", "consumer_gap_attempts", "projection_checkpoint_compatibility", "consumer_checkpoints"} {
			for _, q := range []string{"DELETE FROM eventstore." + table, "TRUNCATE eventstore." + table} {
				s.reject(t, s.runtime, q, "permission denied")
			}
		}
	})
	t.Run("wrong lineage runtime install and excessive direct column reachable default privileges", func(t *testing.T) {
		for _, args := range [][]string{{"-v", "base_schema_sha256=" + strings.Repeat("0", 64)}, {"-v", "prior_migration_sha256=" + strings.Repeat("0", 64)}, {"-v", "correction_migration_sha256=" + strings.Repeat("0", 64)}, {"-v", "checkpoint_migration_sha256=bad"}, {"-v", "owner_service=retail"}, {"-v", "backup_ref="}} {
			s := checkpointStore(t, f)
			checkpointRejectInstall(t, s, args...)
		}
		for _, grant := range []string{"GRANT UPDATE ON eventstore.inbox TO %s", "GRANT REFERENCES(event_id) ON eventstore.inbox TO %s", "GRANT INSERT(owner_service) ON eventstore.messaging_route_compatibility TO %s", "GRANT MAINTAIN ON eventstore.events TO %s", "GRANT SELECT ON eventstore.inbox TO PUBLIC", "GRANT UPDATE(event_id) ON eventstore.inbox TO PUBLIC", "GRANT SELECT ON eventstore.inbox TO %s WITH GRANT OPTION", "GRANT USAGE ON SCHEMA eventstore TO PUBLIC", "ALTER DEFAULT PRIVILEGES IN SCHEMA eventstore GRANT INSERT ON TABLES TO %s", "ALTER DEFAULT PRIVILEGES GRANT SELECT ON TABLES TO PUBLIC", "ALTER DEFAULT PRIVILEGES GRANT EXECUTE ON FUNCTIONS TO %s", "REVOKE SELECT ON eventstore.inbox FROM %s"} {
			s := checkpointStore(t, f)
			q := grant
			if strings.Contains(q, "%s") {
				q = fmt.Sprintf(q, s.runtime)
			}
			s.must(t, s.migration, q)
			checkpointRejectInstall(t, s)
		}
		s := checkpointStore(t, f)
		role := "bridge_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		middle := "middle_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		s.must(t, "postgres", "CREATE ROLE "+role+" NOINHERIT; CREATE ROLE "+middle+" NOINHERIT; ALTER ROLE "+s.runtime+" NOINHERIT; GRANT "+role+" TO "+middle+"; GRANT "+middle+" TO "+s.runtime)
		s.must(t, s.migration, "GRANT UPDATE(event_id) ON eventstore.inbox TO "+role)
		checkpointRejectInstall(t, s)
		s.must(t, s.migration, "REVOKE UPDATE(event_id) ON eventstore.inbox FROM "+role+"; ALTER DEFAULT PRIVILEGES IN SCHEMA eventstore GRANT INSERT ON TABLES TO "+role)
		checkpointRejectInstall(t, s)
		for _, badShape := range []string{"CREATE TABLE eventstore.projection_checkpoint_compatibility(wrong int)", "ALTER FUNCTION eventstore.messaging_immutable() SECURITY DEFINER", "ALTER TABLE eventstore.messaging_route_compatibility DISABLE TRIGGER immutable_content; UPDATE eventstore.messaging_route_compatibility SET correction_migration_sha256=decode(repeat('00',32),'hex'); ALTER TABLE eventstore.messaging_route_compatibility ENABLE TRIGGER immutable_content"} {
			broken := checkpointStore(t, f)
			broken.must(t, broken.migration, badShape)
			checkpointRejectInstall(t, broken)
		}
		s = checkpointStore(t, f)
		script := checkpointScript(t)
		out, err := s.installAs(s.runtime, script, "-v", "base_schema_sha256="+checkpointBaseHash, "-v", fmt.Sprintf("checkpoint_migration_sha256=%x", sha256.Sum256([]byte(script))))
		if err == nil || !strings.Contains(out, "migration owner required") {
			t.Fatalf("runtime install: %v %s", err, out)
		}
	})
	t.Run("populated custody remains intact and creates no inferred checkpoint", func(t *testing.T) {
		s := checkpointStore(t, f)
		records := s.produce(t, events.OwnerInventory)
		record := records[0]
		sourceAdmission, localAdmission := uuid.NewString(), uuid.NewString()
		s.must(t, s.runtime, routeAdmission(sourceAdmission, record.Stream, "source-recipient", "inventory", "inventory.fixture.changed.v1"))
		s.must(t, s.migration, fmt.Sprintf("INSERT INTO eventstore.messaging_legacy_authorizations VALUES('%s','inventory','%s',NULL,'synthetic-self-review')", record.ID, sourceAdmission))
		s.cutover(t)
		s.must(t, s.runtime, "BEGIN;"+routeAdmission(localAdmission, record.Stream, "local-consumer", "retained-consumer", "inventory.fixture.changed.v1")+routeCustody(record, localAdmission, "retained-consumer", "generation-1")+routeComplete(record, "retained-consumer", record.Hash)+"COMMIT;")
		before := s.snapshot(t)
		marker := s.must(t, s.runtime, "SELECT row_to_json(t) FROM eventstore.messaging_route_compatibility t")
		checkpointSaveEvidence(t, s, "checkpoint-before-custody")
		mustCheckpoint(t, s)
		if s.snapshot(t) != before || s.must(t, s.runtime, "SELECT row_to_json(t) FROM eventstore.messaging_route_compatibility t") != marker {
			t.Fatal("retained custody changed")
		}
		if got := s.must(t, s.runtime, "SELECT (SELECT count(*) FROM eventstore.consumer_bootstraps)+(SELECT count(*) FROM eventstore.consumer_checkpoints)+(SELECT count(*) FROM eventstore.consumer_gaps)+(SELECT count(*) FROM eventstore.consumer_gap_attempts)"); got != "0" {
			t.Fatal("inferred checkpoint/history", got)
		}
		checkpointSaveEvidence(t, s, "checkpoint-after-custody")
	})
	t.Run("snapshot bootstrap failure panic cancellation and request conflict are atomic", func(t *testing.T) {
		s := checkpointStore(t, f)
		mustCheckpoint(t, s)
		s.must(t, s.migration, "CREATE TABLE public.synthetic_checkpoint_snapshot(bootstrap_id uuid PRIMARY KEY,value text NOT NULL);GRANT SELECT,INSERT ON public.synthetic_checkpoint_snapshot TO "+s.runtime)
		write := func(tx *gorm.DB, b checkpointIdentity) error {
			return tx.Exec(bootstrapSQL(b, "", "projection") + checkpointSQL(b) + fmt.Sprintf("INSERT INTO public.synthetic_checkpoint_snapshot VALUES('%s','synthetic-authorized-snapshot')", b.bootstrap)).Error
		}
		assertAbsent := func(b checkpointIdentity) {
			t.Helper()
			if got := s.must(t, s.runtime, "SELECT (SELECT count(*) FROM eventstore.consumer_bootstraps WHERE bootstrap_id='"+b.bootstrap+"')+(SELECT count(*) FROM eventstore.consumer_checkpoints WHERE "+checkpointWhere(b)+")+(SELECT count(*) FROM public.synthetic_checkpoint_snapshot WHERE bootstrap_id='"+b.bootstrap+"')"); got != "0" {
				t.Fatal("partial snapshot/bootstrap", got)
			}
		}
		b := checkpointID(12)
		runner, err := eventstore.NewTransactions(s.db, func(tx *gorm.DB) (*gorm.DB, error) {
			if err := write(tx, b); err != nil {
				return nil, err
			}
			return nil, errors.New("synthetic factory refusal")
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := runner.Run(context.Background(), func(*gorm.DB) error { return nil }); err == nil {
			t.Fatal("factory succeeded")
		}
		assertAbsent(b)
		runner, err = eventstore.NewTransactions(s.db, func(tx *gorm.DB) (*gorm.DB, error) { return tx, nil })
		if err != nil {
			t.Fatal(err)
		}
		b = checkpointID(13)
		func() {
			defer func() {
				if recover() != "synthetic panic" {
					t.Error("panic not preserved")
				}
			}()
			_ = runner.Run(context.Background(), func(tx *gorm.DB) error {
				if err := write(tx, b); err != nil {
					return err
				}
				panic("synthetic panic")
			})
		}()
		assertAbsent(b)
		b = checkpointID(14)
		ctx, cancel := context.WithCancel(context.Background())
		err = runner.Run(ctx, func(tx *gorm.DB) error { err := write(tx, b); cancel(); return err })
		if err == nil {
			t.Fatal("cancelled install succeeded")
		}
		assertAbsent(b)
		b = checkpointID(15)
		if err := runner.Run(context.Background(), func(tx *gorm.DB) error { return write(tx, b) }); err != nil {
			t.Fatal(err)
		}
		conflict := checkpointID(2)
		conflict.request = b.request
		s.reject(t, s.runtime, bootstrapSQL(conflict, "", "projection"), "duplicate key")
		assertAbsent(conflict)
	})
	t.Run("concurrent gap attempts cannot fork retained chain", func(t *testing.T) {
		s := checkpointStore(t, f)
		mustCheckpoint(t, s)
		b := checkpointID(2)
		gap, root := uuid.NewString(), uuid.NewString()
		s.must(t, s.runtime, "BEGIN;"+bootstrapSQL(b, "", "projection")+checkpointSQL(b)+gapSQL(b, gap, uuid.NewString(), 3, 5)+attemptSQL(gap, root, "", "requested", 3, 4, 2)+"COMMIT;")
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- s.db.Exec(attemptSQL(gap, uuid.NewString(), root, "held", 3, 4, 2)).Error
			}()
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
			t.Fatalf("chain winners %d", wins)
		}
		if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.consumer_gap_attempts WHERE gap_id='"+gap+"'"); got != "2" {
			t.Fatal(got)
		}
	})
	t.Run("concurrent installers one winner and bounded active writer rollback", func(t *testing.T) {
		s := checkpointStore(t, f)
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := installCheckpoint(t, s); results <- err }()
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
			t.Fatalf("winners %d", wins)
		}
		s = checkpointStore(t, f)
		tx := s.db.Begin()
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		defer tx.Rollback()
		if err := tx.Exec("LOCK TABLE eventstore.outbox IN ROW EXCLUSIVE MODE").Error; err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		checkpointRejectInstall(t, s)
		if time.Since(start) > 9*time.Second {
			t.Fatal("lock wait not bounded")
		}
		tx.Rollback()
		mustCheckpoint(t, s)
	})
	t.Run("concurrent checkpoint CAS and real commit reply ambiguity", func(t *testing.T) {
		s := checkpointStore(t, f)
		mustCheckpoint(t, s)
		b := checkpointID(7)
		s.must(t, s.runtime, "BEGIN;"+bootstrapSQL(b, "", "projection")+checkpointSQL(b)+"COMMIT;")
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tx := s.db.Begin()
				defer tx.Rollback()
				err := tx.Exec(checkpointAdvance(b, uuid.NewString(), 8)).Error
				if err == nil {
					err = tx.Commit().Error
				}
				results <- err
			}()
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
			t.Fatalf("winners %d", wins)
		}
		for _, commit := range []bool{false, true} {
			b := checkpointID(5)
			pool, err := s.db.DB()
			if err != nil {
				t.Fatal(err)
			}
			fault, err := gorm.Open(postgres.New(postgres.Config{Conn: commitFaultPool{DB: pool, commitFirst: commit}}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			runner, err := eventstore.NewTransactions(fault, func(tx *gorm.DB) (*gorm.DB, error) { return tx, nil })
			if err != nil {
				t.Fatal(err)
			}
			err = runner.Run(context.Background(), func(tx *gorm.DB) error { return tx.Exec(bootstrapSQL(b, "", "projection") + checkpointSQL(b)).Error })
			if !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) {
				t.Fatalf("lost reply %v", err)
			}
			want := "0"
			if commit {
				want = "1"
			}
			if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.consumer_checkpoints WHERE "+checkpointWhere(b)); got != want {
				t.Fatalf("got %s want %s", got, want)
			}
		}
	})
}
