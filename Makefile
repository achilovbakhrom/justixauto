# JustixAuto — local development. `make help` lists the targets.
#
# First run:   make env && make dev   → API + four web apps with hot reload
#              (the URLs are printed on start; Ctrl-C stops everything)
# One app:     make api (one terminal) + make web APP=realization (another)
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
COMPOSE       := docker compose --env-file .env -f deploy/local/compose.yaml
GO            := bash tools/go.sh
LINT          := $(GO) tool -modfile=tools/lint/go.mod golangci-lint
API_URL       = http://$(or $(HTTP_ADDR),127.0.0.1:$(API_PORT))

.PHONY: help env db-up db-down db-reset db-psql migrate migrate-down \
        api web web-install web-build dev test test-go test-web lint typecheck check \
        openapi openapi-check hooks lint-go fmt deadcode image

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ---- setup ----

env: ## Create .env from .env.example with generated secrets (never overwrites)
	@$(GO) run ./tools/devtool env

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

dev: db-up migrate ## Run the API and all four web apps with hot reload (URLs printed on start)
	@$(GO) run ./tools/devtool dev

# ---- checks ----

test-go: ## Go tests incl. database and S3 (throwaway PostgreSQL + MinIO containers)
	$(GO) run ./tools/devtool test

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
	$(LINT) fmt ./cmd/... ./internal/... ./migrations/... ./tools/devtool/...
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

# ---- container image ----

image: ## Build the Docker image justixauto:dev
	docker build -t justixauto:dev --build-arg VERSION=$$(git rev-parse --short HEAD) .
