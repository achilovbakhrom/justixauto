# Deployment and local services

The only infrastructure folder. The backend and frontend run on your machine
(`make api`, `make web APP=…`). Their third-party services run in Docker.

## Local services

`local/compose.yaml` runs the single PostgreSQL 18.6 database used by the
modular monolith ([ADR-14](../docs/justix-auto/adr-14-classic-modular-monolith.md)),
bound to loopback port 55432 with a named volume. Credentials come from the
git-ignored `.env` (see `.env.example`); never commit real secrets.

```sh
make db-up      # start and wait until ready
make db-down    # stop, keep data
make db-reset   # delete the volume and start again
```

## Servers (Kubernetes)

This folder is also the Helm chart (`Chart.yaml`, `templates/`), see
[ADR-15](../docs/justix-auto/adr-15-kubernetes-multi-replica-observability.md).
`values-dev.yaml` holds non-production server values; `values-prod.yaml` is a
template (registry, image digest, S3 bucket, IRSA role, ingress host). Rollout to
any cluster is human-only (`AGENTS.md`).
