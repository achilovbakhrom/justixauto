# Local infrastructure

`local/compose.yaml` runs the single PostgreSQL 18.6 database used by the
modular monolith ([ADR-14](../docs/justix-auto/adr-14-classic-modular-monolith.md)),
bound to loopback port 55432 with a named volume. Credentials come from the
git-ignored `.env` (see `.env.example`); never commit real secrets.

```sh
docker compose --env-file .env -f infra/local/compose.yaml up -d     # start
docker compose --env-file .env -f infra/local/compose.yaml stop      # stop, keep data
```

Deployment (Kubernetes, CI/CD) is not set up yet; see `AGENTS.md` in this folder.
