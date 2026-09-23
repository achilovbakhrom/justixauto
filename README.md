# JustixAuto

Go backend (modular monolith: Echo + GORM + PostgreSQL) and four React apps
(Realization, Financing, Insurance, Admin), with the business rules and
runnable HTML mocks they are built from. Open **this folder**, not the
enclosing `startups` repository.

## Start here

- [Backend architecture — ADR-14](docs/justix-auto/adr-14-classic-modular-monolith.md)
- [Business logic](docs/justix-auto/business-logic.md)
- [Open product decisions](docs/justix-auto/open-decisions.md)
- [Mock navigation map](docs/justix-auto/mock-map.md)
- [Development status](docs/justix-auto/dev/dev-state.md)
- [Agent workflow](docs/justix-auto/dev/workflow.md)

## Preview the compact mocks

```sh
npm ci --ignore-scripts
npm run mocks:verify
npm run mocks:test
npm run mocks:serve
```

Open `http://127.0.0.1:4180/`, `/finance/`, `/insurance/`, `/admin/`.
The server binds only to loopback and exposes only runtime files in the mock
manifest. The four apps exchange **demo** data in the same browser/origin.
No real personal documents, passwords, financial actions or external APIs.
Use another `MOCK_PORT` if 4180 is occupied; do not kill an unrelated server.

## Quick start (Makefile)

```sh
make env                 # .env with generated secrets (ports: make env POSTGRES_PORT=55433 API_PORT=8090)
make dev                 # PostgreSQL + migrations + built web apps + API on one port
make bootstrap-admin     # once per database: first platform admin (asks for a password)
```

For UI work with hot reload run `make api` in one terminal and
`make web APP=realization` (or `financing`, `insurance`, `admin`) in another;
Vite proxies `/api` to the API. `make help` lists everything: tests
(`make test`, `make check`), database (`make db-psql`, `make db-reset`), the
Docker image (`make image`) and the local Kubernetes stack (`make k8s-up`).

## Run the backend

```sh
cp .env.example .env                    # local-only credentials
docker compose --env-file .env -f infra/local/compose.yaml up -d
set -a && . ./.env && set +a
bash tools/go.sh run ./cmd/migrate up   # apply SQL migrations
# once: first platform administrator (password on stdin, single-use)
printf '%s\n' 'choose-a-long-password' | bash tools/go.sh run ./cmd/bootstrap-admin \
  -login admin -email admin@example.com -name "Platform Admin"
bash tools/go.sh run ./cmd/api          # http://127.0.0.1:8080/healthz
```

`.env` needs your own `MFA_KEY` (`openssl rand -base64 32`); it encrypts
two-factor secrets, so keep it stable and secret. `MFA_DISABLED=true` switches
two-factor authentication off for local development only; business rules
require it for sensitive actions everywhere else.

Sign in with `POST /api/v1/identity/session/login` `{"login","password"}`; the
session is an HttpOnly cookie. Every state-changing request sends the
`X-CSRF-Token` returned by login / `GET /api/v1/identity/session`, and every
authenticated POST (outside `/session/`) sends a fresh `Idempotency-Key` UUID.
Administrators must set up TOTP (`POST /session/mfa/enrollment`, then
`…/{id}/confirm`) before sensitive actions; after that, login returns an MFA
challenge answered with `POST /session/mfa/verify`, and sensitive actions need
a code from the last 5 minutes (`POST /session/mfa/step-up`).

## Run the web apps

Four cabinets share one sign-in and API: Realization (sellers, `/`), Financing
(banks and MFOs, `/finance/`), Insurance (`/insurance/`) and Admin (platform,
`/admin/`). Build them once and let the API serve them:

```sh
npm ci --ignore-scripts
npm run build:apps
WEB_DIR=web/apps bash tools/go.sh run ./cmd/api   # http://127.0.0.1:8080/
```

For UI work run one app with hot reload instead; it proxies `/api/` to the
API (`JUSTIX_API`, default `http://127.0.0.1:8080`):

```sh
npm run dev --workspace web/apps/realization      # also: financing, insurance, admin
```

First steps after bootstrap: sign in at `/admin/`, enable two-factor
protection (“Безопасность”), create seller / bank / MFO / insurance companies
with their first administrator and activate them. A company administrator
manages only company details and branches; business work needs a role — create
one under “Роли и права” (e.g. all `inventory.*`, `commerce.*`, `retail.*`
permissions for a seller) and assign it to the user.

Shared UI code lives in `web/packages/kit` (HTTP client with CSRF,
Idempotency-Key and If-Match, session gate with MFA, shell, forms, tables and
the insurance/financing application views). Checks:

```sh
npm run typecheck && npm run lint && npm run test:unit && npm run build
```

Tests:

```sh
bash tools/go.sh vet ./...
bash tools/test-go.sh        # all tests incl. database and S3 tests (throwaway PostgreSQL + MinIO)
bash tools/go.sh test ./...  # without Docker: database tests are skipped
```

New migration: add the next numbered pair
`migrations/00000N_<name>.up.sql` / `.down.sql`; never edit an applied one.

## Layout

```text
cmd/api/                  HTTP API entry point (composition root)
cmd/migrate/              migration CLI (up / down [N] / version)
cmd/bootstrap-admin/      single-use creation of the first platform admin
internal/modules/<name>/  one module: handler → service → repository, model
internal/platform/        shared tech: config, database, HTTP server, errors
migrations/               versioned SQL migrations (embedded)
web/                      four React apps and shared packages (kit = shared UI)
docs/justix-auto/         business rules, decisions, mocks and dev state
tools/                    Go wrapper, mock tooling, agent/Git helpers
infra/local/              local PostgreSQL (Docker Compose)
AGENTS.md / CLAUDE.md     rules for coding agents
```

Mock rebuilding requires the original readable source path:
`npm run mocks:build -- /absolute/path/to/prototype`. The builder refuses an
existing output directory; preserve/review the old generated bundle first.
Original source and its hashes are recorded in `mocks/manifest.json` and
`reference/source-manifest.json`. Original files were copied, not deleted/moved
out of spec-team. Shared dependencies remain single files and classic-script
order is preserved; only local variable names and whitespace are minified.
