package commands

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"justixauto/pkg/events"
	"justixauto/pkg/eventstore"
)

type input struct {
	Label    string          `json:"label"`
	Expected events.Revision `json:"ifMatch"`
	Amount   string          `json:"amountMinor"`
}
type related struct {
	Type     string          `json:"type"`
	ID       string          `json:"id"`
	Revision events.Revision `json:"revision"`
}
type outcome struct {
	ID      string    `json:"id"`
	State   string    `json:"state"`
	Related []related `json:"related,omitempty"`
}

func normalize(v input) (input, error) {
	v.Label = strings.ToLower(strings.TrimSpace(v.Label))
	if v.Label == "" {
		return input{}, errors.New("invalid synthetic label")
	}
	return v, nil
}
func validate(v outcome) error {
	if !validID(v.ID) || v.State != "created" {
		return errors.New("invalid synthetic result")
	}
	for _, r := range v.Related {
		if !validID(r.ID) || r.Type != "fixture" {
			return ErrInvalidReceipt
		}
	}
	return nil
}
func schema(t *testing.T) Schema[input, outcome] {
	t.Helper()
	s, err := NewSchema(events.OwnerInventory, "inventory.fixture.create", false, normalize, validate)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func scope() Scope { return Scope{uuid.NewString(), uuid.NewString(), "", uuid.NewString()} }
func rev(i int64) events.Revision {
	v, e := events.NewRevision(i)
	if e != nil {
		panic(e)
	}
	return v
}
func receipt(t *testing.T, status int) Receipt[outcome] {
	t.Helper()
	r, err := NewReceipt(status, outcome{ID: uuid.NewString(), State: "created", Related: []related{{"fixture", uuid.NewString(), rev(9223372036854775807)}}}, rev(1), uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

var request = input{Label: "fixture", Expected: rev(0), Amount: "12345678901234567890123456789012345678"}

func TestTypedCanonicalDigestAndSecretBoundary(t *testing.T) {
	s := schema(t)
	a, err := s.digest(request)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.digest(input{Label: " FIXTURE ", Amount: request.Amount})
	if err != nil || a != b {
		t.Fatal("normalized retry differs", err)
	}
	c, _ := s.digest(input{Label: request.Label, Expected: rev(1), Amount: request.Amount})
	if a == c {
		t.Fatal("If-Match omitted from digest")
	}
	if a != sha256.Sum256([]byte(`{"amountMinor":"12345678901234567890123456789012345678","ifMatch":"0","label":"fixture"}`)) {
		t.Fatal("deterministic exact-number representation changed")
	}
	key := bytes.Repeat([]byte{42}, 32)
	secret, err := NewSecretSafeSchema("identity.providers.create", true, key, normalize, validate)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := secret.digest(request)
	h := hmac.New(sha256.New, key)
	h.Write([]byte(`{"amountMinor":"12345678901234567890123456789012345678","ifMatch":"0","label":"fixture"}`))
	if !hmac.Equal(d[:], h.Sum(nil)) || a == d {
		t.Fatal("HMAC mismatch")
	}
	key[0] = 1
	e, _ := secret.digest(request)
	if e != d {
		t.Fatal("key aliases caller storage")
	}
	unsafe, err := NewSchema(events.OwnerIdentity, "identity.fixture.create", true, func(v map[string]any) (map[string]any, error) { return v, nil }, validate)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"password", "passwordConfirmation", "credential_hash", "sessionHandle", "access-token", "MFA_Code", "nested"} {
		v := map[string]any{key: "synthetic-sensitive"}
		if key == "nested" {
			v[key] = []any{map[string]any{"secret": "synthetic-sensitive"}}
		}
		if _, err := unsafe.digest(v); !errors.Is(err, ErrInvalidCommand) || strings.Contains(err.Error(), "synthetic-sensitive") {
			t.Fatalf("secret field %s admitted or exposed: %v", key, err)
		}
	}
	if _, err := NewSecretSafeSchema("identity.providers.create", true, []byte("short"), normalize, validate); err == nil {
		t.Fatal("short HMAC key admitted")
	}
}

func TestScopeAndReceiptValidation(t *testing.T) {
	s := schema(t)
	for _, field := range []string{"actor", "company", "target", "key"} {
		p := scope()
		switch field {
		case "actor":
			p.ActorID = ""
		case "company":
			p.CompanyID = ""
		case "target":
			p.TargetID = "bad"
		case "key":
			p.Key = uuid.Nil.String()
		}
		if s.scopeOK(p) {
			t.Fatal("invalid scope admitted", field)
		}
	}
	if _, err := NewSchema(events.OwnerInventory, "inventory.fixture.create", true, normalize, validate); err == nil {
		t.Fatal("foreign global command admitted")
	}
	if _, err := NewSchema(events.OwnerInventory, "retail.fixture.create", false, normalize, validate); err == nil {
		t.Fatal("owner mismatch admitted")
	}
	if _, err := NewSchema(events.OwnerInventory, "inventory.fixture.create", false, normalize, (func(outcome) error)(nil)); err == nil {
		t.Fatal("missing owner validator admitted")
	}
	if _, err := NewLedger((*gorm.DB)(nil), s); !errors.Is(err, eventstore.ErrTransactionRequired) {
		t.Fatal(err)
	}
	for _, status := range []int{200, 201} {
		r := receipt(t, status)
		b := r.Bytes()
		b[0] = 'x'
		if r.Bytes()[0] != '{' {
			t.Fatal("receipt byte alias")
		}
		data, err := r.Data()
		if err != nil {
			t.Fatal(err)
		}
		data.Related[0].ID = "mutated"
		again, _ := r.Data()
		if again.Related[0].ID == "mutated" {
			t.Fatal("receipt data alias")
		}
		if !strings.Contains(string(r.Bytes()), `"revision":"9223372036854775807"`) {
			t.Fatal("revision precision loss")
		}
	}
	p, err := Pending[outcome](uuid.NewString())
	if err != nil || p.Status() != 202 {
		t.Fatal(err)
	}
	if _, err := decodeReceipt(p, validate); err != nil {
		t.Fatal(err)
	}
	for _, status := range []int{0, 202, 204, 400} {
		if _, err := NewReceipt(status, outcome{}, rev(0), uuid.NewString()); err == nil {
			t.Fatal("invalid synchronous status", status)
		}
	}
	for _, body := range []string{`{"data":{},"operationId":"` + uuid.NewString() + `"}`, `{"data":{},"revision":1,"operationId":"` + uuid.NewString() + `"}`, `{"data":{"password":"no"},"revision":"0","operationId":"` + uuid.NewString() + `"}`} {
		if _, err := decodeReceipt(Receipt[outcome]{200, []byte(body)}, validate); err == nil {
			t.Fatal("invalid stored receipt admitted", body)
		}
	}
	if HTTPStatus(fmt.Errorf("wrapped: %w", ErrConflict)) != 409 || HTTPStatus(io.EOF) != 0 {
		t.Fatal("error mapping")
	}
}

type ports struct {
	ledger *Ledger[input, outcome]
	tx     *gorm.DB
}

func run(db *gorm.DB, s Schema[input, outcome]) *eventstore.Transactions[ports] {
	r, err := eventstore.NewTransactions(db, func(tx *gorm.DB) (ports, error) { l, err := NewLedger(tx, s); return ports{l, tx}, err })
	if err != nil {
		panic(err)
	}
	return r
}
func effect(ctx context.Context, p ports, r Receipt[outcome]) (Receipt[outcome], error) {
	data, err := r.Data()
	if err != nil {
		return Receipt[outcome]{}, err
	}
	err = p.tx.WithContext(ctx).Exec("INSERT INTO eventstore.fixture_effects(id) VALUES (?)", data.ID).Error
	return r, err
}
func count(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var n int64
	if err := db.Table("eventstore." + table).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestLedgerPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_COMMAND_LEDGER") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_COMMAND_LEDGER=1 for isolated PostgreSQL 18.6 fixture")
	}
	db := fixtureDB(t)
	ctx := context.Background()
	s := schema(t)
	if _, err := NewLedger(db, s); !errors.Is(err, eventstore.ErrTransactionRequired) {
		t.Fatal("nontransaction admitted", err)
	}
	t.Run("concurrent identical and changed requests", func(t *testing.T) {
		for _, changed := range []bool{false, true} {
			p := scope()
			original := receipt(t, 201)
			var calls atomic.Int32
			var replays atomic.Int32
			errs := make(chan error, 2)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					in := request
					if changed && i == 1 {
						in.Label = "other"
					}
					errs <- run(db, s).Run(ctx, func(u ports) error {
						r, replayed, err := u.ledger.Execute(ctx, p, in, nil, func(ctx context.Context) (Receipt[outcome], error) { calls.Add(1); return effect(ctx, u, original) })
						if replayed {
							replays.Add(1)
						}
						if err == nil {
							got, _ := r.Data()
							want, _ := original.Data()
							if got.ID != want.ID || r.Status() != 201 {
								return errors.New("wrong replay")
							}
						}
						return err
					})
				}(i)
			}
			close(start)
			wg.Wait()
			close(errs)
			conflicts := 0
			for err := range errs {
				if errors.Is(err, ErrConflict) {
					conflicts++
				} else if err != nil {
					t.Fatal(err)
				}
			}
			wantConflicts, wantReplays := 0, int32(1)
			if changed {
				wantConflicts = 1
				wantReplays = 0
			}
			if calls.Load() != 1 || conflicts != wantConflicts || replays.Load() != wantReplays {
				t.Fatalf("calls=%d conflicts=%d replays=%d", calls.Load(), conflicts, replays.Load())
			}
		}
	})
	t.Run("rollback includes receipt and local effect", func(t *testing.T) {
		p := scope()
		beforeR, beforeE := count(t, db, "command_receipts"), count(t, db, "fixture_effects")
		injected := errors.New("abort after receipt")
		err := run(db, s).Run(ctx, func(u ports) error {
			_, _, err := u.ledger.Execute(ctx, p, request, nil, func(ctx context.Context) (Receipt[outcome], error) { return effect(ctx, u, receipt(t, 200)) })
			if err != nil {
				return err
			}
			return injected
		})
		if !errors.Is(err, injected) || count(t, db, "command_receipts") != beforeR || count(t, db, "fixture_effects") != beforeE {
			t.Fatal("partial commit", err)
		}
		if _, found, err := Recover(ctx, db, s, p); err != nil || found {
			t.Fatal(found, err)
		}
		err = run(db, s).Run(ctx, func(u ports) error {
			_, replayed, err := u.ledger.Execute(ctx, p, request, nil, func(ctx context.Context) (Receipt[outcome], error) { return effect(ctx, u, receipt(t, 200)) })
			if replayed {
				t.Fatal("rolled back receipt replayed")
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("missing receipt is not proof of in-flight failure", func(t *testing.T) {
		p := scope()
		candidate := receipt(t, 201)
		entered := make(chan struct{})
		release := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- run(db, s).Run(ctx, func(u ports) error {
				_, _, err := u.ledger.Execute(ctx, p, request, nil, func(ctx context.Context) (Receipt[outcome], error) { return effect(ctx, u, candidate) })
				close(entered)
				<-release
				return err
			})
		}()
		<-entered
		_, found, readErr := Recover(ctx, db, s, p)
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if readErr != nil || found {
			t.Fatal("uncommitted receipt was visible", found, readErr)
		}
		if _, found, err := Recover(ctx, db, s, p); err != nil || !found {
			t.Fatal("committed receipt was not recovered", found, err)
		}
	})
	t.Run("all scope components isolate receipt lookup and execution", func(t *testing.T) {
		original := scope()
		original.TargetID = uuid.NewString()
		for i := 0; i < 6; i++ {
			p := original
			current := s
			switch i {
			case 1:
				p.ActorID = uuid.NewString()
			case 2:
				p.CompanyID = uuid.NewString()
			case 3:
				p.TargetID = uuid.NewString()
			case 4:
				p.Key = uuid.NewString()
			case 5:
				current.command = "inventory.fixture.other"
			}
			if _, found, err := Recover(ctx, db, current, p); err != nil || found {
				t.Fatal("scope leak", i, found, err)
			}
			err := run(db, current).Run(ctx, func(u ports) error {
				_, replayed, err := u.ledger.Execute(ctx, p, request, nil, func(ctx context.Context) (Receipt[outcome], error) { return effect(ctx, u, receipt(t, 200)) })
				if replayed {
					t.Fatal("scope collision", i)
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("pending typed receipt", func(t *testing.T) {
		p := scope()
		pending, _ := Pending[outcome](uuid.NewString())
		for i := 0; i < 2; i++ {
			err := run(db, s).Run(ctx, func(u ports) error {
				r, replayed, err := u.ledger.Execute(ctx, p, request, nil, func(context.Context) (Receipt[outcome], error) { return pending, nil })
				if err == nil && (r.Status() != 202 || replayed != (i == 1)) {
					t.Fatal("wrong pending replay")
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("secret-safe private verification and password-independent recovery", func(t *testing.T) {
		secret, err := NewSecretSafeSchema("identity.providers.create", true, bytes.Repeat([]byte{7}, 32), normalize, validate)
		if err != nil {
			t.Fatal(err)
		}
		// This fixture's schema is identity-owned, so NULL company is admitted.
		p := scope()
		p.CompanyID = ""
		r := receipt(t, 201)
		privatePassword := "synthetic-password"
		supplied := privatePassword
		checks := 0
		effects := 0
		verify := func(_ context.Context, stored Receipt[outcome]) (bool, error) {
			checks++
			a, _ := stored.Data()
			b, _ := r.Data()
			if a.ID != b.ID || supplied != privatePassword {
				return false, nil
			}
			return true, nil
		}
		command := func(in input, check func(context.Context, Receipt[outcome]) (bool, error)) error {
			return run(db, secret).Run(ctx, func(u ports) error {
				_, _, err := u.ledger.Execute(ctx, p, in, check, func(ctx context.Context) (Receipt[outcome], error) { effects++; return effect(ctx, u, r) })
				return err
			})
		}
		if err := command(request, nil); !errors.Is(err, ErrInvalidCommand) {
			t.Fatal("missing verifier admitted", err)
		}
		if err := command(request, verify); err != nil {
			t.Fatal(err)
		}
		if err := command(request, verify); err != nil {
			t.Fatal(err)
		}
		privatePassword = "synthetic-changed-password"
		if err := command(request, verify); !errors.Is(err, ErrConflict) {
			t.Fatal("changed credential replayed", err)
		}
		in := request
		in.Label = "changed"
		before := checks
		if err := command(in, verify); !errors.Is(err, ErrConflict) || checks != before {
			t.Fatal("changed nonsecret mismatch", err)
		}
		if checks != 2 || effects != 1 {
			t.Fatal("credential check/effects", checks, effects)
		}
		unavailable := errors.New("private credential store unavailable")
		if err := command(request, func(context.Context, Receipt[outcome]) (bool, error) { return false, unavailable }); !errors.Is(err, unavailable) || errors.Is(err, ErrConflict) {
			t.Fatal("dependency outage became a credential mismatch", err)
		}
		got, found, err := Recover(ctx, db, secret, p)
		if err != nil || !found || got.Status() != 201 {
			t.Fatal(found, err)
		}
		row, _, err := lookup(ctx, db, secret.command, p)
		if err != nil {
			t.Fatal(err)
		}
		d, _ := secret.digest(request)
		if !hmac.Equal(row.RequestHash, d[:]) || bytes.Contains(row.Receipt, []byte("password")) {
			t.Fatal("unsafe stored data")
		}
	})
	t.Run("lost commit reply resolved only by authoritative receipt", func(t *testing.T) {
		for _, committed := range []bool{false, true} {
			p := scope()
			candidate := receipt(t, 201)
			before := count(t, db, "fixture_effects")
			calls := 0
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			fault := db.Session(&gorm.Session{NewDB: true})
			fault.Statement = &gorm.Statement{DB: fault, ConnPool: faultPool{DB: sqlDB, committed: committed}}
			err = run(fault, s).Run(ctx, func(u ports) error {
				_, _, err := u.ledger.Execute(ctx, p, request, nil, func(ctx context.Context) (Receipt[outcome], error) { calls++; return effect(ctx, u, candidate) })
				return err
			})
			if !errors.Is(err, eventstore.ErrCommitOutcomeUnknown) || calls != 1 {
				t.Fatal("unknown outcome not preserved", err, calls)
			}
			_, found, err := Recover(ctx, db, s, p)
			if err != nil || found != committed {
				t.Fatal("receipt recovery", found, committed, err)
			}
			want := before
			if committed {
				want++
			}
			if count(t, db, "fixture_effects") != want {
				t.Fatal("partial effect")
			}
			err = run(db, s).Run(ctx, func(u ports) error {
				_, replayed, err := u.ledger.Execute(ctx, p, request, nil, func(ctx context.Context) (Receipt[outcome], error) { calls++; return effect(ctx, u, candidate) })
				if replayed != committed {
					t.Fatal("incorrect retry state")
				}
				return err
			})
			if err != nil || count(t, db, "fixture_effects") != before+1 {
				t.Fatal("retry did not converge", err)
			}
		}
	})
	t.Run("invalid outcome rolls back local effect", func(t *testing.T) {
		p := scope()
		before := count(t, db, "fixture_effects")
		err := run(db, s).Run(ctx, func(u ports) error {
			_, _, err := u.ledger.Execute(ctx, p, request, nil, func(ctx context.Context) (Receipt[outcome], error) {
				_, err := effect(ctx, u, receipt(t, 201))
				bad, _ := NewReceipt(201, outcome{ID: uuid.NewString(), State: "not-approved"}, rev(0), uuid.NewString())
				return bad, err
			})
			return err
		})
		if !errors.Is(err, ErrInvalidReceipt) || count(t, db, "fixture_effects") != before {
			t.Fatal("invalid outcome committed", err)
		}
	})
}

type faultPool struct {
	*sql.DB
	committed bool
}

func (p faultPool) BeginTx(ctx context.Context, options *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return &faultTx{tx, p.committed}, nil
}

type faultTx struct {
	*sql.Tx
	committed bool
}

func (t faultTx) Commit() error {
	var err error
	if t.committed {
		err = t.Tx.Commit()
	} else {
		err = t.Tx.Rollback()
	}
	if err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}

func fixtureDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := "justixauto-t017-" + uuid.NewString()
	password := "synthetic-" + uuid.NewString()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	image := "postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"
	out, err := exec.CommandContext(ctx, "docker", "run", "-d", "--name", name, "-e", "POSTGRES_PASSWORD="+password, "-e", "POSTGRES_DB=justix_identity", "-p", "127.0.0.1::5432", image).CombinedOutput()
	if err != nil {
		t.Fatalf("start owned fixture: %v %s", err, out)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "docker", "rm", "-f", name).CombinedOutput(); err != nil {
			t.Errorf("remove owned fixture: %v %s", err, out)
		}
	})
	psql := func(query string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "docker", append([]string{"exec", "-i", name, "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "justix_identity"}, args...)...)
		cmd.Stdin = strings.NewReader(query)
		return cmd.CombinedOutput()
	}
	for {
		if _, err := psql("SELECT 1"); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("fixture startup timeout")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if out, err := psql("SHOW server_version_num"); err != nil || strings.TrimSpace(string(out)) != "180006" {
		t.Fatalf("fixture version: %v %s", err, out)
	}
	if out, err := psql("CREATE ROLE justix_identity_runtime LOGIN PASSWORD '" + password + "';"); err != nil {
		t.Fatalf("create runtime: %v %s", err, out)
	}
	schema, err := os.ReadFile("../eventstore/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := psql(string(schema), "-v", "owner_service=identity", "-v", "runtime_role=justix_identity_runtime"); err != nil {
		t.Fatalf("install schema: %v %s", err, out)
	}
	if out, err := psql("CREATE TABLE eventstore.fixture_effects(id uuid PRIMARY KEY); GRANT SELECT,INSERT ON eventstore.fixture_effects TO justix_identity_runtime;"); err != nil {
		t.Fatalf("fixture effect table: %v %s", err, out)
	}
	out, err = exec.CommandContext(ctx, "docker", "port", name, "5432/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	address := strings.TrimSpace(string(out))
	if !strings.HasPrefix(address, "127.0.0.1:") || strings.Contains(address, "\n") {
		t.Fatalf("unexpected binding %q", address)
	}
	dsn := fmt.Sprintf("host=127.0.0.1 port=%s user=justix_identity_runtime password=%s dbname=justix_identity sslmode=disable", strings.TrimPrefix(address, "127.0.0.1:"), password)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(8)
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

// Compile-time check that revision serialization remains a JSON string.
var _ json.Marshaler = events.Revision{}
