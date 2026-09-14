# JustixAuto

Development handoff for Go microservices and React. Open **this folder**, not
the enclosing `startups` repository, as the Codex project.

This delivery contains reconciled business documentation, four runnable HTML
references, mock verification tools and project-scoped agent roles. It is not
yet a working Go/React application. Git initialization and architecture approval
are required before implementation.

## Start here

- [Business logic](docs/justix-auto/business-logic.md)
- [Open product decisions](docs/justix-auto/open-decisions.md)
- [Gaze reference audit](docs/justix-auto/reference/gaze-reference.md)
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

## Before development (user action)

The parent `/Users/bakhromachilov/startups` is already a Git repository. This
project must have its own root and initial commit before agents use worktrees:

```sh
cd /Users/bakhromachilov/startups/justixauto
git init -b main
git add .
git commit -m "Prepare development workspace"
npm run doctor
```

These Git commands have **not** been executed by the assistant. No remote has
been configured. `bash tools/go.sh version` selects an installed Go >= 1.23
(on this laptop Homebrew Go 1.24.2, since the default here is 1.22.2).
Use this wrapper for future Go commands, or explicitly set `JUSTIX_GO_BIN`;
no global PATH/toolchain changes are made. Open this folder in Codex, trust it
if prompted, and ask:

> Read AGENTS.md and dev-state.md. Start architecture planning with the
> architect agent using the current business logic and Gaze audit. Propose
> service boundaries, event/API contracts and the React setup for approval.
> Do not implement application features yet.

After architecture/backlog approval, ask the coordinator to implement approved
task IDs using backend/frontend workers and independent QA. No persistent Codex
daemon, personal config edits or API-key setup is required by this handoff.

## Layout

```text
.codex/agents/             bounded architect/backend/frontend/qa roles
AGENTS.md                 coordination, safety and evidence rules
docs/justix-auto/          business rules, decisions, mock map and dev state
docs/justix-auto/mocks/    minified runnable references + baseline tests
tools/                    mock build, verify, serve and environment checks
services/                 reserved for Go services after architecture approval
pkg/                      reserved for Go infrastructure shared across services
web/                      reserved for four React apps and shared UI packages
infra/                    local infrastructure constraints, not running services
```

Mock rebuilding requires the original readable source path:
`npm run mocks:build -- /absolute/path/to/prototype`. The builder refuses an
existing output directory; preserve/review the old generated bundle first.
Original source and its hashes are recorded in `mocks/manifest.json` and
`reference/source-manifest.json`. Original files were copied, not deleted/moved
out of spec-team. Shared dependencies remain single files and classic-script
order is preserved; only local variable names and whitespace are minified.
