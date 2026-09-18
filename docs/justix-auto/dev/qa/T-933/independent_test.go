package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestQA933IntermediateCompletionIsNotOverallSuccess exercises one actual
// migrate.Up call with more than one pending upward artifact. After version 12
// commits, the next artifact's on-disk preflight is made to fail. The file is
// restored only when the runner opens its fresh reconciliation connection.
// Reconciliation may prove that version 12 completed, but the requested head 17
// is still absent and the overall command must not report success.
func TestQA933IntermediateCompletionIsNotOverallSuccess(t *testing.T) {
	f := startFixture(t)
	f.bootstrap()
	cfg := f.config(17, false)
	path := filepath.Join(f.ownerRoot, f.spec.Artifacts[2].Identity.Filename)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	store := &qaMutatingReportStore{t: t, path: path}
	connections := 0
	open := func(ctx context.Context) (*pgx.Conn, error) {
		connections++
		if connections == 2 {
			if !store.mutated {
				t.Fatal("runner opened reconciliation before the injected next-artifact failure")
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
	if runErr == nil {
		t.Fatalf("runner reported overall success after only an intermediate artifact completed: result=%#v ledger=%s receipts=%s feature17_missing=%s reports=%s",
			result,
			f.must(`SELECT version||':'||dirty FROM public.schema_migrations`),
			f.must(`SELECT string_agg(version::text,',' ORDER BY version) FROM owner_migrations.artifacts`),
			f.must(`SELECT to_regnamespace('synthetic_feature17') IS NULL`),
			store.json())
	}
	if got := f.must(`SELECT version||':'||dirty FROM public.schema_migrations`); got != "12:false" {
		t.Fatalf("unexpected retained intermediate ledger: %s", got)
	}
	if got := f.must(`SELECT string_agg(version::text,',' ORDER BY version) FROM owner_migrations.artifacts`); got != "1,12" {
		t.Fatalf("unexpected retained receipts: %s", got)
	}
}

type qaMutatingReportStore struct {
	t       *testing.T
	path    string
	mutated bool
	reports []AttemptReport
}

func (s *qaMutatingReportStore) Save(report AttemptReport) error {
	s.reports = append(s.reports, report)
	if !s.mutated && report.Status == StatusRecordedCompletion && report.Version == 12 {
		if err := os.WriteFile(s.path, []byte("changed before the next artifact dirty transition"), 0600); err != nil {
			s.t.Fatal(err)
		}
		s.mutated = true
	}
	return nil
}

func (s *qaMutatingReportStore) json() string {
	b, _ := json.Marshal(s.reports)
	return string(b)
}

// Resolved evidence must itself have the exact status-specific shape before a
// later invocation is permitted to replace it. These malformed files share the
// right owner/database/profile and therefore isolate that validation boundary.
func TestQA933MalformedResolvedReportCannotReleasePath(t *testing.T) {
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
