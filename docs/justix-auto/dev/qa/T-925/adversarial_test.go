package eventstore_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"justixauto/pkg/events"
)

// Independent assertions reuse only the committed fixture's isolated-container
// plumbing. The overlay adds this test without editing the reviewed source.
func TestQAT925Adversarial(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_MESSAGING_ROUTE_MIGRATION") != "1" {
		t.Skip("opt-in isolated PostgreSQL fixture")
	}
	f := newRouteFixture(t)
	t.Run("all seven actual producer destinations and cross-owner isolation", func(t *testing.T) {
		source := f.store(t, "inventory", true)
		owners := []events.Owner{events.OwnerIdentity, events.OwnerInventory, events.OwnerCommerce, events.OwnerRetail, events.OwnerFinancing, events.OwnerInsurance, events.OwnerDocuments}
		rows := source.produce(t, owners...)
		for i, owner := range owners {
			t.Run(string(owner), func(t *testing.T) {
				s := f.store(t, string(owner), true)
				s.correct(t)
				s.cutover(t)
				r := rows[i]
				a, consumer := uuid.NewString(), "qa.route-projection"
				s.must(t, s.runtime, routeAdmission(a, r.Stream, "local-consumer", consumer, "inventory.fixture.changed.v1"))
				before := s.snapshot(t)
				wrong := rows[(i+1)%len(rows)]
				s.reject(t, s.runtime, "BEGIN;"+routeCustody(wrong, a, consumer, "generation-1")+"COMMIT;", "invalid custody destination route")
				if s.snapshot(t) != before { t.Fatal("foreign destination leaked custody") }
				s.must(t, s.runtime, "BEGIN;"+routeCustody(r, a, consumer, "generation-1")+"COMMIT;")
				before = s.snapshot(t)
				s.reject(t, s.runtime, "BEGIN;"+routeCustody(r, a, consumer, "generation-1")+"COMMIT;", "duplicate key")
				conflict := r
				conflict.Body = []byte("different synthetic bytes")
				h := sha256.Sum256(conflict.Body)
				conflict.Hash = h[:]
				s.reject(t, s.runtime, "BEGIN;"+routeCustody(conflict, a, consumer, "generation-1")+"COMMIT;", "duplicate key")
				if s.snapshot(t) != before { t.Fatal("duplicate/conflicting event changed custody") }
				s.must(t, s.runtime, "BEGIN;"+routeComplete(r, consumer, r.Hash)+"COMMIT;")
				before = s.snapshot(t)
				s.reject(t, s.runtime, "BEGIN;"+routeComplete(r, consumer, r.Hash)+"COMMIT;", "duplicate key")
				if s.snapshot(t) != before { t.Fatal("duplicate completion changed committed state") }
				if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.dispatch_jobs WHERE completed_at IS NOT NULL"); got != "1" { t.Fatal(got) }
			})
		}
	})
	t.Run("two-hop noinherit and public column default grant audits", func(t *testing.T) {
		s := f.store(t, "documents", true)
		a := "qa_a_"+strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
		b := "qa_b_"+strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
		s.must(t, "postgres", "CREATE ROLE "+a+"; CREATE ROLE "+b+"; GRANT "+a+" TO "+s.runtime+" WITH INHERIT FALSE; GRANT "+b+" TO "+a+" WITH INHERIT FALSE")
		probes := []struct{name, grant, revoke, want string}{
			{"reachable references", "GRANT REFERENCES(event_id) ON eventstore.dispatch_messages TO "+b, "REVOKE REFERENCES(event_id) ON eventstore.dispatch_messages FROM "+b, "column privileges"},
			{"reachable database ddl", "GRANT CREATE ON DATABASE "+s.database+" TO "+b, "REVOKE CREATE ON DATABASE "+s.database+" FROM "+b, "schema CREATE"},
			{"public schema ddl", "GRANT CREATE ON SCHEMA eventstore TO PUBLIC", "REVOKE CREATE ON SCHEMA eventstore FROM PUBLIC", "schema CREATE"},
			{"public column read", "GRANT SELECT(event_id) ON eventstore.dispatch_messages TO PUBLIC", "REVOKE SELECT(event_id) ON eventstore.dispatch_messages FROM PUBLIC", "PUBLIC or delegable"},
			{"delegable work column", "GRANT UPDATE(attempts) ON eventstore.dispatch_jobs TO "+s.runtime+" WITH GRANT OPTION", "REVOKE GRANT OPTION FOR UPDATE(attempts) ON eventstore.dispatch_jobs FROM "+s.runtime, "PUBLIC or delegable"},
			{"reachable default grant option", "ALTER DEFAULT PRIVILEGES IN SCHEMA eventstore GRANT SELECT ON TABLES TO "+b+" WITH GRANT OPTION", "ALTER DEFAULT PRIVILEGES IN SCHEMA eventstore REVOKE SELECT ON TABLES FROM "+b, "default privileges"},
			{"reachable migration evidence column", "GRANT INSERT(owner_service) ON eventstore.messaging_mode TO "+b, "REVOKE INSERT(owner_service) ON eventstore.messaging_mode FROM "+b, "evidence INSERT"},
		}
		for _, p := range probes {
			t.Run(p.name, func(t *testing.T) {
				s.must(t, s.migration, p.grant)
				s.rejectCorrection(t, p.want)
				s.must(t, s.migration, p.revoke)
			})
		}
		s.correct(t)
	})
	t.Run("empty evidence and incompatible function properties abort", func(t *testing.T) {
		s := f.store(t, "insurance", true)
		for _, key := range []string{"stopped_runtimes_ref", "compatibility_ref"} {
			s.rejectCorrection(t, "check constraint", "-v", key+"= ")
		}
		for _, p := range []struct{change, restore string}{
			{"SECURITY DEFINER", "SECURITY INVOKER"},
			{"SET search_path=pg_catalog,public", "SET search_path=pg_catalog"},
		} {
			s.must(t, s.migration, "ALTER FUNCTION eventstore.messaging_job_guard() "+p.change)
			s.rejectCorrection(t, "incompatible route function prerequisite")
			s.must(t, s.migration, "ALTER FUNCTION eventstore.messaging_job_guard() "+p.restore)
		}
		s.correct(t)
		want := fmt.Sprintf("insurance:%s:2:3:1:true:true", s.runtime)
		got := s.must(t, s.runtime, "SELECT owner_service||':'||runtime_role||':'||base_schema_version||':'||migration_revision||':'||route_format_version||':'||isfinite(installed_at)||':'||(octet_length(prior_migration_sha256)=32 AND octet_length(correction_migration_sha256)=32) FROM eventstore.messaging_route_compatibility")
		if got != want { t.Fatalf("marker %s", got) }
	})
}
