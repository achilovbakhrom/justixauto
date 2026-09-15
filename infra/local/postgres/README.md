# Local PostgreSQL owner isolation

This bootstrap creates one login and one database for each approved owner:
`identity`, `inventory`, `commerce`, `retail`, `financing`, `insurance`, and
`documents`. Each login owns only its matching `justix_<owner>` database.
Connection and schema privileges are revoked from `PUBLIC` and from every other
service login.

The supported image is PostgreSQL 18.6 on Alpine 3.24, pinned by the approved
ADR-13 multi-platform index digest:

```text
docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2
```

## Isolated local fixture

Supply fresh synthetic credentials in the shell that starts the container. The
SQL script deliberately has no password fallback. These values are only for the
disposable T-005 fixture and must not be reused for an application or deployment.

```sh
export JUSTIXAUTO_TEST_POSTGRES_ADMIN_PASSWORD="$(openssl rand -hex 24)"
export JUSTIXAUTO_IDENTITY_DB_PASSWORD="$(openssl rand -hex 24)"
export JUSTIXAUTO_INVENTORY_DB_PASSWORD="$(openssl rand -hex 24)"
export JUSTIXAUTO_COMMERCE_DB_PASSWORD="$(openssl rand -hex 24)"
export JUSTIXAUTO_RETAIL_DB_PASSWORD="$(openssl rand -hex 24)"
export JUSTIXAUTO_FINANCING_DB_PASSWORD="$(openssl rand -hex 24)"
export JUSTIXAUTO_INSURANCE_DB_PASSWORD="$(openssl rand -hex 24)"
export JUSTIXAUTO_DOCUMENTS_DB_PASSWORD="$(openssl rand -hex 24)"
```

Choose a unique container name, then start the fixture on the approved loopback
port. The bind address is explicit so PostgreSQL is not exposed on LAN interfaces.

```sh
docker run --name justixauto-t005-postgres \
  -p 127.0.0.1:55432:5432 \
  -e POSTGRES_PASSWORD="$JUSTIXAUTO_TEST_POSTGRES_ADMIN_PASSWORD" \
  -e JUSTIXAUTO_IDENTITY_DB_PASSWORD \
  -e JUSTIXAUTO_INVENTORY_DB_PASSWORD \
  -e JUSTIXAUTO_COMMERCE_DB_PASSWORD \
  -e JUSTIXAUTO_RETAIL_DB_PASSWORD \
  -e JUSTIXAUTO_FINANCING_DB_PASSWORD \
  -e JUSTIXAUTO_INSURANCE_DB_PASSWORD \
  -e JUSTIXAUTO_DOCUMENTS_DB_PASSWORD \
  -v "$PWD/infra/local/postgres/init-owners.sql:/docker-entrypoint-initdb.d/10-justix-owners.sql:ro" \
  docker.io/library/postgres:18.6-alpine3.24@sha256:d3e1620b530c944afa6e887d22eb899824da68e19c52024bf98f5220c88a65b2
```

Wait for `database system is ready to accept connections`. Export the admin DSN
for the integration harness; owner DSNs are derived from the loopback endpoint
and the seven injected passwords without printing them:

```sh
export JUSTIXAUTO_TEST_POSTGRES_ADMIN_DSN="postgres://postgres:$JUSTIXAUTO_TEST_POSTGRES_ADMIN_PASSWORD@127.0.0.1:55432/postgres?sslmode=disable"
bash tools/go.sh test -race ./tests/integration
```

The test verifies the exact server version, restricted role attributes, catalog
privileges, all 42 forbidden cross-owner connection pairs, owner-local writes,
rollback behavior, and concurrent use of the same table name in seven databases.
It skips when the admin DSN is absent, so an ordinary unit-test run never reaches
an unrequested database. It rejects a non-loopback admin endpoint.

The bootstrap failure regression runs 14 fresh containers: one missing and one
explicitly empty case for every required owner password. It publishes no host
port and removes each disposable container. Enable this slower Docker check only
when requested:

```sh
JUSTIXAUTO_TEST_POSTGRES_BOOTSTRAP=1 bash tools/go.sh test -race -count=1 \
  -run TestPostgresInitRejectsEveryMissingOrEmptyCredential ./tests/integration
```

This task provisions empty owner stores only. It does not run owner migrations,
seed users or vehicles, create production credentials, or establish event delivery
guarantees. Database owners can run their own later migration set; service runtime
roles may be narrowed further by those owner-specific migration tasks.
