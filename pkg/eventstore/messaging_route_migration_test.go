package eventstore_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
	"justixauto/pkg/outbox"
)

const routePriorHash = "1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2"

// All helpers belong to this leaf. This fixture accepts no existing DSN and
// publishes only its randomly assigned loopback port. Its roles/databases are
// synthetic and local to its own disposable pinned container.
type routeFixture struct {
	t                                   *testing.T
	container, password, port, evidence string
	base, prior, correction             string
}
type routeStore struct {
	f                                   *routeFixture
	owner, database, migration, runtime string
	db                                  *gorm.DB
}

func newRouteFixture(t *testing.T) *routeFixture {
	t.Helper()
	f := &routeFixture{t: t, container: "justixauto-t925-" + uuid.NewString(), password: "synthetic-" + uuid.NewString()}
	var err error
	f.evidence, err = os.MkdirTemp("", "justixauto-t925-evidence-")
	if err != nil {
		t.Fatal(err)
	}
	// Keep synthetic function definitions, ACLs and complete row preimages for
	// independent review after the disposable database is removed. No passwords.
	t.Logf("Recoverable synthetic migration evidence: %s", f.evidence)
	for path, dst := range map[string]*string{
		"schema.sql": &f.base, "migrations/000002_messaging_delivery.up.sql": &f.prior,
		"migrations/000003_messaging_route_compatibility.up.sql": &f.correction,
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		*dst = string(b)
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(f.prior))) != routePriorHash {
		t.Fatal("installed 000002 artifact changed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	image := "docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"
	if out, err := exec.CommandContext(ctx, "docker", "run", "-d", "--name", f.container, "-e", "POSTGRES_PASSWORD="+f.password, "-p", "127.0.0.1::5432", image).CombinedOutput(); err != nil {
		t.Fatalf("start fixture: %v %s", err, out)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "docker", "rm", "-f", f.container).CombinedOutput(); err != nil {
			t.Errorf("remove owned fixture: %v %s", err, out)
		}
	})
	for {
		if out, err := f.sql("postgres", "postgres", "SHOW server_version_num"); err == nil {
			if out != "180006" {
				t.Fatalf("unexpected server: %s", out)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("fixture startup timeout")
		case <-time.After(100 * time.Millisecond):
		}
	}
	out, err := exec.CommandContext(ctx, "docker", "port", f.container, "5432/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	address := strings.TrimSpace(string(out))
	if !strings.HasPrefix(address, "127.0.0.1:") || strings.Contains(address, "\n") {
		t.Fatalf("unexpected binding %q", address)
	}
	f.port = strings.TrimPrefix(address, "127.0.0.1:")
	t.Logf("PostgreSQL 180006; 000002 SHA-256 %s; 000003 SHA-256 %x", routePriorHash, sha256.Sum256([]byte(f.correction)))
	return f
}
func (f *routeFixture) sql(database, role, query string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", append([]string{"exec", "-i", "-e", "PGPASSWORD=" + f.password, f.container, "psql", "-X", "-qAt", "-h", "127.0.0.1", "-U", role, "-d", database, "-v", "ON_ERROR_STOP=1"}, args...)...)
	cmd.Stdin = strings.NewReader(query)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
func (s *routeStore) must(t *testing.T, role, query string) string {
	t.Helper()
	out, err := s.f.sql(s.database, role, query)
	if err != nil {
		t.Fatalf("SQL failed: %v %s\n%s", err, out, query)
	}
	return out
}
func (s *routeStore) reject(t *testing.T, role, query, want string) {
	t.Helper()
	out, err := s.f.sql(s.database, role, query)
	if err == nil || !strings.Contains(out, want) {
		t.Fatalf("expected %q: %v %s", want, err, out)
	}
}
func (f *routeFixture) store(t *testing.T, owner string, withPrior bool) *routeStore {
	t.Helper()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	s := &routeStore{f: f, owner: owner, database: "route_" + suffix, migration: "migration_" + suffix, runtime: "runtime_" + suffix}
	for _, q := range []string{"CREATE ROLE " + s.migration + " LOGIN PASSWORD '" + f.password + "'", "CREATE ROLE " + s.runtime + " LOGIN PASSWORD '" + f.password + "'", "CREATE DATABASE " + s.database + " OWNER " + s.migration} {
		if out, err := f.sql("postgres", "postgres", q); err != nil {
			t.Fatalf("create owner: %v %s", err, out)
		}
	}
	if out, err := s.install(f.base); err != nil {
		t.Fatalf("T008: %v %s", err, out)
	}
	if withPrior {
		if out, err := s.install(f.prior); err != nil {
			t.Fatalf("T917: %v %s", err, out)
		}
	}
	dsn := fmt.Sprintf("host=127.0.0.1 port=%s user=%s password=%s dbname=%s sslmode=disable", f.port, s.runtime, f.password, s.database)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	s.db = db
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(8)
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}
func (s *routeStore) install(script string, overrides ...string) (string, error) {
	return s.installAs(s.migration, script, overrides...)
}
func (s *routeStore) installAs(role, script string, overrides ...string) (string, error) {
	args := []string{"-v", "owner_service=" + s.owner, "-v", "runtime_role=" + s.runtime, "-v", "prior_migration_sha256=" + routePriorHash, "-v", fmt.Sprintf("correction_migration_sha256=%x", sha256.Sum256([]byte(s.f.correction))), "-v", "backup_ref=synthetic-recoverable-test-preimage", "-v", "stopped_runtimes_ref=synthetic-no-service-or-AMQP-processes", "-v", "compatibility_ref=synthetic-regression-evidence"}
	return s.f.sql(s.database, role, script, append(args, overrides...)...)
}
func (s *routeStore) correct(t *testing.T) {
	t.Helper()
	s.saveEvidence(t, "before-correction")
	grants := s.catalog(t)
	functionACLs := s.functions(t, false)
	var priorFunctions, correctedFunctions map[string]string
	if err := json.Unmarshal([]byte(s.functions(t, true)), &priorFunctions); err != nil {
		t.Fatal(err)
	}
	if out, err := s.install(s.f.correction); err != nil {
		t.Fatalf("correction: %v %s", err, out)
	}
	if s.catalog(t) != grants {
		t.Fatal("correction changed existing table/column grants or trigger attachments")
	}
	if s.functions(t, false) != functionACLs {
		t.Fatal("correction changed function OIDs, ACLs, owner or execution properties")
	}
	if err := json.Unmarshal([]byte(s.functions(t, true)), &correctedFunctions); err != nil {
		t.Fatal(err)
	}
	if len(priorFunctions) != len(correctedFunctions) {
		t.Fatal("correction added or removed functions")
	}
	for name, definition := range priorFunctions {
		wantChanged := name == "messaging_delivery_guard" || name == "messaging_job_guard" || name == "messaging_dispatch_route"
		if (definition != correctedFunctions[name]) != wantChanged {
			t.Fatalf("unexpected function replacement: %s", name)
		}
	}
	s.saveEvidence(t, "after-correction")
}
func (s *routeStore) cutover(t *testing.T) { t.Helper(); s.must(t, s.migration, routeCutover()) }
func routeCutover() string {
	return fmt.Sprintf("SELECT eventstore.activate_messaging_custody('%s',sha256('corrected-schema-and-marker'::bytea),decode('%s','hex'),'synthetic-backup','synthetic-no-processes-or-AMQP','synthetic-inbox-checkpoints','synthetic-cumulative-compatibility')", uuid.NewString(), routePriorHash)
}
func routeQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", "''") + "'" }
func routeRevision(n int64) events.Revision {
	v, err := events.NewRevision(n)
	if err != nil {
		panic(err)
	}
	return v
}

type routePayload struct {
	Reference string `json:"reference"`
}
type routePorts struct {
	append *eventstore.Appender
	insert *outbox.Inserter
}
type routeRecord struct {
	ID, Stream, Route string
	Sequence          int64
	Body, Hash        []byte
}

func routeEvent(t *testing.T, stream string, revision int64) (eventstore.Event, events.EventSchema[routePayload]) {
	t.Helper()
	schema, err := events.NewEventSchema("inventory.fixture.changed.v1", 1, "fixture", events.CompanyScopeTenantRequired, func(v routePayload) error {
		if v.Reference == "" {
			return errors.New("missing reference")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	company := uuid.NewString()
	e, err := eventstore.NewEvent(events.EnvelopeInput{EventID: uuid.NewString(), AggregateID: stream, AggregateVersion: routeRevision(revision), IntegrationSequence: routeRevision(revision), CompanyID: &company, OccurredAt: time.Now().UTC(), Actor: events.Actor{Kind: events.ActorUser, ID: uuid.NewString()}, CorrelationID: uuid.NewString(), CausationID: uuid.NewString(), OperationID: uuid.NewString()}, schema, routePayload{Reference: "synthetic-nonsecret-reference"})
	if err != nil {
		t.Fatal(err)
	}
	return e, schema
}
func (s *routeStore) produce(t *testing.T, targets ...events.Owner) []routeRecord {
	t.Helper()
	runner, err := eventstore.NewTransactions(s.db, func(tx *gorm.DB) (routePorts, error) {
		a, err := eventstore.NewAppender(tx, events.OwnerInventory)
		if err != nil {
			return routePorts{}, err
		}
		i, err := outbox.NewInserter(tx, events.OwnerInventory)
		return routePorts{a, i}, err
	})
	if err != nil {
		t.Fatal(err)
	}
	stream := uuid.NewString()
	var es []eventstore.Event
	var records []outbox.Record
	var expected []routeRecord
	for n, target := range targets {
		e, schema := routeEvent(t, stream, int64(n+1))
		env, _ := e.IntegrationEnvelope()
		record, err := outbox.NewRecord(env, schema, target)
		if err != nil {
			t.Fatal(err)
		}
		body, err := env.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(body)
		es = append(es, e)
		records = append(records, record)
		expected = append(expected, routeRecord{ID: env.EventID(), Stream: env.AggregateID(), Sequence: env.IntegrationSequence().Int64(), Body: body, Hash: hash[:]})
	}
	ctx := context.Background()
	if err := runner.Run(ctx, func(p routePorts) error {
		if err := p.append.Append(ctx, eventstore.Batch{Expected: routeRevision(0), Events: es}); err != nil {
			return err
		}
		return p.insert.Insert(ctx, records...)
	}); err != nil {
		t.Fatal(err)
	}
	for n := range expected {
		var row struct {
			RoutingKey             string
			Envelope, EnvelopeHash []byte
			IntegrationSequence    int64
		}
		if err := s.db.Raw("SELECT routing_key,envelope,envelope_hash,integration_sequence FROM eventstore.outbox WHERE event_id=?::uuid", expected[n].ID).Scan(&row).Error; err != nil {
			t.Fatal(err)
		}
		if row.RoutingKey != "inventory."+string(targets[n])+".inventory.fixture.changed.v1" || !bytes.Equal(row.Envelope, expected[n].Body) || !bytes.Equal(row.EnvelopeHash, expected[n].Hash) || row.IntegrationSequence != expected[n].Sequence {
			t.Fatal("public producer persisted incompatible route or bytes")
		}
		// Subsequent migration/intake tests use the actual SQL route, not a fixture reconstruction.
		expected[n].Route = row.RoutingKey
	}
	return expected
}
func routeAdmission(id, stream, namespace, subject, schema string) string {
	local := "NULL,NULL"
	if namespace == "local-consumer" {
		local = "'projection','generation-1'"
	}
	return fmt.Sprintf(`INSERT INTO eventstore.messaging_admissions(admission_id,namespace,source_owner,aggregate_type,aggregate_id,subject,action,contract_id,contract_version,contract_digest,schema_allowlist,authority_ref,scope_ref,purpose,bootstrap_ref,start_after,consumer_kind,generation) VALUES('%s','%s','inventory','fixture','%s','%s','admit','synthetic-reviewed-contract',1,sha256('synthetic-contract'::bytea),'["%s"]','synthetic-authority','synthetic-company-scope','synthetic-test-only','synthetic-empty-stream-bootstrap',0,%s);`, id, namespace, stream, subject, schema, local)
}
func routeSource(r routeRecord, admission, destination string) string {
	return fmt.Sprintf(`INSERT INTO eventstore.outbox_messages(event_id,source_owner,aggregate_type,aggregate_id,integration_sequence,envelope,envelope_hash,recipients,plan_authority_ref) VALUES('%s','inventory','fixture','%s',%d,decode('%s','hex'),decode('%s','hex'),'[{"destination":"%s","admission_id":"%s"}]','synthetic-plan'); INSERT INTO eventstore.outbox_deliveries(event_id,destination,admission_id,exchange,routing_key) VALUES('%s','%s','%s','justix.integration.v1',%s);`, r.ID, r.Stream, r.Sequence, hex.EncodeToString(r.Body), hex.EncodeToString(r.Hash), destination, admission, r.ID, destination, admission, routeQuote(r.Route))
}
func routeCustody(r routeRecord, admission, consumer, generation string) string {
	enrollment := uuid.NewString()
	return fmt.Sprintf(`INSERT INTO eventstore.dispatch_messages(event_id,source_owner,aggregate_type,aggregate_id,integration_sequence,envelope,envelope_hash,exchange,routing_key,membership_version,admission_ref,bootstrap_ref,initial_enrollment_id) VALUES('%s','inventory','fixture','%s',%d,decode('%s','hex'),decode('%s','hex'),'justix.integration.v1',%s,'membership-1','synthetic-admission','synthetic-bootstrap','%s'); INSERT INTO eventstore.dispatch_enrollments(event_id,enrollment_id,membership_version,cutover_ref,consumers) VALUES('%s','%s','membership-1','synthetic-cutover','[{"consumer_name":"%s","admission_id":"%s"}]'); INSERT INTO eventstore.dispatch_jobs(consumer_name,event_id,enrollment_id,admission_id,consumer_kind,generation) VALUES('%s','%s','%s','%s','projection','%s');`, r.ID, r.Stream, r.Sequence, hex.EncodeToString(r.Body), hex.EncodeToString(r.Hash), routeQuote(r.Route), enrollment, r.ID, enrollment, consumer, admission, consumer, r.ID, enrollment, admission, generation)
}
func routeComplete(r routeRecord, consumer string, hash []byte) string {
	return fmt.Sprintf(`SET LOCAL justix.messaging_mode='custody'; INSERT INTO eventstore.inbox(consumer_name,event_id,envelope_hash) VALUES('%s','%s',decode('%s','hex')); UPDATE eventstore.dispatch_jobs SET completed_at=clock_timestamp(),inbox_consumer_name=consumer_name,inbox_event_id=event_id WHERE event_id='%s' AND consumer_name='%s';`, consumer, r.ID, hex.EncodeToString(hash), r.ID, consumer)
}
func (s *routeStore) snapshot(t *testing.T) string {
	t.Helper()
	return s.must(t, s.migration, `SELECT jsonb_object_agg(name,rows ORDER BY name) FROM (
	 SELECT 'events' name,coalesce(jsonb_agg(to_jsonb(t) ORDER BY event_id),'[]') rows FROM eventstore.events t UNION ALL
	 SELECT 'receipts',coalesce(jsonb_agg(to_jsonb(t) ORDER BY receipt_id),'[]') FROM eventstore.command_receipts t UNION ALL
	 SELECT 'operations',coalesce(jsonb_agg(to_jsonb(t) ORDER BY operation_id),'[]') FROM eventstore.operations t UNION ALL
	 SELECT 'steps',coalesce(jsonb_agg(to_jsonb(t) ORDER BY operation_id,step),'[]') FROM eventstore.operation_steps t UNION ALL
	 SELECT 'outbox',coalesce(jsonb_agg(to_jsonb(t) ORDER BY event_id),'[]') FROM eventstore.outbox t UNION ALL
 SELECT 'inbox',coalesce(jsonb_agg(to_jsonb(t) ORDER BY consumer_name,event_id),'[]') FROM eventstore.inbox t UNION ALL
 SELECT 'mode',coalesce(jsonb_agg(to_jsonb(t)),'[]') FROM eventstore.messaging_mode t UNION ALL
 SELECT 'admissions',coalesce(jsonb_agg(to_jsonb(t) ORDER BY admission_id),'[]') FROM eventstore.messaging_admissions t UNION ALL
 SELECT 'authorizations',coalesce(jsonb_agg(to_jsonb(t) ORDER BY event_id),'[]') FROM eventstore.messaging_legacy_authorizations t UNION ALL
 SELECT 'cutovers',coalesce(jsonb_agg(to_jsonb(t) ORDER BY cutover_id),'[]') FROM eventstore.messaging_cutovers t UNION ALL
 SELECT 'evidence',coalesce(jsonb_agg(to_jsonb(t) ORDER BY event_id),'[]') FROM eventstore.messaging_legacy_evidence t UNION ALL
 SELECT 'messages',coalesce(jsonb_agg(to_jsonb(t) ORDER BY event_id),'[]') FROM eventstore.outbox_messages t UNION ALL
 SELECT 'deliveries',coalesce(jsonb_agg(to_jsonb(t) ORDER BY event_id,destination),'[]') FROM eventstore.outbox_deliveries t UNION ALL
 SELECT 'custody',coalesce(jsonb_agg(to_jsonb(t) ORDER BY event_id),'[]') FROM eventstore.dispatch_messages t UNION ALL
 SELECT 'enrollments',coalesce(jsonb_agg(to_jsonb(t) ORDER BY event_id,enrollment_id),'[]') FROM eventstore.dispatch_enrollments t UNION ALL
 SELECT 'jobs',coalesce(jsonb_agg(to_jsonb(t) ORDER BY event_id,consumer_name),'[]') FROM eventstore.dispatch_jobs t) evidence`)
}
func (s *routeStore) catalog(t *testing.T) string {
	t.Helper()
	return s.must(t, s.migration, `SELECT jsonb_object_agg(c.relname,jsonb_build_object('oid',c.oid,'owner',c.relowner,'acl',c.relacl,
 'columns',(SELECT jsonb_agg(jsonb_build_object('name',a.attname,'acl',a.attacl) ORDER BY a.attnum) FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped),
 'triggers',(SELECT jsonb_agg(jsonb_build_object('oid',g.oid,'function',g.tgfoid,'definition',pg_get_triggerdef(g.oid)) ORDER BY g.oid) FROM pg_trigger g WHERE g.tgrelid=c.oid)))
 FROM pg_class c WHERE c.relnamespace='eventstore'::regnamespace AND c.relkind='r' AND c.relname<>'messaging_route_compatibility'`)
}
func (s *routeStore) saveEvidence(t *testing.T, label string) {
	t.Helper()
	name := s.database + "-" + label + "-" + uuid.NewString()
	rows := s.snapshot(t)
	var tables map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rows), &tables); err != nil {
		t.Fatal(err)
	}
	type tableEvidence struct {
		Count  int    `json:"count"`
		SHA256 string `json:"sha256"`
	}
	counts := make(map[string]tableEvidence, len(tables))
	for name, content := range tables {
		var entries []json.RawMessage
		if err := json.Unmarshal(content, &entries); err != nil {
			t.Fatal(err)
		}
		counts[name] = tableEvidence{len(entries), fmt.Sprintf("%x", sha256.Sum256(content))}
	}
	countJSON, err := json.Marshal(counts)
	if err != nil {
		t.Fatal(err)
	}
	for kind, content := range map[string]string{"rows": rows, "table-counts-digests": string(countJSON), "functions": s.functions(t, true), "function-acls": s.functions(t, false), "table-acls-triggers": s.catalog(t)} {
		path := filepath.Join(s.f.evidence, name+"-"+kind+".json")
		if err := os.WriteFile(path, []byte(content+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path+".sha256", []byte(fmt.Sprintf("%x\n", sha256.Sum256([]byte(content+"\n")))), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func (s *routeStore) functions(t *testing.T, definitions bool) string {
	t.Helper()
	field := "jsonb_build_object('oid',oid,'owner',proowner,'acl',proacl,'definer',prosecdef,'config',proconfig)"
	if definitions {
		field = "to_jsonb(pg_get_functiondef(oid))"
	}
	return s.must(t, s.migration, "SELECT jsonb_object_agg(proname,"+field+" ORDER BY proname) FROM pg_proc WHERE pronamespace='eventstore'::regnamespace")
}
func (s *routeStore) rejectCorrection(t *testing.T, want string, overrides ...string) {
	t.Helper()
	before, defs, acls := s.snapshot(t), s.functions(t, true), s.functions(t, false)
	s.saveEvidence(t, "rejected-correction-preimage")
	out, err := s.install(s.f.correction, overrides...)
	if err == nil || !strings.Contains(out, want) {
		t.Fatalf("expected correction %q: %v %s", want, err, out)
	}
	if s.snapshot(t) != before || s.functions(t, true) != defs || s.functions(t, false) != acls || s.must(t, s.migration, "SELECT to_regclass('eventstore.messaging_route_compatibility') IS NULL") != "t" {
		t.Fatal("failed correction changed historical content/functions or left a marker")
	}
}

func TestMessagingRouteMigrationPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_MESSAGING_ROUTE_MIGRATION") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_MESSAGING_ROUTE_MIGRATION=1 for isolated pinned PostgreSQL")
	}
	f := newRouteFixture(t)
	s := f.store(t, "inventory", false)
	rows := s.produce(t, events.OwnerRetail, events.OwnerRetail, events.OwnerFinancing, events.OwnerInventory)
	s.must(t, s.runtime, "UPDATE eventstore.outbox SET attempts=4,next_attempt_at='2026-09-14T10:00:00Z',lease_owner='"+uuid.NewString()+"',lease_until='2026-09-16T12:00:00Z'; UPDATE eventstore.outbox SET sent_at='2026-09-14T12:00:00Z' WHERE event_id='"+rows[0].ID+"'; INSERT INTO eventstore.inbox VALUES('legacy.consumer','"+uuid.NewString()+"',sha256('legacy'::bytea),'2026-09-14T12:00:00Z')")
	if out, err := s.install(f.prior); err != nil {
		t.Fatalf("unchanged T917: %v %s", err, out)
	}
	admit, selfAdmit := uuid.NewString(), uuid.NewString()
	s.must(t, s.runtime, routeAdmission(admit, rows[0].Stream, "source-recipient", "retail", "inventory.fixture.changed.v1")+routeAdmission(selfAdmit, rows[0].Stream, "source-recipient", "inventory", "inventory.fixture.changed.v1"))
	s.must(t, s.migration, fmt.Sprintf(`INSERT INTO eventstore.messaging_legacy_authorizations VALUES('%s','retail','%s',NULL,'synthetic-sent-review'),('%s','retail','%s',NULL,'synthetic-unsent-review'),('%s','financing',NULL,'historical-authority-unavailable','synthetic-held-review'),('%s','inventory','%s',NULL,'synthetic-self-review');`, rows[0].ID, admit, rows[1].ID, admit, rows[2].ID, rows[3].ID, selfAdmit))
	t.Run("public producer populated old cutover fails and rolls back", func(t *testing.T) {
		before := s.snapshot(t)
		s.reject(t, s.migration, routeCutover(), "delivery schema is not admitted")
		if s.snapshot(t) != before {
			t.Fatal("old failed cutover changed populated legacy state")
		}
		if got := s.must(t, s.runtime, "SELECT mode||':'||(SELECT count(*) FROM eventstore.messaging_cutovers)||':'||(SELECT count(*) FROM eventstore.messaging_legacy_evidence)||':'||(SELECT count(*) FROM eventstore.outbox_messages)||':'||(SELECT count(*) FROM eventstore.outbox_deliveries) FROM eventstore.messaging_mode"); got != "legacy:0:0:0:0" {
			t.Fatal(got)
		}
	})
	t.Run("forward correction and populated cutover retain exact evidence", func(t *testing.T) {
		before, acls := s.snapshot(t), s.functions(t, false)
		s.correct(t)
		if s.snapshot(t) != before || s.functions(t, false) != acls {
			t.Fatal("correction changed retained content or function identity/ACL")
		}
		t.Logf("recoverable synthetic preimage SHA-256 %x; original function/ACL evidence SHA-256 %x", sha256.Sum256([]byte(before)), sha256.Sum256([]byte(acls)))
		want := fmt.Sprintf("inventory:%s:2:3:1:%s:%x", s.runtime, routePriorHash, sha256.Sum256([]byte(f.correction)))
		if got := s.must(t, s.runtime, "SELECT owner_service||':'||runtime_role||':'||base_schema_version||':'||migration_revision||':'||route_format_version||':'||encode(prior_migration_sha256,'hex')||':'||encode(correction_migration_sha256,'hex') FROM eventstore.messaging_route_compatibility"); got != want {
			t.Fatalf("marker %s", got)
		}
		legacy := s.must(t, s.runtime, "SELECT jsonb_agg(to_jsonb(o) ORDER BY event_id) FROM eventstore.outbox o")
		inbox := s.must(t, s.runtime, "SELECT jsonb_agg(to_jsonb(i) ORDER BY consumer_name,event_id) FROM eventstore.inbox i")
		s.cutover(t)
		s.saveEvidence(t, "after-populated-cutover")
		if s.must(t, s.runtime, "SELECT jsonb_agg(to_jsonb(o) ORDER BY event_id) FROM eventstore.outbox o") != legacy || s.must(t, s.runtime, "SELECT jsonb_agg(original_row ORDER BY event_id) FROM eventstore.messaging_legacy_evidence") != legacy || s.must(t, s.runtime, "SELECT jsonb_agg(to_jsonb(i) ORDER BY consumer_name,event_id) FROM eventstore.inbox i") != inbox {
			t.Fatal("cutover lost original rows/evidence")
		}
		if got := s.must(t, s.runtime, `SELECT count(*) FROM eventstore.outbox o JOIN eventstore.outbox_messages m USING(event_id) JOIN eventstore.outbox_deliveries d USING(event_id) WHERE m.envelope=o.envelope AND m.envelope_hash=o.envelope_hash AND m.aggregate_id=o.aggregate_id AND m.aggregate_type=o.aggregate_type AND m.integration_sequence=o.integration_sequence AND m.created_at=o.created_at AND d.routing_key=o.routing_key AND d.exchange=o.exchange AND d.attempts=o.attempts AND d.next_attempt_at=o.next_attempt_at AND d.sent_at IS NOT DISTINCT FROM o.sent_at AND d.lease_owner IS NULL AND d.lease_until IS NULL`); got != "4" {
			t.Fatal("retained copy mismatch", got)
		}
		if got := s.must(t, s.runtime, "SELECT legacy_rows||':'||legacy_inbox_rows FROM eventstore.messaging_cutovers"); got != "4:1" {
			t.Fatal(got)
		}
		for _, q := range []string{"UPDATE eventstore.outbox_deliveries SET hold_ref=NULL WHERE event_id='" + rows[2].ID + "'", "UPDATE eventstore.outbox_deliveries SET sent_at=clock_timestamp() WHERE event_id='" + rows[2].ID + "'", "UPDATE eventstore.outbox_deliveries SET sent_at=NULL WHERE event_id='" + rows[0].ID + "'"} {
			s.reject(t, s.runtime, q, "ERROR")
		}
		s.must(t, s.runtime, "UPDATE eventstore.outbox_deliveries SET attempts=attempts+1 WHERE event_id='"+rows[1].ID+"'")
	})
	t.Run("distinct target and self custody use exact public producer records", func(t *testing.T) {
		target := f.store(t, "retail", true)
		target.correct(t)
		target.cutover(t)
		for _, tc := range []struct {
			store  *routeStore
			record routeRecord
		}{{target, rows[1]}, {s, rows[3]}} {
			consumer := "inventory-fixture.projection"
			a := uuid.NewString()
			r := tc.record
			tc.store.must(t, tc.store.runtime, "BEGIN;"+routeAdmission(a, r.Stream, "local-consumer", consumer, "inventory.fixture.changed.v1")+routeCustody(r, a, consumer, "generation-1")+"COMMIT;")
			tc.store.must(t, tc.store.runtime, "UPDATE eventstore.dispatch_jobs SET attempts=attempts+1,lease_owner='"+uuid.NewString()+"',lease_until=clock_timestamp()+interval '1 minute' WHERE event_id='"+r.ID+"'")
			tc.store.must(t, tc.store.runtime, "BEGIN;"+routeComplete(r, consumer, r.Hash)+"COMMIT;")
			if got := tc.store.must(t, tc.store.runtime, "SELECT count(*) FROM eventstore.dispatch_messages m JOIN eventstore.dispatch_jobs j USING(event_id) JOIN eventstore.inbox i ON i.consumer_name=j.consumer_name AND i.event_id=j.event_id WHERE m.event_id='"+r.ID+"' AND m.routing_key="+routeQuote(r.Route)+" AND m.envelope=decode('"+hex.EncodeToString(r.Body)+"','hex') AND m.envelope_hash=i.envelope_hash AND j.completed_at IS NOT NULL"); got != "1" {
				t.Fatal("custody content/completion mismatch")
			}
		}
	})
	t.Run("invalid source routes and incomplete children roll back", func(t *testing.T) {
		r := rows[1]
		r.ID = uuid.NewString()
		r.Sequence = 100
		invalid := []string{"inventory.retail.retail.fixture.changed.v1", "inventory.commerce.inventory.fixture.changed.v1", "inventory.retail.inventory.other.v1", "inventory.retail.inventory.fixture.changed.v2", "inventory.retail.fixture.changed.v1", "inventory.retail.inventory.v1", "inventory.retail.inventory.fixture.changed.v0", "inventory.retail.inventory.fixture.changed.v01", "inventory.retail.inventory.*.v1", "inventory.retail.inventory." + strings.Repeat("a", 240) + ".v1", "inventory.retail.inventory.fixture.changed.v1.extra", "prefix.inventory.retail.inventory.fixture.changed.v1", "inventory.unknown.inventory.fixture.changed.v1"}
		for _, key := range invalid {
			r.Route = key
			s.reject(t, s.runtime, "BEGIN;"+routeSource(r, admit, "retail")+"COMMIT;", "ERROR")
			if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.outbox_messages WHERE event_id='"+r.ID+"'"); got != "0" {
				t.Fatal("invalid route left parent")
			}
		}
		r.Route = rows[1].Route
		complete := routeSource(r, admit, "retail")
		parent := strings.Split(complete, "; INSERT INTO eventstore.outbox_deliveries")[0] + ";"
		s.reject(t, s.runtime, "BEGIN;"+parent+"COMMIT;", "incomplete or conflicting")
		s.must(t, s.runtime, "BEGIN;"+complete+"COMMIT;")
		for _, q := range []string{"UPDATE eventstore.outbox_messages SET envelope='changed',envelope_hash=sha256('changed'::bytea)", "UPDATE eventstore.outbox_deliveries SET routing_key='changed'"} {
			s.reject(t, s.migration, q, "immutable")
		}
		// A repeated owner word is a legitimate nested event name when explicitly admitted.
		nested := r
		nested.ID = uuid.NewString()
		nested.Sequence++
		nested.Route = "inventory.retail.inventory.inventory.changed.v1"
		a := uuid.NewString()
		s.must(t, s.runtime, "BEGIN;"+routeAdmission(a, nested.Stream, "source-recipient", "retail", "inventory.inventory.changed.v1")+routeSource(nested, a, "retail")+"COMMIT;")
	})
	t.Run("job schema generation hash hold completeness and rollback remain enforced", func(t *testing.T) {
		target := f.store(t, "retail", true)
		target.correct(t)
		target.cutover(t)
		r := rows[1]
		a, consumer := uuid.NewString(), "fixture.projection"
		target.must(t, target.runtime, routeAdmission(a, r.Stream, "local-consumer", consumer, "inventory.fixture.changed.v1"))
		for _, key := range []string{"inventory.retail.fixture.changed.v1", "inventory.inventory.inventory.fixture.changed.v1", "inventory.retail.retail.fixture.changed.v1", "inventory.retail.inventory.fixture.changed.v0", "inventory.retail.inventory.*.v1", "inventory.retail.inventory." + strings.Repeat("a", 240) + ".v1", "inventory.retail.inventory.other.v1"} {
			bad := r
			bad.Route = key
			target.reject(t, target.runtime, "BEGIN;"+routeCustody(bad, a, consumer, "generation-1")+"COMMIT;", "ERROR")
		}
		target.reject(t, target.runtime, "BEGIN;"+routeCustody(r, a, consumer, "generation-2")+"COMMIT;", "job admission mismatch")
		complete := routeCustody(r, a, consumer, "generation-1")
		withoutJob := strings.Split(complete, "; INSERT INTO eventstore.dispatch_jobs")[0] + ";"
		target.reject(t, target.runtime, "BEGIN;"+withoutJob+"COMMIT;", "incomplete or conflicting")
		target.must(t, target.runtime, "BEGIN;"+complete+"COMMIT;")
		wrongHash := sha256.Sum256([]byte("different"))
		before := target.snapshot(t)
		target.reject(t, target.runtime, "BEGIN;"+routeComplete(r, consumer, wrongHash[:])+"COMMIT;", "matching inbox identity and bytes")
		if target.snapshot(t) != before {
			t.Fatal("failed completion leaked inbox or work")
		}
		target.must(t, target.runtime, "UPDATE eventstore.dispatch_jobs SET hold_ref='synthetic-hold'")
		target.reject(t, target.runtime, "BEGIN;"+routeComplete(r, consumer, r.Hash)+"COMMIT;", "held or quarantined")
		target.must(t, target.runtime, "UPDATE eventstore.dispatch_jobs SET hold_ref=NULL,quarantine_ref='synthetic-quarantine'")
		target.reject(t, target.runtime, "BEGIN;"+routeComplete(r, consumer, r.Hash)+"COMMIT;", "held or quarantined")
		target.must(t, target.runtime, "UPDATE eventstore.dispatch_jobs SET quarantine_ref=NULL")
		before = target.snapshot(t)
		target.must(t, target.runtime, "BEGIN;"+routeComplete(r, consumer, r.Hash)+"ROLLBACK;")
		if target.snapshot(t) != before {
			t.Fatal("explicit rollback leaked effects")
		}
		target.must(t, target.runtime, "BEGIN;"+routeComplete(r, consumer, r.Hash)+"COMMIT;")
		target.reject(t, target.runtime, "UPDATE eventstore.dispatch_jobs SET completed_at=NULL,inbox_consumer_name=NULL,inbox_event_id=NULL", "completion is immutable")
		for _, q := range []string{"UPDATE eventstore.dispatch_messages SET routing_key='changed'", "UPDATE eventstore.dispatch_messages SET envelope='changed',envelope_hash=sha256('changed'::bytea)", "UPDATE eventstore.dispatch_jobs SET generation='changed'"} {
			target.reject(t, target.migration, q, "immutable")
		}
	})
	t.Run("concurrent stream writers preserve one immutable winner", func(t *testing.T) {
		r := rows[1]
		r.ID = uuid.NewString()
		r.Sequence = 200
		other := r
		other.ID = uuid.NewString()
		start := make(chan struct{})
		outcomes := make(chan error, 2)
		var wg sync.WaitGroup
		for _, record := range []routeRecord{r, other} {
			wg.Go(func() {
				<-start
				_, err := s.f.sql(s.database, s.runtime, "BEGIN;"+routeSource(record, admit, "retail")+"COMMIT;")
				outcomes <- err
			})
		}
		close(start)
		wg.Wait()
		close(outcomes)
		success := 0
		for err := range outcomes {
			if err == nil {
				success++
			}
		}
		if success != 1 {
			t.Fatalf("concurrent winners %d", success)
		}
		if got := s.must(t, s.runtime, "SELECT count(*) FROM eventstore.outbox_messages m JOIN eventstore.outbox_deliveries d USING(event_id) WHERE m.aggregate_id='"+r.Stream+"' AND m.integration_sequence=200"); got != "1" {
			t.Fatal("incomplete concurrent winner")
		}
	})
	t.Run("empty installation prerequisites marker immutability and reverse mode", func(t *testing.T) {
		empty := f.store(t, "identity", false)
		if out, err := empty.install(f.correction); err == nil || !strings.Contains(out, "schema version 2 required") {
			t.Fatalf("missing prerequisite: %v %s", err, out)
		}
		if out, err := empty.install(f.prior); err != nil {
			t.Fatal(out, err)
		}
		empty.rejectCorrection(t, "identity mismatch", "-v", "owner_service=retail")
		empty.rejectCorrection(t, "runtime role must already exist", "-v", "runtime_role=absent_runtime")
		empty.rejectCorrection(t, "artifact digest", "-v", "prior_migration_sha256="+strings.Repeat("0", 64))
		empty.rejectCorrection(t, "artifact digest", "-v", "correction_migration_sha256=00")
		empty.rejectCorrection(t, "check constraint", "-v", "backup_ref= ")
		if out, err := empty.installAs(empty.runtime, f.correction); err == nil || !strings.Contains(out, "existing eventstore migration owner required") {
			t.Fatalf("runtime attempted installation: %v %s", err, out)
		}
		empty.correct(t)
		marker := empty.must(t, empty.runtime, "SELECT to_jsonb(m) FROM eventstore.messaging_route_compatibility m")
		before, defs := empty.snapshot(t), empty.functions(t, true)
		if out, err := empty.install(f.correction); err == nil || !strings.Contains(out, "already exists") {
			t.Fatalf("reinstall: %v %s", err, out)
		}
		if empty.snapshot(t) != before || empty.functions(t, true) != defs || empty.must(t, empty.runtime, "SELECT to_jsonb(m) FROM eventstore.messaging_route_compatibility m") != marker {
			t.Fatal("reinstall changed installation")
		}
		for _, q := range []string{"CREATE TABLE eventstore.injected(id integer)", "INSERT INTO eventstore.messaging_route_compatibility SELECT * FROM eventstore.messaging_route_compatibility", "UPDATE eventstore.messaging_route_compatibility SET route_format_version=2", "DELETE FROM eventstore.messaging_route_compatibility", "TRUNCATE eventstore.messaging_route_compatibility", routeCutover()} {
			empty.reject(t, empty.runtime, q, "permission denied")
		}
		empty.reject(t, empty.migration, "UPDATE eventstore.messaging_route_compatibility SET backup_ref='changed'", "immutable")
		empty.reject(t, empty.migration, "DELETE FROM eventstore.messaging_route_compatibility", "immutable")
		empty.cutover(t)
		empty.reject(t, empty.migration, "UPDATE eventstore.messaging_mode SET mode='legacy'", "forward custody transition")
		if got := empty.must(t, empty.runtime, "SELECT mode||':'||schema_version||':'||(SELECT legacy_rows FROM eventstore.messaging_cutovers) FROM eventstore.messaging_mode"); got != "custody:2:0" {
			t.Fatal(got)
		}
		wrong := f.store(t, "financing", true)
		wrong.must(t, wrong.migration, "CREATE TABLE eventstore.messaging_route_compatibility(migration_revision integer); INSERT INTO eventstore.messaging_route_compatibility VALUES(99)")
		defs = wrong.functions(t, true)
		if out, err := wrong.install(f.correction); err == nil || !strings.Contains(out, "marker already exists") {
			t.Fatalf("wrong existing marker: %v %s", err, out)
		}
		if wrong.functions(t, true) != defs || wrong.must(t, wrong.migration, "SELECT migration_revision FROM eventstore.messaging_route_compatibility") != "99" {
			t.Fatal("wrong marker automatically repaired")
		}
	})
	t.Run("direct default column and reachable grant escalation fails atomically", func(t *testing.T) {
		p := f.store(t, "commerce", true)
		role := "reachable_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
		p.must(t, "postgres", "CREATE ROLE "+role+"; GRANT "+role+" TO "+p.runtime+" WITH INHERIT FALSE")
		for _, probe := range []struct{ grant, revoke, want string }{
			{"GRANT UPDATE ON eventstore.outbox_messages TO " + p.runtime, "REVOKE UPDATE ON eventstore.outbox_messages FROM " + p.runtime, "table privileges"},
			{"GRANT MAINTAIN ON eventstore.outbox_messages TO " + p.runtime, "REVOKE MAINTAIN ON eventstore.outbox_messages FROM " + p.runtime, "table privileges"},
			{"GRANT UPDATE(envelope) ON eventstore.dispatch_messages TO " + p.runtime, "REVOKE UPDATE(envelope) ON eventstore.dispatch_messages FROM " + p.runtime, "column privileges"},
			{"GRANT INSERT ON eventstore.messaging_mode TO " + role, "REVOKE INSERT ON eventstore.messaging_mode FROM " + role, "evidence INSERT"},
			{"GRANT EXECUTE ON FUNCTION eventstore.messaging_job_guard() TO " + role, "REVOKE EXECUTE ON FUNCTION eventstore.messaging_job_guard() FROM " + role, "runtime may activate"},
			{"ALTER DEFAULT PRIVILEGES GRANT INSERT ON TABLES TO " + p.runtime, "ALTER DEFAULT PRIVILEGES REVOKE INSERT ON TABLES FROM " + p.runtime, "default privileges"},
			{"ALTER DEFAULT PRIVILEGES IN SCHEMA eventstore GRANT UPDATE ON TABLES TO " + role, "ALTER DEFAULT PRIVILEGES IN SCHEMA eventstore REVOKE UPDATE ON TABLES FROM " + role, "default privileges"},
			{"ALTER DEFAULT PRIVILEGES GRANT EXECUTE ON FUNCTIONS TO " + role, "ALTER DEFAULT PRIVILEGES REVOKE EXECUTE ON FUNCTIONS FROM " + role, "default privileges"},
			{"ALTER DEFAULT PRIVILEGES GRANT SELECT ON TABLES TO PUBLIC", "ALTER DEFAULT PRIVILEGES REVOKE SELECT ON TABLES FROM PUBLIC", "default privileges"},
			{"GRANT SELECT ON eventstore.messaging_mode TO PUBLIC", "REVOKE SELECT ON eventstore.messaging_mode FROM PUBLIC", "PUBLIC or delegable"},
			{"GRANT INSERT ON eventstore.outbox_messages TO " + p.runtime + " WITH GRANT OPTION", "REVOKE GRANT OPTION FOR INSERT ON eventstore.outbox_messages FROM " + p.runtime, "PUBLIC or delegable"},
		} {
			p.must(t, p.migration, probe.grant)
			p.rejectCorrection(t, probe.want)
			p.must(t, p.migration, probe.revoke)
		}
		p.must(t, "postgres", "GRANT pg_write_all_data TO "+p.runtime+" WITH INHERIT FALSE")
		p.rejectCorrection(t, "privileged ownership or membership")
		p.must(t, "postgres", "REVOKE pg_write_all_data FROM "+p.runtime)
		p.correct(t)
	})
	t.Run("retained canonical allowlists are checked and compatible custody is unchanged", func(t *testing.T) {
		for _, compatible := range []bool{false, true} {
			source := f.store(t, "inventory", true)
			source.cutover(t)
			target := f.store(t, "retail", true)
			target.cutover(t)
			r := rows[1]
			r.ID = uuid.NewString()
			a, local, consumer := uuid.NewString(), uuid.NewString(), "fixture.projection"
			// Historical 000002 reconstructs this extra owner prefix. Admitting
			// both exact types is a synthetic compatibility case, not a new grant.
			sourceAdmission := routeAdmission(a, r.Stream, "source-recipient", "retail", "inventory.inventory.fixture.changed.v1")
			localAdmission := routeAdmission(local, r.Stream, "local-consumer", consumer, "inventory.inventory.fixture.changed.v1")
			if compatible {
				sourceAdmission = strings.Replace(sourceAdmission, `["inventory.inventory.fixture.changed.v1"]`, `["inventory.inventory.fixture.changed.v1","inventory.fixture.changed.v1"]`, 1)
				localAdmission = strings.Replace(localAdmission, `["inventory.inventory.fixture.changed.v1"]`, `["inventory.inventory.fixture.changed.v1","inventory.fixture.changed.v1"]`, 1)
			}
			source.must(t, source.runtime, "BEGIN;"+sourceAdmission+routeSource(r, a, "retail")+"COMMIT;")
			target.must(t, target.runtime, "BEGIN;"+localAdmission+routeCustody(r, local, consumer, "generation-1")+routeComplete(r, consumer, r.Hash)+"COMMIT;")
			if !compatible {
				source.rejectCorrection(t, "incompatible retained delivery route or schema admission")
				target.rejectCorrection(t, "incompatible retained job schema admission")
			} else {
				for _, store := range []*routeStore{source, target} {
					before := store.snapshot(t)
					store.correct(t)
					if store.snapshot(t) != before {
						t.Fatal("compatible custody correction changed retained rows or prior cutover")
					}
				}
			}
		}
	})
	t.Run("incompatible historical v2 custody and legacy routes are never repaired", func(t *testing.T) {
		bad := f.store(t, "inventory", true)
		r := rows[1]
		r.ID = uuid.NewString()
		r.Route = "inventory.retail.fixture.changed.v1"
		a := uuid.NewString()
		bad.cutover(t)
		bad.must(t, bad.runtime, "BEGIN;"+routeAdmission(a, r.Stream, "source-recipient", "retail", "inventory.fixture.changed.v1")+routeSource(r, a, "retail")+"COMMIT;")
		bad.rejectCorrection(t, "incompatible retained delivery")
		custody := f.store(t, "retail", true)
		custody.cutover(t)
		local := uuid.NewString()
		consumer := "fixture.projection"
		custody.must(t, custody.runtime, "BEGIN;"+routeAdmission(local, r.Stream, "local-consumer", consumer, "inventory.fixture.changed.v1")+routeCustody(r, local, consumer, "generation-1")+"COMMIT;")
		custody.rejectCorrection(t, "incompatible retained custody")
		legacy := f.store(t, "inventory", true)
		legacy.must(t, legacy.runtime, fmt.Sprintf("INSERT INTO eventstore.outbox(event_id,aggregate_type,aggregate_id,integration_sequence,envelope,envelope_hash,exchange,routing_key) VALUES('%s','fixture','%s',1,'synthetic',sha256('synthetic'::bytea),'justix.integration.v1','inventory.retail.fixture.changed.v1')", uuid.NewString(), uuid.NewString()))
		legacy.rejectCorrection(t, "incompatible retained legacy")
	})
	t.Run("bounded lock wait rolls back and simultaneous installs have one winner", func(t *testing.T) {
		p := f.store(t, "insurance", true)
		// A runtime transaction holds the same old-writer table lock the migration
		// must acquire first; no timing assumption about PostgreSQL startup is used.
		tx := p.db.Begin()
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		if err := tx.Exec("LOCK TABLE eventstore.outbox IN ROW EXCLUSIVE MODE").Error; err != nil {
			t.Fatal(err)
		}
		p.rejectCorrection(t, "lock timeout")
		if err := tx.Rollback().Error; err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		outcomes := make(chan error, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Go(func() { <-start; _, err := p.install(f.correction); outcomes <- err })
		}
		close(start)
		wg.Wait()
		close(outcomes)
		success := 0
		for err := range outcomes {
			if err == nil {
				success++
			}
		}
		if success != 1 {
			t.Fatalf("installation winners %d", success)
		}
		if got := p.must(t, p.runtime, "SELECT count(*) FROM eventstore.messaging_route_compatibility"); got != "1" {
			t.Fatal("missing exact singleton")
		}
	})
}
