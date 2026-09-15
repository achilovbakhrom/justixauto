package migrationqa

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	migrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
)

// This exercises the real engine's public call protocol. It deliberately does
// not implement PostgreSQL or claim to prove the proposed driver's atomicity.
type observedDB struct {
	calls []string
	fail string
	dirty bool
	ran bool
	lockDelay time.Duration
}

var probeErr = errors.New("synthetic boundary failure")
var _ database.Driver = (*observedDB)(nil)

func (d *observedDB) record(s string) error {
	d.calls = append(d.calls, s)
	if d.fail == s { return probeErr }
	return nil
}
func (d *observedDB) Open(string) (database.Driver, error) { return nil, probeErr }
func (d *observedDB) Close() error { return d.record("close") }
func (d *observedDB) Lock() error { time.Sleep(d.lockDelay); return d.record("lock") }
func (d *observedDB) Unlock() error { return d.record("unlock") }
func (d *observedDB) Drop() error { return probeErr }
func (d *observedDB) Version() (int, bool, error) { return 1, d.dirty, d.record("version") }
func (d *observedDB) SetVersion(v int, dirty bool) error {
	err := d.record(fmt.Sprintf("set:%d:%t", v, dirty))
	if err != nil { return err }
	if !dirty && !d.ran { return probeErr }
	d.dirty = dirty
	return nil
}
func (d *observedDB) Run(r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil { return err }
	if string(b) != "BEGIN; SELECT 12; COMMIT;" { return fmt.Errorf("altered bytes: %q", b) }
	if err := d.record("run:original-bytes"); err != nil { return err }
	d.ran = true
	return nil
}

type sealedSource struct { absent bool }
var _ source.Driver = (*sealedSource)(nil)
func (*sealedSource) Open(string) (source.Driver, error) { return nil, probeErr }
func (*sealedSource) Close() error { return nil }
func (*sealedSource) First() (uint, error) { return 1, nil }
func (*sealedSource) Prev(uint) (uint, error) { return 0, os.ErrNotExist }
func (*sealedSource) Next(v uint) (uint, error) {
	if v == 1 { return 12, nil }; return 0, os.ErrNotExist
}
func (s *sealedSource) ReadUp(v uint) (io.ReadCloser, string, error) {
	if v != 1 && v != 12 || v == 12 && s.absent { return nil, "", os.ErrNotExist }
	return io.NopCloser(strings.NewReader("BEGIN; SELECT 12; COMMIT;")), "verified.sql", nil
}
func (*sealedSource) ReadDown(uint) (io.ReadCloser, string, error) { return nil, "", os.ErrNotExist }

func TestRealEngineProtocol(t *testing.T) {
	for _, tc := range []struct { name, fail string; absent, dirty bool; want []string }{
		{"success", "", false, false, []string{"lock", "version", "set:12:true", "run:original-bytes", "set:12:false", "unlock"}},
		{"dirty-write-error", "set:12:true", false, false, []string{"lock", "version", "set:12:true", "unlock"}},
		{"DDL-error", "run:original-bytes", false, false, []string{"lock", "version", "set:12:true", "run:original-bytes", "unlock"}},
		{"finalization-error", "set:12:false", false, false, []string{"lock", "version", "set:12:true", "run:original-bytes", "set:12:false", "unlock"}},
		{"missing-body-clean-guard", "", true, false, []string{"lock", "version", "set:12:true", "set:12:false", "unlock"}},
		{"retained-dirty-rejected", "", false, true, []string{"lock", "version", "unlock"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &observedDB{fail: tc.fail, dirty: tc.dirty}
			m, err := migrate.NewWithInstance("sealed", &sealedSource{absent: tc.absent}, "owned-pgx", d)
			if err != nil { t.Fatal(err) }
			err = m.Up()
			if (err == nil) != (tc.name == "success") { t.Fatalf("unexpected error: %v", err) }
			if !reflect.DeepEqual(d.calls, tc.want) { t.Fatalf("calls %v, want %v", d.calls, tc.want) }
			t.Logf("calls=%v error=%v", d.calls, err)
			if s, db := m.Close(); s != nil || db != nil { t.Fatalf("close: %v %v", s, db) }
		})
	}
}

func TestForceCannotBypassPendingRunGuard(t *testing.T) {
	d := &observedDB{}
	m, _ := migrate.NewWithInstance("sealed", &sealedSource{}, "owned-pgx", d)
	if err := m.Force(12); !errors.Is(err, probeErr) { t.Fatalf("Force unexpectedly passed: %v", err) }
	if !reflect.DeepEqual(d.calls, []string{"lock", "set:12:false", "unlock"}) { t.Fatal(d.calls) }
	t.Log(d.calls)
	m.Close()
}

func TestDriverBoundPrecedesEngineLockTimeout(t *testing.T) {
	d := &observedDB{fail: "lock", lockDelay: 10*time.Millisecond}
	m, _ := migrate.NewWithInstance("sealed", &sealedSource{}, "owned-pgx", d)
	m.LockTimeout = time.Second
	if err := m.Up(); !errors.Is(err, probeErr) { t.Fatalf("driver bound lost: %v", err) }
	if !reflect.DeepEqual(d.calls, []string{"lock"}) { t.Fatal(d.calls) }
	t.Log("driver-controlled bounded failure surfaced before the longer engine timeout")
	m.Close()
}
