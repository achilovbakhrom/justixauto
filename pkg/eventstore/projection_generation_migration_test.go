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
	"justixauto/pkg/eventstore"
)

const generationQuarantineHash = "1c1e7d17efa8ff84f2469d6d55b3b6c810143e5260aabf5f82d748797f2296a3"

// Reuse T927's reviewed fixture lifecycle verbatim: fresh UUID name and separate
// UUID label, cleanup registered before allocation, restricted inspect, exact
// immutable-ID removal and verified absence. Its prefix is historical; each
// invocation owns its unique tmpfs fixture. No existing database or secrets.
func generationStore(t *testing.T, f *routeFixture) *routeStore {
	t.Helper()
	s := quarantineStore(t, f)
	mustQuarantine(t, s)
	// Explicit installer prerequisite, not a change by the migration: PostgreSQL
	// otherwise gives PUBLIC EXECUTE/USAGE via implicit global default ACLs.
	s.must(t, s.migration, "ALTER DEFAULT PRIVILEGES REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC; ALTER DEFAULT PRIVILEGES REVOKE USAGE ON TYPES FROM PUBLIC")
	return s
}
func generationScript(t *testing.T) string {
	t.Helper()
	b, e := os.ReadFile("migrations/000006_projection_generation.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func installGenerationScript(t *testing.T, s *routeStore, script string, overrides ...string) (string, error) {
	t.Helper()
	args := []string{"-v", "base_schema_sha256=" + checkpointBaseHash, "-v", "checkpoint_migration_sha256=" + quarantineCheckpointHash, "-v", "quarantine_migration_sha256=" + generationQuarantineHash, "-v", fmt.Sprintf("generation_migration_sha256=%x", sha256.Sum256([]byte(script)))}
	return s.install(script, append(args, overrides...)...)
}
func installGeneration(t *testing.T, s *routeStore, overrides ...string) (string, error) {
	t.Helper()
	return installGenerationScript(t, s, generationScript(t), overrides...)
}
func mustGeneration(t *testing.T, s *routeStore) {
	t.Helper()
	if out, e := installGeneration(t, s); e != nil {
		t.Fatalf("000006: %v %s", e, out)
	}
}
func generationPreimage(t *testing.T, s *routeStore) string {
	t.Helper()
	body := quarantinePreimage(t, s)
	for _, tab := range []string{"quarantine_compatibility", "quarantine_evidence", "quarantine_actions"} {
		body += "\n" + s.must(t, s.migration, "SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]') FROM eventstore."+tab+" t")
	}
	return body
}
func generationConstraints(t *testing.T, s *routeStore) string {
	return s.must(t, s.migration, `SELECT jsonb_object_agg(c.oid::text,jsonb_build_object('name',c.conname,'table',c.conrelid,'definition',pg_get_constraintdef(c.oid),'validated',c.convalidated,'enforced',c.conenforced)) FROM pg_constraint c JOIN pg_class r ON r.oid=c.conrelid WHERE r.relnamespace='eventstore'::regnamespace AND c.conname<>'consumer_bootstraps_generation_check'`)
}
func generationLabel(t *testing.T, s *routeStore) string {
	return s.must(t, s.migration, `SELECT conname||'|'||pg_get_expr(conbin,conrelid)||'|'||convalidated||'|'||conenforced FROM pg_constraint WHERE conrelid='eventstore.consumer_bootstraps'::regclass AND conname='consumer_bootstraps_generation_check'`)
}
func generationSave(t *testing.T, s *routeStore, label string) {
	t.Helper()
	for name, body := range map[string]string{"rows": generationPreimage(t, s), "catalog": s.catalog(t), "functions": s.functions(t, true), "function-acls": s.functions(t, false), "constraints": generationConstraints(t, s), "label": generationLabel(t, s), "defaults": s.must(t, s.migration, "SELECT coalesce(jsonb_agg(to_jsonb(d) ORDER BY oid),'[]') FROM pg_default_acl d WHERE defaclrole=current_user::regrole::oid")} {
		p := filepath.Join(s.f.evidence, s.database+"-generation-"+label+"-"+name+".txt")
		b := []byte(body + "\n")
		if e := os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p+".sha256", []byte(fmt.Sprintf("%x\n", sha256.Sum256(b))), 0600); e != nil {
			t.Fatal(e)
		}
	}
}
func generationSubset(t *testing.T, before, after string) {
	t.Helper()
	var a, b map[string]json.RawMessage
	if e := json.Unmarshal([]byte(before), &a); e != nil {
		t.Fatal(e)
	}
	if e := json.Unmarshal([]byte(after), &b); e != nil {
		t.Fatal(e)
	}
	for k, v := range a {
		if string(v) != string(b[k]) {
			t.Fatalf("retained catalog entry changed %s", k)
		}
	}
}
func generationReject(t *testing.T, s *routeStore, script string, overrides ...string) {
	t.Helper()
	before, cat, cons, label, fn, acl := generationPreimage(t, s), s.catalog(t), generationConstraints(t, s), generationLabel(t, s), s.functions(t, true), s.functions(t, false)
	if out, e := installGenerationScript(t, s, script, overrides...); e == nil {
		t.Fatal("invalid install succeeded", out)
	}
	if before != generationPreimage(t, s) || cat != s.catalog(t) || cons != generationConstraints(t, s) || label != generationLabel(t, s) || fn != s.functions(t, true) || acl != s.functions(t, false) {
		t.Fatal("failed install mutated retained state")
	}
	if got := s.must(t, s.migration, "SELECT count(*) FROM pg_class WHERE relnamespace='eventstore'::regnamespace AND relname IN ('projection_generations','projection_generation_events','projection_heads','projection_generation_compatibility')"); got != "0" {
		t.Fatal("partial generation storage", got)
	}
}

type generationIdentity struct{ projection, generation, consumer, request string }

func generationID() generationIdentity {
	return generationIdentity{"fixture-" + uuid.NewString(), uuid.NewString(), "consumer-" + uuid.NewString(), uuid.NewString()}
}
func generationSQL(g generationIdentity) string {
	return fmt.Sprintf(`INSERT INTO eventstore.projection_generations(projection_name,generation_id,consumer_name,consumer_kind,handler_digest,contract_id,contract_version,contract_digest,evidence_format_version,authority_ref,scope_ref,purpose_ref,history_manifest_ref,history_manifest_sha256,bootstrap_manifest_ref,bootstrap_manifest_sha256,creation_request_id)
 VALUES('%s','%s','%s','projection',decode(repeat('11',32),'hex'),'fixture-contract',1,decode(repeat('22',32),'hex'),1,'synthetic-authority','synthetic-scope','synthetic-purpose','synthetic-history',decode(repeat('33',32),'hex'),'synthetic-bootstrap',decode(repeat('44',32),'hex'),'%s');`, g.projection, g.generation, g.consumer, g.request)
}
func generationEventSQL(g generationIdentity, id, previous, action string, epoch int64, active string) string {
	prev, head, expected := "NULL", "NULL", "NULL"
	if previous != "" {
		prev = routeQuote(previous)
	}
	if epoch > 0 {
		head = fmt.Sprint(epoch)
	}
	if active != "" {
		expected = routeQuote(active)
	}
	comparator := "NULL,NULL,NULL,NULL,NULL"
	if action == "validated" || action == "switch" {
		comparator = "'synthetic-comparator',1,'passed','synthetic-comparison',decode(repeat('77',32),'hex')"
	}
	hold := "NULL"
	if action == "hold" || action == "abandon" {
		hold = "'synthetic-hold'"
	}
	return fmt.Sprintf(`INSERT INTO eventstore.projection_generation_events(event_id,projection_name,generation_id,request_id,previous_event_id,action,evidence_format_version,source_vector_ref,source_vector_sha256,cursor_manifest_ref,cursor_manifest_sha256,comparator_id,comparator_version,comparison_result,comparison_ref,comparison_sha256,expected_head_epoch,expected_active_generation_id,hold_ref)
 VALUES('%s','%s','%s','%s',%s,'%s',1,'synthetic-finite-vector',decode(repeat('55',32),'hex'),'synthetic-history-cursor',decode(repeat('66',32),'hex'),%s,%s,%s,%s);`, id, g.projection, g.generation, id, prev, action, comparator, head, expected, hold)
}
func generationPrepare(t *testing.T, s *routeStore, g generationIdentity) string {
	t.Helper()
	build, validated := uuid.NewString(), uuid.NewString()
	s.must(t, s.runtime, generationSQL(g)+generationEventSQL(g, build, "", "build-progress", 0, "")+generationEventSQL(g, validated, build, "validated", 0, ""))
	return validated
}
func generationHeadSQL(g generationIdentity) string {
	return "INSERT INTO eventstore.projection_heads(projection_name,epoch) VALUES('" + g.projection + "',1);"
}
func generationSwitchSQL(g generationIdentity, event, previous string, epoch int64, active string) string {
	return generationEventSQL(g, event, previous, "switch", epoch, active) + fmt.Sprintf("UPDATE eventstore.projection_heads SET active_generation_id='%s',epoch=%d,last_event_id='%s',last_switch_event_id='%s',hold_ref=NULL,updated_at=clock_timestamp() WHERE projection_name='%s' AND epoch=%d;", g.generation, epoch+1, event, event, g.projection, epoch)
}

func TestProjectionGenerationFailurePostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_PROJECTION_GENERATION") != "1" {
		t.Skip("explicit owned PostgreSQL fixture opt-in required")
	}
	f := newQuarantineFixture(t)
	t.Run("strict prerequisite tampering and effective default escalation", func(t *testing.T) {
		for name, change := range map[string]func(*routeStore) string{
			"disabled bootstrap guard": func(s *routeStore) string {
				return "ALTER TABLE eventstore.consumer_bootstraps DISABLE TRIGGER bootstrap_guard"
			},
			"conditional bootstrap guard": func(s *routeStore) string {
				return "DROP TRIGGER bootstrap_guard ON eventstore.consumer_bootstraps; CREATE TRIGGER bootstrap_guard BEFORE INSERT ON eventstore.consumer_bootstraps FOR EACH ROW WHEN(false) EXECUTE FUNCTION eventstore.consumer_bootstrap_guard()"
			},
			"disabled checkpoint guard": func(s *routeStore) string {
				return "ALTER TABLE eventstore.consumer_checkpoints DISABLE TRIGGER checkpoint_guard"
			},
			"wrong installed checkpoint digest": func(s *routeStore) string {
				return "ALTER TABLE eventstore.projection_checkpoint_compatibility DISABLE TRIGGER immutable_content; UPDATE eventstore.projection_checkpoint_compatibility SET checkpoint_migration_sha256=decode(repeat('00',32),'hex'); ALTER TABLE eventstore.projection_checkpoint_compatibility ENABLE TRIGGER immutable_content"
			},
			"missing check": func(s *routeStore) string {
				return "ALTER TABLE eventstore.consumer_bootstraps DROP CONSTRAINT consumer_bootstraps_generation_check"
			},
			"renamed check": func(s *routeStore) string {
				return "ALTER TABLE eventstore.consumer_bootstraps RENAME CONSTRAINT consumer_bootstraps_generation_check TO wrong_name"
			},
			"duplicate check": func(s *routeStore) string {
				return "ALTER TABLE eventstore.consumer_bootstraps ADD CONSTRAINT extra_generation CHECK(length(generation)>0)"
			},
			"altered check": func(s *routeStore) string {
				return "ALTER TABLE eventstore.consumer_bootstraps DROP CONSTRAINT consumer_bootstraps_generation_check; ALTER TABLE eventstore.consumer_bootstraps ADD CONSTRAINT consumer_bootstraps_generation_check CHECK(length(generation)>0)"
			},
			"unvalidated check": func(s *routeStore) string {
				return "ALTER TABLE eventstore.consumer_bootstraps DROP CONSTRAINT consumer_bootstraps_generation_check; ALTER TABLE eventstore.consumer_bootstraps ADD CONSTRAINT consumer_bootstraps_generation_check CHECK(length(btrim(generation))>0) NOT VALID"
			},
			"nullable generation": func(s *routeStore) string {
				return "ALTER TABLE eventstore.consumer_bootstraps ALTER COLUMN generation DROP NOT NULL"
			},
			"table update": func(s *routeStore) string { return "GRANT UPDATE ON eventstore.quarantine_evidence TO " + s.runtime },
			"column references": func(s *routeStore) string {
				return "GRANT REFERENCES(raw_sha256) ON eventstore.quarantine_evidence TO " + s.runtime
			},
			"column marker insert": func(s *routeStore) string {
				return "GRANT INSERT(singleton) ON eventstore.quarantine_compatibility TO " + s.runtime
			},
			"PUBLIC column": func(s *routeStore) string {
				return "GRANT SELECT(raw_sha256) ON eventstore.quarantine_evidence TO PUBLIC"
			},
			"delegable select": func(s *routeStore) string {
				return "GRANT SELECT ON eventstore.quarantine_evidence TO " + s.runtime + " WITH GRANT OPTION"
			},
			"missing select": func(s *routeStore) string { return "REVOKE SELECT ON eventstore.quarantine_evidence FROM " + s.runtime },
			"missing insert": func(s *routeStore) string { return "REVOKE INSERT ON eventstore.consumer_gaps FROM " + s.runtime },
			"database temp":  func(s *routeStore) string { return "GRANT TEMP ON DATABASE " + s.database + " TO " + s.runtime },
			"schema create":  func(s *routeStore) string { return "GRANT CREATE ON SCHEMA eventstore TO " + s.runtime },
			"function execute": func(s *routeStore) string {
				return "GRANT EXECUTE ON FUNCTION eventstore.consumer_bootstrap_guard() TO " + s.runtime
			},
			"implicit PUBLIC functions": func(s *routeStore) string { return "ALTER DEFAULT PRIVILEGES GRANT EXECUTE ON FUNCTIONS TO PUBLIC" },
			"implicit PUBLIC types":     func(s *routeStore) string { return "ALTER DEFAULT PRIVILEGES GRANT USAGE ON TYPES TO PUBLIC" },
			"schema default insert": func(s *routeStore) string {
				return "ALTER DEFAULT PRIVILEGES IN SCHEMA eventstore GRANT INSERT ON TABLES TO " + s.runtime
			},
			"global default select": func(s *routeStore) string { return "ALTER DEFAULT PRIVILEGES GRANT SELECT ON TABLES TO " + s.runtime },
			"default sequence":      func(s *routeStore) string { return "ALTER DEFAULT PRIVILEGES GRANT USAGE ON SEQUENCES TO " + s.runtime },
			"RLS enabled": func(s *routeStore) string {
				return "ALTER TABLE eventstore.consumer_bootstraps ENABLE ROW LEVEL SECURITY"
			},
		} {
			t.Run(name, func(t *testing.T) {
				s := generationStore(t, f)
				s.must(t, s.migration, change(s))
				generationReject(t, s, generationScript(t))
			})
		}
		for name, override := range map[string]string{"owner": "owner_service=retail", "runtime": "runtime_role=postgres", "base": "base_schema_sha256=" + strings.Repeat("0", 64), "prior": "prior_migration_sha256=" + strings.Repeat("0", 64), "route": "correction_migration_sha256=" + strings.Repeat("0", 64), "checkpoint": "checkpoint_migration_sha256=" + strings.Repeat("0", 64), "quarantine": "quarantine_migration_sha256=" + strings.Repeat("0", 64), "self": "generation_migration_sha256=invalid", "backup": "backup_ref="} {
			t.Run(name, func(t *testing.T) {
				s := generationStore(t, f)
				generationReject(t, s, generationScript(t), "-v", override)
			})
		}
		s := generationStore(t, f)
		a, b := "hop_"+strings.ReplaceAll(uuid.NewString(), "-", "")[:10], "hop_"+strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
		s.must(t, "postgres", "CREATE ROLE "+a+" NOINHERIT; CREATE ROLE "+b+" NOINHERIT; GRANT "+b+" TO "+a+"; GRANT "+a+" TO "+s.runtime+" WITH INHERIT FALSE; GRANT UPDATE ON eventstore.quarantine_evidence TO "+b)
		generationReject(t, s, generationScript(t))
	})
	t.Run("late failure rollback bounded locks and concurrent install", func(t *testing.T) {
		s := generationStore(t, f)
		generationSave(t, s, "late-failure-before")
		generationReject(t, s, strings.Replace(generationScript(t), "COMMIT;", "SELECT 1/0; COMMIT;", 1))
		generationSave(t, s, "late-failure-after")
		tx := s.db.Begin()
		defer tx.Rollback()
		if e := tx.Exec("LOCK TABLE eventstore.consumer_bootstraps IN ROW EXCLUSIVE MODE").Error; e != nil {
			t.Fatal(e)
		}
		started := time.Now()
		generationReject(t, s, generationScript(t))
		if elapsed := time.Since(started); elapsed < 4*time.Second || elapsed > 10*time.Second {
			t.Fatal("unexpected lock bound", elapsed)
		}
		tx.Rollback()
		var wg sync.WaitGroup
		outcomes := make(chan error, 2)
		for range 2 {
			wg.Add(1)
			go func() { defer wg.Done(); _, e := installGeneration(t, s); outcomes <- e }()
		}
		wg.Wait()
		close(outcomes)
		wins := 0
		for e := range outcomes {
			if e == nil {
				wins++
			}
		}
		if wins != 1 {
			t.Fatal("installer winners", wins)
		}
	})
	t.Run("switch contention complete rollback panic cancellation and unknown outcomes", func(t *testing.T) {
		s := generationStore(t, f)
		mustGeneration(t, s)
		base := generationID()
		s.must(t, s.runtime, generationHeadSQL(base))
		first, second := generationID(), generationID()
		first.projection, second.projection = base.projection, base.projection
		p1, p2 := generationPrepare(t, s, first), generationPrepare(t, s, second)
		ch := make(chan error, 2)
		var wg sync.WaitGroup
		for _, v := range []struct {
			g generationIdentity
			p string
		}{{first, p1}, {second, p2}} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tx := s.db.Begin()
				if tx.Error != nil {
					ch <- tx.Error
					return
				}
				defer tx.Rollback()
				if e := tx.Exec(generationSwitchSQL(v.g, uuid.NewString(), v.p, 1, "")).Error; e != nil {
					ch <- e
					return
				}
				ch <- tx.Commit().Error
			}()
		}
		wg.Wait()
		close(ch)
		wins := 0
		for e := range ch {
			if e == nil {
				wins++
			}
		}
		if wins != 1 {
			t.Fatal("switch CAS winners", wins)
		}
		if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.projection_generation_events WHERE action='switch'"); got != "1" {
			t.Fatal("losing switch left evidence", got)
		}
		runner, e := eventstore.NewTransactions(s.db, func(tx *gorm.DB) (*gorm.DB, error) { return tx, nil })
		if e != nil {
			t.Fatal(e)
		}
		for _, mode := range []string{"error", "panic", "cancel", "lost-rollback", "lost-commit"} {
			t.Run(mode, func(t *testing.T) {
				g := generationID()
				p := generationPrepare(t, s, g)
				s.must(t, s.runtime, generationHeadSQL(g))
				event := uuid.NewString()
				q := generationSwitchSQL(g, event, p, 1, "")
				r := runner
				if strings.HasPrefix(mode, "lost-") {
					pool, e := s.db.DB()
					if e != nil {
						t.Fatal(e)
					}
					db, e := gorm.Open(postgres.New(postgres.Config{Conn: commitFaultPool{DB: pool, commitFirst: mode == "lost-commit"}}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
					if e != nil {
						t.Fatal(e)
					}
					r, e = eventstore.NewTransactions(db, func(tx *gorm.DB) (*gorm.DB, error) { return tx, nil })
					if e != nil {
						t.Fatal(e)
					}
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				calls := 0
				run := func() error {
					return r.Run(ctx, func(tx *gorm.DB) error {
						calls++
						if e := tx.Exec(q).Error; e != nil {
							return e
						}
						switch mode {
						case "error":
							return errors.New("fixture failure")
						case "panic":
							panic("fixture panic")
						case "cancel":
							cancel()
						}
						return nil
					})
				}
				var err error
				if mode == "panic" {
					func() {
						defer func() {
							if recover() != "fixture panic" {
								t.Error("panic missing")
							}
						}()
						_ = run()
					}()
				} else {
					err = run()
					if err == nil {
						t.Fatal("unexpected success")
					}
				}
				if strings.HasPrefix(mode, "lost-") && !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) {
					t.Fatal("unknown outcome missing", err)
				}
				if calls != 1 {
					t.Fatal("automatic retry", calls)
				}
				want := "0"
				epoch := "1"
				if mode == "lost-commit" {
					want, epoch = "1", "2"
				}
				if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.projection_generation_events WHERE request_id='"+event+"' AND event_id='"+event+"' AND generation_id='"+g.generation+"' AND expected_head_epoch=1 AND source_vector_sha256=decode(repeat('55',32),'hex')"); got != want {
					t.Fatal("exact request reconciliation", got, want)
				}
				if got := s.must(t, s.runtime, "SELECT epoch FROM eventstore.projection_heads WHERE projection_name='"+g.projection+"'"); got != epoch {
					t.Fatal("partial pointer commit", got, epoch)
				}
			})
		}
	})
}

func TestProjectionGenerationAdversarialPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_PROJECTION_GENERATION") != "1" {
		t.Skip("explicit owned PostgreSQL fixture opt-in required")
	}
	f := newQuarantineFixture(t)
	t.Run("foreign owner isolation runtime migration and empty legacy retention", func(t *testing.T) {
		s := generationStore(t, f)
		copy := *s
		copy.migration = s.runtime
		if out, err := installGeneration(t, &copy); err == nil {
			t.Fatal("runtime installed migration", out)
		}
		generationSave(t, s, "legacy-before")
		before := generationPreimage(t, s)
		mustGeneration(t, s)
		if generationPreimage(t, s) != before {
			t.Fatal("legacy preimage changed")
		}
		generationSave(t, s, "legacy-after")
		foreign := f.store(t, "retail", true)
		if out, err := f.sql(s.database, foreign.runtime, "SELECT count(*) FROM eventstore.projection_generations"); err == nil || !strings.Contains(out, "permission denied") {
			t.Fatalf("foreign owner read: %v %s", err, out)
		}
	})
	t.Run("malformed immutable evidence failed comparison and concurrent chain forks", func(t *testing.T) {
		s := generationStore(t, f)
		mustGeneration(t, s)
		for name, mutate := range map[string]func(string) string{
			"zero generation": func(q string) string {
				g := generationID()
				return strings.Replace(generationSQL(g), g.generation, uuid.Nil.String(), 1)
			},
			"empty projection": func(q string) string {
				g := generationID()
				return strings.Replace(generationSQL(g), g.projection, "", 1)
			},
			"missing authority":     func(q string) string { return strings.Replace(q, "'synthetic-authority'", "NULL", 1) },
			"empty history":         func(q string) string { return strings.Replace(q, "'synthetic-history'", "''", 1) },
			"bad digest":            func(q string) string { return strings.Replace(q, "repeat('33',32)", "repeat('33',31)", 1) },
			"zero contract version": func(q string) string { return strings.Replace(q, "'fixture-contract',1", "'fixture-contract',0", 1) },
		} {
			t.Run(name, func(t *testing.T) { s.reject(t, s.runtime, mutate(generationSQL(generationID())), "") })
		}
		g := generationID()
		s.must(t, s.runtime, generationSQL(g)+generationHeadSQL(g))
		root := uuid.NewString()
		s.reject(t, s.runtime, generationEventSQL(g, uuid.NewString(), "", "validated", 0, ""), "generation root")
		s.reject(t, s.runtime, generationEventSQL(g, uuid.NewString(), uuid.NewString(), "build-progress", 0, ""), "no rows")
		s.reject(t, s.runtime, generationEventSQL(g, root, root, "build-progress", 0, ""), "")
		s.must(t, s.runtime, generationEventSQL(g, root, "", "build-progress", 0, ""))
		s.reject(t, s.runtime, generationEventSQL(g, root, "", "build-progress", 0, ""), "duplicate key")
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- s.db.Exec(generationEventSQL(g, uuid.NewString(), root, "build-progress", 0, "")).Error
			}()
		}
		wg.Wait()
		close(results)
		wins := 0
		for e := range results {
			if e == nil {
				wins++
			}
		}
		if wins != 1 {
			t.Fatal("chain successor winners", wins)
		}
		tip := s.must(t, s.runtime, "SELECT event_id FROM eventstore.projection_generation_events WHERE previous_event_id='"+root+"'")
		fail := uuid.NewString()
		s.must(t, s.runtime, strings.Replace(generationEventSQL(g, fail, tip, "validated", 0, ""), "'passed'", "'failed'", 1))
		s.reject(t, s.runtime, "BEGIN;"+generationSwitchSQL(g, uuid.NewString(), fail, 1, "")+"COMMIT;", "exact passing validation")
		if got := s.must(t, s.runtime, "SELECT epoch||'|'||(active_generation_id IS NULL) FROM eventstore.projection_heads WHERE projection_name='"+g.projection+"'"); got != "1|true" {
			t.Fatal("failed comparator moved head", got)
		}
		abandoned := uuid.NewString()
		s.must(t, s.runtime, generationEventSQL(g, abandoned, fail, "abandon", 0, ""))
		s.reject(t, s.runtime, generationEventSQL(g, uuid.NewString(), abandoned, "build-progress", 0, ""), "abandoned")
		for _, q := range []string{"UPDATE eventstore.projection_generation_events SET source_vector_ref='rewrite'", "TRUNCATE eventstore.projection_heads", "INSERT INTO eventstore.projection_generation_compatibility(singleton) VALUES(true)"} {
			s.reject(t, s.runtime, q, "permission denied")
		}
	})
	t.Run("synthetic bounded batch and cooperating generation guard writer fence", func(t *testing.T) {
		s := generationStore(t, f)
		mustGeneration(t, s)
		g := generationID()
		s.must(t, s.runtime, generationSQL(g)+generationHeadSQL(g))
		// This disposable owner table represents only a synthetic read row. It is
		// not a product read model or evidence that real command writers participate.
		s.must(t, s.migration, "CREATE TABLE eventstore.fixture_generation_rows(generation_id uuid PRIMARY KEY,value bigint NOT NULL); GRANT SELECT,INSERT ON eventstore.fixture_generation_rows TO "+s.runtime+"; GRANT UPDATE(value) ON eventstore.fixture_generation_rows TO "+s.runtime)
		runner, e := eventstore.NewTransactions(s.db, func(tx *gorm.DB) (*gorm.DB, error) { return tx, nil })
		if e != nil {
			t.Fatal(e)
		}
		build := uuid.NewString()
		row := fmt.Sprintf("INSERT INTO eventstore.fixture_generation_rows VALUES('%s',1);", g.generation)
		e = runner.Run(context.Background(), func(tx *gorm.DB) error {
			if e := tx.Exec(row + generationEventSQL(g, build, "", "build-progress", 0, "")).Error; e != nil {
				return e
			}
			return errors.New("synthetic batch failure")
		})
		if e == nil {
			t.Fatal("batch failure lost")
		}
		if got := s.must(t, s.runtime, "SELECT (SELECT count(*) FROM eventstore.fixture_generation_rows)+(SELECT count(*) FROM eventstore.projection_generation_events)"); got != "0" {
			t.Fatal("partial batch", got)
		}
		s.must(t, s.runtime, "BEGIN;"+row+generationEventSQL(g, build, "", "build-progress", 0, "")+"COMMIT;")
		// Shared catalog/receiver/generation/guard locks are the real approved helper,
		// while the guarded row and expected comparison are explicitly synthetic.
		stream, e := eventstore.ReceiverFence("inventory", "inventory", "fixture", uuid.NewString())
		if e != nil {
			t.Fatal(e)
		}
		shared, e := eventstore.GenerationFence("inventory", g.projection, g.generation, eventstore.SharedFence)
		if e != nil {
			t.Fatal(e)
		}
		exclusive, e := eventstore.GenerationFence("inventory", g.projection, g.generation, eventstore.ExclusiveFence)
		if e != nil {
			t.Fatal(e)
		}
		guardShared, e := eventstore.GuardFence("inventory", "synthetic-guard", "synthetic-scope", eventstore.SharedFence)
		if e != nil {
			t.Fatal(e)
		}
		guardExclusive, e := eventstore.GuardFence("inventory", "synthetic-guard", "synthetic-scope", eventstore.ExclusiveFence)
		if e != nil {
			t.Fatal(e)
		}
		writer := s.db.Begin()
		defer writer.Rollback()
		if _, e := eventstore.PrepareFences(context.Background(), writer, "inventory", eventstore.SharedFence, stream, shared, guardShared); e != nil {
			t.Fatal(e)
		}
		if e := writer.Exec("UPDATE eventstore.fixture_generation_rows SET value=2 WHERE generation_id='" + g.generation + "'").Error; e != nil {
			t.Fatal(e)
		}
		result := make(chan error, 1)
		go func() {
			result <- runner.Run(context.Background(), func(tx *gorm.DB) error {
				if e := tx.Exec("SET LOCAL application_name='t928-synthetic-switch'").Error; e != nil {
					return e
				}
				if _, e := eventstore.PrepareFences(context.Background(), tx, "inventory", eventstore.ExclusiveFence, stream, exclusive, guardExclusive); e != nil {
					return e
				}
				var value int64
				if e := tx.Raw("SELECT value FROM eventstore.fixture_generation_rows WHERE generation_id=?", g.generation).Scan(&value).Error; e != nil {
					return e
				}
				if value != 1 {
					return errors.New("synthetic comparison changed under fence")
				}
				return errors.New("unexpected stale comparison")
			})
		}()
		deadline := time.Now().Add(4 * time.Second)
		for {
			var waiting int64
			if e := s.db.Raw("SELECT count(*) FROM pg_stat_activity WHERE application_name='t928-synthetic-switch' AND wait_event_type='Lock'").Scan(&waiting).Error; e != nil {
				t.Fatal(e)
			}
			if waiting == 1 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("exclusive switch did not wait for writer")
			}
			time.Sleep(10 * time.Millisecond)
		}
		if e := writer.Commit().Error; e != nil {
			t.Fatal(e)
		}
		if e := <-result; e == nil || !strings.Contains(e.Error(), "comparison changed") {
			t.Fatal("did not revalidate after writer", e)
		}
		if got := s.must(t, s.runtime, "SELECT epoch||'|'||(active_generation_id IS NULL) FROM eventstore.projection_heads WHERE projection_name='"+g.projection+"'"); got != "1|true" {
			t.Fatal("failed switch moved pointer", got)
		}
		if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.projection_generation_events WHERE generation_id='"+g.generation+"'"); got != "1" {
			t.Fatal("failed switch left extra evidence", got)
		}
	})
}

func TestProjectionGenerationMigrationPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_PROJECTION_GENERATION") != "1" {
		t.Skip("explicit owned PostgreSQL fixture opt-in required")
	}
	f := newQuarantineFixture(t)
	for p, want := range map[string]string{"schema.sql": checkpointBaseHash, "migrations/000002_messaging_delivery.up.sql": routePriorHash, "migrations/000003_messaging_route_compatibility.up.sql": checkpointRouteHash, "migrations/000004_projection_checkpoint.up.sql": quarantineCheckpointHash, "migrations/000005_quarantine_evidence.up.sql": generationQuarantineHash} {
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != want {
			t.Fatal("prior artifact changed", p)
		}
	}
	t.Logf("000006 SHA-256 %x", sha256.Sum256([]byte(generationScript(t))))
	t.Run("populated retention precise label correction and both opaque consumer kinds", func(t *testing.T) {
		s := generationStore(t, f)
		r := s.produce(t, "inventory")[0]
		quarantineCutover(t, s, r)
		old := checkpointID(0)
		admission := uuid.NewString()
		s.must(t, s.runtime, checkpointAdmission(old, admission, "projection")+bootstrapSQL(old, admission, "projection")+checkpointSQL(old))
		e := fixtureSeal([]byte("synthetic-nonsecret"), "fixture-context")
		s.must(t, s.runtime, evidenceSQL(e))
		spaces := checkpointID(0)
		spaces.generation = " "
		a := uuid.NewString()
		s.must(t, s.runtime, checkpointAdmission(spaces, a, "process"))
		s.reject(t, s.runtime, "BEGIN;"+bootstrapSQL(spaces, a, "process")+checkpointSQL(spaces)+"COMMIT;", "consumer_bootstraps_generation_check")
		generationSave(t, s, "before")
		rows, cat, cons, fn, acl := generationPreimage(t, s), s.catalog(t), generationConstraints(t, s), s.functions(t, true), s.functions(t, false)
		mustGeneration(t, s)
		if rows != generationPreimage(t, s) {
			t.Fatal("retained rows changed")
		}
		generationSubset(t, cat, s.catalog(t))
		generationSubset(t, cons, generationConstraints(t, s))
		generationSubset(t, fn, s.functions(t, true))
		generationSubset(t, acl, s.functions(t, false))
		if got := generationLabel(t, s); got != "consumer_bootstraps_generation_check|(length(generation) > 0)|true|true" {
			t.Fatal("corrected constraint mismatch", got)
		}
		if got := s.must(t, s.runtime, "SELECT base_schema_version||'|'||migration_revision||'|'||feature_format_version||'|'||encode(generation_migration_sha256,'hex') FROM eventstore.projection_generation_compatibility"); got != "2|6|1|"+fmt.Sprintf("%x", sha256.Sum256([]byte(generationScript(t)))) {
			t.Fatal("marker mismatch", got)
		}
		generationSave(t, s, "after")
		for _, kind := range []string{"projection", "process"} {
			for _, label := range []string{" ", "   ", "\t", " padded ", "ordinary-non-uuid"} {
				b := checkpointID(0)
				b.generation = label
				a := uuid.NewString()
				s.must(t, s.runtime, "BEGIN;"+checkpointAdmission(b, a, kind)+bootstrapSQL(b, a, kind)+checkpointSQL(b)+"COMMIT;")
				// Storage fixture only: explicit custody transaction marker permits
				// the synthetic inbox row. This is not a T922 adapter/ACK proof.
				s.must(t, s.runtime, "BEGIN; SET LOCAL justix.messaging_mode='custody';"+checkpointAdvance(b, uuid.NewString(), 1)+"COMMIT;")
				if got := s.must(t, s.runtime, "SELECT generation='"+label+"' FROM eventstore.consumer_checkpoints WHERE "+checkpointWhere(b)); got != "t" {
					t.Fatal("label normalized", kind, label)
				}
			}
		}
		for _, label := range []string{"", "different"} {
			b := checkpointID(0)
			a := uuid.NewString()
			s.must(t, s.runtime, checkpointAdmission(b, a, "projection"))
			b.generation = label
			s.reject(t, s.runtime, "BEGIN;"+bootstrapSQL(b, a, "projection")+checkpointSQL(b)+"COMMIT;", "")
		}
		if out, e := installGeneration(t, s); e == nil {
			t.Fatal("reinstall succeeded", out)
		}
	})
	t.Run("immutable chains exact passing comparison head CAS and retained reads", func(t *testing.T) {
		s := generationStore(t, f)
		mustGeneration(t, s)
		g := generationID()
		validated := generationPrepare(t, s, g)
		s.must(t, s.runtime, generationHeadSQL(g))
		sw := uuid.NewString()
		s.reject(t, s.runtime, generationEventSQL(g, sw, validated, "switch", 1, ""), "matching committed head")
		s.reject(t, s.runtime, "BEGIN;"+strings.Replace(generationSwitchSQL(g, sw, validated, 1, ""), "synthetic-finite-vector", "different-vector", 1)+"COMMIT;", "vector or cursor changed")
		s.must(t, s.runtime, "BEGIN;"+generationSwitchSQL(g, sw, validated, 1, "")+"COMMIT;")
		s.reject(t, s.runtime, "UPDATE eventstore.projection_heads SET epoch=epoch+1 WHERE projection_name='"+g.projection+"'", "new immutable evidence")
		for _, tab := range []string{"projection_generations", "projection_generation_events", "projection_generation_compatibility"} {
			for _, role := range []string{s.runtime, s.migration} {
				s.reject(t, role, "DELETE FROM eventstore."+tab, "")
				s.reject(t, role, "TRUNCATE eventstore."+tab, "")
			}
		}
		s.reject(t, s.migration, "UPDATE eventstore.projection_generations SET consumer_name='rewrite'", "immutable")
		s.reject(t, s.runtime, "UPDATE eventstore.projection_heads SET projection_name='rewrite'", "permission denied")
		s.reject(t, s.runtime, "DELETE FROM eventstore.projection_heads", "permission denied")
		s.reject(t, s.migration, "DELETE FROM eventstore.projection_heads", "immutable")
		copy := generationID()
		copy.consumer = g.consumer
		s.reject(t, s.runtime, generationSQL(copy), "duplicate key")
		s.reject(t, s.runtime, strings.Replace(generationSQL(generationID()), "'projection'", "'process'", 1), "check constraint")
		hold := uuid.NewString()
		s.reject(t, s.runtime, generationEventSQL(g, hold, sw, "hold", 2, g.generation), "matching committed head")
		s.reject(t, s.runtime, generationEventSQL(g, hold, sw, "hold", 0, ""), "active generation hold")
		// Even individually valid pointer steps cannot commit two changes for
		// the same projection in one transaction: every receipt must match the
		// final head. Both events and both updates roll back on the first check.
		one, two := uuid.NewString(), uuid.NewString()
		multiple := generationEventSQL(g, one, sw, "hold", 2, g.generation) + fmt.Sprintf("UPDATE eventstore.projection_heads SET epoch=3,hold_ref='synthetic-hold',last_event_id='%s' WHERE projection_name='%s';", one, g.projection)
		multiple += generationEventSQL(g, two, one, "hold", 3, g.generation) + fmt.Sprintf("UPDATE eventstore.projection_heads SET epoch=4,hold_ref='synthetic-hold',last_event_id='%s' WHERE projection_name='%s';", two, g.projection)
		s.reject(t, s.runtime, "BEGIN;"+multiple+"COMMIT;", "matching committed head")
		if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.projection_generation_events WHERE event_id IN ('"+one+"','"+two+"')"); got != "0" {
			t.Fatal("multi-change transaction left evidence", got)
		}
		s.must(t, s.runtime, "BEGIN;"+generationEventSQL(g, hold, sw, "hold", 2, g.generation)+fmt.Sprintf("UPDATE eventstore.projection_heads SET epoch=3,hold_ref='synthetic-hold',last_event_id='%s' WHERE projection_name='%s';COMMIT;", hold, g.projection))
		next := generationID()
		next.projection = g.projection
		nv := generationPrepare(t, s, next)
		s.reject(t, s.runtime, "BEGIN;"+generationSwitchSQL(next, uuid.NewString(), nv, 2, g.generation)+"COMMIT;", "matching committed head")
		second := uuid.NewString()
		s.must(t, s.runtime, "BEGIN;"+generationSwitchSQL(next, second, nv, 3, g.generation)+"COMMIT;")
		if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.projection_generations WHERE projection_name='"+g.projection+"'"); got != "2" {
			t.Fatal("old generation removed")
		}
		if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.projection_generation_events WHERE generation_id='"+g.generation+"'"); got != "4" {
			t.Fatal("old history changed", got)
		}
		if got := s.must(t, s.runtime, "SELECT active_generation_id::text||'|'||epoch||'|'||(hold_ref IS NULL) FROM eventstore.projection_heads WHERE projection_name='"+g.projection+"'"); got != next.generation+"|4|true" {
			t.Fatal("head identity", got)
		}
		s.reject(t, s.runtime, generationEventSQL(next, uuid.NewString(), hold, "build-progress", 0, ""), "foreign or abandoned")
		s.reject(t, s.runtime, generationEventSQL(next, uuid.NewString(), "", "build-progress", 0, ""), "duplicate key")
	})
}
