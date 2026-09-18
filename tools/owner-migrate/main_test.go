package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"justixauto/pkg/persistence"
)

func runnerFixture(t *testing.T) (fixture, Config) {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ownerRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{t: t, root: root, ownerRoot: ownerRoot}
	f.buildBundle()
	return f, f.config(17, false)
}

func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestVerifiedSourceImmutableAndUpwardOnly(t *testing.T) {
	f, cfg := runnerFixture(t)
	src, err := newVerifiedSource(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if first, err := src.First(); err != nil || first != 1 {
		t.Fatalf("first: %d %v", first, err)
	}
	if next, err := src.Next(1); err != nil || next != 12 {
		t.Fatalf("next: %d %v", next, err)
	}
	if next, err := src.Next(12); err != nil || next != 17 {
		t.Fatalf("next: %d %v", next, err)
	}
	if _, err := src.Prev(17); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("down migration exposed: %v", err)
	}
	if _, _, err := src.ReadDown(17); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("down body exposed: %v", err)
	}
	r, _, err := src.ReadUp(12)
	if err != nil {
		t.Fatal(err)
	}
	before, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.ownerRoot, f.spec.Artifacts[1].Identity.Filename)
	if err := os.WriteFile(path, []byte("changed after verified source construction"), 0600); err != nil {
		t.Fatal(err)
	}
	r, _, err = src.ReadUp(12)
	if err != nil {
		t.Fatal(err)
	}
	after, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("verified source retained caller/filesystem mutation")
	}
	if _, err := newVerifiedSource(cfg); err == nil {
		t.Fatal("changed artifact passed complete source preflight")
	}
	if err := src.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := src.ReadUp(12); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closed source remained readable: %v", err)
	}
}

func TestBundleAndAttemptReportsAreExact(t *testing.T) {
	_, cfg := runnerFixture(t)
	s := cfg.Profile.Specification()
	bundle := Bundle{FormatRevision: 1, Profile: s, ProfileSHA256: digest(cfg.Profile.Digest()), RepoRoot: cfg.RepoRoot, OwnerRoot: cfg.OwnerRoot, ProvenanceRef: cfg.ProvenanceRef}
	b, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(realTempDir(t), "bundle.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadBundle(path)
	if err != nil || loaded.Profile.Digest() != cfg.Profile.Digest() {
		t.Fatalf("load exact bundle: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded["profile_sha256"] = digest(testHash("different"))
	b, _ = json.Marshal(decoded)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadBundle(path); err == nil {
		t.Fatal("bundle/profile digest mismatch accepted")
	}

	a := Attempt{version: 12, request: s.Baseline.RequestID, profile: cfg.Profile.Digest(), sql: s.Artifacts[1].Identity.SHA256, provenance: cfg.ProvenanceRef}
	reportPath := filepath.Join(realTempDir(t), "attempt.json")
	store := fileReportStore{reportPath}
	if err := store.Save(reportFor(cfg, a, StatusPending)); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0600 {
		t.Fatalf("private report mode: %v", st.Mode())
	}
	loadedAttempt, err := loadAttempt(reportPath, cfg)
	if err != nil || loadedAttempt != a {
		t.Fatalf("load exact attempt: %#v %v", loadedAttempt, err)
	}
	if err := requireResolvedReport(reportPath, cfg); !errors.Is(err, errRetainedIncomplete) {
		t.Fatalf("unresolved attempt could be overwritten: %v", err)
	}
	if err := store.Save(reportFor(cfg, a, StatusRecordedCompletion)); err != nil {
		t.Fatal(err)
	}
	if err := requireResolvedReport(reportPath, cfg); err != nil {
		t.Fatalf("recorded completion did not release report path: %v", err)
	}

	changed := cfg
	changed.Profile, err = persistence.NewProfile(func() persistence.Specification {
		copy := s
		copy.Baseline.ApprovalRef = "different-approved-profile"
		return copy
	}())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadAttempt(reportPath, changed); err == nil {
		t.Fatal("attempt from another profile accepted")
	}
}

func TestMalformedResolvedReportCannotReleasePath(t *testing.T) {
	_, cfg := runnerFixture(t)
	s := cfg.Profile.Specification()
	base := AttemptReport{
		FormatRevision: 1,
		Status:         StatusRecordedCompletion,
		Owner:          s.Owner,
		Database:       s.Database,
		Version:        s.Head,
		RequestID:      s.Baseline.RequestID.String(),
		ProfileSHA256:  digest(cfg.Profile.Digest()),
		SQLSHA256:      digest(s.Artifacts[len(s.Artifacts)-1].Identity.SHA256),
		ProvenanceRef:  cfg.ProvenanceRef,
	}
	cases := map[string]AttemptReport{}
	wrongVersion := base
	wrongVersion.Version = 999
	cases["recorded wrong version"] = wrongVersion
	missingRequest := base
	missingRequest.RequestID = ""
	cases["recorded missing request"] = missingRequest
	wrongSQL := base
	wrongSQL.SQLSHA256 = digest(testHash("wrong SQL"))
	cases["recorded wrong SQL"] = wrongSQL
	wrongProvenance := base
	wrongProvenance.ProvenanceRef = ""
	cases["recorded missing provenance"] = wrongProvenance
	wrongNoOp := base
	wrongNoOp.Status = StatusVerifiedNoOp
	wrongNoOp.Version = s.Head - 1
	wrongNoOp.RequestID = ""
	wrongNoOp.SQLSHA256 = ""
	wrongNoOp.ProvenanceRef = ""
	cases["no-op wrong head"] = wrongNoOp

	for name, report := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(realTempDir(t), "report.json")
			body, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			if err := requireResolvedReport(path, cfg); err == nil {
				t.Fatalf("malformed resolved report released the evidence path: %+v", report)
			}
		})
	}
}

type memoryReportStore struct {
	mu      sync.Mutex
	reports []AttemptReport
}

func (s *memoryReportStore) Save(r AttemptReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports = append(s.reports, r)
	return nil
}

func (s *memoryReportStore) last() AttemptReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.reports) == 0 {
		return AttemptReport{}
	}
	return s.reports[len(s.reports)-1]
}

func TestRunnerActualEnginePhasesUpgradeNoOpAndConcurrency(t *testing.T) {
	f := startFixture(t)
	open := func(ctx context.Context) (*pgx.Conn, error) {
		cfg, err := pgx.ParseConfig(fmt.Sprintf("host=127.0.0.1 port=%s user=justix_identity password=%s dbname=justix_identity sslmode=disable", f.port, f.password))
		if err != nil {
			return nil, err
		}
		cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		return pgx.ConnectConfig(ctx, cfg)
	}
	store := &memoryReportStore{}
	psql, err := exec.LookPath("psql")
	if err != nil {
		t.Fatal(err)
	}
	dsn := fmt.Sprintf("host=127.0.0.1 port=%s user=justix_identity password=%s dbname=justix_identity sslmode=disable", f.port, f.password)
	// The fixture bootstrap normally installs the shared base. Remove only that
	// disposable schema so this test exercises the runner's real first phase.
	f.must(`DROP SCHEMA eventstore CASCADE`)
	holder := f.connect(nil)
	if _, err := holder.Exec(context.Background(), `SELECT pg_advisory_lock($1)`, OwnerLockKey("justix_identity")); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := runTemplate(context.Background(), psql, dsn, f.config(1, true), phaseSharedBase); err == nil || time.Since(started) < 4*time.Second || time.Since(started) > 7*time.Second {
		t.Fatalf("shared phase lock was not bounded: %v %s", err, time.Since(started))
	}
	if got := f.must(`SELECT to_regnamespace('eventstore') IS NULL`); got != "t" {
		t.Fatal("contended shared phase changed the database")
	}
	if _, err := holder.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, OwnerLockKey("justix_identity")); err != nil {
		t.Fatal(err)
	}
	if err := runTemplate(context.Background(), psql, dsn, f.config(1, true), phaseSharedBase); err != nil {
		t.Fatal(err)
	}
	base, err := runPrivate(context.Background(), open, f.config(1, true), store)
	if err != nil || base.Status != StatusApplied {
		t.Fatalf("actual fresh v1: %#v %v", base, err)
	}
	if err := runTemplate(context.Background(), psql, dsn, f.config(1, true), phaseHistory); err != nil {
		t.Fatal(err)
	}

	before := f.snapshot()
	results := make(chan RunResult, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			result, err := runPrivate(context.Background(), open, f.config(12, false), store)
			results <- result
			errs <- err
		}()
	}
	statuses := map[RunStatus]int{}
	for i := 0; i < 2; i++ {
		result := <-results
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		statuses[result.Status]++
	}
	if statuses[StatusApplied] != 1 || statuses[StatusVerifiedNoOp] != 1 {
		t.Fatalf("concurrent outcomes: %#v", statuses)
	}
	if got := f.must(`SELECT count(*) FROM owner_migrations.artifacts WHERE version=12`); got != "1" {
		t.Fatalf("duplicate receipt after concurrent run: %s", got)
	}
	if f.snapshot() == before {
		t.Fatal("upgrade did not append version 12")
	}
	noOp, err := runPrivate(context.Background(), open, f.config(12, false), store)
	if err != nil || noOp.Status != StatusVerifiedNoOp {
		t.Fatalf("verified no-op: %#v %v", noOp, err)
	}
	upgrade, err := runPrivate(context.Background(), open, f.config(17, false), store)
	if err != nil || upgrade.Status != StatusApplied {
		t.Fatalf("retained 12 to 17 upgrade: %#v %v", upgrade, err)
	}
	if got := f.must(`SELECT string_agg(version::text,',' ORDER BY version) FROM owner_migrations.artifacts`); got != "1,12,17" {
		t.Fatalf("complete retained history: %s", got)
	}
}

func TestRunnerFailedDDLRetainsDirtyEvidenceWithoutReplay(t *testing.T) {
	f := startFixture(t)
	f.bootstrap()
	f.writeArtifact(1, `BEGIN;
CREATE SCHEMA synthetic_feature12;
SELECT 1/0;
COMMIT;`)
	cfg := f.config(12, false)
	open := func(ctx context.Context) (*pgx.Conn, error) {
		parsed, err := pgx.ParseConfig(fmt.Sprintf("host=127.0.0.1 port=%s user=justix_identity password=%s dbname=justix_identity sslmode=disable", f.port, f.password))
		if err != nil {
			return nil, err
		}
		parsed.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		return pgx.ConnectConfig(ctx, parsed)
	}
	store := &memoryReportStore{}
	result, err := runPrivate(context.Background(), open, cfg, store)
	if err == nil || !errors.Is(err, errRetainedIncomplete) || result.Status != StatusRetainedIncomplete {
		t.Fatalf("failed DDL was not retained as incomplete: %#v %v", result, err)
	}
	if got := f.must(`SELECT version||':'||dirty FROM public.schema_migrations`); got != "12:true" {
		t.Fatalf("dirty evidence was changed: %s", got)
	}
	if got := f.must(`SELECT count(*) FROM owner_migrations.artifacts`); got != "1" {
		t.Fatalf("failed artifact receipt was invented: %s", got)
	}
	if got := f.must(`SELECT to_regnamespace('synthetic_feature12') IS NULL`); got != "t" {
		t.Fatal("failed DDL transaction was not rolled back")
	}
	if store.last().Status != StatusRetainedIncomplete {
		t.Fatalf("durable report inferred success: %#v", store.last())
	}
}

func TestRunnerUnknownFinalCommitReconcilesAuthoritatively(t *testing.T) {
	f := startFixture(t)
	f.bootstrap()
	fault := &wireFault{}
	fault.arm("COMMIT", 3)
	connections := 0
	open := func(context.Context) (*pgx.Conn, error) {
		connections++
		if connections == 1 {
			return f.connect(fault), nil
		}
		return f.connect(nil), nil
	}
	store := &memoryReportStore{}
	result, err := runPrivate(context.Background(), open, f.config(12, false), store)
	if err != nil {
		t.Fatal(err)
	}
	if !fault.wasHit() || connections != 2 || result.Status != StatusRecordedCompletion {
		t.Fatalf("unknown outcome was not freshly reconciled: hit=%v connections=%d result=%#v", fault.wasHit(), connections, result)
	}
	if store.last().Status != StatusRecordedCompletion {
		t.Fatalf("durable report was not reconciled: %#v", store.last())
	}
	if got := f.must(`SELECT version||':'||dirty FROM public.schema_migrations`); got != "12:false" {
		t.Fatalf("authoritative ledger: %s", got)
	}
}

type intermediateMutatingReportStore struct {
	t       *testing.T
	path    string
	mutated bool
	reports []AttemptReport
}

func (s *intermediateMutatingReportStore) Save(report AttemptReport) error {
	s.reports = append(s.reports, report)
	if !s.mutated && report.Status == StatusRecordedCompletion && report.Version == 12 {
		if err := os.WriteFile(s.path, []byte("changed before the next artifact dirty transition"), 0600); err != nil {
			s.t.Fatal(err)
		}
		s.mutated = true
	}
	return nil
}

func TestRunnerIntermediateCompletionIsNotOverallSuccess(t *testing.T) {
	f := startFixture(t)
	f.bootstrap()
	cfg := f.config(17, false)
	path := filepath.Join(f.ownerRoot, f.spec.Artifacts[2].Identity.Filename)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store := &intermediateMutatingReportStore{t: t, path: path}
	connections := 0
	open := func(ctx context.Context) (*pgx.Conn, error) {
		connections++
		if connections == 2 {
			if !store.mutated {
				t.Fatal("runner opened reconciliation before the next-artifact failure")
			}
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
		}
		parsed, err := pgx.ParseConfig(fmt.Sprintf("host=127.0.0.1 port=%s user=justix_identity password=%s dbname=justix_identity sslmode=disable", f.port, f.password))
		if err != nil {
			return nil, err
		}
		parsed.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		return pgx.ConnectConfig(ctx, parsed)
	}

	result, runErr := runPrivate(context.Background(), open, cfg, store)
	if runErr == nil || !errors.Is(runErr, errRequestedHeadNotReached) {
		t.Fatalf("intermediate completion satisfied requested head: result=%#v error=%v", result, runErr)
	}
	if result.Status != StatusRecordedCompletion || result.Version != 12 {
		t.Fatalf("authoritative intermediate outcome was lost: %#v", result)
	}
	if got := f.must(`SELECT version||':'||dirty FROM public.schema_migrations`); got != "12:false" {
		t.Fatalf("unexpected intermediate ledger: %s", got)
	}
	if got := f.must(`SELECT string_agg(version::text,',' ORDER BY version) FROM owner_migrations.artifacts`); got != "1,12" {
		t.Fatalf("unexpected intermediate receipts: %s", got)
	}
	if got := f.must(`SELECT to_regnamespace('synthetic_feature17') IS NULL`); got != "t" {
		t.Fatal("requested head feature unexpectedly exists")
	}
}
