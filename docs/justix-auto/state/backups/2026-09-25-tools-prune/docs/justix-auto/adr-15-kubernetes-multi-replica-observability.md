# ADR-15 — Kubernetes packaging, multiple API replicas and observability

- Date: 2026-09-23
- Status: **requested by the user** (this session: "run 2 instances in kuber … setup
  kuber with loki, grafana, otel"; local kind cluster; dev and prod via a Helm chart)
- Builds on: [ADR-14](adr-14-classic-modular-monolith.md)

## Decision

| Concern | Choice |
|---|---|
| Image | One image (`Dockerfile`): API, `migrate` and the four built web apps; distroless `nonroot`, base images pinned by digest |
| Packaging | Helm chart `deploy/helm/justixauto`, `values-dev.yaml` (kind) and `values-prod.yaml` (template; human-only rollout) |
| Replicas | 2 by default, rolling update `maxUnavailable: 0`, PodDisruptionBudget `minAvailable: 1`, topology spread over nodes and zones, optional HPA |
| Migrations | Helm `pre-install,pre-upgrade` Job running `migrate up` before new pods start |
| Probes | `/healthz` liveness/startup (process), `/readyz` readiness (database ping) |
| Shutdown | `SHUTDOWN_DRAIN` keeps serving after SIGTERM while endpoints update, then graceful Echo shutdown within the 30 s grace period |
| Files | `FILE_STORAGE=s3` is mandatory with more than one replica (chart guard) |
| Telemetry | OpenTelemetry SDK in the API: OTLP/HTTP traces (span per request and per SQL statement) and metrics (`http.server.request.duration`); JSON logs on stdout with `trace_id`/`span_id` |
| Local stack | kind cluster `justixauto` (`infra/kind/up.sh`): PostgreSQL 18.6, MinIO, OpenTelemetry Collector (DaemonSet: OTLP + pod stdout), Loki (logs), Tempo (traces), Prometheus (metrics via remote write), Grafana (datasources linked logs ↔ traces, API dashboard) |

## Multi-replica review (2026-09-23)

The API keeps no per-process state; every shared fact lives in PostgreSQL.

| Area | Result |
|---|---|
| Sessions, CSRF, context | Rows in `identity.sessions`; any replica serves any request |
| Repeated writes | Idempotent by data: unique business keys, state checks, If-Match; no request-key ledger. Background jobs take a PostgreSQL advisory lock (`database.RunOnce`) so one replica runs them |
| Login lockout | **Fixed**: failed attempts were read-modify-written and could lose counts under parallel attempts on several replicas; now one atomic `UPDATE … failed_logins + 1` (test `TestLoginLockoutUnderConcurrency`) |
| Capacity, stock, orders, deals | Row locks (`SELECT … FOR UPDATE`) and version checks (If-Match) inside transactions |
| Uploaded files | Local directory is per pod → S3 required (guarded in the chart) |
| Background jobs, caches, timers | None |
| Migrations | golang-migrate holds a PostgreSQL advisory lock; one Job per release. Old pods run against the new schema during the rollout, so migrations must be backward compatible (expand → deploy → contract). All 17 current migrations predate the first release; 000002 replaced a constraint and an index of a table created in the same release, the rest only add schemas, tables, columns with defaults and indexes. From the first release on: additive only, and `CREATE INDEX CONCURRENTLY` (its own migration, no transaction) on large tables |
| Connection pool | 20 per replica; keep `replicas × 20` below PostgreSQL `max_connections` (100 by default), or add PgBouncer before scaling past ~4 replicas |

## Consequences

- Operating rules: never ship a migration that drops or renames something the
  previous release still uses; split it across two releases.
- The chart refuses a missing image digest or insecure cookies in
  `environment: prod`. (Two-factor authentication and `MFA_DISABLED` were
  removed on 2026-09-24; migration 000019 drops the MFA schema.)
- The Grafana Tempo single-binary chart is marked deprecated upstream; it is fine
  for the local stack. Production observability (managed Grafana/Loki/Tempo or
  `tempo-distributed`) needs its own environment decision.
- MinIO no longer publishes public images; the local stack loads an existing local
  `minio/minio` image into kind. Production uses AWS S3 (IRSA service-account role).
