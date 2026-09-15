package integration_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type databaseOwner struct {
	name        string
	role        string
	database    string
	passwordEnv string
}

var databaseOwners = []databaseOwner{
	{name: "identity", role: "justix_identity", database: "justix_identity", passwordEnv: "JUSTIXAUTO_IDENTITY_DB_PASSWORD"},
	{name: "inventory", role: "justix_inventory", database: "justix_inventory", passwordEnv: "JUSTIXAUTO_INVENTORY_DB_PASSWORD"},
	{name: "commerce", role: "justix_commerce", database: "justix_commerce", passwordEnv: "JUSTIXAUTO_COMMERCE_DB_PASSWORD"},
	{name: "retail", role: "justix_retail", database: "justix_retail", passwordEnv: "JUSTIXAUTO_RETAIL_DB_PASSWORD"},
	{name: "financing", role: "justix_financing", database: "justix_financing", passwordEnv: "JUSTIXAUTO_FINANCING_DB_PASSWORD"},
	{name: "insurance", role: "justix_insurance", database: "justix_insurance", passwordEnv: "JUSTIXAUTO_INSURANCE_DB_PASSWORD"},
	{name: "documents", role: "justix_documents", database: "justix_documents", passwordEnv: "JUSTIXAUTO_DOCUMENTS_DB_PASSWORD"},
}

const postgresImage = "docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2"

type postgresFixture struct {
	admin     *pgx.ConnConfig
	passwords map[string]string
}

func loadPostgresFixture(t *testing.T) postgresFixture {
	t.Helper()
	dsn := os.Getenv("JUSTIXAUTO_TEST_POSTGRES_ADMIN_DSN")
	if dsn == "" {
		t.Skip("JUSTIXAUTO_TEST_POSTGRES_ADMIN_DSN is not set; isolated PostgreSQL fixture not requested")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse PostgreSQL test DSN: %v", err)
	}
	if !isLoopbackHost(config.Host) {
		t.Fatalf("PostgreSQL integration fixture must be loopback-only, got host %q", config.Host)
	}
	passwords := make(map[string]string, len(databaseOwners))
	for _, owner := range databaseOwners {
		password := os.Getenv(owner.passwordEnv)
		if password == "" {
			t.Fatalf("%s is required when the PostgreSQL fixture is enabled", owner.passwordEnv)
		}
		passwords[owner.name] = password
	}
	return postgresFixture{admin: config, passwords: passwords}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func TestPostgresInitRejectsEveryMissingOrEmptyCredential(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_POSTGRES_BOOTSTRAP") != "1" {
		t.Skip("JUSTIXAUTO_TEST_POSTGRES_BOOTSTRAP=1 is required for destructive disposable-container checks")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("find Docker CLI: %v", err)
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate integration test source")
	}
	initScript := filepath.Join(filepath.Dir(source), "../../infra/local/postgres/init-owners.sql")
	initScript, err := filepath.Abs(initScript)
	if err != nil {
		t.Fatalf("resolve init script: %v", err)
	}

	for _, missing := range databaseOwners {
		missing := missing
		for _, mode := range []string{"missing", "empty"} {
			mode := mode
			t.Run(missing.name+"/"+mode, func(t *testing.T) {
				containerName := "justixauto-t005-credential-" + strings.ReplaceAll(uuid.NewString(), "-", "")
				args := []string{
					"run", "--rm", "--name", containerName,
					"-e", "POSTGRES_PASSWORD=t005-bootstrap-admin-" + uuid.NewString(),
				}
				for _, owner := range databaseOwners {
					switch {
					case owner.name != missing.name:
						args = append(args, "-e", owner.passwordEnv+"=t005-"+owner.name+"-"+uuid.NewString())
					case mode == "empty":
						args = append(args, "-e", owner.passwordEnv+"=")
					}
				}
				args = append(args,
					"-v", initScript+":/docker-entrypoint-initdb.d/10-justix-owners.sql:ro",
					postgresImage,
				)

				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				output, runErr := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
				// A client timeout can leave the daemon-side container alive. Cleanup is
				// scoped to the random name created by this test.
				exec.Command("docker", "rm", "-f", containerName).Run() //nolint:errcheck -- --rm normally removed it
				if ctx.Err() != nil {
					t.Fatalf("credential guard timed out: %v", ctx.Err())
				}
				if runErr == nil {
					t.Fatalf("entrypoint accepted %s %s", mode, missing.passwordEnv)
				}
				log := string(output)
				if strings.Contains(log, "CREATE ROLE") || strings.Contains(log, "CREATE DATABASE") ||
					strings.Contains(log, "PostgreSQL init process complete") {
					t.Fatalf("entrypoint created owner state after %s %s:\n%s", mode, missing.passwordEnv, log)
				}
				if !strings.Contains(log, missing.passwordEnv) {
					t.Fatalf("entrypoint failure did not identify %s %s:\n%s", mode, missing.passwordEnv, log)
				}
				if mode == "empty" && !strings.Contains(log, missing.passwordEnv+" is required and must not be empty") {
					t.Fatalf("entrypoint did not identify empty %s:\n%s", missing.passwordEnv, log)
				}
			})
		}
	}
}

func connectAs(t *testing.T, fixture postgresFixture, owner databaseOwner, database string) (*pgx.Conn, error) {
	t.Helper()
	config := fixture.admin.Copy()
	config.User = owner.role
	config.Password = fixture.passwords[owner.name]
	config.Database = database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return pgx.ConnectConfig(ctx, config)
}

func TestPostgresOwnerCatalogIsolation(t *testing.T) {
	fixture := loadPostgresFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, fixture.admin)
	if err != nil {
		t.Fatalf("connect to isolated PostgreSQL fixture: %v", err)
	}
	defer admin.Close(context.Background())

	var version string
	if err := admin.QueryRow(ctx, "SHOW server_version").Scan(&version); err != nil {
		t.Fatalf("read PostgreSQL version: %v", err)
	}
	if version != "18.6" {
		t.Fatalf("server version = %q, want exact approved version 18.6", version)
	}

	for _, owner := range databaseOwners {
		t.Run(owner.name, func(t *testing.T) {
			var databaseOwner string
			if err := admin.QueryRow(ctx, `
				SELECT pg_get_userbyid(datdba)
				FROM pg_database
				WHERE datname = $1`, owner.database).Scan(&databaseOwner); err != nil {
				t.Fatalf("read database owner: %v", err)
			}
			if databaseOwner != owner.role {
				t.Fatalf("database owner = %q, want %q", databaseOwner, owner.role)
			}

			var login, superuser, inherit, createRole, createDB, replication, bypassRLS bool
			if err := admin.QueryRow(ctx, `
				SELECT rolcanlogin, rolsuper, rolinherit, rolcreaterole, rolcreatedb, rolreplication, rolbypassrls
				FROM pg_roles
				WHERE rolname = $1`, owner.role).Scan(
				&login, &superuser, &inherit, &createRole, &createDB, &replication, &bypassRLS,
			); err != nil {
				t.Fatalf("read role attributes: %v", err)
			}
			if !login || superuser || inherit || createRole || createDB || replication || bypassRLS {
				t.Fatalf("unsafe role attributes: login=%t superuser=%t inherit=%t createRole=%t createDB=%t replication=%t bypassRLS=%t",
					login, superuser, inherit, createRole, createDB, replication, bypassRLS)
			}

			for _, actor := range databaseOwners {
				var canConnect bool
				if err := admin.QueryRow(ctx,
					"SELECT has_database_privilege($1, $2, 'CONNECT')",
					actor.role, owner.database,
				).Scan(&canConnect); err != nil {
					t.Fatalf("read %s -> %s CONNECT privilege: %v", actor.name, owner.name, err)
				}
				want := actor.name == owner.name
				if canConnect != want {
					t.Errorf("%s -> %s CONNECT = %t, want %t", actor.name, owner.name, canConnect, want)
				}
			}
		})
	}
}

func TestPostgresRejectsEveryCrossOwnerConnection(t *testing.T) {
	fixture := loadPostgresFixture(t)
	for _, actor := range databaseOwners {
		actor := actor
		t.Run(actor.name, func(t *testing.T) {
			for _, target := range databaseOwners {
				if target.name == actor.name {
					continue
				}
				t.Run(target.name, func(t *testing.T) {
					conn, err := connectAs(t, fixture, actor, target.database)
					if err == nil {
						conn.Close(context.Background())
						t.Fatalf("%s unexpectedly connected to %s database", actor.name, target.name)
					}
					var pgErr *pgconn.PgError
					if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
						t.Fatalf("cross-owner connection failed with %T/%v, want SQLSTATE 42501", err, err)
					}
				})
			}
		})
	}
}

func TestPostgresOwnersWriteAndRollbackIndependently(t *testing.T) {
	fixture := loadPostgresFixture(t)
	probeTable := pgx.Identifier{"t005_probe_" + strings.ReplaceAll(uuid.NewString(), "-", "")}.Sanitize()
	start := make(chan struct{})
	results := make(chan error, len(databaseOwners))
	var ready sync.WaitGroup
	ready.Add(len(databaseOwners))

	for _, owner := range databaseOwners {
		owner := owner
		go func() {
			conn, err := connectAs(t, fixture, owner, owner.database)
			if err != nil {
				ready.Done()
				results <- fmt.Errorf("%s connect to own database: %w", owner.name, err)
				return
			}
			defer conn.Close(context.Background())
			ready.Done()
			<-start

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if _, err := conn.Exec(ctx, "CREATE TABLE "+probeTable+" (owner_name text PRIMARY KEY)"); err != nil {
				results <- fmt.Errorf("%s create owner-local table: %w", owner.name, err)
				return
			}
			defer conn.Exec(context.Background(), "DROP TABLE IF EXISTS "+probeTable) //nolint:errcheck -- best-effort test cleanup
			if _, err := conn.Exec(ctx, "INSERT INTO "+probeTable+" (owner_name) VALUES ($1)", owner.name); err != nil {
				results <- fmt.Errorf("%s insert owner-local row: %w", owner.name, err)
				return
			}
			var got string
			if err := conn.QueryRow(ctx, "SELECT owner_name FROM "+probeTable).Scan(&got); err != nil || got != owner.name {
				results <- fmt.Errorf("%s read isolated row: got %q, err %v", owner.name, got, err)
				return
			}

			tx, err := conn.Begin(ctx)
			if err != nil {
				results <- fmt.Errorf("%s begin rollback probe: %w", owner.name, err)
				return
			}
			if _, err := tx.Exec(ctx, "UPDATE "+probeTable+" SET owner_name = $1", "rolled-back"); err != nil {
				tx.Rollback(ctx) //nolint:errcheck -- preserve original failure
				results <- fmt.Errorf("%s update rollback probe: %w", owner.name, err)
				return
			}
			if err := tx.Rollback(ctx); err != nil {
				results <- fmt.Errorf("%s rollback probe: %w", owner.name, err)
				return
			}
			if err := conn.QueryRow(ctx, "SELECT owner_name FROM "+probeTable).Scan(&got); err != nil || got != owner.name {
				results <- fmt.Errorf("%s rollback changed row: got %q, err %v", owner.name, got, err)
				return
			}
			results <- nil
		}()
	}

	ready.Wait()
	close(start)
	for range databaseOwners {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
}
