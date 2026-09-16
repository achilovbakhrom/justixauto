// Package main contains the owner-only migration adapter. There is deliberately
// no URL registration, command line, automatic adoption, repair or down path.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/golang-migrate/migrate/v4/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"justixauto/pkg/persistence"
)

const LockBound = 5 * time.Second
const OperationBound = 30 * time.Second

var ErrUnsupported = errors.New("owner driver operation unsupported")
var ErrProtocol = errors.New("owner migration protocol mismatch")
var ErrPoisoned = errors.New("owner migration connection outcome requires reconciliation")

// Config is trusted release configuration, never request input. FreshBootstrap
// admits only a one-artifact v1 profile and an absent ledger/mechanics/history.
// RepoRoot supplies the original shared/template bytes; OwnerRoot supplies all
// private SQL and adjacent manifests. Neither root is a runtime SQL callback.
type Config struct {
	Profile             persistence.Profile
	RepoRoot, OwnerRoot string
	ProvenanceRef       string
	FreshBootstrap      bool
}

// Attempt is immutable by construction and identifies one actual dirty request.
// It is audit evidence for a fresh-connection read, not permission to rerun SQL.
type Attempt struct {
	version      int64
	request      uuid.UUID
	profile, sql persistence.Digest
	provenance   string
}

func (a Attempt) Version() int64       { return a.version }
func (a Attempt) RequestID() uuid.UUID { return a.request }

type pending struct {
	attempt Attempt
	index   int
	ran     bool
}
type Driver struct {
	mu                                    sync.Mutex
	ctx                                   context.Context
	conn                                  *pgx.Conn
	cfg                                   Config
	spec                                  persistence.Specification
	locked, poisoned, closed, freshLedger bool
	pending                               *pending
	last                                  Attempt
	bodies                                map[int64][]byte
}

var _ database.Driver = (*Driver)(nil)

// NewDriver transfers exclusive connection ownership on success, but performs
// no database queries or mutation. The caller must stop using conn afterwards.
func NewDriver(ctx context.Context, conn *pgx.Conn, cfg Config) (*Driver, error) {
	if ctx == nil || conn == nil || !cfg.Profile.Valid() || !filepath.IsAbs(cfg.RepoRoot) || !filepath.IsAbs(cfg.OwnerRoot) || !validReference(cfg.ProvenanceRef) {
		return nil, ErrProtocol
	}
	s := cfg.Profile.Specification()
	if s.Shared.Mode != "legacy" || len(s.Shared.Artifacts) > 4 || (cfg.FreshBootstrap && (s.Head != 1 || len(s.Artifacts) != 1 || s.Baseline.Kind != "verified-installation")) {
		return nil, ErrUnsupported
	}
	return &Driver{ctx: ctx, conn: conn, cfg: cfg, spec: s}, nil
}
func validReference(s string) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 2048 && !strings.ContainsFunc(s, unicode.IsControl)
}
func (*Driver) Open(string) (database.Driver, error) { return nil, ErrUnsupported }
func (*Driver) Drop() error                          { return ErrUnsupported }
func OwnerLockKey(databaseName string) int64 {
	h := sha256.Sum256([]byte("justixauto:owner-install:v1:" + databaseName))
	return int64(binary.BigEndian.Uint64(h[:8]))
}
func digest(d persistence.Digest) string { return hex.EncodeToString(d[:]) }
func (d *Driver) timeout(bound time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(d.ctx, bound)
}
func (d *Driver) idle() error {
	if d.closed || d.poisoned || d.conn.IsClosed() {
		return ErrPoisoned
	}
	if d.conn.PgConn().TxStatus() != 'I' || d.conn.PgConn().IsBusy() {
		return ErrProtocol
	}
	return nil
}
func (d *Driver) poison(cause error) error {
	d.poisoned = true
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	closeErr := d.closeOwned(ctx)
	d.locked = false
	return errors.Join(ErrPoisoned, cause, closeErr)
}

// pgx can mark a connection closed BEFORE asynchronous cleanup closes its
// transport. Native Close then returns immediately, while cleanup may still be
// waiting on its separate cancel request. Always close our exclusively owned
// public native transport as well. net.Conn permits concurrent Close with that
// cleanup. This proves client closure, not synchronous server lock release;
// authoritative server state still belongs to fresh bounded reconciliation.
func (d *Driver) closeOwned(ctx context.Context) error {
	err := d.conn.Close(ctx)
	transportErr := d.conn.PgConn().Conn().Close()
	if errors.Is(transportErr, net.ErrClosed) {
		transportErr = nil
	}
	return errors.Join(err, transportErr)
}
func (d *Driver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	d.locked = false
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return d.closeOwned(ctx)
}
func (d *Driver) LastAttempt() Attempt { d.mu.Lock(); defer d.mu.Unlock(); return d.last }

// Lock includes original-file verification and all SQL preflight in its bound.
// No asynchronous acquisition survives return; the engine must use a longer
// LockTimeout (its approved default is 15 seconds).
func (d *Driver) Lock() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	ctx, cancel := d.timeout(LockBound)
	defer cancel()
	if err := d.idle(); err != nil {
		return err
	}
	if d.locked {
		return database.ErrLocked
	}
	if err := d.verifyFiles(ctx); err != nil {
		return err
	}
	if err := d.owner(ctx); err != nil {
		return err
	}
	for {
		var acquired bool
		if err := d.conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", OwnerLockKey(d.spec.Database)).Scan(&acquired); err != nil {
			return d.poison(err)
		}
		if acquired {
			d.locked = true
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err := d.preflight(ctx); err != nil {
		return d.poison(err)
	}
	return nil
}
func (d *Driver) Unlock() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.locked {
		return database.ErrNotLocked
	}
	if err := d.idle(); err != nil {
		return d.poison(err)
	}
	ctx, cancel := d.timeout(LockBound)
	defer cancel()
	var released bool
	if err := d.conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", OwnerLockKey(d.spec.Database)).Scan(&released); err != nil {
		return d.poison(err)
	}
	if !released {
		return d.poison(database.ErrNotLocked)
	}
	d.locked = false
	return nil
}

func (d *Driver) verifyFiles(ctx context.Context) error {
	if err := d.cfg.Profile.VerifyFiles(d.cfg.OwnerRoot); err != nil {
		return err
	}
	bodies := make(map[int64][]byte, len(d.spec.Artifacts))
	for _, a := range d.spec.Artifacts {
		if err := ctx.Err(); err != nil {
			return err
		}
		b, err := readOriginal(d.cfg.OwnerRoot, a.Identity.Filename, a.Identity.SHA256)
		if err != nil {
			return err
		}
		bodies[a.Identity.Version] = b
		if _, err = readOriginal(d.cfg.OwnerRoot, strings.TrimSuffix(a.Identity.Filename, ".up.sql")+".manifest.json", a.ManifestSHA256); err != nil {
			return err
		}
	}
	for i, a := range d.spec.Shared.Artifacts {
		if a.Filename != sharedArtifacts[i].name || digest(a.SHA256) != sharedArtifacts[i].hash {
			return persistence.ErrUnsupported
		}
		if _, err := readOriginal(d.cfg.RepoRoot, a.Filename, a.SHA256); err != nil {
			return err
		}
	}
	if _, err := readOriginal(d.cfg.RepoRoot, "pkg/eventstore/owner_history.sql", d.spec.HistorySHA256); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	d.bodies = bodies
	return nil
}
func readOriginal(root, name string, want persistence.Digest) ([]byte, error) {
	p := filepath.Join(root, filepath.FromSlash(name))
	// Paths are typed profile identities. Still reject every symlink component,
	// including the trusted root, and nonregular bodies at the last boundary.
	for at := p; ; at = filepath.Dir(at) {
		st, err := os.Lstat(at)
		if err != nil {
			return nil, err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return nil, ErrProtocol
		}
		if at == p && !st.Mode().IsRegular() {
			return nil, ErrProtocol
		}
		if filepath.Dir(at) == at {
			break
		}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 || persistence.Digest(sha256.Sum256(b)) != want {
		return nil, ErrProtocol
	}
	return b, nil
}

type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func require(ctx context.Context, q querier, sql string, args ...any) error {
	var ok bool
	if err := q.QueryRow(ctx, sql, args...).Scan(&ok); err != nil {
		return fmt.Errorf("owner compatibility query: %w", err)
	}
	if !ok {
		return persistence.ErrIncompatible
	}
	return nil
}
func (d *Driver) owner(ctx context.Context) error {
	return require(ctx, d.conn, `SELECT current_database()=$1 AND current_user=$1 AND session_user=$1 AND current_setting('session_replication_role')='origin' AND (SELECT datdba=current_user::regrole FROM pg_database WHERE datname=current_database())`, d.spec.Database)
}

func (d *Driver) held(ctx context.Context) error {
	if !d.locked {
		return database.ErrNotLocked
	}
	return require(ctx, d.conn, `SELECT EXISTS(SELECT FROM pg_locks WHERE locktype='advisory' AND pid=pg_backend_pid() AND database=(SELECT oid FROM pg_database WHERE datname=current_database()) AND classid=(($1::bigint >> 32) & 4294967295)::oid AND objid=($1::bigint & 4294967295)::oid AND objsubid=1 AND mode='ExclusiveLock' AND granted)`, OwnerLockKey(d.spec.Database))
}
func (d *Driver) preflight(ctx context.Context) error {
	var ledger, history, mechanics bool
	if err := d.conn.QueryRow(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL,to_regnamespace('owner_migrations') IS NOT NULL,to_regnamespace($1) IS NOT NULL`, d.spec.Owner+"_mechanics").Scan(&ledger, &history, &mechanics); err != nil {
		return err
	}
	if d.cfg.FreshBootstrap {
		if ledger || history || mechanics || d.freshLedger {
			return ErrProtocol
		}
		if err := d.checkCatalog(ctx, d.conn, 0, false, false); err != nil {
			return err
		}
		if err := d.checkShared(ctx, d.conn); err != nil {
			return err
		}
		if _, err := d.conn.Exec(ctx, `CREATE TABLE public.schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL)`); err != nil {
			return err
		}
		d.freshLedger = true
		return nil
	}
	if !ledger || !history || !mechanics {
		return persistence.ErrIncompatible
	}
	v, dirty, err := d.ledger(ctx, d.conn, false)
	if err != nil {
		return err
	}
	if dirty {
		return ErrProtocol
	}
	i := d.index(v)
	if i < 0 {
		return ErrProtocol
	}
	return d.checkState(ctx, d.conn, i+1, i+1, true)
}
func (d *Driver) index(v int64) int {
	for i, a := range d.spec.Artifacts {
		if a.Identity.Version == v {
			return i
		}
	}
	return -1
}
func (d *Driver) ledger(ctx context.Context, q querier, allowEmpty bool) (int64, bool, error) {
	if err := require(ctx, q, `SELECT count(*)=2 AND count(*) FILTER(WHERE attname='version' AND atttypid='bigint'::regtype AND attnotnull)=1 AND count(*) FILTER(WHERE attname='dirty' AND atttypid='boolean'::regtype AND attnotnull)=1 FROM pg_attribute WHERE attrelid='public.schema_migrations'::regclass AND attnum>0 AND NOT attisdropped`); err != nil {
		return 0, false, err
	}
	var count int64
	var v *int64
	var dirty *bool
	if err := q.QueryRow(ctx, `SELECT count(*),min(version),bool_or(dirty) FROM public.schema_migrations`).Scan(&count, &v, &dirty); err != nil {
		return 0, false, err
	}
	if count == 0 && allowEmpty {
		return int64(database.NilVersion), false, nil
	}
	if count != 1 || v == nil || dirty == nil || *v < 1 {
		return 0, false, ErrProtocol
	}
	return *v, *dirty, nil
}
func (d *Driver) Version() (int, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.idle(); err != nil {
		return 0, false, err
	}
	ctx, cancel := d.timeout(OperationBound)
	defer cancel()
	v, dirty, err := d.ledger(ctx, d.conn, d.freshLedger && d.locked && d.last.request == uuid.Nil)
	if int64(int(v)) != v {
		return 0, false, ErrProtocol
	}
	return int(v), dirty, err
}

func (d *Driver) SetVersion(version int, dirty bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.idle(); err != nil {
		return err
	}
	if !d.locked {
		return database.ErrNotLocked
	}
	ctx, cancel := d.timeout(OperationBound)
	defer cancel()
	if dirty {
		return d.beginAttempt(ctx, int64(version))
	}
	return d.finalize(ctx, int64(version))
}
func (d *Driver) beginAttempt(ctx context.Context, version int64) error {
	if d.pending != nil {
		return ErrProtocol
	}
	if err := d.held(ctx); err != nil {
		return d.poison(err)
	}
	// Recheck original bytes even if an outer caller changed the directory after
	// Lock. Historical files are mandatory, not merely the next source body.
	if err := d.verifyFiles(ctx); err != nil {
		return err
	}
	i := d.index(version)
	if i < 0 || (d.freshLedger && i != 0) {
		return ErrProtocol
	}
	tx, err := d.conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return d.poison(err)
	}
	abort := func(cause error) error {
		if err := tx.Rollback(ctx); err != nil {
			return d.poison(errors.Join(cause, err))
		}
		return cause
	}
	if _, err := tx.Exec(ctx, `LOCK TABLE public.schema_migrations IN ACCESS EXCLUSIVE MODE`); err != nil {
		return abort(err)
	}
	v, dirty, err := d.ledger(ctx, tx, d.freshLedger)
	if err != nil {
		return abort(err)
	}
	if dirty || (i == 0 && v != int64(database.NilVersion)) || (i > 0 && v != d.spec.Artifacts[i-1].Identity.Version) {
		return abort(ErrProtocol)
	}
	if i > 0 {
		if err := d.checkState(ctx, tx, i, i, true); err != nil {
			return abort(err)
		}
	}
	if i == 0 && !d.freshLedger {
		return abort(ErrProtocol)
	}
	a := Attempt{version: version, request: uuid.New(), profile: d.cfg.Profile.Digest(), sql: d.spec.Artifacts[i].Identity.SHA256, provenance: d.cfg.ProvenanceRef}
	if i == 0 {
		a.request = d.spec.Baseline.RequestID
	}
	if i == 0 {
		_, err = tx.Exec(ctx, `INSERT INTO public.schema_migrations(version,dirty) VALUES($1,true)`, version)
	} else {
		_, err = tx.Exec(ctx, `UPDATE public.schema_migrations SET version=$1,dirty=true`, version)
	}
	if err != nil {
		return abort(err)
	}
	written, marked, err := d.ledger(ctx, tx, false)
	if err != nil || written != version || !marked {
		return abort(errors.Join(ErrProtocol, err))
	}
	d.last = a
	if err := tx.Commit(ctx); err != nil {
		return d.poison(err)
	}
	d.pending = &pending{attempt: a, index: i}
	return nil
}
func (d *Driver) Run(reader io.Reader) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.idle(); err != nil {
		return err
	}
	if !d.locked || d.pending == nil || d.pending.ran || reader == nil {
		return ErrProtocol
	}
	a := d.pending
	original := d.bodies[a.attempt.version]
	ctx, cancel := d.timeout(OperationBound)
	defer cancel()
	// The pinned engine hands Run an *io.PipeReader. Cancel that pipe directly
	// so no read goroutine can outlive this method. Direct callers may supply
	// standard immutable memory readers; arbitrary blocking Reader callbacks
	// are deliberately unsupported at this execution boundary.
	switch r := reader.(type) {
	case *io.PipeReader:
		stop := context.AfterFunc(ctx, func() { _ = r.CloseWithError(ctx.Err()) })
		defer stop()
		defer r.Close()
	case *bytes.Reader, *strings.Reader:
	default:
		return d.poison(ErrUnsupported)
	}
	body, err := io.ReadAll(io.LimitReader(reader, int64(len(original))+1))
	if err != nil || len(body) == 0 || !bytes.Equal(body, original) || persistence.Digest(sha256.Sum256(body)) != a.attempt.sql {
		return d.poison(ErrProtocol)
	}
	v, dirty, err := d.ledger(ctx, d.conn, false)
	if err != nil || v != a.attempt.version || !dirty {
		return d.poison(errors.Join(ErrProtocol, err))
	}
	if err := d.held(ctx); err != nil {
		return d.poison(err)
	}
	_, err = d.conn.PgConn().Exec(ctx, string(body)).ReadAll()
	if err != nil || d.conn.PgConn().TxStatus() != 'I' {
		if !d.conn.IsClosed() && d.conn.PgConn().TxStatus() != 'I' {
			cleanup, stop := context.WithTimeout(context.Background(), time.Second)
			_, rollbackErr := d.conn.PgConn().Exec(cleanup, "ROLLBACK").ReadAll()
			stop()
			err = errors.Join(err, rollbackErr)
		}
		return d.poison(errors.Join(ErrProtocol, err))
	}
	if err := d.held(ctx); err != nil {
		return d.poison(err)
	}
	a.ran = true
	return nil
}
func (d *Driver) finalize(ctx context.Context, version int64) error {
	p := d.pending
	if p == nil || !p.ran || p.attempt.version != version {
		return ErrProtocol
	}
	if err := d.held(ctx); err != nil {
		return d.poison(err)
	}
	tx, err := d.conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return d.poison(err)
	}
	abort := func(cause error) error {
		if err := tx.Rollback(ctx); err != nil {
			return d.poison(errors.Join(cause, err))
		}
		return d.poison(cause)
	}
	if _, err := tx.Exec(ctx, `LOCK TABLE public.schema_migrations IN ACCESS EXCLUSIVE MODE`); err != nil {
		return abort(err)
	}
	v, dirty, err := d.ledger(ctx, tx, false)
	if err != nil || v != version || !dirty {
		return abort(errors.Join(ErrProtocol, err))
	}
	if err := d.checkState(ctx, tx, p.index, p.index+1, p.index > 0); err != nil {
		return abort(err)
	}
	if p.index > 0 {
		a := d.spec.Artifacts[p.index]
		prior := d.spec.Artifacts[p.index-1].Identity
		recorded, err := tx.Exec(ctx, `INSERT INTO owner_migrations.artifacts(version,filename,sql_sha256,predecessor_version,predecessor_sha256,feature_contract_id,feature_contract_revision,feature_contract_sha256,installation_request_id,artifact_manifest_sha256,installing_profile_sha256,provenance_kind,provenance_ref)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'verified-installation',$12)`, version, a.Identity.Filename, a.Identity.SHA256[:], prior.Version, prior.SHA256[:], a.Feature.ID, int64(a.Feature.Revision), a.Feature.SHA256[:], p.attempt.request.String(), a.ManifestSHA256[:], p.attempt.profile[:], p.attempt.provenance)
		if err != nil {
			return abort(err)
		}
		if recorded.RowsAffected() != 1 {
			return abort(ErrProtocol)
		}
		if err := d.checkHistory(ctx, tx, p.index+1, p.index+1); err != nil {
			return abort(err)
		}
		if err := require(ctx, tx, `SELECT count(*)=1 FROM owner_migrations.artifacts WHERE version=$1 AND installation_request_id=$2::uuid AND installing_profile_sha256=$3 AND provenance_ref=$4`, version, p.attempt.request.String(), p.attempt.profile[:], p.attempt.provenance); err != nil {
			return abort(err)
		}
	}
	changed, err := tx.Exec(ctx, `UPDATE public.schema_migrations SET dirty=false WHERE version=$1 AND dirty`, version)
	if err != nil {
		return abort(err)
	}
	if changed.RowsAffected() != 1 {
		return abort(ErrProtocol)
	}
	cleanVersion, stillDirty, err := d.ledger(ctx, tx, false)
	if err != nil || cleanVersion != version || stillDirty {
		return abort(errors.Join(ErrProtocol, err))
	}
	if err := tx.Commit(ctx); err != nil {
		return d.poison(err)
	}
	d.pending = nil
	return nil
}

var sharedArtifacts = []struct{ name, hash string }{
	{"pkg/eventstore/schema.sql", "86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53"},
	{"pkg/eventstore/migrations/000002_messaging_delivery.up.sql", "1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2"},
	{"pkg/eventstore/migrations/000003_messaging_route_compatibility.up.sql", "c198af498e39056a7a251b5906d6db050ad994e04588bf6cecf8316221ea85ec"},
	{"pkg/eventstore/migrations/000004_projection_checkpoint.up.sql", "41536429fb95d4b60a11866843a2ccbd6a4cf723cd919884933b6bc1d8202c4c"},
}

func (d *Driver) checkState(ctx context.Context, q querier, receipts, markers int, history bool) error {
	if err := d.checkCatalog(ctx, q, markers, history, true); err != nil {
		return err
	}
	if err := require(ctx, q, fmt.Sprintf(`SELECT count(*)=1 AND coalesce(bool_and(singleton AND owner_service=$1 AND mechanics_version=1),false) FROM %s.compatibility`, pgx.Identifier{d.spec.Owner + "_mechanics"}.Sanitize()), d.spec.Owner); err != nil {
		return err
	}
	if err := d.checkShared(ctx, q); err != nil {
		return err
	}
	if !history {
		return require(ctx, q, `SELECT to_regnamespace('owner_migrations') IS NULL`)
	}
	return d.checkHistory(ctx, q, receipts, markers)
}
func (d *Driver) checkShared(ctx context.Context, q querier) error {
	if err := require(ctx, q, `SELECT EXISTS(SELECT FROM pg_attrdef d JOIN pg_attribute a ON a.attrelid=d.adrelid AND a.attnum=d.adnum WHERE d.adrelid='eventstore.events'::regclass AND a.attname='owner_service' AND pg_get_expr(d.adbin,d.adrelid)=quote_literal($1)||'::text')`, d.spec.Owner); err != nil {
		return err
	}
	for i, name := range []string{"messaging_mode", "messaging_route_compatibility", "projection_checkpoint_compatibility", "quarantine_compatibility", "projection_generation_compatibility"} {
		if err := require(ctx, q, `SELECT (to_regclass($1) IS NOT NULL)=$2`, "eventstore."+name, i+2 <= len(d.spec.Shared.Artifacts)); err != nil {
			return err
		}
	}
	if len(d.spec.Shared.Artifacts) >= 2 {
		if err := require(ctx, q, `SELECT count(*)=1 AND coalesce(bool_and(singleton AND owner_service=$1 AND runtime_role::text=$2 AND schema_version=2 AND mode='legacy'),false) FROM eventstore.messaging_mode`, d.spec.Owner, d.spec.RuntimeRole); err != nil {
			return err
		}
	}
	if len(d.spec.Shared.Artifacts) >= 3 {
		if err := require(ctx, q, `SELECT count(*)=1 AND coalesce(bool_and(singleton AND owner_service=$1 AND runtime_role::text=$2 AND base_schema_version=2 AND migration_revision=3 AND route_format_version=1 AND prior_migration_sha256=decode($3,'hex') AND correction_migration_sha256=decode($4,'hex') AND isfinite(installed_at)),false) FROM eventstore.messaging_route_compatibility`, d.spec.Owner, d.spec.RuntimeRole, sharedArtifacts[1].hash, sharedArtifacts[2].hash); err != nil {
			return err
		}
	}
	if len(d.spec.Shared.Artifacts) >= 4 {
		if err := require(ctx, q, `SELECT count(*)=1 AND coalesce(bool_and(singleton AND owner_service=$1 AND runtime_role::text=$2 AND base_schema_version=2 AND migration_revision=4 AND feature_format_version=1 AND base_schema_sha256=decode($3,'hex') AND prior_migration_sha256=decode($4,'hex') AND correction_migration_sha256=decode($5,'hex') AND checkpoint_migration_sha256=decode($6,'hex') AND isfinite(installed_at)),false) FROM eventstore.projection_checkpoint_compatibility`, d.spec.Owner, d.spec.RuntimeRole, sharedArtifacts[0].hash, sharedArtifacts[1].hash, sharedArtifacts[2].hash, sharedArtifacts[3].hash); err != nil {
			return err
		}
	}
	return nil
}

type receipt struct {
	Version                                 int64
	Filename, SQLHash, ManifestHash         string
	Predecessor                             *int64
	PredecessorHash, FeatureID, FeatureHash *string
	FeatureRevision                         *int64
	ValidEvidence                           bool
}

func (d *Driver) checkHistory(ctx context.Context, q querier, n, markerCount int) error {
	s := d.spec
	b := s.Baseline
	if n < 1 || n > len(s.Artifacts) || markerCount < n || markerCount > n+1 {
		return ErrProtocol
	}
	if err := require(ctx, q, `SELECT count(*)=count(DISTINCT installation_request_id) FROM owner_migrations.artifacts`); err != nil {
		return err
	}
	if err := require(ctx, q, `SELECT count(*)=1 AND coalesce(bool_and(singleton AND owner_service=$1 AND database_name::text=$2 AND runtime_role::text=$3 AND format_revision=$4 AND template_sha256=decode($5,'hex') AND base_schema_sha256=decode($6,'hex') AND base_version=1 AND base_artifact_sha256=decode($7,'hex') AND baseline_kind=$8 AND evidence_ref=$9 AND approval_ref=$10 AND backup_ref=$11 AND stopped_runtimes_ref=$12 AND installation_request_id=$13::uuid AND isfinite(installed_at)),false) FROM owner_migrations.compatibility`, s.Owner, s.Database, s.RuntimeRole, s.HistoryRevision, digest(s.HistorySHA256), sharedArtifacts[0].hash, digest(s.Artifacts[0].Identity.SHA256), b.Kind, b.EvidenceRef, b.ApprovalRef, b.BackupRef, b.StoppedRuntimesRef, b.RequestID.String()); err != nil {
		return err
	}
	var raw []byte
	if err := q.QueryRow(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY "Version"),'[]') FROM (
 SELECT version AS "Version",filename AS "Filename",encode(sql_sha256,'hex') AS "SQLHash",predecessor_version AS "Predecessor",encode(predecessor_sha256,'hex') AS "PredecessorHash",feature_contract_id AS "FeatureID",feature_contract_revision AS "FeatureRevision",encode(feature_contract_sha256,'hex') AS "FeatureHash",encode(artifact_manifest_sha256,'hex') AS "ManifestHash",
 installation_request_id<>'00000000-0000-0000-0000-000000000000'::uuid AND octet_length(installing_profile_sha256)=32 AND installing_profile_sha256<>decode(repeat('00',32),'hex') AND isfinite(completed_at) AND CASE WHEN version=1 THEN installation_request_id=$1::uuid AND provenance_kind=$2 AND provenance_ref=$3 ELSE provenance_kind='verified-installation' AND length(btrim(provenance_ref)) BETWEEN 1 AND 2048 AND provenance_ref !~ '[[:cntrl:]]' END AS "ValidEvidence"
 FROM owner_migrations.artifacts) r`, b.RequestID.String(), b.Kind, b.EvidenceRef).Scan(&raw); err != nil {
		return err
	}
	var rows []receipt
	if err := json.Unmarshal(raw, &rows); err != nil {
		return err
	}
	if len(rows) != n {
		return ErrProtocol
	}
	for i, r := range rows {
		a := s.Artifacts[i]
		if r.Version != a.Identity.Version || r.Filename != a.Identity.Filename || r.SQLHash != digest(a.Identity.SHA256) || r.ManifestHash != digest(a.ManifestSHA256) || !r.ValidEvidence {
			return ErrProtocol
		}
		if i == 0 {
			if r.Predecessor != nil || r.PredecessorHash != nil || r.FeatureID != nil || r.FeatureRevision != nil || r.FeatureHash != nil {
				return ErrProtocol
			}
		} else {
			prior := s.Artifacts[i-1].Identity
			if r.Predecessor == nil || *r.Predecessor != prior.Version || r.PredecessorHash == nil || *r.PredecessorHash != digest(prior.SHA256) || r.FeatureID == nil || *r.FeatureID != a.Feature.ID || r.FeatureRevision == nil || *r.FeatureRevision != int64(a.Feature.Revision) || r.FeatureHash == nil || *r.FeatureHash != digest(a.Feature.SHA256) {
				return ErrProtocol
			}
		}
	}
	var markers int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM owner_migrations.feature_contracts`).Scan(&markers); err != nil {
		return err
	}
	if markers != markerCount-1 {
		return ErrProtocol
	}
	for _, a := range s.Artifacts[1:markerCount] {
		if err := require(ctx, q, `SELECT count(*)=1 FROM owner_migrations.feature_contracts WHERE contract_id=$1 AND contract_revision=$2 AND migration_version=$3 AND contract_sha256=$4`, a.Feature.ID, int64(a.Feature.Revision), a.Identity.Version, a.Feature.SHA256[:]); err != nil {
			return err
		}
	}
	return require(ctx, q, `SELECT coalesce(array_agg(c.relname||':'||t.tgname||':'||p.proname||':'||t.tgtype ORDER BY c.relname,t.tgname),ARRAY[]::text[])=ARRAY['artifacts:artifacts_immutable:immutable:58','artifacts:artifacts_insert:artifact_guard:7','compatibility:compatibility_immutable:immutable:58','feature_contracts:feature_contracts_immutable:immutable:58','feature_contracts:feature_contracts_insert:feature_guard:7'] FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_proc p ON p.oid=t.tgfoid JOIN pg_namespace n ON n.oid=p.pronamespace WHERE c.relnamespace='owner_migrations'::regnamespace AND NOT t.tgisinternal AND t.tgenabled IN ('O','A') AND t.tgqual IS NULL AND t.tgnargs=0 AND octet_length(t.tgargs)=0 AND t.tgattr=''::int2vector AND t.tgconstraint=0 AND t.tgconstrrelid=0 AND t.tgconstrindid=0 AND NOT t.tgdeferrable AND NOT t.tginitdeferred AND t.tgparentid=0 AND t.tgoldtable IS NULL AND t.tgnewtable IS NULL AND n.nspname='owner_migrations' AND NOT p.prosecdef AND p.proconfig=ARRAY['search_path=pg_catalog']::text[] AND p.proowner=(SELECT datdba FROM pg_database WHERE datname=current_database())`)
}

// Fixed T-930 policy copied deliberately for native pgx owner-session use.
// Required privileges target runtime OID, never the privileged current_user.
type grantRecord struct {
	Schema        string   `json:"schema"`
	Table         string   `json:"tbl"`
	Select        bool     `json:"sel"`
	Insert        bool     `json:"ins"`
	Update        bool     `json:"upd"`
	Delete        bool     `json:"del"`
	SelectColumns []string `json:"sc"`
	InsertColumns []string `json:"ic"`
	UpdateColumns []string `json:"uc"`
}

func grants(s persistence.Specification, count int, history, mechanics bool) []grantRecord {
	var out []grantRecord
	add := func(schema, table string, insert bool, updates ...string) {
		out = append(out, grantRecord{schema, table, true, insert, false, false, []string{}, []string{}, updates})
	}
	if mechanics {
		add("public", "schema_migrations", false)
		add(s.Owner+"_mechanics", "compatibility", false)
	}
	if history {
		for _, name := range []string{"compatibility", "artifacts", "feature_contracts"} {
			add("owner_migrations", name, false)
		}
	}
	for _, name := range []string{"events", "command_receipts", "inbox", "operation_steps"} {
		add("eventstore", name, true)
	}
	add("eventstore", "outbox", true, "attempts", "next_attempt_at", "lease_owner", "lease_until", "sent_at")
	add("eventstore", "operations", true, "phase", "decision", "attention_required", "result", "error", "attempts", "next_attempt_at", "lease_owner", "lease_until", "revision", "updated_at")
	if len(s.Shared.Artifacts) >= 2 {
		for _, name := range []string{"messaging_mode", "messaging_legacy_authorizations", "messaging_cutovers", "messaging_legacy_evidence"} {
			add("eventstore", name, false)
		}
		for _, name := range []string{"messaging_admissions", "outbox_messages", "dispatch_messages", "dispatch_enrollments"} {
			add("eventstore", name, true)
		}
		add("eventstore", "outbox_deliveries", true, "attempts", "next_attempt_at", "lease_owner", "lease_until", "sent_at", "hold_ref")
		add("eventstore", "dispatch_jobs", true, "attempts", "next_attempt_at", "lease_owner", "lease_until", "completed_at", "inbox_consumer_name", "inbox_event_id", "hold_ref", "quarantine_ref")
	}
	if len(s.Shared.Artifacts) >= 3 {
		add("eventstore", "messaging_route_compatibility", false)
	}
	if len(s.Shared.Artifacts) >= 4 {
		add("eventstore", "projection_checkpoint_compatibility", false)
		for _, name := range []string{"consumer_bootstraps", "consumer_gaps", "consumer_gap_attempts"} {
			add("eventstore", name, true)
		}
		add("eventstore", "consumer_checkpoints", true, "position", "last_event_id", "last_event_hash", "revision", "updated_at")
	}
	features := map[persistence.FeatureIdentity]bool{}
	for _, a := range s.Artifacts[:count] {
		if a.Feature != nil {
			features[*a.Feature] = true
		}
	}
	for _, f := range s.Features {
		if !features[f.Identity] {
			continue
		}
		for _, g := range f.Tables {
			out = append(out, grantRecord{g.Schema, g.Table, g.Select, g.Insert, g.Update, g.Delete, g.SelectColumns, g.InsertColumns, g.UpdateColumns})
		}
	}
	// JSON arrays, including empty sets, must never become SQL NULL permissions.
	for i := range out {
		for _, p := range []*[]string{&out[i].SelectColumns, &out[i].InsertColumns, &out[i].UpdateColumns} {
			if *p == nil {
				*p = []string{}
			}
		}
	}
	return out
}

func (d *Driver) checkCatalog(ctx context.Context, q querier, count int, history, mechanics bool) error {
	encoded, err := json.Marshal(grants(d.spec, count, history, mechanics))
	if err != nil {
		return err
	}
	return require(ctx, q, catalogSQL, string(encoded), d.spec.Database, d.spec.RuntimeRole)
}

type Reconciliation string

const (
	RecordedCompletion Reconciliation = "recorded-completion"
	RetainedIncomplete Reconciliation = "retained-incomplete"
)

// Reconcile consumes a NEW exclusively owned verified owner connection and
// closes it before return. A missing receipt is never proof of rollback. This
// read-only operation does not clear a dirty head or execute any artifact.
func Reconcile(ctx context.Context, conn *pgx.Conn, cfg Config, a Attempt) (Reconciliation, error) {
	d, err := NewDriver(ctx, conn, cfg)
	if err != nil {
		return "", err
	}
	defer d.Close()
	if cfg.FreshBootstrap || a.request == uuid.Nil || a.profile != cfg.Profile.Digest() || !validReference(a.provenance) {
		return "", ErrProtocol
	}
	i := d.index(a.version)
	if i < 1 || d.spec.Artifacts[i].Identity.SHA256 != a.sql {
		return "", ErrProtocol
	}
	ctx, cancel := context.WithTimeout(ctx, LockBound)
	defer cancel()
	if err := d.idle(); err != nil {
		return "", err
	}
	if err := d.verifyFiles(ctx); err != nil {
		return "", err
	}
	if err := d.owner(ctx); err != nil {
		return "", err
	}
	for {
		var acquired bool
		if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, OwnerLockKey(d.spec.Database)).Scan(&acquired); err != nil {
			return "", err
		}
		if acquired {
			d.locked = true
			break
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadOnly})
	if err != nil {
		return "", err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	v, dirty, err := d.ledger(ctx, tx, false)
	if err != nil || v != a.version {
		return "", errors.Join(ErrProtocol, err)
	}
	if !dirty {
		if err := d.checkState(ctx, tx, i+1, i+1, true); err != nil {
			return "", err
		}
		if err := require(ctx, tx, `SELECT count(*)=1 FROM owner_migrations.artifacts WHERE version=$1 AND installation_request_id=$2::uuid AND sql_sha256=$3 AND installing_profile_sha256=$4 AND provenance_kind='verified-installation' AND provenance_ref=$5`, a.version, a.request.String(), a.sql[:], a.profile[:], a.provenance); err != nil {
			return "", err
		}
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		return RecordedCompletion, nil
	}
	var markerCount int
	if err := tx.QueryRow(ctx, `SELECT count(*)+1 FROM owner_migrations.feature_contracts`).Scan(&markerCount); err != nil {
		return "", err
	}
	if markerCount != i && markerCount != i+1 {
		return "", ErrProtocol
	}
	if err := d.checkState(ctx, tx, i, markerCount, true); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return RetainedIncomplete, nil
}

const catalogSQL = `WITH expected AS (
 SELECT * FROM jsonb_to_recordset($1::jsonb) AS e(schema text,tbl text,sel boolean,ins boolean,upd boolean,del boolean,sc text[],ic text[],uc text[])
), owner AS (SELECT oid FROM pg_roles WHERE rolname=$2), runtime AS (SELECT oid FROM pg_roles WHERE rolname=$3),
 roles AS (SELECT r.* FROM pg_roles r,runtime u WHERE pg_has_role(u.oid,r.oid,'MEMBER')),
 namespaces AS (SELECT * FROM pg_namespace WHERE nspname NOT LIKE 'pg\_%' ESCAPE '\' AND nspname<>'information_schema'),
 relations AS (SELECT c.*,n.nspname FROM pg_class c JOIN namespaces n ON n.oid=c.relnamespace WHERE c.relkind IN ('r','p','v','m','f','S')),
 default_families AS (SELECT kind::"char",builtin::"char" FROM (VALUES ('r','r'),('S','s'),('f','f'),('T','T'),('n','n'),('L','L')) family(kind,builtin)),
 effective_global_defaults AS (SELECT coalesce(d.defaclacl,acldefault(f.builtin,o.oid)) AS acl FROM owner o CROSS JOIN default_families f LEFT JOIN pg_default_acl d ON d.defaclrole=o.oid AND d.defaclnamespace=0 AND d.defaclobjtype=f.kind)
SELECT (SELECT count(*)=1 FROM owner) AND (SELECT count(*)=1 FROM runtime)
 AND current_setting('session_replication_role')='origin'
 AND (SELECT datdba=(SELECT oid FROM owner) FROM pg_database WHERE datname=current_database())
 AND has_database_privilege((SELECT oid FROM runtime),current_database(),'CONNECT')
 AND NOT EXISTS(SELECT FROM roles r WHERE r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolreplication OR r.rolbypassrls OR r.oid=(SELECT oid FROM owner) OR r.rolname LIKE 'pg\_%' ESCAPE '\' OR has_database_privilege(r.oid,current_database(),'CREATE,TEMPORARY') OR has_parameter_privilege(r.oid,'session_replication_role','SET,ALTER SYSTEM'))
 AND NOT EXISTS(SELECT FROM pg_database d CROSS JOIN LATERAL aclexplode(d.datacl) a WHERE d.datname=current_database() AND (a.grantee=0 OR (a.grantee IN (SELECT oid FROM roles) AND a.is_grantable)))
 AND NOT EXISTS(SELECT FROM pg_database d CROSS JOIN roles r WHERE d.datname LIKE 'justix\_%' ESCAPE '\' AND d.datname<>current_database() AND has_database_privilege(r.oid,d.oid,'CONNECT,CREATE,TEMPORARY'))
 AND NOT EXISTS(SELECT FROM expected e LEFT JOIN namespaces n ON n.nspname=e.schema WHERE n.oid IS NULL OR (n.nspname<>'public' AND n.nspowner<>(SELECT oid FROM owner)) OR NOT has_schema_privilege((SELECT oid FROM runtime),n.oid,'USAGE'))
 AND NOT EXISTS(SELECT FROM namespaces n CROSS JOIN roles r WHERE has_schema_privilege(r.oid,n.oid,'CREATE') OR (has_schema_privilege(r.oid,n.oid,'USAGE') AND n.nspname<>'public' AND NOT EXISTS(SELECT FROM expected e WHERE e.schema=n.nspname)))
 AND NOT EXISTS(SELECT FROM namespaces n CROSS JOIN LATERAL aclexplode(n.nspacl) a WHERE (a.grantee=0 AND n.nspname<>'public') OR (a.grantee IN (SELECT oid FROM roles) AND a.is_grantable))
 AND NOT EXISTS(SELECT FROM expected e LEFT JOIN relations c ON c.nspname=e.schema AND c.relname=e.tbl WHERE c.oid IS NULL OR c.relkind<>'r' OR c.relowner<>(SELECT oid FROM owner) OR c.relrowsecurity OR c.relforcerowsecurity
   OR (e.sel AND NOT has_table_privilege((SELECT oid FROM runtime),c.oid,'SELECT')) OR (e.ins AND NOT has_table_privilege((SELECT oid FROM runtime),c.oid,'INSERT')) OR (e.upd AND NOT has_table_privilege((SELECT oid FROM runtime),c.oid,'UPDATE')) OR (e.del AND NOT has_table_privilege((SELECT oid FROM runtime),c.oid,'DELETE'))
   OR EXISTS(SELECT FROM unnest(e.sc) col WHERE NOT EXISTS(SELECT FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped AND a.attname=col AND has_column_privilege((SELECT oid FROM runtime),c.oid,a.attnum,'SELECT')))
   OR EXISTS(SELECT FROM unnest(e.ic) col WHERE NOT EXISTS(SELECT FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped AND a.attname=col AND has_column_privilege((SELECT oid FROM runtime),c.oid,a.attnum,'INSERT')))
   OR EXISTS(SELECT FROM unnest(e.uc) col WHERE NOT EXISTS(SELECT FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped AND a.attname=col AND has_column_privilege((SELECT oid FROM runtime),c.oid,a.attnum,'UPDATE'))))
 AND NOT EXISTS(SELECT FROM relations c CROSS JOIN roles r LEFT JOIN expected e ON e.schema=c.nspname AND e.tbl=c.relname WHERE c.relkind<>'S' AND (
   has_table_privilege(r.oid,c.oid,'TRUNCATE,TRIGGER,REFERENCES,MAINTAIN') OR (has_table_privilege(r.oid,c.oid,'SELECT') AND NOT coalesce(e.sel,false)) OR (has_table_privilege(r.oid,c.oid,'INSERT') AND NOT coalesce(e.ins,false)) OR (has_table_privilege(r.oid,c.oid,'UPDATE') AND NOT coalesce(e.upd,false)) OR (has_table_privilege(r.oid,c.oid,'DELETE') AND NOT coalesce(e.del,false))))
 AND NOT EXISTS(SELECT FROM relations c CROSS JOIN roles r WHERE c.relkind='S' AND has_sequence_privilege(r.oid,c.oid,'SELECT,UPDATE,USAGE'))
 AND NOT EXISTS(SELECT FROM relations c CROSS JOIN roles r JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped LEFT JOIN expected e ON e.schema=c.nspname AND e.tbl=c.relname WHERE c.relkind<>'S' AND (
   has_column_privilege(r.oid,c.oid,a.attnum,'REFERENCES') OR (has_column_privilege(r.oid,c.oid,a.attnum,'SELECT') AND NOT coalesce(e.sel OR a.attname=ANY(e.sc),false)) OR (has_column_privilege(r.oid,c.oid,a.attnum,'INSERT') AND NOT coalesce(e.ins OR a.attname=ANY(e.ic),false)) OR (has_column_privilege(r.oid,c.oid,a.attnum,'UPDATE') AND NOT coalesce(e.upd OR a.attname=ANY(e.uc),false))))
 AND NOT EXISTS(SELECT FROM relations c CROSS JOIN LATERAL aclexplode(c.relacl) a WHERE a.grantee=0 OR (a.grantee IN (SELECT oid FROM roles) AND a.is_grantable))
 AND NOT EXISTS(SELECT FROM relations c JOIN pg_attribute col ON col.attrelid=c.oid CROSS JOIN LATERAL aclexplode(col.attacl) a WHERE a.grantee=0 OR (a.grantee IN (SELECT oid FROM roles) AND a.is_grantable))
 AND NOT EXISTS(SELECT FROM pg_proc p JOIN namespaces n ON n.oid=p.pronamespace CROSS JOIN roles r WHERE has_function_privilege(r.oid,p.oid,'EXECUTE'))
 -- Global ACLs replace built-in defaults. Per-schema ACLs ADD to that set;
 -- their revocations cannot subtract implicit/global PUBLIC privileges.
 AND NOT EXISTS(SELECT FROM effective_global_defaults d CROSS JOIN LATERAL aclexplode(d.acl) a WHERE a.grantee<>(SELECT oid FROM owner))
 AND NOT EXISTS(SELECT FROM pg_default_acl d CROSS JOIN LATERAL aclexplode(d.defaclacl) a WHERE d.defaclrole=(SELECT oid FROM owner) AND d.defaclnamespace<>0 AND d.defaclobjtype IN ('r','f','S','T','n','L') AND a.grantee<>(SELECT oid FROM owner))`
