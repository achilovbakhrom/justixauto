# JustixAuto — local development. `make help` lists the targets.
#
# First run:   make env && make dev        → http://127.0.0.1:$(API_PORT)
# Hot reload:  make api  (one terminal) + make web APP=realization (another)
#
# Ports are configurable when the defaults are taken:
#   make env POSTGRES_PORT=55433 API_PORT=8090

SHELL := /bin/bash
.DEFAULT_GOAL := help

-include .env
export

POSTGRES_PORT ?= 55432
API_PORT      ?= 8080
APP           ?= realization
COMPOSE       := docker compose --env-file .env -f infra/local/compose.yaml
GO            := bash tools/go.sh
LINT          := $(GO) tool -modfile=tools/lint/go.mod golangci-lint
API_URL       = http://$(or $(HTTP_ADDR),127.0.0.1:$(API_PORT))

.PHONY: help env db-up db-down db-reset db-psql migrate migrate-down \
        api web web-install web-build dev test test-go test-web lint typecheck check \
        openapi openapi-check hooks lint-go fmt deadcode image k8s-up k8s-down

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ---- setup ----

env: ## Create .env from .env.example with generated secrets (never overwrites)
	@if [ -f .env ]; then echo ".env exists — edit it or delete it first"; exit 0; fi; \
	pw=$$(openssl rand -hex 16); key=$$(openssl rand -base64 32); \
	sed -e "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=$$pw|" \
	    -e "s|^DATABASE_URL=.*|DATABASE_URL=postgres://justixauto:$$pw@127.0.0.1:$(POSTGRES_PORT)/justixauto?sslmode=disable|" \
	    -e "s|^HTTP_ADDR=.*|HTTP_ADDR=127.0.0.1:$(API_PORT)|" \
	    -e "s|^MFA_KEY=.*|MFA_KEY=$$key|" \
	    -e "s|^ALLOWED_ORIGINS=.*|ALLOWED_ORIGINS=http://127.0.0.1:5173,http://127.0.0.1:5174,http://127.0.0.1:5175,http://127.0.0.1:5176|" \
	    .env.example > .env; \
	printf 'POSTGRES_PORT=%s\nMFA_DISABLED=true\n' "$(POSTGRES_PORT)" >> .env; \
	echo "created .env (Postgres on $(POSTGRES_PORT), API on $(API_PORT), two-factor off for local use)"

web-install: ## Install web dependencies (npm ci)
	npm ci --ignore-scripts

# ---- database ----

db-up: ## Start PostgreSQL (Docker) and wait until it accepts connections
	@test -f .env || { echo "run: make env"; exit 1; }
	$(COMPOSE) up -d --wait postgres

db-down: ## Stop PostgreSQL, keep data
	$(COMPOSE) stop

db-reset: ## Delete the local database volume (all data!) and start again
	$(COMPOSE) down -v
	$(MAKE) db-up migrate

db-psql: ## Open psql in the database container
	$(COMPOSE) exec postgres psql -U justixauto -d justixauto

migrate: ## Apply SQL migrations
	$(GO) run ./cmd/migrate up

migrate-down: ## Roll back the last migration
	$(GO) run ./cmd/migrate down 1

# ---- run ----

api: db-up migrate ## Run the API (http://$HTTP_ADDR); serves built web apps if present
	WEB_DIR=$(or $(WEB_DIR),web/apps) $(GO) run ./cmd/api

web: ## Vite dev server with hot reload for one app: make web APP=realization|financing|insurance|admin
	JUSTIX_API=$(API_URL) npm run dev --workspace web/apps/$(APP)

web-build: ## Build the four web apps into web/apps/*/dist
	npm run build:apps

dev: web-build api ## Build the web apps and run everything on one port

# ---- checks ----

test-go: ## Go tests incl. database and S3 (throwaway PostgreSQL + MinIO containers)
	bash tools/test-go.sh

test-web: ## Web unit tests
	npm run test:unit

test: test-go test-web ## All tests

typecheck: ## TypeScript typecheck
	npm run typecheck

lint: openapi-check lint-go deadcode ## golangci-lint + deadcode + ESLint + Prettier + knip + OpenAPI spec up to date
	npm run lint
	npm run format:check
	npm run knip

lint-go: ## golangci-lint (config: .golangci.yml)
	$(LINT) run ./...

fmt: ## Format Go (gofumpt, goimports) and web (Prettier) sources
	$(LINT) fmt ./cmd/... ./internal/... ./migrations/...
	npm run format

deadcode: ## Fail on unreachable Go functions (tests count as callers)
	@out=$$($(GO) tool -modfile=tools/lint/go.mod deadcode -test ./...) || { echo "$$out"; exit 1; }; \
	  if [ -n "$$out" ]; then echo "$$out"; exit 1; fi

# ---- API docs ----

openapi: ## Regenerate the OpenAPI spec from handler annotations (served at /api/docs with API_DOCS=true)
	$(GO) tool swag fmt --dir ./cmd/api,./internal
	$(GO) tool swag init --quiet --dir ./cmd/api,./internal --generalInfo main.go \
	  --output internal/pkg/apidocs --outputTypes json --parseInternal --parseDependency --requiredByDefault

openapi-check: openapi ## Fail when the committed spec differs from the annotations
	@git diff --quiet -- internal/pkg/apidocs/swagger.json || \
	  { echo "OpenAPI spec is out of date: review and commit internal/pkg/apidocs/swagger.json"; exit 1; }

hooks: ## Install the Git hooks (lefthook.yml); npm install does this too
	npx lefthook install

check: lint typecheck test ## Everything CI would run

# ---- containers / Kubernetes ----

image: ## Build the Docker image justixauto:dev
	docker build -t justixauto:dev --build-arg VERSION=$$(git rev-parse --short HEAD) .

k8s-up: ## Local kind cluster: 2 API replicas + Postgres, MinIO, Loki, Tempo, Prometheus, Grafana
	bash infra/kind/up.sh

k8s-down: ## Delete the local kind cluster
	bash infra/kind/down.sh
