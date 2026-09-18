package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"justixauto/pkg/persistence"
)

const bundleFormatRevision = 1

var errRunnerConfig = errors.New("invalid owner migration runner configuration")
var errRetainedIncomplete = errors.New("owner migration retained an incomplete dirty installation")

// Bundle is a release-owned, digest-bound input. It contains no connection
// string or credential. The CLI accepts only this complete profile and never
// discovers migrations by scanning a directory.
type Bundle struct {
	FormatRevision uint32                    `json:"format_revision"`
	Profile        persistence.Specification `json:"profile"`
	ProfileSHA256  string                    `json:"profile_sha256"`
	RepoRoot       string                    `json:"repo_root"`
	OwnerRoot      string                    `json:"owner_root"`
	ProvenanceRef  string                    `json:"provenance_ref"`
	FreshBootstrap bool                      `json:"fresh_bootstrap"`
}

func (b Bundle) config() (Config, error) {
	if b.FormatRevision != bundleFormatRevision || !filepath.IsAbs(b.RepoRoot) || !filepath.IsAbs(b.OwnerRoot) || !validReference(b.ProvenanceRef) {
		return Config{}, errRunnerConfig
	}
	p, err := persistence.NewProfile(b.Profile)
	if err != nil {
		return Config{}, errors.Join(errRunnerConfig, err)
	}
	want, err := hex.DecodeString(b.ProfileSHA256)
	profileDigest := p.Digest()
	if err != nil || len(want) != sha256.Size || !bytes.Equal(want, profileDigest[:]) {
		return Config{}, errRunnerConfig
	}
	cfg := Config{Profile: p, RepoRoot: filepath.Clean(b.RepoRoot), OwnerRoot: filepath.Clean(b.OwnerRoot), ProvenanceRef: b.ProvenanceRef, FreshBootstrap: b.FreshBootstrap}
	s := p.Specification()
	if s.Shared.Mode != "legacy" || len(s.Shared.Artifacts) > len(sharedArtifacts) || (cfg.FreshBootstrap && (s.Head != 1 || len(s.Artifacts) != 1 || s.Baseline.Kind != "verified-installation")) {
		return Config{}, errors.Join(errRunnerConfig, ErrUnsupported)
	}
	return cfg, nil
}

func loadBundle(name string) (Config, error) {
	if !filepath.IsAbs(name) {
		return Config{}, errRunnerConfig
	}
	b, err := readRegularNoSymlink(name)
	if err != nil {
		return Config{}, errors.Join(errRunnerConfig, err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var bundle Bundle
	if err := dec.Decode(&bundle); err != nil {
		return Config{}, errors.Join(errRunnerConfig, err)
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return Config{}, errRunnerConfig
	}
	return bundle.config()
}

func readRegularNoSymlink(name string) ([]byte, error) {
	clean := filepath.Clean(name)
	for at := clean; ; at = filepath.Dir(at) {
		st, err := os.Lstat(at)
		if err != nil || st.Mode()&os.ModeSymlink != 0 || (at == clean && !st.Mode().IsRegular()) {
			return nil, errRunnerConfig
		}
		if filepath.Dir(at) == at {
			break
		}
	}
	return os.ReadFile(clean)
}

func requireDirectoryNoSymlink(name string) error {
	clean := filepath.Clean(name)
	for at := clean; ; at = filepath.Dir(at) {
		st, err := os.Lstat(at)
		if err != nil || st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			return errRunnerConfig
		}
		if filepath.Dir(at) == at {
			return nil
		}
	}
}

// verifiedSource owns immutable copies of all verified upward artifacts. It has
// no URL registration, directory discovery, down path or caller-owned buffers.
type verifiedSource struct {
	mu       sync.RWMutex
	versions []uint
	names    map[uint]string
	bodies   map[uint][]byte
	closed   bool
}

var _ source.Driver = (*verifiedSource)(nil)

func newVerifiedSource(cfg Config) (*verifiedSource, error) {
	if err := cfg.Profile.VerifyFiles(cfg.OwnerRoot); err != nil {
		return nil, err
	}
	s := cfg.Profile.Specification()
	out := &verifiedSource{names: make(map[uint]string, len(s.Artifacts)), bodies: make(map[uint][]byte, len(s.Artifacts))}
	for _, a := range s.Artifacts {
		v := uint(a.Identity.Version)
		if int64(v) != a.Identity.Version {
			return nil, errRunnerConfig
		}
		body, err := readOriginal(cfg.OwnerRoot, a.Identity.Filename, a.Identity.SHA256)
		if err != nil {
			return nil, err
		}
		out.versions = append(out.versions, v)
		out.names[v] = a.Identity.Filename
		out.bodies[v] = append([]byte(nil), body...)
	}
	return out, nil
}

func verifyRunnerBundle(cfg Config) error {
	if err := cfg.Profile.VerifyFiles(cfg.OwnerRoot); err != nil {
		return err
	}
	s := cfg.Profile.Specification()
	if len(s.Shared.Artifacts) > len(sharedArtifacts) {
		return ErrUnsupported
	}
	for i, a := range s.Shared.Artifacts {
		if a.Filename != sharedArtifacts[i].name || digest(a.SHA256) != sharedArtifacts[i].hash {
			return ErrUnsupported
		}
		if _, err := readOriginal(cfg.RepoRoot, a.Filename, a.SHA256); err != nil {
			return err
		}
	}
	_, err := readOriginal(cfg.RepoRoot, "pkg/eventstore/owner_history.sql", s.HistorySHA256)
	return err
}

func (*verifiedSource) Open(string) (source.Driver, error) { return nil, ErrUnsupported }
func (s *verifiedSource) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}
func (s *verifiedSource) First() (uint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed || len(s.versions) == 0 {
		return 0, os.ErrNotExist
	}
	return s.versions[0], nil
}
func (s *verifiedSource) Next(version uint) (uint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return 0, os.ErrNotExist
	}
	for _, candidate := range s.versions {
		if candidate > version {
			return candidate, nil
		}
	}
	return 0, os.ErrNotExist
}
func (*verifiedSource) Prev(uint) (uint, error) { return 0, os.ErrNotExist }
func (s *verifiedSource) ReadUp(version uint) (io.ReadCloser, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	body, ok := s.bodies[version]
	if s.closed || !ok {
		return nil, "", os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), body...))), s.names[version], nil
}
func (*verifiedSource) ReadDown(uint) (io.ReadCloser, string, error) {
	return nil, "", os.ErrNotExist
}

type RunStatus string

const (
	StatusApplied            RunStatus = "applied"
	StatusVerifiedNoOp       RunStatus = "verified-no-op"
	StatusRecordedCompletion RunStatus = "recorded-completion"
	StatusRetainedIncomplete RunStatus = "retained-incomplete"
	StatusPending            RunStatus = "pending"
)

type AttemptReport struct {
	FormatRevision uint32    `json:"format_revision"`
	Status         RunStatus `json:"status"`
	Owner          string    `json:"owner"`
	Database       string    `json:"database"`
	Version        int64     `json:"version"`
	RequestID      string    `json:"request_id,omitempty"`
	ProfileSHA256  string    `json:"profile_sha256"`
	SQLSHA256      string    `json:"sql_sha256,omitempty"`
	ProvenanceRef  string    `json:"provenance_ref,omitempty"`
}

type reportStore interface {
	Save(AttemptReport) error
}

type fileReportStore struct{ path string }

func (s fileReportStore) Save(report AttemptReport) error {
	if !filepath.IsAbs(s.path) {
		return errRunnerConfig
	}
	dir := filepath.Dir(filepath.Clean(s.path))
	if err := requireDirectoryNoSymlink(dir); err != nil {
		return errRunnerConfig
	}
	b, err := json.Marshal(report)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(dir, ".owner-migration-report-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, filepath.Clean(s.path)); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil || closeErr != nil {
		return errors.Join(syncErr, closeErr)
	}
	ok = true
	return nil
}

func reportFor(cfg Config, a Attempt, status RunStatus) AttemptReport {
	s := cfg.Profile.Specification()
	r := AttemptReport{FormatRevision: 1, Status: status, Owner: s.Owner, Database: s.Database, Version: a.version, ProfileSHA256: digest(cfg.Profile.Digest())}
	if a.request != uuid.Nil {
		r.RequestID = a.request.String()
		r.SQLSHA256 = digest(a.sql)
		r.ProvenanceRef = a.provenance
	}
	return r
}

// reportingDriver durably records the exact request after the dirty transaction
// and before the engine can receive source bytes. A failed report write leaves
// the ledger dirty and prevents Run; it never silently proceeds without evidence.
type reportingDriver struct {
	*Driver
	store reportStore
}

var _ database.Driver = (*reportingDriver)(nil)

func (d *reportingDriver) SetVersion(version int, dirty bool) error {
	if err := d.Driver.SetVersion(version, dirty); err != nil {
		return err
	}
	if d.store == nil {
		return nil
	}
	status := StatusRecordedCompletion
	if dirty {
		status = StatusPending
	}
	if err := d.store.Save(reportFor(d.cfg, d.LastAttempt(), status)); err != nil {
		if dirty {
			return errors.Join(errRunnerConfig, err)
		}
		return errors.Join(ErrPoisoned, err)
	}
	return nil
}

type connectionFactory func(context.Context) (*pgx.Conn, error)

type RunResult struct {
	Status  RunStatus
	Version int64
	Request uuid.UUID
}

// runPrivate executes only the sealed upward source through the actual migrate
// engine. Every error after a durable non-bootstrap attempt closes the original
// connection and performs fresh, read-only, lock-bounded reconciliation.
func runPrivate(ctx context.Context, open connectionFactory, cfg Config, store reportStore) (RunResult, error) {
	src, err := newVerifiedSource(cfg)
	if err != nil {
		return RunResult{}, err
	}
	conn, err := open(ctx)
	if err != nil {
		_ = src.Close()
		return RunResult{}, err
	}
	driver, err := NewDriver(ctx, conn, cfg)
	if err != nil {
		_ = conn.Close(context.Background())
		_ = src.Close()
		return RunResult{}, err
	}
	reported := &reportingDriver{Driver: driver, store: store}
	engine, err := migrate.NewWithInstance("verified-owner-bundle", src, "justixauto-owner-pgx", reported)
	if err != nil {
		_ = driver.Close()
		_ = src.Close()
		return RunResult{}, err
	}
	engine.LockTimeout = LockBound + 5*time.Second
	runErr := engine.Up()
	a := driver.LastAttempt()
	sourceCloseErr, databaseCloseErr := engine.Close()
	runErr = errors.Join(runErr, sourceCloseErr, databaseCloseErr)
	if runErr == nil || (errors.Is(runErr, migrate.ErrNoChange) && sourceCloseErr == nil && databaseCloseErr == nil) {
		status := StatusApplied
		if errors.Is(runErr, migrate.ErrNoChange) {
			status = StatusVerifiedNoOp
		}
		if store != nil && status == StatusVerifiedNoOp {
			verified := Attempt{version: cfg.Profile.Specification().Head, profile: cfg.Profile.Digest()}
			if err := store.Save(reportFor(cfg, verified, status)); err != nil {
				return RunResult{}, err
			}
		}
		return RunResult{Status: status, Version: cfg.Profile.Specification().Head, Request: a.request}, nil
	}
	if cfg.FreshBootstrap || a.request == uuid.Nil {
		return RunResult{}, runErr
	}
	fresh, openErr := open(ctx)
	if openErr != nil {
		return RunResult{}, errors.Join(runErr, openErr)
	}
	reconciliation, reconcileErr := Reconcile(ctx, fresh, cfg, a)
	if reconcileErr != nil {
		return RunResult{}, errors.Join(runErr, reconcileErr)
	}
	status := StatusRetainedIncomplete
	if reconciliation == RecordedCompletion {
		status = StatusRecordedCompletion
	}
	if store != nil {
		if saveErr := store.Save(reportFor(cfg, a, status)); saveErr != nil {
			return RunResult{}, errors.Join(runErr, saveErr)
		}
	}
	result := RunResult{Status: status, Version: a.version, Request: a.request}
	if reconciliation == RecordedCompletion {
		return result, nil
	}
	return result, errors.Join(errRetainedIncomplete, runErr)
}

func decodeReport(b []byte) (AttemptReport, error) {
	var r AttemptReport
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return r, err
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return AttemptReport{}, errRunnerConfig
	}
	return r, nil
}

func loadAttempt(path string, cfg Config) (Attempt, error) {
	b, err := readRegularNoSymlink(path)
	if err != nil {
		return Attempt{}, err
	}
	r, err := decodeReport(b)
	if err != nil {
		return Attempt{}, err
	}
	s := cfg.Profile.Specification()
	request, requestErr := uuid.Parse(r.RequestID)
	profileBytes, profileErr := hex.DecodeString(r.ProfileSHA256)
	sqlBytes, sqlErr := hex.DecodeString(r.SQLSHA256)
	if r.FormatRevision != 1 || r.Owner != s.Owner || r.Database != s.Database || request == uuid.Nil || requestErr != nil || profileErr != nil || sqlErr != nil || len(profileBytes) != 32 || len(sqlBytes) != 32 || r.ProfileSHA256 != digest(cfg.Profile.Digest()) || !validReference(r.ProvenanceRef) || (r.Status != StatusPending && r.Status != StatusRetainedIncomplete && r.Status != StatusRecordedCompletion) {
		return Attempt{}, errRunnerConfig
	}
	var profileDigest, sqlDigest persistence.Digest
	copy(profileDigest[:], profileBytes)
	copy(sqlDigest[:], sqlBytes)
	return Attempt{version: r.Version, request: request, profile: profileDigest, sql: sqlDigest, provenance: r.ProvenanceRef}, nil
}

func requireResolvedReport(path string, cfg Config) error {
	if !filepath.IsAbs(path) {
		return errRunnerConfig
	}
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	b, err := readRegularNoSymlink(path)
	if err != nil {
		return err
	}
	r, err := decodeReport(b)
	if err != nil {
		return err
	}
	s := cfg.Profile.Specification()
	if r.FormatRevision != 1 || r.Owner != s.Owner || r.Database != s.Database || r.ProfileSHA256 != digest(cfg.Profile.Digest()) {
		return errRunnerConfig
	}
	switch r.Status {
	case StatusRecordedCompletion, StatusVerifiedNoOp:
		return nil
	case StatusPending, StatusRetainedIncomplete:
		return errRetainedIncomplete
	default:
		return errRunnerConfig
	}
}

type templatePhase string

const (
	phaseSharedBase templatePhase = "shared-base"
	phaseHistory    templatePhase = "history"
)

// runTemplate executes the exact original psql template on one psql session.
// That same session acquires the owner lock before sending the template and
// releases it afterwards; an error closes the session and releases the lock.
func runTemplate(ctx context.Context, psqlPath, dsn string, cfg Config, phase templatePhase) error {
	if !cfg.FreshBootstrap || cfg.Profile.Specification().Head != 1 {
		return errRunnerConfig
	}
	if err := verifyRunnerBundle(cfg); err != nil {
		return err
	}
	if filepath.Base(psqlPath) != "psql" {
		return errRunnerConfig
	}
	s := cfg.Profile.Specification()
	var name string
	vars := map[string]string{"owner_service": s.Owner, "runtime_role": s.RuntimeRole}
	switch phase {
	case phaseSharedBase:
		name = s.Shared.Artifacts[0].Filename
		if name != sharedArtifacts[0].name || digest(s.Shared.Artifacts[0].SHA256) != sharedArtifacts[0].hash {
			return errRunnerConfig
		}
	case phaseHistory:
		name = "pkg/eventstore/owner_history.sql"
		base := s.Artifacts[0]
		vars["template_sha256"] = digest(s.HistorySHA256)
		vars["base_schema_sha256"] = digest(s.Shared.Artifacts[0].SHA256)
		vars["base_artifact_sha256"] = digest(base.Identity.SHA256)
		vars["base_manifest_sha256"] = digest(base.ManifestSHA256)
		vars["profile_sha256"] = digest(cfg.Profile.Digest())
		vars["baseline_kind"] = s.Baseline.Kind
		vars["evidence_ref"] = s.Baseline.EvidenceRef
		vars["approval_ref"] = s.Baseline.ApprovalRef
		vars["backup_ref"] = s.Baseline.BackupRef
		vars["stopped_runtimes_ref"] = s.Baseline.StoppedRuntimesRef
		vars["request_id"] = s.Baseline.RequestID.String()
	default:
		return errRunnerConfig
	}
	want := s.HistorySHA256
	if phase == phaseSharedBase {
		want = s.Shared.Artifacts[0].SHA256
	}
	body, err := readOriginal(cfg.RepoRoot, name, want)
	if err != nil {
		return err
	}
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil || parsed.Database != s.Database || parsed.User != s.Database || parsed.TLSConfig != nil {
		return errRunnerConfig
	}
	args := []string{"-X", "-qAt", "-v", "ON_ERROR_STOP=1"}
	for _, key := range []string{"owner_service", "runtime_role", "template_sha256", "base_schema_sha256", "base_artifact_sha256", "base_manifest_sha256", "profile_sha256", "baseline_kind", "evidence_ref", "approval_ref", "backup_ref", "stopped_runtimes_ref", "request_id"} {
		if value, ok := vars[key]; ok {
			args = append(args, "-v", key+"="+value)
		}
	}
	commandCtx, cancel := context.WithTimeout(ctx, LockBound+OperationBound)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, psqlPath, args...)
	cmd.Env = psqlEnvironment(parsed)
	key := OwnerLockKey(s.Database)
	var input bytes.Buffer
	fmt.Fprintf(&input, "\\set ON_ERROR_STOP on\nSET statement_timeout='5s';\nSELECT pg_advisory_lock(%d);\nSET statement_timeout='30s';\n", key)
	input.Write(body)
	fmt.Fprintf(&input, "\nDO $unlock$ BEGIN IF NOT pg_advisory_unlock(%d) THEN RAISE EXCEPTION 'owner installation lock lost'; END IF; END $unlock$;\n", key)
	cmd.Stdin = &input
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		// Do not return command arguments, the DSN, SQL body or unbounded server
		// output. PostgreSQL diagnostics can contain release data and identifiers.
		return fmt.Errorf("owner %s template phase failed: %w", phase, err)
	}
	return nil
}

func psqlEnvironment(cfg *pgx.ConnConfig) []string {
	blocked := map[string]bool{"PGHOST": true, "PGPORT": true, "PGUSER": true, "PGPASSWORD": true, "PGDATABASE": true, "PGSSLMODE": true}
	env := make([]string, 0, len(os.Environ())+6)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if !blocked[key] {
			env = append(env, item)
		}
	}
	return append(env,
		"PGHOST="+cfg.Host,
		"PGPORT="+strconv.FormatUint(uint64(cfg.Port), 10),
		"PGUSER="+cfg.User,
		"PGPASSWORD="+cfg.Password,
		"PGDATABASE="+cfg.Database,
		"PGSSLMODE=disable",
	)
}

func runCLI(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("owner-migrate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var bundlePath, phase, reportPath string
	flags.StringVar(&bundlePath, "bundle", "", "absolute trusted bundle path")
	flags.StringVar(&phase, "phase", "", "shared-base, private, history, or reconcile")
	flags.StringVar(&reportPath, "report", "", "absolute durable attempt report path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || bundlePath == "" || phase == "" {
		return errRunnerConfig
	}
	cfg, err := loadBundle(bundlePath)
	if err != nil {
		return err
	}
	dsn := os.Getenv("JUSTIXAUTO_OWNER_MIGRATION_DSN")
	if dsn == "" {
		return errRunnerConfig
	}
	switch phase {
	case string(phaseSharedBase), string(phaseHistory):
		psqlPath, err := exec.LookPath("psql")
		if err != nil {
			return ErrUnsupported
		}
		return runTemplate(ctx, psqlPath, dsn, cfg, templatePhase(phase))
	case "private":
		if !filepath.IsAbs(reportPath) {
			return errRunnerConfig
		}
		if err := requireResolvedReport(reportPath, cfg); err != nil {
			return err
		}
		parsed, err := pgx.ParseConfig(dsn)
		if err != nil {
			return errRunnerConfig
		}
		open := func(ctx context.Context) (*pgx.Conn, error) { return pgx.ConnectConfig(ctx, parsed.Copy()) }
		_, err = runPrivate(ctx, open, cfg, fileReportStore{reportPath})
		return err
	case "reconcile":
		if cfg.FreshBootstrap || !filepath.IsAbs(reportPath) {
			return errRunnerConfig
		}
		a, err := loadAttempt(reportPath, cfg)
		if err != nil {
			return err
		}
		parsed, err := pgx.ParseConfig(dsn)
		if err != nil {
			return errRunnerConfig
		}
		conn, err := pgx.ConnectConfig(ctx, parsed.Copy())
		if err != nil {
			return err
		}
		outcome, err := Reconcile(ctx, conn, cfg, a)
		if err != nil {
			return err
		}
		status := StatusRetainedIncomplete
		if outcome == RecordedCompletion {
			status = StatusRecordedCompletion
		}
		if err := (fileReportStore{reportPath}).Save(reportFor(cfg, a, status)); err != nil {
			return err
		}
		if outcome != RecordedCompletion {
			return errRetainedIncomplete
		}
		return nil
	default:
		return errRunnerConfig
	}
}

func main() {
	if err := runCLI(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "owner migration did not complete")
		os.Exit(1)
	}
}
